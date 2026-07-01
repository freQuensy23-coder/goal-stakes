import { describe, expect, it } from "vitest";
import { approvalCoversAmount, hasAnyRecordedApproval } from "./approvals";
import type { ApiClient } from "./api";
import type { ApprovalStatus, ChainInfo } from "../types";

const chains: ChainInfo[] = [
  {
    key: "ethereum",
    stake_enforcer_address: "0x1111111111111111111111111111111111111111",
    tokens: { USDC: "0x2222222222222222222222222222222222222222" },
  },
];

function approval(allowance: string): ApprovalStatus {
  return { chain: "ethereum", token_symbol: "USDC", allowance, approved: allowance !== "0" };
}

describe("approval helpers", () => {
  it("requires allowance to cover the human approval amount", () => {
    expect(approvalCoversAmount(approval("999999"), "1")).toBe(false);
    expect(approvalCoversAmount(approval("1000000"), "1")).toBe(true);
    expect(approvalCoversAmount(approval("2500000"), "2.5")).toBe(true);
  });

  it("does not count low cached approval as launch-ready", async () => {
    const api = {
      listChains: async () => chains,
      getApproval: async () => approval("999999"),
    } as unknown as ApiClient;

    await expect(hasAnyRecordedApproval(api, "1")).resolves.toBe(false);
  });
});
