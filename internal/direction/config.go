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
func DefaultConfig() Config {
	return Config{
		// 数据质量阈值
		MinOverallScore: 0.60,
		MinMicroDQ:      0.50,

		// 欺骗风险阈值
		SpoofHard: 0.80,
		SpoofSoft: 0.60,

		// 流动性阈值
		LiqThin: 0.20,

		// 墙体闪烁阈值
		WallFlickerN: 120,

		// 动态权重范围（订单流权重更高）
		WOFMin: 0.60,
		WOFMax: 0.80,

		// 方向裁决阈值
		ThetaNeutral: 0.12,
		MinConf:      0.30,

		// 归一化尺度
		CVDFallbackScale: 1e6,
		OIScalePct:       0.30,
		WallScaleUSD:     1_000_000,
	}
}
