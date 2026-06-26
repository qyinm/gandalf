#!/usr/bin/env bash
# Capture all verification-plan artifacts for code-review patch acceptance.
# Usage: scripts/verify-review-patches.sh [SCRATCH_DIR]
#        HEM_SCRATCH_DIR is used when SCRATCH_DIR is omitted.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

SCRATCH="${1:-${HEM_SCRATCH_DIR:-}}"
if [[ -z "$SCRATCH" ]]; then
  echo "usage: $0 <scratch-dir>  (or set HEM_SCRATCH_DIR)" >&2
  exit 2
fi
mkdir -p "$SCRATCH"
export HEM_SCRATCH_DIR="$SCRATCH"

HEM_BIN="${HEM_BIN:-$REPO_ROOT/target/debug/hem}"
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

log "build hem-cli"
cargo build -p hem-cli -q

# --- Step 1: restore tests (twice) + hem-core summary ---
log "step 1: restore_test x2"
cargo test -p hem-core --test restore_test >"$SCRATCH/restore-test-1.log" 2>&1
cargo test -p hem-core --test restore_test >"$SCRATCH/restore-test-2.log" 2>&1
cargo test -p hem-core -- --test-threads=1 2>&1 | tail -20 >"$SCRATCH/hem-core-summary.log"

# --- Step 2: CLI restore help/dry-run + bundle verify ---
log "step 2: CLI restore dry-run and bundle verify"
CLI_SANDBOX="$SCRATCH/cli-restore-sandbox"
mkdir -p "$CLI_SANDBOX/project" "$CLI_SANDBOX/home" "$CLI_SANDBOX/store"
printf '%s\n' '{"mcpServers":{"github":{"command":"gh-baseline"}}}' >"$CLI_SANDBOX/project/.mcp.json"
"$HEM_BIN" snapshot create --name baseline --metadata-only \
  --project "$CLI_SANDBOX/project" \
  --home "$CLI_SANDBOX/home" \
  --store "$CLI_SANDBOX/store" >/dev/null
printf '%s\n' '{"mcpServers":{"github":{"command":"gh-changed"}}}' >"$CLI_SANDBOX/project/.mcp.json"

{
  echo "=== hem restore --help ==="
  "$HEM_BIN" restore --help
  echo
  echo "=== hem restore --snapshot baseline --dry-run (MCP sandbox) ==="
  "$HEM_BIN" restore --snapshot baseline --dry-run \
    --project "$CLI_SANDBOX/project" \
    --home "$CLI_SANDBOX/home" \
    --store "$CLI_SANDBOX/store"
  echo
  echo "restore_dry_run_exit=0"
} >"$SCRATCH/cli-restore-dryrun.out" 2>&1

BUNDLE_SANDBOX="$SCRATCH/bundle-sandbox"
mkdir -p "$BUNDLE_SANDBOX/project" "$BUNDLE_SANDBOX/home" "$BUNDLE_SANDBOX/store"
printf '%s\n' '{"mcpServers":{"github":{"command":"gh"}}}' >"$BUNDLE_SANDBOX/project/.mcp.json"
"$HEM_BIN" snapshot create --name verify-test --metadata-only \
  --project "$BUNDLE_SANDBOX/project" \
  --home "$BUNDLE_SANDBOX/home" \
  --store "$BUNDLE_SANDBOX/store" >/dev/null
"$HEM_BIN" bundle export --name verify-test --out "$SCRATCH/valid.hem" --metadata-only \
  --project "$BUNDLE_SANDBOX/project" \
  --home "$BUNDLE_SANDBOX/home" \
  --store "$BUNDLE_SANDBOX/store" >/dev/null

{
  echo "=== bundle verify valid.hem ==="
  set +e
  "$HEM_BIN" bundle verify "$SCRATCH/valid.hem"
  echo "exit_code=$?"
  set -e
} >"$SCRATCH/bundle-verify-valid.out" 2>&1

{
  echo "=== bundle verify missing.hem ==="
  set +e
  "$HEM_BIN" bundle verify "$SCRATCH/missing.hem"
  echo "exit_code=$?"
  set -e
} >"$SCRATCH/bundle-verify-invalid.out" 2>&1

# --- Step 3: restore pipeline evidence JSON ---
log "step 3: restore pipeline evidence"
cargo test -p hem-core --test restore_test restore_plan_pipeline_applies_mcp_permission_and_env_with_confinement \
  >>"$SCRATCH/restore-test-1.log" 2>&1

# --- Step 4: desktop home state evidence ---
log "step 4: desktop home state"
cargo check -p hem-desktop -q
cargo test -p hem-desktop --lib populates_current_snapshot_id_from_store >>"$SCRATCH/desktop-home-state.log" 2>&1

# --- Step 5: review commits log ---
log "step 5: review commits"
git log --oneline -12 >"$SCRATCH/review-commits.log"

# --- Step 6: workspace tests ---
log "step 6: workspace tests"
set +e
cargo test --workspace >"$SCRATCH/workspace-tests-full.log" 2>&1
WORKSPACE_EXIT=$?
set -e
tail -30 "$SCRATCH/workspace-tests-full.log" >"$SCRATCH/workspace-tests.log"
echo "workspace_exit=$WORKSPACE_EXIT" >>"$SCRATCH/workspace-tests.log"
if [[ "$WORKSPACE_EXIT" -ne 0 ]]; then
  echo "CHECK FAIL: cargo test --workspace exited $WORKSPACE_EXIT" >&2
  FAILED=1
fi

# --- Checklist ---
log "artifact checklist"
require_file "$SCRATCH/restore-test-1.log" "test result: ok"
require_file "$SCRATCH/restore-test-2.log" "test result: ok"
require_file "$SCRATCH/hem-core-summary.log"
require_file "$SCRATCH/cli-restore-dryrun.out" "--help"
require_file "$SCRATCH/cli-restore-dryrun.out" "hem restore dry-run"
require_file "$SCRATCH/cli-restore-dryrun.out" "mcp_server"
if grep -F -- "No apply handler" "$SCRATCH/cli-restore-dryrun.out" >/dev/null; then
  echo "CHECK FAIL: cli-restore-dryrun.out contains 'No apply handler'" >&2
  FAILED=1
else
  echo "CHECK OK: no handler errors in cli-restore-dryrun.out"
fi
require_file "$SCRATCH/bundle-verify-valid.out" "Bundle verification passed"
require_file "$SCRATCH/bundle-verify-valid.out" "exit_code=0"
require_file "$SCRATCH/bundle-verify-invalid.out" "HEM_BUNDLE_VERIFY_FAILED"
require_file "$SCRATCH/bundle-verify-invalid.out" "exit_code=1"
require_file "$SCRATCH/review-restore-apply.json"
require_file "$SCRATCH/desktop-home-state.json" "currentSnapshotId"
require_file "$SCRATCH/review-commits.log" "fix(review)"
require_file "$SCRATCH/workspace-tests.log" "test result:"

if [[ "$FAILED" -ne 0 ]]; then
  echo "verification FAILED — see $SCRATCH" >&2
  exit 1
fi

echo "verification PASSED — artifacts in $SCRATCH"
exit 0