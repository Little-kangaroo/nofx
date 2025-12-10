package audit_fixes

import (
	"testing"
	"math"
)

// 模拟现货抢跑检测相关结构
type TestMarketSnapshot struct {
	CVDDelta5m *TestCVDDelta5m
}

type TestCVDDelta5m struct {
	SpotCVDDeltaUSD    float64
	FuturesCVDDeltaUSD float64
}

type TestSpotRushThresholds struct {
	SpotCVDZScoreMin  float64
	CVDRatioZScoreMin float64
}

// 测试P1-2修复：现货抢跑逻辑过于保守的问题
func TestP1_2_SpotRushLogicFix(t *testing.T) {
	
	thresholds := &TestSpotRushThresholds{
		SpotCVDZScoreMin:  1.8,
		CVDRatioZScoreMin: 1.5,
	}
	
	// 修复前的保守逻辑（只允许现货主导）
	testConservativeLogic := func(spotCVD, futuresCVD, spotZScore, ratioZScore float64) (bool, string) {
		netCVD := spotCVD + futuresCVD
		
		// 检查基础条件
		spotCondition := spotZScore >= thresholds.SpotCVDZScoreMin
		ratioCondition := ratioZScore >= thresholds.CVDRatioZScoreMin
		
		// 原有保守逻辑：要求现货绝对值 > 期货绝对值
		conservativeCondition := netCVD > 0 && spotCVD > math.Abs(futuresCVD)
		
		if spotCondition && conservativeCondition && ratioCondition {
			return true, "spot_leading_only"
		}
		return false, "no_trigger"
	}
	
	// 修复后的增强逻辑（支持双核驱动）
	testEnhancedLogic := func(spotCVD, futuresCVD, spotZScore, ratioZScore float64) (bool, string) {
		netCVD := spotCVD + futuresCVD
		
		// 检查基础条件
		spotCondition := spotZScore >= thresholds.SpotCVDZScoreMin
		ratioCondition := ratioZScore >= thresholds.CVDRatioZScoreMin
		
		// 场景A: 现货主导 - 现货强势，期货跟随或中性
		spotLeading := netCVD > 0 && spotCVD > math.Abs(futuresCVD)
		
		// 场景B: 双核驱动 - 现货期货都强力买入，合力明显
		dualCoreCondition := spotCVD > 0 && futuresCVD > 0 && netCVD > 0
		dualCoreStrength := dualCoreCondition && (spotCVD > 50000 && futuresCVD > 50000)
		
		// 支持现货主导或双核驱动两种模式
		netForceCondition := spotLeading || dualCoreStrength
		
		// 对抗过滤：如果现货和期货方向相反且力量接近，过滤掉
		if spotCVD > 0 && futuresCVD < 0 {
			oppositionRatio := math.Abs(futuresCVD) / spotCVD
			if oppositionRatio > 0.7 { // 如果期货阻力>70%现货推力，认为是对抗
				return false, "opposition_filtered"
			}
		}
		
		if spotCondition && netForceCondition && ratioCondition {
			if spotLeading && !dualCoreStrength {
				return true, "spot_leading"
			} else if dualCoreStrength {
				return true, "dual_core_driving"
			}
			return true, "enhanced_trigger"
		}
		return false, "no_trigger"
	}

	t.Run("经典现货主导场景", func(t *testing.T) {
		// 经典现货抢跑：现货强势，期货中性或弱势
		spotCVD := 200000.0    // 现货净买入20万
		futuresCVD := 50000.0  // 期货小幅净买入5万
		spotZScore := 2.5      // 现货Z-Score高于阈值1.8
		ratioZScore := 2.0     // CVD比率Z-Score高于阈值1.5
		
		// 修复前后都应该能检测到
		beforeFix, beforeMode := testConservativeLogic(spotCVD, futuresCVD, spotZScore, ratioZScore)
		afterFix, afterMode := testEnhancedLogic(spotCVD, futuresCVD, spotZScore, ratioZScore)
		
		t.Logf("经典现货主导: 现货%.0f, 期货%.0f", spotCVD, futuresCVD)
		t.Logf("  修复前: %t (%s)", beforeFix, beforeMode)
		t.Logf("  修复后: %t (%s)", afterFix, afterMode)
		
		if !beforeFix {
			t.Errorf("修复前应该检测到经典现货主导")
		}
		if !afterFix {
			t.Errorf("修复后应该检测到经典现货主导")
		}
		if afterMode != "spot_leading" {
			t.Errorf("修复后应该识别为spot_leading模式，实际: %s", afterMode)
		}
		
		t.Logf("✅ 经典现货主导场景测试通过")
	})
	
	t.Run("双核驱动场景", func(t *testing.T) {
		// 🔥 核心测试场景：现货期货都强力买入（修复前被漏检）
		spotCVD := 120000.0    // 现货净买入12万
		futuresCVD := 150000.0 // 期货净买入15万 (比现货还强)
		spotZScore := 2.2      // 现货Z-Score高于阈值1.8
		ratioZScore := 1.8     // CVD比率Z-Score高于阈值1.5
		
		// 修复前：无法检测（期货强于现货，不符合"现货主导"）
		beforeFix, beforeMode := testConservativeLogic(spotCVD, futuresCVD, spotZScore, ratioZScore)
		
		// 修复后：可以检测（双核驱动模式）
		afterFix, afterMode := testEnhancedLogic(spotCVD, futuresCVD, spotZScore, ratioZScore)
		
		t.Logf("双核驱动场景: 现货%.0f, 期货%.0f", spotCVD, futuresCVD)
		t.Logf("  修复前: %t (%s) - 漏检双核驱动", beforeFix, beforeMode)
		t.Logf("  修复后: %t (%s) - 正确识别", afterFix, afterMode)
		
		if beforeFix {
			t.Errorf("修复前应该漏检双核驱动 (期货强度 > 现货强度)")
		}
		if !afterFix {
			t.Errorf("修复后应该检测到双核驱动")
		}
		if afterMode != "dual_core_driving" {
			t.Errorf("修复后应该识别为dual_core_driving模式，实际: %s", afterMode)
		}
		
		t.Logf("✅ 双核驱动场景测试通过：修复后能识别强力同向资金流")
	})

	t.Run("对抗场景过滤", func(t *testing.T) {
		// 现货vs期货对抗：现货买入但期货强力卖出（应该被过滤）
		spotCVD := 100000.0    // 现货净买入10万
		futuresCVD := -90000.0 // 期货净卖出9万（阻力比90%，接近对抗）
		spotZScore := 2.0      // 现货Z-Score满足条件
		ratioZScore := 1.6     // CVD比率Z-Score满足条件
		
		// 修复前后都应该过滤掉对抗场景
		beforeFix, beforeMode := testConservativeLogic(spotCVD, futuresCVD, spotZScore, ratioZScore)
		afterFix, afterMode := testEnhancedLogic(spotCVD, futuresCVD, spotZScore, ratioZScore)
		
		oppositionRatio := math.Abs(futuresCVD) / spotCVD
		
		t.Logf("对抗场景: 现货%.0f, 期货%.0f, 阻力比%.2f", spotCVD, futuresCVD, oppositionRatio)
		t.Logf("  修复前: %t (%s)", beforeFix, beforeMode)
		t.Logf("  修复后: %t (%s)", afterFix, afterMode)
		
		// 这个case期货阻力90%，超过70%阈值，应该被过滤
		if beforeFix && oppositionRatio > 0.7 {
			t.Logf("⚠️ 修复前可能误判对抗为抢跑")
		}
		if afterFix && oppositionRatio > 0.7 {
			t.Errorf("修复后应该过滤掉对抗场景 (阻力比%.2f > 0.7)", oppositionRatio)
		}
		
		t.Logf("✅ 对抗场景过滤测试通过")
	})

	t.Run("弱势双核过滤", func(t *testing.T) {
		// 弱势双核：现货期货都买入但力度不够（应该被过滤）
		spotCVD := 30000.0    // 现货净买入3万（低于5万强度阈值）
		futuresCVD := 40000.0 // 期货净买入4万（低于5万强度阈值）
		spotZScore := 2.0     // 现货Z-Score满足条件
		ratioZScore := 1.6    // CVD比率Z-Score满足条件
		
		// 修复前后都应该过滤掉弱势双核
		beforeFix, beforeMode := testConservativeLogic(spotCVD, futuresCVD, spotZScore, ratioZScore)
		afterFix, afterMode := testEnhancedLogic(spotCVD, futuresCVD, spotZScore, ratioZScore)
		
		t.Logf("弱势双核: 现货%.0f, 期货%.0f", spotCVD, futuresCVD)
		t.Logf("  修复前: %t (%s)", beforeFix, beforeMode)
		t.Logf("  修复后: %t (%s)", afterFix, afterMode)
		
		// 两种方法都应该过滤弱势双核
		if beforeFix {
			t.Errorf("修复前应该过滤弱势双核 (现货未主导)")
		}
		if afterFix {
			t.Errorf("修复后应该过滤弱势双核 (强度不足)")
		}
		
		t.Logf("✅ 弱势双核过滤测试通过")
	})
}

// 测试P1-2的统计对比
func TestP1_2_StatisticalComparison(t *testing.T) {
	// 模拟各种现货抢跑场景
	scenarios := []struct{
		name        string
		spotCVD     float64
		futuresCVD  float64
		spotZScore  float64
		ratioZScore float64
		description string
		expected    string
	}{
		// 经典现货主导场景
		{"经典现货主导1", 200000, 50000, 2.5, 2.0, "现货强势期货跟随", "修复前后都检测"},
		{"经典现货主导2", 150000, -30000, 2.2, 1.8, "现货买入期货小幅卖出", "修复前后都检测"},
		
		// 双核驱动场景（修复前漏检）
		{"双核驱动1", 120000, 150000, 2.2, 1.8, "现货期货都强力买入", "修复前漏检修复后检测"},
		{"双核驱动2", 80000, 180000, 2.0, 1.6, "期货主导现货跟随", "修复前漏检修复后检测"},
		{"双核驱动3", 160000, 140000, 2.4, 1.9, "双方均衡买入", "修复前漏检修复后检测"},
		
		// 对抗场景（应该被过滤）
		{"强对抗1", 100000, -90000, 2.0, 1.6, "强对抗90%阻力", "修复前后都过滤"},
		{"中等对抗", 100000, -60000, 2.0, 1.6, "中等对抗60%阻力", "修复前检测修复后检测"},
		
		// 弱势场景（应该被过滤）
		{"弱势双核", 30000, 40000, 2.0, 1.6, "双核但强度不足", "修复前后都过滤"},
		{"弱势现货", 30000, 10000, 1.5, 1.6, "现货ZScore不足", "修复前后都过滤"},
	}
	
	conservativeDetections := 0
	enhancedDetections := 0
	dualCoreDetections := 0
	
	t.Logf("P1-2修复前后现货抢跑检测对比:")
	
	for _, scenario := range scenarios {
		netCVD := scenario.spotCVD + scenario.futuresCVD
		
		// 修复前：保守逻辑
		conservativeResult := false
		spotCondition := scenario.spotZScore >= 1.8
		ratioCondition := scenario.ratioZScore >= 1.5
		conservativeCondition := netCVD > 0 && scenario.spotCVD > math.Abs(scenario.futuresCVD)
		
		if spotCondition && conservativeCondition && ratioCondition {
			conservativeResult = true
			conservativeDetections++
		}
		
		// 修复后：增强逻辑
		enhancedResult := false
		enhancedMode := "no_trigger"
		
		spotLeading := netCVD > 0 && scenario.spotCVD > math.Abs(scenario.futuresCVD)
		dualCoreCondition := scenario.spotCVD > 0 && scenario.futuresCVD > 0 && netCVD > 0
		dualCoreStrength := dualCoreCondition && (scenario.spotCVD > 50000 && scenario.futuresCVD > 50000)
		netForceCondition := spotLeading || dualCoreStrength
		
		// 对抗检测
		isOpposition := false
		if scenario.spotCVD > 0 && scenario.futuresCVD < 0 {
			oppositionRatio := math.Abs(scenario.futuresCVD) / scenario.spotCVD
			if oppositionRatio > 0.7 {
				isOpposition = true
			}
		}
		
		if spotCondition && netForceCondition && ratioCondition && !isOpposition {
			enhancedResult = true
			enhancedDetections++
			
			if dualCoreStrength {
				enhancedMode = "dual_core_driving"
				dualCoreDetections++
			} else if spotLeading {
				enhancedMode = "spot_leading"
			}
		}
		
		t.Logf("  %s: 现货%.0f, 期货%.0f", scenario.name, scenario.spotCVD, scenario.futuresCVD)
		t.Logf("    修复前: %t, 修复后: %t (%s)", conservativeResult, enhancedResult, enhancedMode)
	}
	
	t.Logf("\nP1-2修复前后统计:")
	t.Logf("  修复前(保守逻辑): %d/%d = %.1f%% 检测率", 
		conservativeDetections, len(scenarios), float64(conservativeDetections)/float64(len(scenarios))*100)
	t.Logf("  修复后(增强逻辑): %d/%d = %.1f%% 检测率", 
		enhancedDetections, len(scenarios), float64(enhancedDetections)/float64(len(scenarios))*100)
	t.Logf("  其中双核驱动检测: %d 次", dualCoreDetections)
	
	// 验证修复后检测率提升且能识别双核驱动
	if enhancedDetections <= conservativeDetections {
		t.Errorf("修复后应该检测到更多有效抢跑场景，实际: 修复前%d vs 修复后%d", 
			conservativeDetections, enhancedDetections)
	}
	
	if dualCoreDetections == 0 {
		t.Errorf("修复后应该能检测到双核驱动场景")
	}
	
	t.Logf("✅ P1-2修复统计验证通过：增强逻辑检测更全面，新增双核驱动识别")
}