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
	"sort"
	"sync"
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
	cli := tailscaleCLI()
	if cli == "" {
		return nil, fmt.Errorf("tailscale CLI not found")
	}
	cmd := exec.Command(cli, "status", "--json")
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

	var candidates []string
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
			if !seen[addr] {
				seen[addr] = true
				candidates = append(candidates, addr)
			}
		}
	}

	// Probe in parallel: offline or firewalled peers each take the full
	// timeout, and startup waits for discovery.
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		peers []string
	)
	for _, addr := range candidates {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			if isClipSyncPeer(httpClient, addr, token) {
				mu.Lock()
				peers = append(peers, addr)
				mu.Unlock()
			}
		}(addr)
	}
	wg.Wait()
	sort.Strings(peers)
	return peers, nil
}

type healthInfo struct {
	Status     string `json:"status"`
	Service    string `json:"service"`
	Version    string `json:"version"`
	Device     string `json:"device"`
	Authorized bool   `json:"authorized"`
}

// isClipSyncPeer accepts only a clipsync daemon that accepts our token.
// Anything else on the port (another web server answering "ok") would
// otherwise receive every clipboard copy.
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

	var health healthInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&health); err != nil {
		return false
	}
	return health.Service == "clipsync" && health.Authorized
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

	if err := os.WriteFile(path, data, 0600); err != nil {
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
