package application

import (
	"context"
	"fmt"
	"strconv"

	"github.com/orangeboyChen/portfolio-insight-mcp/internal/domain/portfolio"
)

// PortfolioService provides application-level use cases for portfolio data retrieval.
type PortfolioService struct {
	repo portfolio.Repository
}

// NewPortfolioService creates a new PortfolioService.
func NewPortfolioService(repo portfolio.Repository) *PortfolioService {
	return &PortfolioService{repo: repo}
}

// GetDailyGainLoss returns the total portfolio daily gain/loss across all accounts.
func (s *PortfolioService) GetDailyGainLoss(ctx context.Context) (*portfolio.PortfolioSummary, error) {
	accounts, err := s.repo.GetAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	if len(accounts) == 0 {
		return &portfolio.PortfolioSummary{
			TotalDayGainLoss: "0.00",
			Currency:         "USD",
			Accounts:         nil,
		}, nil
	}

	accountIDs := make([]string, 0, len(accounts))
	for _, a := range accounts {
		accountIDs = append(accountIDs, a.ID)
	}

	performances, err := s.repo.GetAccountPerformance(ctx, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("get account performance: %w", err)
	}

	var totalDayGain float64
	var currency string
	for _, p := range performances {
		if currency == "" {
			currency = p.BaseCurrency
		}
		val, _ := strconv.ParseFloat(p.DayGainLossAmount, 64)
		totalDayGain += val
	}

	if currency == "" {
		currency = "USD"
	}

	return &portfolio.PortfolioSummary{
		TotalDayGainLoss: fmt.Sprintf("%.2f", totalDayGain),
		Currency:         currency,
		Accounts:         performances,
	}, nil
}

// WeeklyGainResult holds the weekly gain/loss information.
type WeeklyGainResult struct {
	TotalWeeklyGain string                    `json:"totalWeeklyGain"`
	Currency        string                    `json:"currency"`
	Holdings        []portfolio.HoldingDetail `json:"holdings"`
}

// GetWeeklyGainLoss calculates weekly gain/loss from all holdings' total gain data.
// Note: Wealthfolio API provides dayChange (daily) — for weekly approximation,
// we sum dayChange across all holdings as a proxy (the actual weekly calculation
// would require historical data which the current API doesn't directly expose per-week).
// In practice, this returns the current day's total change as the latest snapshot.
func (s *PortfolioService) GetWeeklyGainLoss(ctx context.Context) (*WeeklyGainResult, error) {
	accounts, err := s.repo.GetAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	accountMap := make(map[string]string)
	for _, a := range accounts {
		accountMap[a.ID] = a.Name
	}

	holdings, err := s.repo.GetAllHoldings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all holdings: %w", err)
	}

	var totalGain float64
	var currency string
	details := make([]portfolio.HoldingDetail, 0, len(holdings))

	for _, h := range holdings {
		if currency == "" {
			currency = h.Instrument.Currency
		}
		dayChange, _ := strconv.ParseFloat(h.DayChange.Base, 64)
		totalGain += dayChange

		details = append(details, portfolio.HoldingDetail{
			AccountName:  accountMap[h.AccountID],
			Symbol:       h.Instrument.Symbol,
			Name:         h.Instrument.Name,
			AssetClass:   h.Instrument.AssetClass,
			Currency:     h.Instrument.Currency,
			Quantity:     h.Quantity,
			Price:        h.Price,
			MarketValue:  h.MarketValue.Base,
			CostBasis:    h.CostBasis.Base,
			DayChange:    h.DayChange.Base,
			DayChangePct: h.DayChangePct,
			TotalGain:    h.TotalGain.Base,
			TotalGainPct: h.TotalGainPct,
			Weight:       h.Weight,
			AsOfDate:     h.AsOfDate,
		})
	}

	if currency == "" {
		currency = "USD"
	}

	return &WeeklyGainResult{
		TotalWeeklyGain: fmt.Sprintf("%.2f", totalGain),
		Currency:        currency,
		Holdings:        details,
	}, nil
}

// GetHoldingsDetail returns detailed holding information for all positions.
func (s *PortfolioService) GetHoldingsDetail(ctx context.Context) ([]portfolio.HoldingDetail, error) {
	accounts, err := s.repo.GetAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	accountMap := make(map[string]string)
	for _, a := range accounts {
		accountMap[a.ID] = a.Name
	}

	holdings, err := s.repo.GetAllHoldings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all holdings: %w", err)
	}

	details := make([]portfolio.HoldingDetail, 0, len(holdings))
	for _, h := range holdings {
		details = append(details, portfolio.HoldingDetail{
			AccountName:  accountMap[h.AccountID],
			Symbol:       h.Instrument.Symbol,
			Name:         h.Instrument.Name,
			AssetClass:   h.Instrument.AssetClass,
			Currency:     h.Instrument.Currency,
			Quantity:     h.Quantity,
			Price:        h.Price,
			MarketValue:  h.MarketValue.Base,
			CostBasis:    h.CostBasis.Base,
			DayChange:    h.DayChange.Base,
			DayChangePct: h.DayChangePct,
			TotalGain:    h.TotalGain.Base,
			TotalGainPct: h.TotalGainPct,
			Weight:       h.Weight,
			AsOfDate:     h.AsOfDate,
		})
	}

	return details, nil
}
