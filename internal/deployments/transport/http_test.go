package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/pendergraft/contrafactory/internal/deployments/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockService implements domain.Service for testing
type mockService struct {
	deployments map[string]*domain.Deployment
}

func newMockService() *mockService {
	return &mockService{
		deployments: make(map[string]*domain.Deployment),
	}
}

func (m *mockService) Record(ctx context.Context, req domain.RecordRequest) (*domain.Deployment, error) {
	d := &domain.Deployment{
		ID:       "deploy-new",
		ChainID:  "1",
		Address:  req.Address,
		Verified: false,
	}
	key := d.ChainID + "/" + d.Address
	m.deployments[key] = d
	return d, nil
}

func (m *mockService) Get(ctx context.Context, chainID, address string) (*domain.Deployment, error) {
	key := chainID + "/" + address
	if d, ok := m.deployments[key]; ok {
		return d, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockService) List(ctx context.Context, filter domain.ListFilter, pagination domain.PaginationParams) (*domain.ListResult, error) {
	var deployments []domain.Deployment
	for _, d := range m.deployments {
		deployments = append(deployments, *d)
	}
	return &domain.ListResult{Deployments: deployments}, nil
}

func (m *mockService) UpdateVerificationStatus(ctx context.Context, chainID, address string, verified bool, verifiedOn []string) error {
	key := chainID + "/" + address
	if d, ok := m.deployments[key]; ok {
		d.Verified = verified
		d.VerifiedOn = verifiedOn
		return nil
	}
	return domain.ErrNotFound
}

func (m *mockService) ListByPackage(ctx context.Context, packageName, version string) ([]domain.DeploymentSummary, error) {
	var summaries []domain.DeploymentSummary
	for _, d := range m.deployments {
		summaries = append(summaries, domain.DeploymentSummary{
			ChainID:      d.ChainID,
			Address:      d.Address,
			ContractName: d.ContractName,
			Verified:     d.Verified,
			TxHash:       d.TxHash,
		})
	}
	return summaries, nil
}

func setupRouter(svc domain.Service) *chi.Mux {
	r := chi.NewRouter()
	h := NewHandler(svc)
	r.Route("/deployments", func(r chi.Router) {
		h.RegisterRoutes(r)
	})
	return r
}

func TestHandler_List(t *testing.T) {
	svc := newMockService()
	svc.deployments["1/0x1234567890abcdef1234567890abcdef12345678"] = &domain.Deployment{
		ID:      "deploy-1",
		ChainID: "1",
		Address: "0x1234567890abcdef1234567890abcdef12345678",
	}

	router := setupRouter(svc)

	req := httptest.NewRequest("GET", "/deployments/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp, "data")
	assert.Contains(t, resp, "pagination")
}

func TestHandler_Record(t *testing.T) {
	svc := newMockService()
	router := setupRouter(svc)

	body := `{
		"package": "my-pkg",
		"version": "1.0.0",
		"contract": "Token",
		"chainId": 1,
		"address": "0x1234567890abcdef1234567890abcdef12345678",
		"txHash": "0xabcdef"
	}`

	req := httptest.NewRequest("POST", "/deployments/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "0x1234567890abcdef1234567890abcdef12345678", resp["address"])
}

func TestHandler_Get(t *testing.T) {
	svc := newMockService()
	svc.deployments["1/0x1234567890abcdef1234567890abcdef12345678"] = &domain.Deployment{
		ID:           "deploy-1",
		ChainID:      "1",
		Address:      "0x1234567890abcdef1234567890abcdef12345678",
		ContractName: "Token",
		Verified:     true,
		VerifiedOn:   []string{"etherscan"},
	}

	router := setupRouter(svc)

	t.Run("existing deployment", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/deployments/1/0x1234567890abcdef1234567890abcdef12345678", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "1", resp["chainId"])
		assert.Equal(t, "0x1234567890abcdef1234567890abcdef12345678", resp["address"])
		assert.Equal(t, true, resp["verified"])
	})

	t.Run("non-existing deployment", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/deployments/1/0x0000000000000000000000000000000000000000", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestHandler_Record_InvalidJSON(t *testing.T) {
	svc := newMockService()
	router := setupRouter(svc)

	req := httptest.NewRequest("POST", "/deployments/", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
