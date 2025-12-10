package microstructure

import (
	"log"
	"math"
	"sync"
	"time"
)

// ===== V-12.2 Z-Score动态阈值系统 =====

// DynamicThresholdConfig 动态阈值配置
type DynamicThresholdConfig struct {
	// 基础阈值参数
	BaseZScoreThreshold   float64 `json:"base_zscore_threshold"`   // 基础Z-Score阈值: 1.5
	MaxZScoreThreshold    float64 `json:"max_zscore_threshold"`    // 最大Z-Score阈值: 3.0
	MinZScoreThreshold    float64 `json:"min_zscore_threshold"`    // 最小Z-Score阈值: 0.8
	
	// 自适应调整参数
	AdaptationSpeed       float64 `json:"adaptation_speed"`        // 适应速度: 0.1 (10%)
	VolatilityWeight      float64 `json:"volatility_weight"`       // 波动性权重: 0.3
	TrendWeight           float64 `json:"trend_weight"`            // 趋势权重: 0.2
	VolumeWeight          float64 `json:"volume_weight"`           // 成交量权重: 0.3
	TimeDecay             float64 `json:"time_decay"`              // 时间衰减: 0.95
	
	// 市场状态分类阈值
	HighVolatilityThreshold  float64 `json:"high_volatility_threshold"`  // 高波动阈值: 2.0
	TrendingMarketThreshold  float64 `json:"trending_market_threshold"`  // 趋势市场阈值: 1.8
	LowLiquidityThreshold    float64 `json:"low_liquidity_threshold"`    // 低流动性阈值: -1.5
}

// MarketRegime 市场状态分类
type MarketRegime string

const (
	RegimeNormal        MarketRegime = "normal"         // 正常市场
	RegimeHighVolatility MarketRegime = "high_volatility" // 高波动市场
	RegimeTrending      MarketRegime = "trending"       // 趋势市场
	RegimeLowLiquidity  MarketRegime = "low_liquidity"  // 低流动性市场
	RegimeManipulation  MarketRegime = "manipulation"   // 疑似操纵市场
)

// SymbolThresholds 单个交易对的动态阈值
type SymbolThresholds struct {
	Symbol                string                 `json:"symbol"`
	LastUpdate           time.Time              `json:"last_update"`
	CurrentRegime        MarketRegime           `json:"current_regime"`
	
	// 动态阈值（按指标分类）
	CVDThresholds        map[string]float64     `json:"cvd_thresholds"`      // CVD相关阈值
	VolumeThresholds     map[string]float64     `json:"volume_thresholds"`   // 成交量相关阈值  
	OrderbookThresholds  map[string]float64     `json:"orderbook_thresholds"` // 盘口相关阈值
	PriceThresholds      map[string]float64     `json:"price_thresholds"`    // 价格相关阈值
	
	// 历史统计信息（用于动态调整）
	HistoricalStats      map[string]*RollingStats `json:"historical_stats"`
	
	// 市场状态评估
	VolatilityLevel      float64                `json:"volatility_level"`
	TrendStrength        float64                `json:"trend_strength"`
	LiquidityLevel       float64                `json:"liquidity_level"`
	ManipulationRisk     float64                `json:"manipulation_risk"`
}

// DynamicThresholdSystem V-12.2动态阈值系统
type DynamicThresholdSystem struct {
	mu                  sync.RWMutex
	config              DynamicThresholdConfig
	symbolThresholds    map[string]*SymbolThresholds  // symbol -> thresholds
	updateTicker        *time.Ticker
	isRunning          bool
	stopChan           chan bool
	
	// 全局统计管理器引用
	globalStatsManager  *GlobalStatsManager
}

// NewDynamicThresholdSystem 创建动态阈值系统
func NewDynamicThresholdSystem() *DynamicThresholdSystem {
	return &DynamicThresholdSystem{
		config: DynamicThresholdConfig{
			BaseZScoreThreshold:     1.5,
			MaxZScoreThreshold:      3.0,
			MinZScoreThreshold:      0.8,
			AdaptationSpeed:         0.1,
			VolatilityWeight:        0.3,
			TrendWeight:             0.2,
			VolumeWeight:            0.3,
			TimeDecay:               0.95,
			HighVolatilityThreshold: 2.0,
			TrendingMarketThreshold: 1.8,
			LowLiquidityThreshold:   -1.5,
		},
		symbolThresholds:    make(map[string]*SymbolThresholds),
		stopChan:           make(chan bool),
		globalStatsManager: GetGlobalStatsManager(),
	}
}

// Start 启动动态阈值系统
func (dts *DynamicThresholdSystem) Start() {
	dts.mu.Lock()
	defer dts.mu.Unlock()
	
	if dts.isRunning {
		return
	}
	
	dts.updateTicker = time.NewTicker(10 * time.Second) // 每10秒更新阈值
	dts.isRunning = true
	
	go dts.updateLoop()
	log.Printf("🚀 V-12.2动态阈值系统启动完成")
}

// Stop 停止动态阈值系统
func (dts *DynamicThresholdSystem) Stop() {
	dts.mu.Lock()
	defer dts.mu.Unlock()
	
	if !dts.isRunning {
		return
	}
	
	close(dts.stopChan)
	if dts.updateTicker != nil {
		dts.updateTicker.Stop()
	}
	dts.isRunning = false
	log.Printf("⛔ V-12.2动态阈值系统已停止")
}

// updateLoop 阈值更新主循环
func (dts *DynamicThresholdSystem) updateLoop() {
	for {
		select {
		case <-dts.updateTicker.C:
			dts.updateAllThresholds()
		case <-dts.stopChan:
			return
		}
	}
}

// updateAllThresholds 更新所有交易对的动态阈值
func (dts *DynamicThresholdSystem) updateAllThresholds() {
	// 获取订单流管理器
	ofm := GetGlobalOrderFlowManager()
	if ofm == nil {
		return
	}
	
	// 获取所有市场快照
	snapshots := ofm.GetAllMarketSnapshots()
	
	for symbol, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		
		dts.updateSymbolThresholds(symbol, snapshot)
	}
}

// updateSymbolThresholds 更新单个交易对的阈值
func (dts *DynamicThresholdSystem) updateSymbolThresholds(symbol string, snapshot *MarketSnapshot) {
	dts.mu.Lock()
	defer dts.mu.Unlock()
	
	// 获取或创建该交易对的阈值结构
	thresholds, exists := dts.symbolThresholds[symbol]
	if !exists {
		thresholds = dts.createSymbolThresholds(symbol)
		dts.symbolThresholds[symbol] = thresholds
	}
	
	// 获取Z-Score数据
	zScores := dts.globalStatsManager.GetSymbolZScores(symbol, snapshot)
	if len(zScores) == 0 {
		return // 数据不足，跳过
	}
	
	// 评估市场状态
	dts.assessMarketRegime(thresholds, snapshot, zScores)
	
	// 根据市场状态调整阈值
	dts.adjustThresholds(thresholds, snapshot, zScores)
	
	// 更新时间戳
	thresholds.LastUpdate = time.Now()
}

// createSymbolThresholds 创建新的交易对阈值结构
func (dts *DynamicThresholdSystem) createSymbolThresholds(symbol string) *SymbolThresholds {
	return &SymbolThresholds{
		Symbol:              symbol,
		LastUpdate:         time.Now(),
		CurrentRegime:      RegimeNormal,
		CVDThresholds: map[string]float64{
			"momentum_ignition": dts.config.BaseZScoreThreshold,     // 动量点燃
			"spot_rush":        dts.config.BaseZScoreThreshold,     // 现货抢跑
			"cvd_divergence":   dts.config.BaseZScoreThreshold,     // CVD背离
		},
		VolumeThresholds: map[string]float64{
			"abnormal_volume":  dts.config.BaseZScoreThreshold,     // 异常成交量
			"volume_spike":     dts.config.BaseZScoreThreshold * 1.2, // 成交量激增
			"volume_dryup":     dts.config.BaseZScoreThreshold,     // 成交量枯竭
		},
		OrderbookThresholds: map[string]float64{
			"spoofing_risk":    dts.config.BaseZScoreThreshold,     // 虚假挂单风险
			"liquidity_crisis": dts.config.BaseZScoreThreshold,     // 流动性危机
			"imbalance_extreme": dts.config.BaseZScoreThreshold,    // 极端失衡
		},
		PriceThresholds: map[string]float64{
			"price_momentum":   dts.config.BaseZScoreThreshold,     // 价格动量
			"price_reversal":   dts.config.BaseZScoreThreshold,     // 价格反转
			"volatility_spike": dts.config.BaseZScoreThreshold,     // 波动率激增
		},
		HistoricalStats:    make(map[string]*RollingStats),
		VolatilityLevel:    1.0,
		TrendStrength:      0.0,
		LiquidityLevel:     1.0,
		ManipulationRisk:   0.0,
	}
}

// assessMarketRegime 评估市场状态
func (dts *DynamicThresholdSystem) assessMarketRegime(thresholds *SymbolThresholds, snapshot *MarketSnapshot, zScores map[string]float64) {
	// 计算波动性水平
	volatilityScore := dts.calculateVolatilityScore(zScores)
	thresholds.VolatilityLevel = volatilityScore
	
	// 计算趋势强度
	trendScore := dts.calculateTrendScore(snapshot, zScores)
	thresholds.TrendStrength = trendScore
	
	// 计算流动性水平
	liquidityScore := dts.calculateLiquidityScore(snapshot, zScores)
	thresholds.LiquidityLevel = liquidityScore
	
	// 计算操纵风险
	manipulationScore := dts.calculateManipulationScore(snapshot, zScores)
	thresholds.ManipulationRisk = manipulationScore
	
	// 根据得分确定市场状态
	previousRegime := thresholds.CurrentRegime
	
	if manipulationScore > 0.7 {
		thresholds.CurrentRegime = RegimeManipulation
	} else if liquidityScore < dts.config.LowLiquidityThreshold {
		thresholds.CurrentRegime = RegimeLowLiquidity
	} else if volatilityScore > dts.config.HighVolatilityThreshold {
		thresholds.CurrentRegime = RegimeHighVolatility
	} else if math.Abs(trendScore) > dts.config.TrendingMarketThreshold {
		thresholds.CurrentRegime = RegimeTrending
	} else {
		thresholds.CurrentRegime = RegimeNormal
	}
	
	// 记录状态变化
	if previousRegime != thresholds.CurrentRegime {
		log.Printf("📊 [%s] 市场状态变化: %s -> %s (波动性:%.2f, 趋势:%.2f, 流动性:%.2f, 操纵风险:%.2f)", 
			thresholds.Symbol, previousRegime, thresholds.CurrentRegime,
			volatilityScore, trendScore, liquidityScore, manipulationScore)
	}
}

// calculateVolatilityScore 计算波动性得分
func (dts *DynamicThresholdSystem) calculateVolatilityScore(zScores map[string]float64) float64 {
	volatilityScore := 0.0
	count := 0
	
	// 检查价格相关的Z-Score
	if priceZScore, exists := zScores["price_change_1m"]; exists {
		volatilityScore += math.Abs(priceZScore)
		count++
	}
	
	// 检查成交量相关的Z-Score
	if volumeZScore, exists := zScores["volume_1m"]; exists {
		volatilityScore += math.Abs(volumeZScore) * 0.8 // 成交量权重略低
		count++
	}
	
	// 检查CVD相关的Z-Score
	spotCVD := zScores["spot_cvd_1m"]
	futuresCVD := zScores["futures_cvd_1m"]
	if spotCVD != 0 || futuresCVD != 0 {
		maxCVD := math.Max(math.Abs(spotCVD), math.Abs(futuresCVD))
		volatilityScore += maxCVD * 0.9
		count++
	}
	
	if count > 0 {
		return volatilityScore / float64(count)
	}
	return 1.0 // 默认正常波动性
}

// calculateTrendScore 计算趋势强度得分
func (dts *DynamicThresholdSystem) calculateTrendScore(snapshot *MarketSnapshot, zScores map[string]float64) float64 {
	if snapshot.CVDDelta5m == nil {
		return 0.0
	}
	
	trendScore := 0.0
	
	// 价格趋势贡献 (40%)
	priceChange := snapshot.CVDDelta5m.PriceDeltaPct
	if math.Abs(priceChange) > 0.5 {
		trendScore += math.Copysign(math.Min(math.Abs(priceChange)/2.0, 1.0), priceChange) * 0.4
	}
	
	// CVD趋势贡献 (35%)
	spotCVD := zScores["spot_cvd_1m"]
	futuresCVD := zScores["futures_cvd_1m"]
	avgCVD := (spotCVD + futuresCVD) / 2
	if math.Abs(avgCVD) > 1.0 {
		trendScore += math.Copysign(math.Min(math.Abs(avgCVD)/3.0, 1.0), avgCVD) * 0.35
	}
	
	// 成交量确认贡献 (25%)
	volumeZScore := zScores["volume_1m"]
	if volumeZScore > 1.0 {
		trendScore += math.Copysign(math.Min(volumeZScore/2.0, 1.0), trendScore) * 0.25
	}
	
	return math.Max(-2.0, math.Min(2.0, trendScore)) // 限制在[-2, 2]范围
}

// calculateLiquidityScore 计算流动性得分
func (dts *DynamicThresholdSystem) calculateLiquidityScore(snapshot *MarketSnapshot, zScores map[string]float64) float64 {
	if snapshot.OrderBookData == nil {
		return 0.0 // 无盘口数据，假设流动性正常
	}
	
	liquidityScore := 0.0
	
	// 流动性评分直接贡献 (50%)
	liquidityZScore := zScores["liquidity_score"]
	liquidityScore += liquidityZScore * 0.5
	
	// 价差影响 (30%)
	// 注：这里需要额外计算价差Z-Score，暂时使用流动性评分作为代理
	liquidityScore += liquidityZScore * 0.3
	
	// 盘口深度影响 (20%)
	liquidityScore += liquidityZScore * 0.2
	
	return liquidityScore
}

// calculateManipulationScore 计算操纵风险得分
func (dts *DynamicThresholdSystem) calculateManipulationScore(snapshot *MarketSnapshot, zScores map[string]float64) float64 {
	if snapshot.OrderBookData == nil {
		return 0.0
	}
	
	manipulationScore := 0.0
	
	// 虚假挂单风险贡献 (40%)
	spoofingZScore := zScores["spoofing_risk"]
	manipulationScore += math.Min(spoofingZScore/2.0, 1.0) * 0.4
	
	// 失衡异常贡献 (30%)
	imbalanceZScore := math.Abs(zScores["imbalance_ratio"])
	manipulationScore += math.Min(imbalanceZScore/2.5, 1.0) * 0.3
	
	// 流动性枯竭贡献 (30%)
	liquidityWeakness := math.Max(0, -zScores["liquidity_score"])
	manipulationScore += math.Min(liquidityWeakness/2.0, 1.0) * 0.3
	
	return math.Min(1.0, manipulationScore)
}

// adjustThresholds 根据市场状态调整阈值
func (dts *DynamicThresholdSystem) adjustThresholds(thresholds *SymbolThresholds, snapshot *MarketSnapshot, zScores map[string]float64) {
	// 根据市场状态确定调整因子
	adjustmentFactors := dts.getAdjustmentFactors(thresholds.CurrentRegime)
	
	// 调整CVD阈值
	for key, currentThreshold := range thresholds.CVDThresholds {
		factor := adjustmentFactors["cvd"]
		newThreshold := dts.adaptiveAdjust(currentThreshold, factor, key, zScores)
		thresholds.CVDThresholds[key] = newThreshold
	}
	
	// 调整成交量阈值
	for key, currentThreshold := range thresholds.VolumeThresholds {
		factor := adjustmentFactors["volume"]
		newThreshold := dts.adaptiveAdjust(currentThreshold, factor, key, zScores)
		thresholds.VolumeThresholds[key] = newThreshold
	}
	
	// 调整盘口阈值
	for key, currentThreshold := range thresholds.OrderbookThresholds {
		factor := adjustmentFactors["orderbook"]
		newThreshold := dts.adaptiveAdjust(currentThreshold, factor, key, zScores)
		thresholds.OrderbookThresholds[key] = newThreshold
	}
	
	// 调整价格阈值
	for key, currentThreshold := range thresholds.PriceThresholds {
		factor := adjustmentFactors["price"]
		newThreshold := dts.adaptiveAdjust(currentThreshold, factor, key, zScores)
		thresholds.PriceThresholds[key] = newThreshold
	}
}

// getAdjustmentFactors 获取基于市场状态的调整因子
func (dts *DynamicThresholdSystem) getAdjustmentFactors(regime MarketRegime) map[string]float64 {
	switch regime {
	case RegimeHighVolatility:
		// 高波动市场：提高阈值以减少误报
		return map[string]float64{
			"cvd":       1.3,
			"volume":    1.2,
			"orderbook": 1.1,
			"price":     1.4,
		}
	case RegimeTrending:
		// 趋势市场：CVD和价格阈值略微提高
		return map[string]float64{
			"cvd":       1.1,
			"volume":    1.0,
			"orderbook": 1.0,
			"price":     1.2,
		}
	case RegimeLowLiquidity:
		// 低流动性市场：降低盘口相关阈值
		return map[string]float64{
			"cvd":       1.0,
			"volume":    1.0,
			"orderbook": 0.8,
			"price":     1.0,
		}
	case RegimeManipulation:
		// 疑似操纵市场：降低所有阈值以提高敏感性
		return map[string]float64{
			"cvd":       0.7,
			"volume":    0.8,
			"orderbook": 0.6,
			"price":     0.8,
		}
	default: // RegimeNormal
		// 正常市场：使用基础阈值
		return map[string]float64{
			"cvd":       1.0,
			"volume":    1.0,
			"orderbook": 1.0,
			"price":     1.0,
		}
	}
}

// adaptiveAdjust 自适应调整单个阈值
func (dts *DynamicThresholdSystem) adaptiveAdjust(currentThreshold, regimeFactor float64, metricKey string, zScores map[string]float64) float64 {
	// 基于状态的调整
	targetThreshold := dts.config.BaseZScoreThreshold * regimeFactor
	
	// 渐进式调整（避免阈值剧烈变化）
	adjustedThreshold := currentThreshold + (targetThreshold-currentThreshold)*dts.config.AdaptationSpeed
	
	// 限制阈值范围
	adjustedThreshold = math.Max(dts.config.MinZScoreThreshold, 
		math.Min(dts.config.MaxZScoreThreshold, adjustedThreshold))
	
	return adjustedThreshold
}

// GetThresholds 获取指定交易对的动态阈值
func (dts *DynamicThresholdSystem) GetThresholds(symbol string) *SymbolThresholds {
	dts.mu.RLock()
	defer dts.mu.RUnlock()
	
	if thresholds, exists := dts.symbolThresholds[symbol]; exists {
		// 返回深拷贝以避免并发问题
		return dts.copySymbolThresholds(thresholds)
	}
	
	return nil
}

// copySymbolThresholds 创建SymbolThresholds的深拷贝
func (dts *DynamicThresholdSystem) copySymbolThresholds(original *SymbolThresholds) *SymbolThresholds {
	copy := &SymbolThresholds{
		Symbol:               original.Symbol,
		LastUpdate:          original.LastUpdate,
		CurrentRegime:       original.CurrentRegime,
		CVDThresholds:       make(map[string]float64),
		VolumeThresholds:    make(map[string]float64),
		OrderbookThresholds: make(map[string]float64),
		PriceThresholds:     make(map[string]float64),
		VolatilityLevel:     original.VolatilityLevel,
		TrendStrength:       original.TrendStrength,
		LiquidityLevel:      original.LiquidityLevel,
		ManipulationRisk:    original.ManipulationRisk,
	}
	
	// 深拷贝阈值映射
	for k, v := range original.CVDThresholds {
		copy.CVDThresholds[k] = v
	}
	for k, v := range original.VolumeThresholds {
		copy.VolumeThresholds[k] = v
	}
	for k, v := range original.OrderbookThresholds {
		copy.OrderbookThresholds[k] = v
	}
	for k, v := range original.PriceThresholds {
		copy.PriceThresholds[k] = v
	}
	
	return copy
}

// GetSystemStats 获取动态阈值系统统计信息
func (dts *DynamicThresholdSystem) GetSystemStats() map[string]interface{} {
	dts.mu.RLock()
	defer dts.mu.RUnlock()
	
	regimeCounts := make(map[MarketRegime]int)
	for _, thresholds := range dts.symbolThresholds {
		regimeCounts[thresholds.CurrentRegime]++
	}
	
	return map[string]interface{}{
		"is_running":         dts.isRunning,
		"symbols_tracked":   len(dts.symbolThresholds),
		"regime_distribution": regimeCounts,
		"update_frequency":  "10s",
		"adaptation_speed":  dts.config.AdaptationSpeed,
	}
}

// ===== 全局动态阈值系统实例 =====

var globalDynamicThresholdSystem *DynamicThresholdSystem
var dynamicThresholdOnce sync.Once

// GetGlobalDynamicThresholdSystem 获取全局动态阈值系统实例
func GetGlobalDynamicThresholdSystem() *DynamicThresholdSystem {
	dynamicThresholdOnce.Do(func() {
		globalDynamicThresholdSystem = NewDynamicThresholdSystem()
		log.Printf("✨ V-12.2动态阈值系统全局实例已创建")
	})
	return globalDynamicThresholdSystem
}