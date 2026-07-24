# Setup / build helper for Hermes Desktop Watchdog (source distribution).
# Primary runtime target: Windows. Non-Windows builds use stubs.
param(
    [string]$OutputName = "hermes-watchdog.exe",
    [switch]$SkipTest
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
& (Join-Path $ScriptDir "Build-HermesGoWatchdog.ps1") -OutputName $OutputName -SkipTest:$SkipTest

Write-Host ""
Write-Host "NOTE: Full Desktop/Backend process supervision is Windows-only."
Write-Host "      Linux/macOS packages ship source + stubs for compile/smoke only."
