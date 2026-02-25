package protect

import (
	"math"
	"testing"
)

// TestROITakeProfitCalculation 测试ROI止盈价格计算
func TestROITakeProfitCalculation(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	tests := []struct {
		name               string
		side               Side
		entryPrice         float64
		markPrice          float64
		leverage           float64
		prevTakeProfit     float64
		expectedShouldUpdate bool
		expectedTPPrice    float64
		expectedReason     string
	}{
		{
			name:               "LONG - 10% ROI触发15%止盈",
			side:               Long,
			entryPrice:         100000.0,
			markPrice:          101000.0, // +1% price, 10% ROI at 10x leverage
			leverage:           10.0,
			prevTakeProfit:     0.0,
			expectedShouldUpdate: true,
			expectedTPPrice:    101500.0, // 🔧 修复后：entry * (1 + 0.15/10) = entry * 1.015
			expectedReason:     "ROI_10_PCT",
		},
		{
			name:               "LONG - 20% ROI触发30%止盈",
			side:               Long,
			entryPrice:         100000.0,
			markPrice:          102000.0, // +2% price, 20% ROI at 10x leverage
			leverage:           10.0,
			prevTakeProfit:     101500.0, // 🔧 修复后：已有15%止盈
			expectedShouldUpdate: true,
			expectedTPPrice:    103000.0, // 🔧 修复后：entry * (1 + 0.30/10) = entry * 1.03
			expectedReason:     "ROI_20_PCT",
		},
		{
			name:               "LONG - 50% ROI触发70%止盈",
			side:               Long,
			entryPrice:         100000.0,
			markPrice:          105000.0, // +5% price, 50% ROI at 10x leverage
			leverage:           10.0,
			prevTakeProfit:     103000.0, // 🔧 修复后：已有30%止盈
			expectedShouldUpdate: true,
			expectedTPPrice:    107000.0, // 🔧 修复后：entry * (1 + 0.70/10) = entry * 1.07
			expectedReason:     "ROI_50_PCT",
		},
		{
			name:               "SHORT - 10% ROI触发15%止盈",
			side:               Short,
			entryPrice:         100000.0,
			markPrice:          99000.0, // -1% price, 10% ROI at 10x leverage
			leverage:           10.0,
			prevTakeProfit:     0.0,
			expectedShouldUpdate: true,
			expectedTPPrice:    98500.0, // 🔧 修复后：entry * (1 - 0.15/10) = entry * 0.985
			expectedReason:     "ROI_10_PCT",
		},
		{
			name:               "SHORT - 20% ROI触发30%止盈",
			side:               Short,
			entryPrice:         100000.0,
			markPrice:          98000.0, // -2% price, 20% ROI at 10x leverage
			leverage:           10.0,
			prevTakeProfit:     98500.0, // 🔧 修复后：已有15%止盈
			expectedShouldUpdate: true,
			expectedTPPrice:    97000.0, // 🔧 修复后：entry * (1 - 0.30/10) = entry * 0.97
			expectedReason:     "ROI_20_PCT",
		},
		{
			name:               "LONG - 单调性：止盈不应下移",
			side:               Long,
			entryPrice:         100000.0,
			markPrice:          101000.0, // 10% ROI
			leverage:           10.0,
			prevTakeProfit:     120000.0, // 已设置更高的止盈
			expectedShouldUpdate: false,  // 不应更新（115000 < 120000）
			expectedTPPrice:    0.0,
			expectedReason:     "",
		},
		{
			name:               "SHORT - 单调性：止盈不应上移",
			side:               Short,
			entryPrice:         100000.0,
			markPrice:          99000.0, // 10% ROI
			leverage:           10.0,
			prevTakeProfit:     80000.0, // 已设置更低的止盈
			expectedShouldUpdate: false, // 不应更新（85000 > 80000）
			expectedTPPrice:    0.0,
			expectedReason:     "",
		},
		{
			name:               "LONG - ROI不足，不触发",
			side:               Long,
			entryPrice:         100000.0,
			markPrice:          100500.0, // +0.5% price, 5% ROI (低于10%阈值)
			leverage:           10.0,
			prevTakeProfit:     0.0,
			expectedShouldUpdate: false,
			expectedTPPrice:    0.0,
			expectedReason:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 根据方向设置正确的InitStop
			initStop := tt.entryPrice * 0.95 // LONG默认：entry下方5%
			if tt.side == Short {
				initStop = tt.entryPrice * 1.05 // SHORT：entry上方5%（价格上涨止损）
			}

			pos := PositionState{
				Symbol:         "BTCUSDT",
				Side:           tt.side,
				Entry:          tt.entryPrice,
				Leverage:       tt.leverage,
				InitStop:       initStop,
				PrevStop:       initStop,
				PrevTakeProfit: tt.prevTakeProfit,
				Qty:            1.0,
			}

			snap := MarketSnapshot{
				Symbol:    "BTCUSDT",
				LastPrice: tt.markPrice,
				MarkPrice: tt.markPrice,
				TickSize:  0.1,
				NowMs:     1000000,
			}

			plan := eng.Evaluate(pos, snap)

			// 验证是否应该更新止盈
			if plan.ShouldUpdateTP != tt.expectedShouldUpdate {
				t.Errorf("ShouldUpdateTP = %v, want %v", plan.ShouldUpdateTP, tt.expectedShouldUpdate)
			}

			if tt.expectedShouldUpdate {
				// 验证止盈价格（允许0.01的浮点误差）
				if math.Abs(plan.NewTakeProfit-tt.expectedTPPrice) > 0.01 {
					t.Errorf("NewTakeProfit = %.2f, want %.2f", plan.NewTakeProfit, tt.expectedTPPrice)
				}

				// 验证止盈原因
				if plan.TPReason != tt.expectedReason {
					t.Errorf("TPReason = %s, want %s", plan.TPReason, tt.expectedReason)
				}
			}

			// 验证ROI计算
			expectedROI := 0.0
			if tt.side == Long {
				expectedROI = ((tt.markPrice - tt.entryPrice) / tt.entryPrice) * tt.leverage
			} else {
				expectedROI = ((tt.entryPrice - tt.markPrice) / tt.entryPrice) * tt.leverage
			}

			if math.Abs(plan.RoiUnr-expectedROI) > 0.0001 {
				t.Errorf("ROI calculation: got %.4f, want %.4f", plan.RoiUnr, expectedROI)
			}
		})
	}
}

// TestROITakeProfitMultipleMilestones 测试多重里程碑触发（选择最高档）
func TestROITakeProfitMultipleMilestones(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	// LONG持仓，50% ROI（同时触发10%, 20%, 50%三个里程碑）
	pos := PositionState{
		Symbol:         "BTCUSDT",
		Side:           Long,
		Entry:          100000.0,
		Leverage:       10.0,
		InitStop:       95000.0,  // LONG止损在entry下方
		PrevStop:       95000.0,
		PrevTakeProfit: 0.0,
		Qty:            1.0,
	}

	// +5%价格变动 = 50% ROI at 10x leverage
	snap := MarketSnapshot{
		Symbol:    "BTCUSDT",
		LastPrice: 105000.0,
		MarkPrice: 105000.0,
		TickSize:  0.1,
		NowMs:     1000000,
	}

	plan := eng.Evaluate(pos, snap)

	// 应该触发最高的50%里程碑（70%止盈）
	if !plan.ShouldUpdateTP {
		t.Fatalf("ShouldUpdateTP should be true for 50%% ROI")
	}

	expectedTP := 107000.0 // entry * (1 + 0.70/10) = 107000
	if math.Abs(plan.NewTakeProfit-expectedTP) > 0.01 {
		t.Errorf("NewTakeProfit = %.2f, want %.2f (should use highest milestone)",
			plan.NewTakeProfit, expectedTP)
	}

	if plan.TPReason != "ROI_50_PCT" {
		t.Errorf("TPReason = %s, want ROI_50_PCT", plan.TPReason)
	}
}

// TestROITakeProfitMonotonicity 测试止盈单调性约束
func TestROITakeProfitMonotonicity(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	tests := []struct {
		name         string
		side         Side
		entryPrice   float64
		prevTP       float64
		roiPercent   float64 // 当前ROI百分比（用于计算markPrice）
		shouldUpdate bool
		expectedTP   float64
	}{
		{
			name:         "LONG - 止盈上移（15%→30%）",
			side:         Long,
			entryPrice:   100000.0,
			prevTP:       101500.0, // 已有15% ROI TP: entry*(1+0.15/10)=101500
			roiPercent:   0.20,     // 20% ROI触发30%止盈
			shouldUpdate: true,
			expectedTP:   103000.0, // 30% ROI TP: entry*(1+0.30/10)=103000
		},
		{
			name:         "LONG - 止盈不下移（已有20%，触发15%）",
			side:         Long,
			entryPrice:   100000.0,
			prevTP:       120000.0, // 已有20%止盈（高于15%）
			roiPercent:   0.10,     // 10% ROI仅触发15%止盈
			shouldUpdate: false,    // maxFloat(120000, 115000) = 120000，无变化
		},
		{
			name:         "SHORT - 止盈下移（15%→30%）",
			side:         Short,
			entryPrice:   100000.0,
			prevTP:       98500.0, // 已有15% ROI TP: entry*(1-0.15/10)=98500
			roiPercent:   0.20,    // 20% ROI触发30%止盈
			shouldUpdate: true,
			expectedTP:   97000.0, // 30% ROI TP: entry*(1-0.30/10)=97000
		},
		{
			name:         "SHORT - 止盈不上移（已有20%，触发15%）",
			side:         Short,
			entryPrice:   100000.0,
			prevTP:       80000.0, // 已有20%止盈（低于15%）
			roiPercent:   0.10,    // 10% ROI仅触发15%止盈
			shouldUpdate: false,   // minFloat(80000, 85000) = 80000，无变化
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 根据方向设置正确的InitStop
			initStop := tt.entryPrice * 0.95 // LONG默认：entry下方5%
			if tt.side == Short {
				initStop = tt.entryPrice * 1.05 // SHORT：entry上方5%
			}

			pos := PositionState{
				Symbol:         "BTCUSDT",
				Side:           tt.side,
				Entry:          tt.entryPrice,
				Leverage:       10.0,
				InitStop:       initStop,
				PrevStop:       initStop,
				PrevTakeProfit: tt.prevTP,
				Qty:            1.0,
			}

			// 根据ROI百分比计算markPrice
			// ROI = ((markPrice - entry) / entry) * leverage (for LONG)
			// ROI = ((entry - markPrice) / entry) * leverage (for SHORT)
			var markPrice float64
			if tt.side == Long {
				// roi = ((mark - entry) / entry) * lev => mark = entry * (1 + roi/lev)
				markPrice = tt.entryPrice * (1 + tt.roiPercent/10.0)
			} else {
				// roi = ((entry - mark) / entry) * lev => mark = entry * (1 - roi/lev)
				markPrice = tt.entryPrice * (1 - tt.roiPercent/10.0)
			}

			snap := MarketSnapshot{
				Symbol:    "BTCUSDT",
				LastPrice: markPrice,
				MarkPrice: markPrice,
				TickSize:  0.1,
				NowMs:     1000000,
			}

			plan := eng.Evaluate(pos, snap)

			// 验证是否应该更新
			if plan.ShouldUpdateTP != tt.shouldUpdate {
				t.Errorf("ShouldUpdateTP = %v, want %v (monotonicity check). ROI=%.2f%%, prevTP=%.0f",
					plan.ShouldUpdateTP, tt.shouldUpdate, plan.RoiUnr*100, tt.prevTP)
			}

			// 如果应该更新，验证新止盈价
			if tt.shouldUpdate && plan.NewTakeProfit != tt.expectedTP {
				t.Errorf("NewTakeProfit = %.2f, want %.2f",
					plan.NewTakeProfit, tt.expectedTP)
			}
		})
	}
}

// TestConfigurableTPMilestones 测试可配置的ROI里程碑
func TestConfigurableTPMilestones(t *testing.T) {
	// 自定义配置：更激进的止盈策略
	cfg := DefaultConfig()
	cfg.TPMilestones = map[float64]float64{
		0.05: 0.10, // 5% ROI → 10% 止盈
		0.15: 0.25, // 15% ROI → 25% 止盈
		0.30: 0.50, // 30% ROI → 50% 止盈
	}

	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	pos := PositionState{
		Symbol:         "BTCUSDT",
		Side:           Long,
		Entry:          100000.0,
		Leverage:       10.0,
		InitStop:       95000.0,  // LONG止损在entry下方5%
		PrevStop:       95000.0,
		PrevTakeProfit: 0.0,
		Qty:            1.0,
	}

	// +0.5%价格变动 = 5% ROI at 10x leverage
	snap := MarketSnapshot{
		Symbol:    "BTCUSDT",
		LastPrice: 100500.0,
		MarkPrice: 100500.0,
		TickSize:  0.1,
		NowMs:     1000000,
	}

	plan := eng.Evaluate(pos, snap)

	// 应该触发5% ROI里程碑（10%止盈）
	if !plan.ShouldUpdateTP {
		t.Fatalf("ShouldUpdateTP should be true for custom 5%% ROI milestone")
	}

	expectedTP := 101000.0 // entry * (1 + 0.10/10) = 101000
	if math.Abs(plan.NewTakeProfit-expectedTP) > 0.01 {
		t.Errorf("NewTakeProfit = %.2f, want %.2f (custom milestone)",
			plan.NewTakeProfit, expectedTP)
	}
}
