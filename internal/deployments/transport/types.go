// Package transport provides HTTP request/response types for the deployments domain.
package transport

import "github.com/pendergraft/contrafactory/internal/deployments/domain"

// RecordRequest is the HTTP request body for recording a deployment.
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

// ToDomain converts RecordRequest to domain.RecordRequest.
func (r RecordRequest) ToDomain() domain.RecordRequest {
	return domain.RecordRequest{
		Package:         r.Package,
		Version:         r.Version,
		Contract:        r.Contract,
		ChainID:         r.ChainID,
		Address:         r.Address,
		TxHash:          r.TxHash,
		DeployerAddress: r.DeployerAddress,
		BlockNumber:     r.BlockNumber,
		ConstructorArgs: r.ConstructorArgs,
		Libraries:       r.Libraries,
	}
}

// DeploymentListResponse is the response for listing deployments.
type DeploymentListResponse struct {
	Data       []DeploymentItem `json:"data"`
	Pagination Pagination       `json:"pagination"`
}

// DeploymentItem is a deployment in a list.
type DeploymentItem struct {
	ChainID      string `json:"chainId"`
	Address      string `json:"address"`
	ContractName string `json:"contractName"`
	Verified     bool   `json:"verified"`
	TxHash       string `json:"txHash,omitempty"`
}

// Pagination provides pagination metadata.
type Pagination struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor"`
}

// DeploymentResponse is the response for getting a deployment.
type DeploymentResponse struct {
	ID              string   `json:"id"`
	PackageID       string   `json:"packageId"`
	ChainID         string   `json:"chainId"`
	Address         string   `json:"address"`
	ContractName    string   `json:"contractName"`
	DeployerAddress string   `json:"deployerAddress"`
	TxHash          string   `json:"txHash"`
	BlockNumber     int64    `json:"blockNumber"`
	Verified        bool     `json:"verified"`
	VerifiedOn      []string `json:"verifiedOn"`
	CreatedAt       string   `json:"createdAt"`
}

// RecordResponse is the response for recording a deployment.
type RecordResponse struct {
	ID       string `json:"id"`
	ChainID  string `json:"chainId"`
	Address  string `json:"address"`
	Verified bool   `json:"verified"`
	Message  string `json:"message"`
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
