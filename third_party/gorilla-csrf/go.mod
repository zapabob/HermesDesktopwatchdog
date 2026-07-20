// Local drop-in replacement for github.com/gorilla/csrf.
// Sourced from filippo.io/csrf/gorilla v0.2.1 to remediate CVE-2025-47909
// (no upstream patched release exists for github.com/gorilla/csrf).
module github.com/gorilla/csrf

go 1.24.0

require filippo.io/csrf v0.2.1
