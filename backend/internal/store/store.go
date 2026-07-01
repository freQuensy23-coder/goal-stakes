// Package store defines the persistence boundary (IF0) for goal-stakes and
// provides two implementations: an in-memory store for unit tests
// (NewMemory) and a Postgres/pgx store for production (NewPostgres).
//
// Every method takes a context.Context as its first argument (GPC7) so that
// cancellation, deadlines, and request-scoped logging propagate to the DB.
//
// Idempotency (GPC5): CreateViolationIfAbsent and the Upsert* methods are safe
// to call repeatedly for the same key; they converge on a single row.
package store

import (
	"context"
	"errors"
	"time"

	"goalstakes/internal/domain"
)

// ErrNotFound is returned by Get* methods when no row matches. Callers must
// distinguish "absent" from "query failed" (GPC6 — no silent empty results).
var ErrNotFound = errors.New("store: not found")

// CreateGoalInput carries the fields needed to create a Goal. The store assigns
// ID and CreatedAt.
type CreateGoalInput struct {
	UserID      domain.UUID
	Title       string
	Description string
	Type        domain.GoalType
	Cadence     domain.Cadence
	StakeAmount string
	TokenSymbol string
	Chain       string
	Timezone    string
	StartsAt    time.Time
	EndsAt      *time.Time
}

// UpdateGoalInput carries the mutable fields of a Goal. Identity (ID) selects
// the row; UserID is unchanged.
type UpdateGoalInput struct {
	ID          domain.UUID
	Title       string
	Description string
	StakeAmount string
	TokenSymbol string
	Chain       string
	EndsAt      *time.Time
}

// CreateViolationInput is the row written before any on-chain charge. It always
// starts in domain.ViolationPending.
type CreateViolationInput struct {
	GoalID domain.UUID
	Period domain.Period
	Amount string
	Reason string
}

// UpdateViolationInput transitions a violation's on-chain status (IV6).
type UpdateViolationInput struct {
	ID     domain.UUID
	Status domain.ViolationStatus
	TxHash string
}

// CreateApiKeyInput stores only the one-way hash and non-secret prefix (IV3).
// The raw key never reaches the store.
type CreateApiKeyInput struct {
	UserID  domain.UUID
	Name    string
	Prefix  string
	KeyHash string
}

type CreateTelegramLinkCodeInput struct {
	UserID    domain.UUID
	CodeHash  string
	ExpiresAt time.Time
}

type UpsertTelegramLinkInput struct {
	UserID   domain.UUID
	ChatID   int64
	ChatKind string
}

type CreateAgentLinkInput struct {
	UserID    domain.UUID
	APIKeyID  domain.UUID
	Name      string
	TokenHash string
	ExpiresAt time.Time
}

// Store is the full persistence interface consumed by the service layer. It is
// implemented by both *Memory and *Postgres.
type Store interface {
	// Users (AS1: wallet address is identity).
	CreateUser(ctx context.Context, walletAddress, timezone string) (domain.User, error)
	GetUser(ctx context.Context, id domain.UUID) (domain.User, error)
	GetUserByWallet(ctx context.Context, walletAddress string) (domain.User, error)

	// Goals.
	CreateGoal(ctx context.Context, in CreateGoalInput) (domain.Goal, error)
	GetGoal(ctx context.Context, id domain.UUID) (domain.Goal, error)
	GetGoalsByUser(ctx context.Context, userID domain.UUID) ([]domain.Goal, error)
	UpdateGoal(ctx context.Context, in UpdateGoalInput) (domain.Goal, error)
	ArchiveGoal(ctx context.Context, id domain.UUID) error
	ListActiveGoals(ctx context.Context) ([]domain.Goal, error)

	// Check-ins (one per goal+period).
	UpsertCheckIn(ctx context.Context, goalID domain.UUID, period domain.Period, note string) (domain.CheckIn, error)
	GetCheckIn(ctx context.Context, goalID domain.UUID, period domain.Period) (domain.CheckIn, error)

	// Violations. CreateViolation always inserts; CreateViolationIfAbsent is for
	// deadline-driven do-goal idempotency.
	CreateViolation(ctx context.Context, in CreateViolationInput) (domain.Violation, error)
	CreateViolationIfAbsent(ctx context.Context, in CreateViolationInput) (domain.Violation, bool, error)
	UpdateViolation(ctx context.Context, in UpdateViolationInput) (domain.Violation, error)
	ListViolations(ctx context.Context, goalID domain.UUID) ([]domain.Violation, error)

	// Wallet approvals (one per user+chain+token).
	UpsertWalletApproval(ctx context.Context, userID domain.UUID, chain, tokenSymbol, allowance string) (domain.WalletApproval, error)
	GetWalletApproval(ctx context.Context, userID domain.UUID, chain, tokenSymbol string) (domain.WalletApproval, error)

	// API keys (IV3: hash + prefix only).
	CreateApiKey(ctx context.Context, in CreateApiKeyInput) (domain.ApiKey, error)
	GetApiKeyByHash(ctx context.Context, keyHash string) (domain.ApiKey, error)
	TouchApiKey(ctx context.Context, id domain.UUID) (domain.ApiKey, error)
	ListApiKeysByUser(ctx context.Context, userID domain.UUID) ([]domain.ApiKey, error)
	RevokeApiKey(ctx context.Context, id domain.UUID) error

	// Telegram links (raw link codes are never stored).
	CreateTelegramLinkCode(ctx context.Context, in CreateTelegramLinkCodeInput) (domain.TelegramLinkCode, error)
	ConsumeTelegramLinkCode(ctx context.Context, codeHash string, now time.Time) (domain.TelegramLinkCode, error)
	UpsertTelegramLink(ctx context.Context, in UpsertTelegramLinkInput) (domain.TelegramLink, error)
	GetTelegramLinkByChatID(ctx context.Context, chatID int64) (domain.TelegramLink, error)

	// Agent links (raw URL tokens and raw agent secrets are never stored).
	CreateAgentLink(ctx context.Context, in CreateAgentLinkInput) (domain.AgentLink, error)
	ListAgentLinksByUser(ctx context.Context, userID domain.UUID) ([]domain.AgentLink, error)
	GetAgentLinkByTokenHash(ctx context.Context, tokenHash string) (domain.AgentLink, error)
	RevokeAgentLink(ctx context.Context, id domain.UUID) (domain.AgentLink, error)

	// Conversations & messages (AI coaching threads).
	CreateConversation(ctx context.Context, userID domain.UUID, title string) (domain.Conversation, error)
	AppendMessage(ctx context.Context, conversationID domain.UUID, role domain.MessageRole, content string) (domain.Message, error)
	GetConversation(ctx context.Context, id domain.UUID) (domain.Conversation, []domain.Message, error)
}
