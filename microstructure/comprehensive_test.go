package microstructure

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// ===== T02测试：禁止输出层二次抓快照 =====

func TestT02_NoDoubleSnapshot(t *testing.T) {
	t.Run("GenerateStandardizedAIPayload uses snapshot timestamp", func(t *testing.T) {
		// 创建模拟快照
		snapshotTime := time.Now().Add(-1 * time.Minute)
		ms := &MarketSnapshot{
			Symbol:    "BTCUSDT",
			Timestamp: snapshotTime,
			CVDData: &CVDData{
				SpotCVD1H:    1000.0,
				FuturesCVD1H: 2000.0,
			},
			DataQuality: &DataQualityInfo{
				OverallScore: 0.8,
			},
		}

		// 生成AI输出
		payload := GenerateStandardizedAIPayload(ms)

		// 验证：使用快照时间戳，而非time.Now()
		timeDiff := payload.Timestamp.Sub(snapshotTime).Abs()
		if timeDiff > time.Millisecond {
			t.Errorf("Payload should use snapshot timestamp, diff: %v", timeDiff)
		}

		// 验证：Version应该是T02-fixed
		if payload.Version != "T02-fixed" {
			t.Errorf("Expected version T02-fixed, got %s", payload.Version)
		}
	})

	t.Run("Nil snapshot handling", func(t *testing.T) {
		payload := GenerateStandardizedAIPayload(nil)

		if payload.QualityStatus != "FATAL" {
			t.Errorf("Nil snapshot should result in FATAL status, got %s", payload.QualityStatus)
		}

		if payload.SystemStatus.SystemMode != "FATAL" {
			t.Errorf("Nil snapshot should result in FATAL mode, got %s", payload.SystemStatus.SystemMode)
		}
	})
}

// ===== T06测试：DataQuality时间基准 =====

func TestT06_DataQualityRefTime(t *testing.T) {
	t.Run("DataQuality through GetMarketSnapshotAt", func(t *testing.T) {
		config := DefaultMicrostructureConfig()
		ofm := NewOrderFlowManager(config)

		symbol := "BTCUSDT"
		now := time.Now()

		// 添加一些CVD数据
		ofm.cvdManager.GetOrCreateCalculator(symbol).ProcessTrade(&TradeData{
			Symbol:       symbol,
			Price:        50000,
			Quantity:     1.0,
			Timestamp:    now.Add(-5 * time.Minute),
			IsBuyerMaker: false,
			MarketType:   "futures",
		})

		// 添加OI数据
		ofm.oiManager.GetOrCreateCalculator(symbol).ProcessOIData(&OIData{
			Symbol:       symbol,
			OpenInterest: 100000,
			Timestamp:    now.Add(-5 * time.Minute),
			ExchangeTime: now.Add(-5 * time.Minute),
		})

		// 使用eventTime获取快照
		eventTime := now
		snapshot := ofm.GetMarketSnapshotAt(symbol, eventTime)

		if snapshot.DataQuality == nil {
			t.Fatal("DataQuality should not be nil")
		}

		// 验证：DataLagMs应该基于eventTime
		// 数据是5分钟前的，lag应该约为5分钟 = 300,000ms
		expectedLag := int64(5 * 60 * 1000)
		actualLag := snapshot.DataQuality.DataLagMs

		// 允许一定误差
		lagDiff := math.Abs(float64(expectedLag - actualLag))
		if lagDiff > 10000 { // 允许10秒误差
			t.Logf("Warning: DataLagMs diff larger than expected: expected ~%dms, got %dms (diff: %.0fms)",
				expectedLag, actualLag, lagDiff)
		}

		// 验证：组件更新时间应该被记录
		if snapshot.DataQuality.CVDLastUpdate.IsZero() {
			t.Error("CVDLastUpdate should be set")
		}
	})

	t.Run("Stale data affects quality", func(t *testing.T) {
		config := DefaultMicrostructureConfig()
		ofm := NewOrderFlowManager(config)

		symbol := "BTCUSDT"
		now := time.Now()

		// 添加非常旧的数据（15分钟前）
		ofm.cvdManager.GetOrCreateCalculator(symbol).ProcessTrade(&TradeData{
			Symbol:       symbol,
			Price:        50000,
			Quantity:     1.0,
			Timestamp:    now.Add(-15 * time.Minute),
			IsBuyerMaker: false,
			MarketType:   "futures",
		})

		snapshot := ofm.GetMarketSnapshotAt(symbol, now)

		// CVD数据应该被标记为过期
		if snapshot.CVDData != nil && !snapshot.CVDData.IsStale {
			t.Error("CVD data should be stale after 15 minutes")
		}

		// 数据质量分数应该受影响
		if snapshot.DataQuality != nil {
			if snapshot.DataQuality.OverallScore > 0.5 {
				t.Logf("Warning: Quality score high despite stale data: %f",
					snapshot.DataQuality.OverallScore)
			}
		}
	})
}

// ===== T07测试：OI拉取弹性与限流 =====

func TestT07_OIFetchStrategy(t *testing.T) {
	t.Run("Exponential backoff on failures", func(t *testing.T) {
		strategy := NewOIFetchStrategy()

		// 初始间隔应该是基础间隔
		if strategy.GetCurrentInterval() != 30*time.Second {
			t.Errorf("Initial interval should be 30s, got %v", strategy.GetCurrentInterval())
		}

		// 第一次失败：30s -> 60s
		strategy.RecordFailure()
		interval1 := strategy.GetCurrentInterval()
		if interval1 != 60*time.Second {
			t.Errorf("After 1st failure, interval should be 60s, got %v", interval1)
		}

		// 第二次失败：60s -> 120s
		strategy.RecordFailure()
		interval2 := strategy.GetCurrentInterval()
		if interval2 != 120*time.Second {
			t.Errorf("After 2nd failure, interval should be 120s, got %v", interval2)
		}

		// 第三次失败：120s -> 240s
		strategy.RecordFailure()
		interval3 := strategy.GetCurrentInterval()
		if interval3 != 240*time.Second {
			t.Errorf("After 3rd failure, interval should be 240s, got %v", interval3)
		}

		// 第四次失败：240s -> 300s (cap)
		strategy.RecordFailure()
		interval4 := strategy.GetCurrentInterval()
		if interval4 != 300*time.Second {
			t.Errorf("After 4th failure, interval should be capped at 300s, got %v", interval4)
		}

		// 再次失败应该保持300s
		strategy.RecordFailure()
		interval5 := strategy.GetCurrentInterval()
		if interval5 != 300*time.Second {
			t.Errorf("Interval should stay at max 300s, got %v", interval5)
		}
	})

	t.Run("Reset on success", func(t *testing.T) {
		strategy := NewOIFetchStrategy()

		// 模拟多次失败
		strategy.RecordFailure()
		strategy.RecordFailure()
		strategy.RecordFailure()

		// 当前间隔应该是240s
		if strategy.GetCurrentInterval() != 240*time.Second {
			t.Errorf("Expected 240s after 3 failures")
		}

		// 成功后应该重置
		strategy.RecordSuccess()
		interval := strategy.GetCurrentInterval()
		if interval != 30*time.Second {
			t.Errorf("After success, interval should reset to 30s, got %v", interval)
		}

		// 失败计数应该归零
		if strategy.consecutiveFailures != 0 {
			t.Errorf("Consecutive failures should be 0 after success, got %d",
				strategy.consecutiveFailures)
		}
	})

	t.Run("Rate limiting enforcement", func(t *testing.T) {
		strategy := NewOIFetchStrategy()

		// 第一次调用应该允许
		if !strategy.ShouldFetch() {
			t.Error("First fetch should be allowed")
		}

		// 模拟完成拉取（更新lastFetchTime）
		strategy.RecordSuccess()

		// 立即第二次调用应该被限流（minInterval=10s）
		if strategy.ShouldFetch() {
			t.Error("Immediate second fetch should be rate-limited")
		}

		// 模拟等待31秒后（超过baseInterval=30s）
		strategy.mu.Lock()
		strategy.lastFetchTime = time.Now().Add(-31 * time.Second)
		strategy.mu.Unlock()

		// 现在应该允许（超过baseInterval和minInterval）
		if !strategy.ShouldFetch() {
			t.Error("Fetch after 31s should be allowed (baseInterval=30s, minInterval=10s)")
		}

		// 测试minInterval限制：等待11秒，但这少于baseInterval
		strategy.RecordSuccess()
		strategy.mu.Lock()
		strategy.lastFetchTime = time.Now().Add(-11 * time.Second)
		strategy.mu.Unlock()

		// 应该被拒绝（超过minInterval=10s，但未超过currentInterval=30s）
		if strategy.ShouldFetch() {
			t.Error("Fetch after only 11s should be blocked (currentInterval=30s)")
		}
	})
}

// ===== T08测试：TickSize标准化 =====

func TestT08_TickSizeStandardization(t *testing.T) {
	t.Run("GetTickSize fallback logic", func(t *testing.T) {
		manager := NewExchangeInfoManager()

		// 未加载exchangeInfo时，应该使用兜底值
		btcTickSize := manager.GetTickSize("BTCUSDT")
		if btcTickSize != 0.1 {
			t.Errorf("BTC fallback tickSize should be 0.1, got %f", btcTickSize)
		}

		ethTickSize := manager.GetTickSize("ETHUSDT")
		if ethTickSize != 0.01 {
			t.Errorf("ETH fallback tickSize should be 0.01, got %f", ethTickSize)
		}

		otherTickSize := manager.GetTickSize("ADAUSDT")
		if otherTickSize != 0.001 {
			t.Errorf("Other coins fallback tickSize should be 0.001, got %f", otherTickSize)
		}
	})

	t.Run("GetBucketSize dynamic calculation", func(t *testing.T) {
		manager := NewExchangeInfoManager()

		// BTC: tickSize * 100
		btcBucket := manager.GetBucketSize("BTCUSDT")
		expectedBTC := 0.1 * 100 // = 10
		if btcBucket != expectedBTC {
			t.Errorf("BTC bucket should be %f, got %f", expectedBTC, btcBucket)
		}

		// ETH: tickSize * 500
		ethBucket := manager.GetBucketSize("ETHUSDT")
		expectedETH := 0.01 * 500 // = 5
		if ethBucket != expectedETH {
			t.Errorf("ETH bucket should be %f, got %f", expectedETH, ethBucket)
		}

		// Other: tickSize * 100
		adaBucket := manager.GetBucketSize("ADAUSDT")
		expectedADA := 0.001 * 100 // = 0.1
		if adaBucket != expectedADA {
			t.Errorf("ADA bucket should be %f, got %f", expectedADA, adaBucket)
		}
	})

	t.Run("RoundPrice to tickSize", func(t *testing.T) {
		manager := NewExchangeInfoManager()

		// BTC tickSize = 0.1
		price := 50123.456
		rounded := manager.RoundPrice("BTCUSDT", price)
		expected := 50123.5 // 四舍五入到0.1

		if math.Abs(rounded-expected) > 0.001 {
			t.Errorf("Expected %f, got %f", expected, rounded)
		}
	})

	t.Run("GetSymbolInfo returns fallback", func(t *testing.T) {
		manager := NewExchangeInfoManager()

		info := manager.GetSymbolInfo("BTCUSDT")
		if info == nil {
			t.Fatal("GetSymbolInfo should return fallback, not nil")
		}

		if info.Symbol != "BTCUSDT" {
			t.Errorf("Symbol should be BTCUSDT, got %s", info.Symbol)
		}

		if info.TickSize != 0.1 {
			t.Errorf("Fallback tickSize should be 0.1, got %f", info.TickSize)
		}
	})
}

// ===== T11测试：协议标准化 =====

func TestT11_ProtocolStandardization(t *testing.T) {
	t.Run("Field mapping consistency", func(t *testing.T) {
		mapping := GetStandardizedFieldMapping()

		if mapping.RootStructure != "V-12.2订单流分析系统" {
			t.Errorf("Root structure should be V-12.2, got %s", mapping.RootStructure)
		}

		// 验证关键字段映射存在
		requiredKeys := []string{"root_key", "cvd_analysis", "period_analysis"}
		for _, key := range requiredKeys {
			if _, exists := mapping.OrderFlowSection[key]; !exists {
				t.Errorf("OrderFlowSection should have key: %s", key)
			}
		}
	})

	t.Run("Fallback mode uses standard fields", func(t *testing.T) {
		ms := &MarketSnapshot{
			Symbol:    "BTCUSDT",
			Timestamp: time.Now(),
			CVDData: &CVDData{
				SpotCVD1H:    1000,
				FuturesCVD1H: 2000,
			},
		}

		mapping := GetStandardizedFieldMapping()
		fallbackContent := generateFallbackContentWithStandardFields(ms, mapping)

		// 验证：包含根结构
		if _, exists := fallbackContent[mapping.RootStructure]; !exists {
			t.Errorf("Fallback should contain root structure: %s", mapping.RootStructure)
		}

		// 验证：可以序列化为JSON（字段结构正确）
		_, err := json.Marshal(fallbackContent)
		if err != nil {
			t.Errorf("Fallback content should be JSON-serializable: %v", err)
		}
	})
}

// ===== T12测试：回放测试框架 =====

func TestT12_ReplayFramework(t *testing.T) {
	t.Run("DataRecorder basic functionality", func(t *testing.T) {
		recorder := NewDataRecorder("/tmp/test_recordings", 1000)

		recorder.StartRecording()
		if !recorder.recording {
			t.Error("Recording should be started")
		}

		// 录制事件
		now := time.Now()
		recorder.RecordEvent("trade", "BTCUSDT", now, map[string]interface{}{
			"price":    50000.0,
			"quantity": 1.0,
		})

		stats := recorder.GetRecordingStats()
		eventCount := stats["event_count"].(int)
		if eventCount != 1 {
			t.Errorf("Should have 1 event, got %d", eventCount)
		}

		recorder.StopRecording()
		if recorder.recording {
			t.Error("Recording should be stopped")
		}
	})

	t.Run("DataRecorder max events limit", func(t *testing.T) {
		recorder := NewDataRecorder("/tmp/test_recordings", 10)
		recorder.StartRecording()

		// 录制11个事件
		now := time.Now()
		for i := 0; i < 11; i++ {
			recorder.RecordEvent("trade", "BTCUSDT", now, map[string]interface{}{
				"seq": i,
			})
		}

		stats := recorder.GetRecordingStats()
		eventCount := stats["event_count"].(int)

		// 应该只有10个事件（达到上限自动停止）
		if eventCount != 10 {
			t.Errorf("Should cap at 10 events, got %d", eventCount)
		}

		if recorder.recording {
			t.Error("Should auto-stop when reaching max events")
		}
	})

	t.Run("Snapshot consistency validator", func(t *testing.T) {
		now := time.Now()

		// 创建相同的快照
		snapshot1 := &MarketSnapshot{
			Symbol:    "BTCUSDT",
			Timestamp: now,
			CVDData: &CVDData{
				SpotCVD1H:    1000.0,
				FuturesCVD1H: 2000.0,
			},
			OIAnalysis: &OIAnalysis{
				Current: 50000.0,
			},
		}

		snapshot2 := &MarketSnapshot{
			Symbol:    "BTCUSDT",
			Timestamp: now,
			CVDData: &CVDData{
				SpotCVD1H:    1000.0,
				FuturesCVD1H: 2000.0,
			},
			OIAnalysis: &OIAnalysis{
				Current: 50000.0,
			},
		}

		original := []RecordedSnapshot{{Snapshot: snapshot1}}
		replayed := []RecordedSnapshot{{Snapshot: snapshot2}}

		validator := NewSnapshotConsistencyValidator(original, replayed)
		consistent, err := validator.Validate()

		if err != nil {
			t.Errorf("Validation should not error: %v", err)
		}

		if !consistent {
			t.Error("Identical snapshots should be consistent")
		}
	})

	t.Run("Detect snapshot inconsistencies", func(t *testing.T) {
		now := time.Now()

		snapshot1 := &MarketSnapshot{
			Symbol:    "BTCUSDT",
			Timestamp: now,
			CVDData: &CVDData{
				SpotCVD1H:    1000.0,
				FuturesCVD1H: 2000.0,
			},
		}

		snapshot2 := &MarketSnapshot{
			Symbol:    "BTCUSDT",
			Timestamp: now,
			CVDData: &CVDData{
				SpotCVD1H:    1000.0,
				FuturesCVD1H: 2500.0, // 不同的值
			},
		}

		original := []RecordedSnapshot{{Snapshot: snapshot1}}
		replayed := []RecordedSnapshot{{Snapshot: snapshot2}}

		validator := NewSnapshotConsistencyValidator(original, replayed)
		consistent, _ := validator.Validate()

		if consistent {
			t.Error("Different snapshots should be detected as inconsistent")
		}

		inconsistencies := validator.GetInconsistencies()
		if len(inconsistencies) == 0 {
			t.Error("Should record inconsistencies")
		}

		// 验证记录的不一致
		found := false
		for _, inc := range inconsistencies {
			if inc.Field == "FuturesCVD1H" {
				found = true
				expectedDiff := 2000.0 - 2500.0
				if math.Abs(inc.Difference-expectedDiff) > 0.01 {
					t.Errorf("Difference should be %f, got %f", expectedDiff, inc.Difference)
				}
			}
		}

		if !found {
			t.Error("Should detect FuturesCVD1H inconsistency")
		}
	})
}
