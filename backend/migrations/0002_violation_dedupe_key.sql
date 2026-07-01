-- +goose Up
-- +goose StatementBegin

ALTER TABLE violations ADD COLUMN IF NOT EXISTS dedupe_key TEXT;

UPDATE violations
SET dedupe_key = period
WHERE dedupe_key IS NULL;

ALTER TABLE violations DROP CONSTRAINT IF EXISTS violations_goal_period_uniq;

CREATE UNIQUE INDEX IF NOT EXISTS violations_goal_dedupe_uniq
ON violations (goal_id, dedupe_key)
WHERE dedupe_key IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS violations_goal_dedupe_uniq;
ALTER TABLE violations DROP COLUMN IF EXISTS dedupe_key;
ALTER TABLE violations ADD CONSTRAINT violations_goal_period_uniq UNIQUE (goal_id, period);

-- +goose StatementEnd
