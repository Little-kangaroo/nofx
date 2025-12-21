package triggers

import "math"

// clamp 限制值在[lo, hi]范围内
func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// max 返回两个float64的较大值
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// min 返回两个float64的较小值
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// abs 返回绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// safeDiv 安全除法，避免除零
func safeDiv(numerator, denominator float64) float64 {
	if denominator == 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		return 0
	}
	result := numerator / denominator
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0
	}
	return result
}

// normalizeScore 将分数归一化到[0, 1]区间
// value: 当前值
// minVal: 对应0分的最小值
// maxVal: 对应1分的最大值
func normalizeScore(value, minVal, maxVal float64) float64 {
	if maxVal <= minVal {
		return 0
	}
	return clamp((value-minVal)/(maxVal-minVal), 0, 1)
}

// uniqueStrings 去重字符串数组
func uniqueStrings(arr []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// CalculateATR 计算ATR（Average True Range）
// klines: K线数据
// period: ATR周期（建议14）
func CalculateATR(klines []Kline, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}

	trueRanges := make([]float64, 0, len(klines)-1)
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr := max(high-low, max(abs(high-prevClose), abs(low-prevClose)))
		trueRanges = append(trueRanges, tr)
	}

	// 计算ATR（使用EMA方式）
	if len(trueRanges) < period {
		return 0
	}

	// 初始ATR为前period个TR的平均值
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += trueRanges[i]
	}
	atr := sum / float64(period)

	// 后续使用EMA平滑
	multiplier := 1.0 / float64(period)
	for i := period; i < len(trueRanges); i++ {
		atr = (trueRanges[i] * multiplier) + (atr * (1 - multiplier))
	}

	return atr
}

// CalculateVolumeZScore 计算量能Z-Score
// currentVolume: 当前成交量
// klines: 历史K线（用于计算均值和标准差）
// lookback: 回看周期（建议20）
func CalculateVolumeZScore(currentVolume float64, klines []Kline, lookback int) float64 {
	if len(klines) < lookback {
		return 0
	}

	// 取最近lookback根K线计算统计量
	startIdx := len(klines) - lookback
	volumes := make([]float64, 0, lookback)
	for i := startIdx; i < len(klines); i++ {
		volumes = append(volumes, klines[i].Volume)
	}

	// 计算均值
	sum := 0.0
	for _, v := range volumes {
		sum += v
	}
	mean := sum / float64(len(volumes))

	// 计算标准差
	variance := 0.0
	for _, v := range volumes {
		diff := v - mean
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(volumes)))

	// 计算Z-Score
	if stdDev == 0 {
		return 0
	}
	return (currentVolume - mean) / stdDev
}
