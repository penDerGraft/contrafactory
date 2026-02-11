// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title SimpleProxy
 * @notice A minimal proxy contract for testing dependency publishing
 */
contract SimpleProxy {
    address public implementation;
    address public admin;

    event ImplementationUpdated(address newImplementation);

    constructor(address _implementation) {
        implementation = _implementation;
        admin = msg.sender;
    }

    function updateImplementation(address newImplementation) external {
        require(msg.sender == admin, "Only admin");
        implementation = newImplementation;
        emit ImplementationUpdated(newImplementation);
    }
}
