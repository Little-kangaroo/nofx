package direction

import (
	"testing"
)

// TestData17 测试 data17.txt 中的开仓数据
// 原始决策：LONG，未被拦截
// 关键特征：15m flat+breakup, 1h/30m flat+breakdown（矛盾信号）
func TestData17(t *testing.T) {
	input := RootSymbolInput{
		Symbol: "ETHUSDT",
		MTF: MTFAnalysis{
			"15m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "breakup",
					PriceRatio:      0.5,
					Quality:         0.70, // 从signal_confidence 99.4推断
				},
			},
			"30m": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "breakdown",
					PriceRatio:      0.5,
					Quality:         0.70, // 从signal_confidence 98.9推断
				},
			},
			"1h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "breakdown", // 关键：矛盾信号
					PriceRatio:      0.5,
					Quality:         0.70, // 从signal_confidence 99.6推断
				},
			},
			"4h": TimeframeData{
				Channel: ChannelInfo{
					Direction:       "flat",
					CurrentPosition: "breakup",
					PriceRatio:      0.5,
					Quality:         0.70, // 从signal_confidence 95.0推断
				},
			},
		},
		Orderflow: createTestOrderflowBullish(),
	}

	cfg := DefaultConfig()
	result := ComputeDirectionArbitration(input, cfg)

	t.Logf("=== Data17 测试结果 ===")
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)
	t.Logf("标记: %v", result.Flags)
	t.Logf("置信度: %.2f", result.Confidence)
	t.Logf("Delta: %.2f", result.Delta)

	// 分析：data17是盈利的开仓，应该被允许通过
	// 1h显示flat+breakdown（横盘突破），不是强矛盾信号
	// 新逻辑只拦截强矛盾（down通道+breakdown），不拦截flat通道
	if result.PlanSide == SideLong && !result.BlockEntry {
		t.Logf("✅ 正确判断：LONG且未拦截（盈利交易）")
		t.Logf("   置信度: %.2f, Delta: %.2f", result.Confidence, result.Delta)
	} else if result.PlanSide == SideLong && result.BlockEntry {
		t.Errorf("❌ 错误拦截：这是盈利交易，不应该被拦截")
		t.Errorf("   拦截原因: %s", result.BlockReason)
	} else {
		t.Logf("ℹ️ 判断方向: %s, 拦截: %v", result.PlanSide, result.BlockEntry)
	}
}
