// Package transport provides HTTP request/response types for the verification domain.
package transport

import "github.com/pendergraft/contrafactory/internal/verification/domain"

// VerifyRequest is the HTTP request body for verifying a contract.
type VerifyRequest struct {
	Package     string `json:"package"`
	Version     string `json:"version"`
	Contract    string `json:"contract"`
	ChainID     int    `json:"chainId"`
	Address     string `json:"address"`
	RPCEndpoint string `json:"rpcEndpoint,omitempty"`
}

// ToDomain converts VerifyRequest to domain.VerifyRequest.
func (r VerifyRequest) ToDomain() domain.VerifyRequest {
	return domain.VerifyRequest{
		Package:     r.Package,
		Version:     r.Version,
		Contract:    r.Contract,
		ChainID:     r.ChainID,
		Address:     r.Address,
		RPCEndpoint: r.RPCEndpoint,
	}
}

// VerifyResponse is the response for a verification request.
type VerifyResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	ChainID string `json:"chainId,omitempty"`
	Address string `json:"address,omitempty"`
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
