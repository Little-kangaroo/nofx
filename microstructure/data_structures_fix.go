package microstructure

import (
	"fmt"
	"time"
)

// ===== P0-04修复：数据状态强一致性定义 =====

// DataStatus 数据状态枚举（强一致性原则）
type DataStatus string

const (
    DataStatusNoData      DataStatus = "no_data"        // 完全没有数据
    DataStatusStale       DataStatus = "stale"          // 数据过期
    DataStatusPartial     DataStatus = "partial"        // 数据不完整
    DataStatusFresh       DataStatus = "fresh"          // 数据新鲜
    DataStatusDegraded    DataStatus = "degraded"       // 数据质量降级
)

// DataHealthInfo 数据健康度信息（P0-04修复核心结构）
type DataHealthInfo struct {
    Status         DataStatus `json:"status"`            // 明确的数据状态
    HasData        bool       `json:"has_data"`          // 是否有真实数据
    LastRealUpdate time.Time  `json:"last_real_update"`  // 最后真实数据更新时间
    CreatedAt      time.Time  `json:"created_at"`        // 结构创建时间
    DataAge        float64    `json:"data_age_minutes"`  // 数据年龄（分钟）
    Quality        float64    `json:"quality"`           // 质量评分 0-1
}

// ===== P0-04修复：增强版数据结构 =====

// CVDDataV2 CVD数据（P0-04修复版本）
type CVDDataV2 struct {
    SpotCVD1H     float64        `json:"spot_cvd_1h"`
    FuturesCVD1H  float64        `json:"futures_cvd_1h"`
    CVDDivergence string         `json:"cvd_divergence"`
    Signal        string         `json:"signal"`
    DataHealth    DataHealthInfo `json:"data_health"`     // 🔥 P0-04修复：替换LastUpdate+IsStale
}

// OIAnalysisV2 持仓量分析（P0-04修复版本）
type OIAnalysisV2 struct {
    Current      float64        `json:"current"`
    Change1H     float64        `json:"change_1h"`
    Change4H     float64        `json:"change_4h"`
    ChangeRate1H float64        `json:"change_rate_1h"`
    ChangeRate4H float64        `json:"change_rate_4h"`
    Trend        string         `json:"trend"`
    DataHealth   DataHealthInfo `json:"data_health"`     // 🔥 P0-04修复：强一致性
}

// OrderBookDataV2 订单簿数据（P0-04修复版本）
type OrderBookDataV2 struct {
    ImbalanceRatio    float64        `json:"imbalance_ratio"`
    NearestResistance *WallInfo      `json:"nearest_resistance"`
    NearestSupport    *WallInfo      `json:"nearest_support"`
    BidPressure       float64        `json:"bid_pressure"`
    AskPressure       float64        `json:"ask_pressure"`
    ImbalanceTrend    string         `json:"imbalance_trend"`
    PressureDelta5m   float64        `json:"pressure_delta_5m"`
    SpoofingRisk      float64        `json:"spoofing_risk"`
    LiquidityScore    float64        `json:"liquidity_score"`
    WallChangeCount5m int            `json:"wall_change_count_5m"`
    DataHealth        DataHealthInfo `json:"data_health"`     // 🔥 P0-04修复：强一致性
}

// MacroTrendDataV2 宏观趋势数据（P0-04修复版本）
type MacroTrendDataV2 struct {
    SpotCVD1H         float64        `json:"spot_cvd_1h"`
    FuturesCVD1H      float64        `json:"futures_cvd_1h"`
    CVDDivergence4H   bool           `json:"cvd_divergence_4h"`
    MarketRegime      string         `json:"market_regime"`
    TrendStrength     float64        `json:"trend_strength"`
    DominantDirection string         `json:"dominant_direction"`
    ConfidenceLevel   float64        `json:"confidence_level"`
    DataHealth        DataHealthInfo `json:"data_health"`     // 🔥 P0-04修复：强一致性
}

// ===== P0-04修复：工具函数 =====

// NewNoDataHealth 创建"无数据"状态的健康度信息
func NewNoDataHealth() DataHealthInfo {
    return DataHealthInfo{
        Status:         DataStatusNoData,
        HasData:        false,
        LastRealUpdate: time.Time{}, // 🔥 P0-04修复：零值时间，不是time.Now()
        CreatedAt:      time.Now(),
        DataAge:        -1, // -1表示无数据
        Quality:        0,
    }
}

// NewStaleDataHealth 创建"过期数据"状态的健康度信息
func NewStaleDataHealth(lastUpdate time.Time) DataHealthInfo {
    now := time.Now()
    dataAge := now.Sub(lastUpdate).Minutes()
    
    return DataHealthInfo{
        Status:         DataStatusStale,
        HasData:        true, // 有数据，但过期了
        LastRealUpdate: lastUpdate,
        CreatedAt:      now,
        DataAge:        dataAge,
        Quality:        calculateStaleQuality(dataAge),
    }
}

// NewFreshDataHealth 创建"新鲜数据"状态的健康度信息
func NewFreshDataHealth(lastUpdate time.Time, quality float64) DataHealthInfo {
    now := time.Now()
    dataAge := now.Sub(lastUpdate).Minutes()
    
    return DataHealthInfo{
        Status:         DataStatusFresh,
        HasData:        true,
        LastRealUpdate: lastUpdate,
        CreatedAt:      now,
        DataAge:        dataAge,
        Quality:        quality,
    }
}

// calculateStaleQuality 计算过期数据的质量评分
func calculateStaleQuality(dataAgeMinutes float64) float64 {
    if dataAgeMinutes > 30 {
        return 0.0 // 超过30分钟完全不可信
    } else if dataAgeMinutes > 15 {
        return 0.1 // 15-30分钟：极低可信度
    } else if dataAgeMinutes > 5 {
        return 0.3 // 5-15分钟：低可信度
    }
    return 0.7 // 5分钟内：中等可信度（但仍然是过期数据）
}

// ===== P0-04修复：数据质量判定工具 =====

// IsDataReliable 判断数据是否可靠（强一致性判定）
func (dh DataHealthInfo) IsDataReliable() bool {
    return dh.HasData && dh.Status == DataStatusFresh && dh.Quality > 0.5
}

// GetDataConfidence 获取数据置信度（0-1）
func (dh DataHealthInfo) GetDataConfidence() float64 {
    if !dh.HasData {
        return 0.0
    }
    
    switch dh.Status {
    case DataStatusFresh:
        return dh.Quality
    case DataStatusStale:
        return dh.Quality * 0.5 // 过期数据置信度减半
    case DataStatusPartial:
        return dh.Quality * 0.7 // 不完整数据置信度打七折
    case DataStatusDegraded:
        return dh.Quality * 0.3 // 降级数据置信度大幅下降
    case DataStatusNoData:
        return 0.0
    default:
        return 0.0
    }
}

// GetDisplayAge 获取用于显示的数据年龄字符串
func (dh DataHealthInfo) GetDisplayAge() string {
    if !dh.HasData {
        return "无数据"
    }
    
    if dh.DataAge < 0 {
        return "年龄未知"
    }
    
    if dh.DataAge < 1 {
        return "< 1分钟"
    } else if dh.DataAge < 60 {
        return fmt.Sprintf("%.1f分钟", dh.DataAge)
    } else {
        return fmt.Sprintf("%.1f小时", dh.DataAge/60)
    }
}