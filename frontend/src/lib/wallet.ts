import { createPublicClient, createWalletClient, custom, parseUnits } from "viem";
import { requireChainByKey, tokenDecimals } from "./chains";

const erc20Abi = [
  {
    type: "function",
    name: "approve",
    stateMutability: "nonpayable",
    inputs: [
      { name: "spender", type: "address" },
      { name: "amount", type: "uint256" },
    ],
    outputs: [{ name: "", type: "bool" }],
  },
  {
    type: "function",
    name: "allowance",
    stateMutability: "view",
    inputs: [
      { name: "owner", type: "address" },
      { name: "spender", type: "address" },
    ],
    outputs: [{ name: "", type: "uint256" }],
  },
] as const;

export async function connectWallet() {
  if (!window.ethereum) throw new Error("MetaMask is not available");
  const accounts = (await window.ethereum.request({ method: "eth_requestAccounts" })) as string[];
  if (!accounts[0]) throw new Error("No wallet account returned");
  return accounts[0];
}

export async function signMessage(address: string, message: string) {
  if (!window.ethereum) throw new Error("MetaMask is not available");
  return (await window.ethereum.request({
    method: "personal_sign",
    params: [message, address],
  })) as string;
}

export async function currentChainID() {
  if (!window.ethereum) throw new Error("MetaMask is not available");
  const chainID = (await window.ethereum.request({ method: "eth_chainId" })) as string;
  return Number.parseInt(chainID, 16);
}

export function buildSIWEMessage(address: string, nonce: string, chainID: number) {
  const issuedAt = new Date().toISOString();
  return `${window.location.host} wants you to sign in with your Ethereum account:
${address}

Sign in to Goal Stakes.

URI: ${window.location.origin}
Version: 1
Chain ID: ${chainID}
Nonce: ${nonce}
Issued At: ${issuedAt}`;
}

export async function approveStake(chainKey: string, amount: string, token: string, spender: string) {
  if (!window.ethereum) throw new Error("MetaMask is not available");
  const chain = requireChainByKey(chainKey);
  if (!spender || !token) throw new Error(`Missing token or StakeEnforcer address for ${chain.label}`);

  const walletClient = createWalletClient({ chain: chain.viem, transport: custom(window.ethereum) });
  const [account] = await walletClient.requestAddresses();
  await walletClient.switchChain({ id: chain.viem.id });
  const publicClient = createPublicClient({ chain: chain.viem, transport: custom(window.ethereum) });
  const targetAllowance = parseUnits(amount, tokenDecimals);
  const currentAllowance = await publicClient.readContract({
    address: token as `0x${string}`,
    abi: erc20Abi,
    functionName: "allowance",
    args: [account, spender as `0x${string}`],
  });

  if (currentAllowance >= targetAllowance) {
    return { hash: "already approved on-chain", allowance: currentAllowance.toString() };
  }

  if (currentAllowance > 0n && targetAllowance > 0n) {
    const resetHash = await walletClient.writeContract({
      account,
      address: token as `0x${string}`,
      abi: erc20Abi,
      functionName: "approve",
      args: [spender as `0x${string}`, 0n],
      chain: chain.viem,
    });
    const resetReceipt = await publicClient.waitForTransactionReceipt({ hash: resetHash });
    if (resetReceipt.status === "reverted") throw new Error("Approval reset transaction reverted");
  }

  const hash = await walletClient.writeContract({
    account,
    address: token as `0x${string}`,
    abi: erc20Abi,
    functionName: "approve",
    args: [spender as `0x${string}`, targetAllowance],
    chain: chain.viem,
  });
  const receipt = await publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status === "reverted") throw new Error("Approval transaction reverted");
  return { hash, allowance: targetAllowance.toString() };
}
