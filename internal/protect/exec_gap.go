package protect

import "math"

// FloorToTick 向下取整到最近的tick
func FloorToTick(x, tick float64) float64 {
	return math.Floor(x/tick) * tick
}

// CeilToTick 向上取整到最近的tick
func CeilToTick(x, tick float64) float64 {
	return math.Ceil(x/tick) * tick
}

// ExecBoundsForStop 计算止损可执行边界
// 止损价格必须距离参考价格至少 (StopDistanceMinTicks + SafetyTicks) 个tick
//   - LONG: 止损必须 <= UpperExec (refPrice - gap)
//   - SHORT: 止损必须 >= LowerExec (refPrice + gap)
func ExecBoundsForStop(cfg Config, refPrice, tick float64) ExecBounds {
	gap := float64(cfg.StopDistanceMinTicks+cfg.SafetyTicks) * tick
	return ExecBounds{
		UpperExec: refPrice - gap, // LONG止损上界
		LowerExec: refPrice + gap, // SHORT止损下界
	}
}

// ExecCheckResult 可执行性检查结果
type ExecCheckResult struct {
	Ok      bool   // 是否可执行
	ExecGap bool   // 是否存在EXEC_GAP
	Reason  string // 原因说明
}

// CheckExecutable 检查候选止损价是否满足可执行边界
// 如果违反边界，返回EXEC_GAP错误，本轮必须延后（不改单）
func CheckExecutable(pos PositionState, cfg Config, refPrice float64, tick float64, candidate float64) ExecCheckResult {
	b := ExecBoundsForStop(cfg, refPrice, tick)

	if pos.Side == Long {
		// LONG止损必须在当前价下方，且不能太靠近
		if candidate > b.UpperExec {
			return ExecCheckResult{
				Ok:      false,
				ExecGap: true,
				Reason:  "EXEC_GAP_LONG: candidate > upper_exec",
			}
		}
	} else {
		// SHORT止损必须在当前价上方，且不能太靠近
		if candidate < b.LowerExec {
			return ExecCheckResult{
				Ok:      false,
				ExecGap: true,
				Reason:  "EXEC_GAP_SHORT: candidate < lower_exec",
			}
		}
	}

	return ExecCheckResult{Ok: true}
}
