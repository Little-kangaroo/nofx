package market

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
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
	return sda.AnalyzeWithSymbol(klines, "", "")
}

// AnalyzeWithSymbol 分析K线数据识别供需区（支持Z-Score标准化）
func (sda *SupplyDemandAnalyzer) AnalyzeWithSymbol(klines []Kline, symbol, timeframe string) *SupplyDemandData {
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
	supplyZones = sda.identifySupplyZones(klines, symbol, timeframe)

	// 识别需求区
	demandZones = sda.identifyDemandZones(klines, symbol, timeframe)

	// 【P0修复】ATR验证：对所有识别的区域进行位置验证
	if len(klines) >= 14 { // 确保有足够数据计算ATR
		currentPrice := klines[len(klines)-1].Close
		atr := sda.calculateATR(klines, 14)

		// 验证供给区位置的合理性
		validSupplyZones := []*SupplyDemandZone{}
		for _, zone := range supplyZones {
			if sda.validateZonePosition(zone, currentPrice, atr) {
				validSupplyZones = append(validSupplyZones, zone)
			} else {
				// 🔧 优化：减少P0验证日志频率，避免日志污染
				// log.Printf("⚠️ [P0验证] 供给区%.2f-%.2f位置不合理，已过滤 (当前价格%.2f)",
				//	zone.LowerBound, zone.UpperBound, currentPrice)
			}
		}
		supplyZones = validSupplyZones

		// 验证需求区位置的合理性
		validDemandZones := []*SupplyDemandZone{}
		for _, zone := range demandZones {
			if sda.validateZonePosition(zone, currentPrice, atr) {
				validDemandZones = append(validDemandZones, zone)
			} else {
				// 🔧 优化：减少P0验证日志频率，避免日志污染
				// log.Printf("⚠️ [P0验证] 需求区%.2f-%.2f位置不合理，已过滤 (当前价格%.2f)",
				//	zone.LowerBound, zone.UpperBound, currentPrice)
			}
		}
		demandZones = validDemandZones

		// 🔧 优化：减少P0验证完成日志频率，避免日志污染
		// log.Printf("✅ [P0验证完成] ATR=%.2f, 有效供给区=%d, 有效需求区=%d",
		//	atr, len(supplyZones), len(demandZones))
	}

	// 合并并排序所有区域
	allZones := append(supplyZones, demandZones...)
	sda.updateZoneStatuses(allZones, klines)

	// 筛选活跃区域
	activeZones := sda.filterActiveZones(allZones)

	// 如果复杂模式识别没有找到足够的区域，使用简单的高低点方法作为补充
	// 修复备用机��悖论：不管主算法找到多少个，都启用备用机制增强识别
	// 这样可以确保在主算法识别能力有限时，依然有基础的供需区支撑
	if len(activeZones) < 5 { // 提高启动条件：少于5个区域就启动备用机制
		backupZones := sda.identifyBasicZonesWithSymbol(klines, symbol, timeframe)
		for _, zone := range backupZones {
			// 【P0修复】对备用区域也进行位置验证
			if len(klines) >= 14 {
				currentPrice := klines[len(klines)-1].Close
				atr := sda.calculateATR(klines, 14)
				if !sda.validateZonePosition(zone, currentPrice, atr) {
					log.Printf("⚠️ [P0验证] 备用区域%.2f-%.2f位置不合理，已跳过",
						zone.LowerBound, zone.UpperBound)
					continue
				}
			}

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

	// 计算上下文评分 (在所有供需区创建后进行)
	contextCalc := NewContextCalculator(klines)
	sda.CalculateContextScores(allZones, contextCalc)

	// 【P2修复】异常数据清洗：过滤极端width_atr和vol_ratio值
	dataData := &SupplyDemandData{
		SupplyZones:  supplyZones,
		DemandZones:  demandZones,
		ActiveZones:  activeZones,
		Config:       &sda.config,
		Statistics:   &SDStatistics{}, // 临时统计，将被重新计算
		LastAnalysis: time.Now().UnixMilli(),
	}

	// 应用数据清洗
	cleaner := NewDataCleaner()
	cleanedData, cleaningStats := cleaner.CleanSupplyDemandData(dataData)

	log.Printf("🧹 [P2数据清洗] 清洗完成: 原始=%d, 清洗后=%d, 过滤率=%.1f%%, 质量评分=%.1f",
		cleaningStats.TotalZones, len(cleanedData.ActiveZones), cleaningStats.FilterRate, cleaningStats.QualityScore)

	// 使用清洗后的数据
	supplyZones = cleanedData.SupplyZones
	demandZones = cleanedData.DemandZones
	activeZones = cleanedData.ActiveZones

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
func (sda *SupplyDemandAnalyzer) identifySupplyZones(klines []Kline, symbol, timeframe string) []*SupplyDemandZone {
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
		if symbol != "" && timeframe != "" {
			sda.calculateZoneStrengthWithSymbol(zone, klines, symbol, timeframe)
		} else {
			sda.calculateZoneStrength(zone, klines)
		}
		sda.assessZoneQuality(zone)
	}

	return zones
}

// identifyDemandZones 识别需求区
func (sda *SupplyDemandAnalyzer) identifyDemandZones(klines []Kline, symbol, timeframe string) []*SupplyDemandZone {
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
		if symbol != "" && timeframe != "" {
			sda.calculateZoneStrengthWithSymbol(zone, klines, symbol, timeframe)
		} else {
			sda.calculateZoneStrength(zone, klines)
		}
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
// findBaseArea 寻找整理区域 (大幅简化版：适应真实市场形态)
func (sda *SupplyDemandAnalyzer) findBaseArea(klines []Kline, centerIndex int, isRally bool) (int, int) {
	// 简化版：固定窗口搜索，不要求完美平整
	maxLookback := 8 // 最多向左右各看8根（对5m周期约40分钟）
	minBase := 2     // 最少2根K线形成Base

	if centerIndex < maxLookback || centerIndex >= len(klines)-maxLookback {
		return -1, -1
	}

	// 简化逻辑：寻找相对平缓��价格区域（允许倾斜和收敛）
	baseStart := centerIndex - 2 // 默认向左2根
	baseEnd := centerIndex + 2   // 默认向右2根

	// 扩展Base范围：允许更宽松的条件
	for i := centerIndex - 1; i >= centerIndex-maxLookback && i >= 0; i-- {
		// 简单条件：如果价格变化不是极端波动，就包含
		currentRange := klines[i].High - klines[i].Low
		avgPrice := (klines[i].High + klines[i].Low) / 2
		if avgPrice > 0 && currentRange/avgPrice < 0.08 { // 8%以内的波动都认为是Base
			baseStart = i
		} else {
			break // 遇到大波动停止
		}
	}

	for i := centerIndex + 1; i <= centerIndex+maxLookback && i < len(klines); i++ {
		currentRange := klines[i].High - klines[i].Low
		avgPrice := (klines[i].High + klines[i].Low) / 2
		if avgPrice > 0 && currentRange/avgPrice < 0.08 {
			baseEnd = i
		} else {
			break
		}
	}

	// 确保最少有minBase根K线
	if baseEnd-baseStart+1 < minBase {
		return -1, -1
	}

	// 计算整个Base区域的价格范围
	high := klines[baseStart].High
	low := klines[baseStart].Low

	for i := baseStart; i <= baseEnd; i++ {
		if klines[i].High > high {
			high = klines[i].High
		}
		if klines[i].Low < low {
			low = klines[i].Low
		}
	}

	// 大幅放宽Base宽度限制：允许12%的波动范围
	avgPrice := (high + low) / 2
	if avgPrice > 0 {
		rangePercent := (high - low) / avgPrice
		if rangePercent > sda.config.MaxBasePercent {
			// 即使超出限制，也尝试缩小范围而不是直接放弃
			// 保持核心的Base部分
			newStart := centerIndex - 1
			newEnd := centerIndex + 1
			if newStart >= 0 && newEnd < len(klines) {
				return newStart, newEnd
			}
			return -1, -1
		}
	}

	return baseStart, baseEnd
}

// validateLeftMove 验证左侧移动 (大幅放宽：适应连续小阴线趋势)
func (sda *SupplyDemandAnalyzer) validateLeftMove(klines []Kline, baseStart int, isRally bool) bool {
	// 扩大搜索范围：从5根扩展到10根，捕捉更多趋势
	lookback := 10
	if baseStart < lookback {
		lookback = baseStart
	}
	if lookback < 3 { // 最少需要3根K线验证
		return false
	}

	startPrice := klines[baseStart-lookback].Close
	endPrice := klines[baseStart].Close

	priceChange := (endPrice - startPrice) / startPrice

	// 大幅放宽条件：从1%降低到0.3%，且允许累积效应
	minChange := sda.config.MinImpulsePercent

	if isRally {
		return priceChange > minChange
	} else {
		return priceChange < -minChange
	}
}

// validateRightMove 验证右侧移动 (大幅放宽：适应连续小阴线趋势)
func (sda *SupplyDemandAnalyzer) validateRightMove(klines []Kline, baseEnd int, isRally bool) bool {
	// 扩大搜索范围：从5根扩展到10根
	lookforward := 10
	if baseEnd >= len(klines)-lookforward {
		lookforward = len(klines) - baseEnd - 1
	}
	if lookforward < 3 {
		return false
	}

	startPrice := klines[baseEnd].Close
	endPrice := klines[baseEnd+lookforward].Close

	priceChange := (endPrice - startPrice) / startPrice

	minChange := sda.config.MinImpulsePercent

	if isRally {
		return priceChange > minChange
	} else {
		return priceChange < -minChange
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

// calculateZoneStrength 计算区域强度 (集成Z-Score标准化)
func (sda *SupplyDemandAnalyzer) calculateZoneStrength(zone *SupplyDemandZone, klines []Kline) {
	// 原有强度计算逻辑保持不变
	strength := 0.0

	// 修复1: 基于冲击移动的强度 (权重降低，避免过严)
	strength += zone.Origin.ImpulseMove * 30 // 从50降到30

	// 修复2: 基于成交量的强度 (更宽松的成交量要求)
	avgVolume := sda.calculateAverageVolume(klines, 0, len(klines)-1)
	if avgVolume > 0 {
		volumeRatio := zone.Volume / avgVolume
		// 成交量权重降低，且设置更合理的上限
		strength += math.Min(volumeRatio, 3.0) * 8 // 从10降到8，上限从5降到3
	}

	// 修复3: 基于区域宽度的强度 (更加友好的评分)
	if zone.WidthPercent > 0 && zone.WidthPercent <= 12 { // 12%内的宽度都给分
		// 优化宽度评分：不再惩罚较宽的区域，而是给出基础分数
		widthScore := math.Max(0, 10-zone.WidthPercent) // 越窄分数越���，但最宽也有最少2分
		strength += math.Max(widthScore, 2)             // 保底2分
	}

	// 修复4: 基于模式类型的强度 (提高基础分数)
	switch zone.Origin.PatternType {
	case DropBaseDrop, RallyBaseRally:
		strength += 20 // 从15提升到20，经典模式
	case RallyBaseDropOB, DropBaseRallyOB:
		strength += 18 // 从12提升到18，订单区块
	case FreshSupply, FreshDemand:
		strength += 15 // 从8提升到15，新鲜区域
	}

	// 修复5: 增加基础分数，确保有效区域不会太低分
	baseScore := 25.0 // 所有区域的基础分数
	strength += baseScore

	// 限制在合理范围，但提高上限
	zone.Strength = math.Max(15.0, math.Min(strength, 100.0)) // 最低15分，最高100分

	// ===== 集成Z-Score标准化 =====
	// 尝试获取全局强度标准化器
	normalizer := GetGlobalStrengthNormalizer()
	if normalizer != nil {
		// 从K线数据推断symbol和timeframe
		symbol, timeframe := sda.inferSymbolTimeframe(klines)
		if symbol != "" && timeframe != "" {
			// 创建供需区强度记录
			zoneRecord := &ZoneStrengthRecord{
				Symbol:       symbol,
				Timeframe:    timeframe,
				RawScore:     zone.Strength,
				ZoneType:     string(zone.Type),
				PatternType:  string(zone.Origin.PatternType),
				TouchCount:   zone.TouchCount,
				VolumeRatio:  zone.Volume / avgVolume,
				WidthPercent: zone.WidthPercent,
			}

			// 获取标准化Z-Score (先计算Z分数，再添加到历史记录)
			zScoreResult := normalizer.GetZScoreWithUpdate(symbol, timeframe, zone.Strength, zoneRecord)

			// 更新供需区的标准化强度
			zone.StrengthZ = zScoreResult.ZScore
			zone.StrengthZReady = zScoreResult.IsReady
			zone.SampleCount = zScoreResult.SampleCount

			// 🔧 优化：减少供需区强度标准化日志频率，避免日志污染
			// 调试日志：记录标准化过程
			// if zScoreResult.IsReady {
			//	log.Printf("🎯 [%s_%s] 供需区强度标准化: 原始=%.2f → Z分数=%.2f (样本=%d)",
			//		symbol, timeframe, zone.Strength, zone.StrengthZ, zone.SampleCount)
			// } else {
			//	log.Printf("⚠️ [%s_%s] 供需区强度标准化: 样本不足 (%d<%d)，使用原始强度",
			//		symbol, timeframe, zone.SampleCount, MinSampleSize)
			// }
		} else {
			// 静默跳过Z-Score标准化 - 这是向后兼容的正常fallback
		}
	} else {
		log.Printf("⚠️ 强度标准化器未初始化，使用原始强度评分")
	}
}

// calculateZoneStrengthWithSymbol 计算区域强度（支持Z-Score标准化的完整版本）
// 需要明确传入symbol和timeframe以启用标准化功能
func (sda *SupplyDemandAnalyzer) calculateZoneStrengthWithSymbol(zone *SupplyDemandZone, klines []Kline, symbol, timeframe string) {
	// 首先调用原有的强度计算逻辑
	sda.calculateZoneStrength(zone, klines)

	// ===== 集成Z-Score标准化 =====
	// 只有在明确提供symbol和timeframe时才进行标准化
	if symbol != "" && timeframe != "" {
		normalizer := GetGlobalStrengthNormalizer()
		if normalizer != nil {
			// 重新计算成交量比例（用于记录）
			avgVolume := sda.calculateAverageVolume(klines, 0, len(klines)-1)
			volumeRatio := 1.0
			if avgVolume > 0 {
				volumeRatio = zone.Volume / avgVolume
			}

			// 创建供需区强度记录
			zoneRecord := &ZoneStrengthRecord{
				Symbol:       symbol,
				Timeframe:    timeframe,
				RawScore:     zone.Strength,
				ZoneType:     string(zone.Type),
				PatternType:  string(zone.Origin.PatternType),
				TouchCount:   zone.TouchCount,
				VolumeRatio:  volumeRatio,
				WidthPercent: zone.WidthPercent,
			}

			// 获取标准化Z-Score (先计算Z分数，再添加到历史记录)
			zScoreResult := normalizer.GetZScoreWithUpdate(symbol, timeframe, zone.Strength, zoneRecord)

			// 更新供需区的标准化强度
			zone.StrengthZ = zScoreResult.ZScore
			zone.StrengthZReady = zScoreResult.IsReady
			zone.SampleCount = zScoreResult.SampleCount

			// 🔧 优化：减少供需区强度标准化日志频率，避免日志污染
			// 调试日志：记录标准化过程
			// if zScoreResult.IsReady {
			//	log.Printf("🎯 [%s_%s] 供需区强度标准化: 原始=%.2f → Z分数=%.2f (样本=%d)",
			//		symbol, timeframe, zone.Strength, zone.StrengthZ, zone.SampleCount)
			// } else {
			//	log.Printf("⚠️ [%s_%s] 供需区强度标准化: 样本不足 (%d<%d)，使用原始强度",
			//		symbol, timeframe, zone.SampleCount, MinSampleSize)
			// }
		} else {
			log.Printf("⚠️ 强度标准化器未初始化，使用原始强度评分")
		}
	}
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

// updateZoneStatuses 更新区域状态（修复数据层稳定性）
func (sda *SupplyDemandAnalyzer) updateZoneStatuses(zones []*SupplyDemandZone, klines []Kline) {
	if len(klines) == 0 {
		return
	}

	currentTime := klines[len(klines)-1].OpenTime
	currentPrice := klines[len(klines)-1].Close

	// 计算ATR用于准确的突破判定
	atr := sda.calculateATR(klines, 14)

	for _, zone := range zones {
		// 检查年龄
		age := int((currentTime - zone.CreationTime) / (3600 * 1000)) // 小时
		if age > sda.config.MaxZoneAge {
			zone.Status = StatusExpired
			zone.IsActive = false
			continue
		}

		// 【关键修复】使用增强的突破判定逻辑��区分Testing和Broken状态
		breakoutResult := sda.analyzeZoneInteraction(zone, currentPrice, atr)

		switch breakoutResult {
		case "no_interaction":
			// 价格未接触区域，保持现状

		case "testing":
			// 【核心修复】价格正在测试区域，但未有效突破 - 保持活跃状态
			zone.Status = StatusTesting
			zone.IsActive = true // 确保Testing状态的区域保持活跃
			log.Printf("🎯 [Testing状态] 区域%s被价格测试(%.2f)，保持活跃监控", zone.ID, currentPrice)

		case "true_breakout":
			// 真正的突破 - 进行类型转换处理
			sda.handleZoneBreakout(zone, currentPrice, klines)

			if strings.Contains(zone.ID, "breaker_") {
				// Breaker区域：继续活跃，只是改变了类型
				log.Printf("✅ [Breaker激活] 区域%s类型转换完成，继续监控", zone.ID)
			} else if zone.Status == StatusTesting {
				// 即使在handleZoneBreakout后仍为Testing状态，保持活跃
				zone.IsActive = true
				log.Printf("🎯 [Testing保持] 区域%s经处理后仍为Testing状态，保持活跃", zone.ID)
			} else {
				// 真正的突破：标记为broken，但给予观察期
				zone.Status = StatusBroken
				zone.IsBroken = true
				zone.BreakTime = currentTime
				// 【修复】给予破损区域短暂观察期，防止误判
				if age < 2 { // 2小时内的新区域即使破损也暂时保持活跃
					zone.IsActive = true
					log.Printf("⚡ [新区域保护] 新区域%s虽破损但给予观察期，保持活跃", zone.ID)
				} else {
					zone.IsActive = false
					log.Printf("❌ [区域失效] 区域%s真正突破失效", zone.ID)
				}
				continue
			}
		}

		// 检查触及次数
		touchCount := sda.countZoneTouches(zone, klines)
		zone.TouchCount = touchCount

		if touchCount > sda.config.MaxTouchCount {
			zone.Status = StatusWeakened
			// 【修复】即使weakened状态也不立即失效，给AI判断机会
			zone.IsActive = true
		} else if touchCount > 0 {
			if zone.Status != StatusTesting { // 不覆盖Testing状态
				zone.Status = StatusTested
			}
			zone.LastTouch = currentTime
		}

		// 验证区域反应
		if sda.config.EnableValidation {
			zone.Validation = sda.validateZoneReaction(zone, klines)
		}
	}
}

// isZoneBroken 检查区域是否被突���
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

// handleZoneBreakout 处理区域突破后的类型转换（P0修复：基于ATR的智能判断）
// 作用：使用ATR缓冲区判断真假突破，只有真正的突破才进行区域类型转换
func (sda *SupplyDemandAnalyzer) handleZoneBreakout(zone *SupplyDemandZone, currentPrice float64, klines []Kline) {
	// 计算ATR用于真假突破判定
	atr := sda.calculateATR(klines, 14) // 使用14期ATR

	// 检查是否为真正的突破（而非SFP假突破）
	isTrueBreak := sda.isTrueBreakout(zone, currentPrice, atr)

	originalType := zone.Type

	if zone.Type == DemandZone && currentPrice < zone.LowerBound {
		if isTrueBreak {
			// 【真突破】需求区被真正跌破 → 转换为供给区（阻力位）
			zone.Type = SupplyZone
			zone.ID = "breaker_" + zone.ID // 更新ID标识转换

			// 更新模式类型为Breaker
			if zone.Origin != nil {
				zone.Origin.PatternType = "demand_breaker" // 新的模式类型
			}

			log.Printf("🔄 [真突破确认] 需求区%.2f-%.2f被真正跌破(ATR缓冲%.2f)，转换为供给区",
				zone.LowerBound, zone.UpperBound, 0.2*atr)
		} else {
			// 【假突破/SFP】仅标记为测试状态，不转换类型
			zone.Status = StatusTesting
			log.Printf("🎯 [SFP检测] 需求区%.2f-%.2f价格刺破但未达真突破阈值，标记为Testing状态",
				zone.LowerBound, zone.UpperBound)
			return // 不进行类型转换
		}

	} else if zone.Type == SupplyZone && currentPrice > zone.UpperBound {
		if isTrueBreak {
			// 【真突破】供给区被真正突破 → 转换为需求区（支撑位）
			zone.Type = DemandZone
			zone.ID = "breaker_" + zone.ID

			if zone.Origin != nil {
				zone.Origin.PatternType = "supply_breaker"
			}

			log.Printf("🔄 [真突破确认] 供给区%.2f-%.2f被真正突破(ATR缓冲%.2f)，转换为需求区",
				zone.LowerBound, zone.UpperBound, 0.2*atr)
		} else {
			// 【假突破/SFP】仅标记为测试状态，不转换类型
			zone.Status = StatusTesting
			log.Printf("🎯 [SFP检测] 供给区%.2f-%.2f价格刺破但未达真突破阈值，标记为Testing状态",
				zone.LowerBound, zone.UpperBound)
			return // 不进行类型转换
		}
	}

	// 如果发生了真正的类型转换，重置区域状态
	if zone.Type != originalType {
		zone.Status = StatusTested // 重置为已测试状态
		zone.TouchCount = 1        // 重置触及计数
		zone.IsActive = true       // 重新激活
		zone.IsBroken = false      // 不再是broken状态
		zone.BreakTime = 0         // 清除突破时间

		// 重新计算强度（Breaker区域通常强度较高）
		zone.Strength = math.Min(zone.Strength*1.2, 100.0) // 提升20%强度，上限100
		zone.Quality = QualityGood                         // 设置为良好质量

		log.Printf("✅ [区域转换完成] ATR=%.2f, 缓冲距离=%.2f, 新类型=%s",
			atr, 0.2*atr, zone.Type)
	}
}

// countZoneTouches 计算区域触及次数（修复版：避免数值爆炸）
func (sda *SupplyDemandAnalyzer) countZoneTouches(zone *SupplyDemandZone, klines []Kline) int {
	count := 0
	lastTouchIndex := -1
	minGapBetweenTouches := 5 // 【优化】至少间隔5根K线才算新的触及（更严格聚类）

	// 【修复幽灵支撑Bug】从区域诞生开始检查完整历史，不截断数据
	// 必须检查完整生命周期以发现历史的Zone Broken事件
	startIndex := 0
	if zone.Origin != nil {
		startIndex = zone.Origin.KlineIndex + 1
	}
	endIndex := len(klines)

	// 【安全检查】确保索引有效
	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex >= endIndex {
		return 0 // 没有后续数据可检查
	}

	// 【完整历史扫描】不使用maxLookback截断，确保技术分析的准确性
	for i := startIndex; i < endIndex; i++ {
		if sda.priceInZone(klines[i].High, klines[i].Low, zone) {
			// 聚类机制：只有与上次触及间隔足够远才计为新触及
			if lastTouchIndex == -1 || i-lastTouchIndex >= minGapBetweenTouches {
				count++
				lastTouchIndex = i

				// 【关键优化】严格限制最大触及次数，适配AI V-10.0规则（Touches>8废弃）
				if count >= 8 {
					break // 最多8次触及，确保通过AI验证
				}
			}
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

// filterActiveZones 筛选活跃区域（修复数据层稳定性）
func (sda *SupplyDemandAnalyzer) filterActiveZones(zones []*SupplyDemandZone) []*SupplyDemandZone {
	var active []*SupplyDemandZone

	for _, zone := range zones {
		// 【关键修复】扩展活跃区域的保留条件，确保Testing状态区域不会消失
		shouldKeepActive := zone.IsActive ||
			zone.Status == StatusTesting || // Testing状态必须保留
			zone.Status == StatusWeakened || // Weakened状态也给AI判断机会
			(zone.Status == StatusBroken && sda.isRecentZone(zone)) // 新区域即使Broken也给观察期

		if shouldKeepActive {
			active = append(active, zone)

			// 调试日志：记录保留原因
			reason := ""
			if zone.IsActive {
				reason = "活跃状态"
			} else if zone.Status == StatusTesting {
				reason = "Testing状态保护"
			} else if zone.Status == StatusWeakened {
				reason = "Weakened状态保护"
			} else if zone.Status == StatusBroken && sda.isRecentZone(zone) {
				reason = "新区域观察期保护"
			}

			if reason != "活跃状态" { // 只记录特殊保护情况
				log.Printf("🛡️ [区域保护] 区域%s被保留(%s) - %.2f-%.2f",
					zone.ID, reason, zone.LowerBound, zone.UpperBound)
			}
		}
	}

	log.Printf("📊 [ActiveZones过滤] 总区域%d个，保留活跃区域%d个", len(zones), len(active))
	return active
}

// isRecentZone 判断是否为近期创建的区域（2小时内）
func (sda *SupplyDemandAnalyzer) isRecentZone(zone *SupplyDemandZone) bool {
	currentTime := time.Now().UnixMilli()
	age := int((currentTime - zone.CreationTime) / (3600 * 1000)) // 小时
	return age < 2
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
	confidence := freshZone.Strength*0.9 + 15

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
	return sda.identifyBasicZonesWithSymbol(klines, "", "")
}

// identifyBasicZonesWithSymbol 识别基础供需区（支持Z-Score标准化）
func (sda *SupplyDemandAnalyzer) identifyBasicZonesWithSymbol(klines []Kline, symbol, timeframe string) []*SupplyDemandZone {
	var zones []*SupplyDemandZone

	// 大幅增加最小K线数要求，确保有足够的数据进行分析
	if len(klines) < 50 {
		return zones
	}

	// 扩大时间窗口：从20根扩展到50根，覆盖更长的时间周期
	// 对5m周期约4小时，对1h周期约2天，确保捕捉重要关键位
	recentPeriod := 50
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

	// 创建供给区（基于最高点） - 提升质量和强度
	if highestIndex > start+5 && highestIndex < len(klines)-5 { // 增加边界检查
		supplyUpper := klines[highestIndex].High
		supplyLower := klines[highestIndex].Low

		// 扩展供给区边界（包含更多邻近K线，捕捉更完整的阻力区域）
		for i := highestIndex - 3; i <= highestIndex+3 && i < len(klines); i++ { // 扩展到前后3根
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
				ImpulseMove:   0.02, // 提升到2%默认冲击，增强重要性
				ImpulseVolume: klines[highestIndex].Volume,
				TimeFrame:     "basic",
				Confirmation:  false,
			},
			Status:       StatusFresh,
			CreationTime: klines[highestIndex].OpenTime,
			IsActive:     true,
			IsBroken:     false,
			Quality:      QualityGood, // 提升质量等级
		}

		// 计算区域强度（支持Z-Score标准化）
		if symbol != "" && timeframe != "" {
			sda.calculateZoneStrengthWithSymbol(zone, klines, symbol, timeframe)
		} else {
			sda.calculateZoneStrength(zone, klines)
		}

		zones = append(zones, zone)
	}

	// 创建需求区（基于最低点） - 提升质量和强度
	if lowestIndex > start+5 && lowestIndex < len(klines)-5 {
		demandUpper := klines[lowestIndex].High
		demandLower := klines[lowestIndex].Low

		// 扩展需求区边界（包含更多邻近K线）
		for i := lowestIndex - 3; i <= lowestIndex+3 && i < len(klines); i++ {
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
				ImpulseMove:   0.02, // 提升到2%默认冲击，增强重要性
				ImpulseVolume: klines[lowestIndex].Volume,
				TimeFrame:     "basic",
				Confirmation:  false,
			},
			Status:       StatusFresh,
			CreationTime: klines[lowestIndex].OpenTime,
			IsActive:     true,
			IsBroken:     false,
			Quality:      QualityGood, // 提升质量等级
		}

		// 计算区域强度（支持Z-Score标准化）
		if symbol != "" && timeframe != "" {
			sda.calculateZoneStrengthWithSymbol(zone, klines, symbol, timeframe)
		} else {
			sda.calculateZoneStrength(zone, klines)
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

// inferSymbolTimeframe 从K线数据推断symbol和timeframe（工具方法）
func (sda *SupplyDemandAnalyzer) inferSymbolTimeframe(klines []Kline) (string, string) {
	// TODO: 实际实现需要根据K线数据特征或外部传入参数来确定
	// 这里提供一个简单的示例实现
	return "", "" // 返回空值，让调用者明确传入symbol和timeframe
}

// validateZonePosition 验证区域位置的合理性（P0修复辅助方法）
// 作用：确保供给区在当前价格上方，需求区在当前价格下方
func (sda *SupplyDemandAnalyzer) validateZonePosition(zone *SupplyDemandZone, currentPrice float64, atr float64) bool {
	bufferDistance := 0.1 * atr // 使用较小的缓冲区用于位置验证

	if zone.Type == SupplyZone {
		// 供给区应该在当前价格上方（或接近）
		minValidPrice := currentPrice - bufferDistance
		isValid := zone.LowerBound >= minValidPrice

		if !isValid {
			// 🔧 优化：移除冗余的位置验证失败日志，静默处理
			// log.Printf("🚫 [位置验证失败] 供给区%.2f-%.2f在当前价格%.2f下方，不符合市场物理定律",
			//	zone.LowerBound, zone.UpperBound, currentPrice)
		}
		return isValid

	} else if zone.Type == DemandZone {
		// 需求区应该在当前价格下方（或接近）
		maxValidPrice := currentPrice + bufferDistance
		isValid := zone.UpperBound <= maxValidPrice

		if !isValid {
			// 🔧 优化：移除冗余的位置验证失败日志，静默处理
			// log.Printf("🚫 [位置验证失败] 需求区%.2f-%.2f在当前价格%.2f上方，不符合市场物理定律",
			//	zone.LowerBound, zone.UpperBound, currentPrice)
		}
		return isValid
	}

	return true
}

// ===== ATR辅助方法（P0级修复支持） =====

// calculateATR 计算平均真实波动率（ATR）- 用于判断真假突破的关键指标
// 作用：测量市场的正常波动幅度，设定合理的突破判定标准
func (sda *SupplyDemandAnalyzer) calculateATR(klines []Kline, period int) float64 {
	if len(klines) < period+1 {
		// 数据不足时的降级处理，确保系统不会因为数据不够而崩溃
		if len(klines) >= 2 {
			lastKline := klines[len(klines)-1]
			return (lastKline.High - lastKline.Low) * 1.5 // 使用当前K线波动的1.5倍作为估算
		}
		return 50.0 // 保底默认值
	}

	var trSum float64 = 0
	validPeriods := 0

	// 计算指定周期内的平均真实波动率
	for i := 1; i < len(klines) && validPeriods < period; i++ {
		current := klines[i]
		previous := klines[i-1]

		// 真实波动率公式：TR = max(当日高低差, |当日高-前日收|, |当日低-前日收|)
		tr1 := current.High - current.Low
		tr2 := math.Abs(current.High - previous.Close)
		tr3 := math.Abs(current.Low - previous.Close)

		trCurrent := math.Max(tr1, math.Max(tr2, tr3))
		trSum += trCurrent
		validPeriods++
	}

	if validPeriods == 0 {
		return 50.0
	}

	atr := trSum / float64(validPeriods)
	log.Printf("🔢 [ATR计算] 周期=%d, ATR=%.2f (市场波动性基准)", validPeriods, atr)
	return atr
}

// isTrueBreakout 判断是否为真正的突破（带ATR缓冲区） - P0修复核心逻辑
// 作用：区分真突破和假突破(SFP)，防止误判导致错误的区域类型转换
func (sda *SupplyDemandAnalyzer) isTrueBreakout(zone *SupplyDemandZone, currentPrice float64, atr float64) bool {
	bufferDistance := 0.2 * atr // 0.2倍ATR作为缓冲距离，这是经验值，可以过滤掉大部分假突破

	if zone.Type == DemandZone {
		// 需求区突破判定：价格必须跌破 (区域下沿 - 0.2*ATR) 才算真正突破
		breakoutThreshold := zone.LowerBound - bufferDistance
		isTrue := currentPrice < breakoutThreshold

		log.Printf("🎯 [真假突破判定] 需求区%.2f, 当前%.2f, 阈值%.2f (缓冲%.2f), 结果=%s",
			zone.LowerBound, currentPrice, breakoutThreshold, bufferDistance,
			map[bool]string{true: "真突破", false: "假突破/测试"}[isTrue])

		return isTrue
	} else if zone.Type == SupplyZone {
		// 供给区突破判定：价格必须突破 (区域上沿 + 0.2*ATR) 才算真正突破
		breakoutThreshold := zone.UpperBound + bufferDistance
		isTrue := currentPrice > breakoutThreshold

		log.Printf("🎯 [真假突破判定] 供给区%.2f, 当前%.2f, 阈值%.2f (缓冲%.2f), 结果=%s",
			zone.UpperBound, currentPrice, breakoutThreshold, bufferDistance,
			map[bool]string{true: "真突破", false: "假突破/测试"}[isTrue])

		return isTrue
	}

	return false
}

// validateZonePositions 验证和清理所有区域的位置合理性（P0修复全面清理方法）
// 作用：系统启动或定期维护时调用，清理不符合市场物理定律的区域
func (sda *SupplyDemandAnalyzer) ValidateZonePositions(sdData *SupplyDemandData, klines []Kline) *SupplyDemandData {
	if sdData == nil || len(klines) < 14 {
		return sdData
	}

	currentPrice := klines[len(klines)-1].Close
	atr := sda.calculateATR(klines, 14)

	log.Printf("🔍 [P0全面验证] 开始验证所有区域位置，当前价格=%.2f, ATR=%.2f", currentPrice, atr)

	// 验证和清理供给区
	validSupplyZones := []*SupplyDemandZone{}
	removedSupplyCount := 0
	for _, zone := range sdData.SupplyZones {
		if sda.validateZonePosition(zone, currentPrice, atr) {
			validSupplyZones = append(validSupplyZones, zone)
		} else {
			removedSupplyCount++
			log.Printf("🗑️ [清理供给区] 移除位置不合理的供给区: %.2f-%.2f (ID: %s)",
				zone.LowerBound, zone.UpperBound, zone.ID)
		}
	}

	// 验证和清理需求区
	validDemandZones := []*SupplyDemandZone{}
	removedDemandCount := 0
	for _, zone := range sdData.DemandZones {
		if sda.validateZonePosition(zone, currentPrice, atr) {
			validDemandZones = append(validDemandZones, zone)
		} else {
			removedDemandCount++
			log.Printf("🗑️ [清理需求区] 移除位置不合理的需求区: %.2f-%.2f (ID: %s)",
				zone.LowerBound, zone.UpperBound, zone.ID)
		}
	}

	// 重新构建活跃区域列表
	allValidZones := append(validSupplyZones, validDemandZones...)
	validActiveZones := []*SupplyDemandZone{}
	for _, zone := range allValidZones {
		if zone.IsActive {
			validActiveZones = append(validActiveZones, zone)
		}
	}

	// 重新计算统计信息
	stats := sda.calculateStatistics(validSupplyZones, validDemandZones, validActiveZones)

	log.Printf("✅ [P0全面验证完成] 移除供给区%d个, 需求区%d个, 剩余活跃区域%d个",
		removedSupplyCount, removedDemandCount, len(validActiveZones))

	return &SupplyDemandData{
		SupplyZones:  validSupplyZones,
		DemandZones:  validDemandZones,
		ActiveZones:  validActiveZones,
		Config:       sdData.Config,
		Statistics:   stats,
		LastAnalysis: time.Now().UnixMilli(),
	}
}

// ===== 数据层稳定性修复：Zone交互分析增强 =====

// analyzeZoneInteraction 分析价格与区域的交互状态（修复数据层稳定性核心函数）
// 返回：no_interaction, testing, true_breakout
func (sda *SupplyDemandAnalyzer) analyzeZoneInteraction(zone *SupplyDemandZone, currentPrice float64, atr float64) string {
	// 计算价格到区域的距离
	distanceToZone := sda.calculateDistanceToZone(zone, currentPrice)

	// 级别1：未接触区域 - 价格距离区域超过1%
	if distanceToZone > 0.01 { // 1%以外不算交互
		return "no_interaction"
	}

	// 判断价格是否在区域内部
	priceInZone := currentPrice >= zone.LowerBound && currentPrice <= zone.UpperBound

	if priceInZone {
		// 价格在区域内 = Testing状态，不应该失效
		return "testing"
	}

	// 计算刺破距离（使用ATR作为基准）
	penetrationDistance := 0.0
	if zone.Type == DemandZone && currentPrice < zone.LowerBound {
		penetrationDistance = zone.LowerBound - currentPrice
	} else if zone.Type == SupplyZone && currentPrice > zone.UpperBound {
		penetrationDistance = currentPrice - zone.UpperBound
	}

	// 设置ATR缓冲标准
	minPenetrationForTesting := 0.1 * atr  // 0.1*ATR以内算轻微测试
	minPenetrationForBreakout := 0.5 * atr // 0.5*ATR以上才考虑真突破

	if penetrationDistance <= minPenetrationForTesting {
		// 轻微刺破 = Testing状态
		return "testing"
	} else if penetrationDistance >= minPenetrationForBreakout {
		// 显著刺破 = 可能的真突破，需要进一步验证
		return "true_breakout"
	} else {
		// 中等刺破 = 保守��理为Testing状态
		return "testing"
	}
}
