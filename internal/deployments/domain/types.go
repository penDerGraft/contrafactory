// Package domain contains the business logic for deployment management.
package domain

import (
	"time"
)

// Deployment represents a recorded deployment.
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
	VerifiedAt      time.Time
	VerifiedOn      []string
	CreatedAt       time.Time
}

// RecordRequest is the request to record a new deployment.
type RecordRequest struct {
	Package         string            `json:"package"`
	Version         string            `json:"version"`
	Contract        string            `json:"contract"`
	ChainID         int               `json:"chainId"`
	Address         string            `json:"address"`
	TxHash          string            `json:"txHash,omitempty"`
	DeployerAddress string            `json:"deployerAddress,omitempty"`
	BlockNumber     int64             `json:"blockNumber,omitempty"`
	ConstructorArgs string            `json:"constructorArgs,omitempty"`
	Libraries       map[string]string `json:"libraries,omitempty"`
}

// ListFilter contains filter options for listing deployments.
type ListFilter struct {
	Chain    string
	ChainID  string
	Package  string
	Verified *bool
}

// PaginationParams contains pagination options.
type PaginationParams struct {
	Limit  int
	Cursor string
}

// ListResult contains paginated list results.
type ListResult struct {
	Deployments []Deployment
	HasMore     bool
	NextCursor  string
	PrevCursor  string
}
