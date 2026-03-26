uuid:    "reviewer-20260326"
name:    "reviewer"
version: "1.0.0"
player:  "codex"
role:    "member"
skills:  ["skills/go-review", "skills/security-audit", "skills/docker-ops"]
hooks:   []
git: {
    user:         "dal-${name}"
    email:        "dal-${name}@dalcenter.local"
    github_token: "env:GITHUB_TOKEN"
}
