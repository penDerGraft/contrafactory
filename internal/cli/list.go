package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"

	"github.com/pendergraft/contrafactory/pkg/client"
)

func createListCmd() *cobra.Command {
	var limit int
	var jsonOutput bool
	var chain string

	cmd := &cobra.Command{
		Use:   "list [package]",
		Short: "List packages or versions",
		Long: `List all packages in the registry, or list versions of a specific package.

Each package contains exactly one contract and its artifacts (ABI, bytecode, etc.).

EXAMPLES:
  # List all packages
  contrafactory list

  # List versions of a specific package
  contrafactory list Token

  # Filter by chain
  contrafactory list --chain evm

  # Output as JSON
  contrafactory list --json
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.New(getServer(), getAPIKey())

			if len(args) == 1 {
				// List versions of a specific package
				return listVersions(c, args[0], jsonOutput)
			}

			// List all packages
			return listPackages(c, chain, limit, jsonOutput)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "number of items to show")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	cmd.Flags().StringVar(&chain, "chain", "", "filter by chain (evm, solana)")

	return cmd
}

func listPackages(c *client.Client, chain string, limit int, jsonOutput bool) error {
	ctx := context.Background()

	resp, err := c.ListPackages(ctx)
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}

	// Filter by chain if specified
	var packages []client.Package
	for _, p := range resp.Data {
		if chain == "" || p.Chain == chain {
			packages = append(packages, p)
		}
	}

	// Apply limit
	if limit > 0 && len(packages) > limit {
		packages = packages[:limit]
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"packages":   packages,
			"count":      len(packages),
			"hasMore":    resp.Pagination.HasMore,
			"nextCursor": resp.Pagination.NextCursor,
		})
	}

	if len(packages) == 0 {
		fmt.Println("No packages found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCHAIN\tBUILDER\tLATEST")
	for _, p := range packages {
		latest := ""
		if len(p.Versions) > 0 {
			latest = findLatestVersion(p.Versions)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Chain, p.Builder, latest)
	}
	w.Flush()

	if resp.Pagination.HasMore {
		fmt.Printf("\n(showing %d packages, more available)\n", len(packages))
	}

	return nil
}

func listVersions(c *client.Client, name string, jsonOutput bool) error {
	ctx := context.Background()

	pkg, err := c.GetPackage(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get package: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"name":     pkg.Name,
			"versions": pkg.Versions,
		})
	}

	if len(pkg.Versions) == 0 {
		fmt.Printf("No versions found for %s\n", name)
		return nil
	}

	// Find the latest stable version using semver comparison
	latestVersion := findLatestVersion(pkg.Versions)

	fmt.Printf("Versions of %s:\n\n", name)
	for _, v := range pkg.Versions {
		if v == latestVersion {
			fmt.Printf("  %s (latest)\n", v)
		} else {
			fmt.Printf("  %s\n", v)
		}
	}
	fmt.Printf("\n%d version(s)\n", len(pkg.Versions))

	return nil
}

// parsePackageRef parses "package@version" or "package/contract@version"
func parsePackageRef(ref string) (name, version, contract string, err error) {
	// Check for @version
	parts := strings.SplitN(ref, "@", 2)
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid package reference: must be package@version or package/contract@version")
	}

	version = parts[1]
	namePart := parts[0]

	// Check for /contract
	if idx := strings.LastIndex(namePart, "/"); idx != -1 {
		name = namePart[:idx]
		contract = namePart[idx+1:]
	} else {
		name = namePart
	}

	return name, version, contract, nil
}

// findLatestVersion finds the highest stable version using semver comparison.
// Returns the first version if none are valid semver.
func findLatestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}

	latest := versions[0]
	for _, v := range versions[1:] {
		// Normalize versions for semver comparison (add 'v' prefix if missing)
		latestNorm := latest
		if !strings.HasPrefix(latestNorm, "v") {
			latestNorm = "v" + latestNorm
		}
		vNorm := v
		if !strings.HasPrefix(vNorm, "v") {
			vNorm = "v" + vNorm
		}

		// Compare using semver
		if semver.IsValid(vNorm) && semver.IsValid(latestNorm) {
			if semver.Compare(vNorm, latestNorm) > 0 {
				latest = v
			}
		}
	}

	return latest
}
