//go:build linux

package main

import (
	"log"
	"os"
)

// NewBoard uses wl-clipboard in a Wayland session and falls back to the X11
// clipboard otherwise.
func NewBoard() (Board, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		b, err := NewWaylandBoard()
		if err == nil {
			log.Println("clipboard backend: wayland (wl-clipboard)")
			return b, nil
		}
		log.Printf("WARNING: Wayland session but %v; falling back to X11, which most Wayland compositors do not bridge to background clients", err)
	}
	log.Println("clipboard backend: x11")
	return NewSystemBoard()
}
