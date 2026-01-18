package microstructure

import (
	"log"
	"runtime"
	"sync"
	"time"
)

// MetricsCollector 🔥 T10新增：指标收集器
type MetricsCollector struct {
	mu              sync.RWMutex
	config          *MicrostructureConfig
	lastMetrics     *MicrostructureMetrics
	lastCollection  time.Time
	enabled         bool

	// 引用各组件以收集指标
	cvdManager      *CVDManager
	oiManager       *OIManager
	obCalculator    *OrderBookCalculator
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector(config *MicrostructureConfig) *MetricsCollector {
	return &MetricsCollector{
		config:  config,
		enabled: config.EnableMetrics,
	}
}

// SetComponents 设置要监控的组件引用
func (mc *MetricsCollector) SetComponents(cvdMgr *CVDManager, oiMgr *OIManager, obCalc *OrderBookCalculator) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.cvdManager = cvdMgr
	mc.oiManager = oiMgr
	mc.obCalculator = obCalc
}

// CollectMetrics 🔥 T10新增：采集所有指标
func (mc *MetricsCollector) CollectMetrics() *MicrostructureMetrics {
	if !mc.enabled {
		return nil
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	now := time.Now()
	metrics := &MicrostructureMetrics{
		Memory:    mc.collectMemoryMetrics(),
		Timestamp: now,
	}

	// 收集各组件指标
	if mc.cvdManager != nil {
		metrics.CVDComponent = mc.collectCVDMetrics()
	}
	if mc.oiManager != nil {
		metrics.OIComponent = mc.collectOIMetrics()
	}
	if mc.obCalculator != nil {
		metrics.OrderBookComponent = mc.collectOrderBookMetrics()
	}

	mc.lastMetrics = metrics
	mc.lastCollection = now

	return metrics
}

// collectMemoryMetrics 收集内存指标
func (mc *MetricsCollector) collectMemoryMetrics() MemoryMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return MemoryMetrics{
		TotalAllocMB:  float64(m.TotalAlloc) / 1024 / 1024,
		HeapAllocMB:   float64(m.HeapAlloc) / 1024 / 1024,
		HeapInUseMB:   float64(m.HeapInuse) / 1024 / 1024,
		NumGC:         m.NumGC,
		LastGCPauseMs: float64(m.PauseNs[(m.NumGC+255)%256]) / 1000000,
		Timestamp:     time.Now(),
	}
}

// collectCVDMetrics 收集CVD组件指标
func (mc *MetricsCollector) collectCVDMetrics() ComponentMetrics {
	mc.cvdManager.mu.RLock()
	defer mc.cvdManager.mu.RUnlock()

	symbolCount := len(mc.cvdManager.calculators)
	totalRecords := 0
	var oldestAge time.Duration

	now := time.Now()
	for _, calc := range mc.cvdManager.calculators {
		calc.mu.RLock()
		totalRecords += len(calc.spotDeltas) + len(calc.futuresDeltas)

		// 计算最老记录年龄
		if len(calc.spotDeltas) > 0 {
			age := now.Sub(calc.spotDeltas[0].Timestamp)
			if age > oldestAge {
				oldestAge = age
			}
		}
		calc.mu.RUnlock()
	}

	avgRecords := 0.0
	if symbolCount > 0 {
		avgRecords = float64(totalRecords) / float64(symbolCount)
	}

	// 估算内存：每条CVDDelta约40字节
	estimatedMem := float64(totalRecords*40) / 1024 / 1024

	return ComponentMetrics{
		SymbolCount:       symbolCount,
		TotalRecords:      totalRecords,
		AvgRecordsPerSymbol: avgRecords,
		OldestRecordAge:   oldestAge,
		EstimatedMemoryMB: estimatedMem,
	}
}

// collectOIMetrics 收集OI组件指标
func (mc *MetricsCollector) collectOIMetrics() ComponentMetrics {
	mc.oiManager.mu.RLock()
	defer mc.oiManager.mu.RUnlock()

	symbolCount := len(mc.oiManager.calculators)
	totalRecords := 0
	var oldestAge time.Duration

	now := time.Now()
	for _, calc := range mc.oiManager.calculators {
		calc.mu.RLock()
		totalRecords += len(calc.changes)

		if len(calc.changes) > 0 {
			age := now.Sub(calc.changes[0].Timestamp)
			if age > oldestAge {
				oldestAge = age
			}
		}
		calc.mu.RUnlock()
	}

	avgRecords := 0.0
	if symbolCount > 0 {
		avgRecords = float64(totalRecords) / float64(symbolCount)
	}

	// 估算内存：每条OIChange约32字节
	estimatedMem := float64(totalRecords*32) / 1024 / 1024

	return ComponentMetrics{
		SymbolCount:       symbolCount,
		TotalRecords:      totalRecords,
		AvgRecordsPerSymbol: avgRecords,
		OldestRecordAge:   oldestAge,
		EstimatedMemoryMB: estimatedMem,
	}
}

// collectOrderBookMetrics 收集OrderBook组件指标
func (mc *MetricsCollector) collectOrderBookMetrics() ComponentMetrics {
	mc.obCalculator.mu.RLock()
	defer mc.obCalculator.mu.RUnlock()

	symbolCount := len(mc.obCalculator.symbolData)
	totalRecords := 0
	var oldestAge time.Duration

	now := time.Now()
	for _, data := range mc.obCalculator.symbolData {
		// 计算imbalance历史记录数
		totalRecords += len(data.imbalanceHistory)

		// 🔥 T10修复：使用大写Timestamp字段
		if len(data.imbalanceHistory) > 0 {
			age := now.Sub(data.imbalanceHistory[0].Timestamp)
			if age > oldestAge {
				oldestAge = age
			}
		}
	}

	avgRecords := 0.0
	if symbolCount > 0 {
		avgRecords = float64(totalRecords) / float64(symbolCount)
	}

	// 估算内存：每条imbalance记录约32字节
	estimatedMem := float64(totalRecords*32) / 1024 / 1024

	return ComponentMetrics{
		SymbolCount:       symbolCount,
		TotalRecords:      totalRecords,
		AvgRecordsPerSymbol: avgRecords,
		OldestRecordAge:   oldestAge,
		EstimatedMemoryMB: estimatedMem,
	}
}

// GetLastMetrics 获取最后一次采集的指标
func (mc *MetricsCollector) GetLastMetrics() *MicrostructureMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.lastMetrics
}

// LogMetrics 🔥 T10新增：记录指标到日志（格式化输出）
func (mc *MetricsCollector) LogMetrics() {
	metrics := mc.CollectMetrics()
	if metrics == nil {
		return
	}

	log.Printf("📊 ===== 微观结构指标汇总 =====")

	// 内存指标
	log.Printf("🧠 内存: 堆使用=%.2fMB, 总分配=%.2fMB, GC次数=%d, 最后GC暂停=%.2fms",
		metrics.Memory.HeapInUseMB,
		metrics.Memory.TotalAllocMB,
		metrics.Memory.NumGC,
		metrics.Memory.LastGCPauseMs)

	// CVD组件
	if metrics.CVDComponent.SymbolCount > 0 {
		log.Printf("📈 CVD组件: %d币种, %d记录, 平均%.1f/币种, 最老%.0fm, 估算内存%.2fMB",
			metrics.CVDComponent.SymbolCount,
			metrics.CVDComponent.TotalRecords,
			metrics.CVDComponent.AvgRecordsPerSymbol,
			metrics.CVDComponent.OldestRecordAge.Minutes(),
			metrics.CVDComponent.EstimatedMemoryMB)
	}

	// OI组件
	if metrics.OIComponent.SymbolCount > 0 {
		log.Printf("📊 OI组件: %d币种, %d记录, 平均%.1f/币种, 最老%.0fm, 估算内存%.2fMB",
			metrics.OIComponent.SymbolCount,
			metrics.OIComponent.TotalRecords,
			metrics.OIComponent.AvgRecordsPerSymbol,
			metrics.OIComponent.OldestRecordAge.Minutes(),
			metrics.OIComponent.EstimatedMemoryMB)
	}

	// OrderBook组件
	if metrics.OrderBookComponent.SymbolCount > 0 {
		log.Printf("📖 OrderBook组件: %d币种, %d记录, 平均%.1f/币种, 最老%.0fm, 估算内存%.2fMB",
			metrics.OrderBookComponent.SymbolCount,
			metrics.OrderBookComponent.TotalRecords,
			metrics.OrderBookComponent.AvgRecordsPerSymbol,
			metrics.OrderBookComponent.OldestRecordAge.Minutes(),
			metrics.OrderBookComponent.EstimatedMemoryMB)
	}

	// 总体内存估算
	totalEstimatedMem := metrics.CVDComponent.EstimatedMemoryMB +
		metrics.OIComponent.EstimatedMemoryMB +
		metrics.OrderBookComponent.EstimatedMemoryMB

	log.Printf("💾 总估算内存: %.2fMB (配置上限: %dMB)", totalEstimatedMem, mc.config.MemoryLimitMB)

	// 内存预警
	if totalEstimatedMem > float64(mc.config.MemoryLimitMB)*0.8 {
		log.Printf("⚠️  警告: 内存使用接近上限 (%.1f%%), 建议触发清理",
			totalEstimatedMem/float64(mc.config.MemoryLimitMB)*100)
	}

	log.Printf("===============================")
}

// StartPeriodicCollection 🔥 T10新增：启动定期指标采集
func (mc *MetricsCollector) StartPeriodicCollection(stopCh <-chan struct{}) {
	if !mc.enabled {
		log.Printf("📊 指标采集已禁用")
		return
	}

	log.Printf("📊 启动定期指标采集，间隔=%v", mc.config.MetricsInterval)

	ticker := time.NewTicker(mc.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.LogMetrics()
		case <-stopCh:
			log.Printf("📊 停止指标采集")
			return
		}
	}
}
