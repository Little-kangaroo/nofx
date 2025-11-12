package market

import (
	"fmt"
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
	config             *ComprehensiveConfig
}

// ComprehensiveConfig 综合分析配置
type ComprehensiveConfig struct {
	EnableDowTheory    bool    `json:"enable_dow_theory"`    // 启用道氏理论
	EnableVPVR         bool    `json:"enable_vpvr"`          // 启用VPVR
	EnableSupplyDemand bool    `json:"enable_supply_demand"` // 启用供需区
	EnableFVG          bool    `json:"enable_fvg"`           // 启用FVG
	EnableFibonacci    bool    `json:"enable_fibonacci"`     // 启用斐波纳契
	WeightDowTheory    float64 `json:"weight_dow_theory"`    // 道氏理论权重
	WeightVPVR         float64 `json:"weight_vpvr"`          // VPVR权重
	WeightSupplyDemand float64 `json:"weight_supply_demand"` // 供需区权重
	WeightFVG          float64 `json:"weight_fvg"`           // FVG权重
	WeightFibonacci    float64 `json:"weight_fibonacci"`     // 斐波纳契权重
	MinConfidence      float64 `json:"min_confidence"`       // 最小置信度
	MaxSignals         int     `json:"max_signals"`          // 最大信号数量
}

// ComprehensiveResult 综合分析结果
type ComprehensiveResult struct {
	Symbol           string               `json:"symbol"`            // 交易对
	Timestamp        int64                `json:"timestamp"`         // 分析时间
	CurrentPrice     float64              `json:"current_price"`     // 当前价格
	DowTheory        *DowTheoryData       `json:"dow_theory"`        // 道氏理论分析
	ChannelAnalysis  *ChannelData         `json:"channel_analysis"`  // 通道分析（独立指标）
	VolumeProfile    *VolumeProfile       `json:"volume_profile"`    // 成交量分布
	SupplyDemand     *SupplyDemandData    `json:"supply_demand"`     // 供需区分析
	FairValueGaps    *FVGData             `json:"fair_value_gaps"`   // FVG分析
	Fibonacci        *FibonacciData       `json:"fibonacci"`         // 斐波纳契分析
	UnifiedSignals   []*UnifiedSignal     `json:"unified_signals"`   // 统一交易信号
	MarketStructure  *MarketStructure     `json:"market_structure"`  // 市场结构
	RiskAssessment   *RiskAssessment      `json:"risk_assessment"`   // 风险评估
	TradingAdvice    *TradingAdvice       `json:"trading_advice"`    // 交易建议
	Config           *ComprehensiveConfig `json:"config"`            // 分析配置
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
	EnableDowTheory:    true,
	EnableVPVR:         true,
	EnableSupplyDemand: true,
	EnableFVG:          true,
	EnableFibonacci:    true,
	WeightDowTheory:    0.25,
	WeightVPVR:         0.2,
	WeightSupplyDemand: 0.2,
	WeightFVG:          0.15,
	WeightFibonacci:    0.2,
	MinConfidence:      60.0,
	MaxSignals:         6,
}

// NewComprehensiveAnalyzer 创建综合分析器
func NewComprehensiveAnalyzer() *ComprehensiveAnalyzer {
	return &ComprehensiveAnalyzer{
		dowAnalyzer:       NewDowTheoryAnalyzer(),
		channelAnalyzer:   NewChannelAnalyzer(),
		vpvrAnalyzer:      NewVPVRAnalyzer(),
		sdAnalyzer:        NewSupplyDemandAnalyzer(),
		fvgAnalyzer:       NewFVGAnalyzer(),
		fibonacciAnalyzer: NewFibonacciAnalyzer(),
		config:            defaultComprehensiveConfig,
	}
}

// NewComprehensiveAnalyzerWithConfig 使用自定义配置创建综合分析器
func NewComprehensiveAnalyzerWithConfig(config *ComprehensiveConfig) *ComprehensiveAnalyzer {
	return &ComprehensiveAnalyzer{
		dowAnalyzer:       NewDowTheoryAnalyzer(),
		channelAnalyzer:   NewChannelAnalyzer(),
		vpvrAnalyzer:      NewVPVRAnalyzer(),
		sdAnalyzer:        NewSupplyDemandAnalyzer(),
		fvgAnalyzer:       NewFVGAnalyzer(),
		fibonacciAnalyzer: NewFibonacciAnalyzer(),
		config:            config,
	}
}

// Analyze 执行综合市场分析
func (ca *ComprehensiveAnalyzer) Analyze(symbol string, klines3m, klines4h []Kline) *ComprehensiveResult {
	if len(klines3m) == 0 && len(klines4h) == 0 {
		return nil
	}

	currentPrice := 0.0
	timestamp := time.Now().UnixMilli()

	// 确定当前价格
	if len(klines4h) > 0 {
		currentPrice = klines4h[len(klines4h)-1].Close
		timestamp = klines4h[len(klines4h)-1].CloseTime
	} else if len(klines3m) > 0 {
		currentPrice = klines3m[len(klines3m)-1].Close
		timestamp = klines3m[len(klines3m)-1].CloseTime
	}

	result := &ComprehensiveResult{
		Symbol:       symbol,
		Timestamp:    timestamp,
		CurrentPrice: currentPrice,
		Config:       ca.config,
	}

	// 执行道氏理论分析
	if ca.config.EnableDowTheory && len(klines4h) > 20 {
		result.DowTheory = ca.dowAnalyzer.Analyze(klines3m, klines4h, currentPrice)
		result.ChannelAnalysis = ca.channelAnalyzer.Analyze(klines4h, currentPrice)
	}

	// 执行VPVR分析
	if ca.config.EnableVPVR && len(klines4h) > 10 {
		result.VolumeProfile = ca.vpvrAnalyzer.Analyze(klines4h)
	}

	// 执行供需区分析
	if ca.config.EnableSupplyDemand && len(klines4h) > 15 {
		result.SupplyDemand = ca.sdAnalyzer.Analyze(klines4h)
	}

	// 执行FVG分析
	if ca.config.EnableFVG && len(klines4h) > 10 {
		result.FairValueGaps = ca.fvgAnalyzer.Analyze(klines4h)
	}

	// 执行斐波纳契分析
	if ca.config.EnableFibonacci && len(klines4h) > 15 {
		result.Fibonacci = ca.fibonacciAnalyzer.Analyze(klines4h)
	}

	// 生成统一信号
	result.UnifiedSignals = ca.generateUnifiedSignals(result, currentPrice)

	// 分析市场结构
	result.MarketStructure = ca.analyzeMarketStructure(result)

	// 评估风险
	result.RiskAssessment = ca.assessRisk(result)

	// 生成交易建议
	result.TradingAdvice = ca.generateTradingAdvice(result)

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


