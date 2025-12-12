package microstructure

import (
	"log"
	"math"
	"os"
	"sync"
	"time"
)

// ===== V-12.3 P0修复：数据完整性保证 =====

// validateDataIntegrity 验证数据完整性（开发模式断言）
func (rs *RollingStats) validateDataIntegrity(operation string) {
	if !rs.debugMode {
		return // 生产环境跳过验证以提高性能
	}
	
	// 核心不变量验证
	actualCount := len(rs.values)
	if rs.count != actualCount {
		log.Fatalf("💥 [%s] %s: count不一致! cached=%d, actual=%d", 
			rs.name, operation, rs.count, actualCount)
	}
	
	// 重新计算sum和sumSquared进行验证
	var actualSum, actualSumSquared float64
	for _, val := range rs.values {
		actualSum += val.Value
		actualSumSquared += val.Value * val.Value
	}
	
	// 允许小量浮点误差（1e-10）
	sumDiff := math.Abs(rs.sum - actualSum)
	sumSquaredDiff := math.Abs(rs.sumSquared - actualSumSquared)
	
	if sumDiff > 1e-10 {
		log.Fatalf("💥 [%s] %s: sum不一致! cached=%.10f, actual=%.10f, diff=%.2e", 
			rs.name, operation, rs.sum, actualSum, sumDiff)
	}
	
	if sumSquaredDiff > 1e-10 {
		log.Fatalf("💥 [%s] %s: sumSquared不一致! cached=%.10f, actual=%.10f, diff=%.2e", 
			rs.name, operation, rs.sumSquared, actualSumSquared, sumSquaredDiff)
	}
	
	// 验证时间有序性
	for i := 1; i < len(rs.values); i++ {
		if rs.values[i].Timestamp.Before(rs.values[i-1].Timestamp) {
			log.Fatalf("💥 [%s] %s: 时间序列破坏! index %d: %s > %s", 
				rs.name, operation, i, 
				rs.values[i-1].Timestamp.Format("15:04:05.000"),
				rs.values[i].Timestamp.Format("15:04:05.000"))
		}
	}
	
	if rs.debugMode {
		log.Printf("✅ [%s] %s: 数据完整性验证通过 (count=%d, sum=%.2f)", 
			rs.name, operation, rs.count, rs.sum)
	}
}

// ===== V-12.3 滚动统计引擎核心实现 =====

// StatValue 统计数据点
type StatValue struct {
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

// RollingStats 滚动统计计算器（V-12.3 P0修复版本 - 纯时间有序队列）
type RollingStats struct {
	mu            sync.RWMutex
	values        []StatValue   // 🔥 P0修复：时间有序队列（非RingBuffer）
	maxSize       int           // 最大数据点数量
	windowDuration time.Duration // 时间窗口大小
	
	// 🔥 P0修复：移除RingBuffer字段，避免数据结构冲突
	// pointer       int           // ❌ 已删除：RingBuffer指针
	// isFull        bool          // ❌ 已删除：Buffer状态标记
	rebaseCounter int           // 🟠 P1修复: 浮点精度重算计数器
	
	// 🔥 P0修复：统计计算缓存（必须与values队列严格一致）
	sum           float64 // 数值总和（invariant: sum == Σ(values[i].Value)）
	sumSquared    float64 // 平方和（invariant: sumSquared == Σ(values[i].Value²)）
	count         int     // 有效数据点数量（invariant: count == len(values)）
	lastUpdate    time.Time
	
	// 自适应特性
	name          string  // 指标名称（用于日志）
	minSamples    int     // 最小样本数（低于此数不计算Z-Score）
	
	// 🔥 P0修复：数据完整性验证（开发模式）
	debugMode     bool    // 是否启用数据完整性断言
}

// NewRollingStats 创建滚动统计器（V-12.3 P0修复版本）
func NewRollingStats(name string, windowDuration time.Duration, maxSize int) *RollingStats {
	return &RollingStats{
		name:           name,
		values:         make([]StatValue, 0, maxSize), // 🔥 P0修复：预分配时间有序队列
		maxSize:        maxSize,
		windowDuration: windowDuration,
		minSamples:     10, // 至少10个样本才开始计算Z-Score
		debugMode:      os.Getenv("NOFX_DEBUG") == "true", // 🔥 P0修复：开发模式断言
	}
}

// AddValue 添加新数据点（V-12.3 P0修复版本 - 纯时间有序队列）
func (rs *RollingStats) AddValue(value float64, timestamp time.Time) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	// 🔧 数据有效性检查
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	
	// 🔥 P0修复：验证操作前数据完整性
	rs.validateDataIntegrity("AddValue-Before")
	
	// 🔥 P0修复：使用纯时间有序队列，完全抛弃RingBuffer逻辑
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
	
	// 🔥 P0修复：纯队列操作 - 在队尾追加新数据
	rs.values = append(rs.values, newPoint)
	
	// 🔥 P0修复：同步更新统计缓存，确保完全一致性
	rs.sum += value
	rs.sumSquared += value * value
	rs.count++
	rs.lastUpdate = timestamp
	
	// 🔥 P0修复：如果超出最大容量，从队首移除最老数据
	if len(rs.values) > rs.maxSize {
		// 移除队首元素并更新统计缓存
		removedValue := rs.values[0].Value
		rs.values = rs.values[1:] // 队首移除操作
		
		// 同步更新统计缓存
		rs.sum -= removedValue
		rs.sumSquared -= removedValue * removedValue
		rs.count--
	}
	
	// 🔥 P0修复：清理过期数据（保持时间窗口）
	rs.cleanupExpiredDataSafely(timestamp)
	
	// 🔥 P0修复：验证操作后数据完整性
	rs.validateDataIntegrity("AddValue-After")
}

// 🔥 P0修复: 浮点精度重算机制（更新为纯队列模式）
func (rs *RollingStats) recomputeFromScratch() {
	var newSum, newSumSquared float64
	
	// 🔥 P0修复：基于实际队列重新累加，消除IEEE 754累积误差
	for _, value := range rs.values {
		newSum += value.Value
		newSumSquared += value.Value * value.Value
	}
	
	// 更新精确值，确保与队列完全一致
	rs.sum = newSum
	rs.sumSquared = newSumSquared
	rs.count = len(rs.values) // 🔥 P0修复：count必须与队列长度一致
}

// cleanupExpiredDataSafely 安全清理过期数据（V-12.3 P0修复版本）
func (rs *RollingStats) cleanupExpiredDataSafely(currentTime time.Time) {
	cutoffTime := currentTime.Add(-rs.windowDuration)
	
	// 🔥 P0修复：验证清理前数据完整性
	rs.validateDataIntegrity("Cleanup-Before")
	
	// 🔥 P0修复：从队首开始找到第一个未过期数据的位置
	validStartIndex := 0
	for i, point := range rs.values {
		if point.Timestamp.After(cutoffTime) {
			validStartIndex = i
			break
		}
		validStartIndex = i + 1 // 如果都过期，设为下一个位置
	}
	
	// 🔥 P0修复：如果有过期数据需要移除
	if validStartIndex > 0 {
		// 计算被移除数据的统计量
		var removedSum, removedSumSquared float64
		removedCount := validStartIndex
		
		for i := 0; i < validStartIndex; i++ {
			removedSum += rs.values[i].Value
			removedSumSquared += rs.values[i].Value * rs.values[i].Value
		}
		
		// 🔥 P0修复：执行队列切片操作，移除过期数据
		rs.values = rs.values[validStartIndex:]
		
		// 🔥 P0修复：同步更新统计缓存，确保完全一致性
		rs.sum -= removedSum
		rs.sumSquared -= removedSumSquared
		rs.count -= removedCount
		
		if rs.debugMode {
			log.Printf("🧹 [%s] 清理了%d个过期数据点，剩余%d个", 
				rs.name, removedCount, rs.count)
		}
	}
	
	// 🔥 P0修复：验证清理后数据完整性
	rs.validateDataIntegrity("Cleanup-After")
}

// GetMean 获取均值（V-12.3 P0修复版本）
func (rs *RollingStats) GetMean() float64 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	
	// 🔥 P0修复：验证数据完整性
	rs.validateDataIntegrity("GetMean")
	
	if rs.count == 0 {
		return 0
	}
	return rs.sum / float64(rs.count)
}

// GetStdDev 获取标准差（V-12.3 P0修复版本）
func (rs *RollingStats) GetStdDev() float64 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	
	// 🔥 P0修复：验证数据完整性
	rs.validateDataIntegrity("GetStdDev")
	
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

// GetZScore 计算Z-Score（V-12.3 P0修复版本）
func (rs *RollingStats) GetZScore(value float64) float64 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	
	// 🔥 P0修复：验证数据完整性
	rs.validateDataIntegrity("GetZScore")
	
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

// SymbolStatsManager 单个交易对的多维度统计管理器（V-12.3 P0-02修复版本）
type SymbolStatsManager struct {
	symbol     string
	mu         sync.RWMutex
	
	// 🔥 P0-02修复：重命名为正确的时间粒度，避免口径错配
	priceChange5m  *RollingStats // 5分钟价格变化率（每5m更新一次）
	
	// 成交量维度统计（5分钟级别）
	volume5m       *RollingStats // 5分钟成交量（USD）
	volumeRatio5m  *RollingStats // 5分钟成交量比率
	
	// 资金流维度统计（5分钟级别）
	spotCVD5m      *RollingStats // 5分钟现货CVD增量
	futuresCVD5m   *RollingStats // 5分钟合约CVD增量
	cvdRatio       *RollingStats // 现货/合约CVD比率（5分钟级别）
	
	// 盘口维度统计（保持原有频率，因为这些是实时盘口数据）
	spoofingRisk   *RollingStats // 虚假挂单风险（实时更新）
	liquidityScore *RollingStats // 流动性评分（实时更新）
	imbalanceRatio *RollingStats // 买卖失衡比率（实时更新）
	
	lastUpdate     time.Time
	last5mUpdate   time.Time // 🔥 P0-02修复：记录上次5m更新时间
}

// NewSymbolStatsManager 创建交易对统计管理器（V-12.3 P0-02修复版本）
func NewSymbolStatsManager(symbol string) *SymbolStatsManager {
	windowDuration := 1 * time.Hour // 使用1小时滚动窗口
	
	// 🔥 P0-02修复：调整maxSize以反映正确的采样频率
	// 5分钟数据：1小时 = 12个数据点，预留空间给24小时 = 288个点
	maxSize5m := 288
	
	// 实时数据（盘口数据）：1小时 = 3600个数据点（每秒1个）
	maxSizeRealtime := 3600
	
	return &SymbolStatsManager{
		symbol:         symbol,
		priceChange5m:  NewRollingStats(symbol+"_price_5m", windowDuration, maxSize5m),
		volume5m:       NewRollingStats(symbol+"_volume_5m", windowDuration, maxSize5m),
		volumeRatio5m:  NewRollingStats(symbol+"_vol_ratio_5m", windowDuration, maxSize5m),
		spotCVD5m:      NewRollingStats(symbol+"_spot_cvd_5m", windowDuration, maxSize5m),
		futuresCVD5m:   NewRollingStats(symbol+"_futures_cvd_5m", windowDuration, maxSize5m),
		cvdRatio:       NewRollingStats(symbol+"_cvd_ratio_5m", windowDuration, maxSize5m),
		spoofingRisk:   NewRollingStats(symbol+"_spoofing", windowDuration, maxSizeRealtime),
		liquidityScore: NewRollingStats(symbol+"_liquidity", windowDuration, maxSizeRealtime),
		imbalanceRatio: NewRollingStats(symbol+"_imbalance", windowDuration, maxSizeRealtime),
	}
}

// UpdateMarketData 更新市场数据（V-12.3 P0-02修复版本）
// 🔥 P0-02修复：分离实时更新和5m K线收盘更新
func (ssm *SymbolStatsManager) UpdateMarketData(snapshot *MarketSnapshot) {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()
	
	now := time.Now()
	ssm.lastUpdate = now
	
	// 🔥 P0-02修复：实时盘口数据仍然每秒更新（正确的）
	if snapshot.OrderBookData != nil {
		ssm.spoofingRisk.AddValue(snapshot.OrderBookData.SpoofingRisk, now)
		ssm.liquidityScore.AddValue(snapshot.OrderBookData.LiquidityScore, now)
		ssm.imbalanceRatio.AddValue(snapshot.OrderBookData.ImbalanceRatio, now)
	}
	
	// 🔥 P0-02修复：5m CVD数据只在K线收盘时更新，不再每秒重复采样
	// 注意：这个方法现在只处理实时数据，5m数据需要通过专门的方法更新
}

// Update5mStats 更新5分钟统计数据（V-12.3 P0-02修复版本）
// 🔥 P0-02修复：专门用于5m K线收盘事件，避免统计口径错配
func (ssm *SymbolStatsManager) Update5mStats(snapshot *MarketSnapshot, klineCloseTime time.Time) {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()
	
	// 🔥 P0-02修复：检查是否确实是新的5分钟收盘
	if !klineCloseTime.After(ssm.last5mUpdate) {
		return // 避免重复更新同一个5分钟数据
	}
	
	ssm.last5mUpdate = klineCloseTime
	
	// 🔥 P0-02修复：现在输入的时间量纲与统计序列名称一致
	if snapshot.CVDDelta5m != nil {
		ssm.priceChange5m.AddValue(snapshot.CVDDelta5m.PriceDeltaPct, klineCloseTime)
		ssm.spotCVD5m.AddValue(snapshot.CVDDelta5m.SpotCVDDeltaUSD, klineCloseTime)
		ssm.futuresCVD5m.AddValue(snapshot.CVDDelta5m.FuturesCVDDeltaUSD, klineCloseTime)
		ssm.volume5m.AddValue(snapshot.CVDDelta5m.VolumeDelta, klineCloseTime)
		ssm.volumeRatio5m.AddValue(snapshot.CVDDelta5m.VolumeRatio, klineCloseTime)
		
		// CVD比率计算（现货主导程度）
		if snapshot.CVDDelta5m.FuturesCVDDeltaUSD != 0 {
			cvdRatio := snapshot.CVDDelta5m.SpotCVDDeltaUSD / snapshot.CVDDelta5m.FuturesCVDDeltaUSD
			ssm.cvdRatio.AddValue(math.Abs(cvdRatio), klineCloseTime)
		}
	}
	
	if os.Getenv("NOFX_DEBUG") == "true" {
		log.Printf("✅ [%s] 5m统计更新完成: K线收盘时间=%s", 
			ssm.symbol, klineCloseTime.Format("15:04:05"))
	}
}

// GetZScores 获取当前所有维度的Z-Score（V-12.3 P0-02修复版本）
func (ssm *SymbolStatsManager) GetZScores(snapshot *MarketSnapshot) map[string]float64 {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	
	zScores := make(map[string]float64)
	
	// 🔥 P0-02修复：使用正确命名的5m统计序列
	if snapshot.CVDDelta5m != nil {
		zScores["price_change_5m"] = ssm.priceChange5m.GetZScore(snapshot.CVDDelta5m.PriceDeltaPct)
		zScores["spot_cvd_5m"] = ssm.spotCVD5m.GetZScore(snapshot.CVDDelta5m.SpotCVDDeltaUSD)
		zScores["futures_cvd_5m"] = ssm.futuresCVD5m.GetZScore(snapshot.CVDDelta5m.FuturesCVDDeltaUSD)
		zScores["volume_5m"] = ssm.volume5m.GetZScore(snapshot.CVDDelta5m.VolumeDelta)
		zScores["volume_ratio_5m"] = ssm.volumeRatio5m.GetZScore(snapshot.CVDDelta5m.VolumeRatio)
		
		// 现货主导程度Z-Score（5分钟级别）
		if snapshot.CVDDelta5m.FuturesCVDDeltaUSD != 0 {
			cvdRatio := math.Abs(snapshot.CVDDelta5m.SpotCVDDeltaUSD / snapshot.CVDDelta5m.FuturesCVDDeltaUSD)
			zScores["cvd_ratio_5m"] = ssm.cvdRatio.GetZScore(cvdRatio)
		}
	}
	
	// 实时盘口数据Z-Score（保持原有逻辑）
	if snapshot.OrderBookData != nil {
		zScores["spoofing_risk"] = ssm.spoofingRisk.GetZScore(snapshot.OrderBookData.SpoofingRisk)
		zScores["liquidity_score"] = ssm.liquidityScore.GetZScore(snapshot.OrderBookData.LiquidityScore)
		zScores["imbalance_ratio"] = ssm.imbalanceRatio.GetZScore(snapshot.OrderBookData.ImbalanceRatio)
	}
	
	return zScores
}

// GetStatsOverview 获取统计概览（监控和调试）- V-12.3 P0-02修复版本
func (ssm *SymbolStatsManager) GetStatsOverview() map[string]interface{} {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	
	return map[string]interface{}{
		"symbol":          ssm.symbol,
		"last_update":     ssm.lastUpdate,
		"last_5m_update":  ssm.last5mUpdate, // 🔥 P0-02修复：区分不同更新时间
		"price_stats_5m":  ssm.priceChange5m.GetStats(),
		"volume_stats_5m": ssm.volume5m.GetStats(),
		"spot_cvd_stats_5m": ssm.spotCVD5m.GetStats(),
		"futures_cvd_stats_5m": ssm.futuresCVD5m.GetStats(),
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

// updateSymbolStats 更新单个交易对统计（V-12.3 P0-02修复版本）
func (gsm *GlobalStatsManager) updateSymbolStats(symbol string, snapshot *MarketSnapshot) {
	gsm.mu.Lock()
	defer gsm.mu.Unlock()
	
	// 获取或创建该交易对的统计管理器
	stats, exists := gsm.symbolStats[symbol]
	if !exists {
		stats = NewSymbolStatsManager(symbol)
		gsm.symbolStats[symbol] = stats
		log.Printf("✨ 创建V-12.3统计管理器: %s", symbol)
	}
	
	// 🔥 P0-02修复：只更新实时数据（盘口数据），5m数据通过专门接口更新
	stats.UpdateMarketData(snapshot)
}

// Update5mStatsForSymbol 更新指定交易对的5分钟统计数据（V-12.3 P0-02修复版本）
// 🔥 P0-02修复：专门用于5m K线收盘事件触发，确保统计口径一致
func (gsm *GlobalStatsManager) Update5mStatsForSymbol(symbol string, snapshot *MarketSnapshot, klineCloseTime time.Time) {
	gsm.mu.Lock()
	defer gsm.mu.Unlock()
	
	// 获取或创建该交易对的统计管理器
	stats, exists := gsm.symbolStats[symbol]
	if !exists {
		stats = NewSymbolStatsManager(symbol)
		gsm.symbolStats[symbol] = stats
		log.Printf("✨ 创建V-12.3统计管理器: %s", symbol)
	}
	
	// 🔥 P0-02修复：更新5分钟统计数据
	stats.Update5mStats(snapshot, klineCloseTime)
}

// UpdateAll5mStats 批量更新所有交易对的5分钟统计数据（V-12.3 P0-02修复版本）
// 🔥 P0-02修复：用于全局5m K线收盘事件
func (gsm *GlobalStatsManager) UpdateAll5mStats(snapshots map[string]*MarketSnapshot, klineCloseTime time.Time) {
	updateCount := 0
	for symbol, snapshot := range snapshots {
		if snapshot.CVDDelta5m != nil {
			gsm.Update5mStatsForSymbol(symbol, snapshot, klineCloseTime)
			updateCount++
		}
	}
	
	if updateCount > 0 && os.Getenv("NOFX_DEBUG") == "true" {
		log.Printf("🔄 [全局5m更新] 更新了%d个交易对的5m统计数据，K线收盘时间: %s", 
			updateCount, klineCloseTime.Format("15:04:05"))
	}
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