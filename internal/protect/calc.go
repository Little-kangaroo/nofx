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

// ProfitFloor 计算盈利地板价格（至少锁住floor_price_bps利润）
// 多空对称：
//   - LONG: floor = max(BE, entry * (1 + floor_bps/10000))
//   - SHORT: floor = min(BE, entry * (1 - floor_bps/10000))
func ProfitFloor(pos PositionState, cfg Config, be float64) float64 {
	floorRate := cfg.FloorPriceBps / 10000.0

	if pos.Side == Long {
		floorPx := pos.Entry * (1.0 + floorRate)
		if be > floorPx {
			return be
		}
		return floorPx
	}

	// SHORT
	floorPx := pos.Entry * (1.0 - floorRate)
	if be < floorPx {
		return be
	}
	return floorPx
}
