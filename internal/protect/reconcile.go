package protect

import (
	"context"
	"log"
	"math"
)

// StopQuery 止损查询接口（需要adapter实现）
type StopQuery interface {
	GetCurrentStop(ctx context.Context, symbol string, side Side) (stopPrice float64, ok bool, err error)
}

// Reconciler 对账纠偏器
// 定期（建议30-60秒）查询交易所真实止损价，回写到PositionStore
// 确保系统PrevStop与交易所实际挂单一致
type Reconciler struct {
	Store   PositionStore
	Query   StopQuery
	Epsilon float64 // 容忍误差（一个tick），默认1e-12
}

// NewReconciler 创建对账器
func NewReconciler(store PositionStore, query StopQuery) *Reconciler {
	return &Reconciler{
		Store:   store,
		Query:   query,
		Epsilon: 1e-12,
	}
}

// ReconcileOnce 执行一次对账（所有持仓）
func (r *Reconciler) ReconcileOnce(ctx context.Context) {
	positions, err := r.Store.ListOpenPositions(ctx)
	if err != nil {
		log.Printf("[Reconciler] 获取持仓失败: %v", err)
		return
	}

	if len(positions) == 0 {
		return
	}

	for _, pos := range positions {
		// 查询交易所真实止损价
		exchangeStop, ok, err := r.Query.GetCurrentStop(ctx, pos.Symbol, pos.Side)
		if err != nil {
			log.Printf("[Reconciler] 查询止损失败 %s %s: %v", pos.Symbol, pos.Side, err)
			continue
		}

		if !ok {
			// 交易所无止损单（可能已触发或取消）
			if pos.PrevStop > 0 {
				log.Printf("[Reconciler] ⚠️  止损单消失 %s %s (prev=%.6f)", pos.Symbol, pos.Side, pos.PrevStop)
				// 可选：将PrevStop清零或触发告警
			}
			continue
		}

		// 检查是否偏离
		eps := r.Epsilon
		if eps <= 0 {
			eps = 1e-12
		}

		drift := math.Abs(exchangeStop - pos.PrevStop)
		if drift > eps {
			log.Printf("[Reconciler] 🔄 纠偏 %s %s: local=%.6f → exchange=%.6f (drift=%.6f)",
				pos.Symbol, pos.Side, pos.PrevStop, exchangeStop, drift)

			// 回写真实止损价
			pos.PrevStop = exchangeStop
			if err := r.Store.SavePositionState(ctx, pos); err != nil {
				log.Printf("[Reconciler] 保存失败 %s %s: %v", pos.Symbol, pos.Side, err)
			}
		}
	}
}
