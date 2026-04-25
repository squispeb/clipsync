package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// tailscaleStatus represents the JSON output of `tailscale status --json`.
type tailscaleStatus struct {
	Self struct {
		HostName     string   `json:"HostName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
	Peer map[string]struct {
		HostName     string   `json:"HostName"`
		OS           string   `json:"OS"`
		TailscaleIPs []string `json:"TailscaleIPs"`
		Online       bool     `json:"Online"`
	} `json:"Peer"`
}

// DiscoverPeers runs `tailscale status --json`, finds online peers, and probes
// each one on the given port to see if a clipsync daemon is listening.
// It returns a deduplicated list of reachable peer addresses.
func DiscoverPeers(port int, token string, timeout time.Duration) ([]string, error) {
	cmd := exec.Command("tailscale", "status", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale status failed (is tailscale installed and running?): %w", err)
	}

	var status tailscaleStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parse tailscale status: %w", err)
	}

	selfIPs := make(map[string]bool)
	for _, ip := range status.Self.TailscaleIPs {
		selfIPs[ip] = true
	}

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout: timeout,
			}).DialContext,
		},
	}

	var peers []string
	seen := make(map[string]bool)

	for _, peer := range status.Peer {
		if !peer.Online {
			continue
		}
		for _, ip := range peer.TailscaleIPs {
			// Skip self
			if selfIPs[ip] {
				continue
			}
			// Only IPv4 for simplicity (Tailscale gives both v4 and v6)
			if net.ParseIP(ip).To4() == nil {
				continue
			}
			addr := fmt.Sprintf("%s:%d", ip, port)
			if seen[addr] {
				continue
			}
			seen[addr] = true

			if isClipSyncPeer(httpClient, addr, token) {
				peers = append(peers, addr)
			}
		}
	}

	return peers, nil
}

func isClipSyncPeer(client *http.Client, addr, token string) bool {
	url := fmt.Sprintf("http://%s/health", addr)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("X-ClipSync-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Quick sanity check: read a bit of body to ensure it's JSON
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return strings.Contains(string(body), `"status"`) || strings.Contains(string(body), "ok")
}

// EnsureConfig creates a default config file if it doesn't exist.
func EnsureConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	cfg := DefaultConfig()
	cfg.AutoDiscover = true
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write default config: %w", err)
	}
	log.Printf("created default config at %s", path)
	return nil
}

// MergePeerLists combines manual peers with discovered peers, deduplicating.
func MergePeerLists(manual, discovered []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range manual {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	for _, p := range discovered {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}
