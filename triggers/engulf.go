package triggers

// DetectEngulfBear 检测看空吞没（Bearish Engulfing）
// k: 当前K线
// prev: 前一根K线
// atr5m: 5分钟ATR
// cfg: 配置
// 返回: (质量评分, 是否触发)
func DetectEngulfBear(k Kline, prev Kline, atr5m float64, cfg TriggerConfig) (float64, bool) {
	// 前置检查
	if atr5m <= 0 {
		return 0, false
	}

	range_ := k.High - k.Low
	if range_ <= eps {
		return 0, false
	}

	// 1. 当前为阴线
	if k.Close >= k.Open {
		return 0, false
	}

	// 2. 实体吞没前一根实体（保守定义）
	// k.open >= prev.close && k.close <= prev.open
	if k.Open < prev.Close-eps {
		return 0, false
	}
	if k.Close > prev.Open+eps {
		return 0, false
	}

	// 3. 实体大小足够（ATR归一）
	bodySize := abs(k.Close - k.Open)
	bodyAtr := safeDiv(bodySize, atr5m)
	if bodyAtr < cfg.BodyAtrMin {
		return 0, false
	}

	// 4. 收盘位置偏向下侧（可选强化）
	closePos := safeDiv(k.Close-k.Low, range_)
	if closePos > cfg.ClosePosMax {
		// 收盘不够靠近低点，降低质量但不完全否决
	}

	// === 质量评分计算 ===
	// 子项1: Body Score (权重0.60)
	bodyScore := normalizeScore(bodyAtr, cfg.BodyAtrMin, cfg.BodyAtrMax)

	// 子项2: Close Position Score (权重0.20)
	// closePos越小（越靠近低点），分数越高
	closePosScore := 1.0 - normalizeScore(closePos, 0, cfg.ClosePosMax)

	// 子项3: Overlap Score (权重0.20)
	// 计算实体覆盖比例（吞没程度）
	prevBody := abs(prev.Close - prev.Open)
	if prevBody < eps {
		prevBody = eps // 避免除零
	}
	overlapSize := min(k.Open, prev.Close) - max(k.Close, prev.Open)
	if overlapSize < 0 {
		overlapSize = 0 // 无覆盖
	}
	overlapRatio := safeDiv(overlapSize, prevBody)
	overlapScore := clamp(overlapRatio, 0, 1) // 覆盖越多，分数越高

	// 总分
	totalScore := 0.60*bodyScore + 0.20*closePosScore + 0.20*overlapScore

	return clamp(totalScore, 0, 1), true
}

// DetectEngulfBull 检测看多吞没（Bullish Engulfing）
// k: 当前K线
// prev: 前一根K线
// atr5m: 5分钟ATR
// cfg: 配置
// 返回: (质量评分, 是否触发)
func DetectEngulfBull(k Kline, prev Kline, atr5m float64, cfg TriggerConfig) (float64, bool) {
	// 前置检查
	if atr5m <= 0 {
		return 0, false
	}

	range_ := k.High - k.Low
	if range_ <= eps {
		return 0, false
	}

	// 1. 当前为阳线
	if k.Close <= k.Open {
		return 0, false
	}

	// 2. 实体吞没前一根实体（保守定义）
	// k.open <= prev.close && k.close >= prev.open
	if k.Open > prev.Close+eps {
		return 0, false
	}
	if k.Close < prev.Open-eps {
		return 0, false
	}

	// 3. 实体大小足够（ATR归一）
	bodySize := abs(k.Close - k.Open)
	bodyAtr := safeDiv(bodySize, atr5m)
	if bodyAtr < cfg.BodyAtrMin {
		return 0, false
	}

	// 4. 收盘位置偏向上侧（可选强化）
	closePos := safeDiv(k.High-k.Close, range_)
	if closePos > cfg.ClosePosMax {
		// 收盘不够靠近高点，降低质量但不完全否决
	}

	// === 质量评分计算 ===
	// 子项1: Body Score (权重0.60)
	bodyScore := normalizeScore(bodyAtr, cfg.BodyAtrMin, cfg.BodyAtrMax)

	// 子项2: Close Position Score (权重0.20)
	// closePos越小（越靠近高点），分数越高
	closePosScore := 1.0 - normalizeScore(closePos, 0, cfg.ClosePosMax)

	// 子项3: Overlap Score (权重0.20)
	prevBody := abs(prev.Close - prev.Open)
	if prevBody < eps {
		prevBody = eps
	}
	overlapSize := min(k.Close, prev.Open) - max(k.Open, prev.Close)
	if overlapSize < 0 {
		overlapSize = 0
	}
	overlapRatio := safeDiv(overlapSize, prevBody)
	overlapScore := clamp(overlapRatio, 0, 1)

	// 总分
	totalScore := 0.60*bodyScore + 0.20*closePosScore + 0.20*overlapScore

	return clamp(totalScore, 0, 1), true
}
