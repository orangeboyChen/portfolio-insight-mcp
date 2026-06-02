package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/orangeboyChen/portfolio-insight-mcp/internal/application"
)

// ServerConfig holds configuration for the MCP HTTP server.
type ServerConfig struct {
	Addr      string
	AuthToken string
}

// Server wraps the MCP server instance.
type Server struct {
	mcpServer *mcp.Server
}

// NewServer creates a new MCP server with all tools registered.
func NewServer(svc *application.PortfolioService) *Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "portfolio-insight",
		Version: "1.0.0",
	}, nil)

	// Register tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_daily_gain_loss",
		Description: "Get yesterday's (most recent trading day) portfolio gain/loss summary across all accounts. Returns total daily P&L amount, currency, and per-account breakdown including day gain/loss amount and percentage.",
	}, newDailyGainLossHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_weekly_gain_loss",
		Description: "Get the current week's portfolio gain/loss with per-holding breakdown. Returns total weekly P&L, and for each holding: symbol, name, asset class, day change amount/percentage, total gain, market value, cost basis, and weight. Useful for identifying which securities contributed most to weekly performance.",
	}, newWeeklyGainLossHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_holdings_detail",
		Description: "Get detailed position information for every holding across all accounts. Each holding includes: account name, security symbol, security name, asset class, currency, quantity, current price, market value, cost basis, day change (amount and percentage), total gain (amount and percentage), portfolio weight, and data date. Provides sufficient data for an agent to search internet news by symbol/name and compose Telegram notifications.",
	}, newHoldingsDetailHandler(svc))

	return &Server{mcpServer: server}
}

// MCPServer returns the underlying mcp.Server (used for stdio transport).
func (s *Server) MCPServer() *mcp.Server {
	return s.mcpServer
}

// StartHTTP starts the MCP server over Streamable HTTP transport.
func (s *Server) StartHTTP(cfg ServerConfig) error {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer
	}, nil)

	// Wrap with auth middleware if token is configured
	var httpHandler http.Handler = handler
	if cfg.AuthToken != "" {
		log.Println("MCP authentication enabled (Bearer token required)")
		httpHandler = bearerAuthMiddleware(cfg.AuthToken, handler)
	} else {
		log.Println("WARNING: MCP_AUTH_TOKEN not set, running without authentication")
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", httpHandler)

	log.Printf("Starting MCP server (Streamable HTTP) on %s/mcp", cfg.Addr)
	httpServer := &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}
	return httpServer.ListenAndServe()
}

// bearerAuthMiddleware wraps an http.Handler with Bearer token authentication.
func bearerAuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			// Also check query parameter for clients that pass token via query
			if q := r.URL.Query().Get("token"); q != "" {
				auth = "Bearer " + q
			}
		}

		expected := "Bearer " + token
		if !strings.EqualFold(auth, expected) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Tool handler types - using empty struct as input since these tools take no parameters

type emptyInput struct{}

func newDailyGainLossHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
		summary, err := svc.GetDailyGainLoss(ctx)
		if err != nil {
			log.Printf("ERROR get_daily_gain_loss: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get daily gain/loss: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		data, _ := json.MarshalIndent(summary, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	}
}

func newWeeklyGainLossHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.GetWeeklyGainLoss(ctx)
		if err != nil {
			log.Printf("ERROR get_weekly_gain_loss: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get weekly gain/loss: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	}
}

func newHoldingsDetailHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
		details, err := svc.GetHoldingsDetail(ctx)
		if err != nil {
			log.Printf("ERROR get_holdings_detail: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get holdings detail: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		data, _ := json.MarshalIndent(details, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	}
}
