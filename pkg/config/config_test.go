package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoad_WithEnvFile(t *testing.T) {
	// Reset viper state
	viper.Reset()

	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a test .env file
	envContent := `SPACETRADERS_API_TOKEN="test-token-from-file"`
	envFile := filepath.Join(tmpDir, ".env")
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test .env file: %v", err)
	}

	// Change to temp directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Clear any existing environment variable
	if err := os.Unsetenv("SPACETRADERS_API_TOKEN"); err != nil {
		t.Errorf("Failed to unset environment variable: %v", err)
	}

	// Test Load
	config, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	expectedToken := "test-token-from-file"
	if config.SpaceTradersAPIToken != expectedToken {
		t.Errorf("Expected token %s, got %s", expectedToken, config.SpaceTradersAPIToken)
	}
}

func TestLoad_WithEnvironmentVariable(t *testing.T) {
	// Reset viper state
	viper.Reset()

	// Create a temporary directory without .env file
	tmpDir := t.TempDir()

	// Change to temp directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Set environment variable
	testToken := "test-token-from-env"
	if err := os.Setenv("SPACETRADERS_API_TOKEN", testToken); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("SPACETRADERS_API_TOKEN"); err != nil {
			t.Errorf("Failed to unset environment variable: %v", err)
		}
	}()

	// Test Load
	config, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if config.SpaceTradersAPIToken != testToken {
		t.Errorf("Expected token %s, got %s", testToken, config.SpaceTradersAPIToken)
	}
}

func TestLoad_MissingToken(t *testing.T) {
	// Reset viper state
	viper.Reset()

	// Create a temporary directory without .env file
	tmpDir := t.TempDir()

	// Change to temp directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Clear environment variable
	if err := os.Unsetenv("SPACETRADERS_API_TOKEN"); err != nil {
		t.Errorf("Failed to unset environment variable: %v", err)
	}

	// Test Load
	config, err := Load()
	if err == nil {
		t.Fatal("Expected error for missing token, got nil")
	}

	if config != nil {
		t.Error("Expected nil config on error, got non-nil")
	}

	expectedError := "SPACETRADERS_API_TOKEN is required"
	if err.Error() != expectedError {
		t.Errorf("Expected error message '%s', got '%s'", expectedError, err.Error())
	}
}

func TestLoad_EmptyToken(t *testing.T) {
	// Reset viper state
	viper.Reset()

	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a test .env file with empty token
	envContent := `SPACETRADERS_API_TOKEN=""`
	envFile := filepath.Join(tmpDir, ".env")
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test .env file: %v", err)
	}

	// Change to temp directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Clear any existing environment variable
	if err := os.Unsetenv("SPACETRADERS_API_TOKEN"); err != nil {
		t.Errorf("Failed to unset environment variable: %v", err)
	}

	// Test Load
	config, err := Load()
	if err == nil {
		t.Fatal("Expected error for empty token, got nil")
	}

	if config != nil {
		t.Error("Expected nil config on error, got non-nil")
	}
}

func TestLoad_EnvironmentOverridesFile(t *testing.T) {
	// Reset viper state
	viper.Reset()

	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a test .env file
	envContent := `SPACETRADERS_API_TOKEN="token-from-file"`
	envFile := filepath.Join(tmpDir, ".env")
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test .env file: %v", err)
	}

	// Change to temp directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Set environment variable (should override file)
	envToken := "token-from-env"
	if err := os.Setenv("SPACETRADERS_API_TOKEN", envToken); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("SPACETRADERS_API_TOKEN"); err != nil {
			t.Errorf("Failed to unset environment variable: %v", err)
		}
	}()

	// Test Load
	config, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Environment variable should take precedence
	if config.SpaceTradersAPIToken != envToken {
		t.Errorf("Expected token from environment %s, got %s", envToken, config.SpaceTradersAPIToken)
	}
}

func TestLoad_InvalidEnvFile(t *testing.T) {
	// Reset viper state
	viper.Reset()

	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create an invalid .env file (directory instead of file)
	envDir := filepath.Join(tmpDir, ".env")
	err := os.Mkdir(envDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test .env directory: %v", err)
	}

	// Change to temp directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Set environment variable so we don't fail on missing token
	if err := os.Setenv("SPACETRADERS_API_TOKEN", "test-token"); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("SPACETRADERS_API_TOKEN"); err != nil {
			t.Errorf("Failed to unset environment variable: %v", err)
		}
	}()

	// Test Load - should handle invalid config file gracefully
	config, err := Load()
	if err != nil {
		t.Fatalf("Load should handle invalid config file gracefully, got error: %v", err)
	}

	// Should still get the token from environment
	if config.SpaceTradersAPIToken != "test-token" {
		t.Errorf("Expected token from environment, got %s", config.SpaceTradersAPIToken)
	}
}
