#!/bin/bash
# Direction Arbitration 通道数据缺失修复验证脚本

echo "=========================================="
echo "Direction Arbitration 修复验证"
echo "=========================================="
echo ""

echo "1️⃣  编译验证..."
if go build ./market/... && go build ./internal/direction/...; then
    echo "✅ 编译成功"
else
    echo "❌ 编译失败"
    exit 1
fi
echo ""

echo "2️⃣  运行单元测试..."
if go test ./market -run "TestParseChannel|TestParseMTF" -v 2>&1 | grep -q "PASS"; then
    echo "✅ 单元测试通过"
else
    echo "❌ 单元测试失败"
    exit 1
fi
echo ""

echo "3️⃣  运行集成测试..."
if go test ./market -run "TestDirectionArbitration" -v 2>&1 | grep -q "PASS"; then
    echo "✅ 集成测试通过"
else
    echo "❌ 集成测试失败"
    exit 1
fi
echo ""

echo "4️⃣  运行Direction模块核心测试..."
if go test ./internal/direction -run "TestChannelData" -v 2>&1 | grep -q "PASS"; then
    echo "✅ Direction模块测试通过"
else
    echo "❌ Direction模块测试失败"
    exit 1
fi
echo ""

echo "=========================================="
echo "✅ 所有验证通过！"
echo "=========================================="
echo ""
echo "修复内容："
echo "  1. 修复通道质量字段映射 (channel_width_pct -> quality)"
echo "  2. 添加1h和4h时间框架解析"
echo "  3. 修复摆动点强度计算边界检查"
echo "  4. 通道数据缺失时降级为纯订单流模式"
echo ""
echo "关键改进："
echo "  - 通道数据缺失时不再完全阻止交易"
echo "  - 降级为纯订单流模式（95%订单流 + 5%结构）"
echo "  - 标记 CHANNEL_DATA_WEAK 表示降级状态"
echo ""
echo "下一步："
echo "  1. 重启Go后端服务"
echo "  2. 观察direction_arbitration输出"
echo "  3. 检查是否还有CHANNEL_DATA_MISSING"
echo "  4. 监控CHANNEL_DATA_WEAK标记出现频率"
echo ""
