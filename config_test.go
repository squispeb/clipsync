package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"peer":"100.64.0.2:8275","port":9999,"max_size":5000}`), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Peer != "100.64.0.2:8275" {
		t.Errorf("peer = %q, want %q", cfg.Peer, "100.64.0.2:8275")
	}
	if cfg.Port != 9999 {
		t.Errorf("port = %d, want %d", cfg.Port, 9999)
	}
	if cfg.MaxSize != 5000 {
		t.Errorf("max_size = %d, want %d", cfg.MaxSize, 5000)
	}
}

func TestLoadConfig_DefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"peer":"100.64.0.2:8275"}`), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8275 {
		t.Errorf("default port = %d, want %d", cfg.Port, 8275)
	}
	if cfg.MaxSize != 10*1024*1024 {
		t.Errorf("default max_size = %d, want %d", cfg.MaxSize, 10*1024*1024)
	}
}

func TestLoadConfig_NoPeerIsOK(t *testing.T) {
	// peer is command-specific; LoadConfig should NOT reject missing peer
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"port":8275}`), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8275 {
		t.Errorf("port = %d, want %d", cfg.Port, 8275)
	}
}

func TestLoadConfig_InvalidMaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"peer":"x","max_size":0}`), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for zero max_size, got nil")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`not json`), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestIsTailscaleIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.0", true},      // start of CGNAT range
		{"100.64.0.1", true},      // typical Tailscale IP
		{"100.100.100.100", true}, // Tailscale magic IP
		{"100.127.255.255", true}, // end of CGNAT range
		{"100.63.255.255", false}, // just below range
		{"100.128.0.0", false},    // just above range
		{"10.0.0.1", false},       // private but not CGNAT
		{"192.168.1.1", false},    // local network
		{"8.8.8.8", false},        // public IP
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got := isTailscaleIP(net.ParseIP(tt.ip))
			if got != tt.want {
				t.Errorf("isTailscaleIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestResolveBind_ExplicitValue(t *testing.T) {
	got := ResolveBind("192.168.1.100")
	if got != "192.168.1.100" {
		t.Errorf("ResolveBind with explicit value = %q, want %q", got, "192.168.1.100")
	}
}

func TestResolveBind_AutoDetect(t *testing.T) {
	got := ResolveBind("")
	if got == "" {
		t.Fatal("ResolveBind('') returned empty string, want an IP address")
	}
	// Should return either a Tailscale IP or 127.0.0.1 fallback
	ip := net.ParseIP(got)
	if ip == nil {
		t.Fatalf("ResolveBind('') returned invalid IP: %q", got)
	}
}

func TestConfigPath(t *testing.T) {
	got := ConfigPath()
	if got == "" {
		t.Fatal("ConfigPath() returned empty string")
	}
	if !strings.Contains(got, "cliplink") {
		t.Errorf("ConfigPath() = %q, want to contain 'cliplink'", got)
	}
}
