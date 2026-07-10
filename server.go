package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	board      Board
	maxSize    int64
	bind       string
	port       int
	token      string
	version    string
	history    *History
	device     string
	onReceive  func(data []byte, contentType string)
	mux        *http.ServeMux
	httpSrv    *http.Server
}

func NewServer(board Board, bind string, port int, maxSize int64, token string, version string, history *History, device string, onReceive func(data []byte, contentType string)) *Server {
	s := &Server{
		board:     board,
		maxSize:   maxSize,
		bind:      bind,
		port:      port,
		token:     token,
		version:   version,
		history:   history,
		device:    device,
		onReceive: onReceive,
		mux:       http.NewServeMux(),
	}
	s.mux.HandleFunc("/clip", s.handleClipboard)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/history", s.handleHistory)

	s.httpSrv = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", bind, port),
		Handler:      s,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleClipboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.token != "" {
		if r.Header.Get("X-ClipSync-Token") != s.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	if r.ContentLength > s.maxSize {
		http.Error(w, fmt.Sprintf("payload too large (max %d bytes)", s.maxSize), http.StatusRequestEntityTooLarge)
		return
	}

	limited := io.LimitReader(r.Body, s.maxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	if int64(len(data)) > s.maxSize {
		http.Error(w, fmt.Sprintf("payload too large (max %d bytes)", s.maxSize), http.StatusRequestEntityTooLarge)
		return
	}

	if len(data) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	if peerVersion := r.Header.Get("X-ClipSync-Version"); peerVersion != "" && peerVersion != s.version {
		log.Printf("version mismatch: peer %s is running v%s (local v%s)", r.RemoteAddr, peerVersion, s.version)
	}

	contentType := r.Header.Get("Content-Type")

	var writeFn func([]byte) error
	switch {
	case strings.HasPrefix(contentType, "text/plain"):
		writeFn = s.board.WriteText
	case strings.HasPrefix(contentType, "image/png"):
		writeFn = s.board.WriteImage
	default:
		http.Error(w, fmt.Sprintf("unsupported content type: %s", contentType), http.StatusUnsupportedMediaType)
		return
	}

	if s.onReceive != nil {
		s.onReceive(data, contentType)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	go func() {
		if err := writeFn(data); err != nil {
			log.Printf("clipboard write failed: %v", err)
		} else {
			log.Printf("received %s (%d bytes)", contentType, len(data))
		}
	}()
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.token != "" {
		if r.Header.Get("X-ClipSync-Token") != s.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	if s.history == nil {
		http.Error(w, "history not enabled", http.StatusServiceUnavailable)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	items := s.history.List(limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": items,
		"count": len(items),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","time":"%s"}`, time.Now().Format(time.RFC3339))
}

func (s *Server) ListenAndServe() error {
	log.Printf("clipsync daemon listening on %s:%d", s.bind, s.port)
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}
