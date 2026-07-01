package auth_test

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"testing"
	"time"

	"goalstakes/internal/auth"
	"goalstakes/internal/store"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	siwe "github.com/spruceid/siwe-go"
)

func TestSIWEManagerVerifiesSignatureCreatesSessionAndConsumesNonce(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	mgr, err := auth.NewSIWEManager(st, "test-secret", "example.com", time.Hour, auth.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewSIWEManager: %v", err)
	}

	key, wallet := createWallet(t)
	nonce, err := mgr.IssueNonce(ctx, wallet)
	if err != nil {
		t.Fatalf("IssueNonce: %v", err)
	}
	message := signedMessage(t, key, wallet, nonce, now)

	user, token, err := mgr.VerifyAndLogin(ctx, message.text, message.signature)
	if err != nil {
		t.Fatalf("VerifyAndLogin: %v", err)
	}
	if user.WalletAddress != wallet {
		t.Fatalf("WalletAddress = %q, want %q", user.WalletAddress, wallet)
	}
	session, err := mgr.VerifySessionToken(ctx, token)
	if err != nil {
		t.Fatalf("VerifySessionToken: %v", err)
	}
	if session.UserID != user.ID || session.WalletAddress != wallet {
		t.Fatalf("session mismatch: %+v, user %+v", session, user)
	}

	if _, _, err := mgr.VerifyAndLogin(ctx, message.text, message.signature); err == nil {
		t.Fatal("VerifyAndLogin must reject replay with a consumed nonce")
	}
}

func TestSIWEManagerRejectsNonceIssuedForAnotherWallet(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	mgr, err := auth.NewSIWEManager(st, "test-secret", "example.com", time.Hour, auth.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewSIWEManager: %v", err)
	}

	_, issuedFor := createWallet(t)
	key, signer := createWallet(t)
	nonce, err := mgr.IssueNonce(ctx, issuedFor)
	if err != nil {
		t.Fatalf("IssueNonce: %v", err)
	}
	message := signedMessage(t, key, signer, nonce, now)

	if _, _, err := mgr.VerifyAndLogin(ctx, message.text, message.signature); err == nil {
		t.Fatal("VerifyAndLogin must reject a nonce bound to another wallet")
	}
}

func TestSIWEManagerRejectsExpiredNonce(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	mgr, err := auth.NewSIWEManager(st, "test-secret", "example.com", time.Hour, auth.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewSIWEManager: %v", err)
	}

	key, wallet := createWallet(t)
	nonce, err := mgr.IssueNonce(ctx, wallet)
	if err != nil {
		t.Fatalf("IssueNonce: %v", err)
	}
	message := signedMessage(t, key, wallet, nonce, now)

	now = now.Add(11 * time.Minute)
	if _, _, err := mgr.VerifyAndLogin(ctx, message.text, message.signature); err == nil {
		t.Fatal("VerifyAndLogin must reject expired nonces")
	}
}

type siweSignedMessage struct {
	text      string
	signature string
}

func createWallet(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	return privateKey, address
}

func signedMessage(t *testing.T, key *ecdsa.PrivateKey, wallet, nonce string, now time.Time) siweSignedMessage {
	t.Helper()
	msg, err := siwe.InitMessage("example.com", wallet, "https://example.com", nonce, map[string]interface{}{
		"issuedAt":       now.Format(time.RFC3339),
		"expirationTime": now.Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("InitMessage: %v", err)
	}
	text := msg.String()
	hash := crypto.Keccak256Hash([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len([]byte(text)), text)))
	signature, err := crypto.Sign(hash.Bytes(), key)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	signature[64] += 27
	return siweSignedMessage{text: text, signature: hexutil.Encode(signature)}
}
