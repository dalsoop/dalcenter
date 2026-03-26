# CUE 스키마

dalcenter의 dal 정의 스키마 관리.

## 파일 구조

- `dal.spec.cue` — 프로젝트 루트의 전체 스키마 (v2.0.0)
- `.dal/dal.spec.cue` — localdal 내 경량 스키마
- `.dal/{name}/dal.cue` — 개별 dal 프로필

## 필수 필드

```cue
uuid:    string   // 고유 식별자
name:    string   // dal 이름 (컨테이너명에 사용)
version: string   // SemVer
player:  "claude" | "codex" | "gemini"
role:    "leader" | "member"
```

## 검증

```bash
dalcenter validate           # 전체 검증
dalcenter validate .dal/     # localdal만 검증
```

## 주의사항

- `dal.cue`에서 참조하는 skills 경로가 실제 존재하는지 확인
- UUID 중복 불가
- player 값에 따라 컨테이너 이미지와 인증 경로가 결정됨
