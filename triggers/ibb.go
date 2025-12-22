package triggers

// DetectIbbBull 检测看多IBB（Inside Bar Breakout - Bullish）
// k: 当前K线
// prev: 前一根K线（可能是inside bar）
// prev2: 前前一根K线（母K线）
// atr5m: 5分钟ATR
// volZ: 量能Z分数（可选，<0表示不使用）
// cfg: 配置
// 返回: (质量评分, 是否触发)
func DetectIbbBull(k Kline, prev Kline, prev2 Kline, atr5m float64, volZ float64, cfg TriggerConfig) (float64, bool) {
	// 前置检查
	if atr5m <= 0 {
		return 0, false
	}

	range_ := k.High - k.Low
	if range_ <= eps {
		return 0, false
	}

	// 1. 前一根K线必须是inside bar（相对于prev2）
	// prev.high < prev2.high && prev.low > prev2.low
	if prev.High >= prev2.High-eps {
		return 0, false
	}
	if prev.Low <= prev2.Low+eps {
		return 0, false
	}

	// 2. 当前K线向上突破prev.high
	if k.Close <= prev.High {
		return 0, false
	}

	// 3. 突破幅度（ATR归一）
	breakDist := k.Close - prev.High
	breakAtr := safeDiv(breakDist, atr5m)
	if breakAtr < cfg.BreakAtrMin {
		return 0, false
	}

	// 4. 可选：量能确认
	if volZ >= 0 && volZ < cfg.VolZMin {
		// 量能不足，降低质量分但不完全否决
	}

	// === 质量评分计算 ===
	// 子项1: Break Score (权重0.45)
	breakScore := normalizeScore(breakAtr, cfg.BreakAtrMin, cfg.BreakAtrMax)

	// 子项2: Range Score (权重0.35) - 当前K线波幅
	rangeAtr := safeDiv(range_, atr5m)
	rangeScore := normalizeScore(rangeAtr, 0.5, 2.5) // 假设合理范围0.5~2.5 ATR

	// 子项3: Close Position Score (权重0.20) - 收盘靠近高点
	// 🔥 P0-修复：clamp 防止 closePos 超出范围导致负分
	closePos := safeDiv(k.High-k.Close, range_)
	closePosScore := clamp(1.0-normalizeScore(closePos, 0, 0.3), 0, 1)

	// 总分
	totalScore := 0.45*breakScore + 0.35*rangeScore + 0.20*closePosScore

	// 量能加成/惩罚
	if volZ >= 0 {
		if volZ >= cfg.VolZMin*1.5 {
			totalScore = min(1.0, totalScore*1.05)
		} else if volZ < cfg.VolZMin {
			totalScore *= 0.90
		}
	}

	return clamp(totalScore, 0, 1), true
}

// DetectIbbBear 检测看空IBB（Inside Bar Breakout - Bearish）
// k: 当前K线
// prev: 前一根K线（可能是inside bar）
// prev2: 前前一根K线（母K线）
// atr5m: 5分钟ATR
// volZ: 量能Z分数（可选，<0表示不使用）
// cfg: 配置
// 返回: (质量评分, 是否触发)
func DetectIbbBear(k Kline, prev Kline, prev2 Kline, atr5m float64, volZ float64, cfg TriggerConfig) (float64, bool) {
	// 前置检查
	if atr5m <= 0 {
		return 0, false
	}

	range_ := k.High - k.Low
	if range_ <= eps {
		return 0, false
	}

	// 1. 前一根K线必须是inside bar（相对于prev2）
	if prev.High >= prev2.High-eps {
		return 0, false
	}
	if prev.Low <= prev2.Low+eps {
		return 0, false
	}

	// 2. 当前K线向下突破prev.low
	if k.Close >= prev.Low {
		return 0, false
	}

	// 3. 突破幅度（ATR归一）
	breakDist := prev.Low - k.Close
	breakAtr := safeDiv(breakDist, atr5m)
	if breakAtr < cfg.BreakAtrMin {
		return 0, false
	}

	// 4. 可选：量能确认
	if volZ >= 0 && volZ < cfg.VolZMin {
		// 量能不足，降低质量分但不完全否决
	}

	// === 质量评分计算 ===
	// 子项1: Break Score (权重0.45)
	breakScore := normalizeScore(breakAtr, cfg.BreakAtrMin, cfg.BreakAtrMax)

	// 子项2: Range Score (权重0.35)
	rangeAtr := safeDiv(range_, atr5m)
	rangeScore := normalizeScore(rangeAtr, 0.5, 2.5)

	// 子项3: Close Position Score (权重0.20) - 收盘靠近低点
	// 🔥 P0-修复：clamp 防止 closePos 超出范围导致负分
	closePos := safeDiv(k.Close-k.Low, range_)
	closePosScore := clamp(1.0-normalizeScore(closePos, 0, 0.3), 0, 1)

	// 总分
	totalScore := 0.45*breakScore + 0.35*rangeScore + 0.20*closePosScore

	// 量能加成/惩罚
	if volZ >= 0 {
		if volZ >= cfg.VolZMin*1.5 {
			totalScore = min(1.0, totalScore*1.05)
		} else if volZ < cfg.VolZMin {
			totalScore *= 0.90
		}
	}

	return clamp(totalScore, 0, 1), true
}
