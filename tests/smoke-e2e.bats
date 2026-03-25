#!/usr/bin/env bats
# dalcenter E2E smoke tests — 비파괴적, 읽기 전용
#
# 원칙: 운영 중인 dal에 영향 없음. wake/sleep/sync 하지 않음.
# 이미 돌고 있는 환경을 관찰만 한다.
#
# 실행:
#   DALCENTER_URL=http://localhost:11190 bats tests/smoke-e2e.bats

DALCENTER="${DALCENTER:-dalcenter}"

# ── 인프라 상태 확인 ──

@test "daemon HTTP 응답" {
    run curl -sf "$DALCENTER_URL/api/ps"
    [ "$status" -eq 0 ]
}

@test "daemon JSON 응답 파싱 가능" {
    result=$(curl -sf "$DALCENTER_URL/api/ps")
    echo "$result" | python3 -c "import json,sys; json.load(sys.stdin)"
}

@test "Docker 데몬 접근 가능" {
    run docker info --format "{{.ServerVersion}}"
    [ "$status" -eq 0 ]
    [ -n "$output" ]
}

@test "claude 이미지 존재" {
    run docker images dalcenter/claude:latest --quiet
    [ "$status" -eq 0 ]
    [ -n "$output" ]
}

# ── CLI 기본 ──

@test "dalcenter help" {
    run $DALCENTER --help
    [ "$status" -eq 0 ]
    [[ "$output" == *"wake"* ]]
    [[ "$output" == *"sleep"* ]]
    [[ "$output" == *"ps"* ]]
}

@test "dalcenter ps 작동" {
    run $DALCENTER ps
    [ "$status" -eq 0 ]
}

@test "dalcenter status 작동" {
    run $DALCENTER status
    [ "$status" -eq 0 ]
}

@test "dalcenter validate 작동" {
    run $DALCENTER validate
    [ "$status" -eq 0 ]
}

# ── 운영 중인 dal 관찰 (있으면) ──

@test "running dal이 있으면 ps에 표시" {
    result=$($DALCENTER ps 2>&1)
    # running dal이 없어도 OK (no awake dals)
    # 있으면 NAME/PLAYER/ROLE/STATUS 헤더가 있어야 함
    if [[ "$result" != *"no awake"* ]]; then
        [[ "$result" == *"NAME"* ]]
        [[ "$result" == *"PLAYER"* ]]
        [[ "$result" == *"STATUS"* ]]
    fi
}

@test "running dal 컨테이너 상태 확인" {
    containers=$(docker ps --filter "name=dal-" --format "{{.Names}}" 2>/dev/null)
    if [ -n "$containers" ]; then
        for c in $containers; do
            status=$(docker inspect "$c" --format "{{.State.Status}}")
            [ "$status" = "running" ]
        done
    else
        skip "no running dal containers"
    fi
}

@test "running dal 컨테이너에 dalcli 존재" {
    containers=$(docker ps --filter "name=dal-" --format "{{.Names}}" 2>/dev/null)
    if [ -n "$containers" ]; then
        first=$(echo "$containers" | head -1)
        run docker exec "$first" which dalcli
        [ "$status" -eq 0 ]
    else
        skip "no running dal containers"
    fi
}

@test "running dal 컨테이너 환경변수 설정됨" {
    containers=$(docker ps --filter "name=dal-" --format "{{.Names}}" 2>/dev/null)
    if [ -n "$containers" ]; then
        first=$(echo "$containers" | head -1)
        run docker exec "$first" printenv DAL_NAME
        [ "$status" -eq 0 ]
        [ -n "$output" ]

        run docker exec "$first" printenv DALCENTER_URL
        [ "$status" -eq 0 ]
        [[ "$output" == *"host.docker.internal"* ]]
    else
        skip "no running dal containers"
    fi
}

@test "running dal 컨테이너 host resolution" {
    containers=$(docker ps --filter "name=dal-" --format "{{.Names}}" 2>/dev/null)
    if [ -n "$containers" ]; then
        first=$(echo "$containers" | head -1)
        run docker exec "$first" getent hosts host.docker.internal
        [ "$status" -eq 0 ]
    else
        skip "no running dal containers"
    fi
}

@test "running dal 컨테이너에서 dalcli ps 통신" {
    containers=$(docker ps --filter "name=dal-" --format "{{.Names}}" 2>/dev/null)
    if [ -n "$containers" ]; then
        first=$(echo "$containers" | head -1)
        run docker exec "$first" dalcli ps
        [ "$status" -eq 0 ]
    else
        skip "no running dal containers"
    fi
}

@test "running dal 컨테이너에 workspace 마운트" {
    containers=$(docker ps --filter "name=dal-" --format "{{.Names}}" 2>/dev/null)
    if [ -n "$containers" ]; then
        first=$(echo "$containers" | head -1)
        run docker exec "$first" ls /workspace
        [ "$status" -eq 0 ]
    else
        skip "no running dal containers"
    fi
}

@test "running dal 컨테이너에 instructions 마운트" {
    containers=$(docker ps --filter "name=dal-" --format "{{.Names}}" 2>/dev/null)
    if [ -n "$containers" ]; then
        first=$(echo "$containers" | head -1)
        # claude → CLAUDE.md, codex → AGENTS.md
        run docker exec "$first" bash -c "ls /root/.claude/CLAUDE.md 2>/dev/null || ls /root/.codex/AGENTS.md 2>/dev/null || ls /root/.gemini/GEMINI.md 2>/dev/null"
        [ "$status" -eq 0 ]
    else
        skip "no running dal containers"
    fi
}

# ── agent-config API (읽기 전용) ──

@test "agent-config API 응답" {
    containers=$(docker ps --filter "name=dal-" --format "{{.Names}}" 2>/dev/null)
    if [ -n "$containers" ]; then
        first=$(echo "$containers" | head -1)
        dal_name=$(docker exec "$first" printenv DAL_NAME)
        port="${DALCENTER_URL##*:}"

        run docker exec "$first" curl -sf "http://host.docker.internal:$port/api/agent-config/$dal_name"
        [ "$status" -eq 0 ]
        [[ "$output" == *"dal_name"* ]]
        [[ "$output" == *"channel_id"* ]]
    else
        skip "no running dal containers"
    fi
}

# ── Mattermost 연결 (읽기 전용) ──

@test "Mattermost 접근 가능" {
    mm_url=$(docker exec "$(docker ps --filter 'name=dal-' --format '{{.Names}}' | head -1)" printenv DALCENTER_URL 2>/dev/null | sed 's|host.docker.internal|localhost|')
    if [ -n "$mm_url" ]; then
        # daemon 로그에서 MM URL 추출은 어려우니 skip 가능
        skip "MM URL 자동 감지 불가 — 수동 확인 필요"
    fi
}

# ── localdal 구조 검증 ──

@test "localdal에 dal.spec.cue 존재" {
    [ -f "$DALCENTER_LOCALDAL_PATH/dal.spec.cue" ]
}

@test "localdal에 leader 존재" {
    [ -d "$DALCENTER_LOCALDAL_PATH/leader" ]
    [ -f "$DALCENTER_LOCALDAL_PATH/leader/dal.cue" ]
    [ -f "$DALCENTER_LOCALDAL_PATH/leader/instructions.md" ]
}

@test "localdal의 모든 dal에 dal.cue + instructions.md 존재" {
    for dir in "$DALCENTER_LOCALDAL_PATH"/*/; do
        name=$(basename "$dir")
        [ "$name" = "skills" ] && continue
        [ "$name" = "gaya" ] && continue  # nested dals

        if [ -f "$dir/dal.cue" ]; then
            [ -f "$dir/instructions.md" ] || {
                echo "FAIL: $name has dal.cue but no instructions.md"
                return 1
            }
        fi
    done
}

@test "localdal skills 디렉토리에 SKILL.md 존재" {
    if [ -d "$DALCENTER_LOCALDAL_PATH/skills" ]; then
        for skill_dir in "$DALCENTER_LOCALDAL_PATH"/skills/*/; do
            [ -f "$skill_dir/SKILL.md" ] || {
                echo "FAIL: $(basename "$skill_dir") has no SKILL.md"
                return 1
            }
        done
    else
        skip "no skills directory"
    fi
}
