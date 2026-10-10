package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/registry"
)

// On Windows clipsync starts at login from the per-user Run registry key.
// A Windows service would run outside the desktop session and couldn't reach
// the clipboard. The entry uses `daemon --detach` so no console stays open.

const (
	runKeyPath  = `Software\Microsoft\Windows\CurrentVersion\Run`
	runKeyValue = "ClipSync"
)

func windowsLogPath() string { return filepath.Join(appDataDir(), "clipsync.log") }

func runCommand(exe string) string {
	return fmt.Sprintf(`"%s" daemon --detach --log-file "%s"`, exe, windowsLogPath())
}

func servicePlan(exe string) string {
	return fmt.Sprintf("Adds HKCU\\%s\\%s (runs `%s` at login, logs to %s) and starts it.\n", runKeyPath, runKeyValue, runCommand(exe), windowsLogPath())
}

// Entries left by the old setup-windows.ps1, which would start a second daemon.
func legacyStartupEntries() []string {
	dir := filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`)
	return []string{filepath.Join(dir, "ClipSync.lnk"), filepath.Join(dir, "clipsync-startup.bat")}
}

func installService(exe string) error {
	if err := os.MkdirAll(appDataDir(), 0700); err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(runKeyValue, runCommand(exe)); err != nil {
		return fmt.Errorf("set Run entry: %w", err)
	}
	for _, f := range legacyStartupEntries() {
		os.Remove(f)
	}
	if err := stopRunningDaemon(); err != nil {
		return err
	}
	if err := exec.Command(exe, "daemon", "--detach", "--log-file", windowsLogPath()).Run(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	for i := 0; i < 30; i++ {
		if _, ok := runningDaemonPID(); ok {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon exited right after starting; see %s", windowsLogPath())
}

func uninstallService() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err == nil {
		k.DeleteValue(runKeyValue)
		k.Close()
	}
	for _, f := range legacyStartupEntries() {
		os.Remove(f)
	}
	if err := stopRunningDaemon(); err != nil {
		return err
	}
	fmt.Println("Stopped clipsync and removed it from login")
	return nil
}

func serviceStatus() (string, bool) {
	installed := false
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE); err == nil {
		_, _, err := k.GetStringValue(runKeyValue)
		installed = err == nil
		k.Close()
	}
	pid, running := runningDaemonPID()
	switch {
	case !installed && !running:
		return "not installed (run: clipsync service install)", false
	case running:
		return fmt.Sprintf("running (pid %d), starts at login: %v\nlogs: %s", pid, installed, windowsLogPath()), true
	default:
		return fmt.Sprintf("starts at login, but not running now\nlogs: %s", windowsLogPath()), false
	}
}
