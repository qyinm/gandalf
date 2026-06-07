**snaptailor** 제품 설명

### 한 줄 요약
**AI 코딩 에이전트 환경을 Docker image처럼 스냅샷으로 저장하고 Mac 사이에서 안전하게 복원하는 CLI**

### 상세 설명
snaptailor는 AI 코딩 에이전트(Claude Code, Codex, Cursor, Pi Agent, OpenCode)의
MCP 서버, skills, permissions, hooks, system prompt, env key 등 **모든 설정**을
하나의 `.stailor` 번들로 패키징합니다.

이 번들을 다른 Mac에서 import 하면 **동일한 에이전트 환경에 가깝게 복원**됩니다 —
마치 Docker image를 pull 해서 어디서든 같은 컨테이너를 띄우는 것처럼요.

중간에 읽기 전용 scan/diff/audit 파이프라인도 내장되어 있어,
설정이 어떻게 바뀌었는지, 왜 위험한지, 어디서 온 설정인지 추적할 수 있습니다.

### 신뢰 계약 (Trust Contract)
- 절대 MCP command/hook/script를 실행하지 않음
- 절대 네트워크에 접속하지 않음 (기본값)
- 절대 사용자 설정을 동의 없이 수정하지 않음 (restore는 명시적 --apply 플래그 필요)
- `.env` 값은 key name만 수집하고 raw secret 값은 번들/출력하지 않음
- 절대 symlink를 따라가지 않음
- 번들은 지원되는 파일 내용을 기본 포함하며 `--metadata-only`로 opt-out 가능
- doctor/preflight는 누락된 도구와 env key를 보고하지만 패키지 설치나 secret 복원은 하지 않음

### 관리하는 대상
- System Prompt (CLAUDE.md, AGENTS.md, CODE.md)
- MCP Servers (.mcp.json, .cursor/mcp.json, Claude Desktop config)
- Permission Rules & Hooks (settings.json)
- Skills (claude/skills, codex/skills, pi-agent/skills, opencode/skills)
- Env Key Inventory (.env)
- Pi Agent: settings, extensions, skills, themes, prompts, agents, models
- OpenCode: config, skills

### 핵심 기능

**진단 (읽기 전용)**
- **Scan** — 프로젝트의 모든 에이전트 설정을 자동 탐지하고 evidence inventory 생성
- **Audit** — 설정의 보안/위험 finding 자동 감지
- **Diff** — 두 시점 간 변경점 상세 비교 (semantic + raw diff)
- **Provenance** — 모든 설정의 출처 추적 (user vs project)
- **Report** — 사람이 읽을 수 있는 Markdown 리포트 export
- **Doctor** — Mac에서 복원 전 필요한 로컬 도구, MCP command, env key gap 확인

**재현 (쓰기)**
- **Bundle export** — 현재 에이전트 환경을 `.stailor` 번들로 패키징
- **Bundle import** — 다른 머신에서 번들을 import 하여 동일한 환경 복원
- **Restore** — 스냅샷 기반 dry-run plan + per-type apply + rollback
- **Snapshot** — metadata-only 또는 full-content 스냅샷 저장

### 컨셉
영국 Savile Row의 고급 양장점처럼,
**"당신의 AI 에이전트를 최고의 상태로 맞춤 재단한다"**는 철학.

snaptailor는 맞춤 재단의 세 가지 단계를 모두 수행합니다:
1. **측정** — 현재 상태를 정밀하게 스캔하고 기록
2. **본뜨기** — 완성된 패턴을 `.stailor` 번들로 저장
3. **재현** — 어느 머신에서든 동일한 맞춤 환경으로 복원

### 핵심 워크플로우

```bash
# 머신 A: 내가 세팅한 에이전트 환경을 번들로 저장
snaptailor bundle export --name my-setup --out my-setup.stailor --project .

# 머신 B: readiness 확인 후 복원
snaptailor doctor --project .
snaptailor bundle import my-setup.stailor --dry-run --project .
snaptailor bundle import my-setup.stailor --apply-content --quarantine --experimental --project .
```

```bash
# 평소에는 읽기 전용으로 변화 감시
snaptailor scan --project .
snaptailor snapshot create --name baseline --metadata-only --project .
# ... 시간이 흐른 뒤 ...
snaptailor diff baseline current --project .
snaptailor audit current --project .
```

### 사용 예시
- 새 맥북으로 교체했다 → 번들 하나로 모든 에이전트 설정 복원
- 팀원과 동일한 MCP/skills 환경을 공유
- 실수로 에이전트 설정이 망가졌다 → 직전 스냅샷으로 롤백
- 정기적으로 baseline과 diff 비교 → 예상치 못한 설정 변화 조기 발견

---

**타겟**
Claude Code/Codex/Cursor/Pi Agent/OpenCode를 사용하는 개발자,
AI tooling 엔지니어, DevEx/Platform 엔지니어,
여러 머신을 오가며 일하는 개발자
