# AGENTS.md

Instructions for AI agents installing ClipSync for a user, or working on its code. User-facing docs are in [README.md](README.md).

## Ground rules

- **Use the package manager and `clipsync setup`.** Don't hand-write service files or configs, and don't build from source just to install.
- **Treat the group token as a secret.** Never print it, commit it, or paste it into chat, issues or PRs. Prefer pairing codes. If you must move the token, use the `CLIPSYNC_TOKEN` environment variable.
- **No sudo.** Everything is per-user. The only exception is installing the Linux clipboard tool (`wl-clipboard` or `xclip`) with the system package manager; ask the user first.
- **Ask before installing a package manager** (Homebrew, mise) the user doesn't already have.
- **Back up and restore the user's clipboard** around any functional test, because the test overwrites it on every machine. If the user starts copying things during the test, stop testing and leave their clipboard alone.

## Install on a set of machines

Call the first machine **A** and the others **B, C, …**

### 1. Preflight (each machine)

- **Tailscale.** It must be logged in with the same account on every machine. `tailscale ip -4` prints the address; on macOS without the CLI, use `/Applications/Tailscale.app/Contents/MacOS/Tailscale ip -4`.
- **Linux clipboard tool.** If `WAYLAND_DISPLAY` is set in the user's session, it needs `wl-clipboard`; on X11 it needs `xclip`. `clipsync setup` checks for it.

### 2. Install the binary (each machine)

Use the first option that applies:

| Situation | Command |
|---|---|
| Homebrew installed (macOS/Linux) | `brew install --cask squispeb/tap/clipsync` |
| mise installed (any OS) | `mise use -g github:squispeb/clipsync` |
| Windows with winget, once the package is in the catalog | `winget install Squispeb.ClipSync` |
| Otherwise | Download the archive for the OS and arch from the latest release, check it against `checksums.txt`, and put `clipsync` on `PATH` |

Check it with `clipsync version`. Every machine should report the same version.

### 3. Machine A

```bash
clipsync setup --new-group --yes
```

If A already has a group (`clipsync status` lists peers), keep it: run `clipsync setup --yes`, which keeps the existing group.

### 4. Machines B, C, … (one at a time)

1. On A, run `clipsync pair` in the background and read the 6-digit code from its output. It's valid for 2 minutes, and `pair` exits once someone joins.
2. On the new machine:
   ```bash
   clipsync setup --join <A's Tailscale IP or MagicDNS name> --code <code> --yes
   ```
3. The new machine announces itself to every member, so A and the others learn about it automatically.

If you can't run `pair` on A but can read A's token (for example over SSH), you can use the token instead:

```bash
CLIPSYNC_TOKEN="$(ssh A clipsync token)" clipsync setup --join A --yes
```

### 5. Verify (each machine)

1. `clipsync status` must list every other machine as `ok: <name>, v<same version>`.
2. `clipsync service status` must say running.
   - Linux: also check that `journalctl --user -u clipsync -n 20` shows `clipboard backend: wayland` or `x11`.
   - macOS: check the same in `~/Library/Logs/clipsync.log`.
   - Windows: check the same in `%LOCALAPPDATA%\ClipSync\clipsync.log`.
3. Functional test, with each clipboard backed up first:
   - Copy a unique marker on A and read it on B, then the other way round.
   - Linux: `wl-paste -n` / `wl-copy` (`xclip` on X11)
   - macOS: `pbpaste` / `pbcopy`
   - Windows: `Get-Clipboard` / `Set-Clipboard`
   - Restore the backups afterwards.

### 6. Report

Report each machine's OS, install method, version and `status` output. Say whether the functional test passed in each direction. List anything you skipped and why.

## Doing it over SSH

- **macOS:**
  - Non-login shells don't have `/opt/homebrew/bin` on `PATH`. Call `/opt/homebrew/bin/brew` and `/opt/homebrew/bin/clipsync` directly.
  - Run `pbcopy` as `LANG=en_US.UTF-8 pbcopy`, or non-ASCII text gets mangled.
  - `screencapture` doesn't work over SSH. To test images, set a PNG with `osascript -e 'set the clipboard to (read (POSIX file "/tmp/x.png") as «class PNGf»)'`.
- **Linux:**
  - The SSH environment has no `WAYLAND_DISPLAY`, so `clipsync setup`'s clipboard check fails there. Run setup in the user's desktop session instead, or pass the session's variables, e.g. `WAYLAND_DISPLAY=wayland-1 XDG_RUNTIME_DIR=/run/user/$(id -u) clipsync setup ...`.
  - The service itself takes its environment from the systemd user session.
- **Windows:** a daemon started from an OpenSSH session runs outside the user's desktop and can't reach their clipboard. Over SSH, `clipsync setup` can install and configure, but the daemon only works after the user signs out and back in, when the Run entry starts it on their desktop. Alternatively, run setup from a terminal on the desktop.

## Working on the code

- Test with `go test -race ./...`. CI (`.github/workflows/ci.yml`) also runs the xclip backend under Xvfb, a macOS cross-build, and a Windows service smoke test.
- The clipboard backends implement `Board` (`board.go`):
  - `board_wayland_linux.go` (wl-clipboard)
  - `board_x11_linux.go` (xclip)
  - `board_darwin.go` (NSPasteboard via cgo; macOS is the only cgo build)
  - `board_system_windows.go`
  - `NewBoard` in `board_select_*.go` picks one.
- The service code is in `service_{linux,darwin,windows}.go`. Pairing, `/peers` and announcements are in `pair.go`, and the peer list is persisted through `peerbook.go`.
- macOS code only compiles on a Mac. Run `make snapshot` on a Mac (with goreleaser) before changing the release config.
- Releases come from a `v*` tag; see [docs/releasing.md](docs/releasing.md). Don't commit binaries.
