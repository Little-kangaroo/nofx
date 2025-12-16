package market

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"
)

// ComprehensiveAnalyzer 综合市场分析器
type ComprehensiveAnalyzer struct {
	dowAnalyzer        *DowTheoryAnalyzer
	channelAnalyzer    *ChannelAnalyzer
	vpvrAnalyzer       *VPVRAnalyzer
	sdAnalyzer         *SupplyDemandAnalyzer
	fvgAnalyzer        *FVGAnalyzer
	fibonacciAnalyzer  *FibonacciAnalyzer
	srAnalyzer         *SupportResistanceAnalyzer
	config             *ComprehensiveConfig
}

// ComprehensiveConfig 综合分析配置
type ComprehensiveConfig struct {
	EnableDowTheory           bool    `json:"enable_dow_theory"`            // 启用道氏理论
	EnableVPVR                bool    `json:"enable_vpvr"`                  // 启用VPVR
	EnableSupplyDemand        bool    `json:"enable_supply_demand"`         // 启用供需区
	EnableFVG                 bool    `json:"enable_fvg"`                   // 启用FVG
	EnableFibonacci           bool    `json:"enable_fibonacci"`             // 启用斐波纳契
	EnableSupportResistance   bool    `json:"enable_support_resistance"`    // 启用支撑阻力转换线
	WeightDowTheory           float64 `json:"weight_dow_theory"`            // 道氏理论权重
	WeightVPVR                float64 `json:"weight_vpvr"`                  // VPVR权重
	WeightSupplyDemand        float64 `json:"weight_supply_demand"`         // 供需区权重
	WeightFVG                 float64 `json:"weight_fvg"`                   // FVG权重
	WeightFibonacci           float64 `json:"weight_fibonacci"`             // 斐波纳契权重
	WeightSupportResistance   float64 `json:"weight_support_resistance"`    // 支撑阻力权重
	MinConfidence             float64 `json:"min_confidence"`               // 最小置信度
	MaxSignals                int     `json:"max_signals"`                  // 最大信号数量
}

// ComprehensiveResult 综合分析结果
type ComprehensiveResult struct {
	Symbol              string                    `json:"symbol"`                // 交易对
	Timestamp           int64                     `json:"timestamp"`             // 分析时间
	CurrentPrice        float64                   `json:"current_price"`         // 当前价格
	DowTheory           *DowTheoryData            `json:"dow_theory"`            // 道氏理论分析
	ChannelAnalysis     *ChannelData              `json:"channel_analysis"`      // 通道分析（独立指标）
	VolumeProfile       *VolumeProfile            `json:"volume_profile"`        // 成交量分布
	SupplyDemand        *SupplyDemandData         `json:"supply_demand"`         // 供需区分析
	FairValueGaps       *FVGData                  `json:"fair_value_gaps"`       // FVG分析
	Fibonacci           *FibonacciData            `json:"fibonacci"`             // 斐波纳契分析
	SupportResistance   *SupportResistanceData    `json:"support_resistance"`    // 支撑阻力转换线
	UnifiedSignals      []*UnifiedSignal          `json:"unified_signals"`       // 统一交易信号
	MarketStructure     *MarketStructure          `json:"market_structure"`      // 市场结构
	RiskAssessment      *RiskAssessment           `json:"risk_assessment"`       // 风险评估
	TradingAdvice       *TradingAdvice            `json:"trading_advice"`        // 交易建议
	Config              *ComprehensiveConfig      `json:"config"`                // 分析配置
	
	// 🔥 Gate2 结构聚合输出 - V-13.5规范
	StructureGate2      *StructureGate2           `json:"structure_gate2,omitempty"` // Gate2结构指标聚合结果
}

// UnifiedSignal 统一交易信号
type UnifiedSignal struct {
	ID               string          `json:"id"`                // 信号ID
	Type             UnifiedSignalType `json:"type"`            // 信号类型
	Action           SignalAction    `json:"action"`            // 建议动作
	Entry            float64         `json:"entry"`             // 入场价
	StopLoss         float64         `json:"stop_loss"`         // 止损价
	TakeProfit       float64         `json:"take_profit"`       // 止盈价
	RiskReward       float64         `json:"risk_reward"`       // 风险收益比
	Confidence       float64         `json:"confidence"`        // 综合置信度
	Strength         float64         `json:"strength"`          // 信号强度
	Sources          []SignalSource  `json:"sources"`           // 信号来源
	Description      string          `json:"description"`       // 信号描述
	TimeFrame        string          `json:"time_frame"`        // 时间框架
	Priority         SignalPriority  `json:"priority"`          // 信号优先级
	Timestamp        int64           `json:"timestamp"`         // 生成时间
}

// UnifiedSignalType 统一信号类型
type UnifiedSignalType string

const (
	UnifiedSignalTrendFollowing UnifiedSignalType = "trend_following" // 趋势跟随
	UnifiedSignalReversal       UnifiedSignalType = "reversal"        // 反转
	UnifiedSignalBreakout       UnifiedSignalType = "breakout"        // 突破
	UnifiedSignalSupport        UnifiedSignalType = "support"         // 支撑
	UnifiedSignalResistance     UnifiedSignalType = "resistance"      // 阻力
	UnifiedSignalMeanReversion  UnifiedSignalType = "mean_reversion"  // 均值回归
)

// SignalSource 信号来源
type SignalSource struct {
	Source     string  `json:"source"`     // 来源（dow_theory, vpvr, supply_demand）
	Weight     float64 `json:"weight"`     // 权重
	Confidence float64 `json:"confidence"` // 该来源的置信度
	Details    string  `json:"details"`    // 详细信息
}

// SignalPriority 信号优先级
type SignalPriority string

const (
	PriorityHigh   SignalPriority = "high"   // 高优先级
	PriorityMedium SignalPriority = "medium" // 中等优先级
	PriorityLow    SignalPriority = "low"    // 低优先级
)

// MarketStructure 市场结构
type MarketStructure struct {
	TrendDirection    TrendDirection  `json:"trend_direction"`    // 趋势方向
	TrendStrength     float64         `json:"trend_strength"`     // 趋势强度
	SupportLevels     []float64       `json:"support_levels"`     // 支撑位
	ResistanceLevels  []float64       `json:"resistance_levels"`  // 阻力位
	KeyLevels         []KeyLevel      `json:"key_levels"`         // 关键价位
	VolumeProfile     *VPSummary      `json:"volume_profile"`     // 成交量概况
	MarketPhase       MarketPhase     `json:"market_phase"`       // 市场阶段
	Volatility        float64         `json:"volatility"`         // 波动性
}

// KeyLevel 关键价位
type KeyLevel struct {
	Price       float64    `json:"price"`       // 价格
	Type        LevelType  `json:"type"`        // 类型
	Strength    float64    `json:"strength"`    // 强度
	Source      string     `json:"source"`      // 来源
	Description string     `json:"description"` // 描述
}

// LevelType 价位类型
type LevelType string

const (
	LevelSupport    LevelType = "support"    // 支撑
	LevelResistance LevelType = "resistance" // 阻力
	LevelPOC        LevelType = "poc"        // 控制点
	LevelVAH        LevelType = "vah"        // 价值区上沿
	LevelVAL        LevelType = "val"        // 价值区下沿
)

// VPSummary 成交量分布概况
type VPSummary struct {
	POC             float64 `json:"poc"`              // 控制点
	VAH             float64 `json:"vah"`              // 价值区上沿
	VAL             float64 `json:"val"`              // 价值区下沿
	VolumeProfile   string  `json:"volume_profile"`   // 分布形态
	Concentration   float64 `json:"concentration"`    // 集中度
	CurrentPosition string  `json:"current_position"` // 当前位置
}

// MarketPhase 市场阶段
type MarketPhase string

const (
	PhaseAccumulation MarketPhase = "accumulation" // 积累阶段
	PhaseMarkup      MarketPhase = "markup"       // 上涨阶段
	PhaseDistribution MarketPhase = "distribution" // 分发阶段
	PhaseMarkdown    MarketPhase = "markdown"     // 下跌阶段
	PhaseSideways    MarketPhase = "sideways"     // 横盘阶段
)

// RiskAssessment 风险评估
type RiskAssessment struct {
	OverallRisk       RiskLevel `json:"overall_risk"`       // 整体风险
	TrendRisk         RiskLevel `json:"trend_risk"`         // 趋势风险
	VolatilityRisk    RiskLevel `json:"volatility_risk"`    // 波动风险
	LiquidityRisk     RiskLevel `json:"liquidity_risk"`     // 流动性风险
	RecommendedRisk   float64   `json:"recommended_risk"`   // 建议风险百分比
	MaxPositionSize   float64   `json:"max_position_size"`  // 最大仓位
	SuggestedTimeFrame string   `json:"suggested_timeframe"` // 建议时间框架
	RiskFactors       []string  `json:"risk_factors"`       // 风险因素
}

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"    // 低风险
	RiskMedium RiskLevel = "medium" // 中等风险
	RiskHigh   RiskLevel = "high"   // 高风险
)

// TriggerContextInfo 触发上下文信息 - V13.5规范要求 (从decision包复制)
type TriggerContextInfo struct {
	IsKlineClosed     bool   `json:"is_kline_closed"`      // K线是否已收盘 (true=收盘触发, false=盘中触发)
	Mode              string `json:"mode"`                 // 触发模式 ("normal", "emergency")  
	AnchorCloseTime   int64  `json:"anchor_close_time"`    // 锚点收盘时间戳(毫秒)
	AnchorTimeStr     string `json:"anchor_time_str"`      // 锚点时间字符串(可读)
	TriggerType       string `json:"trigger_type"`         // 触发类型 ("5m_close", "manual", "scheduled")
	DataConsistency   string `json:"data_consistency"`     // 数据一致性状态 ("aligned", "mixed", "uncertain")
}

// TradingAdvice 交易建议
type TradingAdvice struct {
	OverallAction     SignalAction  `json:"overall_action"`     // 整体建议
	Confidence        float64       `json:"confidence"`         // 置信度
	ReasoningPoints   []string      `json:"reasoning_points"`   // 推理要点
	EntryStrategy     string        `json:"entry_strategy"`     // 入场策略
	ExitStrategy      string        `json:"exit_strategy"`      // 出场策略
	RiskManagement    string        `json:"risk_management"`    // 风险管理
	AlternativeScenarios []string   `json:"alternative_scenarios"` // 替代方案
	TimeHorizon       string        `json:"time_horizon"`       // 时间周期
}

// 默认综合分析配置
var defaultComprehensiveConfig = &ComprehensiveConfig{
	EnableDowTheory:         true,
	EnableVPVR:              true,
	EnableSupplyDemand:      true,
	EnableFVG:               true,
	EnableFibonacci:         true,
	EnableSupportResistance: true,
	WeightDowTheory:         0.20,
	WeightVPVR:              0.15,
	WeightSupplyDemand:      0.15,
	WeightFVG:               0.15,
	WeightFibonacci:         0.15,
	WeightSupportResistance: 0.20,
	MinConfidence:           60.0,
	MaxSignals:              6,
}

// NewComprehensiveAnalyzer 创建综合分析器
// 🔥 P0-04修复：使用fallback配置，建议通过NewComprehensiveAnalyzerWithExchange创建以支持动态VPVR配置
func NewComprehensiveAnalyzer() *ComprehensiveAnalyzer {
	return &ComprehensiveAnalyzer{
		dowAnalyzer:       NewDowTheoryAnalyzer(),
		channelAnalyzer:   NewChannelAnalyzer(),
		vpvrAnalyzer:      NewVPVRAnalyzer(), // ⚠️ 使用fallback配置，不推荐
		sdAnalyzer:        NewSupplyDemandAnalyzer(),
		fvgAnalyzer:       NewFVGAnalyzer(),
		fibonacciAnalyzer: NewFibonacciAnalyzer(),
		srAnalyzer:        NewSupportResistanceAnalyzer(),
		config:            defaultComprehensiveConfig,
	}
}

// NewComprehensiveAnalyzerWithConfig 使用自定义配置创建综合分析器
// 🔥 P0-04修复：使用fallback配置，建议通过NewComprehensiveAnalyzerWithExchange创建以支持动态VPVR配置
func NewComprehensiveAnalyzerWithConfig(config *ComprehensiveConfig) *ComprehensiveAnalyzer {
	return &ComprehensiveAnalyzer{
		dowAnalyzer:       NewDowTheoryAnalyzer(),
		channelAnalyzer:   NewChannelAnalyzer(),
		vpvrAnalyzer:      NewVPVRAnalyzer(), // ⚠️ 使用fallback配置，不推荐
		sdAnalyzer:        NewSupplyDemandAnalyzer(),
		fvgAnalyzer:       NewFVGAnalyzer(),
		fibonacciAnalyzer: NewFibonacciAnalyzer(),
		srAnalyzer:        NewSupportResistanceAnalyzer(),
		config:            config,
	}
}

// 🔥 P0-04修复：新增支持ExchangeMeta的构造函数，实现动态VPVR配置
// NewComprehensiveAnalyzerWithExchange 使用ExchangeMeta创建支持动态VPVR配置的综合分析器
func NewComprehensiveAnalyzerWithExchange(exchangeMeta *ExchangeMeta, config *ComprehensiveConfig) *ComprehensiveAnalyzer {
	if config == nil {
		config = defaultComprehensiveConfig
	}
	
	// 🔥 P0-04核心修复：使用ExchangeMeta创建动态配置的VPVR分析器
	// 注意：这里使用"4h"作为默认timeframe，在实际分析时会在analyzeSingleTimeframe中使用正确的timeframe
	var vpvrAnalyzer *VPVRAnalyzer
	if exchangeMeta != nil {
		vpvrAnalyzer = NewVPVRAnalyzerWithDynamicConfig(exchangeMeta, "4h") // 默认timeframe，实际使用时动态调整
	} else {
		vpvrAnalyzer = NewVPVRAnalyzer() // 降级到fallback配置
	}
	
	return &ComprehensiveAnalyzer{
		dowAnalyzer:       NewDowTheoryAnalyzer(),
		channelAnalyzer:   NewChannelAnalyzer(),
		vpvrAnalyzer:      vpvrAnalyzer, // ✅ 使用动态配置的VPVR分析器
		sdAnalyzer:        NewSupplyDemandAnalyzer(),
		fvgAnalyzer:       NewFVGAnalyzer(),
		fibonacciAnalyzer: NewFibonacciAnalyzer(),
		srAnalyzer:        NewSupportResistanceAnalyzer(),
		config:            config,
	}
}

// AnalyzeMultiTimeframe 执行多时间框架综合分析
func (ca *ComprehensiveAnalyzer) AnalyzeMultiTimeframe(symbol string, klines5m, klines15m, klines30m, klines1h, klines4h []Kline) *ComprehensiveResult {
	if len(klines5m) == 0 && len(klines4h) == 0 {
		return nil
	}

	currentPrice := 0.0
	timestamp := time.Now().UnixMilli()

	// 🔥 P0-03修复：统一约束currentPrice必须来自5m数据，与V-13.5的vacuum/trigger规则一致
	// 避免优先使用4h导致取到"未收盘4h close（动态变化）"造成全链路时间锚点错配
	if len(klines5m) > 0 {
		currentPrice = klines5m[len(klines5m)-1].Close  // 强制使用5m last closed price
		timestamp = klines5m[len(klines5m)-1].CloseTime
	} else if len(klines4h) > 0 {
		// 仅当5m数据不可用时才降级使用4h（紧急兼容模式）
		currentPrice = klines4h[len(klines4h)-1].Close
		timestamp = klines4h[len(klines4h)-1].CloseTime
		log.Printf("⚠️ [P0-03] 5m数据不可用，降级使用4h价格 - 可能影响时间锚点一致性")
	}

	// 🔥 Gate2 结构聚合 - 计算ATR14用于后续分析
	var atr14 float64 = 100.0 // 默认值，防止数据不足
	if len(klines4h) >= 14 {
		atr14 = calculateATR14(klines4h)
	} else if len(klines1h) >= 14 {
		atr14 = calculateATR14(klines1h) * 0.25 // 1h转4h近似
	}
	
	log.Printf("🎯 [Gate2] %s 开始结构聚合 - 当前价格: %.4f, ATR14: %.4f", symbol, currentPrice, atr14)

	result := &ComprehensiveResult{
		Symbol:       symbol,
		Timestamp:    timestamp,
		CurrentPrice: currentPrice,
		Config:       ca.config,
	}

	// 执行多时间框架分析
	multiTimeframeAnalysis := ca.AnalyzeAllTimeframes(symbol, currentPrice, map[string][]Kline{
		"5m":  klines5m,
		"15m": klines15m,
		"30m": klines30m,
		"1h":  klines1h,
		"4h":  klines4h,
	})

	// 🔥 Gate2 结构聚合 - 步骤1: 执行AnchorEngine聚合
	var structureGate2 *StructureGate2
	if multiTimeframeAnalysis != nil {
		// 获取管理器
		featureManager := GetGate2FeatureManager()
		performanceMonitor := GetGate2PerformanceMonitor()
		
		// 检查feature flag - 保留错误率和黑名单检查
		if featureManager.IsEnabled(symbol) {
			start := time.Now()
			
			// 执行Gate2结构聚合
			structureGate2, anchorEngineTime, structClassifierTime, triggerDetectorTime, err := ca.executeGate2StructureAggregationWithMonitoring(
				symbol, currentPrice, atr14, multiTimeframeAnalysis, klines5m)
			
			elapsed := time.Since(start).Seconds() * 1000 // 转换为毫秒
			
			// 记录性能监控数据
			performanceMonitor.RecordExecution(
				symbol, 
				elapsed,
				anchorEngineTime,
				structClassifierTime, 
				triggerDetectorTime,
				structureGate2.TopAnchorsLong,
				structureGate2.TopAnchorsShort,
				err,
			)
			
			// 记录详细锚点评分统计
			performanceMonitor.LogAnchorScoreDetails(symbol, structureGate2.TopAnchorsLong, structureGate2.TopAnchorsShort)
			
			// 记录性能日志
			if featureManager.IsPerformanceLogEnabled() {
				log.Printf("📊 [Gate2性能] %s 执行耗时: %.2fms, 锚点数: L=%d, S=%d", 
					symbol, elapsed, len(structureGate2.TopAnchorsLong), len(structureGate2.TopAnchorsShort))
				log.Printf("📊 [Gate2性能] %s 模块耗时: AnchorEngine=%.2fms, StructClassifier=%.2fms, TriggerDetector=%.2fms",
					symbol, anchorEngineTime, structClassifierTime, triggerDetectorTime)
			}
			
			if err != nil {
				featureManager.RecordError(err, symbol)
				log.Printf("❌ [Gate2] %s 执行失败: %v", symbol, err)
			} else {
				featureManager.RecordSuccess()
			}
		} else {
			// 只有在错误率超标或黑名单时才禁用，记录原因
			log.Printf("⚠️ [Gate2] %s 被禁用 - 可能原因: 错误率超标或在黑名单中", symbol)
			structureGate2 = &StructureGate2{
				TopAnchorsLong:    []AnchorCandidate{},
				TopAnchorsShort:   []AnchorCandidate{},
				StructStateLong:   StructStateNeutral,
				StructStateShort:  StructStateNeutral,
				TriggerResult:     nil,
			}
		}
	}

	// 向前兼容：提取4小时分析作为单一结果
	if tf4h, exists := multiTimeframeAnalysis.Timeframes["4h"]; exists {
		result.DowTheory = tf4h.DowTheory
		result.ChannelAnalysis = tf4h.ChannelAnalysis
		result.VolumeProfile = tf4h.VolumeProfile
		result.SupplyDemand = tf4h.SupplyDemand
		result.FairValueGaps = tf4h.FairValueGaps
		result.Fibonacci = tf4h.Fibonacci
		result.SupportResistance = tf4h.SupportResistance
	}

	// 生成统一信号
	result.UnifiedSignals = ca.generateUnifiedSignals(result, currentPrice)

	// 分析市场结构
	result.MarketStructure = ca.analyzeMarketStructure(result)

	// 评估风险
	result.RiskAssessment = ca.assessRisk(result)

	// 生成交易建议
	result.TradingAdvice = ca.generateTradingAdvice(result)

	// 🔥 Gate2 结构聚合 - 步骤2: 将Gate2结果附加到ComprehensiveResult
	if structureGate2 != nil {
		result.StructureGate2 = structureGate2
		log.Printf("🎯 [Gate2] %s 结构聚合完成 - LONG:%s SHORT:%s", 
			symbol, structureGate2.StructStateLong, structureGate2.StructStateShort)
	}

	// 将多时间框架分析添加到结果中（需要在ComprehensiveResult中添加该字段）
	// result.MultiTimeframeAnalysis = multiTimeframeAnalysis

	// 应用精度格式化
	ca.ApplyPrecisionFormatting(result, symbol)

	return result
}

// Analyze 执行综合市场分析（向前兼容）
func (ca *ComprehensiveAnalyzer) Analyze(symbol string, klines5m, klines4h []Kline) *ComprehensiveResult {
	if len(klines5m) == 0 && len(klines4h) == 0 {
		return nil
	}

	currentPrice := 0.0
	timestamp := time.Now().UnixMilli()

	// 🔥 P0-03修复：统一约束currentPrice必须来自5m数据，与V-13.5的vacuum/trigger规则一致
	// 避免优先使用4h导致取到"未收盘4h close（动态变化）"造成全链路时间锚点错配
	if len(klines5m) > 0 {
		currentPrice = klines5m[len(klines5m)-1].Close  // 强制使用5m last closed price
		timestamp = klines5m[len(klines5m)-1].CloseTime
	} else if len(klines4h) > 0 {
		// 仅当5m数据不可用时才降级使用4h（紧急兼容模式）
		currentPrice = klines4h[len(klines4h)-1].Close
		timestamp = klines4h[len(klines4h)-1].CloseTime
		log.Printf("⚠️ [P0-03] 5m数据不可用，降级使用4h价格 - 可能影响时间锚点一致性")
	}

	result := &ComprehensiveResult{
		Symbol:       symbol,
		Timestamp:    timestamp,
		CurrentPrice: currentPrice,
		Config:       ca.config,
	}

	// 执行道氏理论分析
	if ca.config.EnableDowTheory && len(klines4h) > 20 {
		result.DowTheory = ca.dowAnalyzer.Analyze(klines5m, klines4h, currentPrice)
		result.ChannelAnalysis = ca.channelAnalyzer.Analyze(klines4h, currentPrice)
	}

	// 执行VPVR分析
	// 🔥 P0-04修复：使用当前时间框架的动态VPVR配置进行分析
	if ca.config.EnableVPVR && len(klines4h) > 10 {
		// 🔥 P0-04核心修复：为4h时间框架动态创建VPVR分析器
		var currentVPVRAnalyzer *VPVRAnalyzer
		
		// 尝试从现有VPVR分析器获取配置中的ExchangeMeta
		existingConfig := ca.vpvrAnalyzer.GetConfig()
		
		// 创建4h时间框架的动态配置
		if existingConfig.TickSize > 0 && existingConfig.TickSize != fallbackVPVRConfig.TickSize {
			// 如果现有分析器使用了动态配置（不是fallback默认值），重建ExchangeMeta
			exchangeMeta := &ExchangeMeta{
				TickSize: existingConfig.TickSize,
				Symbol:   symbol,
			}
			currentVPVRAnalyzer = NewVPVRAnalyzerWithDynamicConfig(exchangeMeta, "4h")
		} else {
			// 使用现有分析器（可能是fallback配置）
			currentVPVRAnalyzer = ca.vpvrAnalyzer
		}
		
		result.VolumeProfile = currentVPVRAnalyzer.Analyze(klines4h)
		
		// 🔥 P0-04修复：在VolumeProfile输出中明确标注timeframe与tick_size
		if result.VolumeProfile != nil {
			// 确保UsedTimeFrame被正确设置为4h
			if result.VolumeProfile.UsedTimeFrame == "" {
				result.VolumeProfile.UsedTimeFrame = "4h"
			}
			// UsedTickSize已在VPVR分析器中设置
		}
	}

	// 执行供需区分析
	if ca.config.EnableSupplyDemand && len(klines4h) > 15 {
		result.SupplyDemand = ca.sdAnalyzer.AnalyzeWithSymbol(klines4h, symbol, "4h")
	}

	// 执行FVG分析
	if ca.config.EnableFVG && len(klines4h) > 10 {
		result.FairValueGaps = ca.fvgAnalyzer.Analyze(klines4h)
	}

	// 执行斐波纳契分析
	if ca.config.EnableFibonacci && len(klines4h) > 15 {
		result.Fibonacci = ca.fibonacciAnalyzer.Analyze(klines4h)
	}

	// 执行支撑阻力转换线分析
	if ca.config.EnableSupportResistance && len(klines4h) > 20 {
		result.SupportResistance = ca.srAnalyzer.Analyze(klines4h)
	}

	// 生成统一信号
	result.UnifiedSignals = ca.generateUnifiedSignals(result, currentPrice)

	// 分析市场结构
	result.MarketStructure = ca.analyzeMarketStructure(result)

	// 评估风险
	result.RiskAssessment = ca.assessRisk(result)

	// 生成交易建议
	result.TradingAdvice = ca.generateTradingAdvice(result)

	// 应用精度格式化
	ca.ApplyPrecisionFormatting(result, symbol)

	return result
}

// generateUnifiedSignals 生成统一交易信号
func (ca *ComprehensiveAnalyzer) generateUnifiedSignals(result *ComprehensiveResult, currentPrice float64) []*UnifiedSignal {
	var allSignals []*UnifiedSignal

	// 收集各个分析模块的信号
	dowSignals := ca.collectDowTheorySignals(result.DowTheory, currentPrice)
	vpvrSignals := ca.collectVPVRSignals(result.VolumeProfile, currentPrice)
	sdSignals := ca.collectSupplyDemandSignals(result.SupplyDemand, currentPrice)
	fvgSignals := ca.collectFVGSignals(result.FairValueGaps, currentPrice)
	fibSignals := ca.collectFibonacciSignals(result.Fibonacci, currentPrice)

	// 合并所有信号
	allSignals = append(allSignals, dowSignals...)
	allSignals = append(allSignals, vpvrSignals...)
	allSignals = append(allSignals, sdSignals...)
	allSignals = append(allSignals, fvgSignals...)
	allSignals = append(allSignals, fibSignals...)

	// 信号融合和去重
	fusedSignals := ca.fuseSignals(allSignals)

	// 过滤低置信度信号
	var finalSignals []*UnifiedSignal
	for _, signal := range fusedSignals {
		if signal.Confidence >= ca.config.MinConfidence {
			finalSignals = append(finalSignals, signal)
		}
	}

	// 按置信度排序
	sort.Slice(finalSignals, func(i, j int) bool {
		return finalSignals[i].Confidence > finalSignals[j].Confidence
	})

	// 限制信号数量
	if len(finalSignals) > ca.config.MaxSignals {
		finalSignals = finalSignals[:ca.config.MaxSignals]
	}

	return finalSignals
}

// collectDowTheorySignals 收集道氏理论信号
func (ca *ComprehensiveAnalyzer) collectDowTheorySignals(dowData *DowTheoryData, currentPrice float64) []*UnifiedSignal {
	var signals []*UnifiedSignal

	if dowData == nil || dowData.TradingSignal == nil {
		return signals
	}

	signal := dowData.TradingSignal
	unifiedSignal := &UnifiedSignal{
		ID:         fmt.Sprintf("dow_%d", time.Now().UnixNano()),
		Action:     signal.Action,
		Entry:      signal.Entry,
		StopLoss:   signal.StopLoss,
		TakeProfit: signal.TakeProfit,
		RiskReward: signal.RiskReward,
		Confidence: signal.Confidence,
		Sources: []SignalSource{
			{
				Source:     "dow_theory",
				Weight:     ca.config.WeightDowTheory,
				Confidence: signal.Confidence,
				Details:    signal.Description,
			},
		},
		Description: signal.Description,
		TimeFrame:   "4h",
		Timestamp:   signal.Timestamp,
	}

	// 确定信号类型
	switch signal.Type {
	case SignalChannelBounce:
		unifiedSignal.Type = UnifiedSignalSupport
	case SignalChannelBreakout:
		unifiedSignal.Type = UnifiedSignalBreakout
	case SignalTrendFollowing:
		unifiedSignal.Type = UnifiedSignalTrendFollowing
	case SignalReversal:
		unifiedSignal.Type = UnifiedSignalReversal
	}

	// 设置优先级
	if signal.Confidence >= 80 {
		unifiedSignal.Priority = PriorityHigh
	} else if signal.Confidence >= 60 {
		unifiedSignal.Priority = PriorityMedium
	} else {
		unifiedSignal.Priority = PriorityLow
	}

	signals = append(signals, unifiedSignal)
	return signals
}

// collectVPVRSignals 收集VPVR信号
func (ca *ComprehensiveAnalyzer) collectVPVRSignals(vpData *VolumeProfile, currentPrice float64) []*UnifiedSignal {
	var signals []*UnifiedSignal

	if vpData == nil {
		return signals
	}

	// 生成VPVR信号
	vpvrSignals := ca.vpvrAnalyzer.GenerateSignals(vpData, currentPrice)

	for _, signal := range vpvrSignals {
		unifiedSignal := &UnifiedSignal{
			ID:         fmt.Sprintf("vpvr_%d", time.Now().UnixNano()),
			Action:     signal.Action,
			Entry:      currentPrice, // VPVR信号通常基于当前价格
			Confidence: signal.Confidence,
			Sources: []SignalSource{
				{
					Source:     "vpvr",
					Weight:     ca.config.WeightVPVR,
					Confidence: signal.Confidence,
					Details:    signal.Description,
				},
			},
			Description: signal.Description,
			TimeFrame:   "4h",
			Timestamp:   signal.Timestamp,
		}

		// 根据VPVR信号类型设置统一信号类型
		switch signal.Type {
		case VPVRSignalPOCTest:
			unifiedSignal.Type = UnifiedSignalSupport
		case VPVRSignalVABreakout:
			unifiedSignal.Type = UnifiedSignalBreakout
		case VPVRSignalVAReturn:
			unifiedSignal.Type = UnifiedSignalMeanReversion
		case VPVRSignalHighVolume:
			unifiedSignal.Type = UnifiedSignalSupport
		}

		// 设置优先级
		if signal.Confidence >= 80 {
			unifiedSignal.Priority = PriorityHigh
		} else if signal.Confidence >= 60 {
			unifiedSignal.Priority = PriorityMedium
		} else {
			unifiedSignal.Priority = PriorityLow
		}

		signals = append(signals, unifiedSignal)
	}

	return signals
}

// collectSupplyDemandSignals 收集供需区信号
func (ca *ComprehensiveAnalyzer) collectSupplyDemandSignals(sdData *SupplyDemandData, currentPrice float64) []*UnifiedSignal {
	var signals []*UnifiedSignal

	if sdData == nil {
		return signals
	}

	// 生成供需区信号
	sdSignals := ca.sdAnalyzer.GenerateSignals(sdData, currentPrice)

	for _, signal := range sdSignals {
		unifiedSignal := &UnifiedSignal{
			ID:         fmt.Sprintf("sd_%d", time.Now().UnixNano()),
			Action:     signal.Action,
			Entry:      signal.Entry,
			StopLoss:   signal.StopLoss,
			TakeProfit: signal.TakeProfit,
			RiskReward: signal.RiskReward,
			Confidence: signal.Confidence,
			Strength:   signal.Strength,
			Sources: []SignalSource{
				{
					Source:     "supply_demand",
					Weight:     ca.config.WeightSupplyDemand,
					Confidence: signal.Confidence,
					Details:    signal.Description,
				},
			},
			Description: signal.Description,
			TimeFrame:   "4h",
			Timestamp:   signal.Timestamp,
		}

		// 根据供需区信号类型设置统一信号类型
		switch signal.Type {
		case SDSignalZoneBounce:
			if signal.Zone.Type == SupplyZone {
				unifiedSignal.Type = UnifiedSignalResistance
			} else {
				unifiedSignal.Type = UnifiedSignalSupport
			}
		case SDSignalZoneBreakout:
			unifiedSignal.Type = UnifiedSignalBreakout
		case SDSignalFreshZone:
			if signal.Zone.Type == SupplyZone {
				unifiedSignal.Type = UnifiedSignalResistance
			} else {
				unifiedSignal.Type = UnifiedSignalSupport
			}
		}

		// 设置优先级
		if signal.Confidence >= 80 {
			unifiedSignal.Priority = PriorityHigh
		} else if signal.Confidence >= 60 {
			unifiedSignal.Priority = PriorityMedium
		} else {
			unifiedSignal.Priority = PriorityLow
		}

		signals = append(signals, unifiedSignal)
	}

	return signals
}

// collectFVGSignals 收集FVG信号
func (ca *ComprehensiveAnalyzer) collectFVGSignals(fvgData *FVGData, currentPrice float64) []*UnifiedSignal {
	var signals []*UnifiedSignal

	if fvgData == nil {
		return signals
	}

	// 生成FVG信号
	fvgSignals := ca.fvgAnalyzer.GenerateSignals(fvgData, currentPrice)

	for _, signal := range fvgSignals {
		unifiedSignal := &UnifiedSignal{
			ID:         fmt.Sprintf("fvg_%d", time.Now().UnixNano()),
			Action:     signal.Action,
			Entry:      signal.Entry,
			StopLoss:   signal.StopLoss,
			TakeProfit: signal.TakeProfit,
			RiskReward: signal.RiskReward,
			Confidence: signal.Confidence,
			Strength:   signal.Strength,
			Sources: []SignalSource{
				{
					Source:     "fvg",
					Weight:     ca.config.WeightFVG,
					Confidence: signal.Confidence,
					Details:    signal.Description,
				},
			},
			Description: signal.Description,
			TimeFrame:   "4h",
			Timestamp:   signal.Timestamp,
		}

		// 根据FVG信号类型设置统一信号类型
		switch signal.Type {
		case FVGSignalReaction:
			if signal.FVG.Type == BullishFVG {
				unifiedSignal.Type = UnifiedSignalSupport
			} else {
				unifiedSignal.Type = UnifiedSignalResistance
			}
		case FVGSignalFillEntry:
			unifiedSignal.Type = UnifiedSignalMeanReversion
		case FVGSignalRejection:
			if signal.FVG.Type == BullishFVG {
				unifiedSignal.Type = UnifiedSignalSupport
			} else {
				unifiedSignal.Type = UnifiedSignalResistance
			}
		case FVGSignalBreakthrough:
			unifiedSignal.Type = UnifiedSignalBreakout
		}

		// 设置优先级
		if signal.Confidence >= 80 {
			unifiedSignal.Priority = PriorityHigh
		} else if signal.Confidence >= 60 {
			unifiedSignal.Priority = PriorityMedium
		} else {
			unifiedSignal.Priority = PriorityLow
		}

		signals = append(signals, unifiedSignal)
	}

	return signals
}

// collectFibonacciSignals 收集斐波纳契信号
func (ca *ComprehensiveAnalyzer) collectFibonacciSignals(fibData *FibonacciData, currentPrice float64) []*UnifiedSignal {
	var signals []*UnifiedSignal

	if fibData == nil {
		return signals
	}

	// 生成斐波纳契信号
	fibSignals := ca.fibonacciAnalyzer.GenerateSignals(fibData, []Kline{{Close: currentPrice}})

	for _, signal := range fibSignals {
		// 安全获取第一个止盈目标
		var takeProfit float64
		if len(signal.TakeProfit) > 0 {
			takeProfit = signal.TakeProfit[0]
		}

		unifiedSignal := &UnifiedSignal{
			ID:         fmt.Sprintf("fib_%d", time.Now().UnixNano()),
			Action:     signal.Action,
			Entry:      signal.EntryPrice,
			StopLoss:   signal.StopLoss,
			TakeProfit: takeProfit,
			RiskReward: signal.RiskReward,
			Confidence: signal.Confidence,
			Strength:   signal.Strength,
			Sources: []SignalSource{
				{
					Source:     "fibonacci",
					Weight:     ca.config.WeightFibonacci,
					Confidence: signal.Confidence,
					Details:    signal.Context,
				},
			},
			Description: signal.Context,
			TimeFrame:   "4h",
			Timestamp:   signal.Timestamp,
		}

		// 根据斐波纳契信号类型设置统一信号类型
		switch signal.Type {
		case FibSignalGoldenPocket:
			if signal.Action == ActionBuy {
				unifiedSignal.Type = UnifiedSignalSupport
			} else {
				unifiedSignal.Type = UnifiedSignalResistance
			}
		case FibSignalBounce:
			if signal.Action == ActionBuy {
				unifiedSignal.Type = UnifiedSignalSupport
			} else {
				unifiedSignal.Type = UnifiedSignalResistance
			}
		case FibSignalBreakout:
			unifiedSignal.Type = UnifiedSignalBreakout
		case FibSignalCluster:
			unifiedSignal.Type = UnifiedSignalSupport
		case FibSignalExtension:
			unifiedSignal.Type = UnifiedSignalTrendFollowing
		}

		// 设置优先级 - 黄金口袋信号优先级更高
		if signal.Type == FibSignalGoldenPocket {
			if signal.Confidence >= 70 {
				unifiedSignal.Priority = PriorityHigh
			} else {
				unifiedSignal.Priority = PriorityMedium
			}
		} else {
			if signal.Confidence >= 80 {
				unifiedSignal.Priority = PriorityHigh
			} else if signal.Confidence >= 60 {
				unifiedSignal.Priority = PriorityMedium
			} else {
				unifiedSignal.Priority = PriorityLow
			}
		}

		signals = append(signals, unifiedSignal)
	}

	return signals
}

// fuseSignals 信号融合
func (ca *ComprehensiveAnalyzer) fuseSignals(signals []*UnifiedSignal) []*UnifiedSignal {
	if len(signals) <= 1 {
		return signals
	}

	var fusedSignals []*UnifiedSignal
	processed := make(map[string]bool)

	for i, signal1 := range signals {
		if processed[signal1.ID] {
			continue
		}

		fusedSignal := &UnifiedSignal{
			ID:          signal1.ID,
			Type:        signal1.Type,
			Action:      signal1.Action,
			Entry:       signal1.Entry,
			StopLoss:    signal1.StopLoss,
			TakeProfit:  signal1.TakeProfit,
			RiskReward:  signal1.RiskReward,
			Confidence:  signal1.Confidence,
			Strength:    signal1.Strength,
			Sources:     signal1.Sources,
			Description: signal1.Description,
			TimeFrame:   signal1.TimeFrame,
			Priority:    signal1.Priority,
			Timestamp:   signal1.Timestamp,
		}

		// 查找可以融合的信号
		for j := i + 1; j < len(signals); j++ {
			signal2 := signals[j]
			if processed[signal2.ID] {
				continue
			}

			// 检查是否可以融合（相同动作和相近价格）
			if ca.canFuseSignals(signal1, signal2) {
				// 融合信号
				ca.mergeSignals(fusedSignal, signal2)
				processed[signal2.ID] = true
			}
		}

		// 重新计算融合后的置信度
		ca.recalculateConfidence(fusedSignal)

		fusedSignals = append(fusedSignals, fusedSignal)
		processed[signal1.ID] = true
	}

	return fusedSignals
}

// canFuseSignals 检查两个信号是否可以融合
func (ca *ComprehensiveAnalyzer) canFuseSignals(signal1, signal2 *UnifiedSignal) bool {
	// 相同动作
	if signal1.Action != signal2.Action {
		return false
	}

	// 价格相近（5%范围内）
	priceDiff := abs(signal1.Entry - signal2.Entry) / signal1.Entry
	if priceDiff > 0.05 {
		return false
	}

	// 时间相近（1小时内）
	timeDiff := abs(float64(signal1.Timestamp - signal2.Timestamp))
	if timeDiff > 3600*1000 { // 1小时的毫秒数
		return false
	}

	return true
}

// mergeSignals ��并两个信号
func (ca *ComprehensiveAnalyzer) mergeSignals(target *UnifiedSignal, source *UnifiedSignal) {
	// 合并信号源
	target.Sources = append(target.Sources, source.Sources...)

	// 加权平均入场价
	totalWeight := 0.0
	weightedEntry := 0.0

	for _, src := range target.Sources {
		totalWeight += src.Weight
		if src.Source == "dow_theory" {
			weightedEntry += target.Entry * src.Weight
		} else if src.Source == "vpvr" {
			weightedEntry += source.Entry * src.Weight
		} else if src.Source == "supply_demand" {
			weightedEntry += source.Entry * src.Weight
		} else if src.Source == "fvg" {
			weightedEntry += source.Entry * src.Weight
		} else if src.Source == "fibonacci" {
			weightedEntry += source.Entry * src.Weight
		}
	}

	if totalWeight > 0 {
		target.Entry = weightedEntry / totalWeight
	}

	// 取较好的止损和止盈
	if source.RiskReward > target.RiskReward {
		target.StopLoss = source.StopLoss
		target.TakeProfit = source.TakeProfit
		target.RiskReward = source.RiskReward
	}

	// 合并描述
	target.Description = fmt.Sprintf("%s; %s", target.Description, source.Description)

	// 取较高强度
	if source.Strength > target.Strength {
		target.Strength = source.Strength
	}
}

// recalculateConfidence 重新计算置信度
func (ca *ComprehensiveAnalyzer) recalculateConfidence(signal *UnifiedSignal) {
	if len(signal.Sources) == 0 {
		return
	}

	// 加权平均置信度
	totalWeight := 0.0
	weightedConfidence := 0.0

	for _, source := range signal.Sources {
		totalWeight += source.Weight
		weightedConfidence += source.Confidence * source.Weight
	}

	if totalWeight > 0 {
		signal.Confidence = weightedConfidence / totalWeight
	}

	// 多源确认加成
	if len(signal.Sources) > 1 {
		confirmationBonus := float64(len(signal.Sources)-1) * 5 // 每个额外源+5%置信度
		signal.Confidence = min(signal.Confidence+confirmationBonus, 100)
	}

	// 重新评估优先级
	if signal.Confidence >= 80 {
		signal.Priority = PriorityHigh
	} else if signal.Confidence >= 60 {
		signal.Priority = PriorityMedium
	} else {
		signal.Priority = PriorityLow
	}
}

// UpdateConfig 更新配置
func (ca *ComprehensiveAnalyzer) UpdateConfig(config *ComprehensiveConfig) {
	ca.config = config
}

// GetConfig 获取当前配置
func (ca *ComprehensiveAnalyzer) GetConfig() *ComprehensiveConfig {
	return ca.config
}

// analyzeMarketStructure 分析市场结构
func (ca *ComprehensiveAnalyzer) analyzeMarketStructure(result *ComprehensiveResult) *MarketStructure {
	structure := &MarketStructure{
		SupportLevels:    make([]float64, 0),
		ResistanceLevels: make([]float64, 0),
		KeyLevels:        make([]KeyLevel, 0),
	}

	// 从道氏理论获取趋势信息
	if result.DowTheory != nil && result.DowTheory.TrendStrength != nil {
		structure.TrendDirection = result.DowTheory.TrendStrength.Direction
		structure.TrendStrength = result.DowTheory.TrendStrength.Overall
		structure.Volatility = 100 - result.DowTheory.TrendStrength.Consistency
	}

	// 从供需区获取支撑阻力
	if result.SupplyDemand != nil {
		for _, zone := range result.SupplyDemand.ActiveZones {
			if zone.Type == SupplyZone {
				structure.ResistanceLevels = append(structure.ResistanceLevels, zone.CenterPrice)
				structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
					Price:       zone.CenterPrice,
					Type:        LevelResistance,
					Strength:    zone.Strength,
					Source:      "supply_zone",
					Description: fmt.Sprintf("供给区 %.2f-%.2f", zone.LowerBound, zone.UpperBound),
				})
			} else {
				structure.SupportLevels = append(structure.SupportLevels, zone.CenterPrice)
				structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
					Price:       zone.CenterPrice,
					Type:        LevelSupport,
					Strength:    zone.Strength,
					Source:      "demand_zone",
					Description: fmt.Sprintf("需求区 %.2f-%.2f", zone.LowerBound, zone.UpperBound),
				})
			}
		}
	}

	// 从VPVR获取关键价位
	if result.VolumeProfile != nil {
		if result.VolumeProfile.POC != nil {
			structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
				Price:       result.VolumeProfile.POC.Price,
				Type:        LevelPOC,
				Strength:    result.VolumeProfile.POC.VolumePercent,
				Source:      "vpvr_poc",
				Description: fmt.Sprintf("POC (%.1f%%成交量)", result.VolumeProfile.POC.VolumePercent),
			})
		}

		structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
			Price:       result.VolumeProfile.VAH,
			Type:        LevelVAH,
			Strength:    70,
			Source:      "vpvr_vah",
			Description: "价值区上沿",
		})

		structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
			Price:       result.VolumeProfile.VAL,
			Type:        LevelVAL,
			Strength:    70,
			Source:      "vpvr_val",
			Description: "价值区下沿",
		})

		// VPVR概况
		structure.VolumeProfile = &VPSummary{
			POC: result.VolumeProfile.POC.Price,
			VAH: result.VolumeProfile.VAH,
			VAL: result.VolumeProfile.VAL,
			Concentration: result.VolumeProfile.ValueArea.Concentration,
		}

		// 确定当前价格在价值区的位置
		if result.CurrentPrice > result.VolumeProfile.VAH {
			structure.VolumeProfile.CurrentPosition = "价值区上方"
		} else if result.CurrentPrice < result.VolumeProfile.VAL {
			structure.VolumeProfile.CurrentPosition = "价值区下方"
		} else {
			structure.VolumeProfile.CurrentPosition = "价值区内"
		}
	}

	// 从FVG获取关键价位
	if result.FairValueGaps != nil {
		for _, fvg := range result.FairValueGaps.ActiveFVGs {
			if fvg.Type == BullishFVG {
				structure.SupportLevels = append(structure.SupportLevels, fvg.CenterPrice)
				structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
					Price:       fvg.CenterPrice,
					Type:        LevelSupport,
					Strength:    fvg.Strength,
					Source:      "bullish_fvg",
					Description: fmt.Sprintf("看涨FVG %.2f-%.2f (强度: %.1f)", fvg.LowerBound, fvg.UpperBound, fvg.Strength),
				})
			} else {
				structure.ResistanceLevels = append(structure.ResistanceLevels, fvg.CenterPrice)
				structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
					Price:       fvg.CenterPrice,
					Type:        LevelResistance,
					Strength:    fvg.Strength,
					Source:      "bearish_fvg",
					Description: fmt.Sprintf("看跌FVG %.2f-%.2f (强度: %.1f)", fvg.LowerBound, fvg.UpperBound, fvg.Strength),
				})
			}
		}
	}

	// 从斐波纳契获取关键价位
	if result.Fibonacci != nil {
		// 添加黄金口袋级别
		if result.Fibonacci.GoldenPocket != nil && result.Fibonacci.GoldenPocket.IsActive {
			goldenPocket := result.Fibonacci.GoldenPocket
			
			// 黄金口袋作为重要的支撑/阻力位
			if goldenPocket.TrendContext == TrendUpward {
				structure.SupportLevels = append(structure.SupportLevels, goldenPocket.CenterPrice)
				structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
					Price:       goldenPocket.CenterPrice,
					Type:        LevelSupport,
					Strength:    goldenPocket.Strength,
					Source:      "fibonacci_golden_pocket",
					Description: fmt.Sprintf("斐波黄金口袋0.618 (强度: %.1f)", goldenPocket.Strength),
				})
			} else {
				structure.ResistanceLevels = append(structure.ResistanceLevels, goldenPocket.CenterPrice)
				structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
					Price:       goldenPocket.CenterPrice,
					Type:        LevelResistance,
					Strength:    goldenPocket.Strength,
					Source:      "fibonacci_golden_pocket",
					Description: fmt.Sprintf("斐波黄金口袋0.618 (强度: %.1f)", goldenPocket.Strength),
				})
			}
		}

		// 添加重要的斐波回调级别
		for _, retracement := range result.Fibonacci.Retracements {
			if !retracement.IsActive || retracement.Quality != FibQualityHigh {
				continue
			}

			for _, level := range retracement.Levels {
				if level.Importance < 0.8 { // 只添加重要性高的级别
					continue
				}

				var levelType LevelType
				if retracement.TrendType == TrendUpward {
					levelType = LevelSupport
					structure.SupportLevels = append(structure.SupportLevels, level.Price)
				} else {
					levelType = LevelResistance
					structure.ResistanceLevels = append(structure.ResistanceLevels, level.Price)
				}

				description := fmt.Sprintf("斐波%.1f%%回调 (重要性: %.1f)", level.Ratio*100, level.Importance)
				if level.IsGoldenRatio {
					description += " ★黄金比率"
				}

				structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
					Price:       level.Price,
					Type:        levelType,
					Strength:    level.Importance * 100,
					Source:      "fibonacci_retracement",
					Description: description,
				})
			}
		}

		// 添加斐波聚集区
		for _, cluster := range result.Fibonacci.Clusters {
			if cluster.Importance < 70 { // 只添加重要的聚集区
				continue
			}

			structure.KeyLevels = append(structure.KeyLevels, KeyLevel{
				Price:       cluster.CenterPrice,
				Type:        LevelSupport, // 聚集区默认作为支撑位
				Strength:    cluster.Importance,
				Source:      "fibonacci_cluster",
				Description: fmt.Sprintf("斐波聚集区(%d级别, 密度: %.2f)", cluster.LevelCount, cluster.Density),
			})
		}
	}

	// 确定市场阶段
	structure.MarketPhase = ca.determineMarketPhase(result)

	return structure
}

// determineMarketPhase 确定市场阶段
func (ca *ComprehensiveAnalyzer) determineMarketPhase(result *ComprehensiveResult) MarketPhase {
	// 基于道氏理论和成交量分布判断市场阶段
	if result.DowTheory != nil && result.DowTheory.TrendStrength != nil {
		trendStrength := result.DowTheory.TrendStrength.Overall
		direction := result.DowTheory.TrendStrength.Direction

		if trendStrength > 70 {
			if direction == TrendUp {
				return PhaseMarkup
			} else if direction == TrendDown {
				return PhaseMarkdown
			}
		} else if trendStrength < 30 {
			// 低趋势强度，可能是积累或分发阶段
			if result.VolumeProfile != nil && result.VolumeProfile.ValueArea != nil {
				concentration := result.VolumeProfile.ValueArea.Concentration
				if concentration > 2.0 {
					// 高集中度表示积累
					return PhaseAccumulation
				} else {
					// 低集中度表示分发
					return PhaseDistribution
				}
			}
			return PhaseSideways
		}
	}

	return PhaseSideways
}

// assessRisk 评估风险
func (ca *ComprehensiveAnalyzer) assessRisk(result *ComprehensiveResult) *RiskAssessment {
	assessment := &RiskAssessment{
		RiskFactors: make([]string, 0),
	}

	var riskScore float64 = 0

	// 趋势风险评估
	if result.DowTheory != nil && result.DowTheory.TrendStrength != nil {
		trendStrength := result.DowTheory.TrendStrength.Overall
		consistency := result.DowTheory.TrendStrength.Consistency

		if trendStrength > 70 && consistency > 70 {
			assessment.TrendRisk = RiskLow
			riskScore += 1
		} else if trendStrength > 50 {
			assessment.TrendRisk = RiskMedium
			riskScore += 2
		} else {
			assessment.TrendRisk = RiskHigh
			riskScore += 3
			assessment.RiskFactors = append(assessment.RiskFactors, "趋势不明确")
		}
	}

	// 波动性风险评估
	if result.MarketStructure != nil {
		volatility := result.MarketStructure.Volatility
		if volatility < 20 {
			assessment.VolatilityRisk = RiskLow
			riskScore += 1
		} else if volatility < 50 {
			assessment.VolatilityRisk = RiskMedium
			riskScore += 2
		} else {
			assessment.VolatilityRisk = RiskHigh
			riskScore += 3
			assessment.RiskFactors = append(assessment.RiskFactors, "高波动性")
		}
	}

	// 流动性风险评估（基于成交量分布）
	if result.VolumeProfile != nil && result.VolumeProfile.ValueArea != nil {
		concentration := result.VolumeProfile.ValueArea.Concentration
		if concentration > 2.0 {
			assessment.LiquidityRisk = RiskLow
			riskScore += 1
		} else if concentration > 1.2 {
			assessment.LiquidityRisk = RiskMedium
			riskScore += 2
		} else {
			assessment.LiquidityRisk = RiskHigh
			riskScore += 3
			assessment.RiskFactors = append(assessment.RiskFactors, "成交量分散")
		}
	}

	// 供需区风险
	if result.SupplyDemand != nil && result.SupplyDemand.Statistics != nil {
		successRate := result.SupplyDemand.Statistics.SuccessRate
		if successRate < 50 {
			riskScore += 1
			assessment.RiskFactors = append(assessment.RiskFactors, "供需区成功率低")
		}
	}

	// 计算整体风险
	avgRisk := riskScore / 3
	if avgRisk <= 1.5 {
		assessment.OverallRisk = RiskLow
		assessment.RecommendedRisk = 0.02 // 2%
		assessment.MaxPositionSize = 0.1  // 10%
		assessment.SuggestedTimeFrame = "中长期"
	} else if avgRisk <= 2.5 {
		assessment.OverallRisk = RiskMedium
		assessment.RecommendedRisk = 0.015 // 1.5%
		assessment.MaxPositionSize = 0.05  // 5%
		assessment.SuggestedTimeFrame = "中期"
	} else {
		assessment.OverallRisk = RiskHigh
		assessment.RecommendedRisk = 0.01 // 1%
		assessment.MaxPositionSize = 0.02 // 2%
		assessment.SuggestedTimeFrame = "短期"
		if len(assessment.RiskFactors) == 0 {
			assessment.RiskFactors = append(assessment.RiskFactors, "整体风险较高")
		}
	}

	return assessment
}

// generateTradingAdvice 生成交易建议
func (ca *ComprehensiveAnalyzer) generateTradingAdvice(result *ComprehensiveResult) *TradingAdvice {
	advice := &TradingAdvice{
		ReasoningPoints:      make([]string, 0),
		AlternativeScenarios: make([]string, 0),
	}

	// 基于统一信号生成建议
	if len(result.UnifiedSignals) == 0 {
		advice.OverallAction = ActionHold
		advice.Confidence = 30
		advice.ReasoningPoints = append(advice.ReasoningPoints, "无明确交易信号")
		advice.EntryStrategy = "等待明确信号"
		advice.ExitStrategy = "保持观望"
		advice.RiskManagement = "避免交易"
		advice.TimeHorizon = "等待"
		return advice
	}

	// 取置信度最高的信号作为主要建议
	primarySignal := result.UnifiedSignals[0]
	advice.OverallAction = primarySignal.Action
	advice.Confidence = primarySignal.Confidence

	// 生成推理要点
	advice.ReasoningPoints = append(advice.ReasoningPoints, 
		fmt.Sprintf("主要信号类型: %s (置信度: %.1f%%)", primarySignal.Type, primarySignal.Confidence))

	// 统计各源的支持情况
	sourceSupport := make(map[string]int)
	for _, signal := range result.UnifiedSignals {
		if signal.Action == primarySignal.Action {
			for _, source := range signal.Sources {
				sourceSupport[source.Source]++
			}
		}
	}

	if len(sourceSupport) > 1 {
		advice.ReasoningPoints = append(advice.ReasoningPoints, "多重分析确认")
	}

	// 基于市场结构增加推理
	if result.MarketStructure != nil {
		if result.MarketStructure.TrendDirection == TrendUp && primarySignal.Action == ActionBuy {
			advice.ReasoningPoints = append(advice.ReasoningPoints, "顺势而为，符合上升趋势")
		} else if result.MarketStructure.TrendDirection == TrendDown && primarySignal.Action == ActionSell {
			advice.ReasoningPoints = append(advice.ReasoningPoints, "顺势而为，符合下降趋势")
		} else if primarySignal.Action != ActionHold {
			advice.ReasoningPoints = append(advice.ReasoningPoints, "逆势交易，风险较高")
			advice.Confidence *= 0.8 // 降低置信度
		}
	}

	// 生成入场策略
	if primarySignal.Action == ActionBuy {
		advice.EntryStrategy = fmt.Sprintf("在%.2f附近分批买入，突破%.2f后加仓", 
			primarySignal.Entry, primarySignal.Entry*1.01)
	} else if primarySignal.Action == ActionSell {
		advice.EntryStrategy = fmt.Sprintf("在%.2f附近分批卖出，跌破%.2f后加仓", 
			primarySignal.Entry, primarySignal.Entry*0.99)
	} else {
		advice.EntryStrategy = "保持观望，等待更明确的信号"
	}

	// 生成出场策略
	if primarySignal.StopLoss > 0 && primarySignal.TakeProfit > 0 {
		advice.ExitStrategy = fmt.Sprintf("止损%.2f, 止盈%.2f (风险收益比1:%.1f)", 
			primarySignal.StopLoss, primarySignal.TakeProfit, primarySignal.RiskReward)
	} else {
		advice.ExitStrategy = "根据技术位和资金管理设置止损止盈"
	}

	// 风险管理建议
	if result.RiskAssessment != nil {
		advice.RiskManagement = fmt.Sprintf("建议风险敞口不超过%.1f%%, 最大仓位不超过%.1f%%", 
			result.RiskAssessment.RecommendedRisk*100, 
			result.RiskAssessment.MaxPositionSize*100)
		advice.TimeHorizon = result.RiskAssessment.SuggestedTimeFrame
	}

	// 生成替代方案
	if len(result.UnifiedSignals) > 1 {
		secondarySignal := result.UnifiedSignals[1]
		if secondarySignal.Action != primarySignal.Action {
			advice.AlternativeScenarios = append(advice.AlternativeScenarios, 
				fmt.Sprintf("备选方案: %s (置信度: %.1f%%)", 
					secondarySignal.Action, secondarySignal.Confidence))
		}
	}

	// 基于风险评估添加替代方案
	if result.RiskAssessment != nil && result.RiskAssessment.OverallRisk == RiskHigh {
		advice.AlternativeScenarios = append(advice.AlternativeScenarios, "高风险环境下考虑降低仓位或暂停交易")
	}

	return advice
}

// AnalyzeAllTimeframes 分析所有时间框架
func (ca *ComprehensiveAnalyzer) AnalyzeAllTimeframes(symbol string, currentPrice float64, klinesMap map[string][]Kline) *MultiTimeframeAnalysis {
	timeframes := []string{"5m", "15m", "30m", "1h", "4h"}
	weights := map[string]float64{
		"5m":  0.15, // 短期趨势，适中提高权重
		"15m": 0.15, // 短期趋势
		"30m": 0.2,  // 中短期趋势
		"1h":  0.25, // 中期趋势
		"4h":  0.3,  // 长期趋势，权重最高
	}

	analysis := &MultiTimeframeAnalysis{
		Timeframes: make(map[string]*TimeframeAnalysis),
	}

	// 分析每个时间框架
	for _, tf := range timeframes {
		klines, exists := klinesMap[tf]
		if !exists || len(klines) < 10 {
			continue
		}

		tfAnalysis := ca.analyzeSingleTimeframe(tf, symbol, klines, currentPrice, weights[tf])
		analysis.Timeframes[tf] = tfAnalysis
	}

	// 生成综合总结
	analysis.Summary = ca.generateAnalysisSummary(analysis.Timeframes, currentPrice)

	// 应用多时间框架精度格式化
	ca.ApplyMultiTimeframePrecisionFormatting(analysis, symbol)

	return analysis
}

// analyzeSingleTimeframe 分析单一时间框架
func (ca *ComprehensiveAnalyzer) analyzeSingleTimeframe(timeframe, symbol string, klines []Kline, currentPrice, weight float64) *TimeframeAnalysis {
	tfAnalysis := &TimeframeAnalysis{
		Timeframe: timeframe,
		Weight:    weight,
	}

	// 根据时间框架确定最小数据要求与升级后的校验
	minDataPoints := ca.getMinDataPoints(timeframe)
	recommendedDataPoints := minDataPoints * 2 // 建议数据量为最小的2倍
	
	if len(klines) < minDataPoints {
		log.Printf("🚨🔴 [综合分析] ❌ %s时间框架数据不足: 需要%d根，实际%d根 ❌", timeframe, minDataPoints, len(klines))
		tfAnalysis.Reliability = 0.0
		return tfAnalysis
	}
	if len(klines) < recommendedDataPoints {
		log.Printf("🟡⚠️ [综合分析] %s时间框架数据警告: 建议%d根，实际%d根 (可能影响精度) ⚠️🟡", timeframe, recommendedDataPoints, len(klines))
	}

	// 执行各种分析
	if ca.config.EnableDowTheory {
		// 道氏理论需要3m和当前时间框架的数据
		if timeframe == "5m" {
			tfAnalysis.DowTheory = ca.dowAnalyzer.Analyze(klines, klines, currentPrice)
		} else {
			// 对于其他时间框架，使用当前数据
			tfAnalysis.DowTheory = ca.dowAnalyzer.Analyze(klines, klines, currentPrice)
		}
	}

	// 通道分析
	tfAnalysis.ChannelAnalysis = ca.channelAnalyzer.Analyze(klines, currentPrice)

	// VPVR分析
	// 🔥 P0-04修复：为每个时间框架使用正确的动态VPVR配置
	if ca.config.EnableVPVR {
		// 🔥 P0-04核心修复：为当前时间框架动态创建VPVR分析器
		// 这样确保每个时间框架都使用正确的tick_size和timeframe配置
		var currentVPVRAnalyzer *VPVRAnalyzer
		
		// 尝试从现有VPVR分析器获取配置中的ExchangeMeta
		existingConfig := ca.vpvrAnalyzer.GetConfig()
		
		// 创建当前时间框架的动态配置
		if existingConfig.TickSize > 0 && existingConfig.TickSize != fallbackVPVRConfig.TickSize {
			// 如果现有分析器使用了动态配置（不是fallback默认值），重建ExchangeMeta
			exchangeMeta := &ExchangeMeta{
				TickSize: existingConfig.TickSize,
				Symbol:   symbol,
			}
			currentVPVRAnalyzer = NewVPVRAnalyzerWithDynamicConfig(exchangeMeta, timeframe)
		} else {
			// 使用现有分析器（可能是fallback配置）
			currentVPVRAnalyzer = ca.vpvrAnalyzer
		}
		
		tfAnalysis.VolumeProfile = currentVPVRAnalyzer.Analyze(klines)
		
		// 🔥 P0-04修复：在VolumeProfile输出中明确标注timeframe与tick_size
		if tfAnalysis.VolumeProfile != nil {
			// 确保UsedTimeFrame被正确设置
			if tfAnalysis.VolumeProfile.UsedTimeFrame == "" {
				tfAnalysis.VolumeProfile.UsedTimeFrame = timeframe
			}
			// UsedTickSize已在VPVR分析器中设置
		}
	}

	// 供需区分析
	if ca.config.EnableSupplyDemand {
		tfAnalysis.SupplyDemand = ca.sdAnalyzer.AnalyzeWithSymbol(klines, symbol, timeframe)
	}

	// FVG分析
	if ca.config.EnableFVG {
		tfAnalysis.FairValueGaps = ca.fvgAnalyzer.Analyze(klines)
	}

	// 斐波纳契分析
	if ca.config.EnableFibonacci {
		tfAnalysis.Fibonacci = ca.fibonacciAnalyzer.Analyze(klines)
	}

	// 支撑阻力转换线分析
	if ca.config.EnableSupportResistance {
		tfAnalysis.SupportResistance = ca.srAnalyzer.Analyze(klines)
	}

	// 计算可靠性评分
	tfAnalysis.Reliability = ca.calculateTimeframeReliability(tfAnalysis, timeframe, len(klines))

	return tfAnalysis
}

// getMinDataPoints 获取时间框架的最小数据要求
func (ca *ComprehensiveAnalyzer) getMinDataPoints(timeframe string) int {
	switch timeframe {
	case "5m":
		return 100 // 5小时数据
	case "15m":
		return 80  // 20小时数据
	case "30m":
		return 60  // 30小时数据
	case "1h":
		return 48  // 48小时数据
	case "4h":
		return 20  // 80小时数据
	default:
		return 50
	}
}

// calculateTimeframeReliability 计算时间框架可靠性
func (ca *ComprehensiveAnalyzer) calculateTimeframeReliability(tfAnalysis *TimeframeAnalysis, timeframe string, dataPoints int) float64 {
	reliability := 0.0

	// 基础数据充足性评分
	minPoints := ca.getMinDataPoints(timeframe)
	dataScore := min(float64(dataPoints)/float64(minPoints), 1.0)
	reliability += dataScore * 0.3

	// 分析质量评分
	qualityScore := 0.0
	analysisCount := 0

	if tfAnalysis.DowTheory != nil && tfAnalysis.DowTheory.TrendStrength != nil {
		qualityScore += tfAnalysis.DowTheory.TrendStrength.Overall / 100
		analysisCount++
	}

	if tfAnalysis.ChannelAnalysis != nil {
		qualityScore += tfAnalysis.ChannelAnalysis.Quality
		analysisCount++
	}

	if tfAnalysis.VolumeProfile != nil && tfAnalysis.VolumeProfile.ValueArea != nil {
		qualityScore += tfAnalysis.VolumeProfile.ValueArea.Concentration / 3.0 // 标准化到0-1
		analysisCount++
	}

	if analysisCount > 0 {
		reliability += (qualityScore / float64(analysisCount)) * 0.5
	}

	// 时间框架权重调整
	reliability += tfAnalysis.Weight * 0.2

	return min(reliability, 1.0)
}

// generateAnalysisSummary 生成多时间框架综合总结
func (ca *ComprehensiveAnalyzer) generateAnalysisSummary(timeframes map[string]*TimeframeAnalysis, currentPrice float64) *AnalysisSummary {
	summary := &AnalysisSummary{
		TimeframeAlignment: make(map[string]bool),
		KeyLevels: &MultiTimeframeLevels{
			SupportLevels:    []LevelInfo{},
			ResistanceLevels: []LevelInfo{},
			PivotLevels:      []LevelInfo{},
		},
		TradingSignals: []*MultiTimeframeSignal{},
		RiskAssessment: &MultiTimeframeRisk{
			TimeframeRisks: make(map[string]string),
		},
	}

	// 分析趋势一致性
	trendVotes := make(map[string]int) // up, down, flat
	totalWeight := 0.0

	for tf, analysis := range timeframes {
		if analysis.DowTheory != nil && analysis.DowTheory.TrendStrength != nil {
			direction := string(analysis.DowTheory.TrendStrength.Direction)
			trendVotes[direction]++
			totalWeight += analysis.Weight

			// 记录趋势一致性
			summary.TimeframeAlignment[tf] = true // 简化判断，后续可完善
		}
	}

	// 确定总体趋势
	maxVotes := 0
	for trend, votes := range trendVotes {
		if votes > maxVotes {
			maxVotes = votes
			summary.OverallTrend = trend
		}
	}

	// 计算趋势一致性评分
	if len(timeframes) > 0 {
		summary.TrendConsistency = float64(maxVotes) / float64(len(timeframes))
	}

	// 收集关键价位
	ca.collectKeyLevels(timeframes, summary.KeyLevels, currentPrice)

	// 生成多时间框架交易信号
	summary.TradingSignals = ca.generateMultiTimeframeSignals(timeframes, currentPrice)

	// 评估风险
	summary.RiskAssessment = ca.assessMultiTimeframeRisk(timeframes)

	// 计算信号置信度
	summary.SignalConfidence = ca.calculateSignalConfidence(timeframes, summary.TrendConsistency)

	return summary
}

// collectKeyLevels 收集多时间框架关键价位
func (ca *ComprehensiveAnalyzer) collectKeyLevels(timeframes map[string]*TimeframeAnalysis, levels *MultiTimeframeLevels, currentPrice float64) {
	levelMap := make(map[float64]*LevelInfo) // 价格 -> 级别信息

	for tf, analysis := range timeframes {
		// 从通道分析收集支撑阻力位
		if analysis.ChannelAnalysis != nil && analysis.ChannelAnalysis.ActiveChannel != nil {
			channel := analysis.ChannelAnalysis.ActiveChannel
			
			// 上轨作为阻力位
			if channel.UpperLine != nil {
				currentTime := float64(time.Now().UnixMilli())
				upperPrice := channel.UpperLine.Slope*currentTime + channel.UpperLine.Intercept
				ca.addOrUpdateLevel(levelMap, upperPrice, "resistance", "channel_"+tf, tf, channel.Quality)
			}
			
			// 下轨作为支撑位
			if channel.LowerLine != nil {
				currentTime := float64(time.Now().UnixMilli())
				lowerPrice := channel.LowerLine.Slope*currentTime + channel.LowerLine.Intercept
				ca.addOrUpdateLevel(levelMap, lowerPrice, "support", "channel_"+tf, tf, channel.Quality)
			}
		}

		// 从VPVR分析收集关键价位
		if analysis.VolumeProfile != nil {
			if analysis.VolumeProfile.POC != nil {
				ca.addOrUpdateLevel(levelMap, analysis.VolumeProfile.POC.Price, "pivot", "vpvr_poc_"+tf, tf, analysis.VolumeProfile.POC.VolumePercent/100)
			}
			ca.addOrUpdateLevel(levelMap, analysis.VolumeProfile.VAH, "resistance", "vpvr_vah_"+tf, tf, 0.8)
			ca.addOrUpdateLevel(levelMap, analysis.VolumeProfile.VAL, "support", "vpvr_val_"+tf, tf, 0.8)
		}

		// 从斐波纳契分析收集关键价位
		if analysis.Fibonacci != nil && analysis.Fibonacci.GoldenPocket != nil && analysis.Fibonacci.GoldenPocket.IsActive {
			pocket := analysis.Fibonacci.GoldenPocket
			levelType := "support"
			if pocket.TrendContext == TrendDownward {
				levelType = "resistance"
			}
			ca.addOrUpdateLevel(levelMap, pocket.CenterPrice, levelType, "fib_golden_"+tf, tf, pocket.Strength/100)
		}
	}

	// 将收集的价位分类到支撑、阻力、枢轴点
	for _, level := range levelMap {
		switch level.Type {
		case "support":
			levels.SupportLevels = append(levels.SupportLevels, *level)
		case "resistance":
			levels.ResistanceLevels = append(levels.ResistanceLevels, *level)
		case "pivot":
			levels.PivotLevels = append(levels.PivotLevels, *level)
		}
	}
}

// addOrUpdateLevel 添加或更新价位信息
func (ca *ComprehensiveAnalyzer) addOrUpdateLevel(levelMap map[float64]*LevelInfo, price float64, levelType, source, timeframe string, strength float64) {
	// 价格容差（0.1%）
	tolerance := price * 0.001
	
	// 查找是否有接近的价位
	var existingPrice float64 = -1
	for existingP := range levelMap {
		if abs(existingP-price) <= tolerance {
			existingPrice = existingP
			break
		}
	}

	if existingPrice > 0 {
		// 更新现有价位
		level := levelMap[existingPrice]
		level.Sources = append(level.Sources, source)
		level.Timeframes = append(level.Timeframes, timeframe)
		level.Strength += strength
		level.Confidence = min(level.Confidence+0.2, 1.0) // 多时间框架确认增加置信度
	} else {
		// 添加新价位
		levelMap[price] = &LevelInfo{
			Price:      price,
			Strength:   strength,
			Sources:    []string{source},
			Timeframes: []string{timeframe},
			Type:       levelType,
			Confidence: 0.5, // 初始置信度
		}
	}
}

// generateMultiTimeframeSignals 生成多时间框架交易信号
func (ca *ComprehensiveAnalyzer) generateMultiTimeframeSignals(timeframes map[string]*TimeframeAnalysis, currentPrice float64) []*MultiTimeframeSignal {
	var signals []*MultiTimeframeSignal

	// 收集各时间框架的信号投票
	actionVotes := make(map[SignalAction]map[string]float64) // action -> timeframe -> weight
	actionVotes[ActionBuy] = make(map[string]float64)
	actionVotes[ActionSell] = make(map[string]float64)
	actionVotes[ActionHold] = make(map[string]float64)

	for tf, analysis := range timeframes {
		// 从道氏理论获取信号
		if analysis.DowTheory != nil && analysis.DowTheory.TradingSignal != nil {
			signal := analysis.DowTheory.TradingSignal
			actionVotes[signal.Action][tf] = analysis.Weight * (signal.Confidence / 100)
		}

		// 从通道分析获取信号
		if analysis.ChannelAnalysis != nil {
			action := ca.getChannelSignal(analysis.ChannelAnalysis, currentPrice)
			if action != ActionHold {
				actionVotes[action][tf] += analysis.Weight * analysis.ChannelAnalysis.Quality
			}
		}
	}

	// 计算综合信号
	for action, votes := range actionVotes {
		if len(votes) == 0 {
			continue
		}

		totalWeight := 0.0
		for _, weight := range votes {
			totalWeight += weight
		}

		if totalWeight > 0.3 { // 最低阈值
			signal := &MultiTimeframeSignal{
				ID:            fmt.Sprintf("mtf_%s_%d", action, time.Now().Unix()),
				PrimaryAction: action,
				Confidence:    min(totalWeight, 1.0) * 100,
				TimeframeVotes: make(map[string]SignalAction),
				SignalSources: []string{"multi_timeframe_consensus"},
				EntryPrice:    currentPrice,
				Timeframe:     ca.getDominantTimeframe(votes, timeframes),
				Timestamp:     time.Now().Unix(),
			}

			// 设置投票详情
			for tf := range votes {
				signal.TimeframeVotes[tf] = action
			}

			signals = append(signals, signal)
		}
	}

	return signals
}

// getChannelSignal 从通道分析获取交易信号
func (ca *ComprehensiveAnalyzer) getChannelSignal(channelAnalysis *ChannelData, currentPrice float64) SignalAction {
	if channelAnalysis == nil || channelAnalysis.ActiveChannel == nil {
		return ActionHold
	}

	switch channelAnalysis.CurrentPosition {
	case "lower":
		if channelAnalysis.Direction == "up" {
			return ActionBuy // 上升通道下轨买入
		}
	case "upper":
		if channelAnalysis.Direction == "down" {
			return ActionSell // 下降通道上轨卖出
		}
	case "break_up":
		return ActionBuy // 向上突破
	case "break_down":
		return ActionSell // 向下突破
	}

	return ActionHold
}

// getDominantTimeframe 获取主导时间框架
func (ca *ComprehensiveAnalyzer) getDominantTimeframe(votes map[string]float64, timeframes map[string]*TimeframeAnalysis) string {
	maxWeight := 0.0
	dominantTf := "4h" // 默认

	for tf, weight := range votes {
		if analysis, exists := timeframes[tf]; exists {
			adjustedWeight := weight * analysis.Reliability
			if adjustedWeight > maxWeight {
				maxWeight = adjustedWeight
				dominantTf = tf
			}
		}
	}

	return dominantTf
}

// assessMultiTimeframeRisk 评估多时间框架风险
func (ca *ComprehensiveAnalyzer) assessMultiTimeframeRisk(timeframes map[string]*TimeframeAnalysis) *MultiTimeframeRisk {
	risk := &MultiTimeframeRisk{
		TimeframeRisks: make(map[string]string),
	}

	riskScores := []float64{}
	conflictCount := 0

	for tf, analysis := range timeframes {
		// 评估单个时间框架风险
		tfRisk := "medium"
		riskScore := 0.5

		if analysis.DowTheory != nil && analysis.DowTheory.TrendStrength != nil {
			consistency := analysis.DowTheory.TrendStrength.Consistency
			if consistency > 70 {
				tfRisk = "low"
				riskScore = 0.3
			} else if consistency < 40 {
				tfRisk = "high"
				riskScore = 0.8
			}
		}

		risk.TimeframeRisks[tf] = tfRisk
		riskScores = append(riskScores, riskScore)
	}

	// 计算总体风险
	avgRisk := 0.0
	for _, score := range riskScores {
		avgRisk += score
	}
	if len(riskScores) > 0 {
		avgRisk /= float64(len(riskScores))
	}

	if avgRisk < 0.4 {
		risk.OverallRisk = "low"
		risk.RecommendedExposure = 0.03 // 3%
		risk.MaxPositionSize = 0.15     // 15%
	} else if avgRisk < 0.7 {
		risk.OverallRisk = "medium"
		risk.RecommendedExposure = 0.02 // 2%
		risk.MaxPositionSize = 0.10     // 10%
	} else {
		risk.OverallRisk = "high"
		risk.RecommendedExposure = 0.01 // 1%
		risk.MaxPositionSize = 0.05     // 5%
	}

	risk.ConflictingSignals = conflictCount
	return risk
}

// calculateSignalConfidence 计算信号置信度
func (ca *ComprehensiveAnalyzer) calculateSignalConfidence(timeframes map[string]*TimeframeAnalysis, trendConsistency float64) float64 {
	confidence := trendConsistency * 0.4 // 趋势一致性占40%

	// 分析质量占30%
	qualitySum := 0.0
	qualityCount := 0
	for _, analysis := range timeframes {
		if analysis.Reliability > 0 {
			qualitySum += analysis.Reliability
			qualityCount++
		}
	}
	if qualityCount > 0 {
		confidence += (qualitySum / float64(qualityCount)) * 0.3
	}

	// 数据充足性占30%
	dataScore := min(float64(len(timeframes))/5.0, 1.0) // 最多5个时间框架
	confidence += dataScore * 0.3

	return min(confidence, 1.0)
}

// 🔥 Gate2 结构聚合核心实现

// executeGate2StructureAggregationWithMonitoring 执行Gate2结构指标聚合（带性能监控）
// 这是Gate2系统的核心集成方法，统一调用AnchorEngine、StructStateClassifier和TriggerDetector
func (ca *ComprehensiveAnalyzer) executeGate2StructureAggregationWithMonitoring(
	symbol string, 
	lastPrice, atr14 float64, 
	mtfAnalysis *MultiTimeframeAnalysis,
	klines5m []Kline,
) (*StructureGate2, float64, float64, float64, error) {
	start := time.Now()
	
	// 获取feature manager以便记录模块性能
	featureManager := GetGate2FeatureManager()
	
	var anchorEngineTime, structClassifierTime, triggerDetectorTime float64
	
	// 步骤1: 创建并配置AnchorEngine
	if !featureManager.IsModuleEnabled("anchor_engine") {
		log.Printf("🚩 [Gate2] AnchorEngine模块被禁用: %s", symbol)
		return &StructureGate2{
			TopAnchorsLong:    []AnchorCandidate{},
			TopAnchorsShort:   []AnchorCandidate{},
			StructStateLong:   StructStateNeutral,
			StructStateShort:  StructStateNeutral,
			TriggerResult:     nil,
		}, 0, 0, 0, fmt.Errorf("AnchorEngine模块被禁用")
	}
	
	// 执行AnchorEngine处理
	anchorStart := time.Now()
	anchorEngine := NewAnchorEngine(nil) // 使用默认配置
	
	// 步骤2: 创建时间框架数据映射
	timeframes := make(map[string][]Kline)
	for tf, _ := range mtfAnalysis.Timeframes {
		// 从分析中恢复K线数据（简化版本，实际可能需要传入原始数据）
		// 这里我们需要重构来传入时间框架K线数据
		timeframes[tf] = []Kline{} // 暂时为空，后续需要完善
	}
	
	// 步骤3: 执行锚点聚合处理
	longCandidates, shortCandidates := ca.processAnchorCandidates(anchorEngine, mtfAnalysis, timeframes, lastPrice)
	
	anchorEngineTime = time.Since(anchorStart).Seconds() * 1000
	log.Printf("🎯 [Gate2] %s AnchorEngine处理完成 - 多头锚点: %d, 空头锚点: %d, 耗时: %.2fms", 
		symbol, len(longCandidates), len(shortCandidates), anchorEngineTime)
	
	// 步骤4: 创建并执行StructState分类器
	var structStateResult *StructStateResult
	structStart := time.Now()
	
	if featureManager.IsModuleEnabled("struct_classifier") {
		structClassifier := NewStructStateClassifier(nil) // 使用默认配置
		structStateResult = structClassifier.Classify(longCandidates, shortCandidates, lastPrice, atr14)
		
		structClassifierTime = time.Since(structStart).Seconds() * 1000
		log.Printf("🎯 [Gate2] %s StructState分类完成 - LONG:%s(%.2f) SHORT:%s(%.2f), 耗时: %.2fms", 
			symbol, 
			structStateResult.Long.State, structStateResult.Long.Confidence,
			structStateResult.Short.State, structStateResult.Short.Confidence,
			structClassifierTime)
	} else {
		// StructStateClassifier模块禁用，使用默认中性状态
		structStateResult = &StructStateResult{
			Long: StructStateClassification{
				State:      StructStateNeutral,
				Confidence: 0.5,
			},
			Short: StructStateClassification{
				State:      StructStateNeutral,
				Confidence: 0.5,
			},
		}
		log.Printf("🚩 [Gate2] StructStateClassifier模块被禁用: %s", symbol)
	}
	
	// 步骤5: 选择最佳锚点
	var bestLong, bestShort *AnchorCandidate
	if len(longCandidates) > 0 {
		bestLong = &longCandidates[0]
	}
	if len(shortCandidates) > 0 {
		bestShort = &shortCandidates[0]
	}
	
	// 步骤6: 5分钟触发检测（如果有5分钟数据）
	var triggerResult *TriggerResult
	triggerStart := time.Now()
	
	if featureManager.IsModuleEnabled("trigger_detector") && len(klines5m) > 1 {
		triggerResult = ca.detectFiveMinuteTrigger(klines5m, longCandidates, shortCandidates, atr14)
		triggerDetectorTime = time.Since(triggerStart).Seconds() * 1000
		log.Printf("🎯 [Gate2] %s TriggerDetector执行完成 - 触发状态: %v, 耗时: %.2fms", 
			symbol, triggerResult != nil && triggerResult.IsTriggered, triggerDetectorTime)
	} else {
		if !featureManager.IsModuleEnabled("trigger_detector") {
			log.Printf("🚩 [Gate2] TriggerDetector模块被禁用: %s", symbol)
		}
		triggerResult = nil
	}
	
	// 步骤7: 构建评分明细
	scoreBreakdown := ca.buildScoreBreakdown(longCandidates, shortCandidates)
	
	// 步骤8: 构建StructureGate2结果
	processingTime := time.Since(start).Seconds() * 1000 // 转换为毫秒
	
	structureGate2 := &StructureGate2{
		Symbol:               symbol,
		Timestamp:            time.Now(),
		LastPrice:            lastPrice,
		ATR14:                atr14,
		TopAnchorsLong:       getLimitedAnchors(longCandidates, 5),
		TopAnchorsShort:      getLimitedAnchors(shortCandidates, 5),
		BestAnchorLong:       bestLong,
		BestAnchorShort:      bestShort,
		StructStateLong:      structStateResult.Long.State,
		StructStateShort:     structStateResult.Short.State,
		StructStateResult:    structStateResult,
		TriggerResult:        triggerResult,
		AnchorScoreBreakdown: scoreBreakdown,
		TriggerContext: &TriggerContextInfo{
			IsKlineClosed:   true,
			Mode:            "normal",
			AnchorCloseTime: time.Now().UnixMilli(),
			AnchorTimeStr:   time.Now().Format("15:04:05.000"),
			TriggerType:     "5m_close",
			DataConsistency: "aligned",
		},
		TotalCandidatesLong:  len(longCandidates),
		TotalCandidatesShort: len(shortCandidates),
		ProcessingTimeMs:     processingTime,
		Version:              "Gate2-V13.5",
		Revision:             "1.0.0",
	}
	
	return structureGate2, anchorEngineTime, structClassifierTime, triggerDetectorTime, nil
}

// processAnchorCandidates 处理锚点候选者聚合
func (ca *ComprehensiveAnalyzer) processAnchorCandidates(
	engine *AnchorEngine, 
	mtfAnalysis *MultiTimeframeAnalysis, 
	timeframes map[string][]Kline,
	lastPrice float64,
) ([]AnchorCandidate, []AnchorCandidate) {
	
	// 收集各时间框架的分析数据
	var supplyDemandData *SupplyDemandData
	var vpvrData *VolumeProfile
	var srData *SupportResistanceData
	var fvgData *FVGData
	var fibData *FibonacciData
	
	// 优先使用4h时间框架的数据，如果没有则使用其他时间框架
	if tf4h, exists := mtfAnalysis.Timeframes["4h"]; exists {
		supplyDemandData = tf4h.SupplyDemand
		vpvrData = tf4h.VolumeProfile
		srData = tf4h.SupportResistance
		fvgData = tf4h.FairValueGaps
		fibData = tf4h.Fibonacci
		log.Printf("🔍 [Gate2] 使用4h时间框架数据作为主要分析源")
	} else if tf1h, exists := mtfAnalysis.Timeframes["1h"]; exists {
		supplyDemandData = tf1h.SupplyDemand
		vpvrData = tf1h.VolumeProfile
		srData = tf1h.SupportResistance
		fvgData = tf1h.FairValueGaps
		fibData = tf1h.Fibonacci
		log.Printf("🔍 [Gate2] 使用1h时间框架数据作为主要分析源")
	} else {
		log.Printf("⚠️ [Gate2] 未找到4h或1h数据，使用空数据集")
	}
	
	// 计算ATR14（简化版本，使用固定值）
	atr14 := 100.0 // 这应该从调用方传入
	
	// 使用AnchorEngine处理候选者
	longCandidates, shortCandidates, err := engine.ProcessCandidates(
		supplyDemandData, vpvrData, srData, fvgData, fibData,
		lastPrice, atr14, timeframes)
	
	if err != nil {
		log.Printf("❌ [Gate2] AnchorEngine处理失败: %v", err)
		return []AnchorCandidate{}, []AnchorCandidate{}
	}
	
	log.Printf("🔍 [Gate2] AnchorEngine处理完成 - 多头锚点: %d, 空头锚点: %d", 
		len(longCandidates), len(shortCandidates))
	
	return longCandidates, shortCandidates
}

// detectFiveMinuteTrigger 检测5分钟触发信号
func (ca *ComprehensiveAnalyzer) detectFiveMinuteTrigger(
	klines5m []Kline, 
	longCandidates, shortCandidates []AnchorCandidate,
	atr14 float64,
) *TriggerResult {
	
	if len(klines5m) < 2 {
		return nil
	}
	
	// 创建TriggerDetector
	detector := NewTriggerDetector(nil) // 使用默认配置
	
	// 准备当前K线和历史K线数据
	currentKline := &CandleInfo{
		Timestamp: klines5m[len(klines5m)-1].CloseTime,
		Open:      klines5m[len(klines5m)-1].Open,
		High:      klines5m[len(klines5m)-1].High,
		Low:       klines5m[len(klines5m)-1].Low,
		Close:     klines5m[len(klines5m)-1].Close,
		Volume:    klines5m[len(klines5m)-1].Volume,
	}
	
	// 准备历史K线（取前5根）
	var previousKlines []CandleInfo
	start := len(klines5m) - 6 // 取前5根
	if start < 0 {
		start = 0
	}
	for i := start; i < len(klines5m)-1; i++ {
		previousKlines = append(previousKlines, CandleInfo{
			Timestamp: klines5m[i].CloseTime,
			Open:      klines5m[i].Open,
			High:      klines5m[i].High,
			Low:       klines5m[i].Low,
			Close:     klines5m[i].Close,
			Volume:    klines5m[i].Volume,
		})
	}
	
	// 合并所有锚点
	allAnchors := append(longCandidates, shortCandidates...)
	
	// 计算量能比率和强度Z分数（简化版本）
	volumeRatio := 1.0
	strengthZ := 0.5
	if len(previousKlines) > 0 {
		avgVolume := 0.0
		for _, kline := range previousKlines {
			avgVolume += kline.Volume
		}
		avgVolume /= float64(len(previousKlines))
		if avgVolume > 0 {
			volumeRatio = currentKline.Volume / avgVolume
		}
	}
	
	// 执行触发检测
	return detector.DetectTrigger(currentKline, previousKlines, allAnchors, volumeRatio, strengthZ, atr14, time.Now())
}

// buildScoreBreakdown 构建评分明细
func (ca *ComprehensiveAnalyzer) buildScoreBreakdown(longCandidates, shortCandidates []AnchorCandidate) map[string]interface{} {
	breakdown := make(map[string]interface{})
	
	// 统计各优先级锚点数量
	priorityCount := make(map[int]int)
	typeCount := make(map[string]int)
	
	allCandidates := append(longCandidates, shortCandidates...)
	for _, candidate := range allCandidates {
		priorityCount[candidate.PriorityRank]++
		typeCount[string(candidate.Type)]++
	}
	
	breakdown["priority_distribution"] = priorityCount
	breakdown["type_distribution"] = typeCount
	breakdown["total_candidates"] = len(allCandidates)
	breakdown["long_candidates"] = len(longCandidates)
	breakdown["short_candidates"] = len(shortCandidates)
	
	// 计算平均分数
	if len(allCandidates) > 0 {
		totalScore := 0.0
		for _, candidate := range allCandidates {
			totalScore += candidate.AnchorScore
		}
		breakdown["average_score"] = totalScore / float64(len(allCandidates))
	}
	
	return breakdown
}

// getLimitedAnchors 获取限制数量的锚点
func getLimitedAnchors(anchors []AnchorCandidate, limit int) []AnchorCandidate {
	if len(anchors) <= limit {
		return anchors
	}
	return anchors[:limit]
}

// calculateATR14 计算14周期ATR
func calculateATR14(klines []Kline) float64 {
	if len(klines) < 14 {
		return 100.0 // 默认值
	}
	
	var trs []float64
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close
		
		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)
		
		tr := math.Max(tr1, math.Max(tr2, tr3))
		trs = append(trs, tr)
	}
	
	// 计算最后14个TR的平均值
	start := len(trs) - 14
	if start < 0 {
		start = 0
	}
	
	sum := 0.0
	count := 0
	for i := start; i < len(trs); i++ {
		sum += trs[i]
		count++
	}
	
	if count == 0 {
		return 100.0
	}
	
	return sum / float64(count)
}
