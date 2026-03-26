uuid:           "verifier-20260326"
name:           "verifier"
version:        "1.0.0"
player:         "claude"
player_version: "go"
role:           "member"
skills:         ["skills/go-review", "skills/go-ci", "skills/test-strategy", "skills/security-audit"]
hooks:          []
git: {
    user:         "dal-${name}"
    email:        "dal-${name}@dalcenter.local"
    github_token: "env:GITHUB_TOKEN"
}
