package market

// 通用数学辅助函数
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// 格式化辅助函数 - 获取各种枚举的中文名称（统一管理）
// VolumeDistribution和PricePosition相关函数暂时移除，等类型定义完善后再添加

func getTrendQualityName(quality interface{}) string {
	switch q := quality.(type) {
	case string:
		switch q {
		case "strong":
			return "强劲"
		case "moderate":
			return "中等"
		case "weak":
			return "疲弱"
		default:
			return q
		}
	default:
		return "未知"
	}
}

func getFVGTypeName(fvgType FVGType) string {
	switch fvgType {
	case BullishFVG:
		return "看涨FVG"
	case BearishFVG:
		return "看跌FVG"
	default:
		return "未知FVG"
	}
}

func getFVGQualityName(quality interface{}) string {
	if q, ok := quality.(string); ok {
		switch q {
		case "high":
			return "高质量"
		case "medium":
			return "中等"
		case "low":
			return "低质量"
		default:
			return "未评估"
		}
	}
	return "未评估"
}

func getFVGStatusName(status FVGStatus) string {
	switch status {
	case FVGStatusFresh:
		return "新鲜"
	case FVGStatusTested:
		return "已测试"
	case FVGStatusFilled:
		return "已填补"
	case FVGStatusExpired:
		return "已过期"
	default:
		return "未知"
	}
}

func getZoneTypeName(zoneType ZoneType) string {
	switch zoneType {
	case SupplyZone:
		return "供给区"
	case DemandZone:
		return "需求区"
	default:
		return "未知"
	}
}

func getZoneQualityName(quality interface{}) string {
	if q, ok := quality.(string); ok {
		switch q {
		case "high":
			return "高质量"
		case "medium":
			return "中等质量"
		case "low":
			return "低质量"
		default:
			return "未评估"
		}
	}
	return "未评估"
}

func getZoneStatusName(status interface{}) string {
	if s, ok := status.(string); ok {
		switch s {
		case "active":
			return "活跃"
		case "tested":
			return "已测试"
		case "broken":
			return "已突破"
		case "expired":
			return "已过期"
		default:
			return "未知"
		}
	}
	return "未知"
}

func getTrendDirectionName(direction interface{}) string {
	switch d := direction.(type) {
	case string:
		switch d {
		case "uptrend":
			return "上升趋势"
		case "downtrend":
			return "下降趋势"
		case "sideways":
			return "横盘"
		default:
			return d
		}
	default:
		return "未知"
	}
}

func getSourceName(source string) string {
	switch source {
	case "dow_theory":
		return "道氏理论"
	case "vpvr":
		return "VPVR"
	case "supply_demand":
		return "供需区"
	case "fvg":
		return "FVG"
	default:
		return source
	}
}

func getRiskLevelName(level string) string {
	switch level {
	case "low":
		return "低风险"
	case "medium":
		return "中等风险"
	case "high":
		return "高风险"
	default:
		return level
	}
}

// 通用辅助函数
func getFormationTypeName(formation FormationType) string {
	switch formation {
	case FormationBreakout:
		return "突破形成"
	case FormationPullback:
		return "回调形成"
	case FormationContinuation:
		return "延续形成"
	case FormationReversal:
		return "反转形成"
	default:
		return "未知"
	}
}

func getFVGSignalTypeName(signalType FVGSignalType) string {
	switch signalType {
	case FVGSignalReaction:
		return "FVG反应"
	case FVGSignalFillEntry:
		return "填补入场"
	case FVGSignalRejection:
		return "拒绝反弹"
	case FVGSignalPartialFill:
		return "部分填补"
	case FVGSignalBreakthrough:
		return "突破FVG"
	default:
		return "未知信号"
	}
}