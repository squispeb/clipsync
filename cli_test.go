package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(content), 0644)
	return path
}

func TestLoadConfig_OverridePeerAfterLoad(t *testing.T) {
	// Simulate: config file has no peer, CLI flag provides it
	path := writeTestConfig(t, `{"port":8275}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Peer != "" {
		t.Fatalf("expected empty peer from config, got %q", cfg.Peer)
	}

	// Simulate CLI override
	cfg.Peer = "100.64.0.5:8275"
	if err := requirePeer(cfg); err != nil {
		t.Fatalf("requirePeer failed after override: %v", err)
	}
}

func TestRequirePeer_Empty(t *testing.T) {
	cfg := DefaultConfig() // peer is ""
	if err := requirePeer(cfg); err == nil {
		t.Fatal("expected error for empty peer, got nil")
	}
}

func TestRequirePeer_Set(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Peer = "100.64.0.1:8275"
	if err := requirePeer(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDaemonDoesNotRequirePeer(t *testing.T) {
	// daemon only needs port — config with no peer should load fine
	path := writeTestConfig(t, `{"port":9000}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Port != 9000 {
		t.Errorf("port = %d, want 9000", cfg.Port)
	}
	// No requirePeer call — daemon doesn't need it
}

func TestConfigOverrides_PortAndBind(t *testing.T) {
	path := writeTestConfig(t, `{"peer":"100.64.0.1:8275","port":8275,"bind":"0.0.0.0"}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Simulate CLI overrides
	cfg.Port = 9999
	cfg.Bind = "100.64.0.2"

	if cfg.Port != 9999 {
		t.Errorf("port after override = %d, want 9999", cfg.Port)
	}
	if cfg.Bind != "100.64.0.2" {
		t.Errorf("bind after override = %q, want %q", cfg.Bind, "100.64.0.2")
	}
}
