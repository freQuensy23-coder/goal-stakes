#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

echo "== telegram bot e2e =="
node "$ROOT/telegram-bot/integration_test/telegram_bot_e2e.mjs"
