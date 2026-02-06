// Package domain contains the business logic for contract verification.
package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/pendergraft/contrafactory/internal/chains"
	"github.com/pendergraft/contrafactory/internal/storage"
	"github.com/pendergraft/contrafactory/internal/validation"
)

// Common errors returned by the verification service.
var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidAddress = errors.New("invalid address")
	ErrInvalidChainID = errors.New("invalid chain ID")
	ErrChainNotFound  = errors.New("chain not supported")
)

// PackageStore defines the storage operations needed by the verification domain.
type PackageStore interface {
	GetPackage(ctx context.Context, name, version string) (*storage.Package, error)
}

// ContractStore defines the contract storage operations needed by the verification domain.
type ContractStore interface {
	GetContract(ctx context.Context, packageID, contractName string) (*storage.Contract, error)
	GetArtifact(ctx context.Context, contractID, artifactType string) ([]byte, error)
}

type service struct {
	packages PackageStore
	contracts ContractStore
	registry *chains.Registry
}

// NewService creates a new verification service.
func NewService(packages PackageStore, contracts ContractStore, registry *chains.Registry) *service {
	return &service{
		packages:  packages,
		contracts: contracts,
		registry:  registry,
	}
}

// Verify verifies a deployed contract matches the stored artifact.
func (s *service) Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error) {
	// Validate address
	if err := validation.ValidateAddress(req.Address); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}

	// Validate chain ID
	if err := validation.ValidateChainID(req.ChainID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidChainID, err)
	}

	// Get package
	pkg, err := s.packages.GetPackage(ctx, req.Package, req.Version)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting package: %w", err)
	}

	// Get contract
	contract, err := s.contracts.GetContract(ctx, pkg.ID, req.Contract)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting contract: %w", err)
	}

	// Get deployed bytecode from storage
	storedBytecode, err := s.contracts.GetArtifact(ctx, contract.ID, "deployed-bytecode")
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("deployed bytecode not found for contract %s", req.Contract)
		}
		return nil, fmt.Errorf("getting deployed bytecode: %w", err)
	}

	// Get chain module
	chain, ok := s.registry.Get(pkg.Chain)
	if !ok {
		return nil, ErrChainNotFound
	}

	// If RPC endpoint provided, fetch and verify on-chain bytecode
	if req.RPCEndpoint != "" {
		onChainBytecode, err := chain.GetDeployedBytecode(ctx, req.RPCEndpoint, req.Address)
		if err != nil {
			return &VerifyResult{
				Verified:  false,
				MatchType: "none",
				Message:   fmt.Sprintf("Failed to fetch on-chain bytecode: %v", err),
			}, nil
		}

		// Verify using chain module
		result, err := chain.VerifyDeployment(ctx, chains.VerifyOptions{
			RPC:          req.RPCEndpoint,
			Address:      req.Address,
			ExpectedCode: storedBytecode,
		})
		if err != nil {
			return nil, fmt.Errorf("verifying deployment: %w", err)
		}

		// Compare bytecodes
		verified := string(storedBytecode) == string(onChainBytecode)
		matchType := "none"
		if verified {
			matchType = "full"
		} else if result.Match {
			matchType = result.MatchType
			verified = true
		}

		return &VerifyResult{
			Verified:  verified,
			MatchType: matchType,
			Message:   result.Message,
			Details:   &VerifyDetails{},
		}, nil
	}

	// Without RPC, just return the stored bytecode hash for manual verification
	return &VerifyResult{
		Verified:  false,
		MatchType: "pending",
		Message:   "Provide RPC endpoint for on-chain verification, or compare bytecode manually",
		Details: &VerifyDetails{
			ExpectedBytecodeHash: contract.PrimaryHash,
		},
	}, nil
}
