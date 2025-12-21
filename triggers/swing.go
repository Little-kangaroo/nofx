package triggers

// FindSwingLevels 查找Swing High和Swing Low
// klines: K线数据（至少需要 SwingLookback + SwingLeft + SwingRight 根）
// cfg: 触发器配置
// 返回: SwingLevels结构，包含最近的Swing高低点信息
func FindSwingLevels(klines []Kline, cfg TriggerConfig) SwingLevels {
	result := SwingLevels{
		SwingHigh:      0,
		SwingLow:       0,
		SwingHighIndex: -1,
		SwingLowIndex:  -1,
		SwingHighTime:  0,
		SwingLowTime:   0,
	}

	if len(klines) < cfg.SwingLeft+cfg.SwingRight+1 {
		return result
	}

	// 从最近的K线向前扫描，查找Swing点
	// 限制扫描范围为最近 SwingLookback 根K线
	scanStart := 0
	if len(klines) > cfg.SwingLookback {
		scanStart = len(klines) - cfg.SwingLookback
	}

	// 扫描Swing High（局部高点）
	// 需要确保左侧和右侧都有足够的K线用于确认
	for i := len(klines) - cfg.SwingRight - 1; i >= scanStart+cfg.SwingLeft; i-- {
		isSwingHigh := true
		currentHigh := klines[i].High

		// 检查左侧
		for j := 1; j <= cfg.SwingLeft; j++ {
			if i-j < 0 || klines[i-j].High >= currentHigh {
				isSwingHigh = false
				break
			}
		}

		// 检查右侧
		if isSwingHigh {
			for j := 1; j <= cfg.SwingRight; j++ {
				if i+j >= len(klines) || klines[i+j].High > currentHigh {
					isSwingHigh = false
					break
				}
			}
		}

		// 找到Swing High
		if isSwingHigh {
			result.SwingHigh = currentHigh
			result.SwingHighIndex = i
			result.SwingHighTime = klines[i].CloseTime
			break
		}
	}

	// 扫描Swing Low（局部低点）
	for i := len(klines) - cfg.SwingRight - 1; i >= scanStart+cfg.SwingLeft; i-- {
		isSwingLow := true
		currentLow := klines[i].Low

		// 检查左侧
		for j := 1; j <= cfg.SwingLeft; j++ {
			if i-j < 0 || klines[i-j].Low <= currentLow {
				isSwingLow = false
				break
			}
		}

		// 检查右侧
		if isSwingLow {
			for j := 1; j <= cfg.SwingRight; j++ {
				if i+j >= len(klines) || klines[i+j].Low < currentLow {
					isSwingLow = false
					break
				}
			}
		}

		// 找到Swing Low
		if isSwingLow {
			result.SwingLow = currentLow
			result.SwingLowIndex = i
			result.SwingLowTime = klines[i].CloseTime
			break
		}
	}

	return result
}

// FindSwingHigh 仅查找Swing High（如果只需要高点）
func FindSwingHigh(klines []Kline, cfg TriggerConfig) (price float64, index int, timestamp int64) {
	levels := FindSwingLevels(klines, cfg)
	return levels.SwingHigh, levels.SwingHighIndex, levels.SwingHighTime
}

// FindSwingLow 仅查找Swing Low（如果只需要低点）
func FindSwingLow(klines []Kline, cfg TriggerConfig) (price float64, index int, timestamp int64) {
	levels := FindSwingLevels(klines, cfg)
	return levels.SwingLow, levels.SwingLowIndex, levels.SwingLowTime
}
