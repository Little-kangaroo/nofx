package market

// SupplyDemandContextScoring 供需区上下文评分计算器
type SupplyDemandContextScoring struct {
	analyzer *SupplyDemandAnalyzer
}

// NewSupplyDemandContextScoring 创建供需区上下文评分计算器
func NewSupplyDemandContextScoring(analyzer *SupplyDemandAnalyzer) *SupplyDemandContextScoring {
	return &SupplyDemandContextScoring{
		analyzer: analyzer,
	}
}

// CalculateContextScores 计算所有供需区的上下文评分
func (scs *SupplyDemandContextScoring) CalculateContextScores(allZones []*SupplyDemandZone, contextCalc *ContextCalculator) {
	if len(allZones) == 0 {
		return
	}

	// 收集所有强度值和宽度值用于计算Z分数
	var allStrengths []float64
	var allWidths []float64
	var allVolumes []float64
	
	for _, zone := range allZones {
		if zone != nil {
			allStrengths = append(allStrengths, zone.Strength)
			allWidths = append(allWidths, zone.Width)
			if zone.VolumeProfile != nil {
				allVolumes = append(allVolumes, zone.VolumeProfile.TotalVolume)
			}
		}
	}

	// 为每个供需区计算上下文评分
	for _, zone := range allZones {
		if zone == nil {
			continue
		}

		// 计算上下文评分
		zone.Context = scs.calculateSingleZoneContext(zone, allStrengths, allWidths, contextCalc)
	}
}

// calculateSingleZoneContext 计算单个供需区的上下文评分
func (scs *SupplyDemandContextScoring) calculateSingleZoneContext(zone *SupplyDemandZone, allStrengths []float64, allWidths []float64, contextCalc *ContextCalculator) *ContextMetrics {
	// 1. 计算强度标准分 (strength_z)
	// >1.5 为强，>2.0 为极强
	strengthZ := contextCalc.CalculateStrengthZ(zone.Strength, allStrengths)

	// 2. 计算宽度相对ATR的倍数 (width_atr)  
	// 表示区域宽度相对市场正常波动的大小
	// >1.0 表示区域超过正常日内波动
	widthATR := contextCalc.CalculateWidthATR(zone.Width)

	// 3. 计算成交量比率 (vol_ratio)
	// 表示供需区形成时的成交量相对平均成交量的倍数
	// >2.0 表示异常放量
	volRatio := 1.0
	if zone.VolumeProfile != nil {
		volRatio = contextCalc.CalculateVolumeRatio(zone.VolumeProfile.TotalVolume)
	}

	// 4. 判断是否新鲜 (is_fresh)
	// 新鲜的供需区通常具有更强的支撑/阻力效果
	maxAge := int64(scs.analyzer.config.MaxZoneAge * 3600 * 1000) // 转换为毫秒
	isFresh := contextCalc.IsFresh(zone.CreationTime, maxAge)

	// 5. 计算时间评分 (time_score) 
	// 时间衰减评分，越新鲜评分越高
	timeScore := contextCalc.CalculateTimeScore(zone.CreationTime, maxAge)

	// 6. 计算排名百分位 (rank_pct)
	// 在所有供需区中的强度排名，0.8+ 表示前20%
	rankPct := contextCalc.CalculateRankPercentile(zone.Strength, allStrengths)

	// 7. 计算基于ATR的波动率评级 (跨币种统一标准)
	zone.VolatilityGrade = contextCalc.CalculateVolatilityGrade(widthATR)

	return &ContextMetrics{
		StrengthZ: strengthZ,
		WidthATR:  widthATR,  
		VolRatio:  volRatio,
		IsFresh:   isFresh,
		TimeScore: timeScore,
		RankPct:   rankPct,
	}
}

// 为现有的供需区分析器添加扩展方法
func (sd *SupplyDemandAnalyzer) CalculateContextScores(allZones []*SupplyDemandZone, contextCalc *ContextCalculator) {
	scorer := NewSupplyDemandContextScoring(sd)
	scorer.CalculateContextScores(allZones, contextCalc)
}