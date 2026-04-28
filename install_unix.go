//go:build !windows

package main

import "fmt"

func installSelf() (string, error) {
	return "", fmt.Errorf("install is only supported on Windows right now")
}
