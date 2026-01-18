package market

import (
	"encoding/json"
	"testing"
)

// TestSupplyDemandData_TimeframePropagation 测试timeframe在供需区链路中的传递
func TestSupplyDemandData_TimeframePropagation(t *testing.T) {
	testCases := []struct {
		name      string
		timeframe string
	}{
		{"5分钟级别", "5m"},
		{"15分钟级别", "15m"},
		{"30分钟级别", "30m"},
		{"1小时级别", "1h"},
		{"4小时级别", "4h"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := NewSupplyDemandAnalyzer()

			// 构造足够的K线数据以触发区域识别
			klines := generateTestKlines(50)

			// 执行分析
			result := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", tc.timeframe)

			// 验证1: SupplyDemandData.Timeframe 必须设置
			if result.Timeframe != tc.timeframe {
				t.Errorf("SupplyDemandData.Timeframe = %s, 期望 %s", result.Timeframe, tc.timeframe)
			}

			// 验证2: 所有SupplyZones的Origin.TimeFrame必须一致
			for i, zone := range result.SupplyZones {
				if zone.Origin == nil {
					t.Errorf("SupplyZones[%d].Origin 为 nil", i)
					continue
				}
				if zone.Origin.TimeFrame != tc.timeframe {
					t.Errorf("SupplyZones[%d].Origin.TimeFrame = %s, 期望 %s",
						i, zone.Origin.TimeFrame, tc.timeframe)
				}
			}

			// 验证3: 所有DemandZones的Origin.TimeFrame必须一致
			for i, zone := range result.DemandZones {
				if zone.Origin == nil {
					t.Errorf("DemandZones[%d].Origin 为 nil", i)
					continue
				}
				if zone.Origin.TimeFrame != tc.timeframe {
					t.Errorf("DemandZones[%d].Origin.TimeFrame = %s, 期望 %s",
						i, zone.Origin.TimeFrame, tc.timeframe)
				}
			}

			// 验证4: 所有ActiveZones的Origin.TimeFrame必须一致
			for i, zone := range result.ActiveZones {
				if zone.Origin == nil {
					t.Errorf("ActiveZones[%d].Origin 为 nil", i)
					continue
				}
				if zone.Origin.TimeFrame != tc.timeframe {
					t.Errorf("ActiveZones[%d].Origin.TimeFrame = %s, 期望 %s",
						i, zone.Origin.TimeFrame, tc.timeframe)
				}
			}

			t.Logf("✅ Timeframe传递验证通过: %s (supply=%d, demand=%d, active=%d)",
				tc.timeframe, len(result.SupplyZones), len(result.DemandZones), len(result.ActiveZones))
		})
	}
}

// TestSupplyDemandData_EmptyTimeframe 测试空K线场景下timeframe也能正确设置
func TestSupplyDemandData_EmptyTimeframe(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 少于10根K线
	klines := []Kline{
		{OpenTime: 1000, Close: 100.0, High: 101.0, Low: 99.0, Volume: 1000},
	}

	result := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

	if result.Timeframe != "15m" {
		t.Errorf("即使空数据，Timeframe也应该设置: got=%s, want=15m", result.Timeframe)
	}

	t.Logf("✅ 空数据场景timeframe验证通过: %s", result.Timeframe)
}

// TestAnchorEngine_TimeframeFromSupplyDemand 测试AnchorEngine能正确读取timeframe
func TestAnchorEngine_TimeframeFromSupplyDemand(t *testing.T) {
	// 传nil使用默认配置
	engine := NewAnchorEngine(nil)

	// 构造带有timeframe的供需区数据
	sdData := &SupplyDemandData{
		Timeframe: "30m",
		ActiveZones: []*SupplyDemandZone{
			{
				ID:         "test_zone_1",
				Type:       SupplyZone,
				UpperBound: 45000,
				LowerBound: 44500,
				Origin: &ZoneOrigin{
					TimeFrame: "30m",
				},
				Context: &ContextMetrics{
					StrengthZ: 1.5,
					IsFresh:   true,
				},
				StrengthZReady: true,
			},
		},
	}

	// 转换为锚点候选
	candidates := engine.fromSupplyDemand(sdData, nil)

	if len(candidates) == 0 {
		t.Fatal("应该生成至少1个候选锚点")
	}

	// 验证TF字段
	if candidates[0].TF != "30m" {
		t.Errorf("AnchorCandidate.TF = %s, 期望 30m", candidates[0].TF)
	}

	// 验证Type (30m应该是MTF，不是HTF)
	if candidates[0].Type != AnchorZoneMTF {
		t.Errorf("AnchorCandidate.Type = %s, 期望 %s (30m应该是MTF)",
			candidates[0].Type, AnchorZoneMTF)
	}

	t.Logf("✅ AnchorEngine timeframe读取验证通过: TF=%s, Type=%s",
		candidates[0].TF, candidates[0].Type)
}

// TestAnchorEngine_HTFClassification 测试HTF/MTF分类逻辑
func TestAnchorEngine_HTFClassification(t *testing.T) {
	testCases := []struct {
		timeframe    string
		expectedType AnchorType
	}{
		{"4h", AnchorHTFZone},
		{"1h", AnchorHTFZone},
		{"30m", AnchorZoneMTF},
		{"15m", AnchorZoneMTF},
		{"5m", AnchorZoneMTF},
	}

	engine := NewAnchorEngine(nil)

	for _, tc := range testCases {
		t.Run(tc.timeframe, func(t *testing.T) {
			sdData := &SupplyDemandData{
				Timeframe: tc.timeframe,
				ActiveZones: []*SupplyDemandZone{
					{
						ID:         "test_zone",
						Type:       DemandZone,
						UpperBound: 45000,
						LowerBound: 44500,
						Origin: &ZoneOrigin{
							TimeFrame: tc.timeframe,
						},
					},
				},
			}

			candidates := engine.fromSupplyDemand(sdData, nil)

			if len(candidates) == 0 {
				t.Fatal("应该生成候选锚点")
			}

			if candidates[0].Type != tc.expectedType {
				t.Errorf("Timeframe %s: Type = %s, 期望 %s",
					tc.timeframe, candidates[0].Type, tc.expectedType)
			}

			t.Logf("✅ %s 分类正确: %s", tc.timeframe, tc.expectedType)
		})
	}
}

// TestSupplyDemandData_JSONTimeframe 测试JSON序列化包含timeframe
func TestSupplyDemandData_JSONTimeframe(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()
	klines := generateTestKlines(50)

	result := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "1h")

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON序列化失败: %v", err)
	}

	jsonStr := string(jsonBytes)

	// 验证JSON包含timeframe字段
	if !stringContains(jsonStr, `"timeframe":"1h"`) {
		t.Errorf("JSON输出缺少timeframe字段")
	}

	t.Logf("✅ JSON序列化包含timeframe: %s", jsonStr[:200])
}

// generateTestKlines 生成测试用K线数据
func generateTestKlines(count int) []Kline {
	klines := make([]Kline, count)
	basePrice := 44000.0
	baseTime := int64(1700000000000)

	for i := 0; i < count; i++ {
		// 生成随机价格波动
		priceChange := float64(i%10-5) * 10.0
		price := basePrice + priceChange

		klines[i] = Kline{
			OpenTime:  baseTime + int64(i*300000), // 5分钟间隔
			CloseTime: baseTime + int64(i*300000) + 299999,
			Open:      price,
			High:      price + 20.0,
			Low:       price - 20.0,
			Close:     price + float64(i%3-1)*5.0,
			Volume:    1000.0 + float64(i%5)*200.0,
		}
	}

	return klines
}
