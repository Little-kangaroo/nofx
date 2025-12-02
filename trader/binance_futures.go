package trader

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

// FuturesTrader 币安合约交易器
type FuturesTrader struct {
	client *futures.Client

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

// SetStopLoss 设置止损单
func (t *FuturesTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) (int64, error) {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// 🔧 修复精度问题：使用正确的价格精度格式化止损价格
	formattedStopPrice, err := t.FormatPrice(symbol, stopPrice)
	if err != nil {
		log.Printf("⚠️ 获取价格精度失败，使用默认格式: %v", err)
		formattedStopPrice = fmt.Sprintf("%.2f", stopPrice)
	}
	
	log.Printf("🔧 止损价格格式化: 原始=%.8f, 格式化=%s", stopPrice, formattedStopPrice)

	// 🔧 修复止损订单：使用ClosePosition时不需要设置Quantity
	// ClosePosition(true) 会自动平掉整个仓位，quantity参数会被忽略
	response, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.OrderTypeStopMarket).
		StopPrice(formattedStopPrice).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		Do(context.Background())

	if err != nil {
		return 0, fmt.Errorf("设置止损失败: %w", err)
	}

	log.Printf("  ✅ 止损价设置成功: %s", formattedStopPrice)
	return response.OrderID, nil
}

// SetTakeProfit 设置止盈单
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

// GetOpenOrders 获取指定币种的所有挂单
func (t *FuturesTrader) GetOpenOrders(symbol string) ([]map[string]interface{}, error) {
	log.Printf("🔍 [Binance] 查询 %s 的挂单...", symbol)
	
	// 调用币安API获取挂单
	orders, err := t.client.NewListOpenOrdersService().Symbol(symbol).Do(context.Background())
	if err != nil {
		log.Printf("❌ [Binance] 获取挂单失败: %v", err)
		return nil, fmt.Errorf("获取挂单失败: %w", err)
	}
	
	log.Printf("📋 [Binance] %s 找到 %d 个挂单", symbol, len(orders))
	
	// 转换为统一格式
	result := make([]map[string]interface{}, len(orders))
	for i, order := range orders {
		result[i] = map[string]interface{}{
			"symbol":       order.Symbol,
			"orderId":      order.OrderID,
			"type":         string(order.Type),
			"side":         string(order.Side),
			"quantity":     order.OrigQuantity,
			"price":        order.Price,
			"stopPrice":    order.StopPrice,
			"status":       string(order.Status),
			"timeInForce":  string(order.TimeInForce),
			"reduceOnly":   order.ReduceOnly,
			"positionSide": string(order.PositionSide),
		}
		
		log.Printf("  📄 订单#%d: %s %s %s 数量:%s 价格:%s 止损价:%s", 
			order.OrderID, order.Type, order.Side, order.PositionSide, 
			order.OrigQuantity, order.Price, order.StopPrice)
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
