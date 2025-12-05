package main

import (
	"encoding/json"
	"log"
	"nofx/microstructure"
	"time"
)

// OrderFlow使用示例
func main() {
	log.Println("🚀 启动币安订单流分析系统示例...")

	// 1. 初始化订单流系统
	config := microstructure.DefaultMicrostructureConfig()
	
	// 可以自定义配置
	config.EnableCVDAnalysis = true
	config.EnableOrderBook = true
	config.CVDWindowDuration = time.Hour
	config.WallThresholdMultiple = 5.0
	
	ofm := microstructure.NewOrderFlowManager(config)
	
	// 启动系统
	if err := ofm.Start(); err != nil {
		log.Fatalf("❌ 启动订单流管理器失败: %v", err)
	}
	defer ofm.Stop()

	// 2. 订阅币种
	symbols := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	for _, symbol := range symbols {
		if err := ofm.SubscribeSymbol(symbol); err != nil {
			log.Printf("❌ 订阅失败 %s: %v", symbol, err)
		} else {
			log.Printf("✅ 成功订阅: %s", symbol)
		}
	}

	// 等待数据流建立
	log.Println("⏳ 等待数据流建立...")
	time.Sleep(10 * time.Second)

	// 3. 演示数据获取
	demonstrateDataRetrieval(ofm, symbols)

	// 4. 演示AI集成
	demonstrateAIIntegration(ofm, symbols)

	// 5. 演示健康检查
	demonstrateHealthCheck()

	// 6. 演示系统状态
	demonstrateSystemStatus(ofm)

	// 持续运行并定期输出数据
	log.Println("🔄 开始持续监控...")
	runContinuousMonitoring(ofm, symbols)
}

// demonstrateDataRetrieval 演示数据获取
func demonstrateDataRetrieval(ofm *microstructure.OrderFlowManager, symbols []string) {
	log.Println("\n📊 === 演示数据获取 ===")
	
	for _, symbol := range symbols {
		snapshot := ofm.GetMarketSnapshot(symbol)
		if snapshot != nil {
			log.Printf("\n💰 [%s] 订单流快照:", symbol)
			log.Printf("  现货CVD: %.0f万USD", snapshot.CVDData.SpotCVD1H/10000)
			log.Printf("  合约CVD: %.0f万USD", snapshot.CVDData.FuturesCVD1H/10000)
			log.Printf("  OI变化: %+.2f%%", snapshot.OIAnalysis.ChangeRate1H)
			log.Printf("  买卖失衡: %.3f", snapshot.OrderBookData.ImbalanceRatio)
			log.Printf("  市场状态: %s", snapshot.MarketContext.GameMatrix.MatrixType)
			log.Printf("  信号强度: %.0f/100", snapshot.MarketContext.SignalStrength)
			
			if snapshot.OrderBookData.NearestResistance != nil {
				log.Printf("  阻力墙: $%.2f (%.0f万USD)", 
					snapshot.OrderBookData.NearestResistance.Price,
					snapshot.OrderBookData.NearestResistance.StrengthUSD/10000)
			}
		} else {
			log.Printf("⚠️ [%s] 暂无订单流数据", symbol)
		}
	}
}

// demonstrateAIIntegration 演示AI集成
func demonstrateAIIntegration(ofm *microstructure.OrderFlowManager, symbols []string) {
	log.Println("\n🤖 === 演示AI集成格式 ===")
	
	for _, symbol := range symbols {
		snapshot := ofm.GetMarketSnapshot(symbol)
		if snapshot != nil {
			// 生成AI可读的摘要
			summary := microstructure.GenerateOrderFlowSummary(snapshot)
			log.Printf("\n📝 [%s] AI摘要:\n%s", symbol, summary)
			
			// 生成JSON格式供AI使用
			aiPayload := microstructure.FormatOrderFlowForAI(snapshot)
			jsonData, _ := json.MarshalIndent(aiPayload, "", "  ")
			log.Printf("\n📋 [%s] AI JSON Payload:\n%s", symbol, string(jsonData))
			break // 只演示第一个币种的完整格式
		}
	}
}

// demonstrateHealthCheck 演示健康检查
func demonstrateHealthCheck() {
	log.Println("\n🏥 === 演示健康检查 ===")
	
	healthCheck := microstructure.PerformHealthCheck()
	
	log.Printf("系统健康状态: %v", healthCheck.IsHealthy)
	log.Printf("连接币种数: %d", healthCheck.ConnectedSymbols)
	log.Printf("错误计数: %d", healthCheck.ErrorCount)
	
	// 显示WebSocket连接状态
	log.Println("WebSocket连接状态:")
	for stream, connected := range healthCheck.WebSocketStatus {
		status := "❌ 断开"
		if connected {
			status = "✅ 连接"
		}
		log.Printf("  %s: %s", stream, status)
	}
}

// demonstrateSystemStatus 演示系统状态
func demonstrateSystemStatus(ofm *microstructure.OrderFlowManager) {
	log.Println("\n⚙️ === 演示系统状态 ===")
	
	status := ofm.GetStatus()
	log.Printf("运行状态: %v", status["is_running"])
	log.Printf("订阅币种: %v", status["symbols"])
	
	if components, ok := status["components"].(map[string]interface{}); ok {
		log.Println("组件状态:")
		log.Printf("  CVD计算器: %v", components["cvd_manager"])
		log.Printf("  OI计算器: %v", components["oi_manager"])
		log.Printf("  盘口分析器: %v", components["orderbook_manager"])
	}
}

// runContinuousMonitoring 运行持续监控
func runContinuousMonitoring(ofm *microstructure.OrderFlowManager, symbols []string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	count := 0
	maxIterations := 10 // 限制演示次数
	
	for range ticker.C {
		count++
		if count > maxIterations {
			log.Println("📝 演示完成，退出监控")
			break
		}
		
		log.Printf("\n🔍 === 第%d次监控检查 ===", count)
		
		for _, symbol := range symbols {
			snapshot := ofm.GetMarketSnapshot(symbol)
			if snapshot != nil && !snapshot.CVDData.IsStale {
				// 检测重要信号
				if snapshot.MarketContext.CVDDivergence {
					log.Printf("🚨 [%s] 检测到CVD背离信号！", symbol)
				}
				
				if snapshot.MarketContext.SignalStrength > 80 {
					log.Printf("🎯 [%s] 高强度信号: %.0f/100 (%s)", 
						symbol, 
						snapshot.MarketContext.SignalStrength,
						snapshot.MarketContext.GameMatrix.MatrixType)
				}
				
				// 检测极端CVD
				totalCVD := snapshot.CVDData.SpotCVD1H + snapshot.CVDData.FuturesCVD1H
				if abs(totalCVD) > 5000000 { // 500万USD
					direction := "流出"
					if totalCVD > 0 {
						direction = "流入"
					}
					log.Printf("💰 [%s] 极端资金%s: %.0f万USD", symbol, direction, abs(totalCVD)/10000)
				}
			}
		}
		
		// 定期输出系统指标
		if count%3 == 0 {
			metrics := microstructure.GetOrderFlowMetrics()
			log.Printf("📈 系统指标更新: %v", metrics["uptime_status"])
		}
	}
}

// abs 绝对值函数
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ===== 集成示例函数 =====

// ExampleIntegrateWithExistingSystem 演示如何与现有交易系统集成
func ExampleIntegrateWithExistingSystem() {
	log.Println("\n🔗 === 与现有系统集成示例 ===")
	
	// 1. 启动订单流系统
	if err := microstructure.StartOrderFlowSystem(true, true); err != nil {
		log.Printf("❌ 启动订单流系统失败: %v", err)
		return
	}
	defer microstructure.StopOrderFlowSystem()
	
	// 2. 初始化币种
	symbols := []string{"BTCUSDT", "ETHUSDT"}
	microstructure.InitOrderFlowForSymbols(symbols)
	
	// 3. 在决策时获取订单流数据
	for _, symbol := range symbols {
		orderFlowData := microstructure.GetOrderFlowDataForSymbol(symbol)
		
		// 这些数据可以直接注入到AI的prompt中
		log.Printf("📊 [%s] 订单流数据已准备，可注入AI决策", symbol)
		
		// 模拟AI决策过程
		if orderFlowData != nil {
			log.Printf("✅ [%s] 订单流数据成功集成到决策流程", symbol)
		}
	}
}

// ExampleDebugAndTroubleshoot 演示调试和故障排除
func ExampleDebugAndTroubleshoot() {
	log.Println("\n🐛 === 调试和故障排除示例 ===")
	
	// 1. 输出详细快照（用于调试）
	microstructure.DumpOrderFlowSnapshot("BTCUSDT")
	
	// 2. 检查系统健康状态
	healthCheck := microstructure.PerformHealthCheck()
	if !healthCheck.IsHealthy {
		log.Printf("⚠️ 系统健康状态异常:")
		log.Printf("  错误数量: %d", healthCheck.ErrorCount)
		
		// 检查具体问题
		for symbol, freshness := range healthCheck.DataFreshness {
			if data, ok := freshness.(map[string]interface{}); ok {
				if stale, ok := data["stale"].(bool); ok && stale {
					log.Printf("  📊 [%s] 数据过期", symbol)
				}
			}
		}
	}
	
	// 3. 获取详细指标
	metrics := microstructure.GetOrderFlowMetrics()
	log.Printf("📊 详细系统指标: %+v", metrics)
}