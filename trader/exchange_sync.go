package trader

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"time"
	"nofx/config"
	"nofx/market"
)

// FillDetails 成交详情结构
type FillDetails struct {
	OrderID         int64     `json:"order_id"`
	Symbol          string    `json:"symbol"`
	Side            string    `json:"side"`
	AveragePrice    float64   `json:"average_price"`    // 加权平均成交价
	ExecutedQty     float64   `json:"executed_qty"`     // 实际成交数量
	TotalFills      int       `json:"total_fills"`      // 成交次数
	Commission      float64   `json:"commission"`       // 手续费
	CommissionAsset string    `json:"commission_asset"` // 手续费币种
	FillTime        int64     `json:"fill_time"`        // 成交时间
	Status          string    `json:"status"`           // 订单状态
}

// PositionDifference 持仓差异结构
type PositionDifference struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`
	DatabaseQuantity float64 `json:"db_quantity"`
	ExchangeQuantity float64 `json:"exchange_quantity"`
	Difference       float64 `json:"difference"`
	DifferenceType   string  `json:"difference_type"` // "missing_position", "extra_position", "quantity_mismatch"
}

// ExchangeRecordSync 交易所记录同步器
type ExchangeRecordSync struct {
	trader   Trader
	database *config.Database
	traderID string
}

// NewExchangeRecordSync 创建新的交易所记录同步器
func NewExchangeRecordSync(trader Trader, database *config.Database, traderID string) *ExchangeRecordSync {
	return &ExchangeRecordSync{
		trader:   trader,
		database: database,
		traderID: traderID,
	}
}

// ===== Phase 1: 核心修复 - 增强版成交价格获取 =====

// GetEnhancedFillPrice 增强版成交价格获取 - 支持订单ID查询真实成交
func (sync *ExchangeRecordSync) GetEnhancedFillPrice(orderID int64, symbol, side string) (*FillDetails, error) {
	log.Printf("🔍 [Enhanced Fill] 开始获取增强成交信息: 订单ID=%d, 币种=%s, 方向=%s", 
		orderID, symbol, side)
	
	// 1. 优先通过订单ID查询精确成交信息
	if orderID > 0 {
		orderStatus, err := sync.trader.GetOrderStatus(symbol, orderID)
		if err == nil {
			// 解析订单成交信息
			if fillDetails := sync.parseOrderFillDetails(orderStatus, orderID, symbol, side); fillDetails != nil {
				log.Printf("✅ [Enhanced Fill] 通过订单ID获取成交信息成功: 成交价=%.6f, 成交量=%.6f", 
					fillDetails.AveragePrice, fillDetails.ExecutedQty)
				return fillDetails, nil
			}
		} else {
			log.Printf("⚠️ [Enhanced Fill] 订单状态查询失败: %v", err)
		}
	}
	
	// 2. 降级到持仓信息查询（兼容现有逻辑）
	log.Printf("🔄 [Enhanced Fill] 降级使用持仓信息查询...")
	price := sync.getActualFillPriceFromPosition(symbol, side)
	if price > 0 {
		// 构造FillDetails结构（部分信息从持仓推断）
		fillDetails := &FillDetails{
			OrderID:      orderID,
			Symbol:       symbol,
			Side:         side,
			AveragePrice: price,
			ExecutedQty:  0, // 无法从持仓信息获取准确数量
			FillTime:     time.Now().UnixMilli(),
			Status:       "FILLED",
		}
		
		// 尝试从持仓信息获取数量
		if quantity := sync.getPositionQuantity(symbol, side); quantity > 0 {
			fillDetails.ExecutedQty = quantity
		}
		
		log.Printf("✅ [Enhanced Fill] 通过持仓信息获取成交价格: %.6f", price)
		return fillDetails, nil
	}
	
	// 3. 最后降级：使用市场价格（标记为估算）
	log.Printf("⚠️ [Enhanced Fill] 无法获取精确成交信息，使用市场价格估算")
	marketData, err := market.Get(symbol)
	if err != nil {
		return nil, fmt.Errorf("无法获取市场价格: %w", err)
	}
	
	return &FillDetails{
		OrderID:      orderID,
		Symbol:       symbol,
		Side:         side,
		AveragePrice: marketData.CurrentPrice,
		ExecutedQty:  0,
		FillTime:     time.Now().UnixMilli(),
		Status:       "ESTIMATED", // 标记为估算值
	}, nil
}

// parseOrderFillDetails 解析订单成交详情
func (sync *ExchangeRecordSync) parseOrderFillDetails(orderStatus map[string]interface{}, orderID int64, symbol, side string) *FillDetails {
	// 解析订单状态
	status, _ := orderStatus["status"].(string)
	if status != "FILLED" && status != "PARTIALLY_FILLED" {
		return nil
	}
	
	// 解析成���价格
	var avgPrice float64
	if avgPriceStr, ok := orderStatus["avgPrice"].(string); ok && avgPriceStr != "" && avgPriceStr != "0" {
		if price, err := strconv.ParseFloat(avgPriceStr, 64); err == nil {
			avgPrice = price
		}
	}
	
	// 解析成交数量
	var executedQty float64
	if executedQtyStr, ok := orderStatus["executedQty"].(string); ok && executedQtyStr != "" {
		if qty, err := strconv.ParseFloat(executedQtyStr, 64); err == nil {
			executedQty = qty
		}
	}
	
	// 解析成交时间
	var updateTime int64
	if updateTimeValue, ok := orderStatus["updateTime"]; ok {
		if updateTimeInt, ok := updateTimeValue.(int64); ok {
			updateTime = updateTimeInt
		} else if updateTimeFloat, ok := updateTimeValue.(float64); ok {
			updateTime = int64(updateTimeFloat)
		}
	}
	
	// 解析手续费
	var commission float64
	var commissionAsset string
	if commissionValue, ok := orderStatus["commission"].(string); ok && commissionValue != "" {
		commission, _ = strconv.ParseFloat(commissionValue, 64)
	}
	if commissionAssetValue, ok := orderStatus["commissionAsset"].(string); ok {
		commissionAsset = commissionAssetValue
	}
	
	// 验证关键数据
	if avgPrice <= 0 {
		log.Printf("⚠️ [Parse Fill] 订单成交价格无效: %v", orderStatus["avgPrice"])
		return nil
	}
	
	fillDetails := &FillDetails{
		OrderID:         orderID,
		Symbol:          symbol,
		Side:            side,
		AveragePrice:    avgPrice,
		ExecutedQty:     executedQty,
		Commission:      commission,
		CommissionAsset: commissionAsset,
		FillTime:        updateTime,
		Status:          status,
	}
	
	log.Printf("📊 [Parse Fill] 解析成交详情: 价格=%.6f, 数量=%.6f, 手续费=%.6f %s", 
		avgPrice, executedQty, commission, commissionAsset)
	
	return fillDetails
}

// getActualFillPriceFromPosition 从持仓信息获取实际成交价格
func (sync *ExchangeRecordSync) getActualFillPriceFromPosition(symbol, side string) float64 {
	positions, err := sync.trader.GetPositions()
	if err != nil {
		log.Printf("  ⚠️ 获取持仓信息失败: %v", err)
		return 0
	}

	for _, pos := range positions {
		if pos["symbol"] == symbol && pos["side"] == side {
			if entryPrice, ok := pos["entryPrice"].(float64); ok && entryPrice > 0 {
				return entryPrice
			}
			if entryPrice, ok := pos["entryPrice"].(string); ok {
				if price, err := strconv.ParseFloat(entryPrice, 64); err == nil && price > 0 {
					return price
				}
			}
		}
	}
	
	return 0
}

// getPositionQuantity 从持仓信息获取数量
func (sync *ExchangeRecordSync) getPositionQuantity(symbol, side string) float64 {
	positions, err := sync.trader.GetPositions()
	if err != nil {
		return 0
	}
	
	for _, pos := range positions {
		if pos["symbol"] == symbol && pos["side"] == side {
			if quantity, ok := pos["positionAmt"].(float64); ok {
				if quantity < 0 {
					return -quantity // 空仓数量为负，返回绝对值
				}
				return quantity
			}
		}
	}
	return 0
}

// ===== Phase 1: 核心修复 - 基本持仓差异检测 =====

// DetectPositionDifferences 检测持仓差异
func (sync *ExchangeRecordSync) DetectPositionDifferences() ([]*PositionDifference, error) {
	log.Printf("🔍 [Position Diff] 开始检测持仓差异...")
	
	// 1. 获取数据库中的开仓记录
	dbPositions, err := sync.getDatabaseOpenPositions()
	if err != nil {
		return nil, fmt.Errorf("获取数据库开仓记录失败: %w", err)
	}
	
	// 2. 获取交易所实际持仓
	exchangePositions, err := sync.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取交易所持仓失败: %w", err)
	}
	
	log.Printf("📊 [Position Diff] 数据库开仓记录: %d个, 交易所持仓: %d个", 
		len(dbPositions), len(exchangePositions))
	
	var differences []*PositionDifference
	
	// 3. 建立交易所持仓映射
	exchangeMap := make(map[string]float64) // key: symbol_side, value: quantity
	for _, pos := range exchangePositions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		key := fmt.Sprintf("%s_%s", symbol, side)
		exchangeMap[key] = quantity
	}
	
	// 4. 检查数据库记录与交易所的差异
	for _, dbPos := range dbPositions {
		key := fmt.Sprintf("%s_%s", dbPos.Symbol, dbPos.Side)
		exchangeQuantity, exists := exchangeMap[key]
		
		if !exists {
			// 数据库有记录但交易所没有持仓
			diff := &PositionDifference{
				Symbol:           dbPos.Symbol,
				Side:             dbPos.Side,
				DatabaseQuantity: dbPos.Quantity,
				ExchangeQuantity: 0,
				Difference:       dbPos.Quantity,
				DifferenceType:   "missing_position",
			}
			differences = append(differences, diff)
			log.Printf("❗ [Position Diff] 发现缺失持仓: %s %s DB=%.6f Exchange=0", 
				dbPos.Symbol, dbPos.Side, dbPos.Quantity)
		} else {
			// 检查数量是否匹配
			if math.Abs(exchangeQuantity-dbPos.Quantity) > 0.0001 {
				diff := &PositionDifference{
					Symbol:           dbPos.Symbol,
					Side:             dbPos.Side,
					DatabaseQuantity: dbPos.Quantity,
					ExchangeQuantity: exchangeQuantity,
					Difference:       dbPos.Quantity - exchangeQuantity,
					DifferenceType:   "quantity_mismatch",
				}
				differences = append(differences, diff)
				log.Printf("⚠️ [Position Diff] 发现数量差异: %s %s DB=%.6f Exchange=%.6f", 
					dbPos.Symbol, dbPos.Side, dbPos.Quantity, exchangeQuantity)
			}
		}
		
		// 从映射中移除已检查的持仓
		delete(exchangeMap, key)
	}
	
	// 5. 检查交易所有但数据库没有的持仓
	for key, quantity := range exchangeMap {
		parts := splitSymbolSide(key)
		if len(parts) == 2 {
			diff := &PositionDifference{
				Symbol:           parts[0],
				Side:             parts[1],
				DatabaseQuantity: 0,
				ExchangeQuantity: quantity,
				Difference:       -quantity,
				DifferenceType:   "extra_position",
			}
			differences = append(differences, diff)
			log.Printf("🆕 [Position Diff] 发现额外持仓: %s %s DB=0 Exchange=%.6f", 
				parts[0], parts[1], quantity)
		}
	}
	
	log.Printf("📋 [Position Diff] 检测完成: 发现%d个差异", len(differences))
	return differences, nil
}

// getDatabaseOpenPositions 获取数据库中的开仓记录
func (sync *ExchangeRecordSync) getDatabaseOpenPositions() ([]*config.TradeRecord, error) {
	if sync.database == nil {
		return nil, fmt.Errorf("数据库连接不可用")
	}
	
	// 获取所有交易记录
	allTrades, err := sync.database.GetTraderTrades(sync.traderID, 0)
	if err != nil {
		return nil, err
	}
	
	// 过滤出开仓状态的记录
	var openPositions []*config.TradeRecord
	for _, trade := range allTrades {
		if trade.Status == "open" {
			openPositions = append(openPositions, trade)
		}
	}
	
	return openPositions, nil
}

// splitSymbolSide 分割symbol_side字符串
func splitSymbolSide(key string) []string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '_' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return []string{}
}

// ===== Phase 1: 核心修复 - 自动修复机制 =====

// AutoFixPositionDifferences 自动修复持仓差异
func (sync *ExchangeRecordSync) AutoFixPositionDifferences(differences []*PositionDifference) error {
	log.Printf("🔧 [Auto Fix] 开始自动修复%d个持仓差异...", len(differences))
	
	fixedCount := 0
	for _, diff := range differences {
		switch diff.DifferenceType {
		case "missing_position":
			// 数据库有记录但交易所没有持仓 - 更新数据库为已平仓
			if err := sync.fixMissingPosition(diff); err != nil {
				log.Printf("❌ [Auto Fix] 修复缺失持仓失败: %s %s - %v", 
					diff.Symbol, diff.Side, err)
			} else {
				fixedCount++
			}
			
		case "extra_position":
			// 交易所有持仓但数据库没有记录 - 创建开仓记录
			if err := sync.fixExtraPosition(diff); err != nil {
				log.Printf("❌ [Auto Fix] 修复额外持仓失败: %s %s - %v", 
					diff.Symbol, diff.Side, err)
			} else {
				fixedCount++
			}
			
		case "quantity_mismatch":
			// 数量不匹配 - 根据实际情况决定修复策略
			if err := sync.fixQuantityMismatch(diff); err != nil {
				log.Printf("❌ [Auto Fix] 修复数量差异失败: %s %s - %v", 
					diff.Symbol, diff.Side, err)
			} else {
				fixedCount++
			}
		}
	}
	
	log.Printf("✅ [Auto Fix] 自动修复完成: 成功修复%d/%d个差异", fixedCount, len(differences))
	return nil
}

// fixMissingPosition 修复缺失持仓（数据库有记录但交易所没有）
func (sync *ExchangeRecordSync) fixMissingPosition(diff *PositionDifference) error {
	log.Printf("🔧 [Fix Missing] 修���缺失持仓: %s %s", diff.Symbol, diff.Side)
	
	// 估算平仓价格和原因
	estimatedPrice, closeReason := sync.estimateCloseDetails(diff.Symbol, diff.Side)
	
	// 更新数据库记录为已平仓
	return sync.closePositionInDatabase(diff.Symbol, diff.Side, estimatedPrice, 
		"AUTO_SYNC", fmt.Sprintf("auto_fix_%s", closeReason))
}

// fixExtraPosition 修复额外持仓（交易所有持仓但数据库没有记录）
func (sync *ExchangeRecordSync) fixExtraPosition(diff *PositionDifference) error {
	log.Printf("🔧 [Fix Extra] 修复额外持仓: %s %s", diff.Symbol, diff.Side)
	
	// 获取持仓详细信息
	positionDetails, err := sync.getPositionDetails(diff.Symbol, diff.Side)
	if err != nil {
		return fmt.Errorf("获取持仓详情失败: %w", err)
	}
	
	// 创建开仓记录
	return sync.createMissingTradeRecord(positionDetails)
}

// fixQuantityMismatch 修复数量差异
func (sync *ExchangeRecordSync) fixQuantityMismatch(diff *PositionDifference) error {
	log.Printf("🔧 [Fix Mismatch] 修复数量差异: %s %s DB=%.6f Exchange=%.6f", 
		diff.Symbol, diff.Side, diff.DatabaseQuantity, diff.ExchangeQuantity)
	
	if diff.ExchangeQuantity == 0 {
		// 交易所已平仓但数据库未更新
		return sync.fixMissingPosition(diff)
	} else if diff.ExchangeQuantity < diff.DatabaseQuantity {
		// 部分平仓未记录
		return sync.recordPartialClose(diff)
	} else {
		// 数据库数量小于实际持仓，可能是加仓未记录
		return sync.recordAdditionalPosition(diff)
	}
}

// estimateCloseDetails 估算平仓价格和原因 - 智能多层级查询
func (sync *ExchangeRecordSync) estimateCloseDetails(symbol, side string) (float64, string) {
	log.Printf("🔍 [Smart Estimate] 开始智能估算平仓价格: %s %s", symbol, side)
	
	// 1. 优先策略：从数据库获取开仓记录，利用保存的订单号查询真实成交价
	if sync.database != nil {
		openTrade, err := sync.database.GetOpenTrade(sync.traderID, symbol, side)
		if err == nil && openTrade.OpenOrderID != "" && openTrade.OpenOrderID != "SYNC_CREATED" {
			log.Printf("📋 [Smart Estimate] 找到开仓记录，订单号: %s", openTrade.OpenOrderID)
			
			// 通过开仓订单号查询订单状态
			if orderStatus, err := sync.trader.GetOrderStatus(symbol, 0); err == nil { // 暂时传0，实际需要转换orderID
				if fillDetails := sync.parseOrderFillDetails(orderStatus, 0, symbol, side); fillDetails != nil {
					log.Printf("✅ [Smart Estimate] 从订单状态获取真实价格: %.6f", fillDetails.AveragePrice)
					return fillDetails.AveragePrice, "order_status_exact"
				}
			}
		}
	}
	
	// 2. 核心策略：通过仓位历史查询获取真实平仓记录 ⭐
	if realPrice, realTime := sync.getRealClosePriceFromTradeHistory(symbol, side); realPrice > 0 {
		log.Printf("✅ [Smart Estimate] 从成交历史获取真实平仓价格: %.6f (时间: %d)", realPrice, realTime)
		return realPrice, "trade_history_exact"
	}
	
	// 3. 降级策略：使用订单历史智能匹配
	if recentPrice := sync.getRecentClosePriceFromOrderHistory(symbol, side); recentPrice > 0 {
		log.Printf("✅ [Smart Estimate] 从订单历史获取平仓价格: %.6f", recentPrice)
		return recentPrice, "order_history_matched"
	}
	
	// 4. 最后降级：使用智能市场价格估算
	marketData, err := market.Get(symbol)
	if err != nil {
		log.Printf("❌ [Smart Estimate] 获取%s市场价格失败: %v", symbol, err)
		return 0, "unknown_market_unavailable"
	}
	
	// 根据方向选择合适的市场价格（考虑买卖价差）
	var estimatedPrice float64
	if side == "long" {
		estimatedPrice = marketData.CurrentPrice * 0.9995  // 多头平仓略微调低
	} else {
		estimatedPrice = marketData.CurrentPrice * 1.0005  // 空头平仓略微调高
	}
	
	log.Printf("📊 [Smart Estimate] %s %s 智能市场价格估算: %.6f", 
		symbol, side, estimatedPrice)
	
	return estimatedPrice, "market_price_smart_estimate"
}

// getRealClosePriceFromTradeHistory 通过成交历史获取真实平仓价格 ⭐ 核心功能
func (sync *ExchangeRecordSync) getRealClosePriceFromTradeHistory(symbol, side string) (float64, int64) {
	log.Printf("🔍 [Trade History] 查询 %s %s 的成交历史...", symbol, side)
	
	// 获取最近100条成交历史
	trades, err := sync.trader.GetTradeHistory(symbol, 100)
	if err != nil {
		log.Printf("⚠️ [Trade History] 获取成交历史失败: %v", err)
		return 0, 0
	}
	
	log.Printf("📋 [Trade History] 获得 %d 条成交记录", len(trades))
	
	// 按时间倒序查找最近的平仓交易
	for _, trade := range trades {
		positionSide, _ := trade["positionSide"].(string)
		tradeSide, _ := trade["side"].(string)
		realizedPnlStr, _ := trade["realizedPnl"].(string)
		priceStr, _ := trade["price"].(string)
		timeValue := trade["time"]
		
		// 关键判断：找到平仓交易（realizedPnl != "0" 表示有盈亏结算）
		if realizedPnlStr != "" && realizedPnlStr != "0" && realizedPnlStr != "0.00000000" {
			// 验证是否为目标方向的平仓
			if sync.isMatchingClosePosition(positionSide, tradeSide, side) {
				if price := sync.parsePrice(priceStr); price > 0 {
					var tradeTime int64
					if timeInt, ok := timeValue.(int64); ok {
						tradeTime = timeInt
					} else if timeFloat, ok := timeValue.(float64); ok {
						tradeTime = int64(timeFloat)
					}
					
					log.Printf("✅ [Trade History] 找到匹配平仓: %s %s->%s, 价格=%s, 盈亏=%s, 时间=%d", 
						symbol, positionSide, tradeSide, priceStr, realizedPnlStr, tradeTime)
					return price, tradeTime
				}
			}
		}
	}
	
	log.Printf("❌ [Trade History] 未找到匹配的平仓交易")
	return 0, 0
}

// getRecentClosePriceFromOrderHistory 从订单历史获取平仓价格（降级方案）
func (sync *ExchangeRecordSync) getRecentClosePriceFromOrderHistory(symbol, side string) float64 {
	log.Printf("🔍 [Order History] 查询 %s %s 的订单历史...", symbol, side)
	
	orders, err := sync.trader.GetOrderHistory(symbol, 50)
	if err != nil {
		log.Printf("⚠️ [Order History] 获取订单历史失败: %v", err)
		return 0
	}
	
	// 查找最近的平仓订单（reduceOnly=true）
	for _, order := range orders {
		if reduceOnly, ok := order["reduceOnly"].(bool); ok && reduceOnly {
			if avgPriceStr, ok := order["avgPrice"].(string); ok && avgPriceStr != "" && avgPriceStr != "0" {
				if price := sync.parsePrice(avgPriceStr); price > 0 {
					log.Printf("✅ [Order History] 找到平仓订单: 价格=%s", avgPriceStr)
					return price
				}
			}
		}
	}
	
	log.Printf("❌ [Order History] 未找到平仓订单")
	return 0
}

// isMatchingClosePosition 判断是否为匹配的平仓交易
func (sync *ExchangeRecordSync) isMatchingClosePosition(positionSide, tradeSide, expectedSide string) bool {
	if expectedSide == "long" {
		// 多头平仓：positionSide="LONG" && side="SELL"
		return positionSide == "LONG" && tradeSide == "SELL"
	} else if expectedSide == "short" {
		// 空头平仓：positionSide="SHORT" && side="BUY"  
		return positionSide == "SHORT" && tradeSide == "BUY"
	}
	return false
}

// parsePrice 解析价格字符串为浮点数
func (sync *ExchangeRecordSync) parsePrice(priceStr string) float64 {
	if priceStr == "" {
		return 0
	}
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		log.Printf("⚠️ [Parse Price] 解析价格失败: %s -> %v", priceStr, err)
		return 0
	}
	return price
}

// closePositionInDatabase 在数据库中关闭持仓记录
func (sync *ExchangeRecordSync) closePositionInDatabase(symbol, side string, closePrice float64, orderID, reason string) error {
	if sync.database == nil {
		return fmt.Errorf("数据库连接不可用")
	}
	
	log.Printf("⚠️ [ExchangeSync] closePositionInDatabase 被调用，但已废弃估算逻辑")
	log.Printf("    参数: symbol=%s, side=%s, estimatedPrice=%.6f, reason=%s", symbol, side, closePrice, reason)
	log.Printf("🚫 [ExchangeSync] 拒绝使用估算价格 %.6f，必须从交易所获取权威数据", closePrice)
	
	// 查找对应的开仓记录
	openTrade, err := sync.database.GetOpenTrade(sync.traderID, symbol, side)
	if err != nil {
		return fmt.Errorf("未找到开仓记录: %w", err)
	}
	
	log.Printf("✅ [ExchangeSync] 找到开仓记录: ID=%s, 开仓价=%.6f", openTrade.ID, openTrade.OpenPrice)
	
	// 🎯 关键修改：必须从交易所获取权威数据
	binanceTrader, ok := sync.trader.(*FuturesTrader)
	if !ok {
		return fmt.Errorf("交易器不支持权威数据查询")
	}
	
	// 获取权威的平仓数据
	authData, err := binanceTrader.GetAuthoritativeCloseData(symbol, side, orderID)
	if err != nil {
		log.Printf("❌ [ExchangeSync] 无法获取权威数据: %v", err)
		log.Printf("🚫 [ExchangeSync] 拒绝写入估算数据到数据库")
		
		// 记录失败原因，但不写入错误数据
		return fmt.Errorf("拒绝使用估算数据，获取权威数据失败: %w", err)
	}
	
	log.Printf("✅ [ExchangeSync] 获取权威数据成功:")
	log.Printf("    真实价格: %.6f", authData.ActualPrice)
	log.Printf("    真实盈亏: %.2f USDT", authData.ActualPnL)
	log.Printf("    数据来源: %s", authData.DataSource)
	
	// 使用权威数据
	var finalPnL float64
	var finalPnLPct float64
	var finalPrice float64
	var finalTime time.Time
	
	if authData.ActualPnL != 0 && authData.DataSource != "INCOME_HISTORY" {
		finalPnL = authData.ActualPnL
		finalPrice = authData.ActualPrice
		finalTime = authData.ActualTime
	} else if authData.ActualPrice > 0 {
		if side == "long" {
			finalPnL = openTrade.Quantity * (authData.ActualPrice - openTrade.OpenPrice)
		} else {
			finalPnL = openTrade.Quantity * (openTrade.OpenPrice - authData.ActualPrice)
		}
		finalPrice = authData.ActualPrice
		finalTime = authData.ActualTime
	} else {
		return fmt.Errorf("权威数据不完整，无法写入数据库")
	}
	
	if openTrade.MarginUsed > 0 {
		finalPnLPct = (finalPnL / openTrade.MarginUsed) * 100
	}
	
	durationSecs := int(finalTime.Sub(openTrade.OpenTime).Seconds())
	
	// 只有在获得完整权威数据后才更新数据库
	err = sync.database.UpdateTrade(openTrade.ID, finalPrice, finalTime, 
		"closed", reason, orderID, finalPnL, finalPnLPct, durationSecs)
		
	if err != nil {
		return fmt.Errorf("更新交易记录失败: %w", err)
	}
	
	log.Printf("✅ [DB Close] 数据库记录已关闭: %s %s 盈亏=%.2f USDT", 
		symbol, side, finalPnL)
	
	return nil
}

// getPositionDetails 获取持仓详细信息
func (sync *ExchangeRecordSync) getPositionDetails(symbol, side string) (map[string]interface{}, error) {
	positions, err := sync.trader.GetPositions()
	if err != nil {
		return nil, err
	}
	
	for _, pos := range positions {
		if pos["symbol"] == symbol && pos["side"] == side {
			return pos, nil
		}
	}
	
	return nil, fmt.Errorf("未找到持仓: %s %s", symbol, side)
}

// createMissingTradeRecord 创建缺失的交易记录
func (sync *ExchangeRecordSync) createMissingTradeRecord(positionDetails map[string]interface{}) error {
	if sync.database == nil {
		return fmt.Errorf("数据库连接不可用")
	}
	
	symbol := positionDetails["symbol"].(string)
	side := positionDetails["side"].(string)
	quantity := math.Abs(positionDetails["positionAmt"].(float64))
	entryPrice := positionDetails["entryPrice"].(float64)
	
	// 估算杠杆
	leverage := 10
	if lev, ok := positionDetails["leverage"].(float64); ok {
		leverage = int(lev)
	}
	
	// 创建交易记录
	tradeRecord := &config.TradeRecord{
		TraderID:      sync.traderID,
		Symbol:        symbol,
		Side:          side,
		Quantity:      quantity,
		Leverage:      leverage,
		OpenPrice:     entryPrice,
		PositionValue: quantity * entryPrice,
		MarginUsed:    (quantity * entryPrice) / float64(leverage),
		OpenTime:      time.Now().Add(-1 * time.Hour), // 估算开仓时间为1小时前
		Status:        "open",
		OpenOrderID:   "SYNC_CREATED",
	}
	
	err := sync.database.CreateTrade(tradeRecord)
	if err != nil {
		return fmt.Errorf("创建交易记录失败: %w", err)
	}
	
	log.Printf("✅ [Create Record] 创建缺失交易记录: %s %s 数量=%.6f 价格=%.6f", 
		symbol, side, quantity, entryPrice)
	
	return nil
}

// recordPartialClose 记录部分平仓
func (sync *ExchangeRecordSync) recordPartialClose(diff *PositionDifference) error {
	partialCloseQty := diff.DatabaseQuantity - diff.ExchangeQuantity
	estimatedPrice, _ := sync.estimateCloseDetails(diff.Symbol, diff.Side)
	
	log.Printf("📝 [Partial Close] 记录部分平仓: %s %s 数量=%.6f 价格=%.6f", 
		diff.Symbol, diff.Side, partialCloseQty, estimatedPrice)
	
	// TODO: 实现部分平仓记录逻辑
	// 这可能需要修改数据库结构以支持部分平仓记录
	
	return nil
}

// recordAdditionalPosition 记录额外持仓
func (sync *ExchangeRecordSync) recordAdditionalPosition(diff *PositionDifference) error {
	additionalQty := diff.ExchangeQuantity - diff.DatabaseQuantity
	
	log.Printf("📝 [Additional Position] 记录额外持仓: %s %s 数量=%.6f", 
		diff.Symbol, diff.Side, additionalQty)
	
	// TODO: 实现额外持仓记录逻辑
	// 可能需要创建新的交易记录或更新现有记录
	
	return nil
}

// ===== 公用方法 =====

// RunFullSync 执行完整同步
func (sync *ExchangeRecordSync) RunFullSync() error {
	log.Printf("🚀 [Full Sync] 开始执行完整的交易所记录同步...")
	
	// 1. 检测持仓差异
	differences, err := sync.DetectPositionDifferences()
	if err != nil {
		return fmt.Errorf("检测持仓差异失败: %w", err)
	}
	
	// 2. 自动修复差异
	if len(differences) > 0 {
		if err := sync.AutoFixPositionDifferences(differences); err != nil {
			return fmt.Errorf("自动修复失败: %w", err)
		}
	} else {
		log.Printf("✅ [Full Sync] 未发现持仓差异，无需修复")
	}
	
	log.Printf("🎯 [Full Sync] 完整同步完成")
	return nil
}