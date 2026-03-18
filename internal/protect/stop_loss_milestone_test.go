package protect

import (
	"testing"
)

// TestStopLossMilestones 测试止损阶梯功能
// 语义：floorPct 表示"锁住 X% ROI"
// 公式：floor = Entry × (1 + floorPct/leverage)  LONG
//        floor = Entry × (1 - floorPct/leverage)  SHORT
func TestStopLossMilestones(t *testing.T) {
	cfg := DefaultConfig()
	eng := Engine{Cfg: cfg}

	tests := []struct {
		name            string
		roi             float64
		expectedStopPct float64 // 应锁住的ROI百分比（V-22.0）
		lastPrice       float64
		expectedFloor   float64 // Entry × (1 + floorPct/leverage)
	}{
		{"5% ROI → 3.5% 止损", 0.05, 0.0350, 100500, 100350},  // 100000 × 1.0035
		{"8% ROI → 6.08% 止损", 0.08, 0.0608, 100800, 100608}, // 100000 × 1.00608
		{"12% ROI → 9.74% 止损", 0.12, 0.0974, 101200, 100974}, // 100000 × 1.00974
		{"20% ROI → 17.2% 止损", 0.20, 0.1720, 102000, 101720}, // 100000 × 1.0172
		{"30% ROI → 26.2% 止损", 0.30, 0.2620, 103000, 102620}, // 100000 × 1.0262
		{"50% ROI → 45% 止损", 0.50, 0.4500, 105000, 104500},   // 100000 × 1.045
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := PositionState{
				Symbol:          "BTCUSDT",
				Side:            Long,
				Entry:           100000,
				InitStop:        95000,
				PrevStop:        95000,
				Leverage:        10,
				Qty:             1.0,
				ROIArmed:        true,
				OpenTimeMs:      1000000,
				StopTriggerType: TriggerLast,
			}

			m := MarketSnapshot{
				Symbol:    "BTCUSDT",
				LastPrice: tt.lastPrice,
				MarkPrice: tt.lastPrice,
				TickSize:  0.1,
				NowMs:     1000000 + 30000,
			}

			plan := eng.Evaluate(pos, m)

			// 验证盈利地板：floor = Entry × (1 + floorPct/leverage)
			if plan.Floor < tt.expectedFloor-1 || plan.Floor > tt.expectedFloor+1 {
				t.Errorf("Floor = %.2f, want %.2f (entry×(1+%.2f%%/lev))",
					plan.Floor, tt.expectedFloor, tt.expectedStopPct*100)
			}

			// 验证ROI at floor 等于 floorPct（锁住的ROI）
			roiAtFloor := (plan.Floor-pos.Entry)/pos.Entry*pos.Leverage
			if roiAtFloor < tt.expectedStopPct-0.001 || roiAtFloor > tt.expectedStopPct+0.001 {
				t.Errorf("ROI at floor = %.2f%%, want %.2f%%", roiAtFloor*100, tt.expectedStopPct*100)
			}

			// 验证ROI计算
			if plan.RoiUnr < tt.roi-0.001 || plan.RoiUnr > tt.roi+0.001 {
				t.Errorf("RoiUnr = %.4f, want %.4f", plan.RoiUnr, tt.roi)
			}

			t.Logf("✓ %s: Floor=%.2f → 锁住ROI=%.2f%% (当前ROI=%.2f%%)",
				tt.name, plan.Floor, roiAtFloor*100, plan.RoiUnr*100)
		})
	}
}

// TestStopLossMilestonesMonotonicity 测试止损阶梯的单调性
func TestStopLossMilestonesMonotonicity(t *testing.T) {
	cfg := DefaultConfig()
	eng := Engine{Cfg: cfg}

	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Long,
		Entry:           100000,
		InitStop:        95000,
		PrevStop:        95000,
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        false,
		OpenTimeMs:      1000000,
		StopTriggerType: TriggerLast,
	}

	// Entry=100000, Leverage=10
	// floor = Entry × (1 + floorPct/leverage)
	priceSteps := []struct {
		price       float64
		expectedROI float64
		minFloor    float64
	}{
		{100500, 0.05, 100350}, // 5%→3.5%: 100000×1.0035
		{100800, 0.08, 100608}, // 8%→6.08%: 100000×1.00608
		{101200, 0.12, 100974}, // 12%→9.74%: 100000×1.00974
		{102000, 0.20, 101720}, // 20%→17.2%: 100000×1.0172
		{103000, 0.30, 102620}, // 30%→26.2%: 100000×1.0262
	}

	prevFloor := 0.0
	for i, step := range priceSteps {
		m := MarketSnapshot{
			Symbol:    "BTCUSDT",
			LastPrice: step.price,
			MarkPrice: step.price,
			TickSize:  0.1,
			NowMs:     1000000 + int64(i*30000),
		}

		plan := eng.Evaluate(pos, m)

		if plan.RoiUnr < step.expectedROI-0.001 || plan.RoiUnr > step.expectedROI+0.001 {
			t.Errorf("Step %d: RoiUnr = %.4f, want %.4f", i, plan.RoiUnr, step.expectedROI)
		}

		if plan.Floor < prevFloor {
			t.Errorf("Step %d: Floor regressed from %.2f to %.2f", i, prevFloor, plan.Floor)
		}

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
	eng := Engine{Cfg: cfg}

	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Short,
		Entry:           100000,
		InitStop:        105000,
		PrevStop:        105000,
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        true,
		OpenTimeMs:      1000000,
		StopTriggerType: TriggerLast,
	}

	tests := []struct {
		name            string
		lastPrice       float64
		expectedStopPct float64
		expectedFloor   float64
	}{
		{"5% ROI → 3.5% 止损", 99500, 0.0350, 99650},  // 100000×(1-0.0035)
		{"12% ROI → 9.74% 止损", 98800, 0.0974, 99026}, // 100000×(1-0.00974)
		{"30% ROI → 26.2% 止损", 97000, 0.2620, 97380}, // 100000×(1-0.0262)
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

			// SHORT: floor = Entry × (1 - floorPct/leverage)
			if plan.Floor < tt.expectedFloor-1 || plan.Floor > tt.expectedFloor+1 {
				t.Errorf("Floor = %.2f, want %.2f (entry×(1-%.2f%%/lev))",
					plan.Floor, tt.expectedFloor, tt.expectedStopPct*100)
			}

			// 验证ROI at floor
			roiAtFloor := (pos.Entry-plan.Floor)/pos.Entry*pos.Leverage
			if roiAtFloor < tt.expectedStopPct-0.001 || roiAtFloor > tt.expectedStopPct+0.001 {
				t.Errorf("ROI at floor = %.2f%%, want %.2f%%", roiAtFloor*100, tt.expectedStopPct*100)
			}

			t.Logf("✓ %s: Floor=%.2f → 锁住ROI=%.2f%% (当前ROI=%.2f%%)",
				tt.name, plan.Floor, roiAtFloor*100, plan.RoiUnr*100)
		})
	}
}
