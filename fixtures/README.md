# 📦 hem — Sample Drift Fixtures

## 구조

```
fixtures/
  baseline/    ← 정상 상태 기준
    .mcp.json
    CLAUDE.md
    .claude/settings.json
    .cursor/mcp.json
    .env

  drifted/     ← 변경 후 상태 (drift)
    .mcp.json         ← MCP server github 추가
    CLAUDE.md         ← 프롬프트 변경 (biome 도입)
    .claude/settings.json  ← wildcard permission 추가
    .cursor/mcp.json  ← github MCP 추가
    .env              ← API key 추가
```

## 사용법

```bash
# 1. baseline 스냅샷 생성
hem snapshot create --name demo-baseline \
  --metadata-only --project fixtures/baseline

# 2. scan + explain
hem scan --project fixtures/drifted

# 3. diff
hem diff demo-baseline current --project fixtures/drifted

# audit
hem audit current --project fixtures/drifted

# 5. report
hem report current --project fixtures/drifted --out drift-report.md
```
