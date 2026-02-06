package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pendergraft/contrafactory/internal/packages/domain"
)

// mockService implements Service for testing
type mockService struct {
	packages  map[string]*domain.Package
	contracts map[string][]domain.Contract
	artifacts map[string][]byte
}

func newMockService() *mockService {
	return &mockService{
		packages:  make(map[string]*domain.Package),
		contracts: make(map[string][]domain.Contract),
		artifacts: make(map[string][]byte),
	}
}

func (m *mockService) Publish(ctx context.Context, name, version string, ownerID string, req domain.PublishRequest) error {
	key := name + "@" + version
	m.packages[key] = &domain.Package{
		Name:    name,
		Version: version,
		Chain:   req.Chain,
	}
	return nil
}

func (m *mockService) Get(ctx context.Context, name, version string) (*domain.Package, error) {
	key := name + "@" + version
	if pkg, ok := m.packages[key]; ok {
		return pkg, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockService) GetVersions(ctx context.Context, name string, includePrerelease bool) (*domain.VersionsResult, error) {
	var versions []string
	for key := range m.packages {
		if m.packages[key].Name == name {
			versions = append(versions, m.packages[key].Version)
		}
	}
	if len(versions) == 0 {
		return nil, domain.ErrNotFound
	}
	return &domain.VersionsResult{Name: name, Versions: versions}, nil
}

func (m *mockService) List(ctx context.Context, filter domain.ListFilter, pagination domain.PaginationParams) (*domain.ListResult, error) {
	var packages []domain.Package
	for _, pkg := range m.packages {
		packages = append(packages, *pkg)
	}
	return &domain.ListResult{Packages: packages}, nil
}

func (m *mockService) Delete(ctx context.Context, name, version string, ownerID string) error {
	key := name + "@" + version
	delete(m.packages, key)
	return nil
}

func (m *mockService) GetContracts(ctx context.Context, name, version string) ([]domain.Contract, error) {
	key := name + "@" + version
	if contracts, ok := m.contracts[key]; ok {
		return contracts, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockService) GetContract(ctx context.Context, name, version, contractName string) (*domain.Contract, error) {
	key := name + "@" + version
	if contracts, ok := m.contracts[key]; ok {
		for _, c := range contracts {
			if c.Name == contractName {
				return &c, nil
			}
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockService) GetArtifact(ctx context.Context, name, version, contractName, artifactType string) ([]byte, error) {
	key := name + "@" + version + "/" + contractName + "/" + artifactType
	if content, ok := m.artifacts[key]; ok {
		return content, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockService) GetArchive(ctx context.Context, name, version string) ([]byte, error) {
	key := name + "@" + version
	if _, ok := m.packages[key]; ok {
		// Return a minimal valid gzip/tar
		return []byte{0x1f, 0x8b, 0x08, 0x00}, nil
	}
	return nil, domain.ErrNotFound
}

func setupRouter(svc Service) *chi.Mux {
	r := chi.NewRouter()
	h := NewHandler(svc)
	r.Route("/packages", func(r chi.Router) {
		h.RegisterRoutes(r)
	})
	return r
}

func TestHandler_List(t *testing.T) {
	svc := newMockService()
	svc.packages["test-pkg@1.0.0"] = &domain.Package{Name: "test-pkg", Version: "1.0.0", Chain: "evm"}

	router := setupRouter(svc)

	req := httptest.NewRequest("GET", "/packages/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp, "data")
	assert.Contains(t, resp, "pagination")
}

func TestHandler_GetVersions(t *testing.T) {
	svc := newMockService()
	svc.packages["test-pkg@1.0.0"] = &domain.Package{Name: "test-pkg", Version: "1.0.0"}
	svc.packages["test-pkg@2.0.0"] = &domain.Package{Name: "test-pkg", Version: "2.0.0"}

	router := setupRouter(svc)

	t.Run("existing package", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/packages/test-pkg", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "test-pkg", resp["name"])
		assert.Len(t, resp["versions"], 2)
	})

	t.Run("non-existing package", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/packages/not-found", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestHandler_Get(t *testing.T) {
	svc := newMockService()
	svc.packages["test-pkg@1.0.0"] = &domain.Package{
		Name:    "test-pkg",
		Version: "1.0.0",
		Chain:   "evm",
		Builder: "foundry",
	}
	svc.contracts["test-pkg@1.0.0"] = []domain.Contract{
		{Name: "Token"},
	}

	router := setupRouter(svc)

	t.Run("existing version", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/packages/test-pkg/1.0.0", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "test-pkg", resp["name"])
		assert.Equal(t, "1.0.0", resp["version"])
	})

	t.Run("non-existing version", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/packages/test-pkg/9.9.9", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestHandler_Publish(t *testing.T) {
	svc := newMockService()
	router := setupRouter(svc)

	body := `{
		"chain": "evm",
		"artifacts": [
			{"name": "Token", "bytecode": "0x1234"}
		]
	}`

	req := httptest.NewRequest("POST", "/packages/new-pkg/1.0.0", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "new-pkg", resp["name"])
	assert.Equal(t, "1.0.0", resp["version"])
}

func TestHandler_Delete(t *testing.T) {
	svc := newMockService()
	svc.packages["test-pkg@1.0.0"] = &domain.Package{Name: "test-pkg", Version: "1.0.0"}

	router := setupRouter(svc)

	req := httptest.NewRequest("DELETE", "/packages/test-pkg/1.0.0", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_GetArtifact(t *testing.T) {
	svc := newMockService()
	svc.packages["test-pkg@1.0.0"] = &domain.Package{Name: "test-pkg", Version: "1.0.0"}
	svc.contracts["test-pkg@1.0.0"] = []domain.Contract{{Name: "Token"}}
	svc.artifacts["test-pkg@1.0.0/Token/abi"] = []byte(`[{"type":"function"}]`)

	router := setupRouter(svc)

	t.Run("existing artifact", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/packages/test-pkg/1.0.0/contracts/Token/abi", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "function")
	})

	t.Run("non-existing artifact", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/packages/test-pkg/1.0.0/contracts/Token/bytecode", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
