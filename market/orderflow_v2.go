package market

import (
	"log"
	"nofx/microstructure"
)

// GetOrderFlowDataForAIV2 获取指定币种的订单流数据（供AI使用 - V2.0版本）
// 修复版本：始终提供完整的数据结构，符合AI_Prompt_V2_Template.md要求
func GetOrderFlowDataForAIV2(symbol string) map[string]interface{} {
	// 尝试获取全局订单流管理器
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️ 订单流数据获取失败 %s: %v", symbol, r)
		}
	}()

	// 检查订单流系统是否已初始化
	ofm := microstructure.GetGlobalOrderFlowManager()
	if ofm == nil {
		return map[string]interface{}{
			"状态":          "订单流系统未初始化",
			"本周期博弈_5m": buildEmptyCurrentPeriodDataV2(),
			"宏观资金趋势":  buildEmptyMacroTrendDataV2(),
			"盘口结构_v2":   buildEmptyOrderBookDataV2(),
			"数据质量":      buildEmptyDataQualityInfoV2(),
		}
	}

	// 获取市场快照（V2.0增强版本）
	snapshot := ofm.GetMarketSnapshot(symbol)
	if snapshot == nil {
		return map[string]interface{}{
			"状态":          "暂无订单流数据",
			"本周期博弈_5m": buildEmptyCurrentPeriodDataV2(),
			"宏观资金趋势":  buildEmptyMacroTrendDataV2(),
			"盘口结构_v2":   buildEmptyOrderBookDataV2(),
			"数据质量":      buildEmptyDataQualityInfoV2(),
		}
	}

	// 检查数据是否过期 - 但仍然提供完整结构
	isDataStale := snapshot.CVDData.IsStale || snapshot.OIAnalysis.IsStale || snapshot.OrderBookData.IsStale

	// 构建V2.0 AI数据格式 - 始终提供完整结构
	result := map[string]interface{}{
		"本周期博弈_5m": buildCurrentPeriodDataV2(snapshot, isDataStale),
		"宏观资金趋势":  buildMacroTrendDataV2(snapshot, isDataStale),
		"盘口结构_v2":   buildEnhancedOrderBookDataV2(snapshot.OrderBookData, isDataStale),
		"数据质量":      buildDataQualityInfoV2(snapshot, isDataStale),
	}

	// 如果数据过期，添加状态标识但不破坏结构
	if isDataStale {
		result["状态"] = "数据过期"
	}

	return result
}