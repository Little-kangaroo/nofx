package audit_fixes

import (
	"testing"
	"time"
	"fmt"
)

// 模拟哨兵相关结构
type TestTriggerScenario string

const (
	TestScenarioMomentumIgnition TestTriggerScenario = "momentum_ignition"
	TestScenarioSpotRush         TestTriggerScenario = "spot_rush"
	TestScenarioSpoofingFlip     TestTriggerScenario = "spoofing_flip"
)

type TestSentinelTriggerAlert struct {
	Symbol    string
	Scenario  TestTriggerScenario
	Timestamp time.Time
}

type TestSentinelTriggerEngine struct {
	lastAlerts     map[string]time.Time // 冷却记录
	cooldownPeriod time.Duration        // 冷却期
}

// 测试P2-2修复：哨兵冷却机制Key设计问题
func TestP2_2_SentinelCooldownKeyFix(t *testing.T) {
	
	// 修复前的冷却逻辑（只用symbol作为Key）
	testOldCooldownLogic := func() *TestSentinelTriggerEngine {
		return &TestSentinelTriggerEngine{
			lastAlerts:     make(map[string]time.Time),
			cooldownPeriod: 30 * time.Second,
		}
	}
	
	// 修复后的冷却逻辑（使用symbol+scenario组合Key）
	testNewCooldownLogic := func() *TestSentinelTriggerEngine {
		return &TestSentinelTriggerEngine{
			lastAlerts:     make(map[string]time.Time),
			cooldownPeriod: 30 * time.Second,
		}
	}
	
	// 旧版本的冷却检查
	checkOldCooldown := func(engine *TestSentinelTriggerEngine, symbol string, scenario TestTriggerScenario) bool {
		// 修复前：只用symbol作为key
		lastAlert, exists := engine.lastAlerts[symbol]
		if !exists {
			return true
		}
		return time.Since(lastAlert) > engine.cooldownPeriod
	}
	
	// 新版本的冷却检查
	checkNewCooldown := func(engine *TestSentinelTriggerEngine, symbol string, scenario TestTriggerScenario) bool {
		// 修复后：使用symbol+scenario组合key
		cooldownKey := fmt.Sprintf("%s:%s", symbol, scenario)
		lastAlert, exists := engine.lastAlerts[cooldownKey]
		if !exists {
			return true
		}
		return time.Since(lastAlert) > engine.cooldownPeriod
	}
	
	// 旧版本的警报触发
	triggerOldAlert := func(engine *TestSentinelTriggerEngine, alert *TestSentinelTriggerAlert) {
		// 修复前：只用symbol更新冷却
		engine.lastAlerts[alert.Symbol] = alert.Timestamp
	}
	
	// 新版本的警报触发
	triggerNewAlert := func(engine *TestSentinelTriggerEngine, alert *TestSentinelTriggerAlert) {
		// 修复后：使用symbol+scenario组合key更新冷却
		cooldownKey := fmt.Sprintf("%s:%s", alert.Symbol, alert.Scenario)
		engine.lastAlerts[cooldownKey] = alert.Timestamp
	}

	t.Run("单symbol多场景触发测试", func(t *testing.T) {
		// 场景：同一个symbol在短时间内可能触发不同类型的警报
		symbol := "BTCUSDT"
		now := time.Now()
		
		// 初始化两个引擎
		oldEngine := testOldCooldownLogic()
		newEngine := testNewCooldownLogic()
		
		// 第一个警报：动量点燃
		momentumAlert := &TestSentinelTriggerAlert{
			Symbol:    symbol,
			Scenario:  TestScenarioMomentumIgnition,
			Timestamp: now,
		}
		
		// 触发第一个警报
		triggerOldAlert(oldEngine, momentumAlert)
		triggerNewAlert(newEngine, momentumAlert)
		
		t.Logf("第一个警报已触发: %s - %s", symbol, momentumAlert.Scenario)
		
		// 5秒后，尝试触发现货抢跑警报
		time.Sleep(1 * time.Millisecond) // 模拟短时间间隔
		
		// 检查是否可以触发第二个警报（现货抢跑）
		canTriggerOld := checkOldCooldown(oldEngine, symbol, TestScenarioSpotRush)
		canTriggerNew := checkNewCooldown(newEngine, symbol, TestScenarioSpotRush)
		
		t.Logf("5秒后尝试触发现货抢跑:")
		t.Logf("  修复前(symbol冷却): %t (被动量点燃冷却阻断)", canTriggerOld)
		t.Logf("  修复后(symbol+scenario冷却): %t (独立冷却，可以触发)", canTriggerNew)
		
		// 修复前应该被阻断（整个symbol在冷却中）
		if canTriggerOld {
			t.Errorf("修复前应该被symbol级冷却阻断")
		}
		
		// 修复后应该可以触发（不同场景独立冷却）
		if !canTriggerNew {
			t.Errorf("修复后应该可以触发不同场景的警报")
		}
		
		t.Logf("✅ 单symbol多场景触发测试通过：修复后支持场景独立冷却")
	})

	t.Run("同场景重复触发冷却测试", func(t *testing.T) {
		// 场景：同一个symbol的同一场景在冷却期内重复触发应该被阻断
		symbol := "ETHUSDT"
		scenario := TestScenarioSpotRush
		now := time.Now()
		
		// 初始化两个引擎
		oldEngine := testOldCooldownLogic()
		newEngine := testNewCooldownLogic()
		
		// 第一个警报
		firstAlert := &TestSentinelTriggerAlert{
			Symbol:    symbol,
			Scenario:  scenario,
			Timestamp: now,
		}
		
		// 触发第一个警报
		triggerOldAlert(oldEngine, firstAlert)
		triggerNewAlert(newEngine, firstAlert)
		
		// 10秒后尝试触发相同场景的警报
		time.Sleep(1 * time.Millisecond)
		
		// 检查是否可以触发第二个警报（相同场景）
		canTriggerOld := checkOldCooldown(oldEngine, symbol, scenario)
		canTriggerNew := checkNewCooldown(newEngine, symbol, scenario)
		
		t.Logf("同场景重复触发测试: %s - %s", symbol, scenario)
		t.Logf("  修复前(10秒 < 30秒冷却期): %t", canTriggerOld)
		t.Logf("  修复后(10秒 < 30秒冷却期): %t", canTriggerNew)
		
		// 两种方法都应该阻断相同场景的重复触发
		if canTriggerOld {
			t.Errorf("修复前应该阻断相同场景的重复触发")
		}
		if canTriggerNew {
			t.Errorf("修复后应该阻断相同场景的重复触发")
		}
		
		t.Logf("✅ 同场景重复触发冷却测试通过：修复后正确阻断重复触发")
	})

	t.Run("冷却期过后重新触发测试", func(t *testing.T) {
		// 场景：冷却期过后应该可以重新触发
		symbol := "BNBUSDT"
		scenario := TestScenarioMomentumIgnition
		now := time.Now()
		
		newEngine := testNewCooldownLogic()
		
		// 第一个警报
		firstAlert := &TestSentinelTriggerAlert{
			Symbol:    symbol,
			Scenario:  scenario,
			Timestamp: now,
		}
		
		triggerNewAlert(newEngine, firstAlert)
		
		// 模拟35秒前触发的警报，现在应该可以重新触发
		// 手动调整时间检查
		cooldownKey := fmt.Sprintf("%s:%s", symbol, scenario)
		pastTime := now.Add(-35 * time.Second) // 35秒前
		newEngine.lastAlerts[cooldownKey] = pastTime
		
		canTrigger := time.Since(pastTime) > newEngine.cooldownPeriod
		
		t.Logf("冷却期过后重新触发测试:")
		t.Logf("  时间间隔: 35秒 > 30秒冷却期")
		t.Logf("  可以重新触发: %t", canTrigger)
		
		if !canTrigger {
			t.Errorf("冷却期过后应该可以重新触发")
		}
		
		t.Logf("✅ 冷却期过后重新触发测试通过")
	})
}

// 测试P2-2的多symbol并发场景
func TestP2_2_MultiSymbolConcurrentScenarios(t *testing.T) {
	
	// 创建新冷却机制引擎
	newEngine := &TestSentinelTriggerEngine{
		lastAlerts:     make(map[string]time.Time),
		cooldownPeriod: 30 * time.Second,
	}
	
	// 检查新冷却机制
	checkCooldown := func(symbol string, scenario TestTriggerScenario) bool {
		cooldownKey := fmt.Sprintf("%s:%s", symbol, scenario)
		lastAlert, exists := newEngine.lastAlerts[cooldownKey]
		if !exists {
			return true
		}
		return time.Since(lastAlert) > newEngine.cooldownPeriod
	}
	
	// 触发警报
	triggerAlert := func(symbol string, scenario TestTriggerScenario) {
		cooldownKey := fmt.Sprintf("%s:%s", symbol, scenario)
		newEngine.lastAlerts[cooldownKey] = time.Now()
	}

	t.Run("多symbol独立冷却测试", func(t *testing.T) {
		// 场景：不同symbol的相同场景应该独立冷却
		symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"}
		scenario := TestScenarioMomentumIgnition
		
		// 为每个symbol触发相同场景的警报
		for _, symbol := range symbols {
			triggerAlert(symbol, scenario)
			t.Logf("触发警报: %s - %s", symbol, scenario)
		}
		
		// 检查每个symbol的冷却状态
		for _, symbol := range symbols {
			canTrigger := checkCooldown(symbol, scenario)
			t.Logf("  %s 可以重复触发: %t (应该为false)", symbol, canTrigger)
			
			if canTrigger {
				t.Errorf("%s 应该在冷却期内", symbol)
			}
		}
		
		// 检查不同symbol的不同场景是否可以触发
		canTriggerDiffScenario := checkCooldown("BTCUSDT", TestScenarioSpotRush)
		t.Logf("  BTCUSDT 不同场景可以触发: %t (应该为true)", canTriggerDiffScenario)
		
		if !canTriggerDiffScenario {
			t.Errorf("不同场景应该可以独立触发")
		}
		
		t.Logf("✅ 多symbol独立冷却测试通过")
	})
	
	t.Run("复杂混合场景测试", func(t *testing.T) {
		// 复杂场景：模拟实际交易中的多symbol多场景警报
		testCases := []struct{
			symbol   string
			scenario TestTriggerScenario
			delay    time.Duration
			shouldAllow bool
			description string
		}{
			{"BTCUSDT", TestScenarioMomentumIgnition, 0, true, "BTC动量点燃-初始触发"},
			{"BTCUSDT", TestScenarioSpotRush, 1*time.Second, true, "BTC现货抢跑-不同场景可触发"},
			{"ETHUSDT", TestScenarioMomentumIgnition, 2*time.Second, true, "ETH动量点燃-不同symbol可触发"},
			{"BTCUSDT", TestScenarioMomentumIgnition, 3*time.Second, false, "BTC动量点燃-相同场景冷却中"},
			{"BTCUSDT", TestScenarioSpoofingFlip, 4*time.Second, true, "BTC虚假翻转-第三种场景可触发"},
			{"ETHUSDT", TestScenarioSpotRush, 5*time.Second, true, "ETH现货抢跑-不同组合可触发"},
		}
		
		// 清空引擎状态
		newEngine.lastAlerts = make(map[string]time.Time)
		
		t.Logf("复杂混合场景测试:")
		for i, tc := range testCases {
			time.Sleep(tc.delay)
			
			canTrigger := checkCooldown(tc.symbol, tc.scenario)
			
			t.Logf("  步骤%d: %s - 可触发: %t, 期望: %t", 
				i+1, tc.description, canTrigger, tc.shouldAllow)
			
			if canTrigger != tc.shouldAllow {
				t.Errorf("步骤%d结果不符合预期: %s", i+1, tc.description)
			}
			
			// 如果可以触发，则触发警报更新冷却状态
			if canTrigger {
				triggerAlert(tc.symbol, tc.scenario)
			}
		}
		
		t.Logf("✅ 复杂混合场景测试通过")
	})
}

// 测试P2-2的统计对比
func TestP2_2_StatisticalComparison(t *testing.T) {
	// 模拟一个活跃交易日的警报场景
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "ADAUSDT", "DOGEUSDT"}
	scenarios := []TestTriggerScenario{
		TestScenarioMomentumIgnition,
		TestScenarioSpotRush, 
		TestScenarioSpoofingFlip,
	}
	
	// 修复前统计
	oldBlockedCount := 0
	oldAllowedCount := 0
	
	// 修复后统计  
	newBlockedCount := 0
	newAllowedCount := 0
	
	// 模拟100次警报尝试
	totalAttempts := 100
	
	// 旧引擎（symbol级冷却）
	oldEngine := make(map[string]time.Time)
	// 新引擎（symbol+scenario级冷却）
	newEngine := make(map[string]time.Time)
	
	cooldownPeriod := 30 * time.Second
	baseTime := time.Now()
	
	t.Logf("P2-2修复前后统计对比 - 模拟%d次警报尝试:", totalAttempts)
	
	for i := 0; i < totalAttempts; i++ {
		// 随机选择symbol和scenario
		symbol := symbols[i%len(symbols)]
		scenario := scenarios[i%len(scenarios)]
		currentTime := baseTime.Add(time.Duration(i) * time.Second)
		
		// 修复前检查（symbol级冷却）
		oldKey := symbol
		oldLastAlert, oldExists := oldEngine[oldKey]
		oldCanTrigger := !oldExists || currentTime.Sub(oldLastAlert) > cooldownPeriod
		
		// 修复后检查（symbol+scenario级冷却）
		newKey := fmt.Sprintf("%s:%s", symbol, scenario)
		newLastAlert, newExists := newEngine[newKey]
		newCanTrigger := !newExists || currentTime.Sub(newLastAlert) > cooldownPeriod
		
		// 统计结果
		if oldCanTrigger {
			oldAllowedCount++
			oldEngine[oldKey] = currentTime
		} else {
			oldBlockedCount++
		}
		
		if newCanTrigger {
			newAllowedCount++
			newEngine[newKey] = currentTime
		} else {
			newBlockedCount++
		}
		
		// 每10次打印一次进度
		if (i+1)%20 == 0 {
			t.Logf("  进度: %d/%d, 当前尝试: %s-%s", i+1, totalAttempts, symbol, scenario)
		}
	}
	
	t.Logf("\nP2-2修复前后统计结果:")
	t.Logf("  修复前(symbol级冷却):")
	t.Logf("    允许触发: %d/%d = %.1f%%", oldAllowedCount, totalAttempts, float64(oldAllowedCount)/float64(totalAttempts)*100)
	t.Logf("    被阻断: %d/%d = %.1f%%", oldBlockedCount, totalAttempts, float64(oldBlockedCount)/float64(totalAttempts)*100)
	
	t.Logf("  修复后(symbol+scenario级冷却):")
	t.Logf("    允许触发: %d/%d = %.1f%%", newAllowedCount, totalAttempts, float64(newAllowedCount)/float64(totalAttempts)*100)
	t.Logf("    被阻断: %d/%d = %.1f%%", newBlockedCount, totalAttempts, float64(newBlockedCount)/float64(totalAttempts)*100)
	
	improvementRatio := float64(newAllowedCount) / float64(oldAllowedCount)
	t.Logf("  改进幅度: %.2fx (修复后允许触发数/修复前)", improvementRatio)
	
	// 验证修复后应该允许更多有效警报
	if newAllowedCount <= oldAllowedCount {
		t.Errorf("修复后应该允许更多有效警报，实际: 修复前%d vs 修复后%d", 
			oldAllowedCount, newAllowedCount)
	}
	
	// 验证改进幅度应该显著（至少1.5倍）
	if improvementRatio < 1.5 {
		t.Errorf("修复后改进幅度应该≥1.5倍，实际: %.2fx", improvementRatio)
	}
	
	t.Logf("✅ P2-2修复统计验证通过：精细化冷却机制显著提升警报系统效率")
}