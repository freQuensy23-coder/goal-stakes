import { FormEvent, useEffect, useMemo, useState } from "react";
import { AlertTriangle, Check, CircleDollarSign, Pencil, Plus, RefreshCw, Save, Siren, Trash2, X } from "lucide-react";
import type { ApiClient } from "../lib/api";
import type { ChainInfo, Goal, Progress, Violation } from "../types";
import { chainLabel, supportedChainInfo, supportedTokens, tokenOptionsFromChain, type SupportedToken } from "../lib/chains";
import { ensureStakeApproval } from "../lib/stakeApproval";
import { formatStakeAmount, parseStakeAmount } from "../lib/stakeAmount";

export function GoalsScreen({ api }: { api: ApiClient }) {
  const [goals, setGoals] = useState<Goal[]>([]);
  const [progressByGoal, setProgressByGoal] = useState<Record<string, Progress>>({});
  const [status, setStatus] = useState("");
  const [editingID, setEditingID] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<GoalEditForm | null>(null);
  const [chainOptions, setChainOptions] = useState<ChainInfo[]>([]);
  const [form, setForm] = useState<{
    title: string;
    type: Goal["type"];
    cadence: Goal["cadence"];
    stake: string;
    token: SupportedToken;
    chain: string;
    startsAt: string;
    endsAt: string;
  }>(() => ({
    title: "",
    type: "do",
    cadence: "daily",
    stake: "100",
    token: "USDC",
    chain: "",
    startsAt: "",
    endsAt: "",
  }));
  const selectedChain = useMemo(() => chainOptions.find((chain) => chain.key === form.chain), [chainOptions, form.chain]);
  const tokenOptions = useMemo(() => (selectedChain ? tokenOptionsFromChain(selectedChain) : supportedTokens), [selectedChain]);

  useEffect(() => {
    if (!tokenOptions.includes(form.token)) {
      setForm((current) => ({ ...current, token: tokenOptions[0] ?? "USDC" }));
    }
  }, [form.token, tokenOptions]);

  useEffect(() => {
    let cancelled = false;
    void api
      .listChains()
      .then((chains) => {
        if (cancelled) return;
        const sortedChains = supportedChainInfo(chains);
        setChainOptions(sortedChains);
        if (!sortedChains.length) {
          setStatus("No supported chains are configured");
          return;
        }
        const first = sortedChains[0];
        setForm((current) => {
          if (current.chain && sortedChains.some((chain) => chain.key === current.chain)) return current;
          return { ...current, chain: first.key, token: tokenOptionsFromChain(first)[0] ?? "USDC" };
        });
      })
      .catch((error) => {
        if (!cancelled) setStatus(error instanceof Error ? error.message : "Could not load chain configuration");
      });
    return () => {
      cancelled = true;
    };
  }, [api]);

  async function load() {
    try {
      const nextGoals = await api.listGoals();
      setGoals(nextGoals);
      const progressEntries = await Promise.all(
        nextGoals.map(async (goal) => [goal.id, await api.getProgress(goal.id)] as const),
      );
      setProgressByGoal(Object.fromEntries(progressEntries));
      setStatus("");
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Could not load goals");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function create(event: FormEvent) {
    event.preventDefault();
    setStatus("");
    try {
      const stakeAmount = parseStakeAmount(form.stake);
      await ensureStakeApproval({
        api,
        chain: form.chain,
        tokenSymbol: form.token,
        amount: form.stake || "0",
        tokenAddress: selectedChain?.tokens[form.token],
        spender: selectedChain?.stake_enforcer_address,
      });
      await api.createGoal({
        title: form.title,
        type: form.type as Goal["type"],
        cadence: form.cadence as Goal["cadence"],
        stake_amount: stakeAmount,
        token_symbol: form.token,
        chain: form.chain,
        timezone: browserTimezone(),
        starts_at: dateInputToRFC3339(form.startsAt),
        ends_at: dateInputToRFC3339(form.endsAt),
      });
      setForm((current) => ({ ...current, title: "" }));
      await load();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Create goal failed");
    }
  }

  async function action(fn: () => Promise<unknown>) {
    setStatus("");
    try {
      await fn();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Action failed");
    } finally {
      await load();
    }
  }

  function startEdit(goal: Goal) {
    setStatus("");
    setEditingID(goal.id);
    setEditForm({
      title: goal.title,
      stake: formatStakeAmount(goal.stake_amount),
      token: goal.token_symbol,
      chain: goal.chain,
      endsAt: dateTimeToDateInput(goal.ends_at),
    });
  }

  async function saveEdit(event: FormEvent, goal: Goal) {
    event.preventDefault();
    if (!editForm) return;
    const title = editForm.title.trim();
    if (!title) {
      setStatus("Goal title is required");
      return;
    }
    setStatus("");
    try {
      const stakeAmount = parseStakeAmount(editForm.stake);
      const tokenOrChainChanged = editForm.token !== goal.token_symbol || editForm.chain !== goal.chain;
      const stakeChanged = tokenOrChainChanged || stakeAmount !== goal.stake_amount;
      if (stakeChanged) {
        const targetChain = chainOptions.find((chain) => chain.key === editForm.chain);
        await ensureStakeApproval({
          api,
          chain: editForm.chain,
          tokenSymbol: editForm.token,
          amount: editForm.stake || "0",
          tokenAddress: targetChain?.tokens[editForm.token],
          spender: targetChain?.stake_enforcer_address,
        });
      }
      await api.updateGoal(goal.id, {
        title,
        description: goal.description,
        stake_amount: tokenOrChainChanged ? goal.stake_amount : stakeAmount,
        ends_at: dateInputToRFC3339(editForm.endsAt) ?? null,
      });
      if (tokenOrChainChanged) {
        await api.updateStake(goal.id, { stake_amount: stakeAmount, token_symbol: editForm.token, chain: editForm.chain });
      }
      setEditingID(null);
      setEditForm(null);
      await load();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Update goal failed");
    }
  }

  async function archive(goal: Goal) {
    setStatus("");
    try {
      await api.archiveGoal(goal.id);
      if (editingID === goal.id) {
        setEditingID(null);
        setEditForm(null);
      }
      await load();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Archive failed");
    }
  }

  return (
    <section className="goals-layout">
      <form className="goal-form" onSubmit={create}>
        <input value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} placeholder="Do 100 push-ups every day" />
        <select value={form.type} onChange={(event) => setForm({ ...form, type: event.target.value as Goal["type"] })}>
          <option value="do">Do</option>
          <option value="avoid">Avoid</option>
        </select>
        <select value={form.cadence} onChange={(event) => setForm({ ...form, cadence: event.target.value as Goal["cadence"] })}>
          <option value="daily">Daily</option>
          <option value="weekly">Weekly</option>
        </select>
        <input value={form.stake} onChange={(event) => setForm({ ...form, stake: event.target.value })} inputMode="decimal" aria-label="Stake amount" />
        <input type="date" value={form.startsAt} onChange={(event) => setForm({ ...form, startsAt: event.target.value })} aria-label="Start date" />
        <input type="date" value={form.endsAt} onChange={(event) => setForm({ ...form, endsAt: event.target.value })} aria-label="End date" />
        <select value={form.token} onChange={(event) => setForm({ ...form, token: event.target.value as SupportedToken })}>
          {tokenOptions.map((token) => (
            <option key={token} value={token}>{token}</option>
          ))}
        </select>
        <select value={form.chain} onChange={(event) => setForm({ ...form, chain: event.target.value })}>
          {chainOptions.map((chain) => (
            <option key={chain.key} value={chain.key}>{chainLabel(chain.key)}</option>
          ))}
        </select>
        <button className="primary-button" disabled={!form.title.trim() || !form.chain}>
          <Plus size={18} />
          Create
        </button>
      </form>

      {status ? <div className="notice error"><AlertTriangle size={18} />{status}</div> : null}

      <div className="goal-list">
        {goals.map((goal) => {
          const isEditing = editingID === goal.id && editForm;
          const editChain = chainOptions.find((chain) => chain.key === editForm?.chain);
          const editTokenOptions = editChain ? tokenOptionsFromChain(editChain) : tokenOptions;
          return (
            <article className="goal-item" key={goal.id}>
              {isEditing ? (
                <form className="goal-edit-form" onSubmit={(event) => saveEdit(event, goal)}>
                  <input value={editForm.title} onChange={(event) => setEditForm({ ...editForm, title: event.target.value })} aria-label="Goal title" />
                  <input value={editForm.stake} onChange={(event) => setEditForm({ ...editForm, stake: event.target.value })} inputMode="decimal" aria-label="Stake amount" />
                  <input type="date" value={editForm.endsAt} onChange={(event) => setEditForm({ ...editForm, endsAt: event.target.value })} aria-label="End date" />
                  <select value={editForm.token} onChange={(event) => setEditForm({ ...editForm, token: event.target.value as SupportedToken })}>
                    {editTokenOptions.map((token) => (
                      <option key={token} value={token}>{token}</option>
                    ))}
                  </select>
                  <select value={editForm.chain} onChange={(event) => {
                    const nextChain = chainOptions.find((chain) => chain.key === event.target.value);
                    setEditForm({ ...editForm, chain: event.target.value, token: nextChain ? tokenOptionsFromChain(nextChain)[0] ?? "USDC" : editForm.token });
                  }}>
                    {chainOptions.map((chain) => (
                      <option key={chain.key} value={chain.key}>{chainLabel(chain.key)}</option>
                    ))}
                  </select>
                  <button className="primary-button" disabled={!editForm.title.trim() || !editForm.chain}>
                    <Save size={18} />
                    Save
                  </button>
                  <button type="button" className="icon-button" onClick={() => { setEditingID(null); setEditForm(null); }} title="Cancel">
                    <X size={18} />
                  </button>
                </form>
              ) : (
                <>
                  <div className="goal-main">
                    <h2>{goal.title}</h2>
                    <p>{goal.type} · {goal.cadence} · {formatStakeAmount(goal.stake_amount)} {goal.token_symbol} · {goal.chain}{goal.ends_at ? ` · ends ${formatDate(goal.ends_at)}` : ""}</p>
                    <GoalProgress progress={progressByGoal[goal.id]} />
                  </div>
                  <div className="goal-actions">
                    <button className="secondary-button" onClick={() => action(() => api.logCheckIn(goal.id, "done"))} title="Log check-in">
                      <Check size={18} />
                      Done
                    </button>
                    <button className="secondary-button danger" onClick={() => action(() => api.reportViolation(goal.id, "reported in app"))} title="Report violation">
                      <Siren size={18} />
                      Violation
                    </button>
                    <button className="icon-button" onClick={() => startEdit(goal)} title="Edit goal">
                      <Pencil size={18} />
                    </button>
                    <button className="icon-button danger" onClick={() => archive(goal)} title="Archive goal">
                      <Trash2 size={18} />
                    </button>
                  </div>
                </>
              )}
            </article>
          );
        })}
        {!goals.length ? (
          <section className="empty-state compact">
            <CircleDollarSign size={30} />
            <h2>No goals yet</h2>
          </section>
        ) : null}
      </div>
      <button className="secondary-button refresh" onClick={load}>
        <RefreshCw size={18} />
        Refresh
      </button>
    </section>
  );
}

interface GoalEditForm {
  title: string;
  stake: string;
  token: SupportedToken;
  chain: string;
  endsAt: string;
}

function browserTimezone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

function dateInputToRFC3339(value: string) {
  if (!value) return undefined;
  return new Date(`${value}T00:00:00`).toISOString();
}

function dateTimeToDateInput(value?: string) {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "";
  const year = parsed.getFullYear();
  const month = String(parsed.getMonth() + 1).padStart(2, "0");
  const day = String(parsed.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function formatDate(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleDateString(undefined, { dateStyle: "medium" });
}

function GoalProgress({ progress }: { progress?: Progress }) {
  if (!progress) return null;
  const lastViolation = progress.violations.at(-1);
  const note = progress.current_period_check_in?.note?.trim();
  return (
    <div className="goal-progress-block">
      <div className="goal-progress">
        <span className={progress.current_period_completed ? "status-pill success" : "status-pill"}>
          {progress.current_period_completed ? "Current period done" : "Current period open"}
        </span>
        <span>{progress.current_period}</span>
        <span>{progress.violations.length} violation{progress.violations.length === 1 ? "" : "s"}</span>
        {lastViolation ? <ViolationSummary violation={lastViolation} /> : null}
      </div>
      {note ? <p className="check-in-note">Progress note: {note}</p> : null}
    </div>
  );
}

function ViolationSummary({ violation }: { violation: Violation }) {
  return (
    <span className={`status-pill ${violation.status === "charged" ? "success" : violation.status === "failed" ? "danger" : ""}`}>
      Last violation: {violation.status}
    </span>
  );
}
