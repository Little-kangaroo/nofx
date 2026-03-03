package direction

import "strings"

// ========== EDGE触发器语义说明 ==========
// 🔥 重要：EDGE触发器不预设交易方向，只标记结构测试事件
//
// EDGE_BULL: 价格触碰支撑位（结构测试事件）
//   - 在下降趋势中：可能是支撑失效前的最后测试 → 观望或做空
//   - 在上升趋势中：可能是回调买入机会 → 做多
//   - 在震荡市中：方向不明 → 观望
//
// EDGE_BEAR: 价格触碰阻力位（结构测试事件）
//   - 在上升趋势中：可能是阻力突破前的测试 → 观望或做多
//   - 在下降趋势中：可能是反弹卖出机会 → 做空
//   - 在震荡市中：方向不明 → 观望
//
// 交易方向应由HTF结构方向（struct_dir）和订单流方向（of_dir）共同决定
// ========================================

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

	// ========== Step A: Redline 标记（只设置 block_entry，不阻止计算）==========
	blockEntry := false
	blockReason := ""

	// Intent 标记处理（方向感知型，修复误封锁问题）
	// data_insufficient：硬红线，数据不足禁止所有开仓
	// fake_pump/fake_dump：方向感知型，不在后端强制封锁
	//   - 在 SHORT 方向 + HTF 阻力区：fake_pump 实为做空信号（散户被诱多，聪明钱卖出）
	//   - 其他场景由 AI 在 Gate2 根据 INTENT_PUMP_WARN 标记做上下文判断
	intentNorm := NormalizeIntent(in.Orderflow.Micro5m.CandleIntent)
	if intentNorm == "data_insufficient" {
		// 数据不足是硬红线：无法判断方向时禁止一切新开仓
		blockEntry = true
		blockReason = "INTENT_REDLINE"
		flags = append(flags, "INTENT_REDLINE")
	} else if intentNorm == "fake" {
		// fake_pump/fake_dump 改为软警告：由 AI 根据方向上下文决定
		flags = append(flags, "INTENT_PUMP_WARN")
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

	// divergent_mixed 特殊处理：
	// 不能直接定向，但不能无条件归零——利用 dominant_direction 和现货/期货 CVD 量比推导偏置
	// 偏置以 50% 降级（因数据本身存在分歧，置信度打折）
	if strings.ToLower(in.Orderflow.Macro.TrendAlignment) == "divergent_mixed" {
		flags = append(flags, "DIVERGENT_MIXED")
		bias := computeDivergentBias(in.Orderflow.Macro)
		if bias != 0 {
			macroAlign = bias
			macroSign = macroAlign * macroStrength * 0.5 // 降级50%
		}
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
	wOF := cfg.WOFMin + (cfg.WOFMax-cfg.WOFMin)*ofQ
	wST := 1 - wOF

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

	// 🔥 修复：过滤弱信号，避免在震荡市中频繁亏损
	// 当结构和订单流都是弱信号时，强制NEUTRAL
	if abs(structDir) < 0.3 && abs(ofDir) < 0.3 {
		flags = append(flags, "WEAK_SIGNAL_FILTERED")
		side = SideNeutral
		// WEAK_SIGNAL_FILTERED: plan_side=NEUTRAL 时必须设 block_entry=true
		// 否则后端发出 plan_side=NEUTRAL + block_entry=false，AI 报 DIR_INPUT_INCONSISTENT
		if !blockEntry {
			blockEntry = true
			blockReason = "WEAK_SIGNAL_FILTERED"
		}
	} else {
		// 只有非弱信号才进行方向裁决
		if abs(delta) >= cfg.ThetaNeutral && conf >= cfg.MinConf {
			if delta > 0 {
				side = SideLong
			} else {
				side = SideShort
			}
		}
	}

	// ========== Step D: 宏观反向阻断（对应 Gate1 语义）==========
	// 主路：macroAlign 含 divergent_mixed 偏置后的值，已能覆盖大部分情况
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

	// 补充路（Task #2）：当 macroAlign 仍为 0（divergent_mixed 偏置未能确定方向）时，
	// 使用 dominant_direction 作为最后一道宏观否决防线
	if macroAlign == 0 && side != SideNeutral && macroStrength > 0.60 {
		dominantDir := DominantDirMap(in.Orderflow.Macro.DominantDirection)
		if dominantDir != 0 && sign(dominantDir) != signSide(side) {
			if !StrongCounterexample(in, wallSign, flags) {
				blockEntry = true
				blockReason = "MACRO_OPPOSE"
				flags = append(flags, "MACRO_OPPOSE_BLOCK")
			} else {
				flags = append(flags, "MACRO_OPPOSE_BUT_EXCEPT")
				conf *= 0.75
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

	// ========== 安全网：plan_side=NEUTRAL 时始终 block_entry=true ==========
	// 防止 AI 收到 plan_side=NEUTRAL + block_entry=false → DIR_INPUT_INCONSISTENT
	// WEAK_SIGNAL_FILTERED 已在上方处理；此处兜底覆盖 delta/conf 不足等其余 NEUTRAL 路径
	if side == SideNeutral && !blockEntry {
		blockEntry = true
		blockReason = "NEUTRAL_NO_DIR"
	}

	return DirectionArbitration{
		PlanSide:       side,
		BlockEntry:     blockEntry,
		BlockReason:    blockReason,
		Confidence:     conf,
		SignalStrength:  abs(delta), // 绝对信号强度，与相对置信度解耦，供AI感知真实力度
		Delta:          delta,
		OFDir:          ofDir,
		StructDir:      structDir,
		OFQuality:      ofQ,
		Flags:          flags,
		Debug:          dbg,
	}
}
