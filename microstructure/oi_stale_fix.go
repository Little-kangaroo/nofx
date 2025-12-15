package microstructure

import (
	"fmt"
	"log"
	"time"
)

// ===== P0-08修复：OI阈值配置管理 =====

// OIStaleConfig OI过期配置（P0-08修复）
type OIStaleConfig struct {
	// 🔥 P0-08修复：明确的阈值定义，消除注释与代码不一致
	StaleThresholdMinutes    int     `json:"stale_threshold_minutes"`    // 过期阈值（分钟）
	CriticalThresholdMinutes int     `json:"critical_threshold_minutes"` // 严重过期阈值（分钟）
	WSUpdateIntervalSeconds  int     `json:"ws_update_interval_seconds"` // WebSocket更新间隔（秒）
	
	// 配置验证
	ConfigDescription       string  `json:"config_description"`         // 配置说明
	RecommendedThreshold    string  `json:"recommended_threshold"`      // 推荐阈值说明
}

// DefaultOIStaleConfig 获取默认OI过期配置（P0-08修复）
func DefaultOIStaleConfig() *OIStaleConfig {
	return &OIStaleConfig{
		// 🔥 P0-08修复：基于实际业务需求设定合理阈值
		StaleThresholdMinutes:    3,    // OI数据30秒更新，3分钟无更新标记为过期
		CriticalThresholdMinutes: 10,   // 10分钟无更新标记为严重过期
		WSUpdateIntervalSeconds:  30,   // OI WebSocket 30秒更新频率
		
		ConfigDescription:       "OI数据过期判断配置，基于30秒WS更新频率",
		RecommendedThreshold:    "建议stale阈值为2-3分钟，给网络延迟和重连留足缓冲",
	}
}

// ValidateOIStaleConfig 验证OI配置合理性（P0-08修复）
func (config *OIStaleConfig) ValidateOIStaleConfig() error {
	// 基本合理性检查
	if config.StaleThresholdMinutes <= 0 {
		return fmt.Errorf("stale阈值必须大于0分钟，当前: %d", config.StaleThresholdMinutes)
	}
	
	if config.CriticalThresholdMinutes <= config.StaleThresholdMinutes {
		return fmt.Errorf("严重过期阈值必须大于普通过期阈值，当前: critical=%d, stale=%d", 
			config.CriticalThresholdMinutes, config.StaleThresholdMinutes)
	}
	
	// 业务逻辑合理性检查
	if config.WSUpdateIntervalSeconds <= 0 || config.WSUpdateIntervalSeconds > 300 {
		return fmt.Errorf("WS更新间隔应在1-300秒之间，当前: %d", config.WSUpdateIntervalSeconds)
	}
	
	// 阈值与更新频率的合理性
	minRecommendedStaleMinutes := (config.WSUpdateIntervalSeconds * 3) / 60 // 至少3倍更新间隔
	if config.StaleThresholdMinutes < minRecommendedStaleMinutes {
		log.Printf("⚠️ [P0-08] OI过期阈值可能过短：当前%d分钟，建议至少%d分钟（基于%d秒更新间隔）",
			config.StaleThresholdMinutes, minRecommendedStaleMinutes, config.WSUpdateIntervalSeconds)
	}
	
	return nil
}

// GetStaleThreshold 获取过期阈值时间（P0-08修复）
func (config *OIStaleConfig) GetStaleThreshold() time.Duration {
	return time.Duration(config.StaleThresholdMinutes) * time.Minute
}

// GetCriticalThreshold 获取严重过期阈值时间（P0-08修复）
func (config *OIStaleConfig) GetCriticalThreshold() time.Duration {
	return time.Duration(config.CriticalThresholdMinutes) * time.Minute
}

// ===== P0-08修复：OI数据健康状态 =====

// OIDataHealth OI数据健康状态（P0-08修复）
type OIDataHealth struct {
	HasData              bool      `json:"has_data"`              // 🔥 P0-08修复：明确是否有数据
	LastRealUpdate       time.Time `json:"last_real_update"`       // 🔥 P0-08修复：真实数据更新时间（零值表示无数据）
	DataAgeMinutes       float64   `json:"data_age_minutes"`       // 数据年龄（分钟）
	IsStale              bool      `json:"is_stale"`               // 是否过期
	IsCriticallyStale    bool      `json:"is_critically_stale"`    // 是否严重过期
	StalenessLevel       string    `json:"staleness_level"`        // 过期等级：fresh, stale, critical, no_data
	QualityWeight        float64   `json:"quality_weight"`         // 质量权重 (0.0-1.0)
	HealthCheckTime      time.Time `json:"health_check_time"`      // 健康检查时间
}

// CalculateOIDataHealth 计算OI数据健康状态（P0-08修复）
func CalculateOIDataHealth(lastUpdate time.Time, hasData bool, config *OIStaleConfig) OIDataHealth {
	now := time.Now()
	
	// 🔥 P0-08修复：零值时间表示无数据
	if !hasData || lastUpdate.IsZero() {
		return OIDataHealth{
			HasData:              false,
			LastRealUpdate:       time.Time{}, // 🔥 P0-08修复：零值时间，不使用time.Now()
			DataAgeMinutes:       -1,          // 无数据时年龄为-1
			IsStale:              true,
			IsCriticallyStale:    true,
			StalenessLevel:       "no_data",
			QualityWeight:        0.0,         // 无数据时权重为0
			HealthCheckTime:      now,
		}
	}
	
	// 计算数据年龄
	dataAge := now.Sub(lastUpdate)
	dataAgeMinutes := dataAge.Minutes()
	
	// 判断过期状态
	staleThreshold := config.GetStaleThreshold()
	criticalThreshold := config.GetCriticalThreshold()
	
	isStale := dataAge > staleThreshold
	isCriticallyStale := dataAge > criticalThreshold
	
	// 确定过期等级
	var stalenessLevel string
	var qualityWeight float64
	
	if isCriticallyStale {
		stalenessLevel = "critical"
		qualityWeight = 0.1 // 严重过期时权重很低
	} else if isStale {
		stalenessLevel = "stale"
		qualityWeight = 0.5 // 过期时权重减半
	} else {
		stalenessLevel = "fresh"
		qualityWeight = 1.0 // 新鲜数据权重为1
	}
	
	return OIDataHealth{
		HasData:              true,
		LastRealUpdate:       lastUpdate,
		DataAgeMinutes:       dataAgeMinutes,
		IsStale:              isStale,
		IsCriticallyStale:    isCriticallyStale,
		StalenessLevel:       stalenessLevel,
		QualityWeight:        qualityWeight,
		HealthCheckTime:      now,
	}
}

// ===== P0-08修复：增强版OI分析 =====

// OIAnalysisV2EnhancedForP008 P0-08专用增强版OI分析结果
type OIAnalysisV2EnhancedForP008 struct {
	// 原有数据字段
	Current      float64 `json:"current"`
	Change1H     float64 `json:"change_1h"`
	Change4H     float64 `json:"change_4h"`
	ChangeRate1H float64 `json:"change_rate_1h"`
	ChangeRate4H float64 `json:"change_rate_4h"`
	Trend        string  `json:"trend"`
	
	// 🔥 P0-08修复：增强的健康状态
	DataHealth       OIDataHealth  `json:"data_health"`        // 🔥 P0-08修复：明确的数据健康状态
	ConfigUsed       *OIStaleConfig `json:"config_used"`       // 使用的配置
	
	// 🔥 P0-08修复：兼容性字段（向后兼容）
	LastUpdate       time.Time `json:"last_update"`         // 向后兼容
	IsStale          bool      `json:"is_stale"`            // 向后兼容
	
	// 元数据
	GenerationTime   time.Time `json:"generation_time"`     // 分析生成时间
	QualityGrade     string    `json:"quality_grade"`       // 质量等级：A, B, C, D, F
}

// ===== P0-08修复：OI计算器增强方法 =====

// GetOIAnalysisV2Enhanced 获取P0-08增强版OI分析
func (calc *OICalculator) GetOIAnalysisV2Enhanced(config *OIStaleConfig) *OIAnalysisV2EnhancedForP008 {
	calc.mu.RLock()
	defer calc.mu.RUnlock()
	
	now := time.Now()
	
	// 🔥 P0-08修复：计算数据健康状态
	hasData := len(calc.changes) > 0 && !calc.lastUpdate.IsZero()
	dataHealth := CalculateOIDataHealth(calc.lastUpdate, hasData, config)
	
	// 如果没有数据，返回明确的无数据状态
	if !dataHealth.HasData {
		return &OIAnalysisV2EnhancedForP008{
			Current:      0,
			Change1H:     0,
			Change4H:     0,
			ChangeRate1H: 0,
			ChangeRate4H: 0,
			Trend:        "no_data",
			
			// 🔥 P0-08修复：明确的健康状态
			DataHealth:     dataHealth,
			ConfigUsed:     config,
			
			// 🔥 P0-08修复：向后兼容字段，但使用零值时间
			LastUpdate:     time.Time{}, // 🔥 P0-08修复：零值时间，不使用time.Now()
			IsStale:        true,
			
			GenerationTime: now,
			QualityGrade:   "F", // 无数据为F级
		}
	}
	
	// 计算变化数据
	change1H, changeRate1H := calc.calculateChange(time.Hour)
	change4H, changeRate4H := calc.calculateChange(4 * time.Hour)
	trend := calc.determineTrend(changeRate1H)
	
	// 🔥 P0-08修复：确定质量等级
	qualityGrade := determineOIQualityGrade(dataHealth)
	
	return &OIAnalysisV2EnhancedForP008{
		Current:      calc.current,
		Change1H:     change1H,
		Change4H:     change4H,
		ChangeRate1H: changeRate1H,
		ChangeRate4H: changeRate4H,
		Trend:        trend,
		
		// 🔥 P0-08修复：增强的健康状态
		DataHealth:     dataHealth,
		ConfigUsed:     config,
		
		// 向后兼容字段
		LastUpdate:     calc.lastUpdate, // 使用真实时间
		IsStale:        dataHealth.IsStale,
		
		GenerationTime: now,
		QualityGrade:   qualityGrade,
	}
}

// determineOIQualityGrade 确定OI质量等级（P0-08修复）
func determineOIQualityGrade(health OIDataHealth) string {
	if !health.HasData {
		return "F" // 无数据
	}
	
	switch health.StalenessLevel {
	case "fresh":
		return "A" // 新鲜数据
	case "stale":
		if health.DataAgeMinutes <= 5 {
			return "B" // 轻微过期
		}
		return "C" // 中度过期
	case "critical":
		return "D" // 严重过期
	default:
		return "F" // 未知状态
	}
}

// ===== P0-08修复：OI管理器增强方法 =====

// GetOIAnalysisV2Enhanced 获取P0-08增强版OI分析
func (manager *OIManager) GetOIAnalysisV2Enhanced(symbol string, config *OIStaleConfig) *OIAnalysisV2EnhancedForP008 {
	if config == nil {
		config = DefaultOIStaleConfig()
	}
	
	// 验证配置
	if err := config.ValidateOIStaleConfig(); err != nil {
		log.Printf("❌ [P0-08] OI配置验证失败: %v, 使用默认配置", err)
		config = DefaultOIStaleConfig()
	}
	
	manager.mu.RLock()
	calc, exists := manager.calculators[symbol]
	manager.mu.RUnlock()
	
	if !exists {
		// 🔥 P0-08修复：无计算器时返回明确的无数据状态
		now := time.Now()
		return &OIAnalysisV2EnhancedForP008{
			Current:      0,
			Change1H:     0,
			Change4H:     0,
			ChangeRate1H: 0,
			ChangeRate4H: 0,
			Trend:        "no_data",
			
			// 🔥 P0-08修复：明确的无数据健康状态
			DataHealth: OIDataHealth{
				HasData:              false,
				LastRealUpdate:       time.Time{}, // 🔥 P0-08修复：零值时间
				DataAgeMinutes:       -1,
				IsStale:              true,
				IsCriticallyStale:    true,
				StalenessLevel:       "no_data",
				QualityWeight:        0.0,
				HealthCheckTime:      now,
			},
			ConfigUsed: config,
			
			// 🔥 P0-08修复：向后兼容字段，使用零值时间
			LastUpdate:     time.Time{}, // 不再使用time.Now()
			IsStale:        true,
			
			GenerationTime: now,
			QualityGrade:   "F",
		}
	}
	
	return calc.GetOIAnalysisV2Enhanced(config)
}

// ===== P0-08修复：生产环境验证工具 =====

// ValidateOIStaleThreshold 验证OI过期阈值配置（P0-08修复）
func ValidateOIStaleThreshold(manager *OIManager, symbols []string) map[string]interface{} {
	log.Printf("🔬 [P0-08验证] 开始OI过期阈值验证，检查%d个交易对", len(symbols))
	
	config := DefaultOIStaleConfig()
	results := map[string]interface{}{
		"validation_time":      time.Now().Format("15:04:05.000"),
		"config_used":          config,
		"symbols_checked":      len(symbols),
		"threshold_consistency": true,
		"no_data_handling":     true,
		"symbol_results":       map[string]interface{}{},
		"issues_found":         []string{},
	}
	
	issues := []string{}
	
	for _, symbol := range symbols {
		// 获取传统分析
		legacyAnalysis := manager.GetOIAnalysis(symbol)
		
		// 获取修复版分析
		enhancedAnalysis := manager.GetOIAnalysisV2Enhanced(symbol, config)
		
		symbolResult := map[string]interface{}{
			"symbol":                    symbol,
			"legacy_last_update":        legacyAnalysis.LastUpdate,
			"enhanced_last_update":      enhancedAnalysis.LastUpdate,
			"legacy_is_stale":          legacyAnalysis.IsStale,
			"enhanced_is_stale":        enhancedAnalysis.IsStale,
			"has_data":                 enhancedAnalysis.DataHealth.HasData,
			"staleness_level":          enhancedAnalysis.DataHealth.StalenessLevel,
			"quality_grade":            enhancedAnalysis.QualityGrade,
		}
		
		// 检查是否使用time.Now()作为LastUpdate但标记为stale
		if legacyAnalysis.IsStale && 
		   time.Since(legacyAnalysis.LastUpdate) < time.Minute && 
		   !enhancedAnalysis.DataHealth.HasData {
			issue := fmt.Sprintf("%s: 发现P0-08问题 - 无数据但LastUpdate使用time.Now()", symbol)
			issues = append(issues, issue)
			symbolResult["p008_issue_detected"] = true
		} else {
			symbolResult["p008_issue_detected"] = false
		}
		
		// 检查增强版是否正确使用零值时间
		if !enhancedAnalysis.DataHealth.HasData && !enhancedAnalysis.LastUpdate.IsZero() {
			issue := fmt.Sprintf("%s: 增强版无数据时应使用零值时间", symbol)
			issues = append(issues, issue)
		}
		
		results["symbol_results"].(map[string]interface{})[symbol] = symbolResult
	}
	
	results["issues_found"] = issues
	if len(issues) > 0 {
		results["threshold_consistency"] = false
		log.Printf("⚠️ [P0-08验证] 发现%d个问题: %v", len(issues), issues)
	} else {
		log.Printf("✅ [P0-08验证] OI过期阈值验证通过，未发现问题")
	}
	
	return results
}

// TestOIStreamInterruption 模拟OI数据流中断测试（P0-08修复）
func TestOIStreamInterruption(manager *OIManager, symbol string, interruptionMinutes int) {
	log.Printf("🔬 [P0-08测试] 模拟%s OI数据流中断%d分钟", symbol, interruptionMinutes)
	
	config := DefaultOIStaleConfig()
	
	// 记录中断前状态
	beforeAnalysis := manager.GetOIAnalysisV2Enhanced(symbol, config)
	log.Printf("📊 中断前: 数据状态=%s, 质量等级=%s, 有数据=%v", 
		beforeAnalysis.DataHealth.StalenessLevel, beforeAnalysis.QualityGrade, beforeAnalysis.DataHealth.HasData)
	
	// 等待指定时间模拟数据中断
	log.Printf("⏱️ 等待%d分钟模拟数据中断...", interruptionMinutes)
	time.Sleep(time.Duration(interruptionMinutes) * time.Minute)
	
	// 检查中断后状态
	afterAnalysis := manager.GetOIAnalysisV2Enhanced(symbol, config)
	log.Printf("📊 中断后: 数据状态=%s, 质量等级=%s, 数据年龄=%.1f分钟", 
		afterAnalysis.DataHealth.StalenessLevel, afterAnalysis.QualityGrade, afterAnalysis.DataHealth.DataAgeMinutes)
	
	// 验证阈值触发的正确性
	if interruptionMinutes >= config.StaleThresholdMinutes {
		if afterAnalysis.DataHealth.StalenessLevel == "fresh" {
			log.Printf("❌ [P0-08验证] 失败：%d分钟中断后数据仍标记为fresh", interruptionMinutes)
		} else {
			log.Printf("✅ [P0-08验证] 成功：%d分钟中断后正确标记为%s", 
				interruptionMinutes, afterAnalysis.DataHealth.StalenessLevel)
		}
	}
	
	if interruptionMinutes >= config.CriticalThresholdMinutes {
		if afterAnalysis.DataHealth.StalenessLevel != "critical" {
			log.Printf("❌ [P0-08验证] 失败：%d分钟中断后应标记为critical", interruptionMinutes)
		} else {
			log.Printf("✅ [P0-08验证] 成功：%d分钟中断后正确标记为critical", interruptionMinutes)
		}
	}
}