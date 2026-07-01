-- +goose Up
-- +goose StatementBegin

CREATE TABLE telegram_link_codes (
    id          UUID PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash   TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX telegram_link_codes_user_id_idx ON telegram_link_codes (user_id);

CREATE TABLE telegram_links (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id    BIGINT NOT NULL UNIQUE,
    chat_kind  TEXT NOT NULL CHECK (chat_kind IN ('private', 'group', 'supergroup', 'channel')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX telegram_links_user_id_idx ON telegram_links (user_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS telegram_links;
DROP TABLE IF EXISTS telegram_link_codes;
-- +goose StatementEnd
