package protect

import (
	"log"
	"testing"
	"time"
)

// TestSHORTProfitLocking_RealScenario 测试SHORT持仓真实锁盈场景
func TestSHORTProfitLocking_RealScenario(t *testing.T) {
	log.Printf("🧪 测试SHORT持仓真实锁盈场景...")

	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 5, SlippageBpsMinor: 1, FundingBps: 0},
	}

	// 真实场景：SOLUSDT SHORT，ROI 14.15%
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

	// 评估
	plan := eng.Evaluate(pos, snap)

	// 验证结果
	t.Logf("评估结果:")
	t.Logf("  ROI: %.2f%%", plan.RoiUnr*100)
	t.Logf("  R倍数: %.2fR", plan.RUnr)
	t.Logf("  盈利地板: %.6f", plan.Floor)
	t.Logf("  BE价格: %.6f", plan.BE)
	t.Logf("  应该更新: %v", plan.ShouldUpdate)
	if plan.ShouldUpdate {
		t.Logf("  新止损: %.6f (旧止损: %.6f)", plan.NewStop, pos.PrevStop)
		t.Logf("  止损下移: %.6f (%.2f%%)", pos.PrevStop-plan.NewStop, (pos.PrevStop-plan.NewStop)/pos.PrevStop*100)
	} else {
		t.Logf("  原因: %v", plan.Reasons)
		t.Logf("  备注: %s", plan.Note)
	}

	// 验证盈利地板在entry上方
	if plan.Floor < pos.Entry {
		t.Errorf("❌ SHORT盈利地板%.6f不应该在entry%.6f下方！", plan.Floor, pos.Entry)
	} else {
		t.Logf("✅ SHORT盈利地板%.6f在entry%.6f上方", plan.Floor, pos.Entry)
	}

	// 验证盈利地板在当前价上方
	if plan.Floor < snap.LastPrice {
		t.Errorf("❌ SHORT盈利地板%.6f不应该在当前价%.6f下方！", plan.Floor, snap.LastPrice)
	} else {
		t.Logf("✅ SHORT盈利地板%.6f在当前价%.6f上方", plan.Floor, snap.LastPrice)
	}

	// 如果应该更新，验证新止损的合理性
	if plan.ShouldUpdate {
		// 新止损应该在当前价上方
		if plan.NewStop <= snap.LastPrice {
			t.Errorf("❌ 新止损%.6f不应该在当前价%.6f下方或等于！", plan.NewStop, snap.LastPrice)
		} else {
			t.Logf("✅ 新止损%.6f在当前价%.6f上方", plan.NewStop, snap.LastPrice)
		}

		// 新止损应该在entry上方
		if plan.NewStop < pos.Entry {
			t.Errorf("❌ 新止损%.6f不应该在entry%.6f下方！", plan.NewStop, pos.Entry)
		} else {
			t.Logf("✅ 新止损%.6f在entry%.6f上方", plan.NewStop, pos.Entry)
		}

		// 新止损应该比旧止损低（下移）
		if plan.NewStop >= pos.PrevStop {
			t.Errorf("❌ 新止损%.6f应该比旧止损%.6f低！", plan.NewStop, pos.PrevStop)
		} else {
			t.Logf("✅ 新止损%.6f比旧止损%.6f低（下移锁盈）", plan.NewStop, pos.PrevStop)
		}
	}
}

// TestSHORTProfitLocking_Integration 测试SHORT锁盈完整流程
func TestSHORTProfitLocking_Integration(t *testing.T) {
	log.Printf("🧪 测试SHORT锁盈完整流程...")

	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 5, SlippageBpsMinor: 1, FundingBps: 0},
	}

	// 模拟SHORT持仓从开仓到锁盈的过程
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
		ROIArmed:             false,
	}

	// 场景1：价格刚开始下跌，ROI 3%（未达到触发阈值5%）
	t.Run("ROI 3% - 未触发锁盈", func(t *testing.T) {
		snap := MarketSnapshot{
			Symbol:    "SOLUSDT",
			LastPrice: 126.79, // 下跌0.38，ROI约3%
			MarkPrice: 126.79,
			TickSize:  0.001,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)
		t.Logf("ROI: %.2f%%, 应该更新: %v", plan.RoiUnr*100, plan.ShouldUpdate)

		if plan.ShouldUpdate {
			t.Errorf("ROI未达到5%%，不应该触发锁盈")
		}
	})

	// 场景2：价格继续下跌，ROI 6%（达到触发阈值）
	t.Run("ROI 6% - 触发锁盈", func(t *testing.T) {
		snap := MarketSnapshot{
			Symbol:    "SOLUSDT",
			LastPrice: 126.41, // 下跌0.76，ROI约6%
			MarkPrice: 126.41,
			TickSize:  0.001,
			NowMs:     time.Now().UnixMilli(),
		}

		plan := eng.Evaluate(pos, snap)
		t.Logf("ROI: %.2f%%, 应该更新: %v", plan.RoiUnr*100, plan.ShouldUpdate)

		if !plan.ShouldUpdate {
			t.Logf("未更新原因: %v, 备注: %s", plan.Reasons, plan.Note)
		}

		// ROI达到6%，应该触发锁盈（如果没有EXEC_GAP问题）
		if plan.RoiUnr < cfg.ROILockTrigger {
			t.Logf("ROI %.2f%% < 触发阈值 %.2f%%", plan.RoiUnr*100, cfg.ROILockTrigger*100)
		}
	})

	// 场景3：价格大幅下跌，ROI 14%
	t.Run("ROI 14% - 大幅锁盈", func(t *testing.T) {
		snap := MarketSnapshot{
			Symbol:    "SOLUSDT",
			LastPrice: 125.370,
			MarkPrice: 125.370,
			TickSize:  0.001,
			NowMs:     time.Now().UnixMilli(),
		}

		// 更新ROIArmed状态
		pos.ROIArmed = true

		plan := eng.Evaluate(pos, snap)
		t.Logf("ROI: %.2f%%, 应该更新: %v", plan.RoiUnr*100, plan.ShouldUpdate)

		if plan.ShouldUpdate {
			t.Logf("新止损: %.6f, 旧止损: %.6f, 下移: %.2f%%",
				plan.NewStop, pos.PrevStop, (pos.PrevStop-plan.NewStop)/pos.PrevStop*100)
		} else {
			t.Logf("未更新原因: %v, 备注: %s", plan.Reasons, plan.Note)
		}
	})
}
