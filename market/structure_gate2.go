package market

import (
	"log"
	"math"
	"time"
)

// StructureGate2 Gate2结构指标聚合输出模型
type StructureGate2 struct {
	// 基础信息
	Symbol      string    `json:"symbol"`       // 交易对
	Timestamp   time.Time `json:"timestamp"`    // 分析时间
	LastPrice   float64   `json:"last_price"`   // 最新价格（统一字段名）
	ATR14       float64   `json:"atr14"`        // ATR14值
	
	// 锚点聚合结果
	TopAnchorsLong  []AnchorCandidate `json:"top_anchors_long"`  // 前3-5个多头锚点
	TopAnchorsShort []AnchorCandidate `json:"top_anchors_short"` // 前3-5个空头锚点
	BestAnchorLong  *AnchorCandidate  `json:"best_anchor_long"`  // 最佳多头锚点
	BestAnchorShort *AnchorCandidate  `json:"best_anchor_short"` // 最佳空头锚点
	
	// 结构状态分类
	StructStateLong  StructState `json:"struct_state_long"`  // 多头结构状态
	StructStateShort StructState `json:"struct_state_short"` // 空头结构状态
	
	// 结构状态详情
	StructStateResult *StructStateResult `json:"struct_state_result"` // 完整结构状态分析
	
	// 触发检测（如果有5分钟触发）
	TriggerResult *TriggerResult `json:"trigger_result,omitempty"` // 5分钟触发检测结果
	
	// 评分明细（用于复盘）
	AnchorScoreBreakdown map[string]interface{} `json:"anchor_score_breakdown"` // 锚点评分明细
	
	// 时间锚点信息（V-13.5规范）
	TriggerContext *TriggerContextInfo `json:"trigger_context"` // 触发上下文
	
	// 统计信息
	TotalCandidatesLong  int     `json:"total_candidates_long"`  // 多头候选总数
	TotalCandidatesShort int     `json:"total_candidates_short"` // 空头候选总数
	ProcessingTimeMs     float64 `json:"processing_time_ms"`     // 处理耗时（毫秒）
	
	// 数据版本标识
	Version  string `json:"version"`   // "Gate2-V13.5"
	Revision string `json:"revision"`  // 实现版本号
}

// Gate2CompactPayload Gate2紧凑AI输出格式
type Gate2CompactPayload struct {
	// 🔥 V-13.5核心字段：结构状态（防止AI自解读）
	StructStateLong  string `json:"struct_state_long"`  // "FAVORABLE"/"NEUTRAL"/"DANGER"
	StructStateShort string `json:"struct_state_short"` // "FAVORABLE"/"NEUTRAL"/"DANGER"
	
	// 🔥 V-13.5核心字段：最佳锚点
	BestAnchorLong  *Gate2AnchorInfo `json:"best_anchor_long,omitempty"`
	BestAnchorShort *Gate2AnchorInfo `json:"best_anchor_short,omitempty"`
	
	// 🔥 V-13.5核心字段：时间锚点信息
	TriggerContext *TriggerContextInfo `json:"trigger_context"`
	
	// 统一价格字段（避免双契约）
	LastPrice float64 `json:"last_price"` // 替代current_price
	
	// 核心评分信息
	TopAnchorsLong  []Gate2AnchorInfo `json:"top_anchors_long,omitempty"`  // 前3个
	TopAnchorsShort []Gate2AnchorInfo `json:"top_anchors_short,omitempty"` // 前3个
	
	// 可选：5分钟触发（如果检测到）
	FiveMinTrigger *Gate2TriggerInfo `json:"5m_trigger,omitempty"`
}

// Gate2StructuredPayload Gate2结构化AI输出格式（完整版）
type Gate2StructuredPayload struct {
	// 继承紧凑格式的所有字段
	Gate2CompactPayload
	
	// 额外的结构化信息
	AnchorScoreBreakdown map[string]interface{} `json:"anchor_score_breakdown"` // 评分明细
	StructStateEvidence  *StructStateEvidence   `json:"struct_state_evidence"`  // 状态判定证据
	
	// 统计和性能信息
	Stats *Gate2Stats `json:"stats"`
}

// Gate2AnchorInfo 紧凑锚点信息
type Gate2AnchorInfo struct {
	Type         string  `json:"type"`          // "HTF_ZONE"/"VPVR_BOUND"等
	Level        float64 `json:"level"`         // 锚点价格
	TimeFrame    string  `json:"timeframe"`     // "4h"/"1h"等
	PriorityRank int     `json:"priority_rank"` // P1-P7
	AnchorScore  float64 `json:"anchor_score"`  // 0-100评分
	DistanceATR  float64 `json:"distance_atr"`  // ATR归一化距离
	IsFresh      bool    `json:"is_fresh"`      // 是否新鲜
}

// Gate2TriggerInfo 5分钟触发信息
type Gate2TriggerInfo struct {
	IsTriggered  bool    `json:"is_triggered"`   // 是否触发
	Direction    string  `json:"direction"`      // "LONG"/"SHORT"
	TriggerType  string  `json:"trigger_type"`   // "BODY_BREAKOUT"等
	Confidence   float64 `json:"confidence"`     // 置信度
	AnchorLevel  float64 `json:"anchor_level"`   // 被突破的锚点价格
	TriggerTime  string  `json:"trigger_time"`   // 触发时间字符串
}

// Gate2Stats Gate2统计信息
type Gate2Stats struct {
	TotalCandidatesLong    int     `json:"total_candidates_long"`
	TotalCandidatesShort   int     `json:"total_candidates_short"`
	HighQualityAnchorsLong int     `json:"high_quality_anchors_long"`  // >70分
	HighQualityAnchorsShort int    `json:"high_quality_anchors_short"` // >70分
	ProcessingTimeMs       float64 `json:"processing_time_ms"`
	DataQuality            string  `json:"data_quality"`    // "GOOD"/"FAIR"/"POOR"
	Version                string  `json:"version"`         // "Gate2-V13.5"
}

// ToCompactPayload 转换为紧凑AI输出格式
func (sg2 *StructureGate2) ToCompactPayload() *Gate2CompactPayload {
	payload := &Gate2CompactPayload{
		StructStateLong:  string(sg2.StructStateLong),
		StructStateShort: string(sg2.StructStateShort),
		LastPrice:        sg2.LastPrice,
		TriggerContext:   sg2.TriggerContext,
	}
	
	// 转换最佳锚点
	if sg2.BestAnchorLong != nil {
		payload.BestAnchorLong = convertToGate2AnchorInfo(sg2.BestAnchorLong, sg2.LastPrice, sg2.ATR14)
	}
	if sg2.BestAnchorShort != nil {
		payload.BestAnchorShort = convertToGate2AnchorInfo(sg2.BestAnchorShort, sg2.LastPrice, sg2.ATR14)
	}
	
	// 转换前3个锚点
	maxAnchors := 3
	for i, anchor := range sg2.TopAnchorsLong {
		if i >= maxAnchors {
			break
		}
		payload.TopAnchorsLong = append(payload.TopAnchorsLong, 
			*convertToGate2AnchorInfo(&anchor, sg2.LastPrice, sg2.ATR14))
	}
	
	for i, anchor := range sg2.TopAnchorsShort {
		if i >= maxAnchors {
			break
		}
		payload.TopAnchorsShort = append(payload.TopAnchorsShort, 
			*convertToGate2AnchorInfo(&anchor, sg2.LastPrice, sg2.ATR14))
	}
	
	// 转换5分钟触发信息
	if sg2.TriggerResult != nil && sg2.TriggerResult.IsTriggered {
		payload.FiveMinTrigger = &Gate2TriggerInfo{
			IsTriggered: sg2.TriggerResult.IsTriggered,
			Direction:   sg2.TriggerResult.Direction,
			TriggerType: string(sg2.TriggerResult.TriggerType),
			Confidence:  sg2.TriggerResult.Confidence,
			AnchorLevel: sg2.TriggerResult.TriggeredAnchor.Level,
			TriggerTime: sg2.TriggerResult.TriggerTime.Format("15:04:05"),
		}
	}
	
	return payload
}

// ToStructuredPayload 转换为结构化AI输出格式
func (sg2 *StructureGate2) ToStructuredPayload() *Gate2StructuredPayload {
	compact := sg2.ToCompactPayload()
	
	payload := &Gate2StructuredPayload{
		Gate2CompactPayload: *compact,
		AnchorScoreBreakdown: sg2.AnchorScoreBreakdown,
	}
	
	// 添加结构状态证据
	if sg2.StructStateResult != nil {
		if sg2.StructStateResult.Long.Evidence != nil {
			payload.StructStateEvidence = sg2.StructStateResult.Long.Evidence
		} else if sg2.StructStateResult.Short.Evidence != nil {
			payload.StructStateEvidence = sg2.StructStateResult.Short.Evidence
		}
	}
	
	// 统计信息
	payload.Stats = &Gate2Stats{
		TotalCandidatesLong:  sg2.TotalCandidatesLong,
		TotalCandidatesShort: sg2.TotalCandidatesShort,
		ProcessingTimeMs:     sg2.ProcessingTimeMs,
		Version:              sg2.Version,
		DataQuality:          determineDataQuality(sg2),
	}
	
	// 统计高质量锚点
	for _, anchor := range sg2.TopAnchorsLong {
		if anchor.AnchorScore >= 70.0 {
			payload.Stats.HighQualityAnchorsLong++
		}
	}
	for _, anchor := range sg2.TopAnchorsShort {
		if anchor.AnchorScore >= 70.0 {
			payload.Stats.HighQualityAnchorsShort++
		}
	}
	
	return payload
}

// convertToGate2AnchorInfo 转换为Gate2锚点信息
// 🔥 P0-2&P0-3修复：修复distance_atr大量为0的距离计算缺陷，使用统一ATR管理器
func convertToGate2AnchorInfo(anchor *AnchorCandidate, lastPrice, atr14 float64) *Gate2AnchorInfo {
	if anchor == nil {
		return nil
	}
	
	// 🔥 P0-2&P0-3修复：使用统一ATR管理器计算距离，确保一致性
	var distanceATR float64
	var distanceMode string
	
	if atr14 > 0 {
		distanceATR = math.Abs(anchor.Level - lastPrice) / atr14
		distanceMode = "atr"
	} else {
		// 使用统一的fallback机制
		_ = GetGlobalATRManager() // ATR manager for consistency
		fallbackRate := 0.005 // 默认0.5%
		if anchor.TF != "" {
			// 根据锚点时间框架使用不同的fallback率
			switch anchor.TF {
			case "5m":
				fallbackRate = 0.002
			case "15m":
				fallbackRate = 0.003
			case "30m":
				fallbackRate = 0.004
			case "1h":
				fallbackRate = 0.005
			case "4h":
				fallbackRate = 0.008
			}
		}
		
		fallbackATR := lastPrice * fallbackRate
		distanceATR = math.Abs(anchor.Level - lastPrice) / fallbackATR
		distanceMode = "pct_fallback"
		
		log.Printf("⚠️ [P0-2] ATR无效(%.4f)，距离计算降级为%s百分比模式: %.4f ATR (锚点: %s@%.4f)", 
			atr14, anchor.TF, distanceATR, anchor.Type, anchor.Level)
	}
	
	// 在Meta中标注距离计算模式，用于后续分析
	meta := anchor.Meta
	if meta == nil {
		meta = make(map[string]interface{})
	}
	meta["distance_mode"] = distanceMode
	meta["atr14_used"] = atr14
	
	return &Gate2AnchorInfo{
		Type:         string(anchor.Type),
		Level:        anchor.Level,
		TimeFrame:    anchor.TF,
		PriorityRank: anchor.PriorityRank,
		AnchorScore:  anchor.AnchorScore,
		DistanceATR:  distanceATR,
		IsFresh:      anchor.IsFresh,
	}
}

// determineDataQuality 确定数据质量
func determineDataQuality(sg2 *StructureGate2) string {
	totalAnchors := sg2.TotalCandidatesLong + sg2.TotalCandidatesShort
	
	if totalAnchors >= 10 {
		return "GOOD"
	} else if totalAnchors >= 5 {
		return "FAIR"
	} else {
		return "POOR"
	}
}

// abs function removed - using math.Abs from math package