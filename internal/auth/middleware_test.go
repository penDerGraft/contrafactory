package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pendergraft/contrafactory/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAPIKeyStore struct {
	keys map[string]*storage.APIKey
}

func (m *mockAPIKeyStore) CreateAPIKey(ctx context.Context, name string) (string, error) {
	return "", nil
}

func (m *mockAPIKeyStore) ValidateAPIKey(ctx context.Context, key string) (*storage.APIKey, error) {
	if apiKey, ok := m.keys[key]; ok {
		return apiKey, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockAPIKeyStore) ListAPIKeys(ctx context.Context) ([]storage.APIKey, error) {
	return nil, nil
}

func (m *mockAPIKeyStore) RevokeAPIKey(ctx context.Context, id string) error {
	return nil
}

func TestMiddleware_ValidKey(t *testing.T) {
	store := &mockAPIKeyStore{
		keys: map[string]*storage.APIKey{
			"cf_key_valid": {ID: "key-123", Name: "test"},
		},
	}

	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	middleware := Middleware(store, func(w http.ResponseWriter, status int, code, message string) {
		w.WriteHeader(status)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "cf_key_valid")
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	apiKey := GetAPIKeyFromContext(capturedCtx)
	require.NotNil(t, apiKey)
	assert.Equal(t, "key-123", apiKey.ID)
}

func TestMiddleware_InvalidKey(t *testing.T) {
	store := &mockAPIKeyStore{
		keys: map[string]*storage.APIKey{},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := Middleware(store, func(w http.ResponseWriter, status int, code, message string) {
		w.WriteHeader(status)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "cf_key_invalid")
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_MissingKey(t *testing.T) {
	store := &mockAPIKeyStore{
		keys: map[string]*storage.APIKey{},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := Middleware(store, func(w http.ResponseWriter, status int, code, message string) {
		w.WriteHeader(status)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_BearerToken(t *testing.T) {
	store := &mockAPIKeyStore{
		keys: map[string]*storage.APIKey{
			"cf_key_bearer": {ID: "key-456", Name: "bearer-test"},
		},
	}

	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	middleware := Middleware(store, func(w http.ResponseWriter, status int, code, message string) {
		w.WriteHeader(status)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer cf_key_bearer")
	rec := httptest.NewRecorder()

	middleware(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	apiKey := GetAPIKeyFromContext(capturedCtx)
	require.NotNil(t, apiKey)
	assert.Equal(t, "key-456", apiKey.ID)
}

func TestGenerateAPIKey(t *testing.T) {
	key, err := GenerateAPIKey()
	require.NoError(t, err)
	assert.True(t, len(key) > len(KeyPrefix))
	assert.Equal(t, KeyPrefix, key[:len(KeyPrefix)])
}

func TestHashAPIKey(t *testing.T) {
	hash := HashAPIKey("cf_key_test")
	assert.Len(t, hash, 64) // SHA256 hex = 64 chars

	// Same key should produce same hash
	hash2 := HashAPIKey("cf_key_test")
	assert.Equal(t, hash, hash2)

	// Different key should produce different hash
	hash3 := HashAPIKey("cf_key_different")
	assert.NotEqual(t, hash, hash3)
}
