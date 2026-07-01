package auth_test

import (
	"context"
	"strings"
	"testing"

	"goalstakes/internal/auth"
	"goalstakes/internal/store"
)

func TestGenerateAPIKeyReturnsRawPrefixAndHash(t *testing.T) {
	raw1, prefix1, hash1, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	raw2, _, hash2, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey second: %v", err)
	}

	if !strings.HasPrefix(raw1, "sk_") {
		t.Fatalf("raw key prefix = %q, want sk_", raw1)
	}
	if raw1 == raw2 || hash1 == hash2 {
		t.Fatal("GenerateAPIKey must produce unique raw keys and hashes")
	}
	if prefix1 != raw1[:12] {
		t.Fatalf("prefix = %q, want first 12 chars of raw key", prefix1)
	}
	if strings.Contains(hash1, raw1) || len(hash1) != 64 {
		t.Fatalf("hash should be a 64-char digest and must not contain the raw key: %q", hash1)
	}
}

func TestAPIKeyManagerVerifiesByHashAndRejectsRevokedKeys(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "UTC")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	raw, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	stored, err := st.CreateApiKey(ctx, store.CreateApiKeyInput{
		UserID:  user.ID,
		Name:    "integration",
		Prefix:  prefix,
		KeyHash: hash,
	})
	if err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}

	mgr := auth.NewAPIKeyManager(st)
	got, err := mgr.Verify(ctx, raw)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.ID != stored.ID || got.KeyHash != hash {
		t.Fatalf("Verify returned wrong key: %+v", got)
	}

	if err := st.RevokeApiKey(ctx, stored.ID); err != nil {
		t.Fatalf("RevokeApiKey: %v", err)
	}
	if _, err := mgr.Verify(ctx, raw); err == nil {
		t.Fatal("Verify must reject revoked keys")
	} else if strings.Contains(err.Error(), raw) {
		t.Fatalf("Verify error leaked raw API key: %v", err)
	}
}

func TestAPIKeyManagerMarksValidKeyLastUsed(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "UTC")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	raw, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	stored, err := st.CreateApiKey(ctx, store.CreateApiKeyInput{
		UserID:  user.ID,
		Name:    "automation",
		Prefix:  prefix,
		KeyHash: hash,
	})
	if err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}

	got, err := auth.NewAPIKeyManager(st).Verify(ctx, raw)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.ID != stored.ID {
		t.Fatalf("Verify returned id=%s want %s", got.ID, stored.ID)
	}
	if got.LastUsed == nil {
		t.Fatal("Verify must return the updated last-used timestamp")
	}
	persisted, err := st.GetApiKeyByHash(ctx, hash)
	if err != nil {
		t.Fatalf("GetApiKeyByHash: %v", err)
	}
	if persisted.LastUsed == nil {
		t.Fatal("Verify must persist the last-used timestamp")
	}
}
