// Package domain contains the business logic for package management.
package domain

import (
	"encoding/json"
	"time"
)

// Package represents a published package version.
type Package struct {
	ID               string
	Name             string
	Version          string
	Chain            string
	Builder          string
	CompilerVersion  string
	CompilerSettings map[string]any
	Metadata         map[string]string
	OwnerID          string
	CreatedAt        time.Time
	Versions         []string // Used for list aggregation
}

// Contract represents a contract within a package.
type Contract struct {
	ID           string
	PackageID    string
	Name         string
	Chain        string
	SourcePath   string
	License      string
	PrimaryHash  string
	MetadataHash string
	CreatedAt    time.Time
}

// Artifact wraps chain-specific artifact data for publishing.
type Artifact struct {
	Name       string `json:"name"`
	SourcePath string `json:"sourcePath"`
	Chain      string `json:"chain,omitempty"`

	// EVM-specific fields
	ABI               json.RawMessage `json:"abi,omitempty"`
	Bytecode          string          `json:"bytecode,omitempty"`
	DeployedBytecode  string          `json:"deployedBytecode,omitempty"`
	StandardJSONInput json.RawMessage `json:"standardJsonInput,omitempty"`
	StorageLayout     json.RawMessage `json:"storageLayout,omitempty"`
	Compiler          *CompilerInfo   `json:"compiler,omitempty"`
}

// CompilerInfo contains compiler settings.
type CompilerInfo struct {
	Version    string         `json:"version"`
	Optimizer  *OptimizerInfo `json:"optimizer,omitempty"`
	EVMVersion string         `json:"evmVersion,omitempty"`
	ViaIR      bool           `json:"viaIR,omitempty"`
}

// OptimizerInfo contains optimizer settings.
type OptimizerInfo struct {
	Enabled bool `json:"enabled"`
	Runs    int  `json:"runs"`
}

// PublishRequest is the request to publish a new package version.
type PublishRequest struct {
	Chain     string            `json:"chain"`
	Builder   string            `json:"builder,omitempty"`
	Artifacts []Artifact        `json:"artifacts"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ListFilter contains filter options for listing packages.
type ListFilter struct {
	Query string
	Chain string
	Sort  string
	Order string
}

// PaginationParams contains pagination options.
type PaginationParams struct {
	Limit  int
	Cursor string
}

// ListResult contains paginated list results.
type ListResult struct {
	Packages   []Package
	HasMore    bool
	NextCursor string
	PrevCursor string
}

// VersionsResult contains version list results.
type VersionsResult struct {
	Name     string
	Chain    string
	Builder  string
	Versions []string
}
