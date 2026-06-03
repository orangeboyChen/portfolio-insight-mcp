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
// Aggregated holdings (from portfolio-wide queries) have an empty AccountID and list
// real accounts in SourceAccountIDs; we keep them if any source account is active.
func (s *PortfolioService) activeHoldings(ctx context.Context, activeIDs map[string]struct{}) ([]portfolio.Holding, error) {
	holdings, err := s.repo.GetAllHoldings(ctx)
	if err != nil {
		return nil, err
	}
	filtered := holdings[:0]
	for _, h := range holdings {
		if _, ok := activeIDs[h.AccountID]; ok {
			filtered = append(filtered, h)
			continue
		}
		if len(h.SourceAccountIDs) > 0 {
			for _, sid := range h.SourceAccountIDs {
				if _, ok := activeIDs[sid]; ok {
					filtered = append(filtered, h)
					break
				}
			}
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
	localCurrencies := make(map[string]struct{})
	for i, p := range performances {
		performances[i].AccountName = accountMap[p.AccountID]
		if baseCurrency == "" {
			baseCurrency = p.BaseCurrency
		}
		if p.AccountCurrency != "" {
			localCurrencies[p.AccountCurrency] = struct{}{}
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

	// When all accounts share one currency, the local sum is meaningful;
	// otherwise fall back to base values (cross-currency local sum is nonsensical).
	localCurrency := baseCurrency
	localDayGain := totalDayGainBase
	localValue := totalValueBase
	if len(localCurrencies) == 1 {
		for c := range localCurrencies {
			localCurrency = c
		}
		localDayGain = totalDayGainLocal
		localValue = totalValueLocal
	}

	return &portfolio.PortfolioSummary{
		TotalDayGainLoss:    makeDualMoney(localDayGain, totalDayGainBase, localCurrency, baseCurrency),
		TotalDayGainLossPct: fmt.Sprintf("%.4f", totalDayGainPct),
		TotalValue:          makeDualMoney(localValue, totalValueBase, localCurrency, baseCurrency),
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
	Gain    portfolio.Money `json:"gain"`
	GainPct string          `json:"gainPct"`
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
	var baseCurrency string
	localCurrencies := make(map[string]struct{})

	for _, h := range holdings {
		if baseCurrency == "" {
			baseCurrency = h.BaseCurrency
		}
		if h.LocalCurrency != "" {
			localCurrencies[h.LocalCurrency] = struct{}{}
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

	// When all holdings share one local currency, the local sum is meaningful;
	// otherwise fall back to base values (cross-currency local sum is nonsensical).
	localCurrency := baseCurrency
	mvLocal, cbLocal, dcLocal := totalMVBase, totalCBBase, totalDCBase
	if len(localCurrencies) == 1 {
		for c := range localCurrencies {
			localCurrency = c
		}
		mvLocal = totalMVLocal
		cbLocal = totalCBLocal
		dcLocal = totalDCLocal
	}

	totalGLBase := totalMVBase - totalCBBase
	glLocal := mvLocal - cbLocal
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
		TotalMarketValue:  makeDualMoney(mvLocal, totalMVBase, localCurrency, baseCurrency),
		TotalCostBasis:    makeDualMoney(cbLocal, totalCBBase, localCurrency, baseCurrency),
		TotalGainLoss:     makeDualMoney(glLocal, totalGLBase, localCurrency, baseCurrency),
		TotalGainLossPct:  fmt.Sprintf("%.4f", totalGainLossPct),
		TotalDayChange:    makeDualMoney(dcLocal, totalDCBase, localCurrency, baseCurrency),
		TotalDayChangePct: fmt.Sprintf("%.4f", totalDayChangePct),
		HoldingsCount:     len(holdings),
		BaseCurrency:      baseCurrency,
	}

	now := time.Now()
	endDate := now.Format("2006-01-02")

	weekStart := now.AddDate(0, 0, -7).Format("2006-01-02")
	if weekPerf, err := s.repo.GetPerformanceHistory(ctx, "account", "all", weekStart, endDate, accountIDs); err == nil && weekPerf != nil {
		perfCcy := weekPerf.Currency
		if perfCcy == "" {
			perfCcy = baseCurrency
		}
		overview.WeekChange = &PeriodChange{
			Gain:    portfolio.Money{Amount: weekPerf.PeriodGain, Currency: perfCcy},
			GainPct: weekPerf.PeriodReturn.String(),
		}
	}

	monthStart := now.AddDate(0, 0, -30).Format("2006-01-02")
	if monthPerf, err := s.repo.GetPerformanceHistory(ctx, "account", "all", monthStart, endDate, accountIDs); err == nil && monthPerf != nil {
		perfCcy := monthPerf.Currency
		if perfCcy == "" {
			perfCcy = baseCurrency
		}
		overview.MonthChange = &PeriodChange{
			Gain:    portfolio.Money{Amount: monthPerf.PeriodGain, Currency: perfCcy},
			GainPct: monthPerf.PeriodReturn.String(),
		}
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
		mvBase    float64
		cbBase    float64
		mvLocal   float64
		cbLocal   float64
		count     int
		localCcys map[string]struct{}
	}

	classes := make(map[string]*classData)
	var totalMVBase, totalMVLocal float64
	var baseCurrency string
	allLocalCcys := make(map[string]struct{})

	for _, h := range holdings {
		if baseCurrency == "" {
			baseCurrency = h.BaseCurrency
		}
		if h.LocalCurrency != "" {
			allLocalCcys[h.LocalCurrency] = struct{}{}
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
			classes[ac] = &classData{localCcys: make(map[string]struct{})}
		}
		classes[ac].mvBase += mvBase
		classes[ac].cbBase += cbBase
		classes[ac].mvLocal += mvLocal
		classes[ac].cbLocal += cbLocal
		classes[ac].count++
		if h.LocalCurrency != "" {
			classes[ac].localCcys[h.LocalCurrency] = struct{}{}
		}
	}

	if baseCurrency == "" {
		baseCurrency = "USD"
	}

	allocation := make([]AssetAllocationItem, 0, len(classes))
	for ac, d := range classes {
		glBase := d.mvBase - d.cbBase

		// Per-class local sum is only meaningful if all holdings in the class
		// share the same local currency; otherwise fall back to base values.
		classMVLocal, classCBLocal := d.mvBase, d.cbBase
		classLocalCcy := baseCurrency
		if len(d.localCcys) == 1 {
			for c := range d.localCcys {
				classLocalCcy = c
			}
			classMVLocal = d.mvLocal
			classCBLocal = d.cbLocal
		}
		glLocal := classMVLocal - classCBLocal

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
			MarketValue: makeDualMoney(classMVLocal, d.mvBase, classLocalCcy, baseCurrency),
			CostBasis:   makeDualMoney(classCBLocal, d.cbBase, classLocalCcy, baseCurrency),
			GainLoss:    makeDualMoney(glLocal, glBase, classLocalCcy, baseCurrency),
			GainLossPct: fmt.Sprintf("%.4f", gainLossPct),
			Weight:      fmt.Sprintf("%.2f", weight),
			Count:       d.count,
		})
	}

	// Total local sum: meaningful only when all holdings share one local currency.
	totalLocalCcy := baseCurrency
	totalMVLocalFinal := totalMVBase
	if len(allLocalCcys) == 1 {
		for c := range allLocalCcys {
			totalLocalCcy = c
		}
		totalMVLocalFinal = totalMVLocal
	}

	return &AssetAllocationResult{
		TotalMarketValue: makeDualMoney(totalMVLocalFinal, totalMVBase, totalLocalCcy, baseCurrency),
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

// AccountInfo is a safe-to-expose account record for MCP consumers.
type AccountInfo struct {
	Name         string `json:"name"`
	AccountType  string `json:"accountType"`
	Currency     string `json:"currency"`
	TrackingMode string `json:"trackingMode"`
	Group        string `json:"group,omitempty"`
}

// AccountListResult wraps the account list for MCP output.
type AccountListResult struct {
	Accounts []AccountInfo `json:"accounts"`
}

// GetAccounts returns the list of active accounts with metadata.
func (s *PortfolioService) GetAccounts(ctx context.Context) (*AccountListResult, error) {
	accounts, err := s.repo.GetAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	infos := make([]AccountInfo, 0, len(accounts))
	for _, a := range accounts {
		infos = append(infos, AccountInfo{
			Name:         a.Name,
			AccountType:  a.AccountType,
			Currency:     a.Currency,
			TrackingMode: a.TrackingMode,
			Group:        a.Group,
		})
	}
	return &AccountListResult{Accounts: infos}, nil
}

// PerformanceSummaryResult is the MCP-facing DTO for portfolio performance analytics.
type PerformanceSummaryResult struct {
	Currency                string          `json:"currency"`
	StartDate               string          `json:"startDate"`
	EndDate                 string          `json:"endDate"`
	PeriodGain              portfolio.Money `json:"periodGain"`
	PeriodReturn            string          `json:"periodReturn,omitempty"`
	AnnualizedReturn        string          `json:"annualizedReturn"`
	CumulativeTWR           string          `json:"cumulativeTwr,omitempty"`
	AnnualizedTWR           string          `json:"annualizedTwr,omitempty"`
	CumulativeModifiedDietz string          `json:"cumulativeModifiedDietz,omitempty"`
	AnnualizedModifiedDietz string          `json:"annualizedModifiedDietz,omitempty"`
	Volatility              string          `json:"volatility"`
	MaxDrawdown             string          `json:"maxDrawdown"`
	ReturnMethod            string          `json:"returnMethod"`
	Warnings                []string        `json:"warnings"`
}

// GetPerformanceSummary returns detailed portfolio performance analytics
// (annualized return, volatility, max drawdown, etc.) for the given date range.
// Defaults to 1 year lookback if dates are empty.
func (s *PortfolioService) GetPerformanceSummary(ctx context.Context, startDate, endDate string) (*PerformanceSummaryResult, error) {
	now := time.Now()
	if endDate == "" {
		endDate = now.Format("2006-01-02")
	}
	if startDate == "" {
		startDate = now.AddDate(-1, 0, 0).Format("2006-01-02")
	}

	accounts, err := s.repo.GetAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	accountIDs := make([]string, 0, len(accounts))
	for _, a := range accounts {
		accountIDs = append(accountIDs, a.ID)
	}

	summary, err := s.repo.GetPerformanceSummary(ctx, "account", "all", startDate, endDate, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("get performance summary: %w", err)
	}

	ccy := summary.Currency
	if ccy == "" {
		ccy = "USD"
	}

	return &PerformanceSummaryResult{
		Currency:                ccy,
		StartDate:               startDate,
		EndDate:                 endDate,
		PeriodGain:              portfolio.Money{Amount: summary.PeriodGain, Currency: ccy},
		PeriodReturn:            summary.PeriodReturn.String(),
		AnnualizedReturn:        summary.AnnualizedSimpleReturn.String(),
		CumulativeTWR:           summary.CumulativeTWR.String(),
		AnnualizedTWR:           summary.AnnualizedTWR.String(),
		CumulativeModifiedDietz: summary.CumulativeModifiedDietz.String(),
		AnnualizedModifiedDietz: summary.AnnualizedModifiedDietz.String(),
		Volatility:              summary.Volatility.String(),
		MaxDrawdown:             summary.MaxDrawdown.String(),
		ReturnMethod:            summary.ReturnMethod,
		Warnings:                summary.Warnings,
	}, nil
}
