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
	ChannelAnalysis   *ChannelData // 通道分析数据（独立指标）
	VolumeProfile     *VolumeProfile // 成交量分布数据
	SupplyDemand      *SupplyDemandData // 供给需求区数据
	FairValueGaps     *FVGData // 公平价值缺口数据
	Fibonacci         *FibonacciData // 斐波纳契分析数据
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

// VPVR (Volume Profile Visible Range) 相关数据结构
type VolumeProfile struct {
	POC       *PriceLevel     `json:"poc"`        // Point of Control - 最大成交量价格
	VAH       float64         `json:"vah"`        // Value Area High - 价值区域高点
	VAL       float64         `json:"val"`        // Value Area Low - 价值区域低点
	ValueArea *ValueArea      `json:"value_area"` // 价值区域详细信息
	Levels    []*PriceLevel   `json:"levels"`     // 所有价格级别
	Config    *VPVRConfig     `json:"config"`     // VPVR配置
	Stats     *VolumeStats    `json:"stats"`      // 成交量统计
}

// PriceLevel 价格级别
type PriceLevel struct {
	Price         float64 `json:"price"`          // 价格
	Volume        float64 `json:"volume"`         // 成交量
	BuyVolume     float64 `json:"buy_volume"`     // 买入成交量
	SellVolume    float64 `json:"sell_volume"`    // 卖出成交量
	VolumePercent float64 `json:"volume_percent"` // 成交量占比
	Transactions  int     `json:"transactions"`   // 交易次数
	IsPOC         bool    `json:"is_poc"`         // 是否为POC
	InValueArea   bool    `json:"in_value_area"`  // 是否在价值区域内
}

// ValueArea 价值区域
type ValueArea struct {
	High              float64 `json:"high"`                // 价值区域高点
	Low               float64 `json:"low"`                 // 价值区域低点
	VolumePercent     float64 `json:"volume_percent"`      // 价值区域成交量占比
	PriceRange        float64 `json:"price_range"`         // 价格范围
	PriceRangePercent float64 `json:"price_range_percent"` // 价格范围占比
	ProfileWidth      float64 `json:"profile_width"`       // 分布宽度
	Concentration     float64 `json:"concentration"`       // 成交量集中度
}

// VolumeStats 成交量统计
type VolumeStats struct {
	TotalVolume    float64 `json:"total_volume"`    // 总成交量
	TotalBuyVolume float64 `json:"total_buy_volume"` // 总买入成交量
	TotalSellVolume float64 `json:"total_sell_volume"` // 总卖出成交量
	BuySellRatio   float64 `json:"buy_sell_ratio"`  // 买卖比
	AvgPrice       float64 `json:"avg_price"`       // 成交量加权平均价格
	MedianPrice    float64 `json:"median_price"`    // 中位数价格
	PriceStdDev    float64 `json:"price_std_dev"`   // 价格标准差
	MaxLevel       *PriceLevel `json:"max_level"`   // 最大成交量级别
	MinLevel       *PriceLevel `json:"min_level"`   // 最小成交量级别
}

// VPVRConfig VPVR配置
type VPVRConfig struct {
	TickSize         float64 `json:"tick_size"`          // 价格精度
	ValueAreaPercent float64 `json:"value_area_percent"` // 价值区域百分比 (默认70%)
	MinVolume        float64 `json:"min_volume"`         // 最小成交量阈值
	TimeFrame        string  `json:"time_frame"`         // 时间框架
	ShowBuySell      bool    `json:"show_buy_sell"`      // 是否显示买卖分���
	SmoothingFactor  float64 `json:"smoothing_factor"`   // 平滑因子
}

// VPVRSignal VPVR交易信号
type VPVRSignal struct {
	Type        VPVRSignalType `json:"type"`        // 信号类型
	Level       float64        `json:"level"`       // 关键价格级别
	CurrentPrice float64       `json:"current_price"` // 当前价格
	Strength    float64        `json:"strength"`    // 信号强度
	Description string         `json:"description"` // 信号描述
	Action      SignalAction   `json:"action"`      // 建议动作
	Confidence  float64        `json:"confidence"`  // 置信度
	Timestamp   int64          `json:"timestamp"`   // 时间戳
}

type VPVRSignalType string

const (
	VPVRSignalPOCTest      VPVRSignalType = "poc_test"      // POC测试
	VPVRSignalVABreakout   VPVRSignalType = "va_breakout"   // 价值区域突破
	VPVRSignalVAReturn     VPVRSignalType = "va_return"     // 回归价值区域
	VPVRSignalHighVolume   VPVRSignalType = "high_volume"   // 高成交量级别
	VPVRSignalLowVolume    VPVRSignalType = "low_volume"    // 低成交量级别
	VPVRSignalImbalance    VPVRSignalType = "imbalance"     // 买卖不平衡
)

var defaultVPVRConfig = VPVRConfig{
	TickSize:         0.01,   // 默认1分精度
	ValueAreaPercent: 0.70,   // 70%价值区域
	MinVolume:        0.001,  // 最小成交量
	TimeFrame:        "4h",   // 4小时时间框架
	ShowBuySell:      true,   // 显示买卖分布
	SmoothingFactor:  1.0,    // 无平滑
}

// Supply/Demand Zone 供给/需求区相关数据结构
type SupplyDemandData struct {
	SupplyZones  []*SupplyDemandZone `json:"supply_zones"`  // 供给区
	DemandZones  []*SupplyDemandZone `json:"demand_zones"`  // 需求区
	ActiveZones  []*SupplyDemandZone `json:"active_zones"`  // 活跃区域
	Config       *SDConfig           `json:"config"`        // 配置
	Statistics   *SDStatistics       `json:"statistics"`    // 统计信息
	LastAnalysis int64               `json:"last_analysis"` // 最后分析时间
}

// SupplyDemandZone 供给/需求区
type SupplyDemandZone struct {
	ID            string      `json:"id"`             // 区域ID
	Type          ZoneType    `json:"type"`           // 区域类型（供给区/需求区）
	UpperBound    float64     `json:"upper_bound"`    // 上边界
	LowerBound    float64     `json:"lower_bound"`    // 下边界
	CenterPrice   float64     `json:"center_price"`   // 中心价格
	Width         float64     `json:"width"`          // 区域宽度
	WidthPercent  float64     `json:"width_percent"`  // 区域宽度百分比
	Origin        *ZoneOrigin `json:"origin"`         // 区域起源
	Strength      float64     `json:"strength"`       // 区域强度
	Quality       ZoneQuality `json:"quality"`        // 区域质量
	Status        ZoneStatus  `json:"status"`         // 区域状态
	TouchCount    int         `json:"touch_count"`    // 触及次数
	LastTouch     int64       `json:"last_touch"`     // 最后触及时间
	CreationTime  int64       `json:"creation_time"`  // 创建时间
	Volume        float64     `json:"volume"`         // 相关成交量
	VolumeProfile *ZoneVP     `json:"volume_profile"` // 区域成交量分布
	Validation    *Validation `json:"validation"`     // 验证信息
	IsActive      bool        `json:"is_active"`      // 是否活跃
	IsBroken      bool        `json:"is_broken"`      // 是否被突破
	BreakTime     int64       `json:"break_time"`     // 突破时间
}

// ZoneType 区域类型
type ZoneType string

const (
	SupplyZone ZoneType = "supply" // 供给区
	DemandZone ZoneType = "demand" // 需求区
)

// ZoneOrigin 区域起源
type ZoneOrigin struct {
	KlineIndex    int         `json:"kline_index"`    // 起源K线索引
	PatternType   PatternType `json:"pattern_type"`   // 模式类型
	ImpulseMove   float64     `json:"impulse_move"`   // 冲击移动幅度
	ImpulseVolume float64     `json:"impulse_volume"` // 冲击成交量
	TimeFrame     string      `json:"time_frame"`     // 时间框架
	Confirmation  bool        `json:"confirmation"`   // 是否确认
}

// PatternType 模式类型
type PatternType string

const (
	RallyBaseRally   PatternType = "rally_base_rally"   // 上涨-整理-上涨（需求区）
	DropBaseDrop     PatternType = "drop_base_drop"     // 下跌-整理-下跌（供给区）
	RallyBaseDropOB  PatternType = "rally_base_drop_ob" // 上涨-整理-下跌（订单区块）
	DropBaseRallyOB  PatternType = "drop_base_rally_ob" // 下跌-整理-上涨（订单区块）
	FreshSupply      PatternType = "fresh_supply"       // 新鲜供给区
	FreshDemand      PatternType = "fresh_demand"       // 新鲜需求区
)

// ZoneQuality 区域质量
type ZoneQuality string

const (
	QualityStrong   ZoneQuality = "strong"   // 强
	QualityGood     ZoneQuality = "good"     // 良好
	QualityModerate ZoneQuality = "moderate" // 中等
	QualityWeak     ZoneQuality = "weak"     // 弱
)

// ZoneStatus 区域状态
type ZoneStatus string

const (
	StatusFresh    ZoneStatus = "fresh"    // 新鲜
	StatusTested   ZoneStatus = "tested"   // 已测试
	StatusWeakened ZoneStatus = "weakened" // 已弱化
	StatusBroken   ZoneStatus = "broken"   // 已突破
	StatusExpired  ZoneStatus = "expired"  // 已过期
)

// ZoneVP 区域成交量分布
type ZoneVP struct {
	TotalVolume    float64 `json:"total_volume"`    // 总成交量
	BuyVolume      float64 `json:"buy_volume"`      // 买入成交量
	SellVolume     float64 `json:"sell_volume"`     // 卖出成交量
	VolumeAtOrigin float64 `json:"volume_at_origin"` // 起源处成交量
	VolumeImbalance float64 `json:"volume_imbalance"` // 成交量不平衡
}

// Validation 验证信息
type Validation struct {
	HasReaction      bool    `json:"has_reaction"`      // 是否有反应
	ReactionStrength float64 `json:"reaction_strength"` // 反应强度
	TimeInZone       int64   `json:"time_in_zone"`      // 在区域内的时间
	VolumeAtTest     float64 `json:"volume_at_test"`    // 测试时的成交量
	PriceAction      string  `json:"price_action"`      // 价格行为
}

// SDConfig 供需区配置
type SDConfig struct {
	MinImpulsePercent  float64 `json:"min_impulse_percent"`  // 最小冲击百分比
	MinBasePercent     float64 `json:"min_base_percent"`     // 最小整理百分比
	MaxBasePercent     float64 `json:"max_base_percent"`     // 最大整理百分比
	MinVolumeFactor    float64 `json:"min_volume_factor"`    // 最小成交量因子
	MaxZoneAge         int     `json:"max_zone_age"`         // 最大区域年龄（K线数）
	MaxTouchCount      int     `json:"max_touch_count"`      // 最大触及次数
	BreakoutThreshold  float64 `json:"breakout_threshold"`   // 突破阈值
	ConfirmationBars   int     `json:"confirmation_bars"`    // 确认K线数
	TimeFrames         []string `json:"time_frames"`         // 分析时间框架
	EnableValidation   bool    `json:"enable_validation"`    // 是否启用验证
	QualityThreshold   float64 `json:"quality_threshold"`    // 质量阈值
}

// SDStatistics 供需区统计
type SDStatistics struct {
	TotalSupplyZones   int     `json:"total_supply_zones"`   // 总供给区数
	TotalDemandZones   int     `json:"total_demand_zones"`   // 总需求区数
	ActiveSupplyZones  int     `json:"active_supply_zones"`  // 活跃供给区数
	ActiveDemandZones  int     `json:"active_demand_zones"`  // 活跃需求区数
	AvgZoneStrength    float64 `json:"avg_zone_strength"`    // 平均区域强度
	AvgZoneWidth       float64 `json:"avg_zone_width"`       // 平均区域宽度
	SuccessRate        float64 `json:"success_rate"`         // 成功率
	BreakoutRate       float64 `json:"breakout_rate"`        // 突破率
	ReactionRate       float64 `json:"reaction_rate"`        // 反应率
}

// SDSignal 供需区交易信号
type SDSignal struct {
	Type         SDSignalType `json:"type"`         // 信号类型
	Zone         *SupplyDemandZone `json:"zone"`    // 相关区域
	CurrentPrice float64      `json:"current_price"` // 当前价格
	Action       SignalAction `json:"action"`       // 建议动作
	Entry        float64      `json:"entry"`        // 入场价
	StopLoss     float64      `json:"stop_loss"`    // 止损价
	TakeProfit   float64      `json:"take_profit"`  // 止盈价
	RiskReward   float64      `json:"risk_reward"`  // 风险收益比
	Confidence   float64      `json:"confidence"`   // 置信度
	Strength     float64      `json:"strength"`     // 信号强度
	Description  string       `json:"description"`  // 信号描述
	Timestamp    int64        `json:"timestamp"`    // 时间戳
}

// SDSignalType 供需区信号类型
type SDSignalType string

const (
	SDSignalZoneEntry    SDSignalType = "zone_entry"    // 进入区域
	SDSignalZoneBounce   SDSignalType = "zone_bounce"   // 区域反弹
	SDSignalZoneBreakout SDSignalType = "zone_breakout" // 区域突破
	SDSignalZoneRetest   SDSignalType = "zone_retest"   // 区域回测
	SDSignalFreshZone    SDSignalType = "fresh_zone"    // 新鲜区域
)

var defaultSDConfig = SDConfig{
	MinImpulsePercent:  0.02,   // 2%最小冲击
	MinBasePercent:     0.005,  // 0.5%最小整理
	MaxBasePercent:     0.03,   // 3%最大整理
	MinVolumeFactor:    1.5,    // 1.5倍成交量
	MaxZoneAge:         50,     // 50根K线
	MaxTouchCount:      3,      // 最大3次触及
	BreakoutThreshold:  0.01,   // 1%突破阈值
	ConfirmationBars:   2,      // 2根确认K线
	TimeFrames:         []string{"15m", "1h", "4h"},
	EnableValidation:   true,
	QualityThreshold:   0.6,    // 60%质量阈值
}

// Fair Value Gap (FVG) 公平价值缺口相关数据结构
type FVGData struct {
	BullishFVGs  []*FairValueGap `json:"bullish_fvgs"`  // 看涨FVG
	BearishFVGs  []*FairValueGap `json:"bearish_fvgs"`  // 看跌FVG
	ActiveFVGs   []*FairValueGap `json:"active_fvgs"`   // 活跃FVG
	Config       *FVGConfig      `json:"config"`        // FVG配置
	Statistics   *FVGStatistics  `json:"statistics"`    // FVG统计
	LastAnalysis int64           `json:"last_analysis"` // 最后分析时间
}

// FairValueGap 公平价值缺口
type FairValueGap struct {
	ID             string      `json:"id"`              // FVG ID
	Type           FVGType     `json:"type"`            // FVG类型（看涨/看跌）
	UpperBound     float64     `json:"upper_bound"`     // 上边界
	LowerBound     float64     `json:"lower_bound"`     // 下边界
	CenterPrice    float64     `json:"center_price"`    // 中心价格
	Width          float64     `json:"width"`           // 缺口宽度
	WidthPercent   float64     `json:"width_percent"`   // 缺口宽度百分比
	Origin         *FVGOrigin  `json:"origin"`          // 缺口起源
	Strength       float64     `json:"strength"`        // 缺口强度
	Quality        FVGQuality  `json:"quality"`         // 缺口质量
	Status         FVGStatus   `json:"status"`          // 缺口状态
	TouchCount     int         `json:"touch_count"`     // 触及次数
	FillProgress   float64     `json:"fill_progress"`   // 填补进度 (0-100%)
	LastTouch      int64       `json:"last_touch"`      // 最后触及时间
	CreationTime   int64       `json:"creation_time"`   // 创建时间
	FillTime       int64       `json:"fill_time"`       // 填补时间
	IsActive       bool        `json:"is_active"`       // 是否活跃
	IsFilled       bool        `json:"is_filled"`       // 是否已填补
	IsPartialFill  bool        `json:"is_partial_fill"` // 是否部分填补
	VolumeContext  *FVGVolume  `json:"volume_context"`  // 成交量上下文
	Validation     *FVGValidation `json:"validation"`   // 验证信息
}

// FVGType FVG类型
type FVGType string

const (
	BullishFVG FVGType = "bullish" // 看涨FVG
	BearishFVG FVGType = "bearish" // 看跌FVG
)

// FVGOrigin FVG起源信息
type FVGOrigin struct {
	KlineIndex       int          `json:"kline_index"`       // 起源K线索引（中间K线）
	PreviousCandle   *CandleInfo  `json:"previous_candle"`   // 前一根K线信息
	CurrentCandle    *CandleInfo  `json:"current_candle"`    // 当前K线信息（形成缺口的K线）
	NextCandle       *CandleInfo  `json:"next_candle"`       // 下一根K线信息
	ImpulsiveMove    float64      `json:"impulsive_move"`    // 冲动移动幅度
	TimeFrame        string       `json:"time_frame"`        // 时间框架
	FormationType    FormationType `json:"formation_type"`   // 形成类型
}

// CandleInfo K线信息
type CandleInfo struct {
	Index     int     `json:"index"`      // 索引
	Open      float64 `json:"open"`       // 开盘价
	High      float64 `json:"high"`       // 最高价
	Low       float64 `json:"low"`        // 最低价
	Close     float64 `json:"close"`      // 收盘价
	Volume    float64 `json:"volume"`     // 成交量
	Timestamp int64   `json:"timestamp"`  // 时间戳
}

// FormationType FVG形成类型
type FormationType string

const (
	FormationBreakout   FormationType = "breakout"   // 突破形成
	FormationPullback   FormationType = "pullback"   // 回调形成
	FormationContinuation FormationType = "continuation" // 延续形成
	FormationReversal   FormationType = "reversal"   // 反转形成
)

// FVGQuality FVG质量
type FVGQuality string

const (
	FVQualityHigh     FVGQuality = "high"     // 高质量
	FVQualityMedium   FVGQuality = "medium"   // 中等质量
	FVQualityLow      FVGQuality = "low"      // 低质量
)

// FVGStatus FVG状态
type FVGStatus string

const (
	FVGStatusFresh       FVGStatus = "fresh"        // 新鲜
	FVGStatusTested      FVGStatus = "tested"       // 已测试
	FVGStatusPartialFill FVGStatus = "partial_fill" // 部分填补
	FVGStatusFilled      FVGStatus = "filled"       // 已填补
	FVGStatusExpired     FVGStatus = "expired"      // 已过期
)

// FVGVolume FVG成交量上下文
type FVGVolume struct {
	FormationVolume    float64 `json:"formation_volume"`    // 形成时成交量
	AverageVolume      float64 `json:"average_volume"`      // 平均成交量
	VolumeRatio        float64 `json:"volume_ratio"`        // 成交量比率
	TouchVolumes       []float64 `json:"touch_volumes"`     // 各次触及的成交量
	FillVolume         float64 `json:"fill_volume"`         // 填补时成交量
	VolumeConfirmation bool    `json:"volume_confirmation"` // 成交量确认
}

// FVGValidation FVG验证信息
type FVGValidation struct {
	HasReaction       bool    `json:"has_reaction"`       // 是否有反应
	ReactionStrength  float64 `json:"reaction_strength"`  // 反应强度
	HoldingStrength   float64 `json:"holding_strength"`   // 持有强度
	ReversalSign      bool    `json:"reversal_sign"`      // 是否有反转迹象
	VolumeValidation  bool    `json:"volume_validation"`  // 成交量验证
	TimeValidation    bool    `json:"time_validation"`    // 时间验证
}

// FVGConfig FVG配置
type FVGConfig struct {
	MinGapPercent     float64   `json:"min_gap_percent"`     // 最小缺口百分比
	MaxGapPercent     float64   `json:"max_gap_percent"`     // 最大缺口百分比
	MinVolumeRatio    float64   `json:"min_volume_ratio"`    // 最小成交量比率
	MaxAge            int       `json:"max_age"`             // 最大存在时间（K线数）
	MaxTouchCount     int       `json:"max_touch_count"`     // 最大触及次数
	FillThreshold     float64   `json:"fill_threshold"`      // 填补阈值（百分比）
	TimeFrames        []string  `json:"time_frames"`         // 分析时间框架
	EnableValidation  bool      `json:"enable_validation"`   // 是否启用验证
	QualityThreshold  float64   `json:"quality_threshold"`   // 质量阈值
	RequireVolConf    bool      `json:"require_vol_conf"`    // 是否需要成交量确认
}

// FVGStatistics FVG统计信息
type FVGStatistics struct {
	TotalBullishFVGs   int     `json:"total_bullish_fvgs"`   // 总看涨FVG数
	TotalBearishFVGs   int     `json:"total_bearish_fvgs"`   // 总看跌FVG数
	ActiveBullishFVGs  int     `json:"active_bullish_fvgs"`  // 活跃看涨FVG数
	ActiveBearishFVGs  int     `json:"active_bearish_fvgs"`  // 活跃看跌FVG数
	AvgFVGWidth        float64 `json:"avg_fvg_width"`        // 平均FVG宽度
	AvgFVGStrength     float64 `json:"avg_fvg_strength"`     // 平均FVG强度
	FillRate           float64 `json:"fill_rate"`            // 填补率
	SuccessRate        float64 `json:"success_rate"`         // 成功率（产生反应的比例）
	AvgFillTime        float64 `json:"avg_fill_time"`        // 平均填补时间（小时）
	QualityDistribution map[FVGQuality]int `json:"quality_distribution"` // 质量分布
}

// FVGSignal FVG交易信号
type FVGSignal struct {
	Type         FVGSignalType   `json:"type"`         // 信号类型
	FVG          *FairValueGap   `json:"fvg"`          // 相关FVG
	CurrentPrice float64         `json:"current_price"` // 当前价格
	Action       SignalAction    `json:"action"`       // 建议动作
	Entry        float64         `json:"entry"`        // 入场价
	StopLoss     float64         `json:"stop_loss"`    // 止损价
	TakeProfit   float64         `json:"take_profit"`  // 止盈价
	RiskReward   float64         `json:"risk_reward"`  // 风险收益比
	Confidence   float64         `json:"confidence"`   // 置信度
	Strength     float64         `json:"strength"`     // 信号强度
	Description  string          `json:"description"`  // 信号描述
	Timestamp    int64           `json:"timestamp"`    // 时间戳
}

// FVGSignalType FVG信号类型
type FVGSignalType string

const (
	FVGSignalReaction    FVGSignalType = "reaction"    // FVG反应信号
	FVGSignalFillEntry   FVGSignalType = "fill_entry"  // 填补入场信号
	FVGSignalRejection   FVGSignalType = "rejection"   // 拒绝信号
	FVGSignalPartialFill FVGSignalType = "partial_fill" // 部分填补信号
	FVGSignalBreakthrough FVGSignalType = "breakthrough" // 突破信号
)

var defaultFVGConfig = FVGConfig{
	MinGapPercent:    0.002,  // 0.2%最小缺口
	MaxGapPercent:    0.05,   // 5%最大缺口
	MinVolumeRatio:   1.2,    // 1.2倍最小成交量比率
	MaxAge:           50,     // 50根K线最大存在时间
	MaxTouchCount:    3,      // 最大3次触及
	FillThreshold:    0.8,    // 80%填补阈值
	TimeFrames:       []string{"15m", "1h", "4h"},
	EnableValidation: true,
	QualityThreshold: 0.6,    // 60%质量阈值
	RequireVolConf:   false,  // 不强制要求成交量确认
}

// ====================== 斐波纳契分析相关数据结构 ======================

// FibonacciData 斐波纳契分析主数据结构
type FibonacciData struct {
	Retracements []*FibRetracement `json:"retracements"` // 回调分析
	Extensions   []*FibExtension   `json:"extensions"`   // 扩展分析
	Clusters     []*FibCluster     `json:"clusters"`     // 斐波聚集区
	GoldenPocket *GoldenPocket     `json:"golden_pocket"` // 0.618黄金口袋
	Statistics   *FibStatistics    `json:"statistics"`   // 统计信息
	Config       FibonacciConfig   `json:"config"`       // 配置信息
}

// FibRetracement 斐波纳契回调
type FibRetracement struct {
	ID           string         `json:"id"`
	StartPoint   PricePoint     `json:"start_point"`   // 趋势起点
	EndPoint     PricePoint     `json:"end_point"`     // 趋势终点
	TrendType    TrendType      `json:"trend_type"`    // 趋势类型
	Levels       []FibLevel     `json:"levels"`        // 斐波纳契级别
	Quality      FibQuality     `json:"quality"`       // 质量评估
	Strength     float64        `json:"strength"`      // 强度评分
	Age          int            `json:"age"`           // 存在时间
	IsActive     bool           `json:"is_active"`     // 是否活跃
	TouchCount   map[float64]int `json:"touch_count"`  // 各级别触及次数
	CreatedAt    int64          `json:"created_at"`    // 创建时间
}

// FibExtension 斐波纳契扩展
type FibExtension struct {
	ID          string      `json:"id"`
	BaseWave    PriceWave   `json:"base_wave"`     // 基准波段
	ReturnWave  PriceWave   `json:"return_wave"`   // 回调波段
	Levels      []FibLevel  `json:"levels"`        // 扩展级别
	Quality     FibQuality  `json:"quality"`       // 质量评估
	Confidence  float64     `json:"confidence"`    // 置信度
	IsProjected bool        `json:"is_projected"`  // 是否为预测
}

// FibCluster 斐波聚集区
type FibCluster struct {
	ID           string    `json:"id"`
	CenterPrice  float64   `json:"center_price"`   // 中心价位
	PriceRange   PriceRange `json:"price_range"`   // 价格范围
	Density      float64   `json:"density"`        // 密度评分
	LevelCount   int       `json:"level_count"`    // 包含级别数量
	Sources      []string  `json:"sources"`        // 来源（回调/扩展ID）
	Importance   float64   `json:"importance"`     // 重要性评分
	TouchHistory []TouchEvent `json:"touch_history"` // 触及历史
}

// GoldenPocket 0.618黄金口袋
type GoldenPocket struct {
	ID            string      `json:"id"`
	PriceRange    PriceRange  `json:"price_range"`     // 价格范围(0.618-0.65)
	CenterPrice   float64     `json:"center_price"`    // 中心价位
	Quality       FibQuality  `json:"quality"`         // 质量评估
	Strength      float64     `json:"strength"`        // 强度评分
	TrendContext  TrendType   `json:"trend_context"`   // 趋势背景
	VolumeProfile VolumeInfo  `json:"volume_profile"`  // 成交量分析
	TouchEvents   []TouchEvent `json:"touch_events"`   // 触及事件
	IsActive      bool        `json:"is_active"`       // 是否活跃
	LastUpdate    int64       `json:"last_update"`     // 最后更新
}

// FibLevel 斐波纳契级别
type FibLevel struct {
	Ratio         float64       `json:"ratio"`          // 斐波比率
	Price         float64       `json:"price"`          // 价格位置
	LevelType     FibLevelType  `json:"level_type"`     // 级别类型
	Importance    float64       `json:"importance"`     // 重要性
	TouchCount    int           `json:"touch_count"`    // 触及次数
	LastTouch     int64         `json:"last_touch"`     // 最后触及时间
	Reaction      ReactionData  `json:"reaction"`       // 反应数据
	IsGoldenRatio bool          `json:"is_golden_ratio"` // 是否黄金比率
}

// PricePoint 价格点
type PricePoint struct {
	Price     float64 `json:"price"`
	Timestamp int64   `json:"timestamp"`
	Index     int     `json:"index"`  // K线索引
}

// PriceWave 价格波段
type PriceWave struct {
	StartPoint PricePoint `json:"start_point"`
	EndPoint   PricePoint `json:"end_point"`
	Direction  WaveDirection `json:"direction"`
	Length     float64    `json:"length"`     // 波段长度
	Duration   int64      `json:"duration"`   // 持续时间
}

// PriceRange 价格范围
type PriceRange struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// TouchEvent 触及事件
type TouchEvent struct {
	Price     float64     `json:"price"`
	Timestamp int64       `json:"timestamp"`
	Reaction  ReactionType `json:"reaction"`  // 反应类型
	Volume    float64     `json:"volume"`    // 成交量
	Strength  float64     `json:"strength"`  // 反应强度
}

// ReactionData 反应数据
type ReactionData struct {
	Type          ReactionType `json:"type"`
	Strength      float64      `json:"strength"`      // 反应强度 0-100
	Duration      int          `json:"duration"`      // 反应持续时间(分钟)
	VolumeSpike   float64      `json:"volume_spike"`  // 成交量激增倍数
	PriceMovement float64      `json:"price_movement"` // 价格移动幅度
}

// VolumeInfo 成交量信息
type VolumeInfo struct {
	AverageVolume  float64 `json:"average_volume"`
	CurrentVolume  float64 `json:"current_volume"`
	VolumeRatio    float64 `json:"volume_ratio"`
	SpikesCount    int     `json:"spikes_count"`
}

// FibStatistics 斐波纳契统计信息
type FibStatistics struct {
	TotalRetracements int     `json:"total_retracements"`
	ActiveRetracements int    `json:"active_retracements"`
	SuccessRate       float64 `json:"success_rate"`        // 成功率
	AvgReactionTime   float64 `json:"avg_reaction_time"`   // 平均反应时间
	AvgStrength       float64 `json:"avg_strength"`        // 平均强度
	GoldenRatioHits   int     `json:"golden_ratio_hits"`   // 黄金比率命中
	ClusterCount      int     `json:"cluster_count"`       // 聚集区数量
	HighQualityCount  int     `json:"high_quality_count"`  // 高质量数量
}

// FibonacciConfig 斐波纳契配置
type FibonacciConfig struct {
	MinTrendLength    float64   `json:"min_trend_length"`     // 最小趋势长度(%)
	MaxRetracementAge int       `json:"max_retracement_age"`  // 最大回调存在时间
	TouchSensitivity  float64   `json:"touch_sensitivity"`   // 触及敏感度
	QualityThreshold  float64   `json:"quality_threshold"`   // 质量阈值
	ClusterDistance   float64   `json:"cluster_distance"`    // 聚集距离(%)
	GoldenPocketRange [2]float64 `json:"golden_pocket_range"` // 黄金口袋范围
	EnableExtensions  bool      `json:"enable_extensions"`   // 启用扩展分析
	VolumeWeight      float64   `json:"volume_weight"`       // 成交量权重
	DefaultRatios     []float64 `json:"default_ratios"`      // 默认比率
}

// 枚举类型定义
type TrendType int
type FibQuality int
type FibLevelType int
type WaveDirection int
type ReactionType int

// TrendType 趋势类型
const (
	TrendUpward TrendType = iota
	TrendDownward
	TrendSideways
)

// FibQuality 斐波质量
const (
	FibQualityHigh FibQuality = iota
	FibQualityMedium
	FibQualityLow
)

// FibLevelType 斐波级别类型
const (
	FibLevelRetracement FibLevelType = iota
	FibLevelExtension
	FibLevelProjection
)

// WaveDirection 波段方向
const (
	WaveUp WaveDirection = iota
	WaveDown
)

// ReactionType 反应类型
const (
	ReactionBounce ReactionType = iota // 反弹
	ReactionBreak                      // 突破
	ReactionConsolidation              // 整固
	ReactionRejection                  // 拒绝
)

// FibSignal 斐波纳契交易信号
type FibSignal struct {
	ID           string           `json:"id"`
	Type         FibSignalType    `json:"type"`
	Action       SignalAction     `json:"action"`
	Price        float64          `json:"price"`
	Level        float64          `json:"level"`        // 触及的斐波级别
	Confidence   float64          `json:"confidence"`   // 置信度
	Strength     float64          `json:"strength"`     // 信号强度
	EntryPrice   float64          `json:"entry_price"`  // 建议入场价
	StopLoss     float64          `json:"stop_loss"`    // 止损价
	TakeProfit   []float64        `json:"take_profit"`  // 止盈价(多个目标)
	RiskReward   float64          `json:"risk_reward"`  // 风险收益比
	Context      string           `json:"context"`      // 信号背景
	Source       string           `json:"source"`       // 信号来源
	Quality      SignalQuality    `json:"quality"`      // 信号质量
	Timestamp    int64            `json:"timestamp"`    // 生成时间
}

// FibSignalType 斐波信号类型
type FibSignalType string

const (
	FibSignalGoldenPocket FibSignalType = "golden_pocket"  // 黄金口袋信号
	FibSignalBounce       FibSignalType = "bounce"         // 反弹信号
	FibSignalBreakout     FibSignalType = "breakout"       // 突破信号
	FibSignalCluster      FibSignalType = "cluster"        // 聚集区信号
	FibSignalExtension    FibSignalType = "extension"      // 扩展信号
)

// 默认斐波纳契配置
var defaultFibonacciConfig = FibonacciConfig{
	MinTrendLength:    0.03,  // 3%最小趋势长度
	MaxRetracementAge: 100,   // 100根K线最大存在时间
	TouchSensitivity:  0.002, // 0.2%触及敏感度
	QualityThreshold:  0.6,   // 60%质量阈值
	ClusterDistance:   0.005, // 0.5%聚集距离
	GoldenPocketRange: [2]float64{0.618, 0.65}, // 黄金口袋范围
	EnableExtensions:  true,  // 启用扩展分析
	VolumeWeight:      0.3,   // 30%成交量权重
	DefaultRatios:     []float64{0.236, 0.382, 0.5, 0.618, 0.786, 1.0, 1.272, 1.618, 2.618}, // 标准斐波比率
}

