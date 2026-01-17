package direction

import "math"

// ========== 工具函数 ==========

// clamp 将值限制在 [lo, hi] 范围内
func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// tanh 双曲正切函数
func tanh(x float64) float64 {
	return math.Tanh(x)
}

// abs 绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// max 返回两个数中的较大值
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// sign 返回数值的符号 (+1, 0, -1)
func sign(x float64) int {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}

// signSide 返回方向的符号
func signSide(s DirectionSide) int {
	if s == SideLong {
		return 1
	}
	if s == SideShort {
		return -1
	}
	return 0
}

// min3 返回三个数中的最小值
func min3(a, b, c float64) float64 {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// ========== 盘口特征计算 ==========

// PressureSign 计算盘口压力符号 [-1, +1]
// bid_pressure 高 -> 正值（买压）
// ask_pressure 高 -> 负值（卖压）
func PressureSign(bid, ask float64) float64 {
	s := bid + ask
	if s <= 0 {
		return 0
	}
	return clamp((bid-ask)/s, -1, 1)
}

// OBSign 计算盘口方向符号 [-1, +1]
// 综合考虑压力和不平衡，并用流动性评分加权
func OBSign(of Orderflow) float64 {
	pressure := PressureSign(of.OB.BidPressure, of.OB.AskPressure)
	imb := tanh(of.OB.ImbalanceRatio / 0.50)
	liqF := clamp(of.OB.LiquidityScore, 0, 1)
	return liqF * clamp(0.60*pressure+0.40*imb, -1, 1)
}

// ========== 墙体特征计算 ==========

// wallEff 计算单个墙体的效应强度
func wallEff(w *Wall) float64 {
	if w == nil {
		return 0
	}
	// 距离因子：越近效应越强
	near := clamp(1-w.DistancePct/0.30, 0, 1)

	// 稳固性因子
	solid := 1.0
	if !w.IsSolid {
		solid *= 0.6
	}
	solid *= clamp(w.StabilityScore/0.60, 0, 1)
	solid *= clamp(1-float64(w.FlickerCount)/30.0, 0, 1)

	return w.StrengthUSD * near * solid
}

// WallSign 计算墙体方向符号 [-1, +1]
// 支撑墙 -> 正值（看涨）
// 阻力墙 -> 负值（看跌）
func WallSign(of Orderflow, cfg Config, flags *[]string) float64 {
	sup := wallEff(of.OB.SupportWall)
	res := wallEff(of.OB.ResistanceWall)

	// 标记显著的墙体
	if res > sup*1.2 {
		*flags = append(*flags, "WALL_RESIST")
	}
	if sup > res*1.2 {
		*flags = append(*flags, "WALL_SUPPORT")
	}

	return tanh((sup - res) / cfg.WallScaleUSD)
}

// ========== 订单流质量评分 ==========

// OfQuality 计算订单流质量评分 [0, 1]
// 综合考虑整体评分、可靠性、延迟、微观质量
// 并根据流动性、墙体闪烁、欺骗风险进行降级
func OfQuality(of Orderflow, cfg Config, flags *[]string) float64 {
	// 基础评分
	base := clamp(of.Quality.OverallScore, 0, 1)

	// 可靠性评分（取三者最小值）
	reliab := clamp(min3(
		of.Quality.CvdReliability,
		of.Quality.OIReliability,
		of.Quality.OrderbookReliability,
	), 0, 1)

	// 延迟因子
	denom := max(1, float64(of.Quality.UpdateIntervalS)*1000*1.2)
	lagF := clamp(1-float64(of.Quality.DataLagMs)/denom, 0, 1)

	// 微观质量因子
	microF := clamp(of.Micro5m.DataQuality, 0, 1)

	// 加权合成质量评分
	q := 0.45*base + 0.20*reliab + 0.20*lagF + 0.15*microF

	// 降级因子
	if of.OB.LiquidityScore < cfg.LiqThin {
		*flags = append(*flags, "LIQ_THIN")
		q *= 0.75
	}
	if of.OB.WallChange5m > cfg.WallFlickerN {
		*flags = append(*flags, "WALL_FLICKER")
		q *= 0.85
	}
	if of.OB.SpoofingRisk > cfg.SpoofSoft {
		*flags = append(*flags, "SPOOF_MED")
		q *= 0.80
	}

	return clamp(q, 0, 1)
}
