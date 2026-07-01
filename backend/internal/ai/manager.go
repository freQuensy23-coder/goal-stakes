package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"goalstakes/internal/domain"
	"goalstakes/internal/service"
	"goalstakes/internal/store"

	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

var ErrDisabled = errors.New("ai: disabled")

const systemPrompt = "You manage user goals by calling tools. Keep replies concise and report concrete results. Goals currently support daily and weekly cadences. Use an IANA timezone like America/New_York when the user specifies one; otherwise omit timezone and the backend uses UTC. If a tool returns ok=false, explain the concrete next step instead of pretending the action succeeded. stake_amount values are on-chain amounts in smallest token units with 6 decimals for USDC/USDT; convert human dollar amounts before tool calls, so $100 is 100000000 and $2.50 is 2500000. Creating a staked goal or raising its stake requires an approval allowance on the same chain/token that covers the stake amount."

type Client interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

type Transcriber interface {
	Transcribe(ctx context.Context, input AudioInput) (string, error)
}

type Manager struct {
	store       store.Store
	service     *service.Service
	client      Client
	transcriber Transcriber
	model       string
	enabled     bool
}

type ChatInput struct {
	Message        string      `json:"message"`
	ConversationID domain.UUID `json:"conversation_id,omitempty"`
}

type AudioInput struct {
	Filename    string
	ContentType string
	Data        []byte
}

type AudioChatInput struct {
	Audio          AudioInput
	ConversationID domain.UUID
}

type ChatResult struct {
	ConversationID domain.UUID `json:"conversation_id"`
	Reply          string      `json:"reply"`
}

type AudioChatResult struct {
	Transcript     string      `json:"transcript"`
	ConversationID domain.UUID `json:"conversation_id"`
	Reply          string      `json:"reply"`
}

func NewManager(st store.Store, svc *service.Service, apiKey, model, transcriptionModel string, baseURL string) (*Manager, error) {
	if strings.TrimSpace(apiKey) == "" {
		return NewDisabledManager(st, svc), nil
	}
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("ai: OPENAI_MODEL is required when OPENAI_API_KEY is set")
	}
	if strings.TrimSpace(transcriptionModel) == "" {
		return nil, errors.New("ai: OPENAI_TRANSCRIPTION_MODEL is required when OPENAI_API_KEY is set")
	}
	cfg := openai.DefaultConfig(apiKey)
	if strings.TrimSpace(baseURL) != "" {
		cfg.BaseURL = strings.TrimRight(baseURL, "/")
	}
	client := openai.NewClientWithConfig(cfg)
	return NewManagerWithClientAndTranscriber(st, svc, client, NewOpenAITranscriber(client, transcriptionModel), model), nil
}

func NewDisabledManager(st store.Store, svc *service.Service) *Manager {
	return &Manager{store: st, service: svc}
}

func NewManagerWithClient(st store.Store, svc *service.Service, client Client, model string) *Manager {
	return &Manager{store: st, service: svc, client: client, model: model, enabled: client != nil}
}

func NewManagerWithClientAndTranscriber(st store.Store, svc *service.Service, client Client, transcriber Transcriber, model string) *Manager {
	return &Manager{store: st, service: svc, client: client, transcriber: transcriber, model: model, enabled: client != nil}
}

func (m *Manager) Chat(ctx context.Context, userID domain.UUID, in ChatInput) (ChatResult, error) {
	if !m.enabled {
		return ChatResult{}, fmt.Errorf("%w: OPENAI_API_KEY is not configured", ErrDisabled)
	}
	text := strings.TrimSpace(in.Message)
	if text == "" {
		return ChatResult{}, fmt.Errorf("%w: message is required", service.ErrInvalid)
	}

	conversation, priorMessages, err := m.conversation(ctx, userID, in.ConversationID)
	if err != nil {
		return ChatResult{}, err
	}
	if _, err := m.store.AppendMessage(ctx, conversation.ID, domain.RoleUser, text); err != nil {
		return ChatResult{}, err
	}

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
	}
	messages = append(messages, openAIMessages(priorMessages)...)
	messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: text})
	reply := ""
	for i := 0; i < 4; i++ {
		resp, err := m.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:    m.model,
			Messages: messages,
			Tools:    tools(),
		})
		if err != nil {
			return ChatResult{}, err
		}
		if len(resp.Choices) == 0 {
			return ChatResult{}, errors.New("ai: empty chat completion")
		}
		msg := resp.Choices[0].Message
		messages = append(messages, msg)
		if len(msg.ToolCalls) == 0 {
			reply = msg.Content
			break
		}
		for _, call := range msg.ToolCalls {
			result, err := m.dispatchTool(ctx, userID, call.Function.Name, call.Function.Arguments)
			if err != nil {
				result = marshalToolError(err)
			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: call.ID,
				Content:    result,
			})
		}
	}
	if strings.TrimSpace(reply) == "" {
		return ChatResult{}, errors.New("ai: no assistant reply")
	}
	if _, err := m.store.AppendMessage(ctx, conversation.ID, domain.RoleAssistant, reply); err != nil {
		return ChatResult{}, err
	}
	return ChatResult{ConversationID: conversation.ID, Reply: reply}, nil
}

func (m *Manager) ChatAudio(ctx context.Context, userID domain.UUID, in AudioChatInput) (AudioChatResult, error) {
	if !m.enabled {
		return AudioChatResult{}, fmt.Errorf("%w: OPENAI_API_KEY is not configured", ErrDisabled)
	}
	if m.transcriber == nil {
		return AudioChatResult{}, fmt.Errorf("%w: audio transcription is not configured", ErrDisabled)
	}
	if len(in.Audio.Data) == 0 {
		return AudioChatResult{}, fmt.Errorf("%w: audio file is required", service.ErrInvalid)
	}
	transcript, err := m.transcriber.Transcribe(ctx, in.Audio)
	if err != nil {
		return AudioChatResult{}, err
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return AudioChatResult{}, fmt.Errorf("%w: transcript is empty", service.ErrInvalid)
	}
	chat, err := m.Chat(ctx, userID, ChatInput{Message: transcript, ConversationID: in.ConversationID})
	if err != nil {
		return AudioChatResult{}, err
	}
	return AudioChatResult{Transcript: transcript, ConversationID: chat.ConversationID, Reply: chat.Reply}, nil
}

func (m *Manager) conversation(ctx context.Context, userID, conversationID domain.UUID) (domain.Conversation, []domain.Message, error) {
	if conversationID == (domain.UUID{}) {
		conversation, err := m.store.CreateConversation(ctx, userID, "AI goal manager")
		return conversation, nil, err
	}
	conversation, messages, err := m.store.GetConversation(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, nil, err
	}
	if conversation.UserID != userID {
		return domain.Conversation{}, nil, fmt.Errorf("%w: conversation does not belong to user", service.ErrForbidden)
	}
	return conversation, messages, nil
}

func openAIMessages(messages []domain.Message) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, message := range messages {
		role := openai.ChatMessageRoleAssistant
		if message.Role == domain.RoleUser {
			role = openai.ChatMessageRoleUser
		}
		out = append(out, openai.ChatCompletionMessage{Role: role, Content: message.Content})
	}
	return out
}

func (m *Manager) dispatchTool(ctx context.Context, userID domain.UUID, name, args string) (string, error) {
	switch name {
	case "create_goal":
		var in service.CreateGoalInput
		if err := decodeArgs(args, &in); err != nil {
			return "", err
		}
		return marshalToolResult(m.service.CreateGoal(ctx, userID, in))
	case "list_goals":
		return marshalToolResult(m.service.ListGoals(ctx, userID))
	case "list_chains":
		return marshalToolResult(m.service.ListChains(), nil)
	case "update_goal":
		var in struct {
			GoalID      string     `json:"goal_id"`
			Title       string     `json:"title"`
			Description string     `json:"description"`
			StakeAmount string     `json:"stake_amount"`
			EndsAt      *time.Time `json:"ends_at"`
		}
		if err := decodeArgs(args, &in); err != nil {
			return "", err
		}
		endsAtSet, err := hasJSONField(args, "ends_at")
		if err != nil {
			return "", err
		}
		goalID, err := parseID(in.GoalID)
		if err != nil {
			return "", err
		}
		return marshalToolResult(m.service.UpdateGoal(ctx, userID, goalID, service.UpdateGoalInput{Title: in.Title, Description: in.Description, StakeAmount: in.StakeAmount, EndsAt: in.EndsAt, EndsAtSet: endsAtSet}))
	case "archive_goal":
		var in struct {
			GoalID string `json:"goal_id"`
		}
		if err := decodeArgs(args, &in); err != nil {
			return "", err
		}
		goalID, err := parseID(in.GoalID)
		if err != nil {
			return "", err
		}
		if err := m.service.ArchiveGoal(ctx, userID, goalID); err != nil {
			return "", err
		}
		return `{"archived":true}`, nil
	case "log_check_in":
		var in struct {
			GoalID string        `json:"goal_id"`
			Period domain.Period `json:"period"`
			Note   string        `json:"note"`
		}
		if err := decodeArgs(args, &in); err != nil {
			return "", err
		}
		goalID, err := parseID(in.GoalID)
		if err != nil {
			return "", err
		}
		return marshalToolResult(m.service.LogCheckIn(ctx, userID, goalID, service.LogCheckInInput{Period: in.Period, Note: in.Note}))
	case "report_violation":
		var in struct {
			GoalID string        `json:"goal_id"`
			Period domain.Period `json:"period"`
			Reason string        `json:"reason"`
		}
		if err := decodeArgs(args, &in); err != nil {
			return "", err
		}
		goalID, err := parseID(in.GoalID)
		if err != nil {
			return "", err
		}
		return marshalToolResult(m.service.ReportViolation(ctx, userID, goalID, service.ReportViolationInput{Period: in.Period, Reason: in.Reason}))
	case "get_progress":
		var in struct {
			GoalID string `json:"goal_id"`
		}
		if err := decodeArgs(args, &in); err != nil {
			return "", err
		}
		goalID, err := parseID(in.GoalID)
		if err != nil {
			return "", err
		}
		return marshalToolResult(m.service.GetProgress(ctx, userID, goalID))
	case "set_stake":
		var in struct {
			GoalID      string `json:"goal_id"`
			StakeAmount string `json:"stake_amount"`
			TokenSymbol string `json:"token_symbol"`
			Chain       string `json:"chain"`
		}
		if err := decodeArgs(args, &in); err != nil {
			return "", err
		}
		goalID, err := parseID(in.GoalID)
		if err != nil {
			return "", err
		}
		return marshalToolResult(m.service.SetStake(ctx, userID, goalID, service.SetStakeInput{StakeAmount: in.StakeAmount, TokenSymbol: in.TokenSymbol, Chain: in.Chain}))
	case "get_approval_status":
		var in struct {
			Chain       string `json:"chain"`
			TokenSymbol string `json:"token_symbol"`
		}
		if err := decodeArgs(args, &in); err != nil {
			return "", err
		}
		return marshalToolResult(m.service.GetApprovalStatus(ctx, userID, in.Chain, in.TokenSymbol))
	default:
		return "", fmt.Errorf("ai: unknown tool %q", name)
	}
}

func decodeArgs(raw string, dst any) error {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("ai: invalid tool args: %w", err)
	}
	return nil
}

func hasJSONField(raw, field string) (bool, error) {
	var fields map[string]json.RawMessage
	dec := json.NewDecoder(strings.NewReader(raw))
	if err := dec.Decode(&fields); err != nil {
		return false, fmt.Errorf("ai: invalid tool args: %w", err)
	}
	_, ok := fields[field]
	return ok, nil
}

func parseID(raw string) (domain.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return domain.UUID{}, fmt.Errorf("ai: invalid goal_id: %w", err)
	}
	return id, nil
}

func marshalToolResult[T any](value T, err error) (string, error) {
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func marshalToolError(err error) string {
	raw, marshalErr := json.Marshal(map[string]any{
		"ok":    false,
		"error": err.Error(),
	})
	if marshalErr != nil {
		return `{"ok":false,"error":"tool failed"}`
	}
	return string(raw)
}
