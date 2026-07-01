import { Bot, BookOpen, CheckCircle2, CircleDollarSign, ListChecks, LockKeyhole, ShieldCheck, Wallet, Zap } from "lucide-react";
import { baseURL } from "../lib/api";

const docsURL = `${baseURL}/docs`;

export function LandingScreen({ onConnect, status }: { onConnect: () => void; status: string }) {
  return (
    <main className="landing-page">
      <header className="landing-nav">
        <div className="brand">
          <ShieldCheck size={24} />
          <span>Goal Stakes</span>
        </div>
        <a href={docsURL}>API Docs</a>
      </header>

      <section className="landing-hero">
        <div className="landing-hero-copy">
          <p className="eyebrow">Wallet-backed accountability</p>
          <h1>Goal Stakes</h1>
          <p className="landing-copy">
            Set daily or weekly goals, attach USDC or USDT stakes, approve the burn-only StakeEnforcer in MetaMask, and manage progress from the web app, public API, Android client, or GPT goal manager.
          </p>
          <div className="landing-actions">
            <button className="primary-button" onClick={onConnect}>
              <Wallet size={18} />
              Connect MetaMask
            </button>
            <a className="secondary-button" href={docsURL}>
              <BookOpen size={18} />
              API Docs
            </a>
          </div>
          {status ? (
            <div className={status === "Connected" ? "notice success landing-status" : "notice error landing-status"}>
              {status === "Connected" ? <CheckCircle2 size={18} /> : null}
              <span>{status}</span>
            </div>
          ) : null}
        </div>

        <aside className="landing-runtime-panel" aria-label="Production runtime configuration">
          <div className="runtime-header">
            <LockKeyhole size={20} />
            <span>Production runtime</span>
          </div>
          <div className="runtime-grid">
            <div>
              <span>Secrets</span>
              <strong>Backend .env only</strong>
            </div>
            <div>
              <span>Chains</span>
              <strong>Ethereum + Polygon</strong>
            </div>
            <div>
              <span>Tokens</span>
              <strong>USDC / USDT</strong>
            </div>
            <div>
              <span>Approvals</span>
              <strong>StakeEnforcer spender</strong>
            </div>
          </div>
          <p>
            OpenAI keys, RPC credentials, and the enforcer signer are loaded by the Go API process from environment variables. They are never bundled into the React app.
          </p>
        </aside>
      </section>

      <section className="landing-band" aria-label="System capabilities">
        <div>
          <ListChecks size={20} />
          <span>Daily and weekly goal tracking</span>
        </div>
        <div>
          <Zap size={20} />
          <span>Burn-only ETH/POL StakeEnforcer</span>
        </div>
        <div>
          <CircleDollarSign size={20} />
          <span>USDC and USDT stakes</span>
        </div>
        <div>
          <Bot size={20} />
          <span>GPT function-calling manager</span>
        </div>
      </section>
    </main>
  );
}
