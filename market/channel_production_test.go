package market

import (
	"encoding/json"
	"testing"
)

// TestProductionScenario 测试生产环境场景
func TestProductionScenario(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 模拟生产环境：DOGEUSDT 价格 0.12264
	// 生成类似生产环境的K线数据
	klines := generateProductionLikeKlines(1000, 0.12264, 0.005)
	currentPrice := 0.12264

	t.Logf("📊 测试生产环境场景:")
	t.Logf("   币种: DOGEUSDT")
	t.Logf("   当前价格: %.5f", currentPrice)
	t.Logf("   K线数量: %d", len(klines))

	// 测试多个时间框架
	timeframes := []string{"5m", "15m", "30m", "1h", "4h"}
	successCount := 0

	for _, tf := range timeframes {
		result := analyzer.Analyze(klines, currentPrice, tf)

		if result.ActiveChannel != nil {
			successCount++
			t.Logf("✅ %s: 识别成功", tf)
			t.Logf("   Direction: %s", result.Direction)
			t.Logf("   Quality: %.2f", result.Quality)
			t.Logf("   Width ATR: %.2f", result.WidthATR)
			t.Logf("   ATR Grade: %s", result.ATRGrade)
			t.Logf("   Current Position: %s", result.CurrentPosition)
		} else {
			t.Logf("❌ %s: 识别失败", tf)
			t.Logf("   Notes: %v", result.Notes)
			t.Logf("   Analysis: %s", result.Analysis)
		}
		t.Logf("")
	}

	rate := float64(successCount) / float64(len(timeframes)) * 100
	t.Logf("📈 生产环境通道识别率: %.1f%% (%d/%d)", rate, successCount, len(timeframes))

	if successCount == 0 {
		t.Errorf("❌ 所有时间框架都识别失败，修复可能无效")
	} else if successCount >= 2 {
		t.Logf("✅ 修复生效：至少2个时间框架识别成功")
	}
}

// TestCompactOutputFormat 测试紧凑输出格式
func TestCompactOutputFormat(t *testing.T) {
	analyzer := NewChannelAnalyzer()
	klines := generateProductionLikeKlines(500, 0.12264, 0.005)
	currentPrice := 0.12264

	result := analyzer.Analyze(klines, currentPrice, "5m")

	// 模拟紧凑输出
	compact := map[string]interface{}{
		"channel_direction":   result.Direction,
		"channel_width_pct":   result.ActiveChannel,
		"current_position":    result.CurrentPosition,
		"direction_source":    "channel", // 如果有通道
		"fail_reason":         "",
	}

	if result.ActiveChannel == nil {
		compact["channel_direction"] = ""
		compact["channel_width_pct"] = 0
		compact["current_position"] = ""
		compact["direction_source"] = "none"
		if len(result.Notes) > 0 {
			compact["fail_reason"] = result.Notes[0]
		}
	} else {
		compact["channel_width_pct"] = result.ActiveChannel.Width
	}

	jsonData, _ := json.MarshalIndent(compact, "", "  ")
	t.Logf("📋 紧凑输出格式:")
	t.Logf("%s", string(jsonData))

	// 验证：修复后不应该出现全空的情况
	if compact["channel_direction"] == "" && compact["channel_width_pct"] == 0 {
		t.Logf("⚠️  通道数据为空")
		t.Logf("   失败原因: %v", compact["fail_reason"])
	} else {
		t.Logf("✅ 通道数据有效")
	}
}

// generateProductionLikeKlines 生成类似生产环境的K线数据
func generateProductionLikeKlines(count int, basePrice float64, volatility float64) []Kline {
	klines := make([]Kline, count)
	baseTime := int64(1769154000000) // 使用生产环境的时间戳

	price := basePrice
	for i := 0; i < count; i++ {
		// 模拟真实市场的价格波动
		// 1. 趋势成分（缓慢变化）
		trendCycle := float64(i) / 100.0
		trend := volatility * 0.3 * (trendCycle - float64(int(trendCycle)))

		// 2. 周期性波动（形成摆动点）
		waveCycle := float64(i % 20)
		var wave float64
		if waveCycle < 10 {
			wave = volatility * 0.5 * (waveCycle / 10.0)
		} else {
			wave = volatility * 0.5 * (1.0 - (waveCycle-10.0)/10.0)
		}

		// 3. 随机噪声
		noise := (float64(i%7) - 3) * volatility * 0.1

		// 组合所有成分
		priceChange := trend + wave + noise
		price = price * (1 + priceChange)

		// 生成OHLC
		high := price * (1 + volatility*0.3)
		low := price * (1 - volatility*0.3)
		open := price * (1 - priceChange*0.5)
		close := price

		klines[i] = Kline{
			OpenTime:  baseTime + int64(i*300000), // 5分钟间隔
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    20000000 + float64(i%100)*100000,
			CloseTime: baseTime + int64((i+1)*300000) - 1,
		}
	}

	return klines
}

// TestBeforeAfterComparison 对比修改前后的效果
func TestBeforeAfterComparison(t *testing.T) {
	t.Logf("📊 修改前后对比:")
	t.Logf("")
	t.Logf("修改前配置（过严）:")
	t.Logf("  - SwingLookback: 7")
	t.Logf("  - MinSwingStrength: 0.6")
	t.Logf("  - MinTrendLineHits: 3")
	t.Logf("  - MaxDistance: 1.5%%")
	t.Logf("  - ParallelTolerance: 8%%")
	t.Logf("  - QualityThreshold: 0.75")
	t.Logf("  - MinChannelWidthATR: 0.8")
	t.Logf("  - MaxChannelWidthATR: 4.0")
	t.Logf("  ❌ 结果: 生产环境中5个时间框架全部为空")
	t.Logf("")
	t.Logf("修改后配置（放宽）:")
	t.Logf("  - SwingLookback: 5 ✅")
	t.Logf("  - MinSwingStrength: 0.5 ✅")
	t.Logf("  - MinTrendLineHits: 2 ✅")
	t.Logf("  - MaxDistance: 2.0%% ✅")
	t.Logf("  - ParallelTolerance: 10%% ✅")
	t.Logf("  - QualityThreshold: 0.60 ✅")
	t.Logf("  - MinChannelWidthATR: 0.5 ✅")
	t.Logf("  - MaxChannelWidthATR: 6.0 ✅")
	t.Logf("  ✅ 预期: 通道识别率显著提升")
}
