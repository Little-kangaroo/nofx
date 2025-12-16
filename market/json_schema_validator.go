package market

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

// JSONSchemaValidator JSON schema验证器 - P1-2修复：防止AI输出拼写错误
// 解决AI输出中可能出现的字段名拼写错误、类型错误等问题
type JSONSchemaValidator struct {
	schemas                map[string]*FieldSchema
	strictMode            bool
	logValidationErrors   bool
	autoCorrectEnabled    bool
	validationStatistics  *ValidationStatistics
}

// FieldSchema 字段schema定义
type FieldSchema struct {
	Name         string        `json:"name"`          // 字段名
	Type         FieldType     `json:"type"`          // 字段类型
	Required     bool          `json:"required"`      // 是否必需
	AllowedValues []string     `json:"allowed_values"` // 允许的值（枚举）
	Pattern      *regexp.Regexp `json:"-"`             // 正则表达式验证
	MinValue     *float64      `json:"min_value"`     // 最小值
	MaxValue     *float64      `json:"max_value"`     // 最大值
	Description  string        `json:"description"`   // 字段描述
	Aliases      []string      `json:"aliases"`       // 别名（用于拼写纠错）
}

// FieldType 字段类型枚举
type FieldType string

const (
	TypeString      FieldType = "string"
	TypeFloat       FieldType = "float"
	TypeInt         FieldType = "int"
	TypeBool        FieldType = "bool"
	TypeArray       FieldType = "array"
	TypeObject      FieldType = "object"
	TypeEnum        FieldType = "enum"
	TypePrice       FieldType = "price"
	TypePercentage  FieldType = "percentage"
	TypeDirection   FieldType = "direction"
	TypeTimeframe   FieldType = "timeframe"
)

// ValidationResult 验证结果
type ValidationResult struct {
	IsValid         bool                    `json:"is_valid"`
	Errors          []ValidationError       `json:"errors"`
	Warnings        []ValidationWarning     `json:"warnings"`
	CorrectedFields map[string]interface{}  `json:"corrected_fields"`
	ValidationTime  time.Time               `json:"validation_time"`
	OriginalData    interface{}             `json:"original_data"`
	ProcessedData   interface{}             `json:"processed_data"`
}

// ValidationError 验证错误
type ValidationError struct {
	Field       string `json:"field"`
	ErrorType   string `json:"error_type"`
	Message     string `json:"message"`
	ExpectedValue interface{} `json:"expected_value"`
	ActualValue   interface{} `json:"actual_value"`
	Severity    string `json:"severity"` // "critical", "major", "minor"
}

// ValidationWarning 验证警告
type ValidationWarning struct {
	Field   string `json:"field"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ValidationStatistics 验证统计信息
type ValidationStatistics struct {
	TotalValidations      int     `json:"total_validations"`
	SuccessfulValidations int     `json:"successful_validations"`
	FailedValidations     int     `json:"failed_validations"`
	AutoCorrectionsMade   int     `json:"auto_corrections_made"`
	SuccessRate           float64 `json:"success_rate"`
	CommonErrors          map[string]int `json:"common_errors"`
	LastUpdated           time.Time `json:"last_updated"`
}

// NewJSONSchemaValidator 创建JSON schema验证器
func NewJSONSchemaValidator() *JSONSchemaValidator {
	validator := &JSONSchemaValidator{
		schemas:               make(map[string]*FieldSchema),
		strictMode:           true,
		logValidationErrors:  true,
		autoCorrectEnabled:   true,
		validationStatistics: &ValidationStatistics{
			CommonErrors: make(map[string]int),
			LastUpdated:  time.Now(),
		},
	}

	// 初始化标准schema
	validator.initializeStandardSchemas()
	
	return validator
}

// initializeStandardSchemas 初始化标准字段schemas
func (jsv *JSONSchemaValidator) initializeStandardSchemas() {
	// Gate2结构相关字段
	jsv.AddSchema(&FieldSchema{
		Name:        "struct_state_long",
		Type:        TypeEnum,
		Required:    true,
		AllowedValues: []string{"STRONG_LONG", "MODERATE_LONG", "WEAK_LONG", "NEUTRAL", "UNKNOWN"},
		Description: "Gate2长方向结构状态",
		Aliases:     []string{"struct_long", "structure_long", "long_state"},
	})

	jsv.AddSchema(&FieldSchema{
		Name:        "struct_state_short",
		Type:        TypeEnum,
		Required:    true,
		AllowedValues: []string{"STRONG_SHORT", "MODERATE_SHORT", "WEAK_SHORT", "NEUTRAL", "UNKNOWN"},
		Description: "Gate2短方向结构状态",
		Aliases:     []string{"struct_short", "structure_short", "short_state"},
	})

	// 价格相关字段
	jsv.AddSchema(&FieldSchema{
		Name:        "last_price",
		Type:        TypePrice,
		Required:    true,
		MinValue:    &[]float64{0.0}[0],
		Description: "最新价格 (统一字段)",
		Aliases:     []string{"current_price", "price", "latest_price"},
	})

	// 方向字段
	jsv.AddSchema(&FieldSchema{
		Name:        "direction",
		Type:        TypeDirection,
		Required:    false,
		AllowedValues: []string{"bullish", "bearish", "neutral", "unknown"},
		Description: "方向",
		Aliases:     []string{"dir", "trend_direction", "signal_direction"},
	})

	// 时间框架字段
	jsv.AddSchema(&FieldSchema{
		Name:        "timeframe",
		Type:        TypeTimeframe,
		Required:    false,
		AllowedValues: []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "8h", "12h", "1d"},
		Description: "时间框架",
		Aliases:     []string{"tf", "time_frame", "interval"},
	})

	// 质量评分字段（0-1量纲）
	jsv.AddSchema(&FieldSchema{
		Name:        "overall_score",
		Type:        TypeFloat,
		Required:    false,
		MinValue:    &[]float64{0.0}[0],
		MaxValue:    &[]float64{1.0}[0],
		Description: "总体评分 (0-1量纲)",
		Aliases:     []string{"quality_score", "total_score", "overall_quality"},
	})

	jsv.AddSchema(&FieldSchema{
		Name:        "data_quality",
		Type:        TypeFloat,
		Required:    false,
		MinValue:    &[]float64{0.0}[0],
		MaxValue:    &[]float64{1.0}[0],
		Description: "数据质量 (0-1量纲)",
		Aliases:     []string{"quality", "data_score"},
	})

	// 置信度字段
	jsv.AddSchema(&FieldSchema{
		Name:        "confidence",
		Type:        TypeFloat,
		Required:    false,
		MinValue:    &[]float64{0.0}[0],
		MaxValue:    &[]float64{1.0}[0],
		Description: "置信度 (0-1)",
		Aliases:     []string{"confidence_level", "signal_confidence"},
	})

	// 锚点相关字段
	jsv.AddSchema(&FieldSchema{
		Name:        "anchor_score",
		Type:        TypeFloat,
		Required:    false,
		MinValue:    &[]float64{0.0}[0],
		MaxValue:    &[]float64{1.0}[0],
		Description: "锚点评分 (0-1)",
		Aliases:     []string{"anchor_quality", "anchor_strength"},
	})

	// 订单流意图字段
	jsv.AddSchema(&FieldSchema{
		Name:        "candle_intent",
		Type:        TypeEnum,
		Required:    false,
		AllowedValues: []string{
			"strong_buy", "moderate_buy", "weak_buy",
			"strong_sell", "moderate_sell", "weak_sell",
			"neutral", "data_insufficient", "unknown",
		},
		Description: "K线意图分析结果",
		Aliases:     []string{"intent", "candle_signal", "order_flow_intent"},
	})

	// 趋势对齐字段
	jsv.AddSchema(&FieldSchema{
		Name:        "trend_alignment",
		Type:        TypeEnum,
		Required:    false,
		AllowedValues: []string{
			"bullish_aligned", "bearish_aligned", 
			"neutral_consolidation", "divergent_mixed", "unknown",
		},
		Description: "趋势对齐状态",
		Aliases:     []string{"alignment", "trend_consistency"},
	})

	// 市场制度字段
	jsv.AddSchema(&FieldSchema{
		Name:        "market_regime",
		Type:        TypeEnum,
		Required:    false,
		AllowedValues: []string{
			"trending_up", "trending_down", "ranging", 
			"volatile", "consolidation", "unknown",
		},
		Description: "市场制度",
		Aliases:     []string{"regime", "market_state", "market_mode"},
	})

	// Gate2专用字段 - 锚点相关
	jsv.AddSchema(&FieldSchema{
		Name:        "best_anchor_long",
		Type:        TypeObject,
		Required:    false,
		Description: "最佳长方向锚点",
	})

	jsv.AddSchema(&FieldSchema{
		Name:        "best_anchor_short", 
		Type:        TypeObject,
		Required:    false,
		Description: "最佳短方向锚点",
	})

	jsv.AddSchema(&FieldSchema{
		Name:        "top_anchors_long",
		Type:        TypeArray,
		Required:    false,
		Description: "顶级长方向锚点列表",
	})

	jsv.AddSchema(&FieldSchema{
		Name:        "top_anchors_short",
		Type:        TypeArray,
		Required:    false,
		Description: "顶级短方向锚点列表",
	})

	// 分析状态和上下文字段
	jsv.AddSchema(&FieldSchema{
		Name:        "status",
		Type:        TypeEnum,
		Required:    false,
		AllowedValues: []string{"active", "inactive", "pending", "completed", "error"},
		Description: "分析状态",
	})

	jsv.AddSchema(&FieldSchema{
		Name:        "trigger_context",
		Type:        TypeObject,
		Required:    false,
		Description: "触发上下文信息",
	})

	jsv.AddSchema(&FieldSchema{
		Name:        "anchor_score_breakdown",
		Type:        TypeObject,
		Required:    false,
		Description: "锚点评分详细分解",
	})

	// 交易对符号字段 (支持动态交易对名称)
	jsv.AddSchema(&FieldSchema{
		Name:        "symbol",
		Type:        TypeString,
		Required:    false,
		Description: "交易对符号",
		Aliases:     []string{"pair", "trading_pair"},
	})

	log.Printf("✅ [P1-2] JSON Schema验证器初始化完成，已注册 %d 个标准字段schema", len(jsv.schemas))
}

// AddSchema 添加字段schema
func (jsv *JSONSchemaValidator) AddSchema(schema *FieldSchema) {
	if schema.Name == "" {
		return
	}
	
	// 编译正则表达式
	if schema.Type == TypeString && schema.Pattern == nil {
		// 为字符串类型设置基础验证模式
		switch schema.Name {
		case "candle_intent", "trend_alignment", "market_regime":
			// 对于枚举字段，不需要正则验证
		default:
			// 基础字符串验证：不允许特殊字符
			pattern, err := regexp.Compile(`^[a-zA-Z0-9_\-\.]+$`)
			if err == nil {
				schema.Pattern = pattern
			}
		}
	}
	
	jsv.schemas[schema.Name] = schema
	
	// 为别名也注册schema
	for _, alias := range schema.Aliases {
		jsv.schemas[alias] = schema
	}
}

// ValidateJSON 验证JSON数据
func (jsv *JSONSchemaValidator) ValidateJSON(data interface{}) *ValidationResult {
	result := &ValidationResult{
		IsValid:         true,
		Errors:          make([]ValidationError, 0),
		Warnings:        make([]ValidationWarning, 0),
		CorrectedFields: make(map[string]interface{}),
		ValidationTime:  time.Now(),
		OriginalData:    data,
	}

	// 更新统计信息
	jsv.validationStatistics.TotalValidations++
	
	// 根据数据类型进行验证
	switch v := data.(type) {
	case map[string]interface{}:
		jsv.validateObject(v, result)
	case []interface{}:
		jsv.validateArray(v, result)
	default:
		result.Errors = append(result.Errors, ValidationError{
			Field:     "root",
			ErrorType: "invalid_type",
			Message:   "Expected object or array at root level",
			Severity:  "critical",
		})
		result.IsValid = false
	}

	// 应用自动纠错
	if jsv.autoCorrectEnabled && len(result.CorrectedFields) > 0 {
		result.ProcessedData = jsv.applyCorrections(data, result.CorrectedFields)
		jsv.validationStatistics.AutoCorrectionsMade += len(result.CorrectedFields)
	} else {
		result.ProcessedData = data
	}

	// 更新统计信息
	if result.IsValid {
		jsv.validationStatistics.SuccessfulValidations++
	} else {
		jsv.validationStatistics.FailedValidations++
		// 统计常见错误
		for _, err := range result.Errors {
			jsv.validationStatistics.CommonErrors[err.ErrorType]++
		}
	}
	
	jsv.validationStatistics.SuccessRate = float64(jsv.validationStatistics.SuccessfulValidations) / 
		float64(jsv.validationStatistics.TotalValidations) * 100
	jsv.validationStatistics.LastUpdated = time.Now()

	// 记录验证结果
	if jsv.logValidationErrors && (!result.IsValid || len(result.Warnings) > 0) {
		jsv.logValidationResult(result)
	}

	return result
}

// validateObject 验证对象
func (jsv *JSONSchemaValidator) validateObject(obj map[string]interface{}, result *ValidationResult) {
	for field, value := range obj {
		// 特殊处理：交易对符号字段 (如 BTCUSDT, ETHUSDT, HYPEUSDT等)
		if jsv.isLikelyTradingPairSymbol(field) {
			// 将其视为symbol字段处理，不报告为未知字段
			if symbolSchema, exists := jsv.schemas["symbol"]; exists {
				jsv.validateFieldValue("symbol", value, symbolSchema, result)
			}
			continue
		}
		
		// 检查字段拼写
		correctedField := jsv.checkFieldSpelling(field)
		if correctedField != field {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Field:   field,
				Type:    "spelling_correction",
				Message: fmt.Sprintf("建议字段名修正: '%s' -> '%s'", field, correctedField),
			})
			
			if jsv.autoCorrectEnabled {
				result.CorrectedFields[field] = correctedField
				field = correctedField // 使用纠正后的字段名继续验证
			}
		}

		// 获取字段schema
		schema, exists := jsv.schemas[field]
		if !exists {
			if jsv.strictMode {
				result.Errors = append(result.Errors, ValidationError{
					Field:     field,
					ErrorType: "unknown_field",
					Message:   fmt.Sprintf("未知字段: %s", field),
					Severity:  "minor",
				})
			}
			continue
		}

		// 验证字段值
		jsv.validateFieldValue(field, value, schema, result)

		// 递归验证嵌套对象
		if nestedObj, ok := value.(map[string]interface{}); ok && schema.Type == TypeObject {
			jsv.validateObject(nestedObj, result)
		}
	}

	// 检查必需字段
	jsv.checkRequiredFields(obj, result)
}

// validateArray 验证数组
func (jsv *JSONSchemaValidator) validateArray(arr []interface{}, result *ValidationResult) {
	for i, item := range arr {
		if itemObj, ok := item.(map[string]interface{}); ok {
			jsv.validateObject(itemObj, result)
		} else if itemArr, ok := item.([]interface{}); ok {
			jsv.validateArray(itemArr, result)
		}
		// 对于基础类型的数组元素，可以添加额外的验证逻辑
		_ = i // 避免未使用变量警告
	}
}

// validateFieldValue 验证字段值
func (jsv *JSONSchemaValidator) validateFieldValue(field string, value interface{}, schema *FieldSchema, result *ValidationResult) {
	// 类型验证
	if !jsv.validateFieldType(field, value, schema, result) {
		return
	}

	// 枚举值验证
	if schema.Type == TypeEnum && len(schema.AllowedValues) > 0 {
		jsv.validateEnumValue(field, value, schema, result)
	}

	// 数值范围验证
	if (schema.Type == TypeFloat || schema.Type == TypeInt || schema.Type == TypePrice || schema.Type == TypePercentage) {
		jsv.validateNumericRange(field, value, schema, result)
	}

	// 正则表达式验证
	if schema.Type == TypeString && schema.Pattern != nil {
		jsv.validatePattern(field, value, schema, result)
	}
}

// validateFieldType 验证字段类型
func (jsv *JSONSchemaValidator) validateFieldType(field string, value interface{}, schema *FieldSchema, result *ValidationResult) bool {
	valueType := reflect.TypeOf(value)
	
	switch schema.Type {
	case TypeString, TypeEnum, TypeDirection, TypeTimeframe:
		if valueType.Kind() != reflect.String {
			result.Errors = append(result.Errors, ValidationError{
				Field:         field,
				ErrorType:     "type_mismatch",
				Message:       fmt.Sprintf("字段 %s 期望类型为 string，实际为 %v", field, valueType),
				ExpectedValue: "string",
				ActualValue:   valueType.String(),
				Severity:      "major",
			})
			return false
		}
		
	case TypeFloat, TypePrice, TypePercentage:
		if valueType.Kind() != reflect.Float64 && valueType.Kind() != reflect.Int {
			result.Errors = append(result.Errors, ValidationError{
				Field:         field,
				ErrorType:     "type_mismatch", 
				Message:       fmt.Sprintf("字段 %s 期望类型为 number，实际为 %v", field, valueType),
				ExpectedValue: "number",
				ActualValue:   valueType.String(),
				Severity:      "major",
			})
			return false
		}
		
	case TypeInt:
		if valueType.Kind() != reflect.Int && valueType.Kind() != reflect.Int64 && valueType.Kind() != reflect.Float64 {
			result.Errors = append(result.Errors, ValidationError{
				Field:         field,
				ErrorType:     "type_mismatch",
				Message:       fmt.Sprintf("字段 %s 期望类型为 int，实际为 %v", field, valueType),
				ExpectedValue: "int",
				ActualValue:   valueType.String(),
				Severity:      "major",
			})
			return false
		}
		
	case TypeBool:
		if valueType.Kind() != reflect.Bool {
			result.Errors = append(result.Errors, ValidationError{
				Field:         field,
				ErrorType:     "type_mismatch",
				Message:       fmt.Sprintf("字段 %s 期望类型为 bool，实际为 %v", field, valueType),
				ExpectedValue: "bool",
				ActualValue:   valueType.String(),
				Severity:      "major",
			})
			return false
		}
	}
	
	return true
}

// validateEnumValue 验证枚举值
func (jsv *JSONSchemaValidator) validateEnumValue(field string, value interface{}, schema *FieldSchema, result *ValidationResult) {
	strValue, ok := value.(string)
	if !ok {
		return
	}

	for _, allowedValue := range schema.AllowedValues {
		if strValue == allowedValue {
			return
		}
	}

	// 尝试拼写纠错
	correctedValue := jsv.findClosestMatch(strValue, schema.AllowedValues)
	if correctedValue != strValue {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Field:   field,
			Type:    "enum_correction",
			Message: fmt.Sprintf("枚举值建议修正: '%s' -> '%s'", strValue, correctedValue),
		})
		
		if jsv.autoCorrectEnabled {
			result.CorrectedFields[field] = correctedValue
		}
	} else {
		result.Errors = append(result.Errors, ValidationError{
			Field:         field,
			ErrorType:     "invalid_enum_value",
			Message:       fmt.Sprintf("字段 %s 值 '%s' 不在允许列表中: %v", field, strValue, schema.AllowedValues),
			ExpectedValue: schema.AllowedValues,
			ActualValue:   strValue,
			Severity:      "major",
		})
	}
}

// validateNumericRange 验证数值范围
func (jsv *JSONSchemaValidator) validateNumericRange(field string, value interface{}, schema *FieldSchema, result *ValidationResult) {
	var floatValue float64
	
	switch v := value.(type) {
	case float64:
		floatValue = v
	case int:
		floatValue = float64(v)
	case int64:
		floatValue = float64(v)
	default:
		return
	}

	if schema.MinValue != nil && floatValue < *schema.MinValue {
		result.Errors = append(result.Errors, ValidationError{
			Field:         field,
			ErrorType:     "value_too_small",
			Message:       fmt.Sprintf("字段 %s 值 %.4f 小于最小值 %.4f", field, floatValue, *schema.MinValue),
			ExpectedValue: fmt.Sprintf(">= %.4f", *schema.MinValue),
			ActualValue:   floatValue,
			Severity:      "major",
		})
	}

	if schema.MaxValue != nil && floatValue > *schema.MaxValue {
		result.Errors = append(result.Errors, ValidationError{
			Field:         field,
			ErrorType:     "value_too_large",
			Message:       fmt.Sprintf("字段 %s 值 %.4f 大于最大值 %.4f", field, floatValue, *schema.MaxValue),
			ExpectedValue: fmt.Sprintf("<= %.4f", *schema.MaxValue),
			ActualValue:   floatValue,
			Severity:      "major",
		})
	}
}

// validatePattern 验证正则表达式
func (jsv *JSONSchemaValidator) validatePattern(field string, value interface{}, schema *FieldSchema, result *ValidationResult) {
	strValue, ok := value.(string)
	if !ok {
		return
	}

	if !schema.Pattern.MatchString(strValue) {
		result.Errors = append(result.Errors, ValidationError{
			Field:         field,
			ErrorType:     "pattern_mismatch",
			Message:       fmt.Sprintf("字段 %s 值 '%s' 不匹配预期模式", field, strValue),
			ExpectedValue: schema.Pattern.String(),
			ActualValue:   strValue,
			Severity:      "minor",
		})
	}
}

// checkRequiredFields 检查必需字段
func (jsv *JSONSchemaValidator) checkRequiredFields(obj map[string]interface{}, result *ValidationResult) {
	// 收集所有主schema（非别名）
	processedSchemas := make(map[string]*FieldSchema)
	
	for schemaName, schema := range jsv.schemas {
		// 只处理主字段名，跳过别名
		if schemaName == schema.Name {
			processedSchemas[schemaName] = schema
		}
	}
	
	// 检查必需的主字段
	for schemaName, schema := range processedSchemas {
		if schema.Required {
			// 检查主字段名或任何别名是否存在
			fieldFound := false
			
			// 检查主字段名
			if _, exists := obj[schemaName]; exists {
				fieldFound = true
			}
			
			// 检查别名字段
			if !fieldFound {
				for _, alias := range schema.Aliases {
					if _, aliasExists := obj[alias]; aliasExists {
						fieldFound = true
						break
					}
				}
			}
			
			// 如果既没有主字段也没有别名字段，则报错
			if !fieldFound {
				result.Errors = append(result.Errors, ValidationError{
					Field:     schemaName,
					ErrorType: "missing_required_field",
					Message:   fmt.Sprintf("缺少必需字段: %s (可接受别名: %v)", schemaName, schema.Aliases),
					Severity:  "critical",
				})
				result.IsValid = false
			}
		}
	}
}

// checkFieldSpelling 检查字段拼写
func (jsv *JSONSchemaValidator) checkFieldSpelling(field string) string {
	// 如果字段名已存在，直接返回
	if _, exists := jsv.schemas[field]; exists {
		return field
	}

	// 尝试在所有已知字段中找到最相似的
	var allFieldNames []string
	for name := range jsv.schemas {
		allFieldNames = append(allFieldNames, name)
	}

	return jsv.findClosestMatch(field, allFieldNames)
}

// findClosestMatch 寻找最相似的匹配
func (jsv *JSONSchemaValidator) findClosestMatch(target string, candidates []string) string {
	if len(candidates) == 0 {
		return target
	}

	bestMatch := target
	bestScore := -1

	for _, candidate := range candidates {
		score := jsv.calculateSimilarity(target, candidate)
		if score > bestScore {
			bestScore = score
			bestMatch = candidate
		}
	}

	// 只有当相似度超过60%时才建议修正
	if bestScore > 60 {
		return bestMatch
	}

	return target
}

// isLikelyTradingPairSymbol 判断字段名是否可能是交易对符号
func (jsv *JSONSchemaValidator) isLikelyTradingPairSymbol(field string) bool {
	// 常见的交易对模式：
	// 1. 以USDT结尾 (如 BTCUSDT, ETHUSDT, HYPEUSDT)
	// 2. 以USDC结尾 (如 BTCUSDC, ETHUSDC)
	// 3. 以BTC结尾 (如 ETHBTC, ADABTC)
	// 4. 3-8个字符的大写字母组合
	
	if len(field) < 3 || len(field) > 12 {
		return false
	}
	
	// 检查是否全为大写字母
	for _, char := range field {
		if char < 'A' || char > 'Z' {
			return false
		}
	}
	
	// 检查常见的交易对后缀
	commonSuffixes := []string{"USDT", "USDC", "BTC", "ETH", "BNB"}
	for _, suffix := range commonSuffixes {
		if strings.HasSuffix(field, suffix) && len(field) > len(suffix) {
			return true
		}
	}
	
	// 如果是3-6字符的纯大写字母，可能是代币符号
	if len(field) >= 3 && len(field) <= 6 {
		return true
	}
	
	return false
}

// calculateSimilarity 计算字符串相似度（简化版Levenshtein距离）
func (jsv *JSONSchemaValidator) calculateSimilarity(s1, s2 string) int {
	if s1 == s2 {
		return 100
	}

	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	if s1 == s2 {
		return 95 // 大小写不同但内容相同
	}

	// 计算编辑距离
	distance := jsv.levenshteinDistance(s1, s2)
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}

	if maxLen == 0 {
		return 100
	}

	similarity := (1.0 - float64(distance)/float64(maxLen)) * 100
	return int(similarity)
}

// levenshteinDistance 计算编辑距离
func (jsv *JSONSchemaValidator) levenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	len1, len2 := len(r1), len(r2)

	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 1
			if r1[i-1] == r2[j-1] {
				cost = 0
			}

			matrix[i][j] = minInt(
				minInt(matrix[i-1][j]+1, matrix[i][j-1]+1),  // min of deletion and insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len1][len2]
}

// applyCorrections 应用纠错
func (jsv *JSONSchemaValidator) applyCorrections(data interface{}, corrections map[string]interface{}) interface{} {
	if obj, ok := data.(map[string]interface{}); ok {
		corrected := make(map[string]interface{})
		
		for field, value := range obj {
			if correctedField, exists := corrections[field]; exists {
				corrected[correctedField.(string)] = value
			} else {
				corrected[field] = value
			}
		}
		
		return corrected
	}
	
	return data
}

// logValidationResult 记录验证结果
func (jsv *JSONSchemaValidator) logValidationResult(result *ValidationResult) {
	if !result.IsValid {
		log.Printf("❌ [P1-2 JSON验证] 验证失败，发现 %d 个错误:", len(result.Errors))
		for _, err := range result.Errors {
			log.Printf("   - [%s] %s: %s", err.Severity, err.Field, err.Message)
		}
	}

	if len(result.Warnings) > 0 {
		log.Printf("⚠️ [P1-2 JSON验证] 发现 %d 个警告:", len(result.Warnings))
		for _, warning := range result.Warnings {
			log.Printf("   - [%s] %s: %s", warning.Type, warning.Field, warning.Message)
		}
	}

	if len(result.CorrectedFields) > 0 {
		log.Printf("🔧 [P1-2 自动纠错] 已纠正 %d 个字段:", len(result.CorrectedFields))
		for original, corrected := range result.CorrectedFields {
			log.Printf("   - '%s' -> '%s'", original, corrected)
		}
	}
}

// GetValidationStatistics 获取验证统计信息
func (jsv *JSONSchemaValidator) GetValidationStatistics() *ValidationStatistics {
	return jsv.validationStatistics
}

// SetStrictMode 设置严格模式
func (jsv *JSONSchemaValidator) SetStrictMode(strict bool) {
	jsv.strictMode = strict
}

// SetAutoCorrection 设置自动纠错
func (jsv *JSONSchemaValidator) SetAutoCorrection(enabled bool) {
	jsv.autoCorrectEnabled = enabled
}

// ExportSchemaAsJSON 导出schema为JSON
func (jsv *JSONSchemaValidator) ExportSchemaAsJSON() (string, error) {
	// 创建导出用的简化schema结构
	exportSchemas := make(map[string]interface{})
	
	processed := make(map[string]bool) // 避免别名重复
	
	for _, schema := range jsv.schemas {
		if processed[schema.Name] {
			continue // 跳过已处理的schema（避免别名重复）
		}
		
		exportSchema := map[string]interface{}{
			"name":        schema.Name,
			"type":        string(schema.Type),
			"required":    schema.Required,
			"description": schema.Description,
		}
		
		if len(schema.AllowedValues) > 0 {
			exportSchema["allowed_values"] = schema.AllowedValues
		}
		
		if schema.MinValue != nil {
			exportSchema["min_value"] = *schema.MinValue
		}
		
		if schema.MaxValue != nil {
			exportSchema["max_value"] = *schema.MaxValue
		}
		
		if len(schema.Aliases) > 0 {
			exportSchema["aliases"] = schema.Aliases
		}
		
		exportSchemas[schema.Name] = exportSchema
		processed[schema.Name] = true
	}
	
	jsonData, err := json.MarshalIndent(exportSchemas, "", "  ")
	if err != nil {
		return "", err
	}
	
	return string(jsonData), nil
}

// minInt function is defined in utils.go

// ValidateGate2Output 专门验证Gate2结构输出
func (jsv *JSONSchemaValidator) ValidateGate2Output(data map[string]interface{}) *ValidationResult {
	// Gate2输出的关键字段验证
	requiredGate2Fields := []string{
		"struct_state_long", "struct_state_short", 
		"last_price",
	}
	
	// 临时设置必需字段
	originalRequired := make(map[string]bool)
	for _, field := range requiredGate2Fields {
		if schema, exists := jsv.schemas[field]; exists {
			originalRequired[field] = schema.Required
			schema.Required = true
		}
	}
	
	// 执行验证
	result := jsv.ValidateJSON(data)
	
	// 恢复原始required设置
	for field, wasRequired := range originalRequired {
		if schema, exists := jsv.schemas[field]; exists {
			schema.Required = wasRequired
		}
	}
	
	return result
}

// ValidateOrderFlowOutput 专门验证订单流输出
func (jsv *JSONSchemaValidator) ValidateOrderFlowOutput(data map[string]interface{}) *ValidationResult {
	// 订单流输出的关键字段验证
	requiredOrderFlowFields := []string{
		"candle_intent", "data_quality", "overall_score",
	}
	
	// 临时设置必需字段
	originalRequired := make(map[string]bool)
	for _, field := range requiredOrderFlowFields {
		if schema, exists := jsv.schemas[field]; exists {
			originalRequired[field] = schema.Required
			schema.Required = true
		}
	}
	
	// 执行验证
	result := jsv.ValidateJSON(data)
	
	// 恢复原始required设置
	for field, wasRequired := range originalRequired {
		if schema, exists := jsv.schemas[field]; exists {
			schema.Required = wasRequired
		}
	}
	
	return result
}

// 全局验证器实例
var globalJSONValidator = NewJSONSchemaValidator()

// GetGlobalJSONValidator 获取全局JSON验证器
func GetGlobalJSONValidator() *JSONSchemaValidator {
	return globalJSONValidator
}

// ValidateAIOutput 验证AI输出数据的通用接口
func ValidateAIOutput(data interface{}, outputType string) *ValidationResult {
	validator := GetGlobalJSONValidator()
	
	switch outputType {
	case "gate2":
		if objData, ok := data.(map[string]interface{}); ok {
			return validator.ValidateGate2Output(objData)
		}
	case "orderflow":
		if objData, ok := data.(map[string]interface{}); ok {
			return validator.ValidateOrderFlowOutput(objData)
		}
	default:
		return validator.ValidateJSON(data)
	}
	
	return validator.ValidateJSON(data)
}

// LogValidationSummary 记录验证统计摘要
func LogValidationSummary() {
	stats := globalJSONValidator.GetValidationStatistics()
	
	log.Printf("📊 [P1-2 JSON验证统计]")
	log.Printf("   总验证次数: %d", stats.TotalValidations)
	log.Printf("   成功率: %.1f%%", stats.SuccessRate)
	log.Printf("   自动纠错次数: %d", stats.AutoCorrectionsMade)
	
	if len(stats.CommonErrors) > 0 {
		log.Printf("   常见错误类型:")
		
		// 按错误频次排序
		type errorFreq struct {
			errorType string
			count     int
		}
		
		var sortedErrors []errorFreq
		for errType, count := range stats.CommonErrors {
			sortedErrors = append(sortedErrors, errorFreq{errType, count})
		}
		
		sort.Slice(sortedErrors, func(i, j int) bool {
			return sortedErrors[i].count > sortedErrors[j].count
		})
		
		for i, err := range sortedErrors {
			if i >= 5 { // 只显示前5个最常见的错误
				break
			}
			log.Printf("     %d. %s: %d次", i+1, err.errorType, err.count)
		}
	}
}