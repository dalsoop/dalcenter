---
id: DAL:SKILL:ab4a9fad
---
# Go CI 파이프라인

Go 프로젝트의 CI 검증을 수행하는 스킬.

## 실행 순서

```bash
# 1. 의존성 확인
go mod verify

# 2. 정적 분석
go vet ./...

# 3. 유닛 테스트
go test ./... -v -count=1 -timeout 120s

# 4. 빌드 (모든 바이너리)
go build ./...

# 5. (선택) 레이스 디텍터
go test -race ./... -count=1 -timeout 180s
```

## 실패 처리

- `go vet` 실패: 경고가 아닌 에러만 보고. 파일:라인 포함
- `go test` 실패: 실패한 테스트 이름 + 에러 메시지 요약. `FAIL` 라인 추출
- `go build` 실패: 컴파일 에러 전문 포함

## 결과 요약 형식

```
PASS: go vet (0.5s)
PASS: go test — 47/47 tests (3.2s)
PASS: go build — dalcenter, dalcli (1.1s)
```

또는:

```
PASS: go vet (0.5s)
FAIL: go test — 45/47 tests (3.2s)
  FAIL TestCircuitBreaker_HalfOpen (circuit_breaker_test.go:42)
  FAIL TestCredentialWatcher_Refresh (credential_watcher_test.go:55)
PASS: go build (1.1s)
```
