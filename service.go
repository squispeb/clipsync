package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Each OS implements:
//
//	servicePlan(exe) string             what install would write, for prompts and --dry-run
//	installService(exe) error           write the login entry and (re)start the daemon now
//	uninstallService() error            stop the daemon and remove the login entry
//	serviceStatus() (string, bool)      human-readable state, and whether it's running

func cmdService(args []string) error {
	usage := func() error {
		fmt.Println("Usage: clipsync service <install|uninstall|status> [--dry-run]")
		fmt.Println()
		fmt.Println("  install    start clipsync now and at every login (re-run after upgrading)")
		fmt.Println("  uninstall  stop clipsync and remove it from login")
		fmt.Println("  status     show whether the background service is running")
		return nil
	}
	if len(args) == 0 {
		return usage()
	}
	fs := flag.NewFlagSet("service "+args[0], flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "only print what would be done")
	fs.Parse(args[1:])

	switch args[0] {
	case "install":
		exe, err := serviceExecutable()
		if err != nil {
			return err
		}
		fmt.Print(servicePlan(exe))
		if *dryRun {
			return nil
		}
		return installService(exe)
	case "uninstall":
		if *dryRun {
			fmt.Println("would stop clipsync and remove its login entry")
			return nil
		}
		return uninstallService()
	case "status":
		state, _ := serviceStatus()
		fmt.Println(state)
		return nil
	default:
		return usage()
	}
}

// serviceExecutable picks the path the login entry should run. Package
// managers install into versioned directories, so prefer a stable path that
// survives upgrades: the PATH entry when it is this same binary (Homebrew's
// bin symlink, winget's Links), or mise's shim.
func serviceExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	path := exe
	if p, err := exec.LookPath("clipsync"); err == nil {
		if abs, err := filepath.Abs(p); err == nil && sameFile(abs, exe) {
			path = abs
		}
	}
	slashed := filepath.ToSlash(path)
	if strings.Contains(slashed, "/mise/installs/") {
		if shim := miseShim(); shim != "" {
			path = shim
		}
	}
	if strings.Contains(slashed, "/go-build") || strings.HasPrefix(slashed, filepath.ToSlash(os.TempDir())) {
		return "", fmt.Errorf("%s is a temporary build; install clipsync first (see README)", exe)
	}
	return path, nil
}

func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

func miseShim() string {
	dir := os.Getenv("MISE_DATA_DIR")
	if dir == "" {
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			dir = filepath.Join(xdg, "mise")
		} else if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".local", "share", "mise")
		}
	}
	shim := filepath.Join(dir, "shims", "clipsync")
	if _, err := os.Stat(shim); err != nil {
		return ""
	}
	return shim
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
