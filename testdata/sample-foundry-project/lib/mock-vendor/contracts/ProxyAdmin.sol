// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title ProxyAdmin
 * @notice A minimal admin contract for testing dependency publishing
 */
contract ProxyAdmin {
    address public owner;

    event OwnershipTransferred(address previousOwner, address newOwner);

    constructor() {
        owner = msg.sender;
        emit OwnershipTransferred(address(0), msg.sender);
    }

    modifier onlyOwner() {
        require(msg.sender == owner, "Not owner");
        _;
    }

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "Zero address");
        address previousOwner = owner;
        owner = newOwner;
        emit OwnershipTransferred(previousOwner, newOwner);
    }
}
