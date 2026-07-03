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

if [[ "${GOALSTAKES_WEB3_E2E_TIMEOUT_CHILD:-}" != "1" ]]; then
  run_with_timeout "${WEB3_E2E_TOTAL_TIMEOUT_SECONDS:-600}" "web3 fork-local e2e total" env GOALSTAKES_WEB3_E2E_TIMEOUT_CHILD=1 "$0" "$@"
  exit $?
fi

# Public RPC defaults keep this check runnable without committing provider keys.
# Override with private endpoints when available:
#   ETHEREUM_RPC_URL=https://... POLYGON_RPC_URL=https://... web3/integration_test/run_e2e_tests.sh
export ETHEREUM_RPC_URL="${ETHEREUM_RPC_URL:-https://ethereum.publicnode.com}"
export POLYGON_RPC_URL="${POLYGON_RPC_URL:-https://polygon-bor-rpc.publicnode.com}"

echo "== web3 fork-local real token checks =="
echo "ethereum rpc: ${ETHEREUM_RPC_URL%%\?*}"
echo "polygon rpc: ${POLYGON_RPC_URL%%\?*}"

cd "$ROOT/web3"
run_with_timeout "${WEB3_E2E_BUILD_TIMEOUT_SECONDS:-120}" "web3 contract build" forge build
run_with_timeout "${WEB3_E2E_FORK_TEST_TIMEOUT_SECONDS:-300}" "web3 fork-local foundry tests" env FOUNDRY_TEST=integration_test forge test --match-path integration_test/StakeEnforcerFork.t.sol -vv
