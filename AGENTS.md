# AGENTS.md

Instructions for AI agents installing, configuring or working on ClipSync. Human-facing docs are in [README.md](README.md); this file is the exact procedure.

## Ground rules

- **The token is a secret.** Move it between machines in a shell variable or environment variable (`CLIPSYNC_TOKEN`). Never commit it, never paste it into issues, PRs or chat, and don't echo it into logs.
- **Never run without a token.** Without one, any device on the tailnet can read and replace the clipboard.
- **Everything is per-user, with no sudo**, except installing OS packages. Ask the user before installing packages.
- **Back up and restore the user's clipboard** around any functional test. The test overwrites it on every machine.
- Use the setup scripts rather than hand-written steps. They're idempotent, so re-run them to repair or update.

## Install on a set of machines

Machines are referred to as **A** (the first) and **B, C, …** (the others). Repeat the B steps for each other machine.

### 0. Preflight (every machine)

1. Identify the OS: Linux (`uname -s` = `Linux`), macOS (`Darwin`), or Windows.
2. Tailscale must be logged in on every machine with the same account. Record each machine's IPv4: `tailscale ip -4`. On macOS the CLI may be missing; use `ifconfig | grep 'inet 100\.'` instead.
3. Check the prerequisites in the README's *Requirements* table.
   - Linux: `go version` (1.24+), `cc --version`, `/usr/include/X11/Xlib.h`, and on Wayland `wl-copy`/`wl-paste`. If `go` is missing but `mise` exists, the script uses `mise exec go@1.24`.
   - macOS: `go version` (Homebrew installs it at `/opt/homebrew/bin/go`) and `xcode-select -p`.
   - Windows: nothing is required. Without Go, the script downloads the latest release.
4. Get the code: `git clone https://github.com/squispeb/clipsync.git`, or `git pull` in an existing checkout.

### 1. Machine A

Run the script for A's OS from the repo root, without `CLIPSYNC_TOKEN`:

| OS | Command |
|---|---|
| Linux | `./scripts/setup-linux.sh` |
| macOS | `./scripts/setup-mac.sh` |
| Windows | `powershell -ExecutionPolicy Bypass -File .\scripts\setup-windows.ps1` |

If A has no config yet, this generates a token and prints it. If a config already exists, it's kept.

Read the token into a variable without displaying it:

```bash
TOKEN=$(~/.local/bin/clipsync token)               # on A, or: TOKEN=$(ssh A '~/.local/bin/clipsync token')
```

On Windows the binary is `%LOCALAPPDATA%\ClipSync\clipsync.exe`.

### 2. Machines B, C, …

```bash
CLIPSYNC_TOKEN="$TOKEN" CLIPSYNC_PEERS="<A-ip>" ./scripts/setup-linux.sh   # or setup-mac.sh
```
```powershell
$env:CLIPSYNC_TOKEN = "<token>"; $env:CLIPSYNC_PEERS = "<A-ip>"
powershell -ExecutionPolicy Bypass -File .\scripts\setup-windows.ps1
```

`CLIPSYNC_PEERS` is a comma-separated list of every machine *this* machine should send to. With three or more machines, list all the others.

### 3. Back on A: add the peers

```bash
CLIPSYNC_PEERS="<B-ip>,<C-ip>" ./scripts/setup-linux.sh   # token unchanged; peer list replaced
```

### 4. Verify (every machine)

1. Run `clipsync status`. Every peer must show `ok: <name>, v<same version>`.
   - `token mismatch (401)`: re-run step 2 on that machine with the right token.
   - `unreachable`: check the peer's daemon and `tailscale ping <ip>`.
2. The log must show `clipboard backend: wayland` (Linux on Wayland), `listening on 100.x.y.z:8275`, and `sync engine started (N peers, watching for changes)`.
   - Linux: `journalctl --user -u clipsync -n 20`
   - macOS: `tail ~/Library/Logs/clipsync.log`
   - Windows: `Get-Content $env:LOCALAPPDATA\ClipSync\clipsync.log -Tail 20`
3. Functional test, with each clipboard backed up first:
   - Copy a unique marker on A, then read it on B, and the other way round.
   - Linux: `wl-paste -n` / `wl-copy`
   - macOS: `pbpaste` / `pbcopy`
   - Windows: `Get-Clipboard` / `Set-Clipboard`
   - Each side's log shows `clipboard changed` and then `received remote clipboard`.
   - Restore the backups afterwards.
   - If the user starts copying things during the test, stop and leave their clipboard alone.

### 5. Report

Report each machine's OS, address, version and `status` result. Report the backend line from each log. Say whether the functional test passed in each direction. List anything you skipped.

## Doing this over SSH

- **macOS:**
  - Non-login shells lack `/opt/homebrew/bin`. The setup script adds it.
  - Run `pbcopy` as `LANG=en_US.UTF-8 pbcopy`, or non-ASCII text gets mangled before clipsync sees it.
  - `screencapture` doesn't work over SSH (no screen-recording permission). To test images, copy a PNG over and run `osascript -e 'set the clipboard to (read (POSIX file "/tmp/x.png") as «class PNGf»)'`.
- **Linux:** the SSH environment has no `WAYLAND_DISPLAY`. That's fine: the script checks the systemd user manager's environment, which is what the service uses. It needs the user to be logged in to a graphical session.
- **Windows:** processes started from an OpenSSH session don't run on the user's desktop, so a daemon started there can't reach their clipboard. Over SSH, run the script for install and config only. Then have the user sign out and back in (the Startup shortcut starts the daemon on their desktop), or run the script from a terminal on the desktop.

## Platform notes

- **Linux on Wayland** uses `wl-clipboard`. The X11 library can't see the Wayland clipboard from a background process. If the log says `clipboard backend: x11` on a Wayland session, install `wl-clipboard` and re-run the script.
- **Linux service:** the systemd unit is `WantedBy=graphical-session.target`. If the script warns that the target is inactive, the session doesn't use a session manager (uwsm, GNOME, KDE). In that case add `exec-once = ~/.local/bin/clipsync daemon` (Hyprland) or the equivalent to the compositor's autostart, and tell the user.
- **macOS** must be built on the Mac (cgo with AppKit), and must run as a per-user LaunchAgent. Never install it in `/Library/LaunchDaemons`.
- **Windows:** the daemon listens on the Tailscale IP. Windows may ask once whether to allow it on the network; the user must allow private networks.

## Working on the code

- Run tests with `go test -race ./...`. On Linux without a system Go, use `mise exec go@1.24 -- go test -race ./...`. Also vet the other platforms: `GOOS=windows CGO_ENABLED=0 go vet ./...`.
- Clipboard backends implement `Board` (`board.go`):
  - `board_wayland_linux.go` (wl-clipboard)
  - `board_darwin.go` (NSPasteboard via cgo)
  - `board_system.go` (golang.design/x/clipboard, for X11 and Windows)
  - `NewBoard` in `board_select_*.go` picks one.
- Optional interfaces: `Watcher` (event-driven sync instead of polling) and `SensitiveChecker` (skip password-manager copies).
- macOS code can only be compiled on a Mac. If you can reach one, build and test there before committing changes to `board_darwin.go`.
- The version is `version` in `main.go`, also `VERSION` in the `Makefile` and the winget manifests. Bump them together.
- Don't commit binaries: `dist/` and `/clipsync` are ignored.
