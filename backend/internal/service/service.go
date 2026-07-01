// Package service owns goal-stakes business logic. HTTP handlers, AI tools,
// schedulers, and clients should call this layer instead of reimplementing
// behavior at the edge (IV5).
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"goalstakes/internal/auth"
	"goalstakes/internal/config"
	"goalstakes/internal/domain"
	"goalstakes/internal/store"
)

var (
	ErrInvalid      = errors.New("service: invalid input")
	ErrForbidden    = errors.New("service: forbidden")
	ErrChargeFailed = errors.New("service: charge failed")
	ErrExpired      = errors.New("service: expired")
)

const agentLinkTTL = 90 * 24 * time.Hour

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

type Service struct {
	store           store.Store
	chains          map[string]config.ChainConfig
	now             func() time.Time
	charger         PenaltyCharger
	approvalChecker ApprovalChecker
}

// PenaltyCharger is the on-chain boundary used after a Violation row exists.
type PenaltyCharger interface {
	Penalize(ctx context.Context, chain, userWallet, tokenAddress, amount string) (txHash string, err error)
}

// ApprovalChecker is the on-chain boundary for live token allowance reads.
type ApprovalChecker interface {
	AllowanceOf(ctx context.Context, chain, userWallet, tokenAddress string) (*big.Int, error)
}

func WithPenaltyCharger(charger PenaltyCharger) Option {
	return func(s *Service) {
		s.charger = charger
	}
}

func WithApprovalChecker(checker ApprovalChecker) Option {
	return func(s *Service) {
		s.approvalChecker = checker
	}
}

func New(st store.Store, chains map[string]config.ChainConfig, opts ...Option) (*Service, error) {
	if st == nil {
		return nil, errors.New("service: store is required")
	}
	if len(chains) == 0 {
		return nil, errors.New("service: at least one chain is required")
	}
	s := &Service{
		store:  st,
		chains: chains,
		now:    func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

type CreateGoalInput struct {
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Type        domain.GoalType `json:"type"`
	Cadence     domain.Cadence  `json:"cadence"`
	StakeAmount string          `json:"stake_amount"`
	TokenSymbol string          `json:"token_symbol"`
	Chain       string          `json:"chain"`
	Timezone    string          `json:"timezone,omitempty"`
	StartsAt    time.Time       `json:"starts_at,omitempty"`
	EndsAt      *time.Time      `json:"ends_at,omitempty"`
}

type UpdateGoalInput struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	StakeAmount string     `json:"stake_amount"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	EndsAtSet   bool       `json:"-"`
}

func (in *UpdateGoalInput) UnmarshalJSON(raw []byte) error {
	type alias UpdateGoalInput
	var decoded alias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	decoded.EndsAtSet = false
	if value, ok := fields["ends_at"]; ok {
		decoded.EndsAtSet = true
		if string(value) == "null" {
			decoded.EndsAt = nil
		}
	}
	*in = UpdateGoalInput(decoded)
	return nil
}

type LogCheckInInput struct {
	Period domain.Period `json:"period,omitempty"`
	Note   string        `json:"note,omitempty"`
}

type ReportViolationInput struct {
	Period domain.Period `json:"period,omitempty"`
	Reason string        `json:"reason,omitempty"`
}

type SetStakeInput struct {
	StakeAmount string `json:"stake_amount"`
	TokenSymbol string `json:"token_symbol"`
	Chain       string `json:"chain"`
}

type RecordApprovalInput struct {
	Chain           string `json:"chain"`
	TokenSymbol     string `json:"token_symbol"`
	TxHash          string `json:"tx_hash"`
	DryRunAllowance string `json:"dry_run_allowance,omitempty"`
}

type ApprovalStatus struct {
	Chain       string `json:"chain"`
	TokenSymbol string `json:"token_symbol"`
	Allowance   string `json:"allowance"`
	Approved    bool   `json:"approved"`
}

type ChainInfo struct {
	Key                  string            `json:"key"`
	StakeEnforcerAddress string            `json:"stake_enforcer_address"`
	Tokens               map[string]string `json:"tokens"`
}

type Progress struct {
	Goal                   domain.Goal        `json:"goal"`
	CurrentPeriod          domain.Period      `json:"current_period"`
	CurrentPeriodCheckIn   *domain.CheckIn    `json:"current_period_check_in,omitempty"`
	CurrentPeriodCompleted bool               `json:"current_period_completed"`
	Violations             []domain.Violation `json:"violations"`
}

type CreatedAPIKey struct {
	Raw string        `json:"key"`
	Key domain.ApiKey `json:"api_key"`
}

type CreatedTelegramLinkCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

type CreateAgentLinkInput struct {
	Name string `json:"name"`
}

type AgentLinkMetadata struct {
	ID        domain.UUID `json:"id"`
	APIKeyID  domain.UUID `json:"api_key_id"`
	Name      string      `json:"name"`
	ExpiresAt time.Time   `json:"expires_at"`
	CreatedAt time.Time   `json:"created_at"`
	RevokedAt *time.Time  `json:"revoked_at,omitempty"`
}

type CreatedAgentLink struct {
	SkillURL  string            `json:"skill_url"`
	AgentLink AgentLinkMetadata `json:"agent_link"`
}

type LinkTelegramChatInput struct {
	Code     string `json:"code"`
	ChatID   int64  `json:"chat_id"`
	ChatKind string `json:"chat_kind"`
}

func (s *Service) ListChains() []ChainInfo {
	keys := make([]string, 0, len(s.chains))
	for key := range s.chains {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	chains := make([]ChainInfo, 0, len(keys))
	for _, key := range keys {
		cfg := s.chains[key]
		tokens := make(map[string]string, len(cfg.Tokens))
		for symbol, address := range cfg.Tokens {
			tokens[symbol] = address
		}
		chains = append(chains, ChainInfo{
			Key:                  key,
			StakeEnforcerAddress: cfg.StakeEnforcerAddress,
			Tokens:               tokens,
		})
	}
	return chains
}

func (s *Service) CreateGoal(ctx context.Context, userID domain.UUID, in CreateGoalInput) (domain.Goal, error) {
	if strings.TrimSpace(in.Title) == "" {
		return domain.Goal{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if !in.Type.Valid() {
		return domain.Goal{}, fmt.Errorf("%w: invalid goal type %q", ErrInvalid, in.Type)
	}
	if !in.Cadence.Valid() {
		return domain.Goal{}, fmt.Errorf("%w: invalid cadence %q", ErrInvalid, in.Cadence)
	}
	if in.Cadence == domain.CadenceCustom {
		return domain.Goal{}, fmt.Errorf("%w: custom cadence is not supported for goal creation", ErrInvalid)
	}
	timezone, err := normalizeTimezone(in.Timezone)
	if err != nil {
		return domain.Goal{}, err
	}
	token, err := s.validateStake(in.Chain, in.TokenSymbol, in.StakeAmount)
	if err != nil {
		return domain.Goal{}, err
	}
	if err := s.ensureAllowanceCoversStake(ctx, userID, in.Chain, token, in.StakeAmount); err != nil {
		return domain.Goal{}, err
	}
	startsAt := in.StartsAt
	if startsAt.IsZero() {
		startsAt = s.now().UTC()
	}
	return s.store.CreateGoal(ctx, store.CreateGoalInput{
		UserID:      userID,
		Title:       strings.TrimSpace(in.Title),
		Description: in.Description,
		Type:        in.Type,
		Cadence:     in.Cadence,
		StakeAmount: in.StakeAmount,
		TokenSymbol: token,
		Chain:       in.Chain,
		Timezone:    timezone,
		StartsAt:    startsAt,
		EndsAt:      in.EndsAt,
	})
}

func (s *Service) ListGoals(ctx context.Context, userID domain.UUID) ([]domain.Goal, error) {
	goals, err := s.store.GetGoalsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	active := make([]domain.Goal, 0, len(goals))
	for _, goal := range goals {
		if !goal.Archived {
			active = append(active, goal)
		}
	}
	return active, nil
}

func (s *Service) UpdateGoal(ctx context.Context, userID, goalID domain.UUID, in UpdateGoalInput) (domain.Goal, error) {
	goal, err := s.requireGoalOwner(ctx, userID, goalID)
	if err != nil {
		return domain.Goal{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return domain.Goal{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	stake := in.StakeAmount
	if stake == "" {
		stake = goal.StakeAmount
	}
	if err := validateAmount(stake); err != nil {
		return domain.Goal{}, err
	}
	if stake != goal.StakeAmount {
		if err := s.ensureAllowanceCoversStake(ctx, userID, goal.Chain, goal.TokenSymbol, stake); err != nil {
			return domain.Goal{}, err
		}
	}
	endsAt := goal.EndsAt
	if in.EndsAtSet {
		endsAt = in.EndsAt
	}
	return s.store.UpdateGoal(ctx, store.UpdateGoalInput{
		ID:          goal.ID,
		Title:       title,
		Description: in.Description,
		StakeAmount: stake,
		EndsAt:      endsAt,
	})
}

func (s *Service) ArchiveGoal(ctx context.Context, userID, goalID domain.UUID) error {
	if _, err := s.requireGoalOwner(ctx, userID, goalID); err != nil {
		return err
	}
	return s.store.ArchiveGoal(ctx, goalID)
}

func (s *Service) LogCheckIn(ctx context.Context, userID, goalID domain.UUID, in LogCheckInInput) (domain.CheckIn, error) {
	goal, err := s.requireGoalOwner(ctx, userID, goalID)
	if err != nil {
		return domain.CheckIn{}, err
	}
	period, err := s.resolvePeriod(goal, in.Period)
	if err != nil {
		return domain.CheckIn{}, err
	}
	return s.store.UpsertCheckIn(ctx, goal.ID, period, in.Note)
}

func (s *Service) ReportViolation(ctx context.Context, userID, goalID domain.UUID, in ReportViolationInput) (domain.Violation, error) {
	goal, err := s.requireGoalOwner(ctx, userID, goalID)
	if err != nil {
		return domain.Violation{}, err
	}
	period, err := s.resolvePeriod(goal, in.Period)
	if err != nil {
		return domain.Violation{}, err
	}
	violationInput := store.CreateViolationInput{
		GoalID: goal.ID,
		Period: period,
		Amount: goal.StakeAmount,
		Reason: in.Reason,
	}
	var violation domain.Violation
	var created bool
	if goal.Type == domain.GoalAvoid {
		violation, err = s.store.CreateViolation(ctx, violationInput)
		created = err == nil
	} else {
		violation, created, err = s.store.CreateViolationIfAbsent(ctx, violationInput)
	}
	if err != nil {
		return domain.Violation{}, err
	}
	if !created || s.charger == nil {
		return violation, nil
	}

	user, err := s.store.GetUser(ctx, goal.UserID)
	if err != nil {
		return violation, err
	}
	tokenAddress := s.chains[goal.Chain].Tokens[goal.TokenSymbol]
	txHash, chargeErr := s.charger.Penalize(ctx, goal.Chain, user.WalletAddress, tokenAddress, goal.StakeAmount)
	status := domain.ViolationCharged
	if chargeErr != nil {
		status = domain.ViolationFailed
	}
	updated, updateErr := s.store.UpdateViolation(ctx, store.UpdateViolationInput{
		ID:     violation.ID,
		Status: status,
		TxHash: txHash,
	})
	if updateErr != nil {
		return violation, updateErr
	}
	if chargeErr != nil {
		return updated, fmt.Errorf("%w: charge violation %s: %v", ErrChargeFailed, violation.ID, chargeErr)
	}
	return updated, nil
}

func (s *Service) ListViolations(ctx context.Context, userID, goalID domain.UUID) ([]domain.Violation, error) {
	goal, err := s.requireGoalOwner(ctx, userID, goalID)
	if err != nil {
		return nil, err
	}
	return s.store.ListViolations(ctx, goal.ID)
}

func (s *Service) GetProgress(ctx context.Context, userID, goalID domain.UUID) (Progress, error) {
	goal, err := s.requireGoalOwner(ctx, userID, goalID)
	if err != nil {
		return Progress{}, err
	}
	period, err := s.resolvePeriod(goal, "")
	if err != nil {
		return Progress{}, err
	}
	var checkIn *domain.CheckIn
	if c, err := s.store.GetCheckIn(ctx, goal.ID, period); err == nil {
		checkIn = &c
	} else if !errors.Is(err, store.ErrNotFound) {
		return Progress{}, err
	}
	violations, err := s.store.ListViolations(ctx, goal.ID)
	if err != nil {
		return Progress{}, err
	}
	return Progress{
		Goal:                   goal,
		CurrentPeriod:          period,
		CurrentPeriodCheckIn:   checkIn,
		CurrentPeriodCompleted: checkIn != nil,
		Violations:             violations,
	}, nil
}

func (s *Service) SetStake(ctx context.Context, userID, goalID domain.UUID, in SetStakeInput) (domain.Goal, error) {
	goal, err := s.requireGoalOwner(ctx, userID, goalID)
	if err != nil {
		return domain.Goal{}, err
	}
	token, err := s.validateStake(in.Chain, in.TokenSymbol, in.StakeAmount)
	if err != nil {
		return domain.Goal{}, err
	}
	if err := s.ensureAllowanceCoversStake(ctx, userID, in.Chain, token, in.StakeAmount); err != nil {
		return domain.Goal{}, err
	}
	return s.store.UpdateGoal(ctx, store.UpdateGoalInput{
		ID:          goal.ID,
		Title:       goal.Title,
		Description: goal.Description,
		StakeAmount: in.StakeAmount,
		TokenSymbol: token,
		Chain:       in.Chain,
		EndsAt:      goal.EndsAt,
	})
}

func (s *Service) GetApprovalStatus(ctx context.Context, userID domain.UUID, chain, tokenSymbol string) (ApprovalStatus, error) {
	token, err := s.validateToken(chain, tokenSymbol)
	if err != nil {
		return ApprovalStatus{}, err
	}
	if s.approvalChecker != nil {
		user, err := s.store.GetUser(ctx, userID)
		if err != nil {
			return ApprovalStatus{}, err
		}
		allowance, err := s.approvalChecker.AllowanceOf(ctx, chain, user.WalletAddress, s.chains[chain].Tokens[token])
		if err != nil {
			return ApprovalStatus{}, err
		}
		if allowance == nil {
			return ApprovalStatus{}, errors.New("service: approval checker returned nil allowance")
		}
		approval, err := s.store.UpsertWalletApproval(ctx, userID, chain, token, allowance.String())
		if err != nil {
			return ApprovalStatus{}, err
		}
		return ApprovalStatus{Chain: approval.Chain, TokenSymbol: approval.TokenSymbol, Allowance: approval.Allowance, Approved: allowance.Sign() != 0}, nil
	}
	approval, err := s.store.GetWalletApproval(ctx, userID, chain, token)
	if errors.Is(err, store.ErrNotFound) {
		return ApprovalStatus{Chain: chain, TokenSymbol: token, Allowance: "0", Approved: false}, nil
	}
	if err != nil {
		return ApprovalStatus{}, err
	}
	return ApprovalStatus{Chain: chain, TokenSymbol: token, Allowance: approval.Allowance, Approved: approval.Allowance != "0"}, nil
}

func (s *Service) RecordApproval(ctx context.Context, userID domain.UUID, in RecordApprovalInput) (ApprovalStatus, error) {
	token, err := s.validateToken(in.Chain, in.TokenSymbol)
	if err != nil {
		return ApprovalStatus{}, err
	}
	txHash := strings.TrimSpace(in.TxHash)
	if txHash == "" {
		return ApprovalStatus{}, fmt.Errorf("%w: tx_hash is required", ErrInvalid)
	}
	if s.approvalChecker != nil {
		return s.GetApprovalStatus(ctx, userID, in.Chain, token)
	}
	dryRunAllowance := strings.TrimSpace(in.DryRunAllowance)
	if dryRunAllowance == "" {
		return ApprovalStatus{}, fmt.Errorf("%w: dry_run_allowance is required when live allowance checker is disabled", ErrInvalid)
	}
	if err := validateAmountOrZero(dryRunAllowance); err != nil {
		return ApprovalStatus{}, err
	}
	approval, err := s.store.UpsertWalletApproval(ctx, userID, in.Chain, token, dryRunAllowance)
	if err != nil {
		return ApprovalStatus{}, err
	}
	return ApprovalStatus{Chain: approval.Chain, TokenSymbol: approval.TokenSymbol, Allowance: approval.Allowance, Approved: approval.Allowance != "0"}, nil
}

func (s *Service) CreateAPIKey(ctx context.Context, userID domain.UUID, name string) (CreatedAPIKey, error) {
	raw, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		return CreatedAPIKey{}, err
	}
	key, err := s.store.CreateApiKey(ctx, store.CreateApiKeyInput{
		UserID:  userID,
		Name:    strings.TrimSpace(name),
		Prefix:  prefix,
		KeyHash: hash,
	})
	if err != nil {
		return CreatedAPIKey{}, err
	}
	return CreatedAPIKey{Raw: raw, Key: key}, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, userID domain.UUID) ([]domain.ApiKey, error) {
	keys, err := s.store.ListApiKeysByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	active := make([]domain.ApiKey, 0, len(keys))
	for _, key := range keys {
		if !key.Revoked() {
			active = append(active, key)
		}
	}
	return active, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, userID, apiKeyID domain.UUID) error {
	keys, err := s.store.ListApiKeysByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if key.ID == apiKeyID {
			return s.store.RevokeApiKey(ctx, apiKeyID)
		}
	}
	return fmt.Errorf("%w: api key does not belong to user", ErrForbidden)
}

func (s *Service) CreateTelegramLinkCode(ctx context.Context, userID domain.UUID) (CreatedTelegramLinkCode, error) {
	code, err := generateTelegramLinkCode()
	if err != nil {
		return CreatedTelegramLinkCode{}, err
	}
	expiresAt := s.now().Add(10 * time.Minute)
	if _, err := s.store.CreateTelegramLinkCode(ctx, store.CreateTelegramLinkCodeInput{
		UserID:    userID,
		CodeHash:  hashTelegramLinkCode(code),
		ExpiresAt: expiresAt,
	}); err != nil {
		return CreatedTelegramLinkCode{}, err
	}
	return CreatedTelegramLinkCode{Code: code, ExpiresAt: expiresAt}, nil
}

func (s *Service) LinkTelegramChat(ctx context.Context, in LinkTelegramChatInput) (domain.TelegramLink, error) {
	code := normalizeTelegramLinkCode(in.Code)
	if code == "" {
		return domain.TelegramLink{}, fmt.Errorf("%w: telegram link code is required", ErrInvalid)
	}
	if in.ChatID == 0 {
		return domain.TelegramLink{}, fmt.Errorf("%w: telegram chat_id is required", ErrInvalid)
	}
	kind, err := normalizeTelegramChatKind(in.ChatKind)
	if err != nil {
		return domain.TelegramLink{}, err
	}
	consumed, err := s.store.ConsumeTelegramLinkCode(ctx, hashTelegramLinkCode(code), s.now())
	if err != nil {
		return domain.TelegramLink{}, fmt.Errorf("%w: invalid or expired telegram link code", ErrInvalid)
	}
	return s.store.UpsertTelegramLink(ctx, store.UpsertTelegramLinkInput{
		UserID:   consumed.UserID,
		ChatID:   in.ChatID,
		ChatKind: kind,
	})
}

func (s *Service) ResolveTelegramChat(ctx context.Context, chatID int64) (domain.TelegramLink, error) {
	if chatID == 0 {
		return domain.TelegramLink{}, fmt.Errorf("%w: telegram chat_id is required", ErrInvalid)
	}
	return s.store.GetTelegramLinkByChatID(ctx, chatID)
}

func (s *Service) CreateAgentLink(ctx context.Context, userID domain.UUID, in CreateAgentLinkInput, baseURL string) (CreatedAgentLink, error) {
	token, err := generateAgentSkillToken()
	if err != nil {
		return CreatedAgentLink{}, err
	}
	rawAgentSecret := deriveAgentAPIKey(token)
	prefix := rawAgentSecret
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	apiKey, err := s.store.CreateApiKey(ctx, store.CreateApiKeyInput{
		UserID:  userID,
		Name:    agentAPIKeyName(in.Name),
		Prefix:  prefix,
		KeyHash: auth.HashAPIKey(rawAgentSecret),
	})
	if err != nil {
		return CreatedAgentLink{}, err
	}
	link, err := s.store.CreateAgentLink(ctx, store.CreateAgentLinkInput{
		UserID:    userID,
		APIKeyID:  apiKey.ID,
		Name:      strings.TrimSpace(in.Name),
		TokenHash: hashAgentSkillToken(token),
		ExpiresAt: s.now().Add(agentLinkTTL),
	})
	if err != nil {
		_ = s.store.RevokeApiKey(ctx, apiKey.ID)
		return CreatedAgentLink{}, err
	}
	return CreatedAgentLink{
		SkillURL:  agentSkillURL(baseURL, token),
		AgentLink: agentLinkMetadata(link),
	}, nil
}

func (s *Service) ListAgentLinks(ctx context.Context, userID domain.UUID) ([]AgentLinkMetadata, error) {
	links, err := s.store.ListAgentLinksByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]AgentLinkMetadata, 0, len(links))
	now := s.now()
	for _, link := range links {
		if link.Revoked() || !link.ExpiresAt.After(now) {
			continue
		}
		out = append(out, agentLinkMetadata(link))
	}
	return out, nil
}

func (s *Service) RevokeAgentLink(ctx context.Context, userID, agentLinkID domain.UUID) error {
	links, err := s.store.ListAgentLinksByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, link := range links {
		if link.ID == agentLinkID {
			revoked, err := s.store.RevokeAgentLink(ctx, link.ID)
			if err != nil {
				return err
			}
			return s.store.RevokeApiKey(ctx, revoked.APIKeyID)
		}
	}
	return fmt.Errorf("%w: agent link does not belong to user", ErrForbidden)
}

func (s *Service) AgentSkillMarkdown(ctx context.Context, rawToken, baseURL string) (string, error) {
	token := strings.TrimSpace(rawToken)
	if token == "" {
		return "", fmt.Errorf("%w: agent skill token is required", ErrInvalid)
	}
	link, err := s.store.GetAgentLinkByTokenHash(ctx, hashAgentSkillToken(token))
	if err != nil {
		return "", err
	}
	if link.Revoked() {
		return "", fmt.Errorf("agent link revoked: %w", store.ErrNotFound)
	}
	if !link.ExpiresAt.After(s.now()) {
		return "", fmt.Errorf("%w: agent link expired", ErrExpired)
	}
	return buildAgentSkillMarkdown(baseURL, deriveAgentAPIKey(token)), nil
}

func (s *Service) requireGoalOwner(ctx context.Context, userID, goalID domain.UUID) (domain.Goal, error) {
	goal, err := s.store.GetGoal(ctx, goalID)
	if err != nil {
		return domain.Goal{}, err
	}
	if goal.UserID != userID {
		return domain.Goal{}, fmt.Errorf("%w: goal does not belong to user", ErrForbidden)
	}
	if goal.Archived {
		return domain.Goal{}, fmt.Errorf("%w: goal is archived", ErrInvalid)
	}
	return goal, nil
}

func (s *Service) resolvePeriod(goal domain.Goal, period domain.Period) (domain.Period, error) {
	if period != "" {
		if _, _, err := goal.PeriodBounds(period); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		return period, nil
	}
	period = goal.CurrentPeriod(s.now())
	if period == "" {
		return "", fmt.Errorf("%w: custom cadence requires explicit period", ErrInvalid)
	}
	return period, nil
}

func (s *Service) validateStake(chain, tokenSymbol, amount string) (string, error) {
	token, err := s.validateToken(chain, tokenSymbol)
	if err != nil {
		return "", err
	}
	if err := validateAmount(amount); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) ensureAllowanceCoversStake(ctx context.Context, userID domain.UUID, chain, tokenSymbol, stakeAmount string) error {
	status, err := s.GetApprovalStatus(ctx, userID, chain, tokenSymbol)
	if err != nil {
		return err
	}
	allowance, ok := new(big.Int).SetString(status.Allowance, 10)
	if !ok || allowance.Sign() < 0 {
		return fmt.Errorf("%w: approval allowance is invalid", ErrInvalid)
	}
	stake, ok := new(big.Int).SetString(stakeAmount, 10)
	if !ok || stake.Sign() <= 0 {
		return fmt.Errorf("%w: amount must be positive", ErrInvalid)
	}
	if allowance.Cmp(stake) < 0 {
		return fmt.Errorf("%w: approval allowance %s is below stake amount %s for %s on %s", ErrInvalid, status.Allowance, stakeAmount, tokenSymbol, chain)
	}
	return nil
}

func (s *Service) validateToken(chain, tokenSymbol string) (string, error) {
	chainCfg, ok := s.chains[chain]
	if !ok {
		return "", fmt.Errorf("%w: unknown chain %q", ErrInvalid, chain)
	}
	token := strings.ToUpper(strings.TrimSpace(tokenSymbol))
	if token == "" {
		return "", fmt.Errorf("%w: token_symbol is required", ErrInvalid)
	}
	if _, ok := chainCfg.Tokens[token]; !ok {
		return "", fmt.Errorf("%w: token %q is not allowed on chain %q", ErrInvalid, token, chain)
	}
	return token, nil
}

func validateAmount(amount string) error {
	if err := validateAmountOrZero(amount); err != nil {
		return err
	}
	n, _ := new(big.Int).SetString(amount, 10)
	if n.Sign() <= 0 {
		return fmt.Errorf("%w: amount must be positive", ErrInvalid)
	}
	return nil
}

func validateAmountOrZero(amount string) error {
	if strings.TrimSpace(amount) == "" {
		return fmt.Errorf("%w: amount is required", ErrInvalid)
	}
	n, ok := new(big.Int).SetString(amount, 10)
	if !ok || n.Sign() < 0 || n.String() != amount {
		return fmt.Errorf("%w: amount must be a non-negative integer string", ErrInvalid)
	}
	return nil
}

func generateTelegramLinkCode() (string, error) {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate telegram link code: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random), nil
}

func normalizeTelegramLinkCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func hashTelegramLinkCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeTelegramLinkCode(code)))
	return hex.EncodeToString(sum[:])
}

func generateAgentSkillToken() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate agent skill token: %w", err)
	}
	return "agt_" + base64.RawURLEncoding.EncodeToString(random), nil
}

func hashAgentSkillToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func deriveAgentAPIKey(token string) string {
	sum := sha256.Sum256([]byte("goalstakes-agent-api-key-v1:" + strings.TrimSpace(token)))
	return "sk_" + base64.RawURLEncoding.EncodeToString(sum[:])
}

func agentAPIKeyName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "own agent"
	}
	return "own agent: " + name
}

func agentLinkMetadata(link domain.AgentLink) AgentLinkMetadata {
	return AgentLinkMetadata{
		ID:        link.ID,
		APIKeyID:  link.APIKeyID,
		Name:      link.Name,
		ExpiresAt: link.ExpiresAt,
		CreatedAt: link.CreatedAt,
		RevokedAt: link.RevokedAt,
	}
}

func agentSkillURL(baseURL, token string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "/agent-skills/" + token + ".md"
	}
	return base + "/agent-skills/" + token + ".md"
}

func buildAgentSkillMarkdown(baseURL, rawAgentSecret string) string {
	apiBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if apiBase == "" {
		apiBase = "https://api.goalstakes.example"
	}
	return fmt.Sprintf(`---
name: goal-stakes-user-agent
description: Use when this user asks to manage Goal Stakes goals, check progress, send reminders, or record goal updates through the Goal Stakes API.
---

# Goal Stakes User Agent Skill

Use this skill when helping this user manage Goal Stakes.

## Project

Goal Stakes lets the user create do and avoid goals with USDC/USDT stake. If a goal is missed, the backend records a violation and burns the stake through StakeEnforcer. No frontend, mobile app, bot, or external agent owns AI keys, RPC URLs, wallet secrets, or contract signer keys.

## API

API base: %s
Authorization: Bearer %s

Use only the Goal Stakes backend API. Do not call OpenAI, RPC nodes, wallets, or contracts directly for this user.

Supported endpoints:
- GET /api/v1/goals
- POST /api/v1/goals
- PATCH /api/v1/goals/{goalID}
- DELETE /api/v1/goals/{goalID}
- GET /api/v1/goals/{goalID}/progress
- POST /api/v1/goals/{goalID}/checkins
- POST /api/v1/goals/{goalID}/violations
- POST /api/v1/chat
- POST /api/v1/chat/audio

## Safety Rules

- Never ask for wallet seed phrases or private keys.
- Never create a staked goal or increase stake without clear user confirmation.
- Never mark a goal done unless the user explicitly says they completed it.
- Never report a violation unless the user explicitly says they missed or broke the goal.
- Treat this Markdown file and its URL as private bearer secrets.

## Daily Cron

- Run once per day in the user's timezone.
- Call GET /api/v1/goals with the Authorization header above.
- If at least one active unarchived goal exists, remind the user to check in or report a violation.
- If no active goals exist, send no reminder.
- Do not mark a goal done from the reminder alone; wait for explicit user confirmation.

## Revocation

The user can revoke this agent link from Goal Stakes settings. After revocation, this URL stops resolving and the API secret returns 401.
`, apiBase, rawAgentSecret)
}

func normalizeTelegramChatKind(kind string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	switch normalized {
	case "private", "group", "supergroup", "channel":
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: invalid telegram chat kind %q", ErrInvalid, kind)
	}
}

func normalizeTimezone(timezone string) (string, error) {
	tz := strings.TrimSpace(timezone)
	if tz == "" || strings.EqualFold(tz, "UTC") {
		return "", nil
	}
	if tz == "Local" {
		return "", fmt.Errorf("%w: invalid timezone %q", ErrInvalid, tz)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return "", fmt.Errorf("%w: invalid timezone %q", ErrInvalid, tz)
	}
	return loc.String(), nil
}
