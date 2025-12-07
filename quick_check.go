package main

import (
	"fmt"
	"nofx/market"
	"nofx/microstructure"
	"time"
)

func main() {
	// 快速测试：初始化系统后立即检查15分钟后的状态
	config := microstructure.DefaultMicrostructureConfig()
	microstructure.InitGlobalOrderFlowManager(config)
	
	ofm := microstructure.GetGlobalOrderFlowManager()
	ofm.SubscribeSymbol("BTCUSDT")
	
	fmt.Println("系统已启动，等待5秒让数据稳定...")
	time.Sleep(5 * time.Second)
	
	// 检查当前状态
	fmt.Println("\n=== 检查当前数据状态 ===")
	result := market.GetOrderFlowDataForAIV2("BTCUSDT")
	
	if status, exists := result["状态"]; exists {
		fmt.Printf("数据状态: %v\n", status)
	} else {
		fmt.Println("数据状态: 正常 (无状态字段)")
	}
	
	// 检查数据质量
	if dataQuality, exists := result["数据质量"]; exists {
		if quality, ok := dataQuality.(map[string]interface{}); ok {
			if status, ok := quality["status"]; ok {
				fmt.Printf("数据质量状态: %v\n", status)
			}
		}
	}
	
	// 检查具体组件状态
	snapshot := ofm.GetMarketSnapshot("BTCUSDT")
	if snapshot != nil {
		fmt.Printf("\nCVD IsStale: %v\n", snapshot.CVDData.IsStale)
		fmt.Printf("OI IsStale: %v\n", snapshot.OIAnalysis.IsStale) 
		fmt.Printf("OrderBook IsStale: %v\n", snapshot.OrderBookData.IsStale)
		
		fmt.Printf("\nOI LastUpdate: %v (距现在: %v)\n", 
			snapshot.OIAnalysis.LastUpdate.Format("15:04:05"), 
			time.Since(snapshot.OIAnalysis.LastUpdate))
		fmt.Printf("OrderBook LastUpdate: %v (距现在: %v)\n", 
			snapshot.OrderBookData.LastUpdate.Format("15:04:05"), 
			time.Since(snapshot.OrderBookData.LastUpdate))
	}
	
	microstructure.StopGlobalOrderFlowManager()
	fmt.Println("\n测试完成")
}