package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all configuration values for the application.
type Config struct {
	// Wealthfolio API settings
	WealthfolioBaseURL  string
	WealthfolioPassword string

	// MCP server settings
	MCPTransport string // "stdio" or "http"
	MCPAddr      string // Listen address for HTTP mode (e.g., ":8080")
	MCPAuthToken string // Fixed bearer token for MCP API access (HTTP mode)

	// General
	LogLevel string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		WealthfolioBaseURL:  getEnv("WEALTHFOLIO_BASE_URL", ""),
		WealthfolioPassword: getEnv("WEALTHFOLIO_PASSWORD", ""),
		MCPTransport:        getEnv("MCP_TRANSPORT", "http"),
		MCPAddr:             getEnv("MCP_ADDR", ":8080"),
		MCPAuthToken:        getEnv("MCP_AUTH_TOKEN", ""),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
	}

	if cfg.WealthfolioBaseURL == "" {
		return nil, fmt.Errorf("WEALTHFOLIO_BASE_URL is required")
	}

	// Normalize: remove trailing slash
	cfg.WealthfolioBaseURL = strings.TrimRight(cfg.WealthfolioBaseURL, "/")

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
