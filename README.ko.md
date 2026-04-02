<div align="center">
  <h1>dalcenter</h1>
  <p><strong>Dal 생명주기 관리자 — AI 에이전트 컨테이너를 깨우고, 재우고, 동기화</strong></p>
  <p>
    <a href="https://github.com/dalsoop/dalcenter"><img src="https://img.shields.io/badge/github-dalsoop%2Fdalcenter-181717?logo=github&logoColor=white" alt="GitHub repository"></a>
    <a href="./LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-2563eb.svg" alt="AGPL-3.0 License"></a>
  </p>
  <p><a href="./README.md">English</a></p>
</div>

dalcenter는 dal(AI 인형)을 관리합니다. Claude Code, Codex, Gemini가 설치된 Docker 컨테이너를 각각의 스킬, 지시사항, git 인증으로 구성합니다. 템플릿은 git(localdal)으로 관리하고, dalcenter는 런타임을 담당합니다.

## 아키텍처

```
LXC: dalcenter
├── dalcenter serve              HTTP API + Docker 관리
│   ├── repo-watcher             2분 간격 git fetch/pull → .dal/ 변경 시 auto sync
│   ├── cred-watcher             토큰 만료 감지 → 자동 갱신
│   ├── scheduled-dalroot        파이프라인 감시 (미연결 이슈, 장기 대기 PR, 머지 지시)
│   ├── ops-watcher              전체 팀 헬스 폴링 → dal 0개 팀 자동 wake
│   ├── leader-watcher           leader 헬스체크 → 자동 복구
│   └── issue-watcher            GitHub 이슈 폴링 → 이슈 연결 시 auto wake
│
├── Docker: leader (claude)      dalcli-leader 내장 — 작업 라우팅 + 지시
├── Docker: dev (claude)         dalcli 내장 — 핵심 개발
├── Docker: reviewer (codex)     dalcli 내장 — 독립 코드 리뷰
└── Docker: dev-2 (claude)       복수 소환 가능

dalbridge (CT 또는 동일 호스트)
└── MM outgoing webhook → SSE stream 릴레이 (:4280)

dalroot (Proxmox 호스트)
└── Claude Code 인프라 관리 역할
    ├── dalroot-listener          SSE stream → inbox 파일 전달
    ├── dalroot-check-notifications  inbox 읽기 (Claude Code hook)
    └── dalroot-register          세션 등록 + MM 인증
```

### 바이너리

| 바이너리 | 위치 | 역할 |
|---|---|---|
| `dalcenter` | LXC 호스트 | 운영자 — 인프라 + 생명주기 관리 |
| `dalcli-leader` | leader 컨테이너 | 팀장 — 팀원 관리 + 작업 지시 |
| `dalcli` | member 컨테이너 | 팀원 — 상태 조회 + 보고 |
| `dalbridge` | CT 또는 동일 호스트 | MM webhook → SSE stream 릴레이 |

### 스코프 매트릭스

```
                    dalcenter (운영자)    dalcli-leader (팀장)    dalcli (팀원)
인프라
  serve             O                     -                       -
  init              O                     -                       -
  validate          O                     -                       -
생명주기
  wake              O (전체)              O (본인 팀)             -
  sleep             O (전체)              O (본인 팀)             -
관찰
  ps                O (전체)              O (본인 팀)             O (본인 팀)
  status            O (전체)              O (본인 팀)             O (본인만)
  logs              O (전체)              O (본인 팀)             -
  attach            O (전체)              O (본인 팀)             -
동기화
  sync              O                     O                       -
협업 (Mattermost)
  assign            -                     O (팀원에게 지시)       -
  report            -                     -                       O (팀장에게 보고)
```

## Dal 역할

각 dal은 독자적인 charter를 가진 Docker 컨테이너에서 실행됩니다. `.dal/`(활성 역할)과 `.dal/template/`(크로스 팀 재사용 템플릿)에 정의합니다.

### 활성 역할

| 역할 | Player | 설명 |
|------|--------|------|
| **leader** | claude | 이슈 분석, 작업 라우팅, PR 머지 권한. `dalcli-leader assign`으로 전문가에게 배분. |
| **dev** | claude | Go 핵심 개발 — 기능, 버그 수정, 리팩토링. `internal/`, `cmd/`, `dockerfiles/` 담당. |
| **reviewer** | codex | 독립 코드 리뷰 (다른 AI 관점). 보안, 동시성, 단순성 점검. |
| **tester** | claude | 테스트 전략 — 단위 테스트, 스모크 테스트, 커버리지 분석. 프로덕션 코드 변경 불가. |
| **verifier** | claude | 자동 검증 — `go vet`, `go test`, `go build`, `dalcenter validate`. 회귀 감지. |
| **scribe** | claude | 공유 메모리 관리자 — inbox → `decisions.md`/`wisdom.md` 병합, 히스토리 압축, 자동 커밋. |
| **codex-dev** | codex | Codex 기반 대안 개발자. 다른 구현 관점 제공. |
| **host** | - | 전체 팀을 관리하는 사용자 대리인. `#host` 채널로 조율. |

### 템플릿 역할 (`.dal/template/`)

config-manager가 모든 팀 레포에 동기화하는 크로스 팀 역할.

| 역할 | 설명 |
|------|------|
| **auditor** | dalroot 가드레일 위반 감시 (직접 인프라 조작, 무단 머지). dal-control 채널에 보고, GitHub 이슈 자동 생성. |
| **config-manager** | `.dal/template/` → 모든 팀 레포 동기화. 템플릿 drift 감지, sync PR 생성, 도구 설치 감사. |
| **dalops** | CCW 기반 워크플로우 오케스트레이터. 코드화된 워크플로우 실행 (`workflow-lite-plan`, `workflow-tdd-plan` 등). |
| **test-writer** | PR 변경분 자동 테스트 작성. 열린 PR에서 `_test.go` 없는 `.go` 파일 스캔. |
| **dal-infra** | 호스트 레벨 작업용 인프라 dal (바이너리 빌드, systemctl, credential sync). |
| **dal** (base) | 모든 역할이 공유하는 공통 charter 원칙의 기본 템플릿. |

## API 엔드포인트

기본 포트: `:11190`. 쓰기 엔드포인트는 Bearer 토큰 (`DALCENTER_TOKEN`) 필요.

### 읽기 (인증 불필요)

| 엔드포인트 | 설명 |
|-----------|------|
| `GET /api/health` | 서버 상태 + 연결 클라이언트 수 |
| `GET /api/ps` | 전체 컨테이너 목록 (이름, UUID, player, role, idle_for, 상태) |
| `GET /api/status` | 전체 dal 상태 |
| `GET /api/status/{name}` | 개별 dal 상태 (git diff, 마지막 활동, 유휴 시간) |
| `GET /api/logs/{name}` | Docker 컨테이너 로그 |
| `GET /api/tasks` | 전체 태스크 목록 |
| `GET /api/task/{id}` | 태스크 상세 (출력, 이벤트, 검증) |
| `GET /api/claims` | dal이 제출한 claim 목록 |
| `GET /api/claims/{id}` | claim 상세 |
| `GET /api/feedback` | 피드백 항목 |
| `GET /api/feedback/stats` | 피드백 통계 |
| `GET /api/costs` | 비용 추적 항목 |
| `GET /api/costs/summary` | 비용 요약 |
| `GET /api/escalations` | 에스컬레이션 목록 |
| `GET /api/issues` | GitHub 이슈 추적 |
| `GET /api/provider-status` | 프로바이더 서킷 브레이커 상태 (claude/codex/gemini) |
| `GET /api/issue-workflow/{id}` | 이슈 워크플로우 상태 |
| `GET /api/agent-config/{name}` | 에이전트 설정 (CLAUDE.md, 스킬, git 설정) |
| `GET /runs/{id}` | 태스크 실행 결과 HTML 페이지 |

### 쓰기 (Bearer 토큰 필요)

| 엔드포인트 | 설명 |
|-----------|------|
| `POST /api/wake/{name}` | dal 컨테이너 시작 (선택: `--issue`로 브랜치 추적) |
| `POST /api/sleep/{name}` | dal 컨테이너 정지 |
| `POST /api/restart/{name}` | dal 재시작 (sleep → wake) |
| `POST /api/replace/{name}` | Docker 이미지 교체 (하드 재시작) |
| `POST /api/sync` | .dal/ 변경사항 → 실행중인 컨테이너에 반영 |
| `POST /api/validate` | localdal CUE 스키마 + 스킬 참조 검증 |
| `POST /api/message` | Mattermost 메시지 릴레이 (from, message) |
| `POST /api/activity/{name}` | dal 활동 기록 (유휴 추적) |
| `POST /api/task` | 직접 태스크 생성 (dal, task) |
| `POST /api/task/start` | 태스크 실행 시작 |
| `POST /api/task/{id}/event` | 태스크 이벤트 추가 (kind, message) |
| `POST /api/task/{id}/metadata` | 태스크 메타데이터 업데이트 (git_diff, verified, completion) |
| `POST /api/task/{id}/finish` | 태스크 완료 (status, output, error) |
| `POST /api/claim` | claim 제출 (type: env/blocked/bug/improvement) |
| `POST /api/claims/{id}/respond` | claim 응답 (resolution, notes) |
| `POST /api/feedback` | 피드백 제출 (positive/neutral/negative) |
| `POST /api/cost` | 비용 항목 기록 |
| `POST /api/escalate` | 에스컬레이션 생성 (title, detail, priority) |
| `POST /api/escalations/{id}/resolve` | 에스컬레이션 해소 |
| `POST /api/provider-trip` | 프로바이더 서킷 브레이커 수동 trip |
| `POST /api/issue-workflow` | 이슈 워크플로우 트리거 (issue_id, member, task) |

## dalbridge — MM Webhook-to-SSE 릴레이

dalbridge는 Mattermost outgoing webhook을 받아 SSE(Server-Sent Events)로 재전송합니다. dal 컨테이너와 dalroot가 구독합니다.

### 엔드포인트

| 엔드포인트 | 메서드 | 설명 |
|-----------|--------|------|
| `/webhook` | POST | MM outgoing webhook 수신, 정규화, SSE 브로드캐스트 |
| `/stream` | GET | SSE 스트림 (선택: `?gateway=<name>` 필터) |
| `/api/message` | POST | dal→stream 메시지 릴레이 (dalcli-leader, daemon) |
| `/health` | GET | 헬스체크 (`{"status":"ok","clients":N}`) |

### 메시지 흐름

```
사용자가 Mattermost 채널에 입력
  → MM outgoing webhook → dalbridge POST /webhook
  → streamMessage로 정규화 {text, username, channel, gateway, post_id, timestamp}
  → 연결된 모든 클라이언트에 SSE 브로드캐스트
  → dal 컨테이너 (GET /stream)가 수신, 멘션 처리
  → dalroot-listener (GET /stream)가 inbox 파일로 기록
```

### 설정

| 변수 | 기본값 | 설명 |
|------|--------|------|
| `DALBRIDGE_LISTEN` | `:4280` | 리슨 주소 |
| `DALBRIDGE_WEBHOOK_TOKEN` | (선택) | outgoing webhook 검증 토큰 |

### ACK 흐름

1. dal이 문제 발생 → `POST /api/claim` (type: env/blocked/bug/improvement)
2. leader가 확인 → `GET /api/claims`
3. leader가 응답 → `POST /api/claims/{id}/respond`
4. dal이 스트림의 Mattermost 멘션으로 ACK 수신

## dalroot — 인프라 관리

dalroot는 Proxmox 호스트에서 직접 실행되는 Claude Code 인스턴스입니다 (Docker 컨테이너가 아님). 인프라 레이어를 관리합니다: LXC 컨테이너, 네트워킹, 호스트 조율, 서비스 관리.

### 알림 파이프라인

```
Mattermost (#dalcenter, #host, ...)
    │
    ▼ outgoing webhook
dalbridge (:4280/webhook)
    │
    ▼ SSE stream (:4280/stream)
dalroot-listener (호스트 데몬)
    │
    ▼ 파일 기록
/var/lib/dalroot/inbox/{dalroot-id}/*.msg
    │
    ▼ Claude Code hook (UserPromptSubmit)
dalroot-check-notifications → stdout → dalroot 처리
```

### 스크립트 (`proxmox-host-setup/dalroot/`)

| 스크립트 | 설명 |
|---------|------|
| `dalroot-id` | tmux pane으로 고유 ID 생성: `dalroot-{session}-{window}-{pane}` |
| `dalroot-check-notifications` | inbox에서 미읽은 메시지 읽기, stdout 출력, 처리된 파일 삭제 |
| `dalroot-register` | SessionStart hook — inbox 디렉토리 생성, MM에 ID 등록 |
| `dalroot-listener` | dalbridge SSE stream 구독 → inbox 파일 전달 데몬 |
| `dalroot-task` | dalcenter task 래퍼 (팀 라우팅 + 콜백 지원) |
| `install.sh` | 호스트 스크립트, dalbridge 서비스, MM webhook 설치 |
| `setup-webhooks.sh` | 각 팀 채널에 MM outgoing webhook 생성 |

### Claude Code Hooks (dalroot)

dalroot는 Claude Code hook으로 실시간 알림을 수신합니다:

```json
{
  "hooks": {
    "SessionStart": [{"type": "command", "command": "dalroot-register"}],
    "UserPromptSubmit": [{"type": "command", "command": "dalroot-check-notifications"}],
    "Notification": [{"type": "command", "command": "dalroot-check-notifications"}]
  }
}
```

### 가드레일 Hooks

PreToolUse hook이 위험한 작업을 감지하고 경고합니다:

| 패턴 | 경고 |
|------|------|
| `pct exec`, `qm start/stop/set` | 직접 인프라 조작 — 이슈로 dal에 위임 |
| `go build -o /usr/local/bin/` | 바이너리 빌드 — dal-infra/host-ops가 담당 |
| `systemctl restart/stop/start` | systemctl 조작 — host-ops 위임 |
| `gh pr merge` | PR 머지 — reviewer 리뷰 → leader 머지 |
| `curl.*mattermost`, `POST.*/api/v4/` | 직접 MM API 호출 — dalbridge/talk 사용 |

### 예약 파이프라인 감시

dalcenter가 주기적 검사를 실행합니다 (30분 간격 + 매일 09:00 KST 요약):

| 검사 | 조건 | 조치 |
|------|------|------|
| 미연결 이슈 | PR 없는 열린 이슈 (>1시간) | dal-control에 게시, 팀 알림 |
| 장기 대기 PR | 리뷰 없는 열린 PR (>12시간) | dal-control에 게시, reviewer 알림 |
| 승인 대기 PR | LGTM 리뷰 완료, 미머지 | dal-control에 게시, leader에 머지 알림 |
| 일일 요약 | 09:00 KST | 이슈/PR 현황 → dal-control 채널 |

### 자동화 파이프라인

```
사람 (GitHub 이슈 생성)
  → scheduled dalroot (이슈 감시, 리마인드)
  → dal 팀 (브랜치 + PR로 구현)
  → dal-control 채널 (진행 보고)
  → 사람 (이모지/코멘트로 승인·방향 제시)
```

## 설치

### 사전 요구사항

- **Go** 1.25.0+
- **Docker**
- **Git**
- **Mattermost** 서버 및 봇 토큰

### 빌드

```bash
go build -o /usr/local/bin/dalcenter ./cmd/dalcenter/
go build -o /usr/local/bin/dalcli ./cmd/dalcli/
go build -o /usr/local/bin/dalcli-leader ./cmd/dalcli-leader/
go build -o /usr/local/bin/dalbridge ./cmd/dalbridge/
```

### Docker 이미지 빌드

dal을 소환하기 전에 최소한 base 이미지를 빌드해야 합니다:

```bash
cd dockerfiles && docker build -t dalcenter/claude:latest -f claude.Dockerfile .
```

기타 이미지: `claude-go.Dockerfile`, `claude-rust.Dockerfile`, `codex.Dockerfile`, `gemini.Dockerfile`.

## 빠른 시작

```bash
# 1. 데몬 시작
dalcenter serve --addr :11190 --repo /path/to/your-project \
  --mm-url http://mattermost:8065 --mm-token TOKEN --mm-team myteam

# 2. localdal 초기화
dalcenter init --repo /path/to/your-project

# 3. dal 템플릿 작성 (git으로)
# .dal/leader/dal.cue + instructions.md
# .dal/dev/dal.cue + instructions.md
# .dal/skills/code-review/SKILL.md

# 4. 검증
dalcenter validate

# 5. dal 소환
dalcenter wake leader
dalcenter wake dev
dalcenter ps

# 6. 작업 끝
dalcenter sleep --all
```

## CLI 레퍼런스

### dalcenter (운영자)

```
dalcenter serve                   # 데몬 (HTTP API + watcher + Docker)
dalcenter init --repo <path>      # localdal 초기화 (.dal/ + subtree)
dalcenter wake <dal> [--all]      # Docker 컨테이너 생성
dalcenter sleep <dal> [--all]     # Docker 컨테이너 정지
dalcenter sync                    # .dal/ 변경사항 → 실행중인 dal에 반영
dalcenter validate [path]         # CUE 스키마 + 참조 검증
dalcenter status [dal]            # dal 상태
dalcenter ps                      # 소환된 dal 목록
dalcenter logs <dal>              # 컨테이너 로그
dalcenter attach <dal>            # 컨테이너 접속
dalcenter tell <repo> "<msg>"     # 다른 팀 leader에게 메시지 전달
```

### dalcli-leader (leader 컨테이너 내)

```
dalcli-leader wake <dal>          # 팀원 소환
dalcli-leader sleep <dal>         # 팀원 재우기
dalcli-leader ps                  # 팀원 목록
dalcli-leader status <dal>        # 팀원 상태
dalcli-leader logs <dal>          # 팀원 로그
dalcli-leader sync                # .dal/ 변경 반영
dalcli-leader assign <dal> <task> # @멘션으로 작업 지시
```

### dalcli (member 컨테이너 내)

```
dalcli status                     # 본인 상태
dalcli ps                         # 팀원 목록
dalcli report <message>           # 팀장에게 보고
```

## 동작 방식

### Wake 흐름

```
dalcenter wake dev
  1. .dal/dev/dal.cue 읽기 → player, skills, git config
  2. Docker 컨테이너 생성 (dalcenter/claude:latest)
  3. instructions.md → CLAUDE.md 변환 (bind mount)
  4. skills/ → ~/.claude/skills/ 마운트 (bind mount)
  5. .credentials.json 마운트 (read-only)
  6. 서비스 레포 → /workspace 마운트 (bind mount)
  7. GitHub 토큰 주입 (dal.cue git.github_token)
  8. dalcli/dalcli-leader 바이너리 주입 (docker cp)
  9. Mattermost 봇 계정 생성 + 채널 참가
  10. 환경변수: DAL_NAME, DAL_UUID, DAL_ROLE, DALCENTER_URL, GH_TOKEN
```

### Sync 흐름

```
.dal/ 변경 → git push (GitHub)
  → repo-watcher가 원격 변경 감지 (2분 이내)
  → git pull --ff-only
  → .dal/ diff → runSync()
  → bind mount 파일 (instructions.md, skills/) → 즉시 반영
  → dal.cue 구조 변경 (player, skills) → 컨테이너 자동 재시작
```

### 이슈 워크플로우

```
POST /api/issue-workflow {issue_id, member, task}
  → pending → waking (이슈 브랜치로 member 소환)
  → assigned (@멘션으로 작업 전달)
  → working (실행 추적)
  → done/failed (MM으로 dalroot 알림)
```

## localdal 구조

```
.dal/
  dal.spec.cue              스키마 정의
  leader/
    dal.cue                 uuid, player, role:leader
    charter.md              역할 charter
    instructions.md         → wake 시 CLAUDE.md로 변환
  dev/
    dal.cue                 uuid, player, role:member
    charter.md
    instructions.md
  skills/                   공유 스킬 풀
    code-review/SKILL.md
    testing/SKILL.md
    git-workflow/SKILL.md
    ...
  template/                 크로스 팀 재사용 템플릿
    auditor/
    config-manager/
    dalops/
    test-writer/
    dal-infra/
    skills/                 공유 템플릿 스킬
  dalroot/
    dal.cue                 인프라 관리 역할
    charter.md
```

## dal.cue

```cue
uuid:    "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
name:    "dev"
version: "1.0.0"
player:  "claude"
role:    "member"
channel_only: true
skills:  ["skills/code-review", "skills/testing"]
hooks:   []
git: {
    user:         "dal-dev"
    email:        "dal-dev@myproject.dev"
    github_token: "env:GITHUB_TOKEN"
}
```

## 파일명 변환

| 원본 | player | 컨테이너 내 |
|---|---|---|
| instructions.md | claude | CLAUDE.md |
| instructions.md | codex | AGENTS.md |
| instructions.md | gemini | GEMINI.md |

## 통신

dal 간 통신은 Mattermost. 프로젝트당 채널 1개 (serve 시 자동 생성).

- `dalcli-leader assign dev "작업"` → `@dal-dev 작업` 전송
- `dalcli report "완료"` → `[dev] 보고: 완료` 전송
- **dal-control** 채널 — scheduled dalroot 보고 및 사람 승인을 위한 인프라 대시보드

## 인증 (Credentials)

dalcenter는 player별 인증 정보를 컨테이너에 자동 마운트합니다 (read-only). wake 시 토큰 만료 경고.

| Player | 호스트 경로 | 컨테이너 경로 | 만료 체크 |
|--------|-----------|-------------|----------|
| claude | `~/.claude/.credentials.json` | `~/.claude/.credentials.json` | `expiresAt` (ms) |
| codex | `~/.codex/auth.json` | `~/.codex/auth.json` | `tokens.expires_at` (RFC3339) |
| gemini | env `GEMINI_API_KEY` | env `GEMINI_API_KEY` | — |

### 토큰 갱신

- **Claude**: 만료 시 호스트에서 `claude auth login` → `pve-sync-creds`
- **Codex**: 만료 시 호스트에서 `codex auth login` → `pve-sync-creds`
- **Gemini**: API 키 (만료 없음). `GEMINI_API_KEY` 환경변수 설정.
- 실행 중인 dal이 인증 실패 시 dalcli가 credential sync claim을 자동 생성.
- `DALCENTER_CRED_OPS_ENABLED`가 켜져 있으면(기본값), dalcenter가 credential sync를 자동 실행하고 `dal-ops` 채널에 보고.
- LXC 환경에서는 `DALCENTER_CRED_OPS_HTTP_URL`, `DALCENTER_CRED_OPS_HTTP_TOKEN`으로 호스트 bridge 설정. 참고 bridge: [`scripts/dalcenter-cred-ops-httpd.py`](./scripts/dalcenter-cred-ops-httpd.py).

## 환경변수

| 변수 | 설명 |
|------|------|
| `DALCENTER_URL` | dalcenter API 주소 (예: `http://localhost:11190`) |
| `DALCENTER_ADDR` | HTTP API 리슨 주소 |
| `DALCENTER_REPO` | 서비스 레포 경로 |
| `DALCENTER_LOCALDAL_PATH` | localdal 디렉토리 경로 |
| `DALCENTER_TOKEN` | 쓰기 엔드포인트 Bearer 토큰 |
| `DALCENTER_MM_URL` | Mattermost 서버 URL |
| `DALCENTER_MM_TOKEN` | Mattermost 봇 토큰 |
| `DALCENTER_MM_TEAM` | Mattermost 팀 이름 |
| `DALCENTER_GITHUB_REPO` | 이슈 폴링용 GitHub 레포 (`owner/repo`) |
| `DALCENTER_NOTIFY_URL` | 태스크 완료 콜백 URL |
| `DALCENTER_CRED_OPS_ENABLED` | credential sync ops 활성화 (`1`) |
| `DALCENTER_CRED_OPS_HTTP_URL` | 호스트 credential bridge URL |
| `DALCENTER_CRED_OPS_HTTP_TOKEN` | 호스트 bridge Bearer 토큰 |
| `DALCENTER_SCHEDULED_DALROOT` | 예약 파이프라인 감시 활성화 (`1`) |
| `DALCENTER_DALBRIDGE_URL` | dalbridge SSE stream URL |
| `DALCENTER_BRIDGE_URL` | Matterbridge API URL |
| `DALCENTER_BRIDGE_GATEWAY` | Matterbridge gateway 이름 |
| `DALCENTER_URLS` | 멀티 프로젝트 URL 라우팅 (쉼표 구분 `name=url`) |
| `GITHUB_TOKEN` | GitHub 인증 |

## 기여

[`CONTRIBUTING.md`](./CONTRIBUTING.md) 참고.
