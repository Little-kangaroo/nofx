package microstructure

import (
	"log"
	"math"
	"sync"
	"time"
)

// NewCVDCalculator 创建CVD计算器
func NewCVDCalculator(symbol string, windowDuration time.Duration) *CVDCalculator {
	return &CVDCalculator{
		symbol:           symbol,
		spotDeltas:       make([]CVDDelta, 0, 3600), // 预分配1小时的容量（假设每秒1笔交易）
		futuresDeltas:    make([]CVDDelta, 0, 3600),
		windowDuration:   windowDuration,
		lastCleanup:      time.Now(),
		currentSpotCVD:   0,
		currentFuturesCVD: 0,
		last5mSpotCVD:     0,
		last5mFuturesCVD:  0,
		last5mSnapshot:    time.Now(),
		fiveMinuteCache:   make(map[string]*CVDDelta5m),
		priceHistory:      make([]PriceSnapshot, 0, 360), // 6小时价格历史
	}
}

// ProcessTrade 处理交易数据，更新CVD
func (calc *CVDCalculator) ProcessTrade(trade *TradeData) {
	calc.mu.Lock()
	defer calc.mu.Unlock()

	// 🔍 添加CVD处理监控
	log.Printf("💹 CVD处理交易: %s %s 价格:%.4f USD价值:%.2f", 
		calc.symbol, trade.MarketType, trade.Price, trade.Price*trade.Quantity)

	// 计算交易的USD价值增量
	volumeUSD := trade.Price * trade.Quantity
	var deltaUSD float64

	if trade.IsBuyerMaker {
		// 买方是挂单方 = 主动卖单 (Taker是卖方)
		deltaUSD = -volumeUSD
	} else {
		// 卖方是挂单方 = 主动买单 (Taker是买方) 
		deltaUSD = volumeUSD
	}

	// 创建CVD增量记录
	delta := CVDDelta{
		Timestamp: trade.Timestamp,
		DeltaUSD:  deltaUSD,
	}

	// 根据市场类型添加到对应的滑动窗口
	switch trade.MarketType {
	case "spot":
		calc.spotDeltas = append(calc.spotDeltas, delta)
		calc.currentSpotCVD += deltaUSD
	case "futures":
		calc.futuresDeltas = append(calc.futuresDeltas, delta)
		calc.currentFuturesCVD += deltaUSD
	}

	// 定期清理过期数据（每5分钟清理一次）
	if time.Since(calc.lastCleanup) >= 5*time.Minute {
		calc.cleanupExpiredData()
		calc.lastCleanup = time.Now()
	}
}

// cleanupExpiredData 清理过期的CVD数据
func (calc *CVDCalculator) cleanupExpiredData() {
	cutoffTime := time.Now().Add(-calc.windowDuration)
	
	// 清理现货数据
	calc.currentSpotCVD = 0
	validSpotDeltas := make([]CVDDelta, 0, len(calc.spotDeltas))
	for _, delta := range calc.spotDeltas {
		if delta.Timestamp.After(cutoffTime) {
			validSpotDeltas = append(validSpotDeltas, delta)
			calc.currentSpotCVD += delta.DeltaUSD
		}
	}
	calc.spotDeltas = validSpotDeltas

	// 清理合约数据
	calc.currentFuturesCVD = 0
	validFuturesDeltas := make([]CVDDelta, 0, len(calc.futuresDeltas))
	for _, delta := range calc.futuresDeltas {
		if delta.Timestamp.After(cutoffTime) {
			validFuturesDeltas = append(validFuturesDeltas, delta)
			calc.currentFuturesCVD += delta.DeltaUSD
		}
	}
	calc.futuresDeltas = validFuturesDeltas

	log.Printf("🧹 [%s] CVD数据清理完成 - 现货记录:%d, 合约记录:%d", 
		calc.symbol, len(calc.spotDeltas), len(calc.futuresDeltas))
}

// GetCurrentCVD 获取当前CVD数据
func (calc *CVDCalculator) GetCurrentCVD() *CVDData {
	calc.mu.RLock()
	defer calc.mu.RUnlock()

	// 生成CVD信号
	signal := calc.generateSignal()
	divergence := calc.detectDivergence()

	return &CVDData{
		SpotCVD1H:     calc.currentSpotCVD,
		FuturesCVD1H:  calc.currentFuturesCVD,
		CVDDivergence: divergence,
		Signal:        signal,
		LastUpdate:    time.Now(),
		IsStale:       false,
	}
}

// generateSignal 生成CVD信号
func (calc *CVDCalculator) generateSignal() string {
	spotCVD := calc.currentSpotCVD
	futuresCVD := calc.currentFuturesCVD

	// 定义阈值（可配置）
	strongThreshold := 1000000.0 // 100万USD
	moderateThreshold := 100000.0 // 10万USD

	// 现货和合约CVD都很强的情况
	if math.Abs(spotCVD) > strongThreshold && math.Abs(futuresCVD) > strongThreshold {
		if spotCVD > 0 && futuresCVD > 0 {
			return CVDSignalBullishConfirmation
		} else if spotCVD < 0 && futuresCVD < 0 {
			return CVDSignalBearishConfirmation
		}
	}

	// 现货领先情况（现货强，合约弱）
	if math.Abs(spotCVD) > moderateThreshold && math.Abs(futuresCVD) < moderateThreshold {
		return CVDSignalSpotLeading
	}

	// 合约领先情况（合约强，现货弱）
	if math.Abs(futuresCVD) > moderateThreshold && math.Abs(spotCVD) < moderateThreshold {
		return CVDSignalFuturesLeading
	}

	// 背离情况（方向相反）
	if spotCVD > moderateThreshold && futuresCVD < -moderateThreshold {
		return CVDSignalBullishAbsorption // 现货买入，合约卖出 = 吸筹
	} else if spotCVD < -moderateThreshold && futuresCVD > moderateThreshold {
		return CVDSignalBearishDistribution // 现货卖出，合约买入 = 派发
	}

	return CVDSignalNeutral
}

// detectDivergence 检测价格与CVD的背离
// 注意：这里需要价格数据，实际实现中可能需要从外部传入价格变化信息
func (calc *CVDCalculator) detectDivergence() string {
	// 简化实现：基于现货vs合约CVD的背离
	spotCVD := calc.currentSpotCVD
	futuresCVD := calc.currentFuturesCVD

	threshold := 500000.0 // 50万USD阈值

	// 现货净买入，合约净卖出（或相反）
	if spotCVD > threshold && futuresCVD < -threshold {
		return "bullish_spot_futures_divergence"
	} else if spotCVD < -threshold && futuresCVD > threshold {
		return "bearish_spot_futures_divergence"
	}

	return "no_divergence"
}

// GetStatistics 获取CVD统计信息
func (calc *CVDCalculator) GetStatistics() map[string]interface{} {
	calc.mu.RLock()
	defer calc.mu.RUnlock()

	// 计算现货CVD的统计
	spotMean, spotStdDev := calc.calculateStats(calc.spotDeltas)
	futuresMean, futuresStdDev := calc.calculateStats(calc.futuresDeltas)

	return map[string]interface{}{
		"symbol":                calc.symbol,
		"window_duration_hours": calc.windowDuration.Hours(),
		"spot": map[string]interface{}{
			"current_cvd":     calc.currentSpotCVD,
			"mean_delta":      spotMean,
			"std_dev":         spotStdDev,
			"trade_count":     len(calc.spotDeltas),
			"avg_trade_size":  calc.averageTradeSize(calc.spotDeltas),
		},
		"futures": map[string]interface{}{
			"current_cvd":     calc.currentFuturesCVD,
			"mean_delta":      futuresMean,
			"std_dev":         futuresStdDev,
			"trade_count":     len(calc.futuresDeltas),
			"avg_trade_size":  calc.averageTradeSize(calc.futuresDeltas),
		},
		"ratio": map[string]interface{}{
			"spot_futures_ratio": func() float64 {
				if calc.currentFuturesCVD != 0 {
					return calc.currentSpotCVD / calc.currentFuturesCVD
				}
				return 0
			}(),
			"total_cvd": calc.currentSpotCVD + calc.currentFuturesCVD,
		},
	}
}

// calculateStats 计算统计数据
func (calc *CVDCalculator) calculateStats(deltas []CVDDelta) (mean, stdDev float64) {
	if len(deltas) == 0 {
		return 0, 0
	}

	// 计算均值
	var sum float64
	for _, delta := range deltas {
		sum += delta.DeltaUSD
	}
	mean = sum / float64(len(deltas))

	// 计算标准差
	var variance float64
	for _, delta := range deltas {
		diff := delta.DeltaUSD - mean
		variance += diff * diff
	}
	variance /= float64(len(deltas))
	stdDev = math.Sqrt(variance)

	return mean, stdDev
}

// averageTradeSize 计算平均交易规模
func (calc *CVDCalculator) averageTradeSize(deltas []CVDDelta) float64 {
	if len(deltas) == 0 {
		return 0
	}

	var totalSize float64
	for _, delta := range deltas {
		totalSize += math.Abs(delta.DeltaUSD)
	}

	return totalSize / float64(len(deltas))
}

// Reset 重置CVD计算器
func (calc *CVDCalculator) Reset() {
	calc.mu.Lock()
	defer calc.mu.Unlock()

	calc.spotDeltas = calc.spotDeltas[:0] // 保留容量，清空内容
	calc.futuresDeltas = calc.futuresDeltas[:0]
	calc.currentSpotCVD = 0
	calc.currentFuturesCVD = 0
	calc.lastCleanup = time.Now()

	log.Printf("🔄 [%s] CVD计算器已重置", calc.symbol)
}

// ===== CVD管理器 =====

// CVDManager CVD计算管理器（管理多个币种）
type CVDManager struct {
	calculators map[string]*CVDCalculator // key: symbol
	config      *MicrostructureConfig
	mu          sync.RWMutex
}

// NewCVDManager 创建CVD管理器
func NewCVDManager(config *MicrostructureConfig) *CVDManager {
	return &CVDManager{
		calculators: make(map[string]*CVDCalculator),
		config:      config,
	}
}

// GetOrCreateCalculator 获取或创建币种的CVD计算器
func (manager *CVDManager) GetOrCreateCalculator(symbol string) *CVDCalculator {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if calc, exists := manager.calculators[symbol]; exists {
		return calc
	}

	// 创建新的计算器
	calc := NewCVDCalculator(symbol, manager.config.CVDWindowDuration)
	manager.calculators[symbol] = calc
	
	log.Printf("✨ 创建CVD计算器: %s", symbol)
	return calc
}

// ProcessTrade 处理交易数据
func (manager *CVDManager) ProcessTrade(trade *TradeData) {
	calc := manager.GetOrCreateCalculator(trade.Symbol)
	calc.ProcessTrade(trade)
}

// GetCVDData 获取指定币种的CVD数据
func (manager *CVDManager) GetCVDData(symbol string) *CVDData {
	manager.mu.RLock()
	calc, exists := manager.calculators[symbol]
	manager.mu.RUnlock()

	if !exists {
		// 返回空数据（标记为stale）
		return &CVDData{
			SpotCVD1H:     0,
			FuturesCVD1H:  0,
			CVDDivergence: "no_data",
			Signal:        CVDSignalNeutral,
			LastUpdate:    time.Now(),
			IsStale:       true,
		}
	}

	return calc.GetCurrentCVD()
}

// UpdatePrice 更新币种价格（用于5分钟增量计算）
func (manager *CVDManager) UpdatePrice(symbol string, price float64, timestamp time.Time) {
	calc := manager.GetOrCreateCalculator(symbol)
	calc.UpdatePrice(price, timestamp)
}

// GetCVDDelta5m 获取指定币种的5分钟CVD增量数据
func (manager *CVDManager) GetCVDDelta5m(symbol string) *CVDDelta5m {
	manager.mu.RLock()
	calc, exists := manager.calculators[symbol]
	manager.mu.RUnlock()

	if !exists {
		return nil
	}

	return calc.GetCVDDelta5m()
}

// ForceUpdateAllCVDDeltas 强制更新所有币种的5分钟CVD增量数据（用于K线收盘同步）
func (manager *CVDManager) ForceUpdateAllCVDDeltas() {
	manager.mu.RLock()
	symbols := make([]string, 0, len(manager.calculators))
	for symbol := range manager.calculators {
		symbols = append(symbols, symbol)
	}
	manager.mu.RUnlock()
	
	log.Printf("🔧 强制更新所有币种的CVD增量数据，共%d个币种", len(symbols))
	
	for _, symbol := range symbols {
		calc := manager.calculators[symbol]
		if calc != nil {
			calc.ForceUpdate5MinuteDelta()
		}
	}
	
	log.Printf("✅ 所有CVD增量数据更新完成")
}

// GetAllCVDData 获取所有币种的CVD数据
func (manager *CVDManager) GetAllCVDData() map[string]*CVDData {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	result := make(map[string]*CVDData)
	for symbol, calc := range manager.calculators {
		result[symbol] = calc.GetCurrentCVD()
	}

	return result
}

// Cleanup 清理不活跃的计算器
func (manager *CVDManager) Cleanup() {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	threshold := time.Now().Add(-2 * manager.config.CVDWindowDuration)
	
	for symbol, calc := range manager.calculators {
		calc.mu.RLock()
		isEmpty := len(calc.spotDeltas) == 0 && len(calc.futuresDeltas) == 0
		lastActivity := calc.lastCleanup
		calc.mu.RUnlock()

		// 如果计算器长时间无活动且无数据，则删除
		if isEmpty && lastActivity.Before(threshold) {
			delete(manager.calculators, symbol)
			log.Printf("🗑️  清理不活跃的CVD计算器: %s", symbol)
		}
	}
}

// ===== V2.0 5分钟CVD增量计算方法 =====

// UpdatePrice 更新价格（用于5分钟增量计算）
func (calc *CVDCalculator) UpdatePrice(price float64, timestamp time.Time) {
	calc.mu.Lock()
	defer calc.mu.Unlock()
	
	// 添加价格快照
	priceSnapshot := PriceSnapshot{
		Timestamp: timestamp,
		Price:     price,
	}
	calc.priceHistory = append(calc.priceHistory, priceSnapshot)
	
	// 保持6小时的价格历史
	cutoffTime := timestamp.Add(-6 * time.Hour)
	validHistory := make([]PriceSnapshot, 0, len(calc.priceHistory))
	for _, snapshot := range calc.priceHistory {
		if snapshot.Timestamp.After(cutoffTime) {
			validHistory = append(validHistory, snapshot)
		}
	}
	calc.priceHistory = validHistory
	
	// 检查是否需要更新5分钟增量
	if timestamp.Sub(calc.last5mSnapshot) >= 5*time.Minute {
		calc.update5MinuteDelta(timestamp)
	}
}

// update5MinuteDelta 更新5分钟增量数据（V2.0 - 集成缓存）
func (calc *CVDCalculator) update5MinuteDelta(currentTime time.Time) {
	// 计算5分钟CVD增量
	spotDelta := calc.currentSpotCVD - calc.last5mSpotCVD
	futuresDelta := calc.currentFuturesCVD - calc.last5mFuturesCVD
	
	// 计算价格变化
	var priceDeltaPct float64
	if len(calc.priceHistory) >= 2 {
		latestPrice := calc.priceHistory[len(calc.priceHistory)-1].Price
		// 找到5分钟前的价格
		for i := len(calc.priceHistory) - 1; i >= 0; i-- {
			if currentTime.Sub(calc.priceHistory[i].Timestamp) >= 5*time.Minute {
				oldPrice := calc.priceHistory[i].Price
				if oldPrice > 0 {
					priceDeltaPct = ((latestPrice - oldPrice) / oldPrice) * 100
				}
				break
			}
		}
	}
	
	// 推断K线意图（基于价格和CVD的组合）
	candleIntent := calc.inferCandleIntent(priceDeltaPct, spotDelta, futuresDelta)
	
	// 计算成交量变化（简化实现）
	volumeDelta := math.Abs(spotDelta) + math.Abs(futuresDelta)
	volumeRatio := 1.0 // 需要历史平均成交量数据来计算真实比例
	
	// 创建5分钟增量数据
	delta5m := &CVDDelta5m{
		PriceDeltaPct:       priceDeltaPct,
		SpotCVDDeltaUSD:     spotDelta,
		FuturesCVDDeltaUSD:  futuresDelta,
		OIDeltaPct:          0, // 需要OI数据
		CandleIntent:        candleIntent,
		VolumeDelta:         volumeDelta,
		VolumeRatio:         volumeRatio,
		PeriodStartTime:     calc.last5mSnapshot,
		PeriodEndTime:       currentTime,
		DataQuality:         calc.calculateDataQuality(),
	}
	
	// 缓存数据到内存映射（保持原有逻辑）
	cacheKey := currentTime.Format("15:04")
	calc.fiveMinuteCache[cacheKey] = delta5m
	
	// V2.0: 同时缓存到全局缓存系统
	globalCache := GetGlobalCache()
	globalCache.SetCVDDelta5m(calc.symbol, delta5m)
	
	// 获取完整的意图分析并缓存
	intentAnalysis := calc.analyzeCandleIntentWithConfidence(priceDeltaPct, spotDelta, futuresDelta)
	globalCache.SetCandleIntentAnalysis(calc.symbol, intentAnalysis)
	
	// 缓存价格历史
	globalCache.SetPriceHistory(calc.symbol, calc.priceHistory)
	
	// 更新快照
	calc.last5mSpotCVD = calc.currentSpotCVD
	calc.last5mFuturesCVD = calc.currentFuturesCVD
	calc.last5mSnapshot = currentTime
	
	log.Printf("📊 [%s] 5分钟CVD增量更新: 现货%.2f, 合约%.2f, 价格变化%.2f%%, 意图:%s (已缓存)",
		calc.symbol, spotDelta, futuresDelta, priceDeltaPct, candleIntent)
}

// inferCandleIntent 推断K线意图（V2.0核心功能 - 增强版）
func (calc *CVDCalculator) inferCandleIntent(priceDelta, spotDelta, futuresDelta float64) string {
	analysis := calc.analyzeCandleIntentWithConfidence(priceDelta, spotDelta, futuresDelta)
	return analysis.Intent
}

// analyzeCandleIntentWithConfidence 完整的K线意图分析（包含置信度）
func (calc *CVDCalculator) analyzeCandleIntentWithConfidence(priceDelta, spotDelta, futuresDelta float64) *CandleIntentAnalysis {
	// 计算各种市场力量的强度
	priceStrength := math.Abs(priceDelta) / 2.0 // 价格变化强度（标准化到2%为满分）
	spotStrength := math.Abs(spotDelta) / 500000 // 现货CVD强度（50万USD为满分）
	futuresStrength := math.Abs(futuresDelta) / 1000000 // 期货CVD强度（100万USD为满分）
	
	// 限制强度值在0-1范围内
	priceStrength = math.Min(1.0, priceStrength)
	spotStrength = math.Min(1.0, spotStrength)
	futuresStrength = math.Min(1.0, futuresStrength)
	
	// 定义价格和CVD方向
	priceUp := priceDelta > 0.1
	priceDown := priceDelta < -0.1
	spotBuy := spotDelta > 50000
	spotSell := spotDelta < -50000
	futuresBuy := futuresDelta > 50000
	futuresSell := futuresDelta < -50000
	
	// 计算整体市场力量对比
	bullishForce := 0.0
	bearishForce := 0.0
	
	if priceUp { bullishForce += priceStrength * 0.3 }
	if priceDown { bearishForce += priceStrength * 0.3 }
	if spotBuy { bullishForce += spotStrength * 0.4 }
	if spotSell { bearishForce += spotStrength * 0.4 }
	if futuresBuy { bullishForce += futuresStrength * 0.3 }
	if futuresSell { bearishForce += futuresStrength * 0.3 }
	
	// 初始化分析结果
	analysis := &CandleIntentAnalysis{
		Factors: make(map[string]float64),
	}
	
	// 记录各因子贡献度
	analysis.Factors["price_strength"] = priceStrength
	analysis.Factors["spot_strength"] = spotStrength
	analysis.Factors["futures_strength"] = futuresStrength
	analysis.Factors["bullish_force"] = bullishForce
	analysis.Factors["bearish_force"] = bearishForce
	
	// 智能意图推断逻辑
	analysis.Intent, analysis.Confidence, analysis.Strength = calc.determineIntentWithLogic(
		priceDelta, spotDelta, futuresDelta, 
		priceStrength, spotStrength, futuresStrength,
		bullishForce, bearishForce,
	)
	
	// 检查次要意图
	analysis.SecondaryIntent = calc.detectSecondaryIntent(priceDelta, spotDelta, futuresDelta, analysis.Intent)
	
	return analysis
}

// determineIntentWithLogic 核心意图判断逻辑
func (calc *CVDCalculator) determineIntentWithLogic(
	priceDelta, spotDelta, futuresDelta float64,
	priceStr, spotStr, futuresStr float64,
	bullishForce, bearishForce float64,
) (string, float64, float64) {
	
	// 1. 多头确认：价格上涨 + 现货和期货都净买入
	if priceDelta > 0.1 && spotDelta > 50000 && futuresDelta > 50000 {
		confidence := math.Min(0.95, (priceStr + spotStr + futuresStr) / 3.0 + 0.2)
		strength := bullishForce
		return CandleIntentBullishConfirm, confidence, strength
	}
	
	// 2. 空头确认：价格下跌 + 现货和期货都净卖出
	if priceDelta < -0.1 && spotDelta < -50000 && futuresDelta < -50000 {
		confidence := math.Min(0.95, (priceStr + spotStr + futuresStr) / 3.0 + 0.2)
		strength := bearishForce
		return CandleIntentBearishConfirm, confidence, strength
	}
	
	// 3. 散户推动假突破：价格大涨但现货卖出（机构套现）
	if priceDelta > 0.3 && spotDelta < -100000 {
		confidence := math.Min(0.9, priceStr + spotStr/2.0 + 0.1)
		strength := priceStr + spotStr/2.0
		return CandleIntentFakePumpRetail, confidence, strength
	}
	
	// 4. 散户推动假跌破：价格大跌但现货买入（机构抄底）
	if priceDelta < -0.3 && spotDelta > 100000 {
		confidence := math.Min(0.9, priceStr + spotStr/2.0 + 0.1)
		strength := priceStr + spotStr/2.0
		return CandleIntentFakeDumpRetail, confidence, strength
	}
	
	// 5. 主力吸筹：价格平稳但现货大量净买入
	if math.Abs(priceDelta) < 0.3 && spotDelta > 200000 {
		confidence := math.Min(0.85, spotStr + 0.3)
		strength := spotStr
		return CandleIntentSmartMoneyAccum, confidence, strength
	}
	
	// 6. 主力派发：价格平稳但现货大量净卖出
	if math.Abs(priceDelta) < 0.3 && spotDelta < -200000 {
		confidence := math.Min(0.85, spotStr + 0.3)
		strength := spotStr
		return CandleIntentSmartMoneyDistrib, confidence, strength
	}
	
	// 7. 期货领先：期货强势但现货跟随较弱
	if math.Abs(futuresDelta) > 200000 && math.Abs(spotDelta) < math.Abs(futuresDelta)/3 {
		confidence := math.Min(0.75, futuresStr + 0.2)
		strength := futuresStr
		if futuresDelta > 0 {
			return "futures_leading_bullish", confidence, strength
		} else {
			return "futures_leading_bearish", confidence, strength
		}
	}
	
	// 8. 现货领先：现货强势但期货跟随较弱
	if math.Abs(spotDelta) > 200000 && math.Abs(futuresDelta) < math.Abs(spotDelta)/3 {
		confidence := math.Min(0.75, spotStr + 0.2)
		strength := spotStr
		if spotDelta > 0 {
			return "spot_leading_bullish", confidence, strength
		} else {
			return "spot_leading_bearish", confidence, strength
		}
	}
	
	// 9. 整理状态：价格和成交量都较小
	if math.Abs(priceDelta) < 0.15 && math.Abs(spotDelta) < 80000 && math.Abs(futuresDelta) < 80000 {
		confidence := 0.7
		strength := 0.2
		return CandleIntentConsolidation, confidence, strength
	}
	
	// 10. 混合信号：现货期货方向相反
	if (spotDelta > 100000 && futuresDelta < -100000) || (spotDelta < -100000 && futuresDelta > 100000) {
		confidence := math.Min(0.8, (spotStr + futuresStr) / 2.0 + 0.2)
		strength := (spotStr + futuresStr) / 2.0
		return CandleIntentMixedSignals, confidence, strength
	}
	
	// 11. 数据不足或信号微弱
	if priceStr < 0.1 && spotStr < 0.1 && futuresStr < 0.1 {
		return CandleIntentDataInsufficient, 0.3, 0.1
	}
	
	// 默认：低确信度的整理状态
	return CandleIntentConsolidation, 0.5, (priceStr + spotStr + futuresStr) / 3.0
}

// detectSecondaryIntent 检测次要意图
func (calc *CVDCalculator) detectSecondaryIntent(priceDelta, spotDelta, futuresDelta float64, primaryIntent string) string {
	// 如果主要意图是确认型的，检查是否有其他次要力量
	if primaryIntent == CandleIntentBullishConfirm || primaryIntent == CandleIntentBearishConfirm {
		// 检查是否有机构套现/抄底的迹象
		if primaryIntent == CandleIntentBullishConfirm && spotDelta < 0 {
			return "institutional_profit_taking"
		}
		if primaryIntent == CandleIntentBearishConfirm && spotDelta > 0 {
			return "institutional_bottom_fishing"
		}
	}
	
	// 如果是假突破，检查真实意图
	if primaryIntent == CandleIntentFakePumpRetail || primaryIntent == CandleIntentFakeDumpRetail {
		if math.Abs(futuresDelta) > 200000 {
			return "futures_manipulation"
		}
	}
	
	// 如果是整理状态，检查暗流涌动
	if primaryIntent == CandleIntentConsolidation {
		if math.Abs(spotDelta) > 100000 && math.Abs(priceDelta) < 0.1 {
			return "silent_accumulation_distribution"
		}
	}
	
	return "" // 无明显次要意图
}

// GetCandleIntentAnalysis 获取完整的K线意图分析结果
func (calc *CVDCalculator) GetCandleIntentAnalysis() *CandleIntentAnalysis {
	calc.mu.RLock()
	defer calc.mu.RUnlock()
	
	if len(calc.priceHistory) < 2 {
		return &CandleIntentAnalysis{
			Intent:     CandleIntentDataInsufficient,
			Confidence: 0.2,
			Strength:   0.1,
			Factors:    map[string]float64{},
		}
	}
	
	// 计算最近5分钟的价格变化
	latest := calc.priceHistory[len(calc.priceHistory)-1]
	var priceDelta float64
	
	for i := len(calc.priceHistory) - 1; i >= 0; i-- {
		if latest.Timestamp.Sub(calc.priceHistory[i].Timestamp) >= 5*time.Minute {
			if calc.priceHistory[i].Price > 0 {
				priceDelta = ((latest.Price - calc.priceHistory[i].Price) / calc.priceHistory[i].Price) * 100
			}
			break
		}
	}
	
	// 计算5分钟CVD增量
	spotDelta := calc.currentSpotCVD - calc.last5mSpotCVD
	futuresDelta := calc.currentFuturesCVD - calc.last5mFuturesCVD
	
	return calc.analyzeCandleIntentWithConfidence(priceDelta, spotDelta, futuresDelta)
}

// calculateDataQuality 计算数据质量评分
func (calc *CVDCalculator) calculateDataQuality() float64 {
	// 基于数据完整性和时效性计算质量评分
	score := 1.0
	
	// 检查数据时效性（超过1分钟降分）
	timeSinceUpdate := time.Since(calc.lastCleanup)
	if timeSinceUpdate > time.Minute {
		score -= 0.2
	}
	if timeSinceUpdate > 5*time.Minute {
		score -= 0.3
	}
	
	// 检查数据量（交易笔数太少降分）
	totalTrades := len(calc.spotDeltas) + len(calc.futuresDeltas)
	if totalTrades < 10 {
		score -= 0.3
	} else if totalTrades < 50 {
		score -= 0.1
	}
	
	// 价格历史数据完整性
	if len(calc.priceHistory) < 10 {
		score -= 0.2
	}
	
	if score < 0 {
		score = 0
	}
	
	return score
}

// GetCVDDelta5m 获取最新的5分钟CVD增量数据
func (calc *CVDCalculator) GetCVDDelta5m() *CVDDelta5m {
	calc.mu.RLock()
	defer calc.mu.RUnlock()
	
	// 获取最新的5分钟数据
	var latest *CVDDelta5m
	var latestTime time.Time
	
	for _, delta := range calc.fiveMinuteCache {
		if delta.PeriodEndTime.After(latestTime) {
			latestTime = delta.PeriodEndTime
			latest = delta
		}
	}
	
	return latest
}

// ForceUpdate5MinuteDelta 强制更新5分钟增量数据（用于K线收盘同步）
func (calc *CVDCalculator) ForceUpdate5MinuteDelta() {
	calc.mu.Lock()
	defer calc.mu.Unlock()
	
	currentTime := time.Now()
	log.Printf("🔧 [%s] 强制更新5分钟增量数据", calc.symbol)
	calc.update5MinuteDelta(currentTime)
}