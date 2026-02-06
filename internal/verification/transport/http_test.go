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

	"github.com/pendergraft/contrafactory/internal/verification/domain"
)

// mockService implements Service for testing
type mockService struct {
	results map[string]*domain.VerifyResult
}

func newMockService() *mockService {
	return &mockService{
		results: make(map[string]*domain.VerifyResult),
	}
}

func (m *mockService) Verify(ctx context.Context, req domain.VerifyRequest) (*domain.VerifyResult, error) {
	key := req.Package + "@" + req.Version + "/" + req.Contract
	if result, ok := m.results[key]; ok {
		return result, nil
	}
	// Default: return a pending result
	return &domain.VerifyResult{
		Verified:  false,
		MatchType: "pending",
		Message:   "Verification pending",
	}, nil
}

func setupRouter(svc Service) *chi.Mux {
	r := chi.NewRouter()
	h := NewHandler(svc)
	h.RegisterRoutes(r)
	return r
}

func TestHandler_Verify(t *testing.T) {
	svc := newMockService()
	svc.results["my-pkg@1.0.0/Token"] = &domain.VerifyResult{
		Verified:  true,
		MatchType: "full",
		Message:   "Bytecode matches",
	}

	router := setupRouter(svc)

	t.Run("successful verification", func(t *testing.T) {
		body := `{
			"package": "my-pkg",
			"version": "1.0.0",
			"contract": "Token",
			"chainId": 1,
			"address": "0x1234567890abcdef1234567890abcdef12345678"
		}`

		req := httptest.NewRequest("POST", "/verify", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp domain.VerifyResult
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Verified)
		assert.Equal(t, "full", resp.MatchType)
	})

	t.Run("pending verification", func(t *testing.T) {
		body := `{
			"package": "other-pkg",
			"version": "1.0.0",
			"contract": "Other",
			"chainId": 1,
			"address": "0x1234567890abcdef1234567890abcdef12345678"
		}`

		req := httptest.NewRequest("POST", "/verify", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp domain.VerifyResult
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp.Verified)
		assert.Equal(t, "pending", resp.MatchType)
	})
}

func TestHandler_Verify_InvalidJSON(t *testing.T) {
	svc := newMockService()
	router := setupRouter(svc)

	req := httptest.NewRequest("POST", "/verify", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp, "error")
}

func TestHandler_Verify_MissingFields(t *testing.T) {
	svc := newMockService()
	router := setupRouter(svc)

	// Empty request body
	body := `{}`

	req := httptest.NewRequest("POST", "/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should still work - service handles validation
	assert.Equal(t, http.StatusOK, rec.Code)
}
