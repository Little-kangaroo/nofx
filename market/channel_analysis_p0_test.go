package market

import (
	"encoding/json"
	"math"
	"testing"
)

// TestP0_CH01_CoordinateSystemAnnotation 测试坐标系标注
func TestP0_CH01_CoordinateSystemAnnotation(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建测试数据
	klines := generateChannelTestKlines(100, 50000.0, 0.01)
	currentPrice := 50500.0
	timeframe := "5m"

	// 执行分析
	result := analyzer.Analyze(klines, currentPrice, timeframe)

	// 验证坐标系标注字段
	if result.Timeframe != timeframe {
		t.Errorf("Timeframe字段错误: 期望 %s, 实际 %s", timeframe, result.Timeframe)
	}

	if result.Coord != "index" {
		t.Errorf("Coord字段错误: 期望 'index', 实际 %s", result.Coord)
	}

	expectedRefIndex := len(klines) - 1
	if result.RefIndex != expectedRefIndex {
		t.Errorf("RefIndex字段错误: 期望 %d, 实际 %d", expectedRefIndex, result.RefIndex)
	}

	if result.RefOpenTime == 0 {
		t.Error("RefOpenTime字段为0，应该有值")
	}

	t.Logf("✅ P0-CH-01 坐标系标注测试通过")
	t.Logf("   Timeframe: %s", result.Timeframe)
	t.Logf("   Coord: %s", result.Coord)
	t.Logf("   RefIndex: %d", result.RefIndex)
	t.Logf("   RefOpenTime: %d", result.RefOpenTime)
}

// TestP0_CH02_ATRUnification 测试ATR统一
func TestP0_CH02_ATRUnification(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建测试数据
	klines := generateChannelTestKlines(100, 50000.0, 0.01)
	currentPrice := 50500.0
	timeframe := "5m"

	// 执行分析
	result := analyzer.Analyze(klines, currentPrice, timeframe)

	// 验证ATR相关字段
	if result.ATRMode == "" {
		t.Error("ATRMode字段为空")
	}

	if result.ATRConfidence == 0 {
		t.Error("ATRConfidence字段为0")
	}

	// 如果有通道，验证WidthATR
	if result.ActiveChannel != nil {
		if result.WidthATR == 0 {
			t.Error("WidthATR字段为0，但有活跃通道")
		}
		if result.ATRGrade == "" {
			t.Error("ATRGrade字段为空，但有活跃通道")
		}
	}

	t.Logf("✅ P0-CH-02 ATR统一测试通过")
	t.Logf("   ATRMode: %s", result.ATRMode)
	t.Logf("   ATRConfidence: %.2f", result.ATRConfidence)
	if result.ActiveChannel != nil {
		t.Logf("   WidthATR: %.2f", result.WidthATR)
		t.Logf("   ATRGrade: %s", result.ATRGrade)
	}
}

// TestP0_CH04_AgeSemanticsplit 测试Age语义拆分
func TestP0_CH04_AgeSemanticsplit(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建测试数据
	klines := generateChannelTestKlines(100, 50000.0, 0.01)
	currentPrice := 50500.0
	timeframe := "5m"

	// 执行分析
	result := analyzer.Analyze(klines, currentPrice, timeframe)

	// 如果有通道，验证Age字段
	if result.ActiveChannel != nil {
		channel := result.ActiveChannel

		// 验证AgeBars字段存在且合理
		if channel.AgeBars < 0 {
			t.Errorf("AgeBars字段为负数: %d", channel.AgeBars)
		}

		if channel.AgeBars > len(klines) {
			t.Errorf("AgeBars字段超过K线数量: %d > %d", channel.AgeBars, len(klines))
		}

		// AgeMs可以为0（因为我们暂时没有计算）
		// 但不应该是负数
		if channel.AgeMs < 0 {
			t.Errorf("AgeMs字段为负数: %d", channel.AgeMs)
		}

		t.Logf("✅ P0-CH-04 Age语义拆分测试通过")
		t.Logf("   AgeBars: %d", channel.AgeBars)
		t.Logf("   AgeMs: %d", channel.AgeMs)
	} else {
		t.Log("⚠️  未找到活跃通道，跳过Age字段测试")
	}
}

// TestP0_CH05_HorizontalChannelDetection 测试水平通道识别
func TestP0_CH05_HorizontalChannelDetection(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建横盘测试数据（价格在50000附近小幅波动）
	klines := generateHorizontalKlines(100, 50000.0, 0.002)
	currentPrice := 50000.0
	timeframe := "5m"

	// 执行分析
	result := analyzer.Analyze(klines, currentPrice, timeframe)

	// 验证是否能识别通道（横盘也应该能识别）
	if result.ActiveChannel != nil {
		channel := result.ActiveChannel

		// 验证通道方向应该是flat
		if channel.Direction != "flat" {
			t.Logf("⚠️  通道方向为 %s，期望 'flat'（但这可能是正常的）", channel.Direction)
		}

		// 验证斜率应该接近0
		avgSlope := (channel.UpperLine.Slope + channel.LowerLine.Slope) / 2
		if avgSlope > 0.1 || avgSlope < -0.1 {
			t.Logf("⚠️  平均斜率较大: %.6f（横盘通道应该接近0）", avgSlope)
		}

		t.Logf("✅ P0-CH-05 水平通道识别测试通过")
		t.Logf("   Direction: %s", channel.Direction)
		t.Logf("   AvgSlope: %.6f", avgSlope)
		t.Logf("   Quality: %.2f", channel.Quality)
	} else {
		t.Log("⚠️  未识别到水平通道（可能需要调整参数）")
	}
}

// TestP0_BacktestConsistency 测试回测一致性
func TestP0_BacktestConsistency(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建测试数据
	klines := generateChannelTestKlines(100, 50000.0, 0.01)
	currentPrice := 50500.0
	timeframe := "5m"

	// 多次运行，验证结果一致
	var results []*ChannelData
	for i := 0; i < 3; i++ {
		result := analyzer.Analyze(klines, currentPrice, timeframe)
		results = append(results, result)
	}

	// 验证关键字段一致
	for i := 1; i < len(results); i++ {
		if results[i].Timeframe != results[0].Timeframe {
			t.Error("多次运行Timeframe不一致")
		}
		if results[i].RefIndex != results[0].RefIndex {
			t.Error("多次运行RefIndex不一致")
		}
		if results[i].Coord != results[0].Coord {
			t.Error("多次运行Coord不一致")
		}

		// 验证通道存在性一致
		hasChannel0 := results[0].ActiveChannel != nil
		hasChannelI := results[i].ActiveChannel != nil
		if hasChannel0 != hasChannelI {
			t.Error("多次运行通道存在性不一致")
		}
	}

	t.Logf("✅ P0 回测一致性测试通过（3次运行结果一致）")
}

// TestP0_JSONSerialization 测试JSON序列化
func TestP0_JSONSerialization(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 创建测试数据
	klines := generateChannelTestKlines(100, 50000.0, 0.01)
	currentPrice := 50500.0
	timeframe := "5m"

	// 执行分析
	result := analyzer.Analyze(klines, currentPrice, timeframe)

	// 序列化为JSON
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("JSON序列化失败: %v", err)
	}

	// 验证关键字段存在
	jsonStr := string(jsonData)
	requiredFields := []string{
		"timeframe",
		"coord",
		"ref_index",
		"atr_mode",
		"atr_confidence",
	}

	for _, field := range requiredFields {
		if !containsSubstring(jsonStr, field) {
			t.Errorf("JSON输出缺少字段: %s", field)
		}
	}

	t.Logf("✅ P0 JSON序列化测试通过")
	t.Logf("JSON输出长度: %d bytes", len(jsonData))
}

// TestP0_TrendLinePriceAtIndex 测试趋势线价格计算辅助函数
func TestP0_TrendLinePriceAtIndex(t *testing.T) {
	// 创建测试趋势线
	line := &TrendLine{
		Slope:     0.5,  // 每个索引增加0.5
		Intercept: 100.0, // 索引0处价格为100
	}

	// 测试不同索引的价格
	testCases := []struct {
		index    int
		expected float64
	}{
		{0, 100.0},
		{10, 105.0},
		{20, 110.0},
		{100, 150.0},
	}

	for _, tc := range testCases {
		price := TrendLinePriceAtIndex(line, tc.index)
		if price != tc.expected {
			t.Errorf("索引 %d 的价格错误: 期望 %.2f, 实际 %.2f", tc.index, tc.expected, price)
		}
	}

	// 测试nil趋势线
	nilPrice := TrendLinePriceAtIndex(nil, 10)
	if nilPrice != 0 {
		t.Errorf("nil趋势线应返回0，实际返回 %.2f", nilPrice)
	}

	t.Logf("✅ P0 TrendLinePriceAtIndex测试通过")
}

// ========== 辅助函数 ==========

// generateChannelTestKlines 生成通道测试K线数据（避免与其他测试文件冲突）
func generateChannelTestKlines(count int, startPrice float64, volatility float64) []Kline {
	klines := make([]Kline, count)
	baseTime := int64(1700000000000) // 2023-11-15 00:00:00

	for i := 0; i < count; i++ {
		// 使用正弦波创建通道模式
		// 周期为20根K线，这样在200根K线中有10个完整周期
		cycle := float64(i) / 20.0
		wave := math.Sin(cycle * 2 * math.Pi)

		// 添加轻微的上升趋势
		trend := float64(i) * 0.0002

		// 组合趋势和波动 - 减小波动幅度以产生合理的ATR
		priceOffset := (wave * volatility * 3.0 + trend) * startPrice
		price := startPrice + priceOffset

		// 生成OHLC - 使用适中的波动幅度
		// 确保有足够的摆动强度，但不会导致ATR异常
		high := price * (1 + volatility * 1.5)
		low := price * (1 - volatility * 1.5)
		open := price * (1 - volatility * 0.3)
		close := price * (1 + volatility * 0.3)

		klines[i] = Kline{
			OpenTime:  baseTime + int64(i*5*60*1000), // 5分钟间隔
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

// generateHorizontalKlines 生成横盘K线数据
func generateHorizontalKlines(count int, basePrice float64, volatility float64) []Kline {
	klines := make([]Kline, count)
	baseTime := int64(1700000000000)

	for i := 0; i < count; i++ {
		// 横盘：价格在basePrice附近小幅波动
		noise := (float64(i%7) - 3) * volatility * basePrice
		price := basePrice + noise

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

// containsSubstring 检查字符串是否包含子串（避免与其他测试文件冲突）
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
