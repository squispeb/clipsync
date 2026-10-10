package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pairServer is a group member at 100.64.0.1:8275 that already knows one peer.
func pairServer(t *testing.T) (*httptest.Server, *PeerBook) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Peers = []string{"100.64.0.9:8275"}
	book := NewPeerBook(cfg, "", "100.64.0.1:8275", nil)
	srv := NewServer(&MockBoard{}, "127.0.0.1", 8275, 1024, "secret", "test", nil, "member", nil)
	srv.EnablePeers(book, "100.64.0.1:8275")
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts, book
}

func openPairing(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	var opened struct{ Code string }
	if err := callJSON(http.MethodPost, ts.URL+"/pair/open", "secret", nil, &opened); err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(opened.Code) != 6 {
		t.Fatalf("code = %q, want 6 digits", opened.Code)
	}
	return opened.Code
}

func TestPair_JoinWithCode(t *testing.T) {
	ts, book := pairServer(t)
	code := openPairing(t, ts)

	var info groupInfo
	if err := callJSON(http.MethodPost, ts.URL+"/pair/join", "", pairJoinRequest{Code: code, Device: "new", Port: 8275}, &info); err != nil {
		t.Fatalf("join: %v", err)
	}
	if info.Token != "secret" || info.Self != "100.64.0.1:8275" || len(info.Peers) != 1 || info.Peers[0] != "100.64.0.9:8275" {
		t.Errorf("group info = %+v", info)
	}
	// The joiner is recorded by its source address (127.0.0.1 here), not a
	// self-reported one.
	if peers := book.Peers(); len(peers) != 2 || peers[1] != "127.0.0.1:8275" {
		t.Errorf("member peers = %v, want the joiner added", peers)
	}
	// A code works once.
	if err := callJSON(http.MethodPost, ts.URL+"/pair/join", "", pairJoinRequest{Code: code, Port: 8275}, nil); err == nil {
		t.Error("second join with the same code succeeded")
	}
}

func TestPair_RequiresOpenWindowAndToken(t *testing.T) {
	ts, _ := pairServer(t)
	if err := callJSON(http.MethodPost, ts.URL+"/pair/join", "", pairJoinRequest{Code: "123456", Port: 8275}, nil); err == nil || !strings.Contains(err.Error(), "no pairing in progress") {
		t.Errorf("join without an open window: err = %v", err)
	}
	if err := callJSON(http.MethodPost, ts.URL+"/pair/open", "wrong", nil, nil); err == nil {
		t.Error("opening a pairing window with the wrong token succeeded")
	}
}

func TestPair_LockoutAfterWrongCodes(t *testing.T) {
	ts, _ := pairServer(t)
	code := openPairing(t, ts)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	var err error
	for i := 0; i < pairMaxAttempts; i++ {
		err = callJSON(http.MethodPost, ts.URL+"/pair/join", "", pairJoinRequest{Code: wrong, Port: 8275}, nil)
	}
	if err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("after %d wrong codes: err = %v, want lockout", pairMaxAttempts, err)
	}
	if err := callJSON(http.MethodPost, ts.URL+"/pair/join", "", pairJoinRequest{Code: code, Port: 8275}, nil); err == nil {
		t.Error("the right code still worked after lockout")
	}
}

func TestPeers_AnnounceAndList(t *testing.T) {
	ts, book := pairServer(t)
	if err := callJSON(http.MethodPost, ts.URL+"/peers/announce", "wrong", announceRequest{Port: 9000}, nil); err == nil {
		t.Error("announce with the wrong token succeeded")
	}
	if err := callJSON(http.MethodPost, ts.URL+"/peers/announce", "secret", announceRequest{Device: "x", Port: 9000}, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}
	var info groupInfo
	if err := callJSON(http.MethodGet, ts.URL+"/peers", "secret", nil, &info); err != nil {
		t.Fatalf("list: %v", err)
	}
	if info.Token != "" {
		t.Error("/peers leaked the token")
	}
	if peers := book.Peers(); len(peers) != 2 || peers[1] != "127.0.0.1:9000" {
		t.Errorf("peers = %v, want the announcer added", peers)
	}
}

func TestPeerBook_IgnoresSelfAndDuplicates(t *testing.T) {
	book := NewPeerBook(DefaultConfig(), "", "100.64.0.1:8275", nil)
	if book.Add("100.64.0.1:8275") {
		t.Error("added itself")
	}
	if !book.Add("100.64.0.2:8275") || book.Add("100.64.0.2:8275") {
		t.Error("duplicate handling wrong")
	}
}
