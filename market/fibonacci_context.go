package market

// FibonacciContextScoring 斐波纳契上下文评分计算器
type FibonacciContextScoring struct {
	analyzer *FibonacciAnalyzer
}

// NewFibonacciContextScoring 创建斐波纳契上下文评分计算器
func NewFibonacciContextScoring(analyzer *FibonacciAnalyzer) *FibonacciContextScoring {
	return &FibonacciContextScoring{
		analyzer: analyzer,
	}
}

// CalculateContextScores 计算所有斐波纳契回调的上下文评分
func (fcs *FibonacciContextScoring) CalculateContextScores(allRetracements []*FibRetracement, contextCalc *ContextCalculator) {
	if len(allRetracements) == 0 {
		return
	}

	// 收集所有强度值用于计算Z分数
	var allStrengths []float64
	var allTrendLengths []float64
	
	for _, retracement := range allRetracements {
		if retracement != nil {
			allStrengths = append(allStrengths, retracement.Strength)
			// 计算趋势长度作为"宽度"的概念
			trendLength := calculateTrendLength(retracement.StartPoint, retracement.EndPoint)
			allTrendLengths = append(allTrendLengths, trendLength)
		}
	}

	// 为每个斐波纳契回调计算上下文评分
	for _, retracement := range allRetracements {
		if retracement == nil {
			continue
		}

		// 计算上下文评分
		retracement.Context = fcs.calculateSingleRetracementContext(retracement, allStrengths, allTrendLengths, contextCalc)
	}
}

// calculateSingleRetracementContext 计算单个斐波纳契回调的上下文评分
func (fcs *FibonacciContextScoring) calculateSingleRetracementContext(retracement *FibRetracement, allStrengths []float64, allTrendLengths []float64, contextCalc *ContextCalculator) *ContextMetrics {
	// 1. 计算强度标准分 (strength_z)
	// >1.5 为强，>2.0 为极强
	strengthZ := contextCalc.CalculateStrengthZ(retracement.Strength, allStrengths)

	// 2. 计算趋势长度相对ATR的倍数 (width_atr)
	// 对于斐波纳契，我们用趋势长度表示"宽度"概念
	trendLength := calculateTrendLength(retracement.StartPoint, retracement.EndPoint)
	widthATR := contextCalc.CalculateWidthATR(trendLength)

	// 3. 计算成交量比率 (vol_ratio)
	// 斐波纳契分析中，我们可以使用当前市场的成交量比率
	volRatio := 1.0 // 默认值，可以根据斐波纳契级别的触及成交量来计算

	// 4. 判断是否新鲜 (is_fresh)
	// 新鲜的斐波纳契回调通常具有更强的有效性
	maxAge := int64(fcs.analyzer.config.MaxRetracementAge * 3600 * 1000) // 转换为毫秒
	isFresh := contextCalc.IsFresh(retracement.CreatedAt, maxAge)

	// 5. 计算时间评分 (time_score) 
	// 时间衰减评分，越新鲜评分越高
	timeScore := contextCalc.CalculateTimeScore(retracement.CreatedAt, maxAge)

	// 6. 计算排名百分位 (rank_pct)
	// 在所有斐波纳契回调中的强度排名，0.8+ 表示前20%
	rankPct := contextCalc.CalculateRankPercentile(retracement.Strength, allStrengths)

	return &ContextMetrics{
		StrengthZ: strengthZ,
		WidthATR:  widthATR,  
		VolRatio:  volRatio,
		IsFresh:   isFresh,
		TimeScore: timeScore,
		RankPct:   rankPct,
	}
}

// calculateTrendLength 计算趋势长度
func calculateTrendLength(startPoint, endPoint PricePoint) float64 {
	if startPoint.Price == 0 {
		return 0
	}
	return (endPoint.Price - startPoint.Price) / startPoint.Price * 100 // 返回百分比
}

// 为现有的斐波纳契分析器添加扩展方法
func (fa *FibonacciAnalyzer) CalculateContextScores(allRetracements []*FibRetracement, contextCalc *ContextCalculator) {
	scorer := NewFibonacciContextScoring(fa)
	scorer.CalculateContextScores(allRetracements, contextCalc)
}