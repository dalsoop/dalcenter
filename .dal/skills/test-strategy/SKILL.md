---
id: DAL:SKILL:88a22be2
---
# 테스트 전략

dalcenter 테스트 계층 및 원칙.

## 테스트 계층

### 1. 유닛 테스트 (`*_test.go`)
- 순수 로직 테스트. 외부 의존성 없음
- `t.TempDir()` 활용, 시스템 경로 직접 접근 금지
- 테이블 드리븐 패턴 권장

### 2. 스모크 테스트 (`tests/smoke-*.bats`)
- CLI 인터페이스 검증 (exit code, 출력 포맷)
- Docker daemon 필요하지만 실제 컨테이너 생성 최소화
- `@test "description" { run dalcli ...; assert_success; }`

### 3. E2E 테스트 (`tests/smoke-e2e.bats`)
- 전체 흐름 검증 (serve → wake → ps → sleep)
- Mattermost 연결 불요 (mock 가능)
- 운영 환경에 피해 금지

## 핵심 원칙

- **운영 피해 금지**: 실제 서비스(MM, Docker 프로덕션 컨테이너)에 영향주는 테스트 금지
- **결정적 테스트**: 시간, 네트워크 상태에 의존하지 않는 테스트
- **빠른 피드백**: 유닛 테스트는 1초 이내, 스모크는 10초 이내
- **격리**: 테스트 간 상태 공유 없음

## 커버리지 우선순위

1. credential 파싱/만료 체크 (보안 핵심)
2. CircuitBreaker 상태 전이
3. agent loop 메시지 파싱/라우팅
4. Docker 컨테이너 lifecycle
5. CUE 스키마 검증
