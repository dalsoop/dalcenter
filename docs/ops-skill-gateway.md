# Ops Skill Gateway 아키텍처

## 문제

ops dal들이 직접 외부 서비스(Cloudflare, GitHub, DNS)에 접근하는 구조:
- 각 dal 컨테이너에 토큰 분산 → 보안 표면 증가
- 부트스트래핑 반복 → 새 dal마다 토큰 주입 필요
- dal 컨테이너에서 PVE 호스트 직접 접근 불가

## 해결: LXC 101 경유 스킬 게이트웨이

```
ops dal (Docker)
  → dalcenter HTTP API (POST /api/ops/invoke)
    → LXC 101 API (proxmox-host-ops)
      → 외부 서비스 (CF, GitHub, DNS, ...)
```

ops dal은 토큰 없이 **스킬 이름 + 파라미터**만 전송.
인증/토큰은 LXC 101에서 중앙 관리.

## 스킬 목록

| 스킬 | 설명 | LXC 101 엔드포인트 |
|------|------|---------------------|
| `cf-pages-deploy` | Cloudflare Pages 배포 | `POST /api/cf-pages/deploy` |
| `dns-manage` | DNS CNAME/A 레코드 관리 | `POST /api/dns/manage` |
| `git-push` | Git push (토큰 불필요) | `POST /api/git/push` |
| `cert-manage` | TLS 인증서 관리 | `POST /api/cert/manage` |
| `service-restart` | systemd 서비스 재시작 | `POST /api/service/restart` |

## 요청/응답 구조

### 요청 (ops dal → dalcenter)

```json
{
  "skill": "cf-pages-deploy",
  "params": {
    "project": "landing-veilkey",
    "branch": "main",
    "directory": "./dist"
  },
  "dal": "ops-deployer"
}
```

### 응답

```json
{
  "ok": true,
  "skill": "cf-pages-deploy",
  "result": {
    "deployment_id": "abc123",
    "url": "https://landing-veilkey.pages.dev"
  }
}
```

### 에러

```json
{
  "ok": false,
  "skill": "cf-pages-deploy",
  "error": "project not found: landing-veilkey"
}
```

## dalcenter 측 구현

### 패키지 구조

```
internal/opsskill/
  types.go        # SkillRequest, SkillResponse, 스킬 이름 상수
internal/daemon/
  ops_skill.go    # handleOpsInvoke — LXC 101 프록시 핸들러
  client.go       # OpsInvoke() 클라이언트 메서드 추가
```

### 환경변수

| 변수 | 설명 | 예시 |
|------|------|------|
| `DALCENTER_OPS_GATEWAY_URL` | LXC 101 API 주소 | `http://10.50.0.101:8080` |
| `DALCENTER_OPS_GATEWAY_TOKEN` | LXC 101 인증 토큰 | Bearer token |

### HTTP 라우트

```
POST /api/ops/invoke    — 스킬 실행 (auth 필요)
GET  /api/ops/skills    — 사용 가능한 스킬 목록
```

### 프록시 흐름

1. ops dal이 `POST /api/ops/invoke` 호출
2. dalcenter가 요청 검증 (스킬 이름, 필수 파라미터)
3. 스킬 이름 → LXC 101 엔드포인트 매핑
4. `DALCENTER_OPS_GATEWAY_TOKEN`으로 인증 헤더 추가
5. LXC 101에 HTTP 요청 전달
6. 응답을 ops dal에 반환

### 스킬별 파라미터

#### cf-pages-deploy
| 파라미터 | 필수 | 설명 |
|----------|------|------|
| `project` | O | Pages 프로젝트 이름 |
| `branch` | X | 배포 브랜치 (기본: main) |
| `directory` | X | 빌드 출력 디렉토리 |

#### dns-manage
| 파라미터 | 필수 | 설명 |
|----------|------|------|
| `action` | O | create / update / delete |
| `zone` | O | DNS 존 |
| `name` | O | 레코드 이름 |
| `type` | O | A / CNAME / TXT |
| `content` | O | 레코드 값 |

#### git-push
| 파라미터 | 필수 | 설명 |
|----------|------|------|
| `repo` | O | 레포 경로 (org/repo) |
| `branch` | O | 브랜치 이름 |
| `remote` | X | 리모트 이름 (기본: origin) |

#### cert-manage
| 파라미터 | 필수 | 설명 |
|----------|------|------|
| `action` | O | issue / renew / revoke |
| `domain` | O | 도메인 |

#### service-restart
| 파라미터 | 필수 | 설명 |
|----------|------|------|
| `service` | O | systemd 서비스 이름 |
| `host` | X | 대상 호스트 (기본: local) |

## LXC 101 측 (proxmox-host-ops 레포)

별도 레포에서 구현. 여기서는 API 스펙만 정의.

### 엔드포인트

```
POST /api/cf-pages/deploy
POST /api/dns/manage
POST /api/git/push
POST /api/cert/manage
POST /api/service/restart
GET  /api/health
```

### 인증

- Bearer token (`X-Gateway-Token` 헤더)
- LXC 101에서만 CF_API_TOKEN, GITHUB_TOKEN 등 관리

## 보안 고려사항

- ops dal은 토큰 접근 불가 — 스킬 요청만 가능
- dalcenter가 스킬 이름 화이트리스트 검증
- LXC 101은 요청 로깅/감사 수행
- service-restart 스킬은 허용된 서비스 목록으로 제한
