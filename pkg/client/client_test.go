package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListPackages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/packages" {
			t.Errorf("Expected path /api/v1/packages, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"name": "test-package"},
			},
			"pagination": map[string]any{
				"limit":   20,
				"hasMore": false,
			},
		})
	}))
	defer server.Close()

	client := New(server.URL, "test-key")
	resp, err := client.ListPackages(context.Background())
	if err != nil {
		t.Fatalf("ListPackages() error = %v", err)
	}

	if len(resp.Data) != 1 {
		t.Errorf("ListPackages() returned %d packages, want 1", len(resp.Data))
	}
	if resp.Data[0].Name != "test-package" {
		t.Errorf("ListPackages()[0].Name = %s, want test-package", resp.Data[0].Name)
	}
}

func TestClient_GetPackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/packages/my-package" {
			t.Errorf("Expected path /api/v1/packages/my-package, got %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"name":     "my-package",
			"versions": []string{"1.0.0", "1.1.0"},
		})
	}))
	defer server.Close()

	client := New(server.URL, "")
	pkg, err := client.GetPackage(context.Background(), "my-package")
	if err != nil {
		t.Fatalf("GetPackage() error = %v", err)
	}

	if pkg.Name != "my-package" {
		t.Errorf("GetPackage().Name = %s, want my-package", pkg.Name)
	}
	if len(pkg.Versions) != 2 {
		t.Errorf("GetPackage().Versions has %d items, want 2", len(pkg.Versions))
	}
}

func TestClient_Publish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/packages/my-package/1.0.0" {
			t.Errorf("Expected path /api/v1/packages/my-package/1.0.0, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "my-api-key" {
			t.Errorf("Expected X-API-Key header, got %s", r.Header.Get("X-API-Key"))
		}

		var req PublishRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}

		if req.Chain != "evm" {
			t.Errorf("Expected chain evm, got %s", req.Chain)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "success",
		})
	}))
	defer server.Close()

	client := New(server.URL, "my-api-key")
	err := client.Publish(context.Background(), "my-package", "1.0.0", PublishRequest{
		Chain: "evm",
		Artifacts: []Artifact{
			{
				Name:     "Token",
				Bytecode: "0x608060405234801561001057600080fd5b50",
			},
		},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}

func TestClient_GetDeployment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/deployments/31337/0x1234567890abcdef1234567890abcdef12345678" {
			t.Errorf("Expected path /api/v1/deployments/31337/0x1234567890abcdef1234567890abcdef12345678, got %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"id":              "deploy-123",
			"packageId":       "pkg-456",
			"contractName":    "Token",
			"chainId":         "31337",
			"address":         "0x1234567890abcdef1234567890abcdef12345678",
			"blockNumber":     12345,
			"verified":        true,
			"verifiedOn":      []string{"etherscan", "blockscout"},
			"createdAt":       "2024-01-15T10:30:00Z",
		})
	}))
	defer server.Close()

	client := New(server.URL, "")
	deployment, err := client.GetDeployment(context.Background(), "31337", "0x1234567890abcdef1234567890abcdef12345678")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}

	if deployment.ID != "deploy-123" {
		t.Errorf("GetDeployment().ID = %s, want deploy-123", deployment.ID)
	}
	if deployment.ContractName != "Token" {
		t.Errorf("GetDeployment().ContractName = %s, want Token", deployment.ContractName)
	}
	if !deployment.Verified {
		t.Errorf("GetDeployment().Verified = false, want true")
	}
	if len(deployment.VerifiedOn) != 2 {
		t.Errorf("GetDeployment().VerifiedOn has %d items, want 2", len(deployment.VerifiedOn))
	}
	if deployment.VerifiedOn[0] != "etherscan" || deployment.VerifiedOn[1] != "blockscout" {
		t.Errorf("GetDeployment().VerifiedOn = %v, want [etherscan blockscout]", deployment.VerifiedOn)
	}
}

func TestClient_GetDeployment_EmptyVerifiedOn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":           "deploy-456",
			"contractName": "Token",
			"chainId":      "31337",
			"address":      "0x1234567890abcdef1234567890abcdef12345678",
			"verified":     false,
			"verifiedOn":   []string{},
			"createdAt":    "2024-01-15T10:30:00Z",
		})
	}))
	defer server.Close()

	client := New(server.URL, "")
	deployment, err := client.GetDeployment(context.Background(), "31337", "0x1234567890abcdef1234567890abcdef12345678")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}

	if deployment.VerifiedOn == nil {
		t.Error("GetDeployment().VerifiedOn should not be nil when API returns empty array")
	}
	if len(deployment.VerifiedOn) != 0 {
		t.Errorf("GetDeployment().VerifiedOn = %v, want empty slice", deployment.VerifiedOn)
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "NOT_FOUND",
				"message": "Package not found",
			},
		})
	}))
	defer server.Close()

	client := New(server.URL, "")
	_, err := client.GetPackage(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Expected error for 404 response")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected APIError, got %T", err)
	}
	if apiErr.Code != "NOT_FOUND" {
		t.Errorf("Expected code NOT_FOUND, got %s", apiErr.Code)
	}
}
