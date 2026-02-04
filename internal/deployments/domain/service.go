// Package domain contains the business logic for deployment management.
package domain

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pendergraft/contrafactory/internal/storage"
	"github.com/pendergraft/contrafactory/internal/validation"
)

// Common errors returned by the deployment service.
var (
	ErrNotFound        = errors.New("deployment not found")
	ErrPackageNotFound = errors.New("package not found")
	ErrInvalidAddress  = errors.New("invalid address")
	ErrInvalidChainID  = errors.New("invalid chain ID")
)

// Service defines the deployment service interface.
type Service interface {
	// Record records a new deployment.
	Record(ctx context.Context, req RecordRequest) (*Deployment, error)

	// Get retrieves a deployment by chain and address.
	Get(ctx context.Context, chainID, address string) (*Deployment, error)

	// List lists deployments with filtering and pagination.
	List(ctx context.Context, filter ListFilter, pagination PaginationParams) (*ListResult, error)

	// ListByPackage lists deployments for a specific package version.
	ListByPackage(ctx context.Context, packageName, version string) ([]DeploymentSummary, error)

	// UpdateVerificationStatus updates the verification status of a deployment.
	UpdateVerificationStatus(ctx context.Context, chainID, address string, verified bool, verifiedOn []string) error
}

// DeploymentSummary is a lightweight deployment summary.
type DeploymentSummary struct {
	ChainID      string `json:"chainId"`
	Address      string `json:"address"`
	ContractName string `json:"contractName"`
	Verified     bool   `json:"verified"`
	TxHash       string `json:"txHash,omitempty"`
}

// service implements the Service interface.
type service struct {
	store storage.Store
}

// NewService creates a new deployment service.
func NewService(store storage.Store) Service {
	return &service{store: store}
}

// Record records a new deployment.
func (s *service) Record(ctx context.Context, req RecordRequest) (*Deployment, error) {
	// Validate address
	if err := validation.ValidateAddress(req.Address); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}

	// Validate chain ID
	if err := validation.ValidateChainID(req.ChainID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidChainID, err)
	}

	// Get package
	pkg, err := s.store.GetPackage(ctx, req.Package, req.Version)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrPackageNotFound
		}
		return nil, fmt.Errorf("getting package: %w", err)
	}

	// Build deployment data
	deploymentData := make(map[string]any)
	if req.ConstructorArgs != "" {
		deploymentData["constructorArgs"] = req.ConstructorArgs
	}
	if len(req.Libraries) > 0 {
		deploymentData["libraries"] = req.Libraries
	}

	deployment := &storage.Deployment{
		ID:              uuid.New().String(),
		PackageID:       pkg.ID,
		ContractName:    req.Contract,
		Chain:           pkg.Chain,
		ChainID:         strconv.Itoa(req.ChainID),
		Address:         req.Address,
		DeployerAddress: req.DeployerAddress,
		TxHash:          req.TxHash,
		BlockNumber:     req.BlockNumber,
		DeploymentData:  deploymentData,
		Verified:        false,
	}

	if err := s.store.RecordDeployment(ctx, deployment); err != nil {
		return nil, fmt.Errorf("recording deployment: %w", err)
	}

	return toDeployment(deployment), nil
}

// Get retrieves a deployment by chain and address.
func (s *service) Get(ctx context.Context, chainID, address string) (*Deployment, error) {
	deployment, err := s.store.GetDeployment(ctx, "evm", chainID, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting deployment: %w", err)
	}

	return toDeployment(deployment), nil
}

// List lists deployments with filtering and pagination.
func (s *service) List(ctx context.Context, filter ListFilter, pagination PaginationParams) (*ListResult, error) {
	result, err := s.store.ListDeployments(ctx, storage.DeploymentFilter{
		Chain:    filter.Chain,
		ChainID:  filter.ChainID,
		Package:  filter.Package,
		Verified: filter.Verified,
	}, storage.PaginationParams{
		Limit:  pagination.Limit,
		Cursor: pagination.Cursor,
	})
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	deployments := make([]Deployment, len(result.Data))
	for i, d := range result.Data {
		deployments[i] = *toDeployment(&d)
	}

	return &ListResult{
		Deployments: deployments,
		HasMore:     result.HasMore,
		NextCursor:  result.NextCursor,
		PrevCursor:  result.PrevCursor,
	}, nil
}

// UpdateVerificationStatus updates the verification status of a deployment.
func (s *service) UpdateVerificationStatus(ctx context.Context, chainID, address string, verified bool, verifiedOn []string) error {
	deployment, err := s.store.GetDeployment(ctx, "evm", chainID, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("getting deployment: %w", err)
	}

	if err := s.store.UpdateVerificationStatus(ctx, deployment.ID, verified, verifiedOn); err != nil {
		return fmt.Errorf("updating verification status: %w", err)
	}

	return nil
}

// ListByPackage lists deployments for a specific package version.
func (s *service) ListByPackage(ctx context.Context, packageName, version string) ([]DeploymentSummary, error) {
	// Get the package to get its ID
	pkg, err := s.store.GetPackage(ctx, packageName, version)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrPackageNotFound
		}
		return nil, fmt.Errorf("getting package: %w", err)
	}

	// List deployments filtered by package
	result, err := s.store.ListDeployments(ctx, storage.DeploymentFilter{
		Package: packageName,
	}, storage.PaginationParams{
		Limit: 100, // Reasonable limit
	})
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	// Filter to only deployments for this package version
	var summaries []DeploymentSummary
	for _, d := range result.Data {
		if d.PackageID == pkg.ID {
			summaries = append(summaries, DeploymentSummary{
				ChainID:      d.ChainID,
				Address:      d.Address,
				ContractName: d.ContractName,
				Verified:     d.Verified,
				TxHash:       d.TxHash,
			})
		}
	}

	return summaries, nil
}

func toDeployment(d *storage.Deployment) *Deployment {
	var createdAt time.Time
	if d.CreatedAt != "" {
		// Parse SQLite datetime format
		createdAt, _ = time.Parse("2006-01-02 15:04:05", d.CreatedAt)
	}
	return &Deployment{
		ID:              d.ID,
		PackageID:       d.PackageID,
		ContractName:    d.ContractName,
		Chain:           d.Chain,
		ChainID:         d.ChainID,
		Address:         d.Address,
		DeployerAddress: d.DeployerAddress,
		TxHash:          d.TxHash,
		BlockNumber:     d.BlockNumber,
		DeploymentData:  d.DeploymentData,
		Verified:        d.Verified,
		VerifiedOn:      d.VerifiedOn,
		CreatedAt:       createdAt,
	}
}
