package config

import (
	"fmt"
	"time"
)

// ========== 锁盈系统扩展 ==========

// PositionProtectState 锁盈状态（用于更新）
type PositionProtectState struct {
	PrevStop             float64
	LastStopUpdateTimeMs int64
	ROIArmed             bool
	BreakEvenArmed       bool
	RLockStage           int
}

// TradeRecordWithProtect 扩展的交易记录（包含锁盈字段）
type TradeRecordWithProtect struct {
	TradeRecord
	InitialStopPrice     float64   `json:"initial_stop_price"`
	CurrentStopPrice     float64   `json:"current_stop_price"`
	LastStopUpdateTime   time.Time `json:"last_stop_update_time"`
	ROIArmed             bool      `json:"roi_armed"`
	BreakEvenArmed       bool      `json:"break_even_armed"`
	RLockStage           int       `json:"r_lock_stage"`
}

// MigrateProtectFields 添加锁盈相关字段到trades表（迁移）
func (db *Database) MigrateProtectFields() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// 检查字段是否已存在
	var columnCount int
	err := db.db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('trades')
		WHERE name IN ('initial_stop_price', 'current_stop_price', 'last_stop_update_time', 'roi_armed', 'break_even_armed', 'r_lock_stage')
	`).Scan(&columnCount)

	if err != nil {
		return err
	}

	if columnCount == 6 {
		// 字段已存在
		return nil
	}

	// 添加字段
	migrations := []string{
		`ALTER TABLE trades ADD COLUMN initial_stop_price REAL DEFAULT 0`,
		`ALTER TABLE trades ADD COLUMN current_stop_price REAL DEFAULT 0`,
		`ALTER TABLE trades ADD COLUMN last_stop_update_time DATETIME`,
		`ALTER TABLE trades ADD COLUMN roi_armed INTEGER DEFAULT 0`,
		`ALTER TABLE trades ADD COLUMN break_even_armed INTEGER DEFAULT 0`,
		`ALTER TABLE trades ADD COLUMN r_lock_stage INTEGER DEFAULT 0`,
	}

	for _, migration := range migrations {
		if _, err := db.db.Exec(migration); err != nil {
			// 如果字段已存在，忽略错误
			continue
		}
	}

	return nil
}

// GetOpenTradesForTrader 获取trader的所有open trades
func (db *Database) GetOpenTradesForTrader(traderID string) ([]TradeRecordWithProtect, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.db.Query(`
		SELECT
			id, trader_id, symbol, side, quantity, leverage,
			open_price, close_price, position_value, margin_used,
			pnl, pnl_pct, duration_seconds,
			open_time, close_time, status, close_reason,
			open_order_id, close_order_id,
			initial_stop_price, current_stop_price, last_stop_update_time,
			roi_armed, break_even_armed, r_lock_stage,
			created_at, updated_at
		FROM trades
		WHERE trader_id = ? AND status = 'open'
		ORDER BY open_time DESC
	`, traderID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []TradeRecordWithProtect
	for rows.Next() {
		var t TradeRecordWithProtect
		var closePrice, closeTime, closeReason, closeOrderID *string
		var lastStopUpdateTime *time.Time

		err := rows.Scan(
			&t.ID, &t.TraderID, &t.Symbol, &t.Side, &t.Quantity, &t.Leverage,
			&t.OpenPrice, &closePrice, &t.PositionValue, &t.MarginUsed,
			&t.PnL, &t.PnLPct, &t.DurationSecs,
			&t.OpenTime, &closeTime, &t.Status, &closeReason,
			&t.OpenOrderID, &closeOrderID,
			&t.InitialStopPrice, &t.CurrentStopPrice, &lastStopUpdateTime,
			&t.ROIArmed, &t.BreakEvenArmed, &t.RLockStage,
			&t.CreatedAt, &t.UpdatedAt,
		)

		if err != nil {
			return nil, err
		}

		if lastStopUpdateTime != nil {
			t.LastStopUpdateTime = *lastStopUpdateTime
		}

		trades = append(trades, t)
	}

	return trades, rows.Err()
}

// UpdatePositionProtectState 更新持仓的锁盈状态
func (db *Database) UpdatePositionProtectState(traderID, symbol, side string, state *PositionProtectState) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	lastUpdateTime := time.UnixMilli(state.LastStopUpdateTimeMs)

	result, err := db.db.Exec(`
		UPDATE trades SET
			current_stop_price = ?,
			last_stop_update_time = ?,
			roi_armed = ?,
			break_even_armed = ?,
			r_lock_stage = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE trader_id = ? AND symbol = ? AND side = ? AND status = 'open'
	`,
		state.PrevStop,
		lastUpdateTime,
		state.ROIArmed,
		state.BreakEvenArmed,
		state.RLockStage,
		traderID, symbol, side,
	)

	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no open trade found for %s %s %s", traderID, symbol, side)
	}

	return nil
}

// SetInitialStopPrice 设置初始止损价（开仓时调用）
func (db *Database) SetInitialStopPrice(traderID, symbol, side string, initStop float64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.db.Exec(`
		UPDATE trades SET
			initial_stop_price = ?,
			current_stop_price = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE trader_id = ? AND symbol = ? AND side = ? AND status = 'open'
	`, initStop, initStop, traderID, symbol, side)

	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no open trade found for %s %s %s", traderID, symbol, side)
	}

	return nil
}
