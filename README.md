# Hermes Desktop Watchdog

Operator-only **Windows Go process** that keeps Hermes Desktop (`Hermes.exe`) and its backend (`hermes serve`) alive via health checks, crash-loop backoff, and orphan cleanup.

**Not** a Hermes plugin, MCP server, skill, cron, or official NousResearch component.

## Build & start

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\Build-HermesGoWatchdog.ps1

$env:HERMES_WATCHDOG_ADMIN_TOKEN = "your-secure-operator-token"
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\Start-HermesGoWatchdog.ps1
```

Binary → `dist\hermes-watchdog.exe` (gitignored). Default listen: `127.0.0.1:9920`.

## Phase status (shipped)

- **P1** — `ServiceState` machine + crash-loop `RestartPolicy` (watchdog is sole restart authority)
- **P2** — Mutual health (`/health`, `/api/status`, backend live/ready) + heartbeat ingest (`POST /api/v1/heartbeat`)
- **P3** — Report-only IPC (Named Pipe + HTTP `/api/v1/report` / `/command` / `/ipc`); Desktop/Backend cannot restart themselves

Planned (not shipped): drain/checkpoint (P4), Job Object / renderer recovery (P5), update-window suppress (P6) — see ADRs.

## Safety

Loopback-only by default + admin token for mutations. Details: [SECURITY.md](SECURITY.md).

## Dig deeper

**Architecture (diagrams + components):** [`_docs/ARCHITECTURE.md`](_docs/ARCHITECTURE.md)

| Doc | Why |
|-----|-----|
| [`_docs/OPERATOR.md`](_docs/OPERATOR.md) | Short operator runbook |
| [`_docs/ADR-2026-07-21_hermes-watchdog-lifecycle-manager.md`](_docs/ADR-2026-07-21_hermes-watchdog-lifecycle-manager.md) | Lifecycle ADR |
| [`_docs/IPC-CONTRACT-P3.md`](_docs/IPC-CONTRACT-P3.md) | P3 report-only IPC contract |
| [`_docs/`](_docs/) | Phase implementation logs |

```powershell
go test ./... -count=1
```

MIT — see [LICENSE](LICENSE).
