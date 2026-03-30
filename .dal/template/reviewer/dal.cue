uuid:    "a1b2c3d4-e5f6-7890-abcd-reviewer00001"
name:    "reviewer"
version: "1.0.0"
player:  "claude"
role:    "member"
channel_only: false
skills:  ["skills/git-workflow", "skills/pre-flight"]
hooks:   []
auto_task:     "1. gh pr list --state open 으로 리뷰 대기 PR 확인. 2. 각 PR에 대해: checkout → go build → go test → 코드 리뷰 (보안, 로직, 테스트 커버리지). 3. 문제 없으면 gh pr review --approve. 4. 문제 있으면 gh pr review --request-changes + 코멘트. 5. 리뷰 결과를 /workspace/review-log.md에 기록."
auto_interval: "15m"
git: {
	user:         "dal-reviewer"
	email:        "dal-reviewer@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
