package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"../market"
)

// APIResponse 统一API响应格式
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// 处理多币种分析请求
func handleMultiSymbolAnalysis(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// 获取查询参数中的symbols
	symbolsParam := r.URL.Query().Get("symbols")
	if symbolsParam == "" {
		response := APIResponse{
			Success: false,
			Error:   "缺少symbols参数，例如: ?symbols=BTC,ETH,BNB",
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 解析symbols
	symbols := strings.Split(symbolsParam, ",")
	for i := range symbols {
		symbols[i] = strings.TrimSpace(symbols[i])
	}
	
	// 获取分析数据
	data, err := market.GetMultiSymbolAnalysis(symbols)
	if err != nil {
		response := APIResponse{
			Success: false,
			Error:   fmt.Sprintf("获取分析数据失败: %v", err),
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	response := APIResponse{
		Success: true,
		Data:    data,
	}
	json.NewEncoder(w).Encode(response)
}

// 处理单币种分析请求
func handleSingleSymbolAnalysis(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// 获取路径参数中的symbol
	symbol := strings.TrimPrefix(r.URL.Path, "/api/analysis/symbol/")
	if symbol == "" {
		response := APIResponse{
			Success: false,
			Error:   "缺少symbol参数",
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 获取分析数据
	data, err := market.GetSingleSymbolAnalysis(symbol)
	if err != nil {
		response := APIResponse{
			Success: false,
			Error:   fmt.Sprintf("获取%s分析数据失败: %v", symbol, err),
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	response := APIResponse{
		Success: true,
		Data:    data,
	}
	json.NewEncoder(w).Encode(response)
}

// 处理特定时间框架分析请求
func handleSpecificTimeframeAnalysis(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// 解析路径参数
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/analysis/"), "/")
	if len(pathParts) < 3 {
		response := APIResponse{
			Success: false,
			Error:   "路径格式错误，应为: /api/analysis/{symbol}/{timeframe}/{analysis_type}",
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	symbol := pathParts[0]
	timeframe := pathParts[1]
	analysisType := pathParts[2]
	
	// 获取单币种数据
	symbolData, err := market.GetSingleSymbolAnalysis(symbol)
	if err != nil {
		response := APIResponse{
			Success: false,
			Error:   fmt.Sprintf("获取%s分析数据失败: %v", symbol, err),
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 提取特定时间框架和分析类型的数据
	var result interface{}
	if tfData, exists := symbolData[timeframe].(map[string]interface{}); exists {
		// 转换分析类型名称
		var analysisKey string
		switch analysisType {
		case "dow", "dow_theory":
			analysisKey = "道氏理论数据"
		case "channel", "channel_analysis":
			analysisKey = "通道数据"
		case "vpvr", "volume_profile":
			analysisKey = "VPVR数据"
		case "supply_demand", "sd":
			analysisKey = "供需区数据"
		case "fvg", "fair_value_gaps":
			analysisKey = "FVG数据"
		case "fibonacci", "fib":
			analysisKey = "斐波纳契数据"
		default:
			response := APIResponse{
				Success: false,
				Error:   "不支持的分析类型: " + analysisType,
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		
		result = tfData[analysisKey]
	} else {
		response := APIResponse{
			Success: false,
			Error:   "不支持的时间框架: " + timeframe,
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	response := APIResponse{
		Success: true,
		Data:    result,
	}
	json.NewEncoder(w).Encode(response)
}

func main() {
	// 设置路由
	http.HandleFunc("/api/analysis/multi", handleMultiSymbolAnalysis)
	http.HandleFunc("/api/analysis/symbol/", handleSingleSymbolAnalysis)
	http.HandleFunc("/api/analysis/", handleSpecificTimeframeAnalysis)
	
	// 健康检查
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	
	// 启动服务器
	port := ":8080"
	fmt.Printf("服务器启动在端口%s\n", port)
	fmt.Println("API端点:")
	fmt.Println("1. GET /api/analysis/multi?symbols=BTC,ETH,BNB - 获取多币种分析")
	fmt.Println("2. GET /api/analysis/symbol/BTC - 获取单币种分析")
	fmt.Println("3. GET /api/analysis/BTC/3m/dow - 获取特定分析数据")
	fmt.Println("4. GET /health - 健康检查")
	
	log.Fatal(http.ListenAndServe(port, nil))
}