# Escalation

## Report (작업 완료)
`dalcli report` — leader에게 결과 보고. history-buffer에 자동 기록.

## Claim (진행 불가)
`dalcli claim` — leader에게 에스컬레이션.
- 사유 명시: 환경 문제, 의존성, 권한, 스킬 갭 등.
- leader가 판단: 다른 멤버 할당 또는 사용자 에스컬레이션.

## Skill Gap Protocol
1. 적임자 없으면 → leader가 사용자에게 "새 dal 제안할까요?"
2. 사용자가 "그냥 해" → 가장 가까운 멤버에게 라우팅.
3. 같은 갭 2번 발생 → 팀 확장 넛지.
4. 기본값 = 직접 안 함.
