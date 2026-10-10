package main

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// On macOS clipsync runs as a per-user LaunchAgent. It must run inside the
// login session to reach the pasteboard, which a root LaunchDaemon can't.

const launchLabel = "com.clipsync.daemon"

func launchDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist")
}

func darwinLogPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "clipsync.log")
}

func plistFile(exe string) string {
	x := html.EscapeString
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ProcessType</key>
    <string>Interactive</string>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, launchLabel, x(exe), x(darwinLogPath()), x(darwinLogPath()))
}

func servicePlan(exe string) string {
	return fmt.Sprintf("Writes %s (LaunchAgent, runs `%s daemon` at login, logs to %s) and starts it.\n", plistPath(), exe, darwinLogPath())
}

func agentLoaded() bool {
	return exec.Command("launchctl", "print", launchDomain()+"/"+launchLabel).Run() == nil
}

// bootout returns before the job is gone; bootstrapping again too early
// fails with "Bootstrap failed: 5: Input/output error".
func unloadAgent() {
	if !agentLoaded() {
		return
	}
	exec.Command("launchctl", "bootout", launchDomain()+"/"+launchLabel).Run()
	for i := 0; i < 50 && agentLoaded(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
}

func installService(exe string) error {
	if _, err := os.Stat("/Library/LaunchDaemons/" + launchLabel + ".plist"); err == nil {
		fmt.Printf("Warning: an old system-wide /Library/LaunchDaemons/%s.plist exists and can't reach your clipboard. Remove it with:\n", launchLabel)
		fmt.Printf("  sudo launchctl bootout system/%s; sudo rm /Library/LaunchDaemons/%s.plist\n", launchLabel, launchLabel)
	}
	for _, dir := range []string{filepath.Dir(plistPath()), filepath.Dir(darwinLogPath())} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(plistPath(), []byte(plistFile(exe)), 0644); err != nil {
		return err
	}
	unloadAgent()
	return run("launchctl", "bootstrap", launchDomain(), plistPath())
}

func uninstallService() error {
	if _, err := os.Stat(plistPath()); err != nil {
		fmt.Println("clipsync is not installed as a service")
		return nil
	}
	unloadAgent()
	if err := os.Remove(plistPath()); err != nil {
		return err
	}
	fmt.Printf("Stopped clipsync and removed %s\n", plistPath())
	return nil
}

func serviceStatus() (string, bool) {
	if _, err := os.Stat(plistPath()); err != nil {
		return "not installed (run: clipsync service install)", false
	}
	out, _ := exec.Command("launchctl", "print", launchDomain()+"/"+launchLabel).Output()
	running := strings.Contains(string(out), "state = running")
	state := "not running"
	if running {
		state = "running"
	}
	return fmt.Sprintf("LaunchAgent (%s): %s\nlogs: %s", plistPath(), state, darwinLogPath()), running
}
