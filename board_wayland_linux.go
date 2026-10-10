//go:build linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// WaylandBoard talks to the Wayland clipboard through wl-clipboard
// (wl-paste/wl-copy). Both use the wlr-data-control protocol, so they work
// from a background daemon without a focused window. golang.design/x/clipboard
// only speaks X11, and compositors such as Hyprland don't bridge the selection
// to an unfocused XWayland client, so SystemBoard can't be used here.
type WaylandBoard struct {
	mu sync.Mutex
}

const (
	waylandPNG          = "image/png"
	waylandSecretHint   = "x-kde-passwordManagerHint"
	waylandReadTimeout  = 3 * time.Second
	waylandWriteTimeout = 5 * time.Second
)

// Text MIME types in order of preference.
var waylandTextTypes = []string{
	"text/plain;charset=utf-8",
	"UTF8_STRING",
	"text/plain",
	"STRING",
	"TEXT",
}

func NewWaylandBoard() (*WaylandBoard, error) {
	for _, bin := range []string{"wl-paste", "wl-copy"} {
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("%s not found (install wl-clipboard)", bin)
		}
	}
	return &WaylandBoard{}, nil
}

// types lists the MIME types offered by the current clipboard owner.
// An empty clipboard is not an error.
func (b *WaylandBoard) types() []string {
	ctx, cancel := context.WithTimeout(context.Background(), waylandReadTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wl-paste", "--list-types").Output()
	if err != nil {
		// wl-paste exits 1 with "Nothing is copied" on an empty clipboard.
		return nil
	}
	return strings.Fields(string(out))
}

func (b *WaylandBoard) read(mime string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), waylandReadTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wl-paste", "--no-newline", "--type", mime).Output()
	if err != nil {
		return nil, fmt.Errorf("wl-paste %s: %w", mime, err)
	}
	return out, nil
}

func (b *WaylandBoard) ReadText() ([]byte, error) {
	offered := b.types()
	for _, want := range waylandTextTypes {
		for _, t := range offered {
			if t == want {
				return b.read(t)
			}
		}
	}
	return nil, fmt.Errorf("clipboard: no text content")
}

func (b *WaylandBoard) ReadImage() ([]byte, error) {
	for _, t := range b.types() {
		if t == waylandPNG {
			return b.read(t)
		}
	}
	return nil, fmt.Errorf("clipboard: no image content")
}

func (b *WaylandBoard) Sensitive() bool {
	for _, t := range b.types() {
		if t == waylandSecretHint {
			return true
		}
	}
	return false
}

func (b *WaylandBoard) write(mime string, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), waylandWriteTimeout)
	defer cancel()
	// wl-copy forks a child that serves the selection until something else is
	// copied. Leave Stdout/Stderr unset (/dev/null): if they were pipes, Run
	// would wait for that long-lived child to close them.
	cmd := exec.CommandContext(ctx, "wl-copy", "--type", mime)
	cmd.Stdin = bytes.NewReader(data)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wl-copy %s: %w", mime, err)
	}
	return nil
}

// WriteText offers the data as text/plain;charset=utf-8. wl-copy also offers
// the usual text aliases (text/plain, UTF8_STRING, STRING, TEXT) for any text
// MIME type.
func (b *WaylandBoard) WriteText(data []byte) error {
	return b.write("text/plain;charset=utf-8", data)
}

func (b *WaylandBoard) WriteImage(data []byte) error {
	return b.write(waylandPNG, data)
}

// Watch runs `wl-paste --watch echo`, which prints a line every time the
// selection changes (including once at startup), and restarts it if it dies.
func (b *WaylandBoard) Watch(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{}, 1)
	go func() {
		defer close(ch)
		for ctx.Err() == nil {
			if err := b.watchOnce(ctx, ch); err != nil && ctx.Err() == nil {
				log.Printf("wl-paste --watch exited: %v; restarting in 2s", err)
			}
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}()
	return ch
}

func (b *WaylandBoard) watchOnce(ctx context.Context, ch chan<- struct{}) error {
	cmd := exec.CommandContext(ctx, "wl-paste", "--watch", "echo")
	// Don't leave the watcher behind if the daemon is killed.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		select {
		case ch <- struct{}{}:
		default: // a change is already pending; the next read sees the latest content
		}
	}
	return cmd.Wait()
}
