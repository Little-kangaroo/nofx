package microstructure

import (
	"log"
	"math"
	"sync"
	"time"
)

// ImbalancePoint 失衡点记录
type ImbalancePoint struct {
	Timestamp time.Time `json:"timestamp"`
	Ratio     float64   `json:"ratio"`
}

// WallTracker 挂单墙追踪器（V2.0新增）
type WallTracker struct {
	wallHistory     map[float64]*WallHistoryEntry // 价格 -> 墙历史
	maxWallAge      time.Duration                 // 墙的最大存活时间
	flickerThreshold int                          // 闪烁阈值次数
}

// WallHistoryEntry 挂单墙历史记录
type WallHistoryEntry struct {
	price         float64
	firstSeen     time.Time
	lastSeen      time.Time
	appearances   int        // 出现次数
	disappearances int       // 消失次数
	maxSize       float64    // 历史最大规模
	minSize       float64    // 历史最小规模
	avgSize       float64    // 平均规模
	sizeHistory   []float64  // 规模历史
	isActive      bool       // 当前是否活跃
}

// SymbolOrderBookData 单个交易对的订单簿数据
type SymbolOrderBookData struct {
	currentBids       []OrderBookLevel // 当前买单档位
	currentAsks       []OrderBookLevel // 当前卖单档位
	lastUpdate        time.Time
	currentPrice      float64          // 当前价格（用于计算距离）
	imbalanceHistory  []ImbalancePoint // 失衡历史（用于平滑）
	wallTracker       *WallTracker     // 挂单墙追踪器
	wallChangeCount5m int              // 5分钟内墙变化次数
	last5mReset       time.Time        // 最后5分钟重置时间
}

// OrderBookCalculator 盘口计算器（支持多交易对）
type OrderBookCalculator struct {
	mu               sync.RWMutex
	symbolData       map[string]*SymbolOrderBookData // symbol -> 数据
	wallThreshold    float64                          // 挂单墙阈值倍数（默认5倍平均档位量）
	maxHistorySize   int                              // 最大历史记录数量
}

// NewOrderBookCalculator 创建盘口计算器（支持多交易对）
func NewOrderBookCalculator(wallThreshold float64) *OrderBookCalculator {
	return &OrderBookCalculator{
		symbolData:       make(map[string]*SymbolOrderBookData),
		wallThreshold:    wallThreshold,
		maxHistorySize:   100,
	}
}

// getOrCreateSymbolData 获取或创建交易对数据
func (calc *OrderBookCalculator) getOrCreateSymbolData(symbol string) *SymbolOrderBookData {
	if data, exists := calc.symbolData[symbol]; exists {
		return data
	}
	
	// 创建新的交易对数据
	data := &SymbolOrderBookData{
		currentBids:      make([]OrderBookLevel, 0, 20),
		currentAsks:      make([]OrderBookLevel, 0, 20),
		imbalanceHistory: make([]ImbalancePoint, 0, calc.maxHistorySize),
		wallTracker: &WallTracker{
			wallHistory:     make(map[float64]*WallHistoryEntry),
			maxWallAge:      30 * time.Minute,
			flickerThreshold: 3,
		},
		last5mReset: time.Now(),
	}
	
	calc.symbolData[symbol] = data
	return data
}

// ProcessDepthData 处理盘口数据更新
func (calc *OrderBookCalculator) ProcessDepthData(symbol string, depthData *DepthData) {
	calc.mu.Lock()
	defer calc.mu.Unlock()

	// 获取或创建交易对数据
	symbolData := calc.getOrCreateSymbolData(symbol)

	// 更新盘口数据
	symbolData.currentBids = make([]OrderBookLevel, len(depthData.Bids))
	copy(symbolData.currentBids, depthData.Bids)

	symbolData.currentAsks = make([]OrderBookLevel, len(depthData.Asks))
	copy(symbolData.currentAsks, depthData.Asks)

	symbolData.lastUpdate = depthData.Timestamp

	// 更新当前价格（使用盘口中间价）
	if len(symbolData.currentBids) > 0 && len(symbolData.currentAsks) > 0 {
		bestBid := symbolData.currentBids[0].Price
		bestAsk := symbolData.currentAsks[0].Price
		symbolData.currentPrice = (bestBid + bestAsk) / 2
	}

	// V2.0: 检查5分钟周期重置
	if depthData.Timestamp.Sub(symbolData.last5mReset) >= 5*time.Minute {
		symbolData.wallChangeCount5m = 0
		symbolData.last5mReset = depthData.Timestamp
	}

	// V2.0: 更新墙追踪
	calc.updateWallTracking(symbol, depthData.Timestamp)

	// 计算并记录失衡比例
	imbalanceRatio := calc.calculateImbalance(symbol)
	calc.addImbalancePoint(symbol, ImbalancePoint{
		Timestamp: depthData.Timestamp,
		Ratio:     imbalanceRatio,
	})
}

// calculateImbalance 计算买卖失衡比例
func (calc *OrderBookCalculator) calculateImbalance(symbol string) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil || len(symbolData.currentBids) == 0 || len(symbolData.currentAsks) == 0 {
		return 0
	}

	// 计算买单总价值（前10档）
	var totalBidValue float64
	maxLevels := int(math.Min(float64(len(symbolData.currentBids)), 10))
	for i := 0; i < maxLevels; i++ {
		bid := symbolData.currentBids[i]
		totalBidValue += bid.Price * bid.Quantity
	}

	// 计算卖单总价值（前10档）
	var totalAskValue float64
	maxLevels = int(math.Min(float64(len(symbolData.currentAsks)), 10))
	for i := 0; i < maxLevels; i++ {
		ask := symbolData.currentAsks[i]
		totalAskValue += ask.Price * ask.Quantity
	}

	// 计算失衡比例 (-1 到 1)
	totalValue := totalBidValue + totalAskValue
	if totalValue == 0 {
		return 0
	}

	return (totalBidValue - totalAskValue) / totalValue
}

// addImbalancePoint 添加失衡记录点
func (calc *OrderBookCalculator) addImbalancePoint(symbol string, point ImbalancePoint) {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return
	}
	
	symbolData.imbalanceHistory = append(symbolData.imbalanceHistory, point)

	// 限制历史记录大小
	if len(symbolData.imbalanceHistory) > calc.maxHistorySize {
		symbolData.imbalanceHistory = symbolData.imbalanceHistory[len(symbolData.imbalanceHistory)-calc.maxHistorySize:]
	}
}

// findWalls 识别挂单墙
func (calc *OrderBookCalculator) findWalls(symbol string) (resistance *WallInfo, support *WallInfo) {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return nil, nil
	}
	
	// 计算平均档位量
	avgBidValue := calc.calculateAverageLevel(symbolData.currentBids)
	avgAskValue := calc.calculateAverageLevel(symbolData.currentAsks)
	avgValue := (avgBidValue + avgAskValue) / 2

	if avgValue == 0 {
		return nil, nil
	}

	threshold := avgValue * calc.wallThreshold

	// 查找阻力墙（卖单墙）
	resistance = calc.findResistanceWall(symbol, threshold)

	// 查找支撑墙（买单墙）
	support = calc.findSupportWall(symbol, threshold)

	return resistance, support
}

// calculateAverageLevel 计算平均档位价值
func (calc *OrderBookCalculator) calculateAverageLevel(levels []OrderBookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}

	var totalValue float64
	for _, level := range levels {
		totalValue += level.Price * level.Quantity
	}

	return totalValue / float64(len(levels))
}

// findResistanceWall 查找阻力墙
func (calc *OrderBookCalculator) findResistanceWall(symbol string, threshold float64) *WallInfo {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil || len(symbolData.currentAsks) == 0 {
		return nil
	}

	var wallPrice float64
	var wallStrength float64
	var wallLevels int

	// 查找连续的大档位
	for i, ask := range symbolData.currentAsks {
		levelValue := ask.Price * ask.Quantity

		if levelValue > threshold {
			if wallPrice == 0 {
				wallPrice = ask.Price
				wallStrength = levelValue
				wallLevels = 1
			} else {
				// 检查是否是连续的墙
				if i > 0 {
					prevPrice := symbolData.currentAsks[i-1].Price
					if (ask.Price-prevPrice)/prevPrice < 0.001 { // 价差小于0.1%认为连续
						wallStrength += levelValue
						wallLevels++
					} else {
						break // 不连续，停止累加
					}
				}
			}
		} else if wallPrice != 0 {
			break // 已找到墙，后面档位不够大，停止
		}
	}

	if wallPrice == 0 {
		return nil
	}

	// 计算距离当前价格的百分比
	distance := 0.0
	if symbolData.currentPrice > 0 {
		distance = (wallPrice - symbolData.currentPrice) / symbolData.currentPrice * 100
	}

	// V2.0: 计算稳定性评分
	stabilityScore, flickerCount, existenceDuration := calc.calculateWallStability(symbol, wallPrice, wallStrength)

	return &WallInfo{
		Price:       wallPrice,
		StrengthUSD: wallStrength,
		IsSolid:     wallStrength > threshold*2, // 超过2倍阈值认为是实墙
		Distance:    distance,
		LevelCount:  wallLevels,
		// V2.0 新增字段
		StabilityScore:    stabilityScore,
		FlickerCount:     flickerCount,
		ExistenceDuration: existenceDuration,
		LastSeen:         symbolData.lastUpdate,
		FirstSeen:        calc.getWallFirstSeen(symbol, wallPrice),
		AverageSize:      calc.getWallAverageSize(symbol, wallPrice),
		MaxSize:          calc.getWallMaxSize(symbol, wallPrice),
		MinSize:          calc.getWallMinSize(symbol, wallPrice),
	}
}

// findSupportWall 查找支撑墙
func (calc *OrderBookCalculator) findSupportWall(symbol string, threshold float64) *WallInfo {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil || len(symbolData.currentBids) == 0 {
		return nil
	}

	var wallPrice float64
	var wallStrength float64
	var wallLevels int

	// 查找连续的大档位（买单从高到低）
	for i, bid := range symbolData.currentBids {
		levelValue := bid.Price * bid.Quantity

		if levelValue > threshold {
			if wallPrice == 0 {
				wallPrice = bid.Price
				wallStrength = levelValue
				wallLevels = 1
			} else {
				// 检查是否是连续的墙
				if i > 0 {
					prevPrice := symbolData.currentBids[i-1].Price
					if (prevPrice-bid.Price)/prevPrice < 0.001 { // 价差小于0.1%认为连续
						wallStrength += levelValue
						wallLevels++
					} else {
						break // 不连续，停止累加
					}
				}
			}
		} else if wallPrice != 0 {
			break // 已找到墙，后面档位不够大，停止
		}
	}

	if wallPrice == 0 {
		return nil
	}

	// 计算距离当前价格的百分比
	distance := 0.0
	if symbolData.currentPrice > 0 {
		distance = (symbolData.currentPrice - wallPrice) / symbolData.currentPrice * 100
	}

	// V2.0: 计算稳定性评分
	stabilityScore, flickerCount, existenceDuration := calc.calculateWallStability(symbol, wallPrice, wallStrength)

	return &WallInfo{
		Price:       wallPrice,
		StrengthUSD: wallStrength,
		IsSolid:     wallStrength > threshold*2, // 超过2倍阈值认为是实墙
		Distance:    distance,
		LevelCount:  wallLevels,
		// V2.0 新增字段
		StabilityScore:    stabilityScore,
		FlickerCount:     flickerCount,
		ExistenceDuration: existenceDuration,
		LastSeen:         symbolData.lastUpdate,
		FirstSeen:        calc.getWallFirstSeen(symbol, wallPrice),
		AverageSize:      calc.getWallAverageSize(symbol, wallPrice),
		MaxSize:          calc.getWallMaxSize(symbol, wallPrice),
		MinSize:          calc.getWallMinSize(symbol, wallPrice),
	}
}

// getSmoothedImbalance 获取平滑后的失衡比例
func (calc *OrderBookCalculator) getSmoothedImbalance(symbol string, smoothPeriods int) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil || len(symbolData.imbalanceHistory) == 0 {
		return 0
	}

	// 取最近N个点计算平均值
	startIdx := len(symbolData.imbalanceHistory) - smoothPeriods
	if startIdx < 0 {
		startIdx = 0
	}

	var sum float64
	count := 0
	for i := startIdx; i < len(symbolData.imbalanceHistory); i++ {
		sum += symbolData.imbalanceHistory[i].Ratio
		count++
	}

	if count == 0 {
		return 0
	}

	return sum / float64(count)
}

// GetCurrentOrderBookData 获取当前盘口分析数据（V2.0 - 集成缓存）
func (calc *OrderBookCalculator) GetCurrentOrderBookData(symbol string, smoothPeriods int) *OrderBookData {
	calc.mu.RLock()
	defer calc.mu.RUnlock()

	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		// 返回空数据，标记为过期
		return &OrderBookData{
			ImbalanceRatio:    0,
			NearestResistance: nil,
			NearestSupport:    nil,
			BidPressure:       0,
			AskPressure:       0,
			ImbalanceTrend:    "insufficient_data",
			PressureDelta5m:   0,
			SpoofingRisk:      0,
			LiquidityScore:    0,
			WallChangeCount5m: 0,
			LastUpdate:        time.Time{},
			IsStale:           true,
		}
	}

	// 计算失衡比例（平滑）
	imbalanceRatio := calc.getSmoothedImbalance(symbol, smoothPeriods)

	// 识别挂单墙
	resistance, support := calc.findWalls(symbol)

	// 计算买卖压力
	bidPressure := calc.calculatePressure(symbolData.currentBids)
	askPressure := calc.calculatePressure(symbolData.currentAsks)

	// 检查数据是否过期 - 调整为10分钟阈值适配5分钟决策周期
	isStale := time.Since(symbolData.lastUpdate) > 10*time.Minute

	// V2.0: 计算额外的市场微观结构指标
	imbalanceTrend := calc.calculateImbalanceTrend(symbol)
	pressureDelta5m := calc.calculatePressureDelta5m(symbol, bidPressure, askPressure)
	spoofingRisk := calc.calculateSpoofingRisk(symbol)
	liquidityScore := calc.calculateLiquidityScore(symbol)

	orderBookData := &OrderBookData{
		ImbalanceRatio:    imbalanceRatio,
		NearestResistance: resistance,
		NearestSupport:    support,
		BidPressure:       bidPressure,
		AskPressure:       askPressure,
		// V2.0 新增字段
		ImbalanceTrend:      imbalanceTrend,
		PressureDelta5m:     pressureDelta5m,
		SpoofingRisk:        spoofingRisk,
		LiquidityScore:      liquidityScore,
		WallChangeCount5m:   symbolData.wallChangeCount5m,
		LastUpdate:          symbolData.lastUpdate,
		IsStale:             isStale,
	}

	// V2.0: 缓存到全局缓存系统
	globalCache := GetGlobalCache()
	globalCache.SetOrderBookData(symbol, orderBookData)

	log.Printf("📊 [%s] 盘口数据已更新并缓存: 失衡比%.3f, 虚假挂单风险%.3f, 流动性评分%.3f",
		symbol, imbalanceRatio, spoofingRisk, liquidityScore)

	return orderBookData
}

// calculatePressure 计算买卖压力强度
func (calc *OrderBookCalculator) calculatePressure(levels []OrderBookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}

	// 计算前5档的总价值
	var totalValue float64
	maxLevels := int(math.Min(float64(len(levels)), 5))

	for i := 0; i < maxLevels; i++ {
		level := levels[i]
		totalValue += level.Price * level.Quantity
	}

	return totalValue
}

// ===== V2.0 新增方法 =====

// updateWallTracking 更新挂单墙追踪信息（V2.0）
func (calc *OrderBookCalculator) updateWallTracking(symbol string, timestamp time.Time) {
	// 获取当前的挂单墙
	resistance, support := calc.findWalls(symbol)
	
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return
	}
	
	// 更新阻力墙追踪
	if resistance != nil {
		calc.trackWall(symbol, resistance.Price, resistance.StrengthUSD, timestamp)
	}
	
	// 更新支撑墙追踪
	if support != nil {
		calc.trackWall(symbol, support.Price, support.StrengthUSD, timestamp)
	}
	
	// 清理过期的墙记录
	calc.cleanupExpiredWalls(symbol, timestamp)
}

// trackWall 追踪单个墙的变化
func (calc *OrderBookCalculator) trackWall(symbol string, price, size float64, timestamp time.Time) {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return
	}
	
	entry, exists := symbolData.wallTracker.wallHistory[price]
	
	if !exists {
		// 新墙
		symbolData.wallTracker.wallHistory[price] = &WallHistoryEntry{
			price:         price,
			firstSeen:     timestamp,
			lastSeen:      timestamp,
			appearances:   1,
			disappearances: 0,
			maxSize:       size,
			minSize:       size,
			avgSize:       size,
			sizeHistory:   []float64{size},
			isActive:      true,
		}
		symbolData.wallChangeCount5m++
	} else {
		// 更新现有墙
		entry.lastSeen = timestamp
		
		if !entry.isActive {
			// 墙重新出现
			entry.appearances++
			entry.isActive = true
			symbolData.wallChangeCount5m++
		}
		
		// 更新大小统计
		entry.sizeHistory = append(entry.sizeHistory, size)
		if size > entry.maxSize {
			entry.maxSize = size
		}
		if size < entry.minSize {
			entry.minSize = size
		}
		
		// 计算平均大小
		total := 0.0
		for _, s := range entry.sizeHistory {
			total += s
		}
		entry.avgSize = total / float64(len(entry.sizeHistory))
	}
}

// cleanupExpiredWalls 清理过期的墙记录
func (calc *OrderBookCalculator) cleanupExpiredWalls(symbol string, timestamp time.Time) {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return
	}
	
	cutoffTime := timestamp.Add(-symbolData.wallTracker.maxWallAge)
	
	for price, entry := range symbolData.wallTracker.wallHistory {
		if entry.lastSeen.Before(cutoffTime) {
			delete(symbolData.wallTracker.wallHistory, price)
		} else if entry.isActive {
			// 检查墙是否已消失（在当前盘口中不存在）
			if !calc.isWallCurrentlyPresent(symbol, price) {
				entry.isActive = false
				entry.disappearances++
				symbolData.wallChangeCount5m++
			}
		}
	}
}

// isWallCurrentlyPresent 检查指定价格的墙是否在当前盘口中存在
func (calc *OrderBookCalculator) isWallCurrentlyPresent(symbol string, price float64) bool {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return false
	}
	
	// 计算阈值
	avgBidValue := calc.calculateAverageLevel(symbolData.currentBids)
	avgAskValue := calc.calculateAverageLevel(symbolData.currentAsks)
	avgValue := (avgBidValue + avgAskValue) / 2
	
	if avgValue == 0 {
		return false
	}
	
	threshold := avgValue * calc.wallThreshold
	
	// 检查买单档位
	for _, bid := range symbolData.currentBids {
		if math.Abs(bid.Price-price) < 0.0001 { // 价格匹配
			levelValue := bid.Price * bid.Quantity
			return levelValue > threshold
		}
	}
	
	// 检查卖单档位
	for _, ask := range symbolData.currentAsks {
		if math.Abs(ask.Price-price) < 0.0001 { // 价格匹配
			levelValue := ask.Price * ask.Quantity
			return levelValue > threshold
		}
	}
	
	return false
}

// calculateWallStability 计算墙稳定性评分（V2.0核心功能）
func (calc *OrderBookCalculator) calculateWallStability(symbol string, price, currentSize float64) (float64, int, time.Duration) {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return 0.5, 0, 0
	}
	
	entry, exists := symbolData.wallTracker.wallHistory[price]
	if !exists {
		// 新墙，返回默认值
		return 0.5, 0, 0
	}
	
	// 计算存在时长
	existenceDuration := entry.lastSeen.Sub(entry.firstSeen)
	
	// 计算稳定性评分 (0-1)
	stabilityScore := 0.0
	
	// 1. 时间稳定性 (40%权重)
	timeScore := math.Min(existenceDuration.Minutes()/30, 1.0) // 30分钟为满分
	stabilityScore += timeScore * 0.4
	
	// 2. 闪烁频率 (30%权重)
	flickerRatio := float64(entry.disappearances) / float64(entry.appearances)
	flickerScore := math.Max(0, 1.0-flickerRatio) // 闪烁越少评分越高
	stabilityScore += flickerScore * 0.3
	
	// 3. 大小一致性 (20%权重)
	sizeConsistency := 0.0
	if entry.maxSize > 0 {
		sizeVariation := (entry.maxSize - entry.minSize) / entry.maxSize
		sizeConsistency = math.Max(0, 1.0-sizeVariation) // 变化越小评分越高
	}
	stabilityScore += sizeConsistency * 0.2
	
	// 4. 当前活跃性 (10%权重)
	activeScore := 0.0
	if entry.isActive {
		activeScore = 1.0
	}
	stabilityScore += activeScore * 0.1
	
	// 确保评分在0-1范围内
	stabilityScore = math.Max(0, math.Min(1, stabilityScore))
	
	return stabilityScore, entry.disappearances, existenceDuration
}

// calculateImbalanceTrend 计算失衡趋势方向（V2.0）
func (calc *OrderBookCalculator) calculateImbalanceTrend(symbol string) string {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil || len(symbolData.imbalanceHistory) < 3 {
		return "insufficient_data"
	}
	
	// 取最近3个点计算趋势
	recentPoints := symbolData.imbalanceHistory[len(symbolData.imbalanceHistory)-3:]
	
	// 计算斜率
	slope := (recentPoints[2].Ratio - recentPoints[0].Ratio) / 2
	
	if slope > 0.05 {
		return "increasing_bid_pressure" // 买方压力增加
	} else if slope < -0.05 {
		return "increasing_ask_pressure" // 卖方压力增加
	} else {
		return "stable" // 压力平衡
	}
}

// calculatePressureDelta5m 计算5分钟压力变化（V2.0）
func (calc *OrderBookCalculator) calculatePressureDelta5m(symbol string, currentBidPressure, currentAskPressure float64) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return 0
	}
	
	// 简化实现：基于当前失衡比例计算压力差异
	currentImbalance := calc.calculateImbalance(symbol)
	
	if len(symbolData.imbalanceHistory) < 2 {
		return 0
	}
	
	// 计算与5分钟前的压力差异
	fiveMinuteAgo := time.Now().Add(-5 * time.Minute)
	var baseline float64
	
	for i := len(symbolData.imbalanceHistory) - 1; i >= 0; i-- {
		if symbolData.imbalanceHistory[i].Timestamp.Before(fiveMinuteAgo) {
			baseline = symbolData.imbalanceHistory[i].Ratio
			break
		}
	}
	
	return currentImbalance - baseline
}

// calculateSpoofingRisk 计算虚假挂单风险评分（V2.0）
func (calc *OrderBookCalculator) calculateSpoofingRisk(symbol string) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return 0
	}
	
	riskScore := 0.0
	
	// 检查墙的闪烁频率
	totalFlickers := 0
	totalWalls := 0
	
	for _, entry := range symbolData.wallTracker.wallHistory {
		if entry.isActive {
			totalWalls++
			// 如果闪烁次数超过阈值，增加风险评分
			if entry.disappearances > symbolData.wallTracker.flickerThreshold {
				totalFlickers++
				riskScore += float64(entry.disappearances) / 10.0
			}
		}
	}
	
	// 如果5分钟内墙变化过于频繁
	if symbolData.wallChangeCount5m > 10 {
		riskScore += float64(symbolData.wallChangeCount5m) / 50.0
	}
	
	// 标准化风险评分到0-1范围
	riskScore = math.Min(1.0, riskScore)
	
	return riskScore
}

// calculateLiquidityScore 计算流动性评分（V2.0）
func (calc *OrderBookCalculator) calculateLiquidityScore(symbol string) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil || len(symbolData.currentBids) == 0 || len(symbolData.currentAsks) == 0 {
		return 0
	}
	
	liquidityScore := 0.0
	
	// 1. 档位深度评分 (40%权重)
	depthScore := math.Min(float64(len(symbolData.currentBids)+len(symbolData.currentAsks))/40.0, 1.0)
	liquidityScore += depthScore * 0.4
	
	// 2. 价差评分 (30%权重)
	if len(symbolData.currentBids) > 0 && len(symbolData.currentAsks) > 0 {
		bestBid := symbolData.currentBids[0].Price
		bestAsk := symbolData.currentAsks[0].Price
		spread := (bestAsk - bestBid) / bestBid
		spreadScore := math.Max(0, 1.0-spread*1000) // 价差越小评分越高
		liquidityScore += spreadScore * 0.3
	}
	
	// 3. 总量评分 (30%权重)
	totalBidValue := 0.0
	totalAskValue := 0.0
	for _, bid := range symbolData.currentBids {
		totalBidValue += bid.Price * bid.Quantity
	}
	for _, ask := range symbolData.currentAsks {
		totalAskValue += ask.Price * ask.Quantity
	}
	
	totalLiquidity := totalBidValue + totalAskValue
	// 假设1000万USD为高流动性基准
	volumeScore := math.Min(totalLiquidity/10000000.0, 1.0)
	liquidityScore += volumeScore * 0.3
	
	return liquidityScore
}

// getWallFirstSeen 获取墙���首次出现时间
func (calc *OrderBookCalculator) getWallFirstSeen(symbol string, price float64) time.Time {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return time.Now()
	}
	
	if entry, exists := symbolData.wallTracker.wallHistory[price]; exists {
		return entry.firstSeen
	}
	return time.Now() // 如果没有历史记录，返回当前时间
}

// getWallAverageSize 获取墙的平均大小
func (calc *OrderBookCalculator) getWallAverageSize(symbol string, price float64) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return 0
	}
	
	if entry, exists := symbolData.wallTracker.wallHistory[price]; exists {
		return entry.avgSize
	}
	return 0
}

// getWallMaxSize 获取墙的最大历史大小
func (calc *OrderBookCalculator) getWallMaxSize(symbol string, price float64) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return 0
	}
	
	if entry, exists := symbolData.wallTracker.wallHistory[price]; exists {
		return entry.maxSize
	}
	return 0
}

// getWallMinSize 获取墙的最小历史大小
func (calc *OrderBookCalculator) getWallMinSize(symbol string, price float64) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return 0
	}
	
	if entry, exists := symbolData.wallTracker.wallHistory[price]; exists {
		return entry.minSize
	}
	return 0
}
