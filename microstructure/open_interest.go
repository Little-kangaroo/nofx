package microstructure

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ===== Open Interest 相关结构 =====

// OIData Open Interest数据（V-12.3 P0-05修复版本 - 双时间戳支持）
type OIData struct {
	Symbol           string    `json:"symbol"`
	OpenInterest     float64   `json:"open_interest"`     // 当前持仓量
	Timestamp        time.Time `json:"timestamp"`         // 🔥 P0-05修复：接收/处理时间（用于健康度检查）
	ExchangeTime     time.Time `json:"exchange_time"`     // 🔥 P0-05修复：交易所原始时间（用于事件对齐）
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

	// 🔧 修复: 增加数据验证和异常处理
	if err := calc.validateOIData(oiData); err != nil {
		log.Printf("❌ OI数据验证失败 %s: %v", calc.symbol, err)
		return
	}

	// 计算变化量
	var change float64
	var changeRate float64
	
	if calc.current > 0 {
		change = oiData.OpenInterest - calc.current
		changeRate = change / calc.current * 100
		
		// 🔧 修复: 检测异常变化
		if err := calc.detectAnomalousChange(change, changeRate); err != nil {
			log.Printf("⚠️ 检测到异常OI变化 %s: %v, 当前=%.0f, 新值=%.0f", 
				calc.symbol, err, calc.current, oiData.OpenInterest)
			// 不直接拒绝数据，而是标记为可疑并继续处理
		}
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
		isStale := time.Since(calc.lastUpdate) > 5*time.Minute // 🔧 修复：OI数据每30秒更新，5分钟无更新标记过期
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
	isStale := time.Since(calc.lastUpdate) > 5*time.Minute // 🔧 修复：OI数据每30秒更新，5分钟无更新标记过期

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
	// 🔧 修复: 增加全局数据验证
	if err := manager.validateOIDataGlobal(symbol, oiData); err != nil {
		log.Printf("❌ OI管理器数据验证失败 %s: %v", symbol, err)
		return
	}
	
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

// ===== 🔧 修复: OI数据验证和异常处理方法 =====

// validateOIData 验证单个OI数据的有效性
func (calc *OICalculator) validateOIData(oiData *OIData) error {
	if oiData == nil {
		return fmt.Errorf("OI数据为nil")
	}
	
	if oiData.Symbol != calc.symbol {
		return fmt.Errorf("交易对不匹配: 期望%s, 实际%s", calc.symbol, oiData.Symbol)
	}
	
	if oiData.OpenInterest < 0 {
		return fmt.Errorf("持仓量不能为负数: %.2f", oiData.OpenInterest)
	}
	
	// 检查是否为异常大的数值（可能是API错误）
	if oiData.OpenInterest > 1e12 { // 1万亿 - 不合理的大值
		return fmt.Errorf("持仓量异常过大: %.2f", oiData.OpenInterest)
	}
	
	if oiData.Timestamp.IsZero() {
		return fmt.Errorf("时间戳无效")
	}
	
	// 检查时间戳是否过于陈旧或未来时间
	now := time.Now()
	if oiData.Timestamp.After(now.Add(5*time.Minute)) {
		return fmt.Errorf("时间戳过于未来: %v", oiData.Timestamp)
	}
	
	if oiData.Timestamp.Before(now.Add(-24*time.Hour)) {
		return fmt.Errorf("时间戳过于陈旧: %v", oiData.Timestamp)
	}
	
	return nil
}

// detectAnomalousChange 检测异常的OI变化（🔧 修复：优化异常检测阈值）
func (calc *OICalculator) detectAnomalousChange(change, changeRate float64) error {
	// 🔧 修复：使用动态阈值，基于历史波动性调整
	dynamicThreshold := calc.calculateDynamicThreshold()
	
	// 🔧 修复：检测异常大的变化率，使用动态阈值
	if math.Abs(changeRate) > dynamicThreshold {
		// 如果变化率极端高（超过100%），直接认为异常
		if math.Abs(changeRate) > 100 {
			return fmt.Errorf("变化率极度异常: %.2f%% (阈值: %.1f%%)", changeRate, dynamicThreshold)
		}
		
		// 中等异常（50%-100%），记录警告但不拒绝数据
		log.Printf("⚠️ [OI异常检测] 变化率较高但可接受: %.2f%% (动态阈值: %.1f%%)", 
			changeRate, dynamicThreshold)
	}
	
	// 🔧 修复：检测异常大的绝对变化，使用更宽松的阈值
	// 只有在持仓量变化超过50%且绝对值很大时才认为异常
	absoluteThreshold := math.Max(calc.current*0.5, 1000000) // 50%或100万，取较大值
	if math.Abs(change) > absoluteThreshold {
		return fmt.Errorf("绝对变化异常: %.2f (当前持仓: %.2f, 阈值: %.2f)", 
			change, calc.current, absoluteThreshold)
	}
	
	return nil
}

// 🔧 修复：计算动态异常检测阈值
func (calc *OICalculator) calculateDynamicThreshold() float64 {
	if len(calc.changes) < 5 {
		return 80.0 // 数据不足时使用较宽松的80%阈值
	}
	
	// 计算历史变化率的标准差
	var changeRates []float64
	for _, change := range calc.changes {
		// 🔧 修复：使用OIChange结构的实际字段
		changeRates = append(changeRates, math.Abs(change.ChangeRate))
	}
	
	if len(changeRates) < 3 {
		return 80.0 // 数据不足
	}
	
	// 计算变化率的平均值和标准差
	var sum, mean, variance float64
	for _, rate := range changeRates {
		sum += rate
	}
	mean = sum / float64(len(changeRates))
	
	for _, rate := range changeRates {
		diff := rate - mean
		variance += diff * diff
	}
	variance /= float64(len(changeRates))
	stdDev := math.Sqrt(variance)
	
	// 动态阈值 = 平均值 + 3倍标准差，但限制在合理范围内
	dynamicThreshold := mean + 3*stdDev
	
	// 限制阈值范围：最小60%，最大150%
	return math.Max(60.0, math.Min(150.0, dynamicThreshold))
}

// validateOIDataGlobal OI管理器全局数据验证
func (manager *OIManager) validateOIDataGlobal(symbol string, oiData *OIData) error {
	if symbol == "" {
		return fmt.Errorf("交易对符号为空")
	}
	
	if oiData == nil {
		return fmt.Errorf("OI数据为nil")
	}
	
	// 检查是否为已知的交易对
	if !manager.isValidSymbol(symbol) {
		return fmt.Errorf("未识别的交易对: %s", symbol)
	}
	
	return nil
}

// isValidSymbol 检查是否为有效的交易对
func (manager *OIManager) isValidSymbol(symbol string) bool {
	// 简单的交易对格式验证
	if len(symbol) < 6 {
		return false
	}
	
	// 检查是否以USDT结尾（简化验证）
	if !strings.HasSuffix(strings.ToUpper(symbol), "USDT") {
		return false
	}
	
	return true
}

// GetOI5MinuteChangeRate 获取指定币种的5分钟OI变化率（P0-03修复专用方法）
func (manager *OIManager) GetOI5MinuteChangeRate(symbol string) float64 {
	manager.mu.RLock()
	calc, exists := manager.calculators[symbol]
	manager.mu.RUnlock()

	if !exists {
		return 0.0 // 币种不存在，返回0变化率
	}

	// 使用现有的calculateChange方法计算5分钟变化率
	_, changeRate := calc.calculateChange(5 * time.Minute)
	return changeRate
}

// ===== WebSocket扩展支持OI流 =====

