package goalstakes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type Goal struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	Cadence     string `json:"cadence"`
	StakeAmount string `json:"stake_amount"`
	TokenSymbol string `json:"token_symbol"`
	Chain       string `json:"chain"`
}

type CreateGoalInput struct {
	Title       string `json:"title"`
	Type        string `json:"type"`
	Cadence     string `json:"cadence"`
	StakeAmount string `json:"stake_amount"`
	TokenSymbol string `json:"token_symbol"`
	Chain       string `json:"chain"`
}

type Progress struct {
	CurrentPeriod          string `json:"current_period"`
	CurrentPeriodCompleted bool   `json:"current_period_completed"`
	Violations             []any  `json:"violations"`
}

type ChatResult struct {
	Reply string `json:"reply"`
}

type TelegramLinkResult struct {
	Reply string `json:"reply"`
}

type TelegramMessageResult struct {
	Reply string `json:"reply"`
}

type TelegramAudioResult struct {
	Transcript     string `json:"transcript"`
	ConversationID string `json:"conversation_id"`
	Reply          string `json:"reply"`
}

type TelegramAgentLinkResult struct {
	Reply    string `json:"reply"`
	SkillURL string `json:"skill_url"`
}

func NewClient(rawBaseURL string) *Client {
	base := strings.TrimRight(strings.TrimSpace(rawBaseURL), "/")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	return &Client{
		baseURL: base,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

func NewClientWithHTTP(rawBaseURL string, httpClient *http.Client) *Client {
	client := NewClient(rawBaseURL)
	if httpClient != nil {
		client.http = httpClient
	}
	return client
}

func (c *Client) VerifyAPIKey(ctx context.Context, apiKey string) error {
	var out map[string]string
	return c.request(ctx, http.MethodGet, "/api/v1/me", apiKey, nil, &out)
}

func (c *Client) ListGoals(ctx context.Context, apiKey string) ([]Goal, error) {
	var goals []Goal
	if err := c.request(ctx, http.MethodGet, "/api/v1/goals", apiKey, nil, &goals); err != nil {
		return nil, err
	}
	return goals, nil
}

func (c *Client) CreateGoal(ctx context.Context, apiKey string, in CreateGoalInput) (Goal, error) {
	var goal Goal
	if err := c.request(ctx, http.MethodPost, "/api/v1/goals", apiKey, in, &goal); err != nil {
		return Goal{}, err
	}
	return goal, nil
}

func (c *Client) LogCheckIn(ctx context.Context, apiKey string, goalID, note string) error {
	path := "/api/v1/goals/" + url.PathEscape(goalID) + "/checkins"
	return c.request(ctx, http.MethodPost, path, apiKey, map[string]string{"note": note}, nil)
}

func (c *Client) ReportViolation(ctx context.Context, apiKey string, goalID, reason string) error {
	path := "/api/v1/goals/" + url.PathEscape(goalID) + "/violations"
	return c.request(ctx, http.MethodPost, path, apiKey, map[string]string{"reason": reason}, nil)
}

func (c *Client) GetProgress(ctx context.Context, apiKey string, goalID string) (Progress, error) {
	var progress Progress
	path := "/api/v1/goals/" + url.PathEscape(goalID) + "/progress"
	if err := c.request(ctx, http.MethodGet, path, apiKey, nil, &progress); err != nil {
		return Progress{}, err
	}
	return progress, nil
}

func (c *Client) ArchiveGoal(ctx context.Context, apiKey string, goalID string) error {
	path := "/api/v1/goals/" + url.PathEscape(goalID)
	return c.request(ctx, http.MethodDelete, path, apiKey, nil, nil)
}

func (c *Client) Chat(ctx context.Context, apiKey string, message string) (string, error) {
	var result ChatResult
	if err := c.request(ctx, http.MethodPost, "/api/v1/chat", apiKey, map[string]string{"message": message}, &result); err != nil {
		return "", err
	}
	return result.Reply, nil
}

func (c *Client) LinkTelegram(ctx context.Context, botSecret string, chatID int64, chatKind, code string) (string, error) {
	var result TelegramLinkResult
	body := map[string]any{
		"chat_id":   chatID,
		"chat_kind": chatKind,
		"code":      code,
	}
	if err := c.request(ctx, http.MethodPost, "/internal/telegram/link", botSecret, body, &result); err != nil {
		return "", err
	}
	return result.Reply, nil
}

func (c *Client) TelegramMessage(ctx context.Context, botSecret string, chatID int64, chatKind string, messageID int64, text string) (string, error) {
	var result TelegramMessageResult
	body := map[string]any{
		"chat_id":    chatID,
		"chat_kind":  chatKind,
		"message_id": messageID,
		"text":       text,
	}
	if err := c.request(ctx, http.MethodPost, "/internal/telegram/message", botSecret, body, &result); err != nil {
		return "", err
	}
	return result.Reply, nil
}

func (c *Client) TelegramAudio(ctx context.Context, botSecret string, chatID int64, chatKind string, messageID int64, filename, contentType string, audio []byte) (TelegramAudioResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", fmt.Sprintf("%d", chatID)); err != nil {
		return TelegramAudioResult{}, fmt.Errorf("write chat_id: %w", err)
	}
	if err := writer.WriteField("chat_kind", chatKind); err != nil {
		return TelegramAudioResult{}, fmt.Errorf("write chat_kind: %w", err)
	}
	if messageID != 0 {
		if err := writer.WriteField("message_id", fmt.Sprintf("%d", messageID)); err != nil {
			return TelegramAudioResult{}, fmt.Errorf("write message_id: %w", err)
		}
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="audio"; filename="`+escapeMultipartQuote(filenameOrDefault(filename))+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return TelegramAudioResult{}, fmt.Errorf("create audio part: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return TelegramAudioResult{}, fmt.Errorf("write audio part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return TelegramAudioResult{}, fmt.Errorf("close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/telegram/audio", &body)
	if err != nil {
		return TelegramAudioResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(botSecret))
	resp, err := c.http.Do(req)
	if err != nil {
		return TelegramAudioResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return TelegramAudioResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TelegramAudioResult{}, fmt.Errorf("goalstakes api: %s", responseError(resp, raw))
	}
	var result TelegramAudioResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return TelegramAudioResult{}, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

func (c *Client) TelegramAgentLink(ctx context.Context, botSecret string, chatID int64, chatKind string) (string, error) {
	var result TelegramAgentLinkResult
	body := map[string]any{
		"chat_id":   chatID,
		"chat_kind": chatKind,
		"name":      "telegram",
	}
	if err := c.request(ctx, http.MethodPost, "/internal/telegram/agent-link", botSecret, body, &result); err != nil {
		return "", err
	}
	return result.Reply, nil
}

func filenameOrDefault(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "telegram-audio.bin"
	}
	return filename
}

func escapeMultipartQuote(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}

func (c *Client) request(ctx context.Context, method, path, apiKey string, body any, out any) error {
	var requestBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		requestBody = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("goalstakes api: %s", responseError(resp, raw))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func responseError(resp *http.Response, raw []byte) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err == nil && strings.TrimSpace(body.Error) != "" {
		return body.Error
	}
	text := strings.TrimSpace(string(raw))
	if text != "" {
		return text
	}
	return resp.Status
}
