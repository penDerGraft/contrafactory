// Package domain contains the business logic for contract verification.
package domain

// VerifyRequest is the request to verify a deployed contract.
type VerifyRequest struct {
	Package     string `json:"package"`
	Version     string `json:"version"`
	Contract    string `json:"contract"`
	ChainID     int    `json:"chainId"`
	Address     string `json:"address"`
	RPCEndpoint string `json:"rpcEndpoint,omitempty"`
}

// VerifyResult is the result of a verification.
type VerifyResult struct {
	Verified  bool           `json:"verified"`
	MatchType string         `json:"matchType"` // "full", "partial", "none"
	Message   string         `json:"message"`
	Details   *VerifyDetails `json:"details,omitempty"`
}

// VerifyDetails contains detailed verification information.
type VerifyDetails struct {
	ExpectedBytecodeHash string `json:"expectedBytecodeHash,omitempty"`
	ActualBytecodeHash   string `json:"actualBytecodeHash,omitempty"`
	MetadataStripped     bool   `json:"metadataStripped,omitempty"`
	LibrariesLinked      bool   `json:"librariesLinked,omitempty"`
}
