package market

import (
	"math"
	"sort"
	"time"
)

// VPVRAnalyzer 成交量分布分析器
type VPVRAnalyzer struct {
	config VPVRConfig
}

// NewVPVRAnalyzer 创建新的VPVR分析器
func NewVPVRAnalyzer() *VPVRAnalyzer {
	return &VPVRAnalyzer{
		config: defaultVPVRConfig,
	}
}

// NewVPVRAnalyzerWithConfig 使用自定义配置创建VPVR分析器
func NewVPVRAnalyzerWithConfig(config VPVRConfig) *VPVRAnalyzer {
	return &VPVRAnalyzer{
		config: config,
	}
}

// Analyze 分析K线数据生成成交量分布
func (va *VPVRAnalyzer) Analyze(klines []Kline) *VolumeProfile {
	if len(klines) == 0 {
		return nil
	}

	// 计算价格级别
	levels := va.calculatePriceLevels(klines)
	if len(levels) == 0 {
		return nil
	}

	// 计算统计信息
	stats := va.calculateVolumeStats(levels)

	// 查找POC（Point of Control）
	poc := va.findPOC(levels)

	// 计算价值区域
	valueArea := va.calculateValueArea(levels, stats.TotalVolume)
	vah, val := va.findValueAreaBounds(levels, valueArea)

	// 标记价值区域内的级别
	va.markValueAreaLevels(levels, val, vah)

	return &VolumeProfile{
		POC:       poc,
		VAH:       vah,
		VAL:       val,
		ValueArea: valueArea,
		Levels:    levels,
		Config:    &va.config,
		Stats:     stats,
	}
}

// calculatePriceLevels 计算每个价格级别的成交量
func (va *VPVRAnalyzer) calculatePriceLevels(klines []Kline) []*PriceLevel {
	if len(klines) == 0 {
		return nil
	}

	// 确定价格范围
	minPrice, maxPrice := va.findPriceRange(klines)
	
	// 计算价格级别数量
	priceRange := maxPrice - minPrice
	levelCount := int(priceRange / va.config.TickSize)
	
	// 限制最大级别数以避免过度分割
	maxLevels := 200
	if levelCount > maxLevels {
		va.config.TickSize = priceRange / float64(maxLevels)
		levelCount = maxLevels
	}

	// 初始化价格级别映射
	levelMap := make(map[float64]*PriceLevel)

	// 遍历每根K线，计算每个价格级别的成交量
	for _, kline := range klines {
		va.distributePriceVolume(kline, levelMap, minPrice)
	}

	// 转换为切片并排序
	levels := make([]*PriceLevel, 0, len(levelMap))
	for _, level := range levelMap {
		if level.Volume >= va.config.MinVolume {
			levels = append(levels, level)
		}
	}

	// 按价格排序
	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Price < levels[j].Price
	})

	// 计算成交量百分比
	totalVolume := 0.0
	for _, level := range levels {
		totalVolume += level.Volume
	}

	if totalVolume > 0 {
		for _, level := range levels {
			level.VolumePercent = level.Volume / totalVolume * 100
		}
	}

	// 应用平滑处理
	if va.config.SmoothingFactor > 1.0 {
		va.smoothVolumes(levels)
	}

	return levels
}

// findPriceRange 找到价格范围
func (va *VPVRAnalyzer) findPriceRange(klines []Kline) (float64, float64) {
	minPrice := klines[0].Low
	maxPrice := klines[0].High

	for _, kline := range klines {
		if kline.Low < minPrice {
			minPrice = kline.Low
		}
		if kline.High > maxPrice {
			maxPrice = kline.High
		}
	}

	return minPrice, maxPrice
}

// distributePriceVolume 将K线的成交量分配到相应的价格级别
func (va *VPVRAnalyzer) distributePriceVolume(kline Kline, levelMap map[float64]*PriceLevel, minPrice float64) {
	// 计算K线的价格范围
	priceRange := kline.High - kline.Low
	if priceRange == 0 {
		priceRange = va.config.TickSize
	}

	// 将成交量按价格范围均匀分配
	// 这是一个简化的分配方法，实际应用中可能需要更复杂的模型
	numLevels := int(priceRange/va.config.TickSize) + 1
	if numLevels == 0 {
		numLevels = 1
	}

	volumePerLevel := kline.Volume / float64(numLevels)
	
	// 估算买卖成交量分配
	// 如果收盘价高于开盘价，认为买盘更强
	buyRatio := 0.5
	if kline.Close > kline.Open {
		buyRatio = 0.6 + 0.2*(kline.Close-kline.Open)/(kline.High-kline.Low)
	} else if kline.Close < kline.Open {
		buyRatio = 0.4 - 0.2*(kline.Open-kline.Close)/(kline.High-kline.Low)
	}
	buyRatio = math.Max(0.1, math.Min(0.9, buyRatio))

	buyVolumePerLevel := volumePerLevel * buyRatio
	sellVolumePerLevel := volumePerLevel * (1 - buyRatio)

	// 分配到各个价格级别
	for price := kline.Low; price <= kline.High; price += va.config.TickSize {
		levelPrice := va.roundToTick(price, minPrice)
		
		level, exists := levelMap[levelPrice]
		if !exists {
			level = &PriceLevel{
				Price: levelPrice,
			}
			levelMap[levelPrice] = level
		}

		level.Volume += volumePerLevel
		level.BuyVolume += buyVolumePerLevel
		level.SellVolume += sellVolumePerLevel
		level.Transactions++
	}
}

// roundToTick 将价格舍入到指定的tick大小
func (va *VPVRAnalyzer) roundToTick(price, minPrice float64) float64 {
	offset := price - minPrice
	tickLevel := math.Round(offset / va.config.TickSize)
	return minPrice + tickLevel*va.config.TickSize
}

// smoothVolumes 对成交量进行平滑处理
func (va *VPVRAnalyzer) smoothVolumes(levels []*PriceLevel) {
	if len(levels) < 3 || va.config.SmoothingFactor <= 1.0 {
		return
	}

	smoothed := make([]float64, len(levels))
	window := int(va.config.SmoothingFactor)

	for i := range levels {
		sum := 0.0
		count := 0

		// 应用移动平均
		start := i - window/2
		end := i + window/2

		if start < 0 {
			start = 0
		}
		if end >= len(levels) {
			end = len(levels) - 1
		}

		for j := start; j <= end; j++ {
			sum += levels[j].Volume
			count++
		}

		smoothed[i] = sum / float64(count)
	}

	// 更新平滑后的成交量
	for i, level := range levels {
		ratio := smoothed[i] / level.Volume
		level.Volume = smoothed[i]
		level.BuyVolume *= ratio
		level.SellVolume *= ratio
	}
}

// calculateVolumeStats 计算成交量统计信息
func (va *VPVRAnalyzer) calculateVolumeStats(levels []*PriceLevel) *VolumeStats {
	if len(levels) == 0 {
		return &VolumeStats{}
	}

	stats := &VolumeStats{}
	
	var maxLevel, minLevel *PriceLevel
	totalVolumeWeightedPrice := 0.0
	prices := make([]float64, len(levels))

	for i, level := range levels {
		stats.TotalVolume += level.Volume
		stats.TotalBuyVolume += level.BuyVolume
		stats.TotalSellVolume += level.SellVolume
		
		totalVolumeWeightedPrice += level.Price * level.Volume
		prices[i] = level.Price

		if maxLevel == nil || level.Volume > maxLevel.Volume {
			maxLevel = level
		}
		if minLevel == nil || level.Volume < minLevel.Volume {
			minLevel = level
		}
	}

	stats.MaxLevel = maxLevel
	stats.MinLevel = minLevel

	// 计算买卖比
	if stats.TotalSellVolume > 0 {
		stats.BuySellRatio = stats.TotalBuyVolume / stats.TotalSellVolume
	}

	// 计算成交量加权平均价格
	if stats.TotalVolume > 0 {
		stats.AvgPrice = totalVolumeWeightedPrice / stats.TotalVolume
	}

	// 计算中位数价格
	sort.Float64s(prices)
	mid := len(prices) / 2
	if len(prices)%2 == 0 {
		stats.MedianPrice = (prices[mid-1] + prices[mid]) / 2
	} else {
		stats.MedianPrice = prices[mid]
	}

	// 计算价格标准差
	stats.PriceStdDev = va.calculatePriceStdDev(levels, stats.AvgPrice, stats.TotalVolume)

	return stats
}

// calculatePriceStdDev 计算价格标准差（成交量加权）
func (va *VPVRAnalyzer) calculatePriceStdDev(levels []*PriceLevel, avgPrice, totalVolume float64) float64 {
	if totalVolume <= 0 {
		return 0
	}

	variance := 0.0
	for _, level := range levels {
		diff := level.Price - avgPrice
		variance += (diff * diff) * (level.Volume / totalVolume)
	}

	return math.Sqrt(variance)
}

// findPOC 查找POC（Point of Control）- 成交量最大的价格级别
func (va *VPVRAnalyzer) findPOC(levels []*PriceLevel) *PriceLevel {
	if len(levels) == 0 {
		return nil
	}

	poc := levels[0]
	for _, level := range levels {
		if level.Volume > poc.Volume {
			poc = level
		}
	}

	// 标记为POC
	poc.IsPOC = true
	return poc
}

// calculateValueArea 计算价值区域
func (va *VPVRAnalyzer) calculateValueArea(levels []*PriceLevel, totalVolume float64) *ValueArea {
	if len(levels) == 0 || totalVolume <= 0 {
		return &ValueArea{}
	}

	targetVolume := totalVolume * va.config.ValueAreaPercent
	
	// 从POC开始向上下扩展
	poc := va.findPOC(levels)
	if poc == nil {
		return &ValueArea{}
	}

	// 找到POC在数组中的索引
	pocIndex := -1
	for i, level := range levels {
		if level == poc {
			pocIndex = i
			break
		}
	}

	if pocIndex == -1 {
		return &ValueArea{}
	}

	// 从POC开始向两侧扩展
	accumulatedVolume := poc.Volume
	upperIndex := pocIndex
	lowerIndex := pocIndex

	for accumulatedVolume < targetVolume {
		// 选择上方或下方成交量更大的方向扩展
		var upperVolume, lowerVolume float64

		if upperIndex < len(levels)-1 {
			upperVolume = levels[upperIndex+1].Volume
		}
		if lowerIndex > 0 {
			lowerVolume = levels[lowerIndex-1].Volume
		}

		if upperVolume >= lowerVolume && upperIndex < len(levels)-1 {
			upperIndex++
			accumulatedVolume += upperVolume
		} else if lowerIndex > 0 {
			lowerIndex--
			accumulatedVolume += lowerVolume
		} else if upperIndex < len(levels)-1 {
			upperIndex++
			accumulatedVolume += upperVolume
		} else {
			break
		}
	}

	high := levels[upperIndex].Price
	low := levels[lowerIndex].Price
	priceRange := high - low
	
	// 计算完整价格范围
	fullPriceRange := levels[len(levels)-1].Price - levels[0].Price
	priceRangePercent := 0.0
	if fullPriceRange > 0 {
		priceRangePercent = priceRange / fullPriceRange * 100
	}

	// 计算成交量集中度
	concentration := va.calculateConcentration(levels, lowerIndex, upperIndex, accumulatedVolume, totalVolume)

	return &ValueArea{
		High:              high,
		Low:               low,
		VolumePercent:     accumulatedVolume / totalVolume * 100,
		PriceRange:        priceRange,
		PriceRangePercent: priceRangePercent,
		ProfileWidth:      math.Abs(high - low),
		Concentration:     concentration,
	}
}

// calculateConcentration 计算成交量集中度
func (va *VPVRAnalyzer) calculateConcentration(levels []*PriceLevel, lowerIndex, upperIndex int, valueAreaVolume, totalVolume float64) float64 {
	if totalVolume <= 0 || upperIndex <= lowerIndex {
		return 0
	}

	// 集中度 = 价值区域成交量占比 / 价值区域价格范围占比
	volumeRatio := valueAreaVolume / totalVolume
	priceRatio := float64(upperIndex-lowerIndex+1) / float64(len(levels))

	if priceRatio <= 0 {
		return 0
	}

	return volumeRatio / priceRatio
}

// findValueAreaBounds 查找价值区域边界
func (va *VPVRAnalyzer) findValueAreaBounds(levels []*PriceLevel, valueArea *ValueArea) (float64, float64) {
	if valueArea == nil {
		return 0, 0
	}

	vah := valueArea.High
	val := valueArea.Low

	// 微调边界以确保精确度
	for _, level := range levels {
		if level.InValueArea {
			if level.Price > vah {
				vah = level.Price
			}
			if level.Price < val {
				val = level.Price
			}
		}
	}

	return vah, val
}

// markValueAreaLevels 标记价值区域内的级别
func (va *VPVRAnalyzer) markValueAreaLevels(levels []*PriceLevel, val, vah float64) {
	for _, level := range levels {
		level.InValueArea = level.Price >= val && level.Price <= vah
	}
}

// GenerateSignals 生成基于VPVR的交易信号
func (va *VPVRAnalyzer) GenerateSignals(profile *VolumeProfile, currentPrice float64) []*VPVRSignal {
	if profile == nil || profile.POC == nil {
		return nil
	}

	var signals []*VPVRSignal
	timestamp := time.Now().UnixMilli()

	// POC测试信号
	if signal := va.generatePOCSignal(profile, currentPrice, timestamp); signal != nil {
		signals = append(signals, signal)
	}

	// 价值区域突破/回归信号
	if signal := va.generateValueAreaSignal(profile, currentPrice, timestamp); signal != nil {
		signals = append(signals, signal)
	}

	// 高/低成交量级别信号
	if signal := va.generateVolumeSignal(profile, currentPrice, timestamp); signal != nil {
		signals = append(signals, signal)
	}

	// 买卖不平衡信号
	if signal := va.generateImbalanceSignal(profile, currentPrice, timestamp); signal != nil {
		signals = append(signals, signal)
	}

	return signals
}

// generatePOCSignal 生成POC相关信号
func (va *VPVRAnalyzer) generatePOCSignal(profile *VolumeProfile, currentPrice float64, timestamp int64) *VPVRSignal {
	poc := profile.POC
	if poc == nil {
		return nil
	}

	distance := math.Abs(currentPrice - poc.Price) / poc.Price
	
	// 当价格接近POC时生成信号
	if distance < 0.01 { // 1%范围内
		strength := (poc.Volume / profile.Stats.TotalVolume) * 100
		confidence := math.Min(strength*2, 100)

		var action SignalAction
		description := "价格接近POC"

		// 根据买卖成交量比例判断方向
		if poc.BuyVolume > poc.SellVolume*1.2 {
			action = ActionBuy
			description += "，买盘占优，建议买入"
		} else if poc.SellVolume > poc.BuyVolume*1.2 {
			action = ActionSell
			description += "，卖盘占优，建议卖出"
		} else {
			action = ActionHold
			description += "，买卖平衡，建议观望"
		}

		return &VPVRSignal{
			Type:         VPVRSignalPOCTest,
			Level:        poc.Price,
			CurrentPrice: currentPrice,
			Strength:     strength,
			Description:  description,
			Action:       action,
			Confidence:   confidence,
			Timestamp:    timestamp,
		}
	}

	return nil
}

// generateValueAreaSignal 生成价值区域相关信号
func (va *VPVRAnalyzer) generateValueAreaSignal(profile *VolumeProfile, currentPrice float64, timestamp int64) *VPVRSignal {
	if profile.ValueArea == nil {
		return nil
	}

	vah := profile.VAH
	val := profile.VAL
	
	var signal *VPVRSignal

	// 突破价值区域上沿
	if currentPrice > vah*1.005 { // 0.5%突破确认
		signal = &VPVRSignal{
			Type:         VPVRSignalVABreakout,
			Level:        vah,
			CurrentPrice: currentPrice,
			Strength:     (currentPrice - vah) / vah * 100,
			Description:  "突破价值区域上沿，可能继续上涨",
			Action:       ActionBuy,
			Confidence:   70 + profile.ValueArea.Concentration*10,
			Timestamp:    timestamp,
		}
	} else if currentPrice < val*0.995 { // 跌破价值区域下沿
		signal = &VPVRSignal{
			Type:         VPVRSignalVABreakout,
			Level:        val,
			CurrentPrice: currentPrice,
			Strength:     (val - currentPrice) / val * 100,
			Description:  "跌破价值区域下沿，可能继续下跌",
			Action:       ActionSell,
			Confidence:   70 + profile.ValueArea.Concentration*10,
			Timestamp:    timestamp,
		}
	} else if currentPrice > val && currentPrice < vah {
		// 回归价值区域
		centerPrice := (vah + val) / 2
		distanceFromCenter := math.Abs(currentPrice - centerPrice) / centerPrice
		
		signal = &VPVRSignal{
			Type:         VPVRSignalVAReturn,
			Level:        centerPrice,
			CurrentPrice: currentPrice,
			Strength:     (1 - distanceFromCenter) * 100,
			Description:  "价格在价值区域内，趋向均值回归",
			Action:       ActionHold,
			Confidence:   60 - distanceFromCenter*100,
			Timestamp:    timestamp,
		}
	}

	return signal
}

// generateVolumeSignal 生成基于成交量级别的信号
func (va *VPVRAnalyzer) generateVolumeSignal(profile *VolumeProfile, currentPrice float64, timestamp int64) *VPVRSignal {
	// 找到当前价格附近的级别
	var nearestLevel *PriceLevel
	minDistance := math.Inf(1)

	for _, level := range profile.Levels {
		distance := math.Abs(level.Price - currentPrice)
		if distance < minDistance {
			minDistance = distance
			nearestLevel = level
		}
	}

	if nearestLevel == nil {
		return nil
	}

	// 检查是否为高成交量级别
	avgVolume := profile.Stats.TotalVolume / float64(len(profile.Levels))
	volumeRatio := nearestLevel.Volume / avgVolume

	if volumeRatio > 2.0 { // 超过平均成交量2倍
		var action SignalAction
		description := "当前价位成交量异常活跃"

		if nearestLevel.BuyVolume > nearestLevel.SellVolume*1.3 {
			action = ActionBuy
			description += "，买盘占优"
		} else if nearestLevel.SellVolume > nearestLevel.BuyVolume*1.3 {
			action = ActionSell
			description += "，卖盘占优"
		} else {
			action = ActionHold
			description += "，买卖相对平衡"
		}

		return &VPVRSignal{
			Type:         VPVRSignalHighVolume,
			Level:        nearestLevel.Price,
			CurrentPrice: currentPrice,
			Strength:     volumeRatio * 20,
			Description:  description,
			Action:       action,
			Confidence:   math.Min(volumeRatio*25, 100),
			Timestamp:    timestamp,
		}
	} else if volumeRatio < 0.3 { // 低于平均成交量30%
		return &VPVRSignal{
			Type:         VPVRSignalLowVolume,
			Level:        nearestLevel.Price,
			CurrentPrice: currentPrice,
			Strength:     (1 - volumeRatio) * 100,
			Description:  "当前价位成交量稀少，可能缺乏支撑阻力",
			Action:       ActionHold,
			Confidence:   50,
			Timestamp:    timestamp,
		}
	}

	return nil
}

// generateImbalanceSignal 生成买卖不平衡信号
func (va *VPVRAnalyzer) generateImbalanceSignal(profile *VolumeProfile, currentPrice float64, timestamp int64) *VPVRSignal {
	if profile.Stats.BuySellRatio == 0 {
		return nil
	}

	// 检查整体买卖不平衡
	imbalanceThreshold := 1.5
	var signal *VPVRSignal

	if profile.Stats.BuySellRatio > imbalanceThreshold {
		// 买盘占优
		strength := (profile.Stats.BuySellRatio - 1) * 100
		signal = &VPVRSignal{
			Type:         VPVRSignalImbalance,
			Level:        profile.Stats.AvgPrice,
			CurrentPrice: currentPrice,
			Strength:     math.Min(strength, 100),
			Description:  "整体买盘明显强于卖盘，多头氛围浓厚",
			Action:       ActionBuy,
			Confidence:   math.Min(strength*1.5, 100),
			Timestamp:    timestamp,
		}
	} else if profile.Stats.BuySellRatio < (1.0 / imbalanceThreshold) {
		// 卖盘占优
		strength := (1/profile.Stats.BuySellRatio - 1) * 100
		signal = &VPVRSignal{
			Type:         VPVRSignalImbalance,
			Level:        profile.Stats.AvgPrice,
			CurrentPrice: currentPrice,
			Strength:     math.Min(strength, 100),
			Description:  "整体卖盘明显强于买盘，空头氛围浓厚",
			Action:       ActionSell,
			Confidence:   math.Min(strength*1.5, 100),
			Timestamp:    timestamp,
		}
	}

	return signal
}

// UpdateConfig 更新VPVR配置
func (va *VPVRAnalyzer) UpdateConfig(config VPVRConfig) {
	va.config = config
}

// GetConfig 获取当前配置
func (va *VPVRAnalyzer) GetConfig() VPVRConfig {
	return va.config
}