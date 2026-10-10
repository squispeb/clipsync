# ClipSync

Automatic cross-platform clipboard synchronization over [Tailscale](https://tailscale.com).

Copy on one machine, paste on another — no hotkeys, no cloud, no accounts, **one setup command per machine**. Clipboard data travels directly between your devices through Tailscale's encrypted WireGuard tunnel.

Forked from [weishh/cliplink](https://github.com/weishh/cliplink) with multi-peer auto-sync, deduplication, and automatic tailnet peer discovery.

## Features

- **One command per machine** — setup scripts for Linux, macOS and Windows build, configure, install the background service and verify it
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

## Setup

Every machine runs the same daemon. Setting up a group of machines takes three steps:

1. Put every machine on the same tailnet.
2. Run the setup script on the first machine. It creates a shared token.
3. Run the setup script on every other machine with that token and the first machine's address. Then re-run it on the first machine with the others' addresses.

Each setup script checks prerequisites, builds and installs `clipsync`, writes the config, installs the background service for that OS, and verifies it. It's safe to re-run, which is also how you update.

> Using an AI agent to install it? Point it at [AGENTS.md](AGENTS.md).

### Requirements

| | Linux (Wayland) | Linux (X11) | macOS | Windows |
|---|---|---|---|---|
| [Tailscale](https://tailscale.com/download), logged in | ✓ | ✓ | ✓ | ✓ |
| Go 1.24+ ([go.dev/dl](https://go.dev/dl/)) | ✓ | ✓ | ✓ (`brew install go`) | optional: without it the latest release is downloaded |
| C compiler | ✓ | ✓ | Xcode CLT (`xcode-select --install`) | – |
| X11 headers (build only) | ✓ | ✓ | – | – |
| [wl-clipboard](https://github.com/bugaevc/wl-clipboard) (runtime) | ✓ | – | – | – |
| Service manager | systemd user session | systemd user session | launchd (built in) | Startup folder (built in) |

Packages for the Linux rows:

```bash
sudo pacman -S --needed go base-devel libx11 wl-clipboard          # Arch / Omarchy
sudo apt install golang-go build-essential libx11-dev wl-clipboard # Debian / Ubuntu (if its golang-go is older than 1.24, use go.dev/dl)
sudo dnf install golang gcc libX11-devel wl-clipboard              # Fedora
```

### 1. Tailscale on every machine

Install Tailscale, log in with the same account, and note each machine's address:

```bash
tailscale ip -4      # e.g. 100.87.233.53
```

MagicDNS names (e.g. `my-macbook`) also work as peer addresses.

### 2. First machine

Clone the repo and run the script for your OS. With no existing config, it generates a token and prints it.

**Linux**
```bash
git clone https://github.com/squispeb/clipsync.git && cd clipsync
./scripts/setup-linux.sh
```

**macOS**
```bash
git clone https://github.com/squispeb/clipsync.git && cd clipsync
./scripts/setup-mac.sh
```

**Windows** (PowerShell, no admin; no git? download the ZIP from GitHub and run the script from the extracted folder)
```powershell
git clone https://github.com/squispeb/clipsync.git; cd clipsync
powershell -ExecutionPolicy Bypass -File .\scripts\setup-windows.ps1
```

You can print the token again at any time with `clipsync token`.

### 3. Every other machine

Pass the token and the first machine's address:

```bash
# Linux / macOS
CLIPSYNC_TOKEN=<token> CLIPSYNC_PEERS=<first-machine-ip> ./scripts/setup-linux.sh   # or setup-mac.sh
```
```powershell
# Windows
$env:CLIPSYNC_TOKEN = "<token>"; $env:CLIPSYNC_PEERS = "<first-machine-ip>"
powershell -ExecutionPolicy Bypass -File .\scripts\setup-windows.ps1
```

Then go back to the first machine and give it the peer list. The token is kept:

```bash
CLIPSYNC_PEERS=<second-ip>,<third-ip> ./scripts/setup-linux.sh   # or setup-mac.sh / setup-windows.ps1
```

`CLIPSYNC_PEERS` replaces the peer list, so always list every peer that machine should send to.

### 4. Verify

On each machine:

```bash
clipsync status
#   100.87.233.53:8275  ok: my-macbook, v0.5.0
```

Then copy text or a screenshot on one machine and paste it on another. Changes are sent as soon as they happen.

### Where things live

| | Binary | Config | Service | Logs |
|---|---|---|---|---|
| Linux | `~/.local/bin/clipsync` | `~/.config/clipsync/config.json` | `~/.config/systemd/user/clipsync.service` | `journalctl --user -u clipsync` |
| macOS | `~/.local/bin/clipsync` | `~/Library/Application Support/clipsync/config.json` | `~/Library/LaunchAgents/com.clipsync.daemon.plist` | `~/Library/Logs/clipsync.log` |
| Windows | `%LOCALAPPDATA%\ClipSync\clipsync.exe` | `%APPDATA%\clipsync\config.json` | `Startup\ClipSync.lnk` | `%LOCALAPPDATA%\ClipSync\clipsync.log` |

The daemon reads its config only at startup. After editing the config, restart it:

```bash
systemctl --user restart clipsync                              # Linux
launchctl kickstart -k gui/$(id -u)/com.clipsync.daemon       # macOS
```

On Windows, re-run the setup script.

### Updating

```bash
git pull && ./scripts/setup-linux.sh      # or setup-mac.sh / setup-windows.ps1
```

The config is kept. Keep every machine on the same version: `clipsync status` shows each peer's version.

### Uninstalling

```bash
# Linux
systemctl --user disable --now clipsync
rm ~/.config/systemd/user/clipsync.service ~/.local/bin/clipsync
rm -r ~/.config/clipsync

# macOS
launchctl bootout gui/$(id -u)/com.clipsync.daemon
rm ~/Library/LaunchAgents/com.clipsync.daemon.plist ~/.local/bin/clipsync
rm -r ~/Library/Application\ Support/clipsync
```
```powershell
# Windows
Get-Process clipsync -ErrorAction SilentlyContinue | Stop-Process
Remove-Item "$([Environment]::GetFolderPath('Startup'))\ClipSync.lnk"
Remove-Item -Recurse "$env:LOCALAPPDATA\ClipSync", "$env:APPDATA\clipsync"
```

### Without the scripts

```bash
go install github.com/squispeb/clipsync@latest       # Linux needs a C compiler + X11 headers; macOS needs Xcode CLT
clipsync init --new-token --peer <other-machine-ip>  # or CLIPSYNC_TOKEN=... clipsync init --peer ...
clipsync daemon
```

Then run `clipsync daemon` from your OS's service manager. Templates are in `systemd/` and `launchd/`; on macOS replace `__HOME__` in the plist with your home directory.

Pre-built Linux (amd64/arm64) and Windows binaries are attached to [Releases](https://github.com/squispeb/clipsync/releases). They may lag behind `master`; Wayland support and `clipsync init` need v0.5.0 or later. macOS binaries must be built on a Mac (cgo).

### Troubleshooting

| Symptom | Fix |
|---|---|
| `status` says `token mismatch (401)` | The machines have different tokens. Re-run the setup with `CLIPSYNC_TOKEN=$(clipsync token)` taken from a working machine. |
| `status` says `unreachable` | Check that the peer's daemon is running, and that `tailscale ping <ip>` works. On Windows, allow clipsync through the firewall prompt (private networks). |
| Linux: log says `clipboard backend: x11` on Wayland | Install `wl-clipboard`, then restart the service. |
| Linux: service never starts at login | Your session doesn't activate `graphical-session.target`. Either start the compositor through uwsm, GNOME or KDE, or add `exec-once = ~/.local/bin/clipsync daemon` (Hyprland) or the equivalent to your autostart. Make sure `systemctl --user show-environment` lists `WAYLAND_DISPLAY` or `DISPLAY`; if not, add `systemctl --user import-environment WAYLAND_DISPLAY DISPLAY` to your session startup. |
| macOS: nothing syncs, old `/Library/LaunchDaemons/com.clipsync.daemon.plist` | Root daemons can't reach your pasteboard. Remove that plist and re-run `setup-mac.sh`. |
| Log says `waiting for a Tailscale IP` | Tailscale isn't connected yet. The daemon continues on its own once it is. |
| A password didn't sync | Working as intended: copies a password manager marks as secret are skipped. |

## Configuration

`clipsync init` (run by the setup scripts) writes this file; you can also edit it by hand and restart the daemon. It is created readable only by you, since it holds the token.

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
  "token": "<48 hex characters from clipsync init --new-token>",
  "auto_discover": false,
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
| `auto_discover` | `true` | Also discover clipsync peers via `tailscale status` (only ones sharing the token). `clipsync init` turns it off when you list peers |
| `history_max_items` | `100` | Max clipboard history entries to keep in memory |
| `history_max_memory_mb` | `50` | Max total memory for history storage (MB) |

**Tailscale IP auto-detection:** When `bind` is empty, the daemon scans interfaces for an IP in the Tailscale CGNAT range (`100.64.0.0/10`), waiting for Tailscale to come up if needed. The daemon is only reachable within your tailnet.

## Commands

```
clipsync <command> [options]

Commands:
  init        Create or update the config
    --new-token        Generate and print a new shared token
    --token <t>        Use this token (or set CLIPSYNC_TOKEN)
    --peer <host[:port]>  Peer address; repeatable or comma-separated; replaces the list
    --discover         Also auto-discover peers (on by default when no peers are given)
    --device <name>    Name shown in logs (default: hostname)
    --port <n>         Listen port (default 8275)
  token       Print the configured token
  daemon      Start the clipboard receiver and sync daemon
    --port <n>        Override listen port
    --bind <addr>     Override bind address
    --config <path>   Config file path
    --no-sync         Receive-only mode
    --no-discover     Disable auto-discovery
    --log-file <path> Append logs to a file instead of stderr

  send        Manually broadcast clipboard to all peers
  status      Check that peers are reachable, share the token, and their versions
  peers       List configured peers
  discover    Scan tailnet for clipsync peers
  history     Show clipboard history
    --limit <n>       Number of items to show (default 20)
  version     Print version
```

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
