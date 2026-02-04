package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"log/slog"
)

func TestSQLiteStore(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "contrafactory-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	store, err := NewSQLiteStore(dbPath, logger)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Run migrations
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	t.Run("CreateAndGetPackage", func(t *testing.T) {
		pkg := &Package{
			ID:      "test-id-1",
			Name:    "test-package",
			Version: "1.0.0",
			Chain:   "evm",
			Builder: "foundry",
		}

		if err := store.CreatePackage(ctx, pkg); err != nil {
			t.Fatalf("CreatePackage() error = %v", err)
		}

		got, err := store.GetPackage(ctx, "test-package", "1.0.0")
		if err != nil {
			t.Fatalf("GetPackage() error = %v", err)
		}

		if got.Name != pkg.Name {
			t.Errorf("GetPackage().Name = %v, want %v", got.Name, pkg.Name)
		}
		if got.Version != pkg.Version {
			t.Errorf("GetPackage().Version = %v, want %v", got.Version, pkg.Version)
		}
	})

	t.Run("PackageExists", func(t *testing.T) {
		exists, err := store.PackageExists(ctx, "test-package", "1.0.0")
		if err != nil {
			t.Fatalf("PackageExists() error = %v", err)
		}
		if !exists {
			t.Error("PackageExists() = false, want true")
		}

		exists, err = store.PackageExists(ctx, "nonexistent", "1.0.0")
		if err != nil {
			t.Fatalf("PackageExists() error = %v", err)
		}
		if exists {
			t.Error("PackageExists() = true, want false")
		}
	})

	t.Run("GetPackageVersions", func(t *testing.T) {
		// Create another version
		pkg2 := &Package{
			ID:      "test-id-2",
			Name:    "test-package",
			Version: "1.1.0",
			Chain:   "evm",
			Builder: "foundry",
		}
		if err := store.CreatePackage(ctx, pkg2); err != nil {
			t.Fatalf("CreatePackage() error = %v", err)
		}

		versions, err := store.GetPackageVersions(ctx, "test-package", true)
		if err != nil {
			t.Fatalf("GetPackageVersions() error = %v", err)
		}

		if len(versions) != 2 {
			t.Errorf("GetPackageVersions() returned %d versions, want 2", len(versions))
		}
	})

	t.Run("CreateAndGetContract", func(t *testing.T) {
		contract := &Contract{
			ID:          "contract-id-1",
			PackageID:   "test-id-1",
			Name:        "Token",
			Chain:       "evm",
			SourcePath:  "src/Token.sol",
			PrimaryHash: "abc123",
		}

		if err := store.CreateContract(ctx, "test-id-1", contract); err != nil {
			t.Fatalf("CreateContract() error = %v", err)
		}

		got, err := store.GetContract(ctx, "test-id-1", "Token")
		if err != nil {
			t.Fatalf("GetContract() error = %v", err)
		}

		if got.Name != contract.Name {
			t.Errorf("GetContract().Name = %v, want %v", got.Name, contract.Name)
		}
	})

	t.Run("StoreAndGetArtifact", func(t *testing.T) {
		content := []byte(`{"type":"function","name":"transfer"}`)

		if err := store.StoreArtifact(ctx, "contract-id-1", "abi", content); err != nil {
			t.Fatalf("StoreArtifact() error = %v", err)
		}

		got, err := store.GetArtifact(ctx, "contract-id-1", "abi")
		if err != nil {
			t.Fatalf("GetArtifact() error = %v", err)
		}

		if string(got) != string(content) {
			t.Errorf("GetArtifact() = %s, want %s", got, content)
		}
	})

	t.Run("ListPackages", func(t *testing.T) {
		result, err := store.ListPackages(ctx, PackageFilter{}, PaginationParams{Limit: 10})
		if err != nil {
			t.Fatalf("ListPackages() error = %v", err)
		}

		if len(result.Data) == 0 {
			t.Error("ListPackages() returned empty result")
		}

		// Find test-package in results
		var found *Package
		for i := range result.Data {
			if result.Data[i].Name == "test-package" {
				found = &result.Data[i]
				break
			}
		}

		if found == nil {
			t.Fatal("ListPackages() did not return test-package")
		}

		// Verify aggregated fields
		if found.Chain != "evm" {
			t.Errorf("ListPackages().Chain = %v, want evm", found.Chain)
		}

		if found.Builder != "foundry" {
			t.Errorf("ListPackages().Builder = %v, want foundry", found.Builder)
		}

		// Should have 2 versions (1.0.0 and 1.1.0)
		if len(found.Versions) != 2 {
			t.Errorf("ListPackages().Versions has %d items, want 2", len(found.Versions))
		}

		// Versions should include both 1.0.0 and 1.1.0
		hasV1 := false
		hasV11 := false
		for _, v := range found.Versions {
			if v == "1.0.0" {
				hasV1 = true
			}
			if v == "1.1.0" {
				hasV11 = true
			}
		}
		if !hasV1 || !hasV11 {
			t.Errorf("ListPackages().Versions = %v, want [1.0.0, 1.1.0]", found.Versions)
		}
	})

	t.Run("DeletePackage", func(t *testing.T) {
		if err := store.DeletePackage(ctx, "test-package", "1.1.0"); err != nil {
			t.Fatalf("DeletePackage() error = %v", err)
		}

		exists, _ := store.PackageExists(ctx, "test-package", "1.1.0")
		if exists {
			t.Error("Package still exists after deletion")
		}
	})
}

func TestAPIKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contrafactory-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	store, err := NewSQLiteStore(dbPath, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	store.Migrate(ctx)

	t.Run("CreateAndValidateAPIKey", func(t *testing.T) {
		key, err := store.CreateAPIKey(ctx, "test-key")
		if err != nil {
			t.Fatalf("CreateAPIKey() error = %v", err)
		}

		if key == "" {
			t.Fatal("CreateAPIKey() returned empty key")
		}

		apiKey, err := store.ValidateAPIKey(ctx, key)
		if err != nil {
			t.Fatalf("ValidateAPIKey() error = %v", err)
		}

		if apiKey.Name != "test-key" {
			t.Errorf("ValidateAPIKey().Name = %v, want test-key", apiKey.Name)
		}
	})

	t.Run("InvalidAPIKey", func(t *testing.T) {
		_, err := store.ValidateAPIKey(ctx, "invalid-key")
		if err == nil {
			t.Error("ValidateAPIKey() should return error for invalid key")
		}
	})
}
