**snaptailor** 제품 설명

### 한 줄 요약
**AI 코딩 에이전트 설정 환경을 읽기 전용으로 진단하는 드리프트 감사 CLI**

### 상세 설명
snaptailor는 AI 코딩 에이전트(Claude Code, Codex, Cursor, Pi Agent, OpenCode)의 설정 변화를
**읽기 전용**으로 스캔·비교·감사하는 도구입니다.

MCP 서버, skill, permission, hook, env key, system prompt 등이
어떻게 바뀌었는지, 왜 위험한지, 어디서 온 설정인지를 추적합니다.

### 신뢰 계약 (Trust Contract)
- 절대 MCP command/hook/script를 실행하지 않음
- 절대 네트워크에 접속하지 않음 (기본값)
- 절대 사용자 설정을 수정하지 않음 (scan 경로)
- 절대 .env 값을 저장하지 않음 (key 이름만 수집)
- 절대 symlink를 따라가지 않음
- Snapshot은 metadata-only (비밀값 redact/omit)

### 관리하는 대상 (v0.1)
- System Prompt (CLAUDE.md, AGENTS.md)
- MCP Servers (.mcp.json, .cursor/mcp.json)
- Permission Rules &amp; Hooks (settings.json)
- Skills (claude/skills, codex/skills, pi-agent/skills)
- Env Key Inventory (.env, key name only)
- Pi Agent: settings, extensions, skills, themes, prompts, agents, models
- OpenCode: config, skills

### v0.1 핵심 기능
- **Scan** — 프로젝트의 모든 에이전트 설정을 자동 탐지하고 evidence inventory 생성
- **Snapshot** — 현재 상태의 metadata-only 기록 (비밀값 제외)
- **Diff** — 두 시점 간 변경점 상세 비교 (semantic + raw diff)
- **Audit** — 설정 변화의 보안/위험 finding 자동 감지
- **Provenance** — 모든 설정의 출처 추적 (user vs project)
- **Report** — 사람이 읽을 수 있는 Markdown 리포트 export
- **Bundle** (experimental) — .stailor tar 번들 export/import/inspect
- **Restore** (experimental) — dry-run plan + per-type apply + rollback

### 컨셉
영국 Savile Row의 고급 양장점처럼,
**"당신의 AI 에이전트를 최고의 상태로 맞춤 재단한다"**는 철학.
v0.1은 맞춤 재단 전에 **"현재 상태를 정밀하게 측정하고 기록한다"**에 집중합니다.

### 사용 예시
- 에이전트가 갑자기 이상해졌다 → 어떤 설정이 바뀌었는지 확인
- 협업자의 PR이 MCP 설정을 바꿨다 → 변경점 감사
- 정기적으로 baseline과 diff 비교 → 예상치 못한 설정 변화 조기 발견
- CI에서 PR의 에이전트 설정 변화를 자동 감사

---

**타겟**
Claude Code/Codex/Cursor/Pi Agent/OpenCode를 사용하는 개발자,
AI tooling 엔지니어, DevEx/Platform 엔지니어,
보안에 민감한 스타트업 엔지니어
