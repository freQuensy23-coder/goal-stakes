package ai_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"goalstakes/internal/ai"
	"goalstakes/internal/config"
	"goalstakes/internal/domain"
	"goalstakes/internal/service"
	"goalstakes/internal/store"

	openai "github.com/sashabaranov/go-openai"
)

func TestManagerExecutesToolCallAndPersistsConversation(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	mustApprove(t, svc, user.ID, "USDC", "1000000")
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:   "call-create",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "create_goal",
						Arguments: `{"title":"Do push-ups","type":"do","cadence":"daily","stake_amount":"1000000","token_symbol":"USDC","chain":"sepolia"}`,
					},
				}},
			}}},
		},
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "Created the push-up goal.",
			}}},
		},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	result, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "Create a push-up goal"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if result.Reply != "Created the push-up goal." {
		t.Fatalf("reply = %q", result.Reply)
	}
	goals, err := svc.ListGoals(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListGoals: %v", err)
	}
	if len(goals) != 1 || goals[0].Title != "Do push-ups" {
		t.Fatalf("tool call did not create expected goal: %+v", goals)
	}
	_, messages, err := st.GetConversation(ctx, result.ConversationID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != domain.RoleUser || messages[1].Role != domain.RoleAssistant {
		t.Fatalf("conversation messages not persisted as user+assistant: %+v", messages)
	}
	if len(client.requests) != 2 {
		t.Fatalf("client request count=%d want 2", len(client.requests))
	}
	if len(client.requests[0].Tools) == 0 {
		t.Fatal("first request must include AI tools")
	}
	toolSchema, err := json.Marshal(client.requests[0].Tools)
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	if strings.Contains(string(toolSchema), `"required":null`) {
		t.Fatalf("tool schemas must use an empty required array, not null: %s", toolSchema)
	}
	if got := client.requests[1].Messages[len(client.requests[1].Messages)-1]; got.Role != openai.ChatMessageRoleTool || got.ToolCallID != "call-create" {
		t.Fatalf("second request must include tool result message, got %+v", got)
	}
}

func TestManagerLetsModelRespondToToolErrors(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:   "call-create",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "create_goal",
						Arguments: `{"title":"Do push-ups","type":"do","cadence":"daily","stake_amount":"1000000","token_symbol":"USDC","chain":"sepolia"}`,
					},
				}},
			}}},
		},
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "Approve at least 1 USDC on Sepolia first.",
			}}},
		},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	result, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "Create a push-up goal"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if result.Reply != "Approve at least 1 USDC on Sepolia first." {
		t.Fatalf("reply = %q", result.Reply)
	}
	if len(client.requests) != 2 {
		t.Fatalf("client request count=%d want 2", len(client.requests))
	}
	toolResult := client.requests[1].Messages[len(client.requests[1].Messages)-1]
	if toolResult.Role != openai.ChatMessageRoleTool || toolResult.ToolCallID != "call-create" {
		t.Fatalf("second request missing tool error result: %+v", toolResult)
	}
	if !strings.Contains(toolResult.Content, `"ok":false`) || !strings.Contains(toolResult.Content, "approval allowance") {
		t.Fatalf("tool error result = %s, want structured allowance error", toolResult.Content)
	}
	goals, err := svc.ListGoals(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListGoals: %v", err)
	}
	if len(goals) != 0 {
		t.Fatalf("tool error should not create a goal: %+v", goals)
	}
}

func TestManagerTellsModelStakeAmountsUseTokenBaseUnits(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "I can do that.",
		}}}},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	if _, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "Create a push-up goal with a $100 stake"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("client request count=%d want 1", len(client.requests))
	}
	system := client.requests[0].Messages[0].Content
	if !strings.Contains(system, "smallest token units") || !strings.Contains(system, "$100") || !strings.Contains(system, "100000000") {
		t.Fatalf("system prompt does not explain stake unit conversion: %q", system)
	}
	if !strings.Contains(system, "approval allowance") {
		t.Fatalf("system prompt does not explain stake allowance requirement: %q", system)
	}
	if !strings.Contains(system, "ok=false") {
		t.Fatalf("system prompt does not explain tool error handling: %q", system)
	}
	createGoal := findTool(t, client.requests[0].Tools, "create_goal")
	var schema map[string]any
	raw, err := json.Marshal(createGoal.Function.Parameters)
	if err != nil {
		t.Fatalf("marshal tool parameters: %v", err)
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode tool parameters: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	stake := properties["stake_amount"].(map[string]any)
	description, _ := stake["description"].(string)
	if !strings.Contains(description, "smallest token units") || !strings.Contains(description, "$100") || !strings.Contains(description, "100000000") {
		t.Fatalf("stake_amount schema description does not explain conversion: %q", description)
	}
	if _, ok := properties["timezone"]; !ok {
		t.Fatalf("create_goal schema missing optional timezone property: %+v", properties)
	}
	if _, ok := properties["starts_at"]; !ok {
		t.Fatalf("create_goal schema missing optional starts_at property: %+v", properties)
	}
	if _, ok := properties["ends_at"]; !ok {
		t.Fatalf("create_goal schema missing optional ends_at property: %+v", properties)
	}
}

func TestManagerCreateGoalToolExposesOnlySupportedCadences(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "I can do that.",
		}}}},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	if _, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "Create a goal every few days"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	createGoal := findTool(t, client.requests[0].Tools, "create_goal")
	var schema map[string]any
	raw, err := json.Marshal(createGoal.Function.Parameters)
	if err != nil {
		t.Fatalf("marshal tool parameters: %v", err)
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode tool parameters: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	cadence := properties["cadence"].(map[string]any)
	got := cadence["enum"].([]any)
	if len(got) != 2 || got[0] != "daily" || got[1] != "weekly" {
		t.Fatalf("create_goal cadence enum = %+v, want daily/weekly only", got)
	}
}

func TestManagerUpdatesGoalWithToolCall(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	goal := mustGoal(t, svc, user.ID)
	mustApprove(t, svc, user.ID, "USDC", "2500000")
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:   "call-update",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "update_goal",
						Arguments: `{"goal_id":"` + goal.ID.String() + `","title":"Do 120 push-ups","description":"harder","stake_amount":"2500000"}`,
					},
				}},
			}}},
		},
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "Updated the goal.",
			}}},
		},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	if _, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "Rename my push-up goal and make it $2.50"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	updated, err := st.GetGoal(ctx, goal.ID)
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if updated.Title != "Do 120 push-ups" || updated.Description != "harder" || updated.StakeAmount != "2500000" {
		t.Fatalf("goal was not updated by tool call: %+v", updated)
	}
}

func TestManagerCreateGoalToolPersistsScheduleWindow(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	mustApprove(t, svc, user.ID, "USDC", "1000000")
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:   "call-create-scheduled",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "create_goal",
						Arguments: `{"title":"Go to the gym every week","type":"do","cadence":"weekly","stake_amount":"1000000","token_symbol":"USDC","chain":"sepolia","starts_at":"2026-06-01T09:00:00Z","ends_at":"2026-07-01T00:00:00Z"}`,
					},
				}},
			}}},
		},
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "Created the scheduled gym goal.",
			}}},
		},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	if _, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "Create a gym goal starting June 1 and ending July 1"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	goals, err := svc.ListGoals(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListGoals: %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("goals = %+v, want one scheduled goal", goals)
	}
	wantStart := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if !goals[0].StartsAt.Equal(wantStart) || goals[0].EndsAt == nil || !goals[0].EndsAt.Equal(wantEnd) {
		t.Fatalf("goal schedule = starts_at %v ends_at %v, want %v / %v", goals[0].StartsAt, goals[0].EndsAt, wantStart, wantEnd)
	}
}

func TestManagerUpdateGoalToolPersistsEndDate(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	goal := mustGoal(t, svc, user.ID)
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:   "call-update-end",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "update_goal",
						Arguments: `{"goal_id":"` + goal.ID.String() + `","title":"Do push-ups","ends_at":"2026-06-15T00:00:00Z"}`,
					},
				}},
			}}},
		},
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "Updated the goal end date.",
			}}},
		},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	if _, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "End my push-up goal on June 15"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	updateGoal := findTool(t, client.requests[0].Tools, "update_goal")
	var schema map[string]any
	raw, err := json.Marshal(updateGoal.Function.Parameters)
	if err != nil {
		t.Fatalf("marshal tool parameters: %v", err)
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode tool parameters: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	endsAtSchema, ok := properties["ends_at"].(map[string]any)
	if !ok {
		t.Fatalf("update_goal schema missing optional ends_at property: %+v", properties)
	}
	if !schemaAllowsNull(endsAtSchema) {
		t.Fatalf("update_goal ends_at schema should allow null clears: %+v", endsAtSchema)
	}
	updated, err := st.GetGoal(ctx, goal.ID)
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	wantEnd := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if updated.EndsAt == nil || !updated.EndsAt.Equal(wantEnd) {
		t.Fatalf("goal ends_at = %v, want %v", updated.EndsAt, wantEnd)
	}
}

func TestManagerUpdateGoalToolClearsEndDate(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	endsAt := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	goal := mustGoalWithEndDate(t, svc, user.ID, endsAt)
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:   "call-clear-end",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "update_goal",
						Arguments: `{"goal_id":"` + goal.ID.String() + `","title":"Do push-ups","ends_at":null}`,
					},
				}},
			}}},
		},
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "Cleared the goal end date.",
			}}},
		},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	if _, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "Remove the end date from my push-up goal"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	updated, err := st.GetGoal(ctx, goal.ID)
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if updated.EndsAt != nil {
		t.Fatalf("goal ends_at = %v, want cleared nil", updated.EndsAt)
	}
}

func TestManagerListsChainsWithToolCall(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:   "call-chains",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "list_chains",
						Arguments: `{}`,
					},
				}},
			}}},
		},
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "Sepolia supports USDC.",
			}}},
		},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	if _, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "Which chains can I use?"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("client request count=%d want 2", len(client.requests))
	}
	findTool(t, client.requests[0].Tools, "list_chains")
	toolResult := client.requests[1].Messages[len(client.requests[1].Messages)-1]
	if toolResult.Role != openai.ChatMessageRoleTool || toolResult.ToolCallID != "call-chains" {
		t.Fatalf("second request missing list_chains tool result: %+v", toolResult)
	}
	if !strings.Contains(toolResult.Content, `"key":"sepolia"`) || !strings.Contains(toolResult.Content, `"USDC"`) {
		t.Fatalf("list_chains tool result missing chain/token config: %s", toolResult.Content)
	}
}

func TestManagerArchivesGoalWithToolCall(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	goal := mustGoal(t, svc, user.ID)
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:   "call-archive",
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      "archive_goal",
						Arguments: `{"goal_id":"` + goal.ID.String() + `"}`,
					},
				}},
			}}},
		},
		{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: "Archived the goal.",
			}}},
		},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	if _, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "Archive my push-up goal"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	goals, err := svc.ListGoals(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListGoals: %v", err)
	}
	if len(goals) != 0 {
		t.Fatalf("archived goal still listed: %+v", goals)
	}
	archived, err := st.GetGoal(ctx, goal.ID)
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if !archived.Archived {
		t.Fatalf("goal was not archived by tool call: %+v", archived)
	}
}

func TestManagerDisabledWithoutClient(t *testing.T) {
	st := store.NewMemory()
	svc := mustService(t, st)
	mgr := ai.NewDisabledManager(st, svc)

	if _, err := mgr.Chat(context.Background(), domain.NewID(), ai.ChatInput{Message: "hello"}); err == nil {
		t.Fatal("disabled manager must reject chat")
	} else if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled error = %v", err)
	}
}

func TestManagerContinuesExistingConversation(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, "0xabc123", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := mustService(t, st)
	client := &fakeClient{responses: []openai.ChatCompletionResponse{
		{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "First reply.",
		}}}},
		{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "Second reply.",
		}}}},
	}}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	first, err := mgr.Chat(ctx, user.ID, ai.ChatInput{Message: "hello"})
	if err != nil {
		t.Fatalf("first Chat: %v", err)
	}
	second, err := mgr.Chat(ctx, user.ID, ai.ChatInput{ConversationID: first.ConversationID, Message: "what next"})
	if err != nil {
		t.Fatalf("second Chat: %v", err)
	}
	if second.ConversationID != first.ConversationID {
		t.Fatalf("conversation id changed: %s != %s", second.ConversationID, first.ConversationID)
	}

	_, messages, err := st.GetConversation(ctx, first.ConversationID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("persisted message count=%d want 4: %+v", len(messages), messages)
	}
	if len(client.requests) != 2 {
		t.Fatalf("client request count=%d want 2", len(client.requests))
	}
	secondRequest := client.requests[1].Messages
	if got := rolesAndContent(secondRequest); !strings.Contains(got, "|user:hello|assistant:First reply.|user:what next") {
		t.Fatalf("second request history = %s", got)
	}
}

func TestManagerRejectsConversationOwnedByAnotherUser(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	owner, err := st.CreateUser(ctx, "0xowner", "")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	other, err := st.CreateUser(ctx, "0xother", "")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	conversation, err := st.CreateConversation(ctx, owner.ID, "owner thread")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	svc := mustService(t, st)
	client := &fakeClient{}
	mgr := ai.NewManagerWithClient(st, svc, client, "gpt-test")

	if _, err := mgr.Chat(ctx, other.ID, ai.ChatInput{ConversationID: conversation.ID, Message: "hello"}); err == nil {
		t.Fatal("Chat must reject another user's conversation")
	}
	if len(client.requests) != 0 {
		t.Fatalf("client was called before ownership check: %d", len(client.requests))
	}
}

type fakeClient struct {
	requests  []openai.ChatCompletionRequest
	responses []openai.ChatCompletionResponse
}

func (f *fakeClient) CreateChatCompletion(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	f.requests = append(f.requests, req)
	if len(f.responses) == 0 {
		return openai.ChatCompletionResponse{}, nil
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp, nil
}

func rolesAndContent(messages []openai.ChatCompletionMessage) string {
	var parts []string
	for _, message := range messages {
		parts = append(parts, message.Role+":"+message.Content)
	}
	return strings.Join(parts, "|")
}

func findTool(t *testing.T, tools []openai.Tool, name string) openai.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Function != nil && tool.Function.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found in %+v", name, tools)
	return openai.Tool{}
}

func schemaAllowsNull(schema map[string]any) bool {
	if anyOf, ok := schema["anyOf"].([]any); ok {
		for _, item := range anyOf {
			child, ok := item.(map[string]any)
			if ok && child["type"] == "null" {
				return true
			}
		}
	}
	if types, ok := schema["type"].([]any); ok {
		for _, item := range types {
			if item == "null" {
				return true
			}
		}
	}
	return schema["nullable"] == true
}

func mustService(t *testing.T, st store.Store) *service.Service {
	t.Helper()
	svc, err := service.New(st, map[string]config.ChainConfig{
		"sepolia": {
			RPCURL:               "https://sepolia.example/rpc",
			StakeEnforcerAddress: "0x1111111111111111111111111111111111111111",
			Tokens: map[string]string{
				"USDC": "0x2222222222222222222222222222222222222222",
			},
		},
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return svc
}

func mustGoal(t *testing.T, svc *service.Service, userID domain.UUID) domain.Goal {
	t.Helper()
	mustApprove(t, svc, userID, "USDC", "1000000")
	goal, err := svc.CreateGoal(context.Background(), userID, service.CreateGoalInput{
		Title:       "Do push-ups",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	return goal
}

func mustGoalWithEndDate(t *testing.T, svc *service.Service, userID domain.UUID, endsAt time.Time) domain.Goal {
	t.Helper()
	mustApprove(t, svc, userID, "USDC", "1000000")
	goal, err := svc.CreateGoal(context.Background(), userID, service.CreateGoalInput{
		Title:       "Do push-ups",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
		EndsAt:      &endsAt,
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	return goal
}

func mustApprove(t *testing.T, svc *service.Service, userID domain.UUID, token, allowance string) {
	t.Helper()
	if _, err := svc.RecordApproval(context.Background(), userID, service.RecordApprovalInput{
		Chain:           "sepolia",
		TokenSymbol:     token,
		TxHash:          "0xtest-approval",
		DryRunAllowance: allowance,
	}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
}
