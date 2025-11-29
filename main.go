package main

import (
	"encoding/json"
	"fmt"
	"log"
	"nofx/api"
	"nofx/auth"
	"nofx/config"
	"nofx/logger"
	"nofx/manager"
	"nofx/market"
	"nofx/pool"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// LeverageConfig 杠杆配置
type LeverageConfig struct {
	BTCETHLeverage  int `json:"btc_eth_leverage"`
	AltcoinLeverage int `json:"altcoin_leverage"`
}

// ConfigFile 配置文件结构，只包含需要同步到数据库的字段
type ConfigFile struct {
	AdminMode          bool           `json:"admin_mode"`
	BetaMode           bool           `json:"beta_mode"`
	APIServerPort      int            `json:"api_server_port"`
	UseDefaultCoins    bool           `json:"use_default_coins"`
	DefaultCoins       []string       `json:"default_coins"`
	CoinPoolAPIURL     string         `json:"coin_pool_api_url"`
	OITopAPIURL        string         `json:"oi_top_api_url"`
	MaxDailyLoss       float64        `json:"max_daily_loss"`
	MaxDrawdown        float64        `json:"max_drawdown"`
	StopTradingMinutes int            `json:"stop_trading_minutes"`
	Leverage           LeverageConfig `json:"leverage"`
	JWTSecret          string         `json:"jwt_secret"`
	DataKLineTime      string         `json:"data_k_line_time"`
}

// syncConfigToDatabase 从config.json读取配置并同步到数据库
func syncConfigToDatabase(database *config.Database) error {
	// 检查config.json是否存在
	if _, err := os.Stat("config.json"); os.IsNotExist(err) {
		log.Printf("📄 config.json不存在，跳过同步")
		return nil
	}

	// 读取config.json
	data, err := os.ReadFile("config.json")
	if err != nil {
		return fmt.Errorf("读取config.json失败: %w", err)
	}

	// 解析JSON
	var configFile ConfigFile
	if err := json.Unmarshal(data, &configFile); err != nil {
		return fmt.Errorf("解析config.json失败: %w", err)
	}

	log.Printf("🔄 开始同步config.json到数据库...")

	// 同步各配置项到数据库
	configs := map[string]string{
		"admin_mode":            fmt.Sprintf("%t", configFile.AdminMode),
		"beta_mode":             fmt.Sprintf("%t", configFile.BetaMode),
		"api_server_port":       strconv.Itoa(configFile.APIServerPort),
		"use_default_coins":     fmt.Sprintf("%t", configFile.UseDefaultCoins),
		"coin_pool_api_url":     configFile.CoinPoolAPIURL,
		"oi_top_api_url":        configFile.OITopAPIURL,
		"max_daily_loss":        fmt.Sprintf("%.1f", configFile.MaxDailyLoss),
		"max_drawdown":          fmt.Sprintf("%.1f", configFile.MaxDrawdown),
		"stop_trading_minutes":  strconv.Itoa(configFile.StopTradingMinutes),
	}

	// 同步default_coins（转换为JSON字符串存储）
	if len(configFile.DefaultCoins) > 0 {
		defaultCoinsJSON, err := json.Marshal(configFile.DefaultCoins)
		if err == nil {
			configs["default_coins"] = string(defaultCoinsJSON)
		}
	}

	// 同步杠杆配置
	if configFile.Leverage.BTCETHLeverage > 0 {
		configs["btc_eth_leverage"] = strconv.Itoa(configFile.Leverage.BTCETHLeverage)
	}
	if configFile.Leverage.AltcoinLeverage > 0 {
		configs["altcoin_leverage"] = strconv.Itoa(configFile.Leverage.AltcoinLeverage)
	}

	// 如果JWT密钥不为空，也同步
	if configFile.JWTSecret != "" {
		configs["jwt_secret"] = configFile.JWTSecret
	}

	// 更新数据库配置
	for key, value := range configs {
		if err := database.SetSystemConfig(key, value); err != nil {
			log.Printf("⚠️  更新配置 %s 失败: %v", key, err)
		} else {
			log.Printf("✓ 同步配置: %s = %s", key, value)
		}
	}

	log.Printf("✅ config.json同步完成")
	return nil
}

// loadBetaCodesToDatabase 加载内测码文件到数据库
func loadBetaCodesToDatabase(database *config.Database) error {
	betaCodeFile := "beta_codes.txt"
	
	// 检查内测码文件是否存在
	if _, err := os.Stat(betaCodeFile); os.IsNotExist(err) {
		log.Printf("📄 内测码文件 %s 不存在，跳过加载", betaCodeFile)
		return nil
	}

	// 获取文件信息
	fileInfo, err := os.Stat(betaCodeFile)
	if err != nil {
		return fmt.Errorf("获取内测码文件信息失败: %w", err)
	}

	log.Printf("🔄 发现内测码文件 %s (%.1f KB)，开始加载...", betaCodeFile, float64(fileInfo.Size())/1024)
	
	// 加载内测码到数据库
	err = database.LoadBetaCodesFromFile(betaCodeFile)
	if err != nil {
		return fmt.Errorf("加载内测码失败: %w", err)
	}

	// 显示统计信息
	total, used, err := database.GetBetaCodeStats()
	if err != nil {
		log.Printf("⚠️  获取内测码统计失败: %v", err)
	} else {
		log.Printf("✅ 内测码加载完成: 总计 %d 个，已使用 %d 个，剩余 %d 个", total, used, total-used)
	}

	return nil
}

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║    🤖 AI多模型交易系统 - 支持 DeepSeek & Qwen            ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 🔥 优先初始化日志系统
	logManager, err := logger.NewLogManager(logger.LogConfig{
		LogDir:    "logs",          // 日志目录
		MaxSizeMB: 100,            // 每个文件最大100MB
		MaxDays:   30,             // 保留30天
	})
	if err != nil {
		log.Fatalf("❌ 日志系统初始化失败: %v", err)
	}
	defer logManager.Close()

	logger.LogSystemEvent("🚀 nofx系统启动")
	logger.LogInfo("日志文件: %s", logManager.GetLogFile())

	// 检查命令行参数
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "demo", "-demo", "--demo":
			// 运行市场分析演示
			logger.LogInfo("🎯 启动市场分析演示模式...")
			fmt.Println("🎯 启动市场分析演示模式...")
			fmt.Println()
			fmt.Println("演示功能已暂时禁用")
			return
		case "market", "-market", "--market":
			// 运行市场分析示例
			fmt.Println("📊 启动市场分析示例...")
			fmt.Println()
			fmt.Println("示例功能已暂时禁用")
			return
		case "fvg", "-fvg", "--fvg":
			// 只运行FVG演示
			fmt.Println("🕳️ FVG分析功能已实现")
			fmt.Println("功能：公平价值缺口识别、质量评估、填补跟踪、交易信号")
			return
		case "comprehensive", "-comprehensive", "--comprehensive":
			// 运行综合分析演示
			fmt.Println("🔄 综合分析功能已实现")
			fmt.Println("功能：多模块信号融合、权重分配、风险评估、统一决策")
			return
		case "complete", "-complete", "--complete":
			// 运行完整系统演示
			fmt.Println("⭐ 完整系统功能已实现")
			fmt.Println("包含：道氏理论 + VPVR + 供需区 + FVG + 斐波纳契 五大模块")
			return
		case "fibonacci", "-fibonacci", "--fibonacci":
			// 运行斐波纳契演示
			fmt.Println("📊 斐波纳契分析功能已实现")
			fmt.Println("功能：0.618黄金口袋、回调扩展、聚集区分析、交易信号")
			return
		case "help", "-h", "--help":
			// 显示帮助信息
			fmt.Println("🎯 AI多模型交易系统 - 使用说明")
			fmt.Println()
			fmt.Println("使用方式:")
			fmt.Println("  go run main.go                    # 启动交易系统")
			fmt.Println("  go run main.go demo               # 市场分析完整演示")
			fmt.Println("  go run main.go market             # 所有市场分析示例")
			fmt.Println("  go run main.go fvg                # FVG分析演示")
			fmt.Println("  go run main.go comprehensive      # 综合分析演示")
			fmt.Println("  go run main.go fibonacci          # 斐波纳契分析演示")
			fmt.Println("  go run main.go complete           # 完整系统演示")
			fmt.Println()
			fmt.Println("市场分析模块:")
			fmt.Println("  📈 道氏理论 - 趋势识别与通道分析")
			fmt.Println("  📊 VPVR - 成交量分布与关键价位")
			fmt.Println("  ⚖️  供需区 - 供给需求区域分析")
			fmt.Println("  🕳️  FVG - 公平价值缺口分析")
			fmt.Println("  🔢 斐波纳契 - 0.618黄金口袋回调分析")
			fmt.Println()
			return
		}
	}

	// 初始化数据库配置
	dbPath := "config.db"
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		// 如果第一个参数不是选项，则作为数据库路径
		dbPath = os.Args[1]
	}

	logger.LogSystemEvent("📋 初始化配置数据库: %s", dbPath)
	database, err := config.NewDatabase(dbPath)
	if err != nil {
		logger.LogError("❌ 数据库初始化失败: %v", err)
		log.Fatalf("❌ 数据库初始化失败: %v", err)
	}
	defer database.Close()
	logger.LogSuccess("✅ 数据库初始化成功")

	// 同步config.json到数据库
	if err := syncConfigToDatabase(database); err != nil {
		log.Printf("⚠️  同步config.json到数据库失败: %v", err)
	}

	// 加载内测码到数据库
	if err := loadBetaCodesToDatabase(database); err != nil {
		log.Printf("⚠️  加载内测码到数据库失败: %v", err)
	}

	// 获取系统配置
	useDefaultCoinsStr, _ := database.GetSystemConfig("use_default_coins")
	useDefaultCoins := useDefaultCoinsStr == "true"
	apiPortStr, _ := database.GetSystemConfig("api_server_port")

	// 获取管理员模式配置
	adminModeStr, _ := database.GetSystemConfig("admin_mode")
	adminMode := adminModeStr != "false" // 默认为true

	// 设置JWT密钥
	jwtSecret, _ := database.GetSystemConfig("jwt_secret")
	if jwtSecret == "" {
		jwtSecret = "your-jwt-secret-key-change-in-production-make-it-long-and-random"
		log.Printf("⚠️  使用默认JWT密钥，建议在生产环境中配置")
	}
	auth.SetJWTSecret(jwtSecret)

	// 在管理员模式下，确保admin用户存在
	if adminMode {
		err := database.EnsureAdminUser()
		if err != nil {
			log.Printf("⚠️  创建admin用户失败: %v", err)
		} else {
			log.Printf("✓ 管理员模式已启用，无需登录")
		}
		auth.SetAdminMode(true)
	}

	log.Printf("✓ 配置数据库初始化成功")
	fmt.Println()

	// 从数据库读取默认主流币种列表
	defaultCoinsJSON, _ := database.GetSystemConfig("default_coins")
	var defaultCoins []string

	if defaultCoinsJSON != "" {
		// 尝试从JSON解析
		if err := json.Unmarshal([]byte(defaultCoinsJSON), &defaultCoins); err != nil {
			log.Printf("⚠️  解析default_coins配置失败: %v，使用硬编码默认值", err)
			defaultCoins = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT", "HYPEUSDT"}
		} else {
			log.Printf("✓ 从数据库加载默认币种列表（共%d个）: %v", len(defaultCoins), defaultCoins)
		}
	} else {
		// 如果数据库中没有配置，使用硬编码默认值
		defaultCoins = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT", "HYPEUSDT"}
		log.Printf("⚠️  数据库中未配置default_coins，使用硬编码默认值")
	}

	pool.SetDefaultCoins(defaultCoins)
	// 设置是否使用默认主流币种
	pool.SetUseDefaultCoins(useDefaultCoins)
	if useDefaultCoins {
		log.Printf("✓ 已启用默认主流币种列表")
	}

	// 设置币种池API URL
	coinPoolAPIURL, _ := database.GetSystemConfig("coin_pool_api_url")
	if coinPoolAPIURL != "" {
		pool.SetCoinPoolAPI(coinPoolAPIURL)
		log.Printf("✓ 已配置AI500币种池API")
	}

	oiTopAPIURL, _ := database.GetSystemConfig("oi_top_api_url")
	if oiTopAPIURL != "" {
		pool.SetOITopAPI(oiTopAPIURL)
		log.Printf("✓ 已配置OI Top API")
	}

	// 创建TraderManager
	traderManager := manager.NewTraderManager()

	// 从数据库加载所有交易员到内存
	err = traderManager.LoadTradersFromDatabase(database)
	if err != nil {
		log.Fatalf("❌ 加载交易员失败: %v", err)
	}

	// 显示加载的交易员信息（从TraderManager获取实际加载的交易员）
	fmt.Println()
	fmt.Println("🤖 已加载的AI交易员配置:")
	
	allTraders := traderManager.GetAllTraders()
	if len(allTraders) == 0 {
		fmt.Println("  • 暂无配置的交易员，请通过Web界面创建")
	} else {
		for _, at := range allTraders {
			status := at.GetStatus()
			runningStatus := "停止"
			if running, ok := status["is_running"].(bool); ok && running {
				runningStatus = "运行中"
			}
			
			// 安全获取初始余额
			initialBalance := 0.0
			if balance, ok := status["initial_balance"].(float64); ok {
				initialBalance = balance
			}
			
			fmt.Printf("  • %s (%s + %s) - 初始资金: %.0f USDT [%s]\n",
				at.GetName(), strings.ToUpper(at.GetAIModel()), strings.ToUpper(at.GetExchange()),
				initialBalance, runningStatus)
		}
		fmt.Printf("  ✓ 总计已加载 %d 个交易员到内存\n", len(allTraders))
	}

	fmt.Println()
	fmt.Println("🤖 AI全权决策模式:")
	fmt.Printf("  • AI将自主决定每笔交易的杠杆倍数（山寨币最高5倍，BTC/ETH最高5倍）\n")
	fmt.Println("  • AI将自主决定每笔交易的仓位大小")
	fmt.Println("  • AI将自主设置止损和止盈价格")
	fmt.Println("  • AI将基于市场数据、技术指标、账户状态做出全面分析")
	fmt.Println()
	fmt.Println("🕐 AI触发机制:")
	fmt.Println("  • 系统将监听BTCUSDT 5分钟K线收盘事件")
	fmt.Println("  • 每当收到BTCUSDT 5分钟收盘事件时，将自动触发所有运行中的AI交易员执行分析")
	fmt.Println("  • 这确保AI分析与币安真实K线时间同步，避免时间偏差")
	fmt.Println()
	fmt.Println("⚠️  风险提示: AI自动交易有风险，建议小额资金测试！")
	fmt.Println()
	fmt.Println("按 Ctrl+C 停止运行")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// 获取API服务器端口
	apiPort := 8080 // 默认端口
	if apiPortStr != "" {
		if port, err := strconv.Atoi(apiPortStr); err == nil {
			apiPort = port
		}
	}

	// 创建并启动API服务器
	apiServer := api.NewServer(traderManager, database, apiPort)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Printf("❌ API服务器错误: %v", err)
		}
	}()

	// 启动流行情数据 - 默认使用所有交易员设置的币种 如果没有设置币种 则优先使用系统默认
	wsMonitor := market.NewWSMonitor(150)
	
	// 设置BTCUSDT触发器回调：触发所有运行中的trader执行AI分析
	wsMonitor.SetBTCTrigger(func() {
		callbackStart := time.Now()
		log.Printf("🔔 BTCUSDT触发回调开始: %v", callbackStart.Format("15:04:05.000"))
		
		// 获取所有运行中的traders
		allTraders := traderManager.GetAllTraders()
		runningCount := 0
		
		for traderID, trader := range allTraders {
			status := trader.GetStatus()
			if running, ok := status["is_running"].(bool); ok && running {
				runningCount++
				log.Printf("🕐 BTCUSDT触发AI分析: %s", traderID)
				// 同步执行，与原来定时器方式一致
				trader.TriggerCycle()
			}
		}
		
		totalDuration := time.Since(callbackStart)
		log.Printf("🏁 所有AI分析完成: %d个traders, 总耗时 %v", runningCount, totalDuration)
	})
	
	go wsMonitor.Start(database.GetCustomCoins())
	//go market.NewWSMonitor(150).Start([]string{}) //这里是一个使用方式 传入空的话 则使用market市场的所有币种
	// 设置优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// TODO: 启动数据库中配置为运行状态的交易员
	// traderManager.StartAll()

	// 等待退出信号
	<-sigChan
	fmt.Println()
	fmt.Println()
	log.Println("📛 收到退出信号，正在停止所有trader...")
	traderManager.StopAll()

	fmt.Println()
	fmt.Println("👋 感谢使用AI交易系统！")
}
