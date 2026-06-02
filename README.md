# Portfolio Insight MCP

A [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that bridges [Wealthfolio](https://wealthfolio.app/) portfolio data to AI agents. Enables AI platforms like [LobeHub](https://lobehub.com/) or Claude Desktop to query daily/weekly P&L and per-holding details, then perform analysis and push reports to Telegram.

## Use Case

A typical workflow: LobeHub's scheduled agent calls this MCP server every morning to fetch portfolio data from Wealthfolio, analyzes market movements with AI, and sends a daily investment report to Telegram.

```mermaid
graph LR
    A[Wealthfolio<br/>Portfolio Manager] -->|REST API| B[Portfolio Insight MCP<br/>Data Bridge]
    B -->|MCP Protocol| C[LobeHub / Claude Desktop<br/>AI Agent Platform]
    C -->|Analysis + News Search| D[AI Model<br/>GPT / Claude]
    D -->|Push Notification| E[Telegram Bot<br/>Daily Report]
```

### Call Flow

```
┌─────────────┐      ┌──────────────────────┐      ┌───────────────────┐
│ Wealthfolio │ ───► │ Portfolio Insight MCP │ ───► │ AI Agent (LobeHub)│
│ (Data Source)│ REST │ (MCP Server)         │ MCP  │                   │
└─────────────┘      └──────────────────────┘      └────────┬──────────┘
                                                             │
                                                    ┌────────▼──────────┐
                                                    │ Analyze holdings   │
                                                    │ Search market news │
                                                    │ Generate report    │
                                                    └────────┬──────────┘
                                                             │
                                                    ┌────────▼──────────┐
                                                    │ Telegram Bot       │
                                                    │ Push daily report  │
                                                    └───────────────────┘
```

**Workflow:**

1. **Data Fetching** — Portfolio Insight MCP pulls holdings, P&L from Wealthfolio's REST API
2. **MCP Exposure** — Data is exposed as standardized MCP Tools that any AI agent can call
3. **AI Analysis** — The AI agent invokes MCP Tools, combines data with internet news for comprehensive analysis
4. **Scheduled Push** — Via scheduled workflows (e.g., LobeHub cron), generates and pushes daily investment reports to Telegram

## Architecture

The project follows **Domain-Driven Design (DDD)** with a clean layered structure:

```
.
├── main.go                          # Entry point
├── internal/
│   ├── domain/portfolio/            # Domain layer: entities & repository port
│   ├── application/                 # Application layer: use-case orchestration
│   └── infrastructure/
│       ├── config/                  # Configuration loader
│       ├── wealthfolio/             # Infrastructure: Wealthfolio REST client
│       └── mcp/                     # Interface: MCP server (tool registration)
├── Dockerfile                       # Multi-stage distroless container
└── .github/workflows/               # CI/CD pipelines
```

| Layer | Responsibility |
|-------|---------------|
| **Domain** | Core business entities (`Holding`, `Account`, `HoldingDetail`) and repository interface |
| **Application** | Orchestrates use cases: daily P&L, weekly P&L, holdings detail |
| **Infrastructure** | External integrations (Wealthfolio HTTP client, config, MCP server) |

## MCP Tools

| Tool | Description |
|------|-------------|
| `get_daily_gain_loss` | Returns yesterday's (most recent trading day) total portfolio P&L with per-account breakdown |
| `get_weekly_gain_loss` | Returns this week's P&L with per-holding detail (symbol, name, day change, total gain) |
| `get_holdings_detail` | Returns full position details for every holding — sufficient for agents to search news by symbol and send Telegram alerts |

### Response Fields

Each holding detail includes: `accountName`, `symbol`, `name`, `assetClass`, `currency`, `quantity`, `price`, `marketValue`, `costBasis`, `dayChange`, `dayChangePct`, `totalGain`, `totalGainPct`, `weight`, `asOfDate`.

## Prerequisites

- A running [Wealthfolio](https://wealthfolio.app/) instance (self-hosted or cloud)
- Go 1.23+ (for local development)
- Docker (optional, for containerized deployment)

## Configuration

Copy `.env.example` to `.env` and fill in values:

```bash
cp .env.example .env
```

| Variable | Required | Description |
|----------|----------|-------------|
| `WEALTHFOLIO_BASE_URL` | Yes | Base URL of your Wealthfolio instance |
| `WEALTHFOLIO_PASSWORD` | No | Password if Wealthfolio auth is enabled |
| `MCP_TRANSPORT` | No | `stdio` or `http` (default `http`) |
| `MCP_ADDR` | No | Listen address for HTTP mode (default `:8080`) |
| `MCP_AUTH_TOKEN` | No | Bearer token for HTTP mode authentication |
| `LOG_LEVEL` | No | `debug`, `info`, `warn`, `error` |

## Running Locally

```bash
# Install dependencies
go mod download

# Run in HTTP mode (default, Streamable HTTP transport)
go run .

# Run in stdio mode (for MCP client integration, e.g., Claude Desktop)
MCP_TRANSPORT=stdio go run .
```

## Docker

```bash
# Build
docker build -t portfolio-insight-mcp .

# Run (HTTP mode)
docker run --rm --env-file .env -p 8080:8080 portfolio-insight-mcp
```

## Deployment

### Option 1: HTTP Mode (Docker)

Pre-built multi-arch images (`linux/amd64`, `linux/arm64`) are published to GHCR on every tagged release:

```bash
docker pull ghcr.io/orangeboychen/portfolio-insight-mcp:<version>

# Run as HTTP server
docker run --rm --env-file .env -p 8080:8080 ghcr.io/orangeboychen/portfolio-insight-mcp:latest
```

Then configure your MCP client to connect via Streamable HTTP:

```json
{
  "mcpServers": {
    "portfolio-insight": {
      "url": "http://<host>:8080/mcp",
      "headers": {
        "Authorization": "Bearer <your_MCP_AUTH_TOKEN>"
      }
    }
  }
}
```

### Option 2: stdio Mode (Local Binary)

Download the binary from [GitHub Releases](https://github.com/orangeboyChen/portfolio-insight-mcp/releases) or build from source:

```bash
go install github.com/orangeboyChen/portfolio-insight-mcp@latest
```

Then configure your MCP client (e.g., Claude Desktop, LobeHub) to invoke the binary directly:

```json
{
  "mcpServers": {
    "portfolio-insight": {
      "command": "/path/to/portfolio-insight-mcp",
      "env": {
        "WEALTHFOLIO_BASE_URL": "http://your-wealthfolio:3000",
        "WEALTHFOLIO_PASSWORD": "your-password",
        "MCP_TRANSPORT": "stdio"
      }
    }
  }
}
```

## CI/CD

- **CI** (`ci.yml`): Runs lint + tests on every push/PR to `master`
- **Release** (`release.yml`): On `v*.*.*` tags — lint, test, build & push multi-arch Docker image to GHCR, create GitHub Release

## License

MIT
