package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/orangeboyChen/portfolio-insight-mcp/internal/application"
	"github.com/orangeboyChen/portfolio-insight-mcp/internal/infrastructure/config"
	mcpserver "github.com/orangeboyChen/portfolio-insight-mcp/internal/infrastructure/mcp"
	"github.com/orangeboyChen/portfolio-insight-mcp/internal/infrastructure/wealthfolio"
)

func main() {
	// Load .env file (ignore error if not present)
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Infrastructure layer: Wealthfolio API client
	wfClient := wealthfolio.NewClient(cfg.WealthfolioBaseURL, cfg.WealthfolioPassword)

	// Application layer: portfolio service
	svc := application.NewPortfolioService(wfClient)

	// Interface layer: MCP server
	srv := mcpserver.NewServer(svc)

	// Start MCP server based on transport mode
	switch cfg.MCPTransport {
	case "stdio":
		log.Println("Starting MCP server in stdio mode...")
		if err := srv.MCPServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			fmt.Fprintf(os.Stderr, "MCP stdio server error: %v\n", err)
			os.Exit(1)
		}
	case "http":
		if err := srv.StartHTTP(mcpserver.ServerConfig{
			Addr:      cfg.MCPAddr,
			AuthToken: cfg.MCPAuthToken,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "MCP HTTP server error: %v\n", err)
			os.Exit(1)
		}
	default:
		log.Fatalf("Unknown MCP_TRANSPORT: %s (use 'stdio' or 'http')", cfg.MCPTransport)
	}
}
