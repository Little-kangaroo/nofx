package market

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"
)

// TestSupplyDemandZScoreIntegration 供需区Z-Score集成测试
func TestSupplyDemandZScoreIntegration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 初始化全局标准化器
	InitGlobalStrengthNormalizer(db)
	defer func() {
		globalStrengthNormalizer = nil
		normalizerOnce = sync.Once{}
	}()

	// 创建供需区分析器
	analyzer := NewSupplyDemandAnalyzer()
	
	// 创建测试K线数据
	klines := createTestKlines()

	// 手动创建一个供需区用于测试
	testZone := &SupplyDemandZone{
		ID:            "test_supply_1",
		Type:          SupplyZone,
		UpperBound:    52000.0,
		LowerBound:    51800.0,
		CenterPrice:   51900.0,
		Width:         200.0,
		WidthPercent:  0.385, // (200/51800)*100 ≈ 0.385%
		Origin: &ZoneOrigin{
			KlineIndex:    50,
			PatternType:   DropBaseDrop,
			ImpulseMove:   0.025,
			ImpulseVolume: 1500.0,
			TimeFrame:     "5m",
			Confirmation:  true,
		},
		Strength:     0.0, // 将由calculateZoneStrength计算
		Quality:      QualityGood,
		Status:       StatusFresh,
		TouchCount:   0,
		CreationTime: time.Now().UnixMilli(),
		IsActive:     true,
		IsBroken:     false,
		Volume:       1500.0,
		VolumeProfile: &ZoneVP{
			TotalVolume:     1500.0,
			BuyVolume:       450.0,
			SellVolume:      1050.0,
			VolumeAtOrigin:  1500.0,
			VolumeImbalance: 0.43, // 450/1050 ≈ 0.43
		},
	}

	// 使用带symbol参数的强度计算方法
	analyzer.calculateZoneStrengthWithSymbol(testZone, klines, "BTCUSDT", "5m")

	// 验证基础强度计算
	if testZone.Strength <= 0 {
		t.Errorf("供需区强度计算异常: %f", testZone.Strength)
	}

	// 由于是第一个区域，可能样本不足，Z分数应该不可靠
	if testZone.StrengthZReady && testZone.SampleCount < MinSampleSize {
		t.Error("样本不足时StrengthZReady不应为true")
	}

	t.Logf("第一个供需区: 原始强度=%.2f, Z分数=%.3f, 样本数=%d, 是否就绪=%v", 
		testZone.Strength, testZone.StrengthZ, testZone.SampleCount, testZone.StrengthZReady)

	// 创建足够的供需区以达到最小样本要求
	testZones := []*SupplyDemandZone{testZone}
	
	for i := 1; i < MinSampleSize+5; i++ {
		zone := &SupplyDemandZone{
			ID:           fmt.Sprintf("test_zone_%d", i),
			Type:         SupplyZone,
			UpperBound:   52000.0 + float64(i)*100,
			LowerBound:   51800.0 + float64(i)*100,
			CenterPrice:  51900.0 + float64(i)*100,
			Width:        200.0 + float64(i)*5,
			WidthPercent: (200.0 + float64(i)*5) / (51800.0 + float64(i)*100) * 100,
			Origin: &ZoneOrigin{
				KlineIndex:    50 + i,
				PatternType:   DropBaseDrop,
				ImpulseMove:   0.02 + float64(i)*0.005,
				ImpulseVolume: 1000.0 + float64(i)*200,
				TimeFrame:     "5m",
				Confirmation:  true,
			},
			Quality:      QualityGood,
			Status:       StatusFresh,
			TouchCount:   0,
			CreationTime: time.Now().UnixMilli() + int64(i)*60000, // 每分钟一个
			IsActive:     true,
			IsBroken:     false,
			Volume:       1000.0 + float64(i)*200,
			VolumeProfile: &ZoneVP{
				TotalVolume:     1000.0 + float64(i)*200,
				BuyVolume:       300.0 + float64(i)*60,
				SellVolume:      700.0 + float64(i)*140,
				VolumeAtOrigin:  1000.0 + float64(i)*200,
				VolumeImbalance: (300.0 + float64(i)*60) / (700.0 + float64(i)*140),
			},
		}

		// 计算强度（包含Z分数标准化）
		analyzer.calculateZoneStrengthWithSymbol(zone, klines, "BTCUSDT", "5m")
		testZones = append(testZones, zone)
	}

	// 验证最后几个区域的Z分数标准化
	lastZone := testZones[len(testZones)-1]
	if !lastZone.StrengthZReady {
		t.Error("样本充足时最后的供需区StrengthZReady应为true")
	}

	if lastZone.SampleCount < MinSampleSize {
		t.Errorf("最后的供需区样本数不足: %d < %d", lastZone.SampleCount, MinSampleSize)
	}

	if math.IsNaN(lastZone.StrengthZ) || math.IsInf(lastZone.StrengthZ, 0) {
		t.Errorf("最后的供需区Z分数异常: %f", lastZone.StrengthZ)
	}

	if lastZone.StrengthZ > MaxZScore || lastZone.StrengthZ < -MaxZScore {
		t.Errorf("最后的供需区Z分数超出范围: %f", lastZone.StrengthZ)
	}

	t.Logf("最后的供需区: 原始强度=%.2f, Z分数=%.3f, 样本数=%d", 
		lastZone.Strength, lastZone.StrengthZ, lastZone.SampleCount)

	// 验证Z分数的分布合理性
	var zScores []float64
	readyCount := 0
	for _, zone := range testZones {
		if zone.StrengthZReady {
			zScores = append(zScores, zone.StrengthZ)
			readyCount++
		}
	}

	if readyCount > 0 {
		// 计算Z分数的均值和标准差
		zMean := 0.0
		for _, z := range zScores {
			zMean += z
		}
		zMean /= float64(len(zScores))

		zStdDev := 0.0
		for _, z := range zScores {
			diff := z - zMean
			zStdDev += diff * diff
		}
		zStdDev = math.Sqrt(zStdDev / float64(len(zScores)))

		t.Logf("Z分数分布: 均值=%.3f, 标准差=%.3f, 样本数=%d", zMean, zStdDev, len(zScores))

		// Z分数均值应该接近0（标准化的特性）
		if math.Abs(zMean) > 0.5 {
			t.Logf("警告: Z分数均值偏离0较远: %.3f", zMean)
		}
	}

	// 测试不同币种的分离存储
	for i := 0; i < 5; i++ {
		zone := &SupplyDemandZone{
			ID:           fmt.Sprintf("eth_zone_%d", i),
			Type:         DemandZone,
			UpperBound:   3200.0 + float64(i)*10,
			LowerBound:   3180.0 + float64(i)*10,
			CenterPrice:  3190.0 + float64(i)*10,
			Width:        20.0,
			WidthPercent: 0.63, // 20/3180*100 ≈ 0.63%
			Origin: &ZoneOrigin{
				PatternType:   RallyBaseRally,
				ImpulseMove:   0.03,
				ImpulseVolume: 800.0,
				TimeFrame:     "15m",
			},
			Quality:      QualityModerate,
			Status:       StatusFresh,
			IsActive:     true,
			Volume:       800.0,
		}

		analyzer.calculateZoneStrengthWithSymbol(zone, klines, "ETHUSDT", "15m")
	}

	// 验证不同币种的统计状态是分离的
	normalizer := GetGlobalStrengthNormalizer()
	allStats := normalizer.GetAllStatsInfo()
	
	btcKey := "BTCUSDT_5m"
	ethKey := "ETHUSDT_15m"
	
	if btcSamples, exists := allStats[btcKey]; !exists || btcSamples == 0 {
		t.Errorf("BTCUSDT_5m的统计状态缺失或样本数为0")
	}
	
	if ethSamples, exists := allStats[ethKey]; !exists || ethSamples == 0 {
		t.Errorf("ETHUSDT_15m的统计状态缺失或样本数为0")
	}

	t.Logf("统计状态: %s有%d个样本, %s有%d个样本", 
		btcKey, allStats[btcKey], ethKey, allStats[ethKey])

	t.Log("供需区Z-Score集成测试通过")
}

// TestZScoreConsistency Z分数一致性测试
func TestZScoreConsistency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	normalizer := NewStrengthNormalizer(db)
	symbol := "CONSISTENCYTEST"
	timeframe := "1h"

	// 添加一系列已知分布的数据
	knownScores := []float64{
		30, 35, 40, 45, 50, 55, 60, 65, 70, 75, // 基础数据
		32, 38, 42, 48, 52, 58, 62, 68, 72, 78, // 更多变化
		31, 36, 41, 46, 51, 56, 61, 66, 71, 76, // 均匀分布
		33, 37, 43, 47, 53, 57, 63, 67, 73, 77, // 填补空隙
		29, 34, 39, 44, 49, 54, 59, 64, 69, 74, // 扩展范围
	}

	// 添加数据
	for i, score := range knownScores {
		record := &ZoneStrengthRecord{
			Symbol:      symbol,
			Timeframe:   timeframe,
			RawScore:    score,
			ZoneType:    "supply",
			PatternType: "test_pattern",
		}
		normalizer.AddScore(symbol, timeframe, score, record)
		
		if i >= MinSampleSize-1 { // 达到最小样本后开始验证
			result := normalizer.GetZScore(symbol, timeframe, score)
			
			// 验证Z分��计算的一致性
			result2 := normalizer.GetZScore(symbol, timeframe, score)
			if math.Abs(result.ZScore-result2.ZScore) > 1e-10 {
				t.Errorf("相同输入的Z分数计算不一致: %.10f vs %.10f", result.ZScore, result2.ZScore)
			}
			
			// 验证统计特性
			if result.IsReady {
				if result.Mean == 0 && result.StdDev == 0 {
					t.Error("Z分数计算中均值和标准差不应该同时为0")
				}
				
				// 当前分数的Z分数应该接近0（因为它就在历史数据中）
				if math.Abs(result.ZScore) > 0.5 { // 允许一定误差
					t.Logf("当前分数的Z分数偏大: %.3f (分数=%.1f, 均值=%.1f, 标准差=%.1f)", 
						result.ZScore, score, result.Mean, result.StdDev)
				}
			}
		}
	}

	// 测试极值Z分数计算
	testCases := []struct {
		input    float64
		expected string
	}{
		{10.0, "very_negative"}, // 远低于历史数据
		{90.0, "very_positive"}, // 远高于历史数据
		{50.0, "near_zero"},     // 接近均值
	}

	for _, tc := range testCases {
		result := normalizer.GetZScore(symbol, timeframe, tc.input)
		if !result.IsReady {
			t.Errorf("测试用例%v的Z分数计算未就绪", tc)
			continue
		}

		switch tc.expected {
		case "very_negative":
			if result.ZScore >= -1.0 {
				t.Errorf("极低分数应该产生很负的Z分数: 输入=%.1f, Z分数=%.3f", tc.input, result.ZScore)
			}
		case "very_positive":
			if result.ZScore <= 1.0 {
				t.Errorf("极高分数应该产生很正的Z分数: 输入=%.1f, Z分数=%.3f", tc.input, result.ZScore)
			}
		case "near_zero":
			if math.Abs(result.ZScore) > 1.0 {
				t.Errorf("接近均值的分数应该产生接近0的Z分数: 输入=%.1f, Z分数=%.3f", tc.input, result.ZScore)
			}
		}
		
		t.Logf("测试用例: 输入=%.1f, Z分数=%.3f, 期望=%s", tc.input, result.ZScore, tc.expected)
	}

	t.Log("Z分数一致性测试通过")
}

// createTestKlines 创建测试K线数据
func createTestKlines() []Kline {
	klines := make([]Kline, 100)
	basePrice := 51000.0
	baseTime := time.Now().UnixMilli() - int64(100*5*60*1000) // 100个5分钟K线
	
	for i := range klines {
		// 生成有一定波动的价格数据
		priceVariation := float64(i%20-10) * 50 // ±500的波动
		trend := float64(i) * 2                  // 小幅上升趋势
		price := basePrice + trend + priceVariation
		
		volume := 1000.0 + float64(i%30)*50 // 1000-2450的成交量变化
		
		klines[i] = Kline{
			OpenTime:  baseTime + int64(i*5*60*1000), // 5分钟间隔
			Open:      price - 5,
			High:      price + 20 + float64(i%5)*10,
			Low:       price - 20 - float64(i%3)*5,
			Close:     price,
			Volume:    volume,
			CloseTime: baseTime + int64((i+1)*5*60*1000) - 1,
		}
	}
	
	return klines
}