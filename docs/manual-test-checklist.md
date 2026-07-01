# Manual Test Checklist

Use this checklist before marking work done. If any required check fails, fix the cause and rerun the failed check plus related regression checks.

Record proof in [manual-test-evidence.md](manual-test-evidence.md). Screenshot proof requires opening the image and writing a visual pass/fail judgment.

Use separate reviewers or subagents when possible:
- backend/API/web3
- web UX
- Android
- Telegram
- own-agent integration

## Job-Done Gate

1. Every in-scope module has completed checks.
2. No required check is silently skipped.
3. Every skipped check has a reason, risk, and follow-up.
4. At least one full verification run happened after the last code change.
5. No secret, private key, live API key, wallet seed phrase, Telegram token, or raw generated agent key is committed.
6. Every failed check led to a fix, not only a note.
7. The app still matches [../README.md](../README.md): arbitrary goals, crypto stake, burn-only penalty, backend-owned secrets, Telegram voice, own-agent skill links.

## Setup

1. Run `git status --short --branch` and record it.
2. Copy `.env.example` to `.env` if needed.
3. Export env with `set -a; source .env; set +a`.
4. Start Postgres with `docker compose up -d`.
5. Confirm Postgres with `docker compose exec -T postgres pg_isready -U goalstakes -d goalstakes`.
6. Confirm local tools:

```bash
go version
npm --version
forge --version
gradle --version
test -d "$HOME/Library/Android/sdk"
```

## Unit And Build

Run the single monorepo unit runner first:

```bash
scripts/run_unit_tests.sh
```

This runner is the source of truth for backend, frontend, Web3, Android JVM, and Telegram unit tests. It also fails if a unit test is added outside a `tests/` directory or if ad-hoc `scripts/e2e-*` files return.

### Backend

1. Confirm `scripts/run_unit_tests.sh` runs backend tests from `backend/tests/`.
2. Confirm tests cover config validation, SIWE, API keys, goals, approvals, check-ins, violations, scheduler, store, AI tools, Telegram links, agent links, and audio transcription boundaries.
3. Confirm raw API keys and raw agent secrets are never persisted.
4. Confirm revoked API keys, Telegram links, and agent links cannot authenticate.

### Frontend

1. Confirm `scripts/run_unit_tests.sh` runs frontend tests from `frontend/tests/` and builds the app.
2. Confirm tests cover API errors, chain selection, approval failures, stake parsing, voice input, Telegram link-code UI, and own-agent link UI.

### Web3

1. Confirm `scripts/run_unit_tests.sh` runs Web3 unit tests from `web3/tests/`.

```bash
cd web3
forge build
forge test
```

2. Confirm tests prove burn-only transfer, no custody, enforcer-only penalties, allowance failure, USDT-like behavior, false-return token failure, and ABI sync.
3. Run fork-local real token checks from the repo root:

```bash
web3/integration_test/run_e2e_tests.sh
```

4. Confirm the fork-local suite passes all four canonical token cases: Ethereum USDC, Ethereum USDT, Polygon USDC, and Polygon USDT.
5. Confirm the test output shows real fork RPC URLs and `0 failed`; do not accept mock-only, `SimulatedBackend`-only, or skipped fork output as Web3 acceptance.
6. If public RPC defaults are unavailable, rerun with real provider endpoints:

```bash
ETHEREUM_RPC_URL=https://... POLYGON_RPC_URL=https://... web3/integration_test/run_e2e_tests.sh
```

### Android

1. Confirm `scripts/run_unit_tests.sh` runs Android tests from `android-app/app/tests/` and builds the debug APK.
2. Confirm tests cover API settings, goals, check-ins, violations, progress, chat, voice, Telegram/agent link surfaces if present, readable errors, and 6-decimal stake conversion.

### Telegram Bot

1. Confirm `scripts/run_unit_tests.sh` runs Telegram tests from `telegram-bot/tests/`.
2. Confirm tests cover private chat, group, channel update shapes, link code, commands, free text, voice/audio file download, backend forwarding, no token leaks, and own-agent link generation.

## Integration

1. Run the full local suite:

```bash
integrations_tests/run_e2e_tests.sh
```

2. Confirm it runs backend service e2e, web3 fork-local real token checks, browser wallet/API/AI/Android-API e2e, Telegram fake Bot API e2e, own-agent cron e2e, mainnet shape check, Android emulator e2e, and secret-scan.
3. Confirm `.e2e/manual-web/` contains current desktop and mobile screenshots.
4. If any step fails, fix the root cause and rerun the full suite.

## Web Manual Checks

### Wallet And Approval

1. Open the web app.
2. Confirm the landing page explains stakes and burn risk.
3. Connect wallet.
4. Reject SIWE once and confirm a readable error.
5. Sign SIWE and confirm session starts.
6. Open approval flow.
7. Reject approval once and confirm no backend approval is recorded.
8. Approve stake and confirm spender is `StakeEnforcer`.
9. Confirm no frontend bundle contains AI keys, RPC secrets, private keys, JWT secrets, DB credentials, or Telegram tokens.

### Goals

1. Create daily do goal.
2. Create weekly do goal.
3. Create avoid goal.
4. Create goal with start date.
5. Create goal with end date.
6. Log check-in.
7. Report two avoid-goal violations and confirm both remain visible.
8. Edit title, stake, token, chain, start/end window.
9. Clear end date.
10. Archive goal.
11. Refresh and confirm persisted state.
12. Try empty title, invalid stake, unsupported chain/token, and insufficient allowance.

### Chat And Voice

1. Send text: `I did 10 push-ups`.
2. Confirm the assistant either records the matching check-in or asks a clarification.
3. Create a goal through chat.
4. Report a violation through chat.
5. Move stake through chat and confirm missing approval blocks it.
6. Use browser microphone.
7. Confirm transcript is sent through backend-owned AI/transcription boundaries.
8. Deny or disable microphone and confirm friendly error.

### Settings

1. Create API key and confirm raw key appears once.
2. Copy key, use it against `/api/v1/goals`, then revoke it.
3. Confirm revoked key returns `401`.
4. Generate Telegram link code.
5. Confirm the UI warns not to paste raw `sk_` keys into Telegram.
6. Click `Connect own agent`.
7. Confirm private `.md` link is generated.
8. Fetch the `.md` and confirm it contains project summary, API base, generated agent secret, API usage, safety rules, daily cron instruction, and revocation instruction.
9. Revoke the own-agent link and confirm link and agent secret no longer work.

## Android Manual Checks

1. Run emulator smoke:

```bash
android-app/integration_test/run_e2e_tests.sh
```

2. Open every screenshot under `.e2e/android-emulator/`.
3. Fail if text is clipped, controls overlap, IDs must be manually copied, loading is stuck, or connection errors appear.
4. Install debug APK on emulator.
5. Save API URL and API key.
6. Confirm settings persist after restart.
7. Create/edit/archive goals.
8. Check in and report violation.
9. Confirm progress and violation counts update.
10. Send chat text and voice transcript.
11. Deny voice input and confirm friendly fallback.
12. Click `Connect own agent` and verify private skill link output.
13. Rotate device and confirm layout remains usable.

## Telegram Manual Checks

1. Start backend and bot locally.
2. Confirm bot starts without logging token values.
3. Link private chat with backend link code.
4. Link group with backend link code.
5. Link channel with backend link code.
6. Confirm raw `sk_` API keys are not posted to Telegram.
7. Run commands in private chat:
   - `/goals`
   - `/create`
   - `/done`
   - `/violate`
   - `/progress`
   - `/archive`
8. Run the same command shape in group.
9. Run channel post command handling if channel commands are supported.
10. Send free text: `I did 10 push-ups`.
11. Send private/group voice saying `я отжался 10 раз`.
12. Send channel voice post saying `я отжался 10 раз`.
13. Confirm backend receives audio bytes, transcribes with backend AI key, and records check-in only when goal match is clear.
14. Confirm ambiguous voice text asks a clarification and does not mutate state.
15. Click or send `/agent`.
16. Confirm bot returns a private own-agent skill link through backend, not a raw long-lived user API key.
17. Run fake Telegram smoke:

```bash
telegram-bot/integration_test/run_e2e_tests.sh
```

## Own-Agent Checks

1. Generate own-agent link from web.
2. Generate own-agent link from Android.
3. Generate own-agent link from Telegram.
4. Fetch `/agent-skills/{token}.md`.
5. Confirm frontmatter contains `name` and `description`.
6. Confirm skill explains:
   - what Goal Stakes is
   - how stake burns work
   - API base URL
   - generated `sk_` agent secret
   - supported endpoints
   - user-secret handling
   - daily cron instruction
   - revocation
7. Use the generated agent secret to list goals.
8. Simulate daily cron with active goals and confirm reminder is sent.
9. Simulate daily cron with no active goals and confirm no reminder is sent.
10. Revoke the link.
11. Confirm old `.md` URL returns `404` or `410`.
12. Confirm old agent secret returns `401`.

## Penalty Checks

1. Create do-goal and miss a period.
2. Confirm scheduler writes pending violation before charge.
3. Confirm successful charge moves exact amount from user to burn address.
4. Confirm violation becomes `charged` with tx hash.
5. Force failed charge by removing allowance.
6. Confirm violation becomes `failed` and remains visible.
7. Report avoid-goal slip twice.
8. Confirm both reports create separate visible violations.
9. Confirm no penalty transfers to app, admin, treasury, or user-controlled destination.
10. Confirm `web3/integration_test/run_e2e_tests.sh` proves `StakeEnforcer.penalize` works against real forked USDC/USDT contracts on Ethereum and Polygon, not only mocks.

## Security Checks

1. Search built frontend for secret-shaped values.
2. Search logs for raw API keys, agent secrets, JWTs, private keys, Telegram tokens, and authorization headers.
3. Confirm backend rejects malformed JSON.
4. Confirm unknown route returns JSON `404`.
5. Confirm wrong method returns JSON `405`.
6. Confirm ownership checks block cross-user goal access.
7. Confirm mainnet config fails without `ENFORCER_PRIVATE_KEY` unless local dry-run explicitly enables `ALLOW_DISABLED_ENFORCER`.

## Mainnet Gate

1. Run shape check:

```bash
scripts/live_mainnet_gate.sh shape
```

2. Run preflight only with real `.env.mainnet.local` values. This includes the fork-local Web3 real-token check:

```bash
ENV_FILE=.env.mainnet.local scripts/live_mainnet_gate.sh preflight
```

3. Do not run live burn unless there is a written sacrificial-wallet plan.
4. If live burn is approved, record wallet, chain, token, allowance, amount, expected burn tx, and final balance evidence.

## Final Review

1. Run `git status --short --branch`.
2. Review changed docs for stale claims.
3. Review changed UI screenshots.
4. Review money-moving code paths.
5. Review auth, API-key, Telegram-link, and own-agent code paths.
6. Confirm [run.md](run.md) and [../README.md](../README.md) match the implementation.
7. Confirm every failed check was fixed and rerun.
