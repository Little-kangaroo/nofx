package market

import (
	"math"
	"sort"
	"time"
)

// DowTheoryAnalyzer 道氏理论分析器
type DowTheoryAnalyzer struct {
	config DowTheoryConfig
}

// NewDowTheoryAnalyzer 创建新的道氏理论分析器
func NewDowTheoryAnalyzer() *DowTheoryAnalyzer {
	return &DowTheoryAnalyzer{
		config: dowConfig,
	}
}

// Analyze 执行完整的道氏理论分析
func (dta *DowTheoryAnalyzer) Analyze(klines3m, klines4h []Kline, currentPrice float64) *DowTheoryData {
	// 使用4小时数据进行主要分析，3分钟数据用于精确入场点
	swingPoints := dta.identifySwingPoints(klines4h)
	trendLines := dta.calculateTrendLines(swingPoints)
	channel := dta.buildParallelChannel(trendLines, swingPoints, currentPrice)
	trendStrength := dta.assessTrendStrength(klines3m, klines4h, swingPoints, trendLines)
	tradingSignal := dta.generateTradingSignal(klines3m, currentPrice, channel, trendStrength, trendLines)

	return &DowTheoryData{
		SwingPoints:   swingPoints,
		TrendLines:    trendLines,
		Channel:       channel,
		TrendStrength: trendStrength,
		TradingSignal: tradingSignal,
	}
}

// identifySwingPoints 识别摆动点
func (dta *DowTheoryAnalyzer) identifySwingPoints(klines []Kline) []*SwingPoint {
	if len(klines) < dta.config.SwingPointConfig.LookbackPeriod*2+1 {
		return nil
	}

	var swingPoints []*SwingPoint
	lookback := dta.config.SwingPointConfig.LookbackPeriod

	for i := lookback; i < len(klines)-lookback; i++ {
		current := klines[i]

		// 检查是否是高点
		if dta.isSwingHigh(klines, i, lookback) {
			strength := dta.calculateSwingPointStrength(klines, i, SwingHigh)
			if strength >= dta.config.SwingPointConfig.MinStrength {
				swingPoint := &SwingPoint{
					Type:      SwingHigh,
					Price:     current.High,
					Time:      current.OpenTime,
					Index:     i,
					Strength:  strength,
					Confirmed: i < len(klines)-dta.config.SwingPointConfig.ConfirmPeriod,
				}
				swingPoints = append(swingPoints, swingPoint)
			}
		}

		// 检查是否是低点
		if dta.isSwingLow(klines, i, lookback) {
			strength := dta.calculateSwingPointStrength(klines, i, SwingLow)
			if strength >= dta.config.SwingPointConfig.MinStrength {
				swingPoint := &SwingPoint{
					Type:      SwingLow,
					Price:     current.Low,
					Time:      current.OpenTime,
					Index:     i,
					Strength:  strength,
					Confirmed: i < len(klines)-dta.config.SwingPointConfig.ConfirmPeriod,
				}
				swingPoints = append(swingPoints, swingPoint)
			}
		}
	}

	return swingPoints
}

// isSwingHigh 判断是否为摆动高点
func (dta *DowTheoryAnalyzer) isSwingHigh(klines []Kline, index, lookback int) bool {
	currentHigh := klines[index].High

	// 检查左侧
	for i := index - lookback; i < index; i++ {
		if klines[i].High >= currentHigh {
			return false
		}
	}

	// 检查右侧
	for i := index + 1; i <= index+lookback; i++ {
		if klines[i].High >= currentHigh {
			return false
		}
	}

	return true
}

// isSwingLow 判断是否为摆动低点
func (dta *DowTheoryAnalyzer) isSwingLow(klines []Kline, index, lookback int) bool {
	currentLow := klines[index].Low

	// 检查左侧
	for i := index - lookback; i < index; i++ {
		if klines[i].Low <= currentLow {
			return false
		}
	}

	// 检查右侧
	for i := index + 1; i <= index+lookback; i++ {
		if klines[i].Low <= currentLow {
			return false
		}
	}

	return true
}

// calculateSwingPointStrength 计算摆动点强度
func (dta *DowTheoryAnalyzer) calculateSwingPointStrength(klines []Kline, index int, swingType SwingType) float64 {
	if index < 1 || index >= len(klines) {
		return 0
	}

	var priceRange, volumeWeight float64

	if swingType == SwingHigh {
		// 计算高点的价格范围和成交量权重
		priceRange = (klines[index].High - klines[index].Low) / klines[index].Low

		// 向前向后各看几个周期计算相对高度
		maxRange := 10
		start := index - maxRange
		if start < 0 {
			start = 0
		}
		end := index + maxRange
		if end >= len(klines) {
			end = len(klines) - 1
		}

		var maxHigh, minLow float64
		for i := start; i <= end; i++ {
			if i == start || klines[i].High > maxHigh {
				maxHigh = klines[i].High
			}
			if i == start || klines[i].Low < minLow {
				minLow = klines[i].Low
			}
		}

		if maxHigh > minLow {
			priceRange = (klines[index].High - minLow) / (maxHigh - minLow)
		}
	} else {
		// 计算低点的价格范围和成交量权重
		priceRange = (klines[index].High - klines[index].Low) / klines[index].Low

		maxRange := 10
		start := index - maxRange
		if start < 0 {
			start = 0
		}
		end := index + maxRange
		if end >= len(klines) {
			end = len(klines) - 1
		}

		var maxHigh, minLow float64
		for i := start; i <= end; i++ {
			if i == start || klines[i].High > maxHigh {
				maxHigh = klines[i].High
			}
			if i == start || klines[i].Low < minLow {
				minLow = klines[i].Low
			}
		}

		if maxHigh > minLow {
			priceRange = (maxHigh - klines[index].Low) / (maxHigh - minLow)
		}
	}

	// 计算成交量权重（相对于平均成交量）
	volumeSum := 0.0
	volumeCount := 0
	start := index - 20
	if start < 0 {
		start = 0
	}

	for i := start; i < index+20 && i < len(klines); i++ {
		volumeSum += klines[i].Volume
		volumeCount++
	}

	if volumeCount > 0 {
		avgVolume := volumeSum / float64(volumeCount)
		if avgVolume > 0 {
			volumeWeight = klines[index].Volume / avgVolume
		}
	}

	// 综合计算强度
	strength := priceRange*0.7 + math.Min(volumeWeight, 2.0)*0.3
	return math.Min(strength, 10.0) // 限制最大强度
}

// calculateTrendLines 计算趋势线
func (dta *DowTheoryAnalyzer) calculateTrendLines(swingPoints []*SwingPoint) []*TrendLine {
	if len(swingPoints) < 2 {
		return nil
	}

	var trendLines []*TrendLine

	// 分离高点和低点
	var highs, lows []*SwingPoint
	for _, point := range swingPoints {
		if point.Type == SwingHigh {
			highs = append(highs, point)
		} else {
			lows = append(lows, point)
		}
	}

	// 计算阻力线（连接高点）
	resistanceLines := dta.findTrendLinesFromPoints(highs, ResistanceLine)
	trendLines = append(trendLines, resistanceLines...)

	// 计算支撑线（连接低点）
	supportLines := dta.findTrendLinesFromPoints(lows, SupportLine)
	trendLines = append(trendLines, supportLines...)

	// 按强度排序
	sort.Slice(trendLines, func(i, j int) bool {
		return trendLines[i].Strength > trendLines[j].Strength
	})

	// 只保留最强的趋势线
	maxLines := 10
	if len(trendLines) > maxLines {
		trendLines = trendLines[:maxLines]
	}

	return trendLines
}

// findTrendLinesFromPoints 从摆动点中找到趋势线
func (dta *DowTheoryAnalyzer) findTrendLinesFromPoints(points []*SwingPoint, lineType TrendLineType) []*TrendLine {
	if len(points) < 2 {
		return nil
	}

	var trendLines []*TrendLine

	// 尝试连接每对点形成趋势线
	for i := 0; i < len(points)-1; i++ {
		for j := i + 1; j < len(points); j++ {
			point1 := points[i]
			point2 := points[j]

			// 计算趋势线参数
			slope := (point2.Price - point1.Price) / float64(point2.Time-point1.Time)
			intercept := point1.Price - slope*float64(point1.Time)

			// 检查斜率是否满足要求
			if math.Abs(slope) < dta.config.TrendLineConfig.MinSlope {
				continue
			}

			trendLine := &TrendLine{
				Type:      lineType,
				Points:    []*SwingPoint{point1, point2},
				Slope:     slope,
				Intercept: intercept,
				Touches:   2,
				LastTouch: point2.Time,
			}

			// 计算趋势线强度
			trendLine.Strength = dta.calculateTrendLineStrength(trendLine, points)

			// 检查是否有足够的触及点
			touches := dta.countTrendLineTouches(trendLine, points)
			if touches >= dta.config.TrendLineConfig.MinTouches {
				trendLine.Touches = touches
				trendLines = append(trendLines, trendLine)
			}
		}
	}

	return trendLines
}

// calculateTrendLineStrength 计算趋势线强度
func (dta *DowTheoryAnalyzer) calculateTrendLineStrength(trendLine *TrendLine, allPoints []*SwingPoint) float64 {
	strength := 0.0

	// 基础强度：触及次数
	strength += float64(trendLine.Touches) * 1.0

	// 时间跨度加分
	if len(trendLine.Points) >= 2 {
		timeSpan := float64(trendLine.Points[len(trendLine.Points)-1].Time - trendLine.Points[0].Time)
		timeSpanDays := timeSpan / (24 * 3600 * 1000) // 转换为天数
		strength += math.Min(timeSpanDays/10, 2.0)    // 最多加2分
	}

	// 摆动点强度加权
	pointStrengthSum := 0.0
	for _, point := range trendLine.Points {
		pointStrengthSum += point.Strength
	}
	if len(trendLine.Points) > 0 {
		strength += (pointStrengthSum / float64(len(trendLine.Points))) * 0.5
	}

	// 角度适中加分（不要太陡峭也不要太平）
	angle := math.Atan(math.Abs(trendLine.Slope)) * 180 / math.Pi
	if angle > 15 && angle < 75 {
		strength += 0.5
	}

	return strength
}

// countTrendLineTouches 计算趋势线的触及次数
func (dta *DowTheoryAnalyzer) countTrendLineTouches(trendLine *TrendLine, points []*SwingPoint) int {
	touches := 0
	maxDistance := dta.config.TrendLineConfig.MaxDistance

	for _, point := range points {
		// 计算点到趋势线的距离
		expectedPrice := trendLine.Slope*float64(point.Time) + trendLine.Intercept
		distance := math.Abs(point.Price-expectedPrice) / point.Price

		if distance <= maxDistance {
			touches++
		}
	}

	return touches
}

// buildParallelChannel 构建平行通道
func (dta *DowTheoryAnalyzer) buildParallelChannel(trendLines []*TrendLine, swingPoints []*SwingPoint, currentPrice float64) *ParallelChannel {
	if len(trendLines) < 1 {
		return nil
	}

	// 找到最强的趋势线作为主趋势线
	mainTrendLine := trendLines[0]

	// 寻找平行的趋势线
	var parallelLine *TrendLine
	for _, line := range trendLines[1:] {
		if dta.areParallel(mainTrendLine, line) {
			parallelLine = line
			break
		}
	}

	if parallelLine == nil {
		// 如果没有找到平行线，尝试构建一条
		parallelLine = dta.constructParallelLine(mainTrendLine, swingPoints)
	}

	if parallelLine == nil {
		return nil
	}

	// 确定上轨和下轨
	var upperLine, lowerLine *TrendLine
	if mainTrendLine.Type == SupportLine {
		lowerLine = mainTrendLine
		upperLine = parallelLine
	} else {
		upperLine = mainTrendLine
		lowerLine = parallelLine
	}

	// 计算中轨
	middleLine := dta.createMiddleLine(upperLine, lowerLine)

	// 计算通道宽度
	currentTime := time.Now().UnixMilli()
	upperPrice := upperLine.Slope*float64(currentTime) + upperLine.Intercept
	lowerPrice := lowerLine.Slope*float64(currentTime) + lowerLine.Intercept
	width := math.Abs(upperPrice-lowerPrice) / currentPrice

	// 检查宽度是否合理
	if width < dta.config.ChannelConfig.MinWidth || width > dta.config.ChannelConfig.MaxWidth {
		return nil
	}

	// 计算通道方向
	direction := TrendFlat
	if upperLine.Slope > 0.001 {
		direction = TrendUp
	} else if upperLine.Slope < -0.001 {
		direction = TrendDown
	}

	// 计算质量评分
	quality := dta.calculateChannelQuality(upperLine, lowerLine, swingPoints)

	// 计算当前价格位置
	currentPos, priceRatio := dta.calculateCurrentPosition(currentPrice, upperPrice, lowerPrice)

	return &ParallelChannel{
		UpperLine:  upperLine,
		LowerLine:  lowerLine,
		MiddleLine: middleLine,
		Width:      width,
		Direction:  direction,
		Quality:    quality,
		CurrentPos: currentPos,
		PriceRatio: priceRatio,
	}
}

// areParallel 判断两条趋势线是否平行
func (dta *DowTheoryAnalyzer) areParallel(line1, line2 *TrendLine) bool {
	slopeDiff := math.Abs(line1.Slope - line2.Slope)
	tolerance := dta.config.ChannelConfig.ParallelTolerance

	// 计算相对于斜率的容忍度
	avgSlope := (math.Abs(line1.Slope) + math.Abs(line2.Slope)) / 2
	if avgSlope == 0 {
		return slopeDiff < tolerance
	}

	return slopeDiff/avgSlope < tolerance
}

// constructParallelLine 构建平行线
func (dta *DowTheoryAnalyzer) constructParallelLine(mainLine *TrendLine, swingPoints []*SwingPoint) *TrendLine {
	// 寻找与主趋势线平行且距离合适的点
	var candidatePoints []*SwingPoint
	targetType := SwingHigh
	if mainLine.Type == SupportLine {
		targetType = SwingHigh // 支撑线对应阻力线
	} else {
		targetType = SwingLow // 阻力线对应支撑线
	}

	for _, point := range swingPoints {
		if point.Type == targetType {
			candidatePoints = append(candidatePoints, point)
		}
	}

	if len(candidatePoints) < 2 {
		return nil
	}

	// 找到距离主趋势线最远且形成有效平行线的点组合
	bestDistance := 0.0
	var bestLine *TrendLine

	for i := 0; i < len(candidatePoints)-1; i++ {
		for j := i + 1; j < len(candidatePoints); j++ {
			point1 := candidatePoints[i]
			point2 := candidatePoints[j]

			slope := (point2.Price - point1.Price) / float64(point2.Time-point1.Time)
			intercept := point1.Price - slope*float64(point1.Time)

			testLine := &TrendLine{
				Type:      ResistanceLine,
				Points:    []*SwingPoint{point1, point2},
				Slope:     slope,
				Intercept: intercept,
			}

			if mainLine.Type == ResistanceLine {
				testLine.Type = SupportLine
			}

			if dta.areParallel(mainLine, testLine) {
				// 计算平均距离
				avgDistance := dta.calculateAverageDistance(mainLine, testLine)
				if avgDistance > bestDistance {
					bestDistance = avgDistance
					bestLine = testLine
				}
			}
		}
	}

	return bestLine
}

// calculateAverageDistance 计算两条趋势线的平均距离
func (dta *DowTheoryAnalyzer) calculateAverageDistance(line1, line2 *TrendLine) float64 {
	if len(line1.Points) == 0 || len(line2.Points) == 0 {
		return 0
	}

	totalDistance := 0.0
	count := 0

	// 在多个时间点计算距离
	for _, point := range line1.Points {
		price1 := line1.Slope*float64(point.Time) + line1.Intercept
		price2 := line2.Slope*float64(point.Time) + line2.Intercept
		distance := math.Abs(price1 - price2)
		totalDistance += distance
		count++
	}

	for _, point := range line2.Points {
		price1 := line1.Slope*float64(point.Time) + line1.Intercept
		price2 := line2.Slope*float64(point.Time) + line2.Intercept
		distance := math.Abs(price1 - price2)
		totalDistance += distance
		count++
	}

	if count == 0 {
		return 0
	}

	return totalDistance / float64(count)
}

// createMiddleLine 创建中轨
func (dta *DowTheoryAnalyzer) createMiddleLine(upperLine, lowerLine *TrendLine) *TrendLine {
	// 中轨的斜率是上下轨斜率的平均值
	avgSlope := (upperLine.Slope + lowerLine.Slope) / 2

	// 中轨的截距是上下轨截距的平均值
	avgIntercept := (upperLine.Intercept + lowerLine.Intercept) / 2

	return &TrendLine{
		Type:      SupportLine, // 中轨可视为动态支撑
		Slope:     avgSlope,
		Intercept: avgIntercept,
		Strength:  (upperLine.Strength + lowerLine.Strength) / 2,
		Points:    []*SwingPoint{}, // 中轨是计算得出的，不基于具体摆动点
	}
}

// calculateChannelQuality 计算通道质量
func (dta *DowTheoryAnalyzer) calculateChannelQuality(upperLine, lowerLine *TrendLine, swingPoints []*SwingPoint) float64 {
	quality := 0.0

	// 基于趋势线强度
	quality += (upperLine.Strength + lowerLine.Strength) / 2 * 0.3

	// 基于平行度
	parallelScore := 1.0 - math.Abs(upperLine.Slope-lowerLine.Slope)/math.Max(math.Abs(upperLine.Slope), math.Abs(lowerLine.Slope))
	quality += parallelScore * 0.2

	// 基于触及次数
	totalTouches := upperLine.Touches + lowerLine.Touches
	quality += math.Min(float64(totalTouches)/10, 1.0) * 0.3

	// 基于时间跨度
	timeSpan := math.Max(
		float64(upperLine.LastTouch-upperLine.Points[0].Time),
		float64(lowerLine.LastTouch-lowerLine.Points[0].Time),
	)
	timeSpanDays := timeSpan / (24 * 3600 * 1000)
	quality += math.Min(timeSpanDays/30, 1.0) * 0.2

	return math.Min(quality, 1.0)
}

// calculateCurrentPosition 计算当前价格在通道中的位置
func (dta *DowTheoryAnalyzer) calculateCurrentPosition(currentPrice, upperPrice, lowerPrice float64) (ChannelPosition, float64) {
	if currentPrice > upperPrice*1.01 {
		return ChannelBreak, 1.0
	}

	if currentPrice < lowerPrice*0.99 {
		return ChannelBreak, 0.0
	}

	// 计算价格在通道中的比例
	ratio := (currentPrice - lowerPrice) / (upperPrice - lowerPrice)
	ratio = math.Max(0, math.Min(1, ratio))

	var position ChannelPosition
	if ratio > 0.7 {
		position = ChannelUpper
	} else if ratio < 0.3 {
		position = ChannelLower
	} else {
		position = ChannelMiddle
	}

	return position, ratio
}

// assessTrendStrength 评估趋势强度
func (dta *DowTheoryAnalyzer) assessTrendStrength(klines3m, klines4h []Kline, swingPoints []*SwingPoint, trendLines []*TrendLine) *TrendStrength {
	if len(klines4h) < 20 {
		return &TrendStrength{
			Overall:   0,
			Direction: TrendFlat,
			Quality:   TrendWeak,
		}
	}

	// 计算短期趋势强度（基于3分钟数据）
	shortTerm := dta.calculateShortTermStrength(klines3m)

	// 计算长期趋势强度（基于4小时数据）
	longTerm := dta.calculateLongTermStrength(klines4h)

	// 计算整体趋势强度
	overall := (shortTerm*0.3 + longTerm*0.7)

	// 确定趋势方向
	direction := dta.determineTrendDirection(klines4h, swingPoints)

	// 计算动量强度
	momentum := dta.calculateMomentum(klines4h)

	// 计算一致性评分
	consistency := dta.calculateConsistency(klines3m, klines4h)

	// 计算成交量支撑度
	volumeSupport := dta.calculateVolumeSupport(klines4h)

	// 确定趋势质量
	quality := dta.determineTrendQuality(overall, consistency, volumeSupport)

	return &TrendStrength{
		Overall:       overall,
		ShortTerm:     shortTerm,
		LongTerm:      longTerm,
		Direction:     direction,
		Quality:       quality,
		Momentum:      momentum,
		Consistency:   consistency,
		VolumeSupport: volumeSupport,
	}
}

// calculateShortTermStrength 计算短期趋势强度
func (dta *DowTheoryAnalyzer) calculateShortTermStrength(klines []Kline) float64 {
	if len(klines) < 20 {
		return 0
	}

	// 使用最近20个3分钟K线
	recentKlines := klines[len(klines)-20:]

	// 计算价格动量
	priceChange := (recentKlines[len(recentKlines)-1].Close - recentKlines[0].Open) / recentKlines[0].Open

	// 计算移动平均趋势
	ma5 := dta.calculateMA(recentKlines, 5)
	ma10 := dta.calculateMA(recentKlines, 10)
	ma20 := dta.calculateMA(recentKlines, 20)

	maTrend := 0.0
	if ma5 > ma10 && ma10 > ma20 {
		maTrend = 1.0 // 多头排列
	} else if ma5 < ma10 && ma10 < ma20 {
		maTrend = -1.0 // 空头排列
	}

	// 计算波动性
	volatility := dta.calculateVolatility(recentKlines)

	// 综合计算短期强度
	strength := math.Abs(priceChange)*50 + math.Abs(maTrend)*30 + (1-volatility)*20

	return math.Min(strength, 100.0)
}

// calculateLongTermStrength 计算长期趋势强度
func (dta *DowTheoryAnalyzer) calculateLongTermStrength(klines []Kline) float64 {
	if len(klines) < 50 {
		return 0
	}

	// 计算长期价格趋势
	periodLength := 30
	if len(klines) < periodLength {
		periodLength = len(klines)
	}

	recentKlines := klines[len(klines)-periodLength:]

	// 计算趋势斜率
	prices := make([]float64, len(recentKlines))
	for i, k := range recentKlines {
		prices[i] = k.Close
	}

	slope := dta.calculateTrendSlope(prices)

	// 计算R-squared（趋势的线性度）
	rSquared := dta.calculateRSquared(prices)

	// 计算价格相对于移动平均线的位置
	ma20 := dta.calculateMA(recentKlines, 20)
	ma50 := dta.calculateMA(klines, 50)
	currentPrice := recentKlines[len(recentKlines)-1].Close

	maPosition := 0.0
	if currentPrice > ma20 && ma20 > ma50 {
		maPosition = 1.0
	} else if currentPrice < ma20 && ma20 < ma50 {
		maPosition = 1.0
	}

	// 综合计算长期强度
	strength := math.Abs(slope)*40 + rSquared*40 + maPosition*20

	return math.Min(strength, 100.0)
}

// calculateMA 计算简单移动平均
func (dta *DowTheoryAnalyzer) calculateMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}

	sum := 0.0
	start := len(klines) - period
	for i := start; i < len(klines); i++ {
		sum += klines[i].Close
	}

	return sum / float64(period)
}

// calculateVolatility 计算波动率
func (dta *DowTheoryAnalyzer) calculateVolatility(klines []Kline) float64 {
	if len(klines) < 2 {
		return 0
	}

	changes := make([]float64, len(klines)-1)
	for i := 1; i < len(klines); i++ {
		changes[i-1] = (klines[i].Close - klines[i-1].Close) / klines[i-1].Close
	}

	// 计算标准差
	mean := 0.0
	for _, change := range changes {
		mean += change
	}
	mean /= float64(len(changes))

	variance := 0.0
	for _, change := range changes {
		variance += math.Pow(change-mean, 2)
	}
	variance /= float64(len(changes))

	return math.Sqrt(variance)
}

// calculateTrendSlope 计算趋势斜率
func (dta *DowTheoryAnalyzer) calculateTrendSlope(prices []float64) float64 {
	n := float64(len(prices))
	if n < 2 {
		return 0
	}

	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0

	for i, price := range prices {
		x := float64(i)
		sumX += x
		sumY += price
		sumXY += x * price
		sumX2 += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	return slope
}

// calculateRSquared 计算R平方
func (dta *DowTheoryAnalyzer) calculateRSquared(prices []float64) float64 {
	n := float64(len(prices))
	if n < 2 {
		return 0
	}

	slope := dta.calculateTrendSlope(prices)

	// 计算平均值
	meanY := 0.0
	for _, price := range prices {
		meanY += price
	}
	meanY /= n

	// 计算截距
	meanX := (n - 1) / 2
	intercept := meanY - slope*meanX

	// 计算总平方和和残差平方和
	totalSS := 0.0
	residualSS := 0.0

	for i, actual := range prices {
		predicted := slope*float64(i) + intercept
		totalSS += math.Pow(actual-meanY, 2)
		residualSS += math.Pow(actual-predicted, 2)
	}

	if totalSS == 0 {
		return 0
	}

	rSquared := 1 - (residualSS / totalSS)
	return math.Max(0, math.Min(1, rSquared))
}

// determineTrendDirection 确定趋势方向
func (dta *DowTheoryAnalyzer) determineTrendDirection(klines []Kline, swingPoints []*SwingPoint) TrendDirection {
	if len(klines) < 10 {
		return TrendFlat
	}

	// 基于价格的整体方向
	recentKlines := klines[len(klines)-10:]
	priceDirection := (recentKlines[len(recentKlines)-1].Close - recentKlines[0].Open) / recentKlines[0].Open

	// 基于摆动点的方向
	swingDirection := 0.0
	if len(swingPoints) >= 4 {
		recentSwings := swingPoints[len(swingPoints)-4:]

		var recentHighs, recentLows []*SwingPoint
		for _, swing := range recentSwings {
			if swing.Type == SwingHigh {
				recentHighs = append(recentHighs, swing)
			} else {
				recentLows = append(recentLows, swing)
			}
		}

		if len(recentHighs) >= 2 {
			if recentHighs[len(recentHighs)-1].Price > recentHighs[0].Price {
				swingDirection += 0.5
			} else {
				swingDirection -= 0.5
			}
		}

		if len(recentLows) >= 2 {
			if recentLows[len(recentLows)-1].Price > recentLows[0].Price {
				swingDirection += 0.5
			} else {
				swingDirection -= 0.5
			}
		}
	}

	// 综合判断
	overallDirection := priceDirection*0.6 + swingDirection*0.4

	if overallDirection > 0.02 {
		return TrendUp
	} else if overallDirection < -0.02 {
		return TrendDown
	}

	return TrendFlat
}

// calculateMomentum 计算动量
func (dta *DowTheoryAnalyzer) calculateMomentum(klines []Kline) float64 {
	if len(klines) < 10 {
		return 0
	}

	// 计算ROC (Rate of Change)
	period := 10
	current := klines[len(klines)-1].Close
	past := klines[len(klines)-1-period].Close
	roc := (current - past) / past

	// 计算RSI
	rsi := calculateRSI(klines, 14)

	// 计算MACD
	macd := calculateMACD(klines)

	// 综合动量评分
	momentum := math.Abs(roc)*30 + math.Abs(rsi-50)*1.4 + math.Abs(macd)*20

	return math.Min(momentum, 100.0)
}

// calculateConsistency 计算一致性
func (dta *DowTheoryAnalyzer) calculateConsistency(klines3m, klines4h []Kline) float64 {
	if len(klines3m) < 20 || len(klines4h) < 5 {
		return 0
	}

	// 短期趋势方向
	shortTrend := (klines3m[len(klines3m)-1].Close - klines3m[len(klines3m)-20].Close) / klines3m[len(klines3m)-20].Close

	// 长期趋势方向
	longTrend := (klines4h[len(klines4h)-1].Close - klines4h[len(klines4h)-5].Close) / klines4h[len(klines4h)-5].Close

	// 计算一致性
	consistency := 0.0
	if (shortTrend > 0 && longTrend > 0) || (shortTrend < 0 && longTrend < 0) {
		// 趋势方向一致
		consistency = 100.0 - math.Abs(shortTrend-longTrend)*100
	} else {
		// 趋势方向不一致
		consistency = 100.0 - (math.Abs(shortTrend)+math.Abs(longTrend))*100
	}

	return math.Max(0, math.Min(100, consistency))
}

// calculateVolumeSupport 计算成交量支撑度
func (dta *DowTheoryAnalyzer) calculateVolumeSupport(klines []Kline) float64 {
	if len(klines) < 20 {
		return 0
	}

	// 计算最近成交量相对于历史平均的比值
	recentVolumes := klines[len(klines)-5:]
	historicalVolumes := klines[len(klines)-20 : len(klines)-5]

	recentAvg := 0.0
	for _, k := range recentVolumes {
		recentAvg += k.Volume
	}
	recentAvg /= float64(len(recentVolumes))

	historicalAvg := 0.0
	for _, k := range historicalVolumes {
		historicalAvg += k.Volume
	}
	historicalAvg /= float64(len(historicalVolumes))

	if historicalAvg == 0 {
		return 0
	}

	volumeRatio := recentAvg / historicalAvg

	// 计算支撑度评分
	support := 0.0
	if volumeRatio > 1.5 {
		support = 100.0 // 强支撑
	} else if volumeRatio > 1.2 {
		support = 75.0 // 较强支撑
	} else if volumeRatio > 0.8 {
		support = 50.0 // 一般支撑
	} else {
		support = 25.0 // 弱支撑
	}

	return support
}

// determineTrendQuality 确定趋势质量
func (dta *DowTheoryAnalyzer) determineTrendQuality(overall, consistency, volumeSupport float64) TrendQuality {
	score := (overall + consistency + volumeSupport) / 3

	if score > 75 {
		return TrendStrong
	} else if score > 50 {
		return TrendModerate
	}

	return TrendWeak
}

// generateTradingSignal 生成交易信号
func (dta *DowTheoryAnalyzer) generateTradingSignal(klines3m []Kline, currentPrice float64, channel *ParallelChannel,
	trendStrength *TrendStrength, trendLines []*TrendLine) *TradingSignal {

	if len(klines3m) == 0 || trendStrength == nil {
		return &TradingSignal{
			Action:      ActionHold,
			Confidence:  0,
			Description: "数据不足，无法生成信号",
			Timestamp:   time.Now().UnixMilli(),
		}
	}

	// 检查通道信号
	if channel != nil && channel.Quality > dta.config.ChannelConfig.QualityThreshold {
		signal := dta.generateChannelSignal(currentPrice, channel, trendStrength)
		if signal != nil && signal.Confidence >= dta.config.SignalConfig.MinConfidence {
			return signal
		}
	}

	// 检查突破信号
	breakoutSignal := dta.generateBreakoutSignal(klines3m, currentPrice, trendLines, trendStrength)
	if breakoutSignal != nil && breakoutSignal.Confidence >= dta.config.SignalConfig.MinConfidence {
		return breakoutSignal
	}

	// 检查趋势跟随信号
	trendSignal := dta.generateTrendFollowingSignal(currentPrice, trendStrength, channel)
	if trendSignal != nil && trendSignal.Confidence >= dta.config.SignalConfig.MinConfidence {
		return trendSignal
	}

	// 默认持有信号
	return &TradingSignal{
		Action:      ActionHold,
		Confidence:  50,
		Description: "当前无明确交易机会，建议持有观望",
		Timestamp:   time.Now().UnixMilli(),
	}
}

// generateChannelSignal 生成基于通道的信号
func (dta *DowTheoryAnalyzer) generateChannelSignal(currentPrice float64, channel *ParallelChannel,
	trendStrength *TrendStrength) *TradingSignal {

	var signal *TradingSignal
	currentTime := time.Now().UnixMilli()

	// 获取通道边界价格
	upperPrice := channel.UpperLine.Slope*float64(currentTime) + channel.UpperLine.Intercept
	lowerPrice := channel.LowerLine.Slope*float64(currentTime) + channel.LowerLine.Intercept
	middlePrice := channel.MiddleLine.Slope*float64(currentTime) + channel.MiddleLine.Intercept

	confidence := channel.Quality * 100

	switch channel.CurrentPos {
	case ChannelLower:
		// 在下轨附近，考虑买入
		if channel.Direction == TrendUp || (channel.Direction == TrendFlat && trendStrength.Overall > 60) {
			signal = &TradingSignal{
				Type:         SignalChannelBounce,
				Action:       ActionBuy,
				Confidence:   confidence,
				Entry:        currentPrice,
				StopLoss:     lowerPrice * 0.99,
				TakeProfit:   middlePrice,
				Description:  "通道下轨支撑，建议买入",
				ChannelBased: true,
			}
		}

	case ChannelUpper:
		// 在上轨附近，考虑卖出
		if channel.Direction == TrendDown || (channel.Direction == TrendFlat && trendStrength.Overall < 40) {
			signal = &TradingSignal{
				Type:         SignalChannelBounce,
				Action:       ActionSell,
				Confidence:   confidence,
				Entry:        currentPrice,
				StopLoss:     upperPrice * 1.01,
				TakeProfit:   middlePrice,
				Description:  "通道上轨阻力，建议卖出",
				ChannelBased: true,
			}
		}

	case ChannelBreak:
		// 突破通道
		if currentPrice > upperPrice*1.01 && channel.Direction == TrendUp {
			signal = &TradingSignal{
				Type:          SignalChannelBreakout,
				Action:        ActionBuy,
				Confidence:    confidence * 0.9,
				Entry:         currentPrice,
				StopLoss:      upperPrice,
				TakeProfit:    currentPrice * 1.05,
				Description:   "向上突破通道，建议买入",
				ChannelBased:  true,
				BreakoutBased: true,
			}
		} else if currentPrice < lowerPrice*0.99 && channel.Direction == TrendDown {
			signal = &TradingSignal{
				Type:          SignalChannelBreakout,
				Action:        ActionSell,
				Confidence:    confidence * 0.9,
				Entry:         currentPrice,
				StopLoss:      lowerPrice,
				TakeProfit:    currentPrice * 0.95,
				Description:   "向下突破通道，建议卖出",
				ChannelBased:  true,
				BreakoutBased: true,
			}
		}
	}

	if signal != nil {
		signal.Timestamp = currentTime
		signal.RiskReward = dta.calculateRiskReward(signal)

		// 检查风险收益比
		if signal.RiskReward < dta.config.SignalConfig.RiskRewardMin {
			signal.Confidence *= 0.7 // 降低置信度
		}
	}

	return signal
}

// generateBreakoutSignal 生成突破信号
func (dta *DowTheoryAnalyzer) generateBreakoutSignal(klines []Kline, currentPrice float64,
	trendLines []*TrendLine, trendStrength *TrendStrength) *TradingSignal {

	if len(trendLines) == 0 || len(klines) < 5 {
		return nil
	}

	currentTime := time.Now().UnixMilli()

	// 检查是否突破重要趋势线
	for _, line := range trendLines {
		if line.Strength < 3.0 { // 只考虑强趋势线
			continue
		}

		expectedPrice := line.Slope*float64(currentTime) + line.Intercept
		breakoutStrength := math.Abs(currentPrice-expectedPrice) / expectedPrice

		if breakoutStrength > dta.config.SignalConfig.BreakoutStrength {
			var signal *TradingSignal

			if line.Type == SupportLine && currentPrice < expectedPrice*0.99 {
				// 突破支撑线向下
				signal = &TradingSignal{
					Type:          SignalChannelBreakout,
					Action:        ActionSell,
					Confidence:    line.Strength * 15,
					Entry:         currentPrice,
					StopLoss:      expectedPrice,
					TakeProfit:    currentPrice * 0.97,
					Description:   "突破重要支撑线，建议卖出",
					BreakoutBased: true,
					Timestamp:     currentTime,
				}
			} else if line.Type == ResistanceLine && currentPrice > expectedPrice*1.01 {
				// 突破阻力线向上
				signal = &TradingSignal{
					Type:          SignalChannelBreakout,
					Action:        ActionBuy,
					Confidence:    line.Strength * 15,
					Entry:         currentPrice,
					StopLoss:      expectedPrice,
					TakeProfit:    currentPrice * 1.03,
					Description:   "突破重要阻力线，建议买入",
					BreakoutBased: true,
					Timestamp:     currentTime,
				}
			}

			if signal != nil {
				signal.RiskReward = dta.calculateRiskReward(signal)

				// 成交量确认
				if dta.config.SignalConfig.VolumeConfirmation {
					volumeConfirm := dta.confirmWithVolume(klines)
					signal.Confidence *= volumeConfirm
				}

				return signal
			}
		}
	}

	return nil
}

// generateTrendFollowingSignal 生成趋势跟随信号
func (dta *DowTheoryAnalyzer) generateTrendFollowingSignal(currentPrice float64,
	trendStrength *TrendStrength, channel *ParallelChannel) *TradingSignal {

	if trendStrength.Quality != TrendStrong || trendStrength.Overall < 70 {
		return nil
	}

	var signal *TradingSignal
	confidence := trendStrength.Overall * 0.8

	if trendStrength.Direction == TrendUp && trendStrength.Consistency > 70 {
		stopLoss := currentPrice * 0.97
		takeProfit := currentPrice * 1.05

		if channel != nil {
			middlePrice := channel.MiddleLine.Slope*float64(time.Now().UnixMilli()) + channel.MiddleLine.Intercept
			if currentPrice < middlePrice*1.02 { // 在中轨附近或下方
				signal = &TradingSignal{
					Type:        SignalTrendFollowing,
					Action:      ActionBuy,
					Confidence:  confidence,
					Entry:       currentPrice,
					StopLoss:    stopLoss,
					TakeProfit:  takeProfit,
					Description: "强势上涨趋势，建议买入",
					Timestamp:   time.Now().UnixMilli(),
				}
			}
		} else {
			signal = &TradingSignal{
				Type:        SignalTrendFollowing,
				Action:      ActionBuy,
				Confidence:  confidence,
				Entry:       currentPrice,
				StopLoss:    stopLoss,
				TakeProfit:  takeProfit,
				Description: "强势上涨趋势，建议买入",
				Timestamp:   time.Now().UnixMilli(),
			}
		}
	} else if trendStrength.Direction == TrendDown && trendStrength.Consistency > 70 {
		stopLoss := currentPrice * 1.03
		takeProfit := currentPrice * 0.95

		if channel != nil {
			middlePrice := channel.MiddleLine.Slope*float64(time.Now().UnixMilli()) + channel.MiddleLine.Intercept
			if currentPrice > middlePrice*0.98 { // 在中轨附近或上方
				signal = &TradingSignal{
					Type:        SignalTrendFollowing,
					Action:      ActionSell,
					Confidence:  confidence,
					Entry:       currentPrice,
					StopLoss:    stopLoss,
					TakeProfit:  takeProfit,
					Description: "强势下跌趋势，建议卖出",
					Timestamp:   time.Now().UnixMilli(),
				}
			}
		} else {
			signal = &TradingSignal{
				Type:        SignalTrendFollowing,
				Action:      ActionSell,
				Confidence:  confidence,
				Entry:       currentPrice,
				StopLoss:    stopLoss,
				TakeProfit:  takeProfit,
				Description: "强势下跌趋势，建议卖出",
				Timestamp:   time.Now().UnixMilli(),
			}
		}
	}

	if signal != nil {
		signal.RiskReward = dta.calculateRiskReward(signal)
	}

	return signal
}

// calculateRiskReward 计算风险收益比
func (dta *DowTheoryAnalyzer) calculateRiskReward(signal *TradingSignal) float64 {
	if signal.Entry == 0 || signal.StopLoss == 0 || signal.TakeProfit == 0 {
		return 0
	}

	var risk, reward float64

	if signal.Action == ActionBuy {
		risk = signal.Entry - signal.StopLoss
		reward = signal.TakeProfit - signal.Entry
	} else {
		risk = signal.StopLoss - signal.Entry
		reward = signal.Entry - signal.TakeProfit
	}

	if risk <= 0 {
		return 0
	}

	return reward / risk
}

// confirmWithVolume 通过成交量确认信号
func (dta *DowTheoryAnalyzer) confirmWithVolume(klines []Kline) float64 {
	if len(klines) < 10 {
		return 0.8 // 默认确认度
	}

	// 计算最近成交量相对于平均成交量的倍数
	recentVolume := klines[len(klines)-1].Volume

	avgVolume := 0.0
	lookback := 10
	start := len(klines) - lookback - 1
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines)-1; i++ {
		avgVolume += klines[i].Volume
	}
	avgVolume /= float64(len(klines) - 1 - start)

	if avgVolume == 0 {
		return 0.8
	}

	volumeRatio := recentVolume / avgVolume

	// 成交量确认度评分
	if volumeRatio > 2.0 {
		return 1.0 // 强确认
	} else if volumeRatio > 1.5 {
		return 0.9 // 较强确认
	} else if volumeRatio > 1.2 {
		return 0.8 // 一般确认
	} else {
		return 0.6 // 弱确认
	}
}

// GetDowTheoryConfig 获取道氏理论配置
func GetDowTheoryConfig() DowTheoryConfig {
	return dowConfig
}

// UpdateDowTheoryConfig 更新道氏理论配置
func UpdateDowTheoryConfig(newConfig DowTheoryConfig) {
	dowConfig = newConfig
}


