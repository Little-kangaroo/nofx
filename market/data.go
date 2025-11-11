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
	var klines3m, klines4h []Kline
	var err error
	// 标准化symbol
	symbol = Normalize(symbol)
	// 获取3分钟K线数据 (最近10个)
	klines3m, err = WSMonitorCli.GetCurrentKlines(symbol, "3m") // 多获取一些用于计算
	if err != nil {
		return nil, fmt.Errorf("获取3分钟K线失败: %v", err)
	}

	// 获取4小时K线数据 (最近10个)
	klines4h, err = WSMonitorCli.GetCurrentKlines(symbol, "4h") // 多获取用于计算指标
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

	// 道氏理论分析
	dowAnalyzer := NewDowTheoryAnalyzer()
	dowTheoryData := dowAnalyzer.Analyze(klines3m, klines4h, currentPrice)

	return &Data{
		Symbol:            symbol,
		CurrentPrice:      currentPrice,
		PriceChange1h:     priceChange1h,
		PriceChange4h:     priceChange4h,
		CurrentEMA20:      currentEMA20,
		CurrentMACD:       currentMACD,
		CurrentRSI7:       currentRSI7,
		OpenInterest:      oiData,
		FundingRate:       fundingRate,
		IntradaySeries:    intradayData,
		LongerTermContext: longerTermData,
		DowTheory:         dowTheoryData,
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
		sb.WriteString("Volume Profile (VPVR) Analysis:\n\n")
		sb.WriteString(formatVPVRData(data.VolumeProfile))
	}

	// 供需区分析
	if data.SupplyDemand != nil {
		sb.WriteString("Supply/Demand Zones Analysis:\n\n")
		sb.WriteString(formatSupplyDemandData(data.SupplyDemand))
	}

	// FVG分析
	if data.FairValueGaps != nil {
		sb.WriteString("Fair Value Gap (FVG) Analysis:\n\n")
		sb.WriteString(formatFVGData(data.FairValueGaps))
	}

	// 斐波纳契分析
	if data.Fibonacci != nil {
		sb.WriteString("Fibonacci Analysis:\n\n")
		sb.WriteString(formatFibonacciData(data.Fibonacci))
	}

	return sb.String()
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

// formatVPVRData 格式化VPVR数据 - 暂时简化实现
func formatVPVRData(data interface{}) string {
	return "VPVR Analysis: [数据格式化功能正在开发中]\n\n"
}

// formatSupplyDemandData 格式化供需区数据 - 暂时简化实现  
func formatSupplyDemandData(data interface{}) string {
	return "Supply/Demand Zones Analysis: [数据格式化功能正在开发中]\n\n"
}

// formatFVGData 格式化FVG数据 - 暂时简化实现
func formatFVGData(data interface{}) string {
	return "Fair Value Gap Analysis: [数据格式化功能正在开发中]\n\n"
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


