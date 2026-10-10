package main

import (
	"fmt"
	"log"
	"os"
)

// NewBoard uses wl-clipboard in a Wayland session and xclip on X11.
func NewBoard() (Board, error) {
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		b, err := NewWaylandBoard()
		if err != nil {
			return nil, err
		}
		log.Println("clipboard backend: wayland (wl-clipboard)")
		return b, nil
	case os.Getenv("DISPLAY") != "":
		b, err := NewXClipBoard()
		if err != nil {
			return nil, err
		}
		log.Println("clipboard backend: x11 (xclip)")
		return b, nil
	default:
		return nil, fmt.Errorf("no graphical session: neither WAYLAND_DISPLAY nor DISPLAY is set")
	}
}

// boardRequirement describes the clipboard tool this session needs, for
// `clipsync setup`. ok is false when it's missing.
func boardRequirement() (desc string, ok bool, hint string) {
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		_, err := NewWaylandBoard()
		return "Wayland (wl-clipboard)", err == nil, "install wl-clipboard: sudo pacman -S wl-clipboard | sudo apt install wl-clipboard | sudo dnf install wl-clipboard"
	case os.Getenv("DISPLAY") != "":
		_, err := NewXClipBoard()
		return "X11 (xclip)", err == nil, "install xclip: sudo pacman -S xclip | sudo apt install xclip | sudo dnf install xclip"
	default:
		return "no graphical session", false, "run setup from a terminal inside your desktop session"
	}
}
