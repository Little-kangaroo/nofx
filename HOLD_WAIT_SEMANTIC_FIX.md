# HOLD vs WAIT语义修复实现

## 问题背景

用户反馈AI在代码中对HOLD和WAIT的解析存在问题：
- **WAIT**: 应该用于**没有持仓**的标的（观望，不持仓）
- **HOLD**: 应该用于**有现有持仓**的标的（持有当前仓位）

但AI有时会在没有持仓的标的上返回HOLD动作，这在语义上是错误的。

## 解决方案

### 1. 核心实现

在`decision/engine.go`中实现了位置感知的语义验证：

```go
// validateHoldWaitSemantics 验证HOLD vs WAIT动作的语义正确性
func validateHoldWaitSemantics(d *Decision, ctx *Context) error {
    // 检查该标的是否有持仓
    hasPosition := false
    var existingPosition *PositionInfo
    
    for i := range ctx.Positions {
        if ctx.Positions[i].Symbol == d.Symbol {
            hasPosition = true
            existingPosition = &ctx.Positions[i]
            break
        }
    }

    action := strings.ToLower(d.Action)

    // HOLD vs WAIT语义验证
    if action == "hold" {
        if !hasPosition {
            return fmt.Errorf("语义错误: 标的 %s 当前没有持仓，应使用 WAIT 而不是 HOLD", d.Symbol)
        }
        log.Printf("✅ [HOLD语义验证] %s 有持仓(%s %.6f)，使用HOLD正确", 
            d.Symbol, existingPosition.Side, existingPosition.Quantity)
    } else if action == "wait" {
        if hasPosition {
            return fmt.Errorf("语义错误: 标的 %s 当前有持仓(%s %.6f)，应使用 HOLD 而不是 WAIT", 
                d.Symbol, existingPosition.Side, existingPosition.Quantity)
        }
        log.Printf("✅ [WAIT语义验证] %s 无持仓，使用WAIT正确", d.Symbol)
    }

    return nil
}
```

### 2. 模板特定验证

- 只对使用`taro_long_prompts`模板的AI决策进行HOLD vs WAIT语义验证
- 保持其他模板的兼容性

### 3. 验证流程

1. **基础验证**: 先通过原有的`validateDecision`函数进行基础参数验证
2. **语义验证**: 对于taro模板，额外进行HOLD vs WAIT语义验证
3. **错误处理**: 如果语义不正确，返回详细错误信息指导正确使用

### 4. 验证规则

- **HOLD动作**: 只能用于当前有持仓的标的
- **WAIT动作**: 只能用于当前没有持仓的标的
- **错误提示**: 提供明确的错误信息，说明应该使用哪个动作

## 技术细节

### 修改文件
- `decision/engine.go`: 添加语义验证逻辑

### 新增函数
- `parseFullDecisionResponseWithContext()`: 包含上下文的决策解析
- `validateDecisionsWithContext()`: 包含上下文的决策验证
- `validateDecisionWithContext()`: 包含上下文的单个决策验证
- `validateHoldWaitSemantics()`: HOLD vs WAIT语义验证核心逻辑

### 保持兼容性
- 原有的`parseFullDecisionResponse()`等函数保持不变
- 不影响其他模板的决策验证流程

## 使用效果

### Before (问题场景)
```
标的: BTCUSDT, 当前持仓: 无
AI决策: {"action": "hold", "symbol": "BTCUSDT"}
结果: 语义错误但通过验证 ❌
```

### After (修复后)
```
标的: BTCUSDT, 当前持仓: 无
AI决策: {"action": "hold", "symbol": "BTCUSDT"}
结果: 验证失败 - "语义错误: 标的 BTCUSDT 当前没有持仓，应使用 WAIT 而不是 HOLD" ✅
```

## 日志输出

成功验证时会产生详细日志：
- `✅ [HOLD语义验证] BTCUSDT 有持仓(long 0.001000)，使用HOLD正确`
- `✅ [WAIT语义验证] ETHUSDT 无持仓，使用WAIT正确`

验证失败时会产生明确错误信息，帮助理解正确的语义使用方式。
