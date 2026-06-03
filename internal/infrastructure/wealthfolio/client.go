package wealthfolio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/orangeboyChen/portfolio-insight-mcp/internal/domain/portfolio"
)

// Client implements portfolio.Repository by calling the Wealthfolio REST API.
type Client struct {
	baseURL    string
	password   string
	httpClient *http.Client
	token      string
}

// NewClient creates a new Wealthfolio API client.
func NewClient(baseURL, password string) *Client {
	return &Client{
		baseURL:  baseURL,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// authenticate logs in to Wealthfolio and stores the JWT token.
func (c *Client) authenticate(ctx context.Context) error {
	// Check if auth is required
	statusResp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/auth/status", nil)
	if err != nil {
		return fmt.Errorf("check auth status: %w", err)
	}
	defer func() { _ = statusResp.Body.Close() }()

	var status struct {
		RequiresPassword bool `json:"requiresPassword"`
	}
	if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
		return fmt.Errorf("decode auth status: %w", err)
	}

	if !status.RequiresPassword {
		return nil // No auth needed
	}

	if c.password == "" {
		return fmt.Errorf("wealthfolio requires authentication but no password configured")
	}

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": c.password})
	loginResp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = loginResp.Body.Close() }()

	if loginResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(loginResp.Body)
		return fmt.Errorf("login failed (status %d): %s", loginResp.StatusCode, string(body))
	}

	// Extract token from cookies
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "wf_session" {
			c.token = cookie.Value
			return nil
		}
	}

	// Try to extract from response body as fallback
	var loginResult struct {
		Token string `json:"token"`
	}
	// Re-read is not possible after decode above, but cookies should suffice
	_ = loginResult

	return nil
}

// ensureAuth ensures we have a valid authentication token.
func (c *Client) ensureAuth(ctx context.Context) error {
	if c.token != "" {
		return nil
	}
	return c.authenticate(ctx)
}

// doRequest performs an HTTP request with authentication headers.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return c.httpClient.Do(req)
}

// doAuthenticatedRequest performs a request with authentication, retrying once on 401.
func (c *Client) doAuthenticatedRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	resp, err := c.doRequest(ctx, method, path, bodyReader)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Retry on 401
	if resp.StatusCode == http.StatusUnauthorized {
		c.token = ""
		if err := c.authenticate(ctx); err != nil {
			return nil, fmt.Errorf("re-authenticate: %w", err)
		}
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		resp, err = c.doRequest(ctx, method, path, bodyReader)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request %s %s failed (status %d): %s", method, path, resp.StatusCode, string(respBody))
	}

	return io.ReadAll(resp.Body)
}

// GetAccounts retrieves all active, non-archived investment accounts.
// Wealthfolio has two independent flags: is_archived and is_active.
// The API query excludes archived accounts; we additionally filter out
// inactive (hidden) accounts client-side.
func (c *Client) GetAccounts(ctx context.Context) ([]portfolio.Account, error) {
	data, err := c.doAuthenticatedRequest(ctx, http.MethodGet, "/api/v1/accounts?includeArchived=false", nil)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	var accounts []portfolio.Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("decode accounts: %w", err)
	}

	filtered := accounts[:0]
	for _, a := range accounts {
		if a.IsActive {
			filtered = append(filtered, a)
		}
	}
	return filtered, nil
}

// GetAllHoldings retrieves all holdings across all accounts.
func (c *Client) GetAllHoldings(ctx context.Context) ([]portfolio.Holding, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"filter": map[string]string{"type": "all"},
	})

	data, err := c.doAuthenticatedRequest(ctx, http.MethodPost, "/api/v1/holdings/query", reqBody)
	if err != nil {
		return nil, fmt.Errorf("get all holdings: %w", err)
	}

	var holdings []portfolio.Holding
	if err := json.Unmarshal(data, &holdings); err != nil {
		return nil, fmt.Errorf("decode holdings: %w", err)
	}

	return holdings, nil
}

// GetHoldingsByAccount retrieves holdings for a specific account.
func (c *Client) GetHoldingsByAccount(ctx context.Context, accountID string) ([]portfolio.Holding, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"filter": map[string]interface{}{
			"type":      "account",
			"accountId": accountID,
		},
	})

	data, err := c.doAuthenticatedRequest(ctx, http.MethodPost, "/api/v1/holdings/query", reqBody)
	if err != nil {
		return nil, fmt.Errorf("get holdings for account %s: %w", accountID, err)
	}

	var holdings []portfolio.Holding
	if err := json.Unmarshal(data, &holdings); err != nil {
		return nil, fmt.Errorf("decode holdings: %w", err)
	}

	return holdings, nil
}

// GetAccountPerformance retrieves daily performance summary for specified accounts.
func (c *Client) GetAccountPerformance(ctx context.Context, accountIDs []string) ([]portfolio.AccountPerformance, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"accountIds": accountIDs,
	})

	data, err := c.doAuthenticatedRequest(ctx, http.MethodPost, "/api/v1/performance/accounts/simple", reqBody)
	if err != nil {
		return nil, fmt.Errorf("get account performance: %w", err)
	}

	var performances []portfolio.AccountPerformance
	if err := json.Unmarshal(data, &performances); err != nil {
		return nil, fmt.Errorf("decode performance: %w", err)
	}

	return performances, nil
}

// GetQuoteHistory retrieves historical price quotes for a given symbol (asset ID).
func (c *Client) GetQuoteHistory(ctx context.Context, symbol string) ([]portfolio.QuoteRecord, error) {
	path := "/api/v1/market-data/quotes/history?symbol=" + symbol
	data, err := c.doAuthenticatedRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get quote history for %s: %w", symbol, err)
	}

	var quotes []portfolio.QuoteRecord
	if err := json.Unmarshal(data, &quotes); err != nil {
		return nil, fmt.Errorf("decode quote history: %w", err)
	}

	return quotes, nil
}

// RefreshPortfolio triggers a portfolio update (incremental market sync + recalculation).
// The Wealthfolio API returns 202 Accepted and processes asynchronously.
// We subscribe to the SSE event stream to wait for completion or error.
func (c *Client) RefreshPortfolio(ctx context.Context) error {
	if err := c.ensureAuth(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	// Open SSE connection BEFORE triggering update so we don't miss the event.
	sseCtx, sseCancel := context.WithTimeout(ctx, 90*time.Second)
	defer sseCancel()

	sseReq, err := http.NewRequestWithContext(sseCtx, http.MethodGet, c.baseURL+"/api/v1/events/stream", nil)
	if err != nil {
		return fmt.Errorf("create SSE request: %w", err)
	}
	sseReq.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		sseReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	sseResp, err := c.httpClient.Do(sseReq)
	if err != nil {
		return fmt.Errorf("connect to event stream: %w", err)
	}
	defer func() { _ = sseResp.Body.Close() }()

	if sseResp.StatusCode != http.StatusOK {
		_ = sseResp.Body.Close()
		return fmt.Errorf("event stream returned status %d", sseResp.StatusCode)
	}

	// Now trigger the portfolio update with an empty JSON body.
	_, err = c.doAuthenticatedRequest(ctx, http.MethodPost, "/api/v1/portfolio/update", []byte("{}"))
	if err != nil {
		return fmt.Errorf("trigger portfolio update: %w", err)
	}

	// Read SSE events until we see portfolio:update-complete or portfolio:update-error.
	return c.waitForPortfolioEvent(sseCtx, sseResp.Body)
}

// waitForPortfolioEvent reads an SSE stream and returns when a portfolio completion
// or error event is received.
func (c *Client) waitForPortfolioEvent(ctx context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	var currentEvent string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for portfolio update to complete")
		default:
		}

		line := scanner.Text()

		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if line == "" {
			// Empty line = end of event
			switch currentEvent {
			case "portfolio:update-complete":
				return nil
			case "portfolio:update-error":
				return fmt.Errorf("portfolio update failed on server side")
			}
			currentEvent = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading event stream: %w", err)
	}

	return fmt.Errorf("event stream closed before receiving completion event")
}

// SearchActivities searches activities with pagination and optional filters.
func (c *Client) SearchActivities(ctx context.Context, page, pageSize int, accountIDs []string, activityTypes []string) (*portfolio.ActivitySearchResult, error) {
	body := map[string]interface{}{
		"page":     page,
		"pageSize": pageSize,
		"sort":     map[string]interface{}{"id": "date", "desc": true},
	}
	if len(accountIDs) > 0 {
		body["accountIdFilter"] = accountIDs
	}
	if len(activityTypes) > 0 {
		body["activityTypeFilter"] = activityTypes
	}

	reqBody, _ := json.Marshal(body)
	data, err := c.doAuthenticatedRequest(ctx, http.MethodPost, "/api/v1/activities/search", reqBody)
	if err != nil {
		return nil, fmt.Errorf("search activities: %w", err)
	}

	var resp struct {
		Data []struct {
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
			AssetSymbol     string      `json:"assetSymbol"`
			AssetName       *string     `json:"assetName"`
			InstrumentType  *string     `json:"instrumentType"`
			Comment         *string     `json:"comment"`
		} `json:"data"`
		Meta struct {
			TotalRowCount int `json:"totalRowCount"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode activities: %w", err)
	}

	activities := make([]portfolio.Activity, 0, len(resp.Data))
	for _, d := range resp.Data {
		var assetName, instrumentType, comment string
		if d.AssetName != nil {
			assetName = *d.AssetName
		}
		if d.InstrumentType != nil {
			instrumentType = *d.InstrumentType
		}
		if d.Comment != nil {
			comment = *d.Comment
		}
		activities = append(activities, portfolio.Activity{
			ID:              d.ID,
			AccountID:       d.AccountID,
			AccountName:     d.AccountName,
			AccountCurrency: d.AccountCurrency,
			AssetID:         d.AssetID,
			ActivityType:    d.ActivityType,
			Date:            d.Date,
			Quantity:        d.Quantity,
			UnitPrice:       d.UnitPrice,
			Amount:          d.Amount,
			Fee:             d.Fee,
			Currency:        d.Currency,
			Symbol:          d.AssetSymbol,
			SymbolName:      assetName,
			InstrumentType:  instrumentType,
			Comment:         comment,
		})
	}

	return &portfolio.ActivitySearchResult{
		Activities: activities,
		TotalCount: resp.Meta.TotalRowCount,
	}, nil
}

// GetPerformanceHistory retrieves performance metrics for a given scope and date range.
func (c *Client) GetPerformanceHistory(ctx context.Context, itemType, itemID, startDate, endDate string, accountIDs []string) (*portfolio.PerformanceHistory, error) {
	body := map[string]interface{}{
		"itemType":  itemType,
		"itemId":    itemID,
		"startDate": startDate,
		"endDate":   endDate,
	}
	if itemType == "account" && len(accountIDs) > 0 {
		body["filter"] = map[string]interface{}{
			"type":       "accounts",
			"accountIds": accountIDs,
		}
	}

	reqBody, _ := json.Marshal(body)
	data, err := c.doAuthenticatedRequest(ctx, http.MethodPost, "/api/v1/performance/history", reqBody)
	if err != nil {
		return nil, fmt.Errorf("get performance history: %w", err)
	}

	var perf portfolio.PerformanceHistory
	if err := json.Unmarshal(data, &perf); err != nil {
		return nil, fmt.Errorf("decode performance history: %w", err)
	}

	return &perf, nil
}

// GetPerformanceSummary retrieves detailed performance analytics for a given scope and date range.
func (c *Client) GetPerformanceSummary(ctx context.Context, itemType, itemID, startDate, endDate string, accountIDs []string) (*portfolio.PerformanceSummary, error) {
	body := map[string]interface{}{
		"itemType":  itemType,
		"itemId":    itemID,
		"startDate": startDate,
		"endDate":   endDate,
	}
	if itemType == "account" && len(accountIDs) > 0 {
		body["filter"] = map[string]interface{}{
			"type":       "accounts",
			"accountIds": accountIDs,
		}
	}

	reqBody, _ := json.Marshal(body)
	data, err := c.doAuthenticatedRequest(ctx, http.MethodPost, "/api/v1/performance/summary", reqBody)
	if err != nil {
		return nil, fmt.Errorf("get performance summary: %w", err)
	}

	var summary portfolio.PerformanceSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("decode performance summary: %w", err)
	}

	return &summary, nil
}
