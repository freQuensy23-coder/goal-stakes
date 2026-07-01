# Telegram Internal Gateway

name: Telegram internal text gateway
status: done

description:
- Move Telegram command and free-text execution behind backend internal endpoints.
- Current bot parses commands locally and calls public `/api/v1/*` endpoints with a raw user API key.
- Implement `POST /internal/telegram/message` so backend resolves chat/group/channel links to users and executes commands through the service layer.
- Support private chat, group, and channel text update shapes.
- Keep the bot as a thin Telegram transport: receive update, forward text and metadata to backend, send backend reply to Telegram.

definition of done:
- Backend config requires a Telegram internal bot secret when internal Telegram routes are enabled.
- Internal message endpoint accepts `chat_id`, `message_id`, update kind, and text.
- Backend supports `/goals`, `/create`, `/done`, `/violate`, `/progress`, `/archive`, and free text AI for linked Telegram chats.
- Unknown or unlinked chats receive a link-code instruction and no state mutation.
- Command parsing lives in backend or a backend-owned package, not in a bot-owned user-key map.
- Bot handles `message` and `channel_post` updates.
- Bot logs do not include Telegram token, backend secret, raw API keys, or authorization headers.

test scenarios:
- `go test ./...` in `backend`.
- `go test ./...` in `telegram-bot`.
- Backend tests cover each command success and malformed command error.
- Backend tests prove unlinked chat cannot list or mutate goals.
- Bot unit tests cover private `message`, group `message`, and `channel_post` forwarding.
- Fake Telegram e2e sends commands and free text through `/internal/telegram/message`, not public user endpoints.
- Secret-scan test fails if bot logs include tokens or auth headers.
