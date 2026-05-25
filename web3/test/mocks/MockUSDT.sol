// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

/// @notice USDT-like mock: transferFrom / approve / transfer return NO boolean (UK4).
/// USDT on mainnet has these non-standard signatures. A naive `bool ok = token.transferFrom(...)`
/// reverts against this token; only a SafeERC20-style low-level call that tolerates empty
/// return data works. These functions deliberately declare no return type so the EVM
/// produces zero return data on success.
contract MockUSDT {
    string public name = "Tether USD";
    string public symbol = "USDT";
    uint8 public decimals = 6;

    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
    }

    // No bool return — mirrors USDT's non-standard ABI.
    function approve(address spender, uint256 amount) external {
        allowance[msg.sender][spender] = amount;
    }

    // No bool return.
    function transfer(address to, uint256 amount) external {
        _transfer(msg.sender, to, amount);
    }

    // No bool return.
    function transferFrom(address from, address to, uint256 amount) external {
        uint256 allowed = allowance[from][msg.sender];
        require(allowed >= amount, "MockUSDT: insufficient allowance");
        if (allowed != type(uint256).max) {
            allowance[from][msg.sender] = allowed - amount;
        }
        _transfer(from, to, amount);
    }

    function _transfer(address from, address to, uint256 amount) internal {
        require(balanceOf[from] >= amount, "MockUSDT: insufficient balance");
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
    }
}
