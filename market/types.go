package market

import "time"

// Data 市场数据结构
type Data struct {
	Symbol            string
	CurrentPrice      float64
	PriceChange1h     float64 // 1小时价格变化百分比
	PriceChange4h     float64 // 4小时价格变化百分比
	CurrentEMA20      float64
	CurrentMACD       float64
	CurrentRSI7       float64
	OpenInterest      *OIData
	FundingRate       float64
	IntradaySeries    *IntradayData
	LongerTermContext *LongerTermData
	DowTheory         *DowTheoryData // 道氏理论分析数据
}

// OIData Open Interest数据
type OIData struct {
	Latest  float64
	Average float64
}

// IntradayData 日内数据(3分钟间隔)
type IntradayData struct {
	MidPrices   []float64
	EMA20Values []float64
	MACDValues  []float64
	RSI7Values  []float64
	RSI14Values []float64
}

// LongerTermData 长期数据(4小时时间框架)
type LongerTermData struct {
	EMA20         float64
	EMA50         float64
	ATR3          float64
	ATR14         float64
	CurrentVolume float64
	AverageVolume float64
	MACDValues    []float64
	RSI14Values   []float64
}

// Binance API 响应结构
type ExchangeInfo struct {
	Symbols []SymbolInfo `json:"symbols"`
}

type SymbolInfo struct {
	Symbol            string `json:"symbol"`
	Status            string `json:"status"`
	BaseAsset         string `json:"baseAsset"`
	QuoteAsset        string `json:"quoteAsset"`
	ContractType      string `json:"contractType"`
	PricePrecision    int    `json:"pricePrecision"`
	QuantityPrecision int    `json:"quantityPrecision"`
}

type Kline struct {
	OpenTime            int64   `json:"openTime"`
	Open                float64 `json:"open"`
	High                float64 `json:"high"`
	Low                 float64 `json:"low"`
	Close               float64 `json:"close"`
	Volume              float64 `json:"volume"`
	CloseTime           int64   `json:"closeTime"`
	QuoteVolume         float64 `json:"quoteVolume"`
	Trades              int     `json:"trades"`
	TakerBuyBaseVolume  float64 `json:"takerBuyBaseVolume"`
	TakerBuyQuoteVolume float64 `json:"takerBuyQuoteVolume"`
}

type KlineResponse []interface{}

type PriceTicker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

type Ticker24hr struct {
	Symbol             string `json:"symbol"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
}

// 特征数据结构
type SymbolFeatures struct {
	Symbol           string    `json:"symbol"`
	Timestamp        time.Time `json:"timestamp"`
	Price            float64   `json:"price"`
	PriceChange15Min float64   `json:"price_change_15min"`
	PriceChange1H    float64   `json:"price_change_1h"`
	PriceChange4H    float64   `json:"price_change_4h"`
	Volume           float64   `json:"volume"`
	VolumeRatio5     float64   `json:"volume_ratio_5"`
	VolumeRatio20    float64   `json:"volume_ratio_20"`
	VolumeTrend      float64   `json:"volume_trend"`
	RSI14            float64   `json:"rsi_14"`
	SMA5             float64   `json:"sma_5"`
	SMA10            float64   `json:"sma_10"`
	SMA20            float64   `json:"sma_20"`
	HighLowRatio     float64   `json:"high_low_ratio"`
	Volatility20     float64   `json:"volatility_20"`
	PositionInRange  float64   `json:"position_in_range"`
}

// 警报数据结构
type Alert struct {
	Type      string    `json:"type"`
	Symbol    string    `json:"symbol"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type Config struct {
	AlertThresholds AlertThresholds `json:"alert_thresholds"`
	UpdateInterval  int             `json:"update_interval"` // seconds
	CleanupConfig   CleanupConfig   `json:"cleanup_config"`
}

type AlertThresholds struct {
	VolumeSpike      float64 `json:"volume_spike"`
	PriceChange15Min float64 `json:"price_change_15min"`
	VolumeTrend      float64 `json:"volume_trend"`
	RSIOverbought    float64 `json:"rsi_overbought"`
	RSIOversold      float64 `json:"rsi_oversold"`
}
type CleanupConfig struct {
	InactiveTimeout   time.Duration `json:"inactive_timeout"`    // 不活跃超时时间
	MinScoreThreshold float64       `json:"min_score_threshold"` // 最低评分阈值
	NoAlertTimeout    time.Duration `json:"no_alert_timeout"`    // 无警报超时时间
	CheckInterval     time.Duration `json:"check_interval"`      // 检查间隔
}

// 道氏理论相关数据结构
type DowTheoryData struct {
	SwingPoints   []*SwingPoint    `json:"swing_points"`
	TrendLines    []*TrendLine     `json:"trend_lines"`
	Channel       *ParallelChannel `json:"channel"`
	TrendStrength *TrendStrength   `json:"trend_strength"`
	TradingSignal *TradingSignal   `json:"trading_signal"`
}

// 摆动点结构
type SwingPoint struct {
	Type      SwingType `json:"type"`      // 高点或低点
	Price     float64   `json:"price"`     // 价格
	Time      int64     `json:"time"`      // 时间戳
	Index     int       `json:"index"`     // 在K线数组中的索引
	Strength  float64   `json:"strength"`  // 摆动点强度
	Confirmed bool      `json:"confirmed"` // 是否确认
}

type SwingType string

const (
	SwingHigh SwingType = "high"
	SwingLow  SwingType = "low"
)

// 趋势线结构
type TrendLine struct {
	Type      TrendLineType `json:"type"`       // 支撑线或阻力线
	Points    []*SwingPoint `json:"points"`     // 构成趋势线的摆动点
	Slope     float64       `json:"slope"`      // 斜率
	Intercept float64       `json:"intercept"`  // 截距
	Strength  float64       `json:"strength"`   // 趋势线强度
	Touches   int           `json:"touches"`    // 触及次数
	LastTouch int64         `json:"last_touch"` // 最后触及时间
	Broken    bool          `json:"broken"`     // 是否被突破
	BreakTime int64         `json:"break_time"` // 突破时间
}

type TrendLineType string

const (
	SupportLine    TrendLineType = "support"
	ResistanceLine TrendLineType = "resistance"
)

// 平行通道结构
type ParallelChannel struct {
	UpperLine  *TrendLine      `json:"upper_line"`  // 上轨
	LowerLine  *TrendLine      `json:"lower_line"`  // 下轨
	MiddleLine *TrendLine      `json:"middle_line"` // 中轨
	Width      float64         `json:"width"`       // 通道宽度
	Direction  TrendDirection  `json:"direction"`   // 通道方向
	Quality    float64         `json:"quality"`     // 通道质量评分
	CurrentPos ChannelPosition `json:"current_pos"` // 当前价格在通道中的位置
	PriceRatio float64         `json:"price_ratio"` // 价格在通道中的比例 (0-1)
}

type TrendDirection string

const (
	TrendUp   TrendDirection = "up"
	TrendDown TrendDirection = "down"
	TrendFlat TrendDirection = "flat"
)

type ChannelPosition string

const (
	ChannelUpper  ChannelPosition = "upper"
	ChannelMiddle ChannelPosition = "middle"
	ChannelLower  ChannelPosition = "lower"
	ChannelBreak  ChannelPosition = "break"
)

// 趋势强度结构
type TrendStrength struct {
	Overall       float64        `json:"overall"`        // 总体趋势强度
	ShortTerm     float64        `json:"short_term"`     // 短期趋势强度
	LongTerm      float64        `json:"long_term"`      // 长期趋势强度
	Direction     TrendDirection `json:"direction"`      // 趋势方向
	Quality       TrendQuality   `json:"quality"`        // 趋势质量
	Momentum      float64        `json:"momentum"`       // 动量强度
	Consistency   float64        `json:"consistency"`    // 一致性评分
	VolumeSupport float64        `json:"volume_support"` // 成交量支撑度
}

type TrendQuality string

const (
	TrendStrong   TrendQuality = "strong"
	TrendModerate TrendQuality = "moderate"
	TrendWeak     TrendQuality = "weak"
)

// 交易信号结构
type TradingSignal struct {
	Type          SignalType   `json:"type"`           // 信号类型
	Action        SignalAction `json:"action"`         // 建议动作
	Confidence    float64      `json:"confidence"`     // 置信度 (0-100)
	Entry         float64      `json:"entry"`          // 建议入场价
	StopLoss      float64      `json:"stop_loss"`      // 止损价
	TakeProfit    float64      `json:"take_profit"`    // 止盈价
	RiskReward    float64      `json:"risk_reward"`    // 风险收益比
	Description   string       `json:"description"`    // 信号描述
	Timestamp     int64        `json:"timestamp"`      // 信号生成时间
	ChannelBased  bool         `json:"channel_based"`  // 是否基于通道
	BreakoutBased bool         `json:"breakout_based"` // 是否基于突破
}

type SignalType string

const (
	SignalChannelBounce   SignalType = "channel_bounce"   // 通道反弹
	SignalChannelBreakout SignalType = "channel_breakout" // 通道突破
	SignalTrendFollowing  SignalType = "trend_following"  // 趋势跟随
	SignalReversal        SignalType = "reversal"         // 趋势反转
)

type SignalAction string

const (
	ActionBuy   SignalAction = "buy"
	ActionSell  SignalAction = "sell"
	ActionHold  SignalAction = "hold"
	ActionClose SignalAction = "close"
)

// 道氏理论配置
type DowTheoryConfig struct {
	SwingPointConfig SwingPointConfig `json:"swing_point_config"`
	TrendLineConfig  TrendLineConfig  `json:"trend_line_config"`
	ChannelConfig    ChannelConfig    `json:"channel_config"`
	SignalConfig     SignalConfig     `json:"signal_config"`
}

type SwingPointConfig struct {
	LookbackPeriod int     `json:"lookback_period"`  // 回看周期
	MinStrength    float64 `json:"min_strength"`     // 最小强度阈值
	ConfirmPeriod  int     `json:"confirm_period"`   // 确认周期
	MinPriceChange float64 `json:"min_price_change"` // 最小价格变化百分比
}

type TrendLineConfig struct {
	MinTouches     int     `json:"min_touches"`     // 最少触及次数
	MaxDistance    float64 `json:"max_distance"`    // 最大距离百分比
	BreakThreshold float64 `json:"break_threshold"` // 突破阈值百分比
	MinSlope       float64 `json:"min_slope"`       // 最小斜率
	MaxAge         int     `json:"max_age"`         // 最大存活周期
}

type ChannelConfig struct {
	MinWidth          float64 `json:"min_width"`          // 最小通道宽度百分比
	MaxWidth          float64 `json:"max_width"`          // 最大通道宽度百分比
	QualityThreshold  float64 `json:"quality_threshold"`  // 质量阈值
	ParallelTolerance float64 `json:"parallel_tolerance"` // 平行度容忍度
}

type SignalConfig struct {
	MinConfidence      float64 `json:"min_confidence"`      // 最小置信度
	RiskRewardMin      float64 `json:"risk_reward_min"`     // 最小风险收益比
	BreakoutStrength   float64 `json:"breakout_strength"`   // 突破强度要求
	VolumeConfirmation bool    `json:"volume_confirmation"` // 是否需要成交量确认
}

var config = Config{
	AlertThresholds: AlertThresholds{
		VolumeSpike:      3.0,
		PriceChange15Min: 0.05,
		VolumeTrend:      2.0,
		RSIOverbought:    70,
		RSIOversold:      30,
	},
	CleanupConfig: CleanupConfig{
		InactiveTimeout:   30 * time.Minute,
		MinScoreThreshold: 15.0,
		NoAlertTimeout:    20 * time.Minute,
		CheckInterval:     5 * time.Minute,
	},
	UpdateInterval: 60, // 1 minute
}

var dowConfig = DowTheoryConfig{
	SwingPointConfig: SwingPointConfig{
		LookbackPeriod: 5,
		MinStrength:    0.5,
		ConfirmPeriod:  3,
		MinPriceChange: 0.01, // 1%
	},
	TrendLineConfig: TrendLineConfig{
		MinTouches:     2,
		MaxDistance:    0.02, // 2%
		BreakThreshold: 0.01, // 1%
		MinSlope:       0.0001,
		MaxAge:         50,
	},
	ChannelConfig: ChannelConfig{
		MinWidth:          0.02, // 2%
		MaxWidth:          0.15, // 15%
		QualityThreshold:  0.7,
		ParallelTolerance: 0.1,
	},
	SignalConfig: SignalConfig{
		MinConfidence:      60.0,
		RiskRewardMin:      1.5,
		BreakoutStrength:   0.015, // 1.5%
		VolumeConfirmation: true,
	},
}
