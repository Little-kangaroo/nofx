package market

import (
	"log"
	"math"
	"time"
)

// ATRManager 统一ATR管理器 - P0-3修复：建立统一ATR管理机制
// 解决各模块ATR计算不一致、fallback混乱的根本性问题
type ATRManager struct {
	cache map[string]*ATREntry // timeframe -> ATR值缓存
}

// ATREntry ATR缓存条目
type ATREntry struct {
	Value      float64 // ATR值
	Mode       string  // "normal" | "fallback_pct" | "fallback_static"
	Confidence float64 // 置信度 0-1
	Timestamp  int64   // 计算时间戳
}

// NewATRManager 创建ATR管理器
func NewATRManager() *ATRManager {
	return &ATRManager{
		cache: make(map[string]*ATREntry),
	}
}

// GetATR14 获取14周期ATR，带统一fallback机制
// 🔥 P0-3核心修复：统一ATR计算和fallback策略
func (am *ATRManager) GetATR14(klines []Kline, timeframe string, currentPrice float64) *ATREntry {
	// 检查缓存
	if cached, exists := am.cache[timeframe]; exists {
		return cached
	}

	entry := &ATREntry{}

	// 尝试标准ATR计算
	if len(klines) >= 14 {
		atr := am.calculateATR14Standard(klines)
		
		// 合理性检查
		if am.isATRValid(atr, currentPrice) {
			entry.Value = atr
			entry.Mode = "normal"
			entry.Confidence = 1.0
		} else {
			// ATR不合理，使用价格百分比fallback
			entry.Value = currentPrice * am.getTimeframeFallbackRate(timeframe)
			entry.Mode = "fallback_pct"
			entry.Confidence = 0.7
			log.Printf("⚠️ [ATR Manager] %s时间框架ATR异常(%.4f)，使用价格百分比fallback: %.4f", 
				timeframe, atr, entry.Value)
		}
	} else {
		// 数据不足，使用时间框架相关的fallback
		entry.Value = currentPrice * am.getTimeframeFallbackRate(timeframe)
		entry.Mode = "fallback_pct"
		entry.Confidence = 0.5
		log.Printf("⚠️ [ATR Manager] %s时间框架数据不足(%d<14)，使用价格百分比fallback: %.4f", 
			timeframe, len(klines), entry.Value)
	}
	
	entry.Timestamp = getCurrentTimestamp()
	am.cache[timeframe] = entry
	
	return entry
}

// calculateATR14Standard 标准14周期ATR计算
func (am *ATRManager) calculateATR14Standard(klines []Kline) float64 {
	if len(klines) < 14 {
		return 0
	}
	
	var trs []float64
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close
		
		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)
		
		tr := math.Max(tr1, math.Max(tr2, tr3))
		trs = append(trs, tr)
	}
	
	// 计算最后14个TR的平均值
	start := len(trs) - 14
	if start < 0 {
		start = 0
	}
	
	sum := 0.0
	for i := start; i < len(trs); i++ {
		sum += trs[i]
	}
	
	return sum / 14.0
}

// isATRValid 检查ATR是否合理
func (am *ATRManager) isATRValid(atr, currentPrice float64) bool {
	if atr <= 0 || math.IsNaN(atr) || math.IsInf(atr, 0) {
		return false
	}
	
	// ATR应该在价格的0.05%-5%之间
	minATR := currentPrice * 0.0005
	maxATR := currentPrice * 0.05
	
	return atr >= minATR && atr <= maxATR
}

// getTimeframeFallbackRate 获取时间框架相关的fallback费率
func (am *ATRManager) getTimeframeFallbackRate(timeframe string) float64 {
	switch timeframe {
	case "5m":
		return 0.002  // 0.2%
	case "15m":
		return 0.003  // 0.3%
	case "30m":
		return 0.004  // 0.4%
	case "1h":
		return 0.005  // 0.5%
	case "4h":
		return 0.008  // 0.8%
	default:
		return 0.005  // 默认0.5%
	}
}

// GetDistanceATR 计算ATR归一化距离，统一接口
func (am *ATRManager) GetDistanceATR(level, currentPrice float64, timeframe string, klines []Kline) (float64, string) {
	atrEntry := am.GetATR14(klines, timeframe, currentPrice)
	
	distance := math.Abs(level - currentPrice) / atrEntry.Value
	
	// 根据ATR模式标注距离类型
	var distanceMode string
	switch atrEntry.Mode {
	case "normal":
		distanceMode = "atr"
	case "fallback_pct":
		distanceMode = "pct_fallback"
	case "fallback_static":
		distanceMode = "static_fallback"
	default:
		distanceMode = "unknown"
	}
	
	return distance, distanceMode
}

// ClearCache 清理缓存
func (am *ATRManager) ClearCache() {
	am.cache = make(map[string]*ATREntry)
}

// GetCacheStatus 获取缓存状态
func (am *ATRManager) GetCacheStatus() map[string]*ATREntry {
	result := make(map[string]*ATREntry)
	for tf, entry := range am.cache {
		result[tf] = &ATREntry{
			Value:      entry.Value,
			Mode:       entry.Mode,
			Confidence: entry.Confidence,
			Timestamp:  entry.Timestamp,
		}
	}
	return result
}

// getCurrentTimestamp 获取当前时间戳
func getCurrentTimestamp() int64 {
	return time.Now().UnixMilli()
}

// 全局ATR管理器实例
var globalATRManager = NewATRManager()

// GetGlobalATRManager 获取全局ATR管理器
func GetGlobalATRManager() *ATRManager {
	return globalATRManager
}