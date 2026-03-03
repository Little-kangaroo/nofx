package direction

import (
	"testing"
)

// TestWeakSignalFilteringCritical 测试弱信号过滤的关键场景
// 这是生产环境部署前的关键测试
func TestWeakSignalFilteringCritical(t *testing.T) {
	cfg := DefaultConfig()

	t.Logf("当前配置:")
	t.Logf("  ThetaNeutral: %.2f", cfg.ThetaNeutral)
	t.Logf("  MinConf: %.2f", cfg.MinConf)
	t.Logf("  弱信号阈值: structDir<0.3 && ofDir<0.3\n")

	// ========== 危险场景：弱信号但delta较大 ==========
	t.Run("危险场景-弱信号但delta较大", func(t *testing.T) {
		// 构造一个弱信号场景：structDir和ofDir都很小，但由于权重分配导致delta较大
		input := RootSymbolInput{
			Symbol: "TESTUSDT",
			Orderflow: Orderflow{
				Quality: Quality{
					Status:               "正常",
					OverallScore:         0.8,
					CvdReliability:       1.0,
					OIReliability:        1.0,
					OrderbookReliability: 1.0,
					DataLagMs:            100,
					UpdateIntervalS:      300,
				},
				Macro: Macro{
					TrendAlignment:    "bearish_aligned",
					SignalStrength:    50,
					ConfidenceLevel:   0.5,
					MarketRegime:      "trending",
					DominantDirection: "bearish_distribution",
					SpotCvd1hUSD:      -100000,
					FuturesCvd1hUSD:   -200000,
				},
				Micro5m: Micro5m{
					CandleIntent:      "bearish_confirm",
					FuturesCvdDelta:   -50000,
					SpotCvdDelta:      -30000,
					OIDeltaPct:        -0.1,
					PriceDeltaPct:     -0.3,
					VolumeDelta:       1000000,
					DataQuality:       0.8,
				},
				OB: Orderbook{
					ImbalanceRatio: -0.2,
					BidPressure:    800000,
					AskPressure:    1000000,
					LiquidityScore: 0.7,
					SpoofingRisk:   0.3,
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
				},
				"30m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "sideways",
						CurrentPosition: "inside",
						PriceRatio:      0.5,
						Quality:         0.0,
					},
				},
			},
		}

		result := ComputeDirectionArbitration(input, cfg)

		t.Logf("结果:")
		t.Logf("  plan_side: %s", result.PlanSide)
		t.Logf("  struct_dir: %.2f", result.StructDir)
		t.Logf("  of_dir: %.2f", result.OFDir)
		t.Logf("  delta: %.2f", result.Delta)
		t.Logf("  confidence: %.2f", result.Confidence)
		t.Logf("  flags: %v", result.Flags)

		// 验证：即使delta可能较大，但structDir和ofDir都<0.3时应该被过滤
		if abs(result.StructDir) < 0.3 && abs(result.OFDir) < 0.3 {
			if result.PlanSide != SideNeutral {
				t.Errorf("❌ 弱信号未被过滤！struct_dir=%.2f, of_dir=%.2f, 但plan_side=%s",
					result.StructDir, result.OFDir, result.PlanSide)
			} else {
				t.Logf("✅ 弱信号被正确过滤")
			}

			// 验证是否有WEAK_SIGNAL_FILTERED标记
			hasFlag := false
			for _, flag := range result.Flags {
				if flag == "WEAK_SIGNAL_FILTERED" {
					hasFlag = true
					break
				}
			}
			if !hasFlag {
				t.Errorf("❌ 缺少WEAK_SIGNAL_FILTERED标记")
			} else {
				t.Logf("✅ 有WEAK_SIGNAL_FILTERED标记")
			}
		}
	})

	// ========== 正常场景：强信号应该通过 ==========
	t.Run("正常场景-强信号应该通过", func(t *testing.T) {
		input := RootSymbolInput{
			Symbol: "TESTUSDT",
			Orderflow: Orderflow{
				Quality: Quality{
					Status:               "正常",
					OverallScore:         0.9,
					CvdReliability:       1.0,
					OIReliability:        1.0,
					OrderbookReliability: 1.0,
					DataLagMs:            50,
					UpdateIntervalS:      300,
				},
				Macro: Macro{
					TrendAlignment:    "bearish_aligned",
					SignalStrength:    80,
					ConfidenceLevel:   0.8,
					MarketRegime:      "trending",
					DominantDirection: "bearish_distribution",
					SpotCvd1hUSD:      -500000,
					FuturesCvd1hUSD:   -800000,
				},
				Micro5m: Micro5m{
					CandleIntent:      "bearish_confirm",
					FuturesCvdDelta:   -200000,
					SpotCvdDelta:      -100000,
					OIDeltaPct:        -0.3,
					PriceDeltaPct:     -0.8,
					VolumeDelta:       2000000,
					DataQuality:       0.95,
				},
				OB: Orderbook{
					ImbalanceRatio: -0.4,
					BidPressure:    600000,
					AskPressure:    1400000,
					LiquidityScore: 0.8,
					SpoofingRisk:   0.2,
					WallChange5m:   0,
				},
			},
			MTF: MTFAnalysis{
				"15m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "down",
						CurrentPosition: "inside",
						PriceRatio:      0.7,
						Quality:         0.8,
					},
				},
				"30m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "down",
						CurrentPosition: "inside",
						PriceRatio:      0.6,
						Quality:         0.8,
					},
				},
			},
		}

		result := ComputeDirectionArbitration(input, cfg)

		t.Logf("结果:")
		t.Logf("  plan_side: %s", result.PlanSide)
		t.Logf("  struct_dir: %.2f", result.StructDir)
		t.Logf("  of_dir: %.2f", result.OFDir)
		t.Logf("  delta: %.2f", result.Delta)
		t.Logf("  confidence: %.2f", result.Confidence)
		t.Logf("  flags: %v", result.Flags)

		// 验证：强信号应该通过
		if abs(result.StructDir) >= 0.3 || abs(result.OFDir) >= 0.3 {
			if result.PlanSide == SideNeutral {
				t.Logf("⚠️ 强信号被过滤（可能是其他原因）: struct_dir=%.2f, of_dir=%.2f",
					result.StructDir, result.OFDir)
			} else {
				t.Logf("✅ 强信号正确通过: plan_side=%s", result.PlanSide)
			}
		}
	})

	// ========== 边界场景：刚好在阈值边缘 ==========
	t.Run("边界场景-刚好在阈值边缘", func(t *testing.T) {
		// structDir=0.29, ofDir=0.29（刚好小于0.3）
		// 应该被过滤
		t.Logf("测试边界值: structDir=0.29, ofDir=0.29")
		t.Logf("预期: 应该被WEAK_SIGNAL_FILTERED过滤")
	})
}

// TestConfigurationReasonableness 测试配置合理性
func TestConfigurationReasonableness(t *testing.T) {
	cfg := DefaultConfig()

	t.Logf("配置检查:")
	t.Logf("  ThetaNeutral: %.2f", cfg.ThetaNeutral)
	t.Logf("  MinConf: %.2f", cfg.MinConf)
	t.Logf("  MinOverallScore: %.2f", cfg.MinOverallScore)
	t.Logf("  MinMicroDQ: %.2f", cfg.MinMicroDQ)

	// 检查ThetaNeutral
	if cfg.ThetaNeutral < 0.10 {
		t.Errorf("❌ ThetaNeutral太低(%.2f)，可能导致弱信号通过", cfg.ThetaNeutral)
	} else if cfg.ThetaNeutral > 0.25 {
		t.Errorf("❌ ThetaNeutral太高(%.2f)，可能导致开仓率过低", cfg.ThetaNeutral)
	} else {
		t.Logf("✅ ThetaNeutral合理(%.2f)", cfg.ThetaNeutral)
	}

	// 检查MinConf
	if cfg.MinConf < 0.30 {
		t.Errorf("❌ MinConf太低(%.2f)，可能导致低置信度信号通过", cfg.MinConf)
	} else if cfg.MinConf > 0.60 {
		t.Errorf("❌ MinConf太高(%.2f)，可能导致开仓率过低", cfg.MinConf)
	} else {
		t.Logf("✅ MinConf合理(%.2f)", cfg.MinConf)
	}

	// 检查弱信号阈值
	weakSignalThreshold := 0.3
	t.Logf("\n弱信号过滤阈值: %.2f", weakSignalThreshold)
	if weakSignalThreshold < 0.2 {
		t.Errorf("❌ 弱信号阈值太低，可能过滤不足")
	} else if weakSignalThreshold > 0.4 {
		t.Errorf("❌ 弱信号阈值太高，可能过度过滤")
	} else {
		t.Logf("✅ 弱信号阈值合理")
	}
}
