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

	// 🎯 V-21.0: ROI止损里程碑（ROI-based progressive stop-loss）
	StopLossMilestones map[float64]float64 // ROI阈值 -> 止损百分比 (如：0.10 -> 0.05表示10%ROI时止损移到entry+5%)

	// 执行控制与防抖
	StopDistanceMinTicks int64 // 3 - 止损最小距离（tick数）
	SafetyTicks          int64 // 2 - 安全缓冲（tick数）
	MinTickMoveToUpdate  int64 // 3 - 最小移动距离才更新（tick数）
	UpdateCooldownSec    int64 // 25 - 更新冷却期（秒）

	Scheduler SchedulerConfig
}

// DefaultConfig 返回默认配置（V-21.0：ROI止损阶梯）
func DefaultConfig() Config {
	c := Config{
		// ROI锁盈（无时间约束）
		ROILockTrigger: 0.03, // 3%触发（保本阶段起点）
		FloorPriceBps:  20,   // 盈利地板（仅在无StopLossMilestones时使用）

		// 🎯 V-19.0: ROI止盈里程碑
		TPMilestones: map[float64]float64{
			0.10: 0.15, // 10% ROI → 15% 止盈
			0.20: 0.30, // 20% ROI → 30% 止盈
			0.50: 0.70, // 50% ROI → 70% 止盈
		},

		// 🎯 V-21.0: ROI止损里程碑（激进型）
		StopLossMilestones: map[float64]float64{
			0.03: 0.00, // 3% ROI → 保本（止损移至入场价）
			0.05: 0.03, // 5% ROI → 锁住3%
			0.08: 0.05, // 8% ROI → 锁住5%
			0.12: 0.08, // 12% ROI → 锁住8%
			0.16: 0.11, // 16% ROI → 锁住11%
			0.20: 0.14, // 20% ROI → 锁住14%
			0.25: 0.18, // 25% ROI → 锁住18%
			0.30: 0.22, // 30% ROI → 锁住22%
			0.40: 0.30, // 40% ROI → 锁住30%
			0.50: 0.38, // 50% ROI → 锁住38%
			0.60: 0.45, // 60% ROI → 锁住45%
			0.80: 0.55, // 80% ROI → 锁住55%
			1.00: 0.70, // 100% ROI → 锁住70%
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
