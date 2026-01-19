package market

import "math"

// TimeframeMillis 将时间框架字符串转换为毫秒数
// 返回值：(毫秒数, 是否有效)
func TimeframeMillis(tf string) (int64, bool) {
	switch tf {
	case "1m":
		return 1 * 60 * 1000, true
	case "3m":
		return 3 * 60 * 1000, true
	case "5m":
		return 5 * 60 * 1000, true
	case "15m":
		return 15 * 60 * 1000, true
	case "30m":
		return 30 * 60 * 1000, true
	case "1h":
		return 60 * 60 * 1000, true
	case "2h":
		return 2 * 60 * 60 * 1000, true
	case "4h":
		return 4 * 60 * 60 * 1000, true
	case "6h":
		return 6 * 60 * 60 * 1000, true
	case "12h":
		return 12 * 60 * 60 * 1000, true
	case "1d":
		return 24 * 60 * 60 * 1000, true
	default:
		return 0, false
	}
}

// inferTimeframeEnhanced 增强版时间框架推断（使用最近邻匹配）
// 修复P1-02: 避免30m被误判为1h
func inferTimeframeEnhanced(klines []Kline) string {
	if len(klines) < 2 {
		return "unknown"
	}

	// 计算K线间隔（毫秒）
	intervalMs := klines[1].OpenTime - klines[0].OpenTime
	mins := float64(intervalMs) / 60000.0

	// 候选时间框架及其分钟数
	candidates := []struct {
		tf string
		m  float64
	}{
		{"1m", 1},
		{"3m", 3},
		{"5m", 5},
		{"15m", 15},
		{"30m", 30},
		{"1h", 60},
		{"2h", 120},
		{"4h", 240},
		{"6h", 360},
		{"12h", 720},
		{"1d", 1440},
	}

	// 找最近邻
	best := "unknown"
	bestDiff := math.Inf(1)
	for _, c := range candidates {
		diff := math.Abs(mins - c.m)
		if diff < bestDiff {
			bestDiff = diff
			best = c.tf
		}
	}

	// 容忍误差：最多2分钟偏差
	if bestDiff > 2.0 {
		return "unknown"
	}

	return best
}

// safeFloatForJSON 确保float64值可以安全序列化为JSON
// 修复P0-02: 防止NaN/Inf导致JSON序列化失败
func safeFloatForJSON(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0.0
	}
	return v
}
