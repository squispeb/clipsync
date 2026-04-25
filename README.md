# ClipSync

Automatic cross-platform clipboard synchronization over [Tailscale](https://tailscale.com).

Copy on one machine, paste on another — no hotkeys, no cloud, no accounts. Clipboard data travels directly between your devices through Tailscale's encrypted WireGuard tunnel.

Forked from [weishh/cliplink](https://github.com/weishh/cliplink) with multi-peer auto-sync, deduplication, and simple token authentication.

## Features

- **Automatic background sync** — daemon polls clipboard changes and broadcasts to all peers
- **Multi-peer** — sync across 2+ machines simultaneously
- **Deduplication** — SHA-256 content hashing prevents echo loops
- **Text + images** — screenshots and copied images transfer natively
- **Tailscale-native** — auto-binds to Tailscale interface (`100.64.0.0/10`)
- **Optional token auth** — shared secret for extra peace of mind within your tailnet
- **Cross-platform** — Linux, macOS, Windows
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
2. Polls the local clipboard every `sync_interval_ms`
3. Broadcasts changes to all configured peers

## Installation

### Pre-built binaries

Grab the latest release for your platform, or build from source:

```bash
# Linux (amd64)
curl -LO https://github.com/YOURUSER/clipsync/releases/latest/download/clipsync-linux-amd64
chmod +x clipsync-linux-amd64
sudo mv clipsync-linux-amd64 /usr/local/bin/clipsync

# macOS (Apple Silicon)
curl -LO https://github.com/YOURUSER/clipsync/releases/latest/download/clipsync-mac-arm64
chmod +x clipsync-mac-arm64
sudo mv clipsync-mac-arm64 /usr/local/bin/clipsync

# Windows (PowerShell)
# Download clipsync-windows-amd64.exe and place it in your PATH
```

### Build from source

Requires Go 1.23+ and platform-specific clipboard dependencies.

**Linux prerequisites:**
```bash
# Ubuntu/Debian
sudo apt install libx11-dev libxext-dev libxmu-dev libgl1-mesa-dev

# Fedora/RHEL
sudo dnf install libX11-devel libXext-devel libXmu-devel mesa-libGL-devel
```

**Build:**
```bash
git clone https://github.com/YOURUSER/clipsync.git
cd clipsync
make build
# Binaries in dist/
```

## Quick Start

### 1. Find your Tailscale IPs

On each machine:
```bash
tailscale status
```

### 2. Create config file

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
  "token": ""
}
```

| Field | Default | Description |
|---|---|---|
| `device_name` | `""` | Friendly name for log output |
| `peers` | `[]` | List of `<tailscale-ip>:<port>` for all other machines |
| `bind` | auto-detect | Interface to bind to. Empty = auto-detect Tailscale IP |
| `port` | `8275` | Listen port |
| `max_size` | `10485760` | Max clipboard payload in bytes (10 MB) |
| `sync_interval_ms` | `500` | How often to poll clipboard for changes |
| `token` | `""` | Optional shared secret. All peers must use the same token |

**Tailscale IP auto-detection:** When `bind` is empty, the daemon scans interfaces for an IP in the Tailscale CGNAT range (`100.64.0.0/10`). The daemon is only reachable within your tailnet.

### 3. Start the daemon on all machines

```bash
clipsync daemon
```

Or run in receive-only mode (no broadcasting):
```bash
clipsync daemon --no-sync
```

### 4. Verify connectivity

```bash
clipsync status
```

### 5. Test manual broadcast

```bash
clipsync send
```

## Running as a Service

### Linux (systemd --user)

```bash
mkdir -p ~/.config/systemd/user
cp systemd/clipsync.service ~/.config/systemd/user/
# Edit ExecStart path if needed
systemctl --user daemon-reload
systemctl --user enable --now clipsync
systemctl --user status clipsync
```

### macOS (launchd)

```bash
sudo cp launchd/com.clipsync.daemon.plist /Library/LaunchDaemons/
sudo launchctl load /Library/LaunchDaemons/com.clipsync.daemon.plist
sudo launchctl start com.clipsync.daemon
```

### Windows

Use Task Scheduler to run `clipsync.exe daemon` at startup, or run it in a terminal session.

## Commands

```
clipsync <command> [options]

Commands:
  daemon      Start the clipboard receiver and sync daemon
    --port <n>      Override listen port
    --bind <addr>   Override bind address
    --config <path> Config file path
    --no-sync       Receive-only mode

  send        Manually broadcast clipboard to all peers
  status      Check if peer daemons are reachable
  peers       List configured peers
  version     Print version
```

## Security

- Clipboard data travels through Tailscale's WireGuard tunnel (encrypted, authenticated)
- Daemon binds to the Tailscale interface by default — not reachable from other networks
- Optional `token` field adds a shared-secret header (`X-ClipSync-Token`) for extra protection within your tailnet
- No data stored on disk — clipboard content exists only in memory during transfer
- No third-party services — direct peer-to-peer within your tailnet

## Limitations

- **WSL2:** Requires a display server (X11/Wayland) for clipboard access. For headless WSL2, run the Windows binary natively instead, or use an X server like VcXsrv.
- **Polling-based:** We poll the clipboard every `sync_interval_ms`. Very rapid copy-paste cycles may occasionally miss an intermediate state.
- **No file transfer:** Only text and images (PNG) are supported.

## License

MIT
