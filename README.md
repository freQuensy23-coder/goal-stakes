# Goal Stakes Flow Spec

Goal Stakes lets a user stake USDC/USDT on any daily or weekly goal. If the user misses a do-goal or reports an avoid-goal slip, the backend enforcer burns the stake from the user's allowance. No app wallet receives funds.

This file is the acceptance spec. Code is done only when every flow below works and is manually verified.

## Hard Rules

- AI keys live only in `backend`: `OPENAI_API_KEY`, model, transcription config.
- RPC URLs and `ENFORCER_PRIVATE_KEY` live only in `backend`.
- React, Android, Telegram bot, and external clients call the backend for product state.
- React may call the injected wallet only for SIWE, chain switch, allowance read, and ERC-20 approval.
- Telegram bot may call Telegram Bot API and backend only.
- Telegram channel/group/private linking must not require posting a raw `sk_` API key into Telegram.
- Backend stores Telegram chat/channel links. Bot does not persist user API keys.
- Personal agent skill links are bearer secrets. Anyone with the link can read the generated `.md` and its user API secret until the link is revoked or expired.
- Generated agent API secrets are stored only as hashes. The raw secret appears only in the generated private skill document.
- Backend service layer is the only place that creates goals, check-ins, violations, API keys, Telegram links, and charges.
- `StakeEnforcer` can only burn to `0x000000000000000000000000000000000000dEaD`.
- A failed check means redo the work, not update the checklist.

## Required Backend Surface

Public:
- `GET /api/v1/chains`
- `POST /api/v1/auth/nonce`
- `POST /api/v1/auth/siwe`

User-authenticated by session JWT or API key:
- `GET /api/v1/me`
- `GET /api/v1/goals`
- `POST /api/v1/goals`
- `PATCH /api/v1/goals/{goalID}`
- `PATCH /api/v1/goals/{goalID}/stake`
- `DELETE /api/v1/goals/{goalID}`
- `GET /api/v1/goals/{goalID}/progress`
- `POST /api/v1/goals/{goalID}/checkins`
- `GET /api/v1/goals/{goalID}/violations`
- `POST /api/v1/goals/{goalID}/violations`
- `GET /api/v1/approvals`
- `POST /api/v1/approvals`
- `GET /api/v1/apikeys`
- `POST /api/v1/apikeys`
- `DELETE /api/v1/apikeys/{apiKeyID}`
- `POST /api/v1/chat`
- `POST /api/v1/chat/audio`
- `POST /api/v1/telegram/link-codes`
- `GET /api/v1/agent-links`
- `POST /api/v1/agent-links`
- `DELETE /api/v1/agent-links/{agentLinkID}`

Private bearer-by-link:
- `GET /agent-skills/{token}.md`

Bot-authenticated by backend-issued bot secret:
- `POST /internal/telegram/link`
- `POST /internal/telegram/message`
- `POST /internal/telegram/audio`
- `POST /internal/telegram/agent-link`

`POST /api/v1/chat/audio` and `/internal/telegram/audio` return:
- `transcript`
- `conversation_id`
- `reply`
- optional `tool_results`

## Flow Coverage

- Web: boot, wallet sign-in, approval, create, edit, done, violation, archive, progress, chat, microphone.
- Settings: API docs, API key create/copy/revoke, Telegram link code.
- Own agent: web, Android, and Telegram can generate a private `.md` skill link for a user's external agent.
- Android: API-key setup, goals, create, edit, done, violation, archive, progress, chat, microphone.
- Telegram: private chat, group, channel, link code, commands, free text, voice/audio.
- Penalties: missed do-goal scheduler, avoid-goal report, failed charge visibility.
- External API: user-owned API keys only.

## WEB-01 Boot And Wallet Sign-In

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Web as React Web
  participant Wallet as Injected Wallet
  participant API as Backend API
  participant Auth as SIWE Auth
  participant DB as Postgres
  User->>Web: Open app
  Web->>API: GET /api/v1/chains
  API-->>Web: Chain keys, token addresses, StakeEnforcer addresses
  User->>Web: Connect wallet
  Web->>Wallet: eth_requestAccounts
  Wallet-->>Web: wallet address
  Web->>API: POST /api/v1/auth/nonce
  API->>DB: Store single-use nonce
  API-->>Web: nonce
  Web->>Wallet: personal_sign(SIWE message)
  Wallet-->>Web: signature
  Web->>API: POST /api/v1/auth/siwe
  Auth->>DB: Consume nonce, upsert user
  API-->>Web: session JWT + user
```

## WEB-02 Approve Stake

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Web as React Web
  participant Wallet as Injected Wallet
  participant Token as USDC/USDT
  participant API as Backend API
  participant RPC as Backend RPC
  participant DB as Postgres
  User->>Web: Choose chain, token, approval amount
  Web->>API: GET /api/v1/approvals?chain&token_symbol
  API->>RPC: allowance(user, StakeEnforcer)
  API->>DB: Cache observed allowance
  API-->>Web: allowance + approved flag
  alt allowance too low
    Web->>Wallet: switch chain
    Web->>Wallet: approve(StakeEnforcer, amount)
    Wallet->>Token: signed approve transaction
    Token-->>Wallet: receipt
    Web->>API: POST /api/v1/approvals(tx_hash, chain, token)
    API->>RPC: verify allowance on-chain
    API->>DB: Store verified allowance
    API-->>Web: approved
  end
```

Local dry-run mode is the only exception: when the backend has no live allowance checker because `ENFORCER_PRIVATE_KEY` is empty, `POST /api/v1/approvals` still requires `tx_hash` and may accept `dry_run_allowance` so browser e2e can run without RPC signing infrastructure. Production and mainnet deployments must verify allowance through backend RPC.

## WEB-03 Create Goal Manually

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Web as React Web
  participant API as Backend API
  participant Service as Goal Service
  participant RPC as Backend RPC
  participant DB as Postgres
  User->>Web: Fill title, do/avoid, daily/weekly, stake, token, chain, dates
  Web->>API: POST /api/v1/goals
  API->>Service: CreateGoal(input)
  Service->>RPC: Verify current allowance covers stake
  alt valid and allowance covers stake
    Service->>DB: Insert goal
    API-->>Web: Created goal
  else invalid or allowance insufficient
    API-->>Web: 400 with exact blocking reason
  end
```

## WEB-04 Edit Goal Or Stake

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Web as React Web
  participant Wallet as Injected Wallet
  participant API as Backend API
  participant Service as Goal Service
  participant RPC as Backend RPC
  participant DB as Postgres
  User->>Web: Edit title, end date, stake, token, or chain
  alt stake/token/chain increased or changed
    Web->>Wallet: approve new allowance if needed
    Web->>API: POST /api/v1/approvals
    API->>RPC: Verify allowance
  end
  Web->>API: PATCH /api/v1/goals/{id} or /stake
  API->>Service: Validate owner and stake
  Service->>DB: Update goal
  API-->>Web: Updated goal + progress reload
```

## WEB-05 Done, Violation, Archive, Progress

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Web as React Web
  participant API as Backend API
  participant Service as Goal Service
  participant DB as Postgres
  User->>Web: Click Done
  Web->>API: POST /api/v1/goals/{id}/checkins
  Service->>DB: Upsert check-in for current period
  API-->>Web: check-in
  User->>Web: Click Violation
  Web->>API: POST /api/v1/goals/{id}/violations
  Service->>DB: Insert violation before charge
  API-->>Web: violation status
  User->>Web: Click Archive
  Web->>API: DELETE /api/v1/goals/{id}
  Service->>DB: Mark archived
  Web->>API: GET /api/v1/goals and /progress
  API-->>Web: current list and statuses
```

## CHAT-01 Text AI From Any Client

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Client as Web/Android/Telegram/External
  participant API as Backend API
  participant AI as Backend AI Manager
  participant OpenAI as OpenAI Chat API
  participant Service as Goal Service
  participant DB as Postgres
  User->>Client: "I did 10 push-ups"
  Client->>API: POST /api/v1/chat
  API->>AI: Chat(user, text)
  AI->>DB: Save user message
  AI->>OpenAI: Prompt + tool schemas
  OpenAI-->>AI: tool call or reply
  alt tool call
    AI->>Service: create/list/update/check-in/violation/progress
    Service->>DB: Validated read/write
    AI->>OpenAI: tool result
  end
  AI->>DB: Save assistant reply
  API-->>Client: reply + conversation_id
```

## VOICE-01 Microphone From Web Or Android

Voice is product input. If transcription uses AI, it happens on backend.

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Client as Web/Android
  participant API as Backend API
  participant STT as Backend Transcription
  participant AI as Backend AI Manager
  participant Service as Goal Service
  User->>Client: Tap microphone and speak
  Client->>Client: Capture audio file or OS transcript
  alt audio file available
    Client->>API: POST /api/v1/chat/audio multipart audio
    API->>STT: Transcribe with backend AI key
    STT-->>API: transcript
  else OS transcript only
    Client->>API: POST /api/v1/chat transcript text
  end
  API->>AI: Chat(user, transcript)
  AI->>Service: Optional goal tool call
  API-->>Client: transcript + reply
```

## SETTINGS-01 API Key Lifecycle

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Web as React Settings
  participant API as Backend API
  participant DB as Postgres
  participant Client as Android/External Client
  User->>Web: Create API key
  Web->>API: POST /api/v1/apikeys
  API->>DB: Store hash + prefix only
  API-->>Web: raw sk_ key once
  Client->>API: Authorization Bearer sk_...
  API->>DB: Verify hash, revoked_at, touch last_used
  API-->>Client: allowed
  User->>Web: Revoke key
  Web->>API: DELETE /api/v1/apikeys/{id}
  API->>DB: Set revoked_at
  Client->>API: Reuse revoked key
  API-->>Client: 401
```

## SETTINGS-02 Telegram Link Code

Do not paste raw API keys into Telegram channels.

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Web as React Settings
  participant API as Backend API
  participant DB as Postgres
  participant TG as Telegram Chat/Group/Channel
  participant Bot as Telegram Bot
  User->>Web: Create Telegram link code
  Web->>API: POST /api/v1/telegram/link-codes
  API->>DB: Store hashed one-time code with expiry
  API-->>Web: short code
  User->>TG: /link code
  TG->>Bot: update message or channel_post
  Bot->>API: POST /internal/telegram/link(chat_id, code)
  API->>DB: Consume code, store chat_id/channel_id -> user_id
  API-->>Bot: linked
  Bot->>TG: Linked to Goal Stakes
```

## SETTINGS-03 Connect Own Agent

Buttons named `Connect own agent` exist in web settings, Android settings, and Telegram bot. They generate a private Markdown skill link for the current user.

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Client as Web/Android/Telegram
  participant Bot as Telegram Bot
  participant API as Backend API
  participant DB as Postgres
  User->>Client: Click Connect own agent
  alt Web or Android
    Client->>API: POST /api/v1/agent-links
  else Telegram
    Client->>Bot: /agent or button callback
    Bot->>API: POST /internal/telegram/agent-link(chat_id)
    API->>DB: Resolve linked chat_id/channel_id -> user_id
  end
  API->>DB: Create agent API key hash
  API->>DB: Create private skill-link token hash + expiry
  API-->>Client: https://app/agent-skills/{token}.md
  Client-->>User: Show/copy private link
```

## AGENT-01 Private Skill Document

The generated `.md` file is what the user sends to their own agent. It contains user-specific secrets, so the URL is private and revocable.

Required skill content:
- Project: Goal Stakes burns USDC/USDT stake when goals are missed.
- Short model: backend owns AI/RPC/secrets; clients and agents use backend API only.
- API base URL.
- Generated `sk_` agent API secret.
- Supported endpoints for goals, check-ins, violations, progress, chat, and audio chat if the agent can send files.
- Safety rules: never call chains directly, never ask for wallet seed phrases, never create or increase stake without clear user confirmation.
- Cron instruction: once per day, if the user has at least one active unarchived goal, remind the user to check progress.
- Revocation instruction: user can revoke the agent link/key from Goal Stakes settings.

Example generated body:

```md
---
name: goal-stakes-user-agent
description: Use when this user asks to manage Goal Stakes goals, check progress, send reminders, or record goal updates through the Goal Stakes API.
---

# Goal Stakes User Agent Skill

Use this skill when helping this user manage Goal Stakes.

API base: https://api.goalstakes.example
Authorization: Bearer sk_agent_private_value

Goal Stakes lets the user create do/avoid goals with USDC/USDT stake. If a goal is missed, backend burns the stake through StakeEnforcer. You never call OpenAI, RPC, wallets, or contracts for this user.

Daily cron:
- Run once per day in the user's timezone.
- Call GET /api/v1/goals.
- If at least one goal is active, remind the user to check in or report a violation.
- Do not mark a goal done unless the user explicitly says they completed it.
```

## AGENT-02 Daily Reminder Cron

The external agent owns the cron job. Goal Stakes supplies the instructions and API secret through the private skill.

```mermaid
sequenceDiagram
  autonumber
  participant Cron as User Agent Cron
  participant API as Backend API
  participant User as User
  Cron->>API: GET /api/v1/goals with agent sk_
  API-->>Cron: active goals
  alt at least one active goal
    Cron->>User: Daily reminder with open goals
  else no active goals
    Cron->>Cron: No reminder
  end
  User->>Cron: "I did 10 push-ups"
  Cron->>API: POST /api/v1/chat
  API-->>Cron: reply and state change result
  Cron->>User: confirmation or clarification
```

## AGENT-03 Revoke Own Agent

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Client as Web/Android/Telegram
  participant API as Backend API
  participant DB as Postgres
  participant Agent as External Agent
  User->>Client: Revoke agent access
  Client->>API: DELETE /api/v1/agent-links/{agentLinkID}
  API->>DB: Revoke skill link token
  API->>DB: Revoke linked agent API key
  Agent->>API: GET /api/v1/goals with old sk_
  API-->>Agent: 401
  Agent->>API: GET /agent-skills/{token}.md
  API-->>Agent: 404 or 410
```

## ANDROID-01 Setup

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Web as React Settings
  participant Android
  participant API as Backend API
  participant DB as Postgres
  User->>Web: Create API key
  Web-->>User: raw sk_ key once
  User->>Android: Enter backend URL + sk_ key
  Android->>API: GET /api/v1/me
  API->>DB: Verify key hash
  API-->>Android: user id or 401
  Android->>API: GET /api/v1/goals
  API-->>Android: active goals
```

## ANDROID-02 Goals And Chat

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Android
  participant API as Backend API
  participant Service as Goal Service
  participant DB as Postgres
  User->>Android: Create/edit/check in/report/archive/show progress
  Android->>API: Matching /api/v1/goals endpoint
  API->>Service: Same validation as web
  Service->>DB: Read/write user-owned state
  API-->>Android: structured result/error
  User->>Android: Send chat text or voice transcript
  Android->>API: POST /api/v1/chat or /chat/audio
  API-->>Android: transcript/reply
```

## TELEGRAM-01 Text Command In Private, Group, Or Channel

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant TG as Telegram
  participant Bot as Telegram Bot
  participant API as Backend API
  participant Service as Goal Service
  participant DB as Postgres
  User->>TG: /goals, /create, /done, /violate, /progress, /archive
  TG->>Bot: message or channel_post update
  Bot->>API: POST /internal/telegram/message(chat_id, message_id, text)
  API->>DB: Resolve linked chat_id/channel_id -> user_id
  API->>Service: Execute command through service layer
  Service->>DB: Read/write state
  API-->>Bot: reply text
  Bot->>TG: sendMessage or reply to channel post
```

## TELEGRAM-02 Free Text AI

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant TG as Telegram
  participant Bot as Telegram Bot
  participant API as Backend API
  participant AI as Backend AI Manager
  participant Service as Goal Service
  participant DB as Postgres
  User->>TG: "I did 10 push-ups"
  TG->>Bot: message or channel_post text
  Bot->>API: POST /internal/telegram/message(text)
  API->>DB: Resolve Telegram link
  API->>AI: Chat(user, text)
  AI->>Service: Optional tool call
  Service->>DB: Persist validated result
  API-->>Bot: reply
  Bot->>TG: sendMessage
```

## TELEGRAM-03 Voice Message In Channel

Required example: a person posts a Telegram voice message in a linked channel saying `я отжался 10 раз`.

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant TG as Telegram Channel
  participant Bot as Telegram Bot
  participant File as Telegram File API
  participant API as Backend API
  participant STT as Backend Transcription
  participant AI as Backend AI Manager
  participant Service as Goal Service
  participant DB as Postgres
  User->>TG: Voice post: "я отжался 10 раз"
  TG->>Bot: channel_post.voice(file_id, duration, mime)
  Bot->>File: getFile(file_id)
  File-->>Bot: file_path
  Bot->>File: download OGG/OPUS bytes
  Bot->>API: POST /internal/telegram/audio(chat_id, message_id, audio)
  API->>DB: Resolve channel_id -> user_id
  API->>STT: Transcribe audio with backend AI key
  STT-->>API: "я отжался 10 раз"
  API->>AI: Chat(user, transcript)
  AI->>Service: log_check_in or ask clarification
  alt matching push-up goal found
    Service->>DB: Record check-in note "я отжался 10 раз"
    API-->>Bot: "Записал: 10 отжиманий"
  else no matching goal or ambiguous
    API-->>Bot: Clarifying question, no state change
  end
  Bot->>TG: sendMessage reply
```

## TELEGRAM-04 Voice Message In Private Or Group

Same as channel voice, but update type is `message.voice` and the link key is `chat.id`.

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant TG as Telegram Private/Group
  participant Bot as Telegram Bot
  participant API as Backend API
  participant STT as Backend Transcription
  participant AI as Backend AI Manager
  participant Service as Goal Service
  User->>TG: Send voice note
  TG->>Bot: message.voice
  Bot->>API: POST /internal/telegram/audio
  API->>STT: Transcribe
  API->>AI: Chat(user, transcript)
  AI->>Service: Optional tool call
  API-->>Bot: transcript + reply
  Bot->>TG: sendMessage
```

## PENALTY-01 Missed Do-Goal

```mermaid
sequenceDiagram
  autonumber
  participant Scheduler
  participant Service as Goal Service
  participant DB as Postgres
  participant RPC as Backend RPC
  participant Enforcer as StakeEnforcer
  participant Token as USDC/USDT
  Scheduler->>Service: Scan elapsed daily/weekly periods
  Service->>DB: Find active do-goals without check-in
  Service->>DB: Insert pending violation with goal-period dedupe key
  Service->>RPC: Send penalize(user, token, amount)
  RPC->>Enforcer: penalize
  Enforcer->>Token: transferFrom(user, burn, amount)
  alt tx succeeds
    Service->>DB: Mark violation charged with tx hash
  else tx fails
    Service->>DB: Mark violation failed with error
  end
```

## PENALTY-02 Avoid-Goal Slip

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Client as Any Client
  participant API as Backend API
  participant Service as Goal Service
  participant DB as Postgres
  participant RPC as Backend RPC
  User->>Client: Report avoid-goal slip
  Client->>API: POST /api/v1/goals/{id}/violations
  Service->>DB: Insert pending violation for this report
  Service->>RPC: Send burn transaction
  alt charged
    Service->>DB: Mark charged
  else failed
    Service->>DB: Mark failed and keep visible
  end
  API-->>Client: violation status
```

## EXTERNAL-01 Public API Client

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Web as React Settings
  participant External as Script/Automation
  participant API as Backend API
  participant DB as Postgres
  User->>Web: Create API key for automation
  Web-->>User: raw sk_ key once
  External->>API: Authorization Bearer sk_...
  API->>DB: Verify hash and owner
  External->>API: goals, checkins, violations, chat
  API-->>External: JSON or structured error
```

## EXTERNAL-02 Agent Skill Client

```mermaid
sequenceDiagram
  autonumber
  actor User
  participant Agent as User's Agent
  participant Skill as Private .md Skill
  participant API as Backend API
  User->>Agent: Send private skill link
  Agent->>Skill: GET /agent-skills/{token}.md
  Skill-->>Agent: Project rules + API base + agent sk_ secret + cron instruction
  Agent->>API: Use documented endpoints with Authorization Bearer sk_
  API-->>Agent: JSON result or structured error
```

## Done Gate

- Every flow above has unit, integration, e2e, and manual evidence where applicable.
- Web and Android screenshots are opened and visually judged.
- Telegram text and voice tests cover private chat, group, and channel update shapes.
- The voice test includes the transcript `я отжался 10 раз`.
- Own-agent tests prove web, Android, and Telegram generate a private skill `.md` link.
- Own-agent tests prove the generated skill contains API base, agent secret, project rules, API usage, and daily cron reminder instructions.
- Revoking an own-agent link must revoke both the link and the generated agent API secret.
- No UI check passes from screenshot existence alone.
- No client bundle contains AI keys, RPC secrets, private keys, JWT secrets, DB credentials, or Telegram tokens.
- Web3 acceptance includes `web3/integration_test/run_e2e_tests.sh`, which forks Ethereum and Polygon and tests real canonical USDC/USDT contracts.
- Mainnet burn is tested only with an explicit sacrificial-wallet plan.

Use [manual_checklist.md](manual_checklist.md) and [docs/manual-test-checklist.md](docs/manual-test-checklist.md). Record evidence in [docs/manual-test-evidence.md](docs/manual-test-evidence.md).
