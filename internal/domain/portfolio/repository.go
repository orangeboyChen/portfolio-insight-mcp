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
}
