package market

import (
	"log"
	"strconv"
	"sync"
	"time"
)

// ExchangeMetaCache 交易所元数据缓存
// 🔥 P0-2新增：缓存exchangeInfo数据，避免频繁API调用
type ExchangeMetaCache struct {
	mu          sync.RWMutex
	cache       map[string]*ExchangeMeta // symbol -> ExchangeMeta
	lastUpdate  time.Time
	cacheTTL    time.Duration
	apiClient   *APIClient
}

var (
	globalExchangeMetaCache *ExchangeMetaCache
	cacheOnce               sync.Once
)

// GetGlobalExchangeMetaCache 获取全局ExchangeMeta缓存实例（单例模式）
func GetGlobalExchangeMetaCache() *ExchangeMetaCache {
	cacheOnce.Do(func() {
		globalExchangeMetaCache = &ExchangeMetaCache{
			cache:      make(map[string]*ExchangeMeta),
			cacheTTL:   24 * time.Hour, // 24小时TTL
			apiClient:  NewAPIClient(),
		}
		// 启动时预加载
		go func() {
			if err := globalExchangeMetaCache.Refresh(); err != nil {
				log.Printf("⚠️ [ExchangeMetaCache] 初始化加载失败: %v", err)
			}
		}()
	})
	return globalExchangeMetaCache
}

// GetExchangeMeta 获取指定交易对的元数据
func (c *ExchangeMetaCache) GetExchangeMeta(symbol string) (*ExchangeMeta, error) {
	c.mu.RLock()
	// 检查缓存是否过期
	if time.Since(c.lastUpdate) > c.cacheTTL {
		c.mu.RUnlock()
		// 缓存过期，需要刷新
		if err := c.Refresh(); err != nil {
			return nil, err
		}
		c.mu.RLock()
	}

	meta, exists := c.cache[symbol]
	c.mu.RUnlock()

	if !exists {
		// symbol不存在，尝试刷新缓存
		if err := c.Refresh(); err != nil {
			return nil, err
		}
		c.mu.RLock()
		meta, exists = c.cache[symbol]
		c.mu.RUnlock()
		if !exists {
			// 仍然不存在，返回默认值
			log.Printf("⚠️ [ExchangeMetaCache] Symbol %s 未找到，使用默认值", symbol)
			return createFallbackExchangeMeta(symbol), nil
		}
	}

	return meta, nil
}

// Refresh 刷新缓存（从Binance API拉取最新数据）
func (c *ExchangeMetaCache) Refresh() error {
	log.Printf("🔄 [ExchangeMetaCache] 开始刷新缓存...")

	// 调用Binance API
	exchangeInfo, err := c.apiClient.GetExchangeInfo()
	if err != nil {
		return err
	}

	// 解析并更新缓存
	newCache := make(map[string]*ExchangeMeta)

	for _, symbolInfo := range exchangeInfo.Symbols {
		// 只处理TRADING状态的交易对
		if symbolInfo.Status != "TRADING" {
			continue
		}

		meta := parseExchangeMetaFromSymbolInfo(&symbolInfo)
		if meta != nil {
			newCache[symbolInfo.Symbol] = meta
		}
	}

	// 原子更新缓存
	c.mu.Lock()
	c.cache = newCache
	c.lastUpdate = time.Now()
	c.mu.Unlock()

	log.Printf("✅ [ExchangeMetaCache] 缓存刷新成功，共加载 %d 个交易对", len(newCache))
	return nil
}

// parseExchangeMetaFromSymbolInfo 从SymbolInfo解析ExchangeMeta
// 🔥 P0-2核心函数：解析PRICE_FILTER和LOT_SIZE
func parseExchangeMetaFromSymbolInfo(symbolInfo *SymbolInfo) *ExchangeMeta {
	meta := &ExchangeMeta{
		Symbol:   symbolInfo.Symbol,
		TickSize: 0.01,  // 默认值
		LotSize:  1.0,   // 默认值
	}

	// 遍历filters提取tick_size和lot_size
	for _, filter := range symbolInfo.Filters {
		switch filter.FilterType {
		case "PRICE_FILTER":
			// 解析tick_size
			if filter.TickSize != "" {
				if tickSize, err := strconv.ParseFloat(filter.TickSize, 64); err == nil {
					meta.TickSize = tickSize
				}
			}
			// 解析min_price和max_price
			if filter.MinPrice != "" {
				if minPrice, err := strconv.ParseFloat(filter.MinPrice, 64); err == nil {
					meta.MinPrice = minPrice
				}
			}
			if filter.MaxPrice != "" {
				if maxPrice, err := strconv.ParseFloat(filter.MaxPrice, 64); err == nil {
					meta.MaxPrice = maxPrice
				}
			}
		case "LOT_SIZE":
			// 解析lot_size (stepSize)
			if filter.StepSize != "" {
				if lotSize, err := strconv.ParseFloat(filter.StepSize, 64); err == nil {
					meta.LotSize = lotSize
				}
			}
			// 解析min_qty和max_qty
			if filter.MinQty != "" {
				if minQty, err := strconv.ParseFloat(filter.MinQty, 64); err == nil {
					meta.MinQty = minQty
				}
			}
			if filter.MaxQty != "" {
				if maxQty, err := strconv.ParseFloat(filter.MaxQty, 64); err == nil {
					meta.MaxQty = maxQty
				}
			}
		}
	}

	return meta
}

// createFallbackExchangeMeta 创建默认的ExchangeMeta（当API失败或symbol不存在时）
func createFallbackExchangeMeta(symbol string) *ExchangeMeta {
	return &ExchangeMeta{
		Symbol:   symbol,
		TickSize: getSmartTickSizeBySymbol(symbol), // 使用现有的智能推断函数
		LotSize:  1.0,                              // 默认值
	}
}
