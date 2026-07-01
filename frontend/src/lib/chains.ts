import { mainnet, polygon, polygonAmoy, sepolia } from "viem/chains";
import type { ChainInfo } from "../types";

export const supportedChains = [
  { key: "ethereum", label: "Ethereum", viem: mainnet },
  { key: "polygon", label: "Polygon", viem: polygon },
  { key: "sepolia", label: "Sepolia", viem: sepolia },
  { key: "polygon-amoy", label: "Polygon Amoy", viem: polygonAmoy },
] as const;

export type SupportedChainKey = (typeof supportedChains)[number]["key"];

export const supportedTokens = ["USDC", "USDT"] as const;
export type SupportedToken = (typeof supportedTokens)[number];

export const tokenDecimals = 6;

export function chainByKey(key: string) {
  return supportedChains.find((chain) => chain.key === key);
}

export function requireChainByKey(key: string) {
  const chain = chainByKey(key);
  if (!chain) throw new Error(`Unsupported chain: ${key}`);
  return chain;
}

export function chainLabel(key: string) {
  return chainByKey(key)?.label ?? key;
}

export function tokenOptionsFromChain(chain: ChainInfo) {
  const configured = supportedTokens.filter((token) => Boolean(chain.tokens[token]));
  return configured.length ? configured : supportedTokens;
}

export function sortChainInfo(chains: ChainInfo[]) {
  const rank = new Map<string, number>(supportedChains.map((chain, index) => [chain.key, index]));
  return [...chains].sort((a, b) => (rank.get(a.key) ?? 999) - (rank.get(b.key) ?? 999) || a.key.localeCompare(b.key));
}

export function supportedChainInfo(chains: ChainInfo[]) {
  return sortChainInfo(chains.filter((chain) => Boolean(chainByKey(chain.key))));
}
