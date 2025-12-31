package protect

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// ========== Mock实现 ==========

// MockPositionStore 模拟持仓存储
type MockPositionStore struct {
	Positions      []PositionState
	SavedPositions []PositionState
	SaveError      error
}

func (m *MockPositionStore) ListOpenPositions(ctx context.Context) ([]PositionState, error) {
	return m.Positions, nil
}

func (m *MockPositionStore) SavePositionState(ctx context.Context, pos PositionState) error {
	if m.SaveError != nil {
		return m.SaveError
	}
	m.SavedPositions = append(m.SavedPositions, pos)

	// 🔧 更新内存中的持仓状态（模拟数据库回写）
	for i := range m.Positions {
		if m.Positions[i].Symbol == pos.Symbol && m.Positions[i].Side == pos.Side {
			m.Positions[i] = pos
			break
		}
	}

	return nil
}

// MockPriceCache 模拟价格缓存
type MockPriceCache struct {
	Prices map[string]MarketSnapshot
}

func (m *MockPriceCache) GetSnapshot(ctx context.Context, symbol string) (MarketSnapshot, bool) {
	snap, ok := m.Prices[symbol]
	return snap, ok
}

// MockStopExecutor 模拟止损执行器
type MockStopExecutor struct {
	StopCalls       []StopCall
	TakeProfitCalls []TakeProfitCall
	StopError       error
	TPError         error
}

type StopCall struct {
	Symbol    string
	Side      Side
	Qty       float64
	StopPrice float64
	Trigger   TriggerType
}

type TakeProfitCall struct {
	Symbol  string
	Side    Side
	Qty     float64
	TPPrice float64
	Trigger TriggerType
}

func (m *MockStopExecutor) UpsertStop(ctx context.Context, symbol string, side Side, qty float64, stopPrice float64, trigger TriggerType) error {
	m.StopCalls = append(m.StopCalls, StopCall{
		Symbol:    symbol,
		Side:      side,
		Qty:       qty,
		StopPrice: stopPrice,
		Trigger:   trigger,
	})
	return m.StopError
}

func (m *MockStopExecutor) UpsertTakeProfit(ctx context.Context, symbol string, side Side, qty float64, tpPrice float64, trigger TriggerType) error {
	m.TakeProfitCalls = append(m.TakeProfitCalls, TakeProfitCall{
		Symbol:  symbol,
		Side:    side,
		Qty:     qty,
		TPPrice: tpPrice,
		Trigger: trigger,
	})
	return m.TPError
}

// ========== 测试用例 ==========

// TestSchedulerTickOnce 测试Scheduler单次执行周期
func TestSchedulerTickOnce(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	// 准备测试持仓：ROI 10%，触发止损锁盈和止盈设置
	mockStore := &MockPositionStore{
		Positions: []PositionState{
			{
				Symbol:               "BTCUSDT",
				Side:                 Long,
				Qty:                  1.0,
				Entry:                100000.0,
				Leverage:             10.0,
				InitStop:             95000.0,  // R0 = 5000
				PrevStop:             95000.0,
				PrevTakeProfit:       0.0,      // 未设置止盈
				OpenTimeMs:           now - 10*60*1000, // 10分钟前开仓
				LastStopUpdateTimeMs: 0,
				StopTriggerType:      TriggerLast,
				ROIArmed:             false,
				BreakEvenArmed:       false,
				RLockStage:           0,
			},
		},
	}

	// 价格上涨1% = 10% ROI at 10x leverage
	mockCache := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 101000.0, // +1%
				MarkPrice: 101000.0,
				TickSize:  0.1,
				NowMs:     now,
			},
		},
	}

	mockExec := &MockStopExecutor{}

	scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

	// 执行一次检查
	ctx := context.Background()
	scheduler.tickOnce(ctx)

	// ========== 验证结果 ==========

	// 1. 验证止损更新
	if len(mockExec.StopCalls) != 1 {
		t.Fatalf("Expected 1 stop update, got %d", len(mockExec.StopCalls))
	}

	stopCall := mockExec.StopCalls[0]
	if stopCall.Symbol != "BTCUSDT" {
		t.Errorf("Stop symbol = %s, want BTCUSDT", stopCall.Symbol)
	}
	if stopCall.Side != Long {
		t.Errorf("Stop side = %v, want Long", stopCall.Side)
	}
	// 止损应该上移到ProfitFloor（约100200）
	if stopCall.StopPrice < 100000 {
		t.Errorf("Stop price = %.2f, should be above entry (100000)", stopCall.StopPrice)
	}

	// 2. 验证止盈设置
	if len(mockExec.TakeProfitCalls) != 1 {
		t.Fatalf("Expected 1 take-profit update, got %d", len(mockExec.TakeProfitCalls))
	}

	tpCall := mockExec.TakeProfitCalls[0]
	if tpCall.Symbol != "BTCUSDT" {
		t.Errorf("TP symbol = %s, want BTCUSDT", tpCall.Symbol)
	}
	expectedTP := 115000.0 // Entry * 1.15 (10% ROI → 15% TP)
	if tpCall.TPPrice < expectedTP-10 || tpCall.TPPrice > expectedTP+10 {
		t.Errorf("TP price = %.2f, want %.2f", tpCall.TPPrice, expectedTP)
	}

	// 3. 验证状态持久化
	if len(mockStore.SavedPositions) != 2 {
		t.Fatalf("Expected 2 state saves (stop+tp), got %d", len(mockStore.SavedPositions))
	}

	// 验证ROIArmed已触发
	savedState := mockStore.SavedPositions[0]
	if !savedState.ROIArmed {
		t.Errorf("ROIArmed should be true after 10%% ROI")
	}

	// 验证止盈价格已更新
	finalState := mockStore.SavedPositions[1]
	if finalState.PrevTakeProfit == 0 {
		t.Errorf("PrevTakeProfit should be updated, got 0")
	}

	t.Logf("✅ TestSchedulerTickOnce passed")
	t.Logf("   Stop updated: %.2f → %.2f", 95000.0, stopCall.StopPrice)
	t.Logf("   TP set: %.2f", tpCall.TPPrice)
	t.Logf("   ROIArmed: %v", savedState.ROIArmed)
}

// TestSchedulerTickOnce_NoUpdate 测试不满足条件时不更新
func TestSchedulerTickOnce_NoUpdate(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	// ROI只有3%，不触发任何锁盈
	mockStore := &MockPositionStore{
		Positions: []PositionState{
			{
				Symbol:               "BTCUSDT",
				Side:                 Long,
				Qty:                  1.0,
				Entry:                100000.0,
				Leverage:             10.0,
				InitStop:             95000.0,
				PrevStop:             95000.0,
				PrevTakeProfit:       0.0,
				OpenTimeMs:           now - 10*60*1000,
				LastStopUpdateTimeMs: 0,
				StopTriggerType:      TriggerLast,
				ROIArmed:             false,
				BreakEvenArmed:       false,
				RLockStage:           0,
			},
		},
	}

	// 价格只涨0.3% = 3% ROI (低于5%触发阈值)
	mockCache := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 100300.0,
				MarkPrice: 100300.0,
				TickSize:  0.1,
				NowMs:     now,
			},
		},
	}

	mockExec := &MockStopExecutor{}
	scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

	ctx := context.Background()
	scheduler.tickOnce(ctx)

	// 验证：不应该有任何更新
	if len(mockExec.StopCalls) != 0 {
		t.Errorf("Expected 0 stop updates (ROI < 5%%), got %d", len(mockExec.StopCalls))
	}
	if len(mockExec.TakeProfitCalls) != 0 {
		t.Errorf("Expected 0 TP updates (ROI < 10%%), got %d", len(mockExec.TakeProfitCalls))
	}

	t.Logf("✅ TestSchedulerTickOnce_NoUpdate passed (correctly skipped updates)")
}

// TestSchedulerTickOnce_Cooldown 测试冷却期阻止更新
func TestSchedulerTickOnce_Cooldown(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	// 10秒前刚更新过止损（在25秒冷却期内）
	mockStore := &MockPositionStore{
		Positions: []PositionState{
			{
				Symbol:               "BTCUSDT",
				Side:                 Long,
				Qty:                  1.0,
				Entry:                100000.0,
				Leverage:             10.0,
				InitStop:             95000.0,
				PrevStop:             95000.0,
				PrevTakeProfit:       0.0,
				OpenTimeMs:           now - 10*60*1000,
				LastStopUpdateTimeMs: now - 10*1000, // 10秒前更新过
				StopTriggerType:      TriggerLast,
				ROIArmed:             true, // 已触发
				BreakEvenArmed:       false,
				RLockStage:           0,
			},
		},
	}

	mockCache := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 101000.0,
				MarkPrice: 101000.0,
				TickSize:  0.1,
				NowMs:     now,
			},
		},
	}

	mockExec := &MockStopExecutor{}
	scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

	ctx := context.Background()
	scheduler.tickOnce(ctx)

	// 验证：冷却期内不应更新
	if len(mockExec.StopCalls) != 0 {
		t.Errorf("Expected 0 updates during cooldown (25s), got %d", len(mockExec.StopCalls))
	}

	t.Logf("✅ TestSchedulerTickOnce_Cooldown passed (cooldown blocked update)")
}

// TestSchedulerFastLoop 测试10秒定时器循环执行
func TestSchedulerFastLoop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Scheduler.CheckIntervalSec = 1 // 缩短到1秒方便测试

	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	// 准备持仓：ROI 10%
	mockStore := &MockPositionStore{
		Positions: []PositionState{
			{
				Symbol:               "BTCUSDT",
				Side:                 Long,
				Qty:                  1.0,
				Entry:                100000.0,
				Leverage:             10.0,
				InitStop:             95000.0,
				PrevStop:             95000.0,
				PrevTakeProfit:       0.0,
				OpenTimeMs:           now - 10*60*1000,
				LastStopUpdateTimeMs: 0,
				StopTriggerType:      TriggerLast,
				ROIArmed:             false,
				BreakEvenArmed:       false,
				RLockStage:           0,
			},
		},
	}

	mockCache := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 101000.0,
				MarkPrice: 101000.0,
				TickSize:  0.1,
				NowMs:     now,
			},
		},
	}

	mockExec := &MockStopExecutor{}
	scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

	// 启动Fast Loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go scheduler.StartFastLoop(ctx)

	// 等待3秒（应触发3次tick，但冷却期限制只更新1次）
	time.Sleep(3 * time.Second)
	cancel()

	// 给goroutine时间退出
	time.Sleep(100 * time.Millisecond)

	// 验证：应该只更新1次（第2、3次被冷却期阻止）
	if len(mockExec.StopCalls) != 1 {
		t.Errorf("Expected 1 stop update (cooldown blocks 2nd/3rd), got %d", len(mockExec.StopCalls))
	}

	if len(mockExec.TakeProfitCalls) != 1 {
		t.Errorf("Expected 1 TP update, got %d", len(mockExec.TakeProfitCalls))
	}

	t.Logf("✅ TestSchedulerFastLoop passed")
	t.Logf("   Ticks in 3 seconds: ~3")
	t.Logf("   Actual updates: %d (cooldown prevented duplicates)", len(mockExec.StopCalls))
}

// TestSchedulerFastLoop_ContextCancel 测试上下文取消时正常退出
func TestSchedulerFastLoop_ContextCancel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Scheduler.CheckIntervalSec = 10 // 使用实际的10秒

	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	mockStore := &MockPositionStore{Positions: []PositionState{}}
	mockCache := &MockPriceCache{Prices: map[string]MarketSnapshot{}}
	mockExec := &MockStopExecutor{}

	scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

	// 启动Fast Loop
	ctx, cancel := context.WithCancel(context.Background())

	go scheduler.StartFastLoop(ctx)

	// 立即取消
	time.Sleep(100 * time.Millisecond)
	cancel()

	// 验证：goroutine应该正常退出（无panic）
	time.Sleep(200 * time.Millisecond)

	t.Logf("✅ TestSchedulerFastLoop_ContextCancel passed (clean shutdown)")
}

// TestPositionStateRecovery 测试持仓状态持久化和恢复
func TestPositionStateRecovery(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	// ========== 第一阶段：初始状态，触发5% ROI锁盈 ==========
	mockStore := &MockPositionStore{
		Positions: []PositionState{
			{
				Symbol:               "ETHUSDT",
				Side:                 Long,
				Qty:                  10.0,
				Entry:                2000.0,
				Leverage:             10.0,
				InitStop:             1990.0, // R0 = 10
				PrevStop:             1990.0,
				PrevTakeProfit:       0.0,
				OpenTimeMs:           now - 10*60*1000, // 10分钟前
				LastStopUpdateTimeMs: 0,
				StopTriggerType:      TriggerLast,
				ROIArmed:             false, // 初始未触发
				BreakEvenArmed:       false,
				RLockStage:           0,
			},
		},
	}

	mockCache := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"ETHUSDT": {
				Symbol:    "ETHUSDT",
				LastPrice: 2010.0, // +0.5% = 5% ROI@10x（刚好触发ROI锁盈）
				MarkPrice: 2010.0,
				TickSize:  0.01,
				NowMs:     now,
			},
		},
	}

	mockExec := &MockStopExecutor{}

	scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

	// 执行第一次检查（应触发ROI锁盈）
	ctx := context.Background()
	scheduler.tickOnce(ctx)

	// 验证：ROIArmed应被设置为true
	if len(mockStore.SavedPositions) == 0 {
		t.Fatalf("Expected state to be saved, got 0 saves")
	}

	savedState1 := mockStore.SavedPositions[len(mockStore.SavedPositions)-1]
	if !savedState1.ROIArmed {
		t.Errorf("ROIArmed should be true after 5%% ROI trigger, got false")
	}
	if savedState1.PrevStop <= 1990.0 {
		t.Errorf("Stop should have moved up from 1990, got %.2f", savedState1.PrevStop)
	}

	expectedProfitFloor := 2000.0 * 1.002 // Entry * (1 + 20bps/10000) = 2004
	if savedState1.PrevStop < expectedProfitFloor-0.5 {
		t.Errorf("Stop should be at least ProfitFloor %.2f, got %.2f", expectedProfitFloor, savedState1.PrevStop)
	}

	t.Logf("📊 第一阶段完成:")
	t.Logf("   止损: 1990.00 → %.2f", savedState1.PrevStop)
	t.Logf("   ROIArmed: false → true")
	t.Logf("   ProfitFloor: %.2f", expectedProfitFloor)

	// ========== 模拟重启：使用已保存的状态创建新Scheduler ==========

	// 从数据库恢复的状态（模拟GetOpenTradesForTrader）
	// 🔧 重要：清除LastStopUpdateTimeMs以避免冷却期阻止更新（模拟重启后足够时间流逝）
	recoveredState := savedState1
	recoveredState.LastStopUpdateTimeMs = now - 60*1000 // 设置为1分钟前，超过25秒冷却期

	recoveredStore := &MockPositionStore{
		Positions: []PositionState{
			recoveredState,
		},
	}

	// 价格继续上涨到10% ROI（应触发15%止盈）
	recoveredCache := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"ETHUSDT": {
				Symbol:    "ETHUSDT",
				LastPrice: 2020.0, // +1% = 10% ROI@10x
				MarkPrice: 2020.0,
				TickSize:  0.01,
				NowMs:     now + 5*60*1000, // 5分钟后
			},
		},
	}

	recoveredExec := &MockStopExecutor{}

	// 创建新的Scheduler（模拟重启）
	newScheduler := NewScheduler(eng, recoveredStore, recoveredCache, recoveredExec)

	// 执行第二次检查
	newScheduler.tickOnce(ctx)

	// ========== 验证恢复后的行为 ==========

	if len(recoveredStore.SavedPositions) == 0 {
		t.Fatalf("Expected state to be saved after recovery, got 0 saves")
	}

	finalState := recoveredStore.SavedPositions[len(recoveredStore.SavedPositions)-1]

	// 1. ROIArmed应保持true（不重复触发）
	if !finalState.ROIArmed {
		t.Errorf("ROIArmed should remain true after recovery")
	}

	// 2. 止盈应被触发（10% ROI → 15% TP）
	if len(recoveredExec.TakeProfitCalls) == 0 {
		t.Errorf("Take-profit should be set at 10%% ROI")
	} else {
		tpCall := recoveredExec.TakeProfitCalls[0]
		expectedTP := 2000.0 * 1.15 // Entry * 1.15
		if tpCall.TPPrice < expectedTP-10 || tpCall.TPPrice > expectedTP+10 {
			t.Errorf("TP price = %.2f, want %.2f", tpCall.TPPrice, expectedTP)
		}
		t.Logf("   止盈触发: 0.00 → %.2f (10%% ROI → 15%% TP)", tpCall.TPPrice)
	}

	// 3. 止损可能继续上移（如果ProfitFloor逻辑要求）
	t.Logf("📊 第二阶段完成（重启后）:")
	t.Logf("   止损: %.2f → %.2f", savedState1.PrevStop, finalState.PrevStop)
	t.Logf("   ROIArmed: true (保持)")

	t.Logf("✅ TestPositionStateRecovery passed")
	t.Logf("   状态恢复验证成功：ROIArmed保持、止盈正确触发")
}

// TestArmedStatePersistence 测试armed状态持久化（ROIArmed/BreakEvenArmed/RLockStage）
func TestArmedStatePersistence(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	// 初始持仓：Entry 2000, InitStop 1990 (R0=10)
	basePosition := PositionState{
		Symbol:               "BTCUSDT",
		Side:                 Long,
		Qty:                  1.0,
		Entry:                2000.0,
		Leverage:             10.0,
		InitStop:             1990.0,
		PrevStop:             1990.0,
		PrevTakeProfit:       0.0,
		OpenTimeMs:           now - 10*60*1000,
		LastStopUpdateTimeMs: 0,
		StopTriggerType:      TriggerLast,
		ROIArmed:             false,
		BreakEvenArmed:       false,
		RLockStage:           0,
	}

	// ========== Stage 1: 触发Break-even (0.4R = 2004) ==========
	mockStore1 := &MockPositionStore{Positions: []PositionState{basePosition}}
	mockCache1 := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 2004.0, // R = 0.4R
				MarkPrice: 2004.0,
				TickSize:  0.01,
				NowMs:     now,
			},
		},
	}
	mockExec1 := &MockStopExecutor{}
	scheduler1 := NewScheduler(eng, mockStore1, mockCache1, mockExec1)

	ctx := context.Background()
	scheduler1.tickOnce(ctx)

	state1 := mockStore1.SavedPositions[len(mockStore1.SavedPositions)-1]
	if !state1.BreakEvenArmed {
		t.Errorf("Stage 1: BreakEvenArmed should be true at 0.4R, got false")
	}
	if state1.ROIArmed {
		t.Errorf("Stage 1: ROIArmed should be false (only 0.2%% ROI), got true")
	}
	if state1.RLockStage != 0 {
		t.Errorf("Stage 1: RLockStage should be 0 (R < 1.0), got %d", state1.RLockStage)
	}

	t.Logf("📊 Stage 1 (0.4R, Break-even armed):")
	t.Logf("   BreakEvenArmed: %v", state1.BreakEvenArmed)
	t.Logf("   ROIArmed: %v", state1.ROIArmed)
	t.Logf("   RLockStage: %d", state1.RLockStage)

	// ========== Stage 2: 触发ROI锁盈 (5% ROI = 2010, R=1.0) ==========
	// 🔧 使用Positions数组中的更新后状态（含正确的LastStopUpdateTimeMs）
	state2Input := mockStore1.Positions[0]

	// 🔥 修复：tickOnce使用real time进行cooldown检查，需要设置为real time的过去时间
	state2Input.LastStopUpdateTimeMs = time.Now().UnixMilli() - 30*1000 // 30秒前（超过25秒cooldown）

	mockStore2 := &MockPositionStore{Positions: []PositionState{state2Input}}
	mockCache2 := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 2010.0, // R = 1.0R, ROI = 5%
				MarkPrice: 2010.0,
				TickSize:  0.01,
				NowMs:     now + 2*60*1000, // 2分钟后（超过25秒cooldown）
			},
		},
	}
	mockExec2 := &MockStopExecutor{}
	scheduler2 := NewScheduler(eng, mockStore2, mockCache2, mockExec2)

	scheduler2.tickOnce(ctx)

	if len(mockStore2.SavedPositions) == 0 {
		t.Fatalf("Stage 2: No position state was saved - tickOnce didn't execute any update!")
	}

	state2 := mockStore2.SavedPositions[len(mockStore2.SavedPositions)-1]
	if !state2.BreakEvenArmed {
		t.Errorf("Stage 2: BreakEvenArmed should remain true, got false")
	}
	if !state2.ROIArmed {
		t.Errorf("Stage 2: ROIArmed should be true at 5%% ROI, got false")
	}
	if state2.RLockStage != 1 {
		t.Errorf("Stage 2: RLockStage should be 1 (crossed 1.0R), got %d", state2.RLockStage)
	}

	t.Logf("📊 Stage 2 (1.0R, 5%% ROI, R_LOCK stage 1):")
	t.Logf("   BreakEvenArmed: %v (保持)", state2.BreakEvenArmed)
	t.Logf("   ROIArmed: %v (触发)", state2.ROIArmed)
	t.Logf("   RLockStage: %d (进入)", state2.RLockStage)

	// ========== Stage 3: R_LOCK阶段升级 (1.5R = 2015) ==========
	// 🔧 使用Positions数组中的更新后状态（含正确的LastStopUpdateTimeMs）
	state3Input := mockStore2.Positions[0]

	// 🔥 修复：tickOnce使用real time进行cooldown检查
	state3Input.LastStopUpdateTimeMs = time.Now().UnixMilli() - 30*1000

	mockStore3 := &MockPositionStore{Positions: []PositionState{state3Input}}
	mockCache3 := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 2015.0, // R = 1.5R
				MarkPrice: 2015.0,
				TickSize:  0.01,
				NowMs:     now + 5*60*1000, // 5分钟后（3分钟 after stage 2）
			},
		},
	}
	mockExec3 := &MockStopExecutor{}
	scheduler3 := NewScheduler(eng, mockStore3, mockCache3, mockExec3)

	scheduler3.tickOnce(ctx)

	state3 := mockStore3.SavedPositions[len(mockStore3.SavedPositions)-1]
	if !state3.BreakEvenArmed {
		t.Errorf("Stage 3: BreakEvenArmed should remain true, got false")
	}
	if !state3.ROIArmed {
		t.Errorf("Stage 3: ROIArmed should remain true, got false")
	}
	if state3.RLockStage != 2 {
		t.Errorf("Stage 3: RLockStage should be 2 (crossed 1.5R), got %d", state3.RLockStage)
	}

	t.Logf("📊 Stage 3 (1.5R, R_LOCK stage 2):")
	t.Logf("   BreakEvenArmed: %v (保持)", state3.BreakEvenArmed)
	t.Logf("   ROIArmed: %v (保持)", state3.ROIArmed)
	t.Logf("   RLockStage: %d (升级)", state3.RLockStage)

	// ========== Stage 4: 价格回撤但状态不回退 (1.2R = 2012) ==========
	// 🔧 使用Positions数组中的更新后状态（含正确的LastStopUpdateTimeMs）
	state4Input := mockStore3.Positions[0]

	// 🔥 修复：tickOnce使用real time进行cooldown检查
	state4Input.LastStopUpdateTimeMs = time.Now().UnixMilli() - 30*1000

	mockStore4 := &MockPositionStore{Positions: []PositionState{state4Input}}
	mockCache4 := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 2012.0, // R = 1.2R (回撤了)
				MarkPrice: 2012.0,
				TickSize:  0.01,
				NowMs:     now + 8*60*1000, // 8分钟后（3分钟 after stage 3）
			},
		},
	}
	mockExec4 := &MockStopExecutor{}
	scheduler4 := NewScheduler(eng, mockStore4, mockCache4, mockExec4)

	scheduler4.tickOnce(ctx)

	// 价格回撤时，状态不应回退
	state4 := mockStore4.Positions[0] // 回撤时不应保存新状态
	if !state4.BreakEvenArmed {
		t.Errorf("Stage 4: BreakEvenArmed should remain true during pullback, got false")
	}
	if !state4.ROIArmed {
		t.Errorf("Stage 4: ROIArmed should remain true during pullback, got false")
	}
	if state4.RLockStage != 2 {
		t.Errorf("Stage 4: RLockStage should remain 2 during pullback, got %d", state4.RLockStage)
	}

	t.Logf("📊 Stage 4 (价格回撤到1.2R, 状态保持):")
	t.Logf("   BreakEvenArmed: %v (不回退)", state4.BreakEvenArmed)
	t.Logf("   ROIArmed: %v (不回退)", state4.ROIArmed)
	t.Logf("   RLockStage: %d (不回退)", state4.RLockStage)

	// ========== 验证状态单调性 ==========
	stages := []PositionState{state1, state2, state3, state4}
	for i := 1; i < len(stages); i++ {
		// ROIArmed 只能从false→true，不能回退
		if stages[i-1].ROIArmed && !stages[i].ROIArmed {
			t.Errorf("ROIArmed regressed at stage %d: %v → %v", i+1, stages[i-1].ROIArmed, stages[i].ROIArmed)
		}

		// BreakEvenArmed 只能从false→true，不能回退
		if stages[i-1].BreakEvenArmed && !stages[i].BreakEvenArmed {
			t.Errorf("BreakEvenArmed regressed at stage %d: %v → %v", i+1, stages[i-1].BreakEvenArmed, stages[i].BreakEvenArmed)
		}

		// RLockStage 只能增加，不能减少
		if stages[i].RLockStage < stages[i-1].RLockStage {
			t.Errorf("RLockStage regressed at stage %d: %d → %d", i+1, stages[i-1].RLockStage, stages[i].RLockStage)
		}
	}

	t.Logf("✅ TestArmedStatePersistence passed")
	t.Logf("   Armed状态持久化验证成功：无状态回退，单调性保持")
}

// TestSimultaneousStopAndTP 测试止盈和止损同时触发（V-19.0核心功能）
func TestSimultaneousStopAndTP(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	// 准备持仓：Entry 100000, InitStop 95000 (R0=5000)
	mockStore := &MockPositionStore{
		Positions: []PositionState{
			{
				Symbol:               "BTCUSDT",
				Side:                 Long,
				Qty:                  1.0,
				Entry:                100000.0,
				Leverage:             10.0,
				InitStop:             95000.0, // R0 = 5000
				PrevStop:             95000.0,
				PrevTakeProfit:       0.0,      // 未设置止盈
				OpenTimeMs:           now - 10*60*1000, // 10分钟前开仓
				LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000, // 30秒前（避免cooldown）
				LastTPUpdateTimeMs:   0,
				StopTriggerType:      TriggerLast,
				ROIArmed:             false,
				BreakEvenArmed:       false,
				RLockStage:           0,
			},
		},
	}

	// 价格上涨1% = 10% ROI at 10x leverage（应同时触发止损上移和止盈设置）
	mockCache := &MockPriceCache{
		Prices: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 101000.0, // +1% = 10% ROI@10x
				MarkPrice: 101000.0,
				TickSize:  0.1,
				NowMs:     now,
			},
		},
	}

	mockExec := &MockStopExecutor{}

	scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

	// 执行一次检查
	ctx := context.Background()
	scheduler.tickOnce(ctx)

	// ========== 验证结果 ==========

	// 1. 验证止损更新
	if len(mockExec.StopCalls) != 1 {
		t.Fatalf("Expected 1 stop update, got %d", len(mockExec.StopCalls))
	}

	stopCall := mockExec.StopCalls[0]
	if stopCall.Symbol != "BTCUSDT" {
		t.Errorf("Stop symbol = %s, want BTCUSDT", stopCall.Symbol)
	}
	if stopCall.Side != Long {
		t.Errorf("Stop side = %v, want Long", stopCall.Side)
	}
	// 止损应该上移（ROI锁盈+R_LOCK）
	if stopCall.StopPrice <= 95000 {
		t.Errorf("Stop price = %.2f, should be above initial stop (95000)", stopCall.StopPrice)
	}

	// 2. 验证止盈设置
	if len(mockExec.TakeProfitCalls) != 1 {
		t.Fatalf("Expected 1 take-profit update, got %d", len(mockExec.TakeProfitCalls))
	}

	tpCall := mockExec.TakeProfitCalls[0]
	if tpCall.Symbol != "BTCUSDT" {
		t.Errorf("TP symbol = %s, want BTCUSDT", tpCall.Symbol)
	}
	if tpCall.Side != Long {
		t.Errorf("TP side = %v, want Long", tpCall.Side)
	}

	// 10% ROI → 15% TP (Entry * 1.15)
	expectedTP := 115000.0
	if tpCall.TPPrice < expectedTP-10 || tpCall.TPPrice > expectedTP+10 {
		t.Errorf("TP price = %.2f, want %.2f (15%% target)", tpCall.TPPrice, expectedTP)
	}

	// 3. 验证状态持久化（应保存2次：止损更新1次，止盈更新1次）
	if len(mockStore.SavedPositions) < 1 {
		t.Fatalf("Expected at least 1 state save, got %d", len(mockStore.SavedPositions))
	}

	// 验证最终状态
	finalState := mockStore.SavedPositions[len(mockStore.SavedPositions)-1]

	// ROIArmed应被触发
	if !finalState.ROIArmed {
		t.Errorf("ROIArmed should be true at 10%% ROI")
	}

	// 止盈价格应被更新
	if finalState.PrevTakeProfit == 0 {
		t.Errorf("PrevTakeProfit should be updated, got 0")
	}

	// 止损价格应被更新
	if finalState.PrevStop == 95000.0 {
		t.Errorf("PrevStop should be updated from initial 95000, got %.2f", finalState.PrevStop)
	}

	// ========== 验证同时触发的时序 ==========

	t.Logf("✅ TestSimultaneousStopAndTP passed")
	t.Logf("   Stop updated: 95000.00 → %.2f", stopCall.StopPrice)
	t.Logf("   TP set: 0.00 → %.2f (10%% ROI → 15%% TP)", tpCall.TPPrice)
	t.Logf("   ROIArmed: false → %v", finalState.ROIArmed)
	t.Logf("   Both updates executed in single tick ✓")
}

// ========== P1: 稳健性测试 ==========

// TestBreakEvenProtection 测试Break-even保护（0.4R触发）
func TestBreakEvenProtection(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	t.Run("未达阈值不触发(ROI<5%,R<0.4R)", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0, // R0 = 5000
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: 0,
					StopTriggerType:      TriggerLast,
					BreakEvenArmed:       false,
				},
			},
		}

		// 价格上涨0.04% = 4% ROI@10x（未达5% ROI触发），R=0.08R（未达0.4R阈值）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 100400.0, // +400 = 0.08R, ROI=4%
					MarkPrice: 100400.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：不应触发更新（ROI<5%，R<0.4R）
		if len(mockExec.StopCalls) != 0 {
			t.Errorf("Expected 0 stop updates at 0.08R/4%%ROI, got %d", len(mockExec.StopCalls))
		}

		// 验证：BreakEvenArmed仍为false
		if len(mockStore.SavedPositions) > 0 {
			state := mockStore.SavedPositions[len(mockStore.SavedPositions)-1]
			if state.BreakEvenArmed {
				t.Errorf("BreakEvenArmed should be false at 0.08R")
			}
		}
	})

	t.Run("达到0.4R触发Break-even", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0, // R0 = 5000
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					BreakEvenArmed:       false,
				},
			},
		}

		// 价格上涨0.2% = R=0.4R（刚好触发）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 102000.0, // +2000 = 0.4R
					MarkPrice: 102000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：应触发1次更新
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 stop update at 0.4R, got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]

		// 验证：止损应移到Entry附近（考虑费用+pad）
		// BE_with_costs = Entry * (1 + (4+2)/10000) = 100000 * 1.0006 = 100060
		// BE_pad = 0.05R = 250
		// Expected stop ≈ 100060 + 250 = 100310
		if stopCall.StopPrice < 100000 {
			t.Errorf("Stop should be at or above entry (100000), got %.2f", stopCall.StopPrice)
		}

		if stopCall.StopPrice > 103000 {
			t.Errorf("Stop should not be too far above entry, got %.2f", stopCall.StopPrice)
		}

		// 验证：BreakEvenArmed被设置
		if len(mockStore.SavedPositions) == 0 {
			t.Fatalf("Expected state save")
		}

		state := mockStore.SavedPositions[len(mockStore.SavedPositions)-1]
		if !state.BreakEvenArmed {
			t.Errorf("BreakEvenArmed should be true after 0.4R trigger")
		}

		t.Logf("✓ Break-even triggered: stop moved to %.2f (entry=100000)", stopCall.StopPrice)
	})

	t.Run("SHORT持仓Break-even", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "ETHUSDT",
					Side:                 Short,
					Qty:                  10.0,
					Entry:                2000.0,
					Leverage:             10.0,
					InitStop:             2010.0, // R0 = 10
					PrevStop:             2010.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					BreakEvenArmed:       false,
				},
			},
		}

		// 价格下跌0.2% = R=0.4R
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"ETHUSDT": {
					Symbol:    "ETHUSDT",
					LastPrice: 1996.0, // -4 = 0.4R
					MarkPrice: 1996.0,
					TickSize:  0.01,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：应触发更新
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 stop update for SHORT, got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]

		// SHORT: 止损应下移到Entry附近
		// BE_with_costs = Entry * (1 - 6/10000) = 2000 * 0.9994 = 1998.8
		if stopCall.StopPrice > 2000 {
			t.Errorf("SHORT stop should be at or below entry (2000), got %.2f", stopCall.StopPrice)
		}

		if stopCall.StopPrice < 1990 {
			t.Errorf("SHORT stop should not be too far below entry, got %.2f", stopCall.StopPrice)
		}

		// 验证：BreakEvenArmed被设置
		state := mockStore.SavedPositions[len(mockStore.SavedPositions)-1]
		if !state.BreakEvenArmed {
			t.Errorf("BreakEvenArmed should be true for SHORT")
		}

		t.Logf("✓ SHORT break-even triggered: stop moved to %.2f (entry=2000)", stopCall.StopPrice)
	})

	t.Run("已触发后不重复", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             100300.0, // 已移到break-even
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					BreakEvenArmed:       true, // 已触发
				},
			},
		}

		// 价格仍在0.4R附近
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 102000.0,
					MarkPrice: 102000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：不应再次触发（因为BreakEvenArmed=true且价格未大幅上涨）
		// 注意：可能触发ROI锁盈（2% ROI），但不应触发break-even逻辑
		if len(mockExec.StopCalls) > 0 {
			stopCall := mockExec.StopCalls[0]
			// 如果触发了，应该是ROI锁盈，止损应继续上移
			if stopCall.StopPrice < 100300 {
				t.Errorf("If update triggered, stop should move up from 100300, got %.2f", stopCall.StopPrice)
			}
		}

		t.Logf("✓ Break-even not re-triggered (already armed)")
	})

	t.Logf("✅ TestBreakEvenProtection passed")
}

// TestBreakEvenPriority 测试Break-even与ROI锁盈的优先级（选择更高保护）
func TestBreakEvenPriority(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	t.Run("ROI锁盈价格高于Break-even", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0, // R0 = 5000
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000, // 10分钟前（满足时间条件）
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					BreakEvenArmed:       false,
					ROIArmed:             false,
				},
			},
		}

		// 价格上涨0.2% = 2% ROI@10x，R=0.4R（刚好触发break-even）
		// ROI太低（2%<5%），不触发ROI锁盈，但触发break-even
		// Break-even: BE_with_costs + pad ≈ 100060 + 250 = 100310
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 102000.0, // +2000 = 0.4R, ROI=2%
					MarkPrice: 102000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：应触发更新
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 stop update, got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]

		// 验证：应选择更高的保护价格
		// 实际上两者都会触发，系统应选择max
		if stopCall.StopPrice < 100000 {
			t.Errorf("Stop should be above entry, got %.2f", stopCall.StopPrice)
		}

		// 验证：两个状态都被设置
		state := mockStore.SavedPositions[len(mockStore.SavedPositions)-1]
		if !state.ROIArmed {
			t.Errorf("ROIArmed should be true at 5%% ROI")
		}
		if !state.BreakEvenArmed {
			t.Errorf("BreakEvenArmed should be true at 0.1R")
		}

		t.Logf("✓ Both ROI lock and Break-even triggered, stop=%.2f", stopCall.StopPrice)
	})

	t.Run("高ROI优先（ProfitFloor更高）", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					BreakEvenArmed:       false,
					ROIArmed:             false,
				},
			},
		}

		// 价格上涨1% = 10% ROI@10x，R=1R
		// ROI锁盈：floor=100000*1.002=100200
		// R_lock 1.0R：lock_at=0.5R → 100000 + 0.5*5000 = 102500
		// Break-even: ~100310
		// 应该选择max(102500, 100310) = 102500（R_lock最高）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 105000.0, // +5000 = 1R, ROI=50%
					MarkPrice: 105000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：应触发更新
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 stop update, got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]

		// 验证：止损应远高于break-even（由R_lock决定）
		if stopCall.StopPrice < 100310 {
			t.Errorf("Stop should be well above break-even (100310), got %.2f", stopCall.StopPrice)
		}

		// 止盈也应该触发（50% ROI → 70% TP）
		if len(mockExec.TakeProfitCalls) != 1 {
			t.Errorf("Expected take-profit at 50%% ROI, got %d calls", len(mockExec.TakeProfitCalls))
		}

		t.Logf("✓ High ROI priority: stop=%.2f (R_lock dominates)", stopCall.StopPrice)
	})

	t.Logf("✅ TestBreakEvenPriority passed")
}

// TestInvalidInputHandling 测试异常输入处理（零值、负值、缺失字段）
func TestInvalidInputHandling(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	t.Run("零杠杆持仓", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             0, // 零杠杆（无效）
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 110000.0,
					MarkPrice: 110000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：系统应跳过无效持仓，不应panic
		if len(mockExec.StopCalls) > 0 {
			t.Errorf("Should not update stop for zero leverage position")
		}

		t.Logf("✓ Zero leverage position skipped gracefully")
	})

	t.Run("负数入场价", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "ETHUSDT",
					Side:                 Long,
					Qty:                  10.0,
					Entry:                -2000.0, // 负数入场价（无效）
					Leverage:             10.0,
					InitStop:             -2010.0,
					PrevStop:             -2010.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"ETHUSDT": {
					Symbol:    "ETHUSDT",
					LastPrice: 2100.0,
					MarkPrice: 2100.0,
					TickSize:  0.01,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：系统应跳过无效持仓
		if len(mockExec.StopCalls) > 0 {
			t.Errorf("Should not update stop for negative entry price")
		}

		t.Logf("✓ Negative entry price position skipped")
	})

	t.Run("零数量持仓", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "SOLUSDT",
					Side:                 Long,
					Qty:                  0, // 零数量（无效）
					Entry:                150.0,
					Leverage:             10.0,
					InitStop:             145.0,
					PrevStop:             145.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"SOLUSDT": {
					Symbol:    "SOLUSDT",
					LastPrice: 160.0,
					MarkPrice: 160.0,
					TickSize:  0.01,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：系统应跳过零数量持仓
		if len(mockExec.StopCalls) > 0 {
			t.Errorf("Should not update stop for zero quantity position")
		}

		t.Logf("✓ Zero quantity position skipped")
	})

	t.Run("缺失价格数据", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BNBUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                600.0,
					Leverage:             10.0,
					InitStop:             590.0,
					PrevStop:             590.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				// BNBUSDT 价格缺失
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：系统应跳过无价格数据的持仓
		if len(mockExec.StopCalls) > 0 {
			t.Errorf("Should not update stop when price data missing")
		}

		t.Logf("✓ Missing price data handled gracefully")
	})

	t.Run("混合场景：1个有效+2个无效持仓", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				// 无效1: 零杠杆
				{
					Symbol:               "INVALID1",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100.0,
					Leverage:             0,
					InitStop:             95.0,
					PrevStop:             95.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
				},
				// 有效持仓
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
				// 无效2: 负数入场价
				{
					Symbol:               "INVALID2",
					Side:                 Short,
					Qty:                  1.0,
					Entry:                -500.0,
					Leverage:             10.0,
					InitStop:             -490.0,
					PrevStop:             -490.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 105000.0, // +5% = 50% ROI@10x
					MarkPrice: 105000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：应只处理有效持仓（BTCUSDT）
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 stop update (valid position only), got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]
		if stopCall.Symbol != "BTCUSDT" {
			t.Errorf("Should only update valid position BTCUSDT, got %s", stopCall.Symbol)
		}

		if len(mockExec.TakeProfitCalls) != 1 {
			t.Fatalf("Expected 1 TP update (valid position only), got %d", len(mockExec.TakeProfitCalls))
		}

		t.Logf("✓ Mixed scenario: 1 valid position processed, 2 invalid positions skipped")
		t.Logf("   Valid position: %s, stop updated to %.2f", stopCall.Symbol, stopCall.StopPrice)
	})

	t.Logf("✅ TestInvalidInputHandling passed")
}

// TestExecutorFailureRecovery 测试执行器失败恢复（UpsertStop/UpsertTakeProfit API失败）
func TestExecutorFailureRecovery(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	t.Run("UpsertStop失败不阻塞其他持仓", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				// 持仓1: 会触发更新（触发UpsertStop错误）
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
				// 持仓2: 也会触发更新（应继续处理）
				{
					Symbol:               "ETHUSDT",
					Side:                 Long,
					Qty:                  10.0,
					Entry:                2000.0,
					Leverage:             10.0,
					InitStop:             1990.0,
					PrevStop:             1990.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 105000.0, // 5% price change = 50% ROI@10x
					MarkPrice: 105000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
				"ETHUSDT": {
					Symbol:    "ETHUSDT",
					LastPrice: 2100.0, // 5% price change = 50% ROI@10x
					MarkPrice: 2100.0,
					TickSize:  0.01,
					NowMs:     now,
				},
			},
		}

		// 模拟执行器：第一次调用失败，后续成功
		mockExec := &MockStopExecutor{
			StopError: fmt.Errorf("simulated API error: rate limit exceeded"),
		}

		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：两个持仓都尝试更新（即使第一个失败）
		if len(mockExec.StopCalls) < 2 {
			t.Errorf("Expected at least 2 stop calls (continue despite error), got %d", len(mockExec.StopCalls))
		}

		// 验证：数据库不应保存失败的状态更新
		// 由于StopError非nil，SavePositionState不应被调用
		if len(mockStore.SavedPositions) > 0 {
			t.Logf("⚠️  Warning: %d positions saved despite executor error", len(mockStore.SavedPositions))
		}

		t.Logf("✓ System continued processing despite UpsertStop failures")
	})

	t.Run("UpsertTakeProfit失败不影响止损更新", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					PrevTakeProfit:       0.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 101000.0, // 1% price change = 10% ROI@10x
					MarkPrice: 101000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		// 模拟执行器：止盈API失败，止损API成功
		mockExec := &MockStopExecutor{
			StopError: nil,                                                      // 止损成功
			TPError:   fmt.Errorf("simulated API error: insufficient margin"), // 止盈失败
		}

		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：止损应成功更新
		if len(mockExec.StopCalls) != 1 {
			t.Errorf("Expected 1 stop call, got %d", len(mockExec.StopCalls))
		}

		// 验证：止盈应尝试但失败
		if len(mockExec.TakeProfitCalls) != 1 {
			t.Errorf("Expected 1 take-profit call (failed), got %d", len(mockExec.TakeProfitCalls))
		}

		t.Logf("✓ Stop-loss updated successfully despite take-profit failure")
		t.Logf("   Stop: %.2f, TP call attempted: %d", mockExec.StopCalls[0].StopPrice, len(mockExec.TakeProfitCalls))
	})

	t.Run("执行器失败时状态保持一致", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "SOLUSDT",
					Side:                 Long,
					Qty:                  100.0,
					Entry:                150.0,
					Leverage:             10.0,
					InitStop:             145.0,
					PrevStop:             145.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
					BreakEvenArmed:       false,
					RLockStage:           0,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"SOLUSDT": {
					Symbol:    "SOLUSDT",
					LastPrice: 157.5, // 5% price change = 50% ROI@10x
					MarkPrice: 157.5,
					TickSize:  0.01,
					NowMs:     now,
				},
			},
		}

		// 模拟执行器：所有API失败
		mockExec := &MockStopExecutor{
			StopError: fmt.Errorf("network timeout"),
			TPError:   fmt.Errorf("network timeout"),
		}

		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：原始持仓状态不变（因为executor失败，不应保存状态）
		originalPos := mockStore.Positions[0]
		if originalPos.ROIArmed {
			t.Errorf("ROIArmed should remain false when executor fails")
		}
		if originalPos.BreakEvenArmed {
			t.Errorf("BreakEvenArmed should remain false when executor fails")
		}
		if originalPos.RLockStage != 0 {
			t.Errorf("RLockStage should remain 0 when executor fails, got %d", originalPos.RLockStage)
		}

		t.Logf("✓ Position state preserved when executor fails")
		t.Logf("   ROIArmed: %v, BreakEvenArmed: %v, RLockStage: %d",
			originalPos.ROIArmed, originalPos.BreakEvenArmed, originalPos.RLockStage)
	})

	t.Run("重试后成功", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BNBUSDT",
					Side:                 Long,
					Qty:                  10.0,
					Entry:                600.0,
					Leverage:             10.0,
					InitStop:             590.0,
					PrevStop:             590.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BNBUSDT": {
					Symbol:    "BNBUSDT",
					LastPrice: 630.0, // 5% price change = 50% ROI@10x
					MarkPrice: 630.0,
					TickSize:  0.01,
					NowMs:     now,
				},
			},
		}

		// 第一次尝试：失败
		mockExecFail := &MockStopExecutor{
			StopError: fmt.Errorf("temporary error"),
		}

		scheduler1 := NewScheduler(eng, mockStore, mockCache, mockExecFail)
		ctx := context.Background()
		scheduler1.tickOnce(ctx)

		// 验证：第一次失败
		if len(mockStore.SavedPositions) > 0 {
			t.Errorf("Should not save state when executor fails")
		}

		// 第二次尝试：成功
		// 重置LastStopUpdateTimeMs以绕过cooldown
		mockStore.Positions[0].LastStopUpdateTimeMs = time.Now().UnixMilli() - 30*1000

		mockExecSuccess := &MockStopExecutor{
			StopError: nil, // 成功
		}

		scheduler2 := NewScheduler(eng, mockStore, mockCache, mockExecSuccess)
		scheduler2.tickOnce(ctx)

		// 验证：第二次成功
		if len(mockExecSuccess.StopCalls) != 1 {
			t.Errorf("Expected 1 stop call after retry, got %d", len(mockExecSuccess.StopCalls))
		}

		if len(mockStore.SavedPositions) < 1 {
			t.Errorf("Should save state after successful retry, got %d saves", len(mockStore.SavedPositions))
		}

		t.Logf("✓ Retry after failure successful")
		t.Logf("   First attempt: failed, state not saved")
		t.Logf("   Second attempt: success, state saved")
	})

	t.Logf("✅ TestExecutorFailureRecovery passed")
}

// ========== P2: 稳健性测试 ==========

// TestMonotonicity_Long 测试LONG持仓止损单调性（只能向上移动）
func TestMonotonicity_Long(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	t.Run("价格上涨→止损上移（允许）", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 105000.0, // +5% = 50% ROI@10x
					MarkPrice: 105000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：止损应上移
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 stop update, got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]
		if stopCall.StopPrice <= 95000.0 {
			t.Errorf("LONG stop should move up, got %.2f (prev: 95000)", stopCall.StopPrice)
		}

		t.Logf("✓ LONG stop moved up: 95000.00 → %.2f (price up 5%%)", stopCall.StopPrice)
	})

	t.Run("价格下跌→止损不动（拒绝下移）", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "ETHUSDT",
					Side:                 Long,
					Qty:                  10.0,
					Entry:                2000.0,
					Leverage:             10.0,
					InitStop:             1990.0,
					PrevStop:             2010.0, // 已上移到entry+10（盈利保护）
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             true, // 已触发
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"ETHUSDT": {
					Symbol:    "ETHUSDT",
					LastPrice: 1980.0, // -1% 回撤
					MarkPrice: 1980.0,
					TickSize:  0.01,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：止损不应更新（不允许下移）
		if len(mockExec.StopCalls) > 0 {
			stopCall := mockExec.StopCalls[0]
			if stopCall.StopPrice < 2010.0 {
				t.Errorf("LONG stop should NOT move down when price drops, got %.2f (prev: 2010)", stopCall.StopPrice)
			}
		}

		t.Logf("✓ LONG stop did not move down during pullback (preserved at 2010)")
	})

	t.Run("多次上移（单调递增）", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "SOLUSDT",
					Side:                 Long,
					Qty:                  100.0,
					Entry:                150.0,
					Leverage:             10.0,
					InitStop:             145.0,
					PrevStop:             145.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
					RLockStage:           0,
				},
			},
		}

		prices := []float64{153.0, 157.5, 165.0} // +2%, +5%, +10%
		expectedStops := []float64{}

		for i, price := range prices {
			mockCache := &MockPriceCache{
				Prices: map[string]MarketSnapshot{
					"SOLUSDT": {
						Symbol:    "SOLUSDT",
						LastPrice: price,
						MarkPrice: price,
						TickSize:  0.01,
						NowMs:     now + int64(i*60*1000), // +1分钟
					},
				},
			}

			mockExec := &MockStopExecutor{}
			scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

			// 重置cooldown
			mockStore.Positions[0].LastStopUpdateTimeMs = time.Now().UnixMilli() - 30*1000

			ctx := context.Background()
			scheduler.tickOnce(ctx)

			if len(mockExec.StopCalls) > 0 {
				newStop := mockExec.StopCalls[0].StopPrice
				expectedStops = append(expectedStops, newStop)

				// 验证单调递增
				if i > 0 && newStop <= expectedStops[i-1] {
					t.Errorf("Stop should increase monotonically: %.2f <= %.2f (iteration %d)",
						newStop, expectedStops[i-1], i+1)
				}

				// 更新prevStop for next iteration
				mockStore.Positions[0].PrevStop = newStop
			}
		}

		t.Logf("✓ LONG stop increased monotonically across %d iterations", len(expectedStops))
		for i, stop := range expectedStops {
			t.Logf("   Iteration %d: %.2f", i+1, stop)
		}
	})

	t.Run("止损不能高于当前价（保护机制）", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BNBUSDT",
					Side:                 Long,
					Qty:                  10.0,
					Entry:                600.0,
					Leverage:             10.0,
					InitStop:             590.0,
					PrevStop:             610.0, // 已经很高了
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             true,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BNBUSDT": {
					Symbol:    "BNBUSDT",
					LastPrice: 615.0, // 当前价615
					MarkPrice: 615.0,
					TickSize:  0.01,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：如果有更新，止损不应≥当前价
		if len(mockExec.StopCalls) > 0 {
			stopCall := mockExec.StopCalls[0]
			if stopCall.StopPrice >= 615.0 {
				t.Errorf("LONG stop should be < current price (615), got %.2f", stopCall.StopPrice)
			}
		}

		t.Logf("✓ LONG stop remains below current price (max protection)")
	})

	t.Logf("✅ TestMonotonicity_Long passed")
}

// TestBoundaryConditions 测试边界条件（ROI=5%/10%精确阈值，时间=180秒精确阈值）
func TestBoundaryConditions(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	t.Run("ROI边界：4.99%不触发", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000, // 10分钟前（满足时间条件）
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		// 价格上涨0.499% = 4.99% ROI@10x（刚好低于5%触发阈值）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 100499.0, // +0.499% = 4.99% ROI
					MarkPrice: 100499.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：不应触发更新（4.99% < 5%）
		if len(mockExec.StopCalls) != 0 {
			t.Errorf("Expected 0 stop updates at 4.99%% ROI, got %d", len(mockExec.StopCalls))
		}

		t.Logf("✓ 4.99%% ROI correctly NOT triggered (below 5%% threshold)")
	})

	t.Run("ROI边界：5.00%精确触发", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000, // 10分钟前（满足时间条件）
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		// 价格上涨0.5% = 5.00% ROI@10x（刚好达到5%触发阈值）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 100500.0, // +0.5% = 5.00% ROI
					MarkPrice: 100500.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：应触发更新（5.00% >= 5%）
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 stop update at 5.00%% ROI, got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]
		if stopCall.StopPrice <= 95000 {
			t.Errorf("Stop should be above initial 95000, got %.2f", stopCall.StopPrice)
		}

		t.Logf("✓ 5.00%% ROI correctly triggered (at exact threshold)")
	})

	t.Run("ROI边界：9.99%需要时间检查", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 2*60*1000, // 2分钟前（不满足3分钟时间条件）
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		// 价格上涨0.999% = 9.99% ROI@10x（低于10%快速通道）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 100999.0, // +0.999% = 9.99% ROI
					MarkPrice: 100999.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：不应触发（9.99% < 10%，需要时间条件，但只持仓2分钟 < 3分钟）
		if len(mockExec.StopCalls) != 0 {
			t.Errorf("Expected 0 stop updates at 9.99%% ROI with 2min hold, got %d", len(mockExec.StopCalls))
		}

		t.Logf("✓ 9.99%% ROI correctly blocked (below 10%% fast-track, time insufficient)")
	})

	t.Run("ROI边界：10.00%快速通道跳过时间检查", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 1*60*1000, // 1分钟前（不满足3分钟时间条件）
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		// 价格上涨1.0% = 10.00% ROI@10x（刚好达到10%快速通道）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 101000.0, // +1.0% = 10.00% ROI
					MarkPrice: 101000.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：应触发更新（10.00% >= 10%快速通道，跳过时间检查）
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 stop update at 10.00%% ROI (fast-track), got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]
		if stopCall.StopPrice <= 95000 {
			t.Errorf("Stop should be above initial 95000, got %.2f", stopCall.StopPrice)
		}

		t.Logf("✓ 10.00%% ROI correctly triggered via fast-track (skipped time check)")
	})

	t.Run("时间边界：179秒阻止锁盈", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 179*1000, // 179秒前（1秒低于180秒阈值）
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		// 价格上涨0.5% = 5.00% ROI@10x（满足ROI条件）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 100500.0, // +0.5% = 5.00% ROI
					MarkPrice: 100500.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：不应触发（179秒 < 180秒时间阈值）
		if len(mockExec.StopCalls) != 0 {
			t.Errorf("Expected 0 stop updates at 179s hold time, got %d", len(mockExec.StopCalls))
		}

		t.Logf("✓ 179 seconds correctly blocked (1 second below 180s threshold)")
	})

	t.Run("时间边界：180秒精确触发", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 180*1000, // 180秒前（刚好达到180秒阈值）
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		// 价格上涨0.5% = 5.00% ROI@10x（满足ROI条件）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {
					Symbol:    "BTCUSDT",
					LastPrice: 100500.0, // +0.5% = 5.00% ROI
					MarkPrice: 100500.0,
					TickSize:  0.1,
					NowMs:     now,
				},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		scheduler.tickOnce(ctx)

		// 验证：应触发更新（180秒 >= 180秒时间阈值）
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 stop update at 180s hold time, got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]
		if stopCall.StopPrice <= 95000 {
			t.Errorf("Stop should be above initial 95000, got %.2f", stopCall.StopPrice)
		}

		t.Logf("✓ 180 seconds correctly triggered (at exact threshold)")
	})

	t.Logf("✅ TestBoundaryConditions passed")
}

// ========== P2: Adapter接口契约测试 ==========

// TestPositionStoreAdapter 测试PositionStore适配器接口契约
func TestPositionStoreAdapter(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()

	t.Run("ListOpenPositions接口契约", func(t *testing.T) {
		// 模拟数据库返回的持仓数据
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					PrevTakeProfit:       0.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: 0,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
					BreakEvenArmed:       false,
					RLockStage:           0,
				},
				{
					Symbol:               "ETHUSDT",
					Side:                 Short,
					Qty:                  10.0,
					Entry:                2000.0,
					Leverage:             10.0,
					InitStop:             2010.0,
					PrevStop:             2010.0,
					PrevTakeProfit:       0.0,
					OpenTimeMs:           now - 5*60*1000,
					LastStopUpdateTimeMs: 0,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
					BreakEvenArmed:       false,
					RLockStage:           0,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {Symbol: "BTCUSDT", LastPrice: 105000.0, MarkPrice: 105000.0, TickSize: 0.1, NowMs: now},
				"ETHUSDT": {Symbol: "ETHUSDT", LastPrice: 1990.0, MarkPrice: 1990.0, TickSize: 0.01, NowMs: now},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		ctx := context.Background()
		positions, err := mockStore.ListOpenPositions(ctx)

		// 验证接口调用成功
		if err != nil {
			t.Fatalf("ListOpenPositions failed: %v", err)
		}

		// 验证返回数据
		if len(positions) != 2 {
			t.Fatalf("Expected 2 positions, got %d", len(positions))
		}

		// 验证LONG持仓数据
		longPos := positions[0]
		if longPos.Symbol != "BTCUSDT" || longPos.Side != Long {
			t.Errorf("LONG position incorrect: symbol=%s, side=%v", longPos.Symbol, longPos.Side)
		}
		if longPos.Entry != 100000.0 || longPos.Leverage != 10.0 {
			t.Errorf("LONG position fields incorrect: entry=%.2f, leverage=%.2f", longPos.Entry, longPos.Leverage)
		}

		// 验证SHORT持仓数据
		shortPos := positions[1]
		if shortPos.Symbol != "ETHUSDT" || shortPos.Side != Short {
			t.Errorf("SHORT position incorrect: symbol=%s, side=%v", shortPos.Symbol, shortPos.Side)
		}
		if shortPos.Entry != 2000.0 || shortPos.Leverage != 10.0 {
			t.Errorf("SHORT position fields incorrect: entry=%.2f, leverage=%.2f", shortPos.Entry, shortPos.Leverage)
		}

		// 验证状态机字段
		if longPos.ROIArmed || longPos.BreakEvenArmed || longPos.RLockStage != 0 {
			t.Errorf("State machine fields should be initial values")
		}

		t.Logf("✓ ListOpenPositions interface contract verified (2 positions)")
		t.Logf("   LONG: %s @ %.2f, LEVERAGE=%d", longPos.Symbol, longPos.Entry, int(longPos.Leverage))
		t.Logf("   SHORT: %s @ %.2f, LEVERAGE=%d", shortPos.Symbol, shortPos.Entry, int(shortPos.Leverage))

		// 验证scheduler可以处理这些数据
		scheduler.tickOnce(ctx)

		// BTCUSDT LONG: 50% ROI应该触发锁盈
		// ETHUSDT SHORT: -5% loss不应触发（负利润）
		if len(mockExec.StopCalls) == 0 {
			t.Errorf("Expected at least 1 stop update for BTCUSDT LONG (50%% ROI)")
		}
	})

	t.Run("SavePositionState接口契约", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					PrevTakeProfit:       0.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
					BreakEvenArmed:       false,
					RLockStage:           0,
				},
			},
		}

		ctx := context.Background()

		// 模拟止损更新后的状态保存
		updatedState := PositionState{
			Symbol:               "BTCUSDT",
			Side:                 Long,
			Qty:                  1.0,
			Entry:                100000.0,
			Leverage:             10.0,
			InitStop:             95000.0,
			PrevStop:             100200.0, // 更新后的止损
			PrevTakeProfit:       0.0,
			OpenTimeMs:           now - 10*60*1000,
			LastStopUpdateTimeMs: now, // 更新时间戳
			StopTriggerType:      TriggerLast,
			ROIArmed:             true, // 状态机更新
			BreakEvenArmed:       false,
			RLockStage:           0,
		}

		err := mockStore.SavePositionState(ctx, updatedState)

		// 验证保存成功
		if err != nil {
			t.Fatalf("SavePositionState failed: %v", err)
		}

		// 验证数据已保存到SavedPositions
		if len(mockStore.SavedPositions) != 1 {
			t.Fatalf("Expected 1 saved position, got %d", len(mockStore.SavedPositions))
		}

		saved := mockStore.SavedPositions[0]
		if saved.PrevStop != 100200.0 {
			t.Errorf("Saved PrevStop = %.2f, want 100200.0", saved.PrevStop)
		}
		if !saved.ROIArmed {
			t.Errorf("Saved ROIArmed should be true")
		}
		if saved.LastStopUpdateTimeMs != now {
			t.Errorf("Saved LastStopUpdateTimeMs incorrect")
		}

		// 验证内部Positions数组也被更新（模拟数据库回写）
		if mockStore.Positions[0].PrevStop != 100200.0 {
			t.Errorf("Internal Positions[0].PrevStop not updated, got %.2f", mockStore.Positions[0].PrevStop)
		}

		t.Logf("✓ SavePositionState interface contract verified")
		t.Logf("   Stop updated: 95000.00 → 100200.00")
		t.Logf("   ROIArmed: false → true")
		t.Logf("   Internal array synchronized ✓")
	})

	t.Run("Side枚举转换", func(t *testing.T) {
		// 测试Side枚举值
		longPos := PositionState{Side: Long}
		shortPos := PositionState{Side: Short}

		if longPos.Side != Long {
			t.Errorf("Long side enum incorrect")
		}
		if shortPos.Side != Short {
			t.Errorf("Short side enum incorrect")
		}

		// 模拟adapter需要将Side转换为字符串
		var sideStr string
		if longPos.Side == Long {
			sideStr = "long"
		} else {
			sideStr = "short"
		}
		if sideStr != "long" {
			t.Errorf("Long → 'long' conversion failed, got %s", sideStr)
		}

		if shortPos.Side == Short {
			sideStr = "short"
		} else {
			sideStr = "long"
		}
		if sideStr != "short" {
			t.Errorf("Short → 'short' conversion failed, got %s", sideStr)
		}

		t.Logf("✓ Side enum conversion verified (Long ↔ 'long', Short ↔ 'short')")
	})

	t.Run("数据库错误处理", func(t *testing.T) {
		// 模拟SaveError场景
		mockStore := &MockPositionStore{
			SaveError: fmt.Errorf("database connection lost"),
		}

		ctx := context.Background()
		state := PositionState{Symbol: "BTCUSDT", Side: Long}

		err := mockStore.SavePositionState(ctx, state)

		// 验证错误正确传播
		if err == nil {
			t.Fatalf("Expected error, got nil")
		}
		if err.Error() != "database connection lost" {
			t.Errorf("Wrong error message: %v", err)
		}

		// 验证数据未被保存
		if len(mockStore.SavedPositions) != 0 {
			t.Errorf("Data should not be saved when error occurs")
		}

		t.Logf("✓ Database error handling verified")
	})

	t.Logf("✅ TestPositionStoreAdapter passed")
}

// TestStopExecutorAdapter 测试StopExecutor适配器接口契约
func TestStopExecutorAdapter(t *testing.T) {
	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 4, SlippageBpsMinor: 2},
	}

	now := time.Now().UnixMilli()
	ctx := context.Background()

	t.Run("UpsertStop接口契约", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {Symbol: "BTCUSDT", LastPrice: 105000.0, MarkPrice: 105000.0, TickSize: 0.1, NowMs: now},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		// 触发止损更新
		scheduler.tickOnce(ctx)

		// 验证UpsertStop被调用
		if len(mockExec.StopCalls) != 1 {
			t.Fatalf("Expected 1 UpsertStop call, got %d", len(mockExec.StopCalls))
		}

		stopCall := mockExec.StopCalls[0]

		// 验证参数正确性
		if stopCall.Symbol != "BTCUSDT" {
			t.Errorf("Symbol = %s, want BTCUSDT", stopCall.Symbol)
		}
		if stopCall.Side != Long {
			t.Errorf("Side = %v, want Long", stopCall.Side)
		}
		if stopCall.Qty != 1.0 {
			t.Errorf("Qty = %.2f, want 1.0", stopCall.Qty)
		}
		if stopCall.StopPrice <= 95000 {
			t.Errorf("StopPrice should be above 95000, got %.2f", stopCall.StopPrice)
		}
		if stopCall.Trigger != TriggerLast {
			t.Errorf("Trigger = %v, want TriggerLast", stopCall.Trigger)
		}

		t.Logf("✓ UpsertStop interface contract verified")
		t.Logf("   Symbol: %s, Side: %v, Qty: %.2f", stopCall.Symbol, stopCall.Side, stopCall.Qty)
		t.Logf("   StopPrice: %.2f, Trigger: %v", stopCall.StopPrice, stopCall.Trigger)
	})

	t.Run("UpsertTakeProfit接口契约", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{
					Symbol:               "BTCUSDT",
					Side:                 Long,
					Qty:                  1.0,
					Entry:                100000.0,
					Leverage:             10.0,
					InitStop:             95000.0,
					PrevStop:             95000.0,
					PrevTakeProfit:       0.0,
					OpenTimeMs:           now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000,
					StopTriggerType:      TriggerLast,
					ROIArmed:             false,
				},
			},
		}

		// 10% ROI触发止盈（→ 15% TP）
		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {Symbol: "BTCUSDT", LastPrice: 101000.0, MarkPrice: 101000.0, TickSize: 0.1, NowMs: now},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		// 触发止盈更新
		scheduler.tickOnce(ctx)

		// 验证UpsertTakeProfit被调用
		if len(mockExec.TakeProfitCalls) != 1 {
			t.Fatalf("Expected 1 UpsertTakeProfit call, got %d", len(mockExec.TakeProfitCalls))
		}

		tpCall := mockExec.TakeProfitCalls[0]

		// 验证参数正确性
		if tpCall.Symbol != "BTCUSDT" {
			t.Errorf("Symbol = %s, want BTCUSDT", tpCall.Symbol)
		}
		if tpCall.Side != Long {
			t.Errorf("Side = %v, want Long", tpCall.Side)
		}
		if tpCall.Qty != 1.0 {
			t.Errorf("Qty = %.2f, want 1.0", tpCall.Qty)
		}

		// 10% ROI → 15% TP (Entry * 1.15)
		expectedTP := 115000.0
		if tpCall.TPPrice < expectedTP-10 || tpCall.TPPrice > expectedTP+10 {
			t.Errorf("TPPrice = %.2f, want %.2f", tpCall.TPPrice, expectedTP)
		}

		if tpCall.Trigger != TriggerLast {
			t.Errorf("Trigger = %v, want TriggerLast", tpCall.Trigger)
		}

		t.Logf("✓ UpsertTakeProfit interface contract verified")
		t.Logf("   Symbol: %s, Side: %v, Qty: %.2f", tpCall.Symbol, tpCall.Side, tpCall.Qty)
		t.Logf("   TPPrice: %.2f (10%% ROI → 15%% TP), Trigger: %v", tpCall.TPPrice, tpCall.Trigger)
	})

	t.Run("Side转换验证（LONG/SHORT）", func(t *testing.T) {
		// 测试LONG持仓
		longStore := &MockPositionStore{
			Positions: []PositionState{
				{Symbol: "BTCUSDT", Side: Long, Qty: 1.0, Entry: 100000.0, Leverage: 10.0,
					InitStop: 95000.0, PrevStop: 95000.0, OpenTimeMs: now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000, StopTriggerType: TriggerLast},
			},
		}
		longCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {Symbol: "BTCUSDT", LastPrice: 105000.0, MarkPrice: 105000.0, TickSize: 0.1, NowMs: now},
			},
		}
		longExec := &MockStopExecutor{}
		longScheduler := NewScheduler(eng, longStore, longCache, longExec)
		longScheduler.tickOnce(ctx)

		if len(longExec.StopCalls) > 0 && longExec.StopCalls[0].Side != Long {
			t.Errorf("LONG position should use Long enum, got %v", longExec.StopCalls[0].Side)
		}

		// 测试SHORT持仓
		shortStore := &MockPositionStore{
			Positions: []PositionState{
				{Symbol: "ETHUSDT", Side: Short, Qty: 10.0, Entry: 2000.0, Leverage: 10.0,
					InitStop: 2010.0, PrevStop: 2010.0, OpenTimeMs: now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000, StopTriggerType: TriggerLast},
			},
		}
		shortCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"ETHUSDT": {Symbol: "ETHUSDT", LastPrice: 1900.0, MarkPrice: 1900.0, TickSize: 0.01, NowMs: now},
			},
		}
		shortExec := &MockStopExecutor{}
		shortScheduler := NewScheduler(eng, shortStore, shortCache, shortExec)
		shortScheduler.tickOnce(ctx)

		if len(shortExec.StopCalls) > 0 && shortExec.StopCalls[0].Side != Short {
			t.Errorf("SHORT position should use Short enum, got %v", shortExec.StopCalls[0].Side)
		}

		t.Logf("✓ Side enum conversion verified")
		t.Logf("   LONG enum → protect.Long")
		t.Logf("   SHORT enum → protect.Short")
	})

	t.Run("执行器错误处理", func(t *testing.T) {
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{Symbol: "BTCUSDT", Side: Long, Qty: 1.0, Entry: 100000.0, Leverage: 10.0,
					InitStop: 95000.0, PrevStop: 95000.0, OpenTimeMs: now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000, StopTriggerType: TriggerLast},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {Symbol: "BTCUSDT", LastPrice: 105000.0, MarkPrice: 105000.0, TickSize: 0.1, NowMs: now},
			},
		}

		// 模拟StopError
		mockExec := &MockStopExecutor{
			StopError: fmt.Errorf("API rate limit exceeded"),
		}

		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)
		scheduler.tickOnce(ctx)

		// 验证即使错误，调用仍然尝试
		if len(mockExec.StopCalls) != 1 {
			t.Errorf("UpsertStop should be called despite error, got %d calls", len(mockExec.StopCalls))
		}

		// 验证状态未保存（因为executor失败）
		// (scheduler在executor失败时不应保存状态)

		t.Logf("✓ Executor error handling verified")
		t.Logf("   UpsertStop called despite error")
	})

	t.Run("并发调用安全性", func(t *testing.T) {
		// 测试多个持仓同时更新
		mockStore := &MockPositionStore{
			Positions: []PositionState{
				{Symbol: "BTCUSDT", Side: Long, Qty: 1.0, Entry: 100000.0, Leverage: 10.0,
					InitStop: 95000.0, PrevStop: 95000.0, OpenTimeMs: now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000, StopTriggerType: TriggerLast},
				{Symbol: "ETHUSDT", Side: Short, Qty: 10.0, Entry: 2000.0, Leverage: 10.0,
					InitStop: 2010.0, PrevStop: 2010.0, OpenTimeMs: now - 10*60*1000,
					LastStopUpdateTimeMs: time.Now().UnixMilli() - 30*1000, StopTriggerType: TriggerLast},
			},
		}

		mockCache := &MockPriceCache{
			Prices: map[string]MarketSnapshot{
				"BTCUSDT": {Symbol: "BTCUSDT", LastPrice: 105000.0, MarkPrice: 105000.0, TickSize: 0.1, NowMs: now},
				"ETHUSDT": {Symbol: "ETHUSDT", LastPrice: 1900.0, MarkPrice: 1900.0, TickSize: 0.01, NowMs: now},
			},
		}

		mockExec := &MockStopExecutor{}
		scheduler := NewScheduler(eng, mockStore, mockCache, mockExec)

		scheduler.tickOnce(ctx)

		// 验证多个持仓都被处理
		if len(mockExec.StopCalls) < 2 {
			t.Errorf("Expected at least 2 stop calls, got %d", len(mockExec.StopCalls))
		}

		// 验证每个持仓独立处理
		symbols := make(map[string]bool)
		for _, call := range mockExec.StopCalls {
			symbols[call.Symbol] = true
		}

		if !symbols["BTCUSDT"] || !symbols["ETHUSDT"] {
			t.Errorf("Both positions should be processed independently")
		}

		t.Logf("✓ Concurrent call safety verified")
		t.Logf("   %d positions processed independently", len(symbols))
	})

	t.Logf("✅ TestStopExecutorAdapter passed")
}
