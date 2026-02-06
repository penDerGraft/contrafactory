// Package domain contains the business logic for package management.
package domain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pendergraft/contrafactory/internal/storage"
	"github.com/pendergraft/contrafactory/internal/validation"
)

// Common errors returned by the package service.
var (
	ErrNotFound       = errors.New("package not found")
	ErrVersionExists  = errors.New("version already exists")
	ErrForbidden      = errors.New("not authorized to modify this package")
	ErrInvalidVersion = errors.New("invalid semver version")
	ErrInvalidName    = errors.New("invalid package name")
)

// PackageStore defines the storage operations needed by the packages domain.
type PackageStore interface {
	CreatePackage(ctx context.Context, pkg *storage.Package) error
	GetPackage(ctx context.Context, name, version string) (*storage.Package, error)
	GetPackageVersions(ctx context.Context, name string, includePrerelease bool) ([]string, error)
	ListPackages(ctx context.Context, filter storage.PackageFilter, pagination storage.PaginationParams) (*storage.PaginatedResult[storage.Package], error)
	DeletePackage(ctx context.Context, name, version string) error
	PackageExists(ctx context.Context, name, version string) (bool, error)
	GetPackageOwner(ctx context.Context, name string) (string, error)
	SetPackageOwner(ctx context.Context, name, ownerKeyID string) error
}

// ContractStore defines the contract and artifact storage operations needed by the packages domain.
type ContractStore interface {
	CreateContract(ctx context.Context, packageID string, contract *storage.Contract) error
	GetContract(ctx context.Context, packageID, contractName string) (*storage.Contract, error)
	ListContracts(ctx context.Context, packageID string) ([]storage.Contract, error)
	StoreArtifact(ctx context.Context, contractID, artifactType string, content []byte) error
	GetArtifact(ctx context.Context, contractID, artifactType string) ([]byte, error)
}

type service struct {
	packages  PackageStore
	contracts ContractStore
}

// NewService creates a new package service.
func NewService(packages PackageStore, contracts ContractStore) *service {
	return &service{
		packages:  packages,
		contracts: contracts,
	}
}

// Publish publishes a new package version.
func (s *service) Publish(ctx context.Context, name, version string, ownerID string, req PublishRequest) error {
	// Validate package name
	if err := validation.ValidatePackageName(name); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidName, err)
	}

	// Validate and normalize version
	if err := validation.ValidateVersion(version); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidVersion, err)
	}
	version = validation.NormalizeVersion(version)

	// Check package ownership
	currentOwner, err := s.packages.GetPackageOwner(ctx, name)
	if err != nil {
		return fmt.Errorf("checking ownership: %w", err)
	}
	if currentOwner != "" && currentOwner != ownerID {
		return ErrForbidden
	}

	// Check if version already exists
	exists, err := s.packages.PackageExists(ctx, name, version)
	if err != nil {
		return fmt.Errorf("checking existence: %w", err)
	}
	if exists {
		return ErrVersionExists
	}

	// Create package
	pkg := &storage.Package{
		ID:       generateID(),
		Name:     name,
		Version:  version,
		Chain:    req.Chain,
		Builder:  req.Builder,
		Metadata: req.Metadata,
		OwnerID:  ownerID,
	}

	if err := s.packages.CreatePackage(ctx, pkg); err != nil {
		return fmt.Errorf("creating package: %w", err)
	}

	// Set package ownership (first publisher owns the package)
	if ownerID != "" {
		if err := s.packages.SetPackageOwner(ctx, name, ownerID); err != nil {
			// Log but don't fail - ownership is best-effort
			_ = err
		}
	}

	// Create contracts and store artifacts
	for _, artifact := range req.Artifacts {
		contract := &storage.Contract{
			ID:          generateID(),
			PackageID:   pkg.ID,
			Name:        artifact.Name,
			Chain:       req.Chain,
			SourcePath:  artifact.SourcePath,
			PrimaryHash: computeHash([]byte(artifact.Bytecode)),
		}

		if err := s.contracts.CreateContract(ctx, pkg.ID, contract); err != nil {
			return fmt.Errorf("creating contract %s: %w", artifact.Name, err)
		}

		// Store artifacts
		if artifact.ABI != nil {
			if err := s.contracts.StoreArtifact(ctx, contract.ID, "abi", artifact.ABI); err != nil {
				return fmt.Errorf("storing ABI for %s: %w", artifact.Name, err)
			}
		}
		if artifact.Bytecode != "" {
			if err := s.contracts.StoreArtifact(ctx, contract.ID, "bytecode", []byte(artifact.Bytecode)); err != nil {
				return fmt.Errorf("storing bytecode for %s: %w", artifact.Name, err)
			}
		}
		if artifact.DeployedBytecode != "" {
			if err := s.contracts.StoreArtifact(ctx, contract.ID, "deployed-bytecode", []byte(artifact.DeployedBytecode)); err != nil {
				return fmt.Errorf("storing deployed bytecode for %s: %w", artifact.Name, err)
			}
		}
		if artifact.StandardJSONInput != nil {
			if err := s.contracts.StoreArtifact(ctx, contract.ID, "standard-json-input", artifact.StandardJSONInput); err != nil {
				return fmt.Errorf("storing standard JSON input for %s: %w", artifact.Name, err)
			}
		}
		if artifact.StorageLayout != nil {
			if err := s.contracts.StoreArtifact(ctx, contract.ID, "storage-layout", artifact.StorageLayout); err != nil {
				return fmt.Errorf("storing storage layout for %s: %w", artifact.Name, err)
			}
		}
	}

	return nil
}

// Get retrieves a specific package version.
func (s *service) Get(ctx context.Context, name, version string) (*Package, error) {
	// Handle "latest" version
	if version == "latest" {
		versions, err := s.packages.GetPackageVersions(ctx, name, false)
		if err != nil {
			return nil, fmt.Errorf("getting versions: %w", err)
		}
		if len(versions) == 0 {
			return nil, ErrNotFound
		}
		version = validation.ResolveLatest(versions, false)
	}

	pkg, err := s.packages.GetPackage(ctx, name, version)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting package: %w", err)
	}

	return toPackage(pkg), nil
}

// GetVersions retrieves all versions of a package.
func (s *service) GetVersions(ctx context.Context, name string, includePrerelease bool) (*VersionsResult, error) {
	versions, err := s.packages.GetPackageVersions(ctx, name, includePrerelease)
	if err != nil {
		return nil, fmt.Errorf("getting versions: %w", err)
	}

	if len(versions) == 0 {
		return nil, ErrNotFound
	}

	// Get chain/builder from the latest version
	var chain, builder string
	if len(versions) > 0 {
		latestVersion := validation.ResolveLatest(versions, includePrerelease)
		if latestVersion != "" {
			pkg, err := s.packages.GetPackage(ctx, name, latestVersion)
			if err == nil {
				chain = pkg.Chain
				builder = pkg.Builder
			}
		}
	}

	return &VersionsResult{
		Name:     name,
		Chain:    chain,
		Builder:  builder,
		Versions: versions,
	}, nil
}

// List lists packages with filtering and pagination.
func (s *service) List(ctx context.Context, filter ListFilter, pagination PaginationParams) (*ListResult, error) {
	result, err := s.packages.ListPackages(ctx, storage.PackageFilter{
		Query: filter.Query,
		Chain: filter.Chain,
		Sort:  filter.Sort,
		Order: filter.Order,
	}, storage.PaginationParams{
		Limit:  pagination.Limit,
		Cursor: pagination.Cursor,
	})
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}

	packages := make([]Package, len(result.Data))
	for i, p := range result.Data {
		packages[i] = *toPackage(&p)
	}

	return &ListResult{
		Packages:   packages,
		HasMore:    result.HasMore,
		NextCursor: result.NextCursor,
		PrevCursor: result.PrevCursor,
	}, nil
}

// Delete deletes a package version.
func (s *service) Delete(ctx context.Context, name, version string, ownerID string) error {
	// Check package ownership
	currentOwner, err := s.packages.GetPackageOwner(ctx, name)
	if err != nil {
		return fmt.Errorf("checking ownership: %w", err)
	}
	if currentOwner != "" && currentOwner != ownerID {
		return ErrForbidden
	}

	if err := s.packages.DeletePackage(ctx, name, version); err != nil {
		return fmt.Errorf("deleting package: %w", err)
	}

	return nil
}

// GetContracts lists contracts in a package version.
func (s *service) GetContracts(ctx context.Context, name, version string) ([]Contract, error) {
	pkg, err := s.packages.GetPackage(ctx, name, version)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting package: %w", err)
	}

	contracts, err := s.contracts.ListContracts(ctx, pkg.ID)
	if err != nil {
		return nil, fmt.Errorf("listing contracts: %w", err)
	}

	result := make([]Contract, len(contracts))
	for i, c := range contracts {
		result[i] = Contract{
			ID:          c.ID,
			PackageID:   c.PackageID,
			Name:        c.Name,
			Chain:       c.Chain,
			SourcePath:  c.SourcePath,
			License:     c.License,
			PrimaryHash: c.PrimaryHash,
		}
	}

	return result, nil
}

// GetContract retrieves a specific contract.
func (s *service) GetContract(ctx context.Context, name, version, contractName string) (*Contract, error) {
	pkg, err := s.packages.GetPackage(ctx, name, version)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting package: %w", err)
	}

	contract, err := s.contracts.GetContract(ctx, pkg.ID, contractName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting contract: %w", err)
	}

	return &Contract{
		ID:          contract.ID,
		PackageID:   contract.PackageID,
		Name:        contract.Name,
		Chain:       contract.Chain,
		SourcePath:  contract.SourcePath,
		License:     contract.License,
		PrimaryHash: contract.PrimaryHash,
	}, nil
}

// GetArtifact retrieves a specific artifact for a contract.
func (s *service) GetArtifact(ctx context.Context, name, version, contractName, artifactType string) ([]byte, error) {
	pkg, err := s.packages.GetPackage(ctx, name, version)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting package: %w", err)
	}

	contract, err := s.contracts.GetContract(ctx, pkg.ID, contractName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting contract: %w", err)
	}

	content, err := s.contracts.GetArtifact(ctx, contract.ID, artifactType)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting artifact: %w", err)
	}

	return content, nil
}

// GetArchive returns a gzipped tarball of all artifacts for a package version.
func (s *service) GetArchive(ctx context.Context, name, version string) ([]byte, error) {
	// Get package
	pkg, err := s.packages.GetPackage(ctx, name, version)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting package: %w", err)
	}

	// Get contracts
	contracts, err := s.contracts.ListContracts(ctx, pkg.ID)
	if err != nil {
		return nil, fmt.Errorf("listing contracts: %w", err)
	}

	// Create archive
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	basePath := fmt.Sprintf("%s-%s", name, version)

	// Add manifest
	manifest := map[string]any{
		"name":      name,
		"version":   version,
		"chain":     pkg.Chain,
		"builder":   pkg.Builder,
		"contracts": make([]map[string]string, 0, len(contracts)),
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	contractList := manifest["contracts"].([]map[string]string)
	for _, c := range contracts {
		contractList = append(contractList, map[string]string{
			"name":       c.Name,
			"sourcePath": c.SourcePath,
		})
	}
	manifest["contracts"] = contractList

	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	if err := addToTar(tw, basePath+"/manifest.json", manifestData); err != nil {
		return nil, fmt.Errorf("adding manifest: %w", err)
	}

	// Add each contract's artifacts
	for _, contract := range contracts {
		contractPath := fmt.Sprintf("%s/%s", basePath, contract.Name)

		// ABI
		if content, err := s.contracts.GetArtifact(ctx, contract.ID, "abi"); err == nil {
			if err := addToTar(tw, contractPath+"/abi.json", content); err != nil {
				return nil, fmt.Errorf("adding ABI: %w", err)
			}
		}

		// Bytecode
		if content, err := s.contracts.GetArtifact(ctx, contract.ID, "bytecode"); err == nil {
			if err := addToTar(tw, contractPath+"/bytecode.hex", content); err != nil {
				return nil, fmt.Errorf("adding bytecode: %w", err)
			}
		}

		// Deployed bytecode
		if content, err := s.contracts.GetArtifact(ctx, contract.ID, "deployed-bytecode"); err == nil {
			if err := addToTar(tw, contractPath+"/deployed-bytecode.hex", content); err != nil {
				return nil, fmt.Errorf("adding deployed bytecode: %w", err)
			}
		}

		// Standard JSON Input
		if content, err := s.contracts.GetArtifact(ctx, contract.ID, "standard-json-input"); err == nil {
			if err := addToTar(tw, contractPath+"/standard-json-input.json", content); err != nil {
				return nil, fmt.Errorf("adding standard JSON input: %w", err)
			}
		}

		// Storage Layout
		if content, err := s.contracts.GetArtifact(ctx, contract.ID, "storage-layout"); err == nil {
			if err := addToTar(tw, contractPath+"/storage-layout.json", content); err != nil {
				return nil, fmt.Errorf("adding storage layout: %w", err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("closing gzip: %w", err)
	}

	return buf.Bytes(), nil
}

func addToTar(tw *tar.Writer, path string, content []byte) error {
	header := &tar.Header{
		Name:    path,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(content)
	return err
}

// Helper functions

func toPackage(p *storage.Package) *Package {
	var createdAt time.Time
	if p.CreatedAt != "" {
		// Parse SQLite datetime format
		createdAt, _ = time.Parse("2006-01-02 15:04:05", p.CreatedAt)
	}
	return &Package{
		ID:               p.ID,
		Name:             p.Name,
		Version:          p.Version,
		Chain:            p.Chain,
		Builder:          p.Builder,
		CompilerVersion:  p.CompilerVersion,
		CompilerSettings: p.CompilerSettings,
		Metadata:         p.Metadata,
		OwnerID:          p.OwnerID,
		CreatedAt:        createdAt,
		Versions:         p.Versions,
	}
}
