package microstructure

import (
	"sync"
	"time"
)

// ===== CVD分析相关结构 =====

// CVDData CVD（累积成交量增量）分析数据
type CVDData struct {
	SpotCVD1H     float64   `json:"spot_cvd_1h_usd"`     // 现货1小时净买入量（USD）
	FuturesCVD1H  float64   `json:"futures_cvd_1h_usd"`  // 合约1小时净买入量（USD）
	CVDDivergence string    `json:"cvd_divergence"`      // 背离信号
	Signal        string    `json:"signal"`              // 综合信号
	LastUpdate    time.Time `json:"last_update"`         // 最后更新时间
	IsStale       bool      `json:"is_stale"`            // 数据是否过期
}

// ===== V2.0 增量数据结构 =====

// CVDDelta5m 5分钟CVD增量数据（V2.0核心结构）
type CVDDelta5m struct {
	PriceDeltaPct       float64   `json:"price_delta_pct"`        // 价格涨跌幅百分比
	SpotCVDDeltaUSD     float64   `json:"spot_cvd_delta_usd"`     // 现货5分钟CVD增量（USD）
	FuturesCVDDeltaUSD  float64   `json:"futures_cvd_delta_usd"`  // 合约5分钟CVD增量（USD）
	OIDeltaPct          float64   `json:"oi_delta_pct"`           // 持仓量5分钟变化百分比
	CandleIntent        string    `json:"candle_intent"`          // K线意图推断
	VolumeDelta         float64   `json:"volume_delta"`           // 成交量变化
	VolumeRatio         float64   `json:"volume_ratio"`           // 成交量相对平均值倍数
	PeriodStartTime     time.Time `json:"period_start_time"`      // 周期开始时间
	PeriodEndTime       time.Time `json:"period_end_time"`        // 周期结束时间
	DataQuality         float64   `json:"data_quality"`           // 数据质量评分(0-1)
}

// MacroTrendData 宏观趋势数据（保留原有1小时数据）
type MacroTrendData struct {
	SpotCVD1H          float64   `json:"spot_cvd_1h_usd"`        // 现货1小时净买入量
	FuturesCVD1H       float64   `json:"futures_cvd_1h_usd"`     // 合约1小时净买入量
	CVDDivergence4H    bool      `json:"cvd_divergence_4h"`      // 4小时级别背离
	MarketRegime       string    `json:"market_regime"`          // 市场状态
	TrendStrength      float64   `json:"trend_strength"`         // 趋势强度
	DominantDirection  string    `json:"dominant_direction"`     // 主导方向
	ConfidenceLevel    float64   `json:"confidence_level"`       // 置信度
	LastUpdate         time.Time `json:"last_update"`            // 最后更新时间
}

// CandleIntentType K线意图类型常量
const (
	CandleIntentBullishConfirm     = "bullish_confirm"       // 多头确认
	CandleIntentBearishConfirm     = "bearish_confirm"       // 空头确认
	CandleIntentFakePumpRetail     = "fake_pump_retail_driven"  // 散户推动假突破
	CandleIntentFakeDumpRetail     = "fake_dump_retail_driven"  // 散户推动假跌破
	CandleIntentSmartMoneyAccum    = "smart_money_accumulation" // 主力吸筹
	CandleIntentSmartMoneyDistrib  = "smart_money_distribution" // 主力派发
	CandleIntentConsolidation      = "consolidation"         // 整理
	CandleIntentMixedSignals       = "mixed_signals"         // 混合信号
	CandleIntentDataInsufficient   = "data_insufficient"     // 数据不足
)

// CandleIntentAnalysis K线意图分析结果（V2.0增强版）
type CandleIntentAnalysis struct {
	Intent          string             `json:"intent"`           // 主要意图类型
	Confidence      float64            `json:"confidence"`       // 置信度评分(0-1)
	SecondaryIntent string             `json:"secondary_intent"` // 次要意图（如果存在）
	Strength        float64            `json:"strength"`         // 信号强度
	Factors         map[string]float64 `json:"factors"`          // 各因子贡献度
}

// CVDDelta CVD增量记录
type CVDDelta struct {
	Timestamp time.Time `json:"timestamp"`
	DeltaUSD  float64   `json:"delta_usd"` // USD价值增量（正数=买入，负数=卖出）
}

// PriceSnapshot 价格快照
type PriceSnapshot struct {
	Timestamp time.Time
	Price     float64
}

// CVDCalculator CVD计算器（滑动窗口）
type CVDCalculator struct {
	mu               sync.RWMutex
	symbol           string
	spotDeltas       []CVDDelta    // 现货成交增量记录（滑动窗口）
	futuresDeltas    []CVDDelta    // 合约成交增量记录
	windowDuration   time.Duration // 窗口大小（默认1小时）
	lastCleanup      time.Time     // 最后清理时间
	currentSpotCVD   float64       // 当前现货CVD
	currentFuturesCVD float64      // 当前合约CVD
	
	// V2.0 5分钟增量追踪
	last5mSpotCVD     float64       // 5分钟前的现货CVD
	last5mFuturesCVD  float64       // 5分钟前的合约CVD
	last5mSnapshot    time.Time     // 最后5分钟快照时间
	fiveMinuteCache   map[string]*CVDDelta5m // 5分钟增量数据缓存
	priceHistory      []PriceSnapshot        // 价格历史用于计算增量
}

// ===== 盘口分析相关结构 =====

// OrderBookData 盘口分析数据（V2.0增强）
type OrderBookData struct {
	ImbalanceRatio     float64    `json:"imbalance_ratio"`      // 买卖失衡比例 (-1 到 1)
	NearestResistance  *WallInfo  `json:"nearest_resistance"`   // 最近阻力墙
	NearestSupport     *WallInfo  `json:"nearest_support"`      // 最近支撑墙
	BidPressure        float64    `json:"bid_pressure"`         // 买方压力强度
	AskPressure        float64    `json:"ask_pressure"`         // 卖方压力强度
	// V2.0 新增字段
	ImbalanceTrend     string     `json:"imbalance_trend"`      // 失衡趋势方向
	PressureDelta5m    float64    `json:"pressure_delta_5m"`    // 5分钟压力变化
	SpoofingRisk       float64    `json:"spoofing_risk"`        // 虚假挂单风险评分(0-1)
	LiquidityScore     float64    `json:"liquidity_score"`      // 流动性评分(0-1)
	WallChangeCount5m  int        `json:"wall_change_count_5m"` // 5分钟内墙变化次数
	LastUpdate         time.Time  `json:"last_update"`          // 最后更新时间
	IsStale            bool       `json:"is_stale"`             // 数据是否过期
}

// WallInfo 挂单墙信息（V2.0扩展）
type WallInfo struct {
	Price          float64   `json:"price"`           // 价格
	StrengthUSD    float64   `json:"strength_usd"`    // 强度（USD金额）
	IsSolid        bool      `json:"is_solid"`        // 是否为实墙
	Distance       float64   `json:"distance"`        // 距离当前价格的百分比
	LevelCount     int       `json:"level_count"`     // 涉及档位数量
	// V2.0 新增字段
	StabilityScore float64   `json:"stability_score"`  // 稳定性评分(0-1)
	ExistenceDuration time.Duration `json:"existence_duration"` // 存在时长
	FlickerCount   int       `json:"flicker_count"`    // 闪烁次数(出现消失次数)
	LastSeen       time.Time `json:"last_seen"`        // 最后见到时间
	FirstSeen      time.Time `json:"first_seen"`       // 首次见到时间
	AverageSize    float64   `json:"average_size"`     // 平均挂单大小
	MaxSize        float64   `json:"max_size"`         // 最大挂单大小
	MinSize        float64   `json:"min_size"`         // 最小挂单大小
}

// OrderBookLevel 盘口档位
type OrderBookLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

// ===== 综合数据结构 =====

// MicrostructureData 微观结构综合数据
type MicrostructureData struct {
	Symbol      string          `json:"symbol"`
	CVDAnalysis *CVDData        `json:"cvd_analysis"`
	OrderBook   *OrderBookData  `json:"orderbook"`
	LastUpdate  time.Time       `json:"last_update"`
}

// DataQualityInfo 数据质量信息（V2.0新增）
type DataQualityInfo struct {
	CVDReliability       float64   `json:"cvd_reliability"`        // CVD数据可靠性(0-1)
	OrderBookReliability float64   `json:"orderbook_reliability"`  // 盘口数据可靠性(0-1)
	OIReliability        float64   `json:"oi_reliability"`         // OI数据可靠性(0-1)
	OverallScore         float64   `json:"overall_score"`          // 总体质量评分(0-1)
	LastDataUpdate       time.Time `json:"last_data_update"`       // 最后数据更新时间
	DataLagMs            int64     `json:"data_lag_ms"`            // 数据延迟(毫秒)
	Status               string    `json:"status"`                 // 数据状态: "正常", "延迟", "异常"
}

// ===== 配置结构 =====

// MicrostructureConfig 微观结构分析配置
type MicrostructureConfig struct {
	// CVD配置
	CVDWindowDuration    time.Duration `json:"cvd_window_duration"`    // CVD计算窗口（默认1小时）
	CVDCleanupInterval   time.Duration `json:"cvd_cleanup_interval"`   // 清理间隔（默认5分钟）
	CVDDivergenceThreshold float64     `json:"cvd_divergence_threshold"` // 背离阈值（默认0.1）
	
	// 盘口配置
	OrderBookDepth       int           `json:"orderbook_depth"`        // 盘口深度（默认20）
	WallThresholdMultiple float64     `json:"wall_threshold_multiple"` // 挂单墙阈值倍数（默认5.0）
	ImbalanceSmoothing   int           `json:"imbalance_smoothing"`    // 失衡平滑周期（默认5）
	
	// 数据源配置
	SpotWsURL            string        `json:"spot_ws_url"`            // 现货WebSocket地址
	FuturesWsURL         string        `json:"futures_ws_url"`         // 合约WebSocket地址
	ReconnectInterval    time.Duration `json:"reconnect_interval"`     // 重连间隔
	StaleDataThreshold   time.Duration `json:"stale_data_threshold"`   // 数据过期阈值
	
	// 功能开关
	EnableCVDAnalysis    bool          `json:"enable_cvd_analysis"`    // 启用CVD分析
	EnableOrderBook      bool          `json:"enable_orderbook"`       // 启用盘口分析
	EnableSignalDetection bool         `json:"enable_signal_detection"` // 启用信号检测
}

// DefaultMicrostructureConfig 默认配置
func DefaultMicrostructureConfig() *MicrostructureConfig {
	return &MicrostructureConfig{
		CVDWindowDuration:      time.Hour,
		CVDCleanupInterval:     5 * time.Minute,
		CVDDivergenceThreshold: 0.1,
		
		OrderBookDepth:        20,
		WallThresholdMultiple: 5.0,
		ImbalanceSmoothing:    5,
		
		SpotWsURL:    "wss://stream.binance.com:9443/ws/",
		FuturesWsURL: "wss://fstream.binance.com/ws/",
		ReconnectInterval:    30 * time.Second,
		StaleDataThreshold:   2 * time.Minute,
		
		EnableCVDAnalysis:     true,
		EnableOrderBook:       true,
		EnableSignalDetection: true,
	}
}

// ===== 常量定义 =====

// CVD信号类型
const (
	CVDSignalNeutral          = "neutral"            // 中性
	CVDSignalBullishAbsorption = "bullish_absorption" // 看涨吸筹
	CVDSignalBearishDistribution = "bearish_distribution" // 看跌派发
	CVDSignalBullishConfirmation = "bullish_confirmation" // 多头确认
	CVDSignalBearishConfirmation = "bearish_confirmation" // 空头确认
	CVDSignalSpotLeading      = "spot_leading"        // 现货领先
	CVDSignalFuturesLeading   = "futures_leading"     // 合约领先
)