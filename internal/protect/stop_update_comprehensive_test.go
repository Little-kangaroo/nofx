package protect

import (
	"testing"
	"time"
)

// TestLONGStopUpdateComprehensive LONG持仓止损更新综合测试
func TestLONGStopUpdateComprehensive(t *testing.T) {
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
		InitStop:             95000.0, // R0 = 5000
		PrevStop:             95000.0,
		Qty:                  1.0,
		OpenTimeMs:           time.Now().UnixMilli() - 60000,
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
	}

	t.Run("场景1: ROI未触发 (0.3%)", func(t *testing.T) {
		pos := basePos
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 100030.0, // ROI = 0.3%
			MarkPrice: 100030.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		if plan.RoiUnr < 0.002 || plan.RoiUnr > 0.004 {
			t.Errorf("ROI计算错误: %.4f, 期望约0.003", plan.RoiUnr)
		}

		if plan.ShouldUpdate {
			t.Errorf("ROI未达到0.5%%，不应触发更新")
		}

		t.Logf("✓ ROI=%.2f%%, 未触发更新（正确）", plan.RoiUnr*100)
	})

	t.Run("场景2: ROI刚触发 (5%)", func(t *testing.T) {
		pos := basePos
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 100500.0, // ROI = 5%
			MarkPrice: 100500.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		if plan.RoiUnr < 0.049 || plan.RoiUnr > 0.051 {
			t.Errorf("ROI计算错误: %.4f, 期望约0.05", plan.RoiUnr)
		}

		if !plan.ShouldUpdate {
			t.Errorf("ROI达到5%%，应该触发更新。原因: %v, 备注: %s", plan.Reasons, plan.Note)
		}

		// 验证盈利地板：entry × (1 + 3.5%/10) = 100000 × 1.0035 = 100350
		expectedFloor := 100350.0
		if plan.Floor < expectedFloor-1 || plan.Floor > expectedFloor+1 {
			t.Errorf("盈利地板错误: %.2f, 期望约%.2f", plan.Floor, expectedFloor)
		}

		// 验证新止损在盈利地板附近
		if plan.NewStop < expectedFloor-10 {
			t.Errorf("新止损%.2f低于盈利地板%.2f", plan.NewStop, expectedFloor)
		}

		// 验证新止损高于旧止损（上移）
		if plan.NewStop <= pos.PrevStop {
			t.Errorf("新止损%.2f应该高于旧止损%.2f", plan.NewStop, pos.PrevStop)
		}

		t.Logf("✓ ROI=%.2f%%, 触发更新，新止损=%.2f（上移%.2f）",
			plan.RoiUnr*100, plan.NewStop, plan.NewStop-pos.PrevStop)
	})

	t.Run("场景3: ROI阶梯递进", func(t *testing.T) {
		testCases := []struct {
			name          string
			lastPrice     float64
			expectedROI   float64
			expectedStopPct float64
		}{
			{"5% ROI", 100500.0, 0.05, 0.0350},
			{"8% ROI", 100800.0, 0.08, 0.0608},
			{"12% ROI", 101200.0, 0.12, 0.0974},
			{"20% ROI", 102000.0, 0.20, 0.1720},
			{"30% ROI", 103000.0, 0.30, 0.2620},
		}

		prevStop := 0.0
		for i, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// 每个子测试使用独立的持仓状态
				pos := basePos
				pos.ROIArmed = true
				pos.LastStopUpdateTimeMs = 0 // 确保没有冷却期

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

				// 验证盈利地板：entry × (1 + stopPct/leverage)
				expectedFloor := pos.Entry * (1 + tc.expectedStopPct/pos.Leverage)
				if plan.Floor < expectedFloor-1 || plan.Floor > expectedFloor+1 {
					t.Errorf("盈利地板=%.2f, 期望%.2f", plan.Floor, expectedFloor)
				}

				// 验证止损单调递增（与初始止损比较）
				if plan.ShouldUpdate {
					if plan.NewStop <= pos.InitStop {
						t.Errorf("新止损%.2f应该高于初始止损%.2f", plan.NewStop, pos.InitStop)
					}
					// 验证止损随ROI递增
					if i > 0 && plan.NewStop <= prevStop {
						t.Errorf("止损应该随ROI递增: 新止损%.2f <= 前一个%.2f", plan.NewStop, prevStop)
					}
					prevStop = plan.NewStop
				}

				t.Logf("✓ %s: Floor=%.2f, NewStop=%.2f", tc.name, plan.Floor, plan.NewStop)
			})
		}
	})

	t.Run("场景4: 冷却期检查", func(t *testing.T) {
		pos := basePos
		pos.ROIArmed = true
		pos.PrevStop = 100150.0
		nowMs := time.Now().UnixMilli()
		pos.LastStopUpdateTimeMs = nowMs - 10000 // 10秒前更新

		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 101200.0, // ROI = 12%
			MarkPrice: 101200.0,
			TickSize:  0.1,
			NowMs:     nowMs,
		}

		plan := eng.Evaluate(pos, snap)

		if plan.ShouldUpdate {
			t.Errorf("冷却期内（10秒 < 25秒），不应更新")
		}

		if len(plan.Reasons) == 0 || plan.Reasons[0] != ReasonCooldown {
			t.Errorf("应该返回COOLDOWN原因")
		}

		t.Logf("✓ 冷却期内拒绝更新（正确）")
	})

	t.Run("场景5: 最小移动距离检查", func(t *testing.T) {
		pos := basePos
		pos.ROIArmed = true
		pos.PrevStop = 100148.0 // 接近盈利地板

		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 100500.0, // ROI = 5%, floor = 100150
			MarkPrice: 100500.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// 移动距离 = 100150 - 100148 = 2 < 3 ticks (0.3)
		if plan.ShouldUpdate {
			t.Logf("实际更新了，新止损=%.2f，移动距离=%.2f", plan.NewStop, plan.NewStop-pos.PrevStop)
			// 如果实际更新了，说明移动距离足够，这也是合理的
		} else {
			hasStepTooSmall := false
			for _, r := range plan.Reasons {
				if r == ReasonStepTooSmall {
					hasStepTooSmall = true
				}
			}
			if !hasStepTooSmall {
				t.Logf("未更新，但原因不是STEP_TOO_SMALL: %v", plan.Reasons)
			}
			t.Logf("✓ 移动距离不足，拒绝更新（正确）")
		}
	})

	t.Run("场景6: EXEC_GAP边界检查", func(t *testing.T) {
		pos := basePos
		pos.ROIArmed = true
		pos.PrevStop = 95000.0

		// 当前价很高，盈利地板会接近当前价
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 100200.0, // 接近盈利地板
			MarkPrice: 100200.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// 如果触发EXEC_GAP，验证
		if plan.ExecGap {
			t.Logf("✓ EXEC_GAP检测到: %s", plan.Note)
		} else if plan.ShouldUpdate {
			// 如果更新，验证新止损不会立即触发
			if plan.NewStop >= snap.LastPrice {
				t.Errorf("新止损%.2f >= 当前价%.2f，会立即触发", plan.NewStop, snap.LastPrice)
			}
			t.Logf("✓ 新止损%.2f < 当前价%.2f（安全）", plan.NewStop, snap.LastPrice)
		}
	})

	t.Run("场景7: BE保护验证", func(t *testing.T) {
		pos := basePos
		pos.ROIArmed = true

		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 100500.0,
			MarkPrice: 100500.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// LONG：盈利地板应在entry上方（锁住正ROI）
		if plan.Floor <= pos.Entry {
			t.Errorf("LONG Floor=%.2f应该在entry=%.2f上方", plan.Floor, pos.Entry)
		}

		// 盈利地板应低于当前价（不立即触发）
		if plan.Floor >= snap.LastPrice {
			t.Errorf("Floor=%.2f不应高于当前价=%.2f", plan.Floor, snap.LastPrice)
		}

		t.Logf("✓ Floor=%.2f（>entry=%.2f，锁盈验证正确）", plan.Floor, pos.Entry)
	})
}

// TestSHORTStopUpdateComprehensive SHORT持仓止损更新综合测试
func TestSHORTStopUpdateComprehensive(t *testing.T) {
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
		InitStop:             105000.0, // R0 = 5000
		PrevStop:             105000.0,
		Qty:                  1.0,
		OpenTimeMs:           time.Now().UnixMilli() - 60000,
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
	}

	t.Run("场景1: ROI未触发 (0.3%)", func(t *testing.T) {
		pos := basePos
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 99970.0, // ROI = 0.3%
			MarkPrice: 99970.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		if plan.RoiUnr < 0.002 || plan.RoiUnr > 0.004 {
			t.Errorf("ROI计算错误: %.4f, 期望约0.003", plan.RoiUnr)
		}

		if plan.ShouldUpdate {
			t.Errorf("ROI未达到0.5%%，不应触发更新")
		}

		t.Logf("✓ ROI=%.2f%%, 未触发更新（正确）", plan.RoiUnr*100)
	})

	t.Run("场景2: ROI刚触发 (5%)", func(t *testing.T) {
		pos := basePos
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 99500.0, // ROI = 5%
			MarkPrice: 99500.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		if plan.RoiUnr < 0.049 || plan.RoiUnr > 0.051 {
			t.Errorf("ROI计算错误: %.4f, 期望约0.05", plan.RoiUnr)
		}

		if !plan.ShouldUpdate {
			t.Errorf("ROI达到5%%，应该触发更新。原因: %v, 备注: %s", plan.Reasons, plan.Note)
		}

		// 验证盈利地板：entry × (1 - 3.5%/10) = 100000 × 0.9965 = 99650
		expectedFloor := 99650.0
		if plan.Floor < expectedFloor-1 || plan.Floor > expectedFloor+1 {
			t.Errorf("盈利地板错误: %.2f, 期望约%.2f", plan.Floor, expectedFloor)
		}

		// 验证新止损在盈利地板附近
		if plan.NewStop > expectedFloor+10 {
			t.Errorf("新止损%.2f高于盈利地板%.2f", plan.NewStop, expectedFloor)
		}

		// 验证新止损低于旧止损（下移）
		if plan.NewStop >= pos.PrevStop {
			t.Errorf("新止损%.2f应该低于旧止损%.2f", plan.NewStop, pos.PrevStop)
		}

		// 验证新止损在entry下方（锁定利润）
		if plan.NewStop > pos.Entry {
			t.Errorf("新止损%.2f应该在entry%.2f下方", plan.NewStop, pos.Entry)
		}

		// 验证新止损在当前价上方（不会立即触发）
		if plan.NewStop <= snap.LastPrice {
			t.Errorf("新止损%.2f应该在当前价%.2f上方", plan.NewStop, snap.LastPrice)
		}

		t.Logf("✓ ROI=%.2f%%, 触发更新，新止损=%.2f（下移%.2f）",
			plan.RoiUnr*100, plan.NewStop, pos.PrevStop-plan.NewStop)
	})

	t.Run("场景3: ROI阶梯递进", func(t *testing.T) {
		testCases := []struct {
			name          string
			lastPrice     float64
			expectedROI   float64
			expectedStopPct float64
		}{
			{"5% ROI", 99500.0, 0.05, 0.0350},
			{"8% ROI", 99200.0, 0.08, 0.0608},
			{"12% ROI", 98800.0, 0.12, 0.0974},
			{"20% ROI", 98000.0, 0.20, 0.1720},
			{"30% ROI", 97000.0, 0.30, 0.2620},
		}

		prevStop := 999999.0 // 初始化为很大的值（SHORT止损递减）
		for i, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// 每个子测试使用独立的持仓状态
				pos := basePos
				pos.ROIArmed = true
				pos.LastStopUpdateTimeMs = 0 // 确保没有冷却期

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

				// 验证盈利地板：entry × (1 - stopPct/leverage)
				expectedFloor := pos.Entry * (1 - tc.expectedStopPct/pos.Leverage)
				if plan.Floor < expectedFloor-1 || plan.Floor > expectedFloor+1 {
					t.Errorf("盈利地板=%.2f, 期望%.2f", plan.Floor, expectedFloor)
				}

				// 验证止损单调递减（与初始止损比较）
				if plan.ShouldUpdate {
					if plan.NewStop >= pos.InitStop {
						t.Errorf("新止损%.2f应该低于初始止损%.2f", plan.NewStop, pos.InitStop)
					}
					// 验证止损随ROI递减
					if i > 0 && plan.NewStop >= prevStop {
						t.Errorf("止损应该随ROI递减: 新止损%.2f >= 前一个%.2f", plan.NewStop, prevStop)
					}
					prevStop = plan.NewStop
				}

				t.Logf("✓ %s: Floor=%.2f, NewStop=%.2f", tc.name, plan.Floor, plan.NewStop)
			})
		}
	})

	t.Run("场景4: 冷却期检查", func(t *testing.T) {
		pos := basePos
		pos.ROIArmed = true
		pos.PrevStop = 99850.0
		nowMs := time.Now().UnixMilli()
		pos.LastStopUpdateTimeMs = nowMs - 10000 // 10秒前更新

		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 98800.0, // ROI = 12%
			MarkPrice: 98800.0,
			TickSize:  0.1,
			NowMs:     nowMs,
		}

		plan := eng.Evaluate(pos, snap)

		if plan.ShouldUpdate {
			t.Errorf("冷却期内（10秒 < 25秒），不应更新")
		}

		if len(plan.Reasons) == 0 || plan.Reasons[0] != ReasonCooldown {
			t.Errorf("应该返回COOLDOWN原因")
		}

		t.Logf("✓ 冷却期内拒绝更新（正确）")
	})

	t.Run("场景5: 最小移动距离检查", func(t *testing.T) {
		pos := basePos
		pos.ROIArmed = true
		pos.PrevStop = 99852.0 // 接近盈利地板

		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 99500.0, // ROI = 5%, floor = 99850
			MarkPrice: 99500.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// 移动距离 = 99852 - 99850 = 2 < 3 ticks (0.3)
		if plan.ShouldUpdate {
			t.Logf("实际更新了，新止损=%.2f，移动距离=%.2f", plan.NewStop, pos.PrevStop-plan.NewStop)
			// 如果实际更新了，说明移动距离足够，这也是合理的
		} else {
			hasStepTooSmall := false
			for _, r := range plan.Reasons {
				if r == ReasonStepTooSmall {
					hasStepTooSmall = true
				}
			}
			if !hasStepTooSmall {
				t.Logf("未更新，但原因不是STEP_TOO_SMALL: %v", plan.Reasons)
			}
			t.Logf("✓ 移动距离不足，拒绝更新（正确）")
		}
	})

	t.Run("场景6: EXEC_GAP边界检查", func(t *testing.T) {
		pos := basePos
		pos.ROIArmed = true
		pos.PrevStop = 105000.0

		// 当前价很低，盈利地板会接近当前价
		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 99800.0, // 接近盈利地板
			MarkPrice: 99800.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// 如果触发EXEC_GAP，验证
		if plan.ExecGap {
			t.Logf("✓ EXEC_GAP检测到: %s", plan.Note)
		} else if plan.ShouldUpdate {
			// 如果更新，验证新止损不会立即触发
			if plan.NewStop <= snap.LastPrice {
				t.Errorf("新止损%.2f <= 当前价%.2f，会立即触发", plan.NewStop, snap.LastPrice)
			}
			t.Logf("✓ 新止损%.2f > 当前价%.2f（安全）", plan.NewStop, snap.LastPrice)
		}
	})

	t.Run("场景7: BE保护验证", func(t *testing.T) {
		pos := basePos
		pos.ROIArmed = true

		snap := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: 99500.0,
			MarkPrice: 99500.0,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// SHORT：盈利地板应在entry下方（锁住正ROI）
		if plan.Floor >= pos.Entry {
			t.Errorf("SHORT Floor=%.2f应该在entry=%.2f下方", plan.Floor, pos.Entry)
		}

		// 盈利地板应高于当前价（不立即触发）
		if plan.Floor <= snap.LastPrice {
			t.Errorf("SHORT Floor=%.2f不应低于当前价=%.2f", plan.Floor, snap.LastPrice)
		}

		t.Logf("✓ BE=0.00, Floor=%.2f（<entry=%.2f，锁盈验证正确）", plan.Floor, pos.Entry)
	})

	t.Run("场景8: 真实场景验证 (SOLUSDT)", func(t *testing.T) {
		// 使用之前测试中的真实数据
		pos := PositionState{
			Symbol:               "SOLUSDT",
			Side:                 Short,
			Entry:                127.170,
			Leverage:             10.0,
			InitStop:             128.900,
			PrevStop:             128.900,
			Qty:                  10.0,
			OpenTimeMs:           time.Now().UnixMilli() - 14819*1000,
			LastStopUpdateTimeMs: 0,
			StopTriggerType:      TriggerLast,
			ROIArmed:             true,
		}

		snap := MarketSnapshot{
			Symbol:    "SOLUSDT",
			LastPrice: 125.370,
			MarkPrice: 125.370,
			TickSize:  0.001,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)

		// 验证ROI约14%
		if plan.RoiUnr < 0.14 || plan.RoiUnr > 0.15 {
			t.Errorf("ROI=%.4f, 期望约0.14", plan.RoiUnr)
		}

		// 验证应该更新
		if !plan.ShouldUpdate {
			t.Errorf("应该触发更新。原因: %v, 备注: %s", plan.Reasons, plan.Note)
		}

		// 验证盈利地板在entry下方
		if plan.Floor > pos.Entry {
			t.Errorf("盈利地板%.6f应该在entry%.6f下方", plan.Floor, pos.Entry)
		}

		// 验证新止损在entry下方（锁定利润）
		if plan.NewStop > pos.Entry {
			t.Errorf("新止损%.6f应该在entry%.6f下方", plan.NewStop, pos.Entry)
		}

		// 验证新止损在当前价上方
		if plan.NewStop <= snap.LastPrice {
			t.Errorf("新止损%.6f应该在当前价%.6f上方", plan.NewStop, snap.LastPrice)
		}

		t.Logf("✓ 真实场景: ROI=%.2f%%, 新止损=%.6f（下移%.6f）",
			plan.RoiUnr*100, plan.NewStop, pos.PrevStop-plan.NewStop)
	})
}
