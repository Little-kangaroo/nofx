package market

// ApplyPrecisionFormatting 应用精度格式化到综合分析结果
func (ca *ComprehensiveAnalyzer) ApplyPrecisionFormatting(result *ComprehensiveResult, symbol string) {
	if result == nil {
		return
	}

	// 格式化基础价格数据
	result.CurrentPrice = FormatByDataTypeAndSymbol(result.CurrentPrice, "price", symbol)

	// 格式化道氏理论数据
	if result.DowTheory != nil && result.DowTheory.TrendStrength != nil {
		ts := result.DowTheory.TrendStrength
		FormatFloat64Pointer(&ts.Overall, "strength", symbol)
		FormatFloat64Pointer(&ts.ShortTerm, "strength", symbol)
		FormatFloat64Pointer(&ts.LongTerm, "strength", symbol)
		FormatFloat64Pointer(&ts.Momentum, "strength", symbol)
		FormatFloat64Pointer(&ts.Consistency, "confidence", symbol)
		FormatFloat64Pointer(&ts.VolumeSupport, "confidence", symbol)
	}

	// 格式化供需区数据
	if result.SupplyDemand != nil {
		formatSupplyDemandZones(result.SupplyDemand.SupplyZones, symbol)
		formatSupplyDemandZones(result.SupplyDemand.DemandZones, symbol)
		formatSupplyDemandZones(result.SupplyDemand.ActiveZones, symbol)
	}

	// 格式化VPVR数据
	if result.VolumeProfile != nil {
		if result.VolumeProfile.POC != nil {
			result.VolumeProfile.POC.Price = FormatByDataTypeAndSymbol(result.VolumeProfile.POC.Price, "price", symbol)
		}
		result.VolumeProfile.VAH = FormatByDataTypeAndSymbol(result.VolumeProfile.VAH, "price", symbol)
		result.VolumeProfile.VAL = FormatByDataTypeAndSymbol(result.VolumeProfile.VAL, "price", symbol)
	}

	// 格式化斐波纳契数据
	if result.Fibonacci != nil {
		for _, retracement := range result.Fibonacci.Retracements {
			for i, level := range retracement.Levels {
				level.Price = FormatByDataTypeAndSymbol(level.Price, "price", symbol)
				retracement.Levels[i] = level
			}
			retracement.Strength = FormatByDataTypeAndSymbol(retracement.Strength, "strength", symbol)
		}
	}

	// 格式化FVG数据
	if result.FairValueGaps != nil {
		formatFVGList(result.FairValueGaps.BullishFVGs, symbol)
		formatFVGList(result.FairValueGaps.BearishFVGs, symbol)
		formatFVGList(result.FairValueGaps.ActiveFVGs, symbol)
	}

	// 格式化统一信号数据
	for _, signal := range result.UnifiedSignals {
		if signal != nil {
			signal.Entry = FormatByDataTypeAndSymbol(signal.Entry, "price", symbol)
			signal.StopLoss = FormatByDataTypeAndSymbol(signal.StopLoss, "price", symbol)
			signal.TakeProfit = FormatByDataTypeAndSymbol(signal.TakeProfit, "price", symbol)
			signal.Confidence = FormatByDataTypeAndSymbol(signal.Confidence, "confidence", symbol)
			signal.Strength = FormatByDataTypeAndSymbol(signal.Strength, "strength", symbol)
		}
	}

	// 格式化市场结构数据
	if result.MarketStructure != nil {
		result.MarketStructure.TrendStrength = FormatByDataTypeAndSymbol(result.MarketStructure.TrendStrength, "strength", symbol)
		result.MarketStructure.Volatility = FormatByDataTypeAndSymbol(result.MarketStructure.Volatility, "strength", symbol)

		for i, level := range result.MarketStructure.SupportLevels {
			result.MarketStructure.SupportLevels[i] = FormatByDataTypeAndSymbol(level, "price", symbol)
		}
		for i, level := range result.MarketStructure.ResistanceLevels {
			result.MarketStructure.ResistanceLevels[i] = FormatByDataTypeAndSymbol(level, "price", symbol)
		}

		for i, keyLevel := range result.MarketStructure.KeyLevels {
			keyLevel.Price = FormatByDataTypeAndSymbol(keyLevel.Price, "price", symbol)
			keyLevel.Strength = FormatByDataTypeAndSymbol(keyLevel.Strength, "strength", symbol)
			result.MarketStructure.KeyLevels[i] = keyLevel
		}
	}

	// 格式化风险评估数据
	if result.RiskAssessment != nil {
		result.RiskAssessment.RecommendedRisk = FormatByDataTypeAndSymbol(result.RiskAssessment.RecommendedRisk, "ratio", symbol)
		result.RiskAssessment.MaxPositionSize = FormatByDataTypeAndSymbol(result.RiskAssessment.MaxPositionSize, "ratio", symbol)
	}

	// 格式化交易建议数据
	if result.TradingAdvice != nil {
		result.TradingAdvice.Confidence = FormatByDataTypeAndSymbol(result.TradingAdvice.Confidence, "confidence", symbol)
	}

	// 格式化供需区统计数据
	if result.SupplyDemand != nil && result.SupplyDemand.Statistics != nil {
		formatSDStatistics(result.SupplyDemand.Statistics, symbol)
	}

	// 格式化FVG统计数据
	if result.FairValueGaps != nil && result.FairValueGaps.Statistics != nil {
		formatFVGStatistics(result.FairValueGaps.Statistics, symbol)
	}

	// 格式化斐波纳契统计数据
	if result.Fibonacci != nil && result.Fibonacci.Statistics != nil {
		formatFibStatistics(result.Fibonacci.Statistics, symbol)
	}

	// 格式化支撑阻力统计数据
	if result.SupportResistance != nil && result.SupportResistance.Statistics != nil {
		formatSRStatistics(result.SupportResistance.Statistics, symbol)
	}

	// 格式化支撑阻力级别数据
	if result.SupportResistance != nil {
		formatSRLevels(result.SupportResistance.KeyLevels, symbol)
	}
}

// ApplyMultiTimeframePrecisionFormatting 应用精度格式化到多时间框架分析结果
func (ca *ComprehensiveAnalyzer) ApplyMultiTimeframePrecisionFormatting(analysis *MultiTimeframeAnalysis, symbol string) {
	if analysis == nil {
		return
	}

	// 格式化各时间框架的分析结果
	for _, tfAnalysis := range analysis.Timeframes {
		FormatTimeframeAnalysis(tfAnalysis, symbol)
	}

	// 格式化综合总结数据
	if analysis.Summary != nil {
		summary := analysis.Summary
		
		// 格式化置信度和一致性评分
		summary.TrendConsistency = FormatByDataTypeAndSymbol(summary.TrendConsistency, "confidence", symbol)
		summary.SignalConfidence = FormatByDataTypeAndSymbol(summary.SignalConfidence, "confidence", symbol)
		
		// 格式化关键价位
		if summary.KeyLevels != nil {
			for i, level := range summary.KeyLevels.SupportLevels {
				level.Price = FormatByDataTypeAndSymbol(level.Price, "price", symbol)
				level.Strength = FormatByDataTypeAndSymbol(level.Strength, "strength", symbol)
				level.Confidence = FormatByDataTypeAndSymbol(level.Confidence, "confidence", symbol)
				summary.KeyLevels.SupportLevels[i] = level
			}
			
			for i, level := range summary.KeyLevels.ResistanceLevels {
				level.Price = FormatByDataTypeAndSymbol(level.Price, "price", symbol)
				level.Strength = FormatByDataTypeAndSymbol(level.Strength, "strength", symbol)
				level.Confidence = FormatByDataTypeAndSymbol(level.Confidence, "confidence", symbol)
				summary.KeyLevels.ResistanceLevels[i] = level
			}
			
			for i, level := range summary.KeyLevels.PivotLevels {
				level.Price = FormatByDataTypeAndSymbol(level.Price, "price", symbol)
				level.Strength = FormatByDataTypeAndSymbol(level.Strength, "strength", symbol)
				level.Confidence = FormatByDataTypeAndSymbol(level.Confidence, "confidence", symbol)
				summary.KeyLevels.PivotLevels[i] = level
			}
		}
		
		// 格式化多时间框架交易信号
		for _, signal := range summary.TradingSignals {
			if signal != nil {
				signal.Confidence = FormatByDataTypeAndSymbol(signal.Confidence, "confidence", symbol)
				signal.EntryPrice = FormatByDataTypeAndSymbol(signal.EntryPrice, "price", symbol)
				signal.StopLoss = FormatByDataTypeAndSymbol(signal.StopLoss, "price", symbol)
				signal.RiskReward = FormatByDataTypeAndSymbol(signal.RiskReward, "ratio", symbol)
				
				for i, tp := range signal.TakeProfitLevels {
					signal.TakeProfitLevels[i] = FormatByDataTypeAndSymbol(tp, "price", symbol)
				}
			}
		}
		
		// 格式化风险评估
		if summary.RiskAssessment != nil {
			summary.RiskAssessment.RecommendedExposure = FormatByDataTypeAndSymbol(summary.RiskAssessment.RecommendedExposure, "ratio", symbol)
			summary.RiskAssessment.MaxPositionSize = FormatByDataTypeAndSymbol(summary.RiskAssessment.MaxPositionSize, "ratio", symbol)
		}
	}
}

// ApplyBasicDataPrecisionFormatting 应用精度格式化到基础数据
func ApplyBasicDataPrecisionFormatting(data *Data, symbol string) {
	if data == nil {
		return
	}

	// 使用precision_formatter.go中的函数格式化基础指标
	FormatBasicIndicators(data, symbol)
}

// formatSDStatistics 格式化供需区统计数据
func formatSDStatistics(stats *SDStatistics, symbol string) {
	if stats == nil {
		return
	}
	
	stats.AvgZoneStrength = FormatByDataTypeAndSymbol(stats.AvgZoneStrength, "strength", symbol)
	stats.AvgZoneWidth = FormatByDataTypeAndSymbol(stats.AvgZoneWidth, "price", symbol)
	stats.SuccessRate = FormatByDataTypeAndSymbol(stats.SuccessRate, "confidence", symbol)
	stats.BreakoutRate = FormatByDataTypeAndSymbol(stats.BreakoutRate, "confidence", symbol)
	stats.ReactionRate = FormatByDataTypeAndSymbol(stats.ReactionRate, "confidence", symbol)
}

// formatFVGStatistics 格式化FVG统计数据
func formatFVGStatistics(stats *FVGStatistics, symbol string) {
	if stats == nil {
		return
	}
	
	stats.AvgFVGWidth = FormatByDataTypeAndSymbol(stats.AvgFVGWidth, "price", symbol)
	stats.AvgFVGStrength = FormatByDataTypeAndSymbol(stats.AvgFVGStrength, "strength", symbol)
	stats.FillRate = FormatByDataTypeAndSymbol(stats.FillRate, "confidence", symbol)
	stats.SuccessRate = FormatByDataTypeAndSymbol(stats.SuccessRate, "confidence", symbol)
	stats.AvgFillTime = FormatByDataTypeAndSymbol(stats.AvgFillTime, "technical", symbol)
}

// formatFibStatistics 格式化斐波纳契统计数据
func formatFibStatistics(stats *FibStatistics, symbol string) {
	if stats == nil {
		return
	}
	
	stats.SuccessRate = FormatByDataTypeAndSymbol(stats.SuccessRate, "confidence", symbol)
	stats.AvgReactionTime = FormatByDataTypeAndSymbol(stats.AvgReactionTime, "technical", symbol)
	stats.AvgStrength = FormatByDataTypeAndSymbol(stats.AvgStrength, "strength", symbol)
}

// formatSRStatistics 格式化支撑阻力统计数据
func formatSRStatistics(stats *SRStatistics, symbol string) {
	if stats == nil {
		return
	}
	
	stats.AvgStrength = FormatByDataTypeAndSymbol(stats.AvgStrength, "strength", symbol)
	stats.AvgHitCount = FormatByDataTypeAndSymbol(stats.AvgHitCount, "technical", symbol)
}

// formatSRLevels 格式化支撑阻力级别列表
func formatSRLevels(levels []*SRLevel, symbol string) {
	for _, level := range levels {
		if level != nil {
			level.Price = FormatByDataTypeAndSymbol(level.Price, "price", symbol)
			level.Strength = FormatByDataTypeAndSymbol(level.Strength, "strength", symbol)
		}
	}
}