package market

import (
	"log"
	"time"
)

// === 各结构分析器适配器 ===
// 将供需区、VPVR、SR、FVG、Fib的输出统一映射为AnchorCandidate

// fromSupplyDemand 从供需区数据提取锚点候选
func (ae *AnchorEngine) fromSupplyDemand(data *SupplyDemandData, timeframes map[string][]Kline) []AnchorCandidate {
	var candidates []AnchorCandidate

	if data == nil || len(data.ActiveZones) == 0 {
		return candidates
	}

	for _, zone := range data.ActiveZones {
		// 🔥 P0-01修复：优先从data.Timeframe读取，其次从zone.Origin.TimeFrame
		tf := data.Timeframe
		if tf == "" && zone.Origin != nil {
			tf = zone.Origin.TimeFrame
		}
		if tf == "" {
			tf = "unknown" // 避免默认1h误导
			log.Printf("⚠️ [P0-01] 供需区zone %s 缺少timeframe，标记为unknown", zone.ID)
		}

		// 确定锚点类型：4h/1h为HTF，其他为MTF
		// unknown默认为MTF避免误判为HTF
		anchorType := AnchorZoneMTF
		if tf == "4h" || tf == "1h" {
			anchorType = AnchorHTFZone
		}

		// 确定方向：供给区为SHORT(阻力)，需求区为LONG(支撑)
		dir := "LONG"
		if zone.Type == SupplyZone {
			dir = "SHORT"
		}

		// 🔥 P0-05预留：只有StrengthZReady时才传递strengthZ
		var strengthZ *float64
		if zone.StrengthZReady && zone.Context != nil {
			strengthZ = &zone.Context.StrengthZ
		}

		// 🔥 P0-05预留：只有VolRatioValid时才传递volRatio（待P0-05实现）
		var volRatio *float64
		if zone.Context != nil {
			// TODO P0-05: 增加VolRatioValid检查
			volRatio = &zone.Context.VolRatio
		}

		candidate := AnchorCandidate{
			Dir:       dir,
			Type:      anchorType,
			TF:        tf, // 🔥 P0-01: 使用真实TF
			Level:     (zone.LowerBound + zone.UpperBound) / 2, // 区域中心作为锚点
			BandLo:    zone.LowerBound,
			BandHi:    zone.UpperBound,
			IsFresh:   zone.Context != nil && zone.Context.IsFresh,
			StrengthZ: strengthZ,
			VolRatio:  volRatio,
			Meta: map[string]interface{}{
				"source_type":      "supply_demand",
				"zone_id":          zone.ID,
				"touch_count":      zone.TouchCount,
				"zone_strength":    zone.Strength,
				"status":           zone.Status,
				"zone_type":        string(zone.Type),
				"strength_z_ready": zone.StrengthZReady, // 🔥 新增：标记Z分数可靠性
				"sample_count":     zone.SampleCount,
			},
		}

		candidates = append(candidates, candidate)
	}

	return candidates
}

// fromVPVR 从VPVR数据提取锚点候选
func (ae *AnchorEngine) fromVPVR(data *VolumeProfile, timeframes map[string][]Kline) []AnchorCandidate {
	var candidates []AnchorCandidate

	if data == nil {
		return candidates
	}

	// 🔥 P0-04修复：从data.UsedTimeFrame读取真实timeframe，避免硬编码"1h"
	vpvrTF := "unknown"
	if data.UsedTimeFrame != "" {
		vpvrTF = data.UsedTimeFrame
	}

	// VAH作为阻力锚点
	if data.VAH > 0 {
		var strengthZ *float64
		var volRatio *float64

		if data.Context != nil {
			strengthZ = &data.Context.StrengthZ
			volRatio = &data.Context.VolRatio
		}

		vahCandidate := AnchorCandidate{
			Dir:       "SHORT",
			Type:      AnchorVPVRBound,
			TF:        vpvrTF, // 🔥 P0-04修复：使用真实TF
			Level:     data.VAH,
			BandLo:    data.VAH,
			BandHi:    data.VAH,
			IsFresh:   data.Context != nil && data.Context.IsFresh,
			StrengthZ: strengthZ,
			VolRatio:  volRatio,
			Meta: map[string]interface{}{
				"source_type": "vpvr",
				"level_type":  "vah",
				"poc_price":   func() float64 {
					if data.POC != nil {
						return data.POC.Price
					}
					return 0
				}(),
			},
		}

		candidates = append(candidates, vahCandidate)
	}

	// VAL作为支撑锚点
	if data.VAL > 0 {
		var strengthZ *float64
		var volRatio *float64

		if data.Context != nil {
			strengthZ = &data.Context.StrengthZ
			volRatio = &data.Context.VolRatio
		}

		valCandidate := AnchorCandidate{
			Dir:       "LONG",
			Type:      AnchorVPVRBound,
			TF:        vpvrTF, // 🔥 P0-04修复：使用真实TF
			Level:     data.VAL,
			BandLo:    data.VAL,
			BandHi:    data.VAL,
			IsFresh:   data.Context != nil && data.Context.IsFresh,
			StrengthZ: strengthZ,
			VolRatio:  volRatio,
			Meta: map[string]interface{}{
				"source_type": "vpvr",
				"level_type":  "val",
				"poc_price":   func() float64 {
					if data.POC != nil {
						return data.POC.Price
					}
					return 0
				}(),
			},
		}

		candidates = append(candidates, valCandidate)
	}

	// POC作为双向锚点（根据价格位置决定方向）
	if data.POC != nil && data.POC.Price > 0 {
		var strengthZ *float64
		var volRatio *float64

		if data.Context != nil {
			strengthZ = &data.Context.StrengthZ
			volRatio = &data.Context.VolRatio
		}

		// POC可以作为双向锚点，这里创建两个候选
		pocLongCandidate := AnchorCandidate{
			Dir:       "LONG",
			Type:      AnchorVPVRBound,
			TF:        vpvrTF, // 🔥 P0-04修复：使用真实TF
			Level:     data.POC.Price,
			BandLo:    data.POC.Price,
			BandHi:    data.POC.Price,
			IsFresh:   data.Context != nil && data.Context.IsFresh,
			StrengthZ: strengthZ,
			VolRatio:  volRatio,
			Meta: map[string]interface{}{
				"source_type":     "vpvr",
				"level_type":      "poc_long",
				"volume_percent":  data.POC.VolumePercent,
			},
		}

		pocShortCandidate := AnchorCandidate{
			Dir:       "SHORT",
			Type:      AnchorVPVRBound,
			TF:        vpvrTF, // 🔥 P0-04修复：使用真实TF
			Level:     data.POC.Price,
			BandLo:    data.POC.Price,
			BandHi:    data.POC.Price,
			IsFresh:   data.Context != nil && data.Context.IsFresh,
			StrengthZ: strengthZ,
			VolRatio:  volRatio,
			Meta: map[string]interface{}{
				"source_type":     "vpvr",
				"level_type":      "poc_short",
				"volume_percent":  data.POC.VolumePercent,
			},
		}

		candidates = append(candidates, pocLongCandidate, pocShortCandidate)
	}

	return candidates
}

// fromSR 从支撑阻力数据提取锚点候选
// 🔥 P0-05修复：修复TF硬编码、类型混淆、StrengthZ计算错误、IsFresh逻辑问题
func (ae *AnchorEngine) fromSR(data *SupportResistanceData, timeframes map[string][]Kline) []AnchorCandidate {
	var candidates []AnchorCandidate

	if data == nil {
		return candidates
	}

	// 🔥 P0-05A修复：获取真实时间框架，避免硬编码
	TF := data.UsedTimeFrame
	if TF == "" {
		TF = "4h" // 兜底：保持可用（假设默认4h分析）
		log.Printf("[SR-WARN] SR分析未提供UsedTimeFrame，使用默认值: %s", TF)
	}

	// 🔥 P0-05B修复：处理 KeyLevels（普通支撑阻力位）
	for _, level := range data.KeyLevels {
		// 确定方向
		dir := "LONG"
		if level.Type == "resistance" {
			dir = "SHORT"
		}

		// 🔥 P0-05B修复：KeyLevels 应为 AnchorSRLevel，而非 AnchorSRFlip
		candidate := AnchorCandidate{
			Dir:    dir,
			Type:   AnchorSRLevel, // 🔥 修复：普通级别不是 Flip
			TF:     TF,             // 🔥 P0-05A修复：使用真实时间框架
			Level:  level.Price,
			BandLo: level.Price,
			BandHi: level.Price,
			Meta: map[string]interface{}{
				"sr_kind":   "level",       // 区分是level还是flip
				"role":      level.Type,    // support/resistance
				"hit_count": level.HitCount,
				"strength":  level.Strength, // 0-100范围
			},
		}

		// 🔥 P0-05C修复：StrengthZ计算
		// 简单归一化：strength/100 作为临时方案（理想情况应使用zscore）
		if level.Strength > 0 {
			// 方案1：简单归一化（强度已经是0-100）
			strengthZ := level.Strength / 100.0
			candidate.StrengthZ = &strengthZ

			// TODO P1：升级为滑动窗口zscore统计
		}

		// 🔥 IsFresh判断保留（基于hit_count）
		// 注意：MinHits默认为3，所以hit_count<=2意味着刚形成
		if level.HitCount <= 2 {
			candidate.IsFresh = true
		}

		candidates = append(candidates, candidate)
	}

	// 🔥 P0-05B修复：单独处理 SRFlips（支撑阻力转换）
	for _, flip := range data.SRFlips {
		// 确定方向（基于转换后的类型）
		dir := "LONG"
		if flip.FlippedType == "resistance" {
			dir = "SHORT"
		}

		candidate := AnchorCandidate{
			Dir:    dir,
			Type:   AnchorSRFlip, // 🔥 正确：这才是真正的Flip
			TF:     TF,
			Level:  flip.FlipPrice,
			BandLo: flip.FlipPrice,
			BandHi: flip.FlipPrice,
			Meta: map[string]interface{}{
				"sr_kind":         "flip",
				"from":            flip.OriginalType,
				"to":              flip.FlippedType,
				"flip_strength":   flip.FlipStrength, // 0-100
				"flip_confirmed":  flip.FlipConfirmation,
				"pre_touches":     flip.PreFlipTouches,
				"post_touches":    flip.PostFlipTouches,
			},
		}

		// 🔥 P0-05C修复：Flip的StrengthZ（基于FlipStrength）
		if flip.FlipStrength > 0 {
			strengthZ := flip.FlipStrength / 100.0
			candidate.StrengthZ = &strengthZ
		}

		// 🔥 IsFresh判断（基于FlipTime距离现在的时间）
		// TODO P1：实现基于时间戳的freshness判断
		if flip.FlipConfirmation {
			// 已确认的flip可能不那么fresh
			candidate.IsFresh = false
		} else {
			// 未确认的flip比较fresh
			candidate.IsFresh = true
		}

		candidates = append(candidates, candidate)
	}

	return candidates
}

// fromFVG 从FVG数据提取锚点候选
func (ae *AnchorEngine) fromFVG(data *FVGData, timeframes map[string][]Kline) []AnchorCandidate {
	var candidates []AnchorCandidate

	if data == nil || len(data.ActiveFVGs) == 0 {
		return candidates
	}

	for _, fvg := range data.ActiveFVGs {
		// 确定方向：看涨FVG为LONG(支撑)，看跌FVG为SHORT(阻力)
		dir := "LONG"
		if fvg.Type == BearishFVG {
			dir = "SHORT"
		}

		// 🔥 P0-01修复：从fvg.Origin.TimeFrame获取真实timeframe，避免硬编码
		candTF := "unknown"
		if fvg.Origin != nil && fvg.Origin.TimeFrame != "" {
			candTF = fvg.Origin.TimeFrame
		}

		// 🔥 P1-03修复：只有ready时才传递StrengthZ/VolRatio
		var strengthZ *float64
		var volRatio *float64
		if fvg.Context != nil {
			if fvg.Context.StrengthZReady {
				strengthZ = &fvg.Context.StrengthZ
			}
			if fvg.Context.VolRatioReady {
				volRatio = &fvg.Context.VolRatio
			}
		}

		candidate := AnchorCandidate{
			Dir:       dir,
			Type:      AnchorFVG,
			TF:        candTF, // 🔥 P0-01：使用真实TF
			Level:     fvg.CenterPrice,
			BandLo:    fvg.LowerBound,
			BandHi:    fvg.UpperBound,
			IsFresh:   fvg.Context != nil && fvg.Context.IsFresh,
			StrengthZ: strengthZ, // 🔥 P1-03：尊重ready flag
			VolRatio:  volRatio,  // 🔥 P1-03：尊重ready flag
			Meta: map[string]interface{}{
				"source_type":    "fvg",
				"fvg_id":         fvg.ID,
				"fvg_type":       string(fvg.Type),
				"touch_count":    fvg.TouchCount,
				"width_percent":  fvg.WidthPercent,
				"quality":        fvg.Quality,
				"status":         fvg.Status,
				"strength_z_ready": fvg.Context != nil && fvg.Context.StrengthZReady, // 🔥 P1-03：标记可靠性
				"vol_ratio_ready":  fvg.Context != nil && fvg.Context.VolRatioReady,  // 🔥 P1-03：标记可靠性
			},
		}

		candidates = append(candidates, candidate)
	}

	return candidates
}

// fromFib 从斐波纳契数据提取锚点候选
// 🔥 P0-02 & P0-03修复：修复 TF/IsFresh/StrengthZ/VolRatio
func (ae *AnchorEngine) fromFib(data *FibonacciData, timeframes map[string][]Kline) []AnchorCandidate {
	var candidates []AnchorCandidate

	if data == nil {
		return candidates
	}

	// 🔥 P0-02修复：从 data.Timeframe 读取真实TF
	fibTF := data.Timeframe
	if fibTF == "" {
		fibTF = "unknown"
		log.Printf("⚠️ [P0-02] FibonacciData 缺少 Timeframe，标记为 unknown")
	}

	// 从回调级别提取
	for _, retracement := range data.Retracements {
		if !retracement.IsActive {
			continue
		}

		// 🔥 P0-02修复：计算 IsFresh（基于 Age 和 Context）
		isFresh := false
		if retracement.Context != nil {
			isFresh = retracement.Context.IsFresh
		} else if retracement.Age <= data.Config.MaxRetracementAge {
			isFresh = true
		}

		for _, level := range retracement.Levels {
			if level.Importance < 0.5 { // 只选择重要级别
				continue
			}

			// 确定方向：基于回调类型和级别
			dir := "LONG"
			if retracement.TrendType == TrendDownward {
				// 下跌趋势中的深度回调作为反弹阻力
				if level.Ratio > 0.5 {
					dir = "SHORT"
				}
			} else if retracement.TrendType == TrendUpward {
				// 上涨趋势中的浅度回调作为支撑，深度回调作为阻力
				if level.Ratio < 0.5 {
					dir = "LONG"
				} else {
					dir = "SHORT"
				}
			}

			// 🔥 P0-03修复：透传 StrengthZ 和 VolRatio（遵守 Ready flags）
			var strengthZ *float64
			var volRatio *float64

			if retracement.Context != nil {
				if retracement.Context.StrengthZReady {
					strengthZ = &retracement.Context.StrengthZ
				}
				if retracement.Context.VolRatioReady {
					volRatio = &retracement.Context.VolRatio
				}
			}

			// 🔥 P0-03修复：仅在 Context 为 nil 时使用启发式 StrengthZ
			if strengthZ == nil && level.IsGoldenRatio {
				heuristicZ := 1.5
				strengthZ = &heuristicZ
			}

			candidate := AnchorCandidate{
				Dir:       dir,
				Type:      AnchorFib,
				TF:        fibTF,      // 🔥 P0-02: 使用真实TF
				Level:     level.Price,
				BandLo:    level.Price,
				BandHi:    level.Price,
				IsFresh:   isFresh,    // 🔥 P0-02: 使用真实 IsFresh
				StrengthZ: strengthZ,  // 🔥 P0-03: 透传或启发式
				VolRatio:  volRatio,   // 🔥 P0-03: 透传
				Meta: map[string]interface{}{
					"source_type":          "fibonacci",
					"level_ratio":          level.Ratio,
					"importance":           level.Importance,
					"is_golden":            level.IsGoldenRatio,
					"trend_type":           retracement.TrendType.String(),
					"retracement_strength": retracement.Strength,
					"retracement_age":      retracement.Age,
					"heuristic_strengthz":  strengthZ != nil && retracement.Context == nil, // 🔥 标记是否为启发式
				},
			}

			candidates = append(candidates, candidate)
		}
	}

	// 从黄金口袋提取（如果存在）
	if data.GoldenPocket != nil && data.GoldenPocket.IsActive {
		pocket := data.GoldenPocket

		// 🔥 P0-02修复：计算黄金口袋的 IsFresh
		// 注意：GoldenPocket 当前没有 Context，使用 LastUpdate 和 MaxRetracementAge 估算
		isFresh := false
		if pocket.LastUpdate > 0 {
			ageMs := time.Now().UnixMilli() - pocket.LastUpdate
			ageHours := int(ageMs / (3600 * 1000))
			isFresh = ageHours <= data.Config.MaxRetracementAge
		}

		// 黄金口袋方向基于趋势上下文
		dir := "LONG"
		if pocket.TrendContext == TrendDownward {
			dir = "SHORT"
		}

		// 🔥 P0-03修复：黄金口袋的 StrengthZ（启发式，因为没有 Context）
		heuristicZ := 2.0
		strengthZ := &heuristicZ

		candidate := AnchorCandidate{
			Dir:       dir,
			Type:      AnchorFib,
			TF:        fibTF,      // 🔥 P0-02: 使用真实TF
			Level:     pocket.CenterPrice,
			BandLo:    pocket.PriceRange.Low,
			BandHi:    pocket.PriceRange.High,
			IsFresh:   isFresh,    // 🔥 P0-02: 使用真实 IsFresh
			StrengthZ: strengthZ,  // 🔥 P0-03: 启发式
			VolRatio:  nil,        // 🔥 P0-03: 黄金口袋暂无 VolRatio
			Meta: map[string]interface{}{
				"source_type":          "fibonacci",
				"level_type":           "golden_pocket",
				"pocket_strength":      pocket.Strength,
				"quality":              pocket.Quality,
				"trend_context":        pocket.TrendContext.String(),
				"touch_count":          len(pocket.TouchEvents),
				"heuristic_strengthz":  true, // 🔥 标记为启发式
			},
		}

		candidates = append(candidates, candidate)
	}

	return candidates
}

// === 工具函数 ===

// inferTimeframeFromMeta 从Meta信息推断时间框架
func inferTimeframeFromMeta(meta map[string]interface{}, defaultTF string) string {
	if meta == nil {
		return defaultTF
	}
	
	if tf, ok := meta["timeframe"].(string); ok {
		return tf
	}
	
	if tf, ok := meta["tf"].(string); ok {
		return tf
	}
	
	return defaultTF
}

// validateCandidate 验证候选锚点的基础字段
func validateCandidate(c AnchorCandidate) bool {
	// 基础字段验证
	if c.Dir != "LONG" && c.Dir != "SHORT" {
		return false
	}
	
	if c.Level <= 0 {
		return false
	}
	
	if c.BandLo <= 0 || c.BandHi <= 0 {
		return false
	}
	
	if c.BandLo > c.BandHi {
		return false
	}
	
	// StrengthZ范围验证（如果存在）
	if c.StrengthZ != nil {
		if *c.StrengthZ < -5.0 || *c.StrengthZ > 5.0 {
			log.Printf("⚠️ [AnchorEngine] 候选锚点StrengthZ异常: %.2f", *c.StrengthZ)
			return false
		}
	}
	
	// VolRatio范围验证（如果存在）
	if c.VolRatio != nil {
		if *c.VolRatio < 0 || *c.VolRatio > 20.0 {
			log.Printf("⚠️ [AnchorEngine] 候选锚点VolRatio异常: %.2f", *c.VolRatio)
			return false
		}
	}
	
	return true
}