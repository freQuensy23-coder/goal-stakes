package domain

import (
	"time"

	"github.com/google/uuid"
)

// UUID is an alias for the underlying UUID type so other packages can refer to
// domain.UUID without importing the uuid library directly (GPC4: one place
// decides the identity representation).
type UUID = uuid.UUID

// NewID returns a fresh random UUID. Centralized here so the store impls share
// one generator.
func NewID() UUID { return uuid.New() }

// User is identified by their wallet address (AS1: SIWE, no email/password).
// WalletAddress is stored normalized (lowercased hex) and is the natural key.
// Timezone is an optional IANA timezone used as the default for local daily and
// weekly periods. Blank means UTC.
type User struct {
	ID            uuid.UUID `json:"id"`
	WalletAddress string    `json:"wallet_address"`
	Timezone      string    `json:"timezone,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Goal is a commitment a user stakes money on. Type, Cadence, StakeAmount,
// Token and Chain together define how violations are detected and charged
// (AS4). StakeAmount is the on-chain token amount in the token's smallest unit
// (e.g. wei-equivalent), kept as a decimal string to avoid float/precision and
// int64-overflow issues across the wire and DB.
type Goal struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Type        GoalType  `json:"type"`
	Cadence     Cadence   `json:"cadence"`
	StakeAmount string    `json:"stake_amount"`
	TokenSymbol string    `json:"token_symbol"`
	Chain       string    `json:"chain"`
	// Timezone controls local daily/weekly period boundaries. Blank means UTC.
	Timezone  string     `json:"timezone,omitempty"`
	Archived  bool       `json:"archived"`
	CreatedAt time.Time  `json:"created_at"`
	StartsAt  time.Time  `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at,omitempty"`
}

// CheckIn is a user's proof-of-progress for a "do" goal in a given period.
// At most one per (goal, period) — enforced by a unique index in the store.
type CheckIn struct {
	ID        uuid.UUID `json:"id"`
	GoalID    uuid.UUID `json:"goal_id"`
	Period    Period    `json:"period"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Violation records a missed/broken goal for a period. It is always written
// before any on-chain charge (IV6) and starts in ViolationPending. The store
// enforces at most one row per (goal, period) via a unique index, which is what
// makes charging idempotent.
type Violation struct {
	ID        uuid.UUID       `json:"id"`
	GoalID    uuid.UUID       `json:"goal_id"`
	Period    Period          `json:"period"`
	Status    ViolationStatus `json:"status"`
	Amount    string          `json:"amount"`
	Reason    string          `json:"reason,omitempty"`
	TxHash    string          `json:"tx_hash,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// WalletApproval caches the most recent known on-chain allowance the user has
// granted the StakeEnforcer for a given token on a given chain. One row per
// (user, chain, token). Allowance is a decimal string (smallest unit).
type WalletApproval struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Chain       string    `json:"chain"`
	TokenSymbol string    `json:"token_symbol"`
	Allowance   string    `json:"allowance"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ApiKey models a programmatic credential. IV3: the raw key is NEVER persisted.
// We store only a one-way hash (KeyHash) plus a short non-secret Prefix used to
// help the user identify the key in a list. There is intentionally no raw-key
// field on this type.
type ApiKey struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	KeyHash   string     `json:"-"` // never serialized out
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// Revoked reports whether the key has been revoked.
func (k ApiKey) Revoked() bool { return k.RevokedAt != nil }

// TelegramLinkCode is a one-time code generated in authenticated settings and
// consumed from Telegram through the bot-owned internal endpoint. The raw code
// is never stored; CodeHash is the only persisted credential material.
type TelegramLinkCode struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	CodeHash   string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// TelegramLink maps one Telegram private chat, group, supergroup, or channel
// to a Goal Stakes user. Bot code resolves chat IDs through backend storage
// instead of storing user API keys.
type TelegramLink struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	ChatID    int64     `json:"chat_id"`
	ChatKind  string    `json:"chat_kind"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentLink is a private Markdown skill URL plus a generated API key for a
// user-owned external agent. The raw URL token and raw sk_ secret are never
// stored; TokenHash and the linked ApiKey.KeyHash are the only persisted
// credential material.
type AgentLink struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	APIKeyID  uuid.UUID  `json:"api_key_id"`
	Name      string     `json:"name"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func (l AgentLink) Revoked() bool { return l.RevokedAt != nil }

// Conversation is an AI coaching thread owned by a user (AS3 territory; the
// store just persists it). Messages are appended in order.
type Conversation struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message is a single turn within a Conversation.
type Message struct {
	ID             uuid.UUID   `json:"id"`
	ConversationID uuid.UUID   `json:"conversation_id"`
	Role           MessageRole `json:"role"`
	Content        string      `json:"content"`
	CreatedAt      time.Time   `json:"created_at"`
}
