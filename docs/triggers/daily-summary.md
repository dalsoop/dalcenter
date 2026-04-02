# Daily Summary Trigger

매일 09:00에 실행되는 일일 요약 트리거.

## 실행 주기
- cron: `57 8 * * *` (매일 08:57, 09:00 피크 회피)

## 보고 내용

1. **완료 이슈** — 전일 대비 닫힌 이슈 목록
2. **진행 중 이슈** — 현재 열린 이슈 + 담당 dal 상태
3. **블로커** — blocked claim, CI 실패, 리뷰 지연 등
4. **팀 상태** — 각 팀 dal 가동 현황

## 전달 채널
- Mattermost 팀 채널 (dalcenter `/api/message` 경유)
