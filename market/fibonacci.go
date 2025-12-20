package market

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// FibonacciAnalyzer 斐波纳��分析器
type FibonacciAnalyzer struct {
	config FibonacciConfig
}

// NewFibonacciAnalyzer 创建新的斐波纳契分析器
func NewFibonacciAnalyzer(config ...FibonacciConfig) *FibonacciAnalyzer {
	conf := defaultFibonacciConfig
	if len(config) > 0 {
		conf = config[0]
	}
	
	return &FibonacciAnalyzer{
		config: conf,
	}
}

// Analyze 执行斐波纳契分析
func (fa *FibonacciAnalyzer) Analyze(klines []Kline) *FibonacciData {
	if len(klines) < 10 {
		return &FibonacciData{
			Config: fa.config,
		}
	}

	// 识别趋势和关键摆动点
	swingPoints := fa.identifySwingPoints(klines)

	// 计算斐波纳契回调
	retracements := fa.calculateRetracements(swingPoints, klines)

	// 🔥 Token优化：筛选回调中的关键水平线（只保留0.382, 0.5, 0.618, 0.786）
	for i := range retracements {
		retracements[i].Levels = filterKeyFibLevels(retracements[i].Levels)
	}

	// 计算斐波纳契扩展
	extensions := fa.calculateExtensions(swingPoints, klines)

	// 🔥 Token优化：筛选扩展中的关键水平线（只保留0.382, 0.5, 0.618, 0.786）
	for i := range extensions {
		extensions[i].Levels = filterKeyFibLevels(extensions[i].Levels)
	}
	
	// 识别斐波聚集区
	clusters := fa.identifyFibClusters(retracements, extensions)
	
	// 分析黄金口袋
	goldenPocket := fa.analyzeGoldenPocket(retracements, klines)
	
	// 计算上下文评分 (在所有斐波纳契回调创建后进行)
	contextCalc := NewContextCalculator(klines)
	fa.CalculateContextScores(retracements, contextCalc)
	
	// 计算统计信息
	statistics := fa.calculateStatistics(retracements, extensions, clusters, goldenPocket)

	return &FibonacciData{
		Retracements: retracements,
		Extensions:   extensions,
		Clusters:     clusters,
		GoldenPocket: goldenPocket,
		Statistics:   statistics,
		Config:       fa.config,
	}
}

// identifySwingPoints 识别关键摆动点（混合方法：固定窗口+ZigZag动态）
// 🔥 修复：采用混合策略，兼顾历史稳定性和近期动态识别
func (fa *FibonacciAnalyzer) identifySwingPoints(klines []Kline) []PricePoint {
	if len(klines) < 10 {
		return nil
	}

	// 方法1：固定窗口方法识别历史确认的摆动点（稳定性优先）
	historicalSwings := fa.identifyHistoricalSwingPoints(klines)
	
	// 方法2：ZigZag方法识别近期动态摆动点（准确性优先）
	recentSwings := fa.identifyRecentZigZagSwings(klines)
	
	// 合并和去重
	combinedSwings := fa.mergeAndDeduplicateSwings(historicalSwings, recentSwings)
	
	return combinedSwings
}

// identifyHistoricalSwingPoints 使用固定窗口识别历史摆动点
// 用于确保历史斐波纳契分析的稳定性
func (fa *FibonacciAnalyzer) identifyHistoricalSwingPoints(klines []Kline) []PricePoint {
	var swingPoints []PricePoint
	lookback := fa.config.SwingLookback

	// 只分析历史数据，保留最后20%的数据给ZigZag方法
	historicalEnd := int(float64(len(klines)) * 0.8)
	if historicalEnd > len(klines)-lookback {
		historicalEnd = len(klines) - lookback
	}

	for i := lookback; i < historicalEnd; i++ {
		current := klines[i]
		
		// 检查是否为摆动高点
		isSwingHigh := true
		for j := i - lookback; j <= i+lookback; j++ {
			if j != i && klines[j].High >= current.High {
				isSwingHigh = false
				break
			}
		}
		
		// 检查是否为摆动低点
		isSwingLow := true
		for j := i - lookback; j <= i+lookback; j++ {
			if j != i && klines[j].Low <= current.Low {
				isSwingLow = false
				break
			}
		}
		
		// ATR自适应过滤
		if isSwingHigh || isSwingLow {
			if !fa.passesATRFilter(klines, i, lookback) {
				continue
			}
		}
		
		// 添加摆动点
		if isSwingHigh {
			swingPoints = append(swingPoints, PricePoint{
				Price:     current.High,
				Timestamp: current.OpenTime,
				Index:     i,
			})
		} else if isSwingLow {
			swingPoints = append(swingPoints, PricePoint{
				Price:     current.Low,
				Timestamp: current.OpenTime,
				Index:     i,
			})
		}
	}

	return swingPoints
}

// identifyRecentZigZagSwings 使用ZigZag方法识别近期摆动点
// 用于捕捉最新的市场结构变化
func (fa *FibonacciAnalyzer) identifyRecentZigZagSwings(klines []Kline) []PricePoint {
	if len(klines) < 20 {
		return nil
	}

	// 只分析最近30%的数据
	recentStart := int(float64(len(klines)) * 0.7)
	recentKlines := klines[recentStart:]

	// 计算动态阈值
	dynamicThreshold := fa.calculateDynamicSwingThreshold(klines)
	
	// ZigZag算法
	zigzagSwings := fa.performZigZagAnalysis(recentKlines, dynamicThreshold, recentStart)
	
	return zigzagSwings
}

// calculateDynamicSwingThreshold 计算动态摆动阈值
func (fa *FibonacciAnalyzer) calculateDynamicSwingThreshold(klines []Kline) float64 {
	if len(klines) < 20 {
		return 0.01 // 默认1%
	}
	
	// 计算ATR
	atr := fa.calculateATR(klines, 14)
	currentPrice := klines[len(klines)-1].Close
	
	// ATR百分比
	atrPercent := atr / currentPrice
	
	// 动态阈值：1.5倍ATR，限制在0.3%-3%范围内
	threshold := atrPercent * 1.5
	threshold = math.Max(0.003, math.Min(0.03, threshold))
	
	// 根据市场状态调整
	volatility := fa.calculateRecentVolatility(klines)
	if volatility > 0.02 { // 高波动
		threshold *= 1.3
	} else if volatility < 0.005 { // 低波动
		threshold *= 0.8
	}
	
	return threshold
}

// performZigZagAnalysis 执行ZigZag分析
func (fa *FibonacciAnalyzer) performZigZagAnalysis(klines []Kline, threshold float64, offsetIndex int) []PricePoint {
	var swingPoints []PricePoint
	
	if len(klines) < 3 {
		return swingPoints
	}

	type ZigZagState int
	const (
		SearchingHigh ZigZagState = iota
		SearchingLow
	)

	// 初始化
	state := SearchingHigh
	if len(klines) > 1 && klines[1].Close < klines[0].Close {
		state = SearchingLow
	}

	var candidateHigh, candidateLow PricePoint

	for i, current := range klines {
		switch state {
		case SearchingHigh:
			// 更新候选高点
			if current.High > candidateHigh.Price || candidateHigh.Index == 0 {
				candidateHigh = PricePoint{
					Price:     current.High,
					Timestamp: current.OpenTime,
					Index:     i + offsetIndex,
				}
			}
			
			// 检查反向移动
			if candidateHigh.Price > 0 {
				reversalPercent := (candidateHigh.Price - current.Low) / candidateHigh.Price
				if reversalPercent >= threshold {
					// 确认高点
					swingPoints = append(swingPoints, candidateHigh)
					
					// 切换状态
					state = SearchingLow
					candidateLow = PricePoint{
						Price:     current.Low,
						Timestamp: current.OpenTime,
						Index:     i + offsetIndex,
					}
				}
			}
			
		case SearchingLow:
			// 更新候选低点
			if current.Low < candidateLow.Price || candidateLow.Index == 0 {
				candidateLow = PricePoint{
					Price:     current.Low,
					Timestamp: current.OpenTime,
					Index:     i + offsetIndex,
				}
			}
			
			// 检查反向移动
			if candidateLow.Price > 0 {
				reversalPercent := (current.High - candidateLow.Price) / candidateLow.Price
				if reversalPercent >= threshold {
					// 确认低点
					swingPoints = append(swingPoints, candidateLow)
					
					// 切换状态
					state = SearchingHigh
					candidateHigh = PricePoint{
						Price:     current.High,
						Timestamp: current.OpenTime,
						Index:     i + offsetIndex,
					}
				}
			}
		}
	}

	return swingPoints
}

// calculateATR 计算平均真实波幅
func (fa *FibonacciAnalyzer) calculateATR(klines []Kline, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}

	var sum float64
	for i := 1; i <= period && i < len(klines); i++ {
		if len(klines)-i-1 >= 0 {
			tr := fa.calculateTrueRange(klines[len(klines)-i], klines[len(klines)-i-1])
			sum += tr
		}
	}

	return sum / float64(period)
}

// calculateTrueRange 计算真实范围
func (fa *FibonacciAnalyzer) calculateTrueRange(current, previous Kline) float64 {
	tr1 := current.High - current.Low
	tr2 := math.Abs(current.High - previous.Close)
	tr3 := math.Abs(current.Low - previous.Close)
	return math.Max(tr1, math.Max(tr2, tr3))
}

// calculateRecentVolatility 计算近期波动性
func (fa *FibonacciAnalyzer) calculateRecentVolatility(klines []Kline) float64 {
	if len(klines) < 20 {
		return 0.01
	}
	
	// 计算最近20根K线的价格波动性
	recentKlines := klines[len(klines)-20:]
	var returns []float64
	
	for i := 1; i < len(recentKlines); i++ {
		ret := (recentKlines[i].Close - recentKlines[i-1].Close) / recentKlines[i-1].Close
		returns = append(returns, ret)
	}
	
	// 计算标准差
	var sum float64
	for _, ret := range returns {
		sum += ret
	}
	mean := sum / float64(len(returns))
	
	var variance float64
	for _, ret := range returns {
		variance += math.Pow(ret-mean, 2)
	}
	
	stdDev := math.Sqrt(variance / float64(len(returns)))
	return stdDev
}

// passesATRFilter ATR自适应过滤器
func (fa *FibonacciAnalyzer) passesATRFilter(klines []Kline, index, lookback int) bool {
	if index < lookback || index >= len(klines)-lookback {
		return false
	}
	
	// 计算当前区间的ATR
	localATR := fa.calculateATR(klines[index-10:index+10], 14)
	if localATR == 0 {
		return true // 无法计算ATR时默认通过
	}
	
	// 计算摆动幅度
	var minPrice, maxPrice float64
	for i := index - lookback; i <= index + lookback; i++ {
		if i == index-lookback {
			minPrice = klines[i].Low
			maxPrice = klines[i].High
		} else {
			if klines[i].Low < minPrice {
				minPrice = klines[i].Low
			}
			if klines[i].High > maxPrice {
				maxPrice = klines[i].High
			}
		}
	}
	
	swingRange := maxPrice - minPrice
	
	// 摆动范围必须超过0.5倍ATR才被认为有意义
	return swingRange > localATR*0.5
}

// mergeAndDeduplicateSwings 合并和去重摆动点
func (fa *FibonacciAnalyzer) mergeAndDeduplicateSwings(historical, recent []PricePoint) []PricePoint {
	// 合并所有摆动点
	allSwings := append(historical, recent...)
	
	// 按时间排序
	sort.Slice(allSwings, func(i, j int) bool {
		return allSwings[i].Index < allSwings[j].Index
	})
	
	// 去重和过滤太近的点
	var deduplicated []PricePoint
	minDistance := 5 // 最小间隔（5根K线）
	
	for i, swing := range allSwings {
		// 检查是否与已有点太近
		tooClose := false
		for _, existing := range deduplicated {
			if math.Abs(float64(swing.Index-existing.Index)) < float64(minDistance) {
				// 如果太近，保留更極端的价格
				if math.Abs(swing.Price-existing.Price) > math.Abs(swing.Price)*0.001 {
					// 替换为更极端的点
					for j := range deduplicated {
						if deduplicated[j].Index == existing.Index {
							// 选择更极端的价格
							if (swing.Price > existing.Price && isLikelyHigh(swing, allSwings, i)) ||
							   (swing.Price < existing.Price && isLikelyLow(swing, allSwings, i)) {
								deduplicated[j] = swing
							}
							break
						}
					}
				}
				tooClose = true
				break
			}
		}
		
		if !tooClose {
			deduplicated = append(deduplicated, swing)
		}
	}
	
	return deduplicated
}

// isLikelyHigh 判断是否可能是高点
func isLikelyHigh(point PricePoint, allSwings []PricePoint, index int) bool {
	// 检查前后几个点的价格关系
	higherThanPrevious := index == 0 || point.Price > allSwings[index-1].Price
	higherThanNext := index == len(allSwings)-1 || point.Price > allSwings[index+1].Price
	return higherThanPrevious || higherThanNext
}

// isLikelyLow 判断是否可能是低点
func isLikelyLow(point PricePoint, allSwings []PricePoint, index int) bool {
	// 检查前后几个点的价格关系
	lowerThanPrevious := index == 0 || point.Price < allSwings[index-1].Price
	lowerThanNext := index == len(allSwings)-1 || point.Price < allSwings[index+1].Price
	return lowerThanPrevious || lowerThanNext
}

// calculateRetracements 计算斐波纳契回调
func (fa *FibonacciAnalyzer) calculateRetracements(swingPoints []PricePoint, klines []Kline) []*FibRetracement {
	var retracements []*FibRetracement

	for i := 0; i < len(swingPoints)-1; i++ {
		startPoint := swingPoints[i]
		endPoint := swingPoints[i+1]
		// 移除最小趋势长度硬阈值过滤  
		// 原有过滤: 计算价格变动幅度，如果小于阈值则跳过
		// 让AI根据币种波动特性判断趋势重要性
		//
		// priceMove := abs(endPoint.Price - startPoint.Price)
		// priceMovePercent := priceMove / startPoint.Price
		// if priceMovePercent < fa.config.MinTrendLength {
		//     continue
		// }
		
		// 确定趋势类型
		var trendType TrendType
		if endPoint.Price > startPoint.Price {
			trendType = TrendUpward
		} else {
			trendType = TrendDownward
		}
		
		// 计算斐波纳契级别
		levels := fa.calculateFibLevels(startPoint, endPoint, trendType)
		
		// 评估质量和强度
		quality, strength := fa.evaluateRetracementQuality(startPoint, endPoint, levels, klines)
		
		// 计算触及次数
		touchCount := fa.calculateTouchCounts(levels, klines, startPoint.Index, endPoint.Index)
		
		// 🔥 P0修复：时间锚点一致性 - 使用交易所CloseTime而非time.Now()
		currentTimeMs := int64(0)
		if len(klines) > 0 {
			currentTimeMs = klines[len(klines)-1].CloseTime
		} else {
			currentTimeMs = time.Now().UnixMilli()
		}
		endTimeMs := endPoint.Timestamp
		
		// 🔥 P0修复：Age单位一致性 - 统一使用小时单位
		ageMs := currentTimeMs - endTimeMs
		ageHours := int(ageMs / (3600 * 1000)) // 转换为小时
		if ageHours < 0 {
			ageHours = 0 // 防止负数年龄
		}
		
		retracement := &FibRetracement{
			ID:         fmt.Sprintf("fib_ret_%d_%d", startPoint.Index, endPoint.Index),
			StartPoint: startPoint,
			EndPoint:   endPoint,
			TrendType:  trendType,
			Levels:     levels,
			Quality:    quality,
			Strength:   strength,
			Age:        ageHours,    // 🔥 P0修复：使用小时单位
			IsActive:   true,
			TouchCount: touchCount,
			CreatedAt:  endTimeMs,   // 🔥 P0修复：使用交易所时间锚点
		}
		
		// 🔥 修复：添加生存偏差过滤，移除无效历史斐波线
		if fa.isFibRetracementValid(retracement, klines) {
			retracements = append(retracements, retracement)
		}
	}

	return retracements
}

// calculateFibLevels 计算斐波纳契级别价位
func (fa *FibonacciAnalyzer) calculateFibLevels(start, end PricePoint, trendType TrendType) []FibLevel {
	var levels []FibLevel
	priceRange := end.Price - start.Price

	for _, ratio := range fa.config.DefaultRatios {
		var price float64
		if trendType == TrendUpward {
			// 上升趋势，回调位从高点往下计算
			price = end.Price - (priceRange * ratio)
		} else {
			// 下降趋势，回调位从低点往上计算
			price = end.Price + (priceRange * ratio)
		}
		
		// 确定级别类型和重要性
		levelType := FibLevelRetracement
		importance := fa.calculateLevelImportance(ratio)
		isGoldenRatio := ratio == 0.618 || ratio == 0.382
		
		level := FibLevel{
			Ratio:         ratio,
			Price:         price,
			LevelType:     levelType,
			Importance:    importance,
			IsGoldenRatio: isGoldenRatio,
			LastTouch:     0,
		}
		
		levels = append(levels, level)
	}

	return levels
}

// calculateLevelImportance 计算级别重要性
func (fa *FibonacciAnalyzer) calculateLevelImportance(ratio float64) float64 {
	// 黄金比率具有最高重要性
	goldenRatios := []float64{0.618, 0.382}
	for _, golden := range goldenRatios {
		if abs(ratio-golden) < 0.001 {
			return 1.0
		}
	}
	
	// 其他重要比率
	importantRatios := map[float64]float64{
		0.236: 0.7,
		0.5:   0.8,
		0.786: 0.7,
		1.0:   0.6,
		1.272: 0.6,
		1.618: 0.8,
	}
	
	if importance, exists := importantRatios[ratio]; exists {
		return importance
	}
	
	return 0.5 // 默认重要性
}

// evaluateRetracementQuality 评估回调质量
func (fa *FibonacciAnalyzer) evaluateRetracementQuality(start, end PricePoint, levels []FibLevel, klines []Kline) (FibQuality, float64) {
	score := 0.0
	
	// 1. 价格变动幅度评分
	priceMove := abs(end.Price - start.Price) / start.Price
	if priceMove > 0.05 {
		score += 30
	} else if priceMove > 0.03 {
		score += 20
	} else {
		score += 10
	}
	
	// 2. 时间跨度评分
	timeSpan := end.Index - start.Index
	if timeSpan > 20 {
		score += 20
	} else if timeSpan > 10 {
		score += 15
	} else {
		score += 10
	}
	
	// 3. 成交量确认评分
	volumeScore := fa.evaluateVolumeConfirmation(start.Index, end.Index, klines)
	score += volumeScore * fa.config.VolumeWeight * 50
	
	// 确定质量等级
	var quality FibQuality
	if score >= 70 {
		quality = FibQualityHigh
	} else if score >= 40 {
		quality = FibQualityMedium
	} else {
		quality = FibQualityLow
	}
	
	return quality, score
}

// evaluateVolumeConfirmation 评估成交量确认
func (fa *FibonacciAnalyzer) evaluateVolumeConfirmation(startIdx, endIdx int, klines []Kline) float64 {
	if endIdx-startIdx < 2 {
		return 0.5
	}
	
	// 计算趋势期间的平均成交量
	var totalVolume float64
	for i := startIdx; i <= endIdx; i++ {
		totalVolume += klines[i].Volume
	}
	avgTrendVolume := totalVolume / float64(endIdx-startIdx+1)
	
	// 计算整体平均成交量
	var overallVolume float64
	validPeriods := min(float64(len(klines)), 50) // 最多看50个周期
	startPeriod := max(0, float64(endIdx)-validPeriods)
	
	for i := int(startPeriod); i <= endIdx; i++ {
		overallVolume += klines[i].Volume
	}
	avgOverallVolume := overallVolume / validPeriods
	
	// 成交量比率
	volumeRatio := avgTrendVolume / avgOverallVolume
	
	// 转换为0-1评分
	if volumeRatio > 1.5 {
		return 1.0
	} else if volumeRatio > 1.2 {
		return 0.8
	} else if volumeRatio > 1.0 {
		return 0.6
	} else {
		return 0.3
	}
}

// calculateTouchCounts 计算各级别的触及次数
func (fa *FibonacciAnalyzer) calculateTouchCounts(levels []FibLevel, klines []Kline, startIdx, endIdx int) map[string]int {
	touchCount := make(map[string]int)
	tolerance := fa.config.TouchSensitivity
	
	// 只检查趋势形成后的价格行为
	for i := endIdx + 1; i < len(klines); i++ {
		candle := klines[i]
		
		for _, level := range levels {
			// 检查价格是否触及该级别
			if abs(candle.Low-level.Price)/level.Price <= tolerance ||
			   abs(candle.High-level.Price)/level.Price <= tolerance ||
			   (candle.Low <= level.Price && candle.High >= level.Price) {
				ratioKey := fmt.Sprintf("%.3f", level.Ratio)
				touchCount[ratioKey]++
			}
		}
	}
	
	return touchCount
}

// calculateExtensions 计算斐波纳契扩展（优化基准波选择逻辑）
// 🔥 修复：智能基准波选择，提高扩展级别的可靠性和精确度
func (fa *FibonacciAnalyzer) calculateExtensions(swingPoints []PricePoint, klines []Kline) []*FibExtension {
	var extensions []*FibExtension

	if len(swingPoints) < 3 {
		return extensions
	}

	// 计算ATR用于波段幅度验证
	atr := fa.calculateATR(klines, 14)
	
	// 🔥 修复：智能基准波候选选择
	baseWaveCandidates := fa.identifyValidBaseWaves(swingPoints, klines, atr)
	
	// 为每个有效的基准波计算扩展
	for _, candidate := range baseWaveCandidates {
		// 🔥 修复：寻找匹配的回调波
		returnWaves := fa.findMatchingReturnWaves(candidate, swingPoints, klines, atr)
		
		for _, returnWave := range returnWaves {
			// 验证波段关系的有效性
			if !fa.validateWaveRelationship(candidate.BaseWave, returnWave) {
				continue
			}
			
			// 计算扩展级别
			levels := fa.calculateExtensionLevels(candidate.BaseWave, returnWave)
			
			// 评估质量（使用增强的质量评估）
			quality := fa.evaluateExtensionQualityEnhanced(candidate, returnWave, klines)
			
			// 🔥 修复：过滤低质量扩展
			if quality == FibQualityLow && fa.calculateExtensionConfidenceEnhanced(candidate, returnWave) < 0.4 {
				continue
			}
			
			extension := &FibExtension{
				ID:          fmt.Sprintf("fib_ext_%d_%d_%d", 
					candidate.BaseWave.StartPoint.Index, 
					candidate.BaseWave.EndPoint.Index, 
					returnWave.EndPoint.Index),
				BaseWave:    candidate.BaseWave,
				ReturnWave:  returnWave,
				Levels:      levels,
				Quality:     quality,
				Confidence:  fa.calculateExtensionConfidenceEnhanced(candidate, returnWave),
				IsProjected: returnWave.EndPoint.Index == len(klines)-1,
			}
			
			extensions = append(extensions, extension)
		}
	}

	// 🔥 修复：按质量和置信度排序，保留最佳扩展
	extensions = fa.filterAndRankExtensions(extensions)

	return extensions
}

// calculateExtensionLevels 计算扩展级别
func (fa *FibonacciAnalyzer) calculateExtensionLevels(baseWave, returnWave PriceWave) []FibLevel {
	var levels []FibLevel
	
	baseLength := baseWave.Length
	extensionRatios := []float64{1.0, 1.272, 1.618, 2.618}
	
	for _, ratio := range extensionRatios {
		// 根据波浪方向计算扩展价位
		var price float64
		if baseWave.EndPoint.Price > baseWave.StartPoint.Price {
			// 上升基准波 - 扩展向上
			price = returnWave.EndPoint.Price + (baseLength * ratio)
		} else {
			// 下降基准波 - 扩展向下
			price = returnWave.EndPoint.Price - (baseLength * ratio)
		}
		
		level := FibLevel{
			Ratio:      ratio,
			Price:      price,
			LevelType:  FibLevelExtension,
			Importance: fa.calculateLevelImportance(ratio),
			TouchCount: 0,
		}
		
		levels = append(levels, level)
	}
	
	return levels
}

// evaluateExtensionQuality 评估扩展质量
func (fa *FibonacciAnalyzer) evaluateExtensionQuality(baseWave, returnWave PriceWave) FibQuality {
	score := 0.0
	
	// 1. 波浪长度比例评分
	lengthRatio := returnWave.Length / baseWave.Length
	if lengthRatio > 0.3 && lengthRatio < 0.7 {
		score += 40 // 理想的回调幅度
	} else if lengthRatio > 0.2 && lengthRatio < 0.8 {
		score += 25
	} else {
		score += 10
	}
	
	// 2. 时间比例评分
	timeRatio := float64(returnWave.Duration) / float64(baseWave.Duration)
	if timeRatio > 0.3 && timeRatio < 1.5 {
		score += 30
	} else {
		score += 15
	}
	
	// 3. 波浪方向一致性
	baseDirection := baseWave.EndPoint.Price > baseWave.StartPoint.Price
	returnDirection := returnWave.EndPoint.Price > returnWave.StartPoint.Price
	if baseDirection != returnDirection {
		score += 30 // 回调方向正确
	}
	
	if score >= 70 {
		return FibQualityHigh
	} else if score >= 40 {
		return FibQualityMedium
	} else {
		return FibQualityLow
	}
}

// calculateExtensionConfidence 计算扩展置信度
func (fa *FibonacciAnalyzer) calculateExtensionConfidence(baseWave, returnWave PriceWave) float64 {
	// 基于波浪质量计算置信度
	lengthRatio := returnWave.Length / baseWave.Length
	
	// 0.382-0.618范围内的回调具有最高置信度
	if lengthRatio >= 0.382 && lengthRatio <= 0.618 {
		return 0.9
	} else if lengthRatio >= 0.3 && lengthRatio <= 0.7 {
		return 0.7
	} else if lengthRatio >= 0.2 && lengthRatio <= 0.8 {
		return 0.5
	} else {
		return 0.3
	}
}

// BaseWaveCandidate 基准波候选结构
type BaseWaveCandidate struct {
	BaseWave     PriceWave
	Quality      float64   // 波段质量评分
	Strength     float64   // 波段强度
	ValidityScore float64  // 作为基准波的有效性评分
}

// identifyValidBaseWaves 识别有效的基准波候选
// 🔥 修复：智能筛选合适的基准波，避免使用无意义的小幅波动
func (fa *FibonacciAnalyzer) identifyValidBaseWaves(swingPoints []PricePoint, klines []Kline, atr float64) []*BaseWaveCandidate {
	var candidates []*BaseWaveCandidate
	
	// 至少需要3个摆动点才能形成基准波
	if len(swingPoints) < 3 {
		return candidates
	}
	
	// 遍历可能的基准波
	for i := 0; i < len(swingPoints)-1; i++ {
		baseWave := PriceWave{
			StartPoint: swingPoints[i],
			EndPoint:   swingPoints[i+1],
			Length:     abs(swingPoints[i+1].Price - swingPoints[i].Price),
			Duration:   swingPoints[i+1].Timestamp - swingPoints[i].Timestamp,
		}
		
		// 🔥 修复1：ATR幅度验证 - 基准波必须有足够的幅度
		lengthATR := baseWave.Length / atr
		if lengthATR < 0.8 { // 基准波至少0.8倍ATR
			continue
		}
		
		// 🔥 修复2：时间验证 - 避免过于快速或缓慢的波段
		if !fa.validateWaveTiming(baseWave, klines) {
			continue
		}
		
		// 🔥 修复3：成交量验证 - 基准波应该有足够的成交量支撑
		volumeQuality := fa.assessWaveVolumeQuality(baseWave, klines)
		if volumeQuality < 0.3 {
			continue
		}
		
		// 🔥 修复4：结构质量评估 - 基准波应该是清晰的趋势波
		structureQuality := fa.assessWaveStructureQuality(baseWave, swingPoints, i)
		if structureQuality < 0.4 {
			continue
		}
		
		// 计算综合质量评分
		overallQuality := (lengthATR/3.0 + volumeQuality + structureQuality) / 3.0
		strength := fa.calculateWaveStrength(baseWave, klines, atr)
		validityScore := overallQuality * strength
		
		candidate := &BaseWaveCandidate{
			BaseWave:      baseWave,
			Quality:       overallQuality,
			Strength:      strength,
			ValidityScore: validityScore,
		}
		
		candidates = append(candidates, candidate)
	}
	
	// 🔥 修复5：按有效性评分排序，只保留最优候选
	if len(candidates) > 0 {
		// 简单排序：按ValidityScore降序
		for i := 0; i < len(candidates)-1; i++ {
			for j := i + 1; j < len(candidates); j++ {
				if candidates[j].ValidityScore > candidates[i].ValidityScore {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				}
			}
		}
		
		// 限制候选数量，避免过多无意义计算
		maxCandidates := 5
		if len(candidates) > maxCandidates {
			candidates = candidates[:maxCandidates]
		}
	}
	
	return candidates
}

// findMatchingReturnWaves 寻找与基准波匹配的回调波
// 🔥 修复：智能匹配回调波，确保符合斐波纳契扩展的经典模式
func (fa *FibonacciAnalyzer) findMatchingReturnWaves(baseCandidate *BaseWaveCandidate, swingPoints []PricePoint, klines []Kline, atr float64) []PriceWave {
	var returnWaves []PriceWave
	
	// 找到基准波结束点的索引
	baseEndIndex := -1
	for i, point := range swingPoints {
		if point.Index == baseCandidate.BaseWave.EndPoint.Index {
			baseEndIndex = i
			break
		}
	}
	
	if baseEndIndex == -1 || baseEndIndex >= len(swingPoints)-1 {
		return returnWaves
	}
	
	// 🔥 修复：寻找符合条件的回调波
	for i := baseEndIndex + 1; i < len(swingPoints); i++ {
		returnWave := PriceWave{
			StartPoint: baseCandidate.BaseWave.EndPoint,
			EndPoint:   swingPoints[i],
			Length:     abs(swingPoints[i].Price - baseCandidate.BaseWave.EndPoint.Price),
			Duration:   swingPoints[i].Timestamp - baseCandidate.BaseWave.EndPoint.Timestamp,
		}
		
		// 🔥 修复1：方向验证 - 回调波必须与基准波方向相反
		baseDirection := baseCandidate.BaseWave.EndPoint.Price > baseCandidate.BaseWave.StartPoint.Price
		returnDirection := returnWave.EndPoint.Price > returnWave.StartPoint.Price
		if baseDirection == returnDirection {
			continue // 方向相同，不是有效的回调
		}
		
		// 🔥 修复2：回调幅度验证 - 回调不能超过基准波的100%
		retracementRatio := returnWave.Length / baseCandidate.BaseWave.Length
		if retracementRatio > 1.0 || retracementRatio < 0.1 {
			continue // 回调过大或过小
		}
		
		// 🔥 修复3：ATR验证 - 回调波也需要足够的幅度
		returnLengthATR := returnWave.Length / atr
		if returnLengthATR < 0.5 { // 回调波至少0.5倍ATR
			continue
		}
		
		// 🔥 修复4：时间关系验证
		if !fa.validateReturnWaveTiming(baseCandidate.BaseWave, returnWave) {
			continue
		}
		
		returnWaves = append(returnWaves, returnWave)
		
		// 限制回调波数量，避免过多计算
		if len(returnWaves) >= 3 {
			break
		}
	}
	
	return returnWaves
}

// validateWaveRelationship 验证波段关系的有效性
func (fa *FibonacciAnalyzer) validateWaveRelationship(baseWave, returnWave PriceWave) bool {
	// 1. 时间顺序验证
	if returnWave.StartPoint.Timestamp <= baseWave.EndPoint.Timestamp {
		return false
	}
	
	// 2. 连续性验证 - 回调波必须从基准波结束点开始
	if returnWave.StartPoint.Index != baseWave.EndPoint.Index {
		return false
	}
	
	// 3. 比例合理性验证
	ratio := returnWave.Length / baseWave.Length
	return ratio >= 0.1 && ratio <= 0.9 // 回调在10%-90%之间比较合理
}

// validateWaveTiming 验证波段时间有效性
func (fa *FibonacciAnalyzer) validateWaveTiming(wave PriceWave, klines []Kline) bool {
	if len(klines) == 0 {
		return true
	}
	
	// 计算平均K线间隔
	if len(klines) < 2 {
		return true
	}
	avgInterval := (klines[len(klines)-1].OpenTime - klines[0].OpenTime) / int64(len(klines)-1)
	
	// 波段持续时间应该在合理范围内
	minDuration := avgInterval * 2    // 至少2根K线
	maxDuration := avgInterval * 50   // 最多50根K线
	
	return wave.Duration >= minDuration && wave.Duration <= maxDuration
}

// validateReturnWaveTiming 验证回调波时间关系
func (fa *FibonacciAnalyzer) validateReturnWaveTiming(baseWave, returnWave PriceWave) bool {
	// 回调波的持续时间不应该过长
	maxReturnDuration := baseWave.Duration * 3 // 最多3倍基准波时间
	return returnWave.Duration <= maxReturnDuration
}

// assessWaveVolumeQuality 评估波段成交量质量
func (fa *FibonacciAnalyzer) assessWaveVolumeQuality(wave PriceWave, klines []Kline) float64 {
	if len(klines) == 0 {
		return 0.5 // 默认中等质量
	}
	
	// 找到波段对应的K线区间
	startIdx := wave.StartPoint.Index
	endIdx := wave.EndPoint.Index
	
	if startIdx < 0 || endIdx >= len(klines) || startIdx >= endIdx {
		return 0.5
	}
	
	// 计算波段期间的平均成交量
	var waveVolume float64
	for i := startIdx; i <= endIdx; i++ {
		waveVolume += klines[i].Volume
	}
	avgWaveVolume := waveVolume / float64(endIdx-startIdx+1)
	
	// 计算历史平均成交量
	lookback := 20
	var historicalVolume float64
	validCount := 0
	
	start := endIdx - lookback
	if start < 0 {
		start = 0
	}
	
	for i := start; i < endIdx; i++ {
		historicalVolume += klines[i].Volume
		validCount++
	}
	
	if validCount == 0 {
		return 0.5
	}
	
	avgHistoricalVolume := historicalVolume / float64(validCount)
	
	if avgHistoricalVolume == 0 {
		return 0.5
	}
	
	// 成交量比率评分
	volumeRatio := avgWaveVolume / avgHistoricalVolume
	
	if volumeRatio > 1.5 {
		return 1.0 // 优秀
	} else if volumeRatio > 1.2 {
		return 0.8 // 良好
	} else if volumeRatio > 0.8 {
		return 0.6 // 一般
	} else {
		return 0.3 // 较差
	}
}

// assessWaveStructureQuality 评估波段结构质量
func (fa *FibonacciAnalyzer) assessWaveStructureQuality(wave PriceWave, swingPoints []PricePoint, waveIndex int) float64 {
	score := 0.5 // 基础评分
	
	// 1. 检查波段是否是单向的（没有被中间摆动点破坏）
	direction := wave.EndPoint.Price > wave.StartPoint.Price
	
	for _, point := range swingPoints {
		if point.Index > wave.StartPoint.Index && point.Index < wave.EndPoint.Index {
			// 有中间摆动点，检查是否破坏了趋势
			if direction {
				// 上升波段，中间不应该有更高的高点
				if point.Price > wave.EndPoint.Price {
					score -= 0.2
				}
			} else {
				// 下降波段，中间不应该有更低的低点
				if point.Price < wave.EndPoint.Price {
					score -= 0.2
				}
			}
		}
	}
	
	// 2. 波段的相对位置评分（是否处于明显的趋势中）
	if waveIndex > 0 && waveIndex < len(swingPoints)-2 {
		prevPoint := swingPoints[waveIndex-1]
		nextPoint := swingPoints[waveIndex+2]
		
		// 检查是否符合趋势延续
		if direction {
			if wave.StartPoint.Price > prevPoint.Price && wave.EndPoint.Price < nextPoint.Price {
				score += 0.3 // 符合上升趋势
			}
		} else {
			if wave.StartPoint.Price < prevPoint.Price && wave.EndPoint.Price > nextPoint.Price {
				score += 0.3 // 符合下降趋势
			}
		}
	}
	
	return math.Max(0.1, math.Min(1.0, score))
}

// calculateWaveStrength 计算波段强度
func (fa *FibonacciAnalyzer) calculateWaveStrength(wave PriceWave, klines []Kline, atr float64) float64 {
	if atr == 0 {
		return 0.5
	}
	
	// 基于ATR的相对强度
	lengthATR := wave.Length / atr
	
	// 标准化到0-1范围
	if lengthATR > 3.0 {
		return 1.0 // 非常强
	} else if lengthATR > 2.0 {
		return 0.8 // 强
	} else if lengthATR > 1.0 {
		return 0.6 // 中等
	} else if lengthATR > 0.5 {
		return 0.4 // 较弱
	} else {
		return 0.2 // 弱
	}
}

// evaluateExtensionQualityEnhanced 增强的扩展质量评估
func (fa *FibonacciAnalyzer) evaluateExtensionQualityEnhanced(baseCandidate *BaseWaveCandidate, returnWave PriceWave, klines []Kline) FibQuality {
	score := 0.0
	
	// 1. 基准波质量权重 (40%)
	score += baseCandidate.Quality * 40
	
	// 2. 波段比例评分 (30%)
	lengthRatio := returnWave.Length / baseCandidate.BaseWave.Length
	if lengthRatio > 0.3 && lengthRatio < 0.7 {
		score += 30 // 理想的回调幅度
	} else if lengthRatio > 0.2 && lengthRatio < 0.8 {
		score += 20
	} else {
		score += 10
	}
	
	// 3. 时间比例评分 (20%)
	timeRatio := float64(returnWave.Duration) / float64(baseCandidate.BaseWave.Duration)
	if timeRatio > 0.3 && timeRatio < 1.5 {
		score += 20
	} else {
		score += 10
	}
	
	// 4. 成交量确认评分 (10%)
	volumeQuality := fa.assessWaveVolumeQuality(returnWave, klines)
	score += volumeQuality * 10
	
	if score >= 70 {
		return FibQualityHigh
	} else if score >= 50 {
		return FibQualityMedium
	} else {
		return FibQualityLow
	}
}

// calculateExtensionConfidenceEnhanced 增强的扩展置信度计算
func (fa *FibonacciAnalyzer) calculateExtensionConfidenceEnhanced(baseCandidate *BaseWaveCandidate, returnWave PriceWave) float64 {
	confidence := 0.0
	
	// 1. 基准波有效性评分权重
	confidence += baseCandidate.ValidityScore * 0.4
	
	// 2. 经典斐波比例评分
	lengthRatio := returnWave.Length / baseCandidate.BaseWave.Length
	if lengthRatio >= 0.382 && lengthRatio <= 0.618 {
		confidence += 0.4 // 黄金比例回调
	} else if lengthRatio >= 0.3 && lengthRatio <= 0.7 {
		confidence += 0.3
	} else if lengthRatio >= 0.236 && lengthRatio <= 0.786 {
		confidence += 0.2
	} else {
		confidence += 0.1
	}
	
	// 3. 波段强度评分
	confidence += baseCandidate.Strength * 0.2
	
	return math.Min(1.0, confidence)
}

// filterAndRankExtensions 过滤和排序扩展
func (fa *FibonacciAnalyzer) filterAndRankExtensions(extensions []*FibExtension) []*FibExtension {
	if len(extensions) == 0 {
		return extensions
	}
	
	// 按置信度排序（降序）
	for i := 0; i < len(extensions)-1; i++ {
		for j := i + 1; j < len(extensions); j++ {
			if extensions[j].Confidence > extensions[i].Confidence {
				extensions[i], extensions[j] = extensions[j], extensions[i]
			}
		}
	}
	
	// 限制扩展数量，保留最优的
	maxExtensions := 8
	if len(extensions) > maxExtensions {
		extensions = extensions[:maxExtensions]
	}
	
	return extensions
}

// analyzeGoldenPocket 分析黄金口袋(0.618-0.65范围)
func (fa *FibonacciAnalyzer) analyzeGoldenPocket(retracements []*FibRetracement, klines []Kline) *GoldenPocket {
	if len(retracements) == 0 {
		return nil
	}
	
	// 寻找最佳的黄金口袋候选
	var bestRetracement *FibRetracement
	var bestQualityScore float64
	
	for _, ret := range retracements {
		if ret.Quality == FibQualityHigh && ret.Strength > bestQualityScore {
			bestRetracement = ret
			bestQualityScore = ret.Strength
		}
	}
	
	if bestRetracement == nil {
		// 如果没有高质量的，选择最好的中等质量
		for _, ret := range retracements {
			if ret.Quality == FibQualityMedium && ret.Strength > bestQualityScore {
				bestRetracement = ret
				bestQualityScore = ret.Strength
			}
		}
	}
	
	if bestRetracement == nil {
		return nil
	}
	
	// 计算黄金口袋价格范围
	var goldenLow, goldenHigh float64
	priceRange := abs(bestRetracement.EndPoint.Price - bestRetracement.StartPoint.Price)
	
	if bestRetracement.TrendType == TrendUpward {
		// 上升趋势的黄金口袋
		goldenHigh = bestRetracement.EndPoint.Price - (priceRange * fa.config.GoldenPocketRange[0])
		goldenLow = bestRetracement.EndPoint.Price - (priceRange * fa.config.GoldenPocketRange[1])
	} else {
		// 下降趋势的黄金口袋
		goldenLow = bestRetracement.EndPoint.Price + (priceRange * fa.config.GoldenPocketRange[0])
		goldenHigh = bestRetracement.EndPoint.Price + (priceRange * fa.config.GoldenPocketRange[1])
	}
	
	// 分析成交量和触及事件
	touchEvents := fa.analyzeTouchEvents(goldenLow, goldenHigh, klines, bestRetracement.EndPoint.Index, bestRetracement.TrendType)
	volumeProfile := fa.analyzeVolumeProfile(goldenLow, goldenHigh, klines, bestRetracement.EndPoint.Index)
	
	// 评估强度和质量
	strength := fa.evaluateGoldenPocketStrength(bestRetracement, touchEvents, volumeProfile)
	quality := fa.evaluateGoldenPocketQuality(bestRetracement, touchEvents)
	
	goldenPocket := &GoldenPocket{
		ID: fmt.Sprintf("golden_pocket_%s", bestRetracement.ID),
		PriceRange: PriceRange{
			Low:  goldenLow,
			High: goldenHigh,
		},
		CenterPrice:   (goldenLow + goldenHigh) / 2,
		Quality:       quality,
		Strength:      strength,
		TrendContext:  bestRetracement.TrendType,
		VolumeProfile: volumeProfile,
		TouchEvents:   touchEvents,
		IsActive:      fa.isGoldenPocketActive(goldenLow, goldenHigh, klines),
		LastUpdate:    time.Now().UnixMilli(), // 🔥 P0修复：统一使用毫秒时间戳
	}
	
	return goldenPocket
}

// analyzeTouchEvents 分析触及事件
// 🔥 P0修复：基于趋势方向、触碰方向和5m收盘确认的正确反应分类逻辑
func (fa *FibonacciAnalyzer) analyzeTouchEvents(low, high float64, klines []Kline, startIdx int, trendType TrendType) []TouchEvent {
	var touchEvents []TouchEvent
	tolerance := fa.config.TouchSensitivity
	
	for i := startIdx + 1; i < len(klines); i++ {
		candle := klines[i]
		
		// 检查是否触及黄金口袋区域
		if (candle.Low <= high*(1+tolerance) && candle.High >= low*(1-tolerance)) {
			// 🔥 P0修复：重构反应类型判断逻辑
			reactionType := fa.classifyTouchReaction(candle, klines, i, low, high, trendType)
			
			// 计算反应强度
			strength := fa.calculateReactionStrength(candle, klines, i)
			
			touchEvent := TouchEvent{
				Price:     (candle.High + candle.Low) / 2,
				Timestamp: candle.OpenTime,
				Reaction:  reactionType,
				Volume:    candle.Volume,
				Strength:  strength,
			}
			
			touchEvents = append(touchEvents, touchEvent)
		}
	}
	
	return touchEvents
}

// classifyTouchReaction 分类触碰反应
// 🔥 P0修复：基于趋势方向、触碰方向和价格确认的正确分类逻辑
func (fa *FibonacciAnalyzer) classifyTouchReaction(candle Kline, klines []Kline, index int, goldenLow, goldenHigh float64, trendType TrendType) ReactionType {
	// 确保有足够的后续K线进行确认
	confirmationPeriod := 3 // 使用3根K线确认
	if index+confirmationPeriod >= len(klines) {
		return ReactionConsolidation
	}
	
	// 🎯 关键修复1：获取确认期间的价格数据
	priceAtTouch := candle.Close
	priceAfter := klines[index+confirmationPeriod].Close
	
	// 🎯 关键修复2：判断触碰方向（从前一根K线判断）
	var prevPrice float64
	if index > 0 {
		prevPrice = klines[index-1].Close
	} else {
		prevPrice = candle.Open
	}
	
	// 判断是回撤触及还是反弹触及
	isTouchFromAbove := prevPrice > (goldenHigh + goldenLow) / 2
	
	// 🎯 关键修复3：计算价格变化幅度和方向
	priceChangePercent := abs(priceAfter - priceAtTouch) / priceAtTouch
	
	// 价格变化太小，判定为整固
	if priceChangePercent <= 0.01 {
		return ReactionConsolidation
	}
	
	// 🎯 关键修复4：基于趋势方向和触碰方向进行正确分类
	isPriceUp := priceAfter > priceAtTouch
	isTrendUpward := trendType == TrendUpward
	
	switch {
	case isTouchFromAbove: // 从上方回撤触及黄金口袋
		if isTrendUpward {
			// 上升趋势中的回撤触及
			if isPriceUp {
				return ReactionBounce // 回撤后反弹，正常延续
			} else {
				return ReactionBreak // 回撤后继续下跌，趋势失败
			}
		} else {
			// 下降趋势中的回撤触及
			if !isPriceUp {
				return ReactionBounce // 回撤后继续下跌，正常延续
			} else {
				return ReactionBreak // 回撤后反弹，趋势失败
			}
		}
		
	case !isTouchFromAbove: // 从下方反弹触及黄金口袋
		if isTrendUpward {
			// 上升趋势中的反弹触及
			if isPriceUp {
				return ReactionBreak // 反弹后突破口袋，继续上涨
			} else {
				return ReactionBounce // 反弹后被阻，口袋阻力有效
			}
		} else {
			// 下降趋势中的反弹触及
			if !isPriceUp {
				return ReactionBreak // 反弹后继续下跌，突破口袋
			} else {
				return ReactionBounce // 反弹后上涨，口袋支撑有效
			}
		}
	}
	
	return ReactionConsolidation
}

// calculateReactionStrength 计算反应强度
func (fa *FibonacciAnalyzer) calculateReactionStrength(candle Kline, klines []Kline, index int) float64 {
	// 计算价格波动范围
	priceRange := (candle.High - candle.Low) / candle.Open
	
	// 计算成交量比率
	var avgVolume float64
	lookback := minInt(10, index)
	for i := index - lookback; i < index; i++ {
		if i >= 0 {
			avgVolume += klines[i].Volume
		}
	}
	if lookback > 0 {
		avgVolume /= float64(lookback)
	}
	
	volumeRatio := candle.Volume / avgVolume
	if avgVolume == 0 {
		volumeRatio = 1.0
	}
	
	// 综合评分
	strength := (priceRange*50 + min(volumeRatio, 3.0)*25) / 75 * 100
	return min(strength, 100.0)
}

// analyzeVolumeProfile 分析成交量概况
func (fa *FibonacciAnalyzer) analyzeVolumeProfile(low, high float64, klines []Kline, startIdx int) VolumeInfo {
	var totalVolume, volumeInRange float64
	var spikesCount int
	rangeCandles := 0
	
	// 计算平均成交量
	for i := startIdx + 1; i < len(klines); i++ {
		candle := klines[i]
		totalVolume += candle.Volume
		
		// 检查是否在黄金口袋范围内
		if (candle.Low <= high && candle.High >= low) {
			volumeInRange += candle.Volume
			rangeCandles++
		}
		
		// 检查成交量激增
		if i > 0 {
			prevVolume := klines[i-1].Volume
			if prevVolume > 0 && candle.Volume/prevVolume > 2.0 {
				spikesCount++
			}
		}
	}
	
	periods := len(klines) - startIdx - 1
	avgVolume := totalVolume / float64(periods)
	currentVolume := klines[len(klines)-1].Volume
	
	var volumeRatio float64
	if avgVolume > 0 {
		volumeRatio = currentVolume / avgVolume
	} else {
		volumeRatio = 1.0
	}
	
	return VolumeInfo{
		AverageVolume:  avgVolume,
		CurrentVolume:  currentVolume,
		VolumeRatio:    volumeRatio,
		SpikesCount:    spikesCount,
	}
}

// evaluateGoldenPocketStrength 评估黄金口袋强度
func (fa *FibonacciAnalyzer) evaluateGoldenPocketStrength(retracement *FibRetracement, touches []TouchEvent, volume VolumeInfo) float64 {
	score := 0.0
	
	// 1. 基础回调质量评分 (40%)
	score += retracement.Strength * 0.4
	
	// 2. 触及反应评分 (30%)
	touchScore := 0.0
	if len(touches) > 0 {
		var avgStrength float64
		bounceCount := 0
		
		for _, touch := range touches {
			avgStrength += touch.Strength
			if touch.Reaction == ReactionBounce {
				bounceCount++
			}
		}
		
		avgStrength /= float64(len(touches))
		bounceRate := float64(bounceCount) / float64(len(touches))
		
		touchScore = (avgStrength + bounceRate*100) / 2
	}
	score += touchScore * 0.3
	
	// 3. 成交量确认评分 (20%)
	volumeScore := min(volume.VolumeRatio*25, 50.0) + min(float64(volume.SpikesCount)*10, 50.0)
	score += volumeScore * 0.2
	
	// 4. 时间有效性评分 (10%)
	ageScore := max(0, 100-float64(retracement.Age)*2) // 年龄越大评分越低
	score += ageScore * 0.1
	
	return min(score, 100.0)
}

// evaluateGoldenPocketQuality 评估黄金口袋质量
func (fa *FibonacciAnalyzer) evaluateGoldenPocketQuality(retracement *FibRetracement, touches []TouchEvent) FibQuality {
	if retracement.Quality == FibQualityHigh && len(touches) > 0 {
		// 有触及记录的高质量回调
		bounceCount := 0
		for _, touch := range touches {
			if touch.Reaction == ReactionBounce {
				bounceCount++
			}
		}
		
		if float64(bounceCount)/float64(len(touches)) > 0.6 {
			return FibQualityHigh
		} else {
			return FibQualityMedium
		}
	} else if retracement.Quality == FibQualityMedium {
		return FibQualityMedium
	} else {
		return FibQualityLow
	}
}

// isGoldenPocketActive 判断黄金口袋是否活跃
func (fa *FibonacciAnalyzer) isGoldenPocketActive(low, high float64, klines []Kline) bool {
	if len(klines) == 0 {
		return false
	}
	
	currentPrice := klines[len(klines)-1].Close
	
	// 价格在黄金口袋范围内或接近范围
	tolerance := 0.02 // 2%容忍度
	return currentPrice >= low*(1-tolerance) && currentPrice <= high*(1+tolerance)
}

// identifyFibClusters 识别斐波聚集区
func (fa *FibonacciAnalyzer) identifyFibClusters(retracements []*FibRetracement, extensions []*FibExtension) []*FibCluster {
	var allLevels []struct {
		price  float64
		source string
		ratio  float64
	}
	
	// 收集所有斐波级别
	for _, ret := range retracements {
		for _, level := range ret.Levels {
			allLevels = append(allLevels, struct {
				price  float64
				source string
				ratio  float64
			}{level.Price, ret.ID, level.Ratio})
		}
	}
	
	for _, ext := range extensions {
		for _, level := range ext.Levels {
			allLevels = append(allLevels, struct {
				price  float64
				source string
				ratio  float64
			}{level.Price, ext.ID, level.Ratio})
		}
	}
	
	// 按价格排序
	sort.Slice(allLevels, func(i, j int) bool {
		return allLevels[i].price < allLevels[j].price
	})
	
	var clusters []*FibCluster
	clusterTolerance := fa.config.ClusterDistance
	
	// 识别价格聚集区
	for i := 0; i < len(allLevels); {
		currentPrice := allLevels[i].price
		var clusterLevels []struct {
			price  float64
			source string
			ratio  float64
		}
		
		// 收集在容忍范围内的所有级别
		j := i
		for j < len(allLevels) && abs(allLevels[j].price-currentPrice)/currentPrice <= clusterTolerance {
			clusterLevels = append(clusterLevels, allLevels[j])
			j++
		}
		
		// 如果有多个级别聚集，创建聚集区
		if len(clusterLevels) >= 2 {
			var sources []string
			var minPrice, maxPrice float64
			minPrice = clusterLevels[0].price
			maxPrice = clusterLevels[0].price
			
			for _, level := range clusterLevels {
				sources = append(sources, level.source)
				if level.price < minPrice {
					minPrice = level.price
				}
				if level.price > maxPrice {
					maxPrice = level.price
				}
			}
			
			centerPrice := (minPrice + maxPrice) / 2
			density := float64(len(clusterLevels)) / (maxPrice - minPrice)
			importance := fa.calculateClusterImportance(clusterLevels)
			
			cluster := &FibCluster{
				ID:          fmt.Sprintf("fib_cluster_%d", len(clusters)),
				CenterPrice: centerPrice,
				PriceRange: PriceRange{
					Low:  minPrice,
					High: maxPrice,
				},
				Density:    density,
				LevelCount: len(clusterLevels),
				Sources:    sources,
				Importance: importance,
			}
			
			clusters = append(clusters, cluster)
		}
		
		i = j
	}
	
	return clusters
}

// calculateClusterImportance 计算聚集区重要性
func (fa *FibonacciAnalyzer) calculateClusterImportance(levels []struct {
	price  float64
	source string
	ratio  float64
}) float64 {
	importance := 0.0
	
	// 基础重要性 = 级别数量
	importance += float64(len(levels)) * 20
	
	// 黄金比率加成
	for _, level := range levels {
		if level.ratio == 0.618 || level.ratio == 0.382 {
			importance += 30
		} else if level.ratio == 0.5 || level.ratio == 1.618 {
			importance += 20
		} else {
			importance += 10
		}
	}
	
	return min(importance, 100.0)
}

// calculateStatistics 计算统计信息
func (fa *FibonacciAnalyzer) calculateStatistics(retracements []*FibRetracement, extensions []*FibExtension, clusters []*FibCluster, goldenPocket *GoldenPocket) *FibStatistics {
	stats := &FibStatistics{
		TotalRetracements: len(retracements),
		ClusterCount:      len(clusters),
	}
	
	// 计算活跃回调数量
	for _, ret := range retracements {
		if ret.IsActive {
			stats.ActiveRetracements++
		}
		if ret.Quality == FibQualityHigh {
			stats.HighQualityCount++
		}
	}
	
	// 计算平均强度
	if len(retracements) > 0 {
		var totalStrength float64
		for _, ret := range retracements {
			totalStrength += ret.Strength
		}
		stats.AvgStrength = totalStrength / float64(len(retracements))
	}
	
	// 计算黄金比率命中次数
	for _, ret := range retracements {
		for _, level := range ret.Levels {
			if level.IsGoldenRatio && level.TouchCount > 0 {
				stats.GoldenRatioHits += level.TouchCount
			}
		}
	}
	
	// 计算成功率 (简化版本)
	if stats.TotalRetracements > 0 {
		stats.SuccessRate = float64(stats.HighQualityCount) / float64(stats.TotalRetracements)
	}
	
	// 平均反应时间 (简化版本)
	stats.AvgReactionTime = 3.5 // 小时，基于经验值
	
	return stats
}

// GenerateSignals 生成斐波纳契交易信号
func (fa *FibonacciAnalyzer) GenerateSignals(fibData *FibonacciData, klines []Kline) []*FibSignal {
	var signals []*FibSignal
	
	if len(klines) == 0 {
		return signals
	}
	
	currentPrice := klines[len(klines)-1].Close
	
	// 1. 黄金口袋信号
	if fibData.GoldenPocket != nil && fibData.GoldenPocket.IsActive {
		goldenSignal := fa.generateGoldenPocketSignal(fibData.GoldenPocket, currentPrice)
		if goldenSignal != nil {
			signals = append(signals, goldenSignal)
		}
	}
	
	// 2. 关键斐波级别信号
	for _, ret := range fibData.Retracements {
		if !ret.IsActive {
			continue
		}
		
		levelSignals := fa.generateLevelSignals(ret, currentPrice)
		signals = append(signals, levelSignals...)
	}
	
	// 3. 聚集区信号
	for _, cluster := range fibData.Clusters {
		clusterSignal := fa.generateClusterSignal(cluster, currentPrice)
		if clusterSignal != nil {
			signals = append(signals, clusterSignal)
		}
	}
	
	return signals
}

// generateGoldenPocketSignal 生成黄金口袋信号
func (fa *FibonacciAnalyzer) generateGoldenPocketSignal(goldenPocket *GoldenPocket, currentPrice float64) *FibSignal {
	if !goldenPocket.IsActive {
		return nil
	}
	
	// 检查价格是否在黄金口袋范围内
	inRange := currentPrice >= goldenPocket.PriceRange.Low && currentPrice <= goldenPocket.PriceRange.High
	
	if !inRange {
		return nil
	}
	
	// 确定信号方向
	var action SignalAction
	var entry, stopLoss float64
	var takeProfit []float64
	
	if goldenPocket.TrendContext == TrendUpward {
		// 上升趋势中的黄金口袋 - 买入信号
		action = ActionBuy
		entry = currentPrice
		stopLoss = goldenPocket.PriceRange.Low * 0.99 // 在黄金口袋下方1%
		takeProfit = []float64{
			goldenPocket.PriceRange.High * 1.05, // 第一目标：黄金口袋上方5%
			goldenPocket.PriceRange.High * 1.1,  // 第二目标：黄金口袋上方10%
		}
	} else {
		// 下降趋势中的黄金口袋 - 卖出信号
		action = ActionSell
		entry = currentPrice
		stopLoss = goldenPocket.PriceRange.High * 1.01 // 在黄金口袋上方1%
		takeProfit = []float64{
			goldenPocket.PriceRange.Low * 0.95, // 第一目标：黄金口袋下方5%
			goldenPocket.PriceRange.Low * 0.9,  // 第二目标：黄金口袋下方10%
		}
	}
	
	// 计算风险收益比
	riskReward := abs(takeProfit[0]-entry) / abs(entry-stopLoss)
	
	signal := &FibSignal{
		ID:         fmt.Sprintf("golden_pocket_%s", goldenPocket.ID),
		Type:       FibSignalGoldenPocket,
		Action:     action,
		Price:      currentPrice,
		Level:      0.618, // 黄金比率
		Confidence: goldenPocket.Strength,
		Strength:   goldenPocket.Strength,
		EntryPrice: entry,
		StopLoss:   stopLoss,
		TakeProfit: takeProfit,
		RiskReward: riskReward,
		Context:    "黄金口袋0.618回调支撑/阻力",
		Source:     "fibonacci_golden_pocket",
		Quality:    convertFibQualityToSignalQuality(goldenPocket.Quality),
		Timestamp:  time.Now().UnixMilli(), // 🔥 P0修复：统一使用毫秒时间戳
	}
	
	return signal
}

// generateLevelSignals 生成级别信号
func (fa *FibonacciAnalyzer) generateLevelSignals(retracement *FibRetracement, currentPrice float64) []*FibSignal {
	var signals []*FibSignal
	tolerance := fa.config.TouchSensitivity
	
	for _, level := range retracement.Levels {
		// 检查价格是否接近该级别
		priceDistance := abs(currentPrice-level.Price) / level.Price
		if priceDistance > tolerance {
			continue
		}
		
		// 只为重要级别生成信号
		if level.Importance < 0.7 {
			continue
		}
		
		var signalType FibSignalType
		var action SignalAction
		
		// 根据趋势类型确定信号
		if retracement.TrendType == TrendUpward {
			signalType = FibSignalBounce
			action = ActionBuy
		} else {
			signalType = FibSignalBounce
			action = ActionSell
		}
		
		signal := &FibSignal{
			ID:         fmt.Sprintf("fib_level_%s_%.3f", retracement.ID, level.Ratio),
			Type:       signalType,
			Action:     action,
			Price:      currentPrice,
			Level:      level.Ratio,
			Confidence: retracement.Strength * level.Importance,
			Strength:   level.Importance * 100,
			EntryPrice: level.Price,
			Context:    fmt.Sprintf("斐波纳契%.1f%%回调级别", level.Ratio*100),
			Source:     "fibonacci_retracement",
			Quality:    convertFibQualityToSignalQuality(retracement.Quality),
			Timestamp:  time.Now().Unix(),
		}
		
		signals = append(signals, signal)
	}
	
	return signals
}

// generateClusterSignal 生成聚集区信号
func (fa *FibonacciAnalyzer) generateClusterSignal(cluster *FibCluster, currentPrice float64) *FibSignal {
	// 检查价格是否在聚集区范围内
	if currentPrice < cluster.PriceRange.Low || currentPrice > cluster.PriceRange.High {
		return nil
	}
	
	// 聚集区重要性要足够高
	if cluster.Importance < 60 {
		return nil
	}
	
	signal := &FibSignal{
		ID:         fmt.Sprintf("fib_cluster_%s", cluster.ID),
		Type:       FibSignalCluster,
		Action:     ActionHold, // 聚集区通常是观察信号
		Price:      currentPrice,
		Level:      0.0,
		Confidence: cluster.Importance,
		Strength:   cluster.Density * 10,
		EntryPrice: cluster.CenterPrice,
		Context:    fmt.Sprintf("斐波聚集区(%d个级别)", cluster.LevelCount),
		Source:     "fibonacci_cluster",
		Quality:    SignalQualityMedium, // 聚集区默认中等质量
		Timestamp:  time.Now().Unix(),
	}
	
	return signal
}

type SignalQuality int

const (
	SignalQualityHigh SignalQuality = iota
	SignalQualityMedium
	SignalQualityLow
)

// convertFibQualityToSignalQuality 转换斐波纳契质量为信号质量
func convertFibQualityToSignalQuality(fibQuality FibQuality) SignalQuality {
	switch fibQuality {
	case FibQualityHigh:
		return SignalQualityHigh
	case FibQualityMedium:
		return SignalQualityMedium
	case FibQualityLow:
		return SignalQualityLow
	default:
		return SignalQualityMedium
	}
}

// isFibRetracementValid 检查斐波纳契回调是否有效（修复生存偏差）
// 通过历史验证过滤掉无效的斐波线，避免"幸存者偏差"
func (fa *FibonacciAnalyzer) isFibRetracementValid(retracement *FibRetracement, klines []Kline) bool {
	// 1. 基础有效性检查
	if retracement == nil || len(retracement.Levels) == 0 {
		return false
	}

	// 2. 趋势幅度检查 - 过滤微小趋势
	priceMove := math.Abs(retracement.EndPoint.Price - retracement.StartPoint.Price)
	priceMovePercent := priceMove / retracement.StartPoint.Price
	
	// 使用ATR自适应阈值替代固定阈值
	atr := fa.calculateATR(klines, 14)
	minTrendThreshold := fa.calculateMinTrendThreshold(klines, atr)
	
	if priceMovePercent < minTrendThreshold {
		return false // 趋势过小，不足以产生有意义的斐波回调
	}

	// 3. 历史验证检查 - 检查斐波线是否被历史价格验证
	historicalValidation := fa.validateFibWithHistory(retracement, klines)
	if !historicalValidation {
		return false // 未通过历史验证
	}

	// 4. 质量阈值检查 - 过滤低质量回调
	if retracement.Quality == FibQualityLow && retracement.Strength < 30 {
		return false // 质量和强度都太低
	}

	// 5. 时间有效性检查 - 过滤过期的斐波线
	if retracement.Age > fa.config.MaxRetracementAge {
		return false // 太老的斐波线失去意义
	}

	// 6. 成交量确认检查（可选）
	if fa.config.VolumeWeight > 0.0 {
		volumeConfirmation := fa.checkVolumeConfirmation(retracement, klines)
		if !volumeConfirmation {
			return false // 缺乏成交量支撑
		}
	}

	return true
}

// calculateMinTrendThreshold 计算基于ATR的最小趋势阈值
func (fa *FibonacciAnalyzer) calculateMinTrendThreshold(klines []Kline, atr float64) float64 {
	if len(klines) == 0 || atr == 0 {
		return fa.config.MinTrendLength // 使用配置的默认值
	}

	// 基于ATR的自适应阈值：趋势至少应该是2倍ATR
	currentPrice := klines[len(klines)-1].Close
	atrPercent := atr / currentPrice
	adaptiveThreshold := atrPercent * 2.0

	// 限制在合理范围内
	minThreshold := 0.005 // 最小0.5%
	maxThreshold := 0.08  // 最大8%

	return math.Max(minThreshold, math.Min(maxThreshold, adaptiveThreshold))
}

// validateFibWithHistory 通过历史价格验证斐波线
func (fa *FibonacciAnalyzer) validateFibWithHistory(retracement *FibRetracement, klines []Kline) bool {
	// 检查斐波线形成后的价格行为
	startIdx := retracement.EndPoint.Index
	if startIdx >= len(klines)-5 {
		return true // 太新的斐波线暂时认为有效
	}

	historicalKlines := klines[startIdx:]
	if len(historicalKlines) < 5 {
		return true // 历史数据不足
	}

	validLevels := 0
	totalLevels := 0

	// 检查每个重要斐波级别的历史表现
	for _, level := range retracement.Levels {
		if level.Importance < 0.7 {
			continue // 跳过不重要的级别
		}

		totalLevels++
		
		// 检查该级别是否被历史价格触及或接近
		wasRespected := fa.checkLevelRespected(level, historicalKlines)
		if wasRespected {
			validLevels++
		}
	}

	// 如果没有重要级别，或者超过50%的重要级别被历史验证，则认为有效
	if totalLevels == 0 {
		return true
	}

	validationRate := float64(validLevels) / float64(totalLevels)
	return validationRate >= 0.4 // 至少40%的重要级别需要被历史验证
}

// checkLevelRespected 检查斐波级别是否被历史价格尊重
func (fa *FibonacciAnalyzer) checkLevelRespected(level FibLevel, klines []Kline) bool {
	tolerance := fa.config.TouchSensitivity
	touchCount := 0
	significantReactions := 0

	for i, kline := range klines {
		// 检查价格是否触及该级别
		if fa.isPriceTouchingLevel(kline, level.Price, tolerance) {
			touchCount++

			// 检查触及后是否有显著反应
			if i < len(klines)-3 {
				reaction := fa.calculateReactionAfterTouch(klines, i, level.Price)
				if reaction > 0.01 { // 1%以上的反应认为是显著的
					significantReactions++
				}
			}
		}
	}

	// 级别有效的条件：
	// 1. 被触及过至少一次，或者
	// 2. 有显著反应，或者  
	// 3. 从未被明显突破（保持尊重）
	if touchCount == 0 {
		return !fa.wasLevelBroken(level.Price, klines) // 未触及但也未被破坏
	}

	// 被触及过，检查反应率
	if touchCount > 0 && significantReactions > 0 {
		reactionRate := float64(significantReactions) / float64(touchCount)
		return reactionRate >= 0.3 // 至少30%的触及产生了显著反应
	}

	return false
}

// isPriceTouchingLevel 检查价格是否触及级别
func (fa *FibonacciAnalyzer) isPriceTouchingLevel(kline Kline, levelPrice float64, tolerance float64) bool {
	return math.Abs(kline.Low-levelPrice)/levelPrice <= tolerance ||
		   math.Abs(kline.High-levelPrice)/levelPrice <= tolerance ||
		   (kline.Low <= levelPrice && kline.High >= levelPrice)
}

// calculateReactionAfterTouch 计算触及后的反应幅度
func (fa *FibonacciAnalyzer) calculateReactionAfterTouch(klines []Kline, touchIndex int, levelPrice float64) float64 {
	if touchIndex >= len(klines)-3 {
		return 0
	}

	// 计算接下来3根K线的最大移动幅度
	maxMove := 0.0
	touchPrice := (klines[touchIndex].High + klines[touchIndex].Low) / 2

	for i := 1; i <= 3 && touchIndex+i < len(klines); i++ {
		nextKline := klines[touchIndex+i]
		moveHigh := math.Abs(nextKline.High-touchPrice) / touchPrice
		moveLow := math.Abs(nextKline.Low-touchPrice) / touchPrice
		
		if moveHigh > maxMove {
			maxMove = moveHigh
		}
		if moveLow > maxMove {
			maxMove = moveLow
		}
	}

	return maxMove
}

// wasLevelBroken 检查级别是否被明显突破
func (fa *FibonacciAnalyzer) wasLevelBroken(levelPrice float64, klines []Kline) bool {
	breakThreshold := 0.02 // 2%的突破阈值

	for _, kline := range klines {
		// 检查是否有明显的突破（收盘价突破超过阈值）
		if math.Abs(kline.Close-levelPrice)/levelPrice > breakThreshold {
			return true
		}
	}

	return false
}

// checkVolumeConfirmation 检查成交量确认
func (fa *FibonacciAnalyzer) checkVolumeConfirmation(retracement *FibRetracement, klines []Kline) bool {
	// 计算趋势形成期间的平均成交量
	startIdx := retracement.StartPoint.Index
	endIdx := retracement.EndPoint.Index

	if startIdx >= endIdx || endIdx >= len(klines) {
		return true // 无法计算，默认通过
	}

	trendVolume := 0.0
	trendLength := endIdx - startIdx

	for i := startIdx; i <= endIdx && i < len(klines); i++ {
		trendVolume += klines[i].Volume
	}

	if trendLength == 0 {
		return true
	}

	avgTrendVolume := trendVolume / float64(trendLength)

	// 计算同期的历史平均成交量
	lookback := 50
	historicalStart := startIdx - lookback
	if historicalStart < 0 {
		historicalStart = 0
	}

	historicalVolume := 0.0
	historicalLength := startIdx - historicalStart

	for i := historicalStart; i < startIdx && i < len(klines); i++ {
		historicalVolume += klines[i].Volume
	}

	if historicalLength == 0 {
		return true
	}

	avgHistoricalVolume := historicalVolume / float64(historicalLength)

	// 要求趋势期间成交量至少是历史平均的80%
	volumeRatio := avgTrendVolume / avgHistoricalVolume
	return volumeRatio >= 0.8
}

// filterKeyFibLevels 筛选关键斐波那契水平线（只保留4个关键位：0.382, 0.5, 0.618, 0.786）
// 🔥 Token优化：减少AI输入数据量，只保留最重要的斐波那契比率
func filterKeyFibLevels(levels []FibLevel) []FibLevel {
	keyRatios := map[float64]bool{
		0.382: true,
		0.5:   true,
		0.618: true,
		0.786: true,
	}

	var filtered []FibLevel
	for _, level := range levels {
		// 使用小的epsilon值来处理浮点数比较
		epsilon := 0.001
		for keyRatio := range keyRatios {
			if math.Abs(level.Ratio-keyRatio) < epsilon {
				filtered = append(filtered, level)
				break
			}
		}
	}

	return filtered
}