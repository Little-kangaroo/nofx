package protect

// Side 持仓方向
type Side string

const (
	Long  Side = "LONG"
	Short Side = "SHORT"
)

// TriggerType 止损触发类型
type TriggerType string

const (
	TriggerLast TriggerType = "LAST_PRICE" // 最新成交价触发
	TriggerMark TriggerType = "MARK_PRICE" // 标记价格触发
)

// PositionState 持仓状态（必须持久化的核心数据）
type PositionState struct {
	Symbol string // 交易对
	Side   Side   // 方向（LONG/SHORT）
	Qty    float64 // 持仓数量

	Entry    float64 // 入场价
	Leverage float64 // 杠杆倍数

	InitStop float64 // 初始止损价（必须固化，计算R0的唯一来源）
	PrevStop float64 // 当前止损价（与交易所真实挂单对账回写）

	// 🎯 V-19.0: 止盈管理（ROI-based automatic take-profit）
	PrevTakeProfit     float64 // 当前止盈价（与交易所真实挂单对账回写）
	LastTPUpdateTimeMs int64   // 上次止盈更新时间戳（毫秒）

	OpenTimeMs           int64 // 开仓时间戳（毫秒）
	LastStopUpdateTimeMs int64 // 上次止损更新时间戳（毫秒）

	StopTriggerType TriggerType // 止损触发类型

	// 状态机持久化（避免反复触发判定）
	ROIArmed        bool // ROI锁盈已触发
	BreakEvenArmed  bool // Break-even保护已触发
	RLockStage      int  // R_lock里程碑阶段（0=未触发，1=第1级，2=第2级...）
}

// MarketSnapshot 市场快照（高频循环从PriceCache读取）
type MarketSnapshot struct {
	Symbol    string  // 交易对
	LastPrice float64 // 最新成交价
	MarkPrice float64 // 标记价格
	TickSize  float64 // 最小价格变动单位
	NowMs     int64   // 当前时间戳（毫秒）
}

// FeeModel 交易所费用模型（后端配置，不依赖prompt输入）
type FeeModel struct {
	TakerFeeBps      float64 // Taker手续费（bps，1bp=0.01%）
	SlippageBpsMinor float64 // 滑点估算（bps）
	FundingBps       float64 // 资金费率（可选，bps）
}

// ReasonCode 锁盈决策原因码
type ReasonCode string

const (
	ReasonCooldown     ReasonCode = "COOLDOWN"       // 冷却期内
	ReasonStepTooSmall ReasonCode = "STEP_TOO_SMALL" // 移动距离不足
	ReasonExecGap      ReasonCode = "EXEC_GAP"       // 可执行边界冲突
	ReasonROIArm       ReasonCode = "ROI_LOCK_ARMED" // ROI锁盈触发
	ReasonBreakEven    ReasonCode = "BREAK_EVEN_ARMED" // Break-even触发
	ReasonRLock        ReasonCode = "R_LOCK"         // R_lock里程碑触发
	ReasonNoChange     ReasonCode = "NO_CHANGE"      // 无改善
	ReasonInvalidInput ReasonCode = "INVALID_INPUT"  // 输入数据无效
)

// StopUpdatePlan 止损更新计划（包含armed/stage回写字段）
type StopUpdatePlan struct {
	ShouldUpdate bool    // 是否应该更新止损
	NewStop      float64 // 新止损价

	ExecGap  bool       // 是否存在可执行边界冲突
	Bounds   ExecBounds // 可执行边界
	RefPrice float64    // 参考价格

	// 计算中间值（用于日志和调试）
	RoiUnr float64 // ROI未实现（含杠杆）
	RUnr   float64 // R未实现（相对初始风险）
	R0     float64 // 初始风险单位
	BE     float64 // Break-even价格（含成本）
	Floor  float64 // 盈利地板价格

	Reasons []ReasonCode // 决策原因列表
	Note    string       // 额外说明

	// 🎯 V-19.0: 止盈管理字段
	ShouldUpdateTP bool    // 是否应该更新止盈
	NewTakeProfit  float64 // 新止盈价
	TPReason       string  // 止盈更新原因

	// 状态机回写（即使本次未更新，也需保持armed/stage状态）
	NextROIArmed       bool // 下次ROIArmed状态
	NextBreakEvenArmed bool // 下次BreakEvenArmed状态
	NextRLockStage     int  // 下次RLockStage状态
}

// ExecBounds 可执行边界（止损价格必须在此范围内）
type ExecBounds struct {
	UpperExec float64 // LONG止损必须 <= UpperExec
	LowerExec float64 // SHORT止损必须 >= LowerExec
}
