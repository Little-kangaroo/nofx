package market

import (
	"fmt"
	"math"
	"sort"
	"time"
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
			MinChannelWidth:   0.02,  // 2%最小宽度
			MaxChannelWidth:   0.18,  // 18%最大宽度
			ParallelTolerance: 0.08,  // 8%平行容忍度
			QualityThreshold:  0.75,  // 75%质量阈值
		},
	}
}

// Analyze 执行通道分析
func (ca *ChannelAnalyzer) Analyze(klines []Kline, currentPrice float64) *ChannelData {
	if len(klines) < 50 {
		return &ChannelData{
			Analysis: "数据不足，无法进行通道分析",
		}
	}

	// 使用最近300根K线进行分析
	analysisData := klines
	if len(klines) > 300 {
		analysisData = klines[len(klines)-300:]
	}

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

	// 3. 构建最佳通道
	channel := ca.findBestChannel(trendLines, swingPoints, currentPrice)
	if channel == nil {
		return &ChannelData{
			TrendLines: trendLines,
			Analysis:   "未找到有效通道",
		}
	}

	// 4. 计算当前价格位置
	position, ratio := ca.calculatePricePosition(currentPrice, channel)

	// 5. 生成分析描述
	analysis := ca.generateAnalysis(channel, position, ratio)

	return &ChannelData{
		ActiveChannel:   channel,
		TrendLines:      trendLines,
		CurrentPosition: position,
		PriceRatio:      ratio,
		Quality:         channel.Quality,
		Direction:       channel.Direction,
		Analysis:        analysis,
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

			// 计算斜率
			timeDiff := float64(point2.Time - point1.Time)
			if timeDiff <= 0 {
				continue
			}

			slope := (point2.Price - point1.Price) / timeDiff
			intercept := point1.Price - slope*float64(point1.Time)

			trendLine := &TrendLine{
				Type:      lineType,
				Points:    []*SwingPoint{point1, point2},
				Slope:     slope,
				Intercept: intercept,
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
func (ca *ChannelAnalyzer) calculateTrendLineHits(trendLine *TrendLine, points []*SwingPoint) int {
	hits := 0
	for _, point := range points {
		expectedPrice := trendLine.Slope*float64(point.Time) + trendLine.Intercept
		distance := math.Abs(point.Price-expectedPrice) / expectedPrice
		if distance <= ca.config.MaxDistance {
			hits++
		}
	}
	return hits
}

// calculateTrendLineStrength 计算趋势线强度
func (ca *ChannelAnalyzer) calculateTrendLineStrength(trendLine *TrendLine) float64 {
	strength := 0.0

	// 基于命中次数
	strength += float64(trendLine.Touches) * 2.0

	// 基于时间跨度
	if len(trendLine.Points) >= 2 {
		timeSpan := trendLine.Points[len(trendLine.Points)-1].Time - trendLine.Points[0].Time
		days := float64(timeSpan) / (24 * 3600 * 1000)
		strength += math.Min(days/7, 3.0) // 最多加3分
	}

	// 基于点强度
	pointStrength := 0.0
	for _, point := range trendLine.Points {
		pointStrength += point.Strength
	}
	strength += pointStrength / float64(len(trendLine.Points))

	return strength
}

// findBestChannel 寻找最佳通道
func (ca *ChannelAnalyzer) findBestChannel(trendLines []*TrendLine, swingPoints []*SwingPoint, currentPrice float64) *Channel {
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

			channel := ca.createChannel(line1, line2, currentPrice)
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

// createChannel 创建通道
func (ca *ChannelAnalyzer) createChannel(line1, line2 *TrendLine, currentPrice float64) *Channel {
	var upperLine, lowerLine *TrendLine

	// 确定上下线
	currentTime := float64(time.Now().UnixMilli())
	price1 := line1.Slope*currentTime + line1.Intercept
	price2 := line2.Slope*currentTime + line2.Intercept

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

	// 确定方向
	direction := "flat"
	if upperLine.Slope > 0.001 {
		direction = "up"
	} else if upperLine.Slope < -0.001 {
		direction = "down"
	}

	// 计算通道年龄
	upperTime := float64(upperLine.Points[0].Time)
	lowerTime := float64(lowerLine.Points[0].Time)
	age := time.Now().UnixMilli() - int64(math.Min(upperTime, lowerTime))

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

	// 基于通道年龄（较新的通道更好）
	ageDays := float64(channel.Age) / (24 * 3600 * 1000)
	if ageDays <= 7 {
		score += 2.0
	} else if ageDays <= 30 {
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
func (ca *ChannelAnalyzer) calculatePricePosition(currentPrice float64, channel *Channel) (string, float64) {
	currentTime := float64(time.Now().UnixMilli())
	upperPrice := channel.UpperLine.Slope*currentTime + channel.UpperLine.Intercept
	lowerPrice := channel.LowerLine.Slope*currentTime + channel.LowerLine.Intercept

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