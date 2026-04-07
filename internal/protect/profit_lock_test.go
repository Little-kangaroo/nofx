package protect

import (
	"math"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────────────────────────

func newLong(entry, initStop, prevStop, leverage float64) PositionState {
	return PositionState{
		Symbol:          "BTCUSDT",
		Side:            Long,
		Entry:           entry,
		InitStop:        initStop,
		PrevStop:        prevStop,
		Leverage:        leverage,
		Qty:             1.0,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}
}

func newShort(entry, initStop, prevStop, leverage float64) PositionState {
	return PositionState{
		Symbol:          "BTCUSDT",
		Side:            Short,
		Entry:           entry,
		InitStop:        initStop,
		PrevStop:        prevStop,
		Leverage:        leverage,
		Qty:             1.0,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}
}

func snap(lastPrice, tickSize float64, nowMs int64) MarketSnapshot {
	return MarketSnapshot{
		Symbol:    "BTCUSDT",
		LastPrice: lastPrice,
		MarkPrice: lastPrice,
		TickSize:  tickSize,
		NowMs:     nowMs,
	}
}

// roiAtStop 计算止损触发时的ROI
func roiAtStop(side Side, entry, stopPrice, leverage float64) float64 {
	if side == Long {
		return (stopPrice - entry) / entry * leverage
	}
	return (entry - stopPrice) / entry * leverage
}

// defaultEng 默认配置引擎（无冷却，便于测试）
func defaultEng() *Engine {
	cfg := DefaultConfig()
	cfg.UpdateCooldownSec = 0
	return &Engine{Cfg: cfg}
}

// ─────────────────────────────────────────────────────────────────────────────
// 一、盈利地板公式正确性
// ─────────────────────────────────────────────────────────────────────────────

// TestProfitFloor_Semantics 验证 floorPct 表示"锁住 X% ROI"
// floor = Entry × (1 + floorPct/leverage)，ROI at floor == floorPct
func TestProfitFloor_Semantics(t *testing.T) {
	cfg := DefaultConfig()
	entry := 100000.0
	leverage := 10.0

	cases := []struct {
		currentROI float64
		floorPct   float64 // 期望锁住的ROI（V-24.0）
	}{
		{0.05, 0.0350},
		{0.08, 0.0608},
		{0.12, 0.0974},
		{0.16, 0.1338},
		{0.20, 0.1720},
		{0.25, 0.2167},
		{0.30, 0.2620},
		{0.40, 0.3547},
		{0.50, 0.4500},
		{0.60, 0.5424},
		{0.80, 0.7296},
		{1.00, 0.9200},
	}

	for _, c := range cases {
		pos := newLong(entry, entry*0.95, entry*0.95, leverage) // initStop任意

		floor := ProfitFloor(pos, cfg, c.currentROI)
		roiLocked := (floor - entry) / entry * leverage

		// 1. floor > entry（锁住的是盈利，不是亏损）
		if floor <= entry {
			t.Errorf("ROI=%.0f%%: floor(%.2f) <= entry(%.2f)，未锁盈", c.currentROI*100, floor, entry)
		}

		// 2. ROI at floor == floorPct（语义正确）
		if math.Abs(roiLocked-c.floorPct) > 0.0001 {
			t.Errorf("ROI=%.0f%%: 锁住ROI=%.4f，期望%.4f", c.currentROI*100, roiLocked, c.floorPct)
		}

		// 3. floor < 当前价（止损不会立即触发）
		currentPrice := entry * (1 + c.currentROI/leverage)
		if floor >= currentPrice {
			t.Errorf("ROI=%.0f%%: floor(%.2f) >= currentPrice(%.2f)，止损会立即触发", c.currentROI*100, floor, currentPrice)
		}

		t.Logf("ROI=%5.0f%% → 锁住%5.1f%% ROI | floor=%.2f | 当前价=%.2f | 回撤空间=%.1f%% ROI",
			c.currentROI*100, roiLocked*100, floor, currentPrice, (c.currentROI-c.floorPct)*100)
	}
}

// TestProfitFloor_Short 验证 SHORT 方向地板公式
func TestProfitFloor_Short(t *testing.T) {
	cfg := DefaultConfig()
	entry := 100000.0
	leverage := 10.0

	cases := []struct {
		currentROI float64
		floorPct   float64
	}{
		{0.05, 0.0350},
		{0.20, 0.1720},
		{0.50, 0.4500},
		{1.00, 0.9200},
	}

	for _, c := range cases {
		pos := newShort(entry, entry*1.05, entry*1.05, leverage)

		floor := ProfitFloor(pos, cfg, c.currentROI)
		roiLocked := (entry - floor) / entry * leverage

		// floor < entry（SHORT止损往下移）
		if floor >= entry {
			t.Errorf("SHORT ROI=%.0f%%: floor(%.2f) >= entry(%.2f)", c.currentROI*100, floor, entry)
		}

		// ROI at floor == floorPct
		if math.Abs(roiLocked-c.floorPct) > 0.0001 {
			t.Errorf("SHORT ROI=%.0f%%: 锁住ROI=%.4f，期望%.4f", c.currentROI*100, roiLocked, c.floorPct)
		}

		// floor > 当前价（止损不会立即触发）
		currentPrice := entry * (1 - c.currentROI/leverage)
		if floor <= currentPrice {
			t.Errorf("SHORT ROI=%.0f%%: floor(%.2f) <= currentPrice(%.2f)", c.currentROI*100, floor, currentPrice)
		}

		t.Logf("SHORT ROI=%5.0f%% → 锁住%5.1f%% ROI | floor=%.2f | 当前价=%.2f",
			c.currentROI*100, roiLocked*100, floor, currentPrice)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 二、ROI 触发门槛
// ─────────────────────────────────────────────────────────────────────────────

func TestROITrigger_BelowThreshold(t *testing.T) {
	eng := defaultEng()
	pos := newLong(100000, 95000, 95000, 10)
	pos.ROIArmed = false

	// ROI = 0.3%，低于 0.5% 门槛（V-24.0）
	// price = entry × (1 + 0.003/10) = 100030
	m := snap(100030, 0.1, 2_000_000)
	plan := eng.Evaluate(pos, m)

	if plan.ShouldUpdate {
		t.Errorf("ROI=0.3%%：不应触发更新")
	}
	if plan.NextROIArmed {
		t.Errorf("ROI=0.3%%：ROIArmed 不应为 true")
	}
}

func TestROITrigger_ExactThreshold(t *testing.T) {
	eng := defaultEng()
	pos := newLong(100000, 95000, 95000, 10)
	pos.ROIArmed = false

	// ROI = 5.0%，恰好等于门槛
	// price = entry × (1 + 0.05/10) = 100500
	m := snap(100500, 0.1, 2_000_000)
	plan := eng.Evaluate(pos, m)

	if !plan.ShouldUpdate {
		t.Errorf("ROI=5%%：应触发更新，原因: %v, 备注: %s", plan.Reasons, plan.Note)
	}
	if !plan.NextROIArmed {
		t.Errorf("ROI=5%%：NextROIArmed 应为 true")
	}
	// 止损应在 entry 上方（锁住盈利）
	if plan.NewStop <= pos.Entry {
		t.Errorf("新止损(%.2f) 应 > entry(%.2f)", plan.NewStop, pos.Entry)
	}
	t.Logf("ROI=5%%：止损 %.2f → %.2f，锁住ROI=%.2f%%",
		pos.PrevStop, plan.NewStop, roiAtStop(Long, pos.Entry, plan.NewStop, pos.Leverage)*100)
}

func TestROITrigger_AlreadyArmed(t *testing.T) {
	eng := defaultEng()
	pos := newLong(100000, 95000, 95000, 10)
	pos.ROIArmed = true // 已触发过

	// ROI 回落到 3%（低于门槛），但已 armed，仍应继续评估地板
	// floor at 5% milestone = 100000×1.003 = 100300，prevStop=95000 < 100300 → 更新
	m := snap(100300, 0.1, 2_000_000)
	plan := eng.Evaluate(pos, m)

	// ROI=3%，未命中任何里程碑，floor=entry（floorPct=0）
	// floor = 100000 × (1 + 0/10) = 100000，等于 entry
	// 实际上 floorPct=0，floor=entry，candidate = max(95000, 100000) = 100000
	// FloorToTick(100000, 0.1) = 100000，比 prevStop(95000) 好 → 应该更新
	if !plan.NextROIArmed {
		t.Errorf("ROIArmed 状态应保持 true")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 三、LONG 完整里程碑递进模拟
// ─────────────────────────────────────────────────────────────────────────────

// TestLong_FullMilestoneProgression 模拟价格从入场价上涨穿越各里程碑
// Entry=100000，InitStop=95000，Leverage=10
func TestLong_FullMilestoneProgression(t *testing.T) {
	eng := defaultEng()

	pos := newLong(100000, 95000, 95000, 10)
	pos.ROIArmed = false

	type step struct {
		desc         string
		lastPrice    float64
		expectedROI  float64
		expectedLock float64 // 期望锁住的ROI%
		shouldUpdate bool
	}

	steps := []step{
		{"ROI=0.3%（未达门槛）", 100030, 0.003, 0, false},
		{"ROI=5%（触发+锁3.5%）", 100500, 0.05, 0.0350, true},
		{"ROI=8%（锁6.08%）", 100800, 0.08, 0.0608, true},
		{"ROI=12%（锁9.74%）", 101200, 0.12, 0.0974, true},
		{"ROI=16%（锁13.38%）", 101600, 0.16, 0.1338, true},
		{"ROI=20%（锁17.2%）", 102000, 0.20, 0.1720, true},
		{"ROI=25%（锁21.67%）", 102500, 0.25, 0.2167, true},
		{"ROI=30%（锁26.2%）", 103000, 0.30, 0.2620, true},
	}

	prevStop := pos.PrevStop
	nowMs := int64(2_000_000)

	for _, s := range steps {
		nowMs += 30_000 // 每步+30秒
		m := snap(s.lastPrice, 0.1, nowMs)
		plan := eng.Evaluate(pos, m)

		roi := plan.RoiUnr
		if math.Abs(roi-s.expectedROI) > 0.001 {
			t.Errorf("[%s] ROI=%.3f，期望%.3f", s.desc, roi, s.expectedROI)
		}

		if plan.ShouldUpdate != s.shouldUpdate {
			t.Errorf("[%s] ShouldUpdate=%v，期望%v | 原因: %v | 备注: %s",
				s.desc, plan.ShouldUpdate, s.shouldUpdate, plan.Reasons, plan.Note)
		}

		if plan.ShouldUpdate {
			// 止损必须上移
			if plan.NewStop <= prevStop {
				t.Errorf("[%s] 新止损(%.2f) 未上移，旧止损(%.2f)", s.desc, plan.NewStop, prevStop)
			}
			// 止损必须在 entry 上方
			if plan.NewStop <= pos.Entry {
				t.Errorf("[%s] 新止损(%.2f) 应 > entry(%.2f)，未锁盈", s.desc, plan.NewStop, pos.Entry)
			}
			// 止损必须低于当前价
			if plan.NewStop >= s.lastPrice {
				t.Errorf("[%s] 新止损(%.2f) >= 当前价(%.2f)，会立即触发", s.desc, plan.NewStop, s.lastPrice)
			}
			// 验证锁住的ROI
			lockedROI := roiAtStop(Long, pos.Entry, plan.NewStop, pos.Leverage)
			if math.Abs(lockedROI-s.expectedLock) > 0.002 {
				t.Errorf("[%s] 锁住ROI=%.3f，期望%.3f", s.desc, lockedROI, s.expectedLock)
			}

			t.Logf("✅ [%s] 止损 %.2f → %.2f | 锁住ROI=%.1f%% | 当前ROI=%.1f%%",
				s.desc, prevStop, plan.NewStop, lockedROI*100, roi*100)

			prevStop = plan.NewStop
			pos.PrevStop = plan.NewStop
			pos.LastStopUpdateTimeMs = nowMs
		} else {
			t.Logf("⏭  [%s] 不更新 | ROI=%.1f%% | 原因: %v",
				s.desc, roi*100, plan.Reasons)
		}

		pos.ROIArmed = plan.NextROIArmed
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 四、价格回撤后止损不后退
// ─────────────────────────────────────────────────────────────────────────────

func TestLong_StopHoldsOnPullback(t *testing.T) {
	eng := defaultEng()

	pos := newLong(100000, 95000, 95000, 10)
	pos.ROIArmed = false
	nowMs := int64(2_000_000)

	// 先把价格推到 30% ROI，触发止损更新
	m := snap(103000, 0.1, nowMs)
	plan := eng.Evaluate(pos, m)
	if !plan.ShouldUpdate {
		t.Fatalf("setup: 应更新止损")
	}
	pos.PrevStop = plan.NewStop
	pos.ROIArmed = plan.NextROIArmed
	stopAfterUp := plan.NewStop
	lockedROI := roiAtStop(Long, pos.Entry, stopAfterUp, pos.Leverage)
	t.Logf("上涨后止损: %.2f（锁住ROI=%.1f%%）", stopAfterUp, lockedROI*100)

	// 价格回落到 10% ROI
	nowMs += 30_000
	m2 := snap(101000, 0.1, nowMs)
	plan2 := eng.Evaluate(pos, m2)

	if plan2.ShouldUpdate {
		t.Errorf("价格回落时止损不应后退，但建议更新到 %.2f", plan2.NewStop)
	}
	if pos.PrevStop != stopAfterUp {
		t.Errorf("止损应保持 %.2f，实际 %.2f", stopAfterUp, pos.PrevStop)
	}
	t.Logf("✅ 价格回落到 ROI=10%%，止损保持在 %.2f（锁住ROI=%.1f%%）", pos.PrevStop, lockedROI*100)
}

// ─────────────────────────────────────────────────────────────────────────────
// 五、SHORT 完整里程碑递进模拟
// ─────────────────────────────────────────────────────────────────────────────

func TestShort_FullMilestoneProgression(t *testing.T) {
	eng := defaultEng()

	// SHORT：Entry=100000，InitStop=105000（止损在上方）
	pos := newShort(100000, 105000, 105000, 10)
	pos.ROIArmed = false

	type step struct {
		desc         string
		lastPrice    float64
		expectedROI  float64
		expectedLock float64
		shouldUpdate bool
	}

	steps := []step{
		{"ROI=0.3%（未达门槛）", 99970, 0.003, 0, false},
		{"ROI=5%（触发+锁3.5%）", 99500, 0.05, 0.0350, true},
		{"ROI=12%（锁9.74%）", 98800, 0.12, 0.0974, true},
		{"ROI=20%（锁17.2%）", 98000, 0.20, 0.1720, true},
		{"ROI=30%（锁26.2%）", 97000, 0.30, 0.2620, true},
		{"ROI=50%（锁45%）", 95000, 0.50, 0.4500, true},
	}

	prevStop := pos.PrevStop
	nowMs := int64(2_000_000)

	for _, s := range steps {
		nowMs += 30_000
		m := snap(s.lastPrice, 0.1, nowMs)
		plan := eng.Evaluate(pos, m)

		if plan.ShouldUpdate != s.shouldUpdate {
			t.Errorf("[%s] ShouldUpdate=%v，期望%v | 原因: %v | 备注: %s",
				s.desc, plan.ShouldUpdate, s.shouldUpdate, plan.Reasons, plan.Note)
		}

		if plan.ShouldUpdate {
			// SHORT止损必须下移
			if plan.NewStop >= prevStop {
				t.Errorf("[%s] SHORT 新止损(%.2f) 未下移，旧止损(%.2f)", s.desc, plan.NewStop, prevStop)
			}
			// 止损必须在 entry 下方
			if plan.NewStop >= pos.Entry {
				t.Errorf("[%s] SHORT 新止损(%.2f) 应 < entry(%.2f)", s.desc, plan.NewStop, pos.Entry)
			}
			// 止损必须高于当前价
			if plan.NewStop <= s.lastPrice {
				t.Errorf("[%s] SHORT 新止损(%.2f) <= 当前价(%.2f)，会立即触发", s.desc, plan.NewStop, s.lastPrice)
			}
			// 验证锁住的ROI
			lockedROI := roiAtStop(Short, pos.Entry, plan.NewStop, pos.Leverage)
			if math.Abs(lockedROI-s.expectedLock) > 0.002 {
				t.Errorf("[%s] 锁住ROI=%.3f，期望%.3f", s.desc, lockedROI, s.expectedLock)
			}

			t.Logf("✅ [%s] 止损 %.2f → %.2f | 锁住ROI=%.1f%% | 当前ROI=%.1f%%",
				s.desc, prevStop, plan.NewStop, lockedROI*100, plan.RoiUnr*100)

			prevStop = plan.NewStop
			pos.PrevStop = plan.NewStop
			pos.LastStopUpdateTimeMs = nowMs
		} else {
			t.Logf("⏭  [%s] 不更新 | ROI=%.1f%% | 原因: %v", s.desc, plan.RoiUnr*100, plan.Reasons)
		}

		pos.ROIArmed = plan.NextROIArmed
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 六、安全检查：止损不能超过当前价
// ─────────────────────────────────────────────────────────────────────────────

func TestExecGap_FloorAboveCurrentPrice(t *testing.T) {
	// 构造极端情况：floorPct 极高，导致 floor > current price
	// 这种情况正常不会发生（floorPct < trigger），但测试安全阀
	cfg := DefaultConfig()
	cfg.UpdateCooldownSec = 0
	// 强行注入一个 floor > current price 的里程碑
	cfg.StopLossMilestones = map[float64]float64{
		0.05: 0.90, // 5% ROI 时锁住 90%，floor 远超当前价
	}
	eng := &Engine{Cfg: cfg}

	pos := newLong(100000, 95000, 95000, 10)
	pos.ROIArmed = false

	// ROI=5%，当前价=100500，floor = 100000*(1+0.09) = 109000 > 100500
	m := snap(100500, 0.1, 2_000_000)
	plan := eng.Evaluate(pos, m)

	if plan.ShouldUpdate {
		t.Errorf("floor > 当前价，不应更新止损（会立即触发），NewStop=%.2f, ref=%.2f",
			plan.NewStop, plan.RefPrice)
	}
	if !plan.ExecGap {
		t.Errorf("应设置 ExecGap=true")
	}
	t.Logf("✅ 安全阀生效：floor(%.2f) >= ref(%.2f)，拒绝更新", plan.Floor, plan.RefPrice)
}

// ─────────────────────────────────────────────────────────────────────────────
// 七、冷却期
// ─────────────────────────────────────────────────────────────────────────────

func TestCooldown_BlocksUpdate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UpdateCooldownSec = 25
	eng := &Engine{Cfg: cfg}

	pos := newLong(100000, 95000, 95000, 10)
	pos.ROIArmed = false
	pos.LastStopUpdateTimeMs = 2_000_000 // 上次更新时间

	// 5秒后再次评估（冷却期25秒未到）
	m := snap(100500, 0.1, 2_005_000)
	plan := eng.Evaluate(pos, m)

	if plan.ShouldUpdate {
		t.Errorf("冷却期内不应更新止损")
	}
	found := false
	for _, r := range plan.Reasons {
		if r == ReasonCooldown {
			found = true
		}
	}
	if !found {
		t.Errorf("原因应包含 COOLDOWN，实际: %v", plan.Reasons)
	}
	t.Logf("✅ 冷却期拦截成功：距上次更新5s < 25s")
}

func TestCooldown_AllowsAfterExpiry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UpdateCooldownSec = 25
	eng := &Engine{Cfg: cfg}

	pos := newLong(100000, 95000, 95000, 10)
	pos.ROIArmed = false
	pos.LastStopUpdateTimeMs = 2_000_000

	// 30秒后评估（冷却期已过）
	m := snap(100500, 0.1, 2_030_000)
	plan := eng.Evaluate(pos, m)

	if !plan.ShouldUpdate {
		t.Errorf("冷却期过后应允许更新，原因: %v", plan.Reasons)
	}
	t.Logf("✅ 冷却期过后允许更新：30s > 25s")
}

// ─────────────────────────────────────────────────────────────────────────────
// 八、Tick 对齐（格式化，不影响触发判断）
// ─────────────────────────────────────────────────────────────────────────────

func TestTickAlignment(t *testing.T) {
	eng := defaultEng()

	pos := newLong(100000, 95000, 95000, 10)
	pos.ROIArmed = false

	// 5% ROI → floor = 100300.0（整数，无需对齐）
	// 使用 tick=0.5，测试对齐
	m := snap(100500, 0.5, 2_000_000)
	plan := eng.Evaluate(pos, m)

	if !plan.ShouldUpdate {
		t.Fatalf("应更新止损")
	}
	// 验证 NewStop 是 0.5 的整数倍
	remainder := math.Mod(plan.NewStop, 0.5)
	if math.Abs(remainder) > 1e-9 && math.Abs(remainder-0.5) > 1e-9 {
		t.Errorf("NewStop(%.4f) 未对齐 tick(0.5)，余数=%.6f", plan.NewStop, remainder)
	}
	// 验证对齐后仍 > prevStop（止损有效上移）
	if plan.NewStop <= pos.PrevStop {
		t.Errorf("tick对齐后止损(%.4f) 应 > prevStop(%.2f)", plan.NewStop, pos.PrevStop)
	}
	t.Logf("✅ tick=0.5 对齐：floor=%.2f → NewStop=%.4f", plan.Floor, plan.NewStop)
}

// ─────────────────────────────────────────────────────────────────────────────
// 九、真实场景：DOGEUSDT（复现用户日志）
// ─────────────────────────────────────────────────────────────────────────────

func TestRealScenario_DOGEUSDT_22pct(t *testing.T) {
	eng := defaultEng()

	// 来自真实日志：Entry=0.091800，当前价=0.093900，ROI=22.88%
	// InitStop=0.090500，当前止损=0.091980（上次在20%里程碑已更新）
	pos := PositionState{
		Symbol:          "DOGEUSDT",
		Side:            Long,
		Entry:           0.091800,
		InitStop:        0.090500,
		PrevStop:        0.091980, // 已在20%里程碑更新过
		Leverage:        10,
		Qty:             1000,
		ROIArmed:        true,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}

	m := MarketSnapshot{
		Symbol:    "DOGEUSDT",
		LastPrice: 0.093900,
		MarkPrice: 0.093900,
		TickSize:  0.00001,
		NowMs:     2_000_000,
	}

	plan := eng.Evaluate(pos, m)

	// 22.88% ROI，20%里程碑 → floor = 0.091800 × 1.014 = 0.093085
	// FloorToTick(0.093085, 0.00001) = 0.093080（floor向下取整）
	// 但 prevStop = 0.091980 < floor 0.093080 → 应更新
	t.Logf("Floor=%.6f, PrevStop=%.6f, NewStop=%.6f, ShouldUpdate=%v",
		plan.Floor, pos.PrevStop, plan.NewStop, plan.ShouldUpdate)

	if !plan.ShouldUpdate {
		t.Errorf("ROI=22.88%%，floor(%.6f) > prevStop(%.6f)，应更新止损，原因: %v, 备注: %s",
			plan.Floor, pos.PrevStop, plan.Reasons, plan.Note)
	}
	if plan.NewStop <= pos.PrevStop {
		t.Errorf("新止损(%.6f) 应 > 旧止损(%.6f)", plan.NewStop, pos.PrevStop)
	}
	if plan.NewStop >= m.LastPrice {
		t.Errorf("新止损(%.6f) 应 < 当前价(%.6f)", plan.NewStop, m.LastPrice)
	}

	lockedROI := roiAtStop(Long, pos.Entry, plan.NewStop, pos.Leverage)
	t.Logf("✅ DOGEUSDT: 止损 %.6f → %.6f | 锁住ROI=%.2f%% | 当前ROI=%.2f%%",
		pos.PrevStop, plan.NewStop, lockedROI*100, plan.RoiUnr*100)
}

func TestRealScenario_DOGEUSDT_FullHistory(t *testing.T) {
	// 模拟完整历史：从 0 开始，价格逐步上涨
	eng := defaultEng()

	pos := PositionState{
		Symbol:          "DOGEUSDT",
		Side:            Long,
		Entry:           0.091800,
		InitStop:        0.090500,
		PrevStop:        0.090500,
		Leverage:        10,
		Qty:             1000,
		ROIArmed:        false,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}

	priceSteps := []struct {
		price        float64
		shouldUpdate bool
		minLockedROI float64
	}{
		{0.091800 * 1.0003, false, 0},      // 0.3% ROI → 未触发（低于0.5%门槛）
		{0.091800 * 1.0052, true, 0.034},  // ~5.2% ROI → 命中5%里程碑，锁3.5%
		{0.091800 * 1.0082, true, 0.059},  // ~8.2% ROI → 命中8%里程碑，锁6.08%
		{0.091800 * 1.0122, true, 0.096},  // ~12.2% ROI → 命中12%里程碑，锁9.74%
		{0.091800 * 1.0202, true, 0.170},  // ~20.2% ROI → 命中20%里程碑，锁17.2%
		{0.093900, true, 0},               // 22.88% ROI → 命中22%里程碑，锁住18.98%，止损继续上移
		{0.091800 * 1.0252, true, 0.215},  // ~25.2% ROI → 命中25%里程碑，锁21.67%
	}

	nowMs := int64(2_000_000)
	for i, s := range priceSteps {
		nowMs += 30_000
		m := MarketSnapshot{
			Symbol:    "DOGEUSDT",
			LastPrice: s.price,
			MarkPrice: s.price,
			TickSize:  0.00001,
			NowMs:     nowMs,
		}

		plan := eng.Evaluate(pos, m)

		if plan.ShouldUpdate != s.shouldUpdate {
			t.Errorf("步骤%d: price=%.6f ROI=%.2f%% ShouldUpdate=%v 期望%v | %v | %s",
				i, s.price, plan.RoiUnr*100, plan.ShouldUpdate, s.shouldUpdate, plan.Reasons, plan.Note)
		}

		if plan.ShouldUpdate {
			lockedROI := roiAtStop(Long, pos.Entry, plan.NewStop, pos.Leverage)
			if lockedROI < s.minLockedROI-0.002 {
				t.Errorf("步骤%d: 锁住ROI=%.3f，期望>%.3f", i, lockedROI, s.minLockedROI)
			}
			t.Logf("步骤%d: price=%.6f ROI=%.2f%% → 止损%.6f→%.6f 锁住ROI=%.1f%%",
				i, s.price, plan.RoiUnr*100, pos.PrevStop, plan.NewStop, lockedROI*100)
			pos.PrevStop = plan.NewStop
			pos.LastStopUpdateTimeMs = nowMs
		} else {
			t.Logf("步骤%d: price=%.6f ROI=%.2f%% → 不更新 (%v)",
				i, s.price, plan.RoiUnr*100, plan.Reasons)
		}
		pos.ROIArmed = plan.NextROIArmed
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 十、锁盈有效性：止损触发时必然盈利
// ─────────────────────────────────────────────────────────────────────────────

// TestProfitGuarantee 验证：任何里程碑触发后，若止损触发，账户必然盈利
func TestProfitGuarantee(t *testing.T) {
	eng := defaultEng()

	entry := 100000.0
	initStop := 95000.0
	leverage := 10.0

	// 覆盖所有里程碑
	milestoneROIs := []float64{0.05, 0.08, 0.12, 0.16, 0.20, 0.25, 0.30, 0.40, 0.50}

	for _, roi := range milestoneROIs {
		pos := newLong(entry, initStop, initStop, leverage)
		pos.ROIArmed = false

		// 使用比里程碑高0.1%的ROI，避免浮点精度问题导致恰好卡在门槛上
		price := entry * (1 + (roi+0.001)/leverage)
		m := snap(price, 0.1, 2_000_000)
		plan := eng.Evaluate(pos, m)

		if !plan.ShouldUpdate {
			t.Errorf("ROI=%.0f%%：应更新止损", roi*100)
			continue
		}

		// 核心保证：止损触发时账户盈利 > 0
		roiAtTrigger := roiAtStop(Long, entry, plan.NewStop, leverage)
		if roiAtTrigger <= 0 {
			t.Errorf("ROI=%.0f%%：止损触发时ROI=%.2f%%，未锁盈！止损=%.2f，entry=%.2f",
				roi*100, roiAtTrigger*100, plan.NewStop, entry)
		}

		// 止损触发的ROI < 当前ROI（保留了回撤空间）
		if roiAtTrigger >= roi {
			t.Errorf("ROI=%.0f%%：锁住ROI(%.2f%%) >= 当前ROI，没有回撤空间",
				roi*100, roiAtTrigger*100)
		}

		t.Logf("ROI=%5.0f%% → 止损=%.2f，触发时ROI=+%.2f%%（保证盈利✅）",
			roi*100, plan.NewStop, roiAtTrigger*100)
	}
}

// TestProfitGuarantee_Short 同上，SHORT方向
func TestProfitGuarantee_Short(t *testing.T) {
	eng := defaultEng()

	entry := 100000.0
	initStop := 105000.0
	leverage := 10.0

	milestoneROIs := []float64{0.05, 0.12, 0.20, 0.30, 0.50}

	for _, roi := range milestoneROIs {
		pos := newShort(entry, initStop, initStop, leverage)
		pos.ROIArmed = false

		price := entry * (1 - roi/leverage)
		m := snap(price, 0.1, 2_000_000)
		plan := eng.Evaluate(pos, m)

		if !plan.ShouldUpdate {
			t.Errorf("SHORT ROI=%.0f%%：应更新止损", roi*100)
			continue
		}

		roiAtTrigger := roiAtStop(Short, entry, plan.NewStop, leverage)
		if roiAtTrigger <= 0 {
			t.Errorf("SHORT ROI=%.0f%%：止损触发时ROI=%.2f%%，未锁盈！", roi*100, roiAtTrigger*100)
		}

		t.Logf("SHORT ROI=%5.0f%% → 止损=%.2f，触发时ROI=+%.2f%%（保证盈利✅）",
			roi*100, plan.NewStop, roiAtTrigger*100)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 十一、输入验证
// ─────────────────────────────────────────────────────────────────────────────

func TestInvalidInput(t *testing.T) {
	eng := defaultEng()

	cases := []struct {
		desc string
		pos  PositionState
		snap MarketSnapshot
	}{
		{
			"entry=0",
			PositionState{Entry: 0, InitStop: 95000, Leverage: 10, Qty: 1, Side: Long},
			snap(100000, 0.1, 2_000_000),
		},
		{
			"initStop=0",
			PositionState{Entry: 100000, InitStop: 0, Leverage: 10, Qty: 1, Side: Long},
			snap(100000, 0.1, 2_000_000),
		},
		{
			"leverage=0",
			PositionState{Entry: 100000, InitStop: 95000, Leverage: 0, Qty: 1, Side: Long},
			snap(100000, 0.1, 2_000_000),
		},
		{
			"qty=0",
			PositionState{Entry: 100000, InitStop: 95000, Leverage: 10, Qty: 0, Side: Long},
			snap(100000, 0.1, 2_000_000),
		},
		{
			"lastPrice=0",
			PositionState{Entry: 100000, InitStop: 95000, Leverage: 10, Qty: 1, Side: Long},
			snap(0, 0.1, 2_000_000),
		},
		{
			"LONG: initStop > entry（R0<=0）",
			PositionState{Entry: 100000, InitStop: 105000, Leverage: 10, Qty: 1, Side: Long},
			snap(101000, 0.1, 2_000_000),
		},
	}

	for _, c := range cases {
		plan := eng.Evaluate(c.pos, c.snap)
		if plan.ShouldUpdate {
			t.Errorf("[%s] 无效输入不应触发更新", c.desc)
		}
		t.Logf("✅ [%s] 正确拒绝：%v", c.desc, plan.Reasons)
	}
}
