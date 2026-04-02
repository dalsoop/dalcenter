uuid:    "token-optimizer-01"
name:    "token-optimizer"
description: "토큰 사용량 분석 + 최적화 제안 — 비용 절감, 모델 다운그레이드, player 배분"
version: "1.0.0"
player:  "claude"
model:   "haiku"
role:    "member"
skills:  ["skills/escalation"]
hooks:   []
auto_task:      "1. /api/costs에서 dal별 토큰 사용량 수집. 2. 이상치 감지 (평균 대비 2x 초과 사용). 3. 프롬프트 최적화 제안 생성. 4. 단순 작업에 opus→haiku 다운그레이드 후보 식별. 5. codex 활용 가능 역할 분석. 6. 결과를 dal-control 채널에 보고."
auto_interval:  "1h"
git: {
	user:         "dal-token-optimizer"
	email:        "dal-token-optimizer@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
