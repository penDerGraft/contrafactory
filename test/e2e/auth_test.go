//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pendergraft/contrafactory/pkg/client"
)

// TestAuth_UnauthenticatedRead tests that read endpoints work without authentication
func TestAuth_UnauthenticatedRead(t *testing.T) {
	// First publish a package with an API key
	apiKey := createTestAPIKey(t, testCtx.Store, "test-auth-read")
	authedClient := newClient(testCtx.TestServer, apiKey)
	publishFromBuiltArtifacts(t, authedClient, testCtx.FoundryBuiltDir, "auth-read-test", "1.0.0", "Token")

	// Now test read operations without authentication
	unauthedClient := newClient(testCtx.TestServer, "")

	t.Run("list packages without auth", func(t *testing.T) {
		packages, err := unauthedClient.ListPackages(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, packages.Data)
	})

	t.Run("get package without auth", func(t *testing.T) {
		pkg, err := unauthedClient.GetPackageVersion(context.Background(), "auth-read-test", "1.0.0")
		require.NoError(t, err)
		assert.Equal(t, "auth-read-test", pkg.Name)
	})

	t.Run("get ABI without auth", func(t *testing.T) {
		abi, err := unauthedClient.GetABI(context.Background(), "auth-read-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, abi)
	})

	t.Run("get bytecode without auth", func(t *testing.T) {
		bytecode, err := unauthedClient.GetBytecode(context.Background(), "auth-read-test", "1.0.0", "Token")
		require.NoError(t, err)
		assert.NotEmpty(t, bytecode)
	})

	t.Run("get deployment without auth", func(t *testing.T) {
		// First record a deployment
		err := authedClient.RecordDeployment(context.Background(), client.DeploymentRequest{
			Package:  "auth-read-test",
			Version:  "1.0.0",
			Contract: "Token",
			ChainID:  31337,
			Address:  "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		})
		require.NoError(t, err)

		// Now get it without auth
		deployment, err := unauthedClient.GetDeployment(context.Background(), "31337", "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
		require.NoError(t, err)
		assert.Equal(t, "Token", deployment.ContractName)
	})
}

// TestAuth_UnauthenticatedWriteRejected tests that write operations require authentication
func TestAuth_UnauthenticatedWriteRejected(t *testing.T) {
	unauthedClient := newClient(testCtx.TestServer, "")

	t.Run("publish without auth", func(t *testing.T) {
		err := unauthedClient.Publish(context.Background(), "unauth-write", "1.0.0", client.PublishRequest{
			Chain:     "evm",
			Builder:   "foundry",
			Artifacts: []client.Artifact{},
		})
		assertHTTPError(t, err, "UNAUTHORIZED")
	})

	t.Run("record deployment without auth", func(t *testing.T) {
		err := unauthedClient.RecordDeployment(context.Background(), client.DeploymentRequest{
			Package:  "unauth-deploy",
			Version:  "1.0.0",
			Contract: "Token",
			ChainID:  31337,
			Address:  "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		})
		assertHTTPError(t, err, "UNAUTHORIZED")
	})
}

// TestAuth_ValidAPIKey tests that a valid API key allows write operations
func TestAuth_ValidAPIKey(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-valid-key")
	c := newClient(testCtx.TestServer, apiKey)

	t.Run("publish with valid key", func(t *testing.T) {
		err := c.Publish(context.Background(), "valid-key-test", "1.0.0", client.PublishRequest{
			Chain:     "evm",
			Builder:   "foundry",
			Artifacts: []client.Artifact{},
		})
		// This will fail because we're not providing actual artifacts
		// but we're checking that it's not an auth error
		assert.NotEqual(t, "UNAUTHORIZED", getErrorCode(err))
	})

	t.Run("record deployment with valid key", func(t *testing.T) {
		// First publish something
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "valid-deploy-test", "1.0.0", "Token")

		// Then record deployment
		err := c.RecordDeployment(context.Background(), client.DeploymentRequest{
			Package:  "valid-deploy-test",
			Version:  "1.0.0",
			Contract: "Token",
			ChainID:  31337,
			Address:  "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		})
		assert.NoError(t, err, "Should be able to record deployment with valid key")
	})
}

// TestAuth_InvalidAPIKey tests that an invalid API key is rejected
func TestAuth_InvalidAPIKey(t *testing.T) {
	c := newClient(testCtx.TestServer, "invalid-key-12345")

	t.Run("publish with invalid key", func(t *testing.T) {
		err := c.Publish(context.Background(), "invalid-key-test", "1.0.0", client.PublishRequest{
			Chain:     "evm",
			Builder:   "foundry",
			Artifacts: []client.Artifact{},
		})
		assertHTTPError(t, err, "UNAUTHORIZED")
	})

	t.Run("record deployment with invalid key", func(t *testing.T) {
		err := c.RecordDeployment(context.Background(), client.DeploymentRequest{
			Package:  "invalid-key-deploy",
			Version:  "1.0.0",
			Contract: "Token",
			ChainID:  31337,
			Address:  "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		})
		assertHTTPError(t, err, "UNAUTHORIZED")
	})
}

// TestAuth_PackageOwnership tests that the first publisher owns the package
func TestAuth_PackageOwnership(t *testing.T) {
	apiKey1 := createTestAPIKey(t, testCtx.Store, "owner-1")
	client1 := newClient(testCtx.TestServer, apiKey1)

	// First publisher creates the package
	publishFromBuiltArtifacts(t, client1, testCtx.FoundryBuiltDir, "ownership-test", "1.0.0", "Token")

	// Second user tries to publish to the same package
	apiKey2 := createTestAPIKey(t, testCtx.Store, "owner-2")
	client2 := newClient(testCtx.TestServer, apiKey2)

	err := client2.Publish(context.Background(), "ownership-test", "1.0.1", client.PublishRequest{
		Chain:     "evm",
		Builder:   "foundry",
		Artifacts: []client.Artifact{},
	})
	assertHTTPError(t, err, "FORBIDDEN")

	// First user can still publish new versions
	err = client1.Publish(context.Background(), "ownership-test", "1.0.1", client.PublishRequest{
		Chain:     "evm",
		Builder:   "foundry",
		Artifacts: []client.Artifact{},
	})
	// This might fail due to empty artifacts but shouldn't be FORBIDDEN
	assert.NotEqual(t, "FORBIDDEN", getErrorCode(err))
}

// getErrorCode extracts the error code from an API error
func getErrorCode(err error) string {
	if apiErr, ok := err.(*client.APIError); ok {
		return apiErr.Code
	}
	return ""
}
