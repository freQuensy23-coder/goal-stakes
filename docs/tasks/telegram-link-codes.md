# Telegram Link Codes

name: Telegram link code lifecycle
status: done

description:
- Replace Telegram raw API-key linking with backend-issued one-time link codes.
- Current bot command `/apikey sk_...` asks users to paste a raw API key into Telegram and stores it in bot memory.
- Implement `POST /api/v1/telegram/link-codes` for authenticated users.
- Implement backend storage for hashed one-time link codes with expiry and consumed state.
- Implement persistent Telegram chat/group/channel links owned by backend.
- Implement `POST /internal/telegram/link` authenticated by a backend-issued bot secret.

definition of done:
- Database migration adds Telegram link-code and Telegram link tables.
- Store, memory store, and Postgres store support creating, consuming, and resolving Telegram links.
- Public user endpoint returns a short one-time code and never returns a raw API key.
- Internal link endpoint consumes a valid code once and maps Telegram chat/channel id to the user.
- Expired, reused, malformed, and wrong-code link attempts fail with safe messages.
- Bot help and tests no longer mention `/apikey sk_...` as the supported linking path.
- Bot does not persist user API keys in memory or on disk.

test scenarios:
- `go test ./...` in `backend`.
- `go test ./...` in `telegram-bot`.
- Backend unit test: link code is stored hashed, consumed once, and expires.
- Backend router test: `POST /api/v1/telegram/link-codes` requires user auth.
- Internal endpoint test: missing or wrong bot secret returns `401`.
- Bot e2e test: `/link code` links private chat without any `sk_` value in update text, response text, or logs.
- Negative e2e: a second `/link same-code` fails and does not change ownership.
