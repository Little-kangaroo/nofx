package market

import (
	"testing"
)

// TestParseExchangeMetaFromSymbolInfo 测试从SymbolInfo解析ExchangeMeta
// 🔥 P0-2单元测试：验证tick_size和lot_size解析逻辑
func TestParseExchangeMetaFromSymbolInfo(t *testing.T) {
	// 模拟Binance API返回的SymbolInfo
	symbolInfo := &SymbolInfo{
		Symbol: "BTCUSDT",
		Status: "TRADING",
		Filters: []Filter{
			{
				FilterType: "PRICE_FILTER",
				TickSize:   "0.10",
				MinPrice:   "100.00",
				MaxPrice:   "100000.00",
			},
			{
				FilterType: "LOT_SIZE",
				StepSize:   "0.001",
				MinQty:     "0.001",
				MaxQty:     "1000.00",
			},
		},
	}

	// 解析ExchangeMeta
	meta := parseExchangeMetaFromSymbolInfo(symbolInfo)

	// 验证结果
	if meta == nil {
		t.Fatal("parseExchangeMetaFromSymbolInfo 返回 nil")
	}

	if meta.Symbol != "BTCUSDT" {
		t.Errorf("Symbol 不匹配: 期望 BTCUSDT, 实际 %s", meta.Symbol)
	}

	if meta.TickSize != 0.10 {
		t.Errorf("TickSize 不匹配: 期望 0.10, 实际 %f", meta.TickSize)
	}

	if meta.LotSize != 0.001 {
		t.Errorf("LotSize 不匹配: 期望 0.001, 实际 %f", meta.LotSize)
	}

	if meta.MinPrice != 100.00 {
		t.Errorf("MinPrice 不匹配: 期望 100.00, 实际 %f", meta.MinPrice)
	}

	if meta.MaxPrice != 100000.00 {
		t.Errorf("MaxPrice 不匹配: 期望 100000.00, 实际 %f", meta.MaxPrice)
	}

	if meta.MinQty != 0.001 {
		t.Errorf("MinQty 不匹配: 期望 0.001, 实际 %f", meta.MinQty)
	}

	if meta.MaxQty != 1000.00 {
		t.Errorf("MaxQty 不匹配: 期望 1000.00, 实际 %f", meta.MaxQty)
	}
}

// TestParseExchangeMetaWithMissingFilters 测试缺少filters时的fallback行为
func TestParseExchangeMetaWithMissingFilters(t *testing.T) {
	symbolInfo := &SymbolInfo{
		Symbol:  "ETHUSDT",
		Status:  "TRADING",
		Filters: []Filter{}, // 空filters
	}

	meta := parseExchangeMetaFromSymbolInfo(symbolInfo)

	if meta == nil {
		t.Fatal("parseExchangeMetaFromSymbolInfo 返回 nil")
	}

	// 验证使用了默认值
	if meta.TickSize != 0.01 {
		t.Errorf("TickSize 默认值不匹配: 期望 0.01, 实际 %f", meta.TickSize)
	}

	if meta.LotSize != 1.0 {
		t.Errorf("LotSize 默认值不匹配: 期望 1.0, 实际 %f", meta.LotSize)
	}
}

// TestCreateFallbackExchangeMeta 测试fallback ExchangeMeta创建
func TestCreateFallbackExchangeMeta(t *testing.T) {
	symbol := "BTCUSDT"
	meta := createFallbackExchangeMeta(symbol)

	if meta == nil {
		t.Fatal("createFallbackExchangeMeta 返回 nil")
	}

	if meta.Symbol != symbol {
		t.Errorf("Symbol 不匹配: 期望 %s, 实际 %s", symbol, meta.Symbol)
	}

	if meta.TickSize <= 0 {
		t.Errorf("TickSize 必须 > 0, 实际 %f", meta.TickSize)
	}

	if meta.LotSize <= 0 {
		t.Errorf("LotSize 必须 > 0, 实际 %f", meta.LotSize)
	}
}

// TestExtractExchangeMetaForAI 测试extractExchangeMetaForAI函数
func TestExtractExchangeMetaForAI(t *testing.T) {
	// 创建测试Data
	data := &Data{
		Symbol: "BTCUSDT",
		ExchangeMeta: &ExchangeMeta{
			Symbol:   "BTCUSDT",
			TickSize: 0.10,
			LotSize:  0.001,
			MinPrice: 100.00,
			MaxPrice: 100000.00,
			MinQty:   0.001,
			MaxQty:   1000.00,
		},
	}

	// 调用extractExchangeMetaForAI
	result := extractExchangeMetaForAI(data)

	// 验证结果
	if result == nil {
		t.Fatal("extractExchangeMetaForAI 返回 nil")
	}

	if result["symbol"] != "BTCUSDT" {
		t.Errorf("symbol 不匹配")
	}

	if result["tick_size"] != 0.10 {
		t.Errorf("tick_size 不匹配")
	}

	if result["lot_size"] != 0.001 {
		t.Errorf("lot_size 不匹配")
	}

	// 验证可选字段
	if result["min_price"] != 100.00 {
		t.Errorf("min_price 不匹配")
	}

	if result["max_price"] != 100000.00 {
		t.Errorf("max_price 不匹配")
	}
}

// TestExtractExchangeMetaForAI_NilData 测试nil数据处理
func TestExtractExchangeMetaForAI_NilData(t *testing.T) {
	result := extractExchangeMetaForAI(nil)

	if result == nil {
		t.Fatal("extractExchangeMetaForAI 对nil应返回空map而非nil")
	}

	if len(result) != 0 {
		t.Errorf("extractExchangeMetaForAI 对nil应返回空map, 实际长度 %d", len(result))
	}
}
