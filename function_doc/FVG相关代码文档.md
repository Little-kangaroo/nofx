# FVG相关代码完整文档

## 概述

本文档详细记录了AI自动交易系统中FVG (Fair Value Gap，公允价值缺口) 相关的逻辑代码，包括识别算法、质量评估、生命周期管理、上下文评分等核心算法。FVG是ICT交易理论的核心概念之一，用于识别市场中的流动性不平衡区域，为AI提供关键的支撑/阻力位判断依据。

---

## 一、核心文件列表

### 1.1 主要FVG文件

| 文件路径 | 行数 | 主要功能 |
|---------|------|---------|
| `market/fvg.go` | 1518 | FVG识别引擎，生命周期管理，信号生成 |
| `market/fvg_context.go` | 147 | FVG上下文评分计算，样本偏差修复 |
| `market/types.go` | 900+ | FVG数据结构定义 |
| `market/data_cleaner.go` | 600+ | P2级FVG数据清洗 |
| `market/context_calculator.go` | 350+ | 标准分和ATR计算工具 |

### 1.2 集成文件

- `market/comprehensive_analysis.go` - 多时间框架综合分析
- `market/anchor_adapters.go` - 锚点转换适配器
- `market/context_scoring.go` - 上下文评分计算

---

## 二、数据结构定义

### 2.1 核心数据结构

#### FVGData - FVG分析主输出结构
```go
type FVGData struct {
    BullishFVGs  []*FairValueGap  // 看涨FVG列表
    BearishFVGs  []*FairValueGap  // 看跌FVG列表
    ActiveFVGs   []*FairValueGap  // 活跃FVG列表（经过筛选）
    Config       *FVGConfig       // 配置参数
    Statistics   *FVGStatistics   // 统计信息
    LastAnalysis int64            // 最后分析时间（毫秒）
}
```

**用途**: 包含完整的FVG分析结果，传递给AI进行决策

---

#### FairValueGap - FVG实体结构
```go
type FairValueGap struct {
    // 基本信息
    ID           string       // 唯一标识
    Type         FVGType      // "bullish" 或 "bearish"
    UpperBound   float64      // 上边界价格
    LowerBound   float64      // 下边界价格
    CenterPrice  float64      // 中心价格
    Width        float64      // 宽度（绝对值）
    WidthPercent float64      // 宽度百分比
    FormationATR float64      // 形成时ATR（避免时空错配）
    WidthATR     float64      // 宽度相对ATR倍数

    // 起源信息
    Origin       *FVGOrigin   // 形成细节

    // 状态信息
    Status       FVGStatus    // "fresh"/"tested"/"partial_fill"/"filled"/"inversion"
    CreationTime int64        // 创建时间（毫秒）
    IsActive     bool         // 是否活跃
    IsFilled     bool         // 是否已填补
    IsPartialFill bool        // 是否部分填补
    FillProgress float64      // 填补进度 (0-100%)
    FillTime     int64        // 填补时间

    // Inversion FVG特性（新增）
    IsInversion    bool       // 是否为反转FVG
    InversionTime  int64      // 转化为反转FVG的时间

    // 触及统计
    TouchCount   int          // 触及次数
    LastTouch    int64        // 最后触及时间

    // 质量评估
    Strength     float64      // 强度 (0-100)
    Quality      FVGQuality   // "high"/"medium"/"low"

    // 成交量上下文
    VolumeContext *FVGVolume  // 成交量确认信息

    // 验证信息
    Validation   *FVGValidation // 验证结果

    // 上下文评分（新增）
    Context      *ContextMetrics // 上下文度量指标
    Score        float64         // 综合评分（质量40% + 距离40% + 新鲜度20%）
}
```

**识别条件**:
- **看涨FVG**: 第三根K线的低点 > 第一根K线的高点（存在向上缺口）
- **看跌FVG**: 第三根K线的高点 < 第一根K线的低点（存在向下缺口）

---

#### FVGOrigin - FVG起源信息
```go
type FVGOrigin struct {
    KlineIndex     int           // K线索引
    PreviousCandle *CandleInfo   // 第一根K线（index-2）
    CurrentCandle  *CandleInfo   // 中间K线（index-1，形成缺口的关键K线）
    NextCandle     *CandleInfo   // 第三根K线（index）
    ImpulsiveMove  float64       // 冲击强度（百分比）
    TimeFrame      string        // 时间框架
    FormationType  FormationType // 形成类型
}
```

**FormationType枚举**:
```go
const (
    FormationContinuation = "continuation" // 延续形成
    FormationReversal     = "reversal"     // 反转形成
    FormationBreakout     = "breakout"     // 突破形成
    FormationPullback     = "pullback"     // 回调形成
)
```

**ImpulsiveMove计算**:
```go
// 看涨FVG: 中间K线爆发力70% + 总缺口距离30%
ImpulsiveMove = middleBodyMove × 0.7 + totalGapMove × 0.3

// 看跌FVG: 类似，但计算跌幅
```

---

#### FVGVolume - 成交量上下文
```go
type FVGVolume struct {
    FormationVolume    float64   // 形成时成交量
    AverageVolume      float64   // 平均成交量（排除爆发K线）
    VolumeRatio        float64   // 成交量比率
    TouchVolumes       []float64 // 触及时成交量列表
    VolumeConfirmation bool      // 是否成交量确认
}
```

**成交量确认逻辑** (修复幸存者偏差):
```go
// 🔥 排除当前爆发K线，避免幸存者偏差
avgVolume := calculateAverageVolume(klines, index-basePeriod-1, index-1)
volumeConfirmed := formationVolume >= avgVolume × MinVolumeRatio
```

---

#### ContextMetrics - 上下文度量指标
```go
type ContextMetrics struct {
    StrengthZ float64 // 强度标准分 (Z-Score, >1.5为强)
    WidthATR  float64 // 宽度相对ATR倍数
    VolRatio  float64 // 成交量比率 (>2.0为异常放量)
    IsFresh   bool    // 是否新鲜
    TimeScore float64 // 时间评分 (0-100)
    RankPct   float64 // 强度排名百分位 (0.8+为前20%)
}
```

**用途**: 提供跨资产、跨时间框架的标准化评估指标

---

#### FVGConfig - 配置结构
```go
type FVGConfig struct {
    // 识别配置
    MinGapPercent    float64  // 最小缺口百分比 (0.001 = 0.1%)
    UseBodyGap       bool     // 是否使用Body-to-Body模式

    // 成交量配置
    RequireVolConf   bool     // 是否要求成交量确认
    MinVolumeRatio   float64  // 最小成交量比率 (1.5 = 1.5倍)
    VolumeBasePeriod int      // 成交量基准周期 (20)

    // 过滤配置
    MaxAge           int      // 最大年龄（根K线数）
    MaxTouchCount    int      // 最大触及次数
    FillThreshold    float64  // 填补阈值 (0.95 = 95%)
    MaxDistanceATR   float64  // 最大距离（ATR倍数）

    // 验证配置
    EnableValidation bool     // 是否启用验证

    // 时间框架
    TimeFrames       []string // 支持的时间框架
}
```

**默认配置**:
```go
defaultFVGConfig = FVGConfig{
    MinGapPercent:    0.001,  // 0.1%
    UseBodyGap:       false,  // 使用传统ICT模式
    RequireVolConf:   false,  // 可选成交量确认
    MinVolumeRatio:   1.5,    // 1.5倍成交量
    VolumeBasePeriod: 20,     // 20根K线平均
    MaxAge:           50,     // 最多50根K线
    MaxTouchCount:    5,      // 最多触及5次
    FillThreshold:    0.95,   // 95%填补
    MaxDistanceATR:   5.0,    // 5倍ATR
    EnableValidation: true,   // 启用验证
    TimeFrames:       []string{"5m", "15m", "1h", "4h"},
}
```

---

### 2.2 状态枚举

#### FVGStatus - FVG状态
```go
const (
    FVGStatusFresh       = "fresh"        // 新鲜（未触及）
    FVGStatusTested      = "tested"       // 已测试（触及但未填补）
    FVGStatusPartialFill = "partial_fill" // 部分填补（20%-95%）
    FVGStatusFilled      = "filled"       // 完全填补（≥95%）
    FVGStatusInversion   = "inversion"    // 反转FVG（Failed转化）
    FVGStatusExpired     = "expired"      // 已过期
)
```

**状态转换逻辑**:
```
Fresh → Tested → PartialFill → Filled → Inversion
                                         ↑
                                    (满足条件)
```

#### FVGQuality - FVG质量
```go
const (
    FVQualityHigh   = "high"   // 高质量（≥80分）
    FVQualityMedium = "medium" // 中等质量（60-79分）
    FVQualityLow    = "low"    // 低质量（<60分）
)
```

---

## 三、核心算法详解

### 3.1 FVG识别算法

#### identifyBullishFVG - 看涨FVG识别

**位置**: `market/fvg.go:146-272`

**标准三根K线模式**:
```go
// [index-2, index-1, index]
firstCandle := klines[index-2]  // 第一根K线
middleCandle := klines[index-1] // 中间K线（产生缺口的关键K线）
currentCandle := klines[index]  // 当前K线（第三根）
```

**识别逻辑**:
```go
// 1. 根据配置选择边界计算方式
if UseBodyGap {
    // Body-to-Body模式（减少影线干扰，高波动币种推荐）
    firstBound = max(firstCandle.Open, firstCandle.Close)     // 第一根实体上沿
    currentBound = min(currentCandle.Open, currentCandle.Close) // 当前实体下沿
} else {
    // 传统ICT模式（标准定义）
    firstBound = firstCandle.High
    currentBound = currentCandle.Low
}

// 2. 看涨FVG条件：当前边界 > 第一根边界
if currentBound <= firstBound {
    return nil  // 不满足
}

// 3. 计算缺口边界
gapHigh = currentCandle.Low (或实体下沿)
gapLow = firstCandle.High (或实体上沿)
gapWidth = gapHigh - gapLow
gapWidthPercent = gapWidth / gapLow × 100
```

**ATR计算** (避免时空错配):
```go
// 🔥 修复：计算形成时ATR，而非当前ATR
atrPeriod := 14
atrLookback := 15

if index >= atrLookback {
    formationATR = calculateATR(klines[index-atrLookback:index], atrPeriod)
} else if index >= atrPeriod {
    // 降级：使用可用数据
    formationATR = calculateATR(klines[:index], atrPeriod)
} else {
    // 数据不足：简化计算
    formationATR = 简单波动率近似值
}

widthATR = gapWidth / formationATR
```

**成交量确认** (修复幸存者偏差):
```go
if RequireVolConf {
    // 🔥 排除当前爆发K线（index-1），避免幸存者偏差
    avgVolume = calculateAverageVolume(klines, index-basePeriod-1, index-1)
    volumeConfirmed = middleCandle.Volume >= avgVolume × MinVolumeRatio
} else {
    volumeConfirmed = true  // 不要求成交量确认
}
```

**时间框架智能过滤**:
```go
// shouldFilterByTimeframe() - fvg.go:1449
timeframe := inferTimeframe(klines)

if timeframe == "4h" || timeframe == "1h" {
    // HTF: 丢弃 width_percent < 0.3% 的微型FVG
    if widthPercent < 0.3 {
        return nil
    }
}

if timeframe == "15m" || timeframe == "5m" {
    // LTF: 丢弃 width_atr < 0.5 的微型FVG
    if widthATR < 0.5 {
        return nil
    }
}
```

---

#### identifyBearishFVG - 看跌FVG识别

**位置**: `market/fvg.go:275-401`

**识别条件**:
```go
// 看跌FVG条件：当前K线边界 < 第一根K线边界
if currentBound >= firstBound {
    return nil
}

// 缺口边界
gapHigh = firstCandle.Low (或实体下沿)
gapLow = currentCandle.High (或实体上沿)
```

其他逻辑与看涨FVG类似。

---

### 3.2 FVG生命周期管理

#### updateFVGStatuses - 状态更新引擎

**位置**: `market/fvg.go:540-596`

**处理流程**:
```
1. 检查年龄 → 过期处理
2. 计算填补进度 → 状态转换
3. Inversion转化判断
4. 统计触及次数
5. 更新LastTouch时间
```

**填补进度计算** (calculateFillProgress):
```go
// fvg.go:637-673
maxPenetration := 0.0

for i := startIndex + 1; i < len(klines); i++ {
    kline := klines[i]

    if gap.Type == BullishFVG {
        // 看涨FVG：检查价格向下填补到LowerBound的程度
        if kline.Low <= gap.LowerBound {
            penetration = gap.LowerBound - kline.Low
            maxPenetration = max(maxPenetration, penetration)
        }
    } else {
        // 看跌FVG：检查价格向上填补到UpperBound的程度
        if kline.High >= gap.UpperBound {
            penetration = kline.High - gap.UpperBound
            maxPenetration = max(maxPenetration, penetration)
        }
    }
}

fillProgress = (maxPenetration / gapWidth) × 100
```

**状态判断**:
```go
if fillProgress >= FillThreshold × 100 {  // ≥95%
    status = FVGStatusFilled
    isFilled = true
    isActive = false

    // 🔥 新增：Inversion FVG转化
    if shouldConvertToInversion(gap, klines) {
        isInversion = true
        inversionTime = currentTime
        isActive = true  // 重新激活为反转FVG
        status = FVGStatusInversion
    }
} else if fillProgress > 20 {  // 20%-95%
    status = FVGStatusPartialFill
    isPartialFill = true
}
```

---

#### shouldConvertToInversion - Inversion FVG判断

**位置**: `market/fvg.go:599-628`

**转化条件** (5个条件全部满足):
```go
// 1. 强度足够高（高强度FVG失败后更容易成为反向支撑/阻力）
if gap.Strength < 3.0 {
    return false
}

// 2. 形成时有较强的冲击性移动
if gap.Origin.ImpulsiveMove < 0.02 {  // <2%
    return false
}

// 3. 成交量验证：形成时有足够成交量支撑
if gap.VolumeContext.VolumeRatio < 1.5 {
    return false
}

// 4. 不是太老的FVG（超过30根K线的FVG反转意义有限）
if age > 30 {
    return false
}

// 5. 宽度适中（太小的FVG反转效果有限，太大的可能信号过强）
if gap.WidthATR < 0.5 || gap.WidthATR > 3.0 {
    return false
}

return true  // 可以转化为Inversion FVG
```

**Inversion FVG概念**:
- Failed FVG（被完全填补的FVG）往往变成反向的支撑/阻力位
- 也称为Mitigation Block
- 作用：原看涨FVG被填补后，可能成为阻力位；反之亦然

---

### 3.3 FVG强度与质量评估

#### calculateFVGStrength - 强度计算

**位置**: `market/fvg.go:773-818`

**评分公式**:
```go
strength := 0.0

// 1. 基于缺口宽度的强度（最多30分）
strength += min(widthPercent × 20, 30)

// 2. 基于冲动移动的强度（最多25分）
if origin != nil {
    impulsiveMove := abs(origin.ImpulsiveMove)
    strength += min(impulsiveMove × 10, 25)
}

// 3. 基于成交量的强度（最多20分）
if volumeContext != nil && volumeRatio > 1 {
    volumeBonus := min((volumeRatio - 1) × 15, 20)
    strength += volumeBonus
}

// 4. 基于形成类型的强度
switch formationType {
    case FormationBreakout:
        strength += 15
    case FormationReversal:
        strength += 12
    case FormationContinuation:
        strength += 8
    case FormationPullback:
        strength += 5
}

// 5. 基于未填补时间的强度加成（最多10分）
if age > 10 && !isFilled {
    ageBonus := min((age - 10) × 0.5, 10)
    strength += ageBonus
}

// 6. 触及次数惩罚
touchPenalty := touchCount × 3
strength = max(strength - touchPenalty, 0)

gap.Strength = min(strength, 100)
```

---

#### assessFVGQuality - 质量评估

**位置**: `market/fvg.go:820-853`

**评估逻辑**:
```go
score := gap.Strength

// 1. 基于填补进度调整
if fillProgress > 50 {
    score *= 0.7  // 填补超过50%降低质量
} else if fillProgress < 20 {
    score += 10   // 填补很少加分
}

// 2. 基于成交量确认调整
if volumeContext.VolumeConfirmation {
    score += 5
}

// 3. 基于形成类型调整
switch formationType {
    case FormationBreakout, FormationReversal:
        score += 8
    case FormationContinuation:
        score += 5
}

// 4. 质量评级
if score >= 80 {
    quality = FVQualityHigh
} else if score >= 60 {
    quality = FVQualityMedium
} else {
    quality = FVQualityLow
}
```

---

### 3.4 活跃FVG筛选策略

#### filterActiveFVGsWithContext - "先截断后打分"策略

**位置**: `market/fvg.go:720-771`

**三阶段筛选**:

**阶段1: 硬截断** (基础过滤)
```go
for _, gap := range gaps {
    // 1. 基础有效性检查
    if !gap.IsActive || gap.IsFilled {
        continue
    }

    // 2. 完全回补检查
    if gap.FillProgress >= 95.0 {
        continue  // 几乎完全回补，直接过滤
    }

    // 3. 距离硬截断（5 ATR）
    if atr > 0 {
        distanceATR := calculateDistanceInATR(gap, currentPrice, atr)
        if distanceATR > MaxDistanceATR {  // 默认5.0
            continue  // 距离太远，直接过滤
        }
    }

    candidates = append(candidates, gap)
}
```

**阶段2: 综合评分排序**
```go
for i := range candidates {
    candidates[i].Score = calculateFVGScore(candidates[i], currentPrice, atr, klines)
}

sort.Slice(candidates, func(i, j int) bool {
    return candidates[i].Score > candidates[j].Score
})
```

**阶段3: 时间框架独立Top 3限制**
```go
timeframe := inferTimeframe(klines)
maxFVGs := getMaxFVGsForTimeframe(timeframe)  // 各时间框架均为3个

if len(candidates) > maxFVGs {
    candidates = candidates[:maxFVGs]
}
```

**评分公式** (calculateFVGScore):
```go
// fvg.go:1280-1305
score := 0.0

// 1. 质量权重 40%
qualityScore := gap.Strength / 100.0 × 40.0

// 2. 距离权重 40% - 越近分数越高
distanceATR := calculateDistanceInATR(gap, currentPrice, atr)
distanceScore := (1.0 - distanceATR / MaxDistanceATR) × 40.0

// 3. 新鲜度权重 20% - 越新越好
age := calculateAge(gap, klines)
freshnessScore := (1.0 - age / MaxAge) × 20.0

score = qualityScore + distanceScore + freshnessScore
```

---

### 3.5 上下文评分计算

#### CalculateContextScores - 上下文评分引擎

**位置**: `market/fvg_context.go:16-57`

**核心修复**: 解决"矮子里拔将军"问题

**样本池扩展逻辑**:
```go
// 收集当前FVG样本
var allStrengths []float64
var allWidths []float64
var allVolumes []float64

for _, gap := range allFVGs {
    allStrengths = append(allStrengths, gap.Strength)
    allWidths = append(allWidths, gap.Width)
    allVolumes = append(allVolumes, gap.VolumeContext.FormationVolume)
}

// 🔥 修复：如果样本数量过少（<20），使用历史基准扩展
if len(allStrengths) < 20 {
    expandedStrengths := getHistoricalStrengthBaseline()
    expandedWidths := getHistoricalWidthBaseline()

    allStrengths = append(allStrengths, expandedStrengths...)
    allWidths = append(allWidths, expandedWidths...)
}
```

**历史基准数据** (经验值):
```go
// getHistoricalStrengthBaseline() - fvg_context.go:110
强度基准 = [
    0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2,  // 弱强度（低波动期）
    1.5, 1.6, 1.8, 2.0, 2.2, 2.5, 2.8, 3.0,  // 中等强度
    3.5, 4.0, 4.5, 5.0, 6.0, 7.0, 8.0,       // 高强度
    10.0, 12.0, 15.0, 20.0,                  // 极强（极端市场）
]

// getHistoricalWidthBaseline() - fvg_context.go:126
宽度基准 = [
    0.001, 0.002, 0.003, 0.004, 0.005,       // 微小缺口（0.1%-0.5%）
    0.006, 0.007, 0.008, 0.009, 0.010,       // 小缺口（0.5%-1%）
    0.015, 0.020, 0.025, 0.030,              // 中等缺口（1%-3%）
    0.035, 0.040, 0.050, 0.070, 0.100,       // 大缺口（3%-10%）
    0.150, 0.200, 0.300,                     // 巨大缺口（>10%）
]
```

**单个FVG上下文计算** (calculateSingleFVGContext):
```go
// fvg_context.go:60-106
context := &ContextMetrics{
    // 1. 强度标准分（Z-Score, >1.5为强，>2.0为极强）
    StrengthZ: (gap.Strength - mean(allStrengths)) / stdDev(allStrengths),

    // 2. 宽度相对ATR倍数（优先使用FormationATR，避免时空错配）
    WidthATR: gap.WidthATR,  // 或重新计算

    // 3. 成交量比率（>2.0表示异常放量）
    VolRatio: gap.VolumeContext.FormationVolume / avgVolume,

    // 4. 是否新鲜
    IsFresh: (currentTime - gap.CreationTime) <= maxAge,

    // 5. 时间评分（时间衰减评分，越新鲜评分越高）
    TimeScore: calculateTimeScore(gap.CreationTime, maxAge),

    // 6. 排名百分位（0.8+表示前20%）
    RankPct: calculateRankPercentile(gap.Strength, allStrengths),
}
```

---

### 3.6 交易信号生成

#### GenerateSignals - 信号生成引擎

**位置**: `market/fvg.go:1032-1053`

**三种信号类型**:

**1. Reaction Signal（反应信号）** - 价格在FVG内
```go
// generateReactionSignal() - fvg.go:1082-1135
if currentPrice >= gap.LowerBound && currentPrice <= gap.UpperBound {
    if gap.Type == BullishFVG {
        action = ActionBuy
        entry = currentPrice
        stopLoss = gap.LowerBound × 0.995  // 下边界下0.5%
        takeProfit = currentPrice + (gap.Width × 2)  // 2倍缺口宽度
        description = "价格在看涨FVG内，预期向上反应"
    } else {
        action = ActionSell
        entry = currentPrice
        stopLoss = gap.UpperBound × 1.005
        takeProfit = currentPrice - (gap.Width × 2)
        description = "价格在看跌FVG内，预期向下反应"
    }

    // 置信度计算
    confidence = gap.Strength × 0.9
    if gap.Quality == FVQualityHigh { confidence += 10 }
    if gap.TouchCount == 0 { confidence += 5 }  // 首次触及加分
    if gap.Validation.VolumeValidation { confidence += 5 }
}
```

**2. Fill Entry Signal（填补入场信号）** - 价格接近FVG（1%范围内）
```go
// generateEntrySignal() - fvg.go:1138-1195
distanceToFVG := calculateDistanceToFVG(gap, currentPrice)

if distanceToFVG < 0.01 {  // 1%范围内
    if gap.Type == BullishFVG && currentPrice > gap.UpperBound {
        // 等待回调至看涨FVG
        action = ActionBuy
        entry = gap.CenterPrice
        stopLoss = gap.LowerBound × 0.995
        takeProfit = currentPrice + (gap.Width × 1.5)
        description = "等待回调至看涨FVG，准备买入"
    } else if gap.Type == BearishFVG && currentPrice < gap.LowerBound {
        // 等待反弹至看跌FVG
        action = ActionSell
        entry = gap.CenterPrice
        stopLoss = gap.UpperBound × 1.005
        takeProfit = currentPrice - (gap.Width × 1.5)
        description = "等待反弹至看跌FVG，准备卖出"
    }

    // 置信度（距离越近置信度越高）
    confidence = gap.Strength × (1 - distance / 0.01) × 0.8
    if gap.Quality == FVQualityHigh { confidence += 8 }
}
```

**3. Rejection Signal（拒绝信号）** - FVG显示强拒绝
```go
// generateRejectionSignal() - fvg.go:1198-1259
// 条件：
// - 触及次数 ≥ 2
// - 填补进度 ≤ 30%
// - 有强反应历史（ReactionStrength ≥ 2%）
// - 距离 < 0.5%

if gap.TouchCount >= 2 && gap.FillProgress <= 30 {
    if gap.Validation.HasReaction && gap.Validation.ReactionStrength >= 0.02 {
        distance := calculateDistanceToFVG(gap, currentPrice)
        if distance < 0.005 {
            if gap.Type == BullishFVG {
                action = ActionBuy
                entry = gap.LowerBound
                stopLoss = gap.LowerBound × 0.99
                takeProfit = gap.UpperBound + gap.Width
                description = "看涨FVG显示强拒绝，预期反弹"
            }

            // 高置信度
            confidence = gap.Strength × 0.85 + gap.Validation.ReactionStrength × 100
        }
    }
}
```

---

## 四、主分析流程

### Analyze - 主入口函数

**签名**: `func (fvg *FVGAnalyzer) Analyze(klines []Kline) *FVGData`

**流程图**:
```
输入：
  - klines: K线数据（最少3根）

处理步骤：
  1. 数据量校验
     └─ len(klines) >= 3

  2. 创建上下文计算器
     └─ contextCalc := NewContextCalculator(klines)

  3. 扫描识别FVG（从第2根K线开始）
     └─ for i := 2; i < len(klines); i++
        ├─ identifyBullishFVG(klines, i, contextCalc)
        └─ identifyBearishFVG(klines, i, contextCalc)

  4. 更新FVG状态
     └─ updateFVGStatuses(allFVGs, klines)

  5. 计算FVG强度和质量
     └─ for each gap:
        ├─ calculateFVGStrength(gap, klines)
        └─ assessFVGQuality(gap)

  6. 验证FVG（可选）
     └─ if EnableValidation:
        └─ validateFVG(gap, klines)

  7. 计算上下文评分
     └─ CalculateContextScores(allFVGs, contextCalc)

  8. P2级数据清洗
     └─ dataCleaner.CleanFVGData(tempFVGData)

  9. 筛选活跃FVG
     └─ filterActiveFVGsWithContext(allFVGs, currentPrice, atr, klines)

  10. 计算统计信息
      └─ calculateStatistics(bullishFVGs, bearishFVGs, activeFVGs)

  11. 初始化JSON契约
      └─ InitializeFVGData(result)

输出：
  FVGData{
      BullishFVGs,  // 所有看涨FVG
      BearishFVGs,  // 所有看跌FVG
      ActiveFVGs,   // 活跃FVG（经过筛选）
      Statistics,   // 统计信息
  }
```

---

## 五、关键算法特点

### 5.1 修复的问题

**1. 时空错配问题** ✅
- 使用形成时ATR (`FormationATR`) 而非当前ATR
- 避免用"未来的尺子"衡量"过去的缺口"

**2. 幸存者偏差** ✅
- 成交量确认时排除当前爆发K线
- `calculateAverageVolume(klines, index-basePeriod-1, index-1)`

**3. 样本偏差（"矮子里拔将军"）** ✅
- 扩展历史基准样本池
- 避免在低波动期或高波动期的评分失真

**4. 微型FVG干扰** ✅
- 基于时间框架的智能预过滤
- HTF: width_percent < 0.3% 过滤
- LTF: width_atr < 0.5 过滤

**5. 活跃FVG过多问题** ✅
- "先截断后打分"策略
- 时间框架独立Top 3限制

---

### 5.2 高级特性

**1. Body-to-Body FVG模式**
- 使用K线实体边界而非影线
- 减少影线干扰，适合高波动币种
- 配置: `UseBodyGap = true`

**2. Inversion FVG概念**
```go
Failed FVG → Inversion FVG
  ↓             ↓
完全填补    变成反向支撑/阻力
```

**3. 多维度质量评估**
- 宽度（30分）
- 冲击强度（25分）
- 成交量（20分）
- 形成类型（5-15分）
- 年龄加成（最多10分）
- 触及惩罚（每次-3分）

**4. 上下文标准化**
- Z-Score标准分
- ATR归一化
- 排名百分位
- 时间衰减评分

**5. 三层信号体系**
- Reaction（价格在FVG内）
- Fill Entry（价格接近FVG）
- Rejection（FVG显示强拒绝）

---

## 六、辅助指标计算

### 在FVG分析中使用的辅助指标

| 指标 | 周期 | 用途 | 计算方式 |
|------|------|------|---------|
| **ATR** | 14 | 宽度归一化 | 平均真实波幅 |
| **平均成交量** | 20 | 成交量确认 | 简单移动平均 |
| **Z-Score** | - | 强度标准化 | (x - μ) / σ |
| **Percentile** | - | 排名评估 | 百分位计算 |

**计算位置**:
- `calculateATR()` - fvg.go:123-143
- `calculateAverageVolume()` - fvg.go:513-537
- `CalculateStrengthZ()` - context_calculator.go
- `CalculateRankPercentile()` - context_calculator.go

---

## 七、实际应用流程图

```
市场数据获取
    ↓
[K线数据] ──→ 扫描识别FVG（三根K线模式）
    ↓             ↓
    |        时间框架智能预过滤
    |             ↓
    |        成交量确认（可选，修复幸存者偏差）
    ↓             ↓
状态更新 ←────────┘
    ├─ 计算填补进度
    ├─ Inversion转化判断
    └─ 触及统计
    ↓
强度与质量评估
    ├─ 宽度强度
    ├─ 冲击强度
    ├─ 成交量强度
    ├─ 形成类型强度
    └─ 触及惩罚
    ↓
上下文评分（修复样本偏差）
    ├─ 强度Z-Score
    ├─ 宽度ATR
    ├─ 成交量比率
    ├─ 新鲜度评分
    └─ 排名百分位
    ↓
P2级数据清洗
    ↓
活跃FVG筛选（先截断后打分）
    ├─ 硬截断（距离、填补进度）
    ├─ 综合评分排序
    └─ Top 3限制
    ↓
交易信号生成
    ├─ Reaction Signal
    ├─ Fill Entry Signal
    └─ Rejection Signal
    ↓
FVGData输出
```

---

## 八、关键源代码位置

| 功能模块 | 函数 | 行号 |
|---------|------|------|
| 看涨FVG识别 | `identifyBullishFVG` | 146-272 |
| 看跌FVG识别 | `identifyBearishFVG` | 275-401 |
| 状态更新 | `updateFVGStatuses` | 540-596 |
| Inversion转化 | `shouldConvertToInversion` | 599-628 |
| 填补进度 | `calculateFillProgress` | 637-673 |
| 触及统计 | `countTouches` | 676-698 |
| 强度计算 | `calculateFVGStrength` | 773-818 |
| 质量评估 | `assessFVGQuality` | 820-853 |
| 活跃FVG筛选 | `filterActiveFVGsWithContext` | 720-771 |
| 综合评分 | `calculateFVGScore` | 1280-1305 |
| 上下文评分 | `CalculateContextScores` | fvg_context.go:16-57 |
| Reaction信号 | `generateReactionSignal` | 1082-1135 |
| Entry信号 | `generateEntrySignal` | 1138-1195 |
| Rejection信号 | `generateRejectionSignal` | 1198-1259 |

---

## 九、使用示例

### 9.1 基本分析

```go
// 创建分析器
analyzer := NewFVGAnalyzer()

// 获取K线数据
klines := getKlines("BTCUSDT", "5m", 200)

// 执行分析
result := analyzer.Analyze(klines)

// 查看结果
fmt.Printf("看涨FVG数量: %d\n", len(result.BullishFVGs))
fmt.Printf("看跌FVG数量: %d\n", len(result.BearishFVGs))
fmt.Printf("活跃FVG数量: %d\n", len(result.ActiveFVGs))
fmt.Printf("平均FVG宽度: %.2f%%\n", result.Statistics.AvgFVGWidth)
fmt.Printf("填补率: %.1f%%\n", result.Statistics.FillRate)
```

### 9.2 自定义配置

```go
// 修改配置
config := analyzer.GetConfig()
config.UseBodyGap = true           // 启用Body-to-Body模式
config.RequireVolConf = true       // 要求成交量确认
config.MinVolumeRatio = 2.0        // 提高成交量要求到2倍
config.MaxDistanceATR = 3.0        // 减小距离限制到3 ATR
analyzer.UpdateConfig(config)

// 使用新配置分析
result := analyzer.Analyze(klines)
```

### 9.3 查询最近的FVG

```go
currentPrice := 45000.0
maxDistance := 0.02  // 2%范围内

nearFVGs := analyzer.FindNearestFVGs(result, currentPrice, maxDistance)

for i, gap := range nearFVGs {
    distance := calculateDistanceToFVG(gap, currentPrice)
    fmt.Printf("FVG %d: 类型=%s, 距离=%.2f%%, 强度=%.1f\n",
        i, gap.Type, distance*100, gap.Strength)
}
```

### 9.4 生成交易信号

```go
signals := analyzer.GenerateSignals(result, currentPrice)

for i, signal := range signals {
    fmt.Printf("信号 %d:\n", i)
    fmt.Printf("  类型: %s\n", signal.Type)
    fmt.Printf("  动作: %s\n", signal.Action)
    fmt.Printf("  入场: %.2f\n", signal.Entry)
    fmt.Printf("  止损: %.2f\n", signal.StopLoss)
    fmt.Printf("  止盈: %.2f\n", signal.TakeProfit)
    fmt.Printf("  风险收益比: %.2f\n", signal.RiskReward)
    fmt.Printf("  置信度: %.1f%%\n", signal.Confidence)
    fmt.Printf("  描述: %s\n", signal.Description)
}
```

### 9.5 查看上下文评分

```go
for _, gap := range result.ActiveFVGs {
    if gap.Context != nil {
        fmt.Printf("FVG %s:\n", gap.ID)
        fmt.Printf("  强度Z-Score: %.2f\n", gap.Context.StrengthZ)
        fmt.Printf("  宽度ATR: %.2f\n", gap.Context.WidthATR)
        fmt.Printf("  成交量比率: %.2f\n", gap.Context.VolRatio)
        fmt.Printf("  是否新鲜: %v\n", gap.Context.IsFresh)
        fmt.Printf("  时间评分: %.1f\n", gap.Context.TimeScore)
        fmt.Printf("  排名百分位: %.2f\n", gap.Context.RankPct)
        fmt.Printf("  综合评分: %.1f\n", gap.Score)
    }
}
```

---

## 十、常见问题

### 10.1 为什么识别出的FVG很少？

**可能原因**:
1. 时间框架预过滤太严格（width_percent或width_atr阈值）
2. 成交量确认开启但成交量不足
3. 市场处于低波动期，缺少明显缺口

**解决方案**:
```go
config := analyzer.GetConfig()
config.RequireVolConf = false  // 关闭成交量确认
config.UseBodyGap = true       // 尝试Body模式
analyzer.UpdateConfig(config)
```

### 10.2 为什么ActiveFVGs数量很少？

**可能原因**:
1. MaxDistanceATR设置过小（默认5 ATR）
2. 价格远离所有FVG
3. 大部分FVG已填补或过期

**解决方案**:
```go
config.MaxDistanceATR = 10.0  // 增大距离限制
config.MaxAge = 100           // 增大年龄限制
```

### 10.3 如何理解Inversion FVG？

**概念**:
- Failed FVG（被完全填补的FVG）往往变成反向的支撑/阻力位
- 原看涨FVG被填补后，可能成为阻力位
- 原看跌FVG被填补后，可能成为支撑位

**识别**:
```go
for _, gap := range result.BullishFVGs {
    if gap.IsInversion {
        fmt.Printf("Inversion FVG: %.2f-%.2f\n", gap.LowerBound, gap.UpperBound)
        fmt.Printf("  原类型: %s\n", gap.Type)
        fmt.Printf("  转化时间: %d\n", gap.InversionTime)
    }
}
```

### 10.4 如何选择UseBodyGap模式？

**推荐**:
- **高波动币种**（山寨币）: UseBodyGap = true
  - 减少影线干扰
  - 更准确的缺口识别
- **低波动币种**（BTC、ETH）: UseBodyGap = false
  - 使用传统ICT标准
  - 更保守的识别

---

## 十一、总结

该FVG实现是一个**生产级的ICT分析引擎**，特点包括：

✅ **严格的ICT理论**: 三根K线模式，支持Body-to-Body变体
✅ **完整生命周期管理**: Fresh → Tested → PartialFill → Filled → Inversion
✅ **时空错配修复**: 使用FormationATR而非当前ATR
✅ **幸存者偏差修复**: 成交量确认排除爆发K线
✅ **样本偏差修复**: 历史基准扩展样本池
✅ **智能筛选策略**: "先截断后打分" + Top 3限制
✅ **多维度评估**: 强度、质量、上下文、验证
✅ **三层信号体系**: Reaction、Fill Entry、Rejection
✅ **可配置性**: 完整的配置体系，支持动态调整

适用于所有时间框架（5m/15m/1h/4h），尤其在趋势市场和波动市场中表现稳定。

---

**文档版本**: 1.0
**最后更新**: 2026-01-18
**维护者**: AI自动交易系统开发团队
