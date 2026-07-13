#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT="$(mktemp -d "${TMPDIR:-/tmp}/gandalf-changes-home-acceptance-XXXXXX")"
ROOT="$(cd "$ROOT" && pwd -P)"
GOENV_PATH="$(go env GOENV)"
GOCACHE_PATH="$(go env GOCACHE)"
GOMODCACHE_PATH="$(go env GOMODCACHE)"
GOPATH_PATH="$(go env GOPATH)"

if [[ "${GANDALF_KEEP_CHANGES_HOME:-}" != "1" ]]; then
	trap 'rm -rf "$ROOT"' EXIT
fi

export HOME="$ROOT/home"
export GANDALF_STORE="$ROOT/store"
export GANDALF_UPDATE_CHECK=0
export GOENV="$GOENV_PATH"
export GOCACHE="$GOCACHE_PATH"
export GOMODCACHE="$GOMODCACHE_PATH"
export GOPATH="$GOPATH_PATH"
mkdir -p "$HOME" "$GANDALF_STORE"

printf 'Changes-first Home acceptance\n'
printf 'repo=%s\n' "$REPO"
printf 'HOME=%s\n' "$HOME"
printf 'GANDALF_STORE=%s\n' "$GANDALF_STORE"

cd "$REPO"
go test -count=1 ./internal/tui -run '^(TestHomeBaselineActionCreatesBothSupportedBaselinesAndReturnsClean|TestCreateMissingBaselinesPreservesExistingAgentBaseline|TestChangesFirstHomeRollbackRescansToClean)$'

printf '\nChanges-first Home acceptance passed: missing baselines were captured without replacing existing baselines, drift routed through Review Changes, rollback rescanned clean, and narrow terminals retained primary actions.\n'
