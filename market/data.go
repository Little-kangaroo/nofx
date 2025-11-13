package market

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// Get 获取指定代币的市场数据
func Get(symbol string) (*Data, error) {
	var klines3m, klines15m, klines30m, klines1h, klines4h []Kline
	var err error
	// 标准化symbol
	symbol = Normalize(symbol)
	
	// 获取3分钟K线数据
	klines3m, err = WSMonitorCli.GetCurrentKlines(symbol, "3m")
	if err != nil {
		return nil, fmt.Errorf("获取3分钟K线失败: %v", err)
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

	// 计算当前指标 (基于3分钟最新数据)
	currentPrice := klines3m[len(klines3m)-1].Close
	currentEMA20 := calculateEMA(klines3m, 20)
	currentMACD := calculateMACD(klines3m)
	currentRSI7 := calculateRSI(klines3m, 7)

	// 计算价格变化百分比
	// 1小时价格变化 = 20个3分钟K线前的价格
	priceChange1h := 0.0
	if len(klines3m) >= 21 { // 至少需要21根K线 (当前 + 20根前)
		price1hAgo := klines3m[len(klines3m)-21].Close
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
	intradayData := calculateIntradaySeries(klines3m)

	// 计算长期数据
	longerTermData := calculateLongerTermData(klines4h)

	// 计算其他时间框架的基础指标
	mediumTermData15m := calculateMediumTermData(klines15m, "15m")
	mediumTermData30m := calculateMediumTermData(klines30m, "30m")
	mediumTermData1h := calculateMediumTermData(klines1h, "1h")

	// 多时间框架综合分析（包括道氏理论、VPVR、供需区、FVG、斐波纳契、通道分析）
	comprehensiveAnalyzer := NewComprehensiveAnalyzer()
	comprehensiveResult := comprehensiveAnalyzer.AnalyzeMultiTimeframe(symbol, klines3m, klines15m, klines30m, klines1h, klines4h)

	// 执行多时间框架分析
	multiTimeframeAnalysis := comprehensiveAnalyzer.AnalyzeAllTimeframes(symbol, currentPrice, map[string][]Kline{
		"3m":  klines3m,
		"15m": klines15m,
		"30m": klines30m,
		"1h":  klines1h,
		"4h":  klines4h,
	})

	return &Data{
		Symbol:                  symbol,
		CurrentPrice:            currentPrice,
		PriceChange1h:           priceChange1h,
		PriceChange4h:           priceChange4h,
		CurrentEMA20:            currentEMA20,
		CurrentMACD:             currentMACD,
		CurrentRSI7:             currentRSI7,
		OpenInterest:            oiData,
		FundingRate:             fundingRate,
		IntradaySeries:          intradayData,
		LongerTermContext:       longerTermData,
		MediumTerm15m:           mediumTermData15m,
		MediumTerm30m:           mediumTermData30m,
		MediumTerm1h:            mediumTermData1h,
		MultiTimeframeAnalysis:  multiTimeframeAnalysis,
		// 向前兼容的单一分析结果（基于4小时）
		DowTheory:               comprehensiveResult.DowTheory,
		ChannelAnalysis:         comprehensiveResult.ChannelAnalysis,
		VolumeProfile:           comprehensiveResult.VolumeProfile,
		SupplyDemand:            comprehensiveResult.SupplyDemand,
		FairValueGaps:           comprehensiveResult.FairValueGaps,
		Fibonacci:               comprehensiveResult.Fibonacci,
	}, nil
}

// calculateEMA 计算EMA
func calculateEMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
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
	if len(klines) < 26 {
		return 0
	}

	// 计算12期和26期EMA
	ema12 := calculateEMA(klines, 12)
	ema26 := calculateEMA(klines, 26)

	// MACD = EMA12 - EMA26
	return ema12 - ema26
}

// calculateRSI 计算RSI
func calculateRSI(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
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
	if len(klines) <= period {
		return 0
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
				"current_price":    data.CurrentPrice,
				"current_ema20":    data.CurrentEMA20,
				"current_macd":     data.CurrentMACD,
				"current_rsi7":     data.CurrentRSI7,
				"price_change_1h":  data.PriceChange1h,
				"price_change_4h":  data.PriceChange4h,
				"open_interest":    data.OpenInterest,
				"funding_rate":     data.FundingRate,
				"intraday_series":  data.IntradaySeries,
				"longer_term_context": data.LongerTermContext,
				"medium_term_15m":  data.MediumTerm15m,
				"medium_term_30m":  data.MediumTerm30m,
				"medium_term_1h":   data.MediumTerm1h,
			},
			// 多时间框架技术分析
			"多时间框架分析": symbolData,
		},
	}
	
	// 序列化为JSON
	jsonData, err := json.MarshalIndent(result, "", "  ")
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
	klines3m, _ := WSMonitorCli.GetCurrentKlines(symbol, "3m")
	klines15m, _ := WSMonitorCli.GetCurrentKlines(symbol, "15m") 
	klines30m, _ := WSMonitorCli.GetCurrentKlines(symbol, "30m")
	klines1h, _ := WSMonitorCli.GetCurrentKlines(symbol, "1h")
	klines4h, _ := WSMonitorCli.GetCurrentKlines(symbol, "4h")
	
	timeframeKlines := map[string][]Kline{
		"3m":  klines3m,
		"15m": klines15m,
		"30m": klines30m,
		"1h":  klines1h,
		"4h":  klines4h,
	}
	
	result := map[string]interface{}{
		data.Symbol: map[string]interface{}{
			"基础指标": calculateMultiTimeframeBasicIndicators(data, timeframeKlines),
			"多时间框架分析": extractCompactMultiTimeframeAnalysisWithSupertrend(data, timeframeKlines),
		},
	}
	
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("精简JSON序列化失败: %v", err)
	}
	
	return string(jsonData)
}

// calculateMultiTimeframeBasicIndicators 计算多时间框架基础指标
func calculateMultiTimeframeBasicIndicators(data *Data, timeframeKlines map[string][]Kline) map[string]interface{} {
	result := make(map[string]interface{})
	
	// 全局指标（不依赖时间框架）
	result["price"] = data.CurrentPrice
	result["funding_rate"] = data.FundingRate
	result["oi_latest"] = func() float64 {
		if data.OpenInterest != nil {
			return data.OpenInterest.Latest
		}
		return 0
	}()
	
	// 价格变化（基于3分钟K线计算）
	if klines3m, exists := timeframeKlines["3m"]; exists && len(klines3m) > 0 {
		// 1小时价格变化 = 20个3分钟K线前的价格
		if len(klines3m) >= 21 {
			price1hAgo := klines3m[len(klines3m)-21].Close
			if price1hAgo > 0 {
				result["change_1h"] = ((data.CurrentPrice - price1hAgo) / price1hAgo) * 100
			}
		}
	}
	
	// 4小时价格变化（基于4小时K线计算）
	if klines4h, exists := timeframeKlines["4h"]; exists && len(klines4h) >= 2 {
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			result["change_4h"] = ((data.CurrentPrice - price4hAgo) / price4hAgo) * 100
		}
	}
	
	// 各时间框架的基础指标
	timeframes := []string{"3m", "15m", "30m", "1h", "4h"}
	for _, tf := range timeframes {
		klines, exists := timeframeKlines[tf]
		if !exists || len(klines) == 0 {
			continue
		}
		
		tfData := map[string]interface{}{}
		
		// EMA20
		if len(klines) >= 20 {
			tfData["ema20"] = calculateEMA(klines, 20)
		}
		
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
		
		// 成交量
		if len(klines) > 0 {
			tfData["volume"] = klines[len(klines)-1].Volume
			// 平均成交量
			sum := 0.0
			for _, k := range klines {
				sum += k.Volume
			}
			tfData["avg_volume"] = sum / float64(len(klines))
		}
		
		// 只有当有数据时才添加到结果中
		if len(tfData) > 0 {
			result[tf] = tfData
		}
	}
	
	return result
}

// extractCompactMultiTimeframeAnalysis 提取精简的多时间框架分析数据
func extractCompactMultiTimeframeAnalysis(data *Data) map[string]interface{} {
	result := make(map[string]interface{})
	
	if data.MultiTimeframeAnalysis == nil || data.MultiTimeframeAnalysis.Timeframes == nil {
		return result
	}
	
	timeframes := []string{"3m", "15m", "30m", "1h", "4h"}
	
	for _, tf := range timeframes {
		tfData, exists := data.MultiTimeframeAnalysis.Timeframes[tf]
		if !exists || tfData == nil {
			continue
		}
		
		result[tf] = map[string]interface{}{
			"道氏理论数据": extractCompactDowTheory(tfData.DowTheory),
			"通道数据": extractCompactChannelAnalysis(tfData.ChannelAnalysis),
			"VPVR数据": extractCompactVPVR(tfData.VolumeProfile),
			"供需区数据": extractCompactSupplyDemand(tfData.SupplyDemand),
			"FVG数据": extractCompactFVG(tfData.FairValueGaps),
			"斐波纳契数据": extractCompactFibonacci(tfData.Fibonacci),
		}
	}
	
	return result
}

// extractCompactDowTheory 提取道氏理论的关键结果
func extractCompactDowTheory(data *DowTheoryData) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	
	result := map[string]interface{}{
		"trend_direction": "unknown",
		"trend_strength": 0.0,
		"signal_confidence": 0.0,
		"supertrend": map[string]interface{}{
			"direction": "unknown",
			"current_line": 0.0,
			"upper_line": 0.0,
			"lower_line": 0.0,
		},
	}
	
	if data.TrendStrength != nil {
		result["trend_direction"] = data.TrendStrength.Direction
		result["trend_strength"] = data.TrendStrength.Overall
		result["signal_confidence"] = data.TrendStrength.Consistency
	}
	
	return result
}

// extractCompactChannelAnalysis 提取通道分析的关键结果
func extractCompactChannelAnalysis(data *ChannelData) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	
	result := map[string]interface{}{
		"channel_direction": data.Direction,
		"channel_width": data.Quality * 100,
		"current_position": data.CurrentPosition,
	}
	
	if data.ActiveChannel != nil {
		result["channel_width"] = data.ActiveChannel.Width * 100
	}
	
	return result
}

// extractCompactVPVR 提取VPVR的关键结果
func extractCompactVPVR(data *VolumeProfile) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	
	result := map[string]interface{}{
		"poc_price": 0.0,
		"value_area_high": data.VAH,
		"value_area_low": data.VAL,
	}
	
	if data.POC != nil {
		result["poc_price"] = data.POC.Price
	}
	
	return result
}

// extractCompactSupplyDemand 提取供需区的关键结果
func extractCompactSupplyDemand(data *SupplyDemandData) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	
	result := map[string]interface{}{
		"total_zones": len(data.ActiveZones),
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
				"low":  zone.LowerBound,
				"high": zone.UpperBound,
			},
			"strength": zone.Strength,
			"touches": zone.TouchCount,
			"status": zone.Status,
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
		"avg_strength": strengthSum / float64(len(data.ActiveZones)),
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
func extractCompactFVG(data *FVGData) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	
	result := map[string]interface{}{
		"active_gaps": len(data.ActiveFVGs),
		"nearest_gap": 0.0,
		"gap_type": "unknown",
	}
	
	if len(data.ActiveFVGs) > 0 {
		// 取第一个活跃的FVG作为最近的
		fvg := data.ActiveFVGs[0]
		result["nearest_gap"] = (fvg.LowerBound + fvg.UpperBound) / 2
		if fvg.Type == BullishFVG {
			result["gap_type"] = "bullish"
		} else if fvg.Type == BearishFVG {
			result["gap_type"] = "bearish"
		} else {
			result["gap_type"] = "neutral"
		}
	}
	
	return result
}

// extractCompactFibonacci 提取斐波纳契的关键结果
func extractCompactFibonacci(data *FibonacciData) map[string]interface{} {
	if data == nil {
		return map[string]interface{}{}
	}
	
	result := map[string]interface{}{
		"active_retracements": 0,
		"levels": map[string]float64{},
		"trend_direction": "unknown",
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
						levels[ratioKey] = level.Price
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
					levels[ratioKey] = level.Price
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
		timeframeOrder := []string{"3m", "15m", "30m", "1h", "4h"}
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
			"3m": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "dow_theory"),
				"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "volume_profile"),
				"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "supply_demand"),
				"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "fibonacci"),
			},
			"15m": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "dow_theory"),
				"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "volume_profile"),
				"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "supply_demand"),
				"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fibonacci"),
			},
			"30m": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "dow_theory"),
				"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "volume_profile"),
				"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "supply_demand"),
				"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fibonacci"),
			},
			"1h": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "dow_theory"),
				"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "volume_profile"),
				"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "supply_demand"),
				"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fair_value_gaps"),
				"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fibonacci"),
			},
			"4h": map[string]interface{}{
				"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "dow_theory"),
				"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "channel_analysis"),
				"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "volume_profile"),
				"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "supply_demand"),
				"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "fair_value_gaps"),
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
		"3m": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "dow_theory"),
			"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "volume_profile"),
			"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "supply_demand"),
			"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "3m", "fibonacci"),
		},
		"15m": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "dow_theory"),
			"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "volume_profile"),
			"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "supply_demand"),
			"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "15m", "fibonacci"),
		},
		"30m": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "dow_theory"),
			"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "volume_profile"),
			"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "supply_demand"),
			"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "30m", "fibonacci"),
		},
		"1h": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "dow_theory"),
			"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "volume_profile"),
			"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "supply_demand"),
			"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fair_value_gaps"),
			"斐波纳契数据": extractTimeframeData(data.MultiTimeframeAnalysis, "1h", "fibonacci"),
		},
		"4h": map[string]interface{}{
			"道氏理论数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "dow_theory"),
			"通道数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "channel_analysis"),
			"VPVR数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "volume_profile"),
			"供需区数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "supply_demand"),
			"FVG数据": extractTimeframeData(data.MultiTimeframeAnalysis, "4h", "fair_value_gaps"),
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

// calculateSupertrend 计算超级趋势线
func calculateSupertrend(klines []Kline, atrPeriod int, factor float64) SuperTrendResult {
	result := SuperTrendResult{
		Direction:   "unknown",
		CurrentLine: 0.0,
		UpperLine:   0.0,
		LowerLine:   0.0,
	}
	
	if len(klines) < atrPeriod {
		return result
	}
	
	// 计算ATR
	atr := calculateATR(klines, atrPeriod)
	if atr == 0 {
		return result
	}
	
	// 获取最新的K线数据
	latest := klines[len(klines)-1]
	hl2 := (latest.High + latest.Low) / 2 // 中位价
	
	// 计算上轨和下轨
	upperLine := hl2 + (factor * atr)
	lowerLine := hl2 - (factor * atr)
	
	// 判断当前趋势方向
	var direction string
	var currentLine float64
	
	if latest.Close > lowerLine {
		// 价格在下轨之上，多头趋势
		direction = "bullish"
		currentLine = lowerLine
	} else if latest.Close < upperLine {
		// 价格在上轨之下，空头趋势  
		direction = "bearish"
		currentLine = upperLine
	} else {
		// 价格在上下轨之间，方向不明确
		direction = "sideways"
		currentLine = hl2
	}
	
	result.Direction = direction
	result.CurrentLine = currentLine
	result.UpperLine = upperLine
	result.LowerLine = lowerLine
	
	return result
}

// extractCompactMultiTimeframeAnalysisWithSupertrend 提取包含超级趋势的多时间框架分析
func extractCompactMultiTimeframeAnalysisWithSupertrend(data *Data, timeframeKlines map[string][]Kline) map[string]interface{} {
	result := make(map[string]interface{})
	
	if data.MultiTimeframeAnalysis == nil || data.MultiTimeframeAnalysis.Timeframes == nil {
		return result
	}
	
	timeframes := []string{"3m", "15m", "30m", "1h", "4h"}
	
	for _, tf := range timeframes {
		tfData, exists := data.MultiTimeframeAnalysis.Timeframes[tf]
		if !exists || tfData == nil {
			continue
		}
		
		// 计算该时间框架的超级趋势线
		klines := timeframeKlines[tf]
		supertrend := calculateSupertrend(klines, 20, 5.0)
		
		result[tf] = map[string]interface{}{
			"道氏理论数据": extractCompactDowTheoryWithSupertrend(tfData.DowTheory, supertrend),
			"通道数据": extractCompactChannelAnalysis(tfData.ChannelAnalysis),
			"VPVR数据": extractCompactVPVR(tfData.VolumeProfile),
			"供需区数据": extractCompactSupplyDemand(tfData.SupplyDemand),
			"FVG数据": extractCompactFVG(tfData.FairValueGaps),
			"斐波纳契数据": extractCompactFibonacci(tfData.Fibonacci),
		}
	}
	
	return result
}

// extractCompactDowTheoryWithSupertrend 提取包含超级趋势的道氏理论数据
func extractCompactDowTheoryWithSupertrend(data *DowTheoryData, supertrend SuperTrendResult) map[string]interface{} {
	result := map[string]interface{}{
		"trend_direction": "unknown",
		"trend_strength": 0.0,
		"signal_confidence": 0.0,
		"supertrend": map[string]interface{}{
			"direction": supertrend.Direction,
			"current_line": supertrend.CurrentLine,
			"upper_line": supertrend.UpperLine,
			"lower_line": supertrend.LowerLine,
		},
	}
	
	if data != nil && data.TrendStrength != nil {
		result["trend_direction"] = data.TrendStrength.Direction
		result["trend_strength"] = data.TrendStrength.Overall
		result["signal_confidence"] = data.TrendStrength.Consistency
	}
	
	return result
}

