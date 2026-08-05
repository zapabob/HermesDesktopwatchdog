# Prefer explicit HermesRoot packaged Desktop exe

- Date: 2026-08-05
- Branch/worktree: main (local checkout HermesGowatchdog → origin HermesDesktopwatchdog)
- Agent: Cursor

## Overview
Port hermes-agent nested watchdog fix so `defaultPackagedExe` prefers `-hermes-root` packaged `Hermes.exe` over a possibly stale `LOCALAPPDATA` copy.

## Changed files
- `config.go` — reorder candidate preference; keep LOCALAPPDATA as fallback
- `config_test.go` — regression for explicit repo root preference

## Verification
- `go test ./... -count=1` (run after patch)

## Notes
- Source of the behavioral fix: hermes-agent commit `a2e4a6827c`
- Local folder name is `HermesGowatchdog`; GitHub remote remains `zapabob/HermesDesktopwatchdog`
