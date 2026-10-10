package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func appDataDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "ClipSync")
}

func pidFilePath() string { return filepath.Join(appDataDir(), "clipsync.pid") }

// detachDaemon re-runs `clipsync daemon` without a console window and
// returns, so the login entry doesn't leave a terminal open.
func detachDaemon(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	rest := []string{"daemon"}
	for _, a := range args {
		if a == "--detach" || a == "-detach" || strings.HasPrefix(a, "--detach=") || strings.HasPrefix(a, "-detach=") {
			continue
		}
		rest = append(rest, a)
	}
	cmd := exec.Command(exe, rest...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	return cmd.Process.Release()
}

func writePIDFile() error {
	if err := os.MkdirAll(appDataDir(), 0700); err != nil {
		return err
	}
	return os.WriteFile(pidFilePath(), []byte(strconv.Itoa(os.Getpid())), 0600)
}

func removePIDFile() { os.Remove(pidFilePath()) }

// runningDaemonPID returns the PID from the pid file if that process is
// still a clipsync.exe.
func runningDaemonPID() (int, bool) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil || code != 259 { // STILL_ACTIVE
		return 0, false
	}
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return 0, false
	}
	if !strings.EqualFold(filepath.Base(windows.UTF16ToString(buf[:n])), "clipsync.exe") {
		return 0, false
	}
	return pid, true
}

func stopRunningDaemon() error {
	pid, ok := runningDaemonPID()
	if !ok {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Kill(); err != nil {
		return fmt.Errorf("stop running daemon (pid %d): %w", pid, err)
	}
	removePIDFile()
	return nil
}
