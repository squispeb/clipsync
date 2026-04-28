param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [int]$Port = 8000
)

$ErrorActionPreference = "Stop"

$distDir = Join-Path $RepoRoot "dist"
$zipPath = Join-Path $distDir "clipsync-windows-portable.zip"
$exePath = Join-Path $distDir "clipsync-windows-amd64.exe"
$manifestDir = Join-Path $RepoRoot "winget\local"
$tempManifestDir = Join-Path $RepoRoot "winget\local-temp"

if (-not (Test-Path $zipPath)) {
    Write-Host "Portable zip missing, building it now..." -ForegroundColor Yellow
    New-Item -ItemType Directory -Path $distDir -Force | Out-Null
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    & go build -ldflags="-s -w -X main.version=0.4.0" -o $exePath .
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed"
    }
    Compress-Archive -Path $exePath -DestinationPath $zipPath -Force
}

$hash = (Get-FileHash $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()

if (Test-Path $tempManifestDir) {
    Remove-Item $tempManifestDir -Recurse -Force
}
New-Item -ItemType Directory -Path $tempManifestDir -Force | Out-Null

Copy-Item (Join-Path $manifestDir "ClipSync.yaml") $tempManifestDir -Force
Copy-Item (Join-Path $manifestDir "ClipSync.locale.en-US.yaml") $tempManifestDir -Force
Copy-Item (Join-Path $manifestDir "ClipSync.installer.yaml") $tempManifestDir -Force

$installerFile = Join-Path $tempManifestDir "ClipSync.installer.yaml"
(Get-Content $installerFile) |
    ForEach-Object { $_ -replace '^\s*InstallerSha256:.*$', "    InstallerSha256: $hash" } |
    Set-Content $installerFile -Encoding utf8

$serverJob = Start-Job -ArgumentList $zipPath, $Port -ScriptBlock {
    param($ZipPath, $Port)

    $bytes = [System.IO.File]::ReadAllBytes($ZipPath)
    $listener = [System.Net.HttpListener]::new()
    $listener.Prefixes.Add("http://127.0.0.1:$Port/")
    $listener.Start()

    try {
        while ($true) {
            $context = $listener.GetContext()
            try {
                $response = $context.Response
                if ($context.Request.RawUrl -eq "/clipsync-windows-portable.zip") {
                    $response.StatusCode = 200
                    $response.ContentType = "application/zip"
                    $response.ContentLength64 = $bytes.Length
                    $response.OutputStream.Write($bytes, 0, $bytes.Length)
                } else {
                    $response.StatusCode = 404
                }
                $response.OutputStream.Close()
            } finally {
                $context.Response.Close()
            }
        }
    } finally {
        $listener.Stop()
    }
}

try {
    $probeUrl = "http://127.0.0.1:$Port/clipsync-windows-portable.zip"
    for ($i = 0; $i -lt 10; $i++) {
        try {
            Invoke-WebRequest -Uri $probeUrl -Method Head -UseBasicParsing | Out-Null
            break
        } catch {
            Start-Sleep -Seconds 1
        }
    }
    winget settings --enable LocalManifestFiles | Out-Null
    winget install --manifest $tempManifestDir --silent --accept-package-agreements --accept-source-agreements --no-proxy
} finally {
    Stop-Job $serverJob -ErrorAction SilentlyContinue | Out-Null
    Remove-Job $serverJob -Force -ErrorAction SilentlyContinue | Out-Null
}
