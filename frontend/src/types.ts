export type GoalType = "do" | "avoid";
export type Cadence = "daily" | "weekly" | "custom";
export type TokenSymbol = "USDC" | "USDT";

export interface ChainInfo {
  key: string;
  stake_enforcer_address: string;
  tokens: Partial<Record<TokenSymbol, string>>;
}

export interface Goal {
  id: string;
  user_id: string;
  title: string;
  description?: string;
  type: GoalType;
  cadence: Cadence;
  stake_amount: string;
  token_symbol: TokenSymbol;
  chain: string;
  timezone?: string;
  archived: boolean;
  created_at: string;
  starts_at: string;
  ends_at?: string;
}

export interface CheckIn {
  id: string;
  goal_id: string;
  period: string;
  note?: string;
  created_at: string;
}

export interface Violation {
  id: string;
  goal_id: string;
  period: string;
  status: "pending" | "charged" | "failed";
  amount: string;
  reason?: string;
  tx_hash?: string;
}

export interface Progress {
  goal: Goal;
  current_period: string;
  current_period_check_in?: CheckIn;
  current_period_completed: boolean;
  violations: Violation[];
}

export interface ApiKey {
  id: string;
  name: string;
  prefix: string;
  created_at: string;
  last_used?: string;
  revoked_at?: string;
}

export interface CreatedApiKey {
  key: string;
  api_key: ApiKey;
}

export interface AgentLink {
  id: string;
  api_key_id: string;
  name: string;
  expires_at: string;
  created_at: string;
  revoked_at?: string;
}

export interface CreatedAgentLink {
  skill_url: string;
  agent_link: AgentLink;
}

export interface TelegramLinkCode {
  code: string;
  expires_at: string;
}

export interface ChatResult {
  conversation_id: string;
  reply: string;
}

export interface AudioChatResult extends ChatResult {
  transcript: string;
}

export interface ApprovalStatus {
  chain: string;
  token_symbol: TokenSymbol;
  allowance: string;
  approved: boolean;
}
