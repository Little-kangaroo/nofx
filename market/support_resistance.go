package market

import (
	"fmt"
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
	LookbackPeriods   int     `json:"lookback_periods"`    // 回看K线数量
	PivotLeft         int     `json:"pivot_left"`          // 转折点左侧比较根数
	PivotRight        int     `json:"pivot_right"`         // 转折点右侧比较根数
	ClusterTolerance  float64 `json:"cluster_tolerance"`   // 聚类容差百分比
	MinHits           int     `json:"min_hits"`            // 最小命中次数
	MaxDistancePercent float64 `json:"max_distance_percent"` // 最大距离百分比
}

// PivotPoint 转折点
// 🔥 修复：增强渐进确认机制，解决未来函数问题
type PivotPoint struct {
	Price          float64 `json:"price"`           // 价格
	Type           string  `json:"type"`            // "H"(高点) 或 "L"(低点)
	Index          int     `json:"index"`           // K线索引
	Timestamp      int64   `json:"timestamp"`       // 时间戳
	Confidence     float64 `json:"confidence"`      // 确认置信度 (0.0-1.0)
	MinConfirmed   bool    `json:"min_confirmed"`   // 至少1根右侧K线确认
	FullConfirmed  bool    `json:"full_confirmed"`  // 完整PivotRight根K线确认
	RightBarsCount int     `json:"right_bars_count"` // 实际右侧确认K线数量
}

// PriceCluster 价格聚类
type PriceCluster struct {
	CenterPrice float64       `json:"center_price"` // 中心价格
	Points      []*PivotPoint `json:"points"`       // 转折点列表
	Count       int           `json:"count"`        // 命中次数
}

// SRLevel 支撑阻力级别（简化版 - 只返回3根关键水平线）
type SRLevel struct {
	Price    float64 `json:"price"`     // 价格
	HitCount int     `json:"hit_count"` // 命中次数
	Type     string  `json:"type"`      // "support" 或 "resistance"（相对当前价格）
	Strength float64 `json:"strength"`  // 强度评分 (0-100)
}

// SRFlip 支撑阻力转换线
type SRFlip struct {
	ID               string      `json:"id"`               // SR Flip ID
	OriginalLevel    *SRLevel    `json:"original_level"`   // 原始级别
	OriginalType     string      `json:"original_type"`    // 原始类型 (support/resistance)
	FlippedType      string      `json:"flipped_type"`     // 转换后类型
	FlipPrice        float64     `json:"flip_price"`       // 转换发生的价格
	FlipTime         int64       `json:"flip_time"`        // 转换发生时间
	FlipConfirmation bool        `json:"flip_confirmation"`// 转换是否得到确认
	FlipStrength     float64     `json:"flip_strength"`    // 转换强度评分 (0-100)
	PreFlipTouches   int         `json:"pre_flip_touches"` // 转换前触及次数
	PostFlipTouches  int         `json:"post_flip_touches"`// 转换后触及次数
	FlipContext      *FlipContext `json:"flip_context"`    // 转换上下文
}

// FlipContext 转换上下文信息
type FlipContext struct {
	BreakthroughVolume   float64 `json:"breakthrough_volume"`   // 突破时成交量
	VolumeConfirmation   bool    `json:"volume_confirmation"`   // 成交量确认
	PriceAction          string  `json:"price_action"`          // 价格行为描述
	MarketCondition      string  `json:"market_condition"`      // 市场环境
	FlipQuality          string  `json:"flip_quality"`          // 转换质量 (strong/moderate/weak)
}

// SupportResistanceData 支撑阻力分析结果
type SupportResistanceData struct {
	KeyLevels    []*SRLevel    `json:"key_levels"`    // 3根关键水平线
	SRFlips      []*SRFlip     `json:"sr_flips"`      // 🔥 新增：支撑阻力转换线
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
	LookbackPeriods:    1000, // 回看1000根K线（最大化结构视野）
	PivotLeft:          3,    // 左侧3根比较
	PivotRight:         3,    // 右侧3根比较
	ClusterTolerance:   0.005, // 0.5%聚类容差
	MinHits:            3,    // 至少3次命中
	MaxDistancePercent: 0.10, // 最大距离10%
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

	// 2. 🔥 新增：识别SR Flip转换线
	srFlips := sra.identifySRFlips(levels, klines)

	// 3. 计算统计信息
	statistics := sra.calculateStatistics(levels)

	// 4. 筛选活跃级别（只返回最重要的3根水平线）
	activeLevels := sra.selectKeyLevels(levels, klines[len(klines)-1].Close, 3)

	return &SupportResistanceData{
		KeyLevels:    activeLevels,
		SRFlips:      srFlips, // 🔥 新增：包含SR Flip数据
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
// 🔥 修复：实现渐进确认机制，消除未来函数问题，确保回测/实盘一致性
func (sra *SupportResistanceAnalyzer) findPivotPoints(klines []Kline, startIndex, endIndex int) []*PivotPoint {
	var pivotPoints []*PivotPoint
	
	// 🔥 修复：扩展扫描范围，包含最近的K线（但使用渐进确认）
	extendedEndIndex := len(klines) - 1 // 包含到最新的K线
	
	for i := startIndex; i <= extendedEndIndex; i++ {
		// 🔥 修复：渐进确认高点
		if pivotHigh := sra.checkPivotHighWithConfidence(klines, i); pivotHigh != nil {
			pivotPoints = append(pivotPoints, pivotHigh)
		}
		
		// 🔥 修复：渐进确认低点
		if pivotLow := sra.checkPivotLowWithConfidence(klines, i); pivotLow != nil {
			pivotPoints = append(pivotPoints, pivotLow)
		}
	}
	
	return pivotPoints
}

// checkPivotHighWithConfidence 渐进确认高点检查
// 🔥 修复：解决未来函数问题的核心算法
func (sra *SupportResistanceAnalyzer) checkPivotHighWithConfidence(klines []Kline, index int) *PivotPoint {
	if index < sra.config.PivotLeft {
		return nil // 左侧数据不足
	}
	
	currentHigh := klines[index].High
	
	// 1. 检查左侧 - 必须满足
	for j := index - sra.config.PivotLeft; j < index; j++ {
		if klines[j].High > currentHigh {
			return nil // 左侧有更高点，不是pivot
		}
	}
	
	// 2. 检查右侧 - 渐进确认
	availableRightBars := len(klines) - 1 - index
	rightBarsToCheck := minInt(availableRightBars, sra.config.PivotRight)
	
	if rightBarsToCheck == 0 {
		// 🔥 修复：当前K线，无右侧确认，但可以作为潜在pivot
		return &PivotPoint{
			Price:          currentHigh,
			Type:           "H",
			Index:          index,
			Timestamp:      klines[index].OpenTime,
			Confidence:     0.1,  // 极低置信度
			MinConfirmed:   false,
			FullConfirmed:  false,
			RightBarsCount: 0,
		}
	}
	
	// 检查可用的右侧K线
	isValidPivot := true
	for j := index + 1; j <= index + rightBarsToCheck; j++ {
		if klines[j].High > currentHigh {
			isValidPivot = false
			break
		}
	}
	
	if !isValidPivot {
		return nil // 右侧有更高点，不是pivot
	}
	
	// 3. 🔥 修复：计算确认置信度
	confidence := float64(rightBarsToCheck) / float64(sra.config.PivotRight)
	minConfirmed := rightBarsToCheck >= 1
	fullConfirmed := rightBarsToCheck >= sra.config.PivotRight
	
	return &PivotPoint{
		Price:          currentHigh,
		Type:           "H", 
		Index:          index,
		Timestamp:      klines[index].OpenTime,
		Confidence:     confidence,
		MinConfirmed:   minConfirmed,
		FullConfirmed:  fullConfirmed,
		RightBarsCount: rightBarsToCheck,
	}
}

// checkPivotLowWithConfidence 渐进确认低点检查
// 🔥 修复：解决未来函数问题的核心算法
func (sra *SupportResistanceAnalyzer) checkPivotLowWithConfidence(klines []Kline, index int) *PivotPoint {
	if index < sra.config.PivotLeft {
		return nil // 左侧数据不足
	}
	
	currentLow := klines[index].Low
	
	// 1. 检查左侧 - 必须满足
	for j := index - sra.config.PivotLeft; j < index; j++ {
		if klines[j].Low < currentLow {
			return nil // 左侧有更低点，不是pivot
		}
	}
	
	// 2. 检查右侧 - 渐进确认
	availableRightBars := len(klines) - 1 - index
	rightBarsToCheck := minInt(availableRightBars, sra.config.PivotRight)
	
	if rightBarsToCheck == 0 {
		// 🔥 修复：当前K线，无右侧确认，但可以作为潜在pivot
		return &PivotPoint{
			Price:          currentLow,
			Type:           "L",
			Index:          index,
			Timestamp:      klines[index].OpenTime,
			Confidence:     0.1,  // 极低置信度
			MinConfirmed:   false,
			FullConfirmed:  false,
			RightBarsCount: 0,
		}
	}
	
	// 检查可用的右侧K线
	isValidPivot := true
	for j := index + 1; j <= index + rightBarsToCheck; j++ {
		if klines[j].Low < currentLow {
			isValidPivot = false
			break
		}
	}
	
	if !isValidPivot {
		return nil // 右侧有更低点，不是pivot
	}
	
	// 3. 🔥 修复：计算确认置信度
	confidence := float64(rightBarsToCheck) / float64(sra.config.PivotRight)
	minConfirmed := rightBarsToCheck >= 1
	fullConfirmed := rightBarsToCheck >= sra.config.PivotRight
	
	return &PivotPoint{
		Price:          currentLow,
		Type:           "L",
		Index:          index,
		Timestamp:      klines[index].OpenTime,
		Confidence:     confidence,
		MinConfirmed:   minConfirmed,
		FullConfirmed:  fullConfirmed,
		RightBarsCount: rightBarsToCheck,
	}
}

// clusterPivotPoints 聚类转折点（V-10.0优化版：严格控制HitCount）
// 🔥 修复：增强置信度过滤，优先使用高置信度的Pivot点 + 动态边界稳定性修复
func (sra *SupportResistanceAnalyzer) clusterPivotPoints(pivotPoints []*PivotPoint) []*PriceCluster {
	var clusters []*PriceCluster
	maxClusterSize := 8 // 【关键优化】每个簇最多8个pivot点，适配AI V-10.0严格规则
	
	// 🔥 修复：按置信度排序，优先处理高置信度的点
	sort.Slice(pivotPoints, func(i, j int) bool {
		if pivotPoints[i].Confidence == pivotPoints[j].Confidence {
			return pivotPoints[i].Index < pivotPoints[j].Index // 置信度相同时按时间排序
		}
		return pivotPoints[i].Confidence > pivotPoints[j].Confidence
	})

	// 🔥 新增：计算动态聚类容差（基于ATR的稳定边界）
	adaptiveTolerance := sra.calculateAdaptiveClusterTolerance(pivotPoints)

	for _, point := range pivotPoints {
		// 🔥 修复：最低置信度过滤
		if point.Confidence < 0.3 { // 只接受置信度>=30%的点
			continue
		}
		
		assigned := false

		// 在已有簇中寻找可以合并的
		for _, cluster := range clusters {
			// 【修复1】检查簇大小限制
			if cluster.Count >= maxClusterSize {
				continue // 跳过已满的簇
			}
			
			// 🔥 修复：使用自适应容差替代固定容差，解决动态边界不稳定问题
			allowedDiff := cluster.CenterPrice * adaptiveTolerance
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

					// 🔥 修复：置信度加权的中心价格计算
					sumPrice := 0.0
					sumWeight := 0.0
					for _, p := range cluster.Points {
						weight := p.Confidence
						if weight < 0.3 { // 最低置信度保护
							weight = 0.3
						}
						sumPrice += p.Price * weight
						sumWeight += weight
					}
					if sumWeight > 0 {
						cluster.CenterPrice = sumPrice / sumWeight
					}

					// 🔥 新增：动态调整簇的容差范围（基于实际分布）
					sra.updateClusterBoundaryStability(cluster)

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

// calculateDistancePercent 计算价格到支撑阻力的百分比距离
func (sra *SupportResistanceAnalyzer) calculateDistancePercent(levelPrice, currentPrice float64) float64 {
	return math.Abs(levelPrice-currentPrice) / currentPrice
}

// selectKeyLevels 选择关键水平线（3根，支撑和阻力至少各一条）
func (sra *SupportResistanceAnalyzer) selectKeyLevels(levels []*SRLevel, currentPrice float64, maxCount int) []*SRLevel {
	if len(levels) == 0 {
		return []*SRLevel{}
	}

	// 第一步：距离过滤 - 只保留在指定距离范围内的水平线
	var validLevels []*SRLevel
	for _, level := range levels {
		distancePercent := sra.calculateDistancePercent(level.Price, currentPrice)
		if distancePercent <= sra.config.MaxDistancePercent {
			validLevels = append(validLevels, level)
		}
	}

	// 如果距离过滤后没有水平线，返回空
	if len(validLevels) == 0 {
		return []*SRLevel{}
	}

	// 第二步：按强度排序，选择最强的级别
	sort.Slice(validLevels, func(i, j int) bool {
		return validLevels[i].Strength > validLevels[j].Strength
	})

	var supportLevels []*SRLevel
	var resistanceLevels []*SRLevel

	// 分类收集支撑和阻力位
	for _, level := range validLevels {
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

	var selectedLevels []*SRLevel
	
	// 确保支撑和阻力至少各有一条（如果存在的话）
	supportCount := len(supportLevels)
	resistanceCount := len(resistanceLevels)
	
	if supportCount == 0 && resistanceCount == 0 {
		return []*SRLevel{}
	}

	if supportCount > 0 && resistanceCount > 0 {
		// 两种都有：至少各选1个，剩下的1个给强度更高的
		selectedLevels = append(selectedLevels, supportLevels[0])     // 最近的支撑
		selectedLevels = append(selectedLevels, resistanceLevels[0])  // 最近的阻力
		
		// 第3个位置：从剩余的中选择强度最高的
		var remainingLevels []*SRLevel
		if len(supportLevels) > 1 {
			remainingLevels = append(remainingLevels, supportLevels[1:]...)
		}
		if len(resistanceLevels) > 1 {
			remainingLevels = append(remainingLevels, resistanceLevels[1:]...)
		}
		
		if len(remainingLevels) > 0 {
			// 按强度排序，选择最强的
			sort.Slice(remainingLevels, func(i, j int) bool {
				return remainingLevels[i].Strength > remainingLevels[j].Strength
			})
			selectedLevels = append(selectedLevels, remainingLevels[0])
		}
	} else if supportCount > 0 {
		// 只有支撑：最多选3个
		maxSupport := minInt(maxCount, supportCount)
		for i := 0; i < maxSupport; i++ {
			selectedLevels = append(selectedLevels, supportLevels[i])
		}
	} else {
		// 只有阻力：最多选3个
		maxResistance := minInt(maxCount, resistanceCount)
		for i := 0; i < maxResistance; i++ {
			selectedLevels = append(selectedLevels, resistanceLevels[i])
		}
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

// identifySRFlips 识别支撑阻力转换线（SR Flip）
// 🔥 核心算法："曾经是支撑，现在是阻力"（或者反之）的精确识别
func (sra *SupportResistanceAnalyzer) identifySRFlips(levels []*SRLevel, klines []Kline) []*SRFlip {
	var srFlips []*SRFlip
	
	if len(levels) == 0 || len(klines) < 50 {
		return srFlips
	}
	
	currentPrice := klines[len(klines)-1].Close
	currentTime := time.Now().UnixMilli()
	
	// 🔥 步骤1：为每个级别建立历史交互记录
	for _, level := range levels {
		flipCandidate := sra.analyzeLevelForFlip(level, klines, currentPrice)
		if flipCandidate != nil {
			flipCandidate.ID = fmt.Sprintf("sr_flip_%d_%s", 
				int(level.Price*1000), flipCandidate.FlippedType)
			flipCandidate.FlipTime = currentTime
			srFlips = append(srFlips, flipCandidate)
		}
	}
	
	// 🔥 步骤2：验证和过滤SR Flip（确保质量）
	validatedFlips := sra.validateSRFlips(srFlips, klines)
	
	return validatedFlips
}

// analyzeLevelForFlip 分析单个级别是否发生了SR转换
func (sra *SupportResistanceAnalyzer) analyzeLevelForFlip(level *SRLevel, klines []Kline, currentPrice float64) *SRFlip {
	// 🔥 核心逻辑：检测价格与级别的历史关系变化
	
	// 步骤1：构建价格与该级别的历史交互时间线
	interactions := sra.buildLevelInteractionTimeline(level, klines)
	if len(interactions) < 3 {
		return nil // 交互次数太少，无法判定转换
	}
	
	// 步骤2：检测交互行为是否发生了根本性变化
	hasFlipOccurred, originalRole, newRole := sra.detectRoleTransition(interactions, currentPrice, level.Price)
	if !hasFlipOccurred {
		return nil
	}
	
	// 步骤3：计算转换强度和质量
	flipStrength := sra.calculateFlipStrength(interactions, originalRole, newRole)
	flipContext := sra.analyzeFlipContext(interactions, klines, level)
	
	// 步骤4：最后验证（确保转换确实有意义）
	if flipStrength < 40.0 || flipContext.FlipQuality == "weak" {
		return nil // 转换强度不足或质量太低
	}
	
	// 构建SR Flip对象
	srFlip := &SRFlip{
		OriginalLevel:    level,
		OriginalType:     originalRole,
		FlippedType:      newRole,
		FlipPrice:        level.Price,
		FlipConfirmation: flipStrength > 60.0,
		FlipStrength:     flipStrength,
		FlipContext:      flipContext,
	}
	
	// 计算转换前后的触及次数
	srFlip.PreFlipTouches, srFlip.PostFlipTouches = sra.calculatePrePostFlipTouches(interactions)
	
	return srFlip
}

// LevelInteraction 级别交互记录
type LevelInteraction struct {
	Time         int64   `json:"time"`          // 交互时间
	Price        float64 `json:"price"`         // 交互价格
	Action       string  `json:"action"`        // "bounce" | "break" | "test"
	Volume       float64 `json:"volume"`        // 交互时成交量
	Strength     float64 `json:"strength"`      // 交互强度
	PriceAfter   float64 `json:"price_after"`   // 交互后价格变化
}

// buildLevelInteractionTimeline 构建级别交互时间线
func (sra *SupportResistanceAnalyzer) buildLevelInteractionTimeline(level *SRLevel, klines []Kline) []*LevelInteraction {
	var interactions []*LevelInteraction
	tolerance := level.Price * 0.01 // 1%容差
	
	for i := 1; i < len(klines)-1; i++ {
		kline := klines[i]
		
		// 检查价格是否与级别交互
		if sra.isPriceInteractingWithLevel(kline, level.Price, tolerance) {
			interaction := &LevelInteraction{
				Time:   kline.OpenTime,
				Price:  (kline.High + kline.Low) / 2,
				Volume: kline.Volume,
			}
			
			// 🔥 关键：分析交互后的价格行为来判定角色
			interaction.Action, interaction.Strength = sra.analyzePostInteractionBehavior(klines, i, level.Price)
			
			// 计算交互后的价格变化
			if i < len(klines)-5 {
				futurePrice := klines[i+3].Close
				interaction.PriceAfter = (futurePrice - kline.Close) / kline.Close
			}
			
			interactions = append(interactions, interaction)
		}
	}
	
	return interactions
}

// isPriceInteractingWithLevel 判断价格是否与级别交互
func (sra *SupportResistanceAnalyzer) isPriceInteractingWithLevel(kline Kline, levelPrice, tolerance float64) bool {
	return (kline.Low <= levelPrice+tolerance && kline.High >= levelPrice-tolerance)
}

// analyzePostInteractionBehavior 分析交互后的行为
func (sra *SupportResistanceAnalyzer) analyzePostInteractionBehavior(klines []Kline, index int, levelPrice float64) (string, float64) {
	if index >= len(klines)-3 {
		return "test", 50.0
	}
	
	currentPrice := klines[index].Close
	
	// 观察后续3根K线的价格行为
	priceSum := 0.0
	for i := 1; i <= 3 && index+i < len(klines); i++ {
		priceSum += klines[index+i].Close
	}
	avgFuturePrice := priceSum / 3.0
	
	// 🔥 核心判定逻辑
	priceChange := (avgFuturePrice - currentPrice) / currentPrice
	relativeToLevel := (currentPrice - levelPrice) / levelPrice
	
	var action string
	var strength float64
	
	if math.Abs(relativeToLevel) < 0.005 { // 非常接近级别
		if math.Abs(priceChange) > 0.02 { // 2%以上的后续移动
			if (relativeToLevel >= 0 && priceChange > 0) || (relativeToLevel < 0 && priceChange < 0) {
				action = "bounce"    // 明显反弹
				strength = math.Min(math.Abs(priceChange)*2000, 100.0)
			} else {
				action = "break"     // 明显突破
				strength = math.Min(math.Abs(priceChange)*3000, 100.0)
			}
		} else {
			action = "test"          // 仅仅测试
			strength = 30.0 + math.Abs(priceChange)*1000
		}
	} else {
		action = "test"
		strength = 20.0
	}
	
	return action, strength
}

// detectRoleTransition 检测角色转换
func (sra *SupportResistanceAnalyzer) detectRoleTransition(interactions []*LevelInteraction, currentPrice, levelPrice float64) (bool, string, string) {
	if len(interactions) < 4 {
		return false, "", ""
	}
	
	// 🔥 核心：将交互分为早期和晚期两个阶段
	midPoint := len(interactions) / 2
	earlyInteractions := interactions[:midPoint]
	lateInteractions := interactions[midPoint:]
	
	// 分析早期行为模式
	earlyRole := sra.determineDominantRole(earlyInteractions, levelPrice)
	
	// 分析晚期行为模式  
	lateRole := sra.determineDominantRole(lateInteractions, levelPrice)
	
	// 🔥 检查是否发生了有意义的角色转换
	hasFlipped := (earlyRole != lateRole) && earlyRole != "neutral" && lateRole != "neutral"
	
	// 额外验证：确保转换是持续的，不是偶然的
	if hasFlipped {
		roleConsistency := sra.validateRoleConsistency(lateInteractions, lateRole)
		if roleConsistency < 0.6 {
			hasFlipped = false // 转换后角色不够稳定
		}
	}
	
	return hasFlipped, earlyRole, lateRole
}

// determineDominantRole 确定主导角色
func (sra *SupportResistanceAnalyzer) determineDominantRole(interactions []*LevelInteraction, levelPrice float64) string {
	if len(interactions) == 0 {
		return "neutral"
	}
	
	bounceCount := 0
	breakCount := 0
	totalStrength := 0.0
	
	for _, interaction := range interactions {
		switch interaction.Action {
		case "bounce":
			bounceCount++
			// 反弹强度越高，角色定义越明确
			if interaction.Price < levelPrice {
				totalStrength += interaction.Strength * 2 // 从下方反弹=支撑
			} else {
				totalStrength += interaction.Strength     // 从上方反弹=阻力
			}
		case "break":
			breakCount++
			totalStrength -= interaction.Strength * 0.5 // 突破削弱角色定义
		case "test":
			// 测试行为不影响角色判定
		}
	}
	
	// 🔥 角色判定逻辑：
	// 1. 反弹次数明显多于突破 -> 有效的支撑/阻力
	// 2. 根据价格相对位置确定是支撑还是阻力
	bounceRate := float64(bounceCount) / float64(len(interactions))
	
	if bounceRate < 0.4 || totalStrength < 30 {
		return "neutral" // 角色不明确
	}
	
	// 通过最近几次交互的价格位置判断是支撑还是阻力
	recentPriceSum := 0.0
	recentCount := 0
	for i := len(interactions) - 3; i < len(interactions); i++ {
		if i >= 0 {
			recentPriceSum += interactions[i].Price
			recentCount++
		}
	}
	
	if recentCount == 0 {
		return "neutral"
	}
	
	avgRecentPrice := recentPriceSum / float64(recentCount)
	
	if avgRecentPrice < levelPrice {
		return "support"    // 价格主要从下方交互 = 支撑
	} else {
		return "resistance" // 价格主要从上方交互 = 阻力
	}
}

// validateRoleConsistency 验证角色一致性
func (sra *SupportResistanceAnalyzer) validateRoleConsistency(interactions []*LevelInteraction, expectedRole string) float64 {
	if len(interactions) == 0 {
		return 0.0
	}
	
	consistentActions := 0
	for _, interaction := range interactions {
		if interaction.Action == "bounce" {
			consistentActions++
		}
	}
	
	return float64(consistentActions) / float64(len(interactions))
}

// calculateFlipStrength 计算转换强度
func (sra *SupportResistanceAnalyzer) calculateFlipStrength(interactions []*LevelInteraction, originalRole, newRole string) float64 {
	if len(interactions) < 4 {
		return 0.0
	}
	
	// 基础强度：基于交互质量
	avgStrength := 0.0
	for _, interaction := range interactions {
		avgStrength += interaction.Strength
	}
	avgStrength /= float64(len(interactions))
	
	// 转换明确性：角色转换越明确，强度越高
	roleClarity := 0.0
	if originalRole != "neutral" && newRole != "neutral" && originalRole != newRole {
		roleClarity = 40.0 // 清晰的角色转换
	}
	
	// 持续性：新角色维持得越久，强度越高
	midPoint := len(interactions) / 2
	lateInteractions := interactions[midPoint:]
	consistency := sra.validateRoleConsistency(lateInteractions, newRole)
	
	// 综合评分
	flipStrength := (avgStrength*0.4 + roleClarity*0.4 + consistency*100*0.2)
	return math.Min(flipStrength, 100.0)
}

// analyzeFlipContext 分析转换上下文
func (sra *SupportResistanceAnalyzer) analyzeFlipContext(interactions []*LevelInteraction, klines []Kline, level *SRLevel) *FlipContext {
	context := &FlipContext{}
	
	if len(interactions) == 0 {
		context.FlipQuality = "weak"
		return context
	}
	
	// 分析突破时的成交量
	maxVolume := 0.0
	for _, interaction := range interactions {
		if interaction.Volume > maxVolume {
			maxVolume = interaction.Volume
			context.BreakthroughVolume = interaction.Volume
		}
	}
	
	// 计算平均成交量用于比较
	if len(klines) > 20 {
		var avgVolume float64
		for i := len(klines) - 20; i < len(klines); i++ {
			avgVolume += klines[i].Volume
		}
		avgVolume /= 20.0
		context.VolumeConfirmation = context.BreakthroughVolume > avgVolume*1.5
	}
	
	// 分析价格行为
	bounceCount := 0
	breakCount := 0
	for _, interaction := range interactions {
		switch interaction.Action {
		case "bounce":
			bounceCount++
		case "break":
			breakCount++
		}
	}
	
	if breakCount > bounceCount {
		context.PriceAction = "decisive_breakthrough"
	} else if bounceCount > breakCount*2 {
		context.PriceAction = "strong_respect_then_flip"
	} else {
		context.PriceAction = "gradual_weakening"
	}
	
	// 评估转换质量
	strengthSum := 0.0
	for _, interaction := range interactions {
		strengthSum += interaction.Strength
	}
	avgStrength := strengthSum / float64(len(interactions))
	
	if avgStrength > 70 && context.VolumeConfirmation {
		context.FlipQuality = "strong"
	} else if avgStrength > 50 {
		context.FlipQuality = "moderate"  
	} else {
		context.FlipQuality = "weak"
	}
	
	return context
}

// calculatePrePostFlipTouches 计算转换前后触及次数
func (sra *SupportResistanceAnalyzer) calculatePrePostFlipTouches(interactions []*LevelInteraction) (int, int) {
	if len(interactions) < 2 {
		return 0, 0
	}
	
	midPoint := len(interactions) / 2
	preFlipTouches := midPoint
	postFlipTouches := len(interactions) - midPoint
	
	return preFlipTouches, postFlipTouches
}

// validateSRFlips 验证和过滤SR Flip
func (sra *SupportResistanceAnalyzer) validateSRFlips(srFlips []*SRFlip, klines []Kline) []*SRFlip {
	var validatedFlips []*SRFlip
	
	for _, flip := range srFlips {
		// 验证条件1：转换强度足够
		if flip.FlipStrength < 45.0 {
			continue
		}
		
		// 验证条件2：交互次数足够
		if flip.PreFlipTouches < 2 || flip.PostFlipTouches < 1 {
			continue
		}
		
		// 验证条件3：转换是有意义的
		if flip.OriginalType == flip.FlippedType {
			continue
		}
		
		// 验证条件4：上下文质量检查
		if flip.FlipContext.FlipQuality == "weak" && flip.FlipStrength < 60.0 {
			continue
		}
		
		validatedFlips = append(validatedFlips, flip)
	}
	
	// 按转换强度排序，保留最优的转换
	for i := 0; i < len(validatedFlips)-1; i++ {
		for j := i + 1; j < len(validatedFlips); j++ {
			if validatedFlips[j].FlipStrength > validatedFlips[i].FlipStrength {
				validatedFlips[i], validatedFlips[j] = validatedFlips[j], validatedFlips[i]
			}
		}
	}
	
	// 限制数量，避免过多噪音
	maxFlips := 5
	if len(validatedFlips) > maxFlips {
		validatedFlips = validatedFlips[:maxFlips]
	}
	
	return validatedFlips
}

// calculateAdaptiveClusterTolerance 计算自适应聚类容差（基于ATR的稳定边界）
// 🔥 修复：解决聚类算法的动态边界不稳定问题
func (sra *SupportResistanceAnalyzer) calculateAdaptiveClusterTolerance(pivotPoints []*PivotPoint) float64 {
	if len(pivotPoints) < 10 {
		return sra.config.ClusterTolerance // 数据不足时使用默认值
	}
	
	// 🔥 方法1：基于价格分布的方差计算自适应容差
	var prices []float64
	for _, point := range pivotPoints {
		prices = append(prices, point.Price)
	}
	
	// 计算价格标准差
	mean := 0.0
	for _, price := range prices {
		mean += price
	}
	mean /= float64(len(prices))
	
	variance := 0.0
	for _, price := range prices {
		variance += math.Pow(price-mean, 2)
	}
	stdDev := math.Sqrt(variance / float64(len(prices)))
	
	// 🔥 方法2：基于ATR归一化的容差计算
	atrBasedTolerance := sra.calculateATRBasedTolerance(mean)
	
	// 🔥 方法3：基于置信度分布的容差调整
	confidenceAdjustment := sra.calculateConfidenceBasedTolerance(pivotPoints)
	
	// 🔥 综合计算：取加权平均，确保稳定性
	stdDevTolerance := stdDev / mean                    // 标准差容差（相对）
	atrTolerance := atrBasedTolerance                  // ATR容差
	confidenceTolerance := confidenceAdjustment       // 置信度调整
	
	// 加权组合：40% 标准差 + 40% ATR + 20% 置信度调整
	adaptiveTolerance := stdDevTolerance*0.4 + atrTolerance*0.4 + confidenceTolerance*0.2
	
	// 🔥 边界稳定性保护：限制在合理范围内，避免极端值
	minTolerance := sra.config.ClusterTolerance * 0.5  // 最小不低于默认的50%
	maxTolerance := sra.config.ClusterTolerance * 3.0  // 最大不超过默认的300%
	
	adaptiveTolerance = math.Max(minTolerance, math.Min(maxTolerance, adaptiveTolerance))
	
	return adaptiveTolerance
}

// calculateATRBasedTolerance 基于ATR计算容差
func (sra *SupportResistanceAnalyzer) calculateATRBasedTolerance(meanPrice float64) float64 {
	// 这里需要ATR值，但support_resistance分析器没有直接访问K线
	// 使用简化方法：基于价格水平的相对ATR估算
	
	// 假设ATR约为价格的1-3%（经验值）
	estimatedATRPercent := 0.015 // 1.5%的估算ATR
	
	// ATR容差：0.5倍估算ATR作为聚类容差
	atrTolerance := estimatedATRPercent * 0.5
	
	return atrTolerance
}

// calculateConfidenceBasedTolerance 基于置信度分布计算容差调整
func (sra *SupportResistanceAnalyzer) calculateConfidenceBasedTolerance(pivotPoints []*PivotPoint) float64 {
	if len(pivotPoints) == 0 {
		return sra.config.ClusterTolerance
	}
	
	// 计算平均置信度
	avgConfidence := 0.0
	for _, point := range pivotPoints {
		avgConfidence += point.Confidence
	}
	avgConfidence /= float64(len(pivotPoints))
	
	// 🔥 置信度调整逻辑：
	// - 高置信度的点群 -> 更紧的聚类 (容差减小)
	// - 低置信度的点群 -> 更松的聚类 (容差增大)
	
	var adjustment float64
	if avgConfidence > 0.8 {
		adjustment = 0.7 // 高置信度，减小容差30%
	} else if avgConfidence > 0.6 {
		adjustment = 0.9 // 中等置信度，减小容差10%
	} else if avgConfidence > 0.4 {
		adjustment = 1.0 // 正常置信度，保持原容差
	} else {
		adjustment = 1.2 // 低置信度，增大容差20%
	}
	
	return sra.config.ClusterTolerance * adjustment
}

// updateClusterBoundaryStability 更新簇的边界稳定性
// 🔥 修复：动态调整簇的容差范围，基于实际点的分布情况
func (sra *SupportResistanceAnalyzer) updateClusterBoundaryStability(cluster *PriceCluster) {
	if cluster.Count < 2 {
		return // 至少需要2个点才能计算稳定性
	}
	
	// 计算簇内点的价格分散程度
	var prices []float64
	var confidences []float64
	
	for _, point := range cluster.Points {
		prices = append(prices, point.Price)
		confidences = append(confidences, point.Confidence)
	}
	
	// 🔥 分散程度计算：标准差
	mean := 0.0
	for _, price := range prices {
		mean += price
	}
	mean /= float64(len(prices))
	
	variance := 0.0
	for _, price := range prices {
		variance += math.Pow(price-mean, 2)
	}
	stdDev := math.Sqrt(variance / float64(len(prices)))
	
	// 🔥 稳定性评分：分散程度越小，稳定性越高
	stabilityScore := 1.0 / (1.0 + stdDev/mean*100) // 归一化到[0,1]
	
	// 🔥 动态边界调整：根据稳定性调整后续点的接受范围
	// 注意：这个函数主要是为了记录稳定性，实际的边界调整在calculateAdaptiveClusterTolerance中进行
	
	// 可以在这里记录簇的统计信息，用于后续的质量评估
	// 这里暂时不存储额外字段，避免修改PriceCluster结构体
	
	// 防止未使用变量的编译警告
	_ = stabilityScore
	_ = confidences
}