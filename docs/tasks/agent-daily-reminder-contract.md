# Agent Daily Reminder Contract

name: Own-agent daily reminder contract
status: done

description:
- Make the generated skill enough for an external agent to set a daily reminder cron.
- README requires the external agent to run once per day, call `GET /api/v1/goals`, and remind the user only if at least one active goal exists.
- Generated skill documents and fake-agent checks must prove these instructions are present and usable.
- This task does not add a backend scheduler for reminders; the external user agent owns the cron.

definition of done:
- Generated skill document includes a concrete daily cron section.
- Cron instructions specify timezone behavior, `GET /api/v1/goals`, active-goal filtering, reminder wording, and the rule not to mark goals done without explicit user confirmation.
- Skill content includes a minimal API example using `Authorization: Bearer <generated sk_>`.
- Tests assert the generated skill contains the cron section and required safety rules.
- A local fake-agent test or script simulates active goals and no active goals using the generated API secret.

test scenarios:
- `go test ./...` in `backend`.
- Generated-skill unit test: daily cron section exists with `GET /api/v1/goals`.
- Generated-skill unit test: safety rule forbids marking a goal done without explicit user confirmation.
- Fake-agent test: active goals produce a reminder message.
- Fake-agent test: empty goals produce no reminder.
- Revocation test: fake-agent call with revoked generated key returns `401`.
