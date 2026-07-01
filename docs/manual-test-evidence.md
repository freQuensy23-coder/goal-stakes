# Manual Test Evidence

Record fresh proof here after running the checklist. Do not keep old pass claims after the code or spec changes.

## 2026-07-01 Backend Audio Chat Task

### Run Context

- Date/time: 2026-07-01 01:11:31 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `build/goal-stakes-app`
- Commit: `887a339`
- Environment: local backend/frontend tests, fake OpenAI e2e server, browser wallet e2e harness
- Tester/agent: Codex

### Commands

- Command: `cd backend && go test ./...`
- Result: pass
- Relevant output: all backend packages passed, including `internal/ai`, `internal/api`, and `internal/config`
- Fix applied after failure: added authenticated `/api/v1/chat/audio`, injected transcription boundary, explicit `OPENAI_TRANSCRIPTION_MODEL`, multipart validation, and OpenAPI docs
- Rerun result: pass

- Command: `cd frontend && npm test`
- Result: pass
- Relevant output: 7 test files passed, 17 tests passed
- Fix applied after failure: added `ApiClient.chatAudio` FormData method and response type
- Rerun result: pass

- Command: `cd frontend && npm run build`
- Result: pass
- Relevant output: TypeScript and Vite production build completed
- Fix applied after failure: none after final run
- Rerun result: pass

- Command: `node integrations_tests/web_wallet_e2e.mjs`
- Result: pass
- Relevant output: `web wallet e2e passed`
- Fix applied after failure: fake OpenAI server now handles `/v1/audio/transcriptions`; e2e calls `/api/v1/chat/audio` with multipart audio and verifies transcript/reply/conversation id
- Rerun result: pass

- Command: `rg -n "OPENAI_API_KEY|OPENAI_TRANSCRIPTION_MODEL|OPENAI_MODEL|sk-e2e|whisper-e2e" frontend/src frontend/dist android-app/app/src`
- Result: pass
- Relevant output: no matches
- Fix applied after failure: none
- Rerun result: pass

### Screenshot Review

- File path: `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/chat-voice-desktop.png`
- Opened with: `view_image`
- What was visually checked: chat screen after voice checks; sidebar, message list, input, microphone button, send button, and wallet chip are visible with no clipped text or overlapping controls
- Result: pass
- Fix applied after failure: none

### Checklist Results

- Setup: partial, e2e harness started required local services
- Unit and build: pass for backend and frontend scope
- Integration: pass for web-wallet e2e audio chat coverage
- Web: partial pass for chat/voice screenshot review
- Android: not run for this backend/API task
- Telegram: not run for this backend/API task
- Own agent: not in scope for that checkpoint
- Penalties: covered only by existing backend/web e2e regression path
- Security: partial, no AI key added to frontend or Android source
- Mainnet dry run: not run

### Unrun Checks

- Check: full `integrations_tests/run_e2e_tests.sh`
- Reason: current task changed backend audio API and frontend API client only; full suite is reserved for broader integration checkpoints after dependent Telegram/agent tasks land
- Risk: Android, Telegram, web3, and live-shape regressions could still exist outside this task
- Required follow-up: run full local suite before final goal completion

### Final Decision

- All required checks passed: no, only backend audio chat task scope is verified
- Known failures: none in the commands above
- Ready to launch: no, remaining task backlog is still open

## 2026-07-01 Telegram Link Codes Task

### Run Context

- Date/time: 2026-07-01 01:28:06 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `build/goal-stakes-app`
- Commit: `887a339`
- Environment: local backend/frontend/Telegram tests, fake web-wallet e2e, fake Telegram Bot API e2e
- Tester/agent: Codex

### Commands

- Command: `cd backend && go test ./...`
- Result: pass
- Relevant output: all backend packages passed, including migration-backed web e2e coverage from later command
- Fix applied after failure: added Telegram link-code domain/store/service/API, `TELEGRAM_BOT_SECRET`, internal `/internal/telegram/link`, migration `0003_telegram_links.sql`, and OpenAPI/docs
- Rerun result: pass

- Command: `cd frontend && npm test`
- Result: pass
- Relevant output: 7 test files passed, 18 tests passed
- Fix applied after failure: added `ApiClient.createTelegramLinkCode`
- Rerun result: pass

- Command: `cd frontend && npm run build`
- Result: pass
- Relevant output: TypeScript and Vite production build completed
- Fix applied after failure: none after final run
- Rerun result: pass

- Command: `cd telegram-bot && go test ./...`
- Result: pass
- Relevant output: bot, Goal Stakes client, and Telegram client packages passed
- Fix applied after failure: removed bot-side raw API key storage and `/apikey sk_...` supported flow; added `/link code` and backend internal link call
- Rerun result: pass

- Command: `node integrations_tests/web_wallet_e2e.mjs`
- Result: pass
- Relevant output: `web wallet e2e passed`
- Fix applied after failure: web Settings now generates Telegram link code and e2e verifies it is not an `sk_` key
- Rerun result: pass

- Command: `telegram-bot/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: `telegram bot e2e passed`
- Fix applied after failure: fake Telegram e2e now uses `/link code`, bot secret, and `/internal/telegram/link`
- Rerun result: pass

- Command: `rg -n "/apikey sk_|API key linked|Link your Goal Stakes API key|keys\\s+map|apiKeyForChat" telegram-bot`
- Result: pass
- Relevant output: no matches
- Fix applied after failure: removed remaining literal from negative test
- Rerun result: pass

### Screenshot Review

- File path: `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/settings-api-key-created.png`
- Opened with: `view_image`
- What was visually checked: Settings page after API key and Telegram link-code creation; Telegram panel, code, copy button, expiry text, API key panel, approval panel, and docs link are visible with no clipped text or overlapping controls
- Result: pass
- Fix applied after failure: none

### Checklist Results

- Setup: partial, e2e harness started required local services
- Unit and build: pass for backend, frontend, and Telegram bot scope
- Integration: pass for web-wallet e2e and fake Telegram link-code e2e
- Web: pass for Settings link-code surface in desktop screenshot
- Android: not run for this task
- Telegram: pass for fake private-chat link-code flow; group/channel text links still need internal gateway task coverage
- Own agent: not in scope for that checkpoint
- Penalties: existing regressions stayed green in backend/web e2e
- Security: pass for no supported raw `/apikey sk_...` flow and no bot-side user API-key map
- Mainnet dry run: not run

### Unrun Checks

- Check: full private/group/channel Telegram command execution
- Reason: belongs to `telegram-internal-gateway.md`, which is still in progress backlog
- Risk: linked chats cannot execute goal commands until internal gateway task is complete
- Required follow-up: implement and test `/internal/telegram/message`

### Final Decision

- All required checks passed: no, only Telegram link-code task scope is verified
- Known failures: goal-command execution moved to next task and is not complete
- Ready to launch: no, remaining task backlog is still open

## 2026-07-01 Telegram Internal Gateway Task

### Run Context

- Date/time: 2026-07-01 01:41:16 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `build/goal-stakes-app`
- Commit: `887a339`
- Environment: local backend tests, local Telegram bot tests, fake Telegram Bot API e2e, fake Goal Stakes internal API e2e
- Tester/agent: Codex

### Commands

- Command: `cd backend && go test ./...`
- Result: pass
- Relevant output: all backend packages passed, including `internal/api` coverage for `/internal/telegram/message`
- Fix applied after failure: added backend-owned Telegram command parser for `/goals`, `/create`, `/done`, `/violate`, `/progress`, `/archive`, free-text AI forwarding, and OpenAPI/docs for the internal message endpoint
- Rerun result: pass

- Command: `cd telegram-bot && go test ./...`
- Result: pass
- Relevant output: bot handler, Goal Stakes client, and Telegram client packages passed
- Fix applied after failure: handler now forwards non-link text to `/internal/telegram/message`; Telegram client parses `message_id`, private/group `message`, and `channel_post`
- Rerun result: pass

- Command: `telegram-bot/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: `telegram bot e2e passed`
- Fix applied after failure: fake e2e now covers `/link`, private commands, group forwarding, channel post forwarding, free text, and secret-leak checks through internal backend endpoints only
- Rerun result: pass

### Screenshot Review

- File path: none
- Opened with: not applicable
- What was visually checked: no web or Android UI changed in this task; Telegram text replies were checked from fake e2e response payloads
- Result: pass for non-visual transport scope
- Fix applied after failure: none after final run

### Checklist Results

- Setup: partial, fake Telegram and fake backend services started by e2e harness
- Unit and build: pass for backend and Telegram bot scope
- Integration: pass for fake Telegram text gateway e2e
- Web: not changed in this task
- Android: not changed in this task
- Telegram: pass for private command flow, group message forwarding, channel post forwarding, and free text forwarding
- Own agent: not in scope for that checkpoint
- Penalties: existing backend regressions stayed green; Telegram violation command writes through service layer without a live charger in this local task
- Security: pass for bot-secret-only internal calls and no raw API-key flow in bot e2e logs/replies
- Mainnet dry run: not run

### Unrun Checks

- Check: Telegram voice/audio
- Reason: belonged to the later `telegram-voice-audio.md` task and was not in scope for that checkpoint
- Risk: voice updates are still ignored until the next task lands
- Required follow-up: implement `voice`/`audio` update parsing, Telegram file download, and `/internal/telegram/audio`

### Final Decision

- All required checks passed: no, only Telegram internal text gateway task scope is verified
- Known failures: voice/audio Telegram and own-agent tasks remain open
- Ready to launch: no, remaining task backlog is still open

## 2026-07-01 Telegram Voice Audio Task

### Run Context

- Date/time: 2026-07-01 01:49:23 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `build/goal-stakes-app`
- Commit: `887a339`
- Environment: local backend tests, local Telegram bot tests, fake Telegram Bot API e2e, fake Goal Stakes internal API e2e
- Tester/agent: Codex

### Commands

- Command: `cd backend && go test ./...`
- Result: pass
- Relevant output: all backend packages passed, including linked channel multipart `/internal/telegram/audio` and unlinked no-transcription tests
- Fix applied after failure: added internal Telegram audio endpoint, multipart/binary parsing, chat-link resolution before transcription, backend AI transcription flow, and OpenAPI/docs
- Rerun result: pass

- Command: `cd telegram-bot && go test ./...`
- Result: pass
- Relevant output: bot handler, Goal Stakes client, and Telegram client packages passed
- Fix applied after failure: added `voice`/`audio` update structs, `getFile`, token-safe file download, multipart backend upload, and handler audio forwarding
- Rerun result: pass

- Command: `telegram-bot/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: `telegram bot e2e passed`
- Fix applied after failure: fake Telegram e2e now includes `channel_post.voice(file_id=voice-file-id)`, `getFile`, file download, backend `/internal/telegram/audio`, transcript `я отжался 10 раз`, and reply `Записал: 10 отжиманий`
- Rerun result: pass

### Screenshot Review

- File path: none
- Opened with: not applicable
- What was visually checked: no visual UI changed in this task; manual validation used fake Telegram request/reply payload inspection for the channel voice flow
- Result: pass for non-visual Telegram transport scope
- Fix applied after failure: none after final run

### Checklist Results

- Setup: partial, fake Telegram and fake backend services started by e2e harness
- Unit and build: pass for backend and Telegram bot scope
- Integration: pass for fake Telegram channel voice e2e
- Web: not changed in this task
- Android: not changed in this task
- Telegram: pass for private/group/channel text regression and linked channel voice/audio path
- Own agent: not in scope for that checkpoint
- Penalties: existing backend regressions stayed green; voice AI tool mutation is covered through backend `ChatAudio` plumbing with fake transcript
- Security: pass for no Telegram token, bot secret, or raw API key in fake e2e logs/replies; bot does not log file URLs containing the token
- Mainnet dry run: not run

### Unrun Checks

- Check: real Telegram network and real OpenAI transcription
- Reason: local task uses fake Telegram API and fake transcription per unit/e2e contract
- Risk: provider-specific media formats or live Telegram permissions could still fail outside fake e2e
- Required follow-up: run live staging Telegram bot with a real linked channel before production launch

### Final Decision

- All required checks passed: no, only Telegram voice/audio task scope is verified
- Known failures: own-agent and approval-recording tasks remain open
- Ready to launch: no, remaining task backlog is still open

## 2026-07-01 Own Agent Skill Links Task

### Run Context

- Date/time: 2026-07-01 01:58:10 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `build/goal-stakes-app`
- Commit: `887a339`
- Environment: local backend unit/router tests with in-memory store and migration compile coverage
- Tester/agent: Codex

### Commands

- Command: `cd backend && go test ./...`
- Result: pass
- Relevant output: all backend packages passed, including service/router tests for agent-link create/list/fetch/revoke, generated Markdown, expiration, and API-key revocation
- Fix applied after failure: added `agent_links` migration/store/domain/service, deterministic non-persisted agent secret derivation, private `/agent-skills/{token}.md`, authenticated `/api/v1/agent-links`, and OpenAPI/docs
- Rerun result: pass

### Screenshot Review

- File path: none
- Opened with: not applicable
- What was visually checked: no visual UI changed in this backend task
- Result: pass for backend/API scope
- Fix applied after failure: none after final run

### Checklist Results

- Setup: backend in-memory test harness
- Unit and build: pass for backend scope
- Integration: pass for router lifecycle test using generated agent secret against authenticated API
- Web: not changed in this task
- Android: not changed in this task
- Telegram: not changed in this task
- Own agent: pass for backend private skill-link generation, Markdown content, cron instructions, and revocation
- Penalties: existing backend regressions stayed green
- Security: pass for no raw agent secret in create/list responses; raw secret appears only in fetched private Markdown; revoked secret returns `401`
- Mainnet dry run: not run

### Unrun Checks

- Check: Web, Android, and Telegram user-facing `Connect own agent` entrypoints
- Reason: belonged to the later `own-agent-client-entrypoints.md` task and was not in scope for that checkpoint
- Risk: backend contract works but users cannot create links from all required clients yet
- Required follow-up: implement client entrypoints and rerun UI/manual checks

### Final Decision

- All required checks passed: no, only backend own-agent skill-link task scope is verified
- Known failures: client entrypoints, daily reminder fake-agent script, and approval-recording tasks remain open
- Ready to launch: no, remaining task backlog is still open

## Run Context

## 2026-07-01 Own Agent Client Entrypoints Task

### Run Context

- Date/time: 2026-07-01 02:14:53 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `build/goal-stakes-app`
- Commit: `887a339`
- Environment: local frontend unit/build, backend/router tests, Telegram fake e2e, Android JVM tests, Android emulator smoke API
- Tester/agent: Codex

### Commands

- Command: `cd frontend && npm test -- --run && npm run build`
- Result: pass
- Relevant output: 7 frontend test files and 19 tests passed; Vite production build completed
- Fix applied after failure: added agent-link API methods, Settings UI create/list/revoke state, and web e2e checks for generated private skill URL and revocation
- Rerun result: pass

- Command: `cd backend && go test ./...`
- Result: pass
- Relevant output: all backend packages passed, including `/internal/telegram/agent-link` router coverage
- Fix applied after failure: added internal Telegram agent-link endpoint and OpenAPI/docs coverage
- Rerun result: pass

- Command: `cd telegram-bot && go test ./...`
- Result: pass
- Relevant output: bot handler and Goal Stakes client tests passed, including `/agent`
- Fix applied after failure: added `/agent` command path that calls backend with `TELEGRAM_BOT_SECRET`
- Rerun result: pass

- Command: `telegram-bot/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: `telegram bot e2e passed`
- Fix applied after failure: fake Telegram e2e now sends `/agent`, fake backend receives `/internal/telegram/agent-link`, and bot replies with the private skill URL
- Rerun result: pass

- Command: `node integrations_tests/web_wallet_e2e.mjs`
- Result: pass
- Relevant output: web wallet e2e passed, including Settings `Connect own agent`, private Markdown fetch, generated agent secret API call, list response secret redaction, revoke, and post-revoke `401`
- Fix applied after failure: added web Settings own-agent panel and e2e assertions that the UI URL does not expose `sk_`
- Rerun result: pass

- Command: `cd android-app && ANDROID_HOME="$HOME/Library/Android/sdk" gradle testDebugUnitTest`
- Result: pass
- Relevant output: Android JVM tests passed, including `ApiClient.createAgentLink`
- Fix applied after failure: `./gradlew` was not present and Gradle required `ANDROID_HOME`; reran with installed `gradle` and explicit `ANDROID_HOME`
- Rerun result: pass

- Command: `ANDROID_HOME="$HOME/Library/Android/sdk" android-app/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: `android emulator e2e passed`
- Fix applied after failure: after scrolling to the own-agent section, the invalid-URL test could not find the API URL field; added a scroll back toward the API connection section before editing the URL
- Rerun result: pass

### Screenshot Review

- File path: `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/settings-api-key-created.png`
- Opened with: `view_image`
- What was visually checked: desktop Settings shows API key panel, Telegram link-code panel, Own agent panel, generated private agent URL, and copy controls with no clipped text or overlapping controls
- Result: pass
- Fix applied after failure: none after final run

- File path: `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/settings-mobile.png`
- Opened with: `view_image`
- What was visually checked: mobile Settings shows the Own agent panel and `Connect own agent` control with readable labels and no horizontal overflow or overlap
- Result: pass
- Fix applied after failure: none after final run

- File path: `/Users/a.mametyev/PycharmProjects/target-app/.e2e/android-emulator/settings-agent.png`
- Opened with: `view_image` after downscaling copy to `/Users/a.mametyev/PycharmProjects/target-app/.e2e/android-emulator/settings-agent-small.png`
- What was visually checked: Android portrait Settings shows `Own agent`, generated URL `http://10.0.2.2:18080/agent-skills/agt_android.md`, `Connect own agent`, `Copy latest link`, and money-safety card with no clipped text or overlapping controls
- Result: pass
- Fix applied after failure: none after final run

### Checklist Results

- Setup: pass for local fake services and Android emulator harness
- Unit and build: pass for frontend, backend, Telegram bot, and Android JVM tests in this task scope
- Integration: pass for web wallet e2e, Telegram fake e2e, and Android emulator e2e
- Web: pass for own-agent create/fetch/use/list/revoke flow and manual desktop/mobile Settings screenshots
- Android: pass for own-agent link generation/display and manual portrait screenshot
- Telegram: pass for linked-chat `/agent` flow
- Own agent: pass for user-facing entrypoints across web, Android, and Telegram; private URL does not expose raw `sk_`; fetched private Markdown contains the generated agent secret; revocation disables the generated secret
- Penalties: not changed in this task
- Security: pass for no raw agent secret in create/list UI/API responses; raw secret appears only in the private Markdown fetched through the generated link
- Mainnet dry run: not run

### Unrun Checks

- Check: Android landscape own-agent panel after link generation
- Reason: emulator e2e captures landscape for the app shell after full flow, but not specifically while scrolled to generated own-agent link
- Risk: low; Android portrait is verified for the new controls and existing landscape smoke remains captured
- Required follow-up: include own-agent landscape screenshot in the final full manual verification pass

### Final Decision

- All required checks passed: yes for own-agent client entrypoint task scope
- Known failures: approval-recording and final API/e2e coverage tasks remain open
- Ready to launch: no, remaining task backlog is still open

## Run Context

## 2026-07-01 Approval Recording Contract Task

### Run Context

- Date/time: 2026-07-01 02:21:39 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `build/goal-stakes-app`
- Commit: `887a339`
- Environment: local backend tests, frontend unit/build, browser wallet e2e with disabled live enforcer and explicit dry-run allowance
- Tester/agent: Codex

### Commands

- Command: `cd backend && go test ./...`
- Result: pass
- Relevant output: all backend packages passed after adding service/router coverage for `tx_hash` and dry-run approval behavior
- Fix applied after failure: first backend run failed because router test expected only `tx_hash is required`; actual JSON error included `service: invalid input: tx_hash is required`. Updated the test to assert the documented full error body
- Rerun result: pass

- Command: `cd frontend && npm test -- --run`
- Result: pass
- Relevant output: 7 frontend test files and 20 tests passed, including approval serialization with `tx_hash` and no legacy `allowance`
- Fix applied after failure: none
- Rerun result: not needed

- Command: `cd frontend && npm run build`
- Result: pass
- Relevant output: TypeScript check and Vite production build completed
- Fix applied after failure: none
- Rerun result: not needed

- Command: `node integrations_tests/web_wallet_e2e.mjs`
- Result: pass
- Relevant output: `web wallet e2e passed`
- Fix applied after failure: first e2e assertion expected `tx_hash is required` for a legacy payload containing `allowance`; decoder rejects unknown legacy `allowance` as `invalid json`. Updated e2e to assert both behaviors: legacy `allowance` is rejected as invalid JSON, and the new dry-run shape without `tx_hash` returns `tx_hash is required`
- Rerun result: pass

### Screenshot Review

- File path: `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/approval-gate-desktop.png`
- Opened with: `view_image` after downscaling copy to `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/approval-gate-desktop-small.png`
- What was visually checked: desktop approval gate shows chain selector, token selector, amount input, and `Approve and continue` button with no clipped text or overlapping controls
- Result: pass
- Fix applied after failure: none

- File path: `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/approval-reverted-desktop.png`
- Opened with: `view_image` after downscaling copy to `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/approval-reverted-desktop-small.png`
- What was visually checked: reverted approval error is visible/readable, form remains usable, and no controls overlap after the error state appears
- Result: pass
- Fix applied after failure: none

### Checklist Results

- Setup: pass for local backend/frontend/browser e2e harness
- Unit and build: pass for backend and frontend
- Integration: pass for browser wallet e2e, including rejected approval transaction and successful approval recording
- Web: pass for approval gate and reverted transaction state
- Android: not changed in this task
- Telegram: not changed in this task
- Own agent: previous own-agent e2e remains green in the same web wallet script
- Penalties: unchanged; backend allowance enforcement regressions passed
- Security: pass for public approval contract rejecting legacy client-provided `allowance`; live checker ignores dry-run allowance; local dry-run behavior is explicitly documented
- Mainnet dry run: not run

### Unrun Checks

- Check: live RPC approval verification with real wallet transaction hash
- Reason: local e2e runs with `ALLOW_DISABLED_ENFORCER=true` and no `ENFORCER_PRIVATE_KEY`
- Risk: provider/network-specific RPC issues could still fail in staging/mainnet
- Required follow-up: run `scripts/live_mainnet_gate.sh` or staging equivalent with `ENFORCER_PRIVATE_KEY` and real RPC before production launch

### Final Decision

- All required checks passed: yes for approval-recording contract task scope
- Known failures at this checkpoint: final API docs/e2e coverage task was still open; closed by the later API Docs And E2E Coverage Task section below
- Ready to launch at this checkpoint: no, remaining task backlog was still open

## Run Context

## 2026-07-01 Own Agent Daily Reminder Contract Task

### Run Context

- Date/time: 2026-07-01 02:25:46 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `build/goal-stakes-app`
- Commit: `887a339`
- Environment: local backend unit/router tests plus deterministic fake-agent HTTP script
- Tester/agent: Codex

### Commands

- Command: `cd backend && gofmt -w internal/api/router_test.go internal/service/service_test.go && go test ./...`
- Result: pass
- Relevant output: all backend packages passed; generated skill tests now assert timezone cron, active-goal reminder wording, and no reminder-side mutation
- Fix applied after failure: first command used backend-prefixed paths while already in `backend/`; reran with correct paths
- Rerun result: pass

- Command: `node integrations_tests/own_agent_cron_e2e.mjs`
- Result: pass
- Relevant output: `own-agent cron e2e passed`
- Fix applied after failure: added fake-agent script that fetches a private skill, extracts generated `sk_`, calls `GET /api/v1/goals`, sends a reminder for active unarchived goals only, sends nothing for empty goals, and gets `401` after revocation
- Rerun result: pass

- Command: `scripts/secret-scan.sh`
- Result: pass
- Relevant output: `secret scan passed`
- Fix applied after failure: none
- Rerun result: not needed

### Screenshot Review

- File path: none
- Opened with: not applicable
- What was visually checked: no UI changed in this task; behavior is an external-agent cron/API script
- Result: pass for non-visual scope
- Fix applied after failure: none

### Checklist Results

- Setup: pass for local fake-agent HTTP harness
- Unit and build: pass for backend tests in this task scope
- Integration: pass for fake-agent active/empty/revoked cron behavior
- Web: not changed in this task
- Android: not changed in this task
- Telegram: not changed in this task
- Own agent: pass for generated skill cron wording and usable external reminder contract
- Penalties: not changed in this task
- Security: pass for reminder output not leaking `sk_`; secret scan passed
- Mainnet dry run: not run

### Unrun Checks

- Check: real external scheduler installation
- Reason: Goal Stakes supplies the private skill and cron instructions; the user's external agent owns scheduler setup
- Risk: an individual external agent may implement cron incorrectly
- Required follow-up: before a production user relies on a custom agent, run the generated `.md` in that agent and verify its scheduler fires once in the user's timezone

### Final Decision

- All required checks passed: yes for own-agent daily reminder contract task scope
- Known failures at this checkpoint: final API docs/e2e coverage task was still open; closed by the later API Docs And E2E Coverage Task section below
- Ready to launch at this checkpoint: no, remaining task backlog was still open

## 2026-07-01 API Docs And E2E Coverage Task

### Run Context

- Date/time: 2026-07-01 02:32:28 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `build/goal-stakes-app`
- Commit: `887a339`
- Environment: full local suite with Docker Postgres, backend, frontend, browser wallet e2e, web3, Android JVM, Telegram fake e2e, own-agent cron fake-agent, secret scan, plus Android emulator UI smoke
- Tester/agent: Codex

### Commands

- Command: `integrations_tests/run_e2e_tests.sh`
- Result: pass
- Relevant output: `local e2e suite passed`
- Fix applied after failure: first run failed in backend admin smoke because `backend/integration_test/run_e2e_tests.sh` expected goose migration version `2`; current schema has migration `0004_agent_links.sql`. Updated the script to derive the expected migration version from `backend/migrations/[0-9][0-9][0-9][0-9]_*.sql`
- Rerun result: pass

- Command: `ANDROID_HOME="$HOME/Library/Android/sdk" android-app/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: `android emulator e2e passed`; output lists `settings-agent.png`, proving the own-agent screenshot artifact is now included in the script result
- Fix applied after failure: none after final run
- Rerun result: not needed

- Command: contact sheet generation with `montage` for `.e2e/manual-web/*.png` and `.e2e/android-emulator/*.png`
- Result: pass
- Relevant output: generated `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/contact-sheet.png` and `/Users/a.mametyev/PycharmProjects/target-app/.e2e/android-emulator/contact-sheet.png`
- Fix applied after failure: initial `montage` failed because ImageMagick could not find a default font; reran with `/System/Library/Fonts/Supplemental/Arial.ttf`
- Rerun result: pass

- Command: `scripts/secret-scan.sh`
- Result: pass
- Relevant output: `secret scan passed`
- Fix applied after failure: reran after final docs cleanup so docs/fixtures were checked after the last Markdown edits
- Rerun result: pass

### Screenshot Review

- File path: `/Users/a.mametyev/PycharmProjects/target-app/.e2e/manual-web/contact-sheet.png`
- Opened with: `view_image`
- What was visually checked: current web screenshots for approval gate, reverted approval error, chat, voice fallback, goals, settings with own-agent link, desktop landing, mobile landing, mobile settings, and wallet-signature rejection. Controls are readable with no obvious clipping, stuck loading, or incoherent overlap
- Result: pass
- Fix applied after failure: none after final run

- File path: `/Users/a.mametyev/PycharmProjects/target-app/.e2e/android-emulator/contact-sheet.png`
- Opened with: `view_image`
- What was visually checked: current Android emulator screenshots for chat, voice fallback, goal edit/actions, landscape, portrait goals, settings, own-agent generated link, and invalid URL error. Own-agent link is visible, errors are readable, and controls are usable without obvious clipping or overlap
- Result: pass
- Fix applied after failure: none after final run

### Checklist Results

- Setup: pass; Docker Postgres came up and backend admin smoke passed
- Unit and build: pass for backend, frontend, web3, Android JVM, and Telegram bot
- Integration: pass for full local suite, browser wallet e2e, backend admin smoke, backend+web3 e2e, Telegram fake e2e, own-agent fake cron, and Android emulator e2e
- Web: pass with current screenshot contact-sheet review
- Android: pass with emulator e2e and current screenshot contact-sheet review
- Telegram: pass for unit tests and fake Telegram e2e including link code, private/group/channel text, channel voice, and `/agent`
- Own agent: pass for web/Android/Telegram link creation, private Markdown skill, generated agent secret use, cron simulation, revocation, and no raw secret in list responses
- Penalties: pass for web3 unit tests and backend+web3 local e2e; mainnet live burn not executed
- Security: pass for secret scan, bot log leak checks, agent link redaction, legacy approval payload rejection, and backend-owned AI/transcription boundaries
- Mainnet dry run: pass for `verify-mainnet-deploy.sh --dry-run`; live mainnet transaction was not run

### Unrun Checks

- Check: live mainnet burn transaction with real wallet funds
- Reason: manual checklist says not to run live burn without a written sacrificial-wallet plan
- Risk: real RPC/provider or wallet funding issues could appear outside local/staging verification
- Required follow-up: before production mainnet launch, prepare sacrificial wallet plan and run `scripts/live_mainnet_gate.sh` with real `.env.mainnet.local`

### Final Decision

- All required checks passed: yes for local acceptance, docs/e2e coverage, and emulator/browser manual review
- Known failures: none in the implemented local acceptance scope
- Ready to launch: yes for local/staging handoff; mainnet launch still requires real secrets and the explicit sacrificial-wallet live-burn plan

## 2026-07-01 Web3 Fork-Local Redo

### Run Context

- Date/time: 2026-07-01 09:08:16 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `main`
- Commit before this fix: `ea1ea37`
- Environment: local Foundry fork tests using public Ethereum/Polygon RPC defaults, Docker Postgres full local suite
- Tester/agent: Codex

### Commands

- Command: `web3/integration_test/run_e2e_tests.sh`
- Initial result: fail before the RPC default fix
- Relevant output: `eth.llamarpc.com` returned Cloudflare HTTP `521`; `polygon-rpc.com` returned disabled API key / HTTP `401`
- Fix applied after failure: changed fork defaults to `https://ethereum.publicnode.com` and `https://polygon-bor-rpc.publicnode.com`; documented that acceptance should use owned provider RPCs when available
- Rerun result: pass
- Rerun relevant output: 4 fork tests passed, 0 failed, 0 skipped
- Additional fork assertions: each case verifies the expected fork `chainid` and confirms contract code exists at the canonical token address before approving and burning
- Covered cases:
  - `test_forkEthereumUSDC_burnsRealTokenToDeadAddress`
  - `test_forkEthereumUSDT_burnsRealTokenToDeadAddress`
  - `test_forkPolygonUSDC_burnsRealTokenToDeadAddress`
  - `test_forkPolygonUSDT_burnsRealTokenToDeadAddress`

- Command: `cd web3 && forge build && forge test --no-match-path test/StakeEnforcerFork.t.sol`
- Result: pass
- Relevant output: 12 unit tests passed, 0 failed, 0 skipped
- Fix applied after failure: split unit Web3 tests from fork-only tests so `forge test` without RPC does not pretend to be full Web3 acceptance
- Rerun result: pass

- Command: `integrations_tests/run_e2e_tests.sh`
- Result: pass
- Relevant output: `local e2e suite passed`
- New Web3 evidence inside the suite: unit tests passed with 12 tests; fork-local tests passed with Ethereum USDC, Ethereum USDT, Polygon USDC, and Polygon USDT; backend+web3 local e2e passed
- Other suite evidence: backend tests, backend admin smoke, frontend tests/build, browser wallet e2e, Android JVM build/tests, Telegram bot tests/e2e, own-agent cron e2e, mainnet handoff dry-run, and secret scan all passed

### Screenshot Review

- File path: not applicable
- Opened with: not applicable
- What was visually checked: no web or Android UI changed in this redo; this was a Web3 test/manual-gate correction
- Result: not applicable
- Fix applied after failure: none

### Checklist Results

- Setup: pass; Docker Postgres was already running and full local suite completed
- Unit and build: pass for backend, frontend, Web3 unit tests, Android JVM, and Telegram bot
- Integration: pass for full local suite, including browser wallet e2e, backend admin smoke, backend+web3 local e2e, Telegram fake e2e, own-agent fake cron, and secret scan
- Web3 fork-local: pass against real forked canonical USDC/USDT contracts on Ethereum and Polygon
- Penalties: pass for mock unit behavior, backend simulated chain e2e, and real-token fork-local `StakeEnforcer.penalize`
- Documentation: pass; README, runbook, web3 README, live/local e2e scripts, and manual checklist now require explicit fork-local Web3 acceptance
- Mainnet dry run: pass inside `integrations_tests/run_e2e_tests.sh`; live mainnet burn was not executed

### Unrun Checks

- Check: live mainnet burn transaction with real wallet funds
- Reason: still destructive and requires a written sacrificial-wallet plan plus real `.env.mainnet.local` secrets
- Risk: deployed-provider or funded-wallet issues could appear only in live mainnet execution
- Required follow-up: before production mainnet launch, run `ENV_FILE=.env.mainnet.local scripts/live_mainnet_gate.sh preflight`, then run `burn` only with `LIVE_E2E_CONFIRM=burn-real-funds` and recorded wallet/allowance/balance evidence

### Final Decision

- All required checks passed: yes for this Web3 fork-local redo and local acceptance gate
- Known failures: none after rerun
- Ready to launch: yes for local/staging handoff; live mainnet burn remains intentionally gated behind the sacrificial-wallet plan

## 2026-07-01 Test Structure Cleanup

### Run Context

- Date/time: 2026-07-01 11:57 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `main`
- Tester/agent: Codex

### Commands

- Command: `scripts/run_unit_tests.sh`
- Result: pass
- Relevant output: backend tests under `backend/tests/`, frontend tests under `frontend/tests/`, Web3 tests under `web3/tests/`, Android JVM tests under `android-app/app/tests/`, and Telegram tests under `telegram-bot/tests/` all passed; layout guard found no tests outside `tests/` or `integration_test/`.

- Command: `web3/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: 4 fork-local tests passed against Ethereum USDC, Ethereum USDT, Polygon USDC, and Polygon USDT.

- Command: `telegram-bot/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: fake Telegram Bot API e2e passed, including private/group/channel text, channel voice, link code, and own-agent link.

- Command: `backend/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: backend admin smoke passed; backend Web3 simulated-chain e2e passed.

- Command: `node integrations_tests/web_wallet_e2e.mjs`
- Result: pass
- Relevant output: browser wallet/API/AI flow passed and Android API integration passed through the shared backend.

- Command: `node integrations_tests/own_agent_cron_e2e.mjs`
- Result: pass
- Relevant output: own-agent private skill and daily reminder behavior passed.

- Command: `ANDROID_HOME="$HOME/Library/Android/sdk" android-app/integration_test/run_e2e_tests.sh`
- Result: pass
- Relevant output: Android emulator e2e passed and generated 8 PNG screenshots.

- Command: `integrations_tests/run_e2e_tests.sh`
- Result: pass after docs and runner changes
- Relevant output: backend e2e passed; Web3 fork-local 4 tests passed; web wallet e2e passed; Telegram bot e2e passed; own-agent cron e2e passed; mainnet gate shape passed; Android emulator e2e passed; secret scan passed; integration suite passed.

- Command: `scripts/live_mainnet_gate.sh shape`
- Result: pass
- Relevant output: live command build and mainnet handoff dry-run config passed.

### Screenshot Review

- Opened: `.e2e/android-emulator/portrait.png`
- Result: pass; goals screen is readable, selected goal/actions are visible, no overlap or clipped button text.

- Opened: `.e2e/android-emulator/goals-scrolled.png`
- Result: pass; edit panel fields and destructive/archive actions are readable and separated.

- Opened: `.e2e/android-emulator/chat.png`
- Result: pass; chat input, send, voice, and reply area are visible without overlap.

- Opened: `.e2e/android-emulator/chat-voice.png`
- Result: pass; voice cancel error is readable and does not cover controls.

- Opened: `.e2e/android-emulator/settings.png`
- Result: pass; API connection and own-agent controls are visible, key remains masked.

- Opened: `.e2e/android-emulator/settings-agent.png`
- Result: pass; own-agent link result is visible and money-safety copy is readable.

- Opened: `.e2e/android-emulator/settings-invalid-url.png`
- Result: pass; invalid URL error is readable and form controls remain usable.

- Opened: `.e2e/android-emulator/landscape.png`
- Result: pass; landscape layout is usable, no clipped tab/button text or stuck loading.

### Checklist Results

- Unit and build: pass through the single monorepo unit runner.
- Service e2e: pass through per-module `integration_test/run_e2e_tests.sh` runners.
- System e2e: pass through `integrations_tests/run_e2e_tests.sh` after docs and runner changes.
- Documentation: updated README, runbook, manual checklist, Web3 README, and the e2e coverage task to the new runner names.
- Pre-commit: tracked hook added at `.githooks/pre-commit`; install via `scripts/install-hooks.sh`.

### Unrun Checks

- Check: live mainnet burn transaction with real wallet funds
- Reason: destructive and still requires a written sacrificial-wallet plan plus real `.env.mainnet.local` secrets
- Risk: deployed-provider or funded-wallet issues could appear only in live mainnet execution
- Required follow-up: before production mainnet launch, run `ENV_FILE=.env.mainnet.local scripts/live_mainnet_gate.sh preflight`, then run `burn` only with `LIVE_E2E_CONFIRM=burn-real-funds` and recorded wallet/allowance/balance evidence

## 2026-07-01 Test Contract Enforcement And CI AVD Fix

### Run Context

- Date/time: 2026-07-01 13:44 IDT
- Workspace: `/Users/a.mametyev/PycharmProjects/target-app`
- Branch: `main`
- Tester/agent: Codex
- Scope: enforce the `AGENTS.md` test layout contract, remove the shared helper outside the documented runners, and fix the Android AVD path that made GitHub Actions integration red.

### CI Failure Root Cause

- Command: `gh run view 28510032027 --json databaseId,url,status,conclusion,headSha,displayTitle,workflowName,jobs`
- Result: fail confirmed before fix
- Relevant output: unit job passed; integration job failed in `Run integrations_tests/run_e2e_tests.sh`.

- Command: `gh run view 28510032027 --log-failed | rg -n "Unknown AVD name|goalstakes_ci|HOME is defined|Android emulator process exited"`
- Result: fail root cause confirmed
- Relevant output: `Android emulator process exited before adb reported a device`; `Unknown AVD name [goalstakes_ci]`; `HOME is defined but there is no file goalstakes_ci.ini in $HOME/.android/avd`.
- Fix applied: GitHub workflow now creates the AVD under one explicit `ANDROID_AVD_HOME`, passes the same value into `integrations_tests/run_e2e_tests.sh`, and lists AVDs after creation.

- Command: `gh run view 28512021615 --log-failed`
- Result: fail root cause confirmed after the AVD fix
- Relevant output: Android emulator booted and the app installed, then `window-chat.xml` did not contain `AI goal manager`.
- Fix applied: Android e2e tabs now tap by visible UI button text instead of fixed coordinates, waits for expected screen text with bounded retries, and prints the failing UI dump/logcat when an assertion fails.

### Commands

- Command: `bash -n scripts/run_unit_tests.sh integrations_tests/run_e2e_tests.sh backend/integration_test/run_e2e_tests.sh web3/integration_test/run_e2e_tests.sh android-app/integration_test/run_e2e_tests.sh telegram-bot/integration_test/run_e2e_tests.sh scripts/live_mainnet_gate.sh scripts/secret-scan.sh scripts/verify-mainnet-deploy.sh`
- Result: pass
- Relevant output: no shell syntax errors.

- Command: `git diff --check`
- Result: pass
- Relevant output: no whitespace errors.

- Command: `rg -n "$LEGACY_TEST_LAYOUT_PATTERNS" -S . .github -g '!AGENTS.md' -g '!docs/manual-test-checklist.md' -g '!docs/manual-test-evidence.md'`
- Result: pass
- Relevant output: no stale runner/helper/env/checklist references outside the rule docs.

- Command: `scripts/run_unit_tests.sh`
- Result: pass
- Relevant output: test layout guard passed; backend, frontend, Web3, Android JVM/build, and Telegram unit suites passed.
- Fix applied before rerun: the layout guard now covers `*_e2e.mjs`, `.spec.ts`, `.spec.tsx`, `.test.tsx`, and allows only `tests/`, per-module `integration_test/`, or root `integrations_tests/`.

- Command: `integrations_tests/run_e2e_tests.sh`
- Result: pass
- Relevant output: backend e2e passed; Web3 fork-local real-token checks passed 4/4; web wallet/API/AI e2e passed; Telegram bot e2e passed; own-agent cron e2e passed; mainnet gate shape passed; Android emulator e2e passed; secret scan passed; integration suite passed.
- Fix applied before rerun: removed the shared helper script outside the agreed test flow; documented runners now rely on the repository checkout/submodule.

- Command: `ANDROID_HOME="$HOME/Library/Android/sdk" android-app/integration_test/run_e2e_tests.sh`
- Result: pass after Android tab/wait hardening
- Relevant output: Android emulator e2e passed and regenerated portrait, goals-scrolled, chat, chat-voice, settings, settings-agent, settings-invalid-url, and landscape PNGs.

### Screenshot Review

- Opened: `.e2e/manual-web/landing-desktop.png`, `.e2e/manual-web/landing-mobile.png`
- Result: pass; landing desktop/mobile are readable, connected CTAs and runtime copy fit.

- Opened: `.e2e/manual-web/wallet-signature-rejected-desktop.png`, `.e2e/manual-web/approval-gate-desktop.png`, `.e2e/manual-web/approval-reverted-desktop.png`
- Result: pass; wallet rejection, approval gate, and reverted approval error render clearly with usable controls.

- Opened: `.e2e/manual-web/goals-desktop.png`, `.e2e/manual-web/goals-after-api-desktop.png`, `.e2e/manual-web/goals-after-android-desktop.png`
- Result: pass; goal create/edit/action states are visible and aligned.

- Opened: `.e2e/manual-web/chat-desktop.png`, `.e2e/manual-web/chat-voice-desktop.png`
- Result: pass; chat input, messages, send, and voice fallback states are readable without overlap.

- Opened: `.e2e/manual-web/settings-desktop.png`, `.e2e/manual-web/settings-api-key-created.png`, `.e2e/manual-web/settings-mobile.png`
- Result: pass; Settings desktop/mobile are usable. `settings-api-key-created.png` intentionally shows a generated local user API key once for the copy flow; it is not an AI provider key, and the Telegram link code plus own-agent URL do not expose raw `sk_`.

- Opened: every fresh PNG under `.e2e/android-emulator/`
- Result: pass; portrait, goals-scrolled, chat, chat-voice, settings, settings-agent, settings-invalid-url, and landscape screens are nonblank, readable, and have no critical control overlap.

### Checklist Results

- Unit layout: pass through the single documented `scripts/run_unit_tests.sh`.
- Per-module e2e layout: pass through module `integration_test/run_e2e_tests.sh` runners only.
- System e2e layout: pass through the single documented `integrations_tests/run_e2e_tests.sh`.
- Ad-hoc script cleanup: pass; no root ad-hoc e2e runners or shared test helper remain.
- CI fix: local reproduction passes; the next GitHub Actions run must pass before launch.

### Unrun Checks

- Check: live mainnet burn transaction with real wallet funds
- Reason: destructive and still requires a written sacrificial-wallet plan plus real `.env.mainnet.local` secrets
- Risk: deployed-provider or funded-wallet issues could appear only in live mainnet execution
- Required follow-up: before production mainnet launch, run `ENV_FILE=.env.mainnet.local scripts/live_mainnet_gate.sh preflight`, then run `burn` only with `LIVE_E2E_CONFIRM=burn-real-funds` and recorded wallet/allowance/balance evidence
