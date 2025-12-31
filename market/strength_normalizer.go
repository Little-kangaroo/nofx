package market

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"
)

const (
	// WindowSize 滑动窗口大小 - 保存最近200个供需区强度分数
	WindowSize = 200
	// MinSampleSize 计算Z-Score所需的最小样本量 - 修正：降低阈值加速进入正常模式
	MinSampleSize = 15
	// MaxZScore Z-Score的最大绝对值，防止极端值
	MaxZScore = 3.0
	// HistoryRetentionDays 历史数据保留天数
	HistoryRetentionDays = 30

	// 批量写入配置
	// DefaultBatchSize 默认批量写入大小
	DefaultBatchSize = 10
	// DefaultBatchTimeout 默认批量写入超时时间
	DefaultBatchTimeout = 5 * time.Second
	// MaxBatchSize 最大批量写入大小，防止内存过度消耗
	MaxBatchSize = 100
)

// StrengthNormalizer 强度标准化管理器
// 负责管理所有币种+时间框架的历史强度数据，提供标准化的Z-Score计算
type StrengthNormalizer struct {
	db          *sql.DB                   // 数据库连接
	stats       map[string]*StrengthStats // symbol+timeframe -> 统计状态
	mu          sync.RWMutex              // 读写锁保护并发访问
	batchWriter *BatchWriter              // 批量写入管理器
}

// BatchWriter 批量写入管理器
// 解决数据库并发锁定问题，收集多个写入请求后批量提交
type BatchWriter struct {
	records   []*ZoneStrengthRecord // 待写入的记录缓存
	mutex     sync.Mutex            // 保护records的并发访问
	batchSize int                   // 批量大小阈值
	timeout   time.Duration         // 超时阈值
	lastFlush time.Time             // 上次刷新时间
	db        *sql.DB               // 数据库连接
	stopCh    chan struct{}         // 停止信号
	running   bool                  // 运行状态
}

// StrengthStats 单个币种+时间框架的强度统计状态
// 维护内存中的滑动窗口和统计特征
type StrengthStats struct {
	sync.RWMutex            // 内部读写锁
	Symbol        string    // 币种 (如 BTCUSDT)
	Timeframe     string    // 时间框架 (如 5m, 15m, 30m, 1h, 4h)
	HistoryScores []float64 // 滑动窗口：最近200个强度分数
	Mean          float64   // 当前均值
	StdDev        float64   // 当前标准差
	LastUpdated   time.Time // 最后更新时间
	SampleCount   int       // 样本数量（便于调试）

	// Width宽度统计（新增）
	HistoryWidths []float64 // 滑动窗口：最近200个宽度数据（基础点或ATR倍数）
	WidthMean     float64   // 宽度均值
	WidthStdDev   float64   // 宽度标准差
}

// ZScoreResult Z-Score计算结果
type ZScoreResult struct {
	ZScore      float64 `json:"z_score"`      // 标准化的Z分数
	IsReady     bool    `json:"is_ready"`     // 数据是否充足
	SampleCount int     `json:"sample_count"` // 当前样本数量
	Mean        float64 `json:"mean"`         // 当前均值（调试用）
	StdDev      float64 `json:"std_dev"`      // 当前标准差（调试用）
}

// WidthZScoreResult Width宽度Z-Score计算结果（新增）
type WidthZScoreResult struct {
	WidthZScore float64 `json:"width_z_score"` // 宽度标准化的Z分数
	IsReady     bool    `json:"is_ready"`      // 数据是否充足
	SampleCount int     `json:"sample_count"`  // 当前样本数量
	WidthMean   float64 `json:"width_mean"`    // 宽度均值（调试用）
	WidthStdDev float64 `json:"width_std_dev"` // 宽度标准差（调试用）
}

// ZoneStrengthRecord 供需区强度记录（用于数据库存储）
type ZoneStrengthRecord struct {
	ID           int64     `json:"id"`
	Symbol       string    `json:"symbol"`
	Timeframe    string    `json:"timeframe"`
	RawScore     float64   `json:"raw_score"`
	ZoneType     string    `json:"zone_type"`     // supply/demand
	PatternType  string    `json:"pattern_type"`  // 模式类型
	TouchCount   int       `json:"touch_count"`   // 触碰次数
	VolumeRatio  float64   `json:"volume_ratio"`  // 成交量比率
	WidthPercent float64   `json:"width_percent"` // 宽度百分比
	CreatedAt    time.Time `json:"created_at"`
}

// globalStrengthNormalizer 全局单例实例
var (
	globalStrengthNormalizer *StrengthNormalizer
	normalizerOnce           sync.Once
)

// NewStrengthNormalizer 创建强度标准化管理器
func NewStrengthNormalizer(db *sql.DB) *StrengthNormalizer {
	// 创建批量写入器
	batchWriter := &BatchWriter{
		records:   make([]*ZoneStrengthRecord, 0, DefaultBatchSize),
		batchSize: DefaultBatchSize,
		timeout:   DefaultBatchTimeout,
		lastFlush: time.Now(),
		db:        db,
		stopCh:    make(chan struct{}),
		running:   false,
	}

	normalizer := &StrengthNormalizer{
		db:          db,
		stats:       make(map[string]*StrengthStats),
		batchWriter: batchWriter,
	}

	// 启动批量写入器的后台goroutine
	normalizer.startBatchWriter()

	log.Printf("🎯 强度标准化器已创建（批量写入模式），准备预热历史数据...")

	return normalizer
}

// InitGlobalStrengthNormalizer 初始化全局强度标准化器（单例模式）
func InitGlobalStrengthNormalizer(db *sql.DB) {
	normalizerOnce.Do(func() {
		globalStrengthNormalizer = NewStrengthNormalizer(db)
		// 异步预热历史数据，避免阻塞启动
		go globalStrengthNormalizer.Warmup()
	})
}

// GetGlobalStrengthNormalizer 获取全局强度标准化器实例
func GetGlobalStrengthNormalizer() *StrengthNormalizer {
	if globalStrengthNormalizer == nil {
		log.Printf("⚠️ 强度标准化器未初始化")
		return nil
	}
	return globalStrengthNormalizer
}

// getStatsKey 生成统计状态的键值
func getStatsKey(symbol, timeframe string) string {
	return symbol + "_" + timeframe
}

// GetStats 获取指定币种+时间框架的统计状态（调试用）
func (sn *StrengthNormalizer) GetStats(symbol, timeframe string) *StrengthStats {
	key := getStatsKey(symbol, timeframe)

	sn.mu.RLock()
	stats := sn.stats[key]
	sn.mu.RUnlock()

	return stats
}

// GetAllStatsInfo 获取所有统计状态的概览信息（调试用）
func (sn *StrengthNormalizer) GetAllStatsInfo() map[string]int {
	sn.mu.RLock()
	defer sn.mu.RUnlock()

	info := make(map[string]int)
	for key, stats := range sn.stats {
		stats.RLock()
		info[key] = len(stats.HistoryScores)
		stats.RUnlock()
	}

	return info
}

// validateTimeframe 验证时间框架是否有效
func validateTimeframe(timeframe string) bool {
	validTimeframes := map[string]bool{
		"5m": true, "15m": true, "30m": true, "1h": true, "4h": true,
	}
	return validTimeframes[timeframe]
}

// formatSymbol 标准化币种格式
func formatSymbol(symbol string) string {
	// 确保symbol格式一致性（大写，USDT结尾等）
	// 这里可以根据实际需要添加格式化逻辑
	return symbol
}

// ===== 批量写入器方法 =====

// startBatchWriter 启动批量写入器的后台goroutine
func (sn *StrengthNormalizer) startBatchWriter() {
	sn.batchWriter.mutex.Lock()
	if sn.batchWriter.running {
		sn.batchWriter.mutex.Unlock()
		return // 已经启动
	}
	sn.batchWriter.running = true
	sn.batchWriter.mutex.Unlock()

	go sn.batchWriter.backgroundFlush()
	log.Printf("📦 [批量写入器] 后台刷新服务已启动，批量大小: %d, 超时: %v",
		sn.batchWriter.batchSize, sn.batchWriter.timeout)
}

// AddToBatch 添加记录到批量写入缓存
func (bw *BatchWriter) AddToBatch(record *ZoneStrengthRecord) {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	// 防止缓存过大
	if len(bw.records) >= MaxBatchSize {
		log.Printf("⚠️ [批量写入器] 缓存已满，强制刷新")
		go bw.flushBatch()          // 异步刷新，避免阻塞
		bw.records = bw.records[:0] // 清空缓存
		bw.lastFlush = time.Now()
	}

	bw.records = append(bw.records, record)

	// 检查是否需要立即刷新
	if len(bw.records) >= bw.batchSize {
		//log.Printf("📦 [批量写入器] 达到批量大小 (%d)，触发刷新", len(bw.records))
		go bw.flushBatch()          // 异步刷新
		bw.records = bw.records[:0] // 清空缓存
		bw.lastFlush = time.Now()
	}
}

// backgroundFlush 后台定时刷新goroutine
func (bw *BatchWriter) backgroundFlush() {
	ticker := time.NewTicker(1 * time.Second) // 每秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-bw.stopCh:
			// 收到停止信号，最后刷新一次
			bw.flushBatch()
			log.Printf("📦 [批量写入器] 后台刷新服务已停止")
			return
		case <-ticker.C:
			bw.mutex.Lock()
			shouldFlush := len(bw.records) > 0 && time.Since(bw.lastFlush) > bw.timeout
			recordCount := len(bw.records)
			bw.mutex.Unlock()

			if shouldFlush {
				log.Printf("📦 [批量写入器] 超时��发刷新，缓存记录: %d", recordCount)
				bw.flushBatch()
			}
		}
	}
}

// flushBatch 执行批量刷新到数据库
func (bw *BatchWriter) flushBatch() {
	bw.mutex.Lock()
	if len(bw.records) == 0 {
		bw.mutex.Unlock()
		return // 没有记录需要刷新
	}

	// 复制记录到局部变量，快速释放锁
	recordsToFlush := make([]*ZoneStrengthRecord, len(bw.records))
	copy(recordsToFlush, bw.records)
	bw.records = bw.records[:0] // 清空缓存
	bw.lastFlush = time.Now()
	bw.mutex.Unlock()

	// 执行批量写入
	startTime := time.Now()
	err := bw.persistBatch(recordsToFlush)
	duration := time.Since(startTime)

	if err != nil {
		log.Printf("❌ [批量写入器] 批量写入失败: %v, 记录数: %d, 耗时: %v",
			err, len(recordsToFlush), duration)

		// 失败时的降级策略：逐个尝试写入
		log.Printf("🔄 [批量写入器] 启动降级策略，逐个重试写入...")
		successCount := 0
		for _, record := range recordsToFlush {
			if err := bw.persistSingle(record); err != nil {
				log.Printf("❌ [降级策略] 单个记录写入失败 [%s_%s]: %v",
					record.Symbol, record.Timeframe, err)
			} else {
				successCount++
			}
		}
		log.Printf("🔄 [降级策略完成] 成功: %d/%d", successCount, len(recordsToFlush))
	} else {
		log.Printf("✅ [批量写入器] 批量写入成功: %d条记录, 耗时: %v",
			len(recordsToFlush), duration)
	}
}

// persistBatch 批量写入多条记录（使用事务）
func (bw *BatchWriter) persistBatch(records []*ZoneStrengthRecord) error {
	if bw.db == nil {
		return fmt.Errorf("数据库连接为空")
	}

	// 开始事务
	tx, err := bw.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback() // 确保在错误时回滚

	// 准备批量插入语句
	query := `
	INSERT INTO zone_history (
		symbol, timeframe, raw_score, zone_type, 
		pattern_type, touch_count, volume_ratio, width_percent, 
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	// 批量执行插入
	for _, record := range records {
		_, err := stmt.Exec(
			record.Symbol,
			record.Timeframe,
			record.RawScore,
			record.ZoneType,
			record.PatternType,
			record.TouchCount,
			record.VolumeRatio,
			record.WidthPercent,
		)
		if err != nil {
			return fmt.Errorf("执行插入失败 [%s_%s]: %w",
				record.Symbol, record.Timeframe, err)
		}
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// persistSingle 单条记录写入（降级策略）
func (bw *BatchWriter) persistSingle(record *ZoneStrengthRecord) error {
	if bw.db == nil {
		return fmt.Errorf("数据库连接为空")
	}

	query := `
	INSERT INTO zone_history (
		symbol, timeframe, raw_score, zone_type, 
		pattern_type, touch_count, volume_ratio, width_percent, 
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	_, err := bw.db.Exec(query,
		record.Symbol,
		record.Timeframe,
		record.RawScore,
		record.ZoneType,
		record.PatternType,
		record.TouchCount,
		record.VolumeRatio,
		record.WidthPercent,
	)

	if err != nil {
		return fmt.Errorf("执行数据库插入失败: %w", err)
	}

	return nil
}

// StopBatchWriter 停止批量写入器
func (sn *StrengthNormalizer) StopBatchWriter() {
	if sn.batchWriter != nil {
		sn.batchWriter.mutex.Lock()
		if sn.batchWriter.running {
			close(sn.batchWriter.stopCh)
			sn.batchWriter.running = false
		}
		sn.batchWriter.mutex.Unlock()
	}
}

// ===== 数据库操作方法 =====

// SaveZoneStrength 保存供需区强度到数据库（使用批量写入策略）
func (sn *StrengthNormalizer) SaveZoneStrength(record *ZoneStrengthRecord) {
	// 使用批量写入器，解决数据库锁定问题
	if sn.batchWriter != nil {
		sn.batchWriter.AddToBatch(record)
		//log.Printf("📦 [批量写入] 添加记录到缓存: %s_%s (强度: %.2f)",
		//	record.Symbol, record.Timeframe, record.RawScore)
	} else {
		// 降级为直接写入（兼容性）
		log.Printf("⚠️ [批量写入器] 未初始化，使用直接写入模式")
		go func() {
			if err := sn.persistZoneStrengthDirect(record); err != nil {
				log.Printf("❌ 保存供需区强度失败 [%s_%s]: %v", record.Symbol, record.Timeframe, err)
			}
		}()
	}
}

// persistZoneStrengthDirect 直接保存供需区强度到数据库（降级策略）
func (sn *StrengthNormalizer) persistZoneStrengthDirect(record *ZoneStrengthRecord) error {
	if sn.db == nil {
		return fmt.Errorf("数据库连接为空")
	}

	query := `
	INSERT INTO zone_history (
		symbol, timeframe, raw_score, zone_type, 
		pattern_type, touch_count, volume_ratio, width_percent, 
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	_, err := sn.db.Exec(query,
		record.Symbol,
		record.Timeframe,
		record.RawScore,
		record.ZoneType,
		record.PatternType,
		record.TouchCount,
		record.VolumeRatio,
		record.WidthPercent,
	)

	if err != nil {
		return fmt.Errorf("执行数据库插入失败: %w", err)
	}

	return nil
}

// LoadHistoryScores 从数据库加载指定币种+时间框架的历史强度分数
func (sn *StrengthNormalizer) LoadHistoryScores(symbol, timeframe string, limit int) ([]float64, error) {
	if sn.db == nil {
		return nil, fmt.Errorf("数据库连接为空")
	}

	if limit <= 0 {
		limit = WindowSize
	}

	query := `
	SELECT raw_score 
	FROM zone_history 
	WHERE symbol = ? AND timeframe = ? 
	ORDER BY created_at DESC 
	LIMIT ?
	`

	rows, err := sn.db.Query(query, symbol, timeframe, limit)
	if err != nil {
		return nil, fmt.Errorf("查询历史强度数据失败: %w", err)
	}
	defer rows.Close()

	var scores []float64
	for rows.Next() {
		var score float64
		if err := rows.Scan(&score); err != nil {
			log.Printf("⚠️ 扫描历史强度数据失败: %v", err)
			continue
		}
		scores = append(scores, score)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %w", err)
	}

	// 反转数组：数据库查询是DESC顺序，但我们需要按时间正序排列
	for i := len(scores)/2 - 1; i >= 0; i-- {
		opp := len(scores) - 1 - i
		scores[i], scores[opp] = scores[opp], scores[i]
	}

	return scores, nil
}

// GetDistinctSymbolTimeframes 获取所有存在历史数据的symbol+timeframe组合
func (sn *StrengthNormalizer) GetDistinctSymbolTimeframes() ([][2]string, error) {
	if sn.db == nil {
		return nil, fmt.Errorf("数据库连接为空")
	}

	query := `
	SELECT DISTINCT symbol, timeframe 
	FROM zone_history 
	WHERE created_at > datetime('now', '-30 days')
	ORDER BY symbol, timeframe
	`

	rows, err := sn.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("查询symbol+timeframe组合失败: %w", err)
	}
	defer rows.Close()

	var combinations [][2]string
	for rows.Next() {
		var symbol, timeframe string
		if err := rows.Scan(&symbol, &timeframe); err != nil {
			log.Printf("⚠️ 扫描symbol+timeframe失败: %v", err)
			continue
		}
		combinations = append(combinations, [2]string{symbol, timeframe})
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %w", err)
	}

	return combinations, nil
}

// CleanupOldRecords 清理过期的历史记录（建议作为定时任务运行）
func (sn *StrengthNormalizer) CleanupOldRecords() error {
	if sn.db == nil {
		return fmt.Errorf("数据库连接为空")
	}

	query := `
	DELETE FROM zone_history 
	WHERE created_at < datetime('now', '-30 days')
	`

	result, err := sn.db.Exec(query)
	if err != nil {
		return fmt.Errorf("清理过期记录失败: %w", err)
	}

	deletedRows, err := result.RowsAffected()
	if err != nil {
		log.Printf("⚠️ 获取删除行数失败: %v", err)
	} else if deletedRows > 0 {
		log.Printf("🧹 清理了 %d 条过期的供需区强度记录", deletedRows)
	}

	return nil
}

// GetStatsCount 获取数据库中的统计信息（调试用）
func (sn *StrengthNormalizer) GetStatsCount() (map[string]int, error) {
	if sn.db == nil {
		return nil, fmt.Errorf("数据库连接为空")
	}

	query := `
	SELECT 
		symbol || '_' || timeframe as key,
		COUNT(*) as count
	FROM zone_history 
	WHERE created_at > datetime('now', '-30 days')
	GROUP BY symbol, timeframe
	ORDER BY count DESC
	`

	rows, err := sn.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("查询统计信息失败: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			log.Printf("⚠️ 扫描统计信息失败: %v", err)
			continue
		}
		stats[key] = count
	}

	return stats, nil
}

// ===== 内存滑动窗口逻辑 =====

// AddScore 添加新的强度分数到滑动窗口
// 这是核心方法：更新内存窗口，重新计算统计特征，异步持久化
func (sn *StrengthNormalizer) AddScore(symbol, timeframe string, rawScore float64, zoneInfo *ZoneStrengthRecord) {
	// 参数验证
	symbol = formatSymbol(symbol)
	if !validateTimeframe(timeframe) {
		log.Printf("⚠️ 无效的时间框架: %s", timeframe)
		return
	}

	key := getStatsKey(symbol, timeframe)

	// 获取或创建统计状态
	sn.mu.Lock()
	stats := sn.stats[key]
	if stats == nil {
		stats = &StrengthStats{
			Symbol:        symbol,
			Timeframe:     timeframe,
			HistoryScores: make([]float64, 0, WindowSize),
			HistoryWidths: make([]float64, 0, WindowSize), // 新增
			LastUpdated:   time.Now(),
		}
		sn.stats[key] = stats
	}
	sn.mu.Unlock()

	// 更新滑动窗口
	stats.Lock()
	stats.addScore(rawScore)
	stats.Unlock()

	// 异步持久化
	if zoneInfo != nil {
		zoneInfo.Symbol = symbol
		zoneInfo.Timeframe = timeframe
		zoneInfo.RawScore = rawScore
		sn.SaveZoneStrength(zoneInfo)
	}

	// 调试日志
	if len(stats.HistoryScores)%10 == 0 {
		//log.Printf("📊 [%s_%s] 滑动窗口更新: %d个样本, 均值=%.2f, 标准差=%.2f",
		//	symbol, timeframe, len(stats.HistoryScores), stats.Mean, stats.StdDev)
	}
}

// AddScoreWithWidth 添加强度分数和宽度数据到滑动窗口（新增）
// 扩展版本的AddScore，同时支持Width宽度的Z-Score统计
func (sn *StrengthNormalizer) AddScoreWithWidth(symbol, timeframe string, rawScore, widthATR float64, zoneInfo *ZoneStrengthRecord) {
	// 参数验证
	symbol = formatSymbol(symbol)
	if !validateTimeframe(timeframe) {
		log.Printf("⚠️ 无效的时间框架: %s", timeframe)
		return
	}

	key := getStatsKey(symbol, timeframe)

	// 获取或创建统计状态
	sn.mu.Lock()
	stats := sn.stats[key]
	if stats == nil {
		stats = &StrengthStats{
			Symbol:        symbol,
			Timeframe:     timeframe,
			HistoryScores: make([]float64, 0, WindowSize),
			HistoryWidths: make([]float64, 0, WindowSize),
			LastUpdated:   time.Now(),
		}
		sn.stats[key] = stats
	}
	sn.mu.Unlock()

	// 更新滑动窗口（包含强度和宽度）
	stats.Lock()
	stats.addScoreWithWidth(rawScore, widthATR)
	stats.Unlock()

	// 异步持久化
	if zoneInfo != nil {
		zoneInfo.Symbol = symbol
		zoneInfo.Timeframe = timeframe
		zoneInfo.RawScore = rawScore
		sn.SaveZoneStrength(zoneInfo)
	}

	// 调试日志
	if len(stats.HistoryScores)%10 == 0 {
		//log.Printf("📊 [%s_%s] 滑动窗口更新: %d个样本, 强度均值=%.2f±%.2f, 宽度均值=%.3f±%.3f",
		//	symbol, timeframe, len(stats.HistoryScores), stats.Mean, stats.StdDev, stats.WidthMean, stats.WidthStdDev)
	}
}

// addScore 向单个统计状态添加分数（内部方法，已加锁）
func (ss *StrengthStats) addScore(score float64) {
	// 添加新分数
	ss.HistoryScores = append(ss.HistoryScores, score)

	// 维持窗口大小（FIFO队列）
	if len(ss.HistoryScores) > WindowSize {
		// 移除最老的分数
		ss.HistoryScores = ss.HistoryScores[1:]
	}

	// 更新统计特征
	ss.updateStatistics()
	ss.LastUpdated = time.Now()
	ss.SampleCount = len(ss.HistoryScores)
}

// addScoreWithWidth 向单个统计状态添加分数和宽度（内部方法，已加锁）
func (ss *StrengthStats) addScoreWithWidth(score, widthATR float64) {
	// 添加新分数
	ss.HistoryScores = append(ss.HistoryScores, score)
	ss.HistoryWidths = append(ss.HistoryWidths, widthATR)

	// 维持窗口大小（FIFO队列）
	if len(ss.HistoryScores) > WindowSize {
		// 移除最老的分数和宽度
		ss.HistoryScores = ss.HistoryScores[1:]
		ss.HistoryWidths = ss.HistoryWidths[1:]
	}

	// 更新统计特征
	ss.updateStatistics()
	ss.updateWidthStatistics() // 新增宽度统计更新
	ss.LastUpdated = time.Now()
	ss.SampleCount = len(ss.HistoryScores)
}

// updateStatistics 重新计算均值和标准差（内部方法，已加锁）
// 修复：使用时间窗口分离，避免新旧数据扭曲统计特征
func (ss *StrengthStats) updateStatistics() {
	count := len(ss.HistoryScores)
	if count == 0 {
		ss.Mean = 0
		ss.StdDev = 0
		return
	}

	// 修复：分离计算策略，避免时间窗口扭曲
	// 如果样本充足，使用最近的样本计算更稳定的统计特征
	var effectiveScores []float64
	if count >= 50 {
		// 充足样本：只使用最近75%的数据计算统计特征
		// 这样避免早期冷启动数据影响当前统计特征
		recentCount := int(float64(count) * 0.75)
		if recentCount < 30 {
			recentCount = 30 // 最少保留30个样本
		}
		startIdx := count - recentCount
		effectiveScores = ss.HistoryScores[startIdx:]
	} else {
		// 样本不足：使用全部数据
		effectiveScores = ss.HistoryScores
	}

	effectiveCount := len(effectiveScores)

	// 计算均值（基于有效样本）
	sum := 0.0
	for _, score := range effectiveScores {
		sum += score
	}
	ss.Mean = sum / float64(effectiveCount)

	// 计算标准差（基于有效样本）
	if effectiveCount == 1 {
		ss.StdDev = 0
		return
	}

	varianceSum := 0.0
	for _, score := range effectiveScores {
		diff := score - ss.Mean
		varianceSum += diff * diff
	}

	// 使用贝塞尔校正的标准差计算（样本标准差）
	// 除以 (n-1) 而不是 n，提供更准确的总体标准差估计
	variance := varianceSum / float64(effectiveCount-1)
	ss.StdDev = math.Sqrt(variance)

	// 添加最小标准差保护，避��过度敏感的Z-Score
	minStdDev := 5.0 // 假设强度分数范围0-100，最小标准差为5
	if ss.StdDev < minStdDev {
		ss.StdDev = minStdDev
	}
}

// updateWidthStatistics 重新计算宽度的均值和标准差（内部方法，已加锁）
// 与updateStatistics方法类似，但专门处理宽度数据
func (ss *StrengthStats) updateWidthStatistics() {
	count := len(ss.HistoryWidths)
	if count == 0 {
		ss.WidthMean = 0
		ss.WidthStdDev = 0
		return
	}

	// 宽度统计也使用相同的时间窗口分离策略
	var effectiveWidths []float64
	if count >= 50 {
		// 充足样本：只使用最近75%的数据计算统计特征
		recentCount := int(float64(count) * 0.75)
		if recentCount < 30 {
			recentCount = 30 // 最少保留30个样本
		}
		startIdx := count - recentCount
		effectiveWidths = ss.HistoryWidths[startIdx:]
	} else {
		// 样本不足：使用全部数据
		effectiveWidths = ss.HistoryWidths
	}

	effectiveCount := len(effectiveWidths)

	// 计算宽度均值
	sum := 0.0
	for _, width := range effectiveWidths {
		sum += width
	}
	ss.WidthMean = sum / float64(effectiveCount)

	// 计算宽度标准差
	if effectiveCount == 1 {
		ss.WidthStdDev = 0
		return
	}

	varianceSum := 0.0
	for _, width := range effectiveWidths {
		diff := width - ss.WidthMean
		varianceSum += diff * diff
	}

	// 使用贝塞尔校正的标准差计算
	variance := varianceSum / float64(effectiveCount-1)
	ss.WidthStdDev = math.Sqrt(variance)

	// 宽度的最小标准差保护（基于ATR倍数，通常范围0.1-3.0）
	minWidthStdDev := 0.05 // 最小标准差为0.05个ATR倍数
	if ss.WidthStdDev < minWidthStdDev {
		ss.WidthStdDev = minWidthStdDev
	}
}

// loadHistoryScoresIntoMemory ��数据库中的历史分数加载到内存滑动窗口
func (sn *StrengthNormalizer) loadHistoryScoresIntoMemory(symbol, timeframe string) error {
	scores, err := sn.LoadHistoryScores(symbol, timeframe, WindowSize)
	if err != nil {
		return fmt.Errorf("加载历史分数失败: %w", err)
	}

	if len(scores) == 0 {
		return nil // 没有历史数据，正常情况
	}

	key := getStatsKey(symbol, timeframe)

	// 创建统计状态
	stats := &StrengthStats{
		Symbol:        symbol,
		Timeframe:     timeframe,
		HistoryScores: scores,
		LastUpdated:   time.Now(),
		SampleCount:   len(scores),
	}

	// 计算统计特征
	stats.updateStatistics()

	// 存入内存
	sn.mu.Lock()
	sn.stats[key] = stats
	sn.mu.Unlock()

	log.Printf("✓ [%s_%s] 加载历史数据: %d个样本, 均值=%.2f, 标准差=%.2f",
		symbol, timeframe, len(scores), stats.Mean, stats.StdDev)

	return nil
}

// GetWindowUtilization 获取滑动窗口的使用情况（调试用）
func (sn *StrengthNormalizer) GetWindowUtilization() map[string]float64 {
	sn.mu.RLock()
	defer sn.mu.RUnlock()

	utilization := make(map[string]float64)
	for key, stats := range sn.stats {
		stats.RLock()
		utilization[key] = float64(len(stats.HistoryScores)) / float64(WindowSize)
		stats.RUnlock()
	}

	return utilization
}

// PruneInactiveStats 清理长时间未更新的统计状态（内存优化）
func (sn *StrengthNormalizer) PruneInactiveStats(maxIdleDuration time.Duration) int {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	now := time.Now()
	pruned := 0

	for key, stats := range sn.stats {
		stats.RLock()
		isIdle := now.Sub(stats.LastUpdated) > maxIdleDuration
		stats.RUnlock()

		if isIdle {
			delete(sn.stats, key)
			pruned++
		}
	}

	if pruned > 0 {
		log.Printf("🧹 清理了 %d 个不活跃的统计状态", pruned)
	}

	return pruned
}

// ===== 系统启动预热机制 =====

// Warmup 系统启动时预热历史数据到内存
// 阶段A：启动预热 - 在程序启动main()初始化阶段，在连接WebSocket行情之前
func (sn *StrengthNormalizer) Warmup() error {
	log.Printf("🔥 开始系统启动预热...")

	// 获取所有存在历史数据的symbol+timeframe组合
	combinations, err := sn.GetDistinctSymbolTimeframes()
	if err != nil {
		log.Printf("❌ 获取symbol+timeframe组合失败: %v", err)
		return err
	}

	if len(combinations) == 0 {
		log.Printf("⚠️ 没有找到历史数据，跳过预热")
		return nil
	}

	log.Printf("📊 发现 %d 个symbol+timeframe组合需要预热", len(combinations))

	successCount := 0
	for _, combo := range combinations {
		symbol := combo[0]
		timeframe := combo[1]

		err := sn.loadHistoryScoresIntoMemory(symbol, timeframe)
		if err != nil {
			log.Printf("⚠️ 预热 [%s_%s] 失败: %v", symbol, timeframe, err)
			continue
		}
		successCount++
	}

	log.Printf("✅ 系统预热完成: 成功 %d/%d 个组合", successCount, len(combinations))

	// 输出预热后的内存状态
	stats := sn.GetAllStatsInfo()
	log.Printf("📈 预热后内存状态:")
	for key, count := range stats {
		log.Printf("   %s: %d 个样本", key, count)
	}

	return nil
}

// WarmupAsync 异步预热（非阻塞）
// 用于程序启动时不阻塞主流程的场景
func (sn *StrengthNormalizer) WarmupAsync() {
	go func() {
		if err := sn.Warmup(); err != nil {
			log.Printf("❌ 异步预热失败: %v", err)
		}
	}()
}

// WarmupSpecificSymbol 预热指定币种（按需预热）
// 用于运行时动态添加新币种的场景
func (sn *StrengthNormalizer) WarmupSpecificSymbol(symbol string, timeframes []string) error {
	log.Printf("🎯 开始预热指定币种: %s", symbol)

	successCount := 0
	for _, timeframe := range timeframes {
		err := sn.loadHistoryScoresIntoMemory(symbol, timeframe)
		if err != nil {
			log.Printf("⚠️ 预热 [%s_%s] 失败: %v", symbol, timeframe, err)
			continue
		}
		successCount++
	}

	log.Printf("✅ 币种 %s 预热完成: 成功 %d/%d 个时间框架", symbol, successCount, len(timeframes))
	return nil
}

// IsWarmupComplete 检查系统是否已完成预热
// 用于判断系统是否准备好处理实时数据
func (sn *StrengthNormalizer) IsWarmupComplete() bool {
	sn.mu.RLock()
	defer sn.mu.RUnlock()

	// 如果内存中有任何统计状态，说明预热已经开始
	return len(sn.stats) > 0
}

// GetWarmupStatus 获取预热状态详情
// 用于监控和调试预热过程
func (sn *StrengthNormalizer) GetWarmupStatus() map[string]interface{} {
	sn.mu.RLock()
	defer sn.mu.RUnlock()

	status := map[string]interface{}{
		"is_complete":       len(sn.stats) > 0,
		"loaded_symbols":    len(sn.stats),
		"memory_usage":      sn.GetWindowUtilization(),
		"ready_for_scoring": true,
	}

	// 计算准备状态
	readyCount := 0
	for _, stats := range sn.stats {
		stats.RLock()
		if len(stats.HistoryScores) >= MinSampleSize {
			readyCount++
		}
		stats.RUnlock()
	}

	status["ready_symbols"] = readyCount
	status["ready_percentage"] = 0.0
	if len(sn.stats) > 0 {
		status["ready_percentage"] = float64(readyCount) / float64(len(sn.stats)) * 100.0
	}

	return status
}

// ===== 冷启动降级策略 =====

// ColdStartStrategy 冷启动降级策略
// 在系统刚启动、历史数据不足时的处理策略
type ColdStartStrategy struct {
	normalizer *StrengthNormalizer
}

// NewColdStartStrategy 创建冷启动降级策略
func NewColdStartStrategy(normalizer *StrengthNormalizer) *ColdStartStrategy {
	return &ColdStartStrategy{
		normalizer: normalizer,
	}
}

// GetFallbackZScore 获取降级的Z分数
// 当历史数据不足时，使用经验公式计算近似Z分数
func (css *ColdStartStrategy) GetFallbackZScore(symbol, timeframe string, currentScore float64) *ZScoreResult {
	log.Printf("🔄 [%s_%s] 启用冷启动降级策略", symbol, timeframe)

	// 策略1: 基于经验值的固定Z分数映射
	// 根据行业经验，将0-100的强度分数映射为合理的Z分数
	fallbackZ := css.mapScoreToZScore(currentScore)

	// 策略2: 基于当前市场环境动态调整
	// 如果有足够的市场数据，可以基于当前市场波动性调整Z分数
	marketAdjustment := css.calculateMarketAdjustment(symbol, timeframe)
	adjustedZ := fallbackZ * marketAdjustment

	// 应用标准的Z分数截断
	clippedZ := clipZScore(adjustedZ)

	result := &ZScoreResult{
		ZScore:      clippedZ,
		IsReady:     false, // 标记为不可靠，因为基于经验值
		SampleCount: 0,     // 没有实际样本
		Mean:        50.0,  // 经验均值
		StdDev:      15.0,  // 经验标准差
	}

	log.Printf("🎲 [%s_%s] 降级Z分数: 原始=%.2f → 映射=%.2f → 调整=%.2f → 截断=%.2f",
		symbol, timeframe, currentScore, fallbackZ, adjustedZ, clippedZ)

	return result
}

// mapScoreToZScore 将0-100的强度分数映射为合理的Z分数（修复版 - 非线性映射）
// 目标：让60-75分（良）能拿到+0.5~+1.5的Z-Score，触发AI关注
func (css *ColdStartStrategy) mapScoreToZScore(score float64) float64 {
	switch {
	// 垃圾区：0-30分 -> Z: -3.0 ~ -1.0
	// 这类结构本来就该被过滤，给极低分没问题
	case score <= 30:
		return -3.0 + (score/30.0)*2.0

	// 平庸区：30-55分 -> Z: -1.0 ~ 0.0
	// 稍微给一点点"负面评价"，但不至于判死刑
	case score <= 55:
		return -1.0 + ((score-30)/25.0)*1.0

	// 关键区：55-80分 -> Z: 0.0 ~ +1.5
	// 这里是大多数"可交易结构"的分布区，必须让它们变成正数！
	// 67.5分时 Z = +0.75 (边缘机会)
	// 80分时   Z = +1.5  (A级机会)
	case score <= 80:
		return 0.0 + ((score-55)/25.0)*1.5

	// 极品区：80-100分 -> Z: +1.5 ~ +3.0
	// 这种结构必须让AI眼前一亮
	default: // > 80
		val := 1.5 + ((score-80)/20.0)*1.5
		if val > 3.0 {
			return 3.0
		}
		return val
	}
}

// calculateMarketAdjustment 计算基于市场环境的调整因子
func (css *ColdStartStrategy) calculateMarketAdjustment(symbol, timeframe string) float64 {
	// 基础调整因子
	adjustment := 1.0

	// 根据时间框架调整：小时间框架数据更易变，需要保守一些
	switch timeframe {
	case "5m":
		adjustment *= 0.8 // 5分钟周期保守20%
	case "15m":
		adjustment *= 0.9 // 15分钟周期保守10%
	case "30m", "1h":
		adjustment *= 1.0 // 标准调整
	case "4h":
		adjustment *= 1.1 // 4小时周期可以稍微激进10%
	}

	// 根据币种类型调整
	if css.isMajorCoin(symbol) {
		adjustment *= 1.1 // 主流币种稍微激进
	} else {
		adjustment *= 0.9 // 山寨币保守一些
	}

	return adjustment
}

// isMajorCoin 判断是否为主流币种
func (css *ColdStartStrategy) isMajorCoin(symbol string) bool {
	majorCoins := map[string]bool{
		"BTCUSDT":  true,
		"ETHUSDT":  true,
		"BNBUSDT":  true,
		"ADAUSDT":  true,
		"XRPUSDT":  true,
		"SOLUSDT":  true,
		"DOGEUSDT": true,
	}
	return majorCoins[symbol]
}

// ShouldUseFallback 判断是否应该使用降级策略
func (css *ColdStartStrategy) ShouldUseFallback(symbol, timeframe string, currentSampleCount int) bool {
	// 如果样本数量少于最小要求，使用降级策略
	if currentSampleCount < MinSampleSize {
		return true
	}

	// 如果系统刚启动，预热未完成，也可以考虑使用降级策略
	if !css.normalizer.IsWarmupComplete() {
		return true
	}

	return false
}

// GetZScoreWithFallback 获取Z分数（支持冷启动降级）
// 这是一个增强版本的GetZScore，在数据不足时自动降级
func (css *ColdStartStrategy) GetZScoreWithFallback(symbol, timeframe string, currentScore float64) *ZScoreResult {
	// 首先尝试获取正常的Z分数
	normalResult := css.normalizer.GetZScore(symbol, timeframe, currentScore)

	// 如果正常数据不可用，使用降级策略
	if !normalResult.IsReady || css.ShouldUseFallback(symbol, timeframe, normalResult.SampleCount) {
		return css.GetFallbackZScore(symbol, timeframe, currentScore)
	}

	return normalResult
}

// EnhancedZScoreResult 增强的Z分数结果，包含降级信息
type EnhancedZScoreResult struct {
	*ZScoreResult
	IsFallback     bool   `json:"is_fallback"`     // 是否使用了降级策略
	FallbackReason string `json:"fallback_reason"` // 降级原因
}

// GetEnhancedZScore 获取增强的Z分数结果
func (css *ColdStartStrategy) GetEnhancedZScore(symbol, timeframe string, currentScore float64) *EnhancedZScoreResult {
	normalResult := css.normalizer.GetZScore(symbol, timeframe, currentScore)

	if !normalResult.IsReady {
		fallbackResult := css.GetFallbackZScore(symbol, timeframe, currentScore)
		return &EnhancedZScoreResult{
			ZScoreResult:   fallbackResult,
			IsFallback:     true,
			FallbackReason: fmt.Sprintf("样本不足 (%d<%d)", normalResult.SampleCount, MinSampleSize),
		}
	}

	if css.ShouldUseFallback(symbol, timeframe, normalResult.SampleCount) {
		fallbackResult := css.GetFallbackZScore(symbol, timeframe, currentScore)
		return &EnhancedZScoreResult{
			ZScoreResult:   fallbackResult,
			IsFallback:     true,
			FallbackReason: "系统预热未完成",
		}
	}

	return &EnhancedZScoreResult{
		ZScoreResult:   normalResult,
		IsFallback:     false,
		FallbackReason: "",
	}
}

// GetGlobalColdStartStrategy 获取全局冷启动降级策略实例
func GetGlobalColdStartStrategy() *ColdStartStrategy {
	normalizer := GetGlobalStrengthNormalizer()
	if normalizer == nil {
		return nil
	}
	return NewColdStartStrategy(normalizer)
}

// ===== 数据维护和清理逻辑 =====

// MaintenanceManager 数据维护管理器
type MaintenanceManager struct {
	normalizer *StrengthNormalizer
	isRunning  bool
	stopChan   chan bool
	mu         sync.Mutex
}

// NewMaintenanceManager 创建数据维护管理器
func NewMaintenanceManager(normalizer *StrengthNormalizer) *MaintenanceManager {
	return &MaintenanceManager{
		normalizer: normalizer,
		isRunning:  false,
		stopChan:   make(chan bool, 1),
	}
}

// StartMaintenance 启动定时维护任务
func (mm *MaintenanceManager) StartMaintenance() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.isRunning {
		return
	}

	mm.isRunning = true
	log.Printf("🔧 启动Z-Score数据维护任务...")

	go mm.maintenanceLoop()
}

// StopMaintenance 停止维护任务
func (mm *MaintenanceManager) StopMaintenance() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if !mm.isRunning {
		return
	}

	mm.isRunning = false
	log.Printf("🔧 停止Z-Score数据维护任务")

	select {
	case mm.stopChan <- true:
	default:
	}
}

// maintenanceLoop 维护任务主循环
func (mm *MaintenanceManager) maintenanceLoop() {
	maintenanceInterval := 30 * time.Minute // 30分钟执行一次维护
	cleanupInterval := 12 * time.Hour       // 12小时执行一次清理
	memoryCheckInterval := 10 * time.Minute // 10分钟检查一次内存

	maintenanceTicker := time.NewTicker(maintenanceInterval)
	cleanupTicker := time.NewTicker(cleanupInterval)
	memoryTicker := time.NewTicker(memoryCheckInterval)

	defer maintenanceTicker.Stop()
	defer cleanupTicker.Stop()
	defer memoryTicker.Stop()

	// 启动时立即执行一次检查
	go mm.performDataIntegrityCheck()

	for {
		select {
		case <-mm.stopChan:
			log.Printf("🔧 维护任务收到停止信号")
			return

		case <-maintenanceTicker.C:
			// 数据完整性检查
			go mm.performDataIntegrityCheck()

		case <-cleanupTicker.C:
			// 数据清理
			go mm.performDataCleanup()

		case <-memoryTicker.C:
			// 内存优化检查
			go mm.performMemoryOptimization()
		}
	}
}

// performDataIntegrityCheck 执行数据完整性检查
func (mm *MaintenanceManager) performDataIntegrityCheck() {
	log.Printf("🔍 开始Z-Score数据完整性检查...")

	// 1. 检查内存与数据库的一致性
	if err := mm.checkMemoryDatabaseConsistency(); err != nil {
		log.Printf("❌ 内存-数据库一致性检查失败: %v", err)
	}

	// 2. 检查统计数据的正确性
	if err := mm.validateStatisticsData(); err != nil {
		log.Printf("❌ 统计数据验证失败: %v", err)
	}

	// 3. 检查滑动窗口的健康状态
	if err := mm.checkSlidingWindowHealth(); err != nil {
		log.Printf("❌ 滑动窗口健康检查失败: %v", err)
	}

	log.Printf("✅ Z-Score数据完整性检查完成")
}

// checkMemoryDatabaseConsistency 检查内存与数据库的一致性
func (mm *MaintenanceManager) checkMemoryDatabaseConsistency() error {
	// 获取数据库中的统计信息
	dbStats, err := mm.normalizer.GetStatsCount()
	if err != nil {
		return fmt.Errorf("获取数据库统计失败: %w", err)
	}

	// 获取内存中的统计信息
	memoryStats := mm.normalizer.GetAllStatsInfo()

	// 比较数据库和内存中的symbol+timeframe组合
	issuesFound := 0

	for key, dbCount := range dbStats {
		memoryCount, exists := memoryStats[key]
		if !exists {
			log.Printf("⚠️ 内存中缺少symbol+timeframe: %s (数据库有%d条记录)", key, dbCount)
			issuesFound++

			// 尝试从数据库重新加载到内存
			parts := strings.Split(key, "_")
			if len(parts) >= 2 {
				symbol := parts[0]
				timeframe := parts[1]
				if err := mm.normalizer.loadHistoryScoresIntoMemory(symbol, timeframe); err != nil {
					log.Printf("❌ 重新加载%s失败: %v", key, err)
				} else {
					log.Printf("✅ 成功重新加载%s到内存", key)
				}
			}
		} else if memoryCount == 0 && dbCount > 0 {
			log.Printf("⚠️ 内存中%s数据为空，但数据库有%d条记录", key, dbCount)
			issuesFound++
		}
	}

	// 检查内存中是否有数据库中没有的组合
	for key, memoryCount := range memoryStats {
		if _, exists := dbStats[key]; !exists && memoryCount > 0 {
			log.Printf("⚠️ 数据库中缺少symbol+timeframe: %s (内存有%d个样本)", key, memoryCount)
			issuesFound++
		}
	}

	if issuesFound > 0 {
		log.Printf("🔧 发现%d个一致性问题，已尝试自动修复", issuesFound)
	} else {
		log.Printf("✅ 内存-数据库一致性检查通过")
	}

	return nil
}

// validateStatisticsData 验证统计数据的正确性
func (mm *MaintenanceManager) validateStatisticsData() error {
	mm.normalizer.mu.RLock()
	defer mm.normalizer.mu.RUnlock()

	issuesFound := 0

	for key, stats := range mm.normalizer.stats {
		stats.RLock()

		// 检查样本数量
		if len(stats.HistoryScores) != stats.SampleCount {
			log.Printf("⚠️ %s: 样本数量不一致 (实际%d vs 记录%d)", key, len(stats.HistoryScores), stats.SampleCount)
			stats.SampleCount = len(stats.HistoryScores)
			issuesFound++
		}

		// 检查窗口大小限制
		if len(stats.HistoryScores) > WindowSize {
			log.Printf("⚠️ %s: 滑动窗口超出限制 (%d > %d)", key, len(stats.HistoryScores), WindowSize)
			// 截断到正确大小
			stats.HistoryScores = stats.HistoryScores[len(stats.HistoryScores)-WindowSize:]
			stats.SampleCount = len(stats.HistoryScores)
			issuesFound++
		}

		// 检查统计值的有效性
		if len(stats.HistoryScores) > 0 {
			// 重新计算并验证均值和标准差
			oldMean := stats.Mean
			oldStdDev := stats.StdDev

			// 重新计算
			stats.updateStatistics()

			// 检查是否有显著差异（可能表明数据损坏）
			meanDiff := math.Abs(stats.Mean - oldMean)
			stdDevDiff := math.Abs(stats.StdDev - oldStdDev)

			if meanDiff > 0.01 || stdDevDiff > 0.01 {
				log.Printf("🔧 %s: 统计值已更新 (均值: %.3f->%.3f, 标准差: %.3f->%.3f)",
					key, oldMean, stats.Mean, oldStdDev, stats.StdDev)
			}
		}

		stats.RUnlock()
	}

	if issuesFound > 0 {
		log.Printf("🔧 修复了%d个统计数据问题", issuesFound)
	} else {
		log.Printf("✅ 统计数据验证通过")
	}

	return nil
}

// checkSlidingWindowHealth 检查滑动窗口的健康状态
func (mm *MaintenanceManager) checkSlidingWindowHealth() error {
	mm.normalizer.mu.RLock()
	defer mm.normalizer.mu.RUnlock()

	healthyWindows := 0
	partialWindows := 0
	emptyWindows := 0

	for key, stats := range mm.normalizer.stats {
		stats.RLock()
		sampleCount := len(stats.HistoryScores)
		stats.RUnlock()

		if sampleCount >= MinSampleSize {
			healthyWindows++
		} else if sampleCount > 0 {
			partialWindows++
			log.Printf("⚠️ %s: 样本不足 (%d<%d)", key, sampleCount, MinSampleSize)
		} else {
			emptyWindows++
			log.Printf("⚠️ %s: 空的滑动窗口", key)
		}
	}

	totalWindows := len(mm.normalizer.stats)
	healthPercentage := 0.0
	if totalWindows > 0 {
		healthPercentage = float64(healthyWindows) / float64(totalWindows) * 100
	}

	log.Printf("📊 滑动窗口健康状态: 健康%d, 部分%d, 空%d (健康率%.1f%%)",
		healthyWindows, partialWindows, emptyWindows, healthPercentage)

	return nil
}

// performDataCleanup 执行数据清理
func (mm *MaintenanceManager) performDataCleanup() {
	log.Printf("🧹 开始Z-Score数据清理...")

	// 1. 清理数据库中的过期记录
	if err := mm.normalizer.CleanupOldRecords(); err != nil {
		log.Printf("❌ 数据库清理失败: %v", err)
	} else {
		log.Printf("✅ 数据库过期记录清理完成")
	}

	// 2. 清理内存中的不活跃统计状态
	idleThreshold := 2 * time.Hour // 2小时无更新视为不活跃
	prunedCount := mm.normalizer.PruneInactiveStats(idleThreshold)
	if prunedCount > 0 {
		log.Printf("✅ 清理了%d个不活跃的内存状态", prunedCount)
	}

	// 3. 执行数据库优化
	if err := mm.optimizeDatabase(); err != nil {
		log.Printf("❌ 数据库优化失败: %v", err)
	} else {
		log.Printf("��� 数据库优化完成")
	}

	log.Printf("✅ Z-Score数据清理完成")
}

// optimizeDatabase 优化数据库性能
func (mm *MaintenanceManager) optimizeDatabase() error {
	if mm.normalizer.db == nil {
		return fmt.Errorf("数据库连接为空")
	}

	// 执行SQLite的VACUUM命令来优化数据库
	_, err := mm.normalizer.db.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("数据库VACUUM失败: %w", err)
	}

	// 分析数据库以更新统计信息
	_, err = mm.normalizer.db.Exec("ANALYZE")
	if err != nil {
		return fmt.Errorf("数据库ANALYZE失败: %w", err)
	}

	return nil
}

// performMemoryOptimization 执行内存优化
func (mm *MaintenanceManager) performMemoryOptimization() {
	// 1. 检查内存使用情况
	utilization := mm.normalizer.GetWindowUtilization()
	overUsedCount := 0
	underUsedCount := 0

	for key, usage := range utilization {
		if usage > 0.9 { // 90%以上使用率
			overUsedCount++
		} else if usage < 0.1 { // 10%以下使用率
			underUsedCount++
			log.Printf("💡 %s: 滑动窗��使用率较低 (%.1f%%)", key, usage*100)
		}
	}

	// 2. 强制进行垃圾回收（如果内存压力较大）
	if overUsedCount > 10 {
		log.Printf("🗑️ 检测到内存压力，执行垃圾回收")
		// 这里可以添加更积极的内存管理策略
	}

	log.Printf("📊 内存使用情况: 高使用%d, 低使用%d", overUsedCount, underUsedCount)
}

// GetMaintenanceStatus 获取维护状态信息
func (mm *MaintenanceManager) GetMaintenanceStatus() map[string]interface{} {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	status := map[string]interface{}{
		"is_running":          mm.isRunning,
		"database_connection": mm.normalizer.db != nil,
		"total_stats_loaded":  len(mm.normalizer.stats),
	}

	// 添加内存使用统计
	utilization := mm.normalizer.GetWindowUtilization()
	healthyCount := 0
	for _, usage := range utilization {
		if usage >= 0.25 { // 25%以上使用率视为健康
			healthyCount++
		}
	}

	status["healthy_windows"] = healthyCount
	status["total_windows"] = len(utilization)

	if len(utilization) > 0 {
		status["health_percentage"] = float64(healthyCount) / float64(len(utilization)) * 100
	} else {
		status["health_percentage"] = 0.0
	}

	return status
}

// ForceCleanup 强制执行一次完整的清理操作
func (mm *MaintenanceManager) ForceCleanup() error {
	log.Printf("🔧 强制执行Z-Score系统清理...")

	// 执行所有维护操作
	mm.performDataIntegrityCheck()
	mm.performDataCleanup()
	mm.performMemoryOptimization()

	log.Printf("✅ 强制清理完成")
	return nil
}

// GetHealthReport 获取系统健康报告
func (mm *MaintenanceManager) GetHealthReport() map[string]interface{} {
	report := map[string]interface{}{}

	// 基础状态
	report["maintenance_running"] = mm.isRunning
	report["database_available"] = mm.normalizer.db != nil

	// 内存状态
	mm.normalizer.mu.RLock()
	statsCount := len(mm.normalizer.stats)
	mm.normalizer.mu.RUnlock()

	report["loaded_stats"] = statsCount

	// 窗口利用率分析
	utilization := mm.normalizer.GetWindowUtilization()
	if len(utilization) > 0 {
		totalUsage := 0.0
		minUsage := 1.0
		maxUsage := 0.0

		for _, usage := range utilization {
			totalUsage += usage
			if usage < minUsage {
				minUsage = usage
			}
			if usage > maxUsage {
				maxUsage = usage
			}
		}

		report["avg_window_utilization"] = totalUsage / float64(len(utilization))
		report["min_window_utilization"] = minUsage
		report["max_window_utilization"] = maxUsage
	} else {
		report["avg_window_utilization"] = 0.0
		report["min_window_utilization"] = 0.0
		report["max_window_utilization"] = 0.0
	}

	// 数据库统计
	if dbStats, err := mm.normalizer.GetStatsCount(); err == nil {
		totalRecords := 0
		for _, count := range dbStats {
			totalRecords += count
		}
		report["database_records"] = totalRecords
		report["database_symbols"] = len(dbStats)
	} else {
		report["database_records"] = -1
		report["database_symbols"] = -1
		report["database_error"] = err.Error()
	}

	return report
}

// 全局维护管理器实例
var (
	globalMaintenanceManager *MaintenanceManager
	maintenanceOnce          sync.Once
)

// InitGlobalMaintenance 初始化全局维护管理器
func InitGlobalMaintenance() {
	maintenanceOnce.Do(func() {
		if normalizer := GetGlobalStrengthNormalizer(); normalizer != nil {
			globalMaintenanceManager = NewMaintenanceManager(normalizer)
			globalMaintenanceManager.StartMaintenance()
			log.Printf("✅ 全局Z-Score维护管理器已启动")
		} else {
			log.Printf("⚠️ 无法启动维护管理器：强度标准化器未初始化")
		}
	})
}

// GetGlobalMaintenance 获取全局维护管理器
func GetGlobalMaintenance() *MaintenanceManager {
	return globalMaintenanceManager
}

// ===== Z-Score计算和截断逻辑 =====

// GetZScore 获取标准化的Z分数（主要API接口��
// 返回值：ZScoreResult包含Z分数、数据是否就绪等信息
func (sn *StrengthNormalizer) GetZScore(symbol, timeframe string, currentScore float64) *ZScoreResult {
	// 参数验证和标准化
	symbol = formatSymbol(symbol)
	if !validateTimeframe(timeframe) {
		return &ZScoreResult{
			ZScore:  0.0,
			IsReady: false,
		}
	}

	key := getStatsKey(symbol, timeframe)

	sn.mu.RLock()
	stats := sn.stats[key]
	sn.mu.RUnlock()

	if stats == nil {
		// 没有历史数据，返回中性分数
		return &ZScoreResult{
			ZScore:      0.0,
			IsReady:     false,
			SampleCount: 0,
		}
	}

	// 委托给单个统计状态计算
	return stats.calculateZScore(currentScore)
}

// calculateZScore 计算Z分数（内部方法）
func (ss *StrengthStats) calculateZScore(currentScore float64) *ZScoreResult {
	ss.RLock()
	defer ss.RUnlock()

	result := &ZScoreResult{
		SampleCount: len(ss.HistoryScores),
		Mean:        ss.Mean,
		StdDev:      ss.StdDev,
	}

	// 冷启动保护：样本量不足
	if len(ss.HistoryScores) < MinSampleSize {
		result.ZScore = 0.0
		result.IsReady = false
		return result
	}

	// 防止除零错误
	if ss.StdDev == 0 {
		// 特殊情况：所有历史分数都相同
		if currentScore > ss.Mean {
			result.ZScore = MaxZScore
		} else if currentScore < ss.Mean {
			result.ZScore = -MaxZScore
		} else {
			result.ZScore = 0.0
		}
		result.IsReady = true
		return result
	}

	// 标准Z-Score计算
	rawZ := (currentScore - ss.Mean) / ss.StdDev

	// 极值截断保护：强制限制在 [-3.0, +3.0] 范围内
	clippedZ := clipZScore(rawZ)

	result.ZScore = clippedZ
	result.IsReady = true

	// 调试信息：记录极值截断情况
	if math.Abs(rawZ) > MaxZScore {
		log.Printf("🔒 [%s_%s] Z-Score截断: %.3f → %.3f (当前分数=%.2f, 均值=%.2f, 标准差=%.2f)",
			ss.Symbol, ss.Timeframe, rawZ, clippedZ, currentScore, ss.Mean, ss.StdDev)
	}

	return result
}

// clipZScore 截断Z分数到安全范围
func clipZScore(z float64) float64 {
	if z > MaxZScore {
		return MaxZScore
	}
	if z < -MaxZScore {
		return -MaxZScore
	}
	return z
}

// GetZScoreWithUpdate 获取Z分数并同时更新历史记录（常用组合操作）
// 这是最常用的API：既获取当前标准化分数，又将当前分数加入历史窗口
func (sn *StrengthNormalizer) GetZScoreWithUpdate(symbol, timeframe string, currentScore float64, zoneInfo *ZoneStrengthRecord) *ZScoreResult {
	// 先获取基于现有历史数据的Z分数
	result := sn.GetZScore(symbol, timeframe, currentScore)

	// 然后更新历史窗口（为下次计算做准备）
	sn.AddScore(symbol, timeframe, currentScore, zoneInfo)

	return result
}

// BatchGetZScores 批量获取多个币种的Z分数（性能优化）
func (sn *StrengthNormalizer) BatchGetZScores(requests []struct {
	Symbol    string
	Timeframe string
	RawScore  float64
}) map[string]*ZScoreResult {
	results := make(map[string]*ZScoreResult)

	for _, req := range requests {
		key := getStatsKey(req.Symbol, req.Timeframe)
		results[key] = sn.GetZScore(req.Symbol, req.Timeframe, req.RawScore)
	}

	return results
}

// GetWidthZScore 获取宽度的标准化Z分数
// 用于判断供需区宽度相对于历史宽度分布的标准化程度
func (sn *StrengthNormalizer) GetWidthZScore(symbol, timeframe string, currentWidthATR float64) *WidthZScoreResult {
	// 参数验证和标准化
	symbol = formatSymbol(symbol)
	if !validateTimeframe(timeframe) {
		return &WidthZScoreResult{
			WidthZScore: 0.0,
			IsReady:     false,
		}
	}

	key := getStatsKey(symbol, timeframe)

	sn.mu.RLock()
	stats := sn.stats[key]
	sn.mu.RUnlock()

	if stats == nil {
		// 没有历史数据，返回中性分数
		return &WidthZScoreResult{
			WidthZScore: 0.0,
			IsReady:     false,
			SampleCount: 0,
		}
	}

	// 委托给单个统计状态计算宽度Z分数
	return stats.calculateWidthZScore(currentWidthATR)
}

// calculateWidthZScore 计算宽度Z分数（内部方法）
func (ss *StrengthStats) calculateWidthZScore(currentWidthATR float64) *WidthZScoreResult {
	ss.RLock()
	defer ss.RUnlock()

	result := &WidthZScoreResult{
		SampleCount: len(ss.HistoryWidths),
		WidthMean:   ss.WidthMean,
		WidthStdDev: ss.WidthStdDev,
	}

	// 冷启动保护：样本量不足
	if len(ss.HistoryWidths) < MinSampleSize {
		result.WidthZScore = 0.0
		result.IsReady = false
		return result
	}

	// 防止除零错误
	if ss.WidthStdDev == 0 {
		// 特殊情况：所有历史宽度都相同
		if currentWidthATR > ss.WidthMean {
			result.WidthZScore = MaxZScore
		} else if currentWidthATR < ss.WidthMean {
			result.WidthZScore = -MaxZScore
		} else {
			result.WidthZScore = 0.0
		}
		result.IsReady = true
		return result
	}

	// 标准Z-Score计算
	rawZ := (currentWidthATR - ss.WidthMean) / ss.WidthStdDev

	// 极值截断保护：强制限制在 [-3.0, +3.0] 范围内
	clippedZ := clipZScore(rawZ)

	result.WidthZScore = clippedZ
	result.IsReady = true

	// 调试信息：记录极值截断情况
	if math.Abs(rawZ) > MaxZScore {
		log.Printf("🔒 [%s_%s] 宽度Z-Score截断: %.3f → %.3f (当前宽度=%.3f ATR, 均值=%.3f, 标准差=%.3f)",
			ss.Symbol, ss.Timeframe, rawZ, clippedZ, currentWidthATR, ss.WidthMean, ss.WidthStdDev)
	}

	return result
}

// GetZScoreDistribution 获取Z分数分布统计（分析和调试用）
func (sn *StrengthNormalizer) GetZScoreDistribution(symbol, timeframe string) *ZScoreDistribution {
	key := getStatsKey(symbol, timeframe)

	sn.mu.RLock()
	stats := sn.stats[key]
	sn.mu.RUnlock()

	if stats == nil {
		return nil
	}

	return stats.analyzeDistribution()
}

// ZScoreDistribution Z分数分布分析结果
type ZScoreDistribution struct {
	Symbol      string             `json:"symbol"`
	Timeframe   string             `json:"timeframe"`
	SampleCount int                `json:"sample_count"`
	Mean        float64            `json:"mean"`
	StdDev      float64            `json:"std_dev"`
	Min         float64            `json:"min"`
	Max         float64            `json:"max"`
	Percentiles map[string]float64 `json:"percentiles"` // P25, P50, P75, P90, P95

	// Z分数区间分布
	ZScoreBins map[string]int `json:"z_score_bins"` // 各Z分数区间的样本数量
}

// analyzeDistribution 分析历史分数的分布特征
func (ss *StrengthStats) analyzeDistribution() *ZScoreDistribution {
	ss.RLock()
	defer ss.RUnlock()

	if len(ss.HistoryScores) == 0 {
		return &ZScoreDistribution{
			Symbol:    ss.Symbol,
			Timeframe: ss.Timeframe,
		}
	}

	// 复制数据进行排序分析
	scores := make([]float64, len(ss.HistoryScores))
	copy(scores, ss.HistoryScores)

	// 排序以计算百分位数
	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[i] > scores[j] {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// 计算基本统计
	min := scores[0]
	max := scores[len(scores)-1]

	// 计算百分位数
	percentiles := make(map[string]float64)
	percentiles["P25"] = getPercentile(scores, 0.25)
	percentiles["P50"] = getPercentile(scores, 0.50)
	percentiles["P75"] = getPercentile(scores, 0.75)
	percentiles["P90"] = getPercentile(scores, 0.90)
	percentiles["P95"] = getPercentile(scores, 0.95)

	// 分析Z分数分布
	zScoreBins := make(map[string]int)
	for _, score := range ss.HistoryScores {
		if ss.StdDev > 0 {
			z := (score - ss.Mean) / ss.StdDev
			bin := getZScoreBin(z)
			zScoreBins[bin]++
		}
	}

	return &ZScoreDistribution{
		Symbol:      ss.Symbol,
		Timeframe:   ss.Timeframe,
		SampleCount: len(ss.HistoryScores),
		Mean:        ss.Mean,
		StdDev:      ss.StdDev,
		Min:         min,
		Max:         max,
		Percentiles: percentiles,
		ZScoreBins:  zScoreBins,
	}
}

// getPercentile 计算指定百分位数
func getPercentile(sortedScores []float64, p float64) float64 {
	if len(sortedScores) == 0 {
		return 0
	}

	index := p * float64(len(sortedScores)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sortedScores) {
		return sortedScores[len(sortedScores)-1]
	}

	// 线性插值
	weight := index - float64(lower)
	return sortedScores[lower]*(1-weight) + sortedScores[upper]*weight
}

// getZScoreBin 将Z分数分配到对应的区间
func getZScoreBin(z float64) string {
	switch {
	case z >= 2.0:
		return "Z>=2.0(强)"
	case z >= 1.0:
		return "1.0<=Z<2.0(偏强)"
	case z >= 0.0:
		return "0.0<=Z<1.0(中等偏强)"
	case z >= -1.0:
		return "-1.0<=Z<0.0(中等偏弱)"
	case z >= -2.0:
		return "-2.0<=Z<-1.0(偏弱)"
	default:
		return "Z<-2.0(弱)"
	}
}
