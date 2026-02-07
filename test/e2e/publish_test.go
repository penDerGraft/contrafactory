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
