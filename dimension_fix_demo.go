package main

import (
	"fmt"
	"math"
)

// 模拟当前有问题的代码
func calculateTrendLengthCurrent(startPrice, endPrice float64) float64 {
	if startPrice == 0 {
		return 0
	}
	return (endPrice - startPrice) / startPrice * 100 // 返回百分比
}

// 修复后的代码 - 返回绝对价格宽度
func calculateTrendLengthFixed(startPrice, endPrice float64) float64 {
	return math.Abs(endPrice - startPrice) // 返回绝对价格差
}

// CalculateWidthATR - 期望输入绝对宽度
func calculateWidthATR(width, atr float64) float64 {
	if atr == 0 {
		return 0
	}
	return width / atr
}

func main() {
	// 测试场景：BTC从$50000涨到$52000的斐波纳契回调
	startPrice := 50000.0
	endPrice := 52000.0
	atr := 800.0 // 假设ATR是$800

	fmt.Printf("=== 维度不一致问题演示 ===\n")
	fmt.Printf("价格从 $%.0f 涨到 $%.0f\n", startPrice, endPrice)
	fmt.Printf("ATR: $%.0f\n\n", atr)

	// 当前有问题的计算
	currentTrendLength := calculateTrendLengthCurrent(startPrice, endPrice)
	currentWidthATR := calculateWidthATR(currentTrendLength, atr)

	fmt.Printf("❌ 修复前（维度不一致）:\n")
	fmt.Printf("  趋势长度: %.2f%% (百分比)\n", currentTrendLength)
	fmt.Printf("  widthATR: %.6f (百分比/价格 = 无意义单位)\n", currentWidthATR)

	// 修复后的正确计算
	fixedTrendLength := calculateTrendLengthFixed(startPrice, endPrice)
	fixedWidthATR := calculateWidthATR(fixedTrendLength, atr)

	fmt.Printf("\n✅ 修复后（维度一致）:\n")
	fmt.Printf("  趋势长度: $%.0f (绝对价格宽度)\n", fixedTrendLength)
	fmt.Printf("  widthATR: %.2f (ATR倍数，有意义的指标)\n", fixedWidthATR)

	// 展示问题的严重性
	fmt.Printf("\n🚨 问题严重性对比:\n")
	fmt.Printf("  错误widthATR: %.6f (被缩小约%.0f倍)\n",
		currentWidthATR, fixedWidthATR/currentWidthATR)
	fmt.Printf("  正确widthATR: %.2f (合理的2.5倍ATR)\n", fixedWidthATR)

	// 模拟对过滤逻辑的影响
	fmt.Printf("\n📊 对过滤逻辑的影响:\n")

	// 假设过滤条件是 widthATR > 1.0（至少1倍ATR才有意义）
	filterThreshold := 1.0
	fmt.Printf("  过滤条件: widthATR > %.1f\n", filterThreshold)
	fmt.Printf("  错误计算结果: %.6f > %.1f = %v (错误地被过滤)\n",
		currentWidthATR, filterThreshold, currentWidthATR > filterThreshold)
	fmt.Printf("  正确计算结果: %.2f > %.1f = %v (正确地通过过滤)\n",
		fixedWidthATR, filterThreshold, fixedWidthATR > filterThreshold)
}
