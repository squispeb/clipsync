package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
)

type Config struct {
	Peer    string `json:"peer"`
	Bind    string `json:"bind"`
	Port    int    `json:"port"`
	MaxSize int64  `json:"max_size"`
}

func DefaultConfig() Config {
	return Config{
		// Bind intentionally empty — daemon auto-detects Tailscale IP.
		// User can override with explicit IP or "0.0.0.0" for all interfaces.
		Port:    8275,
		MaxSize: 10 * 1024 * 1024,
	}
}

// ResolveBind determines the bind address for the daemon.
// If bind is already set (from config or --bind flag), use it as-is.
// Otherwise, auto-detect the Tailscale interface IP (100.64.0.0/10 CGNAT range).
// Falls back to 127.0.0.1 with a warning if Tailscale is not found.
func ResolveBind(bind string) string {
	if bind != "" {
		return bind
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		log.Printf("WARNING: cannot list interfaces: %v; binding to 127.0.0.1", err)
		return "127.0.0.1"
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}
			// Tailscale CGNAT range: 100.64.0.0/10 (100.64.x.x - 100.127.x.x)
			if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
				log.Printf("auto-detected Tailscale IP: %s", ip.String())
				return ip.String()
			}
		}
	}

	log.Println("WARNING: Tailscale IP not detected; binding to 127.0.0.1 (only local access)")
	return "127.0.0.1"
}

// LoadConfig parses the config file and applies defaults.
// It validates structural fields (port, max_size) but NOT command-specific
// fields like peer — the caller validates those based on which command runs.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Port <= 0 || cfg.Port > 65535 {
		return cfg, fmt.Errorf("config: invalid port %d", cfg.Port)
	}
	if cfg.MaxSize <= 0 {
		return cfg, fmt.Errorf("config: max_size must be positive, got %d", cfg.MaxSize)
	}

	return cfg, nil
}

// ConfigPath returns the platform-appropriate config file location.
// Uses os.UserConfigDir which returns:
//   - macOS: ~/Library/Application Support
//   - Windows: %AppData%
//   - Linux: $XDG_CONFIG_HOME or ~/.config
func ConfigPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "cliplink", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "cliplink", "config.json")
}
