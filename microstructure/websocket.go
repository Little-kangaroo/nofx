package microstructure

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ===== WebSocket消息结构 =====

// BinanceAggTradeMsg 币安aggTrade消息
type BinanceAggTradeMsg struct {
	EventType    string `json:"e"`  // 事件类型
	EventTime    int64  `json:"E"`  // 事件时间
	Symbol       string `json:"s"`  // 交易对
	AggTradeID   int64  `json:"a"`  // 聚合交易ID
	Price        string `json:"p"`  // 价格
	Quantity     string `json:"q"`  // 数量
	FirstTradeID int64  `json:"f"`  // 第一个交易ID
	LastTradeID  int64  `json:"l"`  // 最后一个交易ID
	TradeTime    int64  `json:"T"`  // 交易时间
	IsBuyerMaker bool   `json:"m"`  // 是否买方挂单
}

// BinanceDepthMsg 币安depth消息
type BinanceDepthMsg struct {
	EventType   string     `json:"e"`    // 事件类型
	EventTime   int64      `json:"E"`    // 事件时间
	Symbol      string     `json:"s"`    // 交易对
	FirstUpdate int64      `json:"U"`    // 第一个更新ID
	FinalUpdate int64      `json:"u"`    // 最终更新ID
	Bids        [][]string `json:"b"`    // 买单 [价格, 数量]
	Asks        [][]string `json:"a"`    // 卖单 [价格, 数量]
}

// ===== WebSocket连接管理器 =====

// WSConnection WebSocket连接
type WSConnection struct {
	conn           *websocket.Conn
	url            string
	symbol         string
	streamType     string // "aggTrade" or "depth20"
	marketType     string // "spot" or "futures"
	mu             sync.RWMutex
	isConnected    bool
	lastPing       time.Time
	reconnectCount int
	maxReconnects  int
}

// WSManager WebSocket连接管理器
type WSManager struct {
	connections    map[string]*WSConnection // key: symbol_streamType_marketType
	config         *MicrostructureConfig
	mu             sync.RWMutex
	onTradeMessage func(*TradeData)
	onDepthMessage func(*DepthData)
	stopChan       chan bool
	isRunning      bool
}

// TradeData 交易数据
type TradeData struct {
	Symbol       string    `json:"symbol"`
	Price        float64   `json:"price"`
	Quantity     float64   `json:"quantity"`
	IsBuyerMaker bool      `json:"is_buyer_maker"`
	Timestamp    time.Time `json:"timestamp"`
	MarketType   string    `json:"market_type"` // "spot" or "futures"
}

// DepthData 盘口数据
type DepthData struct {
	Symbol     string           `json:"symbol"`
	Bids       []OrderBookLevel `json:"bids"`
	Asks       []OrderBookLevel `json:"asks"`
	Timestamp  time.Time        `json:"timestamp"`
	MarketType string           `json:"market_type"`
}

// NewWSManager 创建WebSocket管理器
func NewWSManager(config *MicrostructureConfig) *WSManager {
	return &WSManager{
		connections:   make(map[string]*WSConnection),
		config:        config,
		stopChan:      make(chan bool),
	}
}

// SetTradeHandler 设置交易数据处理器
func (wm *WSManager) SetTradeHandler(handler func(*TradeData)) {
	wm.onTradeMessage = handler
}

// SetDepthHandler 设置盘口数据处理器
func (wm *WSManager) SetDepthHandler(handler func(*DepthData)) {
	wm.onDepthMessage = handler
}

// Subscribe 订阅币种的数据流
func (wm *WSManager) Subscribe(symbol string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	symbol = strings.ToUpper(symbol)
	if !strings.HasSuffix(symbol, "USDT") {
		symbol += "USDT"
	}

	// 订阅现货aggTrade
	if err := wm.subscribeStream(symbol, "aggTrade", "spot"); err != nil {
		log.Printf("❌ 订阅现货aggTrade失败 %s: %v", symbol, err)
	}

	// 订阅合约aggTrade
	if err := wm.subscribeStream(symbol, "aggTrade", "futures"); err != nil {
		log.Printf("❌ 订阅合约aggTrade失败 %s: %v", symbol, err)
	}

	// 订阅合约depth20（盘口分析主要用合约数据）
	if err := wm.subscribeStream(symbol, "depth20", "futures"); err != nil {
		log.Printf("❌ 订阅合约depth20失败 %s: %v", symbol, err)
	}

	log.Printf("✅ 成功订阅 %s 的微观数据流", symbol)
	return nil
}

// subscribeStream 订阅单个数据流
func (wm *WSManager) subscribeStream(symbol, streamType, marketType string) error {
	key := fmt.Sprintf("%s_%s_%s", symbol, streamType, marketType)

	// 检查是否已存在连接
	if conn, exists := wm.connections[key]; exists && conn.isConnected {
		return nil // 已连接，跳过
	}

	// 构建WebSocket URL
	var wsURL string
	var stream string
	
	switch marketType {
	case "spot":
		wsURL = wm.config.SpotWsURL
	case "futures":
		wsURL = wm.config.FuturesWsURL
	default:
		return fmt.Errorf("不支持的市场类型: %s", marketType)
	}

	switch streamType {
	case "aggTrade":
		stream = fmt.Sprintf("%s@aggTrade", strings.ToLower(symbol))
	case "depth20":
		stream = fmt.Sprintf("%s@depth20@100ms", strings.ToLower(symbol))
	default:
		return fmt.Errorf("不支持的数据流类型: %s", streamType)
	}

	fullURL := wsURL + stream
	
	// 🔍 添加URL调试信息
	log.Printf("🌐 准备连接WebSocket: %s", fullURL)

	// 创建连接
	conn := &WSConnection{
		url:           fullURL,
		symbol:        symbol,
		streamType:    streamType,
		marketType:    marketType,
		maxReconnects: 10,
	}

	wm.connections[key] = conn

	// 启动连接
	go wm.handleConnection(conn)

	log.Printf("📡 启动WebSocket连接: %s", key)
	return nil
}

// handleConnection 处理单个WebSocket连接
func (wm *WSManager) handleConnection(wsConn *WSConnection) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ WebSocket连接异常恢复: %v", r)
		}
	}()

	for {
		if err := wm.connectAndListen(wsConn); err != nil {
			log.Printf("❌ WebSocket连接错误 %s: %v", wsConn.symbol, err)
			
			wsConn.reconnectCount++
			if wsConn.reconnectCount >= wsConn.maxReconnects {
				log.Printf("❌ 达到最大重连次数，停止重连: %s", wsConn.symbol)
				break
			}

			// 等待重连
			time.Sleep(wm.config.ReconnectInterval)
			log.Printf("🔄 尝试重连 %s (%d/%d)", wsConn.symbol, wsConn.reconnectCount, wsConn.maxReconnects)
		}

		// 检查是否需要停止
		select {
		case <-wm.stopChan:
			log.Printf("📴 停止WebSocket连接: %s", wsConn.symbol)
			return
		default:
		}
	}
}

// connectAndListen 连接并监听WebSocket
func (wm *WSManager) connectAndListen(wsConn *WSConnection) error {
	// 解析URL
	u, err := url.Parse(wsConn.url)
	if err != nil {
		return fmt.Errorf("URL解析失败: %w", err)
	}

	// 建立连接
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %w", err)
	}
	defer conn.Close()

	wsConn.mu.Lock()
	wsConn.conn = conn
	wsConn.isConnected = true
	wsConn.lastPing = time.Now()
	wsConn.mu.Unlock()

	log.Printf("✅ WebSocket已连接: %s_%s_%s", wsConn.symbol, wsConn.streamType, wsConn.marketType)

	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(65 * time.Second))
	
	// 设置pong处理器
	conn.SetPongHandler(func(appData string) error {
		wsConn.mu.Lock()
		wsConn.lastPing = time.Now()
		wsConn.mu.Unlock()
		conn.SetReadDeadline(time.Now().Add(65 * time.Second))
		return nil
	})

	// 启动ping routine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				wsConn.mu.Lock()
				if wsConn.isConnected && wsConn.conn != nil {
					if err := wsConn.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						log.Printf("❌ Ping失败: %v", err)
						wsConn.isConnected = false
					}
				}
				wsConn.mu.Unlock()
			case <-wm.stopChan:
				return
			}
		}
	}()

	// 监听消息
	log.Printf("🎯 开始监听消息: %s_%s_%s", wsConn.symbol, wsConn.streamType, wsConn.marketType)
	
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			wsConn.mu.Lock()
			wsConn.isConnected = false
			wsConn.mu.Unlock()
			log.Printf("❌ WebSocket读取失败 %s_%s_%s: %v", wsConn.symbol, wsConn.streamType, wsConn.marketType, err)
			return fmt.Errorf("读取消息失败: %w", err)
		}

		// 处理消息
		if err := wm.processMessage(wsConn, message); err != nil {
			log.Printf("⚠️ 消息处理失败 %s_%s_%s: %v", wsConn.symbol, wsConn.streamType, wsConn.marketType, err)
		}
	}
}

// processMessage 处理WebSocket消息
func (wm *WSManager) processMessage(wsConn *WSConnection, message []byte) error {
	switch wsConn.streamType {
	case "aggTrade":
		return wm.processAggTradeMessage(wsConn, message)
	case "depth20":
		return wm.processDepthMessage(wsConn, message)
	default:
		return fmt.Errorf("未知消息类型: %s", wsConn.streamType)
	}
}

// processAggTradeMessage 处理aggTrade消息
func (wm *WSManager) processAggTradeMessage(wsConn *WSConnection, message []byte) error {
	var msg BinanceAggTradeMsg
	if err := json.Unmarshal(message, &msg); err != nil {
		return fmt.Errorf("解析aggTrade消息失败: %w", err)
	}

	// 转换为内部格式
	price, err := strconv.ParseFloat(msg.Price, 64)
	if err != nil {
		return fmt.Errorf("价格转换失败: %w", err)
	}

	quantity, err := strconv.ParseFloat(msg.Quantity, 64)
	if err != nil {
		return fmt.Errorf("数量转换失败: %w", err)
	}

	tradeData := &TradeData{
		Symbol:       msg.Symbol,
		Price:        price,
		Quantity:     quantity,
		IsBuyerMaker: msg.IsBuyerMaker,
		Timestamp:    time.Unix(0, msg.TradeTime*int64(time.Millisecond)),
		MarketType:   wsConn.marketType,
	}

	// 调用处理器
	if wm.onTradeMessage != nil {
		wm.onTradeMessage(tradeData)
	} else {
		log.Printf("⚠️ 交易处理器为空，数据被丢弃！")
	}

	return nil
}

// processDepthMessage 处理depth消息
func (wm *WSManager) processDepthMessage(wsConn *WSConnection, message []byte) error {
	var msg BinanceDepthMsg
	if err := json.Unmarshal(message, &msg); err != nil {
		return fmt.Errorf("解析depth消息失败: %w", err)
	}

	// 转换买单
	bids := make([]OrderBookLevel, 0, len(msg.Bids))
	for _, bid := range msg.Bids {
		if len(bid) >= 2 {
			price, _ := strconv.ParseFloat(bid[0], 64)
			quantity, _ := strconv.ParseFloat(bid[1], 64)
			if price > 0 && quantity > 0 {
				bids = append(bids, OrderBookLevel{
					Price:    price,
					Quantity: quantity,
				})
			}
		}
	}

	// 转换卖单
	asks := make([]OrderBookLevel, 0, len(msg.Asks))
	for _, ask := range msg.Asks {
		if len(ask) >= 2 {
			price, _ := strconv.ParseFloat(ask[0], 64)
			quantity, _ := strconv.ParseFloat(ask[1], 64)
			if price > 0 && quantity > 0 {
				asks = append(asks, OrderBookLevel{
					Price:    price,
					Quantity: quantity,
				})
			}
		}
	}

	depthData := &DepthData{
		Symbol:     msg.Symbol,
		Bids:       bids,
		Asks:       asks,
		Timestamp:  time.Unix(0, msg.EventTime*int64(time.Millisecond)),
		MarketType: wsConn.marketType,
	}

	// 调用处理器
	if wm.onDepthMessage != nil {
		wm.onDepthMessage(depthData)
	}

	return nil
}

// Start 启动WebSocket管理器
func (wm *WSManager) Start() {
	wm.mu.Lock()
	wm.isRunning = true
	wm.mu.Unlock()
	log.Printf("🚀 WebSocket管理器启动")
}

// Stop 停止WebSocket管理器
func (wm *WSManager) Stop() {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if !wm.isRunning {
		return
	}

	wm.isRunning = false
	close(wm.stopChan)

	// 关闭所有连接
	for key, conn := range wm.connections {
		conn.mu.Lock()
		if conn.conn != nil {
			conn.conn.Close()
		}
		conn.isConnected = false
		conn.mu.Unlock()
		log.Printf("📴 已关闭连接: %s", key)
	}

	log.Printf("⛔ WebSocket管理器已停止")
}

// GetConnectionStatus 获取连接状态
func (wm *WSManager) GetConnectionStatus() map[string]bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	status := make(map[string]bool)
	for key, conn := range wm.connections {
		conn.mu.RLock()
		status[key] = conn.isConnected
		conn.mu.RUnlock()
	}

	return status
}