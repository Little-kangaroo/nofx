package main

import (
	"encoding/json"
	"fmt"
	"log"

	"../market"
)

func main() {
	// 示例1: 获取多个币种的分析数据
	symbols := []string{"BTC", "ETH", "BNB"}
	
	fmt.Println("=== 获取多个币种的多时间框架分析数据 ===")
	multiSymbolData, err := market.GetMultiSymbolAnalysis(symbols)
	if err != nil {
		log.Fatalf("获取多币种分析数据失败: %v", err)
	}
	
	// 输出JSON格式数据
	jsonData, err := json.MarshalIndent(multiSymbolData, "", "  ")
	if err != nil {
		log.Fatalf("JSON序列化失败: %v", err)
	}
	
	fmt.Printf("多币种分析数据:\n%s\n\n", jsonData)
	
	// 示例2: 获取单个币种的分析数据
	fmt.Println("=== 获取单个币种的多时间框架分析数据 ===")
	singleSymbolData, err := market.GetSingleSymbolAnalysis("BTC")
	if err != nil {
		log.Fatalf("获取单币种分析数据失败: %v", err)
	}
	
	jsonData2, err := json.MarshalIndent(singleSymbolData, "", "  ")
	if err != nil {
		log.Fatalf("JSON序列化失败: %v", err)
	}
	
	fmt.Printf("BTCUSDT分析数据:\n%s\n\n", jsonData2)
	
	// 示例3: 访问特定数据
	fmt.Println("=== 访问特定时间框架和分析类型的数据 ===")
	
	// 访问BTCUSDT的3分钟道氏理论数据
	if btcData, exists := multiSymbolData["BTCUSDT"]; exists {
		if tf3m, exists := btcData["3m"].(map[string]interface{}); exists {
			if dowData := tf3m["道氏理论数据"]; dowData != nil {
				dowJson, _ := json.MarshalIndent(dowData, "", "  ")
				fmt.Printf("BTCUSDT 3分钟道氏理论数据:\n%s\n\n", dowJson)
			}
		}
	}
	
	// 访问ETHUSDT的15分钟VPVR数据
	if ethData, exists := multiSymbolData["ETHUSDT"]; exists {
		if tf15m, exists := ethData["15m"].(map[string]interface{}); exists {
			if vpvrData := tf15m["VPVR数据"]; vpvrData != nil {
				vpvrJson, _ := json.MarshalIndent(vpvrData, "", "  ")
				fmt.Printf("ETHUSDT 15分钟VPVR数据:\n%s\n\n", vpvrJson)
			}
		}
	}
}