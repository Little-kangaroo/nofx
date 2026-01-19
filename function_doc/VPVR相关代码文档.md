# VPVR (成交量分布) 相关代码完整文档

## 概述

本文档详细记录了AI自动交易系统中VPVR(Volume Profile Visible Range/成交量分布可见范围)相关的逻辑代码，包括成交量分布计算、POC识别、HVN/LVN节点检测等核心算法。VPVR是市场微观结构分析的重要工具，为AI提供关键的价量关系信息。

---

## 一、核心文件列表

### 1.1 主要VPVR文件

| 文件路径 | 行数 | 主要功能 |
|---------|------|---------|
| `market/vpvr.go` | 3006 | VPVR分析引擎，成交量分布计算 |
| `market/types.go` | 900+ | 数据结构定义 |
| `market/data_cleaner.go` | 300+ | 数据清洗模块 |
| `market/atr_manager.go` | 189 | ATR管理（用于TickSize计算） |

### 1.2 集成文件

- `market/comprehensive_analysis.go` - 多时间框架综合分析
- `market/context_scoring.go` - 上下文评分计算
- `market/json_contract_utils.go` - JSON序列化

---

## 二、数据结构定义

### 2.1 核心数据结构

#### VolumeProfile - 成交量分布主结构
```go
type VolumeProfile struct {
    POC          *PriceLevel     // Point of Control - 最大成交量价格
    VAH          float64         // Value Area High - 价值区域高点
    VAL          float64         // Value Area Low - 价值区域低点
    ValueArea    *ValueArea      // 价值区域详细信息
    Levels       []*PriceLevel   // 所有价格级别
    Config       *VPVRConfig     // VPVR配置
    Stats        *VolumeStats    // 成交量统计
    Context      *ContextMetrics // 上下文评分
    UsedTimeFrame string         // 实际使用的时间框架
    UsedTickSize  float64        // 实际使用的tick_size
}
```

**用途**: 包含完整的成交量分布分析结果，传递给AI进行决策

---

#### PriceLevel - 价格级别成交量数据
```go
type PriceLevel struct {
    Price         float64       // 价格
    Volume        float64       // 总成交量
    BuyVolume     float64       // 买入成交量
    SellVolume    float64       // 卖出成交量
    VolumePercent float64       // 成交量占比
    Transactions  int           // 交易次数
    IsPOC         bool          // 是否为POC
    InValueArea   bool          // 是否在价值区域内

    // HVN/LVN节点标记
    IsHVN         bool          // 高成交量节点
    IsLVN         bool          // 低成交量节点
    VolumeRank    int           // 成交量排名
    HVNStrength   float64       // HVN强度评分 (0-100)
    LVNStrength   float64       // LVN强度评分 (0-100)
    NodeType      VolumeNodeType // 节点类型
}
```

**节点类型枚举**:
```go
VolumeNodeNormal   // 普通节点
VolumeNodeHVN      // 高成交量节点
VolumeNodeLVN      // 低成交量节点
VolumeNodePOC      // POC节点
VolumeNodeCluster  // 聚集节点
VolumeNodeGap      // 空隙节点
```

---

#### VolumeStats - 成交量统计信息
```go
type VolumeStats struct {
    TotalVolume           float64          // 总成交量
    TotalBuyVolume        float64          // 总买入成交量
    TotalSellVolume       float64          // 总卖出成交量
    BuySellRatio          float64          // 买卖比
    AvgPrice              float64          // 成交量加权平均价格
    MedianPrice           float64          // 中位数价格
    PriceStdDev           float64          // 价格标准差
    MaxLevel, MinLevel    *PriceLevel      // 最大/最小成交量级别

    // POC分析字段
    POCDensity            float64          // POC密度：POC/总量百分比
    POCStrength           float64          // POC强度：相对于周围的强度评分
    POCCluster            *POCClusterInfo  // POC聚集信息

    // 价值区宽度分析
    ValueAreaWidthRatio   float64          // 价值区宽度占比
    MarketProfile         MarketProfileType // 市场廓型类型
    MarketRegimeStrength  float64          // 市场强度评分 (0-100)
    TrendStrength         float64          // 趋势强度评分 (0-100)
    ConsolidationLevel    float64          // 整理程度评分 (0-100)

    // HVN/LVN统计
    HVNCount, LVNCount    int              // 节点数量
    HVNTotalVolume        float64          // HVN总成交量
    LVNTotalVolume        float64          // LVN总成交量
    HVNVolumePercent      float64          // HVN成交量占比
    LVNVolumePercent      float64          // LVN成交量占比
    VolumeConcentration   float64          // 成交量集中度评分 (0-100)
    PriceGapCount         int              // 价格空隙数量
}
```

---

#### POCClusterInfo - POC聚集信息
```go
type POCClusterInfo struct {
    ClusterRange      float64   // POC聚集区间范围
    ClusterVolume     float64   // 聚集区间总成交量
    ClusterLevels     int       // 聚集区间价格级别数
    ClusterDensity    float64   // 聚集密度：成交量/价格范围
    RelativeStrength  float64   // 相对强度：与其他区间的对比
    SupportLevel      float64   // 支撑强度评级 (0-100)
    ResistanceLevel   float64   // 阻力强度评级 (0-100)
    BreakoutProb      float64   // 突破概率估算 (0-1)
    VolumeProfile     []float64 // 聚集区间内的成交量分布
}
```

---

#### MarketProfileType - 市场廓型类型

**6种市场状态**:
```go
const (
    MarketProfileTrending      = "Trending"      // 趋势型
    MarketProfileConsolidation = "Consolidation" // 整理型
    MarketProfileVolatile      = "Volatile"      // 波动型
    MarketProfileBalance       = "Balance"       // 平衡型
    MarketProfileBreakout      = "Breakout"      // 突破型
    MarketProfileRotation      = "Rotation"      // 轮动型
    MarketProfileUnknown       = "Unknown"       // 未知型
)
```

---

## 三、核心算法分析

### 3.1 成交量分布计算

#### calculatePriceLevels - 将K线成交量分配到价格级别

**目标**: 将K线成交量分配到相应的价格级别

**算法步骤**:

**步骤1: 确定价格范围和TickSize**
```go
func (va *VPVRAnalyzer) calculatePriceLevels(klines []Kline) []*PriceLevel {
    // 1. 确定价格范围
    minPrice := klines[0].Low
    maxPrice := klines[0].High
    for _, k := range klines {
        minPrice = min(minPrice, k.Low)
        maxPrice = max(maxPrice, k.High)
    }

    // 2. 计算稳定的TickSize (Task 7)
    tickSize := va.calculateStableTickSize(klines, minPrice, maxPrice)

    // 3. 限制最大级别数 (不超过200)
    numLevels := int((maxPrice - minPrice) / tickSize)
    if numLevels > 200 {
        tickSize = (maxPrice - minPrice) / 200
        numLevels = 200
    }

    // 4. 初始化价格级别映射
    levelMap := make(map[float64]*PriceLevel)
    // ...
}
```

**步骤2: 分配K线成交量**
```go
// 5. 遍历每根K线，分配成交量
for _, kline := range klines {
    // 使用稳定分配算法
    va.distributePriceVolumeStabilized(kline, levelMap, tickSize)
}
```

**步骤3: distributePriceVolumeStabilized - 稳定成交量分配**
```go
func (va *VPVRAnalyzer) distributePriceVolumeStabilized(
    kline Kline, levelMap map[float64]*PriceLevel, tickSize float64) {

    // a. 检查K线价格范围
    priceRange := kline.High - kline.Low
    if priceRange == 0 {
        // 价格无变化，全部分配到收盘价
        price := roundToTickSize(kline.Close, tickSize)
        level := getOrCreateLevel(levelMap, price)
        level.Volume += kline.Volume
        return
    }

    // b. 处理真实买卖量数据优先
    if va.config.UseRealBuySellData && hasRealData(kline) {
        // 使用真实买卖量
        buyVolume := kline.BuyVolume
        sellVolume := kline.SellVolume
    } else {
        // c. 降级: 使用增强的算法估算买卖比例
        buyRatio := va.calculateEnhancedBuySellRatio(kline)
        buyVolume := kline.Volume × buyRatio
        sellVolume := kline.Volume × (1 - buyRatio)
    }

    // d. 多级别插值分配
    va.distributeVolumeAcrossLevels(kline, levelMap, tickSize, buyVolume, sellVolume)
}
```

**步骤4: distributeVolumeAcrossLevels - 多级别插值分配**
```go
func (va *VPVRAnalyzer) distributeVolumeAcrossLevels(
    kline, levelMap, tickSize, buyVolume, sellVolume) {

    // OHLC权重分配: Open(25%), High(20%), Low(20%), Close(35%)
    ohlcPrices := []float64{kline.Open, kline.High, kline.Low, kline.Close}
    ohlcWeights := []float64{0.25, 0.20, 0.20, 0.35}

    // 距离权重: 越接近价格中心，权重越高
    centerPrice := (kline.High + kline.Low) / 2

    for i, price := range ohlcPrices {
        levelPrice := roundToTickSize(price, tickSize)
        level := getOrCreateLevel(levelMap, levelPrice)

        // 计算距离权重
        distance := abs(price - centerPrice) / (kline.High - kline.Low)
        distanceWeight := 1.0 - distance

        // 综合权重
        weight := ohlcWeights[i] × distanceWeight

        // 分配成交量
        level.Volume += kline.Volume × weight
        level.BuyVolume += buyVolume × weight
        level.SellVolume += sellVolume × weight
        level.Transactions++
    }
}
```

**关键优化**:
- 填充空洞效应: 使用插值确保K线跨越的所有价格级别都有成交量
- OHLC权重分配: Open(25%), High(20%), Low(20%), Close(35%)
- 距离权重: 越接近价格中心，权重越高

---

### 3.2 POC识别

#### findPOC - 找到Point of Control

**算法**: 简单线性扫描，找最大成交量的价格级别

```go
func (va *VPVRAnalyzer) findPOC(levels []*PriceLevel) *PriceLevel {
    if len(levels) == 0 {
        return nil
    }

    poc := levels[0]
    for _, level := range levels {
        if level.Volume > poc.Volume {
            poc = level
        }
    }

    poc.IsPOC = true
    return poc
}
```

**时间复杂度**: O(n) - 单次扫描

---

### 3.3 价值区域计算

#### calculateValueArea - 从POC向两侧扩展

**目标**: 找到包含70%成交量（可配置）的价格区间

**算法**:
```go
func (va *VPVRAnalyzer) calculateValueArea(levels []*PriceLevel, poc *PriceLevel, totalVolume float64) *ValueArea {
    targetVolume := totalVolume × 0.7  // 默认70%

    // 1. 找到POC位置
    pocIndex := findIndex(levels, poc)

    // 2. 初始化
    accumulatedVolume := poc.Volume
    upperIndex := pocIndex
    lowerIndex := pocIndex

    // 3. 循环扩展直到累积成交量 >= targetVolume
    for accumulatedVolume < targetVolume {
        // a. 比较上方和下方相邻价格级别
        upperVolume := 0.0
        lowerVolume := 0.0

        if upperIndex < len(levels)-1 {
            upperVolume = levels[upperIndex+1].Volume
        }

        if lowerIndex > 0 {
            lowerVolume = levels[lowerIndex-1].Volume
        }

        // b. 选择成交量较大的方向扩展
        if upperVolume > lowerVolume && upperIndex < len(levels)-1 {
            upperIndex++
            accumulatedVolume += upperVolume
            levels[upperIndex].InValueArea = true
        } else if lowerIndex > 0 {
            lowerIndex--
            accumulatedVolume += lowerVolume
            levels[lowerIndex].InValueArea = true
        } else {
            break
        }
    }

    // 4. 记录VAH和VAL
    vah := levels[upperIndex].Price
    val := levels[lowerIndex].Price

    // 5. 计算集中度
    vaRange := vah - val
    totalRange := levels[len(levels)-1].Price - levels[0].Price
    concentration := (accumulatedVolume / totalVolume) / (vaRange / totalRange)

    return &ValueArea{
        High: vah,
        Low: val,
        VolumePercent: accumulatedVolume / totalVolume × 100,
        Concentration: concentration,
    }
}
```

**时间复杂度**: O(n) - 最多扫描全部级别

---

### 3.4 HVN/LVN节点检测

#### identifyVolumeNodes - 8步骤识别流程

**步骤1: 计算成交量统计指标**
```go
func (va *VPVRAnalyzer) calculateVolumeStatistics(levels []*PriceLevel) *VolumeStatistics {
    volumes := extractVolumes(levels)

    mean := calculateMean(volumes)
    median := calculateMedian(volumes)
    stdDev := calculateStdDev(volumes)

    // 四分位数
    q1 := calculatePercentile(volumes, 25)
    q3 := calculatePercentile(volumes, 75)
    iqr := q3 - q1

    // HVN阈值 = min(Q3 + 1.5×IQR, Mean + 1.2×StdDev)
    hvnThreshold := min(q3 + 1.5×iqr, mean + 1.2×stdDev)

    // LVN阈值 = max(Q1 - 1.5×IQR, Mean × 0.1)
    lvnThreshold := max(q1 - 1.5×iqr, mean × 0.1)

    return &VolumeStatistics{
        Mean: mean,
        Median: median,
        StdDev: stdDev,
        HVNThreshold: hvnThreshold,
        LVNThreshold: lvnThreshold,
    }
}
```

**步骤2: 排名成交量节点**
```go
func (va *VPVRAnalyzer) rankVolumeNodes(levels []*PriceLevel) {
    // 按降序排列所有价格级别
    sortedLevels := sortByVolumeDesc(levels)

    // 分配VolumeRank (1=最高)
    for i, level := range sortedLevels {
        level.VolumeRank = i + 1
    }
}
```

**步骤3: 识别HVN**
```go
func (va *VPVRAnalyzer) identifyHVNs(levels []*PriceLevel, stats *VolumeStatistics) {
    for i, level := range levels {
        // 条件: Volume > HVN阈值 AND (局部最大 OR 相对强度 > 2.0)
        if level.Volume > stats.HVNThreshold {
            isLocalMax := va.isLocalMaximum(levels, i, 3)
            relativeStrength := level.Volume / stats.Mean

            if isLocalMax || relativeStrength > 2.0 {
                level.IsHVN = true
                level.NodeType = VolumeNodeHVN
            }
        }
    }

    // 特殊: POC一定标记为HVN
    if poc != nil {
        poc.IsHVN = true
        poc.NodeType = VolumeNodePOC
    }
}
```

**步骤4: 识别LVN**
```go
func (va *VPVRAnalyzer) identifyLVNs(levels []*PriceLevel, stats *VolumeStatistics) {
    for i, level := range levels {
        // 排除已标记为HVN的节点
        if level.IsHVN {
            continue
        }

        // 条件: Volume < LVN阈值 AND (局部最小 OR 相对强度 < 0.3)
        if level.Volume < stats.LVNThreshold {
            isLocalMin := va.isLocalMinimum(levels, i, 3)
            relativeStrength := level.Volume / stats.Mean

            if isLocalMin || relativeStrength < 0.3 {
                level.IsLVN = true
                level.NodeType = VolumeNodeLVN
            }
        }
    }
}
```

**步骤5: 识别价格空隙**
```go
func (va *VPVRAnalyzer) identifyPriceGaps(levels []*PriceLevel) int {
    gapCount := 0

    // 寻找连续的LVN区域 (>=2个)
    consecutiveLVN := 0
    for _, level := range levels {
        if level.IsLVN {
            consecutiveLVN++
            if consecutiveLVN >= 2 {
                level.NodeType = VolumeNodeGap
                gapCount++
            }
        } else {
            consecutiveLVN = 0
        }
    }

    return gapCount
}
```

**步骤6: 计算节点强度**
```go
func (va *VPVRAnalyzer) calculateNodeStrengths(levels []*PriceLevel, stats *VolumeStatistics) {
    for _, level := range levels {
        if level.IsHVN {
            // HVN强度 = 相对均值倍数 + 相对中位数倍数 + 排名因子 + POC奖励
            meanFactor := level.Volume / stats.Mean × 20
            medianFactor := level.Volume / stats.Median × 20
            rankFactor := (1 - level.VolumeRank/float64(len(levels))) × 40

            level.HVNStrength = meanFactor + medianFactor + rankFactor

            if level.IsPOC {
                level.HVNStrength += 20  // POC额外加分
            }

            level.HVNStrength = min(level.HVNStrength, 100)
        }

        if level.IsLVN {
            // LVN强度 = (1-相对均值比) + 排名因子 + 空隙奖励
            meanFactor := (1 - level.Volume/stats.Mean) × 40
            rankFactor := level.VolumeRank / float64(len(levels)) × 40

            level.LVNStrength = meanFactor + rankFactor

            if level.NodeType == VolumeNodeGap {
                level.LVNStrength += 20  // 空隙额外加分
            }

            level.LVNStrength = min(level.LVNStrength, 100)
        }
    }
}
```

**步骤7: 分类节点类型**
```go
func (va *VPVRAnalyzer) classifyNodeTypes(levels []*PriceLevel) {
    // 寻找HVN聚集区 (>=3个连续HVN)
    consecutiveHVN := 0
    for _, level := range levels {
        if level.IsHVN {
            consecutiveHVN++
            if consecutiveHVN >= 3 {
                level.NodeType = VolumeNodeCluster
            }
        } else {
            consecutiveHVN = 0
        }
    }

    // 普通节点标记
    for _, level := range levels {
        if !level.IsHVN && !level.IsLVN {
            level.NodeType = VolumeNodeNormal
        }
    }
}
```

**步骤8: 更新统计信息**
```go
func (va *VPVRAnalyzer) updateVolumeNodeStats(levels []*PriceLevel, stats *VolumeStats) {
    stats.HVNCount = 0
    stats.LVNCount = 0
    stats.HVNTotalVolume = 0
    stats.LVNTotalVolume = 0

    for _, level := range levels {
        if level.IsHVN {
            stats.HVNCount++
            stats.HVNTotalVolume += level.Volume
        }
        if level.IsLVN {
            stats.LVNCount++
            stats.LVNTotalVolume += level.Volume
        }
    }

    stats.HVNVolumePercent = stats.HVNTotalVolume / stats.TotalVolume × 100
    stats.LVNVolumePercent = stats.LVNTotalVolume / stats.TotalVolume × 100

    // 成交量集中度评分
    stats.VolumeConcentration = va.calculateVolumeConcentration(levels)
}
```

---

### 3.5 市场廓型分类

#### classifyMarketProfile - 识别6种市场状态

**算法**:
```go
func (va *VPVRAnalyzer) classifyMarketProfile(vp *VolumeProfile) (MarketProfileType, float64, float64, float64) {
    // 计算关键指标
    vaWidthRatio := vp.Stats.ValueAreaWidthRatio
    pocDensity := vp.Stats.POCDensity
    pocStrength := vp.Stats.POCStrength
    volumeDispersion := va.calculateVolumeDispersion(vp.Levels)
    priceSkewness := va.calculatePriceSkewness(vp.Levels)

    // 分类决策树
    if vaWidthRatio < 0.2 && pocDensity > 0.3 && pocStrength > 70 {
        // 窄VA + 高POC密度 + 强POC + 明显偏斜
        return MarketProfileTrending, 80, 10, 10

    } else if vaWidthRatio >= 0.2 && vaWidthRatio <= 0.4 && pocStrength >= 50 {
        // 中等VA + 中等POC强度 + 低偏斜
        return MarketProfileConsolidation, 20, 70, 10

    } else if vaWidthRatio > 0.5 && volumeDispersion > 0.6 {
        // 宽VA + 高成交量分散 + 无明确POC
        return MarketProfileVolatile, 40, 30, 30

    } else if vaWidthRatio >= 0.3 && vaWidthRatio <= 0.5 && pocDensity > 0.25 {
        // 中等VA + 集中成交量 + 高POC密度
        return MarketProfileBalance, 10, 80, 10

    } else if vaWidthRatio < 0.15 && pocStrength > 80 {
        // 极窄VA + 极强POC + POC转移
        return MarketProfileBreakout, 90, 5, 5

    } else if vaWidthRatio > 0.4 && abs(priceSkewness) < 0.3 {
        // 宽VA + 无明显偏斜 + POC变化
        return MarketProfileRotation, 30, 60, 10

    } else {
        return MarketProfileUnknown, 0, 0, 0
    }
}
```

**分类指标**:
| 类型 | 特征 | 识别条件 |
|------|------|--------|
| **Trending** | 趋势型 | 窄VA + 高POC密度 + 强POC + 明显偏斜 |
| **Consolidation** | 整理型 | 中等VA + 中等POC强度 + 低偏斜 |
| **Volatile** | 波动型 | 宽VA + 高成交量分散 + 无明确POC |
| **Balance** | 平衡型 | 中等VA + 集中成交量 + 高POC密度 |
| **Breakout** | 突破型 | 极窄VA + 极强POC + POC转移 |
| **Rotation** | 轮动型 | 宽VA + 无明显偏斜 + POC变化 |

---

### 3.6 买卖比例算法

#### calculateEnhancedBuySellRatio - 多因子权重模型

**算法**:
```go
func (va *VPVRAnalyzer) calculateEnhancedBuySellRatio(kline Kline) float64 {
    // 1. movementBias (30%权重)
    priceMovement := (kline.Close - kline.Open) / kline.Open
    priceRange := kline.High - kline.Low
    movementBias := 0.3 × (kline.Close - kline.Open) / priceRange

    // 2. positionBias (20%权重)
    positionInRange := (kline.Close - kline.Low) / priceRange
    positionBias := 0.2 × (positionInRange - 0.5)

    // 3. volumeStrength (10%权重)
    volumeRatio := kline.Volume / avgVolume
    volumeStrength := 0.1 × min(volumeRatio - 1, 1.0) × sign(priceMovement)

    // 4. amplitudeBias (10%权重)
    amplitudeRatio := priceRange / avgRange
    amplitudeBias := 0.1 × min(amplitudeRatio - 1, 1.0) × sign(priceMovement)

    // 综合计算
    buyRatio := 0.5 + movementBias + positionBias + volumeStrength + amplitudeBias

    // 确保范围 [5%, 95%]
    buyRatio = max(0.05, min(buyRatio, 0.95))

    return buyRatio
}
```

**真实数据优先级**:
1. 直接BuyVolume/SellVolume (精确)
2. TakerBuyVolume (币安标准)
3. TakerBuyBaseVolume (币安扩展)
4. BuyerMakerVolume/SellerMakerVolume (Maker/Taker分类)
5. 降级: 算法估算

---

## 四、稳定TickSize计算 (Task 7)

### 4.1 三层融合策略

**目标**: 解决动态TickSize导致的数据抖动

**算法**:
```go
func (va *VPVRAnalyzer) calculateStableTickSize(klines, minPrice, maxPrice) float64 {
    priceRange := maxPrice - minPrice

    // 1. 自适应TickSize
    adaptiveTickSize := va.calculateAdaptiveTickSize(klines, priceRange)
    adaptiveScore := va.scoreTickSize(adaptiveTickSize, priceRange, len(klines))

    // 2. ATR基础TickSize
    atrTickSize := va.calculateATRBasedTickSize(klines, priceRange)
    atrScore := va.scoreTickSize(atrTickSize, priceRange, len(klines))

    // 3. 一致性TickSize
    consistentTickSize := va.calculateConsistentTickSize(priceRange)
    consistentScore := va.scoreTickSize(consistentTickSize, priceRange, len(klines))

    // 4. 评分并选择最优
    bestTickSize := adaptiveTickSize
    bestScore := adaptiveScore

    if atrScore > bestScore {
        bestTickSize = atrTickSize
        bestScore = atrScore
    }

    if consistentScore > bestScore {
        bestTickSize = consistentTickSize
        bestScore = consistentScore
    }

    return bestTickSize
}
```

---

#### calculateAdaptiveTickSize - 自适应TickSize

```go
func (va *VPVRAnalyzer) calculateAdaptiveTickSize(klines, priceRange) float64 {
    numKlines := len(klines)
    targetLevels := 0

    // K线数<50: 目标30级别
    // K线数<100: 目标50级别
    // K线数<200: 目标80级别
    // K线数>=200: 目标120级别

    if numKlines < 50 {
        targetLevels = 30
    } else if numKlines < 100 {
        targetLevels = 50
    } else if numKlines < 200 {
        targetLevels = 80
    } else {
        targetLevels = 120
    }

    return priceRange / float64(targetLevels)
}
```

---

#### calculateATRBasedTickSize - ATR基础TickSize

```go
func (va *VPVRAnalyzer) calculateATRBasedTickSize(klines, priceRange) float64 {
    atr := calculateATR(klines, 14)

    tickSize := atr × 0.4

    // 限制范围: [PriceRange/300, PriceRange/20]
    minTickSize := priceRange / 300
    maxTickSize := priceRange / 20

    tickSize = max(tickSize, minTickSize)
    tickSize = min(tickSize, maxTickSize)

    return tickSize
}
```

---

#### calculateConsistentTickSize - 一致性TickSize

```go
func (va *VPVRAnalyzer) calculateConsistentTickSize(priceRange) float64 {
    // 基于价格范围的标准化精度
    if priceRange < 0.1 {
        return 0.0001
    } else if priceRange < 1.0 {
        return 0.001
    } else if priceRange < 10.0 {
        return 0.01
    } else if priceRange < 100.0 {
        return 0.1
    } else if priceRange < 1000.0 {
        return 1.0
    } else if priceRange < 10000.0 {
        return 10.0
    } else {
        return 100.0
    }
}
```

---

#### scoreTickSize - TickSize评分

```go
func (va *VPVRAnalyzer) scoreTickSize(tickSize, priceRange, numKlines) float64 {
    numLevels := priceRange / tickSize

    // 评分因子:
    // 1. 级别数量合理性 (30-150最优) - 40%权重
    levelScore := 0.0
    if numLevels >= 30 && numLevels <= 150 {
        levelScore = 40.0
    } else if numLevels >= 20 && numLevels <= 200 {
        levelScore = 30.0
    } else if numLevels >= 10 && numLevels <= 250 {
        levelScore = 20.0
    } else {
        levelScore = 10.0
    }

    // 2. 精度合理性 (不过细/过粗) - 35%权重
    precisionScore := 35.0
    if tickSize < priceRange / 500 {
        precisionScore = 15.0  // 过细
    } else if tickSize > priceRange / 10 {
        precisionScore = 15.0  // 过粗
    }

    // 3. 一致性 (与默认配置偏差) - 25%权重
    defaultTickSize := priceRange / 100
    deviation := abs(tickSize - defaultTickSize) / defaultTickSize
    consistencyScore := max(0, 25.0 - deviation × 25.0)

    return levelScore + precisionScore + consistencyScore
}
```

---

## 五、自适应阈值机制 (Task 8)

### 5.1 AdaptiveThresholds结构

```go
type AdaptiveThresholds struct {
    // POC阈值
    POCDistanceThreshold   float64  // 1% (动态调整)
    POCStrengthMultiplier  float64  // 2.0倍

    // 价值区域阈值
    VABreakoutThreshold    float64  // 0.5%
    VAReturnSensitivity    float64  // 1.0

    // 成交量阈值
    HighVolumeMultiplier   float64  // 2.0倍
    LowVolumeMultiplier    float64  // 0.3倍

    // 买卖不平衡阈值
    ImbalanceThreshold     float64  // 1.5倍
    BuySellRatioThreshold  float64  // 1.2倍

    // 环境修正因子
    VolatilityAdjustment   float64
    TrendStrengthFactor    float64
    ConcentrationFactor    float64
}
```

---

### 5.2 动态调整规则

**1. 基于波动性调整**
```go
func (at *AdaptiveThresholds) adjustForMarketVolatility(volatility float64) {
    if volatility > 0.15 {
        // 高波动 (>15%): 放宽50%
        at.POCDistanceThreshold *= 1.5
        at.VABreakoutThreshold *= 1.5
    } else if volatility > 0.08 {
        // 中等波动 (8-15%): 放宽20%
        at.POCDistanceThreshold *= 1.2
        at.VABreakoutThreshold *= 1.2
    } else if volatility < 0.03 {
        // 低波动 (<3%): 收紧20%
        at.POCDistanceThreshold *= 0.8
        at.VABreakoutThreshold *= 0.8
    }
}
```

**2. 基于集中度调整**
```go
func (at *AdaptiveThresholds) adjustForVolumeConcentration(concentration float64) {
    if concentration > 2.0 {
        // 高度集中 (>2.0): 收紧20% (更可靠)
        at.HighVolumeMultiplier *= 0.8
    } else if concentration < 0.8 {
        // 分散市场 (<0.8): 放宽30% (避免噪声)
        at.HighVolumeMultiplier *= 1.3
    }
}
```

**3. 基于趋势强度调整**
```go
func (at *AdaptiveThresholds) adjustForTrendStrength(trendStrength float64) {
    if trendStrength > 70 {
        // 强趋势 (>70%): 突破阈值收紧
        at.VABreakoutThreshold *= 0.8
    } else if trendStrength < 30 {
        // 弱趋势 (<30%): 突破阈值放宽
        at.VABreakoutThreshold *= 1.2
    }
}
```

**4. 基于HVN/LVN密度调整**
```go
func (at *AdaptiveThresholds) adjustForHVNLVNContext(hvnPercent, lvnPercent float64) {
    if hvnPercent > 0.15 {
        // HVN密度高 (>15%): 降低成交量信号阈值
        at.HighVolumeMultiplier *= 0.9
    }

    if lvnPercent > 0.20 {
        // LVN密度高 (>20%): 提高成交量信号阈值
        at.LowVolumeMultiplier *= 1.1
    }
}
```

---

## 六、交易信号生成

### 6.1 五种信号类型

**1. POC测试信号**
```go
func generatePOCSignalAdaptive(vp *VolumeProfile, currentPrice, thresholds) *Signal {
    distance := abs(currentPrice - vp.POC.Price) / currentPrice

    if distance < thresholds.POCDistanceThreshold {
        // 价格接近POC
        buySellRatio := vp.POC.BuyVolume / vp.POC.SellVolume

        if buySellRatio > 1.2 {
            return &Signal{Type: "POC_BUY", Strength: 70}
        } else if buySellRatio < 0.83 {
            return &Signal{Type: "POC_SELL", Strength: 70}
        } else {
            return &Signal{Type: "POC_HOLD", Strength: 50}
        }
    }

    return nil
}
```

**2. 价值区域突破信号**
```go
func generateValueAreaSignalAdaptive(vp *VolumeProfile, currentPrice, thresholds) *Signal {
    vah := vp.VAH
    val := vp.VAL

    if currentPrice > vah × (1 + thresholds.VABreakoutThreshold) {
        return &Signal{Type: "VA_BREAKOUT_UP", Strength: 80}
    } else if currentPrice < val × (1 - thresholds.VABreakoutThreshold) {
        return &Signal{Type: "VA_BREAKOUT_DOWN", Strength: 80}
    } else if currentPrice > val && currentPrice < vah {
        return &Signal{Type: "VA_MEAN_REVERSION", Strength: 60}
    }

    return nil
}
```

**3. 高/低成交量信号**
```go
func generateVolumeSignalAdaptive(currentVolume, avgVolume, thresholds) *Signal {
    volumeRatio := currentVolume / avgVolume

    if volumeRatio > thresholds.HighVolumeMultiplier {
        return &Signal{Type: "HIGH_VOLUME", Strength: 75}
    } else if volumeRatio < thresholds.LowVolumeMultiplier {
        return &Signal{Type: "LOW_VOLUME", Strength: 50}
    }

    return nil
}
```

**4. 买卖不平衡信号**
```go
func generateImbalanceSignalAdaptive(buySellRatio, thresholds) *Signal {
    if buySellRatio > thresholds.ImbalanceThreshold {
        return &Signal{Type: "BUY_IMBALANCE", Strength: 70}
    } else if buySellRatio < (1 / thresholds.ImbalanceThreshold) {
        return &Signal{Type: "SELL_IMBALANCE", Strength: 70}
    }

    return nil
}
```

**5. HVN/LVN节点信号**
```go
func generateHVNLVNSignal(vp *VolumeProfile, currentPrice) *Signal {
    for _, level := range vp.Levels {
        distance := abs(currentPrice - level.Price) / currentPrice

        if distance < 0.01 {  // 1%范围内
            if level.IsHVN && level.HVNStrength > 70 {
                return &Signal{Type: "HVN_SUPPORT", Strength: 75}
            }

            if level.IsLVN && level.NodeType == VolumeNodeGap {
                return &Signal{Type: "LVN_GAP", Strength: 65}
            }
        }
    }

    return nil
}
```

---

## 七、使用示例

### 7.1 基本分析

```go
// 创建分析器（推荐方式：使用动态配置）
analyzer := NewVPVRAnalyzerWithDynamicConfig(exchangeMeta, "4h")

// 分析K线数据
volumeProfile := analyzer.Analyze(klines)

if volumeProfile != nil {
    // POC信息
    log.Printf("POC: %.2f, 成交量: %.0f",
        volumeProfile.POC.Price,
        volumeProfile.POC.Volume)

    // 价值区域
    log.Printf("VAH: %.2f, VAL: %.2f, 占比: %.1f%%",
        volumeProfile.VAH,
        volumeProfile.VAL,
        volumeProfile.ValueArea.VolumePercent)

    // 市场廓型
    log.Printf("市场廓型: %s, 强度: %.1f%%",
        volumeProfile.Stats.MarketProfile,
        volumeProfile.Stats.MarketRegimeStrength)

    // HVN/LVN统计
    log.Printf("HVN: %d个 (%.1f%%), LVN: %d个 (%.1f%%)",
        volumeProfile.Stats.HVNCount,
        volumeProfile.Stats.HVNVolumePercent,
        volumeProfile.Stats.LVNCount,
        volumeProfile.Stats.LVNVolumePercent)

    // 生成交易信号
    signals := analyzer.GenerateSignals(volumeProfile, currentPrice)
    for _, signal := range signals {
        log.Printf("信号: %s, 强度: %.1f, 置信度: %.1f%%",
            signal.Type,
            signal.Strength,
            signal.Confidence)
    }
}
```

### 7.2 自定义配置

```go
config := &VPVRConfig{
    TickSize: 0.0,  // 0表示自动计算
    VAPercent: 0.7,  // 价值区域70%
    EnableATRWidthStandards: true,
    UseRealBuySellData: true,
}

analyzer := &VPVRAnalyzer{config: config}
result := analyzer.Analyze(klines)
```

---

## 八、关键考虑事项

1. **最小数据要求**: 至少50根K线（建议200+）
2. **真实数据优先**: 优先使用交易所买卖量数据
3. **配置动态化**: 根据symbol和timeframe动态调整参数
4. **自适应阈值**: 根据市场波动性实时调整信号参数
5. **节点识别**: 综合使用阈值、局部极值、相对强度等多重条件
6. **市场识别**: 6种市场廓型覆盖从极趋势到极波动的全状态

---

## 九、P0级修复记录

| 修复 | 描述 | 影响 |
|------|------|------|
| **P0-04** | 动态VPVR配置替代硬编码 | 解决不同币种使用相同TickSize的扭曲 |
| **P0级** | VPVR添加Context计算 | 解决StrengthZ/VolRatio为null的问题 |
| **Task 5** | 价值区宽度占比和市场廓型分析 | 新增6种市场状态识别 |
| **Task 6** | HVN/LVN节点检测系统 | 新增8步骤节点识别流程 |
| **Task 7** | 稳定TickSize计算 | 解决动态调整导致的数据抖动 |
| **Task 8** | 自适应阈值机制 | 根据市场条件动态调整信号阈值 |

---

## 十、总结

VPVR系统构成了一个**完整的市场微观结构分析框架**，特点包括：

✅ **精确的成交量分布**: 多级别插值分配，OHLC权重优化
✅ **智能POC识别**: 线性扫描找最大成交量点
✅ **动态价值区**: 从POC向两侧扩展，包含70%成交量
✅ **HVN/LVN系统**: 8步骤识别高低成交量节点
✅ **市场廓型分类**: 6种状态覆盖全市场环境
✅ **稳定TickSize**: 三层融合策略，解决数据抖动
✅ **自适应阈值**: 根据波动率、集中度、趋势强度动态调整

为交易策略提供了丰富的价量关系信息，是AI决策的重要依据。

---

**文档版本**: 1.0
**最后更新**: 2026-01-18
**维护者**: AI自动交易系统开发团队
