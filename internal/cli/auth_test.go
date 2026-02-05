package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
)

// TestStdinFdCrossplatform verifies that os.Stdin.Fd() returns a value
// that can be safely cast to int for use with golang.org/x/term functions.
// This test ensures the cross-platform fix for Windows compatibility works.
func TestStdinFdCrossplatform(t *testing.T) {
	// Get the file descriptor - this is the cross-platform approach
	fd := os.Stdin.Fd()

	// Cast to int - this must work on all platforms (Linux, macOS, Windows)
	stdinFd := int(fd)

	// The file descriptor should be a valid non-negative integer
	// On Unix systems, stdin is typically 0
	// On Windows, it's a handle that when cast to int should still be valid
	assert.GreaterOrEqual(t, stdinFd, 0, "stdin file descriptor should be non-negative")

	// Verify term.IsTerminal accepts the int value without panic
	// This is the key test - it must compile and run on all platforms
	isTerminal := term.IsTerminal(stdinFd)

	// In test environment, stdin is typically not a terminal (piped)
	// We just verify the function can be called without error
	t.Logf("stdin fd=%d, isTerminal=%v", stdinFd, isTerminal)
}

// TestAuthLoginWithFlags tests the auth login command with flags (non-interactive)
func TestAuthLoginWithFlags(t *testing.T) {
	// Create a mock server that accepts any API key
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/packages" {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "valid-key" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"packages":[]}`))
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"code":"UNAUTHORIZED"}}`))
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create temp directory for credentials
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	t.Run("successful login with valid key", func(t *testing.T) {
		err := runAuthLogin(server.URL, "valid-key")
		require.NoError(t, err)

		// Verify credential was saved
		key := getCredential(server.URL)
		assert.Equal(t, "valid-key", key)
	})

	t.Run("failed login with invalid key", func(t *testing.T) {
		err := runAuthLogin(server.URL, "invalid-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid API key")
	})

	t.Run("empty API key rejected", func(t *testing.T) {
		// We need to simulate non-terminal stdin with empty input
		// Since runAuthLogin reads from stdin when apiKey is empty,
		// we'll test with an explicit empty string after prompting
		// This test verifies the validation path
		origStdin := os.Stdin
		defer func() { os.Stdin = origStdin }()

		// Create a pipe with empty input
		r, w, _ := os.Pipe()
		w.Close() // Close immediately to simulate empty input
		os.Stdin = r

		err := runAuthLogin(server.URL, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API key cannot be empty")
	})
}

// TestAuthLoginFromStdin tests reading API key from piped stdin (non-terminal)
func TestAuthLoginFromStdin(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/packages" {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "piped-key" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"packages":[]}`))
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"code":"UNAUTHORIZED"}}`))
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create temp directory for credentials
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	t.Run("read key from piped stdin", func(t *testing.T) {
		// Save original stdin
		origStdin := os.Stdin
		defer func() { os.Stdin = origStdin }()

		// Create a pipe with the API key
		r, w, err := os.Pipe()
		require.NoError(t, err)

		go func() {
			defer w.Close()
			io.WriteString(w, "piped-key\n")
		}()

		os.Stdin = r

		err = runAuthLogin(server.URL, "")
		require.NoError(t, err)

		// Verify credential was saved
		key := getCredential(server.URL)
		assert.Equal(t, "piped-key", key)
	})

	t.Run("read key with trailing whitespace", func(t *testing.T) {
		// Save original stdin
		origStdin := os.Stdin
		defer func() { os.Stdin = origStdin }()

		// Create a pipe with the API key with extra whitespace
		r, w, err := os.Pipe()
		require.NoError(t, err)

		go func() {
			defer w.Close()
			io.WriteString(w, "  piped-key  \n")
		}()

		os.Stdin = r

		// This should work because strings.TrimSpace is used
		// But wait - the current implementation only trims when reading from non-terminal
		// Let's verify the key gets trimmed properly
		err = runAuthLogin(server.URL, "")
		require.NoError(t, err)

		key := getCredential(server.URL)
		// The key should be trimmed
		assert.Equal(t, "piped-key", key)
	})
}

// TestAuthLogout tests the auth logout command
func TestAuthLogout(t *testing.T) {
	// Create temp directory for credentials
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	// First save some credentials
	err := saveCredential("http://server1:8080", "key1")
	require.NoError(t, err)
	err = saveCredential("http://server2:8080", "key2")
	require.NoError(t, err)

	t.Run("logout from specific server", func(t *testing.T) {
		err := runAuthLogout("http://server1:8080", false)
		require.NoError(t, err)

		// Verify server1 credential is gone
		key := getCredential("http://server1:8080")
		assert.Equal(t, "", key)

		// Verify server2 credential still exists
		key = getCredential("http://server2:8080")
		assert.Equal(t, "key2", key)
	})

	t.Run("logout from non-existent server", func(t *testing.T) {
		err := runAuthLogout("http://nonexistent:8080", false)
		require.NoError(t, err) // Should not error, just print message
	})

	t.Run("logout all", func(t *testing.T) {
		// Re-add credentials
		err := saveCredential("http://server1:8080", "key1")
		require.NoError(t, err)
		err = saveCredential("http://server2:8080", "key2")
		require.NoError(t, err)

		err = runAuthLogout("", true)
		require.NoError(t, err)

		// Verify all credentials are gone
		creds, err := loadCredentials()
		// File should be deleted, so we expect an error or empty
		if err == nil {
			assert.Empty(t, creds.Servers)
		}
	})
}

// TestAuthStatus tests the auth status command
func TestAuthStatus(t *testing.T) {
	// Create temp directory for credentials
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	t.Run("no credentials", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := runAuthStatus()
		require.NoError(t, err)

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		io.Copy(&buf, r)
		output := buf.String()

		assert.Contains(t, output, "Not authenticated")
	})

	t.Run("with credentials", func(t *testing.T) {
		// Save some credentials
		err := saveCredential("http://test-server:8080", "test-api-key-12345678901234")
		require.NoError(t, err)

		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = runAuthStatus()
		require.NoError(t, err)

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		io.Copy(&buf, r)
		output := buf.String()

		assert.Contains(t, output, "Authenticated servers")
		assert.Contains(t, output, "http://test-server:8080")
		// Verify key is masked
		assert.Contains(t, output, "test-api...")
	})
}

// TestValidateAPIKey tests the API key validation against a server
func TestValidateAPIKey(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-API-Key") == "valid-key" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"packages":[]}`))
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"code":"UNAUTHORIZED"}}`))
			}
		}))
		defer server.Close()

		valid, err := validateAPIKey(server.URL, "valid-key")
		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("invalid key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"code":"UNAUTHORIZED"}}`))
		}))
		defer server.Close()

		valid, err := validateAPIKey(server.URL, "invalid-key")
		require.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("server error treated as valid", func(t *testing.T) {
		// Server returning 500 should not be treated as invalid key
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		valid, err := validateAPIKey(server.URL, "any-key")
		require.NoError(t, err)
		assert.True(t, valid) // Non-401 treated as valid
	})

	t.Run("connection error", func(t *testing.T) {
		_, err := validateAPIKey("http://localhost:99999", "any-key")
		assert.Error(t, err)
	})
}

// TestCredentialFilePermissions verifies credentials are saved with secure permissions
func TestCredentialFilePermissions(t *testing.T) {
	// Create temp directory for credentials
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	err := saveCredential("http://test:8080", "test-key")
	require.NoError(t, err)

	credPath := filepath.Join(tmpDir, ".contrafactory", "credentials")
	info, err := os.Stat(credPath)
	require.NoError(t, err)

	// Verify file permissions are 0600 (owner read/write only)
	// Note: This test may behave differently on Windows
	if os.Getenv("GOOS") != "windows" {
		mode := info.Mode().Perm()
		assert.Equal(t, os.FileMode(0600), mode, "credentials file should have 0600 permissions")
	}
}

// TestCredentialDirPermissions verifies credential directory is created with secure permissions
func TestCredentialDirPermissions(t *testing.T) {
	// Create temp directory for credentials
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	err := saveCredential("http://test:8080", "test-key")
	require.NoError(t, err)

	credDir := filepath.Join(tmpDir, ".contrafactory")
	info, err := os.Stat(credDir)
	require.NoError(t, err)

	// Verify directory permissions are 0700 (owner only)
	// Note: This test may behave differently on Windows
	if os.Getenv("GOOS") != "windows" {
		mode := info.Mode().Perm()
		assert.Equal(t, os.FileMode(0700), mode, "credentials directory should have 0700 permissions")
	}
}

// TestAuthCommandStructure verifies the auth command and subcommands are properly structured
func TestAuthCommandStructure(t *testing.T) {
	cmd := createAuthCmd()

	assert.Equal(t, "auth", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	// Verify subcommands exist
	subCmds := cmd.Commands()
	subCmdNames := make([]string, len(subCmds))
	for i, c := range subCmds {
		subCmdNames[i] = c.Name()
	}

	assert.Contains(t, subCmdNames, "login")
	assert.Contains(t, subCmdNames, "logout")
	assert.Contains(t, subCmdNames, "status")
}

// TestAuthLoginCmdFlags verifies the login command has the expected flags
func TestAuthLoginCmdFlags(t *testing.T) {
	cmd := createAuthLoginCmd()

	// Verify flags exist
	serverFlag := cmd.Flags().Lookup("server")
	assert.NotNil(t, serverFlag)
	assert.Equal(t, "", serverFlag.DefValue)

	apiKeyFlag := cmd.Flags().Lookup("api-key")
	assert.NotNil(t, apiKeyFlag)
	assert.Equal(t, "", apiKeyFlag.DefValue)
}

// TestAuthLogoutCmdFlags verifies the logout command has the expected flags
func TestAuthLogoutCmdFlags(t *testing.T) {
	cmd := createAuthLogoutCmd()

	// Verify flags exist
	serverFlag := cmd.Flags().Lookup("server")
	assert.NotNil(t, serverFlag)

	allFlag := cmd.Flags().Lookup("all")
	assert.NotNil(t, allFlag)
	assert.Equal(t, "false", allFlag.DefValue)
}

// TestMultipleServersCredentials tests handling multiple server credentials
func TestMultipleServersCredentials(t *testing.T) {
	// Create temp directory for credentials
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	servers := map[string]string{
		"http://server1:8080":               "key1",
		"http://server2:8080":               "key2",
		"https://contrafactory.example.com": "prod-key",
		"http://localhost:8080":             "local-key",
	}

	// Save all credentials
	for server, key := range servers {
		err := saveCredential(server, key)
		require.NoError(t, err)
	}

	// Verify all can be retrieved
	for server, expectedKey := range servers {
		key := getCredential(server)
		assert.Equal(t, expectedKey, key, "credential for %s should match", server)
	}

	// Load and verify structure
	creds, err := loadCredentials()
	require.NoError(t, err)
	assert.Len(t, creds.Servers, len(servers))
}

// TestCredentialOverwrite tests that saving a new key overwrites the old one
func TestCredentialOverwrite(t *testing.T) {
	// Create temp directory for credentials
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	serverURL := "http://test:8080"

	// Save initial key
	err := saveCredential(serverURL, "old-key")
	require.NoError(t, err)
	assert.Equal(t, "old-key", getCredential(serverURL))

	// Save new key
	err = saveCredential(serverURL, "new-key")
	require.NoError(t, err)
	assert.Equal(t, "new-key", getCredential(serverURL))
}

// TestReadPasswordFromNonTerminal specifically tests the non-terminal branch
// This is the code path that works identically across all platforms
func TestReadPasswordFromNonTerminal(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"packages":[]}`))
	}))
	defer server.Close()

	// Create temp directory for credentials
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple key", "my-api-key\n", "my-api-key"},
		{"key with newline", "my-api-key\n\n", "my-api-key"},
		{"key with spaces", "  spaced-key  \n", "spaced-key"},
		{"key without newline", "no-newline-key", "no-newline-key"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save original stdin
			origStdin := os.Stdin
			defer func() { os.Stdin = origStdin }()

			// Create a pipe with the test input
			r, w, err := os.Pipe()
			require.NoError(t, err)

			go func() {
				defer w.Close()
				io.WriteString(w, tc.input)
			}()

			os.Stdin = r

			err = runAuthLogin(server.URL, "")
			require.NoError(t, err)

			key := getCredential(server.URL)
			assert.Equal(t, strings.TrimSpace(tc.expected), key)
		})
	}
}
