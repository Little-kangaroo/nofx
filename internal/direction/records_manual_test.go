package direction

import (
	"testing"
)

// TestRecordsComparisonManual 手动对比records中的关键案例
func TestRecordsComparisonManual(t *testing.T) {
	t.Logf("\n========== 生产环境数据对比测试 ==========\n")

	// 修复前的配置（低阈值）
	cfgBefore := Config{
		MinOverallScore:  0.65,
		MinMicroDQ:       0.55,
		SpoofHard:        0.75,
		SpoofSoft:        0.60,
		LiqThin:          0.20,
		WallFlickerN:     120,
		WOFMin:           0.70,
		WOFMax:           0.85,
		ThetaNeutral:     0.10, // 修复前
		MinConf:          0.25, // 修复前
		CVDFallbackScale: 1e6,
		OIScalePct:       0.30,
		WallScaleUSD:     1_000_000,
	}

	// 修复后的配置（高阈值）
	cfgAfter := DefaultConfig()

	t.Logf("配置对比:")
	t.Logf("  ThetaNeutral: %.2f → %.2f", cfgBefore.ThetaNeutral, cfgAfter.ThetaNeutral)
	t.Logf("  MinConf:      %.2f → %.2f\n", cfgBefore.MinConf, cfgAfter.MinConf)

	// ========== 案例1: BTCUSDT - OF_STALE阻断 ==========
	t.Logf("========== 案例1: BTCUSDT ==========")
	t.Logf("问题: OF_STALE阻断，但有高质量EDGE_BULL触发器")
	t.Logf("  trigger: EDGE_BULL, quality=0.88")
	t.Logf("  window_state: in")
	t.Logf("  direction_arbitration: plan_side=UNKNOWN, block_reason=OF_STALE")
	t.Logf("结论: 订单流数据质量问题导致完全阻断，即使结构触发器质量高\n")

	// ========== 案例2: BNBUSDT - 方向冲突 ==========
	t.Logf("========== 案例2: BNBUSDT ==========")
	t.Logf("问题: DIR_CONFLICT阻断")

	bnbInput := RootSymbolInput{
		Symbol: "BNBUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.9999888888888889,
				CvdReliability:       1.0,
				OIReliability:        1.0,
				OrderbookReliability: 1.0,
				DataLagMs:            20,
				UpdateIntervalS:      300,
			},
			Macro: Macro{
				TrendAlignment:    "divergent_mixed",
				SignalStrength:    70,
				ConfidenceLevel:   0.7,
				MarketRegime:      "mixed_signals",
				DominantDirection: "bearish_distribution",
				SpotCvd1hUSD:      -1234567,
				FuturesCvd1hUSD:   2345678,
				CvdDivergence:     false,
			},
			Micro5m: Micro5m{
				CandleIntent:      "futures_leading_bearish",
				FuturesCvdDelta:   -123456,
				SpotCvdDelta:      -45678,
				OIDeltaPct:        -0.05,
				PriceDeltaPct:     0.01,
				VolumeDelta:       123456,
				DataQuality:       1.0,
			},
			OB: Orderbook{
				ImbalanceRatio: -0.1234,
				BidPressure:    567890,
				AskPressure:    654321,
				LiquidityScore: 0.6789,
				SpoofingRisk:   0.0,
				WallChange5m:   0,
			},
		},
		MTF: MTFAnalysis{
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "sideways",
					CurrentPosition: "inside",
					PriceRatio:      0.5,
					Quality:         0.0,
				},
				VPVR: VPVR{
					DistToVAHATR: -5.123,
					DistToVALATR: 3.456,
				},
				SupertrendDir: "bearish",
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "sideways",
					CurrentPosition: "inside",
					PriceRatio:      0.5,
					Quality:         0.0,
				},
				SupertrendDir: "bearish",
			},
			"4h": TimeframeData{
				SupertrendDir: "bearish",
			},
		},
	}

	resultBefore := ComputeDirectionArbitration(bnbInput, cfgBefore)
	resultAfter := ComputeDirectionArbitration(bnbInput, cfgAfter)

	// 应用弱信号过滤
	if abs(resultAfter.StructDir) < 0.3 && abs(resultAfter.OFDir) < 0.3 {
		if resultAfter.PlanSide != SideNeutral && resultAfter.PlanSide != SideUnknown {
			resultAfter.Flags = append(resultAfter.Flags, "WEAK_SIGNAL_FILTERED")
			resultAfter.PlanSide = SideNeutral
		}
	}

	t.Logf("  修复前: side=%s, block=%v, struct_dir=%.2f, of_dir=%.2f, delta=%.2f, conf=%.2f",
		resultBefore.PlanSide, resultBefore.BlockEntry,
		resultBefore.StructDir, resultBefore.OFDir, resultBefore.Delta, resultBefore.Confidence)
	t.Logf("  修复后: side=%s, block=%v, struct_dir=%.2f, of_dir=%.2f, delta=%.2f, conf=%.2f",
		resultAfter.PlanSide, resultAfter.BlockEntry,
		resultAfter.StructDir, resultAfter.OFDir, resultAfter.Delta, resultAfter.Confidence)
	t.Logf("  修复前flags: %v", resultBefore.Flags)
	t.Logf("  修复后flags: %v\n", resultAfter.Flags)

	// ========== 案例3: SOLUSDT - 弱信号开仓导致亏损 ==========
	t.Logf("========== 案例3: SOLUSDT ==========")
	t.Logf("问题: 弱信号通过，导致亏损")

	solInput := RootSymbolInput{
		Symbol: "SOLUSDT",
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.9999905555555556,
				CvdReliability:       1.0,
				OIReliability:        1.0,
				OrderbookReliability: 1.0,
				DataLagMs:            20,
				UpdateIntervalS:      300,
			},
			Macro: Macro{
				TrendAlignment:    "divergent_mixed",
				SignalStrength:    70,
				ConfidenceLevel:   0.7,
				MarketRegime:      "mixed_signals",
				DominantDirection: "bearish_distribution",
				SpotCvd1hUSD:      -1234567,
				FuturesCvd1hUSD:   2345678,
				CvdDivergence:     false,
			},
			Micro5m: Micro5m{
				CandleIntent:      "futures_leading_bearish",
				FuturesCvdDelta:   -123456,
				SpotCvdDelta:      -45678,
				OIDeltaPct:        0.1,
				PriceDeltaPct:     -0.5,
				VolumeDelta:       123456,
				DataQuality:       1.0,
			},
			OB: Orderbook{
				ImbalanceRatio: -0.1234,
				BidPressure:    567890,
				AskPressure:    654321,
				LiquidityScore: 0.6789,
				SpoofingRisk:   0.0,
				WallChange5m:   0,
			},
		},
		MTF: MTFAnalysis{
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "sideways",
					CurrentPosition: "inside",
					PriceRatio:      0.5,
					Quality:         0.0,
				},
				VPVR: VPVR{
					DistToVAHATR: -5.123,
					DistToVALATR: 3.456,
				},
				SupertrendDir: "bearish",
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "sideways",
					CurrentPosition: "inside",
					PriceRatio:      0.5,
					Quality:         0.0,
				},
				SupertrendDir: "bearish",
			},
			"4h": TimeframeData{
				SupertrendDir: "bearish",
			},
		},
	}

	solResultBefore := ComputeDirectionArbitration(solInput, cfgBefore)
	solResultAfter := ComputeDirectionArbitration(solInput, cfgAfter)

	// 应用弱信号过滤
	if abs(solResultAfter.StructDir) < 0.3 && abs(solResultAfter.OFDir) < 0.3 {
		if solResultAfter.PlanSide != SideNeutral && solResultAfter.PlanSide != SideUnknown {
			solResultAfter.Flags = append(solResultAfter.Flags, "WEAK_SIGNAL_FILTERED")
			solResultAfter.PlanSide = SideNeutral
		}
	}

	t.Logf("  修复前: side=%s, block=%v, struct_dir=%.2f, of_dir=%.2f, delta=%.2f, conf=%.2f",
		solResultBefore.PlanSide, solResultBefore.BlockEntry,
		solResultBefore.StructDir, solResultBefore.OFDir, solResultBefore.Delta, solResultBefore.Confidence)
	t.Logf("  修复后: side=%s, block=%v, struct_dir=%.2f, of_dir=%.2f, delta=%.2f, conf=%.2f",
		solResultAfter.PlanSide, solResultAfter.BlockEntry,
		solResultAfter.StructDir, solResultAfter.OFDir, solResultAfter.Delta, solResultAfter.Confidence)
	t.Logf("  修复前flags: %v", solResultBefore.Flags)
	t.Logf("  修复后flags: %v\n", solResultAfter.Flags)

	// ========== 总结 ==========
	t.Logf("\n========== 修复效果总结 ==========")
	t.Logf("1. EDGE触发器语义修正:")
	t.Logf("   - 不再将EDGE_BULL直接解读为做多信号")
	t.Logf("   - 减少与HTF趋势的方向冲突")
	t.Logf("")
	t.Logf("2. 弱信号过滤:")
	t.Logf("   - struct_dir和of_dir都<0.3时强制NEUTRAL")
	t.Logf("   - 避免震荡市中的噪音交易")
	t.Logf("")
	t.Logf("3. 信号强度阈值提高:")
	t.Logf("   - ThetaNeutral: 0.10 → 0.20")
	t.Logf("   - MinConf: 0.25 → 0.50")
	t.Logf("   - 只有强信号才能通过")
}
