# Docker 운영

dalcenter의 Docker 컨테이너 관리 관련 지식.

## dalcenter Docker 구조

- 컨테이너 prefix: `dal-`
- 이미지 prefix: `dalcenter/`
- 이미지 종류: `claude:latest`, `codex:latest`, `gemini:latest`
- 컨테이너 workdir: `/workspace`

## 마운트

| 목적 | 호스트 | 컨테이너 | 모드 |
|------|--------|----------|------|
| 서비스 레포 | `--repo` 경로 | `/workspace` | rw |
| Claude 인증 | `~/.claude/.credentials.json` | 동일 경로 | rw |
| Codex 인증 | `~/.codex/auth.json` | 동일 경로 | rw |

## 주의사항

- Linux에서 `--add-host host.docker.internal:host-gateway` 필수
- 인증 파일 마운트는 rw (컨테이너 내 토큰 갱신 허용)
- `dalcli` 바이너리는 wake 시 컨테이너에 inject
- entrypoint.sh가 dalcli inject 대기 후 `dalcli run` 실행
