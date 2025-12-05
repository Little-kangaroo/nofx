package trader

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"nofx/config"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// TrackedOrder 跟踪的订单信息
type TrackedOrder struct {
	OrderID     int64     `json:"order_id"`
	Symbol      string    `json:"symbol"`
	Side        string    `json:"side"`         // "long" or "short"
	Type        string    `json:"type"`         // "MARKET", "STOP_MARKET", "LIMIT"
	Quantity    float64   `json:"quantity"`
	StopPrice   float64   `json:"stop_price"`   // 止损价格(仅止损单有效)
	TradeID     string    `json:"trade_id"`     // 关联的数据库trade记录ID
	CreatedAt   time.Time `json:"created_at"`
	Description string    `json:"description"`  // 订单描述，用于日志
}

// ExecutionReport Binance WebSocket执行报告
type ExecutionReport struct {
	EventType            string `json:"e"`  // "executionReport"
	EventTime            int64  `json:"E"`  // 事件时间
	Symbol               string `json:"s"`  // 交易对
	ClientOrderID        string `json:"c"`  // 客户端订单ID
	Side                 string `json:"S"`  // 订单方向 BUY/SELL
	OrderType            string `json:"o"`  // 订单类型
	TimeInForce          string `json:"f"`  // 有效时间类型
	OrderQuantity        string `json:"q"`  // 订单数量
	OrderPrice           string `json:"p"`  // 订单价格
	StopPrice            string `json:"P"`  // 止损价格
	TrailingDelta        string `json:"d"`  // 跟踪止损回调
	IceBergQuantity      string `json:"F"`  // 冰山数量
	OrderListID          int64  `json:"g"`  // OCO订单ID
	OrigClientOrderID    string `json:"C"`  // 原始客户端订单ID
	ExecutionType        string `json:"x"`  // 本次执行的类型
	OrderStatus          string `json:"X"`  // 订单的当前状态
	OrderRejectReason    string `json:"r"`  // 订单被拒绝的原因
	OrderID              int64  `json:"i"`  // 订单ID
	LastExecutedQuantity string `json:"l"`  // 本次成交数量
	CumulativeQuantity   string `json:"z"`  // 累计成交数量
	LastExecutedPrice    string `json:"L"`  // 本次成交价格
	CommissionAmount     string `json:"n"`  // 手续费数量
	CommissionAsset      string `json:"N"`  // 手续费资产类别
	TransactionTime      int64  `json:"T"`  // 成交时间
	TradeID              int64  `json:"t"`  // 成交ID
	WorkingTime          int64  `json:"W"`  // 订单添加到order book的时间
	SelfTradePrevent     bool   `json:"m"`  // 该成交是否为做市商成交
	PositionSide         string `json:"ps"` // 持仓方向 LONG/SHORT
}

// StopLossExecutionEvent 止损成交事件
type StopLossExecutionEvent struct {
	TradeID          string    `json:"trade_id"`
	Symbol           string    `json:"symbol"`
	Side             string    `json:"side"`
	OrderID          int64     `json:"order_id"`
	ExecutionPrice   float64   `json:"execution_price"`
	ExecutionQty     float64   `json:"execution_qty"`
	PnL              float64   `json:"pnl"`
	PnLPct           float64   `json:"pnl_pct"`
	Timestamp        time.Time `json:"timestamp"`
	CalculationMethod string   `json:"calculation_method"`
}

// WebSocketOrderManager WebSocket订单管理器
type WebSocketOrderManager struct {
	// 基础配置
	apiKey      string
	secretKey   string
	isTestnet   bool
	trader      Trader
	database    *config.Database
	autoTrader  *AutoTrader // 回调引用
	
	// WebSocket连接管理
	conn        *websocket.Conn
	listenKey   string
	connMutex   sync.RWMutex
	isConnected bool
	
	// 订单跟踪
	trackedOrders map[int64]*TrackedOrder
	orderMutex    sync.RWMutex
	
	// 控制和状态
	stopChan      chan struct{}
	isRunning     bool
	runMutex      sync.RWMutex
	
	// 重连管理
	reconnectCount   int
	maxReconnects    int
	reconnectDelay   time.Duration
	heartbeatTicker  *time.Ticker
	
	// 降级模式
	fallbackMode     bool
	fallbackTicker   *time.Ticker
}

// NewWebSocketOrderManager 创建新的WebSocket订单管理器
func NewWebSocketOrderManager(apiKey, secretKey string, isTestnet bool, trader Trader, database *config.Database) *WebSocketOrderManager {
	return &WebSocketOrderManager{
		apiKey:           apiKey,
		secretKey:        secretKey,
		isTestnet:        isTestnet,
		trader:           trader,
		database:         database,
		trackedOrders:    make(map[int64]*TrackedOrder),
		stopChan:         make(chan struct{}),
		maxReconnects:    5,
		reconnectDelay:   5 * time.Second,
		isRunning:        false,
		fallbackMode:     false,
	}
}

// SetAutoTrader 设置AutoTrader回调引用
func (wom *WebSocketOrderManager) SetAutoTrader(at *AutoTrader) {
	wom.autoTrader = at
}

// Start 启动WebSocket订单管理器
func (wom *WebSocketOrderManager) Start() error {
	wom.runMutex.Lock()
	defer wom.runMutex.Unlock()
	
	if wom.isRunning {
		return fmt.Errorf("WebSocket订单管理器已经在运行")
	}
	
	log.Printf("🚀 [WebSocketOrderManager] 正在启动...")
	
	// 1. 获取listenKey
	if err := wom.createListenKey(); err != nil {
		log.Printf("❌ [WebSocketOrderManager] 获取listenKey失败: %v", err)
		return err
	}
	
	// 2. 建立WebSocket连接
	if err := wom.connect(); err != nil {
		log.Printf("❌ [WebSocketOrderManager] 建立WebSocket连接失败: %v", err)
		// 启动降级模式
		wom.startFallbackMode()
		return err
	}
	
	wom.isRunning = true
	
	// 3. 启动消息处理协程
	go wom.messageLoop()
	
	// 4. 启动心跳保活协程
	go wom.keepAliveLoop()
	
	log.Printf("✅ [WebSocketOrderManager] 启动成功")
	return nil
}

// Stop 停止WebSocket订单管理器
func (wom *WebSocketOrderManager) Stop() {
	wom.runMutex.Lock()
	defer wom.runMutex.Unlock()
	
	if !wom.isRunning {
		return
	}
	
	log.Printf("⏹️ [WebSocketOrderManager] 正在停止...")
	
	close(wom.stopChan)
	wom.isRunning = false
	
	// 停止心跳
	if wom.heartbeatTicker != nil {
		wom.heartbeatTicker.Stop()
	}
	
	// 停止降级模式
	if wom.fallbackTicker != nil {
		wom.fallbackTicker.Stop()
	}
	
	// 关闭WebSocket连接
	wom.connMutex.Lock()
	if wom.conn != nil {
		wom.conn.Close()
		wom.conn = nil
	}
	wom.isConnected = false
	wom.connMutex.Unlock()
	
	log.Printf("✅ [WebSocketOrderManager] 已停止")
}

// TrackOrder 添加订单到跟踪列表
func (wom *WebSocketOrderManager) TrackOrder(order *TrackedOrder) {
	wom.orderMutex.Lock()
	defer wom.orderMutex.Unlock()
	
	wom.trackedOrders[order.OrderID] = order
	
	log.Printf("📍 [WebSocketOrderManager] 开始跟踪订单: ID=%d %s %s %s (关联TradeID: %s)", 
		order.OrderID, order.Symbol, order.Side, order.Type, order.TradeID)
	log.Printf("    数量: %.6f, 描述: %s", order.Quantity, order.Description)
	
	// 如果在降级模式，立即启动该订单的轮询检查
	if wom.fallbackMode {
		go wom.pollOrderStatus(order)
	}
}

// UntrackOrder 从跟踪列表中移除订单
func (wom *WebSocketOrderManager) UntrackOrder(orderID int64) {
	wom.orderMutex.Lock()
	defer wom.orderMutex.Unlock()
	
	if order, exists := wom.trackedOrders[orderID]; exists {
		delete(wom.trackedOrders, orderID)
		log.Printf("🗑️ [WebSocketOrderManager] 停止跟踪订单: ID=%d %s %s", 
			orderID, order.Symbol, order.Side)
	}
}

// GetTrackedOrderCount 获取当前跟踪的订单数量
func (wom *WebSocketOrderManager) GetTrackedOrderCount() int {
	wom.orderMutex.RLock()
	defer wom.orderMutex.RUnlock()
	return len(wom.trackedOrders)
}

// IsConnected 检查WebSocket连接状态
func (wom *WebSocketOrderManager) IsConnected() bool {
	wom.connMutex.RLock()
	defer wom.connMutex.RUnlock()
	return wom.isConnected
}

// GetStatus 获取管理器状态信息
func (wom *WebSocketOrderManager) GetStatus() map[string]interface{} {
	wom.runMutex.RLock()
	isRunning := wom.isRunning
	wom.runMutex.RUnlock()
	
	wom.connMutex.RLock()
	isConnected := wom.isConnected
	wom.connMutex.RUnlock()
	
	return map[string]interface{}{
		"is_running":       isRunning,
		"is_connected":     isConnected,
		"tracked_orders":   wom.GetTrackedOrderCount(),
		"fallback_mode":    wom.fallbackMode,
		"reconnect_count":  wom.reconnectCount,
		"listen_key":       wom.listenKey,
	}
}

// createListenKey 创建用户数据流的listenKey
func (wom *WebSocketOrderManager) createListenKey() error {
	// 根据是否为测试网选择不同的端点
	var baseURL string
	if wom.isTestnet {
		baseURL = "https://testnet.binancefuture.com"
	} else {
		baseURL = "https://fapi.binance.com"
	}
	
	// 创建HTTP请求获取listenKey
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", baseURL+"/fapi/v1/listenKey", nil)
	if err != nil {
		return fmt.Errorf("创建listenKey请求失败: %w", err)
	}
	
	// 添加API Key
	req.Header.Set("X-MBX-APIKEY", wom.apiKey)
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送listenKey请求失败: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return fmt.Errorf("获取listenKey失败，状态码: %d", resp.StatusCode)
	}
	
	var result struct {
		ListenKey string `json:"listenKey"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析listenKey响应失败: %w", err)
	}
	
	wom.listenKey = result.ListenKey
	log.Printf("✅ [WebSocketOrderManager] 获取listenKey成功: %s", wom.listenKey[:8]+"...")
	
	return nil
}

// connect 建立WebSocket连接
func (wom *WebSocketOrderManager) connect() error {
	// 构建WebSocket URL
	var wsURL string
	if wom.isTestnet {
		wsURL = fmt.Sprintf("wss://stream.binancefuture.com/ws/%s", wom.listenKey)
	} else {
		wsURL = fmt.Sprintf("wss://fstream.binance.com/ws/%s", wom.listenKey)
	}
	
	log.Printf("🔗 [WebSocketOrderManager] 正在连接: %s", wsURL)
	
	// 建立WebSocket连接
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %w", err)
	}
	
	wom.connMutex.Lock()
	wom.conn = conn
	wom.isConnected = true
	wom.connMutex.Unlock()
	
	log.Printf("✅ [WebSocketOrderManager] WebSocket连接成功")
	return nil
}

// messageLoop 消息处理循环
func (wom *WebSocketOrderManager) messageLoop() {
	defer func() {
		log.Printf("📤 [WebSocketOrderManager] 消息处理循环退出")
	}()
	
	for {
		select {
		case <-wom.stopChan:
			return
		default:
			wom.connMutex.RLock()
			conn := wom.conn
			wom.connMutex.RUnlock()
			
			if conn == nil {
				time.Sleep(1 * time.Second)
				continue
			}
			
			// 设置读取超时
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			
			// 读取消息
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("❌ [WebSocketOrderManager] 读取消息失败: %v", err)
				wom.handleConnectionError()
				continue
			}
			
			if messageType == websocket.TextMessage {
				wom.handleMessage(message)
			}
		}
	}
}

// handleMessage 处理WebSocket消息
func (wom *WebSocketOrderManager) handleMessage(message []byte) {
	// 解析消息类型
	var baseMsg struct {
		EventType string `json:"e"`
	}
	
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		log.Printf("❌ [WebSocketOrderManager] 解析消息失败: %v", err)
		return
	}
	
	switch baseMsg.EventType {
	case "executionReport":
		wom.handleExecutionReport(message)
	case "outboundAccountPosition":
		wom.handleAccountUpdate(message)
	case "ACCOUNT_UPDATE":
		wom.handleAccountUpdate(message)
	default:
		// 其他消息类型暂时忽略
		log.Printf("📨 [WebSocketOrderManager] 收到其他类型消息: %s", baseMsg.EventType)
	}
}

// handleExecutionReport 处理执行报告 - 核心订单成交逻辑
func (wom *WebSocketOrderManager) handleExecutionReport(message []byte) {
	var report ExecutionReport
	if err := json.Unmarshal(message, &report); err != nil {
		log.Printf("❌ [WebSocketOrderManager] 解析执行报告失败: %v", err)
		return
	}
	
	log.Printf("📊 [WebSocketOrderManager] 收到执行报告: OrderID=%d %s %s 状态=%s 执行类型=%s", 
		report.OrderID, report.Symbol, report.OrderType, report.OrderStatus, report.ExecutionType)
	
	// 查找是否是我们跟踪的订单
	wom.orderMutex.RLock()
	trackedOrder, exists := wom.trackedOrders[report.OrderID]
	wom.orderMutex.RUnlock()
	
	if !exists {
		log.Printf("📋 [WebSocketOrderManager] 订单ID %d 不在跟踪列表中，忽略", report.OrderID)
		return
	}
	
	log.Printf("🎯 [WebSocketOrderManager] 匹配到跟踪订单: %s %s %s", 
		trackedOrder.Symbol, trackedOrder.Side, trackedOrder.Type)
	
	// 检查订单是否完全成交
	if report.OrderStatus != "FILLED" {
		log.Printf("📋 [WebSocketOrderManager] 订单ID %d 状态为 %s，未完全成交，继续跟踪", 
			report.OrderID, report.OrderStatus)
		return
	}
	
	// 根据订单类型处理成交事件
	switch trackedOrder.Type {
	case "STOP_MARKET", "STOP":
		wom.handleStopLossExecution(trackedOrder, &report)
	case "MARKET", "LIMIT":
		wom.handleMarketOrderExecution(trackedOrder, &report)
	default:
		log.Printf("⚠️ [WebSocketOrderManager] 未知订单类型: %s", trackedOrder.Type)
	}
	
	// 移除已处理的订单
	wom.UntrackOrder(report.OrderID)
}

// handleStopLossExecution 处理止损单成交
func (wom *WebSocketOrderManager) handleStopLossExecution(order *TrackedOrder, report *ExecutionReport) {
	log.Printf("🚨 [WebSocketOrderManager] 处理止损单成交: %s %s OrderID=%d", 
		order.Symbol, order.Side, order.OrderID)
	
	// 1. 解析真实成交价格和数量
	executionPrice, err := strconv.ParseFloat(report.LastExecutedPrice, 64)
	if err != nil {
		log.Printf("❌ [WebSocketOrderManager] 解析成交价格失败: %v", err)
		executionPrice = order.StopPrice // 降级使用止损价格
	}
	
	executionQty, err := strconv.ParseFloat(report.CumulativeQuantity, 64)
	if err != nil {
		log.Printf("❌ [WebSocketOrderManager] 解析成交数量失败: %v", err)
		executionQty = order.Quantity // 降级使用订单数量
	}
	
	log.Printf("💰 [WebSocketOrderManager] 止损成交详情: 价格=%.6f 数量=%.6f", executionPrice, executionQty)
	
	// 2. 获取对应的数据库交易记录用于计算盈亏
	var pnl, pnlPct float64
	var calculationMethod = "websocket_realtime"
	
	if wom.database != nil && order.TradeID != "" {
		// 尝试从数据库获取开仓信息进行精确计算
		if openTrade, err := wom.database.GetOpenTrade(wom.autoTrader.GetID(), order.Symbol, order.Side); err == nil {
			log.Printf("✅ [WebSocketOrderManager] 找到开仓记录: 开仓价=%.6f 保证金=%.2f", 
				openTrade.OpenPrice, openTrade.MarginUsed)
			
			// 计算精确盈亏
			if order.Side == "long" {
				pnl = (executionPrice - openTrade.OpenPrice) * executionQty
			} else {
				pnl = (openTrade.OpenPrice - executionPrice) * executionQty
			}
			
			if openTrade.MarginUsed > 0 {
				pnlPct = (pnl / openTrade.MarginUsed) * 100
			}
			
			calculationMethod = "websocket_precise_with_db"
			log.Printf("💎 [WebSocketOrderManager] 精确盈亏计算: %.2f USDT (%.2f%%)", pnl, pnlPct)
		} else {
			log.Printf("⚠️ [WebSocketOrderManager] 无法获取开仓记录: %v，使用估算方式", err)
			// 降级计算：假设止损就是亏损
			pnl = 0.0 // 无法精确计算
			calculationMethod = "websocket_fallback_no_db"
		}
	}
	
	// 3. 🆕 调用AutoTrader的实时止损处理方法（统一处理）
	if wom.autoTrader != nil {
		log.Printf("🔄 [WebSocketOrderManager] 调用AutoTrader实时止损处理...")
		
		// 调用新的统一处理方法
		wom.autoTrader.handleRealtimeStopLossExecution(order.Symbol, order.Side, 
			order.OrderID, executionPrice, executionQty)
		
		// 记录额外的WebSocket特定动作
		wom.recordStopLossAction(order, executionPrice, executionQty, pnl)
	}
	
	// 4. 创建止损成交事件
	event := StopLossExecutionEvent{
		TradeID:           order.TradeID,
		Symbol:            order.Symbol,
		Side:              order.Side,
		OrderID:           order.OrderID,
		ExecutionPrice:    executionPrice,
		ExecutionQty:      executionQty,
		PnL:               pnl,
		PnLPct:            pnlPct,
		Timestamp:         time.Unix(report.TransactionTime/1000, 0),
		CalculationMethod: calculationMethod,
	}
	
	// 5. 通知系统其他组件止损成交
	wom.publishStopLossEvent(&event)
	
	log.Printf("🎯 [WebSocketOrderManager] 止损单成交处理完成: %s %s 盈亏=%.2f USDT", 
		order.Symbol, order.Side, pnl)
}

// handleMarketOrderExecution 处理市价单成交
func (wom *WebSocketOrderManager) handleMarketOrderExecution(order *TrackedOrder, report *ExecutionReport) {
	log.Printf("📈 [WebSocketOrderManager] 处理市价单成交: %s %s OrderID=%d", 
		order.Symbol, order.Side, order.OrderID)
	
	// 解析成交价格
	executionPrice, err := strconv.ParseFloat(report.LastExecutedPrice, 64)
	if err != nil {
		log.Printf("❌ [WebSocketOrderManager] 解析市价单成交价格失败: %v", err)
		return
	}
	
	log.Printf("✅ [WebSocketOrderManager] 市价单成交: 价格=%.6f", executionPrice)
	
	// 对于开仓订单，我们主要关心真实成交价格的记录
	// 数据库记录的更新由AutoTrader的开仓逻辑负责
}

// handleAccountUpdate 处理账户更新
func (wom *WebSocketOrderManager) handleAccountUpdate(message []byte) {
	// 这里可以处理账户余额变化等信息
	// 暂时只记录日志，未来可以用于实时风控
	log.Printf("📊 [WebSocketOrderManager] 收到账户更新通知")
}

// recordStopLossAction 记录止损成交动作到数据库
func (wom *WebSocketOrderManager) recordStopLossAction(order *TrackedOrder, price, quantity, pnl float64) {
	if wom.database == nil {
		return
	}
	
	actionRecord := &config.TradeActionRecord{
		TraderID:     wom.autoTrader.GetID(),
		Action:       fmt.Sprintf("stop_loss_%s_websocket", order.Side),
		Symbol:       order.Symbol,
		Quantity:     quantity,
		Price:        price,
		OrderID:      fmt.Sprintf("%d", order.OrderID),
		Timestamp:    time.Now(),
		Success:      true,
		ErrorMessage: fmt.Sprintf("WebSocket实时止损成交 - 盈亏: %.2f USDT, TradeID: %s", pnl, order.TradeID),
	}
	
	if err := wom.database.CreateTradeAction(actionRecord); err != nil {
		log.Printf("❌ [WebSocketOrderManager] 记录止损动作失败: %v", err)
	} else {
		log.Printf("✅ [WebSocketOrderManager] ��损动作已记录: ID=%s", actionRecord.ID)
	}
}

// publishStopLossEvent 发布止损成交��件
func (wom *WebSocketOrderManager) publishStopLossEvent(event *StopLossExecutionEvent) {
	// 这里可以实现事件总线，通知系统其他组件
	// 目前先通过日志记录
	log.Printf("📢 [WebSocketOrderManager] 发布止损成交事件:")
	log.Printf("    币种: %s %s", event.Symbol, strings.ToUpper(event.Side))
	log.Printf("    订单ID: %d", event.OrderID)
	log.Printf("    成交价格: %.6f", event.ExecutionPrice)
	log.Printf("    成交数量: %.6f", event.ExecutionQty)
	log.Printf("    盈亏: %.2f USDT (%.2f%%)", event.PnL, event.PnLPct)
	log.Printf("    计算方法: %s", event.CalculationMethod)
	log.Printf("    成交时间: %s", event.Timestamp.Format("15:04:05"))
}

// handleConnectionError 处理连接错误（增强版）
func (wom *WebSocketOrderManager) handleConnectionError() {
	wom.connMutex.Lock()
	wom.isConnected = false
	if wom.conn != nil {
		wom.conn.Close()
		wom.conn = nil
	}
	wom.connMutex.Unlock()
	
	log.Printf("❌ [WebSocketOrderManager] WebSocket连接出错，启动增强重连机制")
	
	// 🆕 增强：立即通知AutoTrader连接状态变化
	if wom.autoTrader != nil {
		log.Printf("📢 [WebSocketOrderManager] 通知AutoTrader连接异常")
		// 可以在这里添加回调通知
	}
	
	// 启动降级模式（更快响应）
	wom.startFallbackMode()
	
	// 🆕 增强：记录连接错误统计
	wom.logConnectionError()
	
	// 尝试重连（带指数退避）
	go wom.reconnectWithBackoff()
}

// logConnectionError 记录连接错误统计
func (wom *WebSocketOrderManager) logConnectionError() {
	errorTime := time.Now().Format("15:04:05")
	log.Printf("📊 [WebSocketOrderManager] 连接错误统计:")
	log.Printf("    错误时间: %s", errorTime)
	log.Printf("    重连次数: %d/%d", wom.reconnectCount, wom.maxReconnects)
	log.Printf("    当前跟踪订单: %d个", wom.GetTrackedOrderCount())
	log.Printf("    降级模式: %v", wom.fallbackMode)
}

// reconnectWithBackoff 带退避机制的重连（增强版）
func (wom *WebSocketOrderManager) reconnectWithBackoff() {
	for wom.isRunning && wom.reconnectCount < wom.maxReconnects {
		wom.reconnectCount++
		
		log.Printf("🔄 [WebSocketOrderManager] 尝试重连 (%d/%d)...", wom.reconnectCount, wom.maxReconnects)
		
		// 🆕 增强：指数退避策略，最大等待时间限制
		backoffSeconds := min(wom.reconnectCount*5, 60) // 最多等待60秒
		backoffDuration := time.Duration(backoffSeconds) * time.Second
		log.Printf("⏰ [WebSocketOrderManager] 等待 %v 后重连", backoffDuration)
		time.Sleep(backoffDuration)
		
		// 🆕 增强：检查是否仍需要重连
		if !wom.isRunning {
			log.Printf("⏹️ [WebSocketOrderManager] 系统已停止，取消重连")
			return
		}
		
		// 重新获取listenKey
		log.Printf("🔑 [WebSocketOrderManager] 重新获取listenKey...")
		if err := wom.createListenKey(); err != nil {
			log.Printf("❌ [WebSocketOrderManager] 重连时获取listenKey失败 (#%d): %v", wom.reconnectCount, err)
			continue
		}
		log.Printf("✅ [WebSocketOrderManager] listenKey获取成功")
		
		// 尝试重新连接
		log.Printf("🔗 [WebSocketOrderManager] 尝试建立WebSocket连接...")
		if err := wom.connect(); err != nil {
			log.Printf("❌ [WebSocketOrderManager] 重连失败 (#%d): %v", wom.reconnectCount, err)
			continue
		}
		
		// 🆕 重连成功后的验证
		time.Sleep(1 * time.Second) // 等待连接稳定
		if !wom.IsConnected() {
			log.Printf("❌ [WebSocketOrderManager] 连接验证失败，继续重试")
			continue
		}
		
		// 重连成功
		log.Printf("✅ [WebSocketOrderManager] 重连成功！")
		wom.reconnectCount = 0 // 重置重连计数
		wom.stopFallbackMode() // 停止降级模式
		
		// 重新启动消息处理
		go wom.messageLoop()
		
		// 🆕 增强：记录重连成功统计
		log.Printf("📊 [WebSocketOrderManager] 重连成功统计:")
		log.Printf("    恢复时间: %s", time.Now().Format("15:04:05"))
		log.Printf("    当前跟踪订单: %d个", wom.GetTrackedOrderCount())
		log.Printf("    降级模式已关闭")
		
		return
	}
	
	// 重连失败，保持降级模式
	if wom.reconnectCount >= wom.maxReconnects {
		log.Printf("❌ [WebSocketOrderManager] 达到最大重连次数(%d)，保持降级模式运行", wom.maxReconnects)
		log.Printf("⚠️ [WebSocketOrderManager] 系统将继续使用轮询模式监控订单状态")
	} else {
		log.Printf("⏹️ [WebSocketOrderManager] 系统停止，终止重连")
	}
}

// min 辅助函数：返回两个整数中的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// startFallbackMode 启动降级模式（轮询检查）- 增强版
func (wom *WebSocketOrderManager) startFallbackMode() {
	if wom.fallbackMode {
		return // 已经在降级模式
	}
	
	wom.fallbackMode = true
	log.Printf("⚠️ [WebSocketOrderManager] 启动增强降级模式 - 使用智能轮询检查订单状态")
	
	// 🆕 增强：更频繁的轮询（连接断开时需要更快响应）
	pollingInterval := 5 * time.Second // 从10秒降低到5秒
	log.Printf("📊 [WebSocketOrderManager] 降级模式配置: 轮询间隔=%v, 当前跟踪订单=%d个", 
		pollingInterval, wom.GetTrackedOrderCount())
	
	// 启动降级轮询
	wom.fallbackTicker = time.NewTicker(pollingInterval)
	go wom.fallbackLoop()
}

// stopFallbackMode 停止降级模式
func (wom *WebSocketOrderManager) stopFallbackMode() {
	if !wom.fallbackMode {
		return
	}
	
	wom.fallbackMode = false
	if wom.fallbackTicker != nil {
		wom.fallbackTicker.Stop()
		wom.fallbackTicker = nil
	}
	
	log.Printf("✅ [WebSocketOrderManager] 降级模式已停止，恢复WebSocket实时监控")
}

// fallbackLoop 降级模式轮询循环
func (wom *WebSocketOrderManager) fallbackLoop() {
	defer func() {
		log.Printf("📤 [WebSocketOrderManager] 降级模式轮询退出")
	}()
	
	for {
		select {
		case <-wom.stopChan:
			return
		case <-wom.fallbackTicker.C:
			if !wom.fallbackMode {
				return // 降级模式已停止
			}
			wom.checkAllTrackedOrders()
		}
	}
}

// checkAllTrackedOrders 检查所有跟踪的订单状态（降级模式）- 增强版
func (wom *WebSocketOrderManager) checkAllTrackedOrders() {
	wom.orderMutex.RLock()
	orders := make([]*TrackedOrder, 0, len(wom.trackedOrders))
	for _, order := range wom.trackedOrders {
		orders = append(orders, order)
	}
	wom.orderMutex.RUnlock()
	
	if len(orders) == 0 {
		return
	}
	
	log.Printf("🔍 [WebSocketOrderManager] 降级模式检查 %d 个跟踪订单", len(orders))
	
	successCount := 0
	errorCount := 0
	
	for i, order := range orders {
		// 🆕 增强：添加订单检查进度日志
		if len(orders) > 5 {
			log.Printf("📋 [WebSocketOrderManager] 检查进度: (%d/%d) %s %s", 
				i+1, len(orders), order.Symbol, order.Type)
		}
		
		if wom.pollOrderStatus(order) {
			successCount++
		} else {
			errorCount++
		}
		
		// 🆕 增强：动态调整延迟（根据订单数量）
		if len(orders) > 10 {
			time.Sleep(50 * time.Millisecond) // 订单多时减少延迟
		} else {
			time.Sleep(100 * time.Millisecond) // 订单少时保持原延迟
		}
	}
	
	// 🆕 增强：检查结果统计
	if errorCount > 0 {
		log.Printf("⚠️ [WebSocketOrderManager] 降级模式检查结果: 成功=%d, 失败=%d", successCount, errorCount)
	}
}

// pollOrderStatus 轮询单个订单状态（降级模式）
func (wom *WebSocketOrderManager) pollOrderStatus(order *TrackedOrder) {
	if wom.trader == nil {
		return
	}
	
	orderStatus, err := wom.trader.GetOrderStatus(order.Symbol, order.OrderID)
	if err != nil {
		log.Printf("❌ [WebSocketOrderManager] 轮询订单状态失败: OrderID=%d, %v", order.OrderID, err)
		return
	}
	
	status, ok := orderStatus["status"].(string)
	if !ok {
		log.Printf("❌ [WebSocketOrderManager] 无法解析订单状态: OrderID=%d", order.OrderID)
		return
	}
	
	// 如果订单已成交，模拟执行报告
	if status == "FILLED" {
		log.Printf("📊 [WebSocketOrderManager] 降级模式检测到订单成交: OrderID=%d", order.OrderID)
		
		// 构建模拟执行报告
		avgPrice, _ := orderStatus["avgPrice"].(string)
		quantity, _ := orderStatus["executedQty"].(string)
		
		mockReport := &ExecutionReport{
			OrderID:              order.OrderID,
			Symbol:               order.Symbol,
			OrderStatus:          "FILLED",
			ExecutionType:        "TRADE",
			LastExecutedPrice:    avgPrice,
			CumulativeQuantity:   quantity,
			TransactionTime:      time.Now().UnixMilli(),
		}
		
		// 处理成交
		if order.Type == "STOP_MARKET" || order.Type == "STOP" {
			wom.handleStopLossExecution(order, mockReport)
		} else {
			wom.handleMarketOrderExecution(order, mockReport)
		}
		
		// 移除已处理的订单
		wom.UntrackOrder(order.OrderID)
	}
}

// keepAliveLoop listenKey保活循环
func (wom *WebSocketOrderManager) keepAliveLoop() {
	wom.heartbeatTicker = time.NewTicker(30 * time.Minute) // 每30分钟延长一次
	defer func() {
		log.Printf("📤 [WebSocketOrderManager] 心跳保活退出")
	}()
	
	for {
		select {
		case <-wom.stopChan:
			return
		case <-wom.heartbeatTicker.C:
			if wom.isConnected {
				wom.extendListenKey()
			}
		}
	}
}

// extendListenKey 延长listenKey的有效期
func (wom *WebSocketOrderManager) extendListenKey() {
	var baseURL string
	if wom.isTestnet {
		baseURL = "https://testnet.binancefuture.com"
	} else {
		baseURL = "https://fapi.binance.com"
	}
	
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("PUT", baseURL+"/fapi/v1/listenKey", nil)
	if err != nil {
		log.Printf("❌ [WebSocketOrderManager] 创建延长listenKey请求失败: %v", err)
		return
	}
	
	req.Header.Set("X-MBX-APIKEY", wom.apiKey)
	
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ [WebSocketOrderManager] 延长listenKey请求失败: %v", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 200 {
		log.Printf("✅ [WebSocketOrderManager] listenKey延长成功")
	} else {
		log.Printf("❌ [WebSocketOrderManager] 延长listenKey失败，状态码: %d", resp.StatusCode)
	}
}