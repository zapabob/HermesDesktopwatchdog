# Hermes Desktop Watchdog

**Canonical home** for the Windows Go lifecycle manager formerly under `hermes-agent/scripts/windows/watchdog-go`.

Operator-only process that keeps Hermes Desktop (`Hermes.exe`) and managed `hermes serve` alive — sole restart authority, health/heartbeat, report-only IPC, warm-start, Job Objects, update suppress.

**Not** a Hermes plugin, MCP, skill, cron, or official NousResearch component. Do **not** register it in the agent tool surface.

## Site & download

- **Pages:** https://zapabob.github.io/HermesDesktopwatchdog/
- **Release v1.3.0:**
  - [Windows amd64 (exe)](https://github.com/zapabob/HermesDesktopwatchdog/releases/download/v1.3.0/hermes-watchdog-windows-amd64-v1.3.0.tar.gz)
  - [Linux amd64 (source + stubs)](https://github.com/zapabob/HermesDesktopwatchdog/releases/download/v1.3.0/hermes-watchdog-linux-amd64-v1.3.0.tar.gz)
  - [macOS arm64 (source + stubs)](https://github.com/zapabob/HermesDesktopwatchdog/releases/download/v1.3.0/hermes-watchdog-darwin-arm64-v1.3.0.tar.gz)
  - [Full source](https://github.com/zapabob/HermesDesktopwatchdog/releases/download/v1.3.0/hermes-watchdog-src-v1.3.0.tar.gz)

Primary runtime is **Windows**. Linux/macOS archives ship source and stub builds for compile/smoke only — not full Desktop supervision.

## 30-second start

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\Build-HermesGoWatchdog.ps1

$env:HERMES_WATCHDOG_ADMIN_TOKEN = "your-secure-operator-token"
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\Start-HermesGoWatchdog.ps1 -HermesRoot "C:\path\to\hermes-agent"
```

Binary → `dist\hermes-watchdog.exe` (gitignored). Listen → `127.0.0.1:9920`.
Managed `hermes serve` → **`127.0.0.1:9119`** (post-merge Desktop contract; not 9118).
Publish `desktop-backend.json` with **both** `baseUrl`/`url` and `token`, or clear remotes on Desktop launch — never URL-only.

### Safe watchdog hot-swap

Build a side-by-side candidate, stop only the watchdog, replace its executable,
then require `/health` and `/api/status` before accepting it. If either probe
fails, the retained binary is restored. Hermes Desktop and the managed backend
are not stopped by this flow.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\Start-HermesGoWatchdog.ps1 -HotSwap -RunBuildTests
```

For a separately installed runtime, add `-RuntimeExe <live hermes-watchdog.exe>`.

## What shipped (P1–P6)

| Phase | Capability |
|-------|------------|
| P1 | `ServiceState` + crash-loop `RestartPolicy` (sole restart authority) |
| P2 | `/live` `/ready` + heartbeat ingest (`instance_id` / `epoch`) |
| P3 | Report-only IPC (Named Pipe + HTTP); Desktop/Backend cannot restart peers |
| P4 | Warm-start sequencer (`interrupted` ≠ success) |
| P5 | Windows Job Object for managed backend tree |
| P6 | Update suppress (env / `update.lock` / admin API) |

Plus migrations from hermes-agent watchdog-go: `serve --skip-build`, session-token auth drift detection & port replace, netstat listener fallback, bounded `taskkill`, async prewarm, and Start/Build timeout flags.

The watchdog publishes `desktop-backend.json` by replacing a complete temporary
file, so Desktop never reads partial JSON. A failed WMI Desktop probe is reported
as `unknown`; it is not treated as evidence that Desktop stopped.

### v1.3.0 — session continuity

| Change | Why |
|--------|-----|
| Session token reused across managed serve restarts | A live Desktop keeps the launch-time `HERMES_DESKTOP_REMOTE_TOKEN`; re-minting on every backend flap made gated APIs 401 until relaunch |
| Kill-and-confirm port clear before replacement serve | A replacement racing a wedged occupant produced token-drift loops |
| `netstat` bounded by timeout | An unbounded enumeration could stall the whole probe loop |
| Orphan reap skipped while the managed port is healthy | Tree-killing a non-listening wrapper took its listening child down (Desktop ECONNRESET → relaunch loop) |
| `desktop-backend.json` accepts `url` alias and numeric `updatedAt` | Desktop and operator tooling also write this file; a strict schema made it unreadable |
| Ensure sequence serialized on its own mutex | Keeps `/api/status` responsive during a multi-minute cold start |

## Safety

Loopback by default. Mutations need `HERMES_WATCHDOG_ADMIN_TOKEN`. See [SECURITY.md](SECURITY.md).

## Docs

| Doc | Why |
|-----|-----|
| [`_docs/ARCHITECTURE.md`](_docs/ARCHITECTURE.md) | Diagrams + components |
| [`_docs/OPERATOR.md`](_docs/OPERATOR.md) | Operator runbook |
| [`_docs/ADR-2026-07-21_hermes-watchdog-lifecycle-manager.md`](_docs/ADR-2026-07-21_hermes-watchdog-lifecycle-manager.md) | Lifecycle ADR |
| [`_docs/IPC-CONTRACT-P3.md`](_docs/IPC-CONTRACT-P3.md) | Report-only IPC |
| [`_docs/WARM-START-CONTRACT.md`](_docs/WARM-START-CONTRACT.md) | Warm-start contract |

```powershell
go test ./... -count=1 -p 1
```

MIT — [LICENSE](LICENSE). Repo: [zapabob/HermesDesktopwatchdog](https://github.com/zapabob/HermesDesktopwatchdog).
