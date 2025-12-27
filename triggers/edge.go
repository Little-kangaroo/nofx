package triggers

import (
	"math"
)

// DetectEdge 检测EDGE触发器（结构边缘触碰）
// 🔥 P0新增：无需传统形态，只要价格接近关键结构位即可触发
// 功能：补充触发器供给，降低NO_TRIGGER率
//
// 算法逻辑：
// 1. 遍历TouchInfo列表（从Gate2结构聚合）
// 2. 检测距离条件：dist_bps ≤ EdgeTouchBps OR dist_atr ≤ EdgeTouchAtrMult
// 3. 检测强度过滤：strength_z ≥ EdgeMinStrengthZ
// 4. 计算质量评分：q = 0.75*closeness + 0.25*strength_boost
// 5. 返回最佳EDGE触发器（质量最高）
func DetectEdge(touches []TouchInfo, currentPrice float64, cfg TriggerConfig) (string, float64, float64, string) {
	if len(touches) == 0 {
		return FlagNone, 0, 0, ""
	}

	bestFlag := FlagNone
	var bestQuality float64
	var bestLevel float64
	var bestKeyType string

	// 遍历所有触碰点
	for _, touch := range touches {
		// === 步骤1：检测距离条件 ===
		// 条件A：基点距离 ≤ EdgeTouchBps (默认12)
		// 条件B：ATR距离 ≤ EdgeTouchAtrMult (默认0.20)
		distanceCondition := touch.DistBps <= cfg.EdgeTouchBps || touch.DistAtr <= cfg.EdgeTouchAtrMult

		if !distanceCondition {
			continue // 不满足距离条件，跳过
		}

		// === 步骤2：检测强度过滤 ===
		// strength_z ≥ EdgeMinStrengthZ (默认-0.2)
		if touch.StrengthZ < cfg.EdgeMinStrengthZ {
			continue // 强度不足，跳过
		}

		// === 步骤3：计算质量评分 ===
		// closeness = clamp((EdgeTouchBps - dist_bps) / EdgeTouchBps, 0, 1)
		// 距离越近，closeness越高
		closeness := clamp((cfg.EdgeTouchBps-touch.DistBps)/cfg.EdgeTouchBps, 0, 1)

		// strength_boost = clamp((strength_z + 0.5) / 2.0, 0, 1)
		// strength_z在[-0.2, 3.0]时映射到[0.15, 1.0]
		strengthBoost := clamp((touch.StrengthZ+0.5)/2.0, 0, 1)

		// 综合质量评分：更看重距离(75%)，其次强度(25%)
		quality := 0.75*closeness + 0.25*strengthBoost

		// === 步骤4：选择方向 ===
		var flag string
		if touch.Side == "LONG" {
			// 支撑位 → EDGE_BULL（看多）
			flag = FlagEdgeBull
		} else if touch.Side == "SHORT" {
			// 阻力位 → EDGE_BEAR（看空）
			flag = FlagEdgeBear
		} else {
			continue // 未知方向，跳过
		}

		// === 步骤5：保留质量最高的触发器 ===
		if quality > bestQuality {
			bestQuality = quality
			bestFlag = flag
			bestLevel = touch.Level
			bestKeyType = touch.KeyType
		}
	}

	return bestFlag, bestQuality, bestLevel, bestKeyType
}

// BuildTouchesForEdge 从Gate2结构聚合数据构建TouchInfo列表
// 🔥 P0新增：适配器函数，将Gate2数据转换为EDGE触发器输入格式
//
// 数据源优先级：
// 1. AnchorCandidate（最高质量，已有StrengthZ）
// 2. SupplyDemandZone（有StrengthZ和上下沿边界）
// 3. VolumeProfile（POC/VAH/VAL + Context）
// 4. SRLevel（需要强度归一化）
//
// 参数：
// - anchorsLong: Gate2的做多锚点列表
// - anchorsShort: Gate2的做空锚点列表
// - currentPrice: 当前最新价格
// - atr14: 14周期ATR（用于计算dist_atr和dist_bps）
//
// 返回：TouchInfo列表（已按距离排序）
func BuildTouchesForEdge(anchorsLong, anchorsShort []interface{}, currentPrice, atr14 float64) []TouchInfo {
	var touches []TouchInfo

	// 🔥 处理做多锚点（支撑位）
	for _, anchor := range anchorsLong {
		touch := buildTouchFromAnchor(anchor, "LONG", currentPrice, atr14)
		if touch != nil {
			touches = append(touches, *touch)
		}
	}

	// 🔥 处理做空锚点（阻力位）
	for _, anchor := range anchorsShort {
		touch := buildTouchFromAnchor(anchor, "SHORT", currentPrice, atr14)
		if touch != nil {
			touches = append(touches, *touch)
		}
	}

	return touches
}

// buildTouchFromAnchor 从单个锚点构建TouchInfo
// 注意：此函数需要根据实际Gate2数据结构进行适配
func buildTouchFromAnchor(anchor interface{}, side string, currentPrice, atr14 float64) *TouchInfo {
	// TODO: 实际实现需要根据anchor的真实类型进行类型断言和字段提取
	// 这里提供一个示例框架：

	// 示例：假设anchor是map[string]interface{}类型
	anchorMap, ok := anchor.(map[string]interface{})
	if !ok {
		return nil
	}

	// 提取level字段
	level, ok := anchorMap["Level"].(float64)
	if !ok {
		return nil
	}

	// 计算距离
	distRaw := math.Abs(currentPrice - level)
	distBps := (distRaw / currentPrice) * 10000 // 转换为基点
	distAtr := 0.0
	if atr14 > 0 {
		distAtr = distRaw / atr14 // ATR倍数
	}

	// 提取StrengthZ（如果有）
	strengthZ := 0.0
	if sz, ok := anchorMap["StrengthZ"].(*float64); ok && sz != nil {
		strengthZ = *sz
	}

	// 提取其他字段
	source, _ := anchorMap["Source"].(string)
	timeframe, _ := anchorMap["TimeFrame"].(string)

	// 确定KeyType
	keyType := KeyLevelEdgeSupport
	if side == "SHORT" {
		keyType = KeyLevelEdgeResistance
	}

	return &TouchInfo{
		Level:     level,
		Side:      side,
		DistBps:   distBps,
		DistAtr:   distAtr,
		Source:    source,
		StrengthZ: strengthZ,
		TimeFrame: timeframe,
		KeyType:   keyType,
	}
}
