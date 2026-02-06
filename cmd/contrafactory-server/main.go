package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/pendergraft/contrafactory/internal/config"
	"github.com/pendergraft/contrafactory/internal/server"
	"github.com/pendergraft/contrafactory/internal/storage"
)

var version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "contrafactory-server",
		Short:   "Contrafactory server - smart contract artifact registry",
		Version: version,
	}

	// Default behavior (no subcommand) is to serve
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runServe()
	}

	// Add subcommands
	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newKeysCmd())

	return rootCmd
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe()
		},
	}
}

func newKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage API keys",
	}

	cmd.AddCommand(newKeysCreateCmd())
	cmd.AddCommand(newKeysListCmd())
	cmd.AddCommand(newKeysRevokeCmd())

	return cmd
}

func newKeysCreateCmd() *cobra.Command {
	var name string
	var outputFile string
	var quiet bool
	var show bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new API key",
		Long: `Create a new API key for publishing packages.

By default, the key is written to a file in the current directory.
The key is only shown once - it cannot be retrieved later.

EXAMPLES:
  # Create key, write to file (default)
  contrafactory-server keys create --name "ci-release"

  # Create key, write to specific file
  contrafactory-server keys create --name "ci-release" --output /secure/path/key.txt

  # Create key, print only (for piping to secrets manager)
  contrafactory-server keys create --name "ci-release" --quiet | gh secret set CONTRAFACTORY_API_KEY

  # Create key, display on screen
  contrafactory-server keys create --name "ci-release" --show
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysCreate(name, outputFile, quiet, show)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "name/label for the key (required)")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "write key to file (default: ./contrafactory-key-{name}.txt)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "print only the key (for piping)")
	cmd.Flags().BoolVar(&show, "show", false, "display key on screen")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newKeysListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysList()
		},
	}
}

func newKeysRevokeCmd() *cobra.Command {
	var keyID string

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke an API key",
		Long: `Revoke an API key to prevent further use.

Use 'contrafactory-server keys list' to find the key ID.

EXAMPLES:
  contrafactory-server keys revoke --id abc123
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysRevoke(keyID)
		},
	}

	cmd.Flags().StringVar(&keyID, "id", "", "key ID to revoke (required)")
	_ = cmd.MarkFlagRequired("id")

	return cmd
}

// Key management commands

func runKeysCreate(name, outputFile string, quiet, show bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	store, err := storage.New(cfg.Storage, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}
	defer store.Close()

	// Ensure migrations are run
	if err := store.Migrate(context.Background()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Create the key
	key, err := store.CreateAPIKey(context.Background(), name)
	if err != nil {
		return fmt.Errorf("creating API key: %w", err)
	}

	// Handle output modes
	if quiet {
		// Just print the key for piping
		fmt.Println(key)
		return nil
	}

	if show {
		// Display on screen with warning
		fmt.Println("⚠️  API key (save this - it cannot be retrieved later):")
		fmt.Println()
		fmt.Println("   ", key)
		fmt.Println()
		return nil
	}

	// Default: write to file
	if outputFile == "" {
		outputFile = fmt.Sprintf("./contrafactory-key-%s.txt", name)
	}

	// Create directory if needed
	dir := filepath.Dir(outputFile)
	if dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}
	}

	// Write key to file with secure permissions
	if err := os.WriteFile(outputFile, []byte(key+"\n"), 0600); err != nil {
		return fmt.Errorf("writing key to file: %w", err)
	}

	fmt.Printf("✅ API key created: %s\n", name)
	fmt.Printf("   Written to: %s (mode 0600)\n", outputFile)
	fmt.Println()
	fmt.Println("   ⚠️  This key cannot be retrieved later. Keep it safe!")
	fmt.Println()
	fmt.Println("   Usage:")
	fmt.Println("     export CONTRAFACTORY_API_KEY=$(cat", outputFile+")")
	fmt.Println("     contrafactory publish --version 1.0.0")

	return nil
}

func runKeysList() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	store, err := storage.New(cfg.Storage, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}
	defer store.Close()

	keys, err := store.ListAPIKeys(context.Background())
	if err != nil {
		return fmt.Errorf("listing API keys: %w", err)
	}

	if len(keys) == 0 {
		fmt.Println("No API keys found")
		fmt.Println()
		fmt.Println("Create one with: contrafactory-server keys create --name \"my-key\"")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tCREATED\tLAST USED")
	for _, k := range keys {
		lastUsed := "never"
		if k.LastUsedAt != "" {
			lastUsed = k.LastUsedAt
		}
		created := k.CreatedAt
		// Truncate ID for display
		idDisplay := k.ID
		if len(k.ID) > 8 {
			idDisplay = k.ID[:8] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", idDisplay, k.Name, created, lastUsed)
	}
	w.Flush()

	return nil
}

func runKeysRevoke(keyID string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	store, err := storage.New(cfg.Storage, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}
	defer store.Close()

	// Find the full key ID if partial was provided
	keys, err := store.ListAPIKeys(context.Background())
	if err != nil {
		return fmt.Errorf("listing API keys: %w", err)
	}

	var fullKeyID string
	for _, k := range keys {
		if k.ID == keyID || (len(keyID) >= 8 && k.ID[:8] == keyID[:8]) {
			fullKeyID = k.ID
			break
		}
	}

	if fullKeyID == "" {
		return fmt.Errorf("key not found: %s", keyID)
	}

	if err := store.RevokeAPIKey(context.Background(), fullKeyID); err != nil {
		return fmt.Errorf("revoking API key: %w", err)
	}

	fmt.Printf("✅ API key revoked: %s\n", keyID)
	return nil
}

// Server command

func runServe() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Setup logger
	logger := setupLogger(cfg)
	logger.Info("starting contrafactory-server", "version", version)

	// Initialize storage
	store, err := storage.New(cfg.Storage, logger)
	if err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}
	defer store.Close()

	// Run migrations
	if err := store.Migrate(context.Background()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Create server
	srv := server.New(cfg, store, logger)

	// Create HTTP server with configurable timeouts
	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      srv.Handler(),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		logger.Info("shutting down", "signal", sig)
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

func setupLogger(cfg *config.Config) *slog.Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Logging.Level),
	}

	if cfg.Logging.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
