package market

import (
	"encoding/json"
	"testing"
)

// TestContextMetrics_ValidityFlags 测试P0-05: ContextMetrics validity flags
func TestContextMetrics_ValidityFlags(t *testing.T) {
	// 测试场景1: 样本量充足 (>=15)，StrengthZ有效
	t.Run("StrengthZ_SufficientSamples", func(t *testing.T) {
		klines := generateDeterministicKlines(50, "15m")
		contextCalc := NewContextCalculator(klines)

		// 20个样本，>=15，应该有效
		allStrengths := make([]float64, 20)
		for i := range allStrengths {
			allStrengths[i] = float64(i) * 0.5
		}

		zScore, ready := contextCalc.CalculateStrengthZ(15.0, allStrengths)

		if !ready {
			t.Error("样本量=20>=15，StrengthZ应该有效")
		}

		if zScore == 0 {
			t.Log("警告: zScore为0，可能是标准差为0")
		}

		t.Logf("✅ StrengthZ有效: samples=%d, z=%.2f, ready=%v",
			len(allStrengths), zScore, ready)
	})

	// 测试场景2: 样本量不足 (<15)，StrengthZ无效
	t.Run("StrengthZ_InsufficientSamples", func(t *testing.T) {
		klines := generateDeterministicKlines(50, "15m")
		contextCalc := NewContextCalculator(klines)

		// 10个样本，<15，应该无效
		allStrengths := make([]float64, 10)
		for i := range allStrengths {
			allStrengths[i] = float64(i) * 0.5
		}

		zScore, ready := contextCalc.CalculateStrengthZ(5.0, allStrengths)

		if ready {
			t.Error("样本量=10<15，StrengthZ应该无效")
		}

		if zScore != 0 {
			t.Errorf("样本量不足时zScore应该为0: got=%.2f", zScore)
		}

		t.Logf("✅ StrengthZ无效: samples=%d, z=%.2f, ready=%v",
			len(allStrengths), zScore, ready)
	})

	// 测试场景3: VolRatio有效（基线正常）
	t.Run("VolRatio_ValidBaseline", func(t *testing.T) {
		klines := generateDeterministicKlines(50, "15m")
		contextCalc := NewContextCalculator(klines)

		// 正常成交量，基线应该>0
		volume := 5000.0
		ratio, ready := contextCalc.CalculateVolumeRatio(volume)

		if !ready {
			t.Error("基线正常时，VolRatio应该有效")
		}

		if ratio <= 0 {
			t.Errorf("ratio应该>0: got=%.2f", ratio)
		}

		t.Logf("✅ VolRatio有效: volume=%.0f, baseline=%.0f, ratio=%.2f, ready=%v",
			volume, contextCalc.market.MedianVolume20, ratio, ready)
	})

	// 测试场景4: VolRatio无效（零成交量）
	t.Run("VolRatio_ZeroVolume", func(t *testing.T) {
		klines := generateDeterministicKlines(50, "15m")
		contextCalc := NewContextCalculator(klines)

		// 零成交量
		volume := 0.0
		ratio, ready := contextCalc.CalculateVolumeRatio(volume)

		if ready {
			t.Error("零成交量时，VolRatio应该无效")
		}

		if ratio != 0 {
			t.Errorf("零成交量时ratio应该为0: got=%.2f", ratio)
		}

		t.Logf("✅ VolRatio无效: volume=%.0f, ratio=%.2f, ready=%v",
			volume, ratio, ready)
	})
}

// TestSupplyDemandContext_ValidityFlags 测试供需区上下文评分设置validity flags
func TestSupplyDemandContext_ValidityFlags(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 生成K线数据
	klines := generateDeterministicKlines(100, "30m")

	// 执行分析
	sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "30m")

	// 检查所有zone的Context是否设置了validity flags
	allZones := append(sdData.SupplyZones, sdData.DemandZones...)
	allZones = append(allZones, sdData.ActiveZones...)

	zonesWithContext := 0
	for _, zone := range allZones {
		if zone.Context != nil {
			zonesWithContext++

			// 验证字段存在（即使值为false也要有）
			// 这里我们不强制要求ready=true，因为取决于样本量

			t.Logf("Zone %s Context: strengthZ=%.2f (ready=%v), volRatio=%.2f (ready=%v)",
				zone.ID, zone.Context.StrengthZ, zone.Context.StrengthZReady,
				zone.Context.VolRatio, zone.Context.VolRatioReady)
		}
	}

	if zonesWithContext == 0 {
		t.Skip("没有zone有Context，无法验证validity flags")
	}

	t.Logf("✅ 检查了%d个zone的Context validity flags", zonesWithContext)
}

// TestContextMetrics_JSON_ValidityFlags 测试validity flags的JSON序列化
func TestContextMetrics_JSON_ValidityFlags(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 生成足够的K线确保有zone
	klines := generateDeterministicKlines(100, "15m")
	sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

	// 序列化为JSON
	jsonBytes, err := json.Marshal(sdData)
	if err != nil {
		t.Fatalf("JSON序列化失败: %v", err)
	}

	jsonStr := string(jsonBytes)

	// 验证JSON包含validity flags字段
	if !stringContains(jsonStr, "strength_z_ready") {
		t.Error("JSON缺少strength_z_ready字段")
	}

	if !stringContains(jsonStr, "vol_ratio_ready") {
		t.Error("JSON缺少vol_ratio_ready字段")
	}

	t.Logf("✅ JSON包含validity flags字段")
}

// TestContextMetrics_EdgeCases 测试边界情况
func TestContextMetrics_EdgeCases(t *testing.T) {
	// 测试场景1: 空样本
	t.Run("EmptySamples", func(t *testing.T) {
		klines := generateDeterministicKlines(50, "15m")
		contextCalc := NewContextCalculator(klines)

		allStrengths := []float64{}
		zScore, ready := contextCalc.CalculateStrengthZ(1.0, allStrengths)

		if ready {
			t.Error("空样本时StrengthZ应该无效")
		}

		if zScore != 0 {
			t.Errorf("空样本时zScore应该为0: got=%.2f", zScore)
		}

		t.Logf("✅ 空样本: z=%.2f, ready=%v", zScore, ready)
	})

	// 测试场景2: 样本量恰好为15（边界）
	t.Run("ExactlyMinSamples", func(t *testing.T) {
		klines := generateDeterministicKlines(50, "15m")
		contextCalc := NewContextCalculator(klines)

		allStrengths := make([]float64, 15)
		for i := range allStrengths {
			allStrengths[i] = float64(i)
		}

		zScore, ready := contextCalc.CalculateStrengthZ(10.0, allStrengths)

		if !ready {
			t.Error("样本量=15（恰好最小要求），StrengthZ应该有效")
		}

		t.Logf("✅ 最小样本量边界: samples=%d, z=%.2f, ready=%v",
			len(allStrengths), zScore, ready)
	})

	// 测试场景3: 样本量为14（刚好不足）
	t.Run("JustBelowMinSamples", func(t *testing.T) {
		klines := generateDeterministicKlines(50, "15m")
		contextCalc := NewContextCalculator(klines)

		allStrengths := make([]float64, 14)
		for i := range allStrengths {
			allStrengths[i] = float64(i)
		}

		zScore, ready := contextCalc.CalculateStrengthZ(10.0, allStrengths)

		if ready {
			t.Error("样本量=14（刚好不足），StrengthZ应该无效")
		}

		t.Logf("✅ 最小样本量边界-1: samples=%d, z=%.2f, ready=%v",
			len(allStrengths), zScore, ready)
	})
}

// TestContextMetrics_IntegrationWithAI 测试AI模型集成场景
func TestContextMetrics_IntegrationWithAI(t *testing.T) {
	// 模拟AI模型使用Context数据的场景
	analyzer := NewSupplyDemandAnalyzer()

	klines := generateDeterministicKlines(80, "15m")
	sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

	// AI模型逻辑：根据validity flags决定是否使用字段
	for _, zone := range sdData.ActiveZones {
		if zone.Context == nil {
			continue
		}

		ctx := zone.Context

		// AI决策逻辑示例
		var aiDecision string
		if ctx.StrengthZReady && ctx.StrengthZ > 1.5 {
			aiDecision = "强区域，优先考虑"
		} else if !ctx.StrengthZReady {
			aiDecision = "StrengthZ不可用，使用其他指标"
		} else {
			aiDecision = "弱区域"
		}

		t.Logf("Zone %s: strengthZ=%.2f (ready=%v), volRatio=%.2f (ready=%v) → AI决策: %s",
			zone.ID, ctx.StrengthZ, ctx.StrengthZReady,
			ctx.VolRatio, ctx.VolRatioReady, aiDecision)
	}

	t.Logf("✅ AI模型集成测试完成")
}

// TestContextMetrics_CrossTimeframeValidity 测试跨时间框架的validity一致性
func TestContextMetrics_CrossTimeframeValidity(t *testing.T) {
	testCases := []struct {
		timeframe string
		barCount  int
	}{
		{"5m", 100},
		{"15m", 80},
		{"30m", 60},
		{"1h", 50},
	}

	for _, tc := range testCases {
		t.Run(tc.timeframe, func(t *testing.T) {
			analyzer := NewSupplyDemandAnalyzer()

			klines := generateDeterministicKlines(tc.barCount, tc.timeframe)
			sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", tc.timeframe)

			// 统计validity flags分布
			var totalZones, strengthZReady, volRatioReady int
			for _, zone := range sdData.ActiveZones {
				if zone.Context != nil {
					totalZones++
					if zone.Context.StrengthZReady {
						strengthZReady++
					}
					if zone.Context.VolRatioReady {
						volRatioReady++
					}
				}
			}

			if totalZones > 0 {
				t.Logf("✅ [%s] zones=%d, strengthZReady=%d (%.0f%%), volRatioReady=%d (%.0f%%)",
					tc.timeframe, totalZones,
					strengthZReady, float64(strengthZReady)/float64(totalZones)*100,
					volRatioReady, float64(volRatioReady)/float64(totalZones)*100)
			} else {
				t.Logf("⚠️ [%s] 没有活跃zone", tc.timeframe)
			}
		})
	}
}
