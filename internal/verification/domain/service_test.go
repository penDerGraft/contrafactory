package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pendergraft/contrafactory/internal/chains"
	"github.com/pendergraft/contrafactory/internal/storage"
)

// mockStore implements storage.Store for testing
type mockStore struct {
	packages  map[string]*storage.Package
	contracts map[string]*storage.Contract
	artifacts map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{
		packages:  make(map[string]*storage.Package),
		contracts: make(map[string]*storage.Contract),
		artifacts: make(map[string][]byte),
	}
}

func (m *mockStore) GetPackage(ctx context.Context, name, version string) (*storage.Package, error) {
	key := name + "@" + version
	if pkg, ok := m.packages[key]; ok {
		return pkg, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) GetContract(ctx context.Context, packageID, contractName string) (*storage.Contract, error) {
	key := packageID + "/" + contractName
	if contract, ok := m.contracts[key]; ok {
		return contract, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) GetArtifact(ctx context.Context, contractID, artifactType string) ([]byte, error) {
	key := contractID + "/" + artifactType
	if artifact, ok := m.artifacts[key]; ok {
		return artifact, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) Close() error                      { return nil }
func (m *mockStore) Migrate(ctx context.Context) error { return nil }

// mockChain implements chains.Chain for testing
type mockChain struct {
	name                string
	deployedBytecode    []byte
	deployedBytecodeErr error
	verifyResult        *chains.VerifyResult
	verifyErr           error
}

func (m *mockChain) Name() string                                     { return m.name }
func (m *mockChain) DisplayName() string                              { return m.name }
func (m *mockChain) DetectBuilder(dir string) (chains.Builder, error) { return nil, nil }
func (m *mockChain) Builders() []chains.Builder                       { return nil }

func (m *mockChain) GetDeployedBytecode(ctx context.Context, rpc string, address string) ([]byte, error) {
	if m.deployedBytecodeErr != nil {
		return nil, m.deployedBytecodeErr
	}
	return m.deployedBytecode, nil
}

func (m *mockChain) VerifyDeployment(ctx context.Context, opts chains.VerifyOptions) (*chains.VerifyResult, error) {
	if m.verifyErr != nil {
		return nil, m.verifyErr
	}
	return m.verifyResult, nil
}

func TestVerify_InvalidAddress(t *testing.T) {
	store := newMockStore()
	registry := chains.NewRegistry()
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:  "test-pkg",
		Version:  "1.0.0",
		Contract: "MyContract",
		ChainID:  1,
		Address:  "invalid-address", // Not a valid hex address
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidAddress))
}

func TestVerify_InvalidChainID(t *testing.T) {
	store := newMockStore()
	registry := chains.NewRegistry()
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:  "test-pkg",
		Version:  "1.0.0",
		Contract: "MyContract",
		ChainID:  -1, // Invalid chain ID
		Address:  "0x1234567890123456789012345678901234567890",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidChainID))
}

func TestVerify_PackageNotFound(t *testing.T) {
	store := newMockStore()
	registry := chains.NewRegistry()
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:  "nonexistent-pkg",
		Version:  "1.0.0",
		Contract: "MyContract",
		ChainID:  1,
		Address:  "0x1234567890123456789012345678901234567890",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestVerify_ContractNotFound(t *testing.T) {
	store := newMockStore()
	store.packages["test-pkg@1.0.0"] = &storage.Package{
		ID:    "pkg-123",
		Name:  "test-pkg",
		Chain: "evm",
	}

	registry := chains.NewRegistry()
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:  "test-pkg",
		Version:  "1.0.0",
		Contract: "NonexistentContract",
		ChainID:  1,
		Address:  "0x1234567890123456789012345678901234567890",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestVerify_DeployedBytecodeNotFound(t *testing.T) {
	store := newMockStore()
	store.packages["test-pkg@1.0.0"] = &storage.Package{
		ID:    "pkg-123",
		Name:  "test-pkg",
		Chain: "evm",
	}
	store.contracts["pkg-123/MyContract"] = &storage.Contract{
		ID:        "contract-456",
		PackageID: "pkg-123",
		Name:      "MyContract",
	}
	// No artifact stored

	registry := chains.NewRegistry()
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:  "test-pkg",
		Version:  "1.0.0",
		Contract: "MyContract",
		ChainID:  1,
		Address:  "0x1234567890123456789012345678901234567890",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deployed bytecode not found")
}

func TestVerify_ChainNotSupported(t *testing.T) {
	store := newMockStore()
	store.packages["test-pkg@1.0.0"] = &storage.Package{
		ID:    "pkg-123",
		Name:  "test-pkg",
		Chain: "unsupported-chain",
	}
	store.contracts["pkg-123/MyContract"] = &storage.Contract{
		ID:        "contract-456",
		PackageID: "pkg-123",
		Name:      "MyContract",
	}
	store.artifacts["contract-456/deployed-bytecode"] = []byte("0x608060")

	registry := chains.NewRegistry()
	// No chain registered
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:  "test-pkg",
		Version:  "1.0.0",
		Contract: "MyContract",
		ChainID:  1,
		Address:  "0x1234567890123456789012345678901234567890",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrChainNotFound))
}

func TestVerify_WithoutRPC_ReturnsPending(t *testing.T) {
	store := newMockStore()
	store.packages["test-pkg@1.0.0"] = &storage.Package{
		ID:    "pkg-123",
		Name:  "test-pkg",
		Chain: "evm",
	}
	store.contracts["pkg-123/MyContract"] = &storage.Contract{
		ID:          "contract-456",
		PackageID:   "pkg-123",
		Name:        "MyContract",
		PrimaryHash: "0xabcdef123456",
	}
	store.artifacts["contract-456/deployed-bytecode"] = []byte("0x608060")

	registry := chains.NewRegistry()
	registry.Register(&mockChain{name: "evm"})
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:  "test-pkg",
		Version:  "1.0.0",
		Contract: "MyContract",
		ChainID:  1,
		Address:  "0x1234567890123456789012345678901234567890",
		// No RPCEndpoint
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Verified)
	assert.Equal(t, "pending", result.MatchType)
	assert.Contains(t, result.Message, "Provide RPC endpoint")
	assert.Equal(t, "0xabcdef123456", result.Details.ExpectedBytecodeHash)
}

func TestVerify_WithRPC_FullMatch(t *testing.T) {
	bytecode := []byte("0x608060405234801561001057600080fd")

	store := newMockStore()
	store.packages["test-pkg@1.0.0"] = &storage.Package{
		ID:    "pkg-123",
		Name:  "test-pkg",
		Chain: "evm",
	}
	store.contracts["pkg-123/MyContract"] = &storage.Contract{
		ID:        "contract-456",
		PackageID: "pkg-123",
		Name:      "MyContract",
	}
	store.artifacts["contract-456/deployed-bytecode"] = bytecode

	mockEVM := &mockChain{
		name:             "evm",
		deployedBytecode: bytecode, // Same bytecode = full match
		verifyResult: &chains.VerifyResult{
			Match:     true,
			MatchType: "full",
			Message:   "Bytecode matches exactly",
		},
	}

	registry := chains.NewRegistry()
	registry.Register(mockEVM)
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:     "test-pkg",
		Version:     "1.0.0",
		Contract:    "MyContract",
		ChainID:     1,
		Address:     "0x1234567890123456789012345678901234567890",
		RPCEndpoint: "https://eth-mainnet.example.com",
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Verified)
	assert.Equal(t, "full", result.MatchType)
}

func TestVerify_WithRPC_PartialMatch(t *testing.T) {
	storedBytecode := []byte("0x608060405234801561001057600080fd")
	onChainBytecode := []byte("0x608060405234801561001057600080fe") // Slightly different

	store := newMockStore()
	store.packages["test-pkg@1.0.0"] = &storage.Package{
		ID:    "pkg-123",
		Name:  "test-pkg",
		Chain: "evm",
	}
	store.contracts["pkg-123/MyContract"] = &storage.Contract{
		ID:        "contract-456",
		PackageID: "pkg-123",
		Name:      "MyContract",
	}
	store.artifacts["contract-456/deployed-bytecode"] = storedBytecode

	mockEVM := &mockChain{
		name:             "evm",
		deployedBytecode: onChainBytecode,
		verifyResult: &chains.VerifyResult{
			Match:     true,
			MatchType: "partial",
			Message:   "Bytecode matches after stripping metadata",
		},
	}

	registry := chains.NewRegistry()
	registry.Register(mockEVM)
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:     "test-pkg",
		Version:     "1.0.0",
		Contract:    "MyContract",
		ChainID:     1,
		Address:     "0x1234567890123456789012345678901234567890",
		RPCEndpoint: "https://eth-mainnet.example.com",
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Verified)
	assert.Equal(t, "partial", result.MatchType)
}

func TestVerify_WithRPC_NoMatch(t *testing.T) {
	storedBytecode := []byte("0x608060405234801561001057600080fd")
	onChainBytecode := []byte("0xcompletely_different_bytecode")

	store := newMockStore()
	store.packages["test-pkg@1.0.0"] = &storage.Package{
		ID:    "pkg-123",
		Name:  "test-pkg",
		Chain: "evm",
	}
	store.contracts["pkg-123/MyContract"] = &storage.Contract{
		ID:        "contract-456",
		PackageID: "pkg-123",
		Name:      "MyContract",
	}
	store.artifacts["contract-456/deployed-bytecode"] = storedBytecode

	mockEVM := &mockChain{
		name:             "evm",
		deployedBytecode: onChainBytecode,
		verifyResult: &chains.VerifyResult{
			Match:     false,
			MatchType: "none",
			Message:   "Bytecode does not match",
		},
	}

	registry := chains.NewRegistry()
	registry.Register(mockEVM)
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:     "test-pkg",
		Version:     "1.0.0",
		Contract:    "MyContract",
		ChainID:     1,
		Address:     "0x1234567890123456789012345678901234567890",
		RPCEndpoint: "https://eth-mainnet.example.com",
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Verified)
	assert.Equal(t, "none", result.MatchType)
}

func TestVerify_WithRPC_FetchBytecodeError(t *testing.T) {
	store := newMockStore()
	store.packages["test-pkg@1.0.0"] = &storage.Package{
		ID:    "pkg-123",
		Name:  "test-pkg",
		Chain: "evm",
	}
	store.contracts["pkg-123/MyContract"] = &storage.Contract{
		ID:        "contract-456",
		PackageID: "pkg-123",
		Name:      "MyContract",
	}
	store.artifacts["contract-456/deployed-bytecode"] = []byte("0x608060")

	mockEVM := &mockChain{
		name:                "evm",
		deployedBytecodeErr: errors.New("RPC connection failed"),
	}

	registry := chains.NewRegistry()
	registry.Register(mockEVM)
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:     "test-pkg",
		Version:     "1.0.0",
		Contract:    "MyContract",
		ChainID:     1,
		Address:     "0x1234567890123456789012345678901234567890",
		RPCEndpoint: "https://eth-mainnet.example.com",
	})

	// Should return a result with verified=false, not an error
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Verified)
	assert.Equal(t, "none", result.MatchType)
	assert.Contains(t, result.Message, "Failed to fetch on-chain bytecode")
}

func TestVerify_WithRPC_VerificationError(t *testing.T) {
	bytecode := []byte("0x608060405234801561001057600080fd")

	store := newMockStore()
	store.packages["test-pkg@1.0.0"] = &storage.Package{
		ID:    "pkg-123",
		Name:  "test-pkg",
		Chain: "evm",
	}
	store.contracts["pkg-123/MyContract"] = &storage.Contract{
		ID:        "contract-456",
		PackageID: "pkg-123",
		Name:      "MyContract",
	}
	store.artifacts["contract-456/deployed-bytecode"] = bytecode

	mockEVM := &mockChain{
		name:             "evm",
		deployedBytecode: bytecode,
		verifyErr:        errors.New("verification internal error"),
	}

	registry := chains.NewRegistry()
	registry.Register(mockEVM)
	svc := NewService(store, store, registry)

	result, err := svc.Verify(context.Background(), VerifyRequest{
		Package:     "test-pkg",
		Version:     "1.0.0",
		Contract:    "MyContract",
		ChainID:     1,
		Address:     "0x1234567890123456789012345678901234567890",
		RPCEndpoint: "https://eth-mainnet.example.com",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verifying deployment")
}

func TestNewService(t *testing.T) {
	store := newMockStore()
	registry := chains.NewRegistry()

	svc := NewService(store, store, registry)
	assert.NotNil(t, svc)
}
