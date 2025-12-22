package triggers

import "sort"

// DetectTriggers 主触发器检测入口
// klines: K线数据（至少需要3根，建议200+根用于Swing检测）
// atr5m: 5分钟ATR（必须）
// volZ: 量能Z分数（<0表示不使用量能过滤）
// cfg: 触发器配置
// 返回: 触发器扫描结果
func DetectTriggers(klines []Kline, atr5m float64, volZ float64, cfg TriggerConfig) TriggerScanResult {
	// 🔥 P0-修复：确保所有 slice/map 字段初始化为空但非 nil，避免 JSON 序列化为 null
	res := TriggerScanResult{
		TF:      "5m",
		Flags:   make([]string, 0, 3),
		Primary: FlagNone,
		Quality: map[string]float64{},

		// 🔥 初始化所有分层字段为空集合（非 nil）
		RawFlags:      make([]string, 0),
		RawQuality:    map[string]float64{},
		AIFlags:       make([]string, 0),
		AIQuality:     map[string]float64{},
		StrictFlags:   make([]string, 0),
		StrictQuality: map[string]float64{},
	}

	// 前置检查
	if len(klines) < 3 || atr5m <= 0 {
		return res
	}

	// 获取最后三根K线
	k := klines[len(klines)-1]
	prev := klines[len(klines)-2]
	prev2 := klines[len(klines)-3]

	// 查找Swing High/Low
	swingLevels := FindSwingLevels(klines, cfg)

	// 🔥 P0-04修复：引入 bestKeyQ 变量，避免使用 res.Primary 比较（此时 Primary 尚未选出）
	bestKeyQ := -1.0

	// === 1. SFP 检测 ===
	if swingLevels.SwingHigh > 0 {
		if q, ok := DetectSfpBear(k, swingLevels.SwingHigh, atr5m, volZ, cfg); ok {
			res.Flags = append(res.Flags, FlagSfpBear)
			res.Quality[FlagSfpBear] = q
			res.KeyLevel = swingLevels.SwingHigh
			res.KeyType = KeyLevelSwingHigh
			bestKeyQ = q
		}
	}

	if swingLevels.SwingLow > 0 {
		if q, ok := DetectSfpBull(k, swingLevels.SwingLow, atr5m, volZ, cfg); ok {
			res.Flags = append(res.Flags, FlagSfpBull)
			res.Quality[FlagSfpBull] = q
			// 如果还没有KeyLevel，或者质量更高，则更新
			if res.KeyLevel == 0 || q > bestKeyQ {
				res.KeyLevel = swingLevels.SwingLow
				res.KeyType = KeyLevelSwingLow
				bestKeyQ = q
			}
		}
	}

	// === 2. Engulf 检测 ===
	if q, ok := DetectEngulfBear(k, prev, atr5m, cfg); ok {
		res.Flags = append(res.Flags, FlagEngulfBear)
		res.Quality[FlagEngulfBear] = q
	}

	if q, ok := DetectEngulfBull(k, prev, atr5m, cfg); ok {
		res.Flags = append(res.Flags, FlagEngulfBull)
		res.Quality[FlagEngulfBull] = q
	}

	// === 3. IBB 检测 ===
	if q, ok := DetectIbbBull(k, prev, prev2, atr5m, volZ, cfg); ok {
		res.Flags = append(res.Flags, FlagIbbBull)
		res.Quality[FlagIbbBull] = q
	}

	if q, ok := DetectIbbBear(k, prev, prev2, atr5m, volZ, cfg); ok {
		res.Flags = append(res.Flags, FlagIbbBear)
		res.Quality[FlagIbbBear] = q
	}

	// === 4. Momentum Ignition 检测 ===
	if q, ok := DetectMomoBull(k, atr5m, volZ, cfg); ok {
		res.Flags = append(res.Flags, FlagMomoBull)
		res.Quality[FlagMomoBull] = q
	}

	if q, ok := DetectMomoBear(k, atr5m, volZ, cfg); ok {
		res.Flags = append(res.Flags, FlagMomoBear)
		res.Quality[FlagMomoBear] = q
	}

	// 🔥 P0-新增：在PostProcess前保存原始数据（用于诊断和Gate3触发窗口）
	res.RawFlags = make([]string, len(res.Flags))
	copy(res.RawFlags, res.Flags)
	res.RawQuality = make(map[string]float64, len(res.Quality))
	for k, v := range res.Quality {
		res.RawQuality[k] = v
	}

	// 🔥 P0-2修复：生成分层输出（raw/ai/strict三层质量过滤）
	// AI层：轻过滤（BorderlineMin），用于Gate3触发窗口（提高召回率）
	aiFlags, aiprimary := PostProcessFlagsWithMin(res.Flags, res.Quality, cfg.BorderlineMin, cfg)
	res.AIFlags = aiFlags
	res.AIQuality = extractQualityMap(res.AIFlags, res.Quality)

	// Strict层：严格过滤（QualityMin），用于内部统计（保证精度）
	strictFlags, strictprimary := PostProcessFlagsWithMin(res.Flags, res.Quality, cfg.QualityMin, cfg)
	res.StrictFlags = strictFlags
	res.StrictQuality = extractQualityMap(res.StrictFlags, res.Quality)

	// === 后处理：过滤质量、去重、选择Primary（向后兼容） ===
	// 🔥 P0-2注：res.Flags/res.Primary 保持与 strict 层相同（向后兼容）
	res.Flags = res.StrictFlags
	res.Primary = strictprimary

	// 🔥 P0-2修复：契约自洽安全带 - AI层
	if len(res.AIFlags) == 0 {
		res.AIQuality = map[string]float64{}
		// aiprimary已经是NONE，无需重置
	}
	if len(res.AIFlags) > 0 && (aiprimary == FlagNone || !containsString(res.AIFlags, aiprimary)) {
		aiprimary = res.AIFlags[0]
	}

	// 🔥 P0-2修复：契约自洽安全带 - Strict层
	if len(res.StrictFlags) == 0 {
		res.StrictQuality = map[string]float64{}
		res.Primary = FlagNone
	}
	if len(res.StrictFlags) > 0 && (res.Primary == FlagNone || !containsString(res.StrictFlags, res.Primary)) {
		res.Primary = res.StrictFlags[0]
	}

	return res
}

// PostProcessFlags 后处理触发器标志：过滤低质量、去重、限制数量、选择Primary
// flags: 原始触发器标志列表
// quality: 质量评分映射
// cfg: 配置
// 返回: (过滤后的flags, primary触发器)
func PostProcessFlags(flags []string, quality map[string]float64, cfg TriggerConfig) ([]string, string) {
	// 1. 过滤低质量触发器
	kept := make([]string, 0, len(flags))
	for _, f := range flags {
		if quality[f] >= cfg.QualityMin {
			kept = append(kept, f)
		}
	}

	// 2. 去重（虽然理论上不会重复，但保险起见）
	kept = uniqueStrings(kept)

	// 3. 按质量评分降序排序
	sort.SliceStable(kept, func(i, j int) bool {
		return quality[kept[i]] > quality[kept[j]]
	})

	// 4. 限制数量（取Top-N）
	if len(kept) > cfg.MaxFlagsPerBar {
		kept = kept[:cfg.MaxFlagsPerBar]
	}

	// 5. 选择Primary（质量最高的）
	primary := FlagNone
	if len(kept) > 0 {
		primary = kept[0]
	}

	return kept, primary
}

// DetectTriggersSimple 简化版触发器检测（不需要量能Z分数）
// klines: K线数据
// atr5m: 5分钟ATR
// cfg: 配置
// 返回: 触发器扫描结果
func DetectTriggersSimple(klines []Kline, atr5m float64, cfg TriggerConfig) TriggerScanResult {
	return DetectTriggers(klines, atr5m, -1, cfg) // volZ=-1表示不使用
}

// DetectTriggersWithDefaultConfig 使用默认配置的触发器检测
// klines: K线数据
// atr5m: 5分钟ATR
// volZ: 量能Z分数
// 返回: 触发器扫描结果
func DetectTriggersWithDefaultConfig(klines []Kline, atr5m float64, volZ float64) TriggerScanResult {
	return DetectTriggers(klines, atr5m, volZ, DefaultConfig())
}

// PostProcessFlagsWithMin 后处理触发器标志（支持自定义质量阈值）
// flags: 原始触发器标志列表
// quality: 质量评分映射
// qualityMinThreshold: 自定义质量阈值（允许不同层级使用不同阈值）
// cfg: 配置（用于MaxFlagsPerBar等参数）
// 返回: (过滤后的flags, primary触发器)
// 🔥 P0-2新增：支持分层过滤（AI层使用BorderlineMin，Strict层使用QualityMin）
func PostProcessFlagsWithMin(flags []string, quality map[string]float64, qualityMinThreshold float64, cfg TriggerConfig) ([]string, string) {
	// 1. 过滤低质量触发器（使用自定义阈值）
	kept := make([]string, 0, len(flags))
	for _, f := range flags {
		if quality[f] >= qualityMinThreshold {
			kept = append(kept, f)
		}
	}

	// 2. 去重（虽然理论上不会重复，但保险起见）
	kept = uniqueStrings(kept)

	// 3. 按质量评分降序排序
	sort.SliceStable(kept, func(i, j int) bool {
		return quality[kept[i]] > quality[kept[j]]
	})

	// 4. 限制数量（取Top-N）
	if len(kept) > cfg.MaxFlagsPerBar {
		kept = kept[:cfg.MaxFlagsPerBar]
	}

	// 5. 选择Primary（质量最高的）
	primary := FlagNone
	if len(kept) > 0 {
		primary = kept[0]
	}

	return kept, primary
}

// extractQualityMap 从完整质量映射中提取指定flags的质量评分
// flags: 需要提取的触发器标志列表
// fullQuality: 完整的质量评分映射
// 返回: 仅包含指定flags的质量映射
// 🔥 P0-2新增：用于为AI层和Strict层生成对应的质量映射
func extractQualityMap(flags []string, fullQuality map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(flags))
	for _, f := range flags {
		if q, exists := fullQuality[f]; exists {
			result[f] = q
		}
	}
	return result
}

// containsString 检查字符串切片中是否包含指定字符串
// 🔥 P0-2新增：用于契约自洽验证
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetPatternHint 将内部触发器标志映射到V-16.4 pattern hint
// flag: 内部触发器标志（如 "SFP_BULL", "ENGULF_BEAR" 等）
// 返回: V-16.4 pattern hint 名称
// 🔥 P1-2新增：支持 AI 模型识别触发器类型
func GetPatternHint(flag string) string {
	// 根据 flag 前缀映射到 pattern
	switch {
	case flag == FlagSfpBull || flag == FlagSfpBear:
		return "SFP"
	case flag == FlagEngulfBull || flag == FlagEngulfBear:
		return "Engulf"
	case flag == FlagIbbBull || flag == FlagIbbBear:
		return "MOM_BREAK"
	case flag == FlagMomoBull || flag == FlagMomoBear:
		return "MOM_BREAK"
	case flag == FlagNone:
		return ""
	default:
		return ""
	}
}

// GetPatternHintFromFlags 从触发器标志列表中获取pattern hint
// flags: 触发器标志列表（通常是ai_flags或trigger_flags）
// 返回: pattern hint（基于第一个flag，即primary触发器）
// 🔥 P1-2新增：简化AI侧调用
func GetPatternHintFromFlags(flags []string) string {
	if len(flags) == 0 {
		return ""
	}
	return GetPatternHint(flags[0])
}
