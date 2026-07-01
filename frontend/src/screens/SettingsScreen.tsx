import { FormEvent, useEffect, useState } from "react";
import { AlertTriangle, Bot, CheckCircle2, Copy, ExternalLink, KeyRound, MessageCircle, Trash2 } from "lucide-react";
import type { ApiClient } from "../lib/api";
import { baseURL } from "../lib/api";
import type { AgentLink, ApiKey, TelegramLinkCode } from "../types";
import { ApprovalPanel } from "./ApprovalGate";

function formatLastUsed(value?: string) {
  if (!value) return "Unused";
  return `Last used ${new Date(value).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })}`;
}

export function SettingsScreen({ api }: { api: ApiClient }) {
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [name, setName] = useState("automation");
  const [rawKey, setRawKey] = useState("");
  const [agentLinks, setAgentLinks] = useState<AgentLink[]>([]);
  const [agentName, setAgentName] = useState("codex");
  const [agentURL, setAgentURL] = useState("");
  const [telegramCode, setTelegramCode] = useState<TelegramLinkCode | null>(null);
  const [status, setStatus] = useState("");

  async function loadKeys() {
    setKeys(await api.listApiKeys());
  }

  async function loadAgentLinks() {
    setAgentLinks(await api.listAgentLinks());
  }

  useEffect(() => {
    void Promise.all([loadKeys(), loadAgentLinks()]).catch((error) => setStatus(error instanceof Error ? error.message : "Could not load settings"));
  }, []);

  async function createKey(event: FormEvent) {
    event.preventDefault();
    setStatus("");
    try {
      const created = await api.createApiKey(name);
      setRawKey(created.key);
      await loadKeys();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Create key failed");
    }
  }

  async function revoke(id: string) {
    setStatus("");
    try {
      await api.revokeApiKey(id);
      await loadKeys();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Revoke failed");
    }
  }

  async function createAgentLink() {
    setStatus("");
    try {
      const created = await api.createAgentLink(agentName);
      setAgentURL(created.skill_url);
      await loadAgentLinks();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Agent link failed");
    }
  }

  async function revokeAgentLink(id: string) {
    setStatus("");
    try {
      await api.revokeAgentLink(id);
      if (agentLinks.some((link) => link.id === id)) setAgentURL("");
      await loadAgentLinks();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Revoke agent link failed");
    }
  }

  async function createTelegramCode() {
    setStatus("");
    try {
      setTelegramCode(await api.createTelegramLinkCode());
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Telegram link code failed");
    }
  }

  return (
    <section className="settings-layout">
      <ApprovalPanel api={api} />

      <section className="settings-panel">
        <div className="section-heading">
          <KeyRound size={22} />
          <h2>API Keys</h2>
        </div>
        <form className="api-key-form" onSubmit={createKey}>
          <input value={name} onChange={(event) => setName(event.target.value)} placeholder="Key name" />
          <button className="primary-button">
            <KeyRound size={18} />
            Create
          </button>
        </form>
        {rawKey ? (
          <div className="raw-key api-key-created">
            <code>{rawKey}</code>
            <button className="icon-button" onClick={() => navigator.clipboard.writeText(rawKey)} title="Copy API key">
              <Copy size={18} />
            </button>
          </div>
        ) : null}
        <div className="key-list">
          {keys.map((key) => (
            <div className="key-row" key={key.id}>
              <div className="key-main">
                <span className="key-name">{key.name || "API key"}</span>
                <span className="key-meta">{formatLastUsed(key.last_used)}</span>
              </div>
              <code className="key-prefix">{key.prefix}</code>
              <button className="icon-button danger" onClick={() => revoke(key.id)} title="Revoke API key">
                <Trash2 size={18} />
              </button>
            </div>
          ))}
        </div>
      </section>

      <section className="settings-panel">
        <div className="section-heading">
          <Bot size={22} />
          <h2>Own agent</h2>
        </div>
        <p className="settings-copy">Generate a private Markdown skill link for your own agent. The link contains a generated API secret and daily reminder cron instructions.</p>
        <div className="api-key-form">
          <input value={agentName} onChange={(event) => setAgentName(event.target.value)} placeholder="Agent name" />
          <button className="primary-button" onClick={createAgentLink}>
            <Bot size={18} />
            Connect own agent
          </button>
        </div>
        {agentURL ? (
          <div className="raw-key agent-skill-link">
            <code>{agentURL}</code>
            <button className="icon-button" onClick={() => navigator.clipboard.writeText(agentURL)} title="Copy own-agent skill link">
              <Copy size={18} />
            </button>
          </div>
        ) : null}
        <div className="key-list">
          {agentLinks.map((link) => (
            <div className="key-row" key={link.id}>
              <div className="key-main">
                <span className="key-name">{link.name || "Own agent"}</span>
                <span className="key-meta">Expires {new Date(link.expires_at).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })}</span>
              </div>
              <button className="icon-button danger" onClick={() => revokeAgentLink(link.id)} title="Revoke own-agent link">
                <Trash2 size={18} />
              </button>
            </div>
          ))}
        </div>
      </section>

      <section className="settings-panel">
        <div className="section-heading">
          <MessageCircle size={22} />
          <h2>Telegram</h2>
        </div>
        <p className="settings-copy">Create a one-time code, then send <code>/link code</code> to the bot in a private chat, group, or channel. Do not paste raw <code>sk_</code> API keys into Telegram.</p>
        <button className="primary-button" onClick={createTelegramCode}>
          <MessageCircle size={18} />
          Create link code
        </button>
        {telegramCode ? (
          <div className="raw-key telegram-link-code">
            <code>{telegramCode.code}</code>
            <button className="icon-button" onClick={() => navigator.clipboard.writeText(telegramCode.code)} title="Copy Telegram link code">
              <Copy size={18} />
            </button>
            <span className="key-meta">Expires {new Date(telegramCode.expires_at).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })}</span>
          </div>
        ) : null}
      </section>

      <a className="docs-link" href={`${baseURL}/docs`} target="_blank" rel="noreferrer">
        <ExternalLink size={18} />
        API docs
      </a>

      {status ? <div className={status.startsWith("Approval submitted") ? "notice success" : "notice error"}>{status.startsWith("Approval submitted") ? <CheckCircle2 size={18} /> : <AlertTriangle size={18} />}{status}</div> : null}
    </section>
  );
}
