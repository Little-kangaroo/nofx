package trader

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"nofx/config"
	"nofx/decision"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
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
	UseQwen     bool
	DeepSeekKey string
	QwenKey     string

	// 自定义AI API配置
	CustomAPIURL    string
	CustomAPIKey    string
	CustomModelName string

	// 扫描配置
	ScanInterval time.Duration // 扫描间隔（建议3分钟）

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

	return &AutoTrader{
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
	}, nil
}

// Run 运行自动交易主循环
func (at *AutoTrader) Run() error {
	at.isRunning = true
	log.Println("🚀 AI驱动自动交易系统启动")
	log.Printf("💰 初始余额: %.2f USDT", at.initialBalance)
	log.Printf("⚙️  扫描间隔: %v", at.config.ScanInterval)
	log.Println("🤖 AI将全权决定杠杆、仓位大小、止损止盈等参数")

	ticker := time.NewTicker(at.config.ScanInterval)
	defer ticker.Stop()

	// 首次立即执行
	if err := at.runCycle(); err != nil {
		log.Printf("❌ 执行失败: %v", err)
	}

	for at.isRunning {
		select {
		case <-ticker.C:
			if err := at.runCycle(); err != nil {
				log.Printf("❌ 执行失败: %v", err)
			}
		}
	}

	return nil
}

// Stop 停止自动交易
func (at *AutoTrader) Stop() {
	at.isRunning = false
	log.Println("⏹ 自动交易系统停止")
}

// runCycle 运行一个交易周期（使用AI全权决策）
func (at *AutoTrader) runCycle() error {
	at.callCount++

	log.Printf("\n" + strings.Repeat("=", 70))
	log.Printf("⏰ %s - AI决策周期 #%d", time.Now().Format("2006-01-02 15:04:05"), at.callCount)
	log.Printf(strings.Repeat("=", 70))

	// 创建决策记录
	record := &logger.DecisionRecord{
		ExecutionLog: []string{},
		Success:      true,
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
	decision, err := decision.GetFullDecisionWithCustomPrompt(ctx, at.mcpClient, at.customPrompt, at.overrideBasePrompt, at.systemPromptTemplate)

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
				log.Printf("\n" + strings.Repeat("=", 70))
				log.Printf("📋 系统提示词 [模板: %s] (错误情况)", at.systemPromptTemplate)
				log.Println(strings.Repeat("=", 70))
				log.Println(decision.SystemPrompt)
				log.Printf(strings.Repeat("=", 70) + "\n")
			}

			if decision.CoTTrace != "" {
				log.Printf("\n" + strings.Repeat("-", 70))
				log.Println("💭 AI思维链分析（错误情况）:")
				log.Println(strings.Repeat("-", 70))
				log.Println(decision.CoTTrace)
				log.Printf(strings.Repeat("-", 70) + "\n")
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
	log.Printf("\n" + strings.Repeat("-", 70))
	log.Println("💭 AI思维链分析:")
	log.Println(strings.Repeat("-", 70))
	log.Println(decision.CoTTrace)
	log.Printf(strings.Repeat("-", 70) + "\n")

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

	// 🔧 重要修复：获取实际成交价格
	// 等待订单确认后获取真实的持仓信息来确定实际成交价
	time.Sleep(2 * time.Second) // 等待订单确认
	actualPrice := marketData.CurrentPrice
	if actualFillPrice := at.getActualFillPrice(decision.Symbol, "long"); actualFillPrice > 0 {
		actualPrice = actualFillPrice
		actionRecord.Price = actualPrice
		log.Printf("  📊 实际开仓价格: %.4f (原请求价格: %.4f)", actualPrice, marketData.CurrentPrice)
	}

	// 记录到数据库
	at.recordTradeToDatabase(decision.Symbol, "long", quantity, decision.Leverage, 
		actualPrice, fmt.Sprintf("%v", order["orderId"]), "open_long", true)

	// 记录开仓时间
	posKey := decision.Symbol + "_long"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// 设置止损（风控必需）
	if _, err := at.trader.SetStopLoss(decision.Symbol, "LONG", quantity, decision.StopLoss); err != nil {
		// 如果错误提到"已存在"或"duplicate"，不视为错误
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "duplicate") || strings.Contains(errStr, "already exists") || strings.Contains(errStr, "已存在") {
			log.Printf("  ℹ 止损订单已存在，跳过设置")
		} else {
			log.Printf("  ⚠ 设置止损失败: %v", err)
		}
	} else {
		log.Printf("  ✓ 设置止损成功: %.2f", decision.StopLoss)
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

	// 🔧 重要修复：获取实际成交价格
	// 等待订单确认后获取真实的持仓信息来确定实际成交价
	time.Sleep(2 * time.Second) // 等待订单确认
	if actualPrice := at.getActualFillPrice(decision.Symbol, "short"); actualPrice > 0 {
		actionRecord.Price = actualPrice
		log.Printf("  📊 实际开仓价格: %.4f (原请求价格: %.4f)", actualPrice, marketData.CurrentPrice)
	}

	// 记录开仓时间
	posKey := decision.Symbol + "_short"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// 设置止损（风控必需）
	if _, err := at.trader.SetStopLoss(decision.Symbol, "SHORT", quantity, decision.StopLoss); err != nil {
		// 如果错误提到"已存在"或"duplicate"，不视为错误
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "duplicate") || strings.Contains(errStr, "already exists") || strings.Contains(errStr, "已存在") {
			log.Printf("  ℹ 止损订单已存在，跳过设置")
		} else {
			log.Printf("  ⚠ 设置止损失败: %v", err)
		}
	} else {
		log.Printf("  ✓ 设置止损成功: %.2f", decision.StopLoss)
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

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
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

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
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

// checkPendingStopOrders 检查待确认的止损单状态，记录成交的订单
func (at *AutoTrader) checkPendingStopOrders(record *logger.DecisionRecord) error {
	// 1. 检查内存中跟踪的止损单（AI更新的）
	if err := at.checkTrackedStopOrders(record); err != nil {
		log.Printf("⚠️ 检查跟踪的止损单失败: %v", err)
	}
	
	// 2. 检查所有持仓的止损单（包括开仓时设置的）
	if err := at.checkAllPositionStopOrders(record); err != nil {
		log.Printf("⚠️ 检查所有持仓止损单失败: %v", err)
	}
	
	return nil
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
					
					// 🔧 关键判断：只有持仓完全消失才认为是真正的止损成交
					if !hasCurrentPosition {
						log.Printf("    ✅ 确认真正的止损成交: %s %s 持仓已完全平仓", symbol, side)
						
						// 获取止损单详细信息
						stopPrice := 0.0
						quantity := posQuantity // 使用上次记录的持仓数量
						if stopPriceStr, ok := lastOrder["stopPrice"].(string); ok {
							stopPrice, _ = strconv.ParseFloat(stopPriceStr, 64)
						}
						
						// 严格验证：只有在确实有数量且价格合理时才记录止损成交
						if quantity > 0 && stopPrice > 0 {
							log.Printf("    ✅ 记录真正的止损成交: %s %s 数量=%.6f 止损价=%.6f", 
								symbol, side, quantity, stopPrice)
							
							pendingOrder := &PendingStopOrder{
								Symbol:         symbol,
								Side:           side,
								OrderID:        lastOrderID,
								StopPrice:      stopPrice,
								Quantity:       quantity,
								CreateTime:     time.Now(),
								OriginalAction: "stop_loss_detected",
							}
							
							at.recordStopLossExecution(pendingOrder, record)
						} else {
							log.Printf("    ❌ 验证失败，跳过记录: quantity=%.6f, stopPrice=%.6f", 
								quantity, stopPrice)
						}
					} else {
						log.Printf("    ⚠️ 止损单消失但持仓仍存在(%.6f)，可能是止损更新而非成交，跳过记录", 
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
	// 🔧 修复：使用止损价格作为成交价，而不是市场价格
	// 止损单成交时，成交价格应该接近止损价格
	executionPrice := pendingOrder.StopPrice
	
	// 获取当前市场价格用于验证和日志记录
	marketData, err := market.Get(pendingOrder.Symbol)
	if err != nil {
		log.Printf("⚠️ 获取 %s 市场价格失败: %v", pendingOrder.Symbol, err)
		// 继续使用止损价格，不因市场价格获取失败而中断
	} else {
		// 记录市场价格与止损价格的差异（用于调试）
		priceDiff := math.Abs(marketData.CurrentPrice - pendingOrder.StopPrice)
		log.Printf("🔍 止损成交验证: 市场价=%.6f, 止损价=%.6f, 差异=%.6f", 
			marketData.CurrentPrice, pendingOrder.StopPrice, priceDiff)
	}
	
	// 计算盈亏（这里是简化计算，实际应该使用精确的成交价格）
	var pnl float64
	if pendingOrder.Side == "long" {
		// 多仓止损：入场价未知，使用止损价作为参考
		pnl = (executionPrice - pendingOrder.StopPrice) * pendingOrder.Quantity
	} else {
		// 空仓止损：入场价未知，使用止损价作为参考
		pnl = (pendingOrder.StopPrice - executionPrice) * pendingOrder.Quantity
	}
	
	log.Printf("📊 记录止损成交: %s %s 数量=%.4f 价格=%.6f 预估盈亏=%.2f", 
		pendingOrder.Symbol, pendingOrder.Side, pendingOrder.Quantity, executionPrice, pnl)
	
	// 创建交易记录
	actionRecord := &logger.DecisionAction{
		Symbol:    pendingOrder.Symbol,
		Action:    fmt.Sprintf("stop_loss_%s", pendingOrder.Side), // stop_loss_long 或 stop_loss_short
		Quantity:  pendingOrder.Quantity,
		Price:     executionPrice,
		OrderID:   pendingOrder.OrderID,
		Success:   true,
		Timestamp: time.Now(),
		Error:     "",
	}
	
	// 添加到决策记录中
	if record.Decisions == nil {
		record.Decisions = []logger.DecisionAction{}
	}
	record.Decisions = append(record.Decisions, *actionRecord)
	
	// 添加执行日志
	logMessage := fmt.Sprintf("🎯 止损成交: %s %s %.4f@%.6f (原订单ID: %d)", 
		pendingOrder.Symbol, strings.ToUpper(pendingOrder.Side), 
		pendingOrder.Quantity, executionPrice, pendingOrder.OrderID)
	record.ExecutionLog = append(record.ExecutionLog, logMessage)
	
	// 交易记录已通过 DecisionAction 添加到 record.Decisions 中
	// 日志将在 LogDecision(record) 时统一记录
}

// recordTradeToDatabase 将交易记录到数据库
func (at *AutoTrader) recordTradeToDatabase(symbol, side string, quantity float64, leverage int, 
	price float64, orderID string, action string, isOpen bool) {
	if at.database == nil {
		return
	}

	if isOpen {
		// 开仓记录
		tradeRecord := &config.TradeRecord{
			TraderID:      at.id,
			Symbol:        symbol,
			Side:          side,
			Quantity:      quantity,
			Leverage:      leverage,
			OpenPrice:     price,
			PositionValue: quantity * price,
			MarginUsed:    (quantity * price) / float64(leverage),
			OpenTime:      time.Now(),
			Status:        "open",
			OpenOrderID:   orderID,
		}
		
		if err := at.database.CreateTrade(tradeRecord); err != nil {
			log.Printf("  ⚠️ 记录开仓到数据库失败: %v", err)
		} else {
			log.Printf("  💾 已记录开仓到数据库: %s", tradeRecord.ID)
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
func (at *AutoTrader) updateTradeInDatabase(symbol, side string, closePrice float64, 
	closeOrderID, closeReason string) {
	if at.database == nil {
		return
	}

	// 查找对应的开仓记录
	openTrade, err := at.database.GetOpenTrade(at.id, symbol, side)
	if err != nil {
		log.Printf("  ⚠️ 查找开仓记录失败: %v", err)
		return
	}

	// 计算盈亏
	var pnl float64
	if side == "long" {
		pnl = openTrade.Quantity * (closePrice - openTrade.OpenPrice)
	} else {
		pnl = openTrade.Quantity * (openTrade.OpenPrice - closePrice)
	}
	
	pnlPct := (pnl / openTrade.MarginUsed) * 100
	closeTime := time.Now()
	durationSecs := int(closeTime.Sub(openTrade.OpenTime).Seconds())

	if err := at.database.UpdateTrade(openTrade.ID, closePrice, closeTime, 
		"closed", closeReason, closeOrderID, pnl, pnlPct, durationSecs); err != nil {
		log.Printf("  ⚠️ 更新交易记录失败: %v", err)
	} else {
		log.Printf("  💾 已更新交易记录: PnL=%.2f USDT (%.2f%%)", pnl, pnlPct)
	}
}
