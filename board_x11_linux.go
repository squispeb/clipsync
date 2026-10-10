package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// XClipBoard talks to the X11 clipboard through xclip. Using the CLI rather
// than a cgo X11 binding keeps Linux builds free of C dependencies. X11 has
// no change notification that xclip exposes, so the sync engine polls.
type XClipBoard struct {
	mu sync.Mutex
}

func NewXClipBoard() (*XClipBoard, error) {
	if _, err := exec.LookPath("xclip"); err != nil {
		return nil, fmt.Errorf("xclip not found (install xclip)")
	}
	return &XClipBoard{}, nil
}

// targets lists the formats offered by the current clipboard owner.
func (b *XClipBoard) targets() []string {
	out, err := b.read("TARGETS")
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

func (b *XClipBoard) read(target string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), waylandReadTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-o", "-t", target).Output()
	if err != nil {
		return nil, fmt.Errorf("xclip %s: %w", target, err)
	}
	return out, nil
}

func (b *XClipBoard) ReadText() ([]byte, error) {
	offered := b.targets()
	for _, want := range waylandTextTypes {
		for _, t := range offered {
			if t == want {
				return b.read(t)
			}
		}
	}
	return nil, fmt.Errorf("clipboard: no text content")
}

func (b *XClipBoard) ReadImage() ([]byte, error) {
	for _, t := range b.targets() {
		if t == waylandPNG {
			return b.read(t)
		}
	}
	return nil, fmt.Errorf("clipboard: no image content")
}

func (b *XClipBoard) Sensitive() bool {
	for _, t := range b.targets() {
		if t == waylandSecretHint {
			return true
		}
	}
	return false
}

func (b *XClipBoard) write(target string, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), waylandWriteTimeout)
	defer cancel()
	// Like wl-copy, xclip forks a child that owns the selection; leave
	// Stdout/Stderr unset so Run doesn't wait on it.
	cmd := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", target, "-i")
	cmd.Stdin = bytes.NewReader(data)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xclip %s: %w", target, err)
	}
	return nil
}

func (b *XClipBoard) WriteText(data []byte) error {
	return b.write("UTF8_STRING", data)
}

func (b *XClipBoard) WriteImage(data []byte) error {
	return b.write(waylandPNG, data)
}
