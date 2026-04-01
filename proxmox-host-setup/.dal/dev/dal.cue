uuid:    "phs-dev-20260401"
name:    "dev"
version: "1.0.0"
player:  "claude"
role:    "member"
description: "PHS developer — recipe files and Rust code"
skills:  ["skills/recipe-writing"]
hooks:   []
git: {
	user:         "dal-${name}"
	email:        "dal-${name}@proxmox-host-setup.local"
	github_token: "env:GITHUB_TOKEN"
}
