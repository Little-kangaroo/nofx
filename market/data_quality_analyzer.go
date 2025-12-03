package market

import (
	"encoding/json"
	"log"
	"math"
	"time"
)

// DataQualityMetrics 数据质量指标 - P2修复最终组件
type DataQualityMetrics struct {
	OverallQualityScore  float64                   `json:"overall_quality_score"`  // 总体质量评分 (0-100)
	ZoneQualityStats     *ZoneQualityStatistics    `json:"zone_quality_stats"`     // 区域质量统计
	CleaningEfficiency   *CleaningEfficiencyStats  `json:"cleaning_efficiency"`    // 清洗效率统计
	OutlierDistribution  *OutlierDistributionStats `json:"outlier_distribution"`   // 异常值分布统计
	QualityTrendAnalysis *QualityTrendAnalysis     `json:"quality_trend_analysis"` // 质量趋势分析
	RecommendedActions   []string                  `json:"recommended_actions"`    // 推荐的改进措施
	Timestamp            int64                     `json:"timestamp"`              // 分析时间戳
}

// ZoneQualityStatistics 区域质量统计
type ZoneQualityStatistics struct {
	TotalZonesAnalyzed     int     `json:"total_zones_analyzed"`     // 总分析区域数
	HighQualityZones       int     `json:"high_quality_zones"`       // 高质量区域数
	MediumQualityZones     int     `json:"medium_quality_zones"`     // 中质量区域数
	LowQualityZones        int     `json:"low_quality_zones"`        // 低质量区域数
	AverageZoneScore       float64 `json:"average_zone_score"`       // 平均区域评分
	ScoreStandardDeviation float64 `json:"score_standard_deviation"` // 评分标准差
	QualityConsistency     float64 `json:"quality_consistency"`      // 质量一致性 (0-100)
}

// CleaningEfficiencyStats 清洗效率统计
type CleaningEfficiencyStats struct {
	FilteredRatePercent      float64 `json:"filtered_rate_percent"`       // 过滤率百分比
	OptimalFilterRate        float64 `json:"optimal_filter_rate"`         // 最优过滤率
	EfficiencyScore          float64 `json:"efficiency_score"`            // 效率评分 (0-100)
	FalsePositiveRate        float64 `json:"false_positive_rate"`         // 误杀率估算
	FalseNegativeRate        float64 `json:"false_negative_rate"`         // 漏检率估算
	CleaningRecommendation   string  `json:"cleaning_recommendation"`     // 清洗建议
}

// OutlierDistributionStats 异常值分布统计
type OutlierDistributionStats struct {
	WidthATROutliers    *OutlierTypeStats `json:"width_atr_outliers"`    // WidthATR异常值统计
	VolRatioOutliers    *OutlierTypeStats `json:"vol_ratio_outliers"`    // VolRatio异常值统计
	SeasonalPatterns    []string          `json:"seasonal_patterns"`     // 季节性异常模式
	AnomalyConcentration float64           `json:"anomaly_concentration"` // 异常集中度
}

// OutlierTypeStats 特定类型异常值统计
type OutlierTypeStats struct {
	Count             int     `json:"count"`               // 异常值数量
	SeverityDistrib   map[string]int `json:"severity_distrib"`    // 严重程度分布
	AverageDeviation  float64 `json:"average_deviation"`   // 平均偏离度
	MaxDeviation      float64 `json:"max_deviation"`       // 最大偏离度
	DetectionMethods  map[string]int `json:"detection_methods"`   // 检测方法分布
}

// QualityTrendAnalysis 质量趋势分析
type QualityTrendAnalysis struct {
	QualityTrend        string  `json:"quality_trend"`         // "improving", "stable", "degrading"
	TrendStrength       float64 `json:"trend_strength"`        // 趋势强度 (0-100)
	PredictedNextScore  float64 `json:"predicted_next_score"`  // 预测下次评分
	ConfidenceLevel     float64 `json:"confidence_level"`      // 预测置信度
	RecommendedInterval int     `json:"recommended_interval"`  // 推荐清洗间隔(分钟)
}

// DataQualityAnalyzer 数据质量分析器
type DataQualityAnalyzer struct {
	historicalScores   []float64    // 历史质量评分
	historicalTimings  []int64      // 历史分析时间
	maxHistorySize     int          // 最大历史记录数
}

// NewDataQualityAnalyzer 创建数据质量分析器
func NewDataQualityAnalyzer() *DataQualityAnalyzer {
	return &DataQualityAnalyzer{
		historicalScores:  make([]float64, 0),
		historicalTimings: make([]int64, 0), 
		maxHistorySize:    50, // 保留最近50次分析记录
	}
}

// AnalyzeDataQuality 分析数据质量并生成完整报告
func (dqa *DataQualityAnalyzer) AnalyzeDataQuality(
	originalData *SupplyDemandData,
	cleanedData *SupplyDemandData, 
	cleaningStats *CleaningStats,
	outliers []OutlierInfo,
) *DataQualityMetrics {
	log.Printf("📊 [数据质量分析] 开始生成质量报告...")
	
	// 计算总体质量评分
	overallScore := dqa.calculateOverallQualityScore(cleanedData, cleaningStats)
	
	// 更新历史记录
	dqa.updateHistoricalData(overallScore)
	
	// 生成各项统计
	zoneQualityStats := dqa.analyzeZoneQuality(originalData, cleanedData)
	cleaningEfficiency := dqa.analyzeCleaningEfficiency(cleaningStats)
	outlierDistribution := dqa.analyzeOutlierDistribution(outliers)
	qualityTrend := dqa.analyzeQualityTrend()
	
	// 生成改进建议
	recommendations := dqa.generateRecommendations(
		zoneQualityStats, cleaningEfficiency, outlierDistribution, qualityTrend,
	)
	
	metrics := &DataQualityMetrics{
		OverallQualityScore:  overallScore,
		ZoneQualityStats:     zoneQualityStats,
		CleaningEfficiency:   cleaningEfficiency,
		OutlierDistribution:  outlierDistribution,
		QualityTrendAnalysis: qualityTrend,
		RecommendedActions:   recommendations,
		Timestamp:            time.Now().UnixMilli(),
	}
	
	// 记录详细日志
	dqa.logQualityReport(metrics)
	
	return metrics
}

// calculateOverallQualityScore 计算总体质量评分
func (dqa *DataQualityAnalyzer) calculateOverallQualityScore(
	cleanedData *SupplyDemandData, 
	cleaningStats *CleaningStats,
) float64 {
	if cleanedData == nil || cleaningStats == nil {
		return 0.0
	}
	
	score := 0.0
	
	// 1. 基础清洗质量评分 (40分)
	score += cleaningStats.QualityScore * 0.4
	
	// 2. 过滤率合理性评分 (25分)
	filterRateScore := dqa.calculateFilterRateScore(cleaningStats.FilterRate)
	score += filterRateScore * 0.25
	
	// 3. 区域数量充足性评分 (20分)
	zoneCountScore := dqa.calculateZoneCountScore(len(cleanedData.ActiveZones))
	score += zoneCountScore * 0.20
	
	// 4. 数据一致性评分 (15分)
	consistencyScore := dqa.calculateConsistencyScore(cleanedData)
	score += consistencyScore * 0.15
	
	return math.Min(score, 100.0)
}

// calculateFilterRateScore 计算过滤率评分
func (dqa *DataQualityAnalyzer) calculateFilterRateScore(filterRate float64) float64 {
	// 理想过滤率范围：5%-25%
	if filterRate >= 5.0 && filterRate <= 25.0 {
		return 100.0 // 理想范围
	} else if filterRate < 5.0 {
		// 过滤太少，可能有遗漏
		return 100.0 - (5.0-filterRate)*5 // 每少1%扣5分
	} else {
		// 过滤太多，可能过度清洗
		return 100.0 - (filterRate-25.0)*3 // 每多1%扣3分
	}
}

// calculateZoneCountScore 计算区域数量评分
func (dqa *DataQualityAnalyzer) calculateZoneCountScore(zoneCount int) float64 {
	// 理想区域数量：8-20个
	if zoneCount >= 8 && zoneCount <= 20 {
		return 100.0
	} else if zoneCount < 8 {
		// 区域太少
		return float64(zoneCount) / 8.0 * 100.0
	} else {
		// 区域太多，可能有噪音
		return 100.0 - float64(zoneCount-20)*2
	}
}

// calculateConsistencyScore 计算一致性评分
func (dqa *DataQualityAnalyzer) calculateConsistencyScore(data *SupplyDemandData) float64 {
	if len(data.ActiveZones) == 0 {
		return 0.0
	}
	
	// 计算强度一致性
	strengths := make([]float64, 0)
	for _, zone := range data.ActiveZones {
		strengths = append(strengths, zone.Strength)
	}
	
	if len(strengths) == 0 {
		return 50.0
	}
	
	// 计算标准差
	mean := 0.0
	for _, s := range strengths {
		mean += s
	}
	mean /= float64(len(strengths))
	
	variance := 0.0
	for _, s := range strengths {
		diff := s - mean
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(strengths)))
	
	// 标准差越小，一致性越好
	consistencyScore := 100.0 - math.Min(stdDev, 50.0)
	return math.Max(consistencyScore, 0.0)
}

// updateHistoricalData 更新历史数据
func (dqa *DataQualityAnalyzer) updateHistoricalData(score float64) {
	dqa.historicalScores = append(dqa.historicalScores, score)
	dqa.historicalTimings = append(dqa.historicalTimings, time.Now().UnixMilli())
	
	// 保持历史记录数量限制
	if len(dqa.historicalScores) > dqa.maxHistorySize {
		dqa.historicalScores = dqa.historicalScores[1:]
		dqa.historicalTimings = dqa.historicalTimings[1:]
	}
}

// analyzeZoneQuality 分析区域质量
func (dqa *DataQualityAnalyzer) analyzeZoneQuality(
	originalData, cleanedData *SupplyDemandData,
) *ZoneQualityStatistics {
	totalAnalyzed := len(originalData.ActiveZones)
	scores := make([]float64, 0)
	highCount, mediumCount, lowCount := 0, 0, 0
	
	for _, zone := range cleanedData.ActiveZones {
		if zone.Context != nil {
			// 基于上下文计算质量评分
			score := dqa.calculateZoneQualityScore(zone)
			scores = append(scores, score)
			
			if score >= 80 {
				highCount++
			} else if score >= 60 {
				mediumCount++
			} else {
				lowCount++
			}
		}
	}
	
	// 计算平均分和标准差
	avgScore := 0.0
	if len(scores) > 0 {
		for _, s := range scores {
			avgScore += s
		}
		avgScore /= float64(len(scores))
	}
	
	stdDev := 0.0
	if len(scores) > 1 {
		variance := 0.0
		for _, s := range scores {
			diff := s - avgScore
			variance += diff * diff
		}
		stdDev = math.Sqrt(variance / float64(len(scores)-1))
	}
	
	// 计算质量一致性
	consistency := 100.0 - math.Min(stdDev*2, 100.0)
	
	return &ZoneQualityStatistics{
		TotalZonesAnalyzed:     totalAnalyzed,
		HighQualityZones:       highCount,
		MediumQualityZones:     mediumCount,
		LowQualityZones:        lowCount,
		AverageZoneScore:       avgScore,
		ScoreStandardDeviation: stdDev,
		QualityConsistency:     consistency,
	}
}

// calculateZoneQualityScore 计算单个区域质量评分
func (dqa *DataQualityAnalyzer) calculateZoneQualityScore(zone *SupplyDemandZone) float64 {
	if zone.Context == nil {
		return 0.0
	}
	
	score := 0.0
	
	// WidthATR评分 (40分)
	widthScore := 0.0
	if zone.Context.WidthATR >= 0.5 && zone.Context.WidthATR <= 2.0 {
		widthScore = 40.0
	} else if zone.Context.WidthATR >= 0.2 && zone.Context.WidthATR <= 3.0 {
		widthScore = 30.0
	} else {
		widthScore = 10.0
	}
	score += widthScore
	
	// VolRatio评分 (30分)
	volScore := 0.0
	if zone.Context.VolRatio >= 1.0 && zone.Context.VolRatio <= 5.0 {
		volScore = 30.0
	} else if zone.Context.VolRatio >= 0.5 && zone.Context.VolRatio <= 8.0 {
		volScore = 20.0
	} else {
		volScore = 5.0
	}
	score += volScore
	
	// 强度评分 (30分) 
	if zone.Strength >= 80 {
		score += 30.0
	} else if zone.Strength >= 60 {
		score += 20.0
	} else if zone.Strength >= 40 {
		score += 10.0
	}
	
	return score
}

// analyzeCleaningEfficiency 分析清洗效率
func (dqa *DataQualityAnalyzer) analyzeCleaningEfficiency(
	cleaningStats *CleaningStats,
) *CleaningEfficiencyStats {
	filterRate := cleaningStats.FilterRate
	optimalRate := 15.0 // 理想过滤率15%
	
	// 计算效率评分
	efficiencyScore := dqa.calculateFilterRateScore(filterRate)
	
	// 估算误杀率和漏检率
	falsePositiveRate := 0.0
	falseNegativeRate := 0.0
	
	if filterRate > 30 {
		falsePositiveRate = (filterRate - 30) * 0.5 // 过度清洗时的误杀率
	}
	
	if filterRate < 5 {
		falseNegativeRate = (5 - filterRate) * 0.3 // 清洗不足时的漏检率
	}
	
	// 生成建议
	recommendation := "当前清洗效果良好"
	if filterRate > 30 {
		recommendation = "过滤过于严格，建议放宽清洗参数"
	} else if filterRate < 5 {
		recommendation = "过滤过于宽松，建议加强清洗参数"
	} else if filterRate > 25 {
		recommendation = "过滤稍显严格，可适当调整"
	}
	
	return &CleaningEfficiencyStats{
		FilteredRatePercent:      filterRate,
		OptimalFilterRate:        optimalRate,
		EfficiencyScore:          efficiencyScore,
		FalsePositiveRate:        falsePositiveRate,
		FalseNegativeRate:        falseNegativeRate,
		CleaningRecommendation:   recommendation,
	}
}

// analyzeOutlierDistribution 分析异常值分布
func (dqa *DataQualityAnalyzer) analyzeOutlierDistribution(
	outliers []OutlierInfo,
) *OutlierDistributionStats {
	widthATRStats := &OutlierTypeStats{
		SeverityDistrib:  make(map[string]int),
		DetectionMethods: make(map[string]int),
	}
	
	volRatioStats := &OutlierTypeStats{
		SeverityDistrib:  make(map[string]int),
		DetectionMethods: make(map[string]int),
	}
	
	var widthATRDeviations, volRatioDeviations []float64
	
	// 分析异常值
	for _, outlier := range outliers {
		if outlier.Type == "width_atr" {
			widthATRStats.Count++
			widthATRStats.SeverityDistrib[outlier.Severity]++
			widthATRStats.DetectionMethods[outlier.Method]++
			widthATRDeviations = append(widthATRDeviations, outlier.Value)
		} else if outlier.Type == "vol_ratio" {
			volRatioStats.Count++
			volRatioStats.SeverityDistrib[outlier.Severity]++
			volRatioStats.DetectionMethods[outlier.Method]++
			volRatioDeviations = append(volRatioDeviations, outlier.Value)
		}
	}
	
	// 计算统计数据
	dqa.calculateOutlierStats(widthATRStats, widthATRDeviations)
	dqa.calculateOutlierStats(volRatioStats, volRatioDeviations)
	
	// 计算异常集中度
	totalOutliers := len(outliers)
	concentration := 0.0
	if totalOutliers > 0 {
		concentration = float64(totalOutliers) / 50.0 * 100 // 假设基准50个区域
	}
	
	return &OutlierDistributionStats{
		WidthATROutliers:     widthATRStats,
		VolRatioOutliers:     volRatioStats,
		SeasonalPatterns:     []string{}, // TODO: 季节性分析
		AnomalyConcentration: concentration,
	}
}

// calculateOutlierStats 计算异常值统计
func (dqa *DataQualityAnalyzer) calculateOutlierStats(
	stats *OutlierTypeStats, 
	deviations []float64,
) {
	if len(deviations) == 0 {
		return
	}
	
	// 计算平均偏离度
	sum := 0.0
	max := deviations[0]
	for _, d := range deviations {
		sum += d
		if d > max {
			max = d
		}
	}
	
	stats.AverageDeviation = sum / float64(len(deviations))
	stats.MaxDeviation = max
}

// analyzeQualityTrend 分析质量趋势
func (dqa *DataQualityAnalyzer) analyzeQualityTrend() *QualityTrendAnalysis {
	if len(dqa.historicalScores) < 3 {
		return &QualityTrendAnalysis{
			QualityTrend:        "insufficient_data",
			TrendStrength:       0.0,
			PredictedNextScore:  0.0,
			ConfidenceLevel:     0.0,
			RecommendedInterval: 60, // 默认60分钟
		}
	}
	
	// 简单线性趋势分析
	scores := dqa.historicalScores
	n := len(scores)
	recent := scores[n-5:] // 取最近5次记录
	
	if len(recent) < 2 {
		recent = scores
	}
	
	// 计算趋势
	trend := "stable"
	trendStrength := 0.0
	
	if len(recent) >= 2 {
		firstHalf := recent[:len(recent)/2]
		secondHalf := recent[len(recent)/2:]
		
		avgFirst := dqa.average(firstHalf)
		avgSecond := dqa.average(secondHalf)
		
		diff := avgSecond - avgFirst
		trendStrength = math.Abs(diff)
		
		if diff > 2 {
			trend = "improving"
		} else if diff < -2 {
			trend = "degrading"
		}
	}
	
	// 预测下次评分
	currentScore := scores[n-1]
	predictedScore := currentScore
	confidence := 50.0
	
	if trendStrength > 0 {
		if trend == "improving" {
			predictedScore = math.Min(currentScore+trendStrength*0.5, 100)
		} else if trend == "degrading" {
			predictedScore = math.Max(currentScore-trendStrength*0.5, 0)
		}
		confidence = math.Min(50+trendStrength*5, 95)
	}
	
	// 推荐清洗间隔
	recommendedInterval := 60 // 默认60分钟
	if trend == "degrading" && trendStrength > 5 {
		recommendedInterval = 30 // 质量下降时更频繁清洗
	} else if trend == "improving" {
		recommendedInterval = 90 // 质量提升时可以延长间隔
	}
	
	return &QualityTrendAnalysis{
		QualityTrend:        trend,
		TrendStrength:       trendStrength,
		PredictedNextScore:  predictedScore,
		ConfidenceLevel:     confidence,
		RecommendedInterval: recommendedInterval,
	}
}

// average 计算平均值
func (dqa *DataQualityAnalyzer) average(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// generateRecommendations 生成改进建议
func (dqa *DataQualityAnalyzer) generateRecommendations(
	zoneStats *ZoneQualityStatistics,
	cleaningEff *CleaningEfficiencyStats,
	outlierDist *OutlierDistributionStats,
	trendAnalysis *QualityTrendAnalysis,
) []string {
	recommendations := make([]string, 0)
	
	// 基于区域质量的建议
	if zoneStats.AverageZoneScore < 60 {
		recommendations = append(recommendations, "区域平均质量偏低，建议检查数据源质量")
	}
	
	if zoneStats.QualityConsistency < 70 {
		recommendations = append(recommendations, "区域质量一致性较差，建议优化识别算法")
	}
	
	// 基于清洗效率的建议
	if cleaningEff.FilteredRatePercent > 30 {
		recommendations = append(recommendations, "过滤率过高，建议放宽清洗参数以避免误杀")
	} else if cleaningEff.FilteredRatePercent < 5 {
		recommendations = append(recommendations, "过滤率过低，建议加强清洗以提高数据质量")
	}
	
	// 基于异常值分布的建议
	if outlierDist.WidthATROutliers.Count > outlierDist.VolRatioOutliers.Count*2 {
		recommendations = append(recommendations, "WidthATR异常值较多，建议检查ATR计算方法")
	}
	
	if outlierDist.VolRatioOutliers.Count > outlierDist.WidthATROutliers.Count*2 {
		recommendations = append(recommendations, "VolRatio异常值较多，建议检查成交量数据源")
	}
	
	// 基于趋势分析的建议
	if trendAnalysis.QualityTrend == "degrading" && trendAnalysis.TrendStrength > 10 {
		recommendations = append(recommendations, "数据质量持续下降，建议立即检查数据源和清洗配置")
	}
	
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "当前数据质量状况良好，保持现有配置")
	}
	
	return recommendations
}

// logQualityReport 记录质量报告
func (dqa *DataQualityAnalyzer) logQualityReport(metrics *DataQualityMetrics) {
	log.Printf("📊 =========================")
	log.Printf("📊 数据质量分析报告")
	log.Printf("📊 =========================")
	log.Printf("📊 总体质量评分: %.1f/100", metrics.OverallQualityScore)
	log.Printf("📊 分析区域总数: %d", metrics.ZoneQualityStats.TotalZonesAnalyzed)
	log.Printf("📊 清洗过滤率: %.1f%%", metrics.CleaningEfficiency.FilteredRatePercent)
	log.Printf("📊 质量趋势: %s (强度: %.1f)", metrics.QualityTrendAnalysis.QualityTrend, metrics.QualityTrendAnalysis.TrendStrength)
	
	log.Printf("📊 质量分级分布:")
	log.Printf("   高质量区域: %d个", metrics.ZoneQualityStats.HighQualityZones)
	log.Printf("   中质量区域: %d个", metrics.ZoneQualityStats.MediumQualityZones)
	log.Printf("   低质量区域: %d个", metrics.ZoneQualityStats.LowQualityZones)
	
	log.Printf("📊 异常值统计:")
	log.Printf("   WidthATR异常: %d个", metrics.OutlierDistribution.WidthATROutliers.Count)
	log.Printf("   VolRatio异常: %d个", metrics.OutlierDistribution.VolRatioOutliers.Count)
	
	log.Printf("📊 改进建议:")
	for i, rec := range metrics.RecommendedActions {
		log.Printf("   %d. %s", i+1, rec)
	}
	
	log.Printf("📊 =========================")
}

// ExportQualityReport 导出质量报告为JSON
func (dqa *DataQualityAnalyzer) ExportQualityReport(metrics *DataQualityMetrics) (string, error) {
	jsonData, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}