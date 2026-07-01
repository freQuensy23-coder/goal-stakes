# web3 — StakeEnforcer

A single Foundry contract, `StakeEnforcer`, that forfeits a user's staked ERC20 by
pulling it from their allowance and **burning** it.

## What it guarantees

- **Burn-only destination (IV1).** Forfeited funds can *only* ever go to the hardcoded
  burn address. The destination is a compile-time `constant` — there is no setter, no
  parameter, no code path that can redirect a penalty anywhere else.
- **No custody (IV2).** A penalty is a single `transferFrom(user → BURN)` bounded by the
  user's current allowance. The contract never holds a token balance between calls.
- **Burn address (AS2).** `BURN = 0x000000000000000000000000000000000000dEaD` — an
  unrecoverable dead address. **Not** `0x0` (USDC reverts transfers to the zero address).
- **USDT-safe (UK4).** Transfers go through a SafeERC20-style low-level call that treats
  *empty return data* as success and only decodes a bool when return data is present, so
  non-standard tokens like USDT (whose `transferFrom` returns no bool) work correctly. A
  token that returns `false` causes a revert (fail fast).

## Layout

```
web3/
  foundry.toml
  src/StakeEnforcer.sol         # the contract (+ minimal inline IERC20, _safeTransferFrom)
  tests/StakeEnforcer.t.sol     # Foundry unit tests
  tests/mocks/                  # MockERC20 (standard), MockUSDT (no bool), MockReturnsFalseERC20
  integration_test/             # fork-local real-token checks + runner
  script/Deploy.s.sol           # testnet deploy script (reads env)
  abi/StakeEnforcer.json        # exported ABI for backend abigen (IF1) — COMMITTED
  lib/forge-std/                # Foundry test dependency submodule
```

## Dependencies

This project has **no production Solidity package dependencies** — `IERC20` and the
SafeERC20-style `_safeTransferFrom` helper are inlined in `src/StakeEnforcer.sol`.

The only build/test dependency is `forge-std`, tracked as a git submodule:

```sh
git submodule update --init --recursive web3/lib/forge-std
```

## Build & test

From `web3/`:

```sh
forge build
forge test -vvv
```

From the repo root, run fork-local checks against canonical Ethereum/Polygon USDC/USDT
contracts:

```sh
web3/integration_test/run_e2e_tests.sh
```

Use owned RPC endpoints for acceptance runs:

```sh
ETHEREUM_RPC_URL=https://... POLYGON_RPC_URL=https://... web3/integration_test/run_e2e_tests.sh
```

The fork suite must show all four real-token cases passing. Mock-only tests or
`SimulatedBackend` checks are not enough to accept money-moving Web3 behavior.

## Export ABI (IF1)

The committed `abi/StakeEnforcer.json` is consumed by the backend's abigen. Regenerate it
after any change to the contract's external surface:

```sh
forge inspect StakeEnforcer abi --json > abi/StakeEnforcer.json
```

## Deploy

The deploy script reads the deployer key and the initial enforcer address from the
environment. **Never hardcode keys or mainnet addresses.**

Required env:

- `PRIVATE_KEY`: deployer private key. Keep out of git.
- `ENFORCER_ADDR`: initial enforcer, derived from the backend `ENFORCER_PRIVATE_KEY`.
- `SEPOLIA_RPC_URL`: Sepolia RPC endpoint.
- `AMOY_RPC_URL`: Polygon Amoy RPC endpoint.
- `ETHEREUM_RPC_URL`: Ethereum mainnet RPC endpoint.
- `POLYGON_RPC_URL`: Polygon PoS mainnet RPC endpoint.

Deploy to **Sepolia**:

```sh
forge script script/Deploy.s.sol:Deploy --rpc-url "$SEPOLIA_RPC_URL" --broadcast
```

Deploy to **Polygon Amoy**:

```sh
forge script script/Deploy.s.sol:Deploy --rpc-url "$AMOY_RPC_URL" --broadcast
```

Deploy to **Ethereum mainnet**:

```sh
forge script script/Deploy.s.sol:Deploy --rpc-url "$ETHEREUM_RPC_URL" --broadcast
```

Deploy to **Polygon mainnet**:

```sh
forge script script/Deploy.s.sol:Deploy --rpc-url "$POLYGON_RPC_URL" --broadcast
```

The script logs the deployed contract address, the configured enforcer, and the (immutable)
burn destination. Put the two mainnet contract addresses into `.env.mainnet.local` before real
users approve USDC or USDT. Backend startup with `ENFORCER_PRIVATE_KEY` set will fail unless
the configured contract reports `BURN() = 0x000000000000000000000000000000000000dEaD` and
`enforcer() = ENFORCER_ADDR`.

After deploying both mainnet contracts, verify the backend handoff before opening the app
to real approvals:

```sh
export ETHEREUM_STAKE_ENFORCER_ADDRESS=0x...
export POLYGON_STAKE_ENFORCER_ADDRESS=0x...
export ETHEREUM_RPC_URL=https://mainnet.infura.io/v3/<key>
export POLYGON_RPC_URL=https://polygon-mainnet.infura.io/v3/<key>
export ENFORCER_PRIVATE_KEY=0x...
scripts/verify-mainnet-deploy.sh
```

The live e2e wrapper consumes the same deployment addresses from `.env.mainnet.local`:

```sh
scripts/live_mainnet_gate.sh shape
ENV_FILE=.env.mainnet.local scripts/live_mainnet_gate.sh preflight
ENV_FILE=.env.mainnet.local LIVE_E2E_CONFIRM=burn-real-funds scripts/live_mainnet_gate.sh burn
```

`burn` is destructive: it starts the Go API with live config, requires a prior MetaMask
approval from `LIVE_E2E_USER_WALLET`, creates and reports the test goal through the
public API, and calls `StakeEnforcer.penalize` for `LIVE_E2E_AMOUNT`.

For a local shape check without RPC calls:

```sh
ETHEREUM_STAKE_ENFORCER_ADDRESS=0x1111111111111111111111111111111111111111 \
POLYGON_STAKE_ENFORCER_ADDRESS=0x2222222222222222222222222222222222222222 \
scripts/verify-mainnet-deploy.sh --dry-run
```

## Usage

1. The user `approve`s the `StakeEnforcer` contract for the stake amount on the staked token.
2. To forfeit, the **enforcer** calls `penalize(user, token, amount)`. The contract pulls
   exactly `amount` from the user straight to `BURN` and emits `Penalized`.
3. The owner can rotate the enforcer via `setEnforcer(newEnforcer)` (emits `EnforcerChanged`).
