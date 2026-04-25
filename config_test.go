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
	os.WriteFile(path, []byte(`{"peers":["100.64.0.2:8275"],"port":9999,"max_size":5000,"sync_interval_ms":1000}`), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Peers) != 1 || cfg.Peers[0] != "100.64.0.2:8275" {
		t.Errorf("peers = %v, want [100.64.0.2:8275]", cfg.Peers)
	}
	if cfg.Port != 9999 {
		t.Errorf("port = %d, want %d", cfg.Port, 9999)
	}
	if cfg.MaxSize != 5000 {
		t.Errorf("max_size = %d, want %d", cfg.MaxSize, 5000)
	}
	if cfg.SyncInterval != 1000 {
		t.Errorf("sync_interval_ms = %d, want %d", cfg.SyncInterval, 1000)
	}
}

func TestLoadConfig_DefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"peers":["100.64.0.2:8275"]}`), 0644)

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
	if cfg.SyncInterval != 500 {
		t.Errorf("default sync_interval_ms = %d, want %d", cfg.SyncInterval, 500)
	}
}

func TestLoadConfig_NoPeersIsOK(t *testing.T) {
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
	if len(cfg.Peers) != 0 {
		t.Errorf("peers = %v, want empty", cfg.Peers)
	}
}

func TestLoadConfig_InvalidMaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"peers":["x"],"max_size":0}`), 0644)

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
		{"100.64.0.0", true},
		{"100.64.0.1", true},
		{"100.100.100.100", true},
		{"100.127.255.255", true},
		{"100.63.255.255", false},
		{"100.128.0.0", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"8.8.8.8", false},
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
	if !strings.Contains(got, "clipsync") {
		t.Errorf("ConfigPath() = %q, want to contain 'clipsync'", got)
	}
}
