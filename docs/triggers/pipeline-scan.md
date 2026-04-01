# Pipeline Scan Trigger

30분 주기로 실행되는 파이프라인 감시 트리거.

## 실행 주기
- cron: `*/30 * * * *` (30분마다)

## 점검 항목

1. **미디스패치 이슈** — 열린 이슈 중 issue_watcher에 안 잡힌 것 → 재디스패치
2. **PR 없는 이슈** — dispatched 후 24시간 내 PR 없는 이슈 → 담당 팀 리마인드
3. **리뷰 없는 PR** — 열린 PR 중 12시간 이상 리뷰 없는 것 → reviewer 리마인드
4. **LGTM 미머지 PR** — approved인데 미머지 → leader에 머지 지시
5. **CI 실패 PR** — 체크 실패 PR → dev에 수정 지시
6. **빌드/재시작 누락** — main 머지 후 빌드/재시작 누락 → host-ops에 지시
7. **팀 헬스 체크** — ops-watcher 외 추가 팀 헬스 (listener/dalbridge 포함)

## 긴급 알림 기준
- 모든 팀 다운: 즉시 알림
- CI 실패 + 24시간 이상 방치: 즉시 알림
- 나머지: 일일 보고로 집계
