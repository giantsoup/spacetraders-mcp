package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"spacetraders-mcp/pkg/config"
)

// MCPResponse represents a JSON-RPC response from the MCP server
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC error
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ResourcesListResult represents the result of a resources/list call
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// Resource represents an MCP resource
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mimeType"`
}

// ResourceReadResult represents the result of a resources/read call
type ResourceReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourceContent represents the content of a resource
type ResourceContent struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType"`
	Text     string `json:"text"`
}

// Helper function to get the project root directory
func getProjectRoot(t *testing.T) string {
	// Get the current working directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// If we're in the test directory, go up one level
	if filepath.Base(wd) == "test" {
		return filepath.Dir(wd)
	}

	// Otherwise assume we're already in the project root
	return wd
}

// Helper function to build the test server binary
func buildTestServer(t *testing.T) string {
	projectRoot := getProjectRoot(t)
	binaryPath := filepath.Join(projectRoot, "spacetraders-mcp-test")

	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build server: %v\nOutput: %s", err, output)
	}

	return binaryPath
}

// Helper function to clean up the test server binary
func cleanupTestServer(t *testing.T, binaryPath string) {
	if err := os.Remove(binaryPath); err != nil {
		t.Logf("Failed to remove test binary: %v", err)
	}
}

// Helper function to call the MCP server with a request
func callMCPServer(t *testing.T, request string) []byte {
	// Build the server first
	binaryPath := buildTestServer(t)
	defer cleanupTestServer(t, binaryPath)

	// Start the server
	serverCmd := exec.Command(binaryPath)

	// Use the real API token if available, otherwise dummy token for basic tests
	// Try to load config from project root to check for .env file
	projectRoot := getProjectRoot(t)
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change to project root: %v", err)
	}

	// Check if we can load a valid config (which means token is available)
	if cfg, err := config.Load(); err == nil && cfg.SpaceTradersAPIToken != "" {
		// Token is available via config system, set it as environment variable for subprocess
		serverCmd.Env = append(os.Environ(), "SPACETRADERS_API_TOKEN="+cfg.SpaceTradersAPIToken)
	} else {
		// No valid token, use dummy token for basic tests
		serverCmd.Env = append(os.Environ(), "SPACETRADERS_API_TOKEN=dummy-token-for-basic-tests")
	}

	stdin, err := serverCmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	stdout, err := serverCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}

	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Send the request
	if _, err := stdin.Write([]byte(request + "\n")); err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	// Close stdin to signal end of input
	_ = stdin.Close()

	// Read the response with a buffer
	var response []byte
	buffer := make([]byte, 1024)
	for {
		n, err := stdout.Read(buffer)
		if n > 0 {
			response = append(response, buffer[:n]...)
		}
		if err != nil {
			break
		}
	}

	// Clean up
	_ = serverCmd.Process.Kill()
	_ = serverCmd.Wait()

	// Trim any trailing whitespace or null bytes
	return []byte(string(response))
}
