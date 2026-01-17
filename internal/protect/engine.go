package protect

import (
	"fmt"
	"math"
)

// Engine 锁盈引擎（V-20.0 简化版：纯ROI锁盈）
type Engine struct {
	Cfg  Config   // 配置
	Fees FeeModel // 费用模型
}

// Evaluate 评估持仓并生成止损更新计划
// V-20.0 简化版：纯ROI锁盈
//  1. ROI触发检查（无时间约束）
//  2. 计算盈利地板（BE + FloorPriceBps）
//  3. EXEC_GAP检查
//  4. 单调性/去抖/冷却
func (e *Engine) Evaluate(pos PositionState, m MarketSnapshot) StopUpdatePlan {
	plan := StopUpdatePlan{}

	// ========== 输入验证 ==========
	if pos.Entry <= 0 || pos.InitStop <= 0 || m.TickSize <= 0 || m.LastPrice <= 0 {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = "missing entry/init_stop/tick/last"
		return plan
	}

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
			plan.NextROIArmed = pos.ROIArmed
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
	tpReason := ""

	if e.Cfg.TPMilestones != nil {
		// 找到满足条件的最高ROI里程碑
		highestROIThreshold := 0.0
		highestTPPct := 0.0
		for roiThreshold, tpPct := range e.Cfg.TPMilestones {
			if plan.RoiUnr >= roiThreshold && roiThreshold > highestROIThreshold {
				highestROIThreshold = roiThreshold
				highestTPPct = tpPct
			}
		}

		// 如果找到了满足条件的里程碑，计算止盈价
		if highestROIThreshold > 0 {
			var targetTP float64
			if pos.Side == Long {
				targetTP = pos.Entry * (1 + highestTPPct/pos.Leverage)
				if pos.PrevTakeProfit == 0 {
					tpCandidate = targetTP
				} else {
					tpCandidate = maxFloat(tpCandidate, targetTP)
				}
			} else {
				targetTP = pos.Entry * (1 - highestTPPct/pos.Leverage)
				if pos.PrevTakeProfit == 0 {
					tpCandidate = targetTP
				} else {
					tpCandidate = minFloat(tpCandidate, targetTP)
				}
			}
			tpReason = fmt.Sprintf("ROI_%.0f_PCT", highestROIThreshold*100)
		}

		// 检查是否需要更新止盈
		shouldUpdateTP := false
		if pos.Side == Long {
			shouldUpdateTP = (tpCandidate > pos.PrevTakeProfit) || (pos.PrevTakeProfit == 0 && tpCandidate > 0)
		} else {
			shouldUpdateTP = (tpCandidate < pos.PrevTakeProfit && pos.PrevTakeProfit > 0) || (pos.PrevTakeProfit == 0 && tpCandidate > 0)
		}

		if shouldUpdateTP {
			plan.ShouldUpdateTP = true
			plan.NewTakeProfit = tpCandidate
			plan.TPReason = tpReason
		}
	}

	// ========== ROI锁盈触发检查（无时间约束） ==========
	roiArmed := pos.ROIArmed
	if !roiArmed && plan.RoiUnr >= e.Cfg.ROILockTrigger {
		roiArmed = true
		plan.Reasons = append(plan.Reasons, ReasonROIArm)
	}

	// ========== 计算盈利地板（仅在ROI armed时生效） ==========
	if !roiArmed {
		// ROI未触发，不更新止损
		plan.Reasons = append(plan.Reasons, ReasonNoChange)
		plan.Note = "ROI not triggered"
		plan.NextROIArmed = roiArmed
		return plan
	}

	// 计算BE_with_costs
	be, err := BEWithCosts(pos, e.Cfg, m, e.Fees)
	if err != nil {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = err.Error()
		return plan
	}
	plan.BE = be

	// 计算盈利地板
	floor := ProfitFloor(pos, e.Cfg, be)
	plan.Floor = floor

	// ========== 计算候选止损价 ==========
	candidate := pos.PrevStop

	if pos.Side == Long {
		// LONG: 止损向盈利地板移动（只能上移）
		candidate = maxFloat(candidate, floor)
		candidate = FloorToTick(candidate, m.TickSize)
	} else {
		// SHORT: 止损向盈利地板移动（只能下移）
		candidate = minFloat(candidate, floor)
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
		return plan
	}

	// ========== 检查最小移动距离 ==========
	minMove := float64(e.Cfg.MinTickMoveToUpdate) * m.TickSize
	if math.Abs(candidate-pos.PrevStop) < minMove {
		plan.Reasons = append(plan.Reasons, ReasonStepTooSmall)
		plan.Note = "min move not reached"
		plan.NextROIArmed = roiArmed
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
		return plan
	}

	// ========== 通过所有检查，准备更新 ==========
	plan.ShouldUpdate = true
	plan.NewStop = candidate
	plan.Note = "ROI lock update"

	plan.NextROIArmed = roiArmed

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
