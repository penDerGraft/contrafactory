//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pendergraft/contrafactory/pkg/client"
)

// TestPublish_RealFoundryArtifacts tests publishing real Foundry-built artifacts
func TestPublish_RealFoundryArtifacts(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-publish-1")
	c := newClient(testCtx.TestServer, apiKey)

	t.Run("publish Token contract", func(t *testing.T) {
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "my-token", "1.0.0", "Token")

		// Verify package was created
		pkg, err := c.GetPackageVersion(context.Background(), "my-token", "1.0.0")
		require.NoError(t, err)
		assert.Equal(t, "my-token", pkg.Name)
		assert.Equal(t, "1.0.0", pkg.Version)
		assert.Equal(t, "evm", pkg.Chain)
		assert.NotEmpty(t, pkg.CompilerVersion)
	})

	t.Run("publish Ownable contract with constructor args", func(t *testing.T) {
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "my-ownable", "1.0.0", "Ownable")

		// Verify package was created
		pkg, err := c.GetPackageVersion(context.Background(), "my-ownable", "1.0.0")
		require.NoError(t, err)
		assert.Equal(t, "my-ownable", pkg.Name)
		assert.Equal(t, "1.0.0", pkg.Version)
	})

	t.Run("publish multi-contract package", func(t *testing.T) {
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "multi-contract", "1.0.0", "Token", "Ownable")

		// Verify package was created with both contracts
		pkg, err := c.GetPackageVersion(context.Background(), "multi-contract", "1.0.0")
		require.NoError(t, err)
		assert.Len(t, pkg.Contracts, 2)
	})
}

// TestPublish_DuplicateVersionRejected tests that duplicate versions are rejected
func TestPublish_DuplicateVersionRejected(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-duplicate")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish first version
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "dup-test", "1.0.0", "Token")

	// Try to publish same version again
	err := c.Publish(context.Background(), "dup-test", "1.0.0", client.PublishRequest{
		Chain:     "evm",
		Builder:   "foundry",
		Artifacts: []client.Artifact{},
	})

	assertHTTPError(t, err, "VERSION_EXISTS")
}

// TestPublish_MultipleVersions tests publishing multiple versions of the same package
func TestPublish_MultipleVersions(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-versions")
	c := newClient(testCtx.TestServer, apiKey)

	versions := []string{"1.0.0", "1.1.0", "2.0.0-beta.1", "2.0.0"}

	for _, version := range versions {
		t.Run("publish version "+version, func(t *testing.T) {
			publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "versioned-package", version, "Token")
		})
	}

	// Verify all versions are listed
	pkg, err := c.GetPackage(context.Background(), "versioned-package")
	require.NoError(t, err)
	assert.Equal(t, "versioned-package", pkg.Name)
	assert.ElementsMatch(t, versions, pkg.Versions)
}

// TestPublish_ArtifactContent tests that all artifact types are stored correctly
func TestPublish_ArtifactContent(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-artifacts")
	c := newClient(testCtx.TestServer, apiKey)

	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "artifact-test", "1.0.0", "Token")

	t.Run("ABI is stored", func(t *testing.T) {
		abi, err := c.GetABI(context.Background(), "artifact-test", "1.0.0", "Token")
		require.NoError(t, err)

		var abiData []json.RawMessage
		err = json.Unmarshal(abi, &abiData)
		require.NoError(t, err)
		assert.NotEmpty(t, abiData, "ABI should not be empty")

		// Check for standard ERC20 functions
		abiStr := string(abi)
		assert.Contains(t, abiStr, "totalSupply")
		assert.Contains(t, abiStr, "balanceOf")
		assert.Contains(t, abiStr, "transfer")
	})

	t.Run("bytecode is stored", func(t *testing.T) {
		bytecode, err := c.GetBytecode(context.Background(), "artifact-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, bytecode, "Bytecode should not be empty")
		// Bytecode should start with 0x (removed by API) and have reasonable length
		assert.Greater(t, len(bytecode), 100, "Bytecode should be substantial")
	})

	t.Run("deployed bytecode is stored", func(t *testing.T) {
		deployedBytecode, err := c.GetDeployedBytecode(context.Background(), "artifact-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, deployedBytecode, "Deployed bytecode should not be empty")
	})

	t.Run("standard JSON input is stored", func(t *testing.T) {
		t.Skip("Foundry doesn't include standard JSON input in contract JSON files by default")
		stdJson, err := c.GetStandardJSONInput(context.Background(), "artifact-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, stdJson, "Standard JSON input should not be empty")

		var stdJsonData map[string]any
		err = json.Unmarshal(stdJson, &stdJsonData)
		require.NoError(t, err, "Standard JSON input should be valid JSON")
	})

	t.Run("storage layout is stored", func(t *testing.T) {
		storageLayout, err := c.GetStorageLayout(context.Background(), "artifact-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, storageLayout, "Storage layout should not be empty")

		// Foundry returns storage layout as an array
		var layoutData []any
		err = json.Unmarshal(storageLayout, &layoutData)
		require.NoError(t, err, "Storage layout should be valid JSON")
	})
}

// TestPublish_UnauthenticatedWriteRejected tests that writes require authentication
func TestPublish_UnauthenticatedWriteRejected(t *testing.T) {
	c := newClient(testCtx.TestServer, "") // No API key

	err := c.Publish(context.Background(), "unauth-test", "1.0.0", client.PublishRequest{
		Chain:     "evm",
		Builder:   "foundry",
		Artifacts: []client.Artifact{},
	})

	assertHTTPError(t, err, "UNAUTHORIZED")
}

// TestPublish_DependencyContracts tests publishing dependency contracts from lib/
func TestPublish_DependencyContracts(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-deps")
	c := newClient(testCtx.TestServer, apiKey)

	t.Run("publish dependency contract alongside src contract", func(t *testing.T) {
		// Publish both Token (src/) and ProxyAdmin (lib/mock-vendor/contracts/)
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "mixed-package", "1.0.0", "Token", "ProxyAdmin")

		pkg, err := c.GetPackageVersion(context.Background(), "mixed-package", "1.0.0")
		require.NoError(t, err)
		assert.Len(t, pkg.Contracts, 2, "should have both src and dependency contracts names")

		// Get full contract details
		contracts, err := c.ListContracts(context.Background(), "mixed-package", "1.0.0")
		require.NoError(t, err)
		assert.Len(t, contracts, 2, "should have both src and dependency contracts")

		// Find each contract
		var tokenFound, proxyAdminFound bool
		for _, contract := range contracts {
			if contract.Name == "Token" {
				tokenFound = true
				assert.Contains(t, contract.SourcePath, "src/", "Token should be from src/")
			}
			if contract.Name == "ProxyAdmin" {
				proxyAdminFound = true
				assert.Contains(t, contract.SourcePath, "lib/", "ProxyAdmin should be from lib/")
			}
		}
		assert.True(t, tokenFound, "Token contract should be present")
		assert.True(t, proxyAdminFound, "ProxyAdmin contract should be present")
	})

	t.Run("dependency contract has correct source path", func(t *testing.T) {
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "dep-source-path", "1.0.0", "SimpleProxy")

		pkg, err := c.GetPackageVersion(context.Background(), "dep-source-path", "1.0.0")
		require.NoError(t, err)
		require.Len(t, pkg.Contracts, 1)

		// Get full contract details
		contracts, err := c.ListContracts(context.Background(), "dep-source-path", "1.0.0")
		require.NoError(t, err)
		require.Len(t, contracts, 1)

		contract := contracts[0]
		assert.Equal(t, "SimpleProxy", contract.Name)
		// Source path should reference lib/ not src/
		assert.Contains(t, contract.SourcePath, "lib/mock-vendor/contracts/SimpleProxy.sol", "source path should point to lib/")
	})

	t.Run("fetch dependency contract independently", func(t *testing.T) {
		// Publish only a dependency contract
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "proxy-admin-only", "1.0.0", "ProxyAdmin")

		// Fetch it back
		abi, err := c.GetABI(context.Background(), "proxy-admin-only", "1.0.0", "ProxyAdmin")
		require.NoError(t, err)
		assert.NotEmpty(t, abi, "ABI should be present for dependency contract")

		var abiData []json.RawMessage
		err = json.Unmarshal(abi, &abiData)
		require.NoError(t, err)
		assert.NotEmpty(t, abiData, "ABI should not be empty")

		// Verify it has expected ProxyAdmin functions
		abiStr := string(abi)
		assert.Contains(t, abiStr, "transferOwnership")
		assert.Contains(t, abiStr, "owner")
	})

	t.Run("publish src and deps as separate packages", func(t *testing.T) {
		// Publish src contract as one package
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "src-only-pkg", "1.0.0", "Token")

		// Publish dependency contract as separate package
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "dep-only-pkg", "1.0.0", "SimpleProxy")

		// Verify both packages exist independently
		srcPkg, err := c.GetPackageVersion(context.Background(), "src-only-pkg", "1.0.0")
		require.NoError(t, err)
		assert.Len(t, srcPkg.Contracts, 1)
		assert.Equal(t, "Token", srcPkg.Contracts[0])

		depPkg, err := c.GetPackageVersion(context.Background(), "dep-only-pkg", "1.0.0")
		require.NoError(t, err)
		assert.Len(t, depPkg.Contracts, 1)
		assert.Equal(t, "SimpleProxy", depPkg.Contracts[0])
	})

	t.Run("dependency contract bytecode is valid", func(t *testing.T) {
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "dep-bytecode", "1.0.0", "ProxyAdmin")

		bytecode, err := c.GetBytecode(context.Background(), "dep-bytecode", "1.0.0", "ProxyAdmin")
		require.NoError(t, err)
		assert.NotEmpty(t, bytecode, "Bytecode should not be empty for dependency contract")
		assert.Greater(t, len(bytecode), 100, "Bytecode should be substantial")

		deployedBytecode, err := c.GetDeployedBytecode(context.Background(), "dep-bytecode", "1.0.0", "ProxyAdmin")
		require.NoError(t, err)
		assert.NotEmpty(t, deployedBytecode, "Deployed bytecode should not be empty")
	})
}
