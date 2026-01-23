package protect

import (
	"testing"
	"time"
)

// TestLONGTakeProfitSimulation 模拟LONG持仓止盈从开仓到盈利增长的完整过程
func TestLONGTakeProfitSimulation(t *testing.T) {
	t.Log("🎬 开始模拟LONG持仓止盈过程...")

	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 5, SlippageBpsMinor: 1, FundingBps: 0},
	}

	// 初始持仓：BTCUSDT LONG @ 100000
	pos := PositionState{
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
	t.Logf("   Entry: %.2f, Leverage: %.0fx", pos.Entry, pos.Leverage)
	t.Logf("   止盈配置: %v", cfg.TPMilestones)

	// 模拟价格上涨过程
	priceSteps := []struct {
		time        int64
		price       float64
		expectedROI float64
		shouldUpdateTP bool
		expectedTP  float64
		description string
	}{
		{30, 100500.0, 0.05, false, 0.0, "ROI 5%，未达到10%阈值"},
		{60, 100800.0, 0.08, false, 0.0, "ROI 8%，未达到10%阈值"},
		{90, 101000.0, 0.10, true, 101500.0, "ROI 10%，触发15%止盈"},
		{120, 101500.0, 0.15, false, 101500.0, "ROI 15%，止盈不变"},
		{150, 102000.0, 0.20, true, 103000.0, "ROI 20%，触发30%止盈"},
		{180, 103000.0, 0.30, false, 103000.0, "ROI 30%，止盈不变"},
		{210, 105000.0, 0.50, true, 107000.0, "ROI 50%，触发70%止盈"},
	}

	t.Logf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Logf("📈 开始模拟价格上涨过程")
	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	updateCount := 0
	for i, step := range priceSteps {
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: step.price,
			MarkPrice: step.price,
			TickSize:  0.1,
			NowMs:     pos.OpenTimeMs + step.time*1000,
		}

		plan := eng.Evaluate(pos, snap)

		// 验证ROI
		if plan.RoiUnr < step.expectedROI-0.001 || plan.RoiUnr > step.expectedROI+0.001 {
			t.Errorf("步骤%d: ROI=%.4f, 期望%.4f", i+1, plan.RoiUnr, step.expectedROI)
		}

		// 验证是否应该更新止盈
		if plan.ShouldUpdateTP != step.shouldUpdateTP {
			t.Errorf("步骤%d: ShouldUpdateTP=%v, 期望%v", i+1, plan.ShouldUpdateTP, step.shouldUpdateTP)
		}

		// 输出日志
		t.Logf("⏱️  T+%ds | 价格: %.2f (%.2f%%) | ROI: %.2f%%",
			step.time, step.price, (step.price-pos.Entry)/pos.Entry*100, plan.RoiUnr*100)

		if plan.ShouldUpdateTP {
			updateCount++

			// 验证止盈价格
			if plan.NewTakeProfit < step.expectedTP-1 || plan.NewTakeProfit > step.expectedTP+1 {
				t.Errorf("   ❌ 止盈价格=%.2f, 期望%.2f", plan.NewTakeProfit, step.expectedTP)
			}

			// 验证止盈在当前价上方
			if plan.NewTakeProfit <= snap.LastPrice {
				t.Errorf("   ❌ 止盈%.2f应该在当前价%.2f上方", plan.NewTakeProfit, snap.LastPrice)
			}

			// 验证止盈单调递增
			if pos.PrevTakeProfit > 0 && plan.NewTakeProfit <= pos.PrevTakeProfit {
				t.Errorf("   ❌ 止盈应该递增: %.2f <= %.2f", plan.NewTakeProfit, pos.PrevTakeProfit)
			}

			tpMove := plan.NewTakeProfit - pos.PrevTakeProfit
			if pos.PrevTakeProfit == 0 {
				t.Logf("   ✅ 止盈设置 #%d: %.2f", updateCount, plan.NewTakeProfit)
			} else {
				t.Logf("   ✅ 止盈更新 #%d: %.2f → %.2f (上移%.2f)",
					updateCount, pos.PrevTakeProfit, plan.NewTakeProfit, tpMove)
			}

			// 更新持仓状态
			pos.PrevTakeProfit = plan.NewTakeProfit
		} else {
			t.Logf("   ⏸️  止盈未更新: 当前TP=%.2f", pos.PrevTakeProfit)
		}
		t.Logf("")
	}

	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Logf("📊 模拟完成统计:")
	t.Logf("   总步骤数: %d", len(priceSteps))
	t.Logf("   止盈更新次数: %d", updateCount)
	t.Logf("   最终价格: %.2f (涨幅%.2f%%)", priceSteps[len(priceSteps)-1].price,
		(priceSteps[len(priceSteps)-1].price-pos.Entry)/pos.Entry*100)
	t.Logf("   最终ROI: %.2f%%", priceSteps[len(priceSteps)-1].expectedROI*100)
	t.Logf("   最终止盈: %.2f (从0设置到%.2f)", pos.PrevTakeProfit, pos.PrevTakeProfit)
	profitPct := (pos.PrevTakeProfit - pos.Entry) / pos.Entry * 100
	t.Logf("   止盈目标: %.2f%% 利润", profitPct)
	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 验证最终状态
	if updateCount != 3 {
		t.Errorf("期望3次止盈更新，实际%d次", updateCount)
	}

	if pos.PrevTakeProfit <= pos.Entry {
		t.Errorf("最终止盈%.2f应该在entry%.2f上方", pos.PrevTakeProfit, pos.Entry)
	}
}

// TestSHORTTakeProfitSimulation 模拟SHORT持仓止盈从开仓到盈利增长的完整过程
func TestSHORTTakeProfitSimulation(t *testing.T) {
	t.Log("🎬 开始模拟SHORT持仓止盈过程...")

	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 5, SlippageBpsMinor: 1, FundingBps: 0},
	}

	// 初始持仓：BTCUSDT SHORT @ 100000
	pos := PositionState{
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
	t.Logf("   Entry: %.2f, Leverage: %.0fx", pos.Entry, pos.Leverage)
	t.Logf("   止盈配置: %v", cfg.TPMilestones)

	// 模拟价格下跌过程
	priceSteps := []struct {
		time        int64
		price       float64
		expectedROI float64
		shouldUpdateTP bool
		expectedTP  float64
		description string
	}{
		{30, 99500.0, 0.05, false, 0.0, "ROI 5%，未达到10%阈值"},
		{60, 99200.0, 0.08, false, 0.0, "ROI 8%，未达到10%阈值"},
		{90, 99000.0, 0.10, true, 98500.0, "ROI 10%，触发15%止盈"},
		{120, 98500.0, 0.15, false, 98500.0, "ROI 15%，止盈不变"},
		{150, 98000.0, 0.20, true, 97000.0, "ROI 20%，触发30%止盈"},
		{180, 97000.0, 0.30, false, 97000.0, "ROI 30%，止盈不变"},
		{210, 95000.0, 0.50, true, 93000.0, "ROI 50%，触发70%止盈"},
	}

	t.Logf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Logf("📉 开始模拟价格下跌过程")
	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	updateCount := 0
	for i, step := range priceSteps {
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: step.price,
			MarkPrice: step.price,
			TickSize:  0.1,
			NowMs:     pos.OpenTimeMs + step.time*1000,
		}

		plan := eng.Evaluate(pos, snap)

		// 验证ROI
		if plan.RoiUnr < step.expectedROI-0.001 || plan.RoiUnr > step.expectedROI+0.001 {
			t.Errorf("步骤%d: ROI=%.4f, 期望%.4f", i+1, plan.RoiUnr, step.expectedROI)
		}

		// 验证是否应该更新止盈
		if plan.ShouldUpdateTP != step.shouldUpdateTP {
			t.Errorf("步骤%d: ShouldUpdateTP=%v, 期望%v", i+1, plan.ShouldUpdateTP, step.shouldUpdateTP)
		}

		// 输出日志
		t.Logf("⏱️  T+%ds | 价格: %.2f (%.2f%%) | ROI: %.2f%%",
			step.time, step.price, (pos.Entry-step.price)/pos.Entry*100, plan.RoiUnr*100)

		if plan.ShouldUpdateTP {
			updateCount++

			// 验证止盈价格
			if plan.NewTakeProfit < step.expectedTP-1 || plan.NewTakeProfit > step.expectedTP+1 {
				t.Errorf("   ❌ 止盈价格=%.2f, 期望%.2f", plan.NewTakeProfit, step.expectedTP)
			}

			// 验证止盈在当前价下方
			if plan.NewTakeProfit >= snap.LastPrice {
				t.Errorf("   ❌ 止盈%.2f应该在当前价%.2f下方", plan.NewTakeProfit, snap.LastPrice)
			}

			// 验证止盈在entry下方
			if plan.NewTakeProfit >= pos.Entry {
				t.Errorf("   ❌ 止盈%.2f应该在entry%.2f下方", plan.NewTakeProfit, pos.Entry)
			}

			// 验证止盈单调递减
			if pos.PrevTakeProfit > 0 && plan.NewTakeProfit >= pos.PrevTakeProfit {
				t.Errorf("   ❌ 止盈应该递减: %.2f >= %.2f", plan.NewTakeProfit, pos.PrevTakeProfit)
			}

			tpMove := pos.PrevTakeProfit - plan.NewTakeProfit
			if pos.PrevTakeProfit == 0 {
				t.Logf("   ✅ 止盈设置 #%d: %.2f", updateCount, plan.NewTakeProfit)
			} else {
				t.Logf("   ✅ 止盈更新 #%d: %.2f → %.2f (下移%.2f)",
					updateCount, pos.PrevTakeProfit, plan.NewTakeProfit, tpMove)
			}

			// 更新持仓状态
			pos.PrevTakeProfit = plan.NewTakeProfit
		} else {
			t.Logf("   ⏸️  止盈未更新: 当前TP=%.2f", pos.PrevTakeProfit)
		}
		t.Logf("")
	}

	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Logf("📊 模拟完成统计:")
	t.Logf("   总步骤数: %d", len(priceSteps))
	t.Logf("   止盈更新次数: %d", updateCount)
	t.Logf("   最终价格: %.2f (跌幅%.2f%%)", priceSteps[len(priceSteps)-1].price,
		(pos.Entry-priceSteps[len(priceSteps)-1].price)/pos.Entry*100)
	t.Logf("   最终ROI: %.2f%%", priceSteps[len(priceSteps)-1].expectedROI*100)
	t.Logf("   最终止盈: %.2f (从0设置到%.2f)", pos.PrevTakeProfit, pos.PrevTakeProfit)
	profitPct := (pos.Entry - pos.PrevTakeProfit) / pos.Entry * 100
	t.Logf("   止盈目标: %.2f%% 利润", profitPct)
	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 验证最终状态
	if updateCount != 3 {
		t.Errorf("期望3次止盈更新，实际%d次", updateCount)
	}

	if pos.PrevTakeProfit >= pos.Entry {
		t.Errorf("最终止盈%.2f应该在entry%.2f下方", pos.PrevTakeProfit, pos.Entry)
	}
}
