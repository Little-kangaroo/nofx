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

		// 归一化尺度
		CVDFallbackScale: 1e6,
		OIScalePct:       0.30,
		WallScaleUSD:     1_000_000,
	}
}
