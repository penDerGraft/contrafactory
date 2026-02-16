package foundry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pendergraft/contrafactory/internal/chains"
)

func TestBuilder_Metadata(t *testing.T) {
	b := New()

	assert.Equal(t, "foundry", b.Name())
	assert.Equal(t, "Foundry", b.DisplayName())
	assert.Equal(t, "evm", b.Chain())
	assert.Equal(t, "foundry.toml", b.ConfigFile())
}

func TestBuilder_Detect(t *testing.T) {
	b := New()

	t.Run("with foundry.toml", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "foundry.toml"), []byte("[profile.default]"), 0644)
		require.NoError(t, err)

		detected, err := b.Detect(dir)
		require.NoError(t, err)
		assert.True(t, detected)
	})

	t.Run("without foundry.toml", func(t *testing.T) {
		dir := t.TempDir()

		detected, err := b.Detect(dir)
		require.NoError(t, err)
		assert.False(t, detected)
	})
}

func TestBuilder_Discover(t *testing.T) {
	b := New()

	t.Run("with artifacts", func(t *testing.T) {
		dir := t.TempDir()
		outDir := filepath.Join(dir, "out")
		buildInfoDir := filepath.Join(outDir, "build-info")

		// Create directory structure
		require.NoError(t, os.MkdirAll(filepath.Join(outDir, "Token.sol"), 0755))
		require.NoError(t, os.MkdirAll(buildInfoDir, 0755))

		// Create artifact with proper metadata (source path must start with src/)
		artifact := map[string]any{
			"abi": []map[string]any{
				{"type": "function", "name": "transfer"},
			},
			"bytecode": map[string]any{
				"object": "0x1234",
			},
			"rawMetadata": `{"settings":{"compilationTarget":{"src/Token.sol":"Token"}}}`,
		}
		artifactBytes, _ := json.Marshal(artifact)
		require.NoError(t, os.WriteFile(filepath.Join(outDir, "Token.sol", "Token.json"), artifactBytes, 0644))

		// Create build-info
		require.NoError(t, os.WriteFile(filepath.Join(buildInfoDir, "abc123.json"), []byte("{}"), 0644))

		paths, err := b.Discover(dir, chains.DiscoverOptions{})
		require.NoError(t, err)
		assert.Len(t, paths, 1)
	})

	t.Run("excludes lib dependencies", func(t *testing.T) {
		dir := t.TempDir()
		outDir := filepath.Join(dir, "out")
		buildInfoDir := filepath.Join(outDir, "build-info")

		// Create directory structure
		require.NoError(t, os.MkdirAll(filepath.Join(outDir, "ERC20.sol"), 0755))
		require.NoError(t, os.MkdirAll(buildInfoDir, 0755))

		// Create artifact from lib/ (should be excluded)
		artifact := map[string]any{
			"abi": []map[string]any{
				{"type": "function", "name": "transfer"},
			},
			"bytecode": map[string]any{
				"object": "0x1234",
			},
			"rawMetadata": `{"settings":{"compilationTarget":{"lib/openzeppelin/ERC20.sol":"ERC20"}}}`,
		}
		artifactBytes, _ := json.Marshal(artifact)
		require.NoError(t, os.WriteFile(filepath.Join(outDir, "ERC20.sol", "ERC20.json"), artifactBytes, 0644))

		// Create build-info
		require.NoError(t, os.WriteFile(filepath.Join(buildInfoDir, "abc123.json"), []byte("{}"), 0644))

		paths, err := b.Discover(dir, chains.DiscoverOptions{})
		require.NoError(t, err)
		assert.Len(t, paths, 0) // Should find nothing since it's from lib/
	})

	t.Run("without build-info", func(t *testing.T) {
		dir := t.TempDir()
		outDir := filepath.Join(dir, "out")
		require.NoError(t, os.MkdirAll(outDir, 0755))

		_, err := b.Discover(dir, chains.DiscoverOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "build-info")
	})
}

func TestBuilder_Parse(t *testing.T) {
	b := New()

	t.Run("valid artifact", func(t *testing.T) {
		dir := t.TempDir()

		artifact := map[string]any{
			"abi": []map[string]any{
				{"type": "function", "name": "transfer"},
			},
			"bytecode": map[string]any{
				"object": "0x608060405234801561001057600080fd5b50",
			},
			"deployedBytecode": map[string]any{
				"object": "0x608060405234801561001057600080fd5b50",
			},
			"rawMetadata": `{"compiler":{"version":"0.8.20"},"settings":{"optimizer":{"enabled":true,"runs":200}}}`,
		}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		result, err := b.Parse(artifactPath)
		require.NoError(t, err)
		assert.Equal(t, "Token", result.Name)
		assert.Equal(t, "evm", result.Chain)
		require.NotNil(t, result.EVM)
		assert.Contains(t, result.EVM.Bytecode, "0x608060")
	})

	t.Run("invalid json", func(t *testing.T) {
		dir := t.TempDir()
		artifactPath := filepath.Join(dir, "Invalid.json")
		require.NoError(t, os.WriteFile(artifactPath, []byte("not json"), 0644))

		_, err := b.Parse(artifactPath)
		require.Error(t, err)
	})

	t.Run("interface (no bytecode)", func(t *testing.T) {
		dir := t.TempDir()

		artifact := map[string]any{
			"abi": []map[string]any{
				{"type": "function", "name": "transfer"},
			},
			"bytecode": map[string]any{
				"object": "",
			},
		}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "IToken.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		_, err := b.Parse(artifactPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no bytecode")
	})
}

func TestExtractContractName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/path/to/Token.json", "Token"},
		{"./out/Token.sol/Token.json", "Token"},
		{"Token.json", "Token"},
		{"/path/to/MyContract.sol/MyContract.json", "MyContract"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractContractName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper for testing - should match the implementation
func extractContractName(path string) string {
	base := filepath.Base(path)
	return base[:len(base)-5] // remove .json
}

func TestBuilder_GetVerificationInput_ReturnsSolcLongVersion(t *testing.T) {
	b := New()

	dir := t.TempDir()
	buildInfoDir := filepath.Join(dir, "out", "build-info")
	require.NoError(t, os.MkdirAll(buildInfoDir, 0755))

	buildInfo := map[string]any{
		"id":              "abc123",
		"solcVersion":     "0.8.28",
		"solcLongVersion": "0.8.28+commit.7893614a",
		"input":           map[string]any{"language": "Solidity", "sources": map[string]any{}},
		"output":          map[string]any{},
	}
	data, err := json.Marshal(buildInfo)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(buildInfoDir, "abc123.json"), data, 0644))

	vi, err := b.GetVerificationInput(dir, "Token")
	require.NoError(t, err)
	assert.Equal(t, "0.8.28+commit.7893614a", vi.SolcLongVersion)
	assert.NotEmpty(t, vi.StandardJSON)
	assert.Contains(t, string(vi.StandardJSON), "Solidity")
}

func TestBuilder_GenerateVerificationInput_DelegatesToGetVerificationInput(t *testing.T) {
	b := New()

	dir := t.TempDir()
	buildInfoDir := filepath.Join(dir, "out", "build-info")
	require.NoError(t, os.MkdirAll(buildInfoDir, 0755))

	stdInput := map[string]any{"language": "Solidity", "sources": map[string]any{"Token.sol": map[string]any{"content": "contract Token {}"}}}
	buildInfo := map[string]any{
		"id":              "xyz789",
		"solcVersion":     "0.8.20",
		"solcLongVersion": "0.8.20+commit.a1b2c3d4",
		"input":           stdInput,
		"output":          map[string]any{},
	}
	data, err := json.Marshal(buildInfo)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(buildInfoDir, "xyz789.json"), data, 0644))

	genOut, err := b.GenerateVerificationInput(dir, "Token")
	require.NoError(t, err)

	vi, err := b.GetVerificationInput(dir, "Token")
	require.NoError(t, err)

	assert.Equal(t, vi.StandardJSON, genOut)
}
