# Package multi-platform release archives for GitHub Releases (v1.1.0+).
param(
    [string]$Version = "v1.1.0"
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "..")).Path
$StagingRoot = Join-Path $Root "release-staging"
$OutDir = Join-Path $Root "release-out"
$Dist = Join-Path $Root "dist"

New-Item -ItemType Directory -Force -Path $StagingRoot, $OutDir | Out-Null
Get-ChildItem $StagingRoot -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
Get-ChildItem $OutDir -Filter "hermes-watchdog-*-$Version.tar.gz*" -ErrorAction SilentlyContinue | Remove-Item -Force

function New-PackageDir([string]$Name) {
    $p = Join-Path $StagingRoot $Name
    New-Item -ItemType Directory -Force -Path $p | Out-Null
    return $p
}

function Copy-CommonDocs([string]$Dest) {
    Copy-Item (Join-Path $Root "LICENSE") $Dest
    Copy-Item (Join-Path $Root "README.md") $Dest
    Copy-Item (Join-Path $Root "SECURITY.md") $Dest
    Copy-Item (Join-Path $Root ".env.example") $Dest -ErrorAction SilentlyContinue
    $scripts = Join-Path $Dest "scripts"
    New-Item -ItemType Directory -Force -Path $scripts | Out-Null
    Copy-Item (Join-Path $Root "scripts\Build-HermesGoWatchdog.ps1") $scripts
    Copy-Item (Join-Path $Root "scripts\Start-HermesGoWatchdog.ps1") $scripts
    Copy-Item (Join-Path $Root "scripts\Setup-HermesGoWatchdog.ps1") $scripts
    Copy-Item (Join-Path $Root "scripts\Setup-HermesGoWatchdog.sh") $scripts
}

function Copy-SourceTree([string]$Dest) {
    $src = Join-Path $Dest "src"
    New-Item -ItemType Directory -Force -Path $src | Out-Null
    Get-ChildItem $Root -File -Include *.go,go.mod,go.sum,LICENSE,README.md,SECURITY.md,.env.example |
        Copy-Item -Destination $src
    # package main lives at repo root — copy all .go + module files
    Copy-Item (Join-Path $Root "*.go") $src
    Copy-Item (Join-Path $Root "go.mod") $src
    Copy-Item (Join-Path $Root "go.sum") $src
    Copy-Item (Join-Path $Root "LICENSE") $src
    Copy-Item (Join-Path $Root "README.md") $src
    Copy-Item (Join-Path $Root "SECURITY.md") $src
    if (Test-Path (Join-Path $Root ".env.example")) {
        Copy-Item (Join-Path $Root ".env.example") $src
    }
    $tp = Join-Path $Root "third_party"
    if (Test-Path $tp) {
        Copy-Item $tp (Join-Path $src "third_party") -Recurse
    }
    $scripts = Join-Path $src "scripts"
    New-Item -ItemType Directory -Force -Path $scripts | Out-Null
    Copy-Item (Join-Path $Root "scripts\*") $scripts
    $docs = Join-Path $src "_docs"
    New-Item -ItemType Directory -Force -Path $docs | Out-Null
    Copy-Item (Join-Path $Root "_docs\ARCHITECTURE.md") $docs -ErrorAction SilentlyContinue
    Copy-Item (Join-Path $Root "_docs\OPERATOR.md") $docs -ErrorAction SilentlyContinue
    Copy-Item (Join-Path $Root "_docs\WARM-START-CONTRACT.md") $docs -ErrorAction SilentlyContinue
    Copy-Item (Join-Path $Root "_docs\IPC-CONTRACT-P3.md") $docs -ErrorAction SilentlyContinue
}

function Write-PlatformReadme([string]$Dest, [string]$Platform, [bool]$HasBinary) {
    $lines = @(
        "# Hermes Desktop Watchdog $Version ($Platform)",
        "",
        "Operator-only Go lifecycle manager for Hermes Desktop + hermes serve.",
        "Primary supported runtime: **Windows**. Restart authority is Windows-first.",
        ""
    )
    if ($Platform -eq "windows-amd64") {
        $lines += @(
            "## Contents",
            "- ``hermes-watchdog.exe`` — prebuilt Windows amd64 binary",
            "- ``scripts/`` — Build / Start / Setup helpers",
            "- LICENSE, README, SECURITY",
            "",
            "## Quick start",
            "``````powershell",
            "tar -xzf hermes-watchdog-windows-amd64-$Version.tar.gz",
            "`$env:HERMES_WATCHDOG_ADMIN_TOKEN = 'your-secure-operator-token'",
            ".\hermes-watchdog.exe",
            "# Default listen: 127.0.0.1:9920",
            "``````"
        )
    } else {
        $lines += @(
            "## Honest scope",
            "- This archive is a **source (+ optional stub binary)** distribution.",
            "- Job Objects, Named Pipes, Hermes.exe WMI supervision: **Windows only**.",
            "- Non-Windows stubs allow ``go test`` / ``go build`` / HTTP smoke — not full Desktop supervision.",
            "",
            "## Build from source",
            "``````bash",
            "tar -xzf hermes-watchdog-$Platform-$Version.tar.gz",
            "cd hermes-watchdog-$Platform-$Version",
            "bash scripts/Setup-HermesGoWatchdog.sh ./hermes-watchdog",
            "``````"
        )
        if ($HasBinary) {
            $lines += @(
                "",
                "## Optional binary",
                "A cross-compiled binary is included for convenience. It will not supervise Windows Desktop processes."
            )
        }
    }
    Set-Content -LiteralPath (Join-Path $Dest "PLATFORM.md") -Value ($lines -join "`n") -Encoding utf8
}

function New-TarGz([string]$FolderName, [string]$ArchiveName) {
    $archive = Join-Path $OutDir $ArchiveName
    Push-Location $StagingRoot
    try {
        if (Test-Path $archive) { Remove-Item $archive -Force }
        tar -czf $archive $FolderName
        if ($LASTEXITCODE -ne 0) { throw "tar failed for $ArchiveName" }
    } finally {
        Pop-Location
    }
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash.ToLowerInvariant()
    Set-Content -LiteralPath ($archive + ".sha256") -Value $hash -Encoding ascii -NoNewline
    return [pscustomobject]@{ Name = $ArchiveName; Path = $archive; SHA256 = $hash }
}

$assets = @()

# --- Windows (binary + scripts) ---
$winName = "hermes-watchdog-windows-amd64-$Version"
$winDir = New-PackageDir $winName
Copy-CommonDocs $winDir
Write-PlatformReadme $winDir "windows-amd64" $true
$winExe = Join-Path $Dist "hermes-watchdog.exe"
if (-not (Test-Path $winExe)) { throw "missing $winExe — build first" }
Copy-Item $winExe (Join-Path $winDir "hermes-watchdog.exe")
$assets += New-TarGz $winName "$winName.tar.gz"

# --- Linux (source + optional binary) ---
$linuxName = "hermes-watchdog-linux-amd64-$Version"
$linuxDir = New-PackageDir $linuxName
Copy-CommonDocs $linuxDir
Copy-SourceTree $linuxDir
Write-PlatformReadme $linuxDir "linux-amd64" $true
$linuxBin = Join-Path $Dist "hermes-watchdog-linux-amd64"
if (Test-Path $linuxBin) {
    Copy-Item $linuxBin (Join-Path $linuxDir "hermes-watchdog")
}
$assets += New-TarGz $linuxName "$linuxName.tar.gz"

# --- Darwin amd64 ---
$darAmdName = "hermes-watchdog-darwin-amd64-$Version"
$darAmdDir = New-PackageDir $darAmdName
Copy-CommonDocs $darAmdDir
Copy-SourceTree $darAmdDir
Write-PlatformReadme $darAmdDir "darwin-amd64" $true
$darAmdBin = Join-Path $Dist "hermes-watchdog-darwin-amd64"
if (Test-Path $darAmdBin) {
    Copy-Item $darAmdBin (Join-Path $darAmdDir "hermes-watchdog")
}
$assets += New-TarGz $darAmdName "$darAmdName.tar.gz"

# --- Darwin arm64 ---
$darArmName = "hermes-watchdog-darwin-arm64-$Version"
$darArmDir = New-PackageDir $darArmName
Copy-CommonDocs $darArmDir
Copy-SourceTree $darArmDir
Write-PlatformReadme $darArmDir "darwin-arm64" $true
$darArmBin = Join-Path $Dist "hermes-watchdog-darwin-arm64"
if (Test-Path $darArmBin) {
    Copy-Item $darArmBin (Join-Path $darArmDir "hermes-watchdog")
}
$assets += New-TarGz $darArmName "$darArmName.tar.gz"

# --- Full source ---
$srcName = "hermes-watchdog-src-$Version"
$srcDir = New-PackageDir $srcName
Copy-SourceTree $srcDir
# Flatten: move src/* up one level for cleaner extract
Get-ChildItem (Join-Path $srcDir "src") | ForEach-Object {
    Move-Item $_.FullName (Join-Path $srcDir $_.Name) -Force
}
Remove-Item (Join-Path $srcDir "src") -Recurse -Force -ErrorAction SilentlyContinue
Copy-CommonDocs $srcDir
Set-Content -LiteralPath (Join-Path $srcDir "PLATFORM.md") -Value @"
# Hermes Desktop Watchdog $Version (full source)

Windows-first operator watchdog. Build on Windows with ``scripts/Setup-HermesGoWatchdog.ps1``.
Non-Windows: ``scripts/Setup-HermesGoWatchdog.sh`` (stubs only — not full Desktop supervision).
"@ -Encoding utf8
$assets += New-TarGz $srcName "$srcName.tar.gz"

# SHA256SUMS
$sumsPath = Join-Path $OutDir "SHA256SUMS"
$sumsLines = foreach ($a in $assets) {
    "{0}  {1}" -f $a.SHA256, $a.Name
}
Set-Content -LiteralPath $sumsPath -Value ($sumsLines -join "`n") -Encoding ascii

# Per-package SHA256SUMS excerpt
foreach ($a in $assets) {
    $folder = Join-Path $StagingRoot ($a.Name -replace '\.tar\.gz$', '')
    if (Test-Path $folder) {
        Set-Content -LiteralPath (Join-Path $folder "SHA256SUMS.txt") -Value ("{0}  {1}" -f $a.SHA256, $a.Name) -Encoding ascii
    }
}

Write-Host ""
Write-Host "=== Release assets ($Version) ==="
$assets | Format-Table Name, SHA256 -AutoSize
Write-Host "SHA256SUMS -> $sumsPath"
$assets | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $OutDir "assets-$Version.json") -Encoding utf8
