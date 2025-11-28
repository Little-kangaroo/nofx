package market

import (
	"math"
)

// PrecisionConfig 精度配置
type PrecisionConfig struct {
	Price      int // 价格精度
	Percentage int // 百分比精度  
	Volume     int // 成交量精度
}

// 交易币种精度映射表
var TradingSymbolPrecision = map[string]PrecisionConfig{
	"BTCUSDT": {
		Price:      2, // 91092.00
		Percentage: 2, // +0.30%
		Volume:     0, // 2197 (整数)
	},
	"ETHUSDT": {
		Price:      2, // 3024.50
		Percentage: 2, // +1.25%
		Volume:     0, // 1543
	},
	"BNBUSDT": {
		Price:      2, // 612.30
		Percentage: 2, // +0.85%
		Volume:     0, // 892
	},
	"SOLUSDT": {
		Price:      2, // 185.50
		Percentage: 2, // +2.14%
		Volume:     0, // 1205
	},
	"XRPUSDT": {
		Price:      4, // 1.2345
		Percentage: 2, // +3.15%
		Volume:     0, // 25641
	},
	"ADAUSDT": {
		Price:      4, // 0.7542
		Percentage: 2, // +1.89%
		Volume:     0, // 15234
	},
	"DOGEUSDT": {
		Price:      5, // 0.38652
		Percentage: 2, // +4.25%
		Volume:     0, // 45123
	},
	"HYPEUSDT": {
		Price:      4, // 暂定4位，可根据实际价格调整
		Percentage: 2, // +2.35%
		Volume:     0, // 8921
	},
}

// FormatByDataTypeAndSymbol 根据数据类型和币种格式化数值
func FormatByDataTypeAndSymbol(value float64, dataType string, symbol string) float64 {
	var precision int
	
	// 获取币种精度配置，如果没有配置则使用默认值
	config, exists := TradingSymbolPrecision[symbol]
	if !exists {
		// 默认配置：基于价格大小自动判断
		if value >= 1000 {
			config = PrecisionConfig{Price: 2, Percentage: 2, Volume: 0}
		} else if value >= 1 {
			config = PrecisionConfig{Price: 2, Percentage: 2, Volume: 0}
		} else if value >= 0.01 {
			config = PrecisionConfig{Price: 4, Percentage: 2, Volume: 0}
		} else {
			config = PrecisionConfig{Price: 5, Percentage: 2, Volume: 0}
		}
	}
	
	switch dataType {
	case "price":
		precision = config.Price
	case "percentage", "change":
		precision = config.Percentage
	case "rsi", "strength", "confidence":
		precision = 1 // 指标统一1位: 55.4
	case "volume":
		if value >= 1000 {
			precision = 0 // 大成交量取整: 2197
		} else {
			precision = 3 // 小成交量3位: 3.861
		}
	case "slope", "ratio":
		precision = 4 // 斜率和比率保留4位
	case "atr", "technical":
		// 技术指标精度：基于数值大小统一规则
		if value < 1.0 {
			precision = 5 // 小于1的保留5位: 0.38543
		} else {
			precision = 2 // 大于等于1的保留2位: 91.23
		}
	default:
		precision = 2 // 默认2位
	}
	
	if precision == 0 {
		return math.Round(value)
	}
	
	multiplier := math.Pow(10, float64(precision))
	return math.Round(value*multiplier) / multiplier
}

// FormatFloat64Pointer 格式化float64指针
func FormatFloat64Pointer(ptr *float64, dataType string, symbol string) {
	if ptr != nil {
		*ptr = FormatByDataTypeAndSymbol(*ptr, dataType, symbol)
	}
}

// FormatTimeframeAnalysis 格式化时间框架分析数据的精度
func FormatTimeframeAnalysis(tfAnalysis *TimeframeAnalysis, symbol string) {
	if tfAnalysis == nil {
		return
	}

	// 格式化可靠性
	tfAnalysis.Reliability = FormatByDataTypeAndSymbol(tfAnalysis.Reliability, "confidence", symbol)

	// 格式化道氏理论数据
	if tfAnalysis.DowTheory != nil && tfAnalysis.DowTheory.TrendStrength != nil {
		ts := tfAnalysis.DowTheory.TrendStrength
		FormatFloat64Pointer(&ts.Overall, "strength", symbol)
		FormatFloat64Pointer(&ts.ShortTerm, "strength", symbol)
		FormatFloat64Pointer(&ts.LongTerm, "strength", symbol)
		FormatFloat64Pointer(&ts.Momentum, "strength", symbol)
		FormatFloat64Pointer(&ts.Consistency, "confidence", symbol)
		FormatFloat64Pointer(&ts.VolumeSupport, "confidence", symbol)
	}

	// 格式化供需区数据
	if tfAnalysis.SupplyDemand != nil {
		formatSupplyDemandZones(tfAnalysis.SupplyDemand.SupplyZones, symbol)
		formatSupplyDemandZones(tfAnalysis.SupplyDemand.DemandZones, symbol)
		formatSupplyDemandZones(tfAnalysis.SupplyDemand.ActiveZones, symbol)
	}

	// 格式化VPVR数据
	if tfAnalysis.VolumeProfile != nil {
		if tfAnalysis.VolumeProfile.POC != nil {
			tfAnalysis.VolumeProfile.POC.Price = FormatByDataTypeAndSymbol(tfAnalysis.VolumeProfile.POC.Price, "price", symbol)
		}
		tfAnalysis.VolumeProfile.VAH = FormatByDataTypeAndSymbol(tfAnalysis.VolumeProfile.VAH, "price", symbol)
		tfAnalysis.VolumeProfile.VAL = FormatByDataTypeAndSymbol(tfAnalysis.VolumeProfile.VAL, "price", symbol)
	}

	// 格式化斐波纳契数据
	if tfAnalysis.Fibonacci != nil {
		for _, retracement := range tfAnalysis.Fibonacci.Retracements {
			for i, level := range retracement.Levels {
				level.Price = FormatByDataTypeAndSymbol(level.Price, "price", symbol)
				retracement.Levels[i] = level
			}
			retracement.Strength = FormatByDataTypeAndSymbol(retracement.Strength, "strength", symbol)
		}
	}

	// 格式化FVG数据
	if tfAnalysis.FairValueGaps != nil {
		formatFVGList(tfAnalysis.FairValueGaps.BullishFVGs, symbol)
		formatFVGList(tfAnalysis.FairValueGaps.BearishFVGs, symbol)
		formatFVGList(tfAnalysis.FairValueGaps.ActiveFVGs, symbol)
		
		// 格式化FVG统计数据
		if tfAnalysis.FairValueGaps.Statistics != nil {
			formatFVGStatisticsInFormatter(tfAnalysis.FairValueGaps.Statistics, symbol)
		}
	}

	// 格式化供需区统计数据
	if tfAnalysis.SupplyDemand != nil && tfAnalysis.SupplyDemand.Statistics != nil {
		formatSDStatisticsInFormatter(tfAnalysis.SupplyDemand.Statistics, symbol)
	}

	// 格式化斐波纳契统计数据  
	if tfAnalysis.Fibonacci != nil && tfAnalysis.Fibonacci.Statistics != nil {
		formatFibStatisticsInFormatter(tfAnalysis.Fibonacci.Statistics, symbol)
	}

	// 格式化支撑阻力统计数据
	if tfAnalysis.SupportResistance != nil && tfAnalysis.SupportResistance.Statistics != nil {
		formatSRStatisticsInFormatter(tfAnalysis.SupportResistance.Statistics, symbol)
	}

	// 格式化支撑阻力级别数据
	if tfAnalysis.SupportResistance != nil {
		formatSRLevelsInFormatter(tfAnalysis.SupportResistance.KeyLevels, symbol)
	}
}

// formatSupplyDemandZones 格式化供需区列表
func formatSupplyDemandZones(zones []*SupplyDemandZone, symbol string) {
	for _, zone := range zones {
		if zone != nil {
			zone.UpperBound = FormatByDataTypeAndSymbol(zone.UpperBound, "price", symbol)
			zone.LowerBound = FormatByDataTypeAndSymbol(zone.LowerBound, "price", symbol)
			zone.CenterPrice = FormatByDataTypeAndSymbol(zone.CenterPrice, "price", symbol)
			zone.Width = FormatByDataTypeAndSymbol(zone.Width, "price", symbol)
			zone.WidthPercent = FormatByDataTypeAndSymbol(zone.WidthPercent, "percentage", symbol)
			zone.Strength = FormatByDataTypeAndSymbol(zone.Strength, "strength", symbol)
			
			// 格式化上下文评分
			formatContextMetrics(zone.Context, symbol)
		}
	}
}

// formatFVGList 格式化FVG列表
func formatFVGList(fvgs []*FairValueGap, symbol string) {
	for _, fvg := range fvgs {
		if fvg != nil {
			fvg.UpperBound = FormatByDataTypeAndSymbol(fvg.UpperBound, "price", symbol)
			fvg.LowerBound = FormatByDataTypeAndSymbol(fvg.LowerBound, "price", symbol)
			fvg.CenterPrice = FormatByDataTypeAndSymbol(fvg.CenterPrice, "price", symbol)
			fvg.Width = FormatByDataTypeAndSymbol(fvg.Width, "price", symbol)
			fvg.WidthPercent = FormatByDataTypeAndSymbol(fvg.WidthPercent, "percentage", symbol)
			fvg.Strength = FormatByDataTypeAndSymbol(fvg.Strength, "strength", symbol)
			
			// 格式化上下文评分
			formatContextMetrics(fvg.Context, symbol)
		}
	}
}

// FormatBasicIndicators 格式化基础指标数据
func FormatBasicIndicators(data *Data, symbol string) {
	if data == nil {
		return
	}

	// 格式化主要价格数据
	data.CurrentPrice = FormatByDataTypeAndSymbol(data.CurrentPrice, "price", symbol)
	data.PriceChange1h = FormatByDataTypeAndSymbol(data.PriceChange1h, "percentage", symbol)
	data.PriceChange4h = FormatByDataTypeAndSymbol(data.PriceChange4h, "percentage", symbol)
	data.CurrentEMA20 = FormatByDataTypeAndSymbol(data.CurrentEMA20, "technical", symbol)
	data.CurrentMACD = FormatByDataTypeAndSymbol(data.CurrentMACD, "technical", symbol)
	data.CurrentRSI7 = FormatByDataTypeAndSymbol(data.CurrentRSI7, "rsi", symbol)
	data.FundingRate = FormatByDataTypeAndSymbol(data.FundingRate, "ratio", symbol)

	// 格式化开放利息数据
	if data.OpenInterest != nil {
		data.OpenInterest.Latest = FormatByDataTypeAndSymbol(data.OpenInterest.Latest, "volume", symbol)
		data.OpenInterest.Average = FormatByDataTypeAndSymbol(data.OpenInterest.Average, "volume", symbol)
	}

	// 格式化长期数据
	if data.LongerTermContext != nil {
		data.LongerTermContext.EMA20 = FormatByDataTypeAndSymbol(data.LongerTermContext.EMA20, "technical", symbol)
		data.LongerTermContext.EMA50 = FormatByDataTypeAndSymbol(data.LongerTermContext.EMA50, "technical", symbol)
		data.LongerTermContext.ATR3 = FormatByDataTypeAndSymbol(data.LongerTermContext.ATR3, "atr", symbol)
		data.LongerTermContext.ATR14 = FormatByDataTypeAndSymbol(data.LongerTermContext.ATR14, "atr", symbol)
		data.LongerTermContext.CurrentVolume = FormatByDataTypeAndSymbol(data.LongerTermContext.CurrentVolume, "volume", symbol)
		data.LongerTermContext.AverageVolume = FormatByDataTypeAndSymbol(data.LongerTermContext.AverageVolume, "volume", symbol)
	}

	// 格式化中期数据
	formatMediumTermData(data.MediumTerm15m, symbol)
	formatMediumTermData(data.MediumTerm30m, symbol)
	formatMediumTermData(data.MediumTerm1h, symbol)
}

// formatMediumTermData 格式化中期数据
func formatMediumTermData(data *MediumTermData, symbol string) {
	if data != nil {
		data.EMA20 = FormatByDataTypeAndSymbol(data.EMA20, "technical", symbol)
		data.EMA50 = FormatByDataTypeAndSymbol(data.EMA50, "technical", symbol)
		data.CurrentMACD = FormatByDataTypeAndSymbol(data.CurrentMACD, "technical", symbol)
		data.CurrentRSI7 = FormatByDataTypeAndSymbol(data.CurrentRSI7, "rsi", symbol)
		data.CurrentRSI14 = FormatByDataTypeAndSymbol(data.CurrentRSI14, "rsi", symbol)
		data.ATR14 = FormatByDataTypeAndSymbol(data.ATR14, "atr", symbol)
		data.CurrentVolume = FormatByDataTypeAndSymbol(data.CurrentVolume, "volume", symbol)
		data.AverageVolume = FormatByDataTypeAndSymbol(data.AverageVolume, "volume", symbol)
	}
}

// formatSDStatisticsInFormatter 格式化供需区统计数据（formatter文件中的实现）
func formatSDStatisticsInFormatter(stats *SDStatistics, symbol string) {
	if stats == nil {
		return
	}
	
	stats.AvgZoneStrength = FormatByDataTypeAndSymbol(stats.AvgZoneStrength, "strength", symbol)
	stats.AvgZoneWidth = FormatByDataTypeAndSymbol(stats.AvgZoneWidth, "price", symbol)
	stats.SuccessRate = FormatByDataTypeAndSymbol(stats.SuccessRate, "confidence", symbol)
	stats.BreakoutRate = FormatByDataTypeAndSymbol(stats.BreakoutRate, "confidence", symbol)
	stats.ReactionRate = FormatByDataTypeAndSymbol(stats.ReactionRate, "confidence", symbol)
}

// formatFVGStatisticsInFormatter 格式化FVG统计数据（formatter文件中的实现）
func formatFVGStatisticsInFormatter(stats *FVGStatistics, symbol string) {
	if stats == nil {
		return
	}
	
	stats.AvgFVGWidth = FormatByDataTypeAndSymbol(stats.AvgFVGWidth, "price", symbol)
	stats.AvgFVGStrength = FormatByDataTypeAndSymbol(stats.AvgFVGStrength, "strength", symbol)
	stats.FillRate = FormatByDataTypeAndSymbol(stats.FillRate, "confidence", symbol)
	stats.SuccessRate = FormatByDataTypeAndSymbol(stats.SuccessRate, "confidence", symbol)
	stats.AvgFillTime = FormatByDataTypeAndSymbol(stats.AvgFillTime, "technical", symbol)
}

// formatFibStatisticsInFormatter 格式化斐波纳契统计数据（formatter文件中的实现）
func formatFibStatisticsInFormatter(stats *FibStatistics, symbol string) {
	if stats == nil {
		return
	}
	
	stats.SuccessRate = FormatByDataTypeAndSymbol(stats.SuccessRate, "confidence", symbol)
	stats.AvgReactionTime = FormatByDataTypeAndSymbol(stats.AvgReactionTime, "technical", symbol)
	stats.AvgStrength = FormatByDataTypeAndSymbol(stats.AvgStrength, "strength", symbol)
}

// formatSRStatisticsInFormatter 格式化支撑阻力统计数据（formatter文件中的实现）
func formatSRStatisticsInFormatter(stats *SRStatistics, symbol string) {
	if stats == nil {
		return
	}
	
	stats.AvgStrength = FormatByDataTypeAndSymbol(stats.AvgStrength, "strength", symbol)
	stats.AvgHitCount = FormatByDataTypeAndSymbol(stats.AvgHitCount, "technical", symbol)
}

// formatSRLevelsInFormatter 格式化支撑阻力级别列表（formatter文件中的实现）
func formatSRLevelsInFormatter(levels []*SRLevel, symbol string) {
	for _, level := range levels {
		if level != nil {
			level.Price = FormatByDataTypeAndSymbol(level.Price, "price", symbol)
			level.Strength = FormatByDataTypeAndSymbol(level.Strength, "strength", symbol)
		}
	}
}

// formatBasicIndicatorsData 格式化基础指标数据的精度
func formatBasicIndicatorsData(data map[string]interface{}, symbol string) {
	if data == nil {
		return
	}

	for _, tfData := range data {
		if tfDataMap, ok := tfData.(map[string]interface{}); ok {
			formatTimeframeBasicIndicators(tfDataMap, symbol)
		}
	}
}

// formatTimeframeBasicIndicators 格式化单个时间框架的基础指标
func formatTimeframeBasicIndicators(tfData map[string]interface{}, symbol string) {
	if tfData == nil {
		return
	}

	// 定义需要格式化的字段及其类型
	fieldFormats := map[string]string{
		// 价格和移动平均线
		"ema20": "technical", "ema50": "technical", "ema100": "technical", "ema200": "technical",
		"sma20": "technical", "sma50": "technical", "vwap": "technical",
		
		// 技术指标
		"atr14": "atr", "macd": "technical", "rsi14": "rsi", "rsi7": "rsi",
		
		// 斜率
		"ema50_slope_3": "slope", "ema200_slope_3": "slope",
		
		// 成交量
		"volume": "volume", "avg_volume": "volume",
		
		// 价格变化
		"change_1h": "percentage", "change_4h": "percentage",
		
		// 价格
		"price": "price", "last_price": "price",
		
		// 其他指标
		"funding_rate": "ratio", "oi_latest": "volume",
	}

	// 遍历并格式化所有字段
	for field, dataType := range fieldFormats {
		if value, exists := tfData[field]; exists {
			if floatValue, ok := value.(float64); ok {
				tfData[field] = FormatByDataTypeAndSymbol(floatValue, dataType, symbol)
			}
		}
	}
}

// formatContextMetrics 格式化上下文评分数据
func formatContextMetrics(ctx *ContextMetrics, symbol string) {
	if ctx == nil {
		return
	}
	
	ctx.StrengthZ = FormatByDataTypeAndSymbol(ctx.StrengthZ, "ratio", symbol)
	ctx.WidthATR = FormatByDataTypeAndSymbol(ctx.WidthATR, "ratio", symbol) 
	ctx.VolRatio = FormatByDataTypeAndSymbol(ctx.VolRatio, "ratio", symbol)
	ctx.TimeScore = FormatByDataTypeAndSymbol(ctx.TimeScore, "ratio", symbol)
	ctx.RankPct = FormatByDataTypeAndSymbol(ctx.RankPct, "ratio", symbol)
}