package portfolio

import "encoding/json"

// Money represents a monetary amount with its currency code.
type Money struct {
	Amount   json.Number `json:"amount"`
	Currency string      `json:"currency"`
}

// DualMoney holds a value expressed in both local (asset) and base (portfolio) currencies.
type DualMoney struct {
	Local Money `json:"local"`
	Base  Money `json:"base"`
}

// MoneyAmount represents a monetary value in both local and base currencies.
type MoneyAmount struct {
	Local json.Number `json:"local"`
	Base  json.Number `json:"base"`
}

// Instrument represents a financial instrument (stock, ETF, fund, etc.).
type Instrument struct {
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Currency      string `json:"currency"`
	AssetClass    string `json:"assetClass"`
	AssetSubClass string `json:"assetSubClass"`
}

// Holding represents a single position in a portfolio.
type Holding struct {
	ID                string      `json:"id"`
	AccountID         string      `json:"accountId"`
	HoldingType       string      `json:"holdingType"`
	Instrument        Instrument  `json:"instrument"`
	LocalCurrency     string      `json:"localCurrency"`
	BaseCurrency      string      `json:"baseCurrency"`
	Quantity          json.Number `json:"quantity"`
	Price             json.Number `json:"price"`
	MarketValue       MoneyAmount `json:"marketValue"`
	CostBasis         MoneyAmount `json:"costBasis"`
	DayChange         MoneyAmount `json:"dayChange"`
	DayChangePct      json.Number `json:"dayChangePct"`
	PrevCloseValue    MoneyAmount `json:"prevCloseValue"`
	UnrealizedGain    MoneyAmount `json:"unrealizedGain"`
	UnrealizedGainPct json.Number `json:"unrealizedGainPct"`
	TotalGain         MoneyAmount `json:"totalGain"`
	TotalGainPct      json.Number `json:"totalGainPct"`
	Weight            json.Number `json:"weight"`
	AsOfDate          string      `json:"asOfDate"`
	SourceAccountIDs  []string    `json:"sourceAccountIds"`
}

// Account represents a brokerage/investment account.
type Account struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AccountType  string `json:"accountType"`
	Currency     string `json:"currency"`
	IsActive     bool   `json:"isActive"`
	TrackingMode string `json:"trackingMode"`
	Group        string `json:"group,omitempty"`
	PlatformID   string `json:"platformId,omitempty"`
}

// AccountPerformance represents account-level daily performance summary.
// Numeric fields use json.Number to handle both string and number JSON values.
type AccountPerformance struct {
	AccountID                string      `json:"accountId"`
	AccountName              string      `json:"accountName"`
	AccountCurrency          string      `json:"accountCurrency"`
	BaseCurrency             string      `json:"baseCurrency"`
	FxRateToBase             json.Number `json:"fxRateToBase"`
	TotalValue               json.Number `json:"totalValue"`
	TotalGainLossAmount      json.Number `json:"totalGainLossAmount"`
	CumulativeReturnPercent  json.Number `json:"cumulativeReturnPercent"`
	DayGainLossAmount        json.Number `json:"dayGainLossAmount"`
	DayReturnPercentModDietz json.Number `json:"dayReturnPercentModDietz"`
	PortfolioWeight          json.Number `json:"portfolioWeight"`
}

// PortfolioSummary aggregates the total daily gain/loss across all accounts.
type PortfolioSummary struct {
	TotalDayGainLoss    DualMoney            `json:"totalDayGainLoss"`
	TotalDayGainLossPct string               `json:"totalDayGainLossPct"`
	TotalValue          DualMoney            `json:"totalValue"`
	BaseCurrency        string               `json:"baseCurrency"`
	Accounts            []AccountPerformance `json:"accounts"`
}

// PerformanceHistory represents historical performance metrics for a given period.
type PerformanceHistory struct {
	ID              string      `json:"id"`
	Currency        string      `json:"currency"`
	PeriodGain      json.Number `json:"periodGain"`
	PeriodReturn    json.Number `json:"periodReturn"`
	CumulativeTWR   json.Number `json:"cumulativeTwr"`
	AnnualizedTWR   json.Number `json:"annualizedTwr"`
	Volatility      json.Number `json:"volatility"`
	MaxDrawdown     json.Number `json:"maxDrawdown"`
	PeriodStartDate string      `json:"periodStartDate"`
	PeriodEndDate   string      `json:"periodEndDate"`
}

// PerformanceSummary holds detailed performance analytics from Wealthfolio's
// /performance/summary endpoint. Fields use json.Number to preserve precision.
type PerformanceSummary struct {
	ID                      string      `json:"id"`
	Currency                string      `json:"currency"`
	PeriodStartDate         string      `json:"periodStartDate"`
	PeriodEndDate           string      `json:"periodEndDate"`
	PeriodGain              json.Number `json:"periodGain"`
	PeriodReturn            json.Number `json:"periodReturn"`
	SimpleReturn            json.Number `json:"simpleReturn"`
	AnnualizedSimpleReturn  json.Number `json:"annualizedSimpleReturn"`
	CumulativeTWR           json.Number `json:"cumulativeTwr"`
	AnnualizedTWR           json.Number `json:"annualizedTwr"`
	CumulativeModifiedDietz json.Number `json:"cumulativeModifiedDietz"`
	AnnualizedModifiedDietz json.Number `json:"annualizedModifiedDietz"`
	Volatility              json.Number `json:"volatility"`
	MaxDrawdown             json.Number `json:"maxDrawdown"`
	IsHoldingsMode          bool        `json:"isHoldingsMode"`
	ReturnMethod            string      `json:"returnMethod"`
	IsMixedTrackingMode     bool        `json:"isMixedTrackingMode"`
	Warnings                []string    `json:"warnings"`
}

// QuoteRecord represents a single historical price quote.
type QuoteRecord struct {
	ID        string      `json:"id"`
	AssetID   string      `json:"assetId"`
	Timestamp string      `json:"timestamp"`
	Close     json.Number `json:"close"`
	AdjClose  json.Number `json:"adjclose"`
}

// Activity represents a single investment activity (buy, sell, dividend, etc.).
type Activity struct {
	ID              string      `json:"id"`
	AccountID       string      `json:"accountId"`
	AccountName     string      `json:"accountName"`
	AccountCurrency string      `json:"accountCurrency"`
	AssetID         string      `json:"assetId"`
	ActivityType    string      `json:"activityType"`
	Date            string      `json:"date"`
	Quantity        json.Number `json:"quantity"`
	UnitPrice       json.Number `json:"unitPrice"`
	Amount          json.Number `json:"amount"`
	Fee             json.Number `json:"fee"`
	Currency        string      `json:"currency"`
	Symbol          string      `json:"symbol"`
	SymbolName      string      `json:"symbolName"`
	InstrumentType  string      `json:"instrumentType,omitempty"`
	Comment         string      `json:"comment,omitempty"`
}

// ActivitySearchResult holds paginated activity search results.
type ActivitySearchResult struct {
	Activities []Activity `json:"activities"`
	TotalCount int        `json:"totalCount"`
}

// QuoteHistoryResult holds historical price quotes for one or more symbols within a date range.
type QuoteHistoryResult struct {
	Symbol    string        `json:"symbol"`
	Currency  string        `json:"currency"`
	Quotes    []QuoteRecord `json:"quotes"`
	StartDate string        `json:"startDate"`
	EndDate   string        `json:"endDate"`
}

// HoldingDetail is an enriched holding record exposed via MCP,
// designed to provide sufficient context for downstream agents.
type HoldingDetail struct {
	AccountName    string    `json:"accountName"`
	Symbol         string    `json:"symbol"`
	Name           string    `json:"name"`
	AssetClass     string    `json:"assetClass"`
	LocalCurrency  string    `json:"localCurrency"`
	BaseCurrency   string    `json:"baseCurrency"`
	Quantity       string    `json:"quantity"`
	Price          string    `json:"price"`
	MarketValue    DualMoney `json:"marketValue"`
	CostBasis      DualMoney `json:"costBasis"`
	DayChange      DualMoney `json:"dayChange"`
	DayChangePct   string    `json:"dayChangePct"`
	WeekChangePct  string    `json:"weekChangePct,omitempty"`
	MonthChangePct string    `json:"monthChangePct,omitempty"`
	TotalGain      DualMoney `json:"totalGain"`
	TotalGainPct   string    `json:"totalGainPct"`
	Weight         string    `json:"weight"`
	AsOfDate       string    `json:"asOfDate"`
}
