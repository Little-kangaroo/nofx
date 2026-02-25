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

// RUnrealized 计算未实现R倍数
//   - LONG: R_unr = (last - entry) / R0
//   - SHORT: R_unr = (entry - last) / R0
func RUnrealized(pos PositionState, last float64, r0 float64) float64 {
	if pos.Side == Long {
		return (last - pos.Entry) / r0
	}
	return (pos.Entry - last) / r0
}

// ROIUnrealized 计算未实现ROI（含杠杆）
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

// ProfitFloor 根据当前ROI查阶梯表，计算止损目标位置
//
// 计算公式（不依赖TickSize，不依赖BE）：
//   - LONG:  floor = Entry + R0 × floorPct
//   - SHORT: floor = Entry - R0 × floorPct
//
// 示例（LONG，Entry=100000，R0=5000）：
//   - 12% ROI → floorPct=0.08 → floor = 100000 + 5000×0.08 = 100400
func ProfitFloor(pos PositionState, cfg Config, currentROI float64) float64 {
	floorPct := 0.0
	if len(cfg.StopLossMilestones) > 0 {
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
		floorPct = cfg.FloorPriceBps / 10000.0
	}

	var r0 float64
	if pos.Side == Long {
		r0 = pos.Entry - pos.InitStop
	} else {
		r0 = pos.InitStop - pos.Entry
	}
	if r0 <= 0 {
		return 0
	}

	if pos.Side == Long {
		return pos.Entry + r0*floorPct
	}
	return pos.Entry - r0*floorPct
}

// FloorToTick 向下取整到最近的tick（用于格式化报价，不影响逻辑判断）
func FloorToTick(x, tick float64) float64 {
	if tick <= 0 {
		return x
	}
	return float64(int64(x/tick)) * tick
}

// CeilToTick 向上取整到最近的tick（用于格式化报价，不影响逻辑判断）
func CeilToTick(x, tick float64) float64 {
	if tick <= 0 {
		return x
	}
	floor := float64(int64(x/tick)) * tick
	if floor < x {
		return floor + tick
	}
	return floor
}
