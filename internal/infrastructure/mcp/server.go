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
		Description: "Trigger a portfolio data refresh: syncs latest market quotes and recalculates portfolio snapshots. MUST be called before querying holdings/overview if market data may be stale (e.g., at the start of a new session or after market close). Returns when the update is complete or after a timeout (max 60s). The operation is idempotent and safe to call multiple times.\n\nParameters: none.",
	}, newRefreshPortfolioHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_daily_gain_loss",
		Description: "Get yesterday's (most recent trading day) portfolio gain/loss summary across all active accounts. Returns total daily P&L as dual-currency (local + base) amounts, base currency, and per-account breakdown including day gain/loss amount, percentage, and FX rate to base. Values are converted to base currency before summing; use totalDayGainLoss.base for portfolio-level totals.\n\nParameters: none.",
	}, newDailyGainLossHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_portfolio_overview",
		Description: "Get a high-level portfolio snapshot: total market value, total cost basis, total unrealized gain/loss (amount and percentage), day change, week change (7-day gain and return %), month change (30-day gain and return %), number of holdings, and base currency. All monetary amounts are dual-currency objects with local and base values; use .base.amount for portfolio-level totals in the user's base currency. Only active accounts are included.\n\nParameters: none.",
	}, newPortfolioOverviewHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_asset_allocation",
		Description: "Get portfolio allocation breakdown by asset class (e.g., Equity, Fixed Income, Cash, Crypto). Each class includes: market value, cost basis, gain/loss as dual-currency objects (local + base), gain/loss percentage, weight percentage, and number of holdings. Use .base.amount for cross-currency comparisons. Only active accounts are included.\n\nParameters: none.",
	}, newAssetAllocationHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_holdings_detail",
		Description: "Get detailed position information for every holding across all active accounts. Each holding includes: account name, security symbol, security name, asset class, localCurrency, baseCurrency, quantity, current price, market value / cost basis / day change / total gain as dual-currency objects (local + base), day change %, 7-day price change %, 30-day price change %, portfolio weight, and data date. Use .local for per-asset display and .base for portfolio-level aggregation.\n\nParameters: none.",
	}, newHoldingsDetailHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_performance_summary",
		Description: "Get detailed portfolio performance analytics for a date range. Returns: period gain (with currency), period return %, annualized return %, cumulative/annualized TWR and Modified Dietz (when available), volatility, max drawdown, return method used, and any warnings. Defaults to 1-year lookback if no dates specified. Use for periodic (weekly/monthly) portfolio health reports.\n\nParameters:\n- startDate (string, optional): Start date in YYYY-MM-DD format. Defaults to 1 year ago if omitted.\n- endDate (string, optional): End date in YYYY-MM-DD format. Defaults to today if omitted.",
	}, newPerformanceSummaryHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_accounts",
		Description: "List all active investment accounts. Each account includes: name, account type (SECURITIES, RETIREMENT, etc.), currency, tracking mode, and optional group label. Useful for understanding the portfolio structure and grouping holdings by account in reports.\n\nParameters: none.",
	}, newAccountsHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_recent_activities",
		Description: "Get recent investment activities (trades, dividends, deposits, withdrawals, etc.) sorted by date descending. Each activity includes: account name, date, type (BUY/SELL/DIVIDEND/DEPOSIT/WITHDRAWAL/etc.), security symbol, security name, quantity, unit price, total amount, fee, and currency. Useful for reviewing recent transactions and assessing whether trading decisions are reasonable.\n\nParameters:\n- limit (integer, optional): Maximum number of records to return. Range: 1-100. Defaults to 20 if omitted.",
	}, newRecentActivitiesHandler(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_quote_history",
		Description: "Get historical price data for one or more holdings over a specified time range. Useful for trend analysis, charting, technical analysis, and comparing price movements across multiple securities. Each result includes: symbol, currency (the denomination of the price, e.g. USD/HKD/EUR), and an array of quote records (date, close price, adjusted close price). Returns results grouped by symbol. Prices are in the asset's native trading currency (see `currency` field).\n\nParameters:\n- symbols (string, REQUIRED): Comma-separated ticker symbols, e.g. \"AAPL,MSFT,BTC\". Do NOT pass as array.\n- days (integer, optional): Number of days of history to retrieve (e.g. 7, 30, 90, 180, 365). If provided, takes precedence over startDate/endDate. Defaults to 30 if neither days nor date range is specified.\n- startDate (string, optional): Start date in YYYY-MM-DD format. Used only if `days` is not provided.\n- endDate (string, optional): End date in YYYY-MM-DD format. Defaults to today if omitted.\n\nExample call: {\"symbols\": \"AAPL,NVDA,BTC\", \"days\": 30}",
	}, newQuoteHistoryHandler(svc))

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
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
	})

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
	Limit int `json:"limit,omitempty" jsonschema:"Maximum number of records to return (1-100). Defaults to 20 if omitted."`
}

func newRecentActivitiesHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input recentActivitiesInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input recentActivitiesInput) (*mcp.CallToolResult, any, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		result, err := svc.GetRecentActivities(ctx, limit)
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

type quoteHistoryInput struct {
	Symbols   []string `json:"symbols,omitempty" jsonschema:"Array of ticker symbols, e.g. [\"AAPL\", \"MSFT\", \"BTC\"]. If omitted, returns history for all current holdings."`
	Days      int      `json:"days,omitempty" jsonschema:"Number of days of history (e.g. 7, 30, 90, 365). Takes precedence over startDate/endDate. Defaults to 30 if nothing specified."`
	StartDate string   `json:"startDate,omitempty" jsonschema:"Start date in YYYY-MM-DD format. Used only if days is not provided."`
	EndDate   string   `json:"endDate,omitempty" jsonschema:"End date in YYYY-MM-DD format. Defaults to today if omitted."`
}

func newQuoteHistoryHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input quoteHistoryInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input quoteHistoryInput) (*mcp.CallToolResult, any, error) {
		symbols := input.Symbols
		days := input.Days
		result, err := svc.GetQuoteHistory(ctx, symbols, days, input.StartDate, input.EndDate)
		if err != nil {
			log.Printf("ERROR get_quote_history: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get quote history: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	}
}

type performanceSummaryInput struct {
	StartDate string `json:"startDate,omitempty" jsonschema:"Start date in YYYY-MM-DD format. Defaults to 1 year ago if omitted."`
	EndDate   string `json:"endDate,omitempty" jsonschema:"End date in YYYY-MM-DD format. Defaults to today if omitted."`
}

func newPerformanceSummaryHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input performanceSummaryInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input performanceSummaryInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.GetPerformanceSummary(ctx, input.StartDate, input.EndDate)
		if err != nil {
			log.Printf("ERROR get_performance_summary: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get performance summary: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	}
}

func newAccountsHandler(svc *application.PortfolioService) func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input emptyInput) (*mcp.CallToolResult, any, error) {
		result, err := svc.GetAccounts(ctx)
		if err != nil {
			log.Printf("ERROR get_accounts: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get accounts: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	}
}
