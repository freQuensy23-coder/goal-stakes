# Backend Audio Chat

name: Backend audio chat endpoint
status: done

description:
- Implement the backend-owned transcription path required by `POST /api/v1/chat/audio`.
- Initial gap: the API only exposed `POST /api/v1/chat`; Web and Android voice flows used browser or OS speech recognition and sent text to `/chat`.
- Add a transcription boundary in `backend/internal/ai` so OpenAI keys stay only in backend config.
- Accept multipart audio from authenticated users, transcribe it on the backend, then pass the transcript into the existing AI chat manager.
- Return `transcript`, `conversation_id`, `reply`, and optional `tool_results`.

definition of done:
- `backend/internal/config` has explicit transcription config where needed and no frontend/mobile env exposes AI secrets.
- `backend/internal/api/router.go` registers authenticated `POST /api/v1/chat/audio`.
- The endpoint rejects missing auth, malformed multipart, missing audio file, empty transcription, and unsupported content types with JSON errors.
- The transcription client is injected for tests; unit tests do not call live OpenAI.
- Existing `POST /api/v1/chat` behavior stays unchanged.
- Web and Android can keep OS transcript fallback, but file-upload clients have a real backend audio endpoint.

test scenarios:
- `go test ./...` in `backend`.
- Router test: unauthenticated `/api/v1/chat/audio` returns `401`.
- Router test: valid audio multipart returns transcript and chat reply.
- Router test: empty/missing audio returns `400`.
- AI test: fake transcription result `I did 10 push-ups` is forwarded to chat and can trigger the existing tool flow.
- Secret scan: frontend bundle and Android source do not contain `OPENAI_API_KEY` or transcription credentials.
