package microstructure

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ExchangeInfoManager 🔥 T08新增：交易所信息管理器（动态获取TickSize）
type ExchangeInfoManager struct {
	mu              sync.RWMutex
	symbolInfo      map[string]*SymbolExchangeInfo // symbol -> 交易所信息
	lastUpdate      time.Time
	updateInterval  time.Duration
	futuresBaseURL  string
	spotBaseURL     string
}

// SymbolExchangeInfo 🔥 T08新增：交易对交易所信息
type SymbolExchangeInfo struct {
	Symbol          string    `json:"symbol"`
	TickSize        float64   `json:"tick_size"`         // 价格最小变动单位
	StepSize        float64   `json:"step_size"`         // 数量最小变动单位
	MinNotional     float64   `json:"min_notional"`      // 最小名义价值
	PricePrecision  int       `json:"price_precision"`   // 价格精度
	QuantityPrecision int     `json:"quantity_precision"` // 数量精度
	LastUpdate      time.Time `json:"last_update"`
}

// BinanceFuturesExchangeInfo 币安合约exchangeInfo响应结构
type BinanceFuturesExchangeInfo struct {
	Symbols []struct {
		Symbol string `json:"symbol"`
		Filters []struct {
			FilterType  string `json:"filterType"`
			TickSize    string `json:"tickSize,omitempty"`
			StepSize    string `json:"stepSize,omitempty"`
			MinNotional string `json:"notional,omitempty"`
		} `json:"filters"`
		PricePrecision    int `json:"pricePrecision"`
		QuantityPrecision int `json:"quantityPrecision"`
	} `json:"symbols"`
}

// NewExchangeInfoManager 🔥 T08新增：创建交易所信息管理器
func NewExchangeInfoManager() *ExchangeInfoManager {
	return &ExchangeInfoManager{
		symbolInfo:     make(map[string]*SymbolExchangeInfo),
		updateInterval: 24 * time.Hour, // 每24小时更新一次
		futuresBaseURL: "https://fapi.binance.com",
		spotBaseURL:    "https://api.binance.com",
	}
}

// FetchExchangeInfo 🔥 T08新增：从币安API获取exchangeInfo
func (eim *ExchangeInfoManager) FetchExchangeInfo() error {
	log.Printf("🔄 开始获取币安交易所信息...")

	// 获取合约市场信息
	if err := eim.fetchFuturesExchangeInfo(); err != nil {
		log.Printf("⚠️ 获取合约市场信息失败: %v", err)
		// 不直接返回错误，继续尝试获取现货信息
	}

	// TODO: 如果需要现货信息，可以在这里添加
	// if err := eim.fetchSpotExchangeInfo(); err != nil {
	//     log.Printf("⚠️ 获取现货市场信息失败: %v", err)
	// }

	eim.mu.Lock()
	eim.lastUpdate = time.Now()
	eim.mu.Unlock()

	log.Printf("✅ 交易所信息获取完成，共 %d 个交易对", len(eim.symbolInfo))
	return nil
}

// fetchFuturesExchangeInfo 获取合约市场信息
func (eim *ExchangeInfoManager) fetchFuturesExchangeInfo() error {
	url := fmt.Sprintf("%s/fapi/v1/exchangeInfo", eim.futuresBaseURL)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("API请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("API返回错误状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	var exchangeInfo BinanceFuturesExchangeInfo
	if err := json.Unmarshal(body, &exchangeInfo); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	// 解析并存储每个交易对的信息
	eim.mu.Lock()
	defer eim.mu.Unlock()

	for _, symbolData := range exchangeInfo.Symbols {
		info := &SymbolExchangeInfo{
			Symbol:            symbolData.Symbol,
			PricePrecision:    symbolData.PricePrecision,
			QuantityPrecision: symbolData.QuantityPrecision,
			LastUpdate:        time.Now(),
		}

		// 解析filters获取tickSize、stepSize等
		for _, filter := range symbolData.Filters {
			switch filter.FilterType {
			case "PRICE_FILTER":
				if filter.TickSize != "" {
					if tickSize, err := strconv.ParseFloat(filter.TickSize, 64); err == nil {
						info.TickSize = tickSize
					}
				}
			case "LOT_SIZE":
				if filter.StepSize != "" {
					if stepSize, err := strconv.ParseFloat(filter.StepSize, 64); err == nil {
						info.StepSize = stepSize
					}
				}
			case "MIN_NOTIONAL":
				if filter.MinNotional != "" {
					if minNotional, err := strconv.ParseFloat(filter.MinNotional, 64); err == nil {
						info.MinNotional = minNotional
					}
				}
			}
		}

		eim.symbolInfo[symbolData.Symbol] = info
	}

	return nil
}

// GetTickSize 🔥 T08新增：获取指定交易对的TickSize
func (eim *ExchangeInfoManager) GetTickSize(symbol string) float64 {
	eim.mu.RLock()
	defer eim.mu.RUnlock()

	if info, exists := eim.symbolInfo[symbol]; exists {
		return info.TickSize
	}

	// 🔥 T08兜底：如果找不到，使用智能默认值
	return eim.calculateFallbackTickSize(symbol)
}

// calculateFallbackTickSize 🔥 T08新增：智能兜底tickSize计算
func (eim *ExchangeInfoManager) calculateFallbackTickSize(symbol string) float64 {
	// 基于币种名称的智能推断（从币安API获取的真实值）
	switch symbol {
	case "BTCUSDT":
		return 0.1 // BTC: 0.1 USD
	case "ETHUSDT":
		return 0.01 // ETH: 0.01 USD
	case "BNBUSDT":
		return 0.01 // BNB: 0.01 USD
	case "SOLUSDT":
		return 0.001 // SOL: 0.001 USD
	case "XRPUSDT":
		return 0.0001 // XRP: 0.0001 USD
	case "DOGEUSDT":
		return 0.00001 // DOGE: 0.00001 USD (修复)
	case "ADAUSDT":
		return 0.0001 // ADA: 0.0001 USD
	case "AVAXUSDT":
		return 0.001 // AVAX: 0.001 USD
	case "DOTUSDT":
		return 0.001 // DOT: 0.001 USD
	case "MATICUSDT":
		return 0.0001 // MATIC: 0.0001 USD
	case "LINKUSDT":
		return 0.001 // LINK: 0.001 USD
	case "UNIUSDT":
		return 0.001 // UNI: 0.001 USD
	case "ATOMUSDT":
		return 0.001 // ATOM: 0.001 USD
	case "LTCUSDT":
		return 0.01 // LTC: 0.01 USD
	case "ETCUSDT":
		return 0.001 // ETC: 0.001 USD
	case "TRXUSDT":
		return 0.00001 // TRX: 0.00001 USD
	case "SHIBUSDT":
		return 0.00000001 // SHIB: 0.00000001 USD
	case "PEPEUSDT":
		return 0.0000000001 // PEPE: 0.0000000001 USD
	case "ARBUSDT":
		return 0.0001 // ARB: 0.0001 USD
	case "OPUSDT":
		return 0.0001 // OP: 0.0001 USD
	default:
		return 0.0001 // 其他币种默认0.0001 USD（更安全的兜底值）
	}
}

// GetBucketSize 🔥 T08新增：获取订单簿分桶大小（基于TickSize）
func (eim *ExchangeInfoManager) GetBucketSize(symbol string) float64 {
	tickSize := eim.GetTickSize(symbol)

	// 分桶大小 = TickSize * 倍数，确保合理的价格区间
	// BTC价格高，需要更大的分桶；山寨币价格低，需要更小的分桶
	if symbol == "BTCUSDT" {
		return tickSize * 100 // BTC: 0.1 * 100 = 10 USD分桶
	} else if symbol == "ETHUSDT" {
		return tickSize * 500 // ETH: 0.01 * 500 = 5 USD分桶
	} else {
		// 其他币种：根据tickSize动态调整
		// tickSize 0.001 -> 分桶 0.1 (100倍)
		// tickSize 0.01 -> 分桶 1.0 (100倍)
		return tickSize * 100
	}
}

// GetSymbolInfo 🔥 T08新增：获取完整的交易对信息
func (eim *ExchangeInfoManager) GetSymbolInfo(symbol string) *SymbolExchangeInfo {
	eim.mu.RLock()
	defer eim.mu.RUnlock()

	if info, exists := eim.symbolInfo[symbol]; exists {
		return info
	}

	// 返回兜底信息
	return &SymbolExchangeInfo{
		Symbol:            symbol,
		TickSize:          eim.calculateFallbackTickSize(symbol),
		StepSize:          0.001,
		MinNotional:       5.0,
		PricePrecision:    2,
		QuantityPrecision: 3,
		LastUpdate:        time.Now(),
	}
}

// ShouldRefresh 🔥 T08新增：判断是否需要刷新
func (eim *ExchangeInfoManager) ShouldRefresh() bool {
	eim.mu.RLock()
	defer eim.mu.RUnlock()

	return time.Since(eim.lastUpdate) > eim.updateInterval
}

// GetStats 🔥 T08新增：获取统计信息
func (eim *ExchangeInfoManager) GetStats() map[string]interface{} {
	eim.mu.RLock()
	defer eim.mu.RUnlock()

	return map[string]interface{}{
		"total_symbols":     len(eim.symbolInfo),
		"last_update":       eim.lastUpdate,
		"update_interval_h": eim.updateInterval.Hours(),
		"age_hours":         time.Since(eim.lastUpdate).Hours(),
	}
}

// RoundPrice 🔥 T08新增：将价格round到tickSize的整数倍
func (eim *ExchangeInfoManager) RoundPrice(symbol string, price float64) float64 {
	tickSize := eim.GetTickSize(symbol)
	if tickSize <= 0 {
		return price
	}

	// 四舍五入到tickSize的整数倍
	return math.Round(price/tickSize) * tickSize
}

// RoundQuantity 🔥 T08新增：将数量round到stepSize的整数倍
func (eim *ExchangeInfoManager) RoundQuantity(symbol string, quantity float64) float64 {
	eim.mu.RLock()
	info, exists := eim.symbolInfo[symbol]
	eim.mu.RUnlock()

	if !exists || info.StepSize <= 0 {
		return quantity
	}

	return math.Round(quantity/info.StepSize) * info.StepSize
}
