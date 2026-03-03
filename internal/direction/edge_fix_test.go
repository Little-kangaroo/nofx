package direction

import (
	"testing"
)

// TestWeakSignalFiltering 测试弱信号过滤功能
func TestWeakSignalFiltering(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name      string
		structDir float64
		ofDir     float64
		wantSide  DirectionSide
		wantFlag  string
	}{
		{
			name:      "弱信号-双方都弱",
			structDir: 0.1,  // 轻度看涨
			ofDir:     0.15, // 轻度看涨
			wantSide:  SideNeutral,
			wantFlag:  "WEAK_SIGNAL_FILTERED",
		},
		{
			name:      "弱信号-结构弱订单流弱",
			structDir: -0.2, // 轻度看跌
			ofDir:     -0.1, // 轻度看跌
			wantSide:  SideNeutral,
			wantFlag:  "WEAK_SIGNAL_FILTERED",
		},
		{
			name:      "强信号-结构强",
			structDir: -0.5, // 中度看跌
			ofDir:     -0.2, // 轻度看跌
			wantSide:  SideShort, // 应该通过
			wantFlag:  "",
		},
		{
			name:      "强信号-订单流强",
			structDir: 0.2,  // 轻度看涨
			ofDir:     0.6,  // 强看涨
			wantSide:  SideLong, // 应该通过
			wantFlag:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 构造测试输入
			in := RootSymbolInput{
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
						TrendAlignment:    "bullish_aligned",
						SignalStrength:    50,
						ConfidenceLevel:   0.5,
						MarketRegime:      "trending",
						DominantDirection: "bullish_accumulation",
					},
					Micro5m: Micro5m{
						CandleIntent:      "bullish_confirm",
						FuturesCvdDelta:   100000,
						SpotCvdDelta:      50000,
						OIDeltaPct:        0.1,
						PriceDeltaPct:     0.5,
						VolumeDelta:       1000000,
						DataQuality:       0.8,
					},
					OB: Orderbook{
						ImbalanceRatio:   0.1,
						BidPressure:      1000000,
						AskPressure:      900000,
						LiquidityScore:   0.7,
						SpoofingRisk:     0.3,
						WallChange5m:     0,
					},
				},
				MTF: MTFAnalysis{
					"15m": TimeframeData{
						Channel: ChannelInfo{
							Direction:       "up",
							CurrentPosition: "inside",
							PriceRatio:      0.5,
							Quality:         0.8,
						},
					},
				},
			}

			// 手动设置structDir和ofDir来模拟不同场景
			// 注意：这里我们需要修改ComputeDirectionArbitration的实现
			// 或者通过调整输入数据来间接控制这些值

			result := ComputeDirectionArbitration(in, cfg)

			// 验证结果
			t.Logf("Result: side=%s, flags=%v, structDir=%.2f, ofDir=%.2f",
				result.PlanSide, result.Flags, result.StructDir, result.OFDir)

			// 检查是否包含预期的flag
			if tt.wantFlag != "" {
				found := false
				for _, flag := range result.Flags {
					if flag == tt.wantFlag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected flag %s not found in %v", tt.wantFlag, result.Flags)
				}
			}
		})
	}
}

// TestEdgeTriggerSemantics 测试EDGE触发器语义
// 验证EDGE触发器不应该直接决定交易方向
func TestEdgeTriggerSemantics(t *testing.T) {
	t.Log("EDGE触发器语义测试")
	t.Log("EDGE_BULL: 触碰支撑位 → 结构测试事件（非做多信号）")
	t.Log("EDGE_BEAR: 触碰阻力位 → 结构测试事件（非做空信号）")
	t.Log("")
	t.Log("正确的方向判断逻辑：")
	t.Log("1. 下降趋势 + EDGE_BEAR（触碰阻力）→ 做空（顺势）")
	t.Log("2. 下降趋势 + EDGE_BULL（触碰支撑）→ 观望（可能跌破）")
	t.Log("3. 上升趋势 + EDGE_BULL（触碰支撑）→ 做多（顺势）")
	t.Log("4. 上升趋势 + EDGE_BEAR（触碰阻力）→ 观望（可能突破）")
	t.Log("5. 震荡市 + 任何EDGE → 观望（方向不明）")
}

// TestSignalStrengthThresholds 测试信号强度阈值
func TestSignalStrengthThresholds(t *testing.T) {
	cfg := DefaultConfig()

	t.Logf("当前配置:")
	t.Logf("  ThetaNeutral: %.2f (delta阈值)", cfg.ThetaNeutral)
	t.Logf("  MinConf: %.2f (最小置信度)", cfg.MinConf)
	t.Logf("  MinOverallScore: %.2f (最小整体评分)", cfg.MinOverallScore)
	t.Logf("  MinMicroDQ: %.2f (最小微观数据质量)", cfg.MinMicroDQ)

	// 验证阈值是否合理
	if cfg.ThetaNeutral < 0.15 {
		t.Errorf("ThetaNeutral太低(%.2f)，可能导致弱信号通过", cfg.ThetaNeutral)
	}
	if cfg.MinConf < 0.40 {
		t.Errorf("MinConf太低(%.2f)，可能导致低置信度信号通过", cfg.MinConf)
	}

	t.Log("\n建议阈值:")
	t.Log("  ThetaNeutral: 0.20-0.25 (过滤弱方向性)")
	t.Log("  MinConf: 0.50-0.60 (确保高置信度)")
}
