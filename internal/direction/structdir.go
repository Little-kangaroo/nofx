package direction

import "strings"

// StructDirFromMTF 从多时间框架分析计算结构方向 [-1, +1]
// 🔥 优化：使用通道指标替代 SuperTrend + 道氏理论
// 权重：30m(60%) > 15m(40%)
// 只使用 15m 和 30m 时间框架，提高反应速度
func StructDirFromMTF(mtf MTFAnalysis) float64 {
	// 时间框架权重配置（只使用 15m 和 30m）
	weights := map[string]float64{
		"30m": 0.60,
		"15m": 0.40,
	}

	sum := 0.0
	for tf, w := range weights {
		t, ok := mtf[tf]
		if !ok {
			continue
		}

		// 通道方向符号
		chSign := channelSign(t.Channel.Direction, t.Channel.CurrentPosition, t.Channel.PriceRatio, t.Channel.Quality)

		// 通道质量因子：质量越高权重越大
		qualityF := clamp(t.Channel.Quality, 0.3, 1.0)

		// 加权累加
		sum += w * chSign * qualityF
	}

	// VPVR tie-break：仅在方向不明确时使用
	if abs(sum) < 0.15 {
		if t, ok := mtf["15m"]; ok {
			sum += vpvrTieBreak(t.VPVR)
		}
	}

	return clamp(sum, -1, 1)
}

// channelSign 通道方向符号 [-1, +1]
// 综合考虑通道方向、价格位置、突破情况、通道质量
func channelSign(direction, position string, priceRatio, quality float64) float64 {
	dir := strings.ToLower(direction)
	pos := strings.ToLower(position)

	// 基础方向符号
	var baseSign float64
	switch dir {
	case "up":
		baseSign = 1.0
	case "down":
		baseSign = -1.0
	case "sideways":
		baseSign = 0.0
	default:
		baseSign = 0.0
	}

	// 根据价格位置调整信号强度
	switch pos {
	case "breakup":
		// 向上突破：检查质量
		if quality < 0.50 {
			return 0.30 // 低质量通道，突破不可靠
		}
		return 1.0
	case "breakdown":
		// 向下突破：检查质量
		if quality < 0.50 {
			return -0.30 // 低质量通道，突破不可靠
		}
		return -1.0
	case "inside":
		// 在通道内：根据位置调整
		if dir == "up" {
			// 上升通道：价格越接近下轨越看涨（回调买入机会）
			// priceRatio: 0=下轨, 1=上轨
			if priceRatio < 0.3 {
				return 1.0 * quality // 接近下轨，强看涨
			} else if priceRatio > 0.7 {
				return 0.5 * quality // 接近上轨，弱看涨（可能回调）
			}
			return 0.8 * quality // 中间位置，正常看涨
		} else if dir == "down" {
			// 下降通道：价格越接近上轨越看跌（反弹卖出机会）
			if priceRatio > 0.7 {
				return -1.0 * quality // 接近上轨，强看跌
			} else if priceRatio < 0.3 {
				return -0.3 * quality // 接近下轨，弱看跌（反弹风险）
			}
			return -0.8 * quality // 中间位置，正常看跌
		} else {
			// 横盘通道
			if priceRatio > 0.7 {
				return -0.3 // 接近上沿，轻微看跌
			} else if priceRatio < 0.3 {
				return 0.3 // 接近下沿，轻微看涨
			}
			return 0
		}
	case "lower":
		// 在通道下部
		if dir == "down" {
			// 下降通道的下部，反弹风险高
			return -0.2 * quality
		} else if dir == "up" {
			// 上升通道的下部，买入机会
			return 0.9 * quality
		}
		return 0.2 // 横盘通道下部，轻微看涨
	case "upper":
		// 在通道上部
		if dir == "up" {
			// 上升通道的上部，回调风险
			return 0.4 * quality
		} else if dir == "down" {
			// 下降通道的上部，卖出机会
			return -0.9 * quality
		}
		return -0.2 // 横盘通道上部，轻微看跌
	case "middle":
		// 在通道中部，跟随通道方向但降级
		return baseSign * 0.6 * quality
	default:
		return baseSign * 0.5
	}
}

// vpvrTieBreak VPVR tie-break 逻辑
// 更靠近 VAL（价值区下沿）-> 看涨 +0.10
// 更靠近 VAH（价值区上沿）-> 看跌 -0.10
func vpvrTieBreak(v VPVR) float64 {
	// 计算到 VAH 和 VAL 的接近度（距离越小越接近）
	nearVAH := 1.0 / (1.0 + abs(v.DistToVAHATR))
	nearVAL := 1.0 / (1.0 + abs(v.DistToVALATR))

	if nearVAL > nearVAH {
		// 更靠近 VAL，看涨
		return 0.10
	}
	// 更靠近 VAH，看跌
	return -0.10
}
