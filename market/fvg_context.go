package market

// FVGContextScoring FVG上下文评分计算器
type FVGContextScoring struct {
	analyzer *FVGAnalyzer
}

// NewFVGContextScoring 创建FVG上下文评分计算器
func NewFVGContextScoring(analyzer *FVGAnalyzer) *FVGContextScoring {
	return &FVGContextScoring{
		analyzer: analyzer,
	}
}

// CalculateContextScores 计算所有FVG的上下文评分
func (fcs *FVGContextScoring) CalculateContextScores(allFVGs []*FairValueGap, contextCalc *ContextCalculator) {
	if len(allFVGs) == 0 {
		return
	}

	// 收集所有强度值用于计算Z分数
	var allStrengths []float64
	var allWidths []float64
	var allVolumes []float64
	
	for _, gap := range allFVGs {
		if gap != nil {
			allStrengths = append(allStrengths, gap.Strength)
			allWidths = append(allWidths, gap.Width)
			if gap.VolumeContext != nil {
				allVolumes = append(allVolumes, gap.VolumeContext.FormationVolume)
			}
		}
	}

	// 为每个FVG计算上下文评分
	for _, gap := range allFVGs {
		if gap == nil {
			continue
		}

		// 计算上下文评分
		gap.Context = fcs.calculateSingleFVGContext(gap, allStrengths, allWidths, contextCalc)
	}
}

// calculateSingleFVGContext 计算单个FVG的上下文评分
func (fcs *FVGContextScoring) calculateSingleFVGContext(gap *FairValueGap, allStrengths []float64, allWidths []float64, contextCalc *ContextCalculator) *ContextMetrics {
	// 1. 计算强度标准分 (strength_z)
	// >1.5 为强，>2.0 为极强
	strengthZ := contextCalc.CalculateStrengthZ(gap.Strength, allStrengths)

	// 2. 计算宽度相对ATR的倍数 (width_atr)  
	// 表示缺口宽度相对市场正常波动的大小
	// >1.0 表示缺口超过正常日内波动
	widthATR := contextCalc.CalculateWidthATR(gap.Width)

	// 3. 计算成交量比率 (vol_ratio)
	// 表示形成FVG时的成交量相对平均成交量的倍数
	// >2.0 表示异常放量
	volRatio := 1.0
	if gap.VolumeContext != nil {
		volRatio = contextCalc.CalculateVolumeRatio(gap.VolumeContext.FormationVolume)
	}

	// 4. 判断是否新鲜 (is_fresh)
	// 新鲜的FVG通常具有更强的支撑/阻力效果
	maxAge := int64(fcs.analyzer.config.MaxAge * 3600 * 1000) // 转换为毫秒 
	isFresh := contextCalc.IsFresh(gap.CreationTime, maxAge)

	// 5. 计算时间评分 (time_score) 
	// 时间衰减评分，越新鲜评分越高
	timeScore := contextCalc.CalculateTimeScore(gap.CreationTime, maxAge)

	// 6. 计算排名百分位 (rank_pct)
	// 在所有FVG中的强度排名，0.8+ 表示前20%
	rankPct := contextCalc.CalculateRankPercentile(gap.Strength, allStrengths)

	return &ContextMetrics{
		StrengthZ: strengthZ,
		WidthATR:  widthATR,  
		VolRatio:  volRatio,
		IsFresh:   isFresh,
		TimeScore: timeScore,
		RankPct:   rankPct,
	}
}

// 为现有的FVG分析器添加扩展方法
func (fvg *FVGAnalyzer) CalculateContextScores(allFVGs []*FairValueGap, contextCalc *ContextCalculator) {
	scorer := NewFVGContextScoring(fvg)
	scorer.CalculateContextScores(allFVGs, contextCalc)
}