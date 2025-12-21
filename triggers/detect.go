package triggers

import "sort"

// DetectTriggers 主触发器检测入口
// klines: K线数据（至少需要3根，建议200+根用于Swing检测）
// atr5m: 5分钟ATR（必须）
// volZ: 量能Z分数（<0表示不使用量能过滤）
// cfg: 触发器配置
// 返回: 触发器扫描结果
func DetectTriggers(klines []Kline, atr5m float64, volZ float64, cfg TriggerConfig) TriggerScanResult {
	res := TriggerScanResult{
		TF:      "5m",
		Flags:   make([]string, 0, 3),
		Primary: FlagNone,
		Quality: map[string]float64{},
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

	// === 1. SFP 检测 ===
	if swingLevels.SwingHigh > 0 {
		if q, ok := DetectSfpBear(k, swingLevels.SwingHigh, atr5m, volZ, cfg); ok {
			res.Flags = append(res.Flags, FlagSfpBear)
			res.Quality[FlagSfpBear] = q
			res.KeyLevel = swingLevels.SwingHigh
			res.KeyType = KeyLevelSwingHigh
		}
	}

	if swingLevels.SwingLow > 0 {
		if q, ok := DetectSfpBull(k, swingLevels.SwingLow, atr5m, volZ, cfg); ok {
			res.Flags = append(res.Flags, FlagSfpBull)
			res.Quality[FlagSfpBull] = q
			// 如果还没有KeyLevel，或者质量更高，则更新
			if res.KeyLevel == 0 || q > res.Quality[res.Primary] {
				res.KeyLevel = swingLevels.SwingLow
				res.KeyType = KeyLevelSwingLow
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

	// === 后处理：过滤质量、去重、选择Primary ===
	res.Flags, res.Primary = PostProcessFlags(res.Flags, res.Quality, cfg)

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
