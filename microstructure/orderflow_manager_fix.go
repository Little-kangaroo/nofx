package microstructure

import (
    "fmt"
    "log"
    "time"
)

// ===== P0-04修复：OrderFlow管理器方法重写 =====

// GetCVDDataV2 获取CVD数据（P0-04修复版本 - 强一致性）
func (manager *CVDManager) GetCVDDataV2(symbol string) *CVDDataV2 {
    manager.mu.RLock()
    calc, exists := manager.calculators[symbol]
    manager.mu.RUnlock()

    if !exists {
        // 🔥 P0-04修复：无数据时使用零值时间，不使用time.Now()
        return &CVDDataV2{
            SpotCVD1H:     0,
            FuturesCVD1H:  0,
            CVDDivergence: "no_data",
            Signal:        CVDSignalNeutral,
            DataHealth:    NewNoDataHealth(), // ✅ 强一致性：明确标记为无数据
        }
    }

    // 获取真实的CVD数据
    cvdData := calc.GetCurrentCVD()
    
    // 基于真实的lastDataUpdate判断数据健康度
    var dataHealth DataHealthInfo
    if cvdData.IsStale {
        dataHealth = NewStaleDataHealth(calc.lastDataUpdate) // ✅ 使用真实的数据更新时间
    } else {
        // 计算真实的数据质量
        realQuality := calc.calculateDataQuality()
        dataHealth = NewFreshDataHealth(calc.lastDataUpdate, realQuality)
    }

    return &CVDDataV2{
        SpotCVD1H:     cvdData.SpotCVD1H,
        FuturesCVD1H:  cvdData.FuturesCVD1H,
        CVDDivergence: cvdData.CVDDivergence,
        Signal:        cvdData.Signal,
        DataHealth:    dataHealth, // ✅ P0-04修复：强一致性健康度
    }
}

// GetOIAnalysisV2 获取OI分析（P0-04修复版本 - 强一致性）
func (manager *OIManager) GetOIAnalysisV2Fixed(symbol string) *OIAnalysisV2 {
    manager.mu.RLock()
    calc, exists := manager.calculators[symbol]
    manager.mu.RUnlock()

    if !exists {
        // 🔥 P0-04修复：无数据时绝不使用time.Now()
        return &OIAnalysisV2{
            Current:      0,
            Change1H:     0,
            Change4H:     0,
            ChangeRate1H: 0,
            ChangeRate4H: 0,
            Trend:        "no_data",
            DataHealth:   NewNoDataHealth(), // ✅ 强一致性：明确标记为无数据
        }
    }

    // 获取真实的OI分析结果
    oiAnalysis := calc.GetOIAnalysis()
    
    // 基于真实的lastUpdate判断数据健康度
    var dataHealth DataHealthInfo
    if oiAnalysis.IsStale {
        dataHealth = NewStaleDataHealth(calc.lastUpdate) // ✅ 使用真实的数据更新时间
    } else {
        // 基于数据年龄和完整性计算质量
        dataAge := time.Since(calc.lastUpdate).Minutes()
        quality := 1.0
        if dataAge > 2 {
            quality = 0.8
        }
        if len(calc.changes) < 5 {
            quality *= 0.7 // 数据点不足降低质量
        }
        dataHealth = NewFreshDataHealth(calc.lastUpdate, quality)
    }

    return &OIAnalysisV2{
        Current:      oiAnalysis.Current,
        Change1H:     oiAnalysis.Change1H,
        Change4H:     oiAnalysis.Change4H,
        ChangeRate1H: oiAnalysis.ChangeRate1H,
        ChangeRate4H: oiAnalysis.ChangeRate4H,
        Trend:        oiAnalysis.Trend,
        DataHealth:   dataHealth, // ✅ P0-04修复：强一致性健康度
    }
}

// GetCurrentOrderBookDataV2 获取订单簿数据（P0-04修复版本）
func (calc *OrderBookCalculator) GetCurrentOrderBookDataV2Fixed(symbol string, smoothPeriods int) *OrderBookDataV2 {
    calc.mu.RLock()
    defer calc.mu.RUnlock()

    symbolData := calc.symbolData[symbol]
    if symbolData == nil {
        // 🔥 P0-04修复：无数据时绝不使用time.Now()
        return &OrderBookDataV2{
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
            DataHealth:        NewNoDataHealth(), // ✅ 强一致性：明确标记为无数据
        }
    }

    // 计算所有指标
    imbalanceRatio := calc.getSmoothedImbalance(symbol, smoothPeriods)
    resistance, support := calc.findWalls(symbol)
    bidPressure := calc.calculatePressureEnhanced(symbol, symbolData.currentBids, true)
    askPressure := calc.calculatePressureEnhanced(symbol, symbolData.currentAsks, false)
    
    // 基于真实的lastUpdate判断数据健康度
    var dataHealth DataHealthInfo
    dataAge := time.Since(symbolData.lastUpdate).Minutes()
    
    if dataAge > 5 { // 订单簿数据5分钟过期
        dataHealth = NewStaleDataHealth(symbolData.lastUpdate)
    } else {
        // 计算数据质量
        quality := 1.0
        if len(symbolData.currentBids) < 5 || len(symbolData.currentAsks) < 5 {
            quality *= 0.6 // 深度不足
        }
        if dataAge > 1 {
            quality *= 0.9 // 轻微老化
        }
        dataHealth = NewFreshDataHealth(symbolData.lastUpdate, quality)
    }

    // V2.0: 计算额外指标
    imbalanceTrend := calc.calculateImbalanceTrend(symbol)
    pressureDelta5m := calc.calculatePressureDelta5mFixed(symbol, bidPressure, askPressure, symbolData.lastUpdate)
    spoofingRisk := calc.calculateSpoofingRisk(symbol)
    liquidityScore := calc.calculateLiquidityScore(symbol)

    return &OrderBookDataV2{
        ImbalanceRatio:    imbalanceRatio,
        NearestResistance: resistance,
        NearestSupport:    support,
        BidPressure:       bidPressure,
        AskPressure:       askPressure,
        ImbalanceTrend:    imbalanceTrend,
        PressureDelta5m:   pressureDelta5m,
        SpoofingRisk:      spoofingRisk,
        LiquidityScore:    liquidityScore,
        WallChangeCount5m: symbolData.wallChangeCount5m,
        DataHealth:        dataHealth, // ✅ P0-04修复：强一致性健康度
    }
}

// generateMacroTrendDataV2 生成宏观趋势数据（P0-04修复版本）
func (ofm *OrderFlowManager) generateMacroTrendDataV2(symbol string, cvdDataV2 *CVDDataV2, oiAnalysisV2 *OIAnalysisV2) *MacroTrendDataV2 {
    // 🔥 P0-04修复：检查数据可用性，绝不使用time.Now()作为LastUpdate
    if cvdDataV2 == nil || oiAnalysisV2 == nil || 
       !cvdDataV2.DataHealth.HasData || !oiAnalysisV2.DataHealth.HasData {
        
        return &MacroTrendDataV2{
            SpotCVD1H:         0,
            FuturesCVD1H:      0,
            CVDDivergence4H:   false,
            MarketRegime:      "数据不足",
            TrendStrength:     0,
            DominantDirection: "neutral",
            ConfidenceLevel:   0.0, // ✅ 无数据时置信度为0
            DataHealth:        NewNoDataHealth(), // ✅ P0-04修复：明确标记为无数据
        }
    }

    // 使用V1数据结构进行计算（兼容现有算法）
    cvdData := &CVDData{
        SpotCVD1H:     cvdDataV2.SpotCVD1H,
        FuturesCVD1H:  cvdDataV2.FuturesCVD1H,
        CVDDivergence: cvdDataV2.CVDDivergence,
        Signal:        cvdDataV2.Signal,
    }
    
    oiAnalysis := &OIAnalysis{
        Current:      oiAnalysisV2.Current,
        Change1H:     oiAnalysisV2.Change1H,
        Change4H:     oiAnalysisV2.Change4H,
        ChangeRate1H: oiAnalysisV2.ChangeRate1H,
        ChangeRate4H: oiAnalysisV2.ChangeRate4H,
        Trend:        oiAnalysisV2.Trend,
    }

    // 计算各项指标
    cvdDivergence4H := ofm.detectCVDDivergence4H(cvdData)
    marketRegime := ofm.determineMarketRegime(cvdData, oiAnalysis)
    trendStrength := ofm.calculateTrendStrength(cvdData, oiAnalysis)
    dominantDirection := ofm.determineDominantDirection(cvdData, oiAnalysis)
    confidenceLevel := ofm.calculateConfidenceLevel(cvdData, oiAnalysis, trendStrength)

    // 🔥 P0-04修复：基于真实数据健康度计算综合健康度
    dataHealth := ofm.calculateCombinedDataHealth(cvdDataV2.DataHealth, oiAnalysisV2.DataHealth)

    // 🔥 P0-04修复：基于数据质量调整置信度
    adjustedConfidence := confidenceLevel * dataHealth.GetDataConfidence()

    return &MacroTrendDataV2{
        SpotCVD1H:         cvdData.SpotCVD1H,
        FuturesCVD1H:      cvdData.FuturesCVD1H,
        CVDDivergence4H:   cvdDivergence4H,
        MarketRegime:      marketRegime,
        TrendStrength:     trendStrength,
        DominantDirection: dominantDirection,
        ConfidenceLevel:   adjustedConfidence, // ✅ 经过数据质量调整的置信度
        DataHealth:        dataHealth,         // ✅ P0-04修复：强一致性健康度
    }
}

// calculateCombinedDataHealth 计算组合数据健康度
func (ofm *OrderFlowManager) calculateCombinedDataHealth(cvdHealth, oiHealth DataHealthInfo) DataHealthInfo {
    // 如果任一数据源无数据，整体为无数据
    if !cvdHealth.HasData || !oiHealth.HasData {
        return NewNoDataHealth()
    }

    // 取最老的数据时间作为整体数据时间
    var oldestUpdate time.Time
    if cvdHealth.LastRealUpdate.Before(oiHealth.LastRealUpdate) {
        oldestUpdate = cvdHealth.LastRealUpdate
    } else {
        oldestUpdate = oiHealth.LastRealUpdate
    }

    // 计算组合质量：取较低值，确保保守估计
    combinedQuality := cvdHealth.Quality
    if oiHealth.Quality < combinedQuality {
        combinedQuality = oiHealth.Quality
    }

    // 数据年龄基于最老的组件
    dataAge := time.Since(oldestUpdate).Minutes()

    // 确定组合状态
    var status DataStatus
    if dataAge > 30 {
        status = DataStatusStale
    } else if cvdHealth.Status == DataStatusStale || oiHealth.Status == DataStatusStale {
        status = DataStatusStale
    } else if dataAge > 5 {
        status = DataStatusPartial
    } else {
        status = DataStatusFresh
    }

    return DataHealthInfo{
        Status:         status,
        HasData:        true,
        LastRealUpdate: oldestUpdate, // ✅ 使用真实的数据时间
        CreatedAt:      time.Now(),   // 结构创建时间可以使用Now()
        DataAge:        dataAge,
        Quality:        combinedQuality,
    }
}

// ===== P0-04修复：数据质量验证工具 =====

// ValidateDataConsistency 验证数据一致性（P0-04测试专用）
func ValidateDataConsistency(symbol string, manager *OrderFlowManager) error {
    // 获取V2版本数据
    cvdV2 := manager.cvdManager.GetCVDDataV2(symbol)
    oiV2 := manager.oiManager.GetOIAnalysisV2Fixed(symbol)
    
    // 验证无数据情况的一致性
    if !cvdV2.DataHealth.HasData {
        if cvdV2.DataHealth.Status != DataStatusNoData {
            return fmt.Errorf("CVD无数据时状态应为no_data，实际为%s", cvdV2.DataHealth.Status)
        }
        if !cvdV2.DataHealth.LastRealUpdate.IsZero() {
            return fmt.Errorf("CVD无数据时LastRealUpdate应为零值，实际为%v", cvdV2.DataHealth.LastRealUpdate)
        }
        log.Printf("✅ [%s] CVD数据一致性验证通过：无数据状态正确", symbol)
    }
    
    if !oiV2.DataHealth.HasData {
        if oiV2.DataHealth.Status != DataStatusNoData {
            return fmt.Errorf("OI无数据时状态应为no_data，实际为%s", oiV2.DataHealth.Status)
        }
        if !oiV2.DataHealth.LastRealUpdate.IsZero() {
            return fmt.Errorf("OI无数据时LastRealUpdate应为零值，实际为%v", oiV2.DataHealth.LastRealUpdate)
        }
        log.Printf("✅ [%s] OI数据一致性验证通过：无数据状态正确", symbol)
    }
    
    // 验证数据质量与置信度的一致性
    cvdConfidence := cvdV2.DataHealth.GetDataConfidence()
    if cvdConfidence < 0 || cvdConfidence > 1 {
        return fmt.Errorf("CVD置信度越界：%.3f", cvdConfidence)
    }
    
    oiConfidence := oiV2.DataHealth.GetDataConfidence()
    if oiConfidence < 0 || oiConfidence > 1 {
        return fmt.Errorf("OI置信度越界：%.3f", oiConfidence)
    }
    
    log.Printf("✅ [%s] 数据一致性验证完成：CVD置信度%.3f，OI置信度%.3f", 
        symbol, cvdConfidence, oiConfidence)
    
    return nil
}