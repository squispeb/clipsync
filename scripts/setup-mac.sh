#!/bin/bash
# ============================================================
# ClipSync macOS setup
#
# Builds clipsync natively (it uses cgo for NSPasteboard), installs
# ~/.local/bin/clipsync, writes the config and runs it as a per-user
# LaunchAgent (a root LaunchDaemon can't reach your pasteboard).
#
#   ./scripts/setup-mac.sh                         # first machine: makes a token
#   CLIPSYNC_TOKEN=... CLIPSYNC_PEERS=100.x.y.z ./scripts/setup-mac.sh
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
LABEL="com.clipsync.daemon"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG="$HOME/Library/Logs/clipsync.log"
CONFIG="$HOME/Library/Application Support/clipsync/config.json"
DOMAIN="gui/$(id -u)"

die() { echo "error: $*" >&2; exit 1; }
warn() { echo "  warning: $*" >&2; }

echo "[1/5] Checking prerequisites..."
# Homebrew and the official Go installer aren't on PATH in non-login shells (ssh).
export PATH="$PATH:/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin"
go version >/dev/null 2>&1 || die "Go 1.24+ not found (brew install go, or https://go.dev/dl/)"
xcode-select -p >/dev/null 2>&1 || die "Xcode command line tools missing (xcode-select --install)"
if ! ifconfig 2>/dev/null | grep -Eq 'inet 100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.'; then
    warn "no Tailscale IP found; the daemon will wait for Tailscale to connect"
fi

echo "[2/5] Building..."
make build-mac-native
mkdir -p "$(dirname "$BIN")"
install -m 0755 "dist/clipsync-mac-$(uname -m)" "$BIN"
echo "  installed $("$BIN" version) -> $BIN"

echo "[3/5] Configuring..."
if [ -n "${CLIPSYNC_TOKEN:-}${CLIPSYNC_PEERS:-}" ]; then
    set --
    old_ifs=$IFS; IFS=','
    for p in ${CLIPSYNC_PEERS:-}; do
        p=$(echo "$p" | tr -d ' ')
        [ -n "$p" ] && set -- "$@" --peer "$p"
    done
    IFS=$old_ifs
    "$BIN" init "$@"   # reads CLIPSYNC_TOKEN itself
elif [ -f "$CONFIG" ]; then
    echo "  keeping $CONFIG (set CLIPSYNC_TOKEN / CLIPSYNC_PEERS to change it)"
else
    "$BIN" init --new-token
fi

echo "[4/5] Installing LaunchAgent..."
if [ -f "/Library/LaunchDaemons/$LABEL.plist" ]; then
    warn "an old system-wide /Library/LaunchDaemons/$LABEL.plist exists and can't reach your clipboard; remove it with:"
    warn "  sudo launchctl bootout system/$LABEL; sudo rm /Library/LaunchDaemons/$LABEL.plist"
fi
mkdir -p "$(dirname "$PLIST")" "$(dirname "$LOG")"
sed "s|__HOME__|$HOME|g" "launchd/$LABEL.plist" > "$PLIST"

# Only look at log lines written by this start.
seen=$(wc -l < "$LOG" 2>/dev/null || echo 0)
launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
launchctl bootstrap "$DOMAIN" "$PLIST"

echo "[5/5] Verifying..."
sleep 2
tail -n "+$((seen + 1))" "$LOG" 2>/dev/null | sed 's/^/  /'
tail -n "+$((seen + 1))" "$LOG" 2>/dev/null | grep -q "listening" \
    || warn "not listening yet (waiting for Tailscale?); see $LOG"
echo
"$BIN" status || warn "some peers are unreachable (fine until clipsync is set up on them)"
