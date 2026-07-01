package ai

import (
	"encoding/json"

	openai "github.com/sashabaranov/go-openai"
)

const stakeAmountDescription = "Amount in smallest token units with 6 decimals for USDC/USDT. Convert human dollar amounts before calling: $100 -> 100000000, $2.50 -> 2500000. The user's approval allowance on the same chain/token must cover this amount."
const timezoneDescription = "Optional IANA timezone for daily/weekly period boundaries, e.g. America/New_York. Defaults to UTC."
const scheduleDescription = "Optional RFC3339 timestamp for a goal scheduling window."
const nullableEndDateDescription = "Optional RFC3339 timestamp for a goal end date. Send null to clear an existing end date."

func tools() []openai.Tool {
	return []openai.Tool{
		functionTool("create_goal", "Create a daily or weekly goal with stake settings.", schema(map[string]any{
			"title":        stringSchema(),
			"description":  stringSchema(),
			"type":         enumSchema("do", "avoid"),
			"cadence":      enumSchema("daily", "weekly"),
			"stake_amount": stakeAmountSchema(),
			"token_symbol": enumSchema("USDC", "USDT"),
			"chain":        stringSchema(),
			"timezone":     timezoneSchema(),
			"starts_at":    dateTimeSchema(),
			"ends_at":      dateTimeSchema(),
		}, []string{"title", "type", "cadence", "stake_amount", "token_symbol", "chain"})),
		functionTool("list_goals", "List the user's goals.", schema(map[string]any{}, nil)),
		functionTool("list_chains", "List configured chain keys, token contract addresses, and StakeEnforcer addresses available for stakes and approvals.", schema(map[string]any{}, nil)),
		functionTool("update_goal", "Update a goal's title, description, stake amount, or end date.", schema(map[string]any{
			"goal_id":      stringSchema(),
			"title":        stringSchema(),
			"description":  stringSchema(),
			"stake_amount": stakeAmountSchema(),
			"ends_at":      nullableDateTimeSchema(),
		}, []string{"goal_id", "title"})),
		functionTool("archive_goal", "Archive a goal so it no longer appears in active goal lists.", schema(map[string]any{
			"goal_id": stringSchema(),
		}, []string{"goal_id"})),
		functionTool("log_check_in", "Record progress for a goal.", schema(map[string]any{
			"goal_id": stringSchema(),
			"period":  stringSchema(),
			"note":    stringSchema(),
		}, []string{"goal_id"})),
		functionTool("report_violation", "Report a broken or missed goal. Avoid goals create one violation per report; do goals dedupe by period.", schema(map[string]any{
			"goal_id": stringSchema(),
			"period":  stringSchema(),
			"reason":  stringSchema(),
		}, []string{"goal_id"})),
		functionTool("get_progress", "Get progress and violation history for a goal.", schema(map[string]any{
			"goal_id": stringSchema(),
		}, []string{"goal_id"})),
		functionTool("set_stake", "Update a goal's stake amount and token.", schema(map[string]any{
			"goal_id":      stringSchema(),
			"stake_amount": stakeAmountSchema(),
			"token_symbol": enumSchema("USDC", "USDT"),
			"chain":        stringSchema(),
		}, []string{"goal_id", "stake_amount", "token_symbol", "chain"})),
		functionTool("get_approval_status", "Get the user's cached token approval status.", schema(map[string]any{
			"chain":        stringSchema(),
			"token_symbol": enumSchema("USDC", "USDT"),
		}, []string{"chain", "token_symbol"})),
	}
}

func functionTool(name, description string, parameters json.RawMessage) openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}
}

func schema(properties map[string]any, required []string) json.RawMessage {
	if required == nil {
		required = []string{}
	}
	raw, _ := json.Marshal(map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	})
	return raw
}

func stringSchema() map[string]string {
	return map[string]string{"type": "string"}
}

func stakeAmountSchema() map[string]string {
	return map[string]string{"type": "string", "description": stakeAmountDescription}
}

func timezoneSchema() map[string]string {
	return map[string]string{"type": "string", "description": timezoneDescription}
}

func dateTimeSchema() map[string]string {
	return map[string]string{"type": "string", "format": "date-time", "description": scheduleDescription}
}

func nullableDateTimeSchema() map[string]any {
	return map[string]any{
		"anyOf": []map[string]string{
			{"type": "string", "format": "date-time"},
			{"type": "null"},
		},
		"description": nullableEndDateDescription,
	}
}

func enumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}
