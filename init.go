package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// peerList collects repeated --peer flags; each may also be comma-separated.
type peerList []string

func (p *peerList) String() string { return strings.Join(*p, ",") }

func (p *peerList) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*p = append(*p, part)
		}
	}
	return nil
}

// normalizePeer validates a peer address and adds the default port when it
// is missing: "100.64.0.2" -> "100.64.0.2:8275", "my-mac" -> "my-mac:8275".
func normalizePeer(peer string, defaultPort int) (string, error) {
	if host, port, err := net.SplitHostPort(peer); err == nil {
		n, err := strconv.Atoi(port)
		if host == "" || err != nil || n <= 0 || n > 65535 {
			return "", fmt.Errorf("invalid peer %q (want host or host:port)", peer)
		}
		return peer, nil
	}
	if peer == "" || strings.ContainsAny(peer, "/ ") {
		return "", fmt.Errorf("invalid peer %q (want host or host:port)", peer)
	}
	return net.JoinHostPort(peer, strconv.Itoa(defaultPort)), nil
}

func newToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// cmdInit creates or updates the config file. It's the one setup step that's
// the same on every OS, so setup scripts and agents can call it
// non-interactively.
func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	configPath := fs.String("config", "", "config file path")
	token := fs.String("token", "", "shared token (or set CLIPSYNC_TOKEN, which keeps it out of shell history)")
	genToken := fs.Bool("new-token", false, "generate a new random token and print it")
	device := fs.String("device", "", "device name shown in logs (default: hostname)")
	port := fs.Int("port", 0, "listen port (default 8275)")
	discover := fs.Bool("discover", false, "also auto-discover clipsync peers on the tailnet (always on when no --peer is given)")
	var peers peerList
	fs.Var(&peers, "peer", "peer address host[:port], repeatable or comma-separated; replaces the existing peer list")
	fs.Parse(args)

	path := *configPath
	if path == "" {
		path = ConfigPath()
	}

	cfg := DefaultConfig()
	existed := false
	if _, err := os.Stat(path); err == nil {
		existed = true
		loaded, err := LoadConfig(path)
		if err != nil {
			return err
		}
		cfg = loaded
	}

	if *port > 0 {
		cfg.Port = *port
	}

	if *token == "" {
		*token = os.Getenv("CLIPSYNC_TOKEN")
	}
	switch {
	case *genToken && *token != "":
		return fmt.Errorf("use either --new-token or --token/CLIPSYNC_TOKEN, not both")
	case *genToken:
		t, err := newToken()
		if err != nil {
			return err
		}
		cfg.Token = t
	case *token != "":
		cfg.Token = *token
	}

	if *device != "" {
		cfg.DeviceName = *device
	} else if cfg.DeviceName == "" {
		if h, err := os.Hostname(); err == nil {
			cfg.DeviceName = strings.ToLower(strings.TrimSuffix(h, ".local"))
		}
	}

	if len(peers) > 0 {
		cfg.Peers = cfg.Peers[:0]
		for _, p := range peers {
			n, err := normalizePeer(p, cfg.Port)
			if err != nil {
				return err
			}
			cfg.Peers = MergePeerLists(cfg.Peers, []string{n})
		}
		cfg.AutoDiscover = *discover
	} else if len(cfg.Peers) == 0 || *discover {
		// Nothing to sync with otherwise.
		cfg.AutoDiscover = true
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := SaveConfig(path, cfg); err != nil {
		return err
	}

	verb := "created"
	if existed {
		verb = "updated"
	}
	fmt.Printf("%s %s\n", verb, path)
	fmt.Printf("  device:        %s\n", cfg.DeviceName)
	if len(cfg.Peers) > 0 {
		fmt.Printf("  peers:         %s\n", strings.Join(cfg.Peers, ", "))
	} else {
		fmt.Printf("  peers:         (none)\n")
	}
	fmt.Printf("  auto-discover: %v\n", cfg.AutoDiscover)
	if cfg.Token == "" {
		fmt.Println("  token:         NOT SET - any device on your tailnet can read and replace this clipboard")
		fmt.Println("                 run: clipsync init --new-token")
	} else {
		fmt.Println("  token:         set")
	}
	if *genToken {
		fmt.Println()
		fmt.Println("New token (every peer needs the same one):")
		fmt.Println(cfg.Token)
		fmt.Println()
		fmt.Println("On each other machine run:")
		fmt.Println("  CLIPSYNC_TOKEN=<token> clipsync init --peer <this machine's Tailscale IP>")
	}
	if ip, ok := TailscaleIP(); ok {
		fmt.Printf("\nThis machine's Tailscale address: %s:%d\n", ip, cfg.Port)
	}
	return nil
}

// cmdToken prints the configured token, for setting up the next machine:
//
//	CLIPSYNC_TOKEN=$(ssh first-machine clipsync token) clipsync init --peer ...
func cmdToken(args []string) error {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	path := *configPath
	if path == "" {
		path = ConfigPath()
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return fmt.Errorf("no token set in %s (run: clipsync init --new-token)", path)
	}
	fmt.Println(cfg.Token)
	return nil
}
