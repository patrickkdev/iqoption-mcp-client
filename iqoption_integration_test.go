package iqclient

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	liveTestTokenEnv = "IQOPTION_TEST_TOKEN"

	// Set IQOPTION_LIVE_TEST=1 to enable tests that actually hit IQ Option.
	liveTestEnv = "IQOPTION_LIVE_TEST"

	// Set IQOPTION_LIVE_TRADE=1 to allow the demo trade test.
	//
	// This is intentionally separate from IQOPTION_LIVE_TEST because a test
	// that merely reads account data should not be able to place an order.
	liveTradeEnv = "IQOPTION_LIVE_TRADE"
)

func liveClient(t *testing.T) *Client {
	t.Helper()

	if os.Getenv(liveTestEnv) != "1" {
		t.Skip(
			"live IQ Option tests disabled; " +
				"set IQOPTION_LIVE_TEST=1",
		)
	}

	token := strings.TrimSpace(
		os.Getenv(liveTestTokenEnv),
	)

	if token == "" {
		t.Skip(
			"IQOPTION_TEST_TOKEN is not configured",
		)
	}

	client, err := New(Config{
		Token: token,

		// Keep the integration tests deliberately conservative.
		MaxRetries:     2,
		RetryBaseDelay: 500 * time.Millisecond,

		ClientName:    "iqoption-mcp-client-integration-test",
		ClientVersion: "test",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
	})

	return client
}

// func TestLive_DebugTools(t *testing.T) {
// 	client := liveClient(t)

// 	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
// 	defer cancel()

// 	tools, err := client.ListTools(ctx)
// 	if err != nil {
// 		t.Fatalf("list tools: %v", err)
// 	}

// 	data, err := json.MarshalIndent(tools, "", "  ")
// 	if err != nil {
// 		t.Fatalf("marshal tools: %v", err)
// 	}

// 	fmt.Println("\n========== MCP TOOLS ==========")
// 	fmt.Println(string(data))
// 	fmt.Println("========== END MCP TOOLS ==========")
// }

func TestLive_InitializeAndCapabilities(t *testing.T) {
	client := liveClient(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	capabilities, err := client.GetCapabilities(ctx)
	if err != nil {
		t.Fatalf(
			"GetCapabilities() failed: %v",
			err,
		)
	}

	if capabilities == nil {
		t.Fatal("GetCapabilities() returned nil capabilities")
	}

	t.Logf(
		"connected successfully; capabilities=%d fields",
		len(capabilities),
	)
}

func TestLive_ListBalances(t *testing.T) {
	client := liveClient(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	balances, err := client.ListBalances(
		ctx,
		BalanceTypeTraining,
	)
	if err != nil {
		t.Fatalf(
			"ListBalances(TRAINING) failed: %v",
			err,
		)
	}

	if len(balances) == 0 {
		t.Fatal(
			"expected at least one training balance",
		)
	}

	var foundTraining bool

	for _, balance := range balances {
		t.Logf(
			"balance id=%d type=%s currency=%s amount=%.2f",
			balance.BalanceID,
			balance.Type,
			balance.Currency,
			balance.Amount,
		)

		if strings.EqualFold(
			balance.Type,
			"training",
		) {
			foundTraining = true

			if balance.BalanceID <= 0 {
				t.Errorf(
					"training balance has invalid id: %d",
					balance.BalanceID,
				)
			}

			if balance.Currency == "" {
				t.Error(
					"training balance has empty currency",
				)
			}
		}
	}

	if !foundTraining {
		t.Fatal(
			"server returned balances but no training balance",
		)
	}
}

func TestLive_ListAssets(t *testing.T) {
	client := liveClient(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	assets, err := client.ListAssets(ctx, true)
	if err != nil {
		t.Fatalf(
			"ListAssets(true) failed: %v",
			err,
		)
	}

	if len(assets) == 0 {
		t.Fatal("expected at least one enabled asset")
	}

	for _, asset := range assets {
		if asset.ID <= 0 {
			t.Errorf(
				"asset %q has invalid ID: %d",
				asset.Name,
				asset.ID,
			)
		}

		if asset.Name == "" {
			t.Errorf(
				"asset %d has empty name",
				asset.ID,
			)
		}

		if !asset.IsOpen {
			t.Errorf(
				"ListAssets(true) returned closed asset %q",
				asset.Name,
			)
		}

		t.Logf(
			"asset id=%d name=%s open=%t profit=%.2f expirations=%d",
			asset.ID,
			asset.Name,
			asset.IsOpen,
			asset.ProfitPercent,
			len(asset.Expirations),
		)

		for _, expiration := range asset.Expirations {
			if expiration.IsZero() {
				t.Errorf(
					"asset %q contains zero expiration",
					asset.Name,
				)
			}
		}
	}
}

func TestLive_GetCandlesM15(t *testing.T) {
	client := liveClient(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		45*time.Second,
	)
	defer cancel()

	assets, err := client.ListAssets(ctx, true)
	if err != nil {
		t.Fatalf(
			"ListAssets() failed: %v",
			err,
		)
	}

	if len(assets) == 0 {
		t.Fatal("no enabled assets returned")
	}

	// Pick the first currently-open asset. We don't hard-code EURUSD because
	// the server's available asset list can change.
	asset := assets[0]

	candles, err := client.GetCandles(
		ctx,
		asset.ID,
		900, // M15
		20,
	)
	if err != nil {
		t.Fatalf(
			"GetCandles(asset=%d, size=900) failed: %v",
			asset.ID,
			err,
		)
	}

	if len(candles) == 0 {
		t.Fatal("expected M15 candles")
	}

	for i, candle := range candles {
		if candle.From.IsZero() {
			t.Errorf(
				"candle %d has zero From timestamp",
				i,
			)
		}

		if candle.To.IsZero() {
			t.Errorf(
				"candle %d has zero To timestamp",
				i,
			)
		}

		if !candle.To.After(candle.From) {
			t.Errorf(
				"candle %d has invalid interval: %v -> %v",
				i,
				candle.From,
				candle.To,
			)
		}

		if candle.Open <= 0 ||
			candle.Close <= 0 ||
			candle.High <= 0 ||
			candle.Low <= 0 {
			t.Errorf(
				"candle %d has invalid OHLC: %+v",
				i,
				candle,
			)
		}

		if candle.High < candle.Low {
			t.Errorf(
				"candle %d has high < low: %+v",
				i,
				candle,
			)
		}

		t.Logf(
			"candle %d from=%s open=%.8f high=%.8f low=%.8f close=%.8f",
			i,
			candle.From.Format(time.RFC3339),
			candle.Open,
			candle.High,
			candle.Low,
			candle.Close,
		)
	}
}

func TestLive_ListPositions(t *testing.T) {
	client := liveClient(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	balances, err := client.ListBalances(
		ctx,
		BalanceTypeTraining,
	)
	if err != nil {
		t.Fatalf(
			"ListBalances() failed: %v",
			err,
		)
	}

	if len(balances) == 0 {
		t.Fatal("no training balances available")
	}

	positions, err := client.ListPositions(
		ctx,
		balances[0].BalanceID,
	)
	if err != nil {
		t.Fatalf(
			"ListPositions() failed: %v",
			err,
		)
	}

	for _, position := range positions {
		if position.PositionID <= 0 {
			t.Errorf(
				"position has invalid ID: %d",
				position.PositionID,
			)
		}

		if position.AssetID <= 0 {
			t.Errorf(
				"position %d has invalid asset ID",
				position.PositionID,
			)
		}

		t.Logf(
			"position id=%d asset=%s direction=%s amount=%.2f remaining=%ds",
			position.PositionID,
			position.AssetName,
			position.Direction,
			position.Amount,
			position.SecondsRemaining,
		)
	}
}

func TestLive_TradeHistory(t *testing.T) {
	client := liveClient(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	trades, err := client.ListTradeHistory(
		ctx,
		0,
		20,
	)
	if err != nil {
		t.Fatalf(
			"ListTradeHistory() failed: %v",
			err,
		)
	}

	for _, trade := range trades {
		if trade.PositionID <= 0 {
			t.Errorf(
				"trade has invalid position ID: %d",
				trade.PositionID,
			)
		}

		if trade.AssetID <= 0 {
			t.Errorf(
				"trade %d has invalid asset ID",
				trade.PositionID,
			)
		}

		t.Logf(
			"trade position=%d asset=%s direction=%s amount=%.2f profit=%.2f result=%s",
			trade.PositionID,
			trade.AssetName,
			trade.Direction,
			trade.Amount,
			trade.Profit,
			trade.Result,
		)
	}
}
