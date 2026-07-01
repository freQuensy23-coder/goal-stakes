#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVIDENCE_DIR="$ROOT/.e2e/backend-admin"
mkdir -p "$EVIDENCE_DIR"
rm -f "$EVIDENCE_DIR"/*.log "$EVIDENCE_DIR"/*.json "$EVIDENCE_DIR"/*.txt

# shellcheck source=../../scripts/lib/test_support.sh
source "$ROOT/scripts/lib/test_support.sh"
ensure_forge_std "$ROOT"

API_PID=""
cleanup() {
  if [[ -n "$API_PID" ]] && kill -0 "$API_PID" >/dev/null 2>&1; then
    kill "$API_PID" >/dev/null 2>&1 || true
    wait "$API_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

free_port() {
  node <<'NODE'
const { createServer } = require("node:http");
const server = createServer();
server.listen(0, "127.0.0.1", () => {
  const address = server.address();
  console.log(address.port);
  server.close();
});
NODE
}

json_get() {
  local url="$1"
  local out="$2"
  curl -fsS "$url" -o "$out"
  node -e 'JSON.parse(require("node:fs").readFileSync(process.argv[1], "utf8"))' "$out"
}

assert_json_error() {
  local method="$1"
  local url="$2"
  local want_status="$3"
  local out="$4"
  local headers="$EVIDENCE_DIR/$(basename "$out" .json).headers.txt"
  local status
  status="$(curl -sS -X "$method" -D "$headers" -o "$out" -w '%{http_code}' "$url")"
  if [[ "$status" != "$want_status" ]]; then
    echo "$method $url returned $status, want $want_status" >&2
    cat "$out" >&2 || true
    exit 1
  fi
  node - "$out" <<'NODE'
const fs = require("node:fs");
const body = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
if (typeof body.error !== "string" || !body.error) {
  throw new Error(`missing JSON error body: ${JSON.stringify(body)}`);
}
NODE
  if ! grep -qi '^content-type: application/json' "$headers"; then
    echo "$method $url did not return application/json" >&2
    cat "$headers" >&2
    exit 1
  fi
}

expect_startup_failure() {
  local name="$1"
  local expected="$2"
  shift 2
  local log="$EVIDENCE_DIR/${name}.log"
  (
    cd "$ROOT/backend"
    env -i PATH="$PATH" HOME="$HOME" GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}" "$@" go run ./cmd/api
  ) >"$log" 2>&1 &
  local pid=$!
  for _ in {1..80}; do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      set +e
      wait "$pid"
      local rc=$?
      set -e
      if [[ "$rc" == "0" ]]; then
        echo "$name startup unexpectedly succeeded" >&2
        cat "$log" >&2
        exit 1
      fi
      if ! grep -q "$expected" "$log"; then
        echo "$name startup failed without expected text: $expected" >&2
        cat "$log" >&2
        exit 1
      fi
      return 0
    fi
    sleep 0.25
  done
  kill "$pid" >/dev/null 2>&1 || true
  wait "$pid" >/dev/null 2>&1 || true
  echo "$name startup kept running; expected failure" >&2
  cat "$log" >&2 || true
  exit 1
}

CHAIN_JSON='{"sepolia":{"rpc_url":"https://sepolia.example/rpc","stake_enforcer_address":"0x1111111111111111111111111111111111111111","tokens":{"USDC":"0x2222222222222222222222222222222222222222","USDT":"0x3333333333333333333333333333333333333333"}},"polygon-amoy":{"rpc_url":"https://amoy.example/rpc","stake_enforcer_address":"0x4444444444444444444444444444444444444444","tokens":{"USDC":"0x5555555555555555555555555555555555555555","USDT":"0x6666666666666666666666666666666666666666"}}}'
MAINNET_CHAIN_JSON='{"ethereum":{"rpc_url":"https://mainnet.example/rpc","stake_enforcer_address":"0x1111111111111111111111111111111111111111","tokens":{"USDC":"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48","USDT":"0xdAC17F958D2ee523a2206206994597C13D831ec7"}}}'
JWT_SECRET_SENTINEL="admin-smoke-jwt-secret-should-not-log"
AUTH_SENTINEL="sk_admin_smoke_should_not_log"
DATABASE_URL="${DATABASE_URL:-postgres://goalstakes:goalstakes@localhost:5433/goalstakes?sslmode=disable}"
API_PORT="$(free_port)"
API_BASE="http://127.0.0.1:${API_PORT}"

echo "== backend dependency warmup =="
(cd "$ROOT/backend" && go mod download)

echo "== backend admin services =="
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

echo "== backend valid startup =="
(
  cd "$ROOT/backend"
  HTTP_PORT="$API_PORT" \
  DATABASE_URL="$DATABASE_URL" \
  JWT_SECRET="$JWT_SECRET_SENTINEL" \
  SIWE_DOMAIN="127.0.0.1:5173" \
  SESSION_TTL="24h" \
  SCHEDULER_INTERVAL="1h" \
  OPENAI_API_KEY="" \
  OPENAI_MODEL="" \
  OPENAI_TRANSCRIPTION_MODEL="" \
  OPENAI_BASE_URL="" \
  ENFORCER_PRIVATE_KEY="" \
  ALLOW_DISABLED_ENFORCER="" \
  CHAINS_JSON="$CHAIN_JSON" \
  go run ./cmd/api
) >"$EVIDENCE_DIR/api.log" 2>&1 &
API_PID=$!

for i in {1..240}; do
  if curl -fsS "$API_BASE/api/v1/chains" >"$EVIDENCE_DIR/chains.json" 2>/dev/null; then
    break
  fi
  if ! kill -0 "$API_PID" >/dev/null 2>&1; then
    echo "backend exited before readiness" >&2
    cat "$EVIDENCE_DIR/api.log" >&2 || true
    exit 1
  fi
  if [[ "$i" == "240" ]]; then
    echo "backend did not become ready" >&2
    cat "$EVIDENCE_DIR/api.log" >&2 || true
    exit 1
  fi
  sleep 0.25
done

echo "== backend docs and health =="
json_get "$API_BASE/api/v1/chains" "$EVIDENCE_DIR/chains.json"
json_get "$API_BASE/openapi.json" "$EVIDENCE_DIR/openapi.json"
curl -fsS "$API_BASE/docs" -o "$EVIDENCE_DIR/docs.html"
grep -q "Goal Stakes API" "$EVIDENCE_DIR/docs.html"

echo "== backend migrations =="
(cd "$ROOT" && docker compose exec -T postgres psql -U goalstakes -d goalstakes -tAc "select max(version_id) from goose_db_version") >"$EVIDENCE_DIR/goose-version.txt"
expected_migration="$(
  find "$ROOT/backend/migrations" -maxdepth 1 -name '[0-9][0-9][0-9][0-9]_*.sql' -print |
    sed -E 's#^.*/([0-9]{4})_.*#\1#' |
    sort -n |
    tail -1 |
    sed -E 's/^0+//'
)"
if ! grep -Eq "^[[:space:]]*${expected_migration}[[:space:]]*$" "$EVIDENCE_DIR/goose-version.txt"; then
  echo "unexpected goose migration version:" >&2
  echo "expected $expected_migration" >&2
  cat "$EVIDENCE_DIR/goose-version.txt" >&2
  exit 1
fi

echo "== backend CORS and JSON fallback errors =="
cors_headers="$EVIDENCE_DIR/cors.headers.txt"
cors_status="$(curl -sS -X OPTIONS "$API_BASE/api/v1/goals" \
  -H "Origin: http://127.0.0.1:5173" \
  -H "Access-Control-Request-Method: GET" \
  -D "$cors_headers" -o /dev/null -w '%{http_code}')"
if [[ "$cors_status" != "204" ]] || ! grep -q "Access-Control-Allow-Origin: http://127.0.0.1:5173" "$cors_headers"; then
  echo "CORS preflight failed" >&2
  cat "$cors_headers" >&2
  exit 1
fi
assert_json_error GET "$API_BASE/nope" 404 "$EVIDENCE_DIR/not-found.json"
assert_json_error POST "$API_BASE/api/v1/chains" 405 "$EVIDENCE_DIR/method-not-allowed.json"
assert_json_error GET "$API_BASE/api/v1/goals" 401 "$EVIDENCE_DIR/unauthorized.json"
curl -sS -H "Authorization: Bearer $AUTH_SENTINEL" "$API_BASE/api/v1/goals" -o "$EVIDENCE_DIR/unauthorized-with-token.json" -w '%{http_code}' | grep -q '^401$'

echo "== backend startup failure guards =="
expect_startup_failure "missing-jwt" "JWT_SECRET" \
  HTTP_PORT="$(free_port)" \
  DATABASE_URL="$DATABASE_URL" \
  SIWE_DOMAIN="127.0.0.1:5173" \
  SESSION_TTL="24h" \
  SCHEDULER_INTERVAL="1h" \
  CHAINS_JSON="$CHAIN_JSON"
expect_startup_failure "invalid-chains-json" "CHAINS_JSON is not valid JSON" \
  HTTP_PORT="$(free_port)" \
  DATABASE_URL="$DATABASE_URL" \
  JWT_SECRET="admin-smoke-jwt" \
  SIWE_DOMAIN="127.0.0.1:5173" \
  SESSION_TTL="24h" \
  SCHEDULER_INTERVAL="1h" \
  CHAINS_JSON="{bad"
expect_startup_failure "mainnet-no-enforcer" "ENFORCER_PRIVATE_KEY is required" \
  HTTP_PORT="$(free_port)" \
  DATABASE_URL="$DATABASE_URL" \
  JWT_SECRET="admin-smoke-jwt" \
  SIWE_DOMAIN="127.0.0.1:5173" \
  SESSION_TTL="24h" \
  SCHEDULER_INTERVAL="1h" \
  CHAINS_JSON="$MAINNET_CHAIN_JSON"

echo "== backend log secrecy =="
sleep 0.5
if grep -Eq "$JWT_SECRET_SENTINEL|$AUTH_SENTINEL|Authorization" "$EVIDENCE_DIR/api.log"; then
  echo "backend logs leaked a secret-shaped value" >&2
  cat "$EVIDENCE_DIR/api.log" >&2
  exit 1
fi

echo "== backend web3 simulated e2e =="
(cd "$ROOT/web3" && forge build)
(cd "$ROOT/backend" && go test -tags e2e ./integration_test/... -count=1)

echo "backend e2e passed"
