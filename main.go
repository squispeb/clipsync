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

const version = "0.1.0"

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
	case "version":
		fmt.Printf("cliplink %s\n", version)
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
	fmt.Println("cliplink - cross-platform clipboard sharing over Tailscale")
	fmt.Println()
	fmt.Println("Usage: cliplink <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  daemon   Start the clipboard receiver daemon")
	fmt.Println("  send     Send clipboard to peer")
	fmt.Println("  status   Check if peer daemon is reachable")
	fmt.Println("  version  Print version")
	fmt.Println()
	fmt.Println("Config file: " + ConfigPath())
}

// loadConfig parses the config file without command-specific validation.
// CLI flag overrides are applied by each command AFTER this call.
func loadConfig(flagConfig string) (Config, error) {
	path := flagConfig
	if path == "" {
		path = ConfigPath()
	}
	return LoadConfig(path)
}

// requirePeer validates that peer is set (needed by send/status, not daemon).
func requirePeer(cfg Config) error {
	if cfg.Peer == "" {
		return fmt.Errorf("peer address is required (set in config file or use --peer flag)")
	}
	return nil
}

func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	port := fs.Int("port", 0, "listen port (overrides config)")
	bind := fs.String("bind", "", "bind address (overrides config)")
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// Apply overrides AFTER loading
	if *port > 0 {
		cfg.Port = *port
	}
	if *bind != "" {
		cfg.Bind = *bind
	}
	// Auto-detect Tailscale IP if bind not specified
	cfg.Bind = ResolveBind(cfg.Bind)

	board, err := NewSystemBoard()
	if err != nil {
		return fmt.Errorf("clipboard init: %w", err)
	}

	srv := NewServer(board, cfg.Bind, cfg.Port, cfg.MaxSize)

	// Graceful shutdown: signal → shutdown HTTP server → exit
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

func cmdSend(args []string) error {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	peer := fs.String("peer", "", "peer address (overrides config)")
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// Apply override BEFORE validation
	if *peer != "" {
		cfg.Peer = *peer
	}
	if err := requirePeer(cfg); err != nil {
		return err
	}

	board, err := NewSystemBoard()
	if err != nil {
		return fmt.Errorf("clipboard init: %w", err)
	}

	client := NewClient(cfg.Peer, board, cfg.MaxSize)
	if err := client.Send(); err != nil {
		return err
	}

	fmt.Println("sent")
	return nil
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	peer := fs.String("peer", "", "peer address (overrides config)")
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if *peer != "" {
		cfg.Peer = *peer
	}
	if err := requirePeer(cfg); err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s/health", cfg.Peer)
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout: 2 * time.Second,
			}).DialContext,
		},
	}
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("peer unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("peer error (%d): %s", resp.StatusCode, string(body))
	}
	fmt.Printf("peer ok: %s\n", string(body))
	return nil
}
