package microstructure

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// OrderBookMonitor 订单簿监控器（V2.0）
type OrderBookMonitor struct {
	mu                sync.RWMutex
	symbol            string
	calculator        *OrderBookCalculator
	wallTracker       *WallStabilityTracker
	alertThresholds   *AlertThresholds
	alertCallback     AlertCallback
	monitoringActive  bool
	stopChannel       chan bool
	updateInterval    time.Duration
	lastAlert         time.Time
	alertCooldown     time.Duration
}

// AlertCallback 警报回调函数类型
type AlertCallback func(alert *OrderBookAlert)

// AlertThresholds 警报阈值配置
type AlertThresholds struct {
	SpoofingRisk          float64 `json:"spoofing_risk"`           // 虚假挂单风险阈值
	WallInstability       float64 `json:"wall_instability"`       // 墙不稳定阈值
	ImbalanceExtreme      float64 `json:"imbalance_extreme"`       // 极端失衡阈值
	LiquidityDrop         float64 `json:"liquidity_drop"`          // 流动性下降阈值
	WallChangeFrequency   int     `json:"wall_change_frequency"`   // 墙变化频率阈值(每5分钟)
	LargeOrderDetection   float64 `json:"large_order_detection"`   // 大单检测阈值
	MarketManipulation    float64 `json:"market_manipulation"`     // 市场操控检测阈值
}

// DefaultAlertThresholds 默认警报阈值
func DefaultAlertThresholds() *AlertThresholds {
	return &AlertThresholds{
		SpoofingRisk:        0.7,
		WallInstability:     0.3,
		ImbalanceExtreme:    0.8,
		LiquidityDrop:       0.3,
		WallChangeFrequency: 15,
		LargeOrderDetection: 1000000, // 100万USD
		MarketManipulation:  0.8,
	}
}

// OrderBookAlert 订单簿警报
type OrderBookAlert struct {
	Symbol      string                 `json:"symbol"`
	AlertType   string                 `json:"alert_type"`
	Severity    string                 `json:"severity"`
	Message     string                 `json:"message"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
	Suggestions []string               `json:"suggestions"`
}

// WallStabilityTracker 墙稳定性追踪器（增强版）
type WallStabilityTracker struct {
	mu                  sync.RWMutex
	symbol              string
	wallSnapshots       []*WallSnapshot     // 墙快照历史
	manipulationEvents  []ManipulationEvent // 操控事件记录
	maxSnapshotHistory  int                 // 最大快照历史
	analysisWindow      time.Duration       // 分析窗口
}

// WallSnapshot 墙快照
type WallSnapshot struct {
	Timestamp         time.Time              `json:"timestamp"`
	ResistanceWalls   []*WallInfo            `json:"resistance_walls"`
	SupportWalls      []*WallInfo            `json:"support_walls"`
	ImbalanceRatio    float64                `json:"imbalance_ratio"`
	SpoofingRisk      float64                `json:"spoofing_risk"`
	MarketMetrics     map[string]interface{} `json:"market_metrics"`
}

// ManipulationEvent 操控事件
type ManipulationEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	EventType   string    `json:"event_type"`
	Severity    float64   `json:"severity"`
	Description string    `json:"description"`
	Evidence    map[string]interface{} `json:"evidence"`
}

// NewOrderBookMonitor 创建订单簿监控器
func NewOrderBookMonitor(symbol string, calculator *OrderBookCalculator, alertCallback AlertCallback) *OrderBookMonitor {
	monitor := &OrderBookMonitor{
		symbol:           symbol,
		calculator:       calculator,
		wallTracker:      NewWallStabilityTracker(symbol),
		alertThresholds:  DefaultAlertThresholds(),
		alertCallback:    alertCallback,
		stopChannel:      make(chan bool),
		updateInterval:   5 * time.Second, // 5秒更新一次
		alertCooldown:    30 * time.Second, // 30秒冷却时间
	}
	
	return monitor
}

// NewWallStabilityTracker 创建墙稳定性追踪器
func NewWallStabilityTracker(symbol string) *WallStabilityTracker {
	return &WallStabilityTracker{
		symbol:             symbol,
		wallSnapshots:      make([]*WallSnapshot, 0, 1000),
		manipulationEvents: make([]ManipulationEvent, 0, 100),
		maxSnapshotHistory: 1000,
		analysisWindow:     30 * time.Minute,
	}
}

// StartMonitoring 启动监控
func (monitor *OrderBookMonitor) StartMonitoring() {
	monitor.mu.Lock()
	if monitor.monitoringActive {
		monitor.mu.Unlock()
		return
	}
	monitor.monitoringActive = true
	monitor.mu.Unlock()
	
	go monitor.monitoringLoop()
	log.Printf("🔍 [%s] 订单簿监控已启动", monitor.symbol)
}

// StopMonitoring 停止监控
func (monitor *OrderBookMonitor) StopMonitoring() {
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	
	if !monitor.monitoringActive {
		return
	}
	
	monitor.monitoringActive = false
	close(monitor.stopChannel)
	log.Printf("🛑 [%s] 订单簿监控已停止", monitor.symbol)
}

// monitoringLoop 监控循环
func (monitor *OrderBookMonitor) monitoringLoop() {
	ticker := time.NewTicker(monitor.updateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			monitor.performMonitoringCheck()
		case <-monitor.stopChannel:
			return
		}
	}
}

// performMonitoringCheck 执行监控检查
func (monitor *OrderBookMonitor) performMonitoringCheck() {
	// 获取当前盘口数据
	orderBookData := monitor.calculator.GetCurrentOrderBookData(5)
	if orderBookData == nil || orderBookData.IsStale {
		return
	}
	
	// 创建墙快照
	snapshot := monitor.createWallSnapshot(orderBookData)
	
	// 添加到历史记录
	monitor.wallTracker.AddSnapshot(snapshot)
	
	// 执行各种检查
	monitor.checkSpoofingRisk(orderBookData)
	monitor.checkWallStability(orderBookData)
	monitor.checkImbalanceExtremes(orderBookData)
	monitor.checkLiquidityDrop(orderBookData)
	monitor.checkWallChangeFrequency(orderBookData)
	monitor.checkLargeOrders(orderBookData)
	monitor.checkMarketManipulation(orderBookData, snapshot)
}

// createWallSnapshot 创建墙快照
func (monitor *OrderBookMonitor) createWallSnapshot(data *OrderBookData) *WallSnapshot {
	snapshot := &WallSnapshot{
		Timestamp:      time.Now(),
		ImbalanceRatio: data.ImbalanceRatio,
		SpoofingRisk:   data.SpoofingRisk,
		MarketMetrics:  make(map[string]interface{}),
	}
	
	// 记录阻力墙
	if data.NearestResistance != nil {
		snapshot.ResistanceWalls = []*WallInfo{data.NearestResistance}
	}
	
	// 记录支撑墙
	if data.NearestSupport != nil {
		snapshot.SupportWalls = []*WallInfo{data.NearestSupport}
	}
	
	// 记录市场指标
	snapshot.MarketMetrics["bid_pressure"] = data.BidPressure
	snapshot.MarketMetrics["ask_pressure"] = data.AskPressure
	snapshot.MarketMetrics["liquidity_score"] = data.LiquidityScore
	snapshot.MarketMetrics["wall_change_count_5m"] = data.WallChangeCount5m
	
	return snapshot
}

// checkSpoofingRisk 检查虚假挂单风险
func (monitor *OrderBookMonitor) checkSpoofingRisk(data *OrderBookData) {
	if data.SpoofingRisk > monitor.alertThresholds.SpoofingRisk {
		// 冷却期检查
		if time.Since(monitor.lastAlert) < monitor.alertCooldown {
			return
		}
		
		severity := "medium"
		if data.SpoofingRisk > 0.9 {
			severity = "high"
		}
		
		alert := &OrderBookAlert{
			Symbol:    monitor.symbol,
			AlertType: "spoofing_risk",
			Severity:  severity,
			Message:   "检测到虚假挂单风险",
			Data: map[string]interface{}{
				"spoofing_risk":        data.SpoofingRisk,
				"wall_change_count_5m": data.WallChangeCount5m,
				"threshold":            monitor.alertThresholds.SpoofingRisk,
			},
			Timestamp: time.Now(),
			Suggestions: []string{
				"谨慎对待大额挂单墙",
				"关注墙体的稳定性评分",
				"等待墙体稳定后再执行交易",
			},
		}
		
		monitor.sendAlert(alert)
	}
}

// checkWallStability 检查墙稳定性
func (monitor *OrderBookMonitor) checkWallStability(data *OrderBookData) {
	checkWall := func(wall *WallInfo, wallType string) {
		if wall != nil && wall.StabilityScore < monitor.alertThresholds.WallInstability {
			alert := &OrderBookAlert{
				Symbol:    monitor.symbol,
				AlertType: "wall_instability",
				Severity:  "medium",
				Message:   fmt.Sprintf("%s墙稳定性较低", wallType),
				Data: map[string]interface{}{
					"wall_type":        wallType,
					"stability_score":  wall.StabilityScore,
					"flicker_count":    wall.FlickerCount,
					"existence_minutes": wall.ExistenceDuration.Minutes(),
					"threshold":        monitor.alertThresholds.WallInstability,
				},
				Timestamp: time.Now(),
				Suggestions: []string{
					"避免依赖此墙体进行交易决策",
					"等待更稳定的支撑/阻力形成",
					"考虑降低仓位规模",
				},
			}
			
			monitor.sendAlert(alert)
		}
	}
	
	checkWall(data.NearestResistance, "阻力")
	checkWall(data.NearestSupport, "支撑")
}

// checkImbalanceExtremes 检查极端失衡
func (monitor *OrderBookMonitor) checkImbalanceExtremes(data *OrderBookData) {
	if math.Abs(data.ImbalanceRatio) > monitor.alertThresholds.ImbalanceExtreme {
		side := "买方"
		if data.ImbalanceRatio < 0 {
			side = "卖方"
		}
		
		alert := &OrderBookAlert{
			Symbol:    monitor.symbol,
			AlertType: "extreme_imbalance",
			Severity:  "high",
			Message:   fmt.Sprintf("检测到%s极端失衡", side),
			Data: map[string]interface{}{
				"imbalance_ratio": data.ImbalanceRatio,
				"imbalance_trend": data.ImbalanceTrend,
				"threshold":       monitor.alertThresholds.ImbalanceExtreme,
			},
			Timestamp: time.Now(),
			Suggestions: []string{
				"预期价格可能出现剧烈波动",
				"考虑调整交易策略",
				"注意风险管理",
			},
		}
		
		monitor.sendAlert(alert)
	}
}

// checkLiquidityDrop 检查流动性下降
func (monitor *OrderBookMonitor) checkLiquidityDrop(data *OrderBookData) {
	if data.LiquidityScore < monitor.alertThresholds.LiquidityDrop {
		alert := &OrderBookAlert{
			Symbol:    monitor.symbol,
			AlertType: "liquidity_drop",
			Severity:  "medium",
			Message:   "市场流动性显著下降",
			Data: map[string]interface{}{
				"liquidity_score": data.LiquidityScore,
				"threshold":       monitor.alertThresholds.LiquidityDrop,
			},
			Timestamp: time.Now(),
			Suggestions: []string{
				"谨慎执行大额订单",
				"考虑分批交易",
				"监控滑点风险",
			},
		}
		
		monitor.sendAlert(alert)
	}
}

// checkWallChangeFrequency 检查墙变化频率
func (monitor *OrderBookMonitor) checkWallChangeFrequency(data *OrderBookData) {
	if data.WallChangeCount5m > monitor.alertThresholds.WallChangeFrequency {
		alert := &OrderBookAlert{
			Symbol:    monitor.symbol,
			AlertType: "high_wall_volatility",
			Severity:  "medium",
			Message:   "挂单墙变化过于频繁",
			Data: map[string]interface{}{
				"wall_changes_5m": data.WallChangeCount5m,
				"threshold":       monitor.alertThresholds.WallChangeFrequency,
			},
			Timestamp: time.Now(),
			Suggestions: []string{
				"市场可能存在操控行为",
				"等待市场稳定后交易",
				"提高警惕",
			},
		}
		
		monitor.sendAlert(alert)
	}
}

// checkLargeOrders 检查大额订单
func (monitor *OrderBookMonitor) checkLargeOrders(data *OrderBookData) {
	checkLargeWall := func(wall *WallInfo, wallType string) {
		if wall != nil && wall.StrengthUSD > monitor.alertThresholds.LargeOrderDetection {
			alert := &OrderBookAlert{
				Symbol:    monitor.symbol,
				AlertType: "large_order_detected",
				Severity:  "info",
				Message:   fmt.Sprintf("检测到大额%s墙", wallType),
				Data: map[string]interface{}{
					"wall_type":    wallType,
					"strength_usd": wall.StrengthUSD,
					"price":        wall.Price,
					"is_solid":     wall.IsSolid,
					"threshold":    monitor.alertThresholds.LargeOrderDetection,
				},
				Timestamp: time.Now(),
				Suggestions: []string{
					"关注此价位的支撑/阻力作用",
					"注意可能的反弹或回落",
					"考虑在此价位附近的交易机会",
				},
			}
			
			monitor.sendAlert(alert)
		}
	}
	
	checkLargeWall(data.NearestResistance, "阻力")
	checkLargeWall(data.NearestSupport, "支撑")
}

// checkMarketManipulation 检查市场操控
func (monitor *OrderBookMonitor) checkMarketManipulation(data *OrderBookData, snapshot *WallSnapshot) {
	// 综合多个指标判断市场操控
	manipulationScore := 0.0
	
	// 1. 虚假挂单风险权重 40%
	manipulationScore += data.SpoofingRisk * 0.4
	
	// 2. 墙变化频率权重 30%
	wallChangeScore := math.Min(1.0, float64(data.WallChangeCount5m)/20.0)
	manipulationScore += wallChangeScore * 0.3
	
	// 3. 失衡极端程度权重 20%
	imbalanceScore := math.Abs(data.ImbalanceRatio)
	manipulationScore += imbalanceScore * 0.2
	
	// 4. 流动性异常权重 10%
	liquidityAbnormal := 1.0 - data.LiquidityScore
	manipulationScore += liquidityAbnormal * 0.1
	
	if manipulationScore > monitor.alertThresholds.MarketManipulation {
		// 记录操控事件
		event := ManipulationEvent{
			Timestamp:   time.Now(),
			EventType:   "potential_manipulation",
			Severity:    manipulationScore,
			Description: "检测到潜在市场操控行为",
			Evidence: map[string]interface{}{
				"manipulation_score":   manipulationScore,
				"spoofing_risk":        data.SpoofingRisk,
				"wall_change_count":    data.WallChangeCount5m,
				"imbalance_ratio":      data.ImbalanceRatio,
				"liquidity_score":      data.LiquidityScore,
			},
		}
		
		monitor.wallTracker.AddManipulationEvent(event)
		
		severity := "medium"
		if manipulationScore > 0.95 {
			severity = "high"
		}
		
		alert := &OrderBookAlert{
			Symbol:    monitor.symbol,
			AlertType: "market_manipulation",
			Severity:  severity,
			Message:   "检测到潜在市场操控行为",
			Data: map[string]interface{}{
				"manipulation_score": manipulationScore,
				"evidence":          event.Evidence,
				"threshold":         monitor.alertThresholds.MarketManipulation,
			},
			Timestamp: time.Now(),
			Suggestions: []string{
				"暂停交易活动",
				"等待市场恢复正常",
				"提高风险管理级别",
				"考虑降低仓位",
			},
		}
		
		monitor.sendAlert(alert)
	}
}

// sendAlert 发送警报
func (monitor *OrderBookMonitor) sendAlert(alert *OrderBookAlert) {
	monitor.lastAlert = time.Now()
	
	if monitor.alertCallback != nil {
		go monitor.alertCallback(alert) // 异步回调避免阻塞
	}
	
	log.Printf("🚨 [%s] %s警报: %s (严重程度: %s)",
		alert.Symbol, alert.AlertType, alert.Message, alert.Severity)
}

// ===== WallStabilityTracker 方法 =====

// AddSnapshot 添加墙快照
func (tracker *WallStabilityTracker) AddSnapshot(snapshot *WallSnapshot) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	
	tracker.wallSnapshots = append(tracker.wallSnapshots, snapshot)
	
	// 限制历史记录大小
	if len(tracker.wallSnapshots) > tracker.maxSnapshotHistory {
		tracker.wallSnapshots = tracker.wallSnapshots[len(tracker.wallSnapshots)-tracker.maxSnapshotHistory:]
	}
	
	// 清理过期数据
	tracker.cleanupExpiredData()
}

// AddManipulationEvent 添加操控事件
func (tracker *WallStabilityTracker) AddManipulationEvent(event ManipulationEvent) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	
	tracker.manipulationEvents = append(tracker.manipulationEvents, event)
	
	// 限制事件记录大小
	if len(tracker.manipulationEvents) > 100 {
		tracker.manipulationEvents = tracker.manipulationEvents[len(tracker.manipulationEvents)-100:]
	}
}

// cleanupExpiredData 清理过期数据
func (tracker *WallStabilityTracker) cleanupExpiredData() {
	cutoffTime := time.Now().Add(-tracker.analysisWindow)
	
	// 清理过期快照
	var validSnapshots []*WallSnapshot
	for _, snapshot := range tracker.wallSnapshots {
		if snapshot.Timestamp.After(cutoffTime) {
			validSnapshots = append(validSnapshots, snapshot)
		}
	}
	tracker.wallSnapshots = validSnapshots
	
	// 清理过期操控事件
	var validEvents []ManipulationEvent
	for _, event := range tracker.manipulationEvents {
		if event.Timestamp.After(cutoffTime) {
			validEvents = append(validEvents, event)
		}
	}
	tracker.manipulationEvents = validEvents
}

// GetStabilityAnalysis 获取稳定性分析
func (tracker *WallStabilityTracker) GetStabilityAnalysis() *WallStabilityAnalysis {
	tracker.mu.RLock()
	defer tracker.mu.RUnlock()
	
	analysis := &WallStabilityAnalysis{
		Symbol:    tracker.symbol,
		Timestamp: time.Now(),
	}
	
	if len(tracker.wallSnapshots) == 0 {
		return analysis
	}
	
	// 分析墙稳定性趋势
	analysis.StabilityTrend = tracker.analyzeStabilityTrend()
	analysis.ManipulationRisk = tracker.calculateManipulationRisk()
	analysis.LiquidityTrend = tracker.analyzeLiquidityTrend()
	analysis.RecommendedAction = tracker.generateRecommendation(analysis)
	
	return analysis
}

// WallStabilityAnalysis 墙稳定性分析结果
type WallStabilityAnalysis struct {
	Symbol              string    `json:"symbol"`
	Timestamp           time.Time `json:"timestamp"`
	StabilityTrend      string    `json:"stability_trend"`
	ManipulationRisk    float64   `json:"manipulation_risk"`
	LiquidityTrend      string    `json:"liquidity_trend"`
	RecommendedAction   string    `json:"recommended_action"`
}

// analyzeStabilityTrend 分析稳��性趋势
func (tracker *WallStabilityTracker) analyzeStabilityTrend() string {
	if len(tracker.wallSnapshots) < 10 {
		return "insufficient_data"
	}
	
	recent := tracker.wallSnapshots[len(tracker.wallSnapshots)-10:]
	var avgSpoofingRisk float64
	
	for _, snapshot := range recent {
		avgSpoofingRisk += snapshot.SpoofingRisk
	}
	avgSpoofingRisk /= float64(len(recent))
	
	if avgSpoofingRisk < 0.3 {
		return "stable"
	} else if avgSpoofingRisk < 0.6 {
		return "moderately_unstable"
	} else {
		return "highly_unstable"
	}
}

// calculateManipulationRisk 计算操控风险
func (tracker *WallStabilityTracker) calculateManipulationRisk() float64 {
	if len(tracker.manipulationEvents) == 0 {
		return 0
	}
	
	// 最近1小时内的操控事件
	recentCutoff := time.Now().Add(-time.Hour)
	var recentEvents []ManipulationEvent
	
	for _, event := range tracker.manipulationEvents {
		if event.Timestamp.After(recentCutoff) {
			recentEvents = append(recentEvents, event)
		}
	}
	
	if len(recentEvents) == 0 {
		return 0
	}
	
	// 计算平均严重程度
	var avgSeverity float64
	for _, event := range recentEvents {
		avgSeverity += event.Severity
	}
	
	return avgSeverity / float64(len(recentEvents))
}

// analyzeLiquidityTrend 分析流动性趋势
func (tracker *WallStabilityTracker) analyzeLiquidityTrend() string {
	if len(tracker.wallSnapshots) < 5 {
		return "insufficient_data"
	}
	
	recent := tracker.wallSnapshots[len(tracker.wallSnapshots)-5:]
	
	// 计算流动性变化趋势
	var liquidityScores []float64
	for _, snapshot := range recent {
		if score, exists := snapshot.MarketMetrics["liquidity_score"]; exists {
			if scoreFloat, ok := score.(float64); ok {
				liquidityScores = append(liquidityScores, scoreFloat)
			}
		}
	}
	
	if len(liquidityScores) < 3 {
		return "insufficient_data"
	}
	
	// 简单线性趋势分析
	start := liquidityScores[0]
	end := liquidityScores[len(liquidityScores)-1]
	change := end - start
	
	if change > 0.1 {
		return "improving"
	} else if change < -0.1 {
		return "deteriorating"
	} else {
		return "stable"
	}
}

// generateRecommendation 生成建议
func (tracker *WallStabilityTracker) generateRecommendation(analysis *WallStabilityAnalysis) string {
	if analysis.ManipulationRisk > 0.8 {
		return "avoid_trading"
	} else if analysis.ManipulationRisk > 0.6 {
		return "trade_with_caution"
	} else if analysis.StabilityTrend == "stable" && analysis.LiquidityTrend != "deteriorating" {
		return "normal_trading"
	} else {
		return "monitor_closely"
	}
}