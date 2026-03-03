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
		return 0 // 不能直接定向，由 computeDivergentBias 二次推导
	default:
		// 未知枚举值安全回退
		return 0
	}
}

// DominantDirMap 将 dominant_direction 映射到方向符号 [-1, 0, +1]
// 直接描述市场主导力量方向
func DominantDirMap(s string) float64 {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "bearish") || strings.Contains(s, "distribution") ||
		strings.Contains(s, "decline") || strings.Contains(s, "selling"):
		return -1
	case strings.Contains(s, "bullish") || strings.Contains(s, "accumulation") ||
		strings.Contains(s, "rally") || strings.Contains(s, "buying"):
		return 1
	default:
		return 0
	}
}

// computeDivergentBias 在 trend_alignment=divergent_mixed 时推导方向偏置
// 综合 dominant_direction 和现货/期货 CVD 量比，给出 [-1, 0, +1] 的方向倾向
// 返回非零值表示有明确偏置（调用方应乘以 0.5 降级），返回 0 表示真正无法判断
func computeDivergentBias(m Macro) float64 {
	dominantSign := DominantDirMap(m.DominantDirection)

	// 计算现货主导程度：|spot_cvd| / (|spot_cvd| + |futures_cvd|)
	spotAbs := abs(m.SpotCvd1hUSD)
	futAbs := abs(m.FuturesCvd1hUSD)
	total := spotAbs + futAbs

	if total == 0 {
		// 无 CVD 数据，只靠 dominant_direction
		return dominantSign
	}

	// 现货方向（spot CVD 的符号）
	spotDir := 0.0
	if m.SpotCvd1hUSD > 0 {
		spotDir = 1.0
	} else if m.SpotCvd1hUSD < 0 {
		spotDir = -1.0
	}

	spotWeight := spotAbs / total

	// 当现货绝对主导（>70%）时：以现货方向为准，并验证与 dominant_direction 一致
	if spotWeight > 0.70 {
		if dominantSign != 0 && sign(dominantSign) == sign(spotDir) {
			return spotDir // 两者一致，偏置确认
		}
		if dominantSign == 0 {
			return spotDir // dominant 未知时，以现货方向为准
		}
		// 现货方向与 dominant_direction 矛盾，信号冲突，回退到 0
		return 0
	}

	// 现货未占绝对主导时（50-70%），仅 dominant_direction 可靠
	return dominantSign
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
// 注意：fake_pump/fake_dump 已改为方向感知型（INTENT_PUMP_WARN），不再是硬红线
// 只有 data_insufficient 是真正的硬红线（数据不足，无法判断方向）
func IsRedlineIntent(norm string) bool {
	return norm == "data_insufficient"
}
