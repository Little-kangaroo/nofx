package microstructure

import (
	"log"
	"math"
	"sync"
	"time"
)

// CVDComparison CVD对比分析结果（V2.0）
type CVDComparison struct {
	Symbol               string    `json:"symbol"`
	SpotCVD              float64   `json:"spot_cvd"`              // 现货CVD
	FuturesCVD           float64   `json:"futures_cvd"`           // 期货CVD
	Ratio                float64   `json:"ratio"`                 // 现货/期货比例
	Divergence           float64   `json:"divergence"`            // 背离度(-1到1)
	Correlation          float64   `json:"correlation"`           // 相关性(-1到1)
	SpotDominance        float64   `json:"spot_dominance"`        // 现货主导度(0-1)
	FuturesDominance     float64   `json:"futures_dominance"`     // 期货主导度(0-1)
	MarketLeader         string    `json:"market_leader"`         // 市场领导者 
	TrendAlignment       string    `json:"trend_alignment"`       // 趋势一致性
	ArbitrageOpportunity string    `json:"arbitrage_opportunity"` // 套利机会
	Timestamp            time.Time `json:"timestamp"`
}

// CVDAnalysisMetrics 分析指标（内部使用）
type CVDAnalysisMetrics struct {
	SpotStrength     float64 `json:"spot_strength"`
	FuturesStrength  float64 `json:"futures_strength"`
	VelocityRatio    float64 `json:"velocity_ratio"`    // 速度比（哪个市场反应更快）
	VolumeRatio      float64 `json:"volume_ratio"`      // 成交量比
	PersistenceRatio float64 `json:"persistence_ratio"` // 持续性比（哪个更稳定）
}

// CVDComparisonAnalyzer CVD对比分析器
type CVDComparisonAnalyzer struct {
	mu                sync.RWMutex
	symbol            string
	historyWindow     time.Duration // 历史数据窗口（默认4小时）
	comparisonHistory []CVDComparison // 对比历史
	maxHistorySize    int // 最大历史记录数
}

// NewCVDComparisonAnalyzer 创建CVD对比分析器
func NewCVDComparisonAnalyzer(symbol string) *CVDComparisonAnalyzer {
	return &CVDComparisonAnalyzer{
		symbol:            symbol,
		historyWindow:     4 * time.Hour,
		comparisonHistory: make([]CVDComparison, 0, 100),
		maxHistorySize:    100,
	}
}

// AnalyzeSpotFuturesCVD 执行现货期货CVD对比分析
func (analyzer *CVDComparisonAnalyzer) AnalyzeSpotFuturesCVD(spotCVD, futuresCVD float64) *CVDComparison {
	analyzer.mu.Lock()
	defer analyzer.mu.Unlock()
	
	// 基础计算
	ratio := analyzer.calculateRatio(spotCVD, futuresCVD)
	divergence := analyzer.calculateDivergence(spotCVD, futuresCVD)
	correlation := analyzer.calculateCorrelation()
	
	// 主导度分析
	spotDominance, futuresDominance := analyzer.calculateDominance(spotCVD, futuresCVD)
	
	// 市场领导者识别
	marketLeader := analyzer.identifyMarketLeader(spotCVD, futuresCVD, spotDominance, futuresDominance)
	
	// 趋势一致性
	trendAlignment := analyzer.analyzeTrendAlignment(spotCVD, futuresCVD)
	
	// 套利机会评估
	arbitrageOpportunity := analyzer.evaluateArbitrageOpportunity(ratio, divergence, correlation)
	
	// 创建对比结果
	comparison := CVDComparison{
		Symbol:               analyzer.symbol,
		SpotCVD:              spotCVD,
		FuturesCVD:           futuresCVD,
		Ratio:                ratio,
		Divergence:           divergence,
		Correlation:          correlation,
		SpotDominance:        spotDominance,
		FuturesDominance:     futuresDominance,
		MarketLeader:         marketLeader,
		TrendAlignment:       trendAlignment,
		ArbitrageOpportunity: arbitrageOpportunity,
		Timestamp:            time.Now(),
	}
	
	// 添加到历史记录
	analyzer.addToHistory(comparison)
	
	log.Printf("📊 [%s] CVD对比分析: 现货主导%.2f, 期货主导%.2f, 市场领导者:%s, 背离度%.3f",
		analyzer.symbol, spotDominance, futuresDominance, marketLeader, divergence)
	
	return &comparison
}

// calculateRatio 计算现货期货CVD比例
func (analyzer *CVDComparisonAnalyzer) calculateRatio(spotCVD, futuresCVD float64) float64 {
	if math.Abs(futuresCVD) < 1000 { // 避免除零错误
		if math.Abs(spotCVD) < 1000 {
			return 1.0 // 两者都很小，认为平衡
		}
		return math.Inf(1) // 期货CVD接近零，现货主导
	}
	
	ratio := spotCVD / futuresCVD
	
	// 限制比例在合理范围内
	if ratio > 10 {
		return 10
	} else if ratio < -10 {
		return -10
	}
	
	return ratio
}

// calculateDivergence 计算背离度
func (analyzer *CVDComparisonAnalyzer) calculateDivergence(spotCVD, futuresCVD float64) float64 {
	// 标准化两个CVD值
	totalCVD := math.Abs(spotCVD) + math.Abs(futuresCVD)
	if totalCVD < 1000 { // 总量太小，无明显背离
		return 0
	}
	
	// 计算方向一致性
	if (spotCVD > 0 && futuresCVD > 0) || (spotCVD < 0 && futuresCVD < 0) {
		// 方向一致，计算强度差异
		stronger := math.Max(math.Abs(spotCVD), math.Abs(futuresCVD))
		weaker := math.Min(math.Abs(spotCVD), math.Abs(futuresCVD))
		
		if stronger == 0 {
			return 0
		}
		
		// 返回较小的分歧度（方向一致时）
		return -(1.0 - weaker/stronger) * 0.5 // 负值表示一致性
	} else {
		// 方向相反，计算背离强度
		divergenceStrength := math.Abs(spotCVD-futuresCVD) / totalCVD
		return divergenceStrength // 正值表示背离
	}
}

// calculateCorrelation 计算历史相关性
func (analyzer *CVDComparisonAnalyzer) calculateCorrelation() float64 {
	if len(analyzer.comparisonHistory) < 10 {
		return 0 // 数据不足
	}
	
	// 使用最近10个数据点计算相关性
	start := len(analyzer.comparisonHistory) - 10
	if start < 0 {
		start = 0
	}
	
	var spotValues, futuresValues []float64
	for i := start; i < len(analyzer.comparisonHistory); i++ {
		h := analyzer.comparisonHistory[i]
		spotValues = append(spotValues, h.SpotCVD)
		futuresValues = append(futuresValues, h.FuturesCVD)
	}
	
	return calculatePearsonCorrelation(spotValues, futuresValues)
}

// calculateDominance 计算市场主导度
func (analyzer *CVDComparisonAnalyzer) calculateDominance(spotCVD, futuresCVD float64) (float64, float64) {
	totalAbsCVD := math.Abs(spotCVD) + math.Abs(futuresCVD)
	
	if totalAbsCVD < 1000 { // 总量太小，认为均衡
		return 0.5, 0.5
	}
	
	spotDominance := math.Abs(spotCVD) / totalAbsCVD
	futuresDominance := math.Abs(futuresCVD) / totalAbsCVD
	
	// 考虑方向因素，同向加权，反向降权
	if (spotCVD > 0 && futuresCVD > 0) || (spotCVD < 0 && futuresCVD < 0) {
		// 同向时，强度大的获得额外权重
		if math.Abs(spotCVD) > math.Abs(futuresCVD) {
			spotDominance += 0.1
			futuresDominance -= 0.1
		} else {
			futuresDominance += 0.1
			spotDominance -= 0.1
		}
	}
	
	// 确保值在0-1范围内
	spotDominance = math.Max(0, math.Min(1, spotDominance))
	futuresDominance = math.Max(0, math.Min(1, futuresDominance))
	
	return spotDominance, futuresDominance
}

// identifyMarketLeader 识别市场领导者
func (analyzer *CVDComparisonAnalyzer) identifyMarketLeader(spotCVD, futuresCVD, spotDominance, futuresDominance float64) string {
	// 阈值：需要明显的主导优势
	dominanceThreshold := 0.65
	
	if spotDominance > dominanceThreshold {
		return "spot_leading"
	} else if futuresDominance > dominanceThreshold {
		return "futures_leading"
	}
	
	// 检查历史趋势
	if len(analyzer.comparisonHistory) >= 5 {
		recent := analyzer.comparisonHistory[len(analyzer.comparisonHistory)-5:]
		spotLeadCount := 0
		futuresLeadCount := 0
		
		for _, h := range recent {
			if h.SpotDominance > h.FuturesDominance {
				spotLeadCount++
			} else {
				futuresLeadCount++
			}
		}
		
		if spotLeadCount >= 4 {
			return "spot_trend_leading"
		} else if futuresLeadCount >= 4 {
			return "futures_trend_leading"
		}
	}
	
	return "balanced"
}

// analyzeTrendAlignment 分析趋势一致性
func (analyzer *CVDComparisonAnalyzer) analyzeTrendAlignment(spotCVD, futuresCVD float64) string {
	threshold := 50000.0 // 5万USD为有效阈值
	
	// 强烈一致性
	if spotCVD > threshold && futuresCVD > threshold {
		return "strong_bullish_alignment"
	} else if spotCVD < -threshold && futuresCVD < -threshold {
		return "strong_bearish_alignment"
	}
	
	// 弱一致性
	if (spotCVD > 0 && futuresCVD > 0) || (spotCVD < 0 && futuresCVD < 0) {
		return "weak_alignment"
	}
	
	// 背离
	if (spotCVD > threshold && futuresCVD < -threshold) || (spotCVD < -threshold && futuresCVD > threshold) {
		return "strong_divergence"
	}
	
	// 混合信号
	if math.Abs(spotCVD) > threshold || math.Abs(futuresCVD) > threshold {
		return "mixed_signals"
	}
	
	return "neutral"
}

// evaluateArbitrageOpportunity 评估套利机会
func (analyzer *CVDComparisonAnalyzer) evaluateArbitrageOpportunity(ratio, divergence, correlation float64) string {
	// 强背离 + 低相关性 = 套利机会
	if divergence > 0.6 && math.Abs(correlation) < 0.3 {
		return "high_arbitrage_potential"
	}
	
	// 适中背离 + 历史相关性破裂
	if divergence > 0.4 && correlation < 0.2 {
		return "medium_arbitrage_potential"
	}
	
	// 比例极端但背离不明显 = 潜在回归机会
	if math.Abs(ratio) > 5 && divergence < 0.3 {
		return "mean_reversion_opportunity"
	}
	
	// 高相关性 + 低背离 = 无套利机会
	if math.Abs(correlation) > 0.7 && divergence < 0.2 {
		return "no_arbitrage_opportunity"
	}
	
	return "low_arbitrage_potential"
}

// addToHistory 添加到历史记录
func (analyzer *CVDComparisonAnalyzer) addToHistory(comparison CVDComparison) {
	analyzer.comparisonHistory = append(analyzer.comparisonHistory, comparison)
	
	// 限制历史记录大小
	if len(analyzer.comparisonHistory) > analyzer.maxHistorySize {
		analyzer.comparisonHistory = analyzer.comparisonHistory[len(analyzer.comparisonHistory)-analyzer.maxHistorySize:]
	}
}

// GetHistoricalTrend 获取历史趋势分析
func (analyzer *CVDComparisonAnalyzer) GetHistoricalTrend(period time.Duration) *CVDTrendAnalysis {
	analyzer.mu.RLock()
	defer analyzer.mu.RUnlock()
	
	cutoffTime := time.Now().Add(-period)
	var recentHistory []CVDComparison
	
	for _, h := range analyzer.comparisonHistory {
		if h.Timestamp.After(cutoffTime) {
			recentHistory = append(recentHistory, h)
		}
	}
	
	return analyzer.analyzeTrend(recentHistory)
}

// CVDTrendAnalysis 趋势分析结果
type CVDTrendAnalysis struct {
	AverageSpotDominance    float64 `json:"avg_spot_dominance"`
	AverageFuturesDominance float64 `json:"avg_futures_dominance"`
	TrendConsistency        float64 `json:"trend_consistency"`
	LeadershipStability     string  `json:"leadership_stability"`
	CorrelationTrend        string  `json:"correlation_trend"`
	DivergenceFrequency     float64 `json:"divergence_frequency"`
}

// analyzeTrend 分析历史趋势
func (analyzer *CVDComparisonAnalyzer) analyzeTrend(history []CVDComparison) *CVDTrendAnalysis {
	if len(history) == 0 {
		return &CVDTrendAnalysis{}
	}
	
	var sumSpotDom, sumFuturesDom, sumCorrelation, divergenceCount float64
	leadershipChanges := 0
	lastLeader := ""
	
	for i, h := range history {
		sumSpotDom += h.SpotDominance
		sumFuturesDom += h.FuturesDominance
		sumCorrelation += h.Correlation
		
		if h.Divergence > 0.3 {
			divergenceCount++
		}
		
		if i > 0 && h.MarketLeader != lastLeader {
			leadershipChanges++
		}
		lastLeader = h.MarketLeader
	}
	
	count := float64(len(history))
	
	// 领导力稳定性
	var leadershipStability string
	changeRate := float64(leadershipChanges) / count
	if changeRate < 0.2 {
		leadershipStability = "very_stable"
	} else if changeRate < 0.4 {
		leadershipStability = "stable"
	} else if changeRate < 0.6 {
		leadershipStability = "volatile"
	} else {
		leadershipStability = "very_volatile"
	}
	
	// 相关性趋势
	avgCorrelation := sumCorrelation / count
	var correlationTrend string
	if avgCorrelation > 0.7 {
		correlationTrend = "highly_correlated"
	} else if avgCorrelation > 0.3 {
		correlationTrend = "moderately_correlated"
	} else if avgCorrelation > -0.3 {
		correlationTrend = "uncorrelated"
	} else {
		correlationTrend = "negatively_correlated"
	}
	
	return &CVDTrendAnalysis{
		AverageSpotDominance:    sumSpotDom / count,
		AverageFuturesDominance: sumFuturesDom / count,
		TrendConsistency:        1.0 - changeRate, // 一致性 = 1 - 变化率
		LeadershipStability:     leadershipStability,
		CorrelationTrend:        correlationTrend,
		DivergenceFrequency:     divergenceCount / count,
	}
}

// calculatePearsonCorrelation 计算皮尔逊相关系数
func calculatePearsonCorrelation(x, y []float64) float64 {
	if len(x) != len(y) || len(x) < 2 {
		return 0
	}
	
	// 计算均值
	var sumX, sumY float64
	n := float64(len(x))
	
	for i := 0; i < len(x); i++ {
		sumX += x[i]
		sumY += y[i]
	}
	
	meanX := sumX / n
	meanY := sumY / n
	
	// 计算协方差和方差
	var covariance, varianceX, varianceY float64
	
	for i := 0; i < len(x); i++ {
		diffX := x[i] - meanX
		diffY := y[i] - meanY
		
		covariance += diffX * diffY
		varianceX += diffX * diffX
		varianceY += diffY * diffY
	}
	
	// 防止除零错误
	if varianceX == 0 || varianceY == 0 {
		return 0
	}
	
	correlation := covariance / math.Sqrt(varianceX*varianceY)
	
	// 限制在[-1, 1]范围内
	if correlation > 1 {
		return 1
	} else if correlation < -1 {
		return -1
	}
	
	return correlation
}

// ===== CVD对比管理器 =====

// CVDComparisonManager CVD对比分析管理器
type CVDComparisonManager struct {
	analyzers map[string]*CVDComparisonAnalyzer // symbol -> analyzer
	mu        sync.RWMutex
}

// NewCVDComparisonManager 创建CVD对比分析管理器
func NewCVDComparisonManager() *CVDComparisonManager {
	return &CVDComparisonManager{
		analyzers: make(map[string]*CVDComparisonAnalyzer),
	}
}

// GetOrCreateAnalyzer 获取或创建分析器
func (manager *CVDComparisonManager) GetOrCreateAnalyzer(symbol string) *CVDComparisonAnalyzer {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	
	if analyzer, exists := manager.analyzers[symbol]; exists {
		return analyzer
	}
	
	analyzer := NewCVDComparisonAnalyzer(symbol)
	manager.analyzers[symbol] = analyzer
	
	log.Printf("✨ 创建CVD对比分析器: %s", symbol)
	return analyzer
}

// AnalyzeCVDComparison 执行CVD对比分析
func (manager *CVDComparisonManager) AnalyzeCVDComparison(symbol string, spotCVD, futuresCVD float64) *CVDComparison {
	analyzer := manager.GetOrCreateAnalyzer(symbol)
	return analyzer.AnalyzeSpotFuturesCVD(spotCVD, futuresCVD)
}