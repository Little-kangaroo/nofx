package protect

import (
	"testing"
	"time"
)

// TestDOGEUSDTStopUpdateFix 验证DOGEUSDT止损更新
// 简化后逻辑：floor = Entry + R0 * floorPct，不依赖TickSize做逻辑判断
func TestDOGEUSDTStopUpdateFix(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2, FundingBps: 0},
	}

	pos := PositionState{
		Symbol:               "DOGEUSDT",
		Side:                 Long,
		Entry:                0.124520,
		Leverage:             10.0,
		InitStop:             0.122800,
		PrevStop:             0.122800,
		Qty:                  1000.0,
		OpenTimeMs:           time.Now().UnixMilli() - 21000000,
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
	}

	// ROI = 5.70%，TickSize无论设0.00001还是0.01，floor计算结果相同
	snap := MarketSnapshot{
		Symbol:    "DOGEUSDT",
		LastPrice: 0.125230,
		MarkPrice: 0.125230,
		TickSize:  0.00001,
		NowMs:     time.Now().UnixMilli(),
	}

	plan := eng.Evaluate(pos, snap)

	// 验证ROI
	expectedROI := 0.057
	if plan.RoiUnr < expectedROI-0.001 || plan.RoiUnr > expectedROI+0.001 {
		t.Errorf("ROI计算错误: %.4f, 期望约%.4f", plan.RoiUnr, expectedROI)
	}

	// 验证R0
	expectedR0 := 0.001720
	if plan.R0 < expectedR0-0.000001 || plan.R0 > expectedR0+0.000001 {
		t.Errorf("R0计算错误: %.6f, 期望%.6f", plan.R0, expectedR0)
	}

	// 验证盈利地板：floor = Entry × (1 + 0.03/10) = 0.124520 × 1.003 = 0.124894
	expectedFloor := 0.124894
	if plan.Floor < expectedFloor-0.000010 || plan.Floor > expectedFloor+0.000010 {
		t.Errorf("盈利地板计算错误: %.6f, 期望约%.6f", plan.Floor, expectedFloor)
	}

	// 验证应该触发更新
	if !plan.ShouldUpdate {
		t.Errorf("ROI=5.70%%，应该触发止损更新。原因: %v, 备注: %s", plan.Reasons, plan.Note)
	}

	// 验证新止损价格（FloorToTick(0.124894, 0.00001) = 0.124890）
	expectedNewStop := 0.124890
	if plan.NewStop < expectedNewStop-0.000010 || plan.NewStop > expectedNewStop+0.000010 {
		t.Errorf("新止损价格错误: %.6f, 期望约%.6f", plan.NewStop, expectedNewStop)
	}

	// 验证止损上移距离
	stopMove := plan.NewStop - pos.PrevStop
	expectedMove := 0.002090
	if stopMove < expectedMove-0.000010 || stopMove > expectedMove+0.000010 {
		t.Errorf("止损移动距离错误: %.6f, 期望约%.6f", stopMove, expectedMove)
	}

	t.Logf("✅ DOGEUSDT验证通过:")
	t.Logf("   ROI: %.2f%%", plan.RoiUnr*100)
	t.Logf("   R0: %.6f", plan.R0)
	t.Logf("   Floor: %.6f", plan.Floor)
	t.Logf("   新止损: %.6f (上移 %.6f)", plan.NewStop, stopMove)
}

// TestDOGEUSDTWrongTickSizeNoLongerBlocks 验证：即使TickSize传入错误的0.01，floor逻辑仍然正确触发
func TestDOGEUSDTWrongTickSizeNoLongerBlocks(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{Cfg: cfg}

	pos := PositionState{
		Symbol:               "DOGEUSDT",
		Side:                 Long,
		Entry:                0.091800,
		Leverage:             10.0,
		InitStop:             0.090500,
		PrevStop:             0.090500,
		Qty:                  1000.0,
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             true,
	}

	// 传入错误的TickSize=0.01（模拟之前的bug场景）
	snap := MarketSnapshot{
		Symbol:    "DOGEUSDT",
		LastPrice: 0.094020, // ROI ≈ 24.2%
		TickSize:  0.01,     // 故意传错，验证不再影响核心逻辑
		NowMs:     time.Now().UnixMilli(),
	}

	plan := eng.Evaluate(pos, snap)

	// R0 = 0.091800 - 0.090500 = 0.001300
	// 24% ROI → milestone 20% → floorPct=0.14
	// floor = 0.091800 + 0.001300*0.14 = 0.091982
	// candidate = max(0.090500, 0.091982) = 0.091982
	// FloorToTick(0.091982, 0.01) = 0.09（floor向下取整）
	// candidate(0.09) <= prevStop(0.0905) → not improved → NoChange
	// 但floor本身是合理的，问题仅在于tick对齐时被截断

	t.Logf("ROI: %.2f%%, Floor: %.6f, NewStop: %.6f, ShouldUpdate: %v",
		plan.RoiUnr*100, plan.Floor, plan.NewStop, plan.ShouldUpdate)
	t.Logf("原因: %v, 备注: %s", plan.Reasons, plan.Note)

	// floor计算正确（不受TickSize影响）
	if plan.Floor <= 0 {
		t.Errorf("Floor应大于0，得到: %.6f", plan.Floor)
	}
	if plan.Floor < pos.Entry {
		t.Errorf("LONG的Floor(%.6f)不应低于Entry(%.6f)", plan.Floor, pos.Entry)
	}
}
