package market

import (
	"math"
	"sort"
	"time"
)

// ContextMetrics 通用上下文评分结构
type ContextMetrics struct {
	StrengthZ  float64 `json:"strength_z"`  // 强度标准分 (>1.5 为强)
	WidthATR   float64 `json:"width_atr"`   // 宽度是 ATR 的倍数
	VolRatio   float64 `json:"vol_ratio"`   // 成交量是均值的倍数
	IsFresh    bool    `json:"is_fresh"`    // 是否新鲜
	TimeScore  float64 `json:"time_score"`  // 时间衰减评分 (0-1)
	RankPct    float64 `json:"rank_pct"`    // 在同类数据中的排名百分位 (0-1)
}

// MarketContext 市场环境上下文
type MarketContext struct {
	ATR14       float64 `json:"atr_14"`       // 14期ATR
	ATR7        float64 `json:"atr_7"`        // 7期ATR
	VolatilityZ float64 `json:"volatility_z"` // 波动率标准分
	TrendStrength float64 `json:"trend_strength"` // 趋势强度 (0-100)
	MedianVolume20 float64 `json:"median_volume_20"` // 20期中位数成交量 (鲁棒基线)
	CurrentPrice float64 `json:"current_price"` // 当前价格
}

// ContextCalculator 上下文计算器
type ContextCalculator struct {
	klines []Kline
	market *MarketContext
	useLogVolume bool // 是否使用对数成交量处理 (适用于山寨币)
}

// NewContextCalculator 创建上下文计算器
func NewContextCalculator(klines []Kline) *ContextCalculator {
	calc := &ContextCalculator{
		klines:       klines,
		useLogVolume: false, // 默认不使用对数处理
	}
	calc.market = calc.calculateMarketContext()
	return calc
}

// NewContextCalculatorWithLogVolume 创建带对数成交量处理的上下文计算器 (适用于山寨币)
func NewContextCalculatorWithLogVolume(klines []Kline) *ContextCalculator {
	calc := &ContextCalculator{
		klines:       klines,
		useLogVolume: true, // 启用对数处理
	}
	calc.market = calc.calculateMarketContext()
	return calc
}

// calculateMarketContext 计算市场环境上下文
func (cc *ContextCalculator) calculateMarketContext() *MarketContext {
	if len(cc.klines) < 20 {
		return &MarketContext{}
	}

	// 计算ATR
	atr14 := cc.calculateATR(14)
	atr7 := cc.calculateATR(7)

	// 计算平均成交量 (使用中位数替代算术平均数以抵御异���值)
	avgVolume20 := cc.calculateMedianVolume(20)

	// 计算波动率标准分
	volatilityZ := cc.calculateVolatilityZScore(20)

	// 计算趋势强度 (简化版本)
	trendStrength := cc.calculateTrendStrength()

	currentPrice := cc.klines[len(cc.klines)-1].Close

	return &MarketContext{
		ATR14:         atr14,
		ATR7:          atr7,
		VolatilityZ:   volatilityZ,
		TrendStrength: trendStrength,
		MedianVolume20: avgVolume20,
		CurrentPrice:  currentPrice,
	}
}

// calculateATR 计算ATR
func (cc *ContextCalculator) calculateATR(period int) float64 {
	if len(cc.klines) < period+1 {
		return 0
	}

	var trSum float64
	for i := len(cc.klines) - period; i < len(cc.klines); i++ {
		if i == 0 {
			continue
		}
		
		high := cc.klines[i].High
		low := cc.klines[i].Low
		prevClose := cc.klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		tr := math.Max(tr1, math.Max(tr2, tr3))
		trSum += tr
	}

	return trSum / float64(period)
}

// calculateMedianVolume 计算中位数成交量 (抵御爆仓插针等异常值)
func (cc *ContextCalculator) calculateMedianVolume(period int) float64 {
	if len(cc.klines) < period {
		return 0
	}

	// 收集成交量数据
	volumes := make([]float64, 0, period)
	for i := len(cc.klines) - period; i < len(cc.klines); i++ {
		volume := cc.klines[i].Volume
		
		// 如果启用对数处理 (适用于山寨币)
		if cc.useLogVolume && volume > 0 {
			volume = math.Log(volume)
		}
		
		volumes = append(volumes, volume)
	}

	// 计算中位数
	return calculateMedian(volumes)
}

// calculateAvgVolume 计算平均成交量 (保留原方法用于兼容性)
// 注意：此方法易受极端值影响，建议使用calculateMedianVolume
func (cc *ContextCalculator) calculateAvgVolume(period int) float64 {
	if len(cc.klines) < period {
		return 0
	}

	var volumeSum float64
	for i := len(cc.klines) - period; i < len(cc.klines); i++ {
		volumeSum += cc.klines[i].Volume
	}

	return volumeSum / float64(period)
}

// calculateMedian 计算数组的中位数
func calculateMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// 创建副本以避免修改原始数据
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		// 偶数个元素：取中间两个数的平均值
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	} else {
		// 奇数个元素：取中间元素
		return sorted[n/2]
	}
}

// calculateZScore 计算Z分数 (标准分)
func (cc *ContextCalculator) calculateZScore(value float64, values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// 计算均值
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// 计算标准差
	var variance float64
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	stdDev := math.Sqrt(variance / float64(len(values)))

	if stdDev == 0 {
		return 0
	}

	return (value - mean) / stdDev
}

// calculateVolatilityZScore 计算波动率标准分
func (cc *ContextCalculator) calculateVolatilityZScore(period int) float64 {
	if len(cc.klines) < period*2 {
		return 0
	}

	// 计算最近period期的波动率
	recentVolatilities := make([]float64, 0, period*2)
	
	for i := len(cc.klines) - period*2; i < len(cc.klines)-1; i++ {
		if i < 0 {
			continue
		}
		volatility := math.Abs(cc.klines[i+1].Close-cc.klines[i].Close) / cc.klines[i].Close
		recentVolatilities = append(recentVolatilities, volatility)
	}

	if len(recentVolatilities) < period {
		return 0
	}

	// 当前波动率
	currentVol := math.Abs(cc.klines[len(cc.klines)-1].Close-cc.klines[len(cc.klines)-2].Close) / cc.klines[len(cc.klines)-2].Close

	return cc.calculateZScore(currentVol, recentVolatilities)
}

// calculateTrendStrength 计算趋势强度 (简化版本)
func (cc *ContextCalculator) calculateTrendStrength() float64 {
	if len(cc.klines) < 20 {
		return 50.0 // 默认中性
	}

	// 简单的EMA趋势强度计算
	period := 20
	var emaSum float64
	var count int

	for i := len(cc.klines) - period; i < len(cc.klines); i++ {
		if i < 0 {
			continue
		}
		emaSum += cc.klines[i].Close
		count++
	}

	if count == 0 {
		return 50.0
	}

	ema := emaSum / float64(count)
	currentPrice := cc.klines[len(cc.klines)-1].Close
	
	// 计算价格相对EMA的位置，转换为0-100的强度值
	priceRatio := currentPrice / ema
	strength := (priceRatio - 0.9) / 0.2 * 100 // 0.9-1.1 映射到 0-100
	
	return math.Max(0, math.Min(100, strength))
}

// CalculateWidthATR 计算宽度相对ATR的倍数
func (cc *ContextCalculator) CalculateWidthATR(width float64) float64 {
	if cc.market.ATR14 == 0 {
		return 0
	}
	return width / cc.market.ATR14
}

// CalculateVolumeRatio 计算成交量比率 (使用中位数基线抵御极端值)
func (cc *ContextCalculator) CalculateVolumeRatio(volume float64) float64 {
	// 标准模式：零成交量返回0比率
	if !cc.useLogVolume && volume == 0 {
		return 0
	}
	
	// 如果启用对数处理
	if cc.useLogVolume {
		// 对数模式下无法处理零成交量，返回默认值
		if volume <= 0 {
			return 1.0
		}
		
		// 对数空间中的比率计算
		logCurrentVolume := math.Log(volume)
		logBaselineVolume := cc.market.MedianVolume20 // 已经是对数值
		
		if logBaselineVolume != 0 {
			return logCurrentVolume / logBaselineVolume
		}
		return 1.0
	}
	
	// 基线为零的情况（数据不足）
	if cc.market.MedianVolume20 == 0 {
		return 1.0
	}
	
	// 标准比率计算 (基于中位数基线)
	return volume / cc.market.MedianVolume20
}

// CalculateVolumeRatioWithPeriod 使用指定周期计算成交量比率
func (cc *ContextCalculator) CalculateVolumeRatioWithPeriod(volume float64, period int) float64 {
	baseline := cc.calculateMedianVolume(period)
	if baseline == 0 {
		return 1.0
	}
	
	// 如果启用对数处理
	if cc.useLogVolume && volume > 0 {
		logCurrentVolume := math.Log(volume)
		return logCurrentVolume / baseline // baseline已经是对数值
	}
	
	return volume / baseline
}

// CalculateStrengthZ 计算强度标准分
func (cc *ContextCalculator) CalculateStrengthZ(strength float64, allStrengths []float64) float64 {
	return cc.calculateZScore(strength, allStrengths)
}

// CalculateTimeScore 计算时间评分 (越新鲜评分越高)
func (cc *ContextCalculator) CalculateTimeScore(creationTime int64, maxAge int64) float64 {
	if maxAge <= 0 {
		return 1.0
	}

	currentTime := time.Now().Unix()
	age := currentTime - creationTime
	
	// 时间衰减函数：新鲜度随时间指数衰减
	timeScore := math.Exp(-float64(age) / float64(maxAge))
	return math.Max(0, math.Min(1.0, timeScore))
}

// IsFresh 判断是否新鲜 (根据创建时间和最大存活时间)
func (cc *ContextCalculator) IsFresh(creationTime int64, maxAge int64) bool {
	currentTime := time.Now().Unix()
	age := currentTime - creationTime
	return age <= maxAge/3 // 在生命周期前1/3认为是新鲜的
}

// CalculateRankPercentile 计算排名百分位
func (cc *ContextCalculator) CalculateRankPercentile(value float64, allValues []float64) float64 {
	if len(allValues) == 0 {
		return 0.5 // 默认中位数
	}

	// 计算有多少值小于当前值
	smallerCount := 0
	for _, v := range allValues {
		if v < value {
			smallerCount++
		}
	}

	return float64(smallerCount) / float64(len(allValues))
}

// GetMarketContext 获取市场环境上下文
func (cc *ContextCalculator) GetMarketContext() *MarketContext {
	return cc.market
}

// CalculateVolatilityGrade 基于ATR归一化计算波动率评级 (跨币种统一标准)
func (cc *ContextCalculator) CalculateVolatilityGrade(widthATR float64) VolatilityGrade {
	// 核心评级标准：基于ATR倍数的统一波动率评估
	switch {
	case widthATR > 0.2 && widthATR < 1.5:
		// A级：0.2 < width_atr < 1.5 - 最优ATR标准
		// - 不会过窄被扫损 (>0.2)
		// - 不会过宽盈亏比差 (<1.5，从原需求的2.0降低到1.5更严格)
		return VolGradeA
		
	case widthATR >= 1.5 && widthATR <= 2.0:
		// B级：适中宽度，基本符合ATR标准
		return VolGradeB
		
	case (widthATR >= 0.15 && widthATR <= 0.2) || (widthATR > 2.0 && widthATR <= 3.0):
		// C级：边缘情况，需谨慎
		// - 稍微偏窄 (0.15-0.2) 
		// - 稍微偏宽 (2.0-3.0)
		return VolGradeC
		
	default:
		// D级：不符合ATR标准
		// - width_atr < 0.15: 过窄，极易被扫损
		// - width_atr > 3.0: 过宽，盈亏比极差
		return VolGradeD
	}
}

// CalculateATRNormalizedSlope 计算ATR归一化斜率 (解决不同币种斜率比较问题)
func (cc *ContextCalculator) CalculateATRNormalizedSlope(prices []float64, periods int) float64 {
	if len(prices) < periods+1 || cc.market.ATR14 == 0 {
		return 0
	}
	
	// 计算价格变化
	priceChange := prices[len(prices)-1] - prices[len(prices)-1-periods]
	
	// ATR归一化：Slope = (Price_Current - Price_N_Ago) / ATR / N
	// 结果含义：每根K线涨跌多少ATR倍数
	normalizedSlope := priceChange / cc.market.ATR14 / float64(periods)
	
	return normalizedSlope
}

// CalculateFuzzyTolerance 计算模糊逻辑的宽容度缓冲区
func (cc *ContextCalculator) CalculateFuzzyTolerance(zoneWidth float64) float64 {
	// 使用两种方式计算宽容度，取较小值确保精度
	// 方式1: 0.1 * Zone_Width (区域宽度的10%)
	widthTolerance := 0.1 * zoneWidth
	
	// 方式2: 0.05 * ATR (ATR的5%)
	atrTolerance := 0.05 * cc.market.ATR14
	
	// 选择较小值作为宽容度，避免过度宽松
	tolerance := math.Min(widthTolerance, atrTolerance)
	
	// 设置最小宽容度，确保极小区域也有基本缓冲
	minTolerance := cc.market.ATR14 * 0.02 // 最小2%ATR
	return math.Max(tolerance, minTolerance)
}

// IsFuzzyTouch 模糊逻辑判定是否触及区域
func (cc *ContextCalculator) IsFuzzyTouch(kline Kline, zone *SupplyDemandZone) bool {
	tolerance := cc.CalculateFuzzyTolerance(zone.Width)
	
	switch zone.Type {
	case SupplyZone:
		// 供给区：价格高点进入上边界缓冲区即为触及
		// High >= Zone.Bottom - tolerance
		return kline.High >= zone.LowerBound-tolerance
		
	case DemandZone:
		// 需求区：价格低点进入下边界缓冲区即为触及  
		// Low <= Zone.Top + tolerance
		return kline.Low <= zone.UpperBound+tolerance
		
	default:
		return false
	}
}

// IsFuzzyBreak 模糊逻辑判定是否破坏区域
func (cc *ContextCalculator) IsFuzzyBreak(kline Kline, zone *SupplyDemandZone) bool {
	// 破坏判定必须是实体收盘价完全越过，且越过幅度 > 0.2 * ATR
	atrThreshold := 0.2 * cc.market.ATR14
	
	switch zone.Type {
	case SupplyZone:
		// 供给区破坏：收盘价突破上边界且幅度足够
		if kline.Close > zone.UpperBound {
			breakDepth := kline.Close - zone.UpperBound
			return breakDepth > atrThreshold
		}
		
	case DemandZone:
		// 需求区破坏：收盘价跌破下边界且幅度足够
		if kline.Close < zone.LowerBound {
			breakDepth := zone.LowerBound - kline.Close
			return breakDepth > atrThreshold
		}
	}
	
	return false
}

// CalculatePenetrationDepth 计算穿透深度百分比
func (cc *ContextCalculator) CalculatePenetrationDepth(kline Kline, zone *SupplyDemandZone) float64 {
	switch zone.Type {
	case SupplyZone:
		if kline.High > zone.LowerBound {
			// 计算高点穿透供给区的深度百分比
			penetration := kline.High - zone.LowerBound
			return math.Min(penetration/zone.Width, 1.0) * 100
		}
		
	case DemandZone:
		if kline.Low < zone.UpperBound {
			// 计算低点穿透需求区的深度百分比
			penetration := zone.UpperBound - kline.Low
			return math.Min(penetration/zone.Width, 1.0) * 100
		}
	}
	
	return 0.0
}