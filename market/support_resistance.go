package market

import (
	"math"
	"sort"
	"time"
)

// SupportResistanceAnalyzer 支撑阻力转换线分析器
type SupportResistanceAnalyzer struct {
	config SRConfig
}

// SRConfig 支撑阻力分析配置
type SRConfig struct {
	LookbackPeriods int     `json:"lookback_periods"` // 回看K线数量
	PivotLeft       int     `json:"pivot_left"`       // 转折点左侧比较根数
	PivotRight      int     `json:"pivot_right"`      // 转折点右侧比较根数
	ClusterTolerance float64 `json:"cluster_tolerance"` // 聚类容差百分比
	MinHits         int     `json:"min_hits"`         // 最小命中次数
}

// PivotPoint 转折点
type PivotPoint struct {
	Price     float64 `json:"price"`     // 价格
	Type      string  `json:"type"`      // "H"(高点) 或 "L"(低点)
	Index     int     `json:"index"`     // K线索引
	Timestamp int64   `json:"timestamp"` // 时间戳
}

// PriceCluster 价格聚类
type PriceCluster struct {
	CenterPrice float64       `json:"center_price"` // 中心价格
	Points      []*PivotPoint `json:"points"`       // 转折点列表
	Count       int           `json:"count"`        // 命中次数
}

// SRLevel 支撑阻力级别（简化版 - 只返回4根关键水平线）
type SRLevel struct {
	Price    float64 `json:"price"`     // 价格
	HitCount int     `json:"hit_count"` // 命中次数
	Type     string  `json:"type"`      // "support" 或 "resistance"（相对当前价格）
	Strength float64 `json:"strength"`  // 强度评分 (0-100)
}

// SupportResistanceData 支撑阻力分析结果
type SupportResistanceData struct {
	KeyLevels    []*SRLevel    `json:"key_levels"`    // 4根关键水平线
	Statistics   *SRStatistics `json:"statistics"`    // 统计信息
	Config       *SRConfig     `json:"config"`        // 配置信息
	LastAnalysis int64         `json:"last_analysis"` // 最后分析时间
}

// SRStatistics 支撑阻力统计
type SRStatistics struct {
	TotalLevels     int     `json:"total_levels"`     // 总级别数
	SupportCount    int     `json:"support_count"`    // 支撑级别数
	ResistanceCount int     `json:"resistance_count"` // 阻力级别数
	AvgStrength     float64 `json:"avg_strength"`     // 平均强度
	AvgHitCount     float64 `json:"avg_hit_count"`    // 平均命中次数
}

// 默认配置
var defaultSRConfig = SRConfig{
	LookbackPeriods:  1000, // 回看1000根K线（最大化结构视野）
	PivotLeft:        3,    // 左侧3根比较
	PivotRight:       3,    // 右侧3根比较
	ClusterTolerance: 0.005, // 0.5%聚类容差
	MinHits:          3,    // 至少3次命中
}

// NewSupportResistanceAnalyzer 创建支撑阻力分析器
func NewSupportResistanceAnalyzer() *SupportResistanceAnalyzer {
	return &SupportResistanceAnalyzer{
		config: defaultSRConfig,
	}
}

// Analyze 分析支撑阻力转换线
func (sra *SupportResistanceAnalyzer) Analyze(klines []Kline) *SupportResistanceData {
	if len(klines) == 0 {
		return &SupportResistanceData{
			KeyLevels:    []*SRLevel{},
			Statistics:   &SRStatistics{},
			Config:       &sra.config,
			LastAnalysis: time.Now().UnixMilli(),
		}
	}

	// 1. 计算支撑阻力转换线
	levels := sra.computeSRLevels(klines)

	// 2. 计算统计信息
	statistics := sra.calculateStatistics(levels)

	// 3. 筛选活跃级别（只返回最重要的4根水平线）
	activeLevels := sra.selectKeyLevels(levels, klines[len(klines)-1].Close, 4)

	return &SupportResistanceData{
		KeyLevels:    activeLevels,
		Statistics:   statistics,
		Config:       &sra.config,
		LastAnalysis: time.Now().UnixMilli(),
	}
}

// computeSRLevels 计算支撑阻力转换线（按照标准算法）
func (sra *SupportResistanceAnalyzer) computeSRLevels(klines []Kline) []*SRLevel {
	N := len(klines)
	if N == 0 {
		return []*SRLevel{}
	}

	// 1. 确定回看区间
	endIndex := N - 1 - sra.config.PivotRight
	startIndex := sra.config.PivotLeft
	if N-sra.config.LookbackPeriods > startIndex {
		startIndex = N - sra.config.LookbackPeriods
	}
	
	if endIndex <= startIndex {
		return []*SRLevel{}
	}

	// 2. 找所有转折点
	pivotPoints := sra.findPivotPoints(klines, startIndex, endIndex)

	// 3. 按价格排序
	sort.Slice(pivotPoints, func(i, j int) bool {
		return pivotPoints[i].Price < pivotPoints[j].Price
	})

	// 4. 聚类：把价格靠近的转折点合并
	clusters := sra.clusterPivotPoints(pivotPoints)

	// 5. 过滤：只保留命中次数足够多的簇
	validClusters := sra.filterClusters(clusters)

	// 6. 生成最终的支撑阻力级别
	levels := sra.generateLevels(validClusters, klines[len(klines)-1].Close)

	// 7. 按价格排序
	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Price < levels[j].Price
	})

	return levels
}

// findPivotPoints 寻找转折点
func (sra *SupportResistanceAnalyzer) findPivotPoints(klines []Kline, startIndex, endIndex int) []*PivotPoint {
	var pivotPoints []*PivotPoint

	for i := startIndex; i <= endIndex; i++ {
		current := klines[i]

		// 检查是否为转折高点
		if sra.isPivotHigh(klines, i) {
			pivotPoints = append(pivotPoints, &PivotPoint{
				Price:     current.High,
				Type:      "H",
				Index:     i,
				Timestamp: current.OpenTime,
			})
		}

		// 检查是否为转折低点
		if sra.isPivotLow(klines, i) {
			pivotPoints = append(pivotPoints, &PivotPoint{
				Price:     current.Low,
				Type:      "L",
				Index:     i,
				Timestamp: current.OpenTime,
			})
		}
	}

	return pivotPoints
}

// isPivotHigh 判断是否为转折高点
func (sra *SupportResistanceAnalyzer) isPivotHigh(klines []Kline, index int) bool {
	if index < sra.config.PivotLeft || index >= len(klines)-sra.config.PivotRight {
		return false
	}

	currentHigh := klines[index].High

	// 检查左侧
	for j := index - sra.config.PivotLeft; j < index; j++ {
		if klines[j].High > currentHigh {
			return false
		}
	}

	// 检查右侧
	for j := index + 1; j <= index+sra.config.PivotRight; j++ {
		if klines[j].High > currentHigh {
			return false
		}
	}

	return true
}

// isPivotLow 判断是否为转折低点
func (sra *SupportResistanceAnalyzer) isPivotLow(klines []Kline, index int) bool {
	if index < sra.config.PivotLeft || index >= len(klines)-sra.config.PivotRight {
		return false
	}

	currentLow := klines[index].Low

	// 检查左侧
	for j := index - sra.config.PivotLeft; j < index; j++ {
		if klines[j].Low < currentLow {
			return false
		}
	}

	// 检查右侧
	for j := index + 1; j <= index+sra.config.PivotRight; j++ {
		if klines[j].Low < currentLow {
			return false
		}
	}

	return true
}

// clusterPivotPoints 聚类转折点（V-10.0优化版：严格控制HitCount）
func (sra *SupportResistanceAnalyzer) clusterPivotPoints(pivotPoints []*PivotPoint) []*PriceCluster {
	var clusters []*PriceCluster
	maxClusterSize := 8 // 【关键优化】每个簇最多8个pivot点，适配AI V-10.0严格规则

	for _, point := range pivotPoints {
		assigned := false

		// 在已有簇中寻找可以合并的
		for _, cluster := range clusters {
			// 【修复1】检查簇大小限制
			if cluster.Count >= maxClusterSize {
				continue // 跳过已满的簇
			}
			
			allowedDiff := cluster.CenterPrice * sra.config.ClusterTolerance
			if math.Abs(point.Price-cluster.CenterPrice) <= allowedDiff {
				// 【修复2】检查pivot点时间间隔，避免同一时间段重复聚类
				canAdd := true
				for _, existingPoint := range cluster.Points {
					timeDiff := math.Abs(float64(point.Timestamp - existingPoint.Timestamp))
					// 【优化】如果间隔少于6小时，跳过（更严格的时间过滤）
					if timeDiff < 6*3600*1000 {
						canAdd = false
						break
					}
				}
				
				if canAdd {
					// 加入现有簇
					cluster.Points = append(cluster.Points, point)
					cluster.Count++

					// 更新中心价格（简单平均）
					sumPrice := 0.0
					for _, p := range cluster.Points {
						sumPrice += p.Price
					}
					cluster.CenterPrice = sumPrice / float64(cluster.Count)

					assigned = true
					break
				}
			}
		}

		// 如果没有合适的簇，新建一个
		if !assigned {
			newCluster := &PriceCluster{
				CenterPrice: point.Price,
				Points:      []*PivotPoint{point},
				Count:       1,
			}
			clusters = append(clusters, newCluster)
		}
	}

	return clusters
}

// filterClusters 过滤簇，只保留命中次数足够的
func (sra *SupportResistanceAnalyzer) filterClusters(clusters []*PriceCluster) []*PriceCluster {
	var validClusters []*PriceCluster

	for _, cluster := range clusters {
		if cluster.Count >= sra.config.MinHits {
			validClusters = append(validClusters, cluster)
		}
	}

	return validClusters
}

// generateLevels 生成支撑阻力级别
func (sra *SupportResistanceAnalyzer) generateLevels(clusters []*PriceCluster, currentPrice float64) []*SRLevel {
	var levels []*SRLevel

	for _, cluster := range clusters {
		// 分析级别类型
		levelType := sra.analyzeLevelType(cluster, currentPrice)

		// 计算强度
		strength := sra.calculateLevelStrength(cluster)

		level := &SRLevel{
			Price:    cluster.CenterPrice,
			HitCount: cluster.Count,
			Type:     levelType,
			Strength: strength,
		}

		levels = append(levels, level)
	}

	return levels
}

// analyzeLevelType 分析级别类型
func (sra *SupportResistanceAnalyzer) analyzeLevelType(cluster *PriceCluster, currentPrice float64) string {
	highCount := 0
	lowCount := 0

	for _, point := range cluster.Points {
		if point.Type == "H" {
			highCount++
		} else if point.Type == "L" {
			lowCount++
		}
	}

	// 根据转折点类型和当前价格位置判断
	if cluster.CenterPrice > currentPrice {
		if highCount > lowCount {
			return "resistance" // 在当前价格上方的高点集群 = 阻力
		} else {
			return "resistance" // 在上方的低点集群可能是之前的支撑转为阻力
		}
	} else {
		if lowCount > highCount {
			return "support" // 在当前价格下方的低点集群 = 支撑
		} else {
			return "support" // 在下方的高点集群可能是之前的阻力转为支撑
		}
	}
}

// calculateLevelStrength 计算级别强度
func (sra *SupportResistanceAnalyzer) calculateLevelStrength(cluster *PriceCluster) float64 {
	// 基于命中次数的强度，归一化到0-1
	baseStrength := math.Min(float64(cluster.Count)/10.0, 1.0)

	// 可以添加其他因素，如时间跨度、价格波动等
	return baseStrength
}

// calculateLevelConfidence 计算级别置信度
func (sra *SupportResistanceAnalyzer) calculateLevelConfidence(cluster *PriceCluster) float64 {
	// 简单的置信度计算：基于命中次数
	confidence := math.Min(float64(cluster.Count)/5.0, 1.0)
	return confidence
}

// selectKeyLevels 选择关键水平线（只返回4根最重要的）
func (sra *SupportResistanceAnalyzer) selectKeyLevels(levels []*SRLevel, currentPrice float64, maxCount int) []*SRLevel {
	if len(levels) == 0 {
		return []*SRLevel{}
	}

	// 按强度排序，选择最强的级别
	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Strength > levels[j].Strength
	})

	var selectedLevels []*SRLevel
	var supportLevels []*SRLevel
	var resistanceLevels []*SRLevel

	// 分类收集支撑和阻力位
	for _, level := range levels {
		if level.Price < currentPrice {
			// 当前价格下方 = 支撑
			level.Type = "support"
			supportLevels = append(supportLevels, level)
		} else if level.Price > currentPrice {
			// 当前价格上方 = 阻力
			level.Type = "resistance"
			resistanceLevels = append(resistanceLevels, level)
		}
	}

	// 支撑位按价格降序排列（离当前价格最近的在前）
	sort.Slice(supportLevels, func(i, j int) bool {
		return supportLevels[i].Price > supportLevels[j].Price
	})

	// 阻力位按价格升序排列（离当前价格最近的在前）
	sort.Slice(resistanceLevels, func(i, j int) bool {
		return resistanceLevels[i].Price < resistanceLevels[j].Price
	})

	// 选择最多2个支撑位和2个阻力位，确保总数不超过maxCount
	supportCount := len(supportLevels)
	resistanceCount := len(resistanceLevels)
	
	maxSupport := maxCount / 2
	maxResistance := maxCount / 2
	
	// 如果某一方不足，另一方可以多选
	if supportCount < maxSupport {
		maxResistance += maxSupport - supportCount
		maxSupport = supportCount
	}
	if resistanceCount < maxResistance {
		maxSupport += maxResistance - resistanceCount
		maxResistance = resistanceCount
	}

	// 确保不超过实际数量
	if maxSupport > supportCount {
		maxSupport = supportCount
	}
	if maxResistance > resistanceCount {
		maxResistance = resistanceCount
	}

	// 添加选中的支撑位
	for i := 0; i < maxSupport; i++ {
		selectedLevels = append(selectedLevels, supportLevels[i])
	}

	// 添加选中的阻力位
	for i := 0; i < maxResistance; i++ {
		selectedLevels = append(selectedLevels, resistanceLevels[i])
	}

	// 最终按价格排序输出
	sort.Slice(selectedLevels, func(i, j int) bool {
		return selectedLevels[i].Price < selectedLevels[j].Price
	})

	return selectedLevels
}

// calculateStatistics 计算统计信息（简化版）
func (sra *SupportResistanceAnalyzer) calculateStatistics(levels []*SRLevel) *SRStatistics {
	if len(levels) == 0 {
		return &SRStatistics{}
	}

	stats := &SRStatistics{
		TotalLevels: len(levels),
	}

	var totalStrength float64
	var totalHitCount int

	for _, level := range levels {
		totalStrength += level.Strength
		totalHitCount += level.HitCount

		switch level.Type {
		case "support":
			stats.SupportCount++
		case "resistance":
			stats.ResistanceCount++
		}
	}

	stats.AvgStrength = totalStrength / float64(len(levels))
	stats.AvgHitCount = float64(totalHitCount) / float64(len(levels))

	return stats
}

// UpdateConfig 更新配置
func (sra *SupportResistanceAnalyzer) UpdateConfig(config SRConfig) {
	sra.config = config
}

// GetConfig 获取配置
func (sra *SupportResistanceAnalyzer) GetConfig() SRConfig {
	return sra.config
}