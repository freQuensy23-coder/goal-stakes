// Package middleware contains HTTP middleware shared by the app and public API.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"goalstakes/internal/auth"
	"goalstakes/internal/domain"
)

type SessionVerifier interface {
	VerifySessionToken(ctx context.Context, token string) (auth.Session, error)
}

type APIKeyVerifier interface {
	Verify(ctx context.Context, raw string) (domain.ApiKey, error)
}

type contextKey string

const userIDKey contextKey = "goalstakes_user_id"

func UserID(ctx context.Context) (domain.UUID, bool) {
	userID, ok := ctx.Value(userIDKey).(domain.UUID)
	return userID, ok
}

func WithUserID(ctx context.Context, userID domain.UUID) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func RequireSession(verifier SessionVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r)
			if !ok {
				unauthorized(w)
				return
			}
			session, err := verifier.VerifySessionToken(r.Context(), raw)
			if err != nil {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), session.UserID)))
		})
	}
}

func RequireAPIKey(verifier APIKeyVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r)
			if !ok {
				unauthorized(w)
				return
			}
			key, err := verifier.Verify(r.Context(), raw)
			if err != nil {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), key.UserID)))
		})
	}
}

// RequireApiKey keeps the spelling used in the plan while the codebase can use
// the Go initialism form RequireAPIKey.
func RequireApiKey(verifier APIKeyVerifier) func(http.Handler) http.Handler {
	return RequireAPIKey(verifier)
}

func bearerToken(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return "", false
	}
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func unauthorized(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}
