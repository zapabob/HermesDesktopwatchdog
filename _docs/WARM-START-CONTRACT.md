# Warm-Start Contract (P4) — Watchdog ↔ Hermes

**Status:** Watchdog-owned surfaces shipped (2026-07-22). Hermes drain/checkpoint/session-restore adapters are **not** in this repo.

**Audience:** Future hermes-agent Desktop/Backend maintainers.

## Goal

Reconstruct durable state after backend restart. Warm start does **not** restore process memory.

## Watchdog sequence (12-step intent)

| Step | Watchdog action | Hermes expectation (future) |
|------|-----------------|----------------------------|
| 1 | Bump epoch / restart intent event | Reject stale heartbeats with old epoch |
| 2–3 | `warmStart.draining=true`, `resumeTraffic=false` on `/api/status` | Stop accepting new runs |
| 4 | Wait `WarmDrainTimeout` (default 15s) watching `active_runs` from heartbeat | Drain in-flight runs |
| 5 | Emit `checkpoint_request` event; wait `WarmCheckpointWait` | Ack checkpoint to state.db (optional HTTP/IPC later) |
| 6 | Signal close children | Close MCP transports |
| 7 | Stop managed backend (Job Object terminate preferred) | Exit cleanly if possible |
| 8 | Start managed backend | Boot with durable state |
| 9 | Readiness probe | Expose `/ready` |
| 10 | Emit `session_routing_restore_signal` | Restore session routing from durable state |
| 11 | Republish `desktop-backend.json` + `desktop_backend_ready` event | Desktop reconnects |
| 12 | `resumeTraffic=true` | Accept traffic |

## Interrupted vs success (normative)

- If `active_runs > 0` at drain deadline → outcome **`interrupted`** (never `success`).
- If checkpoint ack missing → proceed with force-stop; if runs remained, stay **`interrupted`**.
- `success` only when drain completed with zero active runs (or no runs reported) and backend became ready.

## Watchdog APIs / config

- Allowlisted command: `warm_restart` → runs sequencer (not Desktop tree-kill).
- Flags: `-warm-drain-sec`, `-warm-checkpoint-sec`.
- Status: `warmStart` object on `/api/status`.

## Hermes gaps (explicit)

- No drain HTTP notify hook consumed by Hermes yet.
- No checkpoint ack channel from Hermes.
- Session restore is signal-only.
- Desktop renderer recreate is P5 policy stub (no Electron IPC here).

See also: [ADR](ADR-2026-07-21_hermes-watchdog-lifecycle-manager.md), [ARCHITECTURE.md](ARCHITECTURE.md).
