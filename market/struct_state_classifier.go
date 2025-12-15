package market

import (
	"fmt"
	"log"
	"math"
)

// StructState 结构状态枚举（V-13.5规范要求的三态）
type StructState string

const (
	StructStateFavorable StructState = "FAVORABLE" // 有利：结构支持进场
	StructStateNeutral   StructState = "NEUTRAL"   // 中性：结构不明确
	StructStateDanger    StructState = "DANGER"    // 危险：结构反对进场
)

// StructStateResult 结构状态分类结果
type StructStateResult struct {
	Long  StructStateClassification `json:"long"`
	Short StructStateClassification `json:"short"`
}

// StructStateClassification 单向结构状态分类
type StructStateClassification struct {
	State     StructState          `json:"state"`      // FAVORABLE/NEUTRAL/DANGER
	Evidence  *StructStateEvidence `json:"evidence"`   // 判定证据
	Anchor    *AnchorCandidate     `json:"anchor"`     // 选中的锚点
	Confidence float64             `json:"confidence"` // 置信度 (0-1)
}

// StructStateEvidence 结构状态判定证据
type StructStateEvidence struct {
	// 距离信息（ATR归一化）
	NearestDistance    float64 `json:"nearest_distance"`     // 最近锚点距离（ATR倍数）
	DistanceCategory   string  `json:"distance_category"`    // "VERY_CLOSE"/"CLOSE"/"MEDIUM"/"FAR"
	
	// 空间结构信息
	AnchorDensity      float64 `json:"anchor_density"`       // 锚点密度（每ATR内的锚点数）
	StructuralSupport  float64 `json:"structural_support"`   // 结构支持强度 (0-1)
	ConflictingAnchors int     `json:"conflicting_anchors"`  // 冲突锚点数量
	
	// 质量评估
	BestAnchorScore    float64 `json:"best_anchor_score"`    // 最佳锚点评分
	AverageScore       float64 `json:"average_score"`        // 平均锚点评分
	HighQualityCount   int     `json:"high_quality_count"`   // 高质量锚点数量(>70分)
	
	// 时间框架分析
	HTFSupport         bool    `json:"htf_support"`          // 高时间框架支持
	MTFAlignment       bool    `json:"mtf_alignment"`        // 多时间框架对齐
	FreshAnchorCount   int     `json:"fresh_anchor_count"`   // 新鲜锚点数量
}

// StructStateClassifier 结构状态分类器
type StructStateClassifier struct {
	config *StructStateConfig
}

// StructStateConfig 结构状态分类器配置
type StructStateConfig struct {
	// ATR归一化阈值
	VeryCloseThreshold float64 `json:"very_close_threshold"` // 0.5 ATR
	CloseThreshold     float64 `json:"close_threshold"`      // 1.0 ATR  
	MediumThreshold    float64 `json:"medium_threshold"`     // 2.0 ATR
	FarThreshold       float64 `json:"far_threshold"`        // 4.0 ATR
	
	// 评分阈值
	MinFavorableScore  float64 `json:"min_favorable_score"`  // 70.0 最低有利评分
	MinNeutralScore    float64 `json:"min_neutral_score"`    // 40.0 最低中性评分
	
	// 结构质量要求
	MinStructuralSupport float64 `json:"min_structural_support"` // 0.6 最低结构支持
	MaxConflictRatio     float64 `json:"max_conflict_ratio"`     // 0.3 最大冲突比例
	
	// 高质量锚点要求
	HighQualityThreshold float64 `json:"high_quality_threshold"` // 70.0 高质量阈值
	MinHighQualityCount  int     `json:"min_high_quality_count"` // 2 最少高质量数量
}

// NewStructStateClassifier 创建结构状态分类器
func NewStructStateClassifier(config *StructStateConfig) *StructStateClassifier {
	if config == nil {
		config = DefaultStructStateConfig()
	}
	
	return &StructStateClassifier{
		config: config,
	}
}

// DefaultStructStateConfig 默认配置
func DefaultStructStateConfig() *StructStateConfig {
	return &StructStateConfig{
		VeryCloseThreshold:   0.5,
		CloseThreshold:       1.0,
		MediumThreshold:      2.0,
		FarThreshold:        4.0,
		MinFavorableScore:    70.0,
		MinNeutralScore:     40.0,
		MinStructuralSupport: 0.6,
		MaxConflictRatio:    0.3,
		HighQualityThreshold: 70.0,
		MinHighQualityCount: 2,
	}
}

// Classify 分类结构状态（核心方法）
func (ssc *StructStateClassifier) Classify(
	longCandidates, shortCandidates []AnchorCandidate,
	lastPrice, atr14 float64,
) *StructStateResult {
	
	// 分类多头结构状态
	longClassification := ssc.classifySingleDirection(
		longCandidates, "LONG", lastPrice, atr14)
	
	// 分类空头结构状态
	shortClassification := ssc.classifySingleDirection(
		shortCandidates, "SHORT", lastPrice, atr14)
	
	return &StructStateResult{
		Long:  longClassification,
		Short: shortClassification,
	}
}

// classifySingleDirection 分类单个方向的结构状态
func (ssc *StructStateClassifier) classifySingleDirection(
	candidates []AnchorCandidate,
	direction string,
	lastPrice, atr14 float64,
) StructStateClassification {
	
	// 如果没有候选锚点，返回DANGER
	if len(candidates) == 0 {
		return StructStateClassification{
			State: StructStateDanger,
			Evidence: &StructStateEvidence{
				NearestDistance:   999.0, // 表示无穷远
				DistanceCategory:  "FAR",
				StructuralSupport: 0.0,
			},
			Anchor:     nil,
			Confidence: 0.9, // 无锚点时确信度很高
		}
	}
	
	// 生成证据
	evidence := ssc.generateEvidence(candidates, lastPrice, atr14)
	
	// 选择最佳锚点
	bestAnchor := &candidates[0] // candidates已经排序，第一个是最佳的
	
	// 基于证据进行三态分类
	state := ssc.determineState(evidence, direction)
	
	// 计算置信度
	confidence := ssc.calculateConfidence(evidence, state)
	
	return StructStateClassification{
		State:      state,
		Evidence:   evidence,
		Anchor:     bestAnchor,
		Confidence: confidence,
	}
}

// generateEvidence 生成结构状态判定证据
func (ssc *StructStateClassifier) generateEvidence(
	candidates []AnchorCandidate,
	lastPrice, atr14 float64,
) *StructStateEvidence {
	
	if len(candidates) == 0 || atr14 <= 0 {
		return &StructStateEvidence{
			NearestDistance:   999.0,
			DistanceCategory:  "FAR",
			StructuralSupport: 0.0,
		}
	}
	
	// 计算距离信息
	nearestDistance := math.Abs(candidates[0].Level - lastPrice) / atr14
	distanceCategory := ssc.categorizeDistance(nearestDistance)
	
	// 计算锚点密度（每ATR内的锚点数）
	anchorDensity := ssc.calculateAnchorDensity(candidates, lastPrice, atr14)
	
	// 计算结构支持强度
	structuralSupport := ssc.calculateStructuralSupport(candidates)
	
	// 统计冲突锚点
	conflictingAnchors := ssc.countConflictingAnchors(candidates, lastPrice)
	
	// 质量评估
	var totalScore float64
	var highQualityCount int
	for _, candidate := range candidates {
		totalScore += candidate.AnchorScore
		if candidate.AnchorScore >= ssc.config.HighQualityThreshold {
			highQualityCount++
		}
	}
	
	bestAnchorScore := candidates[0].AnchorScore
	averageScore := totalScore / float64(len(candidates))
	
	// 时间框架分析
	htfSupport := ssc.hasHTFSupport(candidates)
	mtfAlignment := ssc.hasMTFAlignment(candidates)
	freshAnchorCount := ssc.countFreshAnchors(candidates)
	
	return &StructStateEvidence{
		NearestDistance:    nearestDistance,
		DistanceCategory:   distanceCategory,
		AnchorDensity:      anchorDensity,
		StructuralSupport:  structuralSupport,
		ConflictingAnchors: conflictingAnchors,
		BestAnchorScore:    bestAnchorScore,
		AverageScore:       averageScore,
		HighQualityCount:   highQualityCount,
		HTFSupport:         htfSupport,
		MTFAlignment:       mtfAlignment,
		FreshAnchorCount:   freshAnchorCount,
	}
}

// categorizeDistance 分类距离
func (ssc *StructStateClassifier) categorizeDistance(distanceATR float64) string {
	if distanceATR <= ssc.config.VeryCloseThreshold {
		return "VERY_CLOSE"
	} else if distanceATR <= ssc.config.CloseThreshold {
		return "CLOSE"
	} else if distanceATR <= ssc.config.MediumThreshold {
		return "MEDIUM"
	} else {
		return "FAR"
	}
}

// calculateAnchorDensity 计算锚点密度
func (ssc *StructStateClassifier) calculateAnchorDensity(
	candidates []AnchorCandidate,
	lastPrice, atr14 float64,
) float64 {
	if atr14 <= 0 {
		return 0.0
	}
	
	// 统计2个ATR范围内的锚点数量
	rangeATR := 2.0
	count := 0
	
	for _, candidate := range candidates {
		distance := math.Abs(candidate.Level - lastPrice) / atr14
		if distance <= rangeATR {
			count++
		}
	}
	
	return float64(count) / rangeATR
}

// calculateStructuralSupport 计算结构支持强度
func (ssc *StructStateClassifier) calculateStructuralSupport(candidates []AnchorCandidate) float64 {
	if len(candidates) == 0 {
		return 0.0
	}
	
	// 基于锚点评分和优先级的加权平均
	var weightedScore float64
	var totalWeight float64
	
	for _, candidate := range candidates {
		// 权重：优先级越高权重越大
		weight := 8.0 - float64(candidate.PriorityRank) // P1=7, P2=6, ..., P7=1
		if weight <= 0 {
			weight = 1.0
		}
		
		// 标准化评分到0-1
		normalizedScore := candidate.AnchorScore / 100.0
		
		weightedScore += normalizedScore * weight
		totalWeight += weight
	}
	
	if totalWeight == 0 {
		return 0.0
	}
	
	return weightedScore / totalWeight
}

// countConflictingAnchors 统计冲突锚点
func (ssc *StructStateClassifier) countConflictingAnchors(
	candidates []AnchorCandidate,
	lastPrice float64,
) int {
	conflictCount := 0
	
	for _, candidate := range candidates {
		// 检查锚点是否在"错误"的方向
		if candidate.Dir == "LONG" && candidate.Level > lastPrice {
			conflictCount++ // 多头锚点在价格上方是冲突的
		} else if candidate.Dir == "SHORT" && candidate.Level < lastPrice {
			conflictCount++ // 空头锚点在价格下方是冲突的
		}
	}
	
	return conflictCount
}

// hasHTFSupport 检查是否有高时间框架支持
func (ssc *StructStateClassifier) hasHTFSupport(candidates []AnchorCandidate) bool {
	for _, candidate := range candidates {
		// HTF_ZONE (P1) 或高时间框架（4h/1h）
		if candidate.Type == AnchorHTFZone ||
		   candidate.TF == "4h" || candidate.TF == "1h" {
			return true
		}
	}
	return false
}

// hasMTFAlignment 检查多时间框架对齐
func (ssc *StructStateClassifier) hasMTFAlignment(candidates []AnchorCandidate) bool {
	timeframes := make(map[string]bool)
	
	for _, candidate := range candidates {
		timeframes[candidate.TF] = true
	}
	
	// 如果有2个或以上不同时间框架的锚点，认为有对齐
	return len(timeframes) >= 2
}

// countFreshAnchors 统计新鲜锚点数量
func (ssc *StructStateClassifier) countFreshAnchors(candidates []AnchorCandidate) int {
	count := 0
	for _, candidate := range candidates {
		if candidate.IsFresh {
			count++
		}
	}
	return count
}

// determineState 基于证据确定结构状态
func (ssc *StructStateClassifier) determineState(
	evidence *StructStateEvidence,
	direction string,
) StructState {
	
	// 检查基础条件
	if evidence.BestAnchorScore < ssc.config.MinNeutralScore {
		return StructStateDanger // 最佳锚点评分过低
	}
	
	if evidence.StructuralSupport < ssc.config.MinStructuralSupport {
		return StructStateDanger // 结构支持不足
	}
	
	// 检查冲突比例
	totalCandidates := evidence.HighQualityCount + evidence.ConflictingAnchors
	if totalCandidates > 0 {
		conflictRatio := float64(evidence.ConflictingAnchors) / float64(totalCandidates)
		if conflictRatio > ssc.config.MaxConflictRatio {
			return StructStateDanger // 冲突过多
		}
	}
	
	// FAVORABLE条件：
	// 1. 最佳锚点评分 >= 70
	// 2. 距离适中（VERY_CLOSE/CLOSE/MEDIUM）
	// 3. 高质量锚点数量 >= 2
	// 4. 有HTF支持或MTF对齐
	if evidence.BestAnchorScore >= ssc.config.MinFavorableScore &&
	   (evidence.DistanceCategory == "VERY_CLOSE" || 
	    evidence.DistanceCategory == "CLOSE" || 
	    evidence.DistanceCategory == "MEDIUM") &&
	   evidence.HighQualityCount >= ssc.config.MinHighQualityCount &&
	   (evidence.HTFSupport || evidence.MTFAlignment) {
		return StructStateFavorable
	}
	
	// 其他情况为NEUTRAL
	return StructStateNeutral
}

// calculateConfidence 计算置信度
func (ssc *StructStateClassifier) calculateConfidence(
	evidence *StructStateEvidence,
	state StructState,
) float64 {
	
	baseConfidence := 0.5
	
	// 基于锚点质量调整
	if evidence.BestAnchorScore >= 80 {
		baseConfidence += 0.2
	} else if evidence.BestAnchorScore >= 60 {
		baseConfidence += 0.1
	}
	
	// 基于结构支持调整
	if evidence.StructuralSupport >= 0.8 {
		baseConfidence += 0.2
	} else if evidence.StructuralSupport >= 0.6 {
		baseConfidence += 0.1
	}
	
	// 基于距离调整
	switch evidence.DistanceCategory {
	case "VERY_CLOSE":
		baseConfidence += 0.15
	case "CLOSE":
		baseConfidence += 0.1
	case "MEDIUM":
		baseConfidence += 0.05
	case "FAR":
		baseConfidence -= 0.1
	}
	
	// 基于多时间框架支持调整
	if evidence.HTFSupport && evidence.MTFAlignment {
		baseConfidence += 0.15
	} else if evidence.HTFSupport || evidence.MTFAlignment {
		baseConfidence += 0.08
	}
	
	// 基于冲突调整
	if evidence.ConflictingAnchors > 0 {
		baseConfidence -= float64(evidence.ConflictingAnchors) * 0.05
	}
	
	// 基于新鲜度调整
	if evidence.FreshAnchorCount >= 2 {
		baseConfidence += 0.1
	} else if evidence.FreshAnchorCount >= 1 {
		baseConfidence += 0.05
	}
	
	// 限制在合理范围内
	return math.Max(0.1, math.Min(0.95, baseConfidence))
}

// FormatStructStateResult 格式化结构状态结果
func FormatStructStateResult(result *StructStateResult) string {
	if result == nil {
		return "StructState: 无数据"
	}
	
	return fmt.Sprintf("StructState: LONG=%s(%.2f), SHORT=%s(%.2f)",
		result.Long.State, result.Long.Confidence,
		result.Short.State, result.Short.Confidence)
}

// LogStructStateDetails 详细日志输出（调试用）
func LogStructStateDetails(result *StructStateResult) {
	if result == nil {
		return
	}
	
	log.Printf("🎯 [StructState] LONG: %s (置信度: %.2f)", 
		result.Long.State, result.Long.Confidence)
	if result.Long.Evidence != nil {
		e := result.Long.Evidence
		log.Printf("  📏 距离: %.2fATR (%s), 密度: %.1f, 支持: %.2f", 
			e.NearestDistance, e.DistanceCategory, e.AnchorDensity, e.StructuralSupport)
		log.Printf("  📊 评分: 最佳%.1f, 平均%.1f, 高质量%d个, 冲突%d个",
			e.BestAnchorScore, e.AverageScore, e.HighQualityCount, e.ConflictingAnchors)
		log.Printf("  🕐 时间框架: HTF=%v, MTF对齐=%v, 新鲜=%d个",
			e.HTFSupport, e.MTFAlignment, e.FreshAnchorCount)
	}
	
	log.Printf("🎯 [StructState] SHORT: %s (置信度: %.2f)", 
		result.Short.State, result.Short.Confidence)
	if result.Short.Evidence != nil {
		e := result.Short.Evidence
		log.Printf("  📏 距离: %.2fATR (%s), 密度: %.1f, 支持: %.2f", 
			e.NearestDistance, e.DistanceCategory, e.AnchorDensity, e.StructuralSupport)
		log.Printf("  📊 评分: 最佳%.1f, 平均%.1f, 高质量%d个, 冲突%d个",
			e.BestAnchorScore, e.AverageScore, e.HighQualityCount, e.ConflictingAnchors)
		log.Printf("  🕐 时间框架: HTF=%v, MTF对齐=%v, 新鲜=%d个",
			e.HTFSupport, e.MTFAlignment, e.FreshAnchorCount)
	}
}