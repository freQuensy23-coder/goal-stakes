import { FormEvent, useEffect, useMemo, useState } from "react";
import { AlertTriangle, CheckCircle2, WalletCards } from "lucide-react";
import type { ApiClient } from "../lib/api";
import { approvalCoversAmount, defaultApprovalAmount } from "../lib/approvals";
import { approveStake } from "../lib/wallet";
import { chainLabel, supportedChainInfo, supportedTokens, tokenOptionsFromChain, type SupportedToken } from "../lib/chains";
import type { ApprovalStatus, ChainInfo } from "../types";

export function ApprovalGate({ api, onApproved }: { api: ApiClient; onApproved: () => void }) {
  return (
    <section className="approval-gate">
      <ApprovalPanel api={api} onApproved={onApproved} submitLabel="Approve and continue" />
    </section>
  );
}

export function ApprovalPanel({
  api,
  onApproved,
  submitLabel = "Approve",
}: {
  api: ApiClient;
  onApproved?: () => void;
  submitLabel?: string;
}) {
  const [chainOptions, setChainOptions] = useState<ChainInfo[]>([]);
  const [approval, setApproval] = useState<{ chain: string; token: SupportedToken; amount: string }>(() => ({
    chain: "",
    token: "USDC",
    amount: defaultApprovalAmount,
  }));
  const selectedChain = useMemo(() => chainOptions.find((chain) => chain.key === approval.chain), [approval.chain, chainOptions]);
  const tokenOptions = useMemo(() => (selectedChain ? tokenOptionsFromChain(selectedChain) : supportedTokens), [selectedChain]);
  const [cached, setCached] = useState<ApprovalStatus | null>(null);
  const [status, setStatus] = useState("");
  const cachedMatchesSelection = cached?.chain === approval.chain && cached?.token_symbol === approval.token;
  const cachedCoversAmount = cachedMatchesSelection ? approvalCoversAmount(cached, approval.amount) : false;

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
        setApproval((current) => {
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

  useEffect(() => {
    if (!tokenOptions.includes(approval.token)) {
      setApproval((current) => ({ ...current, token: tokenOptions[0] ?? "USDC" }));
    }
  }, [approval.token, tokenOptions]);

  useEffect(() => {
    let cancelled = false;
    setStatus("");
    setCached(null);
    if (!approval.chain) return () => {
      cancelled = true;
    };

    void api
      .getApproval(approval.chain, approval.token)
      .then((result) => {
        if (cancelled) return;
        setCached(result);
        if (approvalCoversAmount(result, approval.amount)) onApproved?.();
      })
      .catch((error) => {
        if (!cancelled) setStatus(error instanceof Error ? error.message : "Could not load approval status");
      });

    return () => {
      cancelled = true;
    };
  }, [api, approval.amount, approval.chain, approval.token, onApproved]);

  async function approve(event: FormEvent) {
    event.preventDefault();
    setStatus("");
    try {
      if (cachedCoversAmount) {
        onApproved?.();
        return;
      }
      const tokenAddress = selectedChain?.tokens[approval.token];
      if (!selectedChain?.stake_enforcer_address || !tokenAddress) {
        throw new Error("Selected chain/token is not configured");
      }
      const result = await approveStake(approval.chain, approval.amount, tokenAddress, selectedChain.stake_enforcer_address);
      const recorded = await api.recordApproval({ chain: approval.chain, token_symbol: approval.token, tx_hash: result.hash, dry_run_allowance: result.allowance });
      setCached(recorded);
      if (!approvalCoversAmount(recorded, approval.amount)) {
        setStatus("Approval allowance is below selected amount");
        return;
      }
      setStatus(`Approval submitted: ${result.hash}`);
      onApproved?.();
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Approval failed");
    }
  }

  return (
    <section className="settings-panel approval-panel">
      <div className="section-heading">
        <WalletCards size={22} />
        <h2>Token Approval</h2>
      </div>
      <form className="approval-grid" onSubmit={approve}>
        <select value={approval.chain} onChange={(event) => setApproval((current) => ({ ...current, chain: event.target.value }))}>
          {chainOptions.map((chain) => (
            <option key={chain.key} value={chain.key}>
              {chainLabel(chain.key)}
            </option>
          ))}
        </select>
        <select value={approval.token} onChange={(event) => setApproval((current) => ({ ...current, token: event.target.value as SupportedToken }))}>
          {tokenOptions.map((token) => (
            <option key={token} value={token}>
              {token}
            </option>
          ))}
        </select>
        <input value={approval.amount} onChange={(event) => setApproval((current) => ({ ...current, amount: event.target.value }))} inputMode="decimal" aria-label="Approval amount" />
        <button className="primary-button">
          <CheckCircle2 size={18} />
          {cachedCoversAmount ? "Continue" : submitLabel}
        </button>
      </form>
      {cachedCoversAmount ? <div className="notice success"><CheckCircle2 size={18} />Approval recorded</div> : null}
      {status ? <div className={status.startsWith("Approval submitted") ? "notice success" : "notice error"}>{status.startsWith("Approval submitted") ? <CheckCircle2 size={18} /> : <AlertTriangle size={18} />}{status}</div> : null}
    </section>
  );
}
