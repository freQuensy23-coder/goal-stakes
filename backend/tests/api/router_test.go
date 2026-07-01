package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"goalstakes/internal/ai"
	"goalstakes/internal/api"
	"goalstakes/internal/auth"
	"goalstakes/internal/config"
	"goalstakes/internal/domain"
	"goalstakes/internal/service"
	"goalstakes/internal/store"

	openai "github.com/sashabaranov/go-openai"
)

func TestRouterRejectsUnauthenticatedGoalRequest(t *testing.T) {
	router, _ := newTestRouter(t)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	assertJSONError(t, rr, "Unauthorized")
}

func TestRouterRejectsMalformedRequestsWithJSONErrors(t *testing.T) {
	router, rawKey := newTestRouter(t)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodPost, "/api/v1/goals", strings.NewReader(`{"title":`), rawKey))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid json status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "invalid json")

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodGet, "/api/v1/goals/not-a-uuid/progress", nil, rawKey))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid path status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "invalid goalID")
}

func TestRouterFallbackErrorsUseJSON(t *testing.T) {
	router, rawKey := newTestRouter(t)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodGet, "/api/v1/unknown", nil, rawKey))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "Not Found")

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodPut, "/api/v1/goals", strings.NewReader(`{}`), rawKey))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "Method Not Allowed")
}

func TestRouterSIWEUnavailableReturnsStructuredErrors(t *testing.T) {
	router := api.NewRouter(api.Config{})

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/auth/nonce", strings.NewReader(`{"wallet_address":"0xabc"}`)))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("nonce status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "siwe auth is not configured")

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/auth/siwe", strings.NewReader(`{"message":"m","signature":"s"}`)))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("siwe status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "siwe auth is not configured")
}

func TestRouterResourceErrorsUseDocumentedJSONResponses(t *testing.T) {
	router, rawKey := newTestRouter(t)
	missingGoalID := domain.NewID()

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodGet, "/api/v1/goals/"+missingGoalID.String()+"/progress", nil, rawKey))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing goal status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "get goal "+missingGoalID.String()+": store: not found")

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodDelete, "/api/v1/apikeys/"+domain.NewID().String(), nil, rawKey))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("foreign api key status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "service: forbidden: api key does not belong to user")
}

func TestRouterRecordApprovalRequiresTxHash(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodPost, "/api/v1/approvals", strings.NewReader(`{"chain":"sepolia","token_symbol":"USDC","dry_run_allowance":"1000000"}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("record approval status = %d body=%s, want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	assertJSONError(t, rr, "service: invalid input: tx_hash is required")
}

func TestRouterAllowsFrontendCORSPreflight(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/nonce", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type,authorization")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "authorization") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want authorization", got)
	}
}

func TestRouterListGoalsReturnsEmptyArray(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodGet, "/api/v1/goals", nil, rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Fatalf("body = %s, want []", rr.Body.String())
	}
}

func TestRouterListChainsIsPublic(t *testing.T) {
	router, _ := newTestRouter(t)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/chains", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var chains []service.ChainInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &chains); err != nil {
		t.Fatalf("decode chains: %v", err)
	}
	if len(chains) != 1 || chains[0].Key != "sepolia" {
		t.Fatalf("chains = %+v", chains)
	}
	if chains[0].StakeEnforcerAddress != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("enforcer = %q", chains[0].StakeEnforcerAddress)
	}
	if chains[0].Tokens["USDC"] != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("tokens = %+v", chains[0].Tokens)
	}
	if strings.Contains(rr.Body.String(), "rpc_url") {
		t.Fatalf("public chain config must not expose RPC URLs: %s", rr.Body.String())
	}
}

func TestRouterCreateGoalAndLogCheckInWithAPIKey(t *testing.T) {
	router, rawKey := newTestRouter(t)

	goalPayload := `{
		"title":"Do push-ups",
		"type":"do",
		"cadence":"daily",
		"stake_amount":"1000000",
		"token_symbol":"USDC",
		"chain":"sepolia",
		"starts_at":"2026-05-24T06:00:00Z",
		"ends_at":"2026-06-01T00:00:00Z"
	}`
	req := authedRequest(http.MethodPost, "/api/v1/goals", strings.NewReader(goalPayload), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create goal status = %d body=%s", rr.Code, rr.Body.String())
	}
	var goal domain.Goal
	if err := json.Unmarshal(rr.Body.Bytes(), &goal); err != nil {
		t.Fatalf("decode goal: %v", err)
	}
	if goal.ID == (domain.UUID{}) || goal.Title != "Do push-ups" {
		t.Fatalf("unexpected goal: %+v", goal)
	}
	wantStart := time.Date(2026, 5, 24, 6, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if !goal.StartsAt.Equal(wantStart) || goal.EndsAt == nil || !goal.EndsAt.Equal(wantEnd) {
		t.Fatalf("goal schedule = starts_at %v ends_at %v, want %v / %v", goal.StartsAt, goal.EndsAt, wantStart, wantEnd)
	}

	checkInPayload := `{"note":"done"}`
	req = authedRequest(http.MethodPost, "/api/v1/goals/"+goal.ID.String()+"/checkins", strings.NewReader(checkInPayload), rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("check-in status = %d body=%s", rr.Code, rr.Body.String())
	}
	var checkIn domain.CheckIn
	if err := json.Unmarshal(rr.Body.Bytes(), &checkIn); err != nil {
		t.Fatalf("decode check-in: %v", err)
	}
	if checkIn.Period != domain.Period("2026-05-25") {
		t.Fatalf("check-in period = %q, want 2026-05-25", checkIn.Period)
	}
}

func TestRouterUpdateGoalPreservesStakeWhenStakeIsOmitted(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodPost, "/api/v1/goals", strings.NewReader(`{
		"title":"Do push-ups",
		"type":"do",
		"cadence":"daily",
		"stake_amount":"1000000",
		"token_symbol":"USDC",
		"chain":"sepolia",
		"ends_at":"2026-06-01T00:00:00Z"
	}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create goal status = %d body=%s", rr.Code, rr.Body.String())
	}
	var goal domain.Goal
	if err := json.Unmarshal(rr.Body.Bytes(), &goal); err != nil {
		t.Fatalf("decode goal: %v", err)
	}

	req = authedRequest(http.MethodPatch, "/api/v1/goals/"+goal.ID.String(), strings.NewReader(`{
		"title":"Do 120 push-ups",
		"description":"harder set"
	}`), rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch goal status = %d body=%s", rr.Code, rr.Body.String())
	}
	var updated domain.Goal
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated goal: %v", err)
	}
	if updated.Title != "Do 120 push-ups" || updated.Description != "harder set" {
		t.Fatalf("updated goal fields = %+v", updated)
	}
	if updated.StakeAmount != "1000000" {
		t.Fatalf("stake amount = %q, want preserved original stake", updated.StakeAmount)
	}
	wantEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if updated.EndsAt == nil || !updated.EndsAt.Equal(wantEnd) {
		t.Fatalf("ends_at = %v, want preserved %v", updated.EndsAt, wantEnd)
	}
}

func TestRouterUpdateGoalClearsEndDateWhenExplicitNull(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodPost, "/api/v1/goals", strings.NewReader(`{
		"title":"Avoid soda",
		"type":"avoid",
		"cadence":"daily",
		"stake_amount":"1000000",
		"token_symbol":"USDC",
		"chain":"sepolia",
		"ends_at":"2026-06-01T00:00:00Z"
	}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create goal status = %d body=%s", rr.Code, rr.Body.String())
	}
	var goal domain.Goal
	if err := json.Unmarshal(rr.Body.Bytes(), &goal); err != nil {
		t.Fatalf("decode goal: %v", err)
	}
	if goal.EndsAt == nil {
		t.Fatalf("test setup goal should have an end date")
	}

	req = authedRequest(http.MethodPatch, "/api/v1/goals/"+goal.ID.String(), strings.NewReader(`{
		"title":"Avoid soda",
		"ends_at":null
	}`), rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch goal status = %d body=%s", rr.Code, rr.Body.String())
	}
	var updated domain.Goal
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated goal: %v", err)
	}
	if updated.EndsAt != nil {
		t.Fatalf("ends_at = %v, want cleared nil", updated.EndsAt)
	}
}

func TestRouterReportViolationReturnsStructuredChargeFailure(t *testing.T) {
	router, rawKey := newTestRouterWithServiceOptions(t, service.WithPenaltyCharger(failingPenaltyCharger{}))

	req := authedRequest(http.MethodPost, "/api/v1/goals", strings.NewReader(`{
		"title":"Avoid soda",
		"type":"avoid",
		"cadence":"daily",
		"stake_amount":"1000000",
		"token_symbol":"USDC",
		"chain":"sepolia"
	}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create goal status = %d body=%s", rr.Code, rr.Body.String())
	}
	var goal domain.Goal
	if err := json.Unmarshal(rr.Body.Bytes(), &goal); err != nil {
		t.Fatalf("decode goal: %v", err)
	}

	req = authedRequest(http.MethodPost, "/api/v1/goals/"+goal.ID.String()+"/violations", strings.NewReader(`{"reason":"drank soda"}`), rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("violation status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !strings.Contains(body["error"], "service: charge failed") || !strings.Contains(body["error"], "penalize tx reverted") {
		t.Fatalf("error body = %+v, want structured charge failure", body)
	}

	req = authedRequest(http.MethodGet, "/api/v1/goals/"+goal.ID.String()+"/progress", nil, rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("progress status = %d body=%s", rr.Code, rr.Body.String())
	}
	var progress service.Progress
	if err := json.Unmarshal(rr.Body.Bytes(), &progress); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	if len(progress.Violations) != 1 || progress.Violations[0].Status != domain.ViolationFailed || progress.Violations[0].TxHash != "0xreverted" {
		t.Fatalf("failed violation was not preserved in progress: %+v", progress.Violations)
	}
}

func TestRouterKnownErrorsUseStructuredMessages(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodPost, "/api/v1/goals", strings.NewReader(`{
		"title":"Too expensive",
		"type":"do",
		"cadence":"daily",
		"stake_amount":"2000000",
		"token_symbol":"USDC",
		"chain":"sepolia"
	}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(body["error"], "approval allowance") || !strings.Contains(body["error"], "below stake amount") {
		t.Fatalf("error body = %+v, want actionable allowance message", body)
	}
}

func TestRouterArchiveGoalRemovesItFromList(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodPost, "/api/v1/goals", strings.NewReader(`{
		"title":"Archive me",
		"type":"do",
		"cadence":"daily",
		"stake_amount":"1000000",
		"token_symbol":"USDC",
		"chain":"sepolia"
	}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rr.Code, rr.Body.String())
	}
	var goal domain.Goal
	if err := json.Unmarshal(rr.Body.Bytes(), &goal); err != nil {
		t.Fatalf("decode goal: %v", err)
	}

	req = authedRequest(http.MethodDelete, "/api/v1/goals/"+goal.ID.String(), nil, rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = authedRequest(http.MethodGet, "/api/v1/goals", nil, rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), goal.ID.String()) {
		t.Fatalf("archived goal leaked from list: %s", rr.Body.String())
	}
}

func TestRouterAPIKeysAndOpenAPIDocs(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewBufferString(`{"name":"automation"}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create api key status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created service.CreatedAPIKey
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created api key: %v", err)
	}
	if !strings.HasPrefix(created.Raw, "sk_") || created.Key.KeyHash != "" {
		t.Fatalf("response must show raw once and never expose stored hash: %+v", created)
	}
	if created.Key.LastUsed != nil {
		t.Fatalf("new API key should start unused: %+v", created.Key)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodGet, "/api/v1/goals", nil, created.Raw))
	if rr.Code != http.StatusOK {
		t.Fatalf("goals with created api key status = %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodGet, "/api/v1/apikeys", nil, rawKey))
	if rr.Code != http.StatusOK {
		t.Fatalf("list api keys status = %d body=%s", rr.Code, rr.Body.String())
	}
	var keys []domain.ApiKey
	if err := json.Unmarshal(rr.Body.Bytes(), &keys); err != nil {
		t.Fatalf("decode api keys: %v", err)
	}
	var used *domain.ApiKey
	for i := range keys {
		if keys[i].ID == created.Key.ID {
			used = &keys[i]
			break
		}
	}
	if used == nil || used.LastUsed == nil {
		t.Fatalf("used API key missing last_used in list response: %+v", keys)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodDelete, "/api/v1/apikeys/"+created.Key.ID.String(), nil, rawKey))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("revoke api key status = %d body=%s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodGet, "/api/v1/apikeys", nil, rawKey))
	if rr.Code != http.StatusOK {
		t.Fatalf("list after revoke status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), created.Key.ID.String()) {
		t.Fatalf("revoked api key leaked from active list: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("openapi status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"openapi":"3.0.3"`) || !strings.Contains(rr.Body.String(), `"/api/v1/goals"`) {
		t.Fatalf("openapi response missing expected fields: %s", rr.Body.String())
	}
	var spec map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}
	paths := spec["paths"].(map[string]any)
	for path, methods := range map[string][]string{
		"/api/v1/chains":                    {"get"},
		"/api/v1/auth/nonce":                {"post"},
		"/api/v1/auth/siwe":                 {"post"},
		"/api/v1/me":                        {"get"},
		"/api/v1/goals":                     {"get", "post"},
		"/api/v1/goals/{goalID}":            {"patch", "delete"},
		"/api/v1/goals/{goalID}/stake":      {"patch"},
		"/api/v1/goals/{goalID}/progress":   {"get"},
		"/api/v1/goals/{goalID}/checkins":   {"post"},
		"/api/v1/goals/{goalID}/violations": {"get", "post"},
		"/api/v1/approvals":                 {"get", "post"},
		"/api/v1/apikeys":                   {"get", "post"},
		"/api/v1/apikeys/{apiKeyID}":        {"delete"},
		"/api/v1/chat":                      {"post"},
		"/api/v1/chat/audio":                {"post"},
		"/api/v1/telegram/link-codes":       {"post"},
		"/api/v1/agent-links":               {"get", "post"},
		"/api/v1/agent-links/{agentLinkID}": {"delete"},
		"/agent-skills/{token}.md":          {"get"},
		"/internal/telegram/link":           {"post"},
		"/internal/telegram/message":        {"post"},
		"/internal/telegram/audio":          {"post"},
		"/internal/telegram/agent-link":     {"post"},
	} {
		pathItem, ok := paths[path].(map[string]any)
		if !ok {
			t.Fatalf("openapi paths missing %s: %+v", path, paths)
		}
		for _, method := range methods {
			if _, ok := pathItem[method]; !ok {
				t.Fatalf("openapi path %s missing %s operation: %+v", path, method, pathItem)
			}
		}
	}
	authNonce := paths["/api/v1/auth/nonce"].(map[string]any)["post"].(map[string]any)
	authNonceResponses := authNonce["responses"].(map[string]any)
	if _, ok := authNonceResponses["503"]; !ok {
		t.Fatalf("auth nonce operation missing 503 SIWE-disabled response: %+v", authNonceResponses)
	}
	authSIWE := paths["/api/v1/auth/siwe"].(map[string]any)["post"].(map[string]any)
	authSIWEResponses := authSIWE["responses"].(map[string]any)
	if _, ok := authSIWEResponses["503"]; !ok {
		t.Fatalf("auth siwe operation missing 503 SIWE-disabled response: %+v", authSIWEResponses)
	}
	goalsPath := paths["/api/v1/goals"].(map[string]any)
	createGoal := goalsPath["post"].(map[string]any)
	if _, ok := createGoal["requestBody"]; !ok {
		t.Fatalf("create goal operation missing requestBody: %+v", createGoal)
	}
	responses := createGoal["responses"].(map[string]any)
	if _, ok := responses["201"]; !ok {
		t.Fatalf("create goal operation missing 201 response: %+v", responses)
	}
	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	if _, ok := schemas["CreateGoalRequest"]; !ok {
		t.Fatalf("openapi schemas missing CreateGoalRequest: %+v", schemas)
	}
	if _, ok := schemas["ErrorResponse"]; !ok {
		t.Fatalf("openapi schemas missing ErrorResponse: %+v", schemas)
	}
	recordApprovalSchema := schemas["RecordApprovalRequest"].(map[string]any)
	recordApprovalRequired := recordApprovalSchema["required"].([]any)
	for _, field := range []string{"chain", "token_symbol", "tx_hash"} {
		found := false
		for _, required := range recordApprovalRequired {
			found = found || required == field
		}
		if !found {
			t.Fatalf("RecordApprovalRequest required = %+v, want %s", recordApprovalRequired, field)
		}
	}
	for _, required := range recordApprovalRequired {
		if required == "allowance" || required == "dry_run_allowance" {
			t.Fatalf("RecordApprovalRequest required = %+v, must not require client allowance", recordApprovalRequired)
		}
	}
	recordApprovalProps := recordApprovalSchema["properties"].(map[string]any)
	if _, ok := recordApprovalProps["tx_hash"]; !ok {
		t.Fatalf("RecordApprovalRequest missing tx_hash property: %+v", recordApprovalProps)
	}
	if _, ok := recordApprovalProps["dry_run_allowance"]; !ok {
		t.Fatalf("RecordApprovalRequest missing local dry_run_allowance property: %+v", recordApprovalProps)
	}
	if _, ok := recordApprovalProps["allowance"]; ok {
		t.Fatalf("RecordApprovalRequest must not expose legacy allowance property: %+v", recordApprovalProps)
	}
	updateGoalSchema := schemas["UpdateGoalRequest"].(map[string]any)
	updateGoalRequired := updateGoalSchema["required"].([]any)
	var requiresTitle, requiresStakeAmount bool
	for _, field := range updateGoalRequired {
		requiresTitle = requiresTitle || field == "title"
		requiresStakeAmount = requiresStakeAmount || field == "stake_amount"
	}
	if !requiresTitle {
		t.Fatalf("UpdateGoalRequest required = %+v, want title", updateGoalRequired)
	}
	if requiresStakeAmount {
		t.Fatalf("UpdateGoalRequest required = %+v, stake_amount must be optional", updateGoalRequired)
	}
	goalIDPath := paths["/api/v1/goals/{goalID}"].(map[string]any)
	updateGoalOperation := goalIDPath["patch"].(map[string]any)
	updateGoalResponses := updateGoalOperation["responses"].(map[string]any)
	if _, ok := updateGoalResponses["403"]; !ok {
		t.Fatalf("update goal operation missing 403 forbidden response: %+v", updateGoalResponses)
	}
	if _, ok := updateGoalResponses["404"]; !ok {
		t.Fatalf("update goal operation missing 404 not-found response: %+v", updateGoalResponses)
	}
	revokeAPIKeyOperation := paths["/api/v1/apikeys/{apiKeyID}"].(map[string]any)["delete"].(map[string]any)
	revokeAPIKeyResponses := revokeAPIKeyOperation["responses"].(map[string]any)
	if _, ok := revokeAPIKeyResponses["403"]; !ok {
		t.Fatalf("revoke api key operation missing 403 forbidden response: %+v", revokeAPIKeyResponses)
	}
	badRequest := createGoal["responses"].(map[string]any)["400"].(map[string]any)
	badRequestContent := badRequest["content"].(map[string]any)
	badRequestJSON := badRequestContent["application/json"].(map[string]any)
	if ref := badRequestJSON["schema"].(map[string]any)["$ref"]; ref != "#/components/schemas/ErrorResponse" {
		t.Fatalf("400 response schema = %v, want ErrorResponse", ref)
	}
	violationsPath := paths["/api/v1/goals/{goalID}/violations"].(map[string]any)
	reportViolation := violationsPath["post"].(map[string]any)
	reportViolationResponses := reportViolation["responses"].(map[string]any)
	if _, ok := reportViolationResponses["502"]; !ok {
		t.Fatalf("report violation operation missing 502 charge-failure response: %+v", reportViolationResponses)
	}
	chatPath := paths["/api/v1/chat"].(map[string]any)
	chatOperation := chatPath["post"].(map[string]any)
	chatResponses := chatOperation["responses"].(map[string]any)
	if _, ok := chatResponses["503"]; !ok {
		t.Fatalf("chat operation missing 503 AI-disabled response: %+v", chatResponses)
	}
	chatAudioPath := paths["/api/v1/chat/audio"].(map[string]any)
	chatAudioOperation := chatAudioPath["post"].(map[string]any)
	chatAudioBody := chatAudioOperation["requestBody"].(map[string]any)
	chatAudioContent := chatAudioBody["content"].(map[string]any)
	if _, ok := chatAudioContent["multipart/form-data"]; !ok {
		t.Fatalf("chat audio operation missing multipart request body: %+v", chatAudioContent)
	}
	chatAudioResponses := chatAudioOperation["responses"].(map[string]any)
	chatAudioOK := chatAudioResponses["200"].(map[string]any)
	chatAudioOKContent := chatAudioOK["content"].(map[string]any)["application/json"].(map[string]any)
	if ref := chatAudioOKContent["schema"].(map[string]any)["$ref"]; ref != "#/components/schemas/AudioChatResponse" {
		t.Fatalf("chat audio response schema = %v, want AudioChatResponse", ref)
	}
	if _, ok := paths["/api/v1/telegram/link-codes"]; !ok {
		t.Fatalf("openapi paths missing /api/v1/telegram/link-codes: %+v", paths)
	}
	if _, ok := paths["/api/v1/agent-links"]; !ok {
		t.Fatalf("openapi paths missing /api/v1/agent-links: %+v", paths)
	}
	if _, ok := paths["/api/v1/agent-links/{agentLinkID}"]; !ok {
		t.Fatalf("openapi paths missing /api/v1/agent-links/{agentLinkID}: %+v", paths)
	}
	if _, ok := paths["/agent-skills/{token}.md"]; !ok {
		t.Fatalf("openapi paths missing /agent-skills/{token}.md: %+v", paths)
	}
	if _, ok := paths["/internal/telegram/link"]; !ok {
		t.Fatalf("openapi paths missing /internal/telegram/link: %+v", paths)
	}
	if _, ok := paths["/internal/telegram/message"]; !ok {
		t.Fatalf("openapi paths missing /internal/telegram/message: %+v", paths)
	}
	if _, ok := paths["/internal/telegram/audio"]; !ok {
		t.Fatalf("openapi paths missing /internal/telegram/audio: %+v", paths)
	}
	if _, ok := paths["/internal/telegram/agent-link"]; !ok {
		t.Fatalf("openapi paths missing /internal/telegram/agent-link: %+v", paths)
	}
	createGoalSchema := schemas["CreateGoalRequest"].(map[string]any)
	createGoalProps := createGoalSchema["properties"].(map[string]any)
	cadence := createGoalProps["cadence"].(map[string]any)
	cadenceEnum := cadence["enum"].([]any)
	if len(cadenceEnum) != 2 || cadenceEnum[0] != "daily" || cadenceEnum[1] != "weekly" {
		t.Fatalf("CreateGoalRequest cadence enum = %+v, want daily/weekly only", cadenceEnum)
	}
	stakeAmount := createGoalProps["stake_amount"].(map[string]any)
	description, _ := stakeAmount["description"].(string)
	if !strings.Contains(description, "smallest token units") || !strings.Contains(description, "$100") || !strings.Contains(description, "100000000") {
		t.Fatalf("stake_amount schema description should explain token units, got %q", description)
	}
	if _, ok := createGoalProps["starts_at"]; !ok {
		t.Fatalf("CreateGoalRequest missing optional starts_at property: %+v", createGoalProps)
	}
	if _, ok := createGoalProps["ends_at"]; !ok {
		t.Fatalf("CreateGoalRequest missing optional ends_at property: %+v", createGoalProps)
	}
	if _, ok := schemas["ChainInfo"]; !ok {
		t.Fatalf("openapi schemas missing ChainInfo: %+v", schemas)
	}
	chatRequest := schemas["ChatRequest"].(map[string]any)
	chatProps := chatRequest["properties"].(map[string]any)
	if _, ok := chatProps["conversation_id"]; !ok {
		t.Fatalf("ChatRequest missing conversation_id property: %+v", chatProps)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("docs status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Authorization: Bearer") || !strings.Contains(rr.Body.String(), "POST /api/v1/chat") {
		t.Fatalf("docs response missing expected public API usage text: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "POST /api/v1/chat/audio") {
		t.Fatalf("docs response missing audio chat endpoint: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "POST /api/v1/telegram/link-codes") || !strings.Contains(rr.Body.String(), "POST /internal/telegram/link") {
		t.Fatalf("docs response missing telegram link endpoints: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "POST /api/v1/agent-links") || !strings.Contains(rr.Body.String(), "GET /agent-skills/{token}.md") || !strings.Contains(rr.Body.String(), "daily cron") {
		t.Fatalf("docs response missing own-agent endpoints: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "POST /internal/telegram/message") || !strings.Contains(rr.Body.String(), "/create") {
		t.Fatalf("docs response missing telegram message endpoint and commands: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "POST /internal/telegram/audio") || !strings.Contains(rr.Body.String(), "voice/audio") {
		t.Fatalf("docs response missing telegram audio endpoint: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "POST /internal/telegram/agent-link") {
		t.Fatalf("docs response missing telegram agent-link endpoint: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "approval allowance") {
		t.Fatalf("docs response should explain allowance requirement: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "omit <code>stake_amount</code>") {
		t.Fatalf("docs response should explain optional stake_amount on goal updates: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "starts_at") || !strings.Contains(rr.Body.String(), "ends_at") {
		t.Fatalf("docs response should explain optional goal schedule fields: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "send <code>ends_at</code> as <code>null</code>") {
		t.Fatalf("docs response should explain clearing goal end dates: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "502") || !strings.Contains(rr.Body.String(), "charge failed") {
		t.Fatalf("docs response should explain charge-failure responses: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "503") || !strings.Contains(rr.Body.String(), "OPENAI_API_KEY") {
		t.Fatalf("docs response should explain AI-disabled responses: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "SIWE auth") {
		t.Fatalf("docs response should explain SIWE-disabled responses: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "404") || !strings.Contains(rr.Body.String(), "405") {
		t.Fatalf("docs response should explain fallback error responses: %s", rr.Body.String())
	}
}

func TestRouterAgentLinkLifecycleGeneratesPrivateSkillAndRevokesSecret(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodPost, "/api/v1/agent-links", strings.NewReader(`{"name":"codex"}`), rawKey)
	req.Host = "api.goalstakes.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create agent link status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Authorization: Bearer sk_") {
		t.Fatalf("create response leaked generated secret: %s", rr.Body.String())
	}
	var created service.CreatedAgentLink
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created agent link: %v", err)
	}
	if created.AgentLink.ID == (domain.UUID{}) || created.AgentLink.APIKeyID == (domain.UUID{}) || !strings.HasPrefix(created.SkillURL, "https://api.goalstakes.test/agent-skills/agt_") || !strings.HasSuffix(created.SkillURL, ".md") {
		t.Fatalf("created agent link = %+v", created)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodGet, "/api/v1/agent-links", nil, rawKey))
	if rr.Code != http.StatusOK {
		t.Fatalf("list agent links status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "sk_") || strings.Contains(rr.Body.String(), "agt_") || strings.Contains(rr.Body.String(), "token_hash") {
		t.Fatalf("list agent links leaked secret material: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, created.SkillURL, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("fetch skill status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/markdown") {
		t.Fatalf("Content-Type = %q, want text/markdown", got)
	}
	markdown := rr.Body.String()
	for _, expected := range []string{
		"Goal Stakes lets the user create do and avoid goals",
		"API base: https://api.goalstakes.test",
		"Authorization: Bearer sk_",
		"GET /api/v1/goals",
		"POST /api/v1/chat/audio",
		"Run once per day in the user's timezone.",
		"If at least one active unarchived goal exists",
		"remind the user to check in or report a violation",
		"Never ask for wallet seed phrases",
		"Do not mark a goal done from the reminder alone",
	} {
		if !strings.Contains(markdown, expected) {
			t.Fatalf("skill markdown missing %q in:\n%s", expected, markdown)
		}
	}
	agentSecret := extractAgentSecret(t, markdown)
	if !strings.HasPrefix(agentSecret, "sk_") {
		t.Fatalf("agent secret = %q, want sk_", agentSecret)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodGet, "/api/v1/goals", nil, agentSecret))
	if rr.Code != http.StatusOK {
		t.Fatalf("agent secret goals status = %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodDelete, "/api/v1/agent-links/"+created.AgentLink.ID.String(), nil, rawKey))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("revoke agent link status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, created.SkillURL, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("revoked skill status = %d body=%s, want 404", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authedRequest(http.MethodGet, "/api/v1/goals", nil, agentSecret))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("revoked agent secret status = %d body=%s, want 401", rr.Code, rr.Body.String())
	}
}

func TestRouterCreatesTelegramLinkCode(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodPost, "/api/v1/telegram/link-codes", strings.NewReader(`{}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("telegram link code status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created service.CreatedTelegramLinkCode
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode telegram link code: %v", err)
	}
	if len(created.Code) < 8 || strings.HasPrefix(created.Code, "sk_") || created.ExpiresAt.IsZero() {
		t.Fatalf("telegram link code response = %+v", created)
	}
}

func TestRouterInternalTelegramLinkUsesBotSecretAndConsumesCode(t *testing.T) {
	router, rawKey := newTestRouterWithBotSecret(t, "bot-secret")

	req := authedRequest(http.MethodPost, "/api/v1/telegram/link-codes", strings.NewReader(`{}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create link code status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created service.CreatedTelegramLinkCode
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode link code: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/internal/telegram/link", strings.NewReader(`{"chat_id":-1001234567890,"chat_kind":"channel","code":"`+created.Code+`"}`))
	req.Header.Set("Authorization", "Bearer bot-secret")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("internal telegram link status = %d body=%s", rr.Code, rr.Body.String())
	}
	var linked struct {
		Reply string              `json:"reply"`
		Link  domain.TelegramLink `json:"telegram_link"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &linked); err != nil {
		t.Fatalf("decode linked response: %v", err)
	}
	if linked.Reply != "Linked to Goal Stakes." || linked.Link.ChatID != -1001234567890 || linked.Link.ChatKind != "channel" {
		t.Fatalf("linked response = %+v", linked)
	}

	req = httptest.NewRequest(http.MethodPost, "/internal/telegram/link", strings.NewReader(`{"chat_id":42,"chat_kind":"private","code":"`+created.Code+`"}`))
	req.Header.Set("Authorization", "Bearer bot-secret")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("reused telegram code status = %d body=%s, want 400", rr.Code, rr.Body.String())
	}
}

func TestRouterInternalTelegramLinkRejectsWrongBotSecret(t *testing.T) {
	router, _ := newTestRouterWithBotSecret(t, "bot-secret")

	req := httptest.NewRequest(http.MethodPost, "/internal/telegram/link", strings.NewReader(`{"chat_id":42,"chat_kind":"private","code":"ABC"}`))
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong bot secret status = %d body=%s, want 401", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "Unauthorized")
}

func TestRouterInternalTelegramMessageRequiresLinkedChat(t *testing.T) {
	router, _ := newTestRouterWithBotSecret(t, "bot-secret")

	req := httptest.NewRequest(http.MethodPost, "/internal/telegram/message", strings.NewReader(`{"chat_id":42,"chat_kind":"private","message_id":7,"text":"/goals"}`))
	req.Header.Set("Authorization", "Bearer bot-secret")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("telegram message status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result struct {
		Reply string `json:"reply"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode telegram message: %v", err)
	}
	if !strings.Contains(result.Reply, "Link this Telegram chat first") || !strings.Contains(result.Reply, "/link") {
		t.Fatalf("reply = %q, want link instruction", result.Reply)
	}
}

func TestRouterInternalTelegramMessageListsGoalsForLinkedChat(t *testing.T) {
	router, rawKey := newTestRouterWithBotSecret(t, "bot-secret")
	linkTelegramChatForTest(t, router, rawKey, "bot-secret", 42, "private")

	req := authedRequest(http.MethodPost, "/api/v1/goals", strings.NewReader(`{
		"title":"Do push-ups",
		"type":"do",
		"cadence":"daily",
		"stake_amount":"1000000",
		"token_symbol":"USDC",
		"chain":"sepolia"
	}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create setup goal status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/internal/telegram/message", strings.NewReader(`{"chat_id":42,"chat_kind":"private","message_id":8,"text":"/goals"}`))
	req.Header.Set("Authorization", "Bearer bot-secret")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("telegram goals status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result struct {
		Reply string `json:"reply"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode telegram goals: %v", err)
	}
	if !strings.Contains(result.Reply, "Do push-ups") || !strings.Contains(result.Reply, "USDC") {
		t.Fatalf("reply = %q, want listed goal", result.Reply)
	}
}

func TestRouterInternalTelegramMessageRunsGoalCommands(t *testing.T) {
	router, rawKey := newTestRouterWithBotSecret(t, "bot-secret")
	linkTelegramChatForTest(t, router, rawKey, "bot-secret", -100123, "channel")

	reply := postTelegramMessage(t, router, "bot-secret", -100123, "channel", 9, "/create do daily 1 USDC sepolia Telegram push-ups")
	if !strings.Contains(reply, "Created goal") || !strings.Contains(reply, "Telegram push-ups") {
		t.Fatalf("create reply = %q, want created goal", reply)
	}
	goals := listGoalsForTest(t, router, rawKey)
	if len(goals) != 1 || goals[0].Title != "Telegram push-ups" || goals[0].StakeAmount != "1000000" {
		t.Fatalf("goals after create = %+v", goals)
	}

	reply = postTelegramMessage(t, router, "bot-secret", -100123, "channel", 10, "/done "+goals[0].ID.String()+" finished today")
	if !strings.Contains(reply, "Check-in recorded") || !strings.Contains(reply, "2026-05-25") {
		t.Fatalf("done reply = %q, want check-in confirmation", reply)
	}

	reply = postTelegramMessage(t, router, "bot-secret", -100123, "channel", 11, "/progress "+goals[0].ID.String())
	if !strings.Contains(reply, "Telegram push-ups") || !strings.Contains(reply, "completed: yes") {
		t.Fatalf("progress reply = %q, want completed progress", reply)
	}

	reply = postTelegramMessage(t, router, "bot-secret", -100123, "channel", 12, "/archive "+goals[0].ID.String())
	if !strings.Contains(reply, "Goal archived") {
		t.Fatalf("archive reply = %q, want archive confirmation", reply)
	}
	if goals = listGoalsForTest(t, router, rawKey); len(goals) != 0 {
		t.Fatalf("goals after archive = %+v, want none", goals)
	}

	reply = postTelegramMessage(t, router, "bot-secret", -100123, "channel", 13, "/create avoid daily 1 USDC sepolia No soda")
	if !strings.Contains(reply, "Created goal") || !strings.Contains(reply, "No soda") {
		t.Fatalf("avoid create reply = %q, want created goal", reply)
	}
	goals = listGoalsForTest(t, router, rawKey)
	if len(goals) != 1 || goals[0].Title != "No soda" {
		t.Fatalf("avoid goals = %+v", goals)
	}
	reply = postTelegramMessage(t, router, "bot-secret", -100123, "channel", 14, "/violate "+goals[0].ID.String()+" drank soda")
	if !strings.Contains(reply, "Violation recorded") || !strings.Contains(reply, "pending") {
		t.Fatalf("violate reply = %q, want violation confirmation", reply)
	}
}

func TestRouterInternalTelegramMessageHandlesMalformedCommands(t *testing.T) {
	router, rawKey := newTestRouterWithBotSecret(t, "bot-secret")
	linkTelegramChatForTest(t, router, rawKey, "bot-secret", 42, "private")

	reply := postTelegramMessage(t, router, "bot-secret", 42, "private", 7, "/create do")

	if !strings.Contains(reply, "Usage: /create") {
		t.Fatalf("reply = %q, want create usage", reply)
	}
	if goals := listGoalsForTest(t, router, rawKey); len(goals) != 0 {
		t.Fatalf("malformed command created goals: %+v", goals)
	}
}

func TestRouterInternalTelegramMessageSendsFreeTextToAI(t *testing.T) {
	router, rawKey, aiClient := newTestRouterWithAIAndBotSecret(t, "bot-secret")
	linkTelegramChatForTest(t, router, rawKey, "bot-secret", 42, "private")

	reply := postTelegramMessage(t, router, "bot-secret", 42, "private", 8, "I did 10 push-ups")

	if reply != "Hello from the goal manager." {
		t.Fatalf("reply = %q, want AI reply", reply)
	}
	if len(aiClient.requests) != 1 {
		t.Fatalf("AI request count = %d, want 1", len(aiClient.requests))
	}
	got := aiClient.requests[0].Messages[len(aiClient.requests[0].Messages)-1].Content
	if got != "I did 10 push-ups" {
		t.Fatalf("AI message = %q, want Telegram free text", got)
	}
}

func TestRouterInternalTelegramAudioTranscribesLinkedChannelVoice(t *testing.T) {
	transcriber := &fakeTranscriber{transcript: "я отжался 10 раз"}
	router, rawKey, aiClient := newTestRouterWithAIAndBotSecretAndTranscriber(t, "bot-secret", transcriber)
	linkTelegramChatForTest(t, router, rawKey, "bot-secret", -100123, "channel")

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", "-100123"); err != nil {
		t.Fatalf("write chat_id: %v", err)
	}
	if err := writer.WriteField("chat_kind", "channel"); err != nil {
		t.Fatalf("write chat_kind: %v", err)
	}
	if err := writer.WriteField("message_id", "301"); err != nil {
		t.Fatalf("write message_id: %v", err)
	}
	part, err := writer.CreatePart(textFileHeader("audio", "voice.ogg", "audio/ogg"))
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write([]byte("fake-telegram-ogg")); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/telegram/audio", &body)
	req.Header.Set("Authorization", "Bearer bot-secret")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("telegram audio status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result ai.AudioChatResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode telegram audio: %v", err)
	}
	if result.Transcript != "я отжался 10 раз" || result.Reply != "Hello from the goal manager." || result.ConversationID == (domain.UUID{}) {
		t.Fatalf("result = %+v", result)
	}
	if len(transcriber.inputs) != 1 || transcriber.inputs[0].Filename != "voice.ogg" || transcriber.inputs[0].ContentType != "audio/ogg" || string(transcriber.inputs[0].Data) != "fake-telegram-ogg" {
		t.Fatalf("transcriber inputs = %+v", transcriber.inputs)
	}
	if len(aiClient.requests) != 1 || aiClient.requests[0].Messages[len(aiClient.requests[0].Messages)-1].Content != "я отжался 10 раз" {
		t.Fatalf("AI requests = %+v", aiClient.requests)
	}
}

func TestRouterInternalTelegramAudioRequiresLinkedChatBeforeTranscription(t *testing.T) {
	transcriber := &fakeTranscriber{transcript: "я отжался 10 раз"}
	router, _, _ := newTestRouterWithAIAndBotSecretAndTranscriber(t, "bot-secret", transcriber)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", "-100123"); err != nil {
		t.Fatalf("write chat_id: %v", err)
	}
	if err := writer.WriteField("chat_kind", "channel"); err != nil {
		t.Fatalf("write chat_kind: %v", err)
	}
	part, err := writer.CreatePart(textFileHeader("audio", "voice.ogg", "audio/ogg"))
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write([]byte("fake-telegram-ogg")); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/telegram/audio", &body)
	req.Header.Set("Authorization", "Bearer bot-secret")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("telegram audio status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result struct {
		Reply string `json:"reply"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode telegram audio: %v", err)
	}
	if !strings.Contains(result.Reply, "Link this Telegram chat first") {
		t.Fatalf("reply = %q, want link instruction", result.Reply)
	}
	if len(transcriber.inputs) != 0 {
		t.Fatalf("unlinked chat should not transcribe audio: %+v", transcriber.inputs)
	}
}

func TestRouterInternalTelegramAgentLinkCreatesSkillForLinkedChat(t *testing.T) {
	router, rawKey := newTestRouterWithBotSecret(t, "bot-secret")
	linkTelegramChatForTest(t, router, rawKey, "bot-secret", 42, "private")

	req := httptest.NewRequest(http.MethodPost, "/internal/telegram/agent-link", strings.NewReader(`{"chat_id":42,"chat_kind":"private","name":"telegram"}`))
	req.Header.Set("Authorization", "Bearer bot-secret")
	req.Host = "api.goalstakes.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("telegram agent link status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result struct {
		Reply    string `json:"reply"`
		SkillURL string `json:"skill_url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode telegram agent link: %v", err)
	}
	if !strings.Contains(result.Reply, "Own-agent skill link") || !strings.HasPrefix(result.SkillURL, "https://api.goalstakes.test/agent-skills/agt_") {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.Reply, "Authorization: Bearer sk_") || strings.Contains(result.Reply, "sk_") {
		t.Fatalf("telegram reply leaked raw agent secret: %q", result.Reply)
	}
}

func TestRouterChat(t *testing.T) {
	router, rawKey, aiClient := newTestRouterWithAI(t)

	req := authedRequest(http.MethodPost, "/api/v1/chat", bytes.NewBufferString(`{"message":"hello"}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result ai.ChatResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode chat: %v", err)
	}
	if result.Reply != "Hello from the goal manager." || result.ConversationID == (domain.UUID{}) {
		t.Fatalf("unexpected chat result: %+v", result)
	}

	req = authedRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(`{"message":"continue","conversation_id":"`+result.ConversationID.String()+`"}`), rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("continued chat status = %d body=%s", rr.Code, rr.Body.String())
	}
	var continued ai.ChatResult
	if err := json.Unmarshal(rr.Body.Bytes(), &continued); err != nil {
		t.Fatalf("decode continued chat: %v", err)
	}
	if continued.ConversationID != result.ConversationID {
		t.Fatalf("continued conversation id = %s, want %s", continued.ConversationID, result.ConversationID)
	}
	if len(aiClient.requests) != 2 {
		t.Fatalf("AI request count = %d, want 2", len(aiClient.requests))
	}
	secondMessages := aiClient.requests[1].Messages
	if len(secondMessages) < 4 || secondMessages[1].Content != "hello" || secondMessages[2].Content != "Hello from the goal manager." || secondMessages[3].Content != "continue" {
		t.Fatalf("continued chat did not include prior conversation history: %+v", secondMessages)
	}
}

func TestRouterChatAudioRequiresAuth(t *testing.T) {
	router, _, _ := newTestRouterWithAI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/audio", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("chat audio status = %d body=%s, want %d", rr.Code, rr.Body.String(), http.StatusUnauthorized)
	}
	assertJSONError(t, rr, "Unauthorized")
}

func TestRouterChatAudioTranscribesAndChats(t *testing.T) {
	transcriber := &fakeTranscriber{transcript: "I did 10 push-ups"}
	router, rawKey, aiClient := newTestRouterWithAIAndTranscriber(t, transcriber)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "voice.ogg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("fake-ogg-audio")); err != nil {
		t.Fatalf("write multipart: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/audio", &body)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("chat audio status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result struct {
		Transcript     string      `json:"transcript"`
		ConversationID domain.UUID `json:"conversation_id"`
		Reply          string      `json:"reply"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode chat audio: %v", err)
	}
	if result.Transcript != "I did 10 push-ups" || result.Reply != "Hello from the goal manager." || result.ConversationID == (domain.UUID{}) {
		t.Fatalf("unexpected chat audio result: %+v", result)
	}
	if len(transcriber.inputs) != 1 {
		t.Fatalf("transcriber input count = %d, want 1", len(transcriber.inputs))
	}
	if transcriber.inputs[0].Filename != "voice.ogg" || transcriber.inputs[0].ContentType != "application/octet-stream" || string(transcriber.inputs[0].Data) != "fake-ogg-audio" {
		t.Fatalf("transcriber input = %+v", transcriber.inputs[0])
	}
	if len(aiClient.requests) != 1 || aiClient.requests[0].Messages[len(aiClient.requests[0].Messages)-1].Content != "I did 10 push-ups" {
		t.Fatalf("chat did not receive transcript: %+v", aiClient.requests)
	}
}

func TestRouterChatAudioRejectsMissingAudio(t *testing.T) {
	transcriber := &fakeTranscriber{transcript: "I did 10 push-ups"}
	router, rawKey, _ := newTestRouterWithAIAndTranscriber(t, transcriber)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/audio", &body)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("chat audio status = %d body=%s, want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	assertJSONError(t, rr, "audio file is required")
	if len(transcriber.inputs) != 0 {
		t.Fatalf("transcriber should not receive missing audio: %+v", transcriber.inputs)
	}
}

func TestRouterChatAudioRejectsUnsupportedContentType(t *testing.T) {
	transcriber := &fakeTranscriber{transcript: "I did 10 push-ups"}
	router, rawKey, _ := newTestRouterWithAIAndTranscriber(t, transcriber)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(textFileHeader("audio", "notes.txt", "text/plain"))
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write([]byte("not audio")); err != nil {
		t.Fatalf("write multipart: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/audio", &body)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("chat audio status = %d body=%s, want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	assertJSONError(t, rr, "unsupported audio content type")
	if len(transcriber.inputs) != 0 {
		t.Fatalf("transcriber should not receive unsupported audio: %+v", transcriber.inputs)
	}
}

func TestRouterChatAudioRejectsEmptyTranscript(t *testing.T) {
	transcriber := &fakeTranscriber{transcript: "   "}
	router, rawKey, _ := newTestRouterWithAIAndTranscriber(t, transcriber)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "voice.ogg")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("fake-ogg-audio")); err != nil {
		t.Fatalf("write multipart: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/audio", &body)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("chat audio status = %d body=%s, want %d", rr.Code, rr.Body.String(), http.StatusBadRequest)
	}
	assertJSONError(t, rr, "service: invalid input: transcript is empty")
}

func TestRouterChatUnavailableReturnsStructuredError(t *testing.T) {
	router, rawKey := newTestRouter(t)

	req := authedRequest(http.MethodPost, "/api/v1/chat", bytes.NewBufferString(`{"message":"hello"}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("chat status = %d body=%s", rr.Code, rr.Body.String())
	}
	assertJSONError(t, rr, "ai manager is not configured")
}

func newTestRouter(t *testing.T) (http.Handler, string) {
	t.Helper()
	return newTestRouterWithServiceOptions(t)
}

func newTestRouterWithServiceOptions(t *testing.T, opts ...service.Option) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	serviceOpts := []service.Option{service.WithClock(func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	})}
	serviceOpts = append(serviceOpts, opts...)
	svc, err := service.New(st, map[string]config.ChainConfig{
		"sepolia": {
			RPCURL:               "https://sepolia.example/rpc",
			StakeEnforcerAddress: "0x1111111111111111111111111111111111111111",
			Tokens: map[string]string{
				"USDC": "0x2222222222222222222222222222222222222222",
				"USDT": "0x3333333333333333333333333333333333333333",
			},
		},
	}, serviceOpts...)
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{
		Chain:           "sepolia",
		TokenSymbol:     "USDC",
		TxHash:          "0xtest-approval",
		DryRunAllowance: "1000000",
	}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	raw, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if _, err := st.CreateApiKey(ctx, store.CreateApiKeyInput{
		UserID: user.ID, Name: "test", Prefix: prefix, KeyHash: hash,
	}); err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	router := api.NewRouter(api.Config{
		Service: svc,
		APIKeys: auth.NewAPIKeyManager(st),
	})
	return router, raw
}

func newTestRouterWithBotSecret(t *testing.T, botSecret string) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc, err := service.New(st, map[string]config.ChainConfig{
		"sepolia": {
			RPCURL:               "https://sepolia.example/rpc",
			StakeEnforcerAddress: "0x1111111111111111111111111111111111111111",
			Tokens: map[string]string{
				"USDC": "0x2222222222222222222222222222222222222222",
				"USDT": "0x3333333333333333333333333333333333333333",
			},
		},
	}, service.WithClock(func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDC", TxHash: "0xtest-approval", DryRunAllowance: "1000000"}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	raw, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if _, err := st.CreateApiKey(ctx, store.CreateApiKeyInput{UserID: user.ID, Name: "test", Prefix: prefix, KeyHash: hash}); err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	return api.NewRouter(api.Config{Service: svc, APIKeys: auth.NewAPIKeyManager(st), TelegramBotSecret: botSecret}), raw
}

func linkTelegramChatForTest(t *testing.T, router http.Handler, rawKey, botSecret string, chatID int64, chatKind string) {
	t.Helper()
	req := authedRequest(http.MethodPost, "/api/v1/telegram/link-codes", strings.NewReader(`{}`), rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create link code status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created service.CreatedTelegramLinkCode
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode link code: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/internal/telegram/link", strings.NewReader(fmt.Sprintf(`{"chat_id":%d,"chat_kind":%q,"code":%q}`, chatID, chatKind, created.Code)))
	req.Header.Set("Authorization", "Bearer "+botSecret)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("link telegram chat status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func postTelegramMessage(t *testing.T, router http.Handler, botSecret string, chatID int64, chatKind string, messageID int64, text string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"chat_id":    chatID,
		"chat_kind":  chatKind,
		"message_id": messageID,
		"text":       text,
	})
	if err != nil {
		t.Fatalf("marshal telegram message: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/telegram/message", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+botSecret)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("telegram message status = %d body=%s", rr.Code, rr.Body.String())
	}
	var result struct {
		Reply string `json:"reply"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode telegram message: %v", err)
	}
	return result.Reply
}

func listGoalsForTest(t *testing.T, router http.Handler, rawKey string) []domain.Goal {
	t.Helper()
	req := authedRequest(http.MethodGet, "/api/v1/goals", nil, rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list goals status = %d body=%s", rr.Code, rr.Body.String())
	}
	var goals []domain.Goal
	if err := json.Unmarshal(rr.Body.Bytes(), &goals); err != nil {
		t.Fatalf("decode goals: %v", err)
	}
	return goals
}

func newTestRouterWithAI(t *testing.T) (http.Handler, string, *fakeAIClient) {
	t.Helper()
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc, err := service.New(st, map[string]config.ChainConfig{
		"sepolia": {
			RPCURL:               "https://sepolia.example/rpc",
			StakeEnforcerAddress: "0x1111111111111111111111111111111111111111",
			Tokens: map[string]string{
				"USDC": "0x2222222222222222222222222222222222222222",
				"USDT": "0x3333333333333333333333333333333333333333",
			},
		},
	}, service.WithClock(func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	raw, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if _, err := st.CreateApiKey(ctx, store.CreateApiKeyInput{UserID: user.ID, Name: "test", Prefix: prefix, KeyHash: hash}); err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	client := &fakeAIClient{response: openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "Hello from the goal manager.",
		}}},
	}}
	manager := ai.NewManagerWithClient(st, svc, client, "gpt-test")
	return api.NewRouter(api.Config{Service: svc, APIKeys: auth.NewAPIKeyManager(st), AI: manager}), raw, client
}

func newTestRouterWithAIAndBotSecret(t *testing.T, botSecret string) (http.Handler, string, *fakeAIClient) {
	t.Helper()
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc, err := service.New(st, map[string]config.ChainConfig{
		"sepolia": {
			RPCURL:               "https://sepolia.example/rpc",
			StakeEnforcerAddress: "0x1111111111111111111111111111111111111111",
			Tokens: map[string]string{
				"USDC": "0x2222222222222222222222222222222222222222",
				"USDT": "0x3333333333333333333333333333333333333333",
			},
		},
	}, service.WithClock(func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDC", TxHash: "0xtest-approval", DryRunAllowance: "1000000"}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	raw, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if _, err := st.CreateApiKey(ctx, store.CreateApiKeyInput{UserID: user.ID, Name: "test", Prefix: prefix, KeyHash: hash}); err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	client := &fakeAIClient{response: openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "Hello from the goal manager.",
		}}},
	}}
	manager := ai.NewManagerWithClient(st, svc, client, "gpt-test")
	return api.NewRouter(api.Config{Service: svc, APIKeys: auth.NewAPIKeyManager(st), AI: manager, TelegramBotSecret: botSecret}), raw, client
}

func newTestRouterWithAIAndBotSecretAndTranscriber(t *testing.T, botSecret string, transcriber ai.Transcriber) (http.Handler, string, *fakeAIClient) {
	t.Helper()
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc, err := service.New(st, map[string]config.ChainConfig{
		"sepolia": {
			RPCURL:               "https://sepolia.example/rpc",
			StakeEnforcerAddress: "0x1111111111111111111111111111111111111111",
			Tokens: map[string]string{
				"USDC": "0x2222222222222222222222222222222222222222",
				"USDT": "0x3333333333333333333333333333333333333333",
			},
		},
	}, service.WithClock(func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDC", TxHash: "0xtest-approval", DryRunAllowance: "1000000"}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	raw, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if _, err := st.CreateApiKey(ctx, store.CreateApiKeyInput{UserID: user.ID, Name: "test", Prefix: prefix, KeyHash: hash}); err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	client := &fakeAIClient{response: openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "Hello from the goal manager.",
		}}},
	}}
	manager := ai.NewManagerWithClientAndTranscriber(st, svc, client, transcriber, "gpt-test")
	return api.NewRouter(api.Config{Service: svc, APIKeys: auth.NewAPIKeyManager(st), AI: manager, TelegramBotSecret: botSecret}), raw, client
}

func newTestRouterWithAIAndTranscriber(t *testing.T, transcriber ai.Transcriber) (http.Handler, string, *fakeAIClient) {
	t.Helper()
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc, err := service.New(st, map[string]config.ChainConfig{
		"sepolia": {
			RPCURL:               "https://sepolia.example/rpc",
			StakeEnforcerAddress: "0x1111111111111111111111111111111111111111",
			Tokens: map[string]string{
				"USDC": "0x2222222222222222222222222222222222222222",
				"USDT": "0x3333333333333333333333333333333333333333",
			},
		},
	}, service.WithClock(func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	raw, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if _, err := st.CreateApiKey(ctx, store.CreateApiKeyInput{UserID: user.ID, Name: "test", Prefix: prefix, KeyHash: hash}); err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	client := &fakeAIClient{response: openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "Hello from the goal manager.",
		}}},
	}}
	manager := ai.NewManagerWithClientAndTranscriber(st, svc, client, transcriber, "gpt-test")
	return api.NewRouter(api.Config{Service: svc, APIKeys: auth.NewAPIKeyManager(st), AI: manager}), raw, client
}

type fakeAIClient struct {
	requests []openai.ChatCompletionRequest
	response openai.ChatCompletionResponse
}

func (f *fakeAIClient) CreateChatCompletion(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	f.requests = append(f.requests, req)
	return f.response, nil
}

type fakeTranscriber struct {
	inputs     []ai.AudioInput
	transcript string
	err        error
}

func (f *fakeTranscriber) Transcribe(_ context.Context, input ai.AudioInput) (string, error) {
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return "", f.err
	}
	return f.transcript, nil
}

func textFileHeader(fieldName, filename, contentType string) textproto.MIMEHeader {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	return header
}

func extractAgentSecret(t *testing.T, markdown string) string {
	t.Helper()
	for _, line := range strings.Split(markdown, "\n") {
		if secret, ok := strings.CutPrefix(strings.TrimSpace(line), "Authorization: Bearer "); ok {
			return strings.TrimSpace(secret)
		}
	}
	t.Fatalf("skill markdown did not contain Authorization bearer line:\n%s", markdown)
	return ""
}

type failingPenaltyCharger struct{}

func (failingPenaltyCharger) Penalize(context.Context, string, string, string, string) (string, error) {
	return "0xreverted", errors.New("penalize tx reverted")
}

func authedRequest(method, path string, body io.Reader, rawKey string) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func assertJSONError(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json; body=%s", got, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v body=%s", err, rr.Body.String())
	}
	if body["error"] != want {
		t.Fatalf("error = %q, want %q", body["error"], want)
	}
}
