package market

import (
	"fmt"
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
	
	// 计算斐波纳契扩展
	extensions := fa.calculateExtensions(swingPoints, klines)
	
	// 识别斐波聚集区
	clusters := fa.identifyFibClusters(retracements, extensions)
	
	// 分析黄金口袋
	goldenPocket := fa.analyzeGoldenPocket(retracements, klines)
	
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

// identifySwingPoints 识别关键摆动点
func (fa *FibonacciAnalyzer) identifySwingPoints(klines []Kline) []PricePoint {
	var swingPoints []PricePoint
	lookback := 5

	for i := lookback; i < len(klines)-lookback; i++ {
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

// calculateRetracements 计算斐波纳契回调
func (fa *FibonacciAnalyzer) calculateRetracements(swingPoints []PricePoint, klines []Kline) []*FibRetracement {
	var retracements []*FibRetracement

	for i := 0; i < len(swingPoints)-1; i++ {
		startPoint := swingPoints[i]
		endPoint := swingPoints[i+1]
		
		// 计算价格变动幅度
		priceMove := abs(endPoint.Price - startPoint.Price)
		priceMovePercent := priceMove / startPoint.Price
		
		// 检查是否满足最小趋势长度要求
		if priceMovePercent < fa.config.MinTrendLength {
			continue
		}
		
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
		
		retracement := &FibRetracement{
			ID:         fmt.Sprintf("fib_ret_%d_%d", startPoint.Index, endPoint.Index),
			StartPoint: startPoint,
			EndPoint:   endPoint,
			TrendType:  trendType,
			Levels:     levels,
			Quality:    quality,
			Strength:   strength,
			Age:        len(klines) - endPoint.Index,
			IsActive:   true,
			TouchCount: touchCount,
			CreatedAt:  time.Now().Unix(),
		}
		
		retracements = append(retracements, retracement)
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

// calculateExtensions 计算斐波纳契扩展
func (fa *FibonacciAnalyzer) calculateExtensions(swingPoints []PricePoint, klines []Kline) []*FibExtension {
	var extensions []*FibExtension

	// 需要至少3个摆动点来计算扩展
	for i := 0; i < len(swingPoints)-2; i++ {
		wave1Start := swingPoints[i]
		wave1End := swingPoints[i+1]
		wave2End := swingPoints[i+2]
		
		baseWave := PriceWave{
			StartPoint: wave1Start,
			EndPoint:   wave1End,
			Length:     abs(wave1End.Price - wave1Start.Price),
			Duration:   wave1End.Timestamp - wave1Start.Timestamp,
		}
		
		returnWave := PriceWave{
			StartPoint: wave1End,
			EndPoint:   wave2End,
			Length:     abs(wave2End.Price - wave1End.Price),
			Duration:   wave2End.Timestamp - wave1End.Timestamp,
		}
		
		// 计算扩展级别
		levels := fa.calculateExtensionLevels(baseWave, returnWave)
		
		// 评估质量
		quality := fa.evaluateExtensionQuality(baseWave, returnWave)
		
		extension := &FibExtension{
			ID:          fmt.Sprintf("fib_ext_%d_%d_%d", wave1Start.Index, wave1End.Index, wave2End.Index),
			BaseWave:    baseWave,
			ReturnWave:  returnWave,
			Levels:      levels,
			Quality:     quality,
			Confidence:  fa.calculateExtensionConfidence(baseWave, returnWave),
			IsProjected: wave2End.Index == len(klines)-1, // 如果是最后一个点，则为预测
		}
		
		extensions = append(extensions, extension)
	}

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
	touchEvents := fa.analyzeTouchEvents(goldenLow, goldenHigh, klines, bestRetracement.EndPoint.Index)
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
		LastUpdate:    time.Now().Unix(),
	}
	
	return goldenPocket
}

// analyzeTouchEvents 分析触及事件
func (fa *FibonacciAnalyzer) analyzeTouchEvents(low, high float64, klines []Kline, startIdx int) []TouchEvent {
	var touchEvents []TouchEvent
	tolerance := fa.config.TouchSensitivity
	
	for i := startIdx + 1; i < len(klines); i++ {
		candle := klines[i]
		
		// 检查是否触及黄金口袋区域
		if (candle.Low <= high*(1+tolerance) && candle.High >= low*(1-tolerance)) {
			// 判断反应类型
			var reactionType ReactionType
			nextIdx := minInt(i+3, len(klines)-1)
			
			if i < len(klines)-3 {
				// 检查后续3根K线的价格行为
				priceAfter := klines[nextIdx].Close
				priceAtTouch := candle.Close
				
				if abs(priceAfter-priceAtTouch)/priceAtTouch > 0.01 {
					if (priceAfter > priceAtTouch && low < high) || (priceAfter < priceAtTouch && low > high) {
						reactionType = ReactionBounce
					} else {
						reactionType = ReactionBreak
					}
				} else {
					reactionType = ReactionConsolidation
				}
			} else {
				reactionType = ReactionConsolidation
			}
			
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
		Timestamp:  time.Now().Unix(),
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