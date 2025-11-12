# 多币种多时间框架分析数据使用说明

## 概述

新增的多币种多时间框架分析功能提供了按币种和时间框架组织的技术分析数据，每个币种包含5个时间框架（3m, 15m, 30m, 1h, 4h），每个时间框架包含6种技术分析类型。

## 📢 重要更新

**交易��输出格式已更新为结构化JSON格式！** 现在交易员会自动输出你想要的数据结构格式，而不是之前的文本格式。

## 数据结构

```json
{
  "BTCUSDT": {
    "3m": {
      "道氏理论数据": {...},
      "通道数据": {...},
      "VPVR数据": {...},
      "供需区数据": {...},
      "FVG数据": {...},
      "斐波纳契数据": {...}
    },
    "15m": {...},
    "30m": {...},
    "1h": {...},
    "4h": {...}
  },
  "ETHUSDT": {...}
}
```

## API函数

### 1. FormatAsStructuredData(data *Data) 

**交易员自动使用的新格式函数**

将单个币种的市场数据格式化为你想要的结构化JSON格式。

**参数:**
- `data`: 市场数据结构

**返回:**
- `string`: JSON格式的结构化数据

**使用场景:**
这个函数现在被交易员自动调用，你会看到类似这样的输出：

```json
{
  "BTCUSDT": {
    "3m": {
      "道氏理论数据": {...},
      "通道数据": {...},
      "VPVR数据": {...},
      "供需区数据": {...},
      "FVG数据": {...},
      "斐波纳契数据": {...}
    },
    ...
  }
}
```

### 2. GetMultiSymbolAnalysis(symbols []string)

获取多个币种的多时间框架分析数据。

**参数:**
- `symbols`: 币种列表，如 `["BTC", "ETH", "BNB"]`

**返回:**
- `map[string]map[string]interface{}`: 币种->时间框架->分析数据的映射
- `error`: 错误信息

**使用示例:**
```go
symbols := []string{"BTC", "ETH", "BNB"}
data, err := market.GetMultiSymbolAnalysis(symbols)
if err != nil {
    log.Fatal(err)
}

// 访问BTCUSDT的3分钟道氏理论数据
btcData := data["BTCUSDT"]
tf3m := btcData["3m"].(map[string]interface{})
dowData := tf3m["道氏理论数据"]
```

### 2. GetSingleSymbolAnalysis(symbol string)

获取单个币种的多时间框架分析数据。

**参数:**
- `symbol`: 币种名称，如 `"BTC"`

**返回:**
- `map[string]interface{}`: 时间框架->分析数据的映射
- `error`: 错误信息

**使用示例:**
```go
data, err := market.GetSingleSymbolAnalysis("BTC")
if err != nil {
    log.Fatal(err)
}

// 访问15分钟VPVR数据
tf15m := data["15m"].(map[string]interface{})
vpvrData := tf15m["VPVR数据"]
```

### 3. extractTimeframeData(analysis, timeframe, analysisType)

内部辅助函数，用于从多时间框架分析中提取特定数据。

## 支持的时间框架

- `3m`: 3分钟
- `15m`: 15分钟
- `30m`: 30分钟
- `1h`: 1小时
- `4h`: 4小时

## 支持的分析类型

- `道氏理论数据`: 道氏理论分析
- `通道数据`: 通道分析
- `VPVR数据`: 成交量分布分析
- `供需区数据`: 供需区分析
- `FVG数据`: 公平价值缺口分析
- `斐波纳契数据`: 斐波纳契分析

## Web API 使用示例

启动API服务器后，可以通过以下端点访问数据：

### 1. 获取多币种分析数据
```
GET /api/analysis/multi?symbols=BTC,ETH,BNB
```

### 2. 获取单币种分析数据
```
GET /api/analysis/symbol/BTC
```

### 3. 获取特定分析数据
```
GET /api/analysis/BTC/3m/dow          # BTC 3分钟道氏理论数据
GET /api/analysis/ETH/15m/vpvr        # ETH 15分钟VPVR数据
GET /api/analysis/BNB/1h/fvg          # BNB 1小时FVG数据
```

## 分析类型别名

Web API支持以下分析类型别名：
- `dow`, `dow_theory` -> 道氏理论数据
- `channel`, `channel_analysis` -> 通道数据
- `vpvr`, `volume_profile` -> VPVR数据
- `supply_demand`, `sd` -> 供需区数据
- `fvg`, `fair_value_gaps` -> FVG数据
- `fibonacci`, `fib` -> 斐波纳契数据

## 完整示例

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "your_project/market"
)

func main() {
    // 获取多币种数据
    symbols := []string{"BTC", "ETH"}
    multiData, err := market.GetMultiSymbolAnalysis(symbols)
    if err != nil {
        log.Fatal(err)
    }
    
    // 输出为JSON
    jsonData, _ := json.MarshalIndent(multiData, "", "  ")
    fmt.Printf("多币种分析数据:\n%s\n", jsonData)
    
    // 访问特定数据
    if btcData, exists := multiData["BTCUSDT"]; exists {
        if tf3m, exists := btcData["3m"].(map[string]interface{}); exists {
            dowData := tf3m["道氏理论数据"]
            fmt.Printf("BTC 3分钟道氏理论数据: %+v\n", dowData)
        }
    }
}
```

## 注意事项

1. 币种名称会自动标准化为USDT交易对（如BTC -> BTCUSDT）
2. 如果某个时间框架的数据不存在，会返回空的map[string]interface{}{}
3. 所有分析都基于300根K线数据
4. 数据获取失败的币种会被跳过，不会影响其他币种的数据获取
5. 建议在生产环境中添加缓存机制以提高性能

## 性能建议

1. **批量获取**: 使用`GetMultiSymbolAnalysis`比多次调用`GetSingleSymbolAnalysis`更高效
2. **缓存策略**: 考虑对结果进行缓存，避免频繁计算
3. **异步处理**: 对于大量币种，可以考虑并发处理
4. **数据过滤**: 根据需要只请求必要的币种和时间框架