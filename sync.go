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
	peers    []string
	maxSize  int64
	token    string
	device   string
	client   *http.Client
	interval time.Duration

	mu             sync.Mutex
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
	if len(e.peers) == 0 {
		log.Println("sync engine: no peers configured, only receiving")
	}
	log.Printf("sync engine started (%d peers, interval %v)", len(e.peers), e.interval)
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
	e.mu.Unlock()

	if isNewLocal {
		if e.device != "" {
			log.Printf("[%s] clipboard changed (%s, %d bytes), broadcasting to %d peers", e.device, contentType, len(data), len(e.peers))
		} else {
			log.Printf("clipboard changed (%s, %d bytes), broadcasting to %d peers", contentType, len(data), len(e.peers))
		}
		e.sendToPeers(data, contentType)
	}
}

func (e *SyncEngine) sendToPeers(data []byte, contentType string) {
	var wg sync.WaitGroup
	for _, peer := range e.peers {
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
	e.mu.Unlock()

	e.sendToPeers(data, contentType)
	return nil
}
