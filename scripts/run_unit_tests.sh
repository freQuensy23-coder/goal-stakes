#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=lib/test_support.sh
source "$ROOT/scripts/lib/test_support.sh"

echo "== test layout guard =="
bad_locations="$(
  find "$ROOT" \
    -path "$ROOT/frontend/node_modules" -prune -o \
    -path "$ROOT/web3/lib" -prune -o \
    -path "$ROOT/.git" -prune -o \
    -type f \( -name '*_test.go' -o -name '*.test.ts' -o -name '*.t.sol' \) -print |
    sed "s#^$ROOT/##" |
    awk '($0 !~ /(^|\/)tests\// && $0 !~ /(^|\/)integration_test\//) { print }'
)"
if [[ -n "$bad_locations" ]]; then
  echo "tests must live only under tests/ or integration_test/:" >&2
  echo "$bad_locations" >&2
  exit 1
fi

if find "$ROOT/scripts" -maxdepth 1 -type f -name 'e2e-*' | grep -q .; then
  echo "ad-hoc e2e-* scripts are not allowed under scripts/; use integration_test/ or integrations_tests/" >&2
  find "$ROOT/scripts" -maxdepth 1 -type f -name 'e2e-*' >&2
  exit 1
fi

echo "== backend unit tests =="
(cd "$ROOT/backend" && go test -count=1 ./...)

echo "== frontend unit tests and build =="
(cd "$ROOT/frontend" && npm test && npm run build)

echo "== web3 unit tests =="
ensure_forge_std "$ROOT"
(cd "$ROOT/web3" && forge build && forge test)

echo "== android unit tests and build =="
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
(cd "$ROOT/android-app" && gradle testDebugUnitTest assembleDebug)

echo "== telegram bot unit tests =="
(cd "$ROOT/telegram-bot" && go test -count=1 ./...)

echo "unit suite passed"
