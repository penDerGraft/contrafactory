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

		// Check if this source path should be excluded
		for _, pattern := range opts.ExcludePaths {
			if strings.Contains(sourcePath, pattern) {
				return nil
			}
			if matched, _ := filepath.Match(pattern, sourcePath); matched {
				return nil
			}
		}

		// Only include contracts from src/ directory, unless explicitly listed as a dependency
		if !strings.HasPrefix(sourcePath, "src/") {
			if !isIncludedDependency(contractName, opts.IncludeDependencies) {
				return nil
			}
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
		_ = json.Unmarshal([]byte(raw.RawMetadata), &metadata) // Non-fatal, continue without metadata
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
			StorageLayout:    raw.StorageLayout,
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
	vi, err := b.GetVerificationInput(dir, contractName, "")
	if err != nil {
		return nil, err
	}
	return vi.StandardJSON, nil
}

// buildInfoOutputContracts represents output.contracts from Solidity compiler output
type buildInfoOutputContracts map[string]map[string]json.RawMessage

// GetVerificationInput extracts Standard JSON Input and full solc version from build-info.
// When sourcePath is non-empty, finds the build-info whose output contains contracts[sourcePath][contractName].
// When sourcePath is empty, returns the first valid build-info (legacy behavior).
func (b *Builder) GetVerificationInput(dir string, contractName string, sourcePath string) (*chains.VerificationInput, error) {
	buildInfoDir := filepath.Join(dir, "out", "build-info")

	entries, err := os.ReadDir(buildInfoDir)
	if err != nil {
		return nil, fmt.Errorf("reading build-info directory: %w", err)
	}

	var firstMatch *chains.VerificationInput

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

		// When sourcePath is set, verify this build-info produced the requested contract
		if sourcePath != "" {
			var output struct {
				Contracts buildInfoOutputContracts `json:"contracts"`
			}
			if err := json.Unmarshal(buildInfo.Output, &output); err != nil {
				continue
			}
			if output.Contracts == nil {
				continue
			}
			sourceContracts, ok := output.Contracts[sourcePath]
			if !ok {
				continue
			}
			if _, ok := sourceContracts[contractName]; !ok {
				continue
			}
		}

		stdJSON, err := stripFoundryStandardJSONKeys(buildInfo.Input)
		if err != nil {
			continue
		}

		vi := &chains.VerificationInput{
			StandardJSON:    stdJSON,
			SolcLongVersion: buildInfo.SolcLongVersion,
		}
		if sourcePath != "" {
			return vi, nil
		}
		if firstMatch == nil {
			firstMatch = vi
		}
	}

	if firstMatch != nil {
		return firstMatch, nil
	}
	return nil, fmt.Errorf("build-info not found for contract %s", contractName)
}

// foundryStandardJSONKeysToStrip are top-level keys Foundry adds that the Solidity compiler rejects.
// The standard JSON input spec only allows: language, sources, settings.
var foundryStandardJSONKeysToStrip = []string{"allowPaths", "basePath", "includePaths", "version"}

// stripFoundryStandardJSONKeys removes Foundry-specific keys from standard JSON input
// so it conforms to the Solidity compiler's expected format.
func stripFoundryStandardJSONKeys(input json.RawMessage) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return nil, err
	}
	for _, key := range foundryStandardJSONKeysToStrip {
		delete(m, key)
	}
	return json.Marshal(m)
}

// standardJSONInput is the structure we build for per-contract minimal verification input
type standardJSONInput struct {
	Language string                   `json:"language"`
	Sources  map[string]sourceContent `json:"sources"`
	Settings standardJSONSettings     `json:"settings"`
}

type sourceContent struct {
	Content string `json:"content"`
}

type standardJSONSettings struct {
	Optimizer       optimizerSettings              `json:"optimizer"`
	EVMVersion      string                         `json:"evmVersion,omitempty"`
	ViaIR           bool                           `json:"viaIR,omitempty"`
	Libraries       map[string]map[string]string   `json:"libraries,omitempty"`
	Remappings      []string                       `json:"remappings,omitempty"`
	Metadata        standardJSONMetadataConfig     `json:"metadata,omitempty"`
	OutputSelection map[string]map[string][]string `json:"outputSelection"`
}

type optimizerSettings struct {
	Enabled bool `json:"enabled"`
	Runs    int  `json:"runs"`
}

// standardJSONMetadataConfig holds metadata settings for standard JSON input (not compiler output)
type standardJSONMetadataConfig struct {
	BytecodeHash      string `json:"bytecodeHash,omitempty"`
	UseLiteralContent bool   `json:"useLiteralContent,omitempty"`
	AppendCBOR        *bool  `json:"appendCBOR,omitempty"`
}

func outputSelectionForVerification() map[string]map[string][]string {
	return map[string]map[string][]string{
		"*": {"*": {"abi", "evm.bytecode", "evm.deployedBytecode", "metadata"}},
	}
}

// GeneratePerContractStandardJSON builds a minimal standard JSON input from the artifact's
// rawMetadata, containing only the contract's actual dependencies. This produces verification
// input that matches the metadata hash in the bytecode (unlike project-wide build-info).
func (b *Builder) GeneratePerContractStandardJSON(dir, artifactPath string) ([]byte, error) {
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("reading artifact: %w", err)
	}

	var raw FoundryArtifact
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing artifact: %w", err)
	}

	if raw.RawMetadata == "" {
		return nil, fmt.Errorf("artifact has no rawMetadata")
	}

	var metadata FoundryMetadata
	if err := json.Unmarshal([]byte(raw.RawMetadata), &metadata); err != nil {
		return nil, fmt.Errorf("parsing rawMetadata: %w", err)
	}

	if len(metadata.Sources) == 0 {
		return nil, fmt.Errorf("metadata has no sources")
	}

	// Read each source file from disk
	sources := make(map[string]sourceContent)
	for srcPath := range metadata.Sources {
		fullPath := filepath.Join(dir, srcPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("reading source %s: %w", srcPath, err)
		}
		sources[srcPath] = sourceContent{Content: string(content)}
	}

	// Build settings
	lang := metadata.Language
	if lang == "" {
		lang = "Solidity"
	}

	opt := metadata.Settings.Optimizer
	optSettings := optimizerSettings(opt)
	// Only default runs when optimizer is enabled; when disabled, runs=0 is correct
	if opt.Enabled && opt.Runs == 0 {
		optSettings.Runs = 200
	}

	metaOut := standardJSONMetadataConfig{BytecodeHash: "ipfs"}
	if metadata.Settings.Metadata != nil {
		if metadata.Settings.Metadata.BytecodeHash != "" {
			metaOut.BytecodeHash = metadata.Settings.Metadata.BytecodeHash
		}
		metaOut.UseLiteralContent = metadata.Settings.Metadata.UseLiteralContent
		metaOut.AppendCBOR = metadata.Settings.Metadata.AppendCBOR
	}

	settings := standardJSONSettings{
		Optimizer:       optSettings,
		EVMVersion:      metadata.Settings.EVMVersion,
		ViaIR:           metadata.Settings.ViaIR,
		Libraries:       metadata.Settings.Libraries,
		Remappings:      metadata.Settings.Remappings,
		Metadata:        metaOut,
		OutputSelection: outputSelectionForVerification(),
	}
	// Omit EVMVersion when empty so solc uses its version-appropriate default

	input := standardJSONInput{
		Language: lang,
		Sources:  sources,
		Settings: settings,
	}

	return json.MarshalIndent(input, "", "  ")
}

// FoundryArtifact represents the structure of a Foundry artifact JSON file
type FoundryArtifact struct {
	ABI              json.RawMessage `json:"abi"`
	Bytecode         BytecodeObject  `json:"bytecode"`
	DeployedBytecode BytecodeObject  `json:"deployedBytecode"`
	StorageLayout    json.RawMessage `json:"storageLayout"`
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

// MetadataSettings contains metadata options for standard JSON (bytecodeHash, useLiteralContent, etc.)
type MetadataSettings struct {
	BytecodeHash      string `json:"bytecodeHash,omitempty"`      // default "ipfs"
	UseLiteralContent bool   `json:"useLiteralContent,omitempty"` // some projects set true
	AppendCBOR        *bool  `json:"appendCBOR,omitempty"`        // optional
}

// SettingsMeta contains compiler settings
type SettingsMeta struct {
	CompilationTarget map[string]string            `json:"compilationTarget"`
	EVMVersion        string                       `json:"evmVersion"`
	Libraries         map[string]map[string]string `json:"libraries"` // source path -> library name -> address
	Metadata          *MetadataSettings            `json:"metadata,omitempty"`
	Optimizer         OptimizerMeta                `json:"optimizer"`
	Remappings        []string                     `json:"remappings"`
	ViaIR             bool                         `json:"viaIR"`
}

// getFirstKey returns the first key from a map
func getFirstKey(m map[string]string) string {
	for k := range m {
		return k
	}
	return ""
}

// isIncludedDependency checks if a contract name matches any dependency (case-insensitive)
func isIncludedDependency(name string, deps []string) bool {
	for _, d := range deps {
		if strings.EqualFold(d, name) {
			return true
		}
	}
	return false
}

// DiscoverDependencies finds all dependency contracts (from lib/) available in build artifacts
func (b *Builder) DiscoverDependencies(dir string) ([]chains.DependencyInfo, error) {
	outDir := filepath.Join(dir, "out")

	// Check if out directory exists
	if _, err := os.Stat(outDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("out directory not found - run 'forge build' first")
	}

	var deps []chains.DependencyInfo
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

		// Read the artifact to check its source path
		sourcePath, err := b.getArtifactSourcePath(path)
		if err != nil {
			return nil // Skip artifacts we can't read
		}

		// Only include contracts NOT from src/ directory (these are dependencies)
		if strings.HasPrefix(sourcePath, "src/") {
			return nil
		}

		// Skip contracts without bytecode (interfaces)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var raw FoundryArtifact
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil
		}

		if raw.Bytecode.Object == "" || raw.Bytecode.Object == "0x" {
			return nil // Skip interfaces
		}

		seen[contractName] = true
		deps = append(deps, chains.DependencyInfo{
			Name:       contractName,
			SourcePath: sourcePath,
		})
		return nil
	})

	return deps, err
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

// BuildInfo represents a Foundry build-info file (hh-sol-build-info-1 format)
type BuildInfo struct {
	ID              string          `json:"id"`
	SolcVersion     string          `json:"solcVersion"`     // Short: "0.8.28"
	SolcLongVersion string          `json:"solcLongVersion"` // Full: "0.8.28+commit.7893614a"
	Input           json.RawMessage `json:"input"`           // Standard JSON Input
	Output          json.RawMessage `json:"output"`          // Compilation output
}
