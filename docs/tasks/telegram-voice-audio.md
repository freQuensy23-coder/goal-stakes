# Telegram Voice Audio

name: Telegram voice and audio flow
status: done

description:
- Implement Telegram voice handling for private chat, groups, and channels.
- Initial gap: the Telegram client only modeled `message.chat.id` and `message.text`; it ignored `voice`, `audio`, and `channel_post`.
- Add Telegram `getFile` and file-download support for voice OGG/OPUS files.
- Implement `POST /internal/telegram/audio` so the backend resolves the Telegram link, transcribes audio with backend AI credentials, and runs the same AI tool flow as text chat.
- Required manual example: linked channel voice post saying `я отжался 10 раз`.

definition of done:
- Telegram update structs include private/group `message.voice`, `message.audio`, `channel_post.voice`, and `channel_post.audio`.
- Bot downloads Telegram files without logging file URLs containing the bot token.
- Backend internal audio endpoint accepts multipart or binary audio plus Telegram metadata.
- Backend response includes transcript, conversation id, reply, and optional tool results.
- Clear goal match records a check-in; ambiguous transcript asks a clarification and does not mutate state.
- Private, group, and channel voice flows all use backend-owned transcription.

test scenarios:
- `go test ./...` in `backend`.
- `go test ./...` in `telegram-bot`.
- Telegram client test: `getFile` and file download are called for a voice update.
- Backend audio test: fake transcript `я отжался 10 раз` reaches the chat manager.
- E2E fake Telegram test: `channel_post.voice` produces a backend audio request and a Telegram reply.
- Negative test: unlinked channel voice does not call transcription and replies with link instructions.
- Manual test: inspect screenshots/log evidence and confirm the transcript and reply are correct for `я отжался 10 раз`.
