package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

func createVerifyCmd() *cobra.Command {
	var pkg string
	var chainID int
	var address string
	var rpcURL string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify deployed contract matches stored artifact",
		Long: `Verify that a deployed contract's bytecode matches the stored artifact.

Compares the on-chain bytecode with the stored deployed bytecode,
stripping CBOR metadata for accurate comparison.

EXAMPLES:
  # Verify a deployed contract
  contrafactory verify \
    --package Token@1.0.0 \
    --chain-id 1 \
    --address 0x1234...

  # Specify custom RPC URL
  contrafactory verify \
    --package Token@1.0.0 \
    --chain-id 1 \
    --address 0x1234... \
    --rpc https://eth-mainnet.example.com
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(pkg, chainID, address, rpcURL)
		},
	}

	cmd.Flags().StringVar(&pkg, "package", "", "package/contract@version (required)")
	cmd.Flags().IntVar(&chainID, "chain-id", 0, "chain ID (required)")
	cmd.Flags().StringVar(&address, "address", "", "contract address (required)")
	cmd.Flags().StringVar(&rpcURL, "rpc", "", "RPC URL (optional, uses default for chain)")
	_ = cmd.MarkFlagRequired("package")
	_ = cmd.MarkFlagRequired("chain-id")
	_ = cmd.MarkFlagRequired("address")

	return cmd
}

// VerifyRequest matches the server's expected format
type VerifyRequest struct {
	Package  string `json:"package"`
	Version  string `json:"version"`
	Contract string `json:"contract"`
	ChainID  int    `json:"chainId"`
	Address  string `json:"address"`
	RPC      string `json:"rpc,omitempty"`
}

// VerifyResponse is the response from the verify endpoint
type VerifyResponse struct {
	Match   bool   `json:"match"`
	Type    string `json:"type"` // "full", "partial", "none"
	Message string `json:"message,omitempty"`
	Details struct {
		DeployedLength int `json:"deployedLength,omitempty"`
		ArtifactLength int `json:"artifactLength,omitempty"`
	} `json:"details,omitempty"`
}

func runVerify(pkgRef string, chainID int, address, rpcURL string) error {
	// Parse package reference
	name, version, contract, err := parsePackageRef(pkgRef)
	if err != nil {
		return fmt.Errorf("invalid package reference: %w", err)
	}

	if contract == "" {
		return fmt.Errorf("contract name required (use package/contract@version format)")
	}

	fmt.Printf("🔍 Verifying %s/%s@%s\n", name, contract, version)
	fmt.Printf("   Chain:   %d\n", chainID)
	fmt.Printf("   Address: %s\n", address)

	// Build request
	req := VerifyRequest{
		Package:  name,
		Version:  version,
		Contract: contract,
		ChainID:  chainID,
		Address:  address,
		RPC:      rpcURL,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make request
	serverURL := getServer()
	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", serverURL+"/api/v1/verify", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if key := getAPIKey(); key != "" {
		httpReq.Header.Set("X-API-Key", key)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("verification request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		if json.Unmarshal(body, &errResp) == nil {
			if errObj, ok := errResp["error"].(map[string]any); ok {
				return fmt.Errorf("verification failed: %s - %s", errObj["code"], errObj["message"])
			}
		}
		return fmt.Errorf("verification failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result VerifyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Println()

	switch result.Type {
	case "full":
		fmt.Println("✅ VERIFIED - Full match")
		fmt.Println("   Deployed bytecode exactly matches the artifact (including metadata)")
	case "partial":
		fmt.Println("✅ VERIFIED - Partial match")
		fmt.Println("   Executable code matches, but metadata differs")
		fmt.Println("   (This can happen with different source paths or comments)")
	case "none":
		fmt.Println("❌ NOT VERIFIED - No match")
		fmt.Println("   Deployed bytecode does not match the artifact")
		if result.Message != "" {
			fmt.Printf("   Reason: %s\n", result.Message)
		}
	default:
		if result.Match {
			fmt.Println("✅ VERIFIED")
		} else {
			fmt.Println("❌ NOT VERIFIED")
		}
	}

	return nil
}
