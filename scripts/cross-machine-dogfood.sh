#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GANDALF="$REPO/bin/gandalf"
LINUX_GANDALF="$REPO/bin/gandalf-linux-amd64"

run() {
	"$@"
}

if [[ ! -x "$GANDALF" ]]; then
	mkdir -p "$REPO/bin"
	(cd "$REPO" && go build -o "$GANDALF" ./cmd/gandalf)
fi

if [[ ! -x "$LINUX_GANDALF" ]]; then
	(cd "$REPO" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$LINUX_GANDALF" ./cmd/gandalf)
fi

if ! docker version --format '{{.Server.Version}}' >/dev/null 2>&1; then
	printf 'Docker is required for cross-machine dogfood (Linux import container), but Docker is not available/running.\n' >&2
	exit 1
fi

ROOT="$(mktemp -d "${TMPDIR:-/tmp}/gandalf-cross-machine-XXXXXX")"
if [[ "${GANDALF_KEEP_DOGFOOD:-}" != "1" ]]; then
	trap 'rm -rf "$ROOT"' EXIT
fi

MAC_PROJECT="$ROOT/mac-project"
MAC_HOME="$ROOT/mac-home"
MAC_STORE="$ROOT/mac-store"
OUT="$ROOT/mac-export.gandalf"
IMPORT_JSON="$ROOT/import.json"

mkdir -p "$MAC_PROJECT" "$MAC_HOME/.claude" "$MAC_HOME/.local/bin"
printf '{"permissions":{"allow":["Bash(make test)"]}}\n' >"$MAC_HOME/.claude/settings.json"
printf '#!/bin/sh\nexit 0\n' >"$MAC_HOME/.local/bin/private-mcp"
chmod +x "$MAC_HOME/.local/bin/private-mcp"

cat >"$MAC_PROJECT/.mcp.json" <<EOF
{
  "mcpServers": {
    "github": {
      "transport": "stdio",
      "command": "gh",
      "args": ["mcp", "server"]
    },
    "local": {
      "transport": "stdio",
      "command": "$MAC_HOME/.local/bin/private-mcp"
    }
  }
}
EOF

HOME="$MAC_HOME" GANDALF_STORE="$MAC_STORE" run "$GANDALF" snapshot create --name mac-baseline --metadata-only --project "$MAC_PROJECT"
HOME="$MAC_HOME" GANDALF_STORE="$MAC_STORE" run "$GANDALF" bundle export --name mac-baseline --out "$OUT" --project "$MAC_PROJECT"

docker run --rm \
	-v "$REPO:/repo:ro" \
	-v "$ROOT:/work" \
	debian:bookworm-slim \
	bash -lc 'set -euo pipefail && mkdir -p /home/gandalf /linux/project /linux/store && HOME=/home/gandalf GANDALF_STORE=/linux/store /repo/bin/gandalf-linux-amd64 bundle import /work/mac-export.gandalf --dry-run --project /linux/project --json > /work/import.json'

if grep -q '"contentApplied"[[:space:]]*:[[:space:]]*true' "$IMPORT_JSON"; then
	printf 'Linux import was not a safe dry-run.\n' >&2
	exit 1
fi
if ! grep -q '"sourcePlatform"[[:space:]]*:[[:space:]]*"darwin"' "$IMPORT_JSON"; then
	printf 'Expected sourcePlatform=darwin in Linux dry-run machine diff.\n' >&2
	exit 1
fi
if ! grep -q '"targetPlatform"[[:space:]]*:[[:space:]]*"linux"' "$IMPORT_JSON"; then
	printf 'Expected targetPlatform=linux in Linux dry-run machine diff.\n' >&2
	exit 1
fi
if ! grep -q '"crossOS"[[:space:]]*:[[:space:]]*true' "$IMPORT_JSON"; then
	printf 'Expected crossOS=true in Linux dry-run machine diff.\n' >&2
	exit 1
fi
if ! grep -q '"binaryKind"[[:space:]]*:[[:space:]]*"source_local_path"' "$IMPORT_JSON"; then
	printf 'Expected source-local MCP binary mismatch warning in Linux dry-run.\n' >&2
	exit 1
fi

printf 'Cross-machine dogfood passed: macOS export dry-ran successfully inside Linux container.\n'
printf 'Bundle: %s\n' "$OUT"
