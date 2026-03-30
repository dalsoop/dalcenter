#!/bin/bash
# Dal container entrypoint.
# 1. Wait for dalcli to be injected
# 2. Start agent loop with retry
# 3. Fall back to sleep infinity after max retries

set -e

MAX_WAIT=30
for i in $(seq 1 $MAX_WAIT); do
    if [ -x /usr/local/bin/dalcli ]; then
        break
    fi
    sleep 1
done

sleep 3

if [ ! -x /usr/local/bin/dalcli ]; then
    echo "[entrypoint] dalcli not found after ${MAX_WAIT}s, sleeping"
    exec sleep infinity
fi

# Retry dalcli run up to 5 times with backoff
MAX_RETRY=5
for attempt in $(seq 1 $MAX_RETRY); do
    echo "[entrypoint] starting dalcli run (attempt ${attempt}/${MAX_RETRY})..."
    dalcli run 2>&1 && exit 0
    EXIT_CODE=$?
    if [ $attempt -lt $MAX_RETRY ]; then
        WAIT=$((attempt * 5))
        echo "[entrypoint] dalcli run exited (${EXIT_CODE}), retrying in ${WAIT}s..."
        sleep $WAIT
    fi
done

echo "[entrypoint] dalcli run failed after ${MAX_RETRY} attempts, sleeping"
exec sleep infinity
