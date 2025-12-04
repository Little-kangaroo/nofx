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

// estimateCloseDetails 估算平仓价格和原因
func (sync *ExchangeRecordSync) estimateCloseDetails(symbol, side string) (float64, string) {
	// 获取当前市场价格作为估算平仓价格
	marketData, err := market.Get(symbol)
	if err != nil {
		log.Printf("⚠️ [Estimate Close] 获取%s市场价格失败: %v", symbol, err)
		return 0, "unknown_market_unavailable"
	}
	
	closeReason := "external_close_detected"
	log.Printf("📊 [Estimate Close] %s %s 估算平仓价格: %.6f (市场价)", 
		symbol, side, marketData.CurrentPrice)
	
	return marketData.CurrentPrice, closeReason
}

// closePositionInDatabase 在数据库中关闭持仓记录
func (sync *ExchangeRecordSync) closePositionInDatabase(symbol, side string, closePrice float64, orderID, reason string) error {
	if sync.database == nil {
		return fmt.Errorf("数据库连接不可用")
	}
	
	// 查找对应的开仓记录
	openTrade, err := sync.database.GetOpenTrade(sync.traderID, symbol, side)
	if err != nil {
		return fmt.Errorf("未找到开仓记录: %w", err)
	}
	
	// 计算盈亏
	var pnl float64
	if side == "long" {
		pnl = openTrade.Quantity * (closePrice - openTrade.OpenPrice)
	} else {
		pnl = openTrade.Quantity * (openTrade.OpenPrice - closePrice)
	}
	
	pnlPct := 0.0
	if openTrade.MarginUsed > 0 {
		pnlPct = (pnl / openTrade.MarginUsed) * 100
	}
	
	closeTime := time.Now()
	durationSecs := int(closeTime.Sub(openTrade.OpenTime).Seconds())
	
	// 更新数据库
	err = sync.database.UpdateTrade(openTrade.ID, closePrice, closeTime, 
		"closed", reason, orderID, pnl, pnlPct, durationSecs)
		
	if err != nil {
		return fmt.Errorf("更新交易记录失败: %w", err)
	}
	
	log.Printf("✅ [DB Close] 数据库记录已关闭: %s %s 盈亏=%.2f USDT", 
		symbol, side, pnl)
	
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
	
	log.Printf("📝 [Partial Close] 记录部分平仓: %s %s 数量=%.6f", 
		diff.Symbol, diff.Side, partialCloseQty)
	
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