[CmdletBinding()]
param(
    [string]$Version = $env:NATS_TUI_VERSION,
    [string]$InstallDir = $(if ($env:NATS_TUI_INSTALL_DIR) { $env:NATS_TUI_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "nats-tui\bin" }),
    [string]$Repository = $(if ($env:NATS_TUI_REPO) { $env:NATS_TUI_REPO } else { "fd1az/nats-tui" }),
    [switch]$NoPathUpdate
)

$ErrorActionPreference = "Stop"

if (-not [Environment]::Is64BitOperatingSystem) {
    throw "nats-tui supports 64-bit Windows only"
}

$processor = $env:PROCESSOR_ARCHITECTURE
$arch = switch ($processor) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "Unsupported Windows architecture: $processor" }
}

if (-not $Version) {
    $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/latest"
    $Version = $latest.tag_name
}

$tag = if ($Version.StartsWith("v")) { $Version } else { "v$Version" }
$number = $tag.Substring(1)
$asset = "nats-tui_${number}_windows_${arch}.zip"
$releaseBase = if ($env:NATS_TUI_RELEASE_URL) { $env:NATS_TUI_RELEASE_URL } else { "https://github.com/$Repository/releases/download" }
$temporary = Join-Path ([IO.Path]::GetTempPath()) ("nats-tui-" + [guid]::NewGuid().ToString("N"))

try {
    New-Item -ItemType Directory -Path $temporary | Out-Null
    $archive = Join-Path $temporary $asset
    $checksums = Join-Path $temporary "checksums.txt"

    Write-Host "Downloading nats-tui $number for windows/$arch..."
    Invoke-WebRequest -Uri "$releaseBase/$tag/$asset" -OutFile $archive
    Invoke-WebRequest -Uri "$releaseBase/$tag/checksums.txt" -OutFile $checksums

    $escapedAsset = [regex]::Escape($asset)
    $checksumLine = Get-Content $checksums | Where-Object { $_ -match "^[A-Fa-f0-9]{64}\s+$escapedAsset$" } | Select-Object -First 1
    if (-not $checksumLine) {
        throw "Release checksum not found for $asset"
    }

    $expected = ($checksumLine -split "\s+")[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 -Path $archive).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        throw "Checksum verification failed for $asset"
    }

    Expand-Archive -Path $archive -DestinationPath $temporary -Force
    $executable = Join-Path $temporary "nats-tui.exe"
    if (-not (Test-Path $executable -PathType Leaf)) {
        throw "Release archive does not contain nats-tui.exe"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path $executable -Destination (Join-Path $InstallDir "nats-tui.exe") -Force

    if (-not $NoPathUpdate) {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $entries = @($userPath -split ";" | Where-Object { $_ })
        if ($entries -notcontains $InstallDir) {
            $updatedPath = (@($entries) + $InstallDir) -join ";"
            [Environment]::SetEnvironmentVariable("Path", $updatedPath, "User")
            $env:Path = "$env:Path;$InstallDir"
            Write-Host "Added $InstallDir to your user PATH. Open a new terminal to use it everywhere."
        }
    }

    Write-Host "Installed nats-tui $number at $InstallDir\nats-tui.exe"
} finally {
    if (Test-Path $temporary) {
        Remove-Item -Path $temporary -Recurse -Force
    }
}

