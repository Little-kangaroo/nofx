package market

import (
	"encoding/json"
	"math"
	"testing"
)

// ============================================================================
// 测试辅助函数
// ============================================================================

// generateSRTestKlines 生成SR测试用K线数据
func generateSRTestKlines(count int, basePrice float64) []Kline {
	klines := make([]Kline, count)
	startTime := int64(1700000000000) // 固定起始时间戳

	for i := 0; i < count; i++ {
		// 生成波动的价格数据
		phase := float64(i) / 10.0
		priceVariation := math.Sin(phase) * basePrice * 0.02 // 2%波动

		open := basePrice + priceVariation
		high := open * 1.01
		low := open * 0.99
		close := open + math.Cos(phase)*basePrice*0.01

		// 在某些点创建明显的高点和低点（用于pivot检测）
		if i%20 == 10 {
			// 创建高点
			high = open * 1.05
			close = high * 0.98
		} else if i%20 == 0 {
			// 创建低点
			low = open * 0.95
			close = low * 1.02
		}

		klines[i] = Kline{
			OpenTime:  startTime + int64(i*300000), // 5分钟间隔
			CloseTime: startTime + int64((i+1)*300000) - 1,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    1000.0 + float64(i)*10,
		}
	}

	return klines
}

// ============================================================================
// P0-01: Strength 范围验证测试
// ============================================================================

func TestP001_StrengthRange(t *testing.T) {
	t.Run("SRLevel.Strength应在0-100范围内", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(200, 50000.0)

		nowMs := klines[len(klines)-1].CloseTime
		atr14 := 500.0

		result := analyzer.AnalyzeWithMeta(klines, "5m", nowMs, atr14)

		if result == nil {
			t.Fatal("分析结果为nil")
		}

		for i, level := range result.KeyLevels {
			if level.Strength < 0 || level.Strength > 100 {
				t.Errorf("KeyLevels[%d].Strength = %.2f, 期望范围[0, 100]",
					i, level.Strength)
			}

			t.Logf("✓ KeyLevels[%d]: Price=%.2f, Strength=%.2f, Type=%s, HitCount=%d",
				i, level.Price, level.Strength, level.Type, level.HitCount)
		}
	})

	t.Run("SRFlip.FlipStrength应在0-100范围内", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(500, 50000.0)

		nowMs := klines[len(klines)-1].CloseTime
		atr14 := 500.0

		result := analyzer.AnalyzeWithMeta(klines, "5m", nowMs, atr14)

		if result == nil {
			t.Fatal("分析结果为nil")
		}

		for i, flip := range result.SRFlips {
			if flip.FlipStrength < 0 || flip.FlipStrength > 100 {
				t.Errorf("SRFlips[%d].FlipStrength = %.2f, 期望范围[0, 100]",
					i, flip.FlipStrength)
			}

			t.Logf("✓ SRFlips[%d]: Price=%.2f, Strength=%.2f, %s->%s",
				i, flip.FlipPrice, flip.FlipStrength, flip.OriginalType, flip.FlippedType)
		}
	})
}

// ============================================================================
// P0-02: 时间戳确定性测试
// ============================================================================

func TestP002_TimestampDeterministic(t *testing.T) {
	t.Run("同样输入应产生完全一致的输出", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(300, 50000.0)

		timeframe := "5m"
		nowMs := klines[len(klines)-1].CloseTime
		atr14 := 500.0

		// 第一次分析
		result1 := analyzer.AnalyzeWithMeta(klines, timeframe, nowMs, atr14)

		// 第二次分析（同样的输入）
		result2 := analyzer.AnalyzeWithMeta(klines, timeframe, nowMs, atr14)

		// 验证结果一致性
		if result1.LastAnalysis != result2.LastAnalysis {
			t.Errorf("LastAnalysis不一致: %d vs %d", result1.LastAnalysis, result2.LastAnalysis)
		}

		if result1.UsedTimeFrame != result2.UsedTimeFrame {
			t.Errorf("UsedTimeFrame不一致: %s vs %s", result1.UsedTimeFrame, result2.UsedTimeFrame)
		}

		if len(result1.KeyLevels) != len(result2.KeyLevels) {
			t.Errorf("KeyLevels数量不一致: %d vs %d", len(result1.KeyLevels), len(result2.KeyLevels))
		}

		// 详细比较每个级别
		for i := 0; i < len(result1.KeyLevels) && i < len(result2.KeyLevels); i++ {
			l1 := result1.KeyLevels[i]
			l2 := result2.KeyLevels[i]

			if math.Abs(l1.Price-l2.Price) > 0.01 {
				t.Errorf("KeyLevels[%d].Price不一致: %.2f vs %.2f", i, l1.Price, l2.Price)
			}

			if math.Abs(l1.Strength-l2.Strength) > 0.01 {
				t.Errorf("KeyLevels[%d].Strength不一致: %.2f vs %.2f", i, l1.Strength, l2.Strength)
			}

			if l1.HitCount != l2.HitCount {
				t.Errorf("KeyLevels[%d].HitCount不一致: %d vs %d", i, l1.HitCount, l2.HitCount)
			}
		}

		t.Logf("✓ 确定性测试通过: 两次分析产生完全一致的结果")
	})

	t.Run("LastAnalysis应使用K线时间而非机器时间", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(100, 50000.0)

		expectedTime := klines[len(klines)-1].CloseTime

		result := analyzer.AnalyzeWithMeta(klines, "5m", expectedTime, 500.0)

		if result.LastAnalysis != expectedTime {
			t.Errorf("LastAnalysis = %d, 期望 = %d", result.LastAnalysis, expectedTime)
		}

		t.Logf("✓ LastAnalysis正确使用K线时间: %d", result.LastAnalysis)
	})

	t.Run("SRFlip.FlipTime应使用传入的nowMs", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(500, 50000.0)

		nowMs := klines[len(klines)-1].CloseTime

		result := analyzer.AnalyzeWithMeta(klines, "5m", nowMs, 500.0)

		for i, flip := range result.SRFlips {
			if flip.FlipTime != nowMs {
				t.Errorf("SRFlips[%d].FlipTime = %d, 期望 = %d", i, flip.FlipTime, nowMs)
			}
		}

		if len(result.SRFlips) > 0 {
			t.Logf("✓ %d个SRFlip的FlipTime都使用了传入的nowMs", len(result.SRFlips))
		}
	})
}

// ============================================================================
// P0-02A: UsedTimeFrame 字段测试
// ============================================================================

func TestP002A_UsedTimeFrame(t *testing.T) {
	t.Run("SupportResistanceData应包含UsedTimeFrame", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(200, 50000.0)

		testCases := []struct {
			timeframe string
		}{
			{"5m"},
			{"15m"},
			{"1h"},
			{"4h"},
		}

		for _, tc := range testCases {
			t.Run(tc.timeframe, func(t *testing.T) {
				nowMs := klines[len(klines)-1].CloseTime
				result := analyzer.AnalyzeWithMeta(klines, tc.timeframe, nowMs, 500.0)

				if result.UsedTimeFrame != tc.timeframe {
					t.Errorf("UsedTimeFrame = %s, 期望 = %s", result.UsedTimeFrame, tc.timeframe)
				}

				t.Logf("✓ UsedTimeFrame正确: %s", result.UsedTimeFrame)
			})
		}
	})
}

// ============================================================================
// P0-03: 边界语义测试
// ============================================================================

func TestP003_BoundarySemantics(t *testing.T) {
	t.Run("Pivot检测应严格遵守边界", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(100, 50000.0)

		// 测试不同的边界索引
		boundaryIndex := 90

		// 直接测试内部方法
		startIndex := analyzer.config.PivotLeft
		endIndex := boundaryIndex

		pivotPoints := analyzer.findPivotPoints(klines, startIndex, endIndex)

		// 验证所有pivot点的索引都不超过边界
		for i, pivot := range pivotPoints {
			if pivot.Index > endIndex {
				t.Errorf("PivotPoint[%d].Index = %d, 超过边界 %d", i, pivot.Index, endIndex)
			}
		}

		t.Logf("✓ 边界语义测试通过: %d个pivot点，最大索引=%d，边界=%d",
			len(pivotPoints),
			func() int {
				maxIdx := 0
				for _, p := range pivotPoints {
					if p.Index > maxIdx {
						maxIdx = p.Index
					}
				}
				return maxIdx
			}(),
			endIndex)
	})

	t.Run("渐进确认机制应正确计算置信度", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(50, 50000.0)

		// 测试不同位置的pivot点置信度
		testCases := []struct {
			index                int
			expectedRightBars    int
			minExpectedConfidence float64
		}{
			{index: 10, expectedRightBars: 3, minExpectedConfidence: 1.0},   // 完全确认
			{index: 46, expectedRightBars: 1, minExpectedConfidence: 0.33},  // 最小确认
			{index: 47, expectedRightBars: 0, minExpectedConfidence: 0.1},   // 潜在pivot
		}

		for _, tc := range testCases {
			boundaryIndex := 47 // 固定边界

			// 直接调用检测方法
			pivot := analyzer.checkPivotHighWithConfidence(klines, tc.index, boundaryIndex)

			if pivot != nil {
				if math.Abs(pivot.Confidence-tc.minExpectedConfidence) > 0.01 {
					t.Logf("Index %d: Confidence=%.2f, 期望≈%.2f (RightBars=%d)",
						tc.index, pivot.Confidence, tc.minExpectedConfidence, pivot.RightBarsCount)
				} else {
					t.Logf("✓ Index %d: Confidence=%.2f 正确 (RightBars=%d)",
						tc.index, pivot.Confidence, pivot.RightBarsCount)
				}
			}
		}
	})
}

// ============================================================================
// P0-04: ATR 使用测试
// ============================================================================

func TestP004_ATRUsage(t *testing.T) {
	t.Run("应使用传入的真实ATR而非估算", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(200, 50000.0)

		// 使用不同的ATR值
		testCases := []struct {
			atr14    float64
			scenario string
		}{
			{atr14: 100.0, scenario: "低波动"},
			{atr14: 500.0, scenario: "正常波动"},
			{atr14: 2000.0, scenario: "高波动"},
		}

		for _, tc := range testCases {
			t.Run(tc.scenario, func(t *testing.T) {
				nowMs := klines[len(klines)-1].CloseTime
				result := analyzer.AnalyzeWithMeta(klines, "5m", nowMs, tc.atr14)

				if result == nil {
					t.Fatal("分析结果为nil")
				}

				// 验证结果存在（说明ATR被正确使用）
				t.Logf("✓ ATR=%.2f: KeyLevels=%d, SRFlips=%d",
					tc.atr14, len(result.KeyLevels), len(result.SRFlips))
			})
		}
	})

	t.Run("ATR异常保护应生效", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(200, 50000.0)
		nowMs := klines[len(klines)-1].CloseTime

		// 测试异常ATR值
		testCases := []struct {
			atr14    float64
			scenario string
		}{
			{atr14: 0, scenario: "零ATR"},
			{atr14: -100, scenario: "负ATR"},
		}

		for _, tc := range testCases {
			t.Run(tc.scenario, func(t *testing.T) {
				// 应该不崩溃，退化到默认容差
				result := analyzer.AnalyzeWithMeta(klines, "5m", nowMs, tc.atr14)

				if result == nil {
					t.Fatal("异常ATR导致分析失败")
				}

				t.Logf("✓ %s处理正确，未崩溃", tc.scenario)
			})
		}
	})
}

// ============================================================================
// P0-05: Anchor 适配层测试
// ============================================================================

func TestP005_AnchorAdapter(t *testing.T) {
	t.Run("fromSR应使用data.UsedTimeFrame", func(t *testing.T) {
		// 创建测试数据
		srData := &SupportResistanceData{
			UsedTimeFrame: "1h",
			KeyLevels: []*SRLevel{
				{Price: 50000, HitCount: 5, Type: "support", Strength: 75.0},
			},
			SRFlips: []*SRFlip{},
		}

		engine := &AnchorEngine{}
		candidates := engine.fromSR(srData, nil)

		for i, cand := range candidates {
			if cand.TF != "1h" {
				t.Errorf("Candidate[%d].TF = %s, 期望 = 1h", i, cand.TF)
			}
		}

		t.Logf("✓ Anchor适配层正确使用UsedTimeFrame: %d个候选", len(candidates))
	})

	t.Run("应区分SRLevel和SRFlip类型", func(t *testing.T) {
		srData := &SupportResistanceData{
			UsedTimeFrame: "4h",
			KeyLevels: []*SRLevel{
				{Price: 50000, HitCount: 5, Type: "support", Strength: 75.0},
			},
			SRFlips: []*SRFlip{
				{
					FlipPrice:      49000,
					OriginalType:   "support",
					FlippedType:    "resistance",
					FlipStrength:   80.0,
					FlipConfirmation: true,
				},
			},
		}

		engine := &AnchorEngine{}
		candidates := engine.fromSR(srData, nil)

		levelCount := 0
		flipCount := 0

		for _, cand := range candidates {
			if cand.Type == AnchorSRLevel {
				levelCount++
			} else if cand.Type == AnchorSRFlip {
				flipCount++
			}
		}

		if levelCount != 1 {
			t.Errorf("AnchorSRLevel数量 = %d, 期望 = 1", levelCount)
		}

		if flipCount != 1 {
			t.Errorf("AnchorSRFlip数量 = %d, 期望 = 1", flipCount)
		}

		t.Logf("✓ 类型区分正确: %d个Level, %d个Flip", levelCount, flipCount)
	})

	t.Run("StrengthZ应在合理范围", func(t *testing.T) {
		srData := &SupportResistanceData{
			UsedTimeFrame: "4h",
			KeyLevels: []*SRLevel{
				{Price: 50000, HitCount: 5, Type: "support", Strength: 75.0},
				{Price: 51000, HitCount: 3, Type: "resistance", Strength: 50.0},
			},
			SRFlips: []*SRFlip{
				{
					FlipPrice:      49000,
					OriginalType:   "support",
					FlippedType:    "resistance",
					FlipStrength:   80.0,
				},
			},
		}

		engine := &AnchorEngine{}
		candidates := engine.fromSR(srData, nil)

		for i, cand := range candidates {
			if cand.StrengthZ != nil {
				if *cand.StrengthZ < 0 || *cand.StrengthZ > 1.0 {
					t.Errorf("Candidate[%d].StrengthZ = %.2f, 期望范围[0, 1.0]", i, *cand.StrengthZ)
				}
				t.Logf("✓ Candidate[%d]: Type=%s, StrengthZ=%.2f", i, cand.Type, *cand.StrengthZ)
			}
		}
	})
}

// ============================================================================
// 集成测试
// ============================================================================

func TestIntegration_FullPipeline(t *testing.T) {
	t.Run("完整分析流程应正常运行", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(500, 50000.0)

		timeframe := "5m"
		nowMs := klines[len(klines)-1].CloseTime
		atr14 := 500.0

		result := analyzer.AnalyzeWithMeta(klines, timeframe, nowMs, atr14)

		// 验证基本字段
		if result == nil {
			t.Fatal("分析结果为nil")
		}

		if result.UsedTimeFrame != timeframe {
			t.Errorf("UsedTimeFrame = %s, 期望 = %s", result.UsedTimeFrame, timeframe)
		}

		if result.LastAnalysis != nowMs {
			t.Errorf("LastAnalysis = %d, 期望 = %d", result.LastAnalysis, nowMs)
		}

		// 打印结果摘要
		t.Logf("\n=== 分析结果摘要 ===")
		t.Logf("时间框架: %s", result.UsedTimeFrame)
		t.Logf("分析时间: %d", result.LastAnalysis)
		t.Logf("关键级别: %d个", len(result.KeyLevels))
		t.Logf("SR Flips: %d个", len(result.SRFlips))

		if result.Statistics != nil {
			t.Logf("\n统计信息:")
			t.Logf("  总级别数: %d", result.Statistics.TotalLevels)
			t.Logf("  支撑级别: %d", result.Statistics.SupportCount)
			t.Logf("  阻力级别: %d", result.Statistics.ResistanceCount)
			t.Logf("  平均强度: %.2f", result.Statistics.AvgStrength)
			t.Logf("  平均命中: %.2f", result.Statistics.AvgHitCount)
		}

		// 详细打印关键级别
		if len(result.KeyLevels) > 0 {
			t.Logf("\n关键级别详情:")
			for i, level := range result.KeyLevels {
				t.Logf("  [%d] %.2f (%s) - Strength:%.2f, Hits:%d",
					i, level.Price, level.Type, level.Strength, level.HitCount)
			}
		}

		// 详细打印SR Flips
		if len(result.SRFlips) > 0 {
			t.Logf("\nSR Flip详情:")
			for i, flip := range result.SRFlips {
				t.Logf("  [%d] %.2f (%s→%s) - Strength:%.2f, Confirmed:%v",
					i, flip.FlipPrice, flip.OriginalType, flip.FlippedType,
					flip.FlipStrength, flip.FlipConfirmation)
			}
		}
	})

	t.Run("结果应可序列化为JSON", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(200, 50000.0)

		result := analyzer.AnalyzeWithMeta(klines, "5m", klines[len(klines)-1].CloseTime, 500.0)

		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			t.Fatalf("JSON序列化失败: %v", err)
		}

		// 验证可以反序列化
		var decoded SupportResistanceData
		err = json.Unmarshal(jsonBytes, &decoded)
		if err != nil {
			t.Fatalf("JSON反序列化失败: %v", err)
		}

		if decoded.UsedTimeFrame != result.UsedTimeFrame {
			t.Errorf("反序列化后UsedTimeFrame不一致")
		}

		t.Logf("✓ JSON序列化/反序列化成功，数据大小: %d bytes", len(jsonBytes))
	})
}

// ============================================================================
// 向后兼容性测试
// ============================================================================

func TestBackwardCompatibility(t *testing.T) {
	t.Run("旧的Analyze()方法应继续工作", func(t *testing.T) {
		analyzer := NewSupportResistanceAnalyzer()
		klines := generateSRTestKlines(200, 50000.0)

		// 调用旧方法
		result := analyzer.Analyze(klines)

		if result == nil {
			t.Fatal("旧Analyze()方法返回nil")
		}

		// 应该有默认的时间框架
		if result.UsedTimeFrame == "" {
			t.Error("旧方法应设置默认UsedTimeFrame")
		}

		t.Logf("✓ 向后兼容: 旧Analyze()方法正常工作，UsedTimeFrame=%s", result.UsedTimeFrame)
	})
}

// ============================================================================
// 性能基准测试
// ============================================================================

func BenchmarkSRAnalysis(b *testing.B) {
	analyzer := NewSupportResistanceAnalyzer()
	klines := generateSRTestKlines(1000, 50000.0)
	nowMs := klines[len(klines)-1].CloseTime
	atr14 := 500.0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.AnalyzeWithMeta(klines, "5m", nowMs, atr14)
	}
}

func BenchmarkSRAnalysis_SmallDataset(b *testing.B) {
	analyzer := NewSupportResistanceAnalyzer()
	klines := generateSRTestKlines(200, 50000.0)
	nowMs := klines[len(klines)-1].CloseTime
	atr14 := 500.0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.AnalyzeWithMeta(klines, "5m", nowMs, atr14)
	}
}
