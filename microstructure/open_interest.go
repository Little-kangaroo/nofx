package microstructure

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"
)

// ===== Open Interest 相关结构 =====

// OIData Open Interest数据
type OIData struct {
	Symbol           string    `json:"symbol"`
	OpenInterest     float64   `json:"open_interest"`     // 当前持仓量
	Timestamp        time.Time `json:"timestamp"`         // 更新时间
}

// OIChange 持仓量变化记录
type OIChange struct {
	Timestamp    time.Time `json:"timestamp"`
	OpenInterest float64   `json:"open_interest"`
	Change       float64   `json:"change"`       // 相对于上一次的变化
	ChangeRate   float64   `json:"change_rate"`  // 变化率（百分比）
}

// OIAnalysis 持仓量分析结果
type OIAnalysis struct {
	Current          float64   `json:"current"`           // 当前持仓量
	Change1H         float64   `json:"change_1h"`         // 1小时变化量
	Change4H         float64   `json:"change_4h"`         // 4小时变化量
	ChangeRate1H     float64   `json:"change_rate_1h"`    // 1小时变化率
	ChangeRate4H     float64   `json:"change_rate_4h"`    // 4小时变化率
	Trend            string    `json:"trend"`             // 趋势：increasing/decreasing/stable
	LastUpdate       time.Time `json:"last_update"`       // 最后更新时间
	IsStale          bool      `json:"is_stale"`          // 数据是否过期
}

// OICalculator Open Interest计算器
type OICalculator struct {
	mu               sync.RWMutex
	symbol           string
	changes          []OIChange        // 持仓量变化历史
	current          float64           // 当前持仓量
	lastUpdate       time.Time         // 最后更新时间
	windowDuration   time.Duration     // 数据窗口大小
	maxRecords       int               // 最大记录数
}

// ===== WebSocket消息结构 =====

// BinanceOIMsg 币安Open Interest消息
type BinanceOIMsg struct {
	EventType     string `json:"e"`  // 事件类型
	EventTime     int64  `json:"E"`  // 事件时间
	Symbol        string `json:"s"`  // 交易对
	OpenInterest  string `json:"o"`  // 持仓量
	Time          int64  `json:"T"`  // 统计时间
}

// NewOICalculator 创建OI计算器
func NewOICalculator(symbol string, windowDuration time.Duration) *OICalculator {
	return &OICalculator{
		symbol:         symbol,
		changes:        make([]OIChange, 0, 1440), // 预分配24小时的容量（假设每分钟1次更新）
		windowDuration: windowDuration,
		maxRecords:     1440, // 保留24小时的记录
		lastUpdate:     time.Now(),
	}
}

// ProcessOIData 处理持仓量数据
func (calc *OICalculator) ProcessOIData(oiData *OIData) {
	calc.mu.Lock()
	defer calc.mu.Unlock()

	// 计算变化量
	var change float64
	var changeRate float64
	
	if calc.current > 0 {
		change = oiData.OpenInterest - calc.current
		changeRate = change / calc.current * 100
	}

	// 更新当前持仓量
	calc.current = oiData.OpenInterest

	// 记录变化
	oiChange := OIChange{
		Timestamp:    oiData.Timestamp,
		OpenInterest: oiData.OpenInterest,
		Change:       change,
		ChangeRate:   changeRate,
	}

	calc.changes = append(calc.changes, oiChange)
	calc.lastUpdate = oiData.Timestamp

	// 清理过期数据
	calc.cleanupExpiredData()

	log.Printf("📊 [%s] OI更新: %.0f (变化: %+.0f, %+.2f%%)", 
		calc.symbol, oiData.OpenInterest, change, changeRate)
}

// cleanupExpiredData 清理过期数据
func (calc *OICalculator) cleanupExpiredData() {
	cutoffTime := time.Now().Add(-calc.windowDuration)
	
	// 找到第一个有效记录的位置
	validStart := -1 // 初始化为-1，表示没有找到有效记录
	for i, change := range calc.changes {
		if change.Timestamp.After(cutoffTime) {
			validStart = i
			break
		}
	}

	// 根据找到的有效记录位置进行清理
	if validStart > 0 {
		// 有部分记录有效，从validStart开始保留
		calc.changes = calc.changes[validStart:]
	} else if validStart == -1 {
		// 所有记录都过期，清空数组
		calc.changes = calc.changes[:0]
	}
	// validStart == 0 表示所有记录都有效，不需要清理

	// 限制最大记录数
	if len(calc.changes) > calc.maxRecords {
		calc.changes = calc.changes[len(calc.changes)-calc.maxRecords:]
	}
}

// GetOIAnalysis 获取持仓量分析结果
func (calc *OICalculator) GetOIAnalysis() *OIAnalysis {
	calc.mu.RLock()
	defer calc.mu.RUnlock()

	if len(calc.changes) == 0 {
		// 检查数据是否过期 - 即使没有历史变化数据，也要基于lastUpdate判断
		isStale := time.Since(calc.lastUpdate) > 30*time.Minute
		return &OIAnalysis{
			Current:     calc.current,
			Change1H:    0,
			Change4H:    0,
			ChangeRate1H: 0,
			ChangeRate4H: 0,
			Trend:       "no_data",
			LastUpdate:  calc.lastUpdate,
			IsStale:     isStale,
		}
	}

	// 计算1小时和4小时的变化
	change1H, changeRate1H := calc.calculateChange(time.Hour)
	change4H, changeRate4H := calc.calculateChange(4 * time.Hour)

	// 判断趋势
	trend := calc.determineTrend(changeRate1H)

	// 检查数据是否过期 - 调整为30分钟阈值，给数据更新留足时间
	isStale := time.Since(calc.lastUpdate) > 30*time.Minute

	return &OIAnalysis{
		Current:      calc.current,
		Change1H:     change1H,
		Change4H:     change4H,
		ChangeRate1H: changeRate1H,
		ChangeRate4H: changeRate4H,
		Trend:        trend,
		LastUpdate:   calc.lastUpdate,
		IsStale:      isStale,
	}
}

// calculateChange 计算指定时间窗口的变化
func (calc *OICalculator) calculateChange(duration time.Duration) (change, changeRate float64) {
	if len(calc.changes) == 0 {
		return 0, 0
	}

	cutoffTime := time.Now().Add(-duration)
	
	// 找到时间窗口开始时的OI值
	var startOI float64
	found := false
	
	for _, record := range calc.changes {
		if record.Timestamp.After(cutoffTime) {
			startOI = record.OpenInterest
			found = true
			break
		}
	}

	if !found && len(calc.changes) > 0 {
		// 如果没找到精确时间点，使用最早的记录
		startOI = calc.changes[0].OpenInterest
	}

	if startOI > 0 {
		change = calc.current - startOI
		changeRate = change / startOI * 100
	}

	return change, changeRate
}

// determineTrend 判断趋势
func (calc *OICalculator) determineTrend(changeRate1H float64) string {
	threshold := 0.5 // 0.5%的阈值

	if changeRate1H > threshold {
		return "increasing"
	} else if changeRate1H < -threshold {
		return "decreasing" 
	} else {
		return "stable"
	}
}

// GetStatistics 获取统计信息
func (calc *OICalculator) GetStatistics() map[string]interface{} {
	calc.mu.RLock()
	defer calc.mu.RUnlock()

	return map[string]interface{}{
		"symbol":         calc.symbol,
		"current_oi":     calc.current,
		"records_count":  len(calc.changes),
		"window_hours":   calc.windowDuration.Hours(),
		"last_update":    calc.lastUpdate,
	}
}

// ===== OI管理器 =====

// OIManager Open Interest管理器
type OIManager struct {
	calculators map[string]*OICalculator // key: symbol
	config      *MicrostructureConfig
	mu          sync.RWMutex
}

// NewOIManager 创建OI管理器
func NewOIManager(config *MicrostructureConfig) *OIManager {
	return &OIManager{
		calculators: make(map[string]*OICalculator),
		config:      config,
	}
}

// GetOrCreateCalculator 获取或创建币种的OI计算器
func (manager *OIManager) GetOrCreateCalculator(symbol string) *OICalculator {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if calc, exists := manager.calculators[symbol]; exists {
		return calc
	}

	// 创建新的计算器
	calc := NewOICalculator(symbol, 24*time.Hour) // 保留24小时数据
	manager.calculators[symbol] = calc
	
	log.Printf("✨ 创建OI计算器: %s", symbol)
	return calc
}

// ProcessOIMessage 处理OI WebSocket消息
func (manager *OIManager) ProcessOIMessage(message []byte) error {
	var msg BinanceOIMsg
	if err := json.Unmarshal(message, &msg); err != nil {
		return err
	}

	// 解析持仓量
	oi, err := strconv.ParseFloat(msg.OpenInterest, 64)
	if err != nil {
		return err
	}

	// 创建OI数据
	oiData := &OIData{
		Symbol:       msg.Symbol,
		OpenInterest: oi,
		Timestamp:    time.Unix(0, msg.EventTime*int64(time.Millisecond)),
	}

	// 处理数据
	calc := manager.GetOrCreateCalculator(msg.Symbol)
	calc.ProcessOIData(oiData)

	return nil
}

// ProcessOIData 处理来自API的OI数据
func (manager *OIManager) ProcessOIData(symbol string, oiData *OIData) {
	calc := manager.GetOrCreateCalculator(symbol)
	calc.ProcessOIData(oiData)
}

// GetOIAnalysis 获取指定币种的OI分析
func (manager *OIManager) GetOIAnalysis(symbol string) *OIAnalysis {
	manager.mu.RLock()
	calc, exists := manager.calculators[symbol]
	manager.mu.RUnlock()

	if !exists {
		return &OIAnalysis{
			Current:      0,
			Change1H:     0,
			Change4H:     0,
			ChangeRate1H: 0,
			ChangeRate4H: 0,
			Trend:        "no_data",
			LastUpdate:   time.Now(),
			IsStale:      true,
		}
	}

	return calc.GetOIAnalysis()
}

// GetAllOIAnalysis 获取所有币种的OI分析
func (manager *OIManager) GetAllOIAnalysis() map[string]*OIAnalysis {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	result := make(map[string]*OIAnalysis)
	for symbol, calc := range manager.calculators {
		result[symbol] = calc.GetOIAnalysis()
	}

	return result
}

// Cleanup 清理过期数据（不删除计算器本身）
func (manager *OIManager) Cleanup() {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	log.Printf("🧹 开始清理OI过期数据...")
	cleanedCount := 0
	
	for symbol, calc := range manager.calculators {
		calc.mu.Lock()
		beforeChanges := len(calc.changes)
		
		// 调用内部清理方法
		calc.cleanupExpiredData()
		
		afterChanges := len(calc.changes)
		calc.mu.Unlock()
		
		// 记录清理情况
		if beforeChanges != afterChanges {
			log.Printf("🗑️  [%s] OI数据清理: 变化记录 %d→%d", 
				symbol, beforeChanges, afterChanges)
			cleanedCount++
		}
	}
	
	log.Printf("✅ OI数据清理完成，清理了 %d 个币种的过期数据", cleanedCount)
}

// ===== WebSocket扩展支持OI流 =====

