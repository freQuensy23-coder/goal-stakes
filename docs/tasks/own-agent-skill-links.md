# Own Agent Skill Links

name: Own-agent private skill links
status: done

description:
- Implement private Markdown skill links for user-owned external agents.
- Initial gap: there was no `agent_links` domain type, store methods, routes, or generated skill document.
- Add `GET /api/v1/agent-links`, `POST /api/v1/agent-links`, `DELETE /api/v1/agent-links/{agentLinkID}`, and `GET /agent-skills/{token}.md`.
- Generated agent API secrets must be stored only as hashes. The raw secret appears only in the private generated `.md` response.
- Revoking an agent link must revoke both the skill URL token and the generated agent API key.

definition of done:
- Database migration adds agent-link rows with user id, API-key id, token hash, expiry, created timestamp, and revoked timestamp.
- Store, memory store, and Postgres store support list/create/read-by-token/revoke.
- `POST /api/v1/agent-links` creates a raw `sk_` agent secret, stores only its hash through the API-key path, and returns one private skill URL.
- `GET /agent-skills/{token}.md` returns Markdown with frontmatter, project summary, API base URL, raw agent secret, supported API endpoints, safety rules, daily cron instruction, and revocation instruction.
- `GET /api/v1/agent-links` lists only metadata, never raw secrets or raw tokens.
- `DELETE /api/v1/agent-links/{id}` revokes the skill link and linked API key.
- Revoked or expired skill URLs return `404` or `410`; revoked agent secrets return `401`.

test scenarios:
- `go test ./...` in `backend`.
- Store test: raw skill token and raw agent secret are never persisted.
- Router test: authenticated create/list/delete agent links.
- Router test: generated `.md` contains required frontmatter and user-specific API base/secret.
- Negative router test: revoked link cannot fetch `.md`.
- API-key auth test: old agent secret fails after link revocation.
- Secret scan: repository files and logs do not contain generated raw agent secrets.
