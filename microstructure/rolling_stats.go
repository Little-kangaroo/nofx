package microstructure

import (
	"log"
	"math"
	"sync"
	"time"
)

// ===== V-12.2 滚动统计引擎核心实现 =====

// StatValue 统计数据点
type StatValue struct {
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

// RollingStats 滚动统计计算器（V-12.2核心组件）
type RollingStats struct {
	mu            sync.RWMutex
	values        []StatValue   // Ring Buffer 实现
	maxSize       int           // 最大数据点数量
	windowDuration time.Duration // 时间窗口大小
	
	// 🟡 P2修复: Ring Buffer指针支持
	pointer       int           // 当前写入位置
	isFull        bool          // Buffer是否已满
	rebaseCounter int           // 🟠 P1修复: 重算计数器
	
	// O(1) 统计计算缓存
	sum           float64 // 数值总和
	sumSquared    float64 // 平方和
	count         int     // 有效数据点数量
	lastUpdate    time.Time
	
	// 自适应特性
	name          string  // 指标名称（用于日志）
	minSamples    int     // 最小样本数（低于此数不计算Z-Score）
}

// NewRollingStats 创建滚动统计器
func NewRollingStats(name string, windowDuration time.Duration, maxSize int) *RollingStats {
	return &RollingStats{
		name:           name,
		values:         make([]StatValue, 0, maxSize),
		maxSize:        maxSize,
		windowDuration: windowDuration,
		minSamples:     10, // 至少10个样本才开始计算Z-Score
	}
}

// AddValue 添加新数据点（V-12.2动态原则实现）
func (rs *RollingStats) AddValue(value float64, timestamp time.Time) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	// 🔧 数据有效性检查
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	
	// 添加新数据点
	newPoint := StatValue{
		Value:     value,
		Timestamp: timestamp,
	}
	
	// 🟠 P1修复: 浮点精度重算机制 - 每1000次操作重新计算一次
	rs.rebaseCounter++
	if rs.rebaseCounter >= 1000 && rs.count > 0 {
		rs.recomputeFromScratch()
		rs.rebaseCounter = 0
		log.Printf("🔄 [%s] 浮点精度重算完成，样本数: %d", rs.name, rs.count)
	}
	
	// 🟡 P2修复: 真正的O(1) Ring Buffer实现
	if !rs.isFull {
		// Buffer未满，直接append
		if len(rs.values) < rs.maxSize {
			rs.values = append(rs.values, newPoint)
		} else {
			// 初始化到maxSize
			if len(rs.values) == rs.maxSize {
				rs.isFull = true
			}
		}
		rs.count++
	} else {
		// Buffer已满，需要替换最老的数据
		oldPoint := rs.values[rs.pointer]
		
		// O(1) 更新统计量：先减去旧值
		rs.sum -= oldPoint.Value
		rs.sumSquared -= oldPoint.Value * oldPoint.Value
		
		// O(1) 覆盖操作，不需要copy移动数组
		rs.values[rs.pointer] = newPoint
		rs.pointer = (rs.pointer + 1) % rs.maxSize
	}
	
	// 加上新值
	rs.sum += value
	rs.sumSquared += value * value
	rs.lastUpdate = timestamp
	
	// 清理过期数据（保持时间窗口）- 这个操作频率较低
	rs.cleanupExpiredData(timestamp)
}

// 🟠 P1修复: 浮点精度重算机制
func (rs *RollingStats) recomputeFromScratch() {
	var newSum, newSumSquared float64
	validCount := 0
	
	// 重新从头累加，消除IEEE 754累积误差
	for i := 0; i < len(rs.values); i++ {
		if rs.isFull || i < rs.count {
			value := rs.values[i].Value
			newSum += value
			newSumSquared += value * value
			validCount++
		}
	}
	
	// 更新精确值
	rs.sum = newSum
	rs.sumSquared = newSumSquared
	rs.count = validCount
}

// cleanupExpiredData 清理过期数据（维持时间窗口）
func (rs *RollingStats) cleanupExpiredData(currentTime time.Time) {
	cutoffTime := currentTime.Add(-rs.windowDuration)
	
	// 从头部开始移除过期数据
	validStart := 0
	for i, point := range rs.values {
		if point.Timestamp.After(cutoffTime) {
			validStart = i
			break
		}
		// 更新统计缓存
		rs.sum -= point.Value
		rs.sumSquared -= point.Value * point.Value
		rs.count--
		validStart = i + 1
	}
	
	if validStart > 0 {
		rs.values = rs.values[validStart:]
	}
}

// GetMean 获取均值（V-12.2动态基准）
func (rs *RollingStats) GetMean() float64 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	
	if rs.count == 0 {
		return 0
	}
	return rs.sum / float64(rs.count)
}

// GetStdDev 获取标准差（V-12.2动态基准）
func (rs *RollingStats) GetStdDev() float64 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	
	if rs.count <= 1 {
		return 0
	}
	
	mean := rs.sum / float64(rs.count)
	variance := (rs.sumSquared / float64(rs.count)) - (mean * mean)
	
	// 防止数值误差导致的负方差
	if variance < 0 {
		variance = 0
	}
	
	return math.Sqrt(variance)
}

// GetZScore 计算Z-Score（V-12.2核心算法）
func (rs *RollingStats) GetZScore(value float64) float64 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	
	// 🔧 样本数不足，无法可靠计算Z-Score
	if rs.count < rs.minSamples {
		return 0
	}
	
	mean := rs.sum / float64(rs.count)
	variance := (rs.sumSquared / float64(rs.count)) - (mean * mean)
	
	if variance <= 0 {
		return 0 // 方差为0，所有数据相同
	}
	
	stdDev := math.Sqrt(variance)
	zScore := (value - mean) / stdDev
	
	// 🔧 防止极端Z-Score值
	if math.IsNaN(zScore) || math.IsInf(zScore, 0) {
		return 0
	}
	
	return zScore
}

// GetStats 获取完整统计信息（调试和监控用）
func (rs *RollingStats) GetStats() map[string]interface{} {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	
	return map[string]interface{}{
		"name":            rs.name,
		"count":           rs.count,
		"mean":            rs.GetMeanUnsafe(),
		"std_dev":         rs.GetStdDevUnsafe(),
		"window_minutes":  rs.windowDuration.Minutes(),
		"last_update":     rs.lastUpdate,
		"data_quality":    rs.getDataQuality(),
	}
}

// GetMeanUnsafe 内部使用的无锁均值计算
func (rs *RollingStats) GetMeanUnsafe() float64 {
	if rs.count == 0 {
		return 0
	}
	return rs.sum / float64(rs.count)
}

// GetStdDevUnsafe 内部使用的无锁标准差计算
func (rs *RollingStats) GetStdDevUnsafe() float64 {
	if rs.count <= 1 {
		return 0
	}
	
	mean := rs.sum / float64(rs.count)
	variance := (rs.sumSquared / float64(rs.count)) - (mean * mean)
	
	if variance < 0 {
		variance = 0
	}
	
	return math.Sqrt(variance)
}

// getDataQuality 评估数据质量（用于自适应调整）
func (rs *RollingStats) getDataQuality() float64 {
	if rs.count == 0 {
		return 0
	}
	
	// 基于样本数量和时间覆盖度评估质量
	sampleRatio := float64(rs.count) / float64(rs.maxSize)
	timeCoverage := 1.0
	
	if len(rs.values) > 0 {
		actualDuration := rs.lastUpdate.Sub(rs.values[0].Timestamp)
		timeCoverage = math.Min(1.0, actualDuration.Seconds()/rs.windowDuration.Seconds())
	}
	
	quality := (sampleRatio*0.4 + timeCoverage*0.6)
	return math.Min(1.0, quality)
}

// ===== 多维度统计管理器 =====

// SymbolStatsManager 单个交易对的多维度统计管理器（V-12.2）
type SymbolStatsManager struct {
	symbol     string
	mu         sync.RWMutex
	
	// 价格维度统计
	priceChange1m  *RollingStats // 1分钟价格变化率
	priceChange5m  *RollingStats // 5分钟价格变化率
	
	// 成交量维度统计
	volume1m       *RollingStats // 1分钟成交量（USD）
	volumeRatio1m  *RollingStats // 1分钟成交量比率
	
	// 资金流维度统计
	spotCVD1m      *RollingStats // 1分钟现货CVD增量
	futuresCVD1m   *RollingStats // 1分钟合约CVD增量
	cvdRatio       *RollingStats // 现货/合约CVD比率
	
	// 盘口维度统计
	spoofingRisk   *RollingStats // 虚假挂单风险
	liquidityScore *RollingStats // 流动性评分
	imbalanceRatio *RollingStats // 买卖失衡比率
	
	lastUpdate     time.Time
}

// NewSymbolStatsManager 创建交易对统计管理器
func NewSymbolStatsManager(symbol string) *SymbolStatsManager {
	windowDuration := 1 * time.Hour // V-12.2使用1小时滚动窗口
	maxSize := 3600 // 每秒1个数据点，1小时3600个
	
	return &SymbolStatsManager{
		symbol:         symbol,
		priceChange1m:  NewRollingStats(symbol+"_price_1m", windowDuration, maxSize),
		priceChange5m:  NewRollingStats(symbol+"_price_5m", windowDuration, maxSize/5),
		volume1m:       NewRollingStats(symbol+"_volume_1m", windowDuration, maxSize),
		volumeRatio1m:  NewRollingStats(symbol+"_vol_ratio_1m", windowDuration, maxSize),
		spotCVD1m:      NewRollingStats(symbol+"_spot_cvd_1m", windowDuration, maxSize),
		futuresCVD1m:   NewRollingStats(symbol+"_futures_cvd_1m", windowDuration, maxSize),
		cvdRatio:       NewRollingStats(symbol+"_cvd_ratio", windowDuration, maxSize),
		spoofingRisk:   NewRollingStats(symbol+"_spoofing", windowDuration, maxSize),
		liquidityScore: NewRollingStats(symbol+"_liquidity", windowDuration, maxSize),
		imbalanceRatio: NewRollingStats(symbol+"_imbalance", windowDuration, maxSize),
	}
}

// UpdateMarketData 更新市场数据（每秒调用）
func (ssm *SymbolStatsManager) UpdateMarketData(snapshot *MarketSnapshot) {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()
	
	now := time.Now()
	ssm.lastUpdate = now
	
	// 🔧 更新各维度统计数据
	if snapshot.CVDDelta5m != nil {
		ssm.priceChange1m.AddValue(snapshot.CVDDelta5m.PriceDeltaPct, now)
		ssm.spotCVD1m.AddValue(snapshot.CVDDelta5m.SpotCVDDeltaUSD, now)
		ssm.futuresCVD1m.AddValue(snapshot.CVDDelta5m.FuturesCVDDeltaUSD, now)
		ssm.volume1m.AddValue(snapshot.CVDDelta5m.VolumeDelta, now)
		ssm.volumeRatio1m.AddValue(snapshot.CVDDelta5m.VolumeRatio, now)
		
		// CVD比率计算（现货主导程度）
		if snapshot.CVDDelta5m.FuturesCVDDeltaUSD != 0 {
			cvdRatio := snapshot.CVDDelta5m.SpotCVDDeltaUSD / snapshot.CVDDelta5m.FuturesCVDDeltaUSD
			ssm.cvdRatio.AddValue(math.Abs(cvdRatio), now)
		}
	}
	
	if snapshot.OrderBookData != nil {
		ssm.spoofingRisk.AddValue(snapshot.OrderBookData.SpoofingRisk, now)
		ssm.liquidityScore.AddValue(snapshot.OrderBookData.LiquidityScore, now)
		ssm.imbalanceRatio.AddValue(snapshot.OrderBookData.ImbalanceRatio, now)
	}
}

// GetZScores 获取当前所有维度的Z-Score（V-12.2触发判断核心）
func (ssm *SymbolStatsManager) GetZScores(snapshot *MarketSnapshot) map[string]float64 {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	
	zScores := make(map[string]float64)
	
	if snapshot.CVDDelta5m != nil {
		zScores["price_change_1m"] = ssm.priceChange1m.GetZScore(snapshot.CVDDelta5m.PriceDeltaPct)
		zScores["spot_cvd_1m"] = ssm.spotCVD1m.GetZScore(snapshot.CVDDelta5m.SpotCVDDeltaUSD)
		zScores["futures_cvd_1m"] = ssm.futuresCVD1m.GetZScore(snapshot.CVDDelta5m.FuturesCVDDeltaUSD)
		zScores["volume_1m"] = ssm.volume1m.GetZScore(snapshot.CVDDelta5m.VolumeDelta)
		zScores["volume_ratio_1m"] = ssm.volumeRatio1m.GetZScore(snapshot.CVDDelta5m.VolumeRatio)
		
		// 现货主导程度Z-Score
		if snapshot.CVDDelta5m.FuturesCVDDeltaUSD != 0 {
			cvdRatio := math.Abs(snapshot.CVDDelta5m.SpotCVDDeltaUSD / snapshot.CVDDelta5m.FuturesCVDDeltaUSD)
			zScores["cvd_ratio"] = ssm.cvdRatio.GetZScore(cvdRatio)
		}
	}
	
	if snapshot.OrderBookData != nil {
		zScores["spoofing_risk"] = ssm.spoofingRisk.GetZScore(snapshot.OrderBookData.SpoofingRisk)
		zScores["liquidity_score"] = ssm.liquidityScore.GetZScore(snapshot.OrderBookData.LiquidityScore)
		zScores["imbalance_ratio"] = ssm.imbalanceRatio.GetZScore(snapshot.OrderBookData.ImbalanceRatio)
	}
	
	return zScores
}

// GetStatsOverview 获取统计概览（监控和调试）
func (ssm *SymbolStatsManager) GetStatsOverview() map[string]interface{} {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	
	return map[string]interface{}{
		"symbol":          ssm.symbol,
		"last_update":     ssm.lastUpdate,
		"price_stats":     ssm.priceChange1m.GetStats(),
		"volume_stats":    ssm.volume1m.GetStats(),
		"spot_cvd_stats":  ssm.spotCVD1m.GetStats(),
		"futures_cvd_stats": ssm.futuresCVD1m.GetStats(),
		"spoofing_stats":  ssm.spoofingRisk.GetStats(),
		"liquidity_stats": ssm.liquidityScore.GetStats(),
	}
}

// ===== 全局统计管理器 =====

// GlobalStatsManager 全局统计管理器（V-12.2）
type GlobalStatsManager struct {
	mu            sync.RWMutex
	symbolStats   map[string]*SymbolStatsManager // symbol -> stats
	updateTicker  *time.Ticker
	isRunning     bool
}

var globalStatsManager *GlobalStatsManager
var globalStatsOnce sync.Once

// GetGlobalStatsManager 获取全局统计管理器实例
func GetGlobalStatsManager() *GlobalStatsManager {
	globalStatsOnce.Do(func() {
		globalStatsManager = &GlobalStatsManager{
			symbolStats: make(map[string]*SymbolStatsManager),
		}
	})
	return globalStatsManager
}

// Start 启动统计管理器
func (gsm *GlobalStatsManager) Start() {
	gsm.mu.Lock()
	defer gsm.mu.Unlock()
	
	if gsm.isRunning {
		return
	}
	
	gsm.updateTicker = time.NewTicker(1 * time.Second) // 每秒更新
	gsm.isRunning = true
	
	go gsm.updateLoop()
	
	log.Printf("🚀 V-12.2 全局统计管理器启动完成")
}

// Stop 停止统计管理器
func (gsm *GlobalStatsManager) Stop() {
	gsm.mu.Lock()
	defer gsm.mu.Unlock()
	
	if !gsm.isRunning {
		return
	}
	
	if gsm.updateTicker != nil {
		gsm.updateTicker.Stop()
	}
	
	gsm.isRunning = false
	log.Printf("⛔ V-12.2 全局统计管理器已停止")
}

// updateLoop 统计更新循环
func (gsm *GlobalStatsManager) updateLoop() {
	for range gsm.updateTicker.C {
		gsm.mu.RLock()
		if !gsm.isRunning {
			gsm.mu.RUnlock()
			break
		}
		gsm.mu.RUnlock()
		
		// 更新所有交易对的统计数据
		ofm := GetGlobalOrderFlowManager()
		if ofm == nil {
			continue
		}
		
		snapshots := ofm.GetAllMarketSnapshots()
		for symbol, snapshot := range snapshots {
			gsm.updateSymbolStats(symbol, snapshot)
		}
	}
}

// updateSymbolStats 更新单个交易对统计
func (gsm *GlobalStatsManager) updateSymbolStats(symbol string, snapshot *MarketSnapshot) {
	gsm.mu.Lock()
	defer gsm.mu.Unlock()
	
	// 获取或创建该交易对的统计管理器
	stats, exists := gsm.symbolStats[symbol]
	if !exists {
		stats = NewSymbolStatsManager(symbol)
		gsm.symbolStats[symbol] = stats
		log.Printf("✨ 创建V-12.2统计管理器: %s", symbol)
	}
	
	// 更新统计数据
	stats.UpdateMarketData(snapshot)
}

// GetSymbolZScores 获取交易对的Z-Score（V-12.2触发器调用）
func (gsm *GlobalStatsManager) GetSymbolZScores(symbol string, snapshot *MarketSnapshot) map[string]float64 {
	gsm.mu.RLock()
	defer gsm.mu.RUnlock()
	
	stats, exists := gsm.symbolStats[symbol]
	if !exists {
		return make(map[string]float64) // 返回空的Z-Score
	}
	
	return stats.GetZScores(snapshot)
}

// GetAllStatsOverview 获取所有统计概览（监控接口）
func (gsm *GlobalStatsManager) GetAllStatsOverview() map[string]interface{} {
	gsm.mu.RLock()
	defer gsm.mu.RUnlock()
	
	overview := make(map[string]interface{})
	for symbol, stats := range gsm.symbolStats {
		overview[symbol] = stats.GetStatsOverview()
	}
	
	return overview
}