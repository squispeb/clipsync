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

func TestLoadConfig_OverridePeersAfterLoad(t *testing.T) {
	path := writeTestConfig(t, `{"port":8275}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.Peers) != 0 {
		t.Fatalf("expected empty peers from config, got %v", cfg.Peers)
	}

	cfg.Peers = []string{"100.64.0.5:8275"}
	if len(cfg.Peers) != 1 {
		t.Fatalf("expected 1 peer after override, got %d", len(cfg.Peers))
	}
}

func TestDaemonDoesNotRequirePeers(t *testing.T) {
	path := writeTestConfig(t, `{"port":9000}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Port != 9000 {
		t.Errorf("port = %d, want 9000", cfg.Port)
	}
}

func TestConfigOverrides_PortAndBind(t *testing.T) {
	path := writeTestConfig(t, `{"peers":["100.64.0.1:8275"],"port":8275,"bind":"0.0.0.0"}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	cfg.Port = 9999
	cfg.Bind = "100.64.0.2"

	if cfg.Port != 9999 {
		t.Errorf("port after override = %d, want 9999", cfg.Port)
	}
	if cfg.Bind != "100.64.0.2" {
		t.Errorf("bind after override = %q, want %q", cfg.Bind, "100.64.0.2")
	}
}
