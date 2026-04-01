uuid:           "ops-20260401"
name:           "ops"
version:        "1.0.0"
player:         "claude"
model:          "haiku"
role:           "member"
description:    "cross-team health monitor & auto-recovery"
skills:         ["skills/docker-ops", "skills/escalation"]
hooks:          []
auto_task:      "1. /api/ps 폴링으로 전체 팀 dal 상태 확인. 2. 컨테이너 0개 팀 → leader 자동 시작. 3. leader 응답 없는 팀 → 재시작 시도. 4. dispatched 이슈 2시간 초과 미처리 → dalroot 알림. 5. dalroot-tell 실패 시 재시도 후 에스컬레이션."
auto_interval:  "2m"
git: {
	user:         "dal-${name}"
	email:        "dal-${name}@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
