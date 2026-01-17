package direction

import "strings"

// StructDirFromMTF 从多时间框架分析计算结构方向 [-1, +1]
// 权重：4h(45%) > 1h(30%) > 30m(15%) > 15m(10%)
// 5m 不参与方向背景计算，交给触发器处理
func StructDirFromMTF(mtf MTFAnalysis) float64 {
	// 时间框架权重配置
	weights := map[string]float64{
		"4h":  0.45,
		"1h":  0.30,
		"30m": 0.15,
		"15m": 0.10,
	}

	sum := 0.0
	for tf, w := range weights {
		t, ok := mtf[tf]
		if !ok {
			continue
		}

		// 超级趋势方向符号
		st := stSign(t.SuperTrend.Direction)
		// 道氏理论方向符号
		dow := dowSign(t.Dow.TrendDirection)

		// 综合方向：超级趋势权重 60%，道氏理论权重 40%
		tfSign := 0.60*float64(st) + 0.40*float64(dow)

		// 趋势强度因子：强度越高权重越大
		strengthF := clamp(t.Dow.TrendStrength/80.0, 0.2, 1.0)

		// 加权累加
		sum += w * tfSign * strengthF
	}

	// VPVR tie-break：仅在方向不明确时使用
	if abs(sum) < 0.15 {
		if t, ok := mtf["15m"]; ok {
			sum += vpvrTieBreak(t.VPVR)
		}
	}

	return clamp(sum, -1, 1)
}

// stSign 超级趋势方向符号
// bullish -> +1, bearish -> -1
func stSign(dir string) int {
	d := strings.ToLower(dir)
	if d == "bullish" {
		return 1
	}
	return -1
}

// dowSign 道氏理论方向符号
// up -> +1, down -> -1, sideways -> 0
func dowSign(d string) int {
	s := strings.ToLower(d)
	if s == "up" {
		return 1
	}
	if s == "down" {
		return -1
	}
	return 0
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
