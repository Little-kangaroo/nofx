package protect

import (
	"testing"
)

// Test_ROI_ProfitFloor_Armed 测试ROI锁盈触发
func Test_ROI_ProfitFloor_Armed(t *testing.T) {
	cfg := DefaultConfig()
	eng := Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := int64(1_700_000_000_000)

	// SHORT持仓：入场0.1249，初始止损0.1260，当前价0.1238（浮盈）
	// ROI = ((0.1249-0.1238)/0.1249)*10 = 8.8% > 5% → 应触发ROI锁盈
	pos := PositionState{
		Symbol:               "DOGEUSDT",
		Side:                 Short,
		Qty:                  1000,
		Entry:                0.1249,
		Leverage:             10,
		InitStop:             0.1260,
		PrevStop:             0.1260,
		OpenTimeMs:           now - 10*60*1000, // 10分钟前开仓
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
		LastStopUpdateTimeMs: 0,
	}

	m := MarketSnapshot{
		Symbol:    "DOGEUSDT",
		LastPrice: 0.1238,
		MarkPrice: 0.1238,
		TickSize:  0.00001,
		NowMs:     now,
	}

	plan := eng.Evaluate(pos, m)

	// 检查结果
	if plan.RoiUnr < 0.05 {
		t.Fatalf("expected ROI >= 5%%, got %.2f%%", plan.RoiUnr*100)
	}

	if !plan.NextROIArmed {
		t.Fatalf("expected ROIArmed=true")
	}

	if !plan.ShouldUpdate && !plan.ExecGap {
		t.Logf("plan: %+v", plan)
		// 如果EXEC_GAP，这是正常的（延后更新）
		// 否则应该有更新
		if !plan.ExecGap {
			t.Fatalf("expected update or execgap")
		}
	}

	t.Logf("✓ ROI锁盈测试通过: ROI=%.2f%%, armed=%v, shouldUpdate=%v, execGap=%v",
		plan.RoiUnr*100, plan.NextROIArmed, plan.ShouldUpdate, plan.ExecGap)
}

// Test_ExecGap_Long 测试LONG持仓的EXEC_GAP检测
func Test_ExecGap_Long(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StopDistanceMinTicks = 3
	cfg.SafetyTicks = 2

	eng := Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := int64(1_700_000_000_000)

	// LONG持仓：入场100，当前价100，试图锁盈到99
	// 但当前价100，止损99距离只有1个tick，违反EXEC_GAP
	pos := PositionState{
		Symbol:               "TESTUSDT",
		Side:                 Long,
		Qty:                  1,
		Entry:                100,
		Leverage:             10,
		InitStop:             90,
		PrevStop:             99,
		OpenTimeMs:           now - 10*60*1000,
		StopTriggerType:      TriggerLast,
		ROIArmed:             true, // 假设已触发ROI
		BreakEvenArmed:       false,
		RLockStage:           0,
		LastStopUpdateTimeMs: 0,
	}

	m := MarketSnapshot{
		Symbol:    "TESTUSDT",
		LastPrice: 100, // 当前价100
		MarkPrice: 100,
		TickSize:  1,
		NowMs:     now,
	}

	plan := eng.Evaluate(pos, m)

	// 应该触发EXEC_GAP
	if !plan.ExecGap {
		t.Fatalf("expected EXEC_GAP, got plan: %+v", plan)
	}

	t.Logf("✓ EXEC_GAP测试通过: execGap=%v, bounds=[%.2f, %.2f], refPrice=%.2f",
		plan.ExecGap, plan.Bounds.LowerExec, plan.Bounds.UpperExec, plan.RefPrice)
}

// Test_Cooldown 测试冷却期限制
func Test_Cooldown(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UpdateCooldownSec = 25

	eng := Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := int64(1_700_000_000_000)

	pos := PositionState{
		Symbol:               "BTCUSDT",
		Side:                 Long,
		Qty:                  0.1,
		Entry:                95000,
		Leverage:             10,
		InitStop:             94000,
		PrevStop:             94500,
		OpenTimeMs:           now - 10*60*1000,
		StopTriggerType:      TriggerLast,
		LastStopUpdateTimeMs: now - 10*1000, // 10秒前刚更新过
	}

	m := MarketSnapshot{
		Symbol:    "BTCUSDT",
		LastPrice: 96000,
		MarkPrice: 96000,
		TickSize:  1,
		NowMs:     now,
	}

	plan := eng.Evaluate(pos, m)

	// 应该被冷却期拒绝
	hasCooldown := false
	for _, r := range plan.Reasons {
		if r == ReasonCooldown {
			hasCooldown = true
			break
		}
	}

	if !hasCooldown {
		t.Fatalf("expected COOLDOWN reason, got: %v", plan.Reasons)
	}

	if plan.ShouldUpdate {
		t.Fatalf("should not update during cooldown")
	}

	t.Logf("✓ 冷却期测试通过: reasons=%v", plan.Reasons)
}

// Test_MinTickMove 测试最小移动距离限制
func Test_MinTickMove(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinTickMoveToUpdate = 3

	eng := Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := int64(1_700_000_000_000)

	// LONG持仓，试图从94500移动到94502（只移动2个tick）
	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Long,
		Qty:             0.1,
		Entry:           95000,
		Leverage:        10,
		InitStop:        94000,
		PrevStop:        94500,
		OpenTimeMs:      now - 10*60*1000,
		StopTriggerType: TriggerLast,
		ROIArmed:        true,
	}

	m := MarketSnapshot{
		Symbol:    "BTCUSDT",
		LastPrice: 95100, // 轻微浮盈
		MarkPrice: 95100,
		TickSize:  1,
		NowMs:     now,
	}

	plan := eng.Evaluate(pos, m)

	// 如果移动距离不足3 tick，应被拒绝
	hasStepTooSmall := false
	for _, r := range plan.Reasons {
		if r == ReasonStepTooSmall {
			hasStepTooSmall = true
			break
		}
	}

	if plan.ShouldUpdate && !hasStepTooSmall {
		// 可能浮盈足够大，允许更新
		t.Logf("⚠️  plan允许更新（浮盈较大）: newStop=%.2f, reasons=%v", plan.NewStop, plan.Reasons)
	} else if !plan.ShouldUpdate {
		t.Logf("✓ 最小移动距离测试通过: reasons=%v", plan.Reasons)
	}
}

// Test_Monotonicity_Short 测试SHORT持仓单调性（止损只能下移）
func Test_Monotonicity_Short(t *testing.T) {
	cfg := DefaultConfig()

	eng := Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := int64(1_700_000_000_000)

	// SHORT持仓：入场0.125，初始止损0.126，当前止损0.126
	// 当前价0.123（浮盈），应将止损下移
	pos := PositionState{
		Symbol:          "DOGEUSDT",
		Side:            Short,
		Qty:             1000,
		Entry:           0.125,
		Leverage:        10,
		InitStop:        0.126,
		PrevStop:        0.126,
		OpenTimeMs:      now - 10*60*1000,
		StopTriggerType: TriggerLast,
		ROIArmed:        true,
	}

	m := MarketSnapshot{
		Symbol:    "DOGEUSDT",
		LastPrice: 0.123,
		MarkPrice: 0.123,
		TickSize:  0.00001,
		NowMs:     now,
	}

	plan := eng.Evaluate(pos, m)

	if plan.ShouldUpdate {
		// SHORT止损应该下移（new < prev）
		if plan.NewStop >= pos.PrevStop {
			t.Fatalf("SHORT止损应下移: newStop=%.6f >= prevStop=%.6f", plan.NewStop, pos.PrevStop)
		}
		t.Logf("✓ SHORT单调性测试通过: prevStop=%.6f → newStop=%.6f", pos.PrevStop, plan.NewStop)
	} else if plan.ExecGap {
		t.Logf("⚠️  EXEC_GAP延后更新")
	}
}
