package market

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *sql.DB {
	// 创建内存数据库用于测试
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}

	// 创建zone_history表
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
	);
	CREATE INDEX idx_symbol_timeframe ON zone_history(symbol, timeframe);
	CREATE INDEX idx_created_at ON zone_history(created_at);
	`

	if _, err := db.Exec(createTableSQL); err != nil {
		t.Fatalf("创建测试表失败: %v", err)
	}

	return db
}

// TestStrengthNormalizerBasic 基础功能测试
func TestStrengthNormalizerBasic(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 创建强度标准化器
	normalizer := NewStrengthNormalizer(db)
	if normalizer == nil {
		t.Fatal("创建强度标准化器失败")
	}

	symbol := "BTCUSDT"
	timeframe := "5m"

	// 测试1: 初始状态没有数据
	result := normalizer.GetZScore(symbol, timeframe, 50.0)
	if result.IsReady {
		t.Error("预期IsReady为false，实际为true")
	}
	if result.SampleCount != 0 {
		t.Errorf("预期SampleCount为0，实际为%d", result.SampleCount)
	}

	// 测试2: 添加数据但不足最小样本要求
	for i := 0; i < MinSampleSize-1; i++ {
		score := 40.0 + float64(i)*2.0 // 40-136的分数
		record := &ZoneStrengthRecord{
			Symbol:       symbol,
			Timeframe:    timeframe,
			RawScore:     score,
			ZoneType:     "supply",
			PatternType:  "drop_base_drop",
			TouchCount:   0,
			VolumeRatio:  1.0,
			WidthPercent: 2.0,
		}
		normalizer.AddScore(symbol, timeframe, score, record)
	}

	result = normalizer.GetZScore(symbol, timeframe, 50.0)
	if result.IsReady {
		t.Error("预期IsReady为false（样本不足），实际为true")
	}
	if result.SampleCount != MinSampleSize-1 {
		t.Errorf("预期SampleCount为%d，实际为%d", MinSampleSize-1, result.SampleCount)
	}

	// 测试3: 达到最小样本要求
	normalizer.AddScore(symbol, timeframe, 100.0, &ZoneStrengthRecord{
		Symbol:      symbol,
		Timeframe:   timeframe,
		RawScore:    100.0,
		ZoneType:    "demand",
		PatternType: "rally_base_rally",
	})

	result = normalizer.GetZScore(symbol, timeframe, 50.0)
	if !result.IsReady {
		t.Error("预期IsReady为true（样本充足），实际为false")
	}
	if result.SampleCount != MinSampleSize {
		t.Errorf("预期SampleCount为%d，实际为%d", MinSampleSize, result.SampleCount)
	}

	// 测试Z分数计算
	if math.IsNaN(result.ZScore) || math.IsInf(result.ZScore, 0) {
		t.Errorf("Z分数计算异常: %f", result.ZScore)
	}

	// 测试Z分数截断
	if result.ZScore > MaxZScore || result.ZScore < -MaxZScore {
		t.Errorf("Z分数未正确截断: %f，应在[%.1f, %.1f]范围内", result.ZScore, -MaxZScore, MaxZScore)
	}

	t.Logf("基础测试通过: Z分数=%.3f, 样本数=%d", result.ZScore, result.SampleCount)
}

// TestStrengthNormalizerColdStart 冷启动降级策略测试
func TestStrengthNormalizerColdStart(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	normalizer := NewStrengthNormalizer(db)
	coldStart := NewColdStartStrategy(normalizer)

	symbol := "ETHUSDT"
	timeframe := "1h"
	
	// 测试降级策略
	result := coldStart.GetFallbackZScore(symbol, timeframe, 75.0)
	if result.IsReady {
		t.Error("降级策略应该返回IsReady=false")
	}
	
	if result.ZScore == 0.0 {
		t.Error("降级策略应该返回非零Z分数")
	}
	
	// 测试主流币种调整
	btcResult := coldStart.GetFallbackZScore("BTCUSDT", timeframe, 75.0)
	altResult := coldStart.GetFallbackZScore("DOGEUSDT", timeframe, 75.0)
	
	// 主流币种通常应该有稍高的分数调整
	if btcResult.ZScore == altResult.ZScore {
		t.Log("注意: 主流币种和山寨币种的Z分数调整相同，可能需要检查调整逻辑")
	}

	t.Logf("冷启动测试通过: BTC Z分数=%.3f, 山寨币 Z分数=%.3f", btcResult.ZScore, altResult.ZScore)
}

// TestStrengthNormalizerDatabase 数据库操作测试
func TestStrengthNormalizerDatabase(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	normalizer := NewStrengthNormalizer(db)

	symbol := "ADAUSDT"
	timeframe := "15m"

	// 添加测试数据
	testRecords := []ZoneStrengthRecord{
		{Symbol: symbol, Timeframe: timeframe, RawScore: 45.0, ZoneType: "supply", PatternType: "drop_base_drop"},
		{Symbol: symbol, Timeframe: timeframe, RawScore: 55.0, ZoneType: "demand", PatternType: "rally_base_rally"},
		{Symbol: symbol, Timeframe: timeframe, RawScore: 65.0, ZoneType: "supply", PatternType: "fresh_supply"},
	}

	for _, record := range testRecords {
		if err := normalizer.persistZoneStrength(&record); err != nil {
			t.Errorf("保存测试记录失败: %v", err)
		}
	}

	// 测试加载历史数据
	scores, err := normalizer.LoadHistoryScores(symbol, timeframe, 10)
	if err != nil {
		t.Fatalf("加载历史数据失败: %v", err)
	}

	if len(scores) != 3 {
		t.Errorf("预期加载3条记录，实际加载%d条", len(scores))
	}

	// 验证数据顺序（应该按时间排序）
	expectedScores := []float64{45.0, 55.0, 65.0}
	for i, score := range scores {
		if math.Abs(score-expectedScores[i]) > 0.01 {
			t.Errorf("历史数据顺序错误: 索引%d预期%.1f，实际%.1f", i, expectedScores[i], score)
		}
	}

	// 测试统计信息
	stats, err := normalizer.GetStatsCount()
	if err != nil {
		t.Fatalf("获取统计信息失败: %v", err)
	}

	key := symbol + "_" + timeframe
	if count, exists := stats[key]; !exists || count != 3 {
		t.Errorf("统计信息错误: 预期%s有3条记录，实际%d条", key, count)
	}

	t.Logf("数据库测试通过: 成功保存和加载%d条记录", len(scores))
}

// TestStrengthNormalizerMaintenance 维护机制测试
func TestStrengthNormalizerMaintenance(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	normalizer := NewStrengthNormalizer(db)
	maintenance := NewMaintenanceManager(normalizer)

	symbol := "BNBUSDT"
	timeframe := "30m"

	// 添加一些数据
	for i := 0; i < 10; i++ {
		score := 50.0 + float64(i)*3.0
		normalizer.AddScore(symbol, timeframe, score, &ZoneStrengthRecord{
			Symbol:      symbol,
			Timeframe:   timeframe,
			RawScore:    score,
			ZoneType:    "supply",
			PatternType: "fresh_supply",
		})
	}

	// 测试数据完整性检查
	if err := maintenance.checkMemoryDatabaseConsistency(); err != nil {
		t.Errorf("内存-数据库一致性检查失败: %v", err)
	}

	// 测试统计数据验证
	if err := maintenance.validateStatisticsData(); err != nil {
		t.Errorf("统计数据验证失败: %v", err)
	}

	// 测试滑动窗口健康检查
	if err := maintenance.checkSlidingWindowHealth(); err != nil {
		t.Errorf("滑动窗口健康检查失败: %v", err)
	}

	// 测试清理操作
	maintenance.performDataCleanup()

	// 测试内存优化
	maintenance.performMemoryOptimization()

	// 测试健康报告
	healthReport := maintenance.GetHealthReport()
	if healthReport["loaded_stats"].(int) == 0 {
		t.Error("健康报告显示没有加载统计状态")
	}

	if healthReport["database_available"].(bool) != true {
		t.Error("健康报告显示数据库不可用")
	}

	t.Logf("维护机制测试通过: 加载了%d个统计状态", healthReport["loaded_stats"].(int))
}

// TestStrengthNormalizerPerformance 性能测试
func TestStrengthNormalizerPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过性能测试")
	}

	db := setupTestDB(t)
	defer db.Close()

	normalizer := NewStrengthNormalizer(db)

	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "ADAUSDT", "XRPUSDT"}
	timeframes := []string{"5m", "15m", "30m", "1h"}

	startTime := time.Now()

	// 并发测试多个币种和时间框架
	totalOperations := 0
	for _, symbol := range symbols {
		for _, timeframe := range timeframes {
			// 添加数据到达最小样本要求
			for i := 0; i < MinSampleSize+10; i++ {
				score := 30.0 + float64(i%50)*1.5 // 30-104.5的分数分布
				normalizer.AddScore(symbol, timeframe, score, &ZoneStrengthRecord{
					Symbol:       symbol,
					Timeframe:    timeframe,
					RawScore:     score,
					ZoneType:     "supply",
					PatternType:  "drop_base_drop",
					TouchCount:   i % 5,
					VolumeRatio:  1.0 + float64(i%10)*0.1,
					WidthPercent: 1.0 + float64(i%8)*0.5,
				})
				totalOperations++
			}

			// 测试Z分数计算性能
			for i := 0; i < 100; i++ {
				testScore := 50.0 + float64(i%40)*1.0
				result := normalizer.GetZScore(symbol, timeframe, testScore)
				if !result.IsReady {
					t.Errorf("预期%s_%s的Z分数计算就绪，但IsReady=false", symbol, timeframe)
				}
				totalOperations++
			}
		}
	}

	elapsed := time.Since(startTime)
	opsPerSecond := float64(totalOperations) / elapsed.Seconds()

	t.Logf("性能测试完成:")
	t.Logf("  总操作数: %d", totalOperations)
	t.Logf("  耗时: %v", elapsed)
	t.Logf("  每秒操作数: %.1f", opsPerSecond)

	// 性能基准: 应该达到至少1000操作/秒
	if opsPerSecond < 1000 {
		t.Logf("警告: 性能可能不足，每秒操作数: %.1f < 1000", opsPerSecond)
	}

	// 验证窗口利用率
	utilization := normalizer.GetWindowUtilization()
	if len(utilization) != len(symbols)*len(timeframes) {
		t.Errorf("窗口利用率记录数量错误: 预期%d，实际%d", len(symbols)*len(timeframes), len(utilization))
	}

	avgUtilization := 0.0
	for _, usage := range utilization {
		avgUtilization += usage
	}
	avgUtilization /= float64(len(utilization))

	t.Logf("平均窗口利用率: %.2f", avgUtilization)
}

// TestStrengthNormalizerIntegration 集成测试
func TestStrengthNormalizerIntegration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 初始化全局标准化器
	InitGlobalStrengthNormalizer(db)
	defer func() {
		// 重置全局变量以避免测试间干扰
		globalStrengthNormalizer = nil
		normalizerOnce = sync.Once{}
	}()

	// 测试全局访问
	normalizer := GetGlobalStrengthNormalizer()
	if normalizer == nil {
		t.Fatal("获取全局强度标准化器失败")
	}

	// 初始化全局维护管理器
	InitGlobalMaintenance()
	maintenance := GetGlobalMaintenance()
	if maintenance == nil {
		t.Fatal("获取全局维护管理器失败")
	}

	// 测试供需区集成
	analyzer := NewSupplyDemandAnalyzer()
	if analyzer == nil {
		t.Fatal("创建供需区分析器失败")
	}

	// 创建模拟K线数据
	klines := make([]Kline, 100)
	basePrice := 50000.0
	for i := range klines {
		price := basePrice + float64(i)*10 + float64(i%10)*5
		klines[i] = Kline{
			OpenTime: time.Now().UnixMilli() - int64((100-i)*5*60*1000), // 5分钟间隔
			Open:     price - 5,
			High:     price + 10,
			Low:      price - 10,
			Close:    price,
			Volume:   1000 + float64(i%20)*100,
		}
	}

	// 分析供需区
	sdData := analyzer.Analyze(klines)
	if sdData == nil {
		t.Fatal("供需区分析失败")
	}

	if len(sdData.ActiveZones) == 0 {
		t.Log("警告: 未发现活跃供需区，可能是测试数据问题")
	} else {
		t.Logf("发现%d个活跃供需区", len(sdData.ActiveZones))
		
		// 验证Z分数标准化
		standardizedCount := 0
		for _, zone := range sdData.ActiveZones {
			if zone.StrengthZReady {
				standardizedCount++
				if math.IsNaN(zone.StrengthZ) || math.IsInf(zone.StrengthZ, 0) {
					t.Errorf("供需区Z分数异常: %f", zone.StrengthZ)
				}
				if zone.StrengthZ > MaxZScore || zone.StrengthZ < -MaxZScore {
					t.Errorf("供需区Z分数超出范围: %f", zone.StrengthZ)
				}
			}
		}

		t.Logf("已标准化供需区数量: %d/%d", standardizedCount, len(sdData.ActiveZones))
	}

	t.Logf("集成测试通过: 系统各组件协调工作正常")
}

// TestStrengthNormalizerEdgeCases 边界情况测试
func TestStrengthNormalizerEdgeCases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	normalizer := NewStrengthNormalizer(db)

	// 测试1: 无效时间框架
	result := normalizer.GetZScore("BTCUSDT", "invalid", 50.0)
	if result.IsReady {
		t.Error("无效时间框架应该返回IsReady=false")
	}

	// 测试2: 相同分数的标准差为0情况
	symbol := "TESTCOIN"
	timeframe := "5m"
	sameScore := 75.0

	for i := 0; i < MinSampleSize; i++ {
		normalizer.AddScore(symbol, timeframe, sameScore, &ZoneStrengthRecord{
			Symbol:    symbol,
			Timeframe: timeframe,
			RawScore:  sameScore,
			ZoneType:  "supply",
		})
	}

	// 测试相同分数
	result = normalizer.GetZScore(symbol, timeframe, sameScore)
	if !result.IsReady {
		t.Error("相同分数情况应该返回IsReady=true")
	}
	if result.ZScore != 0.0 {
		t.Errorf("相同分数的Z分数应该为0，实际为%f", result.ZScore)
	}

	// 测试高于平均分的分数
	result = normalizer.GetZScore(symbol, timeframe, sameScore+10)
	if result.ZScore != MaxZScore {
		t.Errorf("高于平均分的Z分数应该为MaxZScore，实际为%f", result.ZScore)
	}

	// 测试低于平均分的分数
	result = normalizer.GetZScore(symbol, timeframe, sameScore-10)
	if result.ZScore != -MaxZScore {
		t.Errorf("低于平均分的Z分数应该为-MaxZScore，实际为%f", result.ZScore)
	}

	// 测试3: 极端Z分数截断
	symbol2 := "EXTREME"
	scores := []float64{10, 15, 20, 25, 30, 35, 40, 45, 50, 55} // 方差较小的数据
	for i := 0; i < MinSampleSize; i++ {
		score := scores[i%len(scores)]
		normalizer.AddScore(symbol2, timeframe, score, &ZoneStrengthRecord{
			Symbol:    symbol2,
			Timeframe: timeframe,
			RawScore:  score,
		})
	}

	// 测试极端高分
	extremeResult := normalizer.GetZScore(symbol2, timeframe, 1000.0)
	if extremeResult.ZScore != MaxZScore {
		t.Errorf("极端高分的Z分数应该被截断到MaxZScore，实际为%f", extremeResult.ZScore)
	}

	// 测试极端低分
	extremeResult = normalizer.GetZScore(symbol2, timeframe, -1000.0)
	if extremeResult.ZScore != -MaxZScore {
		t.Errorf("极端低分的Z分数应该被截断到-MaxZScore，实际为%f", extremeResult.ZScore)
	}

	t.Log("边界情况测试通过")
}

// TestMain 测试主函数
func TestMain(m *testing.M) {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	fmt.Println("开始Z-Score标准化系统测试...")
	
	code := m.Run()
	
	fmt.Println("Z-Score标准化系统测试完成")
	os.Exit(code)
}