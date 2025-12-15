package microstructure

import (
	"fmt"
	"log"
	"time"
)

// ===== P0-07修复：标准化AI输出接口 =====

// AIPayloadStandard 标准化AI输出接口（P0-07修复）
type AIPayloadStandard struct {
	Version          string                 `json:"version"`
	Symbol           string                 `json:"symbol"`
	Timestamp        time.Time              `json:"timestamp"`
	SystemStatus     AISystemStatus         `json:"system_status"`
	MainContent      map[string]interface{} `json:"main_content"`
	QualityStatus    string                 `json:"quality_status"` // NORMAL, DEGRADED, FATAL
	CompatMode       string                 `json:"compat_mode"`    // V12.2, V2.0_FALLBACK
}

// AISystemStatus AI系统状态（P0-07修复）
type AISystemStatus struct {
	AIInterfaceAvailable  bool              `json:"ai_interface_available"`
	SystemMode            string            `json:"system_mode"`     // NORMAL, FALLBACK, FATAL
	DegradationReason     string            `json:"degradation_reason"`
	RecommendedAction     string            `json:"recommended_action"`
	QualityScore          float64           `json:"quality_score"`
	RiskFactors           []string          `json:"risk_factors"`
	LastAIInterfaceCheck  time.Time         `json:"last_ai_interface_check"`
	RecoveryInstructions  []string          `json:"recovery_instructions"`
}

// StandardizedFieldMapping 标准化字段映射（P0-07修复）
type StandardizedFieldMapping struct {
	// 确保主路径和兜底路径使用相同的字段名
	RootStructure    string            `json:"root_structure"`
	OrderFlowSection map[string]string `json:"orderflow_section"`
	OrderBookSection map[string]string `json:"orderbook_section"`
	DataQualitySection map[string]string `json:"data_quality_section"`
	MacroTrendSection map[string]string `json:"macro_trend_section"`
}

// GetStandardizedFieldMapping 🔥 P0-07修复：获取标准化字段映射
func GetStandardizedFieldMapping() *StandardizedFieldMapping {
	return &StandardizedFieldMapping{
		// 🔥 P0-07修复：统一使用V-12.2的字段结构
		RootStructure: "V-12.2订单流分析系统",
		
		OrderFlowSection: map[string]string{
			"root_key":            "市场微观结构",
			"cvd_analysis":        "CVD增量分析",
			"period_analysis":     "本周期博弈_5m",
			"volume_analysis":     "成交量分析",
			"anomaly_detection":   "异常检测",
		},
		
		OrderBookSection: map[string]string{
			"root_key":            "盘口深度分析",
			"imbalance_ratio":     "失衡比例",
			"resistance_wall":     "阻力墙分析",
			"support_wall":        "支撑墙分析",
			"liquidity_analysis":  "流动性分析",
			"manipulation_risk":   "操纵风险评估",
		},
		
		DataQualitySection: map[string]string{
			"root_key":            "统计学上下文",
			"data_quality":        "数据质量评估",
			"reliability_scores":  "可靠性评分",
			"system_health":       "系统健康度",
			"confidence_level":    "置信度校准",
		},
		
		MacroTrendSection: map[string]string{
			"root_key":            "宏观趋势分析",
			"market_regime":       "市场体制",
			"trend_analysis":      "趋势分析",
			"flow_analysis":       "资金流分析",
			"sentiment_analysis":  "市场情绪",
		},
	}
}

// GenerateStandardizedAIPayload 🔥 P0-07修复：生成标准化AI输出
func GenerateStandardizedAIPayload(ms *MarketSnapshot) *AIPayloadStandard {
	mapping := GetStandardizedFieldMapping()
	timestamp := time.Now()
	
	// 🔥 P0-07修复：检测AI接口状态
	aiInterface := GetGlobalAIInterfaceV12()
	systemStatus := evaluateAISystemStatus(aiInterface, timestamp)
	
	var mainContent map[string]interface{}
	var compatMode string
	
	if systemStatus.AIInterfaceAvailable {
		// 主路径：V-12.2 AI接口
		if contextV12, err := aiInterface.GenerateAIContext(ms.Symbol); err == nil {
			mainContent = contextV12.ToAIPromptFormat()
			compatMode = "V12.2"
		} else {
			// AI接口可用但生成失败，降级
			mainContent = generateFallbackContentWithStandardFields(ms, mapping)
			compatMode = "V2.0_FALLBACK"
			systemStatus.SystemMode = "FALLBACK"
			systemStatus.DegradationReason = fmt.Sprintf("AI接口生成失败: %v", err)
			systemStatus.QualityScore = 0.5
		}
	} else {
		// 兜底路径：使用标准化字段结构
		mainContent = generateFallbackContentWithStandardFields(ms, mapping)
		compatMode = "V2.0_FALLBACK"
	}
	
	// 🔥 P0-07修复：确定质量状态
	qualityStatus := determineQualityStatus(systemStatus, ms.DataQuality)
	
	return &AIPayloadStandard{
		Version:      "P0-07-fixed",
		Symbol:       ms.Symbol,
		Timestamp:    timestamp,
		SystemStatus: systemStatus,
		MainContent:  mainContent,
		QualityStatus: qualityStatus,
		CompatMode:   compatMode,
	}
}

// evaluateAISystemStatus 🔥 P0-07修复：评估AI系统状态
func evaluateAISystemStatus(aiInterface interface{}, checkTime time.Time) AISystemStatus {
	status := AISystemStatus{
		LastAIInterfaceCheck: checkTime,
		QualityScore:         1.0,
		RiskFactors:          []string{},
		RecoveryInstructions: []string{},
	}
	
	if aiInterface == nil {
		status.AIInterfaceAvailable = false
		status.SystemMode = "FALLBACK"
		status.DegradationReason = "AI接口不可用 - GetGlobalAIInterfaceV12() 返回 nil"
		status.RecommendedAction = "等待AI接口恢复，谨慎交易"
		status.QualityScore = 0.3
		status.RiskFactors = append(status.RiskFactors, "AI决策支持不可用", "字段结构降级", "统计增强功能缺失")
		status.RecoveryInstructions = append(status.RecoveryInstructions, 
			"检查AI接口服务状态",
			"验证配置文件完整性", 
			"重启微服务组件",
			"联系技术支持")
	} else {
		status.AIInterfaceAvailable = true
		status.SystemMode = "NORMAL"
		status.DegradationReason = ""
		status.RecommendedAction = "系统正常运行"
		status.QualityScore = 1.0
	}
	
	return status
}

// generateFallbackContentWithStandardFields 🔥 P0-07修复：生成使用标准化字段的兜底内容
func generateFallbackContentWithStandardFields(ms *MarketSnapshot, mapping *StandardizedFieldMapping) map[string]interface{} {
	// 🔥 P0-07修复：使用与主路径完全相同的字段结构
	return map[string]interface{}{
		mapping.RootStructure: map[string]interface{}{
			"协议版本": "v12.2-fallback",
			"交易对":   ms.Symbol,
			"时间戳":   ms.Timestamp.Format("15:04:05"),
			"系统模式": "兜底模式",
			
			// 🔥 P0-07修复：统计学上下文（标准化字段）
			mapping.DataQualitySection["root_key"]: map[string]interface{}{
				mapping.DataQualitySection["data_quality"]: generateFallbackDataQuality(ms),
				mapping.DataQualitySection["system_health"]: "兜底模式运行",
				mapping.DataQualitySection["confidence_level"]: 0.3, // 兜底模式降低置信度
				"质量状态": "DEGRADED",
				"系统警告": "AI接口不可用，使用简化分析",
			},
			
			// 🔥 P0-07修复：动态阈值系统（兜底版本）
			"动态阈值系统": map[string]interface{}{
				"当前市场状态": "未知",
				"状态置信度":   0.0,
				"波动性水平":   "无法计算",
				"趋势强度":    0.0,
				"流动性水平":   "基础评估",
				"操纵风险":    "无法评估",
				"动态阈值": map[string]interface{}{
					"CVD阈值": "静态阈值",
					"成交量阈值": "静态阈值",
					"盘口阈值": "静态阈值",
				},
				"系统状态": "兜底模式",
			},
			
			// 🔥 P0-07修复：哨兵监控系统（兜底版本）
			"哨兵监控系统": map[string]interface{}{
				"威胁等级":    "无法评估",
				"场景检测状态": "禁用",
				"警报统计":    "不可用",
				"系统状态":    "降级运行",
			},
			
			// 🔥 P0-07修复：市场微观结构（使用标准化字段）
			mapping.OrderFlowSection["root_key"]: map[string]interface{}{
				mapping.OrderFlowSection["cvd_analysis"]: generateFallbackCVDAnalysis(ms),
				mapping.OrderBookSection["root_key"]: generateFallbackOrderBookAnalysis(ms, mapping),
				mapping.MacroTrendSection["root_key"]: generateFallbackMacroTrend(ms),
			},
			
			// 🔥 P0-07修复：AI决策支持（兜底版本）
			"AI决策支持": map[string]interface{}{
				"综合信号强度": 0.0,
				"风险评估":    "高风险 - AI不可用",
				"市场时机评估": "暂停评估",
				"建议操作":    "等待AI恢复",
				"置信度校准": map[string]interface{}{
					"基础置信度": 0.0,
					"系统降级": true,
					"推荐策略": "谨慎等待",
				},
				"系统状态": "兜底模式",
			},
		},
	}
}

// generateFallbackDataQuality 🔥 P0-07修复：生成兜底数据质量信息
func generateFallbackDataQuality(ms *MarketSnapshot) map[string]interface{} {
	// 🔥 P0-07修复：兜底模式下的数据质量必须明确标记为降级
	quality := map[string]interface{}{
		"整体评分": 0.3, // 兜底模式强制低分
		"CVD可靠性": func() float64 {
			if ms.CVDData != nil && !ms.CVDData.IsStale {
				return 0.7 // CVD数据可用但无AI增强
			}
			return 0.0
		}(),
		"盘口可靠性": func() float64 {
			if ms.OrderBookData != nil && !ms.OrderBookData.IsStale {
				return 0.7 // 盘口数据可用但无AI增强
			}
			return 0.0
		}(),
		"OI可靠性": func() float64 {
			if ms.OIAnalysis != nil && !ms.OIAnalysis.IsStale {
				return 0.7 // OI数据可用但无AI增强
			}
			return 0.0
		}(),
		"数据滞后毫秒": time.Since(ms.Timestamp).Milliseconds(),
		"状态":      "AI接口降级",
		"最后更新":    ms.Timestamp.Format("15:04:05"),
		// 🔥 P0-07修复：关键警告信息
		"系统警告":    "AI接口不可用，分析能力受限",
		"质量等级":    "DEGRADED",
		"建议操作":    "谨慎交易，等待AI接口恢复",
	}
	
	if ms.DataQuality != nil {
		// 使用现有数据质量但强制降级
		quality["CVD可靠性"] = ms.DataQuality.CVDReliability * 0.7      // 兜底模式降级30%
		quality["盘口可靠性"] = ms.DataQuality.OrderBookReliability * 0.7
		quality["OI可靠性"] = ms.DataQuality.OIReliability * 0.7
		quality["整体评分"] = ms.DataQuality.OverallScore * 0.5          // 兜底模式强制降级50%
	}
	
	return quality
}

// generateFallbackCVDAnalysis 🔥 P0-07修复：生成兜底CVD分析
func generateFallbackCVDAnalysis(ms *MarketSnapshot) map[string]interface{} {
	// 🔥 P0-07修复：保持与主路径相同的字段结构，但标记为简化版本
	cvdAnalysis := map[string]interface{}{
		"Z-Score标准化": map[string]interface{}{
			"现货CVD_1H": "无法计算",
			"期货CVD_1H": "无法计算",
			"标准化方法": "AI接口不可用",
		},
		"统计排名": map[string]interface{}{
			"历史百分位": 0.0,
			"显著性水平": 0.0,
			"排名有效性": false,
		},
		"异常评分": map[string]interface{}{
			"异常程度": 0.0,
			"检测状态": "禁用",
			"可信度": 0.0,
		},
		"简化分析": generateBasicCVDData(ms),
		"系统状态": "兜底模式 - 无统计增强",
	}
	
	return cvdAnalysis
}

// generateFallbackOrderBookAnalysis 🔥 P0-07修复：生成兜底盘口分析
func generateFallbackOrderBookAnalysis(ms *MarketSnapshot, mapping *StandardizedFieldMapping) map[string]interface{} {
	orderBook := map[string]interface{}{
		"流动性分布": map[string]interface{}{
			"分布类型": "基础分析",
			"集中度": 0.0,
			"深度评分": 0.0,
		},
		"操纵指标": map[string]interface{}{
			"虚假挂单风险": func() float64 {
				if ms.OrderBookData != nil {
					return ms.OrderBookData.SpoofingRisk
				}
				return 0.0
			}(),
			"墙操纵评分": 0.0,
			"检测状态": "简化模式",
		},
		mapping.OrderBookSection["imbalance_ratio"]: func() float64 {
			if ms.OrderBookData != nil {
				return ms.OrderBookData.ImbalanceRatio
			}
			return 0.0
		}(),
		mapping.OrderBookSection["resistance_wall"]: generateWallInfo(ms.OrderBookData, "resistance"),
		mapping.OrderBookSection["support_wall"]: generateWallInfo(ms.OrderBookData, "support"),
		"系统状态": "兜底模式 - 基础盘口分析",
	}
	
	return orderBook
}

// generateFallbackMacroTrend 🔥 P0-07修复：生成兜底宏观趋势
func generateFallbackMacroTrend(ms *MarketSnapshot) map[string]interface{} {
	macro := map[string]interface{}{
		"资金流向": map[string]interface{}{
			"主导方向": "无法确定",
			"流向强度": 0.0,
			"置信度": 0.0,
		},
		"趋势确认": map[string]interface{}{
			"趋势状态": "未知",
			"强度评分": 0.0,
			"可靠性": 0.0,
		},
	}
	
	// 使用基础数据填充
	if ms.MacroTrend != nil {
		macro["现货CVD_1H"] = ms.MacroTrend.SpotCVD1H
		macro["期货CVD_1H"] = ms.MacroTrend.FuturesCVD1H
		macro["市场体制"] = ms.MacroTrend.MarketRegime
		macro["主导方向"] = ms.MacroTrend.DominantDirection
	} else if ms.CVDData != nil {
		// 兜底到基础CVD数据
		macro["现货CVD_1H"] = ms.CVDData.SpotCVD1H
		macro["期货CVD_1H"] = ms.CVDData.FuturesCVD1H
		macro["CVD分歧"] = ms.CVDData.CVDDivergence
		macro["信号"] = ms.CVDData.Signal
	}
	
	macro["系统状态"] = "兜底模式 - 基础宏观分析"
	return macro
}

// generateBasicCVDData 生成基础CVD数据
func generateBasicCVDData(ms *MarketSnapshot) map[string]interface{} {
	if ms.CVDDelta5m != nil {
		return map[string]interface{}{
			"price_delta_pct":       ms.CVDDelta5m.PriceDeltaPct,
			"spot_cvd_delta_usd":    ms.CVDDelta5m.SpotCVDDeltaUSD,
			"futures_cvd_delta_usd": ms.CVDDelta5m.FuturesCVDDeltaUSD,
			"oi_delta_pct":          ms.CVDDelta5m.OIDeltaPct,
			"candle_intent":         ms.CVDDelta5m.CandleIntent,
			"volume_delta":          ms.CVDDelta5m.VolumeDelta,
			"volume_ratio":          ms.CVDDelta5m.VolumeRatio,
			"period_start":          ms.CVDDelta5m.PeriodStartTime.Format("15:04:05"),
			"period_end":            ms.CVDDelta5m.PeriodEndTime.Format("15:04:05"),
			"data_quality":          ms.CVDDelta5m.DataQuality,
		}
	}
	return map[string]interface{}{
		"status": "数据不可用",
	}
}

// generateWallInfo 生成墙信息
func generateWallInfo(orderBookData *OrderBookData, wallType string) interface{} {
	if orderBookData == nil {
		return map[string]interface{}{
			"status": "数据不可用",
			"reason": "盘口数据缺失",
		}
	}
	
	var wall *WallInfo
	if wallType == "resistance" && orderBookData.NearestResistance != nil {
		wall = orderBookData.NearestResistance
	} else if wallType == "support" && orderBookData.NearestSupport != nil {
		wall = orderBookData.NearestSupport
	}
	
	if wall != nil {
		return map[string]interface{}{
			"price":           wall.Price,
			"strength_usd":    wall.StrengthUSD,
			"is_solid":        wall.IsSolid,
			"distance_pct":    wall.Distance,
			"stability_score": wall.StabilityScore,
			"flicker_count":   wall.FlickerCount,
			"data_source":     "基础盘口分析",
		}
	}
	
	return map[string]interface{}{
		"status": "无墙检测",
		"reason": "当前价位无明显墙体结构",
	}
}

// determineQualityStatus 🔥 P0-07修复：确定质量状态
func determineQualityStatus(systemStatus AISystemStatus, dataQuality *DataQualityInfo) string {
	// 🔥 P0-07修复：AI接口不可用时必须标记为FATAL
	if !systemStatus.AIInterfaceAvailable {
		return "FATAL"
	}
	
	// 主路径可用但数据质量差
	if dataQuality != nil {
		if dataQuality.OverallScore < 0.3 {
			return "FATAL"
		} else if dataQuality.OverallScore < 0.6 {
			return "DEGRADED"
		}
	}
	
	return "NORMAL"
}

// LogSystemDegradation 🔥 P0-07修复：记录系统降级
func LogSystemDegradation(status AISystemStatus, symbol string) {
	if !status.AIInterfaceAvailable {
		log.Printf("🚨 [P0-07] AI系统降级警告 [%s]: %s", symbol, status.DegradationReason)
		log.Printf("   推荐操作: %s", status.RecommendedAction)
		log.Printf("   质量评分: %.2f", status.QualityScore)
		log.Printf("   风险因子: %v", status.RiskFactors)
		log.Printf("   恢复指南: %v", status.RecoveryInstructions)
		
		// 🔥 P0-07修复：关键警告
		log.Printf("⚠️ [P0-07] 兜底模式激活 - 字段结构已标准化但功能受限")
		log.Printf("⚠️ [P0-07] 建议立即检查AI接口状态，避免长期降级运行")
	}
}