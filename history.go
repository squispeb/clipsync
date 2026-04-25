package main

import (
	"encoding/json"
	"sync"
	"time"
)

// HistoryItem represents a single clipboard entry in history.
type HistoryItem struct {
	Hash        string    `json:"hash"`
	ContentType string    `json:"content_type"`
	Size        int       `json:"size"`
	Source      string    `json:"source"`
	Timestamp   time.Time `json:"timestamp"`
	Device      string    `json:"device,omitempty"`
}

// History stores clipboard items in a thread-safe ring buffer.
type History struct {
	mu         sync.RWMutex
	items      []HistoryItem
	contents   map[string][]byte
	maxItems   int
	maxMemory  int64
	currentMem int64
}

// NewHistory creates a new history store with limits.
func NewHistory(maxItems int, maxMemory int64) *History {
	if maxItems <= 0 {
		maxItems = 100
	}
	if maxMemory <= 0 {
		maxMemory = 50 * 1024 * 1024 // 50 MB default
	}
	return &History{
		items:     make([]HistoryItem, 0, maxItems),
		contents:  make(map[string][]byte),
		maxItems:  maxItems,
		maxMemory: maxMemory,
	}
}

// Add stores a clipboard item in history. If the item already exists, it updates
// the timestamp and moves it to the front. Returns the HistoryItem.
func (h *History) Add(hash string, contentType string, data []byte, source string, device string) HistoryItem {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If already exists, remove old entry first
	if _, ok := h.contents[hash]; ok {
		h.removeByHash(hash)
	}

	item := HistoryItem{
		Hash:        hash,
		ContentType: contentType,
		Size:        len(data),
		Source:      source,
		Timestamp:   time.Now(),
		Device:      device,
	}

	// Make room if needed
	itemMem := int64(len(data))
	for len(h.items) >= h.maxItems || (h.currentMem+itemMem > h.maxMemory && len(h.items) > 0) {
		h.evictOldest()
	}

	h.items = append([]HistoryItem{item}, h.items...)
	h.contents[hash] = data
	h.currentMem += itemMem

	return item
}

// Get retrieves a history item and its raw content by hash.
func (h *History) Get(hash string) (HistoryItem, []byte, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, item := range h.items {
		if item.Hash == hash {
			data, ok := h.contents[hash]
			return item, data, ok
		}
	}
	return HistoryItem{}, nil, false
}

// List returns the history items, most recent first.
func (h *History) List(limit int) []HistoryItem {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit <= 0 || limit > len(h.items) {
		limit = len(h.items)
	}

	result := make([]HistoryItem, limit)
	copy(result, h.items[:limit])
	return result
}

// Count returns the total number of items in history.
func (h *History) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.items)
}

// Stats returns memory usage info.
func (h *History) Stats() (items int, memory int64, maxMemory int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.items), h.currentMem, h.maxMemory
}

func (h *History) removeByHash(hash string) {
	for i, item := range h.items {
		if item.Hash == hash {
			// Remove from slice
			h.items = append(h.items[:i], h.items[i+1:]...)
			if data, ok := h.contents[hash]; ok {
				h.currentMem -= int64(len(data))
				delete(h.contents, hash)
			}
			return
		}
	}
}

func (h *History) evictOldest() {
	if len(h.items) == 0 {
		return
	}
	oldest := h.items[len(h.items)-1]
	h.items = h.items[:len(h.items)-1]
	if data, ok := h.contents[oldest.Hash]; ok {
		h.currentMem -= int64(len(data))
		delete(h.contents, oldest.Hash)
	}
}

// MarshalJSON implements custom JSON serialization without content bytes.
func (h *History) MarshalJSON() ([]byte, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return json.Marshal(struct {
		Items     []HistoryItem `json:"items"`
		Count     int           `json:"count"`
		Memory    int64         `json:"memory_bytes"`
		MaxMemory int64         `json:"max_memory_bytes"`
	}{
		Items:     h.items,
		Count:     len(h.items),
		Memory:    h.currentMem,
		MaxMemory: h.maxMemory,
	})
}
