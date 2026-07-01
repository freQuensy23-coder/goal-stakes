package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"goalstakes/internal/auth"
	"goalstakes/internal/domain"
	"goalstakes/internal/middleware"
)

func TestRequireSessionSetsUserIDContext(t *testing.T) {
	userID := domain.NewID()
	verifier := &fakeSessionVerifier{session: auth.Session{UserID: userID, WalletAddress: "0xabc"}}
	handler := middleware.RequireSession(verifier)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := middleware.UserID(r.Context())
		if !ok || got != userID {
			t.Fatalf("UserID context = %s, %v; want %s, true", got, ok, userID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer session-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if verifier.seen != "session-token" {
		t.Fatalf("verifier saw token %q", verifier.seen)
	}
}

func TestRequireAPIKeyRejectsMissingBearer(t *testing.T) {
	handler := middleware.RequireAPIKey(&fakeAPIKeyVerifier{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run without a bearer token")
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/private", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestRequireAPIKeySetsUserIDContext(t *testing.T) {
	userID := domain.NewID()
	verifier := &fakeAPIKeyVerifier{key: domain.ApiKey{UserID: userID}}
	handler := middleware.RequireAPIKey(verifier)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := middleware.UserID(r.Context())
		if !ok || got != userID {
			t.Fatalf("UserID context = %s, %v; want %s, true", got, ok, userID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer sk_test")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if verifier.seen != "sk_test" {
		t.Fatalf("verifier saw token %q", verifier.seen)
	}
}

type fakeSessionVerifier struct {
	session auth.Session
	seen    string
	err     error
}

func (f *fakeSessionVerifier) VerifySessionToken(_ context.Context, token string) (auth.Session, error) {
	f.seen = token
	return f.session, f.err
}

type fakeAPIKeyVerifier struct {
	key  domain.ApiKey
	seen string
	err  error
}

func (f *fakeAPIKeyVerifier) Verify(_ context.Context, raw string) (domain.ApiKey, error) {
	f.seen = raw
	return f.key, f.err
}
