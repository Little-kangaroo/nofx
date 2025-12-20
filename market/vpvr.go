package market

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"
)

// sign 返回数值的符号（-1, 0, 1）
func sign(x float64) float64 {
	if x > 0 {
		return 1
	} else if x < 0 {
		return -1
	}
	return 0
}

// VPVRAnalyzer 成交量分布分析器
type VPVRAnalyzer struct {
	config VPVRConfig
	historicalKlines []Kline // 历史K线数据，用于计算ATR、平均量等指标
}

// NewVPVRAnalyzer 创建新的VPVR分析器
// 🔥 P0-04修复：使用fallbackVPVRConfig替代被移除的defaultVPVRConfig
func NewVPVRAnalyzer() *VPVRAnalyzer {
	return &VPVRAnalyzer{
		config: fallbackVPVRConfig, // 使用降级配置，建议通过NewVPVRAnalyzerWithDynamicConfig创建
		historicalKlines: []Kline{}, // 空的历史数据
	}
}

// 🔥 P0-04修复：新增动态配置构造函数，根据ExchangeMeta和timeframe动态构建VPVR配置
// NewVPVRAnalyzerWithDynamicConfig 使用动态配置创建VPVR分析器（推荐方式）
func NewVPVRAnalyzerWithDynamicConfig(exchangeMeta *ExchangeMeta, timeframe string) *VPVRAnalyzer {
	return &VPVRAnalyzer{
		config: GetDynamicVPVRConfig(exchangeMeta, timeframe), // 使用动态配置避免硬编码扭曲
		historicalKlines: []Kline{}, // 空的历史数据
	}
}

// NewVPVRAnalyzerWithConfig 使用自定义配置创建VPVR分析器
func NewVPVRAnalyzerWithConfig(config VPVRConfig) *VPVRAnalyzer {
	return &VPVRAnalyzer{
		config: config,
		historicalKlines: []Kline{}, // 空的历史数据
	}
}

// NewVPVRAnalyzerWithHistory 创建带历史数据的VPVR分析器（用于精确计算）
func NewVPVRAnalyzerWithHistory(config VPVRConfig, historicalKlines []Kline) *VPVRAnalyzer {
	return &VPVRAnalyzer{
		config: config,
		historicalKlines: historicalKlines,
	}
}

// SetHistoricalData 设置历史K线数据（用于改善买卖比例算法）
func (va *VPVRAnalyzer) SetHistoricalData(klines []Kline) {
	va.historicalKlines = klines
}

// Analyze 分析K线数据生成成交量分布
func (va *VPVRAnalyzer) Analyze(klines []Kline) *VolumeProfile {
	minRequired := 50   // 最小需要50根K线进行成交量分布分析
	recommended := 200  // 建议200根以上获得更准确的价量分布
	
	if len(klines) == 0 {
		log.Printf("🚨🔴 [VPVR分析] ❌ 无K线数据 ❌")
		return nil
	}
	if len(klines) < minRequired {
		log.Printf("🚨🔴 [VPVR分析] ❌ K线数据不足: 需要%d根，实际%d根 ❌", minRequired, len(klines))
		return nil
	}
	if len(klines) < recommended {
		log.Printf("🟡⚠️ [VPVR分析] 价量分析警告: 建议%d根，实际%d根 (可能影响分布精度) ⚠️🟡", recommended, len(klines))
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

	// 确定价格范围
	minPrice, maxPrice := va.findPriceRange(klines)

	// 🔧 Task 7: 修复动态TickSize调整导致的数据抖动问题
	// 使用稳定的TickSize计算策略
	stabilizedTickSize := va.calculateStabilizedTickSize(minPrice, maxPrice, klines)

	// 🔥 P0级新修复：为VPVR添加Context计算，解决StrengthZ和VolRatio为null的问题
	context := va.calculateVPVRContext(levels, stats, poc, vah, val)

	// 🔥 Token优化：筛选关键价格级别（混合策略：POC/VAH/VAL + Top6 volume + Near price 3个）
	currentPrice := klines[len(klines)-1].Close
	filteredLevels := va.filterKeyPriceLevels(levels, poc, vah, val, currentPrice)

	// 🔥 P0-04修复：构建VolumeProfile时添加配置标注，便于复盘和一致性校验
	volumeProfile := &VolumeProfile{
		POC:       poc,
		VAH:       vah,
		VAL:       val,
		ValueArea: valueArea,
		Levels:    filteredLevels, // 使用筛选后的levels
		Config:    &va.config,
		Stats:     stats,
		Context:   context, // 🔥 添加Context字段
		// 🔥 P0-04修复：明确标注实际使用的配置参数
		UsedTimeFrame: va.config.TimeFrame,  // 实际使用的时间框架
		UsedTickSize:  stabilizedTickSize,   // 实际使用的tick_size（可能经过动态调整）
	}

	return volumeProfile
}

// calculatePriceLevels 计算每个价格级别的成交量
func (va *VPVRAnalyzer) calculatePriceLevels(klines []Kline) []*PriceLevel {
	if len(klines) == 0 {
		return nil
	}

	// 确定价格范围
	minPrice, maxPrice := va.findPriceRange(klines)
	
	// 🔧 Task 7: 修复动态TickSize调整导致的数据抖动问题
	// 使用稳定的TickSize计算策略
	stabilizedTickSize := va.calculateStabilizedTickSize(minPrice, maxPrice, klines)
	
	// 计算价格级别数量
	priceRange := maxPrice - minPrice
	levelCount := int(priceRange / stabilizedTickSize)
	
	// 限制最大级别数以避免过度分割
	maxLevels := 200
	if levelCount > maxLevels {
		// 不直接修改config.TickSize，而是使用计算出的稳定TickSize
		stabilizedTickSize = priceRange / float64(maxLevels)
		levelCount = maxLevels
	}

	// 初始化价格级别映射
	levelMap := make(map[float64]*PriceLevel)

	// 遍历每根K线，计算每个价格级别的成交量
	for _, kline := range klines {
		va.distributePriceVolumeStabilized(kline, levelMap, minPrice, stabilizedTickSize)
	}

	// 转换为切片并排序
	levels := make([]*PriceLevel, 0, len(levelMap))
	for _, level := range levelMap {
		// 保留所有成交量数据，让AI分析
		levels = append(levels, level)
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

// distributePriceVolume 将K线的成交量分配到相应的价格级别（修复版 - 填充空洞效应）
func (va *VPVRAnalyzer) distributePriceVolume(kline Kline, levelMap map[float64]*PriceLevel, minPrice float64) {
	// 数据有效性检查
	if kline.Volume <= 0 {
		return
	}
	
	// 计算K线的价格范围
	priceRange := kline.High - kline.Low
	
	// 修复1: 处理价格范围为零的情况（无价格变化K线）
	if priceRange <= 0 {
		// 将所有成交量分配给收盘价位置
		va.addVolumeToSingleLevel(kline.Close, kline.Volume, levelMap, minPrice)
		return
	}

	// 修复2: 成交量分配策略 - 填充所有价格级别
	// 获取K线跨越的所有价格级别
	lowLevelPrice := va.roundToTick(kline.Low, minPrice)
	highLevelPrice := va.roundToTick(kline.High, minPrice)
	
	// 计算价格级别数量
	levelCount := int((highLevelPrice-lowLevelPrice)/va.config.TickSize) + 1
	if levelCount <= 0 {
		levelCount = 1
	}
	
	// 🔧 优化3: 优先使用真实买卖量数据，降级到算法估算
	var buyVolume, sellVolume float64
	if va.useRealBuySellData(kline) {
		// 使用交易所真实买卖量数据
		buyVolume, sellVolume = va.extractRealBuySellVolume(kline)
	} else {
		// 降级：使用改进的算法估算
		buyRatio := va.calculateEnhancedBuySellRatio(kline, priceRange)
		buyVolume = kline.Volume * buyRatio
		sellVolume = kline.Volume * (1 - buyRatio)
	}

	// 修复4: 插值填充所有价格级别，消除空洞效应
	if levelCount == 1 {
		// 单一级别，直接分配
		va.addVolumeToSingleLevel(kline.Close, kline.Volume, levelMap, minPrice)
	} else {
		// 多级别插值分配
		va.distributeVolumeAcrossLevels(kline, buyVolume, sellVolume, lowLevelPrice, highLevelPrice, levelCount, levelMap, minPrice)
	}
}

// distributeVolumeAcrossLevels 在多个价格级别间插值分配成交量
func (va *VPVRAnalyzer) distributeVolumeAcrossLevels(kline Kline, buyVolume, sellVolume float64, lowLevelPrice, highLevelPrice float64, levelCount int, levelMap map[float64]*PriceLevel, minPrice float64) {
	totalVolume := kline.Volume
	
	// 计算关键价位的权重
	ohlcWeights := va.calculateOHLCWeights(kline, lowLevelPrice, highLevelPrice)
	
	// 为每个价格级别分配成交量
	for i := 0; i < levelCount; i++ {
		levelPrice := lowLevelPrice + float64(i)*va.config.TickSize
		
		// 计算该级别的成交量权重
		volumeWeight := va.calculateLevelVolumeWeight(levelPrice, kline, ohlcWeights, levelCount)
		
		// 分配成交量
		level, exists := levelMap[levelPrice]
		if !exists {
			level = &PriceLevel{
				Price: levelPrice,
			}
			levelMap[levelPrice] = level
		}
		
		volumeToAdd := totalVolume * volumeWeight
		level.Volume += volumeToAdd
		level.BuyVolume += buyVolume * volumeWeight
		level.SellVolume += sellVolume * volumeWeight
		level.Transactions++
	}
}

// calculateOHLCWeights 计算OHLC关键价位的权重分布
func (va *VPVRAnalyzer) calculateOHLCWeights(kline Kline, lowLevelPrice, highLevelPrice float64) map[float64]float64 {
	weights := make(map[float64]float64)
	
	// 将OHLC价格映射到价格级别
	openLevel := va.roundToTick(kline.Open, lowLevelPrice)
	highLevel := va.roundToTick(kline.High, lowLevelPrice)
	lowLevel := va.roundToTick(kline.Low, lowLevelPrice)
	closeLevel := va.roundToTick(kline.Close, lowLevelPrice)
	
	// 设置关键价位权重
	weights[openLevel] = 0.25   // 开盘价权重25%
	weights[highLevel] = 0.20   // 最高价权重20%
	weights[lowLevel] = 0.20    // 最低价权重20%
	weights[closeLevel] = 0.35  // 收盘价权重35%
	
	return weights
}

// calculateLevelVolumeWeight 计算单个价格级别的成交量权重
func (va *VPVRAnalyzer) calculateLevelVolumeWeight(levelPrice float64, kline Kline, ohlcWeights map[float64]float64, totalLevels int) float64 {
	// 基础权重：均匀分布
	baseWeight := 1.0 / float64(totalLevels)
	
	// OHLC权重加成
	ohlcWeight, exists := ohlcWeights[levelPrice]
	if !exists {
		ohlcWeight = 0.0
	}
	
	// 距离权重：越接近价格中心，权重越高
	priceCenter := (kline.High + kline.Low) / 2
	distanceFromCenter := math.Abs(levelPrice - priceCenter)
	maxDistance := math.Abs(kline.High - kline.Low) / 2
	
	var distanceWeight float64
	if maxDistance > 0 {
		distanceWeight = 0.1 * (1 - distanceFromCenter/maxDistance)
	}
	
	// 综合权重
	finalWeight := baseWeight + ohlcWeight + distanceWeight
	
	// 确保权重为正数
	return math.Max(finalWeight, 0.001)
}

// addVolumeToSingleLevel 将成交量添加到单一价格级别
func (va *VPVRAnalyzer) addVolumeToSingleLevel(price, volume float64, levelMap map[float64]*PriceLevel, minPrice float64) {
	levelPrice := va.roundToTick(price, minPrice)
	
	level, exists := levelMap[levelPrice]
	if !exists {
		level = &PriceLevel{
			Price: levelPrice,
		}
		levelMap[levelPrice] = level
	}
	
	level.Volume += volume
	// 对于无价格变化的K线，买卖比例设为中性
	level.BuyVolume += volume * 0.5
	level.SellVolume += volume * 0.5
	level.Transactions++
}

// useRealBuySellData 检查是否可以使用真实买卖量数据
func (va *VPVRAnalyzer) useRealBuySellData(kline Kline) bool {
	// 检查K线数据中是否包含真实买卖量字段
	// 这些字段通常来自交易所的高级数据订阅或第三方数据提供商
	
	// 检查是否有 BuyVolume 和 SellVolume 字段
	if kline.BuyVolume > 0 && kline.SellVolume > 0 {
		// 验证数据一致性：买卖量之和应该等于总量（允许小的精度误差）
		totalCalc := kline.BuyVolume + kline.SellVolume
		tolerance := kline.Volume * 0.01 // 允许1%的误差
		
		if math.Abs(totalCalc-kline.Volume) <= tolerance {
			return true
		}
	}
	
	// 检查是否有 TakerBuyVolume 字段
	if kline.TakerBuyVolume > 0 && kline.TakerBuyVolume <= kline.Volume {
		return true
	}
	
	// 检查是否有 TakerBuyBaseVolume 字段（币安等交易所提供）
	if kline.TakerBuyBaseVolume > 0 && kline.TakerBuyBaseVolume <= kline.Volume {
		return true
	}
	
	return false
}

// extractRealBuySellVolume 从K线数据中提取真实的买卖量
func (va *VPVRAnalyzer) extractRealBuySellVolume(kline Kline) (buyVolume, sellVolume float64) {
	// 方式1: 直接使用 BuyVolume 和 SellVolume 字段
	if kline.BuyVolume > 0 && kline.SellVolume > 0 {
		totalCalc := kline.BuyVolume + kline.SellVolume
		tolerance := kline.Volume * 0.01
		
		if math.Abs(totalCalc-kline.Volume) <= tolerance {
			return kline.BuyVolume, kline.SellVolume
		}
	}
	
	// 方式2: 使用 TakerBuyVolume（主动买入量）
	if kline.TakerBuyVolume > 0 && kline.TakerBuyVolume <= kline.Volume {
		buyVolume = kline.TakerBuyVolume
		sellVolume = kline.Volume - kline.TakerBuyVolume
		return buyVolume, sellVolume
	}
	
	// 方式3: 使用现有的TakerBuyBaseVolume（币安API标准字段）
	if kline.TakerBuyBaseVolume > 0 && kline.TakerBuyBaseVolume <= kline.Volume {
		// TakerBuyBaseVolume = 主动买入的基础资产数量
		buyVolume = kline.TakerBuyBaseVolume
		sellVolume = kline.Volume - kline.TakerBuyBaseVolume
		return buyVolume, sellVolume
	}
	
	// 方式4: 基于大单流入流出数据估算（如果可用）
	if kline.BuyerMakerVolume > 0 || kline.SellerMakerVolume > 0 {
		// 主动买入 = TakerBuyVolume
		// 主动卖出 = Total - TakerBuyVolume
		// 但这里我们使用更精确的Maker/Taker分类
		buyVolume = kline.TakerBuyVolume + kline.BuyerMakerVolume
		sellVolume = kline.Volume - buyVolume
		
		// 验证数据有效性
		if buyVolume >= 0 && sellVolume >= 0 && buyVolume+sellVolume <= kline.Volume*1.01 {
			return buyVolume, sellVolume
		}
	}
	
	// 降级：使用增强的算法估算
	priceRange := kline.High - kline.Low
	if priceRange <= 0 {
		return kline.Volume * 0.5, kline.Volume * 0.5
	}
	
	buyRatio := va.calculateEnhancedBuySellRatio(kline, priceRange)
	return kline.Volume * buyRatio, kline.Volume * (1 - buyRatio)
}

// calculateEnhancedBuySellRatio 计算增强的买卖比例（算法估算的改进版本）
func (va *VPVRAnalyzer) calculateEnhancedBuySellRatio(kline Kline, priceRange float64) float64 {
	// 多因子算法估算买卖压力比例
	
	// 因子1: 价格变化方向和幅度（30%权重）
	priceMovement := (kline.Close - kline.Open) / priceRange
	movementBias := 0.3 * priceMovement
	
	// 因子2: 收盘价在区间中的相对位置（20%权重）
	closePosition := (kline.Close - kline.Low) / priceRange
	positionBias := 0.2 * (closePosition - 0.5)
	
	// 因子3: 成交量相对强度（10%权重）
	// 高成交量通常伴随更强的方向性
	avgVolume := va.calculateRecentAvgVolume(10) // 10期平均量
	volumeStrength := 0.0
	if avgVolume > 0 {
		volumeRatio := kline.Volume / avgVolume
		// 成交量越大，方向性越强，增强价格变化的影响
		volumeStrength = 0.1 * math.Min(volumeRatio-1, 1.0) * sign(priceMovement)
	}
	
	// 因子4: 振幅相对强度（10%权重）
	// 高振幅K线通常有更强的买卖不平衡
	atr := va.calculateRecentATR(14) // 14期ATR
	amplitudeBias := 0.0
	if atr > 0 {
		amplitudeRatio := priceRange / atr
		// 振幅大的K线，增强价格变化的影响
		amplitudeBias = 0.1 * math.Min(amplitudeRatio-1, 1.0) * sign(priceMovement)
	}
	
	// 综合买卖压力评估
	buyRatio := 0.5 + movementBias + positionBias + volumeStrength + amplitudeBias
	
	// 确保买卖比例在合理范围内[5%, 95%]
	return math.Max(0.05, math.Min(0.95, buyRatio))
}

// calculateRecentAvgVolume 计算最近N期的平均成交量
func (va *VPVRAnalyzer) calculateRecentAvgVolume(periods int) float64 {
	if len(va.historicalKlines) < periods {
		// 如果历史数据不足，使用默认值
		return 1000000.0
	}
	
	// 使用最近N期的数据计算平均量
	var volumeSum float64
	startIndex := len(va.historicalKlines) - periods
	
	for i := startIndex; i < len(va.historicalKlines); i++ {
		if i >= 0 {
			volumeSum += va.historicalKlines[i].Volume
		}
	}
	
	return volumeSum / float64(periods)
}

// calculateRecentATR 计算最近N期的ATR
func (va *VPVRAnalyzer) calculateRecentATR(periods int) float64 {
	if len(va.historicalKlines) < periods+1 {
		// 如果历史数据不足，使用默认值
		return 100.0
	}
	
	var trSum float64
	startIndex := len(va.historicalKlines) - periods
	
	for i := startIndex; i < len(va.historicalKlines); i++ {
		if i <= 0 {
			continue
		}
		
		current := va.historicalKlines[i]
		previous := va.historicalKlines[i-1]
		
		// 计算True Range
		tr1 := current.High - current.Low
		tr2 := math.Abs(current.High - previous.Close)
		tr3 := math.Abs(current.Low - previous.Close)
		
		tr := math.Max(tr1, math.Max(tr2, tr3))
		trSum += tr
	}
	
	return trSum / float64(periods)
}

// roundToTick 将价格映射到指定的价格级别索引（修复重叠陷阱）
// 使用区间索引方法替代math.Round，避免边界价格分配不确定性
func (va *VPVRAnalyzer) roundToTick(price, minPrice float64) float64 {
	offset := price - minPrice
	// 修复：使用floor函数确保确定性价格级别分配
	// Index = floor((Price - MinPrice) / TickSize)
	levelIndex := int(math.Floor(offset / va.config.TickSize))
	
	// 防止负数索引（理论上不应该发生，但增加保护）
	if levelIndex < 0 {
		levelIndex = 0
	}
	
	return minPrice + float64(levelIndex)*va.config.TickSize
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

	// 🔧 新增：计算POC密度和强度分析
	if stats.MaxLevel != nil && stats.TotalVolume > 0 {
		stats.POCDensity = va.calculatePOCDensity(stats.MaxLevel, stats.TotalVolume)
		stats.POCStrength = va.calculatePOCStrength(stats.MaxLevel, levels)
		stats.POCCluster = va.calculatePOCCluster(stats.MaxLevel, levels, stats.TotalVolume)
	}

	// 🔧 Task 5: 计算价值区宽度占比和市场廓型分析
	if len(levels) > 0 {
		va.calculateValueAreaProfile(levels, stats)
	}
	
	// 🔧 Task 6: 识别并标记HVN/LVN节点
	if len(levels) > 2 {
		va.identifyVolumeNodes(levels, stats)
	}

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

	// 🔧 Task 8: 计算自适应阈值参数
	adaptiveThresholds := va.calculateAdaptiveThresholds(profile, currentPrice)

	// POC测试信号
	if signal := va.generatePOCSignalAdaptive(profile, currentPrice, timestamp, adaptiveThresholds); signal != nil {
		signals = append(signals, signal)
	}

	// 价值区域突破/回归信号
	if signal := va.generateValueAreaSignalAdaptive(profile, currentPrice, timestamp, adaptiveThresholds); signal != nil {
		signals = append(signals, signal)
	}

	// 高/低成交量级别信号
	if signal := va.generateVolumeSignalAdaptive(profile, currentPrice, timestamp, adaptiveThresholds); signal != nil {
		signals = append(signals, signal)
	}

	// 买卖不平衡信号
	if signal := va.generateImbalanceSignalAdaptive(profile, currentPrice, timestamp, adaptiveThresholds); signal != nil {
		signals = append(signals, signal)
	}

	// HVN/LVN节点信号（新增）
	if signal := va.generateHVNLVNSignal(profile, currentPrice, timestamp, adaptiveThresholds); signal != nil {
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

// calculatePOCDensity 计算POC密度 - POC成交量占总量的百分比
func (va *VPVRAnalyzer) calculatePOCDensity(poc *PriceLevel, totalVolume float64) float64 {
	if poc == nil || totalVolume <= 0 {
		return 0
	}
	
	// POC密度 = POC成交量 / 总成交量 * 100%
	density := (poc.Volume / totalVolume) * 100
	return math.Min(density, 100.0) // 确保不超过100%
}

// calculatePOCStrength 计算POC强度 - 相对于周围价格水平的强度评分
func (va *VPVRAnalyzer) calculatePOCStrength(poc *PriceLevel, levels []*PriceLevel) float64 {
	if poc == nil || len(levels) < 3 {
		return 0
	}
	
	// 找到POC在数组中的位置
	pocIndex := -1
	for i, level := range levels {
		if level == poc {
			pocIndex = i
			break
		}
	}
	
	if pocIndex == -1 {
		return 0
	}
	
	// 计算周围价格水平的平均成交量（用于对比）
	var surroundingVolume float64
	var surroundingCount int
	
	// 分析POC前后各5个级别的成交量
	analyzeRange := 5
	for i := pocIndex - analyzeRange; i <= pocIndex + analyzeRange; i++ {
		if i >= 0 && i < len(levels) && i != pocIndex {
			surroundingVolume += levels[i].Volume
			surroundingCount++
		}
	}
	
	if surroundingCount == 0 {
		return 100.0 // 如果没有周围级别，POC强度为最大值
	}
	
	avgSurroundingVolume := surroundingVolume / float64(surroundingCount)
	
	// 计算POC强度：POC相对于周围平均成交量的倍数
	if avgSurroundingVolume <= 0 {
		return 100.0
	}
	
	strengthRatio := poc.Volume / avgSurroundingVolume
	
	// 转换为0-100的评分
	// 2倍 = 60分，3倍 = 70分，5倍 = 80分，10倍以上 = 100分
	var strength float64
	switch {
	case strengthRatio >= 10.0:
		strength = 100.0
	case strengthRatio >= 5.0:
		strength = 80.0 + (strengthRatio-5.0)/5.0*20.0 // 80-100分
	case strengthRatio >= 3.0:
		strength = 70.0 + (strengthRatio-3.0)/2.0*10.0 // 70-80分
	case strengthRatio >= 2.0:
		strength = 60.0 + (strengthRatio-2.0)/1.0*10.0 // 60-70分
	case strengthRatio >= 1.5:
		strength = 40.0 + (strengthRatio-1.5)/0.5*20.0 // 40-60分
	default:
		strength = strengthRatio / 1.5 * 40.0 // 0-40分
	}
	
	return math.Min(strength, 100.0)
}

// calculatePOCCluster 计算POC聚集信息 - 分析POC周围的成交量聚集情况
func (va *VPVRAnalyzer) calculatePOCCluster(poc *PriceLevel, levels []*PriceLevel, totalVolume float64) *POCClusterInfo {
	if poc == nil || len(levels) < 3 {
		return &POCClusterInfo{}
	}
	
	// 找到POC在数组中的位置
	pocIndex := -1
	for i, level := range levels {
		if level == poc {
			pocIndex = i
			break
		}
	}
	
	if pocIndex == -1 {
		return &POCClusterInfo{}
	}
	
	// 定义聚集区间：从POC开始向上下扩展，寻找成交量显著下降的边界
	clusterTop := pocIndex
	clusterBottom := pocIndex
	
	// 定义显著下降的阈值：成交量低于POC的30%
	threshold := poc.Volume * 0.3
	
	// 向上寻找聚集区间边界
	for i := pocIndex + 1; i < len(levels); i++ {
		if levels[i].Volume >= threshold {
			clusterTop = i
		} else {
			break
		}
	}
	
	// 向下寻找聚集区间边界
	for i := pocIndex - 1; i >= 0; i-- {
		if levels[i].Volume >= threshold {
			clusterBottom = i
		} else {
			break
		}
	}
	
	// 计算聚集区间信息
	clusterLevels := clusterTop - clusterBottom + 1
	clusterRange := levels[clusterTop].Price - levels[clusterBottom].Price
	
	var clusterVolume float64
	var volumeProfile []float64
	
	for i := clusterBottom; i <= clusterTop; i++ {
		clusterVolume += levels[i].Volume
		volumeProfile = append(volumeProfile, levels[i].Volume)
	}
	
	// 计算聚集密度：每单位价格的平均成交量
	clusterDensity := 0.0
	if clusterRange > 0 {
		clusterDensity = clusterVolume / clusterRange
	}
	
	// 计算相对强度：聚集区间成交量占总成交量的比例
	relativeStrength := 0.0
	if totalVolume > 0 {
		relativeStrength = clusterVolume / totalVolume
	}
	
	// 计算支撑/阻力强度评级
	supportLevel := va.calculateSupportStrength(clusterVolume, totalVolume, relativeStrength)
	resistanceLevel := va.calculateResistanceStrength(clusterVolume, totalVolume, relativeStrength)
	
	// 计算突破概率
	breakoutProb := va.calculateBreakoutProbability(clusterDensity, relativeStrength, float64(clusterLevels))
	
	return &POCClusterInfo{
		ClusterRange:     clusterRange,
		ClusterVolume:    clusterVolume,
		ClusterLevels:    clusterLevels,
		ClusterDensity:   clusterDensity,
		RelativeStrength: relativeStrength * 100, // 转换为百分比
		SupportLevel:     supportLevel,
		ResistanceLevel:  resistanceLevel,
		BreakoutProb:     breakoutProb,
		VolumeProfile:    volumeProfile,
	}
}

// calculateSupportStrength 计算支撑强度评级
func (va *VPVRAnalyzer) calculateSupportStrength(clusterVolume, totalVolume, relativeStrength float64) float64 {
	// 基于聚集成交量的相对大小计算支撑强度
	// 相对强度越高，支撑越强
	
	var strength float64
	switch {
	case relativeStrength >= 0.4: // 聚集区间占总量40%以上
		strength = 90.0 + relativeStrength*10.0
	case relativeStrength >= 0.25: // 25-40%
		strength = 70.0 + (relativeStrength-0.25)*20.0/0.15
	case relativeStrength >= 0.15: // 15-25%
		strength = 50.0 + (relativeStrength-0.15)*20.0/0.1
	case relativeStrength >= 0.08: // 8-15%
		strength = 30.0 + (relativeStrength-0.08)*20.0/0.07
	default:
		strength = relativeStrength / 0.08 * 30.0
	}
	
	return math.Min(strength, 100.0)
}

// calculateResistanceStrength 计算阻力强度评级
func (va *VPVRAnalyzer) calculateResistanceStrength(clusterVolume, totalVolume, relativeStrength float64) float64 {
	// 阻力强度与支撑强度的计算逻辑类似，但考虑价格突破的难度
	// 通常阻力强度比支撑强度稍低一些
	
	supportStrength := va.calculateSupportStrength(clusterVolume, totalVolume, relativeStrength)
	
	// 阻力强度通常是支撑强度的85-95%
	resistanceFactor := 0.9
	
	return supportStrength * resistanceFactor
}

// calculateBreakoutProbability 计算突破概率
func (va *VPVRAnalyzer) calculateBreakoutProbability(clusterDensity, relativeStrength float64, clusterLevels float64) float64 {
	// 突破概率与聚集密度和相对强度呈反比
	// 密度越高、相对强度越大，突破越困难
	
	// 基础突破概率
	baseProbability := 0.5 // 50%的基础概率
	
	// 密度修正因子（密度越高，突破概率越低）
	densityFactor := 1.0 / (1.0 + clusterDensity/10000.0)
	
	// 强度修正因子（相对强度越高，突破概率越低）
	strengthFactor := 1.0 - relativeStrength*0.8 // 最多降低80%
	
	// 宽度修正因子（聚集区间越宽，突破概率越低）
	widthFactor := 1.0 / (1.0 + clusterLevels/20.0)
	
	// 综合计算突破概率
	breakoutProb := baseProbability * densityFactor * strengthFactor * widthFactor
	
	return math.Max(0.05, math.Min(0.95, breakoutProb)) // 限制在5%-95%范围内
}

// calculateValueAreaProfile 计算价值区宽度占比和市场廓型分析 (Task 5)
func (va *VPVRAnalyzer) calculateValueAreaProfile(levels []*PriceLevel, stats *VolumeStats) {
	if len(levels) < 3 || stats.TotalVolume <= 0 {
		stats.ValueAreaWidthRatio = 0
		stats.MarketProfile = MarketProfileUnknown
		stats.MarketRegimeStrength = 0
		stats.TrendStrength = 0
		stats.ConsolidationLevel = 0
		return
	}

	// 计算总价格范围
	fullPriceRange := levels[len(levels)-1].Price - levels[0].Price
	if fullPriceRange <= 0 {
		stats.ValueAreaWidthRatio = 0
		stats.MarketProfile = MarketProfileUnknown
		return
	}

	// 计算价值区范围
	valueAreaRange := va.calculateValueAreaRange(levels)
	
	// 计算价值区宽度占比
	stats.ValueAreaWidthRatio = valueAreaRange / fullPriceRange

	// 分析市场廓型类型
	stats.MarketProfile = va.classifyMarketProfile(stats, levels, fullPriceRange)
	
	// 计算市场状态强度评分
	stats.MarketRegimeStrength = va.calculateMarketRegimeStrength(stats, levels)
	
	// 计算趋势强度评分
	stats.TrendStrength = va.calculateTrendStrength(stats, levels)
	
	// 计算整理程度评分
	stats.ConsolidationLevel = va.calculateConsolidationLevel(stats, levels)
}

// calculateValueAreaRange 计算价值区的价格范围
func (va *VPVRAnalyzer) calculateValueAreaRange(levels []*PriceLevel) float64 {
	targetVolume := 0.0
	for _, level := range levels {
		targetVolume += level.Volume
	}
	targetVolume *= va.config.ValueAreaPercent

	// 找到POC
	var pocIndex int = -1
	var maxVolume float64 = 0
	for i, level := range levels {
		if level.Volume > maxVolume {
			maxVolume = level.Volume
			pocIndex = i
		}
	}

	if pocIndex == -1 {
		return 0
	}

	// 从POC向两侧扩展
	accumulatedVolume := levels[pocIndex].Volume
	upperIndex := pocIndex
	lowerIndex := pocIndex

	for accumulatedVolume < targetVolume {
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

	return levels[upperIndex].Price - levels[lowerIndex].Price
}

// classifyMarketProfile 分类市场廓型类型
func (va *VPVRAnalyzer) classifyMarketProfile(stats *VolumeStats, levels []*PriceLevel, fullPriceRange float64) MarketProfileType {
	// 基于价值区宽度占比的初步分类
	widthRatio := stats.ValueAreaWidthRatio
	pocDensity := stats.POCDensity
	pocStrength := stats.POCStrength

	// 计算成交量分布的离散程度
	volumeDispersion := va.calculateVolumeDispersion(levels, stats.TotalVolume)
	
	// 计算价格偏斜度（判断是否有明显的方向性）
	priceSkewness := va.calculatePriceSkewness(levels, stats.TotalVolume)

	// 复合分类逻辑
	switch {
	case widthRatio < 0.3 && pocDensity > 15 && pocStrength > 70:
		// 价值区很窄，POC密度高，强度大 -> 趋势型
		if math.Abs(priceSkewness) > 0.5 {
			return MarketProfileTrending
		} else {
			return MarketProfileBalance
		}

	case widthRatio > 0.7 && volumeDispersion > 0.6:
		// 价值区很宽，成交量分散 -> 波动型
		return MarketProfileVolatile

	case widthRatio >= 0.4 && widthRatio <= 0.6 && pocStrength >= 50 && pocStrength <= 70:
		// 价值区适中，POC强度中等 -> 整理型
		return MarketProfileConsolidation

	case widthRatio < 0.25 && pocStrength > 80:
		// 极窄价值区，POC极强 -> 可能突破
		return MarketProfileBreakout

	case volumeDispersion < 0.3 && pocDensity > 20:
		// 成交量集中，POC密度高 -> 平衡型
		return MarketProfileBalance

	case widthRatio > 0.5 && math.Abs(priceSkewness) < 0.3:
		// 宽价值区，无明显偏斜 -> 轮动型
		return MarketProfileRotation

	default:
		return MarketProfileUnknown
	}
}

// calculateVolumeDispersion 计算成交量分布的离散程度
func (va *VPVRAnalyzer) calculateVolumeDispersion(levels []*PriceLevel, totalVolume float64) float64 {
	if totalVolume <= 0 || len(levels) == 0 {
		return 0
	}

	// 计算成交量的基尼系数来衡量分布不均匀程度
	var sortedVolumes []float64
	for _, level := range levels {
		if level.Volume > 0 {
			sortedVolumes = append(sortedVolumes, level.Volume)
		}
	}

	if len(sortedVolumes) < 2 {
		return 0
	}

	// 排序
	for i := 0; i < len(sortedVolumes)-1; i++ {
		for j := i + 1; j < len(sortedVolumes); j++ {
			if sortedVolumes[i] > sortedVolumes[j] {
				sortedVolumes[i], sortedVolumes[j] = sortedVolumes[j], sortedVolumes[i]
			}
		}
	}

	// 计算基尼系数
	n := float64(len(sortedVolumes))
	var giniSum float64
	for i, volume := range sortedVolumes {
		giniSum += (2*float64(i+1) - n - 1) * volume
	}

	giniCoeff := giniSum / (n * totalVolume)
	return math.Abs(giniCoeff) // 返回0-1之间的值，越大表示��布越不均匀
}

// calculatePriceSkewness 计算价格偏斜度（判断成交量向高价还是低价倾斜）
func (va *VPVRAnalyzer) calculatePriceSkewness(levels []*PriceLevel, totalVolume float64) float64 {
	if totalVolume <= 0 || len(levels) < 3 {
		return 0
	}

	// 计算成交量加权的价格三阶矩
	var weightedPriceSum, weightedPriceSquareSum, weightedPriceCubeSum float64
	
	for _, level := range levels {
		if level.Volume > 0 {
			weight := level.Volume / totalVolume
			weightedPriceSum += level.Price * weight
			weightedPriceSquareSum += level.Price * level.Price * weight
			weightedPriceCubeSum += level.Price * level.Price * level.Price * weight
		}
	}

	mean := weightedPriceSum
	variance := weightedPriceSquareSum - mean*mean
	
	if variance <= 0 {
		return 0
	}

	stdDev := math.Sqrt(variance)
	if stdDev == 0 {
		return 0
	}

	// 计算偏斜度
	thirdMoment := weightedPriceCubeSum - 3*mean*weightedPriceSquareSum + 2*mean*mean*mean
	skewness := thirdMoment / (stdDev * stdDev * stdDev)

	return math.Max(-2, math.Min(2, skewness)) // 限制在-2到2之间
}

// calculateMarketRegimeStrength 计算市场状态强度评分
func (va *VPVRAnalyzer) calculateMarketRegimeStrength(stats *VolumeStats, levels []*PriceLevel) float64 {
	// 基于多个指标综合评分
	strengthFactors := []float64{
		stats.POCStrength / 100.0,                    // POC强度因子 (0-1)
		stats.POCDensity / 30.0,                      // POC密度因子 (假设最高30%)
		(1 - stats.ValueAreaWidthRatio) * 1.5,       // 价值区集中度因子
		va.calculateVolumeConsistency(levels),        // 成交量一致性因子
	}

	// 加权平均
	weights := []float64{0.3, 0.25, 0.25, 0.2}
	var weightedSum float64
	for i, factor := range strengthFactors {
		weightedSum += math.Min(1.0, math.Max(0.0, factor)) * weights[i]
	}

	return math.Min(100.0, weightedSum * 100.0)
}

// calculateVolumeConsistency 计算成交量一致性
func (va *VPVRAnalyzer) calculateVolumeConsistency(levels []*PriceLevel) float64 {
	if len(levels) < 3 {
		return 0
	}

	// 计算成交量的变异系数（标准差/均值）
	var volumeSum, volumeSquareSum float64
	validCount := 0

	for _, level := range levels {
		if level.Volume > 0 {
			volumeSum += level.Volume
			volumeSquareSum += level.Volume * level.Volume
			validCount++
		}
	}

	if validCount < 2 {
		return 0
	}

	mean := volumeSum / float64(validCount)
	if mean == 0 {
		return 0
	}

	variance := (volumeSquareSum / float64(validCount)) - (mean * mean)
	stdDev := math.Sqrt(math.Max(0, variance))

	// 变异系数
	coefficientOfVariation := stdDev / mean

	// 一致性 = 1 - 标准化变异系数，值越大表示成交量越一致
	consistency := 1.0 - math.Min(1.0, coefficientOfVariation/2.0)
	return math.Max(0.0, consistency)
}

// calculateTrendStrength 计算趋势强度评分
func (va *VPVRAnalyzer) calculateTrendStrength(stats *VolumeStats, levels []*PriceLevel) float64 {
	if len(levels) < 5 {
		return 0
	}

	// 计算成交量在价格区间的分布斜率
	priceSkewness := va.calculatePriceSkewness(levels, stats.TotalVolume)
	
	// 计算POC相对于价格中心的位置偏移
	minPrice := levels[0].Price
	maxPrice := levels[len(levels)-1].Price
	priceCenter := (minPrice + maxPrice) / 2
	
	var pocPrice float64
	if stats.MaxLevel != nil {
		pocPrice = stats.MaxLevel.Price
	} else {
		pocPrice = priceCenter
	}
	
	pocOffset := math.Abs(pocPrice - priceCenter) / ((maxPrice - minPrice) / 2)

	// 计算成交量分布的方向性
	directionalBias := math.Abs(priceSkewness)
	
	// 综合趋势强度评分
	trendStrength := (directionalBias*0.5 + pocOffset*0.3 + (1-stats.ValueAreaWidthRatio)*0.2) * 100

	return math.Min(100.0, math.Max(0.0, trendStrength))
}

// calculateConsolidationLevel 计算整理程度评分
func (va *VPVRAnalyzer) calculateConsolidationLevel(stats *VolumeStats, levels []*PriceLevel) float64 {
	// 整理程度与趋势强度相反
	trendStrength := va.calculateTrendStrength(stats, levels)
	
	// 基于价值区宽度占比
	widthFactor := math.Min(1.0, stats.ValueAreaWidthRatio / 0.6) * 50
	
	// 基于POC集中度
	concentrationFactor := math.Min(1.0, stats.POCDensity / 20.0) * 30
	
	// 基于成交量分布均匀性
	dispersion := va.calculateVolumeDispersion(levels, stats.TotalVolume)
	uniformityFactor := (1.0 - dispersion) * 20
	
	// 综合整理程度评分
	consolidationLevel := widthFactor + concentrationFactor + uniformityFactor - (trendStrength * 0.3)
	
	return math.Min(100.0, math.Max(0.0, consolidationLevel))
}

// identifyVolumeNodes 识别并标记HVN/LVN节点 (Task 6)
func (va *VPVRAnalyzer) identifyVolumeNodes(levels []*PriceLevel, stats *VolumeStats) {
	if len(levels) < 3 || stats.TotalVolume <= 0 {
		return
	}

	// 1. 计算成交量统计指标
	volumeStats := va.calculateVolumeStatistics(levels, stats.TotalVolume)
	
	// 2. 为每个价格级别分配成交量排名
	va.rankVolumeNodes(levels)
	
	// 3. 识别HVN（高成交量节点）
	hvnLevels := va.identifyHVNs(levels, volumeStats)
	
	// 4. 识别LVN（低成交量节点）
	lvnLevels := va.identifyLVNs(levels, volumeStats)
	
	// 5. 识别价格空隙（LVN形成的空隙）
	gapCount := va.identifyPriceGaps(levels, volumeStats)
	
	// 6. 计算节点强度评分
	va.calculateNodeStrengths(levels, volumeStats)
	
	// 7. 分类节点类型
	va.classifyNodeTypes(levels)
	
	// 8. 更新统计信息
	va.updateVolumeNodeStats(stats, hvnLevels, lvnLevels, gapCount)
}

// VolumeStatistics 成交量统计数据结构（内部使用）
type VolumeStatistics struct {
	Mean          float64   // 成交量均值
	Median        float64   // 成交量中位数  
	StdDev        float64   // 成交量标准差
	Q1            float64   // 第一四分位数
	Q3            float64   // 第三四分位数
	IQR           float64   // 四分位距
	HVNThreshold  float64   // HVN阈值
	LVNThreshold  float64   // LVN阈值
	SortedVolumes []float64 // 排序后的成交量
}

// calculateVolumeStatistics 计算成交量统计指标
func (va *VPVRAnalyzer) calculateVolumeStatistics(levels []*PriceLevel, totalVolume float64) *VolumeStatistics {
	stats := &VolumeStatistics{}
	
	// 收集所有非零成交量
	var volumes []float64
	for _, level := range levels {
		if level.Volume > 0 {
			volumes = append(volumes, level.Volume)
		}
	}
	
	if len(volumes) == 0 {
		return stats
	}
	
	// 排序
	sortedVolumes := make([]float64, len(volumes))
	copy(sortedVolumes, volumes)
	sort.Float64s(sortedVolumes)
	stats.SortedVolumes = sortedVolumes
	
	// 计算基础统计量
	stats.Mean = va.calculateMean(sortedVolumes)
	stats.Median = va.calculateMedian(sortedVolumes)
	stats.StdDev = va.calculateStdDev(sortedVolumes, stats.Mean)
	
	// 计算四分位数
	stats.Q1 = va.calculateQuantile(sortedVolumes, 0.25)
	stats.Q3 = va.calculateQuantile(sortedVolumes, 0.75)
	stats.IQR = stats.Q3 - stats.Q1
	
	// 计算HVN/LVN阈值
	// 方法1：基于四分位数和IQR（Tukey's法则）
	upperFence := stats.Q3 + 1.5*stats.IQR
	lowerFence := stats.Q1 - 1.5*stats.IQR
	
	// 方法2：基于均值和标准差
	meanPlusStd := stats.Mean + stats.StdDev
	meanMinusStd := stats.Mean - stats.StdDev
	
	// 组合阈值（取更保守的值）
	stats.HVNThreshold = math.Min(upperFence, meanPlusStd*1.2)
	stats.LVNThreshold = math.Max(lowerFence, meanMinusStd*0.5)
	
	// 确保阈值合理
	if stats.LVNThreshold < 0 {
		stats.LVNThreshold = stats.Mean * 0.1 // 至少是均值的10%
	}
	
	return stats
}

// rankVolumeNodes 为价格级别分配成交量排名
func (va *VPVRAnalyzer) rankVolumeNodes(levels []*PriceLevel) {
	// 创建排名数组
	type rankItem struct {
		level *PriceLevel
		index int
	}
	
	var items []rankItem
	for i, level := range levels {
		items = append(items, rankItem{level: level, index: i})
	}
	
	// 按成交量降序排序
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].level.Volume < items[j].level.Volume {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	
	// 分配排名
	for rank, item := range items {
		item.level.VolumeRank = rank + 1
	}
}

// identifyHVNs 识别高成交量节点
func (va *VPVRAnalyzer) identifyHVNs(levels []*PriceLevel, volumeStats *VolumeStatistics) []*PriceLevel {
	var hvnLevels []*PriceLevel
	
	for _, level := range levels {
		// 基础阈值判断
		isAboveThreshold := level.Volume > volumeStats.HVNThreshold
		
		// 局部极大值判断：在邻近区域内是否为局部最大值
		isLocalMax := va.isLocalVolumeMax(level, levels)
		
		// 相对强度判断：相对于整体分布的重要性
		relativeStrength := level.Volume / volumeStats.Mean
		
		// 综合判断HVN条件
		if isAboveThreshold && (isLocalMax || relativeStrength > 2.0) {
			level.IsHVN = true
			level.NodeType = VolumeNodeHVN
			hvnLevels = append(hvnLevels, level)
		}
	}
	
	// 特殊处理：确保POC一定是HVN
	for _, level := range levels {
		if level.IsPOC && !level.IsHVN {
			level.IsHVN = true
			level.NodeType = VolumeNodePOC
			hvnLevels = append(hvnLevels, level)
		}
	}
	
	return hvnLevels
}

// identifyLVNs 识别低成交量节点
func (va *VPVRAnalyzer) identifyLVNs(levels []*PriceLevel, volumeStats *VolumeStatistics) []*PriceLevel {
	var lvnLevels []*PriceLevel
	
	for _, level := range levels {
		// 基础阈值判断
		isBelowThreshold := level.Volume < volumeStats.LVNThreshold
		
		// 局部极小值判断
		isLocalMin := va.isLocalVolumeMin(level, levels)
		
		// 相对弱度判断
		relativeStrength := level.Volume / volumeStats.Mean
		
		// 综合判断LVN条件（排除已被标记为HVN的节点）
		if !level.IsHVN && (isBelowThreshold && (isLocalMin || relativeStrength < 0.3)) {
			level.IsLVN = true
			level.NodeType = VolumeNodeLVN
			lvnLevels = append(lvnLevels, level)
		}
	}
	
	return lvnLevels
}

// isLocalVolumeMax 判断是否为局部成交量极大值
func (va *VPVRAnalyzer) isLocalVolumeMax(target *PriceLevel, levels []*PriceLevel) bool {
	targetIndex := -1
	for i, level := range levels {
		if level == target {
			targetIndex = i
			break
		}
	}
	
	if targetIndex == -1 {
		return false
	}
	
	// 检查周围的价格级别（前后各3个）
	lookback := 3
	for i := targetIndex - lookback; i <= targetIndex + lookback; i++ {
		if i >= 0 && i < len(levels) && i != targetIndex {
			if levels[i].Volume > target.Volume {
				return false
			}
		}
	}
	
	return true
}

// isLocalVolumeMin 判断是否为局部成交量极小值  
func (va *VPVRAnalyzer) isLocalVolumeMin(target *PriceLevel, levels []*PriceLevel) bool {
	targetIndex := -1
	for i, level := range levels {
		if level == target {
			targetIndex = i
			break
		}
	}
	
	if targetIndex == -1 {
		return false
	}
	
	// 检查周围的价格级别（前后各3个）
	lookback := 3
	for i := targetIndex - lookback; i <= targetIndex + lookback; i++ {
		if i >= 0 && i < len(levels) && i != targetIndex {
			if levels[i].Volume < target.Volume {
				return false
			}
		}
	}
	
	return true
}

// identifyPriceGaps 识别价格空隙（连续的LVN区域）
func (va *VPVRAnalyzer) identifyPriceGaps(levels []*PriceLevel, volumeStats *VolumeStatistics) int {
	gapCount := 0
	inGap := false
	gapStart := -1
	
	for i, level := range levels {
		if level.IsLVN {
			if !inGap {
				// 开始一个新的空隙
				inGap = true
				gapStart = i
			}
		} else {
			if inGap {
				// 结束当前空隙
				gapLength := i - gapStart
				if gapLength >= 2 { // 至少2个连续的LVN才算空隙
					gapCount++
					// 将空隙中的LVN标记为Gap类型
					for j := gapStart; j < i; j++ {
						if levels[j].IsLVN {
							levels[j].NodeType = VolumeNodeGap
						}
					}
				}
				inGap = false
			}
		}
	}
	
	// 处理结尾的空隙
	if inGap {
		gapLength := len(levels) - gapStart
		if gapLength >= 2 {
			gapCount++
			for j := gapStart; j < len(levels); j++ {
				if levels[j].IsLVN {
					levels[j].NodeType = VolumeNodeGap
				}
			}
		}
	}
	
	return gapCount
}

// calculateNodeStrengths 计算节点强度评分
func (va *VPVRAnalyzer) calculateNodeStrengths(levels []*PriceLevel, volumeStats *VolumeStatistics) {
	for _, level := range levels {
		if level.IsHVN {
			level.HVNStrength = va.calculateHVNStrength(level, volumeStats)
		}
		if level.IsLVN {
			level.LVNStrength = va.calculateLVNStrength(level, volumeStats)
		}
	}
}

// calculateHVNStrength 计算HVN强度评分
func (va *VPVRAnalyzer) calculateHVNStrength(level *PriceLevel, volumeStats *VolumeStatistics) float64 {
	// 基于多个因素计算HVN强度
	
	// 因子1：相对于均值的倍数
	meanRatio := level.Volume / volumeStats.Mean
	meanFactor := math.Min(1.0, meanRatio/5.0) * 30 // 最高30分
	
	// 因子2：相对于中位数的倍数  
	medianRatio := level.Volume / volumeStats.Median
	medianFactor := math.Min(1.0, medianRatio/3.0) * 25 // 最高25分
	
	// 因子3：在总体分布中的位置（基于排名）
	rankFactor := (1.0 - float64(level.VolumeRank-1)/float64(len(volumeStats.SortedVolumes))) * 25 // 最高25分
	
	// 因子4：是否为POC
	pocBonus := 0.0
	if level.IsPOC {
		pocBonus = 20.0
	}
	
	strength := meanFactor + medianFactor + rankFactor + pocBonus
	return math.Min(100.0, strength)
}

// calculateLVNStrength 计算LVN强度评分
func (va *VPVRAnalyzer) calculateLVNStrength(level *PriceLevel, volumeStats *VolumeStatistics) float64 {
	// LVN强度表示其作为"弱点"的强度
	
	// 因子1：相对于均值的比例（越小强度越高）
	meanRatio := level.Volume / volumeStats.Mean
	meanFactor := (1.0 - math.Min(1.0, meanRatio)) * 40 // 最高40分
	
	// 因子2：在总体分布中的位置（排名越靠后强度越高）
	rankFactor := (float64(level.VolumeRank-1) / float64(len(volumeStats.SortedVolumes))) * 30 // 最高30分
	
	// 因子3：是否形成价格空隙
	gapBonus := 0.0
	if level.NodeType == VolumeNodeGap {
		gapBonus = 30.0
	}
	
	strength := meanFactor + rankFactor + gapBonus
	return math.Min(100.0, strength)
}

// classifyNodeTypes 分类节点类型
func (va *VPVRAnalyzer) classifyNodeTypes(levels []*PriceLevel) {
	// 寻找HVN聚集区域
	va.identifyHVNClusters(levels)
	
	// 为普通节点分配类型
	for _, level := range levels {
		if !level.IsHVN && !level.IsLVN {
			level.NodeType = VolumeNodeNormal
		}
	}
}

// identifyHVNClusters 识别HVN聚集区域
func (va *VPVRAnalyzer) identifyHVNClusters(levels []*PriceLevel) {
	// 寻找连续的HVN区域
	var clusterStart = -1
	
	for i, level := range levels {
		if level.IsHVN {
			if clusterStart == -1 {
				clusterStart = i
			}
		} else {
			if clusterStart != -1 {
				clusterLength := i - clusterStart
				if clusterLength >= 3 { // 至少3个连续的HVN形成聚集区
					// 将聚集区中的HVN标记为Cluster类型
					for j := clusterStart; j < i; j++ {
						if levels[j].IsHVN {
							levels[j].NodeType = VolumeNodeCluster
						}
					}
				}
				clusterStart = -1
			}
		}
	}
	
	// 处理结尾的聚集区
	if clusterStart != -1 {
		clusterLength := len(levels) - clusterStart
		if clusterLength >= 3 {
			for j := clusterStart; j < len(levels); j++ {
				if levels[j].IsHVN {
					levels[j].NodeType = VolumeNodeCluster
				}
			}
		}
	}
}

// updateVolumeNodeStats 更新HVN/LVN统计信息
func (va *VPVRAnalyzer) updateVolumeNodeStats(stats *VolumeStats, hvnLevels, lvnLevels []*PriceLevel, gapCount int) {
	// 基础计数
	stats.HVNCount = len(hvnLevels)
	stats.LVNCount = len(lvnLevels)
	stats.PriceGapCount = gapCount
	
	// 计算HVN/LVN总成交量
	stats.HVNTotalVolume = 0
	for _, level := range hvnLevels {
		stats.HVNTotalVolume += level.Volume
	}
	
	stats.LVNTotalVolume = 0
	for _, level := range lvnLevels {
		stats.LVNTotalVolume += level.Volume
	}
	
	// 计算成交量占比
	if stats.TotalVolume > 0 {
		stats.HVNVolumePercent = (stats.HVNTotalVolume / stats.TotalVolume) * 100
		stats.LVNVolumePercent = (stats.LVNTotalVolume / stats.TotalVolume) * 100
	}
	
	// 计算成交量集中度评分
	stats.VolumeConcentration = va.calculateVolumeConcentrationScore(stats)
}

// calculateVolumeConcentrationScore 计算成交量集中度评分
func (va *VPVRAnalyzer) calculateVolumeConcentrationScore(stats *VolumeStats) float64 {
	// 基于HVN占比和数量的集中度评分
	
	// 因子1：HVN成交量占比（越高越集中）
	volumeFactor := stats.HVNVolumePercent // 0-100
	
	// 因子2：HVN数量相对评分（数量适中时集中度最高）
	var countFactor float64
	if stats.HVNCount <= 3 {
		countFactor = 100.0 // 少量HVN，高度集中
	} else if stats.HVNCount <= 6 {
		countFactor = 80.0 // 适量HVN，较为集中
	} else if stats.HVNCount <= 10 {
		countFactor = 60.0 // 较多HVN，中等集中
	} else {
		countFactor = 40.0 // 过多HVN，集中度较低
	}
	
	// 因子3：LVN占比修正（LVN占比高会降低集中度）
	lvnPenalty := stats.LVNVolumePercent * 0.5 // LVN占比的负面影响
	
	// 综合计算
	concentration := (volumeFactor*0.6 + countFactor*0.4) - lvnPenalty
	
	return math.Min(100.0, math.Max(0.0, concentration))
}

// 辅助统计函数
func (va *VPVRAnalyzer) calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func (va *VPVRAnalyzer) calculateMedian(sortedValues []float64) float64 {
	n := len(sortedValues)
	if n == 0 {
		return 0
	}
	if n%2 == 0 {
		return (sortedValues[n/2-1] + sortedValues[n/2]) / 2
	}
	return sortedValues[n/2]
}

func (va *VPVRAnalyzer) calculateStdDev(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		diff := v - mean
		sum += diff * diff
	}
	return math.Sqrt(sum / float64(len(values)))
}

func (va *VPVRAnalyzer) calculateQuantile(sortedValues []float64, p float64) float64 {
	n := len(sortedValues)
	if n == 0 {
		return 0
	}
	index := p * float64(n-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	
	if lower == upper {
		return sortedValues[lower]
	}
	
	// 线性插值
	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

// calculateStabilizedTickSize 计算稳定的TickSize (Task 7)
func (va *VPVRAnalyzer) calculateStabilizedTickSize(minPrice, maxPrice float64, klines []Kline) float64 {
	priceRange := maxPrice - minPrice
	if priceRange <= 0 {
		return va.config.TickSize
	}

	// 方法1：基于价格范围的自适应TickSize
	adaptiveTickSize := va.calculateAdaptiveTickSize(priceRange, len(klines))
	
	// 方法2：基于平均真实范围(ATR)的TickSize
	atrBasedTickSize := va.calculateATRBasedTickSize(klines, priceRange)
	
	// 方法3：基于历史一致性的TickSize
	consistentTickSize := va.calculateConsistentTickSize(priceRange)
	
	// 选择最稳定的TickSize（优先考虑一致性）
	finalTickSize := va.selectOptimalTickSize([]float64{
		adaptiveTickSize,
		atrBasedTickSize, 
		consistentTickSize,
		va.config.TickSize, // 默认配置作为基准
	}, priceRange)
	
	return finalTickSize
}

// calculateAdaptiveTickSize 基于价格范围的自适应TickSize
func (va *VPVRAnalyzer) calculateAdaptiveTickSize(priceRange float64, klineCount int) float64 {
	// 目标级别数基于K线数量动态调整
	var targetLevels float64
	switch {
	case klineCount < 50:
		targetLevels = 30  // 少量K线，减少级别数
	case klineCount < 100:
		targetLevels = 50
	case klineCount < 200:
		targetLevels = 80
	default:
		targetLevels = 120 // 大量K线，增加级别数
	}
	
	return priceRange / targetLevels
}

// calculateATRBasedTickSize 基于ATR的TickSize
func (va *VPVRAnalyzer) calculateATRBasedTickSize(klines []Kline, priceRange float64) float64 {
	if len(klines) < 14 {
		return priceRange / 50 // 默认50个级别
	}
	
	// 计算ATR
	atr := va.calculateATR(klines, 14)
	if atr <= 0 {
		return priceRange / 50
	}
	
	// TickSize = ATR的1/3到1/2，确保能捕捉到价格的微小变化
	atrBasedTick := atr * 0.4
	
	// 限制在合理范围内
	minTick := priceRange / 300  // 最多300个级别
	maxTick := priceRange / 20   // 最少20个级别
	
	return math.Min(maxTick, math.Max(minTick, atrBasedTick))
}

// calculateConsistentTickSize 基于一致性的TickSize
func (va *VPVRAnalyzer) calculateConsistentTickSize(priceRange float64) float64 {
	// 使用基于价格大小的标准化TickSize，保持一致性
	
	// 根据价格范围确定合适的精度等级
	var consistentTick float64
	switch {
	case priceRange < 0.1:
		consistentTick = 0.0001  // 0.01分精度
	case priceRange < 1.0:
		consistentTick = 0.001   // 0.1分精度
	case priceRange < 10.0:
		consistentTick = 0.01    // 1分精度
	case priceRange < 100.0:
		consistentTick = 0.1     // 1角精度
	case priceRange < 1000.0:
		consistentTick = 1.0     // 1元精度
	case priceRange < 10000.0:
		consistentTick = 10.0    // 10元精度
	default:
		consistentTick = 100.0   // 100元精度
	}
	
	return consistentTick
}

// selectOptimalTickSize 选择最优的TickSize
func (va *VPVRAnalyzer) selectOptimalTickSize(candidates []float64, priceRange float64) float64 {
	// 为每个候选TickSize计算评分
	bestScore := -1.0
	bestTickSize := va.config.TickSize
	
	for _, tickSize := range candidates {
		if tickSize <= 0 {
			continue
		}
		
		score := va.evaluateTickSizeStability(tickSize, priceRange)
		if score > bestScore {
			bestScore = score
			bestTickSize = tickSize
		}
	}
	
	return bestTickSize
}

// evaluateTickSizeStability 评估TickSize的稳定性
func (va *VPVRAnalyzer) evaluateTickSizeStability(tickSize, priceRange float64) float64 {
	if tickSize <= 0 || priceRange <= 0 {
		return 0
	}
	
	levelCount := priceRange / tickSize
	
	// 评分因子1：级别数量合理性 (30-150个级别为最佳)
	var levelScore float64
	if levelCount >= 30 && levelCount <= 150 {
		levelScore = 100.0
	} else if levelCount >= 20 && levelCount <= 200 {
		levelScore = 80.0
	} else if levelCount >= 10 && levelCount <= 300 {
		levelScore = 60.0
	} else {
		levelScore = 20.0
	}
	
	// 评分因子2：精度合理性（不要太细也不要太粗）
	var precisionScore float64
	if tickSize >= priceRange/200 && tickSize <= priceRange/30 {
		precisionScore = 100.0
	} else if tickSize >= priceRange/300 && tickSize <= priceRange/20 {
		precisionScore = 80.0
	} else {
		precisionScore = 40.0
	}
	
	// 评分因子3：与默认配置的一致性
	defaultRatio := tickSize / va.config.TickSize
	var consistencyScore float64
	if defaultRatio >= 0.5 && defaultRatio <= 2.0 {
		consistencyScore = 100.0
	} else if defaultRatio >= 0.2 && defaultRatio <= 5.0 {
		consistencyScore = 70.0
	} else {
		consistencyScore = 30.0
	}
	
	// 综合评分
	return levelScore*0.4 + precisionScore*0.35 + consistencyScore*0.25
}

// calculateATR 计算平均真实范围
func (va *VPVRAnalyzer) calculateATR(klines []Kline, period int) float64 {
	if len(klines) < period+1 {
		return 0
	}
	
	var trSum float64
	startIndex := len(klines) - period
	
	for i := startIndex; i < len(klines); i++ {
		if i <= 0 {
			continue
		}
		
		current := klines[i]
		previous := klines[i-1]
		
		// 计算True Range
		tr1 := current.High - current.Low
		tr2 := math.Abs(current.High - previous.Close)
		tr3 := math.Abs(current.Low - previous.Close)
		
		tr := math.Max(tr1, math.Max(tr2, tr3))
		trSum += tr
	}
	
	return trSum / float64(period)
}

// distributePriceVolumeStabilized 使用稳定TickSize分配成交量 (Task 7)
func (va *VPVRAnalyzer) distributePriceVolumeStabilized(kline Kline, levelMap map[float64]*PriceLevel, minPrice, tickSize float64) {
	// 数据有效性检查
	if kline.Volume <= 0 {
		return
	}
	
	// 计算K线的价格范围
	priceRange := kline.High - kline.Low
	
	// 修复1: 处理价格范围为零的情况（无价格变化K线）
	if priceRange <= 0 {
		// 将所有成交量分配给收盘价位置
		va.addVolumeToSingleLevelStabilized(kline.Close, kline.Volume, levelMap, minPrice, tickSize)
		return
	}

	// 修复2: 成交量分配策略 - 填充所有价格级别
	// 获取K线跨越的所有价格级别
	lowLevelPrice := va.roundToTickStabilized(kline.Low, minPrice, tickSize)
	highLevelPrice := va.roundToTickStabilized(kline.High, minPrice, tickSize)
	
	// 计算价格级别数量
	levelCount := int((highLevelPrice-lowLevelPrice)/tickSize) + 1
	if levelCount <= 0 {
		levelCount = 1
	}
	
	// 优化3: 优先使用真实买卖量数据，降级到算法估算
	var buyVolume, sellVolume float64
	if va.useRealBuySellData(kline) {
		// 使用交易所真实买卖量数据
		buyVolume, sellVolume = va.extractRealBuySellVolume(kline)
	} else {
		// 降级：使用改进的算法估算
		buyRatio := va.calculateEnhancedBuySellRatio(kline, priceRange)
		buyVolume = kline.Volume * buyRatio
		sellVolume = kline.Volume * (1 - buyRatio)
	}

	// 修复4: 插值填充所有价格级别，消除空洞效应
	if levelCount == 1 {
		// 单一级别，直接分配
		va.addVolumeToSingleLevelStabilized(kline.Close, kline.Volume, levelMap, minPrice, tickSize)
	} else {
		// 多级别插值分配
		va.distributeVolumeAcrossLevelsStabilized(kline, buyVolume, sellVolume, lowLevelPrice, highLevelPrice, levelCount, levelMap, minPrice, tickSize)
	}
}

// addVolumeToSingleLevelStabilized 使用稳定TickSize添加单一价格级别成交量
func (va *VPVRAnalyzer) addVolumeToSingleLevelStabilized(price, volume float64, levelMap map[float64]*PriceLevel, minPrice, tickSize float64) {
	levelPrice := va.roundToTickStabilized(price, minPrice, tickSize)
	
	level, exists := levelMap[levelPrice]
	if !exists {
		level = &PriceLevel{
			Price: levelPrice,
		}
		levelMap[levelPrice] = level
	}
	
	level.Volume += volume
	// 对于无价格变化的K线，买卖比例设为中性
	level.BuyVolume += volume * 0.5
	level.SellVolume += volume * 0.5
	level.Transactions++
}

// distributeVolumeAcrossLevelsStabilized 使用稳定TickSize在多个价格级别间分配成交量
func (va *VPVRAnalyzer) distributeVolumeAcrossLevelsStabilized(kline Kline, buyVolume, sellVolume float64, lowLevelPrice, highLevelPrice float64, levelCount int, levelMap map[float64]*PriceLevel, minPrice, tickSize float64) {
	totalVolume := kline.Volume
	
	// 计算关键价位的权重
	ohlcWeights := va.calculateOHLCWeightsStabilized(kline, lowLevelPrice, highLevelPrice, tickSize)
	
	// 为每个价格级别分配成交量
	for i := 0; i < levelCount; i++ {
		levelPrice := lowLevelPrice + float64(i)*tickSize
		
		// 计算该级别的成交量权重
		volumeWeight := va.calculateLevelVolumeWeightStabilized(levelPrice, kline, ohlcWeights, levelCount)
		
		// 分配成交量
		level, exists := levelMap[levelPrice]
		if !exists {
			level = &PriceLevel{
				Price: levelPrice,
			}
			levelMap[levelPrice] = level
		}
		
		volumeToAdd := totalVolume * volumeWeight
		level.Volume += volumeToAdd
		level.BuyVolume += buyVolume * volumeWeight
		level.SellVolume += sellVolume * volumeWeight
		level.Transactions++
	}
}

// roundToTickStabilized 使用稳定TickSize映射价格到级别
func (va *VPVRAnalyzer) roundToTickStabilized(price, minPrice, tickSize float64) float64 {
	offset := price - minPrice
	// 使用floor函数确保确定性价格级别分配
	levelIndex := int(math.Floor(offset / tickSize))
	
	// 防止负数索引
	if levelIndex < 0 {
		levelIndex = 0
	}
	
	return minPrice + float64(levelIndex)*tickSize
}

// calculateOHLCWeightsStabilized 使用稳定TickSize计算OHLC权重
func (va *VPVRAnalyzer) calculateOHLCWeightsStabilized(kline Kline, lowLevelPrice, highLevelPrice, tickSize float64) map[float64]float64 {
	weights := make(map[float64]float64)
	
	// 将OHLC价格映射到价格级别
	openLevel := va.roundToTickStabilized(kline.Open, lowLevelPrice, tickSize)
	highLevel := va.roundToTickStabilized(kline.High, lowLevelPrice, tickSize)
	lowLevel := va.roundToTickStabilized(kline.Low, lowLevelPrice, tickSize)
	closeLevel := va.roundToTickStabilized(kline.Close, lowLevelPrice, tickSize)
	
	// 设置关键价位权重
	weights[openLevel] = 0.25   // 开盘价权重25%
	weights[highLevel] = 0.20   // 最高价权重20%
	weights[lowLevel] = 0.20    // 最低价权重20%
	weights[closeLevel] = 0.35  // 收盘价权重35%
	
	return weights
}

// calculateLevelVolumeWeightStabilized 使用稳定TickSize计算级别权重
func (va *VPVRAnalyzer) calculateLevelVolumeWeightStabilized(levelPrice float64, kline Kline, ohlcWeights map[float64]float64, totalLevels int) float64 {
	// 基础权重：均匀分布
	baseWeight := 1.0 / float64(totalLevels)
	
	// OHLC权重加成
	ohlcWeight, exists := ohlcWeights[levelPrice]
	if !exists {
		ohlcWeight = 0.0
	}
	
	// 距离权重：越接近价格中心，权重越高
	priceCenter := (kline.High + kline.Low) / 2
	distanceFromCenter := math.Abs(levelPrice - priceCenter)
	maxDistance := math.Abs(kline.High - kline.Low) / 2
	
	var distanceWeight float64
	if maxDistance > 0 {
		distanceWeight = 0.1 * (1 - distanceFromCenter/maxDistance)
	}
	
	// 综合权重
	finalWeight := baseWeight + ohlcWeight + distanceWeight
	
	// 确保权重为正数
	return math.Max(finalWeight, 0.001)
}
// ===== Task 8: 自适应阈值机制 =====

// AdaptiveThresholds 自适应阈值参数
type AdaptiveThresholds struct {
	// POC相关阈值
	POCDistanceThreshold   float64 `json:"poc_distance_threshold"`   // POC距离阈值（动态调整）
	POCStrengthMultiplier  float64 `json:"poc_strength_multiplier"`  // POC强度倍数
	
	// 价值区域相关阈值
	VABreakoutThreshold    float64 `json:"va_breakout_threshold"`    // 价值区域突破阈值
	VAReturnSensitivity    float64 `json:"va_return_sensitivity"`    // 价值区域回归敏感度
	
	// 成交量相关阈值
	HighVolumeMultiplier   float64 `json:"high_volume_multiplier"`   // 高成交量倍数阈值
	LowVolumeMultiplier    float64 `json:"low_volume_multiplier"`    // 低成交量倍数阈值
	
	// 买卖不平衡阈值
	ImbalanceThreshold     float64 `json:"imbalance_threshold"`      // 买卖不平衡阈值
	BuySellRatioThreshold  float64 `json:"buy_sell_ratio_threshold"` // 买卖比例阈值
	
	// 市场环境修正因子
	VolatilityAdjustment   float64 `json:"volatility_adjustment"`   // 波动性调整因子
	TrendStrengthFactor    float64 `json:"trend_strength_factor"`   // 趋势强度因子
	ConcentrationFactor    float64 `json:"concentration_factor"`    // 集中度因子
}

// calculateAdaptiveThresholds 计算基于市场条件的自适应阈值
func (va *VPVRAnalyzer) calculateAdaptiveThresholds(profile *VolumeProfile, currentPrice float64) *AdaptiveThresholds {
	// 基础阈值（静态配置作为基准）
	baseThresholds := &AdaptiveThresholds{
		POCDistanceThreshold:   0.01,   // 1%
		POCStrengthMultiplier:  2.0,    
		VABreakoutThreshold:    0.005,  // 0.5%
		VAReturnSensitivity:    1.0,    
		HighVolumeMultiplier:   2.0,    // 2倍
		LowVolumeMultiplier:    0.3,    // 0.3倍
		ImbalanceThreshold:     1.5,    // 1.5倍
		BuySellRatioThreshold:  1.2,    // 1.2倍
		VolatilityAdjustment:   1.0,    
		TrendStrengthFactor:    1.0,    
		ConcentrationFactor:    1.0,    
	}
	
	// 基于市场条件动态调整
	va.adjustForMarketVolatility(baseThresholds, profile)
	va.adjustForVolumeConcentration(baseThresholds, profile)
	va.adjustForTrendStrength(baseThresholds, profile)
	va.adjustForPriceRange(baseThresholds, profile, currentPrice)
	va.adjustForHVNLVNContext(baseThresholds, profile)
	
	return baseThresholds
}

// adjustForMarketVolatility 基于市场波动性调整阈值
func (va *VPVRAnalyzer) adjustForMarketVolatility(thresholds *AdaptiveThresholds, profile *VolumeProfile) {
	if profile.ValueArea == nil || len(profile.Levels) < 2 {
		return
	}
	
	// 计算价格范围波动性
	priceRange := profile.Levels[len(profile.Levels)-1].Price - profile.Levels[0].Price
	avgPrice := profile.Stats.AvgPrice
	volatility := priceRange / avgPrice // 相对波动性
	
	// 波动性修正因子：高波动时放宽阈值，低波动时收紧阈值
	var volatilityFactor float64
	switch {
	case volatility > 0.15: // 高波动市场（>15%）
		volatilityFactor = 1.5 // 放宽50%
	case volatility > 0.08: // 中等波动市场（8-15%）
		volatilityFactor = 1.2 // 放宽20%
	case volatility < 0.03: // 低波动市场（<3%）
		volatilityFactor = 0.8 // 收紧20%
	default:
		volatilityFactor = 1.0 // 标准
	}
	
	thresholds.VolatilityAdjustment = volatilityFactor
	
	// 应用波动性调整到各个阈值
	thresholds.POCDistanceThreshold *= volatilityFactor
	thresholds.VABreakoutThreshold *= volatilityFactor
	
	// 成交量相关阈值反向调整：高波动时更严格
	thresholds.HighVolumeMultiplier *= (2.0 - volatilityFactor + 1.0) / 2.0
	thresholds.LowVolumeMultiplier *= (2.0 - volatilityFactor + 1.0) / 2.0
}

// adjustForVolumeConcentration 基于成交量集中度调整阈值
func (va *VPVRAnalyzer) adjustForVolumeConcentration(thresholds *AdaptiveThresholds, profile *VolumeProfile) {
	concentration := profile.ValueArea.Concentration
	
	// 集中度修正因子
	var concentrationFactor float64
	switch {
	case concentration > 2.0: // 高度集中
		concentrationFactor = 0.8 // 收紧阈值，信号更可靠
	case concentration > 1.5: // 中等集中
		concentrationFactor = 0.9
	case concentration < 0.8: // 分散市场
		concentrationFactor = 1.3 // 放宽阈值，避免过多噪声
	default:
		concentrationFactor = 1.0
	}
	
	thresholds.ConcentrationFactor = concentrationFactor
	
	// POC相关阈值：集中度高时可以更敏���
	thresholds.POCDistanceThreshold *= concentrationFactor
	thresholds.POCStrengthMultiplier *= (2.0 - concentrationFactor + 1.0) / 2.0
}

// adjustForTrendStrength 基于趋势强度调整阈值
func (va *VPVRAnalyzer) adjustForTrendStrength(thresholds *AdaptiveThresholds, profile *VolumeProfile) {
	trendStrength := profile.Stats.TrendStrength / 100.0 // 标准化到0-1
	
	// 趋势强度修正因子
	var trendFactor float64
	switch {
	case trendStrength > 0.7: // 强趋势
		trendFactor = 0.9 // 突破更容易，阈值收紧
	case trendStrength < 0.3: // 弱趋势/震荡
		trendFactor = 1.2 // 突破更难，阈值放宽
	default:
		trendFactor = 1.0
	}
	
	thresholds.TrendStrengthFactor = trendFactor
	
	// 价值区域突破阈值：强趋势时更容易突破
	thresholds.VABreakoutThreshold *= trendFactor
	
	// 买卖不平衡阈值：强趋势时降低阈值，更容易触发
	thresholds.ImbalanceThreshold *= (2.0 - trendFactor + 1.0) / 2.0
}

// adjustForPriceRange 基于价格范围调整阈值
func (va *VPVRAnalyzer) adjustForPriceRange(thresholds *AdaptiveThresholds, profile *VolumeProfile, currentPrice float64) {
	if len(profile.Levels) < 2 {
		return
	}
	
	// 价格范围分析
	totalRange := profile.Levels[len(profile.Levels)-1].Price - profile.Levels[0].Price
	valueAreaRange := profile.VAH - profile.VAL
	
	// 价值区域占总范围的比例
	var rangeRatio float64
	if totalRange > 0 {
		rangeRatio = valueAreaRange / totalRange
	}
	
	// 基于价值区域相对大小调整
	switch {
	case rangeRatio < 0.3: // 价值区域很窄，市场高度一致
		thresholds.VAReturnSensitivity = 0.8 // 提高回归敏感度
		thresholds.POCStrengthMultiplier *= 1.2 // 增强POC重要性
	case rangeRatio > 0.7: // 价值区域很宽，市场分歧较大
		thresholds.VAReturnSensitivity = 1.3 // 降低回归敏感度
		thresholds.POCStrengthMultiplier *= 0.8 // 减少POC重要性
	}
}

// adjustForHVNLVNContext 基于HVN/LVN节点上下文调整阈值
func (va *VPVRAnalyzer) adjustForHVNLVNContext(thresholds *AdaptiveThresholds, profile *VolumeProfile) {
	hvnCount := profile.Stats.HVNCount
	lvnCount := profile.Stats.LVNCount
	totalLevels := len(profile.Levels)
	
	if totalLevels == 0 {
		return
	}
	
	// HVN/LVN密度
	hvnDensity := float64(hvnCount) / float64(totalLevels)
	lvnDensity := float64(lvnCount) / float64(totalLevels)
	
	// 节点密度修正
	switch {
	case hvnDensity > 0.15: // HVN节点密度高（>15%）
		// 成交量活跃，降低成交量信号阈值
		thresholds.HighVolumeMultiplier *= 0.9
		thresholds.LowVolumeMultiplier *= 1.1
	case lvnDensity > 0.2: // LVN节点密度高（>20%）
		// 成交量稀疏，提高成交量信号阈值  
		thresholds.HighVolumeMultiplier *= 1.2
		thresholds.LowVolumeMultiplier *= 0.8
	}
}

// generatePOCSignalAdaptive 生成POC相关信号（自适应版本）
func (va *VPVRAnalyzer) generatePOCSignalAdaptive(profile *VolumeProfile, currentPrice float64, timestamp int64, thresholds *AdaptiveThresholds) *VPVRSignal {
	poc := profile.POC
	if poc == nil {
		return nil
	}

	distance := math.Abs(currentPrice - poc.Price) / poc.Price
	
	// 使用自适应阈值
	if distance < thresholds.POCDistanceThreshold {
		strength := (poc.Volume / profile.Stats.TotalVolume) * 100
		confidence := math.Min(strength*thresholds.POCStrengthMultiplier, 100)

		var action SignalAction
		description := "价格接近POC"

		// 使用自适应买卖比例判断
		if poc.BuyVolume > poc.SellVolume*thresholds.BuySellRatioThreshold {
			action = ActionBuy
			description += "，买盘占优，建议买入"
		} else if poc.SellVolume > poc.BuyVolume*thresholds.BuySellRatioThreshold {
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

// generateValueAreaSignalAdaptive 生成价值区域相关信号（自适应版本）
func (va *VPVRAnalyzer) generateValueAreaSignalAdaptive(profile *VolumeProfile, currentPrice float64, timestamp int64, thresholds *AdaptiveThresholds) *VPVRSignal {
	if profile.ValueArea == nil {
		return nil
	}

	vah := profile.VAH
	val := profile.VAL
	
	var signal *VPVRSignal

	// 使用自适应突破阈值
	breakoutThreshold := 1.0 + thresholds.VABreakoutThreshold
	
	// 突破价值区域上沿
	if currentPrice > vah*breakoutThreshold {
		signal = &VPVRSignal{
			Type:         VPVRSignalVABreakout,
			Level:        vah,
			CurrentPrice: currentPrice,
			Strength:     (currentPrice - vah) / vah * 100,
			Description:  "突破价值区域上沿，可能继续上涨",
			Action:       ActionBuy,
			Confidence:   70 + profile.ValueArea.Concentration*10*thresholds.ConcentrationFactor,
			Timestamp:    timestamp,
		}
	} else if currentPrice < val*(2.0-breakoutThreshold) { // 对称计算下沿
		signal = &VPVRSignal{
			Type:         VPVRSignalVABreakout,
			Level:        val,
			CurrentPrice: currentPrice,
			Strength:     (val - currentPrice) / val * 100,
			Description:  "跌破价值���域下沿，可能继续下跌",
			Action:       ActionSell,
			Confidence:   70 + profile.ValueArea.Concentration*10*thresholds.ConcentrationFactor,
			Timestamp:    timestamp,
		}
	} else if currentPrice > val && currentPrice < vah {
		// 回归价值区域（使用���适应敏感度）
		centerPrice := (vah + val) / 2
		distanceFromCenter := math.Abs(currentPrice - centerPrice) / centerPrice
		
		signal = &VPVRSignal{
			Type:         VPVRSignalVAReturn,
			Level:        centerPrice,
			CurrentPrice: currentPrice,
			Strength:     (1 - distanceFromCenter) * 100 * thresholds.VAReturnSensitivity,
			Description:  "价格在价值区域内，趋向均值回归",
			Action:       ActionHold,
			Confidence:   math.Min(60 - distanceFromCenter*100/thresholds.VAReturnSensitivity, 100),
			Timestamp:    timestamp,
		}
	}

	return signal
}

// generateVolumeSignalAdaptive 生成基于成交量级别的信号（自适应版本）
func (va *VPVRAnalyzer) generateVolumeSignalAdaptive(profile *VolumeProfile, currentPrice float64, timestamp int64, thresholds *AdaptiveThresholds) *VPVRSignal {
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

	// 检查是否为高成交量级别（使用自适应阈值）
	avgVolume := profile.Stats.TotalVolume / float64(len(profile.Levels))
	volumeRatio := nearestLevel.Volume / avgVolume

	if volumeRatio > thresholds.HighVolumeMultiplier {
		var action SignalAction
		description := "当前价位成交量异常活跃"

		// 使用自适应买卖比例判断
		buySellThreshold := thresholds.BuySellRatioThreshold + 0.1 // 成交量信号稍微严格一些
		if nearestLevel.BuyVolume > nearestLevel.SellVolume*buySellThreshold {
			action = ActionBuy
			description += "，买盘占优"
		} else if nearestLevel.SellVolume > nearestLevel.BuyVolume*buySellThreshold {
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
			Confidence:   math.Min(volumeRatio*25*thresholds.ConcentrationFactor, 100),
			Timestamp:    timestamp,
		}
	} else if volumeRatio < thresholds.LowVolumeMultiplier {
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

// generateImbalanceSignalAdaptive 生成买卖不平衡信号（自适应版本）
func (va *VPVRAnalyzer) generateImbalanceSignalAdaptive(profile *VolumeProfile, currentPrice float64, timestamp int64, thresholds *AdaptiveThresholds) *VPVRSignal {
	if profile.Stats.BuySellRatio == 0 {
		return nil
	}

	// 使用自适应不平衡阈值
	var signal *VPVRSignal

	if profile.Stats.BuySellRatio > thresholds.ImbalanceThreshold {
		// 买盘占优
		strength := (profile.Stats.BuySellRatio - 1) * 100
		signal = &VPVRSignal{
			Type:         VPVRSignalImbalance,
			Level:        profile.Stats.AvgPrice,
			CurrentPrice: currentPrice,
			Strength:     math.Min(strength, 100),
			Description:  "整体买盘明显强于卖盘，多头氛围浓厚",
			Action:       ActionBuy,
			Confidence:   math.Min(strength*1.5*thresholds.TrendStrengthFactor, 100),
			Timestamp:    timestamp,
		}
	} else if profile.Stats.BuySellRatio < (1.0 / thresholds.ImbalanceThreshold) {
		// 卖盘占优
		strength := (1/profile.Stats.BuySellRatio - 1) * 100
		signal = &VPVRSignal{
			Type:         VPVRSignalImbalance,
			Level:        profile.Stats.AvgPrice,
			CurrentPrice: currentPrice,
			Strength:     math.Min(strength, 100),
			Description:  "整体卖盘明显强于买盘，空头氛围浓厚",
			Action:       ActionSell,
			Confidence:   math.Min(strength*1.5*thresholds.TrendStrengthFactor, 100),
			Timestamp:    timestamp,
		}
	}

	return signal
}

// generateHVNLVNSignal 生成HVN/LVN节点信号（新增）
func (va *VPVRAnalyzer) generateHVNLVNSignal(profile *VolumeProfile, currentPrice float64, timestamp int64, thresholds *AdaptiveThresholds) *VPVRSignal {
	// 找到当前价格附近最强的HVN节点
	var nearestHVN *PriceLevel
	minDistance := math.Inf(1)

	for _, level := range profile.Levels {
		if level.IsHVN && level.HVNStrength > 70 { // 只考虑强HVN节点
			distance := math.Abs(level.Price - currentPrice) / level.Price
			if distance < minDistance {
				minDistance = distance
				nearestHVN = level
			}
		}
	}

	// HVN信号：当价格接近强HVN节点时
	if nearestHVN != nil && minDistance < thresholds.POCDistanceThreshold*1.5 { // HVN阈值稍微放宽
		var action SignalAction
		description := "价格接近高成交量节点(HVN)"

		// 基于HVN的买卖量分布判断方向
		if nearestHVN.BuyVolume > nearestHVN.SellVolume*thresholds.BuySellRatioThreshold {
			action = ActionBuy
			description += "，买盘占优，预期支撑"
		} else if nearestHVN.SellVolume > nearestHVN.BuyVolume*thresholds.BuySellRatioThreshold {
			action = ActionSell
			description += "，卖盘占优，预期阻力"
		} else {
			action = ActionHold
			description += "，买卖平衡，观望为主"
		}

		return &VPVRSignal{
			Type:         VPVRSignalHighVolume, // 复用高成交量信号类型
			Level:        nearestHVN.Price,
			CurrentPrice: currentPrice,
			Strength:     nearestHVN.HVNStrength,
			Description:  description,
			Action:       action,
			Confidence:   nearestHVN.HVNStrength * thresholds.ConcentrationFactor,
			Timestamp:    timestamp,
		}
	}

	// LVN空隙信号：当价格进入LVN空隙区域时
	for _, level := range profile.Levels {
		if level.NodeType == VolumeNodeGap && level.IsLVN {
			distance := math.Abs(level.Price - currentPrice) / level.Price
			if distance < thresholds.POCDistanceThreshold*2 { // LVN空隙阈值更宽
				return &VPVRSignal{
					Type:         VPVRSignalLowVolume,
					Level:        level.Price,
					CurrentPrice: currentPrice,
					Strength:     level.LVNStrength,
					Description:  "价格进入成交量空隙区，缺乏支撑阻力，容易快速通过",
					Action:       ActionHold, // 空隙区域建议观望
					Confidence:   50 + level.LVNStrength*0.3, // 基于LVN强度调整置信度
					Timestamp:    timestamp,
				}
			}
		}
	}

	return nil
}

// calculateVPVRContext 计算VPVR上下文指标
// 🔥 P0级修复：为VPVR添加Context计算，解决StrengthZ和VolRatio为null的问题
func (va *VPVRAnalyzer) calculateVPVRContext(levels []*PriceLevel, stats *VolumeStats, poc *PriceLevel, vah, val float64) *ContextMetrics {
	if stats == nil || poc == nil || len(levels) == 0 {
		return &ContextMetrics{
			StrengthZ:  0.0,
			WidthATR:   0.0,
			VolRatio:   1.0,
			IsFresh:    true,
			TimeScore:  1.0,
			RankPct:    0.5,
		}
	}

	// 计算强度Z-score：基于POC的体量密度
	pocDensity := va.calculatePOCDensity(poc, stats.TotalVolume)
	// 假设平均POC密度为10%，标准差为5%
	avgPOCDensity := 0.10
	stdPOCDensity := 0.05
	strengthZ := (pocDensity - avgPOCDensity) / stdPOCDensity

	// 计算宽度ATR：价值区域宽度相对于价格的比例
	valueAreaWidth := vah - val
	avgPrice := (vah + val) / 2
	var widthATR float64
	if avgPrice > 0 {
		widthATR = valueAreaWidth / (avgPrice * 0.01) // 相对于1%价格变动的倍数
	}

	// 计算体量比率：当前VPVR的总成交量相对于预期的倍数
	// 使用成交量集中度作为体量比率的代理指标
	volRatio := 1.0
	if stats.VolumeConcentration > 0 {
		// 成交量集中度越高，说明体量越集中，比率越高
		volRatio = stats.VolumeConcentration * 2.0 // 调节系数
		if volRatio > 5.0 {
			volRatio = 5.0 // 限制最大值
		}
	}

	// 新鲜度：VPVR通常基于历史数据，设为false
	isFresh := false

	// 时间评分：VPVR基于历史时间窗口，给予中等评分
	timeScore := 0.75

	// 排名百分位：基于POC强度进行排名
	rankPct := 0.5
	if strengthZ > 1.0 {
		rankPct = 0.8
	} else if strengthZ > 0 {
		rankPct = 0.6
	} else if strengthZ < -1.0 {
		rankPct = 0.2
	} else {
		rankPct = 0.4
	}

	context := &ContextMetrics{
		StrengthZ:  strengthZ,
		WidthATR:   widthATR,
		VolRatio:   volRatio,
		IsFresh:    isFresh,
		TimeScore:  timeScore,
		RankPct:    rankPct,
	}

	log.Printf("🔍 [VPVR Context] 计算完成: StrengthZ=%.4f, WidthATR=%.4f, VolRatio=%.4f, POCDensity=%.4f",
		strengthZ, widthATR, volRatio, pocDensity)

	return context
}

// filterKeyPriceLevels 筛选关键价格级别（混合策略）
// 🔥 Token优化：POC/VAH/VAL + Top6 volume + Near price 3个 = 最多12个bins
func (va *VPVRAnalyzer) filterKeyPriceLevels(levels []*PriceLevel, poc *PriceLevel, vah, val, currentPrice float64) []*PriceLevel {
	if len(levels) == 0 {
		return levels
	}

	// 使用map去重，key是价格（使用一定精度避免浮点数问题）
	selectedLevels := make(map[string]*PriceLevel)

	// 辅助函数：生成价格key
	priceKey := func(price float64) string {
		return fmt.Sprintf("%.8f", price)
	}

	// 1. 添加POC对应的level
	if poc != nil {
		selectedLevels[priceKey(poc.Price)] = poc
		log.Printf("🎯 [VPVR筛选] POC: %.2f (Volume: %.2f)", poc.Price, poc.Volume)
	}

	// 2. 添加VAH对应的level（找到最接近VAH价格的level）
	vahLevel := va.findClosestLevel(levels, vah)
	if vahLevel != nil {
		selectedLevels[priceKey(vahLevel.Price)] = vahLevel
		log.Printf("🎯 [VPVR筛选] VAH: %.2f (Volume: %.2f)", vahLevel.Price, vahLevel.Volume)
	}

	// 3. 添加VAL对应的level（找到最接近VAL价格的level）
	valLevel := va.findClosestLevel(levels, val)
	if valLevel != nil {
		selectedLevels[priceKey(valLevel.Price)] = valLevel
		log.Printf("🎯 [VPVR筛选] VAL: %.2f (Volume: %.2f)", valLevel.Price, valLevel.Volume)
	}

	// 4. 添加Top 6 volume levels
	volumeSorted := make([]*PriceLevel, len(levels))
	copy(volumeSorted, levels)
	sort.Slice(volumeSorted, func(i, j int) bool {
		return volumeSorted[i].Volume > volumeSorted[j].Volume
	})

	topVolumeCount := 6
	if len(volumeSorted) < topVolumeCount {
		topVolumeCount = len(volumeSorted)
	}

	log.Printf("🎯 [VPVR筛选] Top %d Volume levels:", topVolumeCount)
	for i := 0; i < topVolumeCount; i++ {
		level := volumeSorted[i]
		selectedLevels[priceKey(level.Price)] = level
		log.Printf("   #%d: %.2f (Volume: %.2f)", i+1, level.Price, level.Volume)
	}

	// 5. 添加距离当前价格最近的3个levels
	distanceSorted := make([]*PriceLevel, len(levels))
	copy(distanceSorted, levels)
	sort.Slice(distanceSorted, func(i, j int) bool {
		distI := math.Abs(distanceSorted[i].Price - currentPrice)
		distJ := math.Abs(distanceSorted[j].Price - currentPrice)
		return distI < distJ
	})

	nearPriceCount := 3
	if len(distanceSorted) < nearPriceCount {
		nearPriceCount = len(distanceSorted)
	}

	log.Printf("🎯 [VPVR筛选] Near Price (current: %.2f) 最近%d个:", currentPrice, nearPriceCount)
	for i := 0; i < nearPriceCount; i++ {
		level := distanceSorted[i]
		selectedLevels[priceKey(level.Price)] = level
		distance := math.Abs(level.Price - currentPrice)
		log.Printf("   #%d: %.2f (Distance: %.2f, Volume: %.2f)", i+1, level.Price, distance, level.Volume)
	}

	// 转换map为slice
	result := make([]*PriceLevel, 0, len(selectedLevels))
	for _, level := range selectedLevels {
		result = append(result, level)
	}

	// 按价格排序（保持原有顺序）
	sort.Slice(result, func(i, j int) bool {
		return result[i].Price < result[j].Price
	})

	log.Printf("📊 [VPVR筛选] 原始%d个bins，筛选后%d个bins（去重后）", len(levels), len(result))

	return result
}

// findClosestLevel 找到最接近指定价格的level
func (va *VPVRAnalyzer) findClosestLevel(levels []*PriceLevel, targetPrice float64) *PriceLevel {
	if len(levels) == 0 {
		return nil
	}

	var closest *PriceLevel
	minDistance := math.MaxFloat64

	for _, level := range levels {
		distance := math.Abs(level.Price - targetPrice)
		if distance < minDistance {
			minDistance = distance
			closest = level
		}
	}

	return closest
}