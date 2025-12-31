package protect

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	CheckIntervalSec         int64 // 10秒检查间隔
	EnableSoftStopInFastLoop bool  // 是否在快速循环中启用软止损（默认false）
}

// Config 锁盈引擎配置
type Config struct {
	// ROI ProfitFloor（基于杠杆浮盈%）
	ROILockTrigger     float64 // 0.05 - 5%浮盈开始锁盈
	ROILockFastTrigger float64 // 0.10 - 10%浮盈快速通道（跳过时间限制）
	FloorPriceBps      float64 // 20 - 盈利地板（20bps约2%@10x）

	// 🎯 V-19.0: ROI止盈里程碑（ROI-based automatic take-profit）
	TPMilestones map[float64]float64 // ROI阈值 -> 止盈百分比 (如：0.10 -> 0.15表示10%ROI触发15%止盈)

	// Break-even保护
	BreakEvenTriggerR float64 // 0.4 - 达到0.4R触发BE保护
	BreakEvenPadR     float64 // 0.05 - BE保护缓冲（0.05R）

	// Protect mode（影响软止损的pad收缩）
	ProtectEnable     bool    // 是否启用保护模式
	ProtectTriggerR   float64 // 0.3 - 达到0.3R触发保护模式
	ProtectTimeMinSec int64   // 180 - 保护模式最小持仓时间（秒）
	ProtectPadShrink  float64 // 0.7 - pad收缩系数
	ProtectShrinkMinR float64 // 0.5 - R<0.5禁止收缩

	// Execution & debounce（执行控制与防抖）
	StopDistanceMinTicks int64 // 3 - 止损最小距离（tick数）
	SafetyTicks          int64 // 2 - 安全缓冲（tick数）
	MinTickMoveToUpdate  int64 // 3 - 最小移动距离才更新（tick数）
	UpdateCooldownSec    int64 // 25 - 更新冷却期（秒）

	// R_lock milestones（里程碑锁盈 - 基础档）
	RLockMilestones []float64 // [1.0, 1.5, 2.0, 3.0]
	RLockAt         []float64 // [0.5, 0.8, 1.2, 2.0]

	// R_lock milestones（里程碑锁盈 - 激进档）
	RLockMilestonesAggr []float64 // [0.25, 0.5, 1.0, 1.5, 2.0]
	RLockAtAggr         []float64 // [0.10, 0.25, 0.70, 1.20, 1.60]

	Scheduler SchedulerConfig
}

// DefaultConfig 返回默认配置（对齐V-18.0规则）
func DefaultConfig() Config {
	c := Config{
		// ROI锁盈
		ROILockTrigger:     0.05,
		ROILockFastTrigger: 0.10,
		FloorPriceBps:      20,

		// 🎯 V-19.0: ROI止盈里程碑
		TPMilestones: map[float64]float64{
			0.10: 0.15, // 10% ROI → 15% 止盈
			0.20: 0.30, // 20% ROI → 30% 止盈
			0.50: 0.70, // 50% ROI → 70% 止盈
		},

		// Break-even
		BreakEvenTriggerR: 0.4,
		BreakEvenPadR:     0.05,

		// Protect mode
		ProtectEnable:     true,
		ProtectTriggerR:   0.3,
		ProtectTimeMinSec: 180,
		ProtectPadShrink:  0.7,
		ProtectShrinkMinR: 0.5,

		// Execution
		StopDistanceMinTicks: 3,
		SafetyTicks:          2,
		MinTickMoveToUpdate:  3,
		UpdateCooldownSec:    25,

		// R_lock基础档（HTF_swing + A级）
		RLockMilestones: []float64{1.0, 1.5, 2.0, 3.0},
		RLockAt:         []float64{0.5, 0.8, 1.2, 2.0},

		// R_lock激进档（LTF_scalp或B级）
		RLockMilestonesAggr: []float64{0.25, 0.5, 1.0, 1.5, 2.0},
		RLockAtAggr:         []float64{0.10, 0.25, 0.70, 1.20, 1.60},
	}

	// 调度器配置
	c.Scheduler = SchedulerConfig{
		CheckIntervalSec:         10,
		EnableSoftStopInFastLoop: false, // 默认关闭，避免高频追踪结构
	}

	return c
}
