#!/bin/bash

# 修复重复函数定义的脚本
cd /Users/guiling/IdeaProjects/nofx

echo "开始清理重复函数..."

# 要删除的函数列表
functions_to_remove=(
    "min"
    "max" 
    "abs"
    "getFVGTypeName"
    "getFVGQualityName"
    "getFVGStatusName"
    "getZoneTypeName"
    "getZoneQualityName"
    "getZoneStatusName"
    "getTrendDirectionName"
    "getTrendQualityName"
    "getFormationTypeName"
    "getFVGSignalTypeName"
    "getSourceName"
    "getRiskLevelName"
    "getMarketPhaseName"
    "getLevelTypeName"
    "getTypeIcon"
    "getSourceIcon"
)

# example文件列表
example_files=(
    "market/supply_demand_example.go"
    "market/fvg_example.go" 
    "market/complete_example.go"
    "market/comprehensive_example.go"
    "market/vpvr_example.go"
)

for file in "${example_files[@]}"; do
    if [[ -f "$file" ]]; then
        echo "处理文件: $file"
        cp "$file" "$file.backup"  # 备份
        
        # 删除重复的函数定义
        for func in "${functions_to_remove[@]}"; do
            # 删除函数定义（从func开始到下一个func或文件末尾）
            awk "
                BEGIN { in_func = 0; func_name = \"$func\" }
                /^func $func\\(/ { in_func = 1; next }
                /^func / && in_func { in_func = 0; print }
                !in_func { print }
            " "$file" > "$file.tmp" && mv "$file.tmp" "$file"
        done
    fi
done

echo "清理完成！"
echo "运行 'go build .' 测试编译..."