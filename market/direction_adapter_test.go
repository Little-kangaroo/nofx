package market

import (
	"testing"
)

// TestParseChannelWithChannelWidthPct 测试通道质量字段映射修复
func TestParseChannelWithChannelWidthPct(t *testing.T) {
	// 模拟实际的时间框架数据（包含"通道数据"字段）
	testCases := []struct {
		name           string
		tfData         map[string]interface{}
		expectedQuality float64
		shouldBeValid  bool
		expectedDir    string
	}{
		{
			name: "正常通道数据-5%宽度",
			tfData: map[string]interface{}{
				"通道数据": map[string]interface{}{
					"channel_direction": "up",
					"current_position":  "Inside",
					"channel_width_pct": 5.0, // 5%宽度
				},
			},
			expectedQuality: 0.5, // 5/10 = 0.5
			shouldBeValid:   true,
			expectedDir:     "up",
		},
		{
			name: "正常通道数据-10%宽度",
			tfData: map[string]interface{}{
				"通道数据": map[string]interface{}{
					"channel_direction": "down",
					"current_position":  "Inside",
					"channel_width_pct": 10.0, // 10%宽度
				},
			},
			expectedQuality: 1.0, // 10/10 = 1.0 (上限)
			shouldBeValid:   true,
			expectedDir:     "down",
		},
		{
			name: "窄通道-0.5%宽度",
			tfData: map[string]interface{}{
				"通道数据": map[string]interface{}{
					"channel_direction": "sideways",
					"current_position":  "Inside",
					"channel_width_pct": 0.5, // 0.5%宽度
				},
			},
			expectedQuality: 0.1, // 最小阈值
			shouldBeValid:   true,
			expectedDir:     "sideways",
		},
		{
			name: "无效通道-0%宽度",
			tfData: map[string]interface{}{
				"通道数据": map[string]interface{}{
					"channel_direction": "sideways",
					"current_position":  "Inside",
					"channel_width_pct": 0.0,
				},
			},
			expectedQuality: 0.0,
			shouldBeValid:   false,
			expectedDir:     "sideways",
		},
		{
			name: "宽通道-20%宽度",
			tfData: map[string]interface{}{
				"通道数据": map[string]interface{}{
					"channel_direction": "up",
					"current_position":  "BreakUp",
					"channel_width_pct": 20.0, // 20%宽度
				},
			},
			expectedQuality: 1.0, // 上限为1.0
			shouldBeValid:   true,
			expectedDir:     "up",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseChannel(tc.tfData)

			// 检查质量值
			if result.Quality != tc.expectedQuality {
				t.Errorf("Quality不匹配: got %.2f, want %.2f", result.Quality, tc.expectedQuality)
			}

			// 检查是否有效（Quality > 0）
			isValid := result.Quality > 0
			if isValid != tc.shouldBeValid {
				t.Errorf("有效性不匹配: got %v, want %v (Quality=%.2f)", isValid, tc.shouldBeValid, result.Quality)
			}

			// 检查其他字段
			if result.Direction != tc.expectedDir {
				t.Errorf("Direction不匹配: got %s, want %s", result.Direction, tc.expectedDir)
			}
		})
	}
}

// TestParseMTFDataWithMultipleTimeframes 测试多时间框架解析
func TestParseMTFDataWithMultipleTimeframes(t *testing.T) {
	// 模拟完整的多时间框架数据
	mtfData := map[string]interface{}{
		"15m": map[string]interface{}{
			"通道数据": map[string]interface{}{
				"channel_direction": "up",
				"current_position":  "Inside",
				"channel_width_pct": 5.0,
			},
			"VPVR数据": map[string]interface{}{
				"dist_to_vah_atr": 1.5,
				"dist_to_val_atr": -2.0,
			},
		},
		"30m": map[string]interface{}{
			"通道数据": map[string]interface{}{
				"channel_direction": "up",
				"current_position":  "Inside",
				"channel_width_pct": 8.0,
			},
			"VPVR数据": map[string]interface{}{
				"dist_to_vah_atr": 1.0,
				"dist_to_val_atr": -1.5,
			},
		},
		"1h": map[string]interface{}{
			"通道数据": map[string]interface{}{
				"channel_direction": "up",
				"current_position":  "Inside",
				"channel_width_pct": 10.0,
			},
			"VPVR数据": map[string]interface{}{
				"dist_to_vah_atr": 0.5,
				"dist_to_val_atr": -1.0,
			},
		},
		"4h": map[string]interface{}{
			"通道数据": map[string]interface{}{
				"channel_direction": "up",
				"current_position":  "Inside",
				"channel_width_pct": 12.0,
			},
			"VPVR数据": map[string]interface{}{
				"dist_to_vah_atr": 0.3,
				"dist_to_val_atr": -0.8,
			},
		},
	}

	result := parseMTFData(mtfData)

	// 检查是否解析了所有关键时间框架
	requiredTimeframes := []string{"15m", "30m", "1h", "4h"}
	for _, tf := range requiredTimeframes {
		if _, exists := result[tf]; !exists {
			t.Errorf("缺少时间框架: %s", tf)
		}
	}

	// 检查15m的通道质量
	if tf15m, ok := result["15m"]; ok {
		if tf15m.Channel.Quality <= 0 {
			t.Errorf("15m通道质量无效: %.2f", tf15m.Channel.Quality)
		}
		if tf15m.Channel.Direction != "up" {
			t.Errorf("15m通道方向错误: %s", tf15m.Channel.Direction)
		}
	}

	// 检查1h的通道质量（关键修复点）
	if tf1h, ok := result["1h"]; ok {
		if tf1h.Channel.Quality <= 0 {
			t.Errorf("1h通道质量无效: %.2f", tf1h.Channel.Quality)
		}
		t.Logf("✅ 1h通道数据正确解析: Direction=%s, Quality=%.2f", tf1h.Channel.Direction, tf1h.Channel.Quality)
	} else {
		t.Error("❌ 缺少1h时间框架数据")
	}
}
