package microstructure

import (
	"log"
	"math"
	"sync"
	"time"
)

// ===== 🔧 修复: 数值计算稳定性工具函数 =====

// safeFloat64 确保浮点数在有效范围内，处理NaN和Inf
func safeFloat64(value float64, defaultValue float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return defaultValue
	}
	return value
}

// safeDivision 安全除法，避免除零和产生无效值
func safeDivision(numerator, denominator float64) float64 {
	if denominator == 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		return 0
	}
	if math.IsNaN(numerator) || math.IsInf(numerator, 0) {
		return 0
	}
	
	result := numerator / denominator
	return safeFloat64(result, 0)
}

// validateFinancialData 验证金融数据的有效性
func validateFinancialData(price, quantity float64) bool {
	return price > 0 && quantity > 0 && 
		   !math.IsNaN(price) && !math.IsInf(price, 0) &&
		   !math.IsNaN(quantity) && !math.IsInf(quantity, 0)
}

// NewCVDCalculator 创建CVD计算器
func NewCVDCalculator(symbol string, windowDuration time.Duration) *CVDCalculator {
	now := time.Now()
	return &CVDCalculator{
		symbol:           symbol,
		spotDeltas:       make([]CVDDelta, 0, 3600), // 预分配1小时的容量（假设每秒1笔交易）
		futuresDeltas:    make([]CVDDelta, 0, 3600),
		windowDuration:   windowDuration,
		lastCleanup:      now,
		currentSpotCVD:   0,
		currentFuturesCVD: 0,
		last5mSpotCVD:     0,
		last5mFuturesCVD:  0,
		last5mSnapshot:    now,
		lastDataUpdate:    now, // 🔧 修复: 初始化最后数据更新时间
		fiveMinuteCache:   make(map[string]*CVDDelta5m),
		priceHistory:      make([]PriceSnapshot, 0, 360), // 6小时价格历史
		volumeHistory:     make([]VolumeSnapshot, 0, 360), // 🔧 修复: 6小时成交量历史
	}
}

// ProcessTrade 处理交易数据，更新CVD
func (calc *CVDCalculator) ProcessTrade(trade *TradeData) {
	calc.mu.Lock()
	defer calc.mu.Unlock()

	// 🔧 修复: 验证交易数据的有效性
	if !validateFinancialData(trade.Price, trade.Quantity) {
		log.Printf("❌ [%s] 交易数据无效: 价格=%.8f, 数量=%.8f", 
			calc.symbol, trade.Price, trade.Quantity)
		return
	}

	// 计算交易的USD价值增量
	volumeUSD := trade.Price * trade.Quantity
	volumeUSD = safeFloat64(volumeUSD, 0) // 🔧 修复: 确保计算结果有效
	
	if volumeUSD <= 0 {
		log.Printf("❌ [%s] 计算出的成交量无效: %.8f", calc.symbol, volumeUSD)
		return
	}

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
		calc.currentSpotCVD = safeFloat64(calc.currentSpotCVD+deltaUSD, calc.currentSpotCVD) // 🔧 修复: 安全累加
	case "futures":
		calc.futuresDeltas = append(calc.futuresDeltas, delta)
		calc.currentFuturesCVD = safeFloat64(calc.currentFuturesCVD+deltaUSD, calc.currentFuturesCVD) // 🔧 修复: 安全累加
	}
	
	// 🔧 修复: 更新最后数据更新时间
	calc.lastDataUpdate = trade.Timestamp
	
	// 🔧 修复: 记录成交量历史用于Volume Ratio计算
	calc.updateVolumeHistory(volumeUSD, trade.Timestamp)

	// 🔧 修复: 优化清理时机，避免在增量计算期间清理
	// 定期清理过期数据（每5分钟清理一次，但要避免清理时机冲突）
	if time.Since(calc.lastCleanup) >= 5*time.Minute {
		// 在清理前保存当前的5分钟基准值，避免清理导致基准值丢失
		calc.preserveCurrentBaselines()
		calc.cleanupExpiredDataSafely()
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

	// 🔧 修复：CVD数据过期判断 - 基于最后收到交易数据的时间
	// CVD数据是基于交易流计算的，如果5分钟没有新交易数据，标记为过期
	isStale := time.Since(calc.lastDataUpdate) > 5*time.Minute

	return &CVDData{
		SpotCVD1H:     calc.currentSpotCVD,
		FuturesCVD1H:  calc.currentFuturesCVD,
		CVDDivergence: divergence,
		Signal:        signal,
		LastUpdate:    time.Now(),
		IsStale:       isStale, // 🔧 修复：使用实际的过期判断而非硬编码false
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

// ForceUpdateAllCVDDeltasWithKLineTime 使用精确K线收盘时间强制更新所有币种（🔧 新增：精确时序版本）
func (manager *CVDManager) ForceUpdateAllCVDDeltasWithKLineTime(klineCloseTime time.Time) {
	manager.mu.RLock()
	symbols := make([]string, 0, len(manager.calculators))
	for symbol := range manager.calculators {
		symbols = append(symbols, symbol)
	}
	manager.mu.RUnlock()
	
	log.Printf("🔧 基于K线收盘时间强制更新所有CVD增量数据: %s, 共%d个币种", 
		klineCloseTime.Format("15:04:05"), len(symbols))
	
	for _, symbol := range symbols {
		calc := manager.calculators[symbol]
		if calc != nil {
			calc.ForceUpdate5MinuteDeltaWithKLineTime(klineCloseTime)
		}
	}
	
	log.Printf("✅ 所有CVD精确时序增量数据更新完成")
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

// Cleanup 清理过期数据（不删除计算器本身）
func (manager *CVDManager) Cleanup() {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	log.Printf("🧹 开始清理CVD过期数据...")
	cleanedCount := 0
	
	for symbol, calc := range manager.calculators {
		// 调用每个计算器的内部清理方法
		beforeSpot := len(calc.spotDeltas)
		beforeFutures := len(calc.futuresDeltas)
		
		calc.cleanupExpiredData()
		
		afterSpot := len(calc.spotDeltas)
		afterFutures := len(calc.futuresDeltas)
		
		// 记录清理情况
		if beforeSpot != afterSpot || beforeFutures != afterFutures {
			log.Printf("🗑️  [%s] CVD数据清理: 现货 %d→%d, 合约 %d→%d", 
				symbol, beforeSpot, afterSpot, beforeFutures, afterFutures)
			cleanedCount++
		}
	}
	
	log.Printf("✅ CVD数据清理完成，清理了 %d 个币种的过期数据", cleanedCount)
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

// update5MinuteDelta 更新5分钟增量数据（V2.0 - 修复数据清理冲突）
func (calc *CVDCalculator) update5MinuteDelta(currentTime time.Time) {
	// 🔧 修复: 使用安全的增量计算方法，避免基准值丢失
	// 不再依赖易失的成员变量，而是基于实际数据计算
	spotDelta, futuresDelta := calc.calculateSafeCVDDelta(currentTime)
	
	// 计算价格变化
	var priceDeltaPct float64
	if len(calc.priceHistory) >= 2 {
		latestPrice := calc.priceHistory[len(calc.priceHistory)-1].Price
		// 🔧 修复: 优化价格查找算法，从线性查找改为高效算法
		oldPrice := calc.findPriceAtTime(currentTime.Add(-5 * time.Minute))
		if oldPrice > 0 {
			priceDeltaPct = ((latestPrice - oldPrice) / oldPrice) * 100
		}
	}
	
	// 推断K线意图（基于价格和CVD的组合）
	candleIntent := calc.inferCandleIntent(priceDeltaPct, spotDelta, futuresDelta)
	
	// 计算成交量变化（修复硬编码问题）
	volumeDelta := math.Abs(spotDelta) + math.Abs(futuresDelta)
	volumeRatio := calc.calculateVolumeRatio(volumeDelta) // 🔧 修复：实现真实的成交量比率计算
	
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
	
	// 🔧 修复: 更安全的基准值更新时机
	calc.updateBaselinesSafely(currentTime, spotDelta, futuresDelta)
	
	// 只在意图不是整理状态或数据质量低时才打印日志
	if candleIntent != CandleIntentConsolidation || delta5m.DataQuality < 0.5 {
		log.Printf("📊 [%s] 5分钟CVD增量更新: 现货%.0f, 合约%.0f, 价格变化%.2f%%, 意图:%s",
			calc.symbol, spotDelta, futuresDelta, priceDeltaPct, candleIntent)
	}
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

// determineIntentWithLogic 核心意图判断逻辑（🔧 修复：重新校准置信度算法）
func (calc *CVDCalculator) determineIntentWithLogic(
	priceDelta, spotDelta, futuresDelta float64,
	priceStr, spotStr, futuresStr float64,
	bullishForce, bearishForce float64,
) (string, float64, float64) {
	
	// 🔧 修复：计算数据质量因子，影响置信度
	dataQuality := calc.calculateCurrentDataQuality()
	
	// 🔧 修复：计算市场波动性，高波动时降低置信度
	volatilityPenalty := calc.calculateVolatilityPenalty()
	
	// 🔧 修复：基础置信度计算，更加保守
	baseConfidence := (priceStr + spotStr + futuresStr) / 3.0
	
	// 1. 多头确认：价格上涨 + 现货和期货都净买入
	if priceDelta > 0.1 && spotDelta > 50000 && futuresDelta > 50000 {
		// 🔧 修复：更保守的置信度计算，考虑数据质量和波动性
		confidence := baseConfidence * dataQuality * (1.0 - volatilityPenalty)
		confidence = math.Max(0.1, math.Min(0.85, confidence)) // 降低最大置信度至0.85
		strength := bullishForce
		return CandleIntentBullishConfirm, confidence, strength
	}
	
	// 2. 空头确认：价格下跌 + 现货和期货都净卖出
	if priceDelta < -0.1 && spotDelta < -50000 && futuresDelta < -50000 {
		// 🔧 修复：更保守的置信度计算
		confidence := baseConfidence * dataQuality * (1.0 - volatilityPenalty)
		confidence = math.Max(0.1, math.Min(0.85, confidence))
		strength := bearishForce
		return CandleIntentBearishConfirm, confidence, strength
	}
	
	// 3. 散户推动假突破：价格大涨但现货卖出（机构套现）
	if priceDelta > 0.3 && spotDelta < -100000 {
		// 🔧 修复：假突破信号置信度应该更低，因为难以确认
		confidence := (priceStr + spotStr/2.0) * dataQuality * 0.8 // 增加折扣因子
		confidence = math.Max(0.1, math.Min(0.75, confidence)) // 降低最大置信度
		strength := priceStr + spotStr/2.0
		return CandleIntentFakePumpRetail, confidence, strength
	}
	
	// 4. 散户推动假跌破：价格大跌但现货买入（机构抄底）
	if priceDelta < -0.3 && spotDelta > 100000 {
		// 🔧 修复：假跌破信号置信度应该更低
		confidence := (priceStr + spotStr/2.0) * dataQuality * 0.8
		confidence = math.Max(0.1, math.Min(0.75, confidence))
		strength := priceStr + spotStr/2.0
		return CandleIntentFakeDumpRetail, confidence, strength
	}
	
	// 5. 主力吸筹：价格平稳但现货大量净买入
	if math.Abs(priceDelta) < 0.3 && spotDelta > 200000 {
		// 🔧 修复：主力操作信号，中等置信度
		confidence := spotStr * dataQuality * 0.9
		confidence = math.Max(0.1, math.Min(0.70, confidence))
		strength := spotStr
		return CandleIntentSmartMoneyAccum, confidence, strength
	}
	
	// 6. 主力派发：价格平稳但现货大量净卖出
	if math.Abs(priceDelta) < 0.3 && spotDelta < -200000 {
		// 🔧 修复：主力操作信号，中等置信度
		confidence := spotStr * dataQuality * 0.9
		confidence = math.Max(0.1, math.Min(0.70, confidence))
		strength := spotStr
		return CandleIntentSmartMoneyDistrib, confidence, strength
	}
	
	// 7. 期货领先：期货强势但现货跟随较弱
	if math.Abs(futuresDelta) > 200000 && math.Abs(spotDelta) < math.Abs(futuresDelta)/3 {
		// 🔧 修复：期货领先信号，中低置信度
		confidence := futuresStr * dataQuality * 0.7
		confidence = math.Max(0.1, math.Min(0.65, confidence))
		strength := futuresStr
		if futuresDelta > 0 {
			return "futures_leading_bullish", confidence, strength
		} else {
			return "futures_leading_bearish", confidence, strength
		}
	}
	
	// 8. 现货领先：现货强势但期货跟随较弱
	if math.Abs(spotDelta) > 200000 && math.Abs(futuresDelta) < math.Abs(spotDelta)/3 {
		// 🔧 修复：现货领先信号，中低置信度
		confidence := spotStr * dataQuality * 0.7
		confidence = math.Max(0.1, math.Min(0.65, confidence))
		strength := spotStr
		if spotDelta > 0 {
			return "spot_leading_bullish", confidence, strength
		} else {
			return "spot_leading_bearish", confidence, strength
		}
	}
	
	// 9. 整理状态：价格和成交量都较小
	if math.Abs(priceDelta) < 0.15 && math.Abs(spotDelta) < 80000 && math.Abs(futuresDelta) < 80000 {
		confidence := 0.6 * dataQuality // 🔧 修复：整理状态也受数据质量影响
		strength := 0.2
		return CandleIntentConsolidation, confidence, strength
	}
	
	// 10. 混合信号：现货期货方向相反
	if (spotDelta > 100000 && futuresDelta < -100000) || (spotDelta < -100000 && futuresDelta > 100000) {
		// 🔧 修复：混合信号置信度应该较低，因为市场分歧大
		confidence := (spotStr + futuresStr) / 2.0 * dataQuality * 0.6
		confidence = math.Max(0.1, math.Min(0.60, confidence))
		strength := (spotStr + futuresStr) / 2.0
		return CandleIntentMixedSignals, confidence, strength
	}
	
	// 11. 数据不足或信号微弱
	if priceStr < 0.1 && spotStr < 0.1 && futuresStr < 0.1 {
		return CandleIntentDataInsufficient, 0.2, 0.1
	}
	
	// 默认：低确信度的整理状态
	defaultConfidence := (priceStr + spotStr + futuresStr) / 3.0 * dataQuality * 0.5
	defaultConfidence = math.Max(0.2, math.Min(0.5, defaultConfidence))
	return CandleIntentConsolidation, defaultConfidence, (priceStr + spotStr + futuresStr) / 3.0
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

// ForceUpdate5MinuteDeltaWithKLineTime 使用精确K线收盘时间更新5分钟增量（🔧 新增：解决时序对齐问题）
func (calc *CVDCalculator) ForceUpdate5MinuteDeltaWithKLineTime(klineCloseTime time.Time) {
	calc.mu.Lock()
	defer calc.mu.Unlock()
	
	log.Printf("🔧 [%s] 基于K线收盘时间更新5分钟增量: %s", 
		calc.symbol, klineCloseTime.Format("15:04:05"))
	calc.update5MinuteDeltaWithExactTiming(klineCloseTime)
}

// update5MinuteDeltaWithExactTiming 使用精确K线时序计算5分钟增量（🔧 新增：解决时序对齐问题）
func (calc *CVDCalculator) update5MinuteDeltaWithExactTiming(klineCloseTime time.Time) {
	// 🔧 修复: K线时间边界精确对齐
	// K线收盘时间例如09:35:00，对应的5分钟窗口是[09:30:00, 09:35:00)
	periodEndTime := klineCloseTime
	periodStartTime := klineCloseTime.Add(-5 * time.Minute)
	
	log.Printf("🕐 [%s] 精确K线时序计算: %s -> %s", calc.symbol, 
		periodStartTime.Format("15:04:05"), periodEndTime.Format("15:04:05"))
	
	// 使用精确时间窗口计算CVD增量
	spotDelta, futuresDelta := calc.calculateCVDDeltaForPeriod(periodStartTime, periodEndTime)
	
	// 计算价格变化（使用K线时序）
	var priceDeltaPct float64
	if len(calc.priceHistory) >= 2 {
		latestPrice := calc.findPriceAtTime(periodEndTime)
		oldPrice := calc.findPriceAtTime(periodStartTime)
		if oldPrice > 0 && latestPrice > 0 {
			priceDeltaPct = ((latestPrice - oldPrice) / oldPrice) * 100
		}
	}
	
	// 推断K线意图（基于精确时间窗口）
	candleIntent := calc.inferCandleIntent(priceDeltaPct, spotDelta, futuresDelta)
	
	// 计算成交量变化（基于精确时间窗口）
	volumeDelta := math.Abs(spotDelta) + math.Abs(futuresDelta)
	volumeRatio := calc.calculateVolumeRatioForPeriod(periodStartTime, periodEndTime)
	
	// 创建精确时序的5分钟增量数据
	delta5m := &CVDDelta5m{
		PriceDeltaPct:       priceDeltaPct,
		SpotCVDDeltaUSD:     spotDelta,
		FuturesCVDDeltaUSD:  futuresDelta,
		OIDeltaPct:          0, // 需要OI数据
		CandleIntent:        candleIntent,
		VolumeDelta:         volumeDelta,
		VolumeRatio:         volumeRatio,
		PeriodStartTime:     periodStartTime,
		PeriodEndTime:       periodEndTime,
		DataQuality:         calc.calculateDataQuality(),
	}
	
	// 缓存数据到内存映射
	cacheKey := periodEndTime.Format("15:04")
	calc.fiveMinuteCache[cacheKey] = delta5m
	
	// V2.0: 同时缓存到全局缓存系统
	globalCache := GetGlobalCache()
	globalCache.SetCVDDelta5m(calc.symbol, delta5m)
	
	// 获取完整的意图分析并缓存
	intentAnalysis := calc.analyzeCandleIntentWithConfidence(priceDeltaPct, spotDelta, futuresDelta)
	globalCache.SetCandleIntentAnalysis(calc.symbol, intentAnalysis)
	
	// 缓存价格历史
	globalCache.SetPriceHistory(calc.symbol, calc.priceHistory)
	
	// 🔧 修复: 更安全的基准值更新时机（使用K线时序）
	calc.updateBaselinesWithKLineTiming(periodEndTime, spotDelta, futuresDelta)
	
	log.Printf("📊 [%s] 精确K线增量更新: 现货%.0f, 合约%.0f, 价格变化%.2f%%, 意图:%s [%s-%s]",
		calc.symbol, spotDelta, futuresDelta, priceDeltaPct, candleIntent,
		periodStartTime.Format("15:04"), periodEndTime.Format("15:04"))
}

// calculateCVDDeltaForPeriod 计算指定时间段的CVD增量（🔧 新增：精确时序计算）
func (calc *CVDCalculator) calculateCVDDeltaForPeriod(startTime, endTime time.Time) (spotDelta, futuresDelta float64) {
	// 🔧 精确时间窗口：Sum(交易 where startTime <= 时间 < endTime)
	// 这确保了与K线时间边界的精确对齐
	
	spotDelta = 0
	for _, delta := range calc.spotDeltas {
		if (delta.Timestamp.Equal(startTime) || delta.Timestamp.After(startTime)) && 
		   delta.Timestamp.Before(endTime) {
			spotDelta += delta.DeltaUSD
		}
	}
	
	futuresDelta = 0
	for _, delta := range calc.futuresDeltas {
		if (delta.Timestamp.Equal(startTime) || delta.Timestamp.After(startTime)) && 
		   delta.Timestamp.Before(endTime) {
			futuresDelta += delta.DeltaUSD
		}
	}
	
	log.Printf("🔧 [%s] 精确时间窗口CVD计算: 现货增量=%.0f, 合约增量=%.0f [%s-%s]",
		calc.symbol, spotDelta, futuresDelta, 
		startTime.Format("15:04:05"), endTime.Format("15:04:05"))
	
	return spotDelta, futuresDelta
}

// calculateVolumeRatioForPeriod 计算指定时间段的成交量比率（🔧 新增：精确时序计算）
func (calc *CVDCalculator) calculateVolumeRatioForPeriod(startTime, endTime time.Time) float64 {
	// 计算当前时间段的总成交量
	var currentPeriodVolume float64
	for _, snapshot := range calc.volumeHistory {
		if (snapshot.Timestamp.Equal(startTime) || snapshot.Timestamp.After(startTime)) && 
		   snapshot.Timestamp.Before(endTime) {
			volumeUSD := safeFloat64(snapshot.VolumeUSD, 0)
			if volumeUSD > 0 {
				currentPeriodVolume += volumeUSD
			}
		}
	}
	
	// 计算历史平均（最近1小时，排除当前周期）
	oneHourAgo := endTime.Add(-1 * time.Hour)
	var totalHistoricalVolume float64
	var historicalPeriods int
	
	for _, snapshot := range calc.volumeHistory {
		if snapshot.Timestamp.After(oneHourAgo) && snapshot.Timestamp.Before(startTime) {
			volumeUSD := safeFloat64(snapshot.VolumeUSD, 0)
			if volumeUSD > 0 {
				totalHistoricalVolume += volumeUSD
				historicalPeriods++
			}
		}
	}
	
	if historicalPeriods == 0 || totalHistoricalVolume <= 0 {
		return 1.0 // 历史数据不足，返回默认值
	}
	
	avgHistoricalVolume := totalHistoricalVolume / float64(historicalPeriods)
	ratio := safeDivision(currentPeriodVolume, avgHistoricalVolume)
	
	// 限制比率在合理范围内
	if ratio > 50.0 {
		ratio = 50.0
	} else if ratio < 0.01 {
		ratio = 0.01
	}
	
	return safeFloat64(ratio, 1.0)
}

// updateBaselinesWithKLineTiming 使用K线时序更新基准值（🔧 新增：精确时序更新）
func (calc *CVDCalculator) updateBaselinesWithKLineTiming(klineCloseTime time.Time, spotDelta, futuresDelta float64) {
	// 更新快照基准值，使其与K线收盘时间精确对齐
	calc.last5mSpotCVD = calc.currentSpotCVD
	calc.last5mFuturesCVD = calc.currentFuturesCVD
	calc.last5mSnapshot = klineCloseTime
	
	log.Printf("🔧 [%s] K线时序基准值更新: 现货基准=%.0f, 合约基准=%.0f, K线收盘=%s", 
		calc.symbol, calc.last5mSpotCVD, calc.last5mFuturesCVD, 
		klineCloseTime.Format("15:04:05"))
}

// preserveCurrentBaselines 在清理前保存当前基准值
func (calc *CVDCalculator) preserveCurrentBaselines() {
	// 记录清理前的基准值，防止数据丢失
	log.Printf("🔧 [%s] 保存清理前基准值: 现货CVD=%.0f, 合约CVD=%.0f", 
		calc.symbol, calc.last5mSpotCVD, calc.last5mFuturesCVD)
}

// cleanupExpiredDataSafely 安全清理过期数据
func (calc *CVDCalculator) cleanupExpiredDataSafely() {
	cutoffTime := time.Now().Add(-calc.windowDuration)
	
	// 清理现货数据但保持计数器准确
	oldSpotCVD := calc.currentSpotCVD
	calc.currentSpotCVD = 0
	validSpotDeltas := make([]CVDDelta, 0, len(calc.spotDeltas))
	for _, delta := range calc.spotDeltas {
		if delta.Timestamp.After(cutoffTime) {
			validSpotDeltas = append(validSpotDeltas, delta)
			calc.currentSpotCVD += delta.DeltaUSD
		}
	}
	calc.spotDeltas = validSpotDeltas

	// 清理合约数据但保持计数器准确
	oldFuturesCVD := calc.currentFuturesCVD
	calc.currentFuturesCVD = 0
	validFuturesDeltas := make([]CVDDelta, 0, len(calc.futuresDeltas))
	for _, delta := range calc.futuresDeltas {
		if delta.Timestamp.After(cutoffTime) {
			validFuturesDeltas = append(validFuturesDeltas, delta)
			calc.currentFuturesCVD += delta.DeltaUSD
		}
	}
	calc.futuresDeltas = validFuturesDeltas

	// 🔧 修复: 清理成交量历史数据（保持6小时数据）
	volumeCutoffTime := time.Now().Add(-6 * time.Hour)
	validVolumeHistory := make([]VolumeSnapshot, 0, len(calc.volumeHistory))
	for _, snapshot := range calc.volumeHistory {
		if snapshot.Timestamp.After(volumeCutoffTime) {
			validVolumeHistory = append(validVolumeHistory, snapshot)
		}
	}
	oldVolumeCount := len(calc.volumeHistory)
	calc.volumeHistory = validVolumeHistory

	log.Printf("🧹 [%s] 安全CVD数据清理完成 - 现货记录:%d (CVD %.0f→%.0f), 合约记录:%d (CVD %.0f→%.0f), 成交量记录:%d→%d", 
		calc.symbol, len(calc.spotDeltas), oldSpotCVD, calc.currentSpotCVD, 
		len(calc.futuresDeltas), oldFuturesCVD, calc.currentFuturesCVD, oldVolumeCount, len(calc.volumeHistory))
}

// calculateSafeCVDDelta 安全计算CVD增量（基于实际数据而非易失变量）
// 🔧 修复"漏桶效应"：使用实时切片求和而非基于总量的减法
func (calc *CVDCalculator) calculateSafeCVDDelta(currentTime time.Time) (spotDelta, futuresDelta float64) {
	fiveMinutesAgo := currentTime.Add(-5 * time.Minute)
	
	// 🔧 修复：直接计算最近5分钟的交易增量，避免滑动窗口导致的漏桶效应
	// 现货5分钟增量：Sum(现货交易 where 时间 > 现在-5分钟)
	spotDelta = 0
	for _, delta := range calc.spotDeltas {
		if delta.Timestamp.After(fiveMinutesAgo) {
			spotDelta += delta.DeltaUSD
		}
	}
	
	// 合约5分钟增量：Sum(合约交易 where 时间 > 现在-5分钟)
	futuresDelta = 0
	for _, delta := range calc.futuresDeltas {
		if delta.Timestamp.After(fiveMinutesAgo) {
			futuresDelta += delta.DeltaUSD
		}
	}
	
	log.Printf("🔧 [%s] CVD增量计算修复：现货5m增量=%.0f, 合约5m增量=%.0f (基于%s后的交易)",
		calc.symbol, spotDelta, futuresDelta, fiveMinutesAgo.Format("15:04:05"))
	
	return spotDelta, futuresDelta
}

// findPriceAtTime 高效查找指定时间点的价格（二分查找优化）
func (calc *CVDCalculator) findPriceAtTime(targetTime time.Time) float64 {
	if len(calc.priceHistory) == 0 {
		return 0
	}
	
	// 🔧 修复: 使用二分查找替代线性查找，提高效率
	left, right := 0, len(calc.priceHistory)-1
	bestIndex := -1
	
	for left <= right {
		mid := (left + right) / 2
		if calc.priceHistory[mid].Timestamp.Before(targetTime) {
			bestIndex = mid
			left = mid + 1
		} else {
			right = mid - 1
		}
	}
	
	if bestIndex >= 0 {
		return calc.priceHistory[bestIndex].Price
	}
	
	// 如果没找到精确时间点，返回最早的价格
	return calc.priceHistory[0].Price
}

// updateBaselinesSafely 安全更新基准值
func (calc *CVDCalculator) updateBaselinesSafely(currentTime time.Time, spotDelta, futuresDelta float64) {
	// 更新快照基准值时确保数据一致性
	calc.last5mSpotCVD = calc.currentSpotCVD
	calc.last5mFuturesCVD = calc.currentFuturesCVD
	calc.last5mSnapshot = currentTime
	
	log.Printf("🔧 [%s] 基准值安全更新: 现货基准=%.0f, 合约基准=%.0f", 
		calc.symbol, calc.last5mSpotCVD, calc.last5mFuturesCVD)
}

// ===== 🔧 修复: Volume Ratio 计算方法 =====

// updateVolumeHistory 更新成交量历史
func (calc *CVDCalculator) updateVolumeHistory(volumeUSD float64, timestamp time.Time) {
	// 添加成交量快照
	volumeSnapshot := VolumeSnapshot{
		Timestamp: timestamp,
		VolumeUSD: volumeUSD,
	}
	calc.volumeHistory = append(calc.volumeHistory, volumeSnapshot)
	
	// 保持6小时的成交量历史
	cutoffTime := timestamp.Add(-6 * time.Hour)
	validHistory := make([]VolumeSnapshot, 0, len(calc.volumeHistory))
	for _, snapshot := range calc.volumeHistory {
		if snapshot.Timestamp.After(cutoffTime) {
			validHistory = append(validHistory, snapshot)
		}
	}
	calc.volumeHistory = validHistory
}

// calculateVolumeRatio 计算当前成交量相对于历史平均值的倍数（修复硬编码问题）
func (calc *CVDCalculator) calculateVolumeRatio(currentVolumeUSD float64) float64 {
	// 🔧 修复: 验证输入数据的有效性
	currentVolumeUSD = safeFloat64(currentVolumeUSD, 0)
	
	if len(calc.volumeHistory) < 30 { // 至少需要30个历史数据点
		return 1.0 // 数据不足时返回默认值
	}
	
	// 计算最近1小时的平均成交量
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	var totalVolume float64
	var count int
	
	for _, snapshot := range calc.volumeHistory {
		if snapshot.Timestamp.After(oneHourAgo) {
			// 🔧 修复: 验证历史数据有效性
			volumeUSD := safeFloat64(snapshot.VolumeUSD, 0)
			if volumeUSD > 0 {
				totalVolume += volumeUSD
				count++
			}
		}
	}
	
	if count == 0 || totalVolume <= 0 {
		return 1.0 // 避免除零
	}
	
	// 🔧 修复: 使用安全除法计算比率
	ratio := safeDivision(currentVolumeUSD, totalVolume/float64(count))
	
	// 限制比率在合理范围内，避免极端值
	if ratio > 50.0 {
		ratio = 50.0
	} else if ratio < 0.01 {
		ratio = 0.01
	}
	
	return safeFloat64(ratio, 1.0) // 🔧 修复: 确保返回值有效
}

// ===== 🔧 修复: K线意图置信度校准工具函数 =====

// calculateCurrentDataQuality 计算当前数据质量评分（影响置信度）
func (calc *CVDCalculator) calculateCurrentDataQuality() float64 {
	quality := 1.0
	
	// 基于数据新鲜度评估
	dataAge := time.Since(calc.lastDataUpdate).Minutes()
	if dataAge > 5 {
		quality *= 0.5 // 数据超过5分钟，质量大幅下降
	} else if dataAge > 2 {
		quality *= 0.8 // 数据超过2分钟，适度降低质量
	}
	
	// 基于数据量评估
	totalTrades := len(calc.spotDeltas) + len(calc.futuresDeltas)
	if totalTrades < 10 {
		quality *= 0.6 // 交易数据不足，降低质量
	} else if totalTrades < 50 {
		quality *= 0.9 // 交易数据较少，轻微降低质量
	}
	
	// 基于价格历史完整性
	if len(calc.priceHistory) < 10 {
		quality *= 0.7 // 价格历史不足，降低质量
	}
	
	return math.Max(0.1, math.Min(1.0, quality))
}

// calculateVolatilityPenalty 计算波动性惩罚因子（高波动时降低置信度）
func (calc *CVDCalculator) calculateVolatilityPenalty() float64 {
	if len(calc.priceHistory) < 10 {
		return 0.2 // 数据不足时给予默认惩罚
	}
	
	// 计算最近1小时的价格波动率
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	var prices []float64
	
	for _, snapshot := range calc.priceHistory {
		if snapshot.Timestamp.After(oneHourAgo) {
			prices = append(prices, snapshot.Price)
		}
	}
	
	if len(prices) < 5 {
		return 0.2 // 价格数据不足
	}
	
	// 计算价格标准差
	var sum, mean, variance float64
	for _, price := range prices {
		sum += price
	}
	mean = sum / float64(len(prices))
	
	for _, price := range prices {
		diff := price - mean
		variance += diff * diff
	}
	variance /= float64(len(prices))
	stdDev := math.Sqrt(variance)
	
	// 计算变异系数（标准差/均值）
	volatilityRatio := safeDivision(stdDev, mean)
	
	// 波动性惩罚：变异系数越高，惩罚越大
	penalty := volatilityRatio * 100 // 转换为百分比
	
	// 限制惩罚范围 0-0.5
	return math.Max(0, math.Min(0.5, penalty/10))
}