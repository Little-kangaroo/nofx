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
)
