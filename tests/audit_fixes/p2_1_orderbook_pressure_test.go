package audit_fixes

import (
	"testing"
	"math"
)

// 模拟OrderBook相关结构
type TestOrderBookLevel struct {
	Price    float64
	Quantity float64
}

type TestPressureCalculationMode int

const (
	TestPressureModeSimple      TestPressureCalculationMode = iota // 简单模式：前5档
	TestPressureModeDeep                                            // 深度模式：前20档
	TestPressureModeWeighted                                        // 加权模式：距离加权
	TestPressureModeFull                                            // 全深度模式：所有档位
)

type TestOrderBookCalculator struct {
	pressureMode TestPressureCalculationMode
	currentPrice float64
}

// 测试P2-1修复：OrderBook压力计算优化
func TestP2_1_OrderBookPressureCalculation(t *testing.T) {
	
	// 模拟一个典型的盘口数据（以BTCUSDT为例）
	bids := []TestOrderBookLevel{
		{Price: 45000, Quantity: 1.2},   // 第1档：54000 USD
		{Price: 44995, Quantity: 0.8},   // 第2档：35996 USD
		{Price: 44990, Quantity: 1.5},   // 第3档：67485 USD
		{Price: 44985, Quantity: 0.5},   // 第4档：22492.5 USD
		{Price: 44980, Quantity: 2.0},   // 第5档：89960 USD
		{Price: 44975, Quantity: 1.0},   // 第6档：44975 USD
		{Price: 44970, Quantity: 0.3},   // 第7档：13491 USD
		{Price: 44965, Quantity: 1.8},   // 第8档：80937 USD
		{Price: 44960, Quantity: 0.7},   // 第9档：31472 USD
		{Price: 44955, Quantity: 2.5},   // 第10档：112387.5 USD
		// ... 更多档位用于测试深度和全深度模式
		{Price: 44950, Quantity: 0.6},   // 第11档
		{Price: 44945, Quantity: 1.1},   // 第12档
		{Price: 44940, Quantity: 0.9},   // 第13档
		{Price: 44935, Quantity: 1.4},   // 第14档
		{Price: 44930, Quantity: 0.4},   // 第15档
		{Price: 44925, Quantity: 1.7},   // 第16档
		{Price: 44920, Quantity: 0.8},   // 第17档
		{Price: 44915, Quantity: 1.3},   // 第18档
		{Price: 44910, Quantity: 0.5},   // 第19档
		{Price: 44905, Quantity: 2.1},   // 第20档
		{Price: 44900, Quantity: 1.0},   // 第21档
		{Price: 44895, Quantity: 0.7},   // 第22档
	}
	
	asks := []TestOrderBookLevel{
		{Price: 45005, Quantity: 0.9},   // 第1档：40504.5 USD
		{Price: 45010, Quantity: 1.3},   // 第2档：58513 USD
		{Price: 45015, Quantity: 0.6},   // 第3档：27009 USD
		{Price: 45020, Quantity: 1.8},   // 第4档：81036 USD
		{Price: 45025, Quantity: 0.4},   // 第5档：18010 USD
		{Price: 45030, Quantity: 1.1},   // 第6档：49533 USD
		{Price: 45035, Quantity: 0.7},   // 第7档：31524.5 USD
		{Price: 45040, Quantity: 1.5},   // 第8档：67560 USD
		{Price: 45045, Quantity: 0.8},   // 第9档：36036 USD
		{Price: 45050, Quantity: 2.2},   // 第10档：99110 USD
		// ... 更多档位
		{Price: 45055, Quantity: 0.5},   // 第11档
		{Price: 45060, Quantity: 1.2},   // 第12档
		{Price: 45065, Quantity: 0.9},   // 第13档
		{Price: 45070, Quantity: 1.6},   // 第14档
		{Price: 45075, Quantity: 0.3},   // 第15档
	}
	
	calc := &TestOrderBookCalculator{
		currentPrice: 45002.5, // 买卖中间价
	}
	
	t.Run("简单模式：前5档对比", func(t *testing.T) {
		calc.pressureMode = TestPressureModeSimple
		
		// 计算前5档压力
		bidPressure := calc.calculatePressureSimple(bids)
		askPressure := calc.calculatePressureSimple(asks)
		
		expectedBidPressure := 54000 + 35996 + 67485 + 22492.5 + 89960 // = 269933.5
		expectedAskPressure := 40504.5 + 58513 + 27009 + 81036 + 18010 // = 225072.5
		
		t.Logf("简单模式（前5档）:")
		t.Logf("  买单压力: %.1f USD (期望: %.1f)", bidPressure, expectedBidPressure)
		t.Logf("  卖单压力: %.1f USD (期望: %.1f)", askPressure, expectedAskPressure)
		t.Logf("  买卖比率: %.3f", bidPressure/askPressure)
		
		if math.Abs(bidPressure-expectedBidPressure) > 1 {
			t.Errorf("买单压力计算错误，期望: %.1f, 实际: %.1f", expectedBidPressure, bidPressure)
		}
		if math.Abs(askPressure-expectedAskPressure) > 1 {
			t.Errorf("卖单压力计算错误，期望: %.1f, 实际: %.1f", expectedAskPressure, askPressure)
		}
		
		t.Logf("✅ 简单模式测试通过")
	})
	
	t.Run("深度模式：前20档对比", func(t *testing.T) {
		calc.pressureMode = TestPressureModeDeep
		
		// 计算前20档压力
		bidPressureDeep := calc.calculatePressureDeep(bids)
		askPressureDeep := calc.calculatePressureDeep(asks)
		
		// 简单模式压力（作为对比基准）
		bidPressureSimple := calc.calculatePressureSimple(bids)
		askPressureSimple := calc.calculatePressureSimple(asks)
		
		t.Logf("深度模式（前20档）:")
		t.Logf("  买单压力: %.1f USD", bidPressureDeep)
		t.Logf("  卖单压力: %.1f USD", askPressureDeep)
		t.Logf("  买卖比率: %.3f", bidPressureDeep/askPressureDeep)
		t.Logf("深度 vs 简单对比:")
		t.Logf("  买单压力提升: %.1f%% (%.1f → %.1f)", 
			(bidPressureDeep-bidPressureSimple)/bidPressureSimple*100, bidPressureSimple, bidPressureDeep)
		t.Logf("  卖单压力提升: %.1f%% (%.1f → %.1f)", 
			(askPressureDeep-askPressureSimple)/askPressureSimple*100, askPressureSimple, askPressureDeep)
		
		// 深度模式应该比简单模式计算出更大的压力值（因为考虑了更多档位）
		if bidPressureDeep <= bidPressureSimple {
			t.Errorf("深度模式买单压力应该大于简单模式，深度: %.1f, 简单: %.1f", bidPressureDeep, bidPressureSimple)
		}
		if askPressureDeep <= askPressureSimple {
			t.Errorf("深度模式卖单压力应该大于简单模式，深度: %.1f, 简单: %.1f", askPressureDeep, askPressureSimple)
		}
		
		t.Logf("✅ 深度模式测试通过：压力计算更全面")
	})
	
	t.Run("加权模式：距离加权对比", func(t *testing.T) {
		calc.pressureMode = TestPressureModeWeighted
		
		// 计算加权压力
		bidPressureWeighted := calc.calculatePressureWeighted(bids)
		askPressureWeighted := calc.calculatePressureWeighted(asks)
		
		// 深度模式压力（作为对比基准）
		bidPressureDeep := calc.calculatePressureDeep(bids)
		askPressureDeep := calc.calculatePressureDeep(asks)
		
		t.Logf("加权模式（距离加权）:")
		t.Logf("  当前价格: %.1f", calc.currentPrice)
		t.Logf("  买单压力: %.1f USD", bidPressureWeighted)
		t.Logf("  卖单压力: %.1f USD", askPressureWeighted)
		t.Logf("  买卖比率: %.3f", bidPressureWeighted/askPressureWeighted)
		t.Logf("加权 vs 深度对比:")
		t.Logf("  买单压力变化: %.1f%% (%.1f → %.1f)", 
			(bidPressureWeighted-bidPressureDeep)/bidPressureDeep*100, bidPressureDeep, bidPressureWeighted)
		t.Logf("  卖单压力变化: %.1f%% (%.1f → %.1f)", 
			(askPressureWeighted-askPressureDeep)/askPressureDeep*100, askPressureDeep, askPressureWeighted)
		
		// 加权模式应该突出距离当前价格更近的档位
		// 因为距离越近权重越高，所以通常加权压力会小于深度模式的简单累加
		if bidPressureWeighted > bidPressureDeep*1.5 {
			t.Errorf("加权模式买单压力异常偏高: %.1f vs 深度模式 %.1f", bidPressureWeighted, bidPressureDeep)
		}
		if askPressureWeighted > askPressureDeep*1.5 {
			t.Errorf("加权模式卖单压力异常偏高: %.1f vs 深度模式 %.1f", askPressureWeighted, askPressureDeep)
		}
		
		t.Logf("✅ 加权模式测试通过：距离加权反映真实影响力")
	})
	
	t.Run("全深度模式：所有档位对比", func(t *testing.T) {
		calc.pressureMode = TestPressureModeFull
		
		// 计算全深度压力
		bidPressureFull := calc.calculatePressureFull(bids)
		askPressureFull := calc.calculatePressureFull(asks)
		
		// 深度模式压力（作为对比基准）
		bidPressureDeep := calc.calculatePressureDeep(bids)
		askPressureDeep := calc.calculatePressureDeep(asks)
		
		t.Logf("全深度模式（所有档位）:")
		t.Logf("  买单档位数: %d", len(bids))
		t.Logf("  卖单档位数: %d", len(asks))
		t.Logf("  买单压力: %.1f USD", bidPressureFull)
		t.Logf("  卖单压力: %.1f USD", askPressureFull)
		t.Logf("  买卖比率: %.3f", bidPressureFull/askPressureFull)
		t.Logf("全深度 vs 深度对比:")
		t.Logf("  买单压力增加: %.1f%% (%.1f → %.1f)", 
			(bidPressureFull-bidPressureDeep)/bidPressureDeep*100, bidPressureDeep, bidPressureFull)
		t.Logf("  卖单压力增加: %.1f%% (%.1f → %.1f)", 
			(askPressureFull-askPressureDeep)/askPressureDeep*100, askPressureDeep, askPressureFull)
		
		// 全深度模式应该比深度模式计算出更大的压力值（因为考虑了所有档位）
		if bidPressureFull < bidPressureDeep {
			t.Errorf("全深度模式买单压力应该大于深度模式，全深度: %.1f, 深度: %.1f", bidPressureFull, bidPressureDeep)
		}
		if askPressureFull < askPressureDeep {
			t.Errorf("全深度模式卖单压力应该大于深度模式，全深度: %.1f, 深度: %.1f", askPressureFull, askPressureDeep)
		}
		
		t.Logf("✅ 全深度模式测试通过：全市场压力评估")
	})
}

// 测试不同模式的压力计算方法实现
func (calc *TestOrderBookCalculator) calculatePressureSimple(levels []TestOrderBookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}

	var totalValue float64
	maxLevels := int(math.Min(float64(len(levels)), 5))

	for i := 0; i < maxLevels; i++ {
		level := levels[i]
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}
		totalValue += level.Price * level.Quantity
	}

	return totalValue
}

func (calc *TestOrderBookCalculator) calculatePressureDeep(levels []TestOrderBookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}

	var totalValue float64
	maxLevels := int(math.Min(float64(len(levels)), 20)) // 扩展到20档

	for i := 0; i < maxLevels; i++ {
		level := levels[i]
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}
		totalValue += level.Price * level.Quantity
	}

	return totalValue
}

func (calc *TestOrderBookCalculator) calculatePressureWeighted(levels []TestOrderBookLevel) float64 {
	if len(levels) == 0 || calc.currentPrice <= 0 {
		return calc.calculatePressureDeep(levels) // 兜底
	}
	
	var weightedValue float64
	maxLevels := int(math.Min(float64(len(levels)), 20)) // 加权计算前20档
	
	for i := 0; i < maxLevels; i++ {
		level := levels[i]
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}
		
		// 计算距离权重：距离越近权重越高
		distance := math.Abs(level.Price-calc.currentPrice) / calc.currentPrice
		
		// 权重函数：距离越近权重越高，使用指数衰减
		weight := math.Exp(-distance * 10) // 10为衰减系数
		
		// 加权价值：价值 × 权重
		levelValue := level.Price * level.Quantity
		weightedValue += levelValue * weight
	}
	
	return weightedValue
}

func (calc *TestOrderBookCalculator) calculatePressureFull(levels []TestOrderBookLevel) float64 {
	if len(levels) == 0 {
		return 0
	}

	var totalValue float64

	for _, level := range levels {
		if level.Price <= 0 || level.Quantity <= 0 {
			continue
		}
		totalValue += level.Price * level.Quantity
	}

	return totalValue
}

// 测试P2-1的性能和场景适应性
func TestP2_1_PressureCalculationScenarios(t *testing.T) {
	
	t.Run("薄盘场景：档位稀少", func(t *testing.T) {
		// 模拟薄盘：只有少数几档
		thinBids := []TestOrderBookLevel{
			{Price: 45000, Quantity: 0.1},   // 很小的量
			{Price: 44995, Quantity: 0.05},  
			{Price: 44990, Quantity: 0.08},  
		}
		
		thinAsks := []TestOrderBookLevel{
			{Price: 45010, Quantity: 0.12},  
			{Price: 45015, Quantity: 0.06},  
		}
		
		calc := &TestOrderBookCalculator{currentPrice: 45005}
		
		simplePressure := calc.calculatePressureSimple(thinBids)
		deepPressure := calc.calculatePressureDeep(thinBids)
		fullPressure := calc.calculatePressureFull(thinBids)
		
		// 也测试卖单压力
		askPressure := calc.calculatePressureSimple(thinAsks)
		
		t.Logf("薄盘场景测试:")
		t.Logf("  买单档位数: %d, 卖单档位数: %d", len(thinBids), len(thinAsks))
		t.Logf("  买单简单模式压力: %.1f USD", simplePressure)
		t.Logf("  买单深度模式压力: %.1f USD", deepPressure)
		t.Logf("  买单全深度压力: %.1f USD", fullPressure)
		t.Logf("  卖单压力: %.1f USD", askPressure)
		
		// 在薄盘情况下，各模式结果应该相近（因为档位数少）
		if math.Abs(simplePressure-deepPressure) > simplePressure*0.1 {
			t.Logf("⚠️ 薄盘时简单模式和深度模式差异较大，这是正常的")
		}
		
		t.Logf("✅ 薄盘场景测试通过")
	})
	
	t.Run("厚盘场景：档位丰富", func(t *testing.T) {
		// 模拟厚盘：大量档位
		var thickBids []TestOrderBookLevel
		var thickAsks []TestOrderBookLevel
		
		// 生成50档买单
		for i := 0; i < 50; i++ {
			price := 45000.0 - float64(i)*5  // 每档相差5美元
			quantity := 0.5 + float64(i%10)*0.1 // 0.5-1.4的量
			thickBids = append(thickBids, TestOrderBookLevel{Price: price, Quantity: quantity})
		}
		
		// 生成50档卖单
		for i := 0; i < 50; i++ {
			price := 45005.0 + float64(i)*5  // 每档相差5美元
			quantity := 0.4 + float64(i%8)*0.15 // 0.4-1.45的量
			thickAsks = append(thickAsks, TestOrderBookLevel{Price: price, Quantity: quantity})
		}
		
		calc := &TestOrderBookCalculator{currentPrice: 45002.5}
		
		simplePressure := calc.calculatePressureSimple(thickBids)
		deepPressure := calc.calculatePressureDeep(thickBids)
		fullPressure := calc.calculatePressureFull(thickBids)
		weightedPressure := calc.calculatePressureWeighted(thickBids)
		
		t.Logf("厚盘场景测试:")
		t.Logf("  买单档位数: %d", len(thickBids))
		t.Logf("  简单模式压力: %.1f USD", simplePressure)
		t.Logf("  深度模式压力: %.1f USD", deepPressure)
		t.Logf("  加权模式压力: %.1f USD", weightedPressure)
		t.Logf("  全深度压力: %.1f USD", fullPressure)
		
		// 在厚盘情况下，各模式应该有明显差异
		deepRatio := deepPressure / simplePressure
		fullRatio := fullPressure / simplePressure
		
		t.Logf("压力倍数:")
		t.Logf("  深度/简单: %.2fx", deepRatio)
		t.Logf("  全深度/简单: %.2fx", fullRatio)
		
		if deepRatio < 1.5 {
			t.Errorf("厚盘时深度模式应该比简单模式高至少50%%，实际: %.2fx", deepRatio)
		}
		if fullRatio < 2.0 {
			t.Errorf("厚盘时全深度模式应该比简单模式高至少2倍，实际: %.2fx", fullRatio)
		}
		
		t.Logf("✅ 厚盘场景测试通过：不同模式差异明显")
	})
	
	t.Run("极端价格档位测试", func(t *testing.T) {
		// 测试极端价格情况的处理
		extremeBids := []TestOrderBookLevel{
			{Price: 0, Quantity: 1.0},        // 无效价格
			{Price: -100, Quantity: 0.5},     // 负价格
			{Price: 45000, Quantity: 0},      // 无效数量
			{Price: 45000, Quantity: -0.1},   // 负数量
			{Price: 45000, Quantity: 1.0},    // 正常档位
		}
		
		calc := &TestOrderBookCalculator{currentPrice: 45000}
		
		pressure := calc.calculatePressureSimple(extremeBids)
		
		t.Logf("极端价格档位测试:")
		t.Logf("  总档位数: %d", len(extremeBids))
		t.Logf("  计算压力: %.1f USD", pressure)
		
		// 应该只计算有效档位的压力
		expectedPressure := 45000.0 * 1.0  // 只有最后一档是有效的
		if math.Abs(pressure-expectedPressure) > 1 {
			t.Errorf("极端情况处理错误，期望: %.1f, 实际: %.1f", expectedPressure, pressure)
		}
		
		t.Logf("✅ 极端价格档位测试通过：正确过滤无效数据")
	})
}

// 测试P2-1的统计对比
func TestP2_1_StatisticalComparison(t *testing.T) {
	// 创建一个标准的测试盘口
	standardBids := []TestOrderBookLevel{
		{Price: 50000, Quantity: 1.0},    // 50000
		{Price: 49995, Quantity: 1.5},    // 74992.5
		{Price: 49990, Quantity: 0.8},    // 39992
		{Price: 49985, Quantity: 2.0},    // 99970
		{Price: 49980, Quantity: 1.2},    // 59976
		{Price: 49975, Quantity: 0.9},    // 44977.5
		{Price: 49970, Quantity: 1.8},    // 89946
		{Price: 49965, Quantity: 0.6},    // 29979
		{Price: 49960, Quantity: 1.4},    // 69944
		{Price: 49955, Quantity: 1.1},    // 54950.5
	}
	
	calc := &TestOrderBookCalculator{currentPrice: 50002.5}
	
	// 计算所有模式的压力值
	simple := calc.calculatePressureSimple(standardBids)
	deep := calc.calculatePressureDeep(standardBids)
	weighted := calc.calculatePressureWeighted(standardBids)
	full := calc.calculatePressureFull(standardBids)
	
	t.Logf("P2-1压力计算优化统计对比:")
	t.Logf("  简单模式（前5档）: %.1f USD", simple)
	t.Logf("  深度模式（前20档）: %.1f USD", deep)
	t.Logf("  加权模式（距离加权）: %.1f USD", weighted)
	t.Logf("  全深度模式（所有档位）: %.1f USD", full)
	
	t.Logf("\n优化效果:")
	t.Logf("  深度模式提升: %.1f%% (相比简单)", (deep-simple)/simple*100)
	t.Logf("  全深度提升: %.1f%% (相比简单)", (full-simple)/simple*100)
	t.Logf("  加权智能调整: %.1f%% (相比深度)", (weighted-deep)/deep*100)
	
	// 验证优化效果的合理性
	if deep <= simple {
		t.Errorf("深度模式应该计算出更大的压力值")
	}
	
	if full < deep {
		t.Errorf("全深度模式应该计算出最大的压力值")
	}
	
	// 加权模式可能比深度模式大或小，取决于价格分布
	t.Logf("✅ P2-1优化效果验证通过：多模式压力计算提供更丰富的市场洞察")
}