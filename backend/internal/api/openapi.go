package api

import (
	"encoding/json"
	"net/http"
)

const stakeAmountDescription = "Amount in smallest token units with 6 decimals for USDC/USDT. Convert human dollar amounts before calling: $100 -> 100000000, $2.50 -> 2500000. The wallet approval allowance on the same chain/token must cover this amount."
const timezoneDescription = "Optional IANA timezone for daily/weekly period boundaries, e.g. America/New_York. Defaults to UTC."

var openAPISpec = map[string]any{
	"openapi": "3.0.3",
	"info": map[string]any{
		"title":       "Goal Stakes Public API",
		"version":     "0.1.0",
		"description": "Wallet-authenticated goal tracking with token approval, monetary stakes, violations, API keys, and AI goal-manager chat.",
	},
	"servers": []map[string]string{{"url": "/"}},
	"security": []map[string]any{
		{"bearerAuth": []string{}},
	},
	"paths": map[string]any{
		"/api/v1/chains": map[string]any{
			"get": publicResponseOnly("List public chain, token, and StakeEnforcer addresses", "#/components/schemas/ChainInfoList"),
		},
		"/api/v1/auth/nonce": map[string]any{
			"post": authOp("Issue a 10-minute SIWE nonce", "#/components/schemas/NonceRequest", "#/components/schemas/NonceResponse"),
		},
		"/api/v1/auth/siwe": map[string]any{
			"post": authOp("Verify a SIWE message and issue a session JWT", "#/components/schemas/SIWERequest", "#/components/schemas/SIWEResponse"),
		},
		"/api/v1/me": map[string]any{
			"get": responseOnly("Return the authenticated user id", "#/components/schemas/MeResponse"),
		},
		"/api/v1/goals": map[string]any{
			"get":  responseOnly("List goals", "#/components/schemas/GoalList"),
			"post": createdOp("Create a goal", "#/components/schemas/CreateGoalRequest", "#/components/schemas/Goal"),
		},
		"/api/v1/goals/{goalID}": map[string]any{
			"patch":  withGoalID(op("Update a goal", true, "#/components/schemas/UpdateGoalRequest", "#/components/schemas/Goal")),
			"delete": withGoalID(noContent("Archive a goal")),
		},
		"/api/v1/goals/{goalID}/stake": map[string]any{
			"patch": withGoalID(op("Update a goal stake", true, "#/components/schemas/SetStakeRequest", "#/components/schemas/Goal")),
		},
		"/api/v1/goals/{goalID}/checkins": map[string]any{
			"post": withGoalID(createdOp("Log a check-in", "#/components/schemas/CheckInRequest", "#/components/schemas/CheckIn")),
		},
		"/api/v1/goals/{goalID}/violations": map[string]any{
			"get":  withGoalID(responseOnly("List violations", "#/components/schemas/ViolationList")),
			"post": withGoalID(reportViolationOp()),
		},
		"/api/v1/goals/{goalID}/progress": map[string]any{
			"get": withGoalID(responseOnly("Get goal progress", "#/components/schemas/Progress")),
		},
		"/api/v1/approvals": map[string]any{
			"get": withQueryParams(responseOnly("Get cached wallet approval status", "#/components/schemas/ApprovalStatus"), []map[string]any{
				queryParam("chain", true),
				queryParam("token_symbol", true),
			}),
			"post": op("Record wallet approval status", true, "#/components/schemas/RecordApprovalRequest", "#/components/schemas/ApprovalStatus"),
		},
		"/api/v1/apikeys": map[string]any{
			"get":  responseOnly("List API keys", "#/components/schemas/ApiKeyList"),
			"post": createdOp("Create an API key", "#/components/schemas/CreateApiKeyRequest", "#/components/schemas/CreatedApiKey"),
		},
		"/api/v1/apikeys/{apiKeyID}": map[string]any{
			"delete": withPathParam(noContent("Revoke an API key"), "apiKeyID"),
		},
		"/api/v1/agent-links": map[string]any{
			"get":  responseOnly("List active own-agent skill links", "#/components/schemas/AgentLinkList"),
			"post": createdOp("Create a private own-agent skill link", "#/components/schemas/CreateAgentLinkRequest", "#/components/schemas/CreatedAgentLink"),
		},
		"/api/v1/agent-links/{agentLinkID}": map[string]any{
			"delete": withPathParam(noContent("Revoke an own-agent skill link and its generated API key"), "agentLinkID"),
		},
		"/agent-skills/{token}.md": map[string]any{
			"get": agentSkillMarkdownOp(),
		},
		"/api/v1/telegram/link-codes": map[string]any{
			"post": createdOp("Create a one-time Telegram link code", "#/components/schemas/EmptyRequest", "#/components/schemas/CreatedTelegramLinkCode"),
		},
		"/api/v1/chat": map[string]any{
			"post": chatOp(),
		},
		"/api/v1/chat/audio": map[string]any{
			"post": audioChatOp(),
		},
		"/internal/telegram/link": map[string]any{
			"post": op("Consume a Telegram link code for a chat, group, or channel", true, "#/components/schemas/TelegramLinkRequest", "#/components/schemas/TelegramLinkResponse"),
		},
		"/internal/telegram/message": map[string]any{
			"post": op("Execute a Telegram text command or free-text message for a linked chat", true, "#/components/schemas/TelegramMessageRequest", "#/components/schemas/TelegramMessageResponse"),
		},
		"/internal/telegram/audio": map[string]any{
			"post": telegramAudioOp(),
		},
		"/internal/telegram/agent-link": map[string]any{
			"post": op("Create a private own-agent skill link for a linked Telegram chat", true, "#/components/schemas/TelegramAgentLinkRequest", "#/components/schemas/TelegramAgentLinkResponse"),
		},
	},
	"components": map[string]any{
		"securitySchemes": map[string]any{
			"bearerAuth": map[string]any{
				"type":        "http",
				"scheme":      "bearer",
				"description": "Use either a session JWT from SIWE login or a public API key beginning with sk_.",
			},
		},
		"schemas": schemas(),
	},
}

func op(summary string, secured bool, requestSchema string, responseSchema string) map[string]any {
	out := responseWithStatus(summary, "200", "OK", responseSchema)
	if !secured {
		out["security"] = []map[string]any{}
	}
	out["requestBody"] = map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{"schema": ref(requestSchema)},
		},
	}
	return out
}

func authOp(summary string, requestSchema string, responseSchema string) map[string]any {
	out := op(summary, false, requestSchema, responseSchema)
	responses := out["responses"].(map[string]any)
	responses["503"] = jsonResponse("SIWE auth is disabled or not configured", "#/components/schemas/ErrorResponse")
	return out
}

func responseOnly(summary string, responseSchema string) map[string]any {
	return responseWithStatus(summary, "200", "OK", responseSchema)
}

func publicResponseOnly(summary string, responseSchema string) map[string]any {
	out := responseOnly(summary, responseSchema)
	out["security"] = []map[string]any{}
	return out
}

func createdOp(summary string, requestSchema string, responseSchema string) map[string]any {
	out := responseWithStatus(summary, "201", "Created", responseSchema)
	out["requestBody"] = map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{"schema": ref(requestSchema)},
		},
	}
	return out
}

func reportViolationOp() map[string]any {
	out := createdOp("Report a violation; avoid goals create one violation per report, do goals dedupe by period", "#/components/schemas/ReportViolationRequest", "#/components/schemas/Violation")
	responses := out["responses"].(map[string]any)
	responses["502"] = jsonResponse("Charge failed after the violation row was recorded", "#/components/schemas/ErrorResponse")
	return out
}

func chatOp() map[string]any {
	out := op("Chat with the AI goal manager", true, "#/components/schemas/ChatRequest", "#/components/schemas/ChatResponse")
	responses := out["responses"].(map[string]any)
	responses["503"] = jsonResponse("AI manager is disabled or not configured", "#/components/schemas/ErrorResponse")
	return out
}

func audioChatOp() map[string]any {
	out := responseWithStatus("Transcribe an audio file and chat with the AI goal manager", "200", "OK", "#/components/schemas/AudioChatResponse")
	out["requestBody"] = map[string]any{
		"required": true,
		"content": map[string]any{
			"multipart/form-data": map[string]any{
				"schema": object(required("audio"),
					prop("audio", map[string]any{"type": "string", "format": "binary"}),
					prop("conversation_id", uuidType()),
				),
			},
		},
	}
	responses := out["responses"].(map[string]any)
	responses["503"] = jsonResponse("AI manager or audio transcription is disabled or not configured", "#/components/schemas/ErrorResponse")
	return out
}

func telegramAudioOp() map[string]any {
	out := responseWithStatus("Transcribe Telegram voice/audio and run the AI goal-manager flow for a linked chat", "200", "OK", "#/components/schemas/AudioChatResponse")
	out["requestBody"] = map[string]any{
		"required": true,
		"content": map[string]any{
			"multipart/form-data": map[string]any{
				"schema": object(required("chat_id", "audio"),
					prop("chat_id", integerType()),
					prop("chat_kind", enumType("private", "group", "supergroup", "channel")),
					prop("message_id", integerType()),
					prop("conversation_id", uuidType()),
					prop("audio", map[string]any{"type": "string", "format": "binary"}),
				),
			},
			"application/octet-stream": map[string]any{
				"schema": map[string]any{"type": "string", "format": "binary"},
			},
		},
	}
	responses := out["responses"].(map[string]any)
	responses["503"] = jsonResponse("AI manager or audio transcription is disabled or not configured", "#/components/schemas/ErrorResponse")
	return out
}

func agentSkillMarkdownOp() map[string]any {
	return map[string]any{
		"summary":  "Fetch a private generated Markdown skill document",
		"security": []map[string]any{},
		"parameters": []map[string]any{{
			"name":     "token",
			"in":       "path",
			"required": true,
			"schema":   stringType(),
		}},
		"responses": map[string]any{
			"200": map[string]any{
				"description": "Markdown skill document",
				"content": map[string]any{
					"text/markdown": map[string]any{"schema": stringType()},
				},
			},
			"404": errorResponse(),
			"410": errorResponse(),
			"500": errorResponse(),
		},
	}
}

func responseWithStatus(summary string, status string, description string, responseSchema string) map[string]any {
	return map[string]any{
		"summary": summary,
		"responses": map[string]any{
			status: jsonResponse(description, responseSchema),
			"400":  errorResponse(),
			"401":  errorResponse(),
			"500":  errorResponse(),
		},
	}
}

func noContent(summary string) map[string]any {
	return map[string]any{
		"summary": summary,
		"responses": map[string]any{
			"204": map[string]string{"description": "No content"},
			"401": errorResponse(),
			"404": errorResponse(),
			"500": errorResponse(),
		},
	}
}

func withGoalID(operation map[string]any) map[string]any {
	return withPathParam(operation, "goalID")
}

func withPathParam(operation map[string]any, name string) map[string]any {
	operation["parameters"] = append(parameters(operation), map[string]any{
		"name":     name,
		"in":       "path",
		"required": true,
		"schema":   map[string]string{"type": "string", "format": "uuid"},
	})
	if responses, ok := operation["responses"].(map[string]any); ok {
		if _, exists := responses["403"]; !exists {
			responses["403"] = errorResponse()
		}
		if _, exists := responses["404"]; !exists {
			responses["404"] = errorResponse()
		}
	}
	return operation
}

func withQueryParams(operation map[string]any, params []map[string]any) map[string]any {
	operation["parameters"] = append(parameters(operation), params...)
	return operation
}

func parameters(operation map[string]any) []map[string]any {
	raw, _ := operation["parameters"].([]map[string]any)
	return raw
}

func queryParam(name string, required bool) map[string]any {
	return map[string]any{
		"name":     name,
		"in":       "query",
		"required": required,
		"schema":   map[string]string{"type": "string"},
	}
}

func jsonResponse(description string, schema string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{"schema": ref(schema)},
		},
	}
}

func errorResponse() map[string]any {
	return jsonResponse("Error", "#/components/schemas/ErrorResponse")
}

func ref(name string) map[string]any {
	return map[string]any{"$ref": name}
}

func schemas() map[string]any {
	return map[string]any{
		"NonceRequest":              object(required("wallet_address"), prop("wallet_address", stringType())),
		"NonceResponse":             object(required("nonce"), prop("nonce", stringType())),
		"SIWERequest":               object(required("message", "signature"), prop("message", stringType()), prop("signature", stringType())),
		"SIWEResponse":              object(required("token", "user"), prop("token", stringType()), prop("user", ref("#/components/schemas/User"))),
		"MeResponse":                object(required("user_id"), prop("user_id", uuidType())),
		"CreateGoalRequest":         object(required("title", "type", "cadence", "stake_amount", "token_symbol", "chain"), prop("title", stringType()), prop("description", stringType()), prop("type", enumType("do", "avoid")), prop("cadence", enumType("daily", "weekly")), prop("stake_amount", stakeAmountType()), prop("token_symbol", enumType("USDC", "USDT")), prop("chain", stringType()), prop("timezone", timezoneType()), prop("starts_at", dateTimeType()), prop("ends_at", dateTimeType())),
		"UpdateGoalRequest":         object(required("title"), prop("title", stringType()), prop("description", stringType()), prop("stake_amount", stakeAmountType()), prop("ends_at", dateTimeType())),
		"SetStakeRequest":           object(required("stake_amount", "token_symbol", "chain"), prop("stake_amount", stakeAmountType()), prop("token_symbol", enumType("USDC", "USDT")), prop("chain", stringType())),
		"CheckInRequest":            object(nil, prop("period", stringType()), prop("note", stringType())),
		"ReportViolationRequest":    object(nil, prop("period", stringType()), prop("reason", stringType())),
		"RecordApprovalRequest":     object(required("chain", "token_symbol", "tx_hash"), prop("chain", stringType()), prop("token_symbol", enumType("USDC", "USDT")), prop("tx_hash", stringType()), prop("dry_run_allowance", stringType())),
		"CreateApiKeyRequest":       object(required("name"), prop("name", stringType())),
		"CreateAgentLinkRequest":    object(nil, prop("name", stringType())),
		"CreatedAgentLink":          object(required("skill_url", "agent_link"), prop("skill_url", stringType()), prop("agent_link", ref("#/components/schemas/AgentLinkMetadata"))),
		"AgentLinkMetadata":         object(required("id", "api_key_id", "name", "expires_at", "created_at"), prop("id", uuidType()), prop("api_key_id", uuidType()), prop("name", stringType()), prop("expires_at", dateTimeType()), prop("created_at", dateTimeType()), prop("revoked_at", dateTimeType())),
		"EmptyRequest":              object(nil),
		"CreatedTelegramLinkCode":   object(required("code", "expires_at"), prop("code", stringType()), prop("expires_at", dateTimeType())),
		"TelegramLinkRequest":       object(required("code", "chat_id", "chat_kind"), prop("code", stringType()), prop("chat_id", integerType()), prop("chat_kind", enumType("private", "group", "supergroup", "channel"))),
		"TelegramLinkResponse":      object(required("reply", "telegram_link"), prop("reply", stringType()), prop("telegram_link", ref("#/components/schemas/TelegramLink"))),
		"TelegramMessageRequest":    object(required("chat_id", "text"), prop("chat_id", integerType()), prop("chat_kind", enumType("private", "group", "supergroup", "channel")), prop("message_id", integerType()), prop("text", stringType())),
		"TelegramMessageResponse":   object(required("reply"), prop("reply", stringType())),
		"TelegramAgentLinkRequest":  object(required("chat_id"), prop("chat_id", integerType()), prop("chat_kind", enumType("private", "group", "supergroup", "channel")), prop("name", stringType())),
		"TelegramAgentLinkResponse": object(required("reply", "skill_url", "agent_link"), prop("reply", stringType()), prop("skill_url", stringType()), prop("agent_link", ref("#/components/schemas/AgentLinkMetadata"))),
		"ChatRequest":               object(required("message"), prop("message", stringType()), prop("conversation_id", uuidType())),
		"ChatResponse":              object(required("conversation_id", "reply"), prop("conversation_id", uuidType()), prop("reply", stringType())),
		"AudioChatResponse":         object(required("transcript", "conversation_id", "reply"), prop("transcript", stringType()), prop("conversation_id", uuidType()), prop("reply", stringType())),
		"ErrorResponse":             object(required("error"), prop("error", stringType())),
		"CreatedApiKey":             object(required("key", "api_key"), prop("key", stringType()), prop("api_key", ref("#/components/schemas/ApiKey"))),
		"ApprovalStatus":            object(required("chain", "token_symbol", "allowance", "approved"), prop("chain", stringType()), prop("token_symbol", enumType("USDC", "USDT")), prop("allowance", stringType()), prop("approved", boolType())),
		"ChainInfo":                 object(required("key", "stake_enforcer_address", "tokens"), prop("key", stringType()), prop("stake_enforcer_address", stringType()), prop("tokens", stringMapType())),
		"Progress":                  object(required("goal", "current_period", "current_period_completed", "violations"), prop("goal", ref("#/components/schemas/Goal")), prop("current_period", stringType()), prop("current_period_check_in", ref("#/components/schemas/CheckIn")), prop("current_period_completed", boolType()), prop("violations", arrayOf("#/components/schemas/Violation"))),
		"User":                      object(required("id", "wallet_address", "created_at"), prop("id", uuidType()), prop("wallet_address", stringType()), prop("timezone", timezoneType()), prop("created_at", dateTimeType())),
		"Goal":                      object(required("id", "user_id", "title", "type", "cadence", "stake_amount", "token_symbol", "chain", "archived", "created_at", "starts_at"), prop("id", uuidType()), prop("user_id", uuidType()), prop("title", stringType()), prop("description", stringType()), prop("type", enumType("do", "avoid")), prop("cadence", enumType("daily", "weekly", "custom")), prop("stake_amount", stringType()), prop("token_symbol", enumType("USDC", "USDT")), prop("chain", stringType()), prop("timezone", timezoneType()), prop("archived", boolType()), prop("created_at", dateTimeType()), prop("starts_at", dateTimeType()), prop("ends_at", dateTimeType())),
		"CheckIn":                   object(required("id", "goal_id", "period", "created_at"), prop("id", uuidType()), prop("goal_id", uuidType()), prop("period", stringType()), prop("note", stringType()), prop("created_at", dateTimeType())),
		"Violation":                 object(required("id", "goal_id", "period", "status", "amount", "created_at", "updated_at"), prop("id", uuidType()), prop("goal_id", uuidType()), prop("period", stringType()), prop("status", enumType("pending", "charged", "failed")), prop("amount", stringType()), prop("reason", stringType()), prop("tx_hash", stringType()), prop("created_at", dateTimeType()), prop("updated_at", dateTimeType())),
		"ApiKey":                    object(required("id", "user_id", "name", "prefix", "created_at"), prop("id", uuidType()), prop("user_id", uuidType()), prop("name", stringType()), prop("prefix", stringType()), prop("created_at", dateTimeType()), prop("last_used", dateTimeType()), prop("revoked_at", dateTimeType())),
		"TelegramLink":              object(required("id", "user_id", "chat_id", "chat_kind", "created_at", "updated_at"), prop("id", uuidType()), prop("user_id", uuidType()), prop("chat_id", integerType()), prop("chat_kind", enumType("private", "group", "supergroup", "channel")), prop("created_at", dateTimeType()), prop("updated_at", dateTimeType())),
		"GoalList":                  arrayOf("#/components/schemas/Goal"),
		"ChainInfoList":             arrayOf("#/components/schemas/ChainInfo"),
		"ViolationList":             arrayOf("#/components/schemas/Violation"),
		"ApiKeyList":                arrayOf("#/components/schemas/ApiKey"),
		"AgentLinkList":             arrayOf("#/components/schemas/AgentLinkMetadata"),
	}
}

func object(required []string, properties ...map[string]any) map[string]any {
	props := make(map[string]any, len(properties))
	for _, item := range properties {
		for name, schema := range item {
			props[name] = schema
		}
	}
	out := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func required(names ...string) []string {
	return names
}

func prop(name string, schema map[string]any) map[string]any {
	return map[string]any{name: schema}
}

func stringType() map[string]any {
	return map[string]any{"type": "string"}
}

func stakeAmountType() map[string]any {
	return map[string]any{"type": "string", "description": stakeAmountDescription}
}

func timezoneType() map[string]any {
	return map[string]any{"type": "string", "description": timezoneDescription}
}

func stringMapType() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": stringType()}
}

func uuidType() map[string]any {
	return map[string]any{"type": "string", "format": "uuid"}
}

func dateTimeType() map[string]any {
	return map[string]any{"type": "string", "format": "date-time", "nullable": true}
}

func boolType() map[string]any {
	return map[string]any{"type": "boolean"}
}

func integerType() map[string]any {
	return map[string]any{"type": "integer", "format": "int64"}
}

func enumType(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func arrayOf(schema string) map[string]any {
	return map[string]any{"type": "array", "items": ref(schema)}
}

func (h handler) openapi(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openAPISpec)
}

func (h handler) docs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Goal Stakes API Docs</title>
  <style>
    body{font-family:Inter,system-ui,sans-serif;margin:0;color:#18212f;background:#f7f9fa}
    main{max-width:920px;margin:0 auto;padding:32px 20px}
    h1{font-size:30px;margin:0 0 10px}
    h2{font-size:18px;margin:28px 0 10px}
    p{color:#52606d;line-height:1.5}
    code{background:#eef3f6;border:1px solid #d8e1e8;border-radius:6px;padding:2px 6px}
    a{color:#006d77;font-weight:650}
    li{margin:8px 0}
  </style>
</head>
<body>
<main>
  <h1>Goal Stakes API</h1>
  <p>Authenticate with <code>Authorization: Bearer &lt;session JWT or sk_ API key&gt;</code>. The full OpenAPI 3.0 document is available at <a href="/openapi.json">/openapi.json</a>.</p>
  <h2>Auth</h2>
  <ul>
    <li><code>GET /api/v1/chains</code> returns public chain, token, and StakeEnforcer addresses for wallet approvals.</li>
    <li><code>POST /api/v1/auth/nonce</code> issues a SIWE nonce that expires after 10 minutes.</li>
    <li><code>POST /api/v1/auth/siwe</code> verify a wallet signature and receive a session JWT.</li>
  </ul>
  <h2>Goals</h2>
  <ul>
    <li><code>GET/POST /api/v1/goals</code> list and create daily or weekly staked goals.</li>
    <li><code>timezone</code> is optional on goal creation; use an IANA name like <code>America/New_York</code>. Blank means UTC.</li>
    <li><code>starts_at</code> and <code>ends_at</code> are optional RFC3339 timestamps for scheduling a goal window.</li>
    <li><code>stake_amount</code> uses 6-decimal token base units: <code>$100</code> USDC/USDT is <code>100000000</code>.</li>
    <li>Creating a goal or raising its stake requires an approval allowance on the same chain/token that is at least the requested <code>stake_amount</code>.</li>
    <li><code>PATCH /api/v1/goals/{goalID}</code> updates title, description, or end date; omit <code>stake_amount</code> to keep the current stake; send <code>ends_at</code> as <code>null</code> to clear the end date.</li>
    <li><code>PATCH /api/v1/goals/{goalID}/stake</code> changes token, chain, or amount.</li>
    <li><code>DELETE /api/v1/goals/{goalID}</code> archives a goal.</li>
    <li><code>POST /api/v1/goals/{goalID}/checkins</code> record progress.</li>
    <li><code>POST /api/v1/goals/{goalID}/violations</code> self-report a violation.</li>
  </ul>
  <h2>Account</h2>
  <ul>
    <li><code>GET /api/v1/approvals</code> reads backend-observed wallet token approval status.</li>
    <li><code>POST /api/v1/approvals</code> accepts <code>tx_hash</code>, <code>chain</code>, and <code>token_symbol</code>; with live web3 enabled the backend reads allowance from RPC before caching it. Local dry-run without an allowance checker may include <code>dry_run_allowance</code>; production must not depend on client-provided allowance.</li>
    <li><code>GET/POST /api/v1/apikeys</code> list and create API keys.</li>
    <li><code>GET/POST /api/v1/agent-links</code> list active own-agent links or create a private <code>.md</code> skill URL.</li>
    <li><code>GET /agent-skills/{token}.md</code> returns the generated private skill document containing the agent API secret and daily cron instructions.</li>
    <li><code>POST /api/v1/telegram/link-codes</code> creates a short one-time code for linking Telegram without posting raw <code>sk_</code> keys.</li>
    <li><code>POST /api/v1/chat</code> send text to the GPT-backed goal manager; include <code>conversation_id</code> for follow-up turns.</li>
    <li><code>POST /api/v1/chat/audio</code> sends multipart <code>audio</code> for backend transcription, then runs the same goal-manager chat flow.</li>
  </ul>
  <h2>Internal Telegram</h2>
  <ul>
    <li><code>POST /internal/telegram/link</code> is for the Telegram bot only. Authenticate with the backend-issued bot bearer secret.</li>
    <li><code>POST /internal/telegram/message</code> is for linked private chats, groups, and channel posts. It supports <code>/goals</code>, <code>/create</code>, <code>/done</code>, <code>/violate</code>, <code>/progress</code>, <code>/archive</code>, and normal AI text.</li>
    <li><code>POST /internal/telegram/audio</code> accepts Telegram voice/audio multipart uploads, transcribes them on the backend, and runs the same AI tool flow.</li>
    <li><code>POST /internal/telegram/agent-link</code> creates a private own-agent skill link for a linked Telegram chat.</li>
  </ul>
  <h2>Errors</h2>
  <ul>
    <li>Error responses use JSON: <code>{"error":"approval allowance ... "}</code>.</li>
    <li>Unknown routes return <code>404</code>; unsupported methods return <code>405</code>.</li>
    <li><code>POST /api/v1/auth/nonce</code> and <code>POST /api/v1/auth/siwe</code> can return <code>503</code> when SIWE auth is not configured.</li>
    <li><code>POST /api/v1/goals/{goalID}/violations</code> can return <code>502</code> when the violation row is recorded but the on-chain charge failed.</li>
    <li><code>POST /api/v1/chat</code> can return <code>503</code> when <code>OPENAI_API_KEY</code> is not configured.</li>
  </ul>
</main>
</body>
</html>`))
}
