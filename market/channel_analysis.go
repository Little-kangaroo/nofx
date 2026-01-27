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

	// 🔥 P0-CH-05修复：平行度判定稳定性参数
	SlopeEpsilon float64 // 斜率最小阈值，避免水平通道判定失败（默认1e-6）
}

// ChannelData 通道分析数据
type ChannelData struct {
	// 🔥 P0-CH-01修复：坐标系标注字段
	Timeframe   string `json:"timeframe"`    // 时间框架（5m/15m/1h/4h）
	Coord       string `json:"coord"`        // 坐标系类型（固定"index"）
	RefIndex    int    `json:"ref_index"`    // 参考索引（len(klines)-1）
	RefOpenTime int64  `json:"ref_open_time,omitempty"` // 参考K线开盘时间

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

	// 🔥 P0-CH-02修复：ATR来源标注
	ATRMode         string        `json:"atr_mode,omitempty"` // ATR计算模式（normal/fallback_pct）
	ATRConfidence   float64       `json:"atr_confidence,omitempty"` // ATR置信度

	// 🔥 P1-CH-06修复：突破强度与距离信息
	RawRatio            float64 `json:"raw_ratio"`              // 原始比例（不clamp）
	DistanceToUpper     float64 `json:"distance_to_upper"`      // 到上轨距离（绝对值）
	DistanceToLower     float64 `json:"distance_to_lower"`      // 到下轨距离（绝对值）
	DistanceToUpperATR  float64 `json:"distance_to_upper_atr"`  // 到上轨距离（ATR倍数）
	DistanceToLowerATR  float64 `json:"distance_to_lower_atr"`  // 到下轨距离（ATR倍数）
	BreakoutDistance    float64 `json:"breakout_distance"`      // 突破距离（绝对值，0表示未突破）
	BreakoutDistanceATR float64 `json:"breakout_distance_atr"`  // 突破距离（ATR倍数）

	// 诊断信息
	Notes           []string      `json:"notes,omitempty"`  // 诊断信息
}

// Channel 通道结构
type Channel struct {
	UpperLine  *TrendLine `json:"upper_line"`
	LowerLine  *TrendLine `json:"lower_line"`
	MiddleLine *TrendLine `json:"middle_line"`
	Width      float64    `json:"width"`
	Quality    float64    `json:"quality"`
	Direction  string     `json:"direction"`

	// 🔥 P0-CH-04修复：Age语义拆分
	AgeBars    int        `json:"age_bars"`    // 通道存在时间（K线根数）
	AgeMs      int64      `json:"age_ms,omitempty"` // 通道存在时间（毫秒，可选）
}

// 🔥 P1-CH-06新增：价格位置详细信息
type ChannelPositionInfo struct {
	Position            string  // Inside/BreakUp/BreakDown
	PriceRatio          float64 // clamp到[0,1]
	RawRatio            float64 // 不clamp
	DistanceToUpper     float64
	DistanceToLower     float64
	DistanceToUpperATR  float64
	DistanceToLowerATR  float64
	BreakoutDistance    float64
	BreakoutDistanceATR float64
}

// NewChannelAnalyzer 创建通道分析器
func NewChannelAnalyzer() *ChannelAnalyzer {
	return &ChannelAnalyzer{
		config: ChannelAnalysisConfig{
			SwingLookback:     5,    // 🔥 修复：从7降到5，提高摆动点识别率
			MinSwingStrength:  0.5,  // 🔥 修复：从0.6降到0.5，降低摆动点强度要求
			MinTrendLineHits:  2,    // 🔥 修复：从3降到2，降低趋势线命中要求
			MaxDistance:       0.020, // 🔥 修复：从1.5%提高到2%，放宽距离容忍度
			MinChannelWidth:   0.02,  // 2%最小宽度（传统模式）
			MaxChannelWidth:   0.18,  // 18%最大宽度（传统模式）
			ParallelTolerance: 0.10,  // 🔥 修复：从8%提高到10%，放宽平行度要求
			QualityThreshold:  0.60,  // 🔥 修复：从0.75降到0.60，降低质量阈值

			// 🔥 修复：放宽ATR动态宽度标准，提高通道识别率
			EnableATRWidthStandards: true,  // 启用ATR标准
			MinChannelWidthATR:      0.5,   // 🔥 修复：从0.8降到0.5，允许更窄的通道
			MaxChannelWidthATR:      6.0,   // 🔥 修复：从4.0提高到6.0，允许更宽的通道
			OptimalWidthATRMin:      1.0,   // 🔥 修复：从1.2降到1.0
			OptimalWidthATRMax:      3.0,   // 🔥 修复：从2.5提高到3.0

			// 🔥 P0-CH-05修复：平行度判定稳定性
			SlopeEpsilon:            1e-6,  // 斜率最小阈值
		},
	}
}

// Analyze 执行通道分析
// 🔥 P0-CH-01/02修复：增加timeframe参数，统一ATR口径，标注坐标系
func (ca *ChannelAnalyzer) Analyze(klines []Kline, currentPrice float64, timeframe string) *ChannelData {
	// 输入验证
	if klines == nil || len(klines) < 50 {
		return &ChannelData{
			Timeframe: timeframe,
			Coord:     "index",
			Analysis:  "数据不足，无法进行通道分析",
			Notes:     []string{"insufficient_data"},
		}
	}

	if currentPrice <= 0 {
		return &ChannelData{
			Timeframe: timeframe,
			Coord:     "index",
			Analysis:  "无效的当前价格",
			Notes:     []string{"invalid_price"},
		}
	}

	// 🔥 P0-CH-02修复：使用全局ATRManager统一ATR计算
	atrEntry := GetGlobalATRManager().GetATR14(klines, timeframe, currentPrice)
	atr14 := atrEntry.Value

	// 坐标系参考点
	refIndex := len(klines) - 1
	refOpenTime := klines[refIndex].OpenTime

	// 使用全部K线进行分析（最大优化结构视野）
	analysisData := klines

	// 1. 识别摆动点
	swingPoints := ca.identifySwingPoints(analysisData)
	if len(swingPoints) < 4 {
		return &ChannelData{
			Timeframe:     timeframe,
			Coord:         "index",
			RefIndex:      refIndex,
			RefOpenTime:   refOpenTime,
			Analysis:      "摆动点不足，无法构建通道",
			ATRMode:       atrEntry.Mode,
			ATRConfidence: atrEntry.Confidence,
			Notes:         []string{"insufficient_swing_points"},
		}
	}

	// 2. 计算趋势线
	trendLines := ca.calculateTrendLines(swingPoints)
	if len(trendLines) < 2 {
		return &ChannelData{
			Timeframe:     timeframe,
			Coord:         "index",
			RefIndex:      refIndex,
			RefOpenTime:   refOpenTime,
			TrendLines:    trendLines,
			Analysis:      "趋势线不足，无法构建通道",
			ATRMode:       atrEntry.Mode,
			ATRConfidence: atrEntry.Confidence,
			Notes:         []string{"insufficient_trend_lines"},
		}
	}

	// 3. 构建最佳通道（包含ATR宽度验证）
	channel := ca.findBestChannelWithATR(trendLines, swingPoints, currentPrice, refIndex, atr14)
	if channel == nil {
		return &ChannelData{
			Timeframe:     timeframe,
			Coord:         "index",
			RefIndex:      refIndex,
			RefOpenTime:   refOpenTime,
			TrendLines:    trendLines,
			Analysis:      "未找到有效通道",
			ATRMode:       atrEntry.Mode,
			ATRConfidence: atrEntry.Confidence,
			Notes:         []string{"no_valid_channel"},
		}
	}

	// 4. 计算当前价格位置
	posInfo := ca.calculatePricePosition(currentPrice, channel, refIndex, atr14)

	// 🔥 P0-CH-01修复：计算ATR分析 - 传入正确的refIndex
	widthATR, atrGrade, atrAnalysis := ca.calculateATRAnalysis(channel, currentPrice, atr14, refIndex)

	// 5. 生成分析描述（整合ATR分析）
	analysis := ca.generateAnalysisWithATR(channel, posInfo.Position, posInfo.PriceRatio, atrGrade, atrAnalysis)

	// 构建诊断信息
	notes := []string{}
	if atrEntry.Mode != "normal" {
		notes = append(notes, "atr_fallback_used")
	}

	return &ChannelData{
		Timeframe:       timeframe,
		Coord:           "index",
		RefIndex:        refIndex,
		RefOpenTime:     refOpenTime,
		ActiveChannel:   channel,
		TrendLines:      trendLines,
		CurrentPosition: posInfo.Position,
		PriceRatio:      posInfo.PriceRatio,
		Quality:         channel.Quality,
		Direction:       channel.Direction,
		Analysis:        analysis,

		// ATR分析结果
		WidthATR:      widthATR,
		ATRGrade:      atrGrade,
		ATRAnalysis:   atrAnalysis,
		ATRMode:       atrEntry.Mode,
		ATRConfidence: atrEntry.Confidence,

		// 🔥 P1-CH-06新增：突破距离与位置信息
		RawRatio:            posInfo.RawRatio,
		DistanceToUpper:     posInfo.DistanceToUpper,
		DistanceToLower:     posInfo.DistanceToLower,
		DistanceToUpperATR:  posInfo.DistanceToUpperATR,
		DistanceToLowerATR:  posInfo.DistanceToLowerATR,
		BreakoutDistance:    posInfo.BreakoutDistance,
		BreakoutDistanceATR: posInfo.BreakoutDistanceATR,

		Notes:         notes,
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

	// 🔥 简化版：只要是局部高低点，就给一个基础强度
	// 价格范围评分（权重40%）
	priceRange := (klines[index].High - klines[index].Low) / klines[index].Close
	priceScore := math.Min(priceRange*10, 1.0) * 0.4

	// 成交量评分（权重30%）
	volumeScore := 0.15 // 默认给一半分数
	if len(klines) > index+20 {
		avgVolume := 0.0
		for i := index - 10; i <= index+10 && i < len(klines); i++ {
			if i >= 0 {
				avgVolume += klines[i].Volume
			}
		}
		avgVolume /= 21
		if avgVolume > 0 {
			volumeRatio := klines[index].Volume / avgVolume
			volumeScore = math.Min(volumeRatio/2.0, 1.0) * 0.3
		}
	}

	// 位置评分（权重30%）- 简化：只要通过了isLocalHigh/Low检查，就给满分
	positionScore := 0.3

	// 总强度
	strength := priceScore + volumeScore + positionScore
	return math.Min(strength, 1.0)
}

// calculateATR 计算平均真实波幅
// 🔥 P0-CH-03修复：边界条件检查
// 注意：此函数仅作为fallback，主要应使用ATRManager
func (ca *ChannelAnalyzer) calculateATR(klines []Kline, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}

	var trSum float64
	count := 0

	for i := len(klines) - period; i < len(klines); i++ {
		// 🔥 P0-CH-03修复：边界检查修正
		if i < 1 {  // 需要访问i-1，所以i必须>=1
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

			// 🔥 P0-C1修复：评分（传入正确的currentIndex参数）
			score := ca.scoreChannelWithATR(channel, swingPoints, atr14, currentIndex)
			if score > bestScore && channel.Quality >= ca.config.QualityThreshold {
				bestScore = score
				bestChannel = channel
			}
		}
	}

	return bestChannel
}

// findBestChannel 寻找最佳通道（已废弃，保留仅为兼容）
// 🔥 P0-CH-03修复：此函数已废弃，请使用findBestChannelWithATR
// 如果必须使用，请确保传入正确的klines和atr14
func (ca *ChannelAnalyzer) findBestChannel(trendLines []*TrendLine, swingPoints []*SwingPoint, currentPrice float64, currentIndex int) *Channel {
	// 直接返回nil，强制使用ATR版本
	// 如果有调用此函数的地方，应该改为调用findBestChannelWithATR
	return nil
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
// 🔥 P0-CH-05修复：平行度判定支持水平通道
func (ca *ChannelAnalyzer) canFormChannel(line1, line2 *TrendLine) bool {
	// 必须是不同类型的线
	if line1.Type == line2.Type {
		return false
	}

	// 🔥 P0-CH-05修复：使用带下限的归一化差，避免水平通道判定失败
	s1, s2 := line1.Slope, line2.Slope
	maxSlope := math.Max(math.Abs(s1), math.Abs(s2))

	// 使用SlopeEpsilon作为最小阈值，避免除以接近0的数
	denominator := math.Max(maxSlope, ca.config.SlopeEpsilon)

	// 检查平行度
	if math.Abs(s1-s2) > ca.config.ParallelTolerance*denominator {
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

	// 🔥 P1-C2修复：使用价格归一化的斜率阈值，确保跨币种/跨周期可比性
	direction := "flat"
	avgSlope := (upperLine.Slope + lowerLine.Slope) / 2
	// 动态阈值：斜率相对于当前价格的比例
	normalizedSlope := math.Abs(avgSlope) / currentPrice
	// 0.001 = 0.1%的价格变化率作为方向性阈值
	threshold := 0.001 
	if normalizedSlope > threshold {
		if avgSlope > 0 {
			direction = "up"
		} else {
			direction = "down"
		}
	}

	// 🔥 P0-CH-04修复：Age语义拆分
	upperStartIndex := upperLine.Points[0].Index
	lowerStartIndex := lowerLine.Points[0].Index
	channelStartIndex := minInt(upperStartIndex, lowerStartIndex)
	ageBars := currentIndex - channelStartIndex

	// 可选：计算真实毫秒差
	ageMs := int64(0)
	if len(upperLine.Points) > 0 && len(lowerLine.Points) > 0 {
		startTime := minInt64(upperLine.Points[0].Time, lowerLine.Points[0].Time)
		// 注意：这里需要klines数据才能获取currentIndex对应的时间
		// 暂时设为0，如果需要可以传入klines参数
		// ageMs = klines[currentIndex].OpenTime - startTime
		_ = startTime // 避免未使用警告
	}

	return &Channel{
		UpperLine:  upperLine,
		LowerLine:  lowerLine,
		MiddleLine: middleLine,
		Width:      percentageWidth, // 保持百分比宽度用于兼容性
		Direction:  direction,
		AgeBars:    ageBars,  // 🔥 修复：使用AgeBars
		AgeMs:      ageMs,    // 🔥 修复：可选的毫秒差
	}
}

// scoreChannelWithATR 为通道评分（ATR增强版）
// 🔥 P0-C1修复：传入正确的currentIndex，避免使用len(swingPoints)导致的坐标错误
func (ca *ChannelAnalyzer) scoreChannelWithATR(channel *Channel, swingPoints []*SwingPoint, atr14 float64, currentIndex int) float64 {
	score := 0.0

	// 基础评分：基于趋势线强度
	score += (channel.UpperLine.Strength + channel.LowerLine.Strength) / 2

	// 基于命中次数
	totalHits := channel.UpperLine.Touches + channel.LowerLine.Touches
	score += float64(totalHits) * 0.5

	// 🔥 P0-CH-04修复：基于索引的通道年龄评分，避免毫秒转换误差
	ageInIndices := float64(channel.AgeBars) // channel.AgeBars现在是索引差
	if ageInIndices <= 50 { // 50根K线内认为是新通道
		score += 2.0
	} else if ageInIndices <= 200 { // 200根K线内认为是较新通道
		score += 1.0
	}

	// 🔥 P0-C1修复：ATR宽度质量评分 - 使用正确的当前索引
	if atr14 > 0 {
		// 🔥 P0-C1修复：使用传入的currentIndex而不是len(swingPoints)
		currentIndexFloat := float64(currentIndex)
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

	// 🔥 P1-C2修复：使用价格归一化的斜率阈值，确保跨币种/跨周期可比性
	direction := "flat"
	avgSlope := (upperLine.Slope + lowerLine.Slope) / 2
	// 动态阈值：斜率相对于当前价格的比例
	normalizedSlope := math.Abs(avgSlope) / currentPrice
	// 0.001 = 0.1%的价格变化率作为方向性阈值
	threshold := 0.001 
	if normalizedSlope > threshold {
		if avgSlope > 0 {
			direction = "up"
		} else {
			direction = "down"
		}
	}

	// 🔥 P0-CH-04修复：Age语义拆分
	upperStartIndex := upperLine.Points[0].Index
	lowerStartIndex := lowerLine.Points[0].Index
	channelStartIndex := minInt(upperStartIndex, lowerStartIndex)
	ageBars := currentIndex - channelStartIndex

	// 可选：计算真实毫秒差（暂时设为0）
	ageMs := int64(0)

	return &Channel{
		UpperLine:  upperLine,
		LowerLine:  lowerLine,
		MiddleLine: middleLine,
		Width:      width,
		Direction:  direction,
		AgeBars:    ageBars,  // 🔥 修复：使用AgeBars
		AgeMs:      ageMs,    // 🔥 修复：可选的毫秒差
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

	// 🔥 P0-CH-04修复：基于索引的通道年龄评分，避免毫秒转换误差
	ageInIndices := float64(channel.AgeBars) // channel.AgeBars现在是索引差
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
// 🔥 P1-CH-06修复：返回完整的位置信息，包括突破距离
func (ca *ChannelAnalyzer) calculatePricePosition(currentPrice float64, channel *Channel, currentIndex int, atr14 float64) *ChannelPositionInfo {
	// 🔥 修复：使用当前索引替代time.Now()
	currentIndexFloat := float64(currentIndex)
	upperPrice := channel.UpperLine.Slope*currentIndexFloat + channel.UpperLine.Intercept
	lowerPrice := channel.LowerLine.Slope*currentIndexFloat + channel.LowerLine.Intercept

	// 防御性检查：避免除零
	if math.Abs(upperPrice-lowerPrice) < 1e-8 {
		return &ChannelPositionInfo{
			Position:   "unknown",
			PriceRatio: 0.5,
			RawRatio:   0.5,
		}
	}

	// 计算原始比例（不clamp）
	rawRatio := (currentPrice - lowerPrice) / (upperPrice - lowerPrice)

	// 计算clamp后的比例（兼容旧版）
	clampedRatio := math.Max(0, math.Min(1, rawRatio))

	// 计算到上下轨的距离
	distToUpper := math.Abs(currentPrice - upperPrice)
	distToLower := math.Abs(currentPrice - lowerPrice)

	// ATR归一化距离
	distToUpperATR := 0.0
	distToLowerATR := 0.0
	if atr14 > 0 {
		distToUpperATR = distToUpper / atr14
		distToLowerATR = distToLower / atr14
	}

	// 确定位置和突破距离
	position := "middle"
	breakoutDist := 0.0
	breakoutDistATR := 0.0

	if currentPrice > upperPrice*1.01 {
		// 向上突破
		position = "break_up"
		breakoutDist = currentPrice - upperPrice
		if atr14 > 0 {
			breakoutDistATR = breakoutDist / atr14
		}
	} else if currentPrice < lowerPrice*0.99 {
		// 向下突破
		position = "break_down"
		breakoutDist = lowerPrice - currentPrice
		if atr14 > 0 {
			breakoutDistATR = breakoutDist / atr14
		}
	} else if rawRatio > 0.8 {
		position = "upper"
	} else if rawRatio < 0.2 {
		position = "lower"
	}

	return &ChannelPositionInfo{
		Position:            position,
		PriceRatio:          clampedRatio,
		RawRatio:            rawRatio,
		DistanceToUpper:     distToUpper,
		DistanceToLower:     distToLower,
		DistanceToUpperATR:  distToUpperATR,
		DistanceToLowerATR:  distToLowerATR,
		BreakoutDistance:    breakoutDist,
		BreakoutDistanceATR: breakoutDistATR,
	}
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
// 🔥 P0-C1修复：基于ATR的通道宽度质量分析 - 使用正确的currentIndex
func (ca *ChannelAnalyzer) calculateATRAnalysis(channel *Channel, currentPrice float64, atr14 float64, currentIndex int) (float64, string, string) {
	if atr14 == 0 {
		return 0, "未知", "ATR数据不足，无法进行ATR分析"
	}

	// 🔥 P0-C1修复：计算通道宽度的ATR倍数 - 使用正确的当前索引
	currentIndexFloat := float64(currentIndex)
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
// 🔥 P0-CH-01修复：趋势线价格计算辅助函数（强制使用索引坐标系）
// TrendLinePriceAtIndex 计算趋势线在指定索引处的价格
// 此函数确保所有消费端使用统一的索引坐标系，禁止使用时间戳
func TrendLinePriceAtIndex(line *TrendLine, index int) float64 {
	if line == nil {
		return 0
	}
	return line.Slope*float64(index) + line.Intercept
}

// 🔥 P0-CH-04修复：辅助函数（minInt已在utils.go中定义）
func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
