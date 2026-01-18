package market

import (
	"encoding/json"
	"testing"
)

// TestSupplyDemand_P0Fixes_Integration 集成测试：验证P0-04和P0-01修复
func TestSupplyDemand_P0Fixes_Integration(t *testing.T) {
	// 模拟真实场景：多时间框架分析
	timeframes := []string{"5m", "15m", "30m", "1h", "4h"}

	for _, tf := range timeframes {
		t.Run(tf+"_完整流程", func(t *testing.T) {
			// 1. 生成测试K线数据（足够触发区域识别）
			klines := generateRealisticKlines(100, tf)

			// 2. 执行供需区分析
			analyzer := NewSupplyDemandAnalyzer()
			sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", tf)

			// 3. 验证P0-01: Timeframe传递
			if sdData.Timeframe != tf {
				t.Errorf("[P0-01失败] sdData.Timeframe = %s, 期望 %s", sdData.Timeframe, tf)
			}

			// 4. 验证P0-04: slice不为null
			jsonBytes, err := json.Marshal(sdData)
			if err != nil {
				t.Fatalf("JSON序列化失败: %v", err)
			}
			jsonStr := string(jsonBytes)

			if stringContains(jsonStr, `"supply_zones":null`) {
				t.Error("[P0-04失败] supply_zones 为 null")
			}
			if stringContains(jsonStr, `"demand_zones":null`) {
				t.Error("[P0-04失败] demand_zones 为 null")
			}
			if stringContains(jsonStr, `"active_zones":null`) {
				t.Error("[P0-04失败] active_zones 为 null")
			}

			// 5. 验证timeframe字段存在
			if !stringContains(jsonStr, `"timeframe":"`+tf+`"`) {
				t.Errorf("[P0-01失败] JSON缺少timeframe字段或值不正确: %s", tf)
			}

			// 6. 验证所有zone的Origin.TimeFrame
			allZones := append(sdData.SupplyZones, sdData.DemandZones...)
			allZones = append(allZones, sdData.ActiveZones...)

			for _, zone := range allZones {
				if zone.Origin == nil {
					t.Error("[P0-01失败] zone.Origin 为 nil")
					continue
				}
				if zone.Origin.TimeFrame != tf {
					t.Errorf("[P0-01失败] zone.Origin.TimeFrame = %s, 期望 %s",
						zone.Origin.TimeFrame, tf)
				}
			}

			t.Logf("✅ [%s] P0修复验证通过 - zones: %d供给/%d需求/%d活跃",
				tf, len(sdData.SupplyZones), len(sdData.DemandZones), len(sdData.ActiveZones))
		})
	}
}

// TestSupplyDemand_AnchorEngine_Integration 测试供需区到锚点的完整流程
func TestSupplyDemand_AnchorEngine_Integration(t *testing.T) {
	testCases := []struct {
		timeframe    string
		expectedType AnchorType
	}{
		{"4h", AnchorHTFZone},
		{"1h", AnchorHTFZone},
		{"30m", AnchorZoneMTF},
		{"15m", AnchorZoneMTF},
	}

	for _, tc := range testCases {
		t.Run(tc.timeframe+"_锚点生成", func(t *testing.T) {
			// 1. 生成供需区数据
			klines := generateRealisticKlines(80, tc.timeframe)
			analyzer := NewSupplyDemandAnalyzer()
			sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", tc.timeframe)

			// 确保有活跃区域
			if len(sdData.ActiveZones) == 0 {
				t.Skip("没有活跃区域，跳过测试")
			}

			// 2. 转换为锚点候选
			engine := NewAnchorEngine(nil)
			candidates := engine.fromSupplyDemand(sdData, nil)

			if len(candidates) == 0 {
				t.Fatal("应该生成锚点候选")
			}

			// 3. 验证所有候选的TF和Type
			for i, candidate := range candidates {
				if candidate.TF != tc.timeframe {
					t.Errorf("候选[%d].TF = %s, 期望 %s", i, candidate.TF, tc.timeframe)
				}
				if candidate.Type != tc.expectedType {
					t.Errorf("候选[%d].Type = %s, 期望 %s (timeframe=%s)",
						i, candidate.Type, tc.expectedType, tc.timeframe)
				}
			}

			t.Logf("✅ [%s] 生成 %d 个锚点候选，类型=%s",
				tc.timeframe, len(candidates), tc.expectedType)
		})
	}
}

// TestSupplyDemand_EmptyScenarios 测试各种边界场景
func TestSupplyDemand_EmptyScenarios(t *testing.T) {
	scenarios := []struct {
		name     string
		klines   []Kline
		symbol   string
		timeframe string
	}{
		{
			name:      "空K线数组",
			klines:    []Kline{},
			symbol:    "BTCUSDT",
			timeframe: "5m",
		},
		{
			name:      "单根K线",
			klines:    generateRealisticKlines(1, "5m"),
			symbol:    "ETHUSDT",
			timeframe: "15m",
		},
		{
			name:      "少于10根K线",
			klines:    generateRealisticKlines(5, "1h"),
			symbol:    "BNBUSDT",
			timeframe: "1h",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			analyzer := NewSupplyDemandAnalyzer()
			sdData := analyzer.AnalyzeWithSymbol(scenario.klines, scenario.symbol, scenario.timeframe)

			// 验证不会panic
			if sdData == nil {
				t.Fatal("sdData 不应该为 nil")
			}

			// 验证P0-01: 即使空数据也有timeframe
			if sdData.Timeframe != scenario.timeframe {
				t.Errorf("Timeframe = %s, 期望 %s", sdData.Timeframe, scenario.timeframe)
			}

			// 验证P0-04: slice不为null
			if sdData.SupplyZones == nil {
				t.Error("SupplyZones 为 nil")
			}
			if sdData.DemandZones == nil {
				t.Error("DemandZones 为 nil")
			}
			if sdData.ActiveZones == nil {
				t.Error("ActiveZones 为 nil")
			}

			// JSON序列化测试
			jsonBytes, err := json.Marshal(sdData)
			if err != nil {
				t.Fatalf("JSON序列化失败: %v", err)
			}

			jsonStr := string(jsonBytes)
			if stringContains(jsonStr, `null`) && !stringContains(jsonStr, `"config":null`) {
				// 允许config为null，但其他字段不应该
				t.Logf("警告: JSON包含null: %s", jsonStr)
			}

			t.Logf("✅ [%s] 边界场景通过", scenario.name)
		})
	}
}

// TestSupplyDemand_DataCleaner_P0Fixes 测试数据清洗器是否保持P0修复
func TestSupplyDemand_DataCleaner_P0Fixes(t *testing.T) {
	// 1. 生成供需区数据
	klines := generateRealisticKlines(100, "30m")
	analyzer := NewSupplyDemandAnalyzer()
	sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "30m")

	// 2. 应用数据清洗
	cleaner := NewDataCleaner()
	cleanedData, stats := cleaner.CleanSupplyDemandData(sdData)

	// 3. 验证P0-01: Timeframe保持不变
	if cleanedData.Timeframe != "30m" {
		t.Errorf("清洗后Timeframe改变: %s -> %s", sdData.Timeframe, cleanedData.Timeframe)
	}

	// 4. 验证P0-04: slice仍然不为null
	if cleanedData.SupplyZones == nil {
		t.Error("清洗后SupplyZones为nil")
	}
	if cleanedData.DemandZones == nil {
		t.Error("清洗后DemandZones为nil")
	}
	if cleanedData.ActiveZones == nil {
		t.Error("清洗后ActiveZones为nil")
	}

	// 5. 验证所有保留zone的TimeFrame
	for _, zone := range cleanedData.ActiveZones {
		if zone.Origin != nil && zone.Origin.TimeFrame != "30m" {
			t.Errorf("清洗后zone.Origin.TimeFrame = %s, 期望 30m", zone.Origin.TimeFrame)
		}
	}

	t.Logf("✅ 数据清洗保持P0修复 - 清洗率: %.1f%%, 质量: %.1f",
		stats.FilterRate, stats.QualityScore)
}

// TestSupplyDemand_MultiSymbol 测试多币种场景
func TestSupplyDemand_MultiSymbol(t *testing.T) {
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT"}
	timeframe := "15m"

	for _, symbol := range symbols {
		t.Run(symbol, func(t *testing.T) {
			klines := generateRealisticKlines(60, timeframe)
			analyzer := NewSupplyDemandAnalyzer()
			sdData := analyzer.AnalyzeWithSymbol(klines, symbol, timeframe)

			// 验证基本要求
			if sdData.Timeframe != timeframe {
				t.Errorf("[%s] Timeframe错误: %s", symbol, sdData.Timeframe)
			}

			if sdData.SupplyZones == nil || sdData.DemandZones == nil || sdData.ActiveZones == nil {
				t.Errorf("[%s] slice为nil", symbol)
			}

			// JSON验证
			jsonBytes, _ := json.Marshal(sdData)
			jsonStr := string(jsonBytes)

			if stringContains(jsonStr, `"supply_zones":null`) {
				t.Errorf("[%s] supply_zones为null", symbol)
			}

			t.Logf("✅ [%s] 验证通过 - zones: %d/%d/%d",
				symbol, len(sdData.SupplyZones), len(sdData.DemandZones), len(sdData.ActiveZones))
		})
	}
}

// generateRealisticKlines 生成更真实的K线数据（带趋势和波动）
func generateRealisticKlines(count int, timeframe string) []Kline {
	klines := make([]Kline, count)
	basePrice := 44000.0
	baseTime := int64(1700000000000)

	// 根据timeframe计算间隔
	intervalMs := getIntervalMs(timeframe)

	// 生成趋势性价格
	trend := 1.0 // 上涨趋势
	if count%2 == 0 {
		trend = -1.0 // 下跌趋势
	}

	for i := 0; i < count; i++ {
		// 添加趋势和随机波动
		trendMove := float64(i) * trend * 5.0
		randomMove := float64((i*7)%20 - 10) * 3.0
		price := basePrice + trendMove + randomMove

		// 确保价格合理
		if price < 1000 {
			price = 1000
		}

		high := price + float64(i%15+5)*2.0
		low := price - float64(i%12+3)*2.0

		klines[i] = Kline{
			OpenTime:  baseTime + int64(i)*intervalMs,
			CloseTime: baseTime + int64(i)*intervalMs + intervalMs - 1,
			Open:      price,
			High:      high,
			Low:       low,
			Close:     price + float64((i%7)-3)*3.0,
			Volume:    800.0 + float64(i%20)*100.0,
			QuoteVolume: (price * (800.0 + float64(i%20)*100.0)),
		}
	}

	return klines
}

// getIntervalMs 根据timeframe获取毫秒间隔
func getIntervalMs(timeframe string) int64 {
	switch timeframe {
	case "1m":
		return 60 * 1000
	case "5m":
		return 5 * 60 * 1000
	case "15m":
		return 15 * 60 * 1000
	case "30m":
		return 30 * 60 * 1000
	case "1h":
		return 60 * 60 * 1000
	case "4h":
		return 4 * 60 * 60 * 1000
	default:
		return 5 * 60 * 1000
	}
}
