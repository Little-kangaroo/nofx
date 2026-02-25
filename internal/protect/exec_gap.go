package protect

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
// 🔥 V-21.1: 修复移动止损锁盈场景的EXEC_GAP检查逻辑
//
// 对于移动止损锁盈，我们需要区分两种情况：
// 1. 止损向不利方向移动（靠近当前价）：需要严格检查EXEC_GAP
// 2. 止损向有利方向移动（远离当前价）：只需确保不会立即触发
func CheckExecutable(pos PositionState, cfg Config, refPrice float64, tick float64, candidate float64) ExecCheckResult {
	b := ExecBoundsForStop(cfg, refPrice, tick)

	if pos.Side == Long {
		// LONG止损：
		// - 如果候选止损 > 当前价（会立即触发），拒绝
		// - 如果候选止损在危险区域内（> UpperExec），且比旧止损更靠近当前价，拒绝
		// - 否则允许（包括向上移动锁盈的情况）
		if candidate >= refPrice {
			return ExecCheckResult{
				Ok:      false,
				ExecGap: true,
				Reason:  "EXEC_GAP_LONG: candidate >= refPrice (would trigger immediately)",
			}
		}

		// 如果候选止损在危险区域内，且是向当前价移动（不利方向），拒绝
		if candidate > b.UpperExec && candidate > pos.PrevStop {
			return ExecCheckResult{
				Ok:      false,
				ExecGap: true,
				Reason:  "EXEC_GAP_LONG: candidate in danger zone and moving toward price",
			}
		}
	} else {
		// SHORT止损：
		// - 如果候选止损 < 当前价（会立即触发），拒绝
		// - 如果候选止损在危险区域内（< LowerExec），且比旧止损更靠近当前价，拒绝
		// - 否则允许（包括向下移动锁盈的情况）
		if candidate <= refPrice {
			return ExecCheckResult{
				Ok:      false,
				ExecGap: true,
				Reason:  "EXEC_GAP_SHORT: candidate <= refPrice (would trigger immediately)",
			}
		}

		// 如果候选止损在危险区域内，且是向当前价移动（不利方向），拒绝
		if candidate < b.LowerExec && candidate < pos.PrevStop {
			return ExecCheckResult{
				Ok:      false,
				ExecGap: true,
				Reason:  "EXEC_GAP_SHORT: candidate in danger zone and moving toward price",
			}
		}
	}

	return ExecCheckResult{Ok: true}
}
