package direction

import (
	"testing"
)

// TestProductionCase_Data21_BTCUSDT 测试生产环境案例：data21.txt (BTCUSDT SHORT)
// 实际情况：15m flat+break_up, 4h flat+break_up（向上突破），但系统判断做空
// 预期修复后：能正确读取通道数据，StructDir不为0
func TestProductionCase_Data21_BTCUSDT(t *testing.T) {
	// 从 data21.txt 提取的真实数据
	input := RootSymbolInput{
		Symbol: "BTCUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.85,
				CvdReliability:       0.80,
				OIReliability:        0.75,
				OrderbookReliability: 0.85,
			},
			Macro: Macro{
				TrendAlignment:  "divergent_mixed",
				SignalStrength:  50.0,
				ConfidenceLevel: 0.50,
				CvdDivergence:   false,
			},
			Micro5m: Micro5m{
				CandleIntent:    "bearish_strong",
				FuturesCvdDelta: -1000000,
				SpotCvdDelta:    -400000,
				OIDeltaPct:      -0.2,
				PriceDeltaPct:   -0.1,
				VolumeDelta:     1169,
				DataQuality:     0.80,
			},
			OB: Orderbook{
				ImbalanceRatio:   -0.15,
				BidPressure:      0.45,
				AskPressure:      0.55,
				LiquidityScore:   0.75,
				SpoofingRisk:     0.30,
			},
		},
		MTF: MTFAnalysis{
			// 🔥 关键数据：15m 和 4h 都是 flat + break_up
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",     // 横盘
					CurrentPosition: "break_up", // 向上突破
					PriceRatio:      1.2,
					Quality:         0.75,
				},
				VPVR: VPVR{
					DistToVAHATR: -13.90,
					DistToVALATR: -0.90,
				},
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "break_down",
					PriceRatio:      -0.1,
					Quality:         0.70,
				},
				VPVR: VPVR{
					DistToVAHATR: -11.78,
					DistToVALATR: 1.18,
				},
			},
			"1h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "break_down",
					PriceRatio:      -0.05,
					Quality:         0.68,
				},
				VPVR: VPVR{
					DistToVAHATR: -6.98,
					DistToVALATR: 2.21,
				},
			},
			"4h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",     // 横盘
					CurrentPosition: "break_up", // 向上突破
					PriceRatio:      1.15,
					Quality:         0.72,
				},
				VPVR: VPVR{
					DistToVAHATR: -24.97,
					DistToVALATR: 7.27,
				},
			},
		},
	}

	cfg := DefaultConfig()
	result := ComputeDirectionArbitration(input, cfg)

	t.Logf("========== BTCUSDT (data21) 测试结果 ==========")
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)
	t.Logf("置信度: %.2f", result.Confidence)
	t.Logf("Delta: %.2f", result.Delta)
	t.Logf("OFDir: %.2f", result.OFDir)
	t.Logf("StructDir: %.2f", result.StructDir)
	t.Logf("OFQuality: %.2f", result.OFQuality)
	t.Logf("标记: %v", result.Flags)
	t.Logf("==============================================")

	// 验证1：通道数据应该被正确读取
	if result.PlanSide == SideChannelDataMissing {
		t.Errorf("❌ 不应该返回 CHANNEL_DATA_MISSING，通道数据是有效的")
	} else {
		t.Logf("✅ 通道数据有效，未返回 CHANNEL_DATA_MISSING")
	}

	// 验证2：StructDir 不应该为0
	if result.StructDir == 0 {
		t.Errorf("❌ StructDir为0，说明通道数据未被正确使用")
	} else {
		t.Logf("✅ StructDir=%.2f，通道数据被正确使用", result.StructDir)
	}

	// 分析：这个案例的问题
	t.Logf("\n📊 案例分析：")
	t.Logf("- 市场实际情况：15m和4h都是向上突破（break_up）")
	t.Logf("- 订单流方向：OFDir=%.2f（看跌）", result.OFDir)
	t.Logf("- 结构方向：StructDir=%.2f", result.StructDir)
	t.Logf("- 最终判断：%s", result.PlanSide)

	if result.PlanSide == SideShort && !result.BlockEntry {
		t.Logf("⚠️  警告：系统判断做空但未拦截")
		t.Logf("⚠️  建议：需要添加反向突破拦截逻辑（15m+4h都是break_up时不应做空）")
	}
}

// TestProductionCase_Data36_DOGEUSDT 测试生产环境案例：data36.txt (DOGEUSDT LONG)
// 实际情况：所有通道数据为空，4h明显下跌趋势，但系统判断做多
// 预期修复后：返回 CHANNEL_DATA_MISSING，拦截开仓
func TestProductionCase_Data36_DOGEUSDT(t *testing.T) {
	// 从 data36.txt 提取的真实数据
	input := RootSymbolInput{
		Symbol: "DOGEUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.82,
				CvdReliability:       0.78,
				OIReliability:        0.75,
				OrderbookReliability: 0.80,
			},
			Macro: Macro{
				TrendAlignment:  "bullish_weak",
				SignalStrength:  45.0,
				ConfidenceLevel: 0.60,
				CvdDivergence:   false,
			},
			Micro5m: Micro5m{
				CandleIntent:    "bullish",
				FuturesCvdDelta: 500000,
				SpotCvdDelta:    200000,
				OIDeltaPct:      0.3,
				PriceDeltaPct:   0.2,
				VolumeDelta:     48935375,
				DataQuality:     0.75,
			},
			OB: Orderbook{
				ImbalanceRatio:   0.25,
				BidPressure:      0.60,
				AskPressure:      0.40,
				LiquidityScore:   0.70,
				SpoofingRisk:     0.25,
			},
		},
		MTF: MTFAnalysis{
			// 🔥 关键数据：所有通道数据为空
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "", // 空
					CurrentPosition: "",
					PriceRatio:      0.5,
					Quality:         0.0, // 质量为0
				},
				VPVR: VPVR{
					DistToVAHATR: -9.33,
					DistToVALATR: 1.54,
				},
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "", // 空
					CurrentPosition: "",
					PriceRatio:      0.5,
					Quality:         0.0,
				},
				VPVR: VPVR{
					DistToVAHATR: -21.63,
					DistToVALATR: 5.56,
				},
			},
			"1h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "", // 空
					CurrentPosition: "",
					PriceRatio:      0.5,
					Quality:         0.0,
				},
				VPVR: VPVR{
					DistToVAHATR: -15.69,
					DistToVALATR: 6.29,
				},
			},
		},
	}

	cfg := DefaultConfig()
	result := ComputeDirectionArbitration(input, cfg)

	t.Logf("========== DOGEUSDT (data36) 测试结果 ==========")
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)
	t.Logf("置信度: %.2f", result.Confidence)
	t.Logf("Delta: %.2f", result.Delta)
	t.Logf("OFDir: %.2f", result.OFDir)
	t.Logf("StructDir: %.2f", result.StructDir)
	t.Logf("标记: %v", result.Flags)
	t.Logf("==============================================")

	// 验证1：应该返回 CHANNEL_DATA_MISSING
	if result.PlanSide != SideChannelDataMissing {
		t.Errorf("❌ 预期返回 CHANNEL_DATA_MISSING，实际: %s", result.PlanSide)
		t.Errorf("❌ 修复失败：通道数据全部为空，应该被拦截")
	} else {
		t.Logf("✅ 正确返回 plan_side=CHANNEL_DATA_MISSING")
	}

	// 验证2：应该拦截开仓
	if !result.BlockEntry {
		t.Errorf("❌ 预期 block_entry=true，实际: false")
		t.Errorf("❌ 修复失败：应该拦截开仓")
	} else {
		t.Logf("✅ 正确拦截开仓 (block_entry=true)")
	}

	// 验证3：拦截原因
	if result.BlockReason != "CHANNEL_DATA_MISSING" {
		t.Errorf("❌ 预期 block_reason=CHANNEL_DATA_MISSING，实际: %s", result.BlockReason)
	} else {
		t.Logf("✅ 拦截原因正确")
	}

	t.Logf("\n📊 案例分析：")
	t.Logf("- 市场实际情况：4h明显下跌趋势（道氏down, SuperTrend bearish）")
	t.Logf("- 通道数据状态：全部为空")
	t.Logf("- 修复前行为：只看订单流，逆势做多，亏损13.6%%")
	t.Logf("- 修复后行为：检测到通道数据缺失，拦截开仓 ✅")
}
