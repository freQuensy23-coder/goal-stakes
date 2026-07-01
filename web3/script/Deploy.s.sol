// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Script, console2} from "forge-std/Script.sol";
import {StakeEnforcer} from "../src/StakeEnforcer.sol";

/// @notice Deploys StakeEnforcer to any configured EVM chain.
/// Reads the deployer key and the initial enforcer address from the environment;
/// nothing is hardcoded (GPC1). Run with an explicit RPC for the target chain:
///
///   forge script script/Deploy.s.sol:Deploy --rpc-url $SEPOLIA_RPC_URL --broadcast
///   forge script script/Deploy.s.sol:Deploy --rpc-url $AMOY_RPC_URL --broadcast
///   forge script script/Deploy.s.sol:Deploy --rpc-url $ETHEREUM_RPC_URL --broadcast
///   forge script script/Deploy.s.sol:Deploy --rpc-url $POLYGON_RPC_URL --broadcast
///
/// Required env:
///   PRIVATE_KEY   - deployer private key (hex). Never commit this.
///   ENFORCER_ADDR - initial enforcer (backend signer) address.
contract Deploy is Script {
    function run() external returns (StakeEnforcer deployed) {
        uint256 deployerKey = vm.envUint("PRIVATE_KEY");
        address enforcer = vm.envAddress("ENFORCER_ADDR");

        vm.startBroadcast(deployerKey);
        deployed = new StakeEnforcer(enforcer);
        vm.stopBroadcast();

        console2.log("StakeEnforcer deployed at:", address(deployed));
        console2.log("Enforcer set to:", enforcer);
        console2.log("Burn destination (immutable):", deployed.BURN());
    }
}
