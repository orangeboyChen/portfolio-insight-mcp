package application

import (
	"context"
	"encoding/json"
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

// activeAccountSet returns the set of active account IDs and the account list.
func (s *PortfolioService) activeAccountSet(ctx context.Context) (map[string]struct{}, []portfolio.Account, error) {
	accounts, err := s.repo.GetAccounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	set := make(map[string]struct{}, len(accounts))
	for _, a := range accounts {
		set[a.ID] = struct{}{}
	}
	return set, accounts, nil
}

// activeHoldings returns holdings filtered to only those belonging to active accounts.
func (s *PortfolioService) activeHoldings(ctx context.Context, activeIDs map[string]struct{}) ([]portfolio.Holding, error) {
	holdings, err := s.repo.GetAllHoldings(ctx)
	if err != nil {
		return nil, err
	}
	filtered := holdings[:0]
	for _, h := range holdings {
		if _, ok := activeIDs[h.AccountID]; ok {
			filtered = append(filtered, h)
		}
	}
	return filtered, nil
}

func makeDualMoney(localAmount, baseAmount float64, localCurrency, baseCurrency string) portfolio.DualMoney {
	return portfolio.DualMoney{
		Local: portfolio.Money{Amount: json.Number(fmt.Sprintf("%.2f", localAmount)), Currency: localCurrency},
		Base:  portfolio.Money{Amount: json.Number(fmt.Sprintf("%.2f", baseAmount)), Currency: baseCurrency},
	}
}

// GetDailyGainLoss returns the total portfolio daily gain/loss across all accounts.
// Values are converted to base currency via fxRateToBase before summing.
func (s *PortfolioService) GetDailyGainLoss(ctx context.Context) (*portfolio.PortfolioSummary, error) {
	accounts, err := s.repo.GetAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	if len(accounts) == 0 {
		empty := makeDualMoney(0, 0, "USD", "USD")
		return &portfolio.PortfolioSummary{
			TotalDayGainLoss: empty,
			TotalValue:       empty,
			BaseCurrency:     "USD",
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

	var totalDayGainBase float64
	var totalValueBase float64
	var totalDayGainLocal float64
	var totalValueLocal float64
	var baseCurrency string
	for i, p := range performances {
		performances[i].AccountName = accountMap[p.AccountID]
		if baseCurrency == "" {
			baseCurrency = p.BaseCurrency
		}

		fxRate, _ := strconv.ParseFloat(p.FxRateToBase.String(), 64)
		if fxRate == 0 {
			fxRate = 1
		}

		dayGain, _ := strconv.ParseFloat(p.DayGainLossAmount.String(), 64)
		tv, _ := strconv.ParseFloat(p.TotalValue.String(), 64)

		totalDayGainLocal += dayGain
		totalValueLocal += tv
		totalDayGainBase += dayGain * fxRate
		totalValueBase += tv * fxRate
	}

	if baseCurrency == "" {
		baseCurrency = "USD"
	}

	var totalDayGainPct float64
	prevValue := totalValueBase - totalDayGainBase
	if prevValue > 0 {
		totalDayGainPct = (totalDayGainBase / prevValue) * 100
	}

	localCurrency := baseCurrency
	if len(performances) == 1 && performances[0].AccountCurrency != "" {
		localCurrency = performances[0].AccountCurrency
	}

	return &portfolio.PortfolioSummary{
		TotalDayGainLoss:    makeDualMoney(totalDayGainLocal, totalDayGainBase, localCurrency, baseCurrency),
		TotalDayGainLossPct: fmt.Sprintf("%.4f", totalDayGainPct),
		TotalValue:          makeDualMoney(totalValueLocal, totalValueBase, localCurrency, baseCurrency),
		BaseCurrency:        baseCurrency,
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
	TotalMarketValue  portfolio.DualMoney `json:"totalMarketValue"`
	TotalCostBasis    portfolio.DualMoney `json:"totalCostBasis"`
	TotalGainLoss     portfolio.DualMoney `json:"totalGainLoss"`
	TotalGainLossPct  string              `json:"totalGainLossPct"`
	TotalDayChange    portfolio.DualMoney `json:"totalDayChange"`
	TotalDayChangePct string              `json:"totalDayChangePct"`
	WeekChange        *PeriodChange       `json:"weekChange,omitempty"`
	MonthChange       *PeriodChange       `json:"monthChange,omitempty"`
	HoldingsCount     int                 `json:"holdingsCount"`
	BaseCurrency      string              `json:"baseCurrency"`
}

// GetPortfolioOverview returns a high-level portfolio snapshot: total market value,
// cost basis, unrealized gain/loss (amount and %), day/week/month change.
func (s *PortfolioService) GetPortfolioOverview(ctx context.Context) (*PortfolioOverview, error) {
	activeIDs, accounts, err := s.activeAccountSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	accountIDs := make([]string, 0, len(accounts))
	for _, a := range accounts {
		accountIDs = append(accountIDs, a.ID)
	}

	holdings, err := s.activeHoldings(ctx, activeIDs)
	if err != nil {
		return nil, fmt.Errorf("get all holdings: %w", err)
	}

	var totalMVBase, totalCBBase, totalDCBase float64
	var totalMVLocal, totalCBLocal, totalDCLocal float64
	var baseCurrency, localCurrency string

	for _, h := range holdings {
		if baseCurrency == "" {
			baseCurrency = h.BaseCurrency
		}
		if localCurrency == "" {
			localCurrency = h.LocalCurrency
		}
		mvBase, _ := strconv.ParseFloat(h.MarketValue.Base.String(), 64)
		cbBase, _ := strconv.ParseFloat(h.CostBasis.Base.String(), 64)
		dcBase, _ := strconv.ParseFloat(h.DayChange.Base.String(), 64)
		mvLocal, _ := strconv.ParseFloat(h.MarketValue.Local.String(), 64)
		cbLocal, _ := strconv.ParseFloat(h.CostBasis.Local.String(), 64)
		dcLocal, _ := strconv.ParseFloat(h.DayChange.Local.String(), 64)
		totalMVBase += mvBase
		totalCBBase += cbBase
		totalDCBase += dcBase
		totalMVLocal += mvLocal
		totalCBLocal += cbLocal
		totalDCLocal += dcLocal
	}

	if baseCurrency == "" {
		baseCurrency = "USD"
	}
	if localCurrency == "" {
		localCurrency = baseCurrency
	}

	totalGLBase := totalMVBase - totalCBBase
	totalGLLocal := totalMVLocal - totalCBLocal
	var totalGainLossPct float64
	if totalCBBase > 0 {
		totalGainLossPct = (totalGLBase / totalCBBase) * 100
	}

	var totalDayChangePct float64
	prevValue := totalMVBase - totalDCBase
	if prevValue > 0 {
		totalDayChangePct = (totalDCBase / prevValue) * 100
	}

	overview := &PortfolioOverview{
		TotalMarketValue:  makeDualMoney(totalMVLocal, totalMVBase, localCurrency, baseCurrency),
		TotalCostBasis:    makeDualMoney(totalCBLocal, totalCBBase, localCurrency, baseCurrency),
		TotalGainLoss:     makeDualMoney(totalGLLocal, totalGLBase, localCurrency, baseCurrency),
		TotalGainLossPct:  fmt.Sprintf("%.4f", totalGainLossPct),
		TotalDayChange:    makeDualMoney(totalDCLocal, totalDCBase, localCurrency, baseCurrency),
		TotalDayChangePct: fmt.Sprintf("%.4f", totalDayChangePct),
		HoldingsCount:     len(holdings),
		BaseCurrency:      baseCurrency,
	}

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
	AssetClass  string              `json:"assetClass"`
	MarketValue portfolio.DualMoney `json:"marketValue"`
	CostBasis   portfolio.DualMoney `json:"costBasis"`
	GainLoss    portfolio.DualMoney `json:"gainLoss"`
	GainLossPct string              `json:"gainLossPct"`
	Weight      string              `json:"weight"`
	Count       int                 `json:"holdingsCount"`
}

// AssetAllocationResult holds the full asset allocation breakdown.
type AssetAllocationResult struct {
	TotalMarketValue portfolio.DualMoney   `json:"totalMarketValue"`
	BaseCurrency     string                `json:"baseCurrency"`
	Allocation       []AssetAllocationItem `json:"allocation"`
}

// GetAssetAllocation returns portfolio allocation grouped by asset class.
func (s *PortfolioService) GetAssetAllocation(ctx context.Context) (*AssetAllocationResult, error) {
	activeIDs, _, err := s.activeAccountSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	holdings, err := s.activeHoldings(ctx, activeIDs)
	if err != nil {
		return nil, fmt.Errorf("get all holdings: %w", err)
	}

	type classData struct {
		mvBase  float64
		cbBase  float64
		mvLocal float64
		cbLocal float64
		count   int
	}

	classes := make(map[string]*classData)
	var totalMVBase, totalMVLocal float64
	var baseCurrency, localCurrency string

	for _, h := range holdings {
		if baseCurrency == "" {
			baseCurrency = h.BaseCurrency
		}
		if localCurrency == "" {
			localCurrency = h.LocalCurrency
		}
		mvBase, _ := strconv.ParseFloat(h.MarketValue.Base.String(), 64)
		cbBase, _ := strconv.ParseFloat(h.CostBasis.Base.String(), 64)
		mvLocal, _ := strconv.ParseFloat(h.MarketValue.Local.String(), 64)
		cbLocal, _ := strconv.ParseFloat(h.CostBasis.Local.String(), 64)
		totalMVBase += mvBase
		totalMVLocal += mvLocal

		ac := h.Instrument.AssetClass
		if ac == "" {
			ac = "Other"
		}
		if classes[ac] == nil {
			classes[ac] = &classData{}
		}
		classes[ac].mvBase += mvBase
		classes[ac].cbBase += cbBase
		classes[ac].mvLocal += mvLocal
		classes[ac].cbLocal += cbLocal
		classes[ac].count++
	}

	if baseCurrency == "" {
		baseCurrency = "USD"
	}
	if localCurrency == "" {
		localCurrency = baseCurrency
	}

	allocation := make([]AssetAllocationItem, 0, len(classes))
	for ac, d := range classes {
		glBase := d.mvBase - d.cbBase
		glLocal := d.mvLocal - d.cbLocal
		var gainLossPct float64
		if d.cbBase > 0 {
			gainLossPct = (glBase / d.cbBase) * 100
		}
		var weight float64
		if totalMVBase > 0 {
			weight = (d.mvBase / totalMVBase) * 100
		}
		allocation = append(allocation, AssetAllocationItem{
			AssetClass:  ac,
			MarketValue: makeDualMoney(d.mvLocal, d.mvBase, localCurrency, baseCurrency),
			CostBasis:   makeDualMoney(d.cbLocal, d.cbBase, localCurrency, baseCurrency),
			GainLoss:    makeDualMoney(glLocal, glBase, localCurrency, baseCurrency),
			GainLossPct: fmt.Sprintf("%.4f", gainLossPct),
			Weight:      fmt.Sprintf("%.2f", weight),
			Count:       d.count,
		})
	}

	return &AssetAllocationResult{
		TotalMarketValue: makeDualMoney(totalMVLocal, totalMVBase, localCurrency, baseCurrency),
		BaseCurrency:     baseCurrency,
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
	activeIDs, accounts, err := s.activeAccountSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	accountMap := make(map[string]string)
	for _, a := range accounts {
		accountMap[a.ID] = a.Name
	}

	holdings, err := s.activeHoldings(ctx, activeIDs)
	if err != nil {
		return nil, fmt.Errorf("get all holdings: %w", err)
	}

	type holdingInfo struct {
		id    string
		price float64
	}
	uniqueAssets := make(map[string]*holdingInfo)
	for _, h := range holdings {
		if _, exists := uniqueAssets[h.ID]; !exists {
			price, _ := strconv.ParseFloat(h.Price.String(), 64)
			uniqueAssets[h.ID] = &holdingInfo{id: h.ID, price: price}
		}
	}

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
			AccountName:   acctName,
			Symbol:        h.Instrument.Symbol,
			Name:          h.Instrument.Name,
			AssetClass:    h.Instrument.AssetClass,
			LocalCurrency: h.LocalCurrency,
			BaseCurrency:  h.BaseCurrency,
			Quantity:      h.Quantity.String(),
			Price:         h.Price.String(),
			MarketValue: portfolio.DualMoney{
				Local: portfolio.Money{Amount: h.MarketValue.Local, Currency: h.LocalCurrency},
				Base:  portfolio.Money{Amount: h.MarketValue.Base, Currency: h.BaseCurrency},
			},
			CostBasis: portfolio.DualMoney{
				Local: portfolio.Money{Amount: h.CostBasis.Local, Currency: h.LocalCurrency},
				Base:  portfolio.Money{Amount: h.CostBasis.Base, Currency: h.BaseCurrency},
			},
			DayChange: portfolio.DualMoney{
				Local: portfolio.Money{Amount: h.DayChange.Local, Currency: h.LocalCurrency},
				Base:  portfolio.Money{Amount: h.DayChange.Base, Currency: h.BaseCurrency},
			},
			DayChangePct: h.DayChangePct.String(),
			TotalGain: portfolio.DualMoney{
				Local: portfolio.Money{Amount: h.TotalGain.Local, Currency: h.LocalCurrency},
				Base:  portfolio.Money{Amount: h.TotalGain.Base, Currency: h.BaseCurrency},
			},
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
