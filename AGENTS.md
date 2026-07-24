# HermesDesktopwatchdog — Agent Instructions

You are in the **standalone** Go watchdog repository (`zapabob/HermesDesktopwatchdog`).
This is the **canonical** home for the Go watchdog formerly at `hermes-agent/scripts/windows/watchdog-go`.
Do not treat hermes-agent as the source of truth for this binary.

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

## Learned User Preferences

- Keep all product changes in this watchdog repo only; do not modify hermes-agent, the Desktop app, or sibling Hermes repos—document Hermes-facing contracts here instead.
- Restart authority must stay asymmetric: Watchdog alone may kill/restart Desktop or Backend; Desktop/Backend only report anomalies.
- Do not treat process-alive as healthy; keep liveness, readiness/deep health, and warm-start as separate concerns, with an explicit `ServiceState` machine (not bools).
- After building, prefer ForceRestart hot-swap of the running watchdog and verify `/api/status` rather than leaving an old binary running.
- Keep README skimable in about 30 seconds; put detailed architecture, ADR, and contracts under `_docs/`.

## Learned Workspace Facts

- Go module path is `github.com/zapabob/HermesDesktopwatchdog` (canonical standalone home for the former `hermes-agent/scripts/windows/watchdog-go` sources).
- Lifecycle design baseline and contracts live under `_docs/` (`ADR-2026-07-21_hermes-watchdog-lifecycle-manager.md`, `ARCHITECTURE.md`, `WARM-START-CONTRACT.md`, `IPC-CONTRACT-P3.md`, `OPERATOR.md`).
- `third_party/gorilla-csrf` is a local `replace` for `github.com/gorilla/csrf` (Filippo drop-in for CVE-2025-47909); do not remove without an equivalent remediation.
- Update-in-progress suppress is gated by `HERMES_WATCHDOG_UPDATE_IN_PROGRESS`, `%LOCALAPPDATA%\HermesWatchdog\update.lock`, and/or the admin update-suppress API.
