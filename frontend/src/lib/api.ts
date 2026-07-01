import type { AgentLink, ApiKey, ApprovalStatus, AudioChatResult, ChainInfo, ChatResult, CheckIn, CreatedAgentLink, CreatedApiKey, Goal, Progress, TelegramLinkCode, Violation } from "../types";

const baseURL = import.meta.env.VITE_API_BASE_URL ?? "http://127.0.0.1:8080";

export class ApiClient {
  constructor(private token: string | null) {}

  setToken(token: string | null) {
    this.token = token;
  }

  async issueNonce(walletAddress: string) {
    return this.request<{ nonce: string }>("/api/v1/auth/nonce", {
      method: "POST",
      body: { wallet_address: walletAddress },
      auth: false,
    });
  }

  async verifySIWE(message: string, signature: string) {
    return this.request<{ token: string; user: { id: string; wallet_address: string } }>("/api/v1/auth/siwe", {
      method: "POST",
      body: { message, signature },
      auth: false,
    });
  }

  listChains() {
    return this.request<ChainInfo[]>("/api/v1/chains", { auth: false });
  }

  listGoals() {
    return this.request<Goal[]>("/api/v1/goals");
  }

  createGoal(body: Partial<Goal>) {
    return this.request<Goal>("/api/v1/goals", { method: "POST", body });
  }

  updateGoal(goalID: string, body: { title: string; description?: string; stake_amount?: string; ends_at?: string | null }) {
    return this.request<Goal>(`/api/v1/goals/${goalID}`, { method: "PATCH", body });
  }

  updateStake(goalID: string, body: { stake_amount: string; token_symbol: string; chain: string }) {
    return this.request<Goal>(`/api/v1/goals/${goalID}/stake`, { method: "PATCH", body });
  }

  archiveGoal(goalID: string) {
    return this.request<void>(`/api/v1/goals/${goalID}`, { method: "DELETE" });
  }

  logCheckIn(goalID: string, note: string) {
    return this.request<CheckIn>(`/api/v1/goals/${goalID}/checkins`, { method: "POST", body: { note } });
  }

  reportViolation(goalID: string, reason: string) {
    return this.request<Violation>(`/api/v1/goals/${goalID}/violations`, { method: "POST", body: { reason } });
  }

  getProgress(goalID: string) {
    return this.request<Progress>(`/api/v1/goals/${goalID}/progress`);
  }

  chat(message: string, conversationID?: string) {
    return this.request<ChatResult>("/api/v1/chat", {
      method: "POST",
      body: conversationID ? { message, conversation_id: conversationID } : { message },
    });
  }

  async chatAudio(audio: Blob, filename: string, conversationID?: string) {
    const body = new FormData();
    body.append("audio", audio, filename);
    if (conversationID) body.append("conversation_id", conversationID);
    const headers: Record<string, string> = {};
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    const response = await fetch(`${baseURL}/api/v1/chat/audio`, {
      method: "POST",
      headers,
      body,
    });
    if (!response.ok) {
      throw new Error(await errorMessage(response));
    }
    return (await response.json()) as AudioChatResult;
  }

  listApiKeys() {
    return this.request<ApiKey[]>("/api/v1/apikeys");
  }

  createApiKey(name: string) {
    return this.request<CreatedApiKey>("/api/v1/apikeys", { method: "POST", body: { name } });
  }

  revokeApiKey(id: string) {
    return this.request<void>(`/api/v1/apikeys/${id}`, { method: "DELETE" });
  }

  listAgentLinks() {
    return this.request<AgentLink[]>("/api/v1/agent-links");
  }

  createAgentLink(name: string) {
    return this.request<CreatedAgentLink>("/api/v1/agent-links", { method: "POST", body: { name } });
  }

  revokeAgentLink(id: string) {
    return this.request<void>(`/api/v1/agent-links/${id}`, { method: "DELETE" });
  }

  createTelegramLinkCode() {
    return this.request<TelegramLinkCode>("/api/v1/telegram/link-codes", { method: "POST", body: {} });
  }

  getApproval(chain: string, tokenSymbol: string) {
    return this.request<ApprovalStatus>(`/api/v1/approvals?chain=${encodeURIComponent(chain)}&token_symbol=${tokenSymbol}`);
  }

  recordApproval(body: { chain: string; token_symbol: string; tx_hash: string; dry_run_allowance?: string }) {
    return this.request<ApprovalStatus>("/api/v1/approvals", { method: "POST", body });
  }

  private async request<T>(path: string, options: { method?: string; body?: unknown; auth?: boolean } = {}): Promise<T> {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (options.auth !== false && this.token) headers.Authorization = `Bearer ${this.token}`;
    const response = await fetch(`${baseURL}${path}`, {
      method: options.method ?? "GET",
      headers,
      body: options.body ? JSON.stringify(options.body) : undefined,
    });
    if (!response.ok) {
      throw new Error(await errorMessage(response));
    }
    if (response.status === 204) return undefined as T;
    return (await response.json()) as T;
  }
}

async function errorMessage(response: Response) {
  const contentType = response.headers.get("Content-Type") ?? "";
  if (contentType.includes("application/json")) {
    try {
      const body = (await response.json()) as { error?: unknown };
      if (typeof body.error === "string" && body.error.trim()) return body.error;
    } catch {
      return response.statusText || `HTTP ${response.status}`;
    }
  }
  const text = await response.text();
  return text.trim() || response.statusText || `HTTP ${response.status}`;
}

export { baseURL };
