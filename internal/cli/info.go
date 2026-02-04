package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pendergraft/contrafactory/pkg/client"
)

func createInfoCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "info <package>[@<version>]",
		Short: "Show package details",
		Long: `Display detailed information about a package or a specific version.

Each package contains one contract and its artifacts.

EXAMPLES:
  # Show package info (lists all versions)
  contrafactory info Token

  # Show specific version details
  contrafactory info Token@1.0.0

  # Output as JSON
  contrafactory info Token@1.0.0 --json
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInfo(args[0], jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")

	return cmd
}

func runInfo(ref string, jsonOutput bool) error {
	c := client.New(getServer(), getAPIKey())
	ctx := context.Background()

	// Check if version is specified
	var name, version string
	parsedName, parsedVersion, _, err := parsePackageRef(ref + "@dummy") // Trick to use parsePackageRef
	if err == nil && parsedVersion != "dummy" {
		// Has version
		name, version, _, _ = parsePackageRef(ref)
	} else {
		// No version - just package name
		name = parsedName
		version = ""
	}

	// Re-parse properly
	if idx := len(ref) - 1; idx > 0 {
		for i := len(ref) - 1; i >= 0; i-- {
			if ref[i] == '@' {
				name = ref[:i]
				version = ref[i+1:]
				break
			}
		}
		if version == "" {
			name = ref
		}
	}

	if version == "" {
		// Show package overview
		return showPackageInfo(c, ctx, name, jsonOutput)
	}

	// Show version details
	return showVersionInfo(c, ctx, name, version, jsonOutput)
}

func showPackageInfo(c *client.Client, ctx context.Context, name string, jsonOutput bool) error {
	pkg, err := c.GetPackage(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get package: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pkg)
	}

	fmt.Printf("Package: %s\n", pkg.Name)
	fmt.Printf("Chain:   %s\n", pkg.Chain)
	if pkg.Builder != "" {
		fmt.Printf("Builder: %s\n", pkg.Builder)
	}
	fmt.Println()

	if len(pkg.Versions) == 0 {
		fmt.Println("No versions published")
	} else {
		latestVersion := findLatestVersion(pkg.Versions)
		fmt.Printf("Versions (%d):\n", len(pkg.Versions))
		for _, v := range pkg.Versions {
			if v == latestVersion {
				fmt.Printf("  • %s (latest)\n", v)
			} else {
				fmt.Printf("  • %s\n", v)
			}
		}
	}

	return nil
}

func showVersionInfo(c *client.Client, ctx context.Context, name, version string, jsonOutput bool) error {
	pkg, err := c.GetPackageVersion(ctx, name, version)
	if err != nil {
		return fmt.Errorf("failed to get package version: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pkg)
	}

	fmt.Printf("Package:  %s\n", pkg.Name)
	fmt.Printf("Version:  %s\n", pkg.Version)
	fmt.Printf("Chain:    %s\n", pkg.Chain)
	if pkg.Builder != "" {
		fmt.Printf("Builder:  %s\n", pkg.Builder)
	}
	if pkg.CompilerVersion != "" {
		fmt.Printf("Compiler: %s\n", pkg.CompilerVersion)
	}
	if pkg.CreatedAt != "" {
		fmt.Printf("Created:  %s\n", pkg.CreatedAt)
	}
	fmt.Println()

	if len(pkg.Contracts) > 0 {
		fmt.Printf("Contracts (%d):\n", len(pkg.Contracts))
		for _, contract := range pkg.Contracts {
			fmt.Printf("  • %s\n", contract)
		}
	}

	fmt.Println()
	fmt.Printf("Fetch:  contrafactory fetch %s@%s\n", name, version)

	return nil
}
