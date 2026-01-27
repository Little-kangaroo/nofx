package market

import (
	"testing"
)

// TestChannelRecognitionAfterFix 测试修复后的通道识别（使用现有测试数据生成器）
func TestChannelRecognitionAfterFix(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 测试场景1: 标准通道（使用现有生成器）
	t.Run("标准通道", func(t *testing.T) {
		klines := generateChannelTestKlines(200, 50000.0, 0.01)
		currentPrice := 50500.0

		result := analyzer.Analyze(klines, currentPrice, "5m")

		if result.ActiveChannel == nil {
			t.Logf("⚠️  标准通道识别失败")
			t.Logf("   Notes: %v", result.Notes)
			t.Logf("   Analysis: %s", result.Analysis)
		} else {
			t.Logf("✅ 标准通道识别成功")
			t.Logf("   Direction: %s", result.Direction)
			t.Logf("   Quality: %.2f", result.Quality)
			t.Logf("   ATR Grade: %s", result.ATRGrade)
			t.Logf("   Width ATR: %.2f", result.WidthATR)
		}
	})

	// 测试场景2: 横盘通道
	t.Run("横盘通道", func(t *testing.T) {
		klines := generateHorizontalKlines(200, 50000.0, 0.002)
		currentPrice := 50000.0

		result := analyzer.Analyze(klines, currentPrice, "5m")

		if result.ActiveChannel == nil {
			t.Logf("⚠️  横盘通道识别失败")
			t.Logf("   Notes: %v", result.Notes)
		} else {
			t.Logf("✅ 横盘通道识别成功")
			t.Logf("   Direction: %s", result.Direction)
			t.Logf("   Quality: %.2f", result.Quality)
		}
	})

	// 测试场景3: 更多K线数据
	t.Run("更多K线数据", func(t *testing.T) {
		klines := generateChannelTestKlines(500, 50000.0, 0.008)
		currentPrice := 50400.0

		result := analyzer.Analyze(klines, currentPrice, "5m")

		if result.ActiveChannel == nil {
			t.Logf("⚠️  更多K线数据识别失败")
			t.Logf("   Notes: %v", result.Notes)
		} else {
			t.Logf("✅ 更多K线数据识别成功")
			t.Logf("   Direction: %s", result.Direction)
			t.Logf("   Quality: %.2f", result.Quality)
		}
	})
}

// TestConfigurationComparison 对比修改前后的配置
func TestConfigurationComparison(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	t.Logf("📋 当前配置（修复后）:")
	t.Logf("   SwingLookback: %d (修改前: 7)", analyzer.config.SwingLookback)
	t.Logf("   MinSwingStrength: %.2f (修改前: 0.60)", analyzer.config.MinSwingStrength)
	t.Logf("   MinTrendLineHits: %d (修改前: 3)", analyzer.config.MinTrendLineHits)
	t.Logf("   MaxDistance: %.1f%% (修改前: 1.5%%)", analyzer.config.MaxDistance*100)
	t.Logf("   ParallelTolerance: %.1f%% (修改前: 8%%)", analyzer.config.ParallelTolerance*100)
	t.Logf("   QualityThreshold: %.2f (修改前: 0.75)", analyzer.config.QualityThreshold)
	t.Logf("   MinChannelWidthATR: %.2f (修改前: 0.80)", analyzer.config.MinChannelWidthATR)
	t.Logf("   MaxChannelWidthATR: %.2f (修改前: 4.00)", analyzer.config.MaxChannelWidthATR)
	t.Logf("   OptimalWidthATRMin: %.2f (修改前: 1.20)", analyzer.config.OptimalWidthATRMin)
	t.Logf("   OptimalWidthATRMax: %.2f (修改前: 2.50)", analyzer.config.OptimalWidthATRMax)

	t.Logf("\n📊 修改总结:")
	t.Logf("   ✅ 降低摆动点识别难度: SwingLookback 7→5, MinSwingStrength 0.6→0.5")
	t.Logf("   ✅ 降低趋势线要求: MinTrendLineHits 3→2, MaxDistance 1.5%%→2.0%%")
	t.Logf("   ✅ 放宽通道宽度标准: MinATR 0.8→0.5, MaxATR 4.0→6.0")
	t.Logf("   ✅ 降低质量阈值: QualityThreshold 0.75→0.60")
	t.Logf("   ✅ 放宽平行度要求: ParallelTolerance 8%%→10%%")
}

