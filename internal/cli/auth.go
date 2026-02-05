package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Credentials stores API keys per server
type Credentials struct {
	Servers map[string]ServerCredential `yaml:"servers"`
}

// ServerCredential stores credentials for a single server
type ServerCredential struct {
	APIKey string `yaml:"api_key"`
	Name   string `yaml:"name,omitempty"` // Optional name/description
}

func createAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
	}

	cmd.AddCommand(createAuthLoginCmd())
	cmd.AddCommand(createAuthLogoutCmd())
	cmd.AddCommand(createAuthStatusCmd())

	return cmd
}

func createAuthLoginCmd() *cobra.Command {
	var serverFlag string
	var apiKeyFlag string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with server",
		Long: `Save API key credentials for a Contrafactory server.

The API key is stored in ~/.contrafactory/credentials with secure file permissions.

EXAMPLES:
  # Interactive login (prompts for API key)
  contrafactory auth login

  # Login to a specific server
  contrafactory auth login --server https://contrafactory.example.com

  # Non-interactive login (for CI)
  contrafactory auth login --api-key $CONTRAFACTORY_API_KEY
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(serverFlag, apiKeyFlag)
		},
	}

	cmd.Flags().StringVar(&serverFlag, "server", "", "server URL (default from config)")
	cmd.Flags().StringVar(&apiKeyFlag, "api-key", "", "API key (prompts if not provided)")

	return cmd
}

func createAuthLogoutCmd() *cobra.Command {
	var serverFlag string
	var allFlag bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear credentials",
		Long: `Remove saved credentials for a server.

EXAMPLES:
  # Logout from default server
  contrafactory auth logout

  # Logout from a specific server
  contrafactory auth logout --server https://contrafactory.example.com

  # Clear all credentials
  contrafactory auth logout --all
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogout(serverFlag, allFlag)
		},
	}

	cmd.Flags().StringVar(&serverFlag, "server", "", "server URL (default from config)")
	cmd.Flags().BoolVar(&allFlag, "all", false, "clear all credentials")

	return cmd
}

func createAuthStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long: `Show current authentication status for all configured servers.

EXAMPLES:
  contrafactory auth status
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus()
		},
	}

	return cmd
}

func runAuthLogin(serverURL, apiKeyInput string) error {
	// Determine server
	if serverURL == "" {
		serverURL = getServer()
	}

	// Get API key
	apiKey := apiKeyInput
	if apiKey == "" {
		// Prompt for API key
		fmt.Printf("Enter API key for %s: ", serverURL)

		// Try to read password without echo
		stdinFd := int(os.Stdin.Fd())
		if term.IsTerminal(stdinFd) {
			byteKey, err := term.ReadPassword(stdinFd)
			fmt.Println() // New line after password input
			if err != nil {
				return fmt.Errorf("failed to read API key: %w", err)
			}
			apiKey = string(byteKey)
		} else {
			// Non-terminal, read from stdin
			reader := bufio.NewReader(os.Stdin)
			key, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed to read API key: %w", err)
			}
			apiKey = strings.TrimSpace(key)
		}
	}

	if apiKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	// Validate the API key by making a request
	fmt.Printf("Validating credentials with %s...\n", serverURL)
	valid, err := validateAPIKey(serverURL, apiKey)
	if err != nil {
		return fmt.Errorf("failed to validate credentials: %w", err)
	}
	if !valid {
		return fmt.Errorf("invalid API key")
	}

	// Save credentials
	if err := saveCredential(serverURL, apiKey); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	// Mask key for display
	masked := maskAPIKey(apiKey)
	fmt.Printf("✅ Authenticated to %s (key: %s)\n", serverURL, masked)
	fmt.Printf("   Credentials saved to %s\n", credentialsFilePath())

	return nil
}

func runAuthLogout(serverURL string, all bool) error {
	if all {
		// Remove all credentials
		path := credentialsFilePath()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove credentials: %w", err)
		}
		fmt.Println("✅ All credentials cleared")
		return nil
	}

	if serverURL == "" {
		serverURL = getServer()
	}

	creds, err := loadCredentials()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	if _, exists := creds.Servers[serverURL]; !exists {
		fmt.Printf("No credentials found for %s\n", serverURL)
		return nil
	}

	delete(creds.Servers, serverURL)

	if err := writeCredentials(creds); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	fmt.Printf("✅ Logged out from %s\n", serverURL)
	return nil
}

func runAuthStatus() error {
	creds, err := loadCredentials()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Not authenticated to any servers")
			fmt.Println("\nRun 'contrafactory auth login' to authenticate")
			return nil
		}
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	if len(creds.Servers) == 0 {
		fmt.Println("Not authenticated to any servers")
		fmt.Println("\nRun 'contrafactory auth login' to authenticate")
		return nil
	}

	fmt.Println("Authenticated servers:")
	for server, cred := range creds.Servers {
		masked := maskAPIKey(cred.APIKey)
		if cred.Name != "" {
			fmt.Printf("  • %s (%s, key: %s)\n", server, cred.Name, masked)
		} else {
			fmt.Printf("  • %s (key: %s)\n", server, masked)
		}
	}

	return nil
}

// Credential file helpers

func credentialsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".contrafactory"
	}
	return filepath.Join(home, ".contrafactory")
}

func credentialsFilePath() string {
	return filepath.Join(credentialsDir(), "credentials")
}

func loadCredentials() (*Credentials, error) {
	path := credentialsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var creds Credentials
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	if creds.Servers == nil {
		creds.Servers = make(map[string]ServerCredential)
	}

	return &creds, nil
}

func writeCredentials(creds *Credentials) error {
	dir := credentialsDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := yaml.Marshal(creds)
	if err != nil {
		return err
	}

	path := credentialsFilePath()
	return os.WriteFile(path, data, 0600) // Secure permissions
}

func saveCredential(serverURL, apiKey string) error {
	creds, err := loadCredentials()
	if err != nil {
		if os.IsNotExist(err) {
			creds = &Credentials{Servers: make(map[string]ServerCredential)}
		} else {
			return err
		}
	}

	creds.Servers[serverURL] = ServerCredential{APIKey: apiKey}
	return writeCredentials(creds)
}

func getCredential(serverURL string) string {
	creds, err := loadCredentials()
	if err != nil {
		return ""
	}
	if cred, ok := creds.Servers[serverURL]; ok {
		return cred.APIKey
	}
	return ""
}

func validateAPIKey(serverURL, apiKey string) (bool, error) {
	// Make a simple request to validate the key
	req, err := http.NewRequestWithContext(context.Background(), "GET", serverURL+"/api/v1/packages?limit=1", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-API-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// 401 means invalid key, anything else is ok
	if resp.StatusCode == http.StatusUnauthorized {
		// Check if it's actually an auth error
		body, _ := io.ReadAll(resp.Body)
		var errResp map[string]any
		if json.Unmarshal(body, &errResp) == nil {
			if errObj, ok := errResp["error"].(map[string]any); ok {
				if errObj["code"] == "UNAUTHORIZED" {
					return false, nil
				}
			}
		}
	}

	return true, nil
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:8] + "..." + key[len(key)-4:]
}
