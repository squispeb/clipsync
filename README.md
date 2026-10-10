# ClipSync

Automatic cross-platform clipboard synchronization over [Tailscale](https://tailscale.com).

Copy on one machine, paste on another — no hotkeys, no cloud, no accounts, **no manual config needed**. Clipboard data travels directly between your devices through Tailscale's encrypted WireGuard tunnel.

Forked from [weishh/cliplink](https://github.com/weishh/cliplink) with multi-peer auto-sync, deduplication, and automatic tailnet peer discovery.

## Features

- **Zero-config setup** — run `clipsync daemon` and it auto-discovers peers on your tailnet
- **Automatic background sync** — daemon watches for clipboard changes and broadcasts to all peers (event-driven on Wayland and macOS, polling on X11 and Windows)
- **Clipboard history** — browse recent clipboard items with `clipsync history`
- **Multi-peer** — sync across 2+ machines simultaneously
- **Deduplication** — SHA-256 content hashing prevents echo loops
- **Text + images** — screenshots and copied images transfer natively
- **Tailscale-native** — auto-binds to Tailscale interface (`100.64.0.0/10`)
- **Optional token auth** — shared secret for extra peace of mind within your tailnet
- **Cross-platform** — Linux (Wayland and X11), macOS, Windows
- **Password-manager aware** — copies a password manager marks as secret (e.g. KeePassXC, 1Password) are never sent
- **Lightweight** — single static binary, ~10MB

## Architecture

```
Machine A                          Machine B
┌─────────────┐    HTTP POST      ┌─────────────┐
│ clipsync    │ ────────────────→ │ clipsync    │
│  daemon     │ ←──────────────── │  daemon     │
└─────────────┘                   └─────────────┘
      ↕                                 ↕
   clipboard                         clipboard
```

Each machine runs a daemon that:
1. Listens for incoming clipboard data from peers
2. Watches the local clipboard for changes (Wayland: `wl-paste --watch`; macOS: the pasteboard change counter; X11/Windows: polls every `sync_interval_ms`)
3. Discovers other clipsync peers on the tailnet automatically
4. Broadcasts changes to all reachable peers

## Installation

### Pre-built binaries (Linux & Windows only)

Grab the latest release for Linux or Windows:

```bash
# Linux (amd64)
curl -LO https://github.com/YOURUSER/clipsync/releases/latest/download/clipsync-linux-amd64
chmod +x clipsync-linux-amd64
sudo mv clipsync-linux-amd64 /usr/local/bin/clipsync

# Windows (PowerShell)
# Download clipsync-windows-amd64.exe from the releases page
```

### One-command install on Windows

If you're on Windows, you can install clipsync with one command after downloading the binary:

```powershell
clipsync install
```

It installs to `%LOCALAPPDATA%\ClipSync\clipsync.exe` and adds that folder to your user PATH.

If you want a proper package manager install, see `docs/winget.md`.

> **macOS users:** The clipboard library (`golang.design/x/clipboard`) requires CGO on macOS, so cross-compiled release binaries won't work. You must build from source on a Mac (see below).

### Build from source

Requires Go 1.23+ and platform-specific clipboard dependencies.

**Linux prerequisites:**

On Wayland, clipsync uses [wl-clipboard](https://github.com/bugaevc/wl-clipboard) at runtime (the X11 clipboard isn't bridged to background clients on compositors such as Hyprland):
```bash
sudo pacman -S wl-clipboard   # Arch
sudo apt install wl-clipboard # Ubuntu/Debian
```

Building still needs the X11 headers (used for the X11 fallback):
```bash
# Arch: libx11 ships the headers

# Ubuntu/Debian
sudo apt install libx11-dev libxext-dev libxmu-dev libgl1-mesa-dev

# Fedora/RHEL
sudo dnf install libX11-devel libXext-devel libXmu-devel mesa-libGL-devel
```

**macOS prerequisites:**
```bash
# Install Xcode Command Line Tools
xcode-select --install

# Install Go (via Homebrew or from https://go.dev/dl/)
brew install go
```

**Build:**
```bash
git clone https://github.com/YOURUSER/clipsync.git
cd clipsync

# Linux / Windows
make build-linux build-windows

# macOS (must be built natively on a Mac; Makefile auto-detects the Xcode SDK)
make build-mac-native
# Binaries in dist/
```

## Quick Start (Zero Config)

### 1. Install Tailscale on all devices

Make sure Tailscale is installed, running, and all devices are on the same tailnet.

### 2. Install clipsync on each device

- **Linux / Windows:** Download pre-built binaries from the [releases page](https://github.com/YOURUSER/clipsync/releases).
- **macOS:** Build from source (see Build from source above). After building, you may need to remove the quarantine attribute:
  ```bash
  sudo xattr -d com.apple.quarantine /usr/local/bin/clipsync
  ```

### 3. Just run the daemon

On **every** device:
```bash
clipsync daemon
```

That's it. On first run, clipsync creates a default config file and automatically scans your tailnet for other clipsync peers. Within seconds, your clipboards are syncing.

### 4. Test it

Copy some text on one machine, paste on another. It should just work.

---

## Manual Configuration (Optional)

If you prefer to explicitly define peers or customize behavior, create a config file:

**Linux:** `~/.config/clipsync/config.json`
**macOS:** `~/Library/Application Support/clipsync/config.json`
**Windows:** `%AppData%\clipsync\config.json`

```json
{
  "device_name": "office-laptop",
  "peers": [
    "100.64.0.2:8275",
    "100.64.0.3:8275"
  ],
  "bind": "",
  "port": 8275,
  "max_size": 10485760,
  "sync_interval_ms": 500,
  "token": "",
  "auto_discover": true,
  "history_max_items": 100,
  "history_max_memory_mb": 50
}
```

| Field | Default | Description |
|---|---|---|
| `device_name` | `""` | Friendly name for log output |
| `peers` | `[]` | Explicit list of `<tailscale-ip>:<port>`. Merged with discovered peers |
| `bind` | auto-detect | Interface to bind to. Empty = auto-detect Tailscale IP |
| `port` | `8275` | Listen port |
| `max_size` | `10485760` | Max clipboard payload in bytes (10 MB) |
| `sync_interval_ms` | `500` | How often to poll for changes (X11 and Windows only; Wayland and macOS are event-driven) |
| `token` | `""` | Shared secret. All peers must use the same token. Strongly recommended: without it any tailnet device can read and replace your clipboard |
| `auto_discover` | `true` | Automatically discover peers via `tailscale status` |
| `history_max_items` | `100` | Max clipboard history entries to keep in memory |
| `history_max_memory_mb` | `50` | Max total memory for history storage (MB) |

**Tailscale IP auto-detection:** When `bind` is empty, the daemon scans interfaces for an IP in the Tailscale CGNAT range (`100.64.0.0/10`), waiting for Tailscale to come up if needed. The daemon is only reachable within your tailnet.

## Commands

```
clipsync <command> [options]

Commands:
  daemon      Start the clipboard receiver and sync daemon
    --port <n>        Override listen port
    --bind <addr>     Override bind address
    --config <path>   Config file path
    --no-sync         Receive-only mode
    --no-discover     Disable auto-discovery

  send        Manually broadcast clipboard to all peers
  status      Check if peer daemons are reachable
  peers       List configured peers
  discover    Scan tailnet for clipsync peers
  history     Show clipboard history
    --limit <n>       Number of items to show (default 20)
  version     Print version
```

## Running as a Service

### Linux (systemd --user)

The unit starts and stops with your graphical session, so the clipboard is available.

```bash
install -Dm755 dist/clipsync-linux-amd64 ~/.local/bin/clipsync
mkdir -p ~/.config/systemd/user
cp systemd/clipsync.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now clipsync
journalctl --user -u clipsync -f
```

### macOS (launchd)

clipsync must run as a per-user LaunchAgent: a system LaunchDaemon (root) can't reach your pasteboard. On the Mac, from the repo:

```bash
./scripts/setup-mac.sh
```

It builds natively, installs `~/.local/bin/clipsync` and `~/Library/LaunchAgents/com.clipsync.daemon.plist`, and logs to `~/Library/Logs/clipsync.log`. Restart with `launchctl kickstart -k gui/$(id -u)/com.clipsync.daemon`.

### Windows

Use Task Scheduler to run `clipsync.exe daemon` at startup, or run `clipsync install` once to add it to your user PATH.

## Clipboard History

ClipSync keeps an in-memory history of recent clipboard items. Both local copies and remote receives are tracked.

```bash
# Show recent clipboard history
clipsync history

# Show more items
clipsync history --limit 50
```

Example output:
```
Clipboard history (5 items):

  1. 14:32:10  a1b2c3d4  [text]  24 bytes  local (office-laptop)
  2. 14:31:45  e5f6g7h8  [image]  15432 bytes  remote (home-desktop)
  3. 14:30:22  i9j0k1l2  [text]  128 bytes  local (office-laptop)
```

The history is stored **only in memory** — nothing is written to disk. You can control the limits via config:

| Setting | Default | Description |
|---|---|---|
| `history_max_items` | 100 | Max number of history entries |
| `history_max_memory_mb` | 50 | Max memory for all stored content (MB) |

When either limit is reached, the oldest entries are evicted automatically.

## How Auto-Discovery Works

1. When the daemon starts, it runs `tailscale status --json` to get all devices on your tailnet
2. It filters out itself and offline devices
3. It probes each online peer's Tailscale IP on the configured port
4. If a peer's `/health` identifies it as clipsync (and accepts your token, when one is set), it's added to the peer list
5. Re-discovery happens every 60 seconds, so new devices joining your tailnet are picked up automatically

If you set `auto_discover: false` or use `--no-discover`, only manually configured peers are used.

## Security

- Clipboard data travels through Tailscale's WireGuard tunnel (encrypted, authenticated)
- Daemon binds to the Tailscale interface by default — not reachable from other networks
- `token` adds a shared-secret header (`X-ClipSync-Token`) required for sending, history and `/health`. Set it: otherwise any device on your tailnet can replace your clipboard, and discovery adds any clipsync daemon it finds
- Copies marked secret by password managers (`x-kde-passwordManagerHint` on Wayland, `org.nspasteboard.ConcealedType` on macOS) are not sent or kept in history
- No data stored on disk — clipboard content exists only in memory during transfer
- No third-party services — direct peer-to-peer within your tailnet

## Limitations

- **WSL2:** Requires a display server (X11/Wayland) for clipboard access. For headless WSL2, run the Windows binary natively instead, or use an X server like VcXsrv.
- **Polling on X11 and Windows:** there the clipboard is polled every `sync_interval_ms`, so very rapid copy-paste cycles may miss an intermediate state.
- **Password-manager detection on Windows/X11:** not implemented; secrets copied there are synced.
- **No file transfer:** Only text and images (PNG) are supported.

## License

MIT
