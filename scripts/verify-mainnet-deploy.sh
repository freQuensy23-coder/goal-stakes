#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
fi

require() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "missing required env: $name" >&2
    exit 1
  fi
}

validate_address() {
  local name="$1"
  local value="${!name:-}"
  if [[ ! "$value" =~ ^0x[0-9a-fA-F]{40}$ ]]; then
    echo "$name must be a 20-byte EVM address, got: $value" >&2
    exit 1
  fi
  if [[ "$value" =~ ^0x0{40}$ ]]; then
    echo "$name must not be the zero address" >&2
    exit 1
  fi
}

require ETHEREUM_STAKE_ENFORCER_ADDRESS
require POLYGON_STAKE_ENFORCER_ADDRESS
validate_address ETHEREUM_STAKE_ENFORCER_ADDRESS
validate_address POLYGON_STAKE_ENFORCER_ADDRESS

if [[ "$DRY_RUN" != "1" ]]; then
  require ETHEREUM_RPC_URL
  require POLYGON_RPC_URL
  require ENFORCER_PRIVATE_KEY
fi

ETHEREUM_RPC_URL_VALUE="${ETHEREUM_RPC_URL:-https://mainnet.example/rpc}"
POLYGON_RPC_URL_VALUE="${POLYGON_RPC_URL:-https://polygon.example/rpc}"

CHAINS_JSON="$(
  ETHEREUM_RPC_URL_VALUE="$ETHEREUM_RPC_URL_VALUE" \
  POLYGON_RPC_URL_VALUE="$POLYGON_RPC_URL_VALUE" \
  node <<'NODE'
const chains = {
  ethereum: {
    rpc_url: process.env.ETHEREUM_RPC_URL_VALUE,
    stake_enforcer_address: process.env.ETHEREUM_STAKE_ENFORCER_ADDRESS,
    tokens: {
      USDC: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
      USDT: "0xdAC17F958D2ee523a2206206994597C13D831ec7",
    },
  },
  polygon: {
    rpc_url: process.env.POLYGON_RPC_URL_VALUE,
    stake_enforcer_address: process.env.POLYGON_STAKE_ENFORCER_ADDRESS,
    tokens: {
      USDC: "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359",
      USDT: "0xc2132D05D31c914a87C6611C10748AEb04B58e8F",
    },
  },
};
process.stdout.write(JSON.stringify(chains));
NODE
)"

echo "CHAINS_JSON='$CHAINS_JSON'"

export HTTP_PORT="${HTTP_PORT:-8080}"
export DATABASE_URL="${DATABASE_URL:-postgres://goalstakes:goalstakes@localhost:5432/goalstakes?sslmode=disable}"
export JWT_SECRET="${JWT_SECRET:-replace-with-a-long-random-secret}"
export SIWE_DOMAIN="${SIWE_DOMAIN:-app.example.com}"
export SESSION_TTL="${SESSION_TTL:-24h}"
export SCHEDULER_INTERVAL="${SCHEDULER_INTERVAL:-1m}"
export OPENAI_API_KEY="${OPENAI_API_KEY:-}"
export OPENAI_MODEL="${OPENAI_MODEL:-}"
export OPENAI_TRANSCRIPTION_MODEL="${OPENAI_TRANSCRIPTION_MODEL:-}"
export OPENAI_BASE_URL="${OPENAI_BASE_URL:-}"
export CHAINS_JSON

if [[ "$DRY_RUN" == "1" ]]; then
  unset ENFORCER_PRIVATE_KEY
  export ALLOW_DISABLED_ENFORCER=true
else
  export ENFORCER_PRIVATE_KEY
  unset ALLOW_DISABLED_ENFORCER
fi

(cd "$ROOT/backend" && go run ./cmd/verifyconfig)
