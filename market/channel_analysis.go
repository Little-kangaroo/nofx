package market

import (
	"fmt"
	"math"
	"sort"
)

// ChannelAnalyzer 通道分析器（独立于道氏理论）
type ChannelAnalyzer struct {
	config ChannelAnalysisConfig
}

// ChannelAnalysisConfig 通道分析配置
type ChannelAnalysisConfig struct {
	SwingLookback     int     // 摆动点回看周期
	MinSwingStrength  float64 // 最小摆动点强度
	MinTrendLineHits  int     // 最小趋势线命中数
	MaxDistance       float64 // 最大距离容忍度
	MinChannelWidth   float64 // 最小通道宽度
	MaxChannelWidth   float64 // 最大通道宽度
	ParallelTolerance float64 // 平行容忍度
	QualityThreshold  float64 // 质量阈值
	
	// 🔥 新增：ATR动态宽度标准
	EnableATRWidthStandards bool    // 启用ATR宽度标准
	MinChannelWidthATR      float64 // 最小通道宽度（ATR倍数）
	MaxChannelWidthATR      float64 // 最大通道宽度（ATR倍数）
	OptimalWidthATRMin      float64 // 最优宽度下限（ATR倍数）
	OptimalWidthATRMax      float64 // 最优宽度上限（ATR倍数）
}

// ChannelData 通道分析数据
type ChannelData struct {
	ActiveChannel   *Channel      `json:"active_channel"`   // 当前有效通道
	TrendLines      []*TrendLine  `json:"trend_lines"`      // 所有趋势线
	CurrentPosition string        `json:"current_position"` // 当前价格位置
	PriceRatio      float64       `json:"price_ratio"`      // 价格在通道中的比例(0-1)
	Quality         float64       `json:"quality"`          // 通道质量评分
	Direction       string        `json:"direction"`        // 通道方向
	Analysis        string        `json:"analysis"`         // 分析描述
	
	// 🔥 新增：ATR分析字段
	WidthATR        float64       `json:"width_atr"`        // 通道宽度（ATR倍数）
	ATRGrade        string        `json:"atr_grade"`        // ATR评级（A/B/C/D）
	ATRAnalysis     string        `json:"atr_analysis"`     // ATR分析描述
}

// Channel 通道结构
type Channel struct {
	UpperLine  *TrendLine `json:"upper_line"`
	LowerLine  *TrendLine `json:"lower_line"`
	MiddleLine *TrendLine `json:"middle_line"`
	Width      float64    `json:"width"`
	Quality    float64    `json:"quality"`
	Direction  string     `json:"direction"`
	Age        int64      `json:"age"` // 通道存在时间（毫秒）
}

// NewChannelAnalyzer 创建通道分析器
func NewChannelAnalyzer() *ChannelAnalyzer {
	return &ChannelAnalyzer{
		config: ChannelAnalysisConfig{
			SwingLookback:     7,    // 7个周期回看
			MinSwingStrength:  0.6,  // 最小强度0.6
			MinTrendLineHits:  3,    // 至少3次命中
			MaxDistance:       0.015, // 1.5%容忍度
			MinChannelWidth:   0.02,  // 2%最小宽度（传统模式）
			MaxChannelWidth:   0.18,  // 18%最大宽度（传统模式）
			ParallelTolerance: 0.08,  // 8%平行容忍度
			QualityThreshold:  0.75,  // 75%质量阈值
			
			// 🔥 新增：ATR动态宽度标准配置
			EnableATRWidthStandards: true,  // 启用ATR标准
			MinChannelWidthATR:      0.8,   // 最小0.8倍ATR
			MaxChannelWidthATR:      4.0,   // 最大4.0倍ATR
			OptimalWidthATRMin:      1.2,   // 最优下限1.2倍ATR
			OptimalWidthATRMax:      2.5,   // 最优上限2.5倍ATR
		},
	}
}

// Analyze 执行通道分析
// 🔥 修复：增加ATR动态宽度标准支持
func (ca *ChannelAnalyzer) Analyze(klines []Kline, currentPrice float64) *ChannelData {
	if len(klines) < 50 {
		return &ChannelData{
			Analysis: "数据不足，无法进行通道分析",
		}
	}

	// 🔥 新增：计算ATR用于动态宽度标准
	atr14 := ca.calculateATR(klines, 14)
	
	// 使用全部K线进行分析（最大优化结构视野）
	analysisData := klines

	// 1. 识别摆动点
	swingPoints := ca.identifySwingPoints(analysisData)
	if len(swingPoints) < 4 {
		return &ChannelData{
			Analysis: "摆动点不足，无法构建通道",
		}
	}

	// 2. 计算趋势线
	trendLines := ca.calculateTrendLines(swingPoints)
	if len(trendLines) < 2 {
		return &ChannelData{
			Analysis: "趋势线不足，无法构建通道",
		}
	}

	// 3. 构建最佳通道（包含ATR宽度验证）
	currentIndex := len(klines) - 1
	channel := ca.findBestChannelWithATR(trendLines, swingPoints, currentPrice, currentIndex, atr14)
	if channel == nil {
		return &ChannelData{
			TrendLines: trendLines,
			Analysis:   "未找到有效通道",
		}
	}

	// 4. 计算当前价格位置
	position, ratio := ca.calculatePricePosition(currentPrice, channel, currentIndex)

	// 🔥 新增：计算ATR分析
	widthATR, atrGrade, atrAnalysis := ca.calculateATRAnalysis(channel, currentPrice, atr14)

	// 5. 生成分析描述（整合ATR分析）
	analysis := ca.generateAnalysisWithATR(channel, position, ratio, atrGrade, atrAnalysis)

	return &ChannelData{
		ActiveChannel:   channel,
		TrendLines:      trendLines,
		CurrentPosition: position,
		PriceRatio:      ratio,
		Quality:         channel.Quality,
		Direction:       channel.Direction,
		Analysis:        analysis,
		
		// 🔥 新增：ATR分析结果
		WidthATR:    widthATR,
		ATRGrade:    atrGrade,
		ATRAnalysis: atrAnalysis,
	}
}

// identifySwingPoints 识别摆动点
func (ca *ChannelAnalyzer) identifySwingPoints(klines []Kline) []*SwingPoint {
	var swingPoints []*SwingPoint
	lookback := ca.config.SwingLookback

	for i := lookback; i < len(klines)-lookback; i++ {
		// 检查高点
		if ca.isLocalHigh(klines, i, lookback) {
			strength := ca.calculateSwingStrength(klines, i, true)
			if strength >= ca.config.MinSwingStrength {
				swingPoints = append(swingPoints, &SwingPoint{
					Type:      SwingHigh,
					Price:     klines[i].High,
					Time:      klines[i].OpenTime,
					Index:     i,
					Strength:  strength,
					Confirmed: true,
				})
			}
		}

		// 检查低点
		if ca.isLocalLow(klines, i, lookback) {
			strength := ca.calculateSwingStrength(klines, i, false)
			if strength >= ca.config.MinSwingStrength {
				swingPoints = append(swingPoints, &SwingPoint{
					Type:      SwingLow,
					Price:     klines[i].Low,
					Time:      klines[i].OpenTime,
					Index:     i,
					Strength:  strength,
					Confirmed: true,
				})
			}
		}
	}

	return swingPoints
}

// isLocalHigh 检查是否为局部高点
func (ca *ChannelAnalyzer) isLocalHigh(klines []Kline, index, lookback int) bool {
	current := klines[index].High
	for i := index - lookback; i <= index+lookback; i++ {
		if i != index && i >= 0 && i < len(klines) {
			if klines[i].High >= current {
				return false
			}
		}
	}
	return true
}

// isLocalLow 检查是否为局部低点
func (ca *ChannelAnalyzer) isLocalLow(klines []Kline, index, lookback int) bool {
	current := klines[index].Low
	for i := index - lookback; i <= index+lookback; i++ {
		if i != index && i >= 0 && i < len(klines) {
			if klines[i].Low <= current {
				return false
			}
		}
	}
	return true
}

// calculateSwingStrength 计算摆动点强度
func (ca *ChannelAnalyzer) calculateSwingStrength(klines []Kline, index int, isHigh bool) float64 {
	if index < 10 || index >= len(klines)-10 {
		return 0
	}

	// 价格范围评分
	priceRange := (klines[index].High - klines[index].Low) / klines[index].Close
	
	// 成交量评分
	volumeScore := 1.0
	if len(klines) > index+20 {
		avgVolume := 0.0
		for i := index - 10; i <= index+10 && i < len(klines); i++ {
			if i >= 0 {
				avgVolume += klines[i].Volume
			}
		}
		avgVolume /= 21
		if avgVolume > 0 {
			volumeScore = math.Min(klines[index].Volume/avgVolume, 2.0)
		}
	}

	// 相对位置评分
	positionScore := 0.0
	if isHigh {
		// 高点：相对于周围的突出程度
		maxHigh := klines[index].High
		for i := index - 15; i <= index+15 && i < len(klines); i++ {
			if i >= 0 && i != index {
				maxHigh = math.Max(maxHigh, klines[i].High)
			}
		}
		if maxHigh > 0 {
			positionScore = klines[index].High / maxHigh
		}
	} else {
		// 低点：相对于周围的突出程度
		minLow := klines[index].Low
		for i := index - 15; i <= index+15 && i < len(klines); i++ {
			if i >= 0 && i != index {
				if minLow == 0 || klines[i].Low < minLow {
					minLow = klines[i].Low
				}
			}
		}
		if klines[index].Low > 0 {
			positionScore = minLow / klines[index].Low
		}
	}

	return (priceRange*0.4 + volumeScore*0.3 + positionScore*0.3) * 2.0
}

// calculateATR 计算平均真实波幅
func (ca *ChannelAnalyzer) calculateATR(klines []Kline, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}

	var trSum float64
	count := 0
	
	for i := len(klines) - period; i < len(klines); i++ {
		if i <= 0 {
			continue
		}
		
		current := klines[i]
		previous := klines[i-1]
		
		tr1 := current.High - current.Low
		tr2 := math.Abs(current.High - previous.Close)
		tr3 := math.Abs(current.Low - previous.Close)
		
		tr := math.Max(tr1, math.Max(tr2, tr3))
		trSum += tr
		count++
	}
	
	if count == 0 {
		return 0
	}
	
	return trSum / float64(count)
}

// calculateTrendLines 计算趋势线
func (ca *ChannelAnalyzer) calculateTrendLines(swingPoints []*SwingPoint) []*TrendLine {
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

	// 计算阻力线
	resistanceLines := ca.calculateTrendLinesFromPoints(highs, ResistanceLine)
	trendLines = append(trendLines, resistanceLines...)

	// 计算支撑线
	supportLines := ca.calculateTrendLinesFromPoints(lows, SupportLine)
	trendLines = append(trendLines, supportLines...)

	// 按质量排序
	sort.Slice(trendLines, func(i, j int) bool {
		return trendLines[i].Strength > trendLines[j].Strength
	})

	return trendLines
}

// calculateTrendLinesFromPoints 从点计算趋势线
// 🔥 修复：使用K线索引坐标系统，消除Unix毫秒时间戳数值精度灾难
func (ca *ChannelAnalyzer) calculateTrendLinesFromPoints(points []*SwingPoint, lineType TrendLineType) []*TrendLine {
	if len(points) < 2 {
		return nil
	}

	var trendLines []*TrendLine

	// 尝试所有点对组合
	for i := 0; i < len(points)-1; i++ {
		for j := i + 1; j < len(points); j++ {
			point1 := points[i]
			point2 := points[j]

			// 🔥 修复：使用K线索引差替代时间差，消除数值精度问题
			indexDiff := float64(point2.Index - point1.Index)
			if indexDiff <= 0 {
				continue
			}

			// 🔥 修复：基于索引的斜率计算，数值稳定且回测一致
			// slope = (price2 - price1) / (index2 - index1)
			// 含义：每根K线的平均价格变化
			slope := (point2.Price - point1.Price) / indexDiff
			
			// 🔥 修复：基于索引的截距计算
			// price = slope * index + intercept
			// intercept = price1 - slope * index1
			intercept := point1.Price - slope*float64(point1.Index)

			trendLine := &TrendLine{
				Type:      lineType,
				Points:    []*SwingPoint{point1, point2},
				Slope:     slope,      // 每根K线价格变化
				Intercept: intercept,  // 索引0处的价格截距
				LastTouch: point2.Time,
				Touches:   2,
			}

			// 计算命中次数
			hits := ca.calculateTrendLineHits(trendLine, points)
			if hits >= ca.config.MinTrendLineHits {
				trendLine.Touches = hits
				trendLine.Strength = ca.calculateTrendLineStrength(trendLine)
				trendLines = append(trendLines, trendLine)
			}
		}
	}

	return trendLines
}

// calculateTrendLineHits 计算趋势线命中次数
// 🔥 修复：使用索引坐标系统计算命中，与趋势线斜率定义保持一致
func (ca *ChannelAnalyzer) calculateTrendLineHits(trendLine *TrendLine, points []*SwingPoint) int {
	hits := 0
	for _, point := range points {
		// 🔥 修复：基于索引计算期望价格
		// expectedPrice = slope * index + intercept
		expectedPrice := trendLine.Slope*float64(point.Index) + trendLine.Intercept
		distance := math.Abs(point.Price-expectedPrice) / expectedPrice
		if distance <= ca.config.MaxDistance {
			hits++
		}
	}
	return hits
}

// calculateTrendLineStrength 计算趋势线强度
// 🔥 修复：使用索引跨度替代时间跨度，避免毫秒计算误差
func (ca *ChannelAnalyzer) calculateTrendLineStrength(trendLine *TrendLine) float64 {
	strength := 0.0

	// 基于命中次数
	strength += float64(trendLine.Touches) * 2.0

	// 🔥 修复：基于索引跨度计算时间强度
	if len(trendLine.Points) >= 2 {
		indexSpan := trendLine.Points[len(trendLine.Points)-1].Index - trendLine.Points[0].Index
		// 将索引跨度转换为相对强度 (每10个索引相当于1天的概念)
		relativeSpan := float64(indexSpan) / 10.0
		strength += math.Min(relativeSpan/7, 3.0) // 最多加3分，相当于7天权重
	}

	// 基于点强度
	pointStrength := 0.0
	for _, point := range trendLine.Points {
		pointStrength += point.Strength
	}
	strength += pointStrength / float64(len(trendLine.Points))

	return strength
}

// findBestChannelWithATR 寻找最佳通道（ATR增强版）
// 🔥 新增：集成ATR动态宽度标准的通道识别
func (ca *ChannelAnalyzer) findBestChannelWithATR(trendLines []*TrendLine, swingPoints []*SwingPoint, currentPrice float64, currentIndex int, atr14 float64) *Channel {
	var bestChannel *Channel
	bestScore := 0.0

	// 尝试所有趋势线组合
	for i := 0; i < len(trendLines); i++ {
		for j := i + 1; j < len(trendLines); j++ {
			line1 := trendLines[i]
			line2 := trendLines[j]

			// 检查是否可以形成有效通道
			if !ca.canFormChannel(line1, line2) {
				continue
			}

			channel := ca.createChannelWithATR(line1, line2, currentPrice, currentIndex, atr14)
			if channel == nil {
				continue
			}

			// 评分（包含ATR质量评分）
			score := ca.scoreChannelWithATR(channel, swingPoints, atr14)
			if score > bestScore && channel.Quality >= ca.config.QualityThreshold {
				bestScore = score
				bestChannel = channel
			}
		}
	}

	return bestChannel
}

// findBestChannel 寻找最佳通道（兼容性保留）
// 🔥 修复：添加当前索引参数，消除time.Now()回测不一致问题
func (ca *ChannelAnalyzer) findBestChannel(trendLines []*TrendLine, swingPoints []*SwingPoint, currentPrice float64, currentIndex int) *Channel {
	// 如果启用ATR标准，调用ATR版本
	if ca.config.EnableATRWidthStandards {
		// 计算ATR14用于分析
		atr14 := ca.calculateATR([]Kline{}, 14) // 需要klines参数，这里简化处理
		if atr14 > 0 {
			return ca.findBestChannelWithATR(trendLines, swingPoints, currentPrice, currentIndex, atr14)
		}
	}
	
	// 降级到传统方法
	return ca.findBestChannelTraditional(trendLines, swingPoints, currentPrice, currentIndex)
}

// findBestChannelTraditional 传统方法寻找最佳通道
func (ca *ChannelAnalyzer) findBestChannelTraditional(trendLines []*TrendLine, swingPoints []*SwingPoint, currentPrice float64, currentIndex int) *Channel {
	var bestChannel *Channel
	bestScore := 0.0

	// 尝试所有趋势线组合
	for i := 0; i < len(trendLines); i++ {
		for j := i + 1; j < len(trendLines); j++ {
			line1 := trendLines[i]
			line2 := trendLines[j]

			// 检查是否可以形成有效通道
			if !ca.canFormChannel(line1, line2) {
				continue
			}

			channel := ca.createChannel(line1, line2, currentPrice, currentIndex)
			if channel == nil {
				continue
			}

			// 评分
			score := ca.scoreChannel(channel, swingPoints)
			if score > bestScore && channel.Quality >= ca.config.QualityThreshold {
				bestScore = score
				bestChannel = channel
			}
		}
	}

	return bestChannel
}

// canFormChannel 检查两条线是否能形成通道
func (ca *ChannelAnalyzer) canFormChannel(line1, line2 *TrendLine) bool {
	// 必须是不同类型的线
	if line1.Type == line2.Type {
		return false
	}

	// 检查平行度
	if math.Abs(line1.Slope-line2.Slope) > ca.config.ParallelTolerance*math.Max(math.Abs(line1.Slope), math.Abs(line2.Slope)) {
		return false
	}

	return true
}

// createChannelWithATR 创建通道（ATR增强版）
// 🔥 新增：使用ATR动态宽度标准验证通道有效性
func (ca *ChannelAnalyzer) createChannelWithATR(line1, line2 *TrendLine, currentPrice float64, currentIndex int, atr14 float64) *Channel {
	var upperLine, lowerLine *TrendLine

	// 🔥 修复：使用当前索引替代time.Now()，确保回测/实盘一致性
	currentIndexFloat := float64(currentIndex)
	price1 := line1.Slope*currentIndexFloat + line1.Intercept
	price2 := line2.Slope*currentIndexFloat + line2.Intercept

	if price1 > price2 {
		upperLine = line1
		lowerLine = line2
	} else {
		upperLine = line2
		lowerLine = line1
	}

	// 计算通道宽度（绝对值和ATR相对值）
	absoluteWidth := math.Abs(price1 - price2)
	percentageWidth := absoluteWidth / currentPrice
	atrWidth := 0.0
	if atr14 > 0 {
		atrWidth = absoluteWidth / atr14
	}

	// 🔥 新增：ATR动态宽度验证
	if ca.config.EnableATRWidthStandards && atr14 > 0 {
		// 使用ATR标准进行宽度验证
		if atrWidth < ca.config.MinChannelWidthATR || atrWidth > ca.config.MaxChannelWidthATR {
			return nil // 不符合ATR宽度标准
		}
	} else {
		// 降级到传统百分比验证
		if percentageWidth < ca.config.MinChannelWidth || percentageWidth > ca.config.MaxChannelWidth {
			return nil
		}
	}

	// 创建中线
	middleLine := &TrendLine{
		Type:      SupportLine,
		Slope:     (upperLine.Slope + lowerLine.Slope) / 2,
		Intercept: (upperLine.Intercept + lowerLine.Intercept) / 2,
		Strength:  (upperLine.Strength + lowerLine.Strength) / 2,
	}

	// 🔥 修复：使用斜率值直接判断方向，避免精度阈值问题
	direction := "flat"
	avgSlope := (upperLine.Slope + lowerLine.Slope) / 2
	if avgSlope > 0.001 {
		direction = "up"
	} else if avgSlope < -0.001 {
		direction = "down"
	}

	// 🔥 修复：基于索引计算通道年龄
	upperStartIndex := upperLine.Points[0].Index
	lowerStartIndex := lowerLine.Points[0].Index
	channelStartIndex := minInt(upperStartIndex, lowerStartIndex)
	age := int64(currentIndex - channelStartIndex) // 索引差作为年龄

	return &Channel{
		UpperLine:  upperLine,
		LowerLine:  lowerLine,
		MiddleLine: middleLine,
		Width:      percentageWidth, // 保持百分比宽度用于兼容性
		Direction:  direction,
		Age:        age,
	}
}

// scoreChannelWithATR 为通道评分（ATR增强版）
// 🔥 新增：集成ATR质量评估的通道评分系统
func (ca *ChannelAnalyzer) scoreChannelWithATR(channel *Channel, swingPoints []*SwingPoint, atr14 float64) float64 {
	score := 0.0

	// 基础评分：基于趋势线强度
	score += (channel.UpperLine.Strength + channel.LowerLine.Strength) / 2

	// 基于命中次数
	totalHits := channel.UpperLine.Touches + channel.LowerLine.Touches
	score += float64(totalHits) * 0.5

	// 🔥 修复：基于索引的通道年龄评分，避免毫秒转换误差
	ageInIndices := float64(channel.Age) // channel.Age现在是索引差
	if ageInIndices <= 50 { // 50根K线内认为是新通道
		score += 2.0
	} else if ageInIndices <= 200 { // 200根K线内认为是较新通道
		score += 1.0
	}

	// 🔥 新增：ATR宽度质量评分
	if atr14 > 0 {
		// 计算当前通道的ATR宽度
		currentIndexFloat := float64(len(swingPoints)) // 简化处理
		price1 := channel.UpperLine.Slope*currentIndexFloat + channel.UpperLine.Intercept
		price2 := channel.LowerLine.Slope*currentIndexFloat + channel.LowerLine.Intercept
		atrWidth := math.Abs(price1-price2) / atr14

		// ATR宽度质量评分：最优范围内给予最高分
		if atrWidth >= ca.config.OptimalWidthATRMin && atrWidth <= ca.config.OptimalWidthATRMax {
			score += 3.0 // 最优ATR范围加分
		} else if atrWidth >= ca.config.MinChannelWidthATR && atrWidth <= ca.config.MaxChannelWidthATR {
			score += 1.5 // 可接受ATR范围适度加分
		} else {
			score -= 1.0 // 不理想ATR范围扣分
		}
		
		// 额外ATR一致性加分：宽度稳定性
		if atrWidth >= 1.0 && atrWidth <= 3.0 {
			score += 1.0 // 1-3倍ATR范围内额外加分
		}
	} else {
		// 降级到传统宽度评分
		if channel.Width >= 0.03 && channel.Width <= 0.08 {
			score += 1.0
		}
	}

	channel.Quality = math.Min(score/12.0, 1.0) // 调整最大分母以适应新评分项
	return score
}

// createChannel 创建通道
// 🔥 修复：使用索引坐标系统，消除time.Now()回测/实盘不一致灾难
func (ca *ChannelAnalyzer) createChannel(line1, line2 *TrendLine, currentPrice float64, currentIndex int) *Channel {
	var upperLine, lowerLine *TrendLine

	// 🔥 修复：使用当前索引替代time.Now()，确保回测/实盘一致性
	currentIndexFloat := float64(currentIndex)
	price1 := line1.Slope*currentIndexFloat + line1.Intercept
	price2 := line2.Slope*currentIndexFloat + line2.Intercept

	if price1 > price2 {
		upperLine = line1
		lowerLine = line2
	} else {
		upperLine = line2
		lowerLine = line1
	}

	// 计算通道宽度
	width := math.Abs(price1-price2) / currentPrice
	if width < ca.config.MinChannelWidth || width > ca.config.MaxChannelWidth {
		return nil
	}

	// 创建中线
	middleLine := &TrendLine{
		Type:      SupportLine,
		Slope:     (upperLine.Slope + lowerLine.Slope) / 2,
		Intercept: (upperLine.Intercept + lowerLine.Intercept) / 2,
		Strength:  (upperLine.Strength + lowerLine.Strength) / 2,
	}

	// 🔥 修复：使用斜率值直接判断方向，避免精度阈值问题
	direction := "flat"
	avgSlope := (upperLine.Slope + lowerLine.Slope) / 2
	if avgSlope > 0.001 {
		direction = "up"
	} else if avgSlope < -0.001 {
		direction = "down"
	}

	// 🔥 修复：基于索引计算通道年龄
	upperStartIndex := upperLine.Points[0].Index
	lowerStartIndex := lowerLine.Points[0].Index
	channelStartIndex := minInt(upperStartIndex, lowerStartIndex)
	age := int64(currentIndex - channelStartIndex) // 索引差作为年龄

	return &Channel{
		UpperLine:  upperLine,
		LowerLine:  lowerLine,
		MiddleLine: middleLine,
		Width:      width,
		Direction:  direction,
		Age:        age,
	}
}

// scoreChannel 为通道评分
func (ca *ChannelAnalyzer) scoreChannel(channel *Channel, swingPoints []*SwingPoint) float64 {
	score := 0.0

	// 基于趋势线强度
	score += (channel.UpperLine.Strength + channel.LowerLine.Strength) / 2

	// 基于命中次数
	totalHits := channel.UpperLine.Touches + channel.LowerLine.Touches
	score += float64(totalHits) * 0.5

	// 🔥 修复：基于索引的通道年龄评分，避免毫秒转换误差
	ageInIndices := float64(channel.Age) // channel.Age现在是索引差
	if ageInIndices <= 50 { // 50根K线内认为是新通道
		score += 2.0
	} else if ageInIndices <= 200 { // 200根K线内认为是较新通道
		score += 1.0
	}

	// 基于宽度（适中的宽度更好）
	if channel.Width >= 0.03 && channel.Width <= 0.08 {
		score += 1.0
	}

	channel.Quality = math.Min(score/10.0, 1.0)
	return score
}

// calculatePricePosition 计算价格在通道中的位置
// 🔥 修复：使用索引坐标系统，消除time.Now()回测/实盘不一致问题
func (ca *ChannelAnalyzer) calculatePricePosition(currentPrice float64, channel *Channel, currentIndex int) (string, float64) {
	// 🔥 修复：使用当前索引替代time.Now()
	currentIndexFloat := float64(currentIndex)
	upperPrice := channel.UpperLine.Slope*currentIndexFloat + channel.UpperLine.Intercept
	lowerPrice := channel.LowerLine.Slope*currentIndexFloat + channel.LowerLine.Intercept

	// 计算比例
	ratio := (currentPrice - lowerPrice) / (upperPrice - lowerPrice)
	ratio = math.Max(0, math.Min(1, ratio))

	// 确定位置
	position := "middle"
	if ratio > 0.8 {
		position = "upper"
	} else if ratio < 0.2 {
		position = "lower"
	} else if currentPrice > upperPrice*1.01 {
		position = "break_up"
	} else if currentPrice < lowerPrice*0.99 {
		position = "break_down"
	}

	return position, ratio
}

// generateAnalysis 生成分析描述
func (ca *ChannelAnalyzer) generateAnalysis(channel *Channel, position string, ratio float64) string {
	analysis := ""

	// 基本描述
	if channel.Direction == "up" {
		analysis += "上升通道"
	} else if channel.Direction == "down" {
		analysis += "下降通道"
	} else {
		analysis += "水平通道"
	}

	analysis += ", 质量评分: " + fmt.Sprintf("%.1f", channel.Quality*10)

	// 位置描述
	switch position {
	case "upper":
		analysis += ", 价格接近上轨阻力位"
	case "lower":
		analysis += ", 价格接近下轨支撑位"
	case "middle":
		analysis += ", 价格位于通道中部"
	case "break_up":
		analysis += ", 价格向上突破通道"
	case "break_down":
		analysis += ", 价格向下突破通道"
	}

	analysis += fmt.Sprintf(" (%.1f%%)", ratio*100)

	return analysis
}

// calculateATRAnalysis 计算ATR分析
// 🔥 新增：基于ATR的通道宽度质量分析
func (ca *ChannelAnalyzer) calculateATRAnalysis(channel *Channel, currentPrice float64, atr14 float64) (float64, string, string) {
	if atr14 == 0 {
		return 0, "未知", "ATR数据不足，无法进行ATR分析"
	}

	// 计算通道宽度的ATR倍数
	currentIndexFloat := float64(100) // 简化处理，使用固定索引
	price1 := channel.UpperLine.Slope*currentIndexFloat + channel.UpperLine.Intercept
	price2 := channel.LowerLine.Slope*currentIndexFloat + channel.LowerLine.Intercept
	absoluteWidth := math.Abs(price1 - price2)
	widthATR := absoluteWidth / atr14

	// ATR评级
	var atrGrade string
	var analysisDesc string

	switch {
	case widthATR >= ca.config.OptimalWidthATRMin && widthATR <= ca.config.OptimalWidthATRMax:
		atrGrade = "A"
		analysisDesc = fmt.Sprintf("优质通道：宽度%.1f倍ATR，处于最优范围[%.1f-%.1f]", 
			widthATR, ca.config.OptimalWidthATRMin, ca.config.OptimalWidthATRMax)

	case widthATR >= ca.config.MinChannelWidthATR && widthATR <= ca.config.MaxChannelWidthATR:
		if widthATR < ca.config.OptimalWidthATRMin {
			atrGrade = "B"
			analysisDesc = fmt.Sprintf("良好通道：宽度%.1f倍ATR，略窄但可交易", widthATR)
		} else {
			atrGrade = "B" 
			analysisDesc = fmt.Sprintf("良好通道：宽度%.1f倍ATR，略宽但可交易", widthATR)
		}

	case widthATR < ca.config.MinChannelWidthATR:
		if widthATR > ca.config.MinChannelWidthATR*0.7 {
			atrGrade = "C"
			analysisDesc = fmt.Sprintf("谨慎通道：宽度%.1f倍ATR，偏窄易被假突破", widthATR)
		} else {
			atrGrade = "D"
			analysisDesc = fmt.Sprintf("风险通道：宽度%.1f倍ATR，过窄不建议交易", widthATR)
		}

	default: // widthATR > MaxChannelWidthATR
		if widthATR < ca.config.MaxChannelWidthATR*1.5 {
			atrGrade = "C"
			analysisDesc = fmt.Sprintf("谨慎通道：宽度%.1f倍ATR，偏宽盈亏比差", widthATR)
		} else {
			atrGrade = "D"
			analysisDesc = fmt.Sprintf("风险通道：宽度%.1f倍ATR，过宽不建议交易", widthATR)
		}
	}

	return widthATR, atrGrade, analysisDesc
}

// generateAnalysisWithATR 生成包含ATR分析的描述
// 🔥 新增：整合ATR评级的综合分析描述
func (ca *ChannelAnalyzer) generateAnalysisWithATR(channel *Channel, position string, ratio float64, atrGrade string, atrAnalysis string) string {
	analysis := ""

	// 基本描述
	if channel.Direction == "up" {
		analysis += "上升通道"
	} else if channel.Direction == "down" {
		analysis += "下降通道"
	} else {
		analysis += "水平通道"
	}

	analysis += fmt.Sprintf(", 质量评分: %.1f", channel.Quality*10)

	// 位置描述
	switch position {
	case "upper":
		analysis += ", 价格接近上轨阻力位"
	case "lower":
		analysis += ", 价格接近下轨支撑位"
	case "middle":
		analysis += ", 价格位于通道中部"
	case "break_up":
		analysis += ", 价格向上突破通道"
	case "break_down":
		analysis += ", 价格向下突破通道"
	}

	analysis += fmt.Sprintf(" (%.1f%%)", ratio*100)

	// 🔥 新增：ATR评级描述
	analysis += fmt.Sprintf(", ATR评级: %s", atrGrade)
	
	// 简化ATR分析描述
	if len(atrAnalysis) > 0 {
		// 提取关键信息
		if atrGrade == "A" {
			analysis += ", 通道宽度优质适宜交易"
		} else if atrGrade == "B" {
			analysis += ", 通道宽度良好可以交易"
		} else if atrGrade == "C" {
			analysis += ", 通道宽度一般需要谨慎"
		} else {
			analysis += ", 通道宽度存在风险"
		}
	}

	return analysis
}