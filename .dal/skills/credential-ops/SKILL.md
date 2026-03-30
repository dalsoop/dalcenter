---
id: DAL:SKILL:cred0001
---
# Credential Ops — 토큰 관리

## 아키텍처

```
호스트 (PVE)
  ~/.claude/.credentials.json    ← Claude Code 자동 갱신
  ~/.codex/auth.json             ← Codex 자동 갱신
  dal-credential-sync (timer, 30분)
    → soft-serve push

LXC 105 (dalcenter)
  cred-watcher (5분)
    → soft-serve pull
    → ~/.claude/.credentials.json 덮어쓰기
    → bind-mount → 모든 Docker 컨테이너 즉시 반영
```

## 진단

### 토큰 만료 확인
```bash
# 호스트
python3 -c "
import json, datetime
with open('/root/.claude/.credentials.json') as f:
    d = json.load(f)
exp = d['claudeAiOauth']['expiresAt']
exp_dt = datetime.datetime.fromtimestamp(exp/1000)
print(f'만료: {exp_dt}, 남은시간: {exp_dt - datetime.datetime.now()}')
"

# LXC 105 (UTC)
pct exec 105 -- python3 -c "
import json, datetime
with open('/root/.claude/.credentials.json') as f:
    d = json.load(f)
exp = d['claudeAiOauth']['expiresAt']
exp_dt = datetime.datetime.fromtimestamp(exp/1000)
print(f'만료: {exp_dt}, 남은시간: {exp_dt - datetime.datetime.now()}')
"
```

### auth error 확인
```bash
pct exec 105 -- bash -c '
for c in $(docker ps --format "{{.Names}}"); do
  errs=$(docker logs --since 1h "$c" 2>&1 | grep -i "auth error" | tail -1)
  [ -n "$errs" ] && echo "$c: $errs"
done
'
```

## 복구

### 수동 토큰 갱신
```bash
# 1. 호스트 토큰 sync (호스트에서)
proxmox-host-setup ai sync --agent claude

# 2. LXC 105로 복사 (호스트에서)
pve-sync-creds 105

# 3. 또는 soft-serve 경유 (호스트에서)
dal-credential-sync
```

### cred-watcher 로그 확인
```bash
pct exec 105 -- journalctl -u dalcenter@dalcenter --since "30 min ago" --no-pager | grep "cred-watcher"
```

### 자동 갱신이 안 될 때
1. 호스트 timer 확인: `systemctl status dal-credential-sync.timer`
2. soft-serve 확인: `pct exec 105 -- ssh -p 23231 localhost info`
3. git repo 상태: `pct exec 105 -- git -C /root/.dalcenter-credential-origin log --oneline -3`
4. 환경변수 확인: `pct exec 105 -- grep CRED_GIT /etc/dalcenter/*.env`
