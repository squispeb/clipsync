# cliplink

Cross-platform clipboard sharing over [Tailscale](https://tailscale.com/). Copy on one machine, paste on another — one hotkey away.

```
Mac (Cmd+C)  →  Cmd+Option+V  →  Windows (Ctrl+V)
Windows (Ctrl+C)  →  Ctrl+Alt+V  →  Mac (Cmd+V)
```

Supports text and images (screenshots). No cloud, no account — clipboard data travels directly between your machines through Tailscale's encrypted WireGuard tunnel.

## How It Works

```
Mac                              Windows
┌──────────┐    HTTP POST    ┌──────────┐
│ cliplink │ ──────────────→ │ cliplink │
│  daemon  │ ←────────────── │  daemon  │
└──────────┘                 └──────────┘
     ↕                            ↕
  clipboard                    clipboard
```

Each machine runs a **daemon** (HTTP server) that listens for incoming clipboard data. When you press the hotkey, `cliplink send` reads your clipboard and POSTs it to the other machine's daemon, which writes it to the system clipboard. That's it.

## Prerequisites

- [Tailscale](https://tailscale.com/) installed and connected on both machines
- macOS: [Hammerspoon](https://www.hammerspoon.org/) for hotkey (optional)
- Windows: [AutoHotkey v2](https://www.autohotkey.com/) for hotkey (optional)

## Quick Start

### Download

Grab the latest binaries from [Releases](../../releases) or build from source:

```bash
git clone https://github.com/weishh/cliplink.git
cd cliplink
make build
# Binaries in dist/
```

### Setup

**macOS:**

```bash
chmod +x scripts/setup-mac.sh
./scripts/setup-mac.sh
# Then: Hammerspoon menu bar → Reload Config
```

**Windows (PowerShell):**

```powershell
powershell -ExecutionPolicy Bypass -File setup-windows.ps1
```

The setup scripts handle everything: install binary, create config, start daemon, configure auto-start, and set up hotkey.

### Manual Setup

If you prefer to set things up yourself:

**1. Find your Tailscale IPs:**

```bash
tailscale status
```

**2. Create config file:**

macOS: `~/Library/Application Support/cliplink/config.json`
Windows: `%APPDATA%\cliplink\config.json`

```json
{
  "peer": "<other-machine-tailscale-ip>:8275",
  "port": 8275,
  "max_size": 10485760
}
```

**3. Start the daemon on both machines:**

```bash
cliplink daemon
```

**4. Test:**

```bash
# Copy something to clipboard, then:
cliplink send

# Check if peer is reachable:
cliplink status
```

## Usage

```
cliplink <command> [options]

Commands:
  daemon   Start the clipboard receiver daemon
  send     Send clipboard to peer
  status   Check if peer daemon is reachable
  version  Print version

Daemon options:
  --port <port>     Listen port (overrides config)
  --bind <addr>     Bind address (overrides config)
  --config <path>   Config file path

Send/Status options:
  --peer <addr>     Peer address (overrides config)
  --config <path>   Config file path
```

## Configuration

| Field | Default | Description |
|-------|---------|-------------|
| `peer` | (required for send/status) | Other machine's `<tailscale-ip>:<port>` |
| `port` | `8275` | Local daemon listen port |
| `bind` | auto-detect | Bind address. Empty = auto-detect Tailscale IP |
| `max_size` | `10485760` (10MB) | Maximum clipboard payload in bytes |

**Tailscale IP auto-detection:** When `bind` is empty (default), the daemon scans network interfaces for an IP in the Tailscale CGNAT range (`100.64.0.0/10`) and binds to it. This means the daemon is only reachable within your tailnet — not from other networks.

## Hotkeys

| Platform | Hotkey | Action |
|----------|--------|--------|
| macOS | `Cmd+Option+V` | Send clipboard to peer |
| Windows | `Ctrl+Alt+V` | Send clipboard to peer |

Same physical key position on both platforms.

**macOS (Hammerspoon):** The setup script writes to `~/.hammerspoon/init.lua`. A subtle toast notification appears in the top-right corner on success/failure.

**Windows (AutoHotkey v2):** The setup script creates `%LOCALAPPDATA%\cliplink\cliplink-hotkey.ahk` with a tooltip notification.

## Building

Requires Go 1.21+ and Xcode command line tools (for macOS clipboard CGO).

```bash
make build          # Build for macOS (arm64 + amd64) and Windows (amd64)
make build-mac      # macOS only
make build-windows  # Windows only (cross-compiled, no CGO needed)
make test           # Run all tests
make clean          # Remove build artifacts
```

## Architecture

```
main.go              CLI entry point, subcommand dispatch, signal handling
config.go            Config struct, JSON loading, Tailscale IP auto-detection
board.go             Board interface (clipboard abstraction)
board_system.go      Real clipboard via golang.design/x/clipboard
server.go            HTTP daemon: POST /clip (receive) + GET /health
client.go            HTTP client: read clipboard → POST to peer
```

Design decisions:

- **Board interface** decouples clipboard access from network logic, enabling full test coverage with MockBoard (no real clipboard needed in tests)
- **HTTP over Tailscale** — no custom encryption or auth needed; WireGuard handles both
- **System proxy bypassed** — `Transport{Proxy: nil}` since all traffic is LAN
- **2-second connect timeout** — fast failure when peer is offline; 15-second total timeout allows large image transfer
- **Graceful shutdown** — `signal.NotifyContext` + `http.Server.Shutdown` for clean daemon termination

## Security

- Clipboard data travels through Tailscale's WireGuard tunnel (encrypted, authenticated)
- Daemon binds to Tailscale interface only (not `0.0.0.0`) by default
- No data stored on disk — clipboard content exists only in memory during transfer
- No third-party services — direct peer-to-peer within your tailnet

## License

MIT
