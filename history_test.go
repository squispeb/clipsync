package main

import (
	"testing"
	"time"
)

func TestHistory_AddAndList(t *testing.T) {
	h := NewHistory(5, 1024)

	item1 := h.Add("hash1", "text/plain", []byte("hello"), "local", "dev1")
	if item1.Hash != "hash1" {
		t.Errorf("hash = %q, want hash1", item1.Hash)
	}
	if item1.Source != "local" {
		t.Errorf("source = %q, want local", item1.Source)
	}

	items := h.List(10)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if string(h.contents["hash1"]) != "hello" {
		t.Error("content not stored correctly")
	}
}

func TestHistory_Deduplication(t *testing.T) {
	h := NewHistory(10, 1024)

	h.Add("hash1", "text/plain", []byte("hello"), "local", "dev1")
	time.Sleep(10 * time.Millisecond)
	h.Add("hash1", "text/plain", []byte("hello"), "remote", "dev2")

	items := h.List(10)
	if len(items) != 1 {
		t.Fatalf("expected 1 item after dedup, got %d", len(items))
	}
	if items[0].Source != "remote" {
		t.Errorf("source should be updated to remote, got %q", items[0].Source)
	}
}

func TestHistory_MaxItems(t *testing.T) {
	h := NewHistory(3, 1024)

	h.Add("a", "text/plain", []byte("1"), "local", "")
	h.Add("b", "text/plain", []byte("2"), "local", "")
	h.Add("c", "text/plain", []byte("3"), "local", "")
	h.Add("d", "text/plain", []byte("4"), "local", "")

	items := h.List(10)
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	if items[0].Hash != "d" {
		t.Errorf("most recent = %q, want d", items[0].Hash)
	}
}

func TestHistory_MaxMemory(t *testing.T) {
	h := NewHistory(100, 10)

	h.Add("a", "text/plain", []byte("12345"), "local", "")
	h.Add("b", "text/plain", []byte("67890"), "local", "")

	items := h.List(10)
	// 5+5=10 bytes exactly fits within maxMemory=10, so both should be present
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2 (exact fit)", len(items))
	}

	// Adding a third item should trigger eviction
	h.Add("c", "text/plain", []byte("x"), "local", "")
	items = h.List(10)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2 (oldest evicted)", len(items))
	}
	if items[0].Hash != "c" {
		t.Errorf("most recent = %q, want c", items[0].Hash)
	}
}

func TestHistory_Get(t *testing.T) {
	h := NewHistory(10, 1024)

	h.Add("hash1", "text/plain", []byte("hello"), "local", "dev1")

	item, data, ok := h.Get("hash1")
	if !ok {
		t.Fatal("expected to find hash1")
	}
	if string(data) != "hello" {
		t.Errorf("data = %q, want hello", string(data))
	}
	if item.Device != "dev1" {
		t.Errorf("device = %q, want dev1", item.Device)
	}

	_, _, ok = h.Get("missing")
	if ok {
		t.Error("expected not to find missing hash")
	}
}

func TestHistory_Stats(t *testing.T) {
	h := NewHistory(10, 100)

	h.Add("a", "text/plain", []byte("12345"), "local", "")

	items, mem, max := h.Stats()
	if items != 1 {
		t.Errorf("items = %d, want 1", items)
	}
	if mem != 5 {
		t.Errorf("mem = %d, want 5", mem)
	}
	if max != 100 {
		t.Errorf("max = %d, want 100", max)
	}
}

func TestHistory_ListLimit(t *testing.T) {
	h := NewHistory(10, 1024)

	h.Add("a", "text/plain", []byte("1"), "local", "")
	h.Add("b", "text/plain", []byte("2"), "local", "")
	h.Add("c", "text/plain", []byte("3"), "local", "")

	items := h.List(2)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Hash != "c" {
		t.Errorf("first = %q, want c", items[0].Hash)
	}
}
