uuid:    "phs-leader-20260401"
name:    "leader"
version: "1.0.0"
player:  "claude"
role:    "leader"
description: "PHS team leader — task routing and oversight"
skills:  ["skills/recipe-writing"]
hooks:   []
git: {
	user:         "dal-${name}"
	email:        "dal-${name}@proxmox-host-setup.local"
	github_token: "env:GITHUB_TOKEN"
}
