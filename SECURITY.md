# Security Policy

Hermes Desktop Watchdog enforces a strict security boundary by design, functioning as an operator-level supervisor outside of the standard Hermes Agent runtime tree.

## Security Controls & Safety Model

- **Loopback Default:** By default, the HTTP listener binds exclusively to localhost (127.0.0.1:9920). It is not reachable from the external network unless Tailscale (	snet) is explicitly configured with a secure key.
- **Admin Token Authentication:** All mutating endpoints (/api/v1/pause, /api/v1/resume, /api/v1/cycle, /api/v1/stop) require a configured HERMES_WATCHDOG_ADMIN_TOKEN passed in the Authorization: Bearer <token> or X-Admin-Token header. If no token is set in the environment, all mutating calls return a 403 Forbidden error.
- **No Arbitrary Command Execution:** Unlike remote terminals or interpreters, the watchdog only supports executing highly specific predefined commands (launching Hermes.exe and spawning hermes serve via Python). It rejects and does not implement generic CLI execution paths.
- **Pinning Executable Paths:** The watchdog only launches binaries from resolved local repository paths or explicitly allowlisted directories.
- **Non-Inheritance of Host Secrets:** The watchdog environment does not pass general host shell environment secrets or .env parameters to child processes unless explicitly required for server authentication (HERMES_DASHBOARD_SESSION_TOKEN).
- **Port Conflict Safeguards:** Excludes binding or killing processes running on registered ops ports (like 9120 or 8787) to protect adjacent developer services from collision.
- **Single-Instance Locks:** Uses a robust PID lock file (watchdog.lock) to prevent duplicate instances from spawning and triggering restart loops.
- **Restart Thresholds:** Automatically halts restarts and enters a backoff StateFailed state when a continuous restart threshold is breached, preventing CPU exhaustion during permanent hardware/software faults.

## Reporting a Vulnerability

Please report any security vulnerabilities directly to the maintainer or open a private security draft on GitHub. We aim to review and respond to all reports within 48 hours.
