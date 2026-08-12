$ErrorActionPreference = "Stop"

$rootDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$testDir = Join-Path ([IO.Path]::GetTempPath()) ("nats-tui-installer-test-" + [guid]::NewGuid().ToString("N"))
$version = "1.2.3"
$tag = "v$version"
$asset = "nats-tui_${version}_windows_amd64.zip"
$releaseDir = Join-Path $testDir "releases\$tag"
$payloadDir = Join-Path $testDir "payload"
$installDir = Join-Path $testDir "bin"
$server = $null

try {
    New-Item -ItemType Directory -Path $releaseDir, $payloadDir | Out-Null
    Set-Content -Path (Join-Path $payloadDir "nats-tui.exe") -Value "nats-tui test binary" -NoNewline
    Compress-Archive -Path (Join-Path $payloadDir "nats-tui.exe") -DestinationPath (Join-Path $releaseDir $asset)

    $digest = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $releaseDir $asset)).Hash.ToLowerInvariant()
    Set-Content -Path (Join-Path $releaseDir "checksums.txt") -Value "$digest  $asset"
    Set-Content -Path (Join-Path $testDir "health") -Value "ok"

    $server = Start-Job -ArgumentList $testDir -ScriptBlock {
        param($contentRoot)
        $listener = [Net.HttpListener]::new()
        $listener.Prefixes.Add("http://127.0.0.1:18765/")
        $listener.Start()
        try {
            while ($listener.IsListening) {
                $context = $listener.GetContext()
                $relative = [Uri]::UnescapeDataString($context.Request.Url.AbsolutePath.TrimStart("/"))
                $path = Join-Path $contentRoot ($relative.Replace("/", [IO.Path]::DirectorySeparatorChar))
                if (Test-Path $path -PathType Leaf) {
                    $bytes = [IO.File]::ReadAllBytes($path)
                    $context.Response.StatusCode = 200
                    $context.Response.ContentLength64 = $bytes.Length
                    $context.Response.OutputStream.Write($bytes, 0, $bytes.Length)
                } else {
                    $context.Response.StatusCode = 404
                }
                $context.Response.Close()
            }
        } finally {
            $listener.Stop()
        }
    }

    $ready = $false
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:18765/health"
            if ($response.StatusCode -eq 200) {
                $ready = $true
                break
            }
        } catch {
            Start-Sleep -Milliseconds 100
        }
    }
    if (-not $ready) {
        throw "Local release server did not start"
    }

    $env:NATS_TUI_RELEASE_URL = "http://127.0.0.1:18765/releases"
    $env:PROCESSOR_ARCHITECTURE = "AMD64"
    & (Join-Path $rootDir "scripts\install.ps1") -Version $version -InstallDir $installDir -NoPathUpdate

    $installed = Join-Path $installDir "nats-tui.exe"
    if ((Get-Content -Raw $installed) -ne "nats-tui test binary") {
        throw "Installed binary does not match the release payload"
    }

    Add-Content -Path (Join-Path $releaseDir $asset) -Value "tampered" -NoNewline
    $rejected = $false
    try {
        & (Join-Path $rootDir "scripts\install.ps1") -Version $version -InstallDir $installDir -NoPathUpdate
    } catch {
        if ($_.Exception.Message -like "Checksum verification failed*") {
            $rejected = $true
        } else {
            throw
        }
    }
    if (-not $rejected) {
        throw "PowerShell installer accepted a tampered archive"
    }

    Write-Host "powershell installer e2e: ok"
} finally {
    Remove-Item Env:NATS_TUI_RELEASE_URL -ErrorAction SilentlyContinue
    if ($server) {
        Stop-Job $server -ErrorAction SilentlyContinue
        Remove-Job $server -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path $testDir) {
        Remove-Item -Path $testDir -Recurse -Force
    }
}
