**Hem** 제품 설명

### 한 줄 요약
**AI 코딩 에이전트 환경을 저장하고, 비교하고, 되돌리는 로컬 타임머신**

### 상세 설명
Hem은 Claude Code, Codex, Cursor, Pi Agent, OpenCode 같은 AI 코딩 에이전트의
MCP 서버, skills, permissions, hooks, system prompt, env key inventory, project instructions를
하나의 agent setup으로 읽어옵니다.

사용자는 현재 설치된 구성요소를 한눈에 보고, 바뀐 내용을 snapshot으로 저장하고,
두 시점의 setup을 비교하고, 꼬이면 이전 상태로 restore할 수 있습니다.

새 Mac이나 다른 장비로 옮길 때는 저장된 setup을 `.hem` bundle로 내보내고,
대상 머신에서 readiness를 확인한 뒤 같은 환경에 가깝게 불러올 수 있습니다.

### 핵심 포지션
Hem은 marketplace나 보안 대시보드가 아니라, 개인 개발자를 위한 로컬 agent setup manager입니다.

초기 사용자는 Claude Code/Codex/Cursor를 자주 바꾸고, agent에게 skills나 MCP 설치를 맡기며,
나중에 무엇이 원래 있었고 무엇이 새로 추가됐는지 헷갈리는 AI coding power user입니다.

핵심 감각은:

> agent setup에 Git-like history를 준다.

### 신뢰 계약 (Trust Contract)
- 절대 MCP command/hook/script를 실행하지 않음
- 절대 네트워크에 접속하지 않음 (기본값)
- 절대 사용자 설정을 동의 없이 수정하지 않음
- `.env` 값은 key name만 수집하고 raw secret 값은 저장하지 않음
- 절대 symlink를 따라가지 않음
- restore/import 전에는 dry-run preview를 제공함
- destructive apply 전에는 현재 setup을 먼저 snapshot으로 저장함
- doctor/preflight는 누락된 도구와 env key를 보고하지만 패키지 설치나 secret 복원은 하지 않음

### 관리하는 대상
- System Prompt / Instructions: `CLAUDE.md`, `AGENTS.md`, `CODE.md`
- MCP Servers: `.mcp.json`, `.cursor/mcp.json`, Claude Desktop config
- Permission Rules & Hooks: `settings.json`
- Skills: Claude/Codex/Pi Agent/OpenCode skills
- Env Key Inventory: `.env` key names only
- Pi Agent: settings, extensions, skills, themes, prompts, agents, models
- OpenCode: config, skills

### 핵심 기능

**로컬 관리**
- **Inventory** — agent별 skills, MCP, hooks, permissions, instructions 목록 보기
- **Save Setup** — 현재 전체 agent setup을 snapshot으로 저장
- **Compare** — 저장된 setup과 현재 setup을 side-by-side로 비교
- **Restore** — 이전 snapshot으로 되돌리기
- **Profile** — `default`, `frontend`, `clean-baseline` 같은 setup line 관리
- **Bundle** — setup을 `.hem` 파일로 내보내고 다른 머신에서 불러오기

**진단**
- **Scan** — 프로젝트의 모든 에이전트 설정을 자동 탐지
- **Diff** — 두 시점 간 변경점 비교
- **Provenance** — 모든 설정의 출처 추적
- **Doctor** — 복원 전 필요한 로컬 도구, MCP command, env key gap 확인
- **Audit** — 개인용 MVP에서는 전면 기능이 아니라 restore/import 전 참고 정보로 사용

### 핵심 워크플로우

```bash
# 현재 agent setup 확인
hem scan --project .

# 현재 상태 저장
hem snapshot create --name baseline --metadata-only --project .

# agent가 MCP/skill을 설치하거나 설정을 바꾼 뒤
hem diff baseline current --project .

# 꼬이면 이전 상태로 복원
hem restore --snapshot baseline --dry-run --project .
hem restore --snapshot baseline --apply --experimental --project .
```

```bash
# 다른 머신으로 옮기기
hem bundle export --name baseline --out daily.hem --project .
hem bundle verify daily.hem
hem doctor --project .
hem bundle import daily.hem --dry-run --project .
```

### 사용 예시
- agent에게 skill을 설치하게 했는데 무엇이 바뀌었는지 모르겠다 → Hem에서 compare
- 새로 설치한 MCP 때문에 환경이 꼬였다 → 직전 snapshot으로 restore
- 새 Mac을 샀다 → `.hem` bundle로 기존 setup 불러오기
- 프로젝트마다 다른 agent setup을 쓰고 싶다 → profile로 관리
- 현재 Claude/Codex/Cursor에 어떤 skills와 MCP가 있는지 모르겠다 → agent별 inventory 확인

### 무료/유료 원칙

**Free**
- TUI/Desktop local app
- agent setup inventory
- MCP/skills add/remove/disable
- local snapshot history
- compare
- restore
- local profiles
- manual `.hem` export/import

**Pro**
- cloud profiles
- multi-machine sync
- encrypted cloud backup
- hosted profile links
- cloud profile history

**Team**
- shared onboarding profiles
- approved MCP/skills catalog
- member/device status
- profile versioning
- admin dashboard

### 타겟
초기 타겟은 AI coding power user입니다.

- Claude Code/Codex/Cursor를 여러 개 같이 쓰는 개발자
- MCP와 skills를 자주 실험하는 개발자
- agent에게 자기 agent 환경 설치/수정을 맡기는 개발자
- 새 Mac이나 여러 장비 사이에서 같은 agent setup을 쓰고 싶은 개발자

장기적으로는 DevEx/Platform 팀의 신규 엔지니어 onboarding profile로 확장합니다.
