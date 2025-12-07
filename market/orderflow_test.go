package market

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// TestOrderFlowDataV2Output 测试V2版本的订单流数据输出
func TestOrderFlowDataV2Output(symbol string) {
	log.Printf("🧪 开始测试 %s 的V2订单流数据输出...", symbol)
	
	// 获取数据
	result := GetOrderFlowDataForAIV2(symbol)
	
	// 输出原始数据用于检查
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Printf("❌ JSON序列化失败: %v", err)
		return
	}
	
	log.Printf("📋 %s 原始数据输出:", symbol)
	fmt.Println(string(jsonData))
	
	// 分析数据质量
	analyzeDataQuality(result, symbol)
}

// analyzeDataQuality 分析数据质量和有效性
func analyzeDataQuality(result map[string]interface{}, symbol string) {
	log.Printf("\n🔍 开始分析 %s 的数据质量...", symbol)
	
	var issues []string
	var validData []string
	
	// 检查整体状态
	if status, ok := result["状态"]; ok {
		log.Printf("📊 系统状态: %v", status)
		if status == "订单流系统未初始化" {
			issues = append(issues, "订单流系统未初始化")
		} else if status == "暂无订单流数据" {
			issues = append(issues, "暂无订单流数据")
		} else if status == "数据过期" {
			issues = append(issues, "数据过期但结构完整")
		}
	}
	
	// 检查本周期博弈_5m
	if periodData, ok := result["本周期博弈_5m"].(map[string]interface{}); ok {
		log.Printf("🎯 分析本周期博弈_5m...")
		
		if candleIntent, exists := periodData["candle_intent"]; exists {
			if candleIntent == "data_insufficient" {
				issues = append(issues, "5分钟K线意图数据不足")
			} else {
				validData = append(validData, fmt.Sprintf("K线意图: %v", candleIntent))
			}
		}
		
		if dataQuality, exists := periodData["data_quality"]; exists {
			if quality, ok := dataQuality.(float64); ok && quality > 0.5 {
				validData = append(validData, fmt.Sprintf("数据质量良好: %.2f", quality))
			} else {
				issues = append(issues, fmt.Sprintf("数据质量偏低: %v", dataQuality))
			}
		}
		
		// 检查CVD数据
		if spotCVD, exists := periodData["spot_cvd_delta_usd"]; exists {
			if cvd, ok := spotCVD.(float64); ok && cvd != 0 {
				validData = append(validData, fmt.Sprintf("现货CVD有效: %.0f", cvd))
			}
		}
		
		if futuresCVD, exists := periodData["futures_cvd_delta_usd"]; exists {
			if cvd, ok := futuresCVD.(float64); ok && cvd != 0 {
				validData = append(validData, fmt.Sprintf("期货CVD有效: %.0f", cvd))
			}
		}
	}
	
	// 检查宏观资金趋势
	if macroData, ok := result["宏观资金趋势"].(map[string]interface{}); ok {
		log.Printf("🌍 分析宏观资金趋势...")
		
		if trendAlignment, exists := macroData["trend_alignment"]; exists {
			if trendAlignment != "unknown" {
				validData = append(validData, fmt.Sprintf("趋势一致性: %v", trendAlignment))
			}
		}
		
		if marketRegime, exists := macroData["market_regime"]; exists {
			if marketRegime != "unknown" {
				validData = append(validData, fmt.Sprintf("市场状态: %v", marketRegime))
			}
		}
		
		if signalStrength, exists := macroData["signal_strength"]; exists {
			if strength, ok := signalStrength.(float64); ok && strength > 0.1 {
				validData = append(validData, fmt.Sprintf("信号强度有效: %.2f", strength))
			}
		}
	}
	
	// 检查盘口结构_v2
	if orderbookData, ok := result["盘口结构_v2"].(map[string]interface{}); ok {
		log.Printf("📊 分析盘口结构_v2...")
		
		if spoofingRisk, exists := orderbookData["spoofing_risk"]; exists {
			if risk, ok := spoofingRisk.(float64); ok {
				if risk > 0.7 {
					issues = append(issues, fmt.Sprintf("高虚假挂单风险: %.2f", risk))
				} else {
					validData = append(validData, fmt.Sprintf("虚假挂单风险正常: %.2f", risk))
				}
			}
		}
		
		// 检查支撑/阻力墙
		if resistanceWall, exists := orderbookData["resistance_wall"]; exists && resistanceWall != nil {
			if wall, ok := resistanceWall.(map[string]interface{}); ok {
				if stabilityScore, exists := wall["stability_score"]; exists {
					if score, ok := stabilityScore.(float64); ok && score > 0.8 {
						validData = append(validData, fmt.Sprintf("阻力墙稳定: %.2f", score))
					}
				}
			}
		}
		
		if supportWall, exists := orderbookData["support_wall"]; exists && supportWall != nil {
			if wall, ok := supportWall.(map[string]interface{}); ok {
				if stabilityScore, exists := wall["stability_score"]; exists {
					if score, ok := stabilityScore.(float64); ok && score > 0.8 {
						validData = append(validData, fmt.Sprintf("支撑墙稳定: %.2f", score))
					}
				}
			}
		}
	}
	
	// 检查数据质量
	if qualityData, ok := result["数据质量"].(map[string]interface{}); ok {
		log.Printf("🔧 分析数据质量...")
		
		if overallScore, exists := qualityData["overall_score"]; exists {
			if score, ok := overallScore.(float64); ok {
				if score > 0.8 {
					validData = append(validData, fmt.Sprintf("整体质量优秀: %.2f", score))
				} else if score > 0.5 {
					validData = append(validData, fmt.Sprintf("整体质量良好: %.2f", score))
				} else {
					issues = append(issues, fmt.Sprintf("整体质量较差: %.2f", score))
				}
			}
		}
		
		if status, exists := qualityData["status"]; exists {
			if status == "正常" {
				validData = append(validData, "数据状态正常")
			} else {
				issues = append(issues, fmt.Sprintf("数据状态异常: %v", status))
			}
		}
	}
	
	// 输出分析结果
	log.Printf("\n✅ 有效数据项 (%d个):", len(validData))
	for _, data := range validData {
		log.Printf("  ✓ %s", data)
	}
	
	log.Printf("\n⚠️ 问题项 (%d个):", len(issues))
	for _, issue := range issues {
		log.Printf("  ⚠️ %s", issue)
	}
	
	// 给出建议
	if len(validData) > len(issues) {
		log.Printf("\n🎉 结论: 数据质量良好，可用于AI分析")
	} else if len(validData) > 0 {
		log.Printf("\n⚡ 结论: 数据部分有效，建议谨慎使用")
	} else {
		log.Printf("\n❌ 结论: 数据质量不足，不建议进行交易分析")
	}
}

// TestMultipleSymbols 测试多个币种的数据输出
func TestMultipleSymbols() {
	symbols := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	
	log.Printf("🔄 开始测试多个币种的订单流数据...")
	
	for _, symbol := range symbols {
		log.Printf("\n" + "="*50)
		TestOrderFlowDataV2Output(symbol)
		time.Sleep(1 * time.Second) // 避免请求过快
	}
}