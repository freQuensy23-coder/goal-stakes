// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

/// @notice Minimal ERC20 interface. Declared with a bool return so the standard path
/// type-checks; non-standard tokens (USDT) that omit the return are handled by the
/// low-level call in `_safeTransferFrom`, which tolerates empty return data (UK4).
interface IERC20 {
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
}

/// @title StakeEnforcer
/// @notice Pulls a forfeited stake from a user's ERC20 allowance to the dead burn address.
///
/// Design guarantees:
/// - IV1: forfeited funds can ONLY go to {BURN}. The destination is
///   immutable; there is no setter, no parameter, no code path that redirects it.
/// - IV2: the contract never takes custody. A penalty is a single
///   transferFrom(user -> PENALTY_RECIPIENT) bounded by the user's current
///   allowance; no token balance is held between calls.
contract StakeEnforcer {
    /// @notice The ONLY destination forfeited stake can ever reach (IV1).
    address public constant BURN = 0x000000000000000000000000000000000000dEaD;

    /// @notice Contract administrator; may rotate the enforcer role.
    address public owner;

    /// @notice Address authorized to trigger penalties (the backend signer).
    address public enforcer;

    error NotOwner();
    error NotEnforcer();
    error ZeroAddress();
    error TransferFailed();

    /// @notice Emitted when a stake is forfeited.
    event Penalized(address indexed user, address indexed token, uint256 amount);

    /// @notice Emitted when the enforcer role is rotated.
    event EnforcerChanged(address indexed previousEnforcer, address indexed newEnforcer);

    modifier onlyOwner() {
        _onlyOwner();
        _;
    }

    modifier onlyEnforcer() {
        _onlyEnforcer();
        _;
    }

    /// @param enforcer_ initial enforcer address (GPC1 — explicit, no hidden config).
    constructor(address enforcer_) {
        if (enforcer_ == address(0)) revert ZeroAddress();
        owner = msg.sender;
        enforcer = enforcer_;
        emit EnforcerChanged(address(0), enforcer_);
    }

    /// @notice Send `amount` of `token` pulled from `user`'s allowance to the burn address.
    /// @dev Single transferFrom(user -> BURN): the contract never
    /// holds the funds (IV2), and the destination is immutable (IV1).
    function penalize(address user, address token, uint256 amount) external onlyEnforcer {
        _safeTransferFrom(token, user, BURN, amount);
        emit Penalized(user, token, amount);
    }

    /// @notice Rotate the enforcer role.
    function setEnforcer(address newEnforcer) external onlyOwner {
        if (newEnforcer == address(0)) revert ZeroAddress();
        address previous = enforcer;
        enforcer = newEnforcer;
        emit EnforcerChanged(previous, newEnforcer);
    }

    function _onlyOwner() internal view {
        if (msg.sender != owner) revert NotOwner();
    }

    function _onlyEnforcer() internal view {
        if (msg.sender != enforcer) revert NotEnforcer();
    }

    /// @dev SafeERC20-style low-level call (UK4). Treats a successful call with EMPTY
    /// return data as success (USDT-like tokens), and decodes a bool only when return
    /// data is present, reverting on `false` (GPC6 — fail fast, no silent success).
    function _safeTransferFrom(address token, address from, address to, uint256 amount) internal {
        bytes memory data =
            abi.encodeWithSelector(IERC20.transferFrom.selector, from, to, amount);
        (bool ok, bytes memory ret) = token.call(data);
        if (!ok) revert TransferFailed();
        if (ret.length != 0 && !abi.decode(ret, (bool))) revert TransferFailed();
    }
}
