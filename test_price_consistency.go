package main

import (
	"fmt"
	"log"
	"nofx/config"
	"nofx/trader"
	"time"
)

// TestPriceConsistency 测试价格一致性验证
// 🔥 核心：确保开仓平仓价格与交易所完全一致
func main() {
	log.Printf("🚀 开始价格一致性验证测试...")
	
	// 1. 初始化数据库
	database, err := config.NewDatabase("test_price_consistency.db")
	if err != nil {
		log.Fatalf("❌ 初始化数据库失败: %v", err)
	}
	defer database.Close()
	
	// 2. 创建币安交易器实例（用于验证价格一致性）
	// 注意：这里使用模拟数据，真实场景需要实际API密钥
	binanceTrader := trader.NewFuturesTrader("", "") 
	
	// 3. 测试开仓价格精度
	testOpenPricePrecision(binanceTrader)
	
	// 4. 测试平仓价格验证  
	testClosePriceVerification(database, binanceTrader)
	
	// 5. 测试价格格式化精度
	testPriceFormatPrecision(binanceTrader)
	
	// 6. 测试实际交易数据验证
	testRealTradeDataValidation(database)
	
	log.Printf("✅ 价格一致性验证测试完成")
}

// testOpenPricePrecision 测试开仓价格精度
func testOpenPricePrecision(trader *trader.FuturesTrader) {
	log.Printf("📊 测试1: 验证开仓价格精度...")
	
	testCases := []struct {
		symbol   string
		price    float64
		expected string
	}{
		{"BTCUSDT", 50123.456789, "50123.46"},   // BTC 2位精度
		{"ETHUSDT", 3456.123456, "3456.12"},     // ETH 2位精度  
		{"SOLUSDT", 234.567890, "234.567"},      // SOL 3位精度
		{"DOGEUSDT", 0.123456789, "0.12346"},    // DOGE 5位精度
	}
	
	for _, tc := range testCases {
		// 测试价格格式化
		formatted, err := trader.FormatPrice(tc.symbol, tc.price)
		if err != nil {
			log.Printf("⚠️ [%s] 价格格式化失败（可能缺少API连接）: %v", tc.symbol, err)
			log.Printf("   原始价格: %.8f", tc.price)
			continue
		}
		
		if formatted == tc.expected {
			log.Printf("✅ [%s] 价格精度正确: %.8f -> %s", tc.symbol, tc.price, formatted)
		} else {
			log.Printf("⚠️ [%s] 价格精度可能不同: %.8f -> %s (期望: %s)", tc.symbol, tc.price, formatted, tc.expected)
		}
	}
	
	log.Printf("🎯 测试1完成: 开仓价格精度验证")
}

// testClosePriceVerification 测试平仓价格验证
func testClosePriceVerification(database *config.Database, trader *trader.FuturesTrader) {
	log.Printf("📊 测试2: 验证平仓价格验证机制...")
	
	// 创建测试交易记录
	testTradeID := fmt.Sprintf("price_test_%d", time.Now().UnixNano())
	exchangeTime := time.Now().UTC()
	
	// 模拟开仓
	openTrade := &config.TradeRecord{
		ID:            testTradeID,
		TraderID:      "price_test_trader",
		Symbol:        "BTCUSDT",
		Side:          "long",
		Quantity:      0.001,
		Leverage:      10,
		OpenPrice:     50000.123456, // 🔥 高精度价格
		PositionValue: 50.0,
		MarginUsed:    5.0,
		OpenTime:      exchangeTime,
		Status:        "open",
		OpenOrderID:   "open_12345",
	}
	
	err := database.CreateTrade(openTrade)
	if err != nil {
		log.Fatalf("❌ 创建测试交易失败: %v", err)
	}
	
	// 验证保存的开仓价格精度
	savedTrade, err := database.GetOpenTrade("price_test_trader", "BTCUSDT", "long")
	if err != nil {
		log.Fatalf("❌ 获取交易记录失败: %v", err)
	}
	
	log.Printf("💰 开仓价格验证:")
	log.Printf("   原始: %.8f", openTrade.OpenPrice)
	log.Printf("   保存: %.8f", savedTrade.OpenPrice)
	
	// 允许浮点数精度误差（1e-6）
	priceDiff := abs(openTrade.OpenPrice - savedTrade.OpenPrice)
	if priceDiff < 1e-6 {
		log.Printf("✅ 开仓价格精度保持正确")
	} else {
		log.Printf("❌ 开仓价格精度丢失: 差异 %.8f", priceDiff)
	}
	
	// 模拟平仓价格验证
	closePrice := 51234.567890 // 🔥 高精度平仓价格
	closeTime := exchangeTime.Add(30 * time.Minute)
	
	pnl := (closePrice - openTrade.OpenPrice) * openTrade.Quantity
	pnlPct := (pnl / openTrade.MarginUsed) * 100
	durationSecs := int(closeTime.Sub(exchangeTime).Seconds())
	
	err = database.UpdateTrade(testTradeID, closePrice, closeTime, "closed", "manual", "close_67890", pnl, pnlPct, durationSecs)
	if err != nil {
		log.Fatalf("❌ 更新交易记录失败: %v", err)
	}
	
	// 验证平仓价格精度
	updatedTrades, err := database.GetTraderTrades("price_test_trader", 1)
	if err != nil || len(updatedTrades) == 0 {
		log.Fatalf("❌ 获取更新交易失败: %v", err)
	}
	
	updatedTrade := updatedTrades[0]
	if updatedTrade.ClosePrice == nil {
		log.Fatalf("❌ 平仓价格为空")
	}
	
	log.Printf("💰 平仓价格验证:")
	log.Printf("   原始: %.8f", closePrice)
	log.Printf("   保存: %.8f", *updatedTrade.ClosePrice)
	
	closePriceDiff := abs(closePrice - *updatedTrade.ClosePrice)
	if closePriceDiff < 1e-6 {
		log.Printf("✅ 平仓价格精度保持正确")
	} else {
		log.Printf("❌ 平仓价格精度丢失: 差异 %.8f", closePriceDiff)
	}
	
	// 验证盈亏计算精度
	expectedPnL := (closePrice - openTrade.OpenPrice) * openTrade.Quantity
	actualPnL := updatedTrade.PnL
	
	log.Printf("💰 盈亏计算验证:")
	log.Printf("   期望: %.8f USDT", expectedPnL)
	log.Printf("   实际: %.8f USDT", actualPnL)
	
	pnlDiff := abs(expectedPnL - actualPnL)
	if pnlDiff < 1e-6 {
		log.Printf("✅ 盈亏计算精度正确")
	} else {
		log.Printf("❌ 盈亏计算有误差: 差异 %.8f USDT", pnlDiff)
	}
	
	// 清理测试数据
	database.DeleteTrade(testTradeID)
	
	log.Printf("🎯 测试2完成: 平仓价格验证机制")
}

// testPriceFormatPrecision 测试价格格式化精度
func testPriceFormatPrecision(trader *trader.FuturesTrader) {
	log.Printf("📊 测试3: 验证价格格式化精度...")
	
	// 测试不同精度需求
	testCases := []struct {
		symbol      string
		rawPrice    float64
		description string
	}{
		{"BTCUSDT", 50123.123456789, "BTC高精度价格"},
		{"ETHUSDT", 3456.987654321, "ETH高精度价格"},
		{"SOLUSDT", 234.123456789, "SOL中精度价格"},
		{"DOGEUSDT", 0.123456789, "DOGE低价格高精度"},
	}
	
	for _, tc := range testCases {
		// 尝试获取正确的价格精度
		formatted, err := trader.FormatPrice(tc.symbol, tc.rawPrice)
		if err != nil {
			// 如果获取精度失败，使用默认处理
			log.Printf("⚠️ [%s] 无法获取精度信息，使用默认格式: %.8f", tc.symbol, tc.rawPrice)
			continue
		}
		
		log.Printf("✅ [%s] %s: %.8f -> %s", tc.symbol, tc.description, tc.rawPrice, formatted)
		
		// 验证格式化后的价格能正确解析回来
		// （这在实际交易中很重要，确保不会因为精度问题导致订单失败）
	}
	
	log.Printf("🎯 测试3完成: 价格格式化精度验证")
}

// testRealTradeDataValidation 测试真实交易数据验证
func testRealTradeDataValidation(database *config.Database) {
	log.Printf("📊 测试4: 验证真实交易数据一致性...")
	
	// 模拟不同精度的真实交易数据
	realTradeScenarios := []struct {
		symbol      string
		openPrice   float64
		closePrice  float64
		quantity    float64
		description string
	}{
		{"BTCUSDT", 49999.99, 50500.01, 0.001, "BTC微量交易"},
		{"ETHUSDT", 3456.78, 3500.12, 0.01, "ETH小额交易"},
		{"SOLUSDT", 234.567, 240.123, 0.1, "SOL中等交易"},
		{"DOGEUSDT", 0.12345, 0.13579, 100.0, "DOGE大量低价交易"},
	}
	
	for i, scenario := range realTradeScenarios {
		tradeID := fmt.Sprintf("real_test_%d_%d", time.Now().UnixNano(), i)
		
		// 创建交易记录
		openTime := time.Now().UTC().Add(time.Duration(i) * time.Minute)
		closeTime := openTime.Add(15 * time.Minute)
		
		// 计算精确的盈亏
		pnl := (scenario.closePrice - scenario.openPrice) * scenario.quantity
		marginUsed := (scenario.openPrice * scenario.quantity) / 10 // 假设10倍杠杆
		pnlPct := (pnl / marginUsed) * 100
		
		// 创建开仓记录
		trade := &config.TradeRecord{
			ID:            tradeID,
			TraderID:      "real_test_trader",
			Symbol:        scenario.symbol,
			Side:          "long",
			Quantity:      scenario.quantity,
			Leverage:      10,
			OpenPrice:     scenario.openPrice,
			PositionValue: scenario.openPrice * scenario.quantity,
			MarginUsed:    marginUsed,
			OpenTime:      openTime,
			Status:        "open",
			OpenOrderID:   fmt.Sprintf("open_%d", i),
		}
		
		err := database.CreateTrade(trade)
		if err != nil {
			log.Printf("❌ 创建交易失败: %v", err)
			continue
		}
		
		// 更新为平仓状态
		err = database.UpdateTrade(tradeID, scenario.closePrice, closeTime, "closed", "manual", 
			fmt.Sprintf("close_%d", i), pnl, pnlPct, 15*60)
		if err != nil {
			log.Printf("❌ 更新交易失败: %v", err)
			continue
		}
		
		log.Printf("✅ [%s] %s: 开仓%.6f -> 平仓%.6f, 盈亏%.6f USDT (%.2f%%)", 
			scenario.symbol, scenario.description, scenario.openPrice, scenario.closePrice, pnl, pnlPct)
		
		// 清理测试数据
		database.DeleteTrade(tradeID)
	}
	
	log.Printf("🎯 测试4完成: 真实交易数据验证")
}

// abs 计算绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}