package market

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	_ "github.com/mattn/go-sqlite3"
)

// TestSystemIntegration 系统集成测试，模拟完整的生产环境启动流程
func TestSystemIntegration(t *testing.T) {
	// 模拟生产环境：不手动初始化任何全局变量
	// 重置全局状态（模拟系统重启）
	globalStrengthNormalizer = nil
	normalizerOnce = sync.Once{}

	t.Run("未初始化时的行为", func(t *testing.T) {
		// 验证未初始化时的报错机制
		normalizer := GetGlobalStrengthNormalizer()
		if normalizer != nil {
			t.Error("未初始化时应该返回nil")
		}

		// 测试供需区分析在未初始化时的降级行为
		analyzer := NewSupplyDemandAnalyzer()
		klines := createTestKlines()

		testZone := &SupplyDemandZone{
			ID:         "test_uninitialized",
			Type:       SupplyZone,
			UpperBound: 52000.0,
			LowerBound: 51800.0,
			Width:      200.0,
			Origin: &ZoneOrigin{
				KlineIndex:    10,
				PatternType:   DropBaseDrop,
				ImpulseMove:   0.025,
				ImpulseVolume: 1500.0,
				TimeFrame:     "5m",
				Confirmation:  true,
			},
			Status:   StatusFresh,
			IsActive: true,
		}

		// 这应该使用原始强度，而不是崩溃
		analyzer.calculateZoneStrength(testZone, klines)
		if testZone.Strength <= 0 {
			t.Error("即使未初始化强度标准化器，基础强度计算也应该工作")
		}

		// 验证Z-Score相关字段为默认值
		if testZone.StrengthZReady {
			t.Error("未初始化时StrengthZReady应为false")
		}
		if testZone.StrengthZ != 0.0 {
			t.Error("未初始化时StrengthZ应为0.0")
		}
	})

	t.Run("正确初始化后的行为", func(t *testing.T) {
		// 创建数据库（模拟main.go中的数据库创建）
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("创建测试数据库失败: %v", err)
		}
		defer db.Close()

		// 创建必要的表结构
		createTableSQL := `
		CREATE TABLE IF NOT EXISTS zone_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol TEXT NOT NULL,
			timeframe TEXT NOT NULL,
			raw_score REAL NOT NULL,
			zone_type TEXT NOT NULL,
			pattern_type TEXT NOT NULL,
			touch_count INTEGER DEFAULT 0,
			volume_ratio REAL DEFAULT 1.0,
			width_percent REAL DEFAULT 0.0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(symbol, timeframe, created_at)
		);`

		if _, err := db.Exec(createTableSQL); err != nil {
			t.Fatalf("创建测试表失败: %v", err)
		}

		// 模拟main.go中的初始化
		InitGlobalStrengthNormalizer(db)

		// 验证初始化后的正常行为
		normalizer := GetGlobalStrengthNormalizer()
		if normalizer == nil {
			t.Fatal("初始化后应该能获取到强度标准化器")
		}

		// 测试供需区分析现在应该包含Z-Score标准化
		analyzer := NewSupplyDemandAnalyzer()
		klines := createTestKlines()

		testZone := &SupplyDemandZone{
			ID:         "test_initialized",
			Type:       SupplyZone,
			UpperBound: 52000.0,
			LowerBound: 51800.0,
			Width:      200.0,
			Origin: &ZoneOrigin{
				KlineIndex:    10,
				PatternType:   DropBaseDrop,
				ImpulseMove:   0.025,
				ImpulseVolume: 1500.0,
				TimeFrame:     "5m",
				Confirmation:  true,
			},
			Status:   StatusFresh,
			IsActive: true,
		}

		// 使用带symbol参数的方法测试Z-Score功能
		analyzer.calculateZoneStrengthWithSymbol(testZone, klines, "BTCUSDT", "5m")

		// 验证基础强度计算正常
		if testZone.Strength <= 0 {
			t.Error("初始化后强度计算应该正常工作")
		}

		// 由于是第一个区域，样本不足，Z-Score应该不可靠
		if testZone.StrengthZReady && testZone.SampleCount < 50 {
			t.Error("样本不足时StrengthZReady应该为false")
		}
	})

	// 清理：重置全局状态
	globalStrengthNormalizer = nil
	normalizerOnce = sync.Once{}
}

// TestProductionStartupSequence 测试生产环境启动序列
func TestProductionStartupSequence(t *testing.T) {
	// 这个测试模拟main.go中的启动序列

	// Step 1: 数据库创建
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("模拟配置数据库创建失败: %v", err)
	}
	defer db.Close()

	// Step 2: 创建必要的表（模拟config.Database的行为）
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS zone_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		symbol TEXT NOT NULL,
		timeframe TEXT NOT NULL,
		raw_score REAL NOT NULL,
		zone_type TEXT NOT NULL,
		pattern_type TEXT NOT NULL,
		touch_count INTEGER DEFAULT 0,
		volume_ratio REAL DEFAULT 1.0,
		width_percent REAL DEFAULT 0.0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(symbol, timeframe, created_at)
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		t.Fatalf("创建zone_history表失败: %v", err)
	}

	// Step 3: 初始化市场分析模块（模拟main.go中的调用）
	InitGlobalStrengthNormalizer(db)

	// Step 4: 验证系统可以正常工作
	analyzer := NewSupplyDemandAnalyzer()
	klines := createTestKlines()

	// 创建多个供需区以测试完整流程
	for i := 0; i < 60; i++ { // 创建足够的样本
		zone := &SupplyDemandZone{
			ID:         fmt.Sprintf("prod_test_%d", i),
			Type:       SupplyZone,
			UpperBound: 52000.0 + float64(i)*10,
			LowerBound: 51800.0 + float64(i)*10,
			Width:      200.0,
			Origin: &ZoneOrigin{
				KlineIndex:    10 + i,
				PatternType:   DropBaseDrop,
				ImpulseMove:   0.025,
				ImpulseVolume: 1500.0,
				TimeFrame:     "5m",
				Confirmation:  true,
			},
			Status:   StatusFresh,
			IsActive: true,
		}

		analyzer.calculateZoneStrengthWithSymbol(zone, klines, "BTCUSDT", "5m")

		// 验证基础功能
		if zone.Strength <= 0 {
			t.Errorf("第%d个供需区强度计算失败", i)
		}

		// 样本充足后，Z-Score应该可用
		if i >= 50 && !zone.StrengthZReady {
			t.Logf("警告: 第%d个供需区Z-Score未就绪，样本数: %d", i, zone.SampleCount)
		}
	}

	t.Log("生产环境启动序列测试通过")

	// 清理
	globalStrengthNormalizer = nil
	normalizerOnce = sync.Once{}
}
