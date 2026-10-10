#!/bin/bash
# ============================================================
# ClipSync macOS setup
#
# Builds clipsync natively (it uses cgo for NSPasteboard), installs it to
# ~/.local/bin/clipsync and runs it as a per-user LaunchAgent.
#
#   ./scripts/setup-mac.sh
#
# Needs Go and the Xcode command line tools. Configure peers and the shared
# token in ~/Library/Application Support/clipsync/config.json (created with
# defaults on first run).
# ============================================================

set -euo pipefail

cd "$(dirname "$0")/.."

BIN="$HOME/.local/bin/clipsync"
LABEL="com.clipsync.daemon"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG="$HOME/Library/Logs/clipsync.log"
DOMAIN="gui/$(id -u)"

echo "[1/3] Building..."
make build-mac-native
mkdir -p "$(dirname "$BIN")"
install -m 0755 "dist/clipsync-mac-$(uname -m)" "$BIN"
echo "  Installed: $("$BIN" version) -> $BIN"

echo "[2/3] Installing LaunchAgent..."
# Remove the old system-wide daemon from earlier instructions, if present.
if [ -f "/Library/LaunchDaemons/$LABEL.plist" ]; then
    echo "  Note: /Library/LaunchDaemons/$LABEL.plist exists from an older install;"
    echo "        remove it with: sudo launchctl bootout system/$LABEL; sudo rm /Library/LaunchDaemons/$LABEL.plist"
fi
mkdir -p "$(dirname "$PLIST")" "$(dirname "$LOG")"
sed "s|__HOME__|$HOME|g" launchd/$LABEL.plist > "$PLIST"

echo "[3/3] (Re)starting..."
# Only look at log lines written by this start.
seen=$(wc -l < "$LOG" 2>/dev/null || echo 0)
launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
launchctl bootstrap "$DOMAIN" "$PLIST"
sleep 2

if tail -n "+$((seen + 1))" "$LOG" 2>/dev/null | grep -q "listening"; then
    echo "  $(tail -n "+$((seen + 1))" "$LOG" | grep "listening" | tail -1)"
else
    echo "  Daemon not listening yet; check $LOG"
fi
