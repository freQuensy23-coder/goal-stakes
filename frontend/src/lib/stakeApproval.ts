import type { ApiClient } from "./api";
import { approvalCoversAmount } from "./approvals";
import { approveStake } from "./wallet";
import type { SupportedToken } from "./chains";

type ApproveStake = typeof approveStake;

export interface EnsureStakeApprovalInput {
  api: ApiClient;
  chain: string;
  tokenSymbol: SupportedToken;
  amount: string;
  tokenAddress?: string;
  spender?: string;
  approve?: ApproveStake;
}

export async function ensureStakeApproval({
  api,
  chain,
  tokenSymbol,
  amount,
  tokenAddress,
  spender,
  approve = approveStake,
}: EnsureStakeApprovalInput) {
  const current = await api.getApproval(chain, tokenSymbol);
  if (approvalCoversAmount(current, amount)) {
    return { status: current, hash: undefined as string | undefined };
  }
  if (!tokenAddress || !spender) {
    throw new Error("Selected chain/token is not configured");
  }
  const result = await approve(chain, amount, tokenAddress, spender);
  const recorded = await api.recordApproval({ chain, token_symbol: tokenSymbol, tx_hash: result.hash, dry_run_allowance: result.allowance });
  if (!approvalCoversAmount(recorded, amount)) {
    throw new Error("Approval allowance is below selected amount");
  }
  return { status: recorded, hash: result.hash };
}
