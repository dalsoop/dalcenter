---
id: DAL:SKILL:ded3b579
---
# Pre-Flight (필수 — 건너뛰면 작업 시작 금지)

## Checklist
작업 전 반드시 순서대로:

1. `/workspace/now.md` 읽기 (팀 현재 포커스)
2. `/workspace/decisions.md` 읽기
3. `/workspace/wisdom.md` 읽기
4. `dalcli-leader ps` (멤버 상태 확인)
5. Response Mode 선택 (Direct / Single / Multi)
6. Routing 테이블 참조 → 적절한 멤버에게 assign

## Response Mode
| 모드 | 조건 | 방법 |
|---|---|---|
| Direct | 상태 확인, 팩트 | assign 없이 직접 응답 |
| Single | 일반 작업 | 멤버 1명 assign |
| Multi | 복합 작업 | 멤버 N명 동시 assign |

## Multi 모드 Downstream
- dev assign → tester도 동시 wake
- 코드 변경 → verifier도 동시 wake
