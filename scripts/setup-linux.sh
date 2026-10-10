#!/usr/bin/env bash
# ============================================================
# ClipSync Linux setup
#
# Builds clipsync from this checkout, installs ~/.local/bin/clipsync,
# writes the config and runs it as a systemd user service tied to the
# graphical session.
#
#   ./scripts/setup-linux.sh                       # first machine: makes a token
#   CLIPSYNC_TOKEN=... CLIPSYNC_PEERS=100.x.y.z ./scripts/setup-linux.sh
#
# CLIPSYNC_TOKEN  shared token (every machine needs the same one)
# CLIPSYNC_PEERS  comma-separated peer Tailscale IPs or hostnames, [:port]
#
# Without either variable an existing config is kept; with no config a new
# token is generated and printed.
# ============================================================

set -euo pipefail
cd "$(dirname "$0")/.."

BIN="$HOME/.local/bin/clipsync"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/clipsync/config.json"

die() { echo "error: $*" >&2; exit 1; }
warn() { echo "  warning: $*" >&2; }

echo "[1/5] Checking prerequisites..."
command -v systemctl >/dev/null || die "systemd is required for the service (or run '$BIN daemon' from your session autostart)"
if command -v tailscale >/dev/null; then
    tailscale ip -4 >/dev/null 2>&1 || warn "Tailscale is not up; the daemon will wait for it (run: tailscale up)"
else
    warn "tailscale CLI not found; install Tailscale (https://tailscale.com/download/linux)"
fi

# `command -v go` isn't enough: a version-manager shim may exist without a Go.
if go version >/dev/null 2>&1; then
    GO=(go)
elif command -v mise >/dev/null; then
    GO=(mise exec go@1.24 -- go)
else
    die "Go 1.24+ not found (Arch: pacman -S go, Debian/Ubuntu: https://go.dev/doc/install, Fedora: dnf install golang)"
fi
command -v cc >/dev/null || command -v gcc >/dev/null \
    || die "a C compiler is needed for cgo (Arch: pacman -S base-devel, Debian/Ubuntu: apt install build-essential, Fedora: dnf install gcc)"
[ -f /usr/include/X11/Xlib.h ] \
    || die "X11 headers missing (Arch: pacman -S libx11, Debian/Ubuntu: apt install libx11-dev, Fedora: dnf install libX11-devel)"

# What the service will see: the systemd user manager's environment.
session_env=$(systemctl --user show-environment 2>/dev/null || true)
if grep -q '^WAYLAND_DISPLAY=' <<<"$session_env"; then
    echo "  session: Wayland"
    command -v wl-copy >/dev/null && command -v wl-paste >/dev/null \
        || die "wl-clipboard missing (Arch: pacman -S wl-clipboard, Debian/Ubuntu: apt install wl-clipboard, Fedora: dnf install wl-clipboard)"
elif grep -q '^DISPLAY=' <<<"$session_env"; then
    echo "  session: X11"
else
    warn "the systemd user manager has neither WAYLAND_DISPLAY nor DISPLAY, so the service can't reach the clipboard."
    warn "add this to your compositor/session startup, then re-run:"
    warn "  systemctl --user import-environment WAYLAND_DISPLAY DISPLAY"
fi

echo "[2/5] Building..."
mkdir -p "$(dirname "$BIN")"
"${GO[@]}" build -ldflags "-s -w" -o "$BIN.new" .
mv -f "$BIN.new" "$BIN"
echo "  installed $("$BIN" version) -> $BIN"
case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) warn "$HOME/.local/bin is not on your PATH" ;; esac

echo "[3/5] Configuring..."
if [ -n "${CLIPSYNC_TOKEN:-}${CLIPSYNC_PEERS:-}" ]; then
    args=()
    IFS=',' read -r -a peers <<<"${CLIPSYNC_PEERS:-}"
    for p in "${peers[@]}"; do [ -n "$p" ] && args+=(--peer "$p"); done
    "$BIN" init "${args[@]}"   # reads CLIPSYNC_TOKEN itself
elif [ -f "$CONFIG" ]; then
    echo "  keeping $CONFIG (set CLIPSYNC_TOKEN / CLIPSYNC_PEERS to change it)"
else
    "$BIN" init --new-token
fi

echo "[4/5] Installing systemd user service..."
mkdir -p "$UNIT_DIR"
cp systemd/clipsync.service "$UNIT_DIR/clipsync.service"
systemctl --user daemon-reload
systemctl --user enable clipsync.service >/dev/null
if systemctl --user is-active --quiet graphical-session.target; then
    since=$(date '+%Y-%m-%d %H:%M:%S')
    systemctl --user restart clipsync.service
else
    warn "graphical-session.target is not active, so the service won't start with your session."
    warn "Either start your compositor through a session manager that activates it (uwsm, GNOME, KDE),"
    warn "or run '$BIN daemon' from your compositor's autostart instead (e.g. Hyprland: exec-once = $BIN daemon)."
    exit 1
fi

echo "[5/5] Verifying..."
sleep 2
journalctl --user -u clipsync.service --since "$since" --no-pager -o cat | sed 's/^/  /'
systemctl --user is-active --quiet clipsync.service || die "service is not running; see: journalctl --user -u clipsync -e"
journalctl --user -u clipsync.service --since "$since" --no-pager -o cat | grep -q 'clipboard backend: wayland\|clipboard backend: x11' \
    || warn "no clipboard backend line in the log yet (still waiting for Tailscale?)"
echo
"$BIN" status || warn "some peers are unreachable (fine until clipsync is set up on them)"
