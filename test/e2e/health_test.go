//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealth_Endpoints tests all health check endpoints
func TestHealth_Endpoints(t *testing.T) {

	t.Run("/health returns 200", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCtx.TestServer.URL+"/health", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	})

	t.Run("/healthz returns 200", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCtx.TestServer.URL+"/healthz", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	})

	t.Run("/readyz returns 200", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCtx.TestServer.URL+"/readyz", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	})
}

// TestCORS_Headers tests that CORS headers are set correctly
func TestCORS_Headers(t *testing.T) {
	t.Run("OPTIONS request returns CORS headers", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, testCtx.TestServer.URL+"/api/v1/packages", nil)
		req.Header.Set("Origin", "https://example.com")
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "GET")
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST")
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "Authorization")
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "X-API-Key")
	})

	t.Run("GET request has CORS headers", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCtx.TestServer.URL+"/api/v1/packages", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	})
}

// TestListPackages tests the list packages endpoint
func TestListPackages(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-list")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish some packages first
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "list-test-a", "1.0.0", "Token")
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "list-test-b", "1.0.0", "Token")
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "list-test-a", "2.0.0", "Token")

	t.Run("list all packages", func(t *testing.T) {
		resp, err := c.ListPackages(context.Background())
		require.NoError(t, err)

		assert.GreaterOrEqual(t, len(resp.Data), 2, "Should have at least our 2 published packages")
		assert.Equal(t, 20, resp.Pagination.Limit, "Default limit is 20")
	})

	t.Run("list with limit", func(t *testing.T) {
		// Use raw HTTP request to test limit parameter
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCtx.TestServer.URL+"/api/v1/packages?limit=2", nil)
		req.Header.Set("X-API-Key", apiKey)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("list with chain filter", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCtx.TestServer.URL+"/api/v1/packages?chain=evm", nil)
		req.Header.Set("X-API-Key", apiKey)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// TestListContracts tests the list contracts endpoint
func TestListContracts(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-contracts")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish a multi-contract package
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "contracts-test", "1.0.0", "Token", "Ownable")

	t.Run("list contracts in package", func(t *testing.T) {
		contracts, err := c.ListContracts(context.Background(), "contracts-test", "1.0.0")
		require.NoError(t, err)

		assert.Len(t, contracts, 2, "Should have 2 contracts")

		// Check contract names
		names := make([]string, len(contracts))
		for i, c := range contracts {
			names[i] = c.Name
		}
		assert.Contains(t, names, "Token")
		assert.Contains(t, names, "Ownable")
	})

	t.Run("get individual contract details", func(t *testing.T) {
		contract, err := c.GetContract(context.Background(), "contracts-test", "1.0.0", "Token")
		require.NoError(t, err)

		assert.Equal(t, "Token", contract.Name)
		assert.Equal(t, "evm", contract.Chain)
		assert.NotEmpty(t, contract.SourcePath, "SourcePath should not be empty")
	})

	t.Run("get non-existent contract returns 404", func(t *testing.T) {
		_, err := c.GetContract(context.Background(), "contracts-test", "1.0.0", "Nonexistent")
		assertHTTPError(t, err, "NOT_FOUND")
	})
}

// TestGetVersionDeployments tests the version deployments endpoint
func TestGetVersionDeployments(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-deployments")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish a package
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "get-deployments-test", "1.0.0", "Token")

	t.Run("get deployments for package version (empty list)", func(t *testing.T) {
		deployments, err := c.GetVersionDeployments(context.Background(), "get-deployments-test", "1.0.0")
		require.NoError(t, err)

		assert.Empty(t, deployments, "Should be empty before any deployments are recorded")
	})

	t.Run("get deployments for non-existent package returns 404", func(t *testing.T) {
		_, err := c.GetVersionDeployments(context.Background(), "nonexistent", "1.0.0")
		assertHTTPError(t, err, "NOT_FOUND")
	})
}

// TestRequestHeaders tests that the API properly handles various headers
func TestRequestHeaders(t *testing.T) {
	t.Run("Accept header is honored", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCtx.TestServer.URL+"/api/v1/packages", nil)
		req.Header.Set("Accept", "application/json")
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	})

	t.Run("User-Agent is accepted", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCtx.TestServer.URL+"/health", nil)
		req.Header.Set("User-Agent", "contrafactory-test/1.0.0")
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// TestNotFoundPaths tests that non-existent paths return 404
func TestNotFoundPaths(t *testing.T) {
	paths := []string{
		"/api/v1/nonexistent",
		"/api/v1/packages/does-not-exist/1.0.0",
		"/api/v1/deployments/1/0xinvalid",
	}

	for _, path := range paths {
		t.Run("path "+path+" returns 404", func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testCtx.TestServer.URL+path, nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
}
