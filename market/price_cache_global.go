package market

import (
	"sync"

	marketinternal "nofx/internal/market"
)

var (
	globalPriceCache     *marketinternal.PriceCache
	globalPriceCacheOnce sync.Once
)

// NewGlobalPriceCache 获取全局PriceCache单例
func NewGlobalPriceCache() *marketinternal.PriceCache {
	globalPriceCacheOnce.Do(func() {
		globalPriceCache = marketinternal.NewPriceCache()
	})
	return globalPriceCache
}

// GetGlobalPriceCache 获取全局PriceCache（如果未初始化则返回nil）
func GetGlobalPriceCache() *marketinternal.PriceCache {
	return globalPriceCache
}
