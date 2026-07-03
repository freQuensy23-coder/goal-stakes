#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

run_with_timeout() {
  local seconds="$1"
  local label="$2"
  shift 2

  "$@" &
  local pid=$!
  local deadline=$((SECONDS + seconds))
  while kill -0 "$pid" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      echo "$label timed out after ${seconds}s" >&2
      kill "$pid" >/dev/null 2>&1 || true
      sleep 2
      kill -9 "$pid" >/dev/null 2>&1 || true
      wait "$pid" >/dev/null 2>&1 || true
      return 124
    fi
    sleep 1
  done
  wait "$pid"
}

if [[ "${GOALSTAKES_TELEGRAM_E2E_TIMEOUT_CHILD:-}" != "1" ]]; then
  run_with_timeout "${TELEGRAM_E2E_TOTAL_TIMEOUT_SECONDS:-600}" "telegram bot e2e total" env GOALSTAKES_TELEGRAM_E2E_TIMEOUT_CHILD=1 "$0" "$@"
  exit $?
fi

echo "== telegram bot e2e =="
run_with_timeout "${TELEGRAM_E2E_TIMEOUT_SECONDS:-120}" "telegram bot e2e" node "$ROOT/telegram-bot/integration_test/telegram_bot_e2e.mjs"
