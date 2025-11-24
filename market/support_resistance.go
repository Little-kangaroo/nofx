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
	MinTouchCount    int     `json:"min_touch_count"`    // 最小触及次数
	TouchTolerance   float64 `json:"touch_tolerance"`    // 触及容忍度(百分比)
	MinStrength      float64 `json:"min_strength"`       // 最小强度阈值
	ConversionBuffer float64 `json:"conversion_buffer"`  // 转换缓冲区(百分比)
	MaxLevelAge      int     `json:"max_level_age"`      // 最大级别年龄(小时)
	LookbackPeriod   int     `json:"lookback_period"`    // 回看周期
}

// SupportResistanceLevel 支撑阻力级别
type SupportResistanceLevel struct {
	ID               string                 `json:"id"`
	Price            float64                `json:"price"`
	Type             SRLevelType            `json:"type"`
	OriginalType     SRLevelType            `json:"original_type"`
	Strength         float64                `json:"strength"`
	TouchCount       int                    `json:"touch_count"`
	TouchEvents      []*SRTouchEvent        `json:"touch_events"`
	ConversionEvents []*ConversionEvent     `json:"conversion_events"`
	CreationTime     int64                  `json:"creation_time"`
	LastTouch        int64                  `json:"last_touch"`
	IsActive         bool                   `json:"is_active"`
	HasConverted     bool                   `json:"has_converted"`
	ConversionCount  int                    `json:"conversion_count"`
	Confidence       float64                `json:"confidence"`
	Status           SRStatus               `json:"status"`
}

// SRLevelType 支撑阻力级别类型
type SRLevelType string

const (
	SRSupport    SRLevelType = "support"
	SRResistance SRLevelType = "resistance"
	SRNeutral    SRLevelType = "neutral"
)

// SRStatus 支撑阻力状态
type SRStatus string

const (
	SRStatusActive    SRStatus = "active"
	SRStatusConverted SRStatus = "converted"
	SRStatusBroken    SRStatus = "broken"
	SRStatusExpired   SRStatus = "expired"
)

// SRTouchEvent 支撑阻力触及事件
type SRTouchEvent struct {
	Timestamp   int64   `json:"timestamp"`
	Price       float64 `json:"price"`
	TouchType   string  `json:"touch_type"` // "bounce", "break", "test"
	Reaction    float64 `json:"reaction"`   // 反应强度
	Volume      float64 `json:"volume"`
	Confirmed   bool    `json:"confirmed"`
}

// ConversionEvent 转换事件
type ConversionEvent struct {
	Timestamp     int64       `json:"timestamp"`
	FromType      SRLevelType `json:"from_type"`
	ToType        SRLevelType `json:"to_type"`
	BreakPrice    float64     `json:"break_price"`
	BreakStrength float64     `json:"break_strength"`
	Volume        float64     `json:"volume"`
	Confirmed     bool        `json:"confirmed"`
}

// SupportResistanceData 支撑阻力分析结果
type SupportResistanceData struct {
	Levels       []*SupportResistanceLevel `json:"levels"`
	ActiveLevels []*SupportResistanceLevel `json:"active_levels"`
	Statistics   *SRStatistics             `json:"statistics"`
	Config       *SRConfig                 `json:"config"`
	LastAnalysis int64                     `json:"last_analysis"`
}

// SRStatistics 支撑阻力统计
type SRStatistics struct {
	TotalLevels        int     `json:"total_levels"`
	ActiveSupport      int     `json:"active_support"`
	ActiveResistance   int     `json:"active_resistance"`
	ConversionRate     float64 `json:"conversion_rate"`
	AvgStrength        float64 `json:"avg_strength"`
	SuccessRate        float64 `json:"success_rate"`
	MostReliableLevel  float64 `json:"most_reliable_level"`
}

// 默认配置
var defaultSRConfig = SRConfig{
	MinTouchCount:    2,
	TouchTolerance:   0.002, // 0.2%
	MinStrength:      0.3,
	ConversionBuffer: 0.005, // 0.5%
	MaxLevelAge:      168,   // 7天
	LookbackPeriod:   200,   // 200根K线
}

// NewSupportResistanceAnalyzer 创建支撑阻力分析器
func NewSupportResistanceAnalyzer() *SupportResistanceAnalyzer {
	return &SupportResistanceAnalyzer{
		config: defaultSRConfig,
	}
}

// Analyze 分析支撑阻力转换线
func (sra *SupportResistanceAnalyzer) Analyze(klines []Kline) *SupportResistanceData {
	if len(klines) < sra.config.LookbackPeriod {
		return &SupportResistanceData{
			Levels:       []*SupportResistanceLevel{},
			ActiveLevels: []*SupportResistanceLevel{},
			Statistics:   &SRStatistics{},
			Config:       &sra.config,
			LastAnalysis: time.Now().UnixMilli(),
		}
	}

	// 识别潜在的支撑阻力级别
	levels := sra.identifyLevels(klines)

	// 分析历史触及和转换
	sra.analyzeTouchEvents(levels, klines)
	sra.analyzeConversions(levels, klines)

	// 更新级别状态和强度
	sra.updateLevelStatuses(levels, klines)

	// 筛选活跃级别
	activeLevels := sra.filterActiveLevels(levels)

	// 计算统计数据
	statistics := sra.calculateStatistics(levels, activeLevels)

	return &SupportResistanceData{
		Levels:       levels,
		ActiveLevels: activeLevels,
		Statistics:   statistics,
		Config:       &sra.config,
		LastAnalysis: time.Now().UnixMilli(),
	}
}

// identifyLevels 识别支撑阻力级别
func (sra *SupportResistanceAnalyzer) identifyLevels(klines []Kline) []*SupportResistanceLevel {
	var levels []*SupportResistanceLevel
	levelMap := make(map[float64]*SupportResistanceLevel)

	// 寻找显著的高低点
	for i := 5; i < len(klines)-5; i++ {
		// 检查是否为局部高点（阻力位候选）
		if sra.isLocalHigh(klines, i) {
			price := klines[i].High
			level := sra.getOrCreateLevel(levelMap, price, SRResistance, klines[i].OpenTime)
			if level != nil {
				levels = append(levels, level)
			}
		}

		// 检查是否为局部低点（支撑位候选）
		if sra.isLocalLow(klines, i) {
			price := klines[i].Low
			level := sra.getOrCreateLevel(levelMap, price, SRSupport, klines[i].OpenTime)
			if level != nil {
				levels = append(levels, level)
			}
		}
	}

	// 识别重要的价格聚集区
	clusterLevels := sra.identifyPriceClusters(klines)
	for _, level := range clusterLevels {
		if !sra.isLevelExisting(levelMap, level.Price) {
			levels = append(levels, level)
		}
	}

	return levels
}

// isLocalHigh 检查是否为局部高点
func (sra *SupportResistanceAnalyzer) isLocalHigh(klines []Kline, index int) bool {
	current := klines[index].High
	
	// 检查左侧
	for i := index - 3; i < index; i++ {
		if klines[i].High >= current {
			return false
		}
	}
	
	// 检查右侧
	for i := index + 1; i <= index + 3; i++ {
		if klines[i].High >= current {
			return false
		}
	}
	
	return true
}

// isLocalLow 检查是否为局部低点
func (sra *SupportResistanceAnalyzer) isLocalLow(klines []Kline, index int) bool {
	current := klines[index].Low
	
	// 检查左侧
	for i := index - 3; i < index; i++ {
		if klines[i].Low <= current {
			return false
		}
	}
	
	// 检查右侧
	for i := index + 1; i <= index + 3; i++ {
		if klines[i].Low <= current {
			return false
		}
	}
	
	return true
}

// getOrCreateLevel 获取或创建级别
func (sra *SupportResistanceAnalyzer) getOrCreateLevel(levelMap map[float64]*SupportResistanceLevel, price float64, levelType SRLevelType, timestamp int64) *SupportResistanceLevel {
	// 检查是否已存在相近的级别
	tolerance := price * sra.config.TouchTolerance
	for existingPrice, level := range levelMap {
		if math.Abs(price-existingPrice) <= tolerance {
			return level // 返回现有级别，不创建新的
		}
	}

	// 创建新级别
	level := &SupportResistanceLevel{
		ID:              fmt.Sprintf("sr_%s_%.2f_%d", levelType, price, timestamp),
		Price:           price,
		Type:            levelType,
		OriginalType:    levelType,
		TouchCount:      1,
		TouchEvents:     []*SRTouchEvent{},
		ConversionEvents: []*ConversionEvent{},
		CreationTime:    timestamp,
		LastTouch:       timestamp,
		IsActive:        true,
		HasConverted:    false,
		ConversionCount: 0,
		Confidence:      0.5,
		Status:          SRStatusActive,
	}

	levelMap[price] = level
	return level
}

// isLevelExisting 检查级别是否已存在
func (sra *SupportResistanceAnalyzer) isLevelExisting(levelMap map[float64]*SupportResistanceLevel, price float64) bool {
	tolerance := price * sra.config.TouchTolerance
	for existingPrice := range levelMap {
		if math.Abs(price-existingPrice) <= tolerance {
			return true
		}
	}
	return false
}

// identifyPriceClusters 识别价格聚集区
func (sra *SupportResistanceAnalyzer) identifyPriceClusters(klines []Kline) []*SupportResistanceLevel {
	var levels []*SupportResistanceLevel
	
	// 收集所有高低点
	var prices []float64
	for i := 0; i < len(klines); i++ {
		prices = append(prices, klines[i].High, klines[i].Low)
	}
	
	sort.Float64s(prices)
	
	// 寻找价格聚集区
	clusterSize := 5
	for i := 0; i < len(prices)-clusterSize; i++ {
		clusterPrices := prices[i : i+clusterSize]
		priceRange := clusterPrices[clusterSize-1] - clusterPrices[0]
		avgPrice := (clusterPrices[0] + clusterPrices[clusterSize-1]) / 2
		
		// 如果价格聚集度高（变化范围小）
		if priceRange/avgPrice < 0.01 { // 1%范围内
			level := &SupportResistanceLevel{
				ID:               fmt.Sprintf("sr_cluster_%.2f", avgPrice),
				Price:            avgPrice,
				Type:             SRNeutral,
				OriginalType:     SRNeutral,
				TouchCount:       clusterSize,
				TouchEvents:      []*SRTouchEvent{},
				ConversionEvents: []*ConversionEvent{},
				CreationTime:     time.Now().UnixMilli(),
				LastTouch:        time.Now().UnixMilli(),
				IsActive:         true,
				HasConverted:     false,
				ConversionCount:  0,
				Confidence:       0.7,
				Status:           SRStatusActive,
			}
			levels = append(levels, level)
		}
	}
	
	return levels
}

// analyzeTouchEvents 分析触及事件
func (sra *SupportResistanceAnalyzer) analyzeTouchEvents(levels []*SupportResistanceLevel, klines []Kline) {
	for _, level := range levels {
		touchCount := 0
		
		for i, kline := range klines {
			if sra.isPriceNearLevel(kline.High, level.Price) || sra.isPriceNearLevel(kline.Low, level.Price) {
				touchCount++
				
				// 创建触及事件
				touchEvent := &SRTouchEvent{
					Timestamp: kline.OpenTime,
					Price:     level.Price,
					Volume:    kline.Volume,
					Confirmed: true,
				}
				
				// 分析反应
				reaction := sra.analyzeReaction(klines, i, level)
				touchEvent.Reaction = reaction
				
				// 确定触及类型
				if reaction > 0.01 {
					touchEvent.TouchType = "bounce"
				} else if reaction < -0.005 {
					touchEvent.TouchType = "break"
				} else {
					touchEvent.TouchType = "test"
				}
				
				level.TouchEvents = append(level.TouchEvents, touchEvent)
				level.LastTouch = kline.OpenTime
			}
		}
		
		level.TouchCount = touchCount
	}
}

// isPriceNearLevel 检查价格是否接近级别
func (sra *SupportResistanceAnalyzer) isPriceNearLevel(price, levelPrice float64) bool {
	tolerance := levelPrice * sra.config.TouchTolerance
	return math.Abs(price-levelPrice) <= tolerance
}

// analyzeReaction 分析价格反应
func (sra *SupportResistanceAnalyzer) analyzeReaction(klines []Kline, touchIndex int, level *SupportResistanceLevel) float64 {
	if touchIndex >= len(klines)-3 {
		return 0
	}
	
	touchPrice := klines[touchIndex].Close
	
	// 检查后续3根K线的反应
	reactionPeriod := 3
	var maxReaction float64
	
	for i := 1; i <= reactionPeriod && touchIndex+i < len(klines); i++ {
		futurePrice := klines[touchIndex+i].Close
		reaction := (futurePrice - touchPrice) / touchPrice
		
		if level.Type == SRSupport {
			// 支撑位期望向��反应
			if reaction > maxReaction {
				maxReaction = reaction
			}
		} else if level.Type == SRResistance {
			// 阻力位期望向下反应
			if -reaction > maxReaction {
				maxReaction = -reaction
			}
		}
	}
	
	return maxReaction
}

// analyzeConversions 分析转换事件
func (sra *SupportResistanceAnalyzer) analyzeConversions(levels []*SupportResistanceLevel, klines []Kline) {
	for _, level := range levels {
		sra.detectConversion(level, klines)
	}
}

// detectConversion 检测转换
func (sra *SupportResistanceAnalyzer) detectConversion(level *SupportResistanceLevel, klines []Kline) {
	if len(level.TouchEvents) < 2 {
		return
	}
	
	currentPrice := klines[len(klines)-1].Close
	conversionBuffer := level.Price * sra.config.ConversionBuffer
	
	// 检查支撑转阻力
	if level.Type == SRSupport && currentPrice > level.Price+conversionBuffer {
		// 验证转换：价格突破后在级别上方停留
		if sra.confirmConversion(level, klines, true) {
			sra.recordConversion(level, SRSupport, SRResistance, klines[len(klines)-1])
		}
	}
	
	// 检查阻力转支撑
	if level.Type == SRResistance && currentPrice < level.Price-conversionBuffer {
		// 验证转换：价格突破后在级别下方停留
		if sra.confirmConversion(level, klines, false) {
			sra.recordConversion(level, SRResistance, SRSupport, klines[len(klines)-1])
		}
	}
}

// confirmConversion 确认转换
func (sra *SupportResistanceAnalyzer) confirmConversion(level *SupportResistanceLevel, klines []Kline, isUpwardBreak bool) bool {
	confirmationPeriod := 5
	requiredConfirmation := 3
	
	if len(klines) < confirmationPeriod {
		return false
	}
	
	confirmationCount := 0
	recentKlines := klines[len(klines)-confirmationPeriod:]
	
	for _, kline := range recentKlines {
		if isUpwardBreak {
			// 向上突破：要求价格保持在级别之上
			if kline.Close > level.Price {
				confirmationCount++
			}
		} else {
			// 向下突破：要求价格保持在级别之下
			if kline.Close < level.Price {
				confirmationCount++
			}
		}
	}
	
	return confirmationCount >= requiredConfirmation
}

// recordConversion 记录转换事件
func (sra *SupportResistanceAnalyzer) recordConversion(level *SupportResistanceLevel, fromType, toType SRLevelType, breakKline Kline) {
	conversion := &ConversionEvent{
		Timestamp:     breakKline.OpenTime,
		FromType:      fromType,
		ToType:        toType,
		BreakPrice:    breakKline.Close,
		BreakStrength: math.Abs(breakKline.Close-level.Price) / level.Price,
		Volume:        breakKline.Volume,
		Confirmed:     true,
	}
	
	level.ConversionEvents = append(level.ConversionEvents, conversion)
	level.Type = toType
	level.HasConverted = true
	level.ConversionCount++
	level.Status = SRStatusConverted
}

// updateLevelStatuses 更新级别状态
func (sra *SupportResistanceAnalyzer) updateLevelStatuses(levels []*SupportResistanceLevel, klines []Kline) {
	currentTime := klines[len(klines)-1].OpenTime
	
	for _, level := range levels {
		// 更新强度
		sra.calculateLevelStrength(level)
		
		// 更新置信度
		sra.calculateLevelConfidence(level)
		
		// 检查年龄
		age := int((currentTime - level.CreationTime) / (3600 * 1000)) // 转换为小时
		if age > sra.config.MaxLevelAge {
			level.Status = SRStatusExpired
			level.IsActive = false
		}
		
		// 检查是否被完全突破
		if sra.isLevelBroken(level, klines) {
			level.Status = SRStatusBroken
			level.IsActive = false
		}
	}
}

// calculateLevelStrength 计算级别强度
func (sra *SupportResistanceAnalyzer) calculateLevelStrength(level *SupportResistanceLevel) {
	strength := 0.0
	
	// 基于触及次数
	strength += math.Min(float64(level.TouchCount)*0.1, 0.5)
	
	// 基于成功反应次数
	bounceCount := 0
	for _, event := range level.TouchEvents {
		if event.TouchType == "bounce" && event.Reaction > 0.005 {
			bounceCount++
		}
	}
	strength += math.Min(float64(bounceCount)*0.15, 0.3)
	
	// 基于转换历史
	if level.HasConverted {
		strength += 0.1
	}
	
	// 基于年龄（越老越可靠）
	ageBonus := math.Min(float64(len(level.TouchEvents))/10.0*0.1, 0.1)
	strength += ageBonus
	
	level.Strength = math.Min(strength, 1.0)
}

// calculateLevelConfidence 计算级别置信度
func (sra *SupportResistanceAnalyzer) calculateLevelConfidence(level *SupportResistanceLevel) {
	confidence := level.Strength
	
	// 基于触及事件质量
	if len(level.TouchEvents) > 0 {
		totalReaction := 0.0
		for _, event := range level.TouchEvents {
			totalReaction += event.Reaction
		}
		avgReaction := totalReaction / float64(len(level.TouchEvents))
		confidence += avgReaction * 100
	}
	
	// 转换历史加成
	if level.HasConverted {
		confidence += 0.1
	}
	
	level.Confidence = math.Min(confidence, 1.0)
}

// isLevelBroken 检查级别是否被突破
func (sra *SupportResistanceAnalyzer) isLevelBroken(level *SupportResistanceLevel, klines []Kline) bool {
	if len(klines) < 5 {
		return false
	}
	
	recentKlines := klines[len(klines)-5:]
	breakCount := 0
	
	for _, kline := range recentKlines {
		buffer := level.Price * sra.config.ConversionBuffer
		
		if level.Type == SRSupport && kline.Close < level.Price-buffer {
			breakCount++
		} else if level.Type == SRResistance && kline.Close > level.Price+buffer {
			breakCount++
		}
	}
	
	return breakCount >= 4 // 5根K线中有4根突破
}

// filterActiveLevels 筛选活跃级别
func (sra *SupportResistanceAnalyzer) filterActiveLevels(levels []*SupportResistanceLevel) []*SupportResistanceLevel {
	var activeLevels []*SupportResistanceLevel
	
	for _, level := range levels {
		if level.IsActive && 
		   level.TouchCount >= sra.config.MinTouchCount && 
		   level.Strength >= sra.config.MinStrength {
			activeLevels = append(activeLevels, level)
		}
	}
	
	// 按强度排序
	sort.Slice(activeLevels, func(i, j int) bool {
		return activeLevels[i].Strength > activeLevels[j].Strength
	})
	
	return activeLevels
}

// calculateStatistics 计算统计数据
func (sra *SupportResistanceAnalyzer) calculateStatistics(levels, activeLevels []*SupportResistanceLevel) *SRStatistics {
	stats := &SRStatistics{
		TotalLevels: len(levels),
	}
	
	if len(levels) == 0 {
		return stats
	}
	
	// 计算活跃级别统计
	totalStrength := 0.0
	conversionCount := 0
	successCount := 0
	var mostReliable *SupportResistanceLevel
	
	for _, level := range activeLevels {
		totalStrength += level.Strength
		
		if level.Type == SRSupport {
			stats.ActiveSupport++
		} else if level.Type == SRResistance {
			stats.ActiveResistance++
		}
		
		if level.HasConverted {
			conversionCount++
		}
		
		// 计算成功率（反弹次数/总触及次数）
		bounceCount := 0
		for _, event := range level.TouchEvents {
			if event.TouchType == "bounce" {
				bounceCount++
			}
		}
		if bounceCount > 0 {
			successCount++
		}
		
		// 找到最可靠的级别
		if mostReliable == nil || level.Confidence > mostReliable.Confidence {
			mostReliable = level
		}
	}
	
	if len(activeLevels) > 0 {
		stats.AvgStrength = totalStrength / float64(len(activeLevels))
		stats.ConversionRate = float64(conversionCount) / float64(len(activeLevels)) * 100
		stats.SuccessRate = float64(successCount) / float64(len(activeLevels)) * 100
	}
	
	if mostReliable != nil {
		stats.MostReliableLevel = mostReliable.Price
	}
	
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