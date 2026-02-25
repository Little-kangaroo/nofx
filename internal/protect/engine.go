package protect

import (
	"fmt"
	"math"
)

// Engine 锁盈引擎
type Engine struct {
	Cfg  Config
	Fees FeeModel // 保留字段兼容现有初始化代码，当前未使用
}

// Evaluate 评估持仓并生成止损更新计划
//
// 流程（简化版，不依赖TickSize做逻辑判断）：
//  1. 输入验证
//  2. 冷却期检查
//  3. 计算 R0 / RUnr / RoiUnr
//  4. ROI 触发检查（>= 5%）
//  5. 查阶梯表计算盈利地板 floor = Entry ± R0×floorPct
//  6. 对齐 tick（仅格式化，不影响判断）
//  7. 单调性：只能往有利方向移动
//  8. 安全检查：不能超过当前价（防止立即触发）
//  9. 更新
func (e *Engine) Evaluate(pos PositionState, m MarketSnapshot) StopUpdatePlan {
	plan := StopUpdatePlan{}

	// ========== 输入验证 ==========
	if pos.Entry <= 0 || pos.InitStop <= 0 || m.LastPrice <= 0 {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = "missing entry/init_stop/last"
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

	// ========== 止盈计算（ROI阶梯，独立于止损逻辑） ==========
	tpCandidate := pos.PrevTakeProfit
	tpReason := ""

	if e.Cfg.TPMilestones != nil {
		highestROIThreshold := 0.0
		highestTPPct := 0.0
		for roiThreshold, tpPct := range e.Cfg.TPMilestones {
			if plan.RoiUnr >= roiThreshold && roiThreshold > highestROIThreshold {
				highestROIThreshold = roiThreshold
				highestTPPct = tpPct
			}
		}
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

	// ========== ROI 触发检查 ==========
	roiArmed := pos.ROIArmed
	if !roiArmed && plan.RoiUnr >= e.Cfg.ROILockTrigger {
		roiArmed = true
		plan.Reasons = append(plan.Reasons, ReasonROIArm)
	}
	if !roiArmed {
		plan.Reasons = append(plan.Reasons, ReasonNoChange)
		plan.Note = "ROI not triggered"
		plan.NextROIArmed = roiArmed
		return plan
	}

	// ========== 计算盈利地板（纯价格计算，不依赖TickSize） ==========
	floor := ProfitFloor(pos, e.Cfg, plan.RoiUnr)
	plan.Floor = floor
	if floor <= 0 {
		plan.Reasons = append(plan.Reasons, ReasonInvalidInput)
		plan.Note = "invalid floor <= 0"
		plan.NextROIArmed = roiArmed
		return plan
	}

	// ========== 候选止损（取较优值，对齐tick仅用于报价格式） ==========
	var candidate float64
	if pos.Side == Long {
		candidate = maxFloat(pos.PrevStop, floor)
		if m.TickSize > 0 {
			candidate = FloorToTick(candidate, m.TickSize)
		}
	} else {
		candidate = minFloat(pos.PrevStop, floor)
		if m.TickSize > 0 {
			candidate = CeilToTick(candidate, m.TickSize)
		}
	}

	// ========== 单调性：只能往有利方向移动 ==========
	if pos.Side == Long && candidate <= pos.PrevStop {
		plan.Reasons = append(plan.Reasons, ReasonNoChange)
		plan.Note = "not improved"
		plan.NextROIArmed = roiArmed
		return plan
	}
	if pos.Side == Short && candidate >= pos.PrevStop {
		plan.Reasons = append(plan.Reasons, ReasonNoChange)
		plan.Note = "not improved"
		plan.NextROIArmed = roiArmed
		return plan
	}

	// ========== 安全检查：不能超过当前价（防止立即触发） ==========
	if pos.Side == Long && candidate >= ref {
		plan.Reasons = append(plan.Reasons, ReasonExecGap)
		plan.ExecGap = true
		plan.Note = fmt.Sprintf("candidate(%.6f) >= ref(%.6f), would trigger immediately", candidate, ref)
		plan.NextROIArmed = roiArmed
		return plan
	}
	if pos.Side == Short && candidate <= ref {
		plan.Reasons = append(plan.Reasons, ReasonExecGap)
		plan.ExecGap = true
		plan.Note = fmt.Sprintf("candidate(%.6f) <= ref(%.6f), would trigger immediately", candidate, ref)
		plan.NextROIArmed = roiArmed
		return plan
	}

	// ========== 全部通过，准备更新 ==========
	plan.ShouldUpdate = true
	plan.NewStop = candidate
	plan.Note = "ROI lock update"
	plan.NextROIArmed = roiArmed
	return plan
}

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
