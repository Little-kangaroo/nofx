package market

import (
	"encoding/json"
	"os"
	"testing"
)

// TestWithRealProductionData 使用真实生产数据测试
// 使用方法：
// 1. 从生产环境导出K线数据到 /tmp/test_klines.json
// 2. 运行: go test -v ./market -run TestWithRealProductionData
func TestWithRealProductionData(t *testing.T) {
	// 读取真实K线数据
	data, err := os.ReadFile("/tmp/test_klines.json")
	if err != nil {
		t.Skip("跳过测试：未找到真实数据文件 /tmp/test_klines.json")
		return
	}

	var klines []Kline
	if err := json.Unmarshal(data, &klines); err != nil {
		t.Fatalf("解析K线数据失败: %v", err)
	}

	if len(klines) < 50 {
		t.Fatalf("K线数据不足: %d根", len(klines))
	}

	// 使用最后一根K线的收盘价作为当前价格
	currentPrice := klines[len(klines)-1].Close

	t.Logf("📊 真实生产数据测试:")
	t.Logf("   K线数量: %d", len(klines))
	t.Logf("   当前价格: %.5f", currentPrice)
	t.Logf("   时间范围: %d - %d", klines[0].OpenTime, klines[len(klines)-1].OpenTime)

	// 创建分析器
	analyzer := NewChannelAnalyzer()

	// 测试多个时间框架
	timeframes := []string{"5m", "15m", "30m", "1h", "4h"}
	successCount := 0

	for _, tf := range timeframes {
		result := analyzer.Analyze(klines, currentPrice, tf)

		t.Logf("\n⏱️  时间框架: %s", tf)
		if result.ActiveChannel != nil {
			successCount++
			t.Logf("✅ 识别成功!")
			t.Logf("   Direction: %s", result.Direction)
			t.Logf("   Quality: %.2f", result.Quality)
			t.Logf("   Width: %.2f%%", result.ActiveChannel.Width*100)
			t.Logf("   Width ATR: %.2f", result.WidthATR)
			t.Logf("   ATR Grade: %s", result.ATRGrade)
			t.Logf("   Current Position: %s", result.CurrentPosition)
			t.Logf("   Price Ratio: %.2f%%", result.PriceRatio*100)
		} else {
			t.Logf("❌ 识别失败")
			t.Logf("   Notes: %v", result.Notes)
			t.Logf("   Analysis: %s", result.Analysis)
		}
	}

	t.Logf("\n📈 总体识别率: %.1f%% (%d/%d)",
		float64(successCount)/float64(len(timeframes))*100,
		successCount, len(timeframes))

	if successCount == 0 {
		t.Errorf("❌ 所有时间框架都识别失败，修复可能无效")
	} else {
		t.Logf("✅ 修复生效：%d个时间框架识别成功", successCount)
	}
}

// TestQuickValidation 快速验证修复（使用简单数据）
func TestQuickValidation(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	t.Logf("📋 当前配置:")
	t.Logf("   SwingLookback: %d", analyzer.config.SwingLookback)
	t.Logf("   MinSwingStrength: %.2f", analyzer.config.MinSwingStrength)
	t.Logf("   MinTrendLineHits: %d", analyzer.config.MinTrendLineHits)
	t.Logf("   QualityThreshold: %.2f", analyzer.config.QualityThreshold)
	t.Logf("   MinChannelWidthATR: %.2f", analyzer.config.MinChannelWidthATR)
	t.Logf("   MaxChannelWidthATR: %.2f", analyzer.config.MaxChannelWidthATR)

	// 使用现有的测试数据生成器
	klines := generateChannelTestKlines(200, 50000.0, 0.01)
	currentPrice := 50500.0

	result := analyzer.Analyze(klines, currentPrice, "5m")

	t.Logf("\n📊 快速验证结果:")
	if result.ActiveChannel != nil {
		t.Logf("✅ 通道识别成功")
		t.Logf("   Direction: %s", result.Direction)
		t.Logf("   Quality: %.2f", result.Quality)
	} else {
		t.Logf("❌ 通道识别失败")
		t.Logf("   Notes: %v", result.Notes)
	}
}
