# Debug Mode Usage

## 启用调试模式

为了减少生产环境的日志输出量，系统默认关闭详细的调试日志。

### 启用所有调试日志

设置环境变量来启用详细的调试日志：

```bash
# Linux/macOS
export NOFX_DEBUG=true
go run main.go

# Windows
set NOFX_DEBUG=true
go run main.go

# 或者一次性运行
NOFX_DEBUG=true go run main.go
```

### 调试日志包含的内容

启用调试模式后，系统将输出：

1. **CVD计算详情** (`cvd_calculator.go`)
   - P0-1修复后的成交量比率计算过程
   - CVD增量计算的详细数据

2. **AI请求详情** (`mcp/client.go`)
   - HTTP请求耗时统计
   - 详细的API请求参数
   - JSON请求体大小信息

3. **订单流分析** (`orderflow_manager.go`)
   - P1-1 CVD背离检测的触发日志

### 生产环境建议

- **生产环境**: 不设置 `NOFX_DEBUG` 环境变量，保持日志简洁
- **开发调试**: 设置 `NOFX_DEBUG=true` 获取详细日志
- **问题排查**: 临时启用调试模式来定位问题

### 当前已优化的日志

系统已经优化了以下冗余日志输出：
- ✅ CVD比率计算过程日志
- ✅ AI请求耗时统计日志
- ✅ 详细API请求参数日志
- ✅ CVD背离检测触发日志

这样既保证了生产环境的日志清洁，又保留了调试时的详细信息。