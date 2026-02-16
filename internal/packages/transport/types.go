// Package transport provides HTTP request/response types for the packages domain.
package transport

import (
	"encoding/json"

	"github.com/pendergraft/contrafactory/internal/packages/domain"
)

// PublishRequest is the HTTP request body for publishing a package.
type PublishRequest struct {
	Chain     string            `json:"chain"`
	Builder   string            `json:"builder,omitempty"`
	Project   string            `json:"project,omitempty"`
	Artifacts []ArtifactRequest `json:"artifacts"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ArtifactRequest is an artifact in a publish request.
type ArtifactRequest struct {
	Name              string               `json:"name"`
	SourcePath        string               `json:"sourcePath"`
	Chain             string               `json:"chain,omitempty"`
	ABI               json.RawMessage      `json:"abi,omitempty"`
	Bytecode          string               `json:"bytecode,omitempty"`
	DeployedBytecode  string               `json:"deployedBytecode,omitempty"`
	StandardJSONInput json.RawMessage      `json:"standardJsonInput,omitempty"`
	StorageLayout     json.RawMessage      `json:"storageLayout,omitempty"`
	Compiler          *CompilerInfoRequest `json:"compiler,omitempty"`
}

// CompilerInfoRequest is compiler info in a publish request.
type CompilerInfoRequest struct {
	Version    string                `json:"version"`
	Optimizer  *OptimizerInfoRequest `json:"optimizer,omitempty"`
	EVMVersion string                `json:"evmVersion,omitempty"`
	ViaIR      bool                  `json:"viaIR,omitempty"`
}

// OptimizerInfoRequest is optimizer settings in a publish request.
type OptimizerInfoRequest struct {
	Enabled bool `json:"enabled"`
	Runs    int  `json:"runs"`
}

// ToDomain converts PublishRequest to domain.PublishRequest.
func (r PublishRequest) ToDomain() domain.PublishRequest {
	artifacts := make([]domain.Artifact, len(r.Artifacts))
	for i, a := range r.Artifacts {
		artifacts[i] = a.ToDomain()
	}
	return domain.PublishRequest{
		Chain:     r.Chain,
		Builder:   r.Builder,
		Project:   r.Project,
		Artifacts: artifacts,
		Metadata:  r.Metadata,
	}
}

// ToDomain converts ArtifactRequest to domain.Artifact.
func (a ArtifactRequest) ToDomain() domain.Artifact {
	art := domain.Artifact{
		Name:              a.Name,
		SourcePath:        a.SourcePath,
		Chain:             a.Chain,
		ABI:               a.ABI,
		Bytecode:          a.Bytecode,
		DeployedBytecode:  a.DeployedBytecode,
		StandardJSONInput: a.StandardJSONInput,
		StorageLayout:     a.StorageLayout,
	}
	if a.Compiler != nil {
		info := a.Compiler.ToDomain()
		art.Compiler = &info
	}
	return art
}

// ToDomain converts CompilerInfoRequest to domain.CompilerInfo.
func (c CompilerInfoRequest) ToDomain() domain.CompilerInfo {
	info := domain.CompilerInfo{
		Version:    c.Version,
		EVMVersion: c.EVMVersion,
		ViaIR:      c.ViaIR,
	}
	if c.Optimizer != nil {
		info.Optimizer = &domain.OptimizerInfo{
			Enabled: c.Optimizer.Enabled,
			Runs:    c.Optimizer.Runs,
		}
	}
	return info
}

// ListResponse is the response for listing packages.
type ListResponse struct {
	Data       []PackageItem `json:"data"`
	Pagination Pagination    `json:"pagination"`
}

// PackageItem is a package summary in a list.
type PackageItem struct {
	Name      string   `json:"name"`
	Chain     string   `json:"chain"`
	Builder   string   `json:"builder"`
	Versions  []string `json:"versions"`
	Contracts []string `json:"contracts,omitempty"`
}

// Pagination provides pagination metadata.
type Pagination struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor"`
}

// VersionsResponse is the response for getting package versions.
type VersionsResponse struct {
	Name     string   `json:"name"`
	Chain    string   `json:"chain"`
	Builder  string   `json:"builder"`
	Versions []string `json:"versions"`
}

// PackageResponse is the response for getting a package version.
type PackageResponse struct {
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	Chain           string         `json:"chain"`
	Builder         string         `json:"builder"`
	CompilerVersion string         `json:"compilerVersion"`
	Contracts       []string       `json:"contracts"`
	CreatedAt       string         `json:"createdAt"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// PublishResponse is the response for publishing a package.
type PublishResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Message string `json:"message"`
}

// ContractsResponse is the response for listing contracts.
type ContractsResponse struct {
	Contracts []ContractItem `json:"contracts"`
}

// ContractItem is a contract summary.
type ContractItem struct {
	Name       string `json:"name"`
	SourcePath string `json:"sourcePath"`
	Chain      string `json:"chain"`
}

// ContractResponse is the response for getting a contract.
type ContractResponse struct {
	Name              string            `json:"name"`
	SourcePath        string            `json:"sourcePath"`
	Chain             string            `json:"chain"`
	License           string            `json:"license"`
	CompilationTarget map[string]string `json:"compilationTarget,omitempty"`
	Compiler          *CompilerInfoResp `json:"compiler,omitempty"`
}

// CompilerInfoResp is compiler info in a contract response.
type CompilerInfoResp struct {
	Version    string             `json:"version"`
	EVMVersion string             `json:"evmVersion,omitempty"`
	Optimizer  *OptimizerInfoResp `json:"optimizer,omitempty"`
	ViaIR      bool               `json:"viaIR,omitempty"`
}

// OptimizerInfoResp is optimizer settings in a contract response.
type OptimizerInfoResp struct {
	Enabled bool `json:"enabled"`
	Runs    int  `json:"runs"`
}

// DeploymentsResponse is the response for getting package deployments.
type DeploymentsResponse struct {
	Deployments []DeploymentSummary `json:"deployments"`
}

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
