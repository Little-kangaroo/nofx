package protect

import (
	"context"
	"log"
	"math"
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
		log.Printf("❌ [ProfitLocking] 获取持仓失败: %v", err)
		return
	}

	if len(positions) == 0 {
		log.Printf("ℹ️  [ProfitLocking] Fast Loop检查: 当前无开仓持仓")
		return
	}

	nowMs := time.Now().UnixMilli()
	log.Printf("🔍 [ProfitLocking] Fast Loop检查开始 ━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("📊 [ProfitLocking] 发现 %d 个持仓，开始逐个评估...\n", len(positions))

	for i, pos := range positions {
		// 获取市场快照
		snap, ok := s.Prices.GetSnapshot(ctx, pos.Symbol)
		if !ok {
			log.Printf("⚠️  [持仓%d/%d] %s %s - 价格数据缺失或陈旧，跳过评估",
				i+1, len(positions), pos.Symbol, pos.Side)
			continue
		}
		snap.NowMs = nowMs

		// 计算基础指标
		currentPrice := snap.LastPrice
		roi := ((currentPrice - pos.Entry) / pos.Entry) * pos.Leverage
		if pos.Side == Short {
			roi = ((pos.Entry - currentPrice) / pos.Entry) * pos.Leverage
		}

		pricePnlPct := ((currentPrice - pos.Entry) / pos.Entry) * 100
		if pos.Side == Short {
			pricePnlPct = ((pos.Entry - currentPrice) / pos.Entry) * 100
		}

		// 持仓时长
		holdTimeSec := int64(0)
		if pos.OpenTimeMs > 0 && nowMs > pos.OpenTimeMs {
			holdTimeSec = (nowMs - pos.OpenTimeMs) / 1000
		}

		// 冷却期剩余时间
		cooldownRemainingSec := int64(0)
		if pos.LastStopUpdateTimeMs > 0 {
			cooldownMs := s.Eng.Cfg.UpdateCooldownSec * 1000
			elapsed := nowMs - pos.LastStopUpdateTimeMs
			if elapsed < cooldownMs {
				cooldownRemainingSec = (cooldownMs - elapsed) / 1000
			}
		}

		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("📍 [持仓%d/%d] %s %s", i+1, len(positions), pos.Symbol, pos.Side)
		log.Printf("   💰 入场价: %.6f | 当前价: %.6f (%.2f%%)", pos.Entry, currentPrice, pricePnlPct)
		log.Printf("   📊 当前ROI: %.2f%% (杠杆: %.0fx)", roi*100, pos.Leverage)
		log.Printf("   🛡️  当前止损: %.6f | 初始止损: %.6f", pos.PrevStop, pos.InitStop)
		log.Printf("   ⏱️  持仓时长: %ds | 冷却期剩余: %ds", holdTimeSec, cooldownRemainingSec)
		log.Printf("   🎯 状态机: ROIArmed=%v | BEArmed=%v | RLockStage=%d",
			pos.ROIArmed, pos.BreakEvenArmed, pos.RLockStage)

		// 使用Fast引擎评估（无SoftStop）
		plan := s.EngFast.Evaluate(pos, snap)

		// 详细打印评估结果
		log.Printf("   🔍 锁盈引擎评估:")
		log.Printf("      R0=%.6f | RUnr=%.2fR | RoiUnr=%.2f%%", plan.R0, plan.RUnr, plan.RoiUnr*100)

		if plan.Floor > 0 && !math.IsNaN(plan.Floor) {
			log.Printf("      盈利地板(Floor)=%.6f", plan.Floor)
		}
		if plan.BE > 0 && !math.IsNaN(plan.BE) {
			log.Printf("      盈亏平衡(BE)=%.6f", plan.BE)
		}

		// 判断是否更新
		if !plan.ShouldUpdate {
			log.Printf("   ❌ 不更新止损 - 原因:")
			for _, reason := range plan.Reasons {
				switch reason {
				case ReasonInvalidInput:
					log.Printf("      ⚠️  输入无效: %s", plan.Note)
				case ReasonCooldown:
					log.Printf("      ⏳ 冷却期限制 (还需等待 %ds)", cooldownRemainingSec)
				case ReasonNoChange:
					log.Printf("      📊 止损未改善 (候选价=%.6f, 当前止损=%.6f)", plan.NewStop, pos.PrevStop)
				case ReasonStepTooSmall:
					minMove := float64(s.Eng.Cfg.MinTickMoveToUpdate) * snap.TickSize
					actualMove := math.Abs(plan.NewStop - pos.PrevStop)
					log.Printf("      📏 移动距离太小 (%.6f < %.6f)", actualMove, minMove)
				case ReasonExecGap:
					log.Printf("      ⚠️  EXEC_GAP: 候选止损%.6f距离当前价%.6f太近", plan.NewStop, plan.RefPrice)
					log.Printf("         可执行区间: [%.6f, %.6f]", plan.Bounds.LowerExec, plan.Bounds.UpperExec)
				}
			}

			// 打印未触发ROI锁盈的原因
			if plan.RoiUnr < s.Eng.Cfg.ROILockTrigger {
				log.Printf("      💡 ROI %.2f%% < 触发阈值 %.2f%% (未达到锁盈条件)",
					plan.RoiUnr*100, s.Eng.Cfg.ROILockTrigger*100)
			} else if !pos.ROIArmed {
				if holdTimeSec < s.Eng.Cfg.ProtectTimeMinSec && plan.RoiUnr < s.Eng.Cfg.ROILockFastTrigger {
					log.Printf("      ⏰ ROI %.2f%% 已达标，但需持仓 %ds (当前 %ds) 或 ROI >= %.2f%%",
						plan.RoiUnr*100, s.Eng.Cfg.ProtectTimeMinSec, holdTimeSec,
						s.Eng.Cfg.ROILockFastTrigger*100)
				}
			}

			if plan.Note != "" && plan.Note != "not improved" && plan.Note != "cooldown" && plan.Note != "min move not reached" {
				log.Printf("      📝 备注: %s", plan.Note)
			}
		} else {
			log.Printf("   ✅ 准备更新止损: %.6f → %.6f", pos.PrevStop, plan.NewStop)
			log.Printf("      触发原因: %v", plan.Reasons)
		}

		// 打印止盈评估（如果有）
		if plan.ShouldUpdateTP {
			log.Printf("   📈 准备更新止盈: %.6f → %.6f | 原因: %s",
				pos.PrevTakeProfit, plan.NewTakeProfit, plan.TPReason)
		} else if s.Eng.Cfg.TPMilestones != nil && len(s.Eng.Cfg.TPMilestones) > 0 {
			// 检查距离下一个止盈里程碑还差多少
			nextMilestone := 0.0
			for roiThreshold := range s.Eng.Cfg.TPMilestones {
				if plan.RoiUnr < roiThreshold && (nextMilestone == 0 || roiThreshold < nextMilestone) {
					nextMilestone = roiThreshold
				}
			}
			if nextMilestone > 0 {
				log.Printf("   💡 距离下一个止盈里程碑 %.0f%% 还差 %.2f%%",
					nextMilestone*100, (nextMilestone-plan.RoiUnr)*100)
			}
		}

		// 显示下一次要更新的止损位置
		if plan.NewStop > 0 && !math.IsNaN(plan.NewStop) && plan.NewStop != pos.PrevStop {
			stopDiff := plan.NewStop - pos.PrevStop
			stopDiffPct := (stopDiff / pos.PrevStop) * 100
			if pos.Side == "LONG" {
				log.Printf("   🎯 下次止损目标: %.6f (当前: %.6f, 提升: +%.6f / +%.2f%%)",
					plan.NewStop, pos.PrevStop, stopDiff, stopDiffPct)
			} else {
				log.Printf("   🎯 下次止损目标: %.6f (当前: %.6f, 提升: %.6f / %.2f%%)",
					plan.NewStop, pos.PrevStop, stopDiff, stopDiffPct)
			}
		} else {
			// 当NewStop为0或无效时，显示其他参考信息
			if plan.Floor > 0 && !math.IsNaN(plan.Floor) {
				floorDiff := math.Abs(plan.Floor - pos.PrevStop)
				if floorDiff > snap.TickSize*2 { // 只有差距足够大时才显示
					log.Printf("   🎯 盈利地板价: %.6f (当前止损: %.6f)", plan.Floor, pos.PrevStop)
				}
			}
			if plan.BE > 0 && !math.IsNaN(plan.BE) {
				beDiff := math.Abs(plan.BE - pos.PrevStop)
				if beDiff > snap.TickSize*2 {
					log.Printf("   🎯 盈亏平衡价: %.6f (当前止损: %.6f)", plan.BE, pos.PrevStop)
				}
			}
			// 显示触发条件
			if plan.RoiUnr < s.Eng.Cfg.ROILockTrigger {
				roiNeeded := (s.Eng.Cfg.ROILockTrigger - plan.RoiUnr) * 100
				log.Printf("   🎯 等待ROI提升 %.2f%% 后将计算新止损位置", roiNeeded)
			}
		}

		log.Printf("")

		// 回写状态机（即使没有改单，也要保持armed/stage）
		pos.ROIArmed = plan.NextROIArmed
		pos.BreakEvenArmed = plan.NextBreakEvenArmed
		pos.RLockStage = plan.NextRLockStage

		if !plan.ShouldUpdate {
			// 无需更新，仅保存状态
			if err := s.Store.SavePositionState(ctx, pos); err != nil {
				log.Printf("⚠️ [锁盈] 保存持仓状态失败: %s %s: %v", pos.Symbol, pos.Side, err)
			}
			continue
		}

		// 执行止损更新
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("🚨🚨🚨 [止损更新] %s %s 🚨🚨🚨", pos.Symbol, pos.Side)
		log.Printf("   📊 止损变化: %.6f → %.6f", pos.PrevStop, plan.NewStop)
		log.Printf("   💰 当前ROI: %.2f%% | R倍数: %.2fR", plan.RoiUnr*100, plan.RUnr)
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

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

		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("✅✅✅ [止损更新成功] %s %s ✅✅✅", pos.Symbol, pos.Side)
		log.Printf("   🎯 新止损价格: %.6f", plan.NewStop)
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

		// 🎯 V-19.0: 止盈更新逻辑（独立于止损更新）
		if plan.ShouldUpdateTP {
			log.Printf("📈 [止盈执行] %s %s: %.6f → %.6f | ROI:%.2f%% | Reason:%s",
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
					log.Printf("✅ [止盈成功] %s %s | 新止盈: %.6f\n", pos.Symbol, pos.Side, plan.NewTakeProfit)
				}
			}
		}
	}

	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("✅ [ProfitLocking] Fast Loop检查完成\n")
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
