//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPackagesFilter_Project tests the project filter on list packages
func TestPackagesFilter_Project(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-project-filter")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish packages with project "e2e-proj"
	publishFromBuiltArtifactsWithProject(t, c, testCtx.FoundryBuiltDir, "proj-filter-a", "1.0.0", "e2e-proj", "Token")
	publishFromBuiltArtifactsWithProject(t, c, testCtx.FoundryBuiltDir, "proj-filter-b", "1.0.0", "e2e-proj", "Token")
	// Publish a package with different project
	publishFromBuiltArtifactsWithProject(t, c, testCtx.FoundryBuiltDir, "proj-filter-other", "1.0.0", "other-proj", "Token")

	u, _ := url.Parse(testCtx.TestServer.URL + "/api/v1/packages")
	q := u.Query()
	q.Set("project", "e2e-proj")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	names := make([]string, len(result.Data))
	for i, p := range result.Data {
		names[i] = p.Name
	}
	assert.Contains(t, names, "proj-filter-a")
	assert.Contains(t, names, "proj-filter-b")
	assert.NotContains(t, names, "proj-filter-other")
}

// TestPackagesFilter_Contract tests the contract filter on list packages
func TestPackagesFilter_Contract(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-contract-filter")
	c := newClient(testCtx.TestServer, apiKey)

	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "contract-filter-token", "1.0.0", "Token")
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "contract-filter-ownable", "1.0.0", "Ownable")

	u, _ := url.Parse(testCtx.TestServer.URL + "/api/v1/packages")
	q := u.Query()
	q.Set("contract", "Token")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	names := make([]string, len(result.Data))
	for i, p := range result.Data {
		names[i] = p.Name
	}
	assert.Contains(t, names, "contract-filter-token")
	assert.NotContains(t, names, "contract-filter-ownable")
}

// TestPackagesFilter_LatestWithoutProject_Returns400 tests that latest without project returns 400
func TestPackagesFilter_LatestWithoutProject_Returns400(t *testing.T) {
	u, _ := url.Parse(testCtx.TestServer.URL + "/api/v1/packages")
	q := u.Query()
	q.Set("latest", "true")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, "INVALID_REQUEST", result.Error.Code)
	assert.Contains(t, result.Error.Message, "latest")
}

// TestPackagesFilter_ContractDetail_IncludesCompilationTargetAndCompiler tests contract detail response
func TestPackagesFilter_ContractDetail_IncludesCompilationTargetAndCompiler(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-contract-detail")
	c := newClient(testCtx.TestServer, apiKey)

	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "contract-detail-test", "1.0.0", "Token")

	contract, err := c.GetContract(context.Background(), "contract-detail-test", "1.0.0", "Token")
	require.NoError(t, err)

	assert.Equal(t, "Token", contract.Name)
	assert.NotEmpty(t, contract.CompilationTarget, "compilationTarget should be present")
	assert.NotNil(t, contract.Compiler, "compiler should be present")
	assert.NotEmpty(t, contract.Compiler.Version, "compiler version should be set")
}
