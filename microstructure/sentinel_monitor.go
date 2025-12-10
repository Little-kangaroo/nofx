package microstructure

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// ===== Go哨兵监控和异动触发机制 =====

// SentinelAlert 哨兵告警
type SentinelAlert struct {
	Symbol      string    `json:"symbol"`
	AlertType   string    `json:"alert_type"`
	Level       string    `json:"level"`       // INFO, WARNING, CRITICAL
	Message     string    `json:"message"`
	Metrics     map[string]float64 `json:"metrics"`
	Timestamp   time.Time `json:"timestamp"`
	Duration    time.Duration `json:"duration"` // 异动持续时间
}

// SentinelRule 监控规则
type SentinelRule struct {
	Name        string             `json:"name"`
	Enabled     bool              `json:"enabled"`
	Level       string            `json:"level"`
	Checker     func(*MarketSnapshot) *SentinelAlert
	Cooldown    time.Duration     `json:"cooldown"` // 冷却时间，避免重复告警
}

// SentinelMonitor 哨兵监控器
type SentinelMonitor struct {
	mu           sync.RWMutex
	rules        []SentinelRule
	alerts       []SentinelAlert
	lastAlert    map[string]time.Time // rule_name -> last_alert_time
	alertChannel chan *SentinelAlert
	isRunning    bool
	ticker       *time.Ticker
}

// NewSentinelMonitor 创建哨兵监控器
func NewSentinelMonitor() *SentinelMonitor {
	monitor := &SentinelMonitor{
		lastAlert:    make(map[string]time.Time),
		alertChannel: make(chan *SentinelAlert, 100),
	}
	
	// 初始化监控规则
	monitor.initializeRules()
	
	return monitor
}

// initializeRules 初始化监控规则
func (sm *SentinelMonitor) initializeRules() {
	sm.rules = []SentinelRule{
		// 🔧 规则1：CVD异动监控
		{
			Name:     "cvd_anomaly",
			Enabled:  true,
			Level:    "WARNING",
			Cooldown: 5 * time.Minute,
			Checker:  sm.checkCVDAnomaly,
		},
		
		// 🔧 规则2：持仓量突变监控
		{
			Name:     "oi_spike",
			Enabled:  true,
			Level:    "WARNING",
			Cooldown: 3 * time.Minute,
			Checker:  sm.checkOISpike,
		},
		
		// 🔧 规则3：虚假挂单大量出现
		{
			Name:     "spoofing_surge",
			Enabled:  true,
			Level:    "CRITICAL",
			Cooldown: 10 * time.Minute,
			Checker:  sm.checkSpoofingSurge,
		},
		
		// 🔧 规则4：流动性急剧下降
		{
			Name:     "liquidity_drought",
			Enabled:  true,
			Level:    "CRITICAL",
			Cooldown: 5 * time.Minute,
			Checker:  sm.checkLiquidityDrought,
		},
		
		// 🔧 规则5：数据质量异常
		{
			Name:     "data_quality_drop",
			Enabled:  true,
			Level:    "WARNING",
			Cooldown: 2 * time.Minute,
			Checker:  sm.checkDataQualityDrop,
		},
		
		// 🔧 规则6：成交量异常放大
		{
			Name:     "volume_explosion",
			Enabled:  true,
			Level:    "INFO",
			Cooldown: 1 * time.Minute,
			Checker:  sm.checkVolumeExplosion,
		},
		
		// 🔧 规则7：价格与订单流严重背离
		{
			Name:     "price_flow_divergence",
			Enabled:  true,
			Level:    "WARNING",
			Cooldown: 3 * time.Minute,
			Checker:  sm.checkPriceFlowDivergence,
		},
	}
}

// Start 启动哨兵监控
func (sm *SentinelMonitor) Start() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if sm.isRunning {
		return
	}
	
	sm.isRunning = true
	sm.ticker = time.NewTicker(10 * time.Second) // 每10秒检查一次
	
	// 启动监控协程
	go sm.monitorLoop()
	
	// 启动告警处理协程
	go sm.alertHandlerLoop()
	
	log.Printf("🛡️ 哨兵监控系统启动完成")
}

// Stop 停止哨兵监控
func (sm *SentinelMonitor) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if !sm.isRunning {
		return
	}
	
	sm.isRunning = false
	if sm.ticker != nil {
		sm.ticker.Stop()
	}
	
	close(sm.alertChannel)
	
	log.Printf("🛡️ 哨兵监控系统已停止")
}

// monitorLoop 监控循环
func (sm *SentinelMonitor) monitorLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 哨兵监控循环异常: %v", r)
		}
	}()
	
	for range sm.ticker.C {
		sm.mu.RLock()
		if !sm.isRunning {
			sm.mu.RUnlock()
			break
		}
		sm.mu.RUnlock()
		
		// 获取所有订阅币种的市场快照
		ofm := GetGlobalOrderFlowManager()
		if ofm == nil {
			continue
		}
		
		snapshots := ofm.GetAllMarketSnapshots()
		
		// 对每个快照执行所有规则检查
		for symbol, snapshot := range snapshots {
			if snapshot == nil {
				continue
			}
			
			sm.executeRules(symbol, snapshot)
		}
	}
}

// executeRules 执行监控规则
func (sm *SentinelMonitor) executeRules(symbol string, snapshot *MarketSnapshot) {
	for _, rule := range sm.rules {
		if !rule.Enabled {
			continue
		}
		
		// 检查冷却时间
		ruleKey := fmt.Sprintf("%s_%s", rule.Name, symbol)
		if lastTime, exists := sm.lastAlert[ruleKey]; exists {
			if time.Since(lastTime) < rule.Cooldown {
				continue
			}
		}
		
		// 执行规则检查
		alert := rule.Checker(snapshot)
		if alert != nil {
			alert.Level = rule.Level
			alert.Symbol = symbol
			
			// 发送告警
			select {
			case sm.alertChannel <- alert:
				sm.lastAlert[ruleKey] = time.Now()
			default:
				log.Printf("⚠️ 告警通道已满，丢弃告警: %s", alert.Message)
			}
		}
	}
}

// alertHandlerLoop 告警处理循环
func (sm *SentinelMonitor) alertHandlerLoop() {
	for alert := range sm.alertChannel {
		sm.handleAlert(alert)
	}
}

// handleAlert 处理告警
func (sm *SentinelMonitor) handleAlert(alert *SentinelAlert) {
	sm.mu.Lock()
	sm.alerts = append(sm.alerts, *alert)
	
	// 只保留最近1000条告警
	if len(sm.alerts) > 1000 {
		sm.alerts = sm.alerts[len(sm.alerts)-1000:]
	}
	sm.mu.Unlock()
	
	// 根据告警级别输出不同格式的日志
	switch alert.Level {
	case "CRITICAL":
		log.Printf("🚨 [CRITICAL] %s: %s", alert.Symbol, alert.Message)
	case "WARNING":
		log.Printf("⚠️ [WARNING] %s: %s", alert.Symbol, alert.Message)
	case "INFO":
		log.Printf("ℹ️ [INFO] %s: %s", alert.Symbol, alert.Message)
	default:
		log.Printf("📝 [%s] %s: %s", alert.Level, alert.Symbol, alert.Message)
	}
}

// ===== 监控规则实现 =====

// checkCVDAnomaly 检查CVD异动
func (sm *SentinelMonitor) checkCVDAnomaly(snapshot *MarketSnapshot) *SentinelAlert {
	if snapshot.CVDData == nil || snapshot.CVDDelta5m == nil {
		return nil
	}
	
	// 检查5分钟CVD增量是否异常
	spotDelta := math.Abs(snapshot.CVDDelta5m.SpotCVDDeltaUSD)
	futuresDelta := math.Abs(snapshot.CVDDelta5m.FuturesCVDDeltaUSD)
	
	// 异动阈值：5分钟内现货或期货CVD变化超过500万USD
	if spotDelta > 5000000 || futuresDelta > 5000000 {
		return &SentinelAlert{
			AlertType: "cvd_anomaly",
			Message: fmt.Sprintf("CVD异动检测: 5分钟内现货CVD变化%.0fM, 期货CVD变化%.0fM", 
				spotDelta/1000000, futuresDelta/1000000),
			Metrics: map[string]float64{
				"spot_delta": spotDelta,
				"futures_delta": futuresDelta,
			},
			Timestamp: time.Now(),
		}
	}
	
	return nil
}

// checkOISpike 检查持仓量突变
func (sm *SentinelMonitor) checkOISpike(snapshot *MarketSnapshot) *SentinelAlert {
	if snapshot.OIAnalysis == nil {
		return nil
	}
	
	// 检查1小时持仓量变化率
	changeRate := math.Abs(snapshot.OIAnalysis.ChangeRate1H)
	
	// 突变阈值：1小时内持仓量变化超过30%
	if changeRate > 30 {
		return &SentinelAlert{
			AlertType: "oi_spike",
			Message: fmt.Sprintf("持仓量突变: 1小时内变化%.1f%%, 当前持仓%.0f", 
				snapshot.OIAnalysis.ChangeRate1H, snapshot.OIAnalysis.Current),
			Metrics: map[string]float64{
				"change_rate": snapshot.OIAnalysis.ChangeRate1H,
				"current": snapshot.OIAnalysis.Current,
			},
			Timestamp: time.Now(),
		}
	}
	
	return nil
}

// checkSpoofingSurge 检查虚假挂单大量出现
func (sm *SentinelMonitor) checkSpoofingSurge(snapshot *MarketSnapshot) *SentinelAlert {
	if snapshot.OrderBookData == nil {
		return nil
	}
	
	// 检查虚假挂单风险评分
	spoofingRisk := snapshot.OrderBookData.SpoofingRisk
	wallChanges := snapshot.OrderBookData.WallChangeCount5m
	
	// 严重阈值：虚假挂单风险 > 0.7 且 5分钟内墙变化 > 50次
	if spoofingRisk > 0.7 && wallChanges > 50 {
		return &SentinelAlert{
			AlertType: "spoofing_surge",
			Message: fmt.Sprintf("虚假挂单大量出现: 风险评分%.2f, 5分钟内墙变化%d次", 
				spoofingRisk, wallChanges),
			Metrics: map[string]float64{
				"spoofing_risk": spoofingRisk,
				"wall_changes": float64(wallChanges),
			},
			Timestamp: time.Now(),
		}
	}
	
	return nil
}

// checkLiquidityDrought 检查流动性急剧下降
func (sm *SentinelMonitor) checkLiquidityDrought(snapshot *MarketSnapshot) *SentinelAlert {
	if snapshot.OrderBookData == nil {
		return nil
	}
	
	// 检查流动性评分
	liquidityScore := snapshot.OrderBookData.LiquidityScore
	
	// 严重阈值：流动性评分 < 0.2
	if liquidityScore < 0.2 {
		return &SentinelAlert{
			AlertType: "liquidity_drought",
			Message: fmt.Sprintf("流动性急剧下降: 评分%.2f, 买压%.0f, 卖压%.0f", 
				liquidityScore, snapshot.OrderBookData.BidPressure, snapshot.OrderBookData.AskPressure),
			Metrics: map[string]float64{
				"liquidity_score": liquidityScore,
				"bid_pressure": snapshot.OrderBookData.BidPressure,
				"ask_pressure": snapshot.OrderBookData.AskPressure,
			},
			Timestamp: time.Now(),
		}
	}
	
	return nil
}

// checkDataQualityDrop 检查数据质量异常下降
func (sm *SentinelMonitor) checkDataQualityDrop(snapshot *MarketSnapshot) *SentinelAlert {
	if snapshot.DataQuality == nil {
		return nil
	}
	
	// 检查整体数据质量
	overallScore := snapshot.DataQuality.OverallScore
	
	// 异常阈值：整体质量评分 < 0.5
	if overallScore < 0.5 {
		return &SentinelAlert{
			AlertType: "data_quality_drop",
			Message: fmt.Sprintf("数据质量异常: 整体评分%.2f, CVD可靠性%.2f, OI可靠性%.2f", 
				overallScore, snapshot.DataQuality.CVDReliability, snapshot.DataQuality.OIReliability),
			Metrics: map[string]float64{
				"overall_score": overallScore,
				"cvd_reliability": snapshot.DataQuality.CVDReliability,
				"oi_reliability": snapshot.DataQuality.OIReliability,
			},
			Timestamp: time.Now(),
		}
	}
	
	return nil
}

// checkVolumeExplosion 检查成交量异常放大
func (sm *SentinelMonitor) checkVolumeExplosion(snapshot *MarketSnapshot) *SentinelAlert {
	if snapshot.CVDDelta5m == nil {
		return nil
	}
	
	// 检查成交量比率
	volumeRatio := snapshot.CVDDelta5m.VolumeRatio
	
	// 异动阈值：成交量比率 > 10倍
	if volumeRatio > 10 {
		return &SentinelAlert{
			AlertType: "volume_explosion",
			Message: fmt.Sprintf("成交量异常放大: 当前成交量为平均值的%.1f倍", volumeRatio),
			Metrics: map[string]float64{
				"volume_ratio": volumeRatio,
				"volume_delta": snapshot.CVDDelta5m.VolumeDelta,
			},
			Timestamp: time.Now(),
		}
	}
	
	return nil
}

// checkPriceFlowDivergence 检查价格与订单流严重背离
func (sm *SentinelMonitor) checkPriceFlowDivergence(snapshot *MarketSnapshot) *SentinelAlert {
	if snapshot.CVDDelta5m == nil {
		return nil
	}
	
	priceDelta := snapshot.CVDDelta5m.PriceDeltaPct
	spotDelta := snapshot.CVDDelta5m.SpotCVDDeltaUSD
	futuresDelta := snapshot.CVDDelta5m.FuturesCVDDeltaUSD
	
	// 背离检测：价格大涨(>2%)但现货大卖(< -200万)，或价格大跌(<-2%)但现货大买(> 200万)
	if (priceDelta > 2 && spotDelta < -2000000) || (priceDelta < -2 && spotDelta > 2000000) {
		return &SentinelAlert{
			AlertType: "price_flow_divergence",
			Message: fmt.Sprintf("价格与订单流严重背离: 价格变化%.1f%%, 现货CVD变化%.0fM", 
				priceDelta, spotDelta/1000000),
			Metrics: map[string]float64{
				"price_delta": priceDelta,
				"spot_delta": spotDelta,
				"futures_delta": futuresDelta,
			},
			Timestamp: time.Now(),
		}
	}
	
	return nil
}

// ===== 查询接口 =====

// GetAlerts 获取告警历史
func (sm *SentinelMonitor) GetAlerts(limit int) []SentinelAlert {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	alerts := make([]SentinelAlert, len(sm.alerts))
	copy(alerts, sm.alerts)
	
	// 按时间倒序排列
	if len(alerts) > limit {
		alerts = alerts[len(alerts)-limit:]
	}
	
	return alerts
}

// GetStatus 获取监控状态
func (sm *SentinelMonitor) GetStatus() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	return map[string]interface{}{
		"is_running": sm.isRunning,
		"rules_count": len(sm.rules),
		"alerts_count": len(sm.alerts),
		"enabled_rules": func() []string {
			var enabled []string
			for _, rule := range sm.rules {
				if rule.Enabled {
					enabled = append(enabled, rule.Name)
				}
			}
			return enabled
		}(),
	}
}

// 全局哨兵监控实例
var globalSentinelMonitor *SentinelMonitor
var globalSentinelOnce sync.Once

// GetGlobalSentinelMonitor 获取全局哨兵监控器
func GetGlobalSentinelMonitor() *SentinelMonitor {
	globalSentinelOnce.Do(func() {
		globalSentinelMonitor = NewSentinelMonitor()
	})
	return globalSentinelMonitor
}