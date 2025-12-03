package market

import (
	"log"
	"math"
	"time"
)

// NewMarketContextAnalyzer 创建市场上下文分析器
func NewMarketContextAnalyzer() *MarketContextAnalyzer {
	return &MarketContextAnalyzer{
		priceHistory:   make(map[string][]float64),
		oiHistory:      make(map[string][]float64),
		fundingHistory: make(map[string][]float64),
	}
}

// AnalyzeMarketContext 分析市场上下文（核心方法）
// 参数：目标币种数据、BTC数据、市场数据映射
func (mca *MarketContextAnalyzer) AnalyzeMarketContext(
	targetSymbol string, 
	targetData *Data, 
	btcData *Data,
	marketDataMap map[string]*Data,
) *MarketContextData {
	if targetData == nil || btcData == nil {
		return nil
	}
	
	log.Printf("🔍 [市场上下文分析] 开始分析 %s vs BTC联动性", targetSymbol)
	
	// 更新历史数据
	mca.updateHistoryData(targetSymbol, targetData)
	mca.updateHistoryData("BTCUSDT", btcData)
	
	// 进行各项分析
	correlation := mca.analyzeCorrelation(targetSymbol, "BTCUSDT")
	oiAnalysis := mca.analyzeOI(targetSymbol, targetData, btcData)
	fundingContext := mca.analyzeFundingContext(targetSymbol, targetData, btcData)
	riskAssessment := mca.assessRisk(targetSymbol, targetData, btcData, correlation)
	
	// BTC主导地位分析（简化版，实际中可以通过API获取）
	btcDominance := mca.estimateBTCDominance(btcData, marketDataMap)
	
	return &MarketContextData{
		BTCDominance:   btcDominance,
		Correlation:    correlation,
		OIAnalysis:     oiAnalysis,
		FundingContext: fundingContext,
		RiskAssessment: riskAssessment,
		LastUpdated:    time.Now().UnixMilli(),
	}
}

// updateHistoryData 更新历史数据
func (mca *MarketContextAnalyzer) updateHistoryData(symbol string, data *Data) {
	maxHistory := 100 // 保留最近100个数据点
	
	// 更新价格历史
	if mca.priceHistory[symbol] == nil {
		mca.priceHistory[symbol] = make([]float64, 0, maxHistory)
	}
	mca.priceHistory[symbol] = append(mca.priceHistory[symbol], data.CurrentPrice)
	if len(mca.priceHistory[symbol]) > maxHistory {
		mca.priceHistory[symbol] = mca.priceHistory[symbol][1:]
	}
	
	// 更新OI历史
	if data.OpenInterest != nil {
		if mca.oiHistory[symbol] == nil {
			mca.oiHistory[symbol] = make([]float64, 0, maxHistory)
		}
		mca.oiHistory[symbol] = append(mca.oiHistory[symbol], data.OpenInterest.Latest)
		if len(mca.oiHistory[symbol]) > maxHistory {
			mca.oiHistory[symbol] = mca.oiHistory[symbol][1:]
		}
	}
	
	// 更新资金费率历史
	if mca.fundingHistory[symbol] == nil {
		mca.fundingHistory[symbol] = make([]float64, 0, maxHistory)
	}
	mca.fundingHistory[symbol] = append(mca.fundingHistory[symbol], data.FundingRate)
	if len(mca.fundingHistory[symbol]) > maxHistory {
		mca.fundingHistory[symbol] = mca.fundingHistory[symbol][1:]
	}
}

// analyzeCorrelation 分析相关性
func (mca *MarketContextAnalyzer) analyzeCorrelation(targetSymbol, btcSymbol string) *CorrelationData {
	targetPrices := mca.priceHistory[targetSymbol]
	btcPrices := mca.priceHistory[btcSymbol]
	
	if len(targetPrices) < 10 || len(btcPrices) < 10 {
		return &CorrelationData{
			BTCCorr1h:       0.0,
			BTCCorr4h:       0.0,
			BTCCorr24h:      0.0,
			CorrTrend:       "insufficient_data",
			DecouplingRisk:  50.0, // 中性风险
			BetaCoefficient: 1.0,  // 默认Beta
			Alpha:           0.0,  // 默认Alpha
		}
	}
	
	// 计算不同时间段的相关性
	corr1h := mca.calculateCorrelation(targetPrices, btcPrices, 12)  // 假设5分钟数据，12个点=1小时
	corr4h := mca.calculateCorrelation(targetPrices, btcPrices, 48)  // 48个点=4小时
	corr24h := mca.calculateCorrelation(targetPrices, btcPrices, 288) // 288个点=24小时
	
	// 分析相关性趋势
	corrTrend := mca.analyzeCorrTrend(corr1h, corr4h, corr24h)
	
	// 计算脱钩风险（相关性越低，脱钩风险越高）
	avgCorr := (math.Abs(corr1h) + math.Abs(corr4h) + math.Abs(corr24h)) / 3
	decouplingRisk := (1 - avgCorr) * 100
	
	// 计算Beta系数
	beta := mca.calculateBeta(targetPrices, btcPrices)
	
	// 计算Alpha
	alpha := mca.calculateAlpha(targetPrices, btcPrices, beta)
	
	return &CorrelationData{
		BTCCorr1h:       corr1h,
		BTCCorr4h:       corr4h,
		BTCCorr24h:      corr24h,
		CorrTrend:       corrTrend,
		DecouplingRisk:  decouplingRisk,
		BetaCoefficient: beta,
		Alpha:           alpha,
	}
}

// calculateCorrelation 计算皮尔逊相关系数
func (mca *MarketContextAnalyzer) calculateCorrelation(x, y []float64, periods int) float64 {
	if len(x) < periods || len(y) < periods {
		return 0.0
	}
	
	// 取最近的periods个数据点
	startX := len(x) - periods
	startY := len(y) - periods
	
	xData := x[startX:]
	yData := y[startY:]
	
	// 计算收益率
	xReturns := make([]float64, periods-1)
	yReturns := make([]float64, periods-1)
	
	for i := 1; i < periods; i++ {
		if xData[i-1] != 0 && yData[i-1] != 0 {
			xReturns[i-1] = (xData[i] - xData[i-1]) / xData[i-1]
			yReturns[i-1] = (yData[i] - yData[i-1]) / yData[i-1]
		}
	}
	
	return calculatePearsonCorrelation(xReturns, yReturns)
}

// calculatePearsonCorrelation 计算皮尔逊相关系数
func calculatePearsonCorrelation(x, y []float64) float64 {
	if len(x) != len(y) || len(x) == 0 {
		return 0.0
	}
	
	n := float64(len(x))
	
	// 计算均值
	sumX, sumY := 0.0, 0.0
	for i := 0; i < len(x); i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / n
	meanY := sumY / n
	
	// 计算协方差和标准差
	numerator := 0.0
	sumSqX := 0.0
	sumSqY := 0.0
	
	for i := 0; i < len(x); i++ {
		diffX := x[i] - meanX
		diffY := y[i] - meanY
		
		numerator += diffX * diffY
		sumSqX += diffX * diffX
		sumSqY += diffY * diffY
	}
	
	denominator := math.Sqrt(sumSqX * sumSqY)
	if denominator == 0 {
		return 0.0
	}
	
	return numerator / denominator
}

// analyzeCorrTrend 分析相关性趋势
func (mca *MarketContextAnalyzer) analyzeCorrTrend(corr1h, corr4h, corr24h float64) string {
	// 比较短期和长期相关性
	shortTermAvg := (math.Abs(corr1h) + math.Abs(corr4h)) / 2
	longTermCorr := math.Abs(corr24h)
	
	diff := shortTermAvg - longTermCorr
	
	if diff > 0.1 {
		return "strengthening"
	} else if diff < -0.1 {
		return "weakening"
	} else {
		return "stable"
	}
}

// calculateBeta 计算Beta系数
func (mca *MarketContextAnalyzer) calculateBeta(targetPrices, btcPrices []float64) float64 {
	if len(targetPrices) < 30 || len(btcPrices) < 30 {
		return 1.0 // 默认Beta
	}
	
	periods := int(math.Min(float64(len(targetPrices)), float64(len(btcPrices))))
	periods = int(math.Min(float64(periods), 50)) // 最多使用50个数据点
	
	// 计算收益率
	targetReturns := make([]float64, periods-1)
	btcReturns := make([]float64, periods-1)
	
	for i := 1; i < periods; i++ {
		if targetPrices[i-1] != 0 && btcPrices[i-1] != 0 {
			targetReturns[i-1] = (targetPrices[i] - targetPrices[i-1]) / targetPrices[i-1]
			btcReturns[i-1] = (btcPrices[i] - btcPrices[i-1]) / btcPrices[i-1]
		}
	}
	
	// 计算协方差和方差
	covariance := calculateCovariance(targetReturns, btcReturns)
	variance := calculateVariance(btcReturns)
	
	if variance == 0 {
		return 1.0
	}
	
	return covariance / variance
}

// calculateAlpha 计算Alpha（CAPM模型）
func (mca *MarketContextAnalyzer) calculateAlpha(targetPrices, btcPrices []float64, beta float64) float64 {
	if len(targetPrices) < 30 || len(btcPrices) < 30 {
		return 0.0
	}
	
	periods := int(math.Min(float64(len(targetPrices)), float64(len(btcPrices))))
	periods = int(math.Min(float64(periods), 50))
	
	// 计算平均收益率
	targetReturn := (targetPrices[periods-1] - targetPrices[0]) / targetPrices[0] / float64(periods-1)
	btcReturn := (btcPrices[periods-1] - btcPrices[0]) / btcPrices[0] / float64(periods-1)
	
	// Alpha = 目标收益 - Beta * BTC收益
	alpha := targetReturn - beta*btcReturn
	
	return alpha * 100 // 转换为百分比
}

// calculateCovariance 计算协方差
func calculateCovariance(x, y []float64) float64 {
	if len(x) != len(y) || len(x) == 0 {
		return 0.0
	}
	
	n := float64(len(x))
	
	// 计算均值
	sumX, sumY := 0.0, 0.0
	for i := 0; i < len(x); i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / n
	meanY := sumY / n
	
	// 计算协方差
	covariance := 0.0
	for i := 0; i < len(x); i++ {
		covariance += (x[i] - meanX) * (y[i] - meanY)
	}
	
	return covariance / (n - 1)
}

// calculateVariance 计算方差
func calculateVariance(x []float64) float64 {
	if len(x) == 0 {
		return 0.0
	}
	
	n := float64(len(x))
	
	// 计算均值
	sum := 0.0
	for _, value := range x {
		sum += value
	}
	mean := sum / n
	
	// 计算方差
	variance := 0.0
	for _, value := range x {
		diff := value - mean
		variance += diff * diff
	}
	
	return variance / (n - 1)
}

// analyzeOI 分析持仓量
func (mca *MarketContextAnalyzer) analyzeOI(targetSymbol string, targetData, btcData *Data) *OIAnalysisData {
	if targetData.OpenInterest == nil {
		return &OIAnalysisData{
			CurrentOI:       0,
			OIChange1h:      0,
			OIChange4h:      0,
			OIChange24h:     0,
			OITrend:         "no_data",
			LongShortRatio:  1.0,
			LiquidationRisk: 50.0, // 中性风险
			BTCOICorr:       0.0,
		}
	}
	
	currentOI := targetData.OpenInterest.Latest
	averageOI := targetData.OpenInterest.Average
	
	// 估算OI变化率（基于当前OI与平均OI的比较）
	oiChange24h := 0.0
	if averageOI > 0 {
		oiChange24h = (currentOI - averageOI) / averageOI * 100
	}
	
	// 估算短期变化（简化计算）
	oiChange1h := oiChange24h * 0.1   // 假设1小时变化是24小时变化的10%
	oiChange4h := oiChange24h * 0.3   // 假设4小时变化是24小时变化的30%
	
	// 分析OI趋势
	oiTrend := "stable"
	if oiChange24h > 5 {
		oiTrend = "rising"
	} else if oiChange24h < -5 {
		oiTrend = "falling"
	}
	
	// 估算多空比例（基于资金费率）
	longShortRatio := 1.0
	if targetData.FundingRate > 0.01 { // 资金费率偏高，多头占优
		longShortRatio = 1.2 + targetData.FundingRate*10
	} else if targetData.FundingRate < -0.01 { // 资金费率偏低，空头占优
		longShortRatio = 0.8 + targetData.FundingRate*10
	}
	
	// 计算清算风险
	liquidationRisk := mca.assessLiquidationRisk(oiChange24h, longShortRatio, targetData.FundingRate)
	
	// 计算与BTC OI的相关性
	btcOICorr := 0.0
	if btcData.OpenInterest != nil {
		btcOICorr = mca.calculateOICorrelation(targetSymbol, "BTCUSDT")
	}
	
	return &OIAnalysisData{
		CurrentOI:       currentOI,
		OIChange1h:      oiChange1h,
		OIChange4h:      oiChange4h,
		OIChange24h:     oiChange24h,
		OITrend:         oiTrend,
		LongShortRatio:  longShortRatio,
		LiquidationRisk: liquidationRisk,
		BTCOICorr:       btcOICorr,
	}
}

// calculateOICorrelation 计算OI相关性
func (mca *MarketContextAnalyzer) calculateOICorrelation(targetSymbol, btcSymbol string) float64 {
	targetOI := mca.oiHistory[targetSymbol]
	btcOI := mca.oiHistory[btcSymbol]
	
	if len(targetOI) < 10 || len(btcOI) < 10 {
		return 0.0
	}
	
	return calculatePearsonCorrelation(targetOI, btcOI)
}

// assessLiquidationRisk 评估清算风险
func (mca *MarketContextAnalyzer) assessLiquidationRisk(oiChange, longShortRatio, fundingRate float64) float64 {
	risk := 50.0 // 基础风险
	
	// OI快速增长增加风险
	if oiChange > 20 {
		risk += 20
	} else if oiChange > 10 {
		risk += 10
	}
	
	// 多空严重不平衡增加风险
	if longShortRatio > 1.5 || longShortRatio < 0.67 {
		risk += 15
	}
	
	// 极端资金费率增加风险
	if math.Abs(fundingRate) > 0.05 {
		risk += 15
	}
	
	// 确保风险在0-100范围内
	if risk > 100 {
		risk = 100
	}
	if risk < 0 {
		risk = 0
	}
	
	return risk
}

// analyzeFundingContext 分析资金费率上下文
func (mca *MarketContextAnalyzer) analyzeFundingContext(targetSymbol string, targetData, btcData *Data) *FundingContextData {
	currentRate := targetData.FundingRate
	
	// 计算24小时平均资金费率
	fundingHistory := mca.fundingHistory[targetSymbol]
	averageRate24h := currentRate
	if len(fundingHistory) > 0 {
		sum := 0.0
		count := 0
		for _, rate := range fundingHistory {
			sum += rate
			count++
		}
		if count > 0 {
			averageRate24h = sum / float64(count)
		}
	}
	
	// 计算资金费率波动性
	rateVolatility := mca.calculateRateVolatility(fundingHistory)
	
	// 分析市场情绪
	marketSentiment := "neutral"
	if currentRate > 0.02 {
		marketSentiment = "bullish"
	} else if currentRate < -0.02 {
		marketSentiment = "bearish"
	}
	
	// 评估过热风险
	overheatingRisk := math.Abs(currentRate) * 1000 // 转换为0-100范围
	if overheatingRisk > 100 {
		overheatingRisk = 100
	}
	
	// 计算与BTC资金费率的相关性
	btcRateCorr := mca.calculateFundingCorrelation(targetSymbol, "BTCUSDT")
	
	return &FundingContextData{
		CurrentRate:     currentRate,
		AverageRate24h:  averageRate24h,
		RateVolatility:  rateVolatility,
		MarketSentiment: marketSentiment,
		OverheatingRisk: overheatingRisk,
		BTCRateCorr:     btcRateCorr,
	}
}

// calculateRateVolatility 计算资金费率波动性
func (mca *MarketContextAnalyzer) calculateRateVolatility(rates []float64) float64 {
	if len(rates) < 2 {
		return 0.0
	}
	
	return math.Sqrt(calculateVariance(rates))
}

// calculateFundingCorrelation 计算资金费率相关性
func (mca *MarketContextAnalyzer) calculateFundingCorrelation(targetSymbol, btcSymbol string) float64 {
	targetFunding := mca.fundingHistory[targetSymbol]
	btcFunding := mca.fundingHistory[btcSymbol]
	
	if len(targetFunding) < 10 || len(btcFunding) < 10 {
		return 0.0
	}
	
	return calculatePearsonCorrelation(targetFunding, btcFunding)
}

// estimateBTCDominance 估算BTC市值占比（简化版）
func (mca *MarketContextAnalyzer) estimateBTCDominance(btcData *Data, marketDataMap map[string]*Data) *BTCDominanceData {
	// 实际应用中应该通过API获取真实的BTC市值占比数据
	// 这里提供一个基于价格变化的估算版本
	
	current := 42.5 // 假设当前BTC占比42.5%（需要实际API数据）
	
	// 基于BTC和其他币种的价格变化估算占比变化
	change1h := btcData.PriceChange1h * 0.1   // 简化估算
	change4h := btcData.PriceChange4h * 0.1
	change24h := change4h * 2.0               // 估算24小时变化
	
	// 分析趋势 - 更量化的趋势判断
	trend := "stable"
	trendStrength := 50.0
	
	// 基于4小时价格变化判断趋势（更明确的阈值）
	if change4h > 1.0 {
		trend = "uptrend"  // BTC主导地位上升，山寨币相对走弱
		trendStrength = 55.0 + math.Min(change4h*8, 35)
	} else if change4h < -1.0 {
		trend = "downtrend" // BTC主导地位下降，山寨币相对走强
		trendStrength = 55.0 + math.Min(math.Abs(change4h)*8, 35)
	} else if math.Abs(change4h) >= 0.3 {
		// 轻微变化也要标注方向
		if change4h > 0 {
			trend = "weak_uptrend"
			trendStrength = 52.0 + change4h*10
		} else {
			trend = "weak_downtrend"
			trendStrength = 52.0 + math.Abs(change4h)*10
		}
	}
	
	// 估算支撑阻力位（基于技术分析的简化版本）
	nextResistance := current + 2.5
	nextSupport := current - 2.5
	
	return &BTCDominanceData{
		Current:        current,
		Change1h:       change1h,
		Change4h:       change4h,
		Change24h:      change24h,
		Trend:          trend,
		TrendStrength:  trendStrength,
		NextResistance: nextResistance,
		NextSupport:    nextSupport,
	}
}

// assessRisk 综合风险评估
func (mca *MarketContextAnalyzer) assessRisk(targetSymbol string, targetData, btcData *Data, correlation *CorrelationData) *MarketRiskData {
	// 计算BTC依赖度
	btcDependency := (math.Abs(correlation.BTCCorr1h) + math.Abs(correlation.BTCCorr4h) + math.Abs(correlation.BTCCorr24h)) / 3 * 100
	
	// 风险因子分解
	riskFactors := &RiskFactors{
		BTCDirectional:    btcDependency * 0.8,                    // BTC方向性风险
		BTCVolatility:     math.Abs(btcData.PriceChange4h) * 5,    // BTC波动性风险
		LiquidityRisk:     50.0,                                   // 默认流动性风险
		ConcentrationRisk: btcDependency * 0.6,                    // 集中度风险
		MacroRisk:         40.0,                                   // 默认宏观风险
		TechnicalRisk:     math.Abs(targetData.CurrentRSI7-50),    // 基于RSI的技术风险
	}
	
	// 系统性风险（主要来自BTC）
	systemicRisk := (riskFactors.BTCDirectional + riskFactors.BTCVolatility + riskFactors.MacroRisk) / 3
	
	// 个股特有风险
	idiosyncraticRisk := (riskFactors.LiquidityRisk + riskFactors.TechnicalRisk + correlation.DecouplingRisk) / 3
	
	// 综合风险评分
	riskScore := (systemicRisk + idiosyncraticRisk) / 2
	
	// 风险等级
	overallRisk := "low"
	if riskScore > 75 {
		overallRisk = "extreme"
	} else if riskScore > 60 {
		overallRisk = "high"
	} else if riskScore > 40 {
		overallRisk = "medium"
	}
	
	return &MarketRiskData{
		OverallRisk:       overallRisk,
		RiskScore:         riskScore,
		BTCDependency:     btcDependency,
		SystemicRisk:      systemicRisk,
		IdiosyncraticRisk: idiosyncraticRisk,
		RiskFactors:       riskFactors,
	}
}

// GetMarketRegime 判断当前市场状态
func (mca *MarketContextAnalyzer) GetMarketRegime(btcData *Data, marketContext *MarketContextData) MarketRegime {
	if marketContext == nil || marketContext.BTCDominance == nil || marketContext.Correlation == nil {
		return RegimeCrabMarket
	}
	
	btcChange4h := btcData.PriceChange4h
	dominanceTrend := marketContext.BTCDominance.Trend
	avgCorrelation := (math.Abs(marketContext.Correlation.BTCCorr1h) + 
					 math.Abs(marketContext.Correlation.BTCCorr4h) + 
					 math.Abs(marketContext.Correlation.BTCCorr24h)) / 3
	
	// 判断市场状态 - 更新趋势匹配逻辑
	if btcChange4h < -5 && avgCorrelation > 0.7 {
		return RegimePanicMode // 恐慌模式：BTC大跌且高相关性
	} else if btcChange4h > 3 && (dominanceTrend == "uptrend" || dominanceTrend == "weak_uptrend") {
		return RegimeBTCRally // BTC主导上涨
	} else if btcChange4h < -2 && (dominanceTrend == "downtrend" || dominanceTrend == "weak_downtrend") {
		return RegimeBTCDump // BTC主导下跌
	} else if (dominanceTrend == "downtrend" || dominanceTrend == "weak_downtrend") && avgCorrelation < 0.4 {
		return RegimeAltSeason // 山寨币季节：BTC占比下降且低相关性
	} else if avgCorrelation < 0.3 {
		return RegimeDecoupling // 脱钩行情
	} else {
		return RegimeCrabMarket // 横盘整理
	}
}