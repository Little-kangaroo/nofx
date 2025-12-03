package market

import (
	"log"
	"time"
)

// FuzzyLogicProcessor 模糊逻辑处理器，负责供需区状态的智能更新
type FuzzyLogicProcessor struct {
	contextCalc *ContextCalculator
}

// NewFuzzyLogicProcessor 创建模糊逻辑处理器
func NewFuzzyLogicProcessor(contextCalc *ContextCalculator) *FuzzyLogicProcessor {
	return &FuzzyLogicProcessor{
		contextCalc: contextCalc,
	}
}

// UpdateZoneStatus 智能更新供需区状态 (核心模糊逻辑处理)
func (flp *FuzzyLogicProcessor) UpdateZoneStatus(zone *SupplyDemandZone, kline Kline) {
	// 计算并存储模糊逻辑缓冲区大小
	zone.FuzzyTolerance = flp.contextCalc.CalculateFuzzyTolerance(zone.Width)
	
	// 检查是否触及区域 (使用模糊逻辑)
	if flp.contextCalc.IsFuzzyTouch(kline, zone) {
		// 更新触及计数和时间
		zone.TouchCount++
		zone.LastTouch = time.Now().UnixMilli()
		
		// 计算穿透深度
		penetrationPct := flp.contextCalc.CalculatePenetrationDepth(kline, zone)
		zone.LastPenetrationPct = penetrationPct
		
		// 更新最大穿透深度
		if penetrationPct > zone.MaxPenetrationPct {
			zone.MaxPenetrationPct = penetrationPct
		}
		
		// 统计深度触及次数 (穿透>50%)
		if penetrationPct > 50 {
			zone.DeepTouchCount++
		}
		
		// 检查是否破坏区域 (使用严格的模糊逻辑标准)
		if flp.contextCalc.IsFuzzyBreak(kline, zone) {
			zone.Status = StatusBroken
			zone.IsBroken = true
			zone.BreakTime = time.Now().UnixMilli()
			log.Printf("🔴 [模糊逻辑] 供需区 %s 被破坏: 收盘突破%.2f, 幅度%.2f (>%.2f ATR)", 
				zone.ID, zone.getBreakPrice(kline), zone.getBreakDepth(kline), 0.2*flp.contextCalc.market.ATR14)
		} else {
			// 检查是否应该标记为弱化
			if flp.shouldWeakenZone(zone) {
				if zone.Status != StatusWeakened {
					zone.Status = StatusWeakened
					log.Printf("🟡 [模糊逻辑] 供需区 %s 被弱化: 触及%d次, 最大穿透%.1f%%, 深度触及%d次", 
						zone.ID, zone.TouchCount, zone.MaxPenetrationPct, zone.DeepTouchCount)
				}
			} else {
				// 正常测试状态
				if zone.Status == StatusFresh {
					zone.Status = StatusTested
					log.Printf("🟠 [模糊逻辑] 供需区 %s 首次测试: 穿透深度%.1f%%, 缓冲区±%.2f", 
						zone.ID, penetrationPct, zone.FuzzyTolerance)
				}
			}
		}
	}
}

// shouldWeakenZone 判断是否应该将区域标记为弱化
func (flp *FuzzyLogicProcessor) shouldWeakenZone(zone *SupplyDemandZone) bool {
	// 条件1: 触碰次数超过3次
	if zone.TouchCount > 3 {
		return true
	}
	
	// 条件2: 曾经被深度穿透 (>50% zone width)
	if zone.MaxPenetrationPct > 50 {
		return true
	}
	
	// 条件3: 多次深度触及 (穿透>50%的次数≥2)
	if zone.DeepTouchCount >= 2 {
		return true
	}
	
	return false
}

// ProcessKlineUpdate 处理单根K线对所有供需区的影响
func (flp *FuzzyLogicProcessor) ProcessKlineUpdate(zones []*SupplyDemandZone, kline Kline) {
	for _, zone := range zones {
		if zone == nil || zone.IsBroken {
			continue // 跳过空区域和已破坏区域
		}
		
		// 只处理活跃区域
		if zone.IsActive {
			flp.UpdateZoneStatus(zone, kline)
		}
	}
}

// ValidateZoneIntegrity 验证区域完整性 (去噪后的最终检查)
func (flp *FuzzyLogicProcessor) ValidateZoneIntegrity(zone *SupplyDemandZone) bool {
	// 检查1: 破坏状态的区域应该被过滤掉
	if zone.Status == StatusBroken {
		log.Printf("🚫 [模糊逻辑] 区域 %s 已破坏，建议从AI候选中排除", zone.ID)
		return false
	}
	
	// 检查2: 过度弱化的区域质量降级
	if zone.Status == StatusWeakened && zone.TouchCount > 5 {
		log.Printf("⚠️ [模糊逻辑] 区域 %s 过度弱化，建议降低权重", zone.ID)
		return false
	}
	
	return true
}

// GetCleanZonesForAI 获取经过模糊逻辑清理的供需区 (传给AI的最终结果)
func (flp *FuzzyLogicProcessor) GetCleanZonesForAI(zones []*SupplyDemandZone) []*SupplyDemandZone {
	var cleanZones []*SupplyDemandZone
	
	for _, zone := range zones {
		if zone == nil {
			continue
		}
		
		// 应用模糊逻辑过滤
		if flp.ValidateZoneIntegrity(zone) {
			cleanZones = append(cleanZones, zone)
		}
	}
	
	log.Printf("✅ [模糊逻辑] 清理完成: %d/%d 区域通过验证传给AI", len(cleanZones), len(zones))
	return cleanZones
}

// ===== 辅助方法 =====

// getBreakPrice 获取突破价格 (辅助调试)
func (zone *SupplyDemandZone) getBreakPrice(kline Kline) float64 {
	switch zone.Type {
	case SupplyZone:
		return zone.UpperBound
	case DemandZone:
		return zone.LowerBound
	default:
		return 0
	}
}

// getBreakDepth 获取突破深度 (辅助调试)
func (zone *SupplyDemandZone) getBreakDepth(kline Kline) float64 {
	switch zone.Type {
	case SupplyZone:
		if kline.Close > zone.UpperBound {
			return kline.Close - zone.UpperBound
		}
	case DemandZone:
		if kline.Close < zone.LowerBound {
			return zone.LowerBound - kline.Close
		}
	}
	return 0
}