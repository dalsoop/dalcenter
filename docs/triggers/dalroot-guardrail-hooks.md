# dalroot 가드레일 Hooks

dalroot Claude Code 환경에 설정하는 PreToolUse hook.
위반 패턴 감지 시 **경고 메시지 출력** (차단 아닌 경고).

## 설정 위치

dalroot의 `.claude/settings.json` 에 추가.

## Hook 정의

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hook": "bash -c 'CMD=\"$CLAUDE_TOOL_INPUT\"; WARN=\"\"; echo \"$CMD\" | grep -qE \"pct exec|qm (start|stop|set)\" && WARN=\"[GUARDRAIL] 인프라 직접 조작 감지. dal 경유 필수. 부트스트랩이면 사유를 기록하세요.\"; echo \"$CMD\" | grep -qE \"go build.*-o.*/usr/local/bin\" && WARN=\"$WARN [GUARDRAIL] 바이너리 빌드 감지. dal-infra/host-ops 담당 영역입니다.\"; echo \"$CMD\" | grep -qE \"systemctl (restart|stop|start|reload)\" && WARN=\"$WARN [GUARDRAIL] systemctl 조작 감지. host-ops 경유 필수.\"; echo \"$CMD\" | grep -qE \"gh pr merge\" && WARN=\"$WARN [GUARDRAIL] PR 머지 감지. reviewer 리뷰 후 leader가 지시해야 합니다.\"; echo \"$CMD\" | grep -qE \"curl.*mattermost|POST.*/api/v4/\" && WARN=\"$WARN [GUARDRAIL] MM API 직접 호출 감지. dalbridge/talk 경유 필수.\"; if [ -n \"$WARN\" ]; then echo \"$WARN\" >&2; fi'"
      }
    ]
  }
}
```

## 감지 패턴 상세

| 패턴 | 경고 메시지 | 올바른 경로 |
|------|-------------|-------------|
| `pct exec`, `qm start/stop/set` | 인프라 직접 조작 감지 | dal에게 이슈로 요청 |
| `go build -o /usr/local/bin/` | 바이너리 빌드 감지 | dal-infra 또는 host-ops가 빌드 |
| `systemctl restart/stop/start/reload` | systemctl 조작 감지 | host-ops 경유 |
| `gh pr merge` | PR 머지 감지 | reviewer 리뷰 → leader 지시 |
| `curl.*mattermost`, `POST.*/api/v4/` | MM API 직접 호출 감지 | dalbridge/talk 경유 |

## 동작 방식

- **차단하지 않음** — stderr로 경고 메시지만 출력
- dalroot가 경고를 보고 스스로 판단하도록 유도
- auditor dal이 hook 로그를 주기적으로 수집하여 위반 통계 생성

## 부트스트랩 예외

Phase 0 (긴급 정리) 중에는 직접 조작이 허용되나, 반드시 사유를 기록해야 한다.
경고 메시지에 "부트스트랩이면 사유를 기록하세요" 안내 포함.
