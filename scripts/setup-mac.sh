#!/bin/bash
# ============================================================
# Cliplink macOS Setup Script
# Run: chmod +x scripts/setup-mac.sh && ./scripts/setup-mac.sh
# ============================================================

set -e

# --- Config ---
CLIPLINK_BIN="$HOME/.local/bin/cliplink"
CONFIG_DIR="$HOME/Library/Application Support/cliplink"
CONFIG_FILE="$CONFIG_DIR/config.json"
PLIST_FILE="$HOME/Library/LaunchAgents/com.cliplink.daemon.plist"
WINDOWS_PEER="100.77.9.64:8275"  # <-- Change to your Windows Tailscale IP
LISTEN_PORT=8275

echo ""
echo "=== Cliplink macOS Setup ==="
echo ""

# --- Step 1: Install binary ---
echo "[1/4] Installing binary..."
mkdir -p "$(dirname "$CLIPLINK_BIN")"

# Find the right binary for this architecture
ARCH=$(uname -m)
if [ "$ARCH" = "arm64" ]; then
    SRC="dist/cliplink-mac-arm64"
else
    SRC="dist/cliplink-mac-amd64"
fi

if [ -f "$SRC" ]; then
    cp "$SRC" "$CLIPLINK_BIN"
    chmod +x "$CLIPLINK_BIN"
    echo "  Installed: $($CLIPLINK_BIN version)"
elif [ -f "cliplink" ]; then
    cp cliplink "$CLIPLINK_BIN"
    chmod +x "$CLIPLINK_BIN"
    echo "  Installed: $($CLIPLINK_BIN version)"
else
    echo "  Error: binary not found. Run 'make build' first."
    exit 1
fi

# --- Step 2: Create config ---
echo "[2/4] Creating config..."
mkdir -p "$CONFIG_DIR"

cat > "$CONFIG_FILE" << EOF
{
  "peer": "$WINDOWS_PEER",
  "port": $LISTEN_PORT,
  "max_size": 10485760
}
EOF
echo "  Config: $CONFIG_FILE"
echo "  Peer: $WINDOWS_PEER"

# --- Step 3: Setup launchd daemon ---
echo "[3/4] Setting up daemon (launchd)..."

cat > "$PLIST_FILE" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.cliplink.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>$CLIPLINK_BIN</string>
        <string>daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/cliplink.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/cliplink.log</string>
</dict>
</plist>
EOF

# Reload daemon
launchctl unload "$PLIST_FILE" 2>/dev/null || true
launchctl load "$PLIST_FILE"
sleep 1

if tail -1 /tmp/cliplink.log 2>/dev/null | grep -q "listening"; then
    echo "  Daemon running: $(tail -1 /tmp/cliplink.log)"
else
    echo "  Warning: daemon may not have started. Check /tmp/cliplink.log"
fi

# --- Step 4: Hammerspoon hotkey ---
echo "[4/4] Setting up hotkey (Hammerspoon)..."

HAMMERSPOON_DIR="$HOME/.hammerspoon"
mkdir -p "$HAMMERSPOON_DIR"

# Only write if not already configured
if grep -q "cliplink" "$HAMMERSPOON_DIR/init.lua" 2>/dev/null; then
    echo "  Hammerspoon already configured for cliplink"
else
    cat >> "$HAMMERSPOON_DIR/init.lua" << 'HSEOF'

-- Cliplink: send clipboard to remote machine via Tailscale
-- Hotkey: Cmd+Option+V
local toastCanvas = nil
local toastTimer = nil

local function cliplink_toast(text, success)
    if toastCanvas then toastCanvas:delete() end
    if toastTimer then toastTimer:stop() end
    local screen = hs.screen.mainScreen():frame()
    local w, h = math.max(#text * 8 + 32, 160), 36
    local bgColor = success
        and { red = 0.18, green = 0.22, blue = 0.18, alpha = 0.88 }
        or  { red = 0.28, green = 0.16, blue = 0.16, alpha = 0.88 }
    local dotColor = success
        and { red = 0.35, green = 0.78, blue = 0.48, alpha = 1.0 }
        or  { red = 0.90, green = 0.35, blue = 0.35, alpha = 1.0 }
    toastCanvas = hs.canvas.new({ x = screen.x + screen.w - w - 16, y = screen.y + 12, w = w, h = h })
    toastCanvas:behavior(hs.canvas.windowBehaviors.canJoinAllSpaces)
    toastCanvas:level(hs.canvas.windowLevels.overlay)
    toastCanvas:appendElements(
        { type = "rectangle", roundedRectRadii = { xRadius = 10, yRadius = 10 },
          fillColor = bgColor, strokeColor = { white = 1, alpha = 0.06 }, strokeWidth = 0.5, action = "strokeAndFill" },
        { type = "circle", center = { x = 16, y = h/2 }, radius = 4, fillColor = dotColor, action = "fill" },
        { type = "text", frame = { x = 28, y = (h-18)/2, w = w-36, h = 18 },
          text = hs.styledtext.new(text, { font = { name = ".AppleSystemUIFontRounded-Medium", size = 13 },
          color = { white = 1, alpha = 0.95 }, paragraphStyle = { alignment = "center" } }) }
    )
    toastCanvas:show()
    toastTimer = hs.timer.doAfter(success and 1.2 or 2.5, function()
        if toastCanvas then toastCanvas:delete(); toastCanvas = nil end
    end)
end

hs.hotkey.bind({"cmd", "alt"}, "V", function()
    hs.task.new(os.getenv("HOME") .. "/.local/bin/cliplink", function(exitCode, stdOut, stdErr)
        if exitCode == 0 then
            cliplink_toast("Clipboard sent", true)
        else
            local msg = ((stdErr ~= "" and stdErr) or stdOut or "unknown error"):gsub("^%s+", ""):gsub("%s+$", ""):gsub("error: ", "")
            cliplink_toast(msg, false)
        end
    end, {"send"}):start()
end)
HSEOF
    echo "  Hotkey configured: Cmd+Option+V"
fi

echo ""
echo "============================================"
echo "  Cliplink macOS setup complete!"
echo "============================================"
echo ""
echo "  Binary:  $CLIPLINK_BIN"
echo "  Config:  $CONFIG_FILE"
echo "  Daemon:  Running (auto-starts on login)"
echo "  Hotkey:  Cmd+Option+V (send clipboard to Windows)"
echo ""
echo "  Reload Hammerspoon: click menu bar icon → Reload Config"
echo "  Test: copy text, press Cmd+Option+V, then Ctrl+V on Windows"
echo ""
