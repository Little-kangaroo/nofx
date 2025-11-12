package main

import (
	"encoding/json"
	"fmt"
	"log"

	"nofx/market"
)

func main() {
	fmt.Println("=== 测试新的结构化数据格式 ===")
	
	// 模拟获取BTC的市场数据
	data, err := market.Get("BTC")
	if err != nil {
		log.Fatalf("获取BTC市场数据失败: %v", err)
	}
	
	fmt.Println("1. 原始格式输出（前500字符）:")
	originalFormat := market.Format(data)
	if len(originalFormat) > 500 {
		fmt.Printf("%s...\n\n", originalFormat[:500])
	} else {
		fmt.Printf("%s\n\n", originalFormat)
	}
	
	fmt.Println("2. 新的结构化格式输出:")
	structuredFormat := market.FormatAsStructuredData(data)
	fmt.Printf("%s\n\n", structuredFormat)
	
	fmt.Println("3. 验证JSON格式有效性:")
	var jsonTest interface{}
	err = json.Unmarshal([]byte(structuredFormat), &jsonTest)
	if err != nil {
		fmt.Printf("❌ JSON格式无效: %v\n", err)
	} else {
		fmt.Printf("✅ JSON格式有效\n")
	}
	
	fmt.Println("\n4. 测试多币种格式:")
	multiData, err := market.GetMultiSymbolAnalysis([]string{"BTC", "ETH"})
	if err != nil {
		log.Printf("获取多币种数据失败: %v", err)
	} else {
		jsonData, _ := json.MarshalIndent(multiData, "", "  ")
		// 只显示前1000字符
		if len(jsonData) > 1000 {
			fmt.Printf("%s...\n", jsonData[:1000])
		} else {
			fmt.Printf("%s\n", jsonData)
		}
	}
}