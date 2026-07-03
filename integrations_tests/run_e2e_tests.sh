#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

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

if [[ "${GOALSTAKES_SYSTEM_E2E_TIMEOUT_CHILD:-}" != "1" ]]; then
  run_with_timeout "${E2E_TOTAL_TIMEOUT_SECONDS:-600}" "system e2e total" env GOALSTAKES_SYSTEM_E2E_TIMEOUT_CHILD=1 "$0" "$@"
  exit $?
fi

echo "== shared services =="
(cd "$ROOT" && docker compose up -d)
for i in {1..40}; do
  if (cd "$ROOT" && docker compose exec -T postgres pg_isready -U goalstakes -d goalstakes >/dev/null 2>&1); then
    break
  fi
  if [[ "$i" == "40" ]]; then
    echo "postgres did not become healthy" >&2
    exit 1
  fi
  sleep 0.5
done

echo "== backend e2e =="
run_with_timeout "${E2E_BACKEND_TIMEOUT_SECONDS:-600}" "backend e2e" "$ROOT/backend/integration_test/run_e2e_tests.sh"

echo "== web3 fork-local e2e =="
run_with_timeout "${E2E_WEB3_TIMEOUT_SECONDS:-420}" "web3 fork-local e2e" "$ROOT/web3/integration_test/run_e2e_tests.sh"

echo "== browser/web/api/ai/android-api e2e =="
run_with_timeout "${E2E_WEB_WALLET_TIMEOUT_SECONDS:-300}" "browser/web/api/ai/android-api e2e" node "$ROOT/integrations_tests/web_wallet_e2e.mjs"

echo "== telegram bot e2e =="
run_with_timeout "${E2E_TELEGRAM_TIMEOUT_SECONDS:-120}" "telegram bot e2e" "$ROOT/telegram-bot/integration_test/run_e2e_tests.sh"

echo "== own-agent cron e2e =="
run_with_timeout "${E2E_OWN_AGENT_TIMEOUT_SECONDS:-120}" "own-agent cron e2e" node "$ROOT/integrations_tests/own_agent_cron_e2e.mjs"

echo "== mainnet gate shape =="
run_with_timeout "${E2E_MAINNET_SHAPE_TIMEOUT_SECONDS:-120}" "mainnet gate shape" "$ROOT/scripts/live_mainnet_gate.sh" shape

echo "== android emulator e2e =="
run_with_timeout "${E2E_ANDROID_TIMEOUT_SECONDS:-600}" "android emulator e2e" "$ROOT/android-app/integration_test/run_e2e_tests.sh"

echo "== secret scan =="
run_with_timeout "${E2E_SECRET_SCAN_TIMEOUT_SECONDS:-60}" "secret scan" "$ROOT/scripts/secret-scan.sh"

echo "integration suite passed"
