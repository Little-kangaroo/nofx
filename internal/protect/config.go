package protect

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	CheckIntervalSec         int64 // 10秒检查间隔
	EnableSoftStopInFastLoop bool  // 是否在快速循环中启用软止损（默认false）
}

// Config 锁盈引擎配置（V-20.0 简化版：纯ROI锁盈）
type Config struct {
	// ROI锁盈（唯一触发机制）
	ROILockTrigger float64 // 0.05 - 5%浮盈触发锁盈（无时间约束）
	FloorPriceBps  float64 // 20 - 盈利地板（20bps约2%@10x）

	// 🎯 V-19.0: ROI止盈里程碑（ROI-based automatic take-profit）
	TPMilestones map[float64]float64 // ROI阈值 -> 止盈百分比 (如：0.10 -> 0.15表示10%ROI触发15%止盈)

	// 执行控制与防抖
	StopDistanceMinTicks int64 // 3 - 止损最小距离（tick数）
	SafetyTicks          int64 // 2 - 安全缓冲（tick数）
	MinTickMoveToUpdate  int64 // 3 - 最小移动距离才更新（tick数）
	UpdateCooldownSec    int64 // 25 - 更新冷却期（秒）

	Scheduler SchedulerConfig
}

// DefaultConfig 返回默认配置（V-20.0 简化版：纯ROI锁盈）
func DefaultConfig() Config {
	c := Config{
		// ROI锁盈（无时间约束）
		ROILockTrigger: 0.05, // 5%触发
		FloorPriceBps:  20,   // 盈利地板

		// 🎯 V-19.0: ROI止盈里程碑
		TPMilestones: map[float64]float64{
			0.10: 0.15, // 10% ROI → 15% 止盈
			0.20: 0.30, // 20% ROI → 30% 止盈
			0.50: 0.70, // 50% ROI → 70% 止盈
		},

		// 执行控制
		StopDistanceMinTicks: 3,
		SafetyTicks:          2,
		MinTickMoveToUpdate:  3,
		UpdateCooldownSec:    25,
	}

	// 调度器配置
	c.Scheduler = SchedulerConfig{
		CheckIntervalSec:         10,
		EnableSoftStopInFastLoop: false,
	}

	return c
}
