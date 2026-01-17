package direction

import "strings"

// AlignMap 将 trend_alignment 映射到方向符号 [-1, 0, +1]
// +1: 看涨对齐, -1: 看跌对齐, 0: 分歧混合或未知
func AlignMap(s string) float64 {
	s = strings.ToLower(s)
	switch s {
	case "bullish_aligned", "bullish", "spot_led_rally":
		return 1
	case "bearish_aligned", "bearish", "distribution_phase":
		return -1
	case "divergent_mixed":
		return 0
	default:
		// 未知枚举值安全回退
		return 0
	}
}

// NormalizeIntent 规范化 candle_intent 字符串
// 覆盖所有可能的别名和变体，确保健壮性
func NormalizeIntent(raw string) string {
	s := strings.ToLower(raw)
	switch {
	case strings.Contains(s, "data_insufficient"):
		return "data_insufficient"
	case strings.Contains(s, "fake_pump") || strings.Contains(s, "fake_dump") || strings.Contains(s, "retail"):
		return "fake"
	case strings.Contains(s, "futures_leading_bullish") || strings.Contains(s, "bullish") || strings.Contains(s, "accumulation"):
		return "bullish_confirm"
	case strings.Contains(s, "futures_leading_bearish") || strings.Contains(s, "bearish") || strings.Contains(s, "distribution"):
		return "bearish_confirm"
	case strings.Contains(s, "absorption"):
		return "absorption"
	default:
		// 未知枚举值安全回退
		return "mixed"
	}
}

// IntentMap 将规范化后的 intent 映射到方向强度 [-1, +1]
func IntentMap(norm string) float64 {
	switch norm {
	case "bullish_confirm":
		return 0.70
	case "bearish_confirm":
		return -0.70
	case "absorption":
		// 吸收偏向突破路径，非纯多
		return 0.35
	default:
		// mixed/fake/data_insufficient 返回 0
		// fake 和 data_insufficient 会在其他地方触发红线
		return 0.0
	}
}

// IsRedlineIntent 判断是否为硬红线 intent（禁止新开仓）
func IsRedlineIntent(norm string) bool {
	return norm == "fake" || norm == "data_insufficient"
}
