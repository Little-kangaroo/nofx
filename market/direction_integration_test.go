package market

import (
	"testing"
)

// TestDirectionArbitrationIntegration 端到端集成测试
// 验证从市场数据到方向裁决的完整流程
func TestDirectionArbitrationIntegration(t *testing.T) {
	// 模拟完整的订单流数据
	orderflowData := map[string]interface{}{
		"数据质量": map[string]interface{}{
			"status":                  "正常",
			"overall_score":           0.85,
			"data_lag_ms":             100.0,
			"update_interval_s":       5.0,
			"cvd_reliability":         0.90,
			"oi_reliability":          0.85,
			"orderbook_reliability":   0.88,
		},
		"宏观资金趋势": map[string]interface{}{
			"trend_alignment":  "bullish",
			"signal_strength":  75.0,
			"confidence_level": 0.80,
			"market_regime":    "trending",
			"cvd_divergence":   false,
		},
		"本周期博弈_5m": map[string]interface{}{
			"candle_intent":         "strong_buy",
			"futures_cvd_delta_usd": 1500000.0,
			"spot_cvd_delta_usd":    800000.0,
			"oi_delta_pct":          2.5,
			"price_delta_pct":       1.2,
			"volume_delta":          5000000.0,
			"data_quality":          0.90,
		},
		"盘口结构_v2": map[string]interface{}{
			"imbalance_ratio":       1.5,
			"bid_pressure":          0.65,
			"ask_pressure":          0.35,
			"liquidity_score":       0.75,
			"spoofing_risk":         0.20,
			"wall_change_count_5m":  2.0,
			"support_wall": map[string]interface{}{
				"strength_usd":    500000.0,
				"distance_pct":    0.5,
				"is_solid":        true,
				"stability_score": 0.85,
				"flicker_count":   1.0,
			},
			"resistance_wall": nil,
		},
	}

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

	// 调用方向裁决
	result := ComputeDirectionForSymbol("BTCUSDT", orderflowData, mtfData)

	// 验证结果
	if result == nil {
		t.Fatal("❌ 方向裁决返回nil")
	}

	t.Logf("=== 方向裁决结果 ===")
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)
	t.Logf("置信度: %.2f", result.Confidence)
	t.Logf("订单流方向: %.2f", result.OFDir)
	t.Logf("结构方向: %.2f", result.StructDir)
	t.Logf("订单流质量: %.2f", result.OFQuality)
	t.Logf("标记: %v", result.Flags)

	// 关键断言
	if result.PlanSide == "CHANNEL_DATA_MISSING" {
		t.Error("❌ 不应该返回CHANNEL_DATA_MISSING，通道数据已提供")
	}

	if result.PlanSide == "UNKNOWN" {
		t.Error("❌ 不应该返回UNKNOWN，数据质量正常")
	}

	// 验证通道数据被正确使用
	if result.StructDir == 0 {
		t.Error("❌ StructDir为0，说明通道数据未被使用")
	}

	// 验证订单流数据被正确使用
	if result.OFDir == 0 {
		t.Error("❌ OFDir为0，说明订单流数据未被使用")
	}

	// 验证质量分数
	if result.OFQuality <= 0 {
		t.Error("❌ OFQuality应该>0，数据质量正常")
	}

	t.Logf("✅ 方向裁决正常工作，返回了有效的判断结果")
}

// TestDirectionArbitrationWithInvalidChannelData 测试通道数据缺失场景
func TestDirectionArbitrationWithInvalidChannelData(t *testing.T) {
	// 正常的订单流数据
	orderflowData := map[string]interface{}{
		"数据质量": map[string]interface{}{
			"status":                  "正常",
			"overall_score":           0.85,
			"data_lag_ms":             100.0,
			"update_interval_s":       5.0,
			"cvd_reliability":         0.90,
			"oi_reliability":          0.85,
			"orderbook_reliability":   0.88,
		},
		"宏观资金趋势": map[string]interface{}{
			"trend_alignment":  "bullish",
			"signal_strength":  75.0,
			"confidence_level": 0.80,
			"market_regime":    "trending",
			"cvd_divergence":   false,
		},
		"本周期博弈_5m": map[string]interface{}{
			"candle_intent":         "strong_buy",
			"futures_cvd_delta_usd": 1500000.0,
			"spot_cvd_delta_usd":    800000.0,
			"oi_delta_pct":          2.5,
			"price_delta_pct":       1.2,
			"volume_delta":          5000000.0,
			"data_quality":          0.90,
		},
		"盘口结构_v2": map[string]interface{}{
			"imbalance_ratio":       1.5,
			"bid_pressure":          0.65,
			"ask_pressure":          0.35,
			"liquidity_score":       0.75,
			"spoofing_risk":         0.20,
			"wall_change_count_5m":  2.0,
		},
	}

	// 通道数据质量为0（无效）
	mtfData := map[string]interface{}{
		"15m": map[string]interface{}{
			"通道数据": map[string]interface{}{
				"channel_direction": "sideways",
				"current_position":  "Inside",
				"channel_width_pct": 0.0, // 无效宽度
			},
			"VPVR数据": map[string]interface{}{
				"dist_to_vah_atr": 1.5,
				"dist_to_val_atr": -2.0,
			},
		},
		"30m": map[string]interface{}{
			"通道数据": map[string]interface{}{
				"channel_direction": "sideways",
				"current_position":  "Inside",
				"channel_width_pct": 0.0, // 无效宽度
			},
			"VPVR数据": map[string]interface{}{
				"dist_to_vah_atr": 1.0,
				"dist_to_val_atr": -1.5,
			},
		},
	}

	// 调用方向裁决
	result := ComputeDirectionForSymbol("BTCUSDT", orderflowData, mtfData)

	// 验证结果
	if result == nil {
		t.Fatal("❌ 方向裁决返回nil")
	}

	t.Logf("=== 通道数据缺失场景 ===")
	t.Logf("判断方向: %s", result.PlanSide)
	t.Logf("是否拦截: %v", result.BlockEntry)
	t.Logf("拦截原因: %s", result.BlockReason)

	// 应该返回CHANNEL_DATA_MISSING
	if result.PlanSide != "CHANNEL_DATA_MISSING" {
		t.Errorf("❌ 应该返回CHANNEL_DATA_MISSING，实际返回: %s", result.PlanSide)
	}

	if !result.BlockEntry {
		t.Error("❌ 应该拦截开仓")
	}

	if result.BlockReason != "CHANNEL_DATA_MISSING" {
		t.Errorf("❌ 拦截原因应该是CHANNEL_DATA_MISSING，实际: %s", result.BlockReason)
	}

	t.Logf("✅ 正确识别通道数据缺失并拦截")
}
