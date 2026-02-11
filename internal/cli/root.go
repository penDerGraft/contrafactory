package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	server  string
	apiKey  string
)

// Execute runs the CLI
func Execute(version string) error {
	rootCmd := &cobra.Command{
		Use:     "contrafactory",
		Short:   "Smart contract artifact registry CLI",
		Long:    `Contrafactory is a CLI for publishing, fetching, and managing smart contract artifacts.`,
		Version: version,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: contrafactory.toml or cf.toml)")
	rootCmd.PersistentFlags().StringVar(&server, "server", "", "server URL (default from config)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key for authentication")

	// Add subcommands
	rootCmd.AddCommand(createPublishCmd())
	rootCmd.AddCommand(createFetchCmd())
	rootCmd.AddCommand(createListCmd())
	rootCmd.AddCommand(createInfoCmd())
	rootCmd.AddCommand(createVerifyCmd())
	rootCmd.AddCommand(createAuthCmd())
	rootCmd.AddCommand(createDeploymentCmd())
	rootCmd.AddCommand(createConfigCmd())
	rootCmd.AddCommand(createDiscoverCmd())

	return rootCmd.Execute()
}

// getServer returns the server URL from flag, env, config file, or credentials
func getServer() string {
	// 1. Command line flag
	if server != "" {
		return server
	}

	// 2. Environment variable
	if env := os.Getenv("CONTRAFACTORY_SERVER"); env != "" {
		return env
	}

	// 3. Project config file (TOML)
	if config := loadProjectConfigSilent(); config != nil && config.Server != "" {
		return config.Server
	}

	// 4. Default
	return "http://localhost:8080"
}

// getAPIKey returns the API key from flag, env, config, or credentials file
func getAPIKey() string {
	// 1. Command line flag
	if apiKey != "" {
		return apiKey
	}

	// 2. Environment variable
	if env := os.Getenv("CONTRAFACTORY_API_KEY"); env != "" {
		return env
	}

	// 3. Credentials file (keyed by server URL)
	serverURL := getServer()
	if cred := getCredential(serverURL); cred != "" {
		return cred
	}

	return ""
}
