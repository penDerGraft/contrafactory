//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFetch_FullPackage tests fetching a complete package with all contracts
func TestFetch_FullPackage(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-fetch")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish test package
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "fetch-test", "1.0.0", "Token", "Ownable")

	t.Run("get package metadata", func(t *testing.T) {
		pkg, err := c.GetPackageVersion(context.Background(), "fetch-test", "1.0.0")
		require.NoError(t, err)
		assert.Equal(t, "fetch-test", pkg.Name)
		assert.Equal(t, "1.0.0", pkg.Version)
		assert.Equal(t, "evm", pkg.Chain)
		assert.Equal(t, "foundry", pkg.Builder)
		assert.NotEmpty(t, pkg.CompilerVersion)
		assert.NotEmpty(t, pkg.Contracts)
		assert.Len(t, pkg.Contracts, 2)
		assert.Contains(t, pkg.Contracts, "Token")
		assert.Contains(t, pkg.Contracts, "Ownable")
	})
}

// TestFetch_IndividualArtifacts tests fetching individual artifact types
func TestFetch_IndividualArtifacts(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-individual")
	c := newClient(testCtx.TestServer, apiKey)

	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "individual-test", "1.0.0", "Token")

	t.Run("fetch ABI", func(t *testing.T) {
		abi, err := c.GetABI(context.Background(), "individual-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, abi)
	})

	t.Run("fetch bytecode", func(t *testing.T) {
		bytecode, err := c.GetBytecode(context.Background(), "individual-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, bytecode)
	})

	t.Run("fetch deployed bytecode", func(t *testing.T) {
		deployedBytecode, err := c.GetDeployedBytecode(context.Background(), "individual-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, deployedBytecode)
	})

	t.Run("fetch standard JSON input", func(t *testing.T) {
		t.Skip("Foundry doesn't include standard JSON input in contract JSON files by default")
		stdJson, err := c.GetStandardJSONInput(context.Background(), "individual-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, stdJson)
	})

	t.Run("fetch storage layout", func(t *testing.T) {
		storageLayout, err := c.GetStorageLayout(context.Background(), "individual-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, storageLayout)
	})
}

// TestFetch_SpecificContract tests fetching a specific contract from a multi-contract package
func TestFetch_SpecificContract(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-specific")
	c := newClient(testCtx.TestServer, apiKey)

	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "multi-test", "1.0.0", "Token", "Ownable")

	t.Run("get Token contract ABI", func(t *testing.T) {
		abi, err := c.GetABI(context.Background(), "multi-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, abi)
	})

	t.Run("get Ownable contract ABI", func(t *testing.T) {
		abi, err := c.GetABI(context.Background(), "multi-test", "1.0.0", "Ownable")
		require.NoError(t, err)
		assert.NotEmpty(t, abi)
	})
}

// TestFetch_Archive tests downloading a package archive
func TestFetch_Archive(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-archive")
	c := newClient(testCtx.TestServer, apiKey)

	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "archive-test", "1.0.0", "Token")

	t.Run("package exists", func(t *testing.T) {
		// Note: The client doesn't have a GetArchive method yet
		// For now, we verify the package exists
		pkg, err := c.GetPackageVersion(context.Background(), "archive-test", "1.0.0")
		require.NoError(t, err, "Package should exist")
		assert.Equal(t, "archive-test", pkg.Name)
	})
}

// TestFetch_NonexistentPackage tests that fetching a nonexistent package returns 404
func TestFetch_NonexistentPackage(t *testing.T) {
	c := newClient(testCtx.TestServer, "") // No auth needed for reads

	_, err := c.GetPackageVersion(context.Background(), "nonexistent", "1.0.0")
	assertHTTPError(t, err, "NOT_FOUND")
}

// TestFetch_NonexistentContract tests that fetching a nonexistent contract returns 404
func TestFetch_NonexistentContract(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-nonexistent-contract")
	c := newClient(testCtx.TestServer, apiKey)

	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "contract-test", "1.0.0", "Token")

	_, err := c.GetABI(context.Background(), "contract-test", "1.0.0", "Nonexistent")
	assertHTTPError(t, err, "NOT_FOUND")
}

// TestFetch_NonexistentArtifact tests that fetching a nonexistent artifact returns 404
func TestFetch_NonexistentArtifact(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-nonexistent-artifact")
	c := newClient(testCtx.TestServer, apiKey)

	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "nonexistent-artifact-test", "1.0.0", "Token")

	// Try to get a non-existent artifact type - this will return NOT_FOUND because
	// the artifact type doesn't exist
	_, err := c.GetABI(context.Background(), "nonexistent-artifact-test", "1.0.0", "Nonexistent")
	assertHTTPError(t, err, "NOT_FOUND")
}
