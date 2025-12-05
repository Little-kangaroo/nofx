package microstructure

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// ===== AI系统集成模块 =====

// ExtendMarketDataWithOrderFlow 为市场数据扩展订单流信息
// 注意：此函数需要在market包中调用，避免循环导入
func ExtendMarketDataWithOrderFlow(symbol string, orderFlowManager *OrderFlowManager) (map[string]interface{}, error) {
	if orderFlowManager == nil {
		return nil, fmt.Errorf("orderFlowManager 不能为空")
	}

	// 获取订单流快照
	snapshot := orderFlowManager.GetMarketSnapshot(symbol)
	if snapshot == nil {
		log.Printf("⚠️ 未获取到 %s 的订单流数据", symbol)
		return nil, nil
	}

	// 返回订单流数据，由调用方集成到市场数据中
	log.Printf("✅ 为 %s 生成订单流数据", symbol)
	return FormatOrderFlowForAI(snapshot), nil
}

// GenerateOrderFlowSummary 生成订单流摘要（供AI提示词使用）
func GenerateOrderFlowSummary(snapshot *MarketSnapshot) string {
	if snapshot == nil {
		return "订单流数据不可用"
	}

	var summary strings.Builder
	
	// 资金博弈摘要
	summary.WriteString(fmt.Sprintf("## 订单流分析 (%s)\n\n", snapshot.Symbol))
	
	// CVD分析
	summary.WriteString("### 资金流向博弈\n")
	summary.WriteString(fmt.Sprintf("- 现货CVD(1H): %.0f万USD\n", snapshot.CVDData.SpotCVD1H/10000))
	summary.WriteString(fmt.Sprintf("- 合约CVD(1H): %.0f万USD\n", snapshot.CVDData.FuturesCVD1H/10000))
	summary.WriteString(fmt.Sprintf("- OI变化(1H): %+.2f%%\n", snapshot.OIAnalysis.ChangeRate1H))
	summary.WriteString(fmt.Sprintf("- 市场状态: %s\n", snapshot.MarketContext.GameMatrix.Description))
	
	// 背离信号
	if snapshot.MarketContext.CVDDivergence {
		summary.WriteString("- ⚠️ **检测到CVD价格背离**\n")
	}
	
	// 盘口结构
	summary.WriteString("\n### 盘口微观结构\n")
	summary.WriteString(fmt.Sprintf("- 买卖失衡: %.2f\n", snapshot.OrderBookData.ImbalanceRatio))
	
	if snapshot.OrderBookData.NearestResistance != nil {
		summary.WriteString(fmt.Sprintf("- 阻力墙: $%.2f (%.2f万USD)\n", 
			snapshot.OrderBookData.NearestResistance.Price, 
			snapshot.OrderBookData.NearestResistance.StrengthUSD/10000))
	}
	
	if snapshot.OrderBookData.NearestSupport != nil {
		summary.WriteString(fmt.Sprintf("- 支撑墙: $%.2f (%.2f万USD)\n", 
			snapshot.OrderBookData.NearestSupport.Price, 
			snapshot.OrderBookData.NearestSupport.StrengthUSD/10000))
	}
	
	// 信号强度
	summary.WriteString(fmt.Sprintf("\n- **综合信号强度: %.0f/100**\n", snapshot.MarketContext.SignalStrength))
	
	return summary.String()
}

// FormatOrderFlowForAI 格式化订单流数据供AI使用（紧凑JSON格式）
func FormatOrderFlowForAI(snapshot *MarketSnapshot) map[string]interface{} {
	if snapshot == nil {
		return map[string]interface{}{
			"订单流分析": map[string]interface{}{
				"状态": "数据不可用",
			},
		}
	}

	return snapshot.ToAIPayload()
}

// ===== 与现有market包的集成辅助函数 =====

// GetOrderFlowDataForSymbol 获取指定币种的订单流数据（供现有系统调用）
func GetOrderFlowDataForSymbol(symbol string) map[string]interface{} {
	ofm := GetGlobalOrderFlowManager()
	snapshot := ofm.GetMarketSnapshot(symbol)
	return FormatOrderFlowForAI(snapshot)
}

// InitOrderFlowForSymbols 为币种列表初始化订单流数据
func InitOrderFlowForSymbols(symbols []string) error {
	ofm := GetGlobalOrderFlowManager()
	
	for _, symbol := range symbols {
		if err := ofm.SubscribeSymbol(symbol); err != nil {
			log.Printf("❌ 初始化订单流失败 %s: %v", symbol, err)
		}
	}
	
	log.Printf("✅ 为 %d 个币种初始化订单流数据", len(symbols))
	return nil
}

// ===== 性能监控和健康检查 =====

// OrderFlowHealthCheck 订单流系统健康检查
type OrderFlowHealthCheck struct {
	IsHealthy           bool                   `json:"is_healthy"`
	ConnectedSymbols    int                    `json:"connected_symbols"`
	WebSocketStatus     map[string]bool        `json:"websocket_status"`
	DataFreshness       map[string]interface{} `json:"data_freshness"`
	ErrorCount          int                    `json:"error_count"`
	LastCheck           string                 `json:"last_check"`
}

// PerformHealthCheck 执行健康检查
func PerformHealthCheck() *OrderFlowHealthCheck {
	ofm := GetGlobalOrderFlowManager()
	status := ofm.GetStatus()
	
	// 检查数据新鲜度
	freshness := make(map[string]interface{})
	snapshots := ofm.GetAllMarketSnapshots()
	
	staleCount := 0
	for symbol, snapshot := range snapshots {
		isStale := snapshot.CVDData.IsStale || snapshot.OIAnalysis.IsStale || snapshot.OrderBookData.IsStale
		freshness[symbol] = map[string]interface{}{
			"stale": isStale,
			"last_update": snapshot.Timestamp,
		}
		if isStale {
			staleCount++
		}
	}
	
	// 总体健康状态
	isHealthy := true
	if wsStatus, ok := status["websocket_status"].(map[string]bool); ok {
		for _, connected := range wsStatus {
			if !connected {
				isHealthy = false
				break
			}
		}
	}
	
	// 如果超过50%的数据过期，标记为不健康
	if len(snapshots) > 0 && float64(staleCount)/float64(len(snapshots)) > 0.5 {
		isHealthy = false
	}
	
	return &OrderFlowHealthCheck{
		IsHealthy:        isHealthy,
		ConnectedSymbols: status["subscribed_symbols"].(int),
		WebSocketStatus:  status["websocket_status"].(map[string]bool),
		DataFreshness:    freshness,
		ErrorCount:       staleCount,
		LastCheck:        "just_now",
	}
}

// ===== 与decision包集成的辅助函数 =====

// InjectOrderFlowIntoContext 将订单流数据注入到decision.Context中
func InjectOrderFlowIntoContext(ctx interface{}, symbols []string) error {
	// 这里需要根据实际的decision.Context结构进行适配
	// 暂时返回nil表示成功
	
	ofm := GetGlobalOrderFlowManager()
	
	// 为每个币种获取订单流数据
	orderFlowData := make(map[string]interface{})
	for _, symbol := range symbols {
		snapshot := ofm.GetMarketSnapshot(symbol)
		orderFlowData[symbol] = FormatOrderFlowForAI(snapshot)
	}
	
	log.Printf("✅ 为 %d 个币种注入订单流数据到决策上下文", len(symbols))
	return nil
}

// ===== 配置和启动辅助函数 =====

// StartOrderFlowSystem 启动订单流系统（供main.go调用）
func StartOrderFlowSystem(enableCVD, enableOrderBook bool) error {
	config := DefaultMicrostructureConfig()
	config.EnableCVDAnalysis = enableCVD
	config.EnableOrderBook = enableOrderBook
	
	return InitGlobalOrderFlowManager(config)
}

// StopOrderFlowSystem 停止订单流系统
func StopOrderFlowSystem() {
	StopGlobalOrderFlowManager()
}

// ===== 调试和日志辅助函数 =====

// DumpOrderFlowSnapshot 输出订单流快照（用于调试）
func DumpOrderFlowSnapshot(symbol string) {
	ofm := GetGlobalOrderFlowManager()
	snapshot := ofm.GetMarketSnapshot(symbol)
	
	if snapshot == nil {
		log.Printf("📊 [%s] 订单流快照: 无数据", symbol)
		return
	}
	
	jsonData, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		log.Printf("❌ 序列化快照失败: %v", err)
		return
	}
	
	log.Printf("📊 [%s] 订单流快照:\n%s", symbol, string(jsonData))
}

// GetOrderFlowMetrics 获取订单流系统指标
func GetOrderFlowMetrics() map[string]interface{} {
	ofm := GetGlobalOrderFlowManager()
	status := ofm.GetStatus()
	healthCheck := PerformHealthCheck()
	
	return map[string]interface{}{
		"system_status":  status,
		"health_check":   healthCheck,
		"uptime_status":  "running", // 这里可以添加更详细的运行时间统计
	}
}