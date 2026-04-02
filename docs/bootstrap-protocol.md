# Bootstrap Protocol

dalroot가 직접 인프라를 조작해야 하는 긴급 상황의 프로토콜.
평상시 dalroot는 dal 경유 원칙을 따르지만, 모든 dal이 다운된 상황에서는 예외가 필요하다.

---

## Phase 0: 긴급 정리

**조건**: 모든 팀이 다운되었거나, dal을 경유할 수 없는 상황.

### 허용 범위

- dalcenter API 직접 호출 (`/api/` 엔드포인트)
- 팀 상태 확인 (`dalcenter ps`, `systemctl status`)
- 환경 변수 점검 (`/etc/dalcenter/*.env`)

### 금지 사항 (Phase 0에서도)

- `go build` + 바이너리 교체 (코드 변경 없이 재시작으로 해결)
- `gh pr merge` (긴급 상황과 무관)
- git force push, reset --hard

### 필수 기록

Phase 0 진입 시 반드시 기록:

```
## Bootstrap Log
- 시각: {ISO 8601}
- 사유: {왜 dal 경유가 불가능한지}
- 조치: {수행한 작업 목록}
- 복구: {dal 경유 복구 시점}
```

기록 위치: 해당 팀의 Mattermost 채널 또는 GitHub 이슈.

---

## Phase 1: 최소 1팀 복구

**조건**: 최소 1개 팀이 살아있음.

### 원칙

- **살아있는 팀 경유 필수** — dalroot가 직접 조작하지 않는다.
- 이슈를 생성하여 살아있는 팀의 dal에게 작업을 위임한다.
- 다른 팀 복구도 dal 경유로 진행.

### 허용 범위

- GitHub 이슈 생성 (`gh issue create`)
- Mattermost 메시지 전송 (dalbridge 경유)
- 팀 상태 조회 (읽기 전용)

---

## Phase 2: 정상 운영

**조건**: 모든 팀 정상 가동.

### 원칙

- dalroot 직접 조작 **완전 금지**.
- 모든 작업은 이슈 → leader → dal 흐름.
- auditor dal이 위반을 감시.

---

## 판단 기준

```
모든 팀 다운? ──yes──→ Phase 0 (긴급 정리, API만, 사유 기록)
     │no
     ▼
최소 1팀 가동? ──yes──→ Phase 1 (살아있는 팀 경유)
     │no (불가능: 위에서 걸림)
     ▼
전체 정상? ──yes──→ Phase 2 (정상 운영, 직접 조작 금지)
```

---

## 감사 연동

- auditor dal이 Phase 0 진입/종료를 추적
- 사유 기록 누락 시 경고 이슈 생성
- 회고 보고서에 부트스트랩 사용 현황 포함
