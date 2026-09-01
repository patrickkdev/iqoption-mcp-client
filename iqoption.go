package iqclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ListBalances returns IQ Option balances.
func (c *Client) ListBalances(
	ctx context.Context,
	types BalanceType,
) ([]AccountBalance, error) {
	result, err := c.callTool(
		ctx,
		"list_balances",
		map[string]any{
			"types": string(types),
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list balances: %w",
			err,
		)
	}

	var response struct {
		Balances []AccountBalance `json:"balances"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf(
			"decode balances: %w",
			err,
		)
	}

	return response.Balances, nil
}

// ListAssets returns currently available assets.
//
// onlyEnabled should normally be true for trading applications.
func (c *Client) ListAssets(
	ctx context.Context,
	onlyEnabled bool,
) ([]Asset, error) {
	result, err := c.callTool(
		ctx,
		"list_assets",
		map[string]any{
			"only_enabled": onlyEnabled,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}

	var payload struct {
		Assets []assetRow `json:"assets"`
	}

	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, fmt.Errorf("decode list_assets: %w", err)
	}

	assets := make([]Asset, 0, len(payload.Assets))

	for _, row := range payload.Assets {
		asset := Asset{
			ID:                     row.AssetID,
			Name:                   row.Name,
			IsOpen:                 row.IsOpen,
			Precision:              row.Precision,
			ProfitPercent:          row.ProfitPercent,
			MinimumAmount:          row.MinimumAmount,
			MaximumAmount:          row.MaximumAmount,
			DeadtimeSeconds:        row.DeadtimeSeconds,
			BuybackEnabled:         row.BuybackEnabled,
			BuybackDeadtimeSeconds: row.BuybackDeadtimeSeconds,
		}

		for _, unix := range row.Expirations {
			asset.Expirations = append(
				asset.Expirations,
				time.Unix(unix, 0).UTC(),
			)
		}

		assets = append(assets, asset)
	}

	return assets, nil
}

// GetCandles retrieves OHLC candles.
//
// size must be one of the sizes accepted by the IQ Option MCP server.
// For M15 use size=900.
func (c *Client) GetCandles(
	ctx context.Context,
	assetID int64,
	size int,
	count int,
) ([]Candle, error) {
	if assetID <= 0 {
		return nil, errors.New("get candles: assetID must be positive")
	}

	if count <= 0 {
		return nil, errors.New("get candles: count must be positive")
	}

	if count > 1000 {
		count = 1000
	}

	result, err := c.callTool(
		ctx,
		"get_candles",
		map[string]any{
			"asset_id": assetID,
			"size":     size,
			"count":    count,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("get candles: %w", err)
	}

	var payload struct {
		Candles []candleRow `json:"candles"`
	}

	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, fmt.Errorf("decode get_candles: %w", err)
	}

	candles := make([]Candle, 0, len(payload.Candles))

	for _, row := range payload.Candles {
		candles = append(candles, Candle{
			From:   row.From.UTC(),
			To:     row.To.UTC(),
			Open:   row.Open,
			Close:  row.Close,
			High:   row.High,
			Low:    row.Low,
			Volume: row.Volume,
		})
	}

	return candles, nil
}

// ListPositions returns currently open positions for a balance.
func (c *Client) ListPositions(
	ctx context.Context,
	balanceID int64,
) ([]Position, error) {
	if balanceID <= 0 {
		return nil, errors.New("list positions: balanceID must be positive")
	}

	result, err := c.callTool(
		ctx,
		"list_positions",
		map[string]any{
			"balance_id": balanceID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}

	var payload struct {
		Positions []positionRow `json:"positions"`
	}

	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, fmt.Errorf("decode list_positions: %w", err)
	}

	positions := make([]Position, 0, len(payload.Positions))

	for _, row := range payload.Positions {
		positions = append(positions, Position{
			PositionID:       row.PositionID,
			AssetID:          row.AssetID,
			AssetName:        row.AssetName,
			Status:           row.Status,
			Direction:        row.Direction,
			Amount:           row.Amount,
			OpenPrice:        row.OpenPrice,
			CurrentPrice:     row.CurrentPrice,
			ExpectedProfit:   row.ExpectedProfit,
			SellProfit:       row.SellProfit,
			OpenTime:         row.OpenTime.UTC(),
			Expiration:       row.Expiration.UTC(),
			SecondsRemaining: row.SecondsRemaining,
		})
	}

	return positions, nil
}

// ListTradeHistory returns completed trades.
func (c *Client) ListTradeHistory(
	ctx context.Context,
	skip int,
	limit int,
) ([]Trade, error) {
	if skip < 0 {
		skip = 0
	}

	if limit <= 0 {
		limit = 100
	}

	result, err := c.callTool(
		ctx,
		"get_trade_history",
		map[string]any{
			"skip":  skip,
			"limit": limit,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("get trade history: %w", err)
	}

	var payload struct {
		History []tradeRow `json:"history"`
	}

	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, fmt.Errorf("decode get_trade_history: %w", err)
	}

	trades := make([]Trade, 0, len(payload.History))

	for _, row := range payload.History {
		trades = append(trades, Trade{
			PositionID:  row.PositionID,
			AssetID:     row.AssetID,
			AssetName:   row.AssetName,
			Direction:   row.Direction,
			Amount:      row.Amount,
			OpenPrice:   row.OpenPrice,
			ClosePrice:  row.ClosePrice,
			Profit:      row.Profit,
			OpenTime:    row.OpenTime.UTC(),
			CloseTime:   row.CloseTime.UTC(),
			CloseReason: row.CloseReason,
			Result:      row.Result,
		})
	}

	return trades, nil
}

// PlaceTrade submits a trade.
//
// IMPORTANT:
//
// This method intentionally does not retry.
//
// A timeout after the server accepted the trade is ambiguous. Retrying could
// create a duplicate trade.
//
// For REAL/regular balances, the upstream MCP server requires explicit user
// confirmation before the write. This package does not bypass that requirement.
func (c *Client) PlaceTrade(
	ctx context.Context,
	req TradeRequest,
) (int64, error) {
	if req.BalanceID <= 0 {
		return 0, errors.New("place trade: balanceID must be positive")
	}

	if req.AssetID <= 0 {
		return 0, errors.New("place trade: assetID must be positive")
	}

	direction := strings.ToLower(strings.TrimSpace(req.Direction))

	if direction != "call" && direction != "put" {
		return 0, fmt.Errorf(
			"place trade: invalid direction %q",
			req.Direction,
		)
	}

	if req.Amount <= 0 {
		return 0, errors.New("place trade: amount must be positive")
	}

	if req.ProfitPercent <= 0 {
		return 0, errors.New(
			"place trade: profit percent must be positive",
		)
	}

	if req.Expired.IsZero() {
		return 0, errors.New(
			"place trade: expiration is required",
		)
	}

	result, err := c.postToolCallNoRetry(
		ctx,
		"place_trade",
		map[string]any{
			"balance_id":     req.BalanceID,
			"asset_id":       req.AssetID,
			"direction":      direction,
			"amount":         req.Amount,
			"profit_percent": req.ProfitPercent,
			"expired":        req.Expired.Unix(),
		},
	)
	if err != nil {
		return 0, fmt.Errorf("place trade: %w", err)
	}

	var response struct {
		PositionID int64 `json:"position_id"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return 0, fmt.Errorf("decode trade result: %w", err)
	}

	if response.PositionID <= 0 {
		return 0, errors.New(
			"place trade: server returned invalid position_id",
		)
	}

	return response.PositionID, nil
}

// GetCapabilities retrieves server capabilities.
func (c *Client) GetCapabilities(
	ctx context.Context,
) (map[string]any, error) {
	result, err := c.callTool(
		ctx,
		"get_capabilities",
		map[string]any{},
	)
	if err != nil {
		return nil, fmt.Errorf("get capabilities: %w", err)
	}

	var capabilities map[string]any

	if err := json.Unmarshal(result, &capabilities); err != nil {
		return nil, fmt.Errorf(
			"decode capabilities: %w",
			err,
		)
	}

	return capabilities, nil
}

// GetLimits retrieves server limits.
func (c *Client) GetLimits(
	ctx context.Context,
) (map[string]any, error) {
	result, err := c.callTool(
		ctx,
		"get_limits",
		map[string]any{},
	)
	if err != nil {
		return nil, fmt.Errorf("get limits: %w", err)
	}

	var limits map[string]any

	if err := json.Unmarshal(result, &limits); err != nil {
		return nil, fmt.Errorf("decode limits: %w", err)
	}

	return limits, nil
}

func (c *Client) ListTools(ctx context.Context) ([]byte, error) {
	result, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) WaitForTradeResult(
	ctx context.Context,
	balanceID int64,
	positionID int64,
	pollInterval time.Duration,
) (*Trade, error) {
	if balanceID <= 0 {
		return nil, fmt.Errorf("balance ID must be positive")
	}

	if positionID <= 0 {
		return nil, fmt.Errorf("position ID must be positive")
	}

	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Check immediately instead of waiting for the first ticker tick.
	open, err := c.positionExists(ctx, balanceID, positionID)
	if err != nil {
		return nil, fmt.Errorf("check trade position: %w", err)
	}

	if !open {
		return c.waitForTradeHistory(ctx, positionID, pollInterval)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf(
				"waiting for trade %d: %w",
				positionID,
				ctx.Err(),
			)

		case <-ticker.C:
			open, err := c.positionExists(ctx, balanceID, positionID)
			if err != nil {
				return nil, fmt.Errorf(
					"check trade position %d: %w",
					positionID,
					err,
				)
			}

			if !open {
				return c.waitForTradeHistory(
					ctx,
					positionID,
					pollInterval,
				)
			}
		}
	}
}

func (c *Client) positionExists(
	ctx context.Context,
	balanceID int64,
	positionID int64,
) (bool, error) {
	positions, err := c.ListPositions(ctx, balanceID)
	if err != nil {
		return false, err
	}

	for _, position := range positions {
		if position.PositionID == positionID {
			return true, nil
		}
	}

	return false, nil
}

func (c *Client) waitForTradeHistory(
	ctx context.Context,
	positionID int64,
	pollInterval time.Duration,
) (*Trade, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Check immediately because the history may already have been updated.
	trade, found, err := c.findTradeInHistory(ctx, positionID)
	if err != nil {
		return nil, fmt.Errorf(
			"check trade history %d: %w",
			positionID,
			err,
		)
	}

	if found {
		return trade, nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf(
				"waiting for completed trade %d: %w",
				positionID,
				ctx.Err(),
			)

		case <-ticker.C:
			trade, found, err := c.findTradeInHistory(ctx, positionID)
			if err != nil {
				return nil, fmt.Errorf(
					"check trade history %d: %w",
					positionID,
					err,
				)
			}

			if found {
				return trade, nil
			}
		}
	}
}

func (c *Client) findTradeInHistory(
	ctx context.Context,
	positionID int64,
) (*Trade, bool, error) {
	// 100 should be plenty for finding a just-completed trade.
	history, err := c.ListTradeHistory(ctx, 0, 100)
	if err != nil {
		return nil, false, err
	}

	for i := range history {
		if history[i].PositionID == positionID {
			return &history[i], true, nil
		}
	}

	return nil, false, nil
}

// FindM15Expiration returns the server-provided expiration closest to 15
// minutes from now.
//
// It NEVER calculates an expiration timestamp itself.
//
// This is important because the MCP server owns the valid expiration values.
func FindM15Expiration(
	asset Asset,
	now time.Time,
) (time.Time, bool) {
	const target = 15 * time.Minute

	now = now.UTC()

	var (
		best      time.Time
		bestDelta time.Duration
		found     bool
	)

	for _, expiration := range asset.Expirations {
		expiration = expiration.UTC()

		remaining := expiration.Sub(now)

		// Reject expirations that are clearly not M15.
		//
		// The exact expiration must come from the server. We only choose
		// from the server-provided list.
		if remaining < 13*time.Minute ||
			remaining > 17*time.Minute {
			continue
		}

		delta := absDuration(remaining - target)

		if !found || delta < bestDelta {
			found = true
			best = expiration
			bestDelta = delta
		}
	}

	return best, found
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}

	return value
}

// postToolCallNoRetry is specifically for operations where duplicate execution
// would be dangerous.
func (c *Client) postToolCallNoRetry(
	ctx context.Context,
	tool string,
	arguments map[string]any,
) ([]byte, error) {
	if err := c.initialize(ctx); err != nil {
		return nil, err
	}

	result, err := c.callWithoutRetry(
		ctx,
		"tools/call",
		ToolCallParams{
			Name:      tool,
			Arguments: arguments,
		},
	)
	if err != nil {
		return nil, err
	}

	return decodeMCPToolResult(result)
}

func (c *Client) callWithoutRetry(
	ctx context.Context,
	method string,
	params any,
) ([]byte, error) {
	if c.IsClosed() {
		return nil, ErrClosed
	}

	if err := c.initialize(ctx); err != nil {
		return nil, err
	}

	requestID := c.nextRequestID()

	request := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	result, _, err := c.doHTTP(ctx, body)
	if err != nil {
		return nil, err
	}

	return result, nil
}

type assetRow struct {
	AssetID                int64   `json:"asset_id"`
	Name                   string  `json:"name"`
	IsOpen                 bool    `json:"is_open"`
	Precision              int     `json:"precision"`
	ProfitPercent          float64 `json:"profit_percent"`
	Expirations            []int64 `json:"expirations"`
	MinimumAmount          float64 `json:"minimum_amount"`
	MaximumAmount          float64 `json:"maximum_amount"`
	DeadtimeSeconds        int     `json:"deadtime_seconds"`
	BuybackEnabled         bool    `json:"buyback_enabled"`
	BuybackDeadtimeSeconds int     `json:"buyback_deadtime_seconds"`
}

type candleRow struct {
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Open   float64   `json:"open"`
	Close  float64   `json:"close"`
	High   float64   `json:"max"`
	Low    float64   `json:"min"`
	Volume float64   `json:"volume"`
}

type positionRow struct {
	PositionID       int64     `json:"position_id"`
	AssetID          int64     `json:"asset_id"`
	AssetName        string    `json:"asset_name"`
	Status           string    `json:"status"`
	Direction        string    `json:"direction"`
	Amount           float64   `json:"amount"`
	OpenPrice        float64   `json:"open_price"`
	CurrentPrice     float64   `json:"current_price"`
	ExpectedProfit   float64   `json:"expected_profit"`
	SellProfit       float64   `json:"sell_profit"`
	OpenTime         time.Time `json:"open_time"`
	Expiration       time.Time `json:"expiration"`
	SecondsRemaining int       `json:"seconds_remaining"`
}

type tradeRow struct {
	PositionID  int64     `json:"position_id"`
	AssetID     int64     `json:"asset_id"`
	AssetName   string    `json:"asset_name"`
	Direction   string    `json:"direction"`
	Amount      float64   `json:"amount"`
	OpenPrice   float64   `json:"open_price"`
	ClosePrice  float64   `json:"close_price"`
	Profit      float64   `json:"profit"`
	OpenTime    time.Time `json:"open_time"`
	CloseTime   time.Time `json:"close_time"`
	CloseReason string    `json:"close_reason"`
	Result      string    `json:"result"`
}
