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

if [[ "${GOALSTAKES_UNIT_TIMEOUT_CHILD:-}" != "1" ]]; then
  run_with_timeout "${UNIT_TEST_TIMEOUT_SECONDS:-600}" "unit test suite" env GOALSTAKES_UNIT_TIMEOUT_CHILD=1 "$0" "$@"
  exit $?
fi

echo "== test layout guard =="
bad_locations="$(
  find "$ROOT" \
    -path "$ROOT/frontend/node_modules" -prune -o \
    -path "$ROOT/web3/lib" -prune -o \
    -path "$ROOT/.git" -prune -o \
    -type f \( \
      -name '*_test.go' -o \
      -name '*.test.ts' -o \
      -name '*.test.tsx' -o \
      -name '*.spec.ts' -o \
      -name '*.spec.tsx' -o \
      -name '*.t.sol' -o \
      -name '*_e2e.mjs' \
    \) -print |
    sed "s#^$ROOT/##" |
    awk '($0 !~ /(^|\/)tests\// && $0 !~ /(^|\/)integration_test\// && $0 !~ /^integrations_tests\//) { print }'
)"
if [[ -n "$bad_locations" ]]; then
  echo "tests must live only under tests/, integration_test/, or integrations_tests/:" >&2
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
(cd "$ROOT/web3" && forge build && forge test)

echo "== android unit tests and build =="
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
(cd "$ROOT/android-app" && gradle testDebugUnitTest assembleDebug)

echo "== telegram bot unit tests =="
(cd "$ROOT/telegram-bot" && go test -count=1 ./...)

echo "unit suite passed"
