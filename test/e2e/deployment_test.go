//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pendergraft/contrafactory/pkg/client"
)

// TestDeployment_RecordDeployment tests recording a deployment
func TestDeployment_RecordDeployment(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-deployment")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish package first
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "deploy-test", "1.0.0", "Token")

	t.Run("record deployment", func(t *testing.T) {
		req := client.DeploymentRequest{
			Package:     "deploy-test",
			Version:     "1.0.0",
			Contract:    "Token",
			ChainID:     31337,
			Address:     "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			TxHash:      "0x" + "abcd1234",
			BlockNumber: 12345,
		}

		err := c.RecordDeployment(context.Background(), req)
		require.NoError(t, err, "Failed to record deployment")
	})

	t.Run("get deployment by address", func(t *testing.T) {
		deployment, err := c.GetDeployment(context.Background(), "31337", "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
		require.NoError(t, err)
		assert.NotEmpty(t, deployment.PackageID, "PackageID should be set")
		assert.Equal(t, "Token", deployment.ContractName)
		assert.Equal(t, "31337", deployment.ChainID)
		assert.Equal(t, "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", deployment.Address)
		assert.Equal(t, int64(12345), deployment.BlockNumber)
		assert.NotNil(t, deployment.VerifiedOn, "VerifiedOn should be present (may be empty for unverified deployments)")
	})
}

// TestDeployment_ListDeployments tests listing deployments for a package
func TestDeployment_ListDeployments(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-list-deployments")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish package
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "list-deploy-test", "1.0.0", "Token", "Ownable")

	// Record multiple deployments
	deployments := []client.DeploymentRequest{
		{
			Package:     "list-deploy-test",
			Version:     "1.0.0",
			Contract:    "Token",
			ChainID:     31337,
			Address:     "0x0000000000000000000000000000000000000001",
			TxHash:      "0x" + "aaaa1111",
			BlockNumber: 100,
		},
		{
			Package:     "list-deploy-test",
			Version:     "1.0.0",
			Contract:    "Token",
			ChainID:     31337,
			Address:     "0x0000000000000000000000000000000000000002",
			TxHash:      "0x" + "bbbb2222",
			BlockNumber: 200,
		},
		{
			Package:     "list-deploy-test",
			Version:     "1.0.0",
			Contract:    "Ownable",
			ChainID:     31337,
			Address:     "0x0000000000000000000000000000000000000003",
			TxHash:      "0x" + "cccc3333",
			BlockNumber: 300,
		},
	}

	for _, dep := range deployments {
		err := c.RecordDeployment(context.Background(), dep)
		require.NoError(t, err)
	}

	t.Run("get deployment by different addresses", func(t *testing.T) {
		dep1, err := c.GetDeployment(context.Background(), "31337", "0x0000000000000000000000000000000000000001")
		require.NoError(t, err)
		assert.Equal(t, "Token", dep1.ContractName)

		dep2, err := c.GetDeployment(context.Background(), "31337", "0x0000000000000000000000000000000000000003")
		require.NoError(t, err)
		assert.Equal(t, "Ownable", dep2.ContractName)
	})
}

// TestDeployment_ConstructorArgs tests recording deployment with constructor arguments
func TestDeployment_ConstructorArgs(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-constructor")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish Ownable which has constructor args
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "constructor-test", "1.0.0", "Ownable")

	t.Run("record deployment with constructor args", func(t *testing.T) {
		// Constructor args for Ownable(address): 0x0000000000000000000000000000000000000001
		// ABI-encoded: 32 bytes left-padded
		constructorArgs := "0000000000000000000000000000000000000000000000000000000000000001"

		req := client.DeploymentRequest{
			Package:         "constructor-test",
			Version:         "1.0.0",
			Contract:        "Ownable",
			ChainID:         31337,
			Address:         "0x0000000000000000000000000000000000000001",
			ConstructorArgs: constructorArgs,
			TxHash:          "0x" + "const4444",
			BlockNumber:     400,
		}

		err := c.RecordDeployment(context.Background(), req)
		require.NoError(t, err)

		// Verify deployment was recorded
		deployment, err := c.GetDeployment(context.Background(), "31337", "0x0000000000000000000000000000000000000001")
		require.NoError(t, err)
		assert.Equal(t, "Ownable", deployment.ContractName)
	})
}

// TestDeployment_UnauthenticatedWriteRejected tests that deployment recording requires authentication
func TestDeployment_UnauthenticatedWriteRejected(t *testing.T) {
	c := newClient(testCtx.TestServer, "") // No API key

	req := client.DeploymentRequest{
		Package:  "unauth-deploy",
		Version:  "1.0.0",
		Contract: "Token",
		ChainID:  31337,
		Address:  "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
	}

	err := c.RecordDeployment(context.Background(), req)
	assertHTTPError(t, err, "UNAUTHORIZED")
}

// TestDeployment_GetNonexistent tests that getting a nonexistent deployment returns 404
func TestDeployment_GetNonexistent(t *testing.T) {
	c := newClient(testCtx.TestServer, "")

	_, err := c.GetDeployment(context.Background(), "31337", "0xdeaddeaddeaddeaddeaddeaddeaddeaddeaddead")
	assertHTTPError(t, err, "NOT_FOUND")
}
