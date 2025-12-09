package market

import (
	"log"
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

// Analyze 执行道氏理论分析（专注趋势识别，不包含通道）
func (dta *DowTheoryAnalyzer) Analyze(klines3m, klines4h []Kline, currentPrice float64) *DowTheoryData {
	// 数据量校验
	minRequired4h := 100  // 最小需要100根K线进行结构分析
	recommended4h := 500 // 建议500根以上获得更好的结构视野
	minRequired3m := 50  // 最小需蔂50根K线进行短期分析
	recommended3m := 200 // 建议200根以上获得更好的精度
	
	if len(klines4h) < minRequired4h {
		log.Printf("🚨🔴 [道氏理论] ❌ 4h K线数据不足: 需要%d根，实际%d根 ❌", minRequired4h, len(klines4h))
		return &DowTheoryData{}
	}
	if len(klines3m) < minRequired3m {
		log.Printf("🚨🔴 [道氏理论] ❌ 5m K线数据不足: 需要%d根，实际%d根 ❌", minRequired3m, len(klines3m))
		return &DowTheoryData{}
	}
	
	if len(klines4h) < recommended4h {
		log.Printf("🟡⚠️ [道氏理论] 结构视野警告: 建议%d根，实际%d根 (可能影响结构识别精度) ⚠️🟡", recommended4h, len(klines4h))
	}
	if len(klines3m) < recommended3m {
		log.Printf("🟡⚠️ [道氏理论] 短期分析警告: 建议%d根，实际%d根 (可能影响趋势精度) ⚠️🟡", recommended3m, len(klines3m))
	}
	
	// 使用全部4小时数据进行主要分析（最大优化结构视野）
	analysisKlines := klines4h
	
	// 使用全部5分钟数据进行短期分析（最大优化精度）
	shortTermKlines := klines3m

	swingPoints := dta.identifySwingPoints(analysisKlines)
	trendLines := dta.calculateTrendLines(swingPoints)
	trendStrength := dta.assessTrendStrength(shortTermKlines, analysisKlines, swingPoints, trendLines)
	tradingSignal := dta.generateTradingSignal(shortTermKlines, currentPrice, nil, trendStrength, trendLines)

	return &DowTheoryData{
		SwingPoints:   swingPoints,
		TrendLines:    trendLines,
		Channel:       nil, // 移除通道计算
		TrendStrength: trendStrength,
		TradingSignal: tradingSignal,
	}
}

// identifySwingPoints 识别摆动点（修复未来函数问题）
func (dta *DowTheoryAnalyzer) identifySwingPoints(klines []Kline) []*SwingPoint {
	if len(klines) < dta.config.SwingPointConfig.LookbackPeriod*2+1 {
		return nil
	}

	var swingPoints []*SwingPoint
	lookback := dta.config.SwingPointConfig.LookbackPeriod

	// 🔥 修复1：确认的摆动点（原逻辑，需要未来数据确认）
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
					Confirmed: true, // 这些都是已确认的
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
					Confirmed: true, // 这些都是已确认的
				}
				swingPoints = append(swingPoints, swingPoint)
			}
		}
	}

	// 🔥 修复2：添加实时分形检测（William's Fractal）
	realtimeSwings := dta.identifyRealtimeFractals(klines)
	swingPoints = append(swingPoints, realtimeSwings...)

	return swingPoints
}

// identifyRealtimeFractals 识别实时分形（Williams Fractal逻辑）
func (dta *DowTheoryAnalyzer) identifyRealtimeFractals(klines []Kline) []*SwingPoint {
	var fractals []*SwingPoint
	
	if len(klines) < 5 {
		return fractals
	}

	// 检查最近10根K线中的5根分形模式（只需要左右各2根）
	start := len(klines) - 10
	if start < 2 {
		start = 2
	}

	for i := start; i < len(klines)-2; i++ {
		// 分形高点：中间K线的高点高于左右2根
		if dta.isFractalHigh(klines, i) {
			strength := dta.calculateSwingPointStrength(klines, i, SwingHigh)
			
			// 对于未确认的分形，使用配置的降低系数
			if strength >= dta.config.SwingPointConfig.MinStrength * dta.config.SwingPointConfig.FractalReduction {
				fractal := &SwingPoint{
					Type:      SwingHigh,
					Price:     klines[i].High,
					Time:      klines[i].OpenTime,
					Index:     i,
					Strength:  strength * 0.8, // 未确认分形强度打折
					Confirmed: false,          // 标记为未确认
				}
				fractals = append(fractals, fractal)
			}
		}

		// 分形低点：中间K线的低点低于左右2根
		if dta.isFractalLow(klines, i) {
			strength := dta.calculateSwingPointStrength(klines, i, SwingLow)
			
			if strength >= dta.config.SwingPointConfig.MinStrength * dta.config.SwingPointConfig.FractalReduction {
				fractal := &SwingPoint{
					Type:      SwingLow,
					Price:     klines[i].Low,
					Time:      klines[i].OpenTime,
					Index:     i,
					Strength:  strength * 0.8, // 未确认分形强度打折
					Confirmed: false,          // 标记为未确认
				}
				fractals = append(fractals, fractal)
			}
		}
	}

	return fractals
}

// isFractalHigh 检查是否为分形高点（5根K线模式）
func (dta *DowTheoryAnalyzer) isFractalHigh(klines []Kline, index int) bool {
	if index < 2 || index >= len(klines)-2 {
		return false
	}

	centerHigh := klines[index].High
	
	// 检查左侧2根和右侧2根
	for i := index - 2; i <= index + 2; i++ {
		if i == index {
			continue
		}
		if klines[i].High >= centerHigh {
			return false
		}
	}
	
	return true
}

// isFractalLow 检查是否为分形低点（5根K线模式）
func (dta *DowTheoryAnalyzer) isFractalLow(klines []Kline, index int) bool {
	if index < 2 || index >= len(klines)-2 {
		return false
	}

	centerLow := klines[index].Low
	
	// 检查左侧2根和右侧2根
	for i := index - 2; i <= index + 2; i++ {
		if i == index {
			continue
		}
		if klines[i].Low <= centerLow {
			return false
		}
	}
	
	return true
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

	// 🔥 修复：使用配置的成交量权重
	// 综合计算强度：价格范围和成交量权重按配置比例
	priceWeight := 1.0 - dta.config.VolumeConfig.WeightInStrength
	volumeWeightRatio := dta.config.VolumeConfig.WeightInStrength
	strength := priceRange*priceWeight + math.Min(volumeWeight, 3.0)*volumeWeightRatio
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

// areParallel 判断两条趋势线是否平行（修复版）
func (dta *DowTheoryAnalyzer) areParallel(line1, line2 *TrendLine) bool {
	slope1 := line1.Slope
	slope2 := line2.Slope
	tolerance := dta.config.ChannelConfig.ParallelTolerance

	// 修复1: 验证同向性 - 斜率符号必须相同
	// 如果一个是正斜率，一个是负斜率，则绝对不平行
	if slope1*slope2 < 0 {
		return false
	}

	// 修复2: 处理零斜率的特殊情况
	if slope1 == 0 && slope2 == 0 {
		return true // 两条水平线总是平行
	}
	
	if slope1 == 0 || slope2 == 0 {
		// 只有一条是水平线，另一条不是，则不平行
		return false
	}

	// 修复3: 智能容忍度计算
	slopeDiff := math.Abs(slope1 - slope2)
	
	// 使用两个斜率的平均绝对值作为基准（保持原始意图但修复计算）
	avgSlopeMagnitude := (math.Abs(slope1) + math.Abs(slope2)) / 2
	
	// 对于极小斜率，使用绝对容忍度；对于较大斜率，使用相对容忍度
	if avgSlopeMagnitude < 0.0001 {
		// 极小斜率使用绝对容忍度
		return slopeDiff < tolerance * 0.0001
	}

	// 修复4: 相对容忍度计算
	relativeDiff := slopeDiff / avgSlopeMagnitude
	return relativeDiff < tolerance
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

// assessTrendStrength 评估趋势强度（加强成交量验证）
func (dta *DowTheoryAnalyzer) assessTrendStrength(klines3m, klines4h []Kline, swingPoints []*SwingPoint, trendLines []*TrendLine) *TrendStrength {
	if len(klines4h) < 20 {
		return &TrendStrength{
			Overall:   0,
			Direction: TrendFlat,
			Quality:   TrendWeak,
		}
	}

	// 计算短期趋势强度（基于5分钟数据）
	shortTerm := dta.calculateShortTermStrength(klines3m)

	// 计算长期趋势强度（基于4小时数据）
	longTerm := dta.calculateLongTermStrength(klines4h)

	// 🔥 修复：调整权重，更平衡短期和长期
	overall := (shortTerm*0.4 + longTerm*0.6)

	// 确定趋势方向
	direction := dta.determineTrendDirection(klines4h, swingPoints)

	// 计算动量强度
	momentum := dta.calculateMomentum(klines4h)

	// 计算一致性评分
	consistency := dta.calculateConsistency(klines3m, klines4h)

	// 🔥 修复：大幅提升成交量支撑度权重
	volumeSupport := dta.calculateVolumeSupport(klines4h)

	// 确定趋势质量（成交量权重大幅提升）
	quality := dta.determineTrendQuality(overall, consistency, volumeSupport)

	// 🔥 修复：如果成交量不支撑，使用配置的惩罚系数
	volumePenalty := 1.0
	if volumeSupport < dta.config.VolumeConfig.SupportThreshold { // 使用配置的成交量支撑阈值
		volumePenalty = dta.config.VolumeConfig.PenaltyMultiplier // 使用配置的惩罚系数
	}
	overall *= volumePenalty

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

// calculateShortTermStrength 计算短期趋势强度（修复评分偏低问题）
func (dta *DowTheoryAnalyzer) calculateShortTermStrength(klines []Kline) float64 {
	if len(klines) < 30 {
		return 0
	}

	// 使用最近30个K线确保有足够数据计算所有指标
	recentKlines := klines[len(klines)-30:]
	
	// 计算价格动量（保留方向性）
	priceChange := (recentKlines[len(recentKlines)-1].Close - recentKlines[0].Open) / recentKlines[0].Open

	// 计算移动平均
	ma5 := dta.calculateMA(recentKlines, 5)
	ma10 := dta.calculateMA(recentKlines, 10)
	ma20 := dta.calculateMA(recentKlines, 20)

	// MA趋势判断（保留方向性）
	maTrend := 0.0
	if ma5 > ma10 && ma10 > ma20 {
		maTrend = 1.0 // 多头排列
	} else if ma5 < ma10 && ma10 < ma20 {
		maTrend = -1.0 // 空头排列
	}

	// 计算波动性（标准化到0-1范围）
	volatility := dta.calculateVolatility(recentKlines)
	volatilityScore := math.Max(0, math.Min(1, 1-volatility*10)) // 将波动性映射到0-1分数

	// 🔥 修复：重新设计评分机制，提高敏感度
	// 1. 价格动量分数（-40到+40）- 降低倍数，提高敏感度
	momentumScore := math.Max(-40, math.Min(40, priceChange*500)) // 从1000降低到500
	
	// 2. MA趋势分数（-25到+25）
	trendScore := maTrend * 25
	
	// 3. 波动性分数（0到+15）
	volScore := volatilityScore * 15
	
	// 4. 添加相对强度评分 - 基于最近表现
	relativeStrength := dta.calculateRelativeStrength(recentKlines)
	rsScore := relativeStrength * 20 // (-20到+20)

	// 综合评分（-85到+100）
	rawStrength := momentumScore + trendScore + volScore + rsScore
	
	// 🔥 修复：改进标准化逻辑，提高区分度
	// 中性点设定在40而非50，使得小幅趋势也能获得50+分数
	neutralPoint := 40.0
	maxRange := 85.0
	
	if rawStrength >= 0 {
		// 正值映射到 [neutralPoint, 100]
		return math.Min(neutralPoint + (rawStrength/100.0)*(100-neutralPoint), 100.0)
	} else {
		// 负值映射到 [0, neutralPoint]
		return math.Max(neutralPoint + (rawStrength/maxRange)*neutralPoint, 0.0)
	}
}

// calculateRelativeStrength 计算相对强度（近期相对表现）
func (dta *DowTheoryAnalyzer) calculateRelativeStrength(klines []Kline) float64 {
	if len(klines) < 10 {
		return 0
	}
	
	// 最近5根相对前5根的表现
	recent5 := klines[len(klines)-5:]
	previous5 := klines[len(klines)-10:len(klines)-5]
	
	recentChange := (recent5[len(recent5)-1].Close - recent5[0].Open) / recent5[0].Open
	previousChange := (previous5[len(previous5)-1].Close - previous5[0].Open) / previous5[0].Open
	
	// 相对强度 = 最近表现 - 历史表现
	relativeStrength := recentChange - previousChange
	
	// 标准化到 -1 到 +1 范围
	return math.Max(-1, math.Min(1, relativeStrength*10))
}

// calculateLongTermStrength 计算长期趋势强度（修复版）
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

	// 构建价格序列
	prices := make([]float64, len(recentKlines))
	for i, k := range recentKlines {
		prices[i] = k.Close
	}

	// 修复"资产歧视"：使用百分比斜率而非绝对价格斜率
	percentageSlope := dta.calculatePercentageTrendSlope(prices)

	// 计算R-squared（趋势的线性度）
	rSquared := dta.calculateRSquared(prices)

	// 计算价格相对于移动平均线的位置
	ma20 := dta.calculateMA(recentKlines, 20)
	ma50 := dta.calculateMA(klines, 50)
	currentPrice := recentKlines[len(recentKlines)-1].Close

	// 修复MA位置判断重复逻辑
	maPosition := 0.0
	if currentPrice > ma20 && ma20 > ma50 {
		maPosition = 1.0  // 多头排列
	} else if currentPrice < ma20 && ma20 < ma50 {
		maPosition = -1.0 // 空头排列（修复：不再给相同分数）
	}

	// 修复：标准化计算，保留方向性
	// 百分比斜率分量（-40到+40），标准化处理
	slopeScore := math.Max(-40, math.Min(40, percentageSlope*1000)) // 百分比斜率*1000标准化

	// R²分量（0到40），表示趋势线性度
	rSquaredScore := rSquared * 40

	// MA位置分量（-20到+20），保留方向性
	maScore := maPosition * 20

	// 综合计算（范围-60到+100）
	rawStrength := slopeScore + rSquaredScore + maScore

	// 标准化到0-100范围，保留强弱信息
	if rawStrength >= 0 {
		// 正值：强度越高分数越高
		return math.Min(50 + rawStrength*0.5, 100.0)
	} else {
		// 负值：转换为低分数（0-50范围）
		return math.Max(50 + rawStrength*0.5, 0.0)
	}
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

// calculateTrendSlope 计算趋势斜率（保留原函数用于向后兼容）
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

// calculatePercentageTrendSlope 计算百分比趋势斜率（修复"资产歧视"漏洞）
func (dta *DowTheoryAnalyzer) calculatePercentageTrendSlope(prices []float64) float64 {
	n := len(prices)
	if n < 2 {
		return 0
	}

	// 修复"资产歧视"：使用对数价格消除绝对价格依赖
	logPrices := make([]float64, n)
	for i, price := range prices {
		if price <= 0 {
			return 0 // 避免对数计算错误
		}
		logPrices[i] = math.Log(price)
	}

	// 使用对数价格计算线性回归斜率
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0

	for i, logPrice := range logPrices {
		x := float64(i)
		sumX += x
		sumY += logPrice
		sumXY += x * logPrice
		sumX2 += x * x
	}

	nFloat := float64(n)
	denominator := nFloat*sumX2 - sumX*sumX
	if math.Abs(denominator) < 1e-10 {
		return 0 // 避免除零
	}

	// 对数斜率，表示每个时间单位的百分比变化率
	logSlope := (nFloat*sumXY - sumX*sumY) / denominator
	
	// 转换为百分比变化率（每时间单位）
	// logSlope = ln(P_end/P_start) / periods ≈ percentage_change / periods
	return logSlope // 这是标准化的百分比斜率，不依赖绝对价格
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

// calculateDynamicThreshold 计算基于波动率的动态阈值
func (dta *DowTheoryAnalyzer) calculateDynamicThreshold(klines []Kline) float64 {
	if len(klines) < 20 {
		return dta.config.ThresholdConfig.DefaultThreshold // 使用配置的默认阈值
	}

	// 计算ATR (平均真实波幅)
	atr := dta.calculateATR(klines, dta.config.ThresholdConfig.ATRPeriod) // 使用配置的ATR周期
	currentPrice := klines[len(klines)-1].Close
	
	// 将ATR转换为百分比
	atrPercent := atr / currentPrice
	
	// 动态阈值 = 配置倍数*ATR，但限制在配置范围内
	threshold := atrPercent * dta.config.ThresholdConfig.ATRMultiplier
	threshold = math.Max(
		dta.config.ThresholdConfig.MinThreshold, 
		math.Min(dta.config.ThresholdConfig.MaxThreshold, threshold),
	)
	
	return threshold
}

// calculateATR 计算平均真实波幅
func (dta *DowTheoryAnalyzer) calculateATR(klines []Kline, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}
	
	var trueRanges []float64
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close
		
		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)
		
		trueRange := math.Max(tr1, math.Max(tr2, tr3))
		trueRanges = append(trueRanges, trueRange)
	}
	
	// 计算ATR (简单移动平均)
	sum := 0.0
	start := len(trueRanges) - period
	if start < 0 {
		start = 0
		period = len(trueRanges)
	}
	
	for i := start; i < len(trueRanges); i++ {
		sum += trueRanges[i]
	}
	
	return sum / float64(period)
}

// determineTrendDirection 确定趋势方向（道氏理论标准）
func (dta *DowTheoryAnalyzer) determineTrendDirection(klines []Kline, swingPoints []*SwingPoint) TrendDirection {
	if len(klines) < 30 {
		return TrendFlat
	}

	// 使用配置的时间窗口进行趋势判断
	windowSize := dta.config.ThresholdConfig.TrendConfirmWindow
	if len(klines) < windowSize {
		windowSize = len(klines)
	}
	recentKlines := klines[len(klines)-windowSize:]
	
	// 计算长期价格趋势
	longTermChange := (recentKlines[len(recentKlines)-1].Close - recentKlines[0].Open) / recentKlines[0].Open

	// 基于摆动点的道氏理论判断（更严格的条件）
	swingDirection := 0.0
	if len(swingPoints) >= 6 { // 至少需要6个摆动点
		// 取最近6个摆动点进行分析
		recentSwings := swingPoints[len(swingPoints)-6:]

		var recentHighs, recentLows []*SwingPoint
		for _, swing := range recentSwings {
			if swing.Type == SwingHigh {
				recentHighs = append(recentHighs, swing)
			} else {
				recentLows = append(recentLows, swing)
			}
		}

		// 道氏理论：上升趋势 = 高点逐步抬高 + 低点逐步抬高
		if len(recentHighs) >= 3 && len(recentLows) >= 3 {
			// 检查高点趋势
			highTrend := 0.0
			for i := 1; i < len(recentHighs); i++ {
				if recentHighs[i].Price > recentHighs[i-1].Price {
					highTrend += 1.0
				} else {
					highTrend -= 1.0
				}
			}

			// 检查低点趋势
			lowTrend := 0.0
			for i := 1; i < len(recentLows); i++ {
				if recentLows[i].Price > recentLows[i-1].Price {
					lowTrend += 1.0
				} else {
					lowTrend -= 1.0
				}
			}

			// 道氏理论标准：高点和低点都要同向才确认趋势
			if highTrend > 0 && lowTrend > 0 {
				swingDirection = 1.0 // 明确上升
			} else if highTrend < 0 && lowTrend < 0 {
				swingDirection = -1.0 // 明确下降
			} else {
				swingDirection = 0.0 // 趋势不明确
			}
		}
	}

	// 移动平均确认
	ma20 := dta.calculateMA(recentKlines, 20)
	ma50 := dta.calculateMA(klines, 50)
	currentPrice := recentKlines[len(recentKlines)-1].Close
	
	maDirection := 0.0
	if currentPrice > ma20 && ma20 > ma50 {
		maDirection = 0.5
	} else if currentPrice < ma20 && ma20 < ma50 {
		maDirection = -0.5
	}

	// 综合判断（提高权重给道氏摆动点分析）
	overallDirection := longTermChange*0.3 + swingDirection*0.5 + maDirection*0.2

	// 🔥 修复：使用动态阈值替代硬编码的8%
	dynamicThreshold := dta.calculateDynamicThreshold(recentKlines)
	
	// 动态趋势判断
	if overallDirection > dynamicThreshold {
		return TrendUp
	} else if overallDirection < -dynamicThreshold {
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

// determineTrendQuality 确定趋势质量（使用配置的成交量权重）
func (dta *DowTheoryAnalyzer) determineTrendQuality(overall, consistency, volumeSupport float64) TrendQuality {
	// 使用配置的成交量权重
	volumeWeight := dta.config.VolumeConfig.WeightInQuality
	consistencyWeight := 0.2
	overallWeight := 1.0 - volumeWeight - consistencyWeight

	score := (overall*overallWeight + consistency*consistencyWeight + volumeSupport*volumeWeight)

	if score > 75 {
		return TrendStrong
	} else if score > 50 {
		return TrendModerate
	}

	return TrendWeak
}

// generateTradingSignal 生成交易信号（基于道氏理论，不包含通道）
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

	// 优先基于趋势跟随信号（道氏理论核心）
	trendSignal := dta.generateTrendFollowingSignal(currentPrice, trendStrength, nil)
	if trendSignal != nil && trendSignal.Confidence >= dta.config.SignalConfig.MinConfidence {
		return trendSignal
	}

	// 检查突破信号
	breakoutSignal := dta.generateBreakoutSignal(klines3m, currentPrice, trendLines, trendStrength)
	if breakoutSignal != nil && breakoutSignal.Confidence >= dta.config.SignalConfig.MinConfidence {
		return breakoutSignal
	}

	// 默认持有信号
	return &TradingSignal{
		Action:      ActionHold,
		Confidence:  30, // 降低默认信心度
		Description: "趋势不明确，建议观望等待明确信号",
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

// generateTrendFollowingSignal 生成趋势跟随信号（道氏理论严格标准）
func (dta *DowTheoryAnalyzer) generateTrendFollowingSignal(currentPrice float64,
	trendStrength *TrendStrength, channel *ParallelChannel) *TradingSignal {

	// 提高趋势跟随的标准
	if trendStrength.Quality == TrendWeak || trendStrength.Overall < 75 {
		return nil
	}

	// 要求更高的一致性
	if trendStrength.Consistency < 80 {
		return nil
	}

	var signal *TradingSignal
	confidence := math.Min(trendStrength.Overall * 0.9, 95.0) // 最高95%信心度

	if trendStrength.Direction == TrendUp && trendStrength.Consistency >= 80 {
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

// confirmWithVolume 通过成交量确认信号（使用配置的比例）
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

	// 使用配置的成交量确认比例
	ratios := dta.config.VolumeConfig.ConfirmationRatios
	if volumeRatio > ratios.Strong {
		return 1.0 // 强确认
	} else if volumeRatio > ratios.Moderate {
		return 0.9 // 较强确认
	} else if volumeRatio > ratios.Normal {
		return 0.8 // 一般确认
	} else {
		return ratios.WeakPenalty // 弱确认，使用配置的惩罚值
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


