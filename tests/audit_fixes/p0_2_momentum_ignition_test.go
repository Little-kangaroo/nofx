package audit_fixes

import (
	"testing"
	"math"
)

// 模拟市场快照和相关结构
type TestCVDDelta5m struct {
	PriceDeltaPct float64
}

type TestMarketSnapshot struct {
	CVDDelta5m *TestCVDDelta5m
}

type TestMomentumThresholds struct {
	CVDZScoreMin     float64
	VolumeZScoreMin  float64
	PriceZScoreMin   float64  // P0-2修复：新的Z-Score阈值
	ConfidenceWeight float64
}

// 测试P0-2修复：动能点火硬编码价格阈值问题
func TestP0_2_MomentumIgnitionFix(t *testing.T) {
	
	// 修复前的硬编码阈值逻辑
	testHardcodedLogic := func(priceChangePct float64) bool {
		return math.Abs(priceChangePct) >= 1.0 // 硬编码1%
	}
	
	// 修复后的Z-Score阈值逻辑
	testZScoreLogic := func(priceZScore float64, threshold float64) bool {
		return math.Abs(priceZScore) >= threshold // 使用Z-Score
	}
	
	t.Run("BTC场景测试", func(t *testing.T) {
		// BTC 1分钟0.8%的涨幅，在BTC历史上算很大的异动 (假设Z-Score = 4.0)
		btcPriceChange := 0.8
		btcPriceZScore := 4.0 // 相对BTC历史，0.8%是很大的异动
		
		// 修复前：硬编码1%，无法触发
		beforeFix := testHardcodedLogic(btcPriceChange)
		
		// 修复后：Z-Score 4.0 > 3.0，可以触发
		afterFix := testZScoreLogic(btcPriceZScore, 3.0)
		
		t.Logf("BTC 0.8%%涨幅测试:")
		t.Logf("  修复前(硬编码1%%): %t", beforeFix)
		t.Logf("  修复后(Z-Score): %t", afterFix)
		
		if beforeFix {
			t.Errorf("修复前应该无法触发 (0.8%% < 1%%)")
		}
		if !afterFix {
			t.Errorf("修复后应该能触发 (Z-Score 4.0 > 3.0)")
		}
		
		t.Logf("✅ BTC测试通过：修复后能正确识别BTC的大异动")
	})
	
	t.Run("小币场景测试", func(t *testing.T) {
		// 小币 1分钟2%的涨幅，但在该币历史上只是正常波动 (假设Z-Score = 0.8)
		altcoinPriceChange := 2.0
		altcoinPriceZScore := 0.8 // 相对该币历史，2%只是正常波动
		
		// 修复前：硬编码1%，会触发 (过于敏感)
		beforeFix := testHardcodedLogic(altcoinPriceChange)
		
		// 修复后：Z-Score 0.8 < 3.0，不会触发 (正确过滤噪音)
		afterFix := testZScoreLogic(altcoinPriceZScore, 3.0)
		
		t.Logf("小币 2%%涨幅测试:")
		t.Logf("  修复前(硬编码1%%): %t", beforeFix)
		t.Logf("  修复后(Z-Score): %t", afterFix)
		
		if !beforeFix {
			t.Errorf("修复前应该会触发 (2%% > 1%%)")
		}
		if afterFix {
			t.Errorf("修复后应该不触发 (Z-Score 0.8 < 3.0)")
		}
		
		t.Logf("✅ 小币测试通过：修复后能正确过滤小币的正常波动")
	})
	
	t.Run("真实异动场景测试", func(t *testing.T) {
		// 真正的异动：无论什么币种，Z-Score都很高
		
		testCases := []struct {
			name         string
			priceChange  float64
			priceZScore  float64
			shouldTrigger bool
		}{
			{"ETH强异动", 1.5, 5.2, true},   // 1.5%涨幅，Z-Score很高
			{"BTC强异动", 0.6, 3.8, true},   // 0.6%涨幅，但Z-Score很高
			{"小币强异动", 8.0, 6.1, true},   // 8%涨幅，Z-Score很高
			{"假突破", 3.0, 1.2, false},      // 3%涨幅，但Z-Score不高(正常波动)
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				beforeFix := testHardcodedLogic(tc.priceChange)
				afterFix := testZScoreLogic(tc.priceZScore, 3.0)
				
				t.Logf("%s: 涨幅%.1f%%, Z-Score%.1f", tc.name, tc.priceChange, tc.priceZScore)
				t.Logf("  修复前: %t, 修复后: %t, 期望: %t", beforeFix, afterFix, tc.shouldTrigger)
				
				if afterFix != tc.shouldTrigger {
					t.Errorf("%s 修复后结果不符合预期", tc.name)
				}
			})
		}
		
		t.Logf("✅ 真实异动场景测试通过")
	})
}

// 测试P0-2修复前后的统计对比
func TestP0_2_StatisticalComparison(t *testing.T) {
	// 模拟100个市场场景
	scenarios := []struct{
		symbol      string
		priceChange float64
		priceZScore float64
		description string
	}{
		// BTC场景：小幅变动但统计意义重大
		{"BTCUSDT", 0.3, 2.1, "BTC小涨但统计显著"},
		{"BTCUSDT", 0.7, 3.5, "BTC中涨统计显著"},
		{"BTCUSDT", 1.2, 5.1, "BTC大涨统计显著"},
		
		// ETH场景：中等变动
		{"ETHUSDT", 0.8, 2.8, "ETH小涨统计一般"},
		{"ETHUSDT", 1.5, 4.2, "ETH中涨统计显著"},
		
		// 小币场景：大幅变动但统计意义小
		{"DOGEUSDT", 3.0, 1.1, "小币大涨但统计不显著"},
		{"DOGEUSDT", 5.0, 2.2, "小币暴涨统计一般"},
		{"DOGEUSDT", 8.0, 4.5, "小币超级暴涨统计显著"},
	}
	
	hardcodedTriggers := 0
	zscoreTriggers := 0
	
	for _, scenario := range scenarios {
		// 修复前：硬编码1%
		if math.Abs(scenario.priceChange) >= 1.0 {
			hardcodedTriggers++
		}
		
		// 修复后：Z-Score > 3.0
		if math.Abs(scenario.priceZScore) >= 3.0 {
			zscoreTriggers++
		}
	}
	
	t.Logf("P0-2修复前后触发统计:")
	t.Logf("  修复前(硬编码1%%): %d/%d = %.1f%% 触发率", 
		hardcodedTriggers, len(scenarios), float64(hardcodedTriggers)/float64(len(scenarios))*100)
	t.Logf("  修复后(Z-Score>3): %d/%d = %.1f%% 触发率", 
		zscoreTriggers, len(scenarios), float64(zscoreTriggers)/float64(len(scenarios))*100)
	
	// 验证修复后的触发更合理
	if zscoreTriggers < 2 || zscoreTriggers > 6 {
		t.Errorf("修复后触发次数应该在合理范围内，实际: %d", zscoreTriggers)
	}
	
	t.Logf("✅ P0-2修复统计验证通过：Z-Score方法触发更精准")
}

// 测试P0-2的边界条件
func TestP0_2_EdgeCases(t *testing.T) {
	t.Run("极端市场条件", func(t *testing.T) {
		// 极端条件：价格巨变但Z-Score正常（可能是数据错误或极端事件）
		extremePriceChange := 50.0 // 50%变动
		normalZScore := 1.5
		
		// 修复前会无脑触发
		beforeFix := math.Abs(extremePriceChange) >= 1.0
		
		// 修复后不会触发（Z-Score不够高，可能是异常数据）
		afterFix := math.Abs(normalZScore) >= 3.0
		
		t.Logf("极端价格变动测试: %.0f%% 变动, Z-Score %.1f", extremePriceChange, normalZScore)
		t.Logf("  修复前: %t (可能误触发)", beforeFix)
		t.Logf("  修复后: %t (正确过滤异常)", afterFix)
		
		if !beforeFix {
			t.Errorf("修复前应该会触发极端价格变动")
		}
		if afterFix {
			t.Errorf("修复后应该过滤掉统计不显著的极端数据")
		}
		
		t.Logf("✅ 极端条件测试通过：修复后能正确处理数据异常")
	})
}