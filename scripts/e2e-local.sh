#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "== services =="
(cd "$ROOT" && docker compose up -d)
for i in {1..30}; do
  if (cd "$ROOT" && docker compose exec -T postgres pg_isready -U goalstakes -d goalstakes >/dev/null 2>&1); then
    break
  fi
  if [ "$i" = 30 ]; then
    echo "postgres did not become healthy" >&2
    exit 1
  fi
  sleep 1
done

echo "== backend =="
(cd "$ROOT/backend" && go test -count=1 ./...)

echo "== backend admin smoke =="
"$ROOT/scripts/e2e-backend-admin.sh"

echo "== frontend =="
(cd "$ROOT/frontend" && npm test && npm run build)

echo "== web, api, ai, and android client e2e =="
node "$ROOT/scripts/e2e-web-wallet.mjs"

echo "== web3 =="
(cd "$ROOT/web3" && forge build && forge test)

echo "== mainnet deploy handoff dry-run =="
ETHEREUM_STAKE_ENFORCER_ADDRESS=0x1111111111111111111111111111111111111111 \
POLYGON_STAKE_ENFORCER_ADDRESS=0x2222222222222222222222222222222222222222 \
"$ROOT/scripts/verify-mainnet-deploy.sh" --dry-run

echo "== backend + web3 local e2e =="
(cd "$ROOT/backend" && go test -tags e2e ./internal/web3 -count=1)

echo "== android =="
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
(cd "$ROOT/android-app" && gradle testDebugUnitTest assembleDebug)

echo "== telegram bot =="
(cd "$ROOT/telegram-bot" && go test -count=1 ./...)
node "$ROOT/scripts/e2e-telegram-bot.mjs"

echo "== own-agent cron =="
node "$ROOT/scripts/e2e-own-agent-cron.mjs"

echo "== secret scan =="
"$ROOT/scripts/secret-scan.sh"

echo "local e2e suite passed"
