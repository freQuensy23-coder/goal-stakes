# Own Agent Client Entrypoints

name: Connect own agent from web, Android, and Telegram
status: done

description:
- Add user-facing entrypoints for generating own-agent skill links.
- Initial gap: web Settings only managed approvals and API keys.
- Initial gap: Android Settings only saved API URL and API key.
- Initial gap: Telegram bot had no `/agent` command or button path.
- Implement `Connect own agent` in all three surfaces using backend-owned agent-link APIs.

definition of done:
- Web Settings has a `Connect own agent` control that creates a skill link, copies it, lists active links, and revokes links.
- Android Settings has a `Connect own agent` control that creates and displays/copies a skill link, then supports revocation or clear instructions to revoke from web.
- Telegram supports `/agent` for linked chats through `POST /internal/telegram/agent-link`.
- Telegram `/agent` returns only the private skill URL, not a raw long-lived user API key.
- UI text warns that the link is private and revocable.
- Layout is manually verified on desktop, mobile web, Android portrait, and Android landscape.

test scenarios:
- `npm test` and `npm run build` in `frontend`.
- `ANDROID_HOME="$HOME/Library/Android/sdk" gradle testDebugUnitTest assembleDebug` in `android-app`.
- `go test ./...` in `telegram-bot`.
- Frontend test: agent link create/list/revoke API methods and Settings UI states.
- Android unit test: API client calls `/api/v1/agent-links` and parses skill URL.
- Fake Telegram e2e: `/agent` on a linked chat calls internal backend endpoint and replies with the URL.
- Manual screenshot review: web Settings and Android Settings have no clipped text or overlapping controls.
