#!/usr/bin/env bash
# Capture verification artifacts for code-review patch acceptance.
# Usage: scripts/verify-review-patches.sh [SCRATCH_DIR]
#        GANDALF_SCRATCH_DIR is used when SCRATCH_DIR is omitted.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

SCRATCH="${1:-${GANDALF_SCRATCH_DIR:-}}"
if [[ -z "$SCRATCH" ]]; then
  echo "usage: $0 <scratch-dir>  (or set GANDALF_SCRATCH_DIR)" >&2
  exit 2
fi
mkdir -p "$SCRATCH"
export GANDALF_SCRATCH_DIR="$SCRATCH"

GANDALF_BIN="${GANDALF_BIN:-$REPO_ROOT/bin/gandalf}"
FAILED=0

log() { printf '==> %s\n' "$*"; }
require_file() {
  local path="$1"
  local needle="${2:-}"
  if [[ ! -s "$path" ]]; then
    echo "CHECK FAIL: missing or empty: $path" >&2
    FAILED=1
    return
  fi
  if [[ -n "$needle" ]] && ! grep -F -- "$needle" "$path" >/dev/null; then
    echo "CHECK FAIL: $path does not contain: $needle" >&2
    FAILED=1
    return
  fi
  echo "CHECK OK: $path"
}

log "build Go gandalf"
mkdir -p bin
go build -o "$GANDALF_BIN" ./cmd/gandalf

# --- Step 1: restore tests (twice) + Go summary ---
log "step 1: restore tests x2"
go test ./internal/gandalfcore/restore >"$SCRATCH/restore-test-1.log" 2>&1 &
RESTORE_1_PID=$!
go test ./internal/gandalfcore/restore >"$SCRATCH/restore-test-2.log" 2>&1 &
RESTORE_2_PID=$!

if ! wait "$RESTORE_1_PID"; then
  echo "CHECK FAIL: restore test run 1 failed" >&2
  FAILED=1
fi
if ! wait "$RESTORE_2_PID"; then
  echo "CHECK FAIL: restore test run 2 failed" >&2
  FAILED=1
fi

# --- Step 2: CLI restore help/dry-run + bundle verify ---
log "step 2: CLI restore dry-run and bundle verify"
CLI_SANDBOX="$SCRATCH/cli-restore-sandbox"
mkdir -p "$CLI_SANDBOX/project" "$CLI_SANDBOX/home" "$CLI_SANDBOX/store"
printf '%s\n' '{"mcpServers":{"github":{"command":"gh-baseline"}}}' >"$CLI_SANDBOX/project/.mcp.json"
"$GANDALF_BIN" snapshot create --name baseline --metadata-only \
  --project "$CLI_SANDBOX/project" \
  --home "$CLI_SANDBOX/home" \
  --store "$CLI_SANDBOX/store" >/dev/null
printf '%s\n' '{"mcpServers":{"github":{"command":"gh-changed"}}}' >"$CLI_SANDBOX/project/.mcp.json"

{
  echo "=== gandalf restore --help ==="
  "$GANDALF_BIN" restore --help
  echo
  echo "=== gandalf restore --snapshot baseline --dry-run (MCP sandbox) ==="
  "$GANDALF_BIN" restore --snapshot baseline --dry-run \
    --project "$CLI_SANDBOX/project" \
    --home "$CLI_SANDBOX/home" \
    --store "$CLI_SANDBOX/store"
  echo
  echo "restore_dry_run_exit=0"
} >"$SCRATCH/cli-restore-dryrun.out" 2>&1

BUNDLE_SANDBOX="$SCRATCH/bundle-sandbox"
mkdir -p "$BUNDLE_SANDBOX/project" "$BUNDLE_SANDBOX/home" "$BUNDLE_SANDBOX/store"
printf '%s\n' '{"mcpServers":{"github":{"command":"gh"}}}' >"$BUNDLE_SANDBOX/project/.mcp.json"
"$GANDALF_BIN" snapshot create --name verify-test --metadata-only \
  --project "$BUNDLE_SANDBOX/project" \
  --home "$BUNDLE_SANDBOX/home" \
  --store "$BUNDLE_SANDBOX/store" >/dev/null
"$GANDALF_BIN" bundle export --name verify-test --out "$SCRATCH/valid.gandalf" --metadata-only \
  --project "$BUNDLE_SANDBOX/project" \
  --home "$BUNDLE_SANDBOX/home" \
  --store "$BUNDLE_SANDBOX/store" >/dev/null

{
  echo "=== bundle verify valid.gandalf ==="
  set +e
  "$GANDALF_BIN" bundle verify "$SCRATCH/valid.gandalf"
  echo "exit_code=$?"
  set -e
} >"$SCRATCH/bundle-verify-valid.out" 2>&1

{
  echo "=== bundle verify missing.gandalf ==="
  set +e
  "$GANDALF_BIN" bundle verify "$SCRATCH/missing.gandalf"
  echo "exit_code=$?"
  set -e
} >"$SCRATCH/bundle-verify-invalid.out" 2>&1

# --- Step 3: full workspace tests ---
log "step 3: full Go tests"
set +e
go test ./... >"$SCRATCH/go-tests-full.log" 2>&1
GO_TEST_EXIT=$?
set -e
tail -30 "$SCRATCH/go-tests-full.log" >"$SCRATCH/go-tests.log"
grep -E 'github.com/qyinm/gandalf/internal/gandalfcore' "$SCRATCH/go-tests-full.log" >"$SCRATCH/gandalfcore-summary-full.log" || true
tail -30 "$SCRATCH/gandalfcore-summary-full.log" >"$SCRATCH/gandalfcore-summary.log"
echo "go_test_exit=$GO_TEST_EXIT" >>"$SCRATCH/go-tests.log"
if [[ "$GO_TEST_EXIT" -ne 0 ]]; then
  echo "CHECK FAIL: go test ./... exited $GO_TEST_EXIT" >&2
  FAILED=1
fi

# --- Step 4: review commits log ---
log "step 4: review commits"
git log --oneline -12 >"$SCRATCH/review-commits.log"

# --- Checklist ---
log "artifact checklist"
require_file "$SCRATCH/restore-test-1.log" "ok"
require_file "$SCRATCH/restore-test-2.log" "ok"
require_file "$SCRATCH/gandalfcore-summary.log"
require_file "$SCRATCH/cli-restore-dryrun.out" "--help"
require_file "$SCRATCH/cli-restore-dryrun.out" "gandalf restore dry-run"
require_file "$SCRATCH/cli-restore-dryrun.out" "mcp_server"
if grep -F -- "No apply handler" "$SCRATCH/cli-restore-dryrun.out" >/dev/null; then
  echo "CHECK FAIL: cli-restore-dryrun.out contains 'No apply handler'" >&2
  FAILED=1
else
  echo "CHECK OK: no handler errors in cli-restore-dryrun.out"
fi
require_file "$SCRATCH/bundle-verify-valid.out" "Status: valid"
require_file "$SCRATCH/bundle-verify-valid.out" "exit_code=0"
require_file "$SCRATCH/bundle-verify-invalid.out" "GANDALF_BUNDLE_VERIFY_FAILED"
require_file "$SCRATCH/bundle-verify-invalid.out" "exit_code=1"
require_file "$SCRATCH/review-commits.log"
require_file "$SCRATCH/go-tests.log" "go_test_exit=0"

if [[ "$FAILED" -ne 0 ]]; then
  echo "verification FAILED -- see $SCRATCH" >&2
  exit 1
fi

echo "verification PASSED -- artifacts in $SCRATCH"
exit 0
