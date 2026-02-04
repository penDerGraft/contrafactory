// Package foundry provides the Foundry builder for EVM contracts.
package foundry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pendergraft/contrafactory/internal/chains"
)

// Builder implements chains.Builder for Foundry projects
type Builder struct{}

// New creates a new Foundry builder
func New() *Builder {
	return &Builder{}
}

// Name returns the builder identifier
func (b *Builder) Name() string {
	return "foundry"
}

// DisplayName returns a human-readable name
func (b *Builder) DisplayName() string {
	return "Foundry"
}

// Chain returns the chain this builder targets
func (b *Builder) Chain() string {
	return "evm"
}

// ConfigFile returns the config file name
func (b *Builder) ConfigFile() string {
	return "foundry.toml"
}

// Detect checks if a directory is a Foundry project
func (b *Builder) Detect(dir string) (bool, error) {
	configPath := filepath.Join(dir, b.ConfigFile())
	_, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Discover finds all contract artifacts in a Foundry project
func (b *Builder) Discover(dir string, opts chains.DiscoverOptions) ([]string, error) {
	outDir := filepath.Join(dir, "out")

	// Check if out directory exists
	if _, err := os.Stat(outDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("out directory not found - run 'forge build' first")
	}

	// Check for build-info directory
	buildInfoDir := filepath.Join(outDir, "build-info")
	if _, err := os.Stat(buildInfoDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("build-info directory not found - run 'forge build --build-info' first")
	}

	var artifacts []string
	seen := make(map[string]bool) // Track seen contract names to avoid duplicates

	// Walk the out directory
	err := filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-JSON files
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}

		// Skip build-info files
		if strings.Contains(path, "build-info") {
			return nil
		}

		// Get contract name from path (out/{Source}.sol/{Contract}.json)
		parentDir := filepath.Dir(path)
		if !strings.HasSuffix(parentDir, ".sol") {
			return nil
		}

		contractName := strings.TrimSuffix(info.Name(), ".json")

		// Skip if we've already seen this contract name
		if seen[contractName] {
			return nil
		}

		// Check if this contract should be included (explicit list)
		if len(opts.Contracts) > 0 {
			included := false
			for _, c := range opts.Contracts {
				if c == contractName {
					included = true
					break
				}
			}
			if !included {
				return nil
			}
		}

		// Check if this contract should be excluded by pattern
		for _, pattern := range opts.Exclude {
			// Check suffix match (e.g., "Test" matches "MyContractTest")
			if strings.HasSuffix(contractName, pattern) {
				return nil
			}
			// Check prefix match (e.g., "Mock" matches "MockToken")
			if strings.HasPrefix(contractName, pattern) {
				return nil
			}
			// Check glob pattern match
			matched, _ := filepath.Match(pattern, contractName)
			if matched {
				return nil
			}
		}

		// Read the artifact to check its source path
		sourcePath, err := b.getArtifactSourcePath(path)
		if err != nil {
			return nil // Skip artifacts we can't read
		}

		// Only include contracts from src/ directory (not lib/, test/, script/)
		if !strings.HasPrefix(sourcePath, "src/") {
			return nil
		}

		seen[contractName] = true
		artifacts = append(artifacts, path)
		return nil
	})

	return artifacts, err
}

// getArtifactSourcePath reads an artifact and returns its source path
func (b *Builder) getArtifactSourcePath(artifactPath string) (string, error) {
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return "", err
	}

	var raw FoundryArtifact
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", err
	}

	// Parse metadata to get source path
	if raw.RawMetadata == "" {
		return "", fmt.Errorf("no metadata")
	}

	var metadata FoundryMetadata
	if err := json.Unmarshal([]byte(raw.RawMetadata), &metadata); err != nil {
		return "", err
	}

	return getFirstKey(metadata.Settings.CompilationTarget), nil
}

// Parse parses a Foundry artifact file
func (b *Builder) Parse(artifactPath string) (*chains.Artifact, error) {
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("reading artifact: %w", err)
	}

	var raw FoundryArtifact
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing artifact JSON: %w", err)
	}

	// Skip if no bytecode (interfaces, libraries without code)
	if raw.Bytecode.Object == "" || raw.Bytecode.Object == "0x" {
		return nil, fmt.Errorf("contract has no bytecode (likely an interface)")
	}

	// Parse metadata
	var metadata FoundryMetadata
	if raw.RawMetadata != "" {
		if err := json.Unmarshal([]byte(raw.RawMetadata), &metadata); err != nil {
			// Non-fatal, continue without metadata
		}
	}

	// Extract contract name from path
	contractName := strings.TrimSuffix(filepath.Base(artifactPath), ".json")

	// Build the artifact
	artifact := &chains.Artifact{
		Name:  contractName,
		Chain: "evm",
		EVM: &chains.EVMArtifact{
			SourcePath:       getFirstKey(metadata.Settings.CompilationTarget),
			License:          metadata.Sources.FirstLicense(),
			ABI:              raw.ABI,
			Bytecode:         raw.Bytecode.Object,
			DeployedBytecode: raw.DeployedBytecode.Object,
			Compiler: chains.EVMCompiler{
				Version:    metadata.Compiler.Version,
				EVMVersion: metadata.Settings.EVMVersion,
				ViaIR:      metadata.Settings.ViaIR,
				Optimizer: chains.OptimizerConfig{
					Enabled: metadata.Settings.Optimizer.Enabled,
					Runs:    metadata.Settings.Optimizer.Runs,
				},
			},
		},
	}

	return artifact, nil
}

// GenerateVerificationInput extracts Standard JSON Input from build-info
func (b *Builder) GenerateVerificationInput(dir string, contractName string) ([]byte, error) {
	buildInfoDir := filepath.Join(dir, "out", "build-info")

	// Find build-info files
	entries, err := os.ReadDir(buildInfoDir)
	if err != nil {
		return nil, fmt.Errorf("reading build-info directory: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(buildInfoDir, entry.Name()))
		if err != nil {
			continue
		}

		var buildInfo BuildInfo
		if err := json.Unmarshal(data, &buildInfo); err != nil {
			continue
		}

		// Return the input field which is the Standard JSON Input
		return json.Marshal(buildInfo.Input)
	}

	return nil, fmt.Errorf("build-info not found for contract %s", contractName)
}

// FoundryArtifact represents the structure of a Foundry artifact JSON file
type FoundryArtifact struct {
	ABI              json.RawMessage `json:"abi"`
	Bytecode         BytecodeObject  `json:"bytecode"`
	DeployedBytecode BytecodeObject  `json:"deployedBytecode"`
	RawMetadata      string          `json:"rawMetadata"`
	Metadata         json.RawMessage `json:"metadata"`
}

// BytecodeObject represents bytecode in a Foundry artifact
type BytecodeObject struct {
	Object         string                       `json:"object"`
	SourceMap      string                       `json:"sourceMap"`
	LinkReferences map[string]map[string][]Link `json:"linkReferences"`
}

// Link represents a library link reference
type Link struct {
	Start  int `json:"start"`
	Length int `json:"length"`
}

// FoundryMetadata represents the parsed rawMetadata field
type FoundryMetadata struct {
	Compiler CompilerMeta `json:"compiler"`
	Language string       `json:"language"`
	Output   OutputMeta   `json:"output"`
	Settings SettingsMeta `json:"settings"`
	Sources  SourcesMeta  `json:"sources"`
	Version  int          `json:"version"`
}

// CompilerMeta contains compiler information
type CompilerMeta struct {
	Version string `json:"version"`
}

// OutputMeta contains output information
type OutputMeta struct {
	ABI     json.RawMessage `json:"abi"`
	Devdoc  json.RawMessage `json:"devdoc"`
	Userdoc json.RawMessage `json:"userdoc"`
}

// SettingsMeta contains compiler settings
type SettingsMeta struct {
	CompilationTarget map[string]string `json:"compilationTarget"`
	EVMVersion        string            `json:"evmVersion"`
	Libraries         map[string]string `json:"libraries"`
	Optimizer         OptimizerMeta     `json:"optimizer"`
	Remappings        []string          `json:"remappings"`
	ViaIR             bool              `json:"viaIR"`
}

// getFirstKey returns the first key from a map
func getFirstKey(m map[string]string) string {
	for k := range m {
		return k
	}
	return ""
}

// OptimizerMeta contains optimizer settings
type OptimizerMeta struct {
	Enabled bool `json:"enabled"`
	Runs    int  `json:"runs"`
}

// SourcesMeta contains source file information
type SourcesMeta map[string]SourceMeta

// SourceMeta contains individual source file info
type SourceMeta struct {
	Keccak256 string   `json:"keccak256"`
	License   string   `json:"license"`
	URLs      []string `json:"urls"`
}

// FirstLicense returns the first license found in sources
func (s SourcesMeta) FirstLicense() string {
	for _, src := range s {
		if src.License != "" {
			return src.License
		}
	}
	return ""
}

// BuildInfo represents a Foundry build-info file
type BuildInfo struct {
	ID     string          `json:"id"`
	Input  json.RawMessage `json:"input"`  // Standard JSON Input
	Output json.RawMessage `json:"output"` // Compilation output
}
