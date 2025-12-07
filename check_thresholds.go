package main

import (
	"fmt"
	"nofx/microstructure"
	"time"
)

func main() {
	fmt.Println("=== 检查当前过期阈值设置 ===")
	
	// 创建测试OI计算器
	calc := microstructure.NewOICalculator("BTCUSDT", 24*time.Hour)
	
	// 设置一个旧的lastUpdate时间
	calc.SetLastUpdateForTest(time.Now().Add(-20 * time.Minute))
	
	// 获取分析结果
	analysis := calc.GetOIAnalysis()
	
	fmt.Printf("20分钟前的数据是否过期: %v\n", analysis.IsStale)
	
	// 测试16分钟前的数据
	calc.SetLastUpdateForTest(time.Now().Add(-16 * time.Minute))
	analysis = calc.GetOIAnalysis()
	fmt.Printf("16分钟前的数据是否过期: %v\n", analysis.IsStale)
	
	// 测试14分钟前的数据
	calc.SetLastUpdateForTest(time.Now().Add(-14 * time.Minute))
	analysis = calc.GetOIAnalysis()
	fmt.Printf("14分钟前的数据是否过期: %v\n", analysis.IsStale)
}