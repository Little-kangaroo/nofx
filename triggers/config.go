package triggers

// TriggerConfig 触发器配置（支持per-symbol和per-regime动态配置）
type TriggerConfig struct {
	// === Swing Point 配置 ===
	SwingLookback int `json:"swing_lookback"` // Swing点回看周期（默认：20）
	SwingLeft     int `json:"swing_left"`     // Swing左侧确认周期（默认：2）
	SwingRight    int `json:"swing_right"`    // Swing右侧确认周期（默认：2）

	// === SFP (Sweep Failure Pattern) 配置 ===
	WickRatioMin float64 `json:"wick_ratio_min"` // 最小影线占比（默认：0.45）
	WickRatioMax float64 `json:"wick_ratio_max"` // 最大影线占比（默认：0.85）
	SweepAtrMin  float64 `json:"sweep_atr_min"`  // 最小扫单幅度（ATR倍数，默认：0.10）
	SweepAtrMax  float64 `json:"sweep_atr_max"`  // 最大扫单幅度（ATR倍数，默认：2.0）

	// === Engulf（吞没）配置 ===
	BodyAtrMin  float64 `json:"body_atr_min"`  // 最小实体大小（ATR倍数，默认：0.30）
	BodyAtrMax  float64 `json:"body_atr_max"`  // 最大实体大小（ATR倍数，默认：5.0）
	ClosePosMax float64 `json:"close_pos_max"` // 收盘位置最大值（0-1，默认：0.35，越低越靠近极值）

	// === IBB (Inside Bar Breakout) 配置 ===
	BreakAtrMin float64 `json:"break_atr_min"` // 最小突破幅度（ATR倍数，默认：0.15）
	BreakAtrMax float64 `json:"break_atr_max"` // 最大突破幅度（ATR倍数，默认：3.0）

	// === MomoIgnition（动能点火）配置 ===
	RangeAtrMin      float64 `json:"range_atr_min"`      // 最小波幅（ATR倍数，默认：1.2）
	RangeAtrMax      float64 `json:"range_atr_max"`      // 最大波幅（ATR倍数，默认：5.0）
	CloseNearExtreme float64 `json:"close_near_extreme"` // 收盘靠近极值阈值（默认：0.20）

	// 🔥 P0新增：EDGE（结构边缘触碰）配置
	EdgeTouchBps      float64 `json:"edge_touch_bps"`       // EDGE触碰基点阈值（默认：12）
	EdgeTouchAtrMult  float64 `json:"edge_touch_atr_mult"`  // EDGE触碰ATR倍数阈值（默认：0.20）
	EdgeMinStrengthZ  float64 `json:"edge_min_strength_z"`  // EDGE最小结构强度Z分数（默认：-0.2）

	// 🔥 P0新增：BO_RETEST（突破回测）配置
	BoLookbackBars int     `json:"bo_lookback_bars"` // BO_RETEST回看K线数量（默认：30）
	BoMaxAgeBars   int     `json:"bo_max_age_bars"`  // BO_RETEST最大年龄（默认：6）
	BoBreakMinAtr  float64 `json:"bo_break_min_atr"` // BO_RETEST最小突破幅度（ATR倍数，默认：0.20）
	BoRetestTolBps float64 `json:"bo_retest_tol_bps"` // BO_RETEST回测容差（基点，默认：10）
	BoConfirmMinQ  float64 `json:"bo_confirm_min_q"`  // BO_RETEST确认最小质量（默认：0.30）

	// === 通用量能配置 ===
	VolZMin float64 `json:"vol_z_min"` // 最小量能Z分数（默认：0.8）

	// === 输出控制配置 ===
	MaxFlagsPerBar int     `json:"max_flags_per_bar"` // 每根K线最大触发器数量（默认：3）
	QualityMin     float64 `json:"quality_min"`       // 最小质量阈值（默认：0.55）
	BorderlineMin  float64 `json:"borderline_min"`    // 边界质量阈值（默认：0.50）
}

// DefaultConfig 返回默认配置
func DefaultConfig() TriggerConfig {
	return TriggerConfig{
		// Swing配置
		// 🔥 P1-02修复：SwingLookback 从 20 提升到 120，避免趋势行情中找不到Swing点
		SwingLookback: 120,
		SwingLeft:     2,
		SwingRight:    2,

		// SFP配置
		WickRatioMin: 0.45,
		WickRatioMax: 0.85,
		SweepAtrMin:  0.10,
		SweepAtrMax:  2.0,

		// Engulf配置
		// 🔥 P0修复：BodyAtrMax 5.0→1.8（解决质量刻度压扁问题）
		BodyAtrMin:  0.30,
		BodyAtrMax:  1.8,
		ClosePosMax: 0.35,

		// IBB配置
		// 🔥 P0修复：BreakAtrMax 3.0→1.2（让 breakScore 回到可用区间）
		BreakAtrMin: 0.15,
		BreakAtrMax: 1.2,

		// MomoIgnition配置
		// 🔥 P0修复：RangeAtrMax 5.0→2.8（保留强动量定义，但避免分数过低）
		RangeAtrMin:      1.2,
		RangeAtrMax:      2.8,
		CloseNearExtreme: 0.20,

		// 🔥 P0新增：EDGE配置
		EdgeTouchBps:     12,   // 触碰基点阈值
		EdgeTouchAtrMult: 0.20, // 触碰ATR倍数阈值
		EdgeMinStrengthZ: -0.2, // 最小结构强度Z分数

		// 🔥 P0新增：BO_RETEST配置
		BoLookbackBars: 30,   // 回看K线数量
		BoMaxAgeBars:   6,    // 最大年龄
		BoBreakMinAtr:  0.20, // 最小突破幅度（ATR倍数）
		BoRetestTolBps: 10,   // 回测容差（基点）
		BoConfirmMinQ:  0.30, // 确认最小质量

		// 量能配置
		VolZMin: 0.8,

		// 输出控制
		// 🔥 P0修复：QualityMin 0.55→0.50（让 Gate4 有机会 PASS）
		// - QualityMin=0.50（Strict层，降低门槛让 q 能进入可用区间）
		// - BorderlineMin=0.20（AI层，提高召回率用于Gate3触发窗口，保持不变）
		MaxFlagsPerBar: 3,
		QualityMin:     0.50,
		BorderlineMin:  0.20,
	}
}

// VolatilityAdjustedConfig 根据波动率调整配置（波动分段）
func VolatilityAdjustedConfig(baseConfig TriggerConfig, volRegime string) TriggerConfig {
	cfg := baseConfig

	switch volRegime {
	case "low":
		// 🔥 P0修复：低波动 QualityMin 0.60→0.52（防止 strict 层长期为空）
		// 低波动：提高阈值，减少噪声触发
		cfg.BodyAtrMin *= 1.2
		cfg.BreakAtrMin *= 1.2
		cfg.RangeAtrMin *= 1.1
		cfg.QualityMin = 0.52

	case "high":
		// 🔥 P0修复：高波动 QualityMin 0.50→0.48（保持更宽容）
		// 高波动：降低阈值，避免完全无触发
		cfg.BodyAtrMin *= 0.8
		cfg.BreakAtrMin *= 0.8
		cfg.RangeAtrMin *= 0.9
		cfg.QualityMin = 0.48

	case "normal":
		// 正常波动：使用默认配置
		// 无需调整

	default:
		// 未知状态：使用默认配置
	}

	return cfg
}
