import { FormEvent, useState } from "react";
import { Bot, Mic, Send, UserRound } from "lucide-react";
import type { ApiClient } from "../lib/api";
import { startVoiceTranscript } from "../lib/speech";

interface Message {
  role: "user" | "assistant";
  content: string;
}

export function ChatScreen({ api }: { api: ApiClient }) {
  const [input, setInput] = useState("");
  const [conversationID, setConversationID] = useState(() => localStorage.getItem("goalstakes.conversation") ?? "");
  const [messages, setMessages] = useState<Message[]>([{ role: "assistant", content: "Ready." }]);
  const [busy, setBusy] = useState(false);
  const [listening, setListening] = useState(false);

  async function sendText(raw: string) {
    const text = raw.trim();
    if (!text || busy) return;
    setInput("");
    setBusy(true);
    setMessages((items) => [...items, { role: "user", content: text }]);
    try {
      const result = await api.chat(text, conversationID || undefined);
      setConversationID(result.conversation_id);
      localStorage.setItem("goalstakes.conversation", result.conversation_id);
      setMessages((items) => [...items, { role: "assistant", content: result.reply }]);
    } catch (error) {
      setMessages((items) => [...items, { role: "assistant", content: error instanceof Error ? error.message : "Chat failed" }]);
    } finally {
      setBusy(false);
    }
  }

  async function send(event: FormEvent) {
    event.preventDefault();
    await sendText(input);
  }

  function listen() {
    if (busy || listening) return;
    setListening(true);
    startVoiceTranscript(
      (text) => {
        setInput(text);
        void sendText(text);
      },
      () => setListening(false),
      (message) => setMessages((items) => [...items, { role: "assistant", content: message }]),
    );
  }

  return (
    <section className="chat-layout">
      <div className="message-list">
        {messages.map((message, index) => (
          <div className={`message-row ${message.role}`} key={`${message.role}-${index}`}>
            <span className="avatar">{message.role === "assistant" ? <Bot size={18} /> : <UserRound size={18} />}</span>
            <p>{message.content}</p>
          </div>
        ))}
      </div>
      <form className="chat-composer" onSubmit={send}>
        <input value={input} onChange={(event) => setInput(event.target.value)} placeholder="Message" />
        <button type="button" className="icon-button" disabled={busy || listening} onClick={listen} title={listening ? "Listening" : "Voice input"}>
          <Mic size={18} />
        </button>
        <button className="primary-button" disabled={busy || !input.trim()} title="Send">
          <Send size={18} />
          Send
        </button>
      </form>
    </section>
  );
}
