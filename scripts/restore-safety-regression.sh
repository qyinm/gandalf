#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GANDALF="$REPO/bin/gandalf"

if [[ ! -x "$GANDALF" ]]; then
	mkdir -p "$REPO/bin"
	(cd "$REPO" && go build -o "$GANDALF" ./cmd/gandalf)
fi

ROOT="$(mktemp -d "${TMPDIR:-/tmp}/gandalf-restore-safety-regression-XXXXXX")"
ROOT="$(cd "$ROOT" && pwd -P)"
if [[ "${GANDALF_KEEP_RESTORE_SAFETY:-}" != "1" ]]; then
	trap 'rm -rf "$ROOT"' EXIT
fi

PROJECT="$ROOT/project"
HOME_DIR="$ROOT/home"
STORE="$ROOT/store"
CODEX_DIR="$HOME_DIR/.codex"
CLAUDE_DIR="$HOME_DIR/.claude"
CONFIG_PATH="$CODEX_DIR/config.toml"
ORIGINAL_CONFIG="$ROOT/original-config.toml"
CLAUDE_SETTINGS="$CLAUDE_DIR/settings.json"
ORIGINAL_CLAUDE_SETTINGS="$ROOT/original-claude-settings.json"

mkdir -p "$PROJECT" "$CODEX_DIR" "$CLAUDE_DIR"

cat >"$ORIGINAL_CONFIG" <<'EOF'
model = "gpt-5"
approval_policy = "on-request"

[mcp_servers.github]
command = "gh"
args = ["mcp", "server"]
EOF
cp "$ORIGINAL_CONFIG" "$CONFIG_PATH"
cat >"$ORIGINAL_CLAUDE_SETTINGS" <<'EOF'
{
  "permissions": {
    "allow": [
      "Bash(echo hi)"
    ]
  }
}
EOF
cp "$ORIGINAL_CLAUDE_SETTINGS" "$CLAUDE_SETTINGS"
printf 'Disposable restore-safety regression project.\n' >"$PROJECT/README.md"

run_gandalf() {
	printf '\n$ gandalf %s\n' "$*"
	HOME="$HOME_DIR" GANDALF_STORE="$STORE" GANDALF_UPDATE_CHECK=0 "$GANDALF" "$@"
}

printf 'Restore-safety regression for Codex and Claude Code user-global setup\n'
printf 'HOME=%s\n' "$HOME_DIR"
printf 'GANDALF_STORE=%s\n' "$STORE"
printf 'project=%s\n' "$PROJECT"
printf 'binary=%s\n' "$GANDALF"

cd "$PROJECT"
run_gandalf snapshot create --name clean-codex --agent codex --scope user --project "$PROJECT"
run_gandalf snapshot create --name clean-claude --agent claude-code --scope user --project "$PROJECT"

: >"$CONFIG_PATH"
ADDED_SKILL="$CODEX_DIR/skills/synthetic-harness/SKILL.md"
mkdir -p "$(dirname "$ADDED_SKILL")"
cat >"$ADDED_SKILL" <<'EOF'
---
name: synthetic-harness
---
Adds a disposable acceptance skill.
EOF
printf '\n# Synthetic harness zero-filled config.toml and added a Codex skill.\n'

run_gandalf diff clean-codex current --agent codex --scope user --project "$PROJECT"
run_gandalf restore --snapshot clean-codex --dry-run --agent codex --scope user --project "$PROJECT"
run_gandalf restore --snapshot clean-codex --apply --experimental --agent codex --scope user --project "$PROJECT"

cmp "$ORIGINAL_CONFIG" "$CONFIG_PATH"
if [[ -e "$ADDED_SKILL" ]]; then
	printf 'Expected synthetic skill to be removed, but it still exists: %s\n' "$ADDED_SKILL" >&2
	exit 1
fi

cat >"$CLAUDE_SETTINGS" <<'EOF'
{
  "permissions": {
    "allow": [
      "Bash(npm install)"
    ]
  }
}
EOF
printf '\n# Synthetic harness changed Claude Code settings permissions.\n'

run_gandalf diff clean-claude current --agent claude-code --scope user --project "$PROJECT"
run_gandalf restore --snapshot clean-claude --dry-run --agent claude-code --scope user --project "$PROJECT"
run_gandalf restore --snapshot clean-claude --apply --experimental --agent claude-code --scope user --project "$PROJECT"

cmp "$ORIGINAL_CLAUDE_SETTINGS" "$CLAUDE_SETTINGS"

printf '\nRestore-safety regression passed: Codex config restored, synthetic skill removed, and Claude Code settings restored.\n'
