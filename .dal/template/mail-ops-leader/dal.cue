uuid:    "mail-ops-leader-01"
name:    "mail-ops-leader"
description: "메일 운영 리더 — 전송 요청 라우팅, 실패 분석, 재시도 판단"
version: "1.0.0"
player:  "claude"
role:    "leader"
skills:  ["skills/leader-protocol", "skills/escalation"]
hooks:   []
git: {
	user:         "dal-mail-ops-leader"
	email:        "dal-mail-ops-leader@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
