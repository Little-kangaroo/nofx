package protect

// production_symbols_test.go
//
// 生产标的止损锁盈完整验证
//
// 覆盖维度：
//   标的：BTCUSDT / ETHUSDT / BNBUSDT / SOLUSDT / XRPUSDT / DOGEUSDT
//   方向：做多（Long）/ 做空（Short）
//   梯度：12 个里程碑（ROI 5% ~ 100%）
//
// 测试分组：
//   一、地板公式正确性（全标的 × 全里程碑 × 两方向）
//   二、里程碑递进单调性（价格逐步穿越12级）
//   三、拉回后止损不退
//   四、止损触发必然盈利
//   五、Tick 精度对齐
//   六、安全门 ExecGap（floor > 当前价时拒绝）
//   七、冷却期拦截
//   八、ROI 触发门槛边界（2.8% 不触发 / 3.2% 触发保本）
//   九、无效输入鲁棒性

import (
	"fmt"
	"math"
	"testing"
)

// ─────────────────────────────────────────────────────────────────
// 标的定义
// ─────────────────────────────────────────────────────────────────

type prodSym struct {
	Symbol   string
	Entry    float64 // 典型入场价
	TickSize float64
	Leverage float64
}

func (s prodSym) LongInitStop() float64  { return s.Entry * 0.95 } // R0 = 5% × Entry
func (s prodSym) ShortInitStop() float64 { return s.Entry * 1.05 }

var tradingSymbols = []prodSym{
	{"BTCUSDT", 96000.0, 0.1, 10},    // 高价区，tick 粗
	{"ETHUSDT", 3200.0, 0.01, 10},    // 中高价区
	{"BNBUSDT", 550.0, 0.01, 10},     // 中高价区
	{"SOLUSDT", 180.0, 0.001, 10},    // 中价区
	{"XRPUSDT", 0.55, 0.0001, 10},    // 低价区
	{"DOGEUSDT", 0.12, 0.00001, 10},  // 极低价区（历史上出现过 bug 的标的）
}

// ─────────────────────────────────────────────────────────────────
// 止损里程碑表（与 DefaultConfig.StopLossMilestones 完全一致）
// ─────────────────────────────────────────────────────────────────

type prodMilestone struct {
	ROI     float64 // 触发 ROI
	LockPct float64 // 锁住 ROI
}

// prodMilestones 从 DefaultConfig 动态生成，与配置完全对齐（V-22.0）
var prodMilestones = func() []prodMilestone {
	cfg := DefaultConfig()
	keyROIs := []float64{0.05, 0.08, 0.12, 0.16, 0.20, 0.25, 0.30, 0.40, 0.50, 0.60, 0.80, 1.00}
	var ms []prodMilestone
	for _, roi := range keyROIs {
		if floor, ok := cfg.StopLossMilestones[roi]; ok {
			ms = append(ms, prodMilestone{roi, floor})
		}
	}
	return ms
}()

// ─────────────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────────────

// prodEng 构建零冷却引擎（用于单步测试）
func prodEng() *Engine {
	cfg := DefaultConfig()
	cfg.UpdateCooldownSec = 0
	return &Engine{Cfg: cfg}
}

// prodMkSnap 构建行情快照
func prodMkSnap(sym prodSym, price float64, nowMs int64) MarketSnapshot {
	return MarketSnapshot{
		Symbol:    sym.Symbol,
		LastPrice: price,
		MarkPrice: price,
		TickSize:  sym.TickSize,
		NowMs:     nowMs,
	}
}

// prodPriceAtROI 给定 ROI 计算价格（+0.002 缓冲避免浮点卡门槛）
func prodPriceAtROI(side Side, entry, roi, leverage float64) float64 {
	eff := roi + 0.002
	if side == Long {
		return entry * (1 + eff/leverage)
	}
	return entry * (1 - eff/leverage)
}

// prodExpectedFloor 根据公式计算期望地板
func prodExpectedFloor(side Side, entry, lockPct, leverage float64) float64 {
	if side == Long {
		return entry * (1 + lockPct/leverage)
	}
	return entry * (1 - lockPct/leverage)
}

// prodROIAtStop 计算止损价对应的 ROI
func prodROIAtStop(side Side, entry, stop, leverage float64) float64 {
	if side == Long {
		return (stop - entry) / entry * leverage
	}
	return (entry - stop) / entry * leverage
}

// prodIsTickAligned 验证价格是否对齐 tick
func prodIsTickAligned(price, tick float64) bool {
	ratio := price / tick
	return math.Abs(ratio-math.Round(ratio)) < 1e-4
}

// prodPos 构建持仓
func prodPos(sym prodSym, side Side, armed bool) PositionState {
	initStop := sym.LongInitStop()
	if side == Short {
		initStop = sym.ShortInitStop()
	}
	return PositionState{
		Symbol:          sym.Symbol,
		Side:            side,
		Entry:           sym.Entry,
		InitStop:        initStop,
		PrevStop:        initStop,
		Leverage:        sym.Leverage,
		Qty:             1.0,
		ROIArmed:        armed,
		StopTriggerType: TriggerLast,
		OpenTimeMs:      1_000_000,
	}
}

// ─────────────────────────────────────────────────────────────────
// 一、地板公式正确性
// 验证：floor = Entry × (1 ± lockPct/leverage)，误差 ≤ 1 tick
// ─────────────────────────────────────────────────────────────────

func TestProd_FloorFormula(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			for _, ms := range prodMilestones {
				sym, side, ms := sym, side, ms
				name := fmt.Sprintf("%s/%s/ROI%.0f%%", sym.Symbol, side, ms.ROI*100)

				t.Run(name, func(t *testing.T) {
					t.Parallel()
					pos := prodPos(sym, side, true)
					price := prodPriceAtROI(side, sym.Entry, ms.ROI, sym.Leverage)
					plan := eng.Evaluate(pos, prodMkSnap(sym, price, 2_000_000))

					if !plan.ShouldUpdate {
						t.Fatalf("应触发更新: ROI=%.0f%% | %v | %s",
							ms.ROI*100, plan.Reasons, plan.Note)
					}

					// ① 地板公式误差 ≤ 1 tick
					wantFloor := prodExpectedFloor(side, sym.Entry, ms.LockPct, sym.Leverage)
					if math.Abs(plan.Floor-wantFloor) > sym.TickSize {
						t.Errorf("Floor=%.8f, 期望=%.8f, 误差=%.8f > 1 tick=%.8f",
							plan.Floor, wantFloor, math.Abs(plan.Floor-wantFloor), sym.TickSize)
					}

					// ② NewStop 在 entry 有利方向（锁盈已生效）
					if side == Long && plan.NewStop <= sym.Entry {
						t.Errorf("LONG NewStop=%.8f 应 > entry=%.8f", plan.NewStop, sym.Entry)
					}
					if side == Short && plan.NewStop >= sym.Entry {
						t.Errorf("SHORT NewStop=%.8f 应 < entry=%.8f", plan.NewStop, sym.Entry)
					}

					// ③ NewStop 不超过当前价（不立即触发）
					if side == Long && plan.NewStop >= price {
						t.Errorf("LONG NewStop=%.8f >= 当前价=%.8f，止损会立即触发", plan.NewStop, price)
					}
					if side == Short && plan.NewStop <= price {
						t.Errorf("SHORT NewStop=%.8f <= 当前价=%.8f，止损会立即触发", plan.NewStop, price)
					}

					// ④ NewStop tick 对齐
					if !prodIsTickAligned(plan.NewStop, sym.TickSize) {
						t.Errorf("NewStop=%.8f 未对齐 tick=%.8f", plan.NewStop, sym.TickSize)
					}

					locked := prodROIAtStop(side, sym.Entry, plan.NewStop, sym.Leverage)
					t.Logf("Floor=%.8f NewStop=%.8f 锁住ROI=%.2f%% (当前=%.2f%%)",
						plan.Floor, plan.NewStop, locked*100, plan.RoiUnr*100)
				})
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// 二、里程碑递进单调性
// 模拟价格从低到高逐步穿越 12 个里程碑
// 验证：止损只向有利方向移动，且每步锁住的 ROI 不低于该里程碑的 lockPct
// ─────────────────────────────────────────────────────────────────

func TestProd_MilestoneProgression(t *testing.T) {
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

				for i, ms := range prodMilestones {
					nowMs += 30_000
					price := prodPriceAtROI(side, sym.Entry, ms.ROI, sym.Leverage)
					plan := eng.Evaluate(pos, prodMkSnap(sym, price, nowMs))

					if !plan.ShouldUpdate {
						t.Errorf("[M%d ROI=%.0f%%] 应触发更新: %v %s",
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

					// 锁住 ROI 不低于里程碑 lockPct（允许 0.3% 误差）
					locked := prodROIAtStop(side, sym.Entry, plan.NewStop, sym.Leverage)
					if locked < ms.LockPct-0.003 {
						t.Errorf("[M%d] 锁住ROI=%.3f%% < 期望%.3f%%", i+1, locked*100, ms.LockPct*100)
					}

					// 止损在盈利侧
					if side == Long && plan.NewStop <= sym.Entry {
						t.Errorf("[M%d] LONG stop=%.8f 未超过 entry=%.8f", i+1, plan.NewStop, sym.Entry)
					}
					if side == Short && plan.NewStop >= sym.Entry {
						t.Errorf("[M%d] SHORT stop=%.8f 未低于 entry=%.8f", i+1, plan.NewStop, sym.Entry)
					}

					t.Logf("M%2d ROI=%5.0f%% → stop %.8f → %.8f 锁%.1f%%",
						i+1, ms.ROI*100, prevStop, plan.NewStop, locked*100)

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
// 三、拉回后止损不退
// 场景：价格涨到 30% ROI → 止损更新 → 价格回落到 8% → 止损保持不动
// ─────────────────────────────────────────────────────────────────

func TestProd_PullbackHold(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			sym, side := sym, side
			name := fmt.Sprintf("%s/%s", sym.Symbol, side)

			t.Run(name, func(t *testing.T) {
				t.Parallel()
				pos := prodPos(sym, side, false)

				// Step 1：价格达到 30% ROI，触发止损更新
				peakPrice := prodPriceAtROI(side, sym.Entry, 0.30, sym.Leverage)
				plan1 := eng.Evaluate(pos, prodMkSnap(sym, peakPrice, 2_000_000))
				if !plan1.ShouldUpdate {
					t.Fatalf("30%% ROI 应触发止损更新: %v", plan1.Reasons)
				}
				stopAtPeak := plan1.NewStop
				pos.PrevStop = stopAtPeak
				pos.ROIArmed = plan1.NextROIArmed

				locked := prodROIAtStop(side, sym.Entry, stopAtPeak, sym.Leverage)
				t.Logf("高点止损=%.8f 锁住ROI=%.1f%%", stopAtPeak, locked*100)

				// Step 2：价格回落到 8% ROI，止损不应后退
				valleyPrice := prodPriceAtROI(side, sym.Entry, 0.08, sym.Leverage)
				plan2 := eng.Evaluate(pos, prodMkSnap(sym, valleyPrice, 2_030_000))

				if plan2.ShouldUpdate {
					// 若触发，止损只能保持或继续有利方向，不能后退
					if side == Long && plan2.NewStop < stopAtPeak {
						t.Errorf("拉回后 LONG 止损后退: %.8f < 高点%.8f", plan2.NewStop, stopAtPeak)
					}
					if side == Short && plan2.NewStop > stopAtPeak {
						t.Errorf("拉回后 SHORT 止损后退: %.8f > 高点%.8f", plan2.NewStop, stopAtPeak)
					}
				}

				// 止损仍在盈利侧
				if side == Long && pos.PrevStop <= sym.Entry {
					t.Errorf("拉回后 LONG 止损=%.8f 应仍 > entry=%.8f", pos.PrevStop, sym.Entry)
				}
				if side == Short && pos.PrevStop >= sym.Entry {
					t.Errorf("拉回后 SHORT 止损=%.8f 应仍 < entry=%.8f", pos.PrevStop, sym.Entry)
				}
				t.Logf("✓ 拉回到8%% ROI，止损保持在 %.8f (锁住%.1f%%)", stopAtPeak, locked*100)
			})
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// 四、止损触发必然盈利
// 核心生产保证：任何里程碑触发后，若止损触发，ROI > 0
// ─────────────────────────────────────────────────────────────────

func TestProd_ProfitGuarantee(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			for _, ms := range prodMilestones {
				sym, side, ms := sym, side, ms
				name := fmt.Sprintf("%s/%s/ROI%.0f%%", sym.Symbol, side, ms.ROI*100)

				t.Run(name, func(t *testing.T) {
					t.Parallel()
					pos := prodPos(sym, side, true)
					price := prodPriceAtROI(side, sym.Entry, ms.ROI, sym.Leverage)
					plan := eng.Evaluate(pos, prodMkSnap(sym, price, 2_000_000))

					if !plan.ShouldUpdate {
						t.Fatalf("应触发更新: %v %s", plan.Reasons, plan.Note)
					}

					// 核心断言：止损触发时 ROI > 0（必然盈利）
					roiAtTrigger := prodROIAtStop(side, sym.Entry, plan.NewStop, sym.Leverage)
					if roiAtTrigger <= 0 {
						t.Errorf("止损触发时 ROI=%.4f%% ≤ 0，未锁盈！stop=%.8f entry=%.8f",
							roiAtTrigger*100, plan.NewStop, sym.Entry)
					}

					// 锁住 ROI 接近 lockPct（误差 ≤ 0.5%）
					if math.Abs(roiAtTrigger-ms.LockPct) > 0.005 {
						t.Errorf("锁住ROI=%.4f%% 与期望%.4f%% 偏差过大",
							roiAtTrigger*100, ms.LockPct*100)
					}

					t.Logf("✓ 止损触发时ROI=+%.2f%% (期望锁%.0f%% | 当前ROI=%.0f%%)",
						roiAtTrigger*100, ms.LockPct*100, ms.ROI*100)
				})
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// 五、Tick 精度对齐
// 验证各价格区间标的的止损价格正确对齐各自的 tick
// ─────────────────────────────────────────────────────────────────

func TestProd_TickAlignment(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			sym, side := sym, side
			name := fmt.Sprintf("%s/%s", sym.Symbol, side)

			t.Run(name, func(t *testing.T) {
				t.Parallel()
				pos := prodPos(sym, side, true)
				nowMs := int64(2_000_000)

				for _, ms := range prodMilestones {
					price := prodPriceAtROI(side, sym.Entry, ms.ROI, sym.Leverage)
					plan := eng.Evaluate(pos, prodMkSnap(sym, price, nowMs))
					nowMs += 30_000

					if !plan.ShouldUpdate {
						continue
					}

					// NewStop 必须是 tickSize 的整数倍
					ratio := plan.NewStop / sym.TickSize
					deviation := math.Abs(ratio - math.Round(ratio))
					if deviation > 1e-4 {
						t.Errorf("ROI=%.0f%%: NewStop=%.10f 未对齐 tick=%.10f (偏差=%.2e)",
							ms.ROI*100, plan.NewStop, sym.TickSize, deviation)
					}

					if plan.NewStop <= 0 {
						t.Errorf("ROI=%.0f%%: NewStop=%.10f 不合法（<=0）", ms.ROI*100, plan.NewStop)
					}
				}
				t.Logf("✓ %s %s tick=%.8f 对齐验证通过", sym.Symbol, side, sym.TickSize)
			})
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// 六、安全门 ExecGap
// 当 floor > 当前价时，引擎必须拒绝更新（防止止损立即触发）
// ─────────────────────────────────────────────────────────────────

func TestProd_ExecGapSafety(t *testing.T) {
	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			sym, side := sym, side
			name := fmt.Sprintf("%s/%s", sym.Symbol, side)

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				// 注入极端里程碑：5% ROI 时锁住 90%，floor 必然超过当前价
				cfg := DefaultConfig()
				cfg.UpdateCooldownSec = 0
				cfg.StopLossMilestones = map[float64]float64{0.05: 0.90}
				eng := &Engine{Cfg: cfg}

				pos := prodPos(sym, side, true)
				// 当前价仅 5.2% ROI，但 floor 需要锁住 90% → floor >> 当前价
				price := prodPriceAtROI(side, sym.Entry, 0.05, sym.Leverage)
				plan := eng.Evaluate(pos, prodMkSnap(sym, price, 2_000_000))

				if plan.ShouldUpdate {
					t.Errorf("安全阀未生效：floor=%.8f 超过当前价=%.8f 仍更新 NewStop=%.8f",
						plan.Floor, price, plan.NewStop)
				}
				if !plan.ExecGap {
					t.Errorf("ExecGap 标志应为 true")
				}
				t.Logf("✓ ExecGap 生效: floor=%.8f, ref=%.8f → 拒绝", plan.Floor, plan.RefPrice)
			})
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// 七、冷却期拦截
// 验证 25 秒冷却期内不允许二次更新
// ─────────────────────────────────────────────────────────────────

func TestProd_Cooldown(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UpdateCooldownSec = 25
	eng := &Engine{Cfg: cfg}

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			sym, side := sym, side
			name := fmt.Sprintf("%s/%s", sym.Symbol, side)

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				pos := prodPos(sym, side, true)
				pos.LastStopUpdateTimeMs = 2_000_000 // 刚刚更新过

				price := prodPriceAtROI(side, sym.Entry, 0.20, sym.Leverage)

				// 冷却期内（10 秒 < 25 秒）：不允许更新
				plan1 := eng.Evaluate(pos, prodMkSnap(sym, price, 2_010_000))
				if plan1.ShouldUpdate {
					t.Errorf("冷却期内（10s）不应更新")
				}
				hasCooldown := false
				for _, r := range plan1.Reasons {
					if r == ReasonCooldown {
						hasCooldown = true
					}
				}
				if !hasCooldown {
					t.Errorf("原因应含 COOLDOWN，实际: %v", plan1.Reasons)
				}

				// 冷却期后（30 秒 > 25 秒）：允许更新
				plan2 := eng.Evaluate(pos, prodMkSnap(sym, price, 2_030_000))
				if !plan2.ShouldUpdate {
					t.Errorf("冷却期后（30s）应允许更新: %v %s", plan2.Reasons, plan2.Note)
				}

				t.Logf("✓ 冷却拦截10s / 放行30s")
			})
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// 八、ROI 触发门槛边界
// 0.3% ROI → 不触发；0.6% ROI → 触发（V-24.0: 阈值0.5%）
// ─────────────────────────────────────────────────────────────────

func TestProd_ROIThresholdBoundary(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		for _, side := range []Side{Long, Short} {
			sym, side := sym, side
			name := fmt.Sprintf("%s/%s", sym.Symbol, side)

			t.Run(name, func(t *testing.T) {
				t.Parallel()
				pos := prodPos(sym, side, false)

				// 0.2% ROI：低于 0.5% 门槛，不触发
				priceBelow := prodPriceAtROI(side, sym.Entry, 0.002, sym.Leverage)
				planA := eng.Evaluate(pos, prodMkSnap(sym, priceBelow, 2_000_000))
				if planA.ShouldUpdate {
					t.Errorf("ROI=%.2f%% < 0.5%%，不应触发（当前ROI=%.3f%%）",
						0.2, planA.RoiUnr*100)
				}
				if planA.NextROIArmed {
					t.Errorf("ROI<0.5%% 时 NextROIArmed 不应为 true")
				}

				// 0.8% ROI：高于 0.5% 门槛，触发
				priceAbove := prodPriceAtROI(side, sym.Entry, 0.008, sym.Leverage)
				planB := eng.Evaluate(pos, prodMkSnap(sym, priceAbove, 2_000_000))
				if !planB.ShouldUpdate {
					t.Errorf("ROI=%.2f%% ≥ 0.5%%，应触发: %v %s",
						planB.RoiUnr*100, planB.Reasons, planB.Note)
				}
				if !planB.NextROIArmed {
					t.Errorf("ROI≥0.5%% 后 NextROIArmed 应为 true")
				}
				// 0.5%-2% 区间止损仍在入场价不利方向（缓冲区），不做保本断言

				t.Logf("✓ 门槛验证: ROI=%.2f%%不触发 / ROI=%.2f%%触发(保本)",
					planA.RoiUnr*100, planB.RoiUnr*100)
			})
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// 九、无效输入鲁棒性
// ─────────────────────────────────────────────────────────────────

func TestProd_InvalidInput(t *testing.T) {
	eng := prodEng()

	for _, sym := range tradingSymbols {
		sym := sym
		t.Run(sym.Symbol, func(t *testing.T) {
			t.Parallel()

			cases := []struct {
				desc string
				pos  PositionState
				snap MarketSnapshot
			}{
				{
					"entry=0",
					PositionState{Symbol: sym.Symbol, Side: Long, Entry: 0,
						InitStop: sym.LongInitStop(), Leverage: sym.Leverage, Qty: 1},
					prodMkSnap(sym, sym.Entry*1.06, 2_000_000),
				},
				{
					"initStop=0",
					PositionState{Symbol: sym.Symbol, Side: Long, Entry: sym.Entry,
						InitStop: 0, Leverage: sym.Leverage, Qty: 1},
					prodMkSnap(sym, sym.Entry*1.06, 2_000_000),
				},
				{
					"leverage=0",
					PositionState{Symbol: sym.Symbol, Side: Long, Entry: sym.Entry,
						InitStop: sym.LongInitStop(), Leverage: 0, Qty: 1},
					prodMkSnap(sym, sym.Entry*1.06, 2_000_000),
				},
				{
					"qty=0",
					PositionState{Symbol: sym.Symbol, Side: Long, Entry: sym.Entry,
						InitStop: sym.LongInitStop(), Leverage: sym.Leverage, Qty: 0},
					prodMkSnap(sym, sym.Entry*1.06, 2_000_000),
				},
				{
					"price=0",
					PositionState{Symbol: sym.Symbol, Side: Long, Entry: sym.Entry,
						InitStop: sym.LongInitStop(), Leverage: sym.Leverage, Qty: 1},
					prodMkSnap(sym, 0, 2_000_000),
				},
				{
					"LONG:initStop>entry（R0<=0）",
					PositionState{Symbol: sym.Symbol, Side: Long, Entry: sym.Entry,
						InitStop: sym.Entry * 1.05, Leverage: sym.Leverage, Qty: 1},
					prodMkSnap(sym, sym.Entry*1.06, 2_000_000),
				},
				{
					"SHORT:initStop<entry（R0<=0）",
					PositionState{Symbol: sym.Symbol, Side: Short, Entry: sym.Entry,
						InitStop: sym.Entry * 0.95, Leverage: sym.Leverage, Qty: 1},
					prodMkSnap(sym, sym.Entry*0.94, 2_000_000),
				},
			}

			for _, c := range cases {
				plan := eng.Evaluate(c.pos, c.snap)
				if plan.ShouldUpdate {
					t.Errorf("[%s] 无效输入不应触发止损更新", c.desc)
				}
			}
			t.Logf("✓ %s 全部无效输入正确拒绝（7 种）", sym.Symbol)
		})
	}
}
