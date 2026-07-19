# HermesDesktopwatchdog — Agent Instructions

You are in the **standalone** Go watchdog repository (`zapabob/HermesDesktopwatchdog`).

## What this is

Operator-only Windows process that keeps Hermes Desktop and its backend alive.
It is **not** part of the Hermes agent tool surface.

## Do

- Build with `scripts/Build-HermesGoWatchdog.ps1`
- Start with `scripts/Start-HermesGoWatchdog.ps1 -HermesRoot <hermes-agent>`
- Keep secrets in env vars (`HERMES_WATCHDOG_ADMIN_TOKEN`, Tailscale auth keys)
- Keep `dist/` gitignored

## Do not

- Register this binary as a Hermes plugin, skill, MCP, or cron
- Expose Admin APIs without a token
- Commit auth keys, `.env`, or built `.exe` files
- Treat reserved ports `8787` / `9120` / dashboard listeners as Desktop backends

## Layout

| Path | Role |
|------|------|
| `*.go` | Watchdog sources (`package main`) |
| `scripts/Build-HermesGoWatchdog.ps1` | tidy / test / build → `dist/` |
| `scripts/Start-HermesGoWatchdog.ps1` | Detached launch helper |
| `dist/` | Local build output (ignored) |

## Tests

```powershell
go test ./... -count=1
```
