package microstructure

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
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
	
	// 🔧 修复: 数据流健康监控
	dataFlowHealth      map[string]map[string]time.Time // symbol -> {trade/depth -> 最后数据时间}
	
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
		dataFlowHealth:      make(map[string]map[string]time.Time), // 🔧 修复: 初始化数据流监控
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
	
	// 🔧 修复：启动哨兵监控系统
	sentinelMonitor := GetGlobalSentinelMonitor()
	sentinelMonitor.Start()
	
	// V-12.2 启动哨兵触发引擎
	sentinelTrigger := GetGlobalSentinelTriggerEngine()
	sentinelTrigger.Start()
	// 添加默认警报处理器
	sentinelTrigger.AddAlertHandler(func(alert *SentinelTriggerAlert) {
		log.Printf("📢 V-12.2战术机会 [%s] %s: %s (置信度: %.1f%%)", 
			alert.Symbol, alert.Scenario, alert.Description, alert.Confidence*100)
	})
	
	// V-12.2 启动动态阈值系统
	dynamicThresholds := GetGlobalDynamicThresholdSystem()
	dynamicThresholds.Start()
	
	// 设置WebSocket数据处理器
	ofm.setupWebSocketHandlers()
	
	log.Printf("✅ 订单流管理器组件初始化完成")
}

// setupWebSocketHandlers 设置WebSocket数据处理器
func (ofm *OrderFlowManager) setupWebSocketHandlers() {
	// 设置交易数据处理器
	ofm.wsManager.SetTradeHandler(func(tradeData *TradeData) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ [关键错误] 交易数据处理器panic: %v, 数据: %+v", r, tradeData)
			}
		}()
		
		// 🔧 修复: 增加数据完整性验证
		if err := ofm.validateTradeData(tradeData); err != nil {
			log.Printf("❌ 交易数据验证失败 %s: %v", tradeData.Symbol, err)
			return
		}
		
		// 处理CVD数据并捕获错误
		if ofm.cvdManager != nil {
			ofm.cvdManager.ProcessTrade(tradeData)
		} else {
			log.Printf("⚠️ CVDManager为nil，跳过交易数据处理: %s", tradeData.Symbol)
		}
		
		// 更新价格上下文
		ofm.updatePriceContext(tradeData.Symbol, tradeData.Price)
		
		// 🔧 修复: 检测数据流中断
		ofm.updateDataFlowHealth(tradeData.Symbol, "trade")
	})
	
	// 设置盘口数据处理器
	ofm.wsManager.SetDepthHandler(func(depthData *DepthData) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ [关键错误] 盘口数据处理器panic: %v, 数据: %+v", r, depthData)
			}
		}()
		
		// 🔧 修复: 增加数据完整性验证
		if err := ofm.validateDepthData(depthData); err != nil {
			log.Printf("❌ 盘口数据验证失败 %s: %v", depthData.Symbol, err)
			return
		}
		
		// 处理盘口数据并捕获错误
		if ofm.orderBookManager != nil {
			ofm.orderBookManager.ProcessDepthData(depthData.Symbol, depthData)
		} else {
			log.Printf("⚠️ OrderBookManager为nil，跳过盘口数据处理: %s", depthData.Symbol)
		}
		
		// 🔧 修复: 检测数据流中断
		ofm.updateDataFlowHealth(depthData.Symbol, "depth")
	})
	
	log.Printf("✅ WebSocket数据处理器设置完成（增强错误处理）")
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
	
	// 🔧 修复：同时更新CVD管理器的价格历史，确保CVD计算器能正确计算价格变化
	if ofm.cvdManager != nil {
		ofm.cvdManager.UpdatePrice(symbol, currentPrice, time.Now())
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
	
	// 🔧 修复：停止哨兵监控系统
	sentinelMonitor := GetGlobalSentinelMonitor()
	sentinelMonitor.Stop()
	
	// V-12.2 停止哨兵触发引擎
	sentinelTrigger := GetGlobalSentinelTriggerEngine()
	sentinelTrigger.Stop()
	
	// V-12.2 停止动态阈值系统
	dynamicThresholds := GetGlobalDynamicThresholdSystem()
	dynamicThresholds.Stop()
	
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
		ticker := time.NewTicker(30 * time.Second) // 🔧 修复：提升OI数据更新频率至30秒
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
			
			// 🔧 修复：增强状态检查，防止协程意外退出
			ofm.mu.RLock()
			globalRunning := ofm.isRunning
			symbolSubscribed := ofm.subscribedSymbols[symbol]
			ofm.mu.RUnlock()
			
			log.Printf("🔄 [%s] 检查状态: 全局运行=%v, 币种订阅=%v", symbol, globalRunning, symbolSubscribed)
			
			// 🔧 修复：只有在明确停止时才退出，避免误判
			if !globalRunning {
				log.Printf("⛔ [%s] 全局OrderFlowManager已停止，协程退出", symbol)
				return
			}
			
			if !symbolSubscribed {
				log.Printf("⚠️ [%s] 币种已取消订阅，协程退出", symbol)
				return
			}
			
			// 🔧 修复：增加防护检查，确保实例仍然有效
			if ofm == nil {
				log.Printf("❌ [%s] OrderFlowManager实例为nil，协程异常退出", symbol)
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
		// 🔥 P0-05修复：双时间戳支持
		Timestamp:    time.Now(),                              // 接收/处理时间
		ExchangeTime: time.UnixMilli(result.Time),            // 交易所原始时间
	}, nil
}

// GetMarketSnapshot 获取市场快照（供AI使用）- 🚨 已弃用，请使用GetMarketSnapshotAt
// 🔥 P0-05风险：此方法使用time.Now()破坏5m K线收盘事件的时序对齐
func (ofm *OrderFlowManager) GetMarketSnapshot(symbol string) *MarketSnapshot {
	symbol = strings.ToUpper(symbol)
	if !strings.HasSuffix(symbol, "USDT") {
		symbol += "USDT"
	}
	
	// 🔥 P0-05风险：使用time.Now()而非事件时间
	return ofm.GetMarketSnapshotAt(symbol, time.Now())
}

// GetMarketSnapshotAt 获取指定事件时间的市场快照（V-12.3 P0-05修复版本）
// 🔥 P0-05修复：支持事件时间对齐，确保5m K线收盘事件的数据一致性
func (ofm *OrderFlowManager) GetMarketSnapshotAt(symbol string, eventTime time.Time) *MarketSnapshot {
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
	
	// 🔥 P0-05修复：使用事件时间而非time.Now()进行CVD增量计算
	cvdDelta5m := ofm.cvdManager.CalculateRealtimeCVDDelta5m(symbol, eventTime)
	
	// 生成宏观趋势数据
	macroTrend := ofm.generateMacroTrendData(symbol, cvdData, oiAnalysis)
	
	// 计算数据质量评估
	dataQuality := ofm.calculateDataQuality(symbol, cvdData, oiAnalysis, orderBookData)
	
	return &MarketSnapshot{
		Symbol:         symbol,
		// 🔥 P0-05修复：使用事件时间而非time.Now()
		Timestamp:      eventTime,
		CVDData:        cvdData,
		OIAnalysis:     oiAnalysis,
		OrderBookData:  orderBookData,
		MarketContext:  marketContext,
		PriceContext:   priceContext,
		// V2.0 新增字段 - 现在使用事件时间对齐
		CVDDelta5m:     cvdDelta5m,
		MacroTrend:     macroTrend,
		DataQuality:    dataQuality,
	}
}

// GetAllMarketSnapshots 获取所有订阅币种的市场快照 - 🚨 已弃用，请使用GetAllMarketSnapshotsAt
// 🔥 P0-05风险：此方法使用time.Now()破坏5m K线收盘事件的时序对齐
func (ofm *OrderFlowManager) GetAllMarketSnapshots() map[string]*MarketSnapshot {
	// 🔥 P0-05风险：使用time.Now()而非事件时间
	return ofm.GetAllMarketSnapshotsAt(time.Now())
}

// GetAllMarketSnapshotsAt 获取所有订阅币种在指定事件时间的市场快照（V-12.3 P0-05修复版本）
// 🔥 P0-05修复：支持事件时间对齐，确保5m K线收盘事件的数据一致性
func (ofm *OrderFlowManager) GetAllMarketSnapshotsAt(eventTime time.Time) map[string]*MarketSnapshot {
	ofm.mu.RLock()
	symbols := make([]string, 0, len(ofm.subscribedSymbols))
	for symbol := range ofm.subscribedSymbols {
		symbols = append(symbols, symbol)
	}
	ofm.mu.RUnlock()
	
	snapshots := make(map[string]*MarketSnapshot)
	for _, symbol := range symbols {
		// 🔥 P0-05修复：使用事件时间而非time.Now()
		snapshots[symbol] = ofm.GetMarketSnapshotAt(symbol, eventTime)
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

// ForceUpdateAllCVDDeltasWithKLineTime 使用精确K线收盘时间强制更新（🔧 新增：精确时序版本）
func (ofm *OrderFlowManager) ForceUpdateAllCVDDeltasWithKLineTime(klineCloseTime time.Time) {
	ofm.mu.RLock()
	defer ofm.mu.RUnlock()
	
	if ofm.cvdManager != nil {
		ofm.cvdManager.ForceUpdateAllCVDDeltasWithKLineTime(klineCloseTime)
	} else {
		log.Printf("⚠️ CVDManager为空，无法执行精确时序CVD增量数据更新")
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

// ToAIPayload 转换为AI输入的JSON格式（V2.0结构 + V-12.2增强）
func (ms *MarketSnapshot) ToAIPayload() map[string]interface{} {
	// 优先使用V-12.2 AI接口
	aiInterface := GetGlobalAIInterfaceV12()
	if aiInterface != nil {
		if contextV12, err := aiInterface.GenerateAIContext(ms.Symbol); err == nil {
			return contextV12.ToAIPromptFormat()
		}
	}
	
	// V2.0兜底格式
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
	
	// 🔧 修复：确保单例一致性，避免重复创建
	globalOFMOnce.Do(func() {
		globalOrderFlowManager = NewOrderFlowManager(config)
	})
	
	// 如果已经运行，直接返回
	if globalOrderFlowManager != nil {
		status := globalOrderFlowManager.GetStatus()
		if isRunning, ok := status["is_running"].(bool); ok && isRunning {
			log.Printf("✅ 全局OrderFlowManager已在运行，跳过重复启动")
			return nil
		}
		return globalOrderFlowManager.Start()
	}
	
	return fmt.Errorf("无法创建全局OrderFlowManager")
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

// ===== 🔧 修复: 数据完整性验证和健康监控方法 =====

// validateTradeData 验证交易数据完整性
func (ofm *OrderFlowManager) validateTradeData(tradeData *TradeData) error {
	if tradeData == nil {
		return fmt.Errorf("交易数据为nil")
	}
	if tradeData.Symbol == "" {
		return fmt.Errorf("交易对为空")
	}
	if tradeData.Price <= 0 {
		return fmt.Errorf("价格无效: %.8f", tradeData.Price)
	}
	if tradeData.Quantity <= 0 {
		return fmt.Errorf("数量无效: %.8f", tradeData.Quantity)
	}
	if tradeData.Timestamp.IsZero() {
		return fmt.Errorf("时间戳无效")
	}
	return nil
}

// validateDepthData 验证盘口数据完整性（V-12.3 P0-03修复版本）
func (ofm *OrderFlowManager) validateDepthData(depthData *DepthData) error {
	if depthData == nil {
		return fmt.Errorf("盘口数据为nil")
	}
	if depthData.Symbol == "" {
		return fmt.Errorf("交易对为空")
	}
	if len(depthData.Bids) == 0 && len(depthData.Asks) == 0 {
		return fmt.Errorf("买卖盘均为空")
	}
	
	// 🔥 P0-03修复：验证买盘数据并检查排序
	if len(depthData.Bids) > 0 {
		for i, bid := range depthData.Bids {
			if bid.Price <= 0 || bid.Quantity <= 0 {
				return fmt.Errorf("买单第%d档数据无效: 价格=%.8f, 数量=%.8f", i, bid.Price, bid.Quantity)
			}
			
			// 🔥 P0-03修复：检查买单价格是否按降序排列
			if i > 0 && bid.Price > depthData.Bids[i-1].Price {
				return fmt.Errorf("买单排序错误: 第%d档价格%.8f > 第%d档价格%.8f (应按降序)", 
					i, bid.Price, i-1, depthData.Bids[i-1].Price)
			}
		}
	}
	
	// 🔥 P0-03修复：验证卖盘数据并检查排序
	if len(depthData.Asks) > 0 {
		for i, ask := range depthData.Asks {
			if ask.Price <= 0 || ask.Quantity <= 0 {
				return fmt.Errorf("卖单第%d档数据无效: 价格=%.8f, 数量=%.8f", i, ask.Price, ask.Quantity)
			}
			
			// 🔥 P0-03修复：检查卖单价格是否按升序排列
			if i > 0 && ask.Price < depthData.Asks[i-1].Price {
				return fmt.Errorf("卖单排序错误: 第%d档价格%.8f < 第%d档价格%.8f (应按升序)", 
					i, ask.Price, i-1, depthData.Asks[i-1].Price)
			}
		}
	}
	
	// 🔥 P0-03修复：检查盘口交叉（bestBid >= bestAsk是异常情况）
	if len(depthData.Bids) > 0 && len(depthData.Asks) > 0 {
		bestBid := depthData.Bids[0].Price
		bestAsk := depthData.Asks[0].Price
		
		if bestBid >= bestAsk {
			return fmt.Errorf("盘口交叉异常: bestBid=%.8f >= bestAsk=%.8f", bestBid, bestAsk)
		}
		
		// 🔥 P0-03修复：检查价差是否合理（避免极端窄价差）
		spread := bestAsk - bestBid
		spreadPct := (spread / bestBid) * 100
		
		if spreadPct < 0.0001 { // 0.01bp以下可能是数据错误
			return fmt.Errorf("价差过窄可能异常: spread=%.8f (%.6f%%)", spread, spreadPct)
		}
		
		if spreadPct > 10.0 { // 超过10%价差可能是数据异常
			return fmt.Errorf("价差过宽可能异常: spread=%.8f (%.2f%%)", spread, spreadPct)
		}
	}
	
	return nil
}

// updateDataFlowHealth 更新数据流健康状态
func (ofm *OrderFlowManager) updateDataFlowHealth(symbol string, dataType string) {
	ofm.mu.Lock()
	defer ofm.mu.Unlock()
	
	if ofm.dataFlowHealth[symbol] == nil {
		ofm.dataFlowHealth[symbol] = make(map[string]time.Time)
	}
	
	ofm.dataFlowHealth[symbol][dataType] = time.Now()
}

// checkDataFlowHealth 检查数据流健康状态
func (ofm *OrderFlowManager) checkDataFlowHealth() {
	ofm.mu.RLock()
	defer ofm.mu.RUnlock()
	
	now := time.Now()
	staleThreshold := 2 * time.Minute // 2分钟无数据认为异常
	
	for symbol, health := range ofm.dataFlowHealth {
		for dataType, lastTime := range health {
			if now.Sub(lastTime) > staleThreshold {
				log.Printf("⚠️ [数据流异常] %s的%s数据已%.1f分钟无更新", 
					symbol, dataType, now.Sub(lastTime).Minutes())
			}
		}
	}
}

// ===== 🔧 修复: V2.0 MarketSnapshot 支持方法 =====

// generateMacroTrendData 生成宏观趋势数据
func (ofm *OrderFlowManager) generateMacroTrendData(symbol string, cvdData *CVDData, oiAnalysis *OIAnalysis) *MacroTrendData {
	if cvdData == nil || oiAnalysis == nil {
		return &MacroTrendData{
			SpotCVD1H:         0,
			FuturesCVD1H:      0,
			CVDDivergence4H:   false,
			MarketRegime:      "数据不足",
			TrendStrength:     0,
			DominantDirection: "neutral",
			ConfidenceLevel:   0.2,
			LastUpdate:        time.Now(),
		}
	}

	// 计算4小时级别背离
	cvdDivergence4H := ofm.detectCVDDivergence4H(cvdData)
	
	// 判断市场状态
	marketRegime := ofm.determineMarketRegime(cvdData, oiAnalysis)
	
	// 计算趋势强度
	trendStrength := ofm.calculateTrendStrength(cvdData, oiAnalysis)
	
	// 确定主导方向
	dominantDirection := ofm.determineDominantDirection(cvdData, oiAnalysis)
	
	// 计算置信度
	confidenceLevel := ofm.calculateConfidenceLevel(cvdData, oiAnalysis, trendStrength)

	return &MacroTrendData{
		SpotCVD1H:         cvdData.SpotCVD1H,
		FuturesCVD1H:      cvdData.FuturesCVD1H,
		CVDDivergence4H:   cvdDivergence4H,
		MarketRegime:      marketRegime,
		TrendStrength:     trendStrength,
		DominantDirection: dominantDirection,
		ConfidenceLevel:   confidenceLevel,
		LastUpdate:        time.Now(),
	}
}

// calculateDataQuality 计算数据质量评估（V-12.3 P0-06修复版本 - 真实组件更新时间）
func (ofm *OrderFlowManager) calculateDataQuality(symbol string, cvdData *CVDData, oiAnalysis *OIAnalysis, orderBookData *OrderBookData) *DataQualityInfo {
	// CVD数据可靠性
	cvdReliability := ofm.calculateCVDReliability(cvdData)
	
	// OI数据可靠性
	oiReliability := ofm.calculateOIReliability(oiAnalysis)
	
	// 盘口数据可靠性
	orderBookReliability := ofm.calculateOrderBookReliability(orderBookData)
	
	// 总体评分（加权平均）
	overallScore := (cvdReliability*0.4 + oiReliability*0.3 + orderBookReliability*0.3)
	
	// 🔥 P0-06修复：提取真实的组件更新时间
	var cvdLastUpdate, oiLastUpdate, orderBookLastUpdate time.Time
	var componentTimes []time.Time
	
	if cvdData != nil && !cvdData.LastUpdate.IsZero() {
		cvdLastUpdate = cvdData.LastUpdate
		componentTimes = append(componentTimes, cvdLastUpdate)
	}
	
	if oiAnalysis != nil && !oiAnalysis.LastUpdate.IsZero() {
		oiLastUpdate = oiAnalysis.LastUpdate
		componentTimes = append(componentTimes, oiLastUpdate)
	}
	
	if orderBookData != nil && !orderBookData.LastUpdate.IsZero() {
		orderBookLastUpdate = orderBookData.LastUpdate
		componentTimes = append(componentTimes, orderBookLastUpdate)
	}
	
	// 🔥 P0-06修复：LastDataUpdate = max(组件时间) 而非 time.Now()
	var lastDataUpdate time.Time
	if len(componentTimes) > 0 {
		lastDataUpdate = componentTimes[0]
		for _, t := range componentTimes[1:] {
			if t.After(lastDataUpdate) {
				lastDataUpdate = t
			}
		}
	} else {
		// 兜底：如果所有组件都无效，使用当前时间
		lastDataUpdate = time.Now()
	}
	
	// 🔥 P0-06修复：DataLagMs = now - min(组件时间) 衡量"最老组件落后多少"
	var dataLagMs int64
	if len(componentTimes) > 0 {
		now := time.Now()
		oldestComponentTime := componentTimes[0]
		for _, t := range componentTimes[1:] {
			if t.Before(oldestComponentTime) {
				oldestComponentTime = t
			}
		}
		dataLagMs = now.Sub(oldestComponentTime).Milliseconds()
	} else {
		// 兜底：如果没有有效组件，延迟设为最大值
		dataLagMs = 60000 // 1分钟
	}
	
	// 确定状态
	status := "正常"
	if overallScore < 0.5 {
		status = "异常"
	} else if dataLagMs > 5000 || overallScore < 0.7 {
		status = "延迟"
	}

	return &DataQualityInfo{
		CVDReliability:       cvdReliability,
		OrderBookReliability: orderBookReliability,
		OIReliability:        oiReliability,
		OverallScore:         overallScore,
		// 🔥 P0-06修复：使用真实的组件最新时间而非time.Now()
		LastDataUpdate:       lastDataUpdate,
		DataLagMs:            dataLagMs,
		Status:               status,
		// 🔥 P0-06修复：单独跟踪每个组件的更新时间
		CVDLastUpdate:        cvdLastUpdate,
		OILastUpdate:         oiLastUpdate,
		OrderBookLastUpdate:  orderBookLastUpdate,
	}
}

// detectCVDDivergence4H 🔧 P1-1修复：检测4小时级别CVD背离
// 使用相对统计方法替代硬编码绝对值，适应不同币种和市场状态
func (ofm *OrderFlowManager) detectCVDDivergence4H(cvdData *CVDData) bool {
	if cvdData == nil {
		return false
	}

	// 🔧 P1-1修复：使用Z-Score或相对值替代硬编码10万美元
	// 获取全局统计管理器来计算相对强度
	globalStats := GetGlobalStatsManager()
	if globalStats == nil {
		// 兜底：如果没有统计数据，使用改进的相对阈值
		return ofm.detectCVDDivergenceWithRelativeThreshold(cvdData)
	}
	
	// 基于统计分布判断方向强度
	spotDirection := ofm.classifyCVDDirection(cvdData.SpotCVD1H, "spot")
	futuresDirection := ofm.classifyCVDDirection(cvdData.FuturesCVD1H, "futures")
	
	// 🔧 P1-1修复：只有在两个方向都有明确统计意义时才判断背离
	// 这避免了低市值币的噪音和高市值币的误判
	if spotDirection == 0 || futuresDirection == 0 {
		return false // 至少有一方向不明确，无法判断背离
	}
	
	// 如果现货和期货方向明确且相反，认为有背离
	isDirectionOpposite := (spotDirection > 0 && futuresDirection < 0) || 
	                      (spotDirection < 0 && futuresDirection > 0)
	
	if isDirectionOpposite && os.Getenv("NOFX_DEBUG") == "true" {
		log.Printf("🔧 [P1-1] CVD背离检测: 现货方向%d, 期货方向%d (统计显著)", 
			spotDirection, futuresDirection)
	}
	
	return isDirectionOpposite
}

// detectCVDDivergenceWithRelativeThreshold 🔧 P1-1新增：基于相对阈值的背离检测（兜底方法）
func (ofm *OrderFlowManager) detectCVDDivergenceWithRelativeThreshold(cvdData *CVDData) bool {
	spotCVD := cvdData.SpotCVD1H
	futuresCVD := cvdData.FuturesCVD1H
	
	// 🔧 P1-1修复：计算相对阈值，而非固定10万美元
	// 使用两个CVD的平均绝对值作为基准
	avgAbsCVD := (math.Abs(spotCVD) + math.Abs(futuresCVD)) / 2
	
	// 相对阈值：平均值的20%，最小值为5万美元，最大值为50万美元
	relativeThreshold := math.Max(50000, math.Min(500000, avgAbsCVD*0.2))
	
	// 判断方向
	spotDirection := 0
	futuresDirection := 0
	
	if spotCVD > relativeThreshold {
		spotDirection = 1
	} else if spotCVD < -relativeThreshold {
		spotDirection = -1
	}
	
	if futuresCVD > relativeThreshold {
		futuresDirection = 1
	} else if futuresCVD < -relativeThreshold {
		futuresDirection = -1
	}
	
	log.Printf("🔧 [P1-1兜底] CVD背离检测: 现货%.0f(方向%d), 期货%.0f(方向%d), 阈值%.0f", 
		spotCVD, spotDirection, futuresCVD, futuresDirection, relativeThreshold)
	
	// 如果现货和期货方向相反，认为有背离
	return spotDirection != 0 && futuresDirection != 0 && spotDirection != futuresDirection
}

// classifyCVDDirection 🔧 P1-1新增：基于统计分布分类CVD方向
func (ofm *OrderFlowManager) classifyCVDDirection(cvdValue float64, cvdType string) int {
	// 简化实现：使用CVD绝对值的分位数来判断是否"统计显著"
	
	// 计算相对强度评分 (0-1)
	absValue := math.Abs(cvdValue)
	
	// 动态阈值：基于CVD值的对数特性
	// 小额CVD需要更低的阈值，大额CVD需要更高的阈值
	var significanceThreshold float64
	
	if absValue < 100000 { // 10万以下
		significanceThreshold = 50000 // 5万门槛
	} else if absValue < 1000000 { // 100万以下
		significanceThreshold = absValue * 0.3 // 30%的相对阈值
	} else { // 100万以上
		significanceThreshold = 500000 // 50万门槛
	}
	
	// 判断方向
	if cvdValue > significanceThreshold {
		return 1 // 多头
	} else if cvdValue < -significanceThreshold {
		return -1 // 空头
	} else {
		return 0 // 中性
	}
}

// determineMarketRegime 判断市场状态
func (ofm *OrderFlowManager) determineMarketRegime(cvdData *CVDData, oiAnalysis *OIAnalysis) string {
	if cvdData == nil || oiAnalysis == nil {
		return "数据不足"
	}

	// 基于CVD和OI变化判断市场状态
	spotCVD := cvdData.SpotCVD1H
	futuresCVD := cvdData.FuturesCVD1H
	oiChange := oiAnalysis.ChangeRate1H

	// 牛市确认：现货期货都买入，持仓增加
	if spotCVD > 200000 && futuresCVD > 200000 && oiChange > 2 {
		return "牛市确认"
	}

	// 熊市确认：现货期货都卖出，持仓增加
	if spotCVD < -200000 && futuresCVD < -200000 && oiChange > 2 {
		return "熊市确认"
	}

	// 整理状态：CVD较小，持仓变化不大
	if math.Abs(spotCVD) < 100000 && math.Abs(futuresCVD) < 100000 && math.Abs(oiChange) < 1 {
		return "横盘整理"
	}

	// 分歧状态：现货期货方向不一致
	if (spotCVD > 100000 && futuresCVD < -100000) || (spotCVD < -100000 && futuresCVD > 100000) {
		return "市场分歧"
	}

	return "观察阶段"
}

// calculateTrendStrength 计算趋势强度
func (ofm *OrderFlowManager) calculateTrendStrength(cvdData *CVDData, oiAnalysis *OIAnalysis) float64 {
	if cvdData == nil || oiAnalysis == nil {
		return 0
	}

	// 基于CVD绝对值和一致性计算强度
	spotStrength := math.Abs(cvdData.SpotCVD1H) / 500000 // 50万USD为满分
	futuresStrength := math.Abs(cvdData.FuturesCVD1H) / 1000000 // 100万USD为满分
	oiStrength := math.Abs(oiAnalysis.ChangeRate1H) / 10 // 10%变化为满分

	// 限制在0-1范围内
	spotStrength = math.Min(1.0, spotStrength)
	futuresStrength = math.Min(1.0, futuresStrength)
	oiStrength = math.Min(1.0, oiStrength)

	// 加权平均
	return (spotStrength*0.4 + futuresStrength*0.4 + oiStrength*0.2)
}

// determineDominantDirection 确定主导方向
func (ofm *OrderFlowManager) determineDominantDirection(cvdData *CVDData, oiAnalysis *OIAnalysis) string {
	if cvdData == nil || oiAnalysis == nil {
		return "neutral"
	}

	totalCVD := cvdData.SpotCVD1H + cvdData.FuturesCVD1H

	if totalCVD > 100000 {
		return "bullish"
	} else if totalCVD < -100000 {
		return "bearish"
	} else {
		return "neutral"
	}
}

// calculateConfidenceLevel 计算置信度
func (ofm *OrderFlowManager) calculateConfidenceLevel(cvdData *CVDData, oiAnalysis *OIAnalysis, trendStrength float64) float64 {
	if cvdData == nil || oiAnalysis == nil {
		return 0.2
	}

	confidence := 0.0

	// 基础置信度（趋势强度）
	confidence += trendStrength * 0.4

	// 数据新鲜度
	dataAge := time.Since(cvdData.LastUpdate).Minutes()
	if dataAge < 5 {
		confidence += 0.3
	} else if dataAge < 15 {
		confidence += 0.2
	} else {
		confidence += 0.1
	}

	// CVD和OI一致性
	spotDirection := 0.0
	if cvdData.SpotCVD1H > 0 { spotDirection = 1.0 } else if cvdData.SpotCVD1H < 0 { spotDirection = -1.0 }
	
	futuresDirection := 0.0
	if cvdData.FuturesCVD1H > 0 { futuresDirection = 1.0 } else if cvdData.FuturesCVD1H < 0 { futuresDirection = -1.0 }
	
	oiDirection := 0.0
	if oiAnalysis.ChangeRate1H > 0 { oiDirection = 1.0 } else if oiAnalysis.ChangeRate1H < 0 { oiDirection = -1.0 }

	if spotDirection == futuresDirection && spotDirection != 0 {
		confidence += 0.2 // CVD一致性
	}
	
	if (spotDirection == oiDirection || futuresDirection == oiDirection) && oiDirection != 0 {
		confidence += 0.1 // OI确认
	}

	return math.Min(0.95, math.Max(0.1, confidence))
}

// calculateCVDReliability 计算CVD数据可靠性
func (ofm *OrderFlowManager) calculateCVDReliability(cvdData *CVDData) float64 {
	if cvdData == nil || cvdData.IsStale {
		return 0.0
	}

	reliability := 1.0

	// 基于数据年龄计算可靠性
	dataAge := time.Since(cvdData.LastUpdate).Minutes()
	if dataAge > 30 {
		reliability = 0.0
	} else if dataAge > 15 {
		reliability = 0.3
	} else if dataAge > 5 {
		reliability = 0.7
	}

	return reliability
}

// calculateOIReliability 计算OI数据可靠性
func (ofm *OrderFlowManager) calculateOIReliability(oiAnalysis *OIAnalysis) float64 {
	if oiAnalysis == nil || oiAnalysis.IsStale {
		return 0.0
	}

	reliability := 1.0

	// 基于数据年龄计算可靠性
	dataAge := time.Since(oiAnalysis.LastUpdate).Minutes()
	if dataAge > 30 {
		reliability = 0.0
	} else if dataAge > 15 {
		reliability = 0.3
	} else if dataAge > 10 {
		reliability = 0.7
	}

	return reliability
}

// calculateOrderBookReliability 计算盘口数据可靠性
func (ofm *OrderFlowManager) calculateOrderBookReliability(orderBookData *OrderBookData) float64 {
	if orderBookData == nil || orderBookData.IsStale {
		return 0.0
	}

	reliability := 1.0

	// 基于数据年龄计算可靠性
	dataAge := time.Since(orderBookData.LastUpdate).Minutes()
	if dataAge > 30 {
		reliability = 0.0
	} else if dataAge > 10 {
		reliability = 0.5
	} else if dataAge > 2 {
		reliability = 0.8
	}

	// 基于流动性评分调整可靠性
	if orderBookData.LiquidityScore < 0.3 {
		reliability *= 0.7
	} else if orderBookData.LiquidityScore < 0.6 {
		reliability *= 0.9
	}

	return reliability
}

