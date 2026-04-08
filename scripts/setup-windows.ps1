# ============================================================
# Cliplink Windows Setup Script
# Run in PowerShell (Admin not required)
# ============================================================

$ErrorActionPreference = "Stop"

# --- Config ---
$CLIPLINK_DIR   = "$env:LOCALAPPDATA\cliplink"
$CLIPLINK_EXE   = "$CLIPLINK_DIR\cliplink.exe"
$CONFIG_DIR     = "$env:APPDATA\cliplink"
$CONFIG_FILE    = "$CONFIG_DIR\config.json"
$STARTUP_DIR    = "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup"
$MAC_PEER       = "100.85.255.70:8275"  # <-- Change to your Mac's Tailscale IP
$LISTEN_PORT    = 8275

# --- Step 1: Install binary ---
Write-Host "`n[1/5] Installing cliplink..." -ForegroundColor Cyan

New-Item -ItemType Directory -Path $CLIPLINK_DIR -Force | Out-Null

# Look for the binary in common Taildrop locations
$sources = @(
    "$env:USERPROFILE\Downloads\cliplink-windows-amd64.exe",
    "$env:USERPROFILE\Desktop\cliplink-windows-amd64.exe",
    # Taildrop default on some Windows versions
    "$env:USERPROFILE\Downloads\Taildrop\cliplink-windows-amd64.exe"
)

$found = $false
foreach ($src in $sources) {
    if (Test-Path $src) {
        Copy-Item $src $CLIPLINK_EXE -Force
        Write-Host "  Copied from: $src" -ForegroundColor Green
        $found = $true
        break
    }
}

if (-not $found) {
    Write-Host "  Binary not found in Downloads/Desktop." -ForegroundColor Yellow
    Write-Host "  Please copy cliplink-windows-amd64.exe to: $CLIPLINK_EXE" -ForegroundColor Yellow
    Write-Host "  Then re-run this script." -ForegroundColor Yellow
    exit 1
}

# Verify
$ver = & $CLIPLINK_EXE version 2>&1
Write-Host "  Installed: $ver" -ForegroundColor Green

# --- Step 2: Create config ---
Write-Host "`n[2/5] Creating config..." -ForegroundColor Cyan

New-Item -ItemType Directory -Path $CONFIG_DIR -Force | Out-Null

$config = @"
{
  "peer": "$MAC_PEER",
  "port": $LISTEN_PORT,
  "max_size": 10485760
}
"@
# Write without BOM — PowerShell 5's -Encoding utf8 adds BOM which breaks Go's JSON parser
[System.IO.File]::WriteAllText($CONFIG_FILE, $config, [System.Text.UTF8Encoding]::new($false))
Write-Host "  Config: $CONFIG_FILE" -ForegroundColor Green
Write-Host "  Peer: $MAC_PEER" -ForegroundColor Green

# --- Step 3: Test connectivity ---
Write-Host "`n[3/5] Testing connection to Mac..." -ForegroundColor Cyan

$statusResult = & $CLIPLINK_EXE status 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "  Mac daemon reachable: $statusResult" -ForegroundColor Green
} else {
    Write-Host "  Mac daemon not reachable (is it running?): $statusResult" -ForegroundColor Yellow
    Write-Host "  Continuing setup anyway..." -ForegroundColor Yellow
}

# --- Step 4: Auto-start daemon ---
Write-Host "`n[4/5] Setting up daemon auto-start..." -ForegroundColor Cyan

$vbsContent = @"
Set WshShell = CreateObject("WScript.Shell")
WshShell.Run """$CLIPLINK_EXE"" daemon", 0, False
"@
$vbsPath = "$STARTUP_DIR\cliplink-daemon.vbs"
$vbsContent | Out-File -Encoding ascii $vbsPath
Write-Host "  Startup script: $vbsPath" -ForegroundColor Green

# Start daemon now
Write-Host "  Starting daemon..." -ForegroundColor Cyan
Start-Process -FilePath $CLIPLINK_EXE -ArgumentList "daemon" -WindowStyle Hidden
Start-Sleep -Seconds 1

# Verify daemon is running
$proc = Get-Process -Name "cliplink" -ErrorAction SilentlyContinue
if ($proc) {
    Write-Host "  Daemon running (PID: $($proc.Id))" -ForegroundColor Green
} else {
    Write-Host "  Warning: daemon may not have started" -ForegroundColor Yellow
}

# --- Step 5: Hotkey setup (AutoHotkey) ---
Write-Host "`n[5/5] Setting up hotkey..." -ForegroundColor Cyan

$ahkScript = @"
; Cliplink: Ctrl+Alt+V to send clipboard to remote
; (mirrors Cmd+Option+V on Mac)
#Requires AutoHotkey v2.0

^!v::
{
    result := RunWait('"$($CLIPLINK_EXE.Replace('\','\\'))" send',, "Hide")
    if (result = 0)
        ToolTip("Clipboard sent")
    else
        ToolTip("Send failed")
    SetTimer(() => ToolTip(), -1500)
}
"@
$ahkPath = "$CLIPLINK_DIR\cliplink-hotkey.ahk"
[System.IO.File]::WriteAllText($ahkPath, $ahkScript, [System.Text.UTF8Encoding]::new($false))

# Create startup shortcut for AHK script
$ahkExe = "${env:ProgramFiles}\AutoHotkey\v2\AutoHotkey64.exe"
if (-not (Test-Path $ahkExe)) {
    $ahkExe = "${env:ProgramFiles}\AutoHotkey\AutoHotkey.exe"
}

if (Test-Path $ahkExe) {
    $WshShell = New-Object -ComObject WScript.Shell
    $Shortcut = $WshShell.CreateShortcut("$STARTUP_DIR\cliplink-hotkey.lnk")
    $Shortcut.TargetPath = $ahkExe
    $Shortcut.Arguments = """$ahkPath"""
    $Shortcut.Save()
    Write-Host "  AHK script: $ahkPath" -ForegroundColor Green
    Write-Host "  Hotkey: Ctrl+Alt+V" -ForegroundColor Green

    # Start hotkey now
    Start-Process -FilePath $ahkExe -ArgumentList """$ahkPath"""
    Write-Host "  Hotkey active!" -ForegroundColor Green
} else {
    Write-Host "  AutoHotkey not found." -ForegroundColor Yellow
    Write-Host "  Install from: https://www.autohotkey.com/" -ForegroundColor Yellow
    Write-Host "  Then run: $ahkPath" -ForegroundColor Yellow
}

# --- Done ---
Write-Host "`n============================================" -ForegroundColor Cyan
Write-Host "  Cliplink setup complete!" -ForegroundColor Green
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Binary:  $CLIPLINK_EXE"
Write-Host "  Config:  $CONFIG_FILE"
Write-Host "  Daemon:  Running (auto-starts on login)"
Write-Host "  Hotkey:  Ctrl+Alt+V (send clipboard to Mac)"
Write-Host ""
Write-Host "  Test: copy text, press Ctrl+Alt+V, then Cmd+V on Mac"
Write-Host ""
