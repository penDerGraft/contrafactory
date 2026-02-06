package domain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pendergraft/contrafactory/internal/storage"
)

// mockStore implements storage.Store for testing
type mockStore struct {
	packages  map[string]*storage.Package
	contracts map[string]*storage.Contract
	artifacts map[string][]byte
	owners    map[string]string
}

func newMockStore() *mockStore {
	return &mockStore{
		packages:  make(map[string]*storage.Package),
		contracts: make(map[string]*storage.Contract),
		artifacts: make(map[string][]byte),
		owners:    make(map[string]string),
	}
}

func (m *mockStore) CreatePackage(ctx context.Context, pkg *storage.Package) error {
	key := pkg.Name + "@" + pkg.Version
	m.packages[key] = pkg
	return nil
}

func (m *mockStore) GetPackage(ctx context.Context, name, version string) (*storage.Package, error) {
	key := name + "@" + version
	if pkg, ok := m.packages[key]; ok {
		return pkg, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) GetPackageVersions(ctx context.Context, name string, includePrerelease bool) ([]string, error) {
	var versions []string
	for key, pkg := range m.packages {
		if pkg.Name == name {
			versions = append(versions, pkg.Version)
			_ = key
		}
	}
	return versions, nil
}

func (m *mockStore) ListPackages(ctx context.Context, filter storage.PackageFilter, pagination storage.PaginationParams) (*storage.PaginatedResult[storage.Package], error) {
	var packages []storage.Package
	for _, pkg := range m.packages {
		packages = append(packages, *pkg)
	}
	return &storage.PaginatedResult[storage.Package]{Data: packages}, nil
}

func (m *mockStore) DeletePackage(ctx context.Context, name, version string) error {
	key := name + "@" + version
	delete(m.packages, key)
	return nil
}

func (m *mockStore) PackageExists(ctx context.Context, name, version string) (bool, error) {
	key := name + "@" + version
	_, exists := m.packages[key]
	return exists, nil
}

func (m *mockStore) GetPackageOwner(ctx context.Context, name string) (string, error) {
	return m.owners[name], nil
}

func (m *mockStore) SetPackageOwner(ctx context.Context, name, ownerKeyID string) error {
	if _, exists := m.owners[name]; !exists {
		m.owners[name] = ownerKeyID
	}
	return nil
}

func (m *mockStore) CreateContract(ctx context.Context, packageID string, contract *storage.Contract) error {
	key := packageID + "/" + contract.Name
	contract.PackageID = packageID
	m.contracts[key] = contract
	return nil
}

func (m *mockStore) GetContract(ctx context.Context, packageID, contractName string) (*storage.Contract, error) {
	key := packageID + "/" + contractName
	if c, ok := m.contracts[key]; ok {
		return c, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) ListContracts(ctx context.Context, packageID string) ([]storage.Contract, error) {
	var contracts []storage.Contract
	for _, c := range m.contracts {
		if c.PackageID == packageID {
			contracts = append(contracts, *c)
		}
	}
	return contracts, nil
}

func (m *mockStore) StoreArtifact(ctx context.Context, contractID, artifactType string, content []byte) error {
	key := contractID + "/" + artifactType
	m.artifacts[key] = content
	return nil
}

func (m *mockStore) GetArtifact(ctx context.Context, contractID, artifactType string) ([]byte, error) {
	key := contractID + "/" + artifactType
	if content, ok := m.artifacts[key]; ok {
		return content, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStore) Close() error    { return nil }
func (m *mockStore) Migrate(ctx context.Context) error { return nil }

func TestService_Publish(t *testing.T) {
	tests := []struct {
		name    string
		pkgName string
		version string
		ownerID string
		req     PublishRequest
		wantErr error
		setup   func(*mockStore)
	}{
		{
			name:    "publish new package",
			pkgName: "my-package",
			version: "1.0.0",
			ownerID: "owner-123",
			req: PublishRequest{
				Chain: "evm",
				Artifacts: []Artifact{
					{Name: "Token", Bytecode: "0x1234"},
				},
			},
			wantErr: nil,
		},
		{
			name:    "invalid package name",
			pkgName: "A",
			version: "1.0.0",
			ownerID: "owner-123",
			req:     PublishRequest{Chain: "evm"},
			wantErr: ErrInvalidName,
		},
		{
			name:    "invalid version",
			pkgName: "my-package",
			version: "invalid",
			ownerID: "owner-123",
			req:     PublishRequest{Chain: "evm"},
			wantErr: ErrInvalidVersion,
		},
		{
			name:    "version already exists",
			pkgName: "my-package",
			version: "1.0.0",
			ownerID: "owner-123",
			req:     PublishRequest{Chain: "evm"},
			wantErr: ErrVersionExists,
			setup: func(m *mockStore) {
				m.packages["my-package@1.0.0"] = &storage.Package{
					Name:    "my-package",
					Version: "1.0.0",
				}
			},
		},
		{
			name:    "forbidden - different owner",
			pkgName: "my-package",
			version: "2.0.0",
			ownerID: "owner-456",
			req:     PublishRequest{Chain: "evm"},
			wantErr: ErrForbidden,
			setup: func(m *mockStore) {
				m.owners["my-package"] = "owner-123"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			if tt.setup != nil {
				tt.setup(store)
			}

			svc := NewService(store, store)
			err := svc.Publish(context.Background(), tt.pkgName, tt.version, tt.ownerID, tt.req)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestService_Get(t *testing.T) {
	store := newMockStore()
	store.packages["my-package@1.0.0"] = &storage.Package{
		ID:      "pkg-123",
		Name:    "my-package",
		Version: "1.0.0",
		Chain:   "evm",
	}

	svc := NewService(store, store)

	t.Run("existing package", func(t *testing.T) {
		pkg, err := svc.Get(context.Background(), "my-package", "1.0.0")
		require.NoError(t, err)
		assert.Equal(t, "my-package", pkg.Name)
		assert.Equal(t, "1.0.0", pkg.Version)
	})

	t.Run("non-existing package", func(t *testing.T) {
		_, err := svc.Get(context.Background(), "not-found", "1.0.0")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("latest version", func(t *testing.T) {
		store.packages["my-package@2.0.0"] = &storage.Package{
			ID:      "pkg-456",
			Name:    "my-package",
			Version: "2.0.0",
			Chain:   "evm",
		}
		pkg, err := svc.Get(context.Background(), "my-package", "latest")
		require.NoError(t, err)
		assert.Equal(t, "2.0.0", pkg.Version)
	})
}

func TestService_GetVersions(t *testing.T) {
	store := newMockStore()
	store.packages["my-package@1.0.0"] = &storage.Package{Name: "my-package", Version: "1.0.0"}
	store.packages["my-package@2.0.0"] = &storage.Package{Name: "my-package", Version: "2.0.0"}

	svc := NewService(store, store)

	t.Run("existing package", func(t *testing.T) {
		result, err := svc.GetVersions(context.Background(), "my-package", false)
		require.NoError(t, err)
		assert.Equal(t, "my-package", result.Name)
		assert.Len(t, result.Versions, 2)
	})

	t.Run("non-existing package", func(t *testing.T) {
		_, err := svc.GetVersions(context.Background(), "not-found", false)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestService_List(t *testing.T) {
	store := newMockStore()
	store.packages["pkg-a@1.0.0"] = &storage.Package{Name: "pkg-a", Version: "1.0.0"}
	store.packages["pkg-b@1.0.0"] = &storage.Package{Name: "pkg-b", Version: "1.0.0"}

	svc := NewService(store, store)

	result, err := svc.List(context.Background(), ListFilter{}, PaginationParams{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, result.Packages, 2)
}

func TestService_Delete(t *testing.T) {
	store := newMockStore()
	store.packages["my-package@1.0.0"] = &storage.Package{Name: "my-package", Version: "1.0.0"}
	store.owners["my-package"] = "owner-123"

	svc := NewService(store, store)

	t.Run("owner can delete", func(t *testing.T) {
		err := svc.Delete(context.Background(), "my-package", "1.0.0", "owner-123")
		require.NoError(t, err)
	})

	t.Run("non-owner cannot delete", func(t *testing.T) {
		store.packages["my-package@2.0.0"] = &storage.Package{Name: "my-package", Version: "2.0.0"}
		err := svc.Delete(context.Background(), "my-package", "2.0.0", "owner-456")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrForbidden)
	})
}

func TestService_GetArtifact(t *testing.T) {
	store := newMockStore()
	store.packages["my-package@1.0.0"] = &storage.Package{
		ID:      "pkg-123",
		Name:    "my-package",
		Version: "1.0.0",
	}
	store.contracts["pkg-123/Token"] = &storage.Contract{
		ID:        "contract-456",
		PackageID: "pkg-123",
		Name:      "Token",
	}
	store.artifacts["contract-456/abi"] = []byte(`[{"type":"function"}]`)

	svc := NewService(store, store)

	t.Run("existing artifact", func(t *testing.T) {
		content, err := svc.GetArtifact(context.Background(), "my-package", "1.0.0", "Token", "abi")
		require.NoError(t, err)
		assert.Contains(t, string(content), "function")
	})

	t.Run("non-existing artifact", func(t *testing.T) {
		_, err := svc.GetArtifact(context.Background(), "my-package", "1.0.0", "Token", "bytecode")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestToPackage_TimestampParsing(t *testing.T) {
	tests := []struct {
		name         string
		createdAt    string
		wantYear     int
		wantZeroTime bool
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
			storagePkg := &storage.Package{
				ID:        "test-id",
				Name:      "test-package",
				Version:   "1.0.0",
				CreatedAt: tt.createdAt,
			}

			domainPkg := toPackage(storagePkg)

			if tt.wantZeroTime {
				assert.True(t, domainPkg.CreatedAt.IsZero(), "expected zero time for input: %q", tt.createdAt)
			} else {
				assert.False(t, domainPkg.CreatedAt.IsZero(), "expected non-zero time for input: %q", tt.createdAt)
				assert.Equal(t, tt.wantYear, domainPkg.CreatedAt.Year())
			}
		})
	}
}
