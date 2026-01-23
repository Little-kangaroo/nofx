package protect

import (
	"context"
	"log"
	"testing"
	"time"
)

// TestSchedulerPanicRecovery 测试panic recovery机制
func TestSchedulerPanicRecovery(t *testing.T) {
	log.Printf("🧪 测试panic recovery机制...")

	// 创建一个会触发panic的Store
	panicStore := &panicPositionStore{
		panicCount: 0,
		maxPanics:  2, // 前2次调用会panic
	}

	cfg := DefaultConfig()
	cfg.Scheduler.CheckIntervalSec = 1 // 1秒间隔用于测试

	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 5, SlippageBpsMinor: 1, FundingBps: 0},
	}

	scheduler := NewScheduler(
		eng,
		panicStore,
		&mockPriceCache{},
		&mockStopExecutor{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 启动Fast Loop
	go scheduler.StartFastLoop(ctx)

	// 等待足够长的时间，确保经历了多次panic和恢复
	time.Sleep(4 * time.Second)

	// 检查panic次数
	if panicStore.panicCount < 2 {
		t.Errorf("Expected at least 2 panics, got %d", panicStore.panicCount)
	}

	// 检查是否成功恢复并继续运行
	if panicStore.successCount < 1 {
		t.Errorf("Expected at least 1 successful call after panic recovery, got %d", panicStore.successCount)
	}

	log.Printf("✅ Panic recovery测试通过: %d次panic, %d次成功恢复", panicStore.panicCount, panicStore.successCount)
}

// TestSchedulerDivideByZeroProtection 测试除零保护
func TestSchedulerDivideByZeroProtection(t *testing.T) {
	log.Printf("🧪 测试除零保护机制...")

	// 创建一个返回无效Entry的Store
	invalidStore := &mockPositionStore{
		positions: []PositionState{
			{
				Symbol:   "BTCUSDT",
				Side:     Long,
				Qty:      1.0,
				Entry:    0, // 无效的Entry，会导致除零
				Leverage: 10,
				InitStop: 95000,
				PrevStop: 95000,
			},
		},
	}

	cfg := DefaultConfig()
	eng := &Engine{
		Cfg:  cfg,
		Fees: FeeModel{TakerFeeBps: 5, SlippageBpsMinor: 1, FundingBps: 0},
	}

	priceCache := &mockPriceCache{
		snapshots: map[string]MarketSnapshot{
			"BTCUSDT": {
				Symbol:    "BTCUSDT",
				LastPrice: 101000,
				MarkPrice: 101000,
				TickSize:  0.1,
				NowMs:     time.Now().UnixMilli(),
			},
		},
	}

	scheduler := NewScheduler(
		eng,
		invalidStore,
		priceCache,
		&mockStopExecutor{},
	)

	ctx := context.Background()

	// 执行一次tick，应该跳过无效持仓而不是panic
	scheduler.tickOnce(ctx)

	log.Printf("✅ 除零保护测试通过: 成功跳过无效持仓")
}

// panicPositionStore 会在前N次调用时panic的Store
type panicPositionStore struct {
	panicCount   int
	successCount int
	maxPanics    int
}

func (s *panicPositionStore) ListOpenPositions(ctx context.Context) ([]PositionState, error) {
	if s.panicCount < s.maxPanics {
		s.panicCount++
		log.Printf("💥 模拟panic (第%d次)", s.panicCount)
		panic("simulated panic for testing")
	}
	s.successCount++
	log.Printf("✅ 成功调用 (第%d次)", s.successCount)
	return []PositionState{}, nil
}

func (s *panicPositionStore) SavePositionState(ctx context.Context, pos PositionState) error {
	return nil
}

// mockPositionStore 用于测试的Store
type mockPositionStore struct {
	positions []PositionState
}

func (s *mockPositionStore) ListOpenPositions(ctx context.Context) ([]PositionState, error) {
	return s.positions, nil
}

func (s *mockPositionStore) SavePositionState(ctx context.Context, pos PositionState) error {
	return nil
}

// mockPriceCache 用于测试的PriceCache
type mockPriceCache struct {
	snapshots map[string]MarketSnapshot
}

func (c *mockPriceCache) GetSnapshot(ctx context.Context, symbol string) (MarketSnapshot, bool) {
	if c.snapshots == nil {
		return MarketSnapshot{
			Symbol:    symbol,
			LastPrice: 100000,
			MarkPrice: 100000,
			TickSize:  0.1,
			NowMs:     time.Now().UnixMilli(),
		}, true
	}
	snap, ok := c.snapshots[symbol]
	return snap, ok
}

// mockStopExecutor 用于测试的StopExecutor
type mockStopExecutor struct{}

func (e *mockStopExecutor) UpsertStop(ctx context.Context, symbol string, side Side, qty float64, stopPrice float64, trigger TriggerType) error {
	return nil
}

func (e *mockStopExecutor) UpsertTakeProfit(ctx context.Context, symbol string, side Side, qty float64, tpPrice float64, trigger TriggerType) error {
	return nil
}
