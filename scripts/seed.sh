#!/bin/bash
set -e

echo "=== Seeding Contrafactory Database ==="

SERVER=${SERVER:-http://localhost:8080}

# Create a sample package
echo "Publishing sample package..."
curl -s -X POST "$SERVER/api/v1/packages/sample-contracts/1.0.0" \
  -H "Content-Type: application/json" \
  -d '{
    "chain": "evm",
    "builder": "foundry",
    "artifacts": [
      {
        "name": "Token",
        "sourcePath": "src/Token.sol",
        "license": "MIT",
        "abi": [{"type":"function","name":"name","outputs":[{"type":"string"}]}],
        "bytecode": "0x608060405234801561001057600080fd5b50",
        "deployedBytecode": "0x608060405234801561001057600080fd5b50"
      }
    ]
  }'

echo ""
echo "Listing packages..."
curl -s "$SERVER/api/v1/packages" | jq .

echo ""
echo "=== Seeding complete ==="
