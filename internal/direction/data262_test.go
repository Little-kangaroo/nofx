package direction

import (
	"testing"
)

// TestData262 测试 data262.txt 中的开仓数据
// 原始决策：SHORT，未被拦截
// 关键特征：15m flat+breakdown, 1h/30m/4h down+breakup（矛盾信号）
func TestData262(t *testing.T) {
	input := RootSymbolInput{
		Symbol: "XRPUSDT",
		MTF: MTFAnalysis{
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "breakdown",
					PriceRatio:      0.5,
					Quality:         0.70, // 从signal_confidence 99.3推断
				},
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "down",
					CurrentPosition: "breakup",
					PriceRatio:      0.5,
					Quality:         0.70, // 从signal_confidence 99.9推断
				},
			},
			"1h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "down",
					CurrentPosition: "breakup", // 关键：矛盾信号
					PriceRatio:      0.5,
					Quality:         0.70, // 从signal_confidence 98.7推断
				},
			},
			"4h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "down",
					CurrentPosition: "breakup",
					PriceRatio:      0.5,
					Quality:         0.70, // 从signal_confidence 99.1推断
				},
			},
		},
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.70,
				DataLagMs:            100,
				UpdateIntervalS:      5,
				CvdReliability:       0.75,
				OIReliability:        0.70,
				OrderbookReliability: 0.72,
			},
			Macro: Macro{
				TrendAlignment:  "down",
				SignalStrength:  70, // 提高信号强度
				ConfidenceLevel: 0.70,
				MarketRegime:    "trending",
				CvdDivergence:   false,
			},
			Micro5m: Micro5m{
				CandleIntent:      "bearish",
				FuturesCvdDelta:   -80000, // 更强的卖压
				SpotCvdDelta:      -50000,
				OIDeltaPct:        -0.8,
				PriceDeltaPct:     -0.5,
				VolumeDelta:       1500000,
				DataQuality:       0.70,
			},
			OB: Orderbook{
				ImbalanceRatio:   -0.5, // 更强的卖盘压力
				BidPressure:      0.3,
				AskPressure:      0.7,
				LiquidityScore:   0.75,
				SpoofingRisk:     0.30,
				SupportWall:      nil,
				ResistanceWall:   nil,
				WallChange5m:     10,
			},
		},
	}

	cfg := DefaultConfig()
	result := ComputeDirectionArbitration(input, cfg)

	t.Logf("=== Data262 测试结果 ===")
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)
	t.Logf("标记: %v", result.Flags)
	t.Logf("置信度: %.2f", result.Confidence)
	t.Logf("Delta: %.2f", result.Delta)

	// 分析：根据新逻辑，这个交易应该被1h矛盾信号拦截
	// 因为系统判断SHORT，但1h显示breakup（向上突破）
	if result.PlanSide == SideShort && result.BlockEntry {
		if result.BlockReason == "1H_BREAKUP_CONFLICT" {
			t.Logf("✅ 成功拦截：1h矛盾信号检查生效")
		} else {
			t.Logf("⚠️ 被拦截，但原因不是1h矛盾信号: %s", result.BlockReason)
		}
	} else if result.PlanSide == SideShort && !result.BlockEntry {
		t.Errorf("❌ 未被拦截：新逻辑应该拦截这个交易（1h breakup矛盾）")
	} else {
		t.Logf("ℹ️ 判断方向: %s, 拦截: %v", result.PlanSide, result.BlockEntry)
	}
}
