package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pendergraft/contrafactory/pkg/client"
)

func createFetchCmd() *cobra.Command {
	var output string
	var only string
	var contract string

	cmd := &cobra.Command{
		Use:   "fetch <package>@<version>",
		Short: "Fetch package artifacts from the registry",
		Long: `Download package artifacts from the Contrafactory registry.

Each package contains one contract and its artifacts (ABI, bytecode, Standard JSON Input, etc.).

EXAMPLES:
  # Fetch a package's artifacts
  contrafactory fetch Token@1.0.0

  # Fetch to a specific directory
  contrafactory fetch Token@1.0.0 --output ./artifacts

  # Fetch only ABI
  contrafactory fetch Token@1.0.0 --only abi

  # Fetch only bytecode
  contrafactory fetch Token@1.0.0 --only bytecode

  # Fetch Standard JSON Input (for block explorer verification)
  contrafactory fetch Token@1.0.0 --only standard-json-input

  # Fetch storage layout (for upgradeable contract planning)
  contrafactory fetch Token@1.0.0 --only storage-layout
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFetch(args[0], output, only, contract)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", ".", "output directory")
	cmd.Flags().StringVar(&only, "only", "", "fetch only specific artifact type (abi, bytecode, deployed-bytecode, standard-json-input, storage-layout)")
	cmd.Flags().StringVar(&contract, "contract", "", "fetch only a specific contract")

	return cmd
}

func runFetch(ref, output, only, contractFilter string) error {
	name, version, refContract, err := parsePackageRef(ref)
	if err != nil {
		return err
	}

	// If contract was specified in the ref (package/contract@version), use that
	if refContract != "" {
		contractFilter = refContract
	}

	c := client.New(getServer(), getAPIKey())
	ctx := context.Background()

	// Get package info to list contracts
	pkg, err := c.GetPackageVersion(ctx, name, version)
	if err != nil {
		return fmt.Errorf("failed to get package: %w", err)
	}

	// Create output directory
	outDir := filepath.Join(output, fmt.Sprintf("%s@%s", name, version))
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fmt.Printf("📦 Fetching %s@%s\n", name, version)

	// Determine which contracts to fetch
	contracts := pkg.Contracts
	if contractFilter != "" {
		found := false
		for _, ct := range contracts {
			if ct == contractFilter {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("contract %q not found in package", contractFilter)
		}
		contracts = []string{contractFilter}
	}

	// Fetch each contract
	for _, contractName := range contracts {
		contractDir := filepath.Join(outDir, contractName)
		if err := os.MkdirAll(contractDir, 0755); err != nil {
			return fmt.Errorf("failed to create contract directory: %w", err)
		}

		fmt.Printf("  📄 %s\n", contractName)

		// Fetch requested artifacts
		if only == "" || only == "abi" {
			if err := fetchArtifact(c, ctx, name, version, contractName, "abi", filepath.Join(contractDir, "abi.json")); err != nil {
				fmt.Printf("    ⚠️  ABI: %v\n", err)
			} else {
				fmt.Println("    ✓ abi.json")
			}
		}

		if only == "" || only == "bytecode" {
			if err := fetchArtifact(c, ctx, name, version, contractName, "bytecode", filepath.Join(contractDir, "bytecode.hex")); err != nil {
				fmt.Printf("    ⚠️  bytecode: %v\n", err)
			} else {
				fmt.Println("    ✓ bytecode.hex")
			}
		}

		if only == "" || only == "deployed-bytecode" {
			if err := fetchArtifact(c, ctx, name, version, contractName, "deployed-bytecode", filepath.Join(contractDir, "deployed-bytecode.hex")); err != nil {
				fmt.Printf("    ⚠️  deployed-bytecode: %v\n", err)
			} else {
				fmt.Println("    ✓ deployed-bytecode.hex")
			}
		}

		if only == "" || only == "standard-json-input" {
			if err := fetchArtifact(c, ctx, name, version, contractName, "standard-json-input", filepath.Join(contractDir, "standard-json-input.json")); err != nil {
				fmt.Printf("    ⚠️  standard-json-input: %v\n", err)
			} else {
				fmt.Println("    ✓ standard-json-input.json")
			}
		}

		if only == "" || only == "storage-layout" {
			if err := fetchArtifact(c, ctx, name, version, contractName, "storage-layout", filepath.Join(contractDir, "storage-layout.json")); err != nil {
				fmt.Printf("    ⚠️  storage-layout: %v\n", err)
			} else {
				fmt.Println("    ✓ storage-layout.json")
			}
		}
	}

	// Write manifest
	manifest := map[string]any{
		"name":      name,
		"version":   version,
		"chain":     pkg.Chain,
		"contracts": contracts,
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		fmt.Printf("⚠️  Failed to write manifest: %v\n", err)
	}

	fmt.Printf("\n✅ Artifacts saved to %s\n", outDir)

	return nil
}

func fetchArtifact(c *client.Client, ctx context.Context, name, version, contract, artifactType, outPath string) error {
	var content []byte
	var err error

	switch artifactType {
	case "abi":
		content, err = c.GetABI(ctx, name, version, contract)
	case "bytecode":
		content, err = c.GetBytecode(ctx, name, version, contract)
	case "deployed-bytecode":
		content, err = c.GetDeployedBytecode(ctx, name, version, contract)
	case "standard-json-input":
		content, err = c.GetStandardJSONInput(ctx, name, version, contract)
	case "storage-layout":
		content, err = c.GetStorageLayout(ctx, name, version, contract)
	default:
		return fmt.Errorf("unknown artifact type: %s", artifactType)
	}

	if err != nil {
		return err
	}

	return os.WriteFile(outPath, content, 0644)
}
