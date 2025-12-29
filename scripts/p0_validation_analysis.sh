#!/bin/bash
# P0验收测试分析脚本
# 用途：对比P0修复前后的关键指标变化

echo "================================================"
echo "   P0修复效果验收分析"
echo "================================================"
echo ""

# 定义颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
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

echo "基线数据（P0修复前）:"
echo "$OLD_FILES" | head -3 | while read f; do
    echo "  • $(basename $f) ($(stat -f %Sm -t '%Y-%m-%d %H:%M' $f))"
done

echo ""
echo "最新数据:"
echo "  • $(basename $NEW_FILES) ($(stat -f %Sm -t '%Y-%m-%d %H:%M' $NEW_FILES))"
echo ""

# 分析函数
analyze_metric() {
    local file=$1
    local metric=$2
    local pattern=$3

    count=$(grep -o "$pattern" "$file" | wc -l | tr -d ' ')
    echo $count
}

# 分析struct_pos分布
echo "2️⃣ 分析 struct_pos 分布..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

for file in $OLD_FILES; do
    if [ -f "$file" ]; then
        total=$(grep -c "struct_pos" "$file" 2>/dev/null || echo 0)
        mid_range=$(grep -c '"struct_pos"[[:space:]]*:[[:space:]]*"mid-range"' "$file" 2>/dev/null || echo 0)
        close=$(grep -c '"struct_pos"[[:space:]]*:[[:space:]]*"close"' "$file" 2>/dev/null || echo 0)
        very_close=$(grep -c '"struct_pos"[[:space:]]*:[[:space:]]*"very-close"' "$file" 2>/dev/null || echo 0)

        if [ $total -gt 0 ]; then
            mid_pct=$((mid_range * 100 / total))
            close_pct=$((close * 100 / total))
            very_close_pct=$((very_close * 100 / total))

            echo "$(basename $file):"
            echo "  mid-range: $mid_range/$total (${mid_pct}%)"
            echo "  close: $close/$total (${close_pct}%)"
            echo "  very-close: $very_close/$total (${very_close_pct}%)"
            echo ""
        fi
    fi
done

# 分析DistanceCategory分布
echo "3️⃣ 分析 DistanceCategory 分布..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

for file in $OLD_FILES; do
    if [ -f "$file" ]; then
        total=$(grep -c "DistanceCategory" "$file" 2>/dev/null || echo 0)
        far=$(grep -c '"DistanceCategory"[[:space:]]*:[[:space:]]*"FAR"' "$file" 2>/dev/null || echo 0)
        medium=$(grep -c '"DistanceCategory"[[:space:]]*:[[:space:]]*"MEDIUM"' "$file" 2>/dev/null || echo 0)
        close=$(grep -c '"DistanceCategory"[[:space:]]*:[[:space:]]*"CLOSE"' "$file" 2>/dev/null || echo 0)
        very_close=$(grep -c '"DistanceCategory"[[:space:]]*:[[:space:]]*"VERY_CLOSE"' "$file" 2>/dev/null || echo 0)

        if [ $total -gt 0 ]; then
            far_pct=$((far * 100 / total))
            medium_pct=$((medium * 100 / total))
            close_pct=$((close * 100 / total))
            very_close_pct=$((very_close * 100 / total))

            echo "$(basename $file):"
            echo "  FAR: $far/$total (${far_pct}%)"
            echo "  MEDIUM: $medium/$total (${medium_pct}%)"
            echo "  CLOSE: $close/$total (${close_pct}%)"
            echo "  VERY_CLOSE: $very_close/$total (${very_close_pct}%)"
            echo ""
        fi
    fi
done

# 分析Gate2通过率
echo "4️⃣ 分析 Gate2 通过率..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

for file in $OLD_FILES; do
    if [ -f "$file" ]; then
        total_decisions=$(grep -c '"final_action"' "$file" 2>/dev/null || echo 0)
        gate2_pass=$(grep -c '"STRUCTURE"[[:space:]]*:[[:space:]]*"PASS"' "$file" 2>/dev/null || echo 0)
        gate2_fail=$(grep -c '"STRUCTURE"[[:space:]]*:[[:space:]]*"FAIL"' "$file" 2>/dev/null || echo 0)

        if [ $total_decisions -gt 0 ]; then
            pass_rate=$((gate2_pass * 100 / total_decisions))

            echo "$(basename $file):"
            echo "  总决策数: $total_decisions"
            echo "  Gate2 PASS: $gate2_pass (${pass_rate}%)"
            echo "  Gate2 FAIL: $gate2_fail"
            echo ""
        fi
    fi
done

echo "================================================"
echo "   P0修复预期改善目标:"
echo "================================================"
echo ""
echo "✅ struct_pos mid-range占比: 90% → 40-50%"
echo "✅ DistanceCategory CLOSE/VERY_CLOSE: 15% → 40%+"
echo "✅ Gate2 通过率: +20-30%提升"
echo ""
echo "💡 使用说明:"
echo "   1. 运行系统至少一个完整5分钟周期"
echo "   2. 等待新决策文件生成"
echo "   3. 重新运行此脚本分析新数据"
echo "   4. 对比新旧数据验证改善效果"
echo ""
