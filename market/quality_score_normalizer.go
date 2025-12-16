package market

import (
	"log"
	"math"
)

// QualityScoreNormalizer 质量评分标准化器 - P1-1修复：统一所有质量评分为0-1量纲
// 解决overall_score=1的歧义问题，确保所有质量评分使用统一0-1量纲
type QualityScoreNormalizer struct {
	config NormalizationConfig
}

// NormalizationConfig 标准化配置
type NormalizationConfig struct {
	EnableLegacyCompatibility bool    `json:"enable_legacy_compatibility"` // 是否启用遗留兼容模式
	LogConversions           bool    `json:"log_conversions"`            // 是否记录转换日志
	ValidationThreshold      float64 `json:"validation_threshold"`       // 验证阈值
}

// ScoreMetadata 评分元数据，用于标识量纲和来源
type ScoreMetadata struct {
	OriginalScale    string  `json:"original_scale"`    // "0-1" 或 "0-100"
	NormalizedScore  float64 `json:"normalized_score"`  // 标准化后的0-1评分
	OriginalScore    float64 `json:"original_score"`    // 原始评分
	ConversionMethod string  `json:"conversion_method"` // 转换方法
	Source           string  `json:"source"`           // 评分来源
	Confidence       float64 `json:"confidence"`       // 转换置信度
}

// QualityScoreBundle 统一质量评分包 - 包含所有标准化的评分
type QualityScoreBundle struct {
	// 数据质量评分 (统一为0-1)
	OverallQuality    *ScoreMetadata `json:"overall_quality"`    // 总体质量评分
	DataQuality       *ScoreMetadata `json:"data_quality"`       // 数据质量评分
	CleaningQuality   *ScoreMetadata `json:"cleaning_quality"`   // 清洗质量评分
	ConsistencyScore  *ScoreMetadata `json:"consistency_score"`  // 一致性评分
	
	// 流动性与稳定性评分 (已经是0-1)
	LiquidityScore    *ScoreMetadata `json:"liquidity_score"`    // 流动性评分
	StabilityScore    *ScoreMetadata `json:"stability_score"`    // 稳定性评分
	
	// 技术分析质量评分 (统一为0-1)
	ChannelQuality    *ScoreMetadata `json:"channel_quality"`    // 通道质量评分
	AnchorQuality     *ScoreMetadata `json:"anchor_quality"`     // 锚点质量评分
	StructureQuality  *ScoreMetadata `json:"structure_quality"`  // 结构质量评分
	
	// 聚合评分
	CompositeQuality  *ScoreMetadata `json:"composite_quality"`  // 复合质量评分
	
	// 元数据
	NormalizationStats *NormalizationStats `json:"normalization_stats"` // 标准化统计
}

// NormalizationStats 标准化统计信息
type NormalizationStats struct {
	TotalScoresProcessed int     `json:"total_scores_processed"` // 处理的总评分数
	ConversionCount      int     `json:"conversion_count"`       // 转换数量
	AverageConfidence    float64 `json:"average_confidence"`     // 平均转换置信度
	ValidationErrors     int     `json:"validation_errors"`      // 验证错误数
}

var defaultNormalizationConfig = NormalizationConfig{
	EnableLegacyCompatibility: true,
	LogConversions:           true,
	ValidationThreshold:      0.95,
}

// NewQualityScoreNormalizer 创建质量评分标准化器
func NewQualityScoreNormalizer() *QualityScoreNormalizer {
	return &QualityScoreNormalizer{
		config: defaultNormalizationConfig,
	}
}

// NormalizeDataQualityScore 标准化数据质量评分（0-100 -> 0-1）
// 🔥 P1-1核心修复：解决overall_score量纲不一致问题
func (qsn *QualityScoreNormalizer) NormalizeDataQualityScore(score float64, source string) *ScoreMetadata {
	// 检测原始量纲
	originalScale := qsn.detectScale(score)
	
	var normalizedScore float64
	var conversionMethod string
	confidence := 1.0
	
	switch originalScale {
	case "0-100":
		// 从0-100转换到0-1
		normalizedScore = score / 100.0
		conversionMethod = "divide_by_100"
		
		if qsn.config.LogConversions {
			log.Printf("🔄 [P1-1质量评分标准化] %s: %.2f/100 -> %.4f/1 (0-100->0-1转换)", 
				source, score, normalizedScore)
		}
		
	case "0-1":
		// 已经是0-1量纲
		normalizedScore = score
		conversionMethod = "no_conversion"
		
	case "ambiguous":
		// 模糊量纲，需要智能判断
		if score <= 1.0 {
			// 很可能是0-1量纲
			normalizedScore = score
			conversionMethod = "assume_0_1"
			confidence = 0.8
		} else {
			// 很可能是0-100量纲
			normalizedScore = score / 100.0
			conversionMethod = "assume_0_100"
			confidence = 0.7
		}
		
		if qsn.config.LogConversions {
			log.Printf("⚠️ [P1-1量纲歧义] %s: %.2f -> %.4f (歧义处理, 置信度: %.2f)", 
				source, score, normalizedScore, confidence)
		}
		
	default:
		// 异常值处理
		normalizedScore = math.Max(0.0, math.Min(1.0, score))
		conversionMethod = "clamp_to_range"
		confidence = 0.5
	}
	
	// 验证标准化结果
	if normalizedScore < 0 || normalizedScore > 1 {
		log.Printf("❌ [P1-1验证失败] %s: 标准化后评分%.4f超出0-1范围", source, normalizedScore)
		normalizedScore = math.Max(0.0, math.Min(1.0, normalizedScore))
		confidence *= 0.5
	}
	
	return &ScoreMetadata{
		OriginalScale:    originalScale,
		NormalizedScore:  normalizedScore,
		OriginalScore:    score,
		ConversionMethod: conversionMethod,
		Source:          source,
		Confidence:      confidence,
	}
}

// NormalizeOverallScore 专门处理overall_score的标准化
// 🔥 P1-1核心修复：解决"overall_score=1"的歧义问题
func (qsn *QualityScoreNormalizer) NormalizeOverallScore(score float64, source string) *ScoreMetadata {
	// overall_score特殊处理：很多时候1.0可能意味着完美评分或1%评分
	
	// 上下文启发式判断
	if score == 1.0 {
		// 检查来源上下文
		if qsn.isLikelyPerfectScore(source) {
			// 很可能是完美评分
			return qsn.createScoreMetadata(score, 1.0, "0-1", "perfect_score_assumption", source, 0.9)
		} else {
			// 很可能是1%评分
			return qsn.createScoreMetadata(score, 0.01, "0-100", "one_percent_assumption", source, 0.8)
		}
	}
	
	// 其他情况使用通用标准化
	return qsn.NormalizeDataQualityScore(score, source)
}

// detectScale 检测评分量纲
func (qsn *QualityScoreNormalizer) detectScale(score float64) string {
	if score < 0 || score > 100 {
		return "invalid"
	}
	
	if score > 1.0 {
		return "0-100"
	}
	
	if score == 1.0 || score == 0.0 {
		return "ambiguous" // 1.0和0.0在两种量纲下都是有效值
	}
	
	if score > 0.0 && score < 1.0 {
		return "0-1"
	}
	
	return "unknown"
}

// isLikelyPerfectScore 判断是否可能是完美评分的上下文
func (qsn *QualityScoreNormalizer) isLikelyPerfectScore(source string) bool {
	perfectScoreIndicators := []string{
		"confidence", "probability", "ratio", "normalized", "proportion",
	}
	
	for _, indicator := range perfectScoreIndicators {
		if contains(source, indicator) {
			return true
		}
	}
	
	return false
}

// createScoreMetadata 创建评分元数据
func (qsn *QualityScoreNormalizer) createScoreMetadata(
	originalScore, normalizedScore float64,
	originalScale, conversionMethod, source string,
	confidence float64,
) *ScoreMetadata {
	return &ScoreMetadata{
		OriginalScale:    originalScale,
		NormalizedScore:  normalizedScore,
		OriginalScore:    originalScore,
		ConversionMethod: conversionMethod,
		Source:          source,
		Confidence:      confidence,
	}
}

// CreateQualityBundle 创建统一的质量评分包
// 🔥 P1-1核心功能：将各种来源的质量评分统一为0-1量纲
func (qsn *QualityScoreNormalizer) CreateQualityBundle(
	overallScore float64,
	dataQuality float64,
	liquidityScore float64,
	stabilityScore float64,
	channelQuality float64,
	anchorScore float64,
) *QualityScoreBundle {
	
	bundle := &QualityScoreBundle{
		OverallQuality:    qsn.NormalizeOverallScore(overallScore, "overall_quality"),
		DataQuality:       qsn.NormalizeDataQualityScore(dataQuality, "data_quality"),
		LiquidityScore:    qsn.NormalizeDataQualityScore(liquidityScore, "liquidity_score"),
		StabilityScore:    qsn.NormalizeDataQualityScore(stabilityScore, "stability_score"),
		ChannelQuality:    qsn.NormalizeDataQualityScore(channelQuality, "channel_quality"),
		AnchorQuality:     qsn.NormalizeDataQualityScore(anchorScore/100.0, "anchor_score"), // 锚点评分通常是0-100
	}
	
	// 计算复合质量评分
	bundle.CompositeQuality = qsn.calculateCompositeQuality(bundle)
	
	// 生成标准化统计
	bundle.NormalizationStats = qsn.generateNormalizationStats(bundle)
	
	return bundle
}

// calculateCompositeQuality 计算复合质量评分
func (qsn *QualityScoreNormalizer) calculateCompositeQuality(bundle *QualityScoreBundle) *ScoreMetadata {
	// 加权平均计算复合评分
	weights := map[string]float64{
		"overall_quality": 0.25,
		"data_quality":    0.20,
		"liquidity_score": 0.15,
		"stability_score": 0.15,
		"channel_quality": 0.15,
		"anchor_quality":  0.10,
	}
	
	totalScore := 0.0
	totalWeight := 0.0
	totalConfidence := 0.0
	count := 0
	
	scores := []*ScoreMetadata{
		bundle.OverallQuality, bundle.DataQuality, bundle.LiquidityScore,
		bundle.StabilityScore, bundle.ChannelQuality, bundle.AnchorQuality,
	}
	
	sourceNames := []string{
		"overall_quality", "data_quality", "liquidity_score",
		"stability_score", "channel_quality", "anchor_quality",
	}
	
	for i, score := range scores {
		if score != nil {
			sourceName := sourceNames[i]
			weight := weights[sourceName]
			totalScore += score.NormalizedScore * weight
			totalWeight += weight
			totalConfidence += score.Confidence
			count++
		}
	}
	
	if totalWeight == 0 {
		return qsn.createScoreMetadata(0, 0, "0-1", "no_data", "composite", 0)
	}
	
	compositeScore := totalScore / totalWeight
	avgConfidence := totalConfidence / float64(count)
	
	return qsn.createScoreMetadata(
		compositeScore, compositeScore, "0-1", "weighted_average", "composite", avgConfidence,
	)
}

// generateNormalizationStats 生成标准化统计信息
func (qsn *QualityScoreNormalizer) generateNormalizationStats(bundle *QualityScoreBundle) *NormalizationStats {
	stats := &NormalizationStats{}
	
	scores := []*ScoreMetadata{
		bundle.OverallQuality, bundle.DataQuality, bundle.LiquidityScore,
		bundle.StabilityScore, bundle.ChannelQuality, bundle.AnchorQuality,
	}
	
	totalConfidence := 0.0
	conversionCount := 0
	
	for _, score := range scores {
		if score != nil {
			stats.TotalScoresProcessed++
			totalConfidence += score.Confidence
			
			if score.ConversionMethod != "no_conversion" {
				conversionCount++
			}
			
			if score.Confidence < qsn.config.ValidationThreshold {
				stats.ValidationErrors++
			}
		}
	}
	
	stats.ConversionCount = conversionCount
	if stats.TotalScoresProcessed > 0 {
		stats.AverageConfidence = totalConfidence / float64(stats.TotalScoresProcessed)
	}
	
	return stats
}

// GetNormalizedScore 获取标准化后的评分值
func (sm *ScoreMetadata) GetNormalizedScore() float64 {
	if sm == nil {
		return 0.0
	}
	return sm.NormalizedScore
}

// IsHighConfidence 判断是否是高置信度转换
func (sm *ScoreMetadata) IsHighConfidence() bool {
	if sm == nil {
		return false
	}
	return sm.Confidence >= 0.9
}

// FormatNormalizedScore 格式化标准化评分用于输出
func (sm *ScoreMetadata) FormatNormalizedScore() float64 {
	if sm == nil {
		return 0.0
	}
	return math.Round(sm.NormalizedScore*10000) / 10000 // 保留4位小数
}

// 全局标准化器实例
var globalQualityNormalizer = NewQualityScoreNormalizer()

// GetGlobalQualityNormalizer 获取全局质量评分标准化器
func GetGlobalQualityNormalizer() *QualityScoreNormalizer {
	return globalQualityNormalizer
}

// 辅助函数
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr)))
}