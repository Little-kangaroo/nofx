package trader

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"nofx/config"
	"nofx/decision"
	"nofx/internal/protect"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
	"nofx/microstructure"
	"nofx/pool"
	"strconv"
	"strings"
	"time"
)

// PendingStopOrder 待确认的止损单
type PendingStopOrder struct {
	Symbol      string    `json:"symbol"`       // 币种
	Side        string    `json:"side"`         // 方向 long/short 
	OrderID     int64     `json:"order_id"`     // 止损单ID
	StopPrice   float64   `json:"stop_price"`   // 止损价格
	Quantity    float64   `json:"quantity"`     // 数量
	CreateTime  time.Time `json:"create_time"`  // 创建时间
	OriginalAction string `json:"original_action"` // 原始动作 (update_stop)
	
	// 🎯 新增：补偿闭合重试机制
	ReconcileAttempts int       `json:"-"` // 补偿尝试次数
	LastReconcileAt   time.Time `json:"-"` // 上次补偿时间
}

// AutoTraderConfig 自动交易配置（简化版 - AI全权决策）
type AutoTraderConfig struct {
	// Trader标识
	ID      string // Trader唯一标识（用于日志目录等）
	Name    string // Trader显示名称
	AIModel string // AI模型: "qwen" 或 "deepseek"

	// 交易平台选择
	Exchange string // "binance", "hyperliquid" 或 "aster"

	// 币安API配置
	BinanceAPIKey    string
	BinanceSecretKey string

	// Hyperliquid配置
	HyperliquidPrivateKey string
	HyperliquidWalletAddr string
	HyperliquidTestnet    bool

	// Aster配置
	AsterUser       string // Aster主钱包地址
	AsterSigner     string // Aster API钱包地址
	AsterPrivateKey string // Aster API钱包私钥

	CoinPoolAPIURL string

	// AI配置
	UseQwen    bool
	DeepSeekKey string
	QwenKey     string
	ClaudeKey   string

	// 自定义AI API配置
	CustomAPIURL    string
	CustomAPIKey    string
	CustomModelName string

	// 扫描配置
	ScanInterval time.Duration // 扫描间隔（建议5分钟）

	// 账户配置
	InitialBalance float64 // 初始金额（用于计算盈亏，需手动设置）

	// 杠杆配置
	BTCETHLeverage  int // BTC和ETH的杠杆倍数
	AltcoinLeverage int // 山寨币的杠杆倍数

	// 风险控制（仅作为提示，AI可自主决定）
	MaxDailyLoss    float64       // 最大日亏损百分比（提示）
	MaxDrawdown     float64       // 最大回撤百分比（提示）
	StopTradingTime time.Duration // 触发风控后暂停时长

	// 仓位模式
	IsCrossMargin bool // true=全仓模式, false=逐仓模式

	// 币种配置
	DefaultCoins []string // 默认币种列表（从数据库获取）
	TradingCoins []string // 实际交易币种列表

	// 系统提示词模板
	SystemPromptTemplate string // 系统提示词模板名称（如 "default", "aggressive"）
}

// AutoTrader 自动交易器
type AutoTrader struct {
	id                    string // Trader唯一标识
	name                  string // Trader显示名称
	aiModel               string // AI模型名称
	exchange              string // 交易平台名称
	config                AutoTraderConfig
	trader                Trader // 使用Trader接口（支持多平台）
	mcpClient             *mcp.Client
	decisionLogger        *logger.DecisionLogger // 决策日志记录器
	database              *config.Database       // 数据库连接
	initialBalance        float64
	dailyPnL              float64
	customPrompt          string   // 自定义交易策略prompt
	overrideBasePrompt    bool     // 是否覆盖基础prompt
	systemPromptTemplate  string   // 系统提示词模板名称
	defaultCoins          []string // 默认币种列表（从数据库获取）
	tradingCoins          []string // 实际交易币种列表
	lastResetTime         time.Time
	stopUntil             time.Time
	isRunning             bool
	startTime             time.Time        // 系统启动时间
	callCount             int              // AI调用次数
	positionFirstSeenTime map[string]int64 // 持仓首次出现时间 (symbol_side -> timestamp毫秒)
	pendingStopOrders     map[string]*PendingStopOrder // 待确认的止损单
	lastKnownStopOrders   map[string][]map[string]interface{} // 上次检查的止损单状态 (posKey -> orders)
	exchangeSync          *ExchangeRecordSync // 交易所记录同步器
	wsOrderManager        *WebSocketOrderManager // WebSocket订单管理器

	// 🔒 锁盈系统
	profitScheduler *protect.Scheduler      // 锁盈调度器
	protectCtx      context.Context          // 锁盈系统context
	protectCancel   context.CancelFunc       // 锁盈系统cancel函数
}

// NewAutoTrader 创建自动交易器
func NewAutoTrader(config AutoTraderConfig, database *config.Database) (*AutoTrader, error) {
	// 设置默认值
	if config.ID == "" {
		config.ID = "default_trader"
	}
	if config.Name == "" {
		config.Name = "Default Trader"
	}
	if config.AIModel == "" {
		if config.UseQwen {
			config.AIModel = "qwen"
		} else {
			config.AIModel = "deepseek"
		}
	}

	mcpClient := mcp.New()

	// 初始化AI
	if config.AIModel == "custom" {
		// 使用自定义API
		mcpClient.SetCustomAPI(config.CustomAPIURL, config.CustomAPIKey, config.CustomModelName)
		log.Printf("🤖 [%s] 使用自定义AI API: %s (模型: %s)", config.Name, config.CustomAPIURL, config.CustomModelName)
	} else if config.AIModel == "claude" {
		// 使用Anthropic Claude
		mcpClient.SetClaudeAPI(config.ClaudeKey, config.CustomModelName)
		if config.CustomModelName != "" {
			log.Printf("🤖 [%s] 使用Anthropic Claude (模型: %s)", config.Name, config.CustomModelName)
		} else {
			log.Printf("🤖 [%s] 使用Anthropic Claude", config.Name)
		}
	} else if config.UseQwen || config.AIModel == "qwen" {
		// 使用Qwen (支持自定义URL和Model)
		mcpClient.SetQwenAPIKey(config.QwenKey, config.CustomAPIURL, config.CustomModelName)
		if config.CustomAPIURL != "" || config.CustomModelName != "" {
			log.Printf("🤖 [%s] 使用阿里云Qwen AI (自定义URL: %s, 模型: %s)", config.Name, config.CustomAPIURL, config.CustomModelName)
		} else {
			log.Printf("🤖 [%s] 使用阿里云Qwen AI", config.Name)
		}
	} else {
		// 默认使用DeepSeek (支持自定义URL和Model)
		mcpClient.SetDeepSeekAPIKey(config.DeepSeekKey, config.CustomAPIURL, config.CustomModelName)
		if config.CustomAPIURL != "" || config.CustomModelName != "" {
			log.Printf("🤖 [%s] 使用DeepSeek AI (自定义URL: %s, 模型: %s)", config.Name, config.CustomAPIURL, config.CustomModelName)
		} else {
			log.Printf("🤖 [%s] 使用DeepSeek AI", config.Name)
		}
	}

	// 初始化币种池API
	if config.CoinPoolAPIURL != "" {
		pool.SetCoinPoolAPI(config.CoinPoolAPIURL)
	}

	// 设置默认交易平台
	if config.Exchange == "" {
		config.Exchange = "binance"
	}

	// 根据配置创建对应的交易器
	var trader Trader
	var err error

	// 记录仓位模式（通用）
	marginModeStr := "全仓"
	if !config.IsCrossMargin {
		marginModeStr = "逐仓"
	}
	log.Printf("📊 [%s] 仓位模式: %s", config.Name, marginModeStr)

	switch config.Exchange {
	case "binance":
		log.Printf("🏦 [%s] 使用币安合约交易", config.Name)
		trader = NewFuturesTrader(config.BinanceAPIKey, config.BinanceSecretKey)
	case "hyperliquid":
		log.Printf("🏦 [%s] 使用Hyperliquid交易", config.Name)
		trader, err = NewHyperliquidTrader(config.HyperliquidPrivateKey, config.HyperliquidWalletAddr, config.HyperliquidTestnet)
		if err != nil {
			return nil, fmt.Errorf("初始化Hyperliquid交易器失败: %w", err)
		}
	case "aster":
		log.Printf("🏦 [%s] 使用Aster交易", config.Name)
		trader, err = NewAsterTrader(config.AsterUser, config.AsterSigner, config.AsterPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("初始化Aster交易器失败: %w", err)
		}
	default:
		return nil, fmt.Errorf("不支持的交易平台: %s", config.Exchange)
	}

	// 验证初始金额配置
	if config.InitialBalance <= 0 {
		return nil, fmt.Errorf("初始金额必须大于0，请在配置中设置InitialBalance")
	}

	// 初始化决策日志记录器（使用trader ID创建独立目录）
	logDir := fmt.Sprintf("decision_logs/%s", config.ID)
	decisionLogger := logger.NewDecisionLogger(logDir)

	// 设置默认系统提示词模板
	systemPromptTemplate := config.SystemPromptTemplate
	if systemPromptTemplate == "" {
		systemPromptTemplate = "default" // 默认使用 default 模板
	}

	// 调试信息：检查传入的币种配置
	log.Printf("🔧 [%s] 创建AutoTrader - DefaultCoins: %v (长度:%d)", config.Name, config.DefaultCoins, len(config.DefaultCoins))
	log.Printf("🔧 [%s] 创建AutoTrader - TradingCoins: %v (长度:%d)", config.Name, config.TradingCoins, len(config.TradingCoins))

	// 创建AutoTrader实例
	autoTrader := &AutoTrader{
		id:                    config.ID,
		name:                  config.Name,
		aiModel:               config.AIModel,
		exchange:              config.Exchange,
		config:                config,
		trader:                trader,
		mcpClient:             mcpClient,
		decisionLogger:        decisionLogger,
		database:              database,
		initialBalance:        config.InitialBalance,
		systemPromptTemplate:  systemPromptTemplate,
		defaultCoins:          config.DefaultCoins,
		tradingCoins:          config.TradingCoins,
		lastResetTime:         time.Now(),
		startTime:             time.Now(),
		callCount:             0,
		isRunning:             false,
		positionFirstSeenTime: make(map[string]int64),
		pendingStopOrders:     make(map[string]*PendingStopOrder),
		lastKnownStopOrders:   make(map[string][]map[string]interface{}),
	}

	// 初始化ExchangeRecordSync
	autoTrader.exchangeSync = NewExchangeRecordSync(trader, database, config.ID)
	log.Printf("✅ [%s] ExchangeRecordSync已初始化", config.Name)

	// 🆕 初始化WebSocket订单管理器
	if config.Exchange == "binance" && config.BinanceAPIKey != "" && config.BinanceSecretKey != "" {
		wsOrderManager := NewWebSocketOrderManager(config.BinanceAPIKey, config.BinanceSecretKey, false, trader, database)
		wsOrderManager.SetAutoTrader(autoTrader)
		autoTrader.wsOrderManager = wsOrderManager
		log.Printf("✅ [%s] WebSocket订单管理器已初始化", config.Name)
	} else {
		log.Printf("⚠️ [%s] 跳过WebSocket订单管理器初始化 (仅支持币安)", config.Name)
	}

	// 🔒 初始化锁盈系统
	log.Printf("🔒 [%s] 初始化锁盈系统...", config.Name)

	// V-20.0 简化配置：纯ROI锁盈（无时间约束）
	protectCfg := protect.DefaultConfig()

	// 自定义盈利地板：锁住3% ROI（10x杠杆下0.3%价格 = 3% ROI）
	protectCfg.FloorPriceBps = 30

	// 可选：禁用ROI止盈（不挂止盈单）
	// protectCfg.TPMilestones = nil

	protectEng := &protect.Engine{
		Cfg: protectCfg,
		Fees: protect.FeeModel{
			TakerFeeBps:      4,  // Binance Futures Taker费率
			SlippageBpsMinor: 2,
			FundingBps:       0,  // 可选
		},
	}

	priceCache := market.GetGlobalPriceCache()
	if priceCache == nil {
		log.Printf("⚠️ [%s] 全局PriceCache未初始化，锁盈系统将在首次使用时重试", config.Name)
	}

	scheduler := protect.NewScheduler(
		protectEng,
		newPositionStoreAdapter(database, config.ID),
		priceCache,
		newStopExecutorAdapter(trader),
	)

	autoTrader.profitScheduler = scheduler
	autoTrader.protectCtx, autoTrader.protectCancel = context.WithCancel(context.Background())
	log.Printf("✅ [%s] 锁盈系统已初始化", config.Name)

	return autoTrader, nil
}

// Run 运行自动交易主循环
func (at *AutoTrader) Run() error {
	at.isRunning = true
	log.Println("🚀 AI驱动自动交易系统启动")
	log.Printf("💰 初始余额: %.2f USDT", at.initialBalance)
	log.Println("🕐 使用BTCUSDT 5分钟收盘事件触发AI分析")
	log.Println("🤖 AI将全权决定杠杆、仓位大小、止损止盈等参数")

	// 🔧 修复：仅订阅币种到已启动的全局OrderFlowManager，不尝试启动新实例
	log.Printf("🔗 [%s] 订阅币种到全局订单流分析...", at.name)
	
	var symbols []string
	if len(at.tradingCoins) > 0 {
		symbols = at.tradingCoins
		log.Printf("📊 [%s] 使用自定义币种: %v", at.name, symbols)
	} else if len(at.defaultCoins) > 0 {
		symbols = at.defaultCoins
		log.Printf("📊 [%s] 使用默认币种: %v", at.name, symbols)
	} else {
		// 兜底方案
		symbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
		log.Printf("⚠️ [%s] 未配置币种，使用兜底方案: %v", at.name, symbols)
	}
	
	// 🔧 修复：仅获取已启动的全局实例，如果未启动则等待
	globalOFM := microstructure.GetGlobalOrderFlowManager()
	if globalOFM != nil {
		status := globalOFM.GetStatus()
		if isRunning, ok := status["is_running"].(bool); ok && isRunning {
			// 订阅币种到已启动的实例
			for _, symbol := range symbols {
				if err := globalOFM.SubscribeSymbol(symbol); err != nil {
					log.Printf("⚠️ [%s] 订阅币种失败 %s: %v", at.name, symbol, err)
				}
			}
			log.Printf("✅ [%s] 已订阅 %d 个币种到全局订单流分析: %v", at.name, len(symbols), symbols)
		} else {
			log.Printf("⚠️ [%s] 全局OrderFlowManager未启动，跳过订阅", at.name)
		}
	} else {
		log.Printf("❌ [%s] 全局OrderFlowManager不存在", at.name)
	}

	// 🆕 启动WebSocket订单管理器
	if at.wsOrderManager != nil {
		if err := at.wsOrderManager.Start(); err != nil {
			log.Printf("❌ [%s] WebSocket订单管理器启动失败: %v", at.name, err)
			log.Printf("⚠️ [%s] 将使用传统轮询模式继续运行", at.name)
		} else {
			log.Printf("✅ [%s] WebSocket订单管理器启动成功", at.name)
		}
	}

	// 🔒 启动锁盈系统Fast Loop（V-21.6: 带自动重启）
	if at.profitScheduler != nil {
		at.profitScheduler.StartFastLoopWithAutoRestart(at.protectCtx)
		log.Printf("✅ [%s] 锁盈系统Fast Loop已启动（10秒循环，自动重启）", at.name)
	}

	// 创建结束信号通道
	done := make(chan struct{})
	
	// 监听停止信号
	go func() {
		for at.isRunning {
			time.Sleep(1 * time.Second)
		}
		close(done)
	}()

	// 等待结束信号
	<-done
	return nil
}

// Stop 停止自动交易
func (at *AutoTrader) Stop() {
	at.isRunning = false

	// 🆕 停止WebSocket订单管理器
	if at.wsOrderManager != nil {
		at.wsOrderManager.Stop()
		log.Printf("✅ [%s] WebSocket订单管理器已停止", at.name)
	}

	// 🔒 停止锁盈系统Fast Loop（V-21.6: 优雅停止）
	if at.profitScheduler != nil {
		at.profitScheduler.StopFastLoop()
		log.Printf("✅ [%s] 锁盈系统停止信号已发送", at.name)
	}

	// 取消锁盈系统context
	if at.protectCancel != nil {
		at.protectCancel()
		log.Printf("✅ [%s] 锁盈系统context已取消", at.name)
	}

	log.Println("⏹ 自动交易系统停止")
}

// TriggerCycle 被BTCUSDT事件触发的AI分析周期
func (at *AutoTrader) TriggerCycle() {
	// 🔥 P0-01修复：兼容旧版本，使用当前时间作为锚点
	at.TriggerCycleWithTimeAnchor(time.Now())
}

// TriggerCycleWithTimeAnchor 被BTCUSDT事件触发的AI分析周期（支持时间锚点）
// 🔥 P0-01修复：接收精确的K线收盘时间锚点，确保数据时间一致性
func (at *AutoTrader) TriggerCycleWithTimeAnchor(anchorTime time.Time) {
	if !at.isRunning {
		return // 如果trader已停止，忽略触发
	}
	
	triggerStart := time.Now()
	log.Printf("🚀 [%s] BTCUSDT精确时序触发AI分析开始: %v (锚点: %v)", 
		at.id, triggerStart.Format("15:04:05.000"), anchorTime.Format("15:04:05.000"))
	
	if err := at.runCycleWithTimeAnchor(anchorTime); err != nil {
		log.Printf("❌ [%s] BTCUSDT精确时序触发的AI分析执行失败: %v", at.id, err)
	} else {
		duration := time.Since(triggerStart)
		log.Printf("✅ [%s] BTCUSDT精确时序触发AI分析完成: 耗时 %v", at.id, duration)
	}
}

// runCycle 运行一个交易周期（使用AI全权决策）
func (at *AutoTrader) runCycle() error {
	// 🔥 P0-01修复：兼容旧版本，使用当前时间作为锚点
	return at.runCycleWithTimeAnchor(time.Now())
}

// runCycleWithTimeAnchor 运行一个交易周期（支持时间锚点，使用AI全权决策）
// 🔥 P0-01修复：接收精确的K线收盘时间锚点，确保数据时间一致性
func (at *AutoTrader) runCycleWithTimeAnchor(anchorTime time.Time) error {
	at.callCount++

	log.Printf("%s", "\n" + strings.Repeat("=", 70))
	log.Printf("⏰ %s - AI决策周期 #%d (锚点: %v)", 
		time.Now().Format("2006-01-02 15:04:05"), at.callCount, anchorTime.Format("15:04:05.000"))
	log.Printf("%s", strings.Repeat("=", 70))

	// 创建决策记录
	record := &logger.DecisionRecord{
		ExecutionLog: []string{},
		Success:      true,
	}

	// 🔥 新增：0. 先对账，确保数据库与交易所一致（在所有逻辑之前执行）
	if err := at.ReconcileWithExchange(); err != nil {
		log.Printf("⚠️ [对账] 对账失败（不影响后续流程）: %v", err)
	}

	// 1. 检查是否需要停止交易
	if time.Now().Before(at.stopUntil) {
		remaining := at.stopUntil.Sub(time.Now())
		log.Printf("⏸ 风险控制：暂停交易中，剩余 %.0f 分钟", remaining.Minutes())
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("风险控制暂停中，剩余 %.0f 分钟", remaining.Minutes())
		at.decisionLogger.LogDecision(record)
		return nil
	}

	// 2. 重置日盈亏（每天重置）
	if time.Since(at.lastResetTime) > 24*time.Hour {
		at.dailyPnL = 0
		at.lastResetTime = time.Now()
		log.Println("📅 日盈亏已重置")
	}

	// 3. 检查止损单成交状态
	if err := at.checkPendingStopOrders(record); err != nil {
		log.Printf("⚠️ 检查止损单状态失败: %v", err)
	}

	// 4. 收集交易上下文
	ctx, err := at.buildTradingContext()
	if err != nil {
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("构建交易上下文失败: %v", err)
		at.decisionLogger.LogDecision(record)
		return fmt.Errorf("构建交易上下文失败: %w", err)
	}

	// 保存账户状态快照
	record.AccountState = logger.AccountSnapshot{
		TotalBalance:          ctx.Account.TotalEquity,
		AvailableBalance:      ctx.Account.AvailableBalance,
		TotalUnrealizedProfit: ctx.Account.TotalPnL,
		PositionCount:         ctx.Account.PositionCount,
		MarginUsedPct:         ctx.Account.MarginUsedPct,
	}

	// 保存持仓快照
	for _, pos := range ctx.Positions {
		record.Positions = append(record.Positions, logger.PositionSnapshot{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			PositionAmt:      pos.Quantity,
			EntryPrice:       pos.EntryPrice,
			MarkPrice:        pos.MarkPrice,
			UnrealizedProfit: pos.UnrealizedPnL,
			Leverage:         float64(pos.Leverage),
			LiquidationPrice: pos.LiquidationPrice,
		})
	}

	// 保存候选币种列表
	for _, coin := range ctx.CandidateCoins {
		record.CandidateCoins = append(record.CandidateCoins, coin.Symbol)
	}

	log.Printf("📊 账户净值: %.2f USDT | 可用: %.2f USDT | 持仓: %d",
		ctx.Account.TotalEquity, ctx.Account.AvailableBalance, ctx.Account.PositionCount)

	// 4. 调用AI获取完整决策
	log.Printf("🤖 正在请求AI分析并决策... [模板: %s]", at.systemPromptTemplate)
	aiStart := time.Now()
	log.Printf("⏰ AI请求开始时间: %v", aiStart.Format("15:04:05.000"))
	
	decision, err := decision.GetFullDecisionWithCustomPromptAndAnchor(ctx, at.mcpClient, at.customPrompt, at.overrideBasePrompt, at.systemPromptTemplate, anchorTime)
	
	aiDuration := time.Since(aiStart)
	log.Printf("🎯 AI请求完成耗时: %v", aiDuration)
	
	// 即使有错误，也保存思维链、决策和输入prompt（用于debug）
	if decision != nil {
		record.SystemPrompt = decision.SystemPrompt // 保存系统提示词
		record.InputPrompt = decision.UserPrompt
		record.CoTTrace = decision.CoTTrace
		if len(decision.Decisions) > 0 {
			decisionJSON, _ := json.MarshalIndent(decision.Decisions, "", "  ")
			record.DecisionJSON = string(decisionJSON)
		}
	}

	if err != nil {
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("获取AI决策失败: %v", err)

		// 打印系统提示词和AI思维链（即使有错误，也要输出以便调试）
		if decision != nil {
			if decision.SystemPrompt != "" {
				log.Printf("\n%s", strings.Repeat("=", 70))
				log.Printf("📋 系统提示词 [模板: %s] (错误情况)", at.systemPromptTemplate)
				log.Println(strings.Repeat("=", 70))
				log.Println(decision.SystemPrompt)
				log.Printf("%s\n", strings.Repeat("=", 70))
			}

			if decision.CoTTrace != "" {
				log.Printf("\n%s", strings.Repeat("-", 70))
				log.Println("💭 AI思维链分析（错误情况）:")
				log.Println(strings.Repeat("-", 70))
				log.Println(decision.CoTTrace)
				log.Printf("%s\n", strings.Repeat("-", 70))
			}
		}

		at.decisionLogger.LogDecision(record)
		return fmt.Errorf("获取AI决策失败: %w", err)
	}

	// // 5. 打印系统提示词
	// log.Printf("\n" + strings.Repeat("=", 70))
	// log.Printf("📋 系统提示词 [模板: %s]", at.systemPromptTemplate)
	// log.Println(strings.Repeat("=", 70))
	// log.Println(decision.SystemPrompt)
	// log.Printf(strings.Repeat("=", 70) + "\n")

	// 6. 打印AI思维链
	log.Printf("\n%s", strings.Repeat("-", 70))
	log.Println("💭 AI思维链分析:")
	log.Println(strings.Repeat("-", 70))
	log.Println(decision.CoTTrace)
	log.Printf("%s\n", strings.Repeat("-", 70))

	// 7. 打印AI决策
	log.Printf("📋 AI决策列表 (%d 个):\n", len(decision.Decisions))
	for i, d := range decision.Decisions {
		log.Printf("  [%d] %s: %s - %s", i+1, d.Symbol, d.Action, d.Reasoning)
		if d.Action == "open_long" || d.Action == "open_short" {
			log.Printf("      杠杆: %dx | 仓位: %.2f USDT | 止损: %.4f | 止盈: %.4f",
				d.Leverage, d.PositionSizeUSD, d.StopLoss, d.TakeProfit)
		}
	}
	log.Println()

	// 8. 对决策排序：确保先平仓后开仓（防止仓位叠加超限）
	sortedDecisions := sortDecisionsByPriority(decision.Decisions)

	log.Println("🔄 执行顺序（已优化）: 先平仓→后开仓")
	for i, d := range sortedDecisions {
		log.Printf("  [%d] %s %s", i+1, d.Symbol, d.Action)
	}
	log.Println()

	// 执行决策并记录结果
	for _, d := range sortedDecisions {
		actionRecord := logger.DecisionAction{
			Action:    d.Action,
			Symbol:    d.Symbol,
			Quantity:  0,
			Leverage:  d.Leverage,
			Price:     0,
			Timestamp: time.Now(),
			Success:   false,
		}

		if err := at.executeDecisionWithRecord(&d, &actionRecord); err != nil {
			log.Printf("❌ 执行决策失败 (%s %s): %v", d.Symbol, d.Action, err)
			actionRecord.Error = err.Error()
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("❌ %s %s 失败: %v", d.Symbol, d.Action, err))
		} else {
			actionRecord.Success = true
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ %s %s 成功", d.Symbol, d.Action))
			// 成功执行后短暂延迟
			time.Sleep(1 * time.Second)
		}

		record.Decisions = append(record.Decisions, actionRecord)
	}

	// 9. 保存决策记录
	if err := at.decisionLogger.LogDecision(record); err != nil {
		log.Printf("⚠ 保存决策记录失败: %v", err)
	}

	// 10. 同时保存到数据库
	if at.database != nil {
		if err := at.saveToDatabaseRecord(record); err != nil {
			log.Printf("⚠ 保存决策记录到数据库失败: %v", err)
		}
	}

	// 11. 执行交易所记录同步检查（每5个周期执行一次以减少频率）
	if at.exchangeSync != nil && at.callCount%5 == 0 {
		log.Printf("🔄 [ExchangeSync] 执行定期持仓一致性检查 (周期 #%d)", at.callCount)
		if differences, err := at.exchangeSync.DetectPositionDifferences(); err != nil {
			log.Printf("❌ [ExchangeSync] 持仓差异检测失败: %v", err)
		} else if len(differences) > 0 {
			log.Printf("⚠️ [ExchangeSync] 发现 %d 个持仓差异，开始自动修复", len(differences))
			if err := at.exchangeSync.AutoFixPositionDifferences(differences); err != nil {
				log.Printf("❌ [ExchangeSync] 自动修复失败: %v", err)
			} else {
				log.Printf("✅ [ExchangeSync] 持仓差异修复完成")
			}
		} else {
			log.Printf("✅ [ExchangeSync] 持仓数据一致，无需修复")
		}
	}

	return nil
}

// buildTradingContext 构建交易上下文
func (at *AutoTrader) buildTradingContext() (*decision.Context, error) {
	// 1. 获取账户信息
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("获取账户余额失败: %w", err)
	}

	// 获取账户字段
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Total Equity = 钱包余额 + 未实现盈亏
	totalEquity := totalWalletBalance + totalUnrealizedProfit

	// 2. 获取持仓信息
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	var positionInfos []decision.PositionInfo
	totalMarginUsed := 0.0

	// 当前持仓的key集合（用于清理已平仓的记录）
	currentPositionKeys := make(map[string]bool)

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity // 空仓数量为负，转为正数
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		// 计算盈亏百分比
		pnlPct := 0.0
		if side == "long" {
			pnlPct = ((markPrice - entryPrice) / entryPrice) * 100
		} else {
			pnlPct = ((entryPrice - markPrice) / entryPrice) * 100
		}

		// 计算占用保证金（估算）
		leverage := 10 // 默认值，实际应该从持仓信息获取
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed

		// 跟踪持仓首次出现时间
		posKey := symbol + "_" + side
		currentPositionKeys[posKey] = true
		if _, exists := at.positionFirstSeenTime[posKey]; !exists {
			// 新持仓，记录当前时间
			at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()
		}
		updateTime := at.positionFirstSeenTime[posKey]

		// 获取当前的止损挂单价格（从交易所实时查询）
		var prevStopPrice float64 = 0.0
		
		log.Printf("🔍 [调试] 开始查询 %s %s 的止损挂单...", symbol, side)
		
		// 查询该币种的所有挂单
		orders, err := at.trader.GetOpenOrders(symbol)
		if err != nil {
			log.Printf("❌ [调试] 查询 %s 挂单失败: %v", symbol, err)
		} else {
			log.Printf("📋 [调试] %s 查询到 %d 个挂单", symbol, len(orders))
			
			// 查找对应持仓方向的止损单
			for i, order := range orders {
				orderType, _ := order["type"].(string)
				positionSide, _ := order["positionSide"].(string)
				stopPriceStr, _ := order["stopPrice"].(string)
				side_, _ := order["side"].(string)
				
				log.Printf("  📄 [调试] 挂单#%d: type=%s, side=%s, positionSide=%s, stopPrice=%s", 
					i+1, orderType, side_, positionSide, stopPriceStr)
				
				// 检查是否是止损单
				if orderType == "STOP_MARKET" || orderType == "STOP" {
					log.Printf("    🎯 [调试] 这是止损单，检查方向匹配...")
					// 检查持仓方向是否匹配
					if (side == "long" && positionSide == "LONG") || 
					   (side == "short" && positionSide == "SHORT") {
						log.Printf("    ✅ [调试] 方向匹配，解析止损价格...")
						// 解析止损价格
						if stopPrice, parseErr := strconv.ParseFloat(stopPriceStr, 64); parseErr == nil && stopPrice > 0 {
							prevStopPrice = stopPrice
							log.Printf("🎯 [调试] %s %s 找到止损单: %.6f", symbol, side, prevStopPrice)
							break // 找到第一个匹配的止损单即可
						} else {
							log.Printf("    ❌ [调试] 解析止损价格失败: %v, 原始值: '%s'", parseErr, stopPriceStr)
						}
					} else {
						log.Printf("    ⚠️ [调试] 方向不匹配: 持仓%s vs 挂单%s", side, positionSide)
					}
				}
			}
			
			if prevStopPrice == 0 {
				log.Printf("🔍 [调试] %s %s 未找到活跃的止损挂单", symbol, side)
			}
		}

		positionInfos = append(positionInfos, decision.PositionInfo{
			Symbol:           symbol,
			Side:             side,
			EntryPrice:       entryPrice,
			MarkPrice:        markPrice,
			Quantity:         quantity,
			Leverage:         leverage,
			UnrealizedPnL:    unrealizedPnl,
			UnrealizedPnLPct: pnlPct,
			LiquidationPrice: liquidationPrice,
			MarginUsed:       marginUsed,
			UpdateTime:       updateTime,
			PrevStop:         prevStopPrice, // 添加当前止损挂单价格
		})
	}

	// 清理已平仓的持仓记录
	for key := range at.positionFirstSeenTime {
		if !currentPositionKeys[key] {
			delete(at.positionFirstSeenTime, key)
		}
	}

	// 3. 获取交易员的候选币种池
	candidateCoins, err := at.getCandidateCoins()
	if err != nil {
		return nil, fmt.Errorf("获取候选币种失败: %w", err)
	}

	// 4. 计算总盈亏
	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	// 5. 分析历史表现（最近100笔交易）
	var performance interface{}
	if at.database != nil {
		perf, err := at.database.GetTradePerformanceAnalysis(at.id, 100)
		if err != nil {
			log.Printf("⚠️  分析历史表现失败: %v", err)
			performance = nil
		} else {
			performance = perf
		}
	} else {
		// 兼容旧系统
		perf, err := at.decisionLogger.AnalyzePerformance(100)
		if err != nil {
			log.Printf("⚠️  分析历史表现失败: %v", err)
			performance = nil
		} else {
			performance = perf
		}
	}

	// 6. 构建上下文
	ctx := &decision.Context{
		CurrentTime:     time.Now().Format("2006-01-02 15:04:05"),
		RuntimeMinutes:  int(time.Since(at.startTime).Minutes()),
		CallCount:       at.callCount,
		BTCETHLeverage:  at.config.BTCETHLeverage,  // 使用配置的杠杆倍数
		AltcoinLeverage: at.config.AltcoinLeverage, // 使用配置的杠杆倍数
		Account: decision.AccountInfo{
			TotalEquity:      totalEquity,
			AvailableBalance: availableBalance,
			TotalPnL:         totalPnL,
			TotalPnLPct:      totalPnLPct,
			MarginUsed:       totalMarginUsed,
			MarginUsedPct:    marginUsedPct,
			PositionCount:    len(positionInfos),
		},
		Positions:      positionInfos,
		CandidateCoins: candidateCoins,
		Performance:    performance, // 添加历史表现分析
	}

	return ctx, nil
}

// executeDecisionWithRecord 执行AI决策并记录详细信息
func (at *AutoTrader) executeDecisionWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	switch decision.Action {
	case "open_long":
		return at.executeOpenLongWithRecord(decision, actionRecord)
	case "open_short":
		return at.executeOpenShortWithRecord(decision, actionRecord)
	case "close_long":
		return at.executeCloseLongWithRecord(decision, actionRecord)
	case "close_short":
		return at.executeCloseShortWithRecord(decision, actionRecord)
	case "reduce":
		return at.executeReduceWithRecord(decision, actionRecord)
	case "reduce_long":
		return at.executeReduceLongWithRecord(decision, actionRecord)
	case "reduce_short":
		return at.executeReduceShortWithRecord(decision, actionRecord)
	case "update_stop", "update_stop_loss":
		return at.executeUpdateStopWithRecord(decision, actionRecord)
	case "update_take_profit":
		return at.executeUpdateTakeProfitWithRecord(decision, actionRecord)
	case "partial_close":
		return at.executePartialCloseWithRecord(decision, actionRecord)
	case "open":
		// 通用开仓需要判断方向，这里暂时记录但不执行
		log.Printf("  ℹ 收到通用开仓指令，需要指定方向(open_long/open_short)")
		return nil
	case "close":
		// 通用平仓，需要判断当前持仓方向
		return at.executeCloseAllPositionsWithRecord(decision, actionRecord)
	case "buy_to_enter":
		// 转换为open_long
		decision.Action = "open_long"
		return at.executeOpenLongWithRecord(decision, actionRecord)
	case "sell_to_enter":
		// 转换为open_short
		decision.Action = "open_short"
		return at.executeOpenShortWithRecord(decision, actionRecord)
	case "buy":
		// 简单买入指令，转换为open_long
		decision.Action = "open_long"
		return at.executeOpenLongWithRecord(decision, actionRecord)
	case "sell":
		// 简单卖出指令，转换为open_short
		decision.Action = "open_short"
		return at.executeOpenShortWithRecord(decision, actionRecord)
	case "hold", "wait":
		// 无需执行，仅记录
		return nil
	default:
		return fmt.Errorf("未知的action: %s", decision.Action)
	}
}

// executeOpenLongWithRecord 执行开多仓并记录详细信息
func (at *AutoTrader) executeOpenLongWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  📈 开多仓: %s", decision.Symbol)

	// 🔍 智能仓位管理：检查是否可以加仓（最多3阶梯）
	positions, err := at.trader.GetPositions()
	if err == nil {
		longPositionCount := 0
		for _, pos := range positions {
			if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
				longPositionCount++
			}
		}
		
		// 检查是否超过最大阶梯数（3阶）
		if longPositionCount >= 3 {
			log.Printf("  ⚠️ %s 已达最大持仓阶梯数（3阶），跳过加仓。如需调整仓位，请先减仓", decision.Symbol)
			return nil
		} else if longPositionCount > 0 {
			log.Printf("  📊 %s 当前持有%d阶多仓，准备执行第%d阶加仓", decision.Symbol, longPositionCount, longPositionCount+1)
		}
	}

	// 获取当前价格
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}

	// 计算数量
	quantity := decision.PositionSizeUSD / marketData.CurrentPrice
	
	// 🔧 Binance期货最小名义价值检查：必须≥100 USDT
	notionalValue := quantity * marketData.CurrentPrice
	if notionalValue < 100.0 {
		// 调整到最小名义价值
		quantity = 100.0 / marketData.CurrentPrice
		adjustedNotional := quantity * marketData.CurrentPrice
		log.Printf("  ⚠️ 调整仓位大小: %.2f USDT → %.2f USDT (满足100 USDT最小要求)", 
			decision.PositionSizeUSD, adjustedNotional)
		decision.PositionSizeUSD = adjustedNotional // 更新决策中的仓位大小
	}
	actionRecord.Quantity = quantity
	// 暂时使用市场价格，执行后会更新为实际成交价
	actionRecord.Price = marketData.CurrentPrice

	// 保证金验证已在模板中优化处理，此处跳过验证直接执行

	// 设置仓位模式
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		log.Printf("  ⚠️ 设置仓位模式失败: %v", err)
		// 继续执行，不影响交易
	}

	// 开仓
	order, err := at.trader.OpenLong(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	log.Printf("  ✓ 开仓成功，订单ID: %v, 数量: %.4f", order["orderId"], quantity)

	// 🔧 关键修复：从交易所获取权威的开仓成交数据
	var actualPrice float64
	var priceDataSource string

	// 获取订单ID
	orderID, ok := order["orderId"].(int64)
	if !ok {
		log.Printf("  ⚠️ 无法获取订单ID，使用市场价格作为fallback")
		actualPrice = marketData.CurrentPrice
		priceDataSource = "MARKET_PRICE_FALLBACK"
	} else {
		// 等待订单确认（给交易所一些处理时间）
		time.Sleep(2 * time.Second)

		// 确保trader是FuturesTrader类型
		binanceTrader, isBinance := at.trader.(*FuturesTrader)
		if !isBinance {
			log.Printf("  ⚠️ 交易器不是Binance，使用市场价格")
			actualPrice = marketData.CurrentPrice
			priceDataSource = "MARKET_PRICE_NON_BINANCE"
		} else {
			// 从交易所获取权威开仓数据
			authData, err := binanceTrader.GetAuthoritativeOpenData(decision.Symbol, orderID)
			if err != nil {
				log.Printf("  ⚠️ 无法获取权威开仓数据: %v, 使用市场价格", err)
				actualPrice = marketData.CurrentPrice
				priceDataSource = "MARKET_PRICE_AUTH_FAILED"
			} else {
				actualPrice = authData.ActualPrice
				priceDataSource = authData.DataSource
				log.Printf("  ✅ [权威数据] 真实开仓价格: %.6f", actualPrice)
				log.Printf("     数据来源: %s", authData.DataSource)
				log.Printf("     成交数量: %.6f", authData.ActualQuantity)
				log.Printf("     手续费: %.4f %s", authData.Commission, authData.CommissionAsset)
				log.Printf("     是否挂单成交: %v", authData.IsMaker)
			}
		}
	}

	actionRecord.Price = actualPrice

	// 记录到数据库
	log.Printf("🔍 [调试] 准备记录开仓到数据库:")
	log.Printf("    trader_id: '%s'", at.id)
	log.Printf("    symbol: '%s'", decision.Symbol)
	log.Printf("    side: 'long'")
	log.Printf("    quantity: %.6f", quantity)
	log.Printf("    leverage: %d", decision.Leverage)
	log.Printf("    actualPrice: %.6f (来源: %s)", actualPrice, priceDataSource)


	at.recordTradeToDatabase(decision.Symbol, "long", quantity, decision.Leverage,
		actualPrice, fmt.Sprintf("%v", order["orderId"]), "open_long", true, decision.StopLoss)

	// 🔥 新增：异步验证开仓数据（从交易所获取真实成交价）
	if orderID, ok := order["orderId"].(int64); ok && orderID > 0 {
		go at.SyncOpenTradeData(decision.Symbol, orderID)
	}

	// 记录开仓时间
	posKey := decision.Symbol + "_long"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// 设置止损（风控必需）
	var stopOrderID int64
	if stopOrderResult, err := at.trader.SetStopLoss(decision.Symbol, "LONG", quantity, decision.StopLoss); err != nil {
		// 如果错误提到"已存在"或"duplicate"，不视为错误
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "duplicate") || strings.Contains(errStr, "already exists") || strings.Contains(errStr, "已存在") {
			log.Printf("  ℹ 止损订单已存在，跳过设置")
		} else {
			log.Printf("  ⚠ 设置止损失败: %v", err)
		}
	} else {
		stopOrderID = stopOrderResult
		log.Printf("  ✓ 设置止损成功: %.2f (订单ID: %d)", decision.StopLoss, stopOrderID)
		
		// 🆕 注册止损单到WebSocket管理器
		if at.wsOrderManager != nil && stopOrderID > 0 {
			stopOrder := &TrackedOrder{
				OrderID:     stopOrderID,
				Symbol:      decision.Symbol,
				Side:        "long",
				Type:        "STOP_MARKET",
				Quantity:    quantity,
				StopPrice:   decision.StopLoss,
				TradeID:     "", // 可以从数据库获取
				CreatedAt:   time.Now(),
				Description: fmt.Sprintf("多头止损单 - 开仓时设置"),
			}
			at.wsOrderManager.TrackOrder(stopOrder)
			log.Printf("  📍 已将止损单注册到WebSocket管理器")
		}
	}
	
	// 移动止盈策略：开仓时不设置止盈，等待AI通过update_take_profit动作来设置
	log.Printf("  📈 采用移动止盈策略，开仓时不设置固定止盈价格")

	return nil
}

// getActualFillPrice 获取实际成交价格（通过持仓信息获取）
func (at *AutoTrader) getActualFillPrice(symbol, side string) float64 {
	positions, err := at.trader.GetPositions()
	if err != nil {
		log.Printf("  ⚠️ 获取持仓信息失败: %v", err)
		return 0
	}

	for _, pos := range positions {
		if pos["symbol"] == symbol && pos["side"] == side {
			if entryPrice, ok := pos["entryPrice"].(float64); ok && entryPrice > 0 {
				return entryPrice
			}
			if entryPrice, ok := pos["entryPrice"].(string); ok {
				if price, err := strconv.ParseFloat(entryPrice, 64); err == nil && price > 0 {
					return price
				}
			}
		}
	}
	
	return 0
}

// executeOpenShortWithRecord 执行开空仓并记录详细信息
func (at *AutoTrader) executeOpenShortWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  📉 开空仓: %s", decision.Symbol)

	// 🔍 智能仓位管理：检查是否可以加仓（最多3阶梯）
	positions, err := at.trader.GetPositions()
	if err == nil {
		shortPositionCount := 0
		for _, pos := range positions {
			if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
				shortPositionCount++
			}
		}
		
		// 检查是否超过最大阶梯数（3阶）
		if shortPositionCount >= 3 {
			log.Printf("  ⚠️ %s 已达最大持仓阶梯数（3阶），跳过加仓。如需调整仓位，请先减仓", decision.Symbol)
			return nil
		} else if shortPositionCount > 0 {
			log.Printf("  📊 %s 当前持有%d阶空仓，准备执行第%d阶加仓", decision.Symbol, shortPositionCount, shortPositionCount+1)
		}
	}

	// 获取当前价格
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}

	// 计算数量
	quantity := decision.PositionSizeUSD / marketData.CurrentPrice
	
	// 🔧 Binance期货最小名义价值检查：必须≥100 USDT
	notionalValue := quantity * marketData.CurrentPrice
	if notionalValue < 100.0 {
		// 调整到最小名义价值
		quantity = 100.0 / marketData.CurrentPrice
		adjustedNotional := quantity * marketData.CurrentPrice
		log.Printf("  ⚠️ 调整仓位大小: %.2f USDT → %.2f USDT (满足100 USDT最小要求)", 
			decision.PositionSizeUSD, adjustedNotional)
		decision.PositionSizeUSD = adjustedNotional // 更新决策中的仓位大小
	}
	actionRecord.Quantity = quantity
	// 暂时使用市场价格，执行后会更新为实际成交价
	actionRecord.Price = marketData.CurrentPrice

	// 保证金验证已在模板中优化处理，此处跳过验证直接执行

	// 设置仓位模式
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		log.Printf("  ⚠️ 设置仓位模式失败: %v", err)
		// 继续执行，不影响交易
	}

	// 开仓
	order, err := at.trader.OpenShort(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	log.Printf("  ✓ 开仓成功，订单ID: %v, 数量: %.4f", order["orderId"], quantity)

	// 🔧 关键修复：从交易所获取权威的开仓成交数据
	var actualPrice float64
	var priceDataSource string

	// 获取订单ID
	orderID, ok := order["orderId"].(int64)
	if !ok {
		log.Printf("  ⚠️ 无法获取订单ID，使用市场价格作为fallback")
		actualPrice = marketData.CurrentPrice
		priceDataSource = "MARKET_PRICE_FALLBACK"
	} else {
		// 等待订单确认（给交易所一些处理时间）
		time.Sleep(2 * time.Second)

		// 确保trader是FuturesTrader类型
		binanceTrader, isBinance := at.trader.(*FuturesTrader)
		if !isBinance {
			log.Printf("  ⚠️ 交易器不是Binance，使用市场价格")
			actualPrice = marketData.CurrentPrice
			priceDataSource = "MARKET_PRICE_NON_BINANCE"
		} else {
			// 从交易所获取权威开仓数据
			authData, err := binanceTrader.GetAuthoritativeOpenData(decision.Symbol, orderID)
			if err != nil {
				log.Printf("  ⚠️ 无法获取权威开仓数据: %v, 使用市场价格", err)
				actualPrice = marketData.CurrentPrice
				priceDataSource = "MARKET_PRICE_AUTH_FAILED"
			} else {
				actualPrice = authData.ActualPrice
				priceDataSource = authData.DataSource
				log.Printf("  ✅ [权威数据] 真实开仓价格: %.6f", actualPrice)
				log.Printf("     数据来源: %s", authData.DataSource)
				log.Printf("     成交数量: %.6f", authData.ActualQuantity)
				log.Printf("     手续费: %.4f %s", authData.Commission, authData.CommissionAsset)
				log.Printf("     是否挂单成交: %v", authData.IsMaker)
			}
		}
	}

	actionRecord.Price = actualPrice

	// 🔧 关键修复：添加缺失的数据库记录调用
	log.Printf("🔍 [调试] 准备记录空仓开仓到数据库:")
	log.Printf("    trader_id: '%s'", at.id)
	log.Printf("    symbol: '%s'", decision.Symbol)
	log.Printf("    side: 'short'")
	log.Printf("    quantity: %.6f", quantity)
	log.Printf("    leverage: %d", decision.Leverage)
	log.Printf("    actualPrice: %.6f (来源: %s)", actualPrice, priceDataSource)

	at.recordTradeToDatabase(decision.Symbol, "short", quantity, decision.Leverage,
		actualPrice, fmt.Sprintf("%v", order["orderId"]), "open_short", true, decision.StopLoss)

	// 🔥 新增：异步验证开仓数据（从交易所获取真实成交价）
	if orderID, ok := order["orderId"].(int64); ok && orderID > 0 {
		go at.SyncOpenTradeData(decision.Symbol, orderID)
	}

	// 记录开仓时间
	posKey := decision.Symbol + "_short"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// 设置止损（风控必需）
	var stopOrderID int64
	if stopOrderResult, err := at.trader.SetStopLoss(decision.Symbol, "SHORT", quantity, decision.StopLoss); err != nil {
		// 如果错误提到"已存在"或"duplicate"，不视为错误
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "duplicate") || strings.Contains(errStr, "already exists") || strings.Contains(errStr, "已存在") {
			log.Printf("  ℹ 止损订单已存在，跳过设置")
		} else {
			log.Printf("  ⚠ 设置止损失败: %v", err)
		}
	} else {
		stopOrderID = stopOrderResult
		log.Printf("  ✓ 设置止损成功: %.2f (订单ID: %d)", decision.StopLoss, stopOrderID)
		
		// 🆕 注册止损单到WebSocket管理器
		if at.wsOrderManager != nil && stopOrderID > 0 {
			stopOrder := &TrackedOrder{
				OrderID:     stopOrderID,
				Symbol:      decision.Symbol,
				Side:        "short",
				Type:        "STOP_MARKET",
				Quantity:    quantity,
				StopPrice:   decision.StopLoss,
				TradeID:     "", // 可以从数据库获取
				CreatedAt:   time.Now(),
				Description: fmt.Sprintf("空头止损单 - 开仓时设置"),
			}
			at.wsOrderManager.TrackOrder(stopOrder)
			log.Printf("  📍 已将止损单注册到WebSocket管理器")
		}
	}
	
	// 移动止盈策略：开仓时不设置止盈，等待AI通过update_take_profit动作来设置
	log.Printf("  📈 采用移动止盈策略，开仓时不设置固定止盈价格")

	return nil
}

// executeCloseLongWithRecord 执行平多仓并记录详细信息
func (at *AutoTrader) executeCloseLongWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 平多仓: %s", decision.Symbol)

	// 🔧 重要修复：获取更准确的市场价格用于平仓
	// 先尝试多次获取市场价格来获得更精确的即时价格
	var actualMarketPrice float64
	for i := 0; i < 3; i++ {
		if marketData, err := market.Get(decision.Symbol); err == nil {
			actualMarketPrice = marketData.CurrentPrice
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	
	if actualMarketPrice == 0 {
		return fmt.Errorf("无法获取市场价格")
	}
	
	actionRecord.Price = actualMarketPrice

	// 平仓
	order, err := at.trader.CloseLong(decision.Symbol, 0) // 0 = 全部平仓
	if err != nil {
		return err
	}

	// 🔧 修复：处理持仓已被其他方式关闭的情况
	if orderIDValue, exists := order["orderId"]; exists && orderIDValue == "ALREADY_CLOSED" {
		log.Printf("  ℹ️ %s 多仓已被其他方式平掉，同步更新数据库状态", decision.Symbol)
		
		// 🔧 改进：利用新增的字段信息进行更准确的同步
		estimatedPrice := actualMarketPrice
		closeReason := "unknown_external"
		detectionMethod := "basic"
		
		// 从返回信息中提取更多详细信息
		if estPrice, exists := order["estimated_price"]; exists {
			if price, ok := estPrice.(float64); ok && price > 0 {
				estimatedPrice = price
				log.Printf("  📊 使用估算平仓价格: %.6f", estimatedPrice)
			}
		}
		if reason, exists := order["close_reason"]; exists {
			if reasonStr, ok := reason.(string); ok {
				closeReason = reasonStr
			}
		}
		if method, exists := order["detection_method"]; exists {
			if methodStr, ok := method.(string); ok {
				detectionMethod = methodStr
			}
		}
		
		// 同步更新数据库状态为已关闭
		if at.database != nil {
			log.Printf("🔍 [调试] 同步关闭已平仓位updateTradeInDatabase参数:")
			log.Printf("    trader_id: '%s'", at.id)
			log.Printf("    symbol: '%s'", decision.Symbol)
			log.Printf("    side: 'long'")
			log.Printf("    estimatedPrice: %.6f", estimatedPrice)
			log.Printf("    orderID: 'SYNC_CLOSE'")
			log.Printf("    closeReason: '%s'", closeReason)
			log.Printf("    detectionMethod: '%s'", detectionMethod)
			
			log.Printf("  🔄 正在同步更新数据库中的交易记录状态...")
			at.updateTradeInDatabase(decision.Symbol, "long", 
				"SYNC_CLOSE", closeReason)
		}
		
		log.Printf("  ✓ 同步关闭完成，平仓价格: %.4f (方法: %s)", estimatedPrice, detectionMethod)
		return nil
	}

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// 🔧 关键修复：更新数据库中的交易记录状态
	if at.database != nil {
		// 安全获取orderID，处理nil情况
		var orderIDStr string
		if orderID, ok := order["orderId"]; ok && orderID != nil {
			orderIDStr = fmt.Sprintf("%v", orderID)
		} else {
			orderIDStr = "" // 如果orderID为nil，使用空字符串
			log.Printf("  ⚠️ [警告] 平仓订单ID为空或无效: %v", order["orderId"])
		}
		
		log.Printf("🔍 [调试] 主动平多仓updateTradeInDatabase参数:")
		log.Printf("    trader_id: '%s'", at.id)
		log.Printf("    symbol: '%s'", decision.Symbol)
		log.Printf("    side: 'long'")
		log.Printf("    actualMarketPrice: %.6f", actualMarketPrice)
		log.Printf("    orderID: '%s'", orderIDStr)
		log.Printf("    closeReason: 'ai_close'")
		
		log.Printf("  🔄 正在更新数据库中的交易记录状态...")
		at.updateTradeInDatabase(decision.Symbol, "long", 
			orderIDStr, "ai_close")
	}

	log.Printf("  ✓ 平仓成功，平仓价格: %.4f", actualMarketPrice)
	return nil
}

// executeCloseShortWithRecord 执行平空仓并记录详细信息
func (at *AutoTrader) executeCloseShortWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 平空仓: %s", decision.Symbol)

	// 🔧 重要修复：获取更准确的市场价格用于平仓
	// 先尝试多次获取市场价格来获得更精确的即时价格
	var actualMarketPrice float64
	for i := 0; i < 3; i++ {
		if marketData, err := market.Get(decision.Symbol); err == nil {
			actualMarketPrice = marketData.CurrentPrice
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	
	if actualMarketPrice == 0 {
		return fmt.Errorf("无法获取市场价格")
	}
	
	actionRecord.Price = actualMarketPrice

	// 平仓
	order, err := at.trader.CloseShort(decision.Symbol, 0) // 0 = 全部平仓
	if err != nil {
		return err
	}

	// 🔧 修复：处理持仓已被其他方式关闭的情况
	if orderIDValue, exists := order["orderId"]; exists && orderIDValue == "ALREADY_CLOSED" {
		log.Printf("  ℹ️ %s 空仓已被其他方式平掉，同步更新数据库状态", decision.Symbol)
		
		// 🔧 改进：利用新增的字段信息进行更准确的同步
		estimatedPrice := actualMarketPrice
		closeReason := "unknown_external"
		detectionMethod := "basic"
		
		// 从返回信息中提取更多详细信息
		if estPrice, exists := order["estimated_price"]; exists {
			if price, ok := estPrice.(float64); ok && price > 0 {
				estimatedPrice = price
				log.Printf("  📊 使用估算平仓价格: %.6f", estimatedPrice)
			}
		}
		if reason, exists := order["close_reason"]; exists {
			if reasonStr, ok := reason.(string); ok {
				closeReason = reasonStr
			}
		}
		if method, exists := order["detection_method"]; exists {
			if methodStr, ok := method.(string); ok {
				detectionMethod = methodStr
			}
		}
		
		// 同步更新数据库状态为已关闭
		if at.database != nil {
			log.Printf("🔍 [调试] 同步关闭已平仓位updateTradeInDatabase参数:")
			log.Printf("    trader_id: '%s'", at.id)
			log.Printf("    symbol: '%s'", decision.Symbol)
			log.Printf("    side: 'short'")
			log.Printf("    estimatedPrice: %.6f", estimatedPrice)
			log.Printf("    orderID: 'SYNC_CLOSE'")
			log.Printf("    closeReason: '%s'", closeReason)
			log.Printf("    detectionMethod: '%s'", detectionMethod)
			
			log.Printf("  🔄 正在同步更新数据库中的交易记录状态...")
			at.updateTradeInDatabase(decision.Symbol, "short",
				"SYNC_CLOSE", closeReason)
		}
		
		log.Printf("  ✓ 同步关闭完成，平仓价格: %.4f (方法: %s)", estimatedPrice, detectionMethod)
		return nil
	}

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// 🔧 关键修复：更新数据库中的交易记录状态
	if at.database != nil {
		// 安全获取orderID，处理nil情况
		var orderIDStr string
		if orderID, ok := order["orderId"]; ok && orderID != nil {
			orderIDStr = fmt.Sprintf("%v", orderID)
		} else {
			orderIDStr = "" // 如果orderID为nil，使用空字符串
			log.Printf("  ⚠️ [警告] 平仓订单ID为空或无效: %v", order["orderId"])
		}
		
		log.Printf("🔍 [调试] 主动平空仓updateTradeInDatabase参数:")
		log.Printf("    trader_id: '%s'", at.id)
		log.Printf("    symbol: '%s'", decision.Symbol)
		log.Printf("    side: 'short'")
		log.Printf("    actualMarketPrice: %.6f", actualMarketPrice)
		log.Printf("    orderID: '%s'", orderIDStr)
		log.Printf("    closeReason: 'ai_close'")
		
		log.Printf("  🔄 正在更新数据库中的交易记录状态...")
		at.updateTradeInDatabase(decision.Symbol, "short", 
			orderIDStr, "ai_close")
	}

	log.Printf("  ✓ 平仓成功，平仓价格: %.4f", actualMarketPrice)
	return nil
}

// GetID 获取trader ID
func (at *AutoTrader) GetID() string {
	return at.id
}

// GetName 获取trader名称
func (at *AutoTrader) GetName() string {
	return at.name
}

// GetAIModel 获取AI模型
func (at *AutoTrader) GetAIModel() string {
	return at.aiModel
}

// GetExchange 获取交易所
func (at *AutoTrader) GetExchange() string {
	return at.exchange
}

// SetCustomPrompt 设置自定义交易策略prompt
func (at *AutoTrader) SetCustomPrompt(prompt string) {
	at.customPrompt = prompt
}

// SetOverrideBasePrompt 设置是否覆盖基础prompt
func (at *AutoTrader) SetOverrideBasePrompt(override bool) {
	at.overrideBasePrompt = override
}

// SetSystemPromptTemplate 设置系统提示词模板
func (at *AutoTrader) SetSystemPromptTemplate(templateName string) {
	at.systemPromptTemplate = templateName
}

// GetSystemPromptTemplate 获取当前系统提示词模板名称
func (at *AutoTrader) GetSystemPromptTemplate() string {
	return at.systemPromptTemplate
}

// GetDecisionLogger 获取决策日志记录器
func (at *AutoTrader) GetDecisionLogger() *logger.DecisionLogger {
	return at.decisionLogger
}

// GetStatus 获取系统状态（用于API）
func (at *AutoTrader) GetStatus() map[string]interface{} {
	aiProvider := "DeepSeek"
	if at.config.UseQwen {
		aiProvider = "Qwen"
	}

	return map[string]interface{}{
		"trader_id":       at.id,
		"trader_name":     at.name,
		"ai_model":        at.aiModel,
		"exchange":        at.exchange,
		"is_running":      at.isRunning,
		"start_time":      at.startTime.Format(time.RFC3339),
		"runtime_minutes": int(time.Since(at.startTime).Minutes()),
		"call_count":      at.callCount,
		"initial_balance": at.initialBalance,
		"scan_interval":   at.config.ScanInterval.String(),
		"stop_until":      at.stopUntil.Format(time.RFC3339),
		"last_reset_time": at.lastResetTime.Format(time.RFC3339),
		"ai_provider":     aiProvider,
	}
}

// GetAccountInfo 获取账户信息（用于API）
func (at *AutoTrader) GetAccountInfo() (map[string]interface{}, error) {
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("获取余额失败: %w", err)
	}

	// 获取账户字段
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Total Equity = 钱包余额 + 未实现盈亏
	totalEquity := totalWalletBalance + totalUnrealizedProfit

	// 获取持仓计算总保证金
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	totalMarginUsed := 0.0
	totalUnrealizedPnL := 0.0
	for _, pos := range positions {
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		totalUnrealizedPnL += unrealizedPnl

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed
	}

	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	return map[string]interface{}{
		// 核心字段
		"total_equity":      totalEquity,           // 账户净值 = wallet + unrealized
		"wallet_balance":    totalWalletBalance,    // 钱包余额（不含未实现盈亏）
		"unrealized_profit": totalUnrealizedProfit, // 未实现盈亏（从API）
		"available_balance": availableBalance,      // 可用余额

		// 盈亏统计
		"total_pnl":            totalPnL,           // 总盈亏 = equity - initial
		"total_pnl_pct":        totalPnLPct,        // 总盈亏百分比
		"total_unrealized_pnl": totalUnrealizedPnL, // 未实现盈亏（从持仓计算）
		"initial_balance":      at.initialBalance,  // 初始余额
		"daily_pnl":            at.dailyPnL,        // 日盈亏

		// 持仓信息
		"position_count":  len(positions),  // 持仓数量
		"margin_used":     totalMarginUsed, // 保证金占用
		"margin_used_pct": marginUsedPct,   // 保证金使用率
	}, nil
}

// GetPositions 获取持仓列表（用于API）
func (at *AutoTrader) GetPositions() ([]map[string]interface{}, error) {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		// 计算占用保证金
		marginUsed := (quantity * markPrice) / float64(leverage)

		// 计算盈亏百分比（基于保证金）
		// 收益率 = 未实现盈亏 / 保证金 × 100%
		pnlPct := 0.0
		if marginUsed > 0 {
			pnlPct = (unrealizedPnl / marginUsed) * 100
		}

		// 🆕 获取当前的止损挂单价格（从交易所实时查询）
		var stopPrice float64 = 0.0
		
		// 查询该币种的所有挂单
		orders, err := at.trader.GetOpenOrders(symbol)
		if err != nil {
			log.Printf("⚠️ 查询 %s 挂单失败: %v", symbol, err)
		} else {
			// 查找对应持仓方向的止损单
			for _, order := range orders {
				orderType, _ := order["type"].(string)
				positionSide, _ := order["positionSide"].(string)
				stopPriceStr, _ := order["stopPrice"].(string)
				
				// 检查是否是止损单
				if orderType == "STOP_MARKET" || orderType == "STOP" {
					// 检查持仓方向是否匹配
					if (side == "long" && positionSide == "LONG") || 
					   (side == "short" && positionSide == "SHORT") {
						// 解析止损价格
						if price, parseErr := strconv.ParseFloat(stopPriceStr, 64); parseErr == nil && price > 0 {
							stopPrice = price
							break // 找到第一个匹配的止损单即可
						}
					}
				}
			}
		}

		result = append(result, map[string]interface{}{
			"symbol":             symbol,
			"side":               side,
			"entry_price":        entryPrice,
			"mark_price":         markPrice,
			"quantity":           quantity,
			"leverage":           leverage,
			"unrealized_pnl":     unrealizedPnl,
			"unrealized_pnl_pct": pnlPct,
			"liquidation_price":  liquidationPrice,
			"margin_used":        marginUsed,
			"stop_price":         stopPrice, // 🆕 添加止损价格
		})
	}

	return result, nil
}

// sortDecisionsByPriority 对决策排序：先平仓，再开仓，最后hold/wait
// 这样可以避免换仓时仓位叠加超限
func sortDecisionsByPriority(decisions []decision.Decision) []decision.Decision {
	if len(decisions) <= 1 {
		return decisions
	}

	// 定义优先级
	getActionPriority := func(action string) int {
		switch action {
		case "close_long", "close_short":
			return 1 // 最高优先级：先平仓
		case "open_long", "open_short":
			return 2 // 次优先级：后开仓
		case "hold", "wait":
			return 3 // 最低优先级：观望
		default:
			return 999 // 未知动作放最后
		}
	}

	// 复制决策列表
	sorted := make([]decision.Decision, len(decisions))
	copy(sorted, decisions)

	// 按优先级排序
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if getActionPriority(sorted[i].Action) > getActionPriority(sorted[j].Action) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// getCandidateCoins 获取交易员的候选币种列表
func (at *AutoTrader) getCandidateCoins() ([]decision.CandidateCoin, error) {
	log.Printf("🔍 [%s] getCandidateCoins开始: tradingCoins=%v (长度:%d), defaultCoins=%v (长度:%d)",
		at.name, at.tradingCoins, len(at.tradingCoins), at.defaultCoins, len(at.defaultCoins))
	log.Printf("🔍 [%s] tradingCoins详细: %+v", at.name, at.tradingCoins)
	log.Printf("🔍 [%s] defaultCoins详细: %+v", at.name, at.defaultCoins)

	if len(at.tradingCoins) == 0 {
		log.Printf("🔍 [%s] 条件判断: len(at.tradingCoins) == 0 为true，进入默认币种逻辑", at.name)
		// 使用数据库配置的默认币种列表
		var candidateCoins []decision.CandidateCoin

		if len(at.defaultCoins) > 0 {
			log.Printf("🔍 [%s] 条件判断: len(at.defaultCoins) > 0 为true，开始处理%d个默认币种", at.name, len(at.defaultCoins))
			// 使用数据库中配置的默认币种
			for i, coin := range at.defaultCoins {
				log.Printf("🔍 [%s] 处理第%d个币种: '%s'", at.name, i+1, coin)
				symbol := normalizeSymbol(coin)
				log.Printf("🔍 [%s] 标准化后: '%s'", at.name, symbol)
				if symbol != "" { // 跳过空币种
					candidateCoins = append(candidateCoins, decision.CandidateCoin{
						Symbol:  symbol,
						Sources: []string{"default"}, // 标记为数据库默认币种
					})
					log.Printf("🔍 [%s] 成功添加币种: %s", at.name, symbol)
				} else {
					log.Printf("⚠️ [%s] 币种标准化后为空，跳过: '%s'", at.name, coin)
				}
			}
			log.Printf("📋 [%s] 使用数据库默认币种: %d个币种 %v",
				at.name, len(candidateCoins), at.defaultCoins)
			log.Printf("🔍 [%s] 最终candidateCoins: %+v", at.name, candidateCoins)
			return candidateCoins, nil
		} else {
			log.Printf("🔍 [%s] 条件判断: len(at.defaultCoins) > 0 为false，defaultCoins为空！", at.name)
			// 如果数据库中没有配置默认币种，则使用AI500+OI Top作为fallback
			const ai500Limit = 20 // AI500取前20个评分最高的币种

			mergedPool, err := pool.GetMergedCoinPool(ai500Limit)
			if err != nil {
				log.Printf("⚠️ 获取合并币种池失败: %v，使用硬编码默认币种", err)
				// 使用硬编码默认币种作为最后的fallback
				hardcodedCoins := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT"}
				for _, coin := range hardcodedCoins {
					candidateCoins = append(candidateCoins, decision.CandidateCoin{
						Symbol:  coin,
						Sources: []string{"hardcoded"}, // 标记为硬编码来源
					})
				}
				log.Printf("📋 [%s] 使用硬编码默认币种: %d个币种 %v", at.name, len(candidateCoins), hardcodedCoins)
				return candidateCoins, nil
			}

			// 构建候选币种列表（包含来源信息）
			for _, symbol := range mergedPool.AllSymbols {
				sources := mergedPool.SymbolSources[symbol]
				candidateCoins = append(candidateCoins, decision.CandidateCoin{
					Symbol:  symbol,
					Sources: sources, // "ai500" 和/或 "oi_top"
				})
			}

			// 如果AI500+OI Top都没有返回币种，使用硬编码默认币种
			if len(candidateCoins) == 0 {
				log.Printf("⚠️ AI500+OI Top返回空列表，使用硬编码默认币种")
				hardcodedCoins := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT"}
				for _, coin := range hardcodedCoins {
					candidateCoins = append(candidateCoins, decision.CandidateCoin{
						Symbol:  coin,
						Sources: []string{"hardcoded"}, // 标记为硬编码来源
					})
				}
				log.Printf("📋 [%s] 使用硬编码默认币种: %d个币种 %v", at.name, len(candidateCoins), hardcodedCoins)
				return candidateCoins, nil
			}

			log.Printf("📋 [%s] 数据库无默认币种配置，使用AI500+OI Top: AI500前%d + OI_Top20 = 总计%d个候选币种",
				at.name, ai500Limit, len(candidateCoins))
			return candidateCoins, nil
		}
	} else {
		// 使用自��义币种列表
		var candidateCoins []decision.CandidateCoin
		for _, coin := range at.tradingCoins {
			// 跳过空字符串
			if strings.TrimSpace(coin) == "" {
				continue
			}
			// 确保币种格式正确（转为大写USDT交易对）
			symbol := normalizeSymbol(coin)
			if symbol != "" { // 再次检查，防止normalizeSymbol返回空
				candidateCoins = append(candidateCoins, decision.CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"custom"}, // 标记为自定义来源
				})
			}
		}

		// 如果自定义币种列表处理后为空，使用硬编码默认币种
		if len(candidateCoins) == 0 {
			log.Printf("⚠️ [%s] 自定义币种处理后为空，使用硬编码默认币种", at.name)
			hardcodedCoins := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT"}
			for _, coin := range hardcodedCoins {
				candidateCoins = append(candidateCoins, decision.CandidateCoin{
					Symbol:  coin,
					Sources: []string{"hardcoded"}, // 标记为硬编码来源
				})
			}
			log.Printf("📋 [%s] 使用硬编码默认币种: %d个币种 %v", at.name, len(candidateCoins), hardcodedCoins)
			return candidateCoins, nil
		}

		log.Printf("📋 [%s] 使用自定义币种: %d个币种 %v",
			at.name, len(candidateCoins), at.tradingCoins)
		return candidateCoins, nil
	}
}

// normalizeSymbol 标准化币种符号（确保以USDT结尾）
func normalizeSymbol(symbol string) string {
	// 转为大写并去除空格
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	// 如果是空字符串，返回空（避免生成无效的"USDT"）
	if symbol == "" {
		log.Printf("⚠️ normalizeSymbol: 传入空币种符号")
		return ""
	}

	// 确保以USDT结尾
	if !strings.HasSuffix(symbol, "USDT") {
		symbol = symbol + "USDT"
	}

	return symbol
}

// executeReduceWithRecord 执行智能减仓（根据当前持仓自动判断方向）
func (at *AutoTrader) executeReduceWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 智能减仓: %s", decision.Symbol)
	
	// 获取当前持仓，判断减仓方向
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓失败: %w", err)
	}
	
	var hasLong, hasShort bool
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol {
			side := pos["side"].(string)
			quantity := pos["positionAmt"].(float64)
			
			if side == "long" && quantity > 0 {
				hasLong = true
			} else if side == "short" && quantity < 0 {
				hasShort = true
			}
		}
	}
	
	// 根据持仓情况执行相应的减仓操作
	if hasLong && hasShort {
		// 同时有多空仓位，默认减多仓（可以根据盈亏情况调整）
		return at.executeReduceLongWithRecord(decision, actionRecord)
	} else if hasLong {
		// 只有多仓，减多仓
		return at.executeReduceLongWithRecord(decision, actionRecord)
	} else if hasShort {
		// 只有空仓，减空仓  
		return at.executeReduceShortWithRecord(decision, actionRecord)
	} else {
		return fmt.Errorf("没有找到%s的持仓，无法执行减仓", decision.Symbol)
	}
}

// executeReduceLongWithRecord 执行减多仓并记录详细信息
func (at *AutoTrader) executeReduceLongWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 减多仓: %s", decision.Symbol)
	
	// 获取当前多仓持仓
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓失败: %w", err)
	}
	
	var currentQuantity float64
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
			currentQuantity = pos["positionAmt"].(float64)
			break
		}
	}
	
	if currentQuantity <= 0 {
		return fmt.Errorf("没有找到%s的多仓持仓", decision.Symbol)
	}
	
	// 确定减仓数量（如果decision中指定了数量则使用，否则减仓50%）
	var reduceQuantity float64
	if decision.PositionSizeUSD > 0 {
		// 根据USD金额计算减仓数量
		marketData, err := market.Get(decision.Symbol)
		if err != nil {
			return err
		}
		reduceQuantity = decision.PositionSizeUSD / marketData.CurrentPrice
	} else {
		// 默认减仓50%
		reduceQuantity = currentQuantity * 0.5
	}
	
	// 确保不超过当前持仓
	if reduceQuantity > currentQuantity {
		reduceQuantity = currentQuantity
	}
	
	actionRecord.Quantity = reduceQuantity
	
	// 获取当前价格用于记录
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice
	
	// 执行减仓（部分平仓）
	order, err := at.trader.CloseLong(decision.Symbol, reduceQuantity)
	if err != nil {
		return err
	}
	
	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}
	
	log.Printf("  ✓ 减多仓成功，订单ID: %v, 减仓数量: %.4f", order["orderId"], reduceQuantity)
	return nil
}

// executeReduceShortWithRecord 执行减空仓并记录详细信息
func (at *AutoTrader) executeReduceShortWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 减空仓: %s", decision.Symbol)
	
	// 获取当前空仓持仓
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持���失败: %w", err)
	}
	
	var currentQuantity float64
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
			currentQuantity = math.Abs(pos["positionAmt"].(float64)) // 空仓数量为负数，取绝对值
			break
		}
	}
	
	if currentQuantity <= 0 {
		return fmt.Errorf("没有找到%s的空仓持仓", decision.Symbol)
	}
	
	// 确定减仓数量（如果decision中指定了数量则使用，否则减仓50%）
	var reduceQuantity float64
	if decision.PositionSizeUSD > 0 {
		// 根据USD金额计算减仓数量
		marketData, err := market.Get(decision.Symbol)
		if err != nil {
			return err
		}
		reduceQuantity = decision.PositionSizeUSD / marketData.CurrentPrice
	} else {
		// 默认减仓50%
		reduceQuantity = currentQuantity * 0.5
	}
	
	// 确保不超过当前持仓
	if reduceQuantity > currentQuantity {
		reduceQuantity = currentQuantity
	}
	
	actionRecord.Quantity = reduceQuantity
	
	// 获取当前价格用于记录
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice
	
	// 执行减仓（部分平仓）
	order, err := at.trader.CloseShort(decision.Symbol, reduceQuantity)
	if err != nil {
		return err
	}
	
	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}
	
	log.Printf("  ✓ 减空仓成功，订单ID: %v, 减仓数量: %.4f", order["orderId"], reduceQuantity)
	return nil
}

// executeUpdateStopWithRecord 执行更新止损并记录详细信息
func (at *AutoTrader) executeUpdateStopWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 更新止损: %s", decision.Symbol)
	
	// 获取当前持仓，判断持仓方向
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓失败: %w", err)
	}
	
	var hasLong, hasShort bool
	var longQuantity, shortQuantity float64
	var hasExistingStopOrder bool = false
	
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol {
			side := pos["side"].(string)
			quantity := pos["positionAmt"].(float64)
			
			if side == "long" && quantity > 0 {
				hasLong = true
				longQuantity = quantity
				
				// 检查是否存在多仓止损单 (通过PrevStop字段检查)
				if prevStop, ok := pos["prev_stop"]; ok {
					if prevStopPrice, ok := prevStop.(float64); ok && prevStopPrice > 0 {
						hasExistingStopOrder = true
					}
				}
			} else if side == "short" && quantity < 0 {
				hasShort = true
				shortQuantity = math.Abs(quantity)
				
				// 检查是否存在空仓止损单
				if prevStop, ok := pos["prev_stop"]; ok {
					if prevStopPrice, ok := prevStop.(float64); ok && prevStopPrice > 0 {
						hasExistingStopOrder = true
					}
				}
			}
		}
	}
	
	if !hasLong && !hasShort {
		return fmt.Errorf("没有找到%s的持仓，无法设置止损", decision.Symbol)
	}
	
	// 🔧 统一处理：无论是更新还是创建止损单
	if hasExistingStopOrder {
		log.Printf("  🔄 %s 更新现有止损单至%.6f", decision.Symbol, decision.StopLoss)
	} else {
		log.Printf("  📌 %s 当前没有止损单，创建新的止损保护(%.6f)", decision.Symbol, decision.StopLoss)
	}
	
	// 获取当前价格用于验证
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}
	// 记录止损价格而不是市场价格（前端显示的是操作的目标价格）
	actionRecord.Price = decision.StopLoss
	
	// 先取消现有止损订单
	if err := at.trader.CancelAllOrders(decision.Symbol); err != nil {
		log.Printf("  ⚠ 取消现有订单失败: %v", err)
	}
	
	// 调试日志：打印AI传递的止损价格
	log.Printf("  🔍 [调试] AI传递的止损价格: %.6f", decision.StopLoss)
	log.Printf("  🔍 [调试] 当前市场价格: %.6f", marketData.CurrentPrice)
	
	// 验证止损价格的合理性
	if decision.StopLoss <= 0 {
		return fmt.Errorf("止损价格无效: %.6f (必须大于0)", decision.StopLoss)
	}
	
	// 根据持仓方向设置新的止损
	if hasLong {
		actionRecord.Quantity = longQuantity
		// 🔧 修复：多仓止损验证逻辑
		// 多仓止损分两种情况：
		// 1. 初始止损：应该低于当前价格（防止亏损扩大）
		// 2. 移动止损：可以高于当前价格（锁定利润）
		// 
		// 获取当前持仓的入场价格用于判断
		var entryPrice float64
		for _, pos := range positions {
			if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
				entryPrice = pos["entryPrice"].(float64)
				break
			}
		}
		
		// 基本合理性检查：止损价格不应偏离当前价格过远
		maxDeviation := marketData.CurrentPrice * 0.2 // 20%偏差范围
		if decision.StopLoss < marketData.CurrentPrice - maxDeviation {
			log.Printf("  ⚠️ [警告] 多仓止损价格(%.6f)过低，超出合理范围(%.6f)", decision.StopLoss, marketData.CurrentPrice - maxDeviation)
			return fmt.Errorf("多仓止损价格(%.2f)过低，建议在%.2f以上", decision.StopLoss, marketData.CurrentPrice - maxDeviation)
		}
		
		// 如果有入场价格，进行更精确的验证
		if entryPrice > 0 {
			// 多仓盈利时（当前价格 > 入场价格），允许移动止损
			if marketData.CurrentPrice > entryPrice {
				log.Printf("  📈 多仓盈利中(当前%.6f > 入场%.6f)，允许移动止损至%.6f", 
					marketData.CurrentPrice, entryPrice, decision.StopLoss)
			} else {
				// 多仓亏损时，止损应该低��当前价格以限制亏损
				if decision.StopLoss >= marketData.CurrentPrice {
					log.Printf("  ⚠️ [警告] 多仓亏损中，止损价格(%.6f)应低于当前价格(%.6f)", decision.StopLoss, marketData.CurrentPrice)
					return fmt.Errorf("多仓亏损中，止损价格(%.2f)应低于当前价格(%.2f)", decision.StopLoss, marketData.CurrentPrice)
				}
			}
		}
		log.Printf("  🔄 设置多仓止损: 数量=%.4f, 止损价格=%.6f", longQuantity, decision.StopLoss)
		orderID, err := at.trader.SetStopLoss(decision.Symbol, "LONG", longQuantity, decision.StopLoss)
		if err != nil {
			return fmt.Errorf("设置多仓止损失败: %w", err)
		}
		
		// 记录待确认的止损单
		pendingOrder := &PendingStopOrder{
			Symbol:         decision.Symbol,
			Side:           "long",
			OrderID:        orderID,
			StopPrice:      decision.StopLoss,
			Quantity:       longQuantity,
			CreateTime:     time.Now(),
			OriginalAction: decision.Action,
		}
		
		pendingKey := fmt.Sprintf("%s_long_stop", decision.Symbol)
		at.pendingStopOrders[pendingKey] = pendingOrder
		
		// 🆕 注册到WebSocket管理器（替换旧的跟踪）
		if at.wsOrderManager != nil {
			wsOrder := &TrackedOrder{
				OrderID:     orderID,
				Symbol:      decision.Symbol,
				Side:        "long",
				Type:        "STOP_MARKET",
				Quantity:    longQuantity,
				StopPrice:   decision.StopLoss,
				TradeID:     "", // 可以从数据库获取
				CreatedAt:   time.Now(),
				Description: fmt.Sprintf("多头止损更新 - %s", decision.Action),
			}
			at.wsOrderManager.TrackOrder(wsOrder)
			log.Printf("  📍 已将更新的止损单注册到WebSocket管理器")
		}
		
		actionRecord.OrderID = orderID
		log.Printf("  ✓ 更新多仓止损成功: %.6f (订单ID: %d)", decision.StopLoss, orderID)
		log.Printf("  📋 已记录待确认止损单: %s", pendingKey)
	}
	
	if hasShort {
		actionRecord.Quantity = shortQuantity
		// 🔧 修复：空仓止损验证逻辑
		// 空仓止损分两种情况：
		// 1. 初始止损：应该高于当前价格（防止亏损扩大）
		// 2. 移动止损：可以低于当前价格（锁定利润）
		// 
		// 获取当前持仓的入场价格用于判断
		var entryPrice float64
		for _, pos := range positions {
			if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
				entryPrice = pos["entryPrice"].(float64)
				break
			}
		}
		
		// 基本合理性检查：止损价格不应偏离当前价格过远
		maxDeviation := marketData.CurrentPrice * 0.2 // 20%偏差范围
		if decision.StopLoss > marketData.CurrentPrice + maxDeviation {
			log.Printf("  ⚠️ [警告] 空仓止损价格(%.6f)过高，超出合理范围(%.6f)", decision.StopLoss, marketData.CurrentPrice + maxDeviation)
			return fmt.Errorf("空仓止损价格(%.2f)过高，建议在%.2f以下", decision.StopLoss, marketData.CurrentPrice + maxDeviation)
		}
		
		// 如果有入场价格，进行更精确的验证
		if entryPrice > 0 {
			// 空仓盈利时（当前价格 < 入场价格），允许移动止损
			if marketData.CurrentPrice < entryPrice {
				log.Printf("  📈 空仓盈利中(入场%.6f > 当前%.6f)，允许移动止损至%.6f", 
					entryPrice, marketData.CurrentPrice, decision.StopLoss)
			} else {
				// 空仓亏损时，止损应该高于当前价格以限制亏损
				if decision.StopLoss <= marketData.CurrentPrice {
					log.Printf("  ⚠️ [警告] 空仓亏损中，止损价格(%.6f)应高于当前价格(%.6f)", decision.StopLoss, marketData.CurrentPrice)
					return fmt.Errorf("空仓亏损中，止损价格(%.2f)应高于当前价格(%.2f)", decision.StopLoss, marketData.CurrentPrice)
				}
			}
		}
		log.Printf("  🔄 设置空仓止损: 数量=%.4f, 止损价格=%.6f", shortQuantity, decision.StopLoss)
		orderID, err := at.trader.SetStopLoss(decision.Symbol, "SHORT", shortQuantity, decision.StopLoss)
		if err != nil {
			return fmt.Errorf("设置空仓止损失败: %w", err)
		}
		
		// 记录待确认的止损单
		pendingOrder := &PendingStopOrder{
			Symbol:         decision.Symbol,
			Side:           "short",
			OrderID:        orderID,
			StopPrice:      decision.StopLoss,
			Quantity:       shortQuantity,
			CreateTime:     time.Now(),
			OriginalAction: decision.Action,
		}
		
		pendingKey := fmt.Sprintf("%s_short_stop", decision.Symbol)
		at.pendingStopOrders[pendingKey] = pendingOrder
		
		// 🆕 注册到WebSocket管理器（替换旧的跟踪）
		if at.wsOrderManager != nil {
			wsOrder := &TrackedOrder{
				OrderID:     orderID,
				Symbol:      decision.Symbol,
				Side:        "short",
				Type:        "STOP_MARKET",
				Quantity:    shortQuantity,
				StopPrice:   decision.StopLoss,
				TradeID:     "", // 可以从数据库获取
				CreatedAt:   time.Now(),
				Description: fmt.Sprintf("空头止损更新 - %s", decision.Action),
			}
			at.wsOrderManager.TrackOrder(wsOrder)
			log.Printf("  📍 已将更新的止损单注册到WebSocket管理器")
		}
		
		actionRecord.OrderID = orderID
		log.Printf("  ✓ 更新空仓止损成功: %.6f (订单ID: %d)", decision.StopLoss, orderID)
		log.Printf("  📋 已记录待确认止损单: %s", pendingKey)
	}
	
	return nil
}

// executeUpdateTakeProfitWithRecord 执行更新止盈并记录详细信息
func (at *AutoTrader) executeUpdateTakeProfitWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 更新止盈: %s", decision.Symbol)
	
	// 获取当前持仓，判断持仓方向
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓失败: %w", err)
	}
	
	var hasLong, hasShort bool
	var longQuantity, shortQuantity float64
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol {
			side := pos["side"].(string)
			quantity := pos["positionAmt"].(float64)
			
			if side == "long" && quantity > 0 {
				hasLong = true
				longQuantity = quantity
			} else if side == "short" && quantity < 0 {
				hasShort = true
				shortQuantity = math.Abs(quantity)
			}
		}
	}
	
	if !hasLong && !hasShort {
		return fmt.Errorf("没有找到%s的持仓，无法更新止盈", decision.Symbol)
	}
	
	// 获取当前价格用于记录
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice
	
	// 根据持仓方向设置新的止盈
	if hasLong {
		actionRecord.Quantity = longQuantity
		if err := at.trader.SetTakeProfit(decision.Symbol, "LONG", longQuantity, decision.TakeProfit); err != nil {
			return fmt.Errorf("设置多仓止盈失败: %w", err)
		}
		log.Printf("  ✓ 更新多仓止盈成功: %.2f", decision.TakeProfit)
	}
	
	if hasShort {
		actionRecord.Quantity = shortQuantity
		if err := at.trader.SetTakeProfit(decision.Symbol, "SHORT", shortQuantity, decision.TakeProfit); err != nil {
			return fmt.Errorf("设置空仓止盈失败: %w", err)
		}
		log.Printf("  ✓ 更新空仓止盈成功: %.2f", decision.TakeProfit)
	}
	
	return nil
}

// executePartialCloseWithRecord 执行部分平仓并记录详细信息
func (at *AutoTrader) executePartialCloseWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 部分平仓: %s", decision.Symbol)
	
	// 获取当前持仓，判断持仓方向
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓失败: %w", err)
	}
	
	var hasLong, hasShort bool
	var longQuantity, shortQuantity float64
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol {
			side := pos["side"].(string)
			quantity := pos["positionAmt"].(float64)
			
			if side == "long" && quantity > 0 {
				hasLong = true
				longQuantity = quantity
			} else if side == "short" && quantity < 0 {
				hasShort = true
				shortQuantity = math.Abs(quantity)
			}
		}
	}
	
	if !hasLong && !hasShort {
		return fmt.Errorf("没有找到%s的持仓，无法部分平仓", decision.Symbol)
	}
	
	// 获取当前价格用于记录
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice
	
	// 计算平仓数量（如果指定了USD金额则转换，否则默认平仓50%）
	var closeQuantity float64
	if decision.PositionSizeUSD > 0 {
		closeQuantity = decision.PositionSizeUSD / marketData.CurrentPrice
	} else {
		// 默认平仓50%
		if hasLong {
			closeQuantity = longQuantity * 0.5
		} else {
			closeQuantity = shortQuantity * 0.5
		}
	}
	
	actionRecord.Quantity = closeQuantity
	
	// 根据持仓方向执行部分平仓
	if hasLong {
		// 确保不超过当前持仓
		if closeQuantity > longQuantity {
			closeQuantity = longQuantity
		}
		
		order, err := at.trader.CloseLong(decision.Symbol, closeQuantity)
		if err != nil {
			return fmt.Errorf("部分平多仓失败: %w", err)
		}
		
		// 记录订单ID
		if orderID, ok := order["orderId"].(int64); ok {
			actionRecord.OrderID = orderID
		}
		
		log.Printf("  ✓ 部分平多仓成功，订单ID: %v, 平仓数量: %.4f", order["orderId"], closeQuantity)
	}
	
	if hasShort {
		// 确保不超过当前持仓
		if closeQuantity > shortQuantity {
			closeQuantity = shortQuantity
		}
		
		order, err := at.trader.CloseShort(decision.Symbol, closeQuantity)
		if err != nil {
			return fmt.Errorf("部分平空仓失败: %w", err)
		}
		
		// 记录订单ID
		if orderID, ok := order["orderId"].(int64); ok {
			actionRecord.OrderID = orderID
		}
		
		log.Printf("  ✓ 部分平空仓成功，订单ID: %v, 平仓数量: %.4f", order["orderId"], closeQuantity)
	}
	
	return nil
}

// executeCloseAllPositionsWithRecord 执行通用平仓（智能判断持仓方向）并记录详细信息
func (at *AutoTrader) executeCloseAllPositionsWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 通用平仓: %s", decision.Symbol)
	
	// 获取当前持仓，判断持仓方向
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓失败: %w", err)
	}
	
	var hasLong, hasShort bool
	var longQuantity, shortQuantity float64
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol {
			side := pos["side"].(string)
			quantity := pos["positionAmt"].(float64)
			
			if side == "long" && quantity > 0 {
				hasLong = true
				longQuantity = quantity
			} else if side == "short" && quantity < 0 {
				hasShort = true
				shortQuantity = math.Abs(quantity)
			}
		}
	}
	
	if !hasLong && !hasShort {
		return fmt.Errorf("没有找到%s的持仓，无法平仓", decision.Symbol)
	}
	
	// 获取当前价格用于记录
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice
	
	// 根据持仓方向执行平仓
	if hasLong {
		actionRecord.Quantity = longQuantity
		order, err := at.trader.CloseLong(decision.Symbol, 0) // 0 = 全部平仓
		if err != nil {
			return fmt.Errorf("平多仓失败: %w", err)
		}
		
		// 记录订单ID
		if orderID, ok := order["orderId"].(int64); ok {
			actionRecord.OrderID = orderID
		}
		
		log.Printf("  ✓ 平多仓成功，订单ID: %v, 平仓价格: %.4f", order["orderId"], marketData.CurrentPrice)
	}
	
	if hasShort {
		actionRecord.Quantity = shortQuantity
		order, err := at.trader.CloseShort(decision.Symbol, 0) // 0 = 全部平仓
		if err != nil {
			return fmt.Errorf("平空仓失败: %w", err)
		}
		
		// 记录订单ID
		if orderID, ok := order["orderId"].(int64); ok {
			actionRecord.OrderID = orderID
		}
		
		log.Printf("  ✓ 平空仓成功，订单ID: %v, 平仓价格: %.4f", order["orderId"], marketData.CurrentPrice)
	}
	
	return nil
}

// checkPendingStopOrders 检查待确认的止损单状态（已简化 - 主要依赖WebSocket）
func (at *AutoTrader) checkPendingStopOrders(record *logger.DecisionRecord) error {
	// 🆕 优化：主要依赖WebSocket实时监控，大幅减少轮询检查频率
	if at.wsOrderManager != nil && at.wsOrderManager.IsConnected() {
		log.Printf("🔗 [Stop Check] WebSocket连接正常，使用实时监控模式")
		
		// WebSocket正常时，只做超轻量级检查（每30个周��一次）
		if at.callCount%30 == 0 {
			log.Printf("🔍 [Stop Check] 执行超轻量级检查 (周期 #%d)", at.callCount)
			if err := at.checkTrackedStopOrdersUltraLightweight(record); err != nil {
				log.Printf("⚠️ 超轻量级止损单检查失败: %v", err)
			}
		}
		return nil
	}
	
	// WebSocket不可用时，使用轻量级轮询模式（频率降低）
	log.Printf("⚠️ [Stop Check] WebSocket不可用，使用轻量级轮询模式")
	
	// 每5个周期检查一次（之前是每个周期都检查）
	if at.callCount%5 == 0 {
		log.Printf("🔍 [Stop Check] 执行轮询检查 (周期 #%d)", at.callCount)
		
		// 1. 检查内存中跟踪的止损单（AI更新的）
		if err := at.checkTrackedStopOrders(record); err != nil {
			log.Printf("⚠️ 检查跟踪的止损单失败: %v", err)
		}
		
		// 2. 检查所有持仓的止损单（降频，每15个周期执行一次）
		if at.callCount%15 == 0 {
			if err := at.checkAllPositionStopOrders(record); err != nil {
				log.Printf("⚠️ 检查所有持仓止损单失败: %v", err)
			}
		}
	}
	
	return nil
}

// checkTrackedStopOrdersUltraLightweight 超轻量级止损单检查（WebSocket模式下使用）
func (at *AutoTrader) checkTrackedStopOrdersUltraLightweight(record *logger.DecisionRecord) error {
	// 仅清理明显���效的跟踪记录，不执行API调用
	if len(at.pendingStopOrders) == 0 {
		return nil
	}
	
	log.Printf("🔍 [Ultra Lightweight] 检查 %d 个内存中的止损单（无API调用）", len(at.pendingStopOrders))
	
	// 获取当前持仓，仅用于清理无关记录
	positions, err := at.trader.GetPositions()
	if err != nil {
		log.Printf("⚠️ [Ultra Lightweight] 获取持仓失败，跳过清理: %v", err)
		return nil
	}
	
	// 建立持仓映射
	positionMap := make(map[string]bool)
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		quantity := pos["positionAmt"].(float64)
		if (side == "long" && quantity > 0) || (side == "short" && quantity < 0) {
			key := symbol + "_" + side
			positionMap[key] = true
		}
	}
	
	// 清理已平仓的跟踪记录（超轻量级，不查询订单状态）
	var toRemove []string
	for key, pendingOrder := range at.pendingStopOrders {
		posKey := pendingOrder.Symbol + "_" + pendingOrder.Side
		if !positionMap[posKey] {
			log.Printf("🧹 [Ultra Lightweight] 持仓已消失，清理跟踪记录: %s", key)
			toRemove = append(toRemove, key)
		}
	}
	
	// 移除已平仓的记录
	for _, key := range toRemove {
		delete(at.pendingStopOrders, key)
	}
	
	if len(toRemove) > 0 {
		log.Printf("✅ [Ultra Lightweight] 清理了 %d 个无效的跟踪记录", len(toRemove))
	}
	
	return nil
}

// checkTrackedStopOrdersLightweight 轻量级止损单检查（WebSocket模式下使用）
func (at *AutoTrader) checkTrackedStopOrdersLightweight(record *logger.DecisionRecord) error {
	// 只检查内存中还在跟踪但可能已成交的订单
	if len(at.pendingStopOrders) == 0 {
		return nil
	}
	
	log.Printf("🔍 [Lightweight Check] 检查 %d 个内存中的止损单", len(at.pendingStopOrders))
	
	// 简化版检查：只验证持仓是否仍存在
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓失败: %w", err)
	}
	
	// 建立持仓映射
	positionMap := make(map[string]bool)
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		quantity := pos["positionAmt"].(float64)
		if (side == "long" && quantity > 0) || (side == "short" && quantity < 0) {
			key := symbol + "_" + side
			positionMap[key] = true
		}
	}
	
	// 清理已平仓的跟踪记录，但先尝试补偿闭合
	var toRemove []string
	for key, pendingOrder := range at.pendingStopOrders {
		posKey := pendingOrder.Symbol + "_" + pendingOrder.Side
		if !positionMap[posKey] {
			log.Printf("🧹 [Lightweight Check] 持仓已消失，尝试补偿闭合: %s", key)
			
			// 🎯 修复：先尝试补偿闭合，再删除跟踪
			at.tryReconcileClose(pendingOrder.Symbol, pendingOrder.Side, pendingOrder)
			toRemove = append(toRemove, key)
		}
	}
	
	// 移除已平仓的记录
	for _, key := range toRemove {
		delete(at.pendingStopOrders, key)
	}
	
	if len(toRemove) > 0 {
		log.Printf("✅ [Lightweight Check] 清理了 %d 个已平仓的跟踪记录", len(toRemove))
	}
	
	return nil
}

// tryReconcileClose 尝试补偿性闭合（当持仓消失但跟踪记录还在时）
func (at *AutoTrader) tryReconcileClose(symbol, side string, p *PendingStopOrder) bool {
	// 冷却 30 秒，最多 5 次
	if p.ReconcileAttempts >= 5 {
		log.Printf("⚠️ [ReconcileClose] %s_%s 已达最大重试次数，放弃补偿闭合", symbol, side)
		return true // 放弃跟踪，避免死循环
	}
	if !p.LastReconcileAt.IsZero() && time.Since(p.LastReconcileAt) < 30*time.Second {
		log.Printf("⏰ [ReconcileClose] %s_%s 冷却中，跳过补偿闭合", symbol, side)
		return false // 保持跟踪，等待下次尝试
	}
	
	p.ReconcileAttempts++
	p.LastReconcileAt = time.Now()
	
	log.Printf("🔧 [ReconcileClose] 尝试补偿闭合 %s_%s (第%d次尝试)", symbol, side, p.ReconcileAttempts)
	at.updateTradeInDatabase(symbol, side, "unknown", "position_disappeared")
	
	return true // 假定成功，删除跟踪记录
}

// checkTrackedStopOrders 检查内存中跟踪的止损单（原有逻辑）
func (at *AutoTrader) checkTrackedStopOrders(record *logger.DecisionRecord) error {
	if len(at.pendingStopOrders) == 0 {
		return nil
	}

	log.Printf("🔍 检查 %d 个跟踪的止损单状态", len(at.pendingStopOrders))
	
	var toRemove []string
	
	for key, pendingOrder := range at.pendingStopOrders {
		log.Printf("  🔍 检查止损单: %s (ID: %d)", pendingOrder.Symbol, pendingOrder.OrderID)
		
		// 查询订单状态
		orderStatus, err := at.trader.GetOrderStatus(pendingOrder.Symbol, pendingOrder.OrderID)
		if err != nil {
			log.Printf("  ⚠️ 查询订单 %d 状态失败: %v", pendingOrder.OrderID, err)
			
			// 订单已经不存在，可能已经成交或被取消
			// 检查持仓是否变化来判断是否成交
			positions, posErr := at.trader.GetPositions()
			if posErr == nil {
				hasPosition := false
				for _, pos := range positions {
					if pos["symbol"] == pendingOrder.Symbol && pos["side"] == pendingOrder.Side {
						if quantity, ok := pos["positionAmt"].(float64); ok && quantity > 0 {
							hasPosition = true
							break
						}
					}
				}
				
				// 🔧 加强验证：如果持仓消失，进一步确认是否真的是止损成交
				if !hasPosition {
					// 验证PendingStopOrder的数据完整性
					if pendingOrder.Quantity > 0 && pendingOrder.StopPrice > 0 {
						log.Printf("  ✅ 止损单成交: %s %s (推断，已验证数据)", pendingOrder.Symbol, pendingOrder.Side)
						at.recordStopLossExecution(pendingOrder, record)
						toRemove = append(toRemove, key)
					} else {
						log.Printf("  ❌ 持仓消失但数据不完整，跳过记录: quantity=%.6f, stopPrice=%.6f", 
							pendingOrder.Quantity, pendingOrder.StopPrice)
						// 仍然移除无效的跟踪记录
						toRemove = append(toRemove, key)
					}
				}
			}
			continue
		}
		
		// 解析订单状态
		status, ok := orderStatus["status"].(string)
		if !ok {
			log.Printf("  ⚠️ 无法解析订单状态: %v", orderStatus)
			continue
		}
		
		log.Printf("  📊 订单 %d 状态: %s", pendingOrder.OrderID, status)
		
		// 检查订单是否已成交
		if status == "FILLED" || status == "PARTIALLY_FILLED" {
			log.Printf("  ✅ 止损单成交: %s %s", pendingOrder.Symbol, pendingOrder.Side)
			at.recordStopLossExecution(pendingOrder, record)
			toRemove = append(toRemove, key)
		} else if status == "CANCELED" || status == "REJECTED" || status == "EXPIRED" {
			log.Printf("  ❌ 止损单失效: %s %s (状态: %s)", pendingOrder.Symbol, pendingOrder.Side, status)
			toRemove = append(toRemove, key)
		}
	}
	
	// 移除已处理的订单
	for _, key := range toRemove {
		delete(at.pendingStopOrders, key)
	}
	
	if len(toRemove) > 0 {
		log.Printf("📝 移除了 %d 个已处理的止损单记录", len(toRemove))
	}
	
	return nil
}

// checkAllPositionStopOrders 检查所有当前持仓的止损单状态（包括开仓时设置的）
func (at *AutoTrader) checkAllPositionStopOrders(record *logger.DecisionRecord) error {
	log.Printf("🔍 检查所有持仓的止损挂单状态...")
	
	// 获取当前持仓
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓失败: %w", err)
	}
	
	if len(positions) == 0 {
		return nil // 无持仓，无需检查
	}
	
	// 为每个持仓检查其止损挂单
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		posQuantity := pos["positionAmt"].(float64)
		if posQuantity < 0 {
			posQuantity = -posQuantity // 空仓为负数，转为正数
		}
		
		log.Printf("  🔍 检查 %s %s 的止损挂单 (持仓数量: %.6f)", symbol, side, posQuantity)
		
		// 查询该币种的所有挂单
		orders, err := at.trader.GetOpenOrders(symbol)
		if err != nil {
			log.Printf("  ❌ 查询 %s 挂单失败: %v", symbol, err)
			continue
		}
		
		// 查找该持仓方向的止损单
		var stopLossOrders []map[string]interface{}
		for _, order := range orders {
			orderType, _ := order["type"].(string)
			positionSide, _ := order["positionSide"].(string)
			
			// 检查是否是止损单
			if orderType == "STOP_MARKET" || orderType == "STOP" {
				// 检查持仓方向是否匹配
				if (side == "long" && positionSide == "LONG") || 
				   (side == "short" && positionSide == "SHORT") {
					stopLossOrders = append(stopLossOrders, order)
					log.Printf("    🎯 找到止损单: %s %s", orderType, order["stopPrice"])
				}
			}
		}
		
		if len(stopLossOrders) == 0 {
			log.Printf("    ℹ️ %s %s 无止损挂单", symbol, side)
			continue
		}
		
		// 🔧 关键修复：检查是否有止损单已成交（通过比较上次记录的止损单）
		posKey := symbol + "_" + side
		if lastStopOrders, exists := at.lastKnownStopOrders[posKey]; exists {
			// 比较当前止损单与上次记录的止损单
			currentOrderIDs := make(map[int64]bool)
			for _, order := range stopLossOrders {
				if orderID, ok := order["orderId"]; ok {
					if idFloat, ok := orderID.(float64); ok {
						currentOrderIDs[int64(idFloat)] = true
					} else if idInt, ok := orderID.(int64); ok {
						currentOrderIDs[idInt] = true
					}
				}
			}
			
			// 检查哪些止损单消失了（可能已成交）
			for _, lastOrder := range lastStopOrders {
				lastOrderID := int64(0)
				if orderID, ok := lastOrder["orderId"]; ok {
					if idFloat, ok := orderID.(float64); ok {
						lastOrderID = int64(idFloat)
					} else if idInt, ok := orderID.(int64); ok {
						lastOrderID = idInt
					}
				}
				
				if lastOrderID > 0 && !currentOrderIDs[lastOrderID] {
					log.Printf("    🔍 检测到止损单消失: %s %s (订单ID: %d)", symbol, side, lastOrderID)
					
					// 🔧 智能判断：区分止损单更新和真正的止损成交
					// 1. 检查是否有同样币种和方向的新止损单（可能是价格调整）
					hasNewStopOrder := len(stopLossOrders) > 0
					var lastStopPrice, newStopPrice float64
					
					// 获取旧止损价格
					if stopPriceStr, ok := lastOrder["stopPrice"].(string); ok {
						lastStopPrice, _ = strconv.ParseFloat(stopPriceStr, 64)
					}
					
					// 获取新止损价格（如果存在）
					if hasNewStopOrder {
						if newStopPriceStr, ok := stopLossOrders[0]["stopPrice"].(string); ok {
							newStopPrice, _ = strconv.ParseFloat(newStopPriceStr, 64)
						}
					}
					
					// 如果存在新止损单且价格不同，说明是止损单更新而非成交
					if hasNewStopOrder && lastStopPrice > 0 && newStopPrice > 0 && 
					   math.Abs(lastStopPrice-newStopPrice)/lastStopPrice > 0.001 { // 价格变化超过0.1%
						log.Printf("    💡 检测到止损单价格更新: %s %s %.6f -> %.6f，跳过误报", 
							symbol, side, lastStopPrice, newStopPrice)
						continue // 跳过这个"消失"的订单，不当作成交处理
					}
					
					// 🔧 严格验证：只有持仓完全消失才认为是真正的止损成交
					// 获取当前持仓数量，如果持仓还存在，说明不是真正的止损成交
					currentPositions, posErr := at.trader.GetPositions()
					if posErr != nil {
						log.Printf("    ❌ 重新获取持仓失败: %v", posErr)
						continue
					}
					
					hasCurrentPosition := false
					var currentPosQuantity float64
					for _, pos := range currentPositions {
						if pos["symbol"] == symbol && pos["side"] == side {
							qty, _ := pos["positionAmt"].(float64)
							if qty != 0 { // 持仓数量不为0说明持仓还存在
								hasCurrentPosition = true
								currentPosQuantity = qty
								if currentPosQuantity < 0 {
									currentPosQuantity = -currentPosQuantity
								}
								break
							}
						}
					}
					
					// 🔧 改进的验证逻辑：检测持仓完全平仓或显著减少
					positionClosed := !hasCurrentPosition
					positionReduced := false
					var reductionAmount float64
					
					if hasCurrentPosition {
						// 计算持仓减少量（如果当前持仓小于之前记录的持仓）
						if currentPosQuantity < posQuantity {
							positionReduced = true
							reductionAmount = posQuantity - currentPosQuantity
							log.Printf("    📊 检测到持仓减少: %s %s 从%.6f减少到%.6f (减少%.6f)", 
								symbol, side, posQuantity, currentPosQuantity, reductionAmount)
						}
					}
					
					// 🔧 灵活的止损检测：持仓完全平仓 OR 持仓显著减少
					if positionClosed || positionReduced {
						if positionClosed {
							log.Printf("    ✅ 确认止损成交: %s %s 持仓已完全平仓", symbol, side)
						} else {
							log.Printf("    ✅ 确认部分止损成交: %s %s 持仓减少%.6f", symbol, side, reductionAmount)
						}
						
						// 获取止损单详细信息
						stopPrice := 0.0
						executedQuantity := posQuantity // 默认使用完整持仓数量
						if stopPriceStr, ok := lastOrder["stopPrice"].(string); ok {
							stopPrice, _ = strconv.ParseFloat(stopPriceStr, 64)
						}
						
						// 🔧 改进：对于部分平仓，使用减少的数量作为止损成交数量
						if positionReduced && !positionClosed {
							executedQuantity = reductionAmount
						}
						
						// 验证数据有效性：止损价格和数量都必须合理
						if executedQuantity > 0 && stopPrice > 0 {
							log.Printf("    ✅ 记录止损成交: %s %s 成交数量=%.6f 止损价=%.6f", 
								symbol, side, executedQuantity, stopPrice)
							
							pendingOrder := &PendingStopOrder{
								Symbol:         symbol,
								Side:           side,
								OrderID:        lastOrderID,
								StopPrice:      stopPrice,
								Quantity:       executedQuantity, // 使用实际成交数量
								CreateTime:     time.Now(),
								OriginalAction: "stop_loss_detected",
							}
							
							at.recordStopLossExecution(pendingOrder, record)
						} else {
							log.Printf("    ❌ 验证失败，跳过记录: executedQuantity=%.6f, stopPrice=%.6f", 
								executedQuantity, stopPrice)
						}
					} else {
						log.Printf("    ⚠️ 止损单消失但持仓无变化(%.6f)，可能是止损更新而非成交，跳过记录", 
							currentPosQuantity)
					}
				}
			}
		}
		
		// 更新该持仓的止损单记录
		at.lastKnownStopOrders[posKey] = stopLossOrders
	}
	
	// 清理已平仓持仓的止损单记录
	currentPositionKeys := make(map[string]bool)
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		posKey := symbol + "_" + side
		currentPositionKeys[posKey] = true
	}
	
	for posKey := range at.lastKnownStopOrders {
		if !currentPositionKeys[posKey] {
			log.Printf("  🧹 清理已平仓的止损记录: %s", posKey)
			delete(at.lastKnownStopOrders, posKey)
		}
	}
	
	return nil
}

// recordStopLossExecution 记录止损单成交
func (at *AutoTrader) recordStopLossExecution(pendingOrder *PendingStopOrder, record *logger.DecisionRecord) {
	// 🔧 数据验证：确保pendingOrder数据有效
	if pendingOrder == nil {
		log.Printf("❌ [止损记录] pendingOrder为nil，无法记录止损成交")
		return
	}
	
	if pendingOrder.Symbol == "" {
		log.Printf("❌ [止损记录] pendingOrder.Symbol为空，无法记录止损成交")
		return
	}
	
	if pendingOrder.StopPrice <= 0 {
		log.Printf("❌ [止损记录] pendingOrder.StopPrice无效(%.6f)，无法记录止损成交", pendingOrder.StopPrice)
		return
	}
	
	if pendingOrder.Quantity <= 0 {
		log.Printf("❌ [止损记录] pendingOrder.Quantity无效(%.6f)，无法记录止损成交", pendingOrder.Quantity)
		return
	}
	
	log.Printf("📋 [止损记录] 开始记录止损成交: %s %s 订单ID=%d 数量=%.6f 止损价=%.6f", 
		pendingOrder.Symbol, pendingOrder.Side, pendingOrder.OrderID, 
		pendingOrder.Quantity, pendingOrder.StopPrice)
	
	// 🔧 增强修复：使用ExchangeRecordSync获取更精确的成交价格
	var executionPrice float64
	var pnlCalculationMethod string
	
	if at.exchangeSync != nil {
		log.Printf("🔍 [止损记录] 使用ExchangeRecordSync获取增强成交信息...")
		fillDetails, err := at.exchangeSync.GetEnhancedFillPrice(pendingOrder.OrderID, pendingOrder.Symbol, pendingOrder.Side)
		if err != nil {
			log.Printf("⚠️ [止损记录] 增强成交价格获取失败，降级使用止损价格: %v", err)
			executionPrice = pendingOrder.StopPrice
			pnlCalculationMethod = "fallback_stop_price"
		} else if fillDetails != nil && fillDetails.AveragePrice > 0 {
			executionPrice = fillDetails.AveragePrice
			pnlCalculationMethod = "enhanced_exchange_fill"
			log.Printf("✅ [止损记录] 获取到增强成交信息: 价格=%.6f, 数量=%.6f, 状态=%s", 
				fillDetails.AveragePrice, fillDetails.ExecutedQty, fillDetails.Status)
		} else {
			log.Printf("⚠️ [止损记录] 增强成交信息无效，降级使用止损价格")
			executionPrice = pendingOrder.StopPrice
			pnlCalculationMethod = "fallback_invalid_fill"
		}
	} else {
		// 🔧 降级：使用止损价格作为成交价（原有逻辑）
		log.Printf("⚠️ [止损记录] ExchangeRecordSync未初始化，使用止损价格")
		executionPrice = pendingOrder.StopPrice
		pnlCalculationMethod = "fallback_no_sync"
	}
	
	// 获取当前市场价格用于验证和日志记录
	marketData, err := market.Get(pendingOrder.Symbol)
	if err != nil {
		log.Printf("⚠️ [止损记录] 获取 %s 市场价格失败: %v", pendingOrder.Symbol, err)
		log.Printf("📊 [止损记录] 继续使用止损价格(%.6f)作为成交价格", executionPrice)
		// 继续使用止损价格，不因市场价格获取失败而中断
	} else {
		// 记录市场价格与止损价格的差异（用于调试）
		priceDiff := math.Abs(marketData.CurrentPrice - pendingOrder.StopPrice)
		priceDiffPct := (priceDiff / pendingOrder.StopPrice) * 100
		log.Printf("🔍 [止损记录] 价格验证: 市场价=%.6f, 止损价=%.6f, 差异=%.6f (%.2f%%)", 
			marketData.CurrentPrice, pendingOrder.StopPrice, priceDiff, priceDiffPct)
		
		// 如果价格差异过大，可能是数据异常，记录警告
		if priceDiffPct > 5.0 {
			log.Printf("⚠️ [止损记录] 警告：市场价格与止损价格差异较大(%.2f%%)，可能存在数据异常", priceDiffPct)
		}
	}
	
	// 🔧 关键修复：更新数据库中的交易记录状态
	if at.database != nil {
		log.Printf("📋 [止损记录] 开始更新数据库交易记录...")
		log.Printf("    trader_id: '%s'", at.id)
		log.Printf("    symbol: '%s'", pendingOrder.Symbol)
		log.Printf("    side: '%s'", pendingOrder.Side)
		log.Printf("    executionPrice: %.6f", executionPrice)
		log.Printf("    orderID: '%d'", pendingOrder.OrderID)
		log.Printf("    closeReason: 'stop_loss'")
		
		// 🔧 增强错误处理：检查是否存在对应的开仓记录
		openTrade, checkErr := at.database.GetOpenTrade(at.id, pendingOrder.Symbol, pendingOrder.Side)
		if checkErr != nil {
			log.Printf("❌ [止损记录] 未找到对应的开仓记录: %v", checkErr)
			log.Printf("📋 [止损记录] 查找参数: trader_id='%s', symbol='%s', side='%s'", 
				at.id, pendingOrder.Symbol, pendingOrder.Side)
			
			// 记录到trade_actions表，即使没有对应的开仓记录
			log.Printf("📝 [止损记录] 将止损成交记录到trade_actions表作为独立记录")
			at.recordStopLossAsTradeAction(pendingOrder, executionPrice)
		} else {
			log.Printf("✅ [止损记录] 找到对应开仓记录: ID=%s, 开仓价=%.6f, 保证金=%.2f", 
				openTrade.ID, openTrade.OpenPrice, openTrade.MarginUsed)
			
			// 正常更新交易记录
			log.Printf("🔄 [止损记录] 正在更新数据库中的交易记录状态...")
			at.updateTradeInDatabase(pendingOrder.Symbol, pendingOrder.Side, 
				fmt.Sprintf("%d", pendingOrder.OrderID), "stop_loss")
		}
	} else {
		log.Printf("⚠️ [止损记录] 数据库连接不可用，跳过数据库更新")
	}
	
	// 🔧 增强盈亏计算：提供更准确的盈亏估算和详细日志
	var pnl float64
	var finalPnlMethod string
	
	// 尝试从数据库获取开仓价格进行精确计算
	if at.database != nil {
		openTrade, err := at.database.GetOpenTrade(at.id, pendingOrder.Symbol, pendingOrder.Side)
		if err == nil && openTrade != nil {
			// 使用实际开仓价格计算精确盈亏
			if pendingOrder.Side == "long" {
				pnl = (executionPrice - openTrade.OpenPrice) * pendingOrder.Quantity
			} else {
				pnl = (openTrade.OpenPrice - executionPrice) * pendingOrder.Quantity
			}
			finalPnlMethod = fmt.Sprintf("precise_with_open_price+%s", pnlCalculationMethod)
			log.Printf("💰 [止损记录] 精确盈亏计算: 开仓价=%.6f, 止损价=%.6f, 盈亏=%.2f USDT", 
				openTrade.OpenPrice, executionPrice, pnl)
		} else {
			// 退化到简化计算（使用止损价格作为基准）
			if pendingOrder.Side == "long" {
				// 多仓止损：通常是亏损
				pnl = (executionPrice - pendingOrder.StopPrice) * pendingOrder.Quantity
			} else {
				// 空仓止损：通常是亏损
				pnl = (pendingOrder.StopPrice - executionPrice) * pendingOrder.Quantity
			}
			finalPnlMethod = fmt.Sprintf("simplified_estimation+%s", pnlCalculationMethod)
			log.Printf("💰 [止损记录] 简化盈亏估算: 止损价=%.6f, 估算盈亏=%.2f USDT (方法: %s)", 
				pendingOrder.StopPrice, pnl, finalPnlMethod)
		}
	} else {
		// 无数据库连接，使用简化计算
		if pendingOrder.Side == "long" {
			pnl = (executionPrice - pendingOrder.StopPrice) * pendingOrder.Quantity
		} else {
			pnl = (pendingOrder.StopPrice - executionPrice) * pendingOrder.Quantity
		}
		finalPnlMethod = fmt.Sprintf("no_database_estimation+%s", pnlCalculationMethod)
		log.Printf("💰 [止损记录] 无数据库盈亏估算: 盈亏=%.2f USDT", pnl)
	}
	
	log.Printf("📊 [止损记录] 止损成交汇总: %s %s 数量=%.6f 价格=%.6f 盈亏=%.2f USDT (计算方法: %s)", 
		pendingOrder.Symbol, pendingOrder.Side, pendingOrder.Quantity, executionPrice, pnl, finalPnlMethod)
	
	// 🔧 增强交易记录创建：添加更多元数据和验证
	log.Printf("📝 [止损记录] 创建交易动作记录...")
	
	actionRecord := &logger.DecisionAction{
		Symbol:    pendingOrder.Symbol,
		Action:    fmt.Sprintf("stop_loss_%s", pendingOrder.Side), // stop_loss_long 或 stop_loss_short
		Quantity:  pendingOrder.Quantity,
		Price:     executionPrice,
		OrderID:   pendingOrder.OrderID,
		Success:   true,
		Timestamp: time.Now(),
		Error:     "",
		// Leverage: 无法从PendingStopOrder获取，使用默认值或从数据库查询
	}
	
	// 记录详细的成交信息到错误字段（用于前端显示额外信息）
	additionalInfo := fmt.Sprintf("原始动作=%s, 创建时间=%s, PnL=%.2f", 
		pendingOrder.OriginalAction, pendingOrder.CreateTime.Format("15:04:05"), pnl)
	log.Printf("📋 [止损记录] 附加信息: %s", additionalInfo)
	
	// 🔧 增强决策记录更新：提供更详细的记录和错误处理
	if record == nil {
		log.Printf("⚠️ [止损记录] record为nil，无法更新决策记录")
	} else {
		log.Printf("📋 [止损记录] 正在更新决策记录...")
		
		// 查找该币种的现有决策记录并添加止损成交信息
		found := false
		if record.Decisions != nil {
			for i := range record.Decisions {
				if record.Decisions[i].Symbol == pendingOrder.Symbol {
					log.Printf("📍 [止损记录] 找到对应币种(%s)的现有决策记录，添加止损信息", pendingOrder.Symbol)
					
					// 使用增强的格式，包含更多信息
					stopLossInfo := fmt.Sprintf("💥 止损成交 %.6f@%.6f (PnL: %.2f USDT, 方法: %s)", 
						pendingOrder.Quantity, executionPrice, pnl, pnlCalculationMethod)
					
					if record.Decisions[i].Error == "" {
						record.Decisions[i].Error = stopLossInfo
					} else {
						record.Decisions[i].Error += " | " + stopLossInfo
					}
					found = true
					log.Printf("✅ [止损记录] 已更新现有决策记录的Error字段")
					break
				}
			}
		}
		
		// 如果没有找到对应币种的决策记录，创建独立的止损记录
		if !found {
			log.Printf("📝 [止损记录] 未找到对应币种的现有决策记录，创建独立止损记录")
			
			if record.Decisions == nil {
				record.Decisions = []logger.DecisionAction{}
				log.Printf("📋 [止损记录] 初始化Decisions数组")
			}
			record.Decisions = append(record.Decisions, *actionRecord)
			log.Printf("✅ [止损记录] 已添加独立的止损成交记录")
		} else {
			log.Printf("📋 [止损记录] 止损信息已合并到现有决策记录中")
		}
	}
	
	// 🔧 增强后端日志记录：提供完整的成交摘要
	log.Printf("💾 [止损记录] 止损成交记录完成:")
	log.Printf("    币种: %s %s", pendingOrder.Symbol, strings.ToUpper(pendingOrder.Side))
	log.Printf("    订单ID: %d", pendingOrder.OrderID)
	log.Printf("    数量: %.6f", pendingOrder.Quantity)
	log.Printf("    成交价格: %.6f", executionPrice)
	log.Printf("    盈亏: %.2f USDT (%s)", pnl, pnlCalculationMethod)
	log.Printf("    原始动作: %s", pendingOrder.OriginalAction)
	log.Printf("    创建时间: %s", pendingOrder.CreateTime.Format("2006-01-02 15:04:05"))
	
	// 🔧 关键修复：创建止损成交的决策记录，确保decision_records表数据完整性
	at.createStopLossDecisionRecord(pendingOrder, executionPrice, pnl, finalPnlMethod)
	
	log.Printf("🎯 [止损记录] =================================")
}

// recordStopLossAsTradeAction 将止损成交记录为独立的交易动作（当无法找到对应开仓记录时）
func (at *AutoTrader) recordStopLossAsTradeAction(pendingOrder *PendingStopOrder, executionPrice float64) {
	if at.database == nil {
		log.Printf("⚠️ [止损记录] 数据库连接不可用，跳过独立止损记录")
		return
	}
	
	log.Printf("📝 [止损记录] 创建独立的止损成交记录...")
	
	// 创建独立的交易动作记录
	actionRecord := &config.TradeActionRecord{
		TraderID:     at.id,
		Action:       fmt.Sprintf("stop_loss_%s_detected", pendingOrder.Side),
		Symbol:       pendingOrder.Symbol,
		Quantity:     pendingOrder.Quantity,
		Price:        executionPrice,
		Leverage:     0, // 无法获取杠杆信息
		OrderID:      fmt.Sprintf("%d", pendingOrder.OrderID),
		Timestamp:    time.Now(),
		Success:      true,
		ErrorMessage: fmt.Sprintf("独立止损记录 - 原始动作: %s, 创建时间: %s", 
			pendingOrder.OriginalAction, pendingOrder.CreateTime.Format("15:04:05")),
	}
	
	if err := at.database.CreateTradeAction(actionRecord); err != nil {
		log.Printf("❌ [止损记录] 创建独立止损记录失败: %v", err)
	} else {
		log.Printf("✅ [止损记录] 成功创建独立止损记录: ID=%s", actionRecord.ID)
	}
}

// recordTradeToDatabase 将交易记录到数据库
func (at *AutoTrader) recordTradeToDatabase(symbol, side string, quantity float64, leverage int,
	price float64, orderID string, action string, isOpen bool, initialStopPrice float64) {
	if at.database == nil {
		return
	}

	if isOpen {
		// 开仓记录
		tradeRecord := &config.TradeRecord{
			TraderID:         at.id,
			Symbol:           symbol,
			Side:             side,
			Quantity:         quantity,
			Leverage:         leverage,
			OpenPrice:        price,
			PositionValue:    quantity * price,
			MarginUsed:       (quantity * price) / float64(leverage),
			OpenTime:         time.Now(),
			Status:           "open",
			OpenOrderID:      orderID,
			InitialStopPrice: initialStopPrice,  // 🔒 记录初始止损价（用于锁盈系统计算R0）
			CurrentStopPrice: initialStopPrice,  // 🔒 初始止损价同时作为当前止损价
		}

		if err := at.database.CreateTrade(tradeRecord); err != nil {
			log.Printf("  ⚠️ 记录开仓到数据库失败: %v", err)
		} else {
			log.Printf("  💾 已记录开仓到数据库: %s (InitialStop: %.6f)", tradeRecord.ID, initialStopPrice)
		}
	}

	// 交易动作记录
	actionRecord := &config.TradeActionRecord{
		TraderID:  at.id,
		Action:    action,
		Symbol:    symbol,
		Quantity:  quantity,
		Price:     price,
		Leverage:  leverage,
		OrderID:   orderID,
		Timestamp: time.Now(),
		Success:   true,
	}
	
	if err := at.database.CreateTradeAction(actionRecord); err != nil {
		log.Printf("  ⚠️ 记录交易动作到数据库失败: %v", err)
	}
}

// updateTradeInDatabase 更新数据库中的交易记录（平仓时使用）
func (at *AutoTrader) updateTradeInDatabase(symbol, side string, closeOrderID, closeReason string) {
	if at.database == nil {
		log.Printf("⚠️ [数据库更新] 数据库连接不可用，跳过交易记录更新")
		return
	}

	// 🎯 修复 P0：分离 DB side 与交易所 positionSide
	dbSide := NormalizeInternalSide(side)
	posSide := NormalizePositionSide(side)

	log.Printf("🔄 [数据库更新] 开始更新交易记录: %s %s (dbSide=%s,posSide=%s)", symbol, side, dbSide, posSide)
	
	// 查找对应的开仓记录 - 使用 dbSide
	openTrade, err := at.database.GetOpenTrade(at.id, symbol, dbSide)
	if err != nil {
		log.Printf("❌ [数据库更新] [严重错误] 无法找到开仓记录: %v", err)
		log.Printf("📋 [数据库更新] 查找参数: trader_id='%s', symbol='%s', side='%s'", at.id, symbol, side)
		
		// 🔧 增强错误处理：尝试查找最近的相关记录
		if recentTrades, err := at.database.GetTraderTrades(at.id, 10); err == nil {
			log.Printf("📊 [数据库更新] 最近10笔交易记录:")
			for i, trade := range recentTrades {
				if trade.Symbol == symbol {
					log.Printf("  [%d] %s %s %s 开仓价=%.6f 状态=%s", 
						i+1, trade.Symbol, trade.Side, trade.Status, trade.OpenPrice, trade.Status)
				}
			}
		}
		
		// 记录为独立的平仓动作，但不写入估算的close_price
		log.Printf("📝 [数据库更新] 创建独立的平仓动作记录...")
		actionRecord := &config.TradeActionRecord{
			TraderID:     at.id,
			Action:       fmt.Sprintf("close_%s_orphaned", side),
			Symbol:       symbol,
			Quantity:     0, // 无法获取数量
			Price:        0, // 🔥 拒绝写入估算价格
			OrderID:      closeOrderID,
			Timestamp:    time.Now(),
			Success:      false, // 标记为失败，因为没有权威数据
			ErrorMessage: fmt.Sprintf("孤立平仓 - 未找到开仓记录，无权威数据: %s", closeReason),
		}
		
		if err := at.database.CreateTradeAction(actionRecord); err != nil {
			log.Printf("❌ [数据库更新] 创建孤立平仓记录失败: %v", err)
		} else {
			log.Printf("✅ [数据库更新] 成功创建孤立平仓记录（无估算数据）")
		}
		return
	}

	log.Printf("✅ [数据库更新] 找到对应开仓记录:")
	log.Printf("    交易ID: %s", openTrade.ID)
	log.Printf("    开仓价格: %.6f", openTrade.OpenPrice)
	log.Printf("    持仓数量: %.6f", openTrade.Quantity)
	log.Printf("    保证金: %.2f USDT", openTrade.MarginUsed)
	log.Printf("    开仓时间: %s", openTrade.OpenTime.Format("2006-01-02 15:04:05"))

	// 🎯 关键修改：必须从交易所获取权威数据，拒绝估算
	log.Printf("🔍 [权威数据] 正在从交易所获取真实平仓数据...")
	
	// 确保 trader 实现了新的接口方法
	binanceTrader, ok := at.trader.(*FuturesTrader)
	if !ok {
		log.Printf("❌ [权威数据] 交易器不支持权威数据查询接口")
		return
	}
	
	// 🔧 增强：增加重试机制，最多重试3次，每次间隔2秒
	var authData *AuthoritativeCloseData
	maxRetries := 3
	var retryErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			log.Printf("🔄 [权威数据] 第 %d/%d 次重试...", attempt, maxRetries)
			time.Sleep(2 * time.Second)
		}

		// 获取权威的平仓数据 - 使用 posSide
		authData, retryErr = binanceTrader.GetAuthoritativeCloseData(symbol, posSide, closeOrderID)
		if retryErr == nil {
			log.Printf("✅ [权威数据] 第 %d 次尝试成功", attempt)
			break
		}

		log.Printf("⚠️ [权威数据] 第 %d 次尝试失败: %v", attempt, retryErr)
	}

	if retryErr != nil {
		log.Printf("❌ [权威数据] 所有重试均失败，无法获取交易所权威数据")
		log.Printf("🚫 [权威数据] 拒绝使用估算数据写入数据库")
		
		// 记录失败但不写入错误的平仓数据
		actionRecord := &config.TradeActionRecord{
			TraderID:     at.id,
			Action:       fmt.Sprintf("close_%s_failed", side),
			Symbol:       symbol,
			Quantity:     openTrade.Quantity,
			Price:        0, // 拒绝估算价格
			OrderID:      closeOrderID,
			Timestamp:    time.Now(),
			Success:      false,
			ErrorMessage: fmt.Sprintf("无法获取权威平仓数据: %v", retryErr),
		}
		
		at.database.CreateTradeAction(actionRecord)
		return
	}
	
	log.Printf("✅ [权威数据] 获取成功:")
	log.Printf("    真实平仓价格: %.6f", authData.ActualPrice)
	log.Printf("    真实平仓时间: %s", authData.ActualTime.Format("15:04:05"))
	log.Printf("    真实已实现盈亏: %.2f USDT", authData.ActualPnL)
	log.Printf("    数据来源: %s", authData.DataSource)
	
	// 使用权威数据计算最终盈亏
	var finalPnL float64
	var finalPnLPct float64
	var finalPrice float64
	var finalTime time.Time
	
	// 优先使用交易所返回的已实现盈亏
	if authData.ActualPnL != 0 && authData.DataSource != "INCOME_HISTORY" {
		finalPnL = authData.ActualPnL
		finalPrice = authData.ActualPrice
		finalTime = authData.ActualTime
		log.Printf("💰 [权威数据] 使用交易所返回的已实现盈亏: %.2f USDT", finalPnL)
	} else if authData.ActualPrice > 0 {
		// 如果没有直接的已实现盈亏，使用真实成交价计算
		if side == "long" {
			finalPnL = openTrade.Quantity * (authData.ActualPrice - openTrade.OpenPrice)
		} else {
			finalPnL = openTrade.Quantity * (openTrade.OpenPrice - authData.ActualPrice)
		}
		finalPrice = authData.ActualPrice
		finalTime = authData.ActualTime
		log.Printf("💰 [权威数据] 使用真实成交价计算盈亏: %.2f USDT", finalPnL)
	} else {
		// 如果只有Income数据（DataSource == "INCOME_HISTORY"）
		finalPnL = authData.ActualPnL
		finalPrice = 0 // Income数据中没有价格信息
		finalTime = authData.ActualTime
		log.Printf("💰 [权威数据] 使用资金流水数据: %.2f USDT (无成交价格)", finalPnL)
	}
	
	// 计算盈亏百分比
	if openTrade.MarginUsed > 0 {
		finalPnLPct = (finalPnL / openTrade.MarginUsed) * 100
	}
	
	durationSecs := int(finalTime.Sub(openTrade.OpenTime).Seconds())
	
	log.Printf("💰 [权威数据] 最终盈亏计算结果:")
	log.Printf("    盈亏金额: %.2f USDT (权威)", finalPnL)
	log.Printf("    盈亏百分比: %.2f%%", finalPnLPct)
	log.Printf("    持续时间: %d秒 (%.1f分钟)", durationSecs, float64(durationSecs)/60)

	// 🎯 修复 P0：有权威盈亏就闭合，价格可以为0表示未知
	closePrice := finalPrice  // 可能为 0，表示未知成交价而非估算
	log.Printf("🔄 [权威数据] 正在更新数据库: tradeID=%s, 价格=%.6f, 权威盈亏=%.2f",
		openTrade.ID, closePrice, finalPnL)

	// 🔧 增强：验证价格合理性
	if err := at.validateTradePrice(symbol, side, openTrade.OpenPrice, closePrice, authData.DataSource); err != nil {
		log.Printf("⚠️ [价格验证] 警告: %v", err)
		// 注意：这里只是警告，不阻止数据库更新，因为我们使用的是权威数据
	}

	if err := at.database.UpdateTrade(openTrade.ID, closePrice, finalTime, 
		"closed", closeReason, closeOrderID, finalPnL, finalPnLPct, durationSecs); err != nil {
		log.Printf("❌ [数据库更新] [严重错误] 数据库更新失败: %v", err)
		
		// 失败时才记录 action
		actionRecord := &config.TradeActionRecord{
			TraderID:     at.id,
			Action:       fmt.Sprintf("close_%s_failed", dbSide),
			Symbol:       symbol,
			Quantity:     openTrade.Quantity,
			Price:        closePrice,
			OrderID:      closeOrderID,
			Timestamp:    time.Now(),
			Success:      false,
			ErrorMessage: fmt.Sprintf("数据库更新失败: %v", err),
		}
		at.database.CreateTradeAction(actionRecord)
	} else {
		if closePrice > 0 {
			log.Printf("✅ [数据库更新] 成功闭合 trades（真实成交价: %.6f）", closePrice)
		} else {
			log.Printf("✅ [数据库更新] 成功闭合 trades（权威PnL: %.2f USDT，无成交价）", finalPnL)
		}
		log.Printf("    状态: open → closed")
		log.Printf("    数据来源: %s", authData.DataSource)
	}
}

// saveToDatabaseRecord 将决策记录保存到数据库
func (at *AutoTrader) saveToDatabaseRecord(record *logger.DecisionRecord) error {
	// 序列化各种JSON字段
	accountStateJSON, _ := json.Marshal(record.AccountState)
	positionsJSON, _ := json.Marshal(record.Positions)
	candidateCoinsJSON, _ := json.Marshal(record.CandidateCoins)
	executionLogJSON, _ := json.Marshal(record.ExecutionLog)
	
	// 创建数据库记录
	dbRecord := &config.DecisionRecordDB{
		TraderID:           at.id,
		CycleNumber:        record.CycleNumber,
		Timestamp:          record.Timestamp,
		SystemPrompt:       record.SystemPrompt,
		InputPrompt:        record.InputPrompt,
		CoTTrace:           record.CoTTrace,
		DecisionJSON:       record.DecisionJSON, // 使用现有的DecisionJSON
		AccountStateJSON:   string(accountStateJSON),
		PositionsJSON:      string(positionsJSON),
		CandidateCoinsJSON: string(candidateCoinsJSON),
		ExecutionLogJSON:   string(executionLogJSON),
		Success:            record.Success,
		ErrorMessage:       record.ErrorMessage,
	}
	
	return at.database.CreateDecisionRecord(dbRecord)
}

// performPeriodicStopLossAudit 执行定期止损单全量审计
// 每10个周期执行一次，用于检测可能被遗漏的止损单成交
func (at *AutoTrader) performPeriodicStopLossAudit(record *logger.DecisionRecord) error {
	log.Printf("🔍 [定期审计] 开始执行止损单全量检查 (周期 #%d)", at.callCount)
	
	// 1. 获取数据库中所有状态为'open'的交易记录
	if at.database == nil {
		log.Printf("⚠️ [定期审计] 数据库连接不可用，跳过审计")
		return nil
	}
	
	openTrades, err := at.getOpenTradesFromDatabase()
	if err != nil {
		return fmt.Errorf("获取开仓交易记录失败: %w", err)
	}
	
	if len(openTrades) == 0 {
		log.Printf("📋 [定期审计] 无开仓交易记录，审计完成")
		return nil
	}
	
	log.Printf("📋 [定期审计] 发现 %d 个开仓交易记录，开始验证", len(openTrades))
	
	// 2. 获取当前实际持仓
	currentPositions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取当前持仓失败: %w", err)
	}
	
	// 3. 建立持仓映射，便于快速查找
	positionMap := make(map[string]float64) // key: symbol_side, value: quantity
	for _, pos := range currentPositions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity // 空仓数量为负，转为正数
		}
		key := fmt.Sprintf("%s_%s", symbol, side)
		positionMap[key] = quantity
	}
	
	// 4. 检查每个数据库中的开仓记录
	var discrepancyCount int
	for _, trade := range openTrades {
		posKey := fmt.Sprintf("%s_%s", trade.Symbol, trade.Side)
		currentQuantity, hasPosition := positionMap[posKey]
		
		// 如果数据库显示有开仓但实际没有持仓，或数量不匹配
		if !hasPosition {
			log.Printf("🔍 [定期审计] 发现差异: %s %s 数据库显示开仓(%.6f)但实际无持仓", 
				trade.Symbol, trade.Side, trade.Quantity)
			discrepancyCount++
			
			// 尝试确定平仓原因和价格
			estimatedClosePrice, closeReason := at.estimateCloseDetails(trade.Symbol, trade.OpenPrice, trade.Side)
			
			// 同步更新数据库状态
			log.Printf("🔄 [定期审计] 同步关闭数据库记录: %s %s 估算平仓价=%.6f", 
				trade.Symbol, trade.Side, estimatedClosePrice)
			
			at.updateTradeInDatabase(trade.Symbol, trade.Side,  
				"AUDIT_CLOSE", fmt.Sprintf("periodic_audit_%s", closeReason))
				
		} else if math.Abs(currentQuantity-trade.Quantity) > 0.0001 { // 允许小数精度误差
			log.Printf("🔍 [定期审计] 发现数量差异: %s %s 数据库(%.6f) vs 实际(%.6f)", 
				trade.Symbol, trade.Side, trade.Quantity, currentQuantity)
			discrepancyCount++
			
			// 如果实际持仓小于数据库记录，可能是部分平仓
			if currentQuantity < trade.Quantity {
				partialCloseQuantity := trade.Quantity - currentQuantity
				log.Printf("📊 [定期审计] 检测到部分平仓: %s %s 平仓数量=%.6f", 
					trade.Symbol, trade.Side, partialCloseQuantity)
				
				// 记录部分平仓（作为止损单成交处理）
				estimatedClosePrice, _ := at.estimateCloseDetails(trade.Symbol, trade.OpenPrice, trade.Side)
				pendingOrder := &PendingStopOrder{
					Symbol:         trade.Symbol,
					Side:           trade.Side,
					OrderID:        0, // 审计发现的，没有具体订单ID
					StopPrice:      estimatedClosePrice,
					Quantity:       partialCloseQuantity,
					CreateTime:     time.Now(),
					OriginalAction: "audit_detected_partial_close",
				}
				
				at.recordStopLossExecution(pendingOrder, record)
			}
		}
	}
	
	if discrepancyCount > 0 {
		log.Printf("⚠️ [定期审计] 发现 %d 个数据差异，已进行同步修复", discrepancyCount)
	} else {
		log.Printf("✅ [定期审计] 数据库与实际持仓一致，无需修复")
	}
	
	// 5. 检查订单历史中的止损止盈成交
	if err := at.checkOrderHistoryForMissedExecutions(record); err != nil {
		log.Printf("⚠️ [定期审计] 检查订单历史失败: %v", err)
	}
	
	return nil
}

// getOpenTradesFromDatabase 从数据库获取所有开仓状态的交易记录
func (at *AutoTrader) getOpenTradesFromDatabase() ([]*config.TradeRecord, error) {
	if at.database == nil {
		return nil, fmt.Errorf("数据库连接不可用")
	}
	
	// 使用数据库的GetTraderTrades方法，然后过滤出开仓状态的记录
	allTrades, err := at.database.GetTraderTrades(at.id, 0) // 0表示获取所有记录
	if err != nil {
		return nil, err
	}
	
	var openTrades []*config.TradeRecord
	for _, trade := range allTrades {
		if trade.Status == "open" {
			openTrades = append(openTrades, trade)
		}
	}
	
	return openTrades, nil
}

// validateTradePrice 验证交易价格的合理性
// 用于检测数据异常，防止记录错误的价格导致盈亏统计失真
func (at *AutoTrader) validateTradePrice(symbol, side string, openPrice, closePrice float64, dataSource string) error {
	// 如果平仓价格为0（从Income获取的情况），跳过验证
	if closePrice == 0 {
		log.Printf("⚠️ [价格验证] %s %s 平仓价格为0（来源: %s），跳过验证", symbol, side, dataSource)
		return nil
	}

	// 如果开仓价格为0，这是异常情况
	if openPrice == 0 {
		return fmt.Errorf("开仓价格为0，数据异常")
	}

	// 计算价格差异百分比
	var priceDiffPct float64
	if side == "long" {
		priceDiffPct = ((closePrice - openPrice) / openPrice) * 100
	} else { // short
		priceDiffPct = ((openPrice - closePrice) / openPrice) * 100
	}

	absDiffPct := priceDiffPct
	if absDiffPct < 0 {
		absDiffPct = -absDiffPct
	}

	// 验证1：价格差异不应超过50%（极端情况）
	if absDiffPct > 50 {
		log.Printf("🚨 [价格验证] %s %s 价格差异异常: 开仓=%.6f, 平仓=%.6f, 差异=%.1f%%",
			symbol, side, openPrice, closePrice, priceDiffPct)
		log.Printf("    数据来源: %s", dataSource)
		return fmt.Errorf("价格差异超过50%%，可能存在数据异常")
	}

	// 验证2：警告级别 - 价格差异超过30%
	if absDiffPct > 30 {
		log.Printf("⚠️ [价格验证] %s %s 价格差异较大: 开仓=%.6f, 平仓=%.6f, 差异=%.1f%%",
			symbol, side, openPrice, closePrice, priceDiffPct)
		log.Printf("    数据来源: %s (可能是正常的止损或爆仓)", dataSource)
	}

	// 验证3：记录价格比较信息
	if absDiffPct > 10 {
		log.Printf("📊 [价格验证] %s %s 价格变化: %.1f%% (开仓=%.6f, 平仓=%.6f, 来源=%s)",
			symbol, side, priceDiffPct, openPrice, closePrice, dataSource)
	}

	return nil
}

// estimateCloseDetails 估算平仓价格和原因
func (at *AutoTrader) estimateCloseDetails(symbol string, openPrice float64, side string) (float64, string) {
	// 获取当前市场价格作为估算平仓价格
	marketPrice, err := at.trader.GetMarketPrice(symbol)
	if err != nil {
		log.Printf("⚠️ [定期审计] 获取 %s 市场价格失败: %v，使用开仓价格", symbol, err)
		return openPrice, "unknown_market_price_unavailable"
	}
	
	// 简单的平仓原因推断
	var pnlPct float64
	if side == "long" {
		pnlPct = ((marketPrice - openPrice) / openPrice) * 100
	} else {
		pnlPct = ((openPrice - marketPrice) / openPrice) * 100
	}
	
	closeReason := "unknown_external"
	if pnlPct < -5 { // 亏损超过5%，可能是止损
		closeReason = "likely_stop_loss"
	} else if pnlPct > 10 { // 盈利超过10%，可能是止盈
		closeReason = "likely_take_profit"
	} else {
		closeReason = "likely_manual_close"
	}
	
	return marketPrice, closeReason
}

// checkOrderHistoryForMissedExecutions 通过检查订单历史来发现遗漏的止损止盈成交
func (at *AutoTrader) checkOrderHistoryForMissedExecutions(record *logger.DecisionRecord) error {
	log.Printf("🔍 [订单历史检查] 开始检查近期订单历史中的遗漏成交...")
	
	// 获取当前所有持仓的币种
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓列表失败: %w", err)
	}
	
	// 收集需要检查的币种（当前持仓 + 最近交易的币种）
	symbolsToCheck := make(map[string]bool)
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		symbolsToCheck[symbol] = true
	}
	
	// 从数据库获取最近交易的币种
	if at.database != nil {
		recentTrades, err := at.database.GetTraderTrades(at.id, 20) // 获取最近20笔交易
		if err == nil {
			for _, trade := range recentTrades {
				symbolsToCheck[trade.Symbol] = true
			}
		}
	}
	
	if len(symbolsToCheck) == 0 {
		log.Printf("📋 [订单历史检查] 无需检查的币种，跳过")
		return nil
	}
	
	log.Printf("📋 [订单历史检查] 将检查 %d 个币种的订单历史", len(symbolsToCheck))
	
	var totalMissedExecutions int
	
	// 检查每个币种的订单历史
	for symbol := range symbolsToCheck {
		missedCount, err := at.checkSymbolOrderHistory(symbol, record)
		if err != nil {
			log.Printf("⚠️ [订单历史检查] 检查 %s 失败: %v", symbol, err)
			continue
		}
		totalMissedExecutions += missedCount
		
		// 避免API频率限制，添加短暂延迟
		time.Sleep(200 * time.Millisecond)
	}
	
	if totalMissedExecutions > 0 {
		log.Printf("📊 [订单历史检查] 发现并修复了 %d 个遗漏的止损止盈成交", totalMissedExecutions)
	} else {
		log.Printf("✅ [订单历史检��] 未发现遗漏的成交记录")
	}
	
	return nil
}

// checkSymbolOrderHistory 检查单个币种的订单历史
func (at *AutoTrader) checkSymbolOrderHistory(symbol string, record *logger.DecisionRecord) (int, error) {
	// 获取最近50个订单历史
	orderHistory, err := at.trader.GetOrderHistory(symbol, 50)
	if err != nil {
		return 0, fmt.Errorf("获取 %s 订单历史失败: %w", symbol, err)
	}
	
	if len(orderHistory) == 0 {
		return 0, nil
	}
	
	log.Printf("🔍 [订单历史检查] %s 找到 %d 个历史订单", symbol, len(orderHistory))
	
	var missedCount int
	currentTime := time.Now().Unix()
	
	// 检查最近24小时内的止损止盈成交
	for _, order := range orderHistory {
		orderType, _ := order["type"].(string)
		updateTime, _ := order["updateTime"].(int64)
		
		// 只检查最近24小时的订单
		if currentTime-updateTime/1000 > 86400 { // updateTime是毫秒，转为秒
			continue
		}
		
		// 只检查止损止盈订单
		if orderType != "STOP_MARKET" && orderType != "STOP" && 
		   orderType != "TAKE_PROFIT_MARKET" && orderType != "TAKE_PROFIT" {
			continue
		}
		
		orderID, _ := order["orderId"].(int64)
		positionSide, _ := order["positionSide"].(string)
		avgPrice, _ := order["avgPrice"].(string)
		quantity, _ := order["quantity"].(string)
		
		// 检查这个成交是否已经在我们的记录中
		if !at.isOrderExecutionRecorded(orderID, symbol, positionSide) {
			log.Printf("🔍 [订单历史检查] 发现遗漏的成交: %s %s 订单ID=%d", symbol, orderType, orderID)
			
			// 解析成交信息
			execPrice, _ := strconv.ParseFloat(avgPrice, 64)
			execQuantity, _ := strconv.ParseFloat(quantity, 64)
			
			if execPrice > 0 && execQuantity > 0 {
				// 确定方向
				side := "long"
				if positionSide == "SHORT" {
					side = "short"
				}
				
				// 记录这个遗漏的成交
				pendingOrder := &PendingStopOrder{
					Symbol:         symbol,
					Side:           side,
					OrderID:        orderID,
					StopPrice:      execPrice,
					Quantity:       execQuantity,
					CreateTime:     time.Unix(updateTime/1000, 0),
					OriginalAction: fmt.Sprintf("history_detected_%s", orderType),
				}
				
				log.Printf("📊 [订单历史检查] 记录遗漏成交: %s %s 数量=%.6f 价格=%.6f", 
					symbol, side, execQuantity, execPrice)
				
				at.recordStopLossExecution(pendingOrder, record)
				missedCount++
			}
		}
	}
	
	return missedCount, nil
}

// isOrderExecutionRecorded 检查指定订单的成交是否已经被记录
func (at *AutoTrader) isOrderExecutionRecorded(orderID int64, symbol, positionSide string) bool {
	if at.database == nil {
		return false // 无法验证，保守地认为未记录
	}
	
	// 从数据库检查是否有相关的交易动作记录
	actions, err := at.database.GetTradeActions(at.id, 100) // 获取最近100个动作
	if err != nil {
		log.Printf("⚠️ [订单历史检查] 获取交易动作记录失败: %v", err)
		return false
	}
	
	side := "long"
	if positionSide == "SHORT" {
		side = "short"  
	}
	
	// 检查是否有匹配的记录
	for _, action := range actions {
		if action.OrderID == fmt.Sprintf("%d", orderID) && 
		   action.Symbol == symbol && 
		   (strings.Contains(action.Action, side) || strings.Contains(action.Action, "stop_loss")) {
			return true
		}
	}
	
	return false
}

// validateDatabaseIntegrity 验证数据库交易记录的完整性
// 这个函数检查数据库记录与实际持仓的一致性，发现并修复数据不一致问题
func (at *AutoTrader) validateDatabaseIntegrity(record *logger.DecisionRecord) error {
	log.Printf("🔍 [数据库完整性校验] 开始验证交易记录完整性...")
	
	if at.database == nil {
		log.Printf("⚠️ [数据库完整性校验] 数据库连接不可用，跳过校验")
		return nil
	}
	
	var totalIssues int
	
	// 1. 检查开仓记录与实际持仓的一致性
	issues1, err := at.validateOpenTradesConsistency()
	if err != nil {
		log.Printf("❌ [数据库完整性校验] 开仓记录一致性检查失败: %v", err)
	} else {
		totalIssues += issues1
	}
	
	// 2. 检查交易动作记录的完整性
	issues2, err := at.validateTradeActionsIntegrity()
	if err != nil {
		log.Printf("❌ [数据库完整性校验] 交易动作完整性检查失败: %v", err)
	} else {
		totalIssues += issues2
	}
	
	// 3. 检查止损单跟踪记录的准确性
	issues3, err := at.validateStopOrdersConsistency()
	if err != nil {
		log.Printf("❌ [数据库完整性校验] 止损单记录一致性检查失败: %v", err)
	} else {
		totalIssues += issues3
	}
	
	// 4. 检查孤立和重复记录
	issues4, err := at.validateRecordConsistency()
	if err != nil {
		log.Printf("❌ [数据库完整性校验] 记录一致性检查失败: %v", err)
	} else {
		totalIssues += issues4
	}
	
	if totalIssues > 0 {
		log.Printf("⚠️ [数据库完整性校验] 发现 %d 个数据完整性问题，已进行修复", totalIssues)
	} else {
		log.Printf("✅ [数据库完整性校验] 数据库记录完整，无需修复")
	}
	
	return nil
}

// validateOpenTradesConsistency 验证开仓记录与实际持仓的一致性
func (at *AutoTrader) validateOpenTradesConsistency() (int, error) {
	log.Printf("🔍 [开仓一致性] 检查开仓记录与实际持仓的一致性...")
	
	// 获取数据库中所有开仓状态的交易记录
	openTrades, err := at.getOpenTradesFromDatabase()
	if err != nil {
		return 0, err
	}
	
	// 获取当前实际持仓
	currentPositions, err := at.trader.GetPositions()
	if err != nil {
		return 0, err
	}
	
	// 建立持仓映射
	positionMap := make(map[string]map[string]interface{}) // key: symbol_side
	for _, pos := range currentPositions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		key := fmt.Sprintf("%s_%s", symbol, side)
		positionMap[key] = pos
	}
	
	var issues int
	
	// 检查每个数据库开仓记录
	for _, trade := range openTrades {
		posKey := fmt.Sprintf("%s_%s", trade.Symbol, trade.Side)
		actualPos, hasPosition := positionMap[posKey]
		
		if !hasPosition {
			// 数据库显示开仓但实际无持仓
			log.Printf("❗ [开仓一致性] 发现数据不一致: %s %s 数据库显示开仓但实际无持仓", trade.Symbol, trade.Side)
			issues++
			
			// 估算关闭价格和原因（仅用于日志记录，不用于数据库写入）
			_, closeReason := at.estimateCloseDetails(trade.Symbol, trade.OpenPrice, trade.Side)
			
			// 更新数据库状态
			log.Printf("🔄 [开仓一致性] 修复不一致记录: %s %s", trade.Symbol, trade.Side)
			at.updateTradeInDatabase(trade.Symbol, trade.Side,  
				"INTEGRITY_CHECK", fmt.Sprintf("auto_fix_%s", closeReason))
		} else {
			// 检查数量是否匹配
			actualQuantity := actualPos["positionAmt"].(float64)
			if actualQuantity < 0 {
				actualQuantity = -actualQuantity
			}
			
			if math.Abs(actualQuantity-trade.Quantity) > 0.0001 {
				log.Printf("⚠️ [开仓一致性] 数量不匹配: %s %s 数据库(%.6f) vs ���际(%.6f)", 
					trade.Symbol, trade.Side, trade.Quantity, actualQuantity)
				// 注：这种情况通常是部分平仓导致，需要进一步分析
			}
		}
	}
	
	return issues, nil
}

// validateTradeActionsIntegrity 验证交易动作记录的完整性
func (at *AutoTrader) validateTradeActionsIntegrity() (int, error) {
	log.Printf("🔍 [动作完整性] 检查交易动作记录的完整性...")
	
	// 获取最近的交易动作记录
	actions, err := at.database.GetTradeActions(at.id, 200)
	if err != nil {
		return 0, err
	}
	
	var issues int
	
	// 按币种和方向分组统计动作
	actionGroups := make(map[string][]string) // key: symbol_side, value: []actions
	for _, action := range actions {
		// 从动作中推断方向
		var side string
		if strings.Contains(action.Action, "long") {
			side = "long"
		} else if strings.Contains(action.Action, "short") {
			side = "short"
		} else {
			continue // 跳过无法识别方向的动作
		}
		
		key := fmt.Sprintf("%s_%s", action.Symbol, side)
		actionGroups[key] = append(actionGroups[key], action.Action)
	}
	
	// 检查每个组的动作逻辑
	for key, actions := range actionGroups {
		openCount := 0
		closeCount := 0
		
		for _, action := range actions {
			if strings.Contains(action, "open") {
				openCount++
			} else if strings.Contains(action, "close") || strings.Contains(action, "stop_loss") {
				closeCount++
			}
		}
		
		// 检查开仓平仓逻辑是否合理
		if openCount > 0 && closeCount > openCount {
			log.Printf("⚠️ [动作完整性] %s 平仓次数(%d)超过开仓次数(%d)，可能存在记录遗漏", 
				key, closeCount, openCount)
			issues++
		}
	}
	
	return issues, nil
}

// validateStopOrdersConsistency 验证止损单跟踪记录的准确性
func (at *AutoTrader) validateStopOrdersConsistency() (int, error) {
	log.Printf("🔍 [止损单一致性] 检查止损单跟踪记录的准确性...")
	
	// 获取数据库中的活跃止损单记录
	activeStopOrders, err := at.database.GetActiveStopOrders(at.id)
	if err != nil {
		return 0, err
	}
	
	var issues int
	
	// 检查每个活跃的止损单
	for _, stopOrder := range activeStopOrders {
		// 检查对应的持仓是否还存在
		positions, err := at.trader.GetPositions()
		if err != nil {
			log.Printf("⚠️ [止损单一致性] 获取持仓失败: %v", err)
			continue
		}
		
		hasPosition := false
		for _, pos := range positions {
			if pos["symbol"] == stopOrder.Symbol && pos["side"] == stopOrder.Side {
				hasPosition = true
				break
			}
		}
		
		if !hasPosition {
			// 止损单显示活跃但持仓不存在，应该更新状态
			log.Printf("❗ [止损单一致性] 发现孤立止损单: %s %s (订单ID: %d)", 
				stopOrder.Symbol, stopOrder.Side, stopOrder.OrderID)
			issues++
			
			// 更新止损单状态为已成交
			err := at.database.UpdateStopOrderStatus(at.id, stopOrder.OrderID, "filled")
			if err != nil {
				log.Printf("❌ [止损单一致性] 更新止损单状态失败: %v", err)
			} else {
				log.Printf("✅ [止损单一致性] 已更新孤立止损单状态为已成交")
			}
		}
		
		// 验证止损单是否真的存在于交易所
		if hasPosition {
			orderStatus, err := at.trader.GetOrderStatus(stopOrder.Symbol, stopOrder.OrderID)
			if err != nil {
				// 订单不存在或已取消，但数据库显示活跃
				log.Printf("❗ [止损单一致性] 止损单在交易所不存在: %s %s (订单ID: %d)", 
					stopOrder.Symbol, stopOrder.Side, stopOrder.OrderID)
				issues++
				
				// 更新状态为取消
				at.database.UpdateStopOrderStatus(at.id, stopOrder.OrderID, "cancelled")
			} else {
				// 检查状态是否一致
				exchangeStatus, _ := orderStatus["status"].(string)
				if exchangeStatus == "FILLED" && stopOrder.Status == "active" {
					log.Printf("❗ [止损单一致性] 止损单状态不一致: 交易所已成交但数据库显示活跃")
					issues++
					
					// 更新状态
					at.database.UpdateStopOrderStatus(at.id, stopOrder.OrderID, "filled")
				}
			}
		}
	}
	
	return issues, nil
}

// validateRecordConsistency 检查孤立和重复记录
func (at *AutoTrader) validateRecordConsistency() (int, error) {
	log.Printf("🔍 [记录一致性] 检查孤立和重复记录...")
	
	var issues int
	
	// 检查是否有孤立的交易动作记录（没有对应的交易记录）
	actions, err := at.database.GetTradeActions(at.id, 100)
	if err != nil {
		return 0, err
	}
	
	// 获取所有交易记录用于比对
	trades, err := at.database.GetTraderTrades(at.id, 100)
	if err != nil {
		return 0, err
	}
	
	// 建立交易ID映射
	tradeIDs := make(map[string]bool)
	for _, trade := range trades {
		tradeIDs[trade.ID] = true
	}
	
	// 检查动作记录中的trade_id引用
	for _, action := range actions {
		if action.TradeID != nil && *action.TradeID != "" {
			if !tradeIDs[*action.TradeID] {
				log.Printf("❗ [记录一致性] 发现孤立的交易动作记录: %s (引用了不存在的交易ID: %s)", 
					action.Action, *action.TradeID)
				issues++
				// 注：这里可以选择清理孤立记录，但为了安全起见，只记录日志
			}
		}
	}
	
	// 检查重复的开仓记录（同一币种同一方向有多个开仓状态的记录）
	openTradeGroups := make(map[string][]*config.TradeRecord) // key: symbol_side
	for _, trade := range trades {
		if trade.Status == "open" {
			key := fmt.Sprintf("%s_%s", trade.Symbol, trade.Side)
			openTradeGroups[key] = append(openTradeGroups[key], trade)
		}
	}
	
	// 检查是否有重复的开仓记录
	for key, openTrades := range openTradeGroups {
		if len(openTrades) > 1 {
			log.Printf("❗ [记录一致性] 发现重复的开仓记录: %s 有 %d 个开仓状态的记录", key, len(openTrades))
			issues++
			
			// 保留最新的记录，关闭较旧的记录
			for i := 0; i < len(openTrades)-1; i++ {
				oldTrade := openTrades[i]
				log.Printf("🔄 [记录一致性] 关闭重复的旧开仓记录: %s (ID: %s)", key, oldTrade.ID)
				
				// 估算关闭价格
				estimatedPrice, _ := at.estimateCloseDetails(oldTrade.Symbol, oldTrade.OpenPrice, oldTrade.Side)
				
				// 关闭重复记录
				at.database.UpdateTrade(oldTrade.ID, estimatedPrice, time.Now(), 
					"closed", "duplicate_cleanup", "AUTO_CLEANUP", 0, 0, 0)
			}
		}
	}
	
	return issues, nil
}

// createStopLossDecisionRecord 为止损成交创建决策记录，确保decision_records表数据完整性
func (at *AutoTrader) createStopLossDecisionRecord(pendingOrder *PendingStopOrder, executionPrice, pnl float64, pnlCalculationMethod string) {
	if at.database == nil {
		log.Printf("⚠️ [止损决策记录] 数据库连接不可用，跳过决策记录创建")
		return
	}
	
	log.Printf("📝 [止损决策记录] 开始创建止损成交的决策记录...")
	log.Printf("    币种: %s %s", pendingOrder.Symbol, pendingOrder.Side)
	log.Printf("    订单ID: %d", pendingOrder.OrderID)
	log.Printf("    成交价格: %.6f", executionPrice)
	log.Printf("    盈亏: %.2f USDT (%s)", pnl, pnlCalculationMethod)
	
	// 创建止损决策记录
	decisionRecord := &config.DecisionRecordDB{
		TraderID:    at.id,
		CycleNumber: at.callCount, // 使用当前周期号
		Timestamp:   time.Now(),
		
		// 系统提示词 - 标明这是止损成交记录
		SystemPrompt: fmt.Sprintf("系统自动止损成交记录 - %s %s 订单ID: %d", 
			pendingOrder.Symbol, strings.ToUpper(pendingOrder.Side), pendingOrder.OrderID),
		
		// 输入提示词 - 描述止损成交的详细信息
		InputPrompt: fmt.Sprintf(
			"止损单自动成交:\n"+
				"币种: %s\n"+
				"方向: %s\n"+
				"数量: %.6f\n"+
				"成交价格: %.6f\n"+
				"盈亏: %.2f USDT\n"+
				"计算方法: %s\n"+
				"原始动作: %s\n"+
				"创建时间: %s",
			pendingOrder.Symbol, 
			strings.ToUpper(pendingOrder.Side),
			pendingOrder.Quantity,
			executionPrice,
			pnl,
			pnlCalculationMethod,
			pendingOrder.OriginalAction,
			pendingOrder.CreateTime.Format("2006-01-02 15:04:05")),
		
		// CoT思维链 - 说明止损逻辑
		CoTTrace: fmt.Sprintf(
			"止损成交分析:\n"+
				"1. 检测到止损单 %d 已成交\n"+
				"2. 成交价格: %.6f\n"+
				"3. 盈亏分析: %.2f USDT (使用%s计算方法)\n"+
				"4. 风控措施: 自动平仓保护资金\n"+
				"5. 数据同步: 更新trades和decision_records表",
			pendingOrder.OrderID,
			executionPrice,
			pnl,
			pnlCalculationMethod),
		
		// 决策JSON - 止损成交的决策动作
		DecisionJSON: fmt.Sprintf(`[{
			"symbol": "%s",
			"action": "stop_loss_%s",
			"reasoning": "止损单自动成交 - 订单ID: %d, 成交价格: %.6f",
			"quantity": %.6f,
			"price": %.6f,
			"pnl": %.2f,
			"calculation_method": "%s",
			"order_id": %d,
			"original_action": "%s"
		}]`,
			pendingOrder.Symbol,
			pendingOrder.Side,
			pendingOrder.OrderID,
			executionPrice,
			pendingOrder.Quantity,
			executionPrice,
			pnl,
			pnlCalculationMethod,
			pendingOrder.OrderID,
			pendingOrder.OriginalAction),
		
		// 账户状态JSON - 获取当前账户状态
		AccountStateJSON: at.getCurrentAccountStateJSON(),
		
		// 持仓JSON - 获取当前持仓状态
		PositionsJSON: at.getCurrentPositionsJSON(),
		
		// 候选币种JSON - 空数组，止损成交不涉及币种选择
		CandidateCoinsJSON: "[]",
		
		// 执行日志JSON
		ExecutionLogJSON: fmt.Sprintf(`["止损单 %d 自动成交: %s %s %.6f@%.6f, 盈亏: %.2f USDT"]`,
			pendingOrder.OrderID,
			pendingOrder.Symbol,
			strings.ToUpper(pendingOrder.Side),
			pendingOrder.Quantity,
			executionPrice,
			pnl),
		
		Success:      true,
		ErrorMessage: "",
	}
	
	// 保存到数据库
	err := at.database.CreateDecisionRecord(decisionRecord)
	if err != nil {
		log.Printf("❌ [止损决策记录] 创建决策记录失败: %v", err)
		log.Printf("    记录ID: %s", decisionRecord.ID)
		log.Printf("    TraderID: %s", decisionRecord.TraderID)
	} else {
		log.Printf("✅ [止损决策记录] 成功创建决策记录: %s", decisionRecord.ID)
		log.Printf("    确保了decision_records表与trades表的数据一致性")
	}
}

// getCurrentAccountStateJSON 获取当前账户状态的JSON字符串
func (at *AutoTrader) getCurrentAccountStateJSON() string {
	balance, err := at.trader.GetBalance()
	if err != nil {
		log.Printf("⚠️ [止损决策记录] 获取账户余额失败: %v", err)
		return "{}"
	}
	
	// 提取账户字段
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0
	
	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}
	
	totalEquity := totalWalletBalance + totalUnrealizedProfit
	
	accountState := map[string]interface{}{
		"TotalBalance":          totalEquity,
		"AvailableBalance":      availableBalance,
		"TotalUnrealizedProfit": totalUnrealizedProfit,
		"PositionCount":         0, // 将在持仓信息中更新
		"MarginUsedPct":         0.0,
	}
	
	jsonBytes, _ := json.Marshal(accountState)
	return string(jsonBytes)
}

// getCurrentPositionsJSON 获取当前持仓状态的JSON字符串
func (at *AutoTrader) getCurrentPositionsJSON() string {
	positions, err := at.trader.GetPositions()
	if err != nil {
		log.Printf("⚠️ [止损决策记录] 获取持仓信息失败: %v", err)
		return "[]"
	}
	
	var positionSnapshots []map[string]interface{}
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnL := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)
		
		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		
		positionSnapshot := map[string]interface{}{
			"Symbol":           symbol,
			"Side":             side,
			"PositionAmt":      quantity,
			"EntryPrice":       entryPrice,
			"MarkPrice":        markPrice,
			"UnrealizedProfit": unrealizedPnL,
			"Leverage":         float64(leverage),
			"LiquidationPrice": liquidationPrice,
		}
		
		positionSnapshots = append(positionSnapshots, positionSnapshot)
	}
	
	jsonBytes, _ := json.Marshal(positionSnapshots)
	return string(jsonBytes)
}

// handleRealtimeStopLossExecution 处理实时止损成交（WebSocket回调）
func (at *AutoTrader) handleRealtimeStopLossExecution(symbol, side string, orderID int64, executionPrice, executionQty float64) {
	log.Printf("🚨 [实时止损] 收到止损成交通知: %s %s 订单ID=%d 价格=%.6f 数量=%.6f", 
		symbol, side, orderID, executionPrice, executionQty)
	
	// 🆕 立即更新数据库中的交易记录状态
	if at.database != nil {
		log.Printf("🔄 [实时止损] 立即更新数据库交易记录...")
		at.updateTradeInDatabase(symbol, side,  
			fmt.Sprintf("%d", orderID), "stop_loss_websocket")
	}
	
	// 🆕 清理内存中的待确认止损单跟踪
	pendingKey := fmt.Sprintf("%s_%s_stop", symbol, side)
	if _, exists := at.pendingStopOrders[pendingKey]; exists {
		delete(at.pendingStopOrders, pendingKey)
		log.Printf("🧹 [实时止损] 清理待确认止损单: %s", pendingKey)
	}
	
	// 🆕 记录详细的交易动作到数据库
	if at.database != nil {
		actionRecord := &config.TradeActionRecord{
			TraderID:     at.id,
			Action:       fmt.Sprintf("stop_loss_%s_realtime", side),
			Symbol:       symbol,
			Quantity:     executionQty,
			Price:        executionPrice,
			OrderID:      fmt.Sprintf("%d", orderID),
			Timestamp:    time.Now(),
			Success:      true,
			ErrorMessage: "WebSocket实时止损成交 - 数据同步完成",
		}
		
		if err := at.database.CreateTradeAction(actionRecord); err != nil {
			log.Printf("❌ [实时止损] 记录交易动作失败: %v", err)
		} else {
			log.Printf("✅ [实时止损] 交易动作已记录: %s", actionRecord.ID)
		}
	}
	
	// 🔧 创建决策记录用于前端显示 - 仿照update_stop方式
	stopLossActionRecord := logger.DecisionAction{
		Action:    fmt.Sprintf("stop_loss_%s", side),
		Symbol:    symbol,
		Quantity:  executionQty,
		Price:     executionPrice,
		OrderID:   orderID,
		Timestamp: time.Now(),
		Success:   true,
		Error:     "",
	}
	
	// 创建简单的决策记录
	stopLossRecord := &logger.DecisionRecord{
		Decisions:    []logger.DecisionAction{stopLossActionRecord},
		ExecutionLog: []string{fmt.Sprintf("✓ %s stop_loss_%s 成交", symbol, side)},
		Success:      true,
	}
	
	// 保存决策记录（仿照runCycle中的方式）
	if err := at.decisionLogger.LogDecision(stopLossRecord); err != nil {
		log.Printf("⚠️ [实时止损] 保存决策记录失败: %v", err)
	}
	if at.database != nil {
		if err := at.saveToDatabaseRecord(stopLossRecord); err != nil {
			log.Printf("⚠️ [实时止损] 保存到数据库失败: %v", err)
		}
	}
	
	// 🆕 清理持仓时间跟踪记录
	posKey := symbol + "_" + side
	if _, exists := at.positionFirstSeenTime[posKey]; exists {
		delete(at.positionFirstSeenTime, posKey)
		log.Printf("🧹 [实时止损] 清理持仓时间��录: %s", posKey)
	}
	
	log.Printf("🎯 [实时止损] 止损成交处理完成: %s %s", symbol, side)
}

