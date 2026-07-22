# Operator runbook (short)

Deep architecture: [`ARCHITECTURE.md`](ARCHITECTURE.md). Security: [`../SECURITY.md`](../SECURITY.md).

## Build & start

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\Build-HermesGoWatchdog.ps1

$env:HERMES_WATCHDOG_ADMIN_TOKEN = "your-secure-operator-token"
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\Start-HermesGoWatchdog.ps1
```

Optional: `-HermesRoot <path-to-hermes-agent>`, `-ForceRestart` to replace a running instance.

## Quick checks

```powershell
Invoke-RestMethod http://127.0.0.1:9920/health
Invoke-RestMethod http://127.0.0.1:9920/api/status
```

Useful status fields: `soleRestartAuthority`, `reportOnlyContract`, `desktopService.state`, `backendService.state`, `restart.failed`, `leases`, `recentAnomalies`, `ipcPipe`, `warmStart`, `updateSuppress`, `jobObject`, `recovery`.

## Common ops

| Goal | How |
|------|-----|
| Pause auto-recovery | `POST /api/v1/pause` + admin token |
| Resume (clears Failed path for recovery) | `POST /api/v1/resume` |
| One recovery cycle | `POST /api/v1/cycle` |
| Suppress restart during update (P6) | `POST /api/v1/update-suppress` body `{"suppress":true,"ttlSec":600}` or set env / write `update.lock` |
| Clear update suppress | `POST /api/v1/update-suppress` `{"suppress":false,"clearFile":true}` |
| Warm restart backend (P4) | `POST /api/v1/command` allowlisted `warm_restart` |
| Stop watchdog process | `POST /api/v1/stop` |
| Allowlisted command | `POST /api/v1/command` with nonce + allowlist |

Header: `Authorization: Bearer <token>` or `X-Admin-Token: <token>`.

## Where files live

| What | Where |
|------|-------|
| Binary | `dist\hermes-watchdog.exe` (gitignored) |
| Lock / state / events / update.lock | `%LOCALAPPDATA%\HermesWatchdog\` |
| Log | `%USERPROFILE%\.hermes\logs\hermes-go-watchdog.log` |
| Pipe | `\\.\pipe\hermes-watchdog` |

## Do not

- Register this binary as a Hermes plugin/MCP/skill/cron
- Point the watchdog at reserved ops ports as “backends”
- Commit `.env`, tokens, or `dist\*.exe`
- Expect Desktop/Backend to restart each other (report-only contract)

Desktop adapter contract (for hermes-agent maintainers): [`IPC-CONTRACT-P3.md`](IPC-CONTRACT-P3.md). Warm-start: [`WARM-START-CONTRACT.md`](WARM-START-CONTRACT.md).
