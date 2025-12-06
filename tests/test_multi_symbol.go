package main

import (
	"fmt"
	"log"
	"time"
	"nofx/microstructure"
)

func main() {
	log.Printf("🧪 开始测试多交易对订单流数据独立性...")
	
	// 创建OrderBookCalculator
	calculator := microstructure.NewOrderBookCalculator(5.0)
	
	// 测试数据
	symbol1 := "BTCUSDT"
	symbol2 := "ETHUSDT"
	
	// 创建测试数据
	now := time.Now()
	
	// BTC盘口数据
	btcDepth := &microstructure.DepthData{
		Symbol:    symbol1,
		Timestamp: now,
		Bids: []microstructure.OrderBookLevel{
			{Price: 50000, Quantity: 1.0},
			{Price: 49900, Quantity: 2.0},
		},
		Asks: []microstructure.OrderBookLevel{
			{Price: 50100, Quantity: 1.5},
			{Price: 50200, Quantity: 2.5},
		},
	}
	
	// ETH盘口数据
	ethDepth := &microstructure.DepthData{
		Symbol:    symbol2,
		Timestamp: now,
		Bids: []microstructure.OrderBookLevel{
			{Price: 3000, Quantity: 10.0},
			{Price: 2990, Quantity: 20.0},
		},
		Asks: []microstructure.OrderBookLevel{
			{Price: 3010, Quantity: 15.0},
			{Price: 3020, Quantity: 25.0},
		},
	}
	
	// 处理数据
	calculator.ProcessDepthData(symbol1, btcDepth)
	calculator.ProcessDepthData(symbol2, ethDepth)
	
	// 获取各自的订单簿数据
	btcOrderBook := calculator.GetCurrentOrderBookData(symbol1, 3)
	ethOrderBook := calculator.GetCurrentOrderBookData(symbol2, 3)
	
	// 验证数据独立性
	fmt.Printf("\n=== BTC订单簿数据 ===\n")
	fmt.Printf("失衡比例: %.4f\n", btcOrderBook.ImbalanceRatio)
	fmt.Printf("买方压力: %.2f USD\n", btcOrderBook.BidPressure)
	fmt.Printf("卖方压力: %.2f USD\n", btcOrderBook.AskPressure)
	fmt.Printf("数据是否过期: %v\n", btcOrderBook.IsStale)
	
	fmt.Printf("\n=== ETH订单簿数据 ===\n")
	fmt.Printf("失衡比例: %.4f\n", ethOrderBook.ImbalanceRatio)
	fmt.Printf("买方压力: %.2f USD\n", ethOrderBook.BidPressure)
	fmt.Printf("卖方压力: %.2f USD\n", ethOrderBook.AskPressure)
	fmt.Printf("数据是否过期: %v\n", ethOrderBook.IsStale)
	
	// 验证墙信息是否独立
	fmt.Printf("\n=== 挂单墙信息 ===\n")
	if btcOrderBook.NearestResistance != nil {
		fmt.Printf("BTC阻力墙: %.2f USD (强度: %.2f)\n", 
			btcOrderBook.NearestResistance.Price, 
			btcOrderBook.NearestResistance.StrengthUSD)
	} else {
		fmt.Printf("BTC: 无阻力墙\n")
	}
	
	if ethOrderBook.NearestResistance != nil {
		fmt.Printf("ETH阻力墙: %.2f USD (强度: %.2f)\n", 
			ethOrderBook.NearestResistance.Price, 
			ethOrderBook.NearestResistance.StrengthUSD)
	} else {
		fmt.Printf("ETH: 无阻力墙\n")
	}
	
	// 测试数据隔离 - 更新BTC数据不应该影响ETH
	fmt.Printf("\n=== 测试数据隔离 ===\n")
	
	// 更新BTC数据
	btcDepthUpdated := &microstructure.DepthData{
		Symbol:    symbol1,
		Timestamp: now.Add(time.Minute),
		Bids: []microstructure.OrderBookLevel{
			{Price: 51000, Quantity: 5.0}, // 更高的出价
			{Price: 50900, Quantity: 3.0},
		},
		Asks: []microstructure.OrderBookLevel{
			{Price: 51100, Quantity: 2.0},
			{Price: 51200, Quantity: 1.0},
		},
	}
	
	calculator.ProcessDepthData(symbol1, btcDepthUpdated)
	
	// 重新获取数据
	btcOrderBookUpdated := calculator.GetCurrentOrderBookData(symbol1, 3)
	ethOrderBookAfterBtcUpdate := calculator.GetCurrentOrderBookData(symbol2, 3)
	
	fmt.Printf("BTC更新后 - 买方压力: %.2f USD\n", btcOrderBookUpdated.BidPressure)
	fmt.Printf("ETH未更新 - 买方压力: %.2f USD (应该不变)\n", ethOrderBookAfterBtcUpdate.BidPressure)
	
	// 验证数据确实独立
	if ethOrderBook.BidPressure == ethOrderBookAfterBtcUpdate.BidPressure {
		fmt.Printf("✅ 数据独立性验证成功: ETH数据未受BTC更新影响\n")
	} else {
		fmt.Printf("❌ 数据独立性验证失败: ETH数据被意外修改\n")
	}
	
	// 测试不存在symbol的情况
	fmt.Printf("\n=== 测试不存在的交易对 ===\n")
	unknownOrderBook := calculator.GetCurrentOrderBookData("ADAUSDT", 3)
	if unknownOrderBook.IsStale {
		fmt.Printf("✅ 未知交易对正确返回过期数据\n")
	} else {
		fmt.Printf("❌ 未知交易对处理异常\n")
	}
	
	fmt.Printf("\n🎉 多交易对数据独立性测试完成!\n")
}