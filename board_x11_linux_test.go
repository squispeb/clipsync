package main

import (
	"os"
	"testing"
)

// Runs only in CI under xvfb-run (CLIPSYNC_X11_TEST=1), against a real X
// server with xclip installed.
func TestXClipRoundTrip(t *testing.T) {
	if os.Getenv("CLIPSYNC_X11_TEST") == "" {
		t.Skip("set CLIPSYNC_X11_TEST=1 and run under an X server")
	}
	b, err := NewXClipBoard()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.WriteText([]byte("héllo x11")); err != nil {
		t.Fatalf("write text: %v", err)
	}
	got, err := b.ReadText()
	if err != nil || string(got) != "héllo x11" {
		t.Fatalf("read text = %q, %v", got, err)
	}
	png := []byte("\x89PNG\r\n\x1a\nfake")
	if err := b.WriteImage(png); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if _, err := b.ReadText(); err == nil {
		t.Error("text still offered after an image was copied")
	}
	img, err := b.ReadImage()
	if err != nil || string(img) != string(png) {
		t.Fatalf("read image = %q, %v", img, err)
	}
}
