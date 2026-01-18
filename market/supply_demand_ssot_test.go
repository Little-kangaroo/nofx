package market

import (
	"testing"
)

// TestSupplyDemand_SSOT_LastAnalysis 测试P0-03: LastAnalysis使用klines时间而非time.Now()
func TestSupplyDemand_SSOT_LastAnalysis(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 生成测试K线
	klines := generateDeterministicKlines(50, "15m")
	expectedLastTime := klines[len(klines)-1].CloseTime

	// 执行分析
	result := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

	// 验证LastAnalysis使用klines的最后CloseTime
	if result.LastAnalysis != expectedLastTime {
		t.Errorf("LastAnalysis应该使用klines最后CloseTime: got=%d, want=%d",
			result.LastAnalysis, expectedLastTime)
	}

	t.Logf("✅ LastAnalysis正确使用klines时间: %d", result.LastAnalysis)
}

// TestSupplyDemand_SSOT_EmptyKlines 测试空K线场景的SSOT
func TestSupplyDemand_SSOT_EmptyKlines(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 空K线
	klines := []Kline{}

	result := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

	// 验证空K线时LastAnalysis为0而非time.Now()
	if result.LastAnalysis != 0 {
		t.Errorf("空K线时LastAnalysis应该为0: got=%d", result.LastAnalysis)
	}

	t.Logf("✅ 空K线场景LastAnalysis=0 (不使用time.Now())")
}

// TestSupplyDemand_SSOT_Determinism 测试确定性：相同输入多次调用产生相同LastAnalysis
func TestSupplyDemand_SSOT_Determinism(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 生成确定性K线
	klines := generateDeterministicKlines(60, "30m")

	// 多次执行分析
	results := make([]*SupplyDemandData, 5)
	for i := 0; i < 5; i++ {
		results[i] = analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "30m")
	}

	// 验证所有结果的LastAnalysis完全相同
	firstLastAnalysis := results[0].LastAnalysis
	for i := 1; i < 5; i++ {
		if results[i].LastAnalysis != firstLastAnalysis {
			t.Errorf("多次调用应产生相同LastAnalysis: run[0]=%d, run[%d]=%d",
				firstLastAnalysis, i, results[i].LastAnalysis)
		}
	}

	t.Logf("✅ 确定性验证通过: 5次调用均产生LastAnalysis=%d", firstLastAnalysis)
}

// TestSupplyDemand_SSOT_GenerateSignals 测试GenerateSignals使用sdData.LastAnalysis
func TestSupplyDemand_SSOT_GenerateSignals(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 生成K线和分析数据
	klines := generateDeterministicKlines(80, "1h")
	sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "1h")

	expectedTimestamp := sdData.LastAnalysis

	// 生成信号
	currentPrice := klines[len(klines)-1].Close
	signals := analyzer.GenerateSignals(sdData, currentPrice)

	// 如果有信号，验证时间戳使用sdData.LastAnalysis
	for i, signal := range signals {
		if signal.Timestamp != expectedTimestamp {
			t.Errorf("Signal[%d].Timestamp应该使用sdData.LastAnalysis: got=%d, want=%d",
				i, signal.Timestamp, expectedTimestamp)
		}
	}

	t.Logf("✅ GenerateSignals使用sdData.LastAnalysis: %d个信号", len(signals))
}

// TestSupplyDemand_SSOT_ValidateZonePositions 测试ValidateZonePositions保持SSOT
func TestSupplyDemand_SSOT_ValidateZonePositions(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 生成K线和分析数据
	klines := generateDeterministicKlines(100, "15m")
	sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

	originalLastAnalysis := sdData.LastAnalysis

	// 验证zone位置
	validatedData := analyzer.ValidateZonePositions(sdData, klines)

	// 验证LastAnalysis使用klines的最后时间
	expectedLastTime := klines[len(klines)-1].CloseTime
	if validatedData.LastAnalysis != expectedLastTime {
		t.Errorf("ValidateZonePositions应该使用klines最后时间: got=%d, want=%d",
			validatedData.LastAnalysis, expectedLastTime)
	}

	// 两个时间应该相同（因为klines未变）
	if validatedData.LastAnalysis != originalLastAnalysis {
		t.Errorf("相同klines应产生相同LastAnalysis: original=%d, validated=%d",
			originalLastAnalysis, validatedData.LastAnalysis)
	}

	t.Logf("✅ ValidateZonePositions保持SSOT: %d", validatedData.LastAnalysis)
}

// TestSupplyDemand_SSOT_CrossTimeframe 测试跨时间框架的SSOT一致性
func TestSupplyDemand_SSOT_CrossTimeframe(t *testing.T) {
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

			// 生成确定性K线
			klines := generateDeterministicKlines(tc.barCount, tc.timeframe)
			expectedLastTime := klines[len(klines)-1].CloseTime

			// 执行分析
			result := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", tc.timeframe)

			// 验证LastAnalysis
			if result.LastAnalysis != expectedLastTime {
				t.Errorf("[%s] LastAnalysis不匹配: got=%d, want=%d",
					tc.timeframe, result.LastAnalysis, expectedLastTime)
			}

			t.Logf("✅ [%s] SSOT验证: LastAnalysis=%d",
				tc.timeframe, result.LastAnalysis)
		})
	}
}

// TestSupplyDemand_SSOT_NoTimeNowUsage 验证不再使用time.Now()
func TestSupplyDemand_SSOT_NoTimeNowUsage(t *testing.T) {
	// 这个测试通过确定性验证间接证明没有使用time.Now()
	// 如果使用了time.Now()，多次调用会产生不同的时间戳

	analyzer := NewSupplyDemandAnalyzer()
	klines := generateDeterministicKlines(50, "15m")

	// 短时间内多次调用
	var timestamps []int64
	for i := 0; i < 10; i++ {
		result := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")
		timestamps = append(timestamps, result.LastAnalysis)
	}

	// 所有时间戳应该完全相同
	firstTimestamp := timestamps[0]
	for i, ts := range timestamps {
		if ts != firstTimestamp {
			t.Errorf("检测到time.Now()使用: run[0]=%d, run[%d]=%d (差异=%d ms)",
				firstTimestamp, i, ts, ts-firstTimestamp)
		}
	}

	t.Logf("✅ 未检测到time.Now()使用: 10次调用均产生相同时间戳=%d", firstTimestamp)
}

// TestSupplyDemand_SSOT_BacktestConsistency 测试回测一致性
func TestSupplyDemand_SSOT_BacktestConsistency(t *testing.T) {
	// 模拟回测场景：使用历史K线数据
	analyzer := NewSupplyDemandAnalyzer()

	// 生成"历史"K线（固定时间）
	baseTime := int64(1700000000000) // 2023-11-15的某个时间
	klines := make([]Kline, 50)
	for i := 0; i < 50; i++ {
		klines[i] = Kline{
			OpenTime:  baseTime + int64(i)*15*60*1000,
			CloseTime: baseTime + int64(i)*15*60*1000 + 15*60*1000 - 1,
			Open:      44000 + float64(i)*10,
			High:      44020 + float64(i)*10,
			Low:       43980 + float64(i)*10,
			Close:     44010 + float64(i)*10,
			Volume:    1000,
		}
	}

	expectedLastTime := klines[len(klines)-1].CloseTime

	// 执行"回测"分析
	result := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

	// 验证LastAnalysis使用历史K线时间，而非当前time.Now()
	if result.LastAnalysis != expectedLastTime {
		t.Errorf("回测场景LastAnalysis应该使用历史K线时间: got=%d, want=%d",
			result.LastAnalysis, expectedLastTime)
	}

	// 验证时间戳是历史时间（2023年），而非当前时间（2026年）
	currentYear2026Ms := int64(1737244800000) // 2026-01-19的大致时间
	if result.LastAnalysis > currentYear2026Ms {
		t.Error("回测场景使用了当前时间而非历史K线时间（检测到time.Now()使用）")
	}

	t.Logf("✅ 回测一致性验证通过: LastAnalysis=%d (历史时间)", result.LastAnalysis)
}

// generateDeterministicKlines 生成确定性K线（固定时间，用于测试SSOT）
func generateDeterministicKlines(count int, timeframe string) []Kline {
	klines := make([]Kline, count)
	basePrice := 44000.0
	baseTime := int64(1700000000000) // 固定基准时间

	intervalMs := int64(5 * 60 * 1000) // 默认5分钟
	switch timeframe {
	case "1m":
		intervalMs = 60 * 1000
	case "5m":
		intervalMs = 5 * 60 * 1000
	case "15m":
		intervalMs = 15 * 60 * 1000
	case "30m":
		intervalMs = 30 * 60 * 1000
	case "1h":
		intervalMs = 60 * 60 * 1000
	case "4h":
		intervalMs = 4 * 60 * 60 * 1000
	}

	for i := 0; i < count; i++ {
		// 确定性价格变化（基于索引，无随机性）
		priceOffset := float64(i%20-10) * 5.0
		price := basePrice + priceOffset

		klines[i] = Kline{
			OpenTime:    baseTime + int64(i)*intervalMs,
			CloseTime:   baseTime + int64(i)*intervalMs + intervalMs - 1,
			Open:        price,
			High:        price + 15.0,
			Low:         price - 15.0,
			Close:       price + float64(i%3-1)*3.0,
			Volume:      1000.0 + float64(i%10)*50.0,
			QuoteVolume: (price * (1000.0 + float64(i%10)*50.0)),
		}
	}

	return klines
}
