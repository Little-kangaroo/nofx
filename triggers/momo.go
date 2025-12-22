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

	// 🔥 P0-1修复：移除量能hard-fail，改为软惩罚/加成
	// （MOMO触发成功仅依赖价格行为，量能作为质量调节因子）

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
	// 🔥 P0-1修复：volZ作为加权因子，不再是veto条件
	// - volZ < 0：不可用，给中性分0.5
	// - volZ >= 0：根据实际值计算
	volScore := 0.5
	if volZ >= 0 {
		volScore = normalizeScore(volZ, cfg.VolZMin, cfg.VolZMin*3.0) // 假设最大3倍
	}

	// 子项3: Close Position Score (权重0.20)
	// closeToHighRatio越小（越靠近高点），分数越高
	closePosScore := 1.0 - normalizeScore(closeToHighRatio, 0, cfg.CloseNearExtreme)

	// 总分
	totalScore := 0.45*rangeScore + 0.35*volScore + 0.20*closePosScore

	// 🔥 P0-1修复：量能软惩罚/加成机制（应用于最终总分）
	if volZ >= 0 {
		if volZ >= cfg.VolZMin*1.5 {
			// 高量能加成
			totalScore = min(1.0, totalScore*1.05)
		} else if volZ < cfg.VolZMin {
			// 低量能惩罚
			totalScore *= 0.90
		}
		// cfg.VolZMin <= volZ < cfg.VolZMin*1.5：保持原分数，不惩罚不加成
	}
	// volZ < 0（不可用）：不惩罚不加成

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

	// 🔥 P0-1修复：移除量能hard-fail，改为软惩罚/加成
	// （MOMO触发成功仅依赖价格行为，量能作为质量调节因子）

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
	// 🔥 P0-1修复：volZ作为加权因子，不再是veto条件
	// - volZ < 0：不可用，给中性分0.5
	// - volZ >= 0：根据实际值计算
	volScore := 0.5
	if volZ >= 0 {
		volScore = normalizeScore(volZ, cfg.VolZMin, cfg.VolZMin*3.0)
	}

	// 子项3: Close Position Score (权重0.20)
	closePosScore := 1.0 - normalizeScore(closeToLowRatio, 0, cfg.CloseNearExtreme)

	// 总分
	totalScore := 0.45*rangeScore + 0.35*volScore + 0.20*closePosScore

	// 🔥 P0-1修复：量能软惩罚/加成机制（应用于最终总分）
	if volZ >= 0 {
		if volZ >= cfg.VolZMin*1.5 {
			// 高量能加成
			totalScore = min(1.0, totalScore*1.05)
		} else if volZ < cfg.VolZMin {
			// 低量能惩罚
			totalScore *= 0.90
		}
		// cfg.VolZMin <= volZ < cfg.VolZMin*1.5：保持原分数，不惩罚不加成
	}
	// volZ < 0（不可用）：不惩罚不加成

	return clamp(totalScore, 0, 1), true
}
