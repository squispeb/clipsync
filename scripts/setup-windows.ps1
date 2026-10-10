# ============================================================
# ClipSync Windows setup (PowerShell, no admin needed)
#
# Installs %LOCALAPPDATA%\ClipSync\clipsync.exe (built from this checkout
# when Go is installed, otherwise downloaded from the latest GitHub release),
# writes the config, and starts the daemon hidden at every login.
#
#   .\scripts\setup-windows.ps1                    # first machine: makes a token
#   $env:CLIPSYNC_TOKEN = "..."; $env:CLIPSYNC_PEERS = "100.x.y.z"; .\scripts\setup-windows.ps1
#
# CLIPSYNC_TOKEN  shared token (every machine needs the same one)
# CLIPSYNC_PEERS  comma-separated peer Tailscale IPs or hostnames, [:port]
#
# Without either variable an existing config is kept; with no config a new
# token is generated and printed.
# ============================================================

$ErrorActionPreference = "Stop"

$InstallDir  = Join-Path $env:LOCALAPPDATA "ClipSync"
$Exe         = Join-Path $InstallDir "clipsync.exe"
$LogFile     = Join-Path $InstallDir "clipsync.log"
$ConfigFile  = Join-Path $env:APPDATA "clipsync\config.json"
$StartupDir  = [Environment]::GetFolderPath("Startup")
$Shortcut    = Join-Path $StartupDir "ClipSync.lnk"
$RepoRoot    = Split-Path -Parent $PSScriptRoot
$DownloadUrl = "https://github.com/squispeb/clipsync/releases/latest/download/clipsync-windows-amd64.exe"

function Warn($msg) { Write-Host "  warning: $msg" -ForegroundColor Yellow }

Write-Host "[1/5] Checking prerequisites..." -ForegroundColor Cyan
$tailscale = Get-Command tailscale -ErrorAction SilentlyContinue
if (-not $tailscale) {
    Warn "tailscale CLI not found; install Tailscale (https://tailscale.com/download/windows)"
} else {
    & tailscale ip -4 *> $null
    if ($LASTEXITCODE -ne 0) { Warn "Tailscale is not connected; the daemon will wait for it" }
}

Write-Host "[2/5] Installing binary..." -ForegroundColor Cyan
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
# A running daemon locks the exe.
Get-Process clipsync -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500

$go = Get-Command go -ErrorAction SilentlyContinue
if ($go -and (Test-Path (Join-Path $RepoRoot "go.mod"))) {
    Write-Host "  building from $RepoRoot"
    Push-Location $RepoRoot
    try {
        $env:CGO_ENABLED = "0"
        & go build -ldflags "-s -w" -o $Exe .
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    } finally {
        Pop-Location
    }
} else {
    Write-Host "  Go not found; downloading the latest release"
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $Exe -UseBasicParsing
}
Write-Host "  installed $(& $Exe version) -> $Exe" -ForegroundColor Green

# Put clipsync on the user PATH for new terminals.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not (($userPath -split ";") -contains $InstallDir)) {
    [Environment]::SetEnvironmentVariable("Path", ($userPath.TrimEnd(";") + ";" + $InstallDir), "User")
    Write-Host "  added $InstallDir to your PATH (open a new terminal to use it)"
}

Write-Host "[3/5] Configuring..." -ForegroundColor Cyan
if ($env:CLIPSYNC_TOKEN -or $env:CLIPSYNC_PEERS) {
    $initArgs = @("init")
    if ($env:CLIPSYNC_PEERS) {
        foreach ($p in ($env:CLIPSYNC_PEERS -split ",")) {
            $p = $p.Trim()
            if ($p) { $initArgs += @("--peer", $p) }
        }
    }
    & $Exe @initArgs   # reads CLIPSYNC_TOKEN itself
} elseif (Test-Path $ConfigFile) {
    Write-Host "  keeping $ConfigFile (set CLIPSYNC_TOKEN / CLIPSYNC_PEERS to change it)"
} else {
    & $Exe init --new-token
}
if ($LASTEXITCODE -ne 0) { throw "clipsync init failed" }

Write-Host "[4/5] Starting at login..." -ForegroundColor Cyan
# Remove the batch file used by older versions of this script.
Remove-Item (Join-Path $StartupDir "clipsync-startup.bat") -ErrorAction SilentlyContinue
$daemonArgs = "daemon --log-file `"$LogFile`""
$shell = New-Object -ComObject WScript.Shell
$lnk = $shell.CreateShortcut($Shortcut)
$lnk.TargetPath = $Exe
$lnk.Arguments = $daemonArgs
$lnk.WorkingDirectory = $InstallDir
$lnk.WindowStyle = 7   # minimized
$lnk.Save()
Write-Host "  $Shortcut"

Write-Host "[5/5] Starting and verifying..." -ForegroundColor Cyan
Start-Process -FilePath $Exe -ArgumentList $daemonArgs -WindowStyle Hidden
Start-Sleep -Seconds 3
if (-not (Get-Process clipsync -ErrorAction SilentlyContinue)) {
    throw "daemon exited; see $LogFile"
}
Get-Content $LogFile -Tail 5 | ForEach-Object { "  $_" }
Write-Host ""
Write-Host "If Windows asks whether clipsync may accept connections, allow it on private networks." -ForegroundColor Yellow
& $Exe status
if ($LASTEXITCODE -ne 0) { Warn "some peers are unreachable (fine until clipsync is set up on them)" }
