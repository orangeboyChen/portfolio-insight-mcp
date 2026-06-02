package portfolio

import "context"

// Repository defines the port for accessing portfolio data from external sources.
type Repository interface {
	// GetAccounts retrieves all active investment accounts.
	GetAccounts(ctx context.Context) ([]Account, error)

	// GetAllHoldings retrieves all holdings across all accounts.
	GetAllHoldings(ctx context.Context) ([]Holding, error)

	// GetHoldingsByAccount retrieves holdings for a specific account.
	GetHoldingsByAccount(ctx context.Context, accountID string) ([]Holding, error)

	// GetAccountPerformance retrieves daily performance summary for specified accounts.
	GetAccountPerformance(ctx context.Context, accountIDs []string) ([]AccountPerformance, error)

	// GetPerformanceHistory retrieves performance metrics for a given scope and date range.
	// itemType can be "account" or "symbol", itemID is the corresponding ID.
	// startDate and endDate are in "YYYY-MM-DD" format.
	GetPerformanceHistory(ctx context.Context, itemType, itemID, startDate, endDate string, accountIDs []string) (*PerformanceHistory, error)

	// GetQuoteHistory retrieves historical price quotes for a given symbol.
	GetQuoteHistory(ctx context.Context, symbol string) ([]QuoteRecord, error)

	// SearchActivities searches activities with pagination and filters.
	// pageSize controls how many records to return, page is 0-based.
	SearchActivities(ctx context.Context, page, pageSize int, accountIDs []string, activityTypes []string) (*ActivitySearchResult, error)

	// RefreshPortfolio triggers a portfolio update: syncs market quotes (incremental)
	// and recalculates portfolio snapshots. This should be called before reading data
	// to ensure holdings dayChange reflects current market prices.
	RefreshPortfolio(ctx context.Context) error
}
