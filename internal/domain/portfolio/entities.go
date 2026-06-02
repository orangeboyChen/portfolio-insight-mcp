package portfolio

// MoneyAmount represents a monetary value in both local and base currencies.
type MoneyAmount struct {
	Local string `json:"local"`
	Base  string `json:"base"`
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
	Quantity          string      `json:"quantity"`
	Price             string      `json:"price"`
	MarketValue       MoneyAmount `json:"marketValue"`
	CostBasis         MoneyAmount `json:"costBasis"`
	DayChange         MoneyAmount `json:"dayChange"`
	DayChangePct      string      `json:"dayChangePct"`
	PrevCloseValue    MoneyAmount `json:"prevCloseValue"`
	UnrealizedGain    MoneyAmount `json:"unrealizedGain"`
	UnrealizedGainPct string      `json:"unrealizedGainPct"`
	TotalGain         MoneyAmount `json:"totalGain"`
	TotalGainPct      string      `json:"totalGainPct"`
	Weight            string      `json:"weight"`
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
type AccountPerformance struct {
	AccountID                string `json:"accountId"`
	AccountCurrency          string `json:"accountCurrency"`
	BaseCurrency             string `json:"baseCurrency"`
	TotalValue               string `json:"totalValue"`
	TotalGainLossAmount      string `json:"totalGainLossAmount"`
	CumulativeReturnPercent  string `json:"cumulativeReturnPercent"`
	DayGainLossAmount        string `json:"dayGainLossAmount"`
	DayReturnPercentModDietz string `json:"dayReturnPercentModDietz"`
	PortfolioWeight          string `json:"portfolioWeight"`
}

// PortfolioSummary aggregates the total daily gain/loss across all accounts.
type PortfolioSummary struct {
	TotalDayGainLoss string               `json:"totalDayGainLoss"`
	Currency         string               `json:"currency"`
	Accounts         []AccountPerformance `json:"accounts"`
}

// HoldingDetail is an enriched holding record exposed via MCP,
// designed to provide sufficient context for downstream agents.
type HoldingDetail struct {
	AccountName  string `json:"accountName"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	AssetClass   string `json:"assetClass"`
	Currency     string `json:"currency"`
	Quantity     string `json:"quantity"`
	Price        string `json:"price"`
	MarketValue  string `json:"marketValue"`
	CostBasis    string `json:"costBasis"`
	DayChange    string `json:"dayChange"`
	DayChangePct string `json:"dayChangePct"`
	TotalGain    string `json:"totalGain"`
	TotalGainPct string `json:"totalGainPct"`
	Weight       string `json:"weight"`
	AsOfDate     string `json:"asOfDate"`
}
