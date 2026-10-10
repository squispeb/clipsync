package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(board Board) *Server {
	return NewServer(board, "127.0.0.1", 8275, 1024, "", "", nil, "", nil)
}

func TestHandleClipboard_Text(t *testing.T) {
	board := &MockBoard{}
	srv := newTestServer(board)

	body := strings.NewReader("hello world")
	req := httptest.NewRequest(http.MethodPost, "/clip", body)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := board.waitWritten(); string(got) != "hello world" {
		t.Errorf("clipboard text = %q, want %q", string(got), "hello world")
	}
}

func TestHandleClipboard_Image(t *testing.T) {
	board := &MockBoard{}
	srv := newTestServer(board)

	fakeImage := []byte{0x89, 0x50, 0x4E, 0x47}
	req := httptest.NewRequest(http.MethodPost, "/clip", strings.NewReader(string(fakeImage)))
	req.Header.Set("Content-Type", "image/png")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := board.waitWritten(); len(got) != 4 {
		t.Errorf("clipboard image length = %d, want 4", len(got))
	}
}

func TestHandleClipboard_TooLarge(t *testing.T) {
	board := &MockBoard{}
	srv := newTestServer(board)

	big := strings.Repeat("x", 2000)
	req := httptest.NewRequest(http.MethodPost, "/clip", strings.NewReader(big))
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = int64(len(big))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleClipboard_UnsupportedContentType(t *testing.T) {
	board := &MockBoard{}
	srv := newTestServer(board)

	req := httptest.NewRequest(http.MethodPost, "/clip", strings.NewReader("data"))
	req.Header.Set("Content-Type", "application/octet-stream")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnsupportedMediaType)
	}
}

func TestHandleClipboard_WrongMethod(t *testing.T) {
	board := &MockBoard{}
	srv := newTestServer(board)

	req := httptest.NewRequest(http.MethodGet, "/clip", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleClipboard_EmptyBody(t *testing.T) {
	board := &MockBoard{}
	srv := newTestServer(board)

	req := httptest.NewRequest(http.MethodPost, "/clip", strings.NewReader(""))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleClipboard_WriteError(t *testing.T) {
	board := &MockBoard{writeErr: fmt.Errorf("clipboard locked")}
	srv := newTestServer(board)

	req := httptest.NewRequest(http.MethodPost, "/clip", strings.NewReader("hello"))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestHandleClipboard_TokenAuth(t *testing.T) {
	board := &MockBoard{}
	srv := NewServer(board, "127.0.0.1", 8275, 1024, "secret", "", nil, "", nil)

	req := httptest.NewRequest(http.MethodPost, "/clip", strings.NewReader("hello"))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("without token: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/clip", strings.NewReader("hello"))
	req2.Header.Set("Content-Type", "text/plain")
	req2.Header.Set("X-ClipSync-Token", "secret")
	rr2 := httptest.NewRecorder()
	srv.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("with token: status = %d, want %d", rr2.Code, http.StatusOK)
	}
}

func TestHandleClipboard_OnReceive(t *testing.T) {
	board := &MockBoard{}
	var received bool
	srv := NewServer(board, "127.0.0.1", 8275, 1024, "", "", nil, "", func(data []byte, ct string) {
		received = true
	})

	req := httptest.NewRequest(http.MethodPost, "/clip", strings.NewReader("hello"))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !received {
		t.Error("onReceive callback was not called")
	}
}

func TestHandleHealth(t *testing.T) {
	board := &MockBoard{}
	srv := newTestServer(board)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), `"service":"clipsync"`) {
		t.Errorf("health body = %q, want to identify as clipsync", rr.Body.String())
	}
}

func TestHandleHealth_ReportsAuthorization(t *testing.T) {
	srv := NewServer(&MockBoard{}, "127.0.0.1", 8275, 1024, "secret", "", nil, "box", nil)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("without token: status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"authorized":false`) || strings.Contains(body, "box") {
		t.Errorf("without token: body = %s, want authorized false and no device name", body)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-ClipSync-Token", "secret")
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if body := rr.Body.String(); !strings.Contains(body, `"authorized":true`) || !strings.Contains(body, `"device":"box"`) {
		t.Errorf("with token: body = %s, want authorized true and the device name", body)
	}
}

func TestIsClipSyncPeer(t *testing.T) {
	clipsync := httptest.NewServer(NewServer(&MockBoard{}, "127.0.0.1", 0, 1024, "secret", "", nil, "", nil))
	defer clipsync.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer other.Close()

	addr := func(s *httptest.Server) string { return strings.TrimPrefix(s.URL, "http://") }
	client := &http.Client{}

	if !isClipSyncPeer(client, addr(clipsync), "secret") {
		t.Error("clipsync peer with matching token was rejected")
	}
	if isClipSyncPeer(client, addr(clipsync), "wrong") {
		t.Error("clipsync peer with a different token was accepted")
	}
	if isClipSyncPeer(client, addr(other), "secret") {
		t.Error("non-clipsync server answering ok was accepted")
	}
}
