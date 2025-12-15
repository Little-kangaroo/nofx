package microstructure

import (
    "math"
    "time"
)

// ===== P0-05修复：盘口数据stale语义强化配置 =====

// OrderBookStaleConfig 订单簿数据过期配置（P0-05修复）
type OrderBookStaleConfig struct {
    // 🔥 P0-05修复：细粒度stale阈值配置，不再硬编码5分钟
    StaleThresholdSeconds    int     `json:"stale_threshold_seconds"`     // 过期阈值（秒）默认30
    CriticalThresholdSeconds int     `json:"critical_threshold_seconds"`  // 严重过期阈值（秒）默认60
    UnavailableThresholdSeconds int `json:"unavailable_threshold_seconds"` // 不可用阈值（秒）默认120
    
    // 🔥 P0-05修复：按币种类型差异化配置
    BTCStaleThresholdSeconds int     `json:"btc_stale_threshold_seconds"`   // BTC专用阈值（更严格）
    AltStaleThresholdSeconds int     `json:"alt_stale_threshold_seconds"`   // 山寨币阈值（可放松）
    
    // 🔥 P0-05修复：数据质量权重配置
    StaleDataWeight         float64  `json:"stale_data_weight"`          // 过期数据权重 0-1
    CriticalDataWeight      float64  `json:"critical_data_weight"`       // 严重过期数据权重 0-1
    UnavailableDataWeight   float64  `json:"unavailable_data_weight"`    // 不可用数据权重（通常为0）
}

// DefaultOrderBookStaleConfig 默认配置（P0-05修复）
func DefaultOrderBookStaleConfig() *OrderBookStaleConfig {
    return &OrderBookStaleConfig{
        // 🔥 P0-05修复：盘口数据要求极高实时性
        StaleThresholdSeconds:       30,  // 30秒过期（原来5分钟=300秒太宽松）
        CriticalThresholdSeconds:    60,  // 1分钟严重过期
        UnavailableThresholdSeconds: 120, // 2分钟不可用
        
        // 🔥 P0-05修复：BTC要求更严格的实时性
        BTCStaleThresholdSeconds: 15,  // BTC 15秒过期
        AltStaleThresholdSeconds: 45,  // 山寨币 45秒过期
        
        // 🔥 P0-05修复：过期数据权重大幅降低
        StaleDataWeight:       0.3,  // 过期数据权重降至30%
        CriticalDataWeight:    0.1,  // 严重过期数据权重仅10%
        UnavailableDataWeight: 0.0,  // 不可用数据权重为0
    }
}

// GetStaleThresholdForSymbol 根据币种获取过期阈值（P0-05修复）
func (config *OrderBookStaleConfig) GetStaleThresholdForSymbol(symbol string) time.Duration {
    if symbol == "BTCUSDT" {
        return time.Duration(config.BTCStaleThresholdSeconds) * time.Second
    }
    return time.Duration(config.AltStaleThresholdSeconds) * time.Second
}

// ===== P0-05修复：增强版盘口数据健康状态 =====

// OrderBookHealth 盘口数据健康状态（P0-05修复核心结构）
type OrderBookHealth struct {
    Status              OrderBookDataStatus `json:"status"`               // 数据状态枚举
    AgeMilliseconds     int64              `json:"age_milliseconds"`     // 🔥 P0-05修复：精确到毫秒的年龄
    StaleThresholdMs    int64              `json:"stale_threshold_ms"`   // 过期阈值（毫秒）
    IsUsableForTrading  bool               `json:"is_usable_for_trading"` // 🔥 P0-05修复：交易可用性明确标记
    DataWeight          float64            `json:"data_weight"`          // 数据权重 0-1
    QualityScore        float64            `json:"quality_score"`        // 质量评分 0-1
    LastRealUpdate      time.Time          `json:"last_real_update"`     // 最后真实更新时间
    HealthCheckTime     time.Time          `json:"health_check_time"`    // 健康检查时间
}

// OrderBookDataStatus 盘口数据状态枚举（P0-05修复）
type OrderBookDataStatus string

const (
    OrderBookStatusFresh        OrderBookDataStatus = "fresh"        // 新鲜可用
    OrderBookStatusStale        OrderBookDataStatus = "stale"        // 过期但可用（降权）
    OrderBookStatusCritical     OrderBookDataStatus = "critical"     // 严重过期（不推荐交易）
    OrderBookStatusUnavailable  OrderBookDataStatus = "unavailable"  // 不可用（禁止交易）
    OrderBookStatusNoData       OrderBookDataStatus = "no_data"      // 完全无数据
)

// ===== P0-05修复：增强版盘口数据结构 =====

// OrderBookDataV2Enhanced 增强版盘口数据（P0-05修复）
type OrderBookDataV2Enhanced struct {
    // 🔥 原有字段保持兼容
    ImbalanceRatio    float64        `json:"imbalance_ratio"`
    NearestResistance *WallInfoV2    `json:"nearest_resistance"`    // 🔥 P0-05修复：增强版墙信息
    NearestSupport    *WallInfoV2    `json:"nearest_support"`       // 🔥 P0-05修复：增强版墙信息
    BidPressure       float64        `json:"bid_pressure"`
    AskPressure       float64        `json:"ask_pressure"`
    ImbalanceTrend    string         `json:"imbalance_trend"`
    PressureDelta5m   float64        `json:"pressure_delta_5m"`
    SpoofingRisk      float64        `json:"spoofing_risk"`
    LiquidityScore    float64        `json:"liquidity_score"`
    WallChangeCount5m int            `json:"wall_change_count_5m"`
    
    // 🔥 P0-05修复：强化健康状态和可用性
    Health            OrderBookHealth `json:"health"`              // 健康状态详情
    TradingSafety     TradingSafetyInfo `json:"trading_safety"`    // 交易安全信息
}

// WallInfoV2 增强版墙信息（P0-05修复）
type WallInfoV2 struct {
    Price             float64            `json:"price"`
    StrengthUSD       float64            `json:"strength_usd"`
    IsSolid           bool               `json:"is_solid"`
    Distance          float64            `json:"distance"`
    LevelCount        int                `json:"level_count"`
    StabilityScore    float64            `json:"stability_score"`
    FlickerCount      int                `json:"flicker_count"`
    ExistenceDuration time.Duration      `json:"existence_duration"`
    LastSeen          time.Time          `json:"last_seen"`
    FirstSeen         time.Time          `json:"first_seen"`
    AverageSize       float64            `json:"average_size"`
    MaxSize           float64            `json:"max_size"`
    MinSize           float64            `json:"min_size"`
    
    // 🔥 P0-05修复：墙的可靠性评估
    Reliability       WallReliability    `json:"reliability"`      // 墙可靠性信息
}

// WallReliability 墙可靠性信息（P0-05修复）
type WallReliability struct {
    IsReliable        bool     `json:"is_reliable"`        // 是否可靠（基于数据新鲜度）
    ReliabilityScore  float64  `json:"reliability_score"`  // 可靠性评分 0-1
    DataAge           float64  `json:"data_age_seconds"`   // 数据年龄（秒）
    Warning           string   `json:"warning"`            // 可靠性警告
}

// TradingSafetyInfo 交易安全信息（P0-05修复核心）
type TradingSafetyInfo struct {
    IsSafeForEntry      bool     `json:"is_safe_for_entry"`       // 🔥 是否安全进场
    IsSafeForStopLoss   bool     `json:"is_safe_for_stop_loss"`   // 🔥 是否安全设置止损
    SafetyLevel         string   `json:"safety_level"`            // 安全等级：high/medium/low/unsafe
    RiskFactors         []string `json:"risk_factors"`            // 风险因子列表
    Recommendation      string   `json:"recommendation"`          // 安全建议
}

// ===== P0-05修复：盘口健康度计算工具 =====

// CalculateOrderBookHealth 计算盘口健康度（P0-05修复核心方法）
func CalculateOrderBookHealth(lastUpdate time.Time, config *OrderBookStaleConfig, symbol string) OrderBookHealth {
    now := time.Now()
    
    // 🔥 P0-05修复：精确计算数据年龄（毫秒级）
    ageMs := now.Sub(lastUpdate).Milliseconds()
    staleThresholdMs := config.GetStaleThresholdForSymbol(symbol).Milliseconds()
    criticalThresholdMs := int64(config.CriticalThresholdSeconds) * 1000
    unavailableThresholdMs := int64(config.UnavailableThresholdSeconds) * 1000
    
    // 🔥 P0-05修复：基于精确年龄判断状态
    var status OrderBookDataStatus
    var dataWeight float64
    var isUsableForTrading bool
    var qualityScore float64
    
    if lastUpdate.IsZero() {
        status = OrderBookStatusNoData
        dataWeight = 0.0
        isUsableForTrading = false
        qualityScore = 0.0
    } else if ageMs >= unavailableThresholdMs {
        status = OrderBookStatusUnavailable
        dataWeight = config.UnavailableDataWeight
        isUsableForTrading = false
        qualityScore = 0.0
    } else if ageMs >= criticalThresholdMs {
        status = OrderBookStatusCritical
        dataWeight = config.CriticalDataWeight
        isUsableForTrading = false // 🔥 P0-05修复：严重过期禁止交易
        qualityScore = 0.1
    } else if ageMs >= staleThresholdMs {
        status = OrderBookStatusStale
        dataWeight = config.StaleDataWeight
        isUsableForTrading = true // 可用但降权
        qualityScore = 0.5
    } else {
        status = OrderBookStatusFresh
        dataWeight = 1.0
        isUsableForTrading = true
        qualityScore = 1.0
    }
    
    return OrderBookHealth{
        Status:             status,
        AgeMilliseconds:    ageMs,
        StaleThresholdMs:   staleThresholdMs,
        IsUsableForTrading: isUsableForTrading,
        DataWeight:         dataWeight,
        QualityScore:       qualityScore,
        LastRealUpdate:     lastUpdate,
        HealthCheckTime:    now,
    }
}

// CalculateTradingSafety 计算交易安全性（P0-05修复核心方法）
func CalculateTradingSafety(health OrderBookHealth, imbalanceRatio float64, liquidityScore float64) TradingSafetyInfo {
    var isSafeForEntry bool
    var isSafeForStopLoss bool
    var safetyLevel string
    var riskFactors []string
    var recommendation string
    
    // 🔥 P0-05修复：基于数据健康度判断交易安全性
    switch health.Status {
    case OrderBookStatusFresh:
        isSafeForEntry = true
        isSafeForStopLoss = true
        safetyLevel = "high"
        
        // 进一步检查市场结构风险
        if liquidityScore < 0.3 {
            riskFactors = append(riskFactors, "低流动性")
            safetyLevel = "medium"
        }
        if math.Abs(imbalanceRatio) > 0.8 {
            riskFactors = append(riskFactors, "极端失衡")
            if safetyLevel == "high" {
                safetyLevel = "medium"
            }
        }
        
    case OrderBookStatusStale:
        isSafeForEntry = false // 🔥 P0-05修复：过期数据不安全进场
        isSafeForStopLoss = true // 可以设置止损但要谨慎
        safetyLevel = "low"
        riskFactors = append(riskFactors, "盘口数据过期")
        recommendation = "等待新鲜数据再进场，现有止损可保持"
        
    case OrderBookStatusCritical:
        isSafeForEntry = false
        isSafeForStopLoss = false // 🔥 P0-05修复：严重过期连止损都不安全
        safetyLevel = "unsafe"
        riskFactors = append(riskFactors, "盘口数据严重过期", "支撑阻力位不可信")
        recommendation = "立即停止交易，等待数据恢复"
        
    case OrderBookStatusUnavailable, OrderBookStatusNoData:
        isSafeForEntry = false
        isSafeForStopLoss = false
        safetyLevel = "unsafe"
        riskFactors = append(riskFactors, "盘口数据不可用")
        recommendation = "禁止交易，检查数据连接"
    }
    
    return TradingSafetyInfo{
        IsSafeForEntry:    isSafeForEntry,
        IsSafeForStopLoss: isSafeForStopLoss,
        SafetyLevel:       safetyLevel,
        RiskFactors:       riskFactors,
        Recommendation:    recommendation,
    }
}

// ===== P0-05修复：墙可靠性评估工具 =====

// CalculateWallReliability 计算墙可靠性（P0-05修复）
func CalculateWallReliability(wall *WallInfo, orderBookHealth OrderBookHealth) WallReliability {
    if wall == nil {
        return WallReliability{
            IsReliable:       false,
            ReliabilityScore: 0.0,
            DataAge:         -1,
            Warning:         "墙信息不存在",
        }
    }
    
    // 🔥 P0-05修复：墙的可靠性直接受盘口数据健康度影响
    var isReliable bool
    var reliabilityScore float64
    var warning string
    
    dataAgeSec := float64(orderBookHealth.AgeMilliseconds) / 1000.0
    
    switch orderBookHealth.Status {
    case OrderBookStatusFresh:
        isReliable = true
        reliabilityScore = 0.9 + wall.StabilityScore*0.1 // 基础0.9 + 墙稳定性加成
        
    case OrderBookStatusStale:
        isReliable = false // 🔥 P0-05修复：过期数据的墙不可靠
        reliabilityScore = 0.3
        warning = "基于过期盘口数据，墙位置可能已变化"
        
    case OrderBookStatusCritical:
        isReliable = false
        reliabilityScore = 0.1
        warning = "盘口数据严重过期，墙信息高度不可信"
        
    default:
        isReliable = false
        reliabilityScore = 0.0
        warning = "盘口数据不可用，墙信息无效"
    }
    
    return WallReliability{
        IsReliable:       isReliable,
        ReliabilityScore: reliabilityScore,
        DataAge:         dataAgeSec,
        Warning:         warning,
    }
}