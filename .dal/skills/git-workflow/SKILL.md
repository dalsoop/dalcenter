# Git Workflow

## Rules
1. main에 직접 커밋 금지.
2. 브랜치 생성 → 커밋 → PR → 리뷰 → 머지.
3. 브랜치명: `dal/{name}/{timestamp}` 또는 `feat/{slug}`.
4. 커밋 메시지: `feat:`, `fix:`, `refactor:`, `test:`, `docs:` prefix.
5. PR 생성 시 summary + test plan 포함.
6. force push 금지. destructive git 명령 금지.

## .dal/ 변경
- .dal/ 내 파일만 변경된 경우 → PR 생성하지 않고 auto prefix로 커밋+push.
- .dal/ + 코드 동시 변경 → 정상 PR 플로우.
