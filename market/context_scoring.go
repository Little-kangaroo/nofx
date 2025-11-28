package market

import (
	"math"
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
	AvgVolume20 float64 `json:"avg_volume_20"` // 20期平均成交量
	CurrentPrice float64 `json:"current_price"` // 当前价格
}

// ContextCalculator 上下文计算器
type ContextCalculator struct {
	klines []Kline
	market *MarketContext
}

// NewContextCalculator 创建上下文计算器
func NewContextCalculator(klines []Kline) *ContextCalculator {
	calc := &ContextCalculator{
		klines: klines,
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

	// 计算平均成交量
	avgVolume20 := cc.calculateAvgVolume(20)

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
		AvgVolume20:   avgVolume20,
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

// calculateAvgVolume 计算平均成交量
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

// CalculateVolumeRatio 计算成交量比率
func (cc *ContextCalculator) CalculateVolumeRatio(volume float64) float64 {
	if cc.market.AvgVolume20 == 0 {
		return 1.0
	}
	return volume / cc.market.AvgVolume20
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