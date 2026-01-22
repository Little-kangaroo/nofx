package market

import (
	"encoding/json"
	"log"
	"nofx/internal/direction"
)

// ComputeDirectionForSymbol 为单个币种计算方向裁决
// 从现有的市场数据和订单流数据中提取信息，转换为方向裁决模块需要的格式
func ComputeDirectionForSymbol(symbol string, orderflowData interface{}, mtfData interface{}) *direction.DirectionArbitration {
	// 使用默认配置
	cfg := direction.DefaultConfig()

	// 构建输入数据
	input := buildDirectionInput(symbol, orderflowData, mtfData)
	if input == nil {
		log.Printf("⚠️ [方向裁决] %s 输入数据构建失败", symbol)
		return nil
	}

	// 计算方向裁决
	result := direction.ComputeDirectionArbitration(*input, cfg)

	log.Printf("✅ [方向裁决] %s | plan_side=%s | confidence=%.2f | of_dir=%.2f | struct_dir=%.2f | flags=%v",
		symbol, result.PlanSide, result.Confidence, result.OFDir, result.StructDir, result.Flags)

	return &result
}

// buildDirectionInput 构建方向裁决输入数据
// 将现有的订单流和多时间框架数据转换为 direction.RootSymbolInput 格式
func buildDirectionInput(symbol string, orderflowData interface{}, mtfData interface{}) *direction.RootSymbolInput {
	// 解析订单流数据
	orderflow := parseOrderflowData(orderflowData)
	if orderflow == nil {
		return nil
	}

	// 解析多时间框架数据
	mtf := parseMTFData(mtfData)
	if mtf == nil {
		return nil
	}

	return &direction.RootSymbolInput{
		Symbol:    symbol,
		Orderflow: *orderflow,
		MTF:       mtf,
	}
}

// parseOrderflowData 解析订单流数据
func parseOrderflowData(data interface{}) *direction.Orderflow {
	// 将 interface{} 转换为 JSON，再解析为目标结构
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("❌ [方向裁决] 订单流数据序列化失败: %v", err)
		return nil
	}

	// 先解析为 map 以便灵活处理
	var rawData map[string]interface{}
	if err := json.Unmarshal(jsonData, &rawData); err != nil {
		log.Printf("❌ [方向裁决] 订单流数据解析失败: %v", err)
		return nil
	}

	// 构建 Orderflow 结构
	orderflow := &direction.Orderflow{
		Quality: parseQuality(rawData),
		Macro:   parseMacro(rawData),
		Micro5m: parseMicro5m(rawData),
		OB:      parseOrderbook(rawData),
	}

	return orderflow
}

// parseQuality 解析数据质量
func parseQuality(data map[string]interface{}) direction.Quality {
	qualityData, ok := data["数据质量"].(map[string]interface{})
	if !ok {
		return direction.Quality{Status: "异常", OverallScore: 0}
	}

	return direction.Quality{
		Status:               getStringOrDefault(qualityData, "status", "异常"),
		OverallScore:         getFloatOrDefault(qualityData, "overall_score", 0),
		DataLagMs:            int64(getFloatOrDefault(qualityData, "data_lag_ms", 0)),
		UpdateIntervalS:      int64(getFloatOrDefault(qualityData, "update_interval_s", 0)),
		CvdReliability:       getFloatOrDefault(qualityData, "cvd_reliability", 0),
		OIReliability:        getFloatOrDefault(qualityData, "oi_reliability", 0),
		OrderbookReliability: getFloatOrDefault(qualityData, "orderbook_reliability", 0),
	}
}

// parseMacro 解析宏观资金趋势
func parseMacro(data map[string]interface{}) direction.Macro {
	macroData, ok := data["宏观资金趋势"].(map[string]interface{})
	if !ok {
		return direction.Macro{}
	}

	return direction.Macro{
		TrendAlignment:  getStringOrDefault(macroData, "trend_alignment", ""),
		SignalStrength:  getFloatOrDefault(macroData, "signal_strength", 0),
		ConfidenceLevel: getFloatOrDefault(macroData, "confidence_level", 0),
		MarketRegime:    getStringOrDefault(macroData, "market_regime", ""),
		CvdDivergence:   getBoolOrDefault(macroData, "cvd_divergence", false),
	}
}

// parseMicro5m 解析5分钟微观博弈
func parseMicro5m(data map[string]interface{}) direction.Micro5m {
	microData, ok := data["本周期博弈_5m"].(map[string]interface{})
	if !ok {
		return direction.Micro5m{}
	}

	return direction.Micro5m{
		CandleIntent:      getStringOrDefault(microData, "candle_intent", ""),
		FuturesCvdDelta:   getFloatOrDefault(microData, "futures_cvd_delta_usd", 0),
		SpotCvdDelta:      getFloatOrDefault(microData, "spot_cvd_delta_usd", 0),
		OIDeltaPct:        getFloatOrDefault(microData, "oi_delta_pct", 0),
		PriceDeltaPct:     getFloatOrDefault(microData, "price_delta_pct", 0),
		VolumeDelta:       getFloatOrDefault(microData, "volume_delta", 0),
		DataQuality:       getFloatOrDefault(microData, "data_quality", 0),
	}
}

// parseOrderbook 解析盘口结构
func parseOrderbook(data map[string]interface{}) direction.Orderbook {
	obData, ok := data["盘口结构_v2"].(map[string]interface{})
	if !ok {
		return direction.Orderbook{}
	}

	return direction.Orderbook{
		ImbalanceRatio: getFloatOrDefault(obData, "imbalance_ratio", 0),
		BidPressure:    getFloatOrDefault(obData, "bid_pressure", 0),
		AskPressure:    getFloatOrDefault(obData, "ask_pressure", 0),
		LiquidityScore: getFloatOrDefault(obData, "liquidity_score", 0),
		SpoofingRisk:   getFloatOrDefault(obData, "spoofing_risk", 0),
		SupportWall:    parseWall(obData, "support_wall"),
		ResistanceWall: parseWall(obData, "resistance_wall"),
		WallChange5m:   int64(getFloatOrDefault(obData, "wall_change_count_5m", 0)),
	}
}

// parseWall 解析墙体信息
func parseWall(data map[string]interface{}, key string) *direction.Wall {
	wallData, ok := data[key].(map[string]interface{})
	if !ok || wallData == nil {
		return nil
	}

	return &direction.Wall{
		StrengthUSD:    getFloatOrDefault(wallData, "strength_usd", 0),
		DistancePct:    getFloatOrDefault(wallData, "distance_pct", 0),
		IsSolid:        getBoolOrDefault(wallData, "is_solid", false),
		StabilityScore: getFloatOrDefault(wallData, "stability_score", 0),
		FlickerCount:   int64(getFloatOrDefault(wallData, "flicker_count", 0)),
	}
}

// parseMTFData 解析多时间框架数据
// 🔥 优化：只解析 15m 和 30m 的通道数据，移除 SuperTrend 和道氏理论
func parseMTFData(data interface{}) direction.MTFAnalysis {
	// 将 interface{} 转换为 JSON，再解析为目标结构
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("❌ [方向裁决] 多时间框架数据序列化失败: %v", err)
		return nil
	}

	// 解析为 map
	var rawData map[string]interface{}
	if err := json.Unmarshal(jsonData, &rawData); err != nil {
		log.Printf("❌ [方向裁决] 多时间框架数据解析失败: %v", err)
		return nil
	}

	mtf := make(direction.MTFAnalysis)
	// 🔥 优化：只使用 15m 和 30m 时间框架
	timeframes := []string{"30m", "15m"}

	for _, tf := range timeframes {
		tfData, ok := rawData[tf].(map[string]interface{})
		if !ok {
			continue
		}

		mtf[tf] = direction.TimeframeData{
			Channel: parseChannel(tfData),
			VPVR:    parseVPVR(tfData),
		}
	}

	return mtf
}

// parseChannel 解析通道数据
func parseChannel(data map[string]interface{}) direction.ChannelInfo {
	chData, ok := data["通道分析数据"].(map[string]interface{})
	if !ok {
		return direction.ChannelInfo{
			Direction:       "sideways",
			CurrentPosition: "Inside",
			PriceRatio:      0.5,
			Quality:         0.0,
		}
	}

	return direction.ChannelInfo{
		Direction:       getStringOrDefault(chData, "direction", "sideways"),
		CurrentPosition: getStringOrDefault(chData, "current_position", "Inside"),
		PriceRatio:      getFloatOrDefault(chData, "price_ratio", 0.5),
		Quality:         getFloatOrDefault(chData, "quality", 0.0),
	}
}

// parseVPVR 解析VPVR数据
func parseVPVR(data map[string]interface{}) direction.VPVR {
	vpvrData, ok := data["VPVR数据"].(map[string]interface{})
	if !ok {
		return direction.VPVR{}
	}

	return direction.VPVR{
		DistToVAHATR: getFloatOrDefault(vpvrData, "dist_to_vah_atr", 0),
		DistToVALATR: getFloatOrDefault(vpvrData, "dist_to_val_atr", 0),
	}
}

// ========== 辅助函数 ==========

func getStringOrDefault(data map[string]interface{}, key string, defaultValue string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return defaultValue
}

func getFloatOrDefault(data map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := data[key].(float64); ok {
		return val
	}
	return defaultValue
}

func getBoolOrDefault(data map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := data[key].(bool); ok {
		return val
	}
	return defaultValue
}
