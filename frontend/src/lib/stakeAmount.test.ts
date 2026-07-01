import { describe, expect, it } from "vitest";
import { formatStakeAmount, parseStakeAmount } from "./stakeAmount";

describe("stake amount helpers", () => {
  it("converts decimal stake input to 6-decimal token base units", () => {
    expect(parseStakeAmount("100")).toBe("100000000");
    expect(parseStakeAmount("2.5")).toBe("2500000");
    expect(parseStakeAmount("0.000001")).toBe("1");
  });

  it("rejects stake input beyond 6 decimals with a readable error", () => {
    expect(() => parseStakeAmount("1.0000001")).toThrow("USDC/USDT stakes support up to 6 decimals");
  });

  it("rejects non-decimal stake input with a readable error", () => {
    expect(() => parseStakeAmount("one token")).toThrow("Stake amount must be a decimal number");
  });

  it("formats token base units for display", () => {
    expect(formatStakeAmount("2500000")).toBe("2.5");
  });
});
