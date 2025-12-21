package triggers

const (
	eps = 0.0001 // 小量，用于价格比较容差
)

// DetectSfpBear 检测看空SFP（Sweep Failure Pattern - Bearish）
// k: 当前K线
// swingHigh: Swing高点价格
// atr5m: 5分钟ATR
// volZ: 量能Z分数（可选，<0表示不使用）
// cfg: 配置
// 返回: (质量评分, 是否触发)
func DetectSfpBear(k Kline, swingHigh float64, atr5m float64, volZ float64, cfg TriggerConfig) (float64, bool) {
	// 前置检查
	if swingHigh <= 0 || atr5m <= 0 {
		return 0, false
	}

	range_ := k.High - k.Low
	if range_ <= eps {
		return 0, false
	}

	// 1. 扫高：k.high > swingHigh
	if k.High <= swingHigh+eps {
		return 0, false
	}

	// 2. 收回：k.close < swingHigh
	if k.Close >= swingHigh {
		return 0, false
	}

	// 3. 上影线占比
	upperWick := k.High - max(k.Open, k.Close)
	upperWickRatio := safeDiv(upperWick, range_)
	if upperWickRatio < cfg.WickRatioMin {
		return 0, false
	}

	// 4. 扫高幅度（ATR归一）
	sweepAtr := safeDiv(k.High-swingHigh, atr5m)
	if sweepAtr < cfg.SweepAtrMin {
		return 0, false
	}

	// 5. 可选：量能过滤（如果提供了volZ且>=0）
	if volZ >= 0 && volZ < cfg.VolZMin {
		// 量能不足，降低质量分但不完全否决
		// 这里选择继续，但在质量评分中体现
	}

	// === 质量评分计算 ===
	// 子项1: Wick Score (权重0.45)
	wickScore := normalizeScore(upperWickRatio, cfg.WickRatioMin, cfg.WickRatioMax)

	// 子项2: Sweep Score (权重0.35)
	sweepScore := normalizeScore(sweepAtr, cfg.SweepAtrMin, cfg.SweepAtrMax)

	// 子项3: Close Back Score (权重0.20) - 收盘离Swing High的"收回程度"
	closeBackDist := swingHigh - k.Close
	closeBackAtr := safeDiv(closeBackDist, atr5m)
	// 收回距离越大，分数越高（最大按1.0 ATR归一）
	closeBackScore := normalizeScore(closeBackAtr, 0, 1.0)

	// 总分
	totalScore := 0.45*wickScore + 0.35*sweepScore + 0.20*closeBackScore

	// 量能加成/惩罚（可选）
	if volZ >= 0 {
		if volZ >= cfg.VolZMin*1.5 {
			// 量能强，加成5%
			totalScore = min(1.0, totalScore*1.05)
		} else if volZ < cfg.VolZMin {
			// 量能弱，惩罚10%
			totalScore *= 0.90
		}
	}

	return clamp(totalScore, 0, 1), true
}

// DetectSfpBull 检测看多SFP（Sweep Failure Pattern - Bullish）
// k: 当前K线
// swingLow: Swing低点价格
// atr5m: 5分钟ATR
// volZ: 量能Z分数（可选，<0表示不使用）
// cfg: 配置
// 返回: (质量评分, 是否触发)
func DetectSfpBull(k Kline, swingLow float64, atr5m float64, volZ float64, cfg TriggerConfig) (float64, bool) {
	// 前置检查
	if swingLow <= 0 || atr5m <= 0 {
		return 0, false
	}

	range_ := k.High - k.Low
	if range_ <= eps {
		return 0, false
	}

	// 1. 扫低：k.low < swingLow
	if k.Low >= swingLow-eps {
		return 0, false
	}

	// 2. 收回：k.close > swingLow
	if k.Close <= swingLow {
		return 0, false
	}

	// 3. 下影线占比
	lowerWick := min(k.Open, k.Close) - k.Low
	lowerWickRatio := safeDiv(lowerWick, range_)
	if lowerWickRatio < cfg.WickRatioMin {
		return 0, false
	}

	// 4. 扫低幅度（ATR归一）
	sweepAtr := safeDiv(swingLow-k.Low, atr5m)
	if sweepAtr < cfg.SweepAtrMin {
		return 0, false
	}

	// 5. 可选：量能过滤
	if volZ >= 0 && volZ < cfg.VolZMin {
		// 量能不足，降低质量分但不完全否决
	}

	// === 质量评分计算 ===
	// 子项1: Wick Score (权重0.45)
	wickScore := normalizeScore(lowerWickRatio, cfg.WickRatioMin, cfg.WickRatioMax)

	// 子项2: Sweep Score (权重0.35)
	sweepScore := normalizeScore(sweepAtr, cfg.SweepAtrMin, cfg.SweepAtrMax)

	// 子项3: Close Back Score (权重0.20) - 收盘离Swing Low的"收回程度"
	closeBackDist := k.Close - swingLow
	closeBackAtr := safeDiv(closeBackDist, atr5m)
	closeBackScore := normalizeScore(closeBackAtr, 0, 1.0)

	// 总分
	totalScore := 0.45*wickScore + 0.35*sweepScore + 0.20*closeBackScore

	// 量能加成/惩罚（可选）
	if volZ >= 0 {
		if volZ >= cfg.VolZMin*1.5 {
			totalScore = min(1.0, totalScore*1.05)
		} else if volZ < cfg.VolZMin {
			totalScore *= 0.90
		}
	}

	return clamp(totalScore, 0, 1), true
}
