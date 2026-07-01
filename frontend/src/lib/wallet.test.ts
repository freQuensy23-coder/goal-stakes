import { afterEach, describe, expect, it, vi } from "vitest";

const walletClient = {
  requestAddresses: vi.fn(),
  switchChain: vi.fn(),
  writeContract: vi.fn(),
};

const publicClient = {
  readContract: vi.fn(),
  waitForTransactionReceipt: vi.fn(),
};

vi.mock("viem", async () => {
  const actual = await vi.importActual<typeof import("viem")>("viem");
  return {
    ...actual,
    createWalletClient: vi.fn(() => walletClient),
    createPublicClient: vi.fn(() => publicClient),
    custom: vi.fn((transport) => transport),
  };
});

describe("approveStake", () => {
  afterEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  it("rejects reverted approval receipts before recording allowance", async () => {
    const { approveStake } = await import("./wallet");
    vi.stubGlobal("window", { ethereum: { request: vi.fn() } });
    walletClient.requestAddresses.mockResolvedValue(["0xabc0000000000000000000000000000000000000"]);
    walletClient.switchChain.mockResolvedValue(undefined);
    walletClient.writeContract.mockResolvedValue("0xapprove");
    publicClient.readContract.mockResolvedValue(0n);
    publicClient.waitForTransactionReceipt.mockResolvedValue({ status: "reverted" });

    await expect(
      approveStake(
        "ethereum",
        "100",
        "0x2222222222222222222222222222222222222222",
        "0x1111111111111111111111111111111111111111",
      ),
    ).rejects.toThrow("Approval transaction reverted");
  });

  it("rejects reverted approval reset receipts before submitting the new allowance", async () => {
    const { approveStake } = await import("./wallet");
    vi.stubGlobal("window", { ethereum: { request: vi.fn() } });
    walletClient.requestAddresses.mockResolvedValue(["0xabc0000000000000000000000000000000000000"]);
    walletClient.switchChain.mockResolvedValue(undefined);
    walletClient.writeContract.mockResolvedValueOnce("0xreset");
    publicClient.readContract.mockResolvedValue(1n);
    publicClient.waitForTransactionReceipt.mockResolvedValueOnce({ status: "reverted" });

    await expect(
      approveStake(
        "ethereum",
        "100",
        "0x2222222222222222222222222222222222222222",
        "0x1111111111111111111111111111111111111111",
      ),
    ).rejects.toThrow("Approval reset transaction reverted");

    expect(walletClient.writeContract).toHaveBeenCalledOnce();
  });
});
