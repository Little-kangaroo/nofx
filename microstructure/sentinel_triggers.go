package microstructure

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// ===== V-12.2哨兵触发器系统 =====

// ScanMode 扫描模式枚举
type ScanMode string

const (
	ScanModeNormal    ScanMode = "normal"     // 正常模式: 1秒扫描
	ScanModeHighFreq  ScanMode = "high_freq"  // 高频模式: 100ms扫描
	ScanModeAdaptive  ScanMode = "adaptive"   // 自适应模式: 根据市场状态调整
)

// TriggerScenario 触发场景枚举
type TriggerScenario string

const (
	ScenarioMomentumIgnition TriggerScenario = "momentum_ignition" // 场景A: 动量点燃
	ScenarioSpotRush         TriggerScenario = "spot_rush"         // 场景B: 现货抢跑
	ScenarioSpoofingFlip     TriggerScenario = "spoofing_flip"     // 场景C: 虚假翻转
)

// SentinelTriggerAlert 哨兵触发器警报（V-12.2）
type SentinelTriggerAlert struct {
	Symbol      string          `json:"symbol"`
	Scenario    TriggerScenario `json:"scenario"`
	Timestamp   time.Time       `json:"timestamp"`
	Confidence  float64         `json:"confidence"`  // 置信度 [0-1]
	ZScoreData  map[string]float64 `json:"zscore_data"` // 相关Z-Score数据
	Description string          `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"` // 额外元数据
}

// SentinelTriggerEngine V-12.2哨兵触发引擎
type SentinelTriggerEngine struct {
	mu               sync.RWMutex
	alertHandlers    []func(*SentinelTriggerAlert)      // 警报处理器
	lastAlerts       map[string]time.Time        // 🔧 P2-2修复：symbol+scenario -> last alert time (精细化冷却机制)
	cooldownPeriod   time.Duration               // 警报冷却期
	isRunning        bool
	ticker           *time.Ticker
	stopChan         chan bool
	
	// V-12.2 100ms级高频扫描
	highFreqTicker   *time.Ticker    // 100ms高频扫描定时器
	scanMode         ScanMode        // 扫描模式
	adaptiveScan     bool            // 自适应扫描频率
	
	// V-12.2核心阈值配置（基础值，将被动态阈值系统覆盖）
	momentumThresholds   MomentumThresholds
	spotRushThresholds   SpotRushThresholds  
	spoofingThresholds   SpoofingThresholds
	
	// V-12.2动态阈值系统引用
	dynamicThresholds    *DynamicThresholdSystem
}

// MomentumThresholds 动量点燃场景阈值
type MomentumThresholds struct {
	CVDZScoreMin      float64 `json:"cvd_zscore_min"`      // CVD Z-Score最小值: 2.0
	VolumeZScoreMin   float64 `json:"volume_zscore_min"`   // 成交量 Z-Score最小值: 1.5
	PriceZScoreMin    float64 `json:"price_zscore_min"`    // 🔧 P0-2修复：价格Z-Score最小值: 3.0 (替代硬编码1%)
	ConfidenceWeight  float64 `json:"confidence_weight"`   // 置信度权重: 0.4
}

// SpotRushThresholds 现货抢跑场景阈值
type SpotRushThresholds struct {
	SpotCVDZScoreMin    float64 `json:"spot_cvd_zscore_min"`    // 现货CVD Z-Score最小值: 1.8
	FuturesCVDZScoreMax float64 `json:"futures_cvd_zscore_max"` // 合约CVD Z-Score最大值: -0.5
	CVDRatioZScoreMin   float64 `json:"cvd_ratio_zscore_min"`   // CVD比率 Z-Score最小值: 1.5
	ConfidenceWeight    float64 `json:"confidence_weight"`      // 置信度权重: 0.35
}

// SpoofingThresholds 虚假翻转场景阈值
type SpoofingThresholds struct {
	SpoofingZScoreMin   float64 `json:"spoofing_zscore_min"`   // 虚假挂单 Z-Score最小值: 1.5
	LiquidityZScoreMax  float64 `json:"liquidity_zscore_max"`  // 流动性 Z-Score最大值: -1.0
	ImbalanceZScoreMin  float64 `json:"imbalance_zscore_min"`  // 失衡比率 Z-Score最小值: 1.2
	ConfidenceWeight    float64 `json:"confidence_weight"`     // 置信度权重: 0.25
}

// NewSentinelTriggerEngine 创建哨兵触发引擎
func NewSentinelTriggerEngine() *SentinelTriggerEngine {
	return &SentinelTriggerEngine{
		alertHandlers:   make([]func(*SentinelTriggerAlert), 0),
		lastAlerts:      make(map[string]time.Time),
		cooldownPeriod:  30 * time.Second, // 30秒冷却期
		stopChan:        make(chan bool),
		
		// V-12.2 高频扫描配置
		scanMode:        ScanModeAdaptive, // 默认自适应模式
		adaptiveScan:    true,             // 启用自适应扫描
		
		// V-12.2标准阈值配置
		momentumThresholds: MomentumThresholds{
			CVDZScoreMin:     2.0,
			VolumeZScoreMin:  1.5,
			PriceZScoreMin:   3.0, // 🔧 P0-2修复：使用Z-Score替代硬编码1%
			ConfidenceWeight: 0.4,
		},
		spotRushThresholds: SpotRushThresholds{
			SpotCVDZScoreMin:    1.8,
			FuturesCVDZScoreMax: -0.5,
			CVDRatioZScoreMin:   1.5,
			ConfidenceWeight:    0.35,
		},
		spoofingThresholds: SpoofingThresholds{
			SpoofingZScoreMin:  1.5,
			LiquidityZScoreMax: -1.0,
			ImbalanceZScoreMin: 1.2,
			ConfidenceWeight:   0.25,
		},
		
		// V-12.2动态阈值系统引用
		dynamicThresholds: GetGlobalDynamicThresholdSystem(),
	}
}

// AddAlertHandler 添加警报处理器
func (ste *SentinelTriggerEngine) AddAlertHandler(handler func(*SentinelTriggerAlert)) {
	ste.mu.Lock()
	defer ste.mu.Unlock()
	ste.alertHandlers = append(ste.alertHandlers, handler)
}

// Start 启动哨兵扫描系统（V-12.2增强版）
func (ste *SentinelTriggerEngine) Start() {
	ste.mu.Lock()
	defer ste.mu.Unlock()
	
	if ste.isRunning {
		return
	}
	
	// 根据扫描模式设置不同的定时器
	switch ste.scanMode {
	case ScanModeNormal:
		ste.ticker = time.NewTicker(1 * time.Second) // 正常模式: 1秒
		log.Printf("🚀 V-12.2哨兵触发引擎启动 - 正常扫描模式 (1秒)")
	case ScanModeHighFreq:
		ste.highFreqTicker = time.NewTicker(100 * time.Millisecond) // 高频模式: 100ms
		log.Printf("🚀 V-12.2哨兵触发引擎启动 - 高频扫描模式 (100ms)")
	case ScanModeAdaptive:
		// 自适应模式: 同时启动两个定时器，根据市场状态切换
		ste.ticker = time.NewTicker(1 * time.Second)
		ste.highFreqTicker = time.NewTicker(100 * time.Millisecond)
		log.Printf("🚀 V-12.2哨兵触发引擎启动 - 自适应扫描模式 (100ms~1s)")
	}
	
	ste.isRunning = true
	
	// 启动扫描循环
	go ste.enhancedScanLoop()
}

// Stop 停止哨兵扫描系统（V-12.2增强版）
func (ste *SentinelTriggerEngine) Stop() {
	ste.mu.Lock()
	defer ste.mu.Unlock()
	
	if !ste.isRunning {
		return
	}
	
	close(ste.stopChan)
	
	// 停止所有定时器
	if ste.ticker != nil {
		ste.ticker.Stop()
		ste.ticker = nil
	}
	if ste.highFreqTicker != nil {
		ste.highFreqTicker.Stop()
		ste.highFreqTicker = nil
	}
	
	ste.isRunning = false
	log.Printf("⛔ V-12.2哨兵触发引擎已停止")
}

// enhancedScanLoop V-12.2增强扫描主循环
func (ste *SentinelTriggerEngine) enhancedScanLoop() {
	// 用于跟踪自适应扫描状态
	var isHighFreqActive bool = false
	lastModeSwitch := time.Now()
	
	for {
		select {
		case <-ste.ticker.C:
			// 正常频率扫描（1秒）
			if ste.scanMode == ScanModeNormal || 
			   (ste.scanMode == ScanModeAdaptive && !isHighFreqActive) {
				ste.performScan()
				
				// 自适应模式: 检查是否需要切换到高频扫描
				if ste.scanMode == ScanModeAdaptive {
					if ste.shouldSwitchToHighFreq() {
						isHighFreqActive = true
						lastModeSwitch = time.Now()
						log.Printf("⚡ 切换到高频扫描模式 (100ms)")
					}
				}
			}
			
		case <-ste.highFreqTicker.C:
			// 高频扫描（100ms）
			if ste.scanMode == ScanModeHighFreq || 
			   (ste.scanMode == ScanModeAdaptive && isHighFreqActive) {
				ste.performScan()
				
				// 自适应模式: 检查是否需要切换回正常扫描
				if ste.scanMode == ScanModeAdaptive {
					// 高频扫描最多持续30秒
					if time.Since(lastModeSwitch) > 30*time.Second || 
					   !ste.shouldMaintainHighFreq() {
						isHighFreqActive = false
						lastModeSwitch = time.Now()
						log.Printf("🔄 切换回正常扫描模式 (1秒)")
					}
				}
			}
			
		case <-ste.stopChan:
			return
		}
	}
}

// shouldSwitchToHighFreq 判断是否应该切换到高频扫描
func (ste *SentinelTriggerEngine) shouldSwitchToHighFreq() bool {
	// 检查是否有任何市场处于高风险状态
	globalStats := GetGlobalStatsManager()
	if globalStats == nil {
		return false
	}
	
	ofm := GetGlobalOrderFlowManager()
	if ofm == nil {
		return false
	}
	
	snapshots := ofm.GetAllMarketSnapshots()
	for symbol, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		
		// 获取Z-Score数据
		zScores := globalStats.GetSymbolZScores(symbol, snapshot)
		if len(zScores) == 0 {
			continue
		}
		
		// 检查是否有极端Z-Score值 (>2.5 或 <-2.5)
		for _, zScore := range zScores {
			if math.Abs(zScore) > 2.5 {
				return true
			}
		}
		
		// 检查动态阈值状态
		dynamicThresholds := ste.dynamicThresholds.GetThresholds(symbol)
		if dynamicThresholds != nil {
			// 高波动、操纵风险、或低流动性状态触发高频扫描
			if dynamicThresholds.CurrentRegime == RegimeHighVolatility ||
			   dynamicThresholds.CurrentRegime == RegimeManipulation ||
			   dynamicThresholds.ManipulationRisk > 0.7 ||
			   dynamicThresholds.VolatilityLevel > 2.5 {
				return true
			}
		}
	}
	
	return false
}

// shouldMaintainHighFreq 判断是否应该保持高频扫描
func (ste *SentinelTriggerEngine) shouldMaintainHighFreq() bool {
	// 使用更严格的条件来维持高频扫描
	globalStats := GetGlobalStatsManager()
	if globalStats == nil {
		return false
	}
	
	ofm := GetGlobalOrderFlowManager()
	if ofm == nil {
		return false
	}
	
	snapshots := ofm.GetAllMarketSnapshots()
	highRiskCount := 0
	
	for symbol, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		
		zScores := globalStats.GetSymbolZScores(symbol, snapshot)
		if len(zScores) == 0 {
			continue
		}
		
		// 计算风险评分
		riskScore := 0.0
		for _, zScore := range zScores {
			if math.Abs(zScore) > 2.0 {
				riskScore += 1.0
			}
		}
		
		if riskScore >= 2.0 { // 至少2个指标异常
			highRiskCount++
		}
	}
	
	// 如果有多个交易对处于高风险状态，继续高频扫描
	return highRiskCount >= 2
}

// SetScanMode 设置扫描模式
func (ste *SentinelTriggerEngine) SetScanMode(mode ScanMode) {
	ste.mu.Lock()
	defer ste.mu.Unlock()
	
	if ste.isRunning {
		log.Printf("⚠️ 无法在运行时切换扫描模式，请先停止引擎")
		return
	}
	
	ste.scanMode = mode
	log.Printf("🎯 哨兵扫描模式已设置为: %s", mode)
}

// GetScanMode 获取当前扫描模式
func (ste *SentinelTriggerEngine) GetScanMode() ScanMode {
	ste.mu.RLock()
	defer ste.mu.RUnlock()
	return ste.scanMode
}

// scanLoop 哨兵扫描主循环（保留兼容性）
func (ste *SentinelTriggerEngine) scanLoop() {
	for {
		select {
		case <-ste.ticker.C:
			ste.performScan()
		case <-ste.stopChan:
			return
		}
	}
}

// performScan 执行哨兵扫描
func (ste *SentinelTriggerEngine) performScan() {
	// 获取全局统计管理器
	globalStats := GetGlobalStatsManager()
	if globalStats == nil {
		return
	}
	
	// 获取订单流管理器
	ofm := GetGlobalOrderFlowManager()
	if ofm == nil {
		return
	}
	
	// 获取所有市场快照
	snapshots := ofm.GetAllMarketSnapshots()
	
	// 为每个symbol执行三大场景检测
	for symbol, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		
		// 获取Z-Score数据
		zScores := globalStats.GetSymbolZScores(symbol, snapshot)
		if len(zScores) == 0 {
			continue // 数据不足，跳过
		}
		
		// 🔧 P2-2修复：场景A: 动量点燃检测（独立冷却检查）
		if ste.checkCooldown(symbol, ScenarioMomentumIgnition) {
			if alert := ste.detectMomentumIgnition(symbol, snapshot, zScores); alert != nil {
				ste.triggerAlert(alert)
			}
		}
		
		// 🔧 P2-2修复：场景B: 现货抢跑检测（独立冷却检查）
		if ste.checkCooldown(symbol, ScenarioSpotRush) {
			if alert := ste.detectSpotRush(symbol, snapshot, zScores); alert != nil {
				ste.triggerAlert(alert)
			}
		}
		
		// 🔧 P2-2修复：场景C: 虚假翻转检测（独立冷却检查）
		if ste.checkCooldown(symbol, ScenarioSpoofingFlip) {
			if alert := ste.detectSpoofingFlip(symbol, snapshot, zScores); alert != nil {
				ste.triggerAlert(alert)
			}
		}
	}
}

// detectMomentumIgnition 检测场景A: 动量点燃（V-12.2动态阈值版本）
func (ste *SentinelTriggerEngine) detectMomentumIgnition(symbol string, snapshot *MarketSnapshot, zScores map[string]float64) *SentinelTriggerAlert {
	// V-12.2规格: CVD异常放量 + 突破性价格变化
	if snapshot.CVDDelta5m == nil {
		return nil
	}
	
	// 获取动态阈值
	dynamicThresholds := ste.dynamicThresholds.GetThresholds(symbol)
	var cvdThreshold, volumeThreshold float64
	
	if dynamicThresholds != nil {
		// 使用动态阈值
		cvdThreshold = dynamicThresholds.CVDThresholds["momentum_ignition"]
		volumeThreshold = dynamicThresholds.VolumeThresholds["abnormal_volume"]
		log.Printf("🎯 [%s] 使用动态阈值 - CVD:%.2f, Volume:%.2f (市场状态: %s)", 
			symbol, cvdThreshold, volumeThreshold, dynamicThresholds.CurrentRegime)
	} else {
		// 回退到静态阈值
		cvdThreshold = ste.momentumThresholds.CVDZScoreMin
		volumeThreshold = ste.momentumThresholds.VolumeZScoreMin
		log.Printf("⚠️ [%s] 使用静态阈值 - CVD:%.2f, Volume:%.2f", symbol, cvdThreshold, volumeThreshold)
	}
	
	// 获取关键Z-Score指标
	spotCVDZScore := zScores["spot_cvd_1m"]
	futuresCVDZScore := zScores["futures_cvd_1m"] 
	volumeZScore := zScores["volume_1m"]
	priceZScore := zScores["price_change_1m"] // 🔧 P0-2修复：获取价格变化Z-Score
	
	// 🔥 P0-2修复前的业务逻辑：ATR绝对值过滤依然保留，但作为辅助条件
	priceChangeAbs := math.Abs(snapshot.CVDDelta5m.PriceDeltaPct)
	minATRThreshold := 0.1 // 🔧 降低最小波动率要求到0.1%，避免过度过滤
	
	// 如果价格变化太小，即使Z-Score很高也可能是噪音
	if priceChangeAbs < minATRThreshold {
		log.Printf("🔍 [%s] ATR过滤: 价格变化%.3f%% < 最小阈值%.1f%%, 跳过触发", 
			symbol, priceChangeAbs, minATRThreshold)
		return nil
	}
	
	// 🔥 业务逻辑修正2: 位置感知 - 高位提高触发门槛
	positionMultiplier := 1.0
	
	// 简化的位置判断：基于价格变化趋势
	if snapshot.CVDDelta5m.PriceDeltaPct > 2.0 { // 单周期涨幅>2%，可能是高位加速
		positionMultiplier = 1.5 // 提高50%的触发门槛
		log.Printf("🏔️ [%s] 高位加速检测 - 价格涨幅%.2f%%, 触发门槛提升至%.1fx", 
			symbol, snapshot.CVDDelta5m.PriceDeltaPct, positionMultiplier)
	}
	
	// 应用位置调整后的阈值
	adjustedCVDThreshold := cvdThreshold * positionMultiplier
	adjustedVolumeThreshold := volumeThreshold * positionMultiplier
	adjustedPriceThreshold := ste.momentumThresholds.PriceZScoreMin * positionMultiplier // 🔧 P0-2修复：价格Z-Score阈值
	
	// 🔧 P0-2修复：检查动量点燃条件（使用Z-Score替代硬编码百分比）
	cvdCondition := spotCVDZScore >= adjustedCVDThreshold || futuresCVDZScore >= adjustedCVDThreshold
	volumeCondition := volumeZScore >= adjustedVolumeThreshold
	priceCondition := math.Abs(priceZScore) >= adjustedPriceThreshold // 关键修复：用Z-Score替代硬编码1%
	
	if !cvdCondition || !volumeCondition || !priceCondition {
		return nil
	}
	
	// 计算置信度（考虑动态阈值调整）
	confidence := ste.calculateMomentumConfidence(snapshot, zScores)
	
	// 根据市场状态调整置信度阈值
	minConfidence := 0.6
	if dynamicThresholds != nil && dynamicThresholds.CurrentRegime == RegimeHighVolatility {
		minConfidence = 0.7 // 高波动市场需要更高置信度
	}
	
	if confidence < minConfidence {
		return nil
	}
	
	return &SentinelTriggerAlert{
		Symbol:      symbol,
		Scenario:    ScenarioMomentumIgnition,
		Timestamp:   time.Now(),
		Confidence:  confidence,
		ZScoreData:  zScores,
		Description: "检测到动量点燃信号: CVD异常放量伴随突破性价格变化",
		Metadata: map[string]interface{}{
			"spot_cvd_zscore":      spotCVDZScore,
			"futures_cvd_zscore":   futuresCVDZScore,
			"volume_zscore":        volumeZScore,
			"price_zscore":         priceZScore, // 🔧 P0-2修复：记录价格Z-Score
			"price_change_pct":     snapshot.CVDDelta5m.PriceDeltaPct,
			"volume_delta":         snapshot.CVDDelta5m.VolumeDelta,
			"used_cvd_threshold":   cvdThreshold,
			"used_volume_threshold": volumeThreshold,
			"used_price_threshold": adjustedPriceThreshold, // 🔧 P0-2修复：记录价格Z-Score阈值
			"market_regime":        func() string {
				if dynamicThresholds != nil {
					return string(dynamicThresholds.CurrentRegime)
				}
				return "static"
			}(),
		},
	}
}

// detectSpotRush 检测场景B: 现货抢跑
func (ste *SentinelTriggerEngine) detectSpotRush(symbol string, snapshot *MarketSnapshot, zScores map[string]float64) *SentinelTriggerAlert {
	// V-12.2规格: 现货CVD激增 + 合约CVD相对减弱 + CVD比率异常
	if snapshot.CVDDelta5m == nil {
		return nil
	}
	
	// 获取关键Z-Score指标
	spotCVDZScore := zScores["spot_cvd_1m"]
	futuresCVDZScore := zScores["futures_cvd_1m"]
	cvdRatioZScore := zScores["cvd_ratio"]
	
	// 🔥 业务逻辑修正: CVD合力判断，不只看大小，要看方向
	spotCVDValue := snapshot.CVDDelta5m.SpotCVDDeltaUSD
	futuresCVDValue := snapshot.CVDDelta5m.FuturesCVDDeltaUSD
	netCVD := spotCVDValue + futuresCVDValue
	
	// 检查现货抢跑条件（修正版）
	spotCondition := spotCVDZScore >= ste.spotRushThresholds.SpotCVDZScoreMin
	
	// 🔧 P1-2修复: 允许双核驱动场景，不再要求现货必须主导
	// 场景A: 现货主导 - 现货强势，期货跟随或中性
	spotLeading := netCVD > 0 && spotCVDValue > math.Abs(futuresCVDValue)
	
	// 🔧 P1-2新增: 场景B: 双核驱动 - 现货期货都强力买入，合力明显
	dualCoreCondition := spotCVDValue > 0 && futuresCVDValue > 0 && netCVD > 0
	dualCoreStrength := dualCoreCondition && (spotCVDValue > 50000 && futuresCVDValue > 50000) // 双方都有一定强度
	
	// 🔧 P1-2修复: 支持现货主导或双核驱动两种模式
	netForceCondition := spotLeading || dualCoreStrength
	ratioCondition := cvdRatioZScore >= ste.spotRushThresholds.CVDRatioZScoreMin
	
	// 添加对抗检测：如果现货和合约方向相反且力量接近，可能是对抗而非抢跑
	if spotCVDValue > 0 && futuresCVDValue < 0 {
		oppositionRatio := math.Abs(futuresCVDValue) / spotCVDValue
		if oppositionRatio > 0.7 { // 如果合约阻力>70%现货推力，认为是对抗
			log.Printf("⚔️ [%s] 现货vs合约对抗检测: 现货推力%.0f vs 合约阻力%.0f (阻力比%.2f), 跳过触发", 
				symbol, spotCVDValue, math.Abs(futuresCVDValue), oppositionRatio)
			return nil
		}
	}
	
	// 🔧 P1-2新增: 记录触发的具体模式，便于分析
	triggerMode := ""
	if spotLeading && !dualCoreStrength {
		triggerMode = "spot_leading"
	} else if dualCoreStrength {
		triggerMode = "dual_core_driving"
	}
	
	if !spotCondition || !netForceCondition || !ratioCondition {
		return nil
	}
	
	// 计算置信度
	confidence := ste.calculateSpotRushConfidence(snapshot, zScores)
	if confidence < 0.65 { // 现货抢跑要求更高置信度
		return nil
	}
	
	return &SentinelTriggerAlert{
		Symbol:      symbol,
		Scenario:    ScenarioSpotRush,
		Timestamp:   time.Now(),
		Confidence:  confidence,
		ZScoreData:  zScores,
		Description: fmt.Sprintf("检测到现货抢跑信号 (%s): %s", triggerMode, 
			func() string {
				if triggerMode == "spot_leading" {
					return "现货主导式资金流入"
				} else if triggerMode == "dual_core_driving" {
					return "现货期货双核驱动"
				}
				return "现货强势资金流入"
			}()),
		Metadata: map[string]interface{}{
			"spot_cvd_zscore":    spotCVDZScore,
			"futures_cvd_zscore": futuresCVDZScore,
			"cvd_ratio_zscore":   cvdRatioZScore,
			"spot_cvd_delta":     snapshot.CVDDelta5m.SpotCVDDeltaUSD,
			"futures_cvd_delta":  snapshot.CVDDelta5m.FuturesCVDDeltaUSD,
			"trigger_mode":       triggerMode, // 🔧 P1-2新增: 记录触发模式
			"net_cvd":           netCVD,
			"spot_leading":       spotLeading,
			"dual_core_strength": dualCoreStrength,
		},
	}
}

// detectSpoofingFlip 检测场景C: 虚假翻转
func (ste *SentinelTriggerEngine) detectSpoofingFlip(symbol string, snapshot *MarketSnapshot, zScores map[string]float64) *SentinelTriggerAlert {
	// V-12.2规格: 虚假挂单风险激增 + 流动性骤降 + 失衡比率异常
	if snapshot.OrderBookData == nil {
		return nil
	}
	
	// 获取关键Z-Score指标
	spoofingZScore := zScores["spoofing_risk"]
	liquidityZScore := zScores["liquidity_score"]
	imbalanceZScore := zScores["imbalance_ratio"]
	
	// 🔥 业务逻辑修正: 欺诈反转必须结合CVD方向验证
	spotCVDZScore := zScores["spot_cvd_1m"]
	futuresCVDZScore := zScores["futures_cvd_1m"]
	
	// 检查虚假翻转基础条件
	spoofingCondition := spoofingZScore >= ste.spoofingThresholds.SpoofingZScoreMin
	liquidityCondition := liquidityZScore <= ste.spoofingThresholds.LiquidityZScoreMax
	imbalanceCondition := math.Abs(imbalanceZScore) >= ste.spoofingThresholds.ImbalanceZScoreMin
	
	if !spoofingCondition || !liquidityCondition || !imbalanceCondition {
		return nil
	}
	
	// 🔥 关键修正: CVD方向校验，区分真突破vs诱多陷阱
	priceDirection := 1.0
	if snapshot.CVDDelta5m != nil && snapshot.CVDDelta5m.PriceDeltaPct < 0 {
		priceDirection = -1.0
	}
	
	// 计算CVD合力方向
	avgCVD := (spotCVDZScore + futuresCVDZScore) / 2
	
	// 如果价格向上但CVD没有跟上，或价格向下但CVD异常强劲，可能是虚假信号
	if priceDirection > 0 && avgCVD < 0.5 {
		log.Printf("🎭 [%s] 疑似诱多陷阱: 价格上涨但CVD疲软 (CVD平均Z: %.2f)", symbol, avgCVD)
		// 不直接返回nil，而是降低置信度要求，让后续置信度计算处理
	} else if priceDirection < 0 && avgCVD > 0.5 {
		log.Printf("🎭 [%s] 疑似诱空陷阱: 价格下跌但CVD强劲 (CVD平均Z: %.2f)", symbol, avgCVD)
	}
	
	// 计算置信度
	confidence := ste.calculateSpoofingConfidence(snapshot, zScores)
	if confidence < 0.7 { // 虚假翻转要求最高置信度
		return nil
	}
	
	return &SentinelTriggerAlert{
		Symbol:      symbol,
		Scenario:    ScenarioSpoofingFlip,
		Timestamp:   time.Now(),
		Confidence:  confidence,
		ZScoreData:  zScores,
		Description: "检测到虚假翻转信号: 疑似操纵性盘口行为",
		Metadata: map[string]interface{}{
			"spoofing_zscore":   spoofingZScore,
			"liquidity_zscore":  liquidityZScore,
			"imbalance_zscore":  imbalanceZScore,
			"spoofing_risk":     snapshot.OrderBookData.SpoofingRisk,
			"liquidity_score":   snapshot.OrderBookData.LiquidityScore,
			"imbalance_ratio":   snapshot.OrderBookData.ImbalanceRatio,
		},
	}
}

// calculateMomentumConfidence 计算动量点燃置信度
func (ste *SentinelTriggerEngine) calculateMomentumConfidence(snapshot *MarketSnapshot, zScores map[string]float64) float64 {
	if snapshot.CVDDelta5m == nil {
		return 0
	}
	
	confidence := 0.0
	
	// CVD强度贡献 (40%)
	maxCVDZScore := math.Max(zScores["spot_cvd_1m"], zScores["futures_cvd_1m"])
	cvdContrib := math.Min(1.0, maxCVDZScore/3.0) * 0.4
	confidence += cvdContrib
	
	// 成交量强度贡献 (30%)
	volumeContrib := math.Min(1.0, zScores["volume_1m"]/2.5) * 0.3
	confidence += volumeContrib
	
	// 价格变化贡献 (20%)
	priceContrib := math.Min(1.0, math.Abs(snapshot.CVDDelta5m.PriceDeltaPct)/2.0) * 0.2
	confidence += priceContrib
	
	// 一致性加成 (10%)
	if zScores["spot_cvd_1m"] > 1.5 && zScores["futures_cvd_1m"] > 1.5 {
		confidence += 0.1 // 现货合约同向加成
	}
	
	return math.Min(1.0, confidence)
}

// calculateSpotRushConfidence 计算现货抢跑置信度
func (ste *SentinelTriggerEngine) calculateSpotRushConfidence(snapshot *MarketSnapshot, zScores map[string]float64) float64 {
	if snapshot.CVDDelta5m == nil {
		return 0
	}
	
	confidence := 0.0
	
	// 现货主导强度 (40%)
	spotContrib := math.Min(1.0, zScores["spot_cvd_1m"]/2.5) * 0.4
	confidence += spotContrib
	
	// 合约相对弱势 (30%)
	futuresWeakness := math.Max(0, -zScores["futures_cvd_1m"]/2.0)
	futuresContrib := math.Min(1.0, futuresWeakness) * 0.3
	confidence += futuresContrib
	
	// CVD比率异常 (30%)
	ratioContrib := math.Min(1.0, zScores["cvd_ratio"]/2.0) * 0.3
	confidence += ratioContrib
	
	return math.Min(1.0, confidence)
}

// calculateSpoofingConfidence 计算虚假翻转置信度
func (ste *SentinelTriggerEngine) calculateSpoofingConfidence(snapshot *MarketSnapshot, zScores map[string]float64) float64 {
	if snapshot.OrderBookData == nil {
		return 0
	}
	
	confidence := 0.0
	
	// 虚假挂单风险 (40%)
	spoofingContrib := math.Min(1.0, zScores["spoofing_risk"]/2.0) * 0.4
	confidence += spoofingContrib
	
	// 流动性枯竭程度 (35%)
	liquidityWeakness := math.Max(0, -zScores["liquidity_score"]/1.5)
	liquidityContrib := math.Min(1.0, liquidityWeakness) * 0.35
	confidence += liquidityContrib
	
	// 失衡异常程度 (25%)
	imbalanceContrib := math.Min(1.0, math.Abs(zScores["imbalance_ratio"])/2.0) * 0.25
	confidence += imbalanceContrib
	
	return math.Min(1.0, confidence)
}

// 🔧 P2-2新增：生成冷却Key，实现Symbol+Scenario精细化冷却
func (ste *SentinelTriggerEngine) getCooldownKey(symbol string, scenario TriggerScenario) string {
	return fmt.Sprintf("%s:%s", symbol, scenario)
}

// checkCooldown 🔧 P2-2修复：检查警报冷却期（精细化到Symbol+Scenario）
func (ste *SentinelTriggerEngine) checkCooldown(symbol string, scenario TriggerScenario) bool {
	ste.mu.RLock()
	defer ste.mu.RUnlock()
	
	cooldownKey := ste.getCooldownKey(symbol, scenario)
	lastAlert, exists := ste.lastAlerts[cooldownKey]
	if !exists {
		return true
	}
	
	return time.Since(lastAlert) > ste.cooldownPeriod
}

// triggerAlert 🔧 P2-2修复：触发警报（使用Symbol+Scenario精细化冷却）
func (ste *SentinelTriggerEngine) triggerAlert(alert *SentinelTriggerAlert) {
	// 🔧 P2-2修复：使用Symbol+Scenario组合Key更新冷却记录
	ste.mu.Lock()
	cooldownKey := ste.getCooldownKey(alert.Symbol, alert.Scenario)
	ste.lastAlerts[cooldownKey] = alert.Timestamp
	ste.mu.Unlock()
	
	// 记录日志
	log.Printf("🚨 V-12.2哨兵警报 [%s] %s: %s (置信度: %.2f)", 
		alert.Symbol, alert.Scenario, alert.Description, alert.Confidence)
	
	// 调用所有警报处理器
	ste.mu.RLock()
	handlers := make([]func(*SentinelTriggerAlert), len(ste.alertHandlers))
	copy(handlers, ste.alertHandlers)
	ste.mu.RUnlock()
	
	for _, handler := range handlers {
		go func(h func(*SentinelTriggerAlert)) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️ 警报处理器异常: %v", r)
				}
			}()
			h(alert)
		}(handler)
	}
}

// GetAlertStats 获取警报统计
func (ste *SentinelTriggerEngine) GetAlertStats() map[string]interface{} {
	ste.mu.RLock()
	defer ste.mu.RUnlock()
	
	return map[string]interface{}{
		"is_running":        ste.isRunning,
		"cooldown_seconds":  int(ste.cooldownPeriod.Seconds()),
		"symbols_tracked":   len(ste.lastAlerts),
		"handlers_count":    len(ste.alertHandlers),
	}
}

// ===== 全局哨兵触发引擎实例 =====

var globalSentinelTriggerEngine *SentinelTriggerEngine
var sentinelTriggerOnce sync.Once

// GetGlobalSentinelTriggerEngine 获取全局哨兵触发引擎实例
func GetGlobalSentinelTriggerEngine() *SentinelTriggerEngine {
	sentinelTriggerOnce.Do(func() {
		globalSentinelTriggerEngine = NewSentinelTriggerEngine()
		log.Printf("✨ V-12.2哨兵触发引擎全局实例已创建")
	})
	return globalSentinelTriggerEngine
}