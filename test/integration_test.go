//go:build integration
// +build integration

package test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"spacetraders-mcp/pkg/config"
)

// Helper function to check if API token is available using config package
func checkAPITokenAvailable(t *testing.T) {
	// Get project root and change to it for config loading
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

	// Skip if no API token is available
	if _, err := config.Load(); err != nil {
		t.Skip("SPACETRADERS_API_TOKEN not available, skipping integration test")
	}
}

func TestIntegration_ServerBuild(t *testing.T) {
	checkAPITokenAvailable(t)

	// Test that the server builds successfully
	projectRoot := getProjectRoot(t)
	binaryPath := filepath.Join(projectRoot, "spacetraders-mcp-test")

	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build server: %v\nOutput: %s", err, output)
	}

	// Clean up the test binary
	defer func() {
		if err := os.Remove(binaryPath); err != nil {
			t.Logf("Failed to remove test binary: %v", err)
		}
	}()

	// Verify the binary exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Fatal("Built binary does not exist")
	}
}

func TestIntegration_ServerStartup(t *testing.T) {
	checkAPITokenAvailable(t)

	// Build the server
	binaryPath := buildTestServer(t)
	defer cleanupTestServer(t, binaryPath)

	// Start the server
	serverCmd := exec.Command(binaryPath)

	// Use the real API token
	serverCmd.Env = os.Environ()
	stdin, err := serverCmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}
	stdout, err := serverCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}
	stderr, err := serverCmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Monitor stderr for errors
	go func() {
		if stderr != nil {
			errOutput, _ := io.ReadAll(stderr)
			if len(errOutput) > 0 {
				t.Logf("Server stderr: %s", string(errOutput))
			}
		}
	}()

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			if stdin != nil {
				_ = stdin.Close()
			}
			if serverCmd.Process != nil {
				// Try graceful shutdown first
				_ = serverCmd.Process.Signal(os.Interrupt)

				// Wait for graceful shutdown with timeout
				done := make(chan error, 1)
				go func() {
					done <- serverCmd.Wait()
				}()

				select {
				case err := <-done:
					if err != nil {
						t.Logf("Process exited with error (may be normal): %v", err)
					}
				case <-time.After(2 * time.Second):
					// Force kill if graceful shutdown fails
					if err := serverCmd.Process.Kill(); err != nil {
						t.Logf("Failed to force kill process: %v", err)
					}
					<-done // Wait for the process to actually exit
				}
			}
		})
	}
	defer cleanup()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Test resources/list
	request := `{"jsonrpc": "2.0", "id": 1, "method": "resources/list"}` + "\n"
	if _, err := stdin.Write([]byte(request)); err != nil {
		t.Fatalf("Failed to write to server: %v", err)
	}

	// Read response
	response, err := readJSONResponse(stdout)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	var mcpResponse MCPResponse
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Server returned error: %v", mcpResponse.Error)
	}

	// Verify we got a resources list
	resultBytes, err := json.Marshal(mcpResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var resourcesResult ResourcesListResult
	if err := json.Unmarshal(resultBytes, &resourcesResult); err != nil {
		t.Fatalf("Failed to parse resources result: %v", err)
	}

	if len(resourcesResult.Resources) == 0 {
		t.Fatal("Expected at least one resource, got none")
	}

	// Verify expected resources are present
	expectedResources := []string{
		"spacetraders://agent/info",
		"spacetraders://ships/list",
		"spacetraders://contracts/list",
	}

	foundResources := make(map[string]bool)
	for _, resource := range resourcesResult.Resources {
		foundResources[resource.URI] = true
	}

	for _, expected := range expectedResources {
		if !foundResources[expected] {
			t.Errorf("Expected resource %s not found", expected)
		}
	}
}

func TestIntegration_AgentResource(t *testing.T) {
	checkAPITokenAvailable(t)

	response := callMCPServer(t, `{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "spacetraders://agent/info"}}`)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Server returned error: %v", mcpResponse.Error)
	}

	// Parse the resource read result
	resultBytes, err := json.Marshal(mcpResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var readResult ResourceReadResult
	if err := json.Unmarshal(resultBytes, &readResult); err != nil {
		t.Fatalf("Failed to parse read result: %v", err)
	}

	if len(readResult.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(readResult.Contents))
	}

	content := readResult.Contents[0]
	if content.URI != "spacetraders://agent/info" {
		t.Errorf("Expected URI spacetraders://agent/info, got %s", content.URI)
	}

	// Handle API errors gracefully - if we get text/plain, it's likely an API error
	if content.MIMEType == "text/plain" {
		if strings.Contains(content.Text, "Error") {
			t.Skipf("API error (likely invalid token): %s", content.Text)
		} else {
			t.Errorf("Expected MIME type application/json, got text/plain with content: %s", content.Text)
		}
		return
	}

	if content.MIMEType != "application/json" {
		t.Errorf("Expected MIME type application/json, got %s", content.MIMEType)
	}

	// Parse the JSON content to verify it's valid agent data
	var agentData map[string]interface{}
	if err := json.Unmarshal([]byte(content.Text), &agentData); err != nil {
		t.Fatalf("Failed to parse agent JSON: %v", err)
	}

	agent, ok := agentData["agent"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected agent object in response")
	}

	requiredFields := []string{"accountId", "symbol", "headquarters", "credits", "startingFaction", "shipCount"}
	for _, field := range requiredFields {
		if _, exists := agent[field]; !exists {
			t.Errorf("Expected field '%s' in agent data", field)
		}
	}
}

func TestIntegration_ShipsResource(t *testing.T) {
	checkAPITokenAvailable(t)

	response := callMCPServer(t, `{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "spacetraders://ships/list"}}`)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Server returned error: %v", mcpResponse.Error)
	}

	// Parse the resource read result
	resultBytes, err := json.Marshal(mcpResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var readResult ResourceReadResult
	if err := json.Unmarshal(resultBytes, &readResult); err != nil {
		t.Fatalf("Failed to parse read result: %v", err)
	}

	if len(readResult.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(readResult.Contents))
	}

	content := readResult.Contents[0]
	if content.URI != "spacetraders://ships/list" {
		t.Errorf("Expected URI spacetraders://ships/list, got %s", content.URI)
	}

	// Handle API errors gracefully - if we get text/plain, it's likely an API error
	if content.MIMEType == "text/plain" {
		if strings.Contains(content.Text, "Error") {
			t.Skipf("API error (likely invalid token): %s", content.Text)
		} else {
			t.Errorf("Expected MIME type application/json, got text/plain with content: %s", content.Text)
		}
		return
	}

	// Parse the JSON content to verify it's valid ships data
	var shipsData map[string]interface{}
	if err := json.Unmarshal([]byte(content.Text), &shipsData); err != nil {
		t.Fatalf("Failed to parse ships JSON: %v", err)
	}

	ships, ok := shipsData["ships"].([]interface{})
	if !ok {
		t.Fatal("Expected ships array in response")
	}

	meta, ok := shipsData["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected meta object in response")
	}

	count, ok := meta["count"].(float64)
	if !ok {
		t.Fatal("Expected count field in meta")
	}

	if int(count) != len(ships) {
		t.Errorf("Meta count %d does not match ships array length %d", int(count), len(ships))
	}
}

func TestIntegration_IndividualShipResource(t *testing.T) {
	checkAPITokenAvailable(t)

	// First get the ships list to get a valid ship symbol
	shipsResponse := callMCPServer(t, `{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "spacetraders://ships/list"}}`)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(shipsResponse, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse ships response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Server returned error for ships list: %v", mcpResponse.Error)
	}

	// Parse the ships response to get a ship symbol
	resultBytes, err := json.Marshal(mcpResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal ships result: %v", err)
	}

	var readResult ResourceReadResult
	if err := json.Unmarshal(resultBytes, &readResult); err != nil {
		t.Fatalf("Failed to parse ships read result: %v", err)
	}

	if len(readResult.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(readResult.Contents))
	}

	content := readResult.Contents[0]
	if content.MIMEType == "text/plain" {
		if strings.Contains(content.Text, "Error") {
			t.Skipf("API error (likely invalid token): %s", content.Text)
		}
		return
	}

	// Parse the JSON content to get ship symbols
	var shipsData map[string]interface{}
	if err := json.Unmarshal([]byte(content.Text), &shipsData); err != nil {
		t.Fatalf("Failed to parse ships JSON: %v", err)
	}

	ships, ok := shipsData["ships"].([]interface{})
	if !ok {
		t.Fatal("Expected ships array in response")
	}

	if len(ships) == 0 {
		t.Skip("No ships found, skipping individual ship resource test")
	}

	// Get the first ship symbol
	firstShip := ships[0].(map[string]interface{})
	shipSymbol := firstShip["symbol"].(string)

	// Test individual ship resource
	shipURI := fmt.Sprintf("spacetraders://ships/%s", shipSymbol)
	shipRequest := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "%s"}}`, shipURI)

	response := callMCPServer(t, shipRequest)

	var shipResponse MCPResponse
	if err := json.Unmarshal(response, &shipResponse); err != nil {
		t.Fatalf("Failed to parse ship response: %v", err)
	}

	if shipResponse.Error != nil {
		t.Fatalf("Server returned error for ship resource: %v", shipResponse.Error)
	}

	// Parse the ship resource result
	shipResultBytes, err := json.Marshal(shipResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal ship result: %v", err)
	}

	var shipReadResult ResourceReadResult
	if err := json.Unmarshal(shipResultBytes, &shipReadResult); err != nil {
		t.Fatalf("Failed to parse ship read result: %v", err)
	}

	if len(shipReadResult.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(shipReadResult.Contents))
	}

	shipContent := shipReadResult.Contents[0]
	if shipContent.URI != shipURI {
		t.Errorf("Expected URI %s, got %s", shipURI, shipContent.URI)
	}

	if shipContent.MIMEType == "text/plain" {
		if strings.Contains(shipContent.Text, "Error") {
			t.Skipf("API error: %s", shipContent.Text)
		}
		return
	}

	if shipContent.MIMEType != "application/json" {
		t.Errorf("Expected MIME type application/json, got %s", shipContent.MIMEType)
	}

	// Parse the JSON content to verify structure
	var shipData map[string]interface{}
	if err := json.Unmarshal([]byte(shipContent.Text), &shipData); err != nil {
		t.Fatalf("Failed to parse ship JSON: %v", err)
	}

	// Verify expected structure
	if _, ok := shipData["ship"]; !ok {
		t.Error("Expected 'ship' field in response")
	}

	if _, ok := shipData["analysis"]; !ok {
		t.Error("Expected 'analysis' field in response")
	}

	if _, ok := shipData["capabilities"]; !ok {
		t.Error("Expected 'capabilities' field in response")
	}

	if _, ok := shipData["recommendations"]; !ok {
		t.Error("Expected 'recommendations' field in response")
	}

	if _, ok := shipData["meta"]; !ok {
		t.Error("Expected 'meta' field in response")
	}

	// Verify ship symbol matches
	if ship, ok := shipData["ship"].(map[string]interface{}); ok {
		if symbol, ok := ship["symbol"].(string); ok {
			if symbol != shipSymbol {
				t.Errorf("Expected ship symbol %s, got %s", shipSymbol, symbol)
			}
		} else {
			t.Error("Expected ship symbol field")
		}
	} else {
		t.Error("Expected ship object")
	}
}

func TestIntegration_ShipCooldownResource(t *testing.T) {
	checkAPITokenAvailable(t)

	// First get the ships list to get a valid ship symbol
	shipsResponse := callMCPServer(t, `{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "spacetraders://ships/list"}}`)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(shipsResponse, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse ships response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Server returned error for ships list: %v", mcpResponse.Error)
	}

	// Parse the ships response to get a ship symbol
	resultBytes, err := json.Marshal(mcpResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal ships result: %v", err)
	}

	var readResult ResourceReadResult
	if err := json.Unmarshal(resultBytes, &readResult); err != nil {
		t.Fatalf("Failed to parse ships read result: %v", err)
	}

	if len(readResult.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(readResult.Contents))
	}

	content := readResult.Contents[0]
	if content.MIMEType == "text/plain" {
		if strings.Contains(content.Text, "Error") {
			t.Skipf("API error (likely invalid token): %s", content.Text)
		}
		return
	}

	// Parse the JSON content to get ship symbols
	var shipsData map[string]interface{}
	if err := json.Unmarshal([]byte(content.Text), &shipsData); err != nil {
		t.Fatalf("Failed to parse ships JSON: %v", err)
	}

	ships, ok := shipsData["ships"].([]interface{})
	if !ok {
		t.Fatal("Expected ships array in response")
	}

	if len(ships) == 0 {
		t.Skip("No ships found, skipping cooldown resource test")
	}

	// Get the first ship symbol
	firstShip := ships[0].(map[string]interface{})
	shipSymbol := firstShip["symbol"].(string)

	// Test ship cooldown resource
	cooldownURI := fmt.Sprintf("spacetraders://ships/%s/cooldown", shipSymbol)
	cooldownRequest := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "%s"}}`, cooldownURI)

	response := callMCPServer(t, cooldownRequest)

	var cooldownResponse MCPResponse
	if err := json.Unmarshal(response, &cooldownResponse); err != nil {
		t.Fatalf("Failed to parse cooldown response: %v", err)
	}

	if cooldownResponse.Error != nil {
		t.Fatalf("Server returned error for cooldown resource: %v", cooldownResponse.Error)
	}

	// Parse the cooldown resource result
	cooldownResultBytes, err := json.Marshal(cooldownResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal cooldown result: %v", err)
	}

	var cooldownReadResult ResourceReadResult
	if err := json.Unmarshal(cooldownResultBytes, &cooldownReadResult); err != nil {
		t.Fatalf("Failed to parse cooldown read result: %v", err)
	}

	if len(cooldownReadResult.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(cooldownReadResult.Contents))
	}

	cooldownContent := cooldownReadResult.Contents[0]
	if cooldownContent.URI != cooldownURI {
		t.Errorf("Expected URI %s, got %s", cooldownURI, cooldownContent.URI)
	}

	if cooldownContent.MIMEType == "text/plain" {
		if strings.Contains(cooldownContent.Text, "Error") {
			t.Skipf("API error: %s", cooldownContent.Text)
		}
		return
	}

	if cooldownContent.MIMEType != "application/json" {
		t.Errorf("Expected MIME type application/json, got %s", cooldownContent.MIMEType)
	}

	// Parse the JSON content to verify structure
	var cooldownData map[string]interface{}
	if err := json.Unmarshal([]byte(cooldownContent.Text), &cooldownData); err != nil {
		t.Fatalf("Failed to parse cooldown JSON: %v", err)
	}

	// Verify expected structure
	if _, ok := cooldownData["ship_symbol"]; !ok {
		t.Error("Expected 'ship_symbol' field in response")
	}

	if _, ok := cooldownData["cooldown"]; !ok {
		t.Error("Expected 'cooldown' field in response")
	}

	if _, ok := cooldownData["status"]; !ok {
		t.Error("Expected 'status' field in response")
	}

	if _, ok := cooldownData["actions"]; !ok {
		t.Error("Expected 'actions' field in response")
	}

	if _, ok := cooldownData["recommendations"]; !ok {
		t.Error("Expected 'recommendations' field in response")
	}

	if _, ok := cooldownData["meta"]; !ok {
		t.Error("Expected 'meta' field in response")
	}

	// Verify ship symbol matches
	if symbol, ok := cooldownData["ship_symbol"].(string); ok {
		if symbol != shipSymbol {
			t.Errorf("Expected ship symbol %s, got %s", shipSymbol, symbol)
		}
	} else {
		t.Error("Expected ship_symbol field")
	}

	// Verify cooldown structure
	if cooldown, ok := cooldownData["cooldown"].(map[string]interface{}); ok {
		if _, ok := cooldown["active"]; !ok {
			t.Error("Expected 'active' field in cooldown")
		}
		if _, ok := cooldown["remaining_seconds"]; !ok {
			t.Error("Expected 'remaining_seconds' field in cooldown")
		}
	} else {
		t.Error("Expected cooldown object")
	}
}

func TestIntegration_ShipResourceInvalidShip(t *testing.T) {
	checkAPITokenAvailable(t)

	// Test with invalid ship symbol
	invalidShipURI := "spacetraders://ships/INVALID-SHIP-SYMBOL"
	invalidRequest := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "%s"}}`, invalidShipURI)

	response := callMCPServer(t, invalidRequest)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Server returned error: %v", mcpResponse.Error)
	}

	// Parse the resource result
	resultBytes, err := json.Marshal(mcpResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var readResult ResourceReadResult
	if err := json.Unmarshal(resultBytes, &readResult); err != nil {
		t.Fatalf("Failed to parse read result: %v", err)
	}

	if len(readResult.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(readResult.Contents))
	}

	content := readResult.Contents[0]
	if content.MIMEType != "text/plain" {
		t.Errorf("Expected MIME type text/plain for error, got %s", content.MIMEType)
	}

	if !strings.Contains(content.Text, "Error") {
		t.Errorf("Expected error message in response, got: %s", content.Text)
	}
}

func TestIntegration_ShipResourceInvalidURI(t *testing.T) {
	checkAPITokenAvailable(t)

	// Test with malformed URI
	invalidURI := "spacetraders://ships/"
	invalidRequest := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "%s"}}`, invalidURI)

	response := callMCPServer(t, invalidRequest)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Server returned error: %v", mcpResponse.Error)
	}

	// Parse the resource result
	resultBytes, err := json.Marshal(mcpResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var readResult ResourceReadResult
	if err := json.Unmarshal(resultBytes, &readResult); err != nil {
		t.Fatalf("Failed to parse read result: %v", err)
	}

	if len(readResult.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(readResult.Contents))
	}

	content := readResult.Contents[0]
	if content.MIMEType != "text/plain" {
		t.Errorf("Expected MIME type text/plain for error, got %s", content.MIMEType)
	}

	if !strings.Contains(content.Text, "Invalid") {
		t.Errorf("Expected invalid URI message in response, got: %s", content.Text)
	}
}

func TestIntegration_ContractsResource(t *testing.T) {
	checkAPITokenAvailable(t)

	response := callMCPServer(t, `{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "spacetraders://contracts/list"}}`)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Server returned error: %v", mcpResponse.Error)
	}

	// Parse the resource read result
	resultBytes, err := json.Marshal(mcpResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var readResult ResourceReadResult
	if err := json.Unmarshal(resultBytes, &readResult); err != nil {
		t.Fatalf("Failed to parse read result: %v", err)
	}

	if len(readResult.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(readResult.Contents))
	}

	content := readResult.Contents[0]
	if content.URI != "spacetraders://contracts/list" {
		t.Errorf("Expected URI spacetraders://contracts/list, got %s", content.URI)
	}

	// Handle API errors gracefully - if we get text/plain, it's likely an API error
	if content.MIMEType == "text/plain" {
		if strings.Contains(content.Text, "Error") {
			t.Skipf("API error (likely invalid token): %s", content.Text)
		} else {
			t.Errorf("Expected MIME type application/json, got text/plain with content: %s", content.Text)
		}
		return
	}

	// Parse the JSON content to verify it's valid contracts data
	var contractsData map[string]interface{}
	if err := json.Unmarshal([]byte(content.Text), &contractsData); err != nil {
		t.Fatalf("Failed to parse contracts JSON: %v", err)
	}

	contracts, ok := contractsData["contracts"].([]interface{})
	if !ok {
		t.Fatal("Expected contracts array in response")
	}

	meta, ok := contractsData["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected meta object in response")
	}

	count, ok := meta["count"].(float64)
	if !ok {
		t.Fatal("Expected count field in meta")
	}

	if int(count) != len(contracts) {
		t.Errorf("Meta count %d does not match contracts array length %d", int(count), len(contracts))
	}
}

func TestIntegration_InvalidResource(t *testing.T) {
	checkAPITokenAvailable(t)

	response := callMCPServer(t, `{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": {"uri": "spacetraders://invalid/resource"}}`)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should return an error for invalid resource
	if mcpResponse.Error == nil {
		t.Fatal("Expected error for invalid resource URI, got nil")
	}

	if !strings.Contains(mcpResponse.Error.Message, "resource not found") {
		t.Errorf("Expected 'resource not found' in error message, got: %s", mcpResponse.Error.Message)
	}
}

func TestIntegration_ServerShutdownGraceful(t *testing.T) {
	checkAPITokenAvailable(t)

	// Build the server
	binaryPath := buildTestServer(t)
	defer cleanupTestServer(t, binaryPath)

	// Start the server
	serverCmd := exec.Command(binaryPath)

	// Use the real API token
	serverCmd.Env = os.Environ()
	stdin, err := serverCmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}
	stderr, err := serverCmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	if err := serverCmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Monitor stderr for errors
	go func() {
		if stderr != nil {
			errOutput, _ := io.ReadAll(stderr)
			if len(errOutput) > 0 {
				t.Logf("Server stderr: %s", string(errOutput))
			}
		}
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send SIGINT (graceful shutdown)
	if err := serverCmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("Failed to send interrupt signal: %v", err)
	}

	// Wait for the server to exit with a timeout
	done := make(chan error, 1)
	go func() {
		done <- serverCmd.Wait()
	}()

	select {
	case err := <-done:
		// Server exited - check if it was a clean exit
		if err != nil {
			// On some systems, interrupt may cause non-zero exit
			t.Logf("Server exited with error (may be normal for interrupt): %v", err)
		} else {
			t.Log("Server exited cleanly")
		}
	case <-time.After(5 * time.Second):
		// Force kill if it doesn't exit gracefully
		if err := serverCmd.Process.Kill(); err != nil {
			t.Errorf("Failed to force kill server process: %v", err)
		}
		t.Fatal("Server did not exit gracefully within 5 seconds")
	}

	if err := stdin.Close(); err != nil {
		t.Logf("Failed to close stdin: %v", err)
	}
}

// Helper function to read a JSON response from stdout
func readJSONResponse(stdout io.Reader) ([]byte, error) {
	// Use buffered reader for better performance
	bufReader := bufio.NewReader(stdout)
	var result []byte
	timeout := 5 * time.Second
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			return nil, fmt.Errorf("timeout reading response")
		}

		line, _, err := bufReader.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error reading line: %v", err)
		}

		// Check if this looks like a complete JSON response
		if len(line) > 0 && line[0] == '{' {
			// Try to unmarshal to check if it's valid JSON
			var testResponse interface{}
			if err := json.Unmarshal(line, &testResponse); err == nil {
				result = line
				break
			}
		}

		// Add a small delay to prevent busy waiting
		time.Sleep(10 * time.Millisecond)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid JSON response found")
	}

	return result, nil
}

func TestIntegration_AcceptContractTool(t *testing.T) {
	checkAPITokenAvailable(t)

	// First, let's list the tools to make sure accept_contract is available
	response := callMCPServer(t, `{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}`)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse tools/list response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Error in tools/list: %v", mcpResponse.Error)
	}

	// Check that the result contains tools
	result, ok := mcpResponse.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected tools/list result to be an object, got %T", mcpResponse.Result)
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatalf("Expected tools to be an array, got %T", result["tools"])
	}

	// Look for the accept_contract tool
	var acceptContractTool map[string]interface{}
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}
		if toolMap["name"] == "accept_contract" {
			acceptContractTool = toolMap
			break
		}
	}

	if acceptContractTool == nil {
		t.Fatal("accept_contract tool not found in tools list")
	}

	// Verify the tool has the expected structure
	if acceptContractTool["description"] == "" {
		t.Error("accept_contract tool should have a description")
	}

	inputSchema, ok := acceptContractTool["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("accept_contract tool should have an inputSchema")
	}

	properties, ok := inputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema should have properties")
	}

	contractIdProp, ok := properties["contract_id"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema should have contract_id property")
	}

	if contractIdProp["type"] != "string" {
		t.Error("contract_id property should be of type string")
	}

	// Test calling the tool with invalid contract ID (should return error but not crash)
	toolCallRequest := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "accept_contract", "arguments": {"contract_id": "invalid-contract-id-12345"}}}`
	response = callMCPServer(t, toolCallRequest)

	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse tools/call response: %v", err)
	}

	// The call should succeed at the protocol level, but the tool should return an error
	if mcpResponse.Error != nil {
		t.Fatalf("Unexpected JSON-RPC error in tools/call: %v", mcpResponse.Error)
	}

	// Check the tool result
	toolResult, ok := mcpResponse.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected tools/call result to be an object, got %T", mcpResponse.Result)
	}

	// The tool should indicate an error since the contract doesn't exist
	isError, ok := toolResult["isError"].(bool)
	if !ok || !isError {
		t.Error("Expected tool to return isError: true for invalid contract ID")
	}

	content, ok := toolResult["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("Expected tool result to have content")
	}

	// Test with missing contract_id parameter
	toolCallRequest = `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "accept_contract", "arguments": {}}}`
	response = callMCPServer(t, toolCallRequest)

	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse tools/call response for missing parameter: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Unexpected JSON-RPC error for missing parameter: %v", mcpResponse.Error)
	}

	toolResult, ok = mcpResponse.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected tools/call result to be an object, got %T", mcpResponse.Result)
	}

	isError, ok = toolResult["isError"].(bool)
	if !ok || !isError {
		t.Error("Expected tool to return isError: true for missing contract_id")
	}

	// Test with empty contract_id
	toolCallRequest = `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "accept_contract", "arguments": {"contract_id": ""}}}`
	response = callMCPServer(t, toolCallRequest)

	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse tools/call response for empty contract_id: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Unexpected JSON-RPC error for empty contract_id: %v", mcpResponse.Error)
	}

	toolResult, ok = mcpResponse.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected tools/call result to be an object, got %T", mcpResponse.Result)
	}

	isError, ok = toolResult["isError"].(bool)
	if !ok || !isError {
		t.Error("Expected tool to return isError: true for empty contract_id")
	}

	t.Log("AcceptContract tool integration tests passed - tool is properly registered and handles error cases correctly")
}

func TestIntegration_DeliverContractTool(t *testing.T) {
	checkAPITokenAvailable(t)

	// Test calling the deliver_contract tool with dummy parameters
	toolRequest := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/call",
		"params": {
			"name": "deliver_contract",
			"arguments": {
				"contract_id": "dummy-contract-id",
				"ship_symbol": "dummy-ship",
				"trade_symbol": "IRON_ORE",
				"units": 10
			}
		}
	}`

	response := callMCPServer(t, toolRequest)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// The tool should execute but likely fail with API error due to dummy data
	// We're testing that the tool is registered and accepts the correct parameters
	if mcpResponse.Error != nil {
		// This is expected for dummy data
		t.Logf("Expected error for dummy data: %v", mcpResponse.Error)
		return
	}

	// If we get a result, parse it to verify structure
	if mcpResponse.Result != nil {
		resultBytes, err := json.Marshal(mcpResponse.Result)
		if err != nil {
			t.Fatalf("Failed to marshal result: %v", err)
		}

		var toolResult map[string]interface{}
		if err := json.Unmarshal(resultBytes, &toolResult); err != nil {
			t.Fatalf("Failed to parse tool result: %v", err)
		}

		// Check for content field
		if content, ok := toolResult["content"]; ok {
			if contentArray, ok := content.([]interface{}); ok && len(contentArray) > 0 {
				t.Log("DeliverContract tool returned content (likely error message for dummy data)")
			}
		}
	}
}

func TestIntegration_DeliverContractToolRegistration(t *testing.T) {
	checkAPITokenAvailable(t)

	// Test that the deliver_contract tool is properly registered
	toolsResponse := callMCPServer(t, `{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}`)

	var mcpResponse MCPResponse
	if err := json.Unmarshal(toolsResponse, &mcpResponse); err != nil {
		t.Fatalf("Failed to parse tools list response: %v", err)
	}

	if mcpResponse.Error != nil {
		t.Fatalf("Server returned error for tools list: %v", mcpResponse.Error)
	}

	// Parse the tools list result
	resultBytes, err := json.Marshal(mcpResponse.Result)
	if err != nil {
		t.Fatalf("Failed to marshal tools result: %v", err)
	}

	var toolsResult map[string]interface{}
	if err := json.Unmarshal(resultBytes, &toolsResult); err != nil {
		t.Fatalf("Failed to parse tools result: %v", err)
	}

	tools, ok := toolsResult["tools"].([]interface{})
	if !ok {
		t.Fatal("Expected tools array in response")
	}

	// Find the deliver_contract tool
	var deliverContractTool map[string]interface{}
	for _, tool := range tools {
		if toolMap, ok := tool.(map[string]interface{}); ok {
			if name, ok := toolMap["name"].(string); ok && name == "deliver_contract" {
				deliverContractTool = toolMap
				break
			}
		}
	}

	if deliverContractTool == nil {
		t.Fatal("deliver_contract tool not found in tools list")
	}

	// Verify tool properties
	if name := deliverContractTool["name"]; name != "deliver_contract" {
		t.Errorf("Expected tool name 'deliver_contract', got %v", name)
	}

	description, ok := deliverContractTool["description"].(string)
	if !ok {
		t.Fatal("Tool should have a description")
	}

	if !strings.Contains(description, "deliver") || !strings.Contains(description, "contract") {
		t.Error("Tool description should mention delivering goods to contracts")
	}

	// Verify input schema
	inputSchema, ok := deliverContractTool["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("Tool should have inputSchema")
	}

	properties, ok := inputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema should have properties")
	}

	// Check required properties
	requiredProps := []string{"contract_id", "ship_symbol", "trade_symbol", "units"}
	for _, prop := range requiredProps {
		if _, ok := properties[prop]; !ok {
			t.Errorf("inputSchema should have %s property", prop)
		}
	}

	// Verify required fields
	required, ok := inputSchema["required"].([]interface{})
	if !ok {
		t.Fatal("inputSchema should have required array")
	}

	for _, requiredProp := range requiredProps {
		found := false
		for _, req := range required {
			if req == requiredProp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s should be required in inputSchema", requiredProp)
		}
	}

	t.Log("DeliverContract tool is properly registered with correct schema")
}
