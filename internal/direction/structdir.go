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
		chSign := channelSign(t.Channel.Direction, t.Channel.CurrentPosition, t.Channel.PriceRatio)

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
// 综合考虑通道方向、价格位置、突破情况
func channelSign(direction, position string, priceRatio float64) float64 {
	dir := strings.ToLower(direction)
	// 标准化 position：去除下划线，兼容 "break_up"/"breakup"、"break_down"/"breakdown" 等格式
	pos := strings.ToLower(strings.ReplaceAll(position, "_", ""))

	// 基础方向符号
	var baseSign float64
	switch dir {
	case "up":
		baseSign = 1.0
	case "down":
		baseSign = -1.0
	case "sideways", "flat":
		baseSign = 0.0
	default:
		baseSign = 0.0
	}

	// 根据价格位置调整信号强度
	switch pos {
	case "breakup":
		// 向上突破：强看涨
		return 1.0
	case "breakdown":
		// 向下突破：强看跌
		return -1.0
	case "lower":
		// 处于通道下轨附近：与 inside 接近下轨逻辑一致
		if dir == "up" {
			return 1.0 // 上升通道下轨：强看涨
		} else if dir == "down" {
			return -0.5 // 下降通道下轨：弱看跌
		}
		return 0.0
	case "upper":
		// 处于通道上轨附近
		if dir == "up" {
			return 0.5 // 上升通道上轨：弱看涨
		} else if dir == "down" {
			return -1.0 // 下降通道上轨：强看跌
		}
		return 0.0
	case "inside":
		// 在通道内：根据位置调整
		if dir == "up" {
			// 上升通道：价格越接近下轨越看涨（回调买入机会）
			// priceRatio: 0=下轨, 1=上轨
			if priceRatio < 0.3 {
				return 1.0 // 接近下轨，强看涨
			} else if priceRatio > 0.7 {
				return 0.5 // 接近上轨，弱看涨（可能回调）
			}
			return 0.8 // 中间位置，正常看涨
		} else if dir == "down" {
			// 下降通道：价格越接近上轨越看跌（反弹卖出机会）
			if priceRatio > 0.7 {
				return -1.0 // 接近上轨，强看跌
			} else if priceRatio < 0.3 {
				return -0.5 // 接近下轨，弱看跌（可能反弹）
			}
			return -0.8 // 中间位置，正常看跌
		}
		return baseSign * 0.5 // 横盘通道，信号较弱
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
