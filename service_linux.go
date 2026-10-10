package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// On Linux clipsync runs as a systemd user service bound to the graphical
// session, so it starts after login with the display variables set. Sessions
// without graphical-session.target get an XDG autostart entry instead.

func configHome() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func unitPath() string      { return filepath.Join(configHome(), "systemd", "user", "clipsync.service") }
func autostartPath() string { return filepath.Join(configHome(), "autostart", "clipsync.desktop") }

func systemdSession() bool {
	return exec.Command("systemctl", "--user", "is-active", "--quiet", "graphical-session.target").Run() == nil
}

func unitFile(exe string) string {
	return fmt.Sprintf(`[Unit]
Description=ClipSync clipboard sync over Tailscale
Documentation=https://github.com/squispeb/clipsync
# The clipboard belongs to the graphical session: start and stop with it.
PartOf=graphical-session.target
After=graphical-session.target tailscaled.service
Requisite=graphical-session.target

[Service]
ExecStart=%s daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=graphical-session.target
`, systemdQuote(exe))
}

func systemdQuote(s string) string {
	if strings.ContainsAny(s, " \t\"\\") {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
	}
	return s
}

func autostartFile(exe string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=ClipSync
Comment=Clipboard sync over Tailscale
Exec=%s daemon
X-GNOME-Autostart-enabled=true
NoDisplay=true
`, exe)
}

func servicePlan(exe string) string {
	if systemdSession() {
		return fmt.Sprintf("Writes %s (systemd user service, runs `%s daemon` with your desktop session) and starts it.\n", unitPath(), exe)
	}
	return fmt.Sprintf("Writes %s (desktop autostart entry, runs `%s daemon` at login) and starts it now.\n", autostartPath(), exe)
}

// importDisplayEnv makes sure the systemd user manager knows the display, or
// the service can't reach the clipboard. Sessions started by a session
// manager do this themselves; this covers the rest for the current login.
func importDisplayEnv() {
	out, _ := exec.Command("systemctl", "--user", "show-environment").Output()
	env := string(out)
	var missing []string
	for _, v := range []string{"WAYLAND_DISPLAY", "DISPLAY"} {
		if os.Getenv(v) != "" && !strings.Contains(env, "\n"+v+"=") && !strings.HasPrefix(env, v+"=") {
			missing = append(missing, v)
		}
	}
	if len(missing) == 0 {
		return
	}
	if err := run("systemctl", append([]string{"--user", "import-environment"}, missing...)...); err == nil {
		fmt.Printf("Imported %s into the systemd user session.\n", strings.Join(missing, ", "))
	}
}

func installService(exe string) error {
	if !systemdSession() {
		return installAutostart(exe)
	}
	if err := os.MkdirAll(filepath.Dir(unitPath()), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath(), []byte(unitFile(exe)), 0644); err != nil {
		return err
	}
	importDisplayEnv()
	if err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "--user", "enable", "clipsync.service"); err != nil {
		return err
	}
	return run("systemctl", "--user", "restart", "clipsync.service")
}

func installAutostart(exe string) error {
	if err := os.MkdirAll(filepath.Dir(autostartPath()), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(autostartPath(), []byte(autostartFile(exe)), 0644); err != nil {
		return err
	}
	cmd := exec.Command(exe, "daemon")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	cmd.Process.Release()
	fmt.Println("Note: some compositors (Hyprland, sway, ...) ignore desktop autostart entries.")
	fmt.Printf("If yours does, add `%s daemon` to its startup (Hyprland: exec-once = %s daemon).\n", exe, exe)
	return nil
}

func uninstallService() error {
	removed := false
	if _, err := os.Stat(unitPath()); err == nil {
		run("systemctl", "--user", "disable", "--now", "clipsync.service")
		if err := os.Remove(unitPath()); err != nil {
			return err
		}
		run("systemctl", "--user", "daemon-reload")
		fmt.Printf("Stopped clipsync and removed %s\n", unitPath())
		removed = true
	}
	if _, err := os.Stat(autostartPath()); err == nil {
		if err := os.Remove(autostartPath()); err != nil {
			return err
		}
		fmt.Printf("Removed %s (a daemon started from it keeps running until you log out)\n", autostartPath())
		removed = true
	}
	if !removed {
		fmt.Println("clipsync is not installed as a service")
	}
	return nil
}

func serviceStatus() (string, bool) {
	if _, err := os.Stat(unitPath()); err == nil {
		out, _ := exec.Command("systemctl", "--user", "is-active", "clipsync.service").Output()
		state := strings.TrimSpace(string(out))
		return fmt.Sprintf("systemd user service (%s): %s\nlogs: journalctl --user -u clipsync", unitPath(), state), state == "active"
	}
	if _, err := os.Stat(autostartPath()); err == nil {
		return fmt.Sprintf("autostart entry: %s", autostartPath()), true
	}
	return "not installed (run: clipsync service install)", false
}
