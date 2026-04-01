uuid:    "cfgmgr-0001"
name:    "config-manager"
description: "charter/skill 동기화 + 설정 감사"
version: "1.0.0"
player:  "claude"
model:   "haiku"
role:    "member"
skills:  ["skills/git-workflow", "skills/pre-flight"]
hooks:   []
auto_task:      "1. git -C /workspace diff --name-only HEAD@{30.minutes.ago} -- .dal/template/ 로 템플릿 변경 감지. 변경 없으면 2로. 2. 변경 파일이 charter.md, dal.spec.cue, skills/ 중 하나면 동기화 대상. gh repo list dalsoop --json name -q '.[].name' 으로 팀 레포 목록 조회. 각 팀 레포에 대해: (a) 임시 디렉토리에 clone, (b) .dal/template/ 에서 변경된 공통 파일만 복사 (팀별 dal.cue, instructions.md 등 커스텀 파일은 보존), (c) 변경 있으면 브랜치 생성 + PR (gh pr create --title 'chore: sync template from dalcenter' --body '자동 동기화'). 3. dalcenter ps 로 실행 중인 dal 목록 확인. 각 dal에 대해 charter.md 내 Tools 섹션에 명시된 바이너리(ccw, dalcli 등)가 컨테이너 내 존재하는지 dalcenter attach {dal} -- which {binary} 로 확인. 불일치 시 gh issue create --title 'tool missing: {binary} in {dal}' --label config-audit. 4. 결과를 dalcli report 로 보고."
auto_interval:  "30m"
git: {
	user:         "dal-config-manager"
	email:        "dal-config-manager@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
