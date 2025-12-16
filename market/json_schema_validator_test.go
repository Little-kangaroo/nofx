package market

import (
	"encoding/json"
	"log"
	"testing"
)

// TestJSONSchemaValidation 测试JSON schema验证功能
func TestJSONSchemaValidation(t *testing.T) {
	log.Printf("🧪 [P1-2测试] 开始测试JSON schema验证器...")

	validator := NewJSONSchemaValidator()

	// 测试数据：模拟AI输出中的常见拼写错误
	testCases := []struct {
		name     string
		data     map[string]interface{}
		expected struct {
			hasErrors     bool
			hasWarnings   bool
			hasCorrections bool
		}
	}{
		{
			name: "正确的Gate2输出",
			data: map[string]interface{}{
				"struct_state_long":  "STRONG_LONG",
				"struct_state_short": "NEUTRAL", 
				"last_price":        45678.90,
				"confidence":        0.85,
			},
			expected: struct {
				hasErrors     bool
				hasWarnings   bool
				hasCorrections bool
			}{false, false, false},
		},
		{
			name: "字段名拼写错误",
			data: map[string]interface{}{
				"struct_long":        "STRONG_LONG",  // 应该是 struct_state_long
				"struct_state_short": "NEUTRAL",
				"current_price":      45678.90,       // 应该是 last_price
				"confidenct":        0.85,            // 拼写错误：应该是 confidence
			},
			expected: struct {
				hasErrors     bool
				hasWarnings   bool
				hasCorrections bool
			}{false, true, true},
		},
		{
			name: "枚举值错误",
			data: map[string]interface{}{
				"struct_state_long":  "VERY_STRONG",  // 无效枚举值
				"struct_state_short": "bearish",      // 错误的枚举值
				"last_price":        45678.90,
				"direction":         "bullsh",        // 拼写错误：应该是 bullish
			},
			expected: struct {
				hasErrors     bool
				hasWarnings   bool
				hasCorrections bool
			}{true, true, false},
		},
		{
			name: "数值范围错误", 
			data: map[string]interface{}{
				"struct_state_long":  "STRONG_LONG",
				"struct_state_short": "NEUTRAL",
				"last_price":        -100.0,         // 负数价格无效
				"confidence":        1.5,            // 超出0-1范围
				"overall_score":     150.0,          // 超出0-1范围
			},
			expected: struct {
				hasErrors     bool
				hasWarnings   bool
				hasCorrections bool
			}{true, false, false},
		},
		{
			name: "订单流输出测试",
			data: map[string]interface{}{
				"candle_intent":     "strong_buy",
				"data_quality":      0.85,
				"overall_score":     0.92,
				"trend_aligment":    "bullish_aligned", // 拼写错误：应该是 trend_alignment
			},
			expected: struct {
				hasErrors     bool
				hasWarnings   bool
				hasCorrections bool
			}{false, true, true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			log.Printf("🧪 测试用例: %s", tc.name)

			// 执行验证
			result := validator.ValidateJSON(tc.data)

			// 检查错误
			hasErrors := !result.IsValid || len(result.Errors) > 0
			if hasErrors != tc.expected.hasErrors {
				t.Errorf("错误检测失败: expected hasErrors=%v, got=%v", tc.expected.hasErrors, hasErrors)
				log.Printf("❌ 错误: %v", result.Errors)
			}

			// 检查警告
			hasWarnings := len(result.Warnings) > 0
			if hasWarnings != tc.expected.hasWarnings {
				t.Errorf("警告检测失败: expected hasWarnings=%v, got=%v", tc.expected.hasWarnings, hasWarnings)
				log.Printf("⚠️ 警告: %v", result.Warnings)
			}

			// 检查纠错
			hasCorrections := len(result.CorrectedFields) > 0
			if hasCorrections != tc.expected.hasCorrections {
				t.Errorf("纠错检测失败: expected hasCorrections=%v, got=%v", tc.expected.hasCorrections, hasCorrections)
				log.Printf("🔧 纠错: %v", result.CorrectedFields)
			}

			// 打印详细结果
			if hasErrors {
				log.Printf("  错误数量: %d", len(result.Errors))
			}
			if hasWarnings {
				log.Printf("  警告数量: %d", len(result.Warnings))
			}
			if hasCorrections {
				log.Printf("  纠错数量: %d", len(result.CorrectedFields))
			}
		})
	}
}

// TestSpecificValidationFunctions 测试特定验证函数
func TestSpecificValidationFunctions(t *testing.T) {
	log.Printf("🧪 [P1-2测试] 测试特定验证函数...")

	// 测试Gate2输出验证
	gate2Data := map[string]interface{}{
		"struct_state_long":  "STRONG_LONG",
		"struct_state_short": "NEUTRAL",
		"last_price":        45678.90,
	}

	result := ValidateAIOutput(gate2Data, "gate2")
	if !result.IsValid {
		t.Errorf("Gate2验证失败: %v", result.Errors)
	}

	// 测试订单流输出验证
	orderFlowData := map[string]interface{}{
		"candle_intent":  "strong_buy",
		"data_quality":   0.85,
		"overall_score":  0.92,
	}

	result = ValidateAIOutput(orderFlowData, "orderflow")
	if !result.IsValid {
		t.Errorf("订单流验证失败: %v", result.Errors)
	}
}

// TestAutoCorrection 测试自动纠错功能
func TestAutoCorrection(t *testing.T) {
	log.Printf("🧪 [P1-2测试] 测试自动纠错功能...")

	validator := NewJSONSchemaValidator()
	
	// 含有拼写错误的数据
	originalData := map[string]interface{}{
		"struct_long":    "STRONG_LONG",  // 应该是 struct_state_long
		"current_price":  45678.90,       // 应该是 last_price  
		"confidenct":    0.85,            // 应该是 confidence
	}

	result := validator.ValidateJSON(originalData)

	// 检查是否有纠错
	if len(result.CorrectedFields) == 0 {
		t.Error("自动纠错功能未工作")
		return
	}

	// 检查纠错后的数据
	if result.ProcessedData == nil {
		t.Error("纠错后的数据为空")
		return
	}

	correctedData, ok := result.ProcessedData.(map[string]interface{})
	if !ok {
		t.Error("纠错后的数据类型错误")
		return
	}

	// 验证关键字段是否被正确纠正
	expectedCorrections := map[string]string{
		"struct_long":    "struct_state_long", 
		"current_price":  "last_price",
		"confidenct":    "confidence",
	}

	for original, expected := range expectedCorrections {
		if corrected, exists := result.CorrectedFields[original]; exists {
			if corrected != expected {
				t.Errorf("字段 %s 纠错错误: expected=%s, got=%s", original, expected, corrected)
			} else {
				log.Printf("✅ 字段 '%s' 成功纠正为 '%s'", original, expected)
			}
		}
	}

	// 验证纠错后的数据包含正确的字段
	for _, correctedField := range expectedCorrections {
		if _, exists := correctedData[correctedField]; !exists {
			t.Errorf("纠错后的数据缺少字段: %s", correctedField)
		}
	}

	log.Printf("✅ 自动纠错功能测试通过")
}

// TestValidationStatistics 测试验证统计功能
func TestValidationStatistics(t *testing.T) {
	log.Printf("🧪 [P1-2测试] 测试验证统计功能...")

	validator := NewJSONSchemaValidator()

	// 执行多次验证以生成统计数据
	testData := []map[string]interface{}{
		{"struct_state_long": "STRONG_LONG", "last_price": 45678.90},           // 正确
		{"struct_long": "STRONG_LONG", "current_price": 45678.90},              // 拼写错误
		{"struct_state_long": "INVALID", "last_price": -100},                   // 枚举和范围错误
		{"struct_state_long": "STRONG_LONG", "last_price": 45678.90},           // 正确
	}

	for _, data := range testData {
		validator.ValidateJSON(data)
	}

	stats := validator.GetValidationStatistics()

	if stats.TotalValidations != 4 {
		t.Errorf("总验证次数错误: expected=4, got=%d", stats.TotalValidations)
	}

	if stats.SuccessfulValidations == 0 {
		t.Error("成功验证次数为0")
	}

	if stats.AutoCorrectionsMade == 0 {
		t.Error("自动纠错次数为0")
	}

	if len(stats.CommonErrors) == 0 {
		t.Error("常见错误统计为空")
	}

	log.Printf("📊 验证统计: 总计=%d, 成功=%d, 失败=%d, 纠错=%d",
		stats.TotalValidations, stats.SuccessfulValidations, 
		stats.FailedValidations, stats.AutoCorrectionsMade)

	log.Printf("✅ 验证统计功能测试通过")
}

// TestSchemaExport 测试schema导出功能
func TestSchemaExport(t *testing.T) {
	log.Printf("🧪 [P1-2测试] 测试schema导出功能...")

	validator := NewJSONSchemaValidator()

	schemaJSON, err := validator.ExportSchemaAsJSON()
	if err != nil {
		t.Errorf("Schema导出失败: %v", err)
		return
	}

	// 验证导出的JSON是否有效
	var exportedSchema map[string]interface{}
	if err := json.Unmarshal([]byte(schemaJSON), &exportedSchema); err != nil {
		t.Errorf("导出的Schema JSON无效: %v", err)
		return
	}

	// 检查是否包含关键字段
	expectedFields := []string{
		"struct_state_long", "struct_state_short", "last_price", 
		"direction", "confidence", "candle_intent",
	}

	for _, field := range expectedFields {
		if _, exists := exportedSchema[field]; !exists {
			t.Errorf("导出的Schema缺少字段: %s", field)
		}
	}

	log.Printf("📄 Schema导出包含 %d 个字段定义", len(exportedSchema))
	log.Printf("✅ Schema导出功能测试通过")
}

// TestErrorDetection 测试各种错误检测
func TestErrorDetection(t *testing.T) {
	log.Printf("🧪 [P1-2测试] 测试错误检测功能...")

	validator := NewJSONSchemaValidator()

	// 类型错误测试
	typeErrorData := map[string]interface{}{
		"struct_state_long": 123,           // 应该是string
		"last_price":       "not_a_number", // 应该是number
		"confidence":       "high",         // 应该是float
	}

	result := validator.ValidateJSON(typeErrorData)
	if result.IsValid {
		t.Error("类型错误检测失败：应该检测出错误")
	}

	typeErrors := 0
	for _, err := range result.Errors {
		if err.ErrorType == "type_mismatch" {
			typeErrors++
		}
	}

	if typeErrors == 0 {
		t.Error("没有检测到类型错误")
	}

	log.Printf("📋 检测到 %d 个类型错误", typeErrors)

	// 枚举错误测试
	enumErrorData := map[string]interface{}{
		"struct_state_long": "INVALID_STATE",
		"direction":        "sideways",
		"candle_intent":    "maybe_buy",
	}

	result = validator.ValidateJSON(enumErrorData)
	enumErrors := 0
	for _, err := range result.Errors {
		if err.ErrorType == "invalid_enum_value" {
			enumErrors++
		}
	}

	if enumErrors == 0 {
		t.Error("没有检测到枚举错误")
	}

	log.Printf("📋 检测到 %d 个枚举错误", enumErrors)

	// 范围错误测试  
	rangeErrorData := map[string]interface{}{
		"last_price":    -100.0,  // 负数
		"confidence":    1.5,     // 超出范围
		"overall_score": 150.0,   // 超出范围
	}

	result = validator.ValidateJSON(rangeErrorData)
	rangeErrors := 0
	for _, err := range result.Errors {
		if err.ErrorType == "value_too_small" || err.ErrorType == "value_too_large" {
			rangeErrors++
		}
	}

	if rangeErrors == 0 {
		t.Error("没有检测到范围错误")
	}

	log.Printf("📋 检测到 %d 个范围错误", rangeErrors)
	log.Printf("✅ 错误检测功能测试通过")
}

// BenchmarkValidation 性能测试
func BenchmarkValidation(b *testing.B) {
	validator := NewJSONSchemaValidator()
	
	testData := map[string]interface{}{
		"struct_state_long":  "STRONG_LONG",
		"struct_state_short": "NEUTRAL",
		"last_price":        45678.90,
		"confidence":        0.85,
		"direction":         "bullish",
		"candle_intent":     "strong_buy",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateJSON(testData)
	}
}

// 模拟运行所有测试
func RunAllTests() {
	log.Printf("🧪 [P1-2] 开始运行JSON Schema验证器测试套件...")

	// 创建模拟测试环境
	t := &testing.T{}
	
	// 运行各种测试
	TestJSONSchemaValidation(t)
	TestSpecificValidationFunctions(t)
	TestAutoCorrection(t)
	TestValidationStatistics(t)
	TestSchemaExport(t)
	TestErrorDetection(t)

	// 输出验证统计摘要
	LogValidationSummary()

	log.Printf("✅ [P1-2] JSON Schema验证器测试完成！")
}