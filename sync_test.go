package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// countingPeer records how many clips it receives.
func countingPeer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func waitCount(n *int32, want int32) int32 {
	for i := 0; i < 100 && atomic.LoadInt32(n) < want; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // catch extra sends
	return atomic.LoadInt32(n)
}

func newTestEngine(board Board, peer *httptest.Server) *SyncEngine {
	cfg := DefaultConfig()
	cfg.Peers = []string{strings.TrimPrefix(peer.URL, "http://")}
	return NewSyncEngine(board, cfg, nil, "test")
}

func TestSync_PrimeSkipsStartupContent(t *testing.T) {
	peer, n := countingPeer(t)
	board := &MockBoard{text: []byte("already here")}
	e := newTestEngine(board, peer)

	e.prime()
	e.syncOnce()
	if got := waitCount(n, 0); got != 0 {
		t.Fatalf("startup content was broadcast %d time(s), want 0", got)
	}

	board.setText([]byte("new copy"))
	e.syncOnce()
	if got := waitCount(n, 1); got != 1 {
		t.Fatalf("new copy was broadcast %d time(s), want 1", got)
	}
}

func TestSync_SkipsSensitive(t *testing.T) {
	peer, n := countingPeer(t)
	board := &MockBoard{text: []byte("hunter2"), sensitive: true}
	e := newTestEngine(board, peer)

	e.syncOnce()
	if got := waitCount(n, 0); got != 0 {
		t.Fatalf("sensitive content was broadcast %d time(s), want 0", got)
	}
}

func TestSync_RemoteContentNotEchoed(t *testing.T) {
	peer, n := countingPeer(t)
	board := &MockBoard{}
	e := newTestEngine(board, peer)

	e.OnRemoteContent([]byte("from mac"), "text/plain; charset=utf-8")
	board.WriteText([]byte("from mac"))
	e.syncOnce()
	if got := waitCount(n, 0); got != 0 {
		t.Fatalf("remote content was echoed back %d time(s), want 0", got)
	}
}

// watchBoard hands its channel straight to the engine, so a send returns only
// once the engine has taken the event (and finished the previous sync).
type watchBoard struct {
	MockBoard
	changes chan struct{}
}

func (w *watchBoard) Watch(ctx context.Context) <-chan struct{} { return w.changes }

func TestSync_WatcherDrivesSync(t *testing.T) {
	peer, n := countingPeer(t)
	board := &watchBoard{changes: make(chan struct{})}
	e := newTestEngine(board, peer)
	e.interval = time.Millisecond // must not be used with a Watcher

	done := make(chan struct{})
	go func() { e.Start(context.Background()); close(done) }()

	// An event on the empty clipboard: once the send returns the engine has
	// primed and is watching. Give that (no-op) sync a moment to finish.
	board.changes <- struct{}{}
	time.Sleep(20 * time.Millisecond)
	board.setText([]byte("copied"))
	if got := waitCount(n, 0); got != 0 {
		t.Fatalf("broadcast %d time(s) before a change event, want 0", got)
	}
	board.changes <- struct{}{}
	if got := waitCount(n, 1); got != 1 {
		t.Fatalf("broadcast %d time(s) after one change event, want 1", got)
	}

	close(board.changes) // a closed watch channel stops the engine
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("engine did not stop when the watch channel closed")
	}
}
