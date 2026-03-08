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

// ofStaleLevel 订单流失效等级
// fullyStale=true  → 数据完全缺失，无法计算任何 OF 信号，必须返回 UNKNOWN
// partialStale=true → 状态/评分异常，但底层数据存在，降级为结构主导模式继续计算
func ofStaleLevel(of Orderflow, cfg Config, flags *[]string) (fullyStale bool, partialStale bool) {
	// 软失效优先：状态异常时直接降级为结构主导，不得被后续 LiquidityScore=0 升级为 fullStale
	// 场景：BTC/ETH 盘口 WebSocket 断联 → LiquidityScore=0 + Status=异常
	//   旧逻辑：LiquidityScore<=0 先判 → fullStale → UNKNOWN（错误，丢失结构信号）
	//   新逻辑：Status!=正常 先判 → partialStale → 结构主导（ob_sign=0，CVD/OI/struct 仍可用）
	if of.Quality.Status != "正常" {
		*flags = append(*flags, "OF_STATUS_BAD")
		return false, true
	}

	// 硬失效：状态正常但盘口数据为零 —— 真正无数据（非断联，而是流动性极差）
	if of.OB.LiquidityScore <= 0 {
		*flags = append(*flags, "OF_LIQ_ZERO")
		return true, false
	}
	if of.OB.AskPressure == 0 && of.OB.BidPressure == 0 {
		*flags = append(*flags, "OF_PRESSURE_ZERO")
		return true, false
	}

	// 软失效：整体评分低
	if of.Quality.OverallScore < cfg.MinOverallScore {
		*flags = append(*flags, "OF_SCORE_LOW")
		return false, true
	}

	// 软失效：微观数据质量低
	if of.Micro5m.DataQuality < cfg.MinMicroDQ {
		*flags = append(*flags, "OF_MICRO_DQ_LOW")
		return false, true
	}

	return false, false
}

// isOFStale 保留向后兼容的完全失效判断（供测试引用）
func isOFStale(of Orderflow, cfg Config, flags *[]string) bool {
	fully, _ := ofStaleLevel(of, cfg, flags)
	return fully
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

	// ========== Step A: OF_STALE 分级处理 ==========
	// fullyStale  → 盘口数据为零，无法计算任何信号，直接返回 UNKNOWN
	// partialStale → 状态/评分异常但数据存在，降级为结构主导模式（wOF=0.30）继续计算
	fullyStale, partialStale := ofStaleLevel(in.Orderflow, cfg, &flags)
	if fullyStale {
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
	ofDegraded := partialStale
	if ofDegraded {
		flags = append(flags, "OF_STALE_DEGRADED")
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
		// 价格在机构卖盘中仍然上涨 = 买方正在吸收卖盘
		// smart_money_distribution 意图信号不可信，弱化负向 intentSign
		if intentSign < 0 {
			intentSign *= 0.20
		}
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
	// 🔥 顺势修复：宏观权重自适应衰减
	// 宏观趋势反映较长周期（24h+）资金方向，CVD 反映当前 5m 实时资金流向
	// 当两者方向相反且 CVD 信号较强（>0.30）时，宏观可能已滞后于市场——降低其权重
	// 例：宏观仍显示 bearish_distribution，但 5m CVD 持续流入 → 可能是趋势转折早期
	macroWeight := 0.15
	if sign(cvdSign) != 0 && sign(macroSign) != 0 &&
		sign(cvdSign) != sign(macroSign) && abs(cvdSign) > 0.30 {
		macroWeight = 0.07
		flags = append(flags, "MACRO_CVD_CONFLICT")
	}

	// 归一化：非宏观分量总权重固定为 0.85（intent+CVD+OI+OB+wall）
	// 当 macroWeight 从 0.15 降至 0.07 时，其余分量按比例等比放大，确保总权重始终 = 1.0
	otherScale := (1.0 - macroWeight) / 0.85
	ofDir := clamp(
		macroWeight*macroSign+                           // 宏观趋势：自适应 7-15%
			otherScale*(0.15*intentSign+ // 蜡烛意图
				0.30*cvdSign+  // CVD
				0.10*oiSign+   // OI
				0.20*obSign+   // 订单簿
				0.10*wallSign), // 墙
		-1, 1,
	)

	// ========== Step C: 动态权重 ==========
	wOF := cfg.WOFMin + (cfg.WOFMax-cfg.WOFMin)*ofQ
	wST := 1 - wOF

	// OF降级模式：数据状态异常时切换为结构主导（wOF=0.30）
	// 保留OF方向作为参考，但让结构方向主导最终裁决
	if ofDegraded {
		wOF = 0.30
		wST = 0.70
		flags = append(flags, "STRUCT_DOMINANT")
	}

	// 超级趋势多时间框架共识自适应权重
	// 当 15m/30m/4h 超级趋势高度一致时（≥2/3 同向），增加结构方向权重
	// 防止强牛市/熊市中短期 OF 反向信号独霸方向，导致系统逆势交易
	// consensus=0.67(2/3一致): extraST≈+0.09; consensus=1.0(3/3一致): extraST=+0.4375
	// wOF 最低降至 0.35（wST 最高升至 0.65），使结构在趋势一致时真正主导方向
	// 注意：ofDegraded 时跳过此调整——wOF=0.30 已比 SUPERTREND 下限 0.35 更保守，
	//       若允许执行，clamp(lo=0.35, hi=0.30) 会把 wOF 从 0.30 错误地升至 0.35（反效果）
	stConsensus := SupertrendConsensus(in.MTF)
	if !ofDegraded && abs(stConsensus) >= 0.60 && abs(structDir) >= 0.20 {
		extraST := 0.50 * (abs(stConsensus) - 0.60) / 0.40
		wOF = clamp(wOF-extraST, 0.35, wOF)
		wST = 1 - wOF
		flags = append(flags, "SUPERTREND_CONSENSUS_ADJUST")
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

	// 🔥 修复：过滤弱信号，避免在震荡市中频繁亏损
	// 当结构和订单流都是极弱信号时，强制NEUTRAL（阈值从0.3降至0.2，允许of_dir 0.20-0.29的弱方向信号通过）
	if abs(structDir) < 0.20 && abs(ofDir) < 0.20 {
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

	// ========== Step C+: 结构方向保护 ==========
	// 当裁决方向与结构方向冲突时，要求 |of_dir| 达到 StructOverrideMin 才允许逆势开仓
	// 前置条件：abs(structDir) >= StructProtectMin（排除噪声级别的结构信号，如 +0.02）
	// 例：struct=+0.25（ST 全多头）+ of_dir=-0.43 + side=SHORT
	//   → abs(-0.43)=0.43 < 0.50 → STRUCT_PROTECT → side=NEUTRAL
	// 例：struct=+0.02（通道平坦噪声）+ of_dir=-0.48 + side=SHORT
	//   → abs(struct)=0.02 < StructProtectMin=0.15 → 不触发保护 → SHORT 正常放行
	if side != SideNeutral && abs(structDir) >= cfg.StructProtectMin &&
		sign(structDir) != signSide(side) {
		if abs(ofDir) < cfg.StructOverrideMin {
			flags = append(flags, "STRUCT_PROTECT")
			side = SideNeutral
			if !blockEntry {
				blockEntry = true
				blockReason = "STRUCT_PROTECT"
			}
		}
	}

	// ========== Step C++: 宏观+OF 双重反向保护 ==========
	// 当 plan_side=LONG 且宏观看空（macroSign<0，macroStrength>0.50）且 of_dir < -OFMacroFloor 时，
	// 禁止"结构孤军 LONG"——这类信号三个反向（宏观+OF+意图），仅凭通道/ST 结构开多风险极高
	// 例：SOL struct=+0.66, of=-0.37, macro=bearish_distribution → NEUTRAL（AI 必拒，提前过滤）
	// 例：ETH struct=+0.70, of=-0.12, macro=bearish → of=-0.12 > -0.25 → 允许（OF 尚弱）
	// 对称保护（SHORT 方向同理）：plan_side=SHORT + macro 看多 + of_dir > OFMacroFloor → NEUTRAL
	if side == SideLong && macroSign < 0 && ofDir < -cfg.OFMacroFloor && macroStrength > 0.50 {
		flags = append(flags, "OF_MACRO_PROTECT")
		side = SideNeutral
		if !blockEntry {
			blockEntry = true
			blockReason = "OF_MACRO_PROTECT"
		}
	}
	if side == SideShort && macroSign > 0 && ofDir > cfg.OFMacroFloor && macroStrength > 0.50 {
		flags = append(flags, "OF_MACRO_PROTECT")
		side = SideNeutral
		if !blockEntry {
			blockEntry = true
			blockReason = "OF_MACRO_PROTECT"
		}
	}

	// ========== Step D: 宏观反向阻断（对应 Gate1 语义）==========
	// 主路：macroAlign 含 divergent_mixed 偏置后的值，已能覆盖大部分情况
	if macroAlign != 0 && side != SideNeutral &&
		sign(macroAlign) != signSide(side) && macroStrength > 0.60 {
		// 豁免条件 1：多时间框架超级趋势强共识（3/3 同向）
		// 当 15m/30m/4h 全部同向（|stConsensus| ≥ 0.95）且与 plan_side 一致时，
		// 视为最强"强反例"——多周期趋势共识本身就是宏观信号的有力反驳
		if abs(stConsensus) >= 0.95 && sign(stConsensus) == signSide(side) {
			flags = append(flags, "MACRO_OPPOSE_BUT_EXCEPT")
			conf *= 0.75
		} else if sign(structDir) == signSide(side) && sign(ofDir) == signSide(side) &&
			abs(structDir) >= 0.40 && abs(ofDir) >= 0.35 {
			// 豁免条件 2：结构+订单流双向强度一致（新增）
			// 触发条件：struct_dir ≥ 0.40 且 of_dir ≥ 0.35 且两者方向与 plan_side 一致
			// 语义：本地多周期多维信号均看多/空时，宏观滞后信号不应硬阻断
			// 安全设计：要求 OF 信号达到 0.35（强于仅符号一致），避免在宏观大单反向时误豁免
			// 例：struct=+0.55, of=+0.38, plan=LONG, macro=bearish → 允许开多，conf×0.75
			flags = append(flags, "MACRO_OPPOSE_BUT_EXCEPT")
			conf *= 0.75
		} else if !StrongCounterexample(in, wallSign, flags) {
			// 检查是否满足强反例条件
			// 保留首个 blockReason（优先显示最早触发的封锁原因，如 SPOOF_HIGH）
			if !blockEntry {
				blockEntry = true
				blockReason = "MACRO_OPPOSE"
			} else {
				blockEntry = true // 确保已封锁
			}
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
				// 保留首个 blockReason（优先显示最早触发的封锁原因）
				if !blockEntry {
					blockEntry = true
					blockReason = "MACRO_OPPOSE"
				} else {
					blockEntry = true
				}
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
