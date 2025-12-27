package triggers

// 触发器标志常量定义
const (
	// SFP (Sweep Failure Pattern) - 扫单失败形态
	FlagSfpBear = "SFP_BEAR" // 扫高失败（看空）
	FlagSfpBull = "SFP_BULL" // 扫低失败（看多）

	// Engulf - 吞没形态
	FlagEngulfBear = "ENGULF_BEAR" // 阴线吞没（看空）
	FlagEngulfBull = "ENGULF_BULL" // 阳线吞没（看多）

	// IBB (Inside Bar Breakout) - 内包突破
	FlagIbbBear = "IBB_BEAR" // 内包向下突破（看空）
	FlagIbbBull = "IBB_BULL" // 内包向上突破（看多）

	// MomoIgnition - 动能点火
	FlagMomoBear = "MOMO_BEAR" // 动能向下（看空）
	FlagMomoBull = "MOMO_BULL" // 动能向上（看多）

	// 🔥 P0新增：EDGE - 结构边缘触碰
	FlagEdgeBull = "EDGE_BULL" // 支撑边缘触碰（看多）
	FlagEdgeBear = "EDGE_BEAR" // 阻力边缘触碰（看空）

	// 🔥 P0新增：BO_RETEST - 突破回测
	FlagBoRetestBull = "BO_RETEST_BULL" // 向上突破回测（看多）
	FlagBoRetestBear = "BO_RETEST_BEAR" // 向下突破回测（看空）

	// 无触发器
	FlagNone = "NONE"
)

// Kline K线数据结构（复用market包的定义，这里为了避免循环引用单独定义）
type Kline struct {
	OpenTime  int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime int64
}

// TriggerScanResult 触发器扫描结果
type TriggerScanResult struct {
	TF       string             `json:"trigger_tf"`       // 时间框架（固定"5m"）
	Flags    []string           `json:"trigger_flags"`    // 候选触发器列表（最多3个，PostProcess后）
	Primary  string             `json:"trigger_primary"`  // 推荐主触发器（quality最高的）
	Quality  map[string]float64 `json:"trigger_quality"`  // 各触发器质量评分（0~1，PostProcess后）
	KeyLevel float64            `json:"trigger_key_level,omitempty"` // 触发器关联的关键价位
	KeyType  string             `json:"trigger_key_level_type,omitempty"` // 关键位类型

	// 🔥 P0-新增：原始数据（PostProcess前），用于诊断和Gate3触发窗口
	RawFlags   []string           `json:"raw_flags,omitempty"`   // PostProcess前的所有触发器
	RawQuality map[string]float64 `json:"raw_quality,omitempty"` // PostProcess前的所有质量评分

	// 🔥 P0-2新增：分层输出数据（raw/ai/strict三层质量过滤）
	// AI层：轻过滤（BorderlineMin），用于Gate3触发窗口（提高召回率）
	AIFlags   []string           `json:"ai_flags,omitempty"`    // AI层触发器（BorderlineMin过滤）
	AIQuality map[string]float64 `json:"ai_quality,omitempty"`  // AI层质量评分

	// Strict层：严格过滤（QualityMin），用于内部统计和分析（保证精度）
	StrictFlags   []string           `json:"strict_flags,omitempty"`   // Strict层触发器（QualityMin过滤）
	StrictQuality map[string]float64 `json:"strict_quality,omitempty"` // Strict层质量评分
}

// SwingLevels Swing高低点
type SwingLevels struct {
	SwingHigh      float64 // Swing High价格
	SwingLow       float64 // Swing Low价格
	SwingHighIndex int     // Swing High在K线数组中的索引
	SwingLowIndex  int     // Swing Low在K线数组中的索引
	SwingHighTime  int64   // Swing High时间戳
	SwingLowTime   int64   // Swing Low时间戳
}

// TriggerQualityScore 触发器质量评分详情（用于调试和分析）
type TriggerQualityScore struct {
	Flag       string  // 触发器标志
	TotalScore float64 // 总分（0~1）

	// 各子项评分（依触发器类型而不同）
	SubScores map[string]float64 // 例如：{"wick": 0.72, "sweep": 0.65, "closeBack": 0.80}
}

// KeyLevelType 关键位类型枚举
const (
	KeyLevelSwingHigh = "swing_high" // Swing高点
	KeyLevelSwingLow  = "swing_low"  // Swing低点
	KeyLevel4hSR      = "4h_SR"      // 4小时支撑/阻力（可选，未来扩展）
	KeyLevel1hSR      = "1h_SR"      // 1小时支撑/阻力（可选，未来扩展）

	// 🔥 P0-A新增：IBB/Engulf/Momo触发器关键位类型
	KeyLevelIbbBreakHigh      = "ibb_break_high"      // IBB向上突破的母bar高点
	KeyLevelIbbBreakLow       = "ibb_break_low"       // IBB向下突破的母bar低点
	KeyLevelEngulfInvalidHigh = "engulf_invalid_high" // Engulf失效的前bar高点
	KeyLevelEngulfInvalidLow  = "engulf_invalid_low"  // Engulf失效的前bar低点
	KeyLevelMomoInvalidHigh   = "momo_invalid_high"   // Momo失效的点火bar高点
	KeyLevelMomoInvalidLow    = "momo_invalid_low"    // Momo失效的点火bar低点

	// 🔥 P0新增：EDGE触发器关键位类型
	KeyLevelEdgeSupport    = "edge_support"    // EDGE支撑位（VPVR VAL/POC、S/R Support、供需区下沿）
	KeyLevelEdgeResistance = "edge_resistance" // EDGE阻力位（VPVR VAH/POC、S/R Resistance、供需区上沿）

	// 🔥 P0新增：BO_RETEST触发器关键位类型
	KeyLevelBoBreakoutSupport    = "bo_breakout_support"    // BO_RETEST突破支撑位
	KeyLevelBoBreakoutResistance = "bo_breakout_resistance" // BO_RETEST突破阻力位
)

// 🔥 P0新增：TouchInfo EDGE触发器的触碰信息
type TouchInfo struct {
	Level      float64 `json:"level"`       // 关键位价格
	Side       string  `json:"side"`        // 方向："LONG"（支撑）或"SHORT"（阻力）
	DistBps    float64 `json:"dist_bps"`    // 距离（基点，1基点=0.01%）
	DistAtr    float64 `json:"dist_atr"`    // 距离（ATR倍数）
	Source     string  `json:"source"`      // 数据源："VPVR"/"SR"/"SUPPLY_DEMAND"
	StrengthZ  float64 `json:"strength_z"`  // 结构强度Z分数（标准化）
	TimeFrame  string  `json:"timeframe"`   // 时间框架："5m"/"15m"/"30m"/"1h"/"4h"
	KeyType    string  `json:"key_type"`    // 关键位类型（edge_support/edge_resistance）
}

// 🔥 P0新增：LevelInfo BO_RETEST触发器的关键位信息
type LevelInfo struct {
	Level     float64 `json:"level"`      // 关键位价格
	Side      string  `json:"side"`       // 方向："LONG"（支撑）或"SHORT"（阻力）
	Source    string  `json:"source"`     // 数据源："VPVR"/"SR"/"SUPPLY_DEMAND"
	StrengthZ float64 `json:"strength_z"` // 结构强度Z分数（标准化）
	TimeFrame string  `json:"timeframe"`  // 时间框架："5m"/"15m"/"30m"/"1h"/"4h"
	KeyType   string  `json:"key_type"`   // 关键位类型（bo_breakout_support/bo_breakout_resistance）
}

