package market

import (
	"testing"
)

// TestSwingPointDetectionDebug 测试摆动点识别（调试版）
func TestSwingPointDetectionDebug(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 生成测试数据
	klines := generateProductionLikeKlines(1000, 0.12264, 0.005)

	t.Logf("📊 测试摆动点识别:")
	t.Logf("   K线数量: %d", len(klines))
	t.Logf("   配置:")
	t.Logf("     SwingLookback: %d", analyzer.config.SwingLookback)
	t.Logf("     MinSwingStrength: %.2f", analyzer.config.MinSwingStrength)

	// 调用内部方法识别摆动点
	swingPoints := analyzer.identifySwingPoints(klines)

	t.Logf("   识别结果:")
	t.Logf("     摆动点总数: %d", len(swingPoints))

	if len(swingPoints) > 0 {
		// 显示前10个摆动点
		displayCount := 10
		if len(swingPoints) < displayCount {
			displayCount = len(swingPoints)
		}

		t.Logf("   前%d个摆动点:", displayCount)
		for i := 0; i < displayCount; i++ {
			sp := swingPoints[i]
			t.Logf("     [%d] Index=%d, Type=%s, Price=%.5f, Strength=%.3f",
				i, sp.Index, sp.Type, sp.Price, sp.Strength)
		}
	}

	if len(swingPoints) < 4 {
		t.Errorf("❌ 摆动点不足4个，无法构建通道")

		// 尝试降低强度阈值看看能识别多少
		t.Logf("\n🔍 尝试降低强度阈值:")
		for threshold := 0.4; threshold >= 0.1; threshold -= 0.1 {
			count := 0
			for i := analyzer.config.SwingLookback; i < len(klines)-analyzer.config.SwingLookback; i++ {
				if analyzer.isLocalHigh(klines, i, analyzer.config.SwingLookback) {
					strength := analyzer.calculateSwingStrength(klines, i, true)
					if strength >= threshold {
						count++
					}
				}
				if analyzer.isLocalLow(klines, i, analyzer.config.SwingLookback) {
					strength := analyzer.calculateSwingStrength(klines, i, false)
					if strength >= threshold {
						count++
					}
				}
			}
			t.Logf("   阈值 %.1f: 识别 %d 个摆动点", threshold, count)
		}
	} else {
		t.Logf("✅ 摆动点充足，可以构建通道")
	}
}
