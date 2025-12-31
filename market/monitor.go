package market

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type WSMonitor struct {
	wsClient        *WSClient
	combinedClient  *CombinedStreamsClient
	symbols         []string
	featuresMap     sync.Map
	alertsChan      chan Alert
	klineDataMap5m  sync.Map // 存储每个交易对的K线历史数据
	klineDataMap15m sync.Map // 存储每个交易对的K线历史数据
	klineDataMap30m sync.Map // 存储每个交易对的K线历史数据
	klineDataMap1h  sync.Map // 存储每个交易对的K线历史数据
	klineDataMap4h  sync.Map // 存储每个交易对的K线历史数据
	tickerDataMap   sync.Map // 存储每个交易对的ticker数据
	batchSize       int
	filterSymbols   sync.Map // 使用sync.Map来存储需要监控的币种和其状态
	symbolStats     sync.Map // 存储币种统计信息
	FilterSymbol    []string //经过筛选的币种
	btcTriggerCallback func() // BTCUSDT 5分钟收盘事件触发器回调（兼容版本）
	btcTriggerCallbackWithTime func(time.Time) // 🔧 新增：支持精确K线收盘时间的触发器回调
}
type SymbolStats struct {
	LastActiveTime   time.Time
	AlertCount       int
	VolumeSpikeCount int
	LastAlertTime    time.Time
	Score            float64 // 综合评分
}

var WSMonitorCli *WSMonitor
var subKlineTime = []string{"5m", "15m", "30m", "1h", "4h"} // 管理订阅流的K线周期

func NewWSMonitor(batchSize int) *WSMonitor {
	WSMonitorCli = &WSMonitor{
		wsClient:       NewWSClient(),
		combinedClient: NewCombinedStreamsClient(batchSize),
		alertsChan:     make(chan Alert, 1000),
		batchSize:      batchSize,
	}
	return WSMonitorCli
}

func (m *WSMonitor) Initialize(coins []string) error {
	log.Println("初始化WebSocket监控器...")
	// 获取交易对信息
	apiClient := NewAPIClient()
	// 如果不指定交易对，则使用market市场的所有交易对币种
	if len(coins) == 0 {
		exchangeInfo, err := apiClient.GetExchangeInfo()
		if err != nil {
			return err
		}
		// 筛选永续合约交易对 --仅测试时使用
		//exchangeInfo.Symbols = exchangeInfo.Symbols[0:2]
		for _, symbol := range exchangeInfo.Symbols {
			if symbol.Status == "TRADING" && symbol.ContractType == "PERPETUAL" && strings.ToUpper(symbol.Symbol[len(symbol.Symbol)-4:]) == "USDT" {
				m.symbols = append(m.symbols, symbol.Symbol)
				m.filterSymbols.Store(symbol.Symbol, true)
			}
		}
	} else {
		m.symbols = coins
	}

	log.Printf("找到 %d 个交易对", len(m.symbols))
	// 初始化历史数据
	if err := m.initializeHistoricalData(); err != nil {
		log.Printf("初始化历史数据失败: %v", err)
	}

	return nil
}

func (m *WSMonitor) initializeHistoricalData() error {
	apiClient := NewAPIClient()

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // 限制并发数

	for _, symbol := range m.symbols {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(s string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// 获取所有时间框架的历史K线数据
			timeframes := map[string]*sync.Map{
				"5m":  &m.klineDataMap5m,
				"15m": &m.klineDataMap15m,
				"30m": &m.klineDataMap30m,
				"1h":  &m.klineDataMap1h,
				"4h":  &m.klineDataMap4h,
			}

			for tf, dataMap := range timeframes {
				klines, err := apiClient.GetKlines(s, tf, 1000)
				if err != nil {
					log.Printf("获取 %s %s历史数据失败: %v", s, tf, err)
					continue
				}
				if len(klines) > 0 {
					dataMap.Store(s, klines)
					log.Printf("已加载 %s 的历史K线数据-%s: %d 条", s, tf, len(klines))
				}
			}
		}(symbol)
	}

	wg.Wait()
	return nil
}

func (m *WSMonitor) Start(coins []string) {
	log.Printf("启动WebSocket实时监控...")
	// 初始化交易对
	err := m.Initialize(coins)
	if err != nil {
		log.Fatalf("❌ 初始化币种: %v", err)
		return
	}

	err = m.combinedClient.Connect()
	if err != nil {
		log.Fatalf("❌ 批量订阅流: %v", err)
		return
	}
	// 订阅所有交易对
	err = m.subscribeAll()
	if err != nil {
		log.Fatalf("❌ 订阅币种交易对: %v", err)
		return
	}
}

// subscribeSymbol 注册监听
func (m *WSMonitor) subscribeSymbol(symbol, st string) []string {
	var streams []string
	stream := fmt.Sprintf("%s@kline_%s", strings.ToLower(symbol), st)
	ch := m.combinedClient.AddSubscriber(stream, 100)
	streams = append(streams, stream)
	go m.handleKlineData(symbol, ch, st)

	return streams
}
func (m *WSMonitor) subscribeAll() error {
	// 执行批量订阅
	log.Println("开始订阅所有交易对...")
	for _, symbol := range m.symbols {
		for _, st := range subKlineTime {
			m.subscribeSymbol(symbol, st)
		}
	}
	for _, st := range subKlineTime {
		err := m.combinedClient.BatchSubscribeKlines(m.symbols, st)
		if err != nil {
			log.Fatalf("❌ 订阅5m K线: %v", err)
			return err
		}
	}
	log.Println("所有交易对订阅完成")
	return nil
}

func (m *WSMonitor) handleKlineData(symbol string, ch <-chan []byte, _time string) {
	for data := range ch {
		var klineData KlineWSData
		if err := json.Unmarshal(data, &klineData); err != nil {
			log.Printf("解析Kline数据失败: %v", err)
			continue
		}
		m.processKlineUpdate(symbol, klineData, _time)
	}
}

func (m *WSMonitor) getKlineDataMap(_time string) *sync.Map {
	switch _time {
	case "5m":
		return &m.klineDataMap5m
	case "15m":
		return &m.klineDataMap15m
	case "30m":
		return &m.klineDataMap30m
	case "1h":
		return &m.klineDataMap1h
	case "4h":
		return &m.klineDataMap4h
	default:
		// 返回一个空的sync.Map，避免panic
		return &sync.Map{}
	}
}
func (m *WSMonitor) processKlineUpdate(symbol string, wsData KlineWSData, _time string) {
	// 转换WebSocket数据为Kline结构
	kline := Kline{
		OpenTime:  wsData.Kline.StartTime,
		CloseTime: wsData.Kline.CloseTime,
		Trades:    wsData.Kline.NumberOfTrades,
	}
	kline.Open, _ = parseFloat(wsData.Kline.OpenPrice)
	kline.High, _ = parseFloat(wsData.Kline.HighPrice)
	kline.Low, _ = parseFloat(wsData.Kline.LowPrice)
	kline.Close, _ = parseFloat(wsData.Kline.ClosePrice)
	kline.Volume, _ = parseFloat(wsData.Kline.Volume)
	kline.High, _ = parseFloat(wsData.Kline.HighPrice)
	kline.QuoteVolume, _ = parseFloat(wsData.Kline.QuoteVolume)
	kline.TakerBuyBaseVolume, _ = parseFloat(wsData.Kline.TakerBuyBaseVolume)
	kline.TakerBuyQuoteVolume, _ = parseFloat(wsData.Kline.TakerBuyQuoteVolume)

	// 🔒 更新PriceCache（用于锁盈系统）
	if priceCache := GetGlobalPriceCache(); priceCache != nil {
		tickSize := getSmartTickSizeBySymbol(symbol)
		priceCache.Update(symbol, kline.Close, kline.Close, tickSize)
	}

	// 🎯 BTCUSDT 5分钟收盘事件检测
	if symbol == "BTCUSDT" && _time == "5m" && wsData.Kline.IsFinal {
		closeTime := time.UnixMilli(wsData.Kline.CloseTime)
		log.Printf("🕐 BTCUSDT 5分钟K线收盘事件: %v", closeTime.Format("2006-01-02 15:04:05"))
		
		// 🔧 优先使用精确时序触发器，支持K线收盘时间传递
		if m.btcTriggerCallbackWithTime != nil {
			go func() {
				start := time.Now()
				log.Printf("⏱️ 开始精确时序AI分析触发: %v (K线收盘: %v)", 
					start.Format("15:04:05.000"), closeTime.Format("15:04:05"))
				m.btcTriggerCallbackWithTime(closeTime)
				duration := time.Since(start)
				log.Printf("📊 精确时序AI分析完成耗时: %v", duration)
			}()
		} else if m.btcTriggerCallback != nil {
			// 兼容原有触发器
			go func() {
				start := time.Now()
				log.Printf("⏱️ 开始AI分析触发: %v", start.Format("15:04:05.000"))
				m.btcTriggerCallback()
				duration := time.Since(start)
				log.Printf("📊 AI分析完成耗时: %v (从收盘事件触发到完成)", duration)
			}()
		}
	}

	// 更新K线数据
	var klineDataMap = m.getKlineDataMap(_time)
	value, exists := klineDataMap.Load(symbol)
	var klines []Kline
	if exists {
		klines = value.([]Kline)

		// 检查是否是新的K线
		if len(klines) > 0 && klines[len(klines)-1].OpenTime == kline.OpenTime {
			// 更新当前K线
			klines[len(klines)-1] = kline
		} else {
			// 添加新K线
			klines = append(klines, kline)

			// 保持数据长度为1000条（确保技术指标计算准确性）
			const REQUIRED_KLINES = 1000
			if len(klines) > REQUIRED_KLINES {
				// 保持最新的1000条数据，删除最老的数据
				klines = klines[len(klines)-REQUIRED_KLINES:]
			}
		}
	} else {
		klines = []Kline{kline}
	}

	klineDataMap.Store(symbol, klines)
}

func (m *WSMonitor) GetCurrentKlines(symbol string, _time string) ([]Kline, error) {
	// 对每一个进来的symbol检测是否存在内类 是否的话就订阅它
	value, exists := m.getKlineDataMap(_time).Load(symbol)
	if !exists {
		log.Printf("📊 [K线获取] %s %s时间框架缓存未命中，使用API获取", symbol, _time)
		// 如果Ws数据未初始化完成时,单独使用api获取 - 兼容性代码 (防止在未初始化完成是,已经有交易员运行)
		apiClient := NewAPIClient()
		klines, err := apiClient.GetKlines(symbol, _time, 1000)
		if err != nil {
			log.Printf("❌ [K线获取] API获取%s %s失败: %v", symbol, _time, err)
			return nil, fmt.Errorf("获取%v分钟K线失败: %v", _time, err)
		}
		log.Printf("✓ [K线获取] API获取%s %s成功: %d条数据", symbol, _time, len(klines))
		
		m.getKlineDataMap(_time).Store(strings.ToUpper(symbol), klines) //动态缓存进缓存
		subStr := m.subscribeSymbol(symbol, _time)
		subErr := m.combinedClient.subscribeStreams(subStr)
		log.Printf("📡 动态订阅流: %v", subStr)
		if subErr != nil {
			log.Printf("⚠️ [K线获取] 动态订阅失败: %v", subErr)
			// 不返回错误，因为已经有API数据了
		}
		return klines, nil
	}
	klines := value.([]Kline)
	log.Printf("✓ [K线获取] %s %s缓存命中: %d条数据", symbol, _time, len(klines))
	
	// 🔍 临时检查：验证实际K线数量是否为1000条
	if len(klines) != 1000 {
		log.Printf("⚠️ [K线数量检查] %s %s 实际数量: %d条 (期望1000条)", symbol, _time, len(klines))
	}
	return klines, nil
}

// SetBTCTrigger 设置BTCUSDT 5分钟收盘事件触发器回调
func (m *WSMonitor) SetBTCTrigger(callback func()) {
	m.btcTriggerCallback = callback
}

// SetBTCTriggerWithTime 设置BTCUSDT 5分钟收盘事件触发器回调（🔧 新增：支持精确时序）
func (m *WSMonitor) SetBTCTriggerWithTime(callback func(time.Time)) {
	m.btcTriggerCallbackWithTime = callback
}

func (m *WSMonitor) Close() {
	m.wsClient.Close()
	close(m.alertsChan)
}
