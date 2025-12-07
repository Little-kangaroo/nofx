package main

import (
	"fmt"
	"nofx/microstructure"
	"time"
)

func main() {
	fmt.Println("=== 测试cleanupExpiredData的bug ===")
	
	// 创建OI计算器，窗口期设置为1小时（很短）
	calc := microstructure.NewOICalculator("BTCUSDT", 1*time.Hour)
	
	// 模拟添加一些旧数据
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour) // 2小时前的数据，应该被清理
	
	// 手动添加一些过期数据到changes数组
	// 注意：这里我们需要直接访问calc的内部结构进行测试
	// 通常这需要添加测试方法或者暴露内部状态
	
	// 先模拟正常的数据处理流程
	oiData1 := &microstructure.OIData{
		Symbol:        "BTCUSDT",
		OpenInterest: 1000,
		Timestamp:    oldTime,
	}
	
	oiData2 := &microstructure.OIData{
		Symbol:        "BTCUSDT", 
		OpenInterest: 1100,
		Timestamp:    oldTime.Add(10 * time.Minute),
	}
	
	oiData3 := &microstructure.OIData{
		Symbol:        "BTCUSDT",
		OpenInterest: 1200, 
		Timestamp:    oldTime.Add(20 * time.Minute),
	}
	
	fmt.Printf("窗口期: 1小时\n")
	fmt.Printf("当前时间: %v\n", now.Format("15:04:05"))
	fmt.Printf("截止时间(1小时前): %v\n", now.Add(-1*time.Hour).Format("15:04:05"))
	
	// 处理数据
	calc.ProcessOIData(oiData1)
	fmt.Printf("添加数据1 (2小时前): %v\n", oiData1.Timestamp.Format("15:04:05"))
	
	calc.ProcessOIData(oiData2) 
	fmt.Printf("添加数据2 (1小时50分前): %v\n", oiData2.Timestamp.Format("15:04:05"))
	
	calc.ProcessOIData(oiData3)
	fmt.Printf("添加数据3 (1小时40分前): %v\n", oiData3.Timestamp.Format("15:04:05"))
	
	// 检查清理后的状态
	analysis := calc.GetOIAnalysis()
	fmt.Printf("\n清理后分析结果:\n")
	fmt.Printf("IsStale: %v\n", analysis.IsStale)
	fmt.Printf("LastUpdate: %v\n", analysis.LastUpdate.Format("15:04:05"))
	
	// 由于所有数据都超过1小时，changes数组应该被清空
	// 但由于bug，可能没有被清空，导致len(calc.changes) == 0 条件不成立
}