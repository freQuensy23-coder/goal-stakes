-- +goose Up
-- +goose StatementBegin

-- Goal-stakes initial schema. All money amounts (stake/allowance) are stored as
-- NUMERIC(78,0) to hold an EVM uint256 in the token's smallest unit without
-- precision loss; the Go layer carries them as decimal strings.

CREATE TABLE users (
    id             UUID PRIMARY KEY,
    wallet_address TEXT NOT NULL UNIQUE,           -- AS1: wallet is identity
    timezone       TEXT NOT NULL DEFAULT '',       -- blank means UTC
    created_at     TIMESTAMPTZ NOT NULL
);

CREATE TABLE goals (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL CHECK (type IN ('do', 'avoid')),       -- GoalType (AS4)
    cadence      TEXT NOT NULL CHECK (cadence IN ('daily', 'weekly', 'custom')),
    stake_amount NUMERIC(78, 0) NOT NULL,
    token_symbol TEXT NOT NULL,
    chain        TEXT NOT NULL,
    timezone     TEXT NOT NULL DEFAULT '',
    archived     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL,
    starts_at    TIMESTAMPTZ NOT NULL,
    ends_at      TIMESTAMPTZ
);

CREATE INDEX goals_user_id_idx ON goals (user_id);

CREATE TABLE check_ins (
    id         UUID PRIMARY KEY,
    goal_id    UUID NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    period     TEXT NOT NULL,
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    -- At most one check-in per (goal, period).
    CONSTRAINT check_ins_goal_period_uniq UNIQUE (goal_id, period)
);

CREATE TABLE violations (
    id         UUID PRIMARY KEY,
    goal_id    UUID NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    period     TEXT NOT NULL,
    dedupe_key TEXT,
    status     TEXT NOT NULL CHECK (status IN ('pending', 'charged', 'failed')),
    amount     NUMERIC(78, 0) NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    tx_hash    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- Non-null dedupe keys make deadline-based do-goal violations idempotent.
-- Avoid-goal reports leave dedupe_key NULL so every slip can charge.
CREATE UNIQUE INDEX violations_goal_dedupe_uniq ON violations (goal_id, dedupe_key) WHERE dedupe_key IS NOT NULL;

CREATE TABLE wallet_approvals (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chain        TEXT NOT NULL,
    token_symbol TEXT NOT NULL,
    allowance    NUMERIC(78, 0) NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    -- One cached allowance per (user, chain, token).
    CONSTRAINT wallet_approvals_user_chain_token_uniq UNIQUE (user_id, chain, token_symbol)
);

CREATE TABLE api_keys (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL DEFAULT '',
    prefix     TEXT NOT NULL,                       -- non-secret identifier (IV3)
    key_hash   TEXT NOT NULL UNIQUE,                -- one-way hash only (IV3); no raw column
    created_at TIMESTAMPTZ NOT NULL,
    last_used  TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX api_keys_user_id_idx ON api_keys (user_id);

CREATE TABLE conversations (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX conversations_user_id_idx ON conversations (user_id);

CREATE TABLE messages (
    id              UUID PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX messages_conversation_id_idx ON messages (conversation_id, created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS wallet_approvals;
DROP TABLE IF EXISTS violations;
DROP TABLE IF EXISTS check_ins;
DROP TABLE IF EXISTS goals;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
