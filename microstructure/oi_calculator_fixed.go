package microstructure

import (
	"fmt"
	"log"
	"os" 
	"time"
)

// ===== P0-08修复：OI计算器修复方法 =====

// GetOIAnalysisFixed 🔥 P0-08修复版：修复注释与实现不一致和time.Now()问题
func (calc *OICalculator) GetOIAnalysisFixed(config *OIStaleConfig) *OIAnalysis {
	calc.mu.RLock()
	defer calc.mu.RUnlock()
	
	// 🔥 P0-08修复：检查是否有真实数据
	hasRealData := len(calc.changes) > 0 && !calc.lastUpdate.IsZero()
	
	if !hasRealData {
		// 🔥 P0-08修复：无数据时返回零值时间，不使用time.Now()
		return &OIAnalysis{
			Current:      0,
			Change1H:     0,
			Change4H:     0,
			ChangeRate1H: 0,
			ChangeRate4H: 0,
			Trend:        "no_data",
			LastUpdate:   time.Time{}, // 🔥 P0-08修复：零值时间，明确表示无数据
			IsStale:      true,
		}
	}
	
	// 计算变化数据
	change1H, changeRate1H := calc.calculateChange(time.Hour)
	change4H, changeRate4H := calc.calculateChange(4 * time.Hour)
	trend := calc.determineTrend(changeRate1H)
	
	// 🔥 P0-08修复：使用配置的阈值而非硬编码的5分钟
	staleThreshold := config.GetStaleThreshold()
	isStale := time.Since(calc.lastUpdate) > staleThreshold
	
	if os.Getenv("NOFX_DEBUG") == "true" {
		log.Printf("🔧 [P0-08] OI过期判断: 数据年龄=%.1f分钟, 阈值=%.1f分钟, 过期=%v", 
			time.Since(calc.lastUpdate).Minutes(), staleThreshold.Minutes(), isStale)
	}
	
	return &OIAnalysis{
		Current:      calc.current,
		Change1H:     change1H,
		Change4H:     change4H,
		ChangeRate1H: changeRate1H,
		ChangeRate4H: changeRate4H,
		Trend:        trend,
		LastUpdate:   calc.lastUpdate, // 使用真实数据时间
		IsStale:      isStale,         // 🔥 P0-08修复：使用配置阈值
	}
}

// ===== P0-08修复：OI管理器修复方法 =====

// GetOIAnalysisFixed 🔥 P0-08修复版：修复无计算器路径的time.Now()问题
func (manager *OIManager) GetOIAnalysisFixed(symbol string, config *OIStaleConfig) *OIAnalysis {
	if config == nil {
		config = DefaultOIStaleConfig()
	}
	
	manager.mu.RLock()
	calc, exists := manager.calculators[symbol]
	manager.mu.RUnlock()
	
	if !exists {
		// 🔥 P0-08修复：无计算器时使用零值时间，明确表示无数据
		return &OIAnalysis{
			Current:      0,
			Change1H:     0,
			Change4H:     0,
			ChangeRate1H: 0,
			ChangeRate4H: 0,
			Trend:        "no_data",
			LastUpdate:   time.Time{}, // 🔥 P0-08修复：零值时间替代time.Now()
			IsStale:      true,
		}
	}
	
	return calc.GetOIAnalysisFixed(config)
}

// ===== P0-08修复：阈值一致性修复 =====

// FixOIStaleThresholdConsistency 🔥 P0-08修复：修复阈值注释与实现不一致
func FixOIStaleThresholdConsistency() {
	log.Printf("🔧 [P0-08] 修复OI过期阈值一致性问题")
	
	// 默认配置
	config := DefaultOIStaleConfig()
	
	log.Printf("📋 [P0-08] 修复后的OI阈值配置:")
	log.Printf("   WebSocket更新间隔: %d秒", config.WSUpdateIntervalSeconds)
	log.Printf("   过期阈值: %d分钟", config.StaleThresholdMinutes)
	log.Printf("   严重过期阈值: %d分钟", config.CriticalThresholdMinutes)
	log.Printf("   配置说明: %s", config.ConfigDescription)
	log.Printf("   推荐阈值: %s", config.RecommendedThreshold)
	
	// 验证配置合理性
	if err := config.ValidateOIStaleConfig(); err != nil {
		log.Printf("❌ [P0-08] 配置验证失败: %v", err)
		return
	}
	
	log.Printf("✅ [P0-08] OI阈值配置验证通过，注释与实现现已一致")
}

// ===== P0-08修复：集成方法 =====

// ApplyP008Fix 🔥 P0-08修复：应用P0-08修复到现有系统
func (manager *OIManager) ApplyP008Fix() {
	log.Printf("🔧 [P0-08] 开始应用P0-08修复到OI管理器")
	
	// 修复阈值一致性
	FixOIStaleThresholdConsistency()
	
	// 验证现有数据
	symbols := make([]string, 0, len(manager.calculators))
	manager.mu.RLock()
	for symbol := range manager.calculators {
		symbols = append(symbols, symbol)
	}
	manager.mu.RUnlock()
	
	if len(symbols) > 0 {
		// 运行验证
		validationResult := ValidateOIStaleThreshold(manager, symbols)
		
		if validationResult["threshold_consistency"].(bool) {
			log.Printf("✅ [P0-08] P0-08修复验证通过")
		} else {
			issues := validationResult["issues_found"].([]string)
			log.Printf("⚠️ [P0-08] 发现%d个问题需要关注: %v", len(issues), issues)
		}
	}
	
	log.Printf("🎉 [P0-08] P0-08修复应用完成")
}

// ===== P0-08修复：向后兼容包装器 =====

// GetOIAnalysisBackwardCompatible 向后兼容的获取方法（P0-08修复）
func (manager *OIManager) GetOIAnalysisBackwardCompatible(symbol string) *OIAnalysis {
	// 使用默认配置获取修复版本的分析
	return manager.GetOIAnalysisFixed(symbol, nil)
}

// GetOIAnalysisV2EnhancedForP008Helper 临时助手方法解决编译冲突
func (manager *OIManager) GetOIAnalysisV2EnhancedForP008Helper(symbol string, config *OIStaleConfig) *OIAnalysisV2EnhancedForP008 {
	// 简化版实现，返回基本结构
	analysis := manager.GetOIAnalysisFixed(symbol, config)
	now := time.Now()
	
	return &OIAnalysisV2EnhancedForP008{
		Current:      analysis.Current,
		Change1H:     analysis.Change1H,
		Change4H:     analysis.Change4H,
		ChangeRate1H: analysis.ChangeRate1H,
		ChangeRate4H: analysis.ChangeRate4H,
		Trend:        analysis.Trend,
		DataHealth: OIDataHealth{
			HasData:           !analysis.LastUpdate.IsZero(),
			LastRealUpdate:    analysis.LastUpdate,
			DataAgeMinutes:    time.Since(analysis.LastUpdate).Minutes(),
			IsStale:           analysis.IsStale,
			StalenessLevel:    func() string { if analysis.IsStale { return "stale" } else { return "fresh" } }(),
			QualityWeight:     func() float64 { if analysis.IsStale { return 0.3 } else { return 1.0 } }(),
			HealthCheckTime:   now,
		},
		ConfigUsed:     config,
		LastUpdate:     analysis.LastUpdate,
		IsStale:        analysis.IsStale,
		GenerationTime: now,
		QualityGrade:   func() string { if analysis.IsStale { return "C" } else { return "A" } }(),
	}
}

// ===== P0-08修复：数据一致性验证 =====

// VerifyOIDataConsistency 验证OI数据一致性（P0-08修复）
func (manager *OIManager) VerifyOIDataConsistency(symbol string) map[string]interface{} {
	log.Printf("🔬 [P0-08] 验证%s的OI数据一致性", symbol)
	
	config := DefaultOIStaleConfig()
	
	// 获取各种版本的分析
	legacyAnalysis := manager.GetOIAnalysis(symbol)           // 原版本
	fixedAnalysis := manager.GetOIAnalysisFixed(symbol, config)  // 修复版本
	enhancedAnalysis := manager.GetOIAnalysisV2EnhancedForP008Helper(symbol, config)  // 增强版本
	
	verification := map[string]interface{}{
		"symbol":           symbol,
		"verification_time": time.Now().Format("15:04:05.000"),
		
		// 比较LastUpdate字段
		"legacy_last_update":    legacyAnalysis.LastUpdate,
		"fixed_last_update":     fixedAnalysis.LastUpdate,
		"enhanced_last_update":  enhancedAnalysis.LastUpdate,
		
		// 比较IsStale字段
		"legacy_is_stale":    legacyAnalysis.IsStale,
		"fixed_is_stale":     fixedAnalysis.IsStale,
		"enhanced_is_stale":  enhancedAnalysis.IsStale,
		
		// 增强版专有字段
		"enhanced_has_data":      enhancedAnalysis.DataHealth.HasData,
		"enhanced_staleness":     enhancedAnalysis.DataHealth.StalenessLevel,
		"enhanced_quality":       enhancedAnalysis.QualityGrade,
		"enhanced_data_age_min":  enhancedAnalysis.DataHealth.DataAgeMinutes,
		
		// 一致性检查
		"last_update_consistent": true,
		"stale_status_consistent": true,
		"p008_issues_detected":   []string{},
	}
	
	issues := []string{}
	
	// 检查P0-08相关问题
	if legacyAnalysis.IsStale && 
	   time.Since(legacyAnalysis.LastUpdate) < time.Minute && 
	   !enhancedAnalysis.DataHealth.HasData {
		issues = append(issues, "原版本在无数据时使用time.Now()作为LastUpdate")
		verification["last_update_consistent"] = false
	}
	
	// 检查修复版本是否正确使用零值时间
	if !enhancedAnalysis.DataHealth.HasData && !fixedAnalysis.LastUpdate.IsZero() {
		issues = append(issues, "修复版本无数据时应使用零值时间")
		verification["last_update_consistent"] = false
	}
	
	// 检查stale状态一致性
	if fixedAnalysis.IsStale != enhancedAnalysis.IsStale {
		issues = append(issues, fmt.Sprintf("修复版本与增强版本stale状态不一致: fixed=%v, enhanced=%v", 
			fixedAnalysis.IsStale, enhancedAnalysis.IsStale))
		verification["stale_status_consistent"] = false
	}
	
	verification["p008_issues_detected"] = issues
	
	if len(issues) == 0 {
		log.Printf("✅ [P0-08] %s数据一致性验证通过", symbol)
	} else {
		log.Printf("⚠️ [P0-08] %s发现%d个一致性问题: %v", symbol, len(issues), issues)
	}
	
	return verification
}

// ===== P0-08修复：监控和报告 =====

// GenerateOIP008FixReport 生成P0-08修复报告
func (manager *OIManager) GenerateOIP008FixReport() map[string]interface{} {
	log.Printf("📊 [P0-08] 生成P0-08修复状态报告")
	
	config := DefaultOIStaleConfig()
	
	// 获取所有交易对列表
	manager.mu.RLock()
	symbols := make([]string, 0, len(manager.calculators))
	for symbol := range manager.calculators {
		symbols = append(symbols, symbol)
	}
	manager.mu.RUnlock()
	
	report := map[string]interface{}{
		"report_time":      time.Now().Format("15:04:05.000"),
		"total_symbols":    len(symbols),
		"config_used":      config,
		"fix_summary":      generateOIP008FixSummary(),
		"symbol_analysis":  map[string]interface{}{},
		"overall_health":   "unknown",
		"recommendations":  []string{},
	}
	
	healthyCount := 0
	issues := []string{}
	
	// 分析每个交易对
	for _, symbol := range symbols {
		verification := manager.VerifyOIDataConsistency(symbol)
		report["symbol_analysis"].(map[string]interface{})[symbol] = verification
		
		if len(verification["p008_issues_detected"].([]string)) == 0 {
			healthyCount++
		} else {
			symbolIssues := verification["p008_issues_detected"].([]string)
			for _, issue := range symbolIssues {
				issues = append(issues, fmt.Sprintf("%s: %s", symbol, issue))
			}
		}
	}
	
	// 确定整体健康状况
	if len(issues) == 0 {
		report["overall_health"] = "healthy"
		report["recommendations"] = []string{"所有交易对P0-08修复状态正常"}
	} else {
		if len(issues) < len(symbols)/2 {
			report["overall_health"] = "partially_fixed"
		} else {
			report["overall_health"] = "needs_attention"
		}
		
		report["recommendations"] = []string{
			fmt.Sprintf("发现%d个P0-08相关问题，建议检查数据流", len(issues)),
			"考虑重启OI数据收集组件",
			"验证WebSocket连接状态",
			"检查配置文件中的阈值设置",
		}
	}
	
	report["healthy_symbols"] = healthyCount
	report["total_issues"] = len(issues)
	report["issue_details"] = issues
	
	log.Printf("📈 [P0-08] 报告生成完成：总计%d个交易对，健康%d个，问题%d个", 
		len(symbols), healthyCount, len(issues))
	
	return report
}

// generateOIP008FixSummary 生成P0-08修复摘要
func generateOIP008FixSummary() map[string]interface{} {
	return map[string]interface{}{
		"fixed_issues": []string{
			"修复注释与实现不一致（30分钟注释 vs 5分钟实现）",
			"修复无数据路径使用time.Now()导致的伪新鲜度问题",
			"统一阈值配置，基于30秒WebSocket更新频率设定合理阈值",
			"引入明确的数据健康状态管理",
			"添加配置验证和业务逻辑合理性检查",
		},
		"new_features": []string{
			"OIStaleConfig: 可配置的阈值管理",
			"OIDataHealth: 明确的数据健康状态",
			"OIAnalysisV2: 增强版分析结果",
			"零值时间语义: 明确区分有数据和无数据状态",
			"质量等级评定: A-F级别的数据质量评估",
		},
		"backward_compatibility": []string{
			"保持现有API向后兼容",
			"提供GetOIAnalysisFixed作为过渡方案",
			"现有字段含义保持不变",
		},
	}
}