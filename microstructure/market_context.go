package microstructure

import (
	"math"
	"sync"
	"time"
)

// ===== 市场上下文分析相关结构 =====

// MarketContext 市场上下文（资金博弈分析）
type MarketContext struct {
	Symbol           string            `json:"symbol"`
	CVDDivergence    bool              `json:"cvd_divergence"`     // CVD与价格是否背离
	ContextInference string            `json:"context_inference"`  // 市场上下文推断
	SignalStrength   float64           `json:"signal_strength"`    // 信号强度 (0-100)
	GameMatrix       *GameMatrix       `json:"game_matrix"`        // 博弈矩阵分析
	FlowSummary      *FlowSummary      `json:"flow_summary"`       // 资金流向总结
	LastUpdate       time.Time         `json:"last_update"`
	IsStale          bool              `json:"is_stale"`
}

// GameMatrix 资金博弈矩阵分析
type GameMatrix struct {
	CVDDirection     string  `json:"cvd_direction"`      // CVD方向: bullish/bearish/neutral
	OIDirection      string  `json:"oi_direction"`       // OI方向: increasing/decreasing/stable
	PriceDirection   string  `json:"price_direction"`    // 价格方向: up/down/sideways
	MatrixType       string  `json:"matrix_type"`        // 博弈类型
	Confidence       float64 `json:"confidence"`         // 置信度 (0-100)
	Description      string  `json:"description"`        // 详细描述
}

// FlowSummary 资金流向总结
type FlowSummary struct {
	SpotFlow         string  `json:"spot_flow"`          // 现货资金流向
	FuturesFlow      string  `json:"futures_flow"`       // 合约资金流向
	FlowDivergence   bool    `json:"flow_divergence"`    // 现货合约是否背离
	DominantMarket   string  `json:"dominant_market"`    // 主导市场: spot/futures/balanced
	FlowIntensity    string  `json:"flow_intensity"`     // 流向强度: weak/moderate/strong/extreme
}

// PriceContext 价格上下文（用于分析）
type PriceContext struct {
	CurrentPrice   float64 `json:"current_price"`
	Change1H       float64 `json:"change_1h"`       // 1小时价格变化（百分比）
	Change4H       float64 `json:"change_4h"`       // 4小时价格变化（百分比）
	Volatility     float64 `json:"volatility"`      // 波动率
}

// ===== 市场上下文分析器 =====

// MarketContextAnalyzer 市场上下文分析器
type MarketContextAnalyzer struct {
	mu sync.RWMutex
}

// NewMarketContextAnalyzer 创建市场上下文分析器
func NewMarketContextAnalyzer() *MarketContextAnalyzer {
	return &MarketContextAnalyzer{}
}

// AnalyzeMarketContext 分析市场上下文
func (analyzer *MarketContextAnalyzer) AnalyzeMarketContext(
	symbol string,
	cvdData *CVDData,
	oiAnalysis *OIAnalysis,
	priceContext *PriceContext,
) *MarketContext {
	analyzer.mu.Lock()
	defer analyzer.mu.Unlock()

	// 1. 分析CVD方向
	cvdDirection := analyzer.analyzeCVDDirection(cvdData)
	
	// 2. 分析OI方向
	oiDirection := analyzer.analyzeOIDirection(oiAnalysis)
	
	// 3. 分析价格方向
	priceDirection := analyzer.analyzePriceDirection(priceContext)
	
	// 4. 构建博弈矩阵
	gameMatrix := analyzer.buildGameMatrix(cvdDirection, oiDirection, priceDirection)
	
	// 5. 分析资金流向
	flowSummary := analyzer.analyzeFlowSummary(cvdData, gameMatrix)
	
	// 6. 检测背离
	cvdDivergence := analyzer.detectCVDPriceDivergence(cvdDirection, priceDirection, cvdData, priceContext)
	
	// 7. 生成上下文推断
	contextInference := analyzer.generateContextInference(gameMatrix, flowSummary, cvdDivergence)
	
	// 8. 计算信号强度
	signalStrength := analyzer.calculateSignalStrength(gameMatrix, flowSummary, cvdData, oiAnalysis)
	
	return &MarketContext{
		Symbol:           symbol,
		CVDDivergence:    cvdDivergence,
		ContextInference: contextInference,
		SignalStrength:   signalStrength,
		GameMatrix:       gameMatrix,
		FlowSummary:      flowSummary,
		LastUpdate:       time.Now(),
		IsStale:          cvdData.IsStale || oiAnalysis.IsStale,
	}
}

// analyzeCVDDirection 分析CVD方向
func (analyzer *MarketContextAnalyzer) analyzeCVDDirection(cvdData *CVDData) string {
	// 综合现货和合约CVD
	totalCVD := cvdData.SpotCVD1H + cvdData.FuturesCVD1H
	threshold := 500000.0 // 50万USD阈值

	if totalCVD > threshold {
		return "bullish"
	} else if totalCVD < -threshold {
		return "bearish"
	} else {
		return "neutral"
	}
}

// analyzeOIDirection 分析OI方向
func (analyzer *MarketContextAnalyzer) analyzeOIDirection(oiAnalysis *OIAnalysis) string {
	// 直接使用OI分析的趋势
	switch oiAnalysis.Trend {
	case "increasing":
		return "increasing"
	case "decreasing":
		return "decreasing"
	default:
		return "stable"
	}
}

// analyzePriceDirection 分析价格方向
func (analyzer *MarketContextAnalyzer) analyzePriceDirection(priceContext *PriceContext) string {
	// 基于1小时价格变化判断方向
	threshold := 0.5 // 0.5%阈值

	if priceContext.Change1H > threshold {
		return "up"
	} else if priceContext.Change1H < -threshold {
		return "down"
	} else {
		return "sideways"
	}
}

// buildGameMatrix 构建资金博弈矩阵
func (analyzer *MarketContextAnalyzer) buildGameMatrix(cvdDirection, oiDirection, priceDirection string) *GameMatrix {
	// CVD+OI+Price 三维博弈矩阵分析
	matrixType, description, confidence := analyzer.classifyMarketBehavior(cvdDirection, oiDirection, priceDirection)
	
	return &GameMatrix{
		CVDDirection:   cvdDirection,
		OIDirection:    oiDirection,
		PriceDirection: priceDirection,
		MatrixType:     matrixType,
		Confidence:     confidence,
		Description:    description,
	}
}

// classifyMarketBehavior 分类市场行为
func (analyzer *MarketContextAnalyzer) classifyMarketBehavior(cvd, oi, price string) (matrixType, description string, confidence float64) {
	// 基于技术方案中的博弈矩阵表
	switch {
	case cvd == "bullish" && oi == "increasing" && price == "up":
		return "long_buildup", "Long Build-up (多头强势开仓) - 真涨", 95.0
	
	case cvd == "bullish" && oi == "decreasing" && price == "up":
		return "short_covering", "Short Covering (空头止损平仓) - 逼空", 90.0
	
	case cvd == "bearish" && oi == "increasing" && price == "down":
		return "short_buildup", "Short Build-up (空头强势开仓) - 真跌", 95.0
	
	case cvd == "bearish" && oi == "decreasing" && price == "down":
		return "long_liquidation", "Long Liquidation (多头止损平仓) - 爆仓", 90.0
	
	case cvd == "bullish" && oi == "increasing" && price == "down":
		return "bullish_absorption", "Bullish Absorption (看涨吸筹) - 底部收集", 85.0
	
	case cvd == "bearish" && oi == "increasing" && price == "up":
		return "bearish_distribution", "Bearish Distribution (看跌派发) - 顶部出货", 85.0
	
	case cvd == "neutral" && oi == "stable" && price == "sideways":
		return "consolidation", "Market Consolidation (市场整理) - 横盘震荡", 70.0
	
	case cvd == "bullish" && oi == "stable":
		return "spot_led_rally", "Spot-Led Rally (现货领涨) - 机构建仓", 80.0
	
	case cvd == "bearish" && oi == "stable":
		return "spot_led_decline", "Spot-Led Decline (现货领跌) - 机构减仓", 80.0
	
	default:
		return "mixed_signals", "Mixed Signals (信号混合) - 市场分歧", 50.0
	}
}

// analyzeFlowSummary 分析资金流向总结
func (analyzer *MarketContextAnalyzer) analyzeFlowSummary(cvdData *CVDData, gameMatrix *GameMatrix) *FlowSummary {
	// 分析现货流向
	spotFlow := "neutral"
	if cvdData.SpotCVD1H > 1000000 {
		spotFlow = "strong_inflow"
	} else if cvdData.SpotCVD1H > 100000 {
		spotFlow = "moderate_inflow"
	} else if cvdData.SpotCVD1H < -1000000 {
		spotFlow = "strong_outflow"
	} else if cvdData.SpotCVD1H < -100000 {
		spotFlow = "moderate_outflow"
	}

	// 分析合约流向
	futuresFlow := "neutral"
	if cvdData.FuturesCVD1H > 1000000 {
		futuresFlow = "strong_inflow"
	} else if cvdData.FuturesCVD1H > 100000 {
		futuresFlow = "moderate_inflow"
	} else if cvdData.FuturesCVD1H < -1000000 {
		futuresFlow = "strong_outflow"
	} else if cvdData.FuturesCVD1H < -100000 {
		futuresFlow = "moderate_outflow"
	}

	// 检测现货合约背离
	flowDivergence := analyzer.detectFlowDivergence(cvdData.SpotCVD1H, cvdData.FuturesCVD1H)

	// 确定主导市场
	dominantMarket := analyzer.determineDominantMarket(cvdData.SpotCVD1H, cvdData.FuturesCVD1H)

	// 计算流向强度
	flowIntensity := analyzer.calculateFlowIntensity(cvdData.SpotCVD1H, cvdData.FuturesCVD1H)

	return &FlowSummary{
		SpotFlow:       spotFlow,
		FuturesFlow:    futuresFlow,
		FlowDivergence: flowDivergence,
		DominantMarket: dominantMarket,
		FlowIntensity:  flowIntensity,
	}
}

// detectFlowDivergence 检测资金流背离
func (analyzer *MarketContextAnalyzer) detectFlowDivergence(spotCVD, futuresCVD float64) bool {
	threshold := 500000.0 // 50万USD阈值
	
	// 现货和合约方向相反且都超过阈值
	return (spotCVD > threshold && futuresCVD < -threshold) ||
		   (spotCVD < -threshold && futuresCVD > threshold)
}

// determineDominantMarket 确定主导市场
func (analyzer *MarketContextAnalyzer) determineDominantMarket(spotCVD, futuresCVD float64) string {
	spotAbs := math.Abs(spotCVD)
	futuresAbs := math.Abs(futuresCVD)
	
	if spotAbs > futuresAbs*1.5 {
		return "spot"
	} else if futuresAbs > spotAbs*1.5 {
		return "futures"
	} else {
		return "balanced"
	}
}

// calculateFlowIntensity 计算流向强度
func (analyzer *MarketContextAnalyzer) calculateFlowIntensity(spotCVD, futuresCVD float64) string {
	totalFlow := math.Abs(spotCVD) + math.Abs(futuresCVD)
	
	if totalFlow > 10000000 { // 1000万USD
		return "extreme"
	} else if totalFlow > 5000000 { // 500万USD
		return "strong"
	} else if totalFlow > 1000000 { // 100万USD
		return "moderate"
	} else {
		return "weak"
	}
}

// detectCVDPriceDivergence 检测CVD与价格背离
func (analyzer *MarketContextAnalyzer) detectCVDPriceDivergence(cvdDirection, priceDirection string, cvdData *CVDData, priceContext *PriceContext) bool {
	// 强烈的CVD信号与价格方向相反
	totalCVD := cvdData.SpotCVD1H + cvdData.FuturesCVD1H
	cvdThreshold := 1000000.0  // 100万USD强信号阈值
	priceThreshold := 1.0      // 1%价格变化阈值
	
	strongCVDSignal := math.Abs(totalCVD) > cvdThreshold
	significantPriceMove := math.Abs(priceContext.Change1H) > priceThreshold
	
	if strongCVDSignal && significantPriceMove {
		// CVD看涨但价格下跌，或CVD看跌但价格上涨
		return (cvdDirection == "bullish" && priceDirection == "down") ||
			   (cvdDirection == "bearish" && priceDirection == "up")
	}
	
	return false
}

// generateContextInference 生成上下文推断
func (analyzer *MarketContextAnalyzer) generateContextInference(gameMatrix *GameMatrix, flowSummary *FlowSummary, cvdDivergence bool) string {
	// 基于博弈矩阵和资金流向生成描述性推断
	baseContext := gameMatrix.MatrixType

	// 添加背离标记
	if cvdDivergence {
		baseContext += "_with_divergence"
	}

	// 添加主导市场信息
	if flowSummary.DominantMarket != "balanced" {
		baseContext += "_" + flowSummary.DominantMarket + "_led"
	}

	// 添加流向背离信息
	if flowSummary.FlowDivergence {
		baseContext += "_spot_futures_divergence"
	}

	return baseContext
}

// calculateSignalStrength 计算信号强度
func (analyzer *MarketContextAnalyzer) calculateSignalStrength(gameMatrix *GameMatrix, flowSummary *FlowSummary, cvdData *CVDData, oiAnalysis *OIAnalysis) float64 {
	baseStrength := gameMatrix.Confidence

	// 根据资金流向强度调整
	switch flowSummary.FlowIntensity {
	case "extreme":
		baseStrength += 10
	case "strong":
		baseStrength += 5
	case "weak":
		baseStrength -= 10
	}

	// 根据OI变化率调整
	if math.Abs(oiAnalysis.ChangeRate1H) > 2.0 { // 2%以上变化
		baseStrength += 5
	}

	// 根据背离情况调整
	if flowSummary.FlowDivergence {
		baseStrength += 15 // 背离信号通常更有价值
	}

	// 确保在0-100范围内
	if baseStrength > 100 {
		baseStrength = 100
	} else if baseStrength < 0 {
		baseStrength = 0
	}

	return baseStrength
}

// GetMarketRegime 获取市场状态
func (analyzer *MarketContextAnalyzer) GetMarketRegime(context *MarketContext) string {
	switch context.GameMatrix.MatrixType {
	case "long_buildup":
		return "bull_trend"
	case "short_buildup":
		return "bear_trend"
	case "short_covering":
		return "short_squeeze"
	case "long_liquidation":
		return "long_squeeze"
	case "bullish_absorption":
		return "accumulation"
	case "bearish_distribution":
		return "distribution"
	case "consolidation":
		return "range_bound"
	default:
		return "uncertain"
	}
}