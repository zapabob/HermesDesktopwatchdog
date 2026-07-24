#!/usr/bin/env bash
# Setup / build helper for Hermes Desktop Watchdog (source distribution).
# Primary target is Windows. Linux/macOS builds use stubs — not full Desktop supervision.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "[1/3] go mod tidy"
go mod tidy

echo "[2/3] go test ./... (stubs on non-Windows)"
go test ./... -count=1 -p 1

OUT="${1:-dist/hermes-watchdog}"
mkdir -p "$(dirname "$OUT")"
echo "[3/3] go build -> $OUT"
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$OUT" .

echo "Built $OUT"
echo "NOTE: Full Hermes.exe / Job Object / Named Pipe supervision is Windows-only."
echo "      On Linux/macOS this binary starts HTTP APIs with process stubs only."
