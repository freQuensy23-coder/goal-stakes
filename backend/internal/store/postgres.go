package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"goalstakes/internal/domain"
)

// Postgres is the production Store backed by a pgx connection pool. It
// implements the exact same contract as Memory (IF0) and relies on the unique
// indexes declared in the migrations to guarantee idempotency (IV6, GPC5):
//   - check_ins (goal_id, period)
//   - violations (goal_id, period)
//   - wallet_approvals (user_id, chain, token_symbol)
//
// All queries are parameterized (no string interpolation of values). Missing
// rows are reported as ErrNotFound (wrapped); any other failure is wrapped and
// returned, never swallowed (GPC6).
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres returns a Store backed by the given pgxpool.Pool. The caller owns
// the pool's lifecycle (it is created and closed in main).
func NewPostgres(pool *pgxpool.Pool) Store {
	return &Postgres{pool: pool}
}

// mapErr normalizes pgx's no-rows sentinel into the store's ErrNotFound while
// preserving context, and passes every other error through wrapped. This keeps
// "absent" distinguishable from "query failed" (GPC6).
func mapErr(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", op, ErrNotFound)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// ---- Users -------------------------------------------------------------------

func (p *Postgres) CreateUser(ctx context.Context, walletAddress, timezone string) (domain.User, error) {
	u := domain.User{
		ID:            domain.NewID(),
		WalletAddress: walletAddress,
		Timezone:      timezone,
		CreatedAt:     now(),
	}
	const q = `
		INSERT INTO users (id, wallet_address, timezone, created_at)
		VALUES ($1, $2, $3, $4)`
	if _, err := p.pool.Exec(ctx, q, u.ID, u.WalletAddress, u.Timezone, u.CreatedAt); err != nil {
		return domain.User{}, fmt.Errorf("create user %q: %w", walletAddress, err)
	}
	return u, nil
}

func (p *Postgres) GetUserByWallet(ctx context.Context, walletAddress string) (domain.User, error) {
	const q = `
		SELECT id, wallet_address, timezone, created_at
		FROM users WHERE wallet_address = $1`
	var u domain.User
	err := p.pool.QueryRow(ctx, q, walletAddress).Scan(&u.ID, &u.WalletAddress, &u.Timezone, &u.CreatedAt)
	if err != nil {
		return domain.User{}, mapErr(fmt.Sprintf("get user by wallet %q", walletAddress), err)
	}
	return u, nil
}

func (p *Postgres) GetUser(ctx context.Context, id domain.UUID) (domain.User, error) {
	const q = `
		SELECT id, wallet_address, timezone, created_at
		FROM users WHERE id = $1`
	var u domain.User
	err := p.pool.QueryRow(ctx, q, id).Scan(&u.ID, &u.WalletAddress, &u.Timezone, &u.CreatedAt)
	if err != nil {
		return domain.User{}, mapErr(fmt.Sprintf("get user %s", id), err)
	}
	return u, nil
}

// ---- Goals -------------------------------------------------------------------

func (p *Postgres) CreateGoal(ctx context.Context, in CreateGoalInput) (domain.Goal, error) {
	g := domain.Goal{
		ID:          domain.NewID(),
		UserID:      in.UserID,
		Title:       in.Title,
		Description: in.Description,
		Type:        in.Type,
		Cadence:     in.Cadence,
		StakeAmount: in.StakeAmount,
		TokenSymbol: in.TokenSymbol,
		Chain:       in.Chain,
		Timezone:    in.Timezone,
		Archived:    false,
		CreatedAt:   now(),
		StartsAt:    in.StartsAt,
		EndsAt:      in.EndsAt,
	}
	const q = `
		INSERT INTO goals (id, user_id, title, description, type, cadence,
		                   stake_amount, token_symbol, chain, timezone,
		                   archived, created_at, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	_, err := p.pool.Exec(ctx, q,
		g.ID, g.UserID, g.Title, g.Description, string(g.Type), string(g.Cadence),
		g.StakeAmount, g.TokenSymbol, g.Chain, g.Timezone,
		g.Archived, g.CreatedAt, g.StartsAt, g.EndsAt)
	if err != nil {
		return domain.Goal{}, fmt.Errorf("create goal for user %s: %w", in.UserID, err)
	}
	return g, nil
}

// scanGoal scans one goals row in the canonical column order.
func scanGoal(row pgx.Row) (domain.Goal, error) {
	var g domain.Goal
	var typ, cadence string
	err := row.Scan(
		&g.ID, &g.UserID, &g.Title, &g.Description, &typ, &cadence,
		&g.StakeAmount, &g.TokenSymbol, &g.Chain, &g.Timezone,
		&g.Archived, &g.CreatedAt, &g.StartsAt, &g.EndsAt)
	if err != nil {
		return domain.Goal{}, err
	}
	g.Type = domain.GoalType(typ)
	g.Cadence = domain.Cadence(cadence)
	return g, nil
}

const goalColumns = `id, user_id, title, description, type, cadence,
	stake_amount, token_symbol, chain, timezone,
	archived, created_at, starts_at, ends_at`

func (p *Postgres) GetGoal(ctx context.Context, id domain.UUID) (domain.Goal, error) {
	q := `SELECT ` + goalColumns + ` FROM goals WHERE id = $1`
	g, err := scanGoal(p.pool.QueryRow(ctx, q, id))
	if err != nil {
		return domain.Goal{}, mapErr(fmt.Sprintf("get goal %s", id), err)
	}
	return g, nil
}

func (p *Postgres) GetGoalsByUser(ctx context.Context, userID domain.UUID) ([]domain.Goal, error) {
	q := `SELECT ` + goalColumns + ` FROM goals WHERE user_id = $1 ORDER BY created_at, id`
	rows, err := p.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list goals for user %s: %w", userID, err)
	}
	defer rows.Close()

	out := make([]domain.Goal, 0)
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, fmt.Errorf("list goals for user %s: scan: %w", userID, err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list goals for user %s: %w", userID, err)
	}
	return out, nil
}

func (p *Postgres) UpdateGoal(ctx context.Context, in UpdateGoalInput) (domain.Goal, error) {
	q := `
		UPDATE goals
		SET title = $2,
		    description = $3,
		    stake_amount = $4,
		    token_symbol = CASE WHEN $5 = '' THEN token_symbol ELSE $5 END,
		    chain = CASE WHEN $6 = '' THEN chain ELSE $6 END,
		    ends_at = $7
		WHERE id = $1
		RETURNING ` + goalColumns
	g, err := scanGoal(p.pool.QueryRow(ctx, q, in.ID, in.Title, in.Description, in.StakeAmount, in.TokenSymbol, in.Chain, in.EndsAt))
	if err != nil {
		return domain.Goal{}, mapErr(fmt.Sprintf("update goal %s", in.ID), err)
	}
	return g, nil
}

func (p *Postgres) ArchiveGoal(ctx context.Context, id domain.UUID) error {
	const q = `UPDATE goals SET archived = TRUE WHERE id = $1`
	tag, err := p.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("archive goal %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("archive goal %s: %w", id, ErrNotFound)
	}
	return nil
}

func (p *Postgres) ListActiveGoals(ctx context.Context) ([]domain.Goal, error) {
	q := `SELECT ` + goalColumns + ` FROM goals WHERE archived = FALSE ORDER BY created_at, id`
	rows, err := p.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list active goals: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Goal, 0)
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, fmt.Errorf("list active goals: scan: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active goals: %w", err)
	}
	return out, nil
}

// ---- Check-ins ---------------------------------------------------------------

func (p *Postgres) UpsertCheckIn(ctx context.Context, goalID domain.UUID, period domain.Period, note string) (domain.CheckIn, error) {
	// ON CONFLICT keeps the existing id and created_at (we never overwrite them),
	// only updating the note — matching the memory store's upsert semantics.
	const q = `
		INSERT INTO check_ins (id, goal_id, period, note, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (goal_id, period)
		DO UPDATE SET note = EXCLUDED.note
		RETURNING id, goal_id, period, note, created_at`
	var c domain.CheckIn
	var per string
	err := p.pool.QueryRow(ctx, q, domain.NewID(), goalID, string(period), note, now()).
		Scan(&c.ID, &c.GoalID, &per, &c.Note, &c.CreatedAt)
	if err != nil {
		return domain.CheckIn{}, fmt.Errorf("upsert check-in goal=%s period=%s: %w", goalID, period, err)
	}
	c.Period = domain.Period(per)
	return c, nil
}

func (p *Postgres) GetCheckIn(ctx context.Context, goalID domain.UUID, period domain.Period) (domain.CheckIn, error) {
	const q = `
		SELECT id, goal_id, period, note, created_at
		FROM check_ins WHERE goal_id = $1 AND period = $2`
	var c domain.CheckIn
	var per string
	err := p.pool.QueryRow(ctx, q, goalID, string(period)).
		Scan(&c.ID, &c.GoalID, &per, &c.Note, &c.CreatedAt)
	if err != nil {
		return domain.CheckIn{}, mapErr(fmt.Sprintf("get check-in goal=%s period=%s", goalID, period), err)
	}
	c.Period = domain.Period(per)
	return c, nil
}

// ---- Violations --------------------------------------------------------------

// CreateViolation always inserts a fresh Pending row. It is used for avoid goals
// where each self-reported slip is a separate paid violation.
func (p *Postgres) CreateViolation(ctx context.Context, in CreateViolationInput) (domain.Violation, error) {
	ts := now()
	const q = `
		INSERT INTO violations (id, goal_id, period, dedupe_key, status, amount, reason, tx_hash, created_at, updated_at)
		VALUES ($1, $2, $3, NULL, $4, $5, $6, '', $7, $7)
		RETURNING id, goal_id, period, status, amount, reason, tx_hash, created_at, updated_at`
	v, err := scanViolation(p.pool.QueryRow(ctx, q,
		domain.NewID(), in.GoalID, string(in.Period), string(domain.ViolationPending), in.Amount, in.Reason, ts))
	if err != nil {
		return domain.Violation{}, fmt.Errorf("create violation goal=%s period=%s: %w", in.GoalID, in.Period, err)
	}
	return v, nil
}

// CreateViolationIfAbsent is idempotent on (goal, period) via a non-null
// dedupe key. The INSERT ... ON CONFLICT DO NOTHING returns no row when a row
// already exists, so we then SELECT and return the existing row unchanged.
func (p *Postgres) CreateViolationIfAbsent(ctx context.Context, in CreateViolationInput) (domain.Violation, bool, error) {
	ts := now()
	const insert = `
		INSERT INTO violations (id, goal_id, period, dedupe_key, status, amount, reason, tx_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $3, $4, $5, $6, '', $7, $7)
		ON CONFLICT (goal_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING
		RETURNING id, goal_id, period, status, amount, reason, tx_hash, created_at, updated_at`

	v, err := scanViolation(p.pool.QueryRow(ctx, insert,
		domain.NewID(), in.GoalID, string(in.Period), string(domain.ViolationPending), in.Amount, in.Reason, ts))
	if err == nil {
		return v, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Violation{}, false, fmt.Errorf("create violation goal=%s period=%s: %w", in.GoalID, in.Period, err)
	}

	// Conflict: a row already exists for this (goal, period). Return it as-is.
	const sel = `
		SELECT id, goal_id, period, status, amount, reason, tx_hash, created_at, updated_at
		FROM violations WHERE goal_id = $1 AND dedupe_key = $2`
	v, err = scanViolation(p.pool.QueryRow(ctx, sel, in.GoalID, string(in.Period)))
	if err != nil {
		return domain.Violation{}, false, mapErr(fmt.Sprintf("create violation goal=%s period=%s: load existing", in.GoalID, in.Period), err)
	}
	return v, false, nil
}

func scanViolation(row pgx.Row) (domain.Violation, error) {
	var v domain.Violation
	var per, status string
	err := row.Scan(&v.ID, &v.GoalID, &per, &status, &v.Amount, &v.Reason, &v.TxHash, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return domain.Violation{}, err
	}
	v.Period = domain.Period(per)
	v.Status = domain.ViolationStatus(status)
	return v, nil
}

func (p *Postgres) UpdateViolation(ctx context.Context, in UpdateViolationInput) (domain.Violation, error) {
	const q = `
		UPDATE violations
		SET status = $2, tx_hash = $3, updated_at = $4
		WHERE id = $1
		RETURNING id, goal_id, period, status, amount, reason, tx_hash, created_at, updated_at`
	v, err := scanViolation(p.pool.QueryRow(ctx, q, in.ID, string(in.Status), in.TxHash, now()))
	if err != nil {
		return domain.Violation{}, mapErr(fmt.Sprintf("update violation %s", in.ID), err)
	}
	return v, nil
}

func (p *Postgres) ListViolations(ctx context.Context, goalID domain.UUID) ([]domain.Violation, error) {
	const q = `
		SELECT id, goal_id, period, status, amount, reason, tx_hash, created_at, updated_at
		FROM violations WHERE goal_id = $1 ORDER BY created_at, id`
	rows, err := p.pool.Query(ctx, q, goalID)
	if err != nil {
		return nil, fmt.Errorf("list violations goal=%s: %w", goalID, err)
	}
	defer rows.Close()

	out := make([]domain.Violation, 0)
	for rows.Next() {
		v, err := scanViolation(rows)
		if err != nil {
			return nil, fmt.Errorf("list violations goal=%s: scan: %w", goalID, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list violations goal=%s: %w", goalID, err)
	}
	return out, nil
}

// ---- Wallet approvals --------------------------------------------------------

func (p *Postgres) UpsertWalletApproval(ctx context.Context, userID domain.UUID, chain, tokenSymbol, allowance string) (domain.WalletApproval, error) {
	// One row per (user, chain, token); on conflict we refresh allowance and
	// updated_at and keep the existing id (matching the memory store).
	const q = `
		INSERT INTO wallet_approvals (id, user_id, chain, token_symbol, allowance, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, chain, token_symbol)
		DO UPDATE SET allowance = EXCLUDED.allowance, updated_at = EXCLUDED.updated_at
		RETURNING id, user_id, chain, token_symbol, allowance, updated_at`
	var a domain.WalletApproval
	err := p.pool.QueryRow(ctx, q, domain.NewID(), userID, chain, tokenSymbol, allowance, now()).
		Scan(&a.ID, &a.UserID, &a.Chain, &a.TokenSymbol, &a.Allowance, &a.UpdatedAt)
	if err != nil {
		return domain.WalletApproval{}, fmt.Errorf("upsert wallet approval user=%s chain=%s token=%s: %w", userID, chain, tokenSymbol, err)
	}
	return a, nil
}

func (p *Postgres) GetWalletApproval(ctx context.Context, userID domain.UUID, chain, tokenSymbol string) (domain.WalletApproval, error) {
	const q = `
		SELECT id, user_id, chain, token_symbol, allowance, updated_at
		FROM wallet_approvals WHERE user_id = $1 AND chain = $2 AND token_symbol = $3`
	var a domain.WalletApproval
	err := p.pool.QueryRow(ctx, q, userID, chain, tokenSymbol).
		Scan(&a.ID, &a.UserID, &a.Chain, &a.TokenSymbol, &a.Allowance, &a.UpdatedAt)
	if err != nil {
		return domain.WalletApproval{}, mapErr(fmt.Sprintf("get wallet approval user=%s chain=%s token=%s", userID, chain, tokenSymbol), err)
	}
	return a, nil
}

// ---- API keys (IV3: hash + prefix only; the raw key never reaches here) ------

func (p *Postgres) CreateApiKey(ctx context.Context, in CreateApiKeyInput) (domain.ApiKey, error) {
	k := domain.ApiKey{
		ID:        domain.NewID(),
		UserID:    in.UserID,
		Name:      in.Name,
		Prefix:    in.Prefix,
		KeyHash:   in.KeyHash,
		CreatedAt: now(),
	}
	const q = `
		INSERT INTO api_keys (id, user_id, name, prefix, key_hash, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := p.pool.Exec(ctx, q, k.ID, k.UserID, k.Name, k.Prefix, k.KeyHash, k.CreatedAt); err != nil {
		return domain.ApiKey{}, fmt.Errorf("create api key for user %s: %w", in.UserID, err)
	}
	return k, nil
}

const apiKeyColumns = `id, user_id, name, prefix, key_hash, created_at, last_used, revoked_at`

func scanApiKey(row pgx.Row) (domain.ApiKey, error) {
	var k domain.ApiKey
	err := row.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.KeyHash, &k.CreatedAt, &k.LastUsed, &k.RevokedAt)
	if err != nil {
		return domain.ApiKey{}, err
	}
	return k, nil
}

func (p *Postgres) GetApiKeyByHash(ctx context.Context, keyHash string) (domain.ApiKey, error) {
	q := `SELECT ` + apiKeyColumns + ` FROM api_keys WHERE key_hash = $1`
	k, err := scanApiKey(p.pool.QueryRow(ctx, q, keyHash))
	if err != nil {
		return domain.ApiKey{}, mapErr("get api key by hash", err)
	}
	return k, nil
}

func (p *Postgres) TouchApiKey(ctx context.Context, id domain.UUID) (domain.ApiKey, error) {
	q := `UPDATE api_keys SET last_used = $2 WHERE id = $1 RETURNING ` + apiKeyColumns
	k, err := scanApiKey(p.pool.QueryRow(ctx, q, id, now()))
	if err != nil {
		return domain.ApiKey{}, mapErr(fmt.Sprintf("touch api key %s", id), err)
	}
	return k, nil
}

func (p *Postgres) ListApiKeysByUser(ctx context.Context, userID domain.UUID) ([]domain.ApiKey, error) {
	q := `SELECT ` + apiKeyColumns + ` FROM api_keys WHERE user_id = $1 ORDER BY created_at, id`
	rows, err := p.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys for user %s: %w", userID, err)
	}
	defer rows.Close()

	out := make([]domain.ApiKey, 0)
	for rows.Next() {
		k, err := scanApiKey(rows)
		if err != nil {
			return nil, fmt.Errorf("list api keys for user %s: scan: %w", userID, err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list api keys for user %s: %w", userID, err)
	}
	return out, nil
}

// RevokeApiKey is idempotent: revoking an already-revoked key is a no-op
// success (matching the memory store). Only a genuinely missing key is an error.
func (p *Postgres) RevokeApiKey(ctx context.Context, id domain.UUID) error {
	const q = `UPDATE api_keys SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL`
	tag, err := p.pool.Exec(ctx, q, id, now())
	if err != nil {
		return fmt.Errorf("revoke api key %s: %w", id, err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	// Zero rows: either the key does not exist, or it was already revoked.
	// Distinguish the two so a missing key is reported (GPC6) but a re-revoke
	// stays a no-op success.
	const exists = `SELECT 1 FROM api_keys WHERE id = $1`
	var one int
	if err := p.pool.QueryRow(ctx, exists, id).Scan(&one); err != nil {
		return mapErr(fmt.Sprintf("revoke api key %s", id), err)
	}
	return nil
}

// ---- Telegram links ----------------------------------------------------------

func (p *Postgres) CreateTelegramLinkCode(ctx context.Context, in CreateTelegramLinkCodeInput) (domain.TelegramLinkCode, error) {
	code := domain.TelegramLinkCode{
		ID:        domain.NewID(),
		UserID:    in.UserID,
		CodeHash:  in.CodeHash,
		ExpiresAt: in.ExpiresAt,
		CreatedAt: now(),
	}
	const q = `
		INSERT INTO telegram_link_codes (id, user_id, code_hash, expires_at, consumed_at, created_at)
		VALUES ($1, $2, $3, $4, NULL, $5)`
	if _, err := p.pool.Exec(ctx, q, code.ID, code.UserID, code.CodeHash, code.ExpiresAt, code.CreatedAt); err != nil {
		return domain.TelegramLinkCode{}, fmt.Errorf("create telegram link code for user %s: %w", in.UserID, err)
	}
	return code, nil
}

const telegramLinkCodeColumns = `id, user_id, code_hash, expires_at, consumed_at, created_at`

func scanTelegramLinkCode(row pgx.Row) (domain.TelegramLinkCode, error) {
	var code domain.TelegramLinkCode
	err := row.Scan(&code.ID, &code.UserID, &code.CodeHash, &code.ExpiresAt, &code.ConsumedAt, &code.CreatedAt)
	if err != nil {
		return domain.TelegramLinkCode{}, err
	}
	return code, nil
}

func (p *Postgres) ConsumeTelegramLinkCode(ctx context.Context, codeHash string, consumedAt time.Time) (domain.TelegramLinkCode, error) {
	q := `
		UPDATE telegram_link_codes
		SET consumed_at = $2
		WHERE code_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING ` + telegramLinkCodeColumns
	code, err := scanTelegramLinkCode(p.pool.QueryRow(ctx, q, codeHash, consumedAt))
	if err != nil {
		return domain.TelegramLinkCode{}, mapErr("consume telegram link code", err)
	}
	return code, nil
}

func (p *Postgres) UpsertTelegramLink(ctx context.Context, in UpsertTelegramLinkInput) (domain.TelegramLink, error) {
	ts := now()
	const q = `
		INSERT INTO telegram_links (id, user_id, chat_id, chat_kind, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (chat_id)
		DO UPDATE SET user_id = EXCLUDED.user_id, chat_kind = EXCLUDED.chat_kind, updated_at = EXCLUDED.updated_at
		RETURNING id, user_id, chat_id, chat_kind, created_at, updated_at`
	link, err := scanTelegramLink(p.pool.QueryRow(ctx, q, domain.NewID(), in.UserID, in.ChatID, in.ChatKind, ts))
	if err != nil {
		return domain.TelegramLink{}, fmt.Errorf("upsert telegram link chat=%d: %w", in.ChatID, err)
	}
	return link, nil
}

func (p *Postgres) GetTelegramLinkByChatID(ctx context.Context, chatID int64) (domain.TelegramLink, error) {
	const q = `
		SELECT id, user_id, chat_id, chat_kind, created_at, updated_at
		FROM telegram_links WHERE chat_id = $1`
	link, err := scanTelegramLink(p.pool.QueryRow(ctx, q, chatID))
	if err != nil {
		return domain.TelegramLink{}, mapErr(fmt.Sprintf("get telegram link chat=%d", chatID), err)
	}
	return link, nil
}

func scanTelegramLink(row pgx.Row) (domain.TelegramLink, error) {
	var link domain.TelegramLink
	err := row.Scan(&link.ID, &link.UserID, &link.ChatID, &link.ChatKind, &link.CreatedAt, &link.UpdatedAt)
	if err != nil {
		return domain.TelegramLink{}, err
	}
	return link, nil
}

// ---- Agent links -------------------------------------------------------------

func (p *Postgres) CreateAgentLink(ctx context.Context, in CreateAgentLinkInput) (domain.AgentLink, error) {
	link := domain.AgentLink{
		ID:        domain.NewID(),
		UserID:    in.UserID,
		APIKeyID:  in.APIKeyID,
		Name:      in.Name,
		TokenHash: in.TokenHash,
		ExpiresAt: in.ExpiresAt,
		CreatedAt: now(),
	}
	const q = `
		INSERT INTO agent_links (id, user_id, api_key_id, name, token_hash, expires_at, created_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULL)`
	if _, err := p.pool.Exec(ctx, q, link.ID, link.UserID, link.APIKeyID, link.Name, link.TokenHash, link.ExpiresAt, link.CreatedAt); err != nil {
		return domain.AgentLink{}, fmt.Errorf("create agent link for user %s: %w", in.UserID, err)
	}
	return link, nil
}

const agentLinkColumns = `id, user_id, api_key_id, name, token_hash, expires_at, created_at, revoked_at`

func scanAgentLink(row pgx.Row) (domain.AgentLink, error) {
	var link domain.AgentLink
	err := row.Scan(&link.ID, &link.UserID, &link.APIKeyID, &link.Name, &link.TokenHash, &link.ExpiresAt, &link.CreatedAt, &link.RevokedAt)
	if err != nil {
		return domain.AgentLink{}, err
	}
	return link, nil
}

func (p *Postgres) ListAgentLinksByUser(ctx context.Context, userID domain.UUID) ([]domain.AgentLink, error) {
	q := `SELECT ` + agentLinkColumns + ` FROM agent_links WHERE user_id = $1 ORDER BY created_at, id`
	rows, err := p.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list agent links for user %s: %w", userID, err)
	}
	defer rows.Close()

	out := make([]domain.AgentLink, 0)
	for rows.Next() {
		link, err := scanAgentLink(rows)
		if err != nil {
			return nil, fmt.Errorf("list agent links for user %s: scan: %w", userID, err)
		}
		out = append(out, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list agent links for user %s: %w", userID, err)
	}
	return out, nil
}

func (p *Postgres) GetAgentLinkByTokenHash(ctx context.Context, tokenHash string) (domain.AgentLink, error) {
	q := `SELECT ` + agentLinkColumns + ` FROM agent_links WHERE token_hash = $1`
	link, err := scanAgentLink(p.pool.QueryRow(ctx, q, tokenHash))
	if err != nil {
		return domain.AgentLink{}, mapErr("get agent link by token hash", err)
	}
	return link, nil
}

func (p *Postgres) RevokeAgentLink(ctx context.Context, id domain.UUID) (domain.AgentLink, error) {
	q := `UPDATE agent_links SET revoked_at = COALESCE(revoked_at, $2) WHERE id = $1 RETURNING ` + agentLinkColumns
	link, err := scanAgentLink(p.pool.QueryRow(ctx, q, id, now()))
	if err != nil {
		return domain.AgentLink{}, mapErr(fmt.Sprintf("revoke agent link %s", id), err)
	}
	return link, nil
}

// ---- Conversations & messages ------------------------------------------------

func (p *Postgres) CreateConversation(ctx context.Context, userID domain.UUID, title string) (domain.Conversation, error) {
	ts := now()
	c := domain.Conversation{
		ID:        domain.NewID(),
		UserID:    userID,
		Title:     title,
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	const q = `
		INSERT INTO conversations (id, user_id, title, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := p.pool.Exec(ctx, q, c.ID, c.UserID, c.Title, c.CreatedAt, c.UpdatedAt); err != nil {
		return domain.Conversation{}, fmt.Errorf("create conversation for user %s: %w", userID, err)
	}
	return c, nil
}

// AppendMessage inserts a message and bumps the parent conversation's
// updated_at in a single transaction (mirroring the memory store, which updates
// both). If the conversation does not exist, the foreign key would reject the
// insert; we surface that as an error (GPC6).
func (p *Postgres) AppendMessage(ctx context.Context, conversationID domain.UUID, role domain.MessageRole, content string) (domain.Message, error) {
	msg := domain.Message{
		ID:             domain.NewID(),
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		CreatedAt:      now(),
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Message{}, fmt.Errorf("append message conversation=%s: begin tx: %w", conversationID, err)
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit.

	// Verify the conversation exists first so a missing parent is reported as
	// ErrNotFound rather than an opaque FK violation (matches memory semantics).
	const exists = `SELECT 1 FROM conversations WHERE id = $1`
	var one int
	if err := tx.QueryRow(ctx, exists, conversationID).Scan(&one); err != nil {
		return domain.Message{}, mapErr(fmt.Sprintf("append message conversation=%s", conversationID), err)
	}

	const insertMsg = `
		INSERT INTO messages (id, conversation_id, role, content, created_at)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := tx.Exec(ctx, insertMsg, msg.ID, msg.ConversationID, string(msg.Role), msg.Content, msg.CreatedAt); err != nil {
		return domain.Message{}, fmt.Errorf("append message conversation=%s: insert: %w", conversationID, err)
	}

	const bump = `UPDATE conversations SET updated_at = $2 WHERE id = $1`
	if _, err := tx.Exec(ctx, bump, conversationID, msg.CreatedAt); err != nil {
		return domain.Message{}, fmt.Errorf("append message conversation=%s: bump updated_at: %w", conversationID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Message{}, fmt.Errorf("append message conversation=%s: commit: %w", conversationID, err)
	}
	return msg, nil
}

func (p *Postgres) GetConversation(ctx context.Context, id domain.UUID) (domain.Conversation, []domain.Message, error) {
	const convQ = `
		SELECT id, user_id, title, created_at, updated_at
		FROM conversations WHERE id = $1`
	var c domain.Conversation
	err := p.pool.QueryRow(ctx, convQ, id).Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return domain.Conversation{}, nil, mapErr(fmt.Sprintf("get conversation %s", id), err)
	}

	const msgQ = `
		SELECT id, conversation_id, role, content, created_at
		FROM messages WHERE conversation_id = $1 ORDER BY created_at, id`
	rows, err := p.pool.Query(ctx, msgQ, id)
	if err != nil {
		return domain.Conversation{}, nil, fmt.Errorf("get conversation %s messages: %w", id, err)
	}
	defer rows.Close()

	msgs := make([]domain.Message, 0)
	for rows.Next() {
		var m domain.Message
		var role string
		if err := rows.Scan(&m.ID, &m.ConversationID, &role, &m.Content, &m.CreatedAt); err != nil {
			return domain.Conversation{}, nil, fmt.Errorf("get conversation %s messages: scan: %w", id, err)
		}
		m.Role = domain.MessageRole(role)
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return domain.Conversation{}, nil, fmt.Errorf("get conversation %s messages: %w", id, err)
	}
	return c, msgs, nil
}

// compile-time check that Postgres satisfies the Store interface.
var _ Store = (*Postgres)(nil)

// ensure time is referenced (used via now() in the same package) — no-op guard
// kept out; now() lives in memory.go. The blank below documents the dependency.
var _ = time.Time{}
