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

// OrderBookCalculator 盘口计算器
type OrderBookCalculator struct {
	mu               sync.RWMutex
	symbol           string
	currentBids      []OrderBookLevel // 当前买单档位
	currentAsks      []OrderBookLevel // 当前卖单档位
	lastUpdate       time.Time
	wallThreshold    float64          // 挂单墙阈值倍数（默认5倍平均档位量）
	imbalanceHistory []ImbalancePoint // 失衡历史（用于平滑）
	maxHistorySize   int              // 最大历史记录数量
	currentPrice     float64          // 当前价格（用于计算距离）
	
	// V2.0 稳定性追踪
	wallTracker      *WallTracker     // 挂单墙追踪器
	wallChangeCount5m int             // 5分钟内墙变化次数
	last5mReset      time.Time        // 最后5分钟重置时间
}

// NewOrderBookCalculator 创建盘口计算器
func NewOrderBookCalculator(symbol string, wallThreshold float64) *OrderBookCalculator {
	return &OrderBookCalculator{
		symbol:           symbol,
		currentBids:      make([]OrderBookLevel, 0, 20),
		currentAsks:      make([]OrderBookLevel, 0, 20),
		wallThreshold:    wallThreshold,
		imbalanceHistory: make([]ImbalancePoint, 0, 100),
		maxHistorySize:   100,
		lastUpdate:       time.Now(),
		wallTracker: &WallTracker{
			wallHistory:     make(map[float64]*WallHistoryEntry),
			maxWallAge:      30 * time.Minute, // 墙最多追踪30分钟
			flickerThreshold: 3,                // 超过3次闪烁认为是spoofing
		},
		last5mReset: time.Now(),
	}
}

// ProcessDepthData 处理盘口数据更新
func (calc *OrderBookCalculator) ProcessDepthData(depthData *DepthData) {
	calc.mu.Lock()
	defer calc.mu.Unlock()

	// 更新盘口数据
	calc.currentBids = make([]OrderBookLevel, len(depthData.Bids))
	copy(calc.currentBids, depthData.Bids)

	calc.currentAsks = make([]OrderBookLevel, len(depthData.Asks))
	copy(calc.currentAsks, depthData.Asks)

	calc.lastUpdate = depthData.Timestamp

	// 更新当前价格（使用盘口中间价）
	if len(calc.currentBids) > 0 && len(calc.currentAsks) > 0 {
		bestBid := calc.currentBids[0].Price
		bestAsk := calc.currentAsks[0].Price
		calc.currentPrice = (bestBid + bestAsk) / 2
	}

	// V2.0: 检查5分钟周期重置
	if depthData.Timestamp.Sub(calc.last5mReset) >= 5*time.Minute {
		calc.wallChangeCount5m = 0
		calc.last5mReset = depthData.Timestamp
	}

	// V2.0: 更新墙追踪
	calc.updateWallTracking(depthData.Timestamp)

	// 计算并记录失衡比例
	imbalanceRatio := calc.calculateImbalance()
	calc.addImbalancePoint(ImbalancePoint{
		Timestamp: depthData.Timestamp,
		Ratio:     imbalanceRatio,
	})
}

// calculateImbalance 计算买卖失衡比例
func (calc *OrderBookCalculator) calculateImbalance() float64 {
	if len(calc.currentBids) == 0 || len(calc.currentAsks) == 0 {
		return 0
	}

	// 计算买单总价值（前10档）
	var totalBidValue float64
	maxLevels := int(math.Min(float64(len(calc.currentBids)), 10))
	for i := 0; i < maxLevels; i++ {
		bid := calc.currentBids[i]
		totalBidValue += bid.Price * bid.Quantity
	}

	// 计算卖单总价值（前10档）
	var totalAskValue float64
	maxLevels = int(math.Min(float64(len(calc.currentAsks)), 10))
	for i := 0; i < maxLevels; i++ {
		ask := calc.currentAsks[i]
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
func (calc *OrderBookCalculator) addImbalancePoint(point ImbalancePoint) {
	calc.imbalanceHistory = append(calc.imbalanceHistory, point)

	// 限制历史记录大小
	if len(calc.imbalanceHistory) > calc.maxHistorySize {
		calc.imbalanceHistory = calc.imbalanceHistory[len(calc.imbalanceHistory)-calc.maxHistorySize:]
	}
}

// findWalls 识别挂单墙
func (calc *OrderBookCalculator) findWalls() (resistance *WallInfo, support *WallInfo) {
	// 计算平均档位量
	avgBidValue := calc.calculateAverageLevel(calc.currentBids)
	avgAskValue := calc.calculateAverageLevel(calc.currentAsks)
	avgValue := (avgBidValue + avgAskValue) / 2

	if avgValue == 0 {
		return nil, nil
	}

	threshold := avgValue * calc.wallThreshold

	// 查找阻力墙（卖单墙）
	resistance = calc.findResistanceWall(threshold)

	// 查找支撑墙（买单墙）
	support = calc.findSupportWall(threshold)

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
func (calc *OrderBookCalculator) findResistanceWall(threshold float64) *WallInfo {
	if len(calc.currentAsks) == 0 {
		return nil
	}

	var wallPrice float64
	var wallStrength float64
	var wallLevels int

	// 查找连续的大档位
	for i, ask := range calc.currentAsks {
		levelValue := ask.Price * ask.Quantity

		if levelValue > threshold {
			if wallPrice == 0 {
				wallPrice = ask.Price
				wallStrength = levelValue
				wallLevels = 1
			} else {
				// 检查是否是连续的墙
				if i > 0 {
					prevPrice := calc.currentAsks[i-1].Price
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
	if calc.currentPrice > 0 {
		distance = (wallPrice - calc.currentPrice) / calc.currentPrice * 100
	}

	// V2.0: 计算稳定性评分
	stabilityScore, flickerCount, existenceDuration := calc.calculateWallStability(wallPrice, wallStrength)

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
		LastSeen:         calc.lastUpdate,
		FirstSeen:        calc.getWallFirstSeen(wallPrice),
		AverageSize:      calc.getWallAverageSize(wallPrice),
		MaxSize:          calc.getWallMaxSize(wallPrice),
		MinSize:          calc.getWallMinSize(wallPrice),
	}
}

// findSupportWall 查找支撑墙
func (calc *OrderBookCalculator) findSupportWall(threshold float64) *WallInfo {
	if len(calc.currentBids) == 0 {
		return nil
	}

	var wallPrice float64
	var wallStrength float64
	var wallLevels int

	// 查找连续的大档位（买单从高到低）
	for i, bid := range calc.currentBids {
		levelValue := bid.Price * bid.Quantity

		if levelValue > threshold {
			if wallPrice == 0 {
				wallPrice = bid.Price
				wallStrength = levelValue
				wallLevels = 1
			} else {
				// 检查是否是连续的墙
				if i > 0 {
					prevPrice := calc.currentBids[i-1].Price
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
	if calc.currentPrice > 0 {
		distance = (calc.currentPrice - wallPrice) / calc.currentPrice * 100
	}

	// V2.0: 计算稳定性评分
	stabilityScore, flickerCount, existenceDuration := calc.calculateWallStability(wallPrice, wallStrength)

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
		LastSeen:         calc.lastUpdate,
		FirstSeen:        calc.getWallFirstSeen(wallPrice),
		AverageSize:      calc.getWallAverageSize(wallPrice),
		MaxSize:          calc.getWallMaxSize(wallPrice),
		MinSize:          calc.getWallMinSize(wallPrice),
	}
}

// getSmoothedImbalance 获取平滑后的失衡比例
func (calc *OrderBookCalculator) getSmoothedImbalance(smoothPeriods int) float64 {
	if len(calc.imbalanceHistory) == 0 {
		return 0
	}

	// 取最近N个点计算平均值
	startIdx := len(calc.imbalanceHistory) - smoothPeriods
	if startIdx < 0 {
		startIdx = 0
	}

	var sum float64
	count := 0
	for i := startIdx; i < len(calc.imbalanceHistory); i++ {
		sum += calc.imbalanceHistory[i].Ratio
		count++
	}

	if count == 0 {
		return 0
	}

	return sum / float64(count)
}

// GetCurrentOrderBookData 获取当前盘口分析数据（V2.0 - 集成缓存）
func (calc *OrderBookCalculator) GetCurrentOrderBookData(smoothPeriods int) *OrderBookData {
	calc.mu.RLock()
	defer calc.mu.RUnlock()

	// 计算失衡比例（平滑）
	imbalanceRatio := calc.getSmoothedImbalance(smoothPeriods)

	// 识别挂单墙
	resistance, support := calc.findWalls()

	// 计算买卖压力
	bidPressure := calc.calculatePressure(calc.currentBids)
	askPressure := calc.calculatePressure(calc.currentAsks)

	// 检查数据是否过期
	isStale := time.Since(calc.lastUpdate) > 5*time.Minute

	// V2.0: 计算额外的市场微观结构指标
	imbalanceTrend := calc.calculateImbalanceTrend()
	pressureDelta5m := calc.calculatePressureDelta5m(bidPressure, askPressure)
	spoofingRisk := calc.calculateSpoofingRisk()
	liquidityScore := calc.calculateLiquidityScore()

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
		WallChangeCount5m:   calc.wallChangeCount5m,
		LastUpdate:          calc.lastUpdate,
		IsStale:             isStale,
	}

	// V2.0: 缓存到全局缓存系统
	globalCache := GetGlobalCache()
	globalCache.SetOrderBookData(calc.symbol, orderBookData)

	log.Printf("📊 [%s] 盘口数据已更新并缓存: 失衡比%.3f, 虚假挂单风险%.3f, 流动性评分%.3f",
		calc.symbol, imbalanceRatio, spoofingRisk, liquidityScore)

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
func (calc *OrderBookCalculator) updateWallTracking(timestamp time.Time) {
	// 获取当前的挂单墙
	resistance, support := calc.findWalls()
	
	// 更新阻力墙追踪
	if resistance != nil {
		calc.trackWall(resistance.Price, resistance.StrengthUSD, timestamp)
	}
	
	// 更新支撑墙追踪
	if support != nil {
		calc.trackWall(support.Price, support.StrengthUSD, timestamp)
	}
	
	// 清理过期的墙记录
	calc.cleanupExpiredWalls(timestamp)
}

// trackWall 追踪单个墙的变化
func (calc *OrderBookCalculator) trackWall(price, size float64, timestamp time.Time) {
	entry, exists := calc.wallTracker.wallHistory[price]
	
	if !exists {
		// 新墙
		calc.wallTracker.wallHistory[price] = &WallHistoryEntry{
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
		calc.wallChangeCount5m++
	} else {
		// 更新现有墙
		entry.lastSeen = timestamp
		
		if !entry.isActive {
			// 墙重新出现
			entry.appearances++
			entry.isActive = true
			calc.wallChangeCount5m++
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
func (calc *OrderBookCalculator) cleanupExpiredWalls(timestamp time.Time) {
	cutoffTime := timestamp.Add(-calc.wallTracker.maxWallAge)
	
	for price, entry := range calc.wallTracker.wallHistory {
		if entry.lastSeen.Before(cutoffTime) {
			delete(calc.wallTracker.wallHistory, price)
		} else if entry.isActive {
			// 检查墙是否已消失（在当前盘口中不存在）
			if !calc.isWallCurrentlyPresent(price) {
				entry.isActive = false
				entry.disappearances++
				calc.wallChangeCount5m++
			}
		}
	}
}

// isWallCurrentlyPresent 检查指定价格的墙是否在当前盘口中存在
func (calc *OrderBookCalculator) isWallCurrentlyPresent(price float64) bool {
	// 计算阈值
	avgBidValue := calc.calculateAverageLevel(calc.currentBids)
	avgAskValue := calc.calculateAverageLevel(calc.currentAsks)
	avgValue := (avgBidValue + avgAskValue) / 2
	
	if avgValue == 0 {
		return false
	}
	
	threshold := avgValue * calc.wallThreshold
	
	// 检查买单档位
	for _, bid := range calc.currentBids {
		if math.Abs(bid.Price-price) < 0.0001 { // 价格匹配
			levelValue := bid.Price * bid.Quantity
			return levelValue > threshold
		}
	}
	
	// 检查卖单档位
	for _, ask := range calc.currentAsks {
		if math.Abs(ask.Price-price) < 0.0001 { // 价格匹配
			levelValue := ask.Price * ask.Quantity
			return levelValue > threshold
		}
	}
	
	return false
}

// calculateWallStability 计算墙稳定性评分（V2.0核心功能）
func (calc *OrderBookCalculator) calculateWallStability(price, currentSize float64) (float64, int, time.Duration) {
	entry, exists := calc.wallTracker.wallHistory[price]
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
func (calc *OrderBookCalculator) calculateImbalanceTrend() string {
	if len(calc.imbalanceHistory) < 3 {
		return "insufficient_data"
	}
	
	// 取最近3个点计算趋势
	recentPoints := calc.imbalanceHistory[len(calc.imbalanceHistory)-3:]
	
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
func (calc *OrderBookCalculator) calculatePressureDelta5m(currentBidPressure, currentAskPressure float64) float64 {
	// 简化实现：基于当前失衡比例计算压力差异
	currentImbalance := calc.calculateImbalance()
	
	if len(calc.imbalanceHistory) < 2 {
		return 0
	}
	
	// 计算与5分钟前的压力差异
	fiveMinuteAgo := time.Now().Add(-5 * time.Minute)
	var baseline float64
	
	for i := len(calc.imbalanceHistory) - 1; i >= 0; i-- {
		if calc.imbalanceHistory[i].Timestamp.Before(fiveMinuteAgo) {
			baseline = calc.imbalanceHistory[i].Ratio
			break
		}
	}
	
	return currentImbalance - baseline
}

// calculateSpoofingRisk 计算虚假挂单风险评分（V2.0）
func (calc *OrderBookCalculator) calculateSpoofingRisk() float64 {
	riskScore := 0.0
	
	// 检查墙的闪烁频率
	totalFlickers := 0
	totalWalls := 0
	
	for _, entry := range calc.wallTracker.wallHistory {
		if entry.isActive {
			totalWalls++
			// 如果闪烁次数超过阈值，增加风险评分
			if entry.disappearances > calc.wallTracker.flickerThreshold {
				totalFlickers++
				riskScore += float64(entry.disappearances) / 10.0
			}
		}
	}
	
	// 如果5分钟内墙变化过于频繁
	if calc.wallChangeCount5m > 10 {
		riskScore += float64(calc.wallChangeCount5m) / 50.0
	}
	
	// 标准化风险评分到0-1范围
	riskScore = math.Min(1.0, riskScore)
	
	return riskScore
}

// calculateLiquidityScore 计算流动性评分（V2.0）
func (calc *OrderBookCalculator) calculateLiquidityScore() float64 {
	if len(calc.currentBids) == 0 || len(calc.currentAsks) == 0 {
		return 0
	}
	
	liquidityScore := 0.0
	
	// 1. 档位深度评分 (40%权重)
	depthScore := math.Min(float64(len(calc.currentBids)+len(calc.currentAsks))/40.0, 1.0)
	liquidityScore += depthScore * 0.4
	
	// 2. 价差评分 (30%权重)
	if len(calc.currentBids) > 0 && len(calc.currentAsks) > 0 {
		bestBid := calc.currentBids[0].Price
		bestAsk := calc.currentAsks[0].Price
		spread := (bestAsk - bestBid) / bestBid
		spreadScore := math.Max(0, 1.0-spread*1000) // 价差越小评分越高
		liquidityScore += spreadScore * 0.3
	}
	
	// 3. 总量评分 (30%权重)
	totalBidValue := 0.0
	totalAskValue := 0.0
	for _, bid := range calc.currentBids {
		totalBidValue += bid.Price * bid.Quantity
	}
	for _, ask := range calc.currentAsks {
		totalAskValue += ask.Price * ask.Quantity
	}
	
	totalLiquidity := totalBidValue + totalAskValue
	// 假设1000万USD为高流动性基准
	volumeScore := math.Min(totalLiquidity/10000000.0, 1.0)
	liquidityScore += volumeScore * 0.3
	
	return liquidityScore
}

// getWallFirstSeen 获取墙���首次出现时间
func (calc *OrderBookCalculator) getWallFirstSeen(price float64) time.Time {
	if entry, exists := calc.wallTracker.wallHistory[price]; exists {
		return entry.firstSeen
	}
	return time.Now() // 如果没有历史记录，返回当前时间
}

// getWallAverageSize 获取墙的平均大小
func (calc *OrderBookCalculator) getWallAverageSize(price float64) float64 {
	if entry, exists := calc.wallTracker.wallHistory[price]; exists {
		return entry.avgSize
	}
	return 0
}

// getWallMaxSize 获取墙的最大历史大小
func (calc *OrderBookCalculator) getWallMaxSize(price float64) float64 {
	if entry, exists := calc.wallTracker.wallHistory[price]; exists {
		return entry.maxSize
	}
	return 0
}

// getWallMinSize 获取墙的最小历史大小
func (calc *OrderBookCalculator) getWallMinSize(price float64) float64 {
	if entry, exists := calc.wallTracker.wallHistory[price]; exists {
		return entry.minSize
	}
	return 0
}
