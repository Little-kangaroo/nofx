package direction

// Config 方向裁决配置（阈值与尺度）
type Config struct {
	// 数据质量阈值
	MinOverallScore float64 // 最小整体评分
	MinMicroDQ      float64 // 最小微观数据质量

	// 欺骗风险阈值
	SpoofHard float64 // 硬红线（禁止开仓）
	SpoofSoft float64 // 软阈值（降级）

	// 流动性阈值
	LiqThin float64 // 流动性稀薄阈值

	// 墙体闪烁阈值
	WallFlickerN int64 // 5分钟内墙体变化次数阈值

	// 动态权重范围
	WOFMin float64 // 订单流权重最小值
	WOFMax float64 // 订单流权重最大值

	// 方向裁决阈值
	ThetaNeutral float64 // 中性区间阈值
	MinConf      float64 // 最小置信度

	// 结构方向保护阈值
	StructProtectMin  float64 // STRUCT_PROTECT 触发的最小结构强度（|struct_dir| 低于此值视为噪声，不触发保护）
	StructOverrideMin float64 // 逆结构方向开仓所需的最低 OF 强度（|of_dir| 必须超过此值）

	// 宏观+OF 双重反向保护
	OFMacroFloor float64 // plan_side=LONG 时若宏观为负且 of_dir < -OFMacroFloor，强制 NEUTRAL

	// 归一化尺度
	CVDFallbackScale float64 // CVD 回退尺度（当 volume_delta 过小时）
	OIScalePct       float64 // OI delta 百分比尺度
	WallScaleUSD     float64 // 墙体强度 USD 尺度
}

// DefaultConfig 返回默认配置
// 🔥 优化：提高反应速度的配置调整
func DefaultConfig() Config {
	return Config{
		// 数据质量阈值（恢复原始值：BTC等主流币数据评分偏低并非真实质量差）
		MinOverallScore: 0.60, // 0.65 → 0.60（BTC连续OF_STALE，阈值偏高于真实数据质量差异）
		MinMicroDQ:      0.50, // 0.55 → 0.50（同上）

		// 欺骗风险阈值（更严格的检测）
		SpoofHard: 0.75, // 从 0.80 降低到 0.75
		SpoofSoft: 0.60,

		// 流动性阈值
		LiqThin: 0.20,

		// 墙体闪烁阈值
		WallFlickerN: 120,

		// 动态权重范围（顺势修复：降低订单流权重，使结构方向获得更大话语权）
		// 历史数据显示：wOF=0.85 导致 struct=+0.60 的多头结构中仍开SHORT（反势）
		// 降低到 0.50-0.65 后：struct+OF一致时充分顺势；冲突时自然NEUTRAL而非逆势
		WOFMin: 0.50, // 0.70 → 0.50（结构方向权重从15%提升至35%）
		WOFMax: 0.65, // 0.85 → 0.65（结构方向权重从15%提升至35%）

		// 方向裁决阈值（平衡阈值，既过滤弱信号又不过度保守）
		// 🔥 修复：调整到平衡值，避免过度过滤
		ThetaNeutral: 0.15, // 从 0.20 降低到 0.15，避免过滤中等强度信号
		MinConf:      0.40, // 从 0.50 降低到 0.40，提高开仓率同时保持质量

		// 结构方向保护：逆结构开仓需要 |of_dir| >= 0.50 的强 OF 信号
		// 防止弱 OF（如 5m 短暂卖压 -0.43）在 ST 趋势持续期间引发反向建仓
		// 例：struct=+0.25（ST 全多头）+ of_dir=-0.43 → 0.43<0.50 → NEUTRAL（不开 SHORT）
		// 例：struct=+0.25 + of_dir=-0.55 → 0.55≥0.50 → 允许 SHORT（真正的反转信号）
		StructProtectMin:  0.15, // struct_dir 低于此值视为噪声，不触发 STRUCT_PROTECT
		StructOverrideMin: 0.50,

		// 宏观+OF 双重反向：禁止"结构孤军 LONG"
		// 当 plan_side=LONG 且宏观看空且 of_dir < -0.25 时，强制 NEUTRAL
		// 例：SOL struct=+0.66, of=-0.37, macro=bearish → 0.37>0.25 → NEUTRAL（消除无效噪声）
		// 例：ETH struct=+0.70, of=-0.12, macro=bearish → 0.12<0.25 → 允许（弱 OF 不阻断）
		OFMacroFloor: 0.25,

		// 归一化尺度
		CVDFallbackScale: 1e6,
		OIScalePct:       0.30,
		WallScaleUSD:     1_000_000,
	}
}
