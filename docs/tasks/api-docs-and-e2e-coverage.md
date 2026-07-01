# API Docs And E2E Coverage

name: API docs and full e2e coverage sync
status: done

description:
- Keep implementation, OpenAPI, runbook, manual checklist, and e2e scripts aligned after the missing features land.
- OpenAPI documents all README-required public, private skill-link, and internal Telegram endpoints.
- Fake Telegram e2e covers link codes, private/group/channel text, voice/audio, and `/agent`.
- Full local suite proves audio, Telegram links, own-agent links, fake-agent cron, and secret-scan checks.

definition of done:
- `backend/internal/api/openapi.go` includes all README-required public, private skill-link, and internal Telegram routes.
- `docs/run.md` describes new env vars such as Telegram internal bot secret and public app/API base URLs.
- `docs/manual-test-checklist.md` matches implemented commands, endpoints, and UI labels.
- `scripts/e2e-local.sh` runs the updated backend, frontend, web3, Android JVM, Telegram, own-agent, and secret-scan checks.
- `scripts/e2e-telegram-bot.mjs` covers link code, private/group/channel text, voice download, and `/agent`.
- Manual evidence file records command output and screenshot review for changed web and Android surfaces.

test scenarios:
- `scripts/e2e-local.sh` from repo root.
- `node scripts/e2e-telegram-bot.mjs` from repo root.
- `scripts/e2e-android-emulator.sh` when Android UI changes.
- OpenAPI test: every README-required path exists in `/openapi.json`.
- Docs review: no doc claims a feature is done without matching code or test evidence.
- Secret scan: built frontend, logs, docs, and fixtures do not expose live secrets or generated raw agent secrets.
