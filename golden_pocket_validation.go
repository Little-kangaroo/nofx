package main

import (
	"fmt"
	"math"
)

// 模拟K线数据结构
type Kline struct {
	High  float64
	Low   float64
	Open  float64
	Close float64
}

// 模拟趋势类型
type TrendType int
const (
	TrendUpward   TrendType = 1
	TrendDownward TrendType = -1
)

// 模拟反应类型
type ReactionType int
const (
	ReactionBounce ReactionType = iota
	ReactionBreak
	ReactionConsolidation
)

func (rt ReactionType) String() string {
	switch rt {
	case ReactionBounce:
		return "ReactionBounce"
	case ReactionBreak:
		return "ReactionBreak"
	case ReactionConsolidation:
		return "ReactionConsolidation"
	default:
		return "Unknown"
	}
}

// 修复后的分类逻辑
func classifyTouchReaction(candle Kline, klines []Kline, index int, goldenLow, goldenHigh float64, trendType TrendType) ReactionType {
	// 确保有足够的后续K线进行确认
	confirmationPeriod := 3 // 使用3根K线确认
	if index+confirmationPeriod >= len(klines) {
		return ReactionConsolidation
	}
	
	// 🎯 关键修复1：获取确认期间的价格数据
	priceAtTouch := candle.Close
	priceAfter := klines[index+confirmationPeriod].Close
	
	// 🎯 关键修复2：判断触碰方向（从前一根K线判断）
	var prevPrice float64
	if index > 0 {
		prevPrice = klines[index-1].Close
	} else {
		prevPrice = candle.Open
	}
	
	// 判断是回撤触及还是反弹触及
	isTouchFromAbove := prevPrice > (goldenHigh + goldenLow) / 2
	
	// 🎯 关键修复3：计算价格变化幅度和方向
	priceChangePercent := math.Abs(priceAfter - priceAtTouch) / priceAtTouch
	
	// 价格变化太小，判定为整固
	if priceChangePercent <= 0.01 {
		return ReactionConsolidation
	}
	
	// 🎯 关键修复4：基于趋势方向和触碰方向进行正确分类
	isPriceUp := priceAfter > priceAtTouch
	isTrendUpward := trendType == TrendUpward
	
	switch {
	case isTouchFromAbove: // 从上方回撤触及黄金口袋
		if isTrendUpward {
			// 上升趋势中的回撤触及
			if isPriceUp {
				return ReactionBounce // 回撤后反弹，正常延续
			} else {
				return ReactionBreak // 回撤后继续下跌，趋势失败
			}
		} else {
			// 下降趋势中的回撤触及
			if !isPriceUp {
				return ReactionBounce // 回撤后继续下跌，正常延续
			} else {
				return ReactionBreak // 回撤后反弹，趋势失败
			}
		}
		
	case !isTouchFromAbove: // 从下方反弹触及黄金口袋
		if isTrendUpward {
			// 上升趋势中的反弹触及
			if isPriceUp {
				return ReactionBreak // 反弹后突破口袋，继续上涨
			} else {
				return ReactionBounce // 反弹后被阻，口袋阻力有效
			}
		} else {
			// 下降趋势中的反弹触及
			if !isPriceUp {
				return ReactionBreak // 反弹后继续下跌，突破口袋
			} else {
				return ReactionBounce // 反弹后上涨，口袋支撑有效
			}
		}
	}
	
	return ReactionConsolidation
}

// 原有问题逻辑（用于对比）
func classifyTouchReactionOld(priceAfter, priceAtTouch, low, high float64) ReactionType {
	if math.Abs(priceAfter-priceAtTouch)/priceAtTouch > 0.01 {
		// 🚨 关键问题：low < high 永远为真
		if (priceAfter > priceAtTouch && low < high) || (priceAfter < priceAtTouch && low > high) {
			return ReactionBounce
		} else {
			return ReactionBreak
		}
	}
	return ReactionConsolidation
}

func main() {
	fmt.Printf("=== P0-A3 Golden Pocket 修复验证测试 ===\n\n")
	
	// 黄金口袋范围
	goldenLow := 50000.0
	goldenHigh := 50800.0
	
	// 测试场景：创建模拟K线数据
	testCases := []struct{
		name string
		scenario string
		klines []Kline
		touchIndex int
		trendType TrendType
		expectedReaction ReactionType
	}{
		{
			"上升趋势回撤反弹",
			"价格从上方回撤到黄金口袋，然后反弹继续上涨",
			[]Kline{
				{High: 52000, Low: 51800, Open: 51900, Close: 51900}, // 趋势前期
				{High: 51200, Low: 50300, Open: 51200, Close: 50400}, // 回撤触及黄金口袋 (触碰K线)
				{High: 50600, Low: 50200, Open: 50400, Close: 50500}, // 确认期 +1
				{High: 51000, Low: 50400, Open: 50500, Close: 50900}, // 确认期 +2 
				{High: 52000, Low: 50800, Open: 50900, Close: 51800}, // 确认期 +3 (反弹确认)
			},
			1, // touchIndex
			TrendUpward,
			ReactionBounce,
		},
		{
			"下降趋势回撤延续",
			"价格从上方回撤到黄金口袋，然后继续下跌",
			[]Kline{
				{High: 52000, Low: 51800, Open: 51900, Close: 51900}, // 趋势前期  
				{High: 51200, Low: 50300, Open: 51200, Close: 50400}, // 回撤触及黄金口袋
				{High: 50400, Low: 49800, Open: 50400, Close: 50000}, // 确认期 +1
				{High: 50200, Low: 49500, Open: 50000, Close: 49700}, // 确认期 +2
				{High: 49900, Low: 49300, Open: 49700, Close: 49500}, // 确认期 +3 (继续下跌确认)
			},
			1,
			TrendDownward,
			ReactionBounce, // 下降趋势中继续下跌是正常的"bounce"（延续）
		},
		{
			"上升趋势反弹阻力",
			"价格从下方反弹到黄金口袋，然后被阻力位阻止",
			[]Kline{
				{High: 49500, Low: 49200, Open: 49300, Close: 49300}, // 趋势前期
				{High: 50600, Low: 49800, Open: 49800, Close: 50400}, // 从下方反弹触及
				{High: 50500, Low: 50000, Open: 50400, Close: 50200}, // 确认期 +1
				{High: 50300, Low: 49900, Open: 50200, Close: 50000}, // 确认期 +2
				{High: 50100, Low: 49600, Open: 50000, Close: 49800}, // 确认期 +3 (被阻下跌)
			},
			1,
			TrendUpward,
			ReactionBounce, // 被阻力位阻止，口袋有效
		},
		{
			"上升趋势反弹突破",
			"价格从下方反弹到黄金口袋，然后突破继续上涨",
			[]Kline{
				{High: 49500, Low: 49200, Open: 49300, Close: 49300}, // 趋势前期
				{High: 50600, Low: 49800, Open: 49800, Close: 50400}, // 从下方反弹触及
				{High: 51000, Low: 50300, Open: 50400, Close: 50800}, // 确认期 +1
				{High: 51500, Low: 50700, Open: 50800, Close: 51200}, // 确认期 +2
				{High: 52000, Low: 51000, Open: 51200, Close: 51800}, // 确认期 +3 (突破上涨)
			},
			1,
			TrendUpward,
			ReactionBreak, // 突破口袋继续上涨
		},
	}
	
	fmt.Printf("黄金口袋范围: $%.0f - $%.0f\n\n", goldenLow, goldenHigh)
	
	for i, tc := range testCases {
		fmt.Printf("🧪 测试 %d: %s\n", i+1, tc.name)
		fmt.Printf("📋 场景: %s\n", tc.scenario)
		
		// 获取触碰K线
		touchCandle := tc.klines[tc.touchIndex]
		priceAtTouch := touchCandle.Close
		priceAfter := tc.klines[tc.touchIndex+3].Close
		
		fmt.Printf("📊 数据: 触碰价格 $%.0f → 确认后价格 $%.0f\n", priceAtTouch, priceAfter)
		fmt.Printf("📈 趋势: %s\n", map[TrendType]string{TrendUpward: "上升", TrendDownward: "下降"}[tc.trendType])
		
		// 原有逻辑结果
		oldResult := classifyTouchReactionOld(priceAfter, priceAtTouch, goldenLow, goldenHigh)
		
		// 修复后逻辑结果  
		newResult := classifyTouchReaction(touchCandle, tc.klines, tc.touchIndex, goldenLow, goldenHigh, tc.trendType)
		
		// 判断正确性
		isOldCorrect := oldResult == tc.expectedReaction
		isNewCorrect := newResult == tc.expectedReaction
		
		fmt.Printf("❌ 修复前: %s %s\n", 
			oldResult, 
			map[bool]string{true: "✓", false: "✗ 错误分类"}[isOldCorrect])
			
		fmt.Printf("✅ 修复后: %s %s\n", 
			newResult,
			map[bool]string{true: "✓", false: "✗"}[isNewCorrect])
			
		fmt.Printf("🎯 期望结果: %s\n", tc.expectedReaction)
		
		// 分析改进
		if !isOldCorrect && isNewCorrect {
			fmt.Printf("🚀 修复成功：正确识别了触碰反应类型\n")
		} else if isOldCorrect && isNewCorrect {
			fmt.Printf("✨ 保持正确：两种方法都正确\n")
		} else if !isOldCorrect && !isNewCorrect {
			fmt.Printf("⚠️  需要进一步调试\n")
		}
		
		fmt.Printf("\n")
	}
	
	fmt.Printf("🔧 修复核心要点:\n")
	fmt.Printf("1. 移除永真条件 'low < high'\n")
	fmt.Printf("2. 基于前一根K线判断触碰方向(回撤/反弹)\n")
	fmt.Printf("3. 结合趋势方向和价格确认进行分类\n")
	fmt.Printf("4. 区分'正常延续'vs'趋势失败'场景\n\n")
	
	fmt.Printf("📈 实盘意义:\n")
	fmt.Printf("- 正确识别有效回踩延续 → 增加开仓信号质量\n")
	fmt.Printf("- 准确判断趋势失败 → 提供正确的SL锚点\n")
	fmt.Printf("- 减少误判导致的系统性偏差\n")
}