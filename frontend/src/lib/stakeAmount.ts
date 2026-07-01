import { formatUnits, parseUnits } from "viem";
import { tokenDecimals } from "./chains";

export function parseStakeAmount(value: string) {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error("Stake amount is required");
  }
  const parts = trimmed.split(".");
  if (parts.length > 2 || !parts[0]?.match(/^\d+$/) || (parts[1] && !parts[1].match(/^\d+$/))) {
    throw new Error("Stake amount must be a decimal number");
  }
  if ((parts[1] ?? "").length > tokenDecimals) {
    throw new Error("USDC/USDT stakes support up to 6 decimals");
  }
  return parseUnits(trimmed, tokenDecimals).toString();
}

export function formatStakeAmount(amount: string) {
  try {
    return formatUnits(BigInt(amount), tokenDecimals);
  } catch {
    return amount;
  }
}
