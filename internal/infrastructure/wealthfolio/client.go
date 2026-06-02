package wealthfolio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// GetAccounts retrieves all active investment accounts.
func (c *Client) GetAccounts(ctx context.Context) ([]portfolio.Account, error) {
	data, err := c.doAuthenticatedRequest(ctx, http.MethodGet, "/api/v1/accounts?includeArchived=false", nil)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	var accounts []portfolio.Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("decode accounts: %w", err)
	}

	return accounts, nil
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
