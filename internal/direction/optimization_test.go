package direction

import (
	"testing"
)

// TestLossCase1 测试第一次亏损交易（127.68做空，亏损9.6%）
// 关键特征：15m flat+breakdown, 1h down+breakup（矛盾信号）
func TestLossCase1(t *testing.T) {
	// 构造输入数据
	input := RootSymbolInput{
		Symbol: "SOLUSDT",
		MTF: MTFAnalysis{
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "breakdown",
					PriceRatio:      0.5,
					Quality:         0.60,
				},
				VPVR: VPVR{
					DistToVAHATR: -28.0347,
					DistToVALATR: -6.575,
				},
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "lower",
					PriceRatio:      0.2,
					Quality:         0.65,
				},
				VPVR: VPVR{
					DistToVAHATR: -14.1429,
					DistToVALATR: 3.1649,
				},
			},
			"1h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "down",
					CurrentPosition: "breakup", // 关键：矛盾信号
					PriceRatio:      0.8,
					Quality:         0.70,
				},
				VPVR: VPVR{
					DistToVAHATR: -8.0768,
					DistToVALATR: 6.0679,
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
				SignalStrength:  60,
				ConfidenceLevel: 0.65,
				MarketRegime:    "trending",
				CvdDivergence:   false,
			},
			Micro5m: Micro5m{
				CandleIntent:      "bearish",
				FuturesCvdDelta:   -50000,
				SpotCvdDelta:      -30000,
				OIDeltaPct:        -0.5,
				PriceDeltaPct:     -0.3,
				VolumeDelta:       1000000,
				DataQuality:       0.70,
			},
			OB: Orderbook{
				ImbalanceRatio:   -0.3,
				BidPressure:      0.4,
				AskPressure:      0.6,
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

	// 验证结果
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)
	t.Logf("标记: %v", result.Flags)

	// 断言：应该被拦截
	if !result.BlockEntry {
		t.Errorf("第一次亏损交易应该被拦截，但BlockEntry=false")
	}

	// 断言：应该是1h矛盾信号拦截
	if result.BlockReason != "1H_BREAKUP_CONFLICT" {
		t.Errorf("期望拦截原因为1H_BREAKUP_CONFLICT，实际为: %s", result.BlockReason)
	}

	t.Logf("✅ 第一次亏损交易成功被拦截")
}

// TestLossCase2 测试第二次亏损交易（126.04做空，亏损13.6%）
// 关键特征：30m flat+breakdown, 1h flat+breakdown（多时间框架）
func TestLossCase2(t *testing.T) {
	// 构造输入数据
	input := RootSymbolInput{
		Symbol: "SOLUSDT",
		MTF: MTFAnalysis{
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "middle",
					PriceRatio:      0.5,
					Quality:         0.60,
				},
				VPVR: VPVR{
					DistToVAHATR: -44.5065,
					DistToVALATR: -7.6585,
				},
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "breakdown", // 关键
					PriceRatio:      0.3,
					Quality:         0.65,
				},
				VPVR: VPVR{
					DistToVAHATR: -22.6495,
					DistToVALATR: 1.9093,
				},
			},
			"1h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "breakdown", // 关键
					PriceRatio:      0.2,
					Quality:         0.70,
				},
				VPVR: VPVR{
					DistToVAHATR: -11.6211,
					DistToVALATR: 5.9461,
				},
			},
		},
		Orderflow: Orderflow{
			Quality: Quality{
				Status:               "正常",
				OverallScore:         0.68,
				DataLagMs:            120,
				UpdateIntervalS:      5,
				CvdReliability:       0.73,
				OIReliability:        0.68,
				OrderbookReliability: 0.70,
			},
			Macro: Macro{
				TrendAlignment:  "down",
				SignalStrength:  55,
				ConfidenceLevel: 0.60,
				MarketRegime:    "trending",
				CvdDivergence:   false,
			},
			Micro5m: Micro5m{
				CandleIntent:      "bearish",
				FuturesCvdDelta:   -45000,
				SpotCvdDelta:      -28000,
				OIDeltaPct:        -0.4,
				PriceDeltaPct:     -0.25,
				VolumeDelta:       950000,
				DataQuality:       0.68,
			},
			OB: Orderbook{
				ImbalanceRatio:   -0.25,
				BidPressure:      0.42,
				AskPressure:      0.58,
				LiquidityScore:   0.72,
				SpoofingRisk:     0.28,
				SupportWall:      nil,
				ResistanceWall:   nil,
				WallChange5m:     8,
			},
		},
	}

	cfg := DefaultConfig()
	result := ComputeDirectionArbitration(input, cfg)

	// 验证结果
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)
	t.Logf("标记: %v", result.Flags)

	// 断言：应该被拦截
	if !result.BlockEntry {
		t.Errorf("第二次亏损交易应该被拦截，但BlockEntry=false")
	}

	// 断言：应该是多时间框架拦截
	expectedReason := "MULTI_TF_FLAT_BREAKDOWN:30m+1h"
	if result.BlockReason != expectedReason {
		t.Errorf("期望拦截原因为%s，实际为: %s", expectedReason, result.BlockReason)
	}

	t.Logf("✅ 第二次亏损交易成功被拦截")
}

// TestChannelSign_FlatBreakdown 测试 flat + breakdown 场景（关键优化点）
func TestChannelSign_FlatBreakdown(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		pos      string
		ratio    float64
		quality  float64
		expected float64
	}{
		{
			name:     "flat+breakdown高质量应降级到-0.50",
			dir:      "flat",
			pos:      "breakdown",
			ratio:    0.5,
			quality:  0.70,
			expected: -0.50,
		},
		{
			name:     "flat+breakdown低质量应降级到-0.30",
			dir:      "flat",
			pos:      "breakdown",
			ratio:    0.5,
			quality:  0.40,
			expected: -0.30,
		},
		{
			name:     "sideways+breakdown高质量应降级到-0.50",
			dir:      "sideways",
			pos:      "breakdown",
			ratio:    0.5,
			quality:  0.65,
			expected: -0.50,
		},
		{
			name:     "down+breakdown高质量应返回-1.0",
			dir:      "down",
			pos:      "breakdown",
			ratio:    0.5,
			quality:  0.70,
			expected: -1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := channelSign(tt.dir, tt.pos, tt.ratio, tt.quality)
			// 使用容差比较避免浮点数精度问题
			epsilon := 0.0001
			if abs(result-tt.expected) > epsilon {
				t.Errorf("channelSign(%s, %s, %.2f, %.2f) = %.4f, 期望 %.4f",
					tt.dir, tt.pos, tt.ratio, tt.quality, result, tt.expected)
			}
		})
	}
}

// TestChannelSign_Breakup 测试 breakup 场景
func TestChannelSign_Breakup(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		pos      string
		ratio    float64
		quality  float64
		expected float64
	}{
		{
			name:     "flat+breakup高质量应降级到0.50",
			dir:      "flat",
			pos:      "breakup",
			ratio:    0.5,
			quality:  0.70,
			expected: 0.50,
		},
		{
			name:     "flat+breakup低质量应降级到0.30",
			dir:      "flat",
			pos:      "breakup",
			ratio:    0.5,
			quality:  0.40,
			expected: 0.30,
		},
		{
			name:     "up+breakup高质量应返回1.0",
			dir:      "up",
			pos:      "breakup",
			ratio:    0.5,
			quality:  0.70,
			expected: 1.0,
		},
		{
			name:     "down+breakup高质量应返回1.0（反转信号）",
			dir:      "down",
			pos:      "breakup",
			ratio:    0.5,
			quality:  0.70,
			expected: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := channelSign(tt.dir, tt.pos, tt.ratio, tt.quality)
			// 使用容差比较避免浮点数精度问题
			epsilon := 0.0001
			if abs(result-tt.expected) > epsilon {
				t.Errorf("channelSign(%s, %s, %.2f, %.2f) = %.4f, 期望 %.4f",
					tt.dir, tt.pos, tt.ratio, tt.quality, result, tt.expected)
			}
		})
	}
}

// TestChannelSign_Inside 测试 inside 位置（通道内部）
func TestChannelSign_Inside(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		pos      string
		ratio    float64
		quality  float64
		expected float64
	}{
		{
			name:     "up通道+inside+接近下轨(0.2)应强看涨",
			dir:      "up",
			pos:      "inside",
			ratio:    0.2,
			quality:  0.80,
			expected: 0.80, // 1.0 * 0.80
		},
		{
			name:     "up通道+inside+接近上轨(0.8)应弱看涨",
			dir:      "up",
			pos:      "inside",
			ratio:    0.8,
			quality:  0.80,
			expected: 0.40, // 0.5 * 0.80
		},
		{
			name:     "up通道+inside+中间位置(0.5)应正常看涨",
			dir:      "up",
			pos:      "inside",
			ratio:    0.5,
			quality:  0.80,
			expected: 0.64, // 0.8 * 0.80
		},
		{
			name:     "down通道+inside+接近上轨(0.8)应强看跌",
			dir:      "down",
			pos:      "inside",
			ratio:    0.8,
			quality:  0.80,
			expected: -0.80, // -1.0 * 0.80
		},
		{
			name:     "down通道+inside+接近下轨(0.2)应弱看跌",
			dir:      "down",
			pos:      "inside",
			ratio:    0.2,
			quality:  0.80,
			expected: -0.24, // -0.3 * 0.80
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := channelSign(tt.dir, tt.pos, tt.ratio, tt.quality)
			// 使用容差比较避免浮点数精度问题
			epsilon := 0.0001
			if abs(result-tt.expected) > epsilon {
				t.Errorf("channelSign(%s, %s, %.2f, %.2f) = %.4f, 期望 %.4f",
					tt.dir, tt.pos, tt.ratio, tt.quality, result, tt.expected)
			}
		})
	}
}

// TestChannelSign_LowerUpperMiddle 测试 lower/upper/middle 位置
func TestChannelSign_LowerUpperMiddle(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		pos      string
		ratio    float64
		quality  float64
		expected float64
	}{
		{
			name:     "down通道+lower应弱看跌（反弹风险）",
			dir:      "down",
			pos:      "lower",
			ratio:    0.5,
			quality:  0.80,
			expected: -0.16, // -0.2 * 0.80
		},
		{
			name:     "up通道+lower应强看涨（买入机会）",
			dir:      "up",
			pos:      "lower",
			ratio:    0.5,
			quality:  0.80,
			expected: 0.72, // 0.9 * 0.80
		},
		{
			name:     "up通道+upper应弱看涨（回调风险）",
			dir:      "up",
			pos:      "upper",
			ratio:    0.5,
			quality:  0.80,
			expected: 0.32, // 0.4 * 0.80
		},
		{
			name:     "down通道+upper应强看跌（卖出机会）",
			dir:      "down",
			pos:      "upper",
			ratio:    0.5,
			quality:  0.80,
			expected: -0.72, // -0.9 * 0.80
		},
		{
			name:     "up通道+middle应正常看涨",
			dir:      "up",
			pos:      "middle",
			ratio:    0.5,
			quality:  0.80,
			expected: 0.48, // 1.0 * 0.6 * 0.80
		},
		{
			name:     "down通道+middle应正常看跌",
			dir:      "down",
			pos:      "middle",
			ratio:    0.5,
			quality:  0.80,
			expected: -0.48, // -1.0 * 0.6 * 0.80
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := channelSign(tt.dir, tt.pos, tt.ratio, tt.quality)
			// 使用容差比较避免浮点数精度问题
			epsilon := 0.0001
			if abs(result-tt.expected) > epsilon {
				t.Errorf("channelSign(%s, %s, %.2f, %.2f) = %.4f, 期望 %.4f",
					tt.dir, tt.pos, tt.ratio, tt.quality, result, tt.expected)
			}
		})
	}
}

// TestMultiTFBreakdown 测试多时间框架 flat+breakdown 检查逻辑
func TestMultiTFBreakdown(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("单个时间框架flat+breakdown不应拦截", func(t *testing.T) {
		input := RootSymbolInput{
			Symbol: "TESTUSDT",
			MTF: MTFAnalysis{
				"15m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "flat",
						CurrentPosition: "breakdown",
						Quality:         0.60,
					},
				},
				"30m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "down",
						CurrentPosition: "middle",
						Quality:         0.65,
					},
				},
			},
			Orderflow: createTestOrderflow("down"),
		}

		result := ComputeDirectionArbitration(input, cfg)
		if result.BlockEntry && result.BlockReason == "MULTI_TF_FLAT_BREAKDOWN:15m" {
			t.Errorf("单个时间框架不应触发多时间框架拦截")
		}
	})

	t.Run("两个时间框架flat+breakdown应拦截", func(t *testing.T) {
		input := RootSymbolInput{
			Symbol: "TESTUSDT",
			MTF: MTFAnalysis{
				"15m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "flat",
						CurrentPosition: "breakdown",
						Quality:         0.60,
					},
				},
				"30m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "flat",
						CurrentPosition: "breakdown",
						Quality:         0.65,
					},
				},
			},
			Orderflow: createTestOrderflow("down"),
		}

		result := ComputeDirectionArbitration(input, cfg)
		if !result.BlockEntry {
			t.Errorf("两个时间框架flat+breakdown应该被拦截，但BlockEntry=false")
		}
		if result.BlockReason != "MULTI_TF_FLAT_BREAKDOWN:15m+30m" {
			t.Errorf("期望拦截原因为MULTI_TF_FLAT_BREAKDOWN:15m+30m，实际为: %s", result.BlockReason)
		}
	})

	t.Run("低质量通道不应计入", func(t *testing.T) {
		input := RootSymbolInput{
			Symbol: "TESTUSDT",
			MTF: MTFAnalysis{
				"15m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "flat",
						CurrentPosition: "breakdown",
						Quality:         0.30, // 低质量 < 0.40
					},
				},
				"30m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "flat",
						CurrentPosition: "breakdown",
						Quality:         0.65,
					},
				},
			},
			Orderflow: createTestOrderflow("down"),
		}

		result := ComputeDirectionArbitration(input, cfg)
		// 只有一个高质量的flat+breakdown，不应拦截
		if result.BlockEntry && result.BlockReason == "MULTI_TF_FLAT_BREAKDOWN:30m" {
			t.Errorf("低质量通道不应计入，单个高质量通道不应触发拦截")
		}
	})

	t.Run("三个时间框架flat+breakdown应拦截", func(t *testing.T) {
		input := RootSymbolInput{
			Symbol: "TESTUSDT",
			MTF: MTFAnalysis{
				"15m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "flat",
						CurrentPosition: "breakdown",
						Quality:         0.60,
					},
				},
				"30m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "sideways", // sideways也应该被识别
						CurrentPosition: "breakdown",
						Quality:         0.65,
					},
				},
				"1h": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "flat",
						CurrentPosition: "breakdown",
						Quality:         0.70,
					},
				},
			},
			Orderflow: createTestOrderflow("down"),
		}

		result := ComputeDirectionArbitration(input, cfg)
		if !result.BlockEntry {
			t.Errorf("三个时间框架flat+breakdown应该被拦截，但BlockEntry=false")
		}
		if result.BlockReason != "MULTI_TF_FLAT_BREAKDOWN:15m+30m+1h" {
			t.Errorf("期望拦截原因为MULTI_TF_FLAT_BREAKDOWN:15m+30m+1h，实际为: %s", result.BlockReason)
		}
	})
}

// Test1HConflict 测试1h矛盾信号检查逻辑
func Test1HConflict(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("SHORT判断+1h_breakup应拦截", func(t *testing.T) {
		input := RootSymbolInput{
			Symbol: "TESTUSDT",
			MTF: MTFAnalysis{
				"15m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "down",
						CurrentPosition: "middle",
						Quality:         0.60,
					},
				},
				"1h": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "down",
						CurrentPosition: "breakup", // 矛盾信号
						Quality:         0.70,      // 高质量
					},
				},
			},
			Orderflow: createTestOrderflow("down"),
		}

		result := ComputeDirectionArbitration(input, cfg)
		if !result.BlockEntry {
			t.Errorf("SHORT判断+1h breakup应该被拦截，但BlockEntry=false")
		}
		if result.BlockReason != "1H_BREAKUP_CONFLICT" {
			t.Errorf("期望拦截原因为1H_BREAKUP_CONFLICT，实际为: %s", result.BlockReason)
		}
	})

	t.Run("LONG判断+1h_breakdown应拦截", func(t *testing.T) {
		input := RootSymbolInput{
			Symbol: "TESTUSDT",
			MTF: MTFAnalysis{
				"15m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "flat",
						CurrentPosition: "breakup", // 向上突破
						Quality:         0.60,
					},
				},
				"30m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "flat",
						CurrentPosition: "upper",
						Quality:         0.65,
					},
				},
				"1h": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "up",
						CurrentPosition: "breakdown", // 矛盾信号
						Quality:         0.70,        // 高质量
					},
				},
			},
			Orderflow: createTestOrderflowBullish(), // 使用强烈看涨的订单流
		}

		result := ComputeDirectionArbitration(input, cfg)
		t.Logf("实际判断方向: %s, 是否拦截: %v, 拦截原因: %s", result.PlanSide, result.BlockEntry, result.BlockReason)
		if !result.BlockEntry {
			t.Errorf("LONG判断+1h breakdown应该被拦截，但BlockEntry=false")
		}
		if result.BlockReason != "1H_BREAKDOWN_CONFLICT" {
			t.Errorf("期望拦截原因为1H_BREAKDOWN_CONFLICT，实际为: %s", result.BlockReason)
		}
	})

	t.Run("低质量1h通道不应拦截", func(t *testing.T) {
		input := RootSymbolInput{
			Symbol: "TESTUSDT",
			MTF: MTFAnalysis{
				"15m": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "down",
						CurrentPosition: "middle",
						Quality:         0.60,
					},
				},
				"1h": TimeframeData{
					Channel: ChannelInfo{
						Direction:       "down",
						CurrentPosition: "breakup",
						Quality:         0.40, // 低质量 < 0.50
					},
				},
			},
			Orderflow: createTestOrderflow("down"),
		}

		result := ComputeDirectionArbitration(input, cfg)
		// 低质量通道不应触发1h矛盾信号拦截
		if result.BlockEntry && result.BlockReason == "1H_BREAKUP_CONFLICT" {
			t.Errorf("低质量1h通道不应触发矛盾信号拦截")
		}
	})
}

// createTestOrderflow 创建测试用的订单流数据
func createTestOrderflow(trendAlign string) Orderflow {
	return Orderflow{
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
			TrendAlignment:  trendAlign,
			SignalStrength:  60,
			ConfidenceLevel: 0.65,
			MarketRegime:    "trending",
			CvdDivergence:   false,
		},
		Micro5m: Micro5m{
			CandleIntent:      "bearish",
			FuturesCvdDelta:   -50000,
			SpotCvdDelta:      -30000,
			OIDeltaPct:        -0.5,
			PriceDeltaPct:     -0.3,
			VolumeDelta:       1000000,
			DataQuality:       0.70,
		},
		OB: Orderbook{
			ImbalanceRatio:   -0.3,
			BidPressure:      0.4,
			AskPressure:      0.6,
			LiquidityScore:   0.75,
			SpoofingRisk:     0.30,
			SupportWall:      nil,
			ResistanceWall:   nil,
			WallChange5m:     10,
		},
	}
}

// createTestOrderflowBullish 创建强烈看涨的订单流数据
func createTestOrderflowBullish() Orderflow {
	return Orderflow{
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
			TrendAlignment:  "up",
			SignalStrength:  60,
			ConfidenceLevel: 0.65,
			MarketRegime:    "trending",
			CvdDivergence:   false,
		},
		Micro5m: Micro5m{
			CandleIntent:      "bullish",
			FuturesCvdDelta:   50000,
			SpotCvdDelta:      30000,
			OIDeltaPct:        0.5,
			PriceDeltaPct:     0.3,
			VolumeDelta:       1000000,
			DataQuality:       0.70,
		},
		OB: Orderbook{
			ImbalanceRatio:   0.3,
			BidPressure:      0.6,
			AskPressure:      0.4,
			LiquidityScore:   0.75,
			SpoofingRisk:     0.30,
			SupportWall:      nil,
			ResistanceWall:   nil,
			WallChange5m:     10,
		},
	}
}
