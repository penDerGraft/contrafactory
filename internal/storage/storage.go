package storage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pendergraft/contrafactory/internal/config"
)

// PackageStore handles package operations
type PackageStore interface {
	CreatePackage(ctx context.Context, pkg *Package) error
	GetPackage(ctx context.Context, name, version string) (*Package, error)
	GetPackageVersions(ctx context.Context, name string, includePrerelease bool) ([]string, error)
	ListPackages(ctx context.Context, filter PackageFilter, pagination PaginationParams) (*PaginatedResult[Package], error)
	DeletePackage(ctx context.Context, name, version string) error
	PackageExists(ctx context.Context, name, version string) (bool, error)
	GetPackageOwner(ctx context.Context, name string) (string, error)
	SetPackageOwner(ctx context.Context, name, ownerKeyID string) error
}

// ContractStore handles contract operations
type ContractStore interface {
	CreateContract(ctx context.Context, packageID string, contract *Contract) error
	GetContract(ctx context.Context, packageID, contractName string) (*Contract, error)
	ListContracts(ctx context.Context, packageID string) ([]Contract, error)
	StoreArtifact(ctx context.Context, contractID, artifactType string, content []byte) error
	GetArtifact(ctx context.Context, contractID, artifactType string) ([]byte, error)
	GetArtifactByHash(ctx context.Context, hash string) ([]byte, error)
}

// DeploymentStore handles deployment operations
type DeploymentStore interface {
	RecordDeployment(ctx context.Context, d *Deployment) error
	GetDeployment(ctx context.Context, chain, chainID, address string) (*Deployment, error)
	ListDeployments(ctx context.Context, filter DeploymentFilter, pagination PaginationParams) (*PaginatedResult[Deployment], error)
	UpdateVerificationStatus(ctx context.Context, id string, verified bool, verifiedOn []string) error
}

// APIKeyStore handles API key operations
type APIKeyStore interface {
	CreateAPIKey(ctx context.Context, name string) (key string, err error)
	ValidateAPIKey(ctx context.Context, key string) (*APIKey, error)
	ListAPIKeys(ctx context.Context) ([]APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
}

// Store combines all storage interfaces with lifecycle methods.
// Domain services define their own minimal interfaces based on their actual usage.
type Store interface {
	PackageStore
	ContractStore
	DeploymentStore
	APIKeyStore

	// Lifecycle
	Close() error
	Migrate(ctx context.Context) error
}

// Package represents a published package version
type Package struct {
	ID               string
	Name             string
	Version          string
	Chain            string
	Builder          string
	CompilerVersion  string
	CompilerSettings map[string]any
	Metadata         map[string]string
	OwnerID          string // API key ID that first published this package
	CreatedAt        string
	Versions         []string // Used for list aggregation (not stored directly)
}

// Contract represents a contract within a package
type Contract struct {
	ID           string
	PackageID    string
	Name         string
	Chain        string
	SourcePath   string
	License      string
	PrimaryHash  string
	MetadataHash string
	CreatedAt    string
}

// Artifact represents a stored artifact (ABI, bytecode, etc.)
type Artifact struct {
	ID           string
	ContractID   string
	ArtifactType string
	ContentHash  string
	SizeBytes    int
}

// Deployment represents a recorded deployment
type Deployment struct {
	ID              string
	PackageID       string
	ContractName    string
	Chain           string
	ChainID         string
	Address         string
	DeployerAddress string
	TxHash          string
	BlockNumber     int64
	DeploymentData  map[string]any
	Verified        bool
	VerifiedAt      string
	VerifiedOn      []string
	CreatedAt       string
}

// APIKey represents an API key
type APIKey struct {
	ID         string
	Name       string
	KeyHash    string
	Scopes     map[string]any
	CreatedAt  string
	LastUsedAt string
	RevokedAt  string
}

// PackageFilter contains filter options for listing packages
type PackageFilter struct {
	Query string
	Chain string
	Sort  string
	Order string
}

// DeploymentFilter contains filter options for listing deployments
type DeploymentFilter struct {
	Chain    string
	ChainID  string
	Package  string
	Verified *bool
}

// PaginationParams contains pagination options
type PaginationParams struct {
	Limit  int
	Cursor string
}

// PaginatedResult contains paginated results
type PaginatedResult[T any] struct {
	Data       []T
	HasMore    bool
	NextCursor string
	PrevCursor string
}

// New creates a new store based on configuration
func New(cfg config.StorageConfig, logger *slog.Logger) (Store, error) {
	switch cfg.Type {
	case "sqlite":
		return NewSQLiteStore(cfg.SQLite.Path, logger)
	case "postgres":
		return NewPostgresStore(cfg.Postgres.URL, logger)
	default:
		return nil, fmt.Errorf("unknown storage type: %s", cfg.Type)
	}
}
