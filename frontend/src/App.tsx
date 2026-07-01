import { useEffect, useMemo, useState } from "react";
import { Bot, CheckCircle2, KeyRound, ListChecks, LogOut, ShieldCheck, Wallet } from "lucide-react";
import { ApiClient } from "./lib/api";
import { hasAnyRecordedApproval } from "./lib/approvals";
import { buildSIWEMessage, connectWallet, currentChainID, signMessage } from "./lib/wallet";
import { ApprovalGate } from "./screens/ApprovalGate";
import { ChatScreen } from "./screens/ChatScreen";
import { GoalsScreen } from "./screens/GoalsScreen";
import { LandingScreen } from "./screens/LandingScreen";
import { SettingsScreen } from "./screens/SettingsScreen";

type Screen = "chat" | "goals" | "settings";

export function App() {
  const [screen, setScreen] = useState<Screen>("chat");
  const [token, setToken] = useState(() => localStorage.getItem("goalstakes.session"));
  const [wallet, setWallet] = useState(() => localStorage.getItem("goalstakes.wallet") ?? "");
  const [authStatus, setAuthStatus] = useState("");
  const [approvalReady, setApprovalReady] = useState(false);
  const [approvalLoading, setApprovalLoading] = useState(false);
  const api = useMemo(() => new ApiClient(token), [token]);

  useEffect(() => {
    if (!token) {
      setApprovalReady(false);
      setApprovalLoading(false);
      return;
    }

    let cancelled = false;
    setApprovalLoading(true);
    void hasAnyRecordedApproval(api)
      .then((approved) => {
        if (!cancelled) setApprovalReady(approved);
      })
      .finally(() => {
        if (!cancelled) setApprovalLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [api, token]);

  async function signIn() {
    setAuthStatus("");
    try {
      const address = await connectWallet();
      const [{ nonce }, chainID] = await Promise.all([api.issueNonce(address), currentChainID()]);
      const message = buildSIWEMessage(address, nonce, chainID);
      const signature = await signMessage(address, message);
      const result = await api.verifySIWE(message, signature);
      localStorage.setItem("goalstakes.session", result.token);
      localStorage.setItem("goalstakes.wallet", result.user.wallet_address);
      setToken(result.token);
      setWallet(result.user.wallet_address);
      setAuthStatus("Connected");
    } catch (error) {
      setAuthStatus(error instanceof Error ? error.message : "Wallet sign-in failed");
    }
  }

  function signOut() {
    localStorage.removeItem("goalstakes.session");
    localStorage.removeItem("goalstakes.wallet");
    localStorage.removeItem("goalstakes.conversation");
    setToken(null);
    setWallet("");
    setAuthStatus("");
    setApprovalReady(false);
  }

  const gated = Boolean(token) && !approvalReady;
  const heading = gated ? "Approval" : screen === "chat" ? "Chat" : screen === "goals" ? "Goals" : "Settings";
  const eyebrow = gated ? "Wallet setup" : screen === "chat" ? "AI manager" : screen === "goals" ? "Manual tracking" : "Account";

  if (!token) {
    return <LandingScreen onConnect={signIn} status={authStatus} />;
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <ShieldCheck size={24} />
          <span>Goal Stakes</span>
        </div>
        <nav className="nav-tabs">
          <button className={screen === "chat" ? "active" : ""} onClick={() => setScreen("chat")} title="AI manager">
            <Bot size={18} />
            <span>Chat</span>
          </button>
          <button className={screen === "goals" ? "active" : ""} onClick={() => setScreen("goals")} title="Goals">
            <ListChecks size={18} />
            <span>Goals</span>
          </button>
          <button className={screen === "settings" ? "active" : ""} onClick={() => setScreen("settings")} title="Settings">
            <KeyRound size={18} />
            <span>Settings</span>
          </button>
        </nav>
      </aside>

      <main className="workspace">
        <header className="topbar">
          <div>
            <p className="eyebrow">{eyebrow}</p>
            <h1>{heading}</h1>
          </div>
          <div className="wallet-box">
            {wallet ? <span className="wallet-chip">{wallet.slice(0, 6)}...{wallet.slice(-4)}</span> : null}
            {token ? (
              <button className="icon-button" onClick={signOut} title="Sign out">
                <LogOut size={18} />
              </button>
            ) : (
              <button className="primary-button" onClick={signIn}>
                <Wallet size={18} />
                Connect
              </button>
            )}
          </div>
        </header>

        {authStatus ? (
          <div className={authStatus === "Connected" ? "notice success" : "notice error"}>
            {authStatus === "Connected" ? <CheckCircle2 size={18} /> : null}
            <span>{authStatus}</span>
          </div>
        ) : null}

        {approvalLoading ? (
          <section className="empty-state">
            <Wallet size={34} />
            <h2>Checking token approval</h2>
          </section>
        ) : !approvalReady ? (
          <ApprovalGate api={api} onApproved={() => setApprovalReady(true)} />
        ) : screen === "chat" ? (
          <ChatScreen api={api} />
        ) : screen === "goals" ? (
          <GoalsScreen api={api} />
        ) : (
          <SettingsScreen api={api} />
        )}
      </main>
    </div>
  );
}
