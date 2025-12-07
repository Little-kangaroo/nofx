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
	
	fmt.Println("=== 监控OI数据changes数组长度变化 ===")
	
	startTime := time.Now()
	
	// 每30秒检查一次changes数组长度
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		elapsed := time.Since(startTime)
		snapshot := ofm.GetMarketSnapshot(symbol)
		
		if snapshot == nil {
			fmt.Printf("[%02d分%02d秒] Snapshot为nil\n", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
			continue
		}
		
		// 这里需要暴露OI计算器的changes数组长度进行调试
		fmt.Printf("[%02d分%02d秒] OI LastUpdate: %v (距现在: %v), IsStale: %v\n", 
			int(elapsed.Minutes()), int(elapsed.Seconds())%60,
			snapshot.OIAnalysis.LastUpdate.Format("15:04:05"), 
			time.Since(snapshot.OIAnalysis.LastUpdate),
			snapshot.OIAnalysis.IsStale)
		
		// 20分钟后停止
		if elapsed > 20*time.Minute {
			break
		}
	}
	
	microstructure.StopGlobalOrderFlowManager()
}