package microstructure

import (
	"testing"
	"time"
)

// TestDataQualityP0_06_Fix 验证P0-06数据质量监控修复：使用真实组件更新时间
func TestDataQualityP0_06_Fix(t *testing.T) {
	// 创建订单流管理器
	config := DefaultMicrostructureConfig()
	ofm := NewOrderFlowManager(config)
	
	// 模拟不同时间的组件数据更新
	now := time.Now()
	cvdTime := now.Add(-2 * time.Minute)    // CVD 2分钟前更新
	oiTime := now.Add(-5 * time.Minute)     // OI 5分钟前更新  
	obTime := now.Add(-1 * time.Minute)     // OrderBook 1分钟前更新

	// 创建模拟的组件数据
	cvdData := &CVDData{
		SpotCVD1H:    100000,
		FuturesCVD1H: -50000,
		Signal:       "neutral",
		LastUpdate:   cvdTime,
		IsStale:      false,
	}
	
	oiAnalysis := &OIAnalysis{
		Current:      1000000,
		Change1H:     50000,
		ChangeRate1H: 5.0,
		Trend:        "increasing",
		LastUpdate:   oiTime,
		IsStale:      false,
	}
	
	orderBookData := &OrderBookData{
		ImbalanceRatio:    0.3,
		BidPressure:       0.6,
		AskPressure:       0.4,
		LiquidityScore:    0.8,
		LastUpdate:        obTime,
		IsStale:          false,
	}

	// 🔥 P0-06修复验证：调用修复后的calculateDataQuality方法
	dataQuality := ofm.calculateDataQuality("BTCUSDT", cvdData, oiAnalysis, orderBookData)
	
	// ✅ 验证LastDataUpdate使用最新组件时间（OrderBook 1分钟前）而非time.Now()
	expectedLastUpdate := obTime // OrderBook是最新的
	if !dataQuality.LastDataUpdate.Equal(expectedLastUpdate) {
		t.Errorf("❌ P0-06修复失败: LastDataUpdate期望%v, 实际%v", 
			expectedLastUpdate.Format("15:04:05"), 
			dataQuality.LastDataUpdate.Format("15:04:05"))
	}
	
	// ✅ 验证DataLagMs使用最老组件时间（OI 5分钟前）计算延迟
	expectedLagMs := now.Sub(oiTime).Milliseconds() // now - 最老时间(OI)
	actualLagMs := dataQuality.DataLagMs
	
	// 允许±1秒的误差
	if actualLagMs < expectedLagMs-1000 || actualLagMs > expectedLagMs+1000 {
		t.Errorf("❌ P0-06修复失败: DataLagMs期望约%dms, 实际%dms (差值: %dms)", 
			expectedLagMs, actualLagMs, actualLagMs-expectedLagMs)
	}
	
	// ✅ 验证单独的组件更新时间跟踪
	if !dataQuality.CVDLastUpdate.Equal(cvdTime) {
		t.Errorf("❌ P0-06修复失败: CVDLastUpdate期望%v, 实际%v", 
			cvdTime.Format("15:04:05"), dataQuality.CVDLastUpdate.Format("15:04:05"))
	}
	
	if !dataQuality.OILastUpdate.Equal(oiTime) {
		t.Errorf("❌ P0-06修复失败: OILastUpdate期望%v, 实际%v", 
			oiTime.Format("15:04:05"), dataQuality.OILastUpdate.Format("15:04:05"))
	}
	
	if !dataQuality.OrderBookLastUpdate.Equal(obTime) {
		t.Errorf("❌ P0-06修复失败: OrderBookLastUpdate期望%v, 实际%v", 
			obTime.Format("15:04:05"), dataQuality.OrderBookLastUpdate.Format("15:04:05"))
	}
	
	// ✅ 验证综合数据质量评估正常工作
	if dataQuality.OverallScore <= 0 || dataQuality.OverallScore > 1 {
		t.Errorf("❌ 综合评分异常: %f (应在0-1范围)", dataQuality.OverallScore)
	}
	
	t.Logf("✅ P0-06数据质量监控修复验证通过:")
	t.Logf("   LastDataUpdate: %s (最新组件时间)", dataQuality.LastDataUpdate.Format("15:04:05"))
	t.Logf("   DataLagMs: %dms (最老组件延迟)", dataQuality.DataLagMs)
	t.Logf("   CVD更新: %s", dataQuality.CVDLastUpdate.Format("15:04:05"))
	t.Logf("   OI更新: %s", dataQuality.OILastUpdate.Format("15:04:05"))
	t.Logf("   OrderBook更新: %s", dataQuality.OrderBookLastUpdate.Format("15:04:05"))
	t.Logf("   综合评分: %.2f", dataQuality.OverallScore)
	t.Logf("   状态: %s", dataQuality.Status)
}

// TestDataQualityEdgeCases_P0_06 测试P0-06修复的边界情况
func TestDataQualityEdgeCases_P0_06(t *testing.T) {
	config := DefaultMicrostructureConfig()
	ofm := NewOrderFlowManager(config)
	
	// 测试用例1：所有组件都为nil
	t.Run("所有组件为nil", func(t *testing.T) {
		dataQuality := ofm.calculateDataQuality("BTCUSDT", nil, nil, nil)
		
		// LastDataUpdate应该使用兜底的time.Now()，DataLagMs应该是默认值
		if time.Since(dataQuality.LastDataUpdate) > 2*time.Second {
			t.Errorf("❌ 兜底时间异常: LastDataUpdate距离现在%v", 
				time.Since(dataQuality.LastDataUpdate))
		}
		
		if dataQuality.DataLagMs != 60000 { // 1分钟兜底值
			t.Errorf("❌ 兜底DataLagMs期望60000ms, 实际%dms", dataQuality.DataLagMs)
		}
		
		t.Logf("✅ nil组件兜底处理正确: DataLagMs=%dms", dataQuality.DataLagMs)
	})
	
	// 测试用例2：只有一个组件有效
	t.Run("单组件有效", func(t *testing.T) {
		testTime := time.Now().Add(-3 * time.Minute)
		cvdData := &CVDData{
			SpotCVD1H:  50000,
			LastUpdate: testTime,
			IsStale:    false,
		}
		
		dataQuality := ofm.calculateDataQuality("BTCUSDT", cvdData, nil, nil)
		
		// LastDataUpdate和最老时间都应该是同一个时间
		if !dataQuality.LastDataUpdate.Equal(testTime) {
			t.Errorf("❌ 单组件LastDataUpdate期望%v, 实际%v", 
				testTime.Format("15:04:05"), dataQuality.LastDataUpdate.Format("15:04:05"))
		}
		
		expectedLag := time.Since(testTime).Milliseconds()
		if dataQuality.DataLagMs < expectedLag-1000 || dataQuality.DataLagMs > expectedLag+1000 {
			t.Errorf("❌ 单组件DataLagMs期望约%dms, 实际%dms", 
				expectedLag, dataQuality.DataLagMs)
		}
		
		// 单独组件跟踪
		if !dataQuality.CVDLastUpdate.Equal(testTime) {
			t.Errorf("❌ CVD组件跟踪失败")
		}
		
		if !dataQuality.OILastUpdate.IsZero() {
			t.Errorf("❌ 无效OI组件应为零时间")
		}
		
		t.Logf("✅ 单组件处理正确: Lag=%dms", dataQuality.DataLagMs)
	})
}