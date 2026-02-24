package tests

import (
	"encoding/json"
	"io/ioutil"
	"nofx/internal/direction"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// TestProductionRecords 测试生产环境的决策记录
func TestProductionRecords(t *testing.T) {
	recordsDir := "../../../records"

	// 读取所有 .txt 文件
	files, err := filepath.Glob(filepath.Join(recordsDir, "*.txt"))
	if err != nil {
		t.Fatalf("读取文件列表失败: %v", err)
	}

	if len(files) == 0 {
		t.Fatalf("未找到任何测试文件")
	}

	// 按文件名排序
	sort.Strings(files)

	t.Logf("找到 %d 个测试文件", len(files))

	// 统计结果
	stats := &TestStats{
		Total:      0,
		Success:    0,
		Failed:     0,
		Directions: make(map[direction.DirectionSide]int),
		Flags:      make(map[string]int),
	}

	// 处理每个文件
	for i, file := range files {
		t.Logf("\n========== 测试文件 %d/%d: %s ==========", i+1, len(files), filepath.Base(file))

		// 读取文件内容
		content, err := ioutil.ReadFile(file)
		if err != nil {
			t.Logf("读取文件失败: %v", err)
			stats.Failed++
			continue
		}

		// 提取市场数据
		marketData := extractMarketData(string(content))
		if len(marketData) == 0 {
			t.Logf("未找到市场数据")
			stats.Failed++
			continue
		}

		// 测试每个币种
		for symbol, data := range marketData {
			stats.Total++

			mtfData, ok := data["多时间框架分析"]
			if !ok {
				t.Logf("  [%s] 缺少多时间框架分析数据，跳过", symbol)
				stats.Failed++
				continue
			}

			var result *direction.DirectionArbitration
			orderflowData, hasOF := data["订单流分析"]
			if hasOF {
				result = computeDirection(symbol, orderflowData, mtfData)
			} else {
				// 旧格式记录无订单流字段，使用中性订单流隔离结构方向
				t.Logf("  [%s] 无订单流分析数据，使用中性订单流", symbol)
				mtf := parseMTFData(mtfData)
				if mtf == nil {
					t.Logf("  [%s] MTF 解析失败，跳过", symbol)
					stats.Failed++
					continue
				}
				cfg := direction.DefaultConfig()
				neutral := direction.Orderflow{
					Quality: direction.Quality{Status: "正常", OverallScore: 0.70, DataLagMs: 100, UpdateIntervalS: 5,
						CvdReliability: 0.70, OIReliability: 0.70, OrderbookReliability: 0.70},
					Macro:   direction.Macro{TrendAlignment: "divergent_mixed"},
					Micro5m: direction.Micro5m{CandleIntent: "mixed", DataQuality: 0.70},
					OB:      direction.Orderbook{BidPressure: 0.50, AskPressure: 0.50, LiquidityScore: 0.50},
				}
				r := direction.ComputeDirectionArbitration(direction.RootSymbolInput{
					Symbol:    symbol,
					Orderflow: neutral,
					MTF:       mtf,
				}, cfg)
				result = &r
			}

			if result == nil {
				t.Logf("  [%s] 方向裁决计算失败，跳过", symbol)
				stats.Failed++
				continue
			}

			// 记录统计
			stats.Success++
			stats.Directions[result.PlanSide]++
			for _, flag := range result.Flags {
				stats.Flags[flag]++
			}

			// 输出结果
			t.Logf("  [%s] plan_side=%s | conf=%.2f | delta=%.2f | of_dir=%.2f | struct_dir=%.2f | block=%v | flags=%v",
				symbol, result.PlanSide, result.Confidence, result.Delta,
				result.OFDir, result.StructDir, result.BlockEntry, result.Flags)
		}
	}

	// 输出统计结果
	t.Logf("\n========== 测试统计 ==========")
	t.Logf("总计: %d", stats.Total)
	t.Logf("成功: %d (%.1f%%)", stats.Success, float64(stats.Success)/float64(stats.Total)*100)
	t.Logf("失败: %d (%.1f%%)", stats.Failed, float64(stats.Failed)/float64(stats.Total)*100)

	t.Logf("\n方向分布:")
	for side, count := range stats.Directions {
		t.Logf("  %s: %d (%.1f%%)", side, count, float64(count)/float64(stats.Success)*100)
	}

	t.Logf("\nFlags 分布 (Top 10):")
	flagsList := make([]FlagCount, 0, len(stats.Flags))
	for flag, count := range stats.Flags {
		flagsList = append(flagsList, FlagCount{Flag: flag, Count: count})
	}
	sort.Slice(flagsList, func(i, j int) bool {
		return flagsList[i].Count > flagsList[j].Count
	})
	for i, fc := range flagsList {
		if i >= 10 {
			break
		}
		t.Logf("  %s: %d (%.1f%%)", fc.Flag, fc.Count, float64(fc.Count)/float64(stats.Success)*100)
	}
}

// TestStats 测试统计
type TestStats struct {
	Total      int
	Success    int
	Failed     int
	Directions map[direction.DirectionSide]int
	Flags      map[string]int
}

// FlagCount Flag计数
type FlagCount struct {
	Flag  string
	Count int
}

// extractMarketData 从文件内容中提取市场数据
func extractMarketData(content string) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})

	// 查找所有币种的JSON数据
	// 格式: {"SYMBOL":{"基础指标":...,"多时间框架分析":...,"订单流分析":...}}
	re := regexp.MustCompile(`\{"\w+USDT":\{.*?\}\}(?:\n|$)`)
	matches := re.FindAllString(content, -1)

	for _, match := range matches {
		// 解析JSON
		var data map[string]map[string]interface{}
		if err := json.Unmarshal([]byte(match), &data); err != nil {
			continue
		}

		// 提取币种数据
		for symbol, symbolData := range data {
			result[symbol] = symbolData
		}
	}

	return result
}

// computeDirection 计算方向裁决
func computeDirection(symbol string, orderflowData interface{}, mtfData interface{}) *direction.DirectionArbitration {
	// 构建输入数据
	input := buildDirectionInput(symbol, orderflowData, mtfData)
	if input == nil {
		return nil
	}

	// 使用默认配置
	cfg := direction.DefaultConfig()

	// 计算方向裁决
	result := direction.ComputeDirectionArbitration(*input, cfg)

	return &result
}

// buildDirectionInput 构建方向裁决输入数据
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
	// 将 interface{} 转换为 map
	rawData, ok := data.(map[string]interface{})
	if !ok {
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
		CandleIntent:    getStringOrDefault(microData, "candle_intent", ""),
		FuturesCvdDelta: getFloatOrDefault(microData, "futures_cvd_delta_usd", 0),
		SpotCvdDelta:    getFloatOrDefault(microData, "spot_cvd_delta_usd", 0),
		OIDeltaPct:      getFloatOrDefault(microData, "oi_delta_pct", 0),
		PriceDeltaPct:   getFloatOrDefault(microData, "price_delta_pct", 0),
		VolumeDelta:     getFloatOrDefault(microData, "volume_delta", 0),
		DataQuality:     getFloatOrDefault(microData, "data_quality", 0),
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
func parseMTFData(data interface{}) direction.MTFAnalysis {
	// 将 interface{} 转换为 map
	rawData, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}

	mtf := make(direction.MTFAnalysis)
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
	chData, ok := data["通道数据"].(map[string]interface{})
	if !ok {
		return direction.ChannelInfo{
			Direction:       "sideways",
			CurrentPosition: "inside",
			PriceRatio:      0.5,
			Quality:         0.5,
		}
	}

	return direction.ChannelInfo{
		Direction:       getStringOrDefault(chData, "channel_direction", "sideways"),
		CurrentPosition: getStringOrDefault(chData, "current_position", "inside"),
		PriceRatio:      getFloatOrDefault(chData, "price_ratio", 0.5),
		Quality:         getFloatOrDefault(chData, "quality", 0.5),
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

// TestMain 测试入口
func TestMain(m *testing.M) {
	// 运行测试
	code := m.Run()
	os.Exit(code)
}
