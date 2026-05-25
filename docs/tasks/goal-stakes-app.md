# Goal-Stakes App — staked goal tracking with AI manager and public API

**Status:** planning
**Branch:** main
**Worktree:** none
**Mode:** hands-off

## Original request

Build an app where people set personal goals and track progress, with monetary
stakes attached to failures.

- On launch, the user approves their USDT or USDC tokens in MetaMask (Ethereum / Polygon).
- Users define arbitrary goals: "Do 100 push-ups every day", "Don't drink soda",
  "Go to the gym every week".
- Each goal can carry a price, e.g. $100 per violation.
- Two screens: (1) a chat with an AI goal manager — a GPT-based LLM using function
  calling to manage everything via text ("Create a new push-up goal", "I did 10
  push-ups"); (2) the manual goals interface.
- Public API: account settings expose an API key; the API provides full functionality
  and ships with documentation.

(Scaffold present: empty `android-app/`, `backend/`, `frontend/`, `telegram-bot/`, `web3/`.)

## Design

### Purpose & scope

One product, several surfaces, all over a single Go backend that owns 100% of the
business logic. The user chose to build the whole system in one pass (no task split),
so this design covers every component; the **Plan** sequences them foundation-first
(PH-ordering in `## Plan`) so that if execution pauses, what's built is always coherent.

Components (monorepo, matching the existing scaffold):

- **`backend/` — Go.** The spine. Domain model, REST API, AI function-calling layer,
  web3 enforcement client, violation scheduler. Everything else is a client of it.
- **`web3/` — Solidity.** `StakeEnforcer` contract + tests + deploy scripts.
- **`frontend/` — React + TypeScript.** The two screens (chat + goals) plus wallet
  connect, token-approval flow, and account settings (API key).
- **`android-app/` — Kotlin.** Thin native client over the API (chat + goals + check-ins).
- **`telegram-bot/` — Go.** Thinnest client: chat with the AI manager + quick check-ins.
  Lowest priority (never described in the prompt; scaffolded only).

### How stakes work (the core mechanism)

1. User signs in with their wallet (SIWE / EIP-4361) — the wallet address *is* the account.
2. User calls ERC-20 `approve()` once, granting an allowance to the `StakeEnforcer`
   contract for their chosen token (USDT/USDC) on their chosen chain.
3. User creates goals, each with a stake amount + token.
4. On a **violation** (a "do" goal's period elapses with no "done" check-in, or the user
   self-reports breaking an "avoid" goal), the backend records a `Violation`, then calls
   `StakeEnforcer.penalize(user, token, amount)`.
5. The contract executes `token.transferFrom(user → BURN, amount)` in a single call.
   Funds never touch the platform; they go straight to an unrecoverable dead address.

The contract **hardcodes** the burn destination and restricts `penalize` to an authorized
enforcer role. This is the central security property: a compromised backend key can only
*burn* a user's staked funds (annoying but not theft) — it can never redirect them to an
attacker. This is why we use a contract rather than approving a bare backend EOA.

### Authentication & the public API

- **App auth:** SIWE. Server issues a nonce → client signs with MetaMask → server verifies
  and issues a session JWT. No email/password in v1 (the wallet is the identity).
- **Public API auth:** Bearer API key (`Authorization: Bearer sk_…`). Keys are generated
  and revoked from account settings, shown once, stored only as a hash + visible prefix.
- **Parity:** every state-changing capability is reachable three ways — REST endpoint, AI
  tool call, and the app UI — all routed through the *same* internal service layer (IV5).
- **Docs:** the public REST API ships an OpenAPI 3 spec served via Swagger UI at `/docs`.

### The AI goal manager

A `/chat` endpoint takes free text, calls the LLM (OpenAI GPT, function calling) with a
tool set that mirrors the domain service (`create_goal`, `log_check_in`, `report_violation`,
`list_goals`, `get_progress`, `set_stake`, `get_approval_status`, …). The LLM's tool calls
are executed against the same service the REST API uses, then results are summarized back to
the user. Conversations + messages are persisted. If no LLM key is configured, the chat
feature is disabled and the rest of the app runs normally (AS3).

### Core data model

`User`(wallet_address PK-ish, created_at) · `ApiKey`(user, key_hash, prefix, label, last_used_at, revoked_at)
· `Goal`(user, title, type[do|avoid], cadence[daily|weekly|custom], stake_amount, token, chain, status)
· `CheckIn`(goal, period, status[done|missed], reported_at, source[app|ai|api])
· `Violation`(goal, period, amount, token, chain, status[pending|charged|failed], tx_hash, charged_at)
· `WalletApproval`(user, token, chain, allowance, tx_hash, updated_at)
· `Conversation`(user) · `Message`(conversation, role, content, tool_calls, created_at).
Postgres, with migrations.

### Chosen approach & key tradeoffs

- **Monorepo, single backend owns all logic** vs. microservices: monorepo wins — matches
  the scaffold, keeps the AI/REST/UI parity invariant (IV5) trivial to uphold, far less ops.
- **SIWE-only auth** vs. email/password + wallet: SIWE wins for v1 — the wallet is mandatory
  anyway, so a second identity system is pure overhead. Logged as a hands-off default.
- **Allowance + burn contract** vs. escrow vault: per the user's choices. Soft commitment
  (a user can revoke the allowance to dodge — PC3 accepts this; it's a commitment device, not
  a vault). Burn destination removes all custody/regulatory surface.
- **Testnet-first**: Sepolia (ETH) + Polygon Amoy (POL) are the default deploy targets;
  mainnet addresses are config-only and gated on explicit user go-ahead (PC1, deferred item).

### Backwards compatibility

Greenfield — no existing consumers, no compat risks.

TDD: yes (backend domain/service logic, API auth + handlers, and the Solidity `StakeEnforcer`
contract — deterministic, reusable, regression-critical). Exception: React UI, the Kotlin app,
and live OpenAI calls use smoke / integration tests rather than unit TDD (visual or non-deterministic).

### Invariants
- IV1 — Forfeited stake funds can only ever be sent to the hardcoded burn address; no code path in the contract or backend can redirect a penalty transfer to any other recipient.
- IV2 — `StakeEnforcer` never takes custody: a penalty is a single `transferFrom(user → burn)` bounded by the user's current allowance; the contract holds no token balance between calls.
- IV3 — Raw API keys are never persisted or logged — only a one-way hash plus a non-secret prefix; the full key is shown to the user exactly once at creation.
- IV4 — A session is issued only after verifying an EIP-4361 signature from the claimed wallet against a server-issued, single-use nonce.
- IV5 — Every state-changing capability lives in the internal service layer and is exposed identically to the REST API and the AI tool layer; no business logic resides only in an HTTP handler or only in the AI glue.
- IV6 — A `Violation` row is written before any on-chain charge is attempted and updated with the tx outcome; a given (goal, period) yields at most one `charged` violation (idempotent).

### Principles
- PC1 — Chain operations default to testnet (Sepolia + Polygon Amoy); mainnet addresses are config-only and never the default.
- PC2 — The AI tool set is a thin adapter over the domain service: a new capability is added once in the service, then wired to REST + AI + clients — never reimplemented.
- PC3 — Self-reported progress is trusted by design (honor-system commitment device); the app does not try to objectively verify claims like "did 100 push-ups", and anti-gaming is out of scope.
- PC4 — Fail closed on money: if an on-chain charge cannot complete (allowance revoked, insufficient balance, RPC failure), record `failed` and surface it — never silently drop, and never retry in a way that could double-charge.

### Assumptions
- AS1 — Users authenticate with an Ethereum wallet (MetaMask); the wallet address is the primary identity (SIWE). No email/password system in v1.
- AS2 — "Burn" means an unrecoverable dead address (e.g. `0x…dEaD`), not `0x0`, because USDC and some tokens revert transfers to the zero address.
- AS3 — An OpenAI API key is provided at runtime for the AI manager; without it the chat feature is disabled but the rest of the app runs.
- AS4 — Violation detection is deadline-based for "do" goals (period elapses with no "done" check-in) and self-reported for "avoid" goals.
- AS5 — Testnet RPC endpoints and faucet-funded test tokens are available for development and verification.
- AS6 — USDT/USDC are standard-enough ERC-20s on the target chains; the app supports a fixed per-chain allowlist of token contracts, not arbitrary tokens.

### Unknowns
- UK1 — Exact Go router + OpenAPI tooling (e.g. chi + swaggo vs. spec-first) — settle in plan.
- UK2 — Backend signing model for `penalize`: a single platform enforcer key (operator-trust, bounded by IV1) vs. user-signed meta-transactions — v1 leans enforcer key; confirm in plan.
- UK3 — Final depth of the Kotlin and Telegram clients given the breadth — likely thin; settled per-phase in plan.
- UK4 — USDT's non-standard ERC-20 `approve` (no bool return; some deployments require resetting allowance to 0 before changing) vs. USDC — verify and handle in the contract/web3 phase.
- UK5 — Scheduler design for deadline-based violations (in-process ticker vs. external cron) and timezone handling — settle in plan.

## Plan
<empty — filled by up:uplan>

## Verify
<empty — filled by up:uverify>

## Conclusion
<empty — filled by up:ureview>

### Hands-off decisions
- size: Large — multi-component system (web3 staking, AI chat with function calling, public API, plus web/mobile/bot frontends scaffolded). Full Design → Plan → Execute → Verify → Review flow; Design preserved as the interactive stage.
- udesign: build everything in one task file (no split) — per the user's "all in one pass" answer; Plan phases it foundation-first so a pause still leaves coherent software.
- udesign: testnet default (Sepolia + Polygon Amoy), mainnet config-only — reversible, no real funds at risk during build/verify.
- udesign: SIWE chosen as the only v1 auth — the wallet is already mandatory, so a second email/password identity system would be pure overhead.
- udesign: OpenAI/GPT as the LLM provider — per the user's "GPT-based" wording; `OPENAI_API_KEY` supplied at runtime (AS3).
- udesign: `telegram-bot` kept as a thin, lowest-priority client — never described in the prompt, included only because scaffolded and the user chose "all in one pass".
- udesign: `StakeEnforcer` contract with hardcoded burn destination (vs. approving a bare backend EOA) — limits a compromised operator key to burning, never theft (IV1).

### Deferred (needs user input)
- Mainnet go-live with real USDT/USDC — v1 is built and verified on testnet only (PC1). Deploying to Ethereum/Polygon mainnet and pulling real funds needs an explicit user go-ahead and a security/legal review.
