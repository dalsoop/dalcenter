uuid:    "mail-monitor-01"
name:    "mail-monitor"
description: "메일 모니터링 — maddy 상태, 전송 큐, bounce rate, 일일 통계"
version: "1.0.0"
player:  "claude"
model:   "haiku"
role:    "member"
skills:  ["skills/escalation"]
hooks:   []
auto_task:      "1. maddy 서비스 상태 확인 (CT 122: pct exec <CTID> -- systemctl status maddy). 2. 전송 큐 적체 감지 (maddy 큐 디렉토리 확인). 3. bounce rate 이상 감지 (최근 1시간 bounce/total 비율, 임계값 5% 초과 시 경고). 4. 일일 전송 통계 보고 → dal-control 채널 포스팅."
auto_interval:  "30m"
git: {
	user:         "dal-mail-monitor"
	email:        "dal-mail-monitor@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
