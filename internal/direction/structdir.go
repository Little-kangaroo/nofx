package direction

import "strings"

// StructDirFromMTF 从多时间框架分析计算结构方向 [-1, +1]
// 使用通道指标作为主要信号（15m/30m），4h 超级趋势作为背景约束上限
// 权重：30m(60%) > 15m(40%)
func StructDirFromMTF(mtf MTFAnalysis) float64 {
	// 时间框架权重配置（只使用 15m 和 30m 计算主信号）
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

	sum = clamp(sum, -1, 1)

	// ── 4h 超级趋势背景约束 ───────────────────────────────────────────
	// 当 4h 超级趋势与短线信号方向相反时，将短线信号幅度上限压缩到 ±0.5
	// 防止在高阶空头/多头趋势中，短线 break_up/break_down 产生误导性满分信号
	if h4, ok := mtf["4h"]; ok {
		st := strings.ToLower(h4.SupertrendDir)
		switch st {
		case "bearish":
			// 4h 空头背景：多头信号最大只能到 +0.5
			if sum > 0.5 {
				sum = 0.5
			}
		case "bullish":
			// 4h 多头背景：空头信号最大只能到 -0.5
			if sum < -0.5 {
				sum = -0.5
			}
		}
	}

	return sum
}

// channelSign 通道方向符号 [-1, +1]
// 修复：break_up/break_down 根据通道方向差异化打分，防止牛市陷阱
//
// 核心逻辑：
//   - 顺势突破（up+break_up / down+break_down）：满分，确认趋势延续
//   - 横盘突破（flat+break_up / flat+break_down）：60%，方向未确认
//   - 逆势突破（down+break_up / up+break_down）：30%，大概率熊市反弹/牛市回踩
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
		if dir == "up" {
			return 1.0 // 顺势突破：满分
		}
		if dir == "flat" || dir == "sideways" || dir == "" {
			return 0.6 // 横盘突破：方向待确认
		}
		return 0.3 // 下降通道逆势 break_up：大概率熊市反弹

	case "breakdown":
		if dir == "down" {
			return -1.0 // 顺势跌破：满分
		}
		if dir == "flat" || dir == "sideways" || dir == "" {
			return -0.6 // 横盘跌破：方向待确认
		}
		return -0.3 // 上升通道逆势 break_down：大概率牛市回踩

	case "lower":
		if dir == "up" {
			return 1.0 // 上升通道下轨：强看涨
		} else if dir == "down" {
			return -0.5 // 下降通道下轨：弱看跌
		}
		return 0.0
	case "upper":
		if dir == "up" {
			return 0.5 // 上升通道上轨：弱看涨
		} else if dir == "down" {
			return -1.0 // 下降通道上轨：强看跌
		}
		return 0.0
	case "inside":
		if dir == "up" {
			// priceRatio: 0=下轨, 1=上轨
			if priceRatio < 0.3 {
				return 1.0 // 接近下轨，强看涨
			} else if priceRatio > 0.7 {
				return 0.5 // 接近上轨，弱看涨
			}
			return 0.8
		} else if dir == "down" {
			if priceRatio > 0.7 {
				return -1.0 // 接近上轨，强看跌
			} else if priceRatio < 0.3 {
				return -0.5 // 接近下轨，弱看跌
			}
			return -0.8
		}
		return baseSign * 0.5
	default:
		return baseSign * 0.5
	}
}

// vpvrTieBreak VPVR tie-break 逻辑
// 更靠近 VAL（价值区下沿）-> 看涨 +0.10
// 更靠近 VAH（价值区上沿）-> 看跌 -0.10
func vpvrTieBreak(v VPVR) float64 {
	nearVAH := 1.0 / (1.0 + abs(v.DistToVAHATR))
	nearVAL := 1.0 / (1.0 + abs(v.DistToVALATR))

	if nearVAL > nearVAH {
		return 0.10
	}
	return -0.10
}
