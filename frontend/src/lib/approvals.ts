import type { ApiClient } from "./api";
import { parseUnits } from "viem";
import { supportedChainInfo, supportedTokens, tokenDecimals } from "./chains";
import type { ApprovalStatus } from "../types";

export const defaultApprovalAmount = "100";

export function approvalCoversAmount(approval: ApprovalStatus, amount: string) {
  try {
    const target = parseUnits(amount || "0", tokenDecimals);
    const allowance = BigInt(approval.allowance);
    return target > 0n && allowance >= target;
  } catch {
    return false;
  }
}

export async function hasAnyRecordedApproval(api: ApiClient, requiredAmount = defaultApprovalAmount) {
  const chains = supportedChainInfo(await api.listChains());
  const configured = chains.flatMap((chain) =>
    supportedTokens
      .filter((token) => chain.stake_enforcer_address && chain.tokens[token])
      .map((token) => ({ chain: chain.key, token })),
  );

  const checks = await Promise.all(
    configured.map(({ chain, token }) =>
      api
        .getApproval(chain, token)
        .then((approval) => approvalCoversAmount(approval, requiredAmount))
        .catch(() => false),
    ),
  );
  return checks.some(Boolean);
}
