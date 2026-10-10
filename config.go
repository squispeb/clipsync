package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Peers         []string `json:"peers"`
	Bind          string   `json:"bind"`
	Port          int      `json:"port"`
	MaxSize       int64    `json:"max_size"`
	SyncInterval  int      `json:"sync_interval_ms"`
	Token         string   `json:"token"`
	DeviceName    string   `json:"device_name"`
	AutoDiscover  bool     `json:"auto_discover"`
	HistoryItems  int      `json:"history_max_items"`
	HistoryMemory int64    `json:"history_max_memory_mb"`
}

func DefaultConfig() Config {
	return Config{
		Peers:         []string{},
		Port:          8275,
		MaxSize:       10 * 1024 * 1024,
		SyncInterval:  500,
		AutoDiscover:  true,
		HistoryItems:  100,
		HistoryMemory: 50,
	}
}

func isTailscaleIP(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
}

// TailscaleIP returns this machine's Tailscale IPv4 address, if the tailnet
// interface is up.
func TailscaleIP() (string, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", false
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
			if isTailscaleIP(ipNet.IP) {
				return ipNet.IP.To4().String(), true
			}
		}
	}
	return "", false
}

func ResolveBind(bind string) string {
	if bind != "" {
		return bind
	}
	if ip, ok := TailscaleIP(); ok {
		log.Printf("auto-detected Tailscale IP: %s", ip)
		return ip
	}
	log.Println("WARNING: Tailscale IP not detected; binding to 127.0.0.1 (only local access)")
	return "127.0.0.1"
}

// WaitForTailscaleIP blocks until the Tailscale interface has an address.
// At login the daemon can start before tailscaled is up; binding to 127.0.0.1
// then would leave it running but unreachable until restarted.
func WaitForTailscaleIP(ctx context.Context) (string, error) {
	logged := false
	for {
		if ip, ok := TailscaleIP(); ok {
			log.Printf("auto-detected Tailscale IP: %s", ip)
			return ip, nil
		}
		if !logged {
			log.Println("waiting for a Tailscale IP (set \"bind\" to listen elsewhere)...")
			logged = true
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

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
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 500
	}

	return cfg, nil
}

func ConfigPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "clipsync", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "clipsync", "config.json")
}

// SaveConfig writes the config to disk as formatted JSON.
func SaveConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
