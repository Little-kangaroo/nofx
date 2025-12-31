package protect

import (
	"fmt"
	"math"
)

// Engine 锁盈引擎
type Engine struct {
	Cfg  Config   // 配置
	Fees FeeModel // 费用模型

	// SoftStop 可选的软止损计算函数（基于结构/ATR等）
	// 在Bar Close时可启用，Fast Loop中建议关闭
	SoftStop func(pos PositionState, m MarketSnapshot, r0 float64, protectMode bool) float64

	// UseAggressiveProfile 判断是否使用激进档锁盈
	// 如果为nil，默认使用基础档
	UseAggressiveProfile func(pos PositionState) bool
}

// Evaluate 评估持仓并生成止损更新计划
// 这是锁盈引擎的核心方法，实现V-18.0完整规则：
//  1. ROI ProfitFloor（优先级最高）
//  2. Break-even保护
//  3. R_lock里程碑锁盈
//  4. EXEC_GAP检查
//  5. 单调性/去抖/冷却
func (e *Engine) Evaluate(pos PositionState, m MarketSnapshot) StopUpdatePlan {
	plan := StopUpdatePlan{}

	// ========== 输入验证 ==========
	if pos.Entry <= 0 || pos.InitStop <= 0 || m.TickSize <= 0 || m.LastPrice <= 0 {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = "missing entry/init_stop/tick/last"
		return plan
	}

	// 验证杠杆和数量（零或负值无效）
	if pos.Leverage <= 0 {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = "invalid leverage <= 0"
		return plan
	}

	if pos.Qty <= 0 {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = "invalid qty <= 0"
		return plan
	}

	// 获取参考价格（last或mark）
	ref, err := RefPriceForStop(pos, m)
	if err != nil {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = err.Error()
		return plan
	}
	plan.RefPrice = ref

	// ========== 冷却期检查 ==========
	if pos.LastStopUpdateTimeMs > 0 {
		cooldownMs := e.Cfg.UpdateCooldownSec * 1000
		if (m.NowMs - pos.LastStopUpdateTimeMs) < cooldownMs {
			plan.Reasons = append(plan.Reasons, ReasonCooldown)
			plan.Note = "cooldown"
			// 保持状态机
			plan.NextROIArmed = pos.ROIArmed
			plan.NextBreakEvenArmed = pos.BreakEvenArmed
			plan.NextRLockStage = pos.RLockStage
			return plan
		}
	}

	// ========== 计算基础指标 ==========
	r0, err := RiskUnitR0(pos)
	if err != nil {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = err.Error()
		return plan
	}
	plan.R0 = r0
	plan.RUnr = RUnrealized(pos, m.LastPrice, r0)
	plan.RoiUnr = ROIUnrealized(pos, m.LastPrice)

	// ========== 🎯 V-19.0: ROI止盈计算 ==========
	tpCandidate := pos.PrevTakeProfit

	// 使用Config中的ROI里程碑配置
	tpMilestones := e.Cfg.TPMilestones

	// 找到满足条件的最高ROI里程碑（map迭代顺序随机，需显式查找最大值）
	highestROIThreshold := 0.0
	highestTPPct := 0.0
	for roiThreshold, tpPct := range tpMilestones {
		if plan.RoiUnr >= roiThreshold && roiThreshold > highestROIThreshold {
			highestROIThreshold = roiThreshold
			highestTPPct = tpPct
		}
	}

	// 如果找到了满足条件的里程碑，计算止盈价
	tpReason := ""
	if highestROIThreshold > 0 {
		var targetTP float64
		if pos.Side == Long {
			// LONG: 止盈价 = 入场价 * (1 + 止盈百分比)
			targetTP = pos.Entry * (1 + highestTPPct)
			// 单调性：止盈只能上移（初始值为0时直接设置）
			if pos.PrevTakeProfit == 0 {
				tpCandidate = targetTP
			} else {
				tpCandidate = maxFloat(tpCandidate, targetTP)
			}
		} else {
			// SHORT: 止盈价 = 入场价 * (1 - 止盈百分比)
			targetTP = pos.Entry * (1 - highestTPPct)
			// 单调性：止盈只能下移（初始值为0时直接设置）
			if pos.PrevTakeProfit == 0 {
				tpCandidate = targetTP
			} else {
				tpCandidate = minFloat(tpCandidate, targetTP)
			}
		}
		tpReason = fmt.Sprintf("ROI_%.0f_PCT", highestROIThreshold*100)
	}

	// 检查是否需要更新止盈（单调性约束）
	shouldUpdateTP := false
	if pos.Side == Long {
		// LONG: 止盈只能上移，或者初次设置（prevTP=0）
		shouldUpdateTP = (tpCandidate > pos.PrevTakeProfit) || (pos.PrevTakeProfit == 0 && tpCandidate > 0)
	} else {
		// SHORT: 止盈只能下移，或者初次设置（prevTP=0）
		shouldUpdateTP = (tpCandidate < pos.PrevTakeProfit && pos.PrevTakeProfit > 0) || (pos.PrevTakeProfit == 0 && tpCandidate > 0)
	}

	if shouldUpdateTP {
		plan.ShouldUpdateTP = true
		plan.NewTakeProfit = tpCandidate
		plan.TPReason = tpReason
	}

	// 计算持仓时长（秒）
	timeInPosSec := int64(0)
	if pos.OpenTimeMs > 0 && m.NowMs > pos.OpenTimeMs {
		timeInPosSec = (m.NowMs - pos.OpenTimeMs) / 1000
	}

	// ========== 状态机：ROI锁盈触发 ==========
	roiArmed := pos.ROIArmed
	if !roiArmed {
		// 检查ROI触发条件
		if plan.RoiUnr >= e.Cfg.ROILockTrigger {
			// 时间门槛检查（快速通道可跳过）
			if timeInPosSec >= e.Cfg.ProtectTimeMinSec || plan.RoiUnr >= e.Cfg.ROILockFastTrigger {
				roiArmed = true
				plan.Reasons = append(plan.Reasons, ReasonROIArm)
			}
		}
	}

	// ========== 状态机：Break-even触发 ==========
	beArmed := pos.BreakEvenArmed
	if !beArmed {
		if plan.RUnr >= e.Cfg.BreakEvenTriggerR && timeInPosSec >= e.Cfg.ProtectTimeMinSec {
			beArmed = true
			plan.Reasons = append(plan.Reasons, ReasonBreakEven)
		}
	}

	// ========== 计算BE_with_costs ==========
	be, err := BEWithCosts(pos, e.Cfg, m, e.Fees)
	if err != nil {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = err.Error()
		return plan
	}
	plan.BE = be

	// ========== 计算ProfitFloor（仅在ROI armed时生效） ==========
	floor := math.NaN()
	if roiArmed {
		floor = ProfitFloor(pos, e.Cfg, be)
		plan.Floor = floor
	}

	// ========== Protect模式检查 ==========
	protectMode := false
	if e.Cfg.ProtectEnable && plan.RUnr >= e.Cfg.ProtectTriggerR && timeInPosSec >= e.Cfg.ProtectTimeMinSec {
		protectMode = true
	}

	// ========== 选择R_lock档位（基础档 vs 激进档） ==========
	useAggr := false
	if e.UseAggressiveProfile != nil {
		useAggr = e.UseAggressiveProfile(pos)
	}
	milestones := e.Cfg.RLockMilestones
	locks := e.Cfg.RLockAt
	if useAggr {
		milestones = e.Cfg.RLockMilestonesAggr
		locks = e.Cfg.RLockAtAggr
	}

	// ========== 状态机：R_lock里程碑 ==========
	stage := HighestStage(plan.RUnr, milestones)
	nextStage := pos.RLockStage
	if stage+1 > nextStage {
		nextStage = stage + 1
	}

	rLockStop := math.NaN()
	if stage >= 0 && stage < len(locks) {
		lockR := locks[stage] * r0
		if pos.Side == Long {
			rLockStop = pos.Entry + lockR
		} else {
			rLockStop = pos.Entry - lockR
		}
		plan.Reasons = append(plan.Reasons, ReasonRLock)
	}

	// ========== 聚合候选止损价 ==========
	candidate := pos.PrevStop

	if pos.Side == Long {
		// LONG: 取max（止损只能上移）
		candidate = maxFloat(candidate, floor)
		if beArmed {
			candidate = maxFloat(candidate, be+e.Cfg.BreakEvenPadR*r0)
		}
		candidate = maxFloat(candidate, rLockStop)

		// SoftStop（可选，Bar Close时启用）
		if e.SoftStop != nil {
			s := e.SoftStop(pos, m, r0, protectMode)
			candidate = maxFloat(candidate, s)
		}

		// 向下取整到tick
		candidate = FloorToTick(candidate, m.TickSize)
	} else {
		// SHORT: 取min（止损只能下移）
		candidate = minFloat(candidate, floor)
		if beArmed {
			candidate = minFloat(candidate, be-e.Cfg.BreakEvenPadR*r0)
		}
		candidate = minFloat(candidate, rLockStop)

		// SoftStop（可选）
		if e.SoftStop != nil {
			s := e.SoftStop(pos, m, r0, protectMode)
			candidate = minFloat(candidate, s)
		}

		// 向上取整到tick
		candidate = CeilToTick(candidate, m.TickSize)
	}

	// ========== 检查是否有改善（单调性） ==========
	improved := false
	if pos.Side == Long {
		improved = candidate > pos.PrevStop
	} else {
		improved = candidate < pos.PrevStop
	}

	if !improved {
		plan.Reasons = append(plan.Reasons, ReasonNoChange)
		plan.Note = "not improved"
		plan.NextROIArmed = roiArmed
		plan.NextBreakEvenArmed = beArmed
		plan.NextRLockStage = nextStage
		return plan
	}

	// ========== 检查最小移动距离 ==========
	minMove := float64(e.Cfg.MinTickMoveToUpdate) * m.TickSize
	if math.Abs(candidate-pos.PrevStop) < minMove {
		plan.Reasons = append(plan.Reasons, ReasonStepTooSmall)
		plan.Note = "min move not reached"
		plan.NextROIArmed = roiArmed
		plan.NextBreakEvenArmed = beArmed
		plan.NextRLockStage = nextStage
		return plan
	}

	// ========== 可执行性检查（EXEC_GAP） ==========
	exec := CheckExecutable(pos, e.Cfg, ref, m.TickSize, candidate)
	plan.Bounds = ExecBoundsForStop(e.Cfg, ref, m.TickSize)

	if !exec.Ok {
		plan.Reasons = append(plan.Reasons, ReasonExecGap)
		plan.ExecGap = true
		plan.Note = exec.Reason
		plan.NextROIArmed = roiArmed
		plan.NextBreakEvenArmed = beArmed
		plan.NextRLockStage = nextStage
		return plan
	}

	// ========== 通过所有检查，准备更新 ==========
	plan.ShouldUpdate = true
	plan.NewStop = candidate
	plan.Note = fmt.Sprintf("update (aggr=%v)", useAggr)

	plan.NextROIArmed = roiArmed
	plan.NextBreakEvenArmed = beArmed
	plan.NextRLockStage = nextStage

	return plan
}

// ========== 辅助函数 ==========

// maxFloat 返回两个浮点数中的最大值（忽略NaN）
func maxFloat(cur float64, x float64) float64 {
	if math.IsNaN(x) {
		return cur
	}
	if x > cur {
		return x
	}
	return cur
}

// minFloat 返回两个浮点数中的最小值（忽略NaN）
func minFloat(cur float64, x float64) float64 {
	if math.IsNaN(x) {
		return cur
	}
	if x < cur {
		return x
	}
	return cur
}

// HighestStage 查找当前R_unr达到的最高里程碑阶段
// 返回值：-1表示未达到任何里程碑，否则返回里程碑索引（0-based）
func HighestStage(rUnr float64, milestones []float64) int {
	stage := -1
	for i, m := range milestones {
		if rUnr >= m {
			stage = i
		}
	}
	return stage
}
