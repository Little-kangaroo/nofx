package protect

// breakeven_milestone_test.go
//
// 3% ROI 保本里程碑专项验证
//
// 覆盖维度：
//   一、基础触发（Long / Short）
//   二、ROI 门槛边界（低于不触发 / 精确触发）
//   三、ROIArmed 状态机（首次调用激活并执行）
//   四、保本价格精确等于入场价（多标的 × 多杠杆）
//   五、单调性保护（止损已高于入场价时不回退）
//   六、ExecGap 安全门（价格恰好在入场价时拒绝）
//   七、梯度递进（保本 → 5% → 8% … 全链路）
//   八、完整模拟（含保本的逐步价格序列）
//   九、止损触发时 ROI = 0 保证

import (
	"fmt"
	"math"
	"testing"
)

// breakevenMilestones 从 DefaultConfig 动态生成，与配置完全对齐
// V-22.0: 1%粒度里程碑，关键节点：2%→保本, 5%→锁3.5%, 10%→锁8%, 20%→锁17.2%, 30%→锁26.2%
var breakevenMilestones = func() []prodMilestone {
	cfg := DefaultConfig()
	// 取关键档位用于梯度测试
	keyROIs := []float64{0.02, 0.03, 0.05, 0.08, 0.10, 0.20, 0.30, 0.50, 0.80, 1.00}
	var ms []prodMilestone
	for _, roi := range keyROIs {
		if floor, ok := cfg.StopLossMilestones[roi]; ok {
			ms = append(ms, prodMilestone{roi, floor})
		}
	}
	return ms
}()

// ─────────────────────────────────────────────────────────────────
// 一、基础触发
// ─────────────────────────────────────────────────────────────────

// TestBreakeven_Long_BasicTrigger LONG 2% ROI 触发保本（V-22.0: 保本点从3%降至2%）
func TestBreakeven_Long_BasicTrigger(t *testing.T) {
	eng := prodEng()

	// Entry=100000, Leverage=10
	// 2% ROI → LastPrice = 100000 × (1 + 0.02/10) = 100200
	// 保本 floor = Entry × (1 + 0.00/10) = 100000
	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Long,
		Entry:           100000,
		InitStop:        95000,
		PrevStop:        95000,
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        false,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}
	price := 100200.0
	plan := eng.Evaluate(pos, MarketSnapshot{
		Symbol: "BTCUSDT", LastPrice: price, MarkPrice: price, TickSize: 0.1, NowMs: 1_030_000,
	})

	// ROI ≈ 2%
	if math.Abs(plan.RoiUnr-0.02) > 0.001 {
		t.Errorf("RoiUnr=%.4f, want ~0.02", plan.RoiUnr)
	}
	// ROIArmed 应被激活
	if !plan.NextROIArmed {
		t.Errorf("NextROIArmed=false, want true")
	}
	// 应触发更新
	if !plan.ShouldUpdate {
		t.Errorf("ShouldUpdate=false, want true; reasons=%v note=%s", plan.Reasons, plan.Note)
	}
	// 保本：floor = entry
	if math.Abs(plan.Floor-pos.Entry) > 1 {
		t.Errorf("Floor=%.2f, want entry=%.2f (break-even)", plan.Floor, pos.Entry)
	}
	// NewStop ≈ entry（1 tick 误差以内）
	if math.Abs(plan.NewStop-pos.Entry) > 1 {
		t.Errorf("NewStop=%.2f, want entry=%.2f (break-even)", plan.NewStop, pos.Entry)
	}
	// 止损未超过当前价（ExecGap 安全）
	if plan.NewStop >= price {
		t.Errorf("NewStop=%.2f >= price=%.2f, would trigger immediately", plan.NewStop, price)
	}
	// 止损高于初始止损（单调性）
	if plan.NewStop <= pos.InitStop {
		t.Errorf("NewStop=%.2f should be > InitStop=%.2f", plan.NewStop, pos.InitStop)
	}
	t.Logf("✓ LONG保本: ROI=%.2f%%, NewStop=%.2f (entry=%.2f)", plan.RoiUnr*100, plan.NewStop, pos.Entry)
}

// TestBreakeven_Short_BasicTrigger SHORT 2% ROI 触发保本（V-22.0: 保本点从3%降至2%）
func TestBreakeven_Short_BasicTrigger(t *testing.T) {
	eng := prodEng()

	// Entry=100000, Leverage=10
	// 2% ROI → LastPrice = 100000 × (1 - 0.02/10) = 99800
	// 保本 floor = Entry × (1 - 0.00/10) = 100000
	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Short,
		Entry:           100000,
		InitStop:        105000,
		PrevStop:        105000,
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        false,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}
	price := 99800.0
	plan := eng.Evaluate(pos, MarketSnapshot{
		Symbol: "BTCUSDT", LastPrice: price, MarkPrice: price, TickSize: 0.1, NowMs: 1_030_000,
	})

	if math.Abs(plan.RoiUnr-0.02) > 0.001 {
		t.Errorf("RoiUnr=%.4f, want ~0.02", plan.RoiUnr)
	}
	if !plan.NextROIArmed {
		t.Errorf("NextROIArmed=false, want true")
	}
	if !plan.ShouldUpdate {
		t.Errorf("ShouldUpdate=false, want true; reasons=%v note=%s", plan.Reasons, plan.Note)
	}
	if math.Abs(plan.Floor-pos.Entry) > 1 {
		t.Errorf("Floor=%.2f, want entry=%.2f (break-even)", plan.Floor, pos.Entry)
	}
	if math.Abs(plan.NewStop-pos.Entry) > 1 {
		t.Errorf("NewStop=%.2f, want entry=%.2f (break-even)", plan.NewStop, pos.Entry)
	}
	// SHORT 止损应高于当前价（不立即触发）
	if plan.NewStop <= price {
		t.Errorf("NewStop=%.2f <= price=%.2f, would trigger immediately", plan.NewStop, price)
	}
	// 止损低于初始止损（单调性）
	if plan.NewStop >= pos.InitStop {
		t.Errorf("NewStop=%.2f should be < InitStop=%.2f", plan.NewStop, pos.InitStop)
	}
	t.Logf("✓ SHORT保本: ROI=%.2f%%, NewStop=%.2f (entry=%.2f)", plan.RoiUnr*100, plan.NewStop, pos.Entry)
}

// ─────────────────────────────────────────────────────────────────
// 二、ROI 门槛边界
// ─────────────────────────────────────────────────────────────────

// TestBreakeven_BelowThreshold_NoTrigger ROI < 1.5% 时全标的均不触发（V-22.0）
func TestBreakeven_BelowThreshold_NoTrigger(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			pos := prodPos(sym, side, false)
			// 1.2% ROI（低于 1.5% 门槛）
			price := prodPriceAtROI(side, sym.Entry, 0.012, sym.Leverage)
			plan := eng.Evaluate(pos, prodMkSnap(sym, price, 2_000_000))

			if plan.ShouldUpdate {
				t.Errorf("%s/%s: ROI=%.3f%% < 1.5%%，不应触发; NewStop=%.8f",
					sym.Symbol, side, plan.RoiUnr*100, plan.NewStop)
			}
			if plan.NextROIArmed {
				t.Errorf("%s/%s: ROI=%.3f%% < 1.5%%，NextROIArmed 不应为 true",
					sym.Symbol, side, plan.RoiUnr*100)
			}
		}
	}
	t.Logf("✓ 全标的 ROI < 1.5%% 均未触发（%d 组）", len(tradingSymbols)*2)
}

// TestBreakeven_ExactThreshold_Triggers ROI 精确达到 2% 时全标的触发（V-22.0）
func TestBreakeven_ExactThreshold_Triggers(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			pos := prodPos(sym, side, false)
			// 2% + 0.002 缓冲确保稳定穿越浮点门槛
			price := prodPriceAtROI(side, sym.Entry, 0.02, sym.Leverage)
			plan := eng.Evaluate(pos, prodMkSnap(sym, price, 2_000_000))

			if !plan.NextROIArmed {
				t.Errorf("%s/%s: ROI=%.3f%% >= 2%%，NextROIArmed 应为 true",
					sym.Symbol, side, plan.RoiUnr*100)
			}
			if !plan.ShouldUpdate {
				t.Errorf("%s/%s: ROI=%.3f%% >= 2%%，应触发更新; reasons=%v",
					sym.Symbol, side, plan.RoiUnr*100, plan.Reasons)
			}
			// 保本 floor 应接近 entry（允许 2 tick 浮点误差）
			if math.Abs(plan.Floor-sym.Entry) > sym.TickSize*2 {
				t.Errorf("%s/%s: Floor=%.8f 应接近 entry=%.8f",
					sym.Symbol, side, plan.Floor, sym.Entry)
			}
		}
	}
	t.Logf("✓ 全标的 ROI >= 2%% 均触发保本（%d 组）", len(tradingSymbols)*2)
}

// ─────────────────────────────────────────────────────────────────
// 三、ROIArmed 状态机
// ─────────────────────────────────────────────────────────────────

// TestBreakeven_ROIArmedTransition ROIArmed=false 时，首次到达 2% ROI
// 应在同一次 Evaluate 调用中完成激活并执行保本，不需要等下一个 tick
func TestBreakeven_ROIArmedTransition(t *testing.T) {
	eng := prodEng()
	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Long,
		Entry:           100000,
		InitStop:        95000,
		PrevStop:        95000,
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        false, // 初始未激活
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}
	plan := eng.Evaluate(pos, MarketSnapshot{
		Symbol: "BTCUSDT", LastPrice: 100200, MarkPrice: 100200, TickSize: 0.1, NowMs: 1_030_000,
	})

	// 必须在单次调用中完成激活
	if !plan.NextROIArmed {
		t.Error("ROIArmed 应在首次触发时被激活")
	}
	// 必须在同一次调用中执行保本更新（不能推迟到下一个 tick）
	if !plan.ShouldUpdate {
		t.Errorf("应在 ROIArmed 激活的同一次调用中触发保本; reasons=%v note=%s",
			plan.Reasons, plan.Note)
	}
	t.Logf("✓ ROIArmed 激活与保本更新在同一次 Evaluate 中完成")
}

// ─────────────────────────────────────────────────────────────────
// 四、保本价格精确等于入场价（多标的 × 多杠杆）
// ─────────────────────────────────────────────────────────────────

// TestBreakeven_FloorEqualsEntry_AllSymbols 全标的保本 floor 精确等于入场价（V-22.0: ROI=2%）
func TestBreakeven_FloorEqualsEntry_AllSymbols(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			sym, side := sym, side
			name := fmt.Sprintf("%s/%s", sym.Symbol, side)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				pos := prodPos(sym, side, false)
				price := prodPriceAtROI(side, sym.Entry, 0.02, sym.Leverage)
				plan := eng.Evaluate(pos, prodMkSnap(sym, price, 2_000_000))

				if !plan.ShouldUpdate {
					t.Fatalf("应触发保本; reasons=%v", plan.Reasons)
				}
				// 核心：保本 floor = entry（允许 1 tick 浮点误差）
				if math.Abs(plan.Floor-sym.Entry) > sym.TickSize {
					t.Errorf("Floor=%.8f, want entry=%.8f (误差=%.2e > 1 tick=%.2e)",
						plan.Floor, sym.Entry, math.Abs(plan.Floor-sym.Entry), sym.TickSize)
				}
				t.Logf("Floor=%.8f entry=%.8f 误差=%.2e",
					plan.Floor, sym.Entry, math.Abs(plan.Floor-sym.Entry))
			})
		}
	}
}

// TestBreakeven_FloorEqualsEntry_MultiLeverage 不同杠杆下保本均精确等于入场价（V-22.0: ROI=2%）
func TestBreakeven_FloorEqualsEntry_MultiLeverage(t *testing.T) {
	eng := prodEng()
	leverages := []float64{1, 3, 5, 10, 20, 50}
	entry := 100000.0

	for _, lev := range leverages {
		lev := lev
		t.Run(fmt.Sprintf("lev%gx", lev), func(t *testing.T) {
			t.Parallel()
			pos := PositionState{
				Symbol:          "BTCUSDT",
				Side:            Long,
				Entry:           entry,
				InitStop:        entry * 0.95,
				PrevStop:        entry * 0.95,
				Leverage:        lev,
				Qty:             1.0,
				ROIArmed:        false,
				StopTriggerType: TriggerLast,
				OpenTimeMs:      1_000_000,
			}
			// 2% ROI（含缓冲），价格随杠杆变化
			price := entry * (1 + (0.02+0.002)/lev)
			plan := eng.Evaluate(pos, MarketSnapshot{
				Symbol: "BTCUSDT", LastPrice: price, MarkPrice: price, TickSize: 0.1, NowMs: 1_030_000,
			})

			if !plan.ShouldUpdate {
				t.Errorf("lev=%gx: 应触发保本; reasons=%v", lev, plan.Reasons)
				return
			}
			// 保本 floor 与杠杆无关，始终等于 entry
			if math.Abs(plan.Floor-entry) > 0.1 {
				t.Errorf("lev=%gx: Floor=%.4f, want entry=%.4f (公式与杠杆无关)", lev, plan.Floor, entry)
			}
			// 止损未超过当前价
			if plan.NewStop >= price {
				t.Errorf("lev=%gx: NewStop=%.4f >= price=%.4f, would trigger immediately",
					lev, plan.NewStop, price)
			}
			t.Logf("✓ lev=%gx: price=%.4f ROI=%.2f%%, Floor=%.4f (entry=%.4f)",
				lev, price, plan.RoiUnr*100, plan.Floor, entry)
		})
	}
}

// ─────────────────────────────────────────────────────────────────
// 五、单调性保护
// ─────────────────────────────────────────────────────────────────

// TestBreakeven_Monotonicity_LongStopAboveEntry
// 场景：LONG 止损已被推至入场价以上，价格回落到 3% ROI
// 期望：保本（floor=entry）低于 PrevStop，单调性拒绝回退
func TestBreakeven_Monotonicity_LongStopAboveEntry(t *testing.T) {
	eng := prodEng()
	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Long,
		Entry:           100000,
		InitStop:        95000,
		PrevStop:        100300, // 已在入场价以上（之前 5% 里程碑触发）
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        true,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}
	// 价格回落到 3% ROI 区间
	plan := eng.Evaluate(pos, MarketSnapshot{
		Symbol: "BTCUSDT", LastPrice: 100300, MarkPrice: 100300, TickSize: 0.1, NowMs: 1_030_000,
	})

	// 保本 floor = 100000 < PrevStop = 100300 → 单调性拒绝
	if plan.ShouldUpdate {
		t.Errorf("止损已在 %.2f > entry %.2f，保本不应回退; NewStop=%.2f floor=%.2f",
			pos.PrevStop, pos.Entry, plan.NewStop, plan.Floor)
	}
	t.Logf("✓ 单调性保护 LONG: PrevStop=%.2f > floor=%.2f → 拒绝回退",
		pos.PrevStop, plan.Floor)
}

// TestBreakeven_Monotonicity_ShortStopBelowEntry
// 场景：SHORT 止损已被推至入场价以下，价格回升到 3% ROI
// 期望：保本（floor=entry）高于 PrevStop，单调性拒绝升回
func TestBreakeven_Monotonicity_ShortStopBelowEntry(t *testing.T) {
	eng := prodEng()
	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Short,
		Entry:           100000,
		InitStop:        105000,
		PrevStop:        99700, // 已在入场价以下（之前 5% 里程碑触发）
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        true,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}
	// 价格回升到 3% ROI 区间
	plan := eng.Evaluate(pos, MarketSnapshot{
		Symbol: "BTCUSDT", LastPrice: 99700, MarkPrice: 99700, TickSize: 0.1, NowMs: 1_030_000,
	})

	// 保本 floor = 100000 > PrevStop = 99700 → SHORT 单调性拒绝
	if plan.ShouldUpdate {
		t.Errorf("SHORT 止损已在 %.2f < entry %.2f，保本不应升回; NewStop=%.2f floor=%.2f",
			pos.PrevStop, pos.Entry, plan.NewStop, plan.Floor)
	}
	t.Logf("✓ 单调性保护 SHORT: PrevStop=%.2f < floor=%.2f → 拒绝升回",
		pos.PrevStop, plan.Floor)
}

// ─────────────────────────────────────────────────────────────────
// 六、ExecGap 安全门
// 场景：价格恰好等于入场价（ROI=0）时，即使 ROIArmed=true
// 保本 floor=entry，ExecGap 应拒绝（floor >= ref for LONG）
// ─────────────────────────────────────────────────────────────────

func TestBreakeven_ExecGap_PriceAtEntry(t *testing.T) {
	eng := prodEng()
	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Long,
		Entry:           100000,
		InitStop:        95000,
		PrevStop:        95000,
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        true, // 已激活，但价格在入场价
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}
	// 价格 = entry，ROI = 0，floor = entry（保本止损 = 当前价 → 立即触发）
	plan := eng.Evaluate(pos, MarketSnapshot{
		Symbol: "BTCUSDT", LastPrice: 100000, MarkPrice: 100000, TickSize: 0.1, NowMs: 1_030_000,
	})

	// ROI < 3%（触发门槛），应直接在 ROI 检查阶段被拦截
	if plan.ShouldUpdate {
		t.Errorf("价格=entry 时 ROI=0%% < 3%%，不应触发; NewStop=%.2f", plan.NewStop)
	}
	t.Logf("✓ ExecGap/ROI门槛: price=entry, ROI=%.2f%% → 拒绝", plan.RoiUnr*100)
}

// ─────────────────────────────────────────────────────────────────
// 七、梯度递进（全标的 × 两方向，含保本节点）
// ─────────────────────────────────────────────────────────────────

// TestBreakeven_FullGradient 含保本的完整 13 级里程碑递进测试
func TestBreakeven_FullGradient(t *testing.T) {
	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			sym, side := sym, side
			name := fmt.Sprintf("%s/%s", sym.Symbol, side)

			t.Run(name, func(t *testing.T) {
				t.Parallel()
				eng := prodEng()
				pos := prodPos(sym, side, false)
				prevStop := pos.PrevStop
				nowMs := int64(2_000_000)

				for i, ms := range breakevenMilestones {
					nowMs += 30_000
					price := prodPriceAtROI(side, sym.Entry, ms.ROI, sym.Leverage)
					plan := eng.Evaluate(pos, prodMkSnap(sym, price, nowMs))

					if !plan.ShouldUpdate {
						t.Errorf("[M%d ROI=%.0f%%] 应触发: %v %s",
							i+1, ms.ROI*100, plan.Reasons, plan.Note)
						pos.ROIArmed = plan.NextROIArmed
						continue
					}

					// 单调性
					if side == Long && plan.NewStop <= prevStop {
						t.Errorf("[M%d] LONG 止损未上移: %.8f <= %.8f", i+1, plan.NewStop, prevStop)
					}
					if side == Short && plan.NewStop >= prevStop {
						t.Errorf("[M%d] SHORT 止损未下移: %.8f >= %.8f", i+1, plan.NewStop, prevStop)
					}

					// 不超当前价（ExecGap 安全）
					if side == Long && plan.NewStop >= price {
						t.Errorf("[M%d] LONG NewStop=%.8f >= price=%.8f（会立即触发）",
							i+1, plan.NewStop, price)
					}
					if side == Short && plan.NewStop <= price {
						t.Errorf("[M%d] SHORT NewStop=%.8f <= price=%.8f（会立即触发）",
							i+1, plan.NewStop, price)
					}

					// 地板公式正确（1 tick 误差以内）
					wantFloor := prodExpectedFloor(side, sym.Entry, ms.LockPct, sym.Leverage)
					if math.Abs(plan.Floor-wantFloor) > sym.TickSize*2 {
						t.Errorf("[M%d] Floor=%.8f, want=%.8f (lockPct=%.2f%%)",
							i+1, plan.Floor, wantFloor, ms.LockPct*100)
					}

					locked := prodROIAtStop(side, sym.Entry, plan.NewStop, sym.Leverage)
					t.Logf("M%2d ROI=%5.0f%% → stop %.8f → %.8f 锁%.2f%% (期望=%.2f%%)",
						i+1, ms.ROI*100, prevStop, plan.NewStop, locked*100, ms.LockPct*100)

					prevStop = plan.NewStop
					pos.PrevStop = plan.NewStop
					pos.LastStopUpdateTimeMs = nowMs
					pos.ROIArmed = plan.NextROIArmed
				}
			})
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// 八、完整模拟（含保本的逐步价格序列）
// ─────────────────────────────────────────────────────────────────

// TestBreakeven_Simulation_Long LONG 完整模拟，验证每一步的预期行为
func TestBreakeven_Simulation_Long(t *testing.T) {
	// 关闭冷却，聚焦逻辑正确性
	cfg := DefaultConfig()
	cfg.UpdateCooldownSec = 0
	eng := &Engine{Cfg: cfg}

	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Long,
		Entry:           100000,
		InitStop:        95000,
		PrevStop:        95000,
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        false,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}

	type step struct {
		price      float64
		roi        float64
		wantUpdate bool
		wantStop   float64 // 0 = 不检查具体值
		desc       string
	}
	steps := []step{
		{100100, 0.01, false, 0, "ROI 1%，未达 1.5% 门槛"},
		{100200, 0.02, true, 100000, "ROI 2%，触发保本（止损=入场价）"},
		{100250, 0.025, false, 0, "ROI 2.5%，单调性拦截（floor=entry=PrevStop）"},
		{100300, 0.03, true, 100070, "ROI 3%，锁住0.7%（止损=entry+0.07%@10x）"},
		{100500, 0.05, true, 100350, "ROI 5%，锁住3.5%"},
		{100800, 0.08, true, 100608, "ROI 8%，锁住6.08%"},
		{102000, 0.20, true, 101720, "ROI 20%，锁住17.2%"},
		{103000, 0.30, true, 102620, "ROI 30%，锁住26.2%"},
	}

	nowMs := int64(1_030_000)
	for _, s := range steps {
		plan := eng.Evaluate(pos, MarketSnapshot{
			Symbol: "BTCUSDT", LastPrice: s.price, MarkPrice: s.price, TickSize: 0.1, NowMs: nowMs,
		})
		nowMs += 30_000

		if math.Abs(plan.RoiUnr-s.roi) > 0.001 {
			t.Errorf("[%s] RoiUnr=%.4f, want=%.4f", s.desc, plan.RoiUnr, s.roi)
		}
		if plan.ShouldUpdate != s.wantUpdate {
			t.Errorf("[%s] ShouldUpdate=%v, want=%v; reasons=%v note=%s",
				s.desc, plan.ShouldUpdate, s.wantUpdate, plan.Reasons, plan.Note)
		}
		if s.wantStop > 0 && plan.ShouldUpdate {
			if math.Abs(plan.NewStop-s.wantStop) > 1 {
				t.Errorf("[%s] NewStop=%.2f, want=%.2f", s.desc, plan.NewStop, s.wantStop)
			}
			// 止损必须在当前价下方（不立即触发）
			if plan.NewStop >= s.price {
				t.Errorf("[%s] NewStop=%.2f >= price=%.2f", s.desc, plan.NewStop, s.price)
			}
			pos.PrevStop = plan.NewStop
			pos.LastStopUpdateTimeMs = nowMs
		}
		pos.ROIArmed = plan.NextROIArmed
		t.Logf("[%s] ROI=%.1f%%, Update=%v, Stop=%.2f",
			s.desc, plan.RoiUnr*100, plan.ShouldUpdate, pos.PrevStop)
	}

	// 最终止损应在入场价以上（资金安全）
	if pos.PrevStop <= pos.Entry {
		t.Errorf("最终止损 %.2f 应在 entry %.2f 以上", pos.PrevStop, pos.Entry)
	}
}

// TestBreakeven_Simulation_Short SHORT 完整模拟（与 Long 对称）
func TestBreakeven_Simulation_Short(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UpdateCooldownSec = 0
	eng := &Engine{Cfg: cfg}

	pos := PositionState{
		Symbol:          "BTCUSDT",
		Side:            Short,
		Entry:           100000,
		InitStop:        105000,
		PrevStop:        105000,
		Leverage:        10,
		Qty:             1.0,
		ROIArmed:        false,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}

	type step struct {
		price      float64
		roi        float64
		wantUpdate bool
		wantStop   float64
		desc       string
	}
	steps := []step{
		{99900, 0.01, false, 0, "ROI 1%，未达 1.5% 门槛"},
		{99800, 0.02, true, 100000, "ROI 2%，触发保本（止损=入场价）"},
		{99750, 0.025, false, 0, "ROI 2.5%，单调性拦截（floor=entry=PrevStop）"},
		{99700, 0.03, true, 99930, "ROI 3%，锁住0.7%（止损=entry-0.07%@10x）"},
		{99500, 0.05, true, 99650, "ROI 5%，锁住3.5%"},
		{99200, 0.08, true, 99392, "ROI 8%，锁住6.08%"},
		{98000, 0.20, true, 98280, "ROI 20%，锁住17.2%"},
		{97000, 0.30, true, 97380, "ROI 30%，锁住26.2%"},
	}

	nowMs := int64(1_030_000)
	for _, s := range steps {
		plan := eng.Evaluate(pos, MarketSnapshot{
			Symbol: "BTCUSDT", LastPrice: s.price, MarkPrice: s.price, TickSize: 0.1, NowMs: nowMs,
		})
		nowMs += 30_000

		if math.Abs(plan.RoiUnr-s.roi) > 0.001 {
			t.Errorf("[%s] RoiUnr=%.4f, want=%.4f", s.desc, plan.RoiUnr, s.roi)
		}
		if plan.ShouldUpdate != s.wantUpdate {
			t.Errorf("[%s] ShouldUpdate=%v, want=%v; reasons=%v note=%s",
				s.desc, plan.ShouldUpdate, s.wantUpdate, plan.Reasons, plan.Note)
		}
		if s.wantStop > 0 && plan.ShouldUpdate {
			if math.Abs(plan.NewStop-s.wantStop) > 1 {
				t.Errorf("[%s] NewStop=%.2f, want=%.2f", s.desc, plan.NewStop, s.wantStop)
			}
			// SHORT 止损必须在当前价上方（不立即触发）
			if plan.NewStop <= s.price {
				t.Errorf("[%s] NewStop=%.2f <= price=%.2f", s.desc, plan.NewStop, s.price)
			}
			pos.PrevStop = plan.NewStop
			pos.LastStopUpdateTimeMs = nowMs
		}
		pos.ROIArmed = plan.NextROIArmed
		t.Logf("[%s] ROI=%.1f%%, Update=%v, Stop=%.2f",
			s.desc, plan.RoiUnr*100, plan.ShouldUpdate, pos.PrevStop)
	}

	// 最终止损应在入场价以下（资金安全）
	if pos.PrevStop >= pos.Entry {
		t.Errorf("最终止损 %.2f 应在 entry %.2f 以下", pos.PrevStop, pos.Entry)
	}
}

// ─────────────────────────────────────────────────────────────────
// 九、保本止损触发时 ROI = 0（资金不亏损保证）
// ─────────────────────────────────────────────────────────────────

// TestBreakeven_ProfitZeroAtStop 验证保本止损触发时 ROI 精确为 0
// 核心生产保证：止损挂在入场价，触发时不亏不赚（V-22.0: 保本点=2% ROI，floor=0.000）
func TestBreakeven_ProfitZeroAtStop(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			sym, side := sym, side
			name := fmt.Sprintf("%s/%s", sym.Symbol, side)

			t.Run(name, func(t *testing.T) {
				t.Parallel()
				pos := prodPos(sym, side, false)
				price := prodPriceAtROI(side, sym.Entry, 0.02, sym.Leverage)
				plan := eng.Evaluate(pos, prodMkSnap(sym, price, 2_000_000))

				if !plan.ShouldUpdate {
					t.Fatalf("应触发保本; reasons=%v", plan.Reasons)
				}

				// 核心断言：止损触发时 ROI = 0（保本，不亏损）
				roiAtStop := prodROIAtStop(side, sym.Entry, plan.NewStop, sym.Leverage)
				if math.Abs(roiAtStop) > 0.001 {
					t.Errorf("保本止损 ROI=%.4f%%，want 0%% (stop=%.8f entry=%.8f)",
						roiAtStop*100, plan.NewStop, sym.Entry)
				}
				// 确保止损不低于入场价方向（对 LONG 而言不能低于 entry）
				if side == Long && plan.NewStop < sym.Entry-sym.TickSize {
					t.Errorf("LONG NewStop=%.8f 低于 entry=%.8f，会产生亏损",
						plan.NewStop, sym.Entry)
				}
				if side == Short && plan.NewStop > sym.Entry+sym.TickSize {
					t.Errorf("SHORT NewStop=%.8f 高于 entry=%.8f，会产生亏损",
						plan.NewStop, sym.Entry)
				}
				t.Logf("✓ 止损=%.8f entry=%.8f ROI@stop=%.4f%%",
					plan.NewStop, sym.Entry, roiAtStop*100)
			})
		}
	}
}
