package protect

import (
	"fmt"
)

// RefPriceForStop 获取止损参考价格（根据触发类型选择last或mark）
func RefPriceForStop(pos PositionState, m MarketSnapshot) (float64, error) {
	if pos.StopTriggerType == TriggerMark {
		if m.MarkPrice <= 0 {
			return 0, fmt.Errorf("mark_price missing")
		}
		return m.MarkPrice, nil
	}
	if m.LastPrice <= 0 {
		return 0, fmt.Errorf("last_price missing")
	}
	return m.LastPrice, nil
}

// RiskUnitR0 计算初始风险单位R0（入场价与初始止损的距离）
// 多空对称：
//   - LONG: R0 = Entry - InitStop
//   - SHORT: R0 = InitStop - Entry
func RiskUnitR0(pos PositionState) (float64, error) {
	switch pos.Side {
	case Long:
		r0 := pos.Entry - pos.InitStop
		if r0 <= 0 {
			return 0, fmt.Errorf("invalid R0 for LONG: entry(%.6f) <= init_stop(%.6f)", pos.Entry, pos.InitStop)
		}
		return r0, nil
	case Short:
		r0 := pos.InitStop - pos.Entry
		if r0 <= 0 {
			return 0, fmt.Errorf("invalid R0 for SHORT: init_stop(%.6f) <= entry(%.6f)", pos.InitStop, pos.Entry)
		}
		return r0, nil
	default:
		return 0, fmt.Errorf("invalid side: %s", pos.Side)
	}
}

// RUnrealized 计算未实现R倍数（当前浮盈相对初始风险R0的倍数）
// 多空对称：
//   - LONG: R_unr = (last - entry) / R0
//   - SHORT: R_unr = (entry - last) / R0
func RUnrealized(pos PositionState, last float64, r0 float64) float64 {
	if pos.Side == Long {
		return (last - pos.Entry) / r0
	}
	return (pos.Entry - last) / r0
}

// ROIUnrealized 计算未实现ROI（含杠杆的浮盈百分比）
// 多空对称：
//   - LONG: roi = ((last - entry) / entry) * leverage
//   - SHORT: roi = ((entry - last) / entry) * leverage
func ROIUnrealized(pos PositionState, last float64) float64 {
	if pos.Entry <= 0 || pos.Leverage <= 0 {
		return 0
	}
	raw := (last - pos.Entry) / pos.Entry
	if pos.Side == Short {
		raw = (pos.Entry - last) / pos.Entry
	}
	return raw * pos.Leverage
}

// CostBps 计算总成本（bps）
func CostBps(f FeeModel) float64 {
	return f.TakerFeeBps + f.SlippageBpsMinor + f.FundingBps
}

// BEWithCosts 计算Break-even价格（含成本）
// 多空对称：
//   - LONG: BE = entry * (1 + cost_bps/10000) + stop_distance_pad
//   - SHORT: BE = entry * (1 - cost_bps/10000) - stop_distance_pad
func BEWithCosts(pos PositionState, cfg Config, m MarketSnapshot, f FeeModel) (float64, error) {
	if m.TickSize <= 0 {
		return 0, fmt.Errorf("tick_size missing")
	}
	cost := CostBps(f) / 10000.0
	pad := float64(cfg.StopDistanceMinTicks) * m.TickSize

	if pos.Side == Long {
		return pos.Entry*(1.0+cost) + pad, nil
	}
	return pos.Entry*(1.0-cost) - pad, nil
}

// ProfitFloor 计算盈利地板价格（V-21.2：修复SHORT盈利地板计算）
// 🔥 V-21.2修复：使用基于R0的计算方式，使LONG和SHORT逻辑统一
//
// 盈利地板的含义：止损可以移动到的目标位置，用于锁定盈利
//   - LONG: 止损上移到 entry + (R0 * floor_pct)
//   - SHORT: 止损下移到 entry + (R0 * floor_pct)
//
// 为什么使用R0而不是entry的百分比？
//   1. R0是初始风险，floor_pct表示相对于初始风险的倍数
//   2. 这样LONG和SHORT可以使用相同的配置值
//   3. 盈利地板会随ROI增加而逐渐接近entry
//
// 示例（SHORT）：
//   - Entry: 127.17, InitStop: 128.9, R0: 1.73
//   - 12% ROI时，floor_pct=0.08
//   - 盈利地板 = 127.17 + (1.73 * 0.08) = 127.31
//   - 候选止损 = min(128.9, 127.31) = 127.31（下移1.59，锁定盈利）
func ProfitFloor(pos PositionState, cfg Config, be float64, currentROI float64) float64 {
	// V-21.0: 如果配置了StopLossMilestones，使用阶梯止损
	floorPct := 0.0
	if cfg.StopLossMilestones != nil && len(cfg.StopLossMilestones) > 0 {
		// 找到满足条件的最高ROI阶梯
		highestROI := 0.0
		highestStopPct := 0.0
		for roiThreshold, stopPct := range cfg.StopLossMilestones {
			if currentROI >= roiThreshold && roiThreshold > highestROI {
				highestROI = roiThreshold
				highestStopPct = stopPct
			}
		}
		floorPct = highestStopPct
	} else {
		// 回退到旧逻辑：使用FloorPriceBps
		floorPct = cfg.FloorPriceBps / 10000.0
	}

	// 🔥 V-21.2: 计算R0（初始风险单位）
	var r0 float64
	if pos.Side == Long {
		r0 = pos.Entry - pos.InitStop
	} else {
		r0 = pos.InitStop - pos.Entry
	}

	// 如果R0无效，回退到基于entry的计算
	if r0 <= 0 {
		if pos.Side == Long {
			floorPx := pos.Entry * (1.0 + floorPct)
			if be > floorPx {
				return be
			}
			return floorPx
		}
		// SHORT回退逻辑
		floorPx := pos.Entry * (1.0 + floorPct)
		if be > floorPx {
			return be
		}
		return floorPx
	}

	// 🔥 V-21.5修复：基于ROI百分比锁盈，而不是R0倍数
	// 用户期望：ROI 5%时锁住3%的ROI利润
	// 正确公式：floor = entry ± (entry * floorPct / leverage)
	//   - LONG: 止损上移，盈利地板 = entry + (entry * floorPct / leverage)
	//   - SHORT: 止损下移，盈利地板 = entry - (entry * floorPct / leverage)
	var floorPx float64
	if pos.Side == Long {
		// LONG: 锁住floorPct的ROI
		floorPx = pos.Entry + (pos.Entry * floorPct / pos.Leverage)
	} else {
		// SHORT: 锁住floorPct的ROI
		floorPx = pos.Entry - (pos.Entry * floorPct / pos.Leverage)
	}

	// 🔥 V-21.4修复：SHORT移除BE限制，LONG保持BE保护
	// LONG: BE作为上限保护，防止止损设得太高（太接近当前价）
	// SHORT: 移除BE限制，让floor自由下移，实现渐进式锁盈
	if pos.Side == Long {
		// LONG: 如果BE > floor，返回BE（更保守）
		if be > floorPx {
			return be
		}
	}
	// SHORT: 直接返回floor，不受BE限制
	return floorPx
}
