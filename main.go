package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const version = "0.2.0"

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
	fmt.Println("  daemon   Start the clipboard receiver and sync daemon")
	fmt.Println("  send     Manually broadcast clipboard to all peers")
	fmt.Println("  status   Check if peer daemons are reachable")
	fmt.Println("  peers    List configured peers")
	fmt.Println("  version  Print version")
	fmt.Println()
	fmt.Println("Config file: " + ConfigPath())
}

func loadConfig(flagConfig string) (Config, error) {
	path := flagConfig
	if path == "" {
		path = ConfigPath()
	}
	return LoadConfig(path)
}

func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	port := fs.Int("port", 0, "listen port (overrides config)")
	bind := fs.String("bind", "", "bind address (overrides config)")
	configPath := fs.String("config", "", "config file path")
	noSync := fs.Bool("no-sync", false, "disable auto-sync (receive-only mode)")
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

	engine := NewSyncEngine(board, cfg)
	srv := NewServer(board, cfg.Bind, cfg.Port, cfg.MaxSize, cfg.Token, engine.OnRemoteContent)

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

	engine := NewSyncEngine(board, cfg)
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
	return nil
}
