package direction

import "strings"

// isOFStale 判断订单流数据是否过期/不可用（P0 短路条件）
func isOFStale(of Orderflow, cfg Config, flags *[]string) bool {
	// 状态检查
	if of.Quality.Status != "正常" {
		*flags = append(*flags, "OF_STATUS_BAD")
		return true
	}

	// 整体评分检查
	if of.Quality.OverallScore < cfg.MinOverallScore {
		*flags = append(*flags, "OF_SCORE_LOW")
		return true
	}

	// 微观数据质量检查
	if of.Micro5m.DataQuality < cfg.MinMicroDQ {
		*flags = append(*flags, "OF_MICRO_DQ_LOW")
		return true
	}

	// 盘口数据缺失检查
	if of.OB.LiquidityScore <= 0 {
		*flags = append(*flags, "OF_LIQ_ZERO")
		return true
	}
	if of.OB.AskPressure == 0 && of.OB.BidPressure == 0 {
		*flags = append(*flags, "OF_PRESSURE_ZERO")
		return true
	}

	return false
}

// hasValidChannelData 检查是否有足够的有效通道数据
// 🔥 新增：防止通道数据缺失导致结构方向失效
func hasValidChannelData(mtf MTFAnalysis, flags *[]string) bool {
	// 检查关键时间框架（15m, 30m, 1h）是否有有效通道数据
	criticalTFs := []string{"15m", "30m", "1h"}
	validCount := 0
	missingTFs := []string{}

	for _, tf := range criticalTFs {
		if t, ok := mtf[tf]; ok {
			// 通道方向不为空，且质量>0
			if t.Channel.Direction != "" && t.Channel.Quality > 0 {
				validCount++
			} else {
				missingTFs = append(missingTFs, tf)
			}
		} else {
			missingTFs = append(missingTFs, tf)
		}
	}

	// 至少需要2个关键时间框架有有效通道数据
	if validCount < 2 {
		*flags = append(*flags, "CHANNEL_DATA_INSUFFICIENT")
		return false
	}

	return true
}

// StrongCounterexample 判断是否满足强反例条件（宏观反向时的豁免条件）
// 需要满足"三取二"：
// 1) 吸收提示或 CVD 分歧
// 2) 处于 VPVR 边缘（15m）
// 3) 墙体强且欺骗风险低
func StrongCounterexample(in RootSymbolInput, wallSign float64, flags []string) bool {
	score := 0

	// 条件 1：吸收提示或 CVD 分歧
	for _, f := range flags {
		if f == "ABSORPTION_HINT" || f == "CVD_DIVERGENCE" {
			score++
			break
		}
	}

	// 条件 2：VPVR 边缘（15m 时间框架）
	if t, ok := in.MTF["15m"]; ok {
		nearVAH := 1.0 / (1.0 + abs(t.VPVR.DistToVAHATR))
		nearVAL := 1.0 / (1.0 + abs(t.VPVR.DistToVALATR))
		if max(nearVAH, nearVAL) > 0.25 {
			score++
		}
	}

	// 条件 3：墙体强且欺骗风险低
	if abs(wallSign) > 0.40 && in.Orderflow.OB.SpoofingRisk <= 0.60 {
		score++
	}

	// 三取二
	return score >= 2
}

// ComputeDirectionArbitration 计算方向裁决（SSOT 主函数）
// 这是生产级 SSOT 的唯一入口，完全确定性，不依赖 LLM
func ComputeDirectionArbitration(in RootSymbolInput, cfg Config) DirectionArbitration {
	var flags []string
	dbg := map[string]float64{}

	// ========== Step A: OF_STALE 短路 ==========
	if isOFStale(in.Orderflow, cfg, &flags) {
		flags = append(flags, "OF_STALE")
		return DirectionArbitration{
			PlanSide:    SideUnknown,
			BlockEntry:  true,
			BlockReason: "OF_STALE",
			Confidence:  0,
			Delta:       0,
			OFDir:       0,
			StructDir:   0,
			OFQuality:   0,
			Flags:       flags,
		}
	}

	// ========== Step A2: 通道数据质量检查（修改）==========
	// 🔥 修复：通道数据缺失时不应该完全阻止交易，而是降级为纯订单流模式
	hasValidChannel := hasValidChannelData(in.MTF, &flags)
	if !hasValidChannel {
		flags = append(flags, "CHANNEL_DATA_WEAK")
		// 不再直接返回，而是继续计算，但结构方向权重会降低
	}

	// ========== Step A: Redline 标记（只设置 block_entry，不阻止计算）==========
	blockEntry := false
	blockReason := ""

	// Intent 红线检查
	intentNorm := NormalizeIntent(in.Orderflow.Micro5m.CandleIntent)
	if IsRedlineIntent(intentNorm) {
		blockEntry = true
		blockReason = "INTENT_REDLINE"
		flags = append(flags, "INTENT_REDLINE")
	}

	// 欺骗风险红线检查
	if in.Orderflow.OB.SpoofingRisk > cfg.SpoofHard {
		blockEntry = true
		blockReason = "SPOOF_HIGH"
		flags = append(flags, "SPOOF_HIGH")
	}

	// ========== Step B: 计算订单流质量 ==========
	ofQ := OfQuality(in.Orderflow, cfg, &flags)

	// ========== Step B: 宏观资金趋势 ==========
	macroAlign := AlignMap(in.Orderflow.Macro.TrendAlignment)
	macroStrength := clamp(in.Orderflow.Macro.SignalStrength/100.0, 0, 1) *
		clamp(in.Orderflow.Macro.ConfidenceLevel, 0, 1)
	macroSign := macroAlign * macroStrength

	// divergent_mixed 特殊处理：只降级不否决
	if strings.ToLower(in.Orderflow.Macro.TrendAlignment) == "divergent_mixed" {
		flags = append(flags, "DIVERGENT_MIXED")
		// macroAlign==0 => macroSign==0
	}

	// ========== Step B: 微观博弈（5m）==========
	intentSign := IntentMap(intentNorm)

	// CVD 符号
	cvdRaw := in.Orderflow.Micro5m.FuturesCvdDelta + 0.60*in.Orderflow.Micro5m.SpotCvdDelta
	cvdScale := abs(in.Orderflow.Micro5m.VolumeDelta)
	if cvdScale < cfg.CVDFallbackScale {
		cvdScale = cfg.CVDFallbackScale
	}
	cvdSign := tanh(cvdRaw / cvdScale)

	// OI 符号
	oiSign := tanh(in.Orderflow.Micro5m.OIDeltaPct / cfg.OIScalePct)

	// 吸收提示
	if sign(in.Orderflow.Micro5m.PriceDeltaPct) != sign(cvdRaw) && abs(cvdSign) > 0.50 {
		flags = append(flags, "ABSORPTION_HINT")
	}
	if in.Orderflow.Macro.CvdDivergence {
		flags = append(flags, "CVD_DIVERGENCE")
	}

	// ========== Step B: 盘口结构 ==========
	obSign := OBSign(in.Orderflow)
	wallSign := WallSign(in.Orderflow, cfg, &flags)

	// ========== Step B: 结构背景方向 ==========
	structDir := StructDirFromMTF(in.MTF)

	// ========== Step C: 订单流合成方向（主导方向）==========
	// 🔥 优化：提高 CVD 和订单簿权重，降低宏观权重
	ofDir := clamp(
		0.15*macroSign+      // 宏观趋势：从 30% 降低到 15%
			0.15*intentSign+ // 蜡烛意图：保持 15%
			0.30*cvdSign+    // CVD：从 20% 提高到 30%
			0.10*oiSign+     // OI：保持 10%
			0.20*obSign+     // 订单簿：从 15% 提高到 20%
			0.10*wallSign,   // 墙：保持 10%
		-1, 1,
	)

	// ========== Step C: 动态权重（订单流权重更高）==========
	// 🔥 修复：当通道数据缺失时，完全依赖订单流
	wOF := cfg.WOFMin + (cfg.WOFMax-cfg.WOFMin)*ofQ
	wST := 1 - wOF

	// 如果通道数据质量不足，降低结构权重，提高订单流权重
	if !hasValidChannel {
		wOF = 0.95 // 订单流权重提升到95%
		wST = 0.05 // 结构权重降低到5%
	}

	// ========== Step C: 双边评分 ==========
	scoreLong := wOF*max(ofDir, 0) + wST*max(structDir, 0)
	scoreShort := wOF*max(-ofDir, 0) + wST*max(-structDir, 0)

	delta := scoreLong - scoreShort
	conf := clamp(abs(delta)/max(scoreLong+scoreShort, 1e-9), 0, 1)

	// ========== Step C: 冲突折扣 ==========
	if sign(ofDir) != 0 && sign(structDir) != 0 &&
		sign(ofDir) != sign(structDir) &&
		abs(ofDir) > 0.60 && abs(structDir) > 0.60 {
		flags = append(flags, "DIR_CONFLICT")
		conf *= 0.70
	}

	// ========== Step C: 方向裁决 ==========
	side := SideNeutral
	if abs(delta) >= cfg.ThetaNeutral && conf >= cfg.MinConf {
		if delta > 0 {
			side = SideLong
		} else {
			side = SideShort
		}
	}

	// ========== Step D: 宏观反向阻断（对应 Gate1 语义）==========
	if macroAlign != 0 && side != SideNeutral &&
		sign(macroAlign) != signSide(side) && macroStrength > 0.60 {
		// 检查是否满足强反例条件
		if !StrongCounterexample(in, wallSign, flags) {
			blockEntry = true
			blockReason = "MACRO_OPPOSE"
			flags = append(flags, "MACRO_OPPOSE_BLOCK")
		} else {
			// 满足强反例，放行但降级
			flags = append(flags, "MACRO_OPPOSE_BUT_EXCEPT")
			conf *= 0.75
		}
	}

	// ========== Step D2: 多时间框架flat+break_down检查 ==========
	if side == SideShort {
		flatBreakdownCount := 0
		breakdownTFs := []string{}

		for _, tf := range []string{"15m", "30m", "1h"} {
			if t, ok := in.MTF[tf]; ok {
				dir := strings.ToLower(t.Channel.Direction)
				pos := strings.ToLower(t.Channel.CurrentPosition)
				quality := t.Channel.Quality

				// 只统计高质量通道
				if quality >= 0.40 {
					if (dir == "flat" || dir == "sideways") && pos == "breakdown" {
						flatBreakdownCount++
						breakdownTFs = append(breakdownTFs, tf)
					}
				}
			}
		}

		// 如果>=2个时间框架都是flat + break_down
		if flatBreakdownCount >= 2 {
			blockEntry = true
			blockReason = "MULTI_TF_FLAT_BREAKDOWN:" + strings.Join(breakdownTFs, "+")
			flags = append(flags, "FLAT_BREAKDOWN_CLUSTER")
		}
	} else if side == SideLong {
		// 做多时：检查flat + breakup
		flatBreakupCount := 0
		breakupTFs := []string{}

		for _, tf := range []string{"15m", "30m", "1h"} {
			if t, ok := in.MTF[tf]; ok {
				dir := strings.ToLower(t.Channel.Direction)
				pos := strings.ToLower(t.Channel.CurrentPosition)
				quality := t.Channel.Quality

				if quality >= 0.40 {
					if (dir == "flat" || dir == "sideways") && pos == "breakup" {
						flatBreakupCount++
						breakupTFs = append(breakupTFs, tf)
					}
				}
			}
		}

		if flatBreakupCount >= 2 {
			blockEntry = true
			blockReason = "MULTI_TF_FLAT_BREAKUP:" + strings.Join(breakupTFs, "+")
			flags = append(flags, "FLAT_BREAKUP_CLUSTER")
		}
	}

	// ========== Step D2.5: 反向突破拦截（新增）==========
	// 🔥 修复BTCUSDT案例：做空时检查是否有多个时间框架向上突破
	if side == SideShort {
		reverseBreakupCount := 0
		reverseBreakupTFs := []string{}

		// 检查所有时间框架（包括4h）
		for _, tf := range []string{"15m", "30m", "1h", "4h"} {
			if t, ok := in.MTF[tf]; ok {
				pos := strings.ToLower(t.Channel.CurrentPosition)
				quality := t.Channel.Quality

				// 高质量通道的反向突破（兼容 breakup 和 break_up）
				if quality >= 0.40 && (pos == "breakup" || pos == "break_up") {
					reverseBreakupCount++
					reverseBreakupTFs = append(reverseBreakupTFs, tf)
				}
			}
		}

		// 如果>=2个时间框架出现反向突破，拦截
		if reverseBreakupCount >= 2 {
			blockEntry = true
			blockReason = "REVERSE_BREAKUP_CLUSTER:" + strings.Join(reverseBreakupTFs, "+")
			flags = append(flags, "REVERSE_BREAKUP_VETO")
		}
	} else if side == SideLong {
		// 做多时检查反向breakdown
		reverseBreakdownCount := 0
		reverseBreakdownTFs := []string{}

		for _, tf := range []string{"15m", "30m", "1h", "4h"} {
			if t, ok := in.MTF[tf]; ok {
				pos := strings.ToLower(t.Channel.CurrentPosition)
				quality := t.Channel.Quality

				// 兼容 breakdown 和 break_down
				if quality >= 0.40 && (pos == "breakdown" || pos == "break_down") {
					reverseBreakdownCount++
					reverseBreakdownTFs = append(reverseBreakdownTFs, tf)
				}
			}
		}

		if reverseBreakdownCount >= 2 {
			blockEntry = true
			blockReason = "REVERSE_BREAKDOWN_CLUSTER:" + strings.Join(reverseBreakdownTFs, "+")
			flags = append(flags, "REVERSE_BREAKDOWN_VETO")
		}
	}

	// ========== Step D3: 1h矛盾信号检查 ==========
	if t1h, ok := in.MTF["1h"]; ok {
		dir1h := strings.ToLower(t1h.Channel.Direction)
		pos1h := strings.ToLower(t1h.Channel.CurrentPosition)
		quality1h := t1h.Channel.Quality

		// 只有高质量通道才信任
		if quality1h >= 0.50 {
			// 判断做空，但1h向上突破（排除flat通道的弱信号）
			if side == SideShort && pos1h == "breakup" && dir1h != "flat" && dir1h != "sideways" {
				blockEntry = true
				blockReason = "1H_BREAKUP_CONFLICT"
				flags = append(flags, "1H_BREAKUP_VETO")
			}
			// 判断做多，但1h向下突破（排除flat通道的弱信号）
			if side == SideLong && pos1h == "breakdown" && dir1h != "flat" && dir1h != "sideways" {
				blockEntry = true
				blockReason = "1H_BREAKDOWN_CONFLICT"
				flags = append(flags, "1H_BREAKDOWN_VETO")
			}
		}
	}

	// ========== 调试信息 ==========
	dbg["macro_sign"] = macroSign
	dbg["intent_sign"] = intentSign
	dbg["cvd_sign"] = cvdSign
	dbg["oi_sign"] = oiSign
	dbg["ob_sign"] = obSign
	dbg["wall_sign"] = wallSign

	return DirectionArbitration{
		PlanSide:    side,
		BlockEntry:  blockEntry,
		BlockReason: blockReason,
		Confidence:  conf,
		Delta:       delta,
		OFDir:       ofDir,
		StructDir:   structDir,
		OFQuality:   ofQ,
		Flags:       flags,
		Debug:       dbg,
	}
}
