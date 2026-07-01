// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Test} from "forge-std/Test.sol";
import {StakeEnforcer} from "../src/StakeEnforcer.sol";

interface IERC20MainnetLike {
    function balanceOf(address account) external view returns (uint256);
    function allowance(address owner, address spender) external view returns (uint256);
}

contract StakeEnforcerForkTest is Test {
    address constant BURN = 0x000000000000000000000000000000000000dEaD;

    address constant ETHEREUM_USDC = 0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48;
    address constant ETHEREUM_USDT = 0xdAC17F958D2ee523a2206206994597C13D831ec7;
    address constant POLYGON_USDC = 0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359;
    address constant POLYGON_USDT = 0xc2132D05D31c914a87C6611C10748AEb04B58e8F;

    address internal user = makeAddr("fork-user");
    address internal enforcer = makeAddr("fork-enforcer");

    uint256 internal constant AMOUNT = 1_000_000; // 1 token at 6 decimals.

    function test_forkEthereumUSDC_burnsRealTokenToDeadAddress() public {
        _assertForkPenalty("ETHEREUM_RPC_URL", 1, ETHEREUM_USDC);
    }

    function test_forkEthereumUSDT_burnsRealTokenToDeadAddress() public {
        _assertForkPenalty("ETHEREUM_RPC_URL", 1, ETHEREUM_USDT);
    }

    function test_forkPolygonUSDC_burnsRealTokenToDeadAddress() public {
        _assertForkPenalty("POLYGON_RPC_URL", 137, POLYGON_USDC);
    }

    function test_forkPolygonUSDT_burnsRealTokenToDeadAddress() public {
        _assertForkPenalty("POLYGON_RPC_URL", 137, POLYGON_USDT);
    }

    function _assertForkPenalty(string memory rpcEnv, uint256 expectedChainId, address token) internal {
        vm.createSelectFork(vm.envString(rpcEnv));
        assertEq(block.chainid, expectedChainId, "wrong fork chain id");
        assertGt(token.code.length, 0, "canonical token code missing on fork");

        StakeEnforcer stakeEnforcer = new StakeEnforcer(enforcer);
        assertEq(stakeEnforcer.BURN(), BURN, "wrong burn address");

        IERC20MainnetLike erc20 = IERC20MainnetLike(token);
        uint256 userStart = AMOUNT * 2;
        uint256 burnBefore = erc20.balanceOf(BURN);

        deal(token, user, userStart);
        assertEq(erc20.balanceOf(user), userStart, "fork deal did not fund real token balance");

        vm.prank(user);
        _safeApprove(token, address(stakeEnforcer), AMOUNT);
        assertEq(erc20.allowance(user, address(stakeEnforcer)), AMOUNT, "approval not recorded on real token");

        vm.prank(enforcer);
        stakeEnforcer.penalize(user, token, AMOUNT);

        assertEq(erc20.balanceOf(user), userStart - AMOUNT, "user not debited exactly");
        assertEq(erc20.balanceOf(BURN), burnBefore + AMOUNT, "dead address not credited exactly");
        assertEq(erc20.balanceOf(address(stakeEnforcer)), 0, "StakeEnforcer retained custody");
        assertEq(erc20.allowance(user, address(stakeEnforcer)), 0, "allowance not consumed exactly");
    }

    function _safeApprove(address token, address spender, uint256 amount) internal {
        (bool ok, bytes memory ret) = token.call(
            abi.encodeWithSelector(bytes4(keccak256("approve(address,uint256)")), spender, amount)
        );
        assertTrue(ok, "approve reverted");
        if (ret.length != 0) {
            assertTrue(abi.decode(ret, (bool)), "approve returned false");
        }
    }
}
