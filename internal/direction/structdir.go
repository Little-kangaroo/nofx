package direction

import "strings"

// StructDirFromMTF 从多时间框架分析计算结构方向 [-1, +1]
// 信号融合策略：
//   - 通道方向（channel）+ 超级趋势（supertrend）协同计算每个时间框架的结构信号
//   - 两者方向一致时复合增强；通道平坦时以超级趋势为主；通道与超级趋势反向时信任通道
//   - 4h 超级趋势保持原有背景约束（防止短线逆高阶趋势满分）
//
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

		// 超级趋势方向符号
		stSign := supertrendToSign(t.SupertrendDir)

		// 通道质量因子：质量越高权重越大
		qualityF := clamp(t.Channel.Quality, 0.3, 1.0)

		// 信号融合：根据通道与超级趋势的关系选择混合策略
		var signal float64
		var effectiveQualityF float64
		switch {
		case chSign == 0:
			// 通道无方向（flat/sideways + inside 等）
			if t.Channel.Quality <= 0.0 {
				// quality=0：通道完全无数据（channel_width_pct=0，sideways+inside）
				// 旧逻辑：signal=0，丢弃所有结构信号，导致在全超级趋势多头时 struct_dir 退化为
				// VPVR 噪声（-0.10），系统反而逆势开空。
				// 新逻辑：用超级趋势方向作为兜底，信号强度 0.50（低于通道正常 0.60），
				// effectiveQualityF 设为 0.50 反映数据不完整性。
				// 效果：SOL/XRP 全 ST 多头时 struct_dir 从 -0.10 提升到 +0.25，
				// 配合 SUPERTREND_CONSENSUS_ADJUST 使系统自然持中而非逆势开空。
				signal = stSign * 0.50
				effectiveQualityF = 0.50
			} else {
				// quality>0：通道方向不明但有质量数据，以超级趋势为参考并按质量折扣
				signal = stSign * 0.60
				effectiveQualityF = qualityF
			}
		case sign(chSign) == sign(stSign):
			// 通道与超级趋势方向一致：复合增强
			// 例：flat+break_up(+0.60) + bullish → 0.60×0.75 + 1.0×0.25 = 0.70
			signal = chSign*0.75 + stSign*0.25
			effectiveQualityF = qualityF
		default:
			// 通道与超级趋势方向相反（如牛市中通道跌破，或熊市中横盘向上突破）
			// 信任通道——这是有效的 LTF 短线逆势信号，不应被滞后的超级趋势抵消
			// 例：flat+break_down(-0.60) + bullish ST → signal = -0.60（空信号保留）
			// 例：up+breakdown(-0.30) + bullish ST → signal = -0.30（回调空保留）
			signal = chSign
			effectiveQualityF = qualityF
		}

		sum += w * signal * effectiveQualityF
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

// supertrendToSign 将超级趋势方向字符串转换为方向符号 [-1, 0, +1]
func supertrendToSign(dir string) float64 {
	switch strings.ToLower(dir) {
	case "bullish":
		return 1.0
	case "bearish":
		return -1.0
	default:
		return 0.0
	}
}

// SupertrendConsensus 计算多时间框架超级趋势共识度 [-1, +1]
// 统计 15m/30m/4h 三个时间框架的超级趋势方向，返回加权平均共识：
//   +1.0 = 全部看多（3/3 bullish）
//   +0.67 = 多数看多（2/3 bullish）
//    0.0 = 中立或均等
//   -0.67 = 多数看空（2/3 bearish）
//   -1.0 = 全部看空（3/3 bearish）
func SupertrendConsensus(mtf MTFAnalysis) float64 {
	tfs := []string{"15m", "30m", "4h"}
	totalSign := 0.0
	count := 0.0
	for _, tf := range tfs {
		t, ok := mtf[tf]
		if !ok || t.SupertrendDir == "" {
			continue
		}
		totalSign += supertrendToSign(t.SupertrendDir)
		count++
	}
	if count == 0 {
		return 0
	}
	return totalSign / count
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
