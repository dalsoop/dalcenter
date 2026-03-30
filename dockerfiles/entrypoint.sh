#!/bin/bash
# Dal container entrypoint.
# 1. Wait for dalcli to be injected (dalcenter copies it after container start)
# 2. Try to start agent loop (dalcli run)
# 3. If agent loop fails or exits, fall back to sleep infinity

set -e

# Restore settings.json if missing (may be overwritten by volume mounts)
if [ -f /etc/dal/settings.json.default ] && [ ! -f /root/.claude/settings.json ]; then
    cp /etc/dal/settings.json.default /root/.claude/settings.json
    echo "[entrypoint] restored settings.json from default"
fi

# Wait for dalcli binary (injected by dalcenter after docker run)
MAX_WAIT=30
for i in $(seq 1 $MAX_WAIT); do
    if [ -x /usr/local/bin/dalcli ]; then
        break
    fi
    sleep 1
done

# Give dalcenter daemon time to register this container
sleep 3

# Try agent loop, fall back to sleep
if [ -x /usr/local/bin/dalcli ]; then
    echo "[entrypoint] starting dalcli run..."
    dalcli run 2>&1 || {
        echo "[entrypoint] dalcli run exited ($?), falling back to sleep"
        exec sleep infinity
    }
else
    echo "[entrypoint] dalcli not found after ${MAX_WAIT}s, sleeping"
    exec sleep infinity
fi
