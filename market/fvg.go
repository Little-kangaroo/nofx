package market

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// FVGAnalyzer FVG分析器
type FVGAnalyzer struct {
	config FVGConfig
}

// NewFVGAnalyzer 创建新的FVG分析器
func NewFVGAnalyzer() *FVGAnalyzer {
	return &FVGAnalyzer{
		config: defaultFVGConfig,
	}
}

// NewFVGAnalyzerWithConfig 使用自定义配置创建FVG分析器
func NewFVGAnalyzerWithConfig(config FVGConfig) *FVGAnalyzer {
	return &FVGAnalyzer{
		config: config,
	}
}

// Analyze 分析K线数据识别FVG
func (fvg *FVGAnalyzer) Analyze(klines []Kline) *FVGData {
	if len(klines) < 3 {
		return nil
	}

	var bullishFVGs []*FairValueGap
	var bearishFVGs []*FairValueGap

	// 扫描所有K线寻找FVG
	for i := 1; i < len(klines)-1; i++ {
		// 检查看涨FVG
		if bullishGap := fvg.identifyBullishFVG(klines, i); bullishGap != nil {
			bullishFVGs = append(bullishFVGs, bullishGap)
		}

		// 检查看跌FVG
		if bearishGap := fvg.identifyBearishFVG(klines, i); bearishGap != nil {
			bearishFVGs = append(bearishFVGs, bearishGap)
		}
	}

	// 更新所有FVG的状态
	allFVGs := append(bullishFVGs, bearishFVGs...)
	fvg.updateFVGStatuses(allFVGs, klines)

	// 筛选活跃FVG
	activeFVGs := fvg.filterActiveFVGs(allFVGs)

	// 计算FVG强度和质量
	for _, gap := range allFVGs {
		fvg.calculateFVGStrength(gap, klines)
		fvg.assessFVGQuality(gap)
	}

	// 验证FVG
	if fvg.config.EnableValidation {
		for _, gap := range allFVGs {
			gap.Validation = fvg.validateFVG(gap, klines)
		}
	}

	// 计算统计信息
	statistics := fvg.calculateStatistics(bullishFVGs, bearishFVGs, activeFVGs)

	return &FVGData{
		BullishFVGs:  bullishFVGs,
		BearishFVGs:  bearishFVGs,
		ActiveFVGs:   activeFVGs,
		Config:       &fvg.config,
		Statistics:   statistics,
		LastAnalysis: time.Now().UnixMilli(),
	}
}

// identifyBullishFVG 识别看涨FVG
func (fvg *FVGAnalyzer) identifyBullishFVG(klines []Kline, index int) *FairValueGap {
	if index < 1 || index >= len(klines)-1 {
		return nil
	}

	prev := klines[index-1]
	curr := klines[index]
	next := klines[index+1]

	// 看涨FVG条件：前一根K线的高点 < 后一根K线的低点
	// 说明中间存在价格缺口，表明买方力量强劲
	if prev.High >= next.Low {
		return nil
	}

	// 计算缺口大小
	gapLow := prev.High
	gapHigh := next.Low
	gapWidth := gapHigh - gapLow
	gapWidthPercent := gapWidth / gapLow * 100

	// 检查缺口大小是否在配置范围内
	if gapWidthPercent < fvg.config.MinGapPercent*100 || gapWidthPercent > fvg.config.MaxGapPercent*100 {
		return nil
	}

	// 检查成交量确认（如果需要）
	if fvg.config.RequireVolConf {
		avgVolume := fvg.calculateAverageVolume(klines, index-10, index)
		if curr.Volume < avgVolume*fvg.config.MinVolumeRatio {
			return nil
		}
	}

	// 确定形成类型
	formationType := fvg.determineFormationType(klines, index, BullishFVG)

	// 创建看涨FVG
	gap := &FairValueGap{
		ID:           fmt.Sprintf("bull_fvg_%d", index),
		Type:         BullishFVG,
		UpperBound:   gapHigh,
		LowerBound:   gapLow,
		CenterPrice:  (gapHigh + gapLow) / 2,
		Width:        gapWidth,
		WidthPercent: gapWidthPercent,
		Origin: &FVGOrigin{
			KlineIndex:    index,
			PreviousCandle: fvg.createCandleInfo(&prev, index-1),
			CurrentCandle:  fvg.createCandleInfo(&curr, index),
			NextCandle:     fvg.createCandleInfo(&next, index+1),
			ImpulsiveMove:  (next.Close - prev.Close) / prev.Close * 100,
			TimeFrame:      fvg.config.TimeFrames[0],
			FormationType:  formationType,
		},
		Status:       FVGStatusFresh,
		CreationTime: curr.OpenTime,
		IsActive:     true,
		IsFilled:     false,
		TouchCount:   0,
		FillProgress: 0,
	}

	// 计算成交量上下文
	gap.VolumeContext = fvg.calculateVolumeContext(klines, index)

	return gap
}

// identifyBearishFVG 识别看跌FVG
func (fvg *FVGAnalyzer) identifyBearishFVG(klines []Kline, index int) *FairValueGap {
	if index < 1 || index >= len(klines)-1 {
		return nil
	}

	prev := klines[index-1]
	curr := klines[index]
	next := klines[index+1]

	// 看跌FVG条件：前一根K线的低点 > 后一根K线的高点
	// 说明中间存在价格缺口，表明卖方力量强劲
	if prev.Low <= next.High {
		return nil
	}

	// 计算缺口大小
	gapHigh := prev.Low
	gapLow := next.High
	gapWidth := gapHigh - gapLow
	gapWidthPercent := gapWidth / gapHigh * 100

	// 检查缺口大小是否在配置范围内
	if gapWidthPercent < fvg.config.MinGapPercent*100 || gapWidthPercent > fvg.config.MaxGapPercent*100 {
		return nil
	}

	// 检查成交量确认（如果需要）
	if fvg.config.RequireVolConf {
		avgVolume := fvg.calculateAverageVolume(klines, index-10, index)
		if curr.Volume < avgVolume*fvg.config.MinVolumeRatio {
			return nil
		}
	}

	// 确定形成类型
	formationType := fvg.determineFormationType(klines, index, BearishFVG)

	// 创建看跌FVG
	gap := &FairValueGap{
		ID:           fmt.Sprintf("bear_fvg_%d", index),
		Type:         BearishFVG,
		UpperBound:   gapHigh,
		LowerBound:   gapLow,
		CenterPrice:  (gapHigh + gapLow) / 2,
		Width:        gapWidth,
		WidthPercent: gapWidthPercent,
		Origin: &FVGOrigin{
			KlineIndex:    index,
			PreviousCandle: fvg.createCandleInfo(&prev, index-1),
			CurrentCandle:  fvg.createCandleInfo(&curr, index),
			NextCandle:     fvg.createCandleInfo(&next, index+1),
			ImpulsiveMove:  (prev.Close - next.Close) / prev.Close * 100,
			TimeFrame:      fvg.config.TimeFrames[0],
			FormationType:  formationType,
		},
		Status:       FVGStatusFresh,
		CreationTime: curr.OpenTime,
		IsActive:     true,
		IsFilled:     false,
		TouchCount:   0,
		FillProgress: 0,
	}

	// 计算成交量上下文
	gap.VolumeContext = fvg.calculateVolumeContext(klines, index)

	return gap
}

// createCandleInfo 创建K线信息
func (fvg *FVGAnalyzer) createCandleInfo(kline *Kline, index int) *CandleInfo {
	return &CandleInfo{
		Index:     index,
		Open:      kline.Open,
		High:      kline.High,
		Low:       kline.Low,
		Close:     kline.Close,
		Volume:    kline.Volume,
		Timestamp: kline.OpenTime,
	}
}

// determineFormationType 确定FVG形成类型
func (fvg *FVGAnalyzer) determineFormationType(klines []Kline, index int, fvgType FVGType) FormationType {
	if index < 5 || index >= len(klines)-5 {
		return FormationContinuation
	}

	// 分析前后K线的趋势
	preTrend := fvg.calculateTrend(klines, index-5, index-1)
	postTrend := fvg.calculateTrend(klines, index+1, index+5)

	if fvgType == BullishFVG {
		if preTrend > 0.01 && postTrend > 0.01 {
			return FormationContinuation // 上涨延续
		} else if preTrend < -0.01 && postTrend > 0.01 {
			return FormationReversal // 反转
		} else if preTrend > 0.01 && postTrend < -0.01 {
			return FormationPullback // 回调
		} else {
			return FormationBreakout // 突破
		}
	} else {
		if preTrend < -0.01 && postTrend < -0.01 {
			return FormationContinuation // 下跌延续
		} else if preTrend > 0.01 && postTrend < -0.01 {
			return FormationReversal // 反转
		} else if preTrend < -0.01 && postTrend > 0.01 {
			return FormationPullback // 回调
		} else {
			return FormationBreakout // 突破
		}
	}
}

// calculateTrend 计算趋势
func (fvg *FVGAnalyzer) calculateTrend(klines []Kline, start, end int) float64 {
	if start >= end || end >= len(klines) {
		return 0
	}

	startPrice := klines[start].Close
	endPrice := klines[end].Close
	return (endPrice - startPrice) / startPrice
}

// calculateVolumeContext 计算成交量上下文
func (fvg *FVGAnalyzer) calculateVolumeContext(klines []Kline, index int) *FVGVolume {
	if index < 10 || index >= len(klines) {
		return &FVGVolume{}
	}

	formationVolume := klines[index].Volume
	avgVolume := fvg.calculateAverageVolume(klines, index-10, index)
	
	volumeRatio := 1.0
	if avgVolume > 0 {
		volumeRatio = formationVolume / avgVolume
	}

	return &FVGVolume{
		FormationVolume:    formationVolume,
		AverageVolume:      avgVolume,
		VolumeRatio:        volumeRatio,
		TouchVolumes:       make([]float64, 0),
		VolumeConfirmation: volumeRatio >= fvg.config.MinVolumeRatio,
	}
}

// calculateAverageVolume 计算平均成交量
func (fvg *FVGAnalyzer) calculateAverageVolume(klines []Kline, start, end int) float64 {
	if start < 0 {
		start = 0
	}
	if end >= len(klines) {
		end = len(klines) - 1
	}
	if start >= end {
		return 0
	}

	totalVolume := 0.0
	count := 0

	for i := start; i <= end; i++ {
		totalVolume += klines[i].Volume
		count++
	}

	if count == 0 {
		return 0
	}

	return totalVolume / float64(count)
}

// updateFVGStatuses 更新FVG状态
func (fvg *FVGAnalyzer) updateFVGStatuses(gaps []*FairValueGap, klines []Kline) {
	if len(klines) == 0 {
		return
	}

	currentTime := klines[len(klines)-1].OpenTime
	currentPrice := klines[len(klines)-1].Close

	for _, gap := range gaps {
		// 计算年龄
		age := fvg.calculateAge(gap, klines)
		if age > fvg.config.MaxAge {
			gap.Status = FVGStatusExpired
			gap.IsActive = false
			continue
		}

		// 检查是否被填补
		fillProgress := fvg.calculateFillProgress(gap, klines, gap.Origin.KlineIndex)
		gap.FillProgress = fillProgress

		if fillProgress >= fvg.config.FillThreshold*100 {
			gap.Status = FVGStatusFilled
			gap.IsFilled = true
			gap.IsActive = false
			gap.FillTime = currentTime
		} else if fillProgress > 20 { // 20%以上算部分填补
			gap.Status = FVGStatusPartialFill
			gap.IsPartialFill = true
		}

		// 计算触及次数
		touchCount := fvg.countTouches(gap, klines)
		gap.TouchCount = touchCount

		if touchCount > fvg.config.MaxTouchCount {
			gap.IsActive = false
		} else if touchCount > 0 {
			gap.Status = FVGStatusTested
			gap.LastTouch = currentTime
		}

		// 检查当前价格是否在FVG内
		if currentPrice >= gap.LowerBound && currentPrice <= gap.UpperBound {
			gap.LastTouch = currentTime
		}
	}
}

// calculateAge 计算FVG年龄
func (fvg *FVGAnalyzer) calculateAge(gap *FairValueGap, klines []Kline) int {
	originIndex := gap.Origin.KlineIndex
	return len(klines) - originIndex - 1
}

// calculateFillProgress 计算填补进度
func (fvg *FVGAnalyzer) calculateFillProgress(gap *FairValueGap, klines []Kline, startIndex int) float64 {
	if startIndex >= len(klines)-1 {
		return 0
	}

	maxPenetration := 0.0
	gapWidth := gap.Width

	// 从FVG形成后开始检查价格对缺口的填补程度
	for i := startIndex + 1; i < len(klines); i++ {
		kline := klines[i]
		
		if gap.Type == BullishFVG {
			// 看涨FVG：检查价格向下填补的程度
			if kline.Low <= gap.UpperBound {
				penetration := gap.UpperBound - kline.Low
				if penetration > maxPenetration {
					maxPenetration = penetration
				}
			}
		} else {
			// 看跌FVG：检查价格向上填补的程度
			if kline.High >= gap.LowerBound {
				penetration := kline.High - gap.LowerBound
				if penetration > maxPenetration {
					maxPenetration = penetration
				}
			}
		}
	}

	if gapWidth <= 0 {
		return 0
	}

	return (maxPenetration / gapWidth) * 100
}

// countTouches 计算FVG触及次数
func (fvg *FVGAnalyzer) countTouches(gap *FairValueGap, klines []Kline) int {
	if gap.Origin.KlineIndex >= len(klines)-1 {
		return 0
	}

	touches := 0
	startIndex := gap.Origin.KlineIndex + 1

	for i := startIndex; i < len(klines); i++ {
		kline := klines[i]
		
		// 检查K线是否触及FVG区域
		if fvg.doesCandleTouchFVG(kline, gap) {
			touches++
			// 记录触及时的成交量
			if gap.VolumeContext != nil {
				gap.VolumeContext.TouchVolumes = append(gap.VolumeContext.TouchVolumes, kline.Volume)
			}
		}
	}

	return touches
}

// doesCandleTouchFVG 检查K线是否触及FVG
func (fvg *FVGAnalyzer) doesCandleTouchFVG(kline Kline, gap *FairValueGap) bool {
	return !(kline.High < gap.LowerBound || kline.Low > gap.UpperBound)
}

// filterActiveFVGs 筛选活跃FVG
func (fvg *FVGAnalyzer) filterActiveFVGs(gaps []*FairValueGap) []*FairValueGap {
	var activeFVGs []*FairValueGap

	for _, gap := range gaps {
		if gap.IsActive && !gap.IsFilled {
			activeFVGs = append(activeFVGs, gap)
		}
	}

	return activeFVGs
}

// calculateFVGStrength 计算FVG强度
func (fvg *FVGAnalyzer) calculateFVGStrength(gap *FairValueGap, klines []Kline) {
	strength := 0.0

	// 基于缺口宽度的强度
	strength += math.Min(gap.WidthPercent*20, 30) // 最多30分

	// 基于冲动移动的强度
	if gap.Origin != nil {
		impulsiveMove := math.Abs(gap.Origin.ImpulsiveMove)
		strength += math.Min(impulsiveMove*10, 25) // 最多25分
	}

	// 基于成交量的强度
	if gap.VolumeContext != nil && gap.VolumeContext.VolumeRatio > 1 {
		volumeBonus := math.Min((gap.VolumeContext.VolumeRatio-1)*15, 20) // 最多20分
		strength += volumeBonus
	}

	// 基于形成类型的强度
	if gap.Origin != nil {
		switch gap.Origin.FormationType {
		case FormationBreakout:
			strength += 15 // 突破形成加15分
		case FormationReversal:
			strength += 12 // 反转形成加12分
		case FormationContinuation:
			strength += 8  // 延续形成加8分
		case FormationPullback:
			strength += 5  // 回调形成加5分
		}
	}

	// 基于未填补时间的强度加成
	age := fvg.calculateAge(gap, klines)
	if age > 10 && !gap.IsFilled {
		ageBonus := math.Min(float64(age-10)*0.5, 10) // 最多10分
		strength += ageBonus
	}

	// 触及次数惩罚
	touchPenalty := float64(gap.TouchCount) * 3
	strength = math.Max(strength-touchPenalty, 0)

	gap.Strength = math.Min(strength, 100)
}

// assessFVGQuality 评估FVG质量
func (fvg *FVGAnalyzer) assessFVGQuality(gap *FairValueGap) {
	score := gap.Strength

	// 基于填补进度调整质量
	if gap.FillProgress > 50 {
		score *= 0.7 // 填补超过50%降低质量
	} else if gap.FillProgress < 20 {
		score += 10 // 填补很少加分
	}

	// 基于成交量确认调整质量
	if gap.VolumeContext != nil && gap.VolumeContext.VolumeConfirmation {
		score += 5
	}

	// 基于形成类型调整质量
	if gap.Origin != nil {
		switch gap.Origin.FormationType {
		case FormationBreakout, FormationReversal:
			score += 8
		case FormationContinuation:
			score += 5
		}
	}

	if score >= 80 {
		gap.Quality = FVQualityHigh
	} else if score >= 60 {
		gap.Quality = FVQualityMedium
	} else {
		gap.Quality = FVQualityLow
	}
}

// validateFVG 验证FVG
func (fvg *FVGAnalyzer) validateFVG(gap *FairValueGap, klines []Kline) *FVGValidation {
	validation := &FVGValidation{}

	if gap.Origin.KlineIndex >= len(klines)-5 {
		return validation
	}

	// 检查是否有反应
	reactionStrength := fvg.checkReaction(gap, klines)
	validation.HasReaction = reactionStrength > 0.01 // 1%以上的反应
	validation.ReactionStrength = reactionStrength

	// 检查持有强度
	holdingStrength := fvg.checkHoldingStrength(gap, klines)
	validation.HoldingStrength = holdingStrength

	// 检查是否有反转迹象
	validation.ReversalSign = fvg.checkReversalSigns(gap, klines)

	// 成交量验证
	validation.VolumeValidation = gap.VolumeContext != nil && gap.VolumeContext.VolumeConfirmation

	// 时间验证
	age := fvg.calculateAge(gap, klines)
	validation.TimeValidation = age >= 3 && age <= fvg.config.MaxAge // 存在时间合理

	return validation
}

// checkReaction 检查FVG反应强度
func (fvg *FVGAnalyzer) checkReaction(gap *FairValueGap, klines []Kline) float64 {
	maxReaction := 0.0
	startIndex := gap.Origin.KlineIndex + 1

	for i := startIndex; i < len(klines) && i < startIndex+10; i++ {
		kline := klines[i]
		
		// 检查触及FVG后的价格反应
		if fvg.doesCandleTouchFVG(kline, gap) && i < len(klines)-2 {
			// 检查接下来几根K线的反应
			for j := 1; j <= 3 && i+j < len(klines); j++ {
				nextKline := klines[i+j]
				var reaction float64
				
				if gap.Type == BullishFVG {
					// 看涨FVG：期望价格向上反弹
					reaction = (nextKline.Close - kline.Low) / kline.Low
				} else {
					// 看跌FVG：期望价格向下反弹
					reaction = (kline.High - nextKline.Close) / kline.High
				}
				
				if reaction > maxReaction {
					maxReaction = reaction
				}
			}
		}
	}

	return maxReaction
}

// checkHoldingStrength 检查FVG持有强度
func (fvg *FVGAnalyzer) checkHoldingStrength(gap *FairValueGap, klines []Kline) float64 {
	if gap.TouchCount == 0 {
		return 100 // 未触及表示持有强度最高
	}

	// 基于填补进度计算持有强度
	holdingStrength := 100 - gap.FillProgress

	// 基于触及后的反应调整
	if gap.Validation != nil && gap.Validation.HasReaction {
		holdingStrength += gap.Validation.ReactionStrength * 50
	}

	return math.Min(holdingStrength, 100)
}

// checkReversalSigns 检查反转迹象
func (fvg *FVGAnalyzer) checkReversalSigns(gap *FairValueGap, klines []Kline) bool {
	if gap.TouchCount < 2 || gap.Origin.KlineIndex >= len(klines)-5 {
		return false
	}

	// 检查最近的价格行为是否显示反转迹象
	recentStart := len(klines) - 5
	if recentStart < gap.Origin.KlineIndex {
		recentStart = gap.Origin.KlineIndex + 1
	}

	trend := fvg.calculateTrend(klines, recentStart, len(klines)-1)
	
	if gap.Type == BullishFVG && trend < -0.02 {
		return true // 看涨FVG但价格下跌
	} else if gap.Type == BearishFVG && trend > 0.02 {
		return true // 看跌FVG但价格上涨
	}

	return false
}

// calculateStatistics 计算FVG统计信息
func (fvg *FVGAnalyzer) calculateStatistics(bullishFVGs, bearishFVGs, activeFVGs []*FairValueGap) *FVGStatistics {
	stats := &FVGStatistics{
		TotalBullishFVGs: len(bullishFVGs),
		TotalBearishFVGs: len(bearishFVGs),
		QualityDistribution: make(map[FVGQuality]int),
	}

	// 统计活跃FVG
	for _, gap := range activeFVGs {
		if gap.Type == BullishFVG {
			stats.ActiveBullishFVGs++
		} else {
			stats.ActiveBearishFVGs++
		}
	}

	allFVGs := append(bullishFVGs, bearishFVGs...)
	if len(allFVGs) == 0 {
		return stats
	}

	// 计算平均指标
	totalWidth := 0.0
	totalStrength := 0.0
	filledCount := 0
	successCount := 0
	totalFillTime := 0.0

	for _, gap := range allFVGs {
		totalWidth += gap.WidthPercent
		totalStrength += gap.Strength
		
		// 质量分布统计
		stats.QualityDistribution[gap.Quality]++
		
		// 填补统计
		if gap.IsFilled {
			filledCount++
			if gap.FillTime > gap.CreationTime {
				fillTime := float64(gap.FillTime-gap.CreationTime) / (1000 * 3600) // 转换为小时
				totalFillTime += fillTime
			}
		}
		
		// 成功统计（产生反应的FVG）
		if gap.Validation != nil && gap.Validation.HasReaction {
			successCount++
		}
	}

	stats.AvgFVGWidth = totalWidth / float64(len(allFVGs))
	stats.AvgFVGStrength = totalStrength / float64(len(allFVGs))
	stats.FillRate = float64(filledCount) / float64(len(allFVGs)) * 100
	stats.SuccessRate = float64(successCount) / float64(len(allFVGs)) * 100

	if filledCount > 0 {
		stats.AvgFillTime = totalFillTime / float64(filledCount)
	}

	return stats
}

// UpdateConfig 更新配置
func (fvg *FVGAnalyzer) UpdateConfig(config FVGConfig) {
	fvg.config = config
}

// GetConfig 获取当前配置
func (fvg *FVGAnalyzer) GetConfig() FVGConfig {
	return fvg.config
}

// GenerateSignals 生成基于FVG的交易信号
func (fvg *FVGAnalyzer) GenerateSignals(fvgData *FVGData, currentPrice float64) []*FVGSignal {
	if fvgData == nil {
		return nil
	}

	var signals []*FVGSignal
	timestamp := time.Now().UnixMilli()

	// 检查活跃FVG的信号
	for _, gap := range fvgData.ActiveFVGs {
		if signal := fvg.generateFVGSignal(gap, currentPrice, timestamp); signal != nil {
			signals = append(signals, signal)
		}
	}

	// 按置信度排序
	sort.Slice(signals, func(i, j int) bool {
		return signals[i].Confidence > signals[j].Confidence
	})

	return signals
}

// generateFVGSignal 为单个FVG生成信号
func (fvg *FVGAnalyzer) generateFVGSignal(gap *FairValueGap, currentPrice float64, timestamp int64) *FVGSignal {
	// 计算当前价格与FVG的位置关系
	distanceToFVG := fvg.calculateDistanceToFVG(gap, currentPrice)
	
	// 检查是否在FVG内
	inFVG := currentPrice >= gap.LowerBound && currentPrice <= gap.UpperBound

	var signal *FVGSignal

	if inFVG {
		// 在FVG内，生成反应信号
		signal = fvg.generateReactionSignal(gap, currentPrice, timestamp)
	} else if distanceToFVG < 0.01 { // 1%范围内
		// 接近FVG，生成入场信号
		signal = fvg.generateEntrySignal(gap, currentPrice, timestamp, distanceToFVG)
	}

	// 检查拒绝信号
	if signal == nil {
		signal = fvg.generateRejectionSignal(gap, currentPrice, timestamp)
	}

	return signal
}

// generateReactionSignal 生成FVG反应信号
func (fvg *FVGAnalyzer) generateReactionSignal(gap *FairValueGap, currentPrice float64, timestamp int64) *FVGSignal {
	var action SignalAction
	var entry, stopLoss, takeProfit float64
	var description string

	if gap.Type == BullishFVG {
		action = ActionBuy
		entry = currentPrice
		stopLoss = gap.LowerBound * 0.995
		takeProfit = currentPrice + (gap.Width * 2)
		description = fmt.Sprintf("价格在看涨FVG %.2f-%.2f内，预期向上反应", gap.LowerBound, gap.UpperBound)
	} else {
		action = ActionSell
		entry = currentPrice
		stopLoss = gap.UpperBound * 1.005
		takeProfit = currentPrice - (gap.Width * 2)
		description = fmt.Sprintf("价格在看跌FVG %.2f-%.2f内，预期向下反应", gap.LowerBound, gap.UpperBound)
	}

	// 计算风险收益比
	risk := math.Abs(entry - stopLoss)
	reward := math.Abs(takeProfit - entry)
	riskReward := 0.0
	if risk > 0 {
		riskReward = reward / risk
	}

	// 计算置信度
	confidence := gap.Strength * 0.9
	if gap.Quality == FVQualityHigh {
		confidence += 10
	}
	if gap.TouchCount == 0 {
		confidence += 5 // 首次触及加分
	}
	if gap.Validation != nil && gap.Validation.VolumeValidation {
		confidence += 5
	}

	return &FVGSignal{
		Type:         FVGSignalReaction,
		FVG:          gap,
		CurrentPrice: currentPrice,
		Action:       action,
		Entry:        entry,
		StopLoss:     stopLoss,
		TakeProfit:   takeProfit,
		RiskReward:   riskReward,
		Confidence:   math.Min(confidence, 100),
		Strength:     gap.Strength,
		Description:  description,
		Timestamp:    timestamp,
	}
}

// generateEntrySignal 生成FVG入场信号
func (fvg *FVGAnalyzer) generateEntrySignal(gap *FairValueGap, currentPrice float64, timestamp int64, distance float64) *FVGSignal {
	var action SignalAction
	var entry, stopLoss, takeProfit float64
	var description string

	if gap.Type == BullishFVG {
		if currentPrice > gap.UpperBound {
			// 价格在看涨FVG上方，等待回调
			action = ActionBuy
			entry = gap.CenterPrice
			stopLoss = gap.LowerBound * 0.995
			takeProfit = currentPrice + (gap.Width * 1.5)
			description = fmt.Sprintf("等待回调至看涨FVG %.2f，准备买入", gap.CenterPrice)
		} else {
			return nil
		}
	} else {
		if currentPrice < gap.LowerBound {
			// 价格在看跌FVG下方，等待反弹
			action = ActionSell
			entry = gap.CenterPrice
			stopLoss = gap.UpperBound * 1.005
			takeProfit = currentPrice - (gap.Width * 1.5)
			description = fmt.Sprintf("等待反弹至看跌FVG %.2f，准备卖出", gap.CenterPrice)
		} else {
			return nil
		}
	}

	// 计算风险收益比
	risk := math.Abs(entry - stopLoss)
	reward := math.Abs(takeProfit - entry)
	riskReward := 0.0
	if risk > 0 {
		riskReward = reward / risk
	}

	// 计算置信度（距离越近置信度越高）
	confidence := gap.Strength * (1 - distance/0.01) * 0.8
	if gap.Quality == FVQualityHigh {
		confidence += 8
	}

	return &FVGSignal{
		Type:         FVGSignalFillEntry,
		FVG:          gap,
		CurrentPrice: currentPrice,
		Action:       action,
		Entry:        entry,
		StopLoss:     stopLoss,
		TakeProfit:   takeProfit,
		RiskReward:   riskReward,
		Confidence:   math.Min(confidence, 100),
		Strength:     gap.Strength,
		Description:  description,
		Timestamp:    timestamp,
	}
}

// generateRejectionSignal 生成FVG拒绝信号
func (fvg *FVGAnalyzer) generateRejectionSignal(gap *FairValueGap, currentPrice float64, timestamp int64) *FVGSignal {
	// 检查FVG是否显示拒绝迹象
	if gap.TouchCount < 2 || gap.FillProgress > 30 {
		return nil
	}

	// 检查是否有强反应历史
	if gap.Validation == nil || !gap.Validation.HasReaction || gap.Validation.ReactionStrength < 0.02 {
		return nil
	}

	var signal *FVGSignal
	distance := fvg.calculateDistanceToFVG(gap, currentPrice)

	if distance < 0.005 { // 0.5%范围内
		var action SignalAction
		var entry, stopLoss, takeProfit float64
		var description string

		if gap.Type == BullishFVG {
			action = ActionBuy
			entry = gap.LowerBound
			stopLoss = gap.LowerBound * 0.99
			takeProfit = gap.UpperBound + gap.Width
			description = fmt.Sprintf("看涨FVG %.2f显示强拒绝，预期反弹", gap.LowerBound)
		} else {
			action = ActionSell
			entry = gap.UpperBound
			stopLoss = gap.UpperBound * 1.01
			takeProfit = gap.LowerBound - gap.Width
			description = fmt.Sprintf("看跌FVG %.2f显示强拒绝，预期下跌", gap.UpperBound)
		}

		// 计算风险收益比
		risk := math.Abs(entry - stopLoss)
		reward := math.Abs(takeProfit - entry)
		riskReward := 0.0
		if risk > 0 {
			riskReward = reward / risk
		}

		// 高置信度的拒绝信号
		confidence := gap.Strength * 0.85 + gap.Validation.ReactionStrength*100

		signal = &FVGSignal{
			Type:         FVGSignalRejection,
			FVG:          gap,
			CurrentPrice: currentPrice,
			Action:       action,
			Entry:        entry,
			StopLoss:     stopLoss,
			TakeProfit:   takeProfit,
			RiskReward:   riskReward,
			Confidence:   math.Min(confidence, 100),
			Strength:     gap.Strength,
			Description:  description,
			Timestamp:    timestamp,
		}
	}

	return signal
}

// calculateDistanceToFVG 计算价格到FVG的距离
func (fvg *FVGAnalyzer) calculateDistanceToFVG(gap *FairValueGap, currentPrice float64) float64 {
	if currentPrice >= gap.LowerBound && currentPrice <= gap.UpperBound {
		return 0 // 在FVG内
	}

	var distance float64
	if currentPrice > gap.UpperBound {
		distance = (currentPrice - gap.UpperBound) / gap.UpperBound
	} else {
		distance = (gap.LowerBound - currentPrice) / gap.LowerBound
	}

	return distance
}

// FindNearestFVGs 查找最近的FVG
func (fvg *FVGAnalyzer) FindNearestFVGs(fvgData *FVGData, currentPrice float64, maxDistance float64) []*FairValueGap {
	if fvgData == nil {
		return nil
	}

	var nearFVGs []*FairValueGap

	for _, gap := range fvgData.ActiveFVGs {
		distance := fvg.calculateDistanceToFVG(gap, currentPrice)
		if distance <= maxDistance {
			nearFVGs = append(nearFVGs, gap)
		}
	}

	// 按距离排序
	sort.Slice(nearFVGs, func(i, j int) bool {
		dist1 := fvg.calculateDistanceToFVG(nearFVGs[i], currentPrice)
		dist2 := fvg.calculateDistanceToFVG(nearFVGs[j], currentPrice)
		return dist1 < dist2
	})

	return nearFVGs
}

// GetFVGsByType 按类型获取FVG
func (fvg *FVGAnalyzer) GetFVGsByType(fvgData *FVGData, fvgType FVGType) []*FairValueGap {
	if fvgData == nil {
		return nil
	}

	var fvgs []*FairValueGap

	targetFVGs := fvgData.ActiveFVGs
	if fvgType == BullishFVG {
		targetFVGs = fvgData.BullishFVGs
	} else if fvgType == BearishFVG {
		targetFVGs = fvgData.BearishFVGs
	}

	for _, gap := range targetFVGs {
		if gap.Type == fvgType && gap.IsActive {
			fvgs = append(fvgs, gap)
		}
	}

	return fvgs
}

// GetStrongestFVGs 获取最强的FVG
func (fvg *FVGAnalyzer) GetStrongestFVGs(fvgData *FVGData, count int) []*FairValueGap {
	if fvgData == nil {
		return nil
	}

	// 复制活跃FVG
	fvgs := make([]*FairValueGap, len(fvgData.ActiveFVGs))
	copy(fvgs, fvgData.ActiveFVGs)

	// 按强度排序
	sort.Slice(fvgs, func(i, j int) bool {
		return fvgs[i].Strength > fvgs[j].Strength
	})

	// 返回最强的几个
	if count > len(fvgs) {
		count = len(fvgs)
	}

	return fvgs[:count]
}

// GetFVGByID 根据ID获取FVG
func (fvg *FVGAnalyzer) GetFVGByID(fvgData *FVGData, id string) *FairValueGap {
	if fvgData == nil {
		return nil
	}

	allFVGs := append(fvgData.BullishFVGs, fvgData.BearishFVGs...)
	for _, gap := range allFVGs {
		if gap.ID == id {
			return gap
		}
	}

	return nil
}