package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	peer    string
	board   Board
	maxSize int64
	http    *http.Client
}

func NewClient(peer string, board Board, maxSize int64) *Client {
	return &Client{
		peer:    peer,
		board:   board,
		maxSize: maxSize,
		http: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{Proxy: nil}, // bypass system proxy — always LAN
		},
	}
}

func (c *Client) Send() error {
	data, contentType, err := c.readClipboard()
	if err != nil {
		return err
	}

	if int64(len(data)) > c.maxSize {
		return fmt.Errorf("clipboard data too large (%d bytes, max %d)", len(data), c.maxSize)
	}

	url := fmt.Sprintf("http://%s/clip", c.peer)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send failed (is the peer daemon running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("peer rejected (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// readClipboard reads the system clipboard content.
// Priority: text first, then image.
// Distinguishes "empty clipboard" from actual read errors.
func (c *Client) readClipboard() (data []byte, contentType string, err error) {
	text, textErr := c.board.ReadText()
	if textErr == nil && len(text) > 0 {
		return text, "text/plain; charset=utf-8", nil
	}

	img, imgErr := c.board.ReadImage()
	if imgErr == nil && len(img) > 0 {
		return img, "image/png", nil
	}

	// Both failed — if both returned real errors, propagate them
	if textErr != nil && imgErr != nil {
		return nil, "", fmt.Errorf("clipboard read failed (text: %v; image: %v)", textErr, imgErr)
	}
	return nil, "", fmt.Errorf("clipboard is empty")
}
