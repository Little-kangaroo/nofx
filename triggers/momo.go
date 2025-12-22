package triggers

// DetectMomoBull 检测看多动能点火（Momentum Ignition - Bullish）
// k: 当前K线
// atr5m: 5分钟ATR
// volZ: 量能Z分数（可选，<0表示不使用）
// cfg: 配置
// 返回: (质量评分, 是否触发)
func DetectMomoBull(k Kline, atr5m float64, volZ float64, cfg TriggerConfig) (float64, bool) {
	// 前置检查
	if atr5m <= 0 {
		return 0, false
	}

	range_ := k.High - k.Low
	if range_ <= eps {
		return 0, false
	}

	// 1. 波幅放大（ATR归一）
	rangeAtr := safeDiv(range_, atr5m)
	if rangeAtr < cfg.RangeAtrMin {
		return 0, false
	}

	// 2. 量能放大（可选）
	// 🔥 P1-01修复：仅在 volZ>=0 时启用量能门槛，volZ<0 表示不可用/不使用
	if volZ >= 0 && volZ < cfg.VolZMin {
		return 0, false
	}

	// 3. 收盘靠近高点
	// (k.high - k.close) / range_ <= CloseNearExtreme
	closeToHighRatio := safeDiv(k.High-k.Close, range_)
	if closeToHighRatio > cfg.CloseNearExtreme {
		return 0, false
	}

	// === 质量评分计算 ===
	// 子项1: Range Score (权重0.45)
	rangeScore := normalizeScore(rangeAtr, cfg.RangeAtrMin, cfg.RangeAtrMax)

	// 子项2: Volume Score (权重0.35)
	// 🔥 P1-01修复：处理 volZ<0 的情况（不可用时给中性分0.5）
	volScore := 0.5
	if volZ >= 0 {
		volScore = normalizeScore(volZ, cfg.VolZMin, cfg.VolZMin*3.0) // 假设最大3倍
	}

	// 子项3: Close Position Score (权重0.20)
	// closeToHighRatio越小（越靠近高点），分数越高
	closePosScore := 1.0 - normalizeScore(closeToHighRatio, 0, cfg.CloseNearExtreme)

	// 总分
	totalScore := 0.45*rangeScore + 0.35*volScore + 0.20*closePosScore

	return clamp(totalScore, 0, 1), true
}

// DetectMomoBear 检测看空动能点火（Momentum Ignition - Bearish）
// k: 当前K线
// atr5m: 5分钟ATR
// volZ: 量能Z分数（可选，<0表示不使用）
// cfg: 配置
// 返回: (质量评分, 是否触发)
func DetectMomoBear(k Kline, atr5m float64, volZ float64, cfg TriggerConfig) (float64, bool) {
	// 前置检查
	if atr5m <= 0 {
		return 0, false
	}

	range_ := k.High - k.Low
	if range_ <= eps {
		return 0, false
	}

	// 1. 波幅放大（ATR归一）
	rangeAtr := safeDiv(range_, atr5m)
	if rangeAtr < cfg.RangeAtrMin {
		return 0, false
	}

	// 2. 量能放大（可选）
	// 🔥 P1-01修复：仅在 volZ>=0 时启用量能门槛，volZ<0 表示不可用/不使用
	if volZ >= 0 && volZ < cfg.VolZMin {
		return 0, false
	}

	// 3. 收盘靠近低点
	// (k.close - k.low) / range_ <= CloseNearExtreme
	closeToLowRatio := safeDiv(k.Close-k.Low, range_)
	if closeToLowRatio > cfg.CloseNearExtreme {
		return 0, false
	}

	// === 质量评分计算 ===
	// 子项1: Range Score (权重0.45)
	rangeScore := normalizeScore(rangeAtr, cfg.RangeAtrMin, cfg.RangeAtrMax)

	// 子项2: Volume Score (权重0.35)
	// 🔥 P1-01修复：处理 volZ<0 的情况（不可用时给中性分0.5）
	volScore := 0.5
	if volZ >= 0 {
		volScore = normalizeScore(volZ, cfg.VolZMin, cfg.VolZMin*3.0)
	}

	// 子项3: Close Position Score (权重0.20)
	closePosScore := 1.0 - normalizeScore(closeToLowRatio, 0, cfg.CloseNearExtreme)

	// 总分
	totalScore := 0.45*rangeScore + 0.35*volScore + 0.20*closePosScore

	return clamp(totalScore, 0, 1), true
}
