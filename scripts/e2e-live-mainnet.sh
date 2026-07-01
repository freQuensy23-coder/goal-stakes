#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-preflight}"
ENV_FILE="${ENV_FILE:-$ROOT/.env.mainnet.local}"
LIVE_E2E_API_PID=""

usage() {
  cat <<'EOF'
Usage:
  scripts/e2e-live-mainnet.sh shape
  ENV_FILE=.env.mainnet.local scripts/e2e-live-mainnet.sh preflight
  ENV_FILE=.env.mainnet.local scripts/e2e-live-mainnet.sh burn

Modes:
  shape      Safe local check: builds live e2e commands and runs dry-run handoff validation.
  preflight  Loads .env.mainnet.local, verifies production config/contracts/OpenAI, builds apps.
  burn       Loads .env.mainnet.local and executes one real, explicitly-confirmed burn.

The burn mode requires:
  LIVE_E2E_CONFIRM=burn-real-funds
  LIVE_E2E_USER_WALLET=0x...
  LIVE_E2E_CHAIN=ethereum|polygon
  LIVE_E2E_TOKEN_SYMBOL=USDC|USDT
  LIVE_E2E_AMOUNT=<smallest token units>
  LIVE_E2E_API_BASE=<deployed API URL> to test a running deployment instead of a local API

Do not paste secrets into chat. Keep them in .env.mainnet.local, which is gitignored.
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "missing required env: $name" >&2
    exit 1
  fi
}

has_placeholder_value() {
  local value="$1"
  [[ "$value" == *"<"*">"* ]] && return 0
  [[ "$value" == "replace-with-a-long-random-secret" ]] && return 0
  [[ "$value" == "app.example.com" ]] && return 0
  [[ "$value" == "https://api.example.com" ]] && return 0
  [[ "$value" == *"example/rpc"* ]] && return 0
  return 1
}

env_errors=()

add_env_error() {
  env_errors+=("$1")
}

require_real_env() {
  local name="$1"
  local value="${!name:-}"
  if [[ -z "$value" ]]; then
    add_env_error "missing $name"
    return
  fi
  if has_placeholder_value "$value"; then
    add_env_error "placeholder $name"
  fi
}

flush_env_errors() {
  if (( ${#env_errors[@]} == 0 )); then
    return 0
  fi
  echo "live e2e env is not ready:" >&2
  local item
  for item in "${env_errors[@]}"; do
    echo "  - $item" >&2
  done
  echo "edit $ENV_FILE; do not paste secrets into chat" >&2
  exit 1
}

load_env_file() {
  if [[ ! -f "$ENV_FILE" ]]; then
    echo "missing env file: $ENV_FILE" >&2
    echo "create it from .env.mainnet.example and keep real secrets there" >&2
    exit 1
  fi
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
  if [[ "${ALLOW_DISABLED_ENFORCER:-}" != "" ]]; then
    echo "ALLOW_DISABLED_ENFORCER must be unset for live e2e" >&2
    exit 1
  fi
}

validate_live_env() {
  env_errors=()
  require_real_env HTTP_PORT
  require_real_env DATABASE_URL
  require_real_env JWT_SECRET
  require_real_env SIWE_DOMAIN
  require_real_env SESSION_TTL
  require_real_env SCHEDULER_INTERVAL
  require_real_env OPENAI_API_KEY
  require_real_env OPENAI_MODEL
  require_real_env OPENAI_TRANSCRIPTION_MODEL
  require_real_env ENFORCER_PRIVATE_KEY
  require_real_env VITE_API_BASE_URL
  require_real_env CHAINS_JSON

  if [[ -n "${CHAINS_JSON:-}" ]]; then
    local chain_errors
    chain_errors="$(
      node <<'NODE'
function placeholder(value) {
  return typeof value === "string" && (value.includes("<") || value.includes("example/rpc"));
}
try {
  const chains = JSON.parse(process.env.CHAINS_JSON);
  for (const key of ["ethereum", "polygon"]) {
    const item = chains[key];
    if (!item) {
      console.log(`missing CHAINS_JSON.${key}`);
      continue;
    }
    for (const field of ["rpc_url", "stake_enforcer_address"]) {
      if (!item[field]) console.log(`missing CHAINS_JSON.${key}.${field}`);
      else if (placeholder(item[field])) console.log(`placeholder CHAINS_JSON.${key}.${field}`);
    }
    for (const token of ["USDC", "USDT"]) {
      if (!item.tokens?.[token]) console.log(`missing CHAINS_JSON.${key}.tokens.${token}`);
    }
  }
} catch (error) {
  console.log(`invalid CHAINS_JSON: ${error.message}`);
}
NODE
    )"
    if [[ -n "$chain_errors" ]]; then
      local line
      while IFS= read -r line; do
        [[ -n "$line" ]] && add_env_error "$line"
      done <<<"$chain_errors"
    fi
  fi
  flush_env_errors
}

validate_burn_env() {
  env_errors=()
  require_real_env LIVE_E2E_USER_WALLET
  require_real_env LIVE_E2E_CHAIN
  require_real_env LIVE_E2E_TOKEN_SYMBOL
  require_real_env LIVE_E2E_AMOUNT
  require_real_env LIVE_E2E_MAX_AMOUNT
  if [[ "${LIVE_E2E_CONFIRM:-}" != "burn-real-funds" ]]; then
    add_env_error "LIVE_E2E_CONFIRM must be burn-real-funds"
  fi
  if [[ -n "${LIVE_E2E_CHAIN:-}" && ! "${LIVE_E2E_CHAIN}" =~ ^(ethereum|polygon)$ ]]; then
    add_env_error "LIVE_E2E_CHAIN must be ethereum or polygon"
  fi
  if [[ -n "${LIVE_E2E_TOKEN_SYMBOL:-}" && ! "${LIVE_E2E_TOKEN_SYMBOL}" =~ ^(USDC|USDT)$ ]]; then
    add_env_error "LIVE_E2E_TOKEN_SYMBOL must be USDC or USDT"
  fi
  if [[ -n "${LIVE_E2E_AMOUNT:-}" && ! "${LIVE_E2E_AMOUNT}" =~ ^[0-9]+$ ]]; then
    add_env_error "LIVE_E2E_AMOUNT must be a positive integer in token base units"
  elif [[ -n "${LIVE_E2E_AMOUNT:-}" && "${LIVE_E2E_AMOUNT}" == "0" ]]; then
    add_env_error "LIVE_E2E_AMOUNT must be greater than 0"
  fi
  if [[ -n "${LIVE_E2E_MAX_AMOUNT:-}" && ! "${LIVE_E2E_MAX_AMOUNT}" =~ ^[0-9]+$ ]]; then
    add_env_error "LIVE_E2E_MAX_AMOUNT must be a positive integer in token base units"
  elif [[ -n "${LIVE_E2E_MAX_AMOUNT:-}" && "${LIVE_E2E_MAX_AMOUNT}" == "0" ]]; then
    add_env_error "LIVE_E2E_MAX_AMOUNT must be greater than 0"
  fi
  if [[ -n "${LIVE_E2E_AMOUNT:-}" && -n "${LIVE_E2E_MAX_AMOUNT:-}" && "${LIVE_E2E_AMOUNT}" =~ ^[0-9]+$ && "${LIVE_E2E_MAX_AMOUNT}" =~ ^[0-9]+$ ]]; then
    if (( LIVE_E2E_AMOUNT > LIVE_E2E_MAX_AMOUNT )); then
      add_env_error "LIVE_E2E_AMOUNT exceeds LIVE_E2E_MAX_AMOUNT"
    fi
  fi
  if [[ -n "${LIVE_E2E_USER_WALLET:-}" && ! "${LIVE_E2E_USER_WALLET}" =~ ^0x[0-9a-fA-F]{40}$ ]]; then
    add_env_error "LIVE_E2E_USER_WALLET must be a 20-byte EVM address"
  fi
  flush_env_errors
}

derive_mainnet_env_from_chains_json() {
  require_env CHAINS_JSON
  local parsed
  parsed="$(
    node <<'NODE'
const chains = JSON.parse(process.env.CHAINS_JSON);
for (const key of ["ethereum", "polygon"]) {
  if (!chains[key]) throw new Error(`CHAINS_JSON missing ${key}`);
  const item = chains[key];
  for (const field of ["rpc_url", "stake_enforcer_address"]) {
    if (!item[field]) throw new Error(`CHAINS_JSON ${key}.${field} missing`);
  }
  console.log(`${key.toUpperCase().replace("-", "_")}_RPC_URL=${item.rpc_url}`);
  console.log(`${key.toUpperCase().replace("-", "_")}_STAKE_ENFORCER_ADDRESS=${item.stake_enforcer_address}`);
}
NODE
  )"
  while IFS='=' read -r name value; do
    export "$name=$value"
  done <<<"$parsed"
}

verify_live_contracts() {
  "$ROOT/scripts/verify-mainnet-deploy.sh"
}

run_shape() {
  require_cmd go
  require_cmd node
  require_cmd forge
  echo "== live e2e command builds =="
  (cd "$ROOT/backend" && go test ./cmd/livee2e ./cmd/seedapikey ./cmd/verifyopenai)

  echo "== mainnet handoff shape =="
  ETHEREUM_STAKE_ENFORCER_ADDRESS=0x1111111111111111111111111111111111111111 \
  POLYGON_STAKE_ENFORCER_ADDRESS=0x2222222222222222222222222222222222222222 \
  "$ROOT/scripts/verify-mainnet-deploy.sh" --dry-run

  echo "live e2e shape check passed"
}

run_preflight() {
  require_cmd go
  require_cmd node
  require_cmd npm
  require_cmd forge
  load_env_file
  validate_live_env
  derive_mainnet_env_from_chains_json

  echo "== backend live config + deployed StakeEnforcer validation =="
  verify_live_contracts

  echo "== live OpenAI verification =="
  (cd "$ROOT/backend" && go run ./cmd/verifyopenai)

  echo "== backend tests =="
  (cd "$ROOT/backend" && go test ./...)

  echo "== frontend tests/build =="
  (cd "$ROOT/frontend" && VITE_API_BASE_URL="$VITE_API_BASE_URL" npm test && VITE_API_BASE_URL="$VITE_API_BASE_URL" npm run build)

  echo "== web3 build/tests =="
  (cd "$ROOT/web3" && forge build && forge test --no-match-path test/StakeEnforcerFork.t.sol)
  "$ROOT/scripts/e2e-web3-fork.sh"

  if command -v gradle >/dev/null 2>&1; then
    echo "== android build/tests =="
    export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
    (cd "$ROOT/android-app" && gradle testDebugUnitTest assembleDebug)
  else
    echo "== android build/tests skipped: gradle not found =="
  fi

  echo "live mainnet preflight passed"
  echo "Next: approve LIVE_E2E_AMOUNT in MetaMask for the configured StakeEnforcer, then run:"
  echo "  ENV_FILE=$ENV_FILE scripts/e2e-live-mainnet.sh burn"
}

run_burn() {
  require_cmd go
  require_cmd node
  require_cmd curl
  load_env_file
  validate_live_env
  validate_burn_env
  derive_mainnet_env_from_chains_json

  echo "== deployed StakeEnforcer validation =="
  verify_live_contracts

  local api_base="${LIVE_E2E_API_BASE:-http://127.0.0.1:${HTTP_PORT}}"
  cleanup_api() {
    if [[ -n "$LIVE_E2E_API_PID" ]] && kill -0 "$LIVE_E2E_API_PID" >/dev/null 2>&1; then
      kill "$LIVE_E2E_API_PID" >/dev/null 2>&1 || true
      wait "$LIVE_E2E_API_PID" >/dev/null 2>&1 || true
    fi
  }
  trap cleanup_api EXIT

  local api_log
  if [[ -z "${LIVE_E2E_API_BASE:-}" ]]; then
    echo "== live local API server =="
    api_log="$(mktemp -t goalstakes-live-api.XXXXXX.log)"
    (cd "$ROOT/backend" && go run ./cmd/api >"$api_log" 2>&1) &
    LIVE_E2E_API_PID=$!
  else
    echo "== deployed API readiness =="
  fi
  for _ in {1..60}; do
    if curl -fsS "$api_base/api/v1/chains" >/dev/null 2>&1; then
      break
    fi
    if [[ -n "$LIVE_E2E_API_PID" ]] && ! kill -0 "$LIVE_E2E_API_PID" >/dev/null 2>&1; then
      echo "API exited before becoming ready. Log:" >&2
      sed -n '1,120p' "$api_log" >&2
      exit 1
    fi
    sleep 1
  done
  if ! curl -fsS "$api_base/api/v1/chains" >/dev/null 2>&1; then
    if [[ -n "$LIVE_E2E_API_PID" ]]; then
      echo "API did not become ready. Log:" >&2
      sed -n '1,120p' "$api_log" >&2
    else
      echo "deployed API did not become ready at $api_base" >&2
    fi
    exit 1
  fi

  echo "== temporary public API key =="
  local api_key
  api_key="$(cd "$ROOT/backend" && go run ./cmd/seedapikey)"
  if [[ ! "$api_key" =~ ^sk_ ]]; then
    echo "seeded API key has unexpected format" >&2
    exit 1
  fi

  echo "== real public API burn e2e =="
  echo "This will create a real avoid goal over HTTP and burn LIVE_E2E_AMOUNT from LIVE_E2E_USER_WALLET."
  API_BASE="$api_base" API_KEY="$api_key" node --input-type=module <<'NODE'
const base = process.env.API_BASE;
const apiKey = process.env.API_KEY;
const amount = process.env.LIVE_E2E_AMOUNT;
const chain = process.env.LIVE_E2E_CHAIN;
const tokenSymbol = process.env.LIVE_E2E_TOKEN_SYMBOL;

async function request(path, options = {}) {
  const response = await fetch(base + path, {
    method: options.method || "GET",
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });
  const text = await response.text();
  let body;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  if (!response.ok) {
    throw new Error(`${options.method || "GET"} ${path} failed ${response.status}: ${text}`);
  }
  return body;
}

const approval = await request(`/api/v1/approvals?chain=${encodeURIComponent(chain)}&token_symbol=${encodeURIComponent(tokenSymbol)}`);
if (BigInt(approval.allowance || "0") < BigInt(amount)) {
  throw new Error(`allowance ${approval.allowance} is below ${amount}; approve the StakeEnforcer in MetaMask first`);
}
const goal = await request("/api/v1/goals", {
  method: "POST",
  body: {
    title: `LIVE API E2E burn ${new Date().toISOString()}`,
    description: "Created by scripts/e2e-live-mainnet.sh burn through the public API.",
    type: "avoid",
    cadence: "daily",
    stake_amount: amount,
    token_symbol: tokenSymbol,
    chain,
    timezone: "UTC",
  },
});
const violation = await request(`/api/v1/goals/${goal.id}/violations`, {
  method: "POST",
  body: { reason: "live public API e2e explicit burn test" },
});
if (violation.status !== "charged" || !violation.tx_hash) {
  throw new Error(`violation was not charged with tx hash: ${JSON.stringify(violation)}`);
}
console.log(`live public API burn charged goal=${goal.id} violation=${violation.id} tx=${violation.tx_hash}`);
const keys = await request("/api/v1/apikeys");
const createdKey = keys.find((key) => key.prefix === apiKey.slice(0, 12));
if (createdKey) {
  await request(`/api/v1/apikeys/${createdKey.id}`, { method: "DELETE" });
}
NODE
}

case "$MODE" in
  shape) run_shape ;;
  preflight) run_preflight ;;
  burn) run_burn ;;
  -h|--help|help) usage ;;
  *)
    usage >&2
    exit 1
    ;;
esac
