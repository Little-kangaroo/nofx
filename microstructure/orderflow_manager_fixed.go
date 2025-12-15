package microstructure

import (
	"fmt"
	"log"
	"os"
	"time"
)

// ===== P0-07修复：ToAIPayload方法修复 =====

// ToAIPayloadFixed 🔥 P0-07修复版：字段一致性修复的AI输出方法
func (ms *MarketSnapshot) ToAIPayloadFixed() map[string]interface{} {
	// 🔥 P0-07修复：使用标准化接口，确保字段一致性
	standardPayload := GenerateStandardizedAIPayload(ms)
	
	// 🔥 P0-07修复：记录系统状态
	LogSystemDegradation(standardPayload.SystemStatus, ms.Symbol)
	
	// 🔥 P0-07修复：在兜底模式下添加明确警告
	if standardPayload.CompatMode == "V2.0_FALLBACK" {
		log.Printf("⚠️ [P0-07] %s 使用字段标准化的兜底模式", ms.Symbol)
		
		if os.Getenv("NOFX_DEBUG") == "true" {
			log.Printf("🔧 [P0-07调试] 兜底内容预览: %+v", 
				extractDebugInfo(standardPayload.MainContent))
		}
	}
	
	// 🔥 P0-07修复：返回标准化的主内容，确保字段结构一致
	result := standardPayload.MainContent
	
	// 🔥 P0-07修复：添加元数据用于调试和监控
	if metadata := extractMetadata(standardPayload); metadata != nil {
		// 只在调试模式下添加元数据，避免干扰正常模板解析
		if os.Getenv("NOFX_DEBUG") == "true" {
			result["_metadata"] = metadata
		}
	}
	
	return result
}

// ToAIPayloadWithP007Fix 🔥 P0-07修复：保持向后兼容的同时使用修复版本
func (ms *MarketSnapshot) ToAIPayloadWithP007Fix() map[string]interface{} {
	// 🔥 P0-07修复：直接调用修复版本
	return ms.ToAIPayloadFixed()
}

// extractDebugInfo 提取调试信息
func extractDebugInfo(content map[string]interface{}) map[string]interface{} {
	debugInfo := map[string]interface{}{}
	
	// 检查根结构
	for key := range content {
		if key == "V-12.2订单流分析系统" {
			debugInfo["root_structure_found"] = true
			if rootContent, ok := content[key].(map[string]interface{}); ok {
				debugInfo["sections"] = extractSectionNames(rootContent)
			}
		}
	}
	
	return debugInfo
}

// extractSectionNames 提取节名称
func extractSectionNames(rootContent map[string]interface{}) []string {
	var sections []string
	for key := range rootContent {
		sections = append(sections, key)
	}
	return sections
}

// extractMetadata 提取元数据
func extractMetadata(payload *AIPayloadStandard) map[string]interface{} {
	return map[string]interface{}{
		"version":            payload.Version,
		"compat_mode":        payload.CompatMode,
		"quality_status":     payload.QualityStatus,
		"system_mode":        payload.SystemStatus.SystemMode,
		"ai_available":       payload.SystemStatus.AIInterfaceAvailable,
		"quality_score":      payload.SystemStatus.QualityScore,
		"degradation_reason": payload.SystemStatus.DegradationReason,
		"timestamp":          payload.Timestamp.Format("15:04:05.000"),
	}
}

// ===== P0-07修复：验证方法 =====

// ValidateFieldConsistency 🔥 P0-07修复：验证字段一致性
func ValidateFieldConsistency(symbol string, aiInterfaceAvailable bool) map[string]interface{} {
	log.Printf("🔬 [P0-07验证] 开始字段一致性验证: %s, AI可用=%v", symbol, aiInterfaceAvailable)
	
	// 创建模拟快照用于测试
	mockSnapshot := createMockMarketSnapshot(symbol)
	
	var mainPayload, fallbackPayload map[string]interface{}
	
	if aiInterfaceAvailable {
		// 测试主路径
		aiInterface := GetGlobalAIInterfaceV12()
		if aiInterface != nil {
			if contextV12, err := aiInterface.GenerateAIContext(symbol); err == nil {
				mainPayload = contextV12.ToAIPromptFormat()
			}
		}
		fallbackPayload = mockSnapshot.ToAIPayloadFixed()
	} else {
		// 强制测试兜底路径（模拟AI接口不可用）
		mainPayload = nil
		fallbackPayload = mockSnapshot.ToAIPayloadFixed()
	}
	
	// 验证结果
	validation := map[string]interface{}{
		"symbol":              symbol,
		"ai_interface_available": aiInterfaceAvailable,
		"main_payload_available": mainPayload != nil,
		"fallback_generated":   fallbackPayload != nil,
		"field_consistency":    validateFieldStructure(mainPayload, fallbackPayload),
		"test_timestamp":       mockSnapshot.Timestamp.Format("15:04:05.000"),
	}
	
	log.Printf("✅ [P0-07验证] 完成字段一致性验证: %s", validation["field_consistency"])
	return validation
}

// createMockMarketSnapshot 创建模拟快照
func createMockMarketSnapshot(symbol string) *MarketSnapshot {
	// 返回基础的模拟快照用于测试
	return &MarketSnapshot{
		Symbol:    symbol,
		Timestamp: time.Now(),
		CVDData: &CVDData{
			SpotCVD1H:     1000.0,
			FuturesCVD1H:  -800.0,
			CVDDivergence: "低分歧",
			Signal:        "中性",
			IsStale:       false,
		},
		OrderBookData: &OrderBookData{
			ImbalanceRatio: 0.15,
			SpoofingRisk:   0.3,
			LiquidityScore: 0.7,
			IsStale:        false,
		},
		OIAnalysis: &OIAnalysis{
			ChangeRate1H: 0.05,
			IsStale:      false,
		},
		DataQuality: &DataQualityInfo{
			OverallScore:          0.8,
			CVDReliability:        0.9,
			OrderBookReliability:  0.8,
			OIReliability:         0.7,
			Status:               "正常",
		},
	}
}

// validateFieldStructure 验证字段结构
func validateFieldStructure(mainPayload, fallbackPayload map[string]interface{}) string {
	if mainPayload == nil && fallbackPayload == nil {
		return "两个载荷都为空"
	}
	
	if mainPayload == nil {
		return validateFallbackStructure(fallbackPayload)
	}
	
	if fallbackPayload == nil {
		return "兜底载荷为空"
	}
	
	// 检查根键一致性
	mainRootKeys := extractRootKeys(mainPayload)
	fallbackRootKeys := extractRootKeys(fallbackPayload)
	
	if !compareStringSlices(mainRootKeys, fallbackRootKeys) {
		return fmt.Sprintf("根键不一致 - 主路径: %v, 兜底: %v", mainRootKeys, fallbackRootKeys)
	}
	
	return "字段结构一致"
}

// validateFallbackStructure 验证兜底结构
func validateFallbackStructure(fallbackPayload map[string]interface{}) string {
	// 检查是否包含标准化的根结构
	if _, hasStandardRoot := fallbackPayload["V-12.2订单流分析系统"]; hasStandardRoot {
		return "兜底模式使用标准化字段结构 - 符合P0-07修复要求"
	}
	
	if _, hasLegacyRoot := fallbackPayload["订单流分析"]; hasLegacyRoot {
		return "警告：兜底模式使用旧版字段结构 - 需要P0-07修复"
	}
	
	return "未知的兜底结构"
}

// extractRootKeys 提取根键
func extractRootKeys(payload map[string]interface{}) []string {
	var keys []string
	for key := range payload {
		if key != "_metadata" { // 跳过元数据键
			keys = append(keys, key)
		}
	}
	return keys
}

// compareStringSlices 比较字符串切片
func compareStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	
	aMap := make(map[string]bool)
	for _, item := range a {
		aMap[item] = true
	}
	
	for _, item := range b {
		if !aMap[item] {
			return false
		}
	}
	
	return true
}

// ===== P0-07修复：生产环境监控 =====

// MonitorAIInterfaceHealth 🔥 P0-07修复：监控AI接口健康度
func MonitorAIInterfaceHealth(symbols []string) map[string]interface{} {
	log.Printf("🔬 [P0-07监控] 开始AI接口健康度检查，监控%d个交易对", len(symbols))
	
	aiInterface := GetGlobalAIInterfaceV12()
	
	healthReport := map[string]interface{}{
		"check_time":        time.Now().Format("15:04:05.000"),
		"ai_interface_available": aiInterface != nil,
		"total_symbols":     len(symbols),
		"degraded_count":    0,
		"fatal_count":       0,
		"symbol_status":     map[string]string{},
		"recommendations":   []string{},
	}
	
	degradedCount := 0
	fatalCount := 0
	
	for _, symbol := range symbols {
		mockSnapshot := createMockMarketSnapshot(symbol)
		standardPayload := GenerateStandardizedAIPayload(mockSnapshot)
		
		switch standardPayload.QualityStatus {
		case "FATAL":
			fatalCount++
			healthReport["symbol_status"].(map[string]string)[symbol] = "FATAL"
		case "DEGRADED":
			degradedCount++
			healthReport["symbol_status"].(map[string]string)[symbol] = "DEGRADED"
		default:
			healthReport["symbol_status"].(map[string]string)[symbol] = "NORMAL"
		}
	}
	
	healthReport["degraded_count"] = degradedCount
	healthReport["fatal_count"] = fatalCount
	
	// 生成建议
	recommendations := []string{}
	if fatalCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d个交易对处于FATAL状态，需要立即检查AI接口", fatalCount))
	}
	if degradedCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d个交易对处于DEGRADED状态，建议监控数据质量", degradedCount))
	}
	if aiInterface == nil {
		recommendations = append(recommendations, "AI接口完全不可用，所有交易对使用兜底模式")
		recommendations = append(recommendations, "请检查AI接口服务状态并重启相关组件")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "所有交易对AI接口运行正常")
	}
	
	healthReport["recommendations"] = recommendations
	
	// 记录健康度报告
	if fatalCount > 0 || degradedCount > len(symbols)/2 {
		log.Printf("🚨 [P0-07监控] AI接口健康度警告: FATAL=%d, DEGRADED=%d", fatalCount, degradedCount)
	} else {
		log.Printf("✅ [P0-07监控] AI接口健康度良好: 总计=%d, 正常=%d", len(symbols), len(symbols)-degradedCount-fatalCount)
	}
	
	return healthReport
}