# AI返回JSON格式测试用例

## 1. 标准格式（基础Decision结构）
```json
[
  {
    "symbol": "BTCUSDT",
    "action": "open_long",
    "leverage": 5,
    "position_size_usd": 500.0,
    "stop_loss": 95000.0,
    "take_profit": 100000.0,
    "confidence": 85,
    "risk_usd": 25.0,
    "reasoning": "技术指标显示上涨趋势"
  }
]
```

## 2. Taro模板格式
```json
{
  "analysis": {
    "symbol": "BTCUSDT",
    "mtf_view": "BULL",
    "consensus": "STRONG_BUY",
    "notes": "多时间框架看涨"
  },
  "actions": [
    {
      "type": "OPEN",
      "decision": "OPEN",
      "side": "LONG",
      "qty": "0.001",
      "entry": "95000.0",
      "stop": "94000.0",
      "take_profit_hint": "96500.0",
      "reason": "突破阻力位"
    }
  ]
}
```

## 3. 嵌套entry_plan格式（最新发现）
```json
[
  {
    "symbol": "BTCUSDT",
    "side": "SHORT", 
    "type": "OPEN",
    "entry_plan": {
      "price": 95785.8,
      "qty": 0.00096,
      "leverage": 3,
      "init_stop": 97000.0,
      "take_profit": null,
      "entry_order_policy": "STOP_ONLY_AT_ENTRY"
    },
    "stop_update": {
      "prev_stop": null,
      "new_stop": 97000.0,
      "reason": "initial"
    },
    "order_flags": {
      "reduce_only": true,
      "close_on_trigger": true,
      "trigger_type": "MARK_PRICE"
    },
    "r_multiple_locked": 0.0,
    "notes": "基于多时间框架分析的空头信号"
  }
]
```

## 4. 混合格式（字段类型不匹配）
```json
[
  {
    "symbol": "BTCUSDT",
    "action": "open_long",
    "leverage": "5",  // 字符串而不是数字
    "position_size_usd": 500.0,
    "stop_loss": "95000.0",  // 字符串而不是数字
    "take_profit": [96000.0, 97000.0],  // 数组而不是单个数字
    "confidence": 85,
    "risk_usd": 25.0,
    "reasoning": "技术指标显示上涨趋势"
  }
]
```

## 5. 复杂AI格式（完整结构）
```json
[
  {
    "symbol": "BTCUSDT",
    "open": true,
    "side": "long",
    "playbook": "trend_following",
    "entry": {
      "type": "market",
      "price": 95000.0,
      "tolerance": 0.1
    },
    "stop_loss": 94000.0,
    "take_profit": [96000.0, 97000.0],
    "min_rr": 3.0,
    "confluence_score": 0.85,
    "confidence": 85,
    "positioning": {
      "risk_per_trade": 0.02,
      "leverage_hint": 5,
      "size_safeguard": "max_1x_equity"
    },
    "routing": {
      "post_only": false,
      "time_in_force": "GTC"
    },
    "reason": "多重技术指标汇聚",
    "insufficient_data": []
  }
]
```

## 6. 等待/观望格式
```json
[
  {
    "symbol": "BTCUSDT",
    "action": "wait",
    "reasoning": "市场方向不明，等待明确信号"
  },
  {
    "symbol": "ETHUSDT", 
    "type": "WAIT",
    "side": "LONG",
    "reasoning": "技术指标分歧，暂时观望"
  }
]
```

## 7. 字段名变体汇总

| 标准字段 | 变体1 | 变体2 | 变体3 | 说明 |
|---------|-------|-------|-------|------|
| action | type | decision | - | 动作类型 |
| leverage | - | - | entry_plan.leverage | 杠杆倍数 |
| position_size_usd | qty | entry_plan.qty | position_size | 仓位大小 |
| stop_loss | stop | init_stop | entry_plan.init_stop | 止损价格 |
| take_profit | tp | take_profit_hint | entry_plan.take_profit | 止盈价格 |
| reasoning | reason | notes | entry_plan.reason | 理由说明 |

## 8. 解析器优先级策略

1. **模板检测**：根据templateName选择对应解析器
2. **格式尝试顺序**���
   - Taro模板 → Taro格式 → 标准格式 → 混合格式
   - 其他模板 → 标准格式 → Taro格式 → 混合格式
3. **增强处理**：使用enhanceDecisionsWithTaroFields处理字段映射
4. **兜底机制**：parseComplexAIDecisions处理复杂结构

## 9. 常见问题和解决方案

### 问题1：杠杆为0导致验证失败
- **原因**：杠杆信息在嵌套对象中（如entry_plan.leverage）
- **解决**：增强函数提取嵌套字段

### 问题2：Action字段为空
- **原因**：AI使用type或decision字段代替action
- **解决**：字段映射和转换

### 问题3：数字字段为字符串
- **原因**：JSON序列化时类型不一致
- **解决**：类型转换处理

### 问题4：take_profit为数组
- **原因**：AI返回多个止盈位
- **解决**：取第一个值或处理数组

## 10. 测试验证建议

1. **单元测试**：每种格式创建独立测试用例
2. **集成测试**：模拟完整解析流程
3. **回归测试**：确保新修改不影响旧格式
4. **压力测试**：处理异常和边界情况