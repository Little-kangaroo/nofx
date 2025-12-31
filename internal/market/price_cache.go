package market

import (
	"context"
	"sync"
	"time"

	"nofx/internal/protect"
)

// snap 内部快照结构
type snap struct {
	last float64
	mark float64
	tick float64
	tsMs int64
}

// PriceCache 价格缓存（线程安全）
// 从WebSocket/REST聚合市场数据，提供给锁盈引擎
type PriceCache struct {
	mu           sync.RWMutex
	m            map[string]snap
	StaleAfterMs int64 // 数据陈旧阈值（毫秒），默认3000ms
}

// NewPriceCache 创建价格缓存
func NewPriceCache() *PriceCache {
	return &PriceCache{
		m:            make(map[string]snap),
		StaleAfterMs: 3000, // 3秒
	}
}

// Update 更新市场数据（由WebSocket/REST调用）
func (c *PriceCache) Update(symbol string, last, mark, tick float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.m[symbol] = snap{
		last: last,
		mark: mark,
		tick: tick,
		tsMs: time.Now().UnixMilli(),
	}
}

// GetSnapshot 获取市场快照（实现protect.PriceCache接口）
// 如果数据陈旧（超过StaleAfterMs），返回false
func (c *PriceCache) GetSnapshot(ctx context.Context, symbol string) (protect.MarketSnapshot, bool) {
	c.mu.RLock()
	s, ok := c.m[symbol]
	c.mu.RUnlock()

	if !ok {
		return protect.MarketSnapshot{}, false
	}

	// 陈旧检查
	now := time.Now().UnixMilli()
	if c.StaleAfterMs > 0 && (now-s.tsMs) > c.StaleAfterMs {
		return protect.MarketSnapshot{}, false
	}

	return protect.MarketSnapshot{
		Symbol:    symbol,
		LastPrice: s.last,
		MarkPrice: s.mark,
		TickSize:  s.tick,
		NowMs:     now,
	}, true
}

// UpdateFromKline 从K线数据更新（辅助方法）
func (c *PriceCache) UpdateFromKline(symbol string, close float64, tick float64) {
	c.Update(symbol, close, close, tick)
}
