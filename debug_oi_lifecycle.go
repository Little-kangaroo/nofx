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
	symbol := "BTCUSDT"
	err = ofm.SubscribeSymbol(symbol)
	if err != nil {
		log.Printf("订阅失败: %v", err)
	}
	
	fmt.Println("=== 详细监控OI数据的生命周期 ===")
	
	startTime := time.Now()
	lastOIUpdate := time.Time{}
	consecutiveStaleCount := 0
	
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		elapsed := time.Since(startTime)
		snapshot := ofm.GetMarketSnapshot(symbol)
		
		if snapshot == nil {
			fmt.Printf("[%02d分%02d秒] ❌ Snapshot为nil\n", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
			continue
		}
		
		// 检查OI数据的更新情况
		currentOIUpdate := snapshot.OIAnalysis.LastUpdate
		isNewUpdate := !currentOIUpdate.Equal(lastOIUpdate)
		
		if isNewUpdate {
			lastOIUpdate = currentOIUpdate
			consecutiveStaleCount = 0
			fmt.Printf("[%02d分%02d秒] 🔄 OI数据有新更新: %v\n", 
				int(elapsed.Minutes()), int(elapsed.Seconds())%60,
				currentOIUpdate.Format("15:04:05"))
		}
		
		// 检查IsStale状态
		if snapshot.OIAnalysis.IsStale {
			consecutiveStaleCount++
			fmt.Printf("[%02d分%02d秒] ❌ OI数据过期 (第%d次连续): LastUpdate=%v, 距现在=%v\n", 
				int(elapsed.Minutes()), int(elapsed.Seconds())%60,
				consecutiveStaleCount,
				snapshot.OIAnalysis.LastUpdate.Format("15:04:05"), 
				time.Since(snapshot.OIAnalysis.LastUpdate))
			
			// 如果连续3次过期，说明问题持续存在
			if consecutiveStaleCount >= 3 {
				fmt.Printf("🚨 OI数据持续过期，可能是15分钟问题的表现\n")
			}
		} else {
			if consecutiveStaleCount > 0 {
				fmt.Printf("[%02d分%02d秒] ✅ OI数据恢复正常 (之前连续过期%d次)\n", 
					int(elapsed.Minutes()), int(elapsed.Seconds())%60, consecutiveStaleCount)
			}
			consecutiveStaleCount = 0
		}
		
		// 显示基本状态
		fmt.Printf("[%02d分%02d秒] 状态: CVD=%v, OI=%v, OrderBook=%v\n", 
			int(elapsed.Minutes()), int(elapsed.Seconds())%60,
			map[bool]string{true: "过期", false: "正常"}[snapshot.CVDData.IsStale],
			map[bool]string{true: "过期", false: "正常"}[snapshot.OIAnalysis.IsStale],
			map[bool]string{true: "过期", false: "正常"}[snapshot.OrderBookData.IsStale])
		
		// 监控20分钟
		if elapsed > 20*time.Minute {
			break
		}
	}
	
	microstructure.StopGlobalOrderFlowManager()
}