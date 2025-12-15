package market

import (
	"fmt"
	"log"
	"math"
	"time"
)

// TriggerType 触发类型
type TriggerType string

const (
	TriggerBodyBreakout TriggerType = "BODY_BREAKOUT" // 实体突破
	TriggerVolumeSpike  TriggerType = "VOLUME_SPIKE"  // 量能突破
	TriggerStrengthConfirm TriggerType = "STRENGTH_CONFIRM" // 强度确认
)

// TriggerResult 5分钟触发检测结果
type TriggerResult struct {
	IsTriggered     bool                    `json:"is_triggered"`      // 是否触发
	TriggerType     TriggerType            `json:"trigger_type"`       // 触发类型
	TriggerTime     time.Time              `json:"trigger_time"`       // 触发时间
	Direction       string                 `json:"direction"`          // "LONG" or "SHORT"
	Confidence      float64                `json:"confidence"`         // 置信度 (0-1)
	Validations     *TriggerValidations    `json:"validations"`        // 验证详情
	TriggeredAnchor *AnchorCandidate       `json:"triggered_anchor"`   // 被突破的锚点
	Evidence        *TriggerEvidence       `json:"evidence"`           // 触发证据
}

// TriggerValidations 触发验证项
type TriggerValidations struct {
	BodyBreakout    BodyBreakoutValidation `json:"body_breakout"`    // 实体突破验证
	VolumeCheck     VolumeValidation       `json:"volume_check"`     // 量能验证
	StrengthCheck   StrengthValidation     `json:"strength_check"`   // 强度验证
	TimingCheck     TimingValidation       `json:"timing_check"`     // 时间验证
}

// BodyBreakoutValidation 实体突破验证
type BodyBreakoutValidation struct {
	HasBodyBreakout  bool    `json:"has_body_breakout"`  // 是否有实体突破
	BreakoutDistance float64 `json:"breakout_distance"`  // 突破距离（ATR倍数）
	WickRatio        float64 `json:"wick_ratio"`         // 引线比例 (<0.3 for valid breakout)
	BodySize         float64 `json:"body_size"`          // 实体大小（ATR倍数）
	IsWickOnly       bool    `json:"is_wick_only"`       // 是否仅引线突破
}

// VolumeValidation 量能验证
type VolumeValidation struct {
	VolumeRatio     float64 `json:"volume_ratio"`     // 量能比例（当前/平均）
	MeetsThreshold  bool    `json:"meets_threshold"`  // 是否满足阈值(>=1.2)
	VolumeTrend     string  `json:"volume_trend"`     // "INCREASING"/"DECREASING"/"STABLE"
	VolumeIntensity float64 `json:"volume_intensity"` // 量能强度评分 (0-1)
}

// StrengthValidation 强度验证
type StrengthValidation struct {
	StrengthZ       float64 `json:"strength_z"`       // 强度Z分数
	MeetsThreshold  bool    `json:"meets_threshold"`  // 是否满足阈值(>0.5)
	StrengthTrend   string  `json:"strength_trend"`   // "STRONG"/"MEDIUM"/"WEAK"
	CVDSupport      float64 `json:"cvd_support"`      // CVD支持强度
}

// TimingValidation 时间验证
type TimingValidation struct {
	IsKlineClose     bool      `json:"is_kline_close"`     // 是否在K线收盘时触发
	TriggerLatency   int64     `json:"trigger_latency"`    // 触发延迟（毫秒）
	AnchorAge        float64   `json:"anchor_age"`         // 锚点年龄（分钟）
	IsOptimalTiming  bool      `json:"is_optimal_timing"`  // 是否最佳时机
}

// TriggerEvidence 触发证据
type TriggerEvidence struct {
	// 价格行为
	OpenPrice    float64 `json:"open_price"`    // 开盘价
	ClosePrice   float64 `json:"close_price"`   // 收盘价
	HighPrice    float64 `json:"high_price"`    // 最高价
	LowPrice     float64 `json:"low_price"`     // 最低价
	AnchorLevel  float64 `json:"anchor_level"`  // 锚点水平
	
	// 技术指标
	Volume       float64 `json:"volume"`        // 成交量
	VolumeRatio  float64 `json:"volume_ratio"`  // 量能比率
	StrengthZ    float64 `json:"strength_z"`    // 强度Z分数
	
	// 上下文信息
	PreviousCandles []CandleInfo `json:"previous_candles"` // 前几根K线信息
	MarketContext   string       `json:"market_context"`   // 市场上下文
}

// CandleInfo is defined in types.go

// TriggerDetector 5分钟触发检测器
type TriggerDetector struct {
	config *TriggerConfig
}

// TriggerConfig 触发检测配置
type TriggerConfig struct {
	// 实体突破要求
	MinBodyBreakoutATR   float64 `json:"min_body_breakout_atr"`   // 0.1 ATR 最小实体突破
	MaxWickRatio         float64 `json:"max_wick_ratio"`          // 0.3 最大引线比例
	MinBodySizeATR       float64 `json:"min_body_size_atr"`       // 0.05 ATR 最小实体大小
	
	// 量能要求
	MinVolumeRatio       float64 `json:"min_volume_ratio"`        // 1.2 最小量能比率
	VolumeWindowPeriods  int     `json:"volume_window_periods"`   // 20 量能窗口周期数
	
	// 强度要求
	MinStrengthZ         float64 `json:"min_strength_z"`          // 0.5 最小强度Z分数
	StrengthDecayHours   float64 `json:"strength_decay_hours"`    // 4.0 强度衰减时间
	
	// 时间要求
	MaxTriggerLatencyMs  int64   `json:"max_trigger_latency_ms"`  // 5000 最大触发延迟
	MaxAnchorAgeMinutes  float64 `json:"max_anchor_age_minutes"`  // 240 最大锚点年龄
	RequireKlineClose    bool    `json:"require_kline_close"`     // true 要求K线收盘触发
}

// NewTriggerDetector 创建触发检测器
func NewTriggerDetector(config *TriggerConfig) *TriggerDetector {
	if config == nil {
		config = DefaultTriggerConfig()
	}
	
	return &TriggerDetector{
		config: config,
	}
}

// DefaultTriggerConfig 默认配置
func DefaultTriggerConfig() *TriggerConfig {
	return &TriggerConfig{
		MinBodyBreakoutATR:  0.1,
		MaxWickRatio:        0.3,
		MinBodySizeATR:      0.05,
		MinVolumeRatio:      1.2,
		VolumeWindowPeriods: 20,
		MinStrengthZ:        0.5,
		StrengthDecayHours:  4.0,
		MaxTriggerLatencyMs: 5000,
		MaxAnchorAgeMinutes: 240,
		RequireKlineClose:   true,
	}
}

// DetectTrigger 检测5分钟触发（核心方法）
func (td *TriggerDetector) DetectTrigger(
	currentKline *CandleInfo,
	previousKlines []CandleInfo,
	anchors []AnchorCandidate,
	volumeRatio, strengthZ, atr14 float64,
	triggerTime time.Time,
) *TriggerResult {
	
	if currentKline == nil || len(anchors) == 0 || atr14 <= 0 {
		return &TriggerResult{
			IsTriggered: false,
			TriggerTime: triggerTime,
		}
	}
	
	// 寻找可能被触发的锚点
	triggeredAnchor, direction := td.findTriggeredAnchor(currentKline, anchors)
	if triggeredAnchor == nil {
		return &TriggerResult{
			IsTriggered: false,
			TriggerTime: triggerTime,
		}
	}
	
	// 执行所有验证
	validations := td.performValidations(
		currentKline, previousKlines, triggeredAnchor,
		volumeRatio, strengthZ, atr14, triggerTime)
	
	// 判断是否触发
	isTriggered := td.evaluateValidations(validations)
	
	// 计算置信度
	confidence := td.calculateTriggerConfidence(validations, triggeredAnchor)
	
	// 确定触发类型 - 只有在实际触发时才设置类型
	var triggerType TriggerType
	if isTriggered {
		triggerType = td.determineTriggerType(validations)
	} else {
		triggerType = ""
	}
	
	// 生成证据
	evidence := td.generateTriggerEvidence(
		currentKline, previousKlines, triggeredAnchor,
		volumeRatio, strengthZ)
	
	result := &TriggerResult{
		IsTriggered:     isTriggered,
		TriggerType:     triggerType,
		TriggerTime:     triggerTime,
		Direction:       direction,
		Confidence:      confidence,
		Validations:     validations,
		TriggeredAnchor: triggeredAnchor,
		Evidence:        evidence,
	}
	
	// 记录触发日志
	if isTriggered {
		log.Printf("🔥 [触发检测] %s方向触发! 锚点: %s@%.4f, 置信度: %.2f", 
			direction, triggeredAnchor.Type, triggeredAnchor.Level, confidence)
		LogTriggerDetails(result)
	}
	
	return result
}

// findTriggeredAnchor 寻找被触发的锚点
func (td *TriggerDetector) findTriggeredAnchor(
	kline *CandleInfo,
	anchors []AnchorCandidate,
) (*AnchorCandidate, string) {
	
	for _, anchor := range anchors {
		// 检查多头方向突破
		if anchor.Dir == "LONG" {
			// 多头突破：收盘价突破支撑上方
			if kline.Close > anchor.Level {
				return &anchor, "LONG"
			}
		}
		
		// 检查空头方向突破
		if anchor.Dir == "SHORT" {
			// 空头突破：收盘价突破阻力下方
			if kline.Close < anchor.Level {
				return &anchor, "SHORT"
			}
		}
	}
	
	return nil, ""
}

// performValidations 执行所有验证
func (td *TriggerDetector) performValidations(
	kline *CandleInfo,
	previousKlines []CandleInfo,
	anchor *AnchorCandidate,
	volumeRatio, strengthZ, atr14 float64,
	triggerTime time.Time,
) *TriggerValidations {
	
	// 实体突破验证
	bodyValidation := td.validateBodyBreakout(kline, anchor, atr14)
	
	// 量能验证
	volumeValidation := td.validateVolume(kline, previousKlines, volumeRatio)
	
	// 强度验证
	strengthValidation := td.validateStrength(strengthZ, anchor, triggerTime)
	
	// 时间验证
	timingValidation := td.validateTiming(kline, anchor, triggerTime)
	
	return &TriggerValidations{
		BodyBreakout:  bodyValidation,
		VolumeCheck:   volumeValidation,
		StrengthCheck: strengthValidation,
		TimingCheck:   timingValidation,
	}
}

// validateBodyBreakout 验证实体突破
func (td *TriggerDetector) validateBodyBreakout(
	kline *CandleInfo,
	anchor *AnchorCandidate,
	atr14 float64,
) BodyBreakoutValidation {
	
	bodySize := math.Abs(kline.Close - kline.Open)
	candleRange := kline.High - kline.Low
	
	var wickSize, breakoutDistance float64
	var hasBodyBreakout bool
	
	if anchor.Dir == "LONG" {
		// 多头突破验证
		breakoutDistance = (kline.Close - anchor.Level) / atr14
		hasBodyBreakout = kline.Close > anchor.Level && kline.Open <= anchor.Level
		
		// 计算上引线大小
		upperWick := kline.High - math.Max(kline.Open, kline.Close)
		wickSize = upperWick
	} else {
		// 空头突破验证
		breakoutDistance = (anchor.Level - kline.Close) / atr14
		hasBodyBreakout = kline.Close < anchor.Level && kline.Open >= anchor.Level
		
		// 计算下引线大小
		lowerWick := math.Min(kline.Open, kline.Close) - kline.Low
		wickSize = lowerWick
	}
	
	// 计算引线比例
	wickRatio := 0.0
	if candleRange > 0 {
		wickRatio = wickSize / candleRange
	}
	
	// 判断是否仅引线突破
	isWickOnly := wickRatio > td.config.MaxWickRatio
	
	return BodyBreakoutValidation{
		HasBodyBreakout:  hasBodyBreakout,
		BreakoutDistance: breakoutDistance,
		WickRatio:        wickRatio,
		BodySize:         bodySize / atr14,
		IsWickOnly:       isWickOnly,
	}
}

// validateVolume 验证量能
func (td *TriggerDetector) validateVolume(
	kline *CandleInfo,
	previousKlines []CandleInfo,
	volumeRatio float64,
) VolumeValidation {
	
	meetsThreshold := volumeRatio >= td.config.MinVolumeRatio
	
	// 计算量能强度评分
	var volumeIntensity float64
	if volumeRatio >= 3.0 {
		volumeIntensity = 1.0 // 极强
	} else if volumeRatio >= 2.0 {
		volumeIntensity = 0.8 // 强
	} else if volumeRatio >= 1.5 {
		volumeIntensity = 0.6 // 中等
	} else if volumeRatio >= 1.2 {
		volumeIntensity = 0.4 // 及格
	} else {
		volumeIntensity = 0.2 // 弱
	}
	
	// 分析量能趋势
	volumeTrend := td.analyzeVolumeTrend(kline, previousKlines)
	
	return VolumeValidation{
		VolumeRatio:     volumeRatio,
		MeetsThreshold:  meetsThreshold,
		VolumeTrend:     volumeTrend,
		VolumeIntensity: volumeIntensity,
	}
}

// validateStrength 验证强度
func (td *TriggerDetector) validateStrength(
	strengthZ float64,
	anchor *AnchorCandidate,
	triggerTime time.Time,
) StrengthValidation {
	
	meetsThreshold := strengthZ > td.config.MinStrengthZ
	
	// 分类强度级别
	var strengthTrend string
	if strengthZ > 2.0 {
		strengthTrend = "STRONG"
	} else if strengthZ > 1.0 {
		strengthTrend = "MEDIUM"
	} else {
		strengthTrend = "WEAK"
	}
	
	// 计算CVD支持强度（基于锚点元数据）
	cvdSupport := 0.5 // 默认中等支持
	if anchor.VolRatio != nil {
		if *anchor.VolRatio > 2.0 {
			cvdSupport = 0.9
		} else if *anchor.VolRatio > 1.5 {
			cvdSupport = 0.7
		}
	}
	
	return StrengthValidation{
		StrengthZ:       strengthZ,
		MeetsThreshold:  meetsThreshold,
		StrengthTrend:   strengthTrend,
		CVDSupport:      cvdSupport,
	}
}

// validateTiming 验证时间
func (td *TriggerDetector) validateTiming(
	kline *CandleInfo,
	anchor *AnchorCandidate,
	triggerTime time.Time,
) TimingValidation {
	
	// 计算触发延迟（理想情况下应该在K线收盘时触发）
	expectedCloseTime := time.Unix(kline.Timestamp/1000, 0).Add(5 * time.Minute) // 转换时间戳并假设5分钟K线
	triggerLatency := triggerTime.Sub(expectedCloseTime).Milliseconds()
	
	isKlineClose := math.Abs(float64(triggerLatency)) <= float64(td.config.MaxTriggerLatencyMs)
	
	// 计算锚点年龄
	anchorAge := triggerTime.Sub(time.Unix(kline.Timestamp/1000, 0)).Minutes() // 简化计算
	
	// 判断是否最佳时机
	isOptimalTiming := isKlineClose && 
		anchorAge <= td.config.MaxAnchorAgeMinutes
	
	return TimingValidation{
		IsKlineClose:    isKlineClose,
		TriggerLatency:  triggerLatency,
		AnchorAge:       anchorAge,
		IsOptimalTiming: isOptimalTiming,
	}
}

// analyzeVolumeTrend 分析量能趋势
func (td *TriggerDetector) analyzeVolumeTrend(
	currentKline *CandleInfo,
	previousKlines []CandleInfo,
) string {
	
	if len(previousKlines) < 3 {
		return "STABLE"
	}
	
	// 计算最近3根K线的平均量能
	var recentAvg float64
	for i := len(previousKlines) - 3; i < len(previousKlines); i++ {
		recentAvg += previousKlines[i].Volume
	}
	recentAvg /= 3
	
	// 与当前量能比较
	currentVolume := currentKline.Volume
	if currentVolume > recentAvg*1.5 {
		return "INCREASING"
	} else if currentVolume < recentAvg*0.7 {
		return "DECREASING"
	} else {
		return "STABLE"
	}
}

// evaluateValidations 评估验证结果
func (td *TriggerDetector) evaluateValidations(validations *TriggerValidations) bool {
	
	// 所有关键验证必须通过
	bodyValid := validations.BodyBreakout.HasBodyBreakout && 
		!validations.BodyBreakout.IsWickOnly &&
		validations.BodyBreakout.BodySize >= td.config.MinBodySizeATR
	
	volumeValid := validations.VolumeCheck.MeetsThreshold
	
	strengthValid := validations.StrengthCheck.MeetsThreshold
	
	timingValid := !td.config.RequireKlineClose || validations.TimingCheck.IsKlineClose
	
	return bodyValid && volumeValid && strengthValid && timingValid
}

// calculateTriggerConfidence 计算触发置信度
func (td *TriggerDetector) calculateTriggerConfidence(
	validations *TriggerValidations,
	anchor *AnchorCandidate,
) float64 {
	
	confidence := 0.0
	
	// 实体突破质量 (0-0.3)
	if validations.BodyBreakout.HasBodyBreakout {
		confidence += 0.15
		if validations.BodyBreakout.BreakoutDistance > 0.2 { // 超过0.2ATR突破
			confidence += 0.1
		}
		if !validations.BodyBreakout.IsWickOnly {
			confidence += 0.05
		}
	}
	
	// 量能质量 (0-0.25)
	confidence += validations.VolumeCheck.VolumeIntensity * 0.25
	
	// 强度质量 (0-0.25)
	strengthScore := math.Min(validations.StrengthCheck.StrengthZ / 3.0, 1.0) // 标准化到0-1
	confidence += strengthScore * 0.25
	
	// 锚点质量 (0-0.2)
	anchorScore := anchor.AnchorScore / 100.0
	confidence += anchorScore * 0.2
	
	// 时间质量加成/惩罚
	if validations.TimingCheck.IsOptimalTiming {
		confidence += 0.1
	} else {
		confidence -= 0.05
	}
	
	// CVD支持加成
	confidence += validations.StrengthCheck.CVDSupport * 0.1
	
	return math.Max(0.1, math.Min(0.95, confidence))
}

// determineTriggerType 确定触发类型
func (td *TriggerDetector) determineTriggerType(validations *TriggerValidations) TriggerType {
	
	// 优先级：实体突破 > 量能突破 > 强度确认
	if validations.BodyBreakout.HasBodyBreakout && !validations.BodyBreakout.IsWickOnly {
		return TriggerBodyBreakout
	} else if validations.VolumeCheck.VolumeIntensity > 0.8 {
		return TriggerVolumeSpike
	} else {
		return TriggerStrengthConfirm
	}
}

// generateTriggerEvidence 生成触发证据
func (td *TriggerDetector) generateTriggerEvidence(
	kline *CandleInfo,
	previousKlines []CandleInfo,
	anchor *AnchorCandidate,
	volumeRatio, strengthZ float64,
) *TriggerEvidence {
	
	// 限制previous candles数量
	var limitedPrevious []CandleInfo
	if len(previousKlines) > 5 {
		limitedPrevious = previousKlines[len(previousKlines)-5:]
	} else {
		limitedPrevious = previousKlines
	}
	
	// 确定市场上下文
	marketContext := "normal"
	if volumeRatio > 3.0 {
		marketContext = "high_volume"
	} else if strengthZ > 2.0 {
		marketContext = "high_strength"
	}
	
	return &TriggerEvidence{
		OpenPrice:       kline.Open,
		ClosePrice:      kline.Close,
		HighPrice:       kline.High,
		LowPrice:        kline.Low,
		AnchorLevel:     anchor.Level,
		Volume:          kline.Volume,
		VolumeRatio:     volumeRatio,
		StrengthZ:       strengthZ,
		PreviousCandles: limitedPrevious,
		MarketContext:   marketContext,
	}
}

// LogTriggerDetails 详细日志输出（调试用）
func LogTriggerDetails(result *TriggerResult) {
	if result == nil || !result.IsTriggered {
		return
	}
	
	v := result.Validations
	log.Printf("🔥 [触发详情] 类型: %s, 方向: %s, 时间: %s", 
		result.TriggerType, result.Direction, result.TriggerTime.Format("15:04:05"))
	
	if v.BodyBreakout.HasBodyBreakout {
		log.Printf("  📊 实体突破: 距离%.3fATR, 实体%.3fATR, 引线比%.2f, 仅引线:%v",
			v.BodyBreakout.BreakoutDistance, v.BodyBreakout.BodySize, v.BodyBreakout.WickRatio, v.BodyBreakout.IsWickOnly)
	}
	
	log.Printf("  📈 量能验证: 比率%.2f, 阈值:%v, 趋势:%s, 强度:%.1f",
		v.VolumeCheck.VolumeRatio, v.VolumeCheck.MeetsThreshold, v.VolumeCheck.VolumeTrend, v.VolumeCheck.VolumeIntensity)
	
	log.Printf("  💪 强度验证: Z分数%.2f, 阈值:%v, 级别:%s, CVD支持:%.2f",
		v.StrengthCheck.StrengthZ, v.StrengthCheck.MeetsThreshold, v.StrengthCheck.StrengthTrend, v.StrengthCheck.CVDSupport)
	
	log.Printf("  ⏰ 时间验证: K线收盘:%v, 延迟%dms, 锚点年龄%.1f分钟, 最佳时机:%v",
		v.TimingCheck.IsKlineClose, v.TimingCheck.TriggerLatency, v.TimingCheck.AnchorAge, v.TimingCheck.IsOptimalTiming)
	
	if result.TriggeredAnchor != nil {
		anchor := result.TriggeredAnchor
		log.Printf("  🎯 触发锚点: %s %s@%.4f (P%d, 评分:%.1f)",
			anchor.Type, anchor.TF, anchor.Level, anchor.PriorityRank, anchor.AnchorScore)
	}
}

// FormatTriggerResult 格式化触发结果
func FormatTriggerResult(result *TriggerResult) string {
	if result == nil {
		return "Trigger: 无数据"
	}
	
	if !result.IsTriggered {
		return "Trigger: 未触发"
	}
	
	return fmt.Sprintf("Trigger: %s %s (置信度: %.2f, 时间: %s)", 
		result.Direction, result.TriggerType, result.Confidence, 
		result.TriggerTime.Format("15:04:05"))
}