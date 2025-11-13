package market

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// SupplyDemandAnalyzer 供给需求区分析器
type SupplyDemandAnalyzer struct {
	config SDConfig
}

// NewSupplyDemandAnalyzer 创建新的供需区分析器
func NewSupplyDemandAnalyzer() *SupplyDemandAnalyzer {
	return &SupplyDemandAnalyzer{
		config: defaultSDConfig,
	}
}

// NewSupplyDemandAnalyzerWithConfig 使用自定义配置创建分析器
func NewSupplyDemandAnalyzerWithConfig(config SDConfig) *SupplyDemandAnalyzer {
	return &SupplyDemandAnalyzer{
		config: config,
	}
}

// Analyze 分析K线数据识别供需区
func (sda *SupplyDemandAnalyzer) Analyze(klines []Kline) *SupplyDemandData {
	if len(klines) < 10 {
		// 返回空数据结构而不是nil，避免后续处理报错
		return &SupplyDemandData{
			SupplyZones:  []*SupplyDemandZone{},
			DemandZones:  []*SupplyDemandZone{},
			ActiveZones:  []*SupplyDemandZone{},
			Config:       &sda.config,
			Statistics:   &SDStatistics{},
			LastAnalysis: time.Now().UnixMilli(),
		}
	}

	var supplyZones []*SupplyDemandZone
	var demandZones []*SupplyDemandZone

	// 识别供给区
	supplyZones = sda.identifySupplyZones(klines)

	// 识别需求区
	demandZones = sda.identifyDemandZones(klines)

	// 合并并排序所有区域
	allZones := append(supplyZones, demandZones...)
	sda.updateZoneStatuses(allZones, klines)

	// 筛选活跃区域
	activeZones := sda.filterActiveZones(allZones)
	
	// 如果复杂模式识别没有找到足够的区域，使用简单的高低点方法作为补充
	if len(activeZones) < 2 {
		backupZones := sda.identifyBasicZones(klines)
		for _, zone := range backupZones {
			if !sda.isZoneOverlapping(zone, allZones) {
				allZones = append(allZones, zone)
				if zone.IsActive {
					activeZones = append(activeZones, zone)
				}
				if zone.Type == SupplyZone {
					supplyZones = append(supplyZones, zone)
				} else {
					demandZones = append(demandZones, zone)
				}
			}
		}
	}

	// 计算统计信息
	stats := sda.calculateStatistics(supplyZones, demandZones, activeZones)

	return &SupplyDemandData{
		SupplyZones:  supplyZones,
		DemandZones:  demandZones,
		ActiveZones:  activeZones,
		Config:       &sda.config,
		Statistics:   stats,
		LastAnalysis: time.Now().UnixMilli(),
	}
}

// identifySupplyZones 识别供给区
func (sda *SupplyDemandAnalyzer) identifySupplyZones(klines []Kline) []*SupplyDemandZone {
	var zones []*SupplyDemandZone

	// 遍历K线寻找供给区模式
	for i := 5; i < len(klines)-5; i++ {
		// Drop-Base-Drop 模式
		if zone := sda.identifyDropBaseDrop(klines, i); zone != nil {
			zones = append(zones, zone)
		}

		// Rally-Base-Drop 模式（订单区块）
		if zone := sda.identifyRallyBaseDrop(klines, i); zone != nil {
			zones = append(zones, zone)
		}

		// 新鲜供给区
		if zone := sda.identifyFreshSupply(klines, i); zone != nil {
			zones = append(zones, zone)
		}
	}

	// 过滤和优化区域
	zones = sda.filterOverlappingZones(zones)
	
	// 计算区域强度和质量
	for _, zone := range zones {
		sda.calculateZoneStrength(zone, klines)
		sda.assessZoneQuality(zone)
	}

	return zones
}

// identifyDemandZones 识别需求区
func (sda *SupplyDemandAnalyzer) identifyDemandZones(klines []Kline) []*SupplyDemandZone {
	var zones []*SupplyDemandZone

	// 遍历K线寻找需求区模式
	for i := 5; i < len(klines)-5; i++ {
		// Rally-Base-Rally 模式
		if zone := sda.identifyRallyBaseRally(klines, i); zone != nil {
			zones = append(zones, zone)
		}

		// Drop-Base-Rally 模式（订单区块）
		if zone := sda.identifyDropBaseRally(klines, i); zone != nil {
			zones = append(zones, zone)
		}

		// 新鲜需求区
		if zone := sda.identifyFreshDemand(klines, i); zone != nil {
			zones = append(zones, zone)
		}
	}

	// 过滤和优化区域
	zones = sda.filterOverlappingZones(zones)
	
	// 计算区域强度和质量
	for _, zone := range zones {
		sda.calculateZoneStrength(zone, klines)
		sda.assessZoneQuality(zone)
	}

	return zones
}

// identifyDropBaseDrop 识别下跌-整理-下跌模式（供给区）
func (sda *SupplyDemandAnalyzer) identifyDropBaseDrop(klines []Kline, centerIndex int) *SupplyDemandZone {
	if centerIndex < 5 || centerIndex >= len(klines)-5 {
		return nil
	}

	// 寻找整理区域
	baseStart, baseEnd := sda.findBaseArea(klines, centerIndex, false)
	if baseStart == -1 || baseEnd == -1 {
		return nil
	}

	// 验证左侧下跌
	leftDrop := sda.validateLeftMove(klines, baseStart, false)
	if !leftDrop {
		return nil
	}

	// 验证右侧下跌
	rightDrop := sda.validateRightMove(klines, baseEnd, false)
	if !rightDrop {
		return nil
	}

	// 计算区域边界
	high := sda.findHighestHigh(klines, baseStart, baseEnd)
	low := sda.findLowestLow(klines, baseStart, baseEnd)

	// 创建供给区
	zone := &SupplyDemandZone{
		ID:           fmt.Sprintf("supply_%d_%d", baseStart, baseEnd),
		Type:         SupplyZone,
		UpperBound:   high,
		LowerBound:   low,
		CenterPrice:  (high + low) / 2,
		Width:        high - low,
		WidthPercent: (high - low) / low * 100,
		Origin: &ZoneOrigin{
			KlineIndex:    centerIndex,
			PatternType:   DropBaseDrop,
			ImpulseMove:   sda.calculateImpulseMove(klines, baseEnd, false),
			ImpulseVolume: sda.calculateImpulseVolume(klines, baseEnd, false),
			TimeFrame:     sda.config.TimeFrames[0],
			Confirmation:  true,
		},
		Status:       StatusFresh,
		CreationTime: klines[centerIndex].OpenTime,
		IsActive:     true,
		IsBroken:     false,
	}

	// 计算成交量分布
	zone.VolumeProfile = sda.calculateZoneVolumeProfile(klines, baseStart, baseEnd)
	zone.Volume = zone.VolumeProfile.TotalVolume

	return zone
}

// identifyRallyBaseRally 识别上涨-整理-上涨模式（需求区）
func (sda *SupplyDemandAnalyzer) identifyRallyBaseRally(klines []Kline, centerIndex int) *SupplyDemandZone {
	if centerIndex < 5 || centerIndex >= len(klines)-5 {
		return nil
	}

	// 寻找整理区域
	baseStart, baseEnd := sda.findBaseArea(klines, centerIndex, true)
	if baseStart == -1 || baseEnd == -1 {
		return nil
	}

	// 验证左侧上涨
	leftRally := sda.validateLeftMove(klines, baseStart, true)
	if !leftRally {
		return nil
	}

	// 验证右侧上涨
	rightRally := sda.validateRightMove(klines, baseEnd, true)
	if !rightRally {
		return nil
	}

	// 计算区域边界
	high := sda.findHighestHigh(klines, baseStart, baseEnd)
	low := sda.findLowestLow(klines, baseStart, baseEnd)

	// 创建需求区
	zone := &SupplyDemandZone{
		ID:           fmt.Sprintf("demand_%d_%d", baseStart, baseEnd),
		Type:         DemandZone,
		UpperBound:   high,
		LowerBound:   low,
		CenterPrice:  (high + low) / 2,
		Width:        high - low,
		WidthPercent: (high - low) / low * 100,
		Origin: &ZoneOrigin{
			KlineIndex:    centerIndex,
			PatternType:   RallyBaseRally,
			ImpulseMove:   sda.calculateImpulseMove(klines, baseEnd, true),
			ImpulseVolume: sda.calculateImpulseVolume(klines, baseEnd, true),
			TimeFrame:     sda.config.TimeFrames[0],
			Confirmation:  true,
		},
		Status:       StatusFresh,
		CreationTime: klines[centerIndex].OpenTime,
		IsActive:     true,
		IsBroken:     false,
	}

	// 计算成交量分布
	zone.VolumeProfile = sda.calculateZoneVolumeProfile(klines, baseStart, baseEnd)
	zone.Volume = zone.VolumeProfile.TotalVolume

	return zone
}

// identifyRallyBaseDrop 识别上涨-整理-下跌模式（订单区块）
func (sda *SupplyDemandAnalyzer) identifyRallyBaseDrop(klines []Kline, centerIndex int) *SupplyDemandZone {
	if centerIndex < 5 || centerIndex >= len(klines)-5 {
		return nil
	}

	// 寻找整理区域
	baseStart, baseEnd := sda.findBaseArea(klines, centerIndex, false)
	if baseStart == -1 || baseEnd == -1 {
		return nil
	}

	// 验证左侧上涨
	leftRally := sda.validateLeftMove(klines, baseStart, true)
	if !leftRally {
		return nil
	}

	// 验证右侧下跌
	rightDrop := sda.validateRightMove(klines, baseEnd, false)
	if !rightDrop {
		return nil
	}

	// 计算区域边界
	high := sda.findHighestHigh(klines, baseStart, baseEnd)
	low := sda.findLowestLow(klines, baseStart, baseEnd)

	// 创建供给区（订单区块）
	zone := &SupplyDemandZone{
		ID:           fmt.Sprintf("supply_ob_%d_%d", baseStart, baseEnd),
		Type:         SupplyZone,
		UpperBound:   high,
		LowerBound:   low,
		CenterPrice:  (high + low) / 2,
		Width:        high - low,
		WidthPercent: (high - low) / low * 100,
		Origin: &ZoneOrigin{
			KlineIndex:    centerIndex,
			PatternType:   RallyBaseDropOB,
			ImpulseMove:   sda.calculateImpulseMove(klines, baseEnd, false),
			ImpulseVolume: sda.calculateImpulseVolume(klines, baseEnd, false),
			TimeFrame:     sda.config.TimeFrames[0],
			Confirmation:  true,
		},
		Status:       StatusFresh,
		CreationTime: klines[centerIndex].OpenTime,
		IsActive:     true,
		IsBroken:     false,
	}

	// 计算成交量分布
	zone.VolumeProfile = sda.calculateZoneVolumeProfile(klines, baseStart, baseEnd)
	zone.Volume = zone.VolumeProfile.TotalVolume

	return zone
}

// identifyDropBaseRally 识别下跌-整理-上涨模式（订单区块）
func (sda *SupplyDemandAnalyzer) identifyDropBaseRally(klines []Kline, centerIndex int) *SupplyDemandZone {
	if centerIndex < 5 || centerIndex >= len(klines)-5 {
		return nil
	}

	// 寻找整理区域
	baseStart, baseEnd := sda.findBaseArea(klines, centerIndex, true)
	if baseStart == -1 || baseEnd == -1 {
		return nil
	}

	// 验证左侧下跌
	leftDrop := sda.validateLeftMove(klines, baseStart, false)
	if !leftDrop {
		return nil
	}

	// 验证右侧上涨
	rightRally := sda.validateRightMove(klines, baseEnd, true)
	if !rightRally {
		return nil
	}

	// 计算区域边界
	high := sda.findHighestHigh(klines, baseStart, baseEnd)
	low := sda.findLowestLow(klines, baseStart, baseEnd)

	// 创建需求区（订单区块）
	zone := &SupplyDemandZone{
		ID:           fmt.Sprintf("demand_ob_%d_%d", baseStart, baseEnd),
		Type:         DemandZone,
		UpperBound:   high,
		LowerBound:   low,
		CenterPrice:  (high + low) / 2,
		Width:        high - low,
		WidthPercent: (high - low) / low * 100,
		Origin: &ZoneOrigin{
			KlineIndex:    centerIndex,
			PatternType:   DropBaseRallyOB,
			ImpulseMove:   sda.calculateImpulseMove(klines, baseEnd, true),
			ImpulseVolume: sda.calculateImpulseVolume(klines, baseEnd, true),
			TimeFrame:     sda.config.TimeFrames[0],
			Confirmation:  true,
		},
		Status:       StatusFresh,
		CreationTime: klines[centerIndex].OpenTime,
		IsActive:     true,
		IsBroken:     false,
	}

	// 计算成交量分布
	zone.VolumeProfile = sda.calculateZoneVolumeProfile(klines, baseStart, baseEnd)
	zone.Volume = zone.VolumeProfile.TotalVolume

	return zone
}

// identifyFreshSupply 识别新鲜供给区
func (sda *SupplyDemandAnalyzer) identifyFreshSupply(klines []Kline, index int) *SupplyDemandZone {
	if index < 3 || index >= len(klines)-3 {
		return nil
	}

	// 寻找显著的价格下跌
	priceChange := (klines[index].Close - klines[index-3].Close) / klines[index-3].Close
	if priceChange > -sda.config.MinImpulsePercent {
		return nil
	}

	// 检查成交量确认
	avgVolume := sda.calculateAverageVolume(klines, index-10, index)
	if klines[index].Volume < avgVolume*sda.config.MinVolumeFactor {
		return nil
	}

	// 创建供给区
	high := klines[index-1].High
	low := klines[index].Low
	
	zone := &SupplyDemandZone{
		ID:           fmt.Sprintf("fresh_supply_%d", index),
		Type:         SupplyZone,
		UpperBound:   high,
		LowerBound:   low,
		CenterPrice:  (high + low) / 2,
		Width:        high - low,
		WidthPercent: (high - low) / low * 100,
		Origin: &ZoneOrigin{
			KlineIndex:    index,
			PatternType:   FreshSupply,
			ImpulseMove:   math.Abs(priceChange),
			ImpulseVolume: klines[index].Volume,
			TimeFrame:     sda.config.TimeFrames[0],
			Confirmation:  false,
		},
		Status:       StatusFresh,
		CreationTime: klines[index].OpenTime,
		IsActive:     true,
		IsBroken:     false,
	}

	return zone
}

// identifyFreshDemand 识别新鲜需求区
func (sda *SupplyDemandAnalyzer) identifyFreshDemand(klines []Kline, index int) *SupplyDemandZone {
	if index < 3 || index >= len(klines)-3 {
		return nil
	}

	// 寻找显著的价格上涨
	priceChange := (klines[index].Close - klines[index-3].Close) / klines[index-3].Close
	if priceChange < sda.config.MinImpulsePercent {
		return nil
	}

	// 检查成交量确认
	avgVolume := sda.calculateAverageVolume(klines, index-10, index)
	if klines[index].Volume < avgVolume*sda.config.MinVolumeFactor {
		return nil
	}

	// 创建需求区
	high := klines[index].High
	low := klines[index-1].Low
	
	zone := &SupplyDemandZone{
		ID:           fmt.Sprintf("fresh_demand_%d", index),
		Type:         DemandZone,
		UpperBound:   high,
		LowerBound:   low,
		CenterPrice:  (high + low) / 2,
		Width:        high - low,
		WidthPercent: (high - low) / low * 100,
		Origin: &ZoneOrigin{
			KlineIndex:    index,
			PatternType:   FreshDemand,
			ImpulseMove:   priceChange,
			ImpulseVolume: klines[index].Volume,
			TimeFrame:     sda.config.TimeFrames[0],
			Confirmation:  false,
		},
		Status:       StatusFresh,
		CreationTime: klines[index].OpenTime,
		IsActive:     true,
		IsBroken:     false,
	}

	return zone
}

// findBaseArea 寻找整理区域
func (sda *SupplyDemandAnalyzer) findBaseArea(klines []Kline, centerIndex int, isRally bool) (int, int) {
	start := centerIndex - 3
	end := centerIndex + 3

	// 确保索引有效
	if start < 0 {
		start = 0
	}
	if end >= len(klines) {
		end = len(klines) - 1
	}

	// 计算整理区域的���格范围
	high := klines[start].High
	low := klines[start].Low

	for i := start; i <= end; i++ {
		if klines[i].High > high {
			high = klines[i].High
		}
		if klines[i].Low < low {
			low = klines[i].Low
		}
	}

	// 检查整理区域是否符合要求
	rangePercent := (high - low) / low
	if rangePercent < sda.config.MinBasePercent || rangePercent > sda.config.MaxBasePercent {
		return -1, -1
	}

	return start, end
}

// validateLeftMove 验证左侧移动
func (sda *SupplyDemandAnalyzer) validateLeftMove(klines []Kline, baseStart int, isRally bool) bool {
	if baseStart < 5 {
		return false
	}

	startPrice := klines[baseStart-5].Close
	endPrice := klines[baseStart].Close

	priceChange := (endPrice - startPrice) / startPrice

	if isRally {
		return priceChange > sda.config.MinImpulsePercent
	} else {
		return priceChange < -sda.config.MinImpulsePercent
	}
}

// validateRightMove 验证右侧移动
func (sda *SupplyDemandAnalyzer) validateRightMove(klines []Kline, baseEnd int, isRally bool) bool {
	if baseEnd >= len(klines)-5 {
		return false
	}

	startPrice := klines[baseEnd].Close
	endPrice := klines[baseEnd+5].Close

	priceChange := (endPrice - startPrice) / startPrice

	if isRally {
		return priceChange > sda.config.MinImpulsePercent
	} else {
		return priceChange < -sda.config.MinImpulsePercent
	}
}

// findHighestHigh 找到指定范围内的最高价
func (sda *SupplyDemandAnalyzer) findHighestHigh(klines []Kline, start, end int) float64 {
	high := klines[start].High
	for i := start; i <= end && i < len(klines); i++ {
		if klines[i].High > high {
			high = klines[i].High
		}
	}
	return high
}

// findLowestLow 找到指定范围内的最低价
func (sda *SupplyDemandAnalyzer) findLowestLow(klines []Kline, start, end int) float64 {
	low := klines[start].Low
	for i := start; i <= end && i < len(klines); i++ {
		if klines[i].Low < low {
			low = klines[i].Low
		}
	}
	return low
}

// calculateImpulseMove 计算冲击移动幅度
func (sda *SupplyDemandAnalyzer) calculateImpulseMove(klines []Kline, startIndex int, isRally bool) float64 {
	if startIndex >= len(klines)-5 {
		return 0
	}

	startPrice := klines[startIndex].Close
	endPrice := klines[startIndex+5].Close

	if isRally {
		// 找到最高价
		for i := startIndex; i <= startIndex+5 && i < len(klines); i++ {
			if klines[i].High > endPrice {
				endPrice = klines[i].High
			}
		}
		return (endPrice - startPrice) / startPrice
	} else {
		// 找到最低价
		for i := startIndex; i <= startIndex+5 && i < len(klines); i++ {
			if klines[i].Low < endPrice {
				endPrice = klines[i].Low
			}
		}
		return (startPrice - endPrice) / startPrice
	}
}

// calculateImpulseVolume 计算冲击成交量
func (sda *SupplyDemandAnalyzer) calculateImpulseVolume(klines []Kline, startIndex int, isRally bool) float64 {
	if startIndex >= len(klines)-5 {
		return 0
	}

	totalVolume := 0.0
	for i := startIndex; i <= startIndex+5 && i < len(klines); i++ {
		totalVolume += klines[i].Volume
	}

	return totalVolume
}

// calculateAverageVolume 计算平均成交量
func (sda *SupplyDemandAnalyzer) calculateAverageVolume(klines []Kline, start, end int) float64 {
	if start < 0 {
		start = 0
	}
	if end >= len(klines) {
		end = len(klines) - 1
	}

	totalVolume := 0.0
	count := 0

	for i := start; i <= end; i++ {
		totalVolume += klines[i].Volume
		count++
	}

	if count == 0 {
		return 0
	}

	return totalVolume / float64(count)
}

// calculateZoneVolumeProfile 计算区域成交量分布
func (sda *SupplyDemandAnalyzer) calculateZoneVolumeProfile(klines []Kline, start, end int) *ZoneVP {
	totalVolume := 0.0
	buyVolume := 0.0
	sellVolume := 0.0

	for i := start; i <= end && i < len(klines); i++ {
		volume := klines[i].Volume
		totalVolume += volume

		// 估算买卖比例
		if klines[i].Close > klines[i].Open {
			buyVolume += volume * 0.7
			sellVolume += volume * 0.3
		} else {
			buyVolume += volume * 0.3
			sellVolume += volume * 0.7
		}
	}

	imbalance := 0.0
	if sellVolume > 0 {
		imbalance = buyVolume / sellVolume
	}

	return &ZoneVP{
		TotalVolume:     totalVolume,
		BuyVolume:       buyVolume,
		SellVolume:      sellVolume,
		VolumeAtOrigin:  totalVolume / float64(end-start+1),
		VolumeImbalance: imbalance,
	}
}

// filterOverlappingZones 过滤重叠区域
func (sda *SupplyDemandAnalyzer) filterOverlappingZones(zones []*SupplyDemandZone) []*SupplyDemandZone {
	if len(zones) <= 1 {
		return zones
	}

	// 按强度排序
	sort.Slice(zones, func(i, j int) bool {
		return zones[i].Strength > zones[j].Strength
	})

	var filtered []*SupplyDemandZone
	
	for _, zone := range zones {
		overlaps := false
		for _, existing := range filtered {
			if sda.zonesOverlap(zone, existing) {
				overlaps = true
				break
			}
		}
		
		if !overlaps {
			filtered = append(filtered, zone)
		}
	}

	return filtered
}

// zonesOverlap 检查两个区域是否重叠
func (sda *SupplyDemandAnalyzer) zonesOverlap(zone1, zone2 *SupplyDemandZone) bool {
	return !(zone1.UpperBound < zone2.LowerBound || zone2.UpperBound < zone1.LowerBound)
}

// calculateZoneStrength 计算区域强度
func (sda *SupplyDemandAnalyzer) calculateZoneStrength(zone *SupplyDemandZone, klines []Kline) {
	strength := 0.0

	// 基于冲击移动的强度
	strength += zone.Origin.ImpulseMove * 50

	// 基于成交量的强度
	avgVolume := sda.calculateAverageVolume(klines, 0, len(klines)-1)
	if avgVolume > 0 {
		volumeRatio := zone.Volume / avgVolume
		strength += math.Min(volumeRatio, 5.0) * 10
	}

	// 基于区域宽度的强度（窄区域更强）
	if zone.WidthPercent > 0 {
		strength += (5.0 / zone.WidthPercent) * 5
	}

	// 基于模式类型的强度
	switch zone.Origin.PatternType {
	case DropBaseDrop, RallyBaseRally:
		strength += 15 // 经典模式
	case RallyBaseDropOB, DropBaseRallyOB:
		strength += 12 // 订单区块
	case FreshSupply, FreshDemand:
		strength += 8 // 新鲜区域
	}

	zone.Strength = math.Min(strength, 100.0)
}

// assessZoneQuality 评估区域质量
func (sda *SupplyDemandAnalyzer) assessZoneQuality(zone *SupplyDemandZone) {
	score := zone.Strength

	// 基于成交量不平衡调整质量
	if zone.VolumeProfile != nil {
		if zone.Type == SupplyZone && zone.VolumeProfile.VolumeImbalance < 0.8 {
			score += 10 // 供给区卖盘占优
		} else if zone.Type == DemandZone && zone.VolumeProfile.VolumeImbalance > 1.2 {
			score += 10 // 需求区买盘占优
		}
	}

	// 基于确认状态调整质量
	if zone.Origin.Confirmation {
		score += 5
	}

	if score >= 80 {
		zone.Quality = QualityStrong
	} else if score >= 65 {
		zone.Quality = QualityGood
	} else if score >= 50 {
		zone.Quality = QualityModerate
	} else {
		zone.Quality = QualityWeak
	}
}

// updateZoneStatuses 更新区域状态
func (sda *SupplyDemandAnalyzer) updateZoneStatuses(zones []*SupplyDemandZone, klines []Kline) {
	if len(klines) == 0 {
		return
	}

	currentTime := klines[len(klines)-1].OpenTime
	currentPrice := klines[len(klines)-1].Close

	for _, zone := range zones {
		// 检查年龄
		age := int((currentTime - zone.CreationTime) / (3600 * 1000)) // 小时
		if age > sda.config.MaxZoneAge {
			zone.Status = StatusExpired
			zone.IsActive = false
			continue
		}

		// 检查是否被突破
		if sda.isZoneBroken(zone, klines, currentPrice) {
			zone.Status = StatusBroken
			zone.IsBroken = true
			zone.IsActive = false
			zone.BreakTime = currentTime
			continue
		}

		// 检查触及次数
		touchCount := sda.countZoneTouches(zone, klines)
		zone.TouchCount = touchCount

		if touchCount > sda.config.MaxTouchCount {
			zone.Status = StatusWeakened
		} else if touchCount > 0 {
			zone.Status = StatusTested
			zone.LastTouch = currentTime
		}

		// 验证区域反应
		if sda.config.EnableValidation {
			zone.Validation = sda.validateZoneReaction(zone, klines)
		}
	}
}

// isZoneBroken 检查区域是否被突破
func (sda *SupplyDemandAnalyzer) isZoneBroken(zone *SupplyDemandZone, klines []Kline, currentPrice float64) bool {
	threshold := sda.config.BreakoutThreshold

	if zone.Type == SupplyZone {
		// 供给区被向上突破
		return currentPrice > zone.UpperBound*(1+threshold)
	} else {
		// 需求区被向下突破
		return currentPrice < zone.LowerBound*(1-threshold)
	}
}

// countZoneTouches 计算区域触及次数
func (sda *SupplyDemandAnalyzer) countZoneTouches(zone *SupplyDemandZone, klines []Kline) int {
	count := 0
	
	for i := zone.Origin.KlineIndex + 1; i < len(klines); i++ {
		if sda.priceInZone(klines[i].High, klines[i].Low, zone) {
			count++
		}
	}

	return count
}

// priceInZone 检查价格是否在区域内
func (sda *SupplyDemandAnalyzer) priceInZone(high, low float64, zone *SupplyDemandZone) bool {
	return !(high < zone.LowerBound || low > zone.UpperBound)
}

// validateZoneReaction 验证区域反应
func (sda *SupplyDemandAnalyzer) validateZoneReaction(zone *SupplyDemandZone, klines []Kline) *Validation {
	// 找到最近的测试
	var testIndex = -1
	for i := len(klines) - 1; i > zone.Origin.KlineIndex; i-- {
		if sda.priceInZone(klines[i].High, klines[i].Low, zone) {
			testIndex = i
			break
		}
	}

	if testIndex == -1 {
		return &Validation{
			HasReaction: false,
		}
	}

	// 检查测试后的反应
	reactionBars := 3
	if testIndex+reactionBars >= len(klines) {
		reactionBars = len(klines) - testIndex - 1
	}

	if reactionBars <= 0 {
		return &Validation{
			HasReaction: false,
		}
	}

	testPrice := klines[testIndex].Close
	reactionPrice := klines[testIndex+reactionBars].Close
	reactionStrength := math.Abs(reactionPrice-testPrice) / testPrice

	hasReaction := false
	if zone.Type == SupplyZone && reactionPrice < testPrice {
		hasReaction = reactionStrength > 0.01 // 1%反应
	} else if zone.Type == DemandZone && reactionPrice > testPrice {
		hasReaction = reactionStrength > 0.01 // 1%反应
	}

	return &Validation{
		HasReaction:      hasReaction,
		ReactionStrength: reactionStrength,
		TimeInZone:       klines[testIndex+reactionBars].OpenTime - klines[testIndex].OpenTime,
		VolumeAtTest:     klines[testIndex].Volume,
		PriceAction:      sda.analyzePriceAction(klines, testIndex, testIndex+reactionBars),
	}
}

// analyzePriceAction 分析价格行为
func (sda *SupplyDemandAnalyzer) analyzePriceAction(klines []Kline, start, end int) string {
	if start >= end || end >= len(klines) {
		return "unknown"
	}

	startPrice := klines[start].Close
	endPrice := klines[end].Close
	change := (endPrice - startPrice) / startPrice

	if change > 0.02 {
		return "strong_bullish"
	} else if change > 0.01 {
		return "bullish"
	} else if change < -0.02 {
		return "strong_bearish"
	} else if change < -0.01 {
		return "bearish"
	} else {
		return "sideways"
	}
}

// filterActiveZones 筛选活跃区域
func (sda *SupplyDemandAnalyzer) filterActiveZones(zones []*SupplyDemandZone) []*SupplyDemandZone {
	var active []*SupplyDemandZone

	for _, zone := range zones {
		if zone.IsActive && zone.Strength >= sda.config.QualityThreshold*100 {
			active = append(active, zone)
		}
	}

	return active
}

// calculateStatistics 计算统计信息
func (sda *SupplyDemandAnalyzer) calculateStatistics(supplyZones, demandZones, activeZones []*SupplyDemandZone) *SDStatistics {
	stats := &SDStatistics{
		TotalSupplyZones: len(supplyZones),
		TotalDemandZones: len(demandZones),
	}

	// 计算活跃区域数量
	for _, zone := range activeZones {
		if zone.Type == SupplyZone {
			stats.ActiveSupplyZones++
		} else {
			stats.ActiveDemandZones++
		}
	}

	// 计算平均强度和宽度
	if len(activeZones) > 0 {
		totalStrength := 0.0
		totalWidth := 0.0
		
		for _, zone := range activeZones {
			totalStrength += zone.Strength
			totalWidth += zone.WidthPercent
		}
		
		stats.AvgZoneStrength = totalStrength / float64(len(activeZones))
		stats.AvgZoneWidth = totalWidth / float64(len(activeZones))
	}

	// 计算成功率等指标
	allZones := append(supplyZones, demandZones...)
	if len(allZones) > 0 {
		successCount := 0
		breakoutCount := 0
		reactionCount := 0

		for _, zone := range allZones {
			if zone.Validation != nil {
				if zone.Validation.HasReaction {
					successCount++
					reactionCount++
				}
			}
			
			if zone.IsBroken {
				breakoutCount++
			}
		}

		stats.SuccessRate = float64(successCount) / float64(len(allZones)) * 100
		stats.BreakoutRate = float64(breakoutCount) / float64(len(allZones)) * 100
		stats.ReactionRate = float64(reactionCount) / float64(len(allZones)) * 100
	}

	return stats
}

// UpdateConfig 更新配置
func (sda *SupplyDemandAnalyzer) UpdateConfig(config SDConfig) {
	sda.config = config
}

// GetConfig 获取当前配置
func (sda *SupplyDemandAnalyzer) GetConfig() SDConfig {
	return sda.config
}

// GenerateSignals 生成基于供需区的交易信号
func (sda *SupplyDemandAnalyzer) GenerateSignals(sdData *SupplyDemandData, currentPrice float64) []*SDSignal {
	if sdData == nil {
		return nil
	}

	var signals []*SDSignal
	timestamp := time.Now().UnixMilli()

	// 检查活跃区域的信号
	for _, zone := range sdData.ActiveZones {
		if signal := sda.generateZoneSignal(zone, currentPrice, timestamp); signal != nil {
			signals = append(signals, signal)
		}
	}

	// 检查新鲜区域信号
	if signal := sda.generateFreshZoneSignal(sdData, currentPrice, timestamp); signal != nil {
		signals = append(signals, signal)
	}

	// 按置信度排序
	sort.Slice(signals, func(i, j int) bool {
		return signals[i].Confidence > signals[j].Confidence
	})

	return signals
}

// generateZoneSignal 为单个区域生成信号
func (sda *SupplyDemandAnalyzer) generateZoneSignal(zone *SupplyDemandZone, currentPrice float64, timestamp int64) *SDSignal {
	// 检查价格是否接近区域
	distanceToZone := sda.calculateDistanceToZone(zone, currentPrice)
	
	// 只为接近区域的价格生成信号
	if distanceToZone > 0.05 { // 5%范围外
		return nil
	}

	var signal *SDSignal

	// 检查是否在区域内
	inZone := currentPrice >= zone.LowerBound && currentPrice <= zone.UpperBound

	if inZone {
		// 在区域内，生成反弹信号
		signal = sda.generateBounceSignal(zone, currentPrice, timestamp)
	} else {
		// 接近区域，生成进入信号
		signal = sda.generateEntrySignal(zone, currentPrice, timestamp, distanceToZone)
	}

	return signal
}

// generateBounceSignal 生成区域反弹信号
func (sda *SupplyDemandAnalyzer) generateBounceSignal(zone *SupplyDemandZone, currentPrice float64, timestamp int64) *SDSignal {
	var action SignalAction
	var entry, stopLoss, takeProfit float64
	var description string

	if zone.Type == SupplyZone {
		action = ActionSell
		entry = currentPrice
		stopLoss = zone.UpperBound * 1.01
		takeProfit = currentPrice - (zone.Width * 2)
		description = fmt.Sprintf("在供给区%.2f-%.2f内，预期价格下跌", zone.LowerBound, zone.UpperBound)
	} else {
		action = ActionBuy
		entry = currentPrice
		stopLoss = zone.LowerBound * 0.99
		takeProfit = currentPrice + (zone.Width * 2)
		description = fmt.Sprintf("在需求区%.2f-%.2f内，预期价格上涨", zone.LowerBound, zone.UpperBound)
	}

	// 计算风险收益比
	risk := math.Abs(entry - stopLoss)
	reward := math.Abs(takeProfit - entry)
	riskReward := 0.0
	if risk > 0 {
		riskReward = reward / risk
	}

	// 计算置信度
	confidence := zone.Strength * 0.8
	if zone.Quality == QualityStrong {
		confidence += 10
	}
	if zone.Status == StatusFresh {
		confidence += 5
	}

	return &SDSignal{
		Type:         SDSignalZoneBounce,
		Zone:         zone,
		CurrentPrice: currentPrice,
		Action:       action,
		Entry:        entry,
		StopLoss:     stopLoss,
		TakeProfit:   takeProfit,
		RiskReward:   riskReward,
		Confidence:   math.Min(confidence, 100),
		Strength:     zone.Strength,
		Description:  description,
		Timestamp:    timestamp,
	}
}

// generateEntrySignal 生成区域进入信号
func (sda *SupplyDemandAnalyzer) generateEntrySignal(zone *SupplyDemandZone, currentPrice float64, timestamp int64, distance float64) *SDSignal {
	var action SignalAction
	var entry, stopLoss, takeProfit float64
	var description string

	if zone.Type == SupplyZone {
		if currentPrice > zone.UpperBound {
			// 价格在供给区上方，等待回测
			action = ActionSell
			entry = zone.UpperBound
			stopLoss = zone.UpperBound * 1.02
			takeProfit = zone.LowerBound
			description = fmt.Sprintf("等待回测供给区%.2f，准备做空", zone.UpperBound)
		} else {
			return nil // 价格在供给区下方，不生成信号
		}
	} else {
		if currentPrice < zone.LowerBound {
			// 价格在需求区下方，等待回测
			action = ActionBuy
			entry = zone.LowerBound
			stopLoss = zone.LowerBound * 0.98
			takeProfit = zone.UpperBound
			description = fmt.Sprintf("等待回测需求区%.2f，准备做多", zone.LowerBound)
		} else {
			return nil // 价格在需求区上方，不生成信号
		}
	}

	// 计算风险收益比
	risk := math.Abs(entry - stopLoss)
	reward := math.Abs(takeProfit - entry)
	riskReward := 0.0
	if risk > 0 {
		riskReward = reward / risk
	}

	// 计算置信度（距离越近置信度越高）
	confidence := zone.Strength * (1 - distance/0.05) * 0.7
	if zone.Quality == QualityStrong {
		confidence += 8
	}

	return &SDSignal{
		Type:         SDSignalZoneEntry,
		Zone:         zone,
		CurrentPrice: currentPrice,
		Action:       action,
		Entry:        entry,
		StopLoss:     stopLoss,
		TakeProfit:   takeProfit,
		RiskReward:   riskReward,
		Confidence:   math.Min(confidence, 100),
		Strength:     zone.Strength,
		Description:  description,
		Timestamp:    timestamp,
	}
}

// generateFreshZoneSignal 生成新鲜区域信号
func (sda *SupplyDemandAnalyzer) generateFreshZoneSignal(sdData *SupplyDemandData, currentPrice float64, timestamp int64) *SDSignal {
	// 寻找最新创建的高质量区域
	var freshZone *SupplyDemandZone
	var latestTime int64 = 0

	for _, zone := range sdData.ActiveZones {
		if (zone.Origin.PatternType == FreshSupply || zone.Origin.PatternType == FreshDemand) &&
			zone.Status == StatusFresh &&
			zone.Quality != QualityWeak &&
			zone.CreationTime > latestTime {
			
			freshZone = zone
			latestTime = zone.CreationTime
		}
	}

	if freshZone == nil {
		return nil
	}

	// 检查价格是否接近新鲜区域
	distance := sda.calculateDistanceToZone(freshZone, currentPrice)
	if distance > 0.03 { // 3%范围外
		return nil
	}

	var action SignalAction
	var entry, stopLoss, takeProfit float64
	var description string

	if freshZone.Type == SupplyZone {
		action = ActionSell
		entry = freshZone.CenterPrice
		stopLoss = freshZone.UpperBound * 1.015
		takeProfit = currentPrice - (freshZone.Width * 1.5)
		description = fmt.Sprintf("新鲜供给区%.2f，强阻力预期", freshZone.CenterPrice)
	} else {
		action = ActionBuy
		entry = freshZone.CenterPrice
		stopLoss = freshZone.LowerBound * 0.985
		takeProfit = currentPrice + (freshZone.Width * 1.5)
		description = fmt.Sprintf("新鲜需求区%.2f，强支撑预期", freshZone.CenterPrice)
	}

	// 计算风险收益比
	risk := math.Abs(entry - stopLoss)
	reward := math.Abs(takeProfit - entry)
	riskReward := 0.0
	if risk > 0 {
		riskReward = reward / risk
	}

	// 新鲜区域高置信度
	confidence := freshZone.Strength * 0.9 + 15

	return &SDSignal{
		Type:         SDSignalFreshZone,
		Zone:         freshZone,
		CurrentPrice: currentPrice,
		Action:       action,
		Entry:        entry,
		StopLoss:     stopLoss,
		TakeProfit:   takeProfit,
		RiskReward:   riskReward,
		Confidence:   math.Min(confidence, 100),
		Strength:     freshZone.Strength,
		Description:  description,
		Timestamp:    timestamp,
	}
}

// calculateDistanceToZone 计算价格到区域的距离
func (sda *SupplyDemandAnalyzer) calculateDistanceToZone(zone *SupplyDemandZone, currentPrice float64) float64 {
	if currentPrice >= zone.LowerBound && currentPrice <= zone.UpperBound {
		return 0 // 在区域内
	}

	var distance float64
	if currentPrice > zone.UpperBound {
		distance = (currentPrice - zone.UpperBound) / zone.UpperBound
	} else {
		distance = (zone.LowerBound - currentPrice) / zone.LowerBound
	}

	return distance
}

// FindNearestZones 查找最近的供需区
func (sda *SupplyDemandAnalyzer) FindNearestZones(sdData *SupplyDemandData, currentPrice float64, maxDistance float64) []*SupplyDemandZone {
	if sdData == nil {
		return nil
	}

	var nearZones []*SupplyDemandZone

	for _, zone := range sdData.ActiveZones {
		distance := sda.calculateDistanceToZone(zone, currentPrice)
		if distance <= maxDistance {
			nearZones = append(nearZones, zone)
		}
	}

	// 按距离排序
	sort.Slice(nearZones, func(i, j int) bool {
		dist1 := sda.calculateDistanceToZone(nearZones[i], currentPrice)
		dist2 := sda.calculateDistanceToZone(nearZones[j], currentPrice)
		return dist1 < dist2
	})

	return nearZones
}

// GetZonesByType 按类型获取区域
func (sda *SupplyDemandAnalyzer) GetZonesByType(sdData *SupplyDemandData, zoneType ZoneType) []*SupplyDemandZone {
	if sdData == nil {
		return nil
	}

	var zones []*SupplyDemandZone

	targetZones := sdData.ActiveZones
	if zoneType == SupplyZone {
		targetZones = sdData.SupplyZones
	} else if zoneType == DemandZone {
		targetZones = sdData.DemandZones
	}

	for _, zone := range targetZones {
		if zone.Type == zoneType && zone.IsActive {
			zones = append(zones, zone)
		}
	}

	return zones
}

// GetStrongestZones 获取最强的区域
func (sda *SupplyDemandAnalyzer) GetStrongestZones(sdData *SupplyDemandData, count int) []*SupplyDemandZone {
	if sdData == nil {
		return nil
	}

	// 复制活跃区域
	zones := make([]*SupplyDemandZone, len(sdData.ActiveZones))
	copy(zones, sdData.ActiveZones)

	// 按强度排序
	sort.Slice(zones, func(i, j int) bool {
		return zones[i].Strength > zones[j].Strength
	})

	// 返回最强的几个
	if count > len(zones) {
		count = len(zones)
	}

	return zones[:count]
}

// identifyBasicZones 识别基础供需区（基于近期高低点的简单方法）
func (sda *SupplyDemandAnalyzer) identifyBasicZones(klines []Kline) []*SupplyDemandZone {
	var zones []*SupplyDemandZone
	
	if len(klines) < 20 {
		return zones
	}
	
	// 寻找近期重要高低点
	recentPeriod := 20 // 最近20根K线
	start := len(klines) - recentPeriod
	if start < 0 {
		start = 0
	}
	
	// 找到最高点和最低点
	var highestIndex, lowestIndex int
	highest := klines[start].High
	lowest := klines[start].Low
	
	for i := start; i < len(klines); i++ {
		if klines[i].High > highest {
			highest = klines[i].High
			highestIndex = i
		}
		if klines[i].Low < lowest {
			lowest = klines[i].Low
			lowestIndex = i
		}
	}
	
	// 创建供给区（基于最高点）
	if highestIndex > start+2 && highestIndex < len(klines)-2 {
		supplyUpper := klines[highestIndex].High
		supplyLower := klines[highestIndex].Low
		
		// 扩展供给区边界（包含邻近K线）
		for i := highestIndex-1; i <= highestIndex+1 && i < len(klines); i++ {
			if i >= 0 {
				if klines[i].High > supplyUpper {
					supplyUpper = klines[i].High
				}
				if klines[i].Low < supplyLower {
					supplyLower = klines[i].Low
				}
			}
		}
		
		zone := &SupplyDemandZone{
			ID:           fmt.Sprintf("basic_supply_%d", highestIndex),
			Type:         SupplyZone,
			UpperBound:   supplyUpper,
			LowerBound:   supplyLower,
			CenterPrice:  (supplyUpper + supplyLower) / 2,
			Width:        supplyUpper - supplyLower,
			WidthPercent: (supplyUpper - supplyLower) / supplyLower * 100,
			Origin: &ZoneOrigin{
				KlineIndex:    highestIndex,
				PatternType:   FreshSupply,
				ImpulseMove:   0.015, // 1.5%默认冲击
				ImpulseVolume: klines[highestIndex].Volume,
				TimeFrame:     "basic",
				Confirmation:  false,
			},
			Status:       StatusFresh,
			CreationTime: klines[highestIndex].OpenTime,
			IsActive:     true,
			IsBroken:     false,
			Strength:     60.0, // 中等强度
			Quality:      QualityModerate,
		}
		
		zones = append(zones, zone)
	}
	
	// 创建需求区（基于最低点）
	if lowestIndex > start+2 && lowestIndex < len(klines)-2 {
		demandUpper := klines[lowestIndex].High
		demandLower := klines[lowestIndex].Low
		
		// 扩展需求区边界（包含邻近K线）
		for i := lowestIndex-1; i <= lowestIndex+1 && i < len(klines); i++ {
			if i >= 0 {
				if klines[i].High > demandUpper {
					demandUpper = klines[i].High
				}
				if klines[i].Low < demandLower {
					demandLower = klines[i].Low
				}
			}
		}
		
		zone := &SupplyDemandZone{
			ID:           fmt.Sprintf("basic_demand_%d", lowestIndex),
			Type:         DemandZone,
			UpperBound:   demandUpper,
			LowerBound:   demandLower,
			CenterPrice:  (demandUpper + demandLower) / 2,
			Width:        demandUpper - demandLower,
			WidthPercent: (demandUpper - demandLower) / demandLower * 100,
			Origin: &ZoneOrigin{
				KlineIndex:    lowestIndex,
				PatternType:   FreshDemand,
				ImpulseMove:   0.015, // 1.5%默认冲击
				ImpulseVolume: klines[lowestIndex].Volume,
				TimeFrame:     "basic",
				Confirmation:  false,
			},
			Status:       StatusFresh,
			CreationTime: klines[lowestIndex].OpenTime,
			IsActive:     true,
			IsBroken:     false,
			Strength:     60.0, // 中等强度
			Quality:      QualityModerate,
		}
		
		zones = append(zones, zone)
	}
	
	return zones
}

// isZoneOverlapping 检查新区域是否与现有区域重叠
func (sda *SupplyDemandAnalyzer) isZoneOverlapping(newZone *SupplyDemandZone, existingZones []*SupplyDemandZone) bool {
	for _, existing := range existingZones {
		if sda.zonesOverlap(newZone, existing) {
			return true
		}
	}
	return false
}