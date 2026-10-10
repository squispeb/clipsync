//go:build !windows

package main

import "fmt"

func detachDaemon(args []string) error {
	return fmt.Errorf("--detach is only used on Windows; run `clipsync service install` to start clipsync in the background")
}

// The service manager tracks the process on Linux and macOS.
func writePIDFile() error { return nil }
func removePIDFile()      {}
