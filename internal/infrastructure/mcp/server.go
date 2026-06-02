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
		Name:        "refresh_portfolio",
		Description: "Trigger a portfolio data refresh: syncs latest market quotes and recalculates portfolio snapshots. MUST be called before querying holdings/overview if market data may be stale (e.g., at the start of a new session or after market close). Returns when the update is complete or after a timeout (max 60s). The operation is idempotent and safe to call multiple times.",
	}, newRefreshPortfolioHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_daily_gain_loss",
		Description: "Get yesterday's (most recent trading day) portfolio gain/loss summary across all accounts. Returns total daily P&L amount, currency, and per-account breakdown including day gain/loss amount and percentage.",
	}, newDailyGainLossHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_portfolio_overview",
		Description: "Get a high-level portfolio snapshot: total market value, total cost basis, total unrealized gain/loss (amount and percentage), day change, week change (7-day gain and return %), month change (30-day gain and return %), number of holdings, and base currency. Ideal for a quick health check or daily/weekly summary header.",
	}, newPortfolioOverviewHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_asset_allocation",
		Description: "Get portfolio allocation breakdown by asset class (e.g., Equity, Fixed Income, Cash, Crypto). Each class includes: total market value, cost basis, gain/loss (amount and percentage), weight percentage, and number of holdings. Useful for rebalancing analysis or diversification assessment.",
	}, newAssetAllocationHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_holdings_detail",
		Description: "Get detailed position information for every holding across all accounts. Each holding includes: account name, security symbol, security name, asset class, currency, quantity, current price, market value, cost basis, day change (amount and percentage), 7-day change (amount and percentage), 30-day change (amount and percentage), total gain (amount and percentage), portfolio weight, and data date. Provides sufficient data for an agent to search internet news by symbol/name and compose Telegram notifications.",
	}, newHoldingsDetailHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_recent_activities",
		Description: "Get recent investment activities (trades, dividends, deposits, withdrawals, etc.) sorted by date descending. Returns up to `limit` records (default 20, max 100). Each activity includes: account name, date, type (BUY/SELL/DIVIDEND/DEPOSIT/WITHDRAWAL/etc.), security symbol, security name, quantity, unit price, total amount, fee, and currency. Useful for reviewing recent transactions and assessing whether trading decisions are reasonable.",
	}, newRecentActivitiesHandler(svc))

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

func newRefreshPortfolioHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
		if err := svc.RefreshPortfolio(ctx); err != nil {
			log.Printf("ERROR refresh_portfolio: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to refresh portfolio: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Portfolio refreshed successfully. Market quotes synced and snapshots recalculated. You can now query holdings and overview for up-to-date data."}},
		}, nil, nil
	}
}

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

func newPortfolioOverviewHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
		overview, err := svc.GetPortfolioOverview(ctx)
		if err != nil {
			log.Printf("ERROR get_portfolio_overview: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get portfolio overview: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		data, _ := json.MarshalIndent(overview, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	}
}

func newAssetAllocationHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.GetAssetAllocation(ctx)
		if err != nil {
			log.Printf("ERROR get_asset_allocation: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get asset allocation: %v", err)}},
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

type recentActivitiesInput struct {
	Limit int `json:"limit"`
}

func newRecentActivitiesHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input recentActivitiesInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input recentActivitiesInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.GetRecentActivities(ctx, input.Limit)
		if err != nil {
			log.Printf("ERROR get_recent_activities: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get recent activities: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	}
}
