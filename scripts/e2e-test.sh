#!/bin/bash
set -e

echo "=== Contrafactory E2E Tests ==="

# Start server in background
./bin/contrafactory-server &
SERVER_PID=$!
trap "kill $SERVER_PID 2>/dev/null || true" EXIT

# Wait for server to be ready
echo "Waiting for server to start..."
for i in {1..30}; do
    if curl -s http://localhost:8080/health > /dev/null 2>&1; then
        echo "Server is ready"
        break
    fi
    sleep 1
done

# Check health
echo "Checking health endpoint..."
curl -s http://localhost:8080/health | grep -q "ok" || (echo "Health check failed" && exit 1)
echo "Health check passed"

# List packages (should be empty)
echo "Listing packages (should be empty)..."
PACKAGES=$(curl -s http://localhost:8080/api/v1/packages)
echo "$PACKAGES"

# TODO: Add more E2E tests once we have a sample Foundry project
# - Publish a package
# - List packages (should have one)
# - Fetch package details
# - Fetch artifacts
# - Record a deployment
# - Verify deployment

echo ""
echo "=== All E2E tests passed ==="
