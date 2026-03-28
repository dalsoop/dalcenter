---
id: DAL:SKILL:7b85c395
---
# Inbox Protocol

## Rules
1. decisions.md, wisdom.md를 **직접 수정하지 않는다.**
2. 결정 제안: `/workspace/decisions/inbox/{name}-{date}-{slug}.md`에 드롭.
3. 교훈 제안: `/workspace/wisdom-inbox/{name}-{date}-{slug}.md`에 드롭.
4. 드롭 후 삭제하지 않는다 — scribe가 병합 후 삭제.

## Proposal Format (decisions)
```
### {날짜}: {주제}
**By:** {dal name}
**What:** {결정 내용}
**Why:** {이유}
```

## Proposal Format (wisdom)
Pattern:
```
**Pattern:** {설명}
**Context:** {언제 적용}
```
Anti-Pattern:
```
**Avoid:** {설명}
**Why:** {이유}
**Ref:** {PR/이슈}
```
