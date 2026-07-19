# HermesDesktopwatchdog

Standalone **Go** watchdog for [Hermes Agent](https://github.com/zapabob/hermes-agent) on Windows.

It mutually monitors packaged **Hermes Desktop** (`Hermes.exe`) and the Desktop-spawned / prewarmed `hermes serve` backend.

**Not** a Hermes plugin, skill, MCP server, or cron job. Hermes Agent tools and sessions must **not** control this process.

Extracted from `zapabob/hermes-agent` (`scripts/windows/watchdog-go/`) as a public operator tool.

## Isolation

| Item | Detail |
|------|--------|
| Process | Separate from Hermes Python / Electron |
| State | `%LOCALAPPDATA%\HermesWatchdog\` (lock + status JSON) |
| Logs | `%HERMES_HOME%\logs\hermes-go-watchdog.log` |
| Mutating API | Requires `HERMES_WATCHDOG_ADMIN_TOKEN` (otherwise **403**) |
| Read API | `GET /health`, `GET /api/status` (localhost / optional Tailscale) |

## Requirements

- Windows 10/11
- [Go](https://go.dev/dl/) 1.23+
- A Hermes checkout (for `hermes serve` / packaged Desktop paths)

## Build

```powershell
cd HermesDesktopwatchdog
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\Build-HermesGoWatchdog.ps1
```

Output: `dist\hermes-watchdog.exe`

## Start

```powershell
$env:HERMES_WATCHDOG_ADMIN_TOKEN = "<operator-secret>"
# optional Tailscale tsnet:
# $env:HERMES_WATCHDOG_TS_AUTHKEY = "<ts-authkey>"

powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\Start-HermesGoWatchdog.ps1 `
  -HermesRoot "C:\path\to\hermes-agent" `
  -BuildIfMissing
```

If `-HermesRoot` is omitted, the launcher looks for a sibling `..\hermes-agent` folder.

### Common flags (via Start script)

| Flag | Default | Meaning |
|------|---------|---------|
| `-IntervalSec` | 20 | Poll interval |
| `-FailThreshold` | 2 | Consecutive backend failures before Desktop restart |
| `-Once` | off | Single cycle then exit |
| `-NoTsnet` | off | Force tsnet off |
| `-Listen` | `127.0.0.1:9920` | Local HTTP listen |

### Extra exe flags

| Flag | Default | Meaning |
|------|---------|---------|
| `-prewarm-backend` | on | Prewarm / supervise managed `hermes serve` |
| `-managed-backend-port` | 9118 | Fixed manage port (not 9120/8787/9119) |
| `-backend-start-timeout` | 120 | Seconds waiting for process start |
| `-backend-ready-timeout` | 45 | Seconds waiting for `/api/status` |

## HTTP API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | none | Liveness |
| GET | `/api/status` | none | Watchdog status JSON |
| POST | `/api/v1/pause` | Admin | Pause monitoring |
| POST | `/api/v1/resume` | Admin | Resume |
| POST | `/api/v1/cycle` | Admin | Run one cycle now |
| POST | `/api/v1/stop` | Admin | Graceful stop |

Admin: `Authorization: Bearer <token>` or `X-Admin-Token: <token>`.

## Watch loop (summary)

1. **Prewarm** — start managed `hermes serve` on a fixed port and write `%LOCALAPPDATA%\HermesWatchdog\desktop-backend.json`
2. Desktop missing → launch packaged `Hermes.exe` (inject remote env from manifest when present)
3. Desktop up + backend down → recover managed serve before restarting Electron
4. Failures ≥ threshold → force Desktop restart
5. Reserved ops ports (8787, 9120, …) are never treated as Desktop backends

## Tailscale (optional)

Set `HERMES_WATCHDOG_TS_AUTHKEY` or `TS_AUTHKEY` (never commit secrets). With tsnet enabled the binary can advertise on the tailnet.

## Related

- Hermes fork: https://github.com/zapabob/hermes-agent
- Upstream Hermes: https://github.com/NousResearch/hermes-agent

## License

MIT — see [LICENSE](LICENSE).
