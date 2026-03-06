package direction

// DirectionSide 表示方向裁决结果
type DirectionSide string

const (
	SideLong    DirectionSide = "LONG"
	SideShort   DirectionSide = "SHORT"
	SideNeutral DirectionSide = "NEUTRAL"
	SideUnknown DirectionSide = "UNKNOWN"
)

// DirectionArbitration 方向裁决输出（SSOT）
type DirectionArbitration struct {
	PlanSide       DirectionSide      `json:"plan_side"`        // 计划方向（SSOT）
	BlockEntry     bool               `json:"block_entry"`      // 是否禁止新开仓
	BlockReason    string             `json:"block_reason,omitempty"` // 禁止原因（供AI展示：SPOOF_HIGH/OF_STALE/MACRO_OPPOSE/STRUCT_PROTECT/...）
	Confidence     float64            `json:"confidence"`       // 相对置信度 [0,1]，供AI感知方向把握度
	SignalStrength float64            `json:"signal_strength"`  // 绝对信号强度 = |delta|，供AI区分弱/强信号（0.05=弱，0.30+=强）
	Delta          float64            `json:"-"`                // ScoreLong - ScoreShort（内部使用）
	OFDir          float64            `json:"-"`                // 订单流方向（内部使用）
	StructDir      float64            `json:"-"`                // 结构方向（内部使用）
	OFQuality      float64            `json:"-"`                // 订单流质量（内部使用）
	Flags          []string           `json:"-"`                // 状态标记（内部使用）
	Debug          map[string]float64 `json:"-"`                // 调试信息（内部使用）
}

// RootSymbolInput 单个symbol的完整输入数据
type RootSymbolInput struct {
	Symbol    string        `json:"symbol"`
	Orderflow Orderflow     `json:"订单流分析"`
	MTF       MTFAnalysis   `json:"多时间框架分析"`
}

// Orderflow 订单流分析数据
type Orderflow struct {
	Quality Quality `json:"数据质量"`
	Macro   Macro   `json:"宏观资金趋势"`
	Micro5m Micro5m `json:"本周期博弈_5m"`
	OB      Orderbook `json:"盘口结构_v2"`
}

// Quality 数据质量
type Quality struct {
	Status               string  `json:"status"`
	OverallScore         float64 `json:"overall_score"`
	DataLagMs            int64   `json:"data_lag_ms"`
	UpdateIntervalS      int64   `json:"update_interval_s"`
	CvdReliability       float64 `json:"cvd_reliability"`
	OIReliability        float64 `json:"oi_reliability"`
	OrderbookReliability float64 `json:"orderbook_reliability"`
}

// Macro 宏观资金趋势
type Macro struct {
	TrendAlignment   string  `json:"trend_alignment"`
	SignalStrength   float64 `json:"signal_strength"`
	ConfidenceLevel  float64 `json:"confidence_level"`
	MarketRegime     string  `json:"market_regime"`
	DominantDirection string `json:"dominant_direction"` // 主导方向（bearish_distribution / bullish_accumulation 等）
	SpotCvd1hUSD     float64 `json:"spot_cvd_1h_usd"`   // 1小时现货累计delta（USD）
	FuturesCvd1hUSD  float64 `json:"futures_cvd_1h_usd"` // 1小时期货累计delta（USD）
	CvdDivergence    bool    `json:"cvd_divergence"`
}

// Micro5m 5分钟微观博弈
type Micro5m struct {
	CandleIntent      string  `json:"candle_intent"`
	FuturesCvdDelta   float64 `json:"futures_cvd_delta_usd"`
	SpotCvdDelta      float64 `json:"spot_cvd_delta_usd"`
	OIDeltaPct        float64 `json:"oi_delta_pct"`
	PriceDeltaPct     float64 `json:"price_delta_pct"`
	VolumeDelta       float64 `json:"volume_delta"`
	DataQuality       float64 `json:"data_quality"`
}

// Orderbook 盘口结构
type Orderbook struct {
	ImbalanceRatio   float64 `json:"imbalance_ratio"`
	BidPressure      float64 `json:"bid_pressure"`
	AskPressure      float64 `json:"ask_pressure"`
	LiquidityScore   float64 `json:"liquidity_score"`
	SpoofingRisk     float64 `json:"spoofing_risk"`
	SupportWall      *Wall   `json:"support_wall"`
	ResistanceWall   *Wall   `json:"resistance_wall"`
	WallChange5m     int64   `json:"wall_change_count_5m"`
}

// Wall 墙体结构
type Wall struct {
	StrengthUSD     float64 `json:"strength_usd"`
	DistancePct     float64 `json:"distance_pct"`
	IsSolid         bool    `json:"is_solid"`
	StabilityScore  float64 `json:"stability_score"`
	FlickerCount    int64   `json:"flicker_count"`
}

// MTFAnalysis 多时间框架分析
type MTFAnalysis map[string]TimeframeData

// TimeframeData 单个时间框架数据
// 🔥 优化：使用通道指标替代 SuperTrend + 道氏理论
type TimeframeData struct {
	Channel      ChannelInfo `json:"通道分析数据"` // 通道指标（主要方向判断）
	VPVR         VPVR        `json:"VPVR数据"`    // VPVR（用于平局打破）
	SupertrendDir string     `json:"-"`           // 超级趋势方向（"bullish"/"bearish"，用于4h背景约束）
}

// ChannelInfo 通道信息（简化版，用于方向裁决）
type ChannelInfo struct {
	Direction       string  `json:"direction"`        // "up", "down", "sideways"
	CurrentPosition string  `json:"current_position"` // "Inside", "BreakUp", "BreakDown"
	PriceRatio      float64 `json:"price_ratio"`      // 价格在通道中的位置 0-1
	Quality         float64 `json:"quality"`          // 通道质量 0-1
}

// VPVR Volume Profile Volume Range
type VPVR struct {
	DistToVAHATR float64 `json:"dist_to_vah_atr"`
	DistToVALATR float64 `json:"dist_to_val_atr"`
}
