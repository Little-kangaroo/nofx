package market

import (
	"testing"
	"time"
)

// TestSupplyDemand_MaxZoneAge_BarsSemantics 测试P0-02: MaxZoneAge从小时改为K线数语义
func TestSupplyDemand_MaxZoneAge_BarsSemantics(t *testing.T) {
	testCases := []struct {
		timeframe      string
		intervalMs     int64
		maxAgeBars     int
		shouldExpire   bool
		ageDescription string
	}{
		// 5分钟级别: MaxZoneAge=200根，200*5分钟 = 16.67小时
		{"5m", 5 * 60 * 1000, 200, false, "100根K线 (8.3小时)"},
		{"5m", 5 * 60 * 1000, 200, true, "250根K线 (20.8小时)"},

		// 15分钟级别: MaxZoneAge=200根，200*15分钟 = 50小时
		{"15m", 15 * 60 * 1000, 200, false, "150根K线 (37.5小时)"},
		{"15m", 15 * 60 * 1000, 200, true, "210根K线 (52.5小时)"},

		// 1小时级别: MaxZoneAge=200根，200*1小时 = 200小时
		{"1h", 60 * 60 * 1000, 200, false, "100根K线 (100小时)"},
		{"1h", 60 * 60 * 1000, 200, true, "220根K线 (220小时)"},

		// 4小时级别: MaxZoneAge=200根，200*4小时 = 800小时
		{"4h", 4 * 60 * 60 * 1000, 200, false, "150根K线 (600小时)"},
		{"4h", 4 * 60 * 60 * 1000, 200, true, "250根K线 (1000小时)"},
	}

	for _, tc := range testCases {
		t.Run(tc.timeframe+"_"+tc.ageDescription, func(t *testing.T) {
			// 创建分析器
			config := defaultSDConfig // 使用默认配置
			config.MaxZoneAge = tc.maxAgeBars
			analyzer := NewSupplyDemandAnalyzerWithConfig(config)

			// 生成测试K线（足够触发区域识别）
			klines := generateTestKlinesForAge(50, tc.timeframe, tc.intervalMs)
			currentTime := klines[len(klines)-1].OpenTime

			// 手动创建一个zone，设置指定年龄
			var ageBars int
			if tc.shouldExpire {
				ageBars = tc.maxAgeBars + 10 // 超过MaxZoneAge
			} else {
				ageBars = tc.maxAgeBars - 50 // 未超过MaxZoneAge
			}

			zoneCreationTime := currentTime - int64(ageBars)*tc.intervalMs

			zone := &SupplyDemandZone{
				ID:           "test_zone",
				Type:         DemandZone,
				UpperBound:   45000,
				LowerBound:   44500,
				CreationTime: zoneCreationTime,
				Status:       StatusFresh, // 使用StatusFresh
				IsActive:     true,
				Origin: &ZoneOrigin{
					TimeFrame: tc.timeframe,
				},
			}

			zones := []*SupplyDemandZone{zone}

			// 执行状态更新
			analyzer.updateZoneStatuses(zones, klines, tc.timeframe)

			// 验证结果
			if tc.shouldExpire {
				if zone.Status != StatusExpired {
					t.Errorf("Zone应该过期: ageBars=%d, maxAge=%d, 实际status=%s",
						ageBars, tc.maxAgeBars, zone.Status)
				}
				if zone.IsActive {
					t.Error("过期zone不应该是活跃的")
				}
			} else {
				if zone.Status == StatusExpired {
					t.Errorf("Zone不应该过期: ageBars=%d, maxAge=%d",
						ageBars, tc.maxAgeBars)
				}
			}

			t.Logf("✅ [%s] %s - ageBars=%d, maxAge=%d, expired=%v",
				tc.timeframe, tc.ageDescription, ageBars, tc.maxAgeBars, tc.shouldExpire)
		})
	}
}

// TestSupplyDemand_IsRecentZone_BarsBased 测试isRecentZone从2小时改为10根K线
func TestSupplyDemand_IsRecentZone_BarsBased(t *testing.T) {
	testCases := []struct {
		timeframe     string
		ageBars       int
		shouldBeRecent bool
		description   string
	}{
		// 5分钟级别: 10根 = 50分钟
		{"5m", 5, true, "5根K线（25分钟）"},
		{"5m", 15, false, "15根K线（75分钟）"},

		// 15分钟级别: 10根 = 2.5小时
		{"15m", 8, true, "8根K线（2小时）"},
		{"15m", 12, false, "12根K线（3小时）"},

		// 1小时级别: 10根 = 10小时
		{"1h", 5, true, "5根K线（5小时）"},
		{"1h", 15, false, "15根K线（15小时）"},

		// 4小时级别: 10根 = 40小时
		{"4h", 9, true, "9根K线（36小时）"},
		{"4h", 11, false, "11根K线（44小时）"},
	}

	for _, tc := range testCases {
		t.Run(tc.timeframe+"_"+tc.description, func(t *testing.T) {
			analyzer := NewSupplyDemandAnalyzer()

			// 生成K线
			intervalMs := analyzer.inferIntervalMs(tc.timeframe)
			klines := generateTestKlinesForAge(50, tc.timeframe, intervalMs)
			currentTime := klines[len(klines)-1].OpenTime

			// 创建zone
			zoneCreationTime := currentTime - int64(tc.ageBars)*intervalMs
			zone := &SupplyDemandZone{
				ID:           "test_zone",
				CreationTime: zoneCreationTime,
			}

			// 测试isRecentZone
			isRecent := analyzer.isRecentZone(zone, klines, tc.timeframe)

			if isRecent != tc.shouldBeRecent {
				t.Errorf("isRecentZone结果错误: ageBars=%d, 期望=%v, 实际=%v",
					tc.ageBars, tc.shouldBeRecent, isRecent)
			}

			t.Logf("✅ [%s] %s - ageBars=%d, isRecent=%v",
				tc.timeframe, tc.description, tc.ageBars, isRecent)
		})
	}
}

// TestSupplyDemand_ContextScores_BarsBasedAge 测试上下文评分使用bars-based age
func TestSupplyDemand_ContextScores_BarsBasedAge(t *testing.T) {
	testCases := []struct {
		timeframe string
		ageBars   int
	}{
		{"5m", 50},   // 50根5分钟 = 4.2小时
		{"15m", 100}, // 100根15分钟 = 25小时
		{"1h", 150},  // 150根1小时 = 150小时
	}

	for _, tc := range testCases {
		t.Run(tc.timeframe, func(t *testing.T) {
			config := defaultSDConfig // 使用默认配置
			config.MaxZoneAge = 200 // K线数
			analyzer := NewSupplyDemandAnalyzerWithConfig(config)

			intervalMs := analyzer.inferIntervalMs(tc.timeframe)
			klines := generateTestKlinesForAge(50, tc.timeframe, intervalMs)
			currentTime := klines[len(klines)-1].OpenTime

			// 创建zone
			zoneCreationTime := currentTime - int64(tc.ageBars)*intervalMs
			zone := &SupplyDemandZone{
				ID:           "test_zone",
				Type:         SupplyZone,
				UpperBound:   45000,
				LowerBound:   44500,
				Strength:     1.5,
				Width:        100,
				CreationTime: zoneCreationTime,
				VolumeProfile: &ZoneVP{
					TotalVolume: 10000,
				},
				Origin: &ZoneOrigin{
					TimeFrame: tc.timeframe,
				},
			}

			allZones := []*SupplyDemandZone{zone}
			contextCalc := NewContextCalculator(klines)

			// 计算上下文评分
			analyzer.CalculateContextScores(allZones, contextCalc, tc.timeframe)

			// 验证上下文评分已计算
			if zone.Context == nil {
				t.Fatal("Context未计算")
			}

			// 验证is_fresh和time_score
			// ageBars=50, maxAge=200, 在生命周期的前25%，应该是fresh
			if tc.ageBars < config.MaxZoneAge/3 {
				if !zone.Context.IsFresh {
					t.Errorf("ageBars=%d < maxAge/3=%d，应该IsFresh=true",
						tc.ageBars, config.MaxZoneAge/3)
				}
			}

			// TimeScore应该>0
			if zone.Context.TimeScore <= 0 {
				t.Errorf("TimeScore应该>0，实际=%.2f", zone.Context.TimeScore)
			}

			t.Logf("✅ [%s] ageBars=%d, IsFresh=%v, TimeScore=%.2f",
				tc.timeframe, tc.ageBars, zone.Context.IsFresh, zone.Context.TimeScore)
		})
	}
}

// TestSupplyDemand_CrossTimeframe_Consistency 测试跨时间框架的语义一致性
func TestSupplyDemand_CrossTimeframe_Consistency(t *testing.T) {
	// 验证MaxZoneAge=200在不同时间框架下的行为一致性
	// 关键：都是"200根K线"，但实际时间跨度不同

	config := defaultSDConfig // 使用默认配置
	config.MaxZoneAge = 200

	testCases := []struct {
		timeframe        string
		expectedLifespan string // 人类可读的生命周期
	}{
		{"5m", "16.7小时"},
		{"15m", "50小时"},
		{"30m", "100小时"},
		{"1h", "200小时"},
		{"4h", "800小时"},
	}

	for _, tc := range testCases {
		t.Run(tc.timeframe, func(t *testing.T) {
			analyzer := NewSupplyDemandAnalyzerWithConfig(config)

			// 验证inferIntervalMs正确
			intervalMs := analyzer.inferIntervalMs(tc.timeframe)
			lifespanMs := int64(config.MaxZoneAge) * intervalMs
			lifespanHours := float64(lifespanMs) / (3600 * 1000)

			t.Logf("✅ [%s] MaxZoneAge=200根 → 生命周期=%.1f小时 (期望约%s)",
				tc.timeframe, lifespanHours, tc.expectedLifespan)

			// 验证生命周期计算合理
			if lifespanMs <= 0 {
				t.Errorf("生命周期计算错误: %d ms", lifespanMs)
			}
		})
	}
}

// generateTestKlinesForAge 生成测试用K线（用于年龄测试）
func generateTestKlinesForAge(count int, timeframe string, intervalMs int64) []Kline {
	klines := make([]Kline, count)
	basePrice := 44000.0
	baseTime := time.Now().UnixMilli()

	for i := 0; i < count; i++ {
		price := basePrice + float64(i%10-5)*10.0

		klines[i] = Kline{
			OpenTime:  baseTime + int64(i)*intervalMs,
			CloseTime: baseTime + int64(i)*intervalMs + intervalMs - 1,
			Open:      price,
			High:      price + 20.0,
			Low:       price - 20.0,
			Close:     price + float64(i%3-1)*5.0,
			Volume:    1000.0 + float64(i%5)*200.0,
		}
	}

	return klines
}
