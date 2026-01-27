package market

import (
	"testing"
)

// TestSwingPointDetection 调试摆动点识别
func TestSwingPointDetection(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 使用现有的测试数据生成器
	klines := generateChannelTestKlines(200, 50000.0, 0.01)

	t.Logf("📊 摆动点识别调试:")
	t.Logf("   K线数量: %d", len(klines))
	t.Logf("   SwingLookback: %d", analyzer.config.SwingLookback)
	t.Logf("   MinSwingStrength: %.2f", analyzer.config.MinSwingStrength)

	// 识别摆动点
	swingPoints := analyzer.identifySwingPoints(klines)

	t.Logf("   识别到的摆动点数量: %d", len(swingPoints))

	if len(swingPoints) < 4 {
		t.Logf("❌ 摆动点不足4个，无法构建通道")

		// 详细分析：检查有多少个局部高低点
		highCount := 0
		lowCount := 0
		lookback := analyzer.config.SwingLookback

		for i := lookback; i < len(klines)-lookback; i++ {
			if analyzer.isLocalHigh(klines, i, lookback) {
				highCount++
				strength := analyzer.calculateSwingStrength(klines, i, true)
				if i < 10 {
					t.Logf("   局部高点[%d]: price=%.2f, strength=%.3f, pass=%v",
						i, klines[i].High, strength, strength >= analyzer.config.MinSwingStrength)
				}
			}
			if analyzer.isLocalLow(klines, i, lookback) {
				lowCount++
				strength := analyzer.calculateSwingStrength(klines, i, false)
				if i < 10 {
					t.Logf("   局部低点[%d]: price=%.2f, strength=%.3f, pass=%v",
						i, klines[i].Low, strength, strength >= analyzer.config.MinSwingStrength)
				}
			}
		}

		t.Logf("   局部高点总数: %d", highCount)
		t.Logf("   局部低点总数: %d", lowCount)
		t.Logf("   通过强度筛选的摆动点: %d", len(swingPoints))

	} else {
		t.Logf("✅ 摆动点充足: %d个", len(swingPoints))

		// 显示前几个摆动点
		maxShow := 5
		if len(swingPoints) < maxShow {
			maxShow = len(swingPoints)
		}
		for i := 0; i < maxShow; i++ {
			sp := swingPoints[i]
			t.Logf("   摆动点[%d]: type=%v, price=%.2f, strength=%.3f",
				i, sp.Type, sp.Price, sp.Strength)
		}
	}
}
