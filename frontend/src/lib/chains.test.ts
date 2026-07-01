import { describe, expect, it } from "vitest";
import { chainByKey, chainLabel, supportedChainInfo } from "./chains";
import type { ChainInfo } from "../types";

function chainInfo(key: string): ChainInfo {
  return {
    key,
    stake_enforcer_address: "0x1111111111111111111111111111111111111111",
    tokens: { USDC: "0x2222222222222222222222222222222222222222" },
  };
}

describe("chain helpers", () => {
  it("does not substitute Ethereum for an unknown backend chain key", () => {
    expect(chainByKey("base")).toBeUndefined();
    expect(chainLabel("base")).toBe("base");
  });

  it("keeps MetaMask approval options to supported configured chains", () => {
    const chains = [chainInfo("base"), chainInfo("polygon"), chainInfo("ethereum")];

    expect(supportedChainInfo(chains).map((chain) => chain.key)).toEqual(["ethereum", "polygon"]);
  });
});
