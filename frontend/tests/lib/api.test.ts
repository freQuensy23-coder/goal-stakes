import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "../../src/lib/api";

describe("ApiClient", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uses structured API error messages", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify({ error: "approval allowance is below stake amount" }), { status: 400, headers: { "Content-Type": "application/json" } })),
    );

    const api = new ApiClient("token");

    await expect(api.listGoals()).rejects.toThrow(/^approval allowance is below stake amount$/);
  });

  it("posts audio chat as multipart form data", async () => {
    const fetchMock = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      const headers = init?.headers as Record<string, string>;
      expect(headers.Authorization).toBe("Bearer token");
      expect(headers["Content-Type"]).toBeUndefined();
      const body = init?.body as FormData;
      expect(body.get("conversation_id")).toBe("conversation-1");
      const audio = body.get("audio") as File;
      expect(audio.name).toBe("voice.ogg");
      expect(audio.type).toBe("audio/ogg");
      return new Response(JSON.stringify({ transcript: "I did 10 push-ups", conversation_id: "conversation-1", reply: "Recorded." }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const api = new ApiClient("token");
    const result = await api.chatAudio(new Blob(["audio"], { type: "audio/ogg" }), "voice.ogg", "conversation-1");

    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/api/v1/chat/audio"), expect.objectContaining({ method: "POST" }));
    expect(result).toEqual({ transcript: "I did 10 push-ups", conversation_id: "conversation-1", reply: "Recorded." });
  });

  it("creates Telegram link codes", async () => {
    const fetchMock = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.method).toBe("POST");
      expect(init?.body).toBe("{}");
      return new Response(JSON.stringify({ code: "ABCD1234", expires_at: "2026-07-01T00:10:00Z" }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const api = new ApiClient("token");
    const result = await api.createTelegramLinkCode();

    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/api/v1/telegram/link-codes"), expect.anything());
    expect(result.code).toBe("ABCD1234");
  });

  it("records approvals with tx_hash and local dry-run allowance, not legacy allowance", async () => {
    const fetchMock = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.method).toBe("POST");
      expect(init?.body).toBe(JSON.stringify({ chain: "ethereum", token_symbol: "USDC", tx_hash: "0xapprove", dry_run_allowance: "100000000" }));
      expect(String(init?.body)).not.toContain('"allowance"');
      return new Response(JSON.stringify({ chain: "ethereum", token_symbol: "USDC", allowance: "100000000", approved: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const api = new ApiClient("token");
    const result = await api.recordApproval({ chain: "ethereum", token_symbol: "USDC", tx_hash: "0xapprove", dry_run_allowance: "100000000" });

    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/api/v1/approvals"), expect.anything());
    expect(result.approved).toBe(true);
  });

  it("manages own-agent links", async () => {
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      const path = String(url);
      if (path.includes("/api/v1/agent-links") && init?.method === "POST") {
        expect(init.body).toBe(JSON.stringify({ name: "codex" }));
        return new Response(
          JSON.stringify({
            skill_url: "https://api.goalstakes.test/agent-skills/agt_private.md",
            agent_link: { id: "link-1", api_key_id: "key-1", name: "codex", expires_at: "2026-09-29T00:00:00Z", created_at: "2026-07-01T00:00:00Z" },
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        );
      }
      if (path.endsWith("/api/v1/agent-links") && init?.method === "GET") {
        return new Response(JSON.stringify([{ id: "link-1", api_key_id: "key-1", name: "codex", expires_at: "2026-09-29T00:00:00Z", created_at: "2026-07-01T00:00:00Z" }]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (path.endsWith("/api/v1/agent-links/link-1") && init?.method === "DELETE") {
        return new Response(null, { status: 204 });
      }
      throw new Error(`unexpected request ${path}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const api = new ApiClient("token");
    const created = await api.createAgentLink("codex");
    const links = await api.listAgentLinks();
    await api.revokeAgentLink("link-1");

    expect(created.skill_url).toContain("/agent-skills/agt_private.md");
    expect(links).toHaveLength(1);
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/api/v1/agent-links/link-1"), expect.objectContaining({ method: "DELETE" }));
  });
});
