package market

import (
	"math"
	"testing"
	"time"
)

// TestFuzzyLogicBasics 测试模糊逻辑基础功能
func TestFuzzyLogicBasics(t *testing.T) {
	// 创建测试环境
	klines := createBTCKlines()
	calc := NewContextCalculator(klines)
	
	// 创建测试供需区
	zone := &SupplyDemandZone{
		ID:         "test_supply_fuzzy",
		Type:       SupplyZone,
		UpperBound: 50500.0,
		LowerBound: 50300.0,
		Width:      200.0,
		Status:     StatusFresh,
		IsActive:   true,
	}
	
	// 测试1: 计算缓冲区
	tolerance := calc.CalculateFuzzyTolerance(zone.Width)
	expectedMin := calc.market.ATR14 * 0.02 // 最小2% ATR
	expectedMax := math.Min(0.1*zone.Width, 0.05*calc.market.ATR14)
	
	t.Logf("=== 模糊逻辑缓冲区测试 ===")
	t.Logf("区域宽度: $%.2f", zone.Width)
	t.Logf("ATR14: $%.2f", calc.market.ATR14)
	t.Logf("计算缓冲区: $%.2f", tolerance)
	t.Logf("期望范围: $%.2f - $%.2f", expectedMin, expectedMax)
	
	if tolerance < expectedMin {
		t.Errorf("缓冲区过小: %.2f < %.2f", tolerance, expectedMin)
	}
	if tolerance > expectedMax+1.0 { // 允许小误差
		t.Errorf("缓冲区过大: %.2f > %.2f", tolerance, expectedMax)
	}
}

// TestFuzzyTouchLogic 测试模糊Touch逻辑
func TestFuzzyTouchLogic(t *testing.T) {
	klines := createBTCKlines()
	calc := NewContextCalculator(klines)
	
	// 供给区测试
	supplyZone := &SupplyDemandZone{
		Type:       SupplyZone,
		UpperBound: 50500.0,
		LowerBound: 50300.0,  // 下边界
		Width:      200.0,
	}
	
	tolerance := calc.CalculateFuzzyTolerance(supplyZone.Width)
	
	testCases := []struct {
		name     string
		high     float64
		low      float64
		expected bool
		reason   string
	}{
		// 供给区测试用例
		{"明确未触及", 50250.0, 50200.0, false, "高点低于下边界缓冲区"},
		{"缓冲区边缘", 50300.0 - tolerance + 1, 50250.0, true, "高点进入下边界缓冲区"},
		{"深度穿透", 50400.0, 50350.0, true, "高点穿透供给区"},
		{"完全穿透", 50550.0, 50500.0, true, "高点完全穿透供给区"},
	}
	
	t.Logf("=== 供给区Touch测试 (缓冲区: ±%.2f) ===", tolerance)
	for _, tc := range testCases {
		kline := Kline{High: tc.high, Low: tc.low, Close: (tc.high + tc.low) / 2}
		result := calc.IsFuzzyTouch(kline, supplyZone)
		
		if result != tc.expected {
			t.Errorf("%s: 期望%v, 实际%v - %s", tc.name, tc.expected, result, tc.reason)
		} else {
			t.Logf("✓ %s: %v - %s", tc.name, result, tc.reason)
		}
	}
}

// TestFuzzyBreakLogic 测试模糊Break逻辑
func TestFuzzyBreakLogic(t *testing.T) {
	klines := createBTCKlines()
	calc := NewContextCalculator(klines)
	
	// 供给区测试
	supplyZone := &SupplyDemandZone{
		Type:       SupplyZone,
		UpperBound: 50500.0,  // 上边界
		LowerBound: 50300.0,
		Width:      200.0,
	}
	
	atrThreshold := 0.2 * calc.market.ATR14
	
	testCases := []struct {
		name     string
		close    float64
		expected bool
		reason   string
	}{
		{"影线刺破", 50520.0, false, "收盘价在区域内，不算破坏"},
		{"微小突破", 50500.0 + atrThreshold/2, false, "突破幅度不足0.2ATR"},
		{"有效突破", 50500.0 + atrThreshold + 10, true, "收盘突破且幅度>0.2ATR"},
		{"大幅突破", 50500.0 + atrThreshold*2, true, "大幅度突破"},
	}
	
	t.Logf("=== 供给区Break测试 (ATR阈值: %.2f) ===", atrThreshold)
	for _, tc := range testCases {
		kline := Kline{High: tc.close + 50, Low: tc.close - 50, Close: tc.close}
		result := calc.IsFuzzyBreak(kline, supplyZone)
		
		if result != tc.expected {
			t.Errorf("%s: 期望%v, 实际%v - %s", tc.name, tc.expected, result, tc.reason)
		} else {
			t.Logf("✓ %s: %v - %s", tc.name, result, tc.reason)
		}
	}
}

// TestPenetrationDepth ���试穿透深度计算
func TestPenetrationDepth(t *testing.T) {
	klines := createBTCKlines()
	calc := NewContextCalculator(klines)
	
	demandZone := &SupplyDemandZone{
		Type:       DemandZone,
		UpperBound: 50400.0,  // 上边界
		LowerBound: 50200.0,  // 下边界
		Width:      200.0,    // 区域宽度
	}
	
	testCases := []struct {
		name       string
		low        float64
		expectedPct float64
		reason     string
	}{
		{"未穿透", 50450.0, 0.0, "低点在区域上方"},
		{"轻度穿透", 50350.0, 25.0, "穿透50/200=25%"},
		{"中度穿透", 50300.0, 50.0, "穿透100/200=50%"},
		{"深度穿透", 50250.0, 75.0, "穿透150/200=75%"},
		{"完全穿透", 50100.0, 100.0, "穿透超过区域宽度，限制为100%"},
	}
	
	t.Logf("=== 需求区穿透深度测试 (区域宽度: %.0f) ===", demandZone.Width)
	for _, tc := range testCases {
		kline := Kline{High: tc.low + 100, Low: tc.low, Close: tc.low + 50}
		result := calc.CalculatePenetrationDepth(kline, demandZone)
		
		if math.Abs(result-tc.expectedPct) > 1.0 {
			t.Errorf("%s: 期望%.1f%%, 实际%.1f%% - %s", tc.name, tc.expectedPct, result, tc.reason)
		} else {
			t.Logf("✓ %s: %.1f%% - %s", tc.name, result, tc.reason)
		}
	}
}

// TestZoneStatusUpdate 测试区域状态智能更新
func TestZoneStatusUpdate(t *testing.T) {
	klines := createBTCKlines()
	calc := NewContextCalculator(klines)
	processor := NewFuzzyLogicProcessor(calc)
	
	zone := &SupplyDemandZone{
		ID:         "test_status_update",
		Type:       SupplyZone,
		UpperBound: 50500.0,
		LowerBound: 50300.0,
		Width:      200.0,
		Status:     StatusFresh,
		IsActive:   true,
	}
	
	t.Logf("=== 区域状态智能更新测试 ===")
	t.Logf("初始状态: %s", zone.Status)
	
	// 测试1: 首次轻度触及
	kline1 := Kline{High: 50320.0, Low: 50280.0, Close: 50300.0}
	processor.UpdateZoneStatus(zone, kline1)
	if zone.Status != StatusTested {
		t.Errorf("首次触及后应该是TESTED，实际: %s", zone.Status)
	} else {
		t.Logf("✓ 首次触及 → %s (触及次数: %d)", zone.Status, zone.TouchCount)
	}
	
	// 测试2: 多次触及，但未达到弱化条件
	for i := 0; i < 2; i++ {
		kline := Kline{High: 50350.0, Low: 50300.0, Close: 50325.0}
		processor.UpdateZoneStatus(zone, kline)
	}
	if zone.Status != StatusTested {
		t.Errorf("正常触及应该保持TESTED，实际: %s", zone.Status)
	} else {
		t.Logf("✓ 多次触及 → %s (触及次数: %d)", zone.Status, zone.TouchCount)
	}
	
	// 测试3: 触及次数超过阈值，应该弱化
	kline3 := Kline{High: 50380.0, Low: 50320.0, Close: 50350.0}
	processor.UpdateZoneStatus(zone, kline3)
	if zone.Status != StatusWeakened {
		t.Errorf("触及次数>3应该弱化，实际: %s (次数: %d)", zone.Status, zone.TouchCount)
	} else {
		t.Logf("✓ 过度触及 → %s (触及次数: %d)", zone.Status, zone.TouchCount)
	}
	
	// 测试4: 有效突破
	atrThreshold := 0.2 * calc.market.ATR14
	breakClose := zone.UpperBound + atrThreshold + 10
	kline4 := Kline{High: breakClose + 50, Low: breakClose - 50, Close: breakClose}
	processor.UpdateZoneStatus(zone, kline4)
	if zone.Status != StatusBroken || !zone.IsBroken {
		t.Errorf("有效突破应该标记为BROKEN，实际: %s (IsBroken: %v)", zone.Status, zone.IsBroken)
	} else {
		t.Logf("✓ 有效突破 → %s (突破幅度: %.2f > %.2f)", zone.Status, breakClose-zone.UpperBound, atrThreshold)
	}
}

// TestDeepPenetrationWeakening 测试深度穿透导致的弱化
func TestDeepPenetrationWeakening(t *testing.T) {
	klines := createBTCKlines()
	calc := NewContextCalculator(klines)
	processor := NewFuzzyLogicProcessor(calc)
	
	zone := &SupplyDemandZone{
		ID:         "test_deep_penetration",
		Type:       DemandZone,
		UpperBound: 50400.0,
		LowerBound: 50200.0,
		Width:      200.0,
		Status:     StatusFresh,
		IsActive:   true,
	}
	
	t.Logf("=== 深度穿透弱化测试 ===")
	
	// 深度穿透60%的K线
	deepKline := Kline{
		High:  50350.0,
		Low:   50280.0, // 穿透120/200=60%
		Close: 50320.0,
	}
	
	processor.UpdateZoneStatus(zone, deepKline)
	
	expectedPenetration := 60.0 // (50400-50280)/200*100 = 60%
	if math.Abs(zone.MaxPenetrationPct-expectedPenetration) > 5.0 {
		t.Errorf("穿透深度计算错误: 期望%.1f%%, 实际%.1f%%", expectedPenetration, zone.MaxPenetrationPct)
	}
	
	if zone.Status != StatusWeakened {
		t.Errorf("深度穿透>50%%应该弱化，实际状态: %s", zone.Status)
	} else {
		t.Logf("✓ 深度穿透%.1f%% → %s", zone.MaxPenetrationPct, zone.Status)
	}
}

// TestEdgeCasesAndRobustness 测试边界情况和鲁棒性
func TestEdgeCasesAndRobustness(t *testing.T) {
	klines := createBTCKlines()
	calc := NewContextCalculator(klines)
	processor := NewFuzzyLogicProcessor(calc)
	
	t.Logf("=== 边界情况和鲁棒性测试 ===")
	
	// 测试1: 极小区域的缓冲区
	tinyZone := &SupplyDemandZone{
		Type:   SupplyZone,
		Width:  1.0, // 极小宽度
	}
	tolerance := calc.CalculateFuzzyTolerance(tinyZone.Width)
	minExpected := calc.market.ATR14 * 0.02
	if tolerance < minExpected {
		t.Errorf("极小区域缓冲区过小: %.6f < %.6f", tolerance, minExpected)
	} else {
		t.Logf("✓ 极小区域缓冲区: %.6f >= %.6f (最小保证)", tolerance, minExpected)
	}
	
	// 测试2: 空指针安全性
	var nilZone *SupplyDemandZone
	zones := []*SupplyDemandZone{nilZone, tinyZone}
	kline := Kline{High: 50000, Low: 49900, Close: 49950}
	
	// 这不应该崩溃
	processor.ProcessKlineUpdate(zones, kline)
	cleanZones := processor.GetCleanZonesForAI(zones)
	
	if len(cleanZones) != 1 {
		t.Errorf("空指针过滤失败: 期望1个区域，实际%d个", len(cleanZones))
	} else {
		t.Logf("✓ 空指针安全处理: %d个有效区域", len(cleanZones))
	}
	
	// 测试3: ATR为0的极端情况
	zeroATRKlines := []Kline{
		{High: 50000, Low: 50000, Close: 50000}, // 零波动
		{High: 50000, Low: 50000, Close: 50000},
		{High: 50000, Low: 50000, Close: 50000},
	}
	zeroCalc := NewContextCalculator(zeroATRKlines)
	
	if zeroCalc.market.ATR14 == 0 {
		t.Logf("✓ 零ATR情况被正确处理")
	}
}

// TestAICleaning 测试AI数据清理功能
func TestAICleaning(t *testing.T) {
	klines := createBTCKlines()
	calc := NewContextCalculator(klines)
	processor := NewFuzzyLogicProcessor(calc)
	
	// 创建各种状态的区域
	zones := []*SupplyDemandZone{
		{ID: "fresh_zone", Status: StatusFresh, TouchCount: 0},
		{ID: "tested_zone", Status: StatusTested, TouchCount: 1},
		{ID: "weakened_zone", Status: StatusWeakened, TouchCount: 4},
		{ID: "broken_zone", Status: StatusBroken, IsBroken: true},
		{ID: "over_tested_zone", Status: StatusWeakened, TouchCount: 6}, // 过度测试
	}
	
	cleanZones := processor.GetCleanZonesForAI(zones)
	
	t.Logf("=== AI数据清理测试 ===")
	t.Logf("原始区域数: %d", len(zones))
	t.Logf("清理后区域数: %d", len(cleanZones))
	
	// 验证broken和过度测试的区域被过滤掉
	for _, zone := range cleanZones {
		if zone.Status == StatusBroken {
			t.Errorf("破坏的区域不应该传给AI: %s", zone.ID)
		}
		if zone.Status == StatusWeakened && zone.TouchCount > 5 {
			t.Errorf("过度测试的区域不应该传给AI: %s", zone.ID)
		}
	}
	
	expectedCount := 3 // fresh, tested, weakened(触及4次)
	if len(cleanZones) != expectedCount {
		t.Errorf("清理后区域数量不符: 期望%d, 实际%d", expectedCount, len(cleanZones))
	} else {
		t.Logf("✓ AI数据清理正确")
	}
}


// createBTCKlines 创建BTC测试K线数据
func createBTCKlines() []Kline {
	klines := make([]Kline, 100)
	basePrice := 51000.0
	baseTime := time.Now().UnixMilli() - int64(100*5*60*1000) // 100个5分钟K线
	
	for i := range klines {
		// 生成有一定波动的价格数据
		priceVariation := float64(i%20-10) * 50 // ±500的波动
		trend := float64(i) * 2                  // 小幅上升趋势
		price := basePrice + trend + priceVariation
		
		volume := 1000.0 + float64(i%30)*50 // 1000-2450的成交量变化
		
		klines[i] = Kline{
			OpenTime:  baseTime + int64(i*5*60*1000), // 5分钟间隔
			Open:      price - 5,
			High:      price + 20 + float64(i%5)*10,
			Low:       price - 20 - float64(i%3)*5,
			Close:     price,
			Volume:    volume,
			CloseTime: baseTime + int64((i+1)*5*60*1000) - 1,
		}
	}
	
	return klines
}
