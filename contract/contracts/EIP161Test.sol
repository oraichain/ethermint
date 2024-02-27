// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

contract EIP161Test {
    function selfDestructTo(address target) public {
        // This contract will self-destruct and send its balance to the target address
        selfdestruct(payable(target));
    }

    function callAccount(address target) public payable returns (bytes memory) {
        (bool success, bytes memory data) = target.call{value: msg.value}("");
        require(success, "Failed to call empty account with value");

        return data;
    }
}
