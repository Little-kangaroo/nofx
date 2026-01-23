package protect

import (
	"testing"
)

// TestStopLossMilestones 测试止损阶梯功能
func TestStopLossMilestones(t *testing.T) {
	cfg := DefaultConfig()
	eng := Engine{
		Cfg: cfg,
		Fees: FeeModel{
			TakerFeeBps:      5,
			SlippageBpsMinor: 5,
			FundingBps:       1,
		},
	}

	tests := []struct {
		name           string
		roi            float64
		expectedStopPct float64
		lastPrice      float64
	}{
		{
			name:           "5% ROI → 3% 止损",
			roi:            0.05,
			expectedStopPct: 0.03,
			lastPrice:      100500, // 5% ROI @ 10x = 0.5% price move
		},
		{
			name:           "8% ROI → 5% 止损",
			roi:            0.08,
			expectedStopPct: 0.05,
			lastPrice:      100800,
		},
		{
			name:           "12% ROI → 8% 止损",
			roi:            0.12,
			expectedStopPct: 0.08,
			lastPrice:      101200,
		},
		{
			name:           "20% ROI → 14% 止损",
			roi:            0.20,
			expectedStopPct: 0.14,
			lastPrice:      102000,
		},
		{
			name:           "30% ROI → 22% 止损",
			roi:            0.30,
			expectedStopPct: 0.22,
			lastPrice:      103000,
		},
		{
			name:           "50% ROI → 38% 止损",
			roi:            0.50,
			expectedStopPct: 0.38,
			lastPrice:      105000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := PositionState{
				Symbol:    "BTCUSDT",
				Side:      Long,
				Entry:     100000,
				InitStop:  95000,
				PrevStop:  95000,
				Leverage:  10,
				Qty:       1.0,
				ROIArmed:  true, // 已触发ROI锁盈
				OpenTimeMs: 1000000,
				StopTriggerType: TriggerLast,
			}

			m := MarketSnapshot{
				Symbol:    "BTCUSDT",
				LastPrice: tt.lastPrice,
				MarkPrice: tt.lastPrice,
				TickSize:  0.1,
				NowMs:     1000000 + 30000, // 30秒后
			}

			plan := eng.Evaluate(pos, m)

			// 验证盈利地板是否正确（V-21.3：基于R0的计算）
			// Floor = Entry + (R0 * stopPct)
			r0 := pos.Entry - pos.InitStop // LONG: 100000 - 95000 = 5000
			expectedFloor := pos.Entry + (r0 * tt.expectedStopPct)
			if plan.Floor < expectedFloor-1 || plan.Floor > expectedFloor+1 {
				t.Errorf("Floor = %.2f, want %.2f (entry + R0*%.2f%%)",
					plan.Floor, expectedFloor, tt.expectedStopPct*100)
			}

			// 验证ROI计算
			if plan.RoiUnr < tt.roi-0.001 || plan.RoiUnr > tt.roi+0.001 {
				t.Errorf("RoiUnr = %.4f, want %.4f", plan.RoiUnr, tt.roi)
			}

			t.Logf("✓ %s: Floor=%.2f (entry+%.2f%%), ROI=%.2f%%",
				tt.name, plan.Floor, tt.expectedStopPct*100, plan.RoiUnr*100)
		})
	}
}

// TestStopLossMilestonesMonotonicity 测试止损阶梯的单调性
func TestStopLossMilestonesMonotonicity(t *testing.T) {
	cfg := DefaultConfig()
	eng := Engine{
		Cfg: cfg,
		Fees: FeeModel{
			TakerFeeBps:      5,
			SlippageBpsMinor: 5,
			FundingBps:       1,
		},
	}

	pos := PositionState{
		Symbol:    "BTCUSDT",
		Side:      Long,
		Entry:     100000,
		InitStop:  95000,
		PrevStop:  95000,
		Leverage:  10,
		Qty:       1.0,
		ROIArmed:  false,
		OpenTimeMs: 1000000,
		StopTriggerType: TriggerLast,
	}

	// 模拟价格上涨过程（V-21.3：基于R0的计算）
	// R0 = 100000 - 95000 = 5000
	// Floor = Entry + (R0 * stopPct)
	priceSteps := []struct {
		price       float64
		expectedROI float64
		minFloor    float64
	}{
		{100500, 0.05, 100150},  // 5% ROI → floor >= 100150 (entry + R0*3%)
		{100800, 0.08, 100250},  // 8% ROI → floor >= 100250 (entry + R0*5%)
		{101200, 0.12, 100400},  // 12% ROI → floor >= 100400 (entry + R0*8%)
		{102000, 0.20, 100700},  // 20% ROI → floor >= 100700 (entry + R0*14%)
		{103000, 0.30, 101100},  // 30% ROI → floor >= 101100 (entry + R0*22%)
	}

	prevFloor := 0.0
	for i, step := range priceSteps {
		m := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: step.price,
			MarkPrice: step.price,
			TickSize:  0.1,
			NowMs:     1000000 + int64(i*30000), // 每30秒一次
		}

		plan := eng.Evaluate(pos, m)

		// 验证ROI
		if plan.RoiUnr < step.expectedROI-0.001 || plan.RoiUnr > step.expectedROI+0.001 {
			t.Errorf("Step %d: RoiUnr = %.4f, want %.4f", i, plan.RoiUnr, step.expectedROI)
		}

		// 验证盈利地板单调递增
		if plan.Floor < prevFloor {
			t.Errorf("Step %d: Floor regressed from %.2f to %.2f", i, prevFloor, plan.Floor)
		}

		// 验证盈利地板至少达到预期
		if plan.Floor < step.minFloor-1 {
			t.Errorf("Step %d: Floor = %.2f, want >= %.2f", i, plan.Floor, step.minFloor)
		}

		t.Logf("Step %d: Price=%.2f, ROI=%.2f%%, Floor=%.2f (min=%.2f)",
			i, step.price, plan.RoiUnr*100, plan.Floor, step.minFloor)

		prevFloor = plan.Floor
		pos.ROIArmed = plan.NextROIArmed
		if plan.ShouldUpdate {
			pos.PrevStop = plan.NewStop
		}
	}
}

// TestStopLossMilestonesShort 测试做空的止损阶梯
func TestStopLossMilestonesShort(t *testing.T) {
	cfg := DefaultConfig()
	eng := Engine{
		Cfg: cfg,
		Fees: FeeModel{
			TakerFeeBps:      5,
			SlippageBpsMinor: 5,
			FundingBps:       1,
		},
	}

	pos := PositionState{
		Symbol:    "BTCUSDT",
		Side:      Short,
		Entry:     100000,
		InitStop:  105000,
		PrevStop:  105000,
		Leverage:  10,
		Qty:       1.0,
		ROIArmed:  true,
		OpenTimeMs: 1000000,
		StopTriggerType: TriggerLast,
	}

	tests := []struct {
		name           string
		lastPrice      float64
		expectedStopPct float64
	}{
		{
			name:           "5% ROI → 3% 止损",
			lastPrice:      99500,
			expectedStopPct: 0.03,
		},
		{
			name:           "12% ROI → 8% 止损",
			lastPrice:      98800,
			expectedStopPct: 0.08,
		},
		{
			name:           "30% ROI → 22% 止损",
			lastPrice:      97000,
			expectedStopPct: 0.22,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := MarketSnapshot{
				Symbol:    "BTCUSDT",
				LastPrice: tt.lastPrice,
				MarkPrice: tt.lastPrice,
				TickSize:  0.1,
				NowMs:     1000000 + 30000,
			}

			plan := eng.Evaluate(pos, m)

			// SHORT: floor应该是 entry - (R0 * stopPct)（V-21.3修复）
			r0 := pos.InitStop - pos.Entry // SHORT: 105000 - 100000 = 5000
			expectedFloor := pos.Entry - (r0 * tt.expectedStopPct)
			if plan.Floor < expectedFloor-1 || plan.Floor > expectedFloor+1 {
				t.Errorf("Floor = %.2f, want %.2f (entry - R0*%.2f%%)",
					plan.Floor, expectedFloor, tt.expectedStopPct*100)
			}

			t.Logf("✓ %s: Floor=%.2f (entry-%.2f%%), ROI=%.2f%%",
				tt.name, plan.Floor, tt.expectedStopPct*100, plan.RoiUnr*100)
		})
	}
}
