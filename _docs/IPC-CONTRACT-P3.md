# P3 IPC Adapter Contract (Report-Only)

**Audience:** Hermes Desktop / Backend maintainers integrating with Hermes Desktop Watchdog  
**Phase:** ADR P3  
**Authority:** Watchdog is the sole restart authority. Desktop and Backend **report only**.

## Rules

1. Desktop must **not** kill, restart, or respawn `hermes serve` / managed backend.
2. Backend must **not** kill or relaunch Desktop.
3. Both may send `anomaly_report` and `heartbeat`.
4. `command_request` is **operator/admin only** (HTTP admin token + nonce). Named Pipe connections are treated as service-level and cannot execute commands.
5. Free-form command lines are always rejected. Only allowlisted `CommandType` values exist.

## Allowlisted commands (operator only)

| Command | Meaning |
|---------|---------|
| `start_desktop` | Launch packaged Hermes.exe |
| `stop_desktop` | Stop Desktop process trees |
| `start_backend` | Ensure managed serve |
| `stop_backend` | Stop managed serve |
| `warm_restart` | Desktop last-resort restart (full drain = P4) |

## Transports

| Priority | Transport | Path |
|----------|-----------|------|
| Primary (Windows) | Named Pipe | `\\.\pipe\hermes-watchdog` |
| Fallback | HTTP loopback | `POST http://127.0.0.1:9920/api/v1/report` / `/api/v1/ipc` |

Named Pipe: one JSON envelope per connection (newline-terminated), one JSON `IPCResult` response.

## Envelope

```json
{
  "protocol_version": 1,
  "message_type": "anomaly_report",
  "instance_id": "optional-uuid",
  "epoch": 0,
  "sent_at_mono_ms": 0,
  "nonce": "",
  "payload": {}
}
```

### anomaly_report payload

```json
{
  "service": "hermes-desktop",
  "severity": "error",
  "code": "backend_unhealthy",
  "detail": "ready probe failed",
  "suggest_command": "start_backend",
  "pid": 1234
}
```

`suggest_command` is advisory only and is **never** auto-executed.

### Dual report (T12)

If Desktop and Backend report the same `code` within `--anomaly-merge-sec` (default 5s), Watchdog marks the reports `merged=true` and takes a **single** recovery decision.

### command_request (admin HTTP only)

```json
{
  "protocol_version": 1,
  "message_type": "command_request",
  "nonce": "unique-per-request",
  "payload": {
    "command": "start_backend",
    "reason": "operator recovery",
    "requester": "operator"
  }
}
```

Replay of `nonce` is rejected.

## Desktop adapter checklist

- [ ] Remove any code path that `taskkill`s / respawns backend independently
- [ ] On backend health failure → `anomaly_report` (or heartbeat state) only
- [ ] Prefer Named Pipe; fall back to HTTP `/api/v1/report`
- [ ] Do not call `/api/v1/command` from Desktop process
- [ ] Read `/api/status` `reportOnlyContract` / `soleRestartAuthority` / `leases` / `recentAnomalies`

## Auth

| Endpoint | Auth |
|----------|------|
| `/api/v1/report`, `/api/v1/heartbeat`, `/api/v1/ipc` (non-command) | Heartbeat/admin token, or loopback if no tokens set |
| `/api/v1/command`, `/api/v1/ipc` (`command_request`) | Admin token required |
| Named Pipe | Local machine; service role (no command execution) |
