package microstructure

import (
    "log"
    "math"
    "os"
    "time"
)

// ===== P0-06修复：墙稳定性除零问题修复 =====

// calculateWallStabilityFixed 🔥 P0-06修复版：墙稳定性计算（消除除零风险）
func (calc *OrderBookCalculator) calculateWallStabilityFixed(symbol string, price, currentSize float64) (float64, int, time.Duration) {
    symbolData := calc.symbolData[symbol]
    if symbolData == nil {
        return 0.5, 0, 0
    }

    // 🔧 修复：使用分桶机制查找墙历史
    bucketKey := calc.priceToBucket(price, symbolData.wallTracker.tickSize)
    entry, exists := symbolData.wallTracker.wallHistory[bucketKey]
    if !exists {
        // 新墙，返回默认值
        return 0.5, 0, 0
    }

    // 计算存在时长
    existenceDuration := entry.lastSeen.Sub(entry.firstSeen)

    // 计算稳定性评分 (0-1)
    stabilityScore := 0.0

    // 1. 时间稳定性 (40%权重)
    timeScore := math.Min(existenceDuration.Minutes()/30, 1.0) // 30分钟为满分
    stabilityScore += timeScore * 0.4

    // 🔥 P0-06修复：2. 闪烁频率 (30%权重) - 使用安全除法
    flickerRatio := safeDivisionForOrderBook(float64(entry.disappearances), float64(entry.appearances))
    flickerScore := math.Max(0, 1.0-flickerRatio) // 闪烁越少评分越高
    stabilityScore += flickerScore * 0.3

    // 🔧 P0-06修复：添加业务语义处理
    // appearances=0 时，说明是全新墙，稳定性应该偏低但不为0
    if entry.appearances == 0 {
        // 新墙的闪烁评分应该较低，但给予基础分
        flickerScore = 0.2 // 给予20%的基础分，体现"稳定性未知"的保守评估
        stabilityScore = stabilityScore - (flickerScore * 0.3) + (0.2 * 0.3)
        
        if os.Getenv("NOFX_DEBUG") == "true" {
            log.Printf("🔧 [P0-06] 新墙稳定性评估: %s@%.2f, appearances=0, 使用保守评分=%.3f", 
                symbol, price, 0.2)
        }
    }

    // 3. 大小一致性 (20%权重)
    sizeConsistency := 0.0
    if entry.maxSize > 0 {
        sizeVariation := (entry.maxSize - entry.minSize) / entry.maxSize
        sizeConsistency = math.Max(0, 1.0-sizeVariation) // 变化越小评分越高
    }
    stabilityScore += sizeConsistency * 0.2

    // 4. 当前活跃性 (10%权重)
    activeScore := 0.0
    if entry.isActive {
        activeScore = 1.0
    }
    stabilityScore += activeScore * 0.1

    // 🔧 P0-06修复：确保评分在0-1范围内，并验证没有异常值
    stabilityScore = math.Max(0, math.Min(1, stabilityScore))
    
    // 🔧 P0-06修复：异常值检查和日志
    if math.IsNaN(stabilityScore) || math.IsInf(stabilityScore, 0) {
        log.Printf("❌ [P0-06] 墙稳定性计算异常: symbol=%s, price=%.2f, score=%f, 强制设为0.1", 
            symbol, price, stabilityScore)
        stabilityScore = 0.1 // 异常时使用保守的低评分
    }

    return stabilityScore, entry.disappearances, existenceDuration
}

// calculateSpoofingRiskFixed 🔥 P0-06修复版：虚假挂单风险计算（增强安全性）
func (calc *OrderBookCalculator) calculateSpoofingRiskFixed(symbol string) float64 {
    symbolData := calc.symbolData[symbol]
    if symbolData == nil {
        return 0
    }

    now := time.Now()
    recentTimeThreshold := now.Add(-5 * time.Minute) // 🔧 修复：只看最近5分钟的墙

    var maxSingleRisk float64 = 0.0 // 🔧 修复：取单个墙的最大风险，而非累加
    activeWallCount := 0

    // 🔧 修复：只遍历活跃且最近的墙
    for _, entry := range symbolData.wallTracker.wallHistory {
        // 🔧 修复：过滤条件 - 只看活跃且最近有活动的墙
        if !entry.isActive || entry.lastSeen.Before(recentTimeThreshold) {
            continue
        }

        activeWallCount++

        // 计算单个墙的风险评分
        singleRisk := 0.0

        // 🔥 P0-06修复：闪烁风险 - 使用安全除法，并增强业务逻辑
        if entry.appearances > 0 {
            flickerRatio := safeDivisionForOrderBook(float64(entry.disappearances), float64(entry.appearances))
            if flickerRatio > 0.5 { // 消失次数超过出现次数的一半才算风险
                singleRisk += math.Min(0.6, flickerRatio) // 最高0.6分
            }
        } else {
            // 🔥 P0-06修复：appearances=0的业务处理
            // 全新墙（从未重现）应该有轻微的不确定性风险，但不是高风险
            if entry.disappearances > 0 {
                // 如果有消失记录但无出现记录，这是数据异常，给予中等风险
                singleRisk += 0.4
                log.Printf("⚠️ [P0-06] 检测到异常墙数据: disappearances=%d, appearances=0, 给予中等风险", 
                    entry.disappearances)
            } else {
                // 全新墙，给予轻微的不确定性风险
                singleRisk += 0.1
            }
        }

        // 大小变化风险：如果墙的大小变化过于剧烈
        if entry.maxSize > 0 && len(entry.sizeHistory) > 3 {
            sizeVariation := (entry.maxSize - entry.minSize) / entry.maxSize
            if sizeVariation > 0.8 { // 大小变化超过80%
                singleRisk += math.Min(0.3, sizeVariation-0.5) // 最高0.3分
            }
        }

        // 时间衰减：越久的墙风险越低
        existenceMinutes := now.Sub(entry.firstSeen).Minutes()
        timeDecay := math.Max(0.1, 1.0-existenceMinutes/60) // 1小时后衰减到0.1
        singleRisk *= timeDecay

        // 🔧 修复：取最大值而非累加
        if singleRisk > maxSingleRisk {
            maxSingleRisk = singleRisk
        }
    }

    // 🔧 修复：基于5分钟内墙变化频率的额外风险（但有上限）
    changeFrequencyRisk := 0.0
    if symbolData.wallChangeCount5m > 20 { // 5分钟内变化超过20次才算异常
        changeFrequencyRisk = math.Min(0.4, float64(symbolData.wallChangeCount5m-20)/50.0)
    }

    // 🔧 修复：最终风险评分 = max(单墙风险, 变化频率风险)
    finalRisk := math.Max(maxSingleRisk, changeFrequencyRisk)

    // 🔧 修复：如果没有活跃墙，风险为0
    if activeWallCount == 0 {
        finalRisk = 0
    }

    // 🔥 P0-06修复：异常值检查
    if math.IsNaN(finalRisk) || math.IsInf(finalRisk, 0) {
        log.Printf("❌ [P0-06] 虚假挂单风险计算异常: symbol=%s, risk=%f, 强制设为0.5", 
            symbol, finalRisk)
        finalRisk = 0.5 // 异常时使用中等风险评分
    }

    // 确保在0-1范围内
    return math.Max(0, math.Min(1, finalRisk))
}

// ===== P0-06修复：替换原有方法 =====

// GetCurrentOrderBookDataFixed 🔥 P0-06修复版：使用安全的稳定性计算
func (calc *OrderBookCalculator) GetCurrentOrderBookDataFixed(symbol string, smoothPeriods int) *OrderBookData {
    calc.mu.RLock()
    defer calc.mu.RUnlock()

    symbolData := calc.symbolData[symbol]
    if symbolData == nil {
        // 返回空数据，标记为过期
        return &OrderBookData{
            ImbalanceRatio:    0,
            NearestResistance: nil,
            NearestSupport:    nil,
            BidPressure:       0,
            AskPressure:       0,
            ImbalanceTrend:    "insufficient_data",
            PressureDelta5m:   0,
            SpoofingRisk:      0,
            LiquidityScore:    0,
            WallChangeCount5m: 0,
            LastUpdate:        time.Time{},
            IsStale:           true,
        }
    }

    // 计算失衡比例（平滑）
    imbalanceRatio := calc.getSmoothedImbalance(symbol, smoothPeriods)

    // 🔥 P0-06修复：使用安全的墙查找方法
    resistance, support := calc.findWallsFixed(symbol)

    // 计算买卖压力（使用新的压力计算方法）
    bidPressure := calc.calculatePressureEnhanced(symbol, symbolData.currentBids, true)
    askPressure := calc.calculatePressureEnhanced(symbol, symbolData.currentAsks, false)

    // 检查数据是否过期
    isStale := time.Since(symbolData.lastUpdate) > 5*time.Minute

    // V2.0: 计算额外的市场微观结构指标
    imbalanceTrend := calc.calculateImbalanceTrend(symbol)
    pressureDelta5m := calc.calculatePressureDelta5mFixed(symbol, bidPressure, askPressure, symbolData.lastUpdate)
    // 🔥 P0-06修复：使用安全的虚假挂单风险计算
    spoofingRisk := calc.calculateSpoofingRiskFixed(symbol)
    liquidityScore := calc.calculateLiquidityScore(symbol)

    orderBookData := &OrderBookData{
        ImbalanceRatio:    imbalanceRatio,
        NearestResistance: resistance,
        NearestSupport:    support,
        BidPressure:       bidPressure,
        AskPressure:       askPressure,
        // V2.0 新增字段
        ImbalanceTrend:    imbalanceTrend,
        PressureDelta5m:   pressureDelta5m,
        SpoofingRisk:      spoofingRisk,
        LiquidityScore:    liquidityScore,
        WallChangeCount5m: symbolData.wallChangeCount5m,
        LastUpdate:        symbolData.lastUpdate,
        IsStale:           isStale,
    }

    // V2.0: 缓存到全局缓存系统
    globalCache := GetGlobalCache()
    globalCache.SetOrderBookData(symbol, orderBookData)

    // 只在异常情况下打印详细日志
    if spoofingRisk > 0.8 || math.Abs(imbalanceRatio) > 0.9 || liquidityScore < 0.5 {
        log.Printf("⚠️ [%s] 盘口异常: 失衡比%.3f, 虚假挂单风险%.3f, 流动性评分%.3f (P0-06修复版)",
            symbol, imbalanceRatio, spoofingRisk, liquidityScore)
    }

    return orderBookData
}

// findWallsFixed 🔥 P0-06修复版：识别挂单墙（使用安全的稳定性计算）
func (calc *OrderBookCalculator) findWallsFixed(symbol string) (resistance *WallInfo, support *WallInfo) {
    symbolData := calc.symbolData[symbol]
    if symbolData == nil {
        return nil, nil
    }

    // 计算平均档位量
    avgBidValue := calc.calculateAverageLevel(symbolData.currentBids)
    avgAskValue := calc.calculateAverageLevel(symbolData.currentAsks)
    avgValue := (avgBidValue + avgAskValue) / 2

    if avgValue == 0 {
        return nil, nil
    }

    threshold := avgValue * calc.wallThreshold

    // 🔥 P0-06修复：查找阻力墙（使用安全稳定性计算）
    resistance = calc.findResistanceWallFixed(symbol, threshold)

    // 🔥 P0-06修复：查找支撑墙（使用安全稳定性计算）
    support = calc.findSupportWallFixed(symbol, threshold)

    return resistance, support
}

// findResistanceWallFixed 🔥 P0-06修复版：查找阻力墙
func (calc *OrderBookCalculator) findResistanceWallFixed(symbol string, threshold float64) *WallInfo {
    symbolData := calc.symbolData[symbol]
    if symbolData == nil || len(symbolData.currentAsks) == 0 {
        return nil
    }

    var wallPrice float64
    var wallStrength float64
    var wallLevels int

    // 查找连续的大档位
    for i, ask := range symbolData.currentAsks {
        levelValue := ask.Price * ask.Quantity

        if levelValue > threshold {
            if wallPrice == 0 {
                wallPrice = ask.Price
                wallStrength = levelValue
                wallLevels = 1
            } else {
                // 检查是否是连续的墙
                if i > 0 {
                    prevPrice := symbolData.currentAsks[i-1].Price
                    if (ask.Price-prevPrice)/prevPrice < 0.001 { // 价差小于0.1%认为连续
                        wallStrength += levelValue
                        wallLevels++
                    } else {
                        break // 不连续，停止累加
                    }
                }
            }
        } else if wallPrice != 0 {
            break // 已找到墙，后面档位不够大，停止
        }
    }

    if wallPrice == 0 {
        return nil
    }

    // 计算距离当前价格的百分比
    distance := 0.0
    if symbolData.currentPrice > 0 {
        distance = (wallPrice - symbolData.currentPrice) / symbolData.currentPrice * 100
    }

    // 🔥 P0-06修复：V2.0: 计算稳定性评分（使用安全方法）
    stabilityScore, flickerCount, existenceDuration := calc.calculateWallStabilityFixed(symbol, wallPrice, wallStrength)

    return &WallInfo{
        Price:       wallPrice,
        StrengthUSD: wallStrength,
        IsSolid:     wallStrength > threshold*2, // 超过2倍阈值认为是实墙
        Distance:    distance,
        LevelCount:  wallLevels,
        // 🔥 P0-06修复：V2.0 新增字段（使用安全计算）
        StabilityScore:    stabilityScore,
        FlickerCount:      flickerCount,
        ExistenceDuration: existenceDuration,
        LastSeen:          symbolData.lastUpdate,
        FirstSeen:         calc.getWallFirstSeen(symbol, wallPrice),
        AverageSize:       calc.getWallAverageSize(symbol, wallPrice),
        MaxSize:           calc.getWallMaxSize(symbol, wallPrice),
        MinSize:           calc.getWallMinSize(symbol, wallPrice),
    }
}

// findSupportWallFixed 🔥 P0-06修复版：查找支撑墙
func (calc *OrderBookCalculator) findSupportWallFixed(symbol string, threshold float64) *WallInfo {
    symbolData := calc.symbolData[symbol]
    if symbolData == nil || len(symbolData.currentBids) == 0 {
        return nil
    }

    var wallPrice float64
    var wallStrength float64
    var wallLevels int

    // 查找连续的大档位（买单从高到低）
    for i, bid := range symbolData.currentBids {
        levelValue := bid.Price * bid.Quantity

        if levelValue > threshold {
            if wallPrice == 0 {
                wallPrice = bid.Price
                wallStrength = levelValue
                wallLevels = 1
            } else {
                // 检查是否是连续的墙
                if i > 0 {
                    prevPrice := symbolData.currentBids[i-1].Price
                    if (prevPrice-bid.Price)/prevPrice < 0.001 { // 价差小于0.1%认为连续
                        wallStrength += levelValue
                        wallLevels++
                    } else {
                        break // 不连续，停止累加
                    }
                }
            }
        } else if wallPrice != 0 {
            break // 已找到墙，后面档位不够大，停止
        }
    }

    if wallPrice == 0 {
        return nil
    }

    // 计算距离当前价格的百分比
    distance := 0.0
    if symbolData.currentPrice > 0 {
        distance = (symbolData.currentPrice - wallPrice) / symbolData.currentPrice * 100
    }

    // 🔥 P0-06修复：V2.0: 计算稳定性评分（使用安全方法）
    stabilityScore, flickerCount, existenceDuration := calc.calculateWallStabilityFixed(symbol, wallPrice, wallStrength)

    return &WallInfo{
        Price:       wallPrice,
        StrengthUSD: wallStrength,
        IsSolid:     wallStrength > threshold*2, // 超过2倍阈值认为是实墙
        Distance:    distance,
        LevelCount:  wallLevels,
        // 🔥 P0-06修复：V2.0 新增字段（使用安全计算）
        StabilityScore:    stabilityScore,
        FlickerCount:      flickerCount,
        ExistenceDuration: existenceDuration,
        LastSeen:          symbolData.lastUpdate,
        FirstSeen:         calc.getWallFirstSeen(symbol, wallPrice),
        AverageSize:       calc.getWallAverageSize(symbol, wallPrice),
        MaxSize:           calc.getWallMaxSize(symbol, wallPrice),
        MinSize:           calc.getWallMinSize(symbol, wallPrice),
    }
}