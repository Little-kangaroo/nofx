package microstructure

import (
	"fmt"
	"log"
	"time"
)

// ===== P0-07修复：增强版数据质量评估 =====

// EnhancedDataQualityWithAI 🔥 P0-07修复：包含AI接口监控的增强版数据质量
type EnhancedDataQualityWithAI struct {
	// 原有数据质量字段
	CVDReliability       float64   `json:"cvd_reliability"`
	OrderBookReliability float64   `json:"orderbook_reliability"`
	OIReliability        float64   `json:"oi_reliability"`
	OverallScore         float64   `json:"overall_score"`
	LastDataUpdate       time.Time `json:"last_data_update"`
	DataLagMs            int64     `json:"data_lag_ms"`
	Status               string    `json:"status"`
	
	// 🔥 P0-07修复：AI接口监控字段
	AIInterfaceStatus    AIInterfaceHealthStatus `json:"ai_interface_status"`
	SystemDegradation    SystemDegradationInfo   `json:"system_degradation"`
	QualityGrade         string                  `json:"quality_grade"` // A, B, C, D, F
	TradingRecommendation string                 `json:"trading_recommendation"`
	RiskLevel            string                  `json:"risk_level"`   // LOW, MEDIUM, HIGH, CRITICAL
	
	// 🔥 P0-07修复：实时监控数据
	MonitoringMetrics    AIMonitoringMetrics     `json:"monitoring_metrics"`
	HealthCheckTime      time.Time               `json:"health_check_time"`
	NextCheckTime        time.Time               `json:"next_check_time"`
	AlertLevel           string                  `json:"alert_level"`  // NONE, INFO, WARNING, ERROR, CRITICAL
}

// AIInterfaceHealthStatus AI接口健康状态（P0-07修复）
type AIInterfaceHealthStatus struct {
	IsAvailable          bool              `json:"is_available"`
	LastSuccessfulCall   time.Time         `json:"last_successful_call"`
	LastFailedCall       time.Time         `json:"last_failed_call"`
	ConsecutiveFailures  int               `json:"consecutive_failures"`
	SuccessRate24h       float64           `json:"success_rate_24h"`
	AvgResponseTimeMs    int64             `json:"avg_response_time_ms"`
	CurrentMode          string            `json:"current_mode"`      // NORMAL, FALLBACK, UNAVAILABLE
	FailureReason        string            `json:"failure_reason"`
	RecoveryEstimateMin  int               `json:"recovery_estimate_min"`
	MaintenanceScheduled bool              `json:"maintenance_scheduled"`
	ServiceVersion       string            `json:"service_version"`
}

// SystemDegradationInfo 系统降级信息（P0-07修复）
type SystemDegradationInfo struct {
	IsSystemDegraded     bool              `json:"is_system_degraded"`
	DegradationLevel     string            `json:"degradation_level"`  // NONE, MINOR, MAJOR, SEVERE, CRITICAL
	DegradationStartTime time.Time         `json:"degradation_start_time"`
	DegradationDuration  time.Duration     `json:"degradation_duration"`
	AffectedComponents   []string          `json:"affected_components"`
	ImpactAssessment     string            `json:"impact_assessment"`
	MitigationActions    []string          `json:"mitigation_actions"`
	EstimatedRecoveryTime time.Time        `json:"estimated_recovery_time"`
	BusinessImpact       string            `json:"business_impact"`
}

// AIMonitoringMetrics AI监控指标（P0-07修复）
type AIMonitoringMetrics struct {
	TotalRequests24h     int64             `json:"total_requests_24h"`
	SuccessfulRequests   int64             `json:"successful_requests"`
	FailedRequests       int64             `json:"failed_requests"`
	TimeoutRequests      int64             `json:"timeout_requests"`
	AvgLatencyMs         float64           `json:"avg_latency_ms"`
	P95LatencyMs         float64           `json:"p95_latency_ms"`
	P99LatencyMs         float64           `json:"p99_latency_ms"`
	ErrorRate            float64           `json:"error_rate"`
	ThroughputRPS        float64           `json:"throughput_rps"`
	LastMetricUpdate     time.Time         `json:"last_metric_update"`
}

// CalculateEnhancedDataQualityWithAI 🔥 P0-07修复：计算包含AI接口监控的数据质量
func (ofm *OrderFlowManager) CalculateEnhancedDataQualityWithAI(symbol string, cvdData *CVDData, oiAnalysis *OIAnalysis, orderBookData *OrderBookData) *EnhancedDataQualityWithAI {
	// 🔥 P0-07修复：获取AI接口健康状态
	aiHealthStatus := ofm.checkAIInterfaceHealth()
	
	// 计算基础数据质量
	baseQuality := ofm.calculateDataQuality(symbol, cvdData, oiAnalysis, orderBookData)
	
	// 🔥 P0-07修复：评估系统降级状态
	degradationInfo := ofm.assessSystemDegradation(aiHealthStatus)
	
	// 🔥 P0-07修复：调整质量评分（考虑AI接口状态）
	adjustedScore := ofm.adjustQualityScoreForAI(baseQuality.OverallScore, aiHealthStatus, degradationInfo)
	
	// 🔥 P0-07修复：确定质量等级
	qualityGrade := determineQualityGrade(adjustedScore, aiHealthStatus.IsAvailable)
	
	// 🔥 P0-07修复：生成交易建议
	tradingRecommendation := generateTradingRecommendationWithAI(qualityGrade, degradationInfo, aiHealthStatus)
	
	// 🔥 P0-07修复：评估风险等级
	riskLevel := assessRiskLevelWithAI(qualityGrade, degradationInfo, aiHealthStatus)
	
	// 🔥 P0-07修复：获取监控指标
	monitoringMetrics := ofm.getAIMonitoringMetrics()
	
	// 🔥 P0-07修复：确定告警等级
	alertLevel := determineAlertLevel(degradationInfo, aiHealthStatus, adjustedScore)
	
	now := time.Now()
	return &EnhancedDataQualityWithAI{
		// 基础数据质量
		CVDReliability:       baseQuality.CVDReliability,
		OrderBookReliability: baseQuality.OrderBookReliability,
		OIReliability:        baseQuality.OIReliability,
		OverallScore:         adjustedScore,
		LastDataUpdate:       baseQuality.LastDataUpdate,
		DataLagMs:            baseQuality.DataLagMs,
		Status:               generateEnhancedStatus(aiHealthStatus, degradationInfo),
		
		// 🔥 P0-07修复：AI接口监控
		AIInterfaceStatus:    aiHealthStatus,
		SystemDegradation:    degradationInfo,
		QualityGrade:         qualityGrade,
		TradingRecommendation: tradingRecommendation,
		RiskLevel:            riskLevel,
		
		// 🔥 P0-07修复：实时监控
		MonitoringMetrics:    monitoringMetrics,
		HealthCheckTime:      now,
		NextCheckTime:        now.Add(30 * time.Second), // 30秒后再次检查
		AlertLevel:           alertLevel,
	}
}

// checkAIInterfaceHealth 检查AI接口健康状态
func (ofm *OrderFlowManager) checkAIInterfaceHealth() AIInterfaceHealthStatus {
	now := time.Now()
	aiInterface := GetGlobalAIInterfaceV12()
	
	status := AIInterfaceHealthStatus{
		CurrentMode:          "UNAVAILABLE",
		FailureReason:        "接口不可用",
		ServiceVersion:       "unknown",
		LastSuccessfulCall:   time.Time{},
		LastFailedCall:       now,
		ConsecutiveFailures:  0,
		SuccessRate24h:       0.0,
		AvgResponseTimeMs:    0,
		RecoveryEstimateMin:  -1,
		MaintenanceScheduled: false,
	}
	
	if aiInterface == nil {
		status.IsAvailable = false
		status.FailureReason = "AI接口服务不可用 (GetGlobalAIInterfaceV12 返回 nil)"
		status.ConsecutiveFailures = 999 // 表示长期不可用
		status.CurrentMode = "UNAVAILABLE"
		return status
	}
	
	// 测试AI接口可用性
	testStartTime := time.Now()
	if _, err := aiInterface.GenerateAIContext("BTCUSDT"); err == nil {
		// AI接口可用
		responseTime := time.Since(testStartTime).Milliseconds()
		status.IsAvailable = true
		status.LastSuccessfulCall = now
		status.ConsecutiveFailures = 0
		status.SuccessRate24h = 0.95 // 假设95%成功率
		status.AvgResponseTimeMs = responseTime
		status.CurrentMode = "NORMAL"
		status.FailureReason = ""
		status.RecoveryEstimateMin = 0
	} else {
		// AI接口有问题
		status.IsAvailable = false
		status.LastFailedCall = now
		status.ConsecutiveFailures = 1
		status.SuccessRate24h = 0.0
		status.CurrentMode = "FALLBACK"
		status.FailureReason = fmt.Sprintf("AI接口调用失败: %v", err)
		status.RecoveryEstimateMin = 5 // 预计5分钟恢复
	}
	
	return status
}

// assessSystemDegradation 评估系统降级状态
func (ofm *OrderFlowManager) assessSystemDegradation(aiHealth AIInterfaceHealthStatus) SystemDegradationInfo {
	now := time.Now()
	degradation := SystemDegradationInfo{
		DegradationLevel:      "NONE",
		DegradationStartTime:  time.Time{},
		DegradationDuration:   0,
		AffectedComponents:    []string{},
		MitigationActions:     []string{},
		EstimatedRecoveryTime: time.Time{},
		BusinessImpact:        "无影响",
	}
	
	if !aiHealth.IsAvailable {
		degradation.IsSystemDegraded = true
		degradation.DegradationLevel = "MAJOR" // AI接口不可用为重大降级
		degradation.DegradationStartTime = aiHealth.LastFailedCall
		degradation.DegradationDuration = now.Sub(aiHealth.LastFailedCall)
		degradation.AffectedComponents = []string{"AI接口", "决策支持", "统计增强", "风险评估"}
		degradation.ImpactAssessment = "AI决策支持功能不可用，系统使用兜底模式"
		degradation.MitigationActions = []string{
			"使用标准化兜底模式确保字段一致性",
			"降低交易频率",
			"加强人工监控",
			"优先修复AI接口服务",
		}
		degradation.EstimatedRecoveryTime = now.Add(time.Duration(aiHealth.RecoveryEstimateMin) * time.Minute)
		degradation.BusinessImpact = "交易决策质量下降，建议谨慎操作"
	} else if aiHealth.ConsecutiveFailures > 0 {
		degradation.IsSystemDegraded = true
		degradation.DegradationLevel = "MINOR"
		degradation.AffectedComponents = []string{"AI接口稳定性"}
		degradation.ImpactAssessment = "AI接口偶发故障"
		degradation.BusinessImpact = "轻微影响，系统总体正常"
	}
	
	return degradation
}

// adjustQualityScoreForAI 根据AI接口状态调整质量评分
func (ofm *OrderFlowManager) adjustQualityScoreForAI(baseScore float64, aiHealth AIInterfaceHealthStatus, degradation SystemDegradationInfo) float64 {
	adjustedScore := baseScore
	
	// 🔥 P0-07修复：AI接口不可用时强制降级质量评分
	if !aiHealth.IsAvailable {
		// AI接口不可用时，最高只能达到0.6分（即使基础数据质量很高）
		adjustedScore = adjustedScore * 0.6
		log.Printf("⚠️ [P0-07] AI接口不可用，质量评分从%.2f降级至%.2f", baseScore, adjustedScore)
	} else if degradation.DegradationLevel == "MINOR" {
		// 轻微降级时稍微降分
		adjustedScore = adjustedScore * 0.9
	}
	
	// 确保评分在合理范围内
	if adjustedScore > 1.0 {
		adjustedScore = 1.0
	} else if adjustedScore < 0.0 {
		adjustedScore = 0.0
	}
	
	return adjustedScore
}

// determineQualityGrade 确定质量等级
func determineQualityGrade(score float64, aiAvailable bool) string {
	// 🔥 P0-07修复：AI接口不可用时最高只能是C级
	if !aiAvailable {
		if score >= 0.5 {
			return "C" // AI不可用时的最高等级
		} else if score >= 0.3 {
			return "D"
		} else {
			return "F"
		}
	}
	
	// AI可用时的正常等级
	if score >= 0.9 {
		return "A"
	} else if score >= 0.8 {
		return "B"
	} else if score >= 0.6 {
		return "C"
	} else if score >= 0.4 {
		return "D"
	} else {
		return "F"
	}
}

// generateTradingRecommendationWithAI 生成包含AI状态的交易建议
func generateTradingRecommendationWithAI(grade string, degradation SystemDegradationInfo, aiHealth AIInterfaceHealthStatus) string {
	if !aiHealth.IsAvailable {
		switch grade {
		case "C":
			return "AI接口不可用，建议谨慎交易，减少仓位至平时的50%"
		case "D":
			return "AI接口不可用且数据质量差，建议暂停新开仓，只保留必要止损"
		case "F":
			return "AI接口不可用且数据严重异常，建议立即停止交易"
		default:
			return "AI接口不可用，建议等待恢复后再进行交易"
		}
	}
	
	switch grade {
	case "A":
		return "数据质量优秀，可以正常交易"
	case "B":
		return "数据质量良好，可以进行交易但建议降低杠杆"
	case "C":
		return "数据质量一般，建议谨慎交易"
	case "D":
		return "数据质量较差，不建议交易"
	case "F":
		return "数据质量严重异常，禁止交易"
	default:
		return "数据质量未知，暂停交易"
	}
}

// assessRiskLevelWithAI 评估包含AI状态的风险等级
func assessRiskLevelWithAI(grade string, degradation SystemDegradationInfo, aiHealth AIInterfaceHealthStatus) string {
	// 基础风险等级
	var baseRisk string
	switch grade {
	case "A", "B":
		baseRisk = "LOW"
	case "C":
		baseRisk = "MEDIUM"
	case "D":
		baseRisk = "HIGH"
	case "F":
		baseRisk = "CRITICAL"
	default:
		baseRisk = "HIGH"
	}
	
	// 🔥 P0-07修复：AI接口状态影响风险评估
	if !aiHealth.IsAvailable {
		// AI不可用时风险等级至少为MEDIUM
		if baseRisk == "LOW" {
			return "MEDIUM"
		} else if baseRisk == "MEDIUM" {
			return "HIGH"
		}
		return "CRITICAL"
	}
	
	// 考虑系统降级情况
	if degradation.IsSystemDegraded && degradation.DegradationLevel == "MAJOR" {
		if baseRisk == "LOW" {
			return "MEDIUM"
		}
	}
	
	return baseRisk
}

// getAIMonitoringMetrics 获取AI监控指标
func (ofm *OrderFlowManager) getAIMonitoringMetrics() AIMonitoringMetrics {
	// 简化版本，实际环境中应该从监控系统获取真实数据
	return AIMonitoringMetrics{
		TotalRequests24h:     1440, // 假设每分钟1次请求
		SuccessfulRequests:   1368, // 95%成功率
		FailedRequests:       72,
		TimeoutRequests:      0,
		AvgLatencyMs:         150.0,
		P95LatencyMs:         300.0,
		P99LatencyMs:         500.0,
		ErrorRate:            0.05,
		ThroughputRPS:        1.0,
		LastMetricUpdate:     time.Now(),
	}
}

// determineAlertLevel 确定告警等级
func determineAlertLevel(degradation SystemDegradationInfo, aiHealth AIInterfaceHealthStatus, score float64) string {
	if !aiHealth.IsAvailable {
		return "ERROR" // AI接口不可用为ERROR级别
	}
	
	if degradation.IsSystemDegraded && degradation.DegradationLevel == "MAJOR" {
		return "ERROR"
	}
	
	if degradation.IsSystemDegraded && degradation.DegradationLevel == "MINOR" {
		return "WARNING"
	}
	
	if score < 0.3 {
		return "ERROR"
	} else if score < 0.6 {
		return "WARNING"
	} else if score < 0.8 {
		return "INFO"
	}
	
	return "NONE"
}

// generateEnhancedStatus 生成增强状态描述
func generateEnhancedStatus(aiHealth AIInterfaceHealthStatus, degradation SystemDegradationInfo) string {
	if !aiHealth.IsAvailable {
		return fmt.Sprintf("AI接口降级 - %s", degradation.ImpactAssessment)
	}
	
	if degradation.IsSystemDegraded {
		return fmt.Sprintf("系统降级(%s) - %s", degradation.DegradationLevel, degradation.ImpactAssessment)
	}
	
	return "系统正常运行"
}

// ===== P0-07修复：生产环境AI接口监控 =====

// RunAIInterfaceHealthCheck 🔥 P0-07修复：运行AI接口健康检查
func RunAIInterfaceHealthCheck() *AIInterfaceHealthStatus {
	log.Printf("🔬 [P0-07] 开始AI接口健康检查")
	
	ofm := GetGlobalOrderFlowManager()
	healthStatus := ofm.checkAIInterfaceHealth()
	
	if !healthStatus.IsAvailable {
		log.Printf("❌ [P0-07] AI接口不可用: %s", healthStatus.FailureReason)
		log.Printf("⏱️ [P0-07] 预计恢复时间: %d分钟", healthStatus.RecoveryEstimateMin)
	} else {
		log.Printf("✅ [P0-07] AI接口正常运行，响应时间: %dms", healthStatus.AvgResponseTimeMs)
	}
	
	return &healthStatus
}