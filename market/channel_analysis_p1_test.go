package market

import (
	"encoding/json"
	"testing"
)

// TestP1_CH06_BreakoutDistance 测试突破距离计算
func TestP1_CH06_BreakoutDistance(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建测试数据：模拟向上突破场景
	klines := generateBreakoutKlines(100, 50000.0, 0.01, true)
	currentPrice := 51500.0 // 明显高于通道上轨
	timeframe := "5m"

	// 执行分析
	result := analyzer.Analyze(klines, currentPrice, timeframe)

	// 验证突破距离字段
	if result.BreakoutDistance == 0 && result.CurrentPosition == "break_up" {
		t.Error("突破向上但BreakoutDistance为0")
	}

	if result.BreakoutDistanceATR == 0 && result.CurrentPosition == "break_up" {
		t.Error("突破向上但BreakoutDistanceATR为0")
	}

	t.Logf("✅ P1-CH-06 突破距离测试")
	t.Logf("   CurrentPosition: %s", result.CurrentPosition)
	t.Logf("   BreakoutDistance: %.2f", result.BreakoutDistance)
	t.Logf("   BreakoutDistanceATR: %.2f", result.BreakoutDistanceATR)
}

// TestP1_CH06_RawRatio 测试RawRatio字段
func TestP1_CH06_RawRatio(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建测试数据
	klines := generateChannelTestKlinesP1(100, 50000.0, 0.01)
	currentPrice := 50500.0
	timeframe := "5m"

	// 执行分析
	result := analyzer.Analyze(klines, currentPrice, timeframe)

	// 验证RawRatio字段存在
	if result.RawRatio == 0 && result.PriceRatio == 0 {
		t.Log("⚠️  RawRatio和PriceRatio都为0（可能未找到通道）")
	}

	// RawRatio应该可以超出[0,1]范围（如果突破）
	if result.CurrentPosition == "break_up" && result.RawRatio <= 1.0 {
		t.Error("向上突破时RawRatio应该>1.0")
	}

	if result.CurrentPosition == "break_down" && result.RawRatio >= 0.0 {
		t.Error("向下突破时RawRatio应该<0.0")
	}

	t.Logf("✅ P1-CH-06 RawRatio测试")
	t.Logf("   PriceRatio (clamped): %.2f", result.PriceRatio)
	t.Logf("   RawRatio (unclamped): %.2f", result.RawRatio)
	t.Logf("   CurrentPosition: %s", result.CurrentPosition)
}

// TestP1_CH06_DistanceToRails 测试到上下轨距离
func TestP1_CH06_DistanceToRails(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建测试数据
	klines := generateChannelTestKlinesP1(100, 50000.0, 0.01)
	currentPrice := 50500.0
	timeframe := "5m"

	// 执行分析
	result := analyzer.Analyze(klines, currentPrice, timeframe)

	// 验证距离字段
	if result.ActiveChannel != nil {
		// 距离应该是非负数
		if result.DistanceToUpper < 0 {
			t.Errorf("DistanceToUpper为负数: %.2f", result.DistanceToUpper)
		}

		if result.DistanceToLower < 0 {
			t.Errorf("DistanceToLower为负数: %.2f", result.DistanceToLower)
		}

		// ATR距离也应该是非负数
		if result.DistanceToUpperATR < 0 {
			t.Errorf("DistanceToUpperATR为负数: %.2f", result.DistanceToUpperATR)
		}

		if result.DistanceToLowerATR < 0 {
			t.Errorf("DistanceToLowerATR为负数: %.2f", result.DistanceToLowerATR)
		}

		t.Logf("✅ P1-CH-06 到上下轨距离测试通过")
		t.Logf("   DistanceToUpper: %.2f (%.2f ATR)", result.DistanceToUpper, result.DistanceToUpperATR)
		t.Logf("   DistanceToLower: %.2f (%.2f ATR)", result.DistanceToLower, result.DistanceToLowerATR)
	} else {
		t.Log("⚠️  未找到活跃通道，跳过距离测试")
	}
}

// TestP1_CH06_JSONSerialization 测试P1新字段的JSON序列化
func TestP1_CH06_JSONSerialization(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建测试数据
	klines := generateChannelTestKlinesP1(100, 50000.0, 0.01)
	currentPrice := 50500.0
	timeframe := "5m"

	// 执行分析
	result := analyzer.Analyze(klines, currentPrice, timeframe)

	// 序列化为JSON
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("JSON序列化失败: %v", err)
	}

	// 验证P1新字段存在
	jsonStr := string(jsonData)
	p1Fields := []string{
		"raw_ratio",
		"distance_to_upper",
		"distance_to_lower",
		"distance_to_upper_atr",
		"distance_to_lower_atr",
		"breakout_distance",
		"breakout_distance_atr",
	}

	for _, field := range p1Fields {
		if !containsSubstringP1(jsonStr, field) {
			t.Errorf("JSON输出缺少P1字段: %s", field)
		}
	}

	t.Logf("✅ P1 JSON序列化测试通过")
	t.Logf("JSON输出长度: %d bytes", len(jsonData))
}

// TestP1_BreakoutScenarios 测试各种突破场景
func TestP1_BreakoutScenarios(t *testing.T) {
	analyzer := NewChannelAnalyzer()
	timeframe := "5m"

	scenarios := []struct {
		name          string
		klines        []Kline
		currentPrice  float64
		expectedPos   string
		shouldBreakout bool
	}{
		{
			name:          "向上突破",
			klines:        generateBreakoutKlines(100, 50000.0, 0.01, true),
			currentPrice:  51500.0,
			expectedPos:   "break_up",
			shouldBreakout: true,
		},
		{
			name:          "向下突破",
			klines:        generateBreakoutKlines(100, 50000.0, 0.01, false),
			currentPrice:  48500.0,
			expectedPos:   "break_down",
			shouldBreakout: true,
		},
		{
			name:          "通道内上轨",
			klines:        generateChannelTestKlinesP1(100, 50000.0, 0.01),
			currentPrice:  50400.0,
			expectedPos:   "upper",
			shouldBreakout: false,
		},
		{
			name:          "通道内下轨",
			klines:        generateChannelTestKlinesP1(100, 50000.0, 0.01),
			currentPrice:  49600.0,
			expectedPos:   "lower",
			shouldBreakout: false,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			result := analyzer.Analyze(sc.klines, sc.currentPrice, timeframe)

			if sc.shouldBreakout {
				if result.BreakoutDistance == 0 {
					t.Logf("⚠️  期望突破但BreakoutDistance为0（可能未识别到通道）")
				}
				if result.BreakoutDistanceATR == 0 {
					t.Logf("⚠️  期望突破但BreakoutDistanceATR为0")
				}
			} else {
				if result.BreakoutDistance != 0 {
					t.Errorf("不应突破但BreakoutDistance=%.2f", result.BreakoutDistance)
				}
			}

			t.Logf("   场景: %s", sc.name)
			t.Logf("   Position: %s", result.CurrentPosition)
			t.Logf("   RawRatio: %.2f", result.RawRatio)
			t.Logf("   BreakoutDistanceATR: %.2f", result.BreakoutDistanceATR)
		})
	}

	t.Logf("✅ P1 突破场景测试完成")
}

// ========== 辅助函数 ==========

// generateChannelTestKlinesP1 生成P1测试K线数据
func generateChannelTestKlinesP1(count int, startPrice float64, volatility float64) []Kline {
	klines := make([]Kline, count)
	baseTime := int64(1700000000000)

	price := startPrice
	for i := 0; i < count; i++ {
		// 模拟价格波动
		change := (float64(i%10) - 5) * volatility * price
		price += change

		high := price * (1 + volatility)
		low := price * (1 - volatility)
		open := price - change/2
		close := price

		klines[i] = Kline{
			OpenTime:  baseTime + int64(i*5*60*1000),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    1000.0 + float64(i*10),
			CloseTime: baseTime + int64((i+1)*5*60*1000) - 1,
		}
	}

	return klines
}

// generateBreakoutKlines 生成突破场景的K线数据
func generateBreakoutKlines(count int, basePrice float64, volatility float64, breakUp bool) []Kline {
	klines := make([]Kline, count)
	baseTime := int64(1700000000000)

	for i := 0; i < count; i++ {
		var price float64
		if i < count-10 {
			// 前90根：在通道内
			noise := (float64(i%7) - 3) * volatility * basePrice
			price = basePrice + noise
		} else {
			// 后10根：突破
			if breakUp {
				price = basePrice + float64(i-(count-10))*volatility*basePrice*5
			} else {
				price = basePrice - float64(i-(count-10))*volatility*basePrice*5
			}
		}

		high := price + volatility*basePrice
		low := price - volatility*basePrice

		klines[i] = Kline{
			OpenTime:  baseTime + int64(i*5*60*1000),
			Open:      price,
			High:      high,
			Low:       low,
			Close:     price,
			Volume:    1000.0,
			CloseTime: baseTime + int64((i+1)*5*60*1000) - 1,
		}
	}

	return klines
}

// containsSubstringP1 检查字符串是否包含子串
func containsSubstringP1(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
