#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=../scripts/lib/test_support.sh
source "$ROOT/scripts/lib/test_support.sh"
ensure_forge_std "$ROOT"

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
"$ROOT/backend/integration_test/run_e2e_tests.sh"

echo "== web3 fork-local e2e =="
"$ROOT/web3/integration_test/run_e2e_tests.sh"

echo "== browser/web/api/ai/android-api e2e =="
node "$ROOT/integrations_tests/web_wallet_e2e.mjs"

echo "== telegram bot e2e =="
"$ROOT/telegram-bot/integration_test/run_e2e_tests.sh"

echo "== own-agent cron e2e =="
node "$ROOT/integrations_tests/own_agent_cron_e2e.mjs"

echo "== mainnet gate shape =="
"$ROOT/scripts/live_mainnet_gate.sh" shape

echo "== android emulator e2e =="
"$ROOT/android-app/integration_test/run_e2e_tests.sh"

echo "== secret scan =="
"$ROOT/scripts/secret-scan.sh"

echo "integration suite passed"
