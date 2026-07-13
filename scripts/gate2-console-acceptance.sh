#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

printf 'Gate 2 Unified Agent Setup Console acceptance\n'
printf 'repo=%s\n' "$REPO"

cd "$REPO"

go test -count=1 ./internal/gandalfcore/setup ./internal/tui

printf '\nGate 2 setup-console acceptance passed: setup inventory, marketplace/source rows, unavailable reasons, reviewed setup actions, stale-review rejection, and TUI rendering tests passed.\n'
