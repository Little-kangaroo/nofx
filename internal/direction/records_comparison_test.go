package direction

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestRecordsComparison 使用生产环境records数据对比修复前后的效果
func TestRecordsComparison(t *testing.T) {
	// 读取records文件
	recordFiles := []string{
		"../../records/data34.txt",
		"../../records/data35.txt",
		"../../records/data45.txt",
	}

	var allSymbols []SymbolTestCase
	for _, file := range recordFiles {
		symbols := extractSymbolsFromRecord(t, file)
		allSymbols = append(allSymbols, symbols...)
	}

	t.Logf("\n========== 生产环境数据对比测试 ==========")
	t.Logf("总样本数: %d", len(allSymbols))

	// 统计修复前后的差异
	var (
		beforeOpenCount    int
		beforeBlockedCount int
		beforeConflictCount int

		afterOpenCount    int
		afterBlockedCount int
		afterConflictCount int
		afterWeakSignalCount int
	)

	// 使用修复前的配置
	cfgBefore := Config{
		MinOverallScore: 0.65,
		MinMicroDQ:      0.55,
		SpoofHard:       0.75,
		SpoofSoft:       0.60,
		LiqThin:         0.20,
		WallFlickerN:    120,
		WOFMin:          0.70,
		WOFMax:          0.85,
		ThetaNeutral:    0.10, // 修复前的低阈值
		MinConf:         0.25, // 修复前的低阈值
		CVDFallbackScale: 1e6,
		OIScalePct:       0.30,
		WallScaleUSD:     1_000_000,
	}

	// 使用修复后的配置
	cfgAfter := DefaultConfig()

	t.Logf("\n配置对比:")
	t.Logf("  ThetaNeutral: %.2f → %.2f", cfgBefore.ThetaNeutral, cfgAfter.ThetaNeutral)
	t.Logf("  MinConf:      %.2f → %.2f", cfgBefore.MinConf, cfgAfter.MinConf)

	// 对每个样本进行测试
	for i, tc := range allSymbols {
		// 修复前的决策
		resultBefore := ComputeDirectionArbitration(tc.Input, cfgBefore)

		// 修复后的决策（包含弱信号过滤）
		resultAfter := computeDirectionArbitrationWithWeakFilter(tc.Input, cfgAfter)

		// 统计修复前
		if !resultBefore.BlockEntry && resultBefore.PlanSide != SideNeutral && resultBefore.PlanSide != SideUnknown {
			beforeOpenCount++
		} else {
			beforeBlockedCount++
			if hasFlag(resultBefore.Flags, "DIR_CONFLICT") {
				beforeConflictCount++
			}
		}

		// 统计修复后
		if !resultAfter.BlockEntry && resultAfter.PlanSide != SideNeutral && resultAfter.PlanSide != SideUnknown {
			afterOpenCount++
		} else {
			afterBlockedCount++
			if hasFlag(resultAfter.Flags, "DIR_CONFLICT") {
				afterConflictCount++
			}
			if hasFlag(resultAfter.Flags, "WEAK_SIGNAL_FILTERED") {
				afterWeakSignalCount++
			}
		}

		// 输出差异较大的案例
		if resultBefore.PlanSide != resultAfter.PlanSide {
			t.Logf("\n[案例 %d] %s - 决策变化:", i+1, tc.Symbol)
			t.Logf("  修复前: side=%s, block=%v, struct_dir=%.2f, of_dir=%.2f, delta=%.2f, conf=%.2f",
				resultBefore.PlanSide, resultBefore.BlockEntry,
				resultBefore.StructDir, resultBefore.OFDir, resultBefore.Delta, resultBefore.Confidence)
			t.Logf("  修复后: side=%s, block=%v, struct_dir=%.2f, of_dir=%.2f, delta=%.2f, conf=%.2f",
				resultAfter.PlanSide, resultAfter.BlockEntry,
				resultAfter.StructDir, resultAfter.OFDir, resultAfter.Delta, resultAfter.Confidence)
			t.Logf("  修复前flags: %v", resultBefore.Flags)
			t.Logf("  修复后flags: %v", resultAfter.Flags)
		}
	}

	// 输出统计结果
	t.Logf("\n========== 统计结果对比 ==========")
	t.Logf("\n修复前:")
	t.Logf("  可开仓: %d (%.1f%%)", beforeOpenCount, float64(beforeOpenCount)/float64(len(allSymbols))*100)
	t.Logf("  被阻断: %d (%.1f%%)", beforeBlockedCount, float64(beforeBlockedCount)/float64(len(allSymbols))*100)
	t.Logf("    - 方向冲突: %d", beforeConflictCount)

	t.Logf("\n修复后:")
	t.Logf("  可开仓: %d (%.1f%%)", afterOpenCount, float64(afterOpenCount)/float64(len(allSymbols))*100)
	t.Logf("  被阻断: %d (%.1f%%)", afterBlockedCount, float64(afterBlockedCount)/float64(len(allSymbols))*100)
	t.Logf("    - 方向冲突: %d", afterConflictCount)
	t.Logf("    - 弱信号过滤: %d", afterWeakSignalCount)

	t.Logf("\n改善效果:")
	openRateChange := float64(afterOpenCount-beforeOpenCount) / float64(len(allSymbols)) * 100
	conflictRateChange := float64(beforeConflictCount-afterConflictCount) / float64(len(allSymbols)) * 100
	t.Logf("  开仓率变化: %+.1f%%", openRateChange)
	t.Logf("  方向冲突减少: %.1f%%", conflictRateChange)
}

// computeDirectionArbitrationWithWeakFilter 包含弱信号过滤的方向裁决
func computeDirectionArbitrationWithWeakFilter(in RootSymbolInput, cfg Config) DirectionArbitration {
	result := ComputeDirectionArbitration(in, cfg)

	// 应用弱信号过滤（这是修复后新增的逻辑）
	if abs(result.StructDir) < 0.3 && abs(result.OFDir) < 0.3 {
		if result.PlanSide != SideNeutral && result.PlanSide != SideUnknown {
			result.Flags = append(result.Flags, "WEAK_SIGNAL_FILTERED")
			result.PlanSide = SideNeutral
			result.BlockEntry = true
			result.BlockReason = "WEAK_SIGNAL"
		}
	}

	return result
}

// SymbolTestCase 测试用例
type SymbolTestCase struct {
	Symbol string
	Input  RootSymbolInput
}

// extractSymbolsFromRecord 从record文件中提取symbol数据
func extractSymbolsFromRecord(t *testing.T, filename string) []SymbolTestCase {
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Logf("Warning: 无法读取文件 %s: %v", filename, err)
		return nil
	}

	content := string(data)

	// 查找JSON数据块（在"候选币种"之后）
	lines := strings.Split(content, "\n")
	var jsonStart int
	for i, line := range lines {
		if strings.Contains(line, "## 候选币种") {
			jsonStart = i + 1
			break
		}
	}

	if jsonStart == 0 {
		t.Logf("Warning: 未找到候选币种数据在 %s", filename)
		return nil
	}

	// 提取JSON部分
	var jsonLines []string
	for i := jsonStart; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "###") {
			// 跳过symbol标题行
			continue
		}
		// 找到JSON数据行
		if strings.HasPrefix(line, "{") {
			jsonLines = append(jsonLines, line)
		}
	}

	var symbols []SymbolTestCase
	for _, jsonLine := range jsonLines {
		// 解析JSON
		var rawData map[string]interface{}
		if err := json.Unmarshal([]byte(jsonLine), &rawData); err != nil {
			t.Logf("Warning: JSON解析失败: %v", err)
			continue
		}

		// 提取symbol数据
		for symbol, data := range rawData {
			symbolData, ok := data.(map[string]interface{})
			if !ok {
				continue
			}

			// 构建RootSymbolInput
			input := buildInputFromJSON(t, symbol, symbolData)
			if input != nil {
				symbols = append(symbols, SymbolTestCase{
					Symbol: symbol,
					Input:  *input,
				})
			}
		}
	}

	t.Logf("从 %s 提取了 %d 个样本", filename, len(symbols))
	return symbols
}

// buildInputFromJSON 从JSON数据构建RootSymbolInput
func buildInputFromJSON(t *testing.T, symbol string, data map[string]interface{}) *RootSymbolInput {
	// 提取订单流数据
	ofData, ok := data["订单流分析"].(map[string]interface{})
	if !ok {
		return nil
	}

	// 提取多时间框架数据
	mtfData, ok := data["多时间框架分析"].(map[string]interface{})
	if !ok {
		return nil
	}

	input := &RootSymbolInput{
		Symbol: symbol,
	}

	// 解析订单流数据
	if quality, ok := ofData["数据质量"].(map[string]interface{}); ok {
		input.Orderflow.Quality = Quality{
			Status:               getStringOrDefault(quality, "status", "正常"),
			OverallScore:         getFloatOrDefault(quality, "overall_score", 0),
			CvdReliability:       getFloatOrDefault(quality, "cvd_reliability", 0),
			OIReliability:        getFloatOrDefault(quality, "oi_reliability", 0),
			OrderbookReliability: getFloatOrDefault(quality, "orderbook_reliability", 0),
			DataLagMs:            int64(getFloatOrDefault(quality, "data_lag_ms", 0)),
			UpdateIntervalS:      int64(getFloatOrDefault(quality, "update_interval_s", 0)),
		}
	}

	if macro, ok := ofData["宏观资金趋势"].(map[string]interface{}); ok {
		input.Orderflow.Macro = Macro{
			TrendAlignment:    getStringOrDefault(macro, "trend_alignment", ""),
			SignalStrength:    getFloatOrDefault(macro, "signal_strength", 0),
			ConfidenceLevel:   getFloatOrDefault(macro, "confidence_level", 0),
			MarketRegime:      getStringOrDefault(macro, "market_regime", ""),
			DominantDirection: getStringOrDefault(macro, "dominant_direction", ""),
			SpotCvd1hUSD:      getFloatOrDefault(macro, "spot_cvd_1h_usd", 0),
			FuturesCvd1hUSD:   getFloatOrDefault(macro, "futures_cvd_1h_usd", 0),
			CvdDivergence:     getBoolOrDefault(macro, "cvd_divergence", false),
		}
	}

	if micro, ok := ofData["本周期博弈_5m"].(map[string]interface{}); ok {
		input.Orderflow.Micro5m = Micro5m{
			CandleIntent:      getStringOrDefault(micro, "candle_intent", ""),
			FuturesCvdDelta:   getFloatOrDefault(micro, "futures_cvd_delta_usd", 0),
			SpotCvdDelta:      getFloatOrDefault(micro, "spot_cvd_delta_usd", 0),
			OIDeltaPct:        getFloatOrDefault(micro, "oi_delta_pct", 0),
			PriceDeltaPct:     getFloatOrDefault(micro, "price_delta_pct", 0),
			VolumeDelta:       getFloatOrDefault(micro, "volume_delta", 0),
			DataQuality:       getFloatOrDefault(micro, "data_quality", 0),
		}
	}

	if ob, ok := ofData["盘口结构_v2"].(map[string]interface{}); ok {
		input.Orderflow.OB = Orderbook{
			ImbalanceRatio: getFloatOrDefault(ob, "imbalance_ratio", 0),
			BidPressure:    getFloatOrDefault(ob, "bid_pressure", 0),
			AskPressure:    getFloatOrDefault(ob, "ask_pressure", 0),
			LiquidityScore: getFloatOrDefault(ob, "liquidity_score", 0),
			SpoofingRisk:   getFloatOrDefault(ob, "spoofing_risk", 0),
			WallChange5m:   int64(getFloatOrDefault(ob, "wall_change_count_5m", 0)),
		}
	}

	// 解析多时间框架数据
	input.MTF = make(MTFAnalysis)
	for tf, tfData := range mtfData {
		tfMap, ok := tfData.(map[string]interface{})
		if !ok {
			continue
		}

		var tfd TimeframeData

		if channel, ok := tfMap["通道数据"].(map[string]interface{}); ok {
			tfd.Channel = ChannelInfo{
				Direction:       getStringOrDefault(channel, "channel_direction", ""),
				CurrentPosition: getStringOrDefault(channel, "current_position", ""),
				PriceRatio:      getFloatOrDefault(channel, "price_ratio", 0),
				Quality:         getFloatOrDefault(channel, "quality", 0),
			}
		}

		if vpvr, ok := tfMap["VPVR数据"].(map[string]interface{}); ok {
			tfd.VPVR = VPVR{
				DistToVAHATR: getFloatOrDefault(vpvr, "dist_to_vah_atr", 0),
				DistToVALATR: getFloatOrDefault(vpvr, "dist_to_val_atr", 0),
			}
		}

		if dow, ok := tfMap["道氏理论数据"].(map[string]interface{}); ok {
			if st, ok := dow["supertrend"].(map[string]interface{}); ok {
				tfd.SupertrendDir = getStringOrDefault(st, "direction", "")
			}
		}

		input.MTF[tf] = tfd
	}

	return input
}

func hasFlag(flags []string, target string) bool {
	for _, f := range flags {
		if f == target {
			return true
		}
	}
	return false
}
