package evm

import (
	"github.com/pendergraft/contrafactory/internal/chains"
	"github.com/pendergraft/contrafactory/internal/chains/evm/foundry"
)

// NewFoundryBuilder creates a new Foundry builder
func NewFoundryBuilder() chains.Builder {
	return foundry.New()
}

// NewHardhatBuilder creates a new Hardhat builder (Phase 2)
// func NewHardhatBuilder() chains.Builder {
// 	return hardhat.New()
// }
