package main

import (
	"log"
	"sync"
)

// PeerBook owns the daemon's peer list and the config file it's persisted
// in. Discovery, pairing and peer announcements all add peers through it;
// each change is saved and pushed to the sync engine.
type PeerBook struct {
	mu     sync.Mutex
	cfg    Config
	path   string // empty: don't persist (tests)
	self   string // this daemon's own address, never added as a peer
	engine *SyncEngine
}

func NewPeerBook(cfg Config, path, self string, engine *SyncEngine) *PeerBook {
	return &PeerBook{cfg: cfg, path: path, self: self, engine: engine}
}

// Peers returns a copy of the current peer list.
func (b *PeerBook) Peers() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.cfg.Peers...)
}

// Add merges addrs into the peer list and returns true if anything was new.
func (b *PeerBook) Add(addrs ...string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	var fresh []string
	for _, a := range addrs {
		if a != "" && a != b.self {
			fresh = append(fresh, a)
		}
	}
	merged := MergePeerLists(b.cfg.Peers, fresh)
	if len(merged) == len(b.cfg.Peers) {
		return false
	}
	b.cfg.Peers = merged
	if b.path != "" {
		if err := SaveConfig(b.path, b.cfg); err != nil {
			log.Printf("warning: failed to save config: %v", err)
		}
	}
	if b.engine != nil {
		b.engine.SetPeers(append([]string(nil), merged...))
	}
	log.Printf("peer list now: %v", merged)
	return true
}
