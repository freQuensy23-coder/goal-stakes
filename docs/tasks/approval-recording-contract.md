# Approval Recording Contract

name: Approval recording matches on-chain verification contract
status: done

description:
- Align `POST /api/v1/approvals` with the README flow.
- README says the web client posts `tx_hash`, `chain`, and `token`, then backend verifies allowance on-chain before caching.
- Frontend must not send legacy `{chain, token_symbol, allowance}` from wallet code, and the backend schema must not accept client-provided allowance as the public contract.
- Backend already refreshes live allowance when an `ApprovalChecker` exists; this task removes trust in client-provided allowance as the public contract.

definition of done:
- `RecordApprovalInput` accepts `tx_hash`, `chain`, and `token_symbol`; client-provided allowance is not required for the public API.
- Backend verifies on-chain allowance through `ApprovalChecker` before marking approval recorded.
- If live checker is disabled for local dry-run, behavior is explicit and documented; production/mainnet cannot trust client-provided allowance.
- Frontend approval flow sends transaction hash and reloads backend-observed allowance.
- OpenAPI and API tests describe the `tx_hash` contract.
- Existing wallet rejection e2e still proves a reverted approval is not recorded.

test scenarios:
- `go test ./...` in `backend`.
- `npm test` and `npm run build` in `frontend`.
- Service test: client-provided allowance cannot override live zero allowance.
- Router test: missing `tx_hash` returns `400`.
- Frontend test: approval submit sends `tx_hash`, not legacy `allowance`.
- Browser wallet e2e: rejected transaction leaves allowance at zero; successful transaction records backend-observed allowance.
