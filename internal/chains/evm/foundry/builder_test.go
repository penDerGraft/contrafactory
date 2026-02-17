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

	t.Run("excludes by source path", func(t *testing.T) {
		dir := t.TempDir()
		outDir := filepath.Join(dir, "out")
		buildInfoDir := filepath.Join(outDir, "build-info")

		makeArtifact := func(sourcePath string) []byte {
			artifact := map[string]any{
				"abi":         []map[string]any{{"type": "function", "name": "transfer"}},
				"bytecode":    map[string]any{"object": "0x1234"},
				"rawMetadata": `{"settings":{"compilationTarget":{"` + sourcePath + `":"MetaCoin"}}}`,
			}
			b, _ := json.Marshal(artifact)
			return b
		}

		// Create two MetaCoin artifacts at different paths
		require.NoError(t, os.MkdirAll(filepath.Join(outDir, "examples", "MetaCoin.sol"), 0755))
		require.NoError(t, os.MkdirAll(filepath.Join(outDir, "examples", "inheritance", "MetaCoin.sol"), 0755))
		require.NoError(t, os.MkdirAll(filepath.Join(outDir, "examples", "proxy", "MetaCoin.sol"), 0755))
		require.NoError(t, os.MkdirAll(buildInfoDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(outDir, "examples", "MetaCoin.sol", "MetaCoin.json"), makeArtifact("src/examples/MetaCoin.sol"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(outDir, "examples", "inheritance", "MetaCoin.sol", "MetaCoin.json"), makeArtifact("src/examples/inheritance/MetaCoin.sol"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(outDir, "examples", "proxy", "MetaCoin.sol", "MetaCoin.json"), makeArtifact("src/examples/proxy/MetaCoin.sol"), 0644))

		require.NoError(t, os.WriteFile(filepath.Join(buildInfoDir, "abc.json"), []byte("{}"), 0644))

		// Without ExcludePaths: returns first MetaCoin found (order depends on Walk)
		paths, err := b.Discover(dir, chains.DiscoverOptions{})
		require.NoError(t, err)
		assert.Len(t, paths, 1)

		// With ExcludePaths: exclude proxy and root, keep only inheritance
		paths, err = b.Discover(dir, chains.DiscoverOptions{
			ExcludePaths: []string{"proxy", "examples/MetaCoin.sol"},
		})
		require.NoError(t, err)
		assert.Len(t, paths, 1)
		// Verify we got the inheritance one
		artifact, err := b.Parse(paths[0])
		require.NoError(t, err)
		assert.Equal(t, "src/examples/inheritance/MetaCoin.sol", artifact.EVM.SourcePath)
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

	vi, err := b.GetVerificationInput(dir, "Token", "")
	require.NoError(t, err)
	assert.Equal(t, "0.8.28+commit.7893614a", vi.SolcLongVersion)
	assert.NotEmpty(t, vi.StandardJSON)
	assert.Contains(t, string(vi.StandardJSON), "Solidity")
}

func TestBuilder_GetVerificationInput_MatchesBySourcePath(t *testing.T) {
	b := New()

	dir := t.TempDir()
	buildInfoDir := filepath.Join(dir, "out", "build-info")
	require.NoError(t, os.MkdirAll(buildInfoDir, 0755))

	// Build-info for src/examples/MetaCoin.sol (wrong one)
	buildInfoRoot := map[string]any{
		"id":              "root",
		"solcLongVersion": "0.8.28+commit.aaa",
		"input":           map[string]any{"language": "Solidity", "sources": map[string]any{"src/examples/MetaCoin.sol": map[string]any{"content": "contract MetaCoin {}"}}, "settings": map[string]any{}},
		"output": map[string]any{
			"contracts": map[string]any{
				"src/examples/MetaCoin.sol": map[string]any{"MetaCoin": map[string]any{}},
			},
		},
	}
	dataRoot, _ := json.Marshal(buildInfoRoot)
	require.NoError(t, os.WriteFile(filepath.Join(buildInfoDir, "root.json"), dataRoot, 0644))

	// Build-info for src/examples/inheritance/MetaCoin.sol (correct one)
	buildInfoInheritance := map[string]any{
		"id":              "inheritance",
		"solcLongVersion": "0.8.28+commit.bbb",
		"input":           map[string]any{"language": "Solidity", "sources": map[string]any{"src/examples/inheritance/MetaCoin.sol": map[string]any{"content": "contract MetaCoin {}"}}, "settings": map[string]any{}},
		"output": map[string]any{
			"contracts": map[string]any{
				"src/examples/inheritance/MetaCoin.sol": map[string]any{"MetaCoin": map[string]any{}},
			},
		},
	}
	dataInheritance, _ := json.Marshal(buildInfoInheritance)
	require.NoError(t, os.WriteFile(filepath.Join(buildInfoDir, "inheritance.json"), dataInheritance, 0644))

	// Without sourcePath: returns first build-info found
	vi, err := b.GetVerificationInput(dir, "MetaCoin", "")
	require.NoError(t, err)
	assert.NotEmpty(t, vi.StandardJSON)
	assert.NotEmpty(t, vi.SolcLongVersion)

	// With sourcePath: returns matching build-info (inheritance), not root
	vi, err = b.GetVerificationInput(dir, "MetaCoin", "src/examples/inheritance/MetaCoin.sol")
	require.NoError(t, err)
	assert.Contains(t, string(vi.StandardJSON), "src/examples/inheritance/MetaCoin.sol")
	assert.Equal(t, "0.8.28+commit.bbb", vi.SolcLongVersion)
}

func TestStripFoundryStandardJSONKeys(t *testing.T) {
	input := json.RawMessage(`{"language":"Solidity","sources":{},"settings":{},"allowPaths":["."],"basePath":".","includePaths":[],"version":"hh-sol-build-info-1"}`)
	out, err := stripFoundryStandardJSONKeys(input)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Contains(t, m, "language")
	assert.Contains(t, m, "sources")
	assert.Contains(t, m, "settings")
	assert.NotContains(t, m, "allowPaths")
	assert.NotContains(t, m, "basePath")
	assert.NotContains(t, m, "includePaths")
	assert.NotContains(t, m, "version")
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

	vi, err := b.GetVerificationInput(dir, "Token", "")
	require.NoError(t, err)

	assert.Equal(t, vi.StandardJSON, genOut)
}

func TestBuilder_GeneratePerContractStandardJSON(t *testing.T) {
	b := New()

	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))

		// Create source files
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Token.sol"), []byte("contract Token {}"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Helper.sol"), []byte("contract Helper {}"), 0644))

		// Create artifact with rawMetadata containing sources
		rawMetadata := `{
			"compiler":{"version":"0.8.20"},
			"language":"Solidity",
			"sources":{
				"src/Token.sol":{"keccak256":"0x123","license":"MIT"},
				"src/Helper.sol":{"keccak256":"0x456","license":"MIT"}
			},
			"settings":{
				"compilationTarget":{"src/Token.sol":"Token"},
				"evmVersion":"paris",
				"optimizer":{"enabled":true,"runs":200},
				"viaIR":false
			}
		}`
		artifact := map[string]any{
			"abi":         []map[string]any{},
			"bytecode":    map[string]any{"object": "0x1234"},
			"rawMetadata": rawMetadata,
		}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		out, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(out, &result))
		assert.Equal(t, "Solidity", result["language"])
		sources, ok := result["sources"].(map[string]any)
		require.True(t, ok)
		assert.Len(t, sources, 2)
		assert.Contains(t, sources, "src/Token.sol")
		assert.Contains(t, sources, "src/Helper.sol")
		settings, ok := result["settings"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "paris", settings["evmVersion"])
	})

	t.Run("missing source file", func(t *testing.T) {
		dir := t.TempDir()
		rawMetadata := `{"sources":{"src/Missing.sol":{}},"settings":{"compilationTarget":{"src/Missing.sol":"Missing"}}}`
		artifact := map[string]any{"bytecode": map[string]any{"object": "0x1234"}, "rawMetadata": rawMetadata}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Missing.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		_, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "src/Missing.sol")
	})

	t.Run("empty rawMetadata", func(t *testing.T) {
		dir := t.TempDir()
		artifact := map[string]any{"bytecode": map[string]any{"object": "0x1234"}, "rawMetadata": ""}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		_, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no rawMetadata")
	})

	t.Run("invalid rawMetadata JSON", func(t *testing.T) {
		dir := t.TempDir()
		artifact := map[string]any{"bytecode": map[string]any{"object": "0x1234"}, "rawMetadata": "not valid json {"}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		_, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parsing rawMetadata")
	})

	t.Run("empty metadata.Sources", func(t *testing.T) {
		dir := t.TempDir()
		rawMetadata := `{"sources":{},"settings":{"compilationTarget":{"src/Token.sol":"Token"}}}`
		artifact := map[string]any{"bytecode": map[string]any{"object": "0x1234"}, "rawMetadata": rawMetadata}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		_, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no sources")
	})

	t.Run("libraries round-trip", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Token.sol"), []byte("contract Token {}"), 0644))

		rawMetadata := `{
			"sources":{"src/Token.sol":{}},
			"settings":{
				"compilationTarget":{"src/Token.sol":"Token"},
				"libraries":{"src/Lib.sol":{"MyLib":"0x1234567890123456789012345678901234567890"}}
			}
		}`
		artifact := map[string]any{"bytecode": map[string]any{"object": "0x1234"}, "rawMetadata": rawMetadata}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		out, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(out, &result))
		settings := result["settings"].(map[string]any)
		libs, ok := settings["libraries"].(map[string]any)
		require.True(t, ok)
		libFile, ok := libs["src/Lib.sol"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "0x1234567890123456789012345678901234567890", libFile["MyLib"])
	})

	t.Run("remappings transferred", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Token.sol"), []byte("contract Token {}"), 0644))

		rawMetadata := `{
			"sources":{"src/Token.sol":{}},
			"settings":{
				"compilationTarget":{"src/Token.sol":"Token"},
				"remappings":["@openzeppelin/=lib/openzeppelin/"]
			}
		}`
		artifact := map[string]any{"bytecode": map[string]any{"object": "0x1234"}, "rawMetadata": rawMetadata}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		out, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(out, &result))
		settings := result["settings"].(map[string]any)
		remappings, ok := settings["remappings"].([]any)
		require.True(t, ok)
		assert.Equal(t, "@openzeppelin/=lib/openzeppelin/", remappings[0])
	})

	t.Run("settings.metadata absent defaults to ipfs", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Token.sol"), []byte("contract Token {}"), 0644))

		rawMetadata := `{"sources":{"src/Token.sol":{}},"settings":{"compilationTarget":{"src/Token.sol":"Token"}}}`
		artifact := map[string]any{"bytecode": map[string]any{"object": "0x1234"}, "rawMetadata": rawMetadata}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		out, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(out, &result))
		settings := result["settings"].(map[string]any)
		meta, ok := settings["metadata"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "ipfs", meta["bytecodeHash"])
	})

	t.Run("settings.metadata present preserves fields", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Token.sol"), []byte("contract Token {}"), 0644))

		rawMetadata := `{
			"sources":{"src/Token.sol":{}},
			"settings":{
				"compilationTarget":{"src/Token.sol":"Token"},
				"metadata":{"bytecodeHash":"bzzr1","useLiteralContent":true}
			}
		}`
		artifact := map[string]any{"bytecode": map[string]any{"object": "0x1234"}, "rawMetadata": rawMetadata}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		out, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(out, &result))
		settings := result["settings"].(map[string]any)
		meta, ok := settings["metadata"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "bzzr1", meta["bytecodeHash"])
		assert.Equal(t, true, meta["useLiteralContent"])
	})

	t.Run("metadata.Language used when set", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "Token.sol"), []byte("contract Token {}"), 0644))

		rawMetadata := `{"language":"Yul","sources":{"src/Token.sol":{}},"settings":{"compilationTarget":{"src/Token.sol":"Token"}}}`
		artifact := map[string]any{"bytecode": map[string]any{"object": "0x1234"}, "rawMetadata": rawMetadata}
		artifactBytes, _ := json.Marshal(artifact)
		artifactPath := filepath.Join(dir, "Token.json")
		require.NoError(t, os.WriteFile(artifactPath, artifactBytes, 0644))

		out, err := b.GeneratePerContractStandardJSON(dir, artifactPath)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(out, &result))
		assert.Equal(t, "Yul", result["language"])
	})
}
