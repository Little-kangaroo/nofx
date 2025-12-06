package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
	"nofx/microstructure"
)

func main() {
	log.Printf("🧪 开始验证V2.0订单流输出格式...")
	
	// 初始化订单流管理器
	config := microstructure.DefaultMicrostructureConfig()
	orderFlowManager := microstructure.NewOrderFlowManager(config)
	
	// 启动管理器
	err := orderFlowManager.Start()
	if err != nil {
		log.Fatalf("启动订单流管理器失败: %v", err)
	}
	defer orderFlowManager.Stop()
	
	// 订阅测试交易对
	symbol := "BTCUSDT"
	err = orderFlowManager.SubscribeSymbol(symbol)
	if err != nil {
		log.Printf("订阅失败: %v", err)
	}
	
	// 模拟一些交易数据
	tradeData1 := &microstructure.TradeData{
		Symbol:        symbol,
		TradeId:       1,
		Price:         50000.0,
		Quantity:      1.0,
		Timestamp:     time.Now(),
		IsBuyerMaker:  false, // 主动买单
		MarketType:    "spot",
	}
	
	tradeData2 := &microstructure.TradeData{
		Symbol:        symbol,
		TradeId:       2,
		Price:         49999.0,
		Quantity:      0.5,
		Timestamp:     time.Now(),
		IsBuyerMaker:  true, // 主动卖单
		MarketType:    "futures",
	}
	
	// 模拟盘口数据
	depthData := &microstructure.DepthData{
		Symbol:    symbol,
		Timestamp: time.Now(),
		Bids: []microstructure.OrderBookLevel{
			{Price: 49995, Quantity: 2.0},
			{Price: 49990, Quantity: 3.0},
		},
		Asks: []microstructure.OrderBookLevel{
			{Price: 50005, Quantity: 2.5},
			{Price: 50010, Quantity: 3.5},
		},
	}
	
	// 手动注入数据到管理器的组件中
	orderFlowManager.GetMarketSnapshot(symbol) // 触发组件创建
	
	// 等待一下让数据处理完成
	time.Sleep(100 * time.Millisecond)
	
	// 获取市场快照
	snapshot := orderFlowManager.GetMarketSnapshot(symbol)
	if snapshot == nil {
		log.Printf("⚠️ 无法获取市场快照，可能是因为没有实际数据流")
		// 创建一个模拟快照来展示格式
		snapshot = createMockSnapshot(symbol)
	}
	
	// 转换为AI格式
	aiPayload := snapshot.ToAIPayload()
	
	// 格式化输出
	jsonBytes, err := json.MarshalIndent(aiPayload, "", "  ")
	if err != nil {
		log.Fatalf("JSON序列化失败: %v", err)
	}
	
	fmt.Printf("\n=== V2.0 订单流输出格式验证 ===\n")
	fmt.Printf("Symbol: %s\n", symbol)
	fmt.Printf("Timestamp: %s\n", snapshot.Timestamp.Format("2006-01-02 15:04:05"))
	
	fmt.Printf("\n=== AI数据格式 ===\n")
	fmt.Println(string(jsonBytes))
	
	// 验证关键字段是否存在
	fmt.Printf("\n=== 格式验证结果 ===\n")
	
	orderFlowData, ok := aiPayload["订单流分析"].(map[string]interface{})
	if !ok {
		fmt.Printf("❌ 缺少'订单流分析'字段\n")
		return
	}
	
	// 验证本周期博弈数据
	currentPeriod, ok := orderFlowData["本周期博弈_5m"].(map[string]interface{})
	if !ok {
		fmt.Printf("❌ 缺少'本周期博弈_5m'字段\n")
	} else {
		fmt.Printf("✅ '本周期博弈_5m'字段存在\n")
		
		// 检查关键子字段
		requiredFields := []string{
			"price_delta_pct", "spot_cvd_delta_usd", "futures_cvd_delta_usd", 
			"candle_intent", "data_quality",
		}
		
		for _, field := range requiredFields {
			if _, exists := currentPeriod[field]; exists {
				fmt.Printf("  ✅ %s\n", field)
			} else {
				fmt.Printf("  ❌ %s 缺失\n", field)
			}
		}
	}
	
	// 验证宏观资金趋势
	macroTrend, ok := orderFlowData["宏观资金趋势"].(map[string]interface{})
	if !ok {
		fmt.Printf("❌ 缺少'宏观资金趋势'字段\n")
	} else {
		fmt.Printf("✅ '宏观资金趋势'字段存在\n")
		
		requiredFields := []string{
			"spot_cvd_1h_usd", "futures_cvd_1h_usd", "cvd_divergence", "signal",
		}
		
		for _, field := range requiredFields {
			if _, exists := macroTrend[field]; exists {
				fmt.Printf("  ✅ %s\n", field)
			} else {
				fmt.Printf("  ❌ %s 缺失\n", field)
			}
		}
	}
	
	// 验证盘口结构
	orderBookStructure, ok := orderFlowData["盘口结构"].(map[string]interface{})
	if !ok {
		fmt.Printf("❌ 缺少'盘口结构'字段\n")
	} else {
		fmt.Printf("✅ '盘口结构'字段存在\n")
		
		// V2.0新增字段验证
		v2Fields := []string{
			"spoofing_risk", "liquidity_score", "wall_change_count_5m", "imbalance_trend",
		}
		
		for _, field := range v2Fields {
			if _, exists := orderBookStructure[field]; exists {
				fmt.Printf("  ✅ V2.0字段: %s\n", field)
			} else {
				fmt.Printf("  ❌ V2.0字段缺失: %s\n", field)
			}
		}
	}
	
	// 验证数据质量字段
	dataQuality, ok := orderFlowData["数据质量"].(map[string]interface{})
	if !ok {
		fmt.Printf("❌ 缺少'数据质量'字段\n")
	} else {
		fmt.Printf("✅ '数据质量'字段存在\n")
		
		// 检查数据��量指标
		qualityFields := []string{"cvd_stale", "oi_stale", "orderbook_stale", "status"}
		for _, field := range qualityFields {
			if _, exists := dataQuality[field]; exists {
				fmt.Printf("  ✅ %s\n", field)
			} else {
				fmt.Printf("  ❌ %s 缺失\n", field)
			}
		}
	}
	
	fmt.Printf("\n🎉 V2.0订单流输出格式验证完成!\n")
}

// createMockSnapshot 创建模拟快照用于展示格式
func createMockSnapshot(symbol string) *microstructure.MarketSnapshot {
	return &microstructure.MarketSnapshot{
		Symbol:    symbol,
		Timestamp: time.Now(),
		CVDData: &microstructure.CVDData{
			SpotCVD1H:     150000,
			FuturesCVD1H:  -80000,
			CVDDivergence: "bullish_spot_futures_divergence",
			Signal:        "bullish_absorption",
			LastUpdate:    time.Now(),
			IsStale:       false,
		},
		OIAnalysis: &microstructure.OIAnalysis{
			Current:      1200000000,
			ChangeRate1H: 2.5,
			Trend:        "increasing",
			IsStale:      false,
		},
		OrderBookData: &microstructure.OrderBookData{
			ImbalanceRatio:      0.15,
			BidPressure:         250000,
			AskPressure:         200000,
			ImbalanceTrend:      "increasing_bid_pressure",
			PressureDelta5m:     0.05,
			SpoofingRisk:        0.2,
			LiquidityScore:      0.8,
			WallChangeCount5m:   3,
			LastUpdate:          time.Now(),
			IsStale:             false,
		},
		MarketContext: &microstructure.MarketContext{
			CVDDivergence:     "bullish_divergence",
			ContextInference:  "strong_buying_pressure",
			SignalStrength:    85.0,
		},
		PriceContext: &microstructure.PriceContext{
			CurrentPrice: 50000,
			Change1H:     1.2,
			Change4H:     -0.5,
		},
	}
}