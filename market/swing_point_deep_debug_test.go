package market

import (
	"testing"
)

// TestSwingPointDetectionDeep 深度调试摆动点识别
func TestSwingPointDetectionDeep(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 生成测试数据
	klines := generateProductionLikeKlines(1000, 0.12264, 0.005)

	t.Logf("📊 深度调试摆动点识别:")
	t.Logf("   K线数量: %d", len(klines))
	t.Logf("   SwingLookback: %d", analyzer.config.SwingLookback)

	// 统计局部高低点
	localHighCount := 0
	localLowCount := 0
	highWithStrength := 0
	lowWithStrength := 0

	lookback := analyzer.config.SwingLookback

	for i := lookback; i < len(klines)-lookback; i++ {
		if analyzer.isLocalHigh(klines, i, lookback) {
			localHighCount++
			strength := analyzer.calculateSwingStrength(klines, i, true)
			if strength > 0 {
				highWithStrength++
				if localHighCount <= 5 {
					t.Logf("   高点[%d] Index=%d, Price=%.5f, Strength=%.3f",
						localHighCount, i, klines[i].High, strength)
				}
			}
		}
		if analyzer.isLocalLow(klines, i, lookback) {
			localLowCount++
			strength := analyzer.calculateSwingStrength(klines, i, false)
			if strength > 0 {
				lowWithStrength++
				if localLowCount <= 5 {
					t.Logf("   低点[%d] Index=%d, Price=%.5f, Strength=%.3f",
						localLowCount, i, klines[i].Low, strength)
				}
			}
		}
	}

	t.Logf("\n   统计结果:")
	t.Logf("     局部高点总数: %d", localHighCount)
	t.Logf("     局部低点总数: %d", localLowCount)
	t.Logf("     高点有强度(>0): %d", highWithStrength)
	t.Logf("     低点有强度(>0): %d", lowWithStrength)

	// 检查前几根K线的数据
	t.Logf("\n   前10根K线数据:")
	for i := 0; i < 10 && i < len(klines); i++ {
		k := klines[i]
		t.Logf("     [%d] O=%.5f H=%.5f L=%.5f C=%.5f V=%.0f",
			i, k.Open, k.High, k.Low, k.Close, k.Volume)
	}

	if localHighCount == 0 && localLowCount == 0 {
		t.Errorf("❌ 没有识别出任何局部高低点，isLocalHigh/isLocalLow可能有问题")
	} else if highWithStrength == 0 && lowWithStrength == 0 {
		t.Errorf("❌ 识别出局部高低点，但强度全是0，calculateSwingStrength有问题")
	}
}
