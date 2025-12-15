package main

import (
	"fmt"
	"math"
)

// 修复后的calculateTrendLength - 返回绝对价格宽度
func calculateTrendLength(startPrice, endPrice float64) float64 {
	return math.Abs(endPrice - startPrice)
}

// CalculateWidthATR - 期望输入绝对宽度
func calculateWidthATR(width, atr float64) float64 {
	if atr == 0 {
		return 0
	}
	return width / atr
}

func main() {
	fmt.Printf("=== 修复验证测试 ===\n\n")

	testCases := []struct {
		name      string
		startPrice float64
		endPrice   float64
		atr        float64
	}{
		{"BTC小幅回调", 50000, 51000, 800},
		{"BTC中等回调", 50000, 52500, 800},
		{"BTC大幅回调", 50000, 55000, 800},
		{"ETH小幅回调", 3000, 3150, 60},
		{"山寨币大幅波动", 10, 15, 0.5},
	}

	for _, tc := range testCases {
		trendLength := calculateTrendLength(tc.startPrice, tc.endPrice)
		widthATR := calculateWidthATR(trendLength, tc.atr)
		
		fmt.Printf("%s:\n", tc.name)
		fmt.Printf("  价格: $%.0f → $%.0f\n", tc.startPrice, tc.endPrice)
		fmt.Printf("  绝对宽度: $%.0f\n", trendLength)
		fmt.Printf("  ATR: $%.1f\n", tc.atr)
		fmt.Printf("  widthATR: %.2f倍 (合理指标)\n\n", widthATR)
	}

	fmt.Printf("📈 意义解读:\n")
	fmt.Printf("- widthATR < 1.0: 趋势小于1倍ATR，相对较小\n")
	fmt.Printf("- widthATR 1.0-2.0: 正常趋势范围\n")
	fmt.Printf("- widthATR > 2.0: 大幅趋势，值得关注\n")
	fmt.Printf("- widthATR > 5.0: 极端趋势，高度关注\n")
}