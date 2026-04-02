# send-mail API 설계

## POST /api/send-mail

메일 전송 요청. dalcenter가 mail-ops-leader에게 task로 전달한다.

### 인증

`requireAuth` — 기존 dalcenter API 인증 패턴과 동일.

### Request

```json
{
  "to": ["recipient@example.com"],
  "cc": ["cc@example.com"],
  "bcc": ["bcc@example.com"],
  "subject": "메일 제목",
  "body": "메일 본문 (plain text)",
  "body_html": "<p>HTML 본문 (선택)</p>",
  "from_name": "발신자 표시명 (선택, 기본: env MAIL_FROM_NAME)",
  "reply_to": "reply@example.com",
  "priority": "normal"
}
```

| 필드 | 타입 | 필수 | 설명 |
|------|------|------|------|
| to | []string | O | 수신자 목록 |
| cc | []string | X | 참조 |
| bcc | []string | X | 숨은 참조 |
| subject | string | O | 메일 제목 |
| body | string | O | plain text 본문 |
| body_html | string | X | HTML 본문 (없으면 body 사용) |
| from_name | string | X | 발신자 표시명 |
| reply_to | string | X | 회신 주소 |
| priority | string | X | `low` / `normal` / `high` (기본: `normal`) |

### Response

#### 성공 (202 Accepted)

```json
{
  "id": "mail-task-uuid",
  "status": "queued",
  "message": "mail task created"
}
```

비동기 처리 — 즉시 전송이 아닌 task 큐에 등록.

#### 상태 조회: GET /api/send-mail/{id}

```json
{
  "id": "mail-task-uuid",
  "status": "sent",
  "to": ["recipient@example.com"],
  "subject": "메일 제목",
  "sent_at": "2026-04-02T10:30:00Z",
  "error": null
}
```

| status | 설명 |
|--------|------|
| queued | 큐 대기 중 |
| sending | 전송 중 |
| sent | 전송 완료 |
| failed | 전송 실패 |
| bounced | 반송됨 |

#### 에러

| 코드 | 설명 |
|------|------|
| 400 | 필수 필드 누락 또는 잘못된 이메일 형식 |
| 401 | 인증 실패 |
| 429 | rate limit 초과 |
| 503 | maddy 서비스 불가 |

## 구현 범위

### Phase 1 (이번 이슈)
- `POST /api/send-mail` 엔드포인트 등록 (daemon.go)
- 요청 검증 (to, subject, body 필수)
- mail-ops-leader에게 task 할당 (기존 `/api/task` 패턴 활용)
- 결과 상태 조회 (`GET /api/send-mail/{id}`)

### Phase 2 (후속)
- 첨부 파일 지원
- 템플릿 기반 메일 (template_id + variables)
- 예약 전송 (scheduled_at)
- 발송 이력 조회 API

## 기존 패턴 참고

```go
// daemon.go 등록 패턴
mux.HandleFunc("POST /api/send-mail", d.requireAuth(d.handleSendMail))
mux.HandleFunc("GET /api/send-mail/{id}", d.handleSendMailStatus)
```

핸들러 구조는 `handleMessage`와 동일:
1. JSON decode → struct
2. 필수 필드 검증
3. mail-ops-leader dal에 task 생성 (`d.tasks.New(...)`)
4. 202 + task ID 응답
