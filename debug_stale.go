package main

import (
	"fmt"
	"log"
	"nofx/microstructure"
	"time"
)

func main() {
	// 初始化系统
	config := microstructure.DefaultMicrostructureConfig()
	err := microstructure.InitGlobalOrderFlowManager(config)
	if err != nil {
		log.Fatalf("初始化订单流管理器失败: %v", err)
	}
	
	ofm := microstructure.GetGlobalOrderFlowManager()
	if ofm == nil {
		log.Fatal("获取全局订单流管理器失败")
	}
	
	// 订阅BTCUSDT
	symbol := "BTCUSDT"
	err = ofm.SubscribeSymbol(symbol)
	if err != nil {
		log.Printf("订阅失败: %v", err)
	}
	
	fmt.Println("=== 开始监控数据状态，重点观察15分钟前后 ===")
	
	startTime := time.Now()
	
	// 14分钟后开始密集检查
	time.Sleep(14 * time.Minute)
	fmt.Println("\n=== 进入关键观察期（14-18分钟）===")
	
	// 每30秒检查一次
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		elapsed := time.Since(startTime)
		snapshot := ofm.GetMarketSnapshot(symbol)
		
		if snapshot == nil {
			fmt.Printf("[%02d分%02d秒] Snapshot为nil\n", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
			continue
		}
		
		isDataStale := snapshot.CVDData.IsStale || snapshot.OIAnalysis.IsStale || snapshot.OrderBookData.IsStale
		
		fmt.Printf("\n[%02d分%02d秒] %s:\n", int(elapsed.Minutes()), int(elapsed.Seconds())%60, 
			map[bool]string{true: "*** 数据过期 ***", false: "数据正常"}[isDataStale])
		
		if snapshot.CVDData.IsStale {
			fmt.Printf("  ❌ CVD过期: LastUpdate=%v, 距现在=%v\n", 
				snapshot.CVDData.LastUpdate.Format("15:04:05"), 
				time.Since(snapshot.CVDData.LastUpdate))
		}
		if snapshot.OIAnalysis.IsStale {
			fmt.Printf("  ❌ OI过期: LastUpdate=%v, 距现在=%v\n", 
				snapshot.OIAnalysis.LastUpdate.Format("15:04:05"), 
				time.Since(snapshot.OIAnalysis.LastUpdate))
		}
		if snapshot.OrderBookData.IsStale {
			fmt.Printf("  ❌ OrderBook过期: LastUpdate=%v, 距现在=%v\n", 
				snapshot.OrderBookData.LastUpdate.Format("15:04:05"), 
				time.Since(snapshot.OrderBookData.LastUpdate))
		}
		
		if !isDataStale {
			fmt.Printf("  ✅ 所有数据正常\n")
		}
		
		// 18分钟后停止
		if elapsed > 18*time.Minute {
			break
		}
	}
	
	microstructure.StopGlobalOrderFlowManager()
}