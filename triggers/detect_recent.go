package triggers

import "sort"

// TriggerWithAge 带年龄标记的触发器扫描结果
// 🔥 P1-1新增：支持最近N根K线窗口检测
type TriggerWithAge struct {
	Result         TriggerScanResult // 触发器扫描结果
	AgeBars        int               // K线年龄（0=当前bar，1=上一根，2=上上根）
	BarCloseTimeMs int64             // 触发器所在K线的收盘时间（毫秒时间戳）
}

// createEmptyTriggerScanResult 创建正确初始化的空 TriggerScanResult（避免 nil 字段）
// 🔥 P0-修复：确保所有 slice/map 字段都是空集合而非 nil，避免 JSON 序列化为 null
func createEmptyTriggerScanResult() TriggerScanResult {
	return TriggerScanResult{
		TF:            "5m",
		Flags:         make([]string, 0),
		Primary:       FlagNone,
		Quality:       map[string]float64{},
		RawFlags:      make([]string, 0),
		RawQuality:    map[string]float64{},
		AIFlags:       make([]string, 0),
		AIQuality:     map[string]float64{},
		StrictFlags:   make([]string, 0),
		StrictQuality: map[string]float64{},
	}
}

// DetectTriggersRecent 最近N根K线窗口触发器检测
// klines: K线数据（至少需要3根，建议200+根用于Swing检测）
// atr5m: 5分钟ATR（必须）
// volZ: 量能Z分数（<0表示不使用量能过滤）
// cfg: 触发器配置
// windowSize: 回看窗口大小（默认3，表示检测最近3根K线）
// 返回: 最佳触发器结果（按age优先、quality次优排序）
// 🔥 P1-1新增：Gate3触发窗口扩展，避免单根K线检测的时间敏感性
func DetectTriggersRecent(klines []Kline, atr5m float64, volZ float64, cfg TriggerConfig, windowSize int) TriggerWithAge {
	// 参数验证
	if windowSize <= 0 {
		windowSize = 3 // 默认回看3根
	}
	if len(klines) < windowSize {
		// K线数量不足，降级到可用数量
		windowSize = len(klines)
	}
	if windowSize < 1 {
		// 完全没有K线，返回空结果
		// 🔥 P0-修复：使用 createEmptyTriggerScanResult 确保所有字段正确初始化
		return TriggerWithAge{
			Result:         createEmptyTriggerScanResult(),
			AgeBars:        -1,
			BarCloseTimeMs: 0,
		}
	}

	// 收集所有候选触发器
	candidates := make([]TriggerWithAge, 0, windowSize)

	// 逐根K线检测（从最新到最旧）
	for age := 0; age < windowSize; age++ {
		// 构建截止到当前age的K线切片
		endIdx := len(klines) - age
		if endIdx <= 0 {
			break
		}
		partialKlines := klines[:endIdx]

		// 执行触发器检测
		result := DetectTriggers(partialKlines, atr5m, volZ, cfg)

		// 🔥 P1-1修复：使用 ai_flags（而不是 strict_flags），与Gate3窗口对齐
		// 只有ai_flags非空才认为检测到触发器
		if len(result.AIFlags) > 0 {
			candidates = append(candidates, TriggerWithAge{
				Result:         result,
				AgeBars:        age,
				BarCloseTimeMs: partialKlines[endIdx-1].CloseTime,
			})
		}
	}

	// 如果没有任何候选，返回空结果
	// 🔥 P0-修复：即使无触发器也要返回当前 K 线的 close_time（而不是 0）
	if len(candidates) == 0 {
		return TriggerWithAge{
			Result:         createEmptyTriggerScanResult(),
			AgeBars:        -1,
			BarCloseTimeMs: klines[len(klines)-1].CloseTime, // 使用最后一根K线的收盘时间
		}
	}

	// 排序：age越小越优先，age相同时quality越高越优先
	// 🔥 P1-1注：使用 ai_flags 的 primary 和 ai_quality 进行排序
	sort.SliceStable(candidates, func(i, j int) bool {
		// 优先级1: age越小越优先
		if candidates[i].AgeBars != candidates[j].AgeBars {
			return candidates[i].AgeBars < candidates[j].AgeBars
		}

		// 优先级2: quality越高越优先（使用ai_quality中的primary质量）
		qI := getAIPrimaryQuality(candidates[i].Result)
		qJ := getAIPrimaryQuality(candidates[j].Result)
		return qI > qJ
	})

	// 返回最佳候选（排序后的第一个）
	return candidates[0]
}

// getAIPrimaryQuality 获取AI层primary触发器的质量评分
// 🔥 P1-1辅助函数：用于排序时比较quality
func getAIPrimaryQuality(result TriggerScanResult) float64 {
	if len(result.AIFlags) == 0 {
		return 0
	}
	// 使用第一个ai_flag作为primary（因为ai_flags已经按quality排序）
	primary := result.AIFlags[0]
	if q, exists := result.AIQuality[primary]; exists {
		return q
	}
	return 0
}

// DetectTriggersRecentDefault 使用默认窗口大小（3根）的最近K线检测
// klines: K线数据
// atr5m: 5分钟ATR
// volZ: 量能Z分数
// cfg: 配置
// 返回: 最佳触发器结果
func DetectTriggersRecentDefault(klines []Kline, atr5m float64, volZ float64, cfg TriggerConfig) TriggerWithAge {
	return DetectTriggersRecent(klines, atr5m, volZ, cfg, 3)
}

// DetectTriggersRecentWithStructures 最近N根K线窗口触发器检测（支持EDGE和BO_RETEST）
// 🔥 P1-01-c新增：扩展版，支持传入Gate2结构数据以启用EDGE触发器
//
// 参数：
// - klines: K线数据（至少需要3根，建议200+根用于Swing检测）
// - atr5m: 5分钟ATR（必须）
// - volZ: 量能Z分数（<0表示不使用量能过滤）
// - cfg: 触发器配置
// - windowSize: 回看窗口大小（默认3，表示检测最近3根K线）
// - touches: EDGE触发器的触碰点数据（来自Gate2结构聚合）
// - levels: BO_RETEST触发器的关键位数据（来自Gate2结构聚合）
// - currentPrice: 当前价格（用于距离计算）
//
// 返回: 最佳触发器结果（按age优先、quality次优排序）
func DetectTriggersRecentWithStructures(
	klines []Kline,
	atr5m float64,
	volZ float64,
	cfg TriggerConfig,
	windowSize int,
	touches []TouchInfo,
	levels []LevelInfo,
	currentPrice float64,
) TriggerWithAge {
	// 参数验证
	if windowSize <= 0 {
		windowSize = 3 // 默认回看3根
	}
	if len(klines) < windowSize {
		// K线数量不足，降级到可用数量
		windowSize = len(klines)
	}
	if windowSize < 1 {
		// 完全没有K线，返回空结果
		return TriggerWithAge{
			Result:         createEmptyTriggerScanResult(),
			AgeBars:        -1,
			BarCloseTimeMs: 0,
		}
	}

	// 收集所有候选触发器
	candidates := make([]TriggerWithAge, 0, windowSize)

	// 逐根K线检测（从最新到最旧）
	for age := 0; age < windowSize; age++ {
		// 构建截止到当前age的K线切片
		endIdx := len(klines) - age
		if endIdx <= 0 {
			break
		}
		partialKlines := klines[:endIdx]

		// 🔥 P1-01-c关键修改：调用 DetectTriggersWithStructures 以支持 EDGE 触发器
		result := DetectTriggersWithStructures(
			partialKlines,
			atr5m,
			volZ,
			cfg,
			touches,      // 传入 TouchInfo 用于 EDGE 检测
			levels,       // 传入 LevelInfo 用于 BO_RETEST 检测
			currentPrice, // 当前价格
		)

		// 使用 ai_flags（而不是 strict_flags），与Gate3窗口对齐
		// 只有ai_flags非空才认为检测到触发器
		if len(result.AIFlags) > 0 {
			candidates = append(candidates, TriggerWithAge{
				Result:         result,
				AgeBars:        age,
				BarCloseTimeMs: partialKlines[endIdx-1].CloseTime,
			})
		}
	}

	// 如果没有任何候选，返回空结果
	if len(candidates) == 0 {
		return TriggerWithAge{
			Result:         createEmptyTriggerScanResult(),
			AgeBars:        -1,
			BarCloseTimeMs: klines[len(klines)-1].CloseTime, // 使用最后一根K线的收盘时间
		}
	}

	// 排序：age越小越优先，age相同时quality越高越优先
	// 使用 ai_flags 的 primary 和 ai_quality 进行排序
	sort.SliceStable(candidates, func(i, j int) bool {
		// 优先级1: age越小越优先
		if candidates[i].AgeBars != candidates[j].AgeBars {
			return candidates[i].AgeBars < candidates[j].AgeBars
		}

		// 优先级2: quality越高越优先（使用ai_quality中的primary质量）
		qI := getAIPrimaryQuality(candidates[i].Result)
		qJ := getAIPrimaryQuality(candidates[j].Result)
		return qI > qJ
	})

	// 返回最佳候选（排序后的第一个）
	return candidates[0]
}

// DetectTriggersRecentWithStructuresDefault 使用默认窗口大小（3根）的结构化触发器检测
// 🔥 P1-01-c新增：便捷封装函数
//
// 参数：
// - klines: K线数据
// - atr5m: 5分钟ATR
// - volZ: 量能Z分数
// - cfg: 配置
// - touches: EDGE触发器的触碰点数据（来自Gate2）
// - levels: BO_RETEST触发器的关键位数据（来自Gate2）
// - currentPrice: 当前价格
//
// 返回: 最佳触发器结果
func DetectTriggersRecentWithStructuresDefault(
	klines []Kline,
	atr5m float64,
	volZ float64,
	cfg TriggerConfig,
	touches []TouchInfo,
	levels []LevelInfo,
	currentPrice float64,
) TriggerWithAge {
	return DetectTriggersRecentWithStructures(klines, atr5m, volZ, cfg, 3, touches, levels, currentPrice)
}
