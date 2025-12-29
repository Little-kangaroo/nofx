#!/bin/bash
# P1&P2验收测试分析脚本
# 用途：对比P1&P2修复前后的关键指标变化

echo "================================================"
echo "   P1 & P2修复效果验收分析"
echo "================================================"
echo ""

# 定义颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 定义数据目录
RECORDS_DIR="/Users/guiling/IdeaProjects/nofx/records_new"

# 检查是否有新数据
echo "1️⃣ 检查数据文件..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

OLD_FILES=$(ls -t $RECORDS_DIR/data8[8-9].txt $RECORDS_DIR/data9[0-6].txt 2>/dev/null)
NEW_FILES=$(ls -t $RECORDS_DIR/*.txt 2>/dev/null | head -1)

if [ -z "$NEW_FILES" ]; then
    echo -e "${RED}❌ 未找到决策记录文件${NC}"
    exit 1
fi

echo "基线数据（P0-P2修复前）:"
echo "$OLD_FILES" | head -3 | while read f; do
    echo "  • $(basename $f) ($(stat -f %Sm -t '%Y-%m-%d %H:%M' $f))"
done

echo ""
echo "最新数据:"
echo "  • $(basename $NEW_FILES) ($(stat -f %Sm -t '%Y-%m-%d %H:%M' $NEW_FILES))"
echo ""

# =====================================================
# P1验收：触发器供给优化
# =====================================================
echo ""
echo "================================================"
echo "   P1验收：触发器供给优化"
echo "================================================"
echo ""

echo "2️⃣ 分析触发器类型分布..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

analyze_trigger_distribution() {
    local file=$1
    local label=$2

    if [ ! -f "$file" ]; then
        echo -e "${RED}文件不存在: $file${NC}"
        return
    fi

    total=$(grep -c '"trigger_primary"' "$file" 2>/dev/null || echo 0)
    if [ $total -eq 0 ]; then
        echo -e "${YELLOW}⚠️ $label: 无触发器数据${NC}"
        return
    fi

    # 统计各类触发器
    sfp=$(grep -c '"trigger_primary"[[:space:]]*:[[:space:]]*"SFP"' "$file" 2>/dev/null || echo 0)
    engulf=$(grep -c '"trigger_primary"[[:space:]]*:[[:space:]]*"ENGULF_BULL\|ENGULF_BEAR"' "$file" 2>/dev/null || echo 0)
    ibb=$(grep -c '"trigger_primary"[[:space:]]*:[[:space:]]*"IBB"' "$file" 2>/dev/null || echo 0)
    momo=$(grep -c '"trigger_primary"[[:space:]]*:[[:space:]]*"MOMO_BREAK"' "$file" 2>/dev/null || echo 0)
    edge=$(grep -c '"trigger_primary"[[:space:]]*:[[:space:]]*"EDGE_BULL\|EDGE_BEAR"' "$file" 2>/dev/null || echo 0)
    no_trigger=$(grep -c '"window_state"[[:space:]]*:[[:space:]]*"no_trigger"' "$file" 2>/dev/null || echo 0)

    # 计算百分比
    if [ $total -gt 0 ]; then
        sfp_pct=$((sfp * 100 / total))
        engulf_pct=$((engulf * 100 / total))
        ibb_pct=$((ibb * 100 / total))
        momo_pct=$((momo * 100 / total))
        edge_pct=$((edge * 100 / total))
        no_trigger_pct=$((no_trigger * 100 / total))
    else
        sfp_pct=0
        engulf_pct=0
        ibb_pct=0
        momo_pct=0
        edge_pct=0
        no_trigger_pct=0
    fi

    echo -e "${BLUE}$label:${NC}"
    echo "  总触发器决策: $total"
    echo "  SFP: $sfp (${sfp_pct}%)"
    echo "  ENGULF: $engulf (${engulf_pct}%)"
    echo "  IBB: $ibb (${ibb_pct}%)"
    echo "  MOMO: $momo (${momo_pct}%)"
    if [ $edge -gt 0 ]; then
        echo -e "  ${GREEN}EDGE: $edge (${edge_pct}%) ✅ 新增${NC}"
    else
        echo "  EDGE: $edge (${edge_pct}%)"
    fi
    echo "  NO_TRIGGER: $no_trigger (${no_trigger_pct}%)"
    echo ""
}

# 分析基线数据
for file in $OLD_FILES; do
    if [ -f "$file" ]; then
        analyze_trigger_distribution "$file" "$(basename $file)"
        break  # 只分析第一个基线文件
    fi
done

# 分析最新数据
analyze_trigger_distribution "$NEW_FILES" "$(basename $NEW_FILES) [最新]"

# =====================================================
# P2验收：Gate5 RR优化
# =====================================================
echo ""
echo "================================================"
echo "   P2验收：Gate5 RR优化"
echo "================================================"
echo ""

echo "3️⃣ 分析Gate5通过率..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

analyze_gate5_pass_rate() {
    local file=$1
    local label=$2

    if [ ! -f "$file" ]; then
        echo -e "${RED}文件不存在: $file${NC}"
        return
    fi

    total_decisions=$(grep -c '"final_action"' "$file" 2>/dev/null || echo 0)
    if [ $total_decisions -eq 0 ]; then
        echo -e "${YELLOW}⚠️ $label: 无决策数据${NC}"
        return
    fi

    # Gate5相关统计
    gate5_pass=$(grep -c '"R_EVAL"[[:space:]]*:[[:space:]]*"PASS"' "$file" 2>/dev/null || echo 0)
    gate5_fail=$(grep -c '"R_EVAL"[[:space:]]*:[[:space:]]*"FAIL"' "$file" 2>/dev/null || echo 0)

    # LTF统计
    ltf_normal=$(grep -c '"trade_style"[[:space:]]*:[[:space:]]*"LTF_scalp"' "$file" | grep -v "降级" 2>/dev/null || echo 0)
    ltf_downgrade=$(grep -c "降级 LTF" "$file" 2>/dev/null || echo 0)

    # 最终开仓
    final_open=$(grep -c '"final_action"[[:space:]]*:[[:space:]]*"OPEN"' "$file" 2>/dev/null || echo 0)

    # 计算百分比
    if [ $total_decisions -gt 0 ]; then
        gate5_pass_pct=$((gate5_pass * 100 / total_decisions))
        final_open_pct=$((final_open * 100 / total_decisions))
    else
        gate5_pass_pct=0
        final_open_pct=0
    fi

    echo -e "${BLUE}$label:${NC}"
    echo "  总决策数: $total_decisions"
    echo "  Gate5 PASS: $gate5_pass (${gate5_pass_pct}%)"
    echo "  Gate5 FAIL: $gate5_fail"
    echo "  LTF常规: $ltf_normal"
    echo "  LTF降级: $ltf_downgrade"
    if [ $final_open_pct -ge 10 ]; then
        echo -e "  ${GREEN}最终开仓: $final_open (${final_open_pct}%) ✅ 达标${NC}"
    else
        echo -e "  ${YELLOW}最终开仓: $final_open (${final_open_pct}%) ⚠️ 未达标${NC}"
    fi
    echo ""
}

# 分析基线数据
for file in $OLD_FILES; do
    if [ -f "$file" ]; then
        analyze_gate5_pass_rate "$file" "$(basename $file)"
        break  # 只分析第一个基线文件
    fi
done

# 分析最新数据
analyze_gate5_pass_rate "$NEW_FILES" "$(basename $NEW_FILES) [最新]"

# =====================================================
# 详细指标检查
# =====================================================
echo ""
echo "================================================"
echo "   详细指标检查"
echo "================================================"
echo ""

echo "4️⃣ 检查TP1选择规则应用..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ -f "$NEW_FILES" ]; then
    # 检查是否有使用第二近障碍的情况
    second_obstacle=$(grep -c "使用第二近障碍" "$NEW_FILES" 2>/dev/null || echo 0)
    obstacle_cluster=$(grep -c "OBSTACLE_CLUSTER" "$NEW_FILES" 2>/dev/null || echo 0)

    echo "  OBSTACLE_CLUSTER场景: $obstacle_cluster"
    if [ $second_obstacle -gt 0 ]; then
        echo -e "  ${GREEN}使用第二近障碍: $second_obstacle ✅ P2-01生效${NC}"
    else
        echo "  使用第二近障碍: $second_obstacle"
    fi
    echo ""
fi

echo "5️⃣ 检查R_clean分布..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ -f "$NEW_FILES" ]; then
    # 统计R_clean范围分布
    r_clean_below_03=$(grep -o '"R_clean_eval"[[:space:]]*:[[:space:]]*[0-9.]*' "$NEW_FILES" | awk -F':' '$2 < 0.3' | wc -l | tr -d ' ')
    r_clean_03_05=$(grep -o '"R_clean_eval"[[:space:]]*:[[:space:]]*[0-9.]*' "$NEW_FILES" | awk -F':' '$2 >= 0.3 && $2 < 0.5' | wc -l | tr -d ' ')
    r_clean_05_07=$(grep -o '"R_clean_eval"[[:space:]]*:[[:space:]]*[0-9.]*' "$NEW_FILES" | awk -F':' '$2 >= 0.5 && $2 < 0.7' | wc -l | tr -d ' ')
    r_clean_above_07=$(grep -o '"R_clean_eval"[[:space:]]*:[[:space:]]*[0-9.]*' "$NEW_FILES" | awk -F':' '$2 >= 0.7' | wc -l | tr -d ' ')

    total_r_clean=$((r_clean_below_03 + r_clean_03_05 + r_clean_05_07 + r_clean_above_07))

    if [ $total_r_clean -gt 0 ]; then
        echo "  R_clean < 0.3: $r_clean_below_03 (应被FAIL)"
        echo "  R_clean [0.3, 0.5): $r_clean_03_05 (降级LTF区间)"
        echo "  R_clean [0.5, 0.7): $r_clean_05_07 (常规LTF区间)"
        echo "  R_clean >= 0.7: $r_clean_above_07 (高质量)"

        if [ $r_clean_03_05 -gt 0 ]; then
            echo -e "  ${GREEN}✅ P2-04生效：存在[0.3,0.5)降级LTF通过案例${NC}"
        fi
    else
        echo -e "  ${YELLOW}⚠️ 暂无R_clean数据${NC}"
    fi
    echo ""
fi

# =====================================================
# 验收标准检查
# =====================================================
echo ""
echo "================================================"
echo "   P1 & P2修复验收标准"
echo "================================================"
echo ""

echo -e "${BLUE}P1验收标准:${NC}"
echo "  ✅ EDGE触发器占比 > 15%"
echo "  ✅ NO_TRIGGER降低 > 15%"
echo "  ✅ 触发器类型分布更均衡"
echo ""

echo -e "${BLUE}P2验收标准:${NC}"
echo "  ✅ Gate5通过率提升 > 25%"
echo "  ✅ 最终开仓率 > 10%（目标12-18%）"
echo "  ✅ 降级LTF占比合理（<30%）"
echo "  ✅ 第二近障碍应用场景存在"
echo ""

echo "================================================"
echo "   使用说明"
echo "================================================"
echo ""
echo "💡 验收流程:"
echo "   1. 确保网络连接稳定"
echo "   2. 运行系统至少一个完整5分钟周期"
echo "   3. 等待新决策文件生成到 records_new/"
echo "   4. 运行此脚本分析新数据"
echo "   5. 对比基线数据验证改善效果"
echo ""
echo "📊 如需更详细分析:"
echo "   • 查看完整报告: docs/P1_P2_修复报告.md"
echo "   • 查看决策详情: cat records_new/data*.txt | jq ."
echo ""
