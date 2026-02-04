// Package evm provides the EVM chain module for Ethereum and compatible chains.
package evm

import (
	"context"
	"fmt"

	"github.com/pendergraft/contrafactory/internal/chains"
)

// Chain implements the chains.Chain interface for EVM-compatible blockchains
type Chain struct {
	builders []chains.Builder
}

// NewChain creates a new EVM chain module
func NewChain() *Chain {
	return &Chain{
		builders: []chains.Builder{
			NewFoundryBuilder(),
			// NewHardhatBuilder(), // Phase 2
		},
	}
}

// Name returns the chain identifier
func (c *Chain) Name() string {
	return "evm"
}

// DisplayName returns a human-readable name
func (c *Chain) DisplayName() string {
	return "Ethereum/EVM"
}

// Builders returns all available builders for this chain
func (c *Chain) Builders() []chains.Builder {
	return c.builders
}

// DetectBuilder detects which builder is used in the given directory
func (c *Chain) DetectBuilder(dir string) (chains.Builder, error) {
	for _, b := range c.builders {
		detected, err := b.Detect(dir)
		if err != nil {
			continue
		}
		if detected {
			return b, nil
		}
	}
	return nil, fmt.Errorf("no EVM builder detected in %s", dir)
}

// VerifyDeployment verifies that deployed bytecode matches expected bytecode
func (c *Chain) VerifyDeployment(ctx context.Context, opts chains.VerifyOptions) (*chains.VerifyResult, error) {
	// Get deployed bytecode
	deployed, err := c.GetDeployedBytecode(ctx, opts.RPC, opts.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployed bytecode: %w", err)
	}

	// Compare bytecode
	result := CompareBytecode(deployed, opts.ExpectedCode, opts.Libraries)
	return result, nil
}

// GetDeployedBytecode fetches the deployed bytecode from an RPC endpoint
func (c *Chain) GetDeployedBytecode(ctx context.Context, rpc string, address string) ([]byte, error) {
	// TODO: Implement eth_getCode RPC call
	return nil, fmt.Errorf("GetDeployedBytecode not yet implemented")
}
