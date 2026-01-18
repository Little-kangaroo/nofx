package microstructure

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// ===== 🔥 T12新增：可回放测试框架 =====

// RecordedEvent 录制的事件
type RecordedEvent struct {
	EventType    string          `json:"event_type"`    // "trade", "depth", "oi"
	Symbol       string          `json:"symbol"`
	Timestamp    time.Time       `json:"timestamp"`     // 接收时间
	ExchangeTime time.Time       `json:"exchange_time"` // 交易所时间
	RawData      json.RawMessage `json:"raw_data"`      // 原始事件数据
}

// RecordedSnapshot 录制的快照
type RecordedSnapshot struct {
	Symbol      string           `json:"symbol"`
	EventTime   time.Time        `json:"event_time"`   // 快照事件时间
	Snapshot    *MarketSnapshot  `json:"snapshot"`     // 快照数据
	RecordTime  time.Time        `json:"record_time"`  // 录制时间
}

// DataRecorder 🔥 T12新增：数据录制器
type DataRecorder struct {
	mu              sync.Mutex
	recording       bool
	events          []RecordedEvent
	snapshots       []RecordedSnapshot
	startTime       time.Time
	endTime         time.Time
	maxEvents       int    // 最大事件数限制
	outputDir       string // 输出目录
}

// NewDataRecorder 创建数据录制器
func NewDataRecorder(outputDir string, maxEvents int) *DataRecorder {
	return &DataRecorder{
		events:    make([]RecordedEvent, 0, maxEvents),
		snapshots: make([]RecordedSnapshot, 0, 100),
		maxEvents: maxEvents,
		outputDir: outputDir,
	}
}

// StartRecording 🔥 T12新增：开始录制
func (dr *DataRecorder) StartRecording() {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	dr.recording = true
	dr.startTime = time.Now()
	dr.events = make([]RecordedEvent, 0, dr.maxEvents)
	dr.snapshots = make([]RecordedSnapshot, 0, 100)

	log.Printf("📹 开始录制数据流，最大事件数: %d", dr.maxEvents)
}

// StopRecording 🔥 T12新增：停止录制
func (dr *DataRecorder) StopRecording() {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	dr.recording = false
	dr.endTime = time.Now()

	log.Printf("⏹️  停止录制数据流，共录制 %d 个事件，%d 个快照，耗时 %v",
		len(dr.events), len(dr.snapshots), dr.endTime.Sub(dr.startTime))
}

// RecordEvent 🔥 T12新增：录制事件
func (dr *DataRecorder) RecordEvent(eventType, symbol string, exchangeTime time.Time, rawData interface{}) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if !dr.recording {
		return
	}

	// 检查是否超过最大事件数
	if len(dr.events) >= dr.maxEvents {
		log.Printf("⚠️  录制器已达最大事件数 %d，停止录制新事件", dr.maxEvents)
		dr.recording = false
		return
	}

	// 序列化原始数据
	rawBytes, err := json.Marshal(rawData)
	if err != nil {
		log.Printf("❌ 录制事件失败，序列化错误: %v", err)
		return
	}

	event := RecordedEvent{
		EventType:    eventType,
		Symbol:       symbol,
		Timestamp:    time.Now(),
		ExchangeTime: exchangeTime,
		RawData:      rawBytes,
	}

	dr.events = append(dr.events, event)
}

// RecordSnapshot 🔥 T12新增：录制快照
func (dr *DataRecorder) RecordSnapshot(snapshot *MarketSnapshot) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if !dr.recording {
		return
	}

	recorded := RecordedSnapshot{
		Symbol:     snapshot.Symbol,
		EventTime:  snapshot.Timestamp,
		Snapshot:   snapshot,
		RecordTime: time.Now(),
	}

	dr.snapshots = append(dr.snapshots, recorded)
}

// SaveToFile 🔥 T12新增：保存录制数据到文件
func (dr *DataRecorder) SaveToFile(filename string) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.recording {
		return fmt.Errorf("录制仍在进行中，请先停止录制")
	}

	// 创建输出目录
	if err := os.MkdirAll(dr.outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	filepath := fmt.Sprintf("%s/%s", dr.outputDir, filename)

	// 构建录制数据包
	recordingData := map[string]interface{}{
		"metadata": map[string]interface{}{
			"start_time":    dr.startTime,
			"end_time":      dr.endTime,
			"duration_sec":  dr.endTime.Sub(dr.startTime).Seconds(),
			"event_count":   len(dr.events),
			"snapshot_count": len(dr.snapshots),
		},
		"events":    dr.events,
		"snapshots": dr.snapshots,
	}

	// 序列化为JSON
	jsonData, err := json.MarshalIndent(recordingData, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化录制数据失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	log.Printf("💾 录制数据已保存到: %s (%.2f MB)",
		filepath, float64(len(jsonData))/1024/1024)

	return nil
}

// GetRecordingStats 获取录制统计
func (dr *DataRecorder) GetRecordingStats() map[string]interface{} {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	return map[string]interface{}{
		"recording":      dr.recording,
		"event_count":    len(dr.events),
		"snapshot_count": len(dr.snapshots),
		"start_time":     dr.startTime,
		"duration_sec":   time.Since(dr.startTime).Seconds(),
	}
}

// DataReplayer 🔥 T12新增：数据回放器
type DataReplayer struct {
	events          []RecordedEvent
	snapshots       []RecordedSnapshot
	currentIndex    int
	replaySpeed     float64 // 回放速度倍数（1.0=原速，2.0=2倍速）
	eventCallback   func(RecordedEvent)
}

// NewDataReplayer 创建数据回放器
func NewDataReplayer(filepath string) (*DataReplayer, error) {
	// 读取文件
	jsonData, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("读取录制文件失败: %w", err)
	}

	// 解析JSON
	var recordingData map[string]interface{}
	if err := json.Unmarshal(jsonData, &recordingData); err != nil {
		return nil, fmt.Errorf("解析录制文件失败: %w", err)
	}

	// 提取事件列表
	eventsData, _ := json.Marshal(recordingData["events"])
	var events []RecordedEvent
	if err := json.Unmarshal(eventsData, &events); err != nil {
		return nil, fmt.Errorf("解析事件列表失败: %w", err)
	}

	// 提取快照列表
	snapshotsData, _ := json.Marshal(recordingData["snapshots"])
	var snapshots []RecordedSnapshot
	if err := json.Unmarshal(snapshotsData, &snapshots); err != nil {
		return nil, fmt.Errorf("解析快照列表失败: %w", err)
	}

	log.Printf("📂 加载录制文件成功: %d 个事件, %d 个快照", len(events), len(snapshots))

	return &DataReplayer{
		events:       events,
		snapshots:    snapshots,
		currentIndex: 0,
		replaySpeed:  1.0, // 默认原速回放
	}, nil
}

// SetReplaySpeed 设置回放速度
func (dr *DataReplayer) SetReplaySpeed(speed float64) {
	dr.replaySpeed = speed
}

// SetEventCallback 设置事件回调
func (dr *DataReplayer) SetEventCallback(callback func(RecordedEvent)) {
	dr.eventCallback = callback
}

// Replay 🔥 T12新增：回放录制数据
func (dr *DataReplayer) Replay() error {
	if len(dr.events) == 0 {
		return fmt.Errorf("没有可回放的事件")
	}

	log.Printf("▶️  开始回放数据流 (速度: %.1fx)", dr.replaySpeed)

	startTime := dr.events[0].Timestamp
	replayStartTime := time.Now()

	for i, event := range dr.events {
		dr.currentIndex = i

		// 计算时间延迟（基于录制时的时间间隔）
		if i > 0 {
			recordedDelay := event.Timestamp.Sub(dr.events[i-1].Timestamp)
			actualDelay := time.Duration(float64(recordedDelay) / dr.replaySpeed)

			// 等待到下一个事件的时间
			time.Sleep(actualDelay)
		}

		// 触发事件回调
		if dr.eventCallback != nil {
			dr.eventCallback(event)
		}

		// 定期输出进度
		if i%100 == 0 && i > 0 {
			elapsed := time.Since(replayStartTime)
			progress := float64(i) / float64(len(dr.events)) * 100
			log.Printf("📊 回放进度: %.1f%% (%d/%d), 耗时: %v",
				progress, i, len(dr.events), elapsed)
		}
	}

	totalTime := time.Since(replayStartTime)
	originalDuration := dr.events[len(dr.events)-1].Timestamp.Sub(startTime)

	log.Printf("✅ 回放完成: %d 个事件, 原始时长: %v, 回放时长: %v (%.1fx)",
		len(dr.events), originalDuration, totalTime, dr.replaySpeed)

	return nil
}

// GetSnapshot 根据索引获取快照
func (dr *DataReplayer) GetSnapshot(index int) *RecordedSnapshot {
	if index < 0 || index >= len(dr.snapshots) {
		return nil
	}
	return &dr.snapshots[index]
}

// GetAllSnapshots 获取所有快照
func (dr *DataReplayer) GetAllSnapshots() []RecordedSnapshot {
	return dr.snapshots
}

// SnapshotConsistencyValidator 🔥 T12新增：快照一致性验证器
type SnapshotConsistencyValidator struct {
	originalSnapshots  []RecordedSnapshot
	replayedSnapshots  []RecordedSnapshot
	inconsistencies    []SnapshotInconsistency
}

// SnapshotInconsistency 快照不一致记录
type SnapshotInconsistency struct {
	Symbol       string    `json:"symbol"`
	EventTime    time.Time `json:"event_time"`
	Field        string    `json:"field"`
	OriginalValue interface{} `json:"original_value"`
	ReplayedValue interface{} `json:"replayed_value"`
	Difference   float64   `json:"difference"` // 数值差异
}

// NewSnapshotConsistencyValidator 创建快照一致性验证器
func NewSnapshotConsistencyValidator(original, replayed []RecordedSnapshot) *SnapshotConsistencyValidator {
	return &SnapshotConsistencyValidator{
		originalSnapshots: original,
		replayedSnapshots: replayed,
		inconsistencies:   make([]SnapshotInconsistency, 0),
	}
}

// Validate 🔥 T12新增：验证快照一致性
func (scv *SnapshotConsistencyValidator) Validate() (bool, error) {
	if len(scv.originalSnapshots) != len(scv.replayedSnapshots) {
		return false, fmt.Errorf("快照数量不一致: 原始=%d, 回放=%d",
			len(scv.originalSnapshots), len(scv.replayedSnapshots))
	}

	log.Printf("🔍 开始验证快照一致性，共 %d 个快照", len(scv.originalSnapshots))

	allConsistent := true

	for i := range scv.originalSnapshots {
		original := scv.originalSnapshots[i].Snapshot
		replayed := scv.replayedSnapshots[i].Snapshot

		if original == nil || replayed == nil {
			log.Printf("⚠️  快照 #%d 为 nil，跳过验证", i)
			continue
		}

		// 验证各个字段
		consistent := scv.validateSnapshot(original, replayed)
		if !consistent {
			allConsistent = false
		}
	}

	if allConsistent {
		log.Printf("✅ 快照一致性验证通过: %d 个快照全部一致", len(scv.originalSnapshots))
	} else {
		log.Printf("❌ 快照一致性验证失败: 发现 %d 处不一致", len(scv.inconsistencies))
		scv.printInconsistencies()
	}

	return allConsistent, nil
}

// validateSnapshot 验证单个快照
func (scv *SnapshotConsistencyValidator) validateSnapshot(original, replayed *MarketSnapshot) bool {
	consistent := true

	// 验证CVD数据
	if original.CVDData != nil && replayed.CVDData != nil {
		if !scv.compareFloat(original.CVDData.SpotCVD1H, replayed.CVDData.SpotCVD1H, 0.01) {
			scv.recordInconsistency(original.Symbol, original.Timestamp, "SpotCVD1H",
				original.CVDData.SpotCVD1H, replayed.CVDData.SpotCVD1H)
			consistent = false
		}
		if !scv.compareFloat(original.CVDData.FuturesCVD1H, replayed.CVDData.FuturesCVD1H, 0.01) {
			scv.recordInconsistency(original.Symbol, original.Timestamp, "FuturesCVD1H",
				original.CVDData.FuturesCVD1H, replayed.CVDData.FuturesCVD1H)
			consistent = false
		}
	}

	// 验证OI数据
	if original.OIAnalysis != nil && replayed.OIAnalysis != nil {
		if !scv.compareFloat(original.OIAnalysis.Current, replayed.OIAnalysis.Current, 0.01) {
			scv.recordInconsistency(original.Symbol, original.Timestamp, "OI.Current",
				original.OIAnalysis.Current, replayed.OIAnalysis.Current)
			consistent = false
		}
	}

	// 验证盘口数据
	if original.OrderBookData != nil && replayed.OrderBookData != nil {
		if !scv.compareFloat(original.OrderBookData.ImbalanceRatio, replayed.OrderBookData.ImbalanceRatio, 0.001) {
			scv.recordInconsistency(original.Symbol, original.Timestamp, "OrderBook.ImbalanceRatio",
				original.OrderBookData.ImbalanceRatio, replayed.OrderBookData.ImbalanceRatio)
			consistent = false
		}
	}

	return consistent
}

// compareFloat 比较浮点数（容忍误差）
func (scv *SnapshotConsistencyValidator) compareFloat(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// recordInconsistency 记录不一致
func (scv *SnapshotConsistencyValidator) recordInconsistency(symbol string, eventTime time.Time,
	field string, original, replayed interface{}) {

	diff := 0.0
	if origFloat, ok := original.(float64); ok {
		if replFloat, ok := replayed.(float64); ok {
			diff = origFloat - replFloat
		}
	}

	inconsistency := SnapshotInconsistency{
		Symbol:       symbol,
		EventTime:    eventTime,
		Field:        field,
		OriginalValue: original,
		ReplayedValue: replayed,
		Difference:   diff,
	}

	scv.inconsistencies = append(scv.inconsistencies, inconsistency)
}

// printInconsistencies 打印不一致信息
func (scv *SnapshotConsistencyValidator) printInconsistencies() {
	log.Printf("📋 不一致详情:")
	for i, inc := range scv.inconsistencies {
		log.Printf("  [%d] %s @ %s - 字段: %s, 原始: %v, 回放: %v, 差异: %.6f",
			i+1, inc.Symbol, inc.EventTime.Format("15:04:05"), inc.Field,
			inc.OriginalValue, inc.ReplayedValue, inc.Difference)
	}
}

// GetInconsistencies 获取所有不一致记录
func (scv *SnapshotConsistencyValidator) GetInconsistencies() []SnapshotInconsistency {
	return scv.inconsistencies
}
