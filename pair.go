package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Pairing lets a new machine join a group without copying the token by hand.
// On a machine already in the group, `clipsync pair` asks its daemon to open
// a pairing window and shows a 6-digit code. The new machine sends that code
// to /pair/join and gets the token and the peer list back. All of it travels
// over Tailscale, so only devices on the tailnet can try, and a window allows
// pairMaxAttempts wrong codes before it closes.

const (
	pairTTL         = 2 * time.Minute
	pairMaxAttempts = 5
)

var (
	errPairClosed  = errors.New("no pairing in progress (run `clipsync pair` on that machine first)")
	errPairWrong   = errors.New("wrong pairing code")
	errPairLockout = errors.New("too many wrong codes; pairing cancelled (run `clipsync pair` again)")
)

type pairWindow struct {
	mu       sync.Mutex
	code     string
	expires  time.Time
	attempts int
	joined   string
}

func (w *pairWindow) open() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.code = fmt.Sprintf("%06d", n.Int64())
	w.expires = time.Now().Add(pairTTL)
	w.attempts = 0
	w.joined = ""
	return w.code, nil
}

// try checks a code and closes the window on success or lockout.
func (w *pairWindow) try(code string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.code == "" || time.Now().After(w.expires) {
		w.code = ""
		return errPairClosed
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(w.code)) == 1 {
		w.code = ""
		return nil
	}
	w.attempts++
	if w.attempts >= pairMaxAttempts {
		w.code = ""
		return errPairLockout
	}
	return errPairWrong
}

func (w *pairWindow) status() (open bool, joined string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.code != "" && time.Now().Before(w.expires), w.joined
}

func (w *pairWindow) setJoined(who string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.joined = who
}

type pairJoinRequest struct {
	Code   string `json:"code"`
	Device string `json:"device"`
	Port   int    `json:"port"`
}

type groupInfo struct {
	Token  string   `json:"token,omitempty"`
	Device string   `json:"device"`
	Self   string   `json:"self"`
	Peers  []string `json:"peers"`
}

type announceRequest struct {
	Device string `json:"device"`
	Port   int    `json:"port"`
}

// EnablePeers turns on pairing and peer announcements, which need somewhere
// to record new peers.
func (s *Server) EnablePeers(book *PeerBook, self string) {
	s.peers = book
	s.self = self
	s.mux.HandleFunc("/pair/open", s.handlePairOpen)
	s.mux.HandleFunc("/pair/status", s.handlePairStatus)
	s.mux.HandleFunc("/pair/join", s.handlePairJoin)
	s.mux.HandleFunc("/peers", s.handlePeers)
	s.mux.HandleFunc("/peers/announce", s.handleAnnounce)
}

// remotePeerAddr builds a peer address from the request's source IP, so a
// caller can only add itself, never an arbitrary address.
func remotePeerAddr(r *http.Request, port int) (string, error) {
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("invalid port %d", port)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (s *Server) handlePairOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.token == "" || !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	code, err := s.pair.open()
	if err != nil {
		http.Error(w, "failed to generate code", http.StatusInternalServerError)
		return
	}
	log.Printf("pairing window open for %v", pairTTL)
	writeJSON(w, map[string]any{"code": code, "expires_in": int(pairTTL.Seconds())})
}

func (s *Server) handlePairStatus(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	open, joined := s.pair.status()
	writeJSON(w, map[string]any{"open": open, "joined": joined})
}

func (s *Server) handlePairJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req pairJoinRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	addr, err := remotePeerAddr(r, req.Port)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.pair.try(req.Code); err != nil {
		log.Printf("pairing attempt from %s failed: %v", addr, err)
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	peers := s.peers.Peers()
	s.peers.Add(addr)
	s.pair.setJoined(fmt.Sprintf("%s (%s)", req.Device, addr))
	log.Printf("paired with %s (%s)", req.Device, addr)
	writeJSON(w, groupInfo{Token: s.token, Device: s.device, Self: s.self, Peers: peers})
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	if s.token == "" || !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, groupInfo{Device: s.device, Self: s.self, Peers: s.peers.Peers()})
}

func (s *Server) handleAnnounce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.token == "" || !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req announceRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	addr, err := remotePeerAddr(r, req.Port)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.peers.Add(addr) {
		log.Printf("%s (%s) joined the group", req.Device, addr)
	}
	fmt.Fprint(w, "ok")
}

// --- client side ---

var apiClient = &http.Client{
	Timeout:   5 * time.Second,
	Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext},
}

// callJSON sends in (if non-nil) as JSON and decodes the reply into out.
func callJSON(method, url, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-ClipSync-Token", token)
	}
	resp, err := apiClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s", bytes.TrimSpace(msg))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(out)
}

// announceSelf tells every peer this daemon exists, so groups heal when a
// machine joins through one member or its address changes.
func announceSelf(peers []string, token, device string, port int) {
	for _, p := range peers {
		go func(p string) {
			err := callJSON(http.MethodPost, "http://"+p+"/peers/announce", token, announceRequest{Device: device, Port: port}, nil)
			if err != nil {
				log.Printf("announce to %s: %v", p, err)
			}
		}(p)
	}
}

// localDaemon returns the address this machine's daemon listens on.
func localDaemon(cfg Config) (string, error) {
	host := cfg.Bind
	if host == "" {
		ip, ok := TailscaleIP()
		if !ok {
			return "", fmt.Errorf("Tailscale is not connected")
		}
		host = ip
	}
	return net.JoinHostPort(host, strconv.Itoa(cfg.Port)), nil
}

func cmdPair(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	configPath := fs.String("config", "", "config file path")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("this machine has no token yet; run `clipsync setup` first")
	}
	addr, err := localDaemon(cfg)
	if err != nil {
		return err
	}
	var opened struct {
		Code      string `json:"code"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := callJSON(http.MethodPost, "http://"+addr+"/pair/open", cfg.Token, nil, &opened); err != nil {
		return fmt.Errorf("can't reach the local clipsync daemon at %s (is it running? see `clipsync service status`): %w", addr, err)
	}

	fmt.Println()
	fmt.Printf("  Pairing code:  %s %s\n", opened.Code[:3], opened.Code[3:])
	fmt.Println()
	fmt.Printf("On the new machine run `clipsync setup`, choose to join %s (%s)\n", cfg.DeviceName, addr)
	fmt.Printf("and enter the code. It's valid for %d minutes.\n\n", opened.ExpiresIn/60)
	fmt.Print("Waiting...")

	deadline := time.Now().Add(time.Duration(opened.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		var st struct {
			Open   bool   `json:"open"`
			Joined string `json:"joined"`
		}
		if err := callJSON(http.MethodGet, "http://"+addr+"/pair/status", cfg.Token, nil, &st); err != nil {
			continue
		}
		if st.Joined != "" {
			fmt.Printf(" %s joined.\n", st.Joined)
			return nil
		}
		if !st.Open {
			break
		}
	}
	fmt.Println()
	return fmt.Errorf("pairing window closed without a match")
}
