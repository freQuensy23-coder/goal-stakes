package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"goalstakes/internal/domain"
)

// Memory is a goroutine-safe, in-memory Store implementation. It is the
// reference store used by service-layer unit tests (IF0). It mirrors the
// uniqueness and idempotency guarantees of the Postgres store: unique wallet,
// one check-in per (goal, period), and optional violation dedupe for do goals.
type Memory struct {
	mu sync.Mutex

	users                map[domain.UUID]domain.User
	walletIndex          map[string]domain.UUID
	goals                map[domain.UUID]domain.Goal
	checkIns             map[checkInKey]domain.CheckIn
	violations           map[domain.UUID]domain.Violation
	violationKey         map[goalPeriodKey]domain.UUID
	approvals            map[domain.UUID]domain.WalletApproval
	approvalKey          map[approvalKey]domain.UUID
	apiKeys              map[domain.UUID]domain.ApiKey
	apiKeyByHash         map[string]domain.UUID
	telegramCodes        map[domain.UUID]domain.TelegramLinkCode
	telegramCodeByHash   map[string]domain.UUID
	telegramLinks        map[domain.UUID]domain.TelegramLink
	telegramLinkByChatID map[int64]domain.UUID
	agentLinks           map[domain.UUID]domain.AgentLink
	agentLinkByTokenHash map[string]domain.UUID
	conversations        map[domain.UUID]domain.Conversation
	messages             map[domain.UUID][]domain.Message // by conversation ID, append order

	// ord records insertion order per row ID so list queries are deterministic
	// regardless of clock resolution; seq is its monotonic source.
	ord map[domain.UUID]int64
	seq int64
}

type checkInKey struct {
	goalID domain.UUID
	period domain.Period
}

type goalPeriodKey struct {
	goalID domain.UUID
	period domain.Period
}

type approvalKey struct {
	userID domain.UUID
	chain  string
	token  string
}

// NewMemory returns an empty in-memory Store.
func NewMemory() Store {
	return &Memory{
		users:                make(map[domain.UUID]domain.User),
		walletIndex:          make(map[string]domain.UUID),
		goals:                make(map[domain.UUID]domain.Goal),
		checkIns:             make(map[checkInKey]domain.CheckIn),
		violations:           make(map[domain.UUID]domain.Violation),
		violationKey:         make(map[goalPeriodKey]domain.UUID),
		approvals:            make(map[domain.UUID]domain.WalletApproval),
		approvalKey:          make(map[approvalKey]domain.UUID),
		apiKeys:              make(map[domain.UUID]domain.ApiKey),
		apiKeyByHash:         make(map[string]domain.UUID),
		telegramCodes:        make(map[domain.UUID]domain.TelegramLinkCode),
		telegramCodeByHash:   make(map[string]domain.UUID),
		telegramLinks:        make(map[domain.UUID]domain.TelegramLink),
		telegramLinkByChatID: make(map[int64]domain.UUID),
		agentLinks:           make(map[domain.UUID]domain.AgentLink),
		agentLinkByTokenHash: make(map[string]domain.UUID),
		conversations:        make(map[domain.UUID]domain.Conversation),
		messages:             make(map[domain.UUID][]domain.Message),
		ord:                  make(map[domain.UUID]int64),
	}
}

func (m *Memory) nextSeq() int64 {
	m.seq++
	return m.seq
}

func now() time.Time { return time.Now().UTC() }

func (m *Memory) CreateUser(_ context.Context, walletAddress, timezone string) (domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.walletIndex[walletAddress]; exists {
		return domain.User{}, fmt.Errorf("store: user with wallet %q already exists", walletAddress)
	}
	u := domain.User{
		ID:            domain.NewID(),
		WalletAddress: walletAddress,
		Timezone:      timezone,
		CreatedAt:     now(),
	}
	m.users[u.ID] = u
	m.walletIndex[walletAddress] = u.ID
	return u, nil
}

func (m *Memory) GetUserByWallet(_ context.Context, walletAddress string) (domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.walletIndex[walletAddress]
	if !ok {
		return domain.User{}, fmt.Errorf("get user by wallet %q: %w", walletAddress, ErrNotFound)
	}
	return m.users[id], nil
}

func (m *Memory) GetUser(_ context.Context, id domain.UUID) (domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return domain.User{}, fmt.Errorf("get user %s: %w", id, ErrNotFound)
	}
	return u, nil
}

func (m *Memory) CreateGoal(_ context.Context, in CreateGoalInput) (domain.Goal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[in.UserID]; !ok {
		return domain.Goal{}, fmt.Errorf("create goal: %w: user %s", ErrNotFound, in.UserID)
	}
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
	m.goals[g.ID] = g
	m.ord[g.ID] = m.nextSeq()
	return g, nil
}

func (m *Memory) GetGoal(_ context.Context, id domain.UUID) (domain.Goal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok {
		return domain.Goal{}, fmt.Errorf("get goal %s: %w", id, ErrNotFound)
	}
	return g, nil
}

func (m *Memory) GetGoalsByUser(_ context.Context, userID domain.UUID) ([]domain.Goal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Goal, 0)
	for _, g := range m.goals {
		if g.UserID == userID {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return m.ord[out[i].ID] < m.ord[out[j].ID] })
	return out, nil
}

func (m *Memory) UpdateGoal(_ context.Context, in UpdateGoalInput) (domain.Goal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[in.ID]
	if !ok {
		return domain.Goal{}, fmt.Errorf("update goal %s: %w", in.ID, ErrNotFound)
	}
	g.Title = in.Title
	g.Description = in.Description
	g.StakeAmount = in.StakeAmount
	if in.TokenSymbol != "" {
		g.TokenSymbol = in.TokenSymbol
	}
	if in.Chain != "" {
		g.Chain = in.Chain
	}
	g.EndsAt = in.EndsAt
	m.goals[g.ID] = g
	return g, nil
}

func (m *Memory) ArchiveGoal(_ context.Context, id domain.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok {
		return fmt.Errorf("archive goal %s: %w", id, ErrNotFound)
	}
	g.Archived = true
	m.goals[id] = g
	return nil
}

func (m *Memory) ListActiveGoals(_ context.Context) ([]domain.Goal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Goal, 0)
	for _, g := range m.goals {
		if !g.Archived {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return m.ord[out[i].ID] < m.ord[out[j].ID] })
	return out, nil
}

func (m *Memory) UpsertCheckIn(_ context.Context, goalID domain.UUID, period domain.Period, note string) (domain.CheckIn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.goals[goalID]; !ok {
		return domain.CheckIn{}, fmt.Errorf("upsert check-in: %w: goal %s", ErrNotFound, goalID)
	}
	key := checkInKey{goalID, period}
	existing, ok := m.checkIns[key]
	if ok {
		existing.Note = note
		m.checkIns[key] = existing
		return existing, nil
	}
	c := domain.CheckIn{
		ID:        domain.NewID(),
		GoalID:    goalID,
		Period:    period,
		Note:      note,
		CreatedAt: now(),
	}
	m.checkIns[key] = c
	return c, nil
}

func (m *Memory) GetCheckIn(_ context.Context, goalID domain.UUID, period domain.Period) (domain.CheckIn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.checkIns[checkInKey{goalID, period}]
	if !ok {
		return domain.CheckIn{}, fmt.Errorf("get check-in goal=%s period=%s: %w", goalID, period, ErrNotFound)
	}
	return c, nil
}

// CreateViolation always inserts a fresh Pending row. It is used for avoid goals
// where each self-reported slip is a separate paid violation.
func (m *Memory) CreateViolation(_ context.Context, in CreateViolationInput) (domain.Violation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.goals[in.GoalID]; !ok {
		return domain.Violation{}, fmt.Errorf("create violation: %w: goal %s", ErrNotFound, in.GoalID)
	}
	return m.createViolationLocked(in), nil
}

// CreateViolationIfAbsent is idempotent on (goal, period): the first call
// inserts a Pending row, every subsequent call returns that same row unchanged.
func (m *Memory) CreateViolationIfAbsent(_ context.Context, in CreateViolationInput) (domain.Violation, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.goals[in.GoalID]; !ok {
		return domain.Violation{}, false, fmt.Errorf("create violation: %w: goal %s", ErrNotFound, in.GoalID)
	}
	key := goalPeriodKey{in.GoalID, in.Period}
	if id, exists := m.violationKey[key]; exists {
		return m.violations[id], false, nil
	}
	v := m.createViolationLocked(in)
	m.violationKey[key] = v.ID
	return v, true, nil
}

func (m *Memory) createViolationLocked(in CreateViolationInput) domain.Violation {
	ts := now()
	v := domain.Violation{
		ID:        domain.NewID(),
		GoalID:    in.GoalID,
		Period:    in.Period,
		Status:    domain.ViolationPending,
		Amount:    in.Amount,
		Reason:    in.Reason,
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	m.violations[v.ID] = v
	m.ord[v.ID] = m.nextSeq()
	return v
}

func (m *Memory) UpdateViolation(_ context.Context, in UpdateViolationInput) (domain.Violation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.violations[in.ID]
	if !ok {
		return domain.Violation{}, fmt.Errorf("update violation %s: %w", in.ID, ErrNotFound)
	}
	v.Status = in.Status
	v.TxHash = in.TxHash
	v.UpdatedAt = now()
	m.violations[v.ID] = v
	return v, nil
}

func (m *Memory) ListViolations(_ context.Context, goalID domain.UUID) ([]domain.Violation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Violation, 0)
	for _, v := range m.violations {
		if v.GoalID == goalID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return m.ord[out[i].ID] < m.ord[out[j].ID] })
	return out, nil
}

func (m *Memory) UpsertWalletApproval(_ context.Context, userID domain.UUID, chain, tokenSymbol, allowance string) (domain.WalletApproval, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[userID]; !ok {
		return domain.WalletApproval{}, fmt.Errorf("upsert wallet approval: %w: user %s", ErrNotFound, userID)
	}
	key := approvalKey{userID, chain, tokenSymbol}
	if id, exists := m.approvalKey[key]; exists {
		a := m.approvals[id]
		a.Allowance = allowance
		a.UpdatedAt = now()
		m.approvals[id] = a
		return a, nil
	}
	a := domain.WalletApproval{
		ID:          domain.NewID(),
		UserID:      userID,
		Chain:       chain,
		TokenSymbol: tokenSymbol,
		Allowance:   allowance,
		UpdatedAt:   now(),
	}
	m.approvals[a.ID] = a
	m.approvalKey[key] = a.ID
	return a, nil
}

func (m *Memory) GetWalletApproval(_ context.Context, userID domain.UUID, chain, tokenSymbol string) (domain.WalletApproval, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.approvalKey[approvalKey{userID, chain, tokenSymbol}]
	if !ok {
		return domain.WalletApproval{}, fmt.Errorf("get wallet approval user=%s chain=%s token=%s: %w", userID, chain, tokenSymbol, ErrNotFound)
	}
	return m.approvals[id], nil
}

func (m *Memory) CreateApiKey(_ context.Context, in CreateApiKeyInput) (domain.ApiKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[in.UserID]; !ok {
		return domain.ApiKey{}, fmt.Errorf("create api key: %w: user %s", ErrNotFound, in.UserID)
	}
	if _, exists := m.apiKeyByHash[in.KeyHash]; exists {
		return domain.ApiKey{}, fmt.Errorf("store: api key hash already exists")
	}
	k := domain.ApiKey{
		ID:        domain.NewID(),
		UserID:    in.UserID,
		Name:      in.Name,
		Prefix:    in.Prefix,
		KeyHash:   in.KeyHash,
		CreatedAt: now(),
	}
	m.apiKeys[k.ID] = k
	m.apiKeyByHash[k.KeyHash] = k.ID
	m.ord[k.ID] = m.nextSeq()
	return k, nil
}

func (m *Memory) GetApiKeyByHash(_ context.Context, keyHash string) (domain.ApiKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.apiKeyByHash[keyHash]
	if !ok {
		return domain.ApiKey{}, fmt.Errorf("get api key by hash: %w", ErrNotFound)
	}
	return m.apiKeys[id], nil
}

func (m *Memory) TouchApiKey(_ context.Context, id domain.UUID) (domain.ApiKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.apiKeys[id]
	if !ok {
		return domain.ApiKey{}, fmt.Errorf("touch api key %s: %w", id, ErrNotFound)
	}
	t := now()
	k.LastUsed = &t
	m.apiKeys[id] = k
	return k, nil
}

func (m *Memory) ListApiKeysByUser(_ context.Context, userID domain.UUID) ([]domain.ApiKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.ApiKey, 0)
	for _, k := range m.apiKeys {
		if k.UserID == userID {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return m.ord[out[i].ID] < m.ord[out[j].ID] })
	return out, nil
}

func (m *Memory) RevokeApiKey(_ context.Context, id domain.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.apiKeys[id]
	if !ok {
		return fmt.Errorf("revoke api key %s: %w", id, ErrNotFound)
	}
	if k.RevokedAt == nil {
		t := now()
		k.RevokedAt = &t
		m.apiKeys[id] = k
	}
	return nil
}

func (m *Memory) CreateTelegramLinkCode(_ context.Context, in CreateTelegramLinkCodeInput) (domain.TelegramLinkCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[in.UserID]; !ok {
		return domain.TelegramLinkCode{}, fmt.Errorf("create telegram link code: %w: user %s", ErrNotFound, in.UserID)
	}
	if _, exists := m.telegramCodeByHash[in.CodeHash]; exists {
		return domain.TelegramLinkCode{}, fmt.Errorf("store: telegram link code hash already exists")
	}
	code := domain.TelegramLinkCode{
		ID:        domain.NewID(),
		UserID:    in.UserID,
		CodeHash:  in.CodeHash,
		ExpiresAt: in.ExpiresAt,
		CreatedAt: now(),
	}
	m.telegramCodes[code.ID] = code
	m.telegramCodeByHash[code.CodeHash] = code.ID
	return code, nil
}

func (m *Memory) ConsumeTelegramLinkCode(_ context.Context, codeHash string, consumedAt time.Time) (domain.TelegramLinkCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.telegramCodeByHash[codeHash]
	if !ok {
		return domain.TelegramLinkCode{}, fmt.Errorf("consume telegram link code: %w", ErrNotFound)
	}
	code := m.telegramCodes[id]
	if code.ConsumedAt != nil || !code.ExpiresAt.After(consumedAt) {
		return domain.TelegramLinkCode{}, fmt.Errorf("consume telegram link code: %w", ErrNotFound)
	}
	code.ConsumedAt = &consumedAt
	m.telegramCodes[id] = code
	return code, nil
}

func (m *Memory) UpsertTelegramLink(_ context.Context, in UpsertTelegramLinkInput) (domain.TelegramLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[in.UserID]; !ok {
		return domain.TelegramLink{}, fmt.Errorf("upsert telegram link: %w: user %s", ErrNotFound, in.UserID)
	}
	ts := now()
	if id, exists := m.telegramLinkByChatID[in.ChatID]; exists {
		link := m.telegramLinks[id]
		link.UserID = in.UserID
		link.ChatKind = in.ChatKind
		link.UpdatedAt = ts
		m.telegramLinks[id] = link
		return link, nil
	}
	link := domain.TelegramLink{
		ID:        domain.NewID(),
		UserID:    in.UserID,
		ChatID:    in.ChatID,
		ChatKind:  in.ChatKind,
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	m.telegramLinks[link.ID] = link
	m.telegramLinkByChatID[link.ChatID] = link.ID
	return link, nil
}

func (m *Memory) GetTelegramLinkByChatID(_ context.Context, chatID int64) (domain.TelegramLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.telegramLinkByChatID[chatID]
	if !ok {
		return domain.TelegramLink{}, fmt.Errorf("get telegram link chat=%d: %w", chatID, ErrNotFound)
	}
	return m.telegramLinks[id], nil
}

func (m *Memory) CreateAgentLink(_ context.Context, in CreateAgentLinkInput) (domain.AgentLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[in.UserID]; !ok {
		return domain.AgentLink{}, fmt.Errorf("create agent link: %w: user %s", ErrNotFound, in.UserID)
	}
	if _, ok := m.apiKeys[in.APIKeyID]; !ok {
		return domain.AgentLink{}, fmt.Errorf("create agent link: %w: api key %s", ErrNotFound, in.APIKeyID)
	}
	if _, exists := m.agentLinkByTokenHash[in.TokenHash]; exists {
		return domain.AgentLink{}, fmt.Errorf("store: agent link token hash already exists")
	}
	link := domain.AgentLink{
		ID:        domain.NewID(),
		UserID:    in.UserID,
		APIKeyID:  in.APIKeyID,
		Name:      in.Name,
		TokenHash: in.TokenHash,
		ExpiresAt: in.ExpiresAt,
		CreatedAt: now(),
	}
	m.agentLinks[link.ID] = link
	m.agentLinkByTokenHash[link.TokenHash] = link.ID
	m.ord[link.ID] = m.nextSeq()
	return link, nil
}

func (m *Memory) ListAgentLinksByUser(_ context.Context, userID domain.UUID) ([]domain.AgentLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.AgentLink, 0)
	for _, link := range m.agentLinks {
		if link.UserID == userID {
			out = append(out, link)
		}
	}
	sort.Slice(out, func(i, j int) bool { return m.ord[out[i].ID] < m.ord[out[j].ID] })
	return out, nil
}

func (m *Memory) GetAgentLinkByTokenHash(_ context.Context, tokenHash string) (domain.AgentLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.agentLinkByTokenHash[tokenHash]
	if !ok {
		return domain.AgentLink{}, fmt.Errorf("get agent link by token hash: %w", ErrNotFound)
	}
	return m.agentLinks[id], nil
}

func (m *Memory) RevokeAgentLink(_ context.Context, id domain.UUID) (domain.AgentLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	link, ok := m.agentLinks[id]
	if !ok {
		return domain.AgentLink{}, fmt.Errorf("revoke agent link %s: %w", id, ErrNotFound)
	}
	if link.RevokedAt == nil {
		t := now()
		link.RevokedAt = &t
		m.agentLinks[id] = link
	}
	return link, nil
}

func (m *Memory) CreateConversation(_ context.Context, userID domain.UUID, title string) (domain.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[userID]; !ok {
		return domain.Conversation{}, fmt.Errorf("create conversation: %w: user %s", ErrNotFound, userID)
	}
	ts := now()
	c := domain.Conversation{
		ID:        domain.NewID(),
		UserID:    userID,
		Title:     title,
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	m.conversations[c.ID] = c
	return c, nil
}

func (m *Memory) AppendMessage(_ context.Context, conversationID domain.UUID, role domain.MessageRole, content string) (domain.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conv, ok := m.conversations[conversationID]
	if !ok {
		return domain.Message{}, fmt.Errorf("append message: %w: conversation %s", ErrNotFound, conversationID)
	}
	msg := domain.Message{
		ID:             domain.NewID(),
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		CreatedAt:      now(),
	}
	m.messages[conversationID] = append(m.messages[conversationID], msg)
	conv.UpdatedAt = msg.CreatedAt
	m.conversations[conversationID] = conv
	return msg, nil
}

func (m *Memory) GetConversation(_ context.Context, id domain.UUID) (domain.Conversation, []domain.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conv, ok := m.conversations[id]
	if !ok {
		return domain.Conversation{}, nil, fmt.Errorf("get conversation %s: %w", id, ErrNotFound)
	}
	// Return a copy so callers can't mutate internal state.
	msgs := make([]domain.Message, len(m.messages[id]))
	copy(msgs, m.messages[id])
	return conv, msgs, nil
}
