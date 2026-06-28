#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GANDALF="$REPO/bin/gandalf"

if [[ ! -x "$GANDALF" ]]; then
	mkdir -p "$REPO/bin"
	(cd "$REPO" && go build -o "$GANDALF" ./cmd/gandalf)
fi

ROOT="$(mktemp -d "${TMPDIR:-/tmp}/gandalf-gate2-acceptance-XXXXXX")"
if [[ "${GANDALF_KEEP_GATE2_ACCEPTANCE:-}" != "1" ]]; then
	trap 'rm -rf "$ROOT"' EXIT
fi

PROJECT="$ROOT/project"
HOME_DIR="$ROOT/home"
STORE="$ROOT/store"
CODEX_DIR="$HOME_DIR/.codex"
CONFIG_PATH="$CODEX_DIR/config.toml"
ORIGINAL_CONFIG="$ROOT/original-config.toml"

mkdir -p "$PROJECT" "$CODEX_DIR"

cat >"$ORIGINAL_CONFIG" <<'EOF'
model = "gpt-5"
approval_policy = "on-request"

[mcp_servers.github]
command = "gh"
args = ["mcp", "server"]
EOF
cp "$ORIGINAL_CONFIG" "$CONFIG_PATH"
printf 'Disposable Gate 2 acceptance project.\n' >"$PROJECT/README.md"

run_gandalf() {
	printf '\n$ gandalf %s\n' "$*"
	HOME="$HOME_DIR" GANDALF_STORE="$STORE" GANDALF_UPDATE_CHECK=0 "$GANDALF" "$@"
}

printf 'Gate 2 deterministic Codex rollback acceptance\n'
printf 'HOME=%s\n' "$HOME_DIR"
printf 'GANDALF_STORE=%s\n' "$STORE"
printf 'project=%s\n' "$PROJECT"
printf 'binary=%s\n' "$GANDALF"

cd "$PROJECT"
run_gandalf snapshot create --name clean-codex --agent codex --scope user --project "$PROJECT"

: >"$CONFIG_PATH"
ADDED_SKILL="$CODEX_DIR/skills/synthetic-harness/SKILL.md"
mkdir -p "$(dirname "$ADDED_SKILL")"
cat >"$ADDED_SKILL" <<'EOF'
---
name: synthetic-harness
---
Adds a disposable acceptance skill.
EOF
printf '\n# Synthetic harness install zero-filled config.toml and added a Codex skill.\n'

run_gandalf diff clean-codex current --agent codex --scope user --project "$PROJECT"
run_gandalf restore --snapshot clean-codex --dry-run --agent codex --scope user --project "$PROJECT"
run_gandalf restore --snapshot clean-codex --apply --experimental --agent codex --scope user --project "$PROJECT"

cmp "$ORIGINAL_CONFIG" "$CONFIG_PATH"
if [[ -e "$ADDED_SKILL" ]]; then
	printf 'Expected synthetic skill to be removed, but it still exists: %s\n' "$ADDED_SKILL" >&2
	exit 1
fi

printf '\nGate 2 acceptance passed: config restored and synthetic skill removed.\n'
