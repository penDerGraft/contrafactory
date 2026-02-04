package domain

import (
	"context"
	"testing"

	"github.com/pendergraft/contrafactory/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore implements storage.Store for testing
type mockStore struct {
	packages    map[string]*storage.Package
	deployments map[string]*storage.Deployment
}

func newMockStore() *mockStore {
	return &mockStore{
		packages:    make(map[string]*storage.Package),
		deployments: make(map[string]*storage.Deployment),
	}
}

func (m *mockStore) GetPackage(ctx context.Context, name, version string) (*storage.Package, error) {
	key := name + "@" + version
	if pkg, ok := m.packages[key]; ok {
		return pkg, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) RecordDeployment(ctx context.Context, d *storage.Deployment) error {
	key := d.Chain + "/" + d.ChainID + "/" + d.Address
	m.deployments[key] = d
	return nil
}

func (m *mockStore) GetDeployment(ctx context.Context, chain, chainID, address string) (*storage.Deployment, error) {
	key := chain + "/" + chainID + "/" + address
	if d, ok := m.deployments[key]; ok {
		return d, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) ListDeployments(ctx context.Context, filter storage.DeploymentFilter, pagination storage.PaginationParams) (*storage.PaginatedResult[storage.Deployment], error) {
	var deployments []storage.Deployment
	for _, d := range m.deployments {
		deployments = append(deployments, *d)
	}
	return &storage.PaginatedResult[storage.Deployment]{Data: deployments}, nil
}

func (m *mockStore) UpdateVerificationStatus(ctx context.Context, id string, verified bool, verifiedOn []string) error {
	for _, d := range m.deployments {
		if d.ID == id {
			d.Verified = verified
			d.VerifiedOn = verifiedOn
			return nil
		}
	}
	return storage.ErrNotFound
}

// Stub methods required by interface
func (m *mockStore) CreatePackage(ctx context.Context, pkg *storage.Package) error { return nil }
func (m *mockStore) GetPackageVersions(ctx context.Context, name string, includePrerelease bool) ([]string, error) {
	return nil, nil
}
func (m *mockStore) ListPackages(ctx context.Context, filter storage.PackageFilter, pagination storage.PaginationParams) (*storage.PaginatedResult[storage.Package], error) {
	return nil, nil
}
func (m *mockStore) DeletePackage(ctx context.Context, name, version string) error { return nil }
func (m *mockStore) PackageExists(ctx context.Context, name, version string) (bool, error) {
	return false, nil
}
func (m *mockStore) GetPackageOwner(ctx context.Context, name string) (string, error)   { return "", nil }
func (m *mockStore) SetPackageOwner(ctx context.Context, name, ownerKeyID string) error { return nil }
func (m *mockStore) CreateContract(ctx context.Context, packageID string, contract *storage.Contract) error {
	return nil
}
func (m *mockStore) GetContract(ctx context.Context, packageID, contractName string) (*storage.Contract, error) {
	return nil, nil
}
func (m *mockStore) ListContracts(ctx context.Context, packageID string) ([]storage.Contract, error) {
	return nil, nil
}
func (m *mockStore) StoreArtifact(ctx context.Context, contractID, artifactType string, content []byte) error {
	return nil
}
func (m *mockStore) GetArtifact(ctx context.Context, contractID, artifactType string) ([]byte, error) {
	return nil, nil
}
func (m *mockStore) GetArtifactByHash(ctx context.Context, hash string) ([]byte, error) {
	return nil, nil
}
func (m *mockStore) CreateAPIKey(ctx context.Context, name string) (string, error) { return "", nil }
func (m *mockStore) ValidateAPIKey(ctx context.Context, key string) (*storage.APIKey, error) {
	return nil, nil
}
func (m *mockStore) ListAPIKeys(ctx context.Context) ([]storage.APIKey, error) { return nil, nil }
func (m *mockStore) RevokeAPIKey(ctx context.Context, id string) error         { return nil }
func (m *mockStore) Close() error                                              { return nil }
func (m *mockStore) Migrate(ctx context.Context) error                         { return nil }

func TestService_Record(t *testing.T) {
	tests := []struct {
		name    string
		req     RecordRequest
		wantErr error
		setup   func(*mockStore)
	}{
		{
			name: "record valid deployment",
			req: RecordRequest{
				Package:  "my-pkg",
				Version:  "1.0.0",
				Contract: "Token",
				ChainID:  1,
				Address:  "0x1234567890abcdef1234567890abcdef12345678",
				TxHash:   "0xabcdef",
			},
			wantErr: nil,
			setup: func(m *mockStore) {
				m.packages["my-pkg@1.0.0"] = &storage.Package{
					ID:    "pkg-123",
					Name:  "my-pkg",
					Chain: "evm",
				}
			},
		},
		{
			name: "invalid address",
			req: RecordRequest{
				Package:  "my-pkg",
				Version:  "1.0.0",
				Contract: "Token",
				ChainID:  1,
				Address:  "invalid",
			},
			wantErr: ErrInvalidAddress,
			setup: func(m *mockStore) {
				m.packages["my-pkg@1.0.0"] = &storage.Package{ID: "pkg-123", Chain: "evm"}
			},
		},
		{
			name: "invalid chain ID",
			req: RecordRequest{
				Package:  "my-pkg",
				Version:  "1.0.0",
				Contract: "Token",
				ChainID:  0,
				Address:  "0x1234567890abcdef1234567890abcdef12345678",
			},
			wantErr: ErrInvalidChainID,
			setup: func(m *mockStore) {
				m.packages["my-pkg@1.0.0"] = &storage.Package{ID: "pkg-123", Chain: "evm"}
			},
		},
		{
			name: "package not found",
			req: RecordRequest{
				Package:  "not-found",
				Version:  "1.0.0",
				Contract: "Token",
				ChainID:  1,
				Address:  "0x1234567890abcdef1234567890abcdef12345678",
			},
			wantErr: ErrPackageNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			if tt.setup != nil {
				tt.setup(store)
			}

			svc := NewService(store)
			result, err := svc.Record(context.Background(), tt.req)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, result.ID)
				assert.Equal(t, tt.req.Address, result.Address)
			}
		})
	}
}

func TestService_Get(t *testing.T) {
	store := newMockStore()
	store.deployments["evm/1/0x1234567890abcdef1234567890abcdef12345678"] = &storage.Deployment{
		ID:       "deploy-123",
		ChainID:  "1",
		Address:  "0x1234567890abcdef1234567890abcdef12345678",
		Verified: false,
	}

	svc := NewService(store)

	t.Run("existing deployment", func(t *testing.T) {
		d, err := svc.Get(context.Background(), "1", "0x1234567890abcdef1234567890abcdef12345678")
		require.NoError(t, err)
		assert.Equal(t, "deploy-123", d.ID)
	})

	t.Run("non-existing deployment", func(t *testing.T) {
		_, err := svc.Get(context.Background(), "1", "0x0000000000000000000000000000000000000000")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestService_List(t *testing.T) {
	store := newMockStore()
	store.deployments["evm/1/0x1234567890abcdef1234567890abcdef12345678"] = &storage.Deployment{
		ID:      "deploy-1",
		ChainID: "1",
		Address: "0x1234567890abcdef1234567890abcdef12345678",
	}
	store.deployments["evm/137/0xabcdef1234567890abcdef1234567890abcdef12"] = &storage.Deployment{
		ID:      "deploy-2",
		ChainID: "137",
		Address: "0xabcdef1234567890abcdef1234567890abcdef12",
	}

	svc := NewService(store)

	result, err := svc.List(context.Background(), ListFilter{}, PaginationParams{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, result.Deployments, 2)
}

func TestService_UpdateVerificationStatus(t *testing.T) {
	store := newMockStore()
	store.deployments["evm/1/0x1234567890abcdef1234567890abcdef12345678"] = &storage.Deployment{
		ID:       "deploy-123",
		Chain:    "evm",
		ChainID:  "1",
		Address:  "0x1234567890abcdef1234567890abcdef12345678",
		Verified: false,
	}

	svc := NewService(store)

	err := svc.UpdateVerificationStatus(context.Background(), "1", "0x1234567890abcdef1234567890abcdef12345678", true, []string{"etherscan"})
	require.NoError(t, err)

	// Verify the update
	d := store.deployments["evm/1/0x1234567890abcdef1234567890abcdef12345678"]
	assert.True(t, d.Verified)
	assert.Contains(t, d.VerifiedOn, "etherscan")
}

func TestToDeployment_TimestampParsing(t *testing.T) {
	tests := []struct {
		name          string
		createdAt     string
		wantYear      int
		wantZeroTime  bool
	}{
		{
			name:         "valid datetime",
			createdAt:    "2025-06-15 14:30:45",
			wantYear:     2025,
			wantZeroTime: false,
		},
		{
			name:         "empty datetime",
			createdAt:    "",
			wantYear:     1,
			wantZeroTime: true,
		},
		{
			name:         "invalid datetime format",
			createdAt:    "invalid-date",
			wantYear:     1,
			wantZeroTime: true,
		},
		{
			name:         "different valid datetime",
			createdAt:    "2020-01-01 00:00:00",
			wantYear:     2020,
			wantZeroTime: false,
		},
		{
			name:         "ISO format is not parsed",
			createdAt:    "2025-06-15T14:30:45Z",
			wantYear:     1,
			wantZeroTime: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storageDep := &storage.Deployment{
				ID:        "test-id",
				ChainID:   "1",
				Address:   "0x1234567890abcdef1234567890abcdef12345678",
				CreatedAt: tt.createdAt,
			}

			domainDep := toDeployment(storageDep)

			if tt.wantZeroTime {
				assert.True(t, domainDep.CreatedAt.IsZero(), "expected zero time for input: %q", tt.createdAt)
			} else {
				assert.False(t, domainDep.CreatedAt.IsZero(), "expected non-zero time for input: %q", tt.createdAt)
				assert.Equal(t, tt.wantYear, domainDep.CreatedAt.Year())
			}
		})
	}
}
