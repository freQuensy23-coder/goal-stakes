package goalstakes_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "goalstakes-telegram-bot/internal/goalstakes"
)

func TestClientListGoalsUsesBearerAPIKey(t *testing.T) {
	var gotAuth string
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/v1/goals" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		writeJSON(t, w, []map[string]any{{
			"id": "goal-1", "title": "Do push-ups", "type": "do", "cadence": "daily",
			"stake_amount": "1000000", "token_symbol": "USDC", "chain": "polygon",
		}})
	}))

	goals, err := client.ListGoals(t.Context(), "sk_test")
	if err != nil {
		t.Fatalf("ListGoals error: %v", err)
	}
	if gotAuth != "Bearer sk_test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if len(goals) != 1 || goals[0].Title != "Do push-ups" {
		t.Fatalf("goals = %+v", goals)
	}
}

func TestClientCreateGoalSendsRequestBody(t *testing.T) {
	var body map[string]any
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/goals" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		writeJSON(t, w, map[string]any{
			"id": "goal-1", "title": body["title"], "type": body["type"], "cadence": body["cadence"],
			"stake_amount": body["stake_amount"], "token_symbol": body["token_symbol"], "chain": body["chain"],
		})
	}))

	goal, err := client.CreateGoal(t.Context(), "sk_test", CreateGoalInput{
		Title: "Gym", Type: "do", Cadence: "weekly", StakeAmount: "2500000", TokenSymbol: "USDC", Chain: "polygon",
	})
	if err != nil {
		t.Fatalf("CreateGoal error: %v", err)
	}
	if goal.ID != "goal-1" || body["stake_amount"] != "2500000" || body["token_symbol"] != "USDC" {
		t.Fatalf("goal = %+v body = %+v", goal, body)
	}
}

func TestClientLogCheckInSendsNote(t *testing.T) {
	var body map[string]any
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/goals/goal-1/checkins" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))

	if err := client.LogCheckIn(t.Context(), "sk_test", "goal-1", "finished today"); err != nil {
		t.Fatalf("LogCheckIn error: %v", err)
	}
	if body["note"] != "finished today" {
		t.Fatalf("body = %+v, want note", body)
	}
}

func TestClientReportViolationSendsReason(t *testing.T) {
	var body map[string]any
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/goals/goal-1/violations" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))

	if err := client.ReportViolation(t.Context(), "sk_test", "goal-1", "broke the rule"); err != nil {
		t.Fatalf("ReportViolation error: %v", err)
	}
	if body["reason"] != "broke the rule" {
		t.Fatalf("body = %+v, want reason", body)
	}
}

func TestClientGetProgressParsesViolationCountSource(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/goals/goal-1/progress" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, map[string]any{
			"current_period":           "2026-06-29",
			"current_period_completed": true,
			"violations":               []map[string]any{{"id": "v1"}, {"id": "v2"}},
		})
	}))

	progress, err := client.GetProgress(t.Context(), "sk_test", "goal-1")
	if err != nil {
		t.Fatalf("GetProgress error: %v", err)
	}
	if progress.CurrentPeriod != "2026-06-29" || !progress.CurrentPeriodCompleted || len(progress.Violations) != 2 {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestClientArchiveGoalSendsDelete(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/goals/goal-1" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := client.ArchiveGoal(t.Context(), "sk_test", "goal-1"); err != nil {
		t.Fatalf("ArchiveGoal error: %v", err)
	}
}

func TestClientChatPostsMessageAndReturnsReply(t *testing.T) {
	var body map[string]any
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/chat" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		writeJSON(t, w, map[string]any{"reply": "Created through Telegram chat."})
	}))

	reply, err := client.Chat(t.Context(), "sk_test", "Create a daily push-up goal")
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if body["message"] != "Create a daily push-up goal" || reply != "Created through Telegram chat." {
		t.Fatalf("body = %+v reply = %q", body, reply)
	}
}

func TestClientLinkTelegramUsesBotSecret(t *testing.T) {
	var gotAuth string
	var body map[string]any
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/internal/telegram/link" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		writeJSON(t, w, map[string]any{"reply": "Linked to Goal Stakes."})
	}))

	reply, err := client.LinkTelegram(t.Context(), "bot-secret", -100123, "channel", "ABCD1234")
	if err != nil {
		t.Fatalf("LinkTelegram error: %v", err)
	}
	if gotAuth != "Bearer bot-secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if body["chat_id"] != float64(-100123) || body["chat_kind"] != "channel" || body["code"] != "ABCD1234" || reply != "Linked to Goal Stakes." {
		t.Fatalf("body = %+v reply = %q", body, reply)
	}
}

func TestClientTelegramMessageUsesInternalBotSecret(t *testing.T) {
	var gotAuth string
	var body map[string]any
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/internal/telegram/message" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		writeJSON(t, w, map[string]any{"reply": "No active goals."})
	}))

	reply, err := client.TelegramMessage(t.Context(), "bot-secret", -100123, "channel", 99, "/goals")
	if err != nil {
		t.Fatalf("TelegramMessage error: %v", err)
	}
	if gotAuth != "Bearer bot-secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if body["chat_id"] != float64(-100123) || body["chat_kind"] != "channel" || body["message_id"] != float64(99) || body["text"] != "/goals" || reply != "No active goals." {
		t.Fatalf("body = %+v reply = %q", body, reply)
	}
}

func TestClientTelegramAudioUsesMultipartInternalEndpoint(t *testing.T) {
	var gotAuth string
	var gotContentType string
	var rawBody string
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if r.Method != http.MethodPost || r.URL.Path != "/internal/telegram/audio" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBody = string(body)
		writeJSON(t, w, map[string]any{"transcript": "я отжался 10 раз", "conversation_id": "00000000-0000-0000-0000-000000000001", "reply": "Записал: 10 отжиманий"})
	}))

	result, err := client.TelegramAudio(t.Context(), "bot-secret", -100123, "channel", 301, "voice.ogg", "audio/ogg", []byte("fake-ogg"))
	if err != nil {
		t.Fatalf("TelegramAudio error: %v", err)
	}
	if gotAuth != "Bearer bot-secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data; boundary=") {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
	for _, expected := range []string{
		`name="chat_id"`,
		"-100123",
		`name="chat_kind"`,
		"channel",
		`name="message_id"`,
		"301",
		`name="audio"; filename="voice.ogg"`,
		"Content-Type: audio/ogg",
		"fake-ogg",
	} {
		if !strings.Contains(rawBody, expected) {
			t.Fatalf("multipart body missing %q in:\n%s", expected, rawBody)
		}
	}
	if result.Transcript != "я отжался 10 раз" || result.Reply != "Записал: 10 отжиманий" {
		t.Fatalf("result = %+v", result)
	}
}

func TestClientTelegramAgentLinkUsesInternalBotSecret(t *testing.T) {
	var gotAuth string
	var body map[string]any
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/internal/telegram/agent-link" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		writeJSON(t, w, map[string]any{"reply": "Own-agent skill link: https://api.goalstakes.test/agent-skills/agt_private.md", "skill_url": "https://api.goalstakes.test/agent-skills/agt_private.md"})
	}))

	reply, err := client.TelegramAgentLink(t.Context(), "bot-secret", -100123, "channel")
	if err != nil {
		t.Fatalf("TelegramAgentLink error: %v", err)
	}
	if gotAuth != "Bearer bot-secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if body["chat_id"] != float64(-100123) || body["chat_kind"] != "channel" || body["name"] != "telegram" || !strings.Contains(reply, "/agent-skills/agt_private.md") {
		t.Fatalf("body = %+v reply = %q", body, reply)
	}
	if strings.Contains(reply, "sk_") {
		t.Fatalf("reply leaked raw agent secret: %q", reply)
	}
}

func TestClientUsesStructuredErrorMessages(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"approval allowance is below stake amount"}`))
	}))

	_, err := client.ListGoals(t.Context(), "sk_test")
	if err == nil || !strings.Contains(err.Error(), "approval allowance is below stake amount") {
		t.Fatalf("error = %v", err)
	}
}

func newTestClient(handler http.Handler) *Client {
	return NewClientWithHTTP("http://goalstakes.test", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "http" || req.URL.Host != "goalstakes.test" {
			return nil, fmt.Errorf("unexpected test request URL %s", req.URL.String())
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Result(), nil
	})})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
