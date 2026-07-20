# gorilla/csrf drop-in (CVE-2025-47909)

This directory is a local `replace` target for `github.com/gorilla/csrf`.

Upstream `github.com/gorilla/csrf` has **no patched release** for
[CVE-2025-47909](https://github.com/advisories/GHSA-82ff-hg59-8x73)
(`TrustedOrigins` scheme bypass). Go security guidance is to migrate to
`filippo.io/csrf` / `filippo.io/csrf/gorilla` (or Go 1.25+
`net/http.CrossOriginProtection`).

Sources here are adapted from `filippo.io/csrf/gorilla` **v0.2.1** so the
existing import path `github.com/gorilla/csrf` (pulled transitively by
`tailscale.com/client/web`) resolves to a non-vulnerable implementation
without changing Tailscale or watchdog feature behavior.
