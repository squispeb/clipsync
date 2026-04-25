package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSend_Text(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()

	board := &MockBoard{text: []byte("hello clipboard")}
	client := NewClient(mock.Listener.Addr().String(), board, 10*1024*1024, "")

	err := client.Send()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(gotBody) != "hello clipboard" {
		t.Errorf("sent body = %q, want %q", string(gotBody), "hello clipboard")
	}
	if gotContentType != "text/plain; charset=utf-8" {
		t.Errorf("content-type = %q, want %q", gotContentType, "text/plain; charset=utf-8")
	}
}

func TestSend_Image(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()

	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}
	board := &MockBoard{image: pngData}
	client := NewClient(mock.Listener.Addr().String(), board, 10*1024*1024, "")

	err := client.Send()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotBody) != len(pngData) {
		t.Errorf("sent body length = %d, want %d", len(gotBody), len(pngData))
	}
	if gotContentType != "image/png" {
		t.Errorf("content-type = %q, want %q", gotContentType, "image/png")
	}
}

func TestSend_TextPriorityOverImage(t *testing.T) {
	var gotContentType string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()

	board := &MockBoard{
		text:  []byte("some text"),
		image: []byte{0x89, 0x50, 0x4E, 0x47},
	}
	client := NewClient(mock.Listener.Addr().String(), board, 10*1024*1024, "")

	err := client.Send()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "text/plain; charset=utf-8" {
		t.Errorf("content-type = %q, want text/plain (text should take priority)", gotContentType)
	}
}

func TestSend_EmptyClipboard(t *testing.T) {
	board := &MockBoard{}
	client := NewClient("127.0.0.1:9999", board, 10*1024*1024, "")

	err := client.Send()
	if err == nil {
		t.Fatal("expected error for empty clipboard, got nil")
	}
}

func TestSend_PeerRejects(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
	}))
	defer mock.Close()

	board := &MockBoard{text: []byte("data")}
	client := NewClient(mock.Listener.Addr().String(), board, 10*1024*1024, "")

	err := client.Send()
	if err == nil {
		t.Fatal("expected error when peer rejects, got nil")
	}
}

func TestSend_PeerUnreachable(t *testing.T) {
	board := &MockBoard{text: []byte("data")}
	client := NewClient("127.0.0.1:1", board, 10*1024*1024, "")

	err := client.Send()
	if err == nil {
		t.Fatal("expected error for unreachable peer, got nil")
	}
}

func TestSend_TooLarge(t *testing.T) {
	board := &MockBoard{text: []byte("x")}
	client := NewClient("127.0.0.1:9999", board, 0, "")

	err := client.Send()
	if err == nil {
		t.Fatal("expected error for oversized data, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %q, want to contain 'too large'", err)
	}
}

func TestSend_ClipboardReadError(t *testing.T) {
	board := &MockBoard{readErr: fmt.Errorf("permission denied")}
	client := NewClient("127.0.0.1:9999", board, 10*1024*1024, "")

	err := client.Send()
	if err == nil {
		t.Fatal("expected error for clipboard read failure, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want to contain 'permission denied'", err)
	}
}

func TestSend_WithToken(t *testing.T) {
	var gotToken string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-ClipSync-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()

	board := &MockBoard{text: []byte("secret text")}
	client := NewClient(mock.Listener.Addr().String(), board, 10*1024*1024, "mytoken")

	err := client.Send()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotToken != "mytoken" {
		t.Errorf("token = %q, want %q", gotToken, "mytoken")
	}
}
