#!/usr/bin/env bash
set -euo pipefail

child=""
cleanup() {
  if [[ -n "$child" ]] && kill -0 "$child" 2>/dev/null; then
    kill -TERM -- "-$child" 2>/dev/null || true
    sleep 0.1
    kill -KILL -- "-$child" 2>/dev/null || true
  fi
}
trap cleanup EXIT

setsid bash -c 'sleep 300 & wait' &
child=$!
sleep 0.05
pgid="$(ps -o pgid= -p "$child" | tr -d ' ')"
kill -0 -- "-$pgid"
kill -TERM -- "-$pgid"
for _ in {1..20}; do
  if ! kill -0 -- "-$pgid" 2>/dev/null; then exit 0; fi
  sleep 0.05
done
kill -KILL -- "-$pgid"
