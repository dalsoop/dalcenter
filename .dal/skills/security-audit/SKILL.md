# 보안 감사

dalcenter의 보안 민감 영역 분석 스킬.

## 핵심 영역

### 1. Credential 관리
- Claude OAuth token: `~/.claude/.credentials.json` (expiresAt, milliseconds)
- Codex OAuth token: `~/.codex/auth.json` (tokens.expires_at, RFC3339)
- credential watcher: 5분 주기 체크, 1시간 임계값 내 자동 갱신
- 컨테이너 마운트는 rw (갱신 허용)

### 2. Docker 보안
- 컨테이너 내 root 실행 (현재)
- `--add-host host.docker.internal:host-gateway` 사용
- workspace 마운트 rw → 컨테이너가 호스트 파일 수정 가능

### 3. Mattermost 통신
- bot token이 환경변수로 컨테이너에 전달
- 채널 메시지는 평문
- MM 서버 접근은 내부 네트워크(10.x)

### 4. CircuitBreaker
- 3회 실패 → open → 2분 cooldown → half-open
- 인증 에러 감지 → credential refresh 알림
- fallback player 전환 시 인증 상태 확인 필요

## 체크리스트

- [ ] credential 파일 퍼미션 (0600)
- [ ] 환경변수에 secret 노출 여부
- [ ] Docker 마운트 최소 권한
- [ ] 로그에 token/password 미노출
- [ ] 외부 프로세스 실행 시 인자 injection 방지
