# Go 코드 리뷰

Go 코드 리뷰 시 다음을 체크합니다.

## 체크리스트

- [ ] 에러 반환값 무시하지 않음 (`_ = fn()` 금지, 의도적 무시만 허용)
- [ ] `fmt.Errorf("context: %w", err)` 패턴으로 에러 래핑
- [ ] goroutine 누수 없음 (context 취소, channel close 확인)
- [ ] `sync.Mutex`/`sync.RWMutex` 사용 시 defer Unlock 패턴
- [ ] `defer` 순서가 의도와 일치 (LIFO)
- [ ] 외부 프로세스 실행 시 timeout 설정
- [ ] 로그 메시지에 식별 가능한 prefix 포함
- [ ] 환경변수 하드코딩 없음
- [ ] 불필요한 exported 함수/타입 없음
- [ ] `go vet ./...` 경고 없음
