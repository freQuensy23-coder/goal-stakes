import { describe, expect, it, vi } from "vitest";
import { ensureStakeApproval } from "./stakeApproval";
import type { ApiClient } from "./api";
import type { ApprovalStatus } from "../types";

function approval(allowance: string): ApprovalStatus {
  return { chain: "ethereum", token_symbol: "USDC", allowance, approved: allowance !== "0" };
}

describe("ensureStakeApproval", () => {
  it("does not request MetaMask approval when cached allowance covers the stake", async () => {
    const approve = vi.fn();
    const recordApproval = vi.fn();
    const api = {
      getApproval: vi.fn(async () => approval("100000000")),
      recordApproval,
    } as unknown as ApiClient;

    const result = await ensureStakeApproval({
      api,
      chain: "ethereum",
      tokenSymbol: "USDC",
      amount: "100",
      tokenAddress: "0x2222222222222222222222222222222222222222",
      spender: "0x1111111111111111111111111111111111111111",
      approve,
    });

    expect(result.hash).toBeUndefined();
    expect(approve).not.toHaveBeenCalled();
    expect(recordApproval).not.toHaveBeenCalled();
  });

  it("requests approval and records it when allowance is below the stake", async () => {
    const api = {
      getApproval: vi.fn(async () => approval("99999999")),
      recordApproval: vi.fn(async () => approval("200000000")),
    } as unknown as ApiClient;
    const approve = vi.fn(async () => ({ hash: "0xapprove", allowance: "200000000" }));

    const result = await ensureStakeApproval({
      api,
      chain: "ethereum",
      tokenSymbol: "USDC",
      amount: "200",
      tokenAddress: "0x2222222222222222222222222222222222222222",
      spender: "0x1111111111111111111111111111111111111111",
      approve,
    });

    expect(approve).toHaveBeenCalledWith("ethereum", "200", "0x2222222222222222222222222222222222222222", "0x1111111111111111111111111111111111111111");
    expect(api.recordApproval).toHaveBeenCalledWith({ chain: "ethereum", token_symbol: "USDC", tx_hash: "0xapprove", dry_run_allowance: "200000000" });
    expect(api.recordApproval).not.toHaveBeenCalledWith(expect.objectContaining({ allowance: expect.anything() }));
    expect(result.hash).toBe("0xapprove");
  });

  it("rejects unconfigured chain or token selections with a readable error", async () => {
    const api = {
      getApproval: vi.fn(async () => approval("0")),
      recordApproval: vi.fn(),
    } as unknown as ApiClient;
    const approve = vi.fn();

    await expect(ensureStakeApproval({
      api,
      chain: "ethereum",
      tokenSymbol: "USDC",
      amount: "100",
      approve,
    })).rejects.toThrow("Selected chain/token is not configured");
    expect(approve).not.toHaveBeenCalled();
    expect(api.recordApproval).not.toHaveBeenCalled();
  });
});
