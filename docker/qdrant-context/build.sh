#!/usr/bin/env bash
# 构建知识库 RAG 镜像 doctor-agent-qdrant（docker/qdrant-context 独立 context）
#
# 架构（2026-08-30）：
#   只有两个镜像：
#     doctor-agent-qdrant   纯 gz 知识库(alpine 51 数据集) + 标准 Qdrant + 向量
#                           → 只有知识库变化才更新
#     doctor-agent          Go 源码 + 前端 → 只有代码变化才更新
#   doctor-agent-data 已废弃，gz 数据合并进本镜像。
#
# 为什么独立 context：项目根 .dockerignore 排除了 internal/knowledge/gz
# （为了 app 镜像瘦身）。若本镜像也从项目根构建，gz 会被 dockerignore 吞掉。
# 独立 context = 构建上下文里只有 gz + 本 Dockerfile。
#
# 依赖：先构建 doctor-agent（本镜像从它 COPY --from 拿 vector-bake 二进制，
#       不自己编译 Go 源码 → 代码变化不触发本镜像重建）。
#
# 用法:
#   ./docker/qdrant-context/build.sh [镜像名] [app镜像名]
#     默认: doctor-agent-qdrant:latest  工具来自 doctor-agent:latest
set -euo pipefail
cd "$(dirname "$0")"

IMAGE="${1:-doctor-agent-qdrant:latest}"
APP_IMAGE="${2:-doctor-agent:latest}"
REPO_ROOT="$(cd ../.. && pwd)"

echo "== 检查依赖镜像: $APP_IMAGE =="
if ! docker image inspect "$APP_IMAGE" >/dev/null 2>&1; then
  echo "ERROR: 未找到 $APP_IMAGE。请先构建应用镜像："
  echo "  make docker-build   或   docker build -f Dockerfile -t $APP_IMAGE ."
  exit 1
fi

echo "== 同步 gz 到独立 context =="
rm -rf gz
cp -a "$REPO_ROOT/internal/knowledge/gz" gz
CNT=$(ls gz/*.gz 2>/dev/null | wc -l | tr -d ' ')
if [ "$CNT" -eq 0 ]; then
  echo "ERROR: 没有找到 gz 文件（internal/knowledge/gz 为空）"
  exit 1
fi
echo "  已同步 $CNT 个 gz 数据集"

echo "== 构建 RAG 镜像: $IMAGE =="
docker build --platform linux/amd64 \
  --build-arg APP_IMAGE="$APP_IMAGE" \
  -t "$IMAGE" \
  -f Dockerfile \
  --provenance false \
  .

echo "== 完成: $IMAGE =="
echo "  包含: 纯 gz 知识库(51 数据集) + 标准 Qdrant + 烘好的向量"
echo "  触发: 只有知识库变化时重建本镜像"
