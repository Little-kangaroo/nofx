package protect

import (
	"testing"
	"time"
)

// TestLONGProfitSimulation 模拟LONG持仓从开仓到盈利增长的完整过程
func TestLONGProfitSimulation(t *testing.T) {
	t.Log("🎬 开始模拟LONG持仓盈利过程...")

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
		InitStop:             95000.0, // R0 = 5000
		PrevStop:             95000.0,
		Qty:                  1.0,
		OpenTimeMs:           time.Now().UnixMilli(),
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
	}

	t.Logf("📍 初始状态:")
	t.Logf("   Entry: %.2f, InitStop: %.2f, R0: %.2f", pos.Entry, pos.InitStop, pos.Entry-pos.InitStop)
	t.Logf("   Leverage: %.0fx", pos.Leverage)

	// 模拟价格上涨过程
	priceSteps := []struct {
		time      int64   // 时间（秒）
		price     float64 // 当前价格
		expectedROI float64 // 期望ROI
		shouldUpdate bool  // 是否应该更新
		description string
	}{
		{30, 100100.0, 0.01, false, "价格上涨0.1%，ROI 1%，未触发"},
		{60, 100200.0, 0.02, false, "价格上涨0.2%，ROI 2%，未触发"},
		{90, 100300.0, 0.03, true, "价格上涨0.3%，ROI 3%，触发保本！"},
		{120, 100400.0, 0.04, false, "价格上涨0.4%，ROI 4%，未触发"},
		{150, 100500.0, 0.05, true, "价格上涨0.5%，ROI 5%，触发锁盈 3%"},
		{180, 100600.0, 0.06, false, "价格上涨0.6%，ROI 6%，冷却期内"},
		{210, 100800.0, 0.08, true, "价格上涨0.8%，ROI 8%，触发第二次更新"},
		{240, 101000.0, 0.10, false, "价格上涨1.0%，ROI 10%，冷却期内"},
		{270, 101200.0, 0.12, true, "价格上涨1.2%，ROI 12%，触发第三次更新"},
		{300, 101500.0, 0.15, false, "价格上涨1.5%，ROI 15%，冷却期内"},
		{330, 102000.0, 0.20, true, "价格上涨2.0%，ROI 20%，触发第四次更新"},
		{360, 102500.0, 0.25, true, "价格上涨2.5%，ROI 25%，触发第五次更新"},
		{390, 103000.0, 0.30, true, "价格上涨3.0%，ROI 30%，触发第六次更新"},
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

		// 验证ROI计算
		if plan.RoiUnr < step.expectedROI-0.001 || plan.RoiUnr > step.expectedROI+0.001 {
			t.Errorf("步骤%d: ROI计算错误: %.4f, 期望%.4f", i+1, plan.RoiUnr, step.expectedROI)
		}

		// 验证是否应该更新
		if plan.ShouldUpdate != step.shouldUpdate {
			t.Errorf("步骤%d: 更新状态错误: %v, 期望%v", i+1, plan.ShouldUpdate, step.shouldUpdate)
		}

		// 输出日志
		t.Logf("⏱️  T+%ds | 价格: %.2f (%.2f%%) | ROI: %.2f%% | R: %.2fR",
			step.time, step.price, (step.price-pos.Entry)/pos.Entry*100, plan.RoiUnr*100, plan.RUnr)

		if plan.ShouldUpdate {
			updateCount++
			stopMove := plan.NewStop - pos.PrevStop
			stopMovePct := stopMove / pos.PrevStop * 100

			t.Logf("   ✅ 止损更新 #%d: %.2f → %.2f (上移%.2f, +%.2f%%)",
				updateCount, pos.PrevStop, plan.NewStop, stopMove, stopMovePct)
			t.Logf("   📊 盈利地板: %.2f | BE: %.2f", plan.Floor, plan.BE)

			// 验证止损单调递增
			if plan.NewStop <= pos.PrevStop {
				t.Errorf("   ❌ 止损应该上移: %.2f <= %.2f", plan.NewStop, pos.PrevStop)
			}

			// 验证止损在当前价下方
			if plan.NewStop >= snap.LastPrice {
				t.Errorf("   ❌ 止损%.2f应该在当前价%.2f下方", plan.NewStop, snap.LastPrice)
			}

			// 更新持仓状态
			pos.PrevStop = plan.NewStop
			pos.LastStopUpdateTimeMs = snap.NowMs
			pos.ROIArmed = plan.NextROIArmed
		} else {
			if len(plan.Reasons) > 0 {
				t.Logf("   ⏸️  未更新: %v", plan.Reasons)
			}
		}

		t.Logf("   🛡️  当前止损: %.2f | 距离entry: %.2f (%.2f%%)\n",
			pos.PrevStop, pos.PrevStop-pos.Entry, (pos.PrevStop-pos.Entry)/pos.Entry*100)
	}

	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Logf("📊 模拟完成统计:")
	t.Logf("   总步骤数: %d", len(priceSteps))
	t.Logf("   止损更新次数: %d", updateCount)
	t.Logf("   最终价格: %.2f (涨幅%.2f%%)", priceSteps[len(priceSteps)-1].price,
		(priceSteps[len(priceSteps)-1].price-pos.Entry)/pos.Entry*100)
	t.Logf("   最终ROI: %.2f%%", priceSteps[len(priceSteps)-1].expectedROI*100)
	t.Logf("   最终止损: %.2f (从%.2f上移%.2f)", pos.PrevStop, 95000.0, pos.PrevStop-95000.0)
	t.Logf("   止损保护: %.2f%% 利润", (pos.PrevStop-pos.Entry)/pos.Entry*100)
	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 验证最终状态
	if updateCount != 7 {
		t.Errorf("期望7次止损更新（含3%%保本），实际%d次", updateCount)
	}

	if pos.PrevStop <= pos.Entry {
		t.Errorf("最终止损%.2f应该在entry%.2f上方（锁定利润）", pos.PrevStop, pos.Entry)
	}
}

// TestSHORTProfitSimulation 模拟SHORT持仓从开仓到盈利增长的完整过程
func TestSHORTProfitSimulation(t *testing.T) {
	t.Log("🎬 开始模拟SHORT持仓盈利过程...")

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
		InitStop:             105000.0, // R0 = 5000
		PrevStop:             105000.0,
		Qty:                  1.0,
		OpenTimeMs:           time.Now().UnixMilli(),
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
	}

	t.Logf("📍 初始状态:")
	t.Logf("   Entry: %.2f, InitStop: %.2f, R0: %.2f", pos.Entry, pos.InitStop, pos.InitStop-pos.Entry)
	t.Logf("   Leverage: %.0fx", pos.Leverage)

	// 模拟价格下跌过程
	priceSteps := []struct {
		time      int64   // 时间（秒）
		price     float64 // 当前价格
		expectedROI float64 // 期望ROI
		shouldUpdate bool  // 是否应该更新
		description string
	}{
		{30, 99900.0, 0.01, false, "价格下跌0.1%，ROI 1%，未触发"},
		{60, 99800.0, 0.02, false, "价格下跌0.2%，ROI 2%，未触发"},
		{90, 99700.0, 0.03, true, "价格下跌0.3%，ROI 3%，触发保本！"},
		{120, 99600.0, 0.04, false, "价格下跌0.4%，ROI 4%，未触发"},
		{150, 99500.0, 0.05, true, "价格下跌0.5%，ROI 5%，触发锁盈 3%"},
		{180, 99400.0, 0.06, false, "价格下跌0.6%，ROI 6%，冷却期内"},
		{210, 99200.0, 0.08, true, "价格下跌0.8%，ROI 8%，触发第二次更新"},
		{240, 99000.0, 0.10, false, "价格下跌1.0%，ROI 10%，冷却期内"},
		{270, 98800.0, 0.12, true, "价格下跌1.2%，ROI 12%，触发第三次更新"},
		{300, 98500.0, 0.15, false, "价格下跌1.5%，ROI 15%，冷却期内"},
		{330, 98000.0, 0.20, true, "价格下跌2.0%，ROI 20%，触发第四次更新"},
		{360, 97500.0, 0.25, true, "价格下跌2.5%，ROI 25%，触发第五次更新"},
		{390, 97000.0, 0.30, true, "价格下跌3.0%，ROI 30%，触发第六次更新"},
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

		// 验证ROI计算
		if plan.RoiUnr < step.expectedROI-0.001 || plan.RoiUnr > step.expectedROI+0.001 {
			t.Errorf("步骤%d: ROI计算错误: %.4f, 期望%.4f", i+1, plan.RoiUnr, step.expectedROI)
		}

		// 验证是否应该更新
		if plan.ShouldUpdate != step.shouldUpdate {
			t.Errorf("步骤%d: 更新状态错误: %v, 期望%v", i+1, plan.ShouldUpdate, step.shouldUpdate)
		}

		// 输出日志
		t.Logf("⏱️  T+%ds | 价格: %.2f (%.2f%%) | ROI: %.2f%% | R: %.2fR",
			step.time, step.price, (pos.Entry-step.price)/pos.Entry*100, plan.RoiUnr*100, plan.RUnr)

		if plan.ShouldUpdate {
			updateCount++
			stopMove := pos.PrevStop - plan.NewStop
			stopMovePct := stopMove / pos.PrevStop * 100

			t.Logf("   ✅ 止损更新 #%d: %.2f → %.2f (下移%.2f, -%.2f%%)",
				updateCount, pos.PrevStop, plan.NewStop, stopMove, stopMovePct)
			t.Logf("   📊 盈利地板: %.2f | BE: %.2f", plan.Floor, plan.BE)

			// 验证止损单调递减
			if plan.NewStop >= pos.PrevStop {
				t.Errorf("   ❌ 止损应该下移: %.2f >= %.2f", plan.NewStop, pos.PrevStop)
			}

			// 验证止损在当前价上方
			if plan.NewStop <= snap.LastPrice {
				t.Errorf("   ❌ 止损%.2f应该在当前价%.2f上方", plan.NewStop, snap.LastPrice)
			}

			// 验证止损在entry下方（锁定利润）
			if plan.NewStop > pos.Entry {
				t.Errorf("   ❌ 止损%.2f应该在entry%.2f下方（锁定利润）", plan.NewStop, pos.Entry)
			}

			// 更新持仓状态
			pos.PrevStop = plan.NewStop
			pos.LastStopUpdateTimeMs = snap.NowMs
			pos.ROIArmed = plan.NextROIArmed
		} else {
			if len(plan.Reasons) > 0 {
				t.Logf("   ⏸️  未更新: %v", plan.Reasons)
			}
		}

		t.Logf("   🛡️  当前止损: %.2f | 距离entry: %.2f (%.2f%%)\n",
			pos.PrevStop, pos.Entry-pos.PrevStop, (pos.Entry-pos.PrevStop)/pos.Entry*100)
	}

	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	t.Logf("📊 模拟完成统计:")
	t.Logf("   总步骤数: %d", len(priceSteps))
	t.Logf("   止损更新次数: %d", updateCount)
	t.Logf("   最终价格: %.2f (跌幅%.2f%%)", priceSteps[len(priceSteps)-1].price,
		(pos.Entry-priceSteps[len(priceSteps)-1].price)/pos.Entry*100)
	t.Logf("   最终ROI: %.2f%%", priceSteps[len(priceSteps)-1].expectedROI*100)
	t.Logf("   最终止损: %.2f (从%.2f下移%.2f)", pos.PrevStop, 105000.0, 105000.0-pos.PrevStop)
	t.Logf("   止损保护: %.2f%% 利润", (pos.Entry-pos.PrevStop)/pos.Entry*100)
	t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 验证最终状态
	if updateCount != 7 {
		t.Errorf("期望7次止损更新（含3%%保本），实际%d次", updateCount)
	}

	if pos.PrevStop >= pos.Entry {
		t.Errorf("最终止损%.2f应该在entry%.2f下方（锁定利润）", pos.PrevStop, pos.Entry)
	}
}
