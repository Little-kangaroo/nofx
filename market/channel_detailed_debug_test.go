package market

import (
	"testing"
)

// TestDetailedChannelDebug 详细调试通道构建过程
func TestDetailedChannelDebug(t *testing.T) {
	analyzer := NewChannelAnalyzer()

	// 使用现有的测试数据生成器
	klines := generateChannelTestKlines(200, 50000.0, 0.01)
	currentPrice := 50500.0

	t.Logf("📊 详细通道构建调试:")
	t.Logf("   K线数量: %d", len(klines))
	t.Logf("   当前价格: %.2f", currentPrice)

	// 步骤1: 识别摆动点
	swingPoints := analyzer.identifySwingPoints(klines)
	t.Logf("\n✅ 步骤1 - 摆动点识别:")
	t.Logf("   识别到 %d 个摆动点", len(swingPoints))

	if len(swingPoints) < 4 {
		t.Fatalf("❌ 摆动点不足4个，无法继续")
	}

	// 统计高点和低点
	highCount := 0
	lowCount := 0
	for _, sp := range swingPoints {
		if sp.Type == SwingHigh {
			highCount++
		} else {
			lowCount++
		}
	}
	t.Logf("   高点: %d, 低点: %d", highCount, lowCount)

	// 步骤2: 计算趋势线
	trendLines := analyzer.calculateTrendLines(swingPoints)
	t.Logf("\n✅ 步骤2 - 趋势线计算:")
	t.Logf("   生成 %d 条趋势线", len(trendLines))

	if len(trendLines) == 0 {
		t.Fatalf("❌ 没有生成趋势线")
	}

	// 统计趋势线类型
	resistanceCount := 0
	supportCount := 0
	for _, tl := range trendLines {
		if tl.Type == ResistanceLine {
			resistanceCount++
		} else {
			supportCount++
		}
	}
	t.Logf("   阻力线: %d, 支撑线: %d", resistanceCount, supportCount)

	// 显示前几条趋势线的详细信息
	maxShow := 5
	if len(trendLines) < maxShow {
		maxShow = len(trendLines)
	}
	for i := 0; i < maxShow; i++ {
		tl := trendLines[i]
		t.Logf("   趋势线[%d]: type=%v, touches=%d, strength=%.3f, slope=%.6f",
			i, tl.Type, tl.Touches, tl.Strength, tl.Slope)
	}

	// 步骤3: 检查趋势线对能否形成通道
	t.Logf("\n✅ 步骤3 - 检查趋势线配对:")
	validPairs := 0
	for i := 0; i < len(trendLines) && i < 5; i++ {
		for j := i + 1; j < len(trendLines) && j < 10; j++ {
			line1 := trendLines[i]
			line2 := trendLines[j]
			canForm := analyzer.canFormChannel(line1, line2)
			if canForm {
				validPairs++
				if validPairs <= 3 {
					t.Logf("   ✅ 趋势线[%d,%d]可以形成通道: type=(%v,%v), slope=(%.6f,%.6f)",
						i, j, line1.Type, line2.Type, line1.Slope, line2.Slope)
				}
			}
		}
	}
	t.Logf("   找到 %d 对可以形成通道的趋势线", validPairs)

	// 步骤4: 使用Analyze方法完整分析
	t.Logf("\n✅ 步骤4 - 完整分析:")
	result := analyzer.Analyze(klines, currentPrice, "5m")

	if result.ActiveChannel != nil {
		t.Logf("   ✅ 找到有效通道")
		t.Logf("   Direction: %s", result.Direction)
		t.Logf("   Quality: %.3f", result.Quality)
		t.Logf("   Width: %.2f%%", result.ActiveChannel.Width*100)
		t.Logf("   WidthATR: %.2f", result.WidthATR)
	} else {
		t.Logf("   ❌ 未找到有效通道")
		t.Logf("   Notes: %v", result.Notes)
		t.Logf("   Analysis: %s", result.Analysis)
	}
}
