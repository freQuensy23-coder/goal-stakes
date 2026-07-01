// Package auth contains wallet-session and public API-key authentication.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"goalstakes/internal/domain"
	"goalstakes/internal/store"
)

const (
	apiKeyPrefix    = "sk_"
	apiKeyBytes     = 32
	apiKeyPrefixLen = 12
)

var ErrUnauthorized = errors.New("auth: unauthorized")

// GenerateAPIKey returns the raw key to show once, the non-secret display
// prefix, and the one-way hash that is safe to persist (IV3).
func GenerateAPIKey() (raw, prefix, hash string, err error) {
	random := make([]byte, apiKeyBytes)
	if _, err := rand.Read(random); err != nil {
		return "", "", "", fmt.Errorf("generate api key: %w", err)
	}
	raw = apiKeyPrefix + base64.RawURLEncoding.EncodeToString(random)
	prefix = raw[:apiKeyPrefixLen]
	return raw, prefix, HashAPIKey(raw), nil
}

// HashAPIKey returns the canonical one-way digest for an API key. The raw key
// must never be stored or logged; callers persist only this digest plus prefix.
func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type APIKeyManager struct {
	store store.Store
}

func NewAPIKeyManager(st store.Store) *APIKeyManager {
	return &APIKeyManager{store: st}
}

// Verify resolves a raw Bearer key to its stored hash and rejects revoked keys.
// Errors are intentionally generic so the raw credential never leaks (IV3).
func (m *APIKeyManager) Verify(ctx context.Context, raw string) (domain.ApiKey, error) {
	if m == nil || m.store == nil {
		return domain.ApiKey{}, fmt.Errorf("%w: api key verifier is not configured", ErrUnauthorized)
	}
	if raw == "" || !strings.HasPrefix(raw, apiKeyPrefix) {
		return domain.ApiKey{}, fmt.Errorf("%w: invalid api key", ErrUnauthorized)
	}
	key, err := m.store.GetApiKeyByHash(ctx, HashAPIKey(raw))
	if err != nil {
		return domain.ApiKey{}, fmt.Errorf("%w: invalid api key", ErrUnauthorized)
	}
	if key.Revoked() {
		return domain.ApiKey{}, fmt.Errorf("%w: api key revoked", ErrUnauthorized)
	}
	key, err = m.store.TouchApiKey(ctx, key.ID)
	if err != nil {
		return domain.ApiKey{}, fmt.Errorf("mark api key used: %w", err)
	}
	return key, nil
}
