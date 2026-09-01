# IQ Option MCP Client (Golang)

[![Go Reference](https://pkg.go.dev/badge/github.com/patrickkdev/iqoption-mcp-client.svg)](https://pkg.go.dev/github.com/patrickkdev/iqoption-mcp-client)
[![Go Report Card](https://goreportcard.com/badge/github.com/patrickkdev/iqoption-mcp-client)](https://goreportcard.com/report/github.com/patrickkdev/iqoption-mcp-client)

A resilient, high-performance Go (Golang) client for the **IQ Option API**, built on top of the **Model Context Protocol (MCP)**. This library provides a clean and reliable interface for building trading bots, automated strategies, and financial analysis tools for **Binary Options**.

---

## 🇧🇷 Para Traders Brasileiros (Opções Binárias)

Se você está procurando uma forma estável e profissional de automatizar suas estratégias na **IQ Option** usando Go, este é o cliente ideal. 

- **Automação de Sinais**: Execute trades automaticamente baseados em seus indicadores.
- **Conexão Resiliente**: Gerenciamento automático de sessão e retentativas em caso de falha de rede.
- **Análise Técnica**: Recupere velas (candles) OHLC em tempo real para alimentar seus algoritmos.
- **Suporte a MCP**: Compatível com o novo padrão de protocolos de contexto para IA e automação.

---

## Key Features

- ✅ **Account Management**: List and switch between Practice (Training) and Real (Normal) balances.
- ✅ **Market Data**: List available assets, profit percentages, and real-time open/close status.
- ✅ **Technical Analysis**: Retrieve OHLC candles (history and real-time) for any supported asset.
- ✅ **Trading Operations**: Place Binary Options trades (CALL/PUT) with server-side expiration validation.
- ✅ **Position Tracking**: Monitor open positions and retrieve full trade history.
- ✅ **Resilience**: Built-in automatic retries for idempotent operations and seamless session recovery.
- ✅ **Concurrency Safe**: Designed to be used in high-concurrency Go environments.

## Installation

```bash
go get github.com/patrickkdev/iqoption-mcp-client
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/patrickkdev/iqoption-mcp-client"
)

func main() {
	ctx := context.Background()

	// Initialize the client with your IQ Option token
	// You can obtain your token from the IQ Option MCP server or authorized portal.
	client, err := iqclient.New(iqclient.Config{
		Token: "YOUR_IQ_OPTION_TOKEN",
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// 1. List Balances
	balances, err := client.ListBalances(ctx, iqclient.BalanceTypeAll)
	if err != nil {
		log.Fatalf("Error listing balances: %v", err)
	}

	for _, b := range balances {
		fmt.Printf("Balance [%s]: %.2f %s\n", b.Type, b.Amount, b.Currency)
	}

	// 2. Get Market Data (Candles)
	// Asset 1 = EUR/USD (example)
	candles, err := client.GetCandles(ctx, 1, 60, 10) // 10 candles of 1 minute (60s)
	if err != nil {
		log.Fatalf("Error getting candles: %v", err)
	}

	for _, c := range candles {
		fmt.Printf("Time: %v | Open: %f | Close: %f\n", c.From, c.Open, c.Close)
	}

	// 3. Place a Trade (Practice Account)
	// Important: Use server-provided expirations from ListAssets
	// result, err := client.PlaceTrade(ctx, iqclient.TradeRequest{
	//     BalanceID:     trainingBalanceID,
	//     AssetID:       1,
	//     Direction:     "call",
	//     Amount:        1.0,
	//     ProfitPercent: 85,
	//     Expired:       targetExpiration,
	// })
}
```

## Model Context Protocol (MCP)

This client leverages the **Model Context Protocol**, allowing it to interact seamlessly with IQ Option MCP servers. This architecture ensures that complex business logic (like expiration calculations and protocol handshakes) is handled reliably by the upstream server while providing a type-safe, idiomatic Go experience for the developer.

## Testing

The project includes integration tests to ensure the client communicates correctly with the IQ Option MCP server. By default, these tests are skipped unless the appropriate environment variables are set.

### Running Integration Tests

To run the integration tests, you need to configure your IQ Option test token and enable the tests via environment variables:

```bash
# Set your IQ Option token
export IQOPTION_TEST_TOKEN="your_token_here"

# Enable general integration tests (read-only operations)
export IQOPTION_LIVE_TEST=1

# (Optional) Enable demo trading tests
# WARNING: Only use this if you understand the risks.
export IQOPTION_LIVE_TRADE=1

# Run the tests
go test -v ./...
```

**Note:** 
- `IQOPTION_LIVE_TEST` enables tests for listing balances, assets, candles, and history.
- `IQOPTION_LIVE_TRADE` is required specifically for tests that place orders. These tests are designed to use **Practice (TRAINING)** balances.
- Integration tests will be skipped if `IQOPTION_LIVE_TEST` is not set to `1`.

## Advanced Usage

### Customizing the HTTP Client
You can provide your own `http.Client` for custom timeouts, proxy settings, or observability:

```go
client, _ := iqclient.New(iqclient.Config{
    Token: "...",
    HTTPClient: &http.Client{
        Timeout: 30 * time.Second,
    },
})
```

### Finding Expirations
The library includes helpers to find valid expirations provided by the server, such as `FindM15Expiration`.

## Contributing

Contributions are welcome! Feel free to open issues or submit pull requests to improve the client.

## Keywords

`IQ Option API`, `Binary Options`, `Trading Bot`, `Golang`, `Go`, `MCP`, `Model Context Protocol`, `Algorithmic Trading`, `Fintech`, `Opções Binárias`, `Automação de Trades`, `Estratégia de Trading`.

---

*Disclaimer: Trading involves risk. Use this software at your own risk. The authors are not responsible for any financial losses incurred.*
