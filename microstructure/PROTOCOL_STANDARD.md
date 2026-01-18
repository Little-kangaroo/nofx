# AI输出协议标准 (T11)

## 协议声明

**主协议**: V-12.2 订单流分析系统
**实现**: `AIContextV12` (ai_interface_v12.go)
**状态**: ✅ 生产环境主路径

**兜底协议**: V2.0_FALLBACK
**实现**: `AIPayloadStandard` with fallback mode (ai_payload_standard.go)
**状态**: ✅ 字段结构已标准化，与V12.2对齐

## 协议层次结构

```
统一输出接口: AIPayloadStandard
├── 主路径 (NORMAL模式)
│   ├── 协议: V-12.2
│   ├── 实现: AIContextV12.GenerateAIContextFromSnapshot()
│   ├── 格式: contextV12.ToAIPromptFormat()
│   └── 特性: 完整的统计增强、动态阈值、哨兵监控
│
└── 兜底路径 (FALLBACK模式)
    ├── 协议: V2.0_FALLBACK
    ├── 实现: generateFallbackContentWithStandardFields()
    ├── 格式: 与V12.2字段结构对齐，但功能简化
    └── 特性: 基础分析，无统计增强
```

## 主协议特性 (V-12.2)

### 核心模块
1. **市场微观结构** (市场微观结构)
   - CVD增量分析 (Z-Score标准化、统计排名、异常评分)
   - 本周期博弈_5m (K线意图推断)
   - 成交量分析 (Volume Ratio)
   - 异常检测

2. **盘口深度分析** (盘口深度分析)
   - 失衡比例 (imbalance_ratio)
   - 阻力/支撑墙分析 (带稳定性评分)
   - 流动性分析
   - 操纵风险评估

3. **统计学上下文** (统计学上下文)
   - 数据质量评估 (CVD/OI/OrderBook可靠性)
   - 系统健康度
   - 置信度校准

4. **动态阈值系统**
   - 市场状态识别 (波动性、趋势强度、流动性)
   - 自适应阈值调整

5. **哨兵监控系统**
   - 威胁等级评估
   - 场景检测 (鲸鱼进场、假突破等)
   - 警报统计

6. **AI决策支持**
   - 综合信号强度
   - 风险评估
   - 市场时机评估
   - 置信度校准

### 字段映射标准
所有字段映射定义在 `StandardizedFieldMapping` (ai_payload_standard.go:34-83)

## 兜底协议特性 (V2.0_FALLBACK)

### 设计原则
1. **字段结构对齐**: 与V12.2使用相同的字段名和结构
2. **功能降级**: 禁用统计增强、动态阈值、哨兵监控
3. **明确标记**: 所有模块标记 "系统状态": "兜底模式"
4. **质量降级**: 强制 QualityScore *= 0.5，标记 "DEGRADED"

### 激活条件
1. `GetGlobalAIInterfaceV12()` 返回 nil
2. AI接口可用但 `GenerateAIContextFromSnapshot()` 失败

### 兜底模块实现
- `generateFallbackContentWithStandardFields()` - 主入口
- `generateFallbackDataQuality()` - 数据质量(降级30-50%)
- `generateFallbackCVDAnalysis()` - CVD分析(无Z-Score)
- `generateFallbackOrderBookAnalysis()` - 盘口分析(基础版)
- `generateFallbackMacroTrend()` - 宏观趋势(基础版)

## 系统状态判定 (T11标准)

### 质量状态枚举
```go
QualityStatus: "NORMAL"    // AI接口正常，数据质量 >= 0.6
QualityStatus: "DEGRADED"  // AI接口正常但数据质量 0.3-0.6，或兜底模式
QualityStatus: "FATAL"     // AI接口不可用，或数据质量 < 0.3
```

### 系统模式枚举
```go
SystemMode: "NORMAL"       // AI接口可用，主路径
SystemMode: "FALLBACK"     // AI接口不可用或生成失败，兜底路径
SystemMode: "FATAL"        // 严重错误，MarketSnapshot为nil
```

## 协议版本号约定

| 版本号 | 含义 | 使用场景 |
|--------|------|----------|
| `V-12.2` | 主协议版本号 | 正常运行 |
| `v12.2-fallback` | 兜底协议版本号 | AI接口降级 |
| `T02-fixed` | 修复标识 | 禁止二次取样修复 |

## 使用指南

### 推荐使用方式
```go
// ✅ 推荐：使用统一接口
payload := GenerateStandardizedAIPayload(marketSnapshot)

// payload.CompatMode 可能是:
// - "V12.2" (主路径)
// - "V2.0_FALLBACK" (兜底路径)

// payload.QualityStatus 可能是:
// - "NORMAL" (正常)
// - "DEGRADED" (降级)
// - "FATAL" (严重)
```

### 监控与日志
```go
// 检测系统降级
if payload.CompatMode == "V2.0_FALLBACK" {
    LogSystemDegradation(payload.SystemStatus, symbol)
    // 输出: 🚨 AI系统降级警告, ⚠️ 兜底模式激活
}
```

## 协议兼容性保证 (T11承诺)

1. **向后兼容**: 兜底路径字段结构与主路径一致
2. **明确标记**: 所有降级状态明确标记，不会误导
3. **质量透明**: QualityScore强制降级，置信度明确
4. **可恢复性**: 系统提供恢复指南 (RecoveryInstructions)

## T11 改进总结

### ✅ 已完成
1. **主协议确认**: V-12.2 为生产环境主协议
2. **字段标准化**: 兜底路径与主路径字段对齐
3. **质量管理**: 明确的降级策略和状态判定
4. **监控日志**: `LogSystemDegradation()` 自动警报

### 🔒 协议冻结策略
- V-12.2 作为主协议，不再引入 V2.0/V3.0 等并行协议
- 所有新功能在 V-12.2 基础上迭代，版本号递增 (V-12.3, V-12.4...)
- 兜底协议仅作为紧急降级，不承载新功能

## 参考文件

| 文件 | 职责 |
|------|------|
| `ai_interface_v12.go` | V-12.2主协议实现 |
| `ai_payload_standard.go` | 统一输出接口与兜底路径 |
| `types.go` | 数据结构定义 |
| `orderflow_manager.go:870-920` | 主路径调用示例 |

---
**最后更新**: T11 任务完成
**协议版本**: V-12.2
**维护状态**: 🟢 活跃维护
