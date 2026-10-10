# ClipSync

Copy on one machine, paste on another. ClipSync syncs your clipboard (text and images) between your computers directly over [Tailscale](https://tailscale.com): no cloud service, no account, no hotkeys.

- **Works across macOS, Linux (Wayland and X11) and Windows**
- **Instant:** changes are sent as they happen (event-driven on macOS and Wayland)
- **Private:** data goes only to machines you paired, through Tailscale's encrypted tunnel, and is never written to disk
- **Password-manager aware:** copies a password manager marks as secret (e.g. KeePassXC, 1Password) are never sent
- **One binary, no runtime dependencies** besides Tailscale (and `wl-clipboard` or `xclip` on Linux)

Forked from [weishh/cliplink](https://github.com/weishh/cliplink).

## Quick start

1. Install [Tailscale](https://tailscale.com/download) on each machine and log in with the same account.
2. Install clipsync on each machine (see [Install](#install)).
3. On the first machine, run `clipsync setup` and choose **start a new group**.
4. On each other machine, run `clipsync setup`. Pick the first machine from the list, run `clipsync pair` on that machine, and type the 6-digit code it shows.

That's it. Copy something on one machine and paste it on another.

## Install

### macOS and Linux: Homebrew

```bash
brew install --cask squispeb/tap/clipsync
```

### macOS, Linux and Windows: mise

```bash
mise use -g github:squispeb/clipsync
```

mise verifies the release's build provenance automatically.

### Windows: winget

```powershell
winget install Squispeb.ClipSync
```

Until the package is accepted into the winget catalog, use the Windows zip from [Releases](https://github.com/squispeb/clipsync/releases) or mise.

### Download

Every [release](https://github.com/squispeb/clipsync/releases) has archives for macOS, Linux and Windows (amd64 and arm64) and a `checksums.txt`. Each archive holds the single `clipsync` binary; put it anywhere on your `PATH`.

The builds come from GitHub Actions and are attested, so you can check that a file was built from this repository:

```bash
gh attestation verify clipsync_0.5.0_linux_amd64.tar.gz --repo squispeb/clipsync
```

The macOS binaries are not notarized by Apple. Homebrew and mise install them without any Gatekeeper prompt. If you download one manually in a browser, clear the quarantine flag: `xattr -d com.apple.quarantine clipsync`.

### From source

```bash
go install github.com/squispeb/clipsync@latest   # Go 1.24+; macOS also needs Xcode command line tools
```

### Linux clipboard tool

On Linux, clipsync uses the standard clipboard tool for your session. `clipsync setup` tells you if it's missing.

| Session | Package |
|---|---|
| Wayland | `wl-clipboard` (Arch: `pacman -S wl-clipboard`, Debian/Ubuntu: `apt install wl-clipboard`, Fedora: `dnf install wl-clipboard`) |
| X11 | `xclip` (same package managers, package `xclip`) |

## Set up

```bash
clipsync setup
```

`setup` explains each step and asks before changing anything:

1. **Checks.** Tailscale is connected and the clipboard is usable.
2. **Group.** Start a new group (first machine), or join one. To join, it lists the machines on your tailnet that run clipsync; pick one, run `clipsync pair` on it, and type the code it shows. The code is valid for 2 minutes and for 5 attempts. Pairing gives this machine the group's shared token, and every machine in the group learns about the new one.
3. **Background service.** It shows the exact file it will write, then starts clipsync now and at every login:

   | OS | What gets written | Logs |
   |---|---|---|
   | Linux | `~/.config/systemd/user/clipsync.service` (systemd user service tied to your desktop session; `~/.config/autostart/clipsync.desktop` on sessions without one) | `journalctl --user -u clipsync` |
   | macOS | `~/Library/LaunchAgents/com.clipsync.daemon.plist` (per-user LaunchAgent) | `~/Library/Logs/clipsync.log` |
   | Windows | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\ClipSync` (per-user login entry, no console window) | `%LOCALAPPDATA%\ClipSync\clipsync.log` |

4. **Verify.** It checks that the daemon runs and reaches every machine in the group.

Nothing needs admin rights. The only other file is the config: `~/.config/clipsync/config.json` on Linux, `~/Library/Application Support/clipsync/config.json` on macOS, and `%APPDATA%\clipsync\config.json` on Windows. It's readable only by you, because it holds the token.

### Adding more machines

On any machine already in the group, run `clipsync pair`. On the new machine, run `clipsync setup` and enter the code.

### Scripts and agents

`setup` also runs without prompts:

```bash
clipsync setup --new-group --yes                         # first machine
clipsync setup --join <machine> --code <code> --yes      # others; code from `clipsync pair` on <machine>
CLIPSYNC_TOKEN=... clipsync setup --join <machine> --yes # if you already have the token
```

The exact procedure for AI agents is in [AGENTS.md](AGENTS.md).

### Upgrading

Upgrade with the package manager you installed with: `brew upgrade --cask clipsync`, `mise upgrade`, or `winget upgrade Squispeb.ClipSync`. Then restart the daemon on the new version:

```bash
clipsync service install
```

Keep every machine on the same version. `clipsync status` shows each peer's version.

### Uninstalling

```bash
clipsync service uninstall      # stop it and remove it from login
```

Then remove the config folder listed above, and uninstall with your package manager. `brew uninstall --zap --cask clipsync` also removes the macOS LaunchAgent, logs and config.

## Commands

```
clipsync setup               Set up this machine (interactive; see --help for flags)
clipsync pair                Show a code that lets another machine join this group
clipsync status              Check every peer: reachable, same token, version
clipsync service install     Start clipsync now and at login (re-run after upgrading)
clipsync service uninstall   Stop it and remove it from login
clipsync service status      Is the background service running?
clipsync history [--limit n] Recent clipboard items (metadata only)
clipsync send                Send the current clipboard to all peers now
clipsync daemon              Run in the foreground (what the service runs)
clipsync init                Write the config non-interactively (--new-token, --token, --peer, --device, --port, --bind)
clipsync token               Print the group token
clipsync version
```

## Configuration

`setup` writes the config; you rarely need to edit it. If you do, restart afterwards with `clipsync service install`.

| Field | Default | Description |
|---|---|---|
| `device_name` | hostname | Name shown to other machines and in logs |
| `peers` | `[]` | Other machines, as `<tailscale-ip or name>:<port>`. Pairing and announcements keep this up to date |
| `token` | – | Shared secret for the group. Every machine needs the same one |
| `bind` | Tailscale IP | Listen address. Empty means this machine's Tailscale IP; clipsync waits for Tailscale if it isn't up yet |
| `port` | `8275` | Listen port |
| `max_size` | `10485760` | Largest clipboard item synced, in bytes (10 MB) |
| `sync_interval_ms` | `500` | Polling interval on X11 and Windows. macOS and Wayland are event-driven |
| `auto_discover` | `false` | Also add any clipsync on the tailnet that has the same token |
| `history_max_items` | `100` | Clipboard history entries kept in memory |
| `history_max_memory_mb` | `50` | Memory limit for clipboard history |

## How it works

Each machine runs the same daemon. It listens on its Tailscale IP and watches the local clipboard. When something new is copied, it POSTs the content to every peer. When content arrives, it writes it to the local clipboard. Content hashes stop copies from bouncing back and forth, and a daemon restart doesn't push the current clipboard over the others.

| Platform | Clipboard access |
|---|---|
| macOS | NSPasteboard; watches its change counter |
| Linux, Wayland | `wl-paste --watch` / `wl-copy` |
| Linux, X11 | `xclip`, polled |
| Windows | Win32 clipboard, polled |

## Security

- Traffic travels only inside your tailnet, through Tailscale's WireGuard tunnel. The daemon listens only on the Tailscale interface.
- Every request needs the group token. Without it, a tailnet device can see that clipsync and its version are running, but can't read, send or pair.
- Pairing codes are 6 random digits, valid for 2 minutes and 5 attempts, and are shown only to someone who already has the token. A machine can only add its own address to a group, never someone else's.
- Copies a password manager marks as secret (`x-kde-passwordManagerHint` on Linux, `org.nspasteboard.ConcealedType` on macOS) are not sent or kept in history. Windows has no equivalent marker yet.
- Clipboard history lives only in memory, and `clipsync history` shows metadata only.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `status` says `token mismatch` | That machine is in a different group. Run `clipsync setup` on it and join again. |
| `status` says `unreachable` | Is clipsync running there (`clipsync service status`)? Does `tailscale ping <machine>` work? On Windows, allow clipsync through the firewall prompt (private networks). |
| Linux: `setup` says the clipboard isn't usable | Install `wl-clipboard` (Wayland) or `xclip` (X11). |
| Linux: clipsync doesn't start after login | Your session doesn't activate `graphical-session.target`, so `setup` wrote a desktop autostart entry instead. Compositors like Hyprland or sway ignore those; add `exec-once = clipsync daemon` (Hyprland) or the equivalent to your config. |
| macOS: nothing syncs after upgrading from an old version | An old `/Library/LaunchDaemons/com.clipsync.daemon.plist` (root) can't reach your clipboard. Remove it: `sudo launchctl bootout system/com.clipsync.daemon; sudo rm /Library/LaunchDaemons/com.clipsync.daemon.plist`. |
| Pairing says `no pairing in progress` | Run `clipsync pair` on the other machine first. The code expires after 2 minutes. |
| A password didn't sync | Intended: copies marked secret by a password manager are skipped. |

## Limitations

- Text and PNG images only, no files.
- Windows support is tested in CI but not yet widely on real machines. Reports are welcome.

## Development

```bash
make test       # go test -race ./...
make build      # dist/clipsync for this machine
make snapshot   # full GoReleaser dry run (macOS only, needs goreleaser)
```

Releases: push a `v*` tag. See [docs/releasing.md](docs/releasing.md).

## License

MIT
