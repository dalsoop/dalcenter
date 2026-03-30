---
keywords: architecture, design, structure
category: planning
---
# Architecture

## dalcenter
- cmd/dalcenter/ — CLI 엔트리포인트
- cmd/dalcli/ — dal agent CLI
- cmd/dalcli-leader/ — leader 전용 CLI
- internal/daemon/ — 데몬 로직
- internal/bridge/ — matterbridge 연동
- internal/localdal/ — 로컬 dal 관리

## Docker
- dockerfiles/ — claude/codex 이미지
- 각 dal은 독립 Docker 컨테이너로 실행
