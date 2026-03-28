uuid:           "verifier-20260326"
name:           "verifier"
version:        "1.0.0"
player:         "claude"
player_version: "go"
role:           "member"
skills:         ["skills/go-review", "skills/go-ci", "skills/test-strategy", "skills/security-audit", "skills/git-workflow", "skills/reviewer-protocol", "skills/inbox-protocol", "skills/history-hygiene", "skills/escalation"]
hooks:          []
auto_task:      "1. go vet ./... && go test ./... && go build ./cmd/dalcenter/ && go build ./cmd/dalcli/ 실행. 2. dalcli ps → 30분+ idle 멤버 감지 시 leader에게 보고. 3. gh pr list --state open → 24시간+ 방치 PR 감지 시 leader에게 보고. 실패 항목만 정리해서 보고. 전부 통과하면 보고 불필요 (로그에만 기록)."
auto_interval:  "30m"
git: {
    user:         "dal-${name}"
    email:        "dal-${name}@dalcenter.local"
    github_token: "env:GITHUB_TOKEN"
}
