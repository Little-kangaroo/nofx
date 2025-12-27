package triggers

import (
	"math"
)

// DetectBoRetest 检测BO_RETEST触发器（突破回测）
// 🔥 P0新增：经典的Breakout-Pullback-Continuation形态
// 功能：补充触发器供给，降低NO_TRIGGER率
//
// 算法逻辑：
// 1. 遍历LevelInfo列表（从Gate2结构聚合）
// 2. 在历史K线中查找突破（prev_close穿越level，move ≥ BoBreakMinAtr*ATR）
// 3. 在突破后的K线中查找回测（price返回level附近，age ≤ BoMaxAgeBars）
// 4. 计算质量评分：q = 0.45*breakStrength + 0.40*retestTightness + 0.15*strengthBoost
// 5. 返回最佳BO_RETEST触发器（质量最高）
func DetectBoRetest(levels []LevelInfo, klines []Kline, currentPrice, atr float64, cfg TriggerConfig) (string, float64, float64, string) {
	if len(levels) == 0 || len(klines) < 2 {
		return FlagNone, 0, 0, ""
	}

	bestFlag := FlagNone // 🔥 修复：初始化为FlagNone而不是空字符串
	var bestQuality float64
	var bestLevel float64
	var bestKeyType string

	// 当前K线索引（最后一根）
	currentIdx := len(klines) - 1
	currentBar := klines[currentIdx]

	// 遍历所有关键位
	for _, level := range levels {
		// === 步骤1：在回看窗口内查找突破 ===
		// 回看范围：[currentIdx - BoLookbackBars, currentIdx - 1]
		breakoutIdx := -1
		breakoutMove := 0.0
		breakoutDir := "" // "UP" or "DOWN"

		startIdx := currentIdx - cfg.BoLookbackBars
		if startIdx < 1 {
			startIdx = 1
		}

		for i := startIdx; i < currentIdx; i++ {
			prevBar := klines[i-1]
			currBar := klines[i]

			// 检测向上突破（prev_close < level，curr_close > level）
			if prevBar.Close < level.Level && currBar.Close > level.Level {
				move := currBar.Close - level.Level
				if move >= cfg.BoBreakMinAtr*atr {
					// 发现有效向上突破
					if move > breakoutMove {
						breakoutIdx = i
						breakoutMove = move
						breakoutDir = "UP"
					}
				}
			}

			// 检测向下突破（prev_close > level，curr_close < level）
			if prevBar.Close > level.Level && currBar.Close < level.Level {
				move := level.Level - currBar.Close
				if move >= cfg.BoBreakMinAtr*atr {
					// 发现有效向下突破
					if move > breakoutMove {
						breakoutIdx = i
						breakoutMove = move
						breakoutDir = "DOWN"
					}
				}
			}
		}

		// 未找到突破，跳过此level
		if breakoutIdx < 0 {
			continue
		}

		// === 步骤2：检测回测 ===
		// 回测条件：
		// - 当前bar在突破后的BoMaxAgeBars内
		// - 当前价格返回到level附近（容差：BoRetestTolBps）

		age := currentIdx - breakoutIdx
		if age > cfg.BoMaxAgeBars || age < 1 {
			continue // 超过最大年龄或无后续K线
		}

		// 计算当前价格到level的距离
		retestDist := math.Abs(currentBar.Close - level.Level)
		retestTolRatio := cfg.BoRetestTolBps / 10000.0 // 基点转比率
		retestTol := retestTolRatio * level.Level

		if retestDist > retestTol {
			continue // 未回测到位
		}

		// === 步骤3：方向验证 ===
		// 向上突破 + 回测支撑 → BO_RETEST_BULL（看多）
		// 向下突破 + 回测阻力 → BO_RETEST_BEAR（看空）
		var flag string
		if breakoutDir == "UP" && level.Side == "LONG" {
			flag = FlagBoRetestBull
		} else if breakoutDir == "DOWN" && level.Side == "SHORT" {
			flag = FlagBoRetestBear
		} else {
			continue // 方向不匹配，跳过
		}

		// === 步骤4：计算质量评分 ===
		// 4.1 突破强度评分：breakStrength = clamp(breakMove / (BoBreakMinAtr * atr), 0, 2) / 2
		breakStrength := clamp(breakoutMove/(cfg.BoBreakMinAtr*atr), 0, 2) / 2

		// 4.2 回测紧密度评分：retestTightness = clamp(1.0 - (retestDist / retestTol), 0, 1)
		retestTightness := clamp(1.0-(retestDist/retestTol), 0, 1)

		// 4.3 结构强度加成：strengthBoost = clamp((strength_z + 0.5) / 2.0, 0, 1)
		strengthBoost := clamp((level.StrengthZ+0.5)/2.0, 0, 1)

		// 4.4 综合质量评分：重视突破强度(45%) + 回测紧密度(40%) + 结构强度(15%)
		quality := 0.45*breakStrength + 0.40*retestTightness + 0.15*strengthBoost

		// === 步骤5：确认质量阈值 ===
		if quality < cfg.BoConfirmMinQ {
			continue // 质量不足，跳过
		}

		// === 步骤6：保留质量最高的触发器 ===
		if quality > bestQuality {
			bestQuality = quality
			bestFlag = flag
			bestLevel = level.Level
			bestKeyType = level.KeyType
		}
	}

	return bestFlag, bestQuality, bestLevel, bestKeyType
}

// BuildLevelsForBoRetest 从Gate2结构聚合数据构建LevelInfo列表
// 🔥 P0新增：适配器函数，将Gate2数据转换为BO_RETEST触发器输入格式
//
// 数据源优先级（与EDGE相同）：
// 1. AnchorCandidate（最高质量，已有StrengthZ）
// 2. SupplyDemandZone（有StrengthZ和上下沿边界）
// 3. VolumeProfile（POC/VAH/VAL + Context）
// 4. SRLevel（需要强度归一化）
//
// 参数：
// - anchorsLong: Gate2的做多锚点列表
// - anchorsShort: Gate2的做空锚点列表
// - currentPrice: 当前最新价格
// - atr14: 14周期ATR（用于强度归一化）
//
// 返回：LevelInfo列表
func BuildLevelsForBoRetest(anchorsLong, anchorsShort []interface{}, currentPrice, atr14 float64) []LevelInfo {
	var levels []LevelInfo

	// 🔥 处理做多锚点（支撑位）
	for _, anchor := range anchorsLong {
		level := buildLevelFromAnchor(anchor, "LONG", currentPrice, atr14)
		if level != nil {
			levels = append(levels, *level)
		}
	}

	// 🔥 处理做空锚点（阻力位）
	for _, anchor := range anchorsShort {
		level := buildLevelFromAnchor(anchor, "SHORT", currentPrice, atr14)
		if level != nil {
			levels = append(levels, *level)
		}
	}

	return levels
}

// buildLevelFromAnchor 从单个锚点构建LevelInfo
// 注意：此函数需要根据实际Gate2数据结构进行适配
func buildLevelFromAnchor(anchor interface{}, side string, currentPrice, atr14 float64) *LevelInfo {
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

	// 提取StrengthZ（如果有）
	strengthZ := 0.0
	if sz, ok := anchorMap["StrengthZ"].(*float64); ok && sz != nil {
		strengthZ = *sz
	}

	// 提取其他字段
	source, _ := anchorMap["Source"].(string)
	timeframe, _ := anchorMap["TimeFrame"].(string)

	// 确定KeyType
	keyType := KeyLevelBoBreakoutSupport
	if side == "SHORT" {
		keyType = KeyLevelBoBreakoutResistance
	}

	return &LevelInfo{
		Level:     level,
		Side:      side,
		Source:    source,
		StrengthZ: strengthZ,
		TimeFrame: timeframe,
		KeyType:   keyType,
	}
}
