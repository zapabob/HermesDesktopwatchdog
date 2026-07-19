# Start Go-based Hermes Desktop<->backend watchdog (operator-only; NOT agent-reachable).
param(
    [int]$IntervalSec = 20,
    [int]$FailThreshold = 2,
    [switch]$Once,
    [switch]$NoPrewarm,
    [switch]$NoTsnet,
    [string]$Listen = "127.0.0.1:9920",
    [string]$HermesRoot = "",
    [string]$HermesHome = "",
    [switch]$BuildIfMissing
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$WatchdogRoot = (Resolve-Path (Join-Path $ScriptDir "..")).Path
$RepoRoot = if ($HermesRoot) { $HermesRoot } else {
    # Prefer sibling hermes-agent checkout when present
    $sibling = Join-Path (Split-Path -Parent $WatchdogRoot) "hermes-agent"
    if (Test-Path -LiteralPath $sibling) { (Resolve-Path -LiteralPath $sibling).Path } else { $WatchdogRoot }
}
if (-not $HermesHome) { $HermesHome = Join-Path $env:USERPROFILE ".hermes" }

$Exe = Join-Path $WatchdogRoot "dist\hermes-watchdog.exe"
if (-not (Test-Path -LiteralPath $Exe)) {
    if ($BuildIfMissing) {
        & (Join-Path $ScriptDir "Build-HermesGoWatchdog.ps1")
    } else {
        throw "Missing $Exe — run scripts\Build-HermesGoWatchdog.ps1 first or pass -BuildIfMissing"
    }
}

$argv = @(
    "-interval=$IntervalSec",
    "-fail-threshold=$FailThreshold",
    "-hermes-root=`"$RepoRoot`"",
    "-hermes-home=`"$HermesHome`"",
    "-listen=$Listen"
)
if ($Once) { $argv += "-once" }
if ($NoPrewarm) { $argv += "-prewarm-backend=false" }
if (-not $NoTsnet -and ($env:HERMES_WATCHDOG_TS_AUTHKEY -or $env:TS_AUTHKEY)) {
    $argv += "-tsnet"
}

$env:HERMES_HOME = $HermesHome
Write-Host "Starting Go watchdog: $Exe $($argv -join ' ')"
Start-Process -FilePath $Exe -ArgumentList $argv -WindowStyle Hidden -WorkingDirectory (Split-Path -Parent $Exe) | Out-Null
Write-Host "Go watchdog launched (logs: $(Join-Path $HermesHome 'logs\hermes-go-watchdog.log'))"
