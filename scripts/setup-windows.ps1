# ============================================================
# ClipSync Windows Setup Script
# Run in PowerShell (Admin NOT required)
# ============================================================

$ErrorActionPreference = "Stop"

# --- Config ---
$CLIPSYNC_DIR   = "$env:LOCALAPPDATA\clipsync"
$CLIPSYNC_EXE   = "$CLIPSYNC_DIR\clipsync.exe"
$CONFIG_DIR     = "$env:APPDATA\clipsync"
$CONFIG_FILE    = "$CONFIG_DIR\config.json"
$STARTUP_DIR    = "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup"
$VERSION        = "0.4.0"
$DOWNLOAD_URL   = "https://github.com/squispeb/clipsync/releases/download/v$VERSION/clipsync-windows-amd64.exe"

# --- Step 1: Download and install ---
Write-Host "`n[1/3] Installing ClipSync v$VERSION..." -ForegroundColor Cyan

New-Item -ItemType Directory -Path $CLIPSYNC_DIR -Force | Out-Null

# Check if binary already exists in common locations
$sources = @(
    "$env:USERPROFILE\Downloads\clipsync-windows-amd64.exe",
    "$env:USERPROFILE\Desktop\clipsync-windows-amd64.exe",
    "$env:USERPROFILE\Downloads\Taildrop\clipsync-windows-amd64.exe"
)

$found = $false
foreach ($src in $sources) {
    if (Test-Path $src) {
        Copy-Item $src $CLIPSYNC_EXE -Force
        Write-Host "  Installed from: $src" -ForegroundColor Green
        $found = $true
        break
    }
}

# If not found locally, download from GitHub
if (-not $found) {
    Write-Host "  Downloading from GitHub..." -ForegroundColor Yellow
    try {
        Invoke-WebRequest -Uri $DOWNLOAD_URL -OutFile $CLIPSYNC_EXE -UseBasicParsing
        Write-Host "  Downloaded to: $CLIPSYNC_EXE" -ForegroundColor Green
    } catch {
        Write-Host "  Download failed: $_" -ForegroundColor Red
        Write-Host "  Please download manually from:" -ForegroundColor Yellow
        Write-Host "  https://github.com/squispeb/clipsync/releases" -ForegroundColor Yellow
        exit 1
    }
}

# Verify
$ver = & $CLIPSYNC_EXE version 2>&1
Write-Host "  $ver" -ForegroundColor Green

# --- Step 2: Create config ---
Write-Host "`n[2/3] Creating default config..." -ForegroundColor Cyan

New-Item -ItemType Directory -Path $CONFIG_DIR -Force | Out-Null

if (-not (Test-Path $CONFIG_FILE)) {
    $config = @"
{
  "device_name": "$env:COMPUTERNAME",
  "peers": [],
  "bind": "",
  "port": 8275,
  "max_size": 10485760,
  "sync_interval_ms": 500,
  "token": "",
  "auto_discover": true,
  "history_max_items": 100,
  "history_max_memory_mb": 50
}
"@
    [System.IO.File]::WriteAllText($CONFIG_FILE, $config, [System.Text.UTF8Encoding]::new($false))
    Write-Host "  Config: $CONFIG_FILE" -ForegroundColor Green
} else {
    Write-Host "  Config already exists: $CONFIG_FILE" -ForegroundColor Yellow
}

# --- Step 3: Auto-start setup ---
Write-Host "`n[3/3] Setting up auto-start..." -ForegroundColor Cyan

# Create a simple batch file for startup
$batchContent = @"
@echo off
start /min "" "$CLIPSYNC_EXE" daemon
""@
$batchPath = "$STARTUP_DIR\clipsync-startup.bat"
$batchContent | Out-File -Encoding ascii $batchPath
Write-Host "  Startup script: $batchPath" -ForegroundColor Green

# Start daemon now
Write-Host "  Starting daemon..." -ForegroundColor Cyan
Start-Process -FilePath $CLIPSYNC_EXE -ArgumentList "daemon" -WindowStyle Hidden
Start-Sleep -Seconds 2

# Verify daemon is running
$proc = Get-Process -Name "clipsync" -ErrorAction SilentlyContinue
if ($proc) {
    Write-Host "  Daemon running (PID: $($proc.Id))" -ForegroundColor Green
} else {
    Write-Host "  Warning: daemon may not have started" -ForegroundColor Yellow
}

# --- Done ---
Write-Host "`n============================================" -ForegroundColor Cyan
Write-Host "  ClipSync setup complete!" -ForegroundColor Green
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Binary:  $CLIPSYNC_EXE"
Write-Host "  Config:  $CONFIG_FILE"
Write-Host "  Daemon:  Running (auto-starts on login)"
Write-Host ""
Write-Host "  Commands:"
Write-Host "    clipsync status     - Check peer connectivity"
Write-Host "    clipsync history    - View clipboard history"
Write-Host "    clipsync peers      - List configured peers"
Write-Host ""
Write-Host "  Note: Ensure Tailscale is running on all devices."
Write-Host ""
