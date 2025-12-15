package microstructure

import (
	"log"
	"math"
	"os"
	"sort"
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
	wallHistory      map[int64]*WallHistoryEntry // 🔧 修复：使用分桶价格作为Key，避免浮点数陷阱
	maxWallAge       time.Duration               // 墙的最大存活时间
	flickerThreshold int                         // 闪烁阈值次数
	tickSize         float64                     // 价格最小变动单位（用于分桶）
}

// WallHistoryEntry 挂单墙历史记录
type WallHistoryEntry struct {
	bucketKey      int64      // 🔧 新增：分桶key，容许价格范围内的微调
	priceRange     [2]float64 // 🔧 新增：价格范围 [min, max]
	firstSeen      time.Time
	lastSeen       time.Time
	appearances    int       // 出现次数
	disappearances int       // 消失次数
	maxSize        float64   // 历史最大规模
	minSize        float64   // 历史最小规模
	avgSize        float64   // 平均规模
	sizeHistory    []float64 // 规模历史
	isActive       bool      // 当前是否活跃
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

// PressureCalculationMode 压力计算模式
type PressureCalculationMode int

const (
	PressureModeSimple   PressureCalculationMode = iota // 简单模式：前5档
	PressureModeDeep                                    // 深度模式：前20档
	PressureModeWeighted                                // 加权模式：距离加权
	PressureModeFull                                    // 全深度模式：所有档位
)

// OrderBookCalculator 盘口计算器（支持多交易对）
type OrderBookCalculator struct {
	mu             sync.RWMutex
	symbolData     map[string]*SymbolOrderBookData // symbol -> 数据
	wallThreshold  float64                         // 挂单墙阈值倍数（默认5倍平均档位量）
	maxHistorySize int                             // 最大历史记录数量
	pressureMode   PressureCalculationMode         // 🔧 P2-1新增：压力计算模式
}

// NewOrderBookCalculator 创建盘口计算器（支持多交易对）
func NewOrderBookCalculator(wallThreshold float64) *OrderBookCalculator {
	return &OrderBookCalculator{
		symbolData:     make(map[string]*SymbolOrderBookData),
		wallThreshold:  wallThreshold,
		maxHistorySize: 100,
		pressureMode:   PressureModeDeep, // 🔧 P2-1修复：默认使用深度模式，比简单模式更准确
	}
}

// NewOrderBookCalculatorWithMode 创建带模式的盘口计算器（🔧 P2-1新增）
func NewOrderBookCalculatorWithMode(wallThreshold float64, mode PressureCalculationMode) *OrderBookCalculator {
	return &OrderBookCalculator{
		symbolData:     make(map[string]*SymbolOrderBookData),
		wallThreshold:  wallThreshold,
		maxHistorySize: 100,
		pressureMode:   mode,
	}
}

// SetPressureMode 设置压力计算模式（🔧 P2-1新增）
func (calc *OrderBookCalculator) SetPressureMode(mode PressureCalculationMode) {
	calc.mu.Lock()
	defer calc.mu.Unlock()
	calc.pressureMode = mode
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
			wallHistory:      make(map[int64]*WallHistoryEntry),
			maxWallAge:       30 * time.Minute,
			flickerThreshold: 3,
			tickSize:         calc.calculateTickSize(symbol), // 🔧 动态计算分桶大小
		},
		last5mReset: time.Now(),
	}

	calc.symbolData[symbol] = data
	return data
}

// calculateTickSize 动态计算价格分桶大小
func (calc *OrderBookCalculator) calculateTickSize(symbol string) float64 {
	// 🔧 修复：基于币种动态计算分桶大小，解决浮点数陷阱
	if symbol == "BTCUSDT" {
		return 10.0 // BTC: 10USD为一个分桶
	} else if symbol == "ETHUSDT" {
		return 5.0 // ETH: 5USD为一个分桶
	} else {
		return 0.1 // 其他币种: 0.1USD为一个分桶
	}
}

// priceToBucket 将价格转换为分桶Key
func (calc *OrderBookCalculator) priceToBucket(price float64, tickSize float64) int64 {
	// 🔧 修复：将相近价格归入同一个分桶，容忍价格微调
	return int64(price / tickSize)
}

// bucketToPrice 将分桶Key转换回价格范围
func (calc *OrderBookCalculator) bucketToPrice(bucketKey int64, tickSize float64) (float64, float64) {
	// 返回分桶的价格范围 [min, max]
	minPrice := float64(bucketKey) * tickSize
	maxPrice := minPrice + tickSize
	return minPrice, maxPrice
}

// ProcessDepthData 处理盘口数据更新（V-12.3 P0-03修复版本）
func (calc *OrderBookCalculator) ProcessDepthData(symbol string, depthData *DepthData) {
	calc.mu.Lock()
	defer calc.mu.Unlock()

	// 获取或创建交易对数据
	symbolData := calc.getOrCreateSymbolData(symbol)

	// 🔥 P0-03修复：强制排序和同价位合并，确保数据正确性
	processedBids := calc.processAndSortOrderBookLevels(depthData.Bids, "bids")
	processedAsks := calc.processAndSortOrderBookLevels(depthData.Asks, "asks")

	// 更新盘口数据（使用处理后的数据）
	symbolData.currentBids = processedBids
	symbolData.currentAsks = processedAsks
	symbolData.lastUpdate = depthData.Timestamp

	// 🔥 P0-03修复：重新验证最优价位，确保数据一致性
	if len(symbolData.currentBids) > 0 && len(symbolData.currentAsks) > 0 {
		bestBid := symbolData.currentBids[0].Price
		bestAsk := symbolData.currentAsks[0].Price

		// 双重检查：确保没有盘口交叉
		if bestBid >= bestAsk {
			log.Printf("🚨 [%s] 处理后仍然盘口交叉: bestBid=%.8f >= bestAsk=%.8f, 跳过此次更新",
				symbol, bestBid, bestAsk)
			return
		}

		symbolData.currentPrice = (bestBid + bestAsk) / 2

		// 🔥 P0-03修复：记录盘口健康度数据
		calc.recordOrderBookHealth(symbol, bestBid, bestAsk, len(processedBids), len(processedAsks))
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

// processAndSortOrderBookLevels 处理和排序订单簿档位（V-12.3 P0-03修复版本）
// 🔥 P0-03修复：强制排序、同价位合并、确保数据正确性
func (calc *OrderBookCalculator) processAndSortOrderBookLevels(levels []OrderBookLevel, side string) []OrderBookLevel {
	if len(levels) == 0 {
		return levels
	}

	// 第一步：使用map合并同价位数量
	priceMap := make(map[float64]float64)

	for _, level := range levels {
		// 跳过无效数据
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}

		// 同价位累加数量
		priceMap[level.Price] += level.Quantity
	}

	// 第二步：转换回slice
	result := make([]OrderBookLevel, 0, len(priceMap))
	for price, totalQty := range priceMap {
		if totalQty > 0 { // 确保合并后数量仍为正
			result = append(result, OrderBookLevel{
				Price:    price,
				Quantity: totalQty,
			})
		}
	}

	// 第三步：强制排序
	if side == "bids" {
		// 买单按价格降序排列（最高价在前）
		sort.Slice(result, func(i, j int) bool {
			return result[i].Price > result[j].Price
		})
	} else {
		// 卖单按价格升序排列（最低价在前）
		sort.Slice(result, func(i, j int) bool {
			return result[i].Price < result[j].Price
		})
	}

	return result
}

// recordOrderBookHealth 记录订单簿健康度（V-12.3 P0-03修复版本）
// 🔥 P0-03修复：新增盘口质量监控
func (calc *OrderBookCalculator) recordOrderBookHealth(symbol string, bestBid, bestAsk float64, bidLevels, askLevels int) {
	spread := bestAsk - bestBid
	spreadPct := (spread / bestBid) * 100

	// 记录关键指标用于后续分析
	if spreadPct > 1.0 { // 价差超过1%时记录
		log.Printf("⚠️ [%s] 价差较宽: %.6f%% (%.8f), 档位数: bid=%d, ask=%d",
			symbol, spreadPct, spread, bidLevels, askLevels)
	}

	// 检查档位深度是否足够
	if bidLevels < 5 || askLevels < 5 {
		log.Printf("⚠️ [%s] 盘口深度不足: 买单%d档, 卖单%d档", symbol, bidLevels, askLevels)
	}
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
		// 🔧 修复: 防止无效的价格和数量数据
		if bid.Price <= 0 || bid.Quantity <= 0 {
			continue
		}
		totalBidValue += bid.Price * bid.Quantity
	}

	// 计算卖单总价值（前10档）
	var totalAskValue float64
	maxLevels = int(math.Min(float64(len(symbolData.currentAsks)), 10))
	for i := 0; i < maxLevels; i++ {
		ask := symbolData.currentAsks[i]
		// 🔧 修复: 防止无效的价格和数量数据
		if ask.Price <= 0 || ask.Quantity <= 0 {
			continue
		}
		totalAskValue += ask.Price * ask.Quantity
	}

	// 🔧 修复: 增强除零保护和边界情况处理
	totalValue := totalBidValue + totalAskValue
	if totalValue <= 0 || math.IsNaN(totalValue) || math.IsInf(totalValue, 0) {
		log.Printf("⚠️ [%s] 计算失衡比例时发现异常总值: %.2f (买单:%.2f, 卖单:%.2f)",
			symbol, totalValue, totalBidValue, totalAskValue)
		return 0
	}

	imbalance := (totalBidValue - totalAskValue) / totalValue

	// 🔧 修复: 确保返回值在有效范围内
	if math.IsNaN(imbalance) || math.IsInf(imbalance, 0) {
		log.Printf("⚠️ [%s] 计算失衡比例结果异常: %.6f", symbol, imbalance)
		return 0
	}

	return imbalance
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
	validLevels := 0

	for _, level := range levels {
		// 🔧 修复: 跳过无效的档位数据
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}

		levelValue := level.Price * level.Quantity
		// 🔧 修复: 检查计算结果是否有效
		if math.IsNaN(levelValue) || math.IsInf(levelValue, 0) {
			continue
		}

		totalValue += levelValue
		validLevels++
	}

	// 🔧 修复: 确保有有效档位数据
	if validLevels == 0 || totalValue <= 0 {
		return 0
	}

	return totalValue / float64(validLevels)
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
		FlickerCount:      flickerCount,
		ExistenceDuration: existenceDuration,
		LastSeen:          symbolData.lastUpdate,
		FirstSeen:         calc.getWallFirstSeen(symbol, wallPrice),
		AverageSize:       calc.getWallAverageSize(symbol, wallPrice),
		MaxSize:           calc.getWallMaxSize(symbol, wallPrice),
		MinSize:           calc.getWallMinSize(symbol, wallPrice),
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
		FlickerCount:      flickerCount,
		ExistenceDuration: existenceDuration,
		LastSeen:          symbolData.lastUpdate,
		FirstSeen:         calc.getWallFirstSeen(symbol, wallPrice),
		AverageSize:       calc.getWallAverageSize(symbol, wallPrice),
		MaxSize:           calc.getWallMaxSize(symbol, wallPrice),
		MinSize:           calc.getWallMinSize(symbol, wallPrice),
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

	// 计算买卖压力（使用新的压力计算方法）
	bidPressure := calc.calculatePressureEnhanced(symbol, symbolData.currentBids, true)
	askPressure := calc.calculatePressureEnhanced(symbol, symbolData.currentAsks, false)

	// 检查数据是否过期 - 🔧 修复：使用更合理的过期判断，盘口数据应该更严格
	// 盘口数据是实时的，5分钟无更新就应该标记为过期
	isStale := time.Since(symbolData.lastUpdate) > 5*time.Minute

	// V2.0: 计算额外的市场微观结构指标
	imbalanceTrend := calc.calculateImbalanceTrend(symbol)
	// 🔥 P0-04修复：使用symbolData.lastUpdate而非time.Now()
	pressureDelta5m := calc.calculatePressureDelta5mFixed(symbol, bidPressure, askPressure, symbolData.lastUpdate)
	spoofingRisk := calc.calculateSpoofingRisk(symbol)
	liquidityScore := calc.calculateLiquidityScore(symbol)

	orderBookData := &OrderBookData{
		ImbalanceRatio:    imbalanceRatio,
		NearestResistance: resistance,
		NearestSupport:    support,
		BidPressure:       bidPressure,
		AskPressure:       askPressure,
		// V2.0 新增字段
		ImbalanceTrend:    imbalanceTrend,
		PressureDelta5m:   pressureDelta5m,
		SpoofingRisk:      spoofingRisk,
		LiquidityScore:    liquidityScore,
		WallChangeCount5m: symbolData.wallChangeCount5m,
		LastUpdate:        symbolData.lastUpdate,
		IsStale:           isStale,
	}

	// V2.0: 缓存到全局缓存系统
	globalCache := GetGlobalCache()
	globalCache.SetOrderBookData(symbol, orderBookData)

	// 只在异常情况或调试模式下打印详细日志，避免日志噪音
	if spoofingRisk > 0.8 || math.Abs(imbalanceRatio) > 0.9 || liquidityScore < 0.5 {
		//log.Printf("⚠️ [%s] 盘口异常: 失衡比%.3f, 虚假挂单风险%.3f, 流动性评分%.3f",
		//	symbol, imbalanceRatio, spoofingRisk, liquidityScore)
	}

	return orderBookData
}

// calculatePressure 计算买卖压力强度（保持向后兼容）
func (calc *OrderBookCalculator) calculatePressure(levels []OrderBookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}

	// 保持原有逻辑：计算前5档的总价值
	var totalValue float64
	maxLevels := int(math.Min(float64(len(levels)), 5))

	for i := 0; i < maxLevels; i++ {
		level := levels[i]
		totalValue += level.Price * level.Quantity
	}

	return totalValue
}

// calculatePressureEnhanced 🔧 P2-1新增：增强版压力计算，支持多种模式
func (calc *OrderBookCalculator) calculatePressureEnhanced(symbol string, levels []OrderBookLevel, isBid bool) float64 {
	if len(levels) == 0 {
		return 0
	}

	// 根据配置的压力计算模式选择算法
	switch calc.pressureMode {
	case PressureModeSimple:
		return calc.calculatePressureSimple(levels)
	case PressureModeDeep:
		return calc.calculatePressureDeep(levels)
	case PressureModeWeighted:
		return calc.calculatePressureWeighted(symbol, levels, isBid)
	case PressureModeFull:
		return calc.calculatePressureFull(levels)
	default:
		return calc.calculatePressureSimple(levels)
	}
}

// calculatePressureSimple 简单模式：前5档（原有逻辑）
func (calc *OrderBookCalculator) calculatePressureSimple(levels []OrderBookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}

	var totalValue float64
	maxLevels := int(math.Min(float64(len(levels)), 5))

	for i := 0; i < maxLevels; i++ {
		level := levels[i]
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}
		totalValue += level.Price * level.Quantity
	}

	return totalValue
}

// calculatePressureDeep 🔧 P2-1新增：深度模式：前20档
func (calc *OrderBookCalculator) calculatePressureDeep(levels []OrderBookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}

	var totalValue float64
	maxLevels := int(math.Min(float64(len(levels)), 20)) // 扩展到20档

	for i := 0; i < maxLevels; i++ {
		level := levels[i]
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}
		totalValue += level.Price * level.Quantity
	}

	return totalValue
}

// calculatePressureWeighted 🔧 P2-1新增：加权模式：距离加权计算
func (calc *OrderBookCalculator) calculatePressureWeighted(symbol string, levels []OrderBookLevel, isBid bool) float64 {
	if len(levels) == 0 {
		return 0
	}

	symbolData := calc.symbolData[symbol]
	if symbolData == nil || symbolData.currentPrice <= 0 {
		// 兜底：如果没有当前价格，使用深度模式
		return calc.calculatePressureDeep(levels)
	}

	currentPrice := symbolData.currentPrice
	var weightedValue float64
	maxLevels := int(math.Min(float64(len(levels)), 20)) // 加权计算前20档

	for i := 0; i < maxLevels; i++ {
		level := levels[i]
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}

		// 计算距离权重：距离越近权重越高
		var distance float64
		if isBid {
			// 买单：距离 = |当前价 - 买价| / 当前价
			distance = math.Abs(currentPrice-level.Price) / currentPrice
		} else {
			// 卖单：距离 = |卖价 - 当前价| / 当前价
			distance = math.Abs(level.Price-currentPrice) / currentPrice
		}

		// 权重函数：距离越近权重越高，使用指数衰减
		weight := math.Exp(-distance * 10) // 10为衰减系数，可调节

		// 加权价值：价值 × 权重
		levelValue := level.Price * level.Quantity
		weightedValue += levelValue * weight
	}

	return weightedValue
}

// calculatePressureFull 🔧 P2-1新增：全深度模式：所有档位
func (calc *OrderBookCalculator) calculatePressureFull(levels []OrderBookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}

	var totalValue float64

	for _, level := range levels {
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}
		totalValue += level.Price * level.Quantity
	}

	return totalValue
}

// GetPressureCalculationInfo 🔧 P2-1新增：获取压力计算详细信息
func (calc *OrderBookCalculator) GetPressureCalculationInfo(symbol string) map[string]interface{} {
	calc.mu.RLock()
	defer calc.mu.RUnlock()

	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return map[string]interface{}{
			"error": "symbol not found",
		}
	}

	// 计算所有模式的压力值进行对比
	bidPressures := make(map[string]float64)
	askPressures := make(map[string]float64)

	// 简单模式
	bidPressures["simple"] = calc.calculatePressureSimple(symbolData.currentBids)
	askPressures["simple"] = calc.calculatePressureSimple(symbolData.currentAsks)

	// 深度模式
	bidPressures["deep"] = calc.calculatePressureDeep(symbolData.currentBids)
	askPressures["deep"] = calc.calculatePressureDeep(symbolData.currentAsks)

	// 加权模式
	bidPressures["weighted"] = calc.calculatePressureWeighted(symbol, symbolData.currentBids, true)
	askPressures["weighted"] = calc.calculatePressureWeighted(symbol, symbolData.currentAsks, false)

	// 全深度模式
	bidPressures["full"] = calc.calculatePressureFull(symbolData.currentBids)
	askPressures["full"] = calc.calculatePressureFull(symbolData.currentAsks)

	return map[string]interface{}{
		"symbol":           symbol,
		"current_mode":     calc.getModeString(),
		"current_price":    symbolData.currentPrice,
		"bid_levels_count": len(symbolData.currentBids),
		"ask_levels_count": len(symbolData.currentAsks),
		"bid_pressures":    bidPressures,
		"ask_pressures":    askPressures,
		"pressure_ratios": map[string]float64{
			"simple_ratio":   safeDivisionForOrderBook(bidPressures["simple"], askPressures["simple"]),
			"deep_ratio":     safeDivisionForOrderBook(bidPressures["deep"], askPressures["deep"]),
			"weighted_ratio": safeDivisionForOrderBook(bidPressures["weighted"], askPressures["weighted"]),
			"full_ratio":     safeDivisionForOrderBook(bidPressures["full"], askPressures["full"]),
		},
		"last_update": symbolData.lastUpdate,
	}
}

// getModeString 获取当前模式的字符串表示
func (calc *OrderBookCalculator) getModeString() string {
	switch calc.pressureMode {
	case PressureModeSimple:
		return "simple"
	case PressureModeDeep:
		return "deep"
	case PressureModeWeighted:
		return "weighted"
	case PressureModeFull:
		return "full"
	default:
		return "unknown"
	}
}

// safeDivisionForOrderBook 安全除法，专用于OrderBook计算
func safeDivisionForOrderBook(numerator, denominator float64) float64 {
	if denominator == 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		return 0
	}
	if math.IsNaN(numerator) || math.IsInf(numerator, 0) {
		return 0
	}

	result := numerator / denominator
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0
	}
	return result
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

	// 🔧 修复：使用价格分桶机制，避免浮点数陷阱
	bucketKey := calc.priceToBucket(price, symbolData.wallTracker.tickSize)
	entry, exists := symbolData.wallTracker.wallHistory[bucketKey]

	if !exists {
		// 新墙
		minPrice, maxPrice := calc.bucketToPrice(bucketKey, symbolData.wallTracker.tickSize)
		symbolData.wallTracker.wallHistory[bucketKey] = &WallHistoryEntry{
			bucketKey:      bucketKey,
			priceRange:     [2]float64{minPrice, maxPrice},
			firstSeen:      timestamp,
			lastSeen:       timestamp,
			appearances:    1,
			disappearances: 0,
			maxSize:        size,
			minSize:        size,
			avgSize:        size,
			sizeHistory:    []float64{size},
			isActive:       true,
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

		// 更新价格范围（扩展到包含新价格）
		if price < entry.priceRange[0] {
			entry.priceRange[0] = price
		}
		if price > entry.priceRange[1] {
			entry.priceRange[1] = price
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

	for bucketKey, entry := range symbolData.wallTracker.wallHistory {
		if entry.lastSeen.Before(cutoffTime) {
			delete(symbolData.wallTracker.wallHistory, bucketKey)
		} else if entry.isActive {
			// 检查墙是否已消失（在当前盘口中不存在）
			// 🔧 修复：将bucketKey转换为价格进行检查
			minPrice, _ := calc.bucketToPrice(bucketKey, symbolData.wallTracker.tickSize)
			if !calc.isWallCurrentlyPresent(symbol, minPrice) {
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

	// 🔧 修复：使用分桶机制查找墙
	bucketKey := calc.priceToBucket(price, symbolData.wallTracker.tickSize)
	minPrice, maxPrice := calc.bucketToPrice(bucketKey, symbolData.wallTracker.tickSize)

	// 检查买单档位
	for _, bid := range symbolData.currentBids {
		if bid.Price >= minPrice && bid.Price <= maxPrice { // 价格在分桶范围内
			levelValue := bid.Price * bid.Quantity
			if levelValue > threshold {
				return true
			}
		}
	}

	// 检查卖单档位
	for _, ask := range symbolData.currentAsks {
		if ask.Price >= minPrice && ask.Price <= maxPrice { // 价格在分桶范围内
			levelValue := ask.Price * ask.Quantity
			if levelValue > threshold {
				return true
			}
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

	// 🔧 修复：使用分桶机制查找墙历史
	bucketKey := calc.priceToBucket(price, symbolData.wallTracker.tickSize)
	entry, exists := symbolData.wallTracker.wallHistory[bucketKey]
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

// calculatePressureDelta5m 计算5分钟压力变化（V2.0）- 🚨 已弃用，请使用calculatePressureDelta5mFixed
// 🔥 P0-04风险：此方法使用time.Now()和baseline=0会产生虚假信号
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

	// 🚨 P0-04风险：使用time.Now()而非数据时间戳
	fiveMinuteAgo := time.Now().Add(-5 * time.Minute)
	var baseline float64 // 🚨 P0-04风险：找不到基准点时保持0，产生虚假巨大变化

	for i := len(symbolData.imbalanceHistory) - 1; i >= 0; i-- {
		if symbolData.imbalanceHistory[i].Timestamp.Before(fiveMinuteAgo) {
			baseline = symbolData.imbalanceHistory[i].Ratio
			break
		}
	}

	return currentImbalance - baseline
}

// calculatePressureDelta5mFixed 计算5分钟压力变化（V-12.3 P0-04修复版本）
// 🔥 P0-04修复：解决基准点缺失时的虚假巨大变化问题
func (calc *OrderBookCalculator) calculatePressureDelta5mFixed(symbol string, currentBidPressure, currentAskPressure float64, currentTime time.Time) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return 0
	}

	// 基于当前失衡比例计算压力差异
	currentImbalance := calc.calculateImbalance(symbol)

	if len(symbolData.imbalanceHistory) < 2 {
		return 0 // 数据不足，返回0而非虚假变化
	}

	// 🔥 P0-04修复：使用传入的数据时间戳，而非time.Now()
	fiveMinuteAgo := currentTime.Add(-5 * time.Minute)

	// 🔥 P0-04修复：寻找最接近5分钟前的基准点
	var baseline float64
	var baselineTime time.Time
	var foundBaseline bool

	// 从最新数据向前寻找最接近5分钟前的点
	for i := len(symbolData.imbalanceHistory) - 1; i >= 0; i-- {
		historyPoint := symbolData.imbalanceHistory[i]

		// 🔥 P0-04修复：寻找 <= fiveMinuteAgo 的最近点
		if historyPoint.Timestamp.Before(fiveMinuteAgo) || historyPoint.Timestamp.Equal(fiveMinuteAgo) {
			baseline = historyPoint.Ratio
			baselineTime = historyPoint.Timestamp
			foundBaseline = true
			break
		}
	}

	// 🔥 P0-04修复：找不到基准点时的安全兜底
	if !foundBaseline {
		// 获取最早的数据点作为兜底基准
		if len(symbolData.imbalanceHistory) > 0 {
			oldestPoint := symbolData.imbalanceHistory[0]
			coverageDuration := currentTime.Sub(oldestPoint.Timestamp)

			// 如果数据覆盖不足2分钟，认为变化不可信
			if coverageDuration < 2*time.Minute {
				return 0 // insufficient_data
			}

			// 使用最早点作为基准，但应用置信度衰减
			baseline = oldestPoint.Ratio
			baselineTime = oldestPoint.Timestamp

			// 🔥 P0-04修复：置信度衰减机制
			// 覆盖时长比例：实际覆盖时长 / 期望的5分钟
			coverageRatio := coverageDuration.Minutes() / 5.0
			confidenceDecay := math.Min(1.0, coverageRatio) // 最大衰减到原值

			rawDelta := currentImbalance - baseline

			if os.Getenv("NOFX_DEBUG") == "true" {
				log.Printf("🔧 [%s] P0-04兜底：覆盖时长=%.1fm, 置信度衰减=%.3f, 原始差异=%.6f, 衰减后=%.6f",
					symbol, coverageDuration.Minutes(), confidenceDecay, rawDelta, rawDelta*confidenceDecay)
			}

			return rawDelta * confidenceDecay
		}

		// 完全没有历史数据
		return 0
	}

	// 🔥 P0-04修复：正常情况，计算时间加权的压力差异
	timeDiff := currentTime.Sub(baselineTime)
	rawDelta := currentImbalance - baseline

	// 如果时间差异过大（>10分钟），应用时间衰减
	if timeDiff > 10*time.Minute {
		timeDecay := 10.0 / timeDiff.Minutes() // 10分钟后开始衰减
		rawDelta *= timeDecay

		if os.Getenv("NOFX_DEBUG") == "true" {
			log.Printf("🔧 [%s] P0-04时间衰减：基准点距离=%.1fm, 衰减系数=%.3f",
				symbol, timeDiff.Minutes(), timeDecay)
		}
	}

	return rawDelta
}

// calculateSpoofingRisk 计算虚假挂单风险评分（V2.0修复版）
func (calc *OrderBookCalculator) calculateSpoofingRisk(symbol string) float64 {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return 0
	}

	now := time.Now()
	recentTimeThreshold := now.Add(-5 * time.Minute) // 🔧 修复：只看最近5分钟的墙

	var maxSingleRisk float64 = 0.0 // 🔧 修复：取单个墙的最大风险，而非累加
	activeWallCount := 0

	// 🔧 修复：只遍历活跃且最近的墙
	for _, entry := range symbolData.wallTracker.wallHistory {
		// 🔧 修复：过滤条件 - 只看活跃且最近有活动的墙
		if !entry.isActive || entry.lastSeen.Before(recentTimeThreshold) {
			continue
		}

		activeWallCount++

		// 计算单个墙的风险评分
		singleRisk := 0.0

		// 闪烁风险：基于消失次数相对于出现次数的比例
		if entry.appearances > 0 {
			flickerRatio := float64(entry.disappearances) / float64(entry.appearances)
			if flickerRatio > 0.5 { // 消失次数超过出现次数的一半才算风险
				singleRisk += math.Min(0.6, flickerRatio) // 最高0.6分
			}
		}

		// 大小变化风险：如果墙的大小变化过于剧烈
		if entry.maxSize > 0 && len(entry.sizeHistory) > 3 {
			sizeVariation := (entry.maxSize - entry.minSize) / entry.maxSize
			if sizeVariation > 0.8 { // 大小变化超过80%
				singleRisk += math.Min(0.3, sizeVariation-0.5) // 最高0.3分
			}
		}

		// 时间衰减：越久的墙风险越低
		existenceMinutes := now.Sub(entry.firstSeen).Minutes()
		timeDecay := math.Max(0.1, 1.0-existenceMinutes/60) // 1小时后衰减到0.1
		singleRisk *= timeDecay

		// 🔧 修复：取最大值而非累加
		if singleRisk > maxSingleRisk {
			maxSingleRisk = singleRisk
		}
	}

	// 🔧 修复：基于5分钟内墙变化频率的额外风险（但有上限）
	changeFrequencyRisk := 0.0
	if symbolData.wallChangeCount5m > 20 { // 5分钟内变化超过20次才算异常
		changeFrequencyRisk = math.Min(0.4, float64(symbolData.wallChangeCount5m-20)/50.0)
	}

	// 🔧 修复：最终风险评分 = max(单墙风险, 变化频率风险)
	finalRisk := math.Max(maxSingleRisk, changeFrequencyRisk)

	// 🔧 修复：如果没有活跃墙，风险为0
	if activeWallCount == 0 {
		finalRisk = 0
	}

	// 确保在0-1范围内
	return math.Max(0, math.Min(1, finalRisk))
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

		// 🔧 修复: 增强价差计算的安全性
		if bestBid > 0 && bestAsk > bestBid {
			spread := (bestAsk - bestBid) / bestBid
			// 🔧 修复: 检查spread是否为有效值
			if !math.IsNaN(spread) && !math.IsInf(spread, 0) && spread >= 0 {
				spreadScore := math.Max(0, 1.0-spread*1000) // 价差越小评分越高
				liquidityScore += spreadScore * 0.3
			}
		}
	}

	// 3. 总量评分 (30%权重)
	totalBidValue := 0.0
	totalAskValue := 0.0

	for _, bid := range symbolData.currentBids {
		// 🔧 修复: 跳过无效数据
		if bid.Price > 0 && bid.Quantity > 0 {
			bidValue := bid.Price * bid.Quantity
			if !math.IsNaN(bidValue) && !math.IsInf(bidValue, 0) {
				totalBidValue += bidValue
			}
		}
	}

	for _, ask := range symbolData.currentAsks {
		// 🔧 修复: 跳过无效数据
		if ask.Price > 0 && ask.Quantity > 0 {
			askValue := ask.Price * ask.Quantity
			if !math.IsNaN(askValue) && !math.IsInf(askValue, 0) {
				totalAskValue += askValue
			}
		}
	}

	totalLiquidity := totalBidValue + totalAskValue
	// 🔧 修复: 确保总流动性计算有效
	if totalLiquidity > 0 && !math.IsNaN(totalLiquidity) && !math.IsInf(totalLiquidity, 0) {
		// 假设1000万USD为高流动性基准
		volumeScore := math.Min(totalLiquidity/10000000.0, 1.0)
		liquidityScore += volumeScore * 0.3
	}

	// 🔧 修复: 确保最终评分在有效范围内
	if math.IsNaN(liquidityScore) || math.IsInf(liquidityScore, 0) {
		return 0
	}

	return math.Max(0, math.Min(1, liquidityScore))
}

// getWallFirstSeen 获取墙���首次出现时间
func (calc *OrderBookCalculator) getWallFirstSeen(symbol string, price float64) time.Time {
	symbolData := calc.symbolData[symbol]
	if symbolData == nil {
		return time.Now()
	}

	// 🔧 修复：将价格转换为bucketKey
	bucketKey := calc.priceToBucket(price, symbolData.wallTracker.tickSize)
	if entry, exists := symbolData.wallTracker.wallHistory[bucketKey]; exists {
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

	// 🔧 修复：将价格转换为bucketKey
	bucketKey := calc.priceToBucket(price, symbolData.wallTracker.tickSize)
	if entry, exists := symbolData.wallTracker.wallHistory[bucketKey]; exists {
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

	// 🔧 修复：将价格转换为bucketKey
	bucketKey := calc.priceToBucket(price, symbolData.wallTracker.tickSize)
	if entry, exists := symbolData.wallTracker.wallHistory[bucketKey]; exists {
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

	// 🔧 修复：将价格转换为bucketKey
	bucketKey := calc.priceToBucket(price, symbolData.wallTracker.tickSize)
	if entry, exists := symbolData.wallTracker.wallHistory[bucketKey]; exists {
		return entry.minSize
	}
	return 0
}

// ===== 🔧 修复: 内存管理和清理机制 =====

// Cleanup 清理过期数据和内存（防止内存泄漏）
func (calc *OrderBookCalculator) Cleanup() {
	calc.mu.Lock()
	defer calc.mu.Unlock()

	now := time.Now()
	cleanedSymbols := 0

	for symbol, symbolData := range calc.symbolData {
		if symbolData == nil {
			delete(calc.symbolData, symbol)
			cleanedSymbols++
			continue
		}

		// 清理过期的失衡历史记录（只保留最近1小时的数据）
		cutoffTime := now.Add(-1 * time.Hour)
		var validHistory []ImbalancePoint
		for _, point := range symbolData.imbalanceHistory {
			if point.Timestamp.After(cutoffTime) {
				validHistory = append(validHistory, point)
			}
		}

		// 只有在数据变化时才更新，减少内存分配
		if len(validHistory) != len(symbolData.imbalanceHistory) {
			symbolData.imbalanceHistory = validHistory
		}

		// 清理墙追踪历史中的过期记录
		if symbolData.wallTracker != nil {
			calc.cleanupWallTrackerMemory(symbolData, now)
		}

		// 如果交易对数据已经很久没有更新，删除整个交易对的数据
		if now.Sub(symbolData.lastUpdate) > 2*time.Hour {
			delete(calc.symbolData, symbol)
			cleanedSymbols++
			log.Printf("🗑️ [内存清理] 删除过期交易对数据: %s (最后更新: %s)",
				symbol, symbolData.lastUpdate.Format("15:04:05"))
		}
	}

	if cleanedSymbols > 0 {
		log.Printf("✅ OrderBook内存清理完成，清理了 %d 个交易对的过期数据", cleanedSymbols)
	}
}

// cleanupWallTrackerMemory 清理墙追踪器的内存
func (calc *OrderBookCalculator) cleanupWallTrackerMemory(symbolData *SymbolOrderBookData, now time.Time) {
	if symbolData.wallTracker == nil {
		return
	}

	cleanedWalls := 0
	maxWallAge := 2 * time.Hour // 墙的最大存活时间

	for price, entry := range symbolData.wallTracker.wallHistory {
		if entry == nil || now.Sub(entry.lastSeen) > maxWallAge {
			delete(symbolData.wallTracker.wallHistory, price)
			cleanedWalls++
			continue
		}

		// 限制每个墙的大小历史记录数量，防止无限增长
		if len(entry.sizeHistory) > 100 {
			// 只保留最近的50个记录
			entry.sizeHistory = entry.sizeHistory[len(entry.sizeHistory)-50:]

			// 重新计算平均值
			total := 0.0
			for _, size := range entry.sizeHistory {
				total += size
			}
			if len(entry.sizeHistory) > 0 {
				entry.avgSize = total / float64(len(entry.sizeHistory))
			}
		}
	}

	if cleanedWalls > 0 {
		log.Printf("🗑️ [墙追踪清理] 清理了 %d 个过期墙记录", cleanedWalls)
	}
}

// GetMemoryStats 获取内存使用统计（用于监控）
func (calc *OrderBookCalculator) GetMemoryStats() map[string]interface{} {
	calc.mu.RLock()
	defer calc.mu.RUnlock()

	totalSymbols := len(calc.symbolData)
	totalImbalancePoints := 0
	totalWallRecords := 0

	for _, symbolData := range calc.symbolData {
		if symbolData != nil {
			totalImbalancePoints += len(symbolData.imbalanceHistory)
			if symbolData.wallTracker != nil {
				totalWallRecords += len(symbolData.wallTracker.wallHistory)
			}
		}
	}

	return map[string]interface{}{
		"total_symbols":          totalSymbols,
		"total_imbalance_points": totalImbalancePoints,
		"total_wall_records":     totalWallRecords,
		"avg_imbalance_per_symbol": func() float64 {
			if totalSymbols == 0 {
				return 0
			}
			return float64(totalImbalancePoints) / float64(totalSymbols)
		}(),
		"avg_walls_per_symbol": func() float64 {
			if totalSymbols == 0 {
				return 0
			}
			return float64(totalWallRecords) / float64(totalSymbols)
		}(),
		"memory_health": func() string {
			if totalSymbols > 100 {
				return "内存使用过高"
			} else if totalSymbols > 50 {
				return "内存使用较高"
			} else {
				return "内存使用正常"
			}
		}(),
	}
}
