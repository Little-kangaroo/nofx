package market

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"nofx/microstructure"
	"strconv"
	"strings"
	"time"
)

// Get 获取指定代币的市场数据
func Get(symbol string) (*Data, error) {
	return GetWithTimeAnchor(symbol, time.Now())
}

// GetWithTimeAnchor 获取指定代币的市场数据（使用时间锚点）
// 🔥 P0-01修复：支持精确的5m close时间锚点，确保全链路时间一致性
func GetWithTimeAnchor(symbol string, anchorTime time.Time) (*Data, error) {
	// 技术指标计算总体耗时统计
	totalStart := time.Now()
	anchorCloseTimeMs := anchorTime.UnixMilli()

	var klines5m, klines15m, klines30m, klines1h, klines4h []Kline
	var err error
	// 标准化symbol
	symbol = Normalize(symbol)

	// K线数据获取阶段耗时统计
	klinesFetchStart := time.Now()
	// 获取5分钟K线数据
	klines5m, err = WSMonitorCli.GetCurrentKlines(symbol, "5m")
	if err != nil {
		return nil, fmt.Errorf("获取5分钟K线失败: %v", err)
	}

	// 获取15分钟K线数据
	klines15m, err = WSMonitorCli.GetCurrentKlines(symbol, "15m")
	if err != nil {
		return nil, fmt.Errorf("获取15分钟K线失败: %v", err)
	}

	// 获取30分钟K线数据
	klines30m, err = WSMonitorCli.GetCurrentKlines(symbol, "30m")
	if err != nil {
		return nil, fmt.Errorf("获取30分钟K线失败: %v", err)
	}

	// 获取1小时K线数据
	klines1h, err = WSMonitorCli.GetCurrentKlines(symbol, "1h")
	if err != nil {
		return nil, fmt.Errorf("获取1小时K线失败: %v", err)
	}

	// 获取4小时K线数据
	klines4h, err = WSMonitorCli.GetCurrentKlines(symbol, "4h")
	if err != nil {
		return nil, fmt.Errorf("获取4小时K线失败: %v", err)
	}

	log.Printf("📊 [%s-时间锚点] 使用锚点: %v (毫秒: %d)", symbol, anchorTime.Format("15:04:05.000"), anchorCloseTimeMs)

	// 🔥 P0-01修复：统一时间锚点裁剪 - 所有时间框架只使用锚点时间之前的已收盘K线
	klines5m = filterKlinesByAnchorTime(klines5m, anchorCloseTimeMs)
	klines15m = filterKlinesByAnchorTime(klines15m, anchorCloseTimeMs)
	klines30m = filterKlinesByAnchorTime(klines30m, anchorCloseTimeMs)
	klines1h = filterKlinesByAnchorTime(klines1h, anchorCloseTimeMs)
	klines4h = filterKlinesByAnchorTime(klines4h, anchorCloseTimeMs)

	log.Printf("📊 [%s-时间锚点] 裁剪后K线数量: 5m=%d, 15m=%d, 30m=%d, 1h=%d, 4h=%d",
		symbol, len(klines5m), len(klines15m), len(klines30m), len(klines1h), len(klines4h))

	// K线数据获取阶段耗时统计
	klinesFetchDuration := time.Since(klinesFetchStart)
	log.Printf("📊 [%s-K线获取] 耗时: %v (5m+15m+30m+1h+4h)", symbol, klinesFetchDuration)

	// 基础技术指标计算阶段耗时统计
	basicIndicatorsStart := time.Now()

	// 🔥 P0-01修复：基于锚点时间裁剪后的K线数据计算指标，确保时间一致性
	if len(klines5m) == 0 {
		return nil, fmt.Errorf("5分钟K线数据经锚点裁剪后为空")
	}

	// 计算当前指标 (基于锚点裁剪后的5分钟数据)
	currentPrice := klines5m[len(klines5m)-1].Close
	currentEMA20 := calculateEMA(klines5m, 20)
	currentMACD := calculateMACD(klines5m)
	currentRSI7 := calculateRSI(klines5m, 7)

	// 基础指标计算耗时统计
	basicIndicatorsDuration := time.Since(basicIndicatorsStart)
	log.Printf("📊 [%s-基础指标] 耗时: %v (Price+EMA20+MACD+RSI7)", symbol, basicIndicatorsDuration)

	// 🔥 P0-01修复：计算价格变化百分比 - 基于裁剪后的数据统一时间锚点
	// 1小时价格变化 = 从当前锚点回看12个5分钟K线
	priceChange1h := 0.0
	lookback1h := len(klines5m) - 12 // 从当前锚点回看12根
	if lookback1h >= 0 {             // 确保索引有效
		price1hAgo := klines5m[lookback1h].Close
		if price1hAgo > 0 {
			priceChange1h = ((currentPrice - price1hAgo) / price1hAgo) * 100
		}
	}

	// 4小时价格变化 = 使用4小时K线的锚点裁剪数据
	priceChange4h := 0.0
	if len(klines4h) >= 2 { // 至少需要2根K线（当前已收盘 + 上一根）
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			priceChange4h = ((currentPrice - price4hAgo) / price4hAgo) * 100
		}
	}

	// 获取OI数据
	oiData, err := getOpenInterestData(symbol)
	if err != nil {
		// OI失败不影响整体,使用默认值
		oiData = &OIData{Latest: 0, Average: 0}
	}

	// 获取Funding Rate
	fundingRate, _ := getFundingRate(symbol)

	// 计算日内系列数据
	intradayData := calculateIntradaySeries(klines5m)

	// 计算长期数据
	longerTermData := calculateLongerTermData(klines4h)

	// 计算其他时间框架的基础指标
	mediumTermData15m := calculateMediumTermData(klines15m, "15m")
	mediumTermData30m := calculateMediumTermData(klines30m, "30m")
	mediumTermData1h := calculateMediumTermData(klines1h, "1h")

	// 高级分析阶段耗时统计
	advancedAnalysisStart := time.Now()
	// 多时间框架综合分析（包括道氏理论、VPVR、供需区、FVG、斐波纳契、通道分析）
	// 🔥 P0-04修复：使用支持ExchangeMeta的综合分析器，实现动态VPVR配置
	// 这里暂时使用nil，后续可以根据symbol获取ExchangeMeta
	var exchangeMeta *ExchangeMeta = nil // 默认使用fallback配置
	comprehensiveAnalyzer := NewComprehensiveAnalyzerWithExchange(exchangeMeta, nil)
	comprehensiveResult := comprehensiveAnalyzer.AnalyzeMultiTimeframe(symbol, klines5m, klines15m, klines30m, klines1h, klines4h)

	// 执行多时间框架分析
	multiTimeframeAnalysis := comprehensiveAnalyzer.AnalyzeAllTimeframes(symbol, currentPrice, map[string][]Kline{
		"5m":  klines5m,
		"15m": klines15m,
		"30m": klines30m,
		"1h":  klines1h,
		"4h":  klines4h,
	})

	// 高级分析耗时统计
	advancedAnalysisDuration := time.Since(advancedAnalysisStart)
	log.Printf("📊 [%s-高级分析] 耗时: %v (道氏理论+VPVR+供需区+FVG+斐波纳契+多时间框架)", symbol, advancedAnalysisDuration)

	// OHLC数据提取阶段
	ohlcExtractionStart := time.Now()

	// 🔥 P0-01修复：提取5m级别OHLC数据 (基于锚点裁剪后的数据)
	// 确保与currentPrice使用相同时间锚点，避免进行中K线导致的数据错配
	ohlc5mLastClosed, ohlc5mPrevClosed := extract5mOHLCDataFromFiltered(klines5m)

	// 提取4h级别OHLC数据 (基于锚点裁剪后的数据)
	// 🔥 P0-02修复：获取最新已收盘4h和上一根已收盘4h，避免HTF判断滞后
	ohlc4hLastClosed, ohlc4hPrevClosed := extract4hOHLCDataFromFiltered(klines4h)

	ohlcExtractionDuration := time.Since(ohlcExtractionStart)
	log.Printf("📊 [%s-OHLC提取] 耗时: %v (5m+4h级别)", symbol, ohlcExtractionDuration)

	// 🔥 P0-04修复：填充ExchangeMeta字段，支持动态VPVR配置
	// 根据symbol推断交易所元数据（tick_size、lot_size等）
	exchangeMeta = &ExchangeMeta{
		Symbol:   symbol,
		TickSize: getSmartTickSizeBySymbol(symbol), // 智能推断tick_size
		LotSize:  1.0,                              // 默认lot_size为1.0，实际可根据交易所规则调整
	}

	data := &Data{
		Symbol:        symbol,
		CurrentPrice:  currentPrice,
		LastPrice:     currentPrice, // 🔥 P0-05修复：新增LastPrice字段，与CurrentPrice严格相等
		PriceChange1h: priceChange1h,
		PriceChange4h: priceChange4h,
		CurrentEMA20:  currentEMA20,
		CurrentMACD:   currentMACD,
		CurrentRSI7:   currentRSI7,
		ExchangeMeta:  exchangeMeta, // 🔥 P0-04修复：填充交易所元数据

		// 🔥 P0-01修复：OHLC数据 - 统一时间锚点确保数据一致性
		// ohlc5mLastClosed现在与currentPrice使用相同时间锚点
		OHLC5mPrevClosed:    ohlc5mLastClosed, // 最后已收盘K线（与currentPrice同锚点）
		OHLC5mEarlierClosed: ohlc5mPrevClosed, // 上一根已收盘K线（用于对比）

		// 🔥 P0-02修复：4h OHLC数据 - 修复off-by-one错误，避免HTF判断滞后
		OHLC4hLastClosed: ohlc4hLastClosed, // 最新已收盘4h（主要HTF数据）
		OHLC4hPrevClosed: ohlc4hPrevClosed, // 上一根已收盘4h（用于对比）

		OpenInterest:           oiData,
		FundingRate:            fundingRate,
		IntradaySeries:         intradayData,
		LongerTermContext:      longerTermData,
		MediumTerm15m:          mediumTermData15m,
		MediumTerm30m:          mediumTermData30m,
		MediumTerm1h:           mediumTermData1h,
		MultiTimeframeAnalysis: multiTimeframeAnalysis,
		// 向前兼容的单一分析结果（基于4小时）
		DowTheory:       comprehensiveResult.DowTheory,
		ChannelAnalysis: comprehensiveResult.ChannelAnalysis,
		VolumeProfile:   comprehensiveResult.VolumeProfile,
		SupplyDemand:    comprehensiveResult.SupplyDemand,
		FairValueGaps:   comprehensiveResult.FairValueGaps,
		Fibonacci:       comprehensiveResult.Fibonacci,

		// 🔥 Gate2 结构聚合输出 - V-13.5规范
		StructureGate2: comprehensiveResult.StructureGate2,

		// 🔥 P0-03修复：缓存K线数据，避免FormatAsCompactData二次获取导致数据漂移
		KlineCache: map[string][]Kline{
			"5m":  klines5m,
			"15m": klines15m,
			"30m": klines30m,
			"1h":  klines1h,
			"4h":  klines4h,
		},
	}

	// 技术指标计算总体耗时统计
	totalDuration := time.Since(totalStart)
	log.Printf("📊 [%s-指标计算总结] 总耗时: %v | K线获取: %v (%.1f%%) | 基础指标: %v (%.1f%%) | 高级分析: %v (%.1f%%)",
		symbol, totalDuration,
		klinesFetchDuration, float64(klinesFetchDuration.Nanoseconds())/float64(totalDuration.Nanoseconds())*100,
		basicIndicatorsDuration, float64(basicIndicatorsDuration.Nanoseconds())/float64(totalDuration.Nanoseconds())*100,
		advancedAnalysisDuration, float64(advancedAnalysisDuration.Nanoseconds())/float64(totalDuration.Nanoseconds())*100)

	return data, nil
}

// extractOHLCData 从K线数据中提取OHLC数据
func extractOHLCData(kline Kline) *OHLCData {
	return &OHLCData{
		Open:      kline.Open,
		High:      kline.High,
		Low:       kline.Low,
		Close:     kline.Close,
		Volume:    kline.Volume,
		OpenTime:  kline.OpenTime,
		CloseTime: kline.CloseTime,
	}
}

// 🔥 P0-01修复：lastClosedIndex 通用helper - 统一时间锚点，避免进行中K线导致的数据不一致
// 功能：判断最后一根K线是否为"未来收盘时间"的进行中K线，返回最后一根已收盘K线的索引
// 解决：CurrentPrice vs OHLCPrevClosed 时间锚点不一致导致的指标错配
func lastClosedIndex(klines []Kline) int {
	if len(klines) == 0 {
		return -1
	}
	nowMs := time.Now().UnixMilli()
	last := klines[len(klines)-1]
	// 若 CloseTime 在未来，说明这根大概率是"进行中K线"
	if last.CloseTime > nowMs && len(klines) >= 2 {
		return len(klines) - 2 // 返回倒数第二根（已收盘）
	}
	return len(klines) - 1 // 最后一根就是已收盘
}

// 🔥 P0-01修复：extract5mOHLCData 提取5m级别OHLC数据 - 统一时间锚点
// 使用lastClosedIndex确保与currentPrice锚点一致，避免进行中K线导致的数据错配
func extract5mOHLCData(klines5m []Kline) (*OHLCData, *OHLCData) {
	idx := lastClosedIndex(klines5m)
	if idx < 0 {
		log.Printf("⚠️ [5m OHLC] K线数据不足，无法提取已收盘数据")
		return nil, nil
	}

	// 最后一根已收盘K线 - 与currentPrice使用相同锚点
	lastClosed := extractOHLCData(klines5m[idx])

	// 上一根已收盘K线 - 用于对比分析
	var prevClosed *OHLCData
	if idx >= 1 {
		prevClosed = extractOHLCData(klines5m[idx-1])
	}

	return lastClosed, prevClosed
}

// 🔥 P0-02修复：extract4hOHLCData 提取4h级别OHLC数据 - 对齐5m逻辑，修复off-by-one错误
// 返回最新已收盘4h和上一根已收盘4h，确保HTF结构判断不滞后
func extract4hOHLCData(klines4h []Kline) (*OHLCData, *OHLCData) {
	idx := lastClosedIndex(klines4h)
	if idx < 0 {
		log.Printf("⚠️ [4h OHLC] K线数据不足，无法提取已收盘数据")
		return nil, nil
	}

	// 最后一根已收盘K线 - 与5m逻辑对齐，避免HTF判断滞后
	lastClosed := extractOHLCData(klines4h[idx])

	// 上一根已收盘K线 - 用于对比分析
	var prevClosed *OHLCData
	if idx >= 1 {
		prevClosed = extractOHLCData(klines4h[idx-1])
	}

	return lastClosed, prevClosed
}

// calculateEMA 计算EMA
func calculateEMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		log.Printf("🚨🔴 [EMA%d计算] ❌ K线数据不足: 需要%d根，实际%d根 ❌", period, period, len(klines))
		return 0
	}
	// EMA收敛建议：为达到99%精度，建议至少使用 period * 3.5 根K线
	recommendedKlines := int(float64(period) * 3.5)
	if len(klines) < recommendedKlines {
		log.Printf("🟡⚠️ [EMA%d计算] 精度警告: 建议%d根，实际%d根 (可能影响精度) ⚠️🟡", period, recommendedKlines, len(klines))
	}

	// 计算SMA作为初始EMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)

	// 计算EMA
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}

	return ema
}

// calculateMACD 计算MACD
func calculateMACD(klines []Kline) float64 {
	minRequired := 26
	recommended := int(float64(26) * 3.5) // 约91根K线用于EMA26收敛
	if len(klines) < minRequired {
		log.Printf("🚨🔴 [MACD计算] ❌ K线数据不足: 需要%d根，实际%d根 ❌", minRequired, len(klines))
		return 0
	}
	if len(klines) < recommended {
		log.Printf("🟡⚠️ [MACD计算] 精度警告: 建议%d根，实际%d根 (可能影响精度) ⚠️🟡", recommended, len(klines))
	}

	// 计算12期和26期EMA
	ema12 := calculateEMA(klines, 12)
	ema26 := calculateEMA(klines, 26)

	// MACD = EMA12 - EMA26
	return ema12 - ema26
}

// calculateRSI 计算RSI
func calculateRSI(klines []Kline, period int) float64 {
	minRequired := period + 1
	recommended := period * 3 // RSI建议使用周期的3倍数据
	if len(klines) <= period {
		log.Printf("🚨🔴 [RSI%d计算] ❌ K线数据不足: 需要%d根，实际%d根 ❌", period, minRequired, len(klines))
		return 0
	}
	if len(klines) < recommended {
		log.Printf("🟡⚠️ [RSI%d计算] 稳定性警告: 建议%d根，实际%d根 (可能影响稳定性) ⚠️🟡", period, recommended, len(klines))
	}

	gains := 0.0
	losses := 0.0

	// 计算初始平均涨跌幅
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// 使用Wilder平滑方法计算后续RSI
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + (-change)) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateATR 计算ATR
func calculateATR(klines []Kline, period int) float64 {
	minRequired := period + 1
	recommended := period * 2 // ATR建议使用周期的2倍数据
	if len(klines) <= period {
		log.Printf("🚨🔴 [ATR%d计算] ❌ K线数据不足: 需要%d根，实际%d根 ❌", period, minRequired, len(klines))
		return 0
	}
	if len(klines) < recommended {
		log.Printf("🟡⚠️ [ATR%d计算] 平滑性警告: 建议%d根，实际%d根 (可能影响平滑性) ⚠️🟡", period, recommended, len(klines))
	}

	trs := make([]float64, len(klines))
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// 计算初始ATR
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += trs[i]
	}
	atr := sum / float64(period)

	// Wilder平滑
	for i := period + 1; i < len(klines); i++ {
		atr = (atr*float64(period-1) + trs[i]) / float64(period)
	}

	return atr
}

// calculateSMA 计算简单移动平均线
func calculateSMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}

	sum := 0.0
	for i := len(klines) - period; i < len(klines); i++ {
		sum += klines[i].Close
	}

	return sum / float64(period)
}

// calculateVWAP 计算成交量加权平均价
func calculateVWAP(klines []Kline) float64 {
	if len(klines) == 0 {
		return 0
	}

	var volumeWeightedSum float64
	var totalVolume float64

	for _, kline := range klines {
		typicalPrice := (kline.High + kline.Low + kline.Close) / 3
		volumeWeightedSum += typicalPrice * kline.Volume
		totalVolume += kline.Volume
	}

	if totalVolume == 0 {
		return 0
	}

	return volumeWeightedSum / totalVolume
}

// calculateEMASlope 计算EMA斜率（连续N根K线的变化率）
func calculateEMASlope(klines []Kline, period int, lookback int) float64 {
	if len(klines) < period+lookback {
		return 0
	}

	// 计算当前EMA值
	currentEMA := calculateEMA(klines, period)

	// 计算lookback根K线前的EMA值
	prevKlines := klines[:len(klines)-lookback]
	if len(prevKlines) < period {
		return 0
	}
	prevEMA := calculateEMA(prevKlines, period)

	// 计算斜率（变化率）
	if prevEMA == 0 {
		return 0
	}

	return ((currentEMA - prevEMA) / prevEMA) * 100
}

// calculateIntradaySeries 计算日内系列数据
func calculateIntradaySeries(klines []Kline) *IntradayData {
	data := &IntradayData{
		MidPrices:   make([]float64, 0, 10),
		EMA20Values: make([]float64, 0, 10),
		MACDValues:  make([]float64, 0, 10),
		RSI7Values:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 获取最近10个数据点
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		data.MidPrices = append(data.MidPrices, klines[i].Close)

		// 计算每个点的EMA20
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// 计算每个点的MACD
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// 计算每个点的RSI
		if i >= 7 {
			rsi7 := calculateRSI(klines[:i+1], 7)
			data.RSI7Values = append(data.RSI7Values, rsi7)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// calculateLongerTermData 计算长期数据
func calculateLongerTermData(klines []Kline) *LongerTermData {
	data := &LongerTermData{
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 计算EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)

	// 计算ATR
	data.ATR3 = calculateATR(klines, 3)
	data.ATR14 = calculateATR(klines, 14)

	// 计算成交量
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume
		// 计算平均成交量
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// 计算MACD和RSI序列
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// getOpenInterestData 获取OI数据
func getOpenInterestData(symbol string) (*OIData, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/openInterest?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		OpenInterest string `json:"openInterest"`
		Symbol       string `json:"symbol"`
		Time         int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	oi, _ := strconv.ParseFloat(result.OpenInterest, 64)

	return &OIData{
		Latest:  oi,
		Average: oi * 0.999, // 近似平均值
	}, nil
}

// getFundingRate 获取资金费率
func getFundingRate(symbol string) (float64, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/premiumIndex?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Symbol          string `json:"symbol"`
		MarkPrice       string `json:"markPrice"`
		IndexPrice      string `json:"indexPrice"`
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
		InterestRate    string `json:"interestRate"`
		Time            int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	rate, _ := strconv.ParseFloat(result.LastFundingRate, 64)
	return rate, nil
}

// Format 格式化输出市场数据
func Format(data *Data) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("current_price = %.2f, current_ema20 = %.3f, current_macd = %.3f, current_rsi (7 period) = %.3f\n\n",
		data.CurrentPrice, data.CurrentEMA20, data.CurrentMACD, data.CurrentRSI7))

	sb.WriteString(fmt.Sprintf("In addition, here is the latest %s open interest and funding rate for perps:\n\n",
		data.Symbol))

	if data.OpenInterest != nil {
		sb.WriteString(fmt.Sprintf("Open Interest: Latest: %.2f Average: %.2f\n\n",
			data.OpenInterest.Latest, data.OpenInterest.Average))
	}

	sb.WriteString(fmt.Sprintf("Funding Rate: %.2e\n\n", data.FundingRate))

	if data.IntradaySeries != nil {
		sb.WriteString("Intraday series (3‑minute intervals, oldest → latest):\n\n")

		if len(data.IntradaySeries.MidPrices) > 0 {
			sb.WriteString(fmt.Sprintf("Mid prices: %s\n\n", formatFloatSlice(data.IntradaySeries.MidPrices)))
		}

		if len(data.IntradaySeries.EMA20Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA indicators (20‑period): %s\n\n", formatFloatSlice(data.IntradaySeries.EMA20Values)))
		}

		if len(data.IntradaySeries.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.IntradaySeries.MACDValues)))
		}

		if len(data.IntradaySeries.RSI7Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (7‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI7Values)))
		}

		if len(data.IntradaySeries.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI14Values)))
		}
	}

	if data.LongerTermContext != nil {
		sb.WriteString("Longer‑term context (4‑hour timeframe):\n\n")

		sb.WriteString(fmt.Sprintf("20‑Period EMA: %.3f vs. 50‑Period EMA: %.3f\n\n",
			data.LongerTermContext.EMA20, data.LongerTermContext.EMA50))

		sb.WriteString(fmt.Sprintf("3‑Period ATR: %.3f vs. 14‑Period ATR: %.3f\n\n",
			data.LongerTermContext.ATR3, data.LongerTermContext.ATR14))

		sb.WriteString(fmt.Sprintf("Current Volume: %.3f vs. Average Volume: %.3f\n\n",
			data.LongerTermContext.CurrentVolume, data.LongerTermContext.AverageVolume))

		if len(data.LongerTermContext.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.LongerTermContext.MACDValues)))
		}

		if len(data.LongerTermContext.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.LongerTermContext.RSI14Values)))
		}
	}

	// 道氏理论分析
	if data.DowTheory != nil {
		sb.WriteString("Dow Theory Analysis:\n\n")
		sb.WriteString(formatDowTheoryData(data.DowTheory))
	}

	// VPVR分析
	if data.VolumeProfile != nil {
		sb.WriteString(formatVPVRData(data.VolumeProfile))
	}

	// 供需区分析
	if data.SupplyDemand != nil {
		sb.WriteString(formatSupplyDemandData(data.SupplyDemand))
	}

	// FVG分析
	if data.FairValueGaps != nil {
		sb.WriteString(formatFVGData(data.FairValueGaps))
	}

	// 斐波纳契分析
	if data.Fibonacci != nil {
		sb.WriteString("Fibonacci Analysis:\n\n")
		sb.WriteString(formatFibonacciData(data.Fibonacci))
	}

	// 多时间框架分析总结
	if data.MultiTimeframeAnalysis != nil {
		sb.WriteString(formatMultiTimeframeAnalysis(data.MultiTimeframeAnalysis))
	}

	return sb.String()
}

// FormatAsStructuredData 将市场数据格式化为结构化格式（你想要的格式）
func FormatAsStructuredData(data *Data) string {
	// ��取单币种的多时间框架分析数据
	symbolData, err := GetSingleSymbolAnalysis(data.Symbol)
	if err != nil {
		return fmt.Sprintf("获取%s结构化数据失败: %v", data.Symbol, err)
	}

	// 创建完整的数据结构，包含基础指标和多时间框架分析
	result := map[string]interface{}{
		data.Symbol: map[string]interface{}{
			// 基础市场指标 (保持原有格式)
			"基础指标": map[string]interface{}{
				"current_price":       data.CurrentPrice,
				"current_ema20":       data.CurrentEMA20,
				"current_macd":        data.CurrentMACD,
				"current_rsi7":        data.CurrentRSI7,
				"price_change_1h":     data.PriceChange1h,
				"price_change_4h":     data.PriceChange4h,
				"open_interest":       data.OpenInterest,
				"funding_rate":        data.FundingRate,
				"intraday_series":     data.IntradaySeries,
				"longer_term_context": data.LongerTermContext,
				"medium_term_15m":     data.MediumTerm15m,
				"medium_term_30m":     data.MediumTerm30m,
				"medium_term_1h":      data.MediumTerm1h,
			},
			// 多时间框架技术分析
			"多时间框架分析": symbolData,
		},
	}

	// 序列化为JSON
	jsonData, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("JSON序列化失败: %v", err)
	}

	return string(jsonData)
}

// FormatAsCompactData 精简版市场数据格式化（供AI交易员使用）
// 只包含计算出的关键指标结果，不包含原始K线数据和详细序列
// 🔥 P0-03修复：FormatAsCompactData 消除同请求内K线数据漂移
// 使用缓存的K线数据，避免二次获取导致的时间跨越新K线开始时的数据不一致
func FormatAsCompactData(data *Data) string {
	// 🔥 P0-03修复：使用缓存的K线数据，确保与基础指标计算使用相同时间快照
	if data.KlineCache == nil {
		log.Printf("⚠️ [CompactData] K线缓存为空，可能存在数据一致性风险")
		return fmt.Sprintf("K线缓存数据不可用")
	}

	// 直接使用缓存的K线数据，避免二次网络请求导致的数据漂移
	timeframeKlines := data.KlineCache

	result := map[string]interface{}{
		data.Symbol: map[string]interface{}{
			"基础指标":       calculateMultiTimeframeBasicIndicators(data, timeframeKlines),
			"多时间框架分析": extractCompactMultiTimeframeAnalysisWithSupertrend(data, timeframeKlines),
			"订单流分析":     GetOrderFlowDataForAIV2(data.Symbol),
			//"Gate2结构聚合": buildGate2CompactOutput(data),
		},
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("精简JSON序列化失败: %v", err)
	}

	// 🔥 P1-2修复：JSON schema验证 - 防止AI输出拼写错误
	if len(jsonData) > 0 {
		// 解析JSON数据进行验证
		var parsedData interface{}
		if err := json.Unmarshal(jsonData, &parsedData); err == nil {
			validationResult := ValidateAIOutput(parsedData, "compact")

			// 如果启用自动纠错且有纠错内容，使用纠错后的数据
			if validationResult.ProcessedData != nil && len(validationResult.CorrectedFields) > 0 {
				if correctedJSON, err := json.Marshal(validationResult.ProcessedData); err == nil {
					log.Printf("🔧 [P1-2] 自动纠正了 %d 个字段拼写错误", len(validationResult.CorrectedFields))
					return string(correctedJSON)
				}
			}

			// 记录验证警告（不阻断输出）
			if len(validationResult.Warnings) > 0 {
				log.Printf("⚠️ [P1-2] AI输出验证发现 %d 个警告", len(validationResult.Warnings))
			}
		}
	}

	return string(jsonData)
}

// calculateMultiTimeframeBasicIndicators 计算多时间框架基础指标
func calculateMultiTimeframeBasicIndicators(data *Data, timeframeKlines map[string][]Kline) map[string]interface{} {
	result := make(map[string]interface{})

	// 全局指标（不依赖时间框架）
	// 🔥 P0-05修复：价格字段契约统一 - V-13.5要求只认last_price
	result["last_price"] = FormatByDataTypeAndSymbol(data.LastPrice, "price", data.Symbol) // 主要字段，V-13.5标准
	result["price"] = FormatByDataTypeAndSymbol(data.LastPrice, "price", data.Symbol)      // 向后兼容字段，已废弃，与last_price严格相等
	result["funding_rate"] = FormatByDataTypeAndSymbol(data.FundingRate, "ratio", data.Symbol)
	result["oi_latest"] = func() float64 {
		if data.OpenInterest != nil {
			return FormatByDataTypeAndSymbol(data.OpenInterest.Latest, "volume", data.Symbol)
		}
		return 0
	}()

	// 价格变化（基于5分钟K线计算）
	if klines5m, exists := timeframeKlines["5m"]; exists && len(klines5m) > 0 {
		// 1小时价格变化 = 12个5分钟K线前的价格
		if len(klines5m) >= 13 {
			price1hAgo := klines5m[len(klines5m)-13].Close
			if price1hAgo > 0 {
				// 🔥 P0-05修复：使用统一的LastPrice计算价格变化
				result["change_1h"] = FormatByDataTypeAndSymbol(((data.LastPrice-price1hAgo)/price1hAgo)*100, "percentage", data.Symbol)
			}
		}
	}

	// 4小时价格变化（基于4小时K线计算）
	if klines4h, exists := timeframeKlines["4h"]; exists && len(klines4h) >= 2 {
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			// 🔥 P0-05修复：使用统一的LastPrice计算价格变化
			result["change_4h"] = FormatByDataTypeAndSymbol(((data.LastPrice-price4hAgo)/price4hAgo)*100, "percentage", data.Symbol)
		}
	}

	// 各时间框架的基础指标
	timeframes := []string{"5m", "15m", "30m", "1h", "4h"}
	for _, tf := range timeframes {
		klines, exists := timeframeKlines[tf]
		if !exists {
			log.Printf("⚠️ [基础指标] %s时间框架数据不存在", tf)
			continue
		}
		if len(klines) == 0 {
			log.Printf("⚠️ [基础指标] %s时间框架K线数据为空", tf)
			continue
		}
		log.Printf("✓ [基础指标] %s时间框架: %d条K线数据", tf, len(klines))

		// 记录哪些指标可以计算
		availableIndicators := []string{}
		if len(klines) >= 20 {
			availableIndicators = append(availableIndicators, "EMA20/SMA20")
		}
		if len(klines) >= 50 {
			availableIndicators = append(availableIndicators, "EMA50/SMA50")
		}
		if len(klines) >= 100 {
			availableIndicators = append(availableIndicators, "EMA100")
		}
		if len(klines) >= 200 {
			availableIndicators = append(availableIndicators, "EMA200")
		}
		if len(klines) >= 53 {
			availableIndicators = append(availableIndicators, "EMA50斜率")
		}
		if len(klines) >= 203 {
			availableIndicators = append(availableIndicators, "EMA200斜率")
		}
		log.Printf("✓ [基础指标] %s可计算指标: %v", tf, availableIndicators)

		tfData := map[string]interface{}{}

		// === 移动平均线指标 ===
		// EMA系列
		if len(klines) >= 20 {
			tfData["ema20"] = calculateEMA(klines, 20)
		}
		if len(klines) >= 50 {
			tfData["ema50"] = calculateEMA(klines, 50)
		}
		if len(klines) >= 100 {
			tfData["ema100"] = calculateEMA(klines, 100)
		}
		if len(klines) >= 200 {
			tfData["ema200"] = calculateEMA(klines, 200)
		}

		// SMA系列
		if len(klines) >= 20 {
			tfData["sma20"] = calculateSMA(klines, 20)
		}
		if len(klines) >= 50 {
			tfData["sma50"] = calculateSMA(klines, 50)
		}

		// VWAP (成交量加权平均价)
		if len(klines) > 0 {
			tfData["vwap"] = calculateVWAP(klines)
		}

		// === EMA斜率指标 (3根K线回看期) ===
		if len(klines) >= 53 { // 50 + 3
			tfData["ema50_slope_3"] = calculateEMASlope(klines, 50, 3)
		}
		if len(klines) >= 203 { // 200 + 3
			tfData["ema200_slope_3"] = calculateEMASlope(klines, 200, 3)
		}

		// === 原有指标保持不变 ===
		// MACD
		if len(klines) >= 26 {
			tfData["macd"] = calculateMACD(klines)
		}

		// RSI7 和 RSI14
		if len(klines) >= 8 {
			tfData["rsi7"] = calculateRSI(klines, 7)
		}
		if len(klines) >= 15 {
			tfData["rsi14"] = calculateRSI(klines, 14)
		}

		// ATR14
		if len(klines) >= 15 {
			tfData["atr14"] = calculateATR(klines, 14)
		}

		// === 成交量指标 ===
		if len(klines) > 0 {
			tfData["volume"] = klines[len(klines)-1].Volume
			// 平均成交量
			sum := 0.0
			for _, k := range klines {
				sum += k.Volume
			}
			tfData["avg_volume"] = sum / float64(len(klines))
		}

		// === OHLC数据 ===
		switch tf {
		case "5m":
			// 5m级别: ohlc_last_closed + ohlc_prev_closed
			if data.OHLC5mPrevClosed != nil {
				tfData["ohlc_last_closed"] = formatOHLCData(data.OHLC5mPrevClosed, data.Symbol)
			}
			if data.OHLC5mEarlierClosed != nil {
				tfData["ohlc_prev_closed"] = formatOHLCData(data.OHLC5mEarlierClosed, data.Symbol)
			}
		case "15m":
			// 15m级别: ohlc_last_closed + ohlc_prev_closed
			if data.MediumTerm15m != nil {
				if data.MediumTerm15m.OHLCLastClosed != nil {
					tfData["ohlc_last_closed"] = formatOHLCData(data.MediumTerm15m.OHLCLastClosed, data.Symbol)
				}
				if data.MediumTerm15m.OHLCPrevClosed != nil {
					tfData["ohlc_prev_closed"] = formatOHLCData(data.MediumTerm15m.OHLCPrevClosed, data.Symbol)
				}
			}
		case "30m":
			// 30m级别: ohlc_last_closed + ohlc_prev_closed
			if data.MediumTerm30m != nil {
				if data.MediumTerm30m.OHLCLastClosed != nil {
					tfData["ohlc_last_closed"] = formatOHLCData(data.MediumTerm30m.OHLCLastClosed, data.Symbol)
				}
				if data.MediumTerm30m.OHLCPrevClosed != nil {
					tfData["ohlc_prev_closed"] = formatOHLCData(data.MediumTerm30m.OHLCPrevClosed, data.Symbol)
				}
			}
		case "1h":
			// 1h级别: ohlc_last_closed only
			if data.MediumTerm1h != nil && data.MediumTerm1h.OHLCLastClosed != nil {
				tfData["ohlc_last_closed"] = formatOHLCData(data.MediumTerm1h.OHLCLastClosed, data.Symbol)
			}
		case "4h":
			// 4h级别: ohlc_last_closed only
			// 🔥 P0-02修复：使用OHLC4hLastClosed而不是PrevClosed，修复HTF滞后问题
			if data.OHLC4hLastClosed != nil {
				tfData["ohlc_last_closed"] = formatOHLCData(data.OHLC4hLastClosed, data.Symbol)
			}
		}

		// 只有当有数据时才添加到结果中
		if len(tfData) > 0 {
			result[tf] = tfData
		}
	}

	// 应用精度格式化到所有基础指标数据
	formatBasicIndicatorsData(result, data.Symbol)

	return result
}

// formatOHLCData 格式化OHLC数据，应用适当的精度
func formatOHLCData(ohlcData *OHLCData, symbol string) map[string]interface{} {
	if ohlcData == nil {
		return nil
	}

	return map[string]interface{}{
		"open":       FormatByDataTypeAndSymbol(ohlcData.Open, "price", symbol),
		"high":       FormatByDataTypeAndSymbol(ohlcData.High, "price", symbol),
		"low":        FormatByDataTypeAndSymbol(ohlcData.Low, "price", symbol),
		"close":      FormatByDataTypeAndSymbol(ohlcData.Close, "price", symbol),
		"volume":     FormatByDataTypeAndSymbol(ohlcData.Volume, "volume", symbol),
		"open_time":  ohlcData.OpenTime,
		"close_time": ohlcData.CloseTime,
		"x":          true, // 表示已收盘的K线bar，满足taro模板要求
	}
}

// extractCompactMultiTimeframeAnalysis 提取精简的多时间框架分析数据
func extractCompactMultiTimeframeAnalysis(data *Data) map[string]interface{} {
	result := make(map[string]interface{})

	if data.MultiTimeframeAnalysis == nil || data.MultiTimeframeAnalysis.Timeframes == nil {
		return result
	}

	timeframes := []string{"5m", "15m", "30m", "1h", "4h"}

	for _, tf := range timeframes {
		tfData, exists := data.MultiTimeframeAnalysis.Timeframes[tf]
		if !exists || tfData == nil {
			continue
		}

		result[tf] = map[string]interface{}{
			"道氏理论数据":   extractCompactDowTheory(tfData.DowTheory, data.Symbol),
			"通道数据":       extractCompactChannelAnalysis(tfData.ChannelAnalysis, data.Symbol),
			"VPVR数据":       extractCompactVPVR(tfData.VolumeProfile, data.Symbol),
			"供需区数据":     extractCompactSupplyDemand(tfData.SupplyDemand, data.Symbol),
			"FVG数据":        extractCompactFVG(tfData.FairValueGaps, data.Symbol),
			"斐波纳契数据":   extractCompactFibonacci(tfData.Fibonacci, data.Symbol),
			"支撑阻力转换线": extractCompactSupportResistance(tfData.SupportResistance, data.Symbol),
		}
	}

	return result
}

// extractCompactDowTheory 提取道氏理论的关键结果
func extractCompactDowTheory(data *DowTheoryData, symbol string) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}

	result := map[string]interface{}{
		"trend_direction":   "unknown",
		"trend_strength":    0.0,
		"signal_confidence": 0.0,
		"supertrend": map[string]interface{}{
			"direction":    "unknown",
			"current_line": 0.0,
			"upper_line":   0.0,
			"lower_line":   0.0,
		},
	}

	if data.TrendStrength != nil {
		result["trend_direction"] = data.TrendStrength.Direction
		result["trend_strength"] = FormatByDataTypeAndSymbol(data.TrendStrength.Overall, "strength", symbol)
		result["signal_confidence"] = FormatByDataTypeAndSymbol(data.TrendStrength.Consistency, "confidence", symbol)
	}

	return result
}

// extractCompactChannelAnalysis 提取通道分析的关键结果
func extractCompactChannelAnalysis(data *ChannelData, symbol string) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}

	result := map[string]interface{}{
		"channel_direction": data.Direction,
		"channel_width_pct": FormatByDataTypeAndSymbol(data.Quality*100, "percentage", symbol), // 默认用Quality评分作为百分比
		"current_position":  data.CurrentPosition,
	}

	if data.ActiveChannel != nil {
		// 通道宽度是相对于价格的百分比，已经是0.02形式，乘以100转为百分比显示
		result["channel_width_pct"] = FormatByDataTypeAndSymbol(data.ActiveChannel.Width*100, "percentage", symbol)
	}

	return result
}

// extractCompactVPVR 提取VPVR的关键结果
func extractCompactVPVR(data *VolumeProfile, symbol string) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}

	result := map[string]interface{}{
		"poc_price":       0.0,
		"value_area_high": FormatByDataTypeAndSymbol(data.VAH, "price", symbol),
		"value_area_low":  FormatByDataTypeAndSymbol(data.VAL, "price", symbol),
	}

	if data.POC != nil {
		result["poc_price"] = FormatByDataTypeAndSymbol(data.POC.Price, "price", symbol)
	}

	// 添加上下文评分信息
	if data.Context != nil {
		result["ctx"] = map[string]interface{}{
			"strength_z": FormatByDataTypeAndSymbol(data.Context.StrengthZ, "ratio", symbol),
			"width_atr":  FormatByDataTypeAndSymbol(data.Context.WidthATR, "ratio", symbol),
			"vol_ratio":  FormatByDataTypeAndSymbol(data.Context.VolRatio, "ratio", symbol),
			"is_fresh":   data.Context.IsFresh,
			"time_score": FormatByDataTypeAndSymbol(data.Context.TimeScore, "ratio", symbol),
			"rank_pct":   FormatByDataTypeAndSymbol(data.Context.RankPct, "ratio", symbol),
		}
	}

	return result
}

// extractCompactSupplyDemand 提取供需区的关键结果
func extractCompactSupplyDemand(data *SupplyDemandData, symbol string) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}

	result := map[string]interface{}{
		"total_zones":  len(data.ActiveZones),
		"supply_zones": []map[string]interface{}{},
		"demand_zones": []map[string]interface{}{},
		"zone_stats": map[string]interface{}{
			"avg_strength": 0.0,
			"supply_count": 0,
			"demand_count": 0,
		},
	}

	if len(data.ActiveZones) == 0 {
		return result
	}

	var supplyZones, demandZones []map[string]interface{}
	var strengthSum float64
	var supplyCount, demandCount int

	// 按强度排序并提取重要的供需区
	for _, zone := range data.ActiveZones {
		strengthSum += zone.Strength

		zoneInfo := map[string]interface{}{
			"price_range": map[string]float64{
				"low":  FormatByDataTypeAndSymbol(zone.LowerBound, "price", symbol),
				"high": FormatByDataTypeAndSymbol(zone.UpperBound, "price", symbol),
			},
			"strength": FormatByDataTypeAndSymbol(zone.Strength, "strength", symbol),
			"touches":  zone.TouchCount,
			"status":   zone.Status,
		}

		// 添加上下文评分
		if zone.Context != nil {
			zoneInfo["ctx"] = map[string]interface{}{
				"strength_z": FormatByDataTypeAndSymbol(zone.Context.StrengthZ, "ratio", symbol),
				"width_atr":  FormatByDataTypeAndSymbol(zone.Context.WidthATR, "ratio", symbol),
				"vol_ratio":  FormatByDataTypeAndSymbol(zone.Context.VolRatio, "ratio", symbol),
				"is_fresh":   zone.Context.IsFresh,
				"time_score": FormatByDataTypeAndSymbol(zone.Context.TimeScore, "ratio", symbol),
				"rank_pct":   FormatByDataTypeAndSymbol(zone.Context.RankPct, "ratio", symbol),
			}
		}

		if zone.Type == SupplyZone {
			supplyZones = append(supplyZones, zoneInfo)
			supplyCount++
		} else if zone.Type == DemandZone {
			demandZones = append(demandZones, zoneInfo)
			demandCount++
		}
	}

	// 只保留强度最高的前3个供给区和需求区
	if len(supplyZones) > 3 {
		// 按强度排序，保留前3个最强的
		sortZonesByStrength(supplyZones)
		supplyZones = supplyZones[:3]
	}

	if len(demandZones) > 3 {
		// 按强度排序，保留前3个最强的
		sortZonesByStrength(demandZones)
		demandZones = demandZones[:3]
	}

	result["supply_zones"] = supplyZones
	result["demand_zones"] = demandZones
	result["zone_stats"] = map[string]interface{}{
		"avg_strength": FormatByDataTypeAndSymbol(strengthSum/float64(len(data.ActiveZones)), "strength", symbol),
		"supply_count": supplyCount,
		"demand_count": demandCount,
	}

	return result
}

// sortZonesByStrength 按强度对供需区进行降序排序
func sortZonesByStrength(zones []map[string]interface{}) {
	for i := 0; i < len(zones)-1; i++ {
		for j := i + 1; j < len(zones); j++ {
			strength1 := zones[i]["strength"].(float64)
			strength2 := zones[j]["strength"].(float64)
			if strength1 < strength2 {
				zones[i], zones[j] = zones[j], zones[i]
			}
		}
	}
}

// extractCompactFVG 提取FVG的关键结果
func extractCompactFVG(data *FVGData, symbol string) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}

	result := map[string]interface{}{
		"active_gaps": len(data.ActiveFVGs),
		"nearest_gap": 0.0,
		"gap_type":    "none", // 默认为none，表示无Gap
		"gaps":        []map[string]interface{}{},
	}

	if len(data.ActiveFVGs) > 0 {
		// 取第一个活跃的FVG作为最近的
		fvg := data.ActiveFVGs[0]
		result["nearest_gap"] = FormatByDataTypeAndSymbol((fvg.LowerBound+fvg.UpperBound)/2, "price", symbol)
		if fvg.Type == BullishFVG {
			result["gap_type"] = "bullish"
		} else if fvg.Type == BearishFVG {
			result["gap_type"] = "bearish"
		} else {
			result["gap_type"] = "neutral"
		}

		// 添加详细的FVG信息
		var gaps []map[string]interface{}
		for _, gap := range data.ActiveFVGs {
			gapInfo := map[string]interface{}{
				"id":            gap.ID,
				"type":          gap.Type,
				"upper_bound":   FormatByDataTypeAndSymbol(gap.UpperBound, "price", symbol),
				"lower_bound":   FormatByDataTypeAndSymbol(gap.LowerBound, "price", symbol),
				"center_price":  FormatByDataTypeAndSymbol(gap.CenterPrice, "price", symbol),
				"width":         FormatByDataTypeAndSymbol(gap.Width, "price", symbol),
				"width_percent": FormatByDataTypeAndSymbol(gap.WidthPercent, "percentage", symbol),
				"strength":      FormatByDataTypeAndSymbol(gap.Strength, "strength", symbol),
				"quality":       gap.Quality,
				"status":        gap.Status,
				"touch_count":   gap.TouchCount,
			}

			// 添加上下文评分
			if gap.Context != nil {
				gapInfo["ctx"] = map[string]interface{}{
					"strength_z": FormatByDataTypeAndSymbol(gap.Context.StrengthZ, "ratio", symbol),
					"width_atr":  FormatByDataTypeAndSymbol(gap.Context.WidthATR, "ratio", symbol),
					"vol_ratio":  FormatByDataTypeAndSymbol(gap.Context.VolRatio, "ratio", symbol),
					"is_fresh":   gap.Context.IsFresh,
					"time_score": FormatByDataTypeAndSymbol(gap.Context.TimeScore, "ratio", symbol),
					"rank_pct":   FormatByDataTypeAndSymbol(gap.Context.RankPct, "ratio", symbol),
				}
			}

			gaps = append(gaps, gapInfo)
		}
		result["gaps"] = gaps
	}

	return result
}

// extractCompactFibonacci 提取斐波纳契的关键结果
func extractCompactFibonacci(data *FibonacciData, symbol string) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}

	result := map[string]interface{}{
		"active_retracements": 0,
		"levels":              map[string]float64{},
		"trend_direction":     "unknown",
	}

	// 找到最活跃的回调级别并输出所有重要级别
	if len(data.Retracements) > 0 {
		activeCount := 0
		for _, ret := range data.Retracements {
			if ret.IsActive {
				activeCount++
				// 设置趋势方向
				if ret.TrendType == TrendUpward {
					result["trend_direction"] = "upward"
				} else if ret.TrendType == TrendDownward {
					result["trend_direction"] = "downward"
				}

				// 提取所有重要的斐波纳契级别
				levels := make(map[string]float64)
				for _, level := range ret.Levels {
					if level.Importance >= 0.5 { // 只包含重要性>=50%的级别
						ratioKey := fmt.Sprintf("fib_%.3f", level.Ratio)
						levels[ratioKey] = FormatByDataTypeAndSymbol(level.Price, "price", symbol)
					}
				}

				// 如果找到级别，使用第一个活跃回调的级别
				if len(levels) > 0 && len(result["levels"].(map[string]float64)) == 0 {
					result["levels"] = levels
				}

				break // 只使用第一个活跃的回调
			}
		}
		result["active_retracements"] = activeCount
	}

	// 如果没有活跃的回调，尝试从扩展级别获取
	if len(result["levels"].(map[string]float64)) == 0 && len(data.Extensions) > 0 {
		for _, ext := range data.Extensions {
			if ext.Quality == FibQualityHigh {
				levels := make(map[string]float64)
				for _, level := range ext.Levels {
					ratioKey := fmt.Sprintf("ext_%.3f", level.Ratio)
					levels[ratioKey] = FormatByDataTypeAndSymbol(level.Price, "price", symbol)
				}
				if len(levels) > 0 {
					result["levels"] = levels
					break
				}
			}
		}
	}

	return result
}

// formatFloatSlice 格式化float64切片为字符串
func formatFloatSlice(values []float64) string {
	strValues := make([]string, len(values))
	for i, v := range values {
		strValues[i] = fmt.Sprintf("%.3f", v)
	}
	return "[" + strings.Join(strValues, ", ") + "]"
}

// formatDowTheoryData 格式化道氏理论数据
func formatDowTheoryData(data *DowTheoryData) string {
	var sb strings.Builder

	// 趋势强度分析
	if data.TrendStrength != nil {
		sb.WriteString("Trend Strength Analysis:\n")
		sb.WriteString(fmt.Sprintf("  Overall Strength: %.1f%% (%s trend, %s quality)\n",
			data.TrendStrength.Overall, data.TrendStrength.Direction, data.TrendStrength.Quality))
		sb.WriteString(fmt.Sprintf("  Short-term: %.1f%%, Long-term: %.1f%%\n",
			data.TrendStrength.ShortTerm, data.TrendStrength.LongTerm))
		sb.WriteString(fmt.Sprintf("  Momentum: %.1f%%, Consistency: %.1f%%, Volume Support: %.1f%%\n\n",
			data.TrendStrength.Momentum, data.TrendStrength.Consistency, data.TrendStrength.VolumeSupport))
	}

	// 摆动点分析
	if len(data.SwingPoints) > 0 {
		confirmedHighs := 0
		confirmedLows := 0
		for _, point := range data.SwingPoints {
			if point.Confirmed {
				if point.Type == SwingHigh {
					confirmedHighs++
				} else {
					confirmedLows++
				}
			}
		}
		sb.WriteString(fmt.Sprintf("Swing Points: %d total (%d confirmed highs, %d confirmed lows)\n",
			len(data.SwingPoints), confirmedHighs, confirmedLows))

		// 显示最近的几个确认摆动点
		recentPoints := getRecentSwingPoints(data.SwingPoints, 4)
		if len(recentPoints) > 0 {
			sb.WriteString("  Recent confirmed points: ")
			for i, point := range recentPoints {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf("%s@%.2f", point.Type, point.Price))
			}
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("\n")
		}
	}

	// 趋势线分析
	if len(data.TrendLines) > 0 {
		supportLines := 0
		resistanceLines := 0
		for _, line := range data.TrendLines {
			if line.Type == SupportLine {
				supportLines++
			} else {
				resistanceLines++
			}
		}
		sb.WriteString(fmt.Sprintf("Trend Lines: %d total (%d support, %d resistance)\n",
			len(data.TrendLines), supportLines, resistanceLines))

		// 显示最强的几条趋势线
		strongestLines := getStrongestTrendLines(data.TrendLines, 3)
		for i, line := range strongestLines {
			sb.WriteString(fmt.Sprintf("  %d. %s line: strength %.1f, touches %d\n",
				i+1, line.Type, line.Strength, line.Touches))
		}
		sb.WriteString("\n")
	}

	// 平行通道分析
	if data.Channel != nil {
		sb.WriteString(fmt.Sprintf("Parallel Channel (%s trend):\n", data.Channel.Direction))
		sb.WriteString(fmt.Sprintf("  Quality: %.1f%%, Width: %.1f%%, Current Position: %s\n",
			data.Channel.Quality*100, data.Channel.Width*100, data.Channel.CurrentPos))
		sb.WriteString(fmt.Sprintf("  Price Ratio in Channel: %.1f%% (0=lower rail, 100=upper rail)\n\n",
			data.Channel.PriceRatio*100))
	}

	// 交易信号
	if data.TradingSignal != nil {
		sb.WriteString("Trading Signal:\n")
		sb.WriteString(fmt.Sprintf("  Action: %s (%s signal)\n",
			strings.ToUpper(string(data.TradingSignal.Action)), data.TradingSignal.Type))
		sb.WriteString(fmt.Sprintf("  Confidence: %.1f%%, Risk/Reward: %.2f\n",
			data.TradingSignal.Confidence, data.TradingSignal.RiskReward))

		if data.TradingSignal.Entry > 0 {
			sb.WriteString(fmt.Sprintf("  Entry: %.4f", data.TradingSignal.Entry))
			if data.TradingSignal.StopLoss > 0 {
				sb.WriteString(fmt.Sprintf(", Stop Loss: %.4f", data.TradingSignal.StopLoss))
			}
			if data.TradingSignal.TakeProfit > 0 {
				sb.WriteString(fmt.Sprintf(", Take Profit: %.4f", data.TradingSignal.TakeProfit))
			}
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("  Description: %s\n", data.TradingSignal.Description))

		// 显示信号特征
		features := []string{}
		if data.TradingSignal.ChannelBased {
			features = append(features, "channel-based")
		}
		if data.TradingSignal.BreakoutBased {
			features = append(features, "breakout-based")
		}
		if len(features) > 0 {
			sb.WriteString(fmt.Sprintf("  Features: %s\n", strings.Join(features, ", ")))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatVPVRData 格式化VPVR数据
func formatVPVRData(data *VolumeProfile) string {
	if data == nil {
		return "VPVR Analysis: No data available\n\n"
	}

	var sb strings.Builder
	sb.WriteString("Volume Profile Analysis:\n")

	// POC (Point of Control)
	if data.POC != nil {
		sb.WriteString(fmt.Sprintf("  Point of Control (POC): %.4f (%.1f%% volume)\n",
			data.POC.Price, data.POC.VolumePercent))
	}

	// Value Area
	sb.WriteString(fmt.Sprintf("  Value Area: %.4f - %.4f\n", data.VAL, data.VAH))
	if data.ValueArea != nil {
		sb.WriteString(fmt.Sprintf("  Value Area Volume: %.1f%%, Concentration: %.2f\n",
			data.ValueArea.VolumePercent, data.ValueArea.Concentration))
	}

	// Volume statistics
	if data.Stats != nil {
		sb.WriteString(fmt.Sprintf("  Buy/Sell Ratio: %.2f, Avg Price: %.4f\n",
			data.Stats.BuySellRatio, data.Stats.AvgPrice))
		if data.Stats.MaxLevel != nil {
			sb.WriteString(fmt.Sprintf("  Highest Volume Level: %.4f\n", data.Stats.MaxLevel.Price))
		}
	}

	// Key levels
	if len(data.Levels) > 0 {
		sb.WriteString("  High Volume Nodes:\n")
		count := 0
		for _, level := range data.Levels {
			if level.VolumePercent > 5.0 && count < 3 { // Top 3 high volume levels
				sb.WriteString(fmt.Sprintf("    %.4f (%.1f%% volume)\n",
					level.Price, level.VolumePercent))
				count++
			}
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// formatSupplyDemandData 格式化供需区数据
func formatSupplyDemandData(data *SupplyDemandData) string {
	if data == nil {
		return "Supply/Demand Zones Analysis: No data available\n\n"
	}

	var sb strings.Builder
	sb.WriteString("Supply/Demand Zones Analysis:\n")

	// Active zones summary
	if len(data.ActiveZones) > 0 {
		supplyCount := 0
		demandCount := 0
		for _, zone := range data.ActiveZones {
			if zone.Type == SupplyZone {
				supplyCount++
			} else {
				demandCount++
			}
		}
		sb.WriteString(fmt.Sprintf("  Active Zones: %d total (%d supply, %d demand)\n",
			len(data.ActiveZones), supplyCount, demandCount))

		// Show top zones by strength
		sb.WriteString("  Key Zones:\n")
		count := 0
		for _, zone := range data.ActiveZones {
			if count >= 3 { // Show top 3 zones
				break
			}
			zoneType := "Demand"
			if zone.Type == SupplyZone {
				zoneType = "Supply"
			}
			sb.WriteString(fmt.Sprintf("    %s Zone: %.4f-%.4f (Strength: %.1f, Touches: %d)\n",
				zoneType, zone.LowerBound, zone.UpperBound, zone.Strength, zone.TouchCount))
			count++
		}
	}

	// Statistics
	if data.Statistics != nil {
		sb.WriteString(fmt.Sprintf("  Zone Statistics:\n"))
		sb.WriteString(fmt.Sprintf("    Success Rate: %.1f%%, Average Strength: %.1f\n",
			data.Statistics.SuccessRate, data.Statistics.AvgZoneStrength))
		sb.WriteString(fmt.Sprintf("    Active Supply: %d, Active Demand: %d\n",
			data.Statistics.ActiveSupplyZones, data.Statistics.ActiveDemandZones))
	}

	sb.WriteString("\n")
	return sb.String()
}

// formatFVGData 格式化FVG数据
func formatFVGData(data *FVGData) string {
	if data == nil {
		return "Fair Value Gap Analysis: No data available\n\n"
	}

	var sb strings.Builder
	sb.WriteString("Fair Value Gap (FVG) Analysis:\n")

	// Active FVGs summary
	if len(data.ActiveFVGs) > 0 {
		bullishCount := 0
		bearishCount := 0
		for _, fvg := range data.ActiveFVGs {
			if fvg.Type == BullishFVG {
				bullishCount++
			} else {
				bearishCount++
			}
		}
		sb.WriteString(fmt.Sprintf("  Active FVGs: %d total (%d bullish, %d bearish)\n",
			len(data.ActiveFVGs), bullishCount, bearishCount))

		// Show key FVGs
		sb.WriteString("  Key Fair Value Gaps:\n")
		count := 0
		for _, fvg := range data.ActiveFVGs {
			if count >= 3 { // Show top 3 FVGs
				break
			}
			fvgType := "Bullish"
			if fvg.Type == BearishFVG {
				fvgType = "Bearish"
			}
			sb.WriteString(fmt.Sprintf("    %s FVG: %.4f-%.4f (Strength: %.1f, Status: %s)\n",
				fvgType, fvg.LowerBound, fvg.UpperBound, fvg.Strength, fvg.Status))
			count++
		}
	}

	// Statistics
	if data.Statistics != nil {
		sb.WriteString("  FVG Statistics:\n")
		sb.WriteString(fmt.Sprintf("    Fill Rate: %.1f%%, Average Width: %.4f\n",
			data.Statistics.FillRate*100, data.Statistics.AvgFVGWidth))
		sb.WriteString(fmt.Sprintf("    Active Bullish: %d, Active Bearish: %d\n",
			data.Statistics.ActiveBullishFVGs, data.Statistics.ActiveBearishFVGs))
	}

	sb.WriteString("\n")
	return sb.String()
}

// formatMultiTimeframeAnalysis 格式化多时间框架分析
func formatMultiTimeframeAnalysis(data *MultiTimeframeAnalysis) string {
	if data == nil {
		return "Multi-Timeframe Analysis: No data available\n\n"
	}

	var sb strings.Builder
	sb.WriteString("Multi-Timeframe Technical Analysis Summary:\n")

	// 总体趋势一致性
	if data.Summary != nil {
		sb.WriteString(fmt.Sprintf("  Overall Trend: %s (Consistency: %.1f%%)\n",
			strings.Title(data.Summary.OverallTrend), data.Summary.TrendConsistency*100))
		sb.WriteString(fmt.Sprintf("  Signal Confidence: %.1f%%\n",
			data.Summary.SignalConfidence*100))

		// 时间框架一致性
		if len(data.Summary.TimeframeAlignment) > 0 {
			sb.WriteString("  Timeframe Alignment: ")
			alignedCount := 0
			for _, aligned := range data.Summary.TimeframeAlignment {
				if aligned {
					alignedCount++
				}
			}
			sb.WriteString(fmt.Sprintf("%d/%d timeframes aligned\n", alignedCount, len(data.Summary.TimeframeAlignment)))
		}

		// 关键价位统计
		if data.Summary.KeyLevels != nil {
			supportCount := len(data.Summary.KeyLevels.SupportLevels)
			resistanceCount := len(data.Summary.KeyLevels.ResistanceLevels)
			pivotCount := len(data.Summary.KeyLevels.PivotLevels)
			if supportCount+resistanceCount+pivotCount > 0 {
				sb.WriteString(fmt.Sprintf("  Key Levels: %d support, %d resistance, %d pivot points\n",
					supportCount, resistanceCount, pivotCount))
			}
		}

		// 交易信号
		if len(data.Summary.TradingSignals) > 0 {
			sb.WriteString(fmt.Sprintf("  Cross-Timeframe Signals: %d active\n", len(data.Summary.TradingSignals)))
			for i, signal := range data.Summary.TradingSignals {
				if i >= 2 { // 只显示前2个最重要的信号
					break
				}
				sb.WriteString(fmt.Sprintf("    %d. %s signal (Confidence: %.1f%%, Timeframe: %s)\n",
					i+1, strings.ToUpper(string(signal.PrimaryAction)), signal.Confidence, signal.Timeframe))
			}
		}

		// 风险评估
		if data.Summary.RiskAssessment != nil {
			sb.WriteString(fmt.Sprintf("  Risk Assessment: %s (Max Position: %.1f%%)\n",
				strings.Title(data.Summary.RiskAssessment.OverallRisk),
				data.Summary.RiskAssessment.MaxPositionSize*100))
			if data.Summary.RiskAssessment.ConflictingSignals > 0 {
				sb.WriteString(fmt.Sprintf("  Conflicting Signals: %d detected\n",
					data.Summary.RiskAssessment.ConflictingSignals))
			}
		}
	}

	// 各时间框架可靠性
	if len(data.Timeframes) > 0 {
		sb.WriteString("\n  Timeframe Reliability Scores:\n")
		timeframeOrder := []string{"5m", "15m", "30m", "1h", "4h"}
		for _, tf := range timeframeOrder {
			if tfData, exists := data.Timeframes[tf]; exists {
				sb.WriteString(fmt.Sprintf("    %s: %.1f%% (Weight: %.1f%%)\n",
					tf, tfData.Reliability*100, tfData.Weight*100))
			}
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// formatFibonacciData 格式化斐波纳契分析数据
func formatFibonacciData(data *FibonacciData) string {
	if data == nil {
		return "Fibonacci Analysis: No data available\n\n"
	}

	var sb strings.Builder

	// 斐波纳契回调分析
	if len(data.Retracements) > 0 {
		sb.WriteString("Fibonacci Retracements:\n")
		for i, ret := range data.Retracements {
			if i >= 3 { // 只显示前3个最重要的
				break
			}
			if !ret.IsActive {
				continue
			}

			trendDir := "Uptrend"
			if ret.TrendType == TrendDownward {
				trendDir = "Downtrend"
			}

			qualityStr := "High"
			if ret.Quality == FibQualityMedium {
				qualityStr = "Medium"
			} else if ret.Quality == FibQualityLow {
				qualityStr = "Low"
			}

			sb.WriteString(fmt.Sprintf("  • %s Retracement (Quality: %s, Strength: %.1f)\n",
				trendDir, qualityStr, ret.Strength))
			sb.WriteString(fmt.Sprintf("    Range: %.4f → %.4f\n",
				ret.StartPoint.Price, ret.EndPoint.Price))

			// 显示关键斐波级别
			for _, level := range ret.Levels {
				if level.Importance >= 0.7 { // 只显示重要级别
					goldenStar := ""
					if level.IsGoldenRatio {
						goldenStar = " ★"
					}
					sb.WriteString(fmt.Sprintf("    %.1f%% Level: %.4f%s\n",
						level.Ratio*100, level.Price, goldenStar))
				}
			}
			sb.WriteString("\n")
		}
	}

	// 黄金口袋分析
	if data.GoldenPocket != nil && data.GoldenPocket.IsActive {
		pocket := data.GoldenPocket
		sb.WriteString("Golden Pocket (0.618) Analysis:\n")

		qualityStr := "High"
		if pocket.Quality == FibQualityMedium {
			qualityStr = "Medium"
		} else if pocket.Quality == FibQualityLow {
			qualityStr = "Low"
		}

		trendContext := "Uptrend Support"
		if pocket.TrendContext == TrendDownward {
			trendContext = "Downtrend Resistance"
		}

		sb.WriteString(fmt.Sprintf("  • Range: %.4f - %.4f (Center: %.4f)\n",
			pocket.PriceRange.Low, pocket.PriceRange.High, pocket.CenterPrice))
		sb.WriteString(fmt.Sprintf("  • Quality: %s (Strength: %.1f)\n",
			qualityStr, pocket.Strength))
		sb.WriteString(fmt.Sprintf("  • Context: %s\n", trendContext))

		if len(pocket.TouchEvents) > 0 {
			recentTouches := len(pocket.TouchEvents)
			if recentTouches > 3 {
				recentTouches = 3
			}
			sb.WriteString(fmt.Sprintf("  • Recent Interactions: %d times\n", recentTouches))
		}
		sb.WriteString("\n")
	}

	// 斐波扩展分析
	if len(data.Extensions) > 0 {
		sb.WriteString("Fibonacci Extensions:\n")
		validExtensions := 0
		for _, ext := range data.Extensions {
			if ext.Quality != FibQualityHigh || validExtensions >= 2 {
				continue
			}
			validExtensions++

			sb.WriteString(fmt.Sprintf("  • Base Wave: %.4f → %.4f\n",
				ext.BaseWave.StartPoint.Price, ext.BaseWave.EndPoint.Price))
			sb.WriteString(fmt.Sprintf("    Projected Targets:\n"))

			for _, level := range ext.Levels {
				if level.Ratio == 1.272 || level.Ratio == 1.618 {
					sb.WriteString(fmt.Sprintf("    %.3f Extension: %.4f\n",
						level.Ratio, level.Price))
				}
			}
			sb.WriteString("\n")
		}
	}

	// 斐波聚集区
	if len(data.Clusters) > 0 {
		sb.WriteString("Fibonacci Confluence Zones:\n")
		for i, cluster := range data.Clusters {
			if i >= 2 || cluster.Importance < 70 { // 只显示前2个重要的
				break
			}
			sb.WriteString(fmt.Sprintf("  • Zone at %.4f (Importance: %.1f)\n",
				cluster.CenterPrice, cluster.Importance))
			sb.WriteString(fmt.Sprintf("    Contains %d fibonacci levels\n",
				cluster.LevelCount))
		}
		sb.WriteString("\n")
	}

	// 统计概览
	if data.Statistics != nil {
		stats := data.Statistics
		sb.WriteString("Fibonacci Analysis Summary:\n")
		sb.WriteString(fmt.Sprintf("  • Active Retracements: %d (High Quality: %d)\n",
			stats.ActiveRetracements, stats.HighQualityCount))
		if stats.GoldenRatioHits > 0 {
			sb.WriteString(fmt.Sprintf("  • Golden Ratio Reactions: %d times\n",
				stats.GoldenRatioHits))
		}
		if stats.SuccessRate > 0 {
			sb.WriteString(fmt.Sprintf("  • Success Rate: %.1f%%\n",
				stats.SuccessRate*100))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// getRecentSwingPoints 获取最近的确认摆动点
func getRecentSwingPoints(points []*SwingPoint, count int) []*SwingPoint {
	var confirmed []*SwingPoint
	for _, point := range points {
		if point.Confirmed {
			confirmed = append(confirmed, point)
		}
	}

	if len(confirmed) <= count {
		return confirmed
	}

	// 按时间排序，返回最近的几个
	return confirmed[len(confirmed)-count:]
}

// getStrongestTrendLines 获取最强的趋势线
func getStrongestTrendLines(lines []*TrendLine, count int) []*TrendLine {
	if len(lines) <= count {
		return lines
	}

	// 复制切片以避免修改原数据
	sorted := make([]*TrendLine, len(lines))
	copy(sorted, lines)

	// 按强度排序（已经在原函数中排序过了）
	return sorted[:count]
}

// Normalize 标准化symbol,确保是USDT交易对
func Normalize(symbol string) string {
	symbol = strings.ToUpper(symbol)
	if strings.HasSuffix(symbol, "USDT") {
		return symbol
	}
	return symbol + "USDT"
}

// parseFloat 解析float值
func parseFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case string:
		return strconv.ParseFloat(val, 64)
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}

// GetMultiSymbolAnalysis 获取多个币种的多时间框架分析数据
func GetMultiSymbolAnalysis(symbols []string) (map[string]map[string]interface{}, error) {
	result := make(map[string]map[string]interface{})

	for _, symbol := range symbols {
		// 标准化symbol
		normalizedSymbol := Normalize(symbol)

		// 获取市场数据
		data, err := Get(normalizedSymbol)
		if err != nil {
			fmt.Printf("获取%s市场数据失败: %v\n", normalizedSymbol, err)
			continue
		}

		// 构建时间框架数据
		symbolData := map[string]interface{}{
			"5m": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "dow_theory"),
				"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "channel_analysis"),
				"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "volume_profile"),
				"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "supply_demand"),
				"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "fibonacci"),
			},
			"15m": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "dow_theory"),
				"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "channel_analysis"),
				"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "volume_profile"),
				"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "supply_demand"),
				"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fibonacci"),
			},
			"30m": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "dow_theory"),
				"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "channel_analysis"),
				"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "volume_profile"),
				"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "supply_demand"),
				"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fibonacci"),
			},
			"1h": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "dow_theory"),
				"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "channel_analysis"),
				"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "volume_profile"),
				"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "supply_demand"),
				"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fibonacci"),
			},
			"4h": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "dow_theory"),
				"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "channel_analysis"),
				"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "volume_profile"),
				"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "supply_demand"),
				"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "fibonacci"),
			},
		}

		result[normalizedSymbol] = symbolData
	}

	return result, nil
}

// extractTimeframeData 从多时间框架分析中提取特定时间框架的特定分析类型数据
func extractTimeframeData(multiTimeframeAnalysis *MultiTimeframeAnalysis, timeframe, analysisType string) interface{} {
	if multiTimeframeAnalysis == nil || multiTimeframeAnalysis.Timeframes == nil {
		return map[string]interface{}{}
	}

	timeframeData, exists := multiTimeframeAnalysis.Timeframes[timeframe]
	if !exists || timeframeData == nil {
		return map[string]interface{}{}
	}

	switch analysisType {
	case "dow_theory":
		if timeframeData.DowTheory != nil {
			return timeframeData.DowTheory
		}
	case "channel_analysis":
		if timeframeData.ChannelAnalysis != nil {
			return timeframeData.ChannelAnalysis
		}
	case "volume_profile":
		if timeframeData.VolumeProfile != nil {
			return timeframeData.VolumeProfile
		}
	case "supply_demand":
		if timeframeData.SupplyDemand != nil {
			return timeframeData.SupplyDemand
		}
	case "fair_value_gaps":
		if timeframeData.FairValueGaps != nil {
			return timeframeData.FairValueGaps
		}
	case "fibonacci":
		if timeframeData.Fibonacci != nil {
			return timeframeData.Fibonacci
		}
	}

	return map[string]interface{}{}
}

// GetSingleSymbolAnalysis 获取单个币种的多时间框架分析数据
func GetSingleSymbolAnalysis(symbol string) (map[string]interface{}, error) {
	// 标准化symbol
	normalizedSymbol := Normalize(symbol)

	// 获取市场数据
	data, err := Get(normalizedSymbol)
	if err != nil {
		return nil, fmt.Errorf("获取%s市场数据失败: %v", normalizedSymbol, err)
	}

	// 构建时间框架数据
	symbolData := map[string]interface{}{
		"5m": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "dow_theory"),
			"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "channel_analysis"),
			"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "volume_profile"),
			"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "supply_demand"),
			"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "fibonacci"),
		},
		"15m": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "dow_theory"),
			"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "channel_analysis"),
			"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "volume_profile"),
			"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "supply_demand"),
			"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fibonacci"),
		},
		"30m": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "dow_theory"),
			"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "channel_analysis"),
			"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "volume_profile"),
			"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "supply_demand"),
			"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fibonacci"),
		},
		"1h": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "dow_theory"),
			"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "channel_analysis"),
			"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "volume_profile"),
			"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "supply_demand"),
			"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fibonacci"),
		},
		"4h": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "dow_theory"),
			"通道数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "channel_analysis"),
			"VPVR数据":     extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "volume_profile"),
			"供需区数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "supply_demand"),
			"FVG数据":      extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "fibonacci"),
		},
	}

	return symbolData, nil
}

// CalculateMediumTermData 计算中期时间框架数据(15m/30m/1h) - 导出供测试使用
func CalculateMediumTermData(klines []Kline, timeframe string) *MediumTermData {
	return calculateMediumTermData(klines, timeframe)
}

// calculateMediumTermData 计算中期时间框架数据(15m/30m/1h)
// 🔥 P0-01修复：calculateMediumTermData 使用锚点裁剪后的数据，确保时间一致性
// 移除lastClosedIndex逻辑，直接使用已经过滤的K线数据
func calculateMediumTermData(klines []Kline, timeframe string) *MediumTermData {
	if len(klines) == 0 {
		return &MediumTermData{Timeframe: timeframe}
	}

	data := &MediumTermData{
		Timeframe:   timeframe,
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 🔥 P0-01修复：直接使用传入的已裁剪K线数据，所有指标基于相同时间基准
	// 计算EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)

	// 计算当前指标
	data.CurrentMACD = calculateMACD(klines)
	data.CurrentRSI7 = calculateRSI(klines, 7)
	data.CurrentRSI14 = calculateRSI(klines, 14)

	// 计算ATR
	data.ATR14 = calculateATR(klines, 14)

	// 🔥 P0-01修复：成交量计算使用锚点裁剪后的数据
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume // 使用最后一根K线的成交量
		// 计算平均成交量
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// 🔥 P0-01修复：OHLC数据提取使用锚点裁剪后的数据
	if timeframe == "15m" || timeframe == "30m" {
		// 15m/30m级别: 最后已收盘 + 上一根已收盘
		lastIdx := len(klines) - 1
		if lastIdx >= 0 {
			data.OHLCLastClosed = extractOHLCData(klines[lastIdx]) // 最后已收盘
			if lastIdx >= 1 {
				data.OHLCPrevClosed = extractOHLCData(klines[lastIdx-1]) // 上一根已收盘
			}
		}
	} else if timeframe == "1h" {
		// 1h级别: 使用最后已收盘K线
		lastIdx := len(klines) - 1
		if lastIdx >= 0 {
			data.OHLCLastClosed = extractOHLCData(klines[lastIdx]) // 统一锚点
		}
	}

	// 🔥 P0-01修复：MACD和RSI序列计算基于锚点裁剪后的数据
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// SuperTrendResult 超级趋势计算结果（增强版）
type SuperTrendResult struct {
	// 基础字段
	Direction   string  // "bullish" or "bearish"
	CurrentLine float64 // 当前趋势线价格
	UpperLine   float64 // 上轨价格
	LowerLine   float64 // 下轨价格

	// 🔥 新增：增强结果结构
	IsValid       bool    // 数据是否有效
	TrendDuration int     // 当前趋势持续时间（K线数量）
	FlipPrice     float64 // 最近一次翻转的价格

	// 🔥 新增：趋势强度量化
	TrendStrength float64 // 趋势强度评分 (0-100)
	Confidence    float64 // 置信度评分 (0-1)

	// 🔥 新增：信号质量
	SignalQuality string // "high", "medium", "low"
	LastFlipTime  int64  // 最后翻转时间戳
}

// calculateSupertrendEnhanced 计算增强版超级趋势线
// 🔥 功能：修复初始化逻辑、增加趋势强度量化、实现动态参数配置
func calculateSupertrendEnhanced(klines []Kline, timeframe string, customParams ...float64) SuperTrendResult {
	result := SuperTrendResult{
		Direction:     "unknown",
		CurrentLine:   0.0,
		UpperLine:     0.0,
		LowerLine:     0.0,
		IsValid:       false,
		TrendDuration: 0,
		FlipPrice:     0.0,
		TrendStrength: 0.0,
		Confidence:    0.0,
		SignalQuality: "low",
		LastFlipTime:  0,
	}

	// 🔥 动态参数配置：根据时间框架自动调整
	atrPeriod, factor := getDynamicSupertrendParams(timeframe, customParams...)

	minRequired := atrPeriod + 1
	recommended := atrPeriod * 3 // 建议使用ATR周期的3倍数据以确保稳定性
	if len(klines) < minRequired {
		log.Printf("🚨🔴 [SuperTrend增强版] ❌ K线数据不足: 需要%d根，实际%d根 ❌", minRequired, len(klines))
		return result
	}
	if len(klines) < recommended {
		log.Printf("🟡⚠️ [SuperTrend增强版] 稳定性警告: 建议%d根，实际%d根 (可能影响趋势稳定性) ⚠️🟡", recommended, len(klines))
	}

	length := len(klines)

	// 1. 🔥 改进的ATR计算：使用Wilder's Smoothing (RMA)
	atrs := calculateEnhancedATRSeries(klines, atrPeriod)
	if atrs == nil {
		return result
	}

	// 2. 🔥 增强的SuperTrend计算
	supertrendLines := make([]float64, length)
	directions := make([]string, length)
	upperBands := make([]float64, length)
	lowerBands := make([]float64, length)
	flipPoints := make([]int, 0) // 记录翻转点

	// 3. 🔥 改进的初始化逻辑
	startIdx := atrPeriod
	hl2 := (klines[startIdx].High + klines[startIdx].Low) / 2
	upperBands[startIdx] = hl2 + (factor * atrs[startIdx])
	lowerBands[startIdx] = hl2 - (factor * atrs[startIdx])

	// 🔥 智能初始化方向：综合考虑收盘价位置和最近价格趋势
	if isInitialBullish(klines, startIdx, upperBands[startIdx], lowerBands[startIdx]) {
		directions[startIdx] = "bullish"
		supertrendLines[startIdx] = lowerBands[startIdx]
	} else {
		directions[startIdx] = "bearish"
		supertrendLines[startIdx] = upperBands[startIdx]
	}

	// 4. 主循环计算
	currentTrendStart := startIdx
	for i := startIdx + 1; i < length; i++ {
		// 计算基础带
		hl2 := (klines[i].High + klines[i].Low) / 2
		currATR := atrs[i]

		basicUpper := hl2 + (factor * currATR)
		basicLower := hl2 - (factor * currATR)

		// 🔥 核心改进：更精确的带平滑处理
		upperBands[i] = calculateSmoothedBand(basicUpper, upperBands[i-1], klines[i-1].Close, "upper")
		lowerBands[i] = calculateSmoothedBand(basicLower, lowerBands[i-1], klines[i-1].Close, "lower")

		// 🔥 改进的方向确定逻辑
		prevDir := directions[i-1]
		currDir := determineDirection(prevDir, klines[i].Close, upperBands[i], lowerBands[i])
		directions[i] = currDir

		// 记录趋势翻转
		if prevDir != currDir && prevDir != "unknown" {
			flipPoints = append(flipPoints, i)
			currentTrendStart = i
		}

		// 确定当前趋势线数值
		if currDir == "bullish" {
			supertrendLines[i] = lowerBands[i]
		} else {
			supertrendLines[i] = upperBands[i]
		}
	}

	// 5. 🔥 计算增强指标
	lastIdx := length - 1
	result.Direction = directions[lastIdx]
	result.CurrentLine = supertrendLines[lastIdx]
	result.UpperLine = upperBands[lastIdx]
	result.LowerLine = lowerBands[lastIdx]
	result.IsValid = true

	// 🔥 趋势持续时间
	if len(flipPoints) > 0 {
		result.TrendDuration = lastIdx - flipPoints[len(flipPoints)-1]
		result.FlipPrice = (klines[flipPoints[len(flipPoints)-1]].High + klines[flipPoints[len(flipPoints)-1]].Low) / 2
		result.LastFlipTime = klines[flipPoints[len(flipPoints)-1]].OpenTime
	} else {
		result.TrendDuration = lastIdx - currentTrendStart
		result.FlipPrice = 0.0
		result.LastFlipTime = 0
	}

	// 🔥 趋势强度量化
	result.TrendStrength = calculateTrendStrength(klines, directions, supertrendLines, result.TrendDuration, lastIdx)

	// 🔥 置信度评分
	result.Confidence = calculateConfidence(klines, atrs, result.TrendDuration, flipPoints, lastIdx)

	// 🔥 信号质量评估
	result.SignalQuality = determineSignalQuality(result.TrendStrength, result.Confidence, result.TrendDuration, len(klines))

	return result
}

// getDynamicSupertrendParams 获取动态SuperTrend参数
// 🔥 功能：根据时间框架动态调整参数，LTF使用较小Factor，HTF使用较大Factor
func getDynamicSupertrendParams(timeframe string, customParams ...float64) (int, float64) {
	// 如果提供了自定义参数，使用自定义参数
	if len(customParams) >= 2 {
		return int(customParams[0]), customParams[1]
	}

	// 根据时间框架动态配置参数
	switch timeframe {
	case "1m", "3m", "5m":
		// 低时间框架：更敏感的参数配置
		return 10, 3.0 // ATR周期10，Factor 3.0
	case "15m", "30m":
		// 中时间框架：平衡的参数配置
		return 14, 3.5 // ATR周期14，Factor 3.5
	case "1h", "2h", "4h":
		// 高时间框架：更稳定的参数配置
		return 20, 4.0 // ATR周期20，Factor 4.0
	case "12h", "1d":
		// 超高时间框架：最稳定的参数配置
		return 14, 5.0 // ATR周期14，Factor 5.0
	default:
		// 默认配置：标准参数
		return 14, 3.0 // ATR周期14，Factor 3.0
	}
}

// calculateEnhancedATRSeries 计算增强版ATR序列
// 🔥 功能：使用标准的Wilder's Smoothing方法，符合TradingView标准
func calculateEnhancedATRSeries(klines []Kline, period int) []float64 {
	length := len(klines)
	if length <= period {
		return nil
	}

	atrs := make([]float64, length)

	// 🔥 计算True Range序列
	trs := make([]float64, length)
	for i := 1; i < length; i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close
		trs[i] = math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
	}

	// 🔥 计算第一个ATR (使用SMA)
	sumTR := 0.0
	for i := 1; i <= period; i++ {
		sumTR += trs[i]
	}
	atrs[period] = sumTR / float64(period)

	// 🔥 计算后续ATR (使用Wilder's RMA)
	// 公式：ATR = ((previous ATR) * (period - 1) + current TR) / period
	for i := period + 1; i < length; i++ {
		atrs[i] = (atrs[i-1]*float64(period-1) + trs[i]) / float64(period)
	}

	return atrs
}

// isInitialBullish 智能判断初始趋势方向
// 🔥 功能：改进初始化逻辑，避免错误的初始方向判断
func isInitialBullish(klines []Kline, startIdx int, upperBand, lowerBand float64) bool {
	currentClose := klines[startIdx].Close

	// 基础判断：收盘价与带的关系
	if currentClose > upperBand {
		return true
	}
	if currentClose < lowerBand {
		return false
	}

	// 增强判断：分析最近几根K线的趋势
	lookback := 5
	if startIdx < lookback {
		lookback = startIdx
	}

	bullishSignals := 0
	for i := startIdx - lookback; i <= startIdx; i++ {
		if i > 0 {
			// 检查收盘价趋势
			if klines[i].Close > klines[i-1].Close {
				bullishSignals++
			}
			// 检查实体强度
			bodySize := math.Abs(klines[i].Close - klines[i].Open)
			shadowSize := (klines[i].High - klines[i].Low) - bodySize
			if bodySize > shadowSize && klines[i].Close > klines[i].Open {
				bullishSignals++
			}
		}
	}

	// 如果看涨信号数量超过总信号的50%，则判断为看涨
	return bullishSignals > lookback
}

// calculateSmoothedBand 计算平滑的带线
// 🔥 功能：更精确的带平滑处理，减少假信号
func calculateSmoothedBand(basicBand, prevBand, prevClose float64, bandType string) float64 {
	if bandType == "upper" {
		// 上轨：只能下降，除非价格突破了前一根的上轨
		if basicBand < prevBand || prevClose > prevBand {
			return basicBand
		} else {
			return prevBand
		}
	} else { // lower
		// 下轨：只能上升，除非价格跌破了前一根的下轨
		if basicBand > prevBand || prevClose < prevBand {
			return basicBand
		} else {
			return prevBand
		}
	}
}

// determineDirection 确定趋势方向
// 🔥 功能：改进的方向确定逻辑，减少频繁翻转
func determineDirection(prevDirection string, currentClose, upperBand, lowerBand float64) string {
	switch prevDirection {
	case "bullish":
		// 只有明确跌破下轨才转为看跌
		if currentClose < lowerBand {
			return "bearish"
		}
		return "bullish"
	case "bearish":
		// 只有明确突破上轨才转为看涨
		if currentClose > upperBand {
			return "bullish"
		}
		return "bearish"
	default:
		// 初始状态：根据价格位置确定方向
		if currentClose > upperBand {
			return "bullish"
		} else if currentClose < lowerBand {
			return "bearish"
		} else {
			return "bullish" // 默认看涨
		}
	}
}

// calculateTrendStrength 计算趋势强度
// 🔥 功能：量化趋势的强度，考虑价格距离、成交量、一致性等因素
func calculateTrendStrength(klines []Kline, directions []string, supertrendLines []float64, duration, lastIdx int) float64 {
	if duration <= 0 || lastIdx < duration {
		return 0.0
	}

	totalStrength := 0.0
	factors := 0

	// 1. 价格距离强度 (权重: 30%)
	currentPrice := klines[lastIdx].Close
	supertrendPrice := supertrendLines[lastIdx]
	priceDistance := math.Abs(currentPrice-supertrendPrice) / currentPrice
	distanceStrength := math.Min(priceDistance*500, 100) // 距离越大，强度越高，最大100
	totalStrength += distanceStrength * 0.3
	factors++

	// 2. 趋势一致性强度 (权重: 25%)
	consistentBars := 0
	startIdx := lastIdx - duration + 1
	if startIdx < 0 {
		startIdx = 0
	}

	currentDirection := directions[lastIdx]
	for i := startIdx; i <= lastIdx; i++ {
		if directions[i] == currentDirection {
			consistentBars++
		}
	}
	consistencyStrength := float64(consistentBars) / float64(duration) * 100
	totalStrength += consistencyStrength * 0.25

	// 3. 成交量支撑强度 (权重: 20%)
	if len(klines) > duration {
		avgVolume := 0.0
		for i := startIdx; i <= lastIdx; i++ {
			avgVolume += klines[i].Volume
		}
		avgVolume /= float64(duration)

		recentVolume := klines[lastIdx].Volume
		volumeRatio := recentVolume / avgVolume
		volumeStrength := math.Min(volumeRatio*50, 100) // 成交量比率转换为强度
		totalStrength += volumeStrength * 0.2
	}

	// 4. 持续时间强度 (权重: 15%)
	durationStrength := math.Min(float64(duration)*2, 100) // 持续时间越长，强度越高
	totalStrength += durationStrength * 0.15

	// 5. 价格动量强度 (权重: 10%)
	if duration >= 3 {
		recentRange := 3
		if duration < 3 {
			recentRange = duration
		}

		momentum := 0.0
		for i := lastIdx - recentRange + 1; i <= lastIdx; i++ {
			if i > 0 {
				priceChange := (klines[i].Close - klines[i-1].Close) / klines[i-1].Close
				if currentDirection == "bullish" && priceChange > 0 {
					momentum += priceChange
				} else if currentDirection == "bearish" && priceChange < 0 {
					momentum += math.Abs(priceChange)
				}
			}
		}
		momentumStrength := math.Min(momentum*1000, 100) // 动量转换为强度
		totalStrength += momentumStrength * 0.1
	}

	return math.Max(0, math.Min(100, totalStrength))
}

// calculateConfidence 计算置信度
// 🔥 功能：评估SuperTrend信号的可靠性
func calculateConfidence(klines []Kline, atrs []float64, duration int, flipPoints []int, lastIdx int) float64 {
	if lastIdx <= 0 || len(atrs) <= lastIdx {
		return 0.0
	}

	confidence := 0.0
	factors := 0

	// 1. 数据充分性 (权重: 25%)
	dataRatio := float64(len(klines)) / float64(50) // 50根K线为基准
	dataConfidence := math.Min(dataRatio, 1.0)
	confidence += dataConfidence * 0.25
	factors++

	// 2. 趋势稳定性 (权重: 20%)
	flipFrequency := 0.0
	if len(flipPoints) > 0 && len(klines) > 0 {
		flipFrequency = float64(len(flipPoints)) / float64(len(klines))
	}
	stabilityConfidence := math.Max(0, 1.0-flipFrequency*10) // 翻转频率越低，稳定性越高
	confidence += stabilityConfidence * 0.2

	// 3. 持续时间置信度 (权重: 20%)
	durationConfidence := math.Min(float64(duration)/20.0, 1.0) // 20根K线为基准
	confidence += durationConfidence * 0.2

	// 4. ATR相对强度 (权重: 15%)
	currentATR := atrs[lastIdx]
	avgATR := 0.0
	atrPeriod := 14
	if lastIdx >= atrPeriod {
		for i := lastIdx - atrPeriod + 1; i <= lastIdx; i++ {
			avgATR += atrs[i]
		}
		avgATR /= float64(atrPeriod)

		atrRatio := currentATR / avgATR
		atrConfidence := math.Max(0, math.Min(1.0, atrRatio)) // ATR相对稳定时置信度较高
		confidence += atrConfidence * 0.15
	}

	// 5. 价格行为一致性 (权重: 20%)
	behaviorConsistency := 0.0
	if duration >= 5 {
		consistentMoves := 0
		totalMoves := 0

		startIdx := lastIdx - duration + 1
		if startIdx < 1 {
			startIdx = 1
		}

		for i := startIdx; i <= lastIdx; i++ {
			if i > 0 {
				priceMove := klines[i].Close - klines[i-1].Close
				totalMoves++

				// 检查价格移动是否与趋势一致
				if (priceMove > 0 && klines[i].Close > klines[i].Open) ||
					(priceMove < 0 && klines[i].Close < klines[i].Open) {
					consistentMoves++
				}
			}
		}

		if totalMoves > 0 {
			behaviorConsistency = float64(consistentMoves) / float64(totalMoves)
		}
	}
	confidence += behaviorConsistency * 0.2

	return math.Max(0, math.Min(1.0, confidence))
}

// determineSignalQuality 确定信号质量
// 🔥 功能：基于趋势强度、置信度等综合评估信号质量
func determineSignalQuality(trendStrength, confidence float64, duration, totalBars int) string {
	// 计算综合评分
	score := (trendStrength * 0.4) + (confidence * 100 * 0.3) + (math.Min(float64(duration)/10, 10) * 10 * 0.2) + (math.Min(float64(totalBars)/100, 1) * 100 * 0.1)

	if score >= 70 && trendStrength >= 60 && confidence >= 0.7 {
		return "high"
	} else if score >= 50 && trendStrength >= 40 && confidence >= 0.5 {
		return "medium"
	} else {
		return "low"
	}
}

// calculateSupertrend 标准SuperTrend计算函数（向后兼容）
// 🔥 P0-04修复：calculateSupertrend 正确透传参数到增强版计算
// 确保atrPeriod和factor参数真正生效，避免"伪配置"问题
func calculateSupertrend(klines []Kline, atrPeriod int, factor float64) SuperTrendResult {
	// 🔥 P0-04修复：透传自定义参数到增强版计算，而不是忽略
	// 将atrPeriod和factor作为customParams传递给calculateSupertrendEnhanced
	return calculateSupertrendEnhanced(klines, "custom", float64(atrPeriod), factor)
}

// calculateATRAtIndex 计算指定位置的ATR
func calculateATRAtIndex(klines []Kline, endIndex, period int) float64 {
	if endIndex < period {
		return 0
	}

	startIndex := endIndex - period + 1
	if startIndex < 1 {
		startIndex = 1
	}

	trs := make([]float64, 0, period)
	for i := startIndex; i <= endIndex; i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		tr := math.Max(tr1, math.Max(tr2, tr3))
		trs = append(trs, tr)
	}

	// 计算平均TR作为ATR
	sum := 0.0
	for _, tr := range trs {
		sum += tr
	}

	return sum / float64(len(trs))
}

// extractCompactMultiTimeframeAnalysisWithSupertrend 提取包含超级趋势的多时间框架分析
func extractCompactMultiTimeframeAnalysisWithSupertrend(data *Data, timeframeKlines map[string][]Kline) map[string]interface{} {
	result := make(map[string]interface{})

	if data.MultiTimeframeAnalysis == nil || data.MultiTimeframeAnalysis.Timeframes == nil {
		return result
	}

	timeframes := []string{"5m", "15m", "30m", "1h", "4h"}

	for _, tf := range timeframes {
		tfData, exists := data.MultiTimeframeAnalysis.Timeframes[tf]
		if !exists || tfData == nil {
			continue
		}

		// 计算该时间框架的超级趋势线
		klines := timeframeKlines[tf]
		supertrend := calculateSupertrend(klines, 20, 5.0)

		result[tf] = map[string]interface{}{
			"道氏理论数据": extractCompactDowTheoryWithSupertrend(tfData.DowTheory, supertrend, data.Symbol),
			"超级趋势指标": map[string]interface{}{
				"direction":    supertrend.Direction,
				"current_line": FormatByDataTypeAndSymbol(supertrend.CurrentLine, "price", data.Symbol),
			},
			"通道数据":       extractCompactChannelAnalysis(tfData.ChannelAnalysis, data.Symbol),
			"VPVR数据":       extractCompactVPVR(tfData.VolumeProfile, data.Symbol),
			"供需区数据":     extractCompactSupplyDemand(tfData.SupplyDemand, data.Symbol),
			"FVG数据":        extractCompactFVG(tfData.FairValueGaps, data.Symbol),
			"斐波纳契数据":   extractCompactFibonacci(tfData.Fibonacci, data.Symbol),
			"支撑阻力转换线": extractCompactSupportResistance(tfData.SupportResistance, data.Symbol),
		}
	}

	return result
}

// extractCompactSupportResistance 提取支撑阻力转换线的关键结果
func extractCompactSupportResistance(data *SupportResistanceData, symbol string) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{
			"support_resistance_lines": []map[string]interface{}{},
			"summary": map[string]interface{}{
				"total_lines":      0,
				"support_lines":    0,
				"resistance_lines": 0,
			},
		}
	}

	result := map[string]interface{}{
		"support_resistance_lines": []map[string]interface{}{},
		"summary": map[string]interface{}{
			"total_lines":      0,
			"support_lines":    0,
			"resistance_lines": 0,
		},
	}

	if len(data.KeyLevels) == 0 {
		return result
	}

	var lines []map[string]interface{}
	supportCount := 0
	resistanceCount := 0

	// 提取关键水平线信息
	for _, level := range data.KeyLevels {
		lineInfo := map[string]interface{}{
			"price":     FormatByDataTypeAndSymbol(level.Price, "price", symbol),
			"type":      level.Type,
			"strength":  FormatByDataTypeAndSymbol(level.Strength, "strength", symbol),
			"hit_count": level.HitCount,
		}
		lines = append(lines, lineInfo)

		if level.Type == "support" {
			supportCount++
		} else if level.Type == "resistance" {
			resistanceCount++
		}
	}

	result["support_resistance_lines"] = lines
	result["summary"] = map[string]interface{}{
		"total_lines":      len(data.KeyLevels),
		"support_lines":    supportCount,
		"resistance_lines": resistanceCount,
	}

	return result
}

// extractCompactDowTheoryWithSupertrend 提取包含超级趋势的道氏理论数据
func extractCompactDowTheoryWithSupertrend(data *DowTheoryData, supertrend SuperTrendResult, symbol string) map[string]interface{} {
	result := map[string]interface{}{
		"trend_direction":   "unknown",
		"trend_strength":    0.0,
		"signal_confidence": 0.0,
		"supertrend": map[string]interface{}{
			"direction":    supertrend.Direction,
			"current_line": FormatByDataTypeAndSymbol(supertrend.CurrentLine, "price", symbol),
			"upper_line":   FormatByDataTypeAndSymbol(supertrend.UpperLine, "price", symbol),
			"lower_line":   FormatByDataTypeAndSymbol(supertrend.LowerLine, "price", symbol),
		},
	}

	if data != nil && data.TrendStrength != nil {
		result["trend_direction"] = data.TrendStrength.Direction
		result["trend_strength"] = FormatByDataTypeAndSymbol(data.TrendStrength.Overall, "strength", symbol)
		result["signal_confidence"] = FormatByDataTypeAndSymbol(data.TrendStrength.Consistency, "confidence", symbol)
	}

	return result
}

// ===== 订单流数据集成 =====

// getOrderFlowDataForAI 获取指定币种的订单流数据（供AI使用 - V2.0版本）
func getOrderFlowDataForAI(symbol string) map[string]interface{} {
	// 尝试获取全局订单流管理器
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️ 订单流数据获取失败 %s: %v", symbol, r)
		}
	}()

	// 检查订单流系统是否已初始化
	ofm := microstructure.GetGlobalOrderFlowManager()
	if ofm == nil {
		return map[string]interface{}{
			"状态":          "订单流系统未初始化",
			"本周期博弈_5m": buildEmptyCurrentPeriodDataV2(),
			"宏观资金趋势":  buildEmptyMacroTrendDataV2(),
			"盘口结构_v2":   buildEmptyOrderBookDataV2(),
			"数据质量":      buildEmptyDataQualityInfoV2(),
		}
	}

	// 获取市场快照（V2.0增强版本）
	snapshot := ofm.GetMarketSnapshot(symbol)
	if snapshot == nil {
		return map[string]interface{}{
			"状态":          "暂无订单流数据",
			"本周期博弈_5m": buildEmptyCurrentPeriodDataV2(),
			"宏观资金趋势":  buildEmptyMacroTrendDataV2(),
			"盘口结构_v2":   buildEmptyOrderBookDataV2(),
			"数据质量":      buildEmptyDataQualityInfoV2(),
		}
	}

	// 检查数据是否过期
	if snapshot.CVDData.IsStale || snapshot.OIAnalysis.IsStale || snapshot.OrderBookData.IsStale {
		return map[string]interface{}{
			"状态": "数���过期",
			"本周期博弈_5m": map[string]interface{}{
				"spot_cvd_delta_usd":    0,
				"futures_cvd_delta_usd": 0,
				"candle_intent":         "data_insufficient",
				"数据质量":              "过期",
			},
			"宏观资金趋势": map[string]interface{}{
				"spot_cvd_1h_usd":    0,
				"futures_cvd_1h_usd": 0,
				"数据质量":           "过期",
			},
		}
	}

	// V2.0: 获取5分钟增量CVD数据 - 直接从快照获取
	snapshot = ofm.GetMarketSnapshot(symbol)
	if snapshot == nil {
		return make(map[string]interface{})
	}

	// 构建V2.0 AI数据格式
	result := map[string]interface{}{
		"本周期博弈_5m": buildCurrentPeriodData(snapshot),
		"宏观资金趋势":  buildMacroTrendData(snapshot),
		"盘口结构_v2":   buildEnhancedOrderBookData(snapshot.OrderBookData),
		"数据质量":      buildDataQualityInfo(snapshot),
	}

	// 🔥 P1-2修复：订单流输出JSON schema验证
	validationResult := ValidateAIOutput(result, "orderflow")
	if validationResult.ProcessedData != nil && len(validationResult.CorrectedFields) > 0 {
		log.Printf("🔧 [P1-2] 订单流输出自动纠正了 %d 个字段", len(validationResult.CorrectedFields))
		if correctedResult, ok := validationResult.ProcessedData.(map[string]interface{}); ok {
			return correctedResult
		}
	}

	return result
}

// buildCurrentPeriodData 构建当前周期数据（5分钟增量）
func buildCurrentPeriodData(snapshot *microstructure.MarketSnapshot) map[string]interface{} {
	// 从缓存获取5分钟增量数据
	globalCache := microstructure.GetGlobalCache()
	cvdDelta5m := globalCache.GetCVDDelta5m(snapshot.Symbol)

	if cvdDelta5m == nil {
		// 如果没有缓存数据，创建默认结构
		return map[string]interface{}{
			"price_delta_pct":       0.0,
			"spot_cvd_delta_usd":    0,
			"futures_cvd_delta_usd": 0,
			"oi_delta_pct":          0.0,
			"candle_intent":         "data_insufficient",
			"volume_delta":          0,
			"volume_ratio":          1.0,
			"period_minutes":        5,
			"data_quality":          0.5,
			"analysis_time":         time.Now().Format("15:04:05"),
		}
	}

	return map[string]interface{}{
		"price_delta_pct":       FormatByDataTypeAndSymbol(cvdDelta5m.PriceDeltaPct, "percentage", snapshot.Symbol),
		"spot_cvd_delta_usd":    FormatByDataTypeAndSymbol(cvdDelta5m.SpotCVDDeltaUSD, "volume", snapshot.Symbol),
		"futures_cvd_delta_usd": FormatByDataTypeAndSymbol(cvdDelta5m.FuturesCVDDeltaUSD, "volume", snapshot.Symbol),
		"oi_delta_pct":          FormatByDataTypeAndSymbol(cvdDelta5m.OIDeltaPct, "percentage", snapshot.Symbol),
		"candle_intent":         cvdDelta5m.CandleIntent,
		"volume_delta":          FormatByDataTypeAndSymbol(cvdDelta5m.VolumeDelta, "volume", snapshot.Symbol),
		"volume_ratio":          FormatByDataTypeAndSymbol(cvdDelta5m.VolumeRatio, "ratio", snapshot.Symbol),
		"period_minutes":        5,
		"data_quality":          FormatByDataTypeAndSymbol(cvdDelta5m.DataQuality, "ratio", snapshot.Symbol),
		"period_start":          cvdDelta5m.PeriodStartTime.Format("15:04:05"),
		"period_end":            cvdDelta5m.PeriodEndTime.Format("15:04:05"),
		"analysis_time":         time.Now().Format("15:04:05"),
	}
}

// buildMacroTrendData 构建宏观趋势数据（保留1小时级别数据）
func buildMacroTrendData(snapshot *microstructure.MarketSnapshot) map[string]interface{} {
	return map[string]interface{}{
		"spot_cvd_1h_usd":    FormatByDataTypeAndSymbol(snapshot.CVDData.SpotCVD1H, "volume", snapshot.Symbol),
		"futures_cvd_1h_usd": FormatByDataTypeAndSymbol(snapshot.CVDData.FuturesCVD1H, "volume", snapshot.Symbol),
		"oi_change_1h_pct":   FormatByDataTypeAndSymbol(snapshot.OIAnalysis.ChangeRate1H, "percentage", snapshot.Symbol),
		"cvd_divergence":     snapshot.MarketContext.CVDDivergence,
		"context_inference":  snapshot.MarketContext.ContextInference,
		"signal_strength":    FormatByDataTypeAndSymbol(snapshot.MarketContext.SignalStrength, "strength", snapshot.Symbol),
		"market_regime":      snapshot.MarketContext.GameMatrix.MatrixType,
		"dominant_direction": snapshot.CVDData.Signal,
		"trend_alignment":    calculateTrendAlignment(snapshot),
		"confidence_level":   FormatByDataTypeAndSymbol(snapshot.MarketContext.SignalStrength/100.0, "confidence", snapshot.Symbol),
	}
}

// buildEnhancedOrderBookData 构建增强版盘口数据（V2.0）
func buildEnhancedOrderBookData(orderBookData *microstructure.OrderBookData) map[string]interface{} {
	if orderBookData == nil {
		return map[string]interface{}{
			"imbalance_ratio":      0,
			"imbalance_trend":      "insufficient_data",
			"pressure_delta_5m":    0,
			"spoofing_risk":        0,
			"liquidity_score":      0,
			"wall_change_count_5m": 0,
			"resistance_wall":      nil,
			"support_wall":         nil,
		}
	}

	result := map[string]interface{}{
		"imbalance_ratio":      FormatByDataTypeAndSymbol(orderBookData.ImbalanceRatio, "ratio", ""),
		"imbalance_trend":      orderBookData.ImbalanceTrend,
		"pressure_delta_5m":    FormatByDataTypeAndSymbol(orderBookData.PressureDelta5m, "ratio", ""),
		"spoofing_risk":        FormatByDataTypeAndSymbol(orderBookData.SpoofingRisk, "ratio", ""),
		"liquidity_score":      FormatByDataTypeAndSymbol(orderBookData.LiquidityScore, "ratio", ""),
		"wall_change_count_5m": orderBookData.WallChangeCount5m,
		"bid_pressure":         FormatByDataTypeAndSymbol(orderBookData.BidPressure, "volume", ""),
		"ask_pressure":         FormatByDataTypeAndSymbol(orderBookData.AskPressure, "volume", ""),
	}

	// 增强版阻力墙信息（包含稳定性评分）
	if orderBookData.NearestResistance != nil {
		result["resistance_wall"] = map[string]interface{}{
			"price":             FormatByDataTypeAndSymbol(orderBookData.NearestResistance.Price, "price", ""),
			"strength_usd":      FormatByDataTypeAndSymbol(orderBookData.NearestResistance.StrengthUSD, "volume", ""),
			"is_solid":          orderBookData.NearestResistance.IsSolid,
			"distance_pct":      FormatByDataTypeAndSymbol(orderBookData.NearestResistance.Distance, "percentage", ""),
			"stability_score":   FormatByDataTypeAndSymbol(orderBookData.NearestResistance.StabilityScore, "ratio", ""),
			"flicker_count":     orderBookData.NearestResistance.FlickerCount,
			"existence_minutes": int(orderBookData.NearestResistance.ExistenceDuration.Minutes()),
			"level_count":       orderBookData.NearestResistance.LevelCount,
			"max_size":          FormatByDataTypeAndSymbol(orderBookData.NearestResistance.MaxSize, "volume", ""),
			"avg_size":          FormatByDataTypeAndSymbol(orderBookData.NearestResistance.AverageSize, "volume", ""),
		}
	}

	// 增强版支撑墙信息
	if orderBookData.NearestSupport != nil {
		result["support_wall"] = map[string]interface{}{
			"price":             FormatByDataTypeAndSymbol(orderBookData.NearestSupport.Price, "price", ""),
			"strength_usd":      FormatByDataTypeAndSymbol(orderBookData.NearestSupport.StrengthUSD, "volume", ""),
			"is_solid":          orderBookData.NearestSupport.IsSolid,
			"distance_pct":      FormatByDataTypeAndSymbol(orderBookData.NearestSupport.Distance, "percentage", ""),
			"stability_score":   FormatByDataTypeAndSymbol(orderBookData.NearestSupport.StabilityScore, "ratio", ""),
			"flicker_count":     orderBookData.NearestSupport.FlickerCount,
			"existence_minutes": int(orderBookData.NearestSupport.ExistenceDuration.Minutes()),
			"level_count":       orderBookData.NearestSupport.LevelCount,
			"max_size":          FormatByDataTypeAndSymbol(orderBookData.NearestSupport.MaxSize, "volume", ""),
			"avg_size":          FormatByDataTypeAndSymbol(orderBookData.NearestSupport.AverageSize, "volume", ""),
		}
	}

	return result
}

// buildDataQualityInfo 构建数据质量信息（V2.0）
func buildDataQualityInfo(snapshot *microstructure.MarketSnapshot) map[string]interface{} {
	// 从缓存获取5分钟增量数据
	globalCache := microstructure.GetGlobalCache()
	cvdDelta5m := globalCache.GetCVDDelta5m(snapshot.Symbol)
	// 计算整体质量评分
	overallScore := 1.0

	// CVD数据质量
	cvdScore := 1.0
	if snapshot.CVDData.IsStale {
		cvdScore = 0.3
	} else if time.Since(snapshot.CVDData.LastUpdate) > 7*time.Minute {
		cvdScore = 0.7
	}

	// 盘口数据质量
	orderBookScore := 1.0
	if snapshot.OrderBookData.IsStale {
		orderBookScore = 0.3
	} else if time.Since(snapshot.OrderBookData.LastUpdate) > 6*time.Minute {
		orderBookScore = 0.8
	}

	// OI数据质量
	oiScore := 1.0
	if snapshot.OIAnalysis.IsStale {
		oiScore = 0.5
	}

	// 5分钟增量数据质量
	incrementalScore := 0.5
	if cvdDelta5m != nil {
		incrementalScore = cvdDelta5m.DataQuality
	}

	// 🔥 P1-1修复：使用质量评分标准化器确保所有评分使用0-1量纲
	normalizer := GetGlobalQualityNormalizer()
	qualityBundle := normalizer.CreateQualityBundle(
		0,                // overallScore将在下面计算
		incrementalScore, // 数据质量
		orderBookScore,   // 流动性评分
		0,                // 稳定性评分（未提供）
		0,                // 通道质量（未提供）
		0,                // 锚点评分（未提供）
	)

	// 加权平均 - 使用标准化后的评分
	normalizedCvdScore := normalizer.NormalizeDataQualityScore(cvdScore, "cvd_score").GetNormalizedScore()
	normalizedOrderBookScore := qualityBundle.LiquidityScore.GetNormalizedScore()
	normalizedOiScore := normalizer.NormalizeDataQualityScore(oiScore, "oi_score").GetNormalizedScore()
	normalizedIncrementalScore := qualityBundle.DataQuality.GetNormalizedScore()

	overallScore = (normalizedCvdScore*0.3 + normalizedOrderBookScore*0.3 + normalizedOiScore*0.2 + normalizedIncrementalScore*0.2)

	// 确定状态
	status := "正常"
	if overallScore < 0.5 {
		status = "异常"
	} else if overallScore < 0.8 {
		status = "延迟"
	}

	return map[string]interface{}{
		"cvd_reliability":       FormatByDataTypeAndSymbol(normalizedCvdScore, "ratio", ""),
		"orderbook_reliability": FormatByDataTypeAndSymbol(normalizedOrderBookScore, "ratio", ""),
		"oi_reliability":        FormatByDataTypeAndSymbol(normalizedOiScore, "ratio", ""),
		"incremental_quality":   FormatByDataTypeAndSymbol(normalizedIncrementalScore, "ratio", ""),
		"overall_score":         FormatByDataTypeAndSymbol(overallScore, "ratio", ""),
		"last_update":           snapshot.Timestamp.Format("15:04:05"),
		"data_lag_ms":           time.Since(snapshot.Timestamp).Milliseconds(),
		"status":                status,
		"update_interval_s":     5 * 60, // 5分钟更新间隔
	}
}

// calculateTrendAlignment 计算趋势一致性
func calculateTrendAlignment(snapshot *microstructure.MarketSnapshot) string {
	spotCVD := snapshot.CVDData.SpotCVD1H
	futuresCVD := snapshot.CVDData.FuturesCVD1H

	// 判断趋势一致性
	if spotCVD > 0 && futuresCVD > 0 {
		return "bullish_aligned"
	} else if spotCVD < 0 && futuresCVD < 0 {
		return "bearish_aligned"
	} else if math.Abs(spotCVD) < 100000 && math.Abs(futuresCVD) < 100000 {
		return "neutral_consolidation"
	} else {
		return "divergent_mixed"
	}
}

// ===== V2.0 Enhanced Order Flow Functions =====

// buildEmptyCurrentPeriodDataV2 构建空的当前周期数据结构（V2.0完整版）
func buildEmptyCurrentPeriodDataV2() map[string]interface{} {
	return map[string]interface{}{
		"price_delta_pct":       0.0,
		"spot_cvd_delta_usd":    0,
		"futures_cvd_delta_usd": 0,
		"oi_delta_pct":          0.0,
		"candle_intent":         "data_insufficient",
		"volume_delta":          0,
		"volume_ratio":          1.0,
		"period_minutes":        5,
		"data_quality":          0.0,
		"period_start":          time.Now().Add(-5 * time.Minute).Format("15:04:05"),
		"period_end":            time.Now().Format("15:04:05"),
		"analysis_time":         time.Now().Format("15:04:05"),
	}
}

// buildEmptyMacroTrendDataV2 构建空的宏观趋势数据结构（V2.0完整版）
func buildEmptyMacroTrendDataV2() map[string]interface{} {
	return map[string]interface{}{
		"spot_cvd_1h_usd":    0,
		"futures_cvd_1h_usd": 0,
		"oi_change_1h_pct":   0.0,
		"cvd_divergence":     "unknown",
		"context_inference":  "insufficient_data",
		"signal_strength":    0.0,
		"market_regime":      "unknown",
		"dominant_direction": "unknown",
		"trend_alignment":    "unknown",
		"confidence_level":   0.0,
	}
}

// buildEmptyOrderBookDataV2 构建空的盘口结构数据（V2.0完整版）
func buildEmptyOrderBookDataV2() map[string]interface{} {
	return map[string]interface{}{
		"imbalance_ratio":      0.0,
		"imbalance_trend":      "insufficient_data",
		"pressure_delta_5m":    0.0,
		"spoofing_risk":        0.0,
		"liquidity_score":      0.0,
		"wall_change_count_5m": 0,
		"bid_pressure":         0.0,
		"ask_pressure":         0.0,
		"resistance_wall":      nil,
		"support_wall":         nil,
	}
}

// buildEmptyDataQualityInfoV2 构建空的数据质量信息（V2.0完整版）
func buildEmptyDataQualityInfoV2() map[string]interface{} {
	return map[string]interface{}{
		"cvd_reliability":       0.0,
		"orderbook_reliability": 0.0,
		"oi_reliability":        0.0,
		"incremental_quality":   0.0,
		"overall_score":         0.0,
		"last_update":           time.Now().Format("15:04:05"),
		"data_lag_ms":           0,
		"status":                "insufficient_data",
		"update_interval_s":     300,
	}
}

// buildCurrentPeriodDataV2 构建增强版当前周期数据（V2.0）
func buildCurrentPeriodDataV2(snapshot *microstructure.MarketSnapshot, isStale bool) map[string]interface{} {
	if isStale {
		data := buildEmptyCurrentPeriodDataV2()
		data["candle_intent"] = "data_insufficient"
		data["data_quality"] = 0.1
		return data
	}

	// 从缓存获取5分钟增量数据
	globalCache := microstructure.GetGlobalCache()
	cvdDelta5m := globalCache.GetCVDDelta5m(snapshot.Symbol)

	if cvdDelta5m == nil {
		return buildEmptyCurrentPeriodDataV2()
	}

	return map[string]interface{}{
		"price_delta_pct":       FormatByDataTypeAndSymbol(cvdDelta5m.PriceDeltaPct, "percentage", snapshot.Symbol),
		"spot_cvd_delta_usd":    FormatByDataTypeAndSymbol(cvdDelta5m.SpotCVDDeltaUSD, "volume", snapshot.Symbol),
		"futures_cvd_delta_usd": FormatByDataTypeAndSymbol(cvdDelta5m.FuturesCVDDeltaUSD, "volume", snapshot.Symbol),
		"oi_delta_pct":          FormatByDataTypeAndSymbol(cvdDelta5m.OIDeltaPct, "percentage", snapshot.Symbol),
		"candle_intent":         cvdDelta5m.CandleIntent,
		"volume_delta":          FormatByDataTypeAndSymbol(cvdDelta5m.VolumeDelta, "volume", snapshot.Symbol),
		"volume_ratio":          FormatByDataTypeAndSymbol(cvdDelta5m.VolumeRatio, "ratio", snapshot.Symbol),
		"period_minutes":        5,
		"data_quality":          FormatByDataTypeAndSymbol(cvdDelta5m.DataQuality, "ratio", snapshot.Symbol),
		"period_start":          cvdDelta5m.PeriodStartTime.Format("15:04:05"),
		"period_end":            cvdDelta5m.PeriodEndTime.Format("15:04:05"),
		"analysis_time":         time.Now().Format("15:04:05"),
	}
}

// buildMacroTrendDataV2 构建增强版宏观趋势数据（V2.0）
func buildMacroTrendDataV2(snapshot *microstructure.MarketSnapshot, isStale bool) map[string]interface{} {
	if isStale {
		data := buildEmptyMacroTrendDataV2()
		data["signal_strength"] = 0.1
		data["confidence_level"] = 0.1
		return data
	}

	return map[string]interface{}{
		"spot_cvd_1h_usd":    FormatByDataTypeAndSymbol(snapshot.CVDData.SpotCVD1H, "volume", snapshot.Symbol),
		"futures_cvd_1h_usd": FormatByDataTypeAndSymbol(snapshot.CVDData.FuturesCVD1H, "volume", snapshot.Symbol),
		"oi_change_1h_pct":   FormatByDataTypeAndSymbol(snapshot.OIAnalysis.ChangeRate1H, "percentage", snapshot.Symbol),
		"cvd_divergence":     snapshot.MarketContext.CVDDivergence,
		"context_inference":  snapshot.MarketContext.ContextInference,
		"signal_strength":    FormatByDataTypeAndSymbol(snapshot.MarketContext.SignalStrength, "strength", snapshot.Symbol),
		"market_regime":      snapshot.MarketContext.GameMatrix.MatrixType,
		"dominant_direction": snapshot.CVDData.Signal,
		"trend_alignment":    calculateTrendAlignment(snapshot),
		"confidence_level":   FormatByDataTypeAndSymbol(snapshot.MarketContext.SignalStrength/100.0, "confidence", snapshot.Symbol),
	}
}

// buildEnhancedOrderBookDataV2 构建增强版盘口数据（V2.0）
func buildEnhancedOrderBookDataV2(orderBookData *microstructure.OrderBookData, isStale bool) map[string]interface{} {
	if isStale || orderBookData == nil {
		return buildEmptyOrderBookDataV2()
	}

	result := map[string]interface{}{
		"imbalance_ratio":      FormatByDataTypeAndSymbol(orderBookData.ImbalanceRatio, "ratio", ""),
		"imbalance_trend":      orderBookData.ImbalanceTrend,
		"pressure_delta_5m":    FormatByDataTypeAndSymbol(orderBookData.PressureDelta5m, "ratio", ""),
		"spoofing_risk":        FormatByDataTypeAndSymbol(orderBookData.SpoofingRisk, "ratio", ""),
		"liquidity_score":      FormatByDataTypeAndSymbol(orderBookData.LiquidityScore, "ratio", ""),
		"wall_change_count_5m": orderBookData.WallChangeCount5m,
		"bid_pressure":         FormatByDataTypeAndSymbol(orderBookData.BidPressure, "volume", ""),
		"ask_pressure":         FormatByDataTypeAndSymbol(orderBookData.AskPressure, "volume", ""),
	}

	// 增强版阻力墙信息（包含V2.0新字段）
	if orderBookData.NearestResistance != nil {
		result["resistance_wall"] = map[string]interface{}{
			"price":             FormatByDataTypeAndSymbol(orderBookData.NearestResistance.Price, "price", ""),
			"strength_usd":      FormatByDataTypeAndSymbol(orderBookData.NearestResistance.StrengthUSD, "volume", ""),
			"is_solid":          orderBookData.NearestResistance.IsSolid,
			"distance_pct":      FormatByDataTypeAndSymbol(orderBookData.NearestResistance.Distance, "percentage", ""),
			"stability_score":   FormatByDataTypeAndSymbol(orderBookData.NearestResistance.StabilityScore, "ratio", ""),
			"flicker_count":     orderBookData.NearestResistance.FlickerCount,
			"existence_minutes": int(orderBookData.NearestResistance.ExistenceDuration.Minutes()),
			"level_count":       orderBookData.NearestResistance.LevelCount,
			"max_size":          FormatByDataTypeAndSymbol(orderBookData.NearestResistance.MaxSize, "volume", ""),
			"avg_size":          FormatByDataTypeAndSymbol(orderBookData.NearestResistance.AverageSize, "volume", ""),
		}
	}

	// 增强版支撑墙信息
	if orderBookData.NearestSupport != nil {
		result["support_wall"] = map[string]interface{}{
			"price":             FormatByDataTypeAndSymbol(orderBookData.NearestSupport.Price, "price", ""),
			"strength_usd":      FormatByDataTypeAndSymbol(orderBookData.NearestSupport.StrengthUSD, "volume", ""),
			"is_solid":          orderBookData.NearestSupport.IsSolid,
			"distance_pct":      FormatByDataTypeAndSymbol(orderBookData.NearestSupport.Distance, "percentage", ""),
			"stability_score":   FormatByDataTypeAndSymbol(orderBookData.NearestSupport.StabilityScore, "ratio", ""),
			"flicker_count":     orderBookData.NearestSupport.FlickerCount,
			"existence_minutes": int(orderBookData.NearestSupport.ExistenceDuration.Minutes()),
			"level_count":       orderBookData.NearestSupport.LevelCount,
			"max_size":          FormatByDataTypeAndSymbol(orderBookData.NearestSupport.MaxSize, "volume", ""),
			"avg_size":          FormatByDataTypeAndSymbol(orderBookData.NearestSupport.AverageSize, "volume", ""),
		}
	}

	return result
}

// buildDataQualityInfoV2 构建增强版数据质量信息（V2.0）
func buildDataQualityInfoV2(snapshot *microstructure.MarketSnapshot, isStale bool) map[string]interface{} {
	// 从缓存获取5分钟增量数据
	globalCache := microstructure.GetGlobalCache()
	cvdDelta5m := globalCache.GetCVDDelta5m(snapshot.Symbol)

	// 计算整体质量评分
	overallScore := 1.0

	// CVD数据质量
	cvdScore := 1.0
	if isStale || snapshot.CVDData.IsStale {
		cvdScore = 0.3
	} else if time.Since(snapshot.CVDData.LastUpdate) > 7*time.Minute {
		cvdScore = 0.7
	}

	// 盘口数据质量
	orderBookScore := 1.0
	if isStale || snapshot.OrderBookData.IsStale {
		orderBookScore = 0.3
	} else if time.Since(snapshot.OrderBookData.LastUpdate) > 6*time.Minute {
		orderBookScore = 0.8
	}

	// OI数据质量
	oiScore := 1.0
	if isStale || snapshot.OIAnalysis.IsStale {
		oiScore = 0.5
	}

	// 5分钟增量数据质量
	incrementalScore := 0.5
	if cvdDelta5m != nil {
		incrementalScore = cvdDelta5m.DataQuality
	}

	// 加权平均
	overallScore = (cvdScore*0.3 + orderBookScore*0.3 + oiScore*0.2 + incrementalScore*0.2)

	// 确定状态
	status := "正常"
	if overallScore < 0.5 {
		status = "异常"
	} else if overallScore < 0.8 {
		status = "延迟"
	}

	return map[string]interface{}{
		"cvd_reliability":       FormatByDataTypeAndSymbol(cvdScore, "ratio", ""),
		"orderbook_reliability": FormatByDataTypeAndSymbol(orderBookScore, "ratio", ""),
		"oi_reliability":        FormatByDataTypeAndSymbol(oiScore, "ratio", ""),
		"incremental_quality":   FormatByDataTypeAndSymbol(incrementalScore, "ratio", ""),
		"overall_score":         FormatByDataTypeAndSymbol(overallScore, "ratio", ""),
		"last_update":           snapshot.Timestamp.Format("15:04:05"),
		"data_lag_ms":           time.Since(snapshot.Timestamp).Milliseconds(),
		"status":                status,
		"update_interval_s":     300, // 5分钟更新间隔
	}
}

// ===== P0-01修复：时间锚点统一处理函数 =====

// filterKlinesByAnchorTime 根据时间锚点过滤K线数据
// 🔥 P0-01修复核心函数：确保所有K线数据都在锚点时间之前已收盘
func filterKlinesByAnchorTime(klines []Kline, anchorCloseTimeMs int64) []Kline {
	if len(klines) == 0 {
		return klines
	}

	// 找到最大的满足 CloseTime <= anchorCloseTimeMs 的索引
	maxValidIndex := -1
	for i := len(klines) - 1; i >= 0; i-- {
		if klines[i].CloseTime <= anchorCloseTimeMs {
			maxValidIndex = i
			break
		}
	}

	if maxValidIndex < 0 {
		log.Printf("⚠️ [时间锚点] 所有K线都在锚点时间 %d 之后，返回空切片", anchorCloseTimeMs)
		return []Kline{}
	}

	// 返回裁剪后的K线数据 (0到maxValidIndex，包含maxValidIndex)
	return klines[:maxValidIndex+1]
}

// extract5mOHLCDataFromFiltered 从经过锚点裁剪的5m K线中提取OHLC数据
// 🔥 P0-01修复：基于锚点裁剪后的数据提取，确保时间一致性
func extract5mOHLCDataFromFiltered(filteredKlines []Kline) (*OHLCData, *OHLCData) {
	if len(filteredKlines) == 0 {
		log.Printf("⚠️ [5m OHLC] 经锚点裁剪的K线数据为空")
		return nil, nil
	}

	// 最后一根K线 (锚点时间内的最后已收盘K线)
	lastIdx := len(filteredKlines) - 1
	lastClosed := extractOHLCData(filteredKlines[lastIdx])

	// 上一根K线 (用于对比分析)
	var prevClosed *OHLCData
	if lastIdx >= 1 {
		prevClosed = extractOHLCData(filteredKlines[lastIdx-1])
	}

	return lastClosed, prevClosed
}

// extract4hOHLCDataFromFiltered 从经过锚点裁剪的4h K线中提取OHLC数据
// 🔥 P0-02修复：对齐5m逻辑，返回最新已收盘和上一根已收盘4h，修复HTF滞后问题
func extract4hOHLCDataFromFiltered(filteredKlines []Kline) (*OHLCData, *OHLCData) {
	if len(filteredKlines) < 1 {
		log.Printf("⚠️ [4h OHLC] 经锚点裁剪的K线数据不足")
		return nil, nil
	}

	// 最后一根K线 (最新已收盘K线) - 修复off-by-one错误
	lastIdx := len(filteredKlines) - 1
	lastClosed := extractOHLCData(filteredKlines[lastIdx])

	// 上一根K线 (前一根已收盘K线) - 用于对比分析
	var prevClosed *OHLCData
	if len(filteredKlines) >= 2 {
		prevIdx := len(filteredKlines) - 2
		prevClosed = extractOHLCData(filteredKlines[prevIdx])
	}

	return lastClosed, prevClosed
}

// ===== Gate2结构聚合AI输出函数 =====

// buildGate2CompactOutput 构建Gate2结构聚合的AI友好输出
// 🔥 功能：将Gate2聚合结果格式化为AI易于理解的结构化输出
func buildGate2CompactOutput(data *Data) map[string]interface{} {
	if data.StructureGate2 == nil {
		return map[string]interface{}{
			"status":                 "disabled",
			"struct_state_long":      "UNKNOWN",
			"struct_state_short":     "UNKNOWN",
			"best_anchor_long":       nil,
			"best_anchor_short":      nil,
			"top_anchors_long":       []interface{}{},
			"top_anchors_short":      []interface{}{},
			"anchor_score_breakdown": map[string]interface{}{},
			"trigger_context": map[string]interface{}{
				"is_triggered": false,
				"trigger_time": time.Now().Format("15:04:05"),
				"time_anchor":  "5m_close_aligned",
			},
		}
	}

	result := map[string]interface{}{
		"status": "active",

		// 🔥 V-13.5统一价格字段 (避免双契约)
		"last_price": FormatByDataTypeAndSymbol(data.LastPrice, "price", data.Symbol),

		// 🔥 结构状态输出
		"struct_state_long":  data.StructureGate2.StructStateLong,
		"struct_state_short": data.StructureGate2.StructStateShort,

		// 🔥 最优锚点输出
		"best_anchor_long":  buildBestAnchorOutput(data.StructureGate2.TopAnchorsLong, data.Symbol),
		"best_anchor_short": buildBestAnchorOutput(data.StructureGate2.TopAnchorsShort, data.Symbol),

		// 🔥 前3-5个锚点输出
		"top_anchors_long":  buildTopAnchorsOutput(data.StructureGate2.TopAnchorsLong, data.Symbol, 3),
		"top_anchors_short": buildTopAnchorsOutput(data.StructureGate2.TopAnchorsShort, data.Symbol, 3),

		// 🔥 锚点评分breakdown
		"anchor_score_breakdown": buildAnchorScoreBreakdown(data.StructureGate2),

		// 🔥 触发上下文
		"trigger_context": map[string]interface{}{
			"is_triggered": data.StructureGate2.TriggerResult != nil && data.StructureGate2.TriggerResult.IsTriggered,
			"trigger_time": time.Now().Format("15:04:05"),
			"time_anchor":  "5m_close_aligned",
			"trigger_details": func() interface{} {
				if data.StructureGate2.TriggerResult != nil && data.StructureGate2.TriggerResult.IsTriggered {
					return map[string]interface{}{
						"direction":    data.StructureGate2.TriggerResult.Direction,
						"trigger_type": data.StructureGate2.TriggerResult.TriggerType,
						"confidence":   FormatByDataTypeAndSymbol(data.StructureGate2.TriggerResult.Confidence, "confidence", data.Symbol),
						"anchor_triggered": func() interface{} {
							if data.StructureGate2.TriggerResult.TriggeredAnchor != nil {
								return buildSingleAnchorOutput(*data.StructureGate2.TriggerResult.TriggeredAnchor, data.Symbol)
							}
							return nil
						}(),
					}
				}
				return nil
			}(),
		},
	}

	// 🔥 P1-2修复：Gate2输出JSON schema验证
	validationResult := ValidateAIOutput(result, "gate2")
	if validationResult.ProcessedData != nil && len(validationResult.CorrectedFields) > 0 {
		log.Printf("🔧 [P1-2] Gate2输出自动纠正了 %d 个字段", len(validationResult.CorrectedFields))
		if correctedResult, ok := validationResult.ProcessedData.(map[string]interface{}); ok {
			return correctedResult
		}
	}

	return result
}

// buildBestAnchorOutput 构建最优锚点输出
func buildBestAnchorOutput(anchors []AnchorCandidate, symbol string) interface{} {
	if len(anchors) == 0 {
		return nil
	}

	// 返回排序后的第一个（最优）锚点
	bestAnchor := anchors[0]
	return buildSingleAnchorOutput(bestAnchor, symbol)
}

// buildTopAnchorsOutput 构建前N个锚点输出
func buildTopAnchorsOutput(anchors []AnchorCandidate, symbol string, maxCount int) []interface{} {
	result := make([]interface{}, 0, maxCount)

	count := len(anchors)
	if count > maxCount {
		count = maxCount
	}

	for i := 0; i < count; i++ {
		anchorOutput := buildSingleAnchorOutput(anchors[i], symbol)
		result = append(result, anchorOutput)
	}

	return result
}

// buildSingleAnchorOutput 构建单个锚点输出
func buildSingleAnchorOutput(anchor AnchorCandidate, symbol string) map[string]interface{} {
	// 计算ATR归一化距离（如果需要的话，从Meta中获取或计算）
	var distanceATR float64
	if meta := anchor.Meta; meta != nil {
		if val, exists := meta["distance_atr"]; exists {
			if f, ok := val.(float64); ok {
				distanceATR = f
			}
		}
	}

	return map[string]interface{}{
		"type":          anchor.Type,
		"timeframe":     anchor.TF,
		"level":         FormatByDataTypeAndSymbol(anchor.Level, "price", symbol),
		"direction":     anchor.Dir,
		"priority_rank": anchor.PriorityRank,
		"anchor_score":  FormatByDataTypeAndSymbol(anchor.AnchorScore, "ratio", symbol),
		"distance_atr":  FormatByDataTypeAndSymbol(distanceATR, "ratio", symbol),
		"strength_z": func() interface{} {
			if anchor.StrengthZ != nil {
				return FormatByDataTypeAndSymbol(*anchor.StrengthZ, "ratio", symbol)
			}
			return nil
		}(),
		"vol_ratio": func() interface{} {
			if anchor.VolRatio != nil {
				return FormatByDataTypeAndSymbol(*anchor.VolRatio, "ratio", symbol)
			}
			return nil
		}(),
		"is_fresh": anchor.IsFresh,
		"score_breakdown": func() interface{} {
			if meta := anchor.Meta; meta != nil {
				if breakdown, exists := meta["score_breakdown"]; exists {
					return breakdown
				}
			}
			return nil
		}(),
	}
}

// buildAnchorScoreBreakdown 构建锚点评分breakdown
func buildAnchorScoreBreakdown(gate2 *StructureGate2) map[string]interface{} {
	result := map[string]interface{}{
		"long_anchors_count":  len(gate2.TopAnchorsLong),
		"short_anchors_count": len(gate2.TopAnchorsShort),
		"total_anchors":       len(gate2.TopAnchorsLong) + len(gate2.TopAnchorsShort),
	}

	// 添加最优锚点的详细breakdown（从Meta中获取）
	if len(gate2.TopAnchorsLong) > 0 {
		if meta := gate2.TopAnchorsLong[0].Meta; meta != nil {
			if breakdown, exists := meta["score_breakdown"]; exists {
				result["best_long_breakdown"] = breakdown
			}
		}
	}

	if len(gate2.TopAnchorsShort) > 0 {
		if meta := gate2.TopAnchorsShort[0].Meta; meta != nil {
			if breakdown, exists := meta["score_breakdown"]; exists {
				result["best_short_breakdown"] = breakdown
			}
		}
	}

	// 添加优先级分布统计
	priorityStats := make(map[string]int)
	allAnchors := append(gate2.TopAnchorsLong, gate2.TopAnchorsShort...)

	for _, anchor := range allAnchors {
		priorityKey := fmt.Sprintf("P%d", anchor.PriorityRank)
		priorityStats[priorityKey]++
	}

	result["priority_distribution"] = priorityStats

	return result
}
