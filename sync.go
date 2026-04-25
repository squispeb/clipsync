package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type SyncEngine struct {
	board    Board
	maxSize  int64
	token    string
	device   string
	client   *http.Client
	interval time.Duration

	mu             sync.RWMutex
	peers          []string
	lastLocalHash  string
	lastRemoteHash string
}

func NewSyncEngine(board Board, cfg Config) *SyncEngine {
	interval := time.Duration(cfg.SyncInterval) * time.Millisecond
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &SyncEngine{
		board:    board,
		peers:    cfg.Peers,
		maxSize:  cfg.MaxSize,
		token:    cfg.Token,
		device:   cfg.DeviceName,
		interval: interval,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
				DialContext: (&net.Dialer{
					Timeout: 2 * time.Second,
				}).DialContext,
			},
		},
	}
}

func hashContent(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (e *SyncEngine) Start(ctx context.Context) {
	e.mu.RLock()
	peerCount := len(e.peers)
	e.mu.RUnlock()
	if peerCount == 0 {
		log.Println("sync engine: no peers configured, only receiving")
	}
	log.Printf("sync engine started (%d peers, interval %v)", peerCount, e.interval)
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("sync engine stopping...")
			return
		case <-ticker.C:
			e.syncOnce()
		}
	}
}

func (e *SyncEngine) syncOnce() {
	data, contentType, err := readClipboard(e.board)
	if err != nil {
		return
	}
	if int64(len(data)) > e.maxSize {
		return
	}

	h := hashContent(data)

	e.mu.Lock()
	isNewLocal := h != e.lastLocalHash && h != e.lastRemoteHash
	if isNewLocal {
		e.lastLocalHash = h
	}
	peers := make([]string, len(e.peers))
	copy(peers, e.peers)
	e.mu.Unlock()

	if isNewLocal {
		if e.device != "" {
			log.Printf("[%s] clipboard changed (%s, %d bytes), broadcasting to %d peers", e.device, contentType, len(data), len(peers))
		} else {
			log.Printf("clipboard changed (%s, %d bytes), broadcasting to %d peers", contentType, len(data), len(peers))
		}
		e.sendToPeers(peers, data, contentType)
	}
}

func (e *SyncEngine) sendToPeers(peers []string, data []byte, contentType string) {
	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			if err := e.sendToPeer(p, data, contentType); err != nil {
				log.Printf("send to %s failed: %v", p, err)
			}
		}(peer)
	}
	wg.Wait()
}

func (e *SyncEngine) sendToPeer(peer string, data []byte, contentType string) error {
	url := fmt.Sprintf("http://%s/clip", peer)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if e.token != "" {
		req.Header.Set("X-ClipSync-Token", e.token)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("peer rejected (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (e *SyncEngine) OnRemoteContent(data []byte, contentType string) {
	h := hashContent(data)
	e.mu.Lock()
	e.lastRemoteHash = h
	e.lastLocalHash = h
	e.mu.Unlock()

	if e.device != "" {
		log.Printf("[%s] received remote clipboard (%s, %d bytes)", e.device, contentType, len(data))
	} else {
		log.Printf("received remote clipboard (%s, %d bytes)", contentType, len(data))
	}
}

func (e *SyncEngine) SetPeers(peers []string) {
	e.mu.Lock()
	changed := len(peers) != len(e.peers)
	if !changed {
		for i := range peers {
			if peers[i] != e.peers[i] {
				changed = true
				break
			}
		}
	}
	if changed {
		log.Printf("sync engine: peer list updated (%d peers)", len(peers))
	}
	e.peers = peers
	e.mu.Unlock()
}

func (e *SyncEngine) SendNow() error {
	data, contentType, err := readClipboard(e.board)
	if err != nil {
		return err
	}
	if int64(len(data)) > e.maxSize {
		return fmt.Errorf("clipboard data too large (%d bytes, max %d)", len(data), e.maxSize)
	}
	h := hashContent(data)
	e.mu.Lock()
	e.lastLocalHash = h
	peers := make([]string, len(e.peers))
	copy(peers, e.peers)
	e.mu.Unlock()

	e.sendToPeers(peers, data, contentType)
	return nil
}
