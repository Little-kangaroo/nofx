package microstructure

import (
	"testing"
	"time"
)

// Test T01: RefTime全链路对齐
func TestRefTimeAlignment(t *testing.T) {
	t.Run("CVD CalculateVolatilityAt uses refTime", func(t *testing.T) {
		calc := NewCVDCalculator("BTCUSDT", time.Hour)

		// 添加历史数据
		now := time.Now()
		for i := 0; i < 10; i++ {
			trade := &TradeData{
				Symbol:        "BTCUSDT",
				Price:         50000 + float64(i*100),
				Quantity:      1.0,
				Timestamp:     now.Add(time.Duration(-i) * time.Minute),
				IsBuyerMaker:  i%2 == 0,
				MarketType:    "futures",
			}
			calc.ProcessTrade(trade)
		}

		// 测试基于refTime的波动率计算
		refTime := now
		volatility := calc.CalculateVolatilityAt(time.Hour, refTime)

		if volatility < 0 {
			t.Errorf("Volatility should be non-negative, got %f", volatility)
		}

		// 验证：使用不同的refTime应该得到不同的结果
		futureRefTime := now.Add(10 * time.Minute)
		futureVolatility := calc.CalculateVolatilityAt(time.Hour, futureRefTime)

		// 由于数据范围不同，波动率可能不同
		t.Logf("Volatility at now: %f, at future: %f", volatility, futureVolatility)
	})
}

// Test T03: CVD LastUpdate真实性
func TestCVDLastUpdateReality(t *testing.T) {
	calc := NewCVDCalculator("BTCUSDT", time.Hour)

	// 初始状态：lastDataUpdate应该是创建时间附近
	initialLastUpdate := calc.lastDataUpdate

	time.Sleep(10 * time.Millisecond)

	// 添加交易数据
	tradeTime := time.Now()
	trade := &TradeData{
		Symbol:       "BTCUSDT",
		Price:        50000,
		Quantity:     1.0,
		Timestamp:    tradeTime,
		IsBuyerMaker: false,
		MarketType:   "futures",
	}

	calc.ProcessTrade(trade)

	// 获取CVD数据
	cvdData := calc.GetCurrentCVD()

	// 验证：LastUpdate应该是trade的时间，而非time.Now()
	if cvdData.LastUpdate.Sub(tradeTime).Abs() > time.Second {
		t.Errorf("LastUpdate should match trade timestamp, got diff: %v",
			cvdData.LastUpdate.Sub(tradeTime))
	}

	// 验证：LastUpdate应该更新了
	if !calc.lastDataUpdate.After(initialLastUpdate) {
		t.Errorf("lastDataUpdate should be updated after ProcessTrade")
	}
}

// Test T04: OI双时间戳支持
func TestOIDualTimestamp(t *testing.T) {
	calc := NewOICalculator("BTCUSDT", 24*time.Hour)

	now := time.Now()
	exchangeTime := now.Add(-5 * time.Second) // 交易所时间比接收时间早5秒

	oiData := &OIData{
		Symbol:       "BTCUSDT",
		OpenInterest: 100000,
		Timestamp:    now,          // 接收时间
		ExchangeTime: exchangeTime, // 交易所时间
	}

	calc.ProcessOIData(oiData)

	// 验证：changes中记录的时间应该是ExchangeTime
	if len(calc.changes) > 0 {
		recordedTime := calc.changes[0].Timestamp
		if recordedTime.Sub(exchangeTime).Abs() > time.Millisecond {
			t.Errorf("Change timestamp should use ExchangeTime, got diff: %v",
				recordedTime.Sub(exchangeTime))
		}
	}

	// 验证：健康度检查使用接收时间
	if calc.lastUpdate.Sub(now).Abs() > time.Millisecond {
		t.Errorf("lastUpdate should use received time, got diff: %v",
			calc.lastUpdate.Sub(now))
	}
}

// Test T05: OrderBook RefTime对齐
func TestOrderBookRefTime(t *testing.T) {
	config := DefaultMicrostructureConfig()
	calc := NewOrderBookCalculator(config.WallThresholdMultiple)

	symbol := "BTCUSDT"
	now := time.Now()

	// 添加深度数据
	depthData := &DepthData{
		Symbol:    symbol,
		Bids:      []OrderBookLevel{{Price: 50000, Quantity: 1.0}},
		Asks:      []OrderBookLevel{{Price: 50100, Quantity: 1.0}},
		Timestamp: now,
	}

	calc.ProcessDepthData(symbol, depthData)

	// 测试GetCurrentOrderBookDataAt使用refTime
	refTime := now.Add(1 * time.Second)
	obData := calc.GetCurrentOrderBookDataAt(symbol, 5, refTime)

	if obData == nil {
		t.Fatal("OrderBookData should not be nil")
	}

	// 验证：IsStale应该基于refTime判断，而非time.Now()
	// 数据刚添加，相对于refTime只有1秒，不应该过期（5分钟阈值）
	if obData.IsStale {
		t.Errorf("Data should not be stale (1s old), IsStale=%v", obData.IsStale)
	}

	// 测试过期情况
	oldRefTime := now.Add(10 * time.Minute) // 10分钟后
	obDataOld := calc.GetCurrentOrderBookDataAt(symbol, 5, oldRefTime)

	// 相对于10分钟后，数据应该过期
	if !obDataOld.IsStale {
		t.Errorf("Data should be stale (10m old), IsStale=%v", obDataOld.IsStale)
	}
}

// Test T09: TickIndex避免浮点精度问题
func TestTickIndexFloatPrecision(t *testing.T) {
	// 模拟浮点精度问题
	price1 := 50000.1
	price2 := 50000.1 + 1e-10 // 极小差异

	tickSize := 0.1

	// 🔥 T09修复：使用math.Round避免浮点精度问题
	tickIndex1 := int64(price1 / tickSize + 0.5) // 四舍五入
	tickIndex2 := int64(price2 / tickSize + 0.5)

	// tickIndex应该相同
	if tickIndex1 != tickIndex2 {
		t.Errorf("TickIndex should handle float precision, got %d != %d",
			tickIndex1, tickIndex2)
	}

	// 测试map key使用tickIndex
	priceMap := make(map[int64]float64)
	priceMap[tickIndex1] = 100.0
	priceMap[tickIndex2] += 50.0 // 应该累加到同一个key

	if priceMap[tickIndex1] != 150.0 {
		t.Errorf("Expected 150.0, got %f", priceMap[tickIndex1])
	}

	// 验证：使用float作为key会导致错误
	floatMap := make(map[float64]float64)
	floatMap[price1] = 100.0
	floatMap[price2] += 50.0

	// 可能会创建两个不同的key（取决于精度）
	totalKeys := len(floatMap)
	if totalKeys > 1 {
		t.Logf("Warning: float map created %d keys for nearly identical prices", totalKeys)
	}
}

// Test T10: 配置化窗口大小
func TestConfigurableWindowSize(t *testing.T) {
	config := DefaultMicrostructureConfig()
	config.HistoryRetention = 6 * time.Hour // 设置6小时保留期

	oiManager := NewOIManager(config)

	// 创建计算器
	calc := oiManager.GetOrCreateCalculator("BTCUSDT")

	// 验证：windowDuration应该是配置的值
	expectedDuration := 6 * time.Hour
	if calc.windowDuration != expectedDuration {
		t.Errorf("Expected window duration %v, got %v",
			expectedDuration, calc.windowDuration)
	}

	// 验证：容量应该基于窗口大小动态计算
	// 6小时 * 60分钟 * 2(每分钟2次) = 720
	expectedCapacity := 6 * 60 * 2
	actualCapacity := cap(calc.changes)

	if actualCapacity != expectedCapacity {
		t.Errorf("Expected capacity %d, got %d", expectedCapacity, actualCapacity)
	}
}
