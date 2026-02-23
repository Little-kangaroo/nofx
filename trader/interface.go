package trader

import (
	"fmt"
	"time"
)

// OrderTradeDetail 真实成交明细（与交易所API完全一致）
type OrderTradeDetail struct {
	TradeID       int64     `json:"trade_id"`       // 成交唯一ID
	OrderID       int64     `json:"order_id"`       // 订单ID
	Symbol        string    `json:"symbol"`         // 币种
	Side          string    `json:"side"`           // BUY/SELL
	PositionSide  string    `json:"position_side"`  // LONG/SHORT/BOTH
	Price         float64   `json:"price"`          // 成交价格
	Quantity      float64   `json:"quantity"`       // 成交数量
	QuoteQty      float64   `json:"quote_qty"`      // 成交金额
	Commission    float64   `json:"commission"`     // 手续费
	CommissionAsset string  `json:"commission_asset"` // 手续费币种
	RealizedPnL   float64   `json:"realized_pnl"`   // 已实现盈亏
	IsMaker       bool      `json:"is_maker"`       // 是否为挂单方
	TradeTime     time.Time `json:"trade_time"`     // 成交时间
}

// IncomeRecord 资金流水记录（与交易所API完全一致）  
type IncomeRecord struct {
	Symbol     string    `json:"symbol"`      // 币种
	IncomeType string    `json:"income_type"` // REALIZED_PNL/COMMISSION/FUNDING_FEE等
	Income     float64   `json:"income"`      // 金额（正负）
	Asset      string    `json:"asset"`       // 资产类型（USDT/BNB等）
	Info       string    `json:"info"`        // 附加信息
	Time       time.Time `json:"time"`        // 发生时间
	TranID     int64     `json:"tran_id"`     // 交易ID（去重用）
	TradeID    string    `json:"trade_id"`    // 相关成交ID（可能为空）
}

// AuthoritativeOpenData 权威开仓数据（从交易所获取的真实数据）
type AuthoritativeOpenData struct {
	ActualPrice    float64   `json:"actual_price"`     // 真实成交价格
	ActualTime     time.Time `json:"actual_time"`      // 真实成交时间
	ActualQuantity float64   `json:"actual_quantity"`  // 真实成交数量
	Commission     float64   `json:"commission"`       // 手续费
	CommissionAsset string   `json:"commission_asset"` // 手续费币种
	DataSource     string    `json:"data_source"`      // 数据来源标识
	OrderID        string    `json:"order_id"`         // 相关订单ID
	IsMaker        bool      `json:"is_maker"`         // 是否为挂单方
}

// AuthoritativeCloseData 权威平仓数据（从交易所获取的真实数据）
type AuthoritativeCloseData struct {
	ActualPrice    float64   `json:"actual_price"`     // 真实成交价格
	ActualTime     time.Time `json:"actual_time"`      // 真实成交时间
	ActualQuantity float64   `json:"actual_quantity"`  // 真实成交数量
	ActualPnL      float64   `json:"actual_pnl"`       // 真实已实现盈亏
	ActualPnLPct   float64   `json:"actual_pnl_pct"`   // 真实盈亏百分比
	Duration       int       `json:"duration"`         // 持仓时长（秒）
	Commission     float64   `json:"commission"`       // 手续费
	DataSource     string    `json:"data_source"`      // 数据来源标识
	OrderID        string    `json:"order_id"`         // 相关订单ID
}

// Trader 交易器统一接口
// 支持多个交易平台（币安、Hyperliquid等）
type Trader interface {
	// GetBalance 获取账户余额
	GetBalance() (map[string]interface{}, error)

	// GetPositions 获取所有持仓
	GetPositions() ([]map[string]interface{}, error)

	// OpenLong 开多仓
	OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error)

	// OpenShort 开空仓
	OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error)

	// CloseLong 平多仓（quantity=0表示全部平仓）
	CloseLong(symbol string, quantity float64) (map[string]interface{}, error)

	// CloseShort 平空仓（quantity=0表示全部平仓）
	CloseShort(symbol string, quantity float64) (map[string]interface{}, error)

	// SetLeverage 设置杠杆
	SetLeverage(symbol string, leverage int) error

	// SetMarginMode 设置仓位模式 (true=全仓, false=逐仓)
	SetMarginMode(symbol string, isCrossMargin bool) error

	// GetMarketPrice 获取市场价格
	GetMarketPrice(symbol string) (float64, error)

	// SetStopLoss 设置止损单
	SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) (int64, error)

	// SetTakeProfit 设置止盈单
	SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error

	// CancelAllOrders 取消该币种的所有挂单
	CancelAllOrders(symbol string) error

	// GetOrderStatus 获取订单状态
	GetOrderStatus(symbol string, orderID int64) (map[string]interface{}, error)

	// GetOpenOrders 获取指定币种的所有挂单
	GetOpenOrders(symbol string) ([]map[string]interface{}, error)

	// GetOrderHistory 获取指定币种的订单历史（最近N个订单）
	GetOrderHistory(symbol string, limit int) ([]map[string]interface{}, error)

	// GetTradeHistory 获取指定币种的成交历史（包含positionSide、realizedPnl等信息）
	GetTradeHistory(symbol string, limit int) ([]map[string]interface{}, error)

	// FormatQuantity 格式化数量到正确的精度
	FormatQuantity(symbol string, quantity float64) (string, error)

	// ===== 权威数据查询方法（确保与交易所一致） =====
	
	// GetOrderTrades 获取指定订单的真实成交明细
	GetOrderTrades(symbol string, orderID int64) ([]OrderTradeDetail, error)
	
	// GetRecentTrades 获取最近的成交历史
	GetRecentTrades(symbol string, limit int) ([]OrderTradeDetail, error)
	
	// GetIncomeHistory 获取资金流水历史
	GetIncomeHistory(symbol string, incomeType string, limit int) ([]IncomeRecord, error)
}

// TraderFactory 交易器工厂函数类型
type TraderFactory func(apiKey, secretKey string) Trader

var traderFactories = make(map[string]TraderFactory)

// RegisterTraderFactory 注册交易器工厂函数
func RegisterTraderFactory(exchange string, factory TraderFactory) {
	traderFactories[exchange] = factory
}

// NewTrader 根据交易所类型创建交易器
func NewTrader(exchange, apiKey, secretKey string) (Trader, error) {
	factory, ok := traderFactories[exchange]
	if !ok {
		return nil, fmt.Errorf("不支持的交易所: %s", exchange)
	}
	return factory(apiKey, secretKey), nil
}
