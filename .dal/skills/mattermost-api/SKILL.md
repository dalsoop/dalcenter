# Mattermost API 통합

dalcenter의 Mattermost 통신 구조.

## 구조

- dalcenter가 프로젝트 채널 자동 생성
- dal당 bot 1개 (이름: `dal-{name}`)
- bot token은 dalcenter가 생성하여 컨테이너에 환경변수로 전달

## API 패턴

```
POST /api/v4/posts              — 메시지 전송
GET  /api/v4/channels/{id}/posts — 채널 메시지 조회
GET  /api/v4/posts/{id}/thread   — 스레드 컨텍스트 조회
POST /api/v4/bots                — bot 생성
```

## 메시지 프로토콜

- `@dal-{name} 작업 지시: {task}` → dal이 작업 시작
- `@dal-{name} {자유 멘션}` → free-form 응답
- `[{name}] 보고: {message}` → 결과 보고
- 스레드 내 후속 메시지 → 자동 응답 (activeThreads 추적)

## 주의사항

- bot은 다른 bot을 생성할 수 없음 → personal access token 필요
- MM URL, token, team은 `dalcenter serve` 플래그로 전달
