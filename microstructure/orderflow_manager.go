package microstructure

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ===== 订单流管理器主结构 =====

// OrderFlowManager 订单流分析管理器（统一管理所有组件）
type OrderFlowManager struct {
	// 核心组件
	wsManager           *WSManager
	cvdManager          *CVDManager
	oiManager           *OIManager
	orderBookManager    *OrderBookCalculator
	marketContextAnalyzer *MarketContextAnalyzer
	
	// 配置和状态
	config              *MicrostructureConfig
	mu                  sync.RWMutex
	isRunning           bool
	subscribedSymbols   map[string]bool // 已订阅的币种
	
	// 🆕 协程监控
	activeOIGoroutines  map[string]time.Time // symbol -> 上次活动时间
	goroutineCheckTicker *time.Ticker        // 协程检查定时器
	
	// 数据缓存（用于快速获取）
	latestPriceContext  map[string]*PriceContext // symbol -> PriceContext
	
	// 清理任务
	cleanupTicker       *time.Ticker
}

// NewOrderFlowManager 创建订单流管理器
func NewOrderFlowManager(config *MicrostructureConfig) *OrderFlowManager {
	if config == nil {
		config = DefaultMicrostructureConfig()
	}

	manager := &OrderFlowManager{
		config:              config,
		subscribedSymbols:   make(map[string]bool),
		latestPriceContext:  make(map[string]*PriceContext),
		activeOIGoroutines:  make(map[string]time.Time), // 🆕 初始化协程监控
	}

	// 初始化各个组件
	manager.initializeComponents()
	
	return manager
}

// initializeComponents 初始化各个组件
func (ofm *OrderFlowManager) initializeComponents() {
	// 创建WebSocket管理器
	ofm.wsManager = NewWSManager(ofm.config)
	
	// 创建CVD管理器
	ofm.cvdManager = NewCVDManager(ofm.config)
	
	// 创建OI管理器
	ofm.oiManager = NewOIManager(ofm.config)
	
	// 创建盘口分析管理器（支持多交易对）
	ofm.orderBookManager = NewOrderBookCalculator(5.0)
	
	// 创建市场上下文分析器
	ofm.marketContextAnalyzer = NewMarketContextAnalyzer()
	
	// 设置WebSocket数据处理器
	ofm.setupWebSocketHandlers()
	
	log.Printf("✅ 订单流管理器组件初始化完成")
}

// setupWebSocketHandlers 设置WebSocket数据处理器
func (ofm *OrderFlowManager) setupWebSocketHandlers() {
	// 设置交易数据处理器
	ofm.wsManager.SetTradeHandler(func(tradeData *TradeData) {
		ofm.cvdManager.ProcessTrade(tradeData)
		
		// 更新价格上下文
		ofm.updatePriceContext(tradeData.Symbol, tradeData.Price)
	})
	
	// 设置盘口数据处理器
	ofm.wsManager.SetDepthHandler(func(depthData *DepthData) {
		ofm.orderBookManager.ProcessDepthData(depthData.Symbol, depthData)
	})
	
	log.Printf("✅ WebSocket数据处理器设置完成")
}

// updatePriceContext 更新价格上下文
func (ofm *OrderFlowManager) updatePriceContext(symbol string, currentPrice float64) {
	ofm.mu.Lock()
	defer ofm.mu.Unlock()
	
	// 获取或创建价格上下文
	priceCtx, exists := ofm.latestPriceContext[symbol]
	if !exists {
		priceCtx = &PriceContext{
			CurrentPrice: currentPrice,
			Change1H:     0,
			Change4H:     0,
			Volatility:   0,
		}
		ofm.latestPriceContext[symbol] = priceCtx
	} else {
		// 更新当前价格（这里是简化实现，实际应该基于历史数据计算变化率）
		priceCtx.CurrentPrice = currentPrice
	}
}

// Start 启动订单流管理器
func (ofm *OrderFlowManager) Start() error {
	ofm.mu.Lock()
	defer ofm.mu.Unlock()
	
	if ofm.isRunning {
		return fmt.Errorf("订单流管理器已在运行")
	}
	
	// 启动WebSocket管理器
	ofm.wsManager.Start()
	
	// 启动清理定时器
	ofm.cleanupTicker = time.NewTicker(60 * time.Minute) // 每60分钟清理一次
	go ofm.runCleanupLoop()
	
	// 🆕 启动协程监控定时器
	ofm.goroutineCheckTicker = time.NewTicker(1 * time.Minute) // 每1分钟检查一次协程状态
	go ofm.runGoroutineMonitor()
	
	ofm.isRunning = true
	log.Printf("🚀 订单流管理器启动完成")
	
	return nil
}

// Stop 停止订单流管理器
func (ofm *OrderFlowManager) Stop() {
	// 🆕🆕🆕 强制记录调用栈 - 多重日志确保能看到
	fmt.Printf("🚨🚨🚨 [CRITICAL] OrderFlowManager.Stop() 被调用！时间: %s\n", time.Now().Format("15:04:05.000"))
	fmt.Printf("🔍🔍🔍 [CRITICAL] 调用栈详情:\n%s\n", string(debug.Stack()))
	log.Printf("🚨🚨🚨 [CRITICAL] OrderFlowManager.Stop() 被调用！时间: %s", time.Now().Format("15:04:05.000"))
	log.Printf("🔍🔍🔍 [CRITICAL] 调用栈详情:\n%s", string(debug.Stack()))
	
	ofm.mu.Lock()
	defer ofm.mu.Unlock()
	
	if !ofm.isRunning {
		fmt.Printf("⚠️⚠️⚠️ [CRITICAL] OrderFlowManager已经停止，忽略重复调用\n")
		log.Printf("⚠️⚠️⚠️ [CRITICAL] OrderFlowManager已经停止，忽略重复调用")
		return
	}
	
	fmt.Printf("⛔⛔⛔ [CRITICAL] 开始停止OrderFlowManager...\n")
	log.Printf("⛔⛔⛔ [CRITICAL] 开始停止OrderFlowManager...")
	
	// 停止WebSocket管理器
	ofm.wsManager.Stop()
	
	// 停止清理定时器
	if ofm.cleanupTicker != nil {
		ofm.cleanupTicker.Stop()
	}
	
	// 🆕 停止协程监控定时器
	if ofm.goroutineCheckTicker != nil {
		ofm.goroutineCheckTicker.Stop()
	}
	
	fmt.Printf("🚨🚨🚨 [CRITICAL] 即将设置isRunning=false！调用栈:\n%s\n", string(debug.Stack()))
	log.Printf("🚨🚨🚨 [CRITICAL] 即将设置isRunning=false！调用栈:\n%s", string(debug.Stack()))
	ofm.isRunning = false
	fmt.Printf("⛔⛔⛔ [CRITICAL] 订单流管理器已停止\n")
	log.Printf("⛔⛔⛔ [CRITICAL] 订单流管理器已停止")
}

// SubscribeSymbol 订阅币种的订单流数据
func (ofm *OrderFlowManager) SubscribeSymbol(symbol string) error {
	symbol = strings.ToUpper(symbol)
	if !strings.HasSuffix(symbol, "USDT") {
		symbol += "USDT"
	}
	
	ofm.mu.Lock()
	if ofm.subscribedSymbols[symbol] {
		ofm.mu.Unlock()
		return nil // 已订阅
	}
	ofm.subscribedSymbols[symbol] = true
	// 🆕 初始化协程监控记录
	ofm.activeOIGoroutines[symbol] = time.Now()
	ofm.mu.Unlock()
	
	// 订阅各种数据流
	if err := ofm.wsManager.Subscribe(symbol); err != nil {
		return fmt.Errorf("订阅WebSocket数据流失败: %w", err)
	}
	
	// 订阅OI数据流（扩展WebSocket管理器功能）
	if err := ofm.subscribeOIStream(symbol); err != nil {
		log.Printf("⚠️ 订阅OI数据流失败 %s: %v", symbol, err)
	}
	
	log.Printf("✅ 成功订阅币种: %s", symbol)
	return nil
}

// subscribeOIStream 订阅OI数据流（内部方法）
func (ofm *OrderFlowManager) subscribeOIStream(symbol string) error {
	// 启动定期获取OI数据的协程
	go func() {
		log.Printf("🔄 [%s] OI更新协程启动", symbol)
		ticker := time.NewTicker(5 * time.Minute) // 每5分钟获取一次OI数据
		defer ticker.Stop()
		
		// 🆕 增强的协程退出检测和日志
		defer func() {
			// 检查协程退出原因
			ofm.mu.Lock() // 使用Lock而不是RLock，因为需要删除监控记录
			globalRunning := ofm.isRunning
			symbolSubscribed := ofm.subscribedSymbols[symbol]
			
			// 🆕 清理协程监控记录
			delete(ofm.activeOIGoroutines, symbol)
			
			ofm.mu.Unlock()
			
			if globalRunning && symbolSubscribed {
				// 如果全局还在运行且币种还在订阅中，但协程退出了，这是异常情况
				log.Printf("❌ [%s] OI更新协程异常退出！全局运行状态: %v, 币种订阅状态: %v", 
					symbol, globalRunning, symbolSubscribed)
				log.Printf("❌ [%s] 这可能导致订单流数据过期问题，需要检查系统状态", symbol)
			} else if !globalRunning {
				log.Printf("⛔ [%s] OI更新协程正常退出: 全局OrderFlowManager已停止", symbol)
			} else if !symbolSubscribed {
				log.Printf("✅ [%s] OI更新协程正常退出: 币种已取消订阅", symbol)
			} else {
				log.Printf("⚠️ [%s] OI更新协程退出", symbol)
			}
		}()
		
		// 立即获取一次数据
		log.Printf("🔄 [%s] 执行立即OI获取", symbol)
		ofm.fetchAndProcessOIData(symbol)
		
		for range ticker.C {
			log.Printf("🔄 [%s] 定时器触发，准备获取OI数据", symbol)
			
			// 🔧 修复: 检查币种订阅状态而不是全局运行状态，防止单个trader停止影响全局系统
			ofm.mu.RLock()
			globalRunning := ofm.isRunning
			symbolSubscribed := ofm.subscribedSymbols[symbol]
			ofm.mu.RUnlock()
			
			log.Printf("🔄 [%s] 检查状态: 全局运行=%v, 币种订阅=%v", symbol, globalRunning, symbolSubscribed)
			
			// 只有在全局停止或币种取消订阅时才退出
			if !globalRunning {
				log.Printf("⛔ [%s] 全局OrderFlowManager已停止，协程退出", symbol)
				return
			}
			
			if !symbolSubscribed {
				log.Printf("⚠️ [%s] 币种已取消订阅，协程退出", symbol)
				return
			}
			
			log.Printf("🔄 [%s] 开始获取OI数据", symbol)
			ofm.fetchAndProcessOIData(symbol)
			log.Printf("🔄 [%s] OI数据获取完成", symbol)
		}
	}()
	
	return nil
}

// fetchAndProcessOIData 获取并处理OI数据
func (ofm *OrderFlowManager) fetchAndProcessOIData(symbol string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 获取OI数据异常 %s: %v", symbol, r)
		}
	}()
	
	// 🆕 记录协程活动时间
	ofm.mu.Lock()
	ofm.activeOIGoroutines[symbol] = time.Now()
	ofm.mu.Unlock()
	
	// 调用币安API获取OI数据
	oiData, err := ofm.getOpenInterestFromAPI(symbol)
	if err != nil {
		log.Printf("❌ 获取OI数据失败 %s: %v", symbol, err)
		return
	}
	
	// 传递给OI管理器处理
	ofm.oiManager.ProcessOIData(symbol, oiData)
	log.Printf("✅ 成功获取并处理OI数据 %s: %.0f", symbol, oiData.OpenInterest)
}

// getOpenInterestFromAPI 从币安API获取持仓量数据
func (ofm *OrderFlowManager) getOpenInterestFromAPI(symbol string) (*OIData, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/openInterest?symbol=%s", symbol)
	
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("API请求失败: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API返回错误状态码: %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	
	var result struct {
		OpenInterest string `json:"openInterest"`
		Symbol       string `json:"symbol"`
		Time         int64  `json:"time"`
	}
	
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	
	openInterest, err := strconv.ParseFloat(result.OpenInterest, 64)
	if err != nil {
		return nil, fmt.Errorf("解析持仓量失败: %w", err)
	}
	
	return &OIData{
		Symbol:       symbol,
		OpenInterest: openInterest,
		Timestamp:    time.Now(),
	}, nil
}

// GetMarketSnapshot 获取市场快照（供AI使用）
func (ofm *OrderFlowManager) GetMarketSnapshot(symbol string) *MarketSnapshot {
	symbol = strings.ToUpper(symbol)
	if !strings.HasSuffix(symbol, "USDT") {
		symbol += "USDT"
	}
	
	// 获取各个组件的数据
	cvdData := ofm.cvdManager.GetCVDData(symbol)
	oiAnalysis := ofm.oiManager.GetOIAnalysis(symbol)
	orderBookData := ofm.orderBookManager.GetCurrentOrderBookData(symbol, 5)
	
	// 获取价格上下文
	ofm.mu.RLock()
	priceContext, exists := ofm.latestPriceContext[symbol]
	ofm.mu.RUnlock()
	
	if !exists {
		priceContext = &PriceContext{
			CurrentPrice: 0,
			Change1H:     0,
			Change4H:     0,
			Volatility:   0,
		}
	}
	
	// 生成市场上下文分析
	marketContext := ofm.marketContextAnalyzer.AnalyzeMarketContext(
		symbol, cvdData, oiAnalysis, priceContext,
	)
	
	return &MarketSnapshot{
		Symbol:         symbol,
		Timestamp:      time.Now(),
		CVDData:        cvdData,
		OIAnalysis:     oiAnalysis,
		OrderBookData:  orderBookData,
		MarketContext:  marketContext,
		PriceContext:   priceContext,
	}
}

// GetAllMarketSnapshots 获取所有订阅币种的市场快照
func (ofm *OrderFlowManager) GetAllMarketSnapshots() map[string]*MarketSnapshot {
	ofm.mu.RLock()
	symbols := make([]string, 0, len(ofm.subscribedSymbols))
	for symbol := range ofm.subscribedSymbols {
		symbols = append(symbols, symbol)
	}
	ofm.mu.RUnlock()
	
	snapshots := make(map[string]*MarketSnapshot)
	for _, symbol := range symbols {
		snapshots[symbol] = ofm.GetMarketSnapshot(symbol)
	}
	
	return snapshots
}

// runCleanupLoop 运行清理循环
func (ofm *OrderFlowManager) runCleanupLoop() {
	for range ofm.cleanupTicker.C {
		ofm.performCleanup()
	}
}

// 🆕 runGoroutineMonitor 运行协程监控
func (ofm *OrderFlowManager) runGoroutineMonitor() {
	for range ofm.goroutineCheckTicker.C {
		ofm.checkGoroutineHealth()
	}
}

// 🆕 checkGoroutineHealth 检查协程健康状态
func (ofm *OrderFlowManager) checkGoroutineHealth() {
	ofm.mu.RLock()
	defer ofm.mu.RUnlock()
	
	now := time.Now()
	deadlineMinutes := 8 // 如果8分钟内没有活动，则认为协程可能已死
	
	log.Printf("🔍 检查OI协程健康状态...")
	
	deadGoroutines := 0
	totalGoroutines := 0
	
	for symbol, isSubscribed := range ofm.subscribedSymbols {
		if !isSubscribed {
			continue // 跳过已取消订阅的币种
		}
		
		totalGoroutines++
		lastActive, exists := ofm.activeOIGoroutines[symbol]
		
		if !exists {
			log.Printf("❌ [%s] OI协程监控记录不存在，协程可能未正常启动", symbol)
			deadGoroutines++
			continue
		}
		
		inactiveMinutes := now.Sub(lastActive).Minutes()
		
		if inactiveMinutes > float64(deadlineMinutes) {
			log.Printf("❌ [%s] OI协程可能已死！上次活动: %.1f分钟前", symbol, inactiveMinutes)
			log.Printf("❌ [%s] 这会导致30分钟后订单流数据过期，需要重启系统或重新订阅", symbol)
			deadGoroutines++
		} else if inactiveMinutes > 6 { // 6分钟给出警告
			log.Printf("⚠️ [%s] OI协程活动缓慢，上次活动: %.1f分钟前", symbol, inactiveMinutes)
		}
	}
	
	if deadGoroutines > 0 {
		log.Printf("❌ 协程健康检查完成: %d/%d 协程异常，系统可能需要重启", deadGoroutines, totalGoroutines)
	} else if totalGoroutines > 0 {
		log.Printf("✅ 协程健康检查完成: 所有 %d 个OI协程运行正常", totalGoroutines)
	} else {
		log.Printf("⚠️ 协程健康检查完成: 没有活跃的OI协程")
	}
}

// performCleanup 执行清理任务
func (ofm *OrderFlowManager) performCleanup() {
	log.Printf("🧹 开始执行订单流数据清理...")
	
	// 清理各个管理器中的过期数据
	ofm.cvdManager.Cleanup()
	ofm.oiManager.Cleanup()
	// OrderBookCalculator 不需要Cleanup方法
	// ofm.orderBookManager.Cleanup()
	
	// 清理价格上下文中的过期数据
	ofm.cleanupPriceContexts()
	
	log.Printf("✅ 订单流数据清理完成")
}

// cleanupPriceContexts 清理价格上下文
func (ofm *OrderFlowManager) cleanupPriceContexts() {
	ofm.mu.Lock()
	defer ofm.mu.Unlock()
	
	// 检查哪些币种已经不再订阅
	for symbol := range ofm.latestPriceContext {
		if !ofm.subscribedSymbols[symbol] {
			delete(ofm.latestPriceContext, symbol)
			log.Printf("🗑️  清理价格上下文: %s", symbol)
		}
	}
}

// GetStatus 获取订单流管理器状态
func (ofm *OrderFlowManager) GetStatus() map[string]interface{} {
	ofm.mu.RLock()
	defer ofm.mu.RUnlock()
	
	// 获取WebSocket连接状态
	wsStatus := ofm.wsManager.GetConnectionStatus()
	
	return map[string]interface{}{
		"is_running":         ofm.isRunning,
		"subscribed_symbols": len(ofm.subscribedSymbols),
		"symbols":            ofm.getSubscribedSymbolsList(),
		"websocket_status":   wsStatus,
		"components": map[string]interface{}{
			"cvd_manager":       len(ofm.cvdManager.calculators),
			"oi_manager":        len(ofm.oiManager.calculators),
			"orderbook_manager": 1, // 单个OrderBookCalculator实例
		},
		"last_status_check": time.Now(),
	}
}

// getSubscribedSymbolsList 获取已订阅的币种列表
func (ofm *OrderFlowManager) getSubscribedSymbolsList() []string {
	symbols := make([]string, 0, len(ofm.subscribedSymbols))
	for symbol := range ofm.subscribedSymbols {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// ForceUpdateAllCVDDeltas 强制更新所有币种的5分钟CVD增量数据（用于K线收盘同步）
func (ofm *OrderFlowManager) ForceUpdateAllCVDDeltas() {
	ofm.mu.RLock()
	defer ofm.mu.RUnlock()
	
	if ofm.cvdManager != nil {
		ofm.cvdManager.ForceUpdateAllCVDDeltas()
	} else {
		log.Printf("⚠️ CVDManager为空，无法强制更新CVD增量数据")
	}
}

// ===== 市场快照结构 =====

// MarketSnapshot 市场快照（V2.0整合所有订单流数据）
type MarketSnapshot struct {
	Symbol         string           `json:"symbol"`
	Timestamp      time.Time        `json:"timestamp"`
	CVDData        *CVDData         `json:"cvd_data"`
	OIAnalysis     *OIAnalysis      `json:"oi_analysis"`
	OrderBookData  *OrderBookData   `json:"orderbook_data"`
	MarketContext  *MarketContext   `json:"market_context"`
	PriceContext   *PriceContext    `json:"price_context"`
	// V2.0 新增字段
	CVDDelta5m     *CVDDelta5m      `json:"cvd_delta_5m"`     // 5分钟增量数据
	MacroTrend     *MacroTrendData  `json:"macro_trend"`      // 宏观趋势数据
	DataQuality    *DataQualityInfo `json:"data_quality"`     // 数据质量评估
}

// ToAIPayload 转换为AI输入的JSON格式（V2.0结构）
func (ms *MarketSnapshot) ToAIPayload() map[string]interface{} {
	return map[string]interface{}{
		"订单流分析": map[string]interface{}{
			"本周期博弈_5m": func() map[string]interface{} {
				if ms.CVDDelta5m != nil {
					return map[string]interface{}{
						"price_delta_pct":         ms.CVDDelta5m.PriceDeltaPct,
						"spot_cvd_delta_usd":      ms.CVDDelta5m.SpotCVDDeltaUSD,
						"futures_cvd_delta_usd":   ms.CVDDelta5m.FuturesCVDDeltaUSD,
						"oi_delta_pct":            ms.CVDDelta5m.OIDeltaPct,
						"candle_intent":           ms.CVDDelta5m.CandleIntent,
						"volume_delta":            ms.CVDDelta5m.VolumeDelta,
						"volume_ratio":            ms.CVDDelta5m.VolumeRatio,
						"period_start":            ms.CVDDelta5m.PeriodStartTime.Format("15:04:05"),
						"period_end":              ms.CVDDelta5m.PeriodEndTime.Format("15:04:05"),
						"data_quality":            ms.CVDDelta5m.DataQuality,
					}
				}
				return map[string]interface{}{
					"status": "数据不可用",
				}
			}(),
			"宏观资金趋势": func() map[string]interface{} {
				if ms.MacroTrend != nil {
					return map[string]interface{}{
						"spot_cvd_1h_usd":       ms.MacroTrend.SpotCVD1H,
						"futures_cvd_1h_usd":    ms.MacroTrend.FuturesCVD1H,
						"cvd_divergence_4h":     ms.MacroTrend.CVDDivergence4H,
						"market_regime":         ms.MacroTrend.MarketRegime,
						"trend_strength":        ms.MacroTrend.TrendStrength,
						"dominant_direction":    ms.MacroTrend.DominantDirection,
						"confidence_level":      ms.MacroTrend.ConfidenceLevel,
					}
				}
				// 兜底使用原有CVD数据
				return map[string]interface{}{
					"spot_cvd_1h_usd":      ms.CVDData.SpotCVD1H,
					"futures_cvd_1h_usd":   ms.CVDData.FuturesCVD1H,
					"cvd_divergence":       ms.CVDData.CVDDivergence,
					"signal":               ms.CVDData.Signal,
				}
			}(),
			"盘口结构": map[string]interface{}{
				"imbalance_ratio": ms.OrderBookData.ImbalanceRatio,
				"resistance_wall": func() interface{} {
					if ms.OrderBookData.NearestResistance != nil {
						return map[string]interface{}{
							"price":           ms.OrderBookData.NearestResistance.Price,
							"strength_usd":    ms.OrderBookData.NearestResistance.StrengthUSD,
							"is_solid":        ms.OrderBookData.NearestResistance.IsSolid,
							"distance_pct":    ms.OrderBookData.NearestResistance.Distance,
							"stability_score": ms.OrderBookData.NearestResistance.StabilityScore,
							"flicker_count":   ms.OrderBookData.NearestResistance.FlickerCount,
						}
					}
					return nil
				}(),
				"support_wall": func() interface{} {
					if ms.OrderBookData.NearestSupport != nil {
						return map[string]interface{}{
							"price":           ms.OrderBookData.NearestSupport.Price,
							"strength_usd":    ms.OrderBookData.NearestSupport.StrengthUSD,
							"is_solid":        ms.OrderBookData.NearestSupport.IsSolid,
							"distance_pct":    ms.OrderBookData.NearestSupport.Distance,
							"stability_score": ms.OrderBookData.NearestSupport.StabilityScore,
							"flicker_count":   ms.OrderBookData.NearestSupport.FlickerCount,
						}
					}
					return nil
				}(),
				// V2.0 新增盘口分析字段
				"spoofing_risk":        ms.OrderBookData.SpoofingRisk,
				"liquidity_score":      ms.OrderBookData.LiquidityScore,
				"wall_change_count_5m": ms.OrderBookData.WallChangeCount5m,
				"imbalance_trend":      ms.OrderBookData.ImbalanceTrend,
			},
			"数据质量": func() map[string]interface{} {
				if ms.DataQuality != nil {
					return map[string]interface{}{
						"overall_score":          ms.DataQuality.OverallScore,
						"cvd_reliability":        ms.DataQuality.CVDReliability,
						"orderbook_reliability":  ms.DataQuality.OrderBookReliability,
						"oi_reliability":         ms.DataQuality.OIReliability,
						"data_lag_ms":            ms.DataQuality.DataLagMs,
						"status":                 ms.DataQuality.Status,
						"last_update":            ms.DataQuality.LastDataUpdate.Format("15:04:05"),
					}
				}
				// 兜底数据质量评估
				return map[string]interface{}{
					"cvd_stale":       ms.CVDData.IsStale,
					"oi_stale":        ms.OIAnalysis.IsStale,
					"orderbook_stale": ms.OrderBookData.IsStale,
					"status":          "V1.0兼容模式",
					"last_update":     ms.Timestamp.Format("15:04:05"),
				}
			}(),
		},
	}
}

// 全局订单流管理器实例
var globalOrderFlowManager *OrderFlowManager
var globalOFMOnce sync.Once

// GetGlobalOrderFlowManager 获取全局订单流管理器
func GetGlobalOrderFlowManager() *OrderFlowManager {
	globalOFMOnce.Do(func() {
		config := DefaultMicrostructureConfig()
		globalOrderFlowManager = NewOrderFlowManager(config)
	})
	return globalOrderFlowManager
}

// InitGlobalOrderFlowManager 初始化全局订单流管理器
func InitGlobalOrderFlowManager(config *MicrostructureConfig) error {
	if config == nil {
		config = DefaultMicrostructureConfig()
	}
	
	globalOrderFlowManager = NewOrderFlowManager(config)
	return globalOrderFlowManager.Start()
}

// StopGlobalOrderFlowManager 停止全局订单流管理器
func StopGlobalOrderFlowManager() {
	// 🆕🆕🆕 强制记录全局停止的调用栈 - 多重日志
	fmt.Printf("🚨🚨🚨 [CRITICAL] StopGlobalOrderFlowManager() 被调用！时间: %s\n", time.Now().Format("15:04:05.000"))
	fmt.Printf("🔍🔍🔍 [CRITICAL] 全局停止调用栈:\n%s\n", string(debug.Stack()))
	log.Printf("🚨🚨🚨 [CRITICAL] StopGlobalOrderFlowManager() 被调用！时间: %s", time.Now().Format("15:04:05.000"))
	log.Printf("🔍🔍🔍 [CRITICAL] 全局停止调用栈:\n%s", string(debug.Stack()))
	
	if globalOrderFlowManager != nil {
		fmt.Printf("🛑🛑🛑 [CRITICAL] 调用globalOrderFlowManager.Stop()\n")
		log.Printf("🛑🛑🛑 [CRITICAL] 调用globalOrderFlowManager.Stop()")
		globalOrderFlowManager.Stop()
	} else {
		fmt.Printf("⚠️⚠️⚠️ [CRITICAL] 全局OrderFlowManager为nil，无需停止\n")
		log.Printf("⚠️⚠️⚠️ [CRITICAL] 全局OrderFlowManager为nil，无需停止")
	}
}