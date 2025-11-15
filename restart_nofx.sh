#!/bin/bash

# 简化版Docker重建脚本
set -e

echo "🚀 开始重建Docker服务..."

# 停止服务
echo "⏹️  停止服务..."
docker compose down

# 清理
echo "🧹 清理系统..."
docker system prune -f
docker images | grep "nofx" | awk '{print $3}' | xargs docker rmi -f 2>/dev/null || true

# 重建
echo "🔨 重新构建..."
docker compose build --no-cache

# 启动
echo "🚀 启动服务..."
docker compose up -d

# 显示状态
echo "📊 服务状态:"
docker compose ps

echo "✅ 重建完成!"