package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const version = "0.4.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "daemon":
		err = cmdDaemon(os.Args[2:])
	case "send":
		err = cmdSend(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "peers":
		err = cmdPeers(os.Args[2:])
	case "discover":
		err = cmdDiscover(os.Args[2:])
	case "history":
		err = cmdHistory(os.Args[2:])
	case "version":
		fmt.Printf("clipsync %s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("clipsync - cross-platform clipboard sharing over Tailscale")
	fmt.Println()
	fmt.Println("Usage: clipsync <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  daemon     Start the clipboard receiver and sync daemon")
	fmt.Println("  send       Manually broadcast clipboard to all peers")
	fmt.Println("  status     Check if peer daemons are reachable")
	fmt.Println("  peers      List configured peers")
	fmt.Println("  discover   Scan tailnet for clipsync peers")
	fmt.Println("  history    Show clipboard history")
	fmt.Println("  version    Print version")
	fmt.Println()
	fmt.Println("Config file: " + ConfigPath())
}

func loadConfig(flagConfig string) (Config, error) {
	path := flagConfig
	if path == "" {
		path = ConfigPath()
	}
	if err := EnsureConfig(path); err != nil {
		return Config{}, err
	}
	return LoadConfig(path)
}

func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	port := fs.Int("port", 0, "listen port (overrides config)")
	bind := fs.String("bind", "", "bind address (overrides config)")
	configPath := fs.String("config", "", "config file path")
	noSync := fs.Bool("no-sync", false, "disable auto-sync (receive-only mode)")
	noDiscover := fs.Bool("no-discover", false, "disable auto-discovery")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if *port > 0 {
		cfg.Port = *port
	}
	if *bind != "" {
		cfg.Bind = *bind
	}
	cfg.Bind = ResolveBind(cfg.Bind)

	board, err := NewSystemBoard()
	if err != nil {
		return fmt.Errorf("clipboard init: %w", err)
	}

	// Auto-discovery
	if cfg.AutoDiscover && !*noDiscover {
		discovered, err := DiscoverPeers(cfg.Port, cfg.Token, 3*time.Second)
		if err != nil {
			log.Printf("auto-discovery failed: %v", err)
		} else {
			if len(discovered) > 0 {
				log.Printf("auto-discovered %d peer(s): %v", len(discovered), discovered)
			}
			cfg.Peers = MergePeerLists(cfg.Peers, discovered)
		}
	}

	history := NewHistory(cfg.HistoryItems, cfg.HistoryMemory*1024*1024)
	engine := NewSyncEngine(board, cfg, history)
	srv := NewServer(board, cfg.Bind, cfg.Port, cfg.MaxSize, cfg.Token, history, cfg.DeviceName, engine.OnRemoteContent)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if !*noSync {
		go engine.Start(ctx)
	}

	// Periodic re-discovery
	if cfg.AutoDiscover && !*noDiscover {
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					discovered, err := DiscoverPeers(cfg.Port, cfg.Token, 3*time.Second)
					if err != nil {
						continue
					}
					newPeers := MergePeerLists(cfg.Peers, discovered)
					engine.SetPeers(newPeers)
				}
			}
		}()
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

func cmdSend(args []string) error {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if len(cfg.Peers) == 0 {
		return fmt.Errorf("no peers configured")
	}

	board, err := NewSystemBoard()
	if err != nil {
		return fmt.Errorf("clipboard init: %w", err)
	}

	history := NewHistory(cfg.HistoryItems, cfg.HistoryMemory*1024*1024)
	engine := NewSyncEngine(board, cfg, history)
	if err := engine.SendNow(); err != nil {
		return err
	}

	fmt.Printf("broadcasted to %d peer(s)\n", len(cfg.Peers))
	return nil
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if len(cfg.Peers) == 0 {
		return fmt.Errorf("no peers configured")
	}

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout: 2 * time.Second,
			}).DialContext,
		},
	}

	allOK := true
	for _, peer := range cfg.Peers {
		url := fmt.Sprintf("http://%s/health", peer)
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		if cfg.Token != "" {
			req.Header.Set("X-ClipSync-Token", cfg.Token)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			fmt.Printf("  %s  unreachable (%v)\n", peer, err)
			allOK = false
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("  %s  ok (%s)\n", peer, string(body))
		} else {
			fmt.Printf("  %s  error %d\n", peer, resp.StatusCode)
			allOK = false
		}
	}

	if !allOK {
		return fmt.Errorf("some peers are unreachable")
	}
	return nil
}

func cmdPeers(args []string) error {
	fs := flag.NewFlagSet("peers", flag.ExitOnError)
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if len(cfg.Peers) == 0 {
		fmt.Println("No peers configured.")
		return nil
	}

	fmt.Printf("Configured peers (%d):\n", len(cfg.Peers))
	for _, p := range cfg.Peers {
		fmt.Printf("  - %s\n", p)
	}
	fmt.Printf("\nListen address: %s:%d\n", cfg.Bind, cfg.Port)
	fmt.Printf("Sync interval: %d ms\n", cfg.SyncInterval)
	fmt.Printf("Auto-discover: %v\n", cfg.AutoDiscover)
	return nil
}

func cmdDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	port := fs.Int("port", 8275, "port to probe")
	token := fs.String("token", "", "auth token")
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	// If config exists, load defaults from it
	cfg := DefaultConfig()
	if *configPath != "" {
		if loaded, err := LoadConfig(*configPath); err == nil {
			cfg = loaded
		}
	} else {
		if loaded, err := LoadConfig(ConfigPath()); err == nil {
			cfg = loaded
		}
	}
	if *port > 0 {
		cfg.Port = *port
	}
	if *token != "" {
		cfg.Token = *token
	}

	fmt.Println("Scanning tailnet for clipsync peers...")
	peers, err := DiscoverPeers(cfg.Port, cfg.Token, 3*time.Second)
	if err != nil {
		return err
	}
	if len(peers) == 0 {
		fmt.Println("No clipsync peers found on your tailnet.")
		fmt.Println("Make sure clipsync daemon is running on other devices.")
		return nil
	}

	fmt.Printf("Found %d peer(s):\n", len(peers))
	for _, p := range peers {
		fmt.Printf("  - %s\n", p)
	}
	return nil
}

func cmdHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	limit := fs.Int("limit", 20, "max items to show")
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Try to connect to local daemon's history API
	bind := ResolveBind(cfg.Bind)
	if bind == "127.0.0.1" && cfg.Bind != "" {
		bind = cfg.Bind
	}
	url := fmt.Sprintf("http://%s:%d/history?limit=%d", bind, cfg.Port, *limit)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if cfg.Token != "" {
		req.Header.Set("X-ClipSync-Token", cfg.Token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot connect to local daemon (is it running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("daemon error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items []HistoryItem `json:"items"`
		Count int           `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if len(result.Items) == 0 {
		fmt.Println("No clipboard history available.")
		return nil
	}

	fmt.Printf("Clipboard history (%d items):\n\n", result.Count)
	for i, item := range result.Items {
		timeStr := item.Timestamp.Format("15:04:05")
		source := item.Source
		if source == "" {
			source = "unknown"
		}
		if item.Device != "" {
			source = fmt.Sprintf("%s (%s)", source, item.Device)
		}

		preview := ""
		if item.ContentType == "text/plain; charset=utf-8" {
			preview = "[text]"
		} else if strings.HasPrefix(item.ContentType, "image/") {
			preview = "[image]"
		} else {
			preview = fmt.Sprintf("[%s]", item.ContentType)
		}

		fmt.Printf("  %d. %s  %s  %s  %d bytes  %s\n",
			i+1,
			timeStr,
			item.Hash[:8],
			preview,
			item.Size,
			source,
		)
	}
	return nil
}
