---
id: DAL:SKILL:78799ec9
---
# Reviewer Protocol

## Rules
1. 작성자 ≠ 리뷰어. 자기 코드를 자기가 리뷰하지 않는다.
2. 리뷰어가 리젝한 PR → 원작성자가 수정. 리뷰어 본인이 수정 금지 (독립성).
3. 모든 멤버 lockout 시 → leader가 사용자에게 에스컬레이션.
4. 리뷰 시 확인:
   - 에러 핸들링 (swallowed errors 없는지)
   - 보안 (crypto/rand, 하드코딩된 시크릿)
   - 테스트 유무
   - 기존 테스트 깨지지 않는지

## Product Isolation
- dal 이름 (dal-dev 등)을 코드에 하드코딩하지 않는다.
- 팀 구성 변경 시 깨지는 코드를 만들지 않는다.
