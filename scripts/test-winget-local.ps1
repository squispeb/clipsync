param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [int]$Port = 8000
)

$ErrorActionPreference = "Stop"

$distDir = Join-Path $RepoRoot "dist"
$zipPath = Join-Path $distDir "clipsync-windows-portable.zip"
$manifestDir = Join-Path $RepoRoot "winget\local"

if (-not (Test-Path $zipPath)) {
    throw "Missing $zipPath. Run 'make build-windows-portable' first."
}

$serverJob = Start-Job -ArgumentList $zipPath, $Port -ScriptBlock {
    param($ZipPath, $Port)

    $bytes = [System.IO.File]::ReadAllBytes($ZipPath)
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, [int]$Port)
    $listener.Start()

    try {
        while ($true) {
            $client = $listener.AcceptTcpClient()
            try {
                $stream = $client.GetStream()
                $reader = New-Object System.IO.StreamReader($stream)
                $requestLine = $reader.ReadLine()

                while ($true) {
                    $line = $reader.ReadLine()
                    if ([string]::IsNullOrEmpty($line)) {
                        break
                    }
                }

                if ($requestLine -like 'GET /clipsync-windows-portable.zip*') {
                    $header = "HTTP/1.1 200 OK`r`nContent-Type: application/zip`r`nContent-Length: $($bytes.Length)`r`nConnection: close`r`n`r`n"
                    $headerBytes = [System.Text.Encoding]::ASCII.GetBytes($header)
                    $stream.Write($headerBytes, 0, $headerBytes.Length)
                    $stream.Write($bytes, 0, $bytes.Length)
                } else {
                    $notFound = [System.Text.Encoding]::ASCII.GetBytes("HTTP/1.1 404 Not Found`r`nContent-Length: 0`r`nConnection: close`r`n`r`n")
                    $stream.Write($notFound, 0, $notFound.Length)
                }

                $stream.Close()
            } finally {
                $client.Close()
            }
        }
    } finally {
        $listener.Stop()
    }
}

try {
    Start-Sleep -Seconds 1
    winget settings --enable LocalManifestFiles | Out-Null
    winget install --manifest $manifestDir --silent --accept-package-agreements --accept-source-agreements
} finally {
    Stop-Job $serverJob -ErrorAction SilentlyContinue | Out-Null
    Remove-Job $serverJob -Force -ErrorAction SilentlyContinue | Out-Null
}
