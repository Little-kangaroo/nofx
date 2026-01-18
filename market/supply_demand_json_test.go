package market

import (
	"encoding/json"
	"testing"
)

// TestSupplyDemandData_JSONSlicesNotNull 测试供需区数据序列化时slice不为null
func TestSupplyDemandData_JSONSlicesNotNull(t *testing.T) {
	tests := []struct {
		name string
		data *SupplyDemandData
	}{
		{
			name: "完全为空的数据",
			data: &SupplyDemandData{},
		},
		{
			name: "使用InitializeSupplyDemandData初始化",
			data: InitializeSupplyDemandData(nil),
		},
		{
			name: "有Config但无zones",
			data: &SupplyDemandData{
				Config:     &SDConfig{},
				Statistics: &SDStatistics{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 确保数据经过初始化
			normalized := InitializeSupplyDemandData(tt.data)

			// 序列化为JSON
			jsonBytes, err := json.Marshal(normalized)
			if err != nil {
				t.Fatalf("JSON序列化失败: %v", err)
			}

			jsonStr := string(jsonBytes)

			// 验证关键slice字段不为null
			checkSliceNotNull(t, jsonStr, "supply_zones")
			checkSliceNotNull(t, jsonStr, "demand_zones")
			checkSliceNotNull(t, jsonStr, "active_zones")

			t.Logf("JSON输出: %s", jsonStr)
		})
	}
}

// checkSliceNotNull 检查JSON字符串中指定字段不为null
func checkSliceNotNull(t *testing.T, jsonStr, fieldName string) {
	// 检查字段存在且值为[]而不是null
	expectedPattern := `"` + fieldName + `":[]`
	nullPattern := `"` + fieldName + `":null`

	if stringContains(jsonStr, nullPattern) {
		t.Errorf("字段 %s 的值为null，应该为[]", fieldName)
	}

	if !stringContains(jsonStr, expectedPattern) {
		// 可能有元素，检查是否为数组格式
		arrayStartPattern := `"` + fieldName + `":[`
		if !stringContains(jsonStr, arrayStartPattern) {
			t.Errorf("字段 %s 未找到数组格式", fieldName)
		}
	}
}

// stringContains 简单的字符串包含检查
func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstringMatch(s, substr)
}

func findSubstringMatch(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestSupplyDemandAnalyzer_EmptyKlines 测试空K线输入不返回null
func TestSupplyDemandAnalyzer_EmptyKlines(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 少于10根K线
	klines := []Kline{
		{OpenTime: 1000, Close: 100.0, High: 101.0, Low: 99.0, Volume: 1000},
		{OpenTime: 2000, Close: 101.0, High: 102.0, Low: 100.0, Volume: 1100},
	}

	result := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "5m")

	// 序列化检查
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON序列化失败: %v", err)
	}

	jsonStr := string(jsonBytes)

	// 验证所有slice不为null
	checkSliceNotNull(t, jsonStr, "supply_zones")
	checkSliceNotNull(t, jsonStr, "demand_zones")
	checkSliceNotNull(t, jsonStr, "active_zones")

	t.Logf("空K线结果JSON: %s", jsonStr)
}
