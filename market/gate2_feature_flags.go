package market

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Gate2FeatureFlag Gate2功能特性标志
type Gate2FeatureFlag struct {
	// 全局开关
	Enabled bool `json:"enabled"`
	
	// 分模块开关
	AnchorEngine    bool `json:"anchor_engine"`
	StructClassifier bool `json:"struct_classifier"`
	TriggerDetector bool `json:"trigger_detector"`
	
	// 性能控制
	MaxAnchorsPerDirection int `json:"max_anchors_per_direction"`
	EnablePerformanceLog   bool `json:"enable_performance_log"`
	
	// 渐进式上线控制
	RolloutPercentage float64 `json:"rollout_percentage"` // 0-100
	WhitelistSymbols  []string `json:"whitelist_symbols"`
	BlacklistSymbols  []string `json:"blacklist_symbols"`
	
	// 降级开关
	FallbackOnError bool `json:"fallback_on_error"`
	MaxErrorRate    float64 `json:"max_error_rate"` // 0-1
	
	// 缓存控制
	EnableCaching   bool `json:"enable_caching"`
	CacheTTLSeconds int  `json:"cache_ttl_seconds"`
}

// Gate2FeatureManager 特性管理器
type Gate2FeatureManager struct {
	flags *Gate2FeatureFlag
	mutex sync.RWMutex
	
	// 错误率统计
	errorCount int64
	totalCount int64
	lastReset  time.Time
}

var (
	globalFeatureManager *Gate2FeatureManager
	once                 sync.Once
)

// GetGate2FeatureManager 获取全局特性管理器
func GetGate2FeatureManager() *Gate2FeatureManager {
	once.Do(func() {
		globalFeatureManager = NewGate2FeatureManager()
	})
	return globalFeatureManager
}

// NewGate2FeatureManager 创建特性管理器
func NewGate2FeatureManager() *Gate2FeatureManager {
	manager := &Gate2FeatureManager{
		flags:     loadDefaultFlags(),
		lastReset: time.Now(),
	}
	
	// 从环境变量加载配置
	manager.loadFromEnvironment()
	
	log.Printf("🚩 [Gate2FeatureFlags] 初始化完成: 全局开关=%v, 上线比例=%.1f%%", 
		manager.flags.Enabled, manager.flags.RolloutPercentage)
	
	return manager
}

// loadDefaultFlags 加载默认标志配置
func loadDefaultFlags() *Gate2FeatureFlag {
	return &Gate2FeatureFlag{
		// 默认关闭，需要显式启用
		Enabled:               false,
		AnchorEngine:         true,
		StructClassifier:     true,
		TriggerDetector:      true,
		
		// 性能控制
		MaxAnchorsPerDirection: 5,
		EnablePerformanceLog:   false,
		
		// 渐进式上线 - 默认0%
		RolloutPercentage:     0.0,
		WhitelistSymbols:      []string{},
		BlacklistSymbols:      []string{},
		
		// 降级配置
		FallbackOnError:       true,
		MaxErrorRate:         0.05, // 5%错误率阈值
		
		// 缓存配置
		EnableCaching:        true,
		CacheTTLSeconds:     300, // 5分钟
	}
}

// loadFromEnvironment 从环境变量加载配置
func (fm *Gate2FeatureManager) loadFromEnvironment() {
	fm.mutex.Lock()
	defer fm.mutex.Unlock()
	
	// 全局开关
	if val := os.Getenv("GATE2_ENABLED"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			fm.flags.Enabled = enabled
		}
	}
	
	// 分模块开关
	if val := os.Getenv("GATE2_ANCHOR_ENGINE"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			fm.flags.AnchorEngine = enabled
		}
	}
	
	if val := os.Getenv("GATE2_STRUCT_CLASSIFIER"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			fm.flags.StructClassifier = enabled
		}
	}
	
	if val := os.Getenv("GATE2_TRIGGER_DETECTOR"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			fm.flags.TriggerDetector = enabled
		}
	}
	
	// 上线比例
	if val := os.Getenv("GATE2_ROLLOUT_PERCENTAGE"); val != "" {
		if percentage, err := strconv.ParseFloat(val, 64); err == nil {
			fm.flags.RolloutPercentage = percentage
		}
	}
	
	// 白名单
	if val := os.Getenv("GATE2_WHITELIST_SYMBOLS"); val != "" {
		fm.flags.WhitelistSymbols = strings.Split(val, ",")
		for i, symbol := range fm.flags.WhitelistSymbols {
			fm.flags.WhitelistSymbols[i] = strings.TrimSpace(strings.ToUpper(symbol))
		}
	}
	
	// 黑名单
	if val := os.Getenv("GATE2_BLACKLIST_SYMBOLS"); val != "" {
		fm.flags.BlacklistSymbols = strings.Split(val, ",")
		for i, symbol := range fm.flags.BlacklistSymbols {
			fm.flags.BlacklistSymbols[i] = strings.TrimSpace(strings.ToUpper(symbol))
		}
	}
	
	// 性能日志
	if val := os.Getenv("GATE2_PERFORMANCE_LOG"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			fm.flags.EnablePerformanceLog = enabled
		}
	}
	
	// 缓存控制
	if val := os.Getenv("GATE2_ENABLE_CACHING"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			fm.flags.EnableCaching = enabled
		}
	}
	
	if val := os.Getenv("GATE2_CACHE_TTL_SECONDS"); val != "" {
		if ttl, err := strconv.Atoi(val); err == nil {
			fm.flags.CacheTTLSeconds = ttl
		}
	}
}

// IsEnabled 检查Gate2是否启用
func (fm *Gate2FeatureManager) IsEnabled(symbol string) bool {
	fm.mutex.RLock()
	defer fm.mutex.RUnlock()
	
	// 检查全局开关
	if !fm.flags.Enabled {
		return false
	}
	
	// 检查错误率限制
	if fm.isErrorRateExceeded() {
		log.Printf("🚩 [Gate2FeatureFlags] 错误率超标，自动禁用: symbol=%s", symbol)
		return false
	}
	
	// 检查黑名单
	symbol = strings.ToUpper(symbol)
	for _, blacklisted := range fm.flags.BlacklistSymbols {
		if symbol == blacklisted {
			return false
		}
	}
	
	// 检查白名单（如果存在白名单，只允许白名单内的symbol）
	if len(fm.flags.WhitelistSymbols) > 0 {
		whitelisted := false
		for _, allowed := range fm.flags.WhitelistSymbols {
			if symbol == allowed {
				whitelisted = true
				break
			}
		}
		if !whitelisted {
			return false
		}
	}
	
	// 检查渐进式上线比例
	if fm.flags.RolloutPercentage < 100.0 {
		// 使用symbol作为种子，确保同一symbol的结果一致
		hash := simpleHash(symbol)
		rolloutThreshold := fm.flags.RolloutPercentage / 100.0
		if float64(hash%1000)/1000.0 > rolloutThreshold {
			return false
		}
	}
	
	return true
}

// IsModuleEnabled 检查特定模块是否启用
func (fm *Gate2FeatureManager) IsModuleEnabled(module string) bool {
	fm.mutex.RLock()
	defer fm.mutex.RUnlock()
	
	switch module {
	case "anchor_engine":
		return fm.flags.AnchorEngine
	case "struct_classifier":
		return fm.flags.StructClassifier
	case "trigger_detector":
		return fm.flags.TriggerDetector
	default:
		return false
	}
}

// GetMaxAnchorsPerDirection 获取每个方向的最大锚点数量
func (fm *Gate2FeatureManager) GetMaxAnchorsPerDirection() int {
	fm.mutex.RLock()
	defer fm.mutex.RUnlock()
	return fm.flags.MaxAnchorsPerDirection
}

// IsPerformanceLogEnabled 检查性能日志是否启用
func (fm *Gate2FeatureManager) IsPerformanceLogEnabled() bool {
	fm.mutex.RLock()
	defer fm.mutex.RUnlock()
	return fm.flags.EnablePerformanceLog
}

// IsCachingEnabled 检查缓存是否启用
func (fm *Gate2FeatureManager) IsCachingEnabled() bool {
	fm.mutex.RLock()
	defer fm.mutex.RUnlock()
	return fm.flags.EnableCaching
}

// GetCacheTTL 获取缓存TTL
func (fm *Gate2FeatureManager) GetCacheTTL() time.Duration {
	fm.mutex.RLock()
	defer fm.mutex.RUnlock()
	return time.Duration(fm.flags.CacheTTLSeconds) * time.Second
}

// RecordSuccess 记录成功执行
func (fm *Gate2FeatureManager) RecordSuccess() {
	fm.mutex.Lock()
	defer fm.mutex.Unlock()
	
	fm.totalCount++
	fm.resetStatsIfNeeded()
}

// RecordError 记录错误执行
func (fm *Gate2FeatureManager) RecordError(err error, symbol string) {
	fm.mutex.Lock()
	defer fm.mutex.Unlock()
	
	fm.errorCount++
	fm.totalCount++
	fm.resetStatsIfNeeded()
	
	log.Printf("🚩 [Gate2FeatureFlags] 记录错误: symbol=%s, error=%v, 当前错误率=%.2f%%", 
		symbol, err, fm.getCurrentErrorRate()*100)
}

// isErrorRateExceeded 检查错误率是否超标
func (fm *Gate2FeatureManager) isErrorRateExceeded() bool {
	if fm.totalCount == 0 {
		return false
	}
	
	currentErrorRate := fm.getCurrentErrorRate()
	return currentErrorRate > fm.flags.MaxErrorRate
}

// getCurrentErrorRate 获取当前错误率
func (fm *Gate2FeatureManager) getCurrentErrorRate() float64 {
	if fm.totalCount == 0 {
		return 0.0
	}
	return float64(fm.errorCount) / float64(fm.totalCount)
}

// resetStatsIfNeeded 定期重置统计数据
func (fm *Gate2FeatureManager) resetStatsIfNeeded() {
	// 每小时重置一次统计数据
	if time.Since(fm.lastReset) > time.Hour {
		fm.errorCount = 0
		fm.totalCount = 0
		fm.lastReset = time.Now()
		log.Printf("🚩 [Gate2FeatureFlags] 统计数据已重置")
	}
}

// UpdateFlags 动态更新标志（热更新）
func (fm *Gate2FeatureManager) UpdateFlags(newFlags *Gate2FeatureFlag) {
	fm.mutex.Lock()
	defer fm.mutex.Unlock()
	
	oldEnabled := fm.flags.Enabled
	oldRollout := fm.flags.RolloutPercentage
	
	fm.flags = newFlags
	
	log.Printf("🚩 [Gate2FeatureFlags] 配置已更新: 全局开关 %v->%v, 上线比例 %.1f%%->%.1f%%", 
		oldEnabled, newFlags.Enabled, oldRollout, newFlags.RolloutPercentage)
}

// GetCurrentFlags 获取当前标志配置（只读副本）
func (fm *Gate2FeatureManager) GetCurrentFlags() Gate2FeatureFlag {
	fm.mutex.RLock()
	defer fm.mutex.RUnlock()
	
	// 返回副本，避免外部修改
	return *fm.flags
}

// simpleHash 简单哈希函数
func simpleHash(s string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		hash ^= uint32(s[i])
		hash *= 16777619
	}
	return hash
}

// LogFeatureStatus 记录特性状态
func (fm *Gate2FeatureManager) LogFeatureStatus(symbol string) {
	enabled := fm.IsEnabled(symbol)
	flags := fm.GetCurrentFlags()
	
	log.Printf("🚩 [Gate2FeatureFlags] 状态检查: symbol=%s, enabled=%v, 上线比例=%.1f%%, 错误率=%.2f%%", 
		symbol, enabled, flags.RolloutPercentage, fm.getCurrentErrorRate()*100)
}