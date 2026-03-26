uuid:    "mac-org-20260327"
name:    "mac-organizer"
version: "1.0.0"
player:  "claude"
role:    "member"
description: "MacBook 환경 정리 에이전트 — 폴더 구조, 파일 분류, 스케줄러, 프로젝트 관리"
skills: [
    "skills/macos-launchagent",
    "skills/file-organize",
    "skills/cue-schema",
    "skills/shell-script",
]
hooks: []
git: {
    user:         "dal-mac-organizer"
    email:        "dal-mac-organizer@dalcenter.local"
    github_token: "env:GITHUB_TOKEN"
}
