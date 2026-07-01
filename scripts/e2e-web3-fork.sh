#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Public RPC defaults keep this check runnable without committing provider keys.
# Override with private endpoints when available:
#   ETHEREUM_RPC_URL=https://... POLYGON_RPC_URL=https://... scripts/e2e-web3-fork.sh
export ETHEREUM_RPC_URL="${ETHEREUM_RPC_URL:-https://ethereum.publicnode.com}"
export POLYGON_RPC_URL="${POLYGON_RPC_URL:-https://polygon-bor-rpc.publicnode.com}"

echo "== web3 fork-local real token checks =="
echo "ethereum rpc: ${ETHEREUM_RPC_URL%%\?*}"
echo "polygon rpc: ${POLYGON_RPC_URL%%\?*}"

cd "$ROOT/web3"
forge build
forge test --match-path test/StakeEnforcerFork.t.sol -vv
