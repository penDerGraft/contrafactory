// Package chains provides the chain module interfaces and implementations
// for different blockchain ecosystems (EVM, Solana, etc.).
package chains

import (
	"context"
	"encoding/json"
	"fmt"
)

// Chain represents a blockchain ecosystem (EVM, Solana, etc.)
type Chain interface {
	// Metadata
	Name() string        // "evm", "solana"
	DisplayName() string // "Ethereum/EVM", "Solana"

	// Builder discovery
	DetectBuilder(dir string) (Builder, error)
	Builders() []Builder

	// Verification
	VerifyDeployment(ctx context.Context, opts VerifyOptions) (*VerifyResult, error)
	GetDeployedBytecode(ctx context.Context, rpc string, address string) ([]byte, error)
}

// Builder parses artifacts from a specific build tool
type Builder interface {
	// Metadata
	Name() string        // "foundry", "hardhat", "anchor"
	DisplayName() string // "Foundry", "Hardhat", "Anchor"
	Chain() string       // "evm", "solana"

	// Detection
	Detect(dir string) (bool, error)
	ConfigFile() string // "foundry.toml", "hardhat.config.ts", "Anchor.toml"

	// Artifact handling
	Discover(dir string, opts DiscoverOptions) ([]string, error)
	Parse(artifactPath string) (*Artifact, error)
	GenerateVerificationInput(dir string, contractName string) ([]byte, error)
}

// DiscoverOptions configures artifact discovery
type DiscoverOptions struct {
	// Contracts to include (empty = all)
	Contracts []string
	// Patterns to exclude (e.g., "Test*", "Mock*")
	Exclude []string
}

// VerifyOptions configures verification
type VerifyOptions struct {
	RPC             string
	Address         string
	ExpectedCode    []byte
	ConstructorArgs []byte
	Libraries       map[string]string
}

// VerifyResult contains verification results
type VerifyResult struct {
	Match     bool   // Whether the bytecode matches
	MatchType string // "full", "partial", "none"
	Message   string // Human-readable explanation
}

// Artifact can represent any chain's contract/program
type Artifact struct {
	// Common metadata
	Name  string `json:"name"`
	Chain string `json:"chain"` // "evm", "solana"

	// Chain-specific data (one of these is populated)
	EVM    *EVMArtifact    `json:"evm,omitempty"`
	Solana *SolanaArtifact `json:"solana,omitempty"`
}

// EVMArtifact contains EVM-specific contract data
type EVMArtifact struct {
	SourcePath        string          `json:"sourcePath"`
	License           string          `json:"license,omitempty"`
	ABI               json.RawMessage `json:"abi"`
	Bytecode          string          `json:"bytecode"`
	DeployedBytecode  string          `json:"deployedBytecode"`
	StandardJSONInput json.RawMessage `json:"standardJsonInput,omitempty"`
	StorageLayout     json.RawMessage `json:"storageLayout,omitempty"`
	Compiler          EVMCompiler     `json:"compiler"`
}

// EVMCompiler contains EVM compiler details
type EVMCompiler struct {
	Version    string          `json:"version"` // "v0.8.20+commit.a1b2c3d4"
	Optimizer  OptimizerConfig `json:"optimizer"`
	EVMVersion string          `json:"evmVersion"` // "paris", "shanghai"
	ViaIR      bool            `json:"viaIR"`
}

// OptimizerConfig contains optimizer settings
type OptimizerConfig struct {
	Enabled bool `json:"enabled"`
	Runs    int  `json:"runs"`
}

// SolanaArtifact contains Solana-specific program data (future)
type SolanaArtifact struct {
	IDL         json.RawMessage `json:"idl"`         // Anchor IDL
	ProgramHash string          `json:"programHash"` // SHA256 of .so
	// Binary stored in blob store, referenced by hash
}

// Registry holds all registered chain modules
type Registry struct {
	chains map[string]Chain
}

// NewRegistry creates a new chain registry
func NewRegistry() *Registry {
	return &Registry{
		chains: make(map[string]Chain),
	}
}

// Register adds a chain module to the registry
func (r *Registry) Register(c Chain) {
	r.chains[c.Name()] = c
}

// Get retrieves a chain module by name
func (r *Registry) Get(name string) (Chain, bool) {
	c, ok := r.chains[name]
	return c, ok
}

// List returns all registered chain modules
func (r *Registry) List() []Chain {
	chains := make([]Chain, 0, len(r.chains))
	for _, c := range r.chains {
		chains = append(chains, c)
	}
	return chains
}

// DetectChainAndBuilder detects the chain and builder for a project directory
func (r *Registry) DetectChainAndBuilder(dir string) (Chain, Builder, error) {
	for _, chain := range r.chains {
		builder, err := chain.DetectBuilder(dir)
		if err == nil && builder != nil {
			return chain, builder, nil
		}
	}
	return nil, nil, fmt.Errorf("no supported builder detected in %s", dir)
}

// DefaultRegistry returns a registry with all built-in chain modules
// Note: Import cycle prevention - this is set up by the caller
func DefaultRegistry() *Registry {
	return NewRegistry()
}
