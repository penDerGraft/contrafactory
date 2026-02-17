package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeletePackage(t *testing.T) {
	tests := []struct {
		name           string
		packageName    string
		version        string
		handler        http.HandlerFunc
		wantErr        bool
		wantErrContain string
	}{
		{
			name:        "success",
			packageName: "my-token",
			version:     "1.0.0",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodDelete, r.Method)
				assert.Equal(t, "/api/v1/packages/my-token/1.0.0", r.URL.Path)
				assert.NotEmpty(t, r.Header.Get("X-API-Key"))
				w.WriteHeader(http.StatusNoContent)
			},
			wantErr: false,
		},
		{
			name:        "URL escaping for special characters",
			packageName: "pkg@scope",
			version:     "1.0.0-beta",
			handler: func(w http.ResponseWriter, r *http.Request) {
				// PathEscape encodes @ as %40, etc.
				assert.Contains(t, r.URL.Path, "pkg")
				assert.Contains(t, r.URL.Path, "1.0.0")
				w.WriteHeader(http.StatusNoContent)
			},
			wantErr: false,
		},
		{
			name:        "not found",
			packageName: "nonexistent",
			version:     "1.0.0",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    "NOT_FOUND",
						"message": "package not found",
					},
				})
			},
			wantErr:        true,
			wantErrContain: "NOT_FOUND",
		},
		{
			name:        "server error",
			packageName: "my-token",
			version:     "1.0.0",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("internal error"))
			},
			wantErr:        true,
			wantErrContain: "500",
		},
		{
			name:        "error response with code",
			packageName: "my-token",
			version:     "1.0.0",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    "FORBIDDEN",
						"message": "not authorized",
					},
				})
			},
			wantErr:        true,
			wantErrContain: "FORBIDDEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			err := deletePackage(server.URL, "test-api-key", tt.packageName, tt.version)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
