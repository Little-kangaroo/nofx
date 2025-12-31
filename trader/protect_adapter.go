package trader

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"nofx/config"
	"nofx/internal/protect"
)

// ========== PositionStore Adapter ==========

// positionStoreAdapter 适配器：将database+trader ID转换为protect.PositionStore
type positionStoreAdapter struct {
	db       *config.Database
	traderID string
}

func newPositionStoreAdapter(db *config.Database, traderID string) *positionStoreAdapter {
	return &positionStoreAdapter{
		db:       db,
		traderID: traderID,
	}
}

// ListOpenPositions 实现protect.PositionStore接口
func (a *positionStoreAdapter) ListOpenPositions(ctx context.Context) ([]protect.PositionState, error) {
	// 从database获取该trader的所有open trades
	trades, err := a.db.GetOpenTradesForTrader(a.traderID)
	if err != nil {
		return nil, err
	}

	var positions []protect.PositionState
	for _, trade := range trades {
		// 转换Side
		side := protect.Long
		if trade.Side == "short" {
			side = protect.Short
		}

		// 获取初始止损价（从trade record或metadata）
		initStop := trade.InitialStopPrice // 假设已添加此字段
		if initStop == 0 {
			// Fallback: 如果没有存储，使用当前止损作为估算
			log.Printf("⚠️  [ProfitLocking] %s %s: InitialStopPrice缺失，使用fallback",
				trade.Symbol, trade.Side)
			initStop = trade.CurrentStopPrice
		}

		pos := protect.PositionState{
			Symbol:               trade.Symbol,
			Side:                 side,
			Qty:                  trade.Quantity,
			Entry:                trade.OpenPrice,
			Leverage:             float64(trade.Leverage),
			InitStop:             initStop,
			PrevStop:             trade.CurrentStopPrice,
			OpenTimeMs:           trade.OpenTime.UnixMilli(),
			LastStopUpdateTimeMs: trade.LastStopUpdateTime.UnixMilli(),
			StopTriggerType:      protect.TriggerLast, // 默认使用LAST_PRICE
			ROIArmed:             trade.ROIArmed,
			BreakEvenArmed:       trade.BreakEvenArmed,
			RLockStage:           trade.RLockStage,
		}

		positions = append(positions, pos)
	}

	return positions, nil
}

// SavePositionState 实现protect.PositionStore接口
func (a *positionStoreAdapter) SavePositionState(ctx context.Context, pos protect.PositionState) error {
	sideStr := "long"
	if pos.Side == protect.Short {
		sideStr = "short"
	}

	// 更新数据库中的持仓状态
	return a.db.UpdatePositionProtectState(a.traderID, pos.Symbol, sideStr, &config.PositionProtectState{
		PrevStop:             pos.PrevStop,
		LastStopUpdateTimeMs: pos.LastStopUpdateTimeMs,
		ROIArmed:             pos.ROIArmed,
		BreakEvenArmed:       pos.BreakEvenArmed,
		RLockStage:           pos.RLockStage,
	})
}

// ========== StopExecutor Adapter ==========

// stopExecutorAdapter 适配器：将Trader接口转换为protect.StopExecutor
type stopExecutorAdapter struct {
	trader Trader
}

func newStopExecutorAdapter(trader Trader) *stopExecutorAdapter {
	return &stopExecutorAdapter{
		trader: trader,
	}
}

// UpsertStop 实现protect.StopExecutor接口
func (a *stopExecutorAdapter) UpsertStop(ctx context.Context, symbol string, side protect.Side, qty float64, stopPrice float64, trigger protect.TriggerType) error {
	// 转换Side
	sideStr := "LONG"
	if side == protect.Short {
		sideStr = "SHORT"
	}

	// 调用Trader的SetStopLoss方法（会自动取消旧止损单）
	_, err := a.trader.SetStopLoss(symbol, sideStr, qty, stopPrice)
	return err
}

// 🎯 V-19.0: UpsertTakeProfit 实现protect.StopExecutor接口（ROI-based automatic take-profit）
func (a *stopExecutorAdapter) UpsertTakeProfit(ctx context.Context, symbol string, side protect.Side, qty float64, tpPrice float64, trigger protect.TriggerType) error {
	// 转换Side
	sideStr := "LONG"
	if side == protect.Short {
		sideStr = "SHORT"
	}

	// 调用Trader的SetTakeProfit方法（会自动取消旧止盈单）
	err := a.trader.SetTakeProfit(symbol, sideStr, qty, tpPrice)
	return err
}

// ========== StopQuery Adapter ==========

// stopQueryAdapter 适配器：将Trader接口转换为protect.StopQuery
type stopQueryAdapter struct {
	trader Trader
}

func newStopQueryAdapter(trader Trader) *stopQueryAdapter {
	return &stopQueryAdapter{
		trader: trader,
	}
}

// GetCurrentStop 实现protect.StopQuery接口
func (a *stopQueryAdapter) GetCurrentStop(ctx context.Context, symbol string, side protect.Side) (float64, bool, error) {
	// 获取当前挂单
	orders, err := a.trader.GetOpenOrders(symbol)
	if err != nil {
		return 0, false, err
	}

	// 查找止损单
	sideStr := "LONG"
	if side == protect.Short {
		sideStr = "SHORT"
	}

	for _, order := range orders {
		orderType, _ := order["type"].(string)
		positionSide, _ := order["positionSide"].(string)

		if (orderType == "STOP_MARKET" || orderType == "STOP") && positionSide == sideStr {
			stopPriceStr, ok := order["stopPrice"].(string)
			if !ok {
				continue
			}

			stopPrice, err := strconv.ParseFloat(stopPriceStr, 64)
			if err != nil {
				return 0, false, fmt.Errorf("parse stop price failed: %v", err)
			}

			return stopPrice, true, nil
		}
	}

	// 未找到止损单
	return 0, false, nil
}
