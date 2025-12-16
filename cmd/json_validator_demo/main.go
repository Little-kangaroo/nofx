package main

import (
	"encoding/json"
	"log"
	"strings"

	"nofx/market"
)

// JSONSchemaValidatorCLI JSON Schema验证器命令行工具
// 用于演示和测试P1-2修复的JSON schema验证功能
func main() {
	log.Printf("🚀 [P1-2演示] JSON Schema验证器 - 防止AI输出拼写错误")
	log.Printf(strings.Repeat("=", 60))

	// 演示各种验证场景
	demonstrateValidation()

	// 运行测试套件
	log.Printf("\n🧪 运行测试套件...")
	runValidationDemo()

	log.Printf("\n✅ [P1-2] JSON Schema验证器演示完成！")
}

func demonstrateValidation() {
	validator := market.NewJSONSchemaValidator()

	// 演示案例1: 正确的输出
	log.Printf("\n📋 案例1: 正确的Gate2输出")
	correctGate2 := map[string]interface{}{
		"struct_state_long":  "STRONG_LONG",
		"struct_state_short": "NEUTRAL",
		"last_price":        91256.78,
		"confidence":        0.92,
		"direction":         "bullish",
	}
	
	result := validator.ValidateJSON(correctGate2)
	printValidationResult("正确输出", result)

	// 演示案例2: 字段拼写错误
	log.Printf("\n📋 案例2: 字段拼写错误 (模拟AI输出错误)")
	spellingErrors := map[string]interface{}{
		"struct_long":        "STRONG_LONG",  // 错误：应该是 struct_state_long
		"struct_state_short": "NEUTRAL",
		"current_price":      91256.78,       // 错误：应该是 last_price
		"confidenct":        0.92,            // 错误：应该是 confidence
		"diretion":          "bullish",       // 错误：应该是 direction
	}
	
	result = validator.ValidateJSON(spellingErrors)
	printValidationResult("拼写错误", result)

	// 演示案例3: 枚举值错误
	log.Printf("\n📋 案例3: 枚举值错误")
	enumErrors := map[string]interface{}{
		"struct_state_long":  "VERY_STRONG",    // 错误：不在枚举列表中
		"struct_state_short": "WEAK",           // 错误：应该是 WEAK_SHORT
		"last_price":        91256.78,
		"direction":         "sideways",        // 错误：应该是 neutral
		"candle_intent":     "maybe_buy",       // 错误：不在枚举列表中
	}
	
	result = validator.ValidateJSON(enumErrors)
	printValidationResult("枚举值错误", result)

	// 演示案例4: 数值范围错误
	log.Printf("\n📋 案例4: 数值范围错误")
	rangeErrors := map[string]interface{}{
		"struct_state_long":  "STRONG_LONG",
		"struct_state_short": "NEUTRAL",
		"last_price":        -100.0,           // 错误：负数价格
		"confidence":        1.5,              // 错误：超出0-1范围
		"overall_score":     150.0,            // 错误：超出0-1范围 (应该是1.5, 被误识别为0-100量纲)
	}
	
	result = validator.ValidateJSON(rangeErrors)
	printValidationResult("数值范围错误", result)

	// 演示案例5: 订单流输出验证
	log.Printf("\n📋 案例5: 订单流输出验证")
	orderFlowData := map[string]interface{}{
		"candle_intent":     "strong_buy",
		"data_quality":      0.85,
		"overall_score":     0.92,
		"trend_aligment":    "bullish_aligned", // 错误：拼写错误
		"market_regim":      "trending_up",     // 错误：拼写错误
	}
	
	result = market.ValidateAIOutput(orderFlowData, "orderflow")
	printValidationResult("订单流输出", result)

	// 演示案例6: 自动纠错演示
	log.Printf("\n📋 案例6: 自动纠错演示")
	demonstrateAutoCorrection(validator)

	// 显示验证统计
	log.Printf("\n📊 显示验证统计...")
	stats := validator.GetValidationStatistics()
	printStatistics(stats)
}

func printValidationResult(caseName string, result *market.ValidationResult) {
	log.Printf("  结果: %s", caseName)
	
	if result.IsValid {
		log.Printf("  ✅ 验证通过")
	} else {
		log.Printf("  ❌ 验证失败 (%d 个错误)", len(result.Errors))
	}
	
	if len(result.Errors) > 0 {
		log.Printf("  错误详情:")
		for _, err := range result.Errors {
			log.Printf("    - [%s] %s: %s", err.Severity, err.Field, err.Message)
		}
	}
	
	if len(result.Warnings) > 0 {
		log.Printf("  ⚠️ 警告 (%d 个):", len(result.Warnings))
		for _, warning := range result.Warnings {
			log.Printf("    - %s: %s", warning.Field, warning.Message)
		}
	}
	
	if len(result.CorrectedFields) > 0 {
		log.Printf("  🔧 自动纠错 (%d 个):", len(result.CorrectedFields))
		for original, corrected := range result.CorrectedFields {
			log.Printf("    - '%s' → '%s'", original, corrected)
		}
	}
}

func demonstrateAutoCorrection(validator *market.JSONSchemaValidator) {
	// 原始数据 (包含拼写错误)
	originalData := map[string]interface{}{
		"struct_long":    "STRONG_LONG",
		"current_price":  91256.78,
		"confidenct":    0.92,
		"diretion":      "bullish",
	}
	
	log.Printf("  原始数据 (含拼写错误):")
	printJSON(originalData)
	
	// 执行验证和自动纠错
	result := validator.ValidateJSON(originalData)
	
	if result.ProcessedData != nil {
		log.Printf("  纠错后数据:")
		printJSON(result.ProcessedData)
	}
	
	log.Printf("  纠错映射:")
	for original, corrected := range result.CorrectedFields {
		log.Printf("    '%s' → '%s'", original, corrected)
	}
}

func printJSON(data interface{}) {
	jsonData, err := json.MarshalIndent(data, "    ", "  ")
	if err != nil {
		log.Printf("    JSON序列化失败: %v", err)
		return
	}
	log.Printf("    %s", string(jsonData))
}

func printStatistics(stats *market.ValidationStatistics) {
	log.Printf("  总验证次数: %d", stats.TotalValidations)
	log.Printf("  成功次数: %d", stats.SuccessfulValidations)
	log.Printf("  失败次数: %d", stats.FailedValidations)
	log.Printf("  成功率: %.1f%%", stats.SuccessRate)
	log.Printf("  自动纠错次数: %d", stats.AutoCorrectionsMade)
	
	if len(stats.CommonErrors) > 0 {
		log.Printf("  常见错误类型:")
		for errType, count := range stats.CommonErrors {
			log.Printf("    - %s: %d次", errType, count)
		}
	}
}

func runValidationDemo() {
	validator := market.NewJSONSchemaValidator()
	
	// 模拟运行一些验证测试
	testData := []map[string]interface{}{
		{"struct_state_long": "STRONG_LONG", "last_price": 45678.90},
		{"struct_long": "STRONG_LONG", "current_price": 45678.90}, 
		{"struct_state_long": "INVALID", "last_price": -100},
	}
	
	for i, data := range testData {
		log.Printf("🧪 测试案例 %d", i+1)
		result := validator.ValidateJSON(data)
		if result.IsValid {
			log.Printf("✅ 验证通过")
		} else {
			log.Printf("❌ 验证失败：%d个错误", len(result.Errors))
		}
	}
	
	// 显示最终统计
	market.LogValidationSummary()
}