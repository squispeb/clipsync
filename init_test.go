package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizePeer(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"100.64.0.2", "100.64.0.2:8275", true},
		{"100.64.0.2:9000", "100.64.0.2:9000", true},
		{"my-mac", "my-mac:8275", true},
		{"my-mac.tailnet.ts.net:8275", "my-mac.tailnet.ts.net:8275", true},
		{"", "", false},
		{"host:0", "", false},
		{"host:abc", "", false},
		{"http://host", "", false},
	}
	for _, c := range cases {
		got, err := normalizePeer(c.in, 8275)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("normalizePeer(%q) = %q, %v; want %q, ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
}

func TestInit_CreatesConfigWithTokenAndPeers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	t.Setenv("CLIPSYNC_TOKEN", "shared-secret")

	if err := cmdInit([]string{"--config", path, "--peer", "100.64.0.2, 100.64.0.3:9000", "--device", "box"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Token != "shared-secret" {
		t.Errorf("token = %q, want from CLIPSYNC_TOKEN", cfg.Token)
	}
	if len(cfg.Peers) != 2 || cfg.Peers[0] != "100.64.0.2:8275" || cfg.Peers[1] != "100.64.0.3:9000" {
		t.Errorf("peers = %v", cfg.Peers)
	}
	if cfg.AutoDiscover {
		t.Error("auto_discover should be off when peers are given")
	}
	if cfg.DeviceName != "box" {
		t.Errorf("device = %q", cfg.DeviceName)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(path); fi.Mode().Perm() != 0600 {
			t.Errorf("config mode = %v, want 0600", fi.Mode().Perm())
		}
	}
}

func TestInit_UpdateKeepsOtherFields(t *testing.T) {
	path := writeTestConfig(t, `{"port":8275,"token":"keep-me","peers":["a:8275"],"sync_interval_ms":250}`)
	os.Chmod(path, 0644)
	t.Setenv("CLIPSYNC_TOKEN", "")

	if err := cmdInit([]string{"--config", path, "--peer", "b"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	cfg, _ := LoadConfig(path)
	if cfg.Token != "keep-me" || cfg.SyncInterval != 250 {
		t.Errorf("token/interval not kept: %+v", cfg)
	}
	if len(cfg.Peers) != 1 || cfg.Peers[0] != "b:8275" {
		t.Errorf("peers = %v, want replaced with [b:8275]", cfg.Peers)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(path); fi.Mode().Perm() != 0600 {
			t.Errorf("existing config mode = %v, want tightened to 0600", fi.Mode().Perm())
		}
	}
}

func TestInit_NewTokenWithoutPeersEnablesDiscovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CLIPSYNC_TOKEN", "")

	if err := cmdInit([]string{"--config", path, "--new-token"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	cfg, _ := LoadConfig(path)
	if len(cfg.Token) != 48 {
		t.Errorf("generated token length = %d, want 48 hex chars", len(cfg.Token))
	}
	if !cfg.AutoDiscover {
		t.Error("auto_discover should be on with no peers")
	}

	t.Setenv("CLIPSYNC_TOKEN", "other")
	if err := cmdInit([]string{"--config", path, "--new-token"}); err == nil {
		t.Error("--new-token together with CLIPSYNC_TOKEN should fail")
	}
}
