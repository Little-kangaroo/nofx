package market

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"
)

// AnchorType 锚点类型枚举
type AnchorType string

const (
	AnchorHTFZone   AnchorType = "HTF_ZONE"      // 4h/1h supply/demand
	AnchorVPVRBound AnchorType = "VPVR_BOUND"    // VAH/VAL retest
	AnchorSRFlip    AnchorType = "SR_FLIP"       // 支撑阻力转换
	AnchorSRLevel   AnchorType = "SR_LEVEL"      // 🔥 P0-05B新增：普通支撑阻力位
	AnchorZoneMTF   AnchorType = "MTF_ZONE"      // 30m/15m zone
	AnchorFVG       AnchorType = "FVG"           // Fair Value Gap
	AnchorFib       AnchorType = "FIB"           // Fibonacci levels
)

// AnchorCandidate 统一的结构锚点候选模型
// 将供需区、VPVR、SR、FVG、Fib的输出统一为可排序、可评分的候选锚点
type AnchorCandidate struct {
	Dir       string     `json:"dir"`        // "LONG" or "SHORT"
	Type      AnchorType `json:"type"`       // 锚点类型
	TF        string     `json:"tf"`         // "4h","1h","30m","15m","5m"
	Level     float64    `json:"level"`      // 锚点价格（核心）
	BandLo    float64    `json:"band_lo"`    // 区间结构：下沿；点位结构：=Level
	BandHi    float64    `json:"band_hi"`    // 区间结构：上沿；点位结构：=Level

	IsFresh   bool     `json:"is_fresh"`    // fresh / recently touched
	StrengthZ *float64 `json:"strength_z"`  // 结构强度 z-score（可空）
	VolRatio  *float64 `json:"vol_ratio"`   // 量能相对比（可空）

	// 评分相关
	PriorityRank int     `json:"priority_rank"` // P1-P7 优先级排名
	AnchorScore  float64 `json:"anchor_score"`  // 0-100 综合评分

	Meta map[string]interface{} `json:"meta"` // 形成时间、触碰次数、来源ID、额外证据
}

// ScoreBreakdown 评分明细，用于可复盘分析
type ScoreBreakdown struct {
	Base             float64 `json:"base"`              // 基础分
	TFBonus          float64 `json:"tf_bonus"`          // 时间框架加分
	FreshBonus       float64 `json:"fresh_bonus"`       // 新鲜度加分
	StrengthBonus    float64 `json:"strength_bonus"`    // 强度加分
	VolBonus         float64 `json:"vol_bonus"`         // 量能加分
	AgingPenalty     float64 `json:"aging_penalty"`     // 老化惩罚
	WeakPenalty      float64 `json:"weak_penalty"`      // 弱势惩罚
	ProximityPenalty float64 `json:"proximity_penalty"` // 距离惩罚
	Total            float64 `json:"total"`             // 总分
}

// AnchorEngine 锚点引擎
type AnchorEngine struct {
	config *AnchorEngineConfig
}

// AnchorEngineConfig 锚点引擎配置
type AnchorEngineConfig struct {
	MaxCandidatesPerDir int     `json:"max_candidates_per_dir"` // 每个方向最多保留候选数
	MinAnchorScore      float64 `json:"min_anchor_score"`       // 最低锚点评分阈值
	WeakStrengthZ       float64 `json:"weak_strength_z"`        // 弱锚点strength_z阈值(-0.5)
	MaxProximityATR     float64 `json:"max_proximity_atr"`      // 最大有效距离（ATR倍数）
}

// NewAnchorEngine 创建锚点引擎
func NewAnchorEngine(config *AnchorEngineConfig) *AnchorEngine {
	if config == nil {
		config = &AnchorEngineConfig{
			MaxCandidatesPerDir: 5,
			MinAnchorScore:      30.0,
			WeakStrengthZ:       -0.5,
			MaxProximityATR:     8.0,
		}
	}
	
	return &AnchorEngine{
		config: config,
	}
}

// effectiveDistance 计算价格到锚点的有效距离（Zone边界距离）
// 对于Zone类型（BandLo != BandHi），使用价格到Zone边界的距离
// 对于Point类型（BandLo == BandHi），退化为价格到Level的距离
//
// 返回值说明：
// - 价格在Zone内部：返回0
// - 价格在Zone外部：返回到最近边界的距离（正值）
func effectiveDistance(price float64, c AnchorCandidate) float64 {
	lo, hi := c.BandLo, c.BandHi

	// Edge case 1: 点位结构（BandLo == BandHi）
	// 使用浮点比较容忍度1e-9避免精度问题
	if math.Abs(lo-hi) < 1e-9 {
		return math.Abs(price - lo)
	}

	// Edge case 2: BandLo > BandHi（数据错误，交换修正）
	if lo > hi {
		lo, hi = hi, lo
	}

	// Edge case 3: 价格在Zone内部（返回0）
	if price >= lo && price <= hi {
		return 0
	}

	// Edge case 4: 价格在Zone下方（返回到下沿的距离）
	if price < lo {
		return lo - price
	}

	// Edge case 5: 价格在Zone上方（返回到上沿的距离）
	return price - hi
}

// ProcessCandidates 处理锚点候选：映射->过滤->排序->选择
func (ae *AnchorEngine) ProcessCandidates(
	supplyDemandData *SupplyDemandData,
	vpvrData *VolumeProfile,
	srData *SupportResistanceData,
	fvgData *FVGData,
	fibData *FibonacciData,
	lastPrice float64,
	atr14 float64,
	timeframes map[string][]Kline,
) (topLong, topShort []AnchorCandidate, err error) {

	start := time.Now()
	
	// 1. 从各结构分析器收集候选
	var allCandidates []AnchorCandidate
	
	// 从供需区提取候选
	if supplyDemandData != nil {
		sdCandidates := ae.fromSupplyDemand(supplyDemandData, timeframes)
		allCandidates = append(allCandidates, sdCandidates...)
		log.Printf("🔍 [AnchorEngine] 从供需区提取 %d 个候选", len(sdCandidates))
	}
	
	// 从VPVR提取候选
	if vpvrData != nil {
		vpvrCandidates := ae.fromVPVR(vpvrData, timeframes)
		allCandidates = append(allCandidates, vpvrCandidates...)
		log.Printf("🔍 [AnchorEngine] 从VPVR提取 %d 个候选", len(vpvrCandidates))
	}
	
	// 从支撑阻力转换线提取候选
	if srData != nil {
		srCandidates := ae.fromSR(srData, timeframes)
		allCandidates = append(allCandidates, srCandidates...)
		log.Printf("🔍 [AnchorEngine] 从SR提取 %d 个候选", len(srCandidates))
	}
	
	// 从FVG提取候选
	if fvgData != nil {
		fvgCandidates := ae.fromFVG(fvgData, timeframes)
		allCandidates = append(allCandidates, fvgCandidates...)
		log.Printf("🔍 [AnchorEngine] 从FVG提取 %d 个候选", len(fvgCandidates))
	}
	
	// 从斐波纳契提取候选
	if fibData != nil {
		fibCandidates := ae.fromFib(fibData, timeframes)
		allCandidates = append(allCandidates, fibCandidates...)
		log.Printf("🔍 [AnchorEngine] 从Fibonacci提取 %d 个候选", len(fibCandidates))
	}

	log.Printf("🔍 [AnchorEngine] 收集到 %d 个原始候选锚点", len(allCandidates))

	// 2. 方向一致性过滤
	var validCandidates []AnchorCandidate
	for _, candidate := range allCandidates {
		if ae.directionalFilter(candidate, lastPrice, atr14) {
			validCandidates = append(validCandidates, candidate)
		}
	}
	
	log.Printf("🔍 [AnchorEngine] 方向过滤后剩余 %d 个有效候选", len(validCandidates))

	// 3. 计算优先级和评分
	for i := range validCandidates {
		validCandidates[i].PriorityRank = ae.priorityRank(validCandidates[i])
		score, breakdown := ae.computeAnchorScore(validCandidates[i], lastPrice, atr14)
		validCandidates[i].AnchorScore = score
		
		// 将评分明细添加到Meta中
		if validCandidates[i].Meta == nil {
			validCandidates[i].Meta = make(map[string]interface{})
		}
		validCandidates[i].Meta["score_breakdown"] = breakdown
	}

	// 4. 按方向分组
	var longCandidates, shortCandidates []AnchorCandidate
	filteredCount := 0
	for _, candidate := range validCandidates {
		if candidate.AnchorScore < ae.config.MinAnchorScore {
			filteredCount++
			continue // 过滤低分候选
		}
		
		switch candidate.Dir {
		case "LONG":
			longCandidates = append(longCandidates, candidate)
		case "SHORT":
			shortCandidates = append(shortCandidates, candidate)
		}
	}
	
	if filteredCount > 0 {
		log.Printf("🔍 [AnchorEngine] 过滤低分候选 %d 个 (< %.1f)", filteredCount, ae.config.MinAnchorScore)
	}

	// 5. 排序：Priority决胜，Score细分
	sort.Slice(longCandidates, func(i, j int) bool {
		return ae.compareCandidates(longCandidates[i], longCandidates[j], lastPrice)
	})
	
	sort.Slice(shortCandidates, func(i, j int) bool {
		return ae.compareCandidates(shortCandidates[i], shortCandidates[j], lastPrice)
	})

	// 6. 选择Top N
	maxCount := ae.config.MaxCandidatesPerDir
	if len(longCandidates) > maxCount {
		topLong = longCandidates[:maxCount]
	} else {
		topLong = longCandidates
	}
	
	if len(shortCandidates) > maxCount {
		topShort = shortCandidates[:maxCount]
	} else {
		topShort = shortCandidates
	}

	duration := time.Since(start)
	log.Printf("🎯 [AnchorEngine] 最终选择: LONG %d个, SHORT %d个, 耗时: %v", 
		len(topLong), len(topShort), duration)
	
	// 记录最佳候选信息
	if len(topLong) > 0 {
		best := topLong[0]
		log.Printf("📍 [AnchorEngine] Best LONG: %s %s@%.4f (P%d, Score:%.1f)", 
			best.Type, best.TF, best.Level, best.PriorityRank, best.AnchorScore)
	}
	if len(topShort) > 0 {
		best := topShort[0]
		log.Printf("📍 [AnchorEngine] Best SHORT: %s %s@%.4f (P%d, Score:%.1f)", 
			best.Type, best.TF, best.Level, best.PriorityRank, best.AnchorScore)
	}
	
	return topLong, topShort, nil
}

// compareCandidates 候选比较函数：Priority -> Score -> Distance
func (ae *AnchorEngine) compareCandidates(a, b AnchorCandidate, lastPrice float64) bool {
	// 1. Priority决胜 (数值越小优先级越高)
	if a.PriorityRank != b.PriorityRank {
		return a.PriorityRank < b.PriorityRank
	}
	
	// 2. Score细分 (分数越高越优先)
	if a.AnchorScore != b.AnchorScore {
		return a.AnchorScore > b.AnchorScore
	}
	
	// 3. 距离细分 (更近优先) - 使用有效距离而非Level距离
	distA := effectiveDistance(lastPrice, a)
	distB := effectiveDistance(lastPrice, b)
	return distA < distB
}

// directionalFilter 方向一致性过滤（改进版：使用Zone边界）
// LONG候选：价格必须 >= BandLo（允许容忍度0.1*ATR）
// SHORT候选：价格必须 <= BandHi（允许容忍度0.1*ATR）
func (ae *AnchorEngine) directionalFilter(c AnchorCandidate, lastPrice float64, atr14 float64) bool {
	lo, hi := c.BandLo, c.BandHi

	// 修正BandLo > BandHi的情况
	if lo > hi {
		lo, hi = hi, lo
	}

	// 容忍度：0.1*ATR（避免因微小误差过滤掉边缘触碰）
	tolerance := 0.0
	if atr14 > 0 {
		tolerance = 0.1 * atr14
	}

	switch c.Dir {
	case "LONG":
		// LONG候选：价格应在Zone下方或Zone内（支撑作用）
		// 使用BandLo作为判断基准（而非Level）
		return lastPrice >= lo-tolerance
	case "SHORT":
		// SHORT候选：价格应在Zone上方或Zone内（阻力作用）
		// 使用BandHi作为判断基准（而非Level）
		return lastPrice <= hi+tolerance
	default:
		return false
	}
}

// priorityRank 锚点优先级P1~P7
func (ae *AnchorEngine) priorityRank(c AnchorCandidate) int {
	switch c.Type {
	case AnchorHTFZone:   return 1 // P1: HTF供需区
	case AnchorVPVRBound: return 2 // P2: VPVR边界
	case AnchorSRFlip:    return 3 // P3: 支撑阻力转换
	case AnchorZoneMTF:   return 4 // P4: MTF区域
	case AnchorFVG:       return 6 // P6: FVG
	case AnchorFib:       return 7 // P7: 斐波纳契
	default:              return 99
	}
}

// computeAnchorScore 计算锚点评分（0~100）与可复盘Breakdown
func (ae *AnchorEngine) computeAnchorScore(c AnchorCandidate, lastPrice, atr14 float64) (float64, ScoreBreakdown) {
	breakdown := ScoreBreakdown{}
	
	// 基础分数
	breakdown.Base = 40.0
	
	// 时间框架奖励 (4h>1h>30m>15m>5m)
	switch c.TF {
	case "4h":
		breakdown.TFBonus = 20.0
	case "1h":
		breakdown.TFBonus = 15.0
	case "30m":
		breakdown.TFBonus = 10.0
	case "15m":
		breakdown.TFBonus = 5.0
	case "5m":
		breakdown.TFBonus = 2.0
	default:
		breakdown.TFBonus = 0.0
	}
	
	// 新鲜度奖励 (fresh +12)
	if c.IsFresh {
		breakdown.FreshBonus = 12.0
	}
	
	// 强度奖励/惩罚
	if c.StrengthZ != nil {
		strengthZ := *c.StrengthZ
		if strengthZ < ae.config.WeakStrengthZ {
			// strength_z < -0.5：弱锚点严重惩罚
			breakdown.WeakPenalty = -30.0
		} else if strengthZ > 2.0 {
			breakdown.StrengthBonus = 15.0
		} else if strengthZ > 1.0 {
			breakdown.StrengthBonus = 10.0
		} else if strengthZ > 0.5 {
			breakdown.StrengthBonus = 5.0
		}
	}
	
	// 量能奖励 (分段加分)
	if c.VolRatio != nil {
		volRatio := *c.VolRatio
		if volRatio > 2.0 {
			breakdown.VolBonus = 10.0
		} else if volRatio > 1.5 {
			breakdown.VolBonus = 6.0
		} else if volRatio > 1.2 {
			breakdown.VolBonus = 3.0
		}
	}
	
	// 距离惩罚（ATR归一化）- 使用有效距离而非Level距离
	if atr14 > 0 {
		distance := effectiveDistance(lastPrice, c)
		distanceATR := distance / atr14

		if distanceATR > ae.config.MaxProximityATR {
			breakdown.ProximityPenalty = -20.0 // 过远严重惩罚
		} else if distanceATR > 4.0 {
			breakdown.ProximityPenalty = -10.0
		} else if distanceATR > 2.0 {
			breakdown.ProximityPenalty = -5.0
		}
		// 距离适中或很近，无惩罚

		// 记录距离信息到Meta
		// 这里不能直接修改c，在调用处处理
	}
	
	// 计算总分
	breakdown.Total = breakdown.Base + breakdown.TFBonus + breakdown.FreshBonus + 
		breakdown.StrengthBonus + breakdown.VolBonus + breakdown.AgingPenalty + 
		breakdown.WeakPenalty + breakdown.ProximityPenalty
	
	// 限制在0-100范围
	breakdown.Total = math.Max(0, math.Min(100, breakdown.Total))
	
	return breakdown.Total, breakdown
}

// GetBestAnchor 获取指定方向的最佳锚点
func GetBestAnchor(candidates []AnchorCandidate, dir string) *AnchorCandidate {
	for _, candidate := range candidates {
		if candidate.Dir == dir {
			return &candidate
		}
	}
	return nil
}

// GetNearestOppositeAnchor 获取最近的对侧锚点
func GetNearestOppositeAnchor(candidates []AnchorCandidate, dir string, currentPrice float64) *AnchorCandidate {
	var oppositeDir string
	if dir == "LONG" {
		oppositeDir = "SHORT"
	} else {
		oppositeDir = "LONG"
	}
	
	var nearestCandidate *AnchorCandidate
	minDistance := math.Inf(1)
	
	for _, candidate := range candidates {
		if candidate.Dir == oppositeDir {
			distance := math.Abs(candidate.Level - currentPrice)
			if distance < minDistance {
				minDistance = distance
				nearestCandidate = &candidate
			}
		}
	}
	
	return nearestCandidate
}

// FormatAnchorSummary 格式化锚点摘要信息
func FormatAnchorSummary(anchor *AnchorCandidate) string {
	if anchor == nil {
		return "None"
	}
	
	return fmt.Sprintf("%s %s@%.4f (P%d, Score:%.1f, Fresh:%v)", 
		anchor.Type, anchor.TF, anchor.Level, anchor.PriorityRank, anchor.AnchorScore, anchor.IsFresh)
}