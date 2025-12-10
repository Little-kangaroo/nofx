package microstructure

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// ===== V-12.2 哨兵AI请求触发系统 =====

// SentinelAIRequestTrigger 哨兵AI请求触发器
type SentinelAIRequestTrigger struct {
	mu              sync.Mutex                // 🔴 P0修复: 防止并发Map写崩溃
	aiInterface     *AIInterfaceV12
	requestHistory  map[string]time.Time      // symbol -> last_request_time
	cooldownPeriod  time.Duration             // AI请求冷却期
}

// NewSentinelAIRequestTrigger 创建哨兵AI请求触发器
func NewSentinelAIRequestTrigger() *SentinelAIRequestTrigger {
	return &SentinelAIRequestTrigger{
		aiInterface:    GetGlobalAIInterfaceV12(),
		requestHistory: make(map[string]time.Time),
		cooldownPeriod: 60 * time.Second, // 每个交易对60秒内只触发一次AI请求
	}
}

// HandleSentinelAlert 处理哨兵警报并触发AI请求
func (trigger *SentinelAIRequestTrigger) HandleSentinelAlert(alert *SentinelTriggerAlert) {
	// 检查冷却期
	if !trigger.checkCooldown(alert.Symbol) {
		log.Printf("⏳ [%s] AI请求冷却中，跳过此次触发", alert.Symbol)
		return
	}
	
	// 生成AI请求参数
	aiParams, err := trigger.buildAIRequestParams(alert)
	if err != nil {
		log.Printf("❌ [%s] 构建AI参数失败: %v", alert.Symbol, err)
		return
	}
	
	// 记录触发信息
	log.Printf("🤖 [%s] 哨兵触发AI分析 - 场景: %s, 置信度: %.1f%%", 
		alert.Symbol, alert.Scenario, alert.Confidence*100)
	
	// 发送AI请求
	err = trigger.sendAIRequest(alert, aiParams)
	if err != nil {
		log.Printf("❌ [%s] AI请求发送失败: %v", alert.Symbol, err)
		return
	}
	
	// 更新请求历史 - 🔴 P0修复: 保护Map写操作
	trigger.mu.Lock()
	trigger.requestHistory[alert.Symbol] = time.Now()
	trigger.mu.Unlock()
	
	log.Printf("✅ [%s] AI分析请求已发送", alert.Symbol)
}

// AIRequestParams AI请求参数结构
type AIRequestParams struct {
	// 基础信息
	Symbol          string    `json:"symbol"`
	RequestID       string    `json:"request_id"`
	Timestamp       time.Time `json:"timestamp"`
	TriggerSource   string    `json:"trigger_source"`   // "sentinel_v12"
	
	// 触发原因
	TriggerReason   TriggerReason `json:"trigger_reason"`
	
	// V-12.2核心分析上下文
	AIContext       *AIContextV12 `json:"ai_context"`
	
	// 哨兵特定信息
	SentinelInfo    SentinelRequestInfo `json:"sentinel_info"`
	
	// 紧急程度标识
	Urgency         UrgencyLevel `json:"urgency"`
	
	// 预期响应时间
	ExpectedResponseTime string `json:"expected_response_time"`
}

// TriggerReason 触发原因结构
type TriggerReason struct {
	Scenario        TriggerScenario `json:"scenario"`
	Confidence      float64         `json:"confidence"`
	Description     string          `json:"description"`
	KeyIndicators   []KeyIndicator  `json:"key_indicators"`
	MarketRegime    string          `json:"market_regime"`
	ThreatLevel     string          `json:"threat_level"`
}

// SentinelRequestInfo 哨兵请求信息
type SentinelRequestInfo struct {
	DetectionTime   time.Time              `json:"detection_time"`
	ScanMode        string                 `json:"scan_mode"`          // "normal"/"high_freq"/"adaptive"
	AlertMetadata   map[string]interface{} `json:"alert_metadata"`
	ZScoreTriggers  map[string]float64     `json:"zscore_triggers"`    // 触发的Z-Score值
	ThresholdUsed   map[string]float64     `json:"threshold_used"`     // 使用的阈值
	ConfidenceBreakdown ConfidenceBreakdown `json:"confidence_breakdown"`
}

// KeyIndicator 关键指标
type KeyIndicator struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	ZScore      float64 `json:"z_score"`
	Threshold   float64 `json:"threshold"`
	Significance string `json:"significance"` // "极高"/"高"/"中等"
}

// ConfidenceBreakdown 置信度分解
type ConfidenceBreakdown struct {
	CVDContribution      float64 `json:"cvd_contribution"`      // CVD贡献度
	VolumeContribution   float64 `json:"volume_contribution"`   // 成交量贡献度
	OrderBookContribution float64 `json:"orderbook_contribution"` // 盘口贡献度
	PriceContribution    float64 `json:"price_contribution"`    // 价格贡献度
	RegimeAdjustment     float64 `json:"regime_adjustment"`     // 市场状态调整
	FinalConfidence      float64 `json:"final_confidence"`      // 最终置信度
}

// UrgencyLevel 紧急程度
type UrgencyLevel string

const (
	UrgencyLow      UrgencyLevel = "low"      // 低紧急: 正常处理 (5-10分钟)
	UrgencyMedium   UrgencyLevel = "medium"   // 中等紧急: 优先处理 (1-3分钟)
	UrgencyHigh     UrgencyLevel = "high"     // 高紧急: 立即处理 (30秒内)
	UrgencyCritical UrgencyLevel = "critical" // 极紧急: 毫秒级处理
)

// buildAIRequestParams 构建AI请求参数
func (trigger *SentinelAIRequestTrigger) buildAIRequestParams(alert *SentinelTriggerAlert) (*AIRequestParams, error) {
	// 生成请求ID
	requestID := fmt.Sprintf("sentinel_%s_%d", alert.Symbol, time.Now().UnixNano())
	
	// 生成V-12.2 AI上下文
	aiContext, err := trigger.aiInterface.GenerateAIContext(alert.Symbol)
	if err != nil {
		return nil, fmt.Errorf("生成AI上下文失败: %w", err)
	}
	
	// 构建触发原因
	triggerReason := trigger.buildTriggerReason(alert, aiContext)
	
	// 构建哨兵信息
	sentinelInfo := trigger.buildSentinelInfo(alert, aiContext)
	
	// 评估紧急程度
	urgency := trigger.assessUrgency(alert, aiContext)
	
	// 确定预期响应时间
	expectedResponseTime := trigger.getExpectedResponseTime(urgency)
	
	return &AIRequestParams{
		Symbol:          alert.Symbol,
		RequestID:       requestID,
		Timestamp:       time.Now(),
		TriggerSource:   "sentinel_v12",
		TriggerReason:   triggerReason,
		AIContext:       aiContext,
		SentinelInfo:    sentinelInfo,
		Urgency:         urgency,
		ExpectedResponseTime: expectedResponseTime,
	}, nil
}

// buildTriggerReason 构建触发原因
func (trigger *SentinelAIRequestTrigger) buildTriggerReason(alert *SentinelTriggerAlert, aiContext *AIContextV12) TriggerReason {
	// 提取关键指标
	keyIndicators := trigger.extractKeyIndicators(alert, aiContext)
	
	// 获取市场状态
	marketRegime := "unknown"
	threatLevel := "medium"
	
	if aiContext.DynamicThresholds != nil {
		marketRegime = aiContext.DynamicThresholds.MarketRegime
	}
	
	if aiContext.SentinelAlerts != nil {
		threatLevel = aiContext.SentinelAlerts.ThreatLevel
	}
	
	return TriggerReason{
		Scenario:      alert.Scenario,
		Confidence:    alert.Confidence,
		Description:   alert.Description,
		KeyIndicators: keyIndicators,
		MarketRegime:  marketRegime,
		ThreatLevel:   threatLevel,
	}
}

// extractKeyIndicators 提取关键指标
func (trigger *SentinelAIRequestTrigger) extractKeyIndicators(alert *SentinelTriggerAlert, aiContext *AIContextV12) []KeyIndicator {
	var indicators []KeyIndicator
	
	// 从Z-Score数据提取关键指标
	if aiContext.StatisticalContext != nil {
		for name, zScore := range aiContext.StatisticalContext.ZScores {
			if math.Abs(zScore) > 1.5 { // 只包含显著的指标
				
				// 获取对应的阈值
				threshold := 1.5 // 默认阈值
				if aiContext.DynamicThresholds != nil {
					if name == "spot_cvd_1m" || name == "futures_cvd_1m" {
						if cvdThreshold, exists := aiContext.DynamicThresholds.CVDThresholds["momentum_ignition"]; exists {
							threshold = cvdThreshold
						}
					} else if name == "volume_1m" {
						if volThreshold, exists := aiContext.DynamicThresholds.VolumeThresholds["abnormal_volume"]; exists {
							threshold = volThreshold
						}
					}
				}
				
				// 确定显著性
				significance := trigger.getSignificance(zScore)
				
				// 获取实际值
				value := trigger.getActualValue(name, aiContext)
				
				indicators = append(indicators, KeyIndicator{
					Name:         name,
					Value:        value,
					ZScore:       zScore,
					Threshold:    threshold,
					Significance: significance,
				})
			}
		}
	}
	
	return indicators
}

// getSignificance 获取显著性水平
func (trigger *SentinelAIRequestTrigger) getSignificance(zScore float64) string {
	absZ := math.Abs(zScore)
	if absZ >= 2.58 {
		return "极高"
	} else if absZ >= 1.96 {
		return "高"
	} else if absZ >= 1.28 {
		return "中等"
	}
	return "低"
}

// getActualValue 获取指标的实际值
func (trigger *SentinelAIRequestTrigger) getActualValue(indicatorName string, aiContext *AIContextV12) float64 {
	// 从AI上下文中提取实际值
	if aiContext.MarketMicrostructure != nil && aiContext.MarketMicrostructure.CVDDelta5mEnhanced != nil {
		cvdData := aiContext.MarketMicrostructure.CVDDelta5mEnhanced.CVDDelta5m
		
		switch indicatorName {
		case "spot_cvd_1m":
			return cvdData.SpotCVDDeltaUSD
		case "futures_cvd_1m":
			return cvdData.FuturesCVDDeltaUSD
		case "volume_1m":
			return cvdData.VolumeDelta
		case "price_change_1m":
			return cvdData.PriceDeltaPct
		}
	}
	
	return 0.0
}

// buildSentinelInfo 构建哨兵信息
func (trigger *SentinelAIRequestTrigger) buildSentinelInfo(alert *SentinelTriggerAlert, aiContext *AIContextV12) SentinelRequestInfo {
	// 获取哨兵引擎信息
	sentinelEngine := GetGlobalSentinelTriggerEngine()
	scanMode := string(sentinelEngine.GetScanMode())
	
	// 构建置信度分解
	confidenceBreakdown := trigger.buildConfidenceBreakdown(alert, aiContext)
	
	// 提取使用的阈值
	thresholdUsed := make(map[string]float64)
	if metadata, exists := alert.Metadata["used_cvd_threshold"]; exists {
		if threshold, ok := metadata.(float64); ok {
			thresholdUsed["cvd_threshold"] = threshold
		}
	}
	if metadata, exists := alert.Metadata["used_volume_threshold"]; exists {
		if threshold, ok := metadata.(float64); ok {
			thresholdUsed["volume_threshold"] = threshold
		}
	}
	
	return SentinelRequestInfo{
		DetectionTime:       alert.Timestamp,
		ScanMode:           scanMode,
		AlertMetadata:      alert.Metadata,
		ZScoreTriggers:     alert.ZScoreData,
		ThresholdUsed:      thresholdUsed,
		ConfidenceBreakdown: confidenceBreakdown,
	}
}

// buildConfidenceBreakdown 构建置信度分解
func (trigger *SentinelAIRequestTrigger) buildConfidenceBreakdown(alert *SentinelTriggerAlert, aiContext *AIContextV12) ConfidenceBreakdown {
	// 根据场景类型计算各项贡献度
	breakdown := ConfidenceBreakdown{
		FinalConfidence: alert.Confidence,
	}
	
	switch alert.Scenario {
	case ScenarioMomentumIgnition:
		// 动量点燃: CVD和成交量为主要因子
		breakdown.CVDContribution = alert.Confidence * 0.4
		breakdown.VolumeContribution = alert.Confidence * 0.3
		breakdown.PriceContribution = alert.Confidence * 0.2
		breakdown.OrderBookContribution = alert.Confidence * 0.1
		
	case ScenarioSpotRush:
		// 现货抢跑: CVD比率为主要因子
		breakdown.CVDContribution = alert.Confidence * 0.6
		breakdown.VolumeContribution = alert.Confidence * 0.2
		breakdown.PriceContribution = alert.Confidence * 0.1
		breakdown.OrderBookContribution = alert.Confidence * 0.1
		
	case ScenarioSpoofingFlip:
		// 虚假翻转: 盘口为主要因子
		breakdown.OrderBookContribution = alert.Confidence * 0.5
		breakdown.CVDContribution = alert.Confidence * 0.2
		breakdown.VolumeContribution = alert.Confidence * 0.2
		breakdown.PriceContribution = alert.Confidence * 0.1
	}
	
	// 市场状态调整
	if aiContext.DynamicThresholds != nil {
		regimeAdjustment := 1.0
		switch aiContext.DynamicThresholds.MarketRegime {
		case "high_volatility":
			regimeAdjustment = 0.9 // 高波动环境降低置信度
		case "manipulation":
			regimeAdjustment = 0.8 // 操纵环境大幅降低置信度
		case "low_liquidity":
			regimeAdjustment = 0.85 // 低流动性环境适度降低置信度
		}
		breakdown.RegimeAdjustment = regimeAdjustment
	} else {
		breakdown.RegimeAdjustment = 1.0
	}
	
	return breakdown
}

// assessUrgency 评估紧急程度
func (trigger *SentinelAIRequestTrigger) assessUrgency(alert *SentinelTriggerAlert, aiContext *AIContextV12) UrgencyLevel {
	urgencyScore := 0.0
	
	// 置信度贡献 (30%)
	urgencyScore += alert.Confidence * 0.3
	
	// 威胁等级贡献 (25%)
	if aiContext.SentinelAlerts != nil {
		switch aiContext.SentinelAlerts.ThreatLevel {
		case "critical":
			urgencyScore += 1.0 * 0.25
		case "high":
			urgencyScore += 0.8 * 0.25
		case "medium":
			urgencyScore += 0.5 * 0.25
		case "low":
			urgencyScore += 0.2 * 0.25
		}
	}
	
	// 市场状态贡献 (25%)
	if aiContext.DynamicThresholds != nil {
		switch aiContext.DynamicThresholds.MarketRegime {
		case "manipulation":
			urgencyScore += 1.0 * 0.25
		case "high_volatility":
			urgencyScore += 0.8 * 0.25
		case "low_liquidity":
			urgencyScore += 0.6 * 0.25
		default:
			urgencyScore += 0.3 * 0.25
		}
	}
	
	// Z-Score极值贡献 (20%)
	maxZScore := 0.0
	for _, zScore := range alert.ZScoreData {
		if math.Abs(zScore) > maxZScore {
			maxZScore = math.Abs(zScore)
		}
	}
	if maxZScore > 3.0 {
		urgencyScore += 1.0 * 0.2
	} else if maxZScore > 2.5 {
		urgencyScore += 0.8 * 0.2
	} else if maxZScore > 2.0 {
		urgencyScore += 0.5 * 0.2
	}
	
	// 根据综合得分确定紧急程度
	if urgencyScore >= 0.8 {
		return UrgencyCritical
	} else if urgencyScore >= 0.6 {
		return UrgencyHigh
	} else if urgencyScore >= 0.4 {
		return UrgencyMedium
	}
	return UrgencyLow
}

// getExpectedResponseTime 获取预期响应时间
func (trigger *SentinelAIRequestTrigger) getExpectedResponseTime(urgency UrgencyLevel) string {
	switch urgency {
	case UrgencyCritical:
		return "立即响应 (<10秒)"
	case UrgencyHigh:
		return "高优先级 (30秒内)"
	case UrgencyMedium:
		return "正常优先级 (1-3分钟)"
	case UrgencyLow:
		return "低优先级 (5-10分钟)"
	default:
		return "正常优先级 (1-3分钟)"
	}
}

// sendAIRequest 发送AI请求
func (trigger *SentinelAIRequestTrigger) sendAIRequest(alert *SentinelTriggerAlert, params *AIRequestParams) error {
	// 将参数转换为JSON格式
	jsonData, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return fmt.Errorf("参数序列化失败: %w", err)
	}
	
	// 记录请求参数（用于调试和监控）
	log.Printf("🤖 [%s] AI请求参数构建完成:", params.Symbol)
	log.Printf("   请求ID: %s", params.RequestID)
	log.Printf("   紧急程度: %s (%s)", params.Urgency, params.ExpectedResponseTime)
	log.Printf("   触发场景: %s (置信度: %.1f%%)", params.TriggerReason.Scenario, params.TriggerReason.Confidence*100)
	log.Printf("   关键指标数量: %d", len(params.TriggerReason.KeyIndicators))
	log.Printf("   市场状态: %s", params.TriggerReason.MarketRegime)
	
	// 这里应该调用实际的AI服务接口
	// 目前只记录日志，实际实现时需要：
	// 1. 调用HTTP API发送到AI服务
	// 2. 或者发送到消息队列
	// 3. 或者直接调用本地AI模块
	
	log.Printf("📤 [%s] AI请求已发送 (参数大小: %d bytes)", params.Symbol, len(jsonData))
	
	// TODO: 实际的AI请求发送逻辑
	// return sendToAIService(jsonData, params.Urgency)
	
	return nil
}

// checkCooldown 检查冷却期
func (trigger *SentinelAIRequestTrigger) checkCooldown(symbol string) bool {
	trigger.mu.Lock()  // 🔴 P0修复: 保护Map读操作
	defer trigger.mu.Unlock()
	
	lastRequest, exists := trigger.requestHistory[symbol]
	if !exists {
		return true
	}
	
	return time.Since(lastRequest) > trigger.cooldownPeriod
}

// ToJSONString 将AI请求参数转换为JSON字符串
func (params *AIRequestParams) ToJSONString() (string, error) {
	jsonData, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

// GetRequestSummary 获取请求摘要信息
func (params *AIRequestParams) GetRequestSummary() map[string]interface{} {
	return map[string]interface{}{
		"request_id":        params.RequestID,
		"symbol":           params.Symbol,
		"trigger_scenario": params.TriggerReason.Scenario,
		"confidence":       fmt.Sprintf("%.1f%%", params.TriggerReason.Confidence*100),
		"urgency":          params.Urgency,
		"market_regime":    params.TriggerReason.MarketRegime,
		"threat_level":     params.TriggerReason.ThreatLevel,
		"key_indicators":   len(params.TriggerReason.KeyIndicators),
		"scan_mode":        params.SentinelInfo.ScanMode,
		"timestamp":        params.Timestamp.Format("15:04:05.000"),
	}
}

// ===== 全局实例 =====

var globalSentinelAITrigger *SentinelAIRequestTrigger

// GetSentinelAITrigger 获取哨兵AI触发器实例
func GetSentinelAITrigger() *SentinelAIRequestTrigger {
	if globalSentinelAITrigger == nil {
		globalSentinelAITrigger = NewSentinelAIRequestTrigger()
		log.Printf("✨ 哨兵AI请求触发器已创建")
	}
	return globalSentinelAITrigger
}