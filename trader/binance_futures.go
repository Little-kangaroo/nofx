package trader

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

// TradeRecord 交易记录结构（简化版用于数据验证）
type TradeRecord struct {
	ID            string     `json:"id"`
	TraderID      string     `json:"trader_id"`
	Symbol        string     `json:"symbol"`
	Side          string     `json:"side"` // 'long' 或 'short'
	Quantity      float64    `json:"quantity"`
	Leverage      int        `json:"leverage"`
	OpenPrice     float64    `json:"open_price"`
	ClosePrice    *float64   `json:"close_price"`
	PositionValue float64    `json:"position_value"`
	MarginUsed    float64    `json:"margin_used"`
	PnL           float64    `json:"pnl"`
	PnLPct        float64    `json:"pnl_pct"`
	DurationSecs  int        `json:"duration_seconds"`
	OpenTime      time.Time  `json:"open_time"`
	CloseTime     *time.Time `json:"close_time"`
	Status        string     `json:"status"` // 'open', 'closed', 'liquidated'
	CloseReason   string     `json:"close_reason"` // 'manual', 'stop_loss', 'take_profit', 'liquidation'
	OpenOrderID   string     `json:"open_order_id"`
	CloseOrderID  string     `json:"close_order_id"`
}

// OrderExecutionResult 订单执行结果（含实际成交数据）
type OrderExecutionResult struct {
	OrderID         int64     `json:"order_id"`
	Symbol          string    `json:"symbol"`
	ActualFillPrice float64   `json:"actual_fill_price"` // 实际成交价格
	ActualFillQty   float64   `json:"actual_fill_qty"`   // 实际成交数量
	FillTime        time.Time `json:"fill_time"`         // 实际成交时间
	Status          string    `json:"status"`
}

// FillInfo 成交信息
type FillInfo struct {
	AvgPrice  float64   `json:"avg_price"`
	FilledQty float64   `json:"filled_qty"`
	FillTime  time.Time `json:"fill_time"`
}

// FuturesTrader 币安合约交易器
type FuturesTrader struct {
	client    *futures.Client
	apiKey    string // 新增：API密钥
	secretKey string // 新增：私钥

	// 余额缓存
	cachedBalance     map[string]interface{}
	balanceCacheTime  time.Time
	balanceCacheMutex sync.RWMutex

	// 持仓缓存
	cachedPositions     []map[string]interface{}
	positionsCacheTime  time.Time
	positionsCacheMutex sync.RWMutex

	// 缓存有效期（15秒）
	cacheDuration time.Duration
}

// NewFuturesTrader 创建合约交易器
func NewFuturesTrader(apiKey, secretKey string) *FuturesTrader {
	// 打印API配置信息（脱敏）
	maskedApiKey := ""
	if len(apiKey) > 8 {
		maskedApiKey = apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
	} else {
		maskedApiKey = "***"
	}
	maskedSecretKey := ""
	if len(secretKey) > 8 {
		maskedSecretKey = secretKey[:4] + "..." + secretKey[len(secretKey)-4:]
	} else {
		maskedSecretKey = "***"
	}
	log.Printf("🔧 [Binance] API配置: APIKey=%s, SecretKey=%s", maskedApiKey, maskedSecretKey)

	client := futures.NewClient(apiKey, secretKey)
	return &FuturesTrader{
		client:        client,
		apiKey:        apiKey,    // 新增：保存API密钥
		secretKey:     secretKey, // 新增：保存私钥
		cacheDuration: 15 * time.Second, // 15秒缓存
	}
}

// GetBalance 获取账户余额（带缓存）
func (t *FuturesTrader) GetBalance() (map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.balanceCacheMutex.RLock()
	if t.cachedBalance != nil && time.Since(t.balanceCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.balanceCacheTime)
		t.balanceCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的账户余额（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedBalance, nil
	}
	t.balanceCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	log.Printf("🔄 缓存过期，正在调用币安API获取账户余额...")
	log.Printf("🌐 [Binance API] 调用: NewGetAccountService().Do()")
	account, err := t.client.NewGetAccountService().Do(context.Background())
	if err != nil {
		log.Printf("❌ 币安API调用失败: %v", err)
		return nil, fmt.Errorf("获取账户信息失败: %w", err)
	}

	result := make(map[string]interface{})
	result["totalWalletBalance"], _ = strconv.ParseFloat(account.TotalWalletBalance, 64)
	result["availableBalance"], _ = strconv.ParseFloat(account.AvailableBalance, 64)
	result["totalUnrealizedProfit"], _ = strconv.ParseFloat(account.TotalUnrealizedProfit, 64)

	log.Printf("✓ 币安API返回: 总余额=%s, 可用=%s, 未实现盈亏=%s",
		account.TotalWalletBalance,
		account.AvailableBalance,
		account.TotalUnrealizedProfit)

	// 更新缓存
	t.balanceCacheMutex.Lock()
	t.cachedBalance = result
	t.balanceCacheTime = time.Now()
	t.balanceCacheMutex.Unlock()

	return result, nil
}

// GetPositions 获取所有持仓（带缓存）
func (t *FuturesTrader) GetPositions() ([]map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.positionsCacheMutex.RLock()
	if t.cachedPositions != nil && time.Since(t.positionsCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.positionsCacheTime)
		t.positionsCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的持仓信息（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedPositions, nil
	}
	t.positionsCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	log.Printf("🔄 缓存过期，正在调用币安API获取持仓信息...")
	log.Printf("🌐 [Binance API] 调用: NewGetPositionRiskService().Do()")
	positions, err := t.client.NewGetPositionRiskService().Do(context.Background())
	if err != nil {
		log.Printf("❌ 币安API获取持仓失败: %v", err)
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		posAmt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
		if posAmt == 0 {
			continue // 跳过无持仓的
		}

		posMap := make(map[string]interface{})
		posMap["symbol"] = pos.Symbol
		posMap["positionAmt"], _ = strconv.ParseFloat(pos.PositionAmt, 64)
		posMap["entryPrice"], _ = strconv.ParseFloat(pos.EntryPrice, 64)
		posMap["markPrice"], _ = strconv.ParseFloat(pos.MarkPrice, 64)
		posMap["unRealizedProfit"], _ = strconv.ParseFloat(pos.UnRealizedProfit, 64)
		posMap["leverage"], _ = strconv.ParseFloat(pos.Leverage, 64)
		posMap["liquidationPrice"], _ = strconv.ParseFloat(pos.LiquidationPrice, 64)

		// 判断方向
		if posAmt > 0 {
			posMap["side"] = "long"
		} else {
			posMap["side"] = "short"
		}

		result = append(result, posMap)
	}

	// 更新缓存
	t.positionsCacheMutex.Lock()
	t.cachedPositions = result
	t.positionsCacheTime = time.Now()
	t.positionsCacheMutex.Unlock()

	return result, nil
}

// SetMarginMode 设置仓位模式
func (t *FuturesTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	var marginType futures.MarginType
	if isCrossMargin {
		marginType = futures.MarginTypeCrossed
	} else {
		marginType = futures.MarginTypeIsolated
	}

	// 尝试设置仓位模式
	err := t.client.NewChangeMarginTypeService().
		Symbol(symbol).
		MarginType(marginType).
		Do(context.Background())

	marginModeStr := "全仓"
	if !isCrossMargin {
		marginModeStr = "逐仓"
	}

	if err != nil {
		// 如果错误信息包含"No need to change"，说明仓位模式已经是目标值
		if contains(err.Error(), "No need to change margin type") {
			log.Printf("  ✓ %s 仓位模式已是 %s", symbol, marginModeStr)
			return nil
		}
		// 如果有持仓，无法更改仓位模式，但不影响交易
		if contains(err.Error(), "Margin type cannot be changed if there exists position") {
			log.Printf("  ⚠️ %s 有持仓，无法更改仓位模式，继续使用当前模式", symbol)
			return nil
		}
		log.Printf("  ⚠️ 设置仓位模式失败: %v", err)
		// 不返回错误，让交易继续
		return nil
	}

	log.Printf("  ✓ %s 仓位模式已设置为 %s", symbol, marginModeStr)
	return nil
}

// SetLeverage 设置杠杆（智能判断+冷却期）
func (t *FuturesTrader) SetLeverage(symbol string, leverage int) error {
	// 先尝试获取当前杠杆（从持仓信息）
	currentLeverage := 0
	positions, err := t.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				if lev, ok := pos["leverage"].(float64); ok {
					currentLeverage = int(lev)
					break
				}
			}
		}
	}

	// 如果当前杠杆已经是目标杠杆，跳过
	if currentLeverage == leverage && currentLeverage > 0 {
		log.Printf("  ✓ %s 杠杆已是 %dx，无需切换", symbol, leverage)
		return nil
	}

	// 切换杠杆
	log.Printf("🌐 [Binance API] 调用: NewChangeLeverageService() - Symbol=%s, Leverage=%d", symbol, leverage)
	_, err = t.client.NewChangeLeverageService().
		Symbol(symbol).
		Leverage(leverage).
		Do(context.Background())

	if err != nil {
		// 如果错误信息包含"No need to change"，说明杠杆已经是目标值
		if contains(err.Error(), "No need to change") {
			log.Printf("  ✓ %s 杠杆已是 %dx", symbol, leverage)
			return nil
		}
		return fmt.Errorf("设置杠杆失败: %w", err)
	}

	log.Printf("  ✓ %s 杠杆已切换为 %dx", symbol, leverage)

	// 切换杠杆后等待5秒（避免冷却期错误）
	log.Printf("  ⏱ 等待5秒冷却期...")
	time.Sleep(5 * time.Second)

	return nil
}

// OpenLong 开多仓
func (t *FuturesTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 注意：仓位模式应该由调用方（AutoTrader）在开仓前通过 SetMarginMode 设置

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价买入订单
	log.Printf("🌐 [Binance API] 调用: NewCreateOrderService() - Symbol=%s, Side=BUY, PositionSide=LONG, Type=MARKET, Quantity=%s", symbol, quantityStr)
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("开多仓失败: %w", err)
	}

	log.Printf("✓ 开多仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// OpenShort 开空仓
func (t *FuturesTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 注意：仓位模式应该由调用方（AutoTrader）在开仓前通过 SetMarginMode 设置

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价卖出订单
	log.Printf("🌐 [Binance API] 调用: NewCreateOrderService() - Symbol=%s, Side=SELL, PositionSide=SHORT, Type=MARKET, Quantity=%s", symbol, quantityStr)
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("开空仓失败: %w", err)
	}

	log.Printf("✓ 开空仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseLong 平多仓
func (t *FuturesTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	log.Printf("🔍 [CloseLong] 开始平多仓: symbol=%s, quantity=%.6f", symbol, quantity)
	
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		log.Printf("🔍 [CloseLong] 数量为0，查询当前持仓...")
		
		// 🔧 重要：清除缓存，获取最新持仓状态
		t.positionsCacheMutex.Lock()
		t.cachedPositions = nil
		t.positionsCacheMutex.Unlock()
		
		positions, err := t.GetPositions()
		if err != nil {
			log.Printf("❌ [CloseLong] 获取持仓失败: %v", err)
			return nil, err
		}

		log.Printf("🔍 [CloseLong] 获取到 %d 个持仓，正在查找 %s 的多仓...", len(positions), symbol)
		
		foundPosition := false
		for i, pos := range positions {
			posSymbol := pos["symbol"].(string)
			posSide := pos["side"].(string)
			posAmt := pos["positionAmt"].(float64)
			
			log.Printf("  持仓#%d: symbol=%s, side=%s, amount=%.6f", i+1, posSymbol, posSide, posAmt)
			
			if posSymbol == symbol && posSide == "long" {
				quantity = posAmt
				foundPosition = true
				log.Printf("✅ [CloseLong] 找到目标持仓: %s long, 数量=%.6f", symbol, quantity)
				break
			}
		}

		if !foundPosition {
			log.Printf("❌ [CloseLong] 未找到 %s 的多仓持仓", symbol)
		}

		if quantity == 0 {
			log.Printf("❌ [CloseLong] 最终数量为0，可能持仓已被其他方式平掉")
			
			// 🔧 改进：获取更多信息用于数据库同步
			// 尝试获取当前价格作为推测平仓价格
			currentPrice := 0.0
			if marketPrice, priceErr := t.GetMarketPrice(symbol); priceErr == nil {
				currentPrice = marketPrice
			}
			
			// 返回特殊的"已平仓"标识，包含更多同步所需信息
			return map[string]interface{}{
				"orderId":          "ALREADY_CLOSED",
				"symbol":           symbol,
				"status":           "ALREADY_CLOSED", 
				"message":          "持仓不存在，可能已被其他方式平掉",
				"estimated_price":  currentPrice, // 添加估算平仓价格
				"close_reason":     "external",   // 标识为外部平仓
				"sync_required":    true,         // 标识需要数据库同步
				"detection_method": "position_check", // 检测方法
			}, nil
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价卖出订单（平多）
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("平多仓失败: %w", err)
	}

	log.Printf("✓ 平多仓成功: %s 数量: %s", symbol, quantityStr)

	// 平仓后取消该币种的所有挂单（止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseShort 平空仓
func (t *FuturesTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	log.Printf("🔍 [CloseShort] 开始平空仓: symbol=%s, quantity=%.6f", symbol, quantity)
	
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		log.Printf("🔍 [CloseShort] 数量为0，查询当前持仓...")
		
		// 🔧 重要：清除缓存，获取最新持仓状态
		t.positionsCacheMutex.Lock()
		t.cachedPositions = nil
		t.positionsCacheMutex.Unlock()
		
		positions, err := t.GetPositions()
		if err != nil {
			log.Printf("❌ [CloseShort] 获取持仓失败: %v", err)
			return nil, err
		}

		log.Printf("🔍 [CloseShort] 获取到 %d 个持仓，正在查找 %s 的空仓...", len(positions), symbol)
		
		foundPosition := false
		for i, pos := range positions {
			posSymbol := pos["symbol"].(string)
			posSide := pos["side"].(string)
			posAmt := pos["positionAmt"].(float64)
			
			log.Printf("  持仓#%d: symbol=%s, side=%s, amount=%.6f", i+1, posSymbol, posSide, posAmt)
			
			if posSymbol == symbol && posSide == "short" {
				quantity = -posAmt // 空仓数量是负的，取绝对值
				foundPosition = true
				log.Printf("✅ [CloseShort] 找到目标持仓: %s short, 数量=%.6f", symbol, quantity)
				break
			}
		}

		if !foundPosition {
			log.Printf("❌ [CloseShort] 未找到 %s 的空仓持仓", symbol)
		}

		if quantity == 0 {
			log.Printf("❌ [CloseShort] 最终数量为0，可能持仓已被其他方式平掉")
			// 🔧 修复：返回特殊的"已平仓"标识而不是错误，让上���处理数据库同步
			return map[string]interface{}{
				"orderId": "ALREADY_CLOSED",
				"symbol":  symbol,
				"status":  "ALREADY_CLOSED",
				"message": "持仓不存在，可能已被其他方式平掉",
			}, nil
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价买入订单（平空）
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("平空仓失败: %w", err)
	}

	log.Printf("✓ 平空仓成功: %s 数量: %s", symbol, quantityStr)

	// 平仓后取消该币种的所有挂单（止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CancelAllOrders 取消该币种的所有挂单
func (t *FuturesTrader) CancelAllOrders(symbol string) error {
	err := t.client.NewCancelAllOpenOrdersService().
		Symbol(symbol).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("取消挂单失败: %w", err)
	}

	log.Printf("  ✓ 已取消 %s 的所有挂单", symbol)
	return nil
}

// GetMarketPrice 获取市场价格
func (t *FuturesTrader) GetMarketPrice(symbol string) (float64, error) {
	prices, err := t.client.NewListPricesService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("获取价格失败: %w", err)
	}

	if len(prices) == 0 {
		return 0, fmt.Errorf("未找到价格")
	}

	price, err := strconv.ParseFloat(prices[0].Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}

// CalculatePositionSize 计算仓位大小
func (t *FuturesTrader) CalculatePositionSize(balance, riskPercent, price float64, leverage int) float64 {
	riskAmount := balance * (riskPercent / 100.0)
	positionValue := riskAmount * float64(leverage)
	quantity := positionValue / price
	return quantity
}

// SetStopLoss 设置止损单 - 使用新的算法订单API (2025-12-09后币安要求)
func (t *FuturesTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) (int64, error) {
	var side string
	var binancePosSide string

	if positionSide == "LONG" {
		side = "SELL"
		binancePosSide = "LONG"
	} else {
		side = "BUY" 
		binancePosSide = "SHORT"
	}

	// 🔧 修复精度问题：使用正确的价格精度格式化止损价格
	formattedStopPrice, err := t.FormatPrice(symbol, stopPrice)
	if err != nil {
		log.Printf("⚠️ 获取价格精度失败，使用默认格式: %v", err)
		formattedStopPrice = fmt.Sprintf("%.2f", stopPrice)
	}
	
	log.Printf("🔧 [新算法API] 止损价格格式化: 原始=%.8f, 格式化=%s", stopPrice, formattedStopPrice)

	// 🔥 关键修复：先撤销该交易对所有算法订单，避免重复创建
	log.Printf("🔄 [算法API] 撤销 %s 的所有算法订单...", symbol)
	if err := t.cancelAllAlgoOrders(symbol); err != nil {
		log.Printf("⚠️ [算法API] 撤销所有算法订单失败，继续创建: %v", err)
	}

	// 🔥 关键修复：使用新的算法订单API (POST /fapi/v1/algoOrder)
	// 币安从2025-12-09起要求所有条件单使用算法订单接口
	response, err := t.createAlgoStopOrder(symbol, side, binancePosSide, formattedStopPrice)
	if err != nil {
		return 0, fmt.Errorf("设置止损失败: %w", err)
	}

	log.Printf("  ✅ [算法API] 止损单设置成功: %s, AlgoID: %v", formattedStopPrice, response["algoId"])
	
	// 返回AlgoID作为OrderID（用于跟踪）
	if algoId, ok := response["algoId"].(float64); ok {
		return int64(algoId), nil
	}
	return 0, nil
}

// createAlgoStopOrder 创建算法止损订单 - 直接调用币安新的算法订单API
func (t *FuturesTrader) createAlgoStopOrder(symbol, side, positionSide, triggerPrice string) (map[string]interface{}, error) {
	// 构建请求参数
	params := map[string]interface{}{
		"algoType":      "CONDITIONAL",
		"symbol":        symbol,
		"side":          side,
		"positionSide":  positionSide,
		"type":          "STOP_MARKET",
		"triggerPrice":  triggerPrice,
		"workingType":   "CONTRACT_PRICE",
		"closePosition": "true", // 触发后全部平仓
		"timeInForce":   "GTC",
		"timestamp":     time.Now().UnixMilli(),
	}

	// 构建查询字符串用于签名
	var queryParts []string
	for key, value := range params {
		queryParts = append(queryParts, fmt.Sprintf("%s=%v", key, value))
	}
	queryString := strings.Join(queryParts, "&")

	// 生成签名
	signature := t.generateSignature(queryString)
	queryString += "&signature=" + signature

	// 🔥 修复：币安API使用POST + URL编码表单，不是JSON
	url := "https://fapi.binance.com/fapi/v1/algoOrder"
	req, err := http.NewRequest("POST", url, strings.NewReader(queryString))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-MBX-APIKEY", t.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	log.Printf("🔧 [算法API] 请求参数: %s", queryString)
	log.Printf("🔧 [算法API] 响应状态: %d", resp.StatusCode)
	log.Printf("🔧 [算法API] 响应内容: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API错误 [%d]: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return result, nil
}

// cancelAllAlgoOrders 撤销指定交易对的所有算法订单
func (t *FuturesTrader) cancelAllAlgoOrders(symbol string) error {
	// 构建请求参数
	params := map[string]interface{}{
		"symbol":    symbol,
		"timestamp": time.Now().UnixMilli(),
	}

	// 构建查询字符串用于签名
	var queryParts []string
	for key, value := range params {
		queryParts = append(queryParts, fmt.Sprintf("%s=%v", key, value))
	}
	queryString := strings.Join(queryParts, "&")

	// 生成签名
	signature := t.generateSignature(queryString)
	queryString += "&signature=" + signature

	// 发送DELETE请求到 /fapi/v1/algoOpenOrders
	url := "https://fapi.binance.com/fapi/v1/algoOpenOrders"
	req, err := http.NewRequest("DELETE", url, strings.NewReader(queryString))
	if err != nil {
		return fmt.Errorf("创建撤销请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-MBX-APIKEY", t.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("撤销请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取撤销响应失败: %w", err)
	}

	log.Printf("🔧 [算法API] 撤销所有订单响应状态: %d", resp.StatusCode)
	log.Printf("🔧 [算法API] 撤销所有订单响应内容: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("撤销所有算法订单API错误 [%d]: %s", resp.StatusCode, string(body))
	}

	log.Printf("✅ [算法API] 已撤销 %s 的所有算法订单", symbol)
	return nil
}

// GenerateSignature 生成币安API签名 (导出方法用于测试)
func (t *FuturesTrader) GenerateSignature(queryString string) string {
	return t.generateSignature(queryString)
}

// generateSignature 生成币安API签名
func (t *FuturesTrader) generateSignature(queryString string) string {
	mac := hmac.New(sha256.New, []byte(t.secretKey))
	mac.Write([]byte(queryString))
	return hex.EncodeToString(mac.Sum(nil))
}
func (t *FuturesTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// 🔧 修复精度问题：使用正确的价格精度格式化止盈价格
	formattedTakeProfitPrice, err := t.FormatPrice(symbol, takeProfitPrice)
	if err != nil {
		log.Printf("⚠️ 获取价格精度失败，使用默认格式: %v", err)
		formattedTakeProfitPrice = fmt.Sprintf("%.2f", takeProfitPrice)
	}
	
	log.Printf("🔧 止盈价格格式化: 原始=%.8f, 格式化=%s", takeProfitPrice, formattedTakeProfitPrice)

	// 🔧 修复止盈订单：使用ClosePosition时不需要设置Quantity
	// ClosePosition(true) 会自动平掉整个仓位，quantity参数会被忽略
	_, err = t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.OrderTypeTakeProfitMarket).
		StopPrice(formattedTakeProfitPrice).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("设置止盈失败: %w", err)
	}

	log.Printf("  ✅ 止盈价设置成功: %s", formattedTakeProfitPrice)
	return nil
}

// GetSymbolPrecision 获取交易对的数量精度
func (t *FuturesTrader) GetSymbolPrecision(symbol string) (int, error) {
	exchangeInfo, err := t.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("获取交易规则失败: %w", err)
	}

	for _, s := range exchangeInfo.Symbols {
		if s.Symbol == symbol {
			// 从LOT_SIZE filter获取精度
			for _, filter := range s.Filters {
				if filter["filterType"] == "LOT_SIZE" {
					stepSize := filter["stepSize"].(string)
					precision := calculatePrecision(stepSize)
					log.Printf("  %s 数量精度: %d (stepSize: %s)", symbol, precision, stepSize)
					return precision, nil
				}
			}
		}
	}

	log.Printf("  ⚠ %s 未找到精度信息，使用默认精度3", symbol)
	return 3, nil // 默认精度为3
}

// GetPricePrecision 获取交易对的价格精度
func (t *FuturesTrader) GetPricePrecision(symbol string) (int, error) {
	exchangeInfo, err := t.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("获取交易规则失败: %w", err)
	}

	for _, s := range exchangeInfo.Symbols {
		if s.Symbol == symbol {
			// 从PRICE_FILTER filter获取价格精度
			for _, filter := range s.Filters {
				if filter["filterType"] == "PRICE_FILTER" {
					tickSize := filter["tickSize"].(string)
					precision := calculatePrecision(tickSize)
					log.Printf("  %s 价格精度: %d (tickSize: %s)", symbol, precision, tickSize)
					return precision, nil
				}
			}
		}
	}

	log.Printf("  ⚠ %s 未找到价格精度信息，使用默认精度2", symbol)
	return 2, nil // 默认价格精度为2
}

// calculatePrecision 从stepSize计算精度
func calculatePrecision(stepSize string) int {
	// 去除尾部的0
	stepSize = trimTrailingZeros(stepSize)

	// 查找小数点
	dotIndex := -1
	for i := 0; i < len(stepSize); i++ {
		if stepSize[i] == '.' {
			dotIndex = i
			break
		}
	}

	// 如果没有小数点或小数点在最后，精度为0
	if dotIndex == -1 || dotIndex == len(stepSize)-1 {
		return 0
	}

	// 返回小数点后的位数
	return len(stepSize) - dotIndex - 1
}

// trimTrailingZeros 去除尾部的0
func trimTrailingZeros(s string) string {
	// 如果没有小数点，直接返回
	if !stringContains(s, ".") {
		return s
	}

	// 从后向前遍历，去除尾部的0
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}

	// 如果最后一位是小数点，也去掉
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}

	return s
}

// FormatQuantity 格式化数量到正确的精度
func (t *FuturesTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	precision, err := t.GetSymbolPrecision(symbol)
	if err != nil {
		// 如果获取失败，使用默认格式
		return fmt.Sprintf("%.3f", quantity), nil
	}

	format := fmt.Sprintf("%%.%df", precision)
	return fmt.Sprintf(format, quantity), nil
}

// FormatPrice 格式化价格到正确的精度
func (t *FuturesTrader) FormatPrice(symbol string, price float64) (string, error) {
	precision, err := t.GetPricePrecision(symbol)
	if err != nil {
		// 如果获取失败，使用默认格式
		return fmt.Sprintf("%.2f", price), nil
	}

	format := fmt.Sprintf("%%.%df", precision)
	return fmt.Sprintf(format, price), nil
}

// GetOrderStatus 获取订单状态
func (t *FuturesTrader) GetOrderStatus(symbol string, orderID int64) (map[string]interface{}, error) {
	service := t.client.NewGetOrderService().Symbol(symbol).OrderID(orderID)
	order, err := service.Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("查询订单状态失败: %w", err)
	}

	result := map[string]interface{}{
		"orderId":     order.OrderID,
		"symbol":      order.Symbol,
		"status":      string(order.Status),
		"type":        string(order.Type),
		"side":        string(order.Side),
		"origQty":     order.OrigQuantity,
		"executedQty": order.ExecutedQuantity,
		"price":       order.Price,
		"stopPrice":   order.StopPrice,
		"timeInForce": string(order.TimeInForce),
		"updateTime":  order.UpdateTime,
	}

	return result, nil
}

// 辅助函数
func contains(s, substr string) bool {
	return len(s) >= len(substr) && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetOpenOrders 获取指定币种的所有挂单 (🔥 已更新为使用新的算法订单接口)
func (t *FuturesTrader) GetOpenOrders(symbol string) ([]map[string]interface{}, error) {
	log.Printf("🔍 [Binance] 查询 %s 的挂单 (使用新算法接口)...", symbol)
	
	// 🔥 直接使用新的算法订单接口，因为止损单已迁移到算法服务
	// 从2025-12-09起，STOP_MARKET/TAKE_PROFIT_MARKET等订单类型在旧接口中会被拦截
	return t.getOpenAlgoOrders(symbol)
}

// getOpenAlgoOrders 获取指定币种的所有算法挂单 (新接口)
func (t *FuturesTrader) getOpenAlgoOrders(symbol string) ([]map[string]interface{}, error) {
	log.Printf("🔍 [Binance] 查询 %s 的算法挂单...", symbol)
	
	// 构建请求参数
	params := map[string]interface{}{
		"symbol":    symbol,
		"timestamp": time.Now().UnixMilli(),
	}

	// 构建查询字符串用于签名
	var queryParts []string
	for key, value := range params {
		queryParts = append(queryParts, fmt.Sprintf("%s=%v", key, value))
	}
	queryString := strings.Join(queryParts, "&")

	// 生成签名
	signature := t.generateSignature(queryString)
	queryString += "&signature=" + signature

	// 发送GET请求到 /fapi/v1/openAlgoOrders
	url := "https://fapi.binance.com/fapi/v1/openAlgoOrders?" + queryString
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建查询请求失败: %w", err)
	}

	req.Header.Set("X-MBX-APIKEY", t.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("查询请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != 200 {
		log.Printf("❌ [Binance] 查询算法挂单失败: %s", string(body))
		return nil, fmt.Errorf("API错误 %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var algoOrders []map[string]interface{}
	if err := json.Unmarshal(body, &algoOrders); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	log.Printf("📋 [Binance] %s 找到 %d 个算法挂单", symbol, len(algoOrders))

	// 转换为统一格式，映射算法订单字段到原格式
	result := make([]map[string]interface{}, len(algoOrders))
	for i, order := range algoOrders {
		result[i] = map[string]interface{}{
			"symbol":       order["symbol"],
			"orderId":      order["algoId"],
			"type":         order["orderType"],           // STOP_MARKET等
			"side":         order["side"],
			"quantity":     order["quantity"],
			"price":        order["price"],
			"stopPrice":    order["triggerPrice"],        // 关键：算法订单的触发价格
			"status":       order["algoStatus"],
			"timeInForce":  order["timeInForce"],
			"reduceOnly":   order["reduceOnly"],
			"positionSide": order["positionSide"],
		}
		
		log.Printf("  📄 算法订单#%v: %v %v %v 数量:%v 价格:%v 触发价:%v", 
			order["algoId"], order["orderType"], order["side"], order["positionSide"], 
			order["quantity"], order["price"], order["triggerPrice"])
	}
	
	return result, nil
}

// GetOrderHistory 获取指定币种的订单历史
func (t *FuturesTrader) GetOrderHistory(symbol string, limit int) ([]map[string]interface{}, error) {
	log.Printf("🔍 [Binance] 查询 %s 的订单历史 (最近%d个订单)...", symbol, limit)
	
	// 调用币安API获取订单历史
	service := t.client.NewListOrdersService().Symbol(symbol)
	if limit > 0 && limit <= 1000 { // 币安API限制最多1000个订单
		service = service.Limit(limit)
	}
	
	orders, err := service.Do(context.Background())
	if err != nil {
		log.Printf("❌ [Binance] 获取订单历史失败: %v", err)
		return nil, fmt.Errorf("获取订单历史失败: %w", err)
	}
	
	log.Printf("📋 [Binance] %s 找到 %d 个历史订单", symbol, len(orders))
	
	// 转换为统一格式，只保留已完成的订单
	var result []map[string]interface{}
	stopLossCount := 0
	takeProfitCount := 0
	
	for _, order := range orders {
		// 只处理已完成的订单
		if order.Status != "FILLED" {
			continue
		}
		
		orderMap := map[string]interface{}{
			"symbol":       order.Symbol,
			"orderId":      order.OrderID,
			"type":         string(order.Type),
			"side":         string(order.Side),
			"quantity":     order.OrigQuantity,
			"price":        order.Price,
			"avgPrice":     order.AvgPrice, // 实际成交价格
			"stopPrice":    order.StopPrice,
			"status":       string(order.Status),
			"timeInForce":  string(order.TimeInForce),
			"reduceOnly":   order.ReduceOnly,
			"positionSide": string(order.PositionSide),
			"updateTime":   order.UpdateTime,
			"workingType":  string(order.WorkingType),
		}
		
		// 统计止损止盈订单
		if order.Type == "STOP_MARKET" || order.Type == "STOP" {
			stopLossCount++
			log.Printf("  📄 止损单: ID=%d, %s %s %s, 止损价:%s, 成交价:%s, 时间:%d", 
				order.OrderID, order.Type, order.Side, order.PositionSide, 
				order.StopPrice, order.AvgPrice, order.UpdateTime)
		} else if order.Type == "TAKE_PROFIT_MARKET" || order.Type == "TAKE_PROFIT" {
			takeProfitCount++
			log.Printf("  📄 止盈单: ID=%d, %s %s %s, 止盈价:%s, 成交价:%s, 时间:%d", 
				order.OrderID, order.Type, order.Side, order.PositionSide, 
				order.StopPrice, order.AvgPrice, order.UpdateTime)
		}
		
		result = append(result, orderMap)
	}
	
	log.Printf("📊 [Binance] %s 历史订单统计: 总计%d个已完成订单 (止损:%d, 止盈:%d)", 
		symbol, len(result), stopLossCount, takeProfitCount)
	
	return result, nil
}

// GetTradeHistory 获取指定币种的成交历史（Account Trade List）
func (t *FuturesTrader) GetTradeHistory(symbol string, limit int) ([]map[string]interface{}, error) {
	log.Printf("🔍 [Binance] 查询 %s 的成交历史 (最近%d条)...", symbol, limit)
	
	// 调用币安API获取成交历史 (Account Trade List)
	service := t.client.NewListAccountTradeService().Symbol(symbol)
	if limit > 0 && limit <= 1000 { // 币安API限制最多1000条记录
		service = service.Limit(limit)
	}
	
	trades, err := service.Do(context.Background())
	if err != nil {
		log.Printf("❌ [Binance] 获取成交历史失败: %v", err)
		return nil, fmt.Errorf("获取成交历史失败: %w", err)
	}
	
	log.Printf("📋 [Binance] %s 找到 %d 条成交历史", symbol, len(trades))
	
	// 转换为统一格式
	var result []map[string]interface{}
	for _, trade := range trades {
		tradeMap := map[string]interface{}{
			"symbol":       trade.Symbol,
			"id":           trade.ID,
			"orderId":      trade.OrderID,
			"side":         string(trade.Side),        // BUY/SELL
			"qty":          trade.Quantity,            // 成交数量
			"price":        trade.Price,               // 成交价格
			"quoteQty":     trade.QuoteQuantity,       // 成交金额
			"commission":   trade.Commission,          // 手续费
			"commissionAsset": trade.CommissionAsset, // 手续费币种
			"time":         trade.Time,               // 成交时间
			"positionSide": string(trade.PositionSide), // LONG/SHORT/BOTH
			"realizedPnl":  trade.RealizedPnl,        // 实现盈亏
			"buyer":        trade.Buyer,              // 是否为买方
		}
		result = append(result, tradeMap)
		
		// 记录关键信息用于调试
		if trade.RealizedPnl != "0" {
			log.Printf("  💰 平仓交易: %s %s %s, 价格=%s, 盈亏=%s, 时间=%d", 
				trade.Symbol, trade.PositionSide, trade.Side, trade.Price, trade.RealizedPnl, trade.Time)
		}
	}
	
	log.Printf("✅ [Binance] 成交历史查询完成: %d条记录", len(result))
	return result, nil
}

// waitForOrderFill 等待订单完全成交并返回实际成交信息
func (t *FuturesTrader) waitForOrderFill(symbol string, orderID int64, timeout time.Duration) (*FillInfo, error) {
	log.Printf("⏳ [WaitForFill] 等待订单成交: OrderID=%d, Timeout=%v", orderID, timeout)
	
	ticker := time.NewTicker(200 * time.Millisecond) // 每200ms检查一次
	defer ticker.Stop()
	
	deadline := time.Now().Add(timeout)
	checkCount := 0
	
	for time.Now().Before(deadline) {
		select {
		case <-ticker.C:
			checkCount++
			
			status, err := t.GetOrderStatus(symbol, orderID)
			if err != nil {
				if checkCount%25 == 0 { // 每5秒记录一次错误
					log.Printf("⚠️ [WaitForFill] 检查订单状态失败(#%d): %v", checkCount, err)
				}
				continue // 继续等待
			}
			
			orderStatus, ok := status["status"].(string)
			if !ok {
				continue
			}
			
			log.Printf("🔍 [WaitForFill] 订单状态检查(#%d): %s", checkCount, orderStatus)
			
			if orderStatus == "FILLED" {
				// 订单已完全成交，解析实际成交数据
				avgPriceStr, _ := status["avgPrice"].(string)
				executedQtyStr, _ := status["executedQty"].(string)
				updateTimeInt, _ := status["updateTime"].(int64)
				
				avgPrice, err := strconv.ParseFloat(avgPriceStr, 64)
				if err != nil {
					return nil, fmt.Errorf("无法解析成交价格 %s: %w", avgPriceStr, err)
				}
				
				executedQty, err := strconv.ParseFloat(executedQtyStr, 64)
				if err != nil {
					return nil, fmt.Errorf("无法解析成交数量 %s: %w", executedQtyStr, err)
				}
				
				fillTime := time.Unix(updateTimeInt/1000, 0)
				
				fillInfo := &FillInfo{
					AvgPrice:  avgPrice,
					FilledQty: executedQty,
					FillTime:  fillTime,
				}
				
				log.Printf("✅ [WaitForFill] 订单完全成交: 价格=%.6f, 数量=%.6f, 时间=%s", 
					avgPrice, executedQty, fillTime.Format("15:04:05.000"))
				
				return fillInfo, nil
			}
			
			if orderStatus == "CANCELED" || orderStatus == "EXPIRED" || orderStatus == "REJECTED" {
				return nil, fmt.Errorf("订单失败，状态: %s", orderStatus)
			}
			
			// 对于部分成交等状态，继续等待
		}
	}
	
	return nil, fmt.Errorf("订单在%v内未完全成交", timeout)
}

// OpenLongWithConfirmation 开多仓并等待成交确认
func (t *FuturesTrader) OpenLongWithConfirmation(symbol string, quantity float64, leverage int) (*OrderExecutionResult, error) {
	log.Printf("🚀 [OpenLongWithConfirmation] 开始确认性开多仓: %s, 数量=%.6f", symbol, quantity)
	
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 1. 创建市价买入订单
	log.Printf("🌐 [Binance API] 调用: NewCreateOrderService() - Symbol=%s, Side=BUY, PositionSide=LONG, Type=MARKET, Quantity=%s", symbol, quantityStr)
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("创建开多仓订单失败: %w", err)
	}

	log.Printf("📋 [OpenLongWithConfirmation] 订单已创建: OrderID=%d, 等待成交确认...", order.OrderID)

	// 2. 等待订单完全成交（最多等待30秒），但要处理部分成交情况
	fillInfo, err := t.waitForOrderFill(symbol, order.OrderID, 30*time.Second)
	if err != nil {
		// 🔥 关键修复：即使超时也要检查订单状态，防止数据丢失
		log.Printf("⚠️ [OpenLongWithConfirmation] 订单成交确认超时，检查最终状态...")
		
		finalStatus, statusErr := t.GetOrderStatus(symbol, order.OrderID)
		if statusErr == nil {
			if status, ok := finalStatus["status"].(string); ok {
				if status == "FILLED" {
					// 订单已成交，构建成交信息
					avgPriceStr, _ := finalStatus["avgPrice"].(string)
					executedQtyStr, _ := finalStatus["executedQty"].(string)
					updateTime, _ := finalStatus["updateTime"].(int64)
					
					avgPrice, _ := strconv.ParseFloat(avgPriceStr, 64)
					executedQty, _ := strconv.ParseFloat(executedQtyStr, 64)
					
					result := &OrderExecutionResult{
						OrderID:         order.OrderID,
						Symbol:          symbol,
						ActualFillPrice: avgPrice,
						ActualFillQty:   executedQty,
						FillTime:        time.Unix(updateTime/1000, 0),
						Status:          "FILLED",
					}
					
					log.Printf("✅ [OpenLongWithConfirmation] 超时后确认成交: 价格=%.6f, 数量=%.6f", avgPrice, executedQty)
					return result, nil
				} else if status == "PARTIALLY_FILLED" {
					// 🔥 关键：处理部分成交情况，记录实际成交部分
					avgPriceStr, _ := finalStatus["avgPrice"].(string)
					executedQtyStr, _ := finalStatus["executedQty"].(string)
					updateTime, _ := finalStatus["updateTime"].(int64)
					
					avgPrice, _ := strconv.ParseFloat(avgPriceStr, 64)
					executedQty, _ := strconv.ParseFloat(executedQtyStr, 64)
					
					if executedQty > 0 {
						result := &OrderExecutionResult{
							OrderID:         order.OrderID,
							Symbol:          symbol,
							ActualFillPrice: avgPrice,
							ActualFillQty:   executedQty,
							FillTime:        time.Unix(updateTime/1000, 0),
							Status:          "PARTIALLY_FILLED",
						}
						
						log.Printf("⚠️ [OpenLongWithConfirmation] 记录部分成交: 价格=%.6f, 数量=%.6f (部分成交)", avgPrice, executedQty)
						return result, nil
					}
				}
			}
		}
		
		return nil, fmt.Errorf("开多仓订单处理失败: %w", err)
	}

	// 3. 返回实际成交数据
	result := &OrderExecutionResult{
		OrderID:         order.OrderID,
		Symbol:          symbol,
		ActualFillPrice: fillInfo.AvgPrice,  // 🔥 关键：实际成交价
		ActualFillQty:   fillInfo.FilledQty, // 🔥 关键：实际成交量
		FillTime:        fillInfo.FillTime,  // 🔥 关键：实际成交时间
		Status:          "FILLED",
	}

	log.Printf("✅ [OpenLongWithConfirmation] 开多仓成功确认: 实际价格=%.6f, 实际数量=%.6f", 
		result.ActualFillPrice, result.ActualFillQty)

	return result, nil
}

// OpenShortWithConfirmation 开空仓并等待成交确认
func (t *FuturesTrader) OpenShortWithConfirmation(symbol string, quantity float64, leverage int) (*OrderExecutionResult, error) {
	log.Printf("🚀 [OpenShortWithConfirmation] 开始确认性开空仓: %s, 数量=%.6f", symbol, quantity)
	
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 1. 创建市价卖出订单
	log.Printf("🌐 [Binance API] 调用: NewCreateOrderService() - Symbol=%s, Side=SELL, PositionSide=SHORT, Type=MARKET, Quantity=%s", symbol, quantityStr)
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("创建开空仓订单失败: %w", err)
	}

	log.Printf("📋 [OpenShortWithConfirmation] 订单已创建: OrderID=%d, 等待成交确认...", order.OrderID)

	// 2. 等待订单完全成交（最多等待30秒），但要处理部分成交情况
	fillInfo, err := t.waitForOrderFill(symbol, order.OrderID, 30*time.Second)
	if err != nil {
		// 🔥 关键修复：即使超时也要检查订单状态，防止数据丢失
		log.Printf("⚠️ [OpenShortWithConfirmation] 订单成交确认超时，检查最终状态...")
		
		finalStatus, statusErr := t.GetOrderStatus(symbol, order.OrderID)
		if statusErr == nil {
			if status, ok := finalStatus["status"].(string); ok {
				if status == "FILLED" {
					// 订单已成交，构建成交信息
					avgPriceStr, _ := finalStatus["avgPrice"].(string)
					executedQtyStr, _ := finalStatus["executedQty"].(string)
					updateTime, _ := finalStatus["updateTime"].(int64)
					
					avgPrice, _ := strconv.ParseFloat(avgPriceStr, 64)
					executedQty, _ := strconv.ParseFloat(executedQtyStr, 64)
					
					result := &OrderExecutionResult{
						OrderID:         order.OrderID,
						Symbol:          symbol,
						ActualFillPrice: avgPrice,
						ActualFillQty:   executedQty,
						FillTime:        time.Unix(updateTime/1000, 0),
						Status:          "FILLED",
					}
					
					log.Printf("✅ [OpenShortWithConfirmation] 超时后确认成交: 价格=%.6f, 数量=%.6f", avgPrice, executedQty)
					return result, nil
				} else if status == "PARTIALLY_FILLED" {
					// 🔥 关键：处理部分成交情况，记录实际成交部分
					avgPriceStr, _ := finalStatus["avgPrice"].(string)
					executedQtyStr, _ := finalStatus["executedQty"].(string)
					updateTime, _ := finalStatus["updateTime"].(int64)
					
					avgPrice, _ := strconv.ParseFloat(avgPriceStr, 64)
					executedQty, _ := strconv.ParseFloat(executedQtyStr, 64)
					
					if executedQty > 0 {
						result := &OrderExecutionResult{
							OrderID:         order.OrderID,
							Symbol:          symbol,
							ActualFillPrice: avgPrice,
							ActualFillQty:   executedQty,
							FillTime:        time.Unix(updateTime/1000, 0),
							Status:          "PARTIALLY_FILLED",
						}
						
						log.Printf("⚠️ [OpenShortWithConfirmation] 记录部分成交: 价格=%.6f, 数量=%.6f (部分成交)", avgPrice, executedQty)
						return result, nil
					}
				}
			}
		}
		
		return nil, fmt.Errorf("开空仓订单处理失败: %w", err)
	}

	// 3. 返回实际成交数据
	result := &OrderExecutionResult{
		OrderID:         order.OrderID,
		Symbol:          symbol,
		ActualFillPrice: fillInfo.AvgPrice,  // 🔥 关键：实际成交价
		ActualFillQty:   fillInfo.FilledQty, // 🔥 关键：实际成交量
		FillTime:        fillInfo.FillTime,  // 🔥 关键：实际成交时间
		Status:          "FILLED",
	}

	log.Printf("✅ [OpenShortWithConfirmation] 开空仓成功确认: 实际价格=%.6f, 实际数量=%.6f", 
		result.ActualFillPrice, result.ActualFillQty)

	return result, nil
}

// GetExchangeOrderTime 从订单状态获取交易所时间戳
func (t *FuturesTrader) GetExchangeOrderTime(symbol string, orderID int64) (time.Time, error) {
	orderStatus, err := t.GetOrderStatus(symbol, orderID)
	if err != nil {
		return time.Time{}, fmt.Errorf("无法获取订单状态: %w", err)
	}
	
	// 从订单状态获取交易所时间戳
	updateTime, exists := orderStatus["updateTime"].(int64)
	if !exists {
		return time.Time{}, fmt.Errorf("无法获取订单时间戳")
	}
	
	exchangeTime := time.Unix(updateTime/1000, 0)
	log.Printf("🕐 [GetExchangeOrderTime] 订单时间: OrderID=%d, ExchangeTime=%s", orderID, exchangeTime.Format("15:04:05.000"))
	
	return exchangeTime, nil
}

// VerifyTradeDataConsistency 验证交易数据一致性
func (t *FuturesTrader) VerifyTradeDataConsistency(symbol string, tradeRecords []*TradeRecord) *DataConsistencyReport {
	log.Printf("🔍 [DataVerification] 开始验证交易数据一致性: %s (%d条记录)", symbol, len(tradeRecords))
	
	report := &DataConsistencyReport{
		Symbol:           symbol,
		TotalRecords:     len(tradeRecords),
		VerifiedRecords:  0,
		InconsistentRecords: make([]*InconsistentRecord, 0),
		StartTime:        time.Now(),
	}
	
	// 获取交易所的成交历史用于对比
	exchangeTrades, err := t.GetTradeHistory(symbol, 1000) // 获取最近1000条交易记录
	if err != nil {
		report.ErrorMessage = fmt.Sprintf("获取交易所历史数据失败: %v", err)
		report.EndTime = time.Now()
		return report
	}
	
	log.Printf("📊 [DataVerification] 获取到交易所历史数据: %d条", len(exchangeTrades))
	
	// 创建交易所数据映射（按订单ID索引）
	exchangeTradeMap := make(map[string]map[string]interface{})
	for _, trade := range exchangeTrades {
		if orderID, ok := trade["orderId"]; ok {
			exchangeTradeMap[fmt.Sprintf("%v", orderID)] = trade
		}
	}
	
	// 验证每条数据库记录
	for _, dbRecord := range tradeRecords {
		if dbRecord.Status != "closed" {
			continue // 只验证已关闭的交易
		}
		
		// 验证开仓订单
		if dbRecord.OpenOrderID != "" {
			if err := t.verifyOrderData(dbRecord, dbRecord.OpenOrderID, exchangeTradeMap, "open", report); err != nil {
				log.Printf("⚠️ [DataVerification] 验证开仓订单失败: %v", err)
			}
		}
		
		// 验证平仓订单
		if dbRecord.CloseOrderID != "" {
			if err := t.verifyOrderData(dbRecord, dbRecord.CloseOrderID, exchangeTradeMap, "close", report); err != nil {
				log.Printf("⚠️ [DataVerification] 验证平仓订单失败: %v", err)
			}
		}
		
		report.VerifiedRecords++
	}
	
	report.EndTime = time.Now()
	report.VerificationDuration = report.EndTime.Sub(report.StartTime)
	
	log.Printf("✅ [DataVerification] 验证完成: 总计%d条, 验证%d条, 不一致%d条, 耗时%v", 
		report.TotalRecords, report.VerifiedRecords, len(report.InconsistentRecords), report.VerificationDuration)
	
	return report
}

// verifyOrderData 验证单个订单数据
func (t *FuturesTrader) verifyOrderData(dbRecord *TradeRecord, orderID string, exchangeTradeMap map[string]map[string]interface{}, orderType string, report *DataConsistencyReport) error {
	exchangeTrade, exists := exchangeTradeMap[orderID]
	if !exists {
		// 交易所没有这个订单记录
		inconsistent := &InconsistentRecord{
			TradeID:     dbRecord.ID,
			OrderID:     orderID,
			OrderType:   orderType,
			Issue:       "order_not_found_in_exchange",
			Description: fmt.Sprintf("交易所未找到订单ID %s", orderID),
		}
		report.InconsistentRecords = append(report.InconsistentRecords, inconsistent)
		return nil
	}
	
	// 验证价格一致性
	exchangePriceStr, _ := exchangeTrade["price"].(string)
	exchangePrice, err := strconv.ParseFloat(exchangePriceStr, 64)
	if err == nil {
		var dbPrice float64
		if orderType == "open" {
			dbPrice = dbRecord.OpenPrice
		} else if orderType == "close" && dbRecord.ClosePrice != nil {
			dbPrice = *dbRecord.ClosePrice
		}
		
		// 允许微小的价格差异（0.01%以内）
		priceDiffPct := math.Abs(exchangePrice-dbPrice) / exchangePrice * 100
		if priceDiffPct > 0.01 { // 超过0.01%的差异视为不一致
			inconsistent := &InconsistentRecord{
				TradeID:      dbRecord.ID,
				OrderID:      orderID,
				OrderType:    orderType,
				Issue:        "price_mismatch",
				Description:  fmt.Sprintf("价格不一致: 数据库=%.6f, 交易所=%.6f, 差异=%.4f%%", dbPrice, exchangePrice, priceDiffPct),
				DBValue:      fmt.Sprintf("%.6f", dbPrice),
				ExchangeValue: fmt.Sprintf("%.6f", exchangePrice),
			}
			report.InconsistentRecords = append(report.InconsistentRecords, inconsistent)
		}
	}
	
	// 验证时间一致性
	exchangeTimeInt, _ := exchangeTrade["time"].(int64)
	exchangeTime := time.Unix(exchangeTimeInt/1000, 0)
	
	var dbTime time.Time
	if orderType == "open" {
		dbTime = dbRecord.OpenTime
	} else if orderType == "close" && dbRecord.CloseTime != nil {
		dbTime = *dbRecord.CloseTime
	}
	
	// 允许5秒的时间差异
	timeDiff := math.Abs(exchangeTime.Sub(dbTime).Seconds())
	if timeDiff > 5 {
		inconsistent := &InconsistentRecord{
			TradeID:      dbRecord.ID,
			OrderID:      orderID,
			OrderType:    orderType,
			Issue:        "time_mismatch",
			Description:  fmt.Sprintf("时间不一致: 数据库=%s, 交易所=%s, 差异=%.1f秒", dbTime.Format("15:04:05"), exchangeTime.Format("15:04:05"), timeDiff),
			DBValue:      dbTime.Format("2006-01-02 15:04:05"),
			ExchangeValue: exchangeTime.Format("2006-01-02 15:04:05"),
		}
		report.InconsistentRecords = append(report.InconsistentRecords, inconsistent)
	}
	
	return nil
}

// DataConsistencyReport 数据一致性验证报告
type DataConsistencyReport struct {
	Symbol               string                `json:"symbol"`
	TotalRecords         int                   `json:"total_records"`
	VerifiedRecords      int                   `json:"verified_records"`
	InconsistentRecords  []*InconsistentRecord `json:"inconsistent_records"`
	StartTime            time.Time             `json:"start_time"`
	EndTime              time.Time             `json:"end_time"`
	VerificationDuration time.Duration         `json:"verification_duration"`
	ErrorMessage         string                `json:"error_message,omitempty"`
}

// InconsistentRecord 不一致记录
type InconsistentRecord struct {
	TradeID       string `json:"trade_id"`
	OrderID       string `json:"order_id"`
	OrderType     string `json:"order_type"`     // "open" or "close"
	Issue         string `json:"issue"`          // "price_mismatch", "time_mismatch", "order_not_found_in_exchange"
	Description   string `json:"description"`
	DBValue       string `json:"db_value,omitempty"`
	ExchangeValue string `json:"exchange_value,omitempty"`
}

// ===== 权威数据获取方法（确保与交易所完全一致） =====

// GetOrderTrades 获取指定订单的真实成交明细
func (t *FuturesTrader) GetOrderTrades(symbol string, orderID int64) ([]OrderTradeDetail, error) {
	log.Printf("🔍 [GetOrderTrades] 查询订单成交明细: 币种=%s, 订单ID=%d", symbol, orderID)
	
	// 调用币安 GET /fapi/v1/userTrades API
	result, err := t.client.NewListAccountTradeService().Symbol(symbol).OrderID(orderID).Do(context.Background())
	if err != nil {
		log.Printf("❌ [GetOrderTrades] API调用失败: %v", err)
		return nil, fmt.Errorf("获取订单成交明细失败: %w", err)
	}
	
	var trades []OrderTradeDetail
	for _, trade := range result {
		// 解析并转换数据
		price, _ := strconv.ParseFloat(trade.Price, 64)
		qty, _ := strconv.ParseFloat(trade.Quantity, 64)
		quoteQty, _ := strconv.ParseFloat(trade.QuoteQuantity, 64)
		commission, _ := strconv.ParseFloat(trade.Commission, 64)
		realizedPnl, _ := strconv.ParseFloat(trade.RealizedPnl, 64)
		
		tradeDetail := OrderTradeDetail{
			TradeID:         trade.ID,
			OrderID:         trade.OrderID,
			Symbol:          trade.Symbol,
			Side:            getSideFromBuyer(trade.Buyer),
			PositionSide:    string(trade.PositionSide),
			Price:           price,
			Quantity:        qty,
			QuoteQty:        quoteQty,
			Commission:      commission,
			CommissionAsset: trade.CommissionAsset,
			RealizedPnL:     realizedPnl,
			IsMaker:         trade.Maker,
			TradeTime:       time.Unix(trade.Time/1000, 0),
		}
		
		trades = append(trades, tradeDetail)
	}
	
	log.Printf("✅ [GetOrderTrades] 查询成功: 找到 %d 条成交记录", len(trades))
	return trades, nil
}

// GetRecentTrades 获取最近的成交历史
func (t *FuturesTrader) GetRecentTrades(symbol string, limit int) ([]OrderTradeDetail, error) {
	log.Printf("🔍 [GetRecentTrades] 查询最近成交历史: 币种=%s, 数量=%d", symbol, limit)
	
	if limit <= 0 || limit > 500 {
		limit = 500 // 币安API限制
	}
	
	// 调用币安 GET /fapi/v1/userTrades API
	result, err := t.client.NewListAccountTradeService().Symbol(symbol).Limit(limit).Do(context.Background())
	if err != nil {
		log.Printf("❌ [GetRecentTrades] API调用失败: %v", err)
		return nil, fmt.Errorf("获取成交历史失败: %w", err)
	}
	
	var trades []OrderTradeDetail
	for _, trade := range result {
		// 解析并转换数据（与GetOrderTrades相同的逻辑）
		price, _ := strconv.ParseFloat(trade.Price, 64)
		qty, _ := strconv.ParseFloat(trade.Quantity, 64)
		quoteQty, _ := strconv.ParseFloat(trade.QuoteQuantity, 64)
		commission, _ := strconv.ParseFloat(trade.Commission, 64)
		realizedPnl, _ := strconv.ParseFloat(trade.RealizedPnl, 64)
		
		tradeDetail := OrderTradeDetail{
			TradeID:         trade.ID,
			OrderID:         trade.OrderID,
			Symbol:          trade.Symbol,
			Side:            getSideFromBuyer(trade.Buyer),
			PositionSide:    string(trade.PositionSide),
			Price:           price,
			Quantity:        qty,
			QuoteQty:        quoteQty,
			Commission:      commission,
			CommissionAsset: trade.CommissionAsset,
			RealizedPnL:     realizedPnl,
			IsMaker:         trade.Maker,
			TradeTime:       time.Unix(trade.Time/1000, 0),
		}
		
		trades = append(trades, tradeDetail)
	}
	
	log.Printf("✅ [GetRecentTrades] 查询成功: 找到 %d 条成交记录", len(trades))
	return trades, nil
}

// GetIncomeHistory 获取资金流水历史
func (t *FuturesTrader) GetIncomeHistory(symbol string, incomeType string, limit int) ([]IncomeRecord, error) {
	log.Printf("🔍 [GetIncomeHistory] 查询资金流水: 币种=%s, 类型=%s, 数量=%d", symbol, incomeType, limit)
	
	if limit <= 0 || limit > 1000 {
		limit = 100 // 默认限制
	}
	
	// 构建查询参数
	service := t.client.NewGetIncomeHistoryService()
	if symbol != "" {
		service = service.Symbol(symbol)
	}
	if incomeType != "" {
		service = service.IncomeType(incomeType)
	}
	service = service.Limit(int64(limit))
	
	// 调用币安 GET /fapi/v1/income API
	result, err := service.Do(context.Background())
	if err != nil {
		log.Printf("❌ [GetIncomeHistory] API调用失败: %v", err)
		return nil, fmt.Errorf("获取资金流水失败: %w", err)
	}
	
	var incomes []IncomeRecord
	for _, income := range result {
		// 解析并转换数据
		incomeAmount, _ := strconv.ParseFloat(income.Income, 64)
		
		incomeRecord := IncomeRecord{
			Symbol:     income.Symbol,
			IncomeType: income.IncomeType,
			Income:     incomeAmount,
			Asset:      income.Asset,
			Info:       income.Info,
			Time:       time.Unix(income.Time/1000, 0),
			TranID:     income.TranID,
			TradeID:    income.TradeID,
		}
		
		incomes = append(incomes, incomeRecord)
	}
	
	log.Printf("✅ [GetIncomeHistory] 查询成功: 找到 %d 条资金流水记录", len(incomes))
	return incomes, nil
}

// getSideFromBuyer 根据isBuyer字段转换为标准的Side
func getSideFromBuyer(isBuyer bool) string {
	if isBuyer {
		return "BUY"
	}
	return "SELL"
}

// GetAuthoritativeCloseData 获取权威的平仓数据（核心方法）
func (t *FuturesTrader) GetAuthoritativeCloseData(symbol, positionSide string, closeOrderID string) (*AuthoritativeCloseData, error) {
	log.Printf("🎯 [GetAuthoritativeCloseData] 获取权威平仓数据: %s %s, 订单ID=%s", symbol, positionSide, closeOrderID)
	
	// 方法1: 如果有订单ID，优先查询该订单的成交明细
	if closeOrderID != "" && closeOrderID != "unknown" {
		if orderID, err := strconv.ParseInt(closeOrderID, 10, 64); err == nil {
			trades, err := t.GetOrderTrades(symbol, orderID)
			if err == nil && len(trades) > 0 {
				return t.aggregateOrderTrades(trades, positionSide, "ORDER_TRADES")
			}
			log.Printf("⚠️ [GetAuthoritativeCloseData] 订单成交查询失败，尝试其他方法: %v", err)
		}
	}
	
	// 方法2: 查询最近成交历史并匹配平仓交易
	trades, err := t.GetRecentTrades(symbol, 50)
	if err == nil {
		if closeData := t.findMatchingCloseTrade(trades, symbol, positionSide); closeData != nil {
			return closeData, nil
		}
		log.Printf("⚠️ [GetAuthoritativeCloseData] 未在成交历史中找到匹配的平仓交易")
	}
	
	// 方法3: 查询资金流水获取真实已实现盈亏
	incomes, err := t.GetIncomeHistory(symbol, "REALIZED_PNL", 20)
	if err == nil && len(incomes) > 0 {
		return t.deriveFromIncomeHistory(incomes, symbol, positionSide)
	}
	
	// 如果所有方法都失败，拒绝返回估算数据
	return nil, fmt.Errorf("无法获取 %s %s 的权威平仓数据，拒绝使用估算数据", symbol, positionSide)
}

// aggregateOrderTrades 聚合订单成交数据为权威平仓数据
func (t *FuturesTrader) aggregateOrderTrades(trades []OrderTradeDetail, positionSide, dataSource string) (*AuthoritativeCloseData, error) {
	if len(trades) == 0 {
		return nil, fmt.Errorf("无成交记录")
	}
	
	// 计算加权平均价格
	totalQty := 0.0
	totalValue := 0.0
	totalCommission := 0.0
	totalRealizedPnL := 0.0
	latestTime := trades[0].TradeTime
	
	for _, trade := range trades {
		totalQty += trade.Quantity
		totalValue += trade.QuoteQty
		totalCommission += trade.Commission
		totalRealizedPnL += trade.RealizedPnL
		
		if trade.TradeTime.After(latestTime) {
			latestTime = trade.TradeTime
		}
	}
	
	avgPrice := totalValue / totalQty
	
	log.Printf("✅ [aggregateOrderTrades] 聚合完成: 平均价格=%.6f, 总数量=%.6f, 总盈亏=%.2f", 
		avgPrice, totalQty, totalRealizedPnL)
	
	return &AuthoritativeCloseData{
		ActualPrice:    avgPrice,
		ActualTime:     latestTime,
		ActualQuantity: totalQty,
		ActualPnL:      totalRealizedPnL,
		ActualPnLPct:   0, // 需要根据开仓成本计算
		Duration:       0, // 需要根据开仓时间计算
		Commission:     totalCommission,
		DataSource:     dataSource,
		OrderID:        fmt.Sprintf("%d", trades[0].OrderID),
	}, nil
}

// findMatchingCloseTrade 在成交历史中查找匹配的平仓交易
func (t *FuturesTrader) findMatchingCloseTrade(trades []OrderTradeDetail, symbol, positionSide string) *AuthoritativeCloseData {
	// 🎯 修复 P0：归一化 positionSide，确保兼容 long/short 输入
	positionSide = NormalizePositionSide(positionSide)
	
	log.Printf("🔍 [findMatchingCloseTrade] 在 %d 条成交记录中查找 %s %s 的平仓交易", 
		len(trades), symbol, positionSide)
	
	// 查找最近的减仓交易（可能是平仓）
	for _, trade := range trades {
		if trade.Symbol != symbol {
			continue
		}
		
		// 根据持仓方向判断是否为平仓交易
		// LONG持仓 -> SELL为平仓, SHORT持仓 -> BUY为平仓
		isCloseTrade := false
		if positionSide == "LONG" && trade.Side == "SELL" {
			isCloseTrade = true
		} else if positionSide == "SHORT" && trade.Side == "BUY" {
			isCloseTrade = true
		}

		// 🔧 增强：增加时间窗口过滤，只匹配最近30秒内的交易，避免误匹配
		if isCloseTrade && trade.RealizedPnL != 0 {
			// 检查交易时间是否在30秒内
			timeSinceTradeSeconds := time.Since(trade.TradeTime).Seconds()
			if timeSinceTradeSeconds > 30 {
				log.Printf("  ⏰ [findMatchingCloseTrade] 跳过旧交易: TradeID=%d, 时间差=%.1f秒 (超过30秒阈值)",
					trade.TradeID, timeSinceTradeSeconds)
				continue
			}

			log.Printf("✅ [findMatchingCloseTrade] 找到匹配的平仓交易: TradeID=%d, 价格=%.6f, 盈亏=%.2f, 时间差=%.1f秒",
				trade.TradeID, trade.Price, trade.RealizedPnL, timeSinceTradeSeconds)
			
			return &AuthoritativeCloseData{
				ActualPrice:    trade.Price,
				ActualTime:     trade.TradeTime,
				ActualQuantity: trade.Quantity,
				ActualPnL:      trade.RealizedPnL,
				ActualPnLPct:   0,
				Duration:       0,
				Commission:     trade.Commission,
				DataSource:     "RECENT_TRADES_MATCH",
				OrderID:        fmt.Sprintf("%d", trade.OrderID),
			}
		}
	}
	
	log.Printf("⚠️ [findMatchingCloseTrade] 未找到匹配的平仓交易")
	return nil
}

// deriveFromIncomeHistory 从资金流水推导平仓数据
func (t *FuturesTrader) deriveFromIncomeHistory(incomes []IncomeRecord, symbol, positionSide string) (*AuthoritativeCloseData, error) {
	log.Printf("🔍 [deriveFromIncomeHistory] 从 %d 条资金流水中推导平仓数据", len(incomes))
	positionSide = NormalizePositionSide(positionSide)
	
	// 找到最近的已实现盈亏记录
	for _, income := range incomes {
		if income.Symbol != symbol || income.IncomeType != "REALIZED_PNL" || income.Income == 0 {
			continue
		}

		// 🎯 增强：优先通过 TradeID 反查成交，补齐价格/数量
		if income.TradeID != "" {
			if tradeID, err := strconv.ParseInt(income.TradeID, 10, 64); err == nil {
				trades, err := t.GetRecentTrades(symbol, 200)
				if err == nil {
					for _, tr := range trades {
						if tr.Symbol != symbol {
							continue
						}
						if tr.TradeID != tradeID {
							continue
						}

						// 通过 trade side 推断是否为平仓成交（兜底校验）
						isClose := false
						if positionSide == "LONG" && tr.Side == "SELL" {
							isClose = true
						} else if positionSide == "SHORT" && tr.Side == "BUY" {
							isClose = true
						}

						if isClose {
							log.Printf("✅ [deriveFromIncomeHistory] TradeID 命中成交: tradeID=%d, price=%.6f, qty=%.6f", tr.TradeID, tr.Price, tr.Quantity)
							return &AuthoritativeCloseData{
								ActualPrice:    tr.Price,
								ActualTime:     income.Time, // 以 income 时间作为平仓时间锚点
								ActualQuantity: tr.Quantity,
								ActualPnL:      income.Income,
								ActualPnLPct:   0,
								Duration:       0,
								Commission:     tr.Commission,
								DataSource:     "INCOME_HISTORY+TRADE_MATCH",
								OrderID:        fmt.Sprintf("%d", tr.OrderID),
							}, nil
						}
					}
				}
			}
		}

		// 退化：只有 PnL（无价格）
		log.Printf("✅ [deriveFromIncomeHistory] 找到已实现盈亏记录: %.2f USDT, 时间=%s (无成交价格)", income.Income, income.Time.Format("15:04:05"))
		return &AuthoritativeCloseData{
			ActualPrice:    0, // 无法从Income获取价格，但不是估算
			ActualTime:     income.Time,
			ActualQuantity: 0, // 无法从Income获取数量
			ActualPnL:      income.Income,
			ActualPnLPct:   0,
			Duration:       0,
			Commission:     0,
			DataSource:     "INCOME_HISTORY",
			OrderID:        income.TradeID,
		}, nil
	}

	return nil, fmt.Errorf("未在资金流水中找到相关的已实现盈亏记录")
}

// GetAuthoritativeOpenData 获取权威的开仓数据（确保与交易所记录一致）
// 通过订单ID查询真实成交明细，计算加权平均开仓价
func (t *FuturesTrader) GetAuthoritativeOpenData(symbol string, orderID int64) (*AuthoritativeOpenData, error) {
	log.Printf("🎯 [GetAuthoritativeOpenData] 获取权威开仓数据: 币种=%s, 订单ID=%d", symbol, orderID)

	// 方法1: 通过订单ID查询成交明细（最准确）
	if orderID > 0 {
		trades, err := t.GetOrderTrades(symbol, orderID)
		if err == nil && len(trades) > 0 {
			return t.aggregateOpenOrderTrades(trades, "ORDER_TRADES")
		}
		log.Printf("⚠️ [GetAuthoritativeOpenData] 订单成交查询失败，尝试其他方法: %v", err)
	}

	// 方法2: 从最近成交历史中查找（fallback）
	trades, err := t.GetRecentTrades(symbol, 50)
	if err == nil {
		if openData := t.findMatchingOpenTrade(trades, symbol, orderID); openData != nil {
			return openData, nil
		}
		log.Printf("⚠️ [GetAuthoritativeOpenData] 未在成交历史中找到匹配的开仓交易")
	}

	// 方法3: 从持仓信息中获取（最后的fallback，准确度较低）
	log.Printf("⚠️ [GetAuthoritativeOpenData] 所有方法失败，尝试从持仓信息获取")
	positions, err := t.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				if entryPriceStr, ok := pos["entryPrice"].(string); ok {
					if entryPrice, err := strconv.ParseFloat(entryPriceStr, 64); err == nil && entryPrice > 0 {
						log.Printf("✅ [GetAuthoritativeOpenData] 从持仓信息获取开仓价: %.6f (准确度较低)", entryPrice)
						return &AuthoritativeOpenData{
							ActualPrice:    entryPrice,
							ActualTime:     time.Now(),
							ActualQuantity: 0, // 从持仓无法获取准确的开仓数量
							Commission:     0,
							DataSource:     "POSITION_ENTRY_PRICE",
							OrderID:        fmt.Sprintf("%d", orderID),
							IsMaker:        false,
						}, nil
					}
				}
			}
		}
	}

	// 所有方法都失败
	return nil, fmt.Errorf("无法获取 %s 订单 %d 的权威开仓数据", symbol, orderID)
}

// aggregateOpenOrderTrades 聚合开仓订单成交数据为权威开仓数据
func (t *FuturesTrader) aggregateOpenOrderTrades(trades []OrderTradeDetail, dataSource string) (*AuthoritativeOpenData, error) {
	if len(trades) == 0 {
		return nil, fmt.Errorf("无成交记录")
	}

	// 计算加权平均价格
	totalQty := 0.0
	totalValue := 0.0
	totalCommission := 0.0
	latestTime := trades[0].TradeTime
	isMaker := trades[0].IsMaker
	commissionAsset := trades[0].CommissionAsset

	for _, trade := range trades {
		totalQty += trade.Quantity
		totalValue += trade.QuoteQty
		totalCommission += trade.Commission

		if trade.TradeTime.After(latestTime) {
			latestTime = trade.TradeTime
		}
	}

	avgPrice := totalValue / totalQty

	log.Printf("✅ [aggregateOpenOrderTrades] 聚合完成: 平均开仓价=%.6f, 总数量=%.6f, 手续费=%.4f %s",
		avgPrice, totalQty, totalCommission, commissionAsset)

	return &AuthoritativeOpenData{
		ActualPrice:     avgPrice,
		ActualTime:      latestTime,
		ActualQuantity:  totalQty,
		Commission:      totalCommission,
		CommissionAsset: commissionAsset,
		DataSource:      dataSource,
		OrderID:         fmt.Sprintf("%d", trades[0].OrderID),
		IsMaker:         isMaker,
	}, nil
}

// findMatchingOpenTrade 在成交历史中查找匹配的开仓交易
func (t *FuturesTrader) findMatchingOpenTrade(trades []OrderTradeDetail, symbol string, orderID int64) *AuthoritativeOpenData {
	log.Printf("🔍 [findMatchingOpenTrade] 在 %d 条成交记录中查找 %s 订单 %d 的开仓交易",
		len(trades), symbol, orderID)

	// 优先通过订单ID精确匹配
	if orderID > 0 {
		for _, trade := range trades {
			if trade.Symbol == symbol && trade.OrderID == orderID {
				log.Printf("✅ [findMatchingOpenTrade] 通过订单ID精确匹配: TradeID=%d, 价格=%.6f",
					trade.TradeID, trade.Price)

				return &AuthoritativeOpenData{
					ActualPrice:     trade.Price,
					ActualTime:      trade.TradeTime,
					ActualQuantity:  trade.Quantity,
					Commission:      trade.Commission,
					CommissionAsset: trade.CommissionAsset,
					DataSource:      "RECENT_TRADES_ORDER_ID_MATCH",
					OrderID:         fmt.Sprintf("%d", trade.OrderID),
					IsMaker:         trade.IsMaker,
				}
			}
		}
	}

	// 如果订单ID匹配失败，尝试找最近的开仓交易（30秒内）
	for _, trade := range trades {
		if trade.Symbol != symbol {
			continue
		}

		// 只匹配最近30秒内的交易，避免误匹配
		if time.Since(trade.TradeTime) > 30*time.Second {
			continue
		}

		// 开仓交易的realizedPnL应该为0
		if trade.RealizedPnL == 0 {
			log.Printf("✅ [findMatchingOpenTrade] 找到最近的开仓交易: TradeID=%d, 价格=%.6f, 时间=%s",
				trade.TradeID, trade.Price, trade.TradeTime.Format("15:04:05"))

			return &AuthoritativeOpenData{
				ActualPrice:     trade.Price,
				ActualTime:      trade.TradeTime,
				ActualQuantity:  trade.Quantity,
				Commission:      trade.Commission,
				CommissionAsset: trade.CommissionAsset,
				DataSource:      "RECENT_TRADES_TIME_MATCH",
				OrderID:         fmt.Sprintf("%d", trade.OrderID),
				IsMaker:         trade.IsMaker,
			}
		}
	}

	log.Printf("⚠️ [findMatchingOpenTrade] 未找到匹配的开仓交易")
	return nil
}
