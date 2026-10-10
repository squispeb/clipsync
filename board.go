package main

import "context"

// Board abstracts clipboard operations.
// NewBoard picks the implementation for the current platform:
// WaylandBoard (wl-clipboard), DarwinBoard (NSPasteboard) or SystemBoard
// (golang.design/x/clipboard, used for X11 and Windows).
// MockBoard implements this for testing.
type Board interface {
	ReadText() ([]byte, error)
	ReadImage() ([]byte, error)
	WriteText(data []byte) error
	WriteImage(data []byte) error
}

// Watcher is implemented by boards that can signal clipboard changes without
// reading the content. The sync engine reads the clipboard only when the
// channel fires instead of polling every sync_interval_ms. The channel is
// closed when ctx is done.
type Watcher interface {
	Watch(ctx context.Context) <-chan struct{}
}

// SensitiveChecker is implemented by boards that can tell when the current
// clipboard content was marked secret by a password manager
// (x-kde-passwordManagerHint on Wayland, org.nspasteboard.ConcealedType on
// macOS). Sensitive content is never broadcast or kept in history.
type SensitiveChecker interface {
	Sensitive() bool
}
