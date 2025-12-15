package main

import (
	"fmt"
)

// 演示当前有问题的逻辑
func analyzeTouchEventsCurrent(low, high float64, priceAfter, priceAtTouch float64) string {
	// 🚨 原始有问题的逻辑
	if abs(priceAfter-priceAtTouch)/priceAtTouch > 0.01 {
		// 🚨 关键问题：low < high 永远为真（因为 low 总是小于 high）
		if (priceAfter > priceAtTouch && low < high) || (priceAfter < priceAtTouch && low > high) {
			return "ReactionBounce"
		} else {
			return "ReactionBreak"
		}
	}
	return "ReactionConsolidation"
}

// 修复后的逻辑
func analyzeTouchEventsFixed(low, high float64, priceAfter, priceAtTouch, trendDirection float64, touchDirection string) string {
	// 判断价格变化是否足够显著（1%阈值）
	priceChangePercent := abs(priceAfter-priceAtTouch) / priceAtTouch
	if priceChangePercent <= 0.01 {
		return "ReactionConsolidation" // 价格变化不大，视为整固
	}
	
	// 判断价格反应方向
	isPriceUp := priceAfter > priceAtTouch
	isTrendUpward := trendDirection > 0
	
	// 基于触碰方向和趋势方向判断反应类型
	switch touchDirection {
	case "回撤触及": // 回撤到黄金口袋
		if isTrendUpward {
			// 上升趋势中的回撤触及
			if isPriceUp {
				return "ReactionBounce" // 回撤后反弹，符合预期
			} else {
				return "ReactionBreak" // 回撤后继续下跌，突破失败
			}
		} else {
			// 下降趋势中的回撤触及  
			if !isPriceUp {
				return "ReactionBounce" // 回撤后继续下跌，符合预期
			} else {
				return "ReactionBreak" // 回撤后反弹，突破失败
			}
		}
		
	case "反弹触及": // 反弹到黄金口袋
		if isTrendUpward {
			// 上升趋势中的反弹触及
			if isPriceUp {
				return "ReactionBreak" // 反弹后继续上涨，突破口袋
			} else {
				return "ReactionBounce" // 反弹后回落，口袋阻力有效
			}
		} else {
			// 下降趋势中的反弹触及
			if !isPriceUp {
				return "ReactionBreak" // 反弹后继续下跌，突破口袋
			} else {
				return "ReactionBounce" // 反弹后上涨，口袋支撑有效
			}
		}
	}
	
	return "ReactionConsolidation"
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func main() {
	fmt.Printf("=== P0-A3 Golden Pocket 触碰反应分类逻辑修复演示 ===\\n\\n")
	
	// 测试场景设置
	low := 50000.0   // 黄金口袋下边界
	high := 50800.0  // 黄金口袋上边界
	
	testCases := []struct{
		name string
		priceAtTouch float64
		priceAfter float64
		trendDirection float64 // >0 上升趋势，<0 下降趋势
		touchDirection string
		expectedCorrect string
	}{
		{"上升趋势回撤反弹", 50400.0, 51200.0, 1.0, "回撤触及", "ReactionBounce"},
		{"上升趋势回撤失败", 50400.0, 49800.0, 1.0, "回撤触及", "ReactionBreak"},
		{"下降趋势回撤延续", 50400.0, 49600.0, -1.0, "回撤触及", "ReactionBounce"},
		{"下降趋势回撤失败", 50400.0, 51000.0, -1.0, "回撤触及", "ReactionBreak"},
		{"上升趋势反弹被阻", 50400.0, 50000.0, 1.0, "反弹触及", "ReactionBounce"},
		{"上升趋势反弹突破", 50400.0, 51500.0, 1.0, "反弹触及", "ReactionBreak"},
	}
	
	fmt.Printf("黄金口袋范围: $%.0f - $%.0f\\n\\n", low, high)
	
	for _, tc := range testCases {
		// 当前有问题的逻辑结果
		currentResult := analyzeTouchEventsCurrent(low, high, tc.priceAfter, tc.priceAtTouch)
		
		// 修复后的逻辑结果
		fixedResult := analyzeTouchEventsFixed(low, high, tc.priceAfter, tc.priceAtTouch, tc.trendDirection, tc.touchDirection)
		
		// 判断分类正确性
		isCurrentCorrect := currentResult == tc.expectedCorrect
		isFixedCorrect := fixedResult == tc.expectedCorrect
		
		fmt.Printf("📋 %s:\\n", tc.name)
		fmt.Printf("  触碰价格: $%.0f → 后续价格: $%.0f\\n", tc.priceAtTouch, tc.priceAfter)
		fmt.Printf("  趋势方向: %s, 触碰方向: %s\\n", 
			map[bool]string{true: "上升", false: "下降"}[tc.trendDirection > 0],
			tc.touchDirection)
		
		fmt.Printf("  ❌ 修复前: %s %s\\n", 
			currentResult, 
			map[bool]string{true: "✓", false: "✗ 错误"}[isCurrentCorrect])
			
		fmt.Printf("  ✅ 修复后: %s %s\\n", 
			fixedResult,
			map[bool]string{true: "✓", false: "✗"}[isFixedCorrect])
			
		fmt.Printf("  📈 期望结果: %s\\n\\n", tc.expectedCorrect)
	}
	
	fmt.Printf("🚨 核心问题分析:\\n")
	fmt.Printf("1. 条件 'low < high' 永远为真，导致分类主要由 'priceAfter > priceAtTouch' 决定\\n")
	fmt.Printf("2. 缺少趋势方向和触碰方向的考虑\\n")
	fmt.Printf("3. 没有区分'回撤延续'vs'反转失败'的情况\\n\\n")
	
	fmt.Printf("🎯 修复方案:\\n")
	fmt.Printf("1. 移除永真/永假条件\\n")
	fmt.Printf("2. 基于趋势方向(上升/下降)判断\\n")
	fmt.Printf("3. 基于触碰方向(回撤/反弹)判断\\n")
	fmt.Printf("4. 结合价格后续行为进行综合分类\\n")
}