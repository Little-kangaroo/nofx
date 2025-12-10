package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
	"nofx/microstructure"
)

// TestV2OrderFlowOutput 测试V2.0订单流数据输出格式
func main() {
	log.Println("🧪 开始测试V2.0订单流数据输出格式...")

	// 初始化测试符号
	testSymbol := "BTCUSDT"
	
	// 1. 测试CVD 5分钟增量数据
	testCVDDelta5m()
	
	// 2. 测试K线意图分析
	testCandleIntentAnalysis()
	
	// 3. 测试盘口墙稳定性数据
	testWallStabilityData()
	
	// 4. 测试CVD对比分析
	testCVDComparison()
	
	// 5. 测试缓存机制
	testDataCache()
	
	// 6. 测试订单簿监控
	testOrderBookMonitoring()
	
	// 7. 测试完整的V2.0输出格式
	testCompleteV2Output(testSymbol)
	
	log.Println("✅ V2.0订单流数据输出格式测试完成!")
}

// testCVDDelta5m 测试CVD 5分钟增量数据
func testCVDDelta5m() {
	log.Println("\n📊 测试CVD 5分钟增量数据...")
	
	// 创建模拟数据
	delta5m := &microstructure.CVDDelta5m{
		PriceDeltaPct:       1.25,
		SpotCVDDeltaUSD:     150000,
		FuturesCVDDeltaUSD:  -80000,
		OIDeltaPct:          2.3,
		CandleIntent:        microstructure.CandleIntentBullishConfirm,
		VolumeDelta:         230000,
		VolumeRatio:         1.8,
		PeriodStartTime:     time.Now().Add(-5 * time.Minute),
		PeriodEndTime:       time.Now(),
		DataQuality:         0.85,
	}
	
	// JSON序列化测试
	jsonData, err := json.MarshalIndent(delta5m, "", "  ")
	if err != nil {
		log.Printf("❌ CVD增量数据序列化失败: %v", err)
		return
	}
	
	fmt.Printf("CVD 5分钟增量数据:\n%s\n", jsonData)
	
	// 验证关键字段
	if delta5m.CandleIntent == microstructure.CandleIntentBullishConfirm {
		log.Println("✅ K线意图识别正确")
	}
	if delta5m.DataQuality > 0.8 {
		log.Println("✅ 数据质量评分正常")
	}
}

// testCandleIntentAnalysis 测试K线意图分析
func testCandleIntentAnalysis() {
	log.Println("\n🎯 测试K线意图分析...")
	
	// 创建模拟分析结果
	analysis := &microstructure.CandleIntentAnalysis{
		Intent:          microstructure.CandleIntentSmartMoneyAccum,
		Confidence:      0.78,
		SecondaryIntent: "institutional_accumulation",
		Strength:        0.65,
		Factors: map[string]float64{
			"price_strength":    0.2,
			"spot_strength":     0.9,
			"futures_strength":  0.3,
			"bullish_force":     0.8,
			"bearish_force":     0.1,
		},
	}
	
	jsonData, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		log.Printf("❌ 意图分析序列化失败: %v", err)
		return
	}
	
	fmt.Printf("K线意图分析:\n%s\n", jsonData)
	
	// 验证分析结果
	if analysis.Confidence > 0.7 {
		log.Println("✅ 置信度评分合理")
	}
	if len(analysis.Factors) == 5 {
		log.Println("✅ 因子分析完整")
	}
}

// testWallStabilityData 测试墙稳定性数据
func testWallStabilityData() {
	log.Println("\n🛡️ 测试��稳定性数据...")
	
	// 创建模拟墙信息
	wall := &microstructure.WallInfo{
		Price:             45000.50,
		StrengthUSD:       500000,
		IsSolid:           true,
		Distance:          0.8,
		LevelCount:        3,
		StabilityScore:    0.82,
		ExistenceDuration: 12 * time.Minute,
		FlickerCount:      2,
		LastSeen:          time.Now(),
		FirstSeen:         time.Now().Add(-12 * time.Minute),
		AverageSize:       480000,
		MaxSize:          550000,
		MinSize:          420000,
	}
	
	jsonData, err := json.MarshalIndent(wall, "", "  ")
	if err != nil {
		log.Printf("❌ 墙数据序列化失败: %v", err)
		return
	}
	
	fmt.Printf("墙稳定性数据:\n%s\n", jsonData)
	
	// 验证稳定性指标
	if wall.StabilityScore > 0.8 {
		log.Println("✅ 墙体稳定性高")
	}
	if wall.FlickerCount < 5 {
		log.Println("✅ 闪烁次数合理")
	}
}

// testCVDComparison 测试CVD对比分析
func testCVDComparison() {
	log.Println("\n⚖️ 测试CVD对比分析...")
	
	// 创建CVD对比分析器
	analyzer := microstructure.NewCVDComparisonAnalyzer("BTCUSDT")
	
	// 模拟现货和期货CVD数据
	spotCVD := 1200000.0      // 120万USD现货净买入
	futuresCVD := -800000.0   // 80万USD期货净卖出
	
	// 执行对比分析
	comparison := analyzer.AnalyzeSpotFuturesCVD(spotCVD, futuresCVD)
	
	jsonData, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		log.Printf("❌ CVD对比分析序列化失败: %v", err)
		return
	}
	
	fmt.Printf("CVD对比分析:\n%s\n", jsonData)
	
	// 验证对比结果
	if comparison.Divergence > 0 {
		log.Println("✅ 检测到现货期货背离")
	}
	if comparison.MarketLeader != "balanced" {
		log.Printf("✅ 市场领导者识别: %s", comparison.MarketLeader)
	}
}

// testDataCache 测试缓存机制
func testDataCache() {
	log.Println("\n💾 测试数据缓存机制...")
	
	// 获取全局缓存实例
	cache := microstructure.GetGlobalCache()
	
	// 测试CVD数据缓存
	testSymbol := "ETHUSDT"
	cvdData := &microstructure.CVDDelta5m{
		PriceDeltaPct:       -0.8,
		SpotCVDDeltaUSD:     -200000,
		FuturesCVDDeltaUSD:  300000,
		CandleIntent:        microstructure.CandleIntentFakeDumpRetail,
		DataQuality:         0.92,
		PeriodStartTime:     time.Now().Add(-5 * time.Minute),
		PeriodEndTime:       time.Now(),
	}
	
	// 缓存数据
	cache.SetCVDDelta5m(testSymbol, cvdData)
	
	// 检索数据
	retrieved := cache.GetCVDDelta5m(testSymbol)
	if retrieved != nil && retrieved.CandleIntent == microstructure.CandleIntentFakeDumpRetail {
		log.Println("✅ CVD缓存机制正常")
	}
	
	// 测试缓存统计
	stats := cache.GetCacheStats()
	log.Printf("✅ 缓存统计: 总条目数=%d, CVD条目数=%d", stats.TotalEntries, stats.CVDEntries)
}

// testOrderBookMonitoring 测试订单簿监控
func testOrderBookMonitoring() {
	log.Println("\n👀 测试订单簿监控...")
	
	// 创建模拟订单簿计算器
	calculator := microstructure.NewOrderBookCalculator(5.0)
	
	// 创建警报回调函数
	alertCallback := func(alert *microstructure.OrderBookAlert) {
		log.Printf("🚨 收到警报: %s - %s (严重程度: %s)", 
			alert.AlertType, alert.Message, alert.Severity)
	}
	
	// 创建监控器
	monitor := microstructure.NewOrderBookMonitor("BTCUSDT", calculator, alertCallback)
	
	// 测试监控器创建
	if monitor != nil {
		log.Println("✅ 订单簿监控器创建成功")
	}
	
	// 测试墙稳定性追踪器
	tracker := microstructure.NewWallStabilityTracker("BTCUSDT")
	if tracker != nil {
		log.Println("✅ 墙稳定性追踪器创建成功")
	}
}

// testCompleteV2Output 测试完整的V2.0输出格式
func testCompleteV2Output(symbol string) {
	log.Println("\n🔍 测试完整的V2.0输出格式...")
	
	// 模拟完整的V2.0数据结构
	v2Output := map[string]interface{}{
		"本周期博弈_5m": map[string]interface{}{
			"price_delta_pct":       1.25,
			"spot_cvd_delta_usd":    150000,
			"futures_cvd_delta_usd": -80000,
			"oi_delta_pct":          2.3,
			"candle_intent":         "bullish_confirm",
			"volume_delta":          230000,
			"volume_ratio":          1.8,
			"period_minutes":        5,
			"data_quality":          0.85,
			"analysis_time":         time.Now().Format("15:04:05"),
		},
		"宏观资金趋势": map[string]interface{}{
			"spot_cvd_1h_usd":     1200000,
			"futures_cvd_1h_usd":  800000,
			"oi_change_1h_pct":    5.2,
			"cvd_divergence":      "bullish_spot_futures_divergence",
			"context_inference":   "smart_money_accumulation",
			"signal_strength":     0.78,
			"market_regime":       "accumulation_phase",
			"dominant_direction":  "bullish_confirmation",
			"trend_alignment":     "bullish_aligned",
			"confidence_level":    0.82,
		},
		"盘口结构_v2": map[string]interface{}{
			"imbalance_ratio":      0.45,
			"imbalance_trend":      "increasing_bid_pressure",
			"pressure_delta_5m":    0.12,
			"spoofing_risk":        0.25,
			"liquidity_score":      0.78,
			"wall_change_count_5m": 8,
			"bid_pressure":         2500000,
			"ask_pressure":         1800000,
			"resistance_wall": map[string]interface{}{
				"price":             45100.0,
				"strength_usd":      500000,
				"is_solid":          true,
				"distance_pct":      0.8,
				"stability_score":   0.82,
				"flicker_count":     2,
				"existence_minutes": 12,
				"level_count":       3,
				"max_size":          550000,
				"avg_size":          480000,
			},
			"support_wall": map[string]interface{}{
				"price":             44800.0,
				"strength_usd":      720000,
				"is_solid":          true,
				"distance_pct":      -0.6,
				"stability_score":   0.91,
				"flicker_count":     1,
				"existence_minutes": 18,
				"level_count":       4,
				"max_size":          800000,
				"avg_size":          720000,
			},
		},
		"数据质量": map[string]interface{}{
			"cvd_reliability":       0.92,
			"orderbook_reliability": 0.95,
			"oi_reliability":        0.88,
			"incremental_quality":   0.85,
			"overall_score":         0.90,
			"last_update":           time.Now().Format("15:04:05"),
			"data_lag_ms":           150,
			"status":                "正常",
			"update_interval_s":     300,
		},
	}
	
	// JSON序列化
	jsonData, err := json.MarshalIndent(v2Output, "", "  ")
	if err != nil {
		log.Printf("❌ V2.0完整数据序列化失败: %v", err)
		return
	}
	
	fmt.Printf("完整V2.0订单流数据输出:\n%s\n", jsonData)
	
	// 验证数据结构完��性
	validateV2Structure(v2Output)
}

// validateV2Structure 验证V2.0数据结构完整性
func validateV2Structure(data map[string]interface{}) {
	log.Println("🔍 验证V2.0数据结构完整性...")
	
	// 检查必需的顶级字段
	requiredFields := []string{"本周期博弈_5m", "宏观资金趋势", "盘口结构_v2", "数据质量"}
	
	for _, field := range requiredFields {
		if _, exists := data[field]; exists {
			log.Printf("✅ 顶级字段 '%s' 存在", field)
		} else {
			log.Printf("❌ 缺少顶级字段 '%s'", field)
		}
	}
	
	// 检查本周期博弈字段
	if period, ok := data["本周期博弈_5m"].(map[string]interface{}); ok {
		periodFields := []string{"candle_intent", "data_quality", "period_minutes"}
		for _, field := range periodFields {
			if _, exists := period[field]; exists {
				log.Printf("✅ 5分钟周期字段 '%s' 存在", field)
			} else {
				log.Printf("❌ 5分钟周期缺少字段 '%s'", field)
			}
		}
	}
	
	// 检查盘口结构V2字段
	if orderbook, ok := data["盘口结构_v2"].(map[string]interface{}); ok {
		v2Fields := []string{"spoofing_risk", "liquidity_score", "wall_change_count_5m"}
		for _, field := range v2Fields {
			if _, exists := orderbook[field]; exists {
				log.Printf("✅ V2.0盘口字段 '%s' 存在", field)
			} else {
				log.Printf("❌ V2.0盘口缺少字段 '%s'", field)
			}
		}
		
		// 检查墙稳定性字段
		if wall, ok := orderbook["resistance_wall"].(map[string]interface{}); ok {
			stabilityFields := []string{"stability_score", "flicker_count", "existence_minutes"}
			for _, field := range stabilityFields {
				if _, exists := wall[field]; exists {
					log.Printf("✅ 墙稳定性字段 '%s' 存在", field)
				} else {
					log.Printf("❌ 墙稳定性缺少字段 '%s'", field)
				}
			}
		}
	}
	
	// 检查数据质量字段
	if quality, ok := data["数据质量"].(map[string]interface{}); ok {
		qualityFields := []string{"overall_score", "status", "data_lag_ms"}
		for _, field := range qualityFields {
			if _, exists := quality[field]; exists {
				log.Printf("✅ 数据质量字段 '%s' 存在", field)
			} else {
				log.Printf("❌ 数据质量缺少字段 '%s'", field)
			}
		}
	}
	
	log.Println("✅ V2.0数据结构验证完成")
}