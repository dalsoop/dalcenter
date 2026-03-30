---
keywords: code, convention, style, review
category: general
---
# Coding Conventions

## Go
- go vet + go build 통과 필수
- go test ./... 통과 후 PR
- 에러 핸들링: if err != nil 패턴
- 패키지명은 소문자 단일 단어

## Git
- 브랜치: issue-{N}/{dal-name} 또는 fix/{description}
- PR에 Closes #{N} 포함
- 커밋 메시지: feat/fix/refactor 접두사

## CCW
- 작업 시작: ccw session start
- 작업 종료: ccw session end
- 코드 분석: ccw tool read_file, edit_file
- 리뷰: ccw cli --tool codex --mode review
