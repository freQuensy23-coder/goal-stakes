// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Test} from "forge-std/Test.sol";
import {StakeEnforcer} from "../src/StakeEnforcer.sol";
import {MockERC20} from "./mocks/MockERC20.sol";
import {MockUSDT} from "./mocks/MockUSDT.sol";
import {MockReturnsFalseERC20} from "./mocks/MockReturnsFalseERC20.sol";

contract StakeEnforcerTest is Test {
    // Hardcoded, unrecoverable burn address (AS2). NOT 0x0.
    address constant BURN = 0x000000000000000000000000000000000000dEaD;

    StakeEnforcer internal enforcerContract;

    address internal owner = address(this); // test contract deploys → it is owner
    address internal enforcer = makeAddr("enforcer");
    address internal user = makeAddr("user");
    address internal stranger = makeAddr("stranger");

    // Events must match the contract's declarations exactly.
    event Penalized(address indexed user, address indexed token, uint256 amount);
    event EnforcerChanged(address indexed previousEnforcer, address indexed newEnforcer);

    function setUp() public {
        enforcerContract = new StakeEnforcer(enforcer);
    }

    // ---- helpers ----
    function _fundAndApprove(MockERC20 token, uint256 bal, uint256 allow) internal {
        token.mint(user, bal);
        vm.prank(user);
        token.approve(address(enforcerContract), allow);
    }

    // 1 + 4: penalize moves EXACTLY `amount` user -> BURN, no other balance changes,
    // and the contract holds ZERO balance afterwards (IV1, IV2).
    function test_penalize_transfersExactAmountToBurn_standardToken() public {
        MockERC20 token = new MockERC20();
        uint256 bal = 1_000 ether;
        uint256 amount = 250 ether;
        _fundAndApprove(token, bal, amount);

        uint256 burnBefore = token.balanceOf(BURN);

        vm.prank(enforcer);
        enforcerContract.penalize(user, address(token), amount);

        assertEq(token.balanceOf(user), bal - amount, "user not debited exactly");
        assertEq(token.balanceOf(BURN), burnBefore + amount, "burn not credited exactly");
        // IV2: contract never holds custody.
        assertEq(token.balanceOf(address(enforcerContract)), 0, "contract retained balance");
    }

    // 2: only the enforcer role may call penalize.
    function test_penalize_revertsForNonEnforcer() public {
        MockERC20 token = new MockERC20();
        _fundAndApprove(token, 1_000 ether, 1_000 ether);

        vm.prank(stranger);
        vm.expectRevert(StakeEnforcer.NotEnforcer.selector);
        enforcerContract.penalize(user, address(token), 100 ether);
    }

    // Even the owner cannot call penalize (only enforcer role triggers).
    function test_penalize_revertsForOwnerWhoIsNotEnforcer() public {
        MockERC20 token = new MockERC20();
        _fundAndApprove(token, 1_000 ether, 1_000 ether);

        // owner == address(this); it is NOT the enforcer.
        vm.expectRevert(StakeEnforcer.NotEnforcer.selector);
        enforcerContract.penalize(user, address(token), 100 ether);
    }

    // 3: allowance < amount reverts and does NOT partial-transfer.
    function test_penalize_revertsWhenAllowanceTooLow() public {
        MockERC20 token = new MockERC20();
        token.mint(user, 1_000 ether);
        vm.prank(user);
        token.approve(address(enforcerContract), 50 ether); // less than amount

        uint256 userBefore = token.balanceOf(user);
        uint256 burnBefore = token.balanceOf(BURN);

        vm.prank(enforcer);
        vm.expectRevert(); // underlying token reverts on insufficient allowance
        enforcerContract.penalize(user, address(token), 100 ether);

        // No partial transfer occurred.
        assertEq(token.balanceOf(user), userBefore, "user balance changed on failed penalize");
        assertEq(token.balanceOf(BURN), burnBefore, "burn balance changed on failed penalize");
    }

    // 5a: USDT-like token (no bool return) works (UK4).
    function test_penalize_worksWithUsdtLikeNoReturnToken() public {
        MockUSDT token = new MockUSDT();
        uint256 bal = 1_000e6;
        uint256 amount = 400e6;
        token.mint(user, bal);
        vm.prank(user);
        token.approve(address(enforcerContract), amount);

        uint256 burnBefore = token.balanceOf(BURN);

        vm.prank(enforcer);
        enforcerContract.penalize(user, address(token), amount);

        assertEq(token.balanceOf(user), bal - amount, "USDT: user not debited exactly");
        assertEq(token.balanceOf(BURN), burnBefore + amount, "USDT: burn not credited exactly");
        assertEq(token.balanceOf(address(enforcerContract)), 0, "USDT: contract retained balance");
    }

    // 5b: token that returns false (without reverting) must cause penalize to revert (GPC6).
    function test_penalize_revertsWhenTokenReturnsFalse() public {
        MockReturnsFalseERC20 token = new MockReturnsFalseERC20();
        token.mint(user, 1_000 ether);
        vm.prank(user);
        token.approve(address(enforcerContract), 1_000 ether);

        vm.prank(enforcer);
        vm.expectRevert(StakeEnforcer.TransferFailed.selector);
        enforcerContract.penalize(user, address(token), 100 ether);
    }

    // 6a: penalize emits Penalized.
    function test_penalize_emitsPenalized() public {
        MockERC20 token = new MockERC20();
        uint256 amount = 100 ether;
        _fundAndApprove(token, 1_000 ether, amount);

        vm.expectEmit(true, true, false, true, address(enforcerContract));
        emit Penalized(user, address(token), amount);

        vm.prank(enforcer);
        enforcerContract.penalize(user, address(token), amount);
    }

    // 6b: setEnforcer is onlyOwner; emits EnforcerChanged; updates role.
    function test_setEnforcer_onlyOwner_emitsAndUpdates() public {
        address newEnforcer = makeAddr("newEnforcer");

        vm.expectEmit(true, true, false, false, address(enforcerContract));
        emit EnforcerChanged(enforcer, newEnforcer);

        enforcerContract.setEnforcer(newEnforcer); // owner == address(this)
        assertEq(enforcerContract.enforcer(), newEnforcer, "enforcer not updated");

        // Old enforcer can no longer penalize.
        MockERC20 token = new MockERC20();
        _fundAndApprove(token, 1_000 ether, 1_000 ether);
        vm.prank(enforcer);
        vm.expectRevert(StakeEnforcer.NotEnforcer.selector);
        enforcerContract.penalize(user, address(token), 1 ether);

        // New enforcer can.
        vm.prank(newEnforcer);
        enforcerContract.penalize(user, address(token), 1 ether);
        assertEq(token.balanceOf(BURN), 1 ether, "new enforcer could not penalize");
    }

    // 6c: setEnforcer reverts for non-owner.
    function test_setEnforcer_revertsForNonOwner() public {
        vm.prank(stranger);
        vm.expectRevert(StakeEnforcer.NotOwner.selector);
        enforcerContract.setEnforcer(stranger);
    }

    // setEnforcer rejects the zero address (GPC6 — fail fast on bad state).
    function test_setEnforcer_revertsOnZeroAddress() public {
        vm.expectRevert(StakeEnforcer.ZeroAddress.selector);
        enforcerContract.setEnforcer(address(0));
    }

    // constructor rejects a zero enforcer.
    function test_constructor_revertsOnZeroEnforcer() public {
        vm.expectRevert(StakeEnforcer.ZeroAddress.selector);
        new StakeEnforcer(address(0));
    }

    // 7 + IV1: the burn destination is a compile-time constant exposed read-only,
    // equal to AS2's dead address, and there is no path to change it.
    function test_burnAddress_isHardcodedDeadAddress() public view {
        assertEq(enforcerContract.BURN(), BURN, "BURN is not the AS2 dead address");
        assertTrue(enforcerContract.BURN() != address(0), "BURN must not be 0x0 (USDC reverts)");
    }
}
