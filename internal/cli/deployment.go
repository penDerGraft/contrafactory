package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pendergraft/contrafactory/pkg/client"
)

func createDeploymentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "Deployment commands",
	}

	cmd.AddCommand(createDeploymentRecordCmd())
	cmd.AddCommand(createDeploymentListCmd())
	cmd.AddCommand(createDeploymentInfoCmd())

	return cmd
}

func createDeploymentRecordCmd() *cobra.Command {
	var pkg string
	var chainID int
	var address string
	var txHash string
	var deployerAddress string
	var fromBroadcast string

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a deployment",
		Long: `Record a contract deployment in the registry.

EXAMPLES:
  # Record a deployment
  contrafactory deployment record \
    --package my-contracts/Token@1.0.0 \
    --chain-id 1 \
    --address 0x1234... \
    --tx-hash 0xabcd...

  # Record from Foundry broadcast file
  contrafactory deployment record \
    --from-broadcast broadcast/Deploy.s.sol/1/run-latest.json \
    --package my-contracts@1.0.0
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromBroadcast != "" {
				return runDeploymentRecordFromBroadcast(fromBroadcast, pkg)
			}
			return runDeploymentRecord(pkg, chainID, address, txHash, deployerAddress)
		},
	}

	cmd.Flags().StringVar(&pkg, "package", "", "package/contract@version")
	cmd.Flags().IntVar(&chainID, "chain-id", 0, "chain ID")
	cmd.Flags().StringVar(&address, "address", "", "contract address")
	cmd.Flags().StringVar(&txHash, "tx-hash", "", "transaction hash")
	cmd.Flags().StringVar(&deployerAddress, "deployer", "", "deployer address")
	cmd.Flags().StringVar(&fromBroadcast, "from-broadcast", "", "parse from Foundry broadcast file")

	return cmd
}

func createDeploymentListCmd() *cobra.Command {
	var chainID string
	var packageFilter string
	var verified *bool
	var jsonOutput bool
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployments",
		Long: `List recorded deployments.

EXAMPLES:
  # List all deployments
  contrafactory deployment list

  # Filter by chain
  contrafactory deployment list --chain-id 1

  # Filter by package
  contrafactory deployment list --package my-contracts

  # Show only verified deployments
  contrafactory deployment list --verified
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploymentList(chainID, packageFilter, verified, limit, jsonOutput)
		},
	}

	cmd.Flags().StringVar(&chainID, "chain-id", "", "filter by chain ID")
	cmd.Flags().StringVar(&packageFilter, "package", "", "filter by package name")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	cmd.Flags().IntVar(&limit, "limit", 20, "number of items to show")

	// Handle --verified flag
	var verifiedFlag bool
	cmd.Flags().BoolVar(&verifiedFlag, "verified", false, "show only verified deployments")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("verified") {
			verified = &verifiedFlag
		}
		return nil
	}

	return cmd
}

func createDeploymentInfoCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "info <chain-id> <address>",
		Short: "Show deployment details",
		Long: `Display detailed information about a deployment.

EXAMPLES:
  contrafactory deployment info 1 0x1234...
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploymentInfo(args[0], args[1], jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")

	return cmd
}

func runDeploymentRecord(pkgRef string, chainID int, address, txHash, deployerAddress string) error {
	if pkgRef == "" {
		return fmt.Errorf("--package is required")
	}
	if chainID == 0 {
		return fmt.Errorf("--chain-id is required")
	}
	if address == "" {
		return fmt.Errorf("--address is required")
	}

	name, version, contract, err := parsePackageRef(pkgRef)
	if err != nil {
		return err
	}

	if contract == "" {
		return fmt.Errorf("contract name required (use package/contract@version format)")
	}

	c := client.New(getServer(), getAPIKey())

	req := client.DeploymentRequest{
		Package:         name,
		Version:         version,
		Contract:        contract,
		ChainID:         chainID,
		Address:         address,
		TxHash:          txHash,
		DeployerAddress: deployerAddress,
	}

	if err := c.RecordDeployment(context.Background(), req); err != nil {
		return fmt.Errorf("failed to record deployment: %w", err)
	}

	fmt.Printf("✅ Deployment recorded\n")
	fmt.Printf("   Contract: %s/%s@%s\n", name, contract, version)
	fmt.Printf("   Chain:    %d\n", chainID)
	fmt.Printf("   Address:  %s\n", address)

	return nil
}

func runDeploymentRecordFromBroadcast(broadcastPath, pkgRef string) error {
	// Read broadcast file
	data, err := os.ReadFile(broadcastPath)
	if err != nil {
		return fmt.Errorf("failed to read broadcast file: %w", err)
	}

	var broadcast struct {
		Transactions []struct {
			ContractName    string `json:"contractName"`
			ContractAddress string `json:"contractAddress"`
			Hash            string `json:"hash"`
		} `json:"transactions"`
		Chain int `json:"chain"`
	}

	if err := json.Unmarshal(data, &broadcast); err != nil {
		return fmt.Errorf("failed to parse broadcast file: %w", err)
	}

	if len(broadcast.Transactions) == 0 {
		return fmt.Errorf("no transactions found in broadcast file")
	}

	// Parse package ref
	name, version, _, err := parsePackageRef(pkgRef)
	if err != nil {
		return err
	}

	c := client.New(getServer(), getAPIKey())

	fmt.Printf("📝 Recording %d deployment(s) from broadcast...\n", len(broadcast.Transactions))

	for _, tx := range broadcast.Transactions {
		if tx.ContractAddress == "" {
			continue // Skip non-deployment transactions
		}

		req := client.DeploymentRequest{
			Package:  name,
			Version:  version,
			Contract: tx.ContractName,
			ChainID:  broadcast.Chain,
			Address:  tx.ContractAddress,
			TxHash:   tx.Hash,
		}

		if err := c.RecordDeployment(context.Background(), req); err != nil {
			fmt.Printf("  ⚠️  %s: %v\n", tx.ContractName, err)
			continue
		}

		fmt.Printf("  ✓ %s at %s\n", tx.ContractName, tx.ContractAddress)
	}

	return nil
}

func runDeploymentList(chainID, packageFilter string, verified *bool, limit int, jsonOutput bool) error {
	serverURL := getServer()
	apiKey := getAPIKey()

	// Build query string
	url := serverURL + "/api/v1/deployments?"
	if chainID != "" {
		url += "chain_id=" + chainID + "&"
	}
	if packageFilter != "" {
		url += "package=" + packageFilter + "&"
	}
	if verified != nil {
		if *verified {
			url += "verified=true&"
		} else {
			url += "verified=false&"
		}
	}
	url += fmt.Sprintf("limit=%d", limit)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return err
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to list deployments: %s", string(body))
	}

	var result struct {
		Data []struct {
			ChainID      string `json:"chainId"`
			Address      string `json:"address"`
			ContractName string `json:"contractName"`
			Verified     bool   `json:"verified"`
			TxHash       string `json:"txHash"`
		} `json:"data"`
		Pagination struct {
			HasMore bool `json:"hasMore"`
		} `json:"pagination"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if len(result.Data) == 0 {
		fmt.Println("No deployments found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CHAIN\tADDRESS\tCONTRACT\tVERIFIED")
	for _, d := range result.Data {
		verifiedStr := "no"
		if d.Verified {
			verifiedStr = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", d.ChainID, truncateAddress(d.Address), d.ContractName, verifiedStr)
	}
	w.Flush()

	if result.Pagination.HasMore {
		fmt.Printf("\n(showing %d deployments, more available)\n", len(result.Data))
	}

	return nil
}

func runDeploymentInfo(chainID, address string, jsonOutput bool) error {
	c := client.New(getServer(), getAPIKey())

	deployment, err := c.GetDeployment(context.Background(), chainID, address)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(deployment)
	}

	fmt.Printf("Deployment: %s\n", deployment.Address)
	fmt.Printf("Chain ID:   %s\n", deployment.ChainID)
	fmt.Printf("Contract:   %s\n", deployment.ContractName)
	if deployment.TxHash != "" {
		fmt.Printf("Tx Hash:    %s\n", deployment.TxHash)
	}
	if deployment.DeployerAddress != "" {
		fmt.Printf("Deployer:   %s\n", deployment.DeployerAddress)
	}
	if deployment.BlockNumber > 0 {
		fmt.Printf("Block:      %d\n", deployment.BlockNumber)
	}
	fmt.Printf("Verified:   %v\n", deployment.Verified)
	if deployment.CreatedAt != "" {
		fmt.Printf("Recorded:   %s\n", deployment.CreatedAt)
	}

	return nil
}

func truncateAddress(addr string) string {
	if len(addr) <= 14 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}
