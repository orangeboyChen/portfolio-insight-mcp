package application

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

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

// RefreshPortfolio triggers market quote sync and portfolio recalculation.
// Should be called before reading data to ensure dayChange reflects current prices.
func (s *PortfolioService) RefreshPortfolio(ctx context.Context) error {
	return s.repo.RefreshPortfolio(ctx)
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
	accountMap := make(map[string]string)
	for _, a := range accounts {
		accountIDs = append(accountIDs, a.ID)
		accountMap[a.ID] = a.Name
	}

	performances, err := s.repo.GetAccountPerformance(ctx, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("get account performance: %w", err)
	}

	var totalDayGain float64
	var totalValue float64
	var currency string
	for i, p := range performances {
		performances[i].AccountName = accountMap[p.AccountID]
		if currency == "" {
			currency = p.BaseCurrency
		}
		val, _ := strconv.ParseFloat(p.DayGainLossAmount.String(), 64)
		totalDayGain += val
		tv, _ := strconv.ParseFloat(p.TotalValue.String(), 64)
		totalValue += tv
	}

	if currency == "" {
		currency = "USD"
	}

	// Calculate total day gain/loss percentage: dayGain / (totalValue - dayGain) * 100
	var totalDayGainPct float64
	prevValue := totalValue - totalDayGain
	if prevValue > 0 {
		totalDayGainPct = (totalDayGain / prevValue) * 100
	}

	return &portfolio.PortfolioSummary{
		TotalDayGainLoss:    fmt.Sprintf("%.2f", totalDayGain),
		TotalDayGainLossPct: fmt.Sprintf("%.4f", totalDayGainPct),
		TotalValue:          fmt.Sprintf("%.2f", totalValue),
		Currency:            currency,
		Accounts:            performances,
	}, nil
}

// GetRecentActivities returns recent transaction activities (trades, dividends, etc.).
// limit controls how many records to return (default 20, max 100).
func (s *PortfolioService) GetRecentActivities(ctx context.Context, limit int) (*portfolio.ActivitySearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	result, err := s.repo.SearchActivities(ctx, 0, limit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("search activities: %w", err)
	}

	return result, nil
}

// PeriodChange holds gain amount and percentage for a time period.
type PeriodChange struct {
	Gain    string `json:"gain"`
	GainPct string `json:"gainPct"`
}

// PortfolioOverview provides a high-level snapshot of the entire portfolio.
type PortfolioOverview struct {
	TotalMarketValue  string        `json:"totalMarketValue"`
	TotalCostBasis    string        `json:"totalCostBasis"`
	TotalGainLoss     string        `json:"totalGainLoss"`
	TotalGainLossPct  string        `json:"totalGainLossPct"`
	TotalDayChange    string        `json:"totalDayChange"`
	TotalDayChangePct string        `json:"totalDayChangePct"`
	WeekChange        *PeriodChange `json:"weekChange,omitempty"`
	MonthChange       *PeriodChange `json:"monthChange,omitempty"`
	HoldingsCount     int           `json:"holdingsCount"`
	Currency          string        `json:"currency"`
}

// GetPortfolioOverview returns a high-level portfolio snapshot: total market value,
// cost basis, unrealized gain/loss (amount and %), day/week/month change.
func (s *PortfolioService) GetPortfolioOverview(ctx context.Context) (*PortfolioOverview, error) {
	accounts, err := s.repo.GetAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	accountIDs := make([]string, 0, len(accounts))
	for _, a := range accounts {
		accountIDs = append(accountIDs, a.ID)
	}

	holdings, err := s.repo.GetAllHoldings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all holdings: %w", err)
	}

	var totalMarketValue, totalCostBasis, totalDayChange float64
	var currency string

	for _, h := range holdings {
		if currency == "" {
			currency = h.Instrument.Currency
		}
		mv, _ := strconv.ParseFloat(h.MarketValue.Base.String(), 64)
		cb, _ := strconv.ParseFloat(h.CostBasis.Base.String(), 64)
		dc, _ := strconv.ParseFloat(h.DayChange.Base.String(), 64)
		totalMarketValue += mv
		totalCostBasis += cb
		totalDayChange += dc
	}

	if currency == "" {
		currency = "USD"
	}

	totalGainLoss := totalMarketValue - totalCostBasis
	var totalGainLossPct float64
	if totalCostBasis > 0 {
		totalGainLossPct = (totalGainLoss / totalCostBasis) * 100
	}

	var totalDayChangePct float64
	prevValue := totalMarketValue - totalDayChange
	if prevValue > 0 {
		totalDayChangePct = (totalDayChange / prevValue) * 100
	}

	overview := &PortfolioOverview{
		TotalMarketValue:  fmt.Sprintf("%.2f", totalMarketValue),
		TotalCostBasis:    fmt.Sprintf("%.2f", totalCostBasis),
		TotalGainLoss:     fmt.Sprintf("%.2f", totalGainLoss),
		TotalGainLossPct:  fmt.Sprintf("%.4f", totalGainLossPct),
		TotalDayChange:    fmt.Sprintf("%.2f", totalDayChange),
		TotalDayChangePct: fmt.Sprintf("%.4f", totalDayChangePct),
		HoldingsCount:     len(holdings),
		Currency:          currency,
	}

	// Fetch week and month performance history (best-effort, don't fail if unavailable)
	now := time.Now()
	endDate := now.Format("2006-01-02")

	weekStart := now.AddDate(0, 0, -7).Format("2006-01-02")
	if weekPerf, err := s.repo.GetPerformanceHistory(ctx, "account", "all", weekStart, endDate, accountIDs); err == nil && weekPerf != nil {
		gain := weekPerf.PeriodGain.String()
		gainPct := weekPerf.PeriodReturn.String()
		overview.WeekChange = &PeriodChange{Gain: gain, GainPct: gainPct}
	}

	monthStart := now.AddDate(0, 0, -30).Format("2006-01-02")
	if monthPerf, err := s.repo.GetPerformanceHistory(ctx, "account", "all", monthStart, endDate, accountIDs); err == nil && monthPerf != nil {
		gain := monthPerf.PeriodGain.String()
		gainPct := monthPerf.PeriodReturn.String()
		overview.MonthChange = &PeriodChange{Gain: gain, GainPct: gainPct}
	}

	return overview, nil
}

// AssetAllocationItem represents one asset class in the allocation breakdown.
type AssetAllocationItem struct {
	AssetClass  string `json:"assetClass"`
	MarketValue string `json:"marketValue"`
	CostBasis   string `json:"costBasis"`
	GainLoss    string `json:"gainLoss"`
	GainLossPct string `json:"gainLossPct"`
	Weight      string `json:"weight"`
	Count       int    `json:"holdingsCount"`
}

// AssetAllocationResult holds the full asset allocation breakdown.
type AssetAllocationResult struct {
	TotalMarketValue string                `json:"totalMarketValue"`
	Currency         string                `json:"currency"`
	Allocation       []AssetAllocationItem `json:"allocation"`
}

// GetAssetAllocation returns portfolio allocation grouped by asset class.
func (s *PortfolioService) GetAssetAllocation(ctx context.Context) (*AssetAllocationResult, error) {
	holdings, err := s.repo.GetAllHoldings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all holdings: %w", err)
	}

	type classData struct {
		marketValue float64
		costBasis   float64
		count       int
	}

	classes := make(map[string]*classData)
	var totalMarketValue float64
	var currency string

	for _, h := range holdings {
		if currency == "" {
			currency = h.Instrument.Currency
		}
		mv, _ := strconv.ParseFloat(h.MarketValue.Base.String(), 64)
		cb, _ := strconv.ParseFloat(h.CostBasis.Base.String(), 64)
		totalMarketValue += mv

		ac := h.Instrument.AssetClass
		if ac == "" {
			ac = "Other"
		}
		if classes[ac] == nil {
			classes[ac] = &classData{}
		}
		classes[ac].marketValue += mv
		classes[ac].costBasis += cb
		classes[ac].count++
	}

	if currency == "" {
		currency = "USD"
	}

	allocation := make([]AssetAllocationItem, 0, len(classes))
	for ac, d := range classes {
		gainLoss := d.marketValue - d.costBasis
		var gainLossPct float64
		if d.costBasis > 0 {
			gainLossPct = (gainLoss / d.costBasis) * 100
		}
		var weight float64
		if totalMarketValue > 0 {
			weight = (d.marketValue / totalMarketValue) * 100
		}
		allocation = append(allocation, AssetAllocationItem{
			AssetClass:  ac,
			MarketValue: fmt.Sprintf("%.2f", d.marketValue),
			CostBasis:   fmt.Sprintf("%.2f", d.costBasis),
			GainLoss:    fmt.Sprintf("%.2f", gainLoss),
			GainLossPct: fmt.Sprintf("%.4f", gainLossPct),
			Weight:      fmt.Sprintf("%.2f", weight),
			Count:       d.count,
		})
	}

	return &AssetAllocationResult{
		TotalMarketValue: fmt.Sprintf("%.2f", totalMarketValue),
		Currency:         currency,
		Allocation:       allocation,
	}, nil
}

// symbolPriceChange holds the week/month price change for a symbol derived from quote history.
type symbolPriceChange struct {
	weekChangePct  string
	monthChangePct string
}

// GetHoldingsDetail returns detailed holding information for all positions,
// including 7-day and 30-day price change percentage per symbol.
// Price changes are computed from historical quotes: (currentPrice - pastPrice) / pastPrice * 100.
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

	// Collect unique holding IDs (asset IDs used in quote history API).
	// The quote history API uses the holding ID (asset UUID), not the ticker symbol.
	type holdingInfo struct {
		id    string // holding ID used as quote history symbol param
		price float64
	}
	uniqueAssets := make(map[string]*holdingInfo)
	for _, h := range holdings {
		if _, exists := uniqueAssets[h.ID]; !exists {
			price, _ := strconv.ParseFloat(h.Price.String(), 64)
			uniqueAssets[h.ID] = &holdingInfo{id: h.ID, price: price}
		}
	}

	// Fetch quote history per asset concurrently (best-effort).
	now := time.Now()
	weekCutoff := now.AddDate(0, 0, -7)
	monthCutoff := now.AddDate(0, 0, -30)

	changeMap := make(map[string]*symbolPriceChange)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for assetID, info := range uniqueAssets {
		wg.Add(1)
		go func(id string, currentPrice float64) {
			defer wg.Done()

			quotes, err := s.repo.GetQuoteHistory(ctx, id)
			if err != nil || len(quotes) == 0 {
				return
			}

			sc := &symbolPriceChange{}

			// Find closest quote <= weekCutoff and <= monthCutoff by scanning in reverse.
			// Quotes are typically sorted chronologically, so reverse scan finds the most recent
			// quote before each cutoff date.
			var weekClose, monthClose float64
			for i := len(quotes) - 1; i >= 0; i-- {
				q := quotes[i]
				t, err := time.Parse(time.RFC3339, q.Timestamp)
				if err != nil {
					t, err = time.Parse("2006-01-02T15:04:05Z", q.Timestamp)
					if err != nil {
						continue
					}
				}
				close, _ := strconv.ParseFloat(q.Close.String(), 64)
				if close == 0 {
					continue
				}
				if weekClose == 0 && !t.After(weekCutoff) {
					weekClose = close
				}
				if monthClose == 0 && !t.After(monthCutoff) {
					monthClose = close
				}
				if weekClose != 0 && monthClose != 0 {
					break
				}
			}

			if weekClose > 0 && currentPrice > 0 {
				pct := (currentPrice - weekClose) / weekClose * 100
				sc.weekChangePct = fmt.Sprintf("%.4f", pct)
			}
			if monthClose > 0 && currentPrice > 0 {
				pct := (currentPrice - monthClose) / monthClose * 100
				sc.monthChangePct = fmt.Sprintf("%.4f", pct)
			}

			mu.Lock()
			changeMap[id] = sc
			mu.Unlock()
		}(assetID, info.price)
	}
	wg.Wait()

	details := make([]portfolio.HoldingDetail, 0, len(holdings))
	for _, h := range holdings {
		// Resolve account name: prefer accountId, fall back to sourceAccountIds for aggregated holdings.
		acctName := accountMap[h.AccountID]
		if acctName == "" && len(h.SourceAccountIDs) > 0 {
			names := make([]string, 0, len(h.SourceAccountIDs))
			for _, id := range h.SourceAccountIDs {
				if n := accountMap[id]; n != "" {
					names = append(names, n)
				}
			}
			acctName = strings.Join(names, ", ")
		}

		d := portfolio.HoldingDetail{
			AccountName:  acctName,
			Symbol:       h.Instrument.Symbol,
			Name:         h.Instrument.Name,
			AssetClass:   h.Instrument.AssetClass,
			Currency:     h.Instrument.Currency,
			Quantity:     h.Quantity.String(),
			Price:        h.Price.String(),
			MarketValue:  h.MarketValue.Base.String(),
			CostBasis:    h.CostBasis.Base.String(),
			DayChange:    h.DayChange.Base.String(),
			DayChangePct: h.DayChangePct.String(),
			TotalGain:    h.TotalGain.Base.String(),
			TotalGainPct: h.TotalGainPct.String(),
			Weight:       h.Weight.String(),
			AsOfDate:     h.AsOfDate,
		}
		if sc, ok := changeMap[h.ID]; ok {
			d.WeekChangePct = sc.weekChangePct
			d.MonthChangePct = sc.monthChangePct
		}
		details = append(details, d)
	}

	return details, nil
}
