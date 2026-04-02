uuid:    "auditor-01"
name:    "auditor"
description: "dalroot 원칙 위반 감사 — hook 로그 감시, 위반 경고, 회고 보고서"
version: "1.0.0"
player:  "claude"
model:   "haiku"
role:    "member"
skills:  ["skills/escalation"]
hooks:   []
auto_task:      "1. dalroot hook 로그에서 위반 패턴 감지 (pct exec, go build, systemctl restart, gh pr merge, curl mattermost). 2. 위반 발견 시 dal-control 채널 경고 + GitHub 이슈 생성 (label: audit/violation). 3. 부트스트랩 예외 시 사유 기록 여부 확인. 4. 24시간 감사 회고 보고서 생성."
auto_interval:  "1h"
git: {
	user:         "dal-auditor"
	email:        "dal-auditor@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
