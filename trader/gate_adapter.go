package trader

import (
	"fmt"
	"strconv"
	"time"

	"github.com/antihax/optional"
	"github.com/gateio/gateapi-go/v6"
)

// GateAdapter adapts GateTrader to implement nofx/trader.Trader interface
type GateAdapter struct {
	*GateTrader
}

// NewGateAdapter creates a new Gate adapter instance
func NewGateAdapter(apiKey, secretKey string) *GateAdapter {
	return &GateAdapter{
		GateTrader: NewGateTrader(apiKey, secretKey),
	}
}

// GetOrderStatus gets the status of an order (adapts int64 orderID to string)
func (a *GateAdapter) GetOrderStatus(symbol string, orderID int64) (map[string]interface{}, error) {
	orderIDStr := fmt.Sprintf("%d", orderID)
	return a.GateTrader.GetOrderStatus(symbol, orderIDStr)
}

// GetOrderHistory gets order history (stub implementation for Gate)
func (a *GateAdapter) GetOrderHistory(symbol string, limit int) ([]map[string]interface{}, error) {
	// Gate uses different API for trade history
	// Return empty for now
	return []map[string]interface{}{}, nil
}

// GetTradeHistory gets trade history (stub implementation for Gate)
func (a *GateAdapter) GetTradeHistory(symbol string, limit int) ([]map[string]interface{}, error) {
	// Implemented in GetRecentTrades
	return []map[string]interface{}{}, nil
}

// ===== Authoritative data query methods =====

// GetOrderTrades gets trade details for a specific order
func (a *GateAdapter) GetOrderTrades(symbol string, orderID int64) ([]OrderTradeDetail, error) {
	symbol = a.convertSymbol(symbol)
	orderIDStr := fmt.Sprintf("%d", orderID)

	// Gate doesn't have a direct order trades endpoint
	// Use GetMyTrades and filter by order
	opts := &gateapi.GetMyTradesOpts{
		Order: optional.NewInt64(orderID),
	}

	trades, _, err := a.client.FuturesApi.GetMyTrades(a.ctx, "usdt", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get order trades: %w", err)
	}

	result := make([]OrderTradeDetail, 0, len(trades))
	for _, trade := range trades {
		if trade.OrderId != orderIDStr {
			continue
		}

		price, _ := strconv.ParseFloat(trade.Price, 64)
		fee, _ := strconv.ParseFloat(trade.Fee, 64)
		if fee < 0 {
			fee = -fee
		}

		// Get quanto_multiplier
		quantoMultiplier := 1.0
		contract, err := a.getContract(trade.Contract)
		if err == nil && contract != nil {
			qm, _ := strconv.ParseFloat(contract.QuantoMultiplier, 64)
			if qm > 0 {
				quantoMultiplier = qm
			}
		}

		absSize := trade.Size
		if absSize < 0 {
			absSize = -absSize
		}
		quantity := float64(absSize) * quantoMultiplier

		side := "BUY"
		if trade.Size < 0 {
			side = "SELL"
		}

		result = append(result, OrderTradeDetail{
			TradeID:         trade.Id,
			OrderID:         orderID,
			Symbol:          a.revertSymbol(trade.Contract),
			Side:            side,
			PositionSide:    "BOTH",
			Price:           price,
			Quantity:        quantity,
			QuoteQty:        price * quantity,
			Commission:      fee,
			CommissionAsset: "USDT",
			RealizedPnL:     0, // Gate doesn't provide this in trade record
			IsMaker:         trade.Role == "maker",
			TradeTime:       time.Unix(int64(trade.CreateTime), 0),
		})
	}

	return result, nil
}

// GetRecentTrades gets recent trade history
func (a *GateAdapter) GetRecentTrades(symbol string, limit int) ([]OrderTradeDetail, error) {
	symbol = a.convertSymbol(symbol)

	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}

	opts := &gateapi.GetMyTradesOpts{
		Limit:    optional.NewInt32(int32(limit)),
		Contract: optional.NewString(symbol),
	}

	trades, _, err := a.client.FuturesApi.GetMyTrades(a.ctx, "usdt", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent trades: %w", err)
	}

	result := make([]OrderTradeDetail, 0, len(trades))
	for _, trade := range trades {
		price, _ := strconv.ParseFloat(trade.Price, 64)
		fee, _ := strconv.ParseFloat(trade.Fee, 64)
		if fee < 0 {
			fee = -fee
		}

		// Get quanto_multiplier
		quantoMultiplier := 1.0
		contract, err := a.getContract(trade.Contract)
		if err == nil && contract != nil {
			qm, _ := strconv.ParseFloat(contract.QuantoMultiplier, 64)
			if qm > 0 {
				quantoMultiplier = qm
			}
		}

		absSize := trade.Size
		if absSize < 0 {
			absSize = -absSize
		}
		quantity := float64(absSize) * quantoMultiplier

		side := "BUY"
		if trade.Size < 0 {
			side = "SELL"
		}

		orderID, _ := strconv.ParseInt(trade.OrderId, 10, 64)

		result = append(result, OrderTradeDetail{
			TradeID:         trade.Id,
			OrderID:         orderID,
			Symbol:          a.revertSymbol(trade.Contract),
			Side:            side,
			PositionSide:    "BOTH",
			Price:           price,
			Quantity:        quantity,
			QuoteQty:        price * quantity,
			Commission:      fee,
			CommissionAsset: "USDT",
			RealizedPnL:     0,
			IsMaker:         trade.Role == "maker",
			TradeTime:       time.Unix(int64(trade.CreateTime), 0),
		})
	}

	return result, nil
}

// GetIncomeHistory gets income history (funding fees, realized PnL, etc.)
func (a *GateAdapter) GetIncomeHistory(symbol string, incomeType string, limit int) ([]IncomeRecord, error) {
	// Gate uses ListPositionClose for closed position PnL
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}

	symbol = a.convertSymbol(symbol)

	opts := &gateapi.ListPositionCloseOpts{
		Limit:    optional.NewInt32(int32(limit)),
		Contract: optional.NewString(symbol),
	}

	closedPositions, _, err := a.client.FuturesApi.ListPositionClose(a.ctx, "usdt", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get income history: %w", err)
	}

	result := make([]IncomeRecord, 0, len(closedPositions))
	for _, pos := range closedPositions {
		pnl, _ := strconv.ParseFloat(pos.Pnl, 64)

		result = append(result, IncomeRecord{
			Symbol:     a.revertSymbol(pos.Contract),
			IncomeType: "REALIZED_PNL",
			Income:     pnl,
			Asset:      "USDT",
			Info:       fmt.Sprintf("Side: %s", pos.Side),
			Time:       time.Unix(int64(pos.Time), 0),
			TranID:     int64(pos.Time),
			TradeID:    "",
		})
	}

	return result, nil
}

// Ensure GateAdapter implements Trader interface
var _ Trader = (*GateAdapter)(nil)
