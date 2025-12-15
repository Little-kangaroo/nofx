package market

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// Gate2PerformanceMonitor Gate2性能监控器
type Gate2PerformanceMonitor struct {
	mutex sync.RWMutex
	stats *Gate2PerformanceStats
}

// Gate2PerformanceStats Gate2性能统计
type Gate2PerformanceStats struct {
	// 执行统计
	TotalExecutions    int64   `json:"total_executions"`
	SuccessExecutions  int64   `json:"success_executions"`
	ErrorExecutions    int64   `json:"error_executions"`
	SuccessRate        float64 `json:"success_rate"`
	
	// 时间统计（毫秒）
	TotalExecutionTime float64 `json:"total_execution_time_ms"`
	AverageExecutionTime float64 `json:"average_execution_time_ms"`
	MinExecutionTime   float64 `json:"min_execution_time_ms"`
	MaxExecutionTime   float64 `json:"max_execution_time_ms"`
	
	// 模块执行时间统计
	AnchorEngineTime     *ModuleTimeStats `json:"anchor_engine_time"`
	StructClassifierTime *ModuleTimeStats `json:"struct_classifier_time"`
	TriggerDetectorTime  *ModuleTimeStats `json:"trigger_detector_time"`
	
	// 锚点统计
	AnchorStats    *AnchorStatistics    `json:"anchor_stats"`
	
	// 内存使用统计
	MemoryStats    *MemoryStatistics    `json:"memory_stats"`
	
	// 错误统计
	ErrorStats     *ErrorStatistics     `json:"error_stats"`
	
	// 时间窗口统计
	WindowStats    *WindowStatistics    `json:"window_stats"`
	
	// 最后更新时间
	LastUpdated    time.Time            `json:"last_updated"`
}

// ModuleTimeStats 模块时间统计
type ModuleTimeStats struct {
	TotalTime   float64 `json:"total_time_ms"`
	AverageTime float64 `json:"average_time_ms"`
	MinTime     float64 `json:"min_time_ms"`
	MaxTime     float64 `json:"max_time_ms"`
	Executions  int64   `json:"executions"`
}

// AnchorStatistics 锚点统计
type AnchorStatistics struct {
	// 总体统计
	TotalLongAnchors    int64   `json:"total_long_anchors"`
	TotalShortAnchors   int64   `json:"total_short_anchors"`
	AverageLongAnchors  float64 `json:"average_long_anchors"`
	AverageShortAnchors float64 `json:"average_short_anchors"`
	
	// 评分统计
	ScoreDistribution map[string]int64 `json:"score_distribution"` // "0-20", "21-40", ...
	AverageScore      float64          `json:"average_score"`
	HighScoreCount    int64            `json:"high_score_count"`   // >80分
	MediumScoreCount  int64            `json:"medium_score_count"` // 60-80分
	LowScoreCount     int64            `json:"low_score_count"`    // <60分
	
	// 优先级统计
	PriorityDistribution map[int]int64 `json:"priority_distribution"` // P1-P7
	
	// 类型统计
	TypeDistribution map[string]int64 `json:"type_distribution"` // HTF_ZONE, VPVR_BOUND等
	
	// 时间框架统计
	TimeframeDistribution map[string]int64 `json:"timeframe_distribution"` // 4h, 1h, 30m等
}

// MemoryStatistics 内存统计
type MemoryStatistics struct {
	PeakMemoryUsage    int64 `json:"peak_memory_usage_bytes"`
	AverageMemoryUsage int64 `json:"average_memory_usage_bytes"`
	MemoryLeakDetected bool  `json:"memory_leak_detected"`
}

// ErrorStatistics 错误统计
type ErrorStatistics struct {
	ErrorsByType        map[string]int64 `json:"errors_by_type"`
	ErrorsByModule      map[string]int64 `json:"errors_by_module"`
	MostCommonError     string           `json:"most_common_error"`
	RecentErrors        []ErrorEntry     `json:"recent_errors"`
	ErrorRatePerHour    float64          `json:"error_rate_per_hour"`
}

// ErrorEntry 错误条目
type ErrorEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Module    string    `json:"module"`
	Error     string    `json:"error"`
	Symbol    string    `json:"symbol"`
}

// WindowStatistics 时间窗口统计（近期性能）
type WindowStatistics struct {
	Last1Hour  *WindowPeriodStats `json:"last_1_hour"`
	Last6Hours *WindowPeriodStats `json:"last_6_hours"`
	Last24Hours *WindowPeriodStats `json:"last_24_hours"`
}

// WindowPeriodStats 时间窗口周期统计
type WindowPeriodStats struct {
	Executions      int64   `json:"executions"`
	SuccessRate     float64 `json:"success_rate"`
	AverageTime     float64 `json:"average_time_ms"`
	AverageAnchors  float64 `json:"average_anchors"`
	AverageScore    float64 `json:"average_score"`
}

var (
	performanceMonitor *Gate2PerformanceMonitor
	monitorOnce        sync.Once
)

// GetGate2PerformanceMonitor 获取性能监控器（单例）
func GetGate2PerformanceMonitor() *Gate2PerformanceMonitor {
	monitorOnce.Do(func() {
		performanceMonitor = NewGate2PerformanceMonitor()
	})
	return performanceMonitor
}

// NewGate2PerformanceMonitor 创建性能监控器
func NewGate2PerformanceMonitor() *Gate2PerformanceMonitor {
	return &Gate2PerformanceMonitor{
		stats: &Gate2PerformanceStats{
			AnchorEngineTime:     &ModuleTimeStats{MinTime: 999999},
			StructClassifierTime: &ModuleTimeStats{MinTime: 999999},
			TriggerDetectorTime:  &ModuleTimeStats{MinTime: 999999},
			AnchorStats: &AnchorStatistics{
				ScoreDistribution:     make(map[string]int64),
				PriorityDistribution:  make(map[int]int64),
				TypeDistribution:      make(map[string]int64),
				TimeframeDistribution: make(map[string]int64),
			},
			MemoryStats: &MemoryStatistics{},
			ErrorStats: &ErrorStatistics{
				ErrorsByType:   make(map[string]int64),
				ErrorsByModule: make(map[string]int64),
				RecentErrors:   make([]ErrorEntry, 0),
			},
			WindowStats: &WindowStatistics{
				Last1Hour:   &WindowPeriodStats{},
				Last6Hours:  &WindowPeriodStats{},
				Last24Hours: &WindowPeriodStats{},
			},
			MinExecutionTime: 999999,
			LastUpdated:      time.Now(),
		},
	}
}

// RecordExecution 记录Gate2执行
func (pm *Gate2PerformanceMonitor) RecordExecution(
	symbol string,
	executionTime float64,
	anchorEngineTime, structClassifierTime, triggerDetectorTime float64,
	longAnchors, shortAnchors []AnchorCandidate,
	err error,
) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.stats.TotalExecutions++
	
	if err != nil {
		pm.stats.ErrorExecutions++
		pm.recordError(err, symbol, "general")
	} else {
		pm.stats.SuccessExecutions++
	}
	
	// 更新成功率
	pm.stats.SuccessRate = float64(pm.stats.SuccessExecutions) / float64(pm.stats.TotalExecutions)
	
	// 更新执行时间统计
	pm.updateExecutionTimeStats(executionTime)
	
	// 更新模块时间统计
	pm.updateModuleTimeStats("anchor_engine", anchorEngineTime)
	pm.updateModuleTimeStats("struct_classifier", structClassifierTime)
	pm.updateModuleTimeStats("trigger_detector", triggerDetectorTime)
	
	// 更新锚点统计
	pm.updateAnchorStats(longAnchors, shortAnchors)
	
	// 更新时间窗口统计
	pm.updateWindowStats(executionTime, longAnchors, shortAnchors, err == nil)
	
	pm.stats.LastUpdated = time.Now()
	
	// 定期日志输出
	if pm.stats.TotalExecutions%100 == 0 {
		pm.logPerformanceSummary()
	}
}

// updateExecutionTimeStats 更新执行时间统计
func (pm *Gate2PerformanceMonitor) updateExecutionTimeStats(executionTime float64) {
	pm.stats.TotalExecutionTime += executionTime
	pm.stats.AverageExecutionTime = pm.stats.TotalExecutionTime / float64(pm.stats.TotalExecutions)
	
	if executionTime < pm.stats.MinExecutionTime {
		pm.stats.MinExecutionTime = executionTime
	}
	if executionTime > pm.stats.MaxExecutionTime {
		pm.stats.MaxExecutionTime = executionTime
	}
}

// updateModuleTimeStats 更新模块时间统计
func (pm *Gate2PerformanceMonitor) updateModuleTimeStats(module string, executionTime float64) {
	var moduleStats *ModuleTimeStats
	
	switch module {
	case "anchor_engine":
		moduleStats = pm.stats.AnchorEngineTime
	case "struct_classifier":
		moduleStats = pm.stats.StructClassifierTime
	case "trigger_detector":
		moduleStats = pm.stats.TriggerDetectorTime
	default:
		return
	}
	
	moduleStats.Executions++
	moduleStats.TotalTime += executionTime
	moduleStats.AverageTime = moduleStats.TotalTime / float64(moduleStats.Executions)
	
	if executionTime < moduleStats.MinTime {
		moduleStats.MinTime = executionTime
	}
	if executionTime > moduleStats.MaxTime {
		moduleStats.MaxTime = executionTime
	}
}

// updateAnchorStats 更新锚点统计
func (pm *Gate2PerformanceMonitor) updateAnchorStats(longAnchors, shortAnchors []AnchorCandidate) {
	stats := pm.stats.AnchorStats
	
	// 更新锚点数量统计
	longCount := int64(len(longAnchors))
	shortCount := int64(len(shortAnchors))
	
	stats.TotalLongAnchors += longCount
	stats.TotalShortAnchors += shortCount
	
	// 计算平均锚点数
	totalExecs := pm.stats.TotalExecutions
	stats.AverageLongAnchors = float64(stats.TotalLongAnchors) / float64(totalExecs)
	stats.AverageShortAnchors = float64(stats.TotalShortAnchors) / float64(totalExecs)
	
	// 统计所有锚点
	allAnchors := append(longAnchors, shortAnchors...)
	
	var totalScore float64
	for _, anchor := range allAnchors {
		// 评分分布统计
		scoreRange := pm.getScoreRange(anchor.AnchorScore)
		stats.ScoreDistribution[scoreRange]++
		totalScore += anchor.AnchorScore
		
		// 评分等级统计
		if anchor.AnchorScore >= 80 {
			stats.HighScoreCount++
		} else if anchor.AnchorScore >= 60 {
			stats.MediumScoreCount++
		} else {
			stats.LowScoreCount++
		}
		
		// 优先级分布统计
		stats.PriorityDistribution[anchor.PriorityRank]++
		
		// 类型分布统计
		stats.TypeDistribution[string(anchor.Type)]++
		
		// 时间框架分布统计
		stats.TimeframeDistribution[anchor.TF]++
	}
	
	// 更新平均评分
	if len(allAnchors) > 0 {
		currentAvg := stats.AverageScore * float64(totalExecs-1)
		stats.AverageScore = (currentAvg + totalScore/float64(len(allAnchors))) / float64(totalExecs)
	}
}

// getScoreRange 获取评分范围
func (pm *Gate2PerformanceMonitor) getScoreRange(score float64) string {
	switch {
	case score >= 90:
		return "90-100"
	case score >= 80:
		return "80-89"
	case score >= 70:
		return "70-79"
	case score >= 60:
		return "60-69"
	case score >= 50:
		return "50-59"
	case score >= 40:
		return "40-49"
	case score >= 30:
		return "30-39"
	case score >= 20:
		return "20-29"
	case score >= 10:
		return "10-19"
	default:
		return "0-9"
	}
}

// updateWindowStats 更新时间窗口统计
func (pm *Gate2PerformanceMonitor) updateWindowStats(
	executionTime float64, 
	longAnchors, shortAnchors []AnchorCandidate,
	success bool,
) {
	// TODO: 实现基于时间窗口的统计
	// 这里需要维护时间序列数据，为了简化先跳过
}

// recordError 记录错误
func (pm *Gate2PerformanceMonitor) recordError(err error, symbol, module string) {
	stats := pm.stats.ErrorStats
	
	errorType := fmt.Sprintf("%T", err)
	stats.ErrorsByType[errorType]++
	stats.ErrorsByModule[module]++
	
	// 添加到最近错误列表
	errorEntry := ErrorEntry{
		Timestamp: time.Now(),
		Module:    module,
		Error:     err.Error(),
		Symbol:    symbol,
	}
	
	stats.RecentErrors = append(stats.RecentErrors, errorEntry)
	
	// 保持最近错误列表不超过100条
	if len(stats.RecentErrors) > 100 {
		stats.RecentErrors = stats.RecentErrors[1:]
	}
	
	// 更新最常见错误
	maxCount := int64(0)
	for errorType, count := range stats.ErrorsByType {
		if count > maxCount {
			maxCount = count
			stats.MostCommonError = errorType
		}
	}
}

// GetStats 获取性能统计（只读副本）
func (pm *Gate2PerformanceMonitor) GetStats() *Gate2PerformanceStats {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	// 返回深拷贝以避免并发问题
	statsJSON, _ := json.Marshal(pm.stats)
	var statsCopy Gate2PerformanceStats
	json.Unmarshal(statsJSON, &statsCopy)
	
	return &statsCopy
}

// logPerformanceSummary 输出性能摘要日志
func (pm *Gate2PerformanceMonitor) logPerformanceSummary() {
	stats := pm.stats
	
	log.Printf("📊 [Gate2性能监控] 执行摘要:")
	log.Printf("  总执行次数: %d, 成功率: %.1f%%, 平均耗时: %.2fms", 
		stats.TotalExecutions, stats.SuccessRate*100, stats.AverageExecutionTime)
		
	log.Printf("  模块耗时: AnchorEngine=%.2fms, StructClassifier=%.2fms, TriggerDetector=%.2fms",
		stats.AnchorEngineTime.AverageTime, 
		stats.StructClassifierTime.AverageTime,
		stats.TriggerDetectorTime.AverageTime)
		
	log.Printf("  锚点统计: 平均多头=%.1f, 平均空头=%.1f, 平均评分=%.1f",
		stats.AnchorStats.AverageLongAnchors,
		stats.AnchorStats.AverageShortAnchors,
		stats.AnchorStats.AverageScore)
		
	if stats.ErrorExecutions > 0 {
		log.Printf("  错误统计: 总错误=%d, 错误率=%.2f%%, 最常见错误=%s",
			stats.ErrorExecutions,
			float64(stats.ErrorExecutions)/float64(stats.TotalExecutions)*100,
			stats.ErrorStats.MostCommonError)
	}
}

// LogAnchorScoreDetails 详细锚点评分统计日志
func (pm *Gate2PerformanceMonitor) LogAnchorScoreDetails(symbol string, longAnchors, shortAnchors []AnchorCandidate) {
	featureManager := GetGate2FeatureManager()
	if !featureManager.IsPerformanceLogEnabled() {
		return
	}
	
	allAnchors := append(longAnchors, shortAnchors...)
	if len(allAnchors) == 0 {
		log.Printf("📊 [Gate2锚点统计] %s: 无锚点数据", symbol)
		return
	}
	
	// 统计不同评分段的锚点数量
	scoreRanges := make(map[string]int)
	var totalScore float64
	var maxScore, minScore float64 = 0, 100
	
	// 统计优先级分布
	priorityCount := make(map[int]int)
	
	// 统计类型分布
	typeCount := make(map[string]int)
	
	// 统计时间框架分布
	tfCount := make(map[string]int)
	
	for _, anchor := range allAnchors {
		// 评分统计
		totalScore += anchor.AnchorScore
		if anchor.AnchorScore > maxScore {
			maxScore = anchor.AnchorScore
		}
		if anchor.AnchorScore < minScore {
			minScore = anchor.AnchorScore
		}
		
		scoreRange := pm.getScoreRange(anchor.AnchorScore)
		scoreRanges[scoreRange]++
		
		// 优先级统计
		priorityCount[anchor.PriorityRank]++
		
		// 类型统计
		typeCount[string(anchor.Type)]++
		
		// 时间框架统计
		tfCount[anchor.TF]++
	}
	
	avgScore := totalScore / float64(len(allAnchors))
	
	log.Printf("📊 [Gate2锚点统计] %s 锚点评分详情:", symbol)
	log.Printf("  总锚点数: %d (多头: %d, 空头: %d)", 
		len(allAnchors), len(longAnchors), len(shortAnchors))
	log.Printf("  评分统计: 平均=%.1f, 最高=%.1f, 最低=%.1f", avgScore, maxScore, minScore)
	
	// 输出评分分布
	log.Printf("  评分分布:")
	for score := 90; score >= 0; score -= 10 {
		rangeKey := fmt.Sprintf("%d-%d", score, score+9)
		if score >= 90 {
			rangeKey = "90-100"
		}
		if count, exists := scoreRanges[rangeKey]; exists && count > 0 {
			percentage := float64(count) / float64(len(allAnchors)) * 100
			log.Printf("    %s分: %d个 (%.1f%%)", rangeKey, count, percentage)
		}
	}
	
	// 输出优先级分布
	log.Printf("  优先级分布:")
	for p := 1; p <= 7; p++ {
		if count, exists := priorityCount[p]; exists && count > 0 {
			percentage := float64(count) / float64(len(allAnchors)) * 100
			log.Printf("    P%d: %d个 (%.1f%%)", p, count, percentage)
		}
	}
	
	// 输出类型分布
	log.Printf("  类型分布:")
	for anchorType, count := range typeCount {
		if count > 0 {
			percentage := float64(count) / float64(len(allAnchors)) * 100
			log.Printf("    %s: %d个 (%.1f%%)", anchorType, count, percentage)
		}
	}
	
	// 输出时间框架分布
	log.Printf("  时间框架分布:")
	for tf, count := range tfCount {
		if count > 0 {
			percentage := float64(count) / float64(len(allAnchors)) * 100
			log.Printf("    %s: %d个 (%.1f%%)", tf, count, percentage)
		}
	}
	
	// 输出质量评估
	highQuality := 0
	mediumQuality := 0
	lowQuality := 0
	
	for _, anchor := range allAnchors {
		if anchor.AnchorScore >= 80 {
			highQuality++
		} else if anchor.AnchorScore >= 60 {
			mediumQuality++
		} else {
			lowQuality++
		}
	}
	
	log.Printf("  质量评估: 高质量(>=80)=%d, 中质量(60-79)=%d, 低质量(<60)=%d",
		highQuality, mediumQuality, lowQuality)
}

// ResetStats 重置统计数据
func (pm *Gate2PerformanceMonitor) ResetStats() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	
	log.Printf("🔄 [Gate2性能监控] 重置统计数据")
	pm.stats = &Gate2PerformanceStats{
		AnchorEngineTime:     &ModuleTimeStats{MinTime: 999999},
		StructClassifierTime: &ModuleTimeStats{MinTime: 999999},
		TriggerDetectorTime:  &ModuleTimeStats{MinTime: 999999},
		AnchorStats: &AnchorStatistics{
			ScoreDistribution:     make(map[string]int64),
			PriorityDistribution:  make(map[int]int64),
			TypeDistribution:      make(map[string]int64),
			TimeframeDistribution: make(map[string]int64),
		},
		MemoryStats: &MemoryStatistics{},
		ErrorStats: &ErrorStatistics{
			ErrorsByType:   make(map[string]int64),
			ErrorsByModule: make(map[string]int64),
			RecentErrors:   make([]ErrorEntry, 0),
		},
		WindowStats: &WindowStatistics{
			Last1Hour:   &WindowPeriodStats{},
			Last6Hours:  &WindowPeriodStats{},
			Last24Hours: &WindowPeriodStats{},
		},
		MinExecutionTime: 999999,
		LastUpdated:      time.Now(),
	}
}

// ExportStats 导出统计数据为JSON
func (pm *Gate2PerformanceMonitor) ExportStats() ([]byte, error) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	return json.MarshalIndent(pm.stats, "", "  ")
}