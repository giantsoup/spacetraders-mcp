package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	SpaceTradersAPIToken string
}

// Load initializes and loads configuration using Viper
func Load() (*Config, error) {
	// Set config name and type
	viper.SetConfigName(".env")
	viper.SetConfigType("env")

	// Add current directory to search path
	viper.AddConfigPath(".")

	// Enable automatic environment variable binding
	viper.AutomaticEnv()

	// Try to read the config file (silently)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error since we can use env vars
			// Silent - no logging needed for normal operation
		} else {
			// Config file was found but another error was produced
			fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
		}
	}
	// Silent success - no logging needed for normal operation

	// Create config struct
	config := &Config{
		SpaceTradersAPIToken: viper.GetString("SPACETRADERS_API_TOKEN"),
	}

	// Validate required configuration
	if config.SpaceTradersAPIToken == "" {
		return nil, fmt.Errorf("SPACETRADERS_API_TOKEN is required")
	}

	return config, nil
}
