uuid:    "smtp-worker-01"
name:    "smtp-worker"
description: "SMTP 전송 실행 — maddy 설정 관리, DKIM/SPF/DMARC, 큐 모니터링"
version: "1.0.0"
player:  "claude"
role:    "member"
skills:  ["skills/git-workflow", "skills/escalation"]
hooks:   []
git: {
	user:         "dal-smtp-worker"
	email:        "dal-smtp-worker@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
