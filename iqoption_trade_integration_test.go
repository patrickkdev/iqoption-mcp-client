package iqclient

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLive_DemoPlaceTrade(t *testing.T) {
	if !liveTestsEnabled() {
		t.Skip(
			"live IQ Option tests disabled; " +
				"set IQOPTION_LIVE_TEST=1",
		)
	}

	if !envEnabled(liveTradeEnv) {
		t.Skip(
			"demo trade test disabled; " +
				"set IQOPTION_LIVE_TRADE=1",
		)
	}

	client := liveClient(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		45*time.Second,
	)
	defer cancel()

	// Explicitly request TRAINING only.
	balances, err := client.ListBalances(
		ctx,
		BalanceTypeTraining,
	)
	if err != nil {
		t.Fatalf(
			"ListBalances(TRAINING): %v",
			err,
		)
	}

	var training *AccountBalance

	for i := range balances {
		if strings.EqualFold(
			balances[i].Type,
			"training",
		) {
			training = &balances[i]
			break
		}
	}

	if training == nil {
		t.Fatal("no training balance available")
	}

	if training.BalanceID <= 0 {
		t.Fatalf(
			"invalid training balance ID: %d",
			training.BalanceID,
		)
	}

	assets, err := client.ListAssets(ctx, true)
	if err != nil {
		t.Fatalf(
			"ListAssets(): %v",
			err,
		)
	}

	var asset *Asset

	for i := range assets {
		if assets[i].IsOpen &&
			assets[i].ProfitPercent > 0 &&
			len(assets[i].Expirations) > 0 &&
			assets[i].MinimumAmount > 0 {

			asset = &assets[i]
			break
		}
	}

	if asset == nil {
		t.Skip(
			"no suitable open asset with an expiration " +
				"and positive profit was available",
		)
	}

	// IMPORTANT:
	//
	// We select the expiration from the server-provided list.
	// We do not manufacture an expiration timestamp.
	expiration, ok := FindM15Expiration(
		*asset,
		time.Now().UTC(),
	)
	if !ok {
		t.Skip(
			"no server-provided M15 expiration currently available",
		)
	}

	amount := asset.MinimumAmount

	if amount <= 0 {
		t.Fatalf(
			"server returned invalid minimum amount: %v",
			amount,
		)
	}

	t.Logf(
		"placing DEMO trade: balance=%d asset=%s amount=%.2f expiration=%s",
		training.BalanceID,
		asset.Name,
		amount,
		expiration.Format(time.RFC3339),
	)

	// This is intentionally a deterministic test direction rather than
	// pretending the test is evaluating a trading strategy.
	positionID, err := client.PlaceTrade(
		ctx,
		TradeRequest{
			BalanceID:     training.BalanceID,
			AssetID:       asset.ID,
			Direction:     "call",
			Amount:        amount,
			ProfitPercent: asset.ProfitPercent,
			Expired:       expiration,
		},
	)
	if err != nil {
		t.Fatalf(
			"PlaceTrade(DEMO) failed: %v",
			err,
		)
	}

	if positionID <= 0 {
		t.Fatalf(
			"PlaceTrade(DEMO) returned invalid position ID: %d",
			positionID,
		)
	}

	t.Logf(
		"DEMO trade accepted: position_id=%d",
		positionID,
	)

	// Verify that the newly-created position can actually be found.
	//
	// We do NOT immediately assume that a successful HTTP response means
	// everything is visible in the positions endpoint. Give the backend a
	// little time to make it observable.
	var found bool

	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		positions, err := client.ListPositions(
			ctx,
			training.BalanceID,
		)
		if err != nil {
			t.Fatalf(
				"ListPositions() after trade: %v",
				err,
			)
		}

		for _, position := range positions {
			if position.ID == positionID {
				found = true

				if position.AssetID != asset.ID {
					t.Fatalf(
						"position %d asset mismatch: got %d want %d",
						position.ID,
						position.AssetID,
						asset.ID,
					)
				}

				if position.Direction != "call" {
					t.Fatalf(
						"position %d direction mismatch: got %q",
						position.ID,
						position.Direction,
					)
				}

				t.Logf(
					"verified DEMO position: id=%d status=%s expiration=%s remaining=%ds",
					position.ID,
					position.Status,
					position.Expiration.Format(time.RFC3339),
					position.SecondsRemaining,
				)

				break
			}
		}

		if found {
			break
		}

		select {
		case <-ctx.Done():
			t.Fatalf(
				"timed out waiting for position %d: %v",
				positionID,
				ctx.Err(),
			)
		case <-time.After(500 * time.Millisecond):
		}
	}

	if !found {
		t.Fatalf(
			"PlaceTrade returned position_id=%d, but position "+
				"was not visible through ListPositions",
			positionID,
		)
	}
}

func liveTestsEnabled() bool {
	return envEnabled(liveTestEnv)
}

func envEnabled(name string) bool {
	return strings.TrimSpace(
		os.Getenv(name),
	) == "1"
}
