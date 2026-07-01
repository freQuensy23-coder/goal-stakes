#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "scanning built frontend and e2e artifacts for generated secrets"
if [ -d "$ROOT/frontend/dist" ]; then
  if rg -I -n 'Authorization: Bearer sk_[A-Za-z0-9_-]{20,}|agt_[A-Za-z0-9_-]{32,}|TELEGRAM_BOT_TOKEN|ENFORCER_PRIVATE_KEY=0x[0-9a-fA-F]{64}' "$ROOT/frontend/dist"; then
    echo "built frontend contains secret-shaped material" >&2
    exit 1
  fi
fi

if [ -d "$ROOT/.e2e" ]; then
  if rg -I -n 'Authorization: Bearer sk_[A-Za-z0-9_-]{20,}|TELEGRAM_BOT_TOKEN=[0-9]+:[A-Za-z0-9_-]{20,}|ENFORCER_PRIVATE_KEY=0x[0-9a-fA-F]{64}' "$ROOT/.e2e"; then
    echo "e2e artifacts contain secret-shaped material" >&2
    exit 1
  fi
fi

echo "scanning docs and fixtures for live-looking provider secrets"
if rg -I -n 'OPENAI_API_KEY=sk-[A-Za-z0-9]{20,}|TELEGRAM_BOT_TOKEN=[0-9]+:[A-Za-z0-9_-]{20,}|ENFORCER_PRIVATE_KEY=0x[0-9a-fA-F]{64}|JWT_SECRET=[A-Za-z0-9_-]{32,}' \
  "$ROOT/README.md" "$ROOT/docs" "$ROOT/scripts" "$ROOT/frontend" "$ROOT/backend" "$ROOT/telegram-bot" "$ROOT/android-app" \
  -g '!node_modules' -g '!dist' -g '!build' -g '!*.png'; then
  echo "repository docs or fixtures contain live-looking secrets" >&2
  exit 1
fi

echo "secret scan passed"
