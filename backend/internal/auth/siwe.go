package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"goalstakes/internal/domain"
	"goalstakes/internal/store"

	"github.com/ethereum/go-ethereum/common"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	siwe "github.com/spruceid/siwe-go"
)

const jwtIssuer = "goalstakes"
const nonceTTL = 10 * time.Minute

type SIWEOption func(*SIWEManager)

func WithClock(now func() time.Time) SIWEOption {
	return func(m *SIWEManager) {
		if now != nil {
			m.now = now
		}
	}
}

type SIWEManager struct {
	store      store.Store
	jwtSecret  []byte
	domain     string
	sessionTTL time.Duration
	now        func() time.Time

	mu     sync.Mutex
	nonces map[string]nonceRecord
}

type nonceRecord struct {
	walletAddress string
	issuedAt      time.Time
}

type Session struct {
	UserID        domain.UUID
	WalletAddress string
}

type sessionClaims struct {
	WalletAddress string `json:"wallet_address"`
	jwt.RegisteredClaims
}

func NewSIWEManager(st store.Store, jwtSecret, domain string, sessionTTL time.Duration, opts ...SIWEOption) (*SIWEManager, error) {
	if st == nil {
		return nil, errors.New("auth: store is required")
	}
	if jwtSecret == "" {
		return nil, errors.New("auth: jwt secret is required")
	}
	if domain == "" {
		return nil, errors.New("auth: siwe domain is required")
	}
	if sessionTTL <= 0 {
		return nil, errors.New("auth: positive session ttl is required")
	}
	m := &SIWEManager{
		store:      st,
		jwtSecret:  []byte(jwtSecret),
		domain:     domain,
		sessionTTL: sessionTTL,
		now:        func() time.Time { return time.Now().UTC() },
		nonces:     make(map[string]nonceRecord),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m, nil
}

// IssueNonce creates a single-use nonce bound to the wallet that requested it.
func (m *SIWEManager) IssueNonce(ctx context.Context, walletAddress string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	wallet, err := normalizeWallet(walletAddress)
	if err != nil {
		return "", err
	}
	nonce := siwe.GenerateNonce()
	m.mu.Lock()
	m.nonces[nonce] = nonceRecord{walletAddress: wallet, issuedAt: m.now().UTC()}
	m.mu.Unlock()
	return nonce, nil
}

// VerifyAndLogin validates an EIP-4361 message/signature pair, consumes the
// matching nonce, creates the wallet-backed user if needed, and returns a JWT.
func (m *SIWEManager) VerifyAndLogin(ctx context.Context, rawMessage, signature string) (domain.User, string, error) {
	msg, err := siwe.ParseMessage(rawMessage)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("%w: parse siwe message", ErrUnauthorized)
	}

	nonce := msg.GetNonce()
	wallet := msg.GetAddress().Hex()
	rec, ok := m.lookupNonce(nonce)
	if !ok {
		return domain.User{}, "", fmt.Errorf("%w: nonce not issued or already used", ErrUnauthorized)
	}
	if rec.walletAddress != wallet {
		return domain.User{}, "", fmt.Errorf("%w: nonce wallet mismatch", ErrUnauthorized)
	}

	now := m.now().UTC()
	if now.Sub(rec.issuedAt) > nonceTTL {
		m.consumeNonce(nonce, wallet)
		return domain.User{}, "", fmt.Errorf("%w: nonce expired", ErrUnauthorized)
	}
	if _, err := msg.Verify(signature, &m.domain, &nonce, &now); err != nil {
		return domain.User{}, "", fmt.Errorf("%w: invalid siwe signature", ErrUnauthorized)
	}
	if !m.consumeNonce(nonce, wallet) {
		return domain.User{}, "", fmt.Errorf("%w: nonce not issued or already used", ErrUnauthorized)
	}

	user, err := m.store.GetUserByWallet(ctx, wallet)
	if errors.Is(err, store.ErrNotFound) {
		user, err = m.store.CreateUser(ctx, wallet, "")
	}
	if err != nil {
		return domain.User{}, "", fmt.Errorf("siwe login user lookup: %w", err)
	}
	token, err := m.signSession(user)
	if err != nil {
		return domain.User{}, "", err
	}
	return user, token, nil
}

func (m *SIWEManager) VerifySessionToken(_ context.Context, raw string) (Session, error) {
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		return m.jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(jwtIssuer), jwt.WithExpirationRequired(), jwt.WithTimeFunc(m.now))
	if err != nil || !token.Valid {
		return Session{}, fmt.Errorf("%w: invalid session", ErrUnauthorized)
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return Session{}, fmt.Errorf("%w: invalid session subject", ErrUnauthorized)
	}
	return Session{UserID: userID, WalletAddress: claims.WalletAddress}, nil
}

func (m *SIWEManager) signSession(user domain.User) (string, error) {
	now := m.now().UTC()
	claims := sessionClaims{
		WalletAddress: user.WalletAddress,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    jwtIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.sessionTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString(m.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("sign session jwt: %w", err)
	}
	return raw, nil
}

func (m *SIWEManager) lookupNonce(nonce string) (nonceRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.nonces[nonce]
	return rec, ok
}

func (m *SIWEManager) consumeNonce(nonce, wallet string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.nonces[nonce]
	if !ok || rec.walletAddress != wallet {
		return false
	}
	delete(m.nonces, nonce)
	return true
}

func normalizeWallet(address string) (string, error) {
	if !common.IsHexAddress(address) {
		return "", fmt.Errorf("%w: invalid wallet address", ErrUnauthorized)
	}
	return common.HexToAddress(address).Hex(), nil
}
