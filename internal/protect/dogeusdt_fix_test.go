package protect

import (
	"testing"
	"time"
)

// TestDOGEUSDTStopUpdateFix 验证DOGEUSDT止损更新修复
// 问题：TickSize配置错误导致止损无法更新
// 修复：1. TickSize从0.01改为0.00001  2. 盈利地板计算从基于ROI改回基于R0
func TestDOGEUSDTStopUpdateFix(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2, FundingBps: 0},
	}

	// 真实DOGEUSDT持仓数据
	pos := PositionState{
		Symbol:               "DOGEUSDT",
		Side:                 Long,
		Entry:                0.124520,
		Leverage:             10.0,
		InitStop:             0.122800,
		PrevStop:             0.122800,
		Qty:                  1000.0,
		OpenTimeMs:           time.Now().UnixMilli() - 21000000, // 持仓约6小时
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
	}

	// 当前市场快照（ROI = 5.70%）
	snap := MarketSnapshot{
		Symbol:    "DOGEUSDT",
		LastPrice: 0.125230,
		MarkPrice: 0.125230,
		TickSize:  0.00001, // 修复：正确的TickSize
		NowMs:     time.Now().UnixMilli(),
	}

	plan := eng.Evaluate(pos, snap)

	// 验证ROI计算
	expectedROI := 0.057 // 5.7%
	if plan.RoiUnr < expectedROI-0.001 || plan.RoiUnr > expectedROI+0.001 {
		t.Errorf("ROI计算错误: %.4f, 期望约%.4f", plan.RoiUnr, expectedROI)
	}

	// 验证R0计算
	expectedR0 := 0.001720
	if plan.R0 < expectedR0-0.000001 || plan.R0 > expectedR0+0.000001 {
		t.Errorf("R0计算错误: %.6f, 期望%.6f", plan.R0, expectedR0)
	}

	// 验证BE计算（修复后应该是合理的值）
	// BE = entry * (1 + 0.0006) + (3 * 0.00001) = 0.124520 * 1.0006 + 0.00003 = 0.124625
	expectedBE := 0.124625
	if plan.BE < expectedBE-0.000001 || plan.BE > expectedBE+0.000001 {
		t.Errorf("BE计算错误: %.6f, 期望约%.6f", plan.BE, expectedBE)
	}

	// 验证盈利地板计算（修复：基于R0）
	// FloorPx = entry + (R0 * 3%) = 0.124520 + (0.001720 * 0.03) = 0.124572
	// 但因为 BE (0.124625) > FloorPx (0.124572)，所以最终返回BE
	// 这是LONG持仓的BE保护机制
	expectedFloorPx := 0.124572
	expectedFinalFloor := 0.124625 // 返回BE
	if plan.Floor < expectedFinalFloor-0.000010 || plan.Floor > expectedFinalFloor+0.000010 {
		t.Errorf("盈利地板计算错误: %.6f, 期望约%.6f (BE保护)", plan.Floor, expectedFinalFloor)
	}
	t.Logf("   FloorPx (基于R0): %.6f, 最终Floor (取BE): %.6f", expectedFloorPx, plan.Floor)

	// 验证应该触发更新
	if !plan.ShouldUpdate {
		t.Errorf("ROI=5.70%%，应该触发止损更新。原因: %v, 备注: %s", plan.Reasons, plan.Note)
	}

	// 验证新止损价格（应该是BE，因为BE > Floor）
	expectedNewStop := 0.124625
	if plan.NewStop < expectedNewStop-0.000010 || plan.NewStop > expectedNewStop+0.000010 {
		t.Errorf("新止损价格错误: %.6f, 期望约%.6f", plan.NewStop, expectedNewStop)
	}

	// 验证止损上移距离
	stopMove := plan.NewStop - pos.PrevStop
	expectedMove := 0.001825
	if stopMove < expectedMove-0.000010 || stopMove > expectedMove+0.000010 {
		t.Errorf("止损移动距离错误: %.6f, 期望约%.6f", stopMove, expectedMove)
	}

	t.Logf("✅ DOGEUSDT修复验证通过:")
	t.Logf("   ROI: %.2f%%", plan.RoiUnr*100)
	t.Logf("   R0: %.6f", plan.R0)
	t.Logf("   BE: %.6f", plan.BE)
	t.Logf("   Floor: %.6f", plan.Floor)
	t.Logf("   新止损: %.6f (上移 %.6f)", plan.NewStop, stopMove)
}
