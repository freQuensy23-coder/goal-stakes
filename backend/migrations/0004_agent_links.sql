-- +goose Up
-- +goose StatementBegin

CREATE TABLE agent_links (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    name       TEXT NOT NULL DEFAULT '',
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX agent_links_user_id_idx ON agent_links (user_id);
CREATE INDEX agent_links_api_key_id_idx ON agent_links (api_key_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agent_links;
-- +goose StatementEnd
