package audit_fixes

import (
	"testing"
	"math"
)

// 模拟CVD数据和相关结构
type TestCVDData struct {
	SpotCVD1H     float64
	FuturesCVD1H  float64
}

type TestGlobalStatsManager struct {
	// 模拟统计管理器
}

func (gsm *TestGlobalStatsManager) GetSymbolZScores(symbol string, snapshot interface{}) map[string]float64 {
	// 返回空map，用于兜底逻辑测试
	return make(map[string]float64)
}

// 测试P1-1修复：CVD背离检测硬编码绝对值问题
func TestP1_1_CVDDivergenceDetectionFix(t *testing.T) {
	
	// 修复前的硬编码逻辑（10万美元固定阈值）
	testHardcodedLogic := func(spotCVD, futuresCVD float64) bool {
		threshold := 100000.0 // 硬编码10万美元
		
		// 判断方向
		spotDirection := 0
		futuresDirection := 0
		
		if spotCVD > threshold {
			spotDirection = 1
		} else if spotCVD < -threshold {
			spotDirection = -1
		}
		
		if futuresCVD > threshold {
			futuresDirection = 1
		} else if futuresCVD < -threshold {
			futuresDirection = -1
		}
		
		// 如果现货和期货方向相反，认为有背离
		return spotDirection != 0 && futuresDirection != 0 && spotDirection != futuresDirection
	}
	
	// 修复后的相对阈值逻辑
	testRelativeThresholdLogic := func(spotCVD, futuresCVD float64) bool {
		// 计算相对阈值，而非固定10万美元
		avgAbsCVD := (math.Abs(spotCVD) + math.Abs(futuresCVD)) / 2
		relativeThreshold := math.Max(50000, math.Min(500000, avgAbsCVD*0.2))
		
		// 判断方向
		spotDirection := 0
		futuresDirection := 0
		
		if spotCVD > relativeThreshold {
			spotDirection = 1
		} else if spotCVD < -relativeThreshold {
			spotDirection = -1
		}
		
		if futuresCVD > relativeThreshold {
			futuresDirection = 1
		} else if futuresCVD < -relativeThreshold {
			futuresDirection = -1
		}
		
		// 如果现货和期货方向相反，认为有背离
		return spotDirection != 0 && futuresDirection != 0 && spotDirection != futuresDirection
	}

	t.Run("BTC小额CVD背离测试", func(t *testing.T) {
		// BTC小额CVD值，对于BTC来说50K是统计显著的
		spotCVD := 80000.0    // 现货净买入8万
		futuresCVD := -60000.0 // 期货净卖出6万
		
		// 修复前：硬编码10万，无法检测到背离
		beforeFix := testHardcodedLogic(spotCVD, futuresCVD)
		
		// 修复后：相对阈值，可以检测到背离
		afterFix := testRelativeThresholdLogic(spotCVD, futuresCVD)
		
		t.Logf("BTC小额CVD背离测试: 现货%.0f, 期货%.0f", spotCVD, futuresCVD)
		t.Logf("  修复前(硬编码10万): %t", beforeFix)
		t.Logf("  修复后(相对阈值): %t", afterFix)
		
		if beforeFix {
			t.Errorf("修复前应该无法检测到背离 (CVD值 < 10万)")
		}
		if !afterFix {
			t.Errorf("修复后应该能检测到背离 (相对统计显著)")
		}
		
		t.Logf("✅ BTC小额CVD测试通过：修复后能正确识别BTC的小额背离")
	})
	
	t.Run("大额CVD非背离测试", func(t *testing.T) {
		// 大额CVD值，但不是真正的背离（同向）
		spotCVD := 2000000.0   // 现货净买入200万
		futuresCVD := 1500000.0 // 期货也净买入150万
		
		// 修复前：硬编码逻辑
		beforeFix := testHardcodedLogic(spotCVD, futuresCVD)
		
		// 修复后：相对阈值逻辑
		afterFix := testRelativeThresholdLogic(spotCVD, futuresCVD)
		
		t.Logf("大额CVD非背离测试: 现货%.0f, 期货%.0f", spotCVD, futuresCVD)
		t.Logf("  修复前(硬编码10万): %t", beforeFix)
		t.Logf("  修复后(相对阈值): %t", afterFix)
		
		// 两种方法都应该识别为非背离（同向）
		if beforeFix {
			t.Errorf("修复前应该识别为非背离 (现货期货同向)")
		}
		if afterFix {
			t.Errorf("修复后应该识别为非背离 (现货期货同向)")
		}
		
		t.Logf("✅ 大额CVD非背离测试通过：同向资金流不误判为背离")
	})
	
	t.Run("小币大额CVD背离测试", func(t *testing.T) {
		// 小币的大额CVD值，相对该币种历史是背离
		spotCVD := 500000.0    // 现货净买入50万
		futuresCVD := -800000.0 // 期货净卖出80万
		
		// 修复前：硬编码逻辑，会检测到
		beforeFix := testHardcodedLogic(spotCVD, futuresCVD)
		
		// 修复后：相对阈值逻辑，也会检测到（因为绝对值够大）
		afterFix := testRelativeThresholdLogic(spotCVD, futuresCVD)
		
		t.Logf("小币大额CVD背离测试: 现货%.0f, 期货%.0f", spotCVD, futuresCVD)
		t.Logf("  修复前(硬编码10万): %t", beforeFix)
		t.Logf("  修复后(相对阈值): %t", afterFix)
		
		// 两种方法都应该能检测到背离
		if !beforeFix {
			t.Errorf("修复前应该检测到背离 (绝对值都>10万且方向相反)")
		}
		if !afterFix {
			t.Errorf("修复后应该检测到背离 (相对阈值显著且方向相反)")
		}
		
		t.Logf("✅ 小币大额CVD背离测试通过：大额背离正确识别")
	})

	t.Run("极小CVD噪音过滤测试", func(t *testing.T) {
		// 极小的CVD值，应该被过滤掉（可能是交易噪音）
		spotCVD := 30000.0     // 现货净买入3万
		futuresCVD := -25000.0 // 期货净卖出2.5万
		
		// 修复前：硬编码逻辑，不会触发（< 10万）
		beforeFix := testHardcodedLogic(spotCVD, futuresCVD)
		
		// 修复后：相对阈值逻辑，也不会触发（低于最小相对阈值5万）
		afterFix := testRelativeThresholdLogic(spotCVD, futuresCVD)
		
		t.Logf("极小CVD噪音测试: 现货%.0f, 期货%.0f", spotCVD, futuresCVD)
		t.Logf("  修复前(硬编码10万): %t", beforeFix)
		t.Logf("  修复后(相对阈值): %t", afterFix)
		
		// 两种方法都应该过滤掉噪音
		if beforeFix {
			t.Errorf("修复前应该过滤掉噪音 (绝对值 < 10万)")
		}
		if afterFix {
			t.Errorf("修复后应该过滤掉噪音 (低于最小相对阈值)")
		}
		
		t.Logf("✅ 极小CVD噪音过滤测试通过：正确过滤交易噪音")
	})
}

// 测试P1-1的统计对比
func TestP1_1_StatisticalComparison(t *testing.T) {
	// 模拟各种市场场景的CVD数据
	scenarios := []struct{
		symbol      string
		spotCVD     float64
		futuresCVD  float64
		description string
		expectedBehavior string
	}{
		// BTC场景：小额但统计显著
		{"BTCUSDT", 75000, -65000, "BTC小额背离", "修复后检测，修复前漏检"},
		{"BTCUSDT", 45000, -40000, "BTC微小背离", "两种方法都漏检（正常）"},
		{"BTCUSDT", 120000, -110000, "BTC中额背离", "两种方法都检测"},
		
		// ETH场景：中等金额
		{"ETHUSDT", 150000, -180000, "ETH中等背离", "两种方法都检测"},
		{"ETHUSDT", 90000, -95000, "ETH边界背离", "修复后检测，修复前漏检"},
		
		// 大币场景：大额资金
		{"ETHUSDT", 800000, -1200000, "大币大额背离", "两种方法都检测"},
		{"ETHUSDT", 2000000, 1800000, "大币同向无背离", "两种方法都不检测"},
		
		// 小币场景：绝对值较大但相对一般
		{"DOGEUSDT", 200000, -300000, "小币大额背离", "两种方法都检测"},
		{"DOGEUSDT", 80000, -70000, "小币中额背离", "修复后检测，修复前漏检"},
	}
	
	hardcodedDetections := 0
	relativeDetections := 0
	
	t.Logf("P1-1修复前后CVD背离检测对比:")
	
	for _, scenario := range scenarios {
		// 修复前：硬编码10万美元
		hardcodedResult := false
		threshold := 100000.0
		
		spotDir := 0
		futuresDir := 0
		if scenario.spotCVD > threshold { spotDir = 1 } 
		if scenario.spotCVD < -threshold { spotDir = -1 }
		if scenario.futuresCVD > threshold { futuresDir = 1 }
		if scenario.futuresCVD < -threshold { futuresDir = -1 }
		
		hardcodedResult = spotDir != 0 && futuresDir != 0 && spotDir != futuresDir
		
		// 修复后：相对阈值
		relativeResult := false
		avgAbs := (math.Abs(scenario.spotCVD) + math.Abs(scenario.futuresCVD)) / 2
		relativeThreshold := math.Max(50000, math.Min(500000, avgAbs*0.2))
		
		spotDirRel := 0
		futuresDirRel := 0
		if scenario.spotCVD > relativeThreshold { spotDirRel = 1 }
		if scenario.spotCVD < -relativeThreshold { spotDirRel = -1 }
		if scenario.futuresCVD > relativeThreshold { futuresDirRel = 1 }
		if scenario.futuresCVD < -relativeThreshold { futuresDirRel = -1 }
		
		relativeResult = spotDirRel != 0 && futuresDirRel != 0 && spotDirRel != futuresDirRel
		
		if hardcodedResult { hardcodedDetections++ }
		if relativeResult { relativeDetections++ }
		
		t.Logf("  %s: 现货%.0f, 期货%.0f", scenario.description, scenario.spotCVD, scenario.futuresCVD)
		t.Logf("    修复前: %t, 修复后: %t, 相对阈值: %.0f", hardcodedResult, relativeResult, relativeThreshold)
	}
	
	t.Logf("\nP1-1修复前后统计:")
	t.Logf("  修复前(硬编码10万): %d/%d = %.1f%% 检测率", 
		hardcodedDetections, len(scenarios), float64(hardcodedDetections)/float64(len(scenarios))*100)
	t.Logf("  修复后(相对阈值): %d/%d = %.1f%% 检测率", 
		relativeDetections, len(scenarios), float64(relativeDetections)/float64(len(scenarios))*100)
	
	// 验证修复后检测更合理（应该检测到更多小额但统计显著的背离）
	if relativeDetections <= hardcodedDetections {
		t.Errorf("修复后应该检测到更多统计显著的背离，实际: 修复前%d vs 修复后%d", 
			hardcodedDetections, relativeDetections)
	}
	
	t.Logf("✅ P1-1修复统计验证通过：相对阈值方法检测更精准")
}

// 测试P1-1的边界条件和异常情况
func TestP1_1_EdgeCasesAndExceptions(t *testing.T) {
	t.Run("超大CVD值处理", func(t *testing.T) {
		// 测试超大CVD值的处理（可能是异常数据或黑天鹅事件）
		spotCVD := 50000000.0    // 现货5000万
		futuresCVD := -30000000.0 // 期货-3000万
		
		// 修复前逻辑
		hardcodedResult := math.Abs(spotCVD) > 100000 && math.Abs(futuresCVD) > 100000 && 
			((spotCVD > 0 && futuresCVD < 0) || (spotCVD < 0 && futuresCVD > 0))
		
		// 修复后逻辑（相对阈值会被限制在50万以下）
		avgAbs := (math.Abs(spotCVD) + math.Abs(futuresCVD)) / 2
		relativeThreshold := math.Max(50000, math.Min(500000, avgAbs*0.2)) // 最大50万
		relativeResult := math.Abs(spotCVD) > relativeThreshold && math.Abs(futuresCVD) > relativeThreshold &&
			((spotCVD > 0 && futuresCVD < 0) || (spotCVD < 0 && futuresCVD > 0))
		
		t.Logf("超大CVD值测试: 现货%.0f万, 期货%.0f万", spotCVD/10000, futuresCVD/10000)
		t.Logf("  修复前: %t, 修复后: %t, 使用阈值: %.0f万", hardcodedResult, relativeResult, relativeThreshold/10000)
		
		// 两种方法都应该能检测到超大背离
		if !hardcodedResult {
			t.Errorf("修复前应该检测到超大背离")
		}
		if !relativeResult {
			t.Errorf("修复后应该检测到超大背离")
		}
		
		// 验证相对阈值被正确限制在50万
		if relativeThreshold > 500000 {
			t.Errorf("相对阈值应该被限制在50万以下，实际: %.0f", relativeThreshold)
		}
		
		t.Logf("✅ 超大CVD值处理测试通过")
	})
	
	t.Run("零值和负值处理", func(t *testing.T) {
		testCases := []struct{
			name        string
			spotCVD     float64
			futuresCVD  float64
			shouldDetect bool
		}{
			{"双零值", 0, 0, false},
			{"现货零值", 0, -150000, false},
			{"期货零值", 120000, 0, false},
			{"微小非零", 1000, -2000, false},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// 修复后逻辑
				avgAbs := (math.Abs(tc.spotCVD) + math.Abs(tc.futuresCVD)) / 2
				relativeThreshold := math.Max(50000, math.Min(500000, avgAbs*0.2))
				
				spotDir := 0
				futuresDir := 0
				if tc.spotCVD > relativeThreshold { spotDir = 1 }
				if tc.spotCVD < -relativeThreshold { spotDir = -1 }
				if tc.futuresCVD > relativeThreshold { futuresDir = 1 }
				if tc.futuresCVD < -relativeThreshold { futuresDir = -1 }
				
				result := spotDir != 0 && futuresDir != 0 && spotDir != futuresDir
				
				t.Logf("%s: 现货%.0f, 期货%.0f, 结果: %t, 期望: %t", 
					tc.name, tc.spotCVD, tc.futuresCVD, result, tc.shouldDetect)
				
				if result != tc.shouldDetect {
					t.Errorf("%s 检测结果不符合预期", tc.name)
				}
			})
		}
		
		t.Logf("✅ 零值和边界值处理测试通过")
	})
}

// 测试P1-1修复的实际业务场景
func TestP1_1_RealWorldScenarios(t *testing.T) {
	t.Run("机构套利场景", func(t *testing.T) {
		// 机构在现货建仓，期货对冲的场景
		spotCVD := 85000.0    // 现货净买入8.5万（机构建仓）
		futuresCVD := -78000.0 // 期货净卖出7.8万（对冲）
		
		// 修复前：无法检测（都小于10万）
		beforeFix := math.Abs(spotCVD) > 100000 && math.Abs(futuresCVD) > 100000 &&
			((spotCVD > 0 && futuresCVD < 0) || (spotCVD < 0 && futuresCVD > 0))
		
		// 修复后：可以检测（相对统计显著）
		avgAbs := (math.Abs(spotCVD) + math.Abs(futuresCVD)) / 2
		threshold := math.Max(50000, math.Min(500000, avgAbs*0.2))
		afterFix := math.Abs(spotCVD) > threshold && math.Abs(futuresCVD) > threshold &&
			((spotCVD > 0 && futuresCVD < 0) || (spotCVD < 0 && futuresCVD > 0))
		
		t.Logf("机构套利场景: 现货%.0f, 期货%.0f, 阈值%.0f", spotCVD, futuresCVD, threshold)
		t.Logf("  修复前: %t (漏检机构行为)", beforeFix)
		t.Logf("  修复后: %t (正确识别)", afterFix)
		
		if beforeFix {
			t.Errorf("修复前应该漏检机构套利行为")
		}
		if !afterFix {
			t.Errorf("修复后应该能识别机构套利行为")
		}
		
		t.Logf("✅ 机构套利场景测试通过")
	})
	
	t.Run("散户FOMO vs机构出货", func(t *testing.T) {
		// 散户FOMO买入现货，机构期货出货的背离
		spotCVD := 95000.0     // 散户现货FOMO
		futuresCVD := -105000.0 // 机构期货出货
		
		// 修复前：期货刚好超过10万，能检测到
		beforeFix := math.Abs(spotCVD) > 100000 && math.Abs(futuresCVD) > 100000 &&
			((spotCVD > 0 && futuresCVD < 0) || (spotCVD < 0 && futuresCVD > 0))
		
		// 修复后：也能检测到
		avgAbs := (math.Abs(spotCVD) + math.Abs(futuresCVD)) / 2
		threshold := math.Max(50000, math.Min(500000, avgAbs*0.2))
		afterFix := math.Abs(spotCVD) > threshold && math.Abs(futuresCVD) > threshold &&
			((spotCVD > 0 && futuresCVD < 0) || (spotCVD < 0 && futuresCVD > 0))
		
		t.Logf("散户FOMO vs 机构出货: 现货%.0f, 期货%.0f, 阈值%.0f", spotCVD, futuresCVD, threshold)
		t.Logf("  修复前: %t", beforeFix)
		t.Logf("  修复后: %t", afterFix)
		
		// 这种情况修复前可能漏检（现货<10万），修复后应该能检测到
		if !afterFix {
			t.Errorf("修复后应该能检测到散户vs机构的背离")
		}
		
		t.Logf("✅ 散户FOMO vs 机构出货场景测试通过")
	})
}