package microstructure

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// ===== V-12.2 AI接口协议升级 =====

// AIContextV12 V-12.2 AI分析上下文
type AIContextV12 struct {
	// 基础市场信息
	Symbol          string    `json:"symbol"`
	Timestamp       time.Time `json:"timestamp"`
	ProtocolVersion string    `json:"protocol_version"` // "v12.2"
	
	// V-12.2核心统计上下文
	StatisticalContext *StatisticalContext `json:"statistical_context,omitempty"`   // 🔵 P3修复
	
	// 动态阈值上下文
	DynamicThresholds *ThresholdContext `json:"dynamic_thresholds,omitempty"`       // 🔵 P3修复
	
	// 哨兵警报上下文
	SentinelAlerts *SentinelContext `json:"sentinel_alerts,omitempty"`            // 🔵 P3修复
	
	// 市场微观结构数据
	MarketMicrostructure *MicrostructureContext `json:"market_microstructure,omitempty"` // 🔵 P3修复
	
	// AI决策支持数据
	DecisionSupport *DecisionSupportContext `json:"decision_support,omitempty"`      // 🔵 P3修复
}

// StatisticalContext V-12.2统计上下文
type StatisticalContext struct {
	// Z-Score统计数据
	ZScores map[string]float64 `json:"z_scores,omitempty"`                       // 🔵 P3修复
	
	// 滚动统计质量评估
	DataQuality map[string]float64 `json:"data_quality,omitempty"`                // 🔵 P3修复
	
	// 统计窗口信息
	WindowInfo map[string]interface{} `json:"window_info,omitempty"`             // 🔵 P3修复
	
	// 历史分布特征
	DistributionStats map[string]interface{} `json:"distribution_stats,omitempty"` // 🔵 P3修复
}

// ThresholdContext 动态阈值上下文
type ThresholdContext struct {
	// 当前市场状态
	MarketRegime    string  `json:"market_regime,omitempty"`                     // 🔵 P3修复
	RegimeConfidence float64 `json:"regime_confidence,omitempty"`                // 🔵 P3修复
	
	// 各类动态阈值
	CVDThresholds      map[string]float64 `json:"cvd_thresholds,omitempty"`      // 🔵 P3修复
	VolumeThresholds   map[string]float64 `json:"volume_thresholds,omitempty"`   // 🔵 P3修复
	OrderbookThresholds map[string]float64 `json:"orderbook_thresholds,omitempty"` // 🔵 P3修复
	PriceThresholds    map[string]float64 `json:"price_thresholds,omitempty"`    // 🔵 P3修复
	
	// 市场状态评估
	VolatilityLevel   float64 `json:"volatility_level"`
	TrendStrength     float64 `json:"trend_strength"`
	LiquidityLevel    float64 `json:"liquidity_level"`
	ManipulationRisk  float64 `json:"manipulation_risk"`
	
	// 阈值调整历史
	RecentAdjustments []ThresholdAdjustment `json:"recent_adjustments"`
}

// SentinelContext 哨兵警报上下文
type SentinelContext struct {
	// 最近警报
	RecentAlerts []SentinelAlertSummary `json:"recent_alerts"`
	
	// 当前威胁等级
	ThreatLevel string `json:"threat_level"` // low/medium/high/critical
	
	// 各场景检测状态
	ScenarioStatus map[string]ScenarioDetectionStatus `json:"scenario_status"`
	
	// 警报统计
	AlertStats AlertStatistics `json:"alert_stats"`
}

// MicrostructureContext 市场微观结构上下文
type MicrostructureContext struct {
	// CVD 5分钟增量（增强版）
	CVDDelta5mEnhanced *CVDDelta5mEnhanced `json:"cvd_delta_5m_enhanced"`
	
	// 盘口深度分析（增强版）
	OrderBookEnhanced *OrderBookEnhanced `json:"orderbook_enhanced"`
	
	// 持仓量分析（增强版）
	OIAnalysisEnhanced *OIAnalysisEnhanced `json:"oi_analysis_enhanced"`
	
	// 宏观资金流趋势
	MacroTrends *MacroTrendsEnhanced `json:"macro_trends"`
}

// DecisionSupportContext AI决策支持上下文
type DecisionSupportContext struct {
	// 信号强度综合评分
	SignalStrength float64 `json:"signal_strength"` // 0-100
	
	// 风险等级评估
	RiskAssessment *RiskAssessment `json:"risk_assessment"`
	
	// 市场时机评估
	TimingAssessment *TimingAssessment `json:"timing_assessment"`
	
	// 建议操作类型
	SuggestedActions []ActionSuggestion `json:"suggested_actions"`
	
	// 置信度校准
	ConfidenceCalibration *ConfidenceCalibration `json:"confidence_calibration"`
}

// CVDDelta5mEnhanced CVD 5分钟增量增强版
type CVDDelta5mEnhanced struct {
	*CVDDelta5m // 嵌入原始结构
	
	// V-12.2增强字段
	ZScoreNormalized  map[string]float64 `json:"zscore_normalized"`
	StatisticalRank   map[string]float64 `json:"statistical_rank"` // 0-1，在历史分布中的排名
	AnomalyScore      float64           `json:"anomaly_score"`    // 异常程度综合评分
	RegimeAdjusted    map[string]float64 `json:"regime_adjusted"`  // 基于市场状态调整后的值
}

// OrderBookEnhanced 盘口数据增强版
type OrderBookEnhanced struct {
	*OrderBookData // 嵌入原始结构
	
	// V-12.2增强字段
	LiquidityDistribution map[string]float64 `json:"liquidity_distribution"` // 流动性分布
	PriceImpactEstimate   map[string]float64 `json:"price_impact_estimate"`   // 价格冲击预估
	MarketDepthAnalysis   map[string]interface{} `json:"market_depth_analysis"`
	ManipulationIndicators map[string]float64   `json:"manipulation_indicators"`
}

// OIAnalysisEnhanced 持仓量分析增强版
type OIAnalysisEnhanced struct {
	*OIAnalysis // 嵌入原始结构
	
	// V-12.2增强字段
	OIVelocity        float64 `json:"oi_velocity"`         // 持仓量变化速度
	OIAcceleration    float64 `json:"oi_acceleration"`     // 持仓量变化加速度
	TrendConsistency  float64 `json:"trend_consistency"`   // 趋势一致性
	MarketParticipation float64 `json:"market_participation"` // 市场参与度
}

// MacroTrendsEnhanced 宏观趋势增强版
type MacroTrendsEnhanced struct {
	// 多时间框架趋势
	TrendMultiTimeframe map[string]float64 `json:"trend_multi_timeframe"`
	
	// 资金流动方向强度
	MoneyFlowStrength map[string]float64 `json:"money_flow_strength"`
	
	// 市场主导性分析
	MarketDominance map[string]float64 `json:"market_dominance"`
	
	// 周期性模式识别
	CyclicalPatterns map[string]interface{} `json:"cyclical_patterns"`
}

// 辅助结构体定义
type ThresholdAdjustment struct {
	Timestamp   time.Time `json:"timestamp"`
	ThresholdType string  `json:"threshold_type"`
	OldValue    float64   `json:"old_value"`
	NewValue    float64   `json:"new_value"`
	Reason      string    `json:"reason"`
}

type SentinelAlertSummary struct {
	Timestamp   time.Time       `json:"timestamp"`
	Scenario    TriggerScenario `json:"scenario"`
	Confidence  float64         `json:"confidence"`
	Description string          `json:"description"`
}

type ScenarioDetectionStatus struct {
	LastCheck    time.Time `json:"last_check"`
	IsActive     bool      `json:"is_active"`
	Confidence   float64   `json:"confidence"`
	Cooldown     int       `json:"cooldown"` // 剩余冷却秒数
}

type AlertStatistics struct {
	Total24h        int     `json:"total_24h"`
	MomentumIgnition int     `json:"momentum_ignition"`
	SpotRush        int     `json:"spot_rush"`
	SpoofingFlip    int     `json:"spoofing_flip"`
	AvgConfidence   float64 `json:"avg_confidence"`
}

type RiskAssessment struct {
	OverallRisk     float64            `json:"overall_risk"` // 0-100
	RiskFactors     map[string]float64 `json:"risk_factors"`
	RiskMitigation  []string           `json:"risk_mitigation"`
}

type TimingAssessment struct {
	MarketTiming    float64 `json:"market_timing"`    // 0-100，市场时机评分
	VolatilityTiming float64 `json:"volatility_timing"` // 波动性时机
	LiquidityTiming float64 `json:"liquidity_timing"`  // 流动性时机
	TrendAlignment  float64 `json:"trend_alignment"`   // 趋势对齐度
}

type ActionSuggestion struct {
	ActionType  string  `json:"action_type"`  // "buy", "sell", "hold", "reduce"
	Confidence  float64 `json:"confidence"`
	Reasoning   string  `json:"reasoning"`
	Priority    string  `json:"priority"`     // "high", "medium", "low"
	Timeframe   string  `json:"timeframe"`    // "immediate", "short", "medium"
}

type ConfidenceCalibration struct {
	RawConfidence       float64 `json:"raw_confidence"`
	CalibratedConfidence float64 `json:"calibrated_confidence"`
	CalibrationFactors  map[string]float64 `json:"calibration_factors"`
	HistoricalAccuracy  float64 `json:"historical_accuracy"`
}

// AIInterfaceV12 V-12.2 AI接口主类
type AIInterfaceV12 struct {
	globalStatsManager     *GlobalStatsManager
	dynamicThresholdSystem *DynamicThresholdSystem
	sentinelTriggerEngine  *SentinelTriggerEngine
	orderFlowManager       *OrderFlowManager
}

// NewAIInterfaceV12 创建V-12.2 AI接口
func NewAIInterfaceV12() *AIInterfaceV12 {
	return &AIInterfaceV12{
		globalStatsManager:     GetGlobalStatsManager(),
		dynamicThresholdSystem: GetGlobalDynamicThresholdSystem(),
		sentinelTriggerEngine:  GetGlobalSentinelTriggerEngine(),
		orderFlowManager:       GetGlobalOrderFlowManager(),
	}
}

// GenerateAIContext 生成V-12.2 AI分析上下文（⚠️ Deprecated: 使用 GenerateAIContextFromSnapshot）
func (ai *AIInterfaceV12) GenerateAIContext(symbol string) (*AIContextV12, error) {
	log.Printf("⚠️ Deprecated: GenerateAIContext(symbol) - 建议使用 GenerateAIContextFromSnapshot 避免二次取样")

	if ai.orderFlowManager == nil {
		return nil, fmt.Errorf("订单流管理器未初始化")
	}

	// 🔥 T02修复：获取市场快照，使用 time.Now() 作为兼容路径
	snapshot := ai.orderFlowManager.GetMarketSnapshot(symbol)
	if snapshot == nil {
		return nil, fmt.Errorf("无法获取%s的市场快照", symbol)
	}

	// 🔥 T02修复：转调新接口，避免重复实现
	return ai.GenerateAIContextFromSnapshot(snapshot)
}

// GenerateAIContextFromSnapshot 🔥 T02修复：基于快照生成AI上下文（推荐）
func (ai *AIInterfaceV12) GenerateAIContextFromSnapshot(snapshot *MarketSnapshot) (*AIContextV12, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("快照不能为nil")
	}

	// 🔥 T02修复：使用 snapshot.Timestamp 而非 time.Now()
	symbol := snapshot.Symbol

	// 构建AI上下文
	context := &AIContextV12{
		Symbol:          symbol,
		Timestamp:       snapshot.Timestamp, // 🔥 关键修复：使用快照时间戳
		ProtocolVersion: "v12.2",
	}
	
	// 生成统计上下文
	context.StatisticalContext = ai.generateStatisticalContext(symbol, snapshot)
	
	// 生成动态阈值上下文
	context.DynamicThresholds = ai.generateThresholdContext(symbol)
	
	// 生成哨兵警报上下文
	context.SentinelAlerts = ai.generateSentinelContext(symbol)
	
	// 生成市场微观结构上下文
	context.MarketMicrostructure = ai.generateMicrostructureContext(symbol, snapshot)
	
	// 生成决策支持上下文
	context.DecisionSupport = ai.generateDecisionSupportContext(symbol, snapshot, context)
	
	log.Printf("✨ [%s] V-12.2 AI上下文生成完成，协议版本: %s", symbol, context.ProtocolVersion)
	
	return context, nil
}

// generateStatisticalContext 生成统计上下文
func (ai *AIInterfaceV12) generateStatisticalContext(symbol string, snapshot *MarketSnapshot) *StatisticalContext {
	// 获取Z-Score数据
	zScores := ai.globalStatsManager.GetSymbolZScores(symbol, snapshot)
	
	// 获取数据质量信息
	dataQuality := make(map[string]float64)
	windowInfo := make(map[string]interface{})
	distributionStats := make(map[string]interface{})
	
	// 从全局统计管理器获取详细信息
	if statsOverview := ai.globalStatsManager.GetAllStatsOverview(); statsOverview != nil {
		if symbolStats, exists := statsOverview[symbol]; exists {
			if statsMap, ok := symbolStats.(map[string]interface{}); ok {
				// 提取数据质量信息
				if priceStats, exists := statsMap["price_stats"]; exists {
					if priceStatsMap, ok := priceStats.(map[string]interface{}); ok {
						if quality, exists := priceStatsMap["data_quality"]; exists {
							if qualityFloat, ok := quality.(float64); ok {
								dataQuality["price"] = qualityFloat
							}
						}
					}
				}
				
				// 提取窗口信息
				windowInfo["last_update"] = statsMap["last_update"]
				windowInfo["symbol"] = statsMap["symbol"]
			}
		}
	}
	
	// 计算分布统计
	for metric, zScore := range zScores {
		distributionStats[metric] = map[string]interface{}{
			"z_score":      zScore,
			"abs_z_score":  math.Abs(zScore),
			"percentile":   ai.zScoreToPercentile(zScore),
			"significance": ai.getSignificanceLevel(zScore),
		}
	}
	
	return &StatisticalContext{
		ZScores:           zScores,
		DataQuality:       dataQuality,
		WindowInfo:        windowInfo,
		DistributionStats: distributionStats,
	}
}

// generateThresholdContext 生成动态阈值上下文
func (ai *AIInterfaceV12) generateThresholdContext(symbol string) *ThresholdContext {
	thresholds := ai.dynamicThresholdSystem.GetThresholds(symbol)
	if thresholds == nil {
		// 返回默认阈值上下文
		return &ThresholdContext{
			MarketRegime:     "unknown",
			RegimeConfidence: 0.0,
			CVDThresholds:    make(map[string]float64),
			VolumeThresholds: make(map[string]float64),
			OrderbookThresholds: make(map[string]float64),
			PriceThresholds:  make(map[string]float64),
			VolatilityLevel:  1.0,
			TrendStrength:    0.0,
			LiquidityLevel:   1.0,
			ManipulationRisk: 0.0,
			RecentAdjustments: []ThresholdAdjustment{},
		}
	}
	
	// 计算状态置信度
	regimeConfidence := ai.calculateRegimeConfidence(thresholds)
	
	return &ThresholdContext{
		MarketRegime:        string(thresholds.CurrentRegime),
		RegimeConfidence:    regimeConfidence,
		CVDThresholds:       thresholds.CVDThresholds,
		VolumeThresholds:    thresholds.VolumeThresholds,
		OrderbookThresholds: thresholds.OrderbookThresholds,
		PriceThresholds:     thresholds.PriceThresholds,
		VolatilityLevel:     thresholds.VolatilityLevel,
		TrendStrength:       thresholds.TrendStrength,
		LiquidityLevel:      thresholds.LiquidityLevel,
		ManipulationRisk:    thresholds.ManipulationRisk,
		RecentAdjustments:   []ThresholdAdjustment{}, // TODO: 实现阈值调整历史跟踪
	}
}

// generateSentinelContext 生成哨兵警报上下文
func (ai *AIInterfaceV12) generateSentinelContext(symbol string) *SentinelContext {
	// 生成场景检测状态
	scenarioStatus := map[string]ScenarioDetectionStatus{
		string(ScenarioMomentumIgnition): {
			LastCheck:  time.Now(),
			IsActive:   false,
			Confidence: 0.0,
			Cooldown:   0,
		},
		string(ScenarioSpotRush): {
			LastCheck:  time.Now(),
			IsActive:   false,
			Confidence: 0.0,
			Cooldown:   0,
		},
		string(ScenarioSpoofingFlip): {
			LastCheck:  time.Now(),
			IsActive:   false,
			Confidence: 0.0,
			Cooldown:   0,
		},
	}
	
	// 评估威胁等级
	threatLevel := ai.assessThreatLevel(symbol)
	
	return &SentinelContext{
		RecentAlerts:   []SentinelAlertSummary{}, // TODO: 实现最近警报历史跟踪
		ThreatLevel:    threatLevel,
		ScenarioStatus: scenarioStatus,
		AlertStats: AlertStatistics{
			Total24h:        0, // TODO: 实现24小时警报统计
			MomentumIgnition: 0,
			SpotRush:        0,
			SpoofingFlip:    0,
			AvgConfidence:   0.0,
		},
	}
}

// generateMicrostructureContext 生成市场微观结构上下文
func (ai *AIInterfaceV12) generateMicrostructureContext(symbol string, snapshot *MarketSnapshot) *MicrostructureContext {
	context := &MicrostructureContext{}
	
	// 生成增强CVD数据
	if snapshot.CVDDelta5m != nil {
		zScores := ai.globalStatsManager.GetSymbolZScores(symbol, snapshot)
		
		context.CVDDelta5mEnhanced = &CVDDelta5mEnhanced{
			CVDDelta5m: snapshot.CVDDelta5m,
			ZScoreNormalized: map[string]float64{
				"spot_cvd":    zScores["spot_cvd_1m"],
				"futures_cvd": zScores["futures_cvd_1m"],
				"volume":      zScores["volume_1m"],
			},
			StatisticalRank: map[string]float64{
				"spot_cvd":    ai.zScoreToPercentile(zScores["spot_cvd_1m"]),
				"futures_cvd": ai.zScoreToPercentile(zScores["futures_cvd_1m"]),
				"volume":      ai.zScoreToPercentile(zScores["volume_1m"]),
			},
			AnomalyScore: ai.calculateAnomalyScore(zScores),
			RegimeAdjusted: ai.applyRegimeAdjustment(symbol, zScores),
		}
	}
	
	// 生成增强盘口数据
	if snapshot.OrderBookData != nil {
		context.OrderBookEnhanced = &OrderBookEnhanced{
			OrderBookData:          snapshot.OrderBookData,
			LiquidityDistribution:  ai.calculateLiquidityDistribution(snapshot.OrderBookData),
			PriceImpactEstimate:    ai.calculatePriceImpactEstimate(snapshot.OrderBookData),
			MarketDepthAnalysis:    ai.analyzeMarketDepth(snapshot.OrderBookData),
			ManipulationIndicators: ai.calculateManipulationIndicators(snapshot.OrderBookData),
		}
	}
	
	// 生成增强OI数据
	if snapshot.OIAnalysis != nil {
		context.OIAnalysisEnhanced = &OIAnalysisEnhanced{
			OIAnalysis:          snapshot.OIAnalysis,
			OIVelocity:         ai.calculateOIVelocity(snapshot.OIAnalysis),
			OIAcceleration:     ai.calculateOIAcceleration(snapshot.OIAnalysis),
			TrendConsistency:   ai.calculateTrendConsistency(snapshot.OIAnalysis),
			MarketParticipation: ai.calculateMarketParticipation(snapshot.OIAnalysis),
		}
	}
	
	// 生成宏观趋势数据
	context.MacroTrends = &MacroTrendsEnhanced{
		TrendMultiTimeframe: ai.calculateMultiTimeframeTrends(symbol),
		MoneyFlowStrength:   ai.calculateMoneyFlowStrength(symbol, snapshot),
		MarketDominance:     ai.calculateMarketDominance(symbol, snapshot),
		CyclicalPatterns:    ai.identifyCyclicalPatterns(symbol),
	}
	
	return context
}

// generateDecisionSupportContext 生成决策支持上下文
func (ai *AIInterfaceV12) generateDecisionSupportContext(symbol string, snapshot *MarketSnapshot, context *AIContextV12) *DecisionSupportContext {
	// 计算信号强度
	signalStrength := ai.calculateSignalStrength(context)
	
	// 生成风险评估
	riskAssessment := ai.generateRiskAssessment(context)
	
	// 生成时机评估
	timingAssessment := ai.generateTimingAssessment(context)
	
	// 生成建议操作
	suggestedActions := ai.generateActionSuggestions(context, signalStrength, riskAssessment, timingAssessment)
	
	// 校准置信度
	confidenceCalibration := ai.calibrateConfidence(context, signalStrength)
	
	return &DecisionSupportContext{
		SignalStrength:        signalStrength,
		RiskAssessment:        riskAssessment,
		TimingAssessment:     timingAssessment,
		SuggestedActions:     suggestedActions,
		ConfidenceCalibration: confidenceCalibration,
	}
}

// ToJSON 将AI上下文转换为JSON格式（供AI模型使用）
func (context *AIContextV12) ToJSON() ([]byte, error) {
	return json.MarshalIndent(context, "", "  ")
}

// ToAIPromptFormat 转换为AI提示格式
func (context *AIContextV12) ToAIPromptFormat() map[string]interface{} {
	return map[string]interface{}{
		"V-12.2订单流分析系统": map[string]interface{}{
			"协议版本": context.ProtocolVersion,
			"交易对":   context.Symbol,
			"分析时间": context.Timestamp.Format("15:04:05"),
			
			"统计学上下文": map[string]interface{}{
				"Z-Score指标":   context.StatisticalContext.ZScores,
				"数据质量评估":    context.StatisticalContext.DataQuality,
				"分布统计特征":    context.StatisticalContext.DistributionStats,
			},
			
			"动态阈值系统": map[string]interface{}{
				"当前市场状态":    context.DynamicThresholds.MarketRegime,
				"状态置信度":     fmt.Sprintf("%.1f%%", context.DynamicThresholds.RegimeConfidence*100),
				"波动性水平":     context.DynamicThresholds.VolatilityLevel,
				"趋势强度":      context.DynamicThresholds.TrendStrength,
				"流动性水平":     context.DynamicThresholds.LiquidityLevel,
				"操纵风险":      context.DynamicThresholds.ManipulationRisk,
				"动态阈值": map[string]interface{}{
					"CVD阈值":  context.DynamicThresholds.CVDThresholds,
					"成交量阈值": context.DynamicThresholds.VolumeThresholds,
					"盘口阈值":  context.DynamicThresholds.OrderbookThresholds,
					"价格阈值":  context.DynamicThresholds.PriceThresholds,
				},
			},
			
			"哨兵监控系统": map[string]interface{}{
				"威胁等级":    context.SentinelAlerts.ThreatLevel,
				"场景检测状态": context.SentinelAlerts.ScenarioStatus,
				"警报统计":    context.SentinelAlerts.AlertStats,
			},
			
			"市场微观结构": func() map[string]interface{} {
				microstructure := make(map[string]interface{})
				
				if context.MarketMicrostructure.CVDDelta5mEnhanced != nil {
					microstructure["CVD增量分析"] = map[string]interface{}{
						"基础数据":      context.MarketMicrostructure.CVDDelta5mEnhanced.CVDDelta5m,
						"Z-Score标准化": context.MarketMicrostructure.CVDDelta5mEnhanced.ZScoreNormalized,
						"统计排名":      context.MarketMicrostructure.CVDDelta5mEnhanced.StatisticalRank,
						"异常评分":      context.MarketMicrostructure.CVDDelta5mEnhanced.AnomalyScore,
						"状态调整后":     context.MarketMicrostructure.CVDDelta5mEnhanced.RegimeAdjusted,
					}
				}
				
				if context.MarketMicrostructure.OrderBookEnhanced != nil {
					microstructure["盘口深度分析"] = map[string]interface{}{
						"基础数据":       context.MarketMicrostructure.OrderBookEnhanced.OrderBookData,
						"流动性分布":      context.MarketMicrostructure.OrderBookEnhanced.LiquidityDistribution,
						"价格冲击预估":     context.MarketMicrostructure.OrderBookEnhanced.PriceImpactEstimate,
						"市场深度分析":     context.MarketMicrostructure.OrderBookEnhanced.MarketDepthAnalysis,
						"操纵行为指标":     context.MarketMicrostructure.OrderBookEnhanced.ManipulationIndicators,
					}
				}
				
				microstructure["宏观趋势"] = context.MarketMicrostructure.MacroTrends
				
				return microstructure
			}(),
			
			"AI决策支持": map[string]interface{}{
				"综合信号强度":    fmt.Sprintf("%.1f/100", context.DecisionSupport.SignalStrength),
				"风险评估":       context.DecisionSupport.RiskAssessment,
				"市场时机评估":     context.DecisionSupport.TimingAssessment,
				"建议操作":       context.DecisionSupport.SuggestedActions,
				"置信度校准":      context.DecisionSupport.ConfidenceCalibration,
			},
		},
	}
}

// ===== 辅助计算方法 =====

// zScoreToPercentile 将Z-Score转换为百分位数
func (ai *AIInterfaceV12) zScoreToPercentile(zScore float64) float64 {
	// 使用标准正态分布近似
	// 简化实现：线性映射
	if zScore >= 3.0 {
		return 0.999
	} else if zScore <= -3.0 {
		return 0.001
	}
	
	// 使用近似公式
	return 0.5 + 0.16*zScore - 0.01*zScore*zScore*zScore
}

// getSignificanceLevel 获取Z-Score显著性水平
func (ai *AIInterfaceV12) getSignificanceLevel(zScore float64) string {
	absZ := math.Abs(zScore)
	if absZ >= 2.58 {
		return "高度显著 (p<0.01)"
	} else if absZ >= 1.96 {
		return "显著 (p<0.05)"
	} else if absZ >= 1.28 {
		return "边际显著 (p<0.10)"
	}
	return "不显著"
}

// calculateRegimeConfidence 计算市场状态置信度
func (ai *AIInterfaceV12) calculateRegimeConfidence(thresholds *SymbolThresholds) float64 {
	// 基于多个指标的一致性计算置信度
	confidence := 0.0
	
	// 波动性置信度 (25%)
	if thresholds.VolatilityLevel > 2.0 {
		confidence += 0.25
	}
	
	// 趋势强度置信度 (25%)
	if math.Abs(thresholds.TrendStrength) > 1.5 {
		confidence += 0.25
	}
	
	// 流动性置信度 (25%)
	if math.Abs(thresholds.LiquidityLevel) > 1.0 {
		confidence += 0.25
	}
	
	// 操纵风险置信度 (25%)
	if thresholds.ManipulationRisk > 0.5 {
		confidence += 0.25
	}
	
	return math.Min(1.0, confidence)
}

// assessThreatLevel 评估威胁等级
func (ai *AIInterfaceV12) assessThreatLevel(symbol string) string {
	// 获取动态阈值
	thresholds := ai.dynamicThresholdSystem.GetThresholds(symbol)
	if thresholds == nil {
		return "low"
	}
	
	// 基于多个风险因子评估
	riskScore := 0.0
	
	if thresholds.ManipulationRisk > 0.8 {
		riskScore += 0.4
	}
	if thresholds.VolatilityLevel > 2.5 {
		riskScore += 0.3
	}
	if thresholds.LiquidityLevel < -1.5 {
		riskScore += 0.3
	}
	
	if riskScore >= 0.8 {
		return "critical"
	} else if riskScore >= 0.5 {
		return "high"
	} else if riskScore >= 0.2 {
		return "medium"
	}
	return "low"
}

// 其他辅助方法的简化实现...
func (ai *AIInterfaceV12) calculateAnomalyScore(zScores map[string]float64) float64 {
	totalScore := 0.0
	count := 0
	for _, zScore := range zScores {
		totalScore += math.Abs(zScore)
		count++
	}
	if count > 0 {
		return math.Min(10.0, totalScore/float64(count))
	}
	return 0.0
}

func (ai *AIInterfaceV12) applyRegimeAdjustment(symbol string, zScores map[string]float64) map[string]float64 {
	adjusted := make(map[string]float64)
	thresholds := ai.dynamicThresholdSystem.GetThresholds(symbol)
	
	for key, value := range zScores {
		// 简化的状态调整
		adjustment := 1.0
		if thresholds != nil && thresholds.CurrentRegime == RegimeHighVolatility {
			adjustment = 0.8 // 高波动环境下降权
		}
		adjusted[key] = value * adjustment
	}
	
	return adjusted
}

// 简化实现的其他方法
func (ai *AIInterfaceV12) calculateLiquidityDistribution(orderBookData *OrderBookData) map[string]float64 {
	return map[string]float64{
		"bid_concentration": 0.0,
		"ask_concentration": 0.0,
		"spread_ratio":      0.0,
	}
}

func (ai *AIInterfaceV12) calculatePriceImpactEstimate(orderBookData *OrderBookData) map[string]float64 {
	return map[string]float64{
		"small_order":  0.0,
		"medium_order": 0.0,
		"large_order":  0.0,
	}
}

func (ai *AIInterfaceV12) analyzeMarketDepth(orderBookData *OrderBookData) map[string]interface{} {
	return map[string]interface{}{
		"total_depth": 0.0,
		"depth_ratio": 0.0,
	}
}

func (ai *AIInterfaceV12) calculateManipulationIndicators(orderBookData *OrderBookData) map[string]float64 {
	return map[string]float64{
		"spoofing_score": orderBookData.SpoofingRisk,
		"layering_score": 0.0,
		"momentum_ignition_score": 0.0,
	}
}

func (ai *AIInterfaceV12) calculateOIVelocity(oiAnalysis *OIAnalysis) float64 {
	return oiAnalysis.ChangeRate1H / 60.0 // 每分钟变化率
}

func (ai *AIInterfaceV12) calculateOIAcceleration(oiAnalysis *OIAnalysis) float64 {
	return oiAnalysis.ChangeRate1H - oiAnalysis.ChangeRate4H/4.0 // 简化加速度
}

func (ai *AIInterfaceV12) calculateTrendConsistency(oiAnalysis *OIAnalysis) float64 {
	// 简化的趋势一致性计算
	if oiAnalysis.ChangeRate1H*oiAnalysis.ChangeRate4H > 0 {
		return math.Min(1.0, math.Abs(oiAnalysis.ChangeRate1H/oiAnalysis.ChangeRate4H))
	}
	return 0.0
}

func (ai *AIInterfaceV12) calculateMarketParticipation(oiAnalysis *OIAnalysis) float64 {
	// 基于持仓量变化的市场参与度
	return math.Min(1.0, math.Abs(oiAnalysis.ChangeRate1H)/10.0)
}

func (ai *AIInterfaceV12) calculateMultiTimeframeTrends(symbol string) map[string]float64 {
	return map[string]float64{
		"trend_1m":  0.0,
		"trend_5m":  0.0,
		"trend_15m": 0.0,
		"trend_1h":  0.0,
	}
}

func (ai *AIInterfaceV12) calculateMoneyFlowStrength(symbol string, snapshot *MarketSnapshot) map[string]float64 {
	if snapshot.CVDDelta5m == nil {
		return map[string]float64{
			"spot_strength":    0.0,
			"futures_strength": 0.0,
		}
	}
	
	return map[string]float64{
		"spot_strength":    math.Abs(snapshot.CVDDelta5m.SpotCVDDeltaUSD) / 1000000.0,    // 归一化到百万USD
		"futures_strength": math.Abs(snapshot.CVDDelta5m.FuturesCVDDeltaUSD) / 1000000.0, // 归一化到百万USD
	}
}

func (ai *AIInterfaceV12) calculateMarketDominance(symbol string, snapshot *MarketSnapshot) map[string]float64 {
	if snapshot.CVDDelta5m == nil {
		return map[string]float64{
			"spot_dominance":    0.5,
			"futures_dominance": 0.5,
		}
	}
	
	totalCVD := math.Abs(snapshot.CVDDelta5m.SpotCVDDeltaUSD) + math.Abs(snapshot.CVDDelta5m.FuturesCVDDeltaUSD)
	if totalCVD == 0 {
		return map[string]float64{
			"spot_dominance":    0.5,
			"futures_dominance": 0.5,
		}
	}
	
	spotDominance := math.Abs(snapshot.CVDDelta5m.SpotCVDDeltaUSD) / totalCVD
	return map[string]float64{
		"spot_dominance":    spotDominance,
		"futures_dominance": 1.0 - spotDominance,
	}
}

func (ai *AIInterfaceV12) identifyCyclicalPatterns(symbol string) map[string]interface{} {
	return map[string]interface{}{
		"detected_patterns": []string{},
		"cycle_strength":    0.0,
		"next_expected":     "unknown",
	}
}

func (ai *AIInterfaceV12) calculateSignalStrength(context *AIContextV12) float64 {
	score := 0.0
	count := 0
	
	// 基于Z-Score计算信号强度
	for _, zScore := range context.StatisticalContext.ZScores {
		score += math.Min(10.0, math.Abs(zScore))
		count++
	}
	
	if count > 0 {
		return math.Min(100.0, (score/float64(count))*10)
	}
	return 0.0
}

func (ai *AIInterfaceV12) generateRiskAssessment(context *AIContextV12) *RiskAssessment {
	overallRisk := context.DynamicThresholds.ManipulationRisk*30 + 
		math.Min(50.0, context.DynamicThresholds.VolatilityLevel*20) + 
		math.Max(0, -context.DynamicThresholds.LiquidityLevel*20)
	
	return &RiskAssessment{
		OverallRisk: math.Min(100.0, overallRisk),
		RiskFactors: map[string]float64{
			"manipulation_risk": context.DynamicThresholds.ManipulationRisk,
			"volatility_risk":   context.DynamicThresholds.VolatilityLevel,
			"liquidity_risk":    -context.DynamicThresholds.LiquidityLevel,
		},
		RiskMitigation: []string{
			"使用较小仓位",
			"设置紧密止损",
			"监控流动性变化",
		},
	}
}

func (ai *AIInterfaceV12) generateTimingAssessment(context *AIContextV12) *TimingAssessment {
	return &TimingAssessment{
		MarketTiming:     50.0, // 简化实现
		VolatilityTiming: math.Min(100.0, 100.0-context.DynamicThresholds.VolatilityLevel*20),
		LiquidityTiming:  math.Min(100.0, 50.0+context.DynamicThresholds.LiquidityLevel*25),
		TrendAlignment:   math.Min(100.0, math.Abs(context.DynamicThresholds.TrendStrength)*30),
	}
}

func (ai *AIInterfaceV12) generateActionSuggestions(context *AIContextV12, signalStrength float64, risk *RiskAssessment, timing *TimingAssessment) []ActionSuggestion {
	suggestions := []ActionSuggestion{}
	
	// 🔥 业务逻辑修正: 为突发模式添加ATR目标位支持
	isBreakoutMode := ai.isBreakoutMode(context)
	
	if signalStrength > 70 && risk.OverallRisk < 50 {
		reasoning := "强信号且风险可控"
		if isBreakoutMode {
			reasoning = "突破信号强劲，建议使用ATR动态目标位 (TP=Entry+2*ATR, 默认R=2.0)"
		}
		
		suggestions = append(suggestions, ActionSuggestion{
			ActionType: "buy",
			Confidence: signalStrength / 100.0,
			Reasoning:  reasoning,
			Priority:   "high",
			Timeframe:  "immediate",
		})
	} else if risk.OverallRisk > 80 {
		suggestions = append(suggestions, ActionSuggestion{
			ActionType: "reduce",
			Confidence: risk.OverallRisk / 100.0,
			Reasoning:  "风险过高建议减仓",
			Priority:   "high",
			Timeframe:  "immediate",
		})
	} else if signalStrength > 50 && isBreakoutMode {
		// 🔥 新增: 突破模式的中等信号也值得考虑
		suggestions = append(suggestions, ActionSuggestion{
			ActionType: "buy",
			Confidence: (signalStrength - 20) / 100.0, // 调整置信度
			Reasoning:  "突破模式下的中等信号，使用ATR止盈，重点关注止损位设置",
			Priority:   "medium",
			Timeframe:  "immediate",
		})
	} else {
		suggestions = append(suggestions, ActionSuggestion{
			ActionType: "hold",
			Confidence: 0.6,
			Reasoning:  "信号或风险不明确",
			Priority:   "medium",
			Timeframe:  "short",
		})
	}
	
	return suggestions
}

// isBreakoutMode 判断是否为突破模式
func (ai *AIInterfaceV12) isBreakoutMode(context *AIContextV12) bool {
	// 多个条件综合判断突破模式
	breakoutScore := 0.0
	
	// 条件1: 动量点燃场景
	if len(context.SentinelAlerts.RecentAlerts) > 0 {
		for _, alert := range context.SentinelAlerts.RecentAlerts {
			if alert.Scenario == ScenarioMomentumIgnition && alert.Confidence > 0.7 {
				breakoutScore += 0.4
			}
		}
	}
	
	// 条件2: 价格异常变化
	if context.MarketMicrostructure != nil && context.MarketMicrostructure.CVDDelta5mEnhanced != nil {
		priceChange := math.Abs(context.MarketMicrostructure.CVDDelta5mEnhanced.CVDDelta5m.PriceDeltaPct)
		if priceChange > 1.5 { // 单周期价格变化>1.5%
			breakoutScore += 0.3
		}
	}
	
	// 条件3: 高Z-Score
	if context.StatisticalContext != nil {
		extremeZScores := 0
		for _, zScore := range context.StatisticalContext.ZScores {
			if math.Abs(zScore) > 2.5 {
				extremeZScores++
			}
		}
		if extremeZScores >= 2 { // 至少2个指标Z-Score > 2.5
			breakoutScore += 0.3
		}
	}
	
	return breakoutScore >= 0.6 // 综合评分>=0.6认为是突破模式
}

func (ai *AIInterfaceV12) calibrateConfidence(context *AIContextV12, signalStrength float64) *ConfidenceCalibration {
	rawConfidence := signalStrength / 100.0
	
	// 基于市场状态调整置信度
	adjustment := 1.0
	switch context.DynamicThresholds.MarketRegime {
	case "high_volatility":
		adjustment = 0.8
	case "manipulation":
		adjustment = 0.6
	case "low_liquidity":
		adjustment = 0.7
	}
	
	calibratedConfidence := rawConfidence * adjustment
	
	return &ConfidenceCalibration{
		RawConfidence:       rawConfidence,
		CalibratedConfidence: calibratedConfidence,
		CalibrationFactors: map[string]float64{
			"market_regime_adjustment": adjustment,
			"data_quality_factor":      1.0,
			"historical_accuracy":      0.75, // 假设历史准确率
		},
		HistoricalAccuracy: 0.75,
	}
}

// ===== 全局AI接口实例 =====

var globalAIInterfaceV12 *AIInterfaceV12
var aiInterfaceOnce sync.Once

// GetGlobalAIInterfaceV12 获取全局AI接口实例
func GetGlobalAIInterfaceV12() *AIInterfaceV12 {
	aiInterfaceOnce.Do(func() {
		globalAIInterfaceV12 = NewAIInterfaceV12()
		log.Printf("✨ V-12.2 AI接口全局实例已创建")
	})
	return globalAIInterfaceV12
}