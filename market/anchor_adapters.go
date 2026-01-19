package market

import (
	"log"
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
func (ae *AnchorEngine) fromSR(data *SupportResistanceData, timeframes map[string][]Kline) []AnchorCandidate {
	var candidates []AnchorCandidate
	
	if data == nil || len(data.KeyLevels) == 0 {
		return candidates
	}
	
	for _, level := range data.KeyLevels {
		// 确定方向
		dir := "LONG"
		if level.Type == "resistance" {
			dir = "SHORT"
		}
		
		// 判断是否为转换线（flip）- 支撑阻力转换是SR的核心特征
		anchorType := AnchorSRFlip
		
		candidate := AnchorCandidate{
			Dir:    dir,
			Type:   anchorType,
			TF:     "30m", // SR通常基于中等时间框架
			Level:  level.Price,
			BandLo: level.Price,
			BandHi: level.Price,
			Meta: map[string]interface{}{
				"source_type": "support_resistance",
				"level_type":  level.Type,
				"hit_count":   level.HitCount,
				"strength":    level.Strength,
			},
		}
		
		// 基于hit_count判断是否fresh (触碰次数少的更fresh)
		if level.HitCount <= 2 {
			candidate.IsFresh = true
		}
		
		// 基于strength设置strengthZ (简单映射：假设50为均值，20为标准差)
		if level.Strength > 0 {
			strengthZ := (level.Strength - 50) / 20
			candidate.StrengthZ = &strengthZ
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
func (ae *AnchorEngine) fromFib(data *FibonacciData, timeframes map[string][]Kline) []AnchorCandidate {
	var candidates []AnchorCandidate
	
	if data == nil {
		return candidates
	}
	
	// 从回调级别提取
	for _, retracement := range data.Retracements {
		if !retracement.IsActive {
			continue
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
			
			candidate := AnchorCandidate{
				Dir:    dir,
				Type:   AnchorFib,
				TF:     "30m", // 斐波纳契通常基于中等时间框架
				Level:  level.Price,
				BandLo: level.Price,
				BandHi: level.Price,
				IsFresh: true, // 斐波纳契级别通常认为是"fresh"的
				Meta: map[string]interface{}{
					"source_type":    "fibonacci",
					"level_ratio":    level.Ratio,
					"importance":     level.Importance,
					"is_golden":      level.IsGoldenRatio,
					"trend_type":     retracement.TrendType.String(),
					"retracement_strength": retracement.Strength,
				},
			}
			
			// 黄金比例级别额外加分：设置较高的隐含强度
			if level.IsGoldenRatio {
				strengthZ := 1.5
				candidate.StrengthZ = &strengthZ
			}
			
			candidates = append(candidates, candidate)
		}
	}
	
	// 从黄金口袋提取（如果存在）
	if data.GoldenPocket != nil && data.GoldenPocket.IsActive {
		pocket := data.GoldenPocket
		
		// 黄金口袋方向基于趋势上下文
		dir := "LONG"
		if pocket.TrendContext == TrendDownward {
			dir = "SHORT"
		}
		
		candidate := AnchorCandidate{
			Dir:    dir,
			Type:   AnchorFib,
			TF:     "30m",
			Level:  pocket.CenterPrice,
			BandLo: pocket.PriceRange.Low,
			BandHi: pocket.PriceRange.High,
			IsFresh: true, // 黄金口袋总是被认为是fresh
			Meta: map[string]interface{}{
				"source_type":     "fibonacci",
				"level_type":      "golden_pocket",
				"pocket_strength": pocket.Strength,
				"quality":         pocket.Quality,
				"trend_context":   pocket.TrendContext.String(),
				"touch_count":     len(pocket.TouchEvents),
			},
		}
		
		// 黄金口袋给予高强度评分
		strengthZ := 2.0
		candidate.StrengthZ = &strengthZ
		
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