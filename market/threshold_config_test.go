package market

import (
	"testing"
)

// TestThresholdConfig_DefaultValues 测试P0-06: 默认配置值是否正确设置
func TestThresholdConfig_DefaultValues(t *testing.T) {
	analyzer := NewSupplyDemandAnalyzer()

	// 验证交互判定配置
	if analyzer.config.Interaction.ATRBuffer0_1 != 0.1 {
		t.Errorf("ATRBuffer0_1 默认值错误: 期望0.1, 实际%.2f", analyzer.config.Interaction.ATRBuffer0_1)
	}
	if analyzer.config.Interaction.ATRBuffer0_2 != 0.2 {
		t.Errorf("ATRBuffer0_2 默认值错误: 期望0.2, 实际%.2f", analyzer.config.Interaction.ATRBuffer0_2)
	}
	if analyzer.config.Interaction.ATRBuffer0_5 != 0.5 {
		t.Errorf("ATRBuffer0_5 默认值错误: 期望0.5, 实际%.2f", analyzer.config.Interaction.ATRBuffer0_5)
	}
	if analyzer.config.Interaction.ZoneOverlapThreshold != 0.7 {
		t.Errorf("ZoneOverlapThreshold 默认值错误: 期望0.7, 实际%.2f", analyzer.config.Interaction.ZoneOverlapThreshold)
	}
	if analyzer.config.Interaction.RecentBarThreshold != 10 {
		t.Errorf("RecentBarThreshold 默认值错误: 期望10, 实际%d", analyzer.config.Interaction.RecentBarThreshold)
	}

	// 验证距离配置
	if analyzer.config.Distance.MaxValidDistance != 5.0 {
		t.Errorf("MaxValidDistance 默认值错误: 期望5.0, 实际%.2f", analyzer.config.Distance.MaxValidDistance)
	}
	if analyzer.config.Distance.NoInteractionPct != 0.01 {
		t.Errorf("NoInteractionPct 默认值错误: 期望0.01, 实际%.2f", analyzer.config.Distance.NoInteractionPct)
	}

	// 验证强度权重配置
	if analyzer.config.Strength.BaseScore != 25.0 {
		t.Errorf("BaseScore 默认值错误: 期望25.0, 实际%.2f", analyzer.config.Strength.BaseScore)
	}
	if analyzer.config.Strength.ImpulseMoveWeight != 30.0 {
		t.Errorf("ImpulseMoveWeight 默认值错误: 期望30.0, 实际%.2f", analyzer.config.Strength.ImpulseMoveWeight)
	}
	if analyzer.config.Strength.VolumeWeight != 8.0 {
		t.Errorf("VolumeWeight 默认值错误: 期望8.0, 实际%.2f", analyzer.config.Strength.VolumeWeight)
	}

	// 验证信号距离配置
	if analyzer.config.Signal.SDApproachDistance != 0.05 {
		t.Errorf("SDApproachDistance 默认值错误: 期望0.05, 实际%.2f", analyzer.config.Signal.SDApproachDistance)
	}
	if analyzer.config.Signal.FVGEntryDistance != 0.01 {
		t.Errorf("FVGEntryDistance 默认值错误: 期望0.01, 实际%.2f", analyzer.config.Signal.FVGEntryDistance)
	}

	t.Logf("✅ 所有默认配置值验证通过")
}

// TestThresholdConfig_CustomValues 测试P0-06: 自定义配置是否生效
func TestThresholdConfig_CustomValues(t *testing.T) {
	// 创建自定义配置
	customConfig := defaultSDConfig
	customConfig.Interaction.ATRBuffer0_2 = 0.3 // 修改为0.3
	customConfig.Interaction.ZoneOverlapThreshold = 0.6 // 修改为60%
	customConfig.Distance.MaxValidDistance = 3.0 // 修改为3倍ATR
	customConfig.Strength.BaseScore = 30.0 // 修改基础分数为30

	// 使用自定义配置创建分析器
	analyzer := NewSupplyDemandAnalyzerWithConfig(customConfig)

	// 验证自定义值是否生效
	if analyzer.config.Interaction.ATRBuffer0_2 != 0.3 {
		t.Errorf("自定义ATRBuffer0_2未生效: 期望0.3, 实际%.2f", analyzer.config.Interaction.ATRBuffer0_2)
	}
	if analyzer.config.Interaction.ZoneOverlapThreshold != 0.6 {
		t.Errorf("自定义ZoneOverlapThreshold未生效: 期望0.6, 实际%.2f", analyzer.config.Interaction.ZoneOverlapThreshold)
	}
	if analyzer.config.Distance.MaxValidDistance != 3.0 {
		t.Errorf("自定义MaxValidDistance未生效: 期望3.0, 实际%.2f", analyzer.config.Distance.MaxValidDistance)
	}
	if analyzer.config.Strength.BaseScore != 30.0 {
		t.Errorf("自定义BaseScore未生效: 期望30.0, 实际%.2f", analyzer.config.Strength.BaseScore)
	}

	t.Logf("✅ 自定义配置值验证通过")
}

// TestThresholdConfig_isTrueBreakout 测试P0-06: 真假突破判定使用配置化阈值
func TestThresholdConfig_isTrueBreakout(t *testing.T) {
	klines := generateDeterministicKlines(100, "15m")

	// 场景1: 使用默认配置（0.2倍ATR缓冲）
	t.Run("DefaultBuffer_0_2", func(t *testing.T) {
		analyzer := NewSupplyDemandAnalyzer()
		sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

		if len(sdData.DemandZones) > 0 {
			zone := sdData.DemandZones[0]
			atr := analyzer.calculateATR(klines, 14)

			// 测试价格在缓冲区内（不算真突破）
			priceInBuffer := zone.LowerBound - 0.15*atr
			isBreakout := analyzer.isTrueBreakout(zone, priceInBuffer, atr)
			if isBreakout {
				t.Error("缓冲区内应判定为假突破")
			}

			// 测试价格超过缓冲区（算真突破）
			priceOutBuffer := zone.LowerBound - 0.25*atr
			isBreakout = analyzer.isTrueBreakout(zone, priceOutBuffer, atr)
			if !isBreakout {
				t.Error("超过缓冲区应判定为真突破")
			}

			t.Logf("✅ 默认缓冲配置(0.2*ATR)判定正确")
		}
	})

	// 场景2: 使用自定义配置（0.3倍ATR缓冲）
	t.Run("CustomBuffer_0_3", func(t *testing.T) {
		customConfig := defaultSDConfig
		customConfig.Interaction.ATRBuffer0_2 = 0.3 // 自定义为0.3
		analyzer := NewSupplyDemandAnalyzerWithConfig(customConfig)

		sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

		if len(sdData.DemandZones) > 0 {
			zone := sdData.DemandZones[0]
			atr := analyzer.calculateATR(klines, 14)

			// 测试价格在扩大的缓冲区内（不算真突破）
			priceInBuffer := zone.LowerBound - 0.25*atr
			isBreakout := analyzer.isTrueBreakout(zone, priceInBuffer, atr)
			if isBreakout {
				t.Error("扩大缓冲区内应判定为假突破")
			}

			// 测试价格超过扩大的缓冲区（算真突破）
			priceOutBuffer := zone.LowerBound - 0.35*atr
			isBreakout = analyzer.isTrueBreakout(zone, priceOutBuffer, atr)
			if !isBreakout {
				t.Error("超过扩大缓冲区应判定为真突破")
			}

			t.Logf("✅ 自定义缓冲配置(0.3*ATR)判定正确")
		}
	})
}

// TestThresholdConfig_validateZonePosition 测试P0-06: 区域位置验证使用配置化距离
func TestThresholdConfig_validateZonePosition(t *testing.T) {
	klines := generateDeterministicKlines(100, "15m")

	// 场景1: 默认配置（5倍ATR最大距离）
	t.Run("DefaultMaxDistance_5_0", func(t *testing.T) {
		analyzer := NewSupplyDemandAnalyzer()
		atr := analyzer.calculateATR(klines, 14)
		currentPrice := klines[len(klines)-1].Close

		// 创建测试区域：距离4.5倍ATR（在范围内）
		zoneInRange := &SupplyDemandZone{
			LowerBound: currentPrice + 4.5*atr,
			UpperBound: currentPrice + 4.7*atr,
		}

		if !analyzer.validateZonePosition(zoneInRange, currentPrice, atr) {
			t.Error("4.5倍ATR距离应该验证通过")
		}

		// 创建测试区域：距离5.5倍ATR（超出范围）
		zoneOutRange := &SupplyDemandZone{
			LowerBound: currentPrice + 5.5*atr,
			UpperBound: currentPrice + 5.7*atr,
		}

		if analyzer.validateZonePosition(zoneOutRange, currentPrice, atr) {
			t.Error("5.5倍ATR距离应该验证失败")
		}

		t.Logf("✅ 默认最大距离配置(5.0*ATR)验证正确")
	})

	// 场景2: 自定义配置（3倍ATR最大距离）
	t.Run("CustomMaxDistance_3_0", func(t *testing.T) {
		customConfig := defaultSDConfig
		customConfig.Distance.MaxValidDistance = 3.0
		analyzer := NewSupplyDemandAnalyzerWithConfig(customConfig)

		atr := analyzer.calculateATR(klines, 14)
		currentPrice := klines[len(klines)-1].Close

		// 距离2.5倍ATR（在范围内）
		zoneInRange := &SupplyDemandZone{
			LowerBound: currentPrice + 2.5*atr,
			UpperBound: currentPrice + 2.7*atr,
		}

		if !analyzer.validateZonePosition(zoneInRange, currentPrice, atr) {
			t.Error("2.5倍ATR距离应该验证通过")
		}

		// 距离3.5倍ATR（超出范围）
		zoneOutRange := &SupplyDemandZone{
			LowerBound: currentPrice + 3.5*atr,
			UpperBound: currentPrice + 3.7*atr,
		}

		if analyzer.validateZonePosition(zoneOutRange, currentPrice, atr) {
			t.Error("3.5倍ATR距离应该验证失败")
		}

		t.Logf("✅ 自定义最大距离配置(3.0*ATR)验证正确")
	})
}

// TestThresholdConfig_zonesOverlap 测试P0-06: 区域重叠判定使用配置化阈值
func TestThresholdConfig_zonesOverlap(t *testing.T) {
	// 场景1: 默认配置（70%重叠阈值）
	t.Run("DefaultOverlap_0_7", func(t *testing.T) {
		analyzer := NewSupplyDemandAnalyzer()

		// 创建两个区域：重叠75%
		zone1 := &SupplyDemandZone{
			LowerBound: 100.0,
			UpperBound: 110.0,
		}
		zone2 := &SupplyDemandZone{
			LowerBound: 102.5,
			UpperBound: 112.5,
		}

		if !analyzer.zonesOverlap(zone1, zone2) {
			t.Error("75%重叠应判定为重叠")
		}

		// 创建两个区域：重叠65%
		zone3 := &SupplyDemandZone{
			LowerBound: 100.0,
			UpperBound: 110.0,
		}
		zone4 := &SupplyDemandZone{
			LowerBound: 103.5,
			UpperBound: 113.5,
		}

		if analyzer.zonesOverlap(zone3, zone4) {
			t.Error("65%重叠应判定为不重叠")
		}

		t.Logf("✅ 默认重叠阈值配置(0.7)判定正确")
	})

	// 场景2: 自定义配置（60%重叠阈值）
	t.Run("CustomOverlap_0_6", func(t *testing.T) {
		customConfig := defaultSDConfig
		customConfig.Interaction.ZoneOverlapThreshold = 0.6
		analyzer := NewSupplyDemandAnalyzerWithConfig(customConfig)

		// 65%重叠：在60%阈值下应判定为重叠
		zone1 := &SupplyDemandZone{
			LowerBound: 100.0,
			UpperBound: 110.0,
		}
		zone2 := &SupplyDemandZone{
			LowerBound: 103.5,
			UpperBound: 113.5,
		}

		if !analyzer.zonesOverlap(zone1, zone2) {
			t.Error("65%重叠在60%阈值下应判定为重叠")
		}

		t.Logf("✅ 自定义重叠阈值配置(0.6)判定正确")
	})
}

// TestThresholdConfig_IntegrationTest 测试P0-06: 完整流程集成测试
func TestThresholdConfig_IntegrationTest(t *testing.T) {
	klines := generateDeterministicKlines(100, "15m")

	// 场景1: 默认配置分析
	t.Run("DefaultConfig", func(t *testing.T) {
		analyzer := NewSupplyDemandAnalyzer()
		sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

		t.Logf("默认配置识别: 供给区%d个, 需求区%d个, 活跃区%d个",
			len(sdData.SupplyZones), len(sdData.DemandZones), len(sdData.ActiveZones))

		t.Logf("✅ 默认配置分析完成")
	})

	// 场景2: 宽松配置（扩大缓冲区和距离限制）
	t.Run("RelaxedConfig", func(t *testing.T) {
		relaxedConfig := defaultSDConfig
		relaxedConfig.Interaction.ATRBuffer0_2 = 0.3 // 更大缓冲，减少假突破
		relaxedConfig.Distance.MaxValidDistance = 8.0 // 更大范围，保留更多区域
		relaxedConfig.Interaction.ZoneOverlapThreshold = 0.8 // 更高重叠才互斥

		analyzer := NewSupplyDemandAnalyzerWithConfig(relaxedConfig)
		sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

		t.Logf("宽松配置识别: 供给区%d个, 需求区%d个, 活跃区%d个",
			len(sdData.SupplyZones), len(sdData.DemandZones), len(sdData.ActiveZones))

		t.Logf("✅ 宽松配置分析完成")
	})

	// 场景3: 严格配置（缩小缓冲区和距离限制）
	t.Run("StrictConfig", func(t *testing.T) {
		strictConfig := defaultSDConfig
		strictConfig.Interaction.ATRBuffer0_2 = 0.1 // 更小缓冲，更快确认突破
		strictConfig.Distance.MaxValidDistance = 3.0 // 更小范围，只保留近期区域
		strictConfig.Interaction.ZoneOverlapThreshold = 0.5 // 更低重叠就互斥

		analyzer := NewSupplyDemandAnalyzerWithConfig(strictConfig)
		sdData := analyzer.AnalyzeWithSymbol(klines, "BTCUSDT", "15m")

		t.Logf("严格配置识别: 供给区%d个, 需求区%d个, 活跃区%d个",
			len(sdData.SupplyZones), len(sdData.DemandZones), len(sdData.ActiveZones))

		t.Logf("✅ 严格配置分析完成")
	})
}
