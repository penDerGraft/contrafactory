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
			ID:               "test-id-1",
			Name:             "test-package",
			Version:          "1.0.0",
			Project:          "my-project",
			Chain:            "evm",
			Builder:          "foundry",
			CompilerVersion:  "0.8.28+commit.7893614a",
			CompilerSettings: map[string]any{"evmVersion": "paris", "viaIR": false, "optimizer": map[string]any{"enabled": true, "runs": 200}},
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
		if got.Project != pkg.Project {
			t.Errorf("GetPackage().Project = %v, want %v", got.Project, pkg.Project)
		}
		if got.CompilerVersion != pkg.CompilerVersion {
			t.Errorf("GetPackage().CompilerVersion = %v, want %v", got.CompilerVersion, pkg.CompilerVersion)
		}
		if evm, ok := got.CompilerSettings["evmVersion"].(string); !ok || evm != "paris" {
			t.Errorf("GetPackage().CompilerSettings[evmVersion] = %v, want paris", got.CompilerSettings["evmVersion"])
		}
		if opt, ok := got.CompilerSettings["optimizer"].(map[string]any); ok {
			if runs, ok := opt["runs"].(float64); ok && int(runs) != 200 {
				t.Errorf("GetPackage().CompilerSettings[optimizer].runs = %v, want 200", runs)
			}
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

func TestListPackagesFilters(t *testing.T) {
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
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Create packages: pkg-a and pkg-b in project "proj1", pkg-c in project "proj2"
	// pkg-a has versions 1.0.0, 1.1.0, 2.0.0; pkg-b has 1.0.0; pkg-c has 1.0.0
	for _, p := range []struct {
		id, name, version, project string
	}{
		{"id-a1", "pkg-a", "1.0.0", "proj1"},
		{"id-a2", "pkg-a", "1.1.0", "proj1"},
		{"id-a3", "pkg-a", "2.0.0", "proj1"},
		{"id-b1", "pkg-b", "1.0.0", "proj1"},
		{"id-c1", "pkg-c", "1.0.0", "proj2"},
	} {
		pkg := &Package{ID: p.id, Name: p.name, Version: p.version, Project: p.project, Chain: "evm", Builder: "foundry"}
		if err := store.CreatePackage(ctx, pkg); err != nil {
			t.Fatalf("CreatePackage %s@%s: %v", p.name, p.version, err)
		}
	}

	// Create contracts: Token in pkg-a, Registry in pkg-b
	if err := store.CreateContract(ctx, "id-a1", &Contract{ID: "c1", PackageID: "id-a1", Name: "Token", Chain: "evm", SourcePath: "src/Token.sol", PrimaryHash: "h1"}); err != nil {
		t.Fatalf("CreateContract: %v", err)
	}
	if err := store.CreateContract(ctx, "id-b1", &Contract{ID: "c2", PackageID: "id-b1", Name: "Registry", Chain: "evm", SourcePath: "src/Registry.sol", PrimaryHash: "h2"}); err != nil {
		t.Fatalf("CreateContract: %v", err)
	}
	// pkg-a@1.1.0 also has Token (different package_id)
	if err := store.CreateContract(ctx, "id-a2", &Contract{ID: "c3", PackageID: "id-a2", Name: "Token", Chain: "evm", SourcePath: "src/Token.sol", PrimaryHash: "h3"}); err != nil {
		t.Fatalf("CreateContract: %v", err)
	}

	t.Run("project filter", func(t *testing.T) {
		result, err := store.ListPackages(ctx, PackageFilter{Project: "proj1"}, PaginationParams{Limit: 10})
		if err != nil {
			t.Fatalf("ListPackages() error = %v", err)
		}
		names := make([]string, len(result.Data))
		for i, p := range result.Data {
			names[i] = p.Name
		}
		if len(result.Data) != 2 {
			t.Errorf("ListPackages(project=proj1) returned %d packages, want 2 (pkg-a, pkg-b)", len(result.Data))
		}
		if !contains(names, "pkg-a") || !contains(names, "pkg-b") {
			t.Errorf("ListPackages(project=proj1) = %v, want pkg-a and pkg-b", names)
		}
	})

	t.Run("version filter", func(t *testing.T) {
		result, err := store.ListPackages(ctx, PackageFilter{Version: "1.0.0"}, PaginationParams{Limit: 10})
		if err != nil {
			t.Fatalf("ListPackages() error = %v", err)
		}
		names := make([]string, len(result.Data))
		for i, p := range result.Data {
			names[i] = p.Name
		}
		if len(result.Data) != 3 {
			t.Errorf("ListPackages(version=1.0.0) returned %d packages, want 3 (pkg-a, pkg-b, pkg-c)", len(result.Data))
		}
		if !contains(names, "pkg-a") || !contains(names, "pkg-b") || !contains(names, "pkg-c") {
			t.Errorf("ListPackages(version=1.0.0) = %v", names)
		}
	})

	t.Run("contract filter", func(t *testing.T) {
		result, err := store.ListPackages(ctx, PackageFilter{Contract: "Token"}, PaginationParams{Limit: 10})
		if err != nil {
			t.Fatalf("ListPackages() error = %v", err)
		}
		if len(result.Data) != 1 {
			t.Errorf("ListPackages(contract=Token) returned %d packages, want 1", len(result.Data))
		}
		if len(result.Data) > 0 && result.Data[0].Name != "pkg-a" {
			t.Errorf("ListPackages(contract=Token) = %v, want pkg-a", result.Data[0].Name)
		}
	})

	t.Run("contract filter case insensitive", func(t *testing.T) {
		result, err := store.ListPackages(ctx, PackageFilter{Contract: "registry"}, PaginationParams{Limit: 10})
		if err != nil {
			t.Fatalf("ListPackages() error = %v", err)
		}
		if len(result.Data) != 1 || result.Data[0].Name != "pkg-b" {
			t.Errorf("ListPackages(contract=registry) = %v, want pkg-b", result.Data)
		}
	})

	t.Run("project and latest", func(t *testing.T) {
		result, err := store.ListPackages(ctx, PackageFilter{Project: "proj1", Latest: true}, PaginationParams{Limit: 10})
		if err != nil {
			t.Fatalf("ListPackages() error = %v", err)
		}
		for _, p := range result.Data {
			if p.Name == "pkg-a" && len(p.Versions) != 1 {
				t.Errorf("pkg-a with latest should have 1 version, got %v", p.Versions)
			}
			if p.Name == "pkg-a" && len(p.Versions) == 1 && p.Versions[0] != "2.0.0" {
				t.Errorf("pkg-a latest version = %v, want 2.0.0", p.Versions[0])
			}
		}
	})
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
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
