package protect

import (
	"context"
	"log"
	"time"
)

// PositionStore 持仓存储接口（需要adapter实现）
type PositionStore interface {
	ListOpenPositions(ctx context.Context) ([]PositionState, error)
	SavePositionState(ctx context.Context, pos PositionState) error
}

// PriceCache 价格缓存接口（需要adapter实现）
type PriceCache interface {
	GetSnapshot(ctx context.Context, symbol string) (MarketSnapshot, bool)
}

// StopExecutor 止损执行接口（需要adapter实现）
type StopExecutor interface {
	UpsertStop(ctx context.Context, symbol string, side Side, qty float64, stopPrice float64, trigger TriggerType) error
	// 🎯 V-19.0: 止盈执行接口（ROI-based automatic take-profit）
	UpsertTakeProfit(ctx context.Context, symbol string, side Side, qty float64, tpPrice float64, trigger TriggerType) error
}

// Scheduler 锁盈调度器（双触发器架构）
type Scheduler struct {
	Eng     *Engine      // 完整引擎（带SoftStop）
	EngFast *Engine      // 快速引擎（无SoftStop）
	Store   PositionStore
	Prices  PriceCache
	Exec    StopExecutor
}

// NewScheduler 创建调度器
func NewScheduler(eng *Engine, store PositionStore, prices PriceCache, exec StopExecutor) *Scheduler {
	// 创建Fast Loop专用引擎（关闭SoftStop）
	engFast := *eng
	if !eng.Cfg.Scheduler.EnableSoftStopInFastLoop {
		engFast.SoftStop = nil
	}

	return &Scheduler{
		Eng:     eng,
		EngFast: &engFast,
		Store:   store,
		Prices:  prices,
		Exec:    exec,
	}
}

// StartFastLoop 启动10秒快速检查循环（goroutine）
// 这是主要的锁盈触发器，每10秒检查所有持仓
func (s *Scheduler) StartFastLoop(ctx context.Context) {
	interval := time.Duration(s.Eng.Cfg.Scheduler.CheckIntervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[ProfitLocking] Fast Loop启动 (间隔: %v)", interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[ProfitLocking] Fast Loop停止")
			return
		case <-ticker.C:
			s.tickOnce(ctx)
		}
	}
}

// tickOnce 执行一次锁盈检查（所有持仓）
func (s *Scheduler) tickOnce(ctx context.Context) {
	positions, err := s.Store.ListOpenPositions(ctx)
	if err != nil {
		log.Printf("[ProfitLocking] 获取持仓失败: %v", err)
		return
	}

	if len(positions) == 0 {
		return // 无持仓，跳过
	}

	nowMs := time.Now().UnixMilli()

	for _, pos := range positions {
		// 获取市场快照
		snap, ok := s.Prices.GetSnapshot(ctx, pos.Symbol)
		if !ok {
			// 价格数据陈旧或缺失
			continue
		}
		snap.NowMs = nowMs

		// 使用Fast引擎评估（无SoftStop）
		plan := s.EngFast.Evaluate(pos, snap)

		// 回写状态机（即使没有改单，也要保持armed/stage）
		pos.ROIArmed = plan.NextROIArmed
		pos.BreakEvenArmed = plan.NextBreakEvenArmed
		pos.RLockStage = plan.NextRLockStage

		if !plan.ShouldUpdate {
			// 无需更新，仅保存状态
			if err := s.Store.SavePositionState(ctx, pos); err != nil {
				log.Printf("⚠️ [锁盈] 保存持仓状态失败: %s %s: %v", pos.Symbol, pos.Side, err)
			}

			// 如果有EXEC_GAP，记录日志
			if plan.ExecGap {
				log.Printf("⏸️  [锁盈延后] %s %s | EXEC_GAP: ref=%.6f bounds=[%.6f,%.6f] candidate=%.6f | %s",
					pos.Symbol, pos.Side,
					plan.RefPrice, plan.Bounds.LowerExec, plan.Bounds.UpperExec, plan.NewStop,
					plan.Note)
			}
			continue
		}

		// 执行止损更新
		log.Printf("🔒 [锁盈] %s %s: %.6f → %.6f | ROI:%.2f%% R:%.2fR | Reasons:%v",
			pos.Symbol, pos.Side,
			pos.PrevStop, plan.NewStop,
			plan.RoiUnr*100, plan.RUnr,
			plan.Reasons,
		)

		if err := s.Exec.UpsertStop(ctx, pos.Symbol, pos.Side, pos.Qty, plan.NewStop, pos.StopTriggerType); err != nil {
			log.Printf("❌ [锁盈失败] %s %s: %v", pos.Symbol, pos.Side, err)
			continue
		}

		// 更新持仓状态
		pos.PrevStop = plan.NewStop
		pos.LastStopUpdateTimeMs = nowMs
		if err := s.Store.SavePositionState(ctx, pos); err != nil {
			log.Printf("❌ [锁盈] 更新持仓状态失败: %s %s: %v", pos.Symbol, pos.Side, err)
		}

		log.Printf("✅ [锁盈成功] %s %s | 新止损: %.6f", pos.Symbol, pos.Side, plan.NewStop)

		// 🎯 V-19.0: 止盈更新逻辑（独立于止损更新）
		if plan.ShouldUpdateTP {
			log.Printf("📈 [止盈] %s %s: %.6f → %.6f | ROI:%.2f%% | Reason:%s",
				pos.Symbol, pos.Side,
				pos.PrevTakeProfit, plan.NewTakeProfit,
				plan.RoiUnr*100,
				plan.TPReason,
			)

			if err := s.Exec.UpsertTakeProfit(ctx, pos.Symbol, pos.Side, pos.Qty, plan.NewTakeProfit, pos.StopTriggerType); err != nil {
				log.Printf("❌ [止盈失败] %s %s: %v", pos.Symbol, pos.Side, err)
			} else {
				// 更新持仓状态中的止盈价格
				pos.PrevTakeProfit = plan.NewTakeProfit
				pos.LastTPUpdateTimeMs = nowMs
				if err := s.Store.SavePositionState(ctx, pos); err != nil {
					log.Printf("❌ [止盈] 更新持仓状态失败: %s %s: %v", pos.Symbol, pos.Side, err)
				} else {
					log.Printf("✅ [止盈成功] %s %s | 新止盈: %.6f", pos.Symbol, pos.Side, plan.NewTakeProfit)
				}
			}
		}
	}
}

// OnBarClose5m 5分钟K线收盘钩子（可选）
// 与AI决策周期同步，可启用SoftStop
func (s *Scheduler) OnBarClose5m(ctx context.Context, symbol string) {
	positions, err := s.Store.ListOpenPositions(ctx)
	if err != nil {
		return
	}

	nowMs := time.Now().UnixMilli()

	for _, pos := range positions {
		if pos.Symbol != symbol {
			continue
		}

		snap, ok := s.Prices.GetSnapshot(ctx, symbol)
		if !ok {
			return
		}
		snap.NowMs = nowMs

		// 使用完整引擎（带SoftStop）
		plan := s.Eng.Evaluate(pos, snap)

		pos.ROIArmed = plan.NextROIArmed
		pos.BreakEvenArmed = plan.NextBreakEvenArmed
		pos.RLockStage = plan.NextRLockStage

		if !plan.ShouldUpdate {
			_ = s.Store.SavePositionState(ctx, pos)
			return
		}

		log.Printf("🔒 [Bar Close锁盈] %s %s: %.6f → %.6f | ROI:%.2f%% R:%.2fR",
			pos.Symbol, pos.Side,
			pos.PrevStop, plan.NewStop,
			plan.RoiUnr*100, plan.RUnr,
		)

		if err := s.Exec.UpsertStop(ctx, pos.Symbol, pos.Side, pos.Qty, plan.NewStop, pos.StopTriggerType); err != nil {
			log.Printf("❌ [Bar Close锁盈失败] %s %s: %v", pos.Symbol, pos.Side, err)
			return
		}

		pos.PrevStop = plan.NewStop
		pos.LastStopUpdateTimeMs = nowMs
		_ = s.Store.SavePositionState(ctx, pos)

		log.Printf("✅ [Bar Close锁盈成功] %s %s", pos.Symbol, pos.Side)
		return
	}
}
