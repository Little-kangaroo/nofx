package market

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Get 获取指定代币的市场数据
func Get(symbol string) (*Data, error) {
	// 技术指标计算总体耗时统计
	totalStart := time.Now()

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

	// K线数据获取阶段耗时统计
	klinesFetchDuration := time.Since(klinesFetchStart)
	log.Printf("📊 [%s-K线获取] 耗时: %v (5m+15m+30m+1h+4h)", symbol, klinesFetchDuration)

	// 基础技术指标计算阶段耗时统计
	basicIndicatorsStart := time.Now()
	// 计算当前指标 (基于5分钟最新数据)
	currentPrice := klines5m[len(klines5m)-1].Close
	currentEMA20 := calculateEMA(klines5m, 20)
	currentMACD := calculateMACD(klines5m)
	currentRSI7 := calculateRSI(klines5m, 7)

	// 基础指标计算耗时统计
	basicIndicatorsDuration := time.Since(basicIndicatorsStart)
	log.Printf("📊 [%s-基础指标] 耗时: %v (Price+EMA20+MACD+RSI7)", symbol, basicIndicatorsDuration)

	// 计算价格变化百分比
	// 1小时价格变化 = 12个5分钟K线前的价格
	priceChange1h := 0.0
	if len(klines5m) >= 13 { // 至少需要13根K线 (当前 + 12根前)
		price1hAgo := klines5m[len(klines5m)-13].Close
		if price1hAgo > 0 {
			priceChange1h = ((currentPrice - price1hAgo) / price1hAgo) * 100
		}
	}

	// 4小时价格变化 = 1个4小时K线前的价格
	priceChange4h := 0.0
	if len(klines4h) >= 2 {
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
	comprehensiveAnalyzer := NewComprehensiveAnalyzer()
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
	
	// 提取5m级别OHLC数据 (last_closed + prev_closed)
	ohlc5mLastClosed, ohlc5mPrevClosed := extract5mOHLCData(klines5m)
	
	// 提取4h级别OHLC数据 (last_close only)  
	ohlc4hLastClose := extract4hOHLCData(klines4h)
	
	ohlcExtractionDuration := time.Since(ohlcExtractionStart)
	log.Printf("📊 [%s-OHLC提取] 耗时: %v (5m+4h级别)", symbol, ohlcExtractionDuration)

	data := &Data{
		Symbol:                 symbol,
		CurrentPrice:           currentPrice,
		PriceChange1h:          priceChange1h,
		PriceChange4h:          priceChange4h,
		CurrentEMA20:           currentEMA20,
		CurrentMACD:            currentMACD,
		CurrentRSI7:            currentRSI7,
		
		// OHLC数据
		OHLC5mLastClosed:       ohlc5mLastClosed,
		OHLC5mPrevClosed:       ohlc5mPrevClosed,
		OHLC4hLastClose:        ohlc4hLastClose,
		
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

// extract5mOHLCData 提取5m级别的OHLC数据 (last_closed + prev_closed)
func extract5mOHLCData(klines5m []Kline) (*OHLCData, *OHLCData) {
	if len(klines5m) < 2 {
		log.Printf("⚠️ [5m OHLC] K线数据不足，无法提取prev_closed")
		if len(klines5m) >= 1 {
			// 只有last_closed
			lastClosed := extractOHLCData(klines5m[len(klines5m)-1])
			return lastClosed, nil
		}
		return nil, nil
	}
	
	// 最新已收盘K线(倒数第1根)
	lastClosed := extractOHLCData(klines5m[len(klines5m)-1])
	// 上一根已收盘K线(倒数第2根)
	prevClosed := extractOHLCData(klines5m[len(klines5m)-2])
	
	return lastClosed, prevClosed
}

// extract4hOHLCData 提取4h级别的OHLC数据 (last_close only)
func extract4hOHLCData(klines4h []Kline) *OHLCData {
	if len(klines4h) < 1 {
		log.Printf("⚠️ [4h OHLC] K线数据不足，无法提取last_close")
		return nil
	}
	
	// 最新已收盘K线
	return extractOHLCData(klines4h[len(klines4h)-1])
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
func FormatAsCompactData(data *Data) string {
	// 重新获取K线数据用于超级趋势计算
	symbol := data.Symbol
	klines5m, _ := WSMonitorCli.GetCurrentKlines(symbol, "5m")
	klines15m, _ := WSMonitorCli.GetCurrentKlines(symbol, "15m")
	klines30m, _ := WSMonitorCli.GetCurrentKlines(symbol, "30m")
	klines1h, _ := WSMonitorCli.GetCurrentKlines(symbol, "1h")
	klines4h, _ := WSMonitorCli.GetCurrentKlines(symbol, "4h")

	timeframeKlines := map[string][]Kline{
		"5m":  klines5m,
		"15m": klines15m,
		"30m": klines30m,
		"1h":  klines1h,
		"4h":  klines4h,
	}

	result := map[string]interface{}{
		data.Symbol: map[string]interface{}{
			"基础指标":    calculateMultiTimeframeBasicIndicators(data, timeframeKlines),
			"多时间框架分析": extractCompactMultiTimeframeAnalysisWithSupertrend(data, timeframeKlines),
		},
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("精简JSON序列化失败: %v", err)
	}

	return string(jsonData)
}

// calculateMultiTimeframeBasicIndicators 计算多时间框架基础指标
func calculateMultiTimeframeBasicIndicators(data *Data, timeframeKlines map[string][]Kline) map[string]interface{} {
	result := make(map[string]interface{})

	// 全局指标（不依赖时间框架）
	result["price"] = FormatByDataTypeAndSymbol(data.CurrentPrice, "price", data.Symbol)      // 保留原有字段（向后兼容）
	result["last_price"] = FormatByDataTypeAndSymbol(data.CurrentPrice, "price", data.Symbol) // 新增字段（更清晰的命名）
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
				result["change_1h"] = FormatByDataTypeAndSymbol(((data.CurrentPrice - price1hAgo) / price1hAgo) * 100, "percentage", data.Symbol)
			}
		}
	}

	// 4小时价格变化（基于4小时K线计算）
	if klines4h, exists := timeframeKlines["4h"]; exists && len(klines4h) >= 2 {
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			result["change_4h"] = FormatByDataTypeAndSymbol(((data.CurrentPrice - price4hAgo) / price4hAgo) * 100, "percentage", data.Symbol)
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
			if data.OHLC5mLastClosed != nil {
				tfData["ohlc_last_closed"] = formatOHLCData(data.OHLC5mLastClosed, data.Symbol)
			}
			if data.OHLC5mPrevClosed != nil {
				tfData["ohlc_prev_closed"] = formatOHLCData(data.OHLC5mPrevClosed, data.Symbol)
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
		case "1h":
			// 1h级别: ohlc_last_close only
			if data.MediumTerm1h != nil && data.MediumTerm1h.OHLCLastClosed != nil {
				tfData["ohlc_last_close"] = formatOHLCData(data.MediumTerm1h.OHLCLastClosed, data.Symbol)
			}
		case "4h":
			// 4h级别: ohlc_last_close only
			if data.OHLC4hLastClose != nil {
				tfData["ohlc_last_close"] = formatOHLCData(data.OHLC4hLastClose, data.Symbol)
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
			"道氏理论数据":  extractCompactDowTheory(tfData.DowTheory, data.Symbol),
			"通道数据":    extractCompactChannelAnalysis(tfData.ChannelAnalysis, data.Symbol),
			"VPVR数据":  extractCompactVPVR(tfData.VolumeProfile, data.Symbol),
			"供需区数据":   extractCompactSupplyDemand(tfData.SupplyDemand, data.Symbol),
			"FVG数据":   extractCompactFVG(tfData.FairValueGaps, data.Symbol),
			"斐波纳契数据":  extractCompactFibonacci(tfData.Fibonacci, data.Symbol),
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
		"channel_width":     FormatByDataTypeAndSymbol(data.Quality * 100, "percentage", symbol),
		"current_position":  data.CurrentPosition,
	}

	if data.ActiveChannel != nil {
		result["channel_width"] = FormatByDataTypeAndSymbol(data.ActiveChannel.Width * 100, "percentage", symbol)
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
		"avg_strength": FormatByDataTypeAndSymbol(strengthSum / float64(len(data.ActiveZones)), "strength", symbol),
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
		"gap_type":    "unknown",
		"gaps":        []map[string]interface{}{},
	}

	if len(data.ActiveFVGs) > 0 {
		// 取第一个活跃的FVG作为最近的
		fvg := data.ActiveFVGs[0]
		result["nearest_gap"] = FormatByDataTypeAndSymbol((fvg.LowerBound + fvg.UpperBound) / 2, "price", symbol)
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
				"id":           gap.ID,
				"type":         gap.Type,
				"upper_bound":  FormatByDataTypeAndSymbol(gap.UpperBound, "price", symbol),
				"lower_bound":  FormatByDataTypeAndSymbol(gap.LowerBound, "price", symbol),
				"center_price": FormatByDataTypeAndSymbol(gap.CenterPrice, "price", symbol),
				"width":        FormatByDataTypeAndSymbol(gap.Width, "price", symbol),
				"width_percent": FormatByDataTypeAndSymbol(gap.WidthPercent, "percentage", symbol),
				"strength":     FormatByDataTypeAndSymbol(gap.Strength, "strength", symbol),
				"quality":      gap.Quality,
				"status":       gap.Status,
				"touch_count":  gap.TouchCount,
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
				"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "volume_profile"),
				"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "supply_demand"),
				"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "fibonacci"),
			},
			"15m": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "dow_theory"),
				"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "volume_profile"),
				"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "supply_demand"),
				"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fibonacci"),
			},
			"30m": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "dow_theory"),
				"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "volume_profile"),
				"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "supply_demand"),
				"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fibonacci"),
			},
			"1h": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "dow_theory"),
				"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "volume_profile"),
				"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "supply_demand"),
				"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fibonacci"),
			},
			"4h": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "dow_theory"),
				"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "volume_profile"),
				"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "supply_demand"),
				"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "fair_value_gaps"),
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
			"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "volume_profile"),
			"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "supply_demand"),
			"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "5m", "fibonacci"),
		},
		"15m": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "dow_theory"),
			"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "volume_profile"),
			"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "supply_demand"),
			"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fibonacci"),
		},
		"30m": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "dow_theory"),
			"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "volume_profile"),
			"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "supply_demand"),
			"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fibonacci"),
		},
		"1h": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "dow_theory"),
			"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "volume_profile"),
			"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "supply_demand"),
			"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fibonacci"),
		},
		"4h": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "dow_theory"),
			"通道数据":   extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "volume_profile"),
			"供需区数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "supply_demand"),
			"FVG数据":  extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "fair_value_gaps"),
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
func calculateMediumTermData(klines []Kline, timeframe string) *MediumTermData {
	if len(klines) == 0 {
		return &MediumTermData{Timeframe: timeframe}
	}

	data := &MediumTermData{
		Timeframe:   timeframe,
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 计算EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)

	// 计算当前指标
	data.CurrentMACD = calculateMACD(klines)
	data.CurrentRSI7 = calculateRSI(klines, 7)
	data.CurrentRSI14 = calculateRSI(klines, 14)

	// 计算ATR
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

	// 根据时间框架提取OHLC数据
	if timeframe == "15m" {
		// 15m级别: 提取last_closed + prev_closed
		if len(klines) >= 2 {
			data.OHLCLastClosed = extractOHLCData(klines[len(klines)-1])
			data.OHLCPrevClosed = extractOHLCData(klines[len(klines)-2])
		} else if len(klines) >= 1 {
			data.OHLCLastClosed = extractOHLCData(klines[len(klines)-1])
		}
	} else if timeframe == "1h" {
		// 1h级别: 仅提取last_close
		if len(klines) >= 1 {
			data.OHLCLastClosed = extractOHLCData(klines[len(klines)-1])
		}
	}

	// 计算MACD和RSI序列（最近10个数据点）
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

// SuperTrendResult 超级趋势计算结果
type SuperTrendResult struct {
	Direction   string  // "bullish" or "bearish"
	CurrentLine float64 // 当前趋势线价格
	UpperLine   float64 // 上轨价格
	LowerLine   float64 // 下轨价格
}

// calculateSupertrend 计算超级趋势线（标准实现）
// calculateSupertrend 计算超级趋势线（修正版：优化ATR计算+修复方向初始化）
func calculateSupertrend(klines []Kline, atrPeriod int, factor float64) SuperTrendResult {
	result := SuperTrendResult{
		Direction:   "unknown",
		CurrentLine: 0.0,
		UpperLine:   0.0,
		LowerLine:   0.0,
	}

	minRequired := atrPeriod + 1
	recommended := atrPeriod * 3 // 建议使用ATR周期的3倍数据以确保稳定性
	if len(klines) < minRequired {
		log.Printf("🚨🔴 [SuperTrend计算] ❌ K线数据不足: 需要%d根，实际%d根 ❌", minRequired, len(klines))
		return result
	}
	if len(klines) < recommended {
		log.Printf("🟡⚠️ [SuperTrend计算] 稳定性警告: 建议%d根，实际%d根 (可能影响趋势稳定性) ⚠️🟡", recommended, len(klines))
	}

	length := len(klines)
	// 1. 预先计算ATR序列 (使用Wilder平滑，符合TradingView标准)
	atrs := make([]float64, length)

	// 计算第一个ATR (SMA)
	sumTR := 0.0
	for i := 1; i <= atrPeriod; i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close
		tr := math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
		sumTR += tr
	}
	atrs[atrPeriod] = sumTR / float64(atrPeriod)

	// 计算后续ATR (RMA)
	for i := atrPeriod + 1; i < length; i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close
		tr := math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
		atrs[i] = (atrs[i-1]*float64(atrPeriod-1) + tr) / float64(atrPeriod)
	}

	// 2. 计算SuperTrend
	supertrendLines := make([]float64, length)
	directions := make([]string, length) // "bullish" 或 "bearish"
	upperBands := make([]float64, length)
	lowerBands := make([]float64, length)

	// 初始化第一个点
	hl2 := (klines[atrPeriod].High + klines[atrPeriod].Low) / 2
	upperBands[atrPeriod] = hl2 + (factor * atrs[atrPeriod])
	lowerBands[atrPeriod] = hl2 - (factor * atrs[atrPeriod])

	// 显式初始化方向：如果收盘价在下轨之上，则看多，否则看空
	if klines[atrPeriod].Close > lowerBands[atrPeriod] {
		directions[atrPeriod] = "bullish"
		supertrendLines[atrPeriod] = lowerBands[atrPeriod]
	} else {
		directions[atrPeriod] = "bearish"
		supertrendLines[atrPeriod] = upperBands[atrPeriod]
	}

	for i := atrPeriod + 1; i < length; i++ {
		// 计算基础带
		hl2 := (klines[i].High + klines[i].Low) / 2
		currATR := atrs[i]

		basicUpper := hl2 + (factor * currATR)
		basicLower := hl2 - (factor * currATR)

		// 核心逻辑：带的平滑处理
		// 上轨：只能下降，除非价格突破了前一根的上轨
		if basicUpper < upperBands[i-1] || klines[i-1].Close > upperBands[i-1] {
			upperBands[i] = basicUpper
		} else {
			upperBands[i] = upperBands[i-1]
		}

		// 下轨：只能上升，除非价格跌破了前一根的下轨
		if basicLower > lowerBands[i-1] || klines[i-1].Close < lowerBands[i-1] {
			lowerBands[i] = basicLower
		} else {
			lowerBands[i] = lowerBands[i-1]
		}

		// 确定方向
		prevDir := directions[i-1]
		currDir := prevDir // 默认延续

		if prevDir == "bullish" {
			if klines[i].Close < lowerBands[i] {
				currDir = "bearish"
			}
		} else { // bearish
			if klines[i].Close > upperBands[i] {
				currDir = "bullish"
			}
		}
		directions[i] = currDir

		// 确定当前趋势线数值
		if currDir == "bullish" {
			supertrendLines[i] = lowerBands[i]
		} else {
			supertrendLines[i] = upperBands[i]
		}
	}

	// 返回最新结果
	lastIdx := length - 1
	result.Direction = directions[lastIdx]
	result.CurrentLine = supertrendLines[lastIdx]
	result.UpperLine = upperBands[lastIdx]
	result.LowerLine = lowerBands[lastIdx]

	return result
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
			"通道数据":    extractCompactChannelAnalysis(tfData.ChannelAnalysis, data.Symbol),
			"VPVR数据":  extractCompactVPVR(tfData.VolumeProfile, data.Symbol),
			"供需区数据":   extractCompactSupplyDemand(tfData.SupplyDemand, data.Symbol),
			"FVG数据":   extractCompactFVG(tfData.FairValueGaps, data.Symbol),
			"斐波纳契数据":  extractCompactFibonacci(tfData.Fibonacci, data.Symbol),
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
