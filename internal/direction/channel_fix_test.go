package direction

import (
	"testing"
)

// TestChannelDataMissing_DOGEUSDT 测试通道数据完全缺失的情况（DOGEUSDT案例）
// 预期：返回 plan_side="CHANNEL_DATA_MISSING", block_entry=true
func TestChannelDataMissing_DOGEUSDT(t *testing.T) {
	// 构造输入数据：所有通道数据为空
	input := RootSymbolInput{
		Symbol: "DOGEUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.85,
				CvdReliability:       0.80,
				OIReliability:        0.75,
				OrderbookReliability: 0.80,
			},
			Macro: Macro{
				TrendAlignment:  "bullish_weak",
				SignalStrength:  45.0,
				ConfidenceLevel: 0.60,
			},
			Micro5m: Micro5m{
				CandleIntent:    "bullish",
				FuturesCvdDelta: 500000,
				SpotCvdDelta:    200000,
				OIDeltaPct:      0.5,
				DataQuality:     0.75,
			},
			OB: Orderbook{
				ImbalanceRatio:   0.3,
				BidPressure:      0.6,
				AskPressure:      0.4,
				LiquidityScore:   0.7,
				SpoofingRisk:     0.3,
			},
		},
		MTF: MTFAnalysis{
			// 🔥 关键：所有通道数据为空（direction=""）
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "", // 空字符串
					CurrentPosition: "",
					PriceRatio:      0.5,
					Quality:         0.0, // 质量为0
				},
				VPVR: VPVR{
					DistToVAHATR: -5.79,
					DistToVALATR: 4.73,
				},
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "", // 空字符串
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
					Direction:       "", // 空字符串
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

	// 使用默认配置
	cfg := DefaultConfig()

	// 执行方向裁决
	result := ComputeDirectionArbitration(input, cfg)

	// 验证结果
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)
	t.Logf("标记: %v", result.Flags)

	// 断言：应该返回 CHANNEL_DATA_MISSING
	if result.PlanSide != SideChannelDataMissing {
		t.Errorf("❌ 预期 plan_side=CHANNEL_DATA_MISSING, 实际: %s", result.PlanSide)
	} else {
		t.Logf("✅ 正确返回 plan_side=CHANNEL_DATA_MISSING")
	}

	// 断言：应该拦截开仓
	if !result.BlockEntry {
		t.Errorf("❌ 预期 block_entry=true, 实际: false")
	} else {
		t.Logf("✅ 正确拦截开仓 (block_entry=true)")
	}

	// 断言：拦截原因应该是 CHANNEL_DATA_MISSING
	if result.BlockReason != "CHANNEL_DATA_MISSING" {
		t.Errorf("❌ 预期 block_reason=CHANNEL_DATA_MISSING, 实际: %s", result.BlockReason)
	} else {
		t.Logf("✅ 拦截原因正确")
	}

	// 检查标记
	hasFlag := false
	for _, flag := range result.Flags {
		if flag == "CHANNEL_DATA_MISSING" || flag == "CHANNEL_DATA_INSUFFICIENT" {
			hasFlag = true
			break
		}
	}
	if !hasFlag {
		t.Errorf("❌ 标记中缺少 CHANNEL_DATA_MISSING 或 CHANNEL_DATA_INSUFFICIENT")
	} else {
		t.Logf("✅ 标记正确包含通道数据缺失标识")
	}
}

// TestChannelDataValid_BTCUSDT 测试通道数据有效的情况（BTCUSDT案例）
// 预期：能够正确读取通道数据，structDir不为0
func TestChannelDataValid_BTCUSDT(t *testing.T) {
	// 构造输入数据：通道数据有效
	input := RootSymbolInput{
		Symbol: "BTCUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.82,
				CvdReliability:       0.78,
				OIReliability:        0.80,
				OrderbookReliability: 0.85,
			},
			Macro: Macro{
				TrendAlignment:  "bearish_weak",
				SignalStrength:  40.0,
				ConfidenceLevel: 0.55,
			},
			Micro5m: Micro5m{
				CandleIntent:    "bearish",
				FuturesCvdDelta: -800000,
				SpotCvdDelta:    -300000,
				OIDeltaPct:      -0.3,
				DataQuality:     0.80,
			},
			OB: Orderbook{
				ImbalanceRatio:   -0.2,
				BidPressure:      0.4,
				AskPressure:      0.6,
				LiquidityScore:   0.75,
				SpoofingRisk:     0.25,
			},
		},
		MTF: MTFAnalysis{
			// 🔥 关键：通道数据有效，但是 flat + break_up（向上突破）
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",     // 有值
					CurrentPosition: "break_up", // 向上突破
					PriceRatio:      1.2,
					Quality:         0.75, // 质量>0
				},
				VPVR: VPVR{
					DistToVAHATR: -13.90,
					DistToVALATR: -0.90,
				},
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "break_down", // 向下突破
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
					CurrentPosition: "break_down", // 向下突破
					PriceRatio:      -0.05,
					Quality:         0.65,
				},
				VPVR: VPVR{
					DistToVAHATR: -6.98,
					DistToVALATR: 2.21,
				},
			},
			"4h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "break_up", // 向上突破
					PriceRatio:      1.1,
					Quality:         0.68,
				},
				VPVR: VPVR{
					DistToVAHATR: -24.97,
					DistToVALATR: 7.27,
				},
			},
		},
	}

	// 使用默认配置
	cfg := DefaultConfig()

	// 执行方向裁决
	result := ComputeDirectionArbitration(input, cfg)

	// 验证结果
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)
	t.Logf("置信度: %.2f", result.Confidence)
	t.Logf("OFDir: %.2f", result.OFDir)
	t.Logf("StructDir: %.2f", result.StructDir)
	t.Logf("标记: %v", result.Flags)

	// 断言：不应该返回 CHANNEL_DATA_MISSING
	if result.PlanSide == SideChannelDataMissing {
		t.Errorf("❌ 不应该返回 CHANNEL_DATA_MISSING，通道数据是有效的")
	} else {
		t.Logf("✅ 通道数据有效，未返回 CHANNEL_DATA_MISSING")
	}

	// 断言：StructDir 不应该为0（说明通道数据被正确使用）
	if result.StructDir == 0 {
		t.Errorf("❌ StructDir为0，说明通道数据未被正确使用")
	} else {
		t.Logf("✅ StructDir=%.2f，通道数据被正确使用", result.StructDir)
	}

	// 检查是否有通道数据缺失的标记
	for _, flag := range result.Flags {
		if flag == "CHANNEL_DATA_MISSING" || flag == "CHANNEL_DATA_INSUFFICIENT" {
			t.Errorf("❌ 不应该有通道数据缺失标记: %s", flag)
		}
	}
	t.Logf("✅ 没有通道数据缺失标记")
}

// TestChannelDataPartialMissing 测试部分通道数据缺失的情况
// 预期：如果少于2个时间框架有有效数据，应该拦截
func TestChannelDataPartialMissing(t *testing.T) {
	// 构造输入数据：只有1个时间框架有有效通道数据
	input := RootSymbolInput{
		Symbol: "ETHUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.80,
				CvdReliability:       0.75,
				OIReliability:        0.70,
				OrderbookReliability: 0.80,
			},
			Macro: Macro{
				TrendAlignment:  "neutral",
				SignalStrength:  30.0,
				ConfidenceLevel: 0.50,
			},
			Micro5m: Micro5m{
				CandleIntent:    "neutral",
				FuturesCvdDelta: 100000,
				SpotCvdDelta:    50000,
				OIDeltaPct:      0.1,
				DataQuality:     0.70,
			},
			OB: Orderbook{
				ImbalanceRatio:   0.1,
				BidPressure:      0.5,
				AskPressure:      0.5,
				LiquidityScore:   0.65,
				SpoofingRisk:     0.35,
			},
		},
		MTF: MTFAnalysis{
			// 只有15m有有效数据
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "up",
					CurrentPosition: "middle",
					PriceRatio:      0.5,
					Quality:         0.70, // 有效
				},
				VPVR: VPVR{},
			},
			// 30m和1h都是空数据
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "",
					CurrentPosition: "",
					PriceRatio:      0.5,
					Quality:         0.0, // 无效
				},
				VPVR: VPVR{},
			},
			"1h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "",
					CurrentPosition: "",
					PriceRatio:      0.5,
					Quality:         0.0, // 无效
				},
				VPVR: VPVR{},
			},
		},
	}

	// 使用默认配置
	cfg := DefaultConfig()

	// 执行方向裁决
	result := ComputeDirectionArbitration(input, cfg)

	// 验证结果
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)

	// 断言：应该返回 CHANNEL_DATA_MISSING（因为少于2个有效时间框架）
	if result.PlanSide != SideChannelDataMissing {
		t.Errorf("❌ 预期 plan_side=CHANNEL_DATA_MISSING（只有1个有效时间框架），实际: %s", result.PlanSide)
	} else {
		t.Logf("✅ 正确拦截：只有1个有效时间框架，少于要求的2个")
	}

	if !result.BlockEntry {
		t.Errorf("❌ 预期 block_entry=true")
	} else {
		t.Logf("✅ 正确拦截开仓")
	}
}
