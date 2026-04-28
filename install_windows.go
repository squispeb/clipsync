//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func installSelf() (string, error) {
	src, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}

	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "ClipSync")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create install dir: %w", err)
	}

	dst := filepath.Join(dir, "clipsync.exe")
	if err := copyFile(src, dst); err != nil {
		return "", err
	}

	if err := ensureUserPath(dir); err != nil {
		return dst, err
	}

	return dst, nil
}

func ensureUserPath(dir string) error {
	path := os.Getenv("PATH")
	for _, part := range strings.Split(path, ";") {
		if strings.EqualFold(strings.TrimSpace(part), dir) {
			return nil
		}
	}

	current := os.Getenv("PATH")
	updated := current + ";" + dir
	cmd := exec.Command("setx", "PATH", updated)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update PATH: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
