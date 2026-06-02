# Development Guidelines

## Language & Runtime

- Go 1.25+
- Module: `github.com/orangeboyChen/portfolio-insight-mcp`

## Code Style & Linting

- **Must pass** `golangci-lint run --timeout=5m` with zero issues before committing.
- **Must pass** `go vet ./...`.
- All `io.Closer.Close()` return values must be explicitly handled (use `defer func() { _ = resp.Body.Close() }()` pattern).
- Do not use `string` for JSON numeric fields from external APIs — use `json.Number`.
- Keep imports grouped: stdlib, then third-party, then internal packages.

## Project Structure

```
internal/
  domain/portfolio/     # Entities, repository interface (no external deps)
  application/          # Service layer (use cases, aggregation logic)
  infrastructure/
    wealthfolio/        # Wealthfolio REST API client (implements Repository)
    mcp/                # MCP server setup, tool registration, HTTP transport
main.go                 # Entry point, wiring
```

## Architecture Rules

- Domain layer (`domain/`) must not import infrastructure or application packages.
- Application layer (`application/`) depends only on domain interfaces.
- Infrastructure layer implements domain interfaces and wires everything together.

## Testing

- Run tests: `go test ./... -race`
- Tests must pass in CI before merge.

## MCP Tools

Currently exposed tools:

| Tool | Purpose |
|------|---------|
| `refresh_portfolio` | Trigger market quote sync + portfolio recalculation; must call before querying stale data |
| `get_daily_gain_loss` | Yesterday's P&L: total amount, %, total value, per-account breakdown |
| `get_portfolio_overview` | Portfolio snapshot: total market value, cost basis, gain/loss, day/week/month change |
| `get_asset_allocation` | Allocation by asset class with market value, gain/loss, weight |
| `get_holdings_detail` | Full per-holding details across all accounts |

## CI

- Lint: `golangci-lint v2.12+` (must support Go 1.25)
- Build: `go build -trimpath -ldflags="-s -w" -o bin/server .`
- Do not change `GO_VERSION` in CI workflows without explicit approval.
