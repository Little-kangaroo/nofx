package microstructure

import (
	"log"
	"sync"
	"time"
)

// CacheEntry 缓存条目
type CacheEntry struct {
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
	TTL       time.Duration `json:"ttl"`
}

// IsExpired 检查缓存是否过期
func (entry *CacheEntry) IsExpired() bool {
	return time.Since(entry.Timestamp) > entry.TTL
}

// FiveMinuteDataCache 5分钟数据缓存管理器（V2.0）
type FiveMinuteDataCache struct {
	mu           sync.RWMutex
	cvdCache     map[string]*CacheEntry    // symbol -> CVD 5分钟增量数据
	obCache      map[string]*CacheEntry    // symbol -> 盘口数据
	intentCache  map[string]*CacheEntry    // symbol -> K线意图分析
	priceCache   map[string]*CacheEntry    // symbol -> 价格历史
	cleanupTicker *time.Ticker
	stopCleanup  chan bool
}

// NewFiveMinuteDataCache 创建5分钟数据缓存管理器
func NewFiveMinuteDataCache() *FiveMinuteDataCache {
	cache := &FiveMinuteDataCache{
		cvdCache:    make(map[string]*CacheEntry),
		obCache:     make(map[string]*CacheEntry),
		intentCache: make(map[string]*CacheEntry),
		priceCache:  make(map[string]*CacheEntry),
		stopCleanup: make(chan bool),
	}
	
	// 启动定期清理协程
	cache.startCleanupRoutine()
	
	return cache
}

// startCleanupRoutine 启动定期清理过期数据的协程
func (cache *FiveMinuteDataCache) startCleanupRoutine() {
	cache.cleanupTicker = time.NewTicker(2 * time.Minute) // 每2分钟清理一次
	
	go func() {
		for {
			select {
			case <-cache.cleanupTicker.C:
				cache.cleanupExpiredEntries()
			case <-cache.stopCleanup:
				cache.cleanupTicker.Stop()
				return
			}
		}
	}()
	
	log.Println("🗑️ 5分钟数据缓存清理协程已启动")
}

// Stop 停止缓存管理器
func (cache *FiveMinuteDataCache) Stop() {
	if cache.stopCleanup != nil {
		close(cache.stopCleanup)
	}
	log.Println("🛑 5分钟数据缓存管理器已停止")
}

// cleanupExpiredEntries 清理过期的缓存条目
func (cache *FiveMinuteDataCache) cleanupExpiredEntries() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	
	cleanupCount := 0
	
	// 清理CVD缓存
	for symbol, entry := range cache.cvdCache {
		if entry.IsExpired() {
			delete(cache.cvdCache, symbol)
			cleanupCount++
		}
	}
	
	// 清理盘口缓存
	for symbol, entry := range cache.obCache {
		if entry.IsExpired() {
			delete(cache.obCache, symbol)
			cleanupCount++
		}
	}
	
	// 清理意图分析缓存
	for symbol, entry := range cache.intentCache {
		if entry.IsExpired() {
			delete(cache.intentCache, symbol)
			cleanupCount++
		}
	}
	
	// 清理价格缓存
	for symbol, entry := range cache.priceCache {
		if entry.IsExpired() {
			delete(cache.priceCache, symbol)
			cleanupCount++
		}
	}
	
	if cleanupCount > 0 {
		log.Printf("🗑️ 已清理 %d 个过期缓存条目", cleanupCount)
	}
}

// ===== CVD 5分钟增量数据缓存 =====

// SetCVDDelta5m 缓存CVD 5分钟增量数据
func (cache *FiveMinuteDataCache) SetCVDDelta5m(symbol string, data *CVDDelta5m) {
	if data == nil {
		return
	}
	
	cache.mu.Lock()
	defer cache.mu.Unlock()
	
	cache.cvdCache[symbol] = &CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       15 * time.Minute, // CVD数据保留15分钟
	}
}

// GetCVDDelta5m 获取缓存的CVD 5分钟增量数据
func (cache *FiveMinuteDataCache) GetCVDDelta5m(symbol string) *CVDDelta5m {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	
	entry, exists := cache.cvdCache[symbol]
	if !exists || entry.IsExpired() {
		return nil
	}
	
	if cvdData, ok := entry.Data.(*CVDDelta5m); ok {
		return cvdData
	}
	
	return nil
}

// ===== 盘口数据缓存 =====

// SetOrderBookData 缓存盘口数据
func (cache *FiveMinuteDataCache) SetOrderBookData(symbol string, data *OrderBookData) {
	if data == nil {
		return
	}
	
	cache.mu.Lock()
	defer cache.mu.Unlock()
	
	cache.obCache[symbol] = &CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       5 * time.Minute, // 盘口数据保留5分钟
	}
}

// GetOrderBookData 获取缓存的盘口数据
func (cache *FiveMinuteDataCache) GetOrderBookData(symbol string) *OrderBookData {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	
	entry, exists := cache.obCache[symbol]
	if !exists || entry.IsExpired() {
		return nil
	}
	
	if obData, ok := entry.Data.(*OrderBookData); ok {
		return obData
	}
	
	return nil
}

// ===== K线意图分析缓存 =====

// SetCandleIntentAnalysis 缓存K线意图分析结果
func (cache *FiveMinuteDataCache) SetCandleIntentAnalysis(symbol string, analysis *CandleIntentAnalysis) {
	if analysis == nil {
		return
	}
	
	cache.mu.Lock()
	defer cache.mu.Unlock()
	
	cache.intentCache[symbol] = &CacheEntry{
		Data:      analysis,
		Timestamp: time.Now(),
		TTL:       10 * time.Minute, // 意图分析保留10分钟
	}
}

// GetCandleIntentAnalysis 获取缓存的K线意图分析结果
func (cache *FiveMinuteDataCache) GetCandleIntentAnalysis(symbol string) *CandleIntentAnalysis {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	
	entry, exists := cache.intentCache[symbol]
	if !exists || entry.IsExpired() {
		return nil
	}
	
	if intentData, ok := entry.Data.(*CandleIntentAnalysis); ok {
		return intentData
	}
	
	return nil
}

// ===== 价格历史缓存 =====

// SetPriceHistory 缓存价格历史数据
func (cache *FiveMinuteDataCache) SetPriceHistory(symbol string, history []PriceSnapshot) {
	if len(history) == 0 {
		return
	}
	
	cache.mu.Lock()
	defer cache.mu.Unlock()
	
	// 只保留最近6小时的数据
	cutoffTime := time.Now().Add(-6 * time.Hour)
	var filteredHistory []PriceSnapshot
	
	for _, snapshot := range history {
		if snapshot.Timestamp.After(cutoffTime) {
			filteredHistory = append(filteredHistory, snapshot)
		}
	}
	
	cache.priceCache[symbol] = &CacheEntry{
		Data:      filteredHistory,
		Timestamp: time.Now(),
		TTL:       30 * time.Minute, // 价格历史保留30分钟
	}
}

// GetPriceHistory 获取缓存的价格历史数据
func (cache *FiveMinuteDataCache) GetPriceHistory(symbol string) []PriceSnapshot {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	
	entry, exists := cache.priceCache[symbol]
	if !exists || entry.IsExpired() {
		return nil
	}
	
	if priceData, ok := entry.Data.([]PriceSnapshot); ok {
		return priceData
	}
	
	return nil
}

// ===== 批量操作 =====

// SetBulkData 批量设置缓存数据
func (cache *FiveMinuteDataCache) SetBulkData(symbol string, data *BulkCacheData) {
	if data == nil {
		return
	}
	
	if data.CVDDelta5m != nil {
		cache.SetCVDDelta5m(symbol, data.CVDDelta5m)
	}
	
	if data.OrderBookData != nil {
		cache.SetOrderBookData(symbol, data.OrderBookData)
	}
	
	if data.IntentAnalysis != nil {
		cache.SetCandleIntentAnalysis(symbol, data.IntentAnalysis)
	}
	
	if data.PriceHistory != nil {
		cache.SetPriceHistory(symbol, data.PriceHistory)
	}
}

// GetBulkData 批量获取缓存数据
func (cache *FiveMinuteDataCache) GetBulkData(symbol string) *BulkCacheData {
	return &BulkCacheData{
		CVDDelta5m:     cache.GetCVDDelta5m(symbol),
		OrderBookData:  cache.GetOrderBookData(symbol),
		IntentAnalysis: cache.GetCandleIntentAnalysis(symbol),
		PriceHistory:   cache.GetPriceHistory(symbol),
	}
}

// BulkCacheData 批量缓存数据结构
type BulkCacheData struct {
	CVDDelta5m     *CVDDelta5m           `json:"cvd_delta_5m"`
	OrderBookData  *OrderBookData        `json:"orderbook_data"`
	IntentAnalysis *CandleIntentAnalysis `json:"intent_analysis"`
	PriceHistory   []PriceSnapshot       `json:"price_history"`
}

// ===== 缓存统计 =====

// GetCacheStats 获取缓存统计信息
func (cache *FiveMinuteDataCache) GetCacheStats() *CacheStats {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	
	stats := &CacheStats{
		CVDEntries:    len(cache.cvdCache),
		OBEntries:     len(cache.obCache),
		IntentEntries: len(cache.intentCache),
		PriceEntries:  len(cache.priceCache),
		TotalEntries:  len(cache.cvdCache) + len(cache.obCache) + len(cache.intentCache) + len(cache.priceCache),
	}
	
	// 计算过期条目数
	for _, entry := range cache.cvdCache {
		if entry.IsExpired() {
			stats.ExpiredEntries++
		}
	}
	for _, entry := range cache.obCache {
		if entry.IsExpired() {
			stats.ExpiredEntries++
		}
	}
	for _, entry := range cache.intentCache {
		if entry.IsExpired() {
			stats.ExpiredEntries++
		}
	}
	for _, entry := range cache.priceCache {
		if entry.IsExpired() {
			stats.ExpiredEntries++
		}
	}
	
	return stats
}

// CacheStats 缓存统计信息
type CacheStats struct {
	CVDEntries     int `json:"cvd_entries"`
	OBEntries      int `json:"ob_entries"`
	IntentEntries  int `json:"intent_entries"`
	PriceEntries   int `json:"price_entries"`
	TotalEntries   int `json:"total_entries"`
	ExpiredEntries int `json:"expired_entries"`
}

// ===== 全局缓存实例 =====

var globalCache *FiveMinuteDataCache
var cacheOnce sync.Once

// GetGlobalCache 获取全局缓存实例（单例模式）
func GetGlobalCache() *FiveMinuteDataCache {
	cacheOnce.Do(func() {
		globalCache = NewFiveMinuteDataCache()
		log.Println("✨ 全局5分钟数据缓存已初始化")
	})
	return globalCache
}

// StopGlobalCache 停止全局缓存
func StopGlobalCache() {
	if globalCache != nil {
		globalCache.Stop()
	}
}