# Agent Rules

Goal Stakes contains money-moving flows. Testing rules are strict so agents cannot hide broken behavior behind one-off scripts.

## Test Requirements

- Unit tests live only in `tests/` directories.
- Per-module e2e/integration tests live only in that module's `integration_test/`.
- System e2e tests live only in `integrations_tests/`.
- Do not add ad-hoc `scripts/e2e-*`, throwaway smoke scripts, duplicate runners, or "temporary" test entrypoints.
- Keep tests in the existing style for each module: Go tests under `tests/`, frontend Vitest tests under `frontend/tests`, Web3 Foundry tests under `web3/tests`, Android tests under `android-app/app/tests`, Telegram bot tests under `telegram-bot/tests`.
- Every behavior change must be tested at the right level: unit tests for local logic, module e2e tests for service boundaries, system e2e tests for flows crossing backend/web/mobile/Telegram/Web3.
- Money-moving, auth, secret-handling, Telegram voice, custom-agent, and UI changes require e2e/manual evidence. Mocks alone are not enough.
- Test runners must enforce a 600-second total timeout. E2E runners must also use named stage timeouts and condition-based waits; do not rely on unbounded commands or sleeps as synchronization.
- When tests generate screenshots or recordings, open them and manually verify the UI. File existence is not visual QA.
- If any required test fails, fix the implementation and rerun the failed test plus related regressions before marking the work done.

## How To Run Tests

- All unit tests: `scripts/run_unit_tests.sh`.
- All system e2e tests: `integrations_tests/run_e2e_tests.sh`.
- Backend e2e only: `backend/integration_test/run_e2e_tests.sh`.
- Web3 fork-local e2e only: `web3/integration_test/run_e2e_tests.sh`.
- Telegram bot e2e only: `telegram-bot/integration_test/run_e2e_tests.sh`.
- Android emulator e2e only: `android-app/integration_test/run_e2e_tests.sh`.

Before committing, install the tracked hook with `scripts/install-hooks.sh`. The hook runs `scripts/run_unit_tests.sh`.
