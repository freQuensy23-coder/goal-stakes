// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

/// @notice ERC20-shaped mock whose transferFrom returns `false` WITHOUT reverting.
/// Some non-compliant tokens signal failure this way. The SafeERC20-style helper must
/// treat a `false` return as a failed transfer and revert (GPC6 — fail fast).
contract MockReturnsFalseERC20 {
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        return true;
    }

    // Returns false instead of reverting — never moves funds.
    function transferFrom(address, address, uint256) external pure returns (bool) {
        return false;
    }
}
