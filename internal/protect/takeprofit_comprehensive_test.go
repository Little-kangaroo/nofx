package protect

import (
	"testing"
	"time"
)

// TestLONGTakeProfitComprehensive LONG持仓止盈更新综合测试
func TestLONGTakeProfitComprehensive(t *testing.T) {
	t.Log("🎬 开始LONG持仓止盈综合测试...")

	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 5, SlippageBpsMinor: 1, FundingBps: 0},
	}

	// 基础持仓：BTCUSDT LONG
	basePos := PositionState{
		Symbol:               "BTCUSDT",
		Side:                 Long,
		Entry:                100000.0,
		Leverage:             10.0,
		InitStop:             95000.0,
		PrevStop:             95000.0,
		PrevTakeProfit:       0.0,
		Qty:                  1.0,
		OpenTimeMs:           time.Now().UnixMilli(),
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
	}

	t.Logf("📍 初始状态:")
	t.Logf("   Entry: %.2f, Leverage: %.0fx", basePos.Entry, basePos.Leverage)
	t.Logf("   止盈配置: %v", cfg.TPMilestones)

	t.Run("场景1: ROI未达到阈值 (5%)", func(t *testing.T) {
		pos := basePos
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 100500.0, // ROI = 5%
			MarkPrice: 100500.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// ROI 5% < 10%阈值，不应触发止盈
		if plan.ShouldUpdateTP {
			t.Errorf("ROI 5%% < 10%%阈值，不应触发止盈")
		}

		t.Logf("✓ ROI=%.2f%%, 未触发止盈（正确）", plan.RoiUnr*100)
	})

	t.Run("场景2: ROI达到10%阈值", func(t *testing.T) {
		pos := basePos
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 101000.0, // ROI = 10%
			MarkPrice: 101000.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// 验证ROI
		if plan.RoiUnr < 0.099 || plan.RoiUnr > 0.101 {
			t.Errorf("ROI计算错误: %.4f, 期望约0.10", plan.RoiUnr)
		}

		// 应该触发止盈
		if !plan.ShouldUpdateTP {
			t.Errorf("ROI达到10%%，应该触发止盈")
		}

		// 验证止盈价格：entry * (1 + tpPct/leverage) = 100000 * (1 + 0.15/10) = 101500
		expectedTP := 101500.0
		if plan.NewTakeProfit < expectedTP-1 || plan.NewTakeProfit > expectedTP+1 {
			t.Errorf("止盈价格错误: %.2f, 期望%.2f", plan.NewTakeProfit, expectedTP)
		}

		// 验证止盈在当前价上方
		if plan.NewTakeProfit <= snap.LastPrice {
			t.Errorf("止盈%.2f应该在当前价%.2f上方", plan.NewTakeProfit, snap.LastPrice)
		}

		t.Logf("✓ ROI=%.2f%%, 触发止盈=%.2f", plan.RoiUnr*100, plan.NewTakeProfit)
	})

	t.Run("场景3: ROI阶梯递进", func(t *testing.T) {
		testCases := []struct {
			name        string
			lastPrice   float64
			expectedROI float64
			expectedTP  float64
			prevTP      float64
		}{
			{"10% ROI → 15% TP", 101000.0, 0.10, 101500.0, 0.0},
			{"20% ROI → 30% TP", 102000.0, 0.20, 103000.0, 101500.0},
			{"50% ROI → 70% TP", 105000.0, 0.50, 107000.0, 103000.0},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				pos := basePos
				pos.PrevTakeProfit = tc.prevTP

				snap := MarketSnapshot{
					Symbol:    "BTCUSDT",
					LastPrice: tc.lastPrice,
					MarkPrice: tc.lastPrice,
					TickSize:  0.1,
					NowMs:     time.Now().UnixMilli(),
				}

				plan := eng.Evaluate(pos, snap)

				// 验证ROI
				if plan.RoiUnr < tc.expectedROI-0.001 || plan.RoiUnr > tc.expectedROI+0.001 {
					t.Errorf("ROI=%.4f, 期望%.4f", plan.RoiUnr, tc.expectedROI)
				}

				// 应该触发止盈更新
				if !plan.ShouldUpdateTP {
					t.Errorf("应该触发止盈更新")
				}

				// 验证止盈价格
				if plan.NewTakeProfit < tc.expectedTP-1 || plan.NewTakeProfit > tc.expectedTP+1 {
					t.Errorf("止盈价格=%.2f, 期望%.2f", plan.NewTakeProfit, tc.expectedTP)
				}

				// 验证止盈单调递增
				if tc.prevTP > 0 && plan.NewTakeProfit <= tc.prevTP {
					t.Errorf("止盈应该递增: %.2f <= %.2f", plan.NewTakeProfit, tc.prevTP)
				}

				t.Logf("✓ %s: TP=%.2f", tc.name, plan.NewTakeProfit)
			})
		}
	})

	t.Run("场景4: 止盈单调性保护", func(t *testing.T) {
		pos := basePos
		pos.PrevTakeProfit = 120000.0 // 已有更高的止盈

		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 101000.0, // ROI = 10%，触发15% TP = 101500
			MarkPrice: 101000.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// 不应该更新（101500 < 120000）
		if plan.ShouldUpdateTP {
			t.Errorf("止盈不应下移: 新TP=%.2f < 旧TP=%.2f", plan.NewTakeProfit, pos.PrevTakeProfit)
		}

		t.Logf("✓ 止盈单调性保护正确")
	})
}

// TestSHORTTakeProfitComprehensive SHORT持仓止盈更新综合测试
func TestSHORTTakeProfitComprehensive(t *testing.T) {
	t.Log("🎬 开始SHORT持仓止盈综合测试...")

	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 5, SlippageBpsMinor: 1, FundingBps: 0},
	}

	// 基础持仓：BTCUSDT SHORT
	basePos := PositionState{
		Symbol:               "BTCUSDT",
		Side:                 Short,
		Entry:                100000.0,
		Leverage:             10.0,
		InitStop:             105000.0,
		PrevStop:             105000.0,
		PrevTakeProfit:       0.0,
		Qty:                  1.0,
		OpenTimeMs:           time.Now().UnixMilli(),
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
	}

	t.Logf("📍 初始状态:")
	t.Logf("   Entry: %.2f, Leverage: %.0fx", basePos.Entry, basePos.Leverage)

	t.Run("场景1: ROI未达到阈值 (5%)", func(t *testing.T) {
		pos := basePos
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 99500.0, // ROI = 5%
			MarkPrice: 99500.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// ROI 5% < 10%阈值，不应触发止盈
		if plan.ShouldUpdateTP {
			t.Errorf("ROI 5%% < 10%%阈值，不应触发止盈")
		}

		t.Logf("✓ ROI=%.2f%%, 未触发止盈（正确）", plan.RoiUnr*100)
	})

	t.Run("场景2: ROI达到10%阈值", func(t *testing.T) {
		pos := basePos
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 99000.0, // ROI = 10%
			MarkPrice: 99000.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// 验证ROI
		if plan.RoiUnr < 0.099 || plan.RoiUnr > 0.101 {
			t.Errorf("ROI计算错误: %.4f, 期望约0.10", plan.RoiUnr)
		}

		// 应该触发止盈
		if !plan.ShouldUpdateTP {
			t.Errorf("ROI达到10%%，应该触发止盈")
		}

		// 验证止盈价格：entry * (1 - tpPct/leverage) = 100000 * (1 - 0.15/10) = 98500
		expectedTP := 98500.0
		if plan.NewTakeProfit < expectedTP-1 || plan.NewTakeProfit > expectedTP+1 {
			t.Errorf("止盈价格错误: %.2f, 期望%.2f", plan.NewTakeProfit, expectedTP)
		}

		// 验证止盈在当前价下方
		if plan.NewTakeProfit >= snap.LastPrice {
			t.Errorf("止盈%.2f应该在当前价%.2f下方", plan.NewTakeProfit, snap.LastPrice)
		}

		// 验证止盈在entry下方
		if plan.NewTakeProfit >= pos.Entry {
			t.Errorf("止盈%.2f应该在entry%.2f下方", plan.NewTakeProfit, pos.Entry)
		}

		t.Logf("✓ ROI=%.2f%%, 触发止盈=%.2f", plan.RoiUnr*100, plan.NewTakeProfit)
	})

	t.Run("场景3: ROI阶梯递进", func(t *testing.T) {
		testCases := []struct {
			name        string
			lastPrice   float64
			expectedROI float64
			expectedTP  float64
			prevTP      float64
		}{
			{"10% ROI → 15% TP", 99000.0, 0.10, 98500.0, 0.0},
			{"20% ROI → 30% TP", 98000.0, 0.20, 97000.0, 98500.0},
			{"50% ROI → 70% TP", 95000.0, 0.50, 93000.0, 97000.0},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				pos := basePos
				pos.PrevTakeProfit = tc.prevTP

				snap := MarketSnapshot{
					Symbol:    "BTCUSDT",
					LastPrice: tc.lastPrice,
					MarkPrice: tc.lastPrice,
					TickSize:  0.1,
					NowMs:     time.Now().UnixMilli(),
				}

				plan := eng.Evaluate(pos, snap)

				// 验证ROI
				if plan.RoiUnr < tc.expectedROI-0.001 || plan.RoiUnr > tc.expectedROI+0.001 {
					t.Errorf("ROI=%.4f, 期望%.4f", plan.RoiUnr, tc.expectedROI)
				}

				// 应该触发止盈更新
				if !plan.ShouldUpdateTP {
					t.Errorf("应该触发止盈更新")
				}

				// 验证止盈价格
				if plan.NewTakeProfit < tc.expectedTP-1 || plan.NewTakeProfit > tc.expectedTP+1 {
					t.Errorf("止盈价格=%.2f, 期望%.2f", plan.NewTakeProfit, tc.expectedTP)
				}

				// 验证止盈单调递减（SHORT）
				if tc.prevTP > 0 && plan.NewTakeProfit >= tc.prevTP {
					t.Errorf("止盈应该递减: %.2f >= %.2f", plan.NewTakeProfit, tc.prevTP)
				}

				t.Logf("✓ %s: TP=%.2f", tc.name, plan.NewTakeProfit)
			})
		}
	})

	t.Run("场景4: 止盈单调性保护", func(t *testing.T) {
		pos := basePos
		pos.PrevTakeProfit = 80000.0 // 已有更低的止盈

		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 99000.0, // ROI = 10%，触发15% TP = 98500
			MarkPrice: 99000.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// 不应该更新（98500 > 80000）
		if plan.ShouldUpdateTP {
			t.Errorf("止盈不应上移: 新TP=%.2f > 旧TP=%.2f", plan.NewTakeProfit, pos.PrevTakeProfit)
		}

		t.Logf("✓ 止盈单调性保护正确")
	})
}
