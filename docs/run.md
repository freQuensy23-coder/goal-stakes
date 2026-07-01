# Runbook

This file is for running and verifying Goal Stakes locally. The product contract lives in [../README.md](../README.md).

## Local Stack

1. Start Postgres:

```bash
docker compose up -d
```

2. Create local env:

```bash
cp .env.example .env
set -a
source .env
set +a
```

Required backend env:
- `HTTP_PORT`
- `DATABASE_URL`
- `JWT_SECRET`
- `SIWE_DOMAIN`
- `SESSION_TTL`
- `SCHEDULER_INTERVAL`
- `CHAINS_JSON`

Optional backend env:
- `OPENAI_API_KEY`
- `OPENAI_MODEL`
- `OPENAI_TRANSCRIPTION_MODEL`
- `OPENAI_BASE_URL`
- `ENFORCER_PRIVATE_KEY`
- `ALLOW_DISABLED_ENFORCER=true` for local dry-run only. In this mode `POST /api/v1/approvals` still requires `tx_hash`, but may accept `dry_run_allowance` because the backend has no live allowance checker. Do not use this for mainnet.
- `TELEGRAM_BOT_SECRET` for `/internal/telegram/*`

3. Run backend:

```bash
cd backend
go run ./cmd/api
```

4. Run web:

```bash
cd frontend
npm install
npm run dev
```

Local URLs:
- API docs: `http://127.0.0.1:8080/docs`
- OpenAPI: `http://127.0.0.1:8080/openapi.json`
- Web: `http://127.0.0.1:5173`

## Android

Build and test:

```bash
cd android-app
ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}" gradle testDebugUnitTest assembleDebug
```

Run emulator smoke from the repo root:

```bash
scripts/e2e-android-emulator.sh
```

Open every screenshot under `.e2e/android-emulator/` before accepting Android UI work.

## Telegram Bot

The target bot links Telegram private chats, groups, and channels through backend link codes. Do not paste raw `sk_` API keys into Telegram.

Run tests:

```bash
cd telegram-bot
go test ./...
```

Run local fake Telegram smoke:

```bash
node scripts/e2e-telegram-bot.mjs
```

Run the bot:

```bash
TELEGRAM_BOT_TOKEN=123:abc \
TELEGRAM_BOT_SECRET=replace-with-shared-backend-secret \
GOALSTAKES_API_BASE=http://127.0.0.1:8080 \
go run ./cmd/telegram-bot
```

## Web3

Run contract tests:

```bash
cd web3
forge build
forge test --no-match-path test/StakeEnforcerFork.t.sol
```

Run fork-local real token checks against canonical Ethereum/Polygon USDC/USDT contracts:

```bash
scripts/e2e-web3-fork.sh
```

Public RPC defaults are provided for smoke checks. For acceptance or CI, use owned provider endpoints:

```bash
ETHEREUM_RPC_URL=https://... POLYGON_RPC_URL=https://... scripts/e2e-web3-fork.sh
```

Verify mainnet deployment config before real approvals:

```bash
ETHEREUM_STAKE_ENFORCER_ADDRESS=0x... \
POLYGON_STAKE_ENFORCER_ADDRESS=0x... \
ETHEREUM_RPC_URL=https://mainnet.infura.io/v3/<key> \
POLYGON_RPC_URL=https://polygon-mainnet.infura.io/v3/<key> \
ENFORCER_PRIVATE_KEY=0x... \
scripts/verify-mainnet-deploy.sh
```

## Verification

Run focused checks:

```bash
(cd backend && go test ./...)
(cd frontend && npm test && npm run build)
(cd web3 && forge test --no-match-path test/StakeEnforcerFork.t.sol)
scripts/e2e-web3-fork.sh
(cd android-app && ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}" gradle testDebugUnitTest assembleDebug)
(cd telegram-bot && go test ./...)
```

Run the full local suite from the repo root:

```bash
scripts/e2e-local.sh
```

Run Android UI smoke separately when Android UI or networking changes:

```bash
scripts/e2e-android-emulator.sh
```

After any UI smoke, open generated screenshots and record the visual judgment in [manual-test-evidence.md](manual-test-evidence.md).

## Mainnet Gate

Shape check:

```bash
scripts/e2e-live-mainnet.sh shape
```

Preflight with real values:

```bash
ENV_FILE=.env.mainnet.local scripts/e2e-live-mainnet.sh preflight
```

Live burn is destructive. Run it only with an explicit sacrificial-wallet plan:

```bash
ENV_FILE=.env.mainnet.local LIVE_E2E_CONFIRM=burn-real-funds scripts/e2e-live-mainnet.sh burn
```

Never run live burn with placeholder env values, shared wallets, or funds the user is not prepared to lose.
