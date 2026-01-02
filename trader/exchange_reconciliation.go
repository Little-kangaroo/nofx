package trader

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"nofx/config"
)

// ReconcileWithExchange 每5分钟对账一次，确保数据库与交易所强一致
// 在主循环的5分钟周期开始时调用，先对账再统计
func (at *AutoTrader) ReconcileWithExchange() error {
	log.Printf("💎 [对账] ========== 开始5分钟对账 ==========")
	startTime := time.Now()

	if at.database == nil || at.trader == nil {
		return fmt.Errorf("数据库或交易器未初始化")
	}

	// 1. 处理"待同步"状态的记录（开仓/平仓同步失败的重试）
	if err := at.retryPendingTrades(); err != nil {
		log.Printf("⚠️ [对账] 重试待同步记录失败: %v", err)
	}

	// 2. 检测外部平仓（止损触发、手动平仓等）
	if err := at.detectExternalClosures(); err != nil {
		log.Printf("⚠️ [对账] 检测外部平仓失败: %v", err)
	}

	elapsed := time.Since(startTime)
	log.Printf("✅ [对账] 完成，耗时 %v", elapsed)
	log.Printf("💎 [对账] ========== 对账完成 ==========\n")

	return nil
}

// retryPendingTrades 重试之前同步失败的记录
func (at *AutoTrader) retryPendingTrades() error {
	// 查询所有待同步的记录（需要数据库支持）
	// 这里暂时跳过，因为需要先添加数据库方法
	log.Printf("🔄 [对账] 检查待同步记录...")
	return nil
}

// detectExternalClosures 检测外部平仓（核心逻辑）
func (at *AutoTrader) detectExternalClosures() error {
	binanceTrader, ok := at.trader.(*FuturesTrader)
	if !ok {
		return fmt.Errorf("仅支持 Binance 交易器")
	}

	// 1. 获取所有 status='open' 的持仓
	openTrades, err := at.database.GetOpenTrades(at.id)
	if err != nil {
		return fmt.Errorf("查询数据库失败: %w", err)
	}

	if len(openTrades) == 0 {
		log.Printf("✅ [对账] 无需检查（无开仓记录）")
		return nil
	}

	log.Printf("📊 [对账] 数据库中有 %d 个 open 持仓", len(openTrades))

	// 2. 获取交易所当前持仓
	exchangePositions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取交易所持仓失败: %w", err)
	}

	// 3. 构建交易所持仓映射
	posMap := make(map[string]bool) // key: "BTCUSDT_LONG"
	for _, pos := range exchangePositions {
		symbol, _ := pos["symbol"].(string)
		side, _ := pos["side"].(string)
		amt, _ := pos["positionAmt"].(float64)
		if amt != 0 {
			key := fmt.Sprintf("%s_%s", symbol, strings.ToUpper(side))
			posMap[key] = true
		}
	}

	log.Printf("📊 [对账] 交易所当前持仓数: %d", len(posMap))

	// 4. 检查每条 open 记录
	closedCount := 0
	for _, dbTrade := range openTrades {
		key := fmt.Sprintf("%s_%s", dbTrade.Symbol, strings.ToUpper(dbTrade.Side))

		if !posMap[key] {
			// 🔴 数据库说 open，但交易所没有持仓 → 已被平仓
			log.Printf("⚠️ [对账] 发现外部平仓: %s %s", dbTrade.Symbol, dbTrade.Side)

			// 从交易所同步平仓数据
			if err := at.syncExternalClosure(dbTrade, binanceTrader); err != nil {
				log.Printf("❌ [对账] 同步外部平仓失败: %s %s - %v", dbTrade.Symbol, dbTrade.Side, err)
			} else {
				closedCount++
			}
		}
	}

	if closedCount > 0 {
		log.Printf("✅ [对账] 同步了 %d 个外部平仓", closedCount)
	}

	return nil
}

// syncExternalClosure 从交易所同步外部平仓数据（止损、手动平仓等）
func (at *AutoTrader) syncExternalClosure(dbTrade *config.TradeRecord, binanceTrader *FuturesTrader) error {
	log.Printf("🔍 [对账] 同步外部平仓: %s %s", dbTrade.Symbol, dbTrade.Side)

	// 策略：通过 Income History + UserTrades 组合查询
	// 1. 从 Income History 获取 REALIZED_PNL（权威盈亏）
	// 2. 从 UserTrades 获取成交价格和订单ID

	incomes, err := binanceTrader.GetIncomeHistory(dbTrade.Symbol, "REALIZED_PNL", 50)
	if err != nil {
		return fmt.Errorf("查询 Income History 失败: %w", err)
	}

	userTrades, err := binanceTrader.GetTradeHistory(dbTrade.Symbol, 100)
	if err != nil {
		return fmt.Errorf("查询 UserTrades 失败: %w", err)
	}

	// 匹配逻辑（串行模式下）
	expectedSide := strings.ToUpper(dbTrade.Side) // "long" -> "LONG"

	var closePrice float64
	var closeTime time.Time
	var realizedPnl float64
	var closeOrderID string
	var found bool

	for _, income := range incomes {
		// 必须在开仓之后
		if income.Time.Before(dbTrade.OpenTime) {
			continue
		}

		// 通过 TradeID 找到对应的 UserTrade
		for _, ut := range userTrades {
			tradeID, ok := ut["id"].(int64)
			if !ok {
				continue
			}

			// 将 income.TradeID (string) 转换为 int64 进行比较
			incomeTradeID, err := strconv.ParseInt(income.TradeID, 10, 64)
			if err != nil || tradeID != incomeTradeID {
				continue
			}

			positionSide, _ := ut["positionSide"].(string)
			if positionSide != expectedSide {
				continue
			}

			// realizedPnl != 0 才是平仓
			pnlStr, _ := ut["realizedPnl"].(string)
			pnl, _ := strconv.ParseFloat(pnlStr, 64)
			if pnl == 0 {
				continue
			}

			// ✅ 找到了！
			priceStr, _ := ut["price"].(string)
			closePrice, _ = strconv.ParseFloat(priceStr, 64)
			closeTime = income.Time
			realizedPnl = income.Income
			orderID, _ := ut["orderId"].(int64)
			closeOrderID = fmt.Sprintf("%d", orderID)
			found = true

			log.Printf("✅ [对账] 找到外部平仓数据: 价格=%.6f, 盈亏=%.2f USDT", closePrice, realizedPnl)
			break
		}

		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("未找到匹配的外部平仓记录")
	}

	// 计算其他字段
	pnlPct := 0.0
	if dbTrade.MarginUsed > 0 {
		pnlPct = (realizedPnl / dbTrade.MarginUsed) * 100
	}

	durationSecs := int(closeTime.Sub(dbTrade.OpenTime).Seconds())

	// 更新数据库
	err = at.database.UpdateTrade(
		dbTrade.ID,
		closePrice,
		closeTime,
		"closed",
		"external_closure", // 标记为外部平仓
		closeOrderID,
		realizedPnl, // 🔥 交易所权威盈亏
		pnlPct,
		durationSecs,
	)

	if err != nil {
		return fmt.Errorf("更新数据库失败: %w", err)
	}

	log.Printf("✅ [对账] 外部平仓同步成功: %s %s, 盈亏=%.2f USDT (%.2f%%)",
		dbTrade.Symbol, dbTrade.Side, realizedPnl, pnlPct)

	return nil
}

// SyncOpenTradeData 异步同步开仓数据（在开仓后调用）
func (at *AutoTrader) SyncOpenTradeData(symbol string, orderID int64) {
	log.Printf("🔄 [同步] 开始同步开仓数据: %s, OrderID=%d", symbol, orderID)

	binanceTrader, ok := at.trader.(*FuturesTrader)
	if !ok {
		log.Printf("❌ [同步] trader 类型不支持")
		return
	}

	maxRetries := 20
	retryInterval := 3 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		time.Sleep(retryInterval)

		// 查询订单状态
		orderStatus, err := binanceTrader.GetOrderStatus(symbol, orderID)
		if err != nil {
			log.Printf("⚠️ [同步] 查询订单状态失败 (重试 %d/%d): %v", attempt+1, maxRetries, err)
			continue
		}

		status, ok := orderStatus["status"].(string)
		if !ok {
			continue
		}

		log.Printf("🔍 [同步] 订单状态: %s (重试 %d/%d)", status, attempt+1, maxRetries)

		if status == "FILLED" {
			// ✅ 订单已完全成交，获取真实数据
			trades, err := binanceTrader.GetOrderTrades(symbol, orderID)
			if err != nil {
				log.Printf("⚠️ [同步] 获取成交明细失败: %v", err)
				continue
			}

			if len(trades) == 0 {
				log.Printf("⚠️ [同步] 成交明细为空")
				continue
			}

			// 计算加权平均价和总数量
			var totalNotional float64
			var totalQty float64
			var firstTradeTime time.Time

			for i, trade := range trades {
				totalNotional += trade.Price * trade.Quantity
				totalQty += trade.Quantity
				if i == 0 {
					firstTradeTime = trade.TradeTime
				}
			}

			if totalQty == 0 {
				continue
			}

			realOpenPrice := totalNotional / totalQty

			log.Printf("✅ [同步] 开仓数据确认:")
			log.Printf("    真实开仓价: %.6f", realOpenPrice)
			log.Printf("    真实数量: %.6f", totalQty)
			log.Printf("    真实时间: %s", firstTradeTime.Format("15:04:05"))
			log.Printf("    成交明细数: %d", len(trades))

			// 这里暂时只记录，实际更新数据库的逻辑在开仓时已经完成
			// 如果需要修正，可以添加 database.UpdateOpenTradePrice() 方法
			return
		}

		if status == "PARTIALLY_FILLED" {
			log.Printf("⏳ [同步] 订单部分成交，继续等待...")
			continue
		}

		if status == "CANCELED" || status == "EXPIRED" {
			log.Printf("❌ [同步] 订单未完全成交: %s", status)
			return
		}
	}

	log.Printf("❌ [同步] 同步超时，已重试 %d 次", maxRetries)
}

// SyncCloseTradeData 异步同步平仓数据（在平仓后调用）
func (at *AutoTrader) SyncCloseTradeData(tradeID, symbol string, closeOrderID int64) {
	log.Printf("🔄 [同步] 开始同步平仓数据: %s, OrderID=%d", symbol, closeOrderID)

	binanceTrader, ok := at.trader.(*FuturesTrader)
	if !ok {
		log.Printf("❌ [同步] trader 类型不支持")
		return
	}

	maxRetries := 20
	retryInterval := 3 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		time.Sleep(retryInterval)

		// 查询平仓订单状态
		orderStatus, err := binanceTrader.GetOrderStatus(symbol, closeOrderID)
		if err != nil {
			log.Printf("⚠️ [同步] 查询订单状态失败 (重试 %d/%d): %v", attempt+1, maxRetries, err)
			continue
		}

		status, ok := orderStatus["status"].(string)
		if !ok {
			continue
		}

		log.Printf("🔍 [同步] 平仓订单状态: %s (重试 %d/%d)", status, attempt+1, maxRetries)

		if status == "FILLED" {
			// ✅ 获取平仓成交明细
			closeTrades, err := binanceTrader.GetOrderTrades(symbol, closeOrderID)
			if err != nil {
				log.Printf("⚠️ [同步] 获取成交明细失败: %v", err)
				continue
			}

			if len(closeTrades) == 0 {
				continue
			}

			// 计算平仓均价
			var totalNotional float64
			var totalQty float64
			var lastTradeTime time.Time
			var sumRealizedPnl float64

			for _, trade := range closeTrades {
				totalNotional += trade.Price * trade.Quantity
				totalQty += trade.Quantity
				sumRealizedPnl += trade.RealizedPnL
				lastTradeTime = trade.TradeTime
			}

			if totalQty == 0 {
				continue
			}

			realClosePrice := totalNotional / totalQty

			// 🔥 关键：获取交易所权威盈亏
			time.Sleep(2 * time.Second) // 等待 Income 数据同步
			incomes, err := binanceTrader.GetIncomeHistory(symbol, "REALIZED_PNL", 20)

			var exchangeRealizedPnl float64
			var foundIncome bool

			if err == nil {
				// 找到时间最接近的 REALIZED_PNL 记录
				for _, income := range incomes {
					timeDiff := income.Time.Sub(lastTradeTime)
					if timeDiff < 0 {
						timeDiff = -timeDiff
					}
					if timeDiff < 10*time.Second {
						exchangeRealizedPnl = income.Income
						foundIncome = true
						log.Printf("✅ [同步] 从 Income History 获取权威盈亏: %.2f USDT", exchangeRealizedPnl)
						break
					}
				}
			}

			// 如果找不到 Income，使用成交明细里的 realizedPnl
			if !foundIncome {
				exchangeRealizedPnl = sumRealizedPnl
				log.Printf("⚠️ [同步] 使用成交明细盈亏: %.2f USDT", exchangeRealizedPnl)
			}

			// 获取开仓记录计算盈亏百分比
			openTrade, err := at.database.GetTradeByID(tradeID)
			if err != nil {
				log.Printf("❌ [同步] 获取开仓记录失败: %v", err)
				return
			}

			pnlPct := 0.0
			if openTrade.MarginUsed > 0 {
				pnlPct = (exchangeRealizedPnl / openTrade.MarginUsed) * 100
			}

			durationSecs := int(lastTradeTime.Sub(openTrade.OpenTime).Seconds())

			log.Printf("✅ [同步] 平仓数据确认:")
			log.Printf("    真实平仓价: %.6f", realClosePrice)
			log.Printf("    真实盈亏: %.2f USDT (%.2f%%)", exchangeRealizedPnl, pnlPct)
			log.Printf("    真实时间: %s", lastTradeTime.Format("15:04:05"))
			log.Printf("    持续时长: %d秒", durationSecs)

			// 更新数据库
			err = at.database.UpdateTrade(
				tradeID,
				realClosePrice,
				lastTradeTime,
				"closed",
				"exchange_verified", // 标记为已验证
				fmt.Sprintf("%d", closeOrderID),
				exchangeRealizedPnl, // 🔥 交易所权威盈亏
				pnlPct,
				durationSecs,
			)

			if err != nil {
				log.Printf("❌ [同步] 更新数据库失败: %v", err)
			} else {
				log.Printf("✅ [同步] 平仓同步成功")
			}

			return
		}
	}

	log.Printf("❌ [同步] 同步超时，已重试 %d 次", maxRetries)
}
